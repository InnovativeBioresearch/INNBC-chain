/* START ALPHA CHANGE 2204 -- test proposal pool recovery and reset exclusion -- */
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

func TestIssue7A_BuildGuardRejectsIncompleteReset(t *testing.T) {
	for _, retryDue := range []bool{false, true} {
		name := "throttled"
		if retryDue {
			name = "retry_fails"
		}

		t.Run(name, func(t *testing.T) {
			pool, store, stale, queued := issue7AResetRecoveryFixture(t)

			// Put the stale transaction in the executable queue, where
			// an unguarded builder could select and penalize it.
			pool.handlePromoteRequest(promoteRequest{account: addr1})
			account := pool.accounts.get(addr1)
			require.Same(t, stale, account.promoted.peek())
			require.Same(t, queued, account.enqueued.peek())

			account.incrementDemotions()
			cause := errors.New("injected reset nonce failure")
			store.nonceErr = cause

			pool.processEvent(&blockchain.Event{
				NewChain: []*types.Header{store.block.Header},
			})
			require.ErrorIs(t, pool.resetErr, cause)

			if retryDue {
				issue7AResetRetryDue(pool)
			} else {
				issue7AResetRetryDeferred(pool)
			}

			readsBefore := store.nonceCalls
			slotsBefore := pool.gauge.read()
			called := false

			err := pool.WithReconciledPool(func() error {
				called = true
				pool.Prepare()

				return nil
			})

			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, state.ErrLocalStateFailure)
			require.False(t, called)

			expectedReads := readsBefore
			if retryDue {
				expectedReads++
			}
			require.Equal(t, expectedReads, store.nonceCalls)

			require.Equal(t, uint64(1), account.Demotions())
			require.Equal(t, slotsBefore, pool.gauge.read())
			require.Same(t, stale, account.promoted.peek())
			require.Same(t, queued, account.enqueued.peek())

			_, exists := pool.index.get(stale.Hash)
			require.True(t, exists)
			_, exists = pool.index.get(queued.Hash)
			require.True(t, exists)
		})
	}
}

func TestIssue7A_BuildGuardRecoversBeforeSelection(t *testing.T) {
	pool, store, stale, queued := issue7AResetRecoveryFixture(t)
	pool.handlePromoteRequest(promoteRequest{account: addr1})

	cause := errors.New("temporary reset nonce failure")
	store.nonceErr = cause

	pool.processEvent(&blockchain.Event{
		NewChain: []*types.Header{store.block.Header},
	})
	require.ErrorIs(t, pool.resetErr, cause)

	store.nonceErr = nil
	issue7AResetRetryDue(pool)

	called := false
	err := pool.WithReconciledPool(func() error {
		called = true

		if pool.resetErr != nil {
			return errors.New("selection started with an incomplete reset")
		}
		if _, exists := pool.index.get(stale.Hash); exists {
			return errors.New("selection started before stale index removal")
		}

		pool.Prepare()

		// This fixture handles promotion explicitly. Until that request
		// is handled, no transaction should be executable.
		if tx := pool.Peek(); tx != nil {
			return errors.New("selection exposed a stale promoted transaction")
		}

		return nil
	})

	require.NoError(t, err)
	require.True(t, called)
	require.Nil(t, pool.resetErr)
	require.Equal(t, slotsRequired(queued), pool.gauge.read())

	// The valid later transaction remains and becomes executable.
	issue7ARequireResetPromotion(t, pool, queued)
}

func TestIssue7A_BuildGuardExcludesResetUntilPoolActionCompletes(t *testing.T) {
	pool, store, stale, queued := issue7AResetRecoveryFixture(t)
	pool.handlePromoteRequest(promoteRequest{account: addr1})

	entered := make(chan struct{})
	release := make(chan struct{})
	buildDone := make(chan error, 1)
	resetStarted := make(chan struct{})
	resetDone := make(chan struct{})

	var releaseOnce sync.Once
	unblock := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	// Registered after the fixture cleanup, so failure also releases
	// the callback before pool.Close waits for runtime work.
	t.Cleanup(unblock)

	callbackErr := errors.New("injected callback completion error")

	go func() {
		buildDone <- pool.WithReconciledPool(func() error {
			pool.Prepare()
			selected := pool.Peek()
			if selected != stale {
				return errors.New("unexpected selected transaction")
			}

			close(entered)
			<-release

			// The reset must not prune or reset this account between
			// selecting the transaction and applying its pool action.
			pool.Demote(selected)

			return callbackErr
		})
	}()

	select {
	case <-entered:
	case err := <-buildDone:
		t.Fatalf("callback exited before selection: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("protected selection did not start")
	}

	// Verify the callback actually holds the reset read lock.
	locked := pool.resetMu.TryLock()
	if locked {
		pool.resetMu.Unlock()
	}
	require.False(t, locked)

	go func() {
		close(resetStarted)
		pool.ResetWithHeaders(store.block.Header)
		close(resetDone)
	}()

	<-resetStarted

	select {
	case <-resetDone:
		t.Fatal("reset completed while proposal consumption was protected")
	default:
	}

	unblock()

	select {
	case err := <-buildDone:
		require.ErrorIs(t, err, callbackErr)
	case <-time.After(5 * time.Second):
		t.Fatal("protected callback did not finish")
	}

	// Even an error returned by the callback must release the lock.
	select {
	case <-resetDone:
	case <-time.After(5 * time.Second):
		t.Fatal("reset remained blocked after callback completion")
	}

	account := pool.accounts.get(addr1)
	require.Zero(t, account.Demotions())
	require.Nil(t, pool.resetErr)

	_, exists := pool.index.get(stale.Hash)
	require.False(t, exists)
	_, exists = pool.index.get(queued.Hash)
	require.True(t, exists)

	issue7ARequireResetPromotion(t, pool, queued)
}

func TestIssue7A_BuildGuardRejectsShutdown(t *testing.T) {
	pool, _ := issue7ANewPool(t)
	pool.Close()

	called := false
	err := pool.WithReconciledPool(func() error {
		called = true

		return nil
	})

	require.Error(t, err)
	require.False(t, called)
}

func TestIssue7A_DemotionCounterConcurrentAccess(t *testing.T) {
	account := &account{}

	const workers = 4
	const iterations = 1000

	var wg sync.WaitGroup

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for iteration := 0; iteration < iterations; iteration++ {
				account.incrementDemotions()
				_ = account.Demotions()
			}
		}()
	}

	wg.Wait()
	require.Equal(t, uint64(workers*iterations), account.Demotions())

	// Exercise reset concurrently with reads and increments as well.
	wg.Add(3)

	go func() {
		defer wg.Done()

		for iteration := 0; iteration < iterations; iteration++ {
			account.resetDemotions()
		}
	}()

	go func() {
		defer wg.Done()

		for iteration := 0; iteration < iterations; iteration++ {
			account.incrementDemotions()
		}
	}()

	go func() {
		defer wg.Done()

		for iteration := 0; iteration < iterations; iteration++ {
			_ = account.Demotions()
		}
	}()

	wg.Wait()

	account.resetDemotions()
	require.Zero(t, account.Demotions())
}

/* FINISH ALPHA CHANGE 2204 -- test proposal pool recovery and reset exclusion -- */
