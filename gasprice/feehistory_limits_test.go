/* START ALPHA CHANGE 2445 -- test fixed fee-history limits and arithmetic boundaries -- */
package gasprice

import (
	"math"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type feeHistoryLimitBackend struct {
	head uint64

	headerCalls  int
	blockCalls   int
	fullCalls    int
	receiptCalls int
	baseFeeCalls int

	firstBlock uint64
	lastBlock  uint64
}

func (b *feeHistoryLimitBackend) Header() *types.Header {
	b.headerCalls++

	return &types.Header{Number: b.head}
}

func (b *feeHistoryLimitBackend) GetBlockByNumber(
	number uint64,
	full bool,
) (*types.Block, bool) {
	if b.blockCalls == 0 {
		b.firstBlock = number
	}

	b.blockCalls++
	b.lastBlock = number

	if full {
		b.fullCalls++
	}

	return &types.Block{
		Header: &types.Header{
			Number:   number,
			BaseFee:  7,
			GasLimit: 1000000,
		},
	}, number <= b.head
}

func (b *feeHistoryLimitBackend) GetReceiptsByHash(
	_ types.Hash,
) ([]*types.Receipt, error) {
	b.receiptCalls++

	return nil, nil
}

func (b *feeHistoryLimitBackend) CalculateBaseFee(
	parent *types.Header,
) (uint64, error) {
	b.baseFeeCalls++

	return parent.BaseFee, nil
}

func TestFeeHistoryLimits_EarlyValidation(t *testing.T) {
	cases := []struct {
		name        string
		count       uint64
		percentiles []float64
		wantErr     error
	}{
		{
			name:    "zero blocks",
			count:   0,
			wantErr: ErrBlockCount,
		},
		{
			name:        "one percentile above limit",
			count:       1,
			percentiles: make([]float64, MaxFeeHistoryPercentiles+1),
			wantErr:     ErrTooManyRewardPercentiles,
		},
		{
			name:        "large repeated percentile array",
			count:       256,
			percentiles: make([]float64, 100000),
			wantErr:     ErrTooManyRewardPercentiles,
		},
		{
			name:        "NaN",
			count:       1,
			percentiles: []float64{math.NaN()},
			wantErr:     ErrInvalidPercentile,
		},
		{
			name:        "positive infinity",
			count:       1,
			percentiles: []float64{math.Inf(1)},
			wantErr:     ErrInvalidPercentile,
		},
		{
			name:        "negative infinity",
			count:       1,
			percentiles: []float64{math.Inf(-1)},
			wantErr:     ErrInvalidPercentile,
		},
		{
			name:        "negative percentile",
			count:       1,
			percentiles: []float64{-1},
			wantErr:     ErrInvalidPercentile,
		},
		{
			name:        "percentile above 100",
			count:       1,
			percentiles: []float64{101},
			wantErr:     ErrInvalidPercentile,
		},
		{
			name:        "descending percentiles",
			count:       1,
			percentiles: []float64{50, 49},
			wantErr:     ErrInvalidPercentile,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend := &feeHistoryLimitBackend{head: 100000}
			helper, err := NewGasHelper(&Config{}, backend)
			require.NoError(t, err)

			key := cacheKey{number: 0, percentiles: "existing"}
			cached := &processedFees{reward: []*big.Int{big.NewInt(7)}}
			helper.historyCache.Add(key, cached)

			_, err = helper.FeeHistory(tc.count, backend.head, tc.percentiles)
			require.ErrorIs(t, err, tc.wantErr)

			require.Zero(t, backend.headerCalls)
			require.Zero(t, backend.blockCalls)
			require.Zero(t, backend.fullCalls)
			require.Zero(t, backend.receiptCalls)
			require.Zero(t, backend.baseFeeCalls)

			require.Equal(t, 1, helper.historyCache.Len())
			value, ok := helper.historyCache.Peek(key)
			require.True(t, ok)
			require.Same(t, cached, value)
			require.Zero(t, cached.reward[0].Cmp(big.NewInt(7)))
		})
	}
}

func TestFeeHistoryLimits_ServedRanges(t *testing.T) {
	cases := []struct {
		name   string
		limit  uint64
		head   uint64
		newest uint64
		count  uint64
		width  int
		want   uint64
	}{
		{
			/* START ALPHA CHANGE 2471 -- describe the explicit sampling budget -- */
			name: "maximum rewards at configured boundary",
			/* FINISH ALPHA CHANGE 2471 -- describe the explicit sampling budget -- */
			head:   100000,
			newest: 100000,
			count:  256,
			width:  100,
			want:   256,
		},
		{
			name:   "zero configured cap still clamps reward request",
			head:   100000,
			newest: 100000,
			count:  257,
			width:  100,
			want:   256,
		},
		{
			name:   "zero configured cap still clamps header-only request",
			head:   100000,
			newest: 100000,
			count:  25600,
			want:   256,
		},
		/* START ALPHA CHANGE 2471 -- test an explicitly raised block limit -- */
		{
			name:   "configured 512 blocks with 50 percentiles",
			limit:  512,
			head:   100000,
			newest: 100000,
			count:  math.MaxUint64,
			width:  50,
			want:   512,
		},
		/* FINISH ALPHA CHANGE 2471 -- test an explicitly raised block limit -- */
		{
			name:   "smaller configured cap",
			limit:  32,
			head:   100000,
			newest: 100000,
			count:  math.MaxUint64,
			width:  100,
			want:   32,
		},
		{
			name:   "requested count is smaller",
			limit:  32,
			head:   100000,
			newest: 100000,
			count:  3,
			width:  1,
			want:   3,
		},
		{
			name:   "available history is smaller",
			head:   4,
			newest: 4,
			count:  math.MaxUint64,
			width:  100,
			want:   5,
		},
		{
			name:   "newest block clamps to head",
			head:   4,
			newest: math.MaxUint64,
			count:  math.MaxUint64,
			width:  100,
			want:   5,
		},
		{
			name:   "requested historical endpoint limits availability",
			head:   100000,
			newest: 2,
			count:  math.MaxUint64,
			want:   3,
		},
		{
			name:  "genesis is included",
			count: math.MaxUint64,
			width: 100,
			want:  1,
		},
		{
			name:   "maximum height does not wrap iteration",
			head:   math.MaxUint64,
			newest: math.MaxUint64,
			count:  math.MaxUint64,
			width:  1,
			want:   256,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend := &feeHistoryLimitBackend{head: tc.head}
			/* START ALPHA CHANGE 2471 -- explicitly enable large sampling in range tests -- */
			helper, err := NewGasHelper(
				&Config{
					FeeHistoryBlockLimit:      tc.limit,
					FeeHistoryPercentileLimit: 100,
					FeeHistoryResultLimit:     25600,
				},
				backend,
			)
			/* FINISH ALPHA CHANGE 2471 -- explicitly enable large sampling in range tests -- */
			require.NoError(t, err)

			result, err := helper.FeeHistory(
				tc.count,
				tc.newest,
				make([]float64, tc.width),
			)
			require.NoError(t, err)

			newest := min(tc.newest, tc.head)
			oldest := newest - (tc.want - 1)

			require.Equal(t, oldest, result.OldestBlock)
			require.Len(t, result.BaseFeePerGas, int(tc.want)+1)
			require.Len(t, result.GasUsedRatio, int(tc.want))
			require.Len(t, result.Reward, int(tc.want))

			require.Equal(t, 1, backend.headerCalls)
			require.Equal(t, int(tc.want), backend.blockCalls)
			require.Equal(t, oldest, backend.firstBlock)
			require.Equal(t, newest, backend.lastBlock)
			require.Zero(t, backend.receiptCalls)
			require.Equal(t, 1, backend.baseFeeCalls)

			if tc.width == 0 {
				require.Zero(t, backend.fullCalls)
			} else {
				require.Equal(t, int(tc.want), backend.fullCalls)
			}

			for _, fee := range result.BaseFeePerGas {
				require.Equal(t, uint64(7), fee)
			}

			for _, ratio := range result.GasUsedRatio {
				require.Zero(t, ratio)
			}

			for _, row := range result.Reward {
				require.Len(t, row, tc.width)
				for _, value := range row {
					require.NotNil(t, value)
					require.Zero(t, value.Sign())
				}
			}
		})
	}
}

func TestFeeHistoryLimits_FractionalAndRepeatedPercentiles(t *testing.T) {
	backend := &feeHistoryLimitBackend{head: 1}
	helper, err := NewGasHelper(DefaultGasHelperConfig, backend)
	require.NoError(t, err)

	result, err := helper.FeeHistory(
		1,
		1,
		[]float64{0, 0, 12.5, 12.5, 100},
	)
	require.NoError(t, err)
	require.Len(t, result.Reward, 1)
	require.Len(t, result.Reward[0], 5)

	result.Reward[0][0].SetInt64(99)
	for _, value := range result.Reward[0][1:] {
		require.Zero(t, value.Sign())
	}
}

/* FINISH ALPHA CHANGE 2445 -- test fixed fee-history limits and arithmetic boundaries -- */
