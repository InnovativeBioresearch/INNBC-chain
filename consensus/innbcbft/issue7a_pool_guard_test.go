/* START ALPHA CHANGE 2205 -- test builder reconciliation failures and protected pool actions -- */
package innbcbft

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type issue7ABuildGuardPool struct {
	txPoolMock

	guardErr error
	entered  bool
	active   bool
	released bool
}

func (p *issue7ABuildGuardPool) WithReconciledPool(
	consume func() error,
) error {
	p.entered = true

	if p.guardErr != nil {
		return p.guardErr
	}

	p.active = true
	defer func() {
		p.active = false
		p.released = true
	}()

	return consume()
}

func issue7ANewGuardedBuilder(
	t *testing.T,
	pool txPoolInterface,
) *BlockBuilder {
	t.Helper()

	builder := NewBlockBuilder(&BlockBuilderParams{
		Parent: &types.Header{
			StateRoot: types.EmptyRootHash,
		},
		Executor: issue7AConsensusExecutor(
			itrie.NewState(itrie.NewMemoryStorage()),
		),
		GasLimit: 1_000_000,
		/* START ALPHA CHANGE 3320 -- use a supported builder interval -- */
		BlockTime: time.Second,
		/* FINISH ALPHA CHANGE 3320 -- use a supported builder interval -- */
		BaseFee: 1,
		TxPool:  pool,
		Logger:  hclog.NewNullLogger(),
	})

	require.NoError(t, builder.Reset())

	return builder
}

func TestIssue7A_FillRejectsIncompletePoolReset(t *testing.T) {
	cause := errors.New("injected incomplete pool reset")
	pool := &issue7ABuildGuardPool{
		guardErr: state.WrapLocalStateFailure("reconcile pool", cause),
	}
	builder := issue7ANewGuardedBuilder(t, pool)

	err := builder.Fill()

	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.True(t, pool.entered)
	require.False(t, pool.active)

	pool.AssertNotCalled(t, "Prepare")
	pool.AssertNotCalled(t, "Peek")
	pool.AssertNotCalled(t, "Pop", mock.Anything)
	pool.AssertNotCalled(t, "Demote", mock.Anything)
	pool.AssertNotCalled(t, "Drop", mock.Anything)

	require.Empty(t, builder.txns)
	require.Empty(t, builder.Receipts())
	require.Zero(t, builder.GetState().TotalGas())
}

func TestIssue7A_FillUsesGuardForSelectionAndOrdinaryPoolActions(t *testing.T) {
	pool := &issue7ABuildGuardPool{}
	builder := issue7ANewGuardedBuilder(t, pool)

	// This healthy empty state has nonce zero. Nonce one therefore
	// produces an ordinary recoverable transaction error, not a
	// local state failure.
	tx := &types.Transaction{
		From:     types.StringToAddress("0x7201"),
		To:       types.StringToAddress("0x7202").Ptr(),
		Nonce:    1,
		Gas:      100000,
		GasPrice: big.NewInt(1),
		Value:    new(big.Int),
	}

	assertProtected := func(mock.Arguments) {
		require.True(t, pool.active, "pool operation escaped the protected callback")
	}

	pool.On("Prepare").Run(assertProtected).Once()
	pool.On("Peek").Return(tx).Run(assertProtected).Once()
	pool.On("Demote", tx).Run(assertProtected).Once()
	pool.On("Peek").Return((*types.Transaction)(nil)).Run(assertProtected).Once()

	// Allow enough time for the ordinary transaction-error path under
	// race instrumentation without making correctness depend on a
	// very short block timer.
	/* START ALPHA CHANGE 3320 -- retain supported pacing during pool-action coverage -- */
	builder.params.BlockTime = time.Second
	/* FINISH ALPHA CHANGE 3320 -- retain supported pacing during pool-action coverage -- */

	require.NoError(t, builder.Fill())
	require.True(t, pool.entered)
	require.True(t, pool.released)
	require.False(t, pool.active)
	require.NoError(t, builder.GetState().Error())

	pool.AssertExpectations(t)
	pool.AssertNotCalled(t, "Pop", mock.Anything)
	pool.AssertNotCalled(t, "Drop", mock.Anything)
	require.Empty(t, builder.txns)
	require.Empty(t, builder.Receipts())
}

/* FINISH ALPHA CHANGE 2205 -- test builder reconciliation failures and protected pool actions -- */
