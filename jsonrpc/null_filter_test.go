/* START ALPHA CHANGE 2514 -- test null filter rejection and subsequent background log delivery -- */
package jsonrpc

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func newNullFilterTestDispatcher(t *testing.T) (*Dispatcher, *mockStore) {
	t.Helper()

	store := newMockStore()
	store.header = &types.Header{
		Number: 1,
		Hash:   types.StringToHash("0x01"),
	}
	store.addHeader(store.header)

	d := newTestDispatcher(t, hclog.NewNullLogger(), store, &dispatcherParams{
		jsonRPCBatchLengthLimit: 20,
		blockRangeLimit:         256,
	})
	t.Cleanup(d.Close)

	return d, store
}

func callNullFilterTestRPC(
	t *testing.T,
	d *Dispatcher,
	conn wsConn,
	method string,
	params string,
	wantCode int,
) json.RawMessage {
	t.Helper()

	request := fmt.Appendf(
		nil,
		`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`,
		method,
		params,
	)

	var (
		payload []byte
		err     error
	)
	if conn == nil {
		payload, err = d.Handle(request)
	} else {
		payload, err = d.HandleWs(request, conn)
	}
	require.NoError(t, err)

	var response struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(payload, &response))

	if wantCode != 0 {
		require.NotNil(t, response.Error, string(payload))
		require.Equal(t, wantCode, response.Error.Code)
	} else {
		require.Nil(t, response.Error, string(payload))
		require.NotEmpty(t, response.Result)
	}

	return response.Result
}

func requireNullFilterTestCount(t *testing.T, manager *FilterManager, want int) {
	t.Helper()

	manager.RLock()
	defer manager.RUnlock()

	require.Len(t, manager.filters, want)
}

func TestNullLogFilter_RPCRejectsInvalidQueries(t *testing.T) {
	for _, transport := range []string{"http", "websocket"} {
		t.Run(transport, func(t *testing.T) {
			d, _ := newNullFilterTestDispatcher(t)

			var conn wsConn
			if transport == "websocket" {
				ws, _ := newMockWsConnWithMsgCh()
				conn = ws
			}

			for _, method := range []string{"eth_newFilter", "eth_getLogs"} {
				for _, params := range []string{"[null]", "[]", "null"} {
					t.Run(method+"/"+params, func(t *testing.T) {
						callNullFilterTestRPC(t, d, conn, method, params, -32602)
						requireNullFilterTestCount(t, d.filterManager, 0)
					})
				}
			}

			if conn != nil {
				for _, params := range []string{
					`["logs",null]`,
					`["logs"]`,
					`["logs",false]`,
					`["logs",[]]`,
				} {
					t.Run("subscription/"+params, func(t *testing.T) {
						callNullFilterTestRPC(
							t, d, conn, "eth_subscribe", params, -32602,
						)
						requireNullFilterTestCount(t, d.filterManager, 0)
					})
				}
			}
		})
	}
}

func TestNullLogFilter_RPCPreservesValidQueries(t *testing.T) {
	for _, transport := range []string{"http", "websocket"} {
		for _, query := range []string{`{}`, `{"topics":[null]}`} {
			t.Run(transport+"/"+query, func(t *testing.T) {
				d, _ := newNullFilterTestDispatcher(t)

				var conn wsConn
				if transport == "websocket" {
					ws, _ := newMockWsConnWithMsgCh()
					conn = ws
				}

				callNullFilterTestRPC(
					t, d, conn, "eth_getLogs", "["+query+"]", 0,
				)

				result := callNullFilterTestRPC(
					t, d, conn, "eth_newFilter", "["+query+"]", 0,
				)
				var id string
				require.NoError(t, json.Unmarshal(result, &id))
				require.NotEmpty(t, id)
				require.True(t, d.filterManager.Uninstall(id))

				if conn != nil {
					result = callNullFilterTestRPC(
						t, d, conn, "eth_subscribe", `["logs",`+query+`]`, 0,
					)
					require.NoError(t, json.Unmarshal(result, &id))
					require.NotEmpty(t, id)
					require.True(t, d.filterManager.Uninstall(id))
				}

				requireNullFilterTestCount(t, d.filterManager, 0)
			})
		}
	}
}

func TestNullLogFilter_DirectBoundaries(t *testing.T) {
	manager := NewFilterManager(hclog.NewNullLogger(), newMockStore(), 256)
	t.Cleanup(manager.Close)

	require.Empty(t, manager.NewLogFilter(nil, nil))

	conn, _ := newMockWsConnWithMsgCh()
	require.Empty(t, manager.NewLogFilter(nil, conn))
	requireNullFilterTestCount(t, manager, 0)

	logs, err := manager.GetLogsForQuery(nil)
	require.Nil(t, logs)
	var paramsErr *invalidParamsError
	require.ErrorAs(t, err, &paramsErr)
	require.Equal(t, -32602, paramsErr.ErrorCode())

	query, err := decodeLogQueryFromInterface(nil)
	require.Nil(t, query)
	require.ErrorAs(t, err, &paramsErr)

	for _, raw := range []string{`{}`, `{"topics":[null]}`} {
		var input any
		require.NoError(t, json.Unmarshal([]byte(raw), &input))

		query, err := decodeLogQueryFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, query)
		require.True(t, query.Match(&types.Log{
			Topics: []types.Hash{types.StringToHash("0x02")},
		}))
	}
}

func TestNullLogFilter_BackgroundDeliveryAfterRejectedRequests(t *testing.T) {
	for _, query := range []string{`{}`, `{"topics":[null]}`} {
		t.Run(query, func(t *testing.T) {
			/* START ALPHA CHANGE 3575 -- supply a transaction matching the emitted receipt in the null-filter fixture -- */
			header := &types.Header{
				Number: 1,
				Hash:   types.StringToHash("0x11"),
			}
			store := newLiveReceiptCountStore(&types.Block{
				Header: header,
				Transactions: []*types.Transaction{
					{Hash: types.StringToHash("0x22")},
				},
			})
			/* FINISH ALPHA CHANGE 3575 -- supply a transaction matching the emitted receipt in the null-filter fixture -- */

			d := newTestDispatcher(t, hclog.NewNullLogger(), store, &dispatcherParams{
				jsonRPCBatchLengthLimit: 20,
				blockRangeLimit:         256,
			})
			t.Cleanup(d.Close)

			conn, messages := newMockWsConnWithMsgCh()

			callNullFilterTestRPC(
				t, d, nil, "eth_newFilter", "[null]", -32602,
			)
			callNullFilterTestRPC(
				t, d, nil, "eth_getLogs", "[null]", -32602,
			)
			callNullFilterTestRPC(
				t, d, conn, "eth_subscribe", `["logs",null]`, -32602,
			)
			requireNullFilterTestCount(t, d.filterManager, 0)

			result := callNullFilterTestRPC(
				t, d, nil, "eth_newFilter", "["+query+"]", 0,
			)
			var pollingID string
			require.NoError(t, json.Unmarshal(result, &pollingID))

			result = callNullFilterTestRPC(
				t, d, conn, "eth_subscribe", `["logs",`+query+`]`, 0,
			)
			var subscriptionID string
			require.NoError(t, json.Unmarshal(result, &subscriptionID))
			requireNullFilterTestCount(t, d.filterManager, 2)

			address := types.StringToAddress("0x1234")
			store.emitEvent(&mockEvent{
				NewChain: []*mockHeader{{
					header: header,
					receipts: []*types.Receipt{{
						TxHash: types.StringToHash("0x22"),
						Logs: []*types.Log{{
							Address: address,
							Topics:  []types.Hash{types.StringToHash("0x33")},
						}},
					}},
				}},
			})

			select {
			case payload := <-messages:
				var notification struct {
					Method string `json:"method"`
					Params struct {
						Subscription string `json:"subscription"`
						Result       struct {
							Address string `json:"address"`
						} `json:"result"`
					} `json:"params"`
				}
				require.NoError(t, json.Unmarshal(payload, &notification))
				require.Equal(t, "eth_subscription", notification.Method)
				require.Equal(t, subscriptionID, notification.Params.Subscription)
				require.Equal(t, address.String(), notification.Params.Result.Address)
			case <-time.After(3 * time.Second):
				t.Fatal("background log delivery did not complete")
			}

			updates, err := d.filterManager.GetFilterChanges(pollingID)
			require.NoError(t, err)
			logs, ok := updates.([]*Log)
			require.True(t, ok)
			require.Len(t, logs, 1)
		})
	}
}

/* FINISH ALPHA CHANGE 2514 -- test null filter rejection and subsequent background log delivery -- */
