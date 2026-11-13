/* START ALPHA CHANGE 2175 -- test local state failure classification through RPC dispatch -- */
package jsonrpc

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/stretchr/testify/require"
)

type issue7ADispatchService struct {
	result interface{}
	err    error
}

func (s *issue7ADispatchService) Read() (interface{}, error) {
	return s.result, s.err
}

func TestIssue7A_DispatcherLocalStateFailureIsInternalError(t *testing.T) {
	for _, result := range []interface{}{nil, "unreliable result"} {
		t.Run(map[bool]string{true: "nil", false: "discard result"}[result == nil], func(t *testing.T) {
			dispatcher := &Dispatcher{
				logger: hclog.NewNullLogger(),
				params: &dispatcherParams{},
			}
			cause := errors.New("injected storage failure")
			service := &issue7ADispatchService{
				result: result,
				err:    state.WrapLocalStateFailure("read", cause),
			}
			require.NoError(t, dispatcher.registerService("issue7a", service))

			body, err := dispatcher.Handle([]byte(
				`{"jsonrpc":"2.0","id":1,"method":"issue7a_read","params":[]}`,
			))
			require.NoError(t, err)

			var response struct {
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(body, &response))
			require.NotNil(t, response.Error)
			require.Equal(t, -32603, response.Error.Code)
			require.Contains(t, response.Error.Message, cause.Error())
			require.True(t, len(response.Result) == 0 || string(response.Result) == "null")
		})
	}
}

func TestIssue7A_DispatcherOrdinaryErrorClassificationUnchanged(t *testing.T) {
	dispatcher := &Dispatcher{
		logger: hclog.NewNullLogger(),
		params: &dispatcherParams{},
	}
	require.NoError(t, dispatcher.registerService(
		"issue7a",
		&issue7ADispatchService{err: errors.New("ordinary endpoint error")},
	))

	_, err := dispatcher.handleReq(Request{
		Method: "issue7a_read",
		Params: []byte("[]"),
	})
	require.Error(t, err)
	require.Equal(t, -32600, err.ErrorCode())
}

func TestIssue7A_StateEndpointsReturnInternalErrorsOverRPC(t *testing.T) {
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"eth_getBalance","params":["0x0000000000000000000000000000000000007401","latest"]}`,
		`{"jsonrpc":"2.0","id":1,"method":"eth_getStorageAt","params":["0x0000000000000000000000000000000000007401","0x0","latest"]}`,
		`{"jsonrpc":"2.0","id":1,"method":"eth_getCode","params":["0x0000000000000000000000000000000000007401","latest"]}`,
		`{"jsonrpc":"2.0","id":1,"method":"eth_getTransactionCount","params":["0x0000000000000000000000000000000000007401","latest"]}`,
		`{"jsonrpc":"2.0","id":1,"method":"eth_getTransactionCount","params":["0x0000000000000000000000000000000000007401","pending"]}`,
	}

	for _, request := range requests {
		t.Run(request, func(t *testing.T) {
			endpoint, store := issue7AEndpointFixture()
			local := state.WrapLocalStateFailure("read", ErrStateNotFound)
			store.readErr = local
			store.nonceErr = local

			dispatcher := &Dispatcher{
				logger: hclog.NewNullLogger(),
				params: &dispatcherParams{},
			}
			require.NoError(t, dispatcher.registerService("eth", endpoint))

			body, err := dispatcher.Handle([]byte(request))
			require.NoError(t, err)

			var response struct {
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Code int `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(body, &response))
			require.NotNil(t, response.Error)
			require.Equal(t, -32603, response.Error.Code)
			require.True(t, len(response.Result) == 0 || string(response.Result) == "null")
		})
	}
}

/* FINISH ALPHA CHANGE 2175 -- test local state failure classification through RPC dispatch -- */
