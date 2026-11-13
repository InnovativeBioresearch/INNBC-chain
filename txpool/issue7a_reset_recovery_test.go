/* START ALPHA CHANGE 2199 -- test missed-reset recovery and bounded retries -- */
package txpool

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func issue7AResetRecoveryFixture(
	t *testing.T,
) (*TxPool, *issue7APoolStore, *types.Transaction, *types.Transaction) {
	t.Helper()

	pool, store := issue7ANewPool(t)
	t.Cleanup(pool.Close)

	pool.SetSealing(true)

	// Handle promotion synchronously in these tests. No worker or sleep
	// is needed to observe the reset's promotion request.
	pool.promoteReqCh = make(chan promoteRequest, 8)

	store.DefaultHeader = &types.Header{
		Number:    1,
		GasLimit:  mockHeader.GasLimit,
		StateRoot: types.StringToHash("0x7101"),
	}
	store.nonce = 7

	stale := newTx(addr1, 5, 1)
	queued := newTx(addr1, 7, 1)
	require.NotNil(t, stale)
	require.NotNil(t, queued)

	stale.ComputeHash(1)
	queued.ComputeHash(1)

	account := pool.accounts.initOnce(addr1, 5)
	account.enqueue(stale, false)
	account.enqueue(queued, false)

	require.True(t, pool.index.add(stale))
	require.True(t, pool.index.add(queued))
	pool.gauge.increase(slotsRequired(stale, queued))

	store.block = &types.Block{
		Header:       store.DefaultHeader,
		Transactions: []*types.Transaction{stale},
	}

	return pool, store, stale, queued
}

func issue7AResetRetryDue(pool *TxPool) {
	pool.resetMu.Lock()
	pool.resetRetryAt = time.Time{}
	pool.resetMu.Unlock()
}

func issue7AResetRetryDeferred(pool *TxPool) {
	pool.resetMu.Lock()
	pool.resetRetryAt = time.Now().Add(time.Hour)
	pool.resetMu.Unlock()
}

func issue7ARequireResetPromotion(
	t *testing.T,
	pool *TxPool,
	queued *types.Transaction,
) {
	t.Helper()

	var request promoteRequest

	select {
	case request = <-pool.promoteReqCh:
	default:
		t.Fatal("reconciliation did not request promotion")
	}

	require.Equal(t, addr1, request.account)
	pool.handlePromoteRequest(request)

	account := pool.accounts.get(addr1)
	require.Equal(t, uint64(8), account.getNonce())
	require.Zero(t, account.enqueued.length())
	require.Equal(t, uint64(1), account.promoted.length())
	require.Same(t, queued, account.promoted.peek())

	nonce, err := pool.GetNonce(addr1)
	require.NoError(t, err)
	require.Equal(t, uint64(8), nonce)

	pool.Prepare()
	require.Same(t, queued, pool.Peek())
}

func TestIssue7A_ResetRecoveryIncludesAccountsAbsentFromLaterBlock(t *testing.T) {
	for _, phase := range []string{"nonce", "fee"} {
		t.Run(phase, func(t *testing.T) {
			pool, store, stale, queued := issue7AResetRecoveryFixture(t)
			cause := errors.New("injected reset failure")
			localErr := state.WrapLocalStateFailure("reset fixture", cause)

			if phase == "nonce" {
				store.nonceErr = cause
			} else {
				store.calculateBaseFeeFn = func(*types.Header) (uint64, error) {
					return 0, localErr
				}
			}

			pool.processEvent(&blockchain.Event{
				NewChain: []*types.Header{store.block.Header},
			})

			require.ErrorIs(t, pool.resetErr, cause)
			require.ErrorIs(t, pool.resetErr, state.ErrLocalStateFailure)

			account := pool.accounts.get(addr1)
			require.Equal(t, uint64(5), account.getNonce())
			require.Equal(t, uint64(2), account.enqueued.length())
			require.Equal(t, slotsRequired(stale, queued), pool.gauge.read())

			_, exists := pool.index.get(stale.Hash)
			require.True(t, exists)

			// Make suppression deterministic without sleeping.
			issue7AResetRetryDeferred(pool)

			nonce, err := pool.GetNonce(addr1)
			require.Zero(t, nonce)
			require.ErrorIs(t, err, cause)

			candidate := newTx(addr3, 7, 1)
			require.NotNil(t, candidate)
			require.ErrorIs(t, pool.addTx(local, candidate), cause)
			require.Nil(t, pool.accounts.get(addr3))

			// The queued nonce gap must not cause pruning while the
			// account's reset is known to be incomplete.
			pool.pruneAccountsWithNonceHoles()
			require.Equal(t, uint64(2), account.enqueued.length())
			require.Equal(t, slotsRequired(stale, queued), pool.gauge.read())

			// Repairing only the fee does not rehabilitate missed
			// account/index reconciliation.
			store.calculateBaseFeeFn = func(*types.Header) (uint64, error) {
				return 0, nil
			}
			require.NoError(t, pool.SetBaseFee(store.Header()))

			_, err = pool.GetNonce(addr1)
			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, pool.resetErr, cause)

			// An index-only transaction models an entry that has already
			// left its queue but still needs removal after a missed reset.
			indexOnly := newTx(addr2, 0, 1)
			require.NotNil(t, indexOnly)
			indexOnly.ComputeHash(1)
			require.True(t, pool.index.add(indexOnly))

			store.nonceErr = nil
			store.DefaultHeader = &types.Header{
				Number:    2,
				GasLimit:  mockHeader.GasLimit,
				StateRoot: types.StringToHash("0x7102"),
			}

			unrelated := newTx(addr3, 6, 1)
			require.NotNil(t, unrelated)
			store.block = &types.Block{
				Header:       store.DefaultHeader,
				Transactions: []*types.Transaction{unrelated},
			}

			var feeHeader *types.Header
			store.calculateBaseFeeFn = func(header *types.Header) (uint64, error) {
				feeHeader = header

				return 0, nil
			}

			issue7AResetRetryDue(pool)
			pool.processEvent(&blockchain.Event{
				NewChain: []*types.Header{store.block.Header},
			})

			require.NoError(t, pool.resetErr)
			require.True(t, pool.resetRetryAt.IsZero())
			require.Same(t, store.Header(), feeHeader)

			// Account A was absent from the later block, but its missed
			// reset must still have been completed.
			require.Equal(t, uint64(7), account.getNonce())
			require.Equal(t, uint64(1), account.enqueued.length())
			require.Same(t, queued, account.enqueued.peek())
			require.Equal(t, slotsRequired(queued), pool.gauge.read())

			_, exists = pool.index.get(stale.Hash)
			require.False(t, exists)

			_, exists = pool.index.get(indexOnly.Hash)
			require.False(t, exists)

			_, exists = pool.index.get(queued.Hash)
			require.True(t, exists)

			issue7ARequireResetPromotion(t, pool, queued)
		})
	}
}

func TestIssue7A_ResetRecoveryCanBeTriggeredWithoutAnotherBlock(t *testing.T) {
	for _, trigger := range []string{"pending nonce", "admission"} {
		t.Run(trigger, func(t *testing.T) {
			pool, store, stale, queued := issue7AResetRecoveryFixture(t)
			cause := errors.New("temporary nonce read failure")
			store.nonceErr = cause

			pool.processEvent(&blockchain.Event{
				NewChain: []*types.Header{store.block.Header},
			})
			require.ErrorIs(t, pool.resetErr, cause)

			store.nonceErr = nil
			issue7AResetRetryDue(pool)

			switch trigger {
			case "pending nonce":
				nonce, err := pool.GetNonce(addr1)
				require.NoError(t, err)
				require.Equal(t, uint64(7), nonce)

			case "admission":
				candidate := newTx(addr3, 7, 1)
				require.NotNil(t, candidate)
				require.NoError(t, pool.addTx(local, candidate))

				_, exists := pool.index.get(candidate.Hash)
				require.True(t, exists)
				require.NotNil(t, pool.accounts.get(addr3))
			}

			require.NoError(t, pool.resetErr)

			_, exists := pool.index.get(stale.Hash)
			require.False(t, exists)

			account := pool.accounts.get(addr1)
			require.Equal(t, uint64(7), account.getNonce())
			require.Equal(t, uint64(1), account.enqueued.length())
			require.Same(t, queued, account.enqueued.peek())
		})
	}
}

func TestIssue7A_ResetRetryFailurePreservesQueuesAndIsThrottled(t *testing.T) {
	pool, store, stale, queued := issue7AResetRecoveryFixture(t)
	cause := errors.New("persistent nonce read failure")
	store.nonceErr = cause

	pool.processEvent(&blockchain.Event{
		NewChain: []*types.Header{store.block.Header},
	})
	require.ErrorIs(t, pool.resetErr, cause)

	// One eligible retry encounters the same failure.
	issue7AResetRetryDue(pool)
	readsBefore := store.nonceCalls

	_, err := pool.GetNonce(addr1)
	require.ErrorIs(t, err, cause)
	require.Equal(t, readsBefore+1, store.nonceCalls)

	account := pool.accounts.get(addr1)
	require.Equal(t, uint64(5), account.getNonce())
	require.Equal(t, uint64(2), account.enqueued.length())
	require.Equal(t, slotsRequired(stale, queued), pool.gauge.read())

	_, exists := pool.index.get(stale.Hash)
	require.True(t, exists)

	// Keep the retry window closed deterministically while concurrent
	// callers verify that no additional backend attempts occur.
	issue7AResetRetryDeferred(pool)
	readsBefore = store.nonceCalls

	const callers = 24
	results := make(chan error, callers)
	var wg sync.WaitGroup

	for i := 0; i < callers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := pool.GetNonce(addr1)
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	for err := range results {
		require.ErrorIs(t, err, cause)
		require.ErrorIs(t, err, state.ErrLocalStateFailure)
	}

	require.Equal(t, readsBefore, store.nonceCalls)
	require.Equal(t, uint64(2), account.enqueued.length())

	// A later successful retry must still recover this same pool.
	store.nonceErr = nil
	issue7AResetRetryDue(pool)

	nonce, err := pool.GetNonce(addr1)
	require.NoError(t, err)
	require.Equal(t, uint64(7), nonce)

	issue7ARequireResetPromotion(t, pool, queued)
}

func TestIssue7A_ResetRetryDoesNotStartAfterShutdown(t *testing.T) {
	pool, store, _, _ := issue7AResetRecoveryFixture(t)
	cause := errors.New("reset failed before shutdown")
	store.nonceErr = cause

	pool.processEvent(&blockchain.Event{
		NewChain: []*types.Header{store.block.Header},
	})
	require.ErrorIs(t, pool.resetErr, cause)

	store.nonceErr = nil
	issue7AResetRetryDue(pool)
	readsBefore := store.nonceCalls

	pool.Close()

	_, err := pool.GetNonce(addr1)
	require.ErrorIs(t, err, cause)
	require.Equal(t, readsBefore, store.nonceCalls)
}

/* FINISH ALPHA CHANGE 2199 -- test missed-reset recovery and bounded retries -- */
