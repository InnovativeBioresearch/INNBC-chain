package gasprice

import (
	/* START ALPHA CHANGE 1950 -- test actual receipt-weighted fee percentiles -- */
	"math/big"
	/* FINISH ALPHA CHANGE 1950 -- test actual receipt-weighted fee percentiles -- */
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/helper/tests"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func TestGasHelper_FeeHistory(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		Name                  string
		ExpectedOldestBlock   uint64
		ExpectedBaseFeePerGas []uint64
		ExpectedGasUsedRatio  []float64
		ExpectedRewards       [][]uint64
		BlockRange            uint64
		NewestBlock           uint64
		RewardPercentiles     []float64
		Error                 bool
		GetBackend            func() Blockchain
	}{
		{
			Name:              "Block does not exist",
			Error:             true,
			BlockRange:        10,
			NewestBlock:       30,
			RewardPercentiles: []float64{15, 20},
			GetBackend: func() Blockchain {
				header := &types.Header{
					Number: 1,
					Hash:   types.StringToHash("some header"),
				}
				backend := new(backendMock)
				backend.On("Header").Return(header)
				backend.On("GetBlockByNumber", mock.Anything, mock.Anything).Return(&types.Block{}, false)

				return backend
			},
		},
		{
			Name:              "blockRange < 1",
			Error:             true,
			BlockRange:        0,
			NewestBlock:       30,
			RewardPercentiles: []float64{10, 15},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 30)

				return backend
			},
		},
		{
			Name:              "Invalid rewardPercentile",
			Error:             true,
			BlockRange:        10,
			NewestBlock:       30,
			RewardPercentiles: []float64{101, 0},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 50)
				createTestTxs(t, backend, 1, 200)

				return backend
			},
		},
		{
			/* START ALPHA CHANGE 1078 -- verify feeHistory zero gas limit ratio stays finite -- */
			Name:                "zero gas limit returns zero gas used ratio",
			BlockRange:          1,
			NewestBlock:         1,
			RewardPercentiles:   []float64{},
			ExpectedOldestBlock: 1,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0,
			},
			ExpectedRewards: [][]uint64{
				nil,
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 1)
				backend.blocksByNumber[1].Header.GasLimit = 0
				backend.blocksByNumber[1].Header.GasUsed = 1

				return backend
			},
			/* FINISH ALPHA CHANGE 1078 -- verify feeHistory zero gas limit ratio stays finite -- */
		},
		{
			Name:                "rewardPercentile not set",
			BlockRange:          5,
			NewestBlock:         30,
			RewardPercentiles:   []float64{},
			ExpectedOldestBlock: 26,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0, 0, 0, 0, 0,
			},
			ExpectedRewards: [][]uint64{
				nil, nil, nil, nil, nil,
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 30)
				createTestTxs(t, backend, 5, 500)

				return backend
			},
		},
		{
			/* START ALPHA CHANGE 1958 -- verify fee-history ranges include genesis -- */
			Name:                "blockRange includes genesis",
			BlockRange:          20,
			NewestBlock:         10,
			RewardPercentiles:   []float64{},
			ExpectedOldestBlock: 0,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			ExpectedRewards: [][]uint64{
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 30)
				createTestTxs(t, backend, 5, 500)

				return backend
			},
			/* FINISH ALPHA CHANGE 1958 -- verify fee-history ranges include genesis -- */
		},
		{
			Name:                "blockRange == newestBlock",
			BlockRange:          10,
			NewestBlock:         10,
			RewardPercentiles:   []float64{},
			ExpectedOldestBlock: 1,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee, chain.GenesisBaseFee, chain.GenesisBaseFee,
				chain.GenesisBaseFee, chain.GenesisBaseFee, chain.GenesisBaseFee,
				chain.GenesisBaseFee, chain.GenesisBaseFee, chain.GenesisBaseFee,
				chain.GenesisBaseFee, chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			ExpectedRewards: [][]uint64{
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 30)
				createTestTxs(t, backend, 5, 500)

				return backend
			},
		},
		{
			Name:                "rewardPercentile requested",
			BlockRange:          5,
			NewestBlock:         10,
			RewardPercentiles:   []float64{10, 25},
			ExpectedOldestBlock: 6,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0, 0, 0, 0, 0,
			},
			ExpectedRewards: [][]uint64{
				{0x2e90edd000, 0x2e90edd000},
				{0x2e90edd000, 0x2e90edd000},
				{0x2e90edd000, 0x2e90edd000},
				{0x2e90edd000, 0x2e90edd000},
				{0x2e90edd000, 0x2e90edd000},
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 10)

				senderKey, sender := tests.GenerateKeyAndAddr(t)
				/* START ALPHA CHANGE 1951 -- provide persisted gas usage for existing reward fixture -- */
				backend.receiptsByHash = make(map[types.Hash][]*types.Receipt)
				/* FINISH ALPHA CHANGE 1951 -- provide persisted gas usage for existing reward fixture -- */

				for _, b := range backend.blocksByNumber {
					signer := crypto.NewSigner(backend.Config().Forks.At(b.Number()),
						uint64(backend.Config().ChainID))

					b.Transactions = make([]*types.Transaction, 3)
					b.Header.Miner = sender.Bytes()

					/* START ALPHA CHANGE 1954 -- use modern integer range in fee-history fixture -- */
					for i := range 3 {
						/* FINISH ALPHA CHANGE 1954 -- use modern integer range in fee-history fixture -- */
						tx := &types.Transaction{
							From:      sender,
							Value:     ethgo.Ether(1),
							To:        &types.ZeroAddress,
							Type:      types.DynamicFeeTx,
							GasTipCap: ethgo.Gwei(uint64(200)),
							GasFeeCap: ethgo.Gwei(uint64(200 + 200)),
						}

						tx, err := signer.SignTx(tx, senderKey)
						require.NoError(t, err)
						b.Transactions[i] = tx
					}

					/* START ALPHA CHANGE 1951 -- provide persisted gas usage for existing reward fixture -- */
					backend.receiptsByHash[b.Hash()] = []*types.Receipt{
						{GasUsed: 0},
						{GasUsed: 0},
						{GasUsed: 0},
					}
					/* FINISH ALPHA CHANGE 1951 -- provide persisted gas usage for existing reward fixture -- */
				}

				return backend
			},
		},
		{
			Name:                "BaseFeePerGas sanity check",
			BlockRange:          5,
			NewestBlock:         10,
			RewardPercentiles:   []float64{},
			ExpectedOldestBlock: 6,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0, 0, 0, 0, 0,
			},
			ExpectedRewards: [][]uint64{
				nil, nil, nil, nil, nil,
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 10)

				return backend
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			backend := tc.GetBackend()
			gasHelper, err := NewGasHelper(DefaultGasHelperConfig, backend)
			require.NoError(t, err)
			history, err := gasHelper.FeeHistory(tc.BlockRange, tc.NewestBlock, tc.RewardPercentiles)

			if tc.Error {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.ExpectedOldestBlock, history.OldestBlock)
				require.Equal(t, tc.ExpectedBaseFeePerGas, history.BaseFeePerGas)
				require.Equal(t, tc.ExpectedGasUsedRatio, history.GasUsedRatio)
				/* START ALPHA CHANGE 2381 -- compare existing reward literals without narrowing actual results -- */
				expectedRewards := make([][]*big.Int, len(tc.ExpectedRewards))
				for i, row := range tc.ExpectedRewards {
					if row == nil {
						continue
					}

					expectedRewards[i] = make([]*big.Int, len(row))
					for j, value := range row {
						expectedRewards[i][j] = new(big.Int).SetUint64(value)
					}
				}
				require.Equal(t, expectedRewards, history.Reward)
				/* FINISH ALPHA CHANGE 2381 -- compare existing reward literals without narrowing actual results -- */
			}
		})
	}
}

/* START ALPHA CHANGE 1161 -- guard feeHistory against exceeding configured block cap -- */
func TestGasHelper_FeeHistoryCapsBlockCount(t *testing.T) {
	t.Parallel()

	backend := createTestBlocks(t, 30)

	config := *DefaultGasHelperConfig
	config.FeeHistoryBlockLimit = 3

	gasHelper, err := NewGasHelper(&config, backend)
	require.NoError(t, err)

	history, err := gasHelper.FeeHistory(10, 20, []float64{})
	require.NoError(t, err)

	require.Equal(t, uint64(18), history.OldestBlock)
	require.Len(t, history.BaseFeePerGas, 4)
	require.Len(t, history.GasUsedRatio, 3)
	require.Len(t, history.Reward, 3)
}

/* FINISH ALPHA CHANGE 1161 -- guard feeHistory against exceeding configured block cap -- */

var _ Blockchain = (*backendMock)(nil)

func (b *backendMock) GetBlockByNumber(number uint64, full bool) (*types.Block, bool) {
	/* START ALPHA CHANGE 1952 -- count fee-history block reads -- */
	b.getBlockByNumberCalls++
	/* FINISH ALPHA CHANGE 1952 -- count fee-history block reads -- */

	if len(b.blocks) == 0 {
		args := b.Called(number, full)

		return args.Get(0).(*types.Block), args.Get(1).(bool) //nolint:forcetypeassert
	}

	block, exists := b.blocksByNumber[number]

	return block, exists
}

/* START ALPHA CHANGE 1953 -- test receipt weighting and requested-block next base fee -- */
func TestGasHelper_FeeHistoryUsesReceiptGasAndHistoricalNextBaseFee(t *testing.T) {
	t.Parallel()

	backend := createTestBlocks(t, 5)
	block := backend.blocksByNumber[2]
	block.Header.BaseFee = 4
	block.Header.GasLimit = 200
	block.Header.GasUsed = 100
	block.Transactions = []*types.Transaction{
		{
			Type:      types.DynamicFeeTx,
			Gas:       1000,
			GasTipCap: big.NewInt(2),
			GasFeeCap: big.NewInt(100),
		},
		{
			Type:      types.DynamicFeeTx,
			Gas:       1000,
			GasTipCap: big.NewInt(10),
			GasFeeCap: big.NewInt(20),
		},
	}
	backend.receiptsByHash = map[types.Hash][]*types.Receipt{
		block.Hash(): {
			{GasUsed: 10, CumulativeGasUsed: 10},
			{GasUsed: 90, CumulativeGasUsed: 100},
		},
	}
	backend.calculateBaseFee = func(parent *types.Header) (uint64, error) {
		require.Equal(t, uint64(2), parent.Number)

		return 14, nil
	}

	helper, err := NewGasHelper(DefaultGasHelperConfig, backend)
	require.NoError(t, err)

	history, err := helper.FeeHistory(1, 2, []float64{50})
	require.NoError(t, err)
	require.Equal(t, uint64(2), history.OldestBlock)
	require.Equal(t, []uint64{4, 14}, history.BaseFeePerGas)
	/* START ALPHA CHANGE 2382 -- retain the existing receipt-weighted reward expectation -- */
	require.Equal(t, [][]*big.Int{{big.NewInt(10)}}, history.Reward)
	/* FINISH ALPHA CHANGE 2382 -- retain the existing receipt-weighted reward expectation -- */
	require.Equal(t, []float64{0.5}, history.GasUsedRatio)
	require.Equal(t, 1, backend.getBlockByNumberCalls)
	require.Equal(t, 1, backend.getReceiptsCalls)
}

func TestGasHelper_FeeHistoryEmptyLatestBlockDoesNotSearchBack(t *testing.T) {
	t.Parallel()

	backend := createTestBlocks(t, 5)
	backend.calculateBaseFee = func(parent *types.Header) (uint64, error) {
		require.Equal(t, uint64(5), parent.Number)

		return 1, nil
	}

	helper, err := NewGasHelper(DefaultGasHelperConfig, backend)
	require.NoError(t, err)

	history, err := helper.FeeHistory(1, 5, nil)
	require.NoError(t, err)
	require.Equal(t, []uint64{1, 1}, history.BaseFeePerGas)
	require.Equal(t, 1, backend.getBlockByNumberCalls)
	require.Zero(t, backend.getReceiptsCalls)
}

func TestGasHelper_FeeHistoryAtGenesisDoesNotSearchBack(t *testing.T) {
	t.Parallel()

	backend := createTestBlocks(t, 0)
	helper, err := NewGasHelper(DefaultGasHelperConfig, backend)
	require.NoError(t, err)

	history, err := helper.FeeHistory(1, 0, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(0), history.OldestBlock)
	require.Equal(t, []uint64{1, 1}, history.BaseFeePerGas)
	require.Equal(t, 1, backend.getBlockByNumberCalls)
	require.Zero(t, backend.getReceiptsCalls)
}

/* FINISH ALPHA CHANGE 1953 -- test receipt weighting and requested-block next base fee -- */
