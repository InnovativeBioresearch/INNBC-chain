/* START ALPHA CHANGE 2195 -- test fee refresh failure propagation and recovery -- */
package txpool

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestIssue7A_BaseFeeFailureBlocksAdmissionUntilRefresh(t *testing.T) {
	pool, store := issue7ANewPool(t)
	header := store.Header()

	store.calculateBaseFeeFn = func(*types.Header) (uint64, error) {
		return 0, nil
	}
	require.NoError(t, pool.SetBaseFee(header))

	tx := newTx(addr1, 0, 1)
	require.NotNil(t, tx)
	require.NoError(t, pool.validateTx(tx))

	cause := errors.New("injected governance storage failure")
	local := state.WrapLocalStateFailure("read governance fee cap", cause)
	store.calculateBaseFeeFn = func(*types.Header) (uint64, error) {
		return 0, local
	}

	require.ErrorIs(t, pool.SetBaseFee(header), cause)

	_, err := pool.admissionBaseFee()
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)

	err = pool.addTx(localOriginForIssue7A(), tx)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.NotErrorIs(t, err, ErrUnderpriced)
	require.NotErrorIs(t, err, ErrReplacementUnderpriced)
	require.Zero(t, pool.gauge.read())
	require.Nil(t, pool.accounts.get(addr1))

	_, exists := pool.index.get(tx.Hash)
	require.False(t, exists)

	// The previously successful cached fee is not replaced with a
	// fabricated fee, but admission cannot use it while refresh is failed.
	require.Zero(t, pool.GetBaseFee())

	store.calculateBaseFeeFn = func(*types.Header) (uint64, error) {
		return 0, nil
	}
	require.NoError(t, pool.SetBaseFee(header))

	fee, err := pool.admissionBaseFee()
	require.NoError(t, err)
	require.Zero(t, fee)
	require.NoError(t, pool.validateTx(tx))
}

// Keep the transaction-origin constant distinct from local error variables.
func localOriginForIssue7A() txOrigin {
	return local
}

func TestIssue7A_BaseFeeFailurePreservesResetEntries(t *testing.T) {
	pool, store := issue7ANewPool(t)
	tx := newTx(addr1, 0, 1)
	require.NotNil(t, tx)
	tx.ComputeHash(1)

	require.True(t, pool.index.add(tx))
	account := pool.accounts.initOnce(addr1, 7)
	account.enqueued.queue = minNonceQueue{tx}

	store.block = &types.Block{
		Header:       &types.Header{Number: 1},
		Transactions: []*types.Transaction{tx},
	}

	cause := errors.New("injected base fee refresh failure")
	localErr := state.WrapLocalStateFailure("refresh base fee", cause)
	store.calculateBaseFeeFn = func(*types.Header) (uint64, error) {
		return 0, localErr
	}

	pool.processEvent(&blockchain.Event{
		NewChain: []*types.Header{store.block.Header},
	})

	_, exists := pool.index.get(tx.Hash)
	require.True(t, exists)
	require.Equal(t, uint64(7), account.getNonce())
	require.Equal(t, uint64(1), account.enqueued.length())
	require.Same(t, tx, account.enqueued.peek())

	_, err := pool.admissionBaseFee()
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
}

func TestIssue7A_BaseFeeLocalFailureUsesLoggingLimiter(t *testing.T) {
	pool, store := issue7ANewPool(t)
	var output bytes.Buffer
	pool.logger = hclog.New(&hclog.LoggerOptions{
		Name:   "issue7a-base-fee",
		Level:  hclog.Error,
		Output: &output,
	})

	localErr := state.WrapLocalStateFailure(
		"refresh base fee",
		errors.New("injected backend failure"),
	)
	store.calculateBaseFeeFn = func(*types.Header) (uint64, error) {
		return 0, localErr
	}

	for attempt := 0; attempt < 100; attempt++ {
		require.ErrorIs(t, pool.SetBaseFee(store.Header()), state.ErrLocalStateFailure)
	}

	require.NotContains(t, output.String(), "failed to update txpool base fee")
	require.LessOrEqual(
		t,
		strings.Count(output.String(), "local state unavailable; operation aborted"),
		1,
	)

	// Ordinary errors retain their ordinary diagnostic.
	output.Reset()
	ordinary := errors.New("ordinary fee calculation failure")
	store.calculateBaseFeeFn = func(*types.Header) (uint64, error) {
		return 0, ordinary
	}

	require.ErrorIs(t, pool.SetBaseFee(store.Header()), ordinary)
	require.Contains(t, output.String(), "failed to update txpool base fee")
	require.NotContains(t, output.String(), "local state unavailable; operation aborted")
}

/* FINISH ALPHA CHANGE 2195 -- test fee refresh failure propagation and recovery -- */
