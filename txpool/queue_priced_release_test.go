/* START ALPHA CHANGE 3861 -- regress price-heap reference release and preserve shared pool ownership -- */

package txpool

import (
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func requirePricedQueueReleasedTail(t *testing.T, q *pricedQueue) {
	t.Helper()

	storage := q.queue.txs[:cap(q.queue.txs)]
	for index, tx := range storage {
		if index < len(q.queue.txs) {
			require.NotNil(t, tx, "missing live heap entry at slot %d", index)
		} else {
			require.Nil(t, tx, "retained transaction at discarded heap slot %d", index)
		}
	}
}

func TestPricedQueueRelease_PopOwnershipAndReuse(t *testing.T) {
	tests := []struct {
		name    string
		baseFee uint64
		size    int
	}{
		{
			name:    "empty",
			baseFee: 0,
			size:    0,
		},
		{
			name:    "singleton",
			baseFee: 100,
			size:    1,
		},
		{
			name:    "mixed_transactions_without_base_fee",
			baseFee: 0,
			size:    3,
		},
		{
			name:    "mixed_transactions_with_base_fee",
			baseFee: 100,
			size:    3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			low := queueReleaseTransaction(addr1, 2, txSlotSize)
			low.GasPrice = big.NewInt(120)

			middle := queueReleaseTransaction(addr2, 1, 2*txSlotSize)
			middle.GasPrice = big.NewInt(140)

			high := queueReleaseTransaction(addr3, 0, 3*txSlotSize)
			high.Type = types.DynamicFeeTx
			high.GasPrice = big.NewInt(0)
			high.GasFeeCap = big.NewInt(200)
			high.GasTipCap = big.NewInt(60)

			initial := []*types.Transaction{low, high, middle}
			expected := []*types.Transaction{high, middle, low}

			switch test.size {
			case 0:
				initial = nil
				expected = nil
			case 1:
				initial = []*types.Transaction{high}
				expected = []*types.Transaction{high}
			}

			payloads := make([][]byte, len(expected))
			for index, tx := range expected {
				payloads[index] = append([]byte(nil), tx.Input...)
			}

			q := newPricesQueue(test.baseFee, initial)
			originalCapacity := cap(q.queue.txs)

			for index, tx := range expected {
				actual := q.pop()

				require.Same(t, tx, actual)
				require.Equal(t, payloads[index], actual.Input)
				require.Equal(t, len(expected)-index-1, q.queue.Len())
				requirePricedQueueReleasedTail(t, q)
			}

			require.Zero(t, q.length())
			require.Nil(t, q.pop())
			requirePricedQueueReleasedTail(t, q)

			// Reuse the emptied heap without replacing it. Previously
			// returned transaction objects and payloads must remain intact.
			replacement := queueReleaseTransaction(addr4, 9, 1)
			q.push(replacement)

			if originalCapacity > 0 {
				require.Equal(t, originalCapacity, cap(q.queue.txs))
			}

			require.Same(t, replacement, q.pop())
			require.Zero(t, q.length())
			require.Nil(t, q.pop())
			requirePricedQueueReleasedTail(t, q)

			for index, tx := range expected {
				require.Equal(t, payloads[index], tx.Input)
			}
		})
	}
}

func TestPricedQueueRelease_SelectionAndFinalizedCleanup(t *testing.T) {
	// The fixture starts no pool workers. Admission, promotion,
	// selection and finalized-nonce cleanup run synchronously.
	pool := queueReleasePool(t)

	tx := queueReleaseTransaction(addr1, 0, 3*txSlotSize)
	payload := append([]byte(nil), tx.Input...)

	require.NoError(t, pool.addTx(local, tx))
	pool.handlePromoteRequest(promoteRequest{account: addr1})

	a := pool.accounts.get(addr1)
	require.NotNil(t, a)

	pool.Prepare()
	selection := pool.executables
	require.Len(t, selection.queue.txs, 1)

	selected := pool.Peek()
	require.Same(t, tx, selected)
	require.Equal(t, payload, selected.Input)
	requirePricedQueueReleasedTail(t, selection)

	pool.Pop(selected)
	require.Nil(t, pool.Peek())

	// Popping the proposal heap must not consume the transaction's
	// independent ownership in the shared pool.
	func() {
		require.True(t, a.lockActive())
		defer a.unlockActive()

		require.Equal(t, uint64(1), a.promoted.length())
		require.Same(t, tx, a.promoted.peek())
		require.Same(t, tx, a.nonceToTx.mapping[0])
		require.Equal(t, uint64(1), a.getNonce())
	}()

	stored, found := pool.index.get(tx.Hash)
	require.True(t, found)
	require.Same(t, tx, stored)
	require.Equal(t, slotsRequired(tx), pool.gauge.read())
	require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))

	// Model successful finalized-nonce cleanup while retaining the
	// same old executable heap. No subsequent Prepare releases it.
	pool.resetAccounts(map[types.Address]uint64{
		addr1: 1,
	})

	require.Same(t, selection, pool.executables)
	require.Zero(t, selection.length())
	requirePricedQueueReleasedTail(t, selection)

	_, found = pool.index.get(tx.Hash)
	require.False(t, found)
	require.False(t, pool.index.hasSender(addr1))
	require.Nil(t, pool.accounts.get(addr1))
	require.Zero(t, pool.gauge.read())
	require.Zero(t, atomic.LoadInt64(&pool.pending))
	require.Zero(t, pool.Length())

	// Clearing references must never erase the returned transaction.
	require.Equal(t, payload, selected.Input)
}

/* FINISH ALPHA CHANGE 3861 -- regress price-heap reference release and preserve shared pool ownership -- */
