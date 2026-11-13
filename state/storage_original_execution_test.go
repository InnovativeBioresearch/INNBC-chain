/* START ALPHA CHANGE 3152 -- verify original storage gas accounting through signed execution and trie publication -- */
package state_test

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type originalStorageCountingStorage struct {
	itrie.Storage
	writes uint64
}

type originalStorageCountingBatch struct {
	itrie.Batch
	owner *originalStorageCountingStorage
}

func (s *originalStorageCountingStorage) Batch() itrie.Batch {
	return &originalStorageCountingBatch{
		Batch: s.Storage.Batch(),
		owner: s,
	}
}

func (b *originalStorageCountingBatch) Write() error {
	if err := b.Batch.Write(); err != nil {
		return err
	}
	b.owner.writes++

	return nil
}

type originalStorageExecutionFixture struct {
	storage  *originalStorageCountingStorage
	world    *itrie.State
	executor *statepkg.Executor
	root     types.Hash
	key      *ecdsa.PrivateKey
	sender   types.Address
	target   types.Address
	coinbase types.Address
	path     string
}

func newOriginalStorageExecutionFixture(
	t *testing.T,
	backend string,
	initial uint64,
	code []byte,
) *originalStorageExecutionFixture {
	t.Helper()

	raw := itrie.NewMemoryStorage()
	path := ""
	if backend == "mdbx" {
		path = t.TempDir()
		var err error
		// A small temporary geometry is sufficient for these bounded fixtures.
		raw, err = itrie.NewMDBXStorageWithMapSize(
			path, hclog.NewNullLogger(), 64*1024*1024,
		)
		require.NoError(t, err)
	}

	storage := &originalStorageCountingStorage{Storage: raw}
	t.Cleanup(func() {
		if storage.Storage != nil {
			require.NoError(t, storage.Close())
		}
	})

	key, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	world := itrie.NewState(storage)
	executor := statepkg.NewExecutor(
		&chain.Params{ChainID: 1982, Forks: chain.AllForksEnabled},
		world,
		hclog.NewNullLogger(),
	)
	/* START ALPHA CHANGE 3810 -- adapt original-storage fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt original-storage fixture to checked hash lookups -- */

	fixture := &originalStorageExecutionFixture{
		storage:  storage,
		world:    world,
		executor: executor,
		key:      key,
		sender:   crypto.PubKeyToAddress(&key.PublicKey),
		target:   types.StringToAddress("0x8201"),
		coinbase: types.StringToAddress("0x8202"),
		path:     path,
	}

	fixture.root, err = executor.WriteGenesis(
		map[types.Address]*chain.GenesisAccount{
			fixture.sender: {
				Balance: big.NewInt(1_000_000_000),
			},
			fixture.target: {
				Balance: big.NewInt(1),
				Code:    append([]byte(nil), code...),
				Storage: map[types.Hash]types.Hash{
					types.ZeroHash: types.BytesToHash(
						new(big.Int).SetUint64(initial).Bytes(),
					),
				},
			},
		},
		types.ZeroHash,
	)
	require.NoError(t, err)

	return fixture
}

func (f *originalStorageExecutionFixture) signed(
	t *testing.T,
	txType types.TxType,
	nonce uint64,
	gas uint64,
	input []byte,
) *types.Transaction {
	t.Helper()

	tx := &types.Transaction{
		Type:     txType,
		Nonce:    nonce,
		To:       f.target.Ptr(),
		Gas:      gas,
		GasPrice: big.NewInt(2),
		Value:    new(big.Int),
		Input:    append([]byte(nil), input...),
	}
	if txType == types.DynamicFeeTx {
		tx.ChainID = big.NewInt(1982)
		tx.GasPrice = new(big.Int)
		tx.GasFeeCap = big.NewInt(10)
		tx.GasTipCap = big.NewInt(1)
	}

	signer := crypto.NewSigner(chain.AllForksEnabled.At(1), 1982)
	signed, err := signer.SignTx(tx, f.key)
	require.NoError(t, err)
	signed.ComputeHash(1)

	return signed
}

func originalStorageSlot(
	t *testing.T,
	world *itrie.State,
	root types.Hash,
	address types.Address,
) types.Hash {
	t.Helper()

	snapshot, err := world.NewSnapshotAt(root)
	require.NoError(t, err)
	account, err := snapshot.GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, account)

	value, err := snapshot.GetStorage(address, account.Root, types.ZeroHash)
	require.NoError(t, err)

	return value
}

func TestOriginalStorage_SequentialExecutionAndPublication(t *testing.T) {
	type step struct {
		value   uint64
		gas     uint64
		cost    uint64
		refund  uint64
		success bool
	}

	tests := []struct {
		name    string
		initial uint64
		steps   []step
	}{
		{
			name:    "new_then_modify",
			initial: 0,
			steps: []step{
				{value: 1, gas: 100_000, cost: 20_000, success: true},
				{value: 2, gas: 100_000, cost: 5_000, success: true},
			},
		},
		{
			name:    "clear_then_add",
			initial: 1,
			steps: []step{
				{value: 0, gas: 100_000, cost: 5_000, refund: 15_000, success: true},
				{value: 2, gas: 100_000, cost: 20_000, success: true},
			},
		},
		{
			name:    "insufficient_recreation_gas_then_success",
			initial: 1,
			steps: []step{
				{value: 0, gas: 100_000, cost: 5_000, refund: 15_000, success: true},
				{value: 2, gas: 30_000, success: false},
				{value: 2, gas: 100_000, cost: 20_000, success: true},
			},
		},
	}

	// PUSH1 0; CALLDATALOAD; PUSH1 0; SSTORE; STOP.
	// Non-storage opcode charges total 9 gas.
	code := []byte{0x60, 0x00, 0x35, 0x60, 0x00, 0x55, 0x00}

	for _, backend := range []string{"memory", "mdbx"} {
		t.Run(backend, func(t *testing.T) {
			for _, txType := range []types.TxType{types.LegacyTx, types.DynamicFeeTx} {
				name := "legacy"
				if txType == types.DynamicFeeTx {
					name = "dynamic"
				}

				t.Run(name, func(t *testing.T) {
					for _, tc := range tests {
						t.Run(tc.name, func(t *testing.T) {
							f := newOriginalStorageExecutionFixture(t, backend, tc.initial, code)
							header := &types.Header{
								Number:   1,
								GasLimit: 5_242_880,
								BaseFee:  1,
							}

							txs := make([]*types.Transaction, len(tc.steps))
							expectedGas := make([]uint64, len(tc.steps))
							for index, current := range tc.steps {
								input := new(big.Int).SetUint64(current.value).
									FillBytes(make([]byte, 32))
								txs[index] = f.signed(
									t, txType, uint64(index), current.gas, input,
								)

								if !current.success {
									expectedGas[index] = current.gas
									continue
								}

								intrinsic, err := statepkg.TransactionGasCost(txs[index], true, true)
								require.NoError(t, err)
								used := intrinsic + 9 + current.cost
								expectedGas[index] = used - min(current.refund, used/2)
							}

							before := f.storage.writes
							tr, err := f.executor.ProcessBlock(
								f.root,
								&types.Block{Header: header, Transactions: txs},
								f.coinbase,
							)
							require.NoError(t, err)
							require.NoError(t, tr.Error())
							require.Equal(t, before, f.storage.writes)
							require.Len(t, tr.Receipts(), len(tc.steps))

							var total uint64
							for index, current := range tc.steps {
								receipt := tr.Receipts()[index]
								require.NotNil(t, receipt.Status)
								status := types.ReceiptSuccess
								if !current.success {
									status = types.ReceiptFailed
								}

								require.Equal(t, status, *receipt.Status)
								require.Equal(t, expectedGas[index], receipt.GasUsed)
								total += expectedGas[index]
								require.Equal(t, total, receipt.CumulativeGasUsed)
							}

							require.Equal(t, total, tr.TotalGas())
							require.Equal(t, uint64(len(tc.steps)), tr.GetNonce(f.sender))
							require.Zero(t, tr.GetRefund())
							require.Equal(t, types.StringToHash("0x02"),
								tr.GetStorage(f.target, types.ZeroHash))

							// Both formats have an effective price of 2 wei.
							fees := new(big.Int).Mul(new(big.Int).SetUint64(total), big.NewInt(2))
							remaining := new(big.Int).Sub(big.NewInt(1_000_000_000), fees)
							require.Zero(t, tr.GetBalance(f.sender).Cmp(remaining))
							require.Zero(t, tr.GetBalance(f.coinbase).Cmp(fees))

							initial := types.BytesToHash(
								new(big.Int).SetUint64(tc.initial).Bytes(),
							)
							require.Equal(t, initial,
								originalStorageSlot(t, f.world, f.root, f.target))

							_, committedRoot, err := tr.Commit()
							require.NoError(t, err)
							require.Equal(t, before+1, f.storage.writes)

							if backend == "mdbx" {
								require.NoError(t, f.storage.Close())
								f.storage.Storage = nil
								reopened, err := itrie.NewMDBXStorageWithMapSize(
									f.path, hclog.NewNullLogger(), 64*1024*1024,
								)
								require.NoError(t, err)
								f.storage.Storage = reopened
							}

							// Use a fresh state cache, including a real MDBX reopen.
							reopenedWorld := itrie.NewState(f.storage)
							require.Equal(t, types.StringToHash("0x02"),
								originalStorageSlot(t, reopenedWorld, committedRoot, f.target))
							require.Equal(t, initial,
								originalStorageSlot(t, reopenedWorld, f.root, f.target))

							committed, err := reopenedWorld.NewSnapshotAt(committedRoot)
							require.NoError(t, err)
							sender, err := committed.GetAccount(f.sender)
							require.NoError(t, err)
							require.NotNil(t, sender)
							require.Equal(t, uint64(len(tc.steps)), sender.Nonce)
							require.Zero(t, sender.Balance.Cmp(remaining))
						})
					}
				})
			}
		})
	}
}

func TestOriginalStorage_InTransactionPricing(t *testing.T) {
	tests := []struct {
		name    string
		initial uint64
		code    []byte
		gasUsed uint64
		final   types.Hash
	}{
		{
			name:    "new_slot_then_dirty_rewrite",
			initial: 0,
			code: []byte{
				0x60, 0x01, 0x60, 0x00, 0x55,
				0x60, 0x02, 0x60, 0x00, 0x55,
				0x00,
			},
			gasUsed: 21_000 + 12 + 20_000 + 800,
			final:   types.StringToHash("0x02"),
		},
		{
			name:    "clear_rewrite_then_restore_original",
			initial: 1,
			code: []byte{
				0x60, 0x00, 0x60, 0x00, 0x55,
				0x60, 0x02, 0x60, 0x00, 0x55,
				0x60, 0x01, 0x60, 0x00, 0x55,
				0x00,
			},
			gasUsed: 21_000 + 18 + 5_000 + 800 + 800 - 4_200,
			final:   types.StringToHash("0x01"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newOriginalStorageExecutionFixture(t, "memory", tc.initial, tc.code)
			tr, err := f.executor.BeginTxn(
				f.root,
				&types.Header{Number: 1, GasLimit: 5_242_880, BaseFee: 1},
				f.coinbase,
			)
			require.NoError(t, err)

			before := f.storage.writes
			tx := f.signed(t, types.LegacyTx, 0, 100_000, nil)
			require.NoError(t, tr.Write(tx))
			require.NoError(t, tr.Error())
			require.Len(t, tr.Receipts(), 1)
			require.Equal(t, types.ReceiptSuccess, *tr.Receipts()[0].Status)
			require.Equal(t, tc.gasUsed, tr.Receipts()[0].GasUsed)
			require.Equal(t, tc.final, tr.GetStorage(f.target, types.ZeroHash))
			require.Zero(t, tr.GetRefund())
			require.Equal(t, before, f.storage.writes)
		})
	}
}

/* FINISH ALPHA CHANGE 3152 -- verify original storage gas accounting through signed execution and trie publication -- */
