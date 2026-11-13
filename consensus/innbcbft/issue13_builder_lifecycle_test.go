// START ALPHA CHANGE 2358 -- test real builder failures without losing earlier successful transactions --
package innbcbft

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/txpool"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type issue13BuilderState struct {
	state.State
	address   types.Address
	readErr   error
	commitErr error
}

type issue13BuilderSnapshot struct {
	state.Snapshot
	owner *issue13BuilderState
}

func (s *issue13BuilderState) NewSnapshotAt(root types.Hash) (state.Snapshot, error) {
	snapshot, err := s.State.NewSnapshotAt(root)
	if err != nil {
		return nil, err
	}

	return &issue13BuilderSnapshot{
		Snapshot: snapshot,
		owner:    s,
	}, nil
}

func (s *issue13BuilderSnapshot) GetAccount(address types.Address) (*state.Account, error) {
	if address == s.owner.address && s.owner.readErr != nil {
		return nil, s.owner.readErr
	}

	return s.Snapshot.GetAccount(address)
}

func (s *issue13BuilderSnapshot) Commit(objects []*state.Object) (state.Snapshot, []byte, error) {
	if s.owner.commitErr != nil {
		return nil, nil, s.owner.commitErr
	}

	return s.Snapshot.Commit(objects)
}

type issue13BuilderPoolStore struct {
	state  state.State
	header *types.Header
	block  *types.Block
}

func (s *issue13BuilderPoolStore) Header() *types.Header {
	return s.header
}

func (s *issue13BuilderPoolStore) GetNonce(
	root types.Hash,
	address types.Address,
) (uint64, error) {
	snapshot, err := s.state.NewSnapshotAt(root)
	if err != nil {
		return 0, err
	}

	account, err := snapshot.GetAccount(address)
	if err != nil {
		return 0, err
	}
	if account == nil {
		return 0, nil
	}

	return account.Nonce, nil
}

func (s *issue13BuilderPoolStore) GetBalance(
	root types.Hash,
	address types.Address,
) (*big.Int, error) {
	snapshot, err := s.state.NewSnapshotAt(root)
	if err != nil {
		return nil, err
	}

	account, err := snapshot.GetAccount(address)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return new(big.Int), nil
	}

	return new(big.Int).Set(account.Balance), nil
}

func (s *issue13BuilderPoolStore) GetBlockByHash(
	hash types.Hash,
	_ bool,
) (*types.Block, bool) {
	if s.block == nil || s.block.Hash() != hash {
		return nil, false
	}

	return s.block, true
}

func (s *issue13BuilderPoolStore) CalculateBaseFee(header *types.Header) (uint64, error) {
	return header.BaseFee, nil
}

type issue13BuilderSigner struct{}

func (*issue13BuilderSigner) Sender(tx *types.Transaction) (types.Address, error) {
	return tx.From, nil
}

func TestIssue13_BuilderFailureAndAbandonmentKeepTransactions(t *testing.T) {
	for _, phase := range []string{"later execution failure", "commit failure", "abandoned proposal"} {
		t.Run(phase, func(t *testing.T) {
			logger := hclog.NewNullLogger()
			firstSender := types.StringToAddress("0x135801")
			secondSender := types.StringToAddress("0x135802")
			receiver := types.StringToAddress("0x135803")

			base := itrie.NewState(itrie.NewMemoryStorage())
			genesisExecutor := issue7AConsensusExecutor(base)
			root, err := genesisExecutor.WriteGenesis(
				map[types.Address]*chain.GenesisAccount{
					firstSender:  {Balance: big.NewInt(1_000_000_000)},
					secondSender: {Balance: big.NewInt(1_000_000_000)},
				},
				types.ZeroHash,
			)
			require.NoError(t, err)

			parent := &types.Header{
				StateRoot: root,
				GasLimit:  1_000_000,
				Timestamp: 100,
			}
			store := &issue13BuilderPoolStore{
				state:  base,
				header: parent,
			}
			pool, err := txpool.NewTxPool(
				logger,
				chain.AllForksEnabled,
				store,
				nil,
				nil,
				&txpool.Config{
					PriceLimit:         1,
					MaxSlots:           8,
					MaxAccountEnqueued: 8,
					ChainID:            big.NewInt(100),
				},
			)
			require.NoError(t, err)
			t.Cleanup(pool.Close)
			pool.SetSigner(&issue13BuilderSigner{})
			pool.SetSealing(true)
			pool.Start()

			first := &types.Transaction{
				From:     firstSender,
				To:       &receiver,
				Gas:      21_000,
				GasPrice: big.NewInt(2),
				Value:    new(big.Int),
			}
			second := &types.Transaction{
				From:     secondSender,
				To:       &receiver,
				Gas:      21_000,
				GasPrice: big.NewInt(1),
				Value:    new(big.Int),
			}

			require.NoError(t, pool.AddTx(first))
			require.NoError(t, pool.AddTx(second))
			require.Eventually(t, func() bool {
				return pool.Length() == 2
			}, 5*time.Second, time.Millisecond)

			cause := errors.New("injected builder failure")
			backend := &issue13BuilderState{
				State:   base,
				address: secondSender,
			}
			if phase == "later execution failure" {
				backend.readErr = cause
			}
			if phase == "commit failure" {
				backend.commitErr = cause
			}

			builder := NewBlockBuilder(&BlockBuilderParams{
				Parent:    parent,
				Executor:  issue7AConsensusExecutor(backend),
				GasLimit:  parent.GasLimit,
				BlockTime: time.Second,
				TxPool:    pool,
				Logger:    logger,
			})
			require.NoError(t, builder.Reset())

			done := make(chan error, 1)
			go func() {
				done <- builder.Fill()
			}()

			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("builder did not finish")
			}

			if phase == "later execution failure" {
				require.ErrorIs(t, err, cause)
				require.ErrorIs(t, err, state.ErrLocalStateFailure)
				require.Len(t, builder.txns, 1)
				require.Same(t, first, builder.txns[0])
			} else {
				require.NoError(t, err)
				require.Len(t, builder.txns, 2)

				built, buildErr := builder.Build(nil)
				if phase == "commit failure" {
					require.Nil(t, built)
					require.ErrorIs(t, buildErr, cause)
				} else {
					require.NoError(t, buildErr)
					require.NotNil(t, built)
					// Deliberately do not finalize this candidate.
				}
			}

			// Earlier successful execution must not consume pool ownership.
			require.Equal(t, uint64(2), pool.Length())
			used, _ := pool.GetCapacity()
			require.Equal(t, uint64(2), used)

			for _, tx := range []*types.Transaction{first, second} {
				indexed, exists := pool.GetPendingTx(tx.Hash)
				require.True(t, exists)
				require.Same(t, tx, indexed)
			}

			backend.readErr = nil
			backend.commitErr = nil

			// A fresh candidate must execute both transactions again.
			require.NoError(t, builder.Reset())
			require.NoError(t, builder.Fill())
			require.Len(t, builder.txns, 2)
			require.Same(t, first, builder.txns[0])
			require.Same(t, second, builder.txns[1])

			full, err := builder.Build(nil)
			require.NoError(t, err)
			require.Len(t, full.Block.Transactions, 2)

			// Finalization, rather than proposal construction, releases slots.
			store.header = full.Block.Header
			store.block = full.Block
			pool.ResetWithHeaders(full.Block.Header)

			require.Zero(t, pool.Length())
			used, _ = pool.GetCapacity()
			require.Zero(t, used)

			for _, tx := range []*types.Transaction{first, second} {
				_, exists := pool.GetPendingTx(tx.Hash)
				require.False(t, exists)

				nonce, err := pool.GetNonce(tx.From)
				require.NoError(t, err)
				require.Equal(t, uint64(1), nonce)
			}
		})
	}
}

// FINISH ALPHA CHANGE 2358 -- test real builder failures without losing earlier successful transactions --
