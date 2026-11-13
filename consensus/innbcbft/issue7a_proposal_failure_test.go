/* START ALPHA CHANGE 2157 -- preserve execution failure identity through proposal validation -- */
package innbcbft

import (
	/* START ALPHA CHANGE 3320 -- remove the retired wall-clock fixture dependency -- */
	"errors"
	"testing"

	/* FINISH ALPHA CHANGE 3320 -- remove the retired wall-clock fixture dependency -- */

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIssue7A_FSMValidatePreservesLocalExecutionFailure(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parentExtra := createTestExtraObject(
		validators.GetPublicIdentities(),
		validator.AccountSet{},
		4,
		signaturesCount,
		signaturesCount,
	)
	parentExtra.Validators = nil

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: parentExtra.MarshalRLPTo(nil),
	}
	parent.ComputeHash()

	innbcbftBackendMock := new(innbcbftBackendMock)
	/* START ALPHA CHANGE 2177 -- exercise both FSM and outer runtime validation -- */
	innbcbftBackendMock.On("GetValidators", mock.Anything, mock.Anything).
		Return(validators.GetPublicIdentities(), nil).
		Twice()
	/* FINISH ALPHA CHANGE 2177 -- exercise both FSM and outer runtime validation -- */

	extra := createTestExtraObject(
		validators.GetPublicIdentities(),
		validator.AccountSet{},
		4,
		signaturesCount,
		signaturesCount,
	)
	extra.Validators = nil

	parentCheckpointHash, err := extra.Checkpoint.Hash(0, parentBlockNumber, parent.Hash)
	require.NoError(t, err)

	currentValSetHash, err := validators.GetPublicIdentities().Hash()
	require.NoError(t, err)

	extra.Parent = createSignature(
		t,
		validators.GetPrivateIdentities(),
		parentCheckpointHash,
		signer.DomainCheckpointManager,
	)
	extra.Checkpoint.EpochNumber = 1
	extra.Checkpoint.CurrentValidatorsHash = currentValSetHash
	extra.Checkpoint.NextValidatorsHash = currentValSetHash

	/* START ALPHA CHANGE 3320 -- reach execution failure with a valid chain timestamp -- */
	stateBlock := createDummyStateBlock(
		parent.Number+1,
		types.Hash{100, 15},
		extra.MarshalRLPTo(nil),
		parent.Timestamp,
	)
	/* FINISH ALPHA CHANGE 3320 -- reach execution failure with a valid chain timestamp -- */

	proposalHash, err := extra.Checkpoint.Hash(
		0,
		stateBlock.Block.Number(),
		stateBlock.Block.Hash(),
	)
	require.NoError(t, err)

	stateBlock.Block.Header.Hash = proposalHash
	stateBlock.Block.Header.ParentHash = parent.Hash
	/* START ALPHA CHANGE 3320 -- retain a valid timestamp before exercising later validation -- */
	stateBlock.Block.Header.Timestamp = parent.Timestamp + 1
	/* FINISH ALPHA CHANGE 3320 -- retain a valid timestamp before exercising later validation -- */
	stateBlock.Block.Transactions = []*types.Transaction{}

	proposal := stateBlock.Block.MarshalRLP()

	blockchainMock := new(blockchainMock)
	cause := errors.New("injected proposal execution failure")
	local := state.WrapLocalStateFailure("proposal execution", cause)
	/* START ALPHA CHANGE 2177 -- preserve local failure through repeated validation boundaries -- */
	blockchainMock.On("ProcessBlock", mock.Anything, mock.Anything).
		Return((*types.FullBlock)(nil), local).
		Twice()
	/* FINISH ALPHA CHANGE 2177 -- preserve local failure through repeated validation boundaries -- */

	fsm := &fsm{
		parent:          parent,
		backend:         blockchainMock,
		validators:      validators.ToValidatorSet(),
		logger:          hclog.NewNullLogger(),
		innbcbftBackend: innbcbftBackendMock,
		config:          testChainClockConfig(),
		/* START ALPHA CHANGE 2576 -- provide the authoritative epoch in existing proposal fixtures -- */
		epochNumber: 1,
		/* FINISH ALPHA CHANGE 2576 -- provide the authoritative epoch in existing proposal fixtures -- */
		/* START ALPHA CHANGE 355 -- remove legacy bridge exit event root from proposal validation test -- */
		/* FINISH ALPHA CHANGE 355 -- remove legacy bridge exit event root from proposal validation test -- */
	}

	err = fsm.Validate(proposal)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.Nil(t, fsm.target)

	/* START ALPHA CHANGE 2177 -- verify unavailable state cannot produce an accepted proposal -- */
	runtime := &consensusRuntime{
		fsm:    fsm,
		logger: hclog.NewNullLogger(),
	}
	require.False(t, runtime.IsValidProposal(proposal))
	require.Nil(t, fsm.target)
	/* FINISH ALPHA CHANGE 2177 -- verify unavailable state cannot produce an accepted proposal -- */

	innbcbftBackendMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

/* FINISH ALPHA CHANGE 2157 -- preserve execution failure identity through proposal validation -- */
