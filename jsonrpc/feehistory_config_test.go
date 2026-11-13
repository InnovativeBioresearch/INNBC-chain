/* START ALPHA CHANGE 2474 -- test configured percentile and result limits through dispatch -- */
package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/gasprice"
	"github.com/stretchr/testify/require"
)

func TestFeeHistoryConfig_DispatcherBudgets(t *testing.T) {
	cases := []struct {
		name        string
		percentiles uint64
		budget      uint64
		count       string
		width       int
		wantRows    int
		wantError   string
		early       bool
	}{
		{
			name:      "default rejects eleven before backend",
			count:     "0x1",
			width:     11,
			wantError: gasprice.ErrTooManyRewardPercentiles.Error(),
			early:     true,
		},
		{
			name:        "raised settings permit 512 by 100",
			percentiles: 100,
			budget:      51200,
			count:       "0x200",
			width:       100,
			wantRows:    512,
		},
		{
			name:        "result budget maps to RPC limit error",
			percentiles: 100,
			budget:      25600,
			count:       "0x200",
			width:       100,
			wantError:   gasprice.ErrFeeHistoryResultTooLarge.Error(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &feeHistoryLimitRPCStore{}
			helper, err := gasprice.NewGasHelper(&gasprice.Config{
				FeeHistoryBlockLimit:      512,
				FeeHistoryPercentileLimit: tc.percentiles,
				FeeHistoryResultLimit:     tc.budget,
			}, store)
			require.NoError(t, err)
			store.helper = helper

			dispatcher, err := newDispatcher(
				hclog.NewNullLogger(),
				nil,
				&dispatcherParams{
					feeHistoryPercentileLimit: tc.percentiles,
				},
			)
			require.NoError(t, err)
			defer dispatcher.Close()

			/*
				Keep the endpoint constructed by the real dispatcher.
				Only replace its backend with the focused fixture.
			*/
			dispatcher.endpoints.Eth.store = store

			request, err := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "eth_feeHistory",
				"params": []any{
					tc.count, "latest", make([]float64, tc.width),
				},
			})
			require.NoError(t, err)

			response, err := dispatcher.Handle(request)
			require.NoError(t, err)

			var envelope struct {
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(response, &envelope))

			if tc.wantError != "" {
				require.NotNil(t, envelope.Error)
				require.Equal(t, -32005, envelope.Error.Code)
				require.Contains(t, envelope.Error.Message, tc.wantError)
				require.Zero(t, store.blockCalls)
				require.Zero(t, store.receiptCalls)

				if tc.early {
					require.Zero(t, store.headerCalls)
					require.Zero(t, store.feeCalls)
				} else {
					require.Equal(t, 1, store.feeCalls)
				}

				return
			}

			require.Nil(t, envelope.Error)
			var result struct {
				Reward [][]string `json:"reward"`
			}
			require.NoError(t, json.Unmarshal(envelope.Result, &result))
			require.Len(t, result.Reward, tc.wantRows)
			for _, row := range result.Reward {
				require.Len(t, row, tc.width)
			}
			require.Equal(t, tc.wantRows, store.blockCalls)
		})
	}
}

func TestFeeHistoryConfig_DispatcherRejectsInvalidLimit(t *testing.T) {
	dispatcher, err := newDispatcher(
		hclog.NewNullLogger(),
		nil,
		&dispatcherParams{feeHistoryPercentileLimit: 101},
	)
	require.Error(t, err)
	require.Nil(t, dispatcher)
}

/* FINISH ALPHA CHANGE 2474 -- test configured percentile and result limits through dispatch -- */
