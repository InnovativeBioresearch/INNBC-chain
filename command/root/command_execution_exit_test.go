/* START ALPHA CHANGE 3304 -- verify retained command execution statuses through real subprocesses -- */
package root

/* START ALPHA CHANGE 3351 -- support operator command subprocess and termination regressions -- */
import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
	txpoolProto "github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

/* FINISH ALPHA CHANGE 3351 -- support operator command subprocess and termination regressions -- */

const commandExecutionExitChild = "INNBC_COMMAND_EXECUTION_EXIT_CHILD"

func TestCommandExecutionExit_Subprocess(t *testing.T) {
	if os.Getenv(commandExecutionExitChild) != "1" {
		return
	}

	separator := slices.Index(os.Args, "--")
	if separator < 0 {
		fmt.Fprintln(os.Stderr, "missing command subprocess argument separator")
		os.Exit(2)
	}

	/* START ALPHA CHANGE 3351 -- exercise repeated peer additions without manufacturing failure status -- */
	runs := 1
	if os.Getenv("INNBC_OPERATOR_REPEAT_ADD") == "1" {
		runs = 2
	}

	for i := 0; i < runs; i++ {
		root := NewRootCommand()
		root.baseCmd.SetArgs(os.Args[separator+1:])
		root.Execute()
	}
	/* FINISH ALPHA CHANGE 3351 -- exercise repeated peer additions without manufacturing failure status -- */

	// A normal return must remain success. Only the real command or root
	// handler may produce the failure status expected by the parent.
	os.Exit(0)
}

func commandExecutionExitRun(
	t *testing.T,
	dir string,
	jsonOutput bool,
	args ...string,
) (string, string, error) {
	t.Helper()

	executable, err := os.Executable()
	require.NoError(t, err)

	if jsonOutput {
		args = append([]string{"--json"}, args...)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	childArgs := append(
		[]string{"-test.run=^TestCommandExecutionExit_Subprocess$", "--"},
		args...,
	)
	process := exec.CommandContext(ctx, executable, childArgs...)
	process.Dir = dir
	process.Env = append(os.Environ(), commandExecutionExitChild+"=1")

	var stdout, stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr

	err = process.Run()
	require.NoError(t, ctx.Err(), "command timed out")

	return stdout.String(), stderr.String(), err
}

func commandExecutionExitFailure(
	t *testing.T,
	stdout string,
	stderr string,
	err error,
	jsonOutput bool,
	wantMessage string,
) {
	t.Helper()

	var exitError *exec.ExitError
	require.True(t, errors.As(err, &exitError),
		"expected failed subprocess; err=%v\nstdout:\n%s\nstderr:\n%s",
		err, stdout, stderr)
	require.Equal(t, 1, exitError.ExitCode())
	require.Contains(t, stderr, wantMessage)

	for _, success := range []string{
		"[GENESIS SUCCESS]",
		"[EXPORT SUCCESS]",
		"[CLIENT STATUS]",
		"state root ",
		`"message":`,
		`"export_result":`,
		`"current_block_number":`,
	} {
		require.NotContains(t, stdout, success)
	}

	if jsonOutput {
		// Parsing the whole stderr also rejects duplicate plain-text errors.
		var response map[string]string
		require.NoError(t, json.Unmarshal(
			[]byte(strings.TrimSpace(stderr)), &response,
		))
		require.Len(t, response, 1)
		require.Contains(t, response["error"], wantMessage)
	}
}

func commandExecutionExitValidator(t *testing.T) string {
	t.Helper()

	key, err := bls.UnmarshalPrivateKey([]byte("7"))
	require.NoError(t, err)

	operator := types.StringToAddress("0x10001")
	proof, err := signer.MakeINNBCBLSPoP(key, operator)
	require.NoError(t, err)
	proofBytes, err := proof.Marshal()
	require.NoError(t, err)

	/* START ALPHA CHANGE 3848 -- provide a complete peer identity in shared CLI genesis fixtures -- */
	return strings.Join([]string{
		"/ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAm22et517T1XhG181xGUDikDxUFrek4CuYTgUJjC38vxZR",
		operator.String(),
		"0x" + hex.EncodeToString(key.PublicKey().Marshal()),
		"0x" + hex.EncodeToString(proofBytes),
	}, ":")
	/* FINISH ALPHA CHANGE 3848 -- provide a complete peer identity in shared CLI genesis fixtures -- */
}

func TestCommandExecutionExit_Genesis(t *testing.T) {
	validator := commandExecutionExitValidator(t)

	for _, jsonOutput := range []bool{false, true} {
		for _, scenario := range []string{
			"no_validators",
			"publication_failure",
			"success",
		} {
			t.Run(fmt.Sprintf("%s/json=%t", scenario, jsonOutput), func(t *testing.T) {
				dir := t.TempDir()
				output := filepath.Join(dir, "genesis.json")
				args := []string{"genesis", "--dir", output}
				wantError := ""

				switch scenario {
				case "no_validators":
					// An existing empty directory passes path validation.
					// Generation then fails because it contains no validators.
					args = append(args, "--validators-path", t.TempDir())
					wantError = "genesis validators aren't provided"

				case "publication_failure":
					// The output does not exist, so pre-run validation passes.
					// Its missing parent makes the final publication fail.
					output = filepath.Join(dir, "missing", "genesis.json")
					args = []string{
						"genesis", "--dir", output,
						"--validators", validator,
					}
					wantError = "failed to write genesis"

				case "success":
					args = append(args, "--validators", validator)
				}

				stdout, stderr, err := commandExecutionExitRun(
					t, dir, jsonOutput, args...,
				)

				if wantError != "" {
					commandExecutionExitFailure(
						t, stdout, stderr, err, jsonOutput, wantError,
					)
					require.NoFileExists(t, output)
					return
				}

				require.NoError(t, err, "stderr:\n%s", stderr)
				require.Empty(t, strings.TrimSpace(stderr))

				generated, err := chain.ImportFromFile(output)
				require.NoError(t, err)
				require.NoError(t, generated.ValidateGenesisAllocations())

				if jsonOutput {
					// Genesis retains its existing validator progress output.
					// Its final nonempty line is the JSON command result.
					lines := strings.Split(strings.TrimSpace(stdout), "\n")
					var result map[string]string
					require.NoError(t, json.Unmarshal(
						[]byte(lines[len(lines)-1]), &result,
					))
					require.Len(t, result, 1)
					require.Equal(t,
						fmt.Sprintf("\nGenesis written to %s\n", output),
						result["message"],
					)
				} else {
					require.Contains(t, stdout, "[GENESIS SUCCESS]")
					require.Contains(t, stdout, output)
				}
			})
		}
	}
}

func TestCommandExecutionExit_ServerExport(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		for _, scenario := range []string{
			"unsupported_type",
			"output_failure",
			"success",
		} {
			t.Run(fmt.Sprintf("%s/json=%t", scenario, jsonOutput), func(t *testing.T) {
				dir := t.TempDir()
				output := filepath.Join(dir, "default-config.json")
				args := []string{"server", "export", "--type", "json"}
				wantError := ""
				marker := ""

				switch scenario {
				case "unsupported_type":
					args = []string{"server", "export", "--type", "unsupported"}
					wantError = "invalid file type"

				case "output_failure":
					// A directory at the destination causes a deterministic
					// failure without depending on permissions or disk space.
					require.NoError(t, os.Mkdir(output, 0700))
					marker = filepath.Join(output, "preserve")
					require.NoError(t, os.WriteFile(
						marker, []byte("preserve existing destination"), 0600,
					))
					wantError = "failed to create config file"
				}

				stdout, stderr, err := commandExecutionExitRun(
					t, dir, jsonOutput, args...,
				)

				if wantError != "" {
					commandExecutionExitFailure(
						t, stdout, stderr, err, jsonOutput, wantError,
					)
					require.Empty(t, strings.TrimSpace(stdout))

					if marker != "" {
						data, readErr := os.ReadFile(marker)
						require.NoError(t, readErr)
						require.Equal(t,
							"preserve existing destination", string(data),
						)
					} else {
						require.NoFileExists(t, output)
					}

					return
				}

				require.NoError(t, err, "stderr:\n%s", stderr)
				require.Empty(t, strings.TrimSpace(stderr))

				data, err := os.ReadFile(output)
				require.NoError(t, err)
				require.True(t, json.Valid(data))

				if jsonOutput {
					var result map[string]string
					require.NoError(t, json.Unmarshal([]byte(stdout), &result))
					require.Equal(t, map[string]string{
						"export_result": "Configuration file successfully exported",
					}, result)
				} else {
					require.Contains(t, stdout, "[EXPORT SUCCESS]")
					require.Contains(t, stdout,
						"Configuration file successfully exported")
				}
			})
		}
	}
}

func TestCommandExecutionExit_GetRoot(t *testing.T) {
	var stateRoot ethgo.Hash
	stateRoot[31] = 0x42

	for _, jsonOutput := range []bool{false, true} {
		for _, scenario := range []string{"rpc_error", "null_result", "success"} {
			t.Run(fmt.Sprintf("%s/json=%t", scenario, jsonOutput), func(t *testing.T) {
				var calls atomic.Int32

				rpc := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						defer r.Body.Close()

						var request struct {
							ID     json.RawMessage `json:"id"`
							Method string          `json:"method"`
						}
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						if request.Method != "eth_getBlockByNumber" {
							http.Error(w, "unexpected RPC method", http.StatusBadRequest)
							return
						}

						response := map[string]any{
							"jsonrpc": "2.0",
							"id":      request.ID,
						}

						switch scenario {
						case "rpc_error":
							response["error"] = map[string]any{
								"code":    -32001,
								"message": "controlled getroot RPC failure",
							}
						case "null_result":
							response["result"] = nil
						case "success":
							response["result"] = &ethgo.Block{
								Number:     7,
								StateRoot:  stateRoot,
								Difficulty: big.NewInt(1),
							}
						}

						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(response)
					},
				))
				t.Cleanup(rpc.Close)

				stdout, stderr, err := commandExecutionExitRun(
					t, t.TempDir(), jsonOutput,
					"regenesis", "getroot",
					"--rpc", rpc.URL, "--block", "7",
				)
				require.Equal(t, int32(1), calls.Load())

				if scenario != "success" {
					wantError := "controlled getroot RPC failure"
					if scenario == "null_result" {
						wantError = "block unavailable: 0x7"
					}
					commandExecutionExitFailure(
						t, stdout, stderr, err, jsonOutput, wantError,
					)
					require.Empty(t, strings.TrimSpace(stdout))
					return
				}

				require.NoError(t, err, "stderr:\n%s", stderr)
				require.Empty(t, strings.TrimSpace(stderr))

				// Preserve getroot's existing textual success output in both modes.
				require.Equal(t,
					fmt.Sprintf("state root %s for block 7", stateRoot),
					strings.TrimSpace(stdout),
				)
			})
		}
	}
}

type commandExecutionExitStatusServer struct {
	proto.UnimplementedSystemServer
	fail  bool
	calls atomic.Int32
}

func (s *commandExecutionExitStatusServer) GetStatus(
	context.Context,
	*empty.Empty,
) (*proto.ServerStatus, error) {
	s.calls.Add(1)

	if s.fail {
		return nil, grpcstatus.Error(
			codes.Unavailable, "controlled status RPC failure",
		)
	}

	return &proto.ServerStatus{
		Network: 100,
		Current: &proto.ServerStatus_Block{
			Number: 7,
			Hash:   "0x1234",
		},
		P2PAddr: "/ip4/127.0.0.1/tcp/30301",
	}, nil
}

func TestCommandExecutionExit_Status(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("failure=%t/json=%t", fail, jsonOutput), func(t *testing.T) {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)

				service := &commandExecutionExitStatusServer{fail: fail}
				server := grpc.NewServer()
				proto.RegisterSystemServer(server, service)

				done := make(chan struct{})
				go func() {
					defer close(done)
					_ = server.Serve(listener)
				}()
				t.Cleanup(func() {
					server.Stop()
					<-done
				})

				stdout, stderr, err := commandExecutionExitRun(
					t, t.TempDir(), jsonOutput,
					"status", "--grpc-address", listener.Addr().String(),
				)
				require.Equal(t, int32(1), service.calls.Load())

				if fail {
					commandExecutionExitFailure(
						t, stdout, stderr, err, jsonOutput,
						"controlled status RPC failure",
					)
					require.Empty(t, strings.TrimSpace(stdout))
					return
				}

				require.NoError(t, err, "stderr:\n%s", stderr)
				require.Empty(t, strings.TrimSpace(stderr))

				if jsonOutput {
					var result map[string]any
					require.NoError(t, json.Unmarshal([]byte(stdout), &result))
					require.Equal(t, map[string]any{
						"chain_id":             float64(100),
						"current_block_number": float64(7),
						"current_block_hash":   "0x1234",
						"libp2p_address":       "/ip4/127.0.0.1/tcp/30301",
					}, result)
				} else {
					require.Contains(t, stdout, "[CLIENT STATUS]")
					require.Contains(t, stdout, "0x1234")
					require.Contains(t, stdout, "/ip4/127.0.0.1/tcp/30301")
				}
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3304 -- verify retained command execution statuses through real subprocesses -- */

/* START ALPHA CHANGE 3351 -- verify additional operator command statuses and subscription cleanup -- */

const (
	operatorExitFailureMessage = "controlled operator RPC failure"
	operatorExitGoodAddress    = "/ip4/127.0.0.1/tcp/30301"
	operatorExitBadAddress     = "/ip4/127.0.0.1/tcp/30302"
)

type operatorExitSystemService struct {
	proto.UnimplementedSystemServer
	mode     string
	calls    atomic.Int32
	finished chan struct{}
}

func (s *operatorExitSystemService) PeersList(
	context.Context,
	*empty.Empty,
) (*proto.PeersListResponse, error) {
	s.calls.Add(1)

	if s.mode == "failure" {
		return nil, grpcstatus.Error(codes.Unavailable, operatorExitFailureMessage)
	}

	return &proto.PeersListResponse{
		Peers: []*proto.Peer{{Id: "peer1"}},
	}, nil
}

func (s *operatorExitSystemService) PeersStatus(
	_ context.Context,
	request *proto.PeersStatusRequest,
) (*proto.Peer, error) {
	s.calls.Add(1)

	if s.mode == "failure" {
		return nil, grpcstatus.Error(codes.Unavailable, operatorExitFailureMessage)
	}

	return &proto.Peer{
		Id:        request.Id,
		Protocols: []string{"protocol1"},
		Addrs:     []string{operatorExitGoodAddress},
	}, nil
}

func (s *operatorExitSystemService) PeersAdd(
	_ context.Context,
	request *proto.PeersAddRequest,
) (*proto.PeersAddResponse, error) {
	call := s.calls.Add(1)

	if s.mode == "failure" ||
		(s.mode == "mixed" && request.Id == operatorExitBadAddress) ||
		(s.mode == "repeat" && call > 2) {
		return nil, grpcstatus.Error(codes.Unavailable, operatorExitFailureMessage)
	}

	return &proto.PeersAddResponse{Message: "added"}, nil
}

func (s *operatorExitSystemService) Subscribe(
	_ *empty.Empty,
	stream proto.System_SubscribeServer,
) error {
	return operatorExitStream(
		stream.Context(),
		s.mode,
		s.finished,
		func() error {
			return stream.Send(&proto.BlockchainEvent{
				Added: []*proto.BlockchainEvent_Header{
					{Number: 7, Hash: "0x1234"},
				},
			})
		},
	)
}

type operatorExitPoolService struct {
	txpoolProto.UnimplementedTxnPoolOperatorServer
	mode     string
	calls    atomic.Int32
	finished chan struct{}
}

func (s *operatorExitPoolService) Status(
	context.Context,
	*empty.Empty,
) (*txpoolProto.TxnPoolStatusResp, error) {
	s.calls.Add(1)

	if s.mode == "failure" {
		return nil, grpcstatus.Error(codes.Unavailable, operatorExitFailureMessage)
	}

	return &txpoolProto.TxnPoolStatusResp{Length: 3}, nil
}

func (s *operatorExitPoolService) Subscribe(
	_ *txpoolProto.SubscribeRequest,
	stream txpoolProto.TxnPoolOperator_SubscribeServer,
) error {
	return operatorExitStream(
		stream.Context(),
		s.mode,
		s.finished,
		func() error {
			return stream.Send(&txpoolProto.TxPoolEvent{
				Type:   txpoolProto.EventType_ADDED,
				TxHash: "0x1234",
			})
		},
	)
}

func operatorExitStream(
	ctx context.Context,
	mode string,
	finished chan struct{},
	send func() error,
) error {
	defer close(finished)

	if mode == "setup_failure" {
		return grpcstatus.Error(codes.Unavailable, operatorExitFailureMessage)
	}

	if err := send(); err != nil {
		return err
	}

	switch mode {
	case "receive_failure":
		return grpcstatus.Error(codes.Unavailable, operatorExitFailureMessage)
	case "remote_cancel":
		return grpcstatus.Error(codes.Canceled, operatorExitFailureMessage)
	case "terminate":
		<-ctx.Done()
		return ctx.Err()
	default:
		return nil
	}
}

func operatorExitStartServer(
	t *testing.T,
	mode string,
) (string, *operatorExitSystemService, *operatorExitPoolService) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	system := &operatorExitSystemService{
		mode:     mode,
		finished: make(chan struct{}),
	}
	pool := &operatorExitPoolService{
		mode:     mode,
		finished: make(chan struct{}),
	}

	server := grpc.NewServer()
	proto.RegisterSystemServer(server, system)
	txpoolProto.RegisterTxnPoolOperatorServer(server, pool)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		<-done
	})

	return listener.Addr().String(), system, pool
}

func operatorExitAssertCode(
	t *testing.T,
	err error,
	want int,
	stdout string,
	stderr string,
) {
	t.Helper()

	if want == 0 {
		require.NoError(t, err, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
		return
	}

	var exitError *exec.ExitError
	require.True(t, errors.As(err, &exitError),
		"expected exit %d; err=%v\nstdout:\n%s\nstderr:\n%s",
		want, err, stdout, stderr)
	require.Equal(t, want, exitError.ExitCode())
}

/* START ALPHA CHANGE 3527 -- verify peer-directory refresh process status and preserved output -- */
func (s *operatorExitSystemService) RefreshPeerDirectory(
	context.Context,
	*empty.Empty,
) (*proto.RefreshPeerDirectoryResponse, error) {
	s.calls.Add(1)

	if s.mode == "failure" {
		return nil, grpcstatus.Error(codes.Unavailable, operatorExitFailureMessage)
	}

	return &proto.RefreshPeerDirectoryResponse{
		Peers: []string{operatorExitGoodAddress},
	}, nil
}

func TestOperatorCommandExit_PeerDirectoryRefresh(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		for _, scenario := range []string{
			"success",
			"rpc_rejection",
			"unavailable_endpoint",
		} {
			t.Run(fmt.Sprintf("%s/json=%t", scenario, jsonOutput), func(t *testing.T) {
				/* START ALPHA CHANGE 3529 -- use a controlled unusable RPC endpoint without port-zero connection delays -- */
				address := ""
				var system *operatorExitSystemService
				wantError := ""

				switch scenario {
				case "success":
					address, system, _ = operatorExitStartServer(t, "success")
				case "rpc_rejection":
					address, system, _ = operatorExitStartServer(t, "failure")
					wantError = operatorExitFailureMessage
				case "unavailable_endpoint":
					// Reserve the address throughout the test, but provide no
					// usable RPC transport. Close each connection immediately.
					listener, err := net.Listen("tcp", "127.0.0.1:0")
					require.NoError(t, err)

					done := make(chan struct{})
					go func() {
						defer close(done)

						for {
							conn, err := listener.Accept()
							if err != nil {
								return
							}

							_ = conn.Close()
						}
					}()

					t.Cleanup(func() {
						_ = listener.Close()
						<-done
					})

					address = listener.Addr().String()
					wantError = "Unavailable"
				}
				/* FINISH ALPHA CHANGE 3529 -- use a controlled unusable RPC endpoint without port-zero connection delays -- */

				stdout, stderr, err := commandExecutionExitRun(
					t, t.TempDir(), jsonOutput,
					"peerdirectory", "refresh",
					"--grpc-address", address,
				)

				if system != nil {
					require.Equal(t, int32(1), system.calls.Load())
				}

				if wantError != "" {
					commandExecutionExitFailure(
						t, stdout, stderr, err, jsonOutput, wantError,
					)
					require.Empty(t, strings.TrimSpace(stdout))
					require.Equal(t, 1, strings.Count(stderr, wantError),
						"expected exactly one terminal error report")
					return
				}

				operatorExitAssertCode(t, err, 0, stdout, stderr)
				require.Empty(t, strings.TrimSpace(stderr))

				if jsonOutput {
					var result map[string][]string
					require.NoError(t, json.Unmarshal([]byte(stdout), &result))
					require.Equal(t, map[string][]string{
						"peers": {operatorExitGoodAddress},
					}, result)
				} else {
					require.Equal(t, 1, strings.Count(
						stdout, "[PEER DIRECTORY REFRESHED]",
					))
					require.Contains(t, stdout, "Number of peers: 1")
					require.Contains(t, stdout, operatorExitGoodAddress)
				}
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3527 -- verify peer-directory refresh process status and preserved output -- */

func TestOperatorCommandExit_Unary(t *testing.T) {
	commands := []struct {
		name   string
		args   []string
		marker string
	}{
		{
			name:   "peers_list",
			args:   []string{"peers", "list"},
			marker: "[PEERS LIST]",
		},
		{
			name:   "peers_status",
			args:   []string{"peers", "status", "--peer-id", "peer1"},
			marker: "[PEER STATUS]",
		},
		{
			name:   "txpool_status",
			args:   []string{"txpool", "status"},
			marker: "[TXPOOL STATUS]",
		},
		{
			name: "peers_add",
			args: []string{
				"peers", "add",
				"--addr", operatorExitGoodAddress,
				"--addr", operatorExitBadAddress,
			},
			marker: "[PEERS ADDED]",
		},
	}

	for _, test := range commands {
		for _, jsonOutput := range []bool{false, true} {
			for _, mode := range []string{"success", "failure", "mixed"} {
				if mode == "mixed" && test.name != "peers_add" {
					continue
				}

				t.Run(fmt.Sprintf("%s/%s/json=%t",
					test.name, mode, jsonOutput), func(t *testing.T) {
					address, system, pool := operatorExitStartServer(t, mode)
					args := append([]string{}, test.args...)
					args = append(args, "--grpc-address", address)

					stdout, stderr, err := commandExecutionExitRun(
						t, t.TempDir(), jsonOutput, args...,
					)

					wantCode := 0
					if mode != "success" {
						wantCode = 1
					}
					operatorExitAssertCode(t, err, wantCode, stdout, stderr)

					wantCalls := int32(1)
					if test.name == "peers_add" {
						wantCalls = 2
					}
					if test.name == "txpool_status" {
						require.Equal(t, wantCalls, pool.calls.Load())
					} else {
						require.Equal(t, wantCalls, system.calls.Load())
					}

					if test.name != "peers_add" && mode == "failure" {
						commandExecutionExitFailure(
							t, stdout, stderr, err, jsonOutput,
							operatorExitFailureMessage,
						)
						require.Empty(t, strings.TrimSpace(stdout))
						return
					}

					require.Empty(t, strings.TrimSpace(stderr))

					if test.name == "peers_add" {
						wantAdded := 2
						switch mode {
						case "failure":
							wantAdded = 0
						case "mixed":
							wantAdded = 1
						}

						if jsonOutput {
							var result struct {
								NumRequested int      `json:"num_requested"`
								NumAdded     int      `json:"num_added"`
								Peers        []string `json:"peers"`
								Errors       []string `json:"errors"`
							}
							require.NoError(t, json.Unmarshal([]byte(stdout), &result))
							require.Equal(t, 2, result.NumRequested)
							require.Equal(t, wantAdded, result.NumAdded)
							require.Len(t, result.Peers, wantAdded)
							require.Len(t, result.Errors, 2-wantAdded)
							if wantAdded > 0 {
								require.Equal(t, operatorExitGoodAddress, result.Peers[0])
							}
							for _, message := range result.Errors {
								require.Contains(t, message, operatorExitFailureMessage)
							}
						} else {
							require.Contains(t, stdout, test.marker)
							require.Equal(t, 2-wantAdded,
								strings.Count(stdout, operatorExitFailureMessage))
							if wantAdded > 0 {
								require.Contains(t, stdout, operatorExitGoodAddress)
							}
						}

						return
					}

					if jsonOutput {
						var result map[string]any
						require.NoError(t, json.Unmarshal([]byte(stdout), &result))

						switch test.name {
						case "peers_list":
							require.Equal(t, []any{"peer1"}, result["peers"])
						case "peers_status":
							require.Equal(t, "peer1", result["id"])
							require.Equal(t, []any{"protocol1"}, result["protocols"])
							require.Equal(t, []any{operatorExitGoodAddress},
								result["addresses"])
						case "txpool_status":
							require.Equal(t, float64(3), result["transactions"])
						}
					} else {
						require.Contains(t, stdout, test.marker)
						if test.name != "txpool_status" {
							require.Contains(t, stdout, "peer1")
						}
					}
				})
			}
		}
	}
}

func TestOperatorCommandExit_RepeatedAdd(t *testing.T) {
	t.Setenv("INNBC_OPERATOR_REPEAT_ADD", "1")

	address, system, _ := operatorExitStartServer(t, "repeat")
	stdout, stderr, err := commandExecutionExitRun(
		t, t.TempDir(), true,
		"peers", "add",
		"--grpc-address", address,
		"--addr", operatorExitGoodAddress,
		"--addr", operatorExitBadAddress,
	)

	operatorExitAssertCode(t, err, 1, stdout, stderr)
	require.Empty(t, strings.TrimSpace(stderr))
	require.Equal(t, int32(4), system.calls.Load())

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var first, second struct {
		NumRequested int      `json:"num_requested"`
		NumAdded     int      `json:"num_added"`
		Peers        []string `json:"peers"`
		Errors       []string `json:"errors"`
	}
	require.NoError(t, decoder.Decode(&first))
	require.NoError(t, decoder.Decode(&second))

	require.Equal(t, 2, first.NumRequested)
	require.Equal(t, 2, first.NumAdded)
	require.Len(t, first.Peers, 2)
	require.Empty(t, first.Errors)

	require.Equal(t, 2, second.NumRequested)
	require.Zero(t, second.NumAdded)
	require.Empty(t, second.Peers)
	require.Len(t, second.Errors, 2)

	var extra any
	require.ErrorIs(t, decoder.Decode(&extra), io.EOF)
}

func operatorExitRunStopped(
	t *testing.T,
	jsonOutput bool,
	marker string,
	args ...string,
) (string, string, error) {
	t.Helper()

	executable, err := os.Executable()
	require.NoError(t, err)

	if jsonOutput {
		args = append([]string{"--json"}, args...)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	childArgs := append(
		[]string{"-test.run=^TestCommandExecutionExit_Subprocess$", "--"},
		args...,
	)
	process := exec.CommandContext(ctx, executable, childArgs...)
	process.Dir = t.TempDir()
	process.Env = append(os.Environ(), commandExecutionExitChild+"=1")

	pipe, err := process.StdoutPipe()
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	process.Stderr = &stderr
	require.NoError(t, process.Start())

	scanner := bufio.NewScanner(pipe)
	signaled := false
	var signalError error

	for scanner.Scan() {
		line := scanner.Text()
		stdout.WriteString(line)
		stdout.WriteByte('\n')

		if !signaled && strings.Contains(line, marker) {
			signaled = true
			signalError = process.Process.Signal(syscall.SIGTERM)
		}
	}

	waitError := process.Wait()
	require.NoError(t, ctx.Err(), "subscription did not stop promptly")
	require.NoError(t, scanner.Err())
	require.True(t, signaled, "subscription never printed its first event")
	require.NoError(t, signalError)

	return stdout.String(), stderr.String(), waitError
}

func TestOperatorCommandExit_Streams(t *testing.T) {
	for _, name := range []string{"monitor", "txpool_subscribe"} {
		for _, jsonOutput := range []bool{false, true} {
			for _, mode := range []string{
				"setup_failure",
				"receive_failure",
				"remote_cancel",
				"eof",
				"terminate",
			} {
				t.Run(fmt.Sprintf("%s/%s/json=%t",
					name, mode, jsonOutput), func(t *testing.T) {
					address, system, pool := operatorExitStartServer(t, mode)

					args := []string{"monitor", "--grpc-address", address}
					marker := "[BLOCK EVENT]"
					finished := system.finished

					if name == "txpool_subscribe" {
						args = []string{
							"txpool", "subscribe", "--grpc-address", address,
						}
						marker = "[TXPOOL EVENT]"
						finished = pool.finished
					}

					if jsonOutput {
						marker = `"events":`
						if name == "txpool_subscribe" {
							marker = `"tx_hash":`
						}
					}

					var stdout, stderr string
					var err error
					if mode == "terminate" {
						stdout, stderr, err = operatorExitRunStopped(
							t, jsonOutput, marker, args...,
						)
					} else {
						stdout, stderr, err = commandExecutionExitRun(
							t, t.TempDir(), jsonOutput, args...,
						)
					}

					switch mode {
					case "setup_failure", "receive_failure", "remote_cancel":
						commandExecutionExitFailure(
							t, stdout, stderr, err, jsonOutput,
							operatorExitFailureMessage,
						)
						require.Equal(t, 1,
							strings.Count(stderr, operatorExitFailureMessage))
					default:
						operatorExitAssertCode(t, err, 0, stdout, stderr)
						require.Empty(t, strings.TrimSpace(stderr))
					}

					if mode == "setup_failure" {
						require.Empty(t, strings.TrimSpace(stdout))
					} else {
						// Reject duplicate final event output.
						require.Equal(t, 1, strings.Count(stdout, marker))
						require.Contains(t, stdout, "0x1234")
						if jsonOutput {
							var event map[string]json.RawMessage
							require.NoError(t, json.Unmarshal([]byte(stdout), &event))
						}
					}

					select {
					case <-finished:
					case <-time.After(5 * time.Second):
						t.Fatal("subscription RPC did not release its handler")
					}
				})
			}
		}
	}
}

/* FINISH ALPHA CHANGE 3351 -- verify additional operator command statuses and subscription cleanup -- */
