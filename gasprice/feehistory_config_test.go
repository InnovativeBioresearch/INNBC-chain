/* START ALPHA CHANGE 2473 -- test configurable fee-history limits and rejection ordering -- */
package gasprice

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFeeHistoryConfig_DefaultsAndValidation(t *testing.T) {
	for _, input := range []*Config{nil, {}, DefaultGasHelperConfig} {
		resolved, err := ResolveFeeHistoryConfig(input)
		require.NoError(t, err)
		require.Equal(t, Config{
			FeeHistoryBlockLimit:      256,
			FeeHistoryPercentileLimit: 10,
			FeeHistoryResultLimit:     2560,
		}, resolved)
	}

	input := &Config{
		FeeHistoryBlockLimit:      512,
		FeeHistoryPercentileLimit: 100,
		FeeHistoryResultLimit:     51200,
	}
	before := *input
	resolved, err := ResolveFeeHistoryConfig(input)
	require.NoError(t, err)
	require.Equal(t, before, resolved)
	require.Equal(t, before, *input)

	for _, invalid := range []*Config{
		{FeeHistoryPercentileLimit: 101},
		{FeeHistoryResultLimit: math.MaxUint64},
	} {
		_, err := ResolveFeeHistoryConfig(invalid)
		require.Error(t, err)

		helper, err := NewGasHelper(invalid, nil)
		require.Error(t, err)
		require.Nil(t, helper)
	}
}

func TestFeeHistoryConfig_RequestBudgets(t *testing.T) {
	cases := []struct {
		name    string
		config  Config
		head    uint64
		count   uint64
		width   int
		want    int
		wantErr error
	}{
		{
			name:  "defaults serve 256 by 10",
			head:  100000,
			count: math.MaxUint64,
			width: 10,
			want:  256,
		},
		{
			name:    "default rejects eleven samples before head lookup",
			head:    100000,
			count:   1,
			width:   11,
			wantErr: ErrTooManyRewardPercentiles,
		},
		{
			name: "explicit 512 by 100",
			config: Config{
				FeeHistoryBlockLimit:      512,
				FeeHistoryPercentileLimit: 100,
				FeeHistoryResultLimit:     51200,
			},
			head:  100000,
			count: 512,
			width: 100,
			want:  512,
		},
		{
			name: "512 by 50 fits 25600",
			config: Config{
				FeeHistoryBlockLimit:      512,
				FeeHistoryPercentileLimit: 100,
				FeeHistoryResultLimit:     25600,
			},
			head:  100000,
			count: 512,
			width: 50,
			want:  512,
		},
		{
			name: "512 by 100 exceeds 25600",
			config: Config{
				FeeHistoryBlockLimit:      512,
				FeeHistoryPercentileLimit: 100,
				FeeHistoryResultLimit:     25600,
			},
			head:    100000,
			count:   512,
			width:   100,
			wantErr: ErrFeeHistoryResultTooLarge,
		},
		{
			name: "raising percentile limit does not raise result budget",
			config: Config{
				FeeHistoryPercentileLimit: 100,
			},
			head:    100000,
			count:   256,
			width:   100,
			wantErr: ErrFeeHistoryResultTooLarge,
		},
		{
			name: "available history clamps before budget check",
			config: Config{
				FeeHistoryBlockLimit:      512,
				FeeHistoryPercentileLimit: 100,
			},
			head:  4,
			count: math.MaxUint64,
			width: 100,
			want:  5,
		},
		{
			name: "header-only request still consumes result units",
			config: Config{
				FeeHistoryBlockLimit:  512,
				FeeHistoryResultLimit: 511,
			},
			head:    100000,
			count:   512,
			wantErr: ErrFeeHistoryResultTooLarge,
		},
		{
			name: "extreme request rejects without multiplication overflow",
			config: Config{
				FeeHistoryBlockLimit:      math.MaxUint64,
				FeeHistoryPercentileLimit: 100,
			},
			head:    math.MaxUint64,
			count:   math.MaxUint64,
			width:   100,
			wantErr: ErrFeeHistoryResultTooLarge,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend := &feeHistoryLimitBackend{head: tc.head}
			helper, err := NewGasHelper(&tc.config, backend)
			require.NoError(t, err)

			key := cacheKey{number: 0, percentiles: "unrelated"}
			cached := &processedFees{reward: []*big.Int{big.NewInt(7)}}
			helper.historyCache.Add(key, cached)

			result, err := helper.FeeHistory(
				tc.count, tc.head, make([]float64, tc.width),
			)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Zero(t, backend.blockCalls)
				require.Zero(t, backend.receiptCalls)
				require.Zero(t, backend.baseFeeCalls)
				require.Equal(t, 1, helper.historyCache.Len())
				value, ok := helper.historyCache.Peek(key)
				require.True(t, ok)
				require.Same(t, cached, value)

				if tc.wantErr == ErrTooManyRewardPercentiles {
					require.Zero(t, backend.headerCalls)
				} else {
					require.Equal(t, 1, backend.headerCalls)
				}

				return
			}

			require.NoError(t, err)
			require.Len(t, result.GasUsedRatio, tc.want)
			require.Len(t, result.BaseFeePerGas, tc.want+1)
			require.Equal(t, tc.want, backend.blockCalls)
			for _, row := range result.Reward {
				require.Len(t, row, tc.width)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2473 -- test configurable fee-history limits and rejection ordering -- */
