/* START ALPHA CHANGE 2383 -- test system transaction exclusion and full-width reward ownership -- */
package gasprice

import (
	"math"
	"math/big"

	/* START ALPHA CHANGE 2386 -- support concurrent fee-history ownership testing -- */
	"sync"
	/* FINISH ALPHA CHANGE 2386 -- support concurrent fee-history ownership testing -- */
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func issue16FeeHistoryHelper(
	t *testing.T,
	baseFee uint64,
	txs []*types.Transaction,
	gas []uint64,
) (*GasHelper, *backendMock) {
	t.Helper()
	require.Len(t, gas, len(txs))

	backend := createTestBlocks(t, 1)
	block := backend.blocksByNumber[1]
	block.Header.BaseFee = baseFee
	block.Header.GasLimit = 2000
	block.Transactions = txs

	receipts := make([]*types.Receipt, len(txs))
	var total uint64
	for i, used := range gas {
		total += used
		receipts[i] = &types.Receipt{
			GasUsed:           used,
			CumulativeGasUsed: total,
		}
	}
	block.Header.GasUsed = total
	backend.receiptsByHash = map[types.Hash][]*types.Receipt{
		block.Hash(): receipts,
	}

	helper, err := NewGasHelper(DefaultGasHelperConfig, backend)
	require.NoError(t, err)

	return helper, backend
}

func TestIssue16_FeeHistoryExcludesSystemTransactions(t *testing.T) {
	for _, systemGas := range []uint64{0, 900} {
		t.Run(new(big.Int).SetUint64(systemGas).String(), func(t *testing.T) {
			helper, backend := issue16FeeHistoryHelper(
				t,
				100,
				[]*types.Transaction{
					{Type: types.StateTx, GasPrice: new(big.Int)},
					{Type: types.LegacyTx, GasPrice: big.NewInt(102)},
					{
						Type:      types.DynamicFeeTx,
						GasTipCap: big.NewInt(10),
						GasFeeCap: big.NewInt(200),
					},
				},
				[]uint64{systemGas, 10, 90},
			)

			history, err := helper.FeeHistory(1, 1, []float64{0, 10, 50, 100})
			require.NoError(t, err)
			require.Equal(t, [][]*big.Int{{
				big.NewInt(2),
				big.NewInt(2),
				big.NewInt(10),
				big.NewInt(10),
			}}, history.Reward)
			require.Equal(t, []uint64{100, 100}, history.BaseFeePerGas)
			require.Equal(t, []float64{
				float64(systemGas+100) / 2000,
			}, history.GasUsedRatio)
			require.Equal(t, 1, backend.getBlockByNumberCalls)
			require.Equal(t, 1, backend.getReceiptsCalls)
		})
	}
}

func TestIssue16_FeeHistoryZeroRewardsWithoutFallbackSearch(t *testing.T) {
	cases := []struct {
		name    string
		baseFee uint64
		txs     []*types.Transaction
		gas     []uint64
	}{
		{
			name:    "empty block",
			baseFee: 100,
		},
		{
			name:    "system transaction with zero gas",
			baseFee: 100,
			txs: []*types.Transaction{
				{Type: types.StateTx, GasPrice: new(big.Int)},
			},
			gas: []uint64{0},
		},
		{
			name:    "system transaction with nonzero gas",
			baseFee: 100,
			txs: []*types.Transaction{
				{Type: types.StateTx, GasPrice: new(big.Int)},
			},
			gas: []uint64{500},
		},
		{
			name: "zero base fee and zero price",
			txs: []*types.Transaction{
				{Type: types.LegacyTx, GasPrice: new(big.Int)},
			},
			gas: []uint64{100},
		},
		{
			name:    "ordinary transaction pays only base fee",
			baseFee: 100,
			txs: []*types.Transaction{
				{Type: types.LegacyTx, GasPrice: big.NewInt(100)},
			},
			gas: []uint64{100},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			helper, backend := issue16FeeHistoryHelper(
				t, testCase.baseFee, testCase.txs, testCase.gas,
			)

			history, err := helper.FeeHistory(1, 1, []float64{0, 50, 100})
			require.NoError(t, err)
			require.Equal(t, [][]*big.Int{{
				new(big.Int), new(big.Int), new(big.Int),
			}}, history.Reward)
			require.Equal(t, uint64(1), history.OldestBlock)
			require.Equal(t, 1, backend.getBlockByNumberCalls)

			if len(testCase.txs) == 0 {
				require.Zero(t, backend.getReceiptsCalls)
			} else {
				require.Equal(t, 1, backend.getReceiptsCalls)
			}
		})
	}
}

func TestIssue16_FeeHistoryLargeRewardsAndCacheIsolation(t *testing.T) {
	two64 := new(big.Int).Lsh(big.NewInt(1), 64)
	two128Plus7 := new(big.Int).Add(
		new(big.Int).Lsh(big.NewInt(1), 128),
		big.NewInt(7),
	)

	cases := []struct {
		name string
		tx   *types.Transaction
		want *big.Int
	}{
		{
			name: "legacy above uint64",
			tx: &types.Transaction{
				Type: types.LegacyTx,
				GasPrice: new(big.Int).Add(
					new(big.Int).Set(two64), big.NewInt(100),
				),
			},
			want: two64,
		},
		{
			name: "dynamic tip cap above uint64",
			tx: &types.Transaction{
				Type:      types.DynamicFeeTx,
				GasTipCap: new(big.Int).Set(two128Plus7),
				GasFeeCap: new(big.Int).Add(
					new(big.Int).Set(two128Plus7), big.NewInt(200),
				),
			},
			want: two128Plus7,
		},
		/* START ALPHA CHANGE 2385 -- use valid dynamic fee caps for the fee-limited reward test -- */
		{
			name: "dynamic effective tip limited by fee cap",
			tx: &types.Transaction{
				Type: types.DynamicFeeTx,
				GasTipCap: new(big.Int).Add(
					new(big.Int).Set(two64), big.NewInt(50),
				),
				GasFeeCap: new(big.Int).Add(
					new(big.Int).Set(two64), big.NewInt(100),
				),
			},
			want: two64,
		},
		/* FINISH ALPHA CHANGE 2385 -- use valid dynamic fee caps for the fee-limited reward test -- */
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			helper, backend := issue16FeeHistoryHelper(
				t, 100, []*types.Transaction{testCase.tx}, []uint64{100},
			)
			percentiles := []float64{0, 50, 100}

			first, err := helper.FeeHistory(1, 1, percentiles)
			require.NoError(t, err)
			for _, value := range first.Reward[0] {
				require.Zero(t, value.Cmp(testCase.want))
			}

			first.Reward[0][0].SetInt64(123)
			require.Zero(t, first.Reward[0][1].Cmp(testCase.want))

			second, err := helper.FeeHistory(1, 1, percentiles)
			require.NoError(t, err)
			for _, value := range second.Reward[0] {
				require.Zero(t, value.Cmp(testCase.want))
			}

			second.Reward[0][1].SetInt64(456)

			third, err := helper.FeeHistory(1, 1, percentiles)
			require.NoError(t, err)
			for _, value := range third.Reward[0] {
				require.Zero(t, value.Cmp(testCase.want))
			}

			require.Equal(t, 1, backend.getReceiptsCalls)
			require.Equal(t, 3, backend.getBlockByNumberCalls)
		})
	}
}

func TestIssue16_FeeHistoryRejectsInvalidRewardData(t *testing.T) {
	t.Run("negative ordinary tip", func(t *testing.T) {
		helper, _ := issue16FeeHistoryHelper(
			t,
			100,
			[]*types.Transaction{
				{Type: types.LegacyTx, GasPrice: big.NewInt(99)},
			},
			[]uint64{100},
		)

		_, err := helper.FeeHistory(1, 1, []float64{50})
		require.ErrorContains(t, err, "negative ordinary transaction tip")
	})

	t.Run("eligible gas overflow", func(t *testing.T) {
		helper, backend := issue16FeeHistoryHelper(
			t,
			100,
			[]*types.Transaction{
				{Type: types.LegacyTx, GasPrice: big.NewInt(102)},
				{Type: types.LegacyTx, GasPrice: big.NewInt(110)},
			},
			[]uint64{1, 1},
		)
		block := backend.blocksByNumber[1]
		backend.receiptsByHash[block.Hash()][0].GasUsed = math.MaxUint64

		_, err := helper.FeeHistory(1, 1, []float64{50})
		require.ErrorContains(t, err, "reward gas total overflows")
	})

	t.Run("missing system receipt is still an error", func(t *testing.T) {
		helper, backend := issue16FeeHistoryHelper(
			t,
			100,
			[]*types.Transaction{
				{Type: types.StateTx, GasPrice: new(big.Int)},
			},
			[]uint64{0},
		)
		block := backend.blocksByNumber[1]
		backend.receiptsByHash[block.Hash()][0] = nil

		_, err := helper.FeeHistory(1, 1, []float64{50})
		require.ErrorContains(t, err, "missing transaction or receipt")
	})
}

func TestIssue16_FeeHistoryWithoutRewardsDoesNotReadReceipts(t *testing.T) {
	helper, backend := issue16FeeHistoryHelper(
		t,
		100,
		[]*types.Transaction{
			{Type: types.StateTx, GasPrice: new(big.Int)},
		},
		[]uint64{100},
	)

	history, err := helper.FeeHistory(1, 1, nil)
	require.NoError(t, err)
	require.Equal(t, [][]*big.Int{nil}, history.Reward)
	require.Equal(t, []uint64{100, 100}, history.BaseFeePerGas)
	require.Equal(t, 1, backend.getBlockByNumberCalls)
	require.Zero(t, backend.getReceiptsCalls)
}

/* START ALPHA CHANGE 2387 -- verify concurrent fee-history cache consumers own their rewards -- */

type issue16ConcurrentBackend struct {
	mu      sync.Mutex
	backend *backendMock
}

var _ Blockchain = (*issue16ConcurrentBackend)(nil)

func (b *issue16ConcurrentBackend) Header() *types.Header {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.backend.Header()
}

func (b *issue16ConcurrentBackend) GetBlockByNumber(
	number uint64,
	full bool,
) (*types.Block, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.backend.GetBlockByNumber(number, full)
}

func (b *issue16ConcurrentBackend) GetReceiptsByHash(
	hash types.Hash,
) ([]*types.Receipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.backend.GetReceiptsByHash(hash)
}

func (b *issue16ConcurrentBackend) CalculateBaseFee(
	parent *types.Header,
) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.backend.CalculateBaseFee(parent)
}

func TestIssue16_FeeHistoryConcurrentCacheIsolation(t *testing.T) {
	const (
		workers    = 8
		iterations = 16
	)

	want := new(big.Int).Add(
		new(big.Int).Lsh(big.NewInt(1), 128),
		big.NewInt(7),
	)

	_, backend := issue16FeeHistoryHelper(
		t,
		100,
		[]*types.Transaction{
			{
				Type: types.LegacyTx,
				GasPrice: new(big.Int).Add(
					new(big.Int).Set(want), big.NewInt(100),
				),
			},
		},
		[]uint64{100},
	)

	helper, err := NewGasHelper(
		DefaultGasHelperConfig,
		&issue16ConcurrentBackend{backend: backend},
	)
	require.NoError(t, err)

	percentiles := []float64{0, 50, 100}

	assertRewards := func(history *FeeHistoryReturn) {
		t.Helper()

		require.NotNil(t, history)
		require.Len(t, history.Reward, 1)
		require.Len(t, history.Reward[0], len(percentiles))

		for _, value := range history.Reward[0] {
			require.NotNil(t, value)
			require.Zero(t, value.Cmp(want))
		}
	}

	warm, err := helper.FeeHistory(1, 1, percentiles)
	require.NoError(t, err)
	assertRewards(warm)
	require.Equal(t, 1, helper.historyCache.Len())

	/*
		Acquire all caller results before mutation. Each worker writes
		only its own result slot; the main goroutine reads after Wait.
	*/
	histories := make([]*FeeHistoryReturn, workers)
	readErrors := make([]error, workers)
	startReads := make(chan struct{})
	var readers sync.WaitGroup

	for worker := range workers {
		readers.Add(1)

		go func(worker int) {
			defer readers.Done()
			<-startReads

			histories[worker], readErrors[worker] =
				helper.FeeHistory(1, 1, percentiles)
		}(worker)
	}

	close(startReads)
	readers.Wait()

	for worker := range workers {
		require.NoError(t, readErrors[worker])
		assertRewards(histories[worker])
	}

	/*
		Mutate independently owned results while other callers read
		the same cache entry. Distinct markers also detect sharing
		between callers or between percentile entries.
	*/
	startMutations := make(chan struct{})
	var mutators sync.WaitGroup

	for worker := range workers {
		mutators.Add(1)

		go func(worker int) {
			defer mutators.Done()
			<-startMutations

			for range iterations {
				for j, value := range histories[worker].Reward[0] {
					value.SetInt64(int64((worker+1)*100 + j))
				}

				fresh, readErr := helper.FeeHistory(1, 1, percentiles)
				if readErr != nil {
					t.Errorf("worker %d: fee history failed: %v", worker, readErr)
					return
				}

				if fresh == nil ||
					len(fresh.Reward) != 1 ||
					len(fresh.Reward[0]) != len(percentiles) {
					t.Errorf("worker %d: unexpected reward shape", worker)
					return
				}

				for j, value := range fresh.Reward[0] {
					if value == nil || value.Cmp(want) != 0 {
						t.Errorf(
							"worker %d: cached reward %d changed: got %v, want %v",
							worker, j, value, want,
						)
						return
					}
				}
			}
		}(worker)
	}

	close(startMutations)
	mutators.Wait()

	for worker := range workers {
		for j, value := range histories[worker].Reward[0] {
			expected := big.NewInt(int64((worker+1)*100 + j))
			require.Zero(t, value.Cmp(expected))
		}
	}

	assertRewards(warm)

	final, err := helper.FeeHistory(1, 1, percentiles)
	require.NoError(t, err)
	assertRewards(final)

	/*
		Only the warm-up may read receipts. Later calls must use the
		cached rewards, although they still read the newest header.
	*/
	require.Equal(t, 1, backend.getReceiptsCalls)
	require.Equal(t, 1, helper.historyCache.Len())
}

/* FINISH ALPHA CHANGE 2387 -- verify concurrent fee-history cache consumers own their rewards -- */

/* FINISH ALPHA CHANGE 2383 -- test system transaction exclusion and full-width reward ownership -- */
