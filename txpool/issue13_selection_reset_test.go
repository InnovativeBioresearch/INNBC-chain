// START ALPHA CHANGE 2339 -- test replacement accounting and confirmed-nonce ownership cleanup --
package txpool

import (
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func issue13SelectionTx(nonce, slots, price uint64) *types.Transaction {
	inputSize := 1
	if slots > 1 {
		inputSize = int(txSlotSize * (slots - 1))
	}

	input := make([]byte, inputSize)
	input[0] = 1
	to := addr5

	return &types.Transaction{
		From:     addr1,
		To:       &to,
		Nonce:    nonce,
		Value:    big.NewInt(1),
		GasPrice: new(big.Int).SetUint64(price),
		Gas:      validGasLimit,
		Input:    input,
	}
}

func TestIssue13_PopConcurrentReplacementKeepsOwnership(t *testing.T) {
	cases := []struct {
		name             string
		selectedSlots    uint64
		replacementSlots uint64
	}{
		{
			name:             "larger replacement",
			selectedSlots:    1,
			replacementSlots: 2,
		},
		{
			name:             "smaller replacement",
			selectedSlots:    2,
			replacementSlots: 1,
		},
		{
			name:             "equal size replacement",
			selectedSlots:    1,
			replacementSlots: 1,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			pool, store := issue7ANewPool(t)
			t.Cleanup(pool.Close)
			pool.SetSealing(true)
			pool.promoteReqCh = make(chan promoteRequest, 16)

			selected := issue13SelectionTx(0, testCase.selectedSlots, 10)
			following := issue13SelectionTx(1, 1, 10)
			replacement := issue13SelectionTx(0, testCase.replacementSlots, 20)

			require.NoError(t, pool.AddTx(selected))
			pool.handlePromoteRequest(promoteRequest{account: addr1})

			require.NoError(t, pool.AddTx(following))
			pool.handlePromoteRequest(promoteRequest{account: addr1})

			require.Equal(t, testCase.selectedSlots, slotsRequired(selected))
			require.Equal(t, uint64(1), slotsRequired(following))
			require.Equal(t, testCase.selectedSlots+1, pool.gauge.read())
			require.Equal(t, int64(2), atomic.LoadInt64(&pool.pending))

			originalAccount := pool.accounts.get(addr1)
			require.NotNil(t, originalAccount)

			require.NoError(t, pool.WithReconciledPool(func() error {
				pool.Prepare()

				if got := pool.Peek(); got != selected {
					return errors.New("unexpected first selected transaction")
				}

				// Admission runs on another goroutine while proposal
				// selection holds the reconciliation read guard.
				// No pool worker or reset runs in this fixture.
				admitted := make(chan error, 1)
				go func() {
					admitted <- pool.AddTx(replacement)
				}()

				select {
				case err := <-admitted:
					if err != nil {
						return fmt.Errorf("admit replacement: %w", err)
					}
				case <-time.After(5 * time.Second):
					return errors.New("replacement admission did not finish")
				}

				if got := pool.gauge.read(); got != testCase.replacementSlots+1 {
					return fmt.Errorf(
						"replacement gauge: got %d, want %d",
						got,
						testCase.replacementSlots+1,
					)
				}

				// Execution completed for the original selection,
				// although its queue entry has now been replaced.
				pool.Pop(selected)

				// A stale duplicate Pop must leave nonce 1 untouched.
				pool.Pop(selected)

				if got := pool.Peek(); got != following {
					return errors.New("next transaction was lost or changed")
				}

				return nil
			}))

			// START ALPHA CHANGE 2354 -- retain the current replacement and its capacity during selection --
			require.Equal(t, testCase.replacementSlots, slotsRequired(replacement))
			require.Equal(t, testCase.replacementSlots+1, pool.gauge.read())
			require.Equal(t, int64(2), atomic.LoadInt64(&pool.pending))
			requireIssue13Queues(t, pool, addr1, 0, 2)

			_, exists := pool.index.get(selected.Hash)
			require.False(t, exists)

			indexed, exists := pool.index.get(replacement.Hash)
			require.True(t, exists)
			require.Same(t, replacement, indexed)
			// FINISH ALPHA CHANGE 2354 -- retain the current replacement and its capacity during selection --

			indexed, exists = pool.index.get(following.Hash)
			require.True(t, exists)
			require.Same(t, following, indexed)
			require.Same(t, originalAccount, pool.accounts.get(addr1))

			nonce, err := pool.GetNonce(addr1)
			require.NoError(t, err)
			require.Equal(t, uint64(2), nonce)

			// Another block finalizes the replacement instead of the
			// transaction this node executed for its own candidate.
			// This must work during an ordinary, healthy reset.
			require.NoError(t, pool.resetErr)
			store.nonce = 1
			store.block = &types.Block{
				Header:       store.Header(),
				Transactions: []*types.Transaction{replacement},
			}
			pool.ResetWithHeaders(store.block.Header)

			require.NoError(t, pool.resetErr)
			_, exists = pool.index.get(selected.Hash)
			require.False(t, exists)

			indexed, exists = pool.index.get(following.Hash)
			require.True(t, exists)
			require.Same(t, following, indexed)
			require.Same(t, originalAccount, pool.accounts.get(addr1))
			requireIssue13Queues(t, pool, addr1, 0, 1)
			require.Equal(t, uint64(1), pool.gauge.read())
			require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))

			// Finalizing the remaining transaction performs its one
			// queue decrement and permits account retirement.
			store.nonce = 2
			store.block = &types.Block{
				Header:       store.Header(),
				Transactions: []*types.Transaction{following},
			}
			pool.ResetWithHeaders(store.block.Header)

			require.NoError(t, pool.resetErr)
			require.Nil(t, pool.accounts.get(addr1))
			require.False(t, pool.index.hasSender(addr1))
			require.Zero(t, pool.gauge.read())
			require.Zero(t, atomic.LoadInt64(&pool.pending))
			require.Zero(t, atomic.LoadUint64(&pool.accounts.count))

			nonce, err = pool.GetNonce(addr1)
			require.NoError(t, err)
			require.Equal(t, uint64(2), nonce)
		})
	}
}

func TestIssue13_DifferentMinedHashCleanupWaitsForSuccessfulReset(t *testing.T) {
	for _, failure := range []string{"none", "nonce", "fee"} {
		t.Run(failure, func(t *testing.T) {
			pool, store := issue7ANewPool(t)
			t.Cleanup(pool.Close)
			pool.SetSealing(true)
			pool.promoteReqCh = make(chan promoteRequest, 8)

			selected := issue13SelectionTx(0, 2, 10)
			require.NoError(t, pool.AddTx(selected))
			pool.handlePromoteRequest(promoteRequest{account: addr1})

			originalAccount := pool.accounts.get(addr1)
			require.NotNil(t, originalAccount)

			require.NoError(t, pool.WithReconciledPool(func() error {
				pool.Prepare()
				if got := pool.Peek(); got != selected {
					return errors.New("unexpected selected transaction")
				}

				pool.Pop(selected)

				return nil
			}))

			// START ALPHA CHANGE 2355 -- keep selected transaction capacity until confirmed reset --
			requireIssue13Queues(t, pool, addr1, 0, 1)
			require.Equal(t, uint64(2), pool.gauge.read())
			require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))

			// This transaction was never admitted locally. Only its
			// sender and nonce match the selected transaction.
			// FINISH ALPHA CHANGE 2355 -- keep selected transaction capacity until confirmed reset --
			mined := issue13SelectionTx(0, 1, 20)
			mined.ComputeHash(store.Header().Number)
			require.NotEqual(t, selected.Hash, mined.Hash)

			store.nonce = 1
			store.block = &types.Block{
				Header:       store.Header(),
				Transactions: []*types.Transaction{mined},
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
				require.ErrorIs(t, pool.resetErr, state.ErrLocalStateFailure)
				require.Same(t, originalAccount, pool.accounts.get(addr1))

				indexed, exists := pool.index.get(selected.Hash)
				require.True(t, exists)
				require.Same(t, selected, indexed)
				// START ALPHA CHANGE 2356 -- preserve selected queues and counters after reset failure --
				require.True(t, pool.index.hasSender(addr1))
				requireIssue13Queues(t, pool, addr1, 0, 1)
				require.Equal(t, uint64(2), pool.gauge.read())
				require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))
				// FINISH ALPHA CHANGE 2356 -- preserve selected queues and counters after reset failure --

				store.nonceErr = nil
				store.calculateBaseFeeFn = nil
				issue7AResetRetryDue(pool)

				require.NoError(t, pool.WithReconciledPool(func() error {
					return nil
				}))
			}

			require.NoError(t, pool.resetErr)

			_, exists := pool.index.get(selected.Hash)
			require.False(t, exists)
			require.False(t, pool.index.hasSender(addr1))
			require.Nil(t, pool.accounts.get(addr1))
			require.Zero(t, atomic.LoadUint64(&pool.accounts.count))
			require.Zero(t, pool.gauge.read())
			require.Zero(t, atomic.LoadInt64(&pool.pending))

			// Repeating the reset must not subtract slots or pending
			// transactions a second time.
			pool.ResetWithHeaders(store.block.Header)
			require.NoError(t, pool.resetErr)
			require.Zero(t, pool.gauge.read())
			require.Zero(t, atomic.LoadInt64(&pool.pending))
			require.Zero(t, atomic.LoadUint64(&pool.accounts.count))

			nonce, err := pool.GetNonce(addr1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), nonce)
		})
	}
}

func TestIssue13_IndexNonceCleanupIsSenderScoped(t *testing.T) {
	var index lookupMap

	obsolete := &types.Transaction{
		Hash:  types.StringToHash("0x133901"),
		From:  addr1,
		Nonce: 4,
	}
	current := &types.Transaction{
		Hash:  types.StringToHash("0x133902"),
		From:  addr1,
		Nonce: 5,
	}
	future := &types.Transaction{
		Hash:  types.StringToHash("0x133903"),
		From:  addr1,
		Nonce: 6,
	}
	unmentioned := &types.Transaction{
		Hash:  types.StringToHash("0x133904"),
		From:  addr2,
		Nonce: 0,
	}

	for _, tx := range []*types.Transaction{obsolete, current, future, unmentioned} {
		require.True(t, index.add(tx))
	}

	index.removeBelowNonces(map[types.Address]uint64{
		addr1: 5,
	})

	_, exists := index.get(obsolete.Hash)
	require.False(t, exists)

	for _, tx := range []*types.Transaction{current, future, unmentioned} {
		stored, exists := index.get(tx.Hash)
		require.True(t, exists)
		require.Same(t, tx, stored)
	}

	require.True(t, index.hasSender(addr1))
	require.True(t, index.hasSender(addr2))

	index.removeBelowNonces(map[types.Address]uint64{
		addr1: 7,
	})

	require.False(t, index.hasSender(addr1))
	require.True(t, index.hasSender(addr2))

	stored, exists := index.get(unmentioned.Hash)
	require.True(t, exists)
	require.Same(t, unmentioned, stored)

	// Cleanup and repeated exact-hash removal remain idempotent.
	index.remove(obsolete, current, future)
	index.removeBelowNonces(map[types.Address]uint64{
		addr1: 7,
	})
	require.True(t, index.hasSender(addr2))

	index.remove(unmentioned)
	require.False(t, index.hasSender(addr2))

	index.RLock()
	transactionCount := len(index.all)
	senderCount := len(index.bySender)
	index.RUnlock()

	require.Zero(t, transactionCount)
	require.Zero(t, senderCount)
}

// FINISH ALPHA CHANGE 2339 -- test replacement accounting and confirmed-nonce ownership cleanup --
