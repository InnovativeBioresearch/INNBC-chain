// START ALPHA CHANGE 2357 -- test proposal abandonment capacity and incomplete block reset recovery --
package txpool

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestIssue13_AbandonedSelectionKeepsCapacityAndAllowsReplacement(t *testing.T) {
	for _, abort := range []bool{false, true} {
		name := "successful candidate not finalized"
		if abort {
			name = "candidate aborted"
		}

		t.Run(name, func(t *testing.T) {
			pool := issue13Pool(t, 1)
			pool.promoteReqCh = make(chan promoteRequest, 8)

			original := issue13SelectionTx(0, 1, 10)
			require.NoError(t, pool.AddTx(original))
			pool.handlePromoteRequest(promoteRequest{account: addr1})

			cause := errors.New("candidate abandoned")
			err := pool.WithReconciledPool(func() error {
				pool.Prepare()

				if tx := pool.Peek(); tx != original {
					return errors.New("unexpected original selection")
				}

				pool.Pop(original)
				if tx := pool.Peek(); tx != nil {
					return errors.New("selected nonce was offered twice")
				}

				if abort {
					return cause
				}

				return nil
			})

			if abort {
				require.ErrorIs(t, err, cause)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, uint64(1), pool.gauge.read())
			require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))
			requireIssue13Queues(t, pool, addr1, 0, 1)

			// Selection must not create uncharged space for another sender.
			other := issue13SelectionTx(0, 1, 10)
			other.From = addr2
			other.Input[0] = 2
			require.ErrorIs(t, pool.AddTx(other), ErrTxPoolOverflow)
			require.Nil(t, pool.accounts.get(addr2))

			// The unmined nonce remains replaceable after the candidate
			// has finished or aborted.
			replacement := issue13SelectionTx(0, 1, 20)
			require.NoError(t, pool.AddTx(replacement))

			_, exists := pool.index.get(original.Hash)
			require.False(t, exists)
			stored, exists := pool.index.get(replacement.Hash)
			require.True(t, exists)
			require.Same(t, replacement, stored)
			require.Equal(t, uint64(1), pool.gauge.read())

			require.NoError(t, pool.WithReconciledPool(func() error {
				pool.Prepare()

				if tx := pool.Peek(); tx != replacement {
					return errors.New("replacement was not selectable on retry")
				}

				pool.Pop(replacement)
				pool.Pop(replacement)

				if tx := pool.Peek(); tx != nil {
					return errors.New("duplicate Pop repeated a nonce")
				}

				return nil
			}))

			requireIssue13Queues(t, pool, addr1, 0, 1)
			require.Equal(t, uint64(1), pool.gauge.read())

			nonce, err := pool.GetNonce(addr1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), nonce)
		})
	}
}

func TestIssue13_DropAfterSelectionPreservesSuccessfulPrefix(t *testing.T) {
	for _, operation := range []string{"drop", "demotion threshold"} {
		t.Run(operation, func(t *testing.T) {
			pool := issue13Pool(t, 8)
			pool.promoteReqCh = make(chan promoteRequest, 8)

			first := issue13SelectionTx(0, 1, 10)
			second := issue13SelectionTx(1, 1, 10)
			third := issue13SelectionTx(2, 1, 10)

			for _, tx := range []*types.Transaction{first, second, third} {
				require.NoError(t, pool.AddTx(tx))
			}
			pool.handlePromoteRequest(promoteRequest{account: addr1})

			require.NoError(t, pool.WithReconciledPool(func() error {
				pool.Prepare()
				if tx := pool.Peek(); tx != first {
					return errors.New("unexpected first transaction")
				}
				pool.Pop(first)

				if tx := pool.Peek(); tx != second {
					return errors.New("unexpected second transaction")
				}

				if operation == "drop" {
					pool.Drop(second)
				} else {
					account := pool.accounts.get(addr1)
					atomic.StoreUint64(&account.demotions, maxAccountDemotions)
					pool.Demote(second)
				}

				return nil
			}))

			requireIssue13Queues(t, pool, addr1, 0, 1)
			require.Equal(t, uint64(1), pool.gauge.read())
			require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))

			stored, exists := pool.index.get(first.Hash)
			require.True(t, exists)
			require.Same(t, first, stored)

			for _, tx := range []*types.Transaction{second, third} {
				_, exists := pool.index.get(tx.Hash)
				require.False(t, exists)
			}

			nonce, err := pool.GetNonce(addr1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), nonce)

			require.NoError(t, pool.WithReconciledPool(func() error {
				pool.Prepare()
				if tx := pool.Peek(); tx != first {
					return errors.New("successful prefix was lost")
				}
				pool.Pop(first)

				if tx := pool.Peek(); tx != nil {
					return errors.New("dropped suffix became selectable")
				}

				return nil
			}))
		})
	}
}

type issue13MissingBlockStore struct {
	*issue7APoolStore
	missing types.Hash
	reads   int
}

func (s *issue13MissingBlockStore) GetBlockByHash(
	hash types.Hash,
	full bool,
) (*types.Block, bool) {
	s.reads++
	if hash == s.missing {
		return nil, false
	}

	return s.issue7APoolStore.GetBlockByHash(hash, full)
}

func TestIssue13_MissingResetBlockPreservesPoolAndRecovers(t *testing.T) {
	pool, store := issue7ANewPool(t)
	t.Cleanup(pool.Close)
	pool.SetSealing(true)
	pool.promoteReqCh = make(chan promoteRequest, 8)

	tx := issue13SelectionTx(0, 1, 10)
	require.NoError(t, pool.AddTx(tx))
	pool.handlePromoteRequest(promoteRequest{account: addr1})

	originalAccount := pool.accounts.get(addr1)
	require.NotNil(t, originalAccount)

	store.DefaultHeader = &types.Header{
		Number:   2,
		GasLimit: mockHeader.GasLimit,
	}
	store.nonce = 1

	firstHeader := &types.Header{
		Number: 1,
		Hash:   types.StringToHash("0x135701"),
	}
	missingHeader := &types.Header{
		Number: 2,
		Hash:   types.StringToHash("0x135702"),
	}
	store.block = &types.Block{
		Header:       firstHeader,
		Transactions: []*types.Transaction{tx},
	}

	backend := &issue13MissingBlockStore{
		issue7APoolStore: store,
		missing:          missingHeader.Hash,
	}
	pool.store = backend

	// The first block stages a mined-hash removal. The second fails.
	// Nothing from the incomplete event may be published.
	pool.ResetWithHeaders(firstHeader, missingHeader)

	require.ErrorIs(t, pool.resetErr, state.ErrLocalStateFailure)
	require.Equal(t, 2, backend.reads)
	require.Same(t, originalAccount, pool.accounts.get(addr1))
	requireIssue13Queues(t, pool, addr1, 0, 1)
	require.Equal(t, uint64(1), pool.gauge.read())
	require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))

	indexed, exists := pool.index.get(tx.Hash)
	require.True(t, exists)
	require.Same(t, tx, indexed)

	issue7AResetRetryDeferred(pool)
	called := false
	err := pool.WithReconciledPool(func() error {
		called = true

		return nil
	})
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.False(t, called)
	require.Equal(t, 2, backend.reads)

	// Full reconciliation uses current account state. It does not need
	// another block event or a reread of the unavailable historical body.
	issue7AResetRetryDue(pool)
	require.NoError(t, pool.WithReconciledPool(func() error {
		return nil
	}))

	require.NoError(t, pool.resetErr)
	require.Equal(t, 2, backend.reads)
	require.Nil(t, pool.accounts.get(addr1))
	require.False(t, pool.index.hasSender(addr1))
	require.Zero(t, pool.gauge.read())
	require.Zero(t, atomic.LoadInt64(&pool.pending))
	require.Zero(t, atomic.LoadUint64(&pool.accounts.count))

	nonce, err := pool.GetNonce(addr1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), nonce)
}

// FINISH ALPHA CHANGE 2357 -- test proposal abandonment capacity and incomplete block reset recovery --
