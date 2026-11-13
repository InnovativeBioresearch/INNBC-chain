package gasprice

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/helper/tests"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func TestGasHelper_MaxPriorityFeePerGas(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		Name       string
		Expected   *big.Int
		GetBackend func() Blockchain
	}{
		// START ALPHA CHANGE 1212 -- expect priority fee to use current base fee directly --
		{
			Name:     "Chain just started",
			Expected: big.NewInt(1),
			GetBackend: func() Blockchain {
				genesis := &types.Header{
					Number: 0,
					Hash:   types.StringToHash("genesis"),
				}
				backend := new(backendMock)
				backend.On("Header").Return(genesis)

				return backend
			},
		},
		{
			Name:     "Uses current base fee",
			Expected: new(big.Int).SetUint64(chain.GenesisBaseFee),
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 10)

				return backend
			},
		},
		// FINISH ALPHA CHANGE 1212 -- expect priority fee to use current base fee directly --
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			backend := tc.GetBackend()
			gasHelper, err := NewGasHelper(DefaultGasHelperConfig, backend)
			require.NoError(t, err)
			price, err := gasHelper.MaxPriorityFeePerGas()

			require.NoError(t, err)
			require.Equal(t, tc.Expected, price)
		})
	}
}

// START ALPHA CHANGE 1213 -- guard priority fee recommendation against block reads
func TestGasHelper_MaxPriorityFeePerGasDoesNotReadBlocks(t *testing.T) {
	t.Parallel()

	backend := createTestBlocks(t, 50)
	gasHelper, err := NewGasHelper(DefaultGasHelperConfig, backend)
	require.NoError(t, err)

	price, err := gasHelper.MaxPriorityFeePerGas()
	require.NoError(t, err)
	require.Equal(t, new(big.Int).SetUint64(chain.GenesisBaseFee), price)
	require.Zero(t, backend.getBlockByHashCalls)
}

// FINISH ALPHA CHANGE 1213 -- guard priority fee recommendation against block reads

func createTestBlocks(t *testing.T, numOfBlocks int) *backendMock {
	t.Helper()

	backend := &backendMock{blocks: make(map[types.Hash]*types.Block), blocksByNumber: make(map[uint64]*types.Block)}
	genesis := &types.Block{
		Header: &types.Header{
			Number:  0,
			Hash:    types.StringToHash("genesis"),
			Miner:   types.ZeroAddress.Bytes(),
			BaseFee: chain.GenesisBaseFee,
		},
	}
	backend.blocks[genesis.Hash()] = genesis
	backend.blocksByNumber[genesis.Number()] = genesis

	currentBlock := genesis

	for i := 1; i <= numOfBlocks; i++ {
		block := &types.Block{
			Header: &types.Header{
				Number: uint64(i),
				// START ALPHA CHANGE 1009 -- use fmt.Appendf for test block hash bytes --
				Hash: types.BytesToHash(fmt.Appendf(nil, "Block %d", i)),
				// FINISH ALPHA CHANGE 1009 -- use fmt.Appendf for test block hash bytes --
				Miner:      types.ZeroAddress.Bytes(),
				ParentHash: currentBlock.Hash(),
				BaseFee:    chain.GenesisBaseFee,
			},
		}
		backend.blocksByNumber[block.Number()] = block
		backend.blocks[block.Hash()] = block
		currentBlock = block
	}

	backend.On("Header").Return(currentBlock.Header)

	return backend
}

func createTestTxs(t *testing.T, backend *backendMock, numOfTxsPerBlock, txCap int) {
	t.Helper()

	/* START ALPHA CHANGE 1955 -- use deterministic local randomness in fee-history fixtures -- */
	random := rand.New(rand.NewSource(1))
	/* FINISH ALPHA CHANGE 1955 -- use deterministic local randomness in fee-history fixtures -- */

	for _, b := range backend.blocks {
		signer := crypto.NewSigner(backend.Config().Forks.At(b.Number()),
			uint64(backend.Config().ChainID))

		b.Transactions = make([]*types.Transaction, numOfTxsPerBlock)

		/* START ALPHA CHANGE 1956 -- use modern integer range and local randomness -- */
		for i := range numOfTxsPerBlock {
			senderKey, sender := tests.GenerateKeyAndAddr(t)

			tx := &types.Transaction{
				From:      sender,
				Value:     ethgo.Ether(1),
				To:        &types.ZeroAddress,
				Type:      types.DynamicFeeTx,
				GasTipCap: ethgo.Gwei(uint64(random.Intn(txCap))),
				GasFeeCap: ethgo.Gwei(uint64(random.Intn(txCap) + txCap)),
			}
			/* FINISH ALPHA CHANGE 1956 -- use modern integer range and local randomness -- */

			tx, err := signer.SignTx(tx, senderKey)
			require.NoError(t, err)

			b.Transactions[i] = tx
		}
	}
}

var _ Blockchain = (*backendMock)(nil)

type backendMock struct {
	mock.Mock
	blocks         map[types.Hash]*types.Block
	blocksByNumber map[uint64]*types.Block
	/* START ALPHA CHANGE 1949 -- supply fee-history receipts and next-base-fee behavior -- */
	receiptsByHash        map[types.Hash][]*types.Receipt
	calculateBaseFee      func(*types.Header) (uint64, error)
	getBlockByNumberCalls int
	getReceiptsCalls      int
	/* FINISH ALPHA CHANGE 1949 -- supply fee-history receipts and next-base-fee behavior -- */
	// START ALPHA CHANGE 1214 -- count unexpected priority fee block reads
	getBlockByHashCalls int
	// FINISH ALPHA CHANGE 1214 -- count unexpected priority fee block reads
}

func (b *backendMock) Header() *types.Header {
	args := b.Called()

	return args.Get(0).(*types.Header) //nolint:forcetypeassert
}

func (b *backendMock) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	// START ALPHA CHANGE 1214 -- count unexpected priority fee block reads
	b.getBlockByHashCalls++
	// FINISH ALPHA CHANGE 1214 -- count unexpected priority fee block reads

	if len(b.blocks) == 0 {
		args := b.Called(hash, full)

		return args.Get(0).(*types.Block), args.Get(1).(bool) //nolint:forcetypeassert
	}

	block, exists := b.blocks[hash]

	return block, exists
}

/* START ALPHA CHANGE 1949 -- supply fee-history receipts and next-base-fee behavior -- */
func (b *backendMock) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	b.getReceiptsCalls++

	return b.receiptsByHash[hash], nil
}

func (b *backendMock) CalculateBaseFee(parent *types.Header) (uint64, error) {
	if b.calculateBaseFee != nil {
		return b.calculateBaseFee(parent)
	}

	return parent.BaseFee, nil
}

/* FINISH ALPHA CHANGE 1949 -- supply fee-history receipts and next-base-fee behavior -- */

func (b *backendMock) Config() *chain.Params {
	return &chain.Params{
		ChainID: 1,
		Forks:   chain.AllForksEnabled,
	}
}
