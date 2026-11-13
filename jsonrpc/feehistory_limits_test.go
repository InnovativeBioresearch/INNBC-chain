/* START ALPHA CHANGE 2446 -- test fee-history limits through RPC dispatch -- */
package jsonrpc

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/gasprice"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type feeHistoryLimitRPCStore struct {
	ethStore

	helper      *gasprice.GasHelper
	injectedErr error

	headerCalls  int
	feeCalls     int
	blockCalls   int
	fullCalls    int
	receiptCalls int

	firstBlock uint64
	lastBlock  uint64
}

func (s *feeHistoryLimitRPCStore) Header() *types.Header {
	s.headerCalls++

	return &types.Header{Number: 100000}
}

func (s *feeHistoryLimitRPCStore) GetBlockByNumber(
	number uint64,
	full bool,
) (*types.Block, bool) {
	if s.blockCalls == 0 {
		s.firstBlock = number
	}

	s.blockCalls++
	s.lastBlock = number

	if full {
		s.fullCalls++
	}

	return &types.Block{
		Header: &types.Header{
			Number:   number,
			BaseFee:  7,
			GasLimit: 1000000,
		},
	}, number <= 100000
}

func (s *feeHistoryLimitRPCStore) GetReceiptsByHash(
	_ types.Hash,
) ([]*types.Receipt, error) {
	s.receiptCalls++

	return nil, nil
}

func (s *feeHistoryLimitRPCStore) CalculateBaseFee(
	parent *types.Header,
) (uint64, error) {
	return parent.BaseFee, nil
}

func (s *feeHistoryLimitRPCStore) FeeHistory(
	count uint64,
	newest uint64,
	percentiles []float64,
) (*gasprice.FeeHistoryReturn, error) {
	s.feeCalls++

	if s.injectedErr != nil {
		return nil, s.injectedErr
	}

	return s.helper.FeeHistory(count, newest, percentiles)
}

func TestFeeHistoryLimits_RPCRejectsPercentilesBeforeBlockResolution(t *testing.T) {
	/* START ALPHA CHANGE 2472 -- reject arrays above the default or absolute percentile limit -- */
	for _, count := range []int{11, gasprice.MaxFeeHistoryPercentiles + 1, 100000} {
		/* FINISH ALPHA CHANGE 2472 -- reject arrays above the default or absolute percentile limit -- */
		t.Run(fmt.Sprintf("%d percentiles", count), func(t *testing.T) {
			store := &feeHistoryLimitRPCStore{}
			endpoint := &Eth{store: store}

			result, err := endpoint.FeeHistory(
				argUint64(1),
				LatestBlockNumber,
				make([]float64, count),
			)

			require.Nil(t, result)

			var limitErr *rpcLimitError
			require.ErrorAs(t, err, &limitErr)
			require.Equal(t, -32005, limitErr.ErrorCode())

			require.Zero(t, store.headerCalls)
			require.Zero(t, store.feeCalls)
			require.Zero(t, store.blockCalls)
			require.Zero(t, store.receiptCalls)
		})
	}
}

func TestFeeHistoryLimits_RPCMapsBothCoreErrors(t *testing.T) {
	for _, cause := range []error{
		gasprice.ErrTooManyRewardPercentiles,
		gasprice.ErrFeeHistoryResultTooLarge,
	} {
		t.Run(cause.Error(), func(t *testing.T) {
			wrapped := fmt.Errorf("wrapped core error: %w", cause)
			store := &feeHistoryLimitRPCStore{injectedErr: wrapped}
			endpoint := &Eth{store: store}

			result, err := endpoint.FeeHistory(
				argUint64(1),
				BlockNumber(1),
				nil,
			)

			require.Nil(t, result)

			var limitErr *rpcLimitError
			require.ErrorAs(t, err, &limitErr)
			require.Equal(t, -32005, limitErr.ErrorCode())
			require.Equal(t, wrapped.Error(), limitErr.Error())

			require.Equal(t, 1, store.feeCalls)
			require.Zero(t, store.blockCalls)
		})
	}
}

func TestFeeHistoryLimits_Dispatcher(t *testing.T) {
	cases := []struct {
		name       string
		blockCount string
		width      int
		wantError  bool
	}{
		{
			name:       "large percentile array inside body limit is rejected",
			blockCount: "0x100",
			width:      100000,
			wantError:  true,
		},
		{
			name:       "reward request above 256 is clamped",
			blockCount: "0x101",
			width:      100,
		},
		{
			name:       "25600 header request is clamped",
			blockCount: "0x6400",
			width:      0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &feeHistoryLimitRPCStore{}
			/* START ALPHA CHANGE 2472 -- configure the existing 100-sample dispatcher fixture -- */
			helper, err := gasprice.NewGasHelper(
				&gasprice.Config{
					FeeHistoryBlockLimit:      0,
					FeeHistoryPercentileLimit: 100,
					FeeHistoryResultLimit:     25600,
				},
				store,
			)
			/* FINISH ALPHA CHANGE 2472 -- configure the existing 100-sample dispatcher fixture -- */
			require.NoError(t, err)
			store.helper = helper

			dispatcher := &Dispatcher{
				logger: hclog.NewNullLogger(),
				params: &dispatcherParams{},
			}
			/* START ALPHA CHANGE 2472 -- align fixture RPC and helper percentile limits -- */
			require.NoError(t, dispatcher.registerService(
				"eth",
				&Eth{
					store:                     store,
					feeHistoryPercentileLimit: 100,
				},
			))
			/* FINISH ALPHA CHANGE 2472 -- align fixture RPC and helper percentile limits -- */

			request, err := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "eth_feeHistory",
				"params": []any{
					tc.blockCount,
					"latest",
					make([]float64, tc.width),
				},
			})
			require.NoError(t, err)
			require.Less(
				t,
				int64(len(request)),
				DefaultAdmissionConfig().MaxBodyBytes,
			)

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

			if tc.wantError {
				require.NotNil(t, envelope.Error)
				require.Equal(t, -32005, envelope.Error.Code)
				require.Contains(
					t,
					envelope.Error.Message,
					gasprice.ErrTooManyRewardPercentiles.Error(),
				)
				require.True(t,
					len(envelope.Result) == 0 ||
						string(envelope.Result) == "null",
				)

				require.Zero(t, store.headerCalls)
				require.Zero(t, store.feeCalls)
				require.Zero(t, store.blockCalls)
				require.Zero(t, store.receiptCalls)

				return
			}

			require.Nil(t, envelope.Error)

			var result struct {
				OldestBlock   string     `json:"oldestBlock"`
				BaseFeePerGas []string   `json:"baseFeePerGas"`
				GasUsedRatio  []float64  `json:"gasUsedRatio"`
				Reward        [][]string `json:"reward"`
			}
			require.NoError(t, json.Unmarshal(envelope.Result, &result))

			require.Equal(t, fmt.Sprintf("0x%x", 100000-255), result.OldestBlock)
			require.Len(t, result.BaseFeePerGas, 257)
			require.Len(t, result.GasUsedRatio, 256)
			require.Len(t, result.Reward, 256)

			for _, row := range result.Reward {
				require.Len(t, row, tc.width)
				for _, value := range row {
					require.Equal(t, "0x0", value)
				}
			}

			require.Equal(t, 1, store.feeCalls)
			require.Equal(t, 2, store.headerCalls)
			require.Equal(t, 256, store.blockCalls)
			require.Equal(t, uint64(100000-255), store.firstBlock)
			require.Equal(t, uint64(100000), store.lastBlock)
			require.Zero(t, store.receiptCalls)

			if tc.width == 0 {
				require.Zero(t, store.fullCalls)
			} else {
				require.Equal(t, 256, store.fullCalls)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2446 -- test fee-history limits through RPC dispatch -- */
