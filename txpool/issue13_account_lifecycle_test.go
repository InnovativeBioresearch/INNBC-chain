// START ALPHA CHANGE 2329 -- test bounded unused-account lifetime and preserved pool behavior --
package txpool

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

// Shared by the migrated queue/reset assertions.
// An empty account may remain only while indexed transactions still need it.
func requireIssue13Queues(
	t *testing.T,
	pool *TxPool,
	address types.Address,
	enqueued uint64,
	promoted uint64,
) {
	t.Helper()

	a := pool.accounts.get(address)
	if a == nil {
		require.Zero(t, enqueued)
		require.Zero(t, promoted)
		require.False(t, pool.index.hasSender(address))

		return
	}

	a.promoted.lock(false)
	a.enqueued.lock(false)
	a.nonceToTx.lock()

	defer func() {
		a.nonceToTx.unlock()
		a.enqueued.unlock()
		a.promoted.unlock()
	}()

	require.Equal(t, enqueued, a.enqueued.length())
	require.Equal(t, promoted, a.promoted.length())
	require.Len(t, a.nonceToTx.mapping, int(enqueued+promoted))

	if enqueued == 0 && promoted == 0 && !pool.index.hasSender(address) {
		require.Fail(t, "unused account was not reclaimed")
	}
}

func issue13Address(number uint64) types.Address {
	var address types.Address
	binary.BigEndian.PutUint64(address[12:], number)

	return address
}

func issue13Pool(t *testing.T, slots uint64) *TxPool {
	t.Helper()

	pool, err := newTestPoolWithSlots(slots)
	require.NoError(t, err)
	pool.SetSigner(&mockSigner{})
	t.Cleanup(pool.Close)

	return pool
}

func TestIssue13_IndexOwnershipUsesStoredSender(t *testing.T) {
	var index lookupMap

	first := &types.Transaction{
		Hash: types.StringToHash("0x1301"),
		From: addr1,
	}
	second := &types.Transaction{
		Hash: types.StringToHash("0x1302"),
		From: addr1,
	}

	require.True(t, index.add(first))
	require.True(t, index.add(second))
	require.False(t, index.add(first))
	require.True(t, index.hasSender(addr1))

	// Model removal using a block transaction whose From is unset.
	blockCopy := first.Copy()
	blockCopy.From = types.ZeroAddress

	index.remove(blockCopy, blockCopy)
	require.True(t, index.hasSender(addr1))

	// START ALPHA CHANGE 2338 -- verify the remaining indexed transaction belongs to its sender --
	index.RLock()
	remainingCount := len(index.bySender[addr1])
	remainingTransaction := index.bySender[addr1][second.Hash]
	index.RUnlock()

	require.Equal(t, 1, remainingCount)
	require.Same(t, second, remainingTransaction)
	// FINISH ALPHA CHANGE 2338 -- verify the remaining indexed transaction belongs to its sender --

	index.remove(second, second)
	require.False(t, index.hasSender(addr1))
	require.False(t, index.hasSender(types.ZeroAddress))

	index.RLock()
	require.Empty(t, index.all)
	require.Empty(t, index.bySender)
	index.RUnlock()
}

func TestIssue13_RejectedAdmissionsDoNotAccumulateAccounts(t *testing.T) {
	pool := issue13Pool(t, 1)
	pool.gauge.increase(1)

	const attempts = 256

	for i := range attempts {
		address := issue13Address(uint64(i + 1))
		tx := newTx(address, 0, 1)
		require.NotNil(t, tx)

		require.ErrorIs(t, pool.addTx(local, tx), ErrTxPoolOverflow)
		require.Nil(t, pool.accounts.get(address))
	}

	require.Zero(t, atomic.LoadUint64(&pool.accounts.count))
	require.Equal(t, uint64(1), pool.gauge.read())
	require.Empty(t, pool.index.all)
	require.Empty(t, pool.index.bySender)
}

func TestIssue13_AccountChurnIsReclaimed(t *testing.T) {
	for _, operation := range []string{"drop", "nonce_hole_pruning"} {
		t.Run(operation, func(t *testing.T) {
			pool := issue13Pool(t, 8)

			for i := range 128 {
				address := issue13Address(uint64(i + 1))
				tx := newTx(address, 1, 1)
				require.NotNil(t, tx)
				require.NoError(t, pool.addTx(local, tx))

				requireIssue13Queues(t, pool, address, 1, 0)

				switch operation {
				case "drop":
					require.NoError(t, pool.WithReconciledPool(func() error {
						pool.Drop(tx)

						return nil
					}))
				case "nonce_hole_pruning":
					pool.pruneAccountsWithNonceHoles()
				}

				require.Nil(t, pool.accounts.get(address))
				require.False(t, pool.index.hasSender(address))
				require.Zero(t, pool.gauge.read())
				require.Zero(t, atomic.LoadUint64(&pool.accounts.count))
			}

			require.Empty(t, pool.index.all)
			require.Empty(t, pool.index.bySender)
		})
	}
}

func TestIssue13_PopPreservesAccountUntilSuccessfulReset(t *testing.T) {
	for _, failure := range []string{"none", "nonce", "fee"} {
		t.Run(failure, func(t *testing.T) {
			pool, store := issue7ANewPool(t)
			t.Cleanup(pool.Close)
			pool.SetSealing(true)
			pool.promoteReqCh = make(chan promoteRequest, 8)

			tx := newTx(addr1, 0, 1)
			require.NotNil(t, tx)
			require.NoError(t, pool.addTx(local, tx))
			pool.handlePromoteRequest(promoteRequest{account: addr1})

			original := pool.accounts.get(addr1)
			require.NotNil(t, original)

			require.NoError(t, pool.WithReconciledPool(func() error {
				pool.Prepare()
				selected := pool.Peek()
				if selected != tx {
					return errors.New("unexpected selected transaction")
				}

				pool.Pop(selected)

				return nil
			}))

			// START ALPHA CHANGE 2352 -- retain queued ownership after successful proposal selection --
			require.Same(t, original, pool.accounts.get(addr1))
			require.True(t, pool.index.hasSender(addr1))
			requireIssue13Queues(t, pool, addr1, 0, 1)
			require.Equal(t, uint64(1), pool.gauge.read())

			nonce, err := pool.GetNonce(addr1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), nonce)
			require.Zero(t, store.nonce)

			// A distinct replacement without a price bump is rejected
			// without retiring the original transaction's account.
			replacement := tx.Copy()
			replacement.Value.SetUint64(2)
			require.ErrorIs(t,
				pool.addTx(local, replacement),
				ErrReplacementUnderpriced)
			require.Same(t, original, pool.accounts.get(addr1))
			// FINISH ALPHA CHANGE 2352 -- retain queued ownership after successful proposal selection --

			store.nonce = 1
			store.block = &types.Block{
				Header:       store.Header(),
				Transactions: []*types.Transaction{tx},
			}

			cause := errors.New("injected reset failure")

			switch failure {
			case "nonce":
				store.nonceErr = cause
			case "fee":
				store.calculateBaseFeeFn = func(*types.Header) (uint64, error) {
					return 0, state.WrapLocalStateFailure("test fee refresh", cause)
				}
			}

			pool.ResetWithHeaders(store.block.Header)

			if failure != "none" {
				require.ErrorIs(t, pool.resetErr, cause)
				require.Same(t, original, pool.accounts.get(addr1))
				require.True(t, pool.index.hasSender(addr1))

				store.nonceErr = nil
				store.calculateBaseFeeFn = nil
				issue7AResetRetryDue(pool)

				require.NoError(t, pool.WithReconciledPool(func() error {
					return nil
				}))
			}

			require.NoError(t, pool.resetErr)
			require.Nil(t, pool.accounts.get(addr1))
			require.False(t, pool.index.hasSender(addr1))
			require.Zero(t, atomic.LoadUint64(&pool.accounts.count))

			nonce, err = pool.GetNonce(addr1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), nonce)

			// Retirement must preserve fallback error propagation.
			store.nonceErr = cause
			nonce, err = pool.GetNonce(addr1)
			require.Zero(t, nonce)
			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, state.ErrLocalStateFailure)
			require.Nil(t, pool.accounts.get(addr1))
			store.nonceErr = nil

			// A later valid transaction recreates the account normally.
			require.NoError(t, pool.addTx(local, newTx(addr1, 1, 1)))
			recreated := pool.accounts.get(addr1)
			require.NotNil(t, recreated)
			require.NotSame(t, original, recreated)
			requireIssue13Queues(t, pool, addr1, 1, 0)
		})
	}
}

func TestIssue13_DelayedPromotionAfterRetirement(t *testing.T) {
	pool := issue13Pool(t, 8)
	pool.promoteReqCh = make(chan promoteRequest, 8)

	first := newTx(addr1, 0, 1)
	require.NoError(t, pool.addTx(local, first))
	original := pool.accounts.get(addr1)

	require.NoError(t, pool.WithReconciledPool(func() error {
		pool.Drop(first)

		return nil
	}))
	require.Nil(t, pool.accounts.get(addr1))

	request := promoteRequest{account: addr1}
	require.NotPanics(t, func() {
		pool.handlePromoteRequest(request)
	})
	require.Nil(t, pool.accounts.get(addr1))

	second := newTx(addr1, 0, 1)
	require.NoError(t, pool.addTx(local, second))
	require.NotSame(t, original, pool.accounts.get(addr1))

	// A late request may now find the new account. Normal promotion
	// checks must promote its current transaction exactly once.
	pool.handlePromoteRequest(request)
	pool.handlePromoteRequest(request)

	requireIssue13Queues(t, pool, addr1, 0, 1)
	require.Equal(t, uint64(1), pool.gauge.read())
	require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))

	nonce, err := pool.GetNonce(addr1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), nonce)
}

func TestIssue13_RetiredPointerCannotReceiveAdmission(t *testing.T) {
	pool := issue13Pool(t, 8)
	original := pool.accounts.initOnce(addr1, 0)
	require.True(t, original.lockActive())

	started := make(chan struct{})
	result := make(chan bool, 1)

	go func() {
		close(started)

		locked := original.lockActive()
		if locked {
			original.unlockActive()
		}
		result <- locked
	}()

	<-started

	pool.retireIdleAccountLocked(addr1, original)
	original.unlockActive()

	select {
	case locked := <-result:
		require.False(t, locked)
	case <-time.After(5 * time.Second):
		t.Fatal("waiter did not observe retired account")
	}

	require.NoError(t, pool.lockReconciled())

	recreated, err := pool.lockAccountForAdmission(addr1)
	if err != nil {
		pool.resetMu.RUnlock()
		t.Fatal(err)
	}

	same := recreated == original
	pool.retireIdleAccountLocked(addr1, recreated)
	recreated.unlockActive()
	pool.resetMu.RUnlock()

	require.False(t, same)
	require.Nil(t, pool.accounts.get(addr1))
	require.Zero(t, atomic.LoadUint64(&pool.accounts.count))
}

func TestIssue13_ConcurrentAdmissionPruningAndQueries(t *testing.T) {
	pool := issue13Pool(t, 64)

	const (
		writers    = 4
		iterations = 128
	)

	var wg sync.WaitGroup
	failures := make(chan error, writers+1)

	for range writers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range iterations {
				tx := newTx(addr1, 1, 1)
				if tx == nil {
					failures <- errors.New("failed to create test transaction")

					return
				}

				err := pool.addTx(local, tx)
				if err != nil &&
					!errors.Is(err, ErrAlreadyKnown) &&
					!errors.Is(err, ErrReplacementUnderpriced) {
					failures <- err

					return
				}

				pool.pruneAccountsWithNonceHoles()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for range iterations * writers {
			pool.handlePromoteRequest(promoteRequest{account: addr1})

			nonce, err := pool.GetNonce(addr1)
			if err != nil {
				failures <- err

				return
			}
			if nonce != 0 {
				failures <- fmt.Errorf("unexpected pending nonce: %d", nonce)

				return
			}

			_ = pool.Length()
			_, _ = pool.GetTxs(true)
		}
	}()

	wg.Wait()
	close(failures)

	for err := range failures {
		require.NoError(t, err)
	}

	pool.pruneAccountsWithNonceHoles()

	require.Nil(t, pool.accounts.get(addr1))
	require.False(t, pool.index.hasSender(addr1))
	require.Zero(t, pool.gauge.read())
	require.Zero(t, atomic.LoadUint64(&pool.accounts.count))
	require.Zero(t, atomic.LoadInt64(&pool.pending))
}

// FINISH ALPHA CHANGE 2329 -- test bounded unused-account lifetime and preserved pool behavior --
