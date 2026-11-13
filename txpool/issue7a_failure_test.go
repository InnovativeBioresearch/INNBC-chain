/* START ALPHA CHANGE 2166 -- test pool preservation after local state failures -- */
package txpool

import (
	"errors"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type issue7APoolStore struct {
	defaultMockStore

	nonceErr   error
	balanceErr error
	nonceCalls int
	failAt     int
	block      *types.Block
}

func (s *issue7APoolStore) GetNonce(types.Hash, types.Address) (uint64, error) {
	s.nonceCalls++
	if s.nonceErr != nil && (s.failAt == 0 || s.nonceCalls == s.failAt) {
		return 0, s.nonceErr
	}

	return s.nonce, nil
}

func (s *issue7APoolStore) GetBalance(root types.Hash, address types.Address) (*big.Int, error) {
	if s.balanceErr != nil {
		return nil, s.balanceErr
	}

	return s.defaultMockStore.GetBalance(root, address)
}

func (s *issue7APoolStore) GetBlockByHash(types.Hash, bool) (*types.Block, bool) {
	return s.block, s.block != nil
}

func issue7ANewPool(t *testing.T) (*TxPool, *issue7APoolStore) {
	t.Helper()

	store := &issue7APoolStore{
		defaultMockStore: NewDefaultMockStore(mockHeader),
	}
	pool, err := newTestPool(store)
	require.NoError(t, err)
	pool.SetSigner(&mockSigner{})

	return pool, store
}

func TestIssue7A_PendingNonceFailureDoesNotInitializeAccount(t *testing.T) {
	pool, store := issue7ANewPool(t)
	cause := errors.New("injected pending nonce failure")
	store.nonceErr = cause

	_, err := pool.GetNonce(addr1)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.Nil(t, pool.accounts.get(addr1))

	account, err := pool.getOrCreateAccount(addr1)
	require.Nil(t, account)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.Nil(t, pool.accounts.get(addr1))

	// Previously initialized healthy pool data remains usable.
	pool.accounts.initOnce(addr1, 7)
	callsBefore := store.nonceCalls
	nonce, err := pool.GetNonce(addr1)
	require.NoError(t, err)
	require.Equal(t, uint64(7), nonce)
	require.Equal(t, callsBefore, store.nonceCalls)
}

func TestIssue7A_AdmissionFailureDoesNotPublishTransaction(t *testing.T) {
	for _, phase := range []string{"nonce", "balance", "initialization"} {
		t.Run(phase, func(t *testing.T) {
			pool, store := issue7ANewPool(t)
			cause := errors.New("injected admission state failure")
			switch phase {
			case "nonce":
				store.nonceErr = cause
			case "balance":
				store.balanceErr = cause
			case "initialization":
				// Validation succeeds; the subsequent initialization read fails.
				store.nonceErr = cause
				store.failAt = 2
			}

			tx := newTx(addr1, 0, 1)
			err := pool.addTx(local, tx)
			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, state.ErrLocalStateFailure)
			require.NotErrorIs(t, err, ErrNonceTooLow)
			require.NotErrorIs(t, err, ErrInsufficientFunds)
			require.Nil(t, pool.accounts.get(addr1))
			require.Zero(t, pool.gauge.read())
			_, exists := pool.index.get(tx.Hash)
			require.False(t, exists)
		})
	}
}

func TestIssue7A_ResetNonceFailurePreservesPoolEntries(t *testing.T) {
	pool, store := issue7ANewPool(t)
	tx := newTx(addr1, 0, 1)
	tx.ComputeHash(1)
	require.True(t, pool.index.add(tx))
	account := pool.accounts.initOnce(addr1, 7)
	account.enqueued.queue = minNonceQueue{tx}

	store.block = &types.Block{
		Header:       &types.Header{Number: 1},
		Transactions: []*types.Transaction{tx},
	}
	store.nonceErr = errors.New("injected reset nonce failure")

	pool.processEvent(&blockchain.Event{
		NewChain: []*types.Header{store.block.Header},
	})

	_, exists := pool.index.get(tx.Hash)
	require.True(t, exists)
	require.Equal(t, uint64(7), account.getNonce())
	require.Equal(t, uint64(1), account.enqueued.length())
	require.Same(t, tx, account.enqueued.peek())
}

func TestIssue7A_CleanupNonceFailureDoesNotDropAccount(t *testing.T) {
	pool, store := issue7ANewPool(t)
	tx := newTx(addr1, 0, 1)
	tx.ComputeHash(1)
	require.True(t, pool.index.add(tx))
	account := pool.accounts.initOnce(addr1, 0)
	account.enqueued.queue = minNonceQueue{tx}
	atomic.StoreUint64(&account.skips, uint64(maxAccountSkips))

	store.nonceErr = errors.New("injected cleanup nonce failure")
	pool.updateAccountSkipsCounts(map[types.Address]uint64{})

	_, exists := pool.index.get(tx.Hash)
	require.True(t, exists)
	require.Same(t, account, pool.accounts.get(addr1))
	require.Equal(t, uint64(1), account.enqueued.length())
	require.Same(t, tx, account.enqueued.peek())
}

/* FINISH ALPHA CHANGE 2166 -- test pool preservation after local state failures -- */
