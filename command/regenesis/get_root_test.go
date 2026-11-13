/* START ALPHA CHANGE 3201 -- test legacy root command validation and real null RPC responses -- */
package regenesis

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc/codec"
)

type getRootRPCRequest struct {
	ID     json.RawMessage   `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

type getRootRPCFixture struct {
	server   *httptest.Server
	requests atomic.Int32
	received chan getRootRPCRequest
}

func newGetRootRPCFixture(
	t *testing.T,
	result any,
	rpcError *codec.ErrorObject,
) *getRootRPCFixture {
	t.Helper()

	rawResult, err := json.Marshal(result)
	require.NoError(t, err)

	fixture := &getRootRPCFixture{
		received: make(chan getRootRPCRequest, 1),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			fixture.requests.Add(1)
			defer r.Body.Close()

			var request getRootRPCRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			select {
			case fixture.received <- request:
			default:
			}

			response := map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
			}
			if rpcError != nil {
				response["error"] = rpcError
			} else {
				response["result"] = json.RawMessage(rawResult)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		},
	))
	t.Cleanup(fixture.server.Close)

	return fixture
}

func requireGetRootRequest(
	t *testing.T,
	fixture *getRootRPCFixture,
	selector string,
) {
	t.Helper()

	require.Equal(t, int32(1), fixture.requests.Load())
	select {
	case request := <-fixture.received:
		require.Equal(t, "eth_getBlockByNumber", request.Method)
		require.Len(t, request.Params, 2)
		require.Equal(t, fmt.Sprintf("%q", selector), string(request.Params[0]))
		require.Equal(t, "false", string(request.Params[1]))
	default:
		t.Fatal("expected an RPC request")
	}
}

func TestRegenesisGetRoot_BlockSelectors(t *testing.T) {
	tests := []struct {
		name   string
		number int64
		want   string
	}{
		{"zero", 0, "0x0"},
		{"ordinary", 7, "0x7"},
		{"latest", int64(ethgo.Latest), "latest"},
		{"earliest", int64(ethgo.Earliest), "earliest"},
		{"pending", int64(ethgo.Pending), "pending"},
		{
			"client_maximum",
			int64(math.MaxInt),
			fmt.Sprintf("0x%x", uint64(math.MaxInt)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selector, err := legacyRootBlockSelector(test.number)
			require.NoError(t, err)
			require.Equal(t, test.want, selector.String())
		})
	}

	for _, number := range []int64{-4, -100, math.MinInt64} {
		t.Run(fmt.Sprintf("reject_%d", number), func(t *testing.T) {
			_, err := legacyRootBlockSelector(number)
			require.ErrorContains(t, err, "invalid --block")
		})
	}

	if math.MaxInt == math.MaxInt32 {
		_, err := legacyRootBlockSelector(int64(math.MaxInt32) + 1)
		require.ErrorContains(t, err, "exceeds client maximum")
	}
}

type getRootTestOutput struct {
	bytes.Buffer
	reported error
	flushes  int
}

func (o *getRootTestOutput) SetError(err error) {
	o.reported = err
}

func (*getRootTestOutput) SetCommandResult(command.CommandResult) {}

func (o *getRootTestOutput) WriteOutput() {
	o.flushes++
}

func (*getRootTestOutput) WriteCommandResult(command.CommandResult) {}

func TestRegenesisGetRoot_CommandResponses(t *testing.T) {
	tests := []struct {
		name         string
		block        string
		nullResult   bool
		rpcFailure   bool
		zeroRoot     bool
		wantSelector string
		wantError    string
		wantRequests int32
	}{
		{
			name:         "default_latest",
			wantSelector: "latest",
			wantRequests: 1,
		},
		{
			name:         "numeric",
			block:        "7",
			wantSelector: "0x7",
			wantRequests: 1,
		},
		{
			name:         "earliest",
			block:        "-2",
			wantSelector: "earliest",
			wantRequests: 1,
		},
		{
			name:         "pending",
			block:        "-3",
			wantSelector: "pending",
			wantRequests: 1,
		},
		{
			name:         "null_latest",
			nullResult:   true,
			wantSelector: "latest",
			wantError:    "block unavailable: latest",
			wantRequests: 1,
		},
		{
			name:         "unavailable_future_block",
			block:        "101",
			nullResult:   true,
			wantSelector: "0x65",
			wantError:    "block unavailable: 0x65",
			wantRequests: 1,
		},
		{
			name:         "rpc_error",
			block:        "7",
			rpcFailure:   true,
			wantSelector: "0x7",
			wantError:    "injected getroot RPC failure",
			wantRequests: 1,
		},
		{
			name:         "zero_root_is_not_a_missing_block",
			block:        "7",
			zeroRoot:     true,
			wantSelector: "0x7",
			wantRequests: 1,
		},
		{
			name:       "unsupported_negative",
			block:      "-4",
			wantError:  "invalid --block -4",
			nullResult: true,
		},
		{
			name:       "minimum_signed_integer",
			block:      fmt.Sprint(int64(math.MinInt64)),
			wantError:  "invalid --block",
			nullResult: true,
		},
	}

	if math.MaxInt == math.MaxInt32 {
		tests = append(tests, struct {
			name         string
			block        string
			nullResult   bool
			rpcFailure   bool
			zeroRoot     bool
			wantSelector string
			wantError    string
			wantRequests int32
		}{
			name:       "above_client_integer_range",
			block:      fmt.Sprint(int64(math.MaxInt32) + 1),
			nullResult: true,
			wantError:  "exceeds client maximum",
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var root ethgo.Hash
			if !test.zeroRoot {
				root[31] = 0x42
			}

			number := uint64(7)
			switch test.wantSelector {
			case "earliest":
				number = 0
			case "pending":
				number = 8
			}

			var result *ethgo.Block
			if !test.nullResult {
				result = &ethgo.Block{
					Number:     number,
					StateRoot:  root,
					Difficulty: big.NewInt(1),
				}
			}

			var rpcError *codec.ErrorObject
			if test.rpcFailure {
				rpcError = &codec.ErrorObject{
					Code:    -32001,
					Message: "injected getroot RPC failure",
				}
			}

			fixture := newGetRootRPCFixture(t, result, rpcError)
			/* START ALPHA CHANGE 3302 -- inspect root command exit requests without terminating unit tests -- */
			output := new(getRootTestOutput)
			initializations := 0
			var exitCodes []int

			cmd := newGetRootCommand(
				func(cmd *cobra.Command) command.OutputFormatter {
					require.Equal(t, "getroot", cmd.Name())
					initializations++
					return output
				},
				func(code int) {
					require.Equal(t, 1, output.flushes,
						"output must be flushed before requesting process exit")
					exitCodes = append(exitCodes, code)
				},
			)
			/* FINISH ALPHA CHANGE 3302 -- inspect root command exit requests without terminating unit tests -- */
			require.Equal(t,
				fmt.Sprint(int64(ethgo.Latest)),
				cmd.Flags().Lookup("block").DefValue,
			)

			args := []string{"--rpc", fixture.server.URL}
			if test.block != "" {
				args = append(args, "--block", test.block)
			}
			cmd.SetArgs(args)

			require.NotPanics(t, func() {
				require.NoError(t, cmd.Execute())
			})
			/* START ALPHA CHANGE 3302 -- inspect root command exit requests without terminating unit tests -- */
			require.Equal(t, 1, initializations)
			require.Equal(t, 1, output.flushes)
			require.Equal(t, test.wantRequests, fixture.requests.Load())

			if test.wantError != "" {
				require.Equal(t, []int{1}, exitCodes)
			} else {
				require.Empty(t, exitCodes)
			}
			/* FINISH ALPHA CHANGE 3302 -- inspect root command exit requests without terminating unit tests -- */

			if test.wantError != "" {
				require.ErrorContains(t, output.reported, test.wantError)
				require.Empty(t, output.String())

				if test.rpcFailure {
					var cause *codec.ErrorObject
					require.ErrorAs(t, output.reported, &cause)
					require.Equal(t, -32001, cause.Code)
				}
			} else {
				require.NoError(t, output.reported)
				require.Equal(t,
					fmt.Sprintf("state root %s for block %d\n", root, number),
					output.String(),
				)
			}

			if test.wantRequests == 0 {
				require.Empty(t, fixture.received)
			} else {
				requireGetRootRequest(t, fixture, test.wantSelector)
			}
		})
	}
}

type getRootFailingWriter struct {
	calls int
	cause error
	short bool
}

func (w *getRootFailingWriter) Write(data []byte) (int, error) {
	w.calls++
	if w.short {
		return len(data) - 1, nil
	}
	return 0, w.cause
}

func TestRegenesisGetRoot_OutputFailures(t *testing.T) {
	cause := errors.New("injected state-root output failure")

	for _, test := range []struct {
		name  string
		short bool
		want  error
	}{
		{"write_error", false, cause},
		{"short_write", true, io.ErrShortWrite},
	} {
		t.Run(test.name, func(t *testing.T) {
			var root ethgo.Hash
			root[31] = 0x42

			fixture := newGetRootRPCFixture(t, &ethgo.Block{
				Number:     7,
				StateRoot:  root,
				Difficulty: big.NewInt(1),
			}, nil)
			output := &getRootFailingWriter{
				cause: cause,
				short: test.short,
			}

			err := runLegacyGetRoot(fixture.server.URL, 7, output)
			require.ErrorContains(t, err, "write state root")
			require.ErrorIs(t, err, test.want)
			require.Equal(t, 1, output.calls)
			requireGetRootRequest(t, fixture, "0x7")
		})
	}
}

/* FINISH ALPHA CHANGE 3201 -- test legacy root command validation and real null RPC responses -- */
