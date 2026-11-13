// START ALPHA CHANGE 2332 -- verify query slices survive subsequent queue removal --
package txpool

import (
	"errors"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestIssue13_QuerySlicesSurviveQueueRemoval(t *testing.T) {
	for _, queue := range []string{"promoted", "enqueued"} {
		t.Run(queue, func(t *testing.T) {
			pool := issue13Pool(t, 8)
			pool.promoteReqCh = make(chan promoteRequest, 8)

			nonce := uint64(0)
			if queue == "enqueued" {
				nonce = 1
			}

			tx := newTx(addr1, nonce, 1)
			require.NotNil(t, tx)
			require.NoError(t, pool.addTx(local, tx))

			if queue == "promoted" {
				pool.handlePromoteRequest(promoteRequest{account: addr1})
			}

			pending, queued := pool.GetTxs(true)

			var snapshot []*types.Transaction
			if queue == "promoted" {
				snapshot = pending[addr1]
			} else {
				snapshot = queued[addr1]
			}

			require.Len(t, snapshot, 1)
			require.Same(t, tx, snapshot[0])

			switch queue {
			case "promoted":
				require.NoError(t, pool.WithReconciledPool(func() error {
					pool.Prepare()
					selected := pool.Peek()
					if selected != tx {
						return errors.New("unexpected selected transaction")
					}

					pool.Pop(selected)

					return nil
				}))

				// START ALPHA CHANGE 2353 -- remove the queue through confirmed-nonce cleanup --
				// Selection retains shared ownership. Model the later
				// successful reset that consumes this nonce.
				require.NotNil(t, pool.accounts.get(addr1))
				require.True(t, pool.index.hasSender(addr1))

				pool.resetMu.Lock()
				pool.resetAccounts(map[types.Address]uint64{addr1: 1})
				pool.resetMu.Unlock()

				require.Nil(t, pool.accounts.get(addr1))
				require.False(t, pool.index.hasSender(addr1))
				// FINISH ALPHA CHANGE 2353 -- remove the queue through confirmed-nonce cleanup --

			case "enqueued":
				pool.pruneAccountsWithNonceHoles()

				require.Nil(t, pool.accounts.get(addr1))
				require.False(t, pool.index.hasSender(addr1))
			}

			// Heap removal clears the original queue element. A query
			// result must retain its own slice storage.
			require.Len(t, snapshot, 1)
			require.Same(t, tx, snapshot[0])

			currentPending, currentQueued := pool.GetTxs(true)
			require.Empty(t, currentPending)
			require.Empty(t, currentQueued)
		})
	}
}

// FINISH ALPHA CHANGE 2332 -- verify query slices survive subsequent queue removal --
