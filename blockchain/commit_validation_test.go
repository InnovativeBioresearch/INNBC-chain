/* START ALPHA CHANGE 2668 -- test imported commitment rejection before state and receipt publication -- */
package blockchain

import (
	"errors"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/innovativebioresearch/innbc-chain/types/buildroot"
	"github.com/stretchr/testify/require"
)

type importedCommitValidationStorage struct {
	itrie.Storage
	writes   int
	writeErr error
}

type importedCommitValidationBatch struct {
	itrie.Batch
	owner *importedCommitValidationStorage
}

func (s *importedCommitValidationStorage) Batch() itrie.Batch {
	return &importedCommitValidationBatch{
		Batch: s.Storage.Batch(),
		owner: s,
	}
}

func (b *importedCommitValidationBatch) Write() error {
	b.owner.writes++
	if b.owner.writeErr != nil {
		return b.owner.writeErr
	}

	return b.Batch.Write()
}

func importedCommitValidationTransition(
	storage itrie.Storage,
) *state.Transition {
	snapshot := itrie.NewState(storage).NewSnapshot()
	txn := state.NewTxn(snapshot)
	txn.SetBalance(types.StringToAddress("0x9301"), big.NewInt(7))
	txn.SetCode(types.StringToAddress("0x9301"), []byte{0x60, 0x01})
	txn.SetState(
		types.StringToAddress("0x9301"),
		types.StringToHash("0x01"),
		types.StringToHash("0x02"),
	)

	return state.NewTransition(chain.ForksInTime{}, snapshot, txn)
}

func TestImportedCommitValidation_Publication(t *testing.T) {
	reference := importedCommitValidationTransition(itrie.NewMemoryStorage())
	_, expectedRoot, err := reference.Commit()
	require.NoError(t, err)

	writeFailure := errors.New("injected imported state publication failure")
	cases := []struct {
		name    string
		mutate  func(*types.Block)
		wantErr error
	}{
		{"accepted", func(*types.Block) {}, nil},
		{"state root", func(b *types.Block) {
			b.Header.StateRoot = types.Hash{0x94}
		}, ErrInvalidStateRoot},
		{"gas used", func(b *types.Block) {
			b.Header.GasUsed = 1
		}, ErrInvalidGasUsed},
		{"receipt root", func(b *types.Block) {
			b.Header.ReceiptsRoot = types.ZeroHash
		}, ErrInvalidReceiptsRoot},
		{"bloom", func(b *types.Block) {
			b.Header.LogsBloom[0] = 1
		}, ErrInvalidLogsBloom},
		{"receipt count", func(b *types.Block) {
			b.Transactions = []*types.Transaction{{
				GasPrice: new(big.Int),
				Value:    new(big.Int),
				V:        new(big.Int),
				R:        new(big.Int),
				S:        new(big.Int),
			}}
			b.Header.TxRoot = buildroot.CalculateTransactionsRoot(
				b.Transactions, b.Number(),
			)
		}, ErrInvalidReceiptsSize},
		{"write failure", func(*types.Block) {}, writeFailure},
	}

	/*
		The controlled executor supplies a real, unpublished trie transition.
		This isolates publication ordering and permits an independent
		receipt-count mismatch without claiming an end-to-end import test.
	*/
	for _, entry := range []string{"verifyBlockBody", "extractBlockReceipts"} {
		t.Run(entry, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					storage := &importedCommitValidationStorage{
						Storage: itrie.NewMemoryStorage(),
					}
					transition := importedCommitValidationTransition(storage)
					if tc.name == "write failure" {
						storage.writeErr = writeFailure
					}

					bc := NewTestBlockchain(t, nil)
					executions := 0
					bc.executor = &mockExecutor{
						processBlockFn: func(
							types.Hash, *types.Block, types.Address,
						) (*state.Transition, error) {
							executions++

							return transition, nil
						},
					}

					parent := bc.Header()
					block := &types.Block{
						Header: &types.Header{
							Number:       parent.Number + 1,
							ParentHash:   parent.Hash,
							StateRoot:    expectedRoot,
							TxRoot:       types.EmptyRootHash,
							Sha3Uncles:   types.EmptyUncleHash,
							ReceiptsRoot: types.EmptyRootHash,
							GasLimit:     1_000_000,
						},
					}
					tc.mutate(block)
					block.Header.ComputeHash()

					_, found, readErr := storage.Get(expectedRoot.Bytes())
					require.NoError(t, readErr)
					require.False(t, found)

					var receipts []*types.Receipt
					var executionErr error
					if entry == "verifyBlockBody" {
						receipts, executionErr = bc.verifyBlockBody(block)
					} else {
						receipts, executionErr = bc.extractBlockReceipts(block)
					}
					require.Equal(t, 1, executions)

					if tc.name == "accepted" {
						require.NoError(t, executionErr)
						require.Equal(t, transition.Receipts(), receipts)
						require.Equal(t, 1, storage.writes)
						require.True(t, bc.receiptsCache.Contains(block.Header.Hash))

						_, found, readErr = storage.Get(expectedRoot.Bytes())
						require.NoError(t, readErr)
						require.True(t, found)

						_, cachedErr := bc.extractBlockReceipts(block)
						require.NoError(t, cachedErr)
						require.Equal(t, 1, executions)
						require.Equal(t, 1, storage.writes)

						return
					}

					require.ErrorIs(t, executionErr, tc.wantErr)
					require.Nil(t, receipts)
					require.False(t, bc.receiptsCache.Contains(block.Header.Hash))

					if tc.name == "write failure" {
						require.ErrorIs(t, executionErr, state.ErrLocalStateFailure)
						require.Equal(t, 1, storage.writes)
					} else {
						require.False(t, errors.Is(executionErr, state.ErrLocalStateFailure))
						require.Zero(t, storage.writes)
					}

					_, found, readErr = storage.Get(expectedRoot.Bytes())
					require.NoError(t, readErr)
					require.False(t, found)
				})
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2668 -- test imported commitment rejection before state and receipt publication -- */
