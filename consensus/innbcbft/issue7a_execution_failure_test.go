/* START ALPHA CHANGE 2159 -- test fatal fill propagation and direct consensus state calls -- */
package innbcbft

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

type issue7AConsensusState struct {
	state.State
	address types.Address
	cause   error
}

type issue7AConsensusSnapshot struct {
	state.Snapshot
	owner *issue7AConsensusState
}

func (s *issue7AConsensusState) NewSnapshotAt(root types.Hash) (state.Snapshot, error) {
	snap, err := s.State.NewSnapshotAt(root)
	if err != nil {
		return nil, err
	}

	return &issue7AConsensusSnapshot{Snapshot: snap, owner: s}, nil
}

func (s *issue7AConsensusSnapshot) GetAccount(addr types.Address) (*state.Account, error) {
	if addr == s.owner.address && s.owner.cause != nil {
		return nil, s.owner.cause
	}

	return s.Snapshot.GetAccount(addr)
}

func issue7AConsensusExecutor(s state.State) *state.Executor {
	executor := state.NewExecutor(
		&chain.Params{ChainID: 100, Forks: chain.AllForksEnabled},
		s,
		hclog.NewNullLogger(),
	)
	/* START ALPHA CHANGE 3810 -- adapt consensus failure fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt consensus failure fixture to checked hash lookups -- */

	return executor
}

func TestIssue7A_FillAbortsWithoutPenalizingTriggeringTransaction(t *testing.T) {
	cause := errors.New("injected sender read failure")
	sender := types.StringToAddress("0x7101")
	backend := &issue7AConsensusState{
		State:   itrie.NewState(itrie.NewMemoryStorage()),
		address: sender,
		cause:   cause,
	}
	tx := &types.Transaction{
		From:     sender,
		To:       types.StringToAddress("0x7102").Ptr(),
		Gas:      100000,
		GasPrice: big.NewInt(1),
		Value:    new(big.Int),
	}

	pool := &txPoolMock{}
	pool.On("Prepare").Once()
	pool.On("Peek").Return(tx).Once()

	builder := NewBlockBuilder(&BlockBuilderParams{
		Parent:    &types.Header{StateRoot: types.EmptyRootHash},
		Executor:  issue7AConsensusExecutor(backend),
		GasLimit:  1_000_000,
		BlockTime: time.Hour,
		BaseFee:   1,
		TxPool:    pool,
		Logger:    hclog.NewNullLogger(),
	})
	require.NoError(t, builder.Reset())

	done := make(chan error, 1)
	go func() {
		done <- builder.Fill()
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, state.ErrLocalStateFailure)
		require.ErrorIs(t, err, cause)
	case <-time.After(5 * time.Second):
		t.Fatal("local failure waited for block timer instead of aborting")
	}

	pool.AssertExpectations(t)
	pool.AssertNotCalled(t, "Drop", mock.Anything)
	pool.AssertNotCalled(t, "Demote", mock.Anything)
	pool.AssertNotCalled(t, "Pop", mock.Anything)

	require.Empty(t, builder.txns)
	require.Empty(t, builder.Receipts())
	require.Zero(t, builder.GetState().TotalGas())

	built, err := builder.Build(nil)
	require.Nil(t, built)
	require.ErrorIs(t, err, cause)

	// Latched errors precede later gas-limit checks and do not revisit the pool.
	tx.Gas = 2_000_000
	require.ErrorIs(t, builder.WriteTx(tx), cause)
	require.ErrorIs(t, builder.Fill(), cause)
	pool.AssertNumberOfCalls(t, "Prepare", 1)
}

func TestIssue7A_FSMBuildDoesNotContinueAfterFillError(t *testing.T) {
	validators := validator.NewTestValidators(t, 5)
	extra := createTestExtra(
		validators.GetPublicIdentities(),
		validator.AccountSet{},
		4,
		3,
		3,
	)
	parent := &types.Header{Number: 9, ExtraData: extra}
	parent.ComputeHash()

	cause := errors.New("injected fill failure")
	local := state.WrapLocalStateFailure("test fill", cause)

	builder := &blockBuilderMock{}
	builder.On("Reset").Return(error(nil)).Once()
	builder.On("Fill").Return(local).Once()

	machine := &fsm{
		parent:       parent,
		blockBuilder: builder,
		validators:   validators.ToValidatorSet(),
		logger:       hclog.NewNullLogger(),
	}

	proposal, err := machine.BuildProposal(0)
	require.Nil(t, proposal)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.Nil(t, machine.target)

	builder.AssertNotCalled(t, "Build", mock.Anything)
	builder.AssertExpectations(t)
}

func TestIssue7A_SystemContractReadersReturnLocalFailure(t *testing.T) {
	for _, viaProvider := range []bool{false, true} {
		t.Run(map[bool]string{false: "initializer", true: "state provider"}[viaProvider], func(t *testing.T) {
			address := types.StringToAddress("0x7103")
			cause := errors.New("injected system contract account failure")

			backend := &issue7AConsensusState{
				State:   itrie.NewState(itrie.NewMemoryStorage()),
				address: address,
				cause:   cause,
			}
			executor := issue7AConsensusExecutor(backend)
			tr, err := executor.BeginTxn(
				types.EmptyRootHash,
				&types.Header{GasLimit: 1_000_000},
				types.ZeroAddress,
			)
			require.NoError(t, err)

			if viaProvider {
				data, callErr := NewStateProvider(tr).Call(
					ethgo.Address(address),
					nil,
					nil,
				)
				require.Nil(t, data)
				err = callErr
			} else {
				err = callContract(contracts.SystemCaller, address, nil, "test", tr)
			}

			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, state.ErrLocalStateFailure)
			require.Empty(t, tr.Receipts())
		})
	}
}

func TestIssue7A_ProposalWrapperPreservesLocalFailure(t *testing.T) {
	wrapper := newTestBlockchainWrapper(t)
	parent := wrapper.blockchain.Header()
	proposal := newValidEmptyProposal(t, wrapper, parent)

	cause := errors.New("injected proposal governance read failure")
	backend := &issue7AConsensusState{
		State:   wrapper.executor.State(),
		address: contracts.INNBCBlockRewardGovernanceContract,
		cause:   cause,
	}
	wrapper.executor = issue7AConsensusExecutor(backend)

	full, err := wrapper.ProcessBlock(parent, proposal)
	require.Nil(t, full)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.False(t, errors.Is(err, blockchain.ErrInvalidStateRoot))
	require.False(t, errors.Is(err, blockchain.ErrInvalidReceiptsRoot))
}

/* FINISH ALPHA CHANGE 2159 -- test fatal fill propagation and direct consensus state calls -- */
