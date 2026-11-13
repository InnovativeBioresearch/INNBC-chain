/* START ALPHA CHANGE 3875 -- regress future transaction limits across delayed promotion -- */

package txpool

import (
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestEnqueuedLimit_DelayedPromotion(t *testing.T) {
	const limit uint64 = 128

	// This existing fixture starts no promotion worker. Notifications are
	// buffered, and real promotion is invoked explicitly below.
	pool := queueReleasePool(t)

	assertState := func(enqueued, promoted, nextNonce uint64) {
		t.Helper()

		a := pool.accounts.get(addr1)
		require.NotNil(t, a)

		func() {
			require.True(t, a.lockActive())
			defer a.unlockActive()

			require.Equal(t, limit, a.maxEnqueued)
			require.Equal(t, enqueued, a.enqueued.length())
			require.Equal(t, promoted, a.promoted.length())
			require.Equal(t, nextNonce, a.getNonce())
			require.Len(t, a.nonceToTx.mapping, int(enqueued+promoted))
		}()

		// Every transaction in this fixture occupies one slot.
		require.Equal(t, enqueued+promoted, pool.gauge.read())
		require.Equal(t, int64(promoted), atomic.LoadInt64(&pool.pending))
		require.Equal(t, promoted, pool.Length())
	}

	rejectFuture := func(nonce uint64) {
		t.Helper()

		tx := queueReleaseTransaction(addr1, nonce, 0)
		slotsBefore := pool.gauge.read()
		pendingBefore := atomic.LoadInt64(&pool.pending)

		require.ErrorIs(
			t,
			pool.addTx(local, tx),
			ErrMaxEnqueuedLimitReached,
		)

		_, found := pool.GetPendingTx(tx.Hash)
		require.False(t, found)
		require.Equal(t, slotsBefore, pool.gauge.read())
		require.Equal(t, pendingBefore, atomic.LoadInt64(&pool.pending))
	}

	// Fill the default future queue with nonces 2 through 129.
	// Both nonce zero and nonce one are initially missing.
	var firstFuture *types.Transaction

	for offset := range limit {
		tx := queueReleaseTransaction(addr1, offset+2, 0)
		require.NoError(t, pool.addTx(local, tx))

		if offset == 0 {
			firstFuture = tx
		}
	}

	assertState(limit, 0, 0)
	rejectFuture(limit + 2)

	// The expected nonce must still be accepted when the queue is full.
	head := queueReleaseTransaction(addr1, 0, 0)
	require.NoError(t, pool.addTx(local, head))
	assertState(limit+1, 0, 0)

	// Promotion has deliberately not run. The old equality guard accepts
	// this transaction because the queue is already above its limit.
	rejectFuture(limit + 2)
	assertState(limit+1, 0, 0)

	// A valid replacement must remain possible while above the limit.
	replacement := queueReleaseTransaction(addr1, 2, 0)
	replacement.GasPrice = big.NewInt(2)
	require.NoError(t, pool.addTx(local, replacement))

	_, found := pool.GetPendingTx(firstFuture.Hash)
	require.False(t, found)

	stored, found := pool.GetPendingTx(replacement.Hash)
	require.True(t, found)
	require.Same(t, replacement, stored)
	assertState(limit+1, 0, 0)

	// Promote nonce zero. Missing nonce one leaves all 128 future
	// transactions enqueued, so further future admission remains blocked.
	pool.handlePromoteRequest(promoteRequest{account: addr1})
	assertState(limit, 1, 1)
	rejectFuture(limit + 2)
	assertState(limit, 1, 1)

	// Replacing a promoted transaction also remains allowed while the
	// future queue is full.
	promotedReplacement := queueReleaseTransaction(addr1, 0, 0)
	promotedReplacement.GasPrice = big.NewInt(2)
	require.NoError(t, pool.addTx(local, promotedReplacement))

	_, found = pool.GetPendingTx(head.Hash)
	require.False(t, found)

	stored, found = pool.GetPendingTx(promotedReplacement.Hash)
	require.True(t, found)
	require.Same(t, promotedReplacement, stored)
	assertState(limit, 1, 1)

	// Filling the missing nonce must remain accepted and drain the
	// consecutive sequence through nonce 129.
	missing := queueReleaseTransaction(addr1, 1, 0)
	require.NoError(t, pool.addTx(local, missing))
	assertState(limit+1, 1, 1)

	pool.handlePromoteRequest(promoteRequest{account: addr1})
	assertState(0, limit+2, limit+2)

	// Queue capacity becomes usable again after promotion.
	future := queueReleaseTransaction(addr1, limit+3, 0)
	require.NoError(t, pool.addTx(local, future))
	assertState(1, limit+2, limit+2)

	next := queueReleaseTransaction(addr1, limit+2, 0)
	require.NoError(t, pool.addTx(local, next))

	pool.handlePromoteRequest(promoteRequest{account: addr1})
	assertState(0, limit+4, limit+4)
}

/* FINISH ALPHA CHANGE 3875 -- regress future transaction limits across delayed promotion -- */
