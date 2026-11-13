/* START ALPHA CHANGE 3854 -- regress account queue storage release and preserved pruning accounting -- */

package txpool

import (
	"bytes"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func queueReleaseTransaction(
	address types.Address,
	nonce uint64,
	payloadSize int,
) *types.Transaction {
	to := addr5

	return &types.Transaction{
		From:     address,
		To:       &to,
		Nonce:    nonce,
		Value:    big.NewInt(1),
		GasPrice: new(big.Int).SetUint64(defaultPriceLimit),
		Gas:      validGasLimit,
		Input:    bytes.Repeat([]byte{0x7a}, payloadSize),
	}
}

func queueReleasePool(t *testing.T) *TxPool {
	t.Helper()

	pool, err := newTestPool()
	require.NoError(t, err)

	pool.SetSigner(&mockSigner{})
	pool.SetSealing(true)

	// Notifications remain buffered. Promotion and pruning are invoked
	// synchronously below; no background queue worker is started.
	pool.promoteReqCh = make(chan promoteRequest, 16)

	t.Cleanup(pool.Close)

	return pool
}

func TestQueueRelease_ClearOwnershipAndReuse(t *testing.T) {
	for _, name := range []string{
		"nil_queue",
		"allocated_empty_queue",
		"populated_queue",
	} {
		t.Run(name, func(t *testing.T) {
			q := newAccountQueue()
			q.lock(true)
			defer q.unlock()

			switch name {
			case "nil_queue":
				q.queue = nil
			case "allocated_empty_queue":
				q.queue = make(minNonceQueue, 0, 8)
			case "populated_queue":
				q.push(queueReleaseTransaction(addr1, 0, 3*txSlotSize))
				q.push(queueReleaseTransaction(addr1, 1, txSlotSize))
			}

			expected := append([]*types.Transaction(nil), q.queue...)
			payloads := make([][]byte, len(expected))
			for i, tx := range expected {
				payloads[i] = append([]byte(nil), tx.Input...)
			}

			removed := q.clear()

			require.Nil(t, q.queue)
			require.Zero(t, q.length())
			require.Zero(t, cap(q.queue))
			require.Nil(t, q.peek())
			require.Nil(t, q.pop())

			require.Len(t, removed, len(expected))
			for i, tx := range expected {
				require.Same(t, tx, removed[i])
				require.Equal(t, payloads[i], removed[i].Input)
			}

			// New queue activity must not overwrite the transaction list
			// returned to the preceding caller.
			replacement := queueReleaseTransaction(addr1, 9, 1)
			q.push(replacement)

			require.Equal(t, uint64(1), q.length())
			require.Same(t, replacement, q.peek())
			require.Same(t, replacement, q.pop())
			require.Zero(t, q.length())

			for i, tx := range expected {
				require.Same(t, tx, removed[i])
				require.Equal(t, payloads[i], removed[i].Input)
			}

			// Clearing an already emptied queue remains safe.
			require.Empty(t, q.clear())
			require.Nil(t, q.queue)
			require.Zero(t, cap(q.queue))
		})
	}
}

func queueReleaseRequirePrunedAccount(
	t *testing.T,
	pool *TxPool,
	a *account,
	head *types.Transaction,
	removed ...*types.Transaction,
) {
	t.Helper()

	require.Same(t, a, pool.accounts.get(head.From))
	require.True(t, a.lockActive())
	defer a.unlockActive()

	require.Nil(t, a.enqueued.queue)
	require.Zero(t, a.enqueued.length())
	require.Zero(t, cap(a.enqueued.queue))

	require.Equal(t, uint64(1), a.promoted.length())
	require.Same(t, head, a.promoted.peek())
	require.Equal(t, uint64(1), a.getNonce())

	require.Len(t, a.nonceToTx.mapping, 1)
	require.Same(t, head, a.nonceToTx.mapping[head.Nonce])

	stored, found := pool.index.get(head.Hash)
	require.True(t, found)
	require.Same(t, head, stored)

	for _, tx := range removed {
		require.NotContains(t, a.nonceToTx.mapping, tx.Nonce)

		_, found := pool.index.get(tx.Hash)
		require.False(t, found)
	}

	pool.index.RLock()
	indexed := len(pool.index.all)
	senderIndexed := len(pool.index.bySender[head.From])
	pool.index.RUnlock()

	require.Equal(t, 1, indexed)
	require.Equal(t, 1, senderIndexed)
	require.Equal(t, slotsRequired(head), pool.gauge.read())
	require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))
}

func TestQueueRelease_PruningRetainsPromotedPrefix(t *testing.T) {
	pool := queueReleasePool(t)

	head := queueReleaseTransaction(addr1, 0, 0)
	require.NoError(t, pool.addTx(local, head))
	pool.handlePromoteRequest(promoteRequest{account: addr1})

	a := pool.accounts.get(addr1)
	require.NotNil(t, a)
	require.Equal(t, uint64(1), a.getNonce())

	// Nonce one is missing. These future transactions form a genuine gap
	// while nonce zero remains promoted and keeps the account active.
	first := queueReleaseTransaction(addr1, 2, 3*txSlotSize)
	second := queueReleaseTransaction(addr1, 3, 3*txSlotSize)

	require.NoError(t, pool.addTx(local, first))
	require.NoError(t, pool.addTx(local, second))
	require.Equal(t, uint64(4), slotsRequired(first))
	require.Equal(t, uint64(4), slotsRequired(second))

	func() {
		require.True(t, a.lockActive())
		defer a.unlockActive()

		require.Equal(t, uint64(2), a.enqueued.length())
		require.Greater(t, cap(a.enqueued.queue), 0)
		require.Equal(t, uint64(1), a.promoted.length())
		require.Len(t, a.nonceToTx.mapping, 3)
	}()

	require.Equal(
		t,
		slotsRequired(head, first, second),
		pool.gauge.read(),
	)
	require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))

	pool.pruneAccountsWithNonceHoles()

	queueReleaseRequirePrunedAccount(t, pool, a, head, first, second)

	// An additional pruning pass must not subtract the same slots again.
	pool.pruneAccountsWithNonceHoles()
	queueReleaseRequirePrunedAccount(t, pool, a, head, first, second)

	// Reuse the same active account for another gapped queue and verify
	// that clearing it again releases its storage and preserves the head.
	later := queueReleaseTransaction(addr1, 4, 2*txSlotSize)
	require.NoError(t, pool.addTx(local, later))
	require.Equal(t, slotsRequired(head, later), pool.gauge.read())

	pool.pruneAccountsWithNonceHoles()
	queueReleaseRequirePrunedAccount(
		t,
		pool,
		a,
		head,
		first,
		second,
		later,
	)

	// The expected nonce remains admissible and promotable after clearing.
	next := queueReleaseTransaction(addr1, 1, 0)
	require.NoError(t, pool.addTx(local, next))
	pool.handlePromoteRequest(promoteRequest{account: addr1})

	require.Same(t, a, pool.accounts.get(addr1))

	func() {
		require.True(t, a.lockActive())
		defer a.unlockActive()

		require.Zero(t, a.enqueued.length())
		require.Equal(t, uint64(2), a.promoted.length())
		require.Same(t, head, a.promoted.peek())
		require.Equal(t, uint64(2), a.getNonce())
		require.Len(t, a.nonceToTx.mapping, 2)
		require.Same(t, head, a.nonceToTx.mapping[0])
		require.Same(t, next, a.nonceToTx.mapping[1])
	}()

	stored, found := pool.index.get(next.Hash)
	require.True(t, found)
	require.Same(t, next, stored)
	require.Equal(t, slotsRequired(head, next), pool.gauge.read())
	require.Equal(t, int64(2), atomic.LoadInt64(&pool.pending))
	require.Equal(t, uint64(2), pool.Length())
}

func TestQueueRelease_FullAccountDropPreservesAccounting(t *testing.T) {
	pool := queueReleasePool(t)

	head := queueReleaseTransaction(addr1, 0, 0)
	require.NoError(t, pool.addTx(local, head))
	pool.handlePromoteRequest(promoteRequest{account: addr1})

	future := queueReleaseTransaction(addr1, 2, 3*txSlotSize)
	require.NoError(t, pool.addTx(local, future))

	a := pool.accounts.get(addr1)
	require.NotNil(t, a)
	require.Equal(t, slotsRequired(head, future), pool.gauge.read())
	require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))

	// Exercise the other production clear() caller: maintenance removal
	// of both queues, rather than proposal suffix removal.
	pool.dropAccount(a, 0, head, false)

	require.Nil(t, pool.accounts.get(addr1))
	require.False(t, pool.index.hasSender(addr1))

	for _, tx := range []*types.Transaction{head, future} {
		_, found := pool.index.get(tx.Hash)
		require.False(t, found)
	}

	func() {
		a.promoted.lock(false)
		a.enqueued.lock(false)
		a.nonceToTx.lock()
		defer func() {
			a.nonceToTx.unlock()
			a.enqueued.unlock()
			a.promoted.unlock()
		}()

		require.True(t, a.retired)
		require.Nil(t, a.promoted.queue)
		require.Zero(t, cap(a.promoted.queue))
		require.Nil(t, a.enqueued.queue)
		require.Zero(t, cap(a.enqueued.queue))
		require.Empty(t, a.nonceToTx.mapping)
	}()

	require.Zero(t, pool.gauge.read())
	require.Zero(t, atomic.LoadInt64(&pool.pending))
	require.Zero(t, pool.Length())

	// A stale cleanup request must not repeat removal accounting.
	pool.dropAccount(a, 0, head, false)
	require.Zero(t, pool.gauge.read())
	require.Zero(t, atomic.LoadInt64(&pool.pending))
}

/* FINISH ALPHA CHANGE 3854 -- regress account queue storage release and preserved pruning accounting -- */
