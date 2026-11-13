package innbcbft

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/bitmap"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"

	/* START ALPHA CHANGE 1872 -- test with INNBC-owned IBFT message packages -- */
	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"

	/* FINISH ALPHA CHANGE 1872 -- test with INNBC-owned IBFT message packages -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/contracts"

	/* START ALPHA CHANGE 3320 -- give consensus fixtures explicit valid clock intervals -- */
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	/* FINISH ALPHA CHANGE 3320 -- give consensus fixtures explicit valid clock intervals -- */
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFSM_ValidateHeader(t *testing.T) {
	t.Parallel()

	/* START ALPHA CHANGE 3320 -- validate fixture headers with a one-second interval -- */
	blockTime := time.Second
	/* FINISH ALPHA CHANGE 3320 -- validate fixture headers with a one-second interval -- */
	extra := createTestExtra(validator.AccountSet{}, validator.AccountSet{}, 0, 0, 0)
	parent := &types.Header{Number: 0, Hash: types.BytesToHash([]byte{1, 2, 3})}
	header := &types.Header{Number: 0}

	// extra data
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "extra-data shorter than")
	header.ExtraData = extra

	// parent hash
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "incorrect header parent hash")
	header.ParentHash = parent.Hash

	// sequence number
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "invalid number")
	header.Number = 1

	// failed timestamp
	/* START ALPHA CHANGE 3320 -- require the exact fixture successor timestamp -- */
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "invalid block timestamp")
	header.Timestamp = 1
	/* FINISH ALPHA CHANGE 3320 -- require the exact fixture successor timestamp -- */

	// failed nonce
	header.SetNonce(1)
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "invalid nonce")

	header.SetNonce(0)

	// failed gas
	header.GasLimit = 10
	header.GasUsed = 11
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "invalid gas limit")
	header.GasLimit = 10
	header.GasUsed = 10

	// mix digest
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "mix digest is not correct")
	header.MixHash = INNBCBFTMixDigest

	/* START ALPHA CHANGE 812 -- test fixed INNBCBFT difficulty invariant -- */
	// difficulty
	header.Difficulty = 0
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "difficulty should be 1")

	header.Difficulty = 2
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "difficulty should be 1")

	header.Difficulty = 1
	/* FINISH ALPHA CHANGE 812 -- test fixed INNBCBFT difficulty invariant -- */
	header.Hash = types.BytesToHash([]byte{11, 22, 33})
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "invalid header hash")
	/* START ALPHA CHANGE 3320 -- reject a larger increment independently of local time -- */
	header.Timestamp = 2
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTime), "invalid block timestamp")

	header.Timestamp = 1
	/* FINISH ALPHA CHANGE 3320 -- reject a larger increment independently of local time -- */

	header.ComputeHash()
	require.NoError(t, validateHeaderFields(parent, header, blockTime))
}

func TestFSM_verifyCommitEpochTx(t *testing.T) {
	t.Parallel()

	fsm := &fsm{
		isEndOfEpoch:     true,
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		parent:           &types.Header{},
	}

	// include commit epoch transaction to the epoch ending block
	commitEpochTx, err := fsm.createCommitEpochTx()
	assert.NoError(t, err)
	assert.NotNil(t, commitEpochTx)

	assert.NoError(t, fsm.verifyCommitEpochTx(commitEpochTx))

	// submit tampered commit epoch transaction to the epoch ending block
	alteredCommitEpochTx := &types.Transaction{
		To:    &contracts.ValidatorSetContract,
		Input: []byte{},
		Gas:   0,
		Type:  types.StateTx,
	}
	assert.ErrorContains(t, fsm.verifyCommitEpochTx(alteredCommitEpochTx), "invalid commit epoch transaction")

	// submit validators commit epoch transaction to the non-epoch ending block
	fsm.isEndOfEpoch = false
	commitEpochTx, err = fsm.createCommitEpochTx()
	assert.NoError(t, err)
	assert.NotNil(t, commitEpochTx)
	assert.ErrorContains(t, fsm.verifyCommitEpochTx(commitEpochTx), errCommitEpochTxNotExpected.Error())
}

func TestFSM_BuildProposal_WithoutCommitEpochTxGood(t *testing.T) {
	t.Parallel()

	const (
		accountCount             = 5
		committedCount           = 4
		parentCount              = 3
		confirmedStateSyncsCount = 5
		parentBlockNumber        = 1023
		currentRound             = 1
	)

	eventRoot := types.ZeroHash

	validators := validator.NewTestValidators(t, accountCount)
	validatorSet := validators.GetPublicIdentities()
	extra := createTestExtra(validatorSet, validator.AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra, parent.Timestamp)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	blockchainMock := &blockchainMock{}
	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{
			Key:        wallet.NewKey(validators.GetPrivateIdentities()[0]),
			blockchain: blockchainMock,
		},
	}

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: testChainClockConfig(), backend: blockchainMock,
		validators: validators.ToValidatorSet(), logger: hclog.NewNullLogger()}

	proposal, err := fsm.BuildProposal(currentRound)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	currentValidatorsHash, err := validatorSet.Hash()
	require.NoError(t, err)

	rlpBlock := stateBlock.Block.MarshalRLP()
	assert.Equal(t, rlpBlock, proposal)

	block := types.Block{}
	require.NoError(t, block.UnmarshalRLP(proposal))

	checkpoint := &CheckpointData{
		BlockRound:            currentRound,
		EpochNumber:           fsm.epochNumber,
		EventRoot:             eventRoot,
		CurrentValidatorsHash: currentValidatorsHash,
		NextValidatorsHash:    currentValidatorsHash,
	}

	checkpointHash, err := checkpoint.Hash(fsm.backend.GetChainID(), block.Number(), block.Hash())
	require.NoError(t, err)

	msg := runtime.BuildPrePrepareMessage(proposal, nil, &proto.View{})
	require.Equal(t, checkpointHash.Bytes(), msg.GetPreprepareData().ProposalHash)

	mBlockBuilder.AssertExpectations(t)
}

/* START ALPHA CHANGE 818 -- reject locally built future proposal -- */
/* START ALPHA CHANGE 3320 -- test rejection of a locally built incorrect interval -- */
func TestFSM_BuildProposal_RejectsLocallyBuiltIncorrectTimestamp(t *testing.T) {
	/* FINISH ALPHA CHANGE 3320 -- test rejection of a locally built incorrect interval -- */
	t.Parallel()

	const (
		accountCount      = 5
		parentBlockNumber = 10
	)

	validators := validator.NewTestValidators(t, accountCount)
	extra := createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, accountCount-1, 3, 3)

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: extra,
		Timestamp: uint64(time.Now().UTC().Unix()),
	}
	parent.ComputeHash()

	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra, parent.Timestamp)
	/* START ALPHA CHANGE 3320 -- violate the configured one-second increment -- */
	stateBlock.Block.Header.Timestamp = parent.Timestamp + 2
	/* FINISH ALPHA CHANGE 3320 -- violate the configured one-second increment -- */
	stateBlock.Block.Header.ComputeHash()

	mBlockBuilder := newBlockBuilderMock(stateBlock)

	fsm := &fsm{
		parent:       parent,
		blockBuilder: mBlockBuilder,
		config:       testChainClockConfig(),
		backend:      &blockchainMock{},
		validators:   validators.ToValidatorSet(),
		logger:       hclog.NewNullLogger(),
	}

	_, err := fsm.BuildProposal(0)
	require.ErrorContains(t, err, "locally built proposal failed header validation")
	/* START ALPHA CHANGE 3320 -- retain the intended self-validation failure -- */
	require.ErrorContains(t, err, "invalid block timestamp")
	/* FINISH ALPHA CHANGE 3320 -- retain the intended self-validation failure -- */

	mBlockBuilder.AssertExpectations(t)
}

/* FINISH ALPHA CHANGE 818 -- reject locally built future proposal -- */

func TestFSM_BuildProposal_WithCommitEpochTxGood(t *testing.T) {
	t.Parallel()

	const (
		accountCount             = 5
		committedCount           = 4
		parentCount              = 3
		confirmedStateSyncsCount = 5
		currentRound             = 0
		parentBlockNumber        = 1023
	)

	eventRoot := types.ZeroHash

	validators := validator.NewTestValidators(t, accountCount)
	extra := createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra, parent.Timestamp)

	mBlockBuilder := newBlockBuilderMock(stateBlock)
	/* START ALPHA CHANGE 577 -- expect only commit epoch system transaction in epoch proposal -- */
	/* START ALPHA CHANGE 2062 -- verify the epoch proposal declares the protected gas requirement -- */
	mBlockBuilder.On(
		"WriteTx",
		mock.MatchedBy(func(tx *types.Transaction) bool {
			return tx != nil &&
				tx.Type == types.StateTx &&
				tx.Gas == types.StateTransactionGasLimit
		}),
	).Return(error(nil)).Once()
	/* FINISH ALPHA CHANGE 2062 -- verify the epoch proposal declares the protected gas requirement -- */
	/* FINISH ALPHA CHANGE 577 -- expect only commit epoch system transaction in epoch proposal -- */

	blockChainMock := new(blockchainMock)

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{
			Key:        wallet.NewKey(validators.GetPrivateIdentities()[0]),
			blockchain: blockChainMock,
		},
	}

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: testChainClockConfig(), backend: blockChainMock,
		isEndOfEpoch:     true,
		validators:       validators.ToValidatorSet(),
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		/* START ALPHA CHANGE 578 -- remove obsolete reward distribution input from FSM test -- */
		/* FINISH ALPHA CHANGE 578 -- remove obsolete reward distribution input from FSM test -- */
		/* START ALPHA CHANGE 352 -- remove legacy bridge exit event root from FSM test -- */
		/* FINISH ALPHA CHANGE 352 -- remove legacy bridge exit event root from FSM test -- */
		logger: hclog.NewNullLogger(),
	}

	proposal, err := fsm.BuildProposal(currentRound)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	block := types.Block{}
	require.NoError(t, block.UnmarshalRLP(proposal))

	assert.Equal(t, stateBlock.Block.MarshalRLP(), proposal)

	currentValidatorsHash, err := validators.GetPublicIdentities().Hash()
	require.NoError(t, err)

	nextValidatorsHash, err := validators.ToValidatorSet().Accounts().Hash()
	require.NoError(t, err)

	checkpoint := &CheckpointData{
		BlockRound:            currentRound,
		EpochNumber:           fsm.epochNumber,
		EventRoot:             eventRoot,
		CurrentValidatorsHash: currentValidatorsHash,
		NextValidatorsHash:    nextValidatorsHash,
	}

	checkpointHash, err := checkpoint.Hash(fsm.backend.GetChainID(), block.Number(), block.Hash())
	require.NoError(t, err)

	msg := runtime.BuildPrePrepareMessage(proposal, nil, &proto.View{})
	require.Equal(t, checkpointHash.Bytes(), msg.GetPreprepareData().ProposalHash)

	mBlockBuilder.AssertExpectations(t)
}

func TestFSM_BuildProposal_EpochEndingBlock_FailedToApplyStateTx(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		committedCount    = 4
		parentCount       = 3
		parentBlockNumber = 1023
	)

	validators := validator.NewTestValidators(t, accountCount)
	extra := createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}

	mBlockBuilder := new(blockBuilderMock)
	mBlockBuilder.On("WriteTx", mock.Anything).Return(errors.New("error")).Once()
	mBlockBuilder.On("Reset").Return(error(nil)).Once()

	validatorSet := validator.NewValidatorSet(validators.GetPublicIdentities(), hclog.NewNullLogger())

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, backend: &blockchainMock{},
		isEndOfEpoch:     true,
		validators:       validatorSet,
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		/* START ALPHA CHANGE 353 -- remove legacy bridge exit event root from FSM test -- */
		/* FINISH ALPHA CHANGE 353 -- remove legacy bridge exit event root from FSM test -- */
	}

	_, err := fsm.BuildProposal(0)
	assert.ErrorContains(t, err, "failed to apply commit epoch transaction")
	mBlockBuilder.AssertExpectations(t)
}

func TestFSM_BuildProposal_EpochEndingBlock_ValidatorsDeltaExists(t *testing.T) {
	t.Parallel()

	const (
		validatorsCount          = 6
		remainingValidatorsCount = 3
		signaturesCount          = 4
		parentBlockNumber        = 49
	)

	validators := validator.NewTestValidators(t, validatorsCount).GetPublicIdentities()
	extra := createTestExtraObject(validators, validator.AccountSet{}, validatorsCount-1, signaturesCount, signaturesCount)
	extra.Validators = nil

	extraData := extra.MarshalRLPTo(nil)
	parent := &types.Header{Number: parentBlockNumber, ExtraData: extraData}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extraData, parent.Timestamp)

	blockBuilderMock := newBlockBuilderMock(stateBlock)
	blockBuilderMock.On("WriteTx", mock.Anything).Return(error(nil)).Once()

	addedValidators := validator.NewTestValidators(t, 2).GetPublicIdentities()
	removedValidators := [3]uint64{3, 4, 5}
	removedBitmap := &bitmap.Bitmap{}

	for _, i := range removedValidators {
		removedBitmap.Set(i)
	}

	newDelta := &validator.ValidatorSetDelta{
		Added:   addedValidators,
		Updated: validator.AccountSet{},
		Removed: *removedBitmap,
	}

	blockChainMock := new(blockchainMock)

	validatorSet := validator.NewValidatorSet(validators, hclog.NewNullLogger())

	fsm := &fsm{
		parent:           parent,
		blockBuilder:     blockBuilderMock,
		config:           testChainClockConfig(),
		backend:          blockChainMock,
		isEndOfEpoch:     true,
		validators:       validatorSet,
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		/* START ALPHA CHANGE 579 -- remove obsolete reward distribution input from FSM validator delta test -- */
		/* FINISH ALPHA CHANGE 579 -- remove obsolete reward distribution input from FSM validator delta test -- */
		/* START ALPHA CHANGE 354 -- remove legacy bridge exit event root from FSM test -- */
		/* FINISH ALPHA CHANGE 354 -- remove legacy bridge exit event root from FSM test -- */
		logger:             hclog.NewNullLogger(),
		newValidatorsDelta: newDelta,
	}

	proposal, err := fsm.BuildProposal(0)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	blockExtra, err := GetIbftExtra(stateBlock.Block.Header.ExtraData)
	assert.NoError(t, err)
	assert.Len(t, blockExtra.Validators.Added, 2)
	assert.False(t, blockExtra.Validators.IsEmpty())

	for _, addedValidator := range addedValidators {
		assert.True(t, blockExtra.Validators.Added.ContainsAddress(addedValidator.Address))
	}

	for _, removedValidator := range removedValidators {
		assert.True(
			t,
			blockExtra.Validators.Removed.IsSet(removedValidator),
			fmt.Sprintf("Expected validator at index %d to be marked as removed, but it wasn't", removedValidator),
		)
	}

	blockBuilderMock.AssertExpectations(t)
	blockChainMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_NonEpochEndingBlock_ValidatorsDeltaNil(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 6
		signaturesCount   = 4
		parentBlockNumber = 9
	)

	testValidators := validator.NewTestValidators(t, accountCount)
	extra := createTestExtra(testValidators.GetPublicIdentities(), validator.AccountSet{}, accountCount-1, signaturesCount, signaturesCount)
	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra, parent.Timestamp)

	/* START ALPHA CHANGE 2159 -- provide the Fill error result in the non-epoch fixture -- */
	blockBuilderMock := &blockBuilderMock{}
	blockBuilderMock.On("Build", mock.Anything).Return(stateBlock).Once()
	blockBuilderMock.On("Fill").Return(error(nil)).Once()
	blockBuilderMock.On("Reset").Return(error(nil)).Once()
	/* FINISH ALPHA CHANGE 2159 -- provide the Fill error result in the non-epoch fixture -- */

	fsm := &fsm{parent: parent, blockBuilder: blockBuilderMock,
		config: testChainClockConfig(), backend: &blockchainMock{},
		isEndOfEpoch: false, validators: testValidators.ToValidatorSet(),
		logger: hclog.NewNullLogger()}

	proposal, err := fsm.BuildProposal(0)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	blockExtra, err := GetIbftExtra(stateBlock.Block.Header.ExtraData)
	assert.NoError(t, err)
	assert.Nil(t, blockExtra.Validators)

	blockBuilderMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_EpochEndingBlock_FailToGetNextValidatorsHash(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 6
		signaturesCount   = 4
		parentBlockNumber = 49
	)

	testValidators := validator.NewTestValidators(t, accountCount)
	allAccounts := testValidators.GetPublicIdentities()
	extra := createTestExtraObject(allAccounts, validator.AccountSet{}, accountCount-1, signaturesCount, signaturesCount)
	extra.Validators = nil

	newValidatorDelta := &validator.ValidatorSetDelta{
		// this will prompt an error since all the validators are already in the validator set
		Added: testValidators.ToValidatorSet().Accounts(),
	}

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra.MarshalRLPTo(nil)}

	/* START ALPHA CHANGE 2159 -- provide the Fill error result in the epoch fixture -- */
	blockBuilderMock := new(blockBuilderMock)
	blockBuilderMock.On("WriteTx", mock.Anything).Return(error(nil)).Once()
	blockBuilderMock.On("Reset").Return(error(nil)).Once()
	blockBuilderMock.On("Fill").Return(error(nil)).Once()
	/* FINISH ALPHA CHANGE 2159 -- provide the Fill error result in the epoch fixture -- */

	fsm := &fsm{parent: parent,
		blockBuilder:     blockBuilderMock,
		config:           testChainClockConfig(),
		isEndOfEpoch:     true,
		validators:       testValidators.ToValidatorSet(),
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		/* START ALPHA CHANGE 580 -- remove obsolete reward distribution input from duplicate validator test -- */
		/* FINISH ALPHA CHANGE 580 -- remove obsolete reward distribution input from duplicate validator test -- */
		/* START ALPHA CHANGE 366 -- remove final legacy bridge exit event root from FSM test -- */
		/* FINISH ALPHA CHANGE 366 -- remove final legacy bridge exit event root from FSM test -- */
		newValidatorsDelta: newValidatorDelta,
	}

	proposal, err := fsm.BuildProposal(0)
	assert.ErrorContains(t, err, "already present in the validators snapshot")
	assert.Nil(t, proposal)

	blockBuilderMock.AssertNotCalled(t, "Build")
	blockBuilderMock.AssertExpectations(t)
}

func TestFSM_VerifyStateTransactions_MiddleOfEpochWithTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{commitEpochInput: createTestCommitEpochInput(t, 0, 10), parent: &types.Header{}}
	tx, err := fsm.createCommitEpochTx()
	assert.NoError(t, err)
	err = fsm.VerifyStateTransactions([]*types.Transaction{tx})
	assert.ErrorContains(t, err, err.Error())
}

func TestFSM_VerifyStateTransactions_MiddleOfEpochWithoutTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{commitEpochInput: createTestCommitEpochInput(t, 0, 10), parent: &types.Header{}}
	err := fsm.VerifyStateTransactions([]*types.Transaction{})
	assert.NoError(t, err)
}

func TestFSM_VerifyStateTransactions_EndOfEpochWithoutTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{isEndOfEpoch: true, commitEpochInput: createTestCommitEpochInput(t, 0, 10)}
	assert.EqualError(t, fsm.VerifyStateTransactions([]*types.Transaction{}),
		"commit epoch transaction is not found in the epoch ending block")
}

func TestFSM_VerifyStateTransactions_EndOfEpochWrongCommitEpochTx(t *testing.T) {
	t.Parallel()

	fsm := &fsm{
		isEndOfEpoch:     true,
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		parent:           &types.Header{},
	}
	commitEpochInput, err := createTestCommitEpochInput(t, 1, 5).EncodeAbi()
	require.NoError(t, err)

	commitEpochTx := createStateTransactionWithData(1, contracts.ValidatorSetContract, commitEpochInput)
	assert.ErrorContains(t, fsm.VerifyStateTransactions([]*types.Transaction{commitEpochTx}), "invalid commit epoch transaction")
}

/* START ALPHA CHANGE 357 -- remove obsolete bridge commitment sprint test -- */
/* FINISH ALPHA CHANGE 357 -- remove obsolete bridge commitment sprint test -- */

func TestFSM_VerifyStateTransactions_EndOfEpochMoreThanOneCommitEpochTx(t *testing.T) {
	t.Parallel()

	txs := make([]*types.Transaction, 2)
	fsm := &fsm{
		isEndOfEpoch:     true,
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		parent:           &types.Header{},
	}

	commitEpochTxOne, err := fsm.createCommitEpochTx()
	require.NoError(t, err)

	txs[0] = commitEpochTxOne

	commitEpochTxTwo := createTestCommitEpochInput(t, 0, 100)
	input, err := commitEpochTxTwo.EncodeAbi()
	require.NoError(t, err)

	txs[1] = createStateTransactionWithData(1, types.ZeroAddress, input)

	assert.ErrorIs(t, fsm.VerifyStateTransactions(txs), errCommitEpochTxSingleExpected)
}

func TestFSM_VerifyStateTransactions_StateTransactionPass(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidators(t, 5)

	validatorSet := validator.NewValidatorSet(validators.GetPublicIdentities(), hclog.NewNullLogger())

	fsm := &fsm{
		parent:       &types.Header{Number: 1},
		isEndOfEpoch: true,
		/* START ALPHA CHANGE 356 -- remove legacy sprint flag from state transaction test -- */
		/* FINISH ALPHA CHANGE 356 -- remove legacy sprint flag from state transaction test -- */
		validators:       validatorSet,
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		/* START ALPHA CHANGE 581 -- remove obsolete reward distribution input from state transaction verification test -- */
		/* FINISH ALPHA CHANGE 581 -- remove obsolete reward distribution input from state transaction verification test -- */
		logger: hclog.NewNullLogger(),
	}

	// add commit epoch commitEpochTx to the transactions list
	commitEpochTx, err := fsm.createCommitEpochTx()
	require.NoError(t, err)

	/* START ALPHA CHANGE 582 -- verify epoch state transactions without obsolete reward distribution transaction -- */
	err = fsm.VerifyStateTransactions([]*types.Transaction{commitEpochTx})
	/* FINISH ALPHA CHANGE 582 -- verify epoch state transactions without obsolete reward distribution transaction -- */
	require.NoError(t, err)
}

/* START ALPHA CHANGE 358 -- remove obsolete bridge commitment quorum test -- */
/* FINISH ALPHA CHANGE 358 -- remove obsolete bridge commitment quorum test -- */

/* START ALPHA CHANGE 359 -- remove obsolete bridge commitment signature test -- */
/* FINISH ALPHA CHANGE 359 -- remove obsolete bridge commitment signature test -- */

func TestFSM_ValidateCommit_WrongValidator(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 5, 3, 3),
	}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData, parent.Timestamp)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: testChainClockConfig(), backend: &blockchainMock{},
		validators: validators.ToValidatorSet(), logger: hclog.NewNullLogger()}

	_, err := fsm.BuildProposal(0)
	require.NoError(t, err)

	err = fsm.ValidateCommit([]byte("0x7467674"), types.ZeroAddress.Bytes(), []byte{})
	require.ErrorContains(t, err, "unable to resolve validator")
}

func TestFSM_ValidateCommit_InvalidHash(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
	)

	validators := validator.NewTestValidators(t, accountsCount)

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 5, 3, 3),
	}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData, parent.Timestamp)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: testChainClockConfig(), backend: &blockchainMock{},
		validators: validators.ToValidatorSet(), logger: hclog.NewNullLogger()}

	_, err := fsm.BuildProposal(0)
	require.NoError(t, err)

	nonValidatorAcc := validator.NewTestValidator(t, "non_validator", 1)
	wrongSignature, err := nonValidatorAcc.MustSign([]byte("Foo"), signer.DomainCheckpointManager).Marshal()
	require.NoError(t, err)

	err = fsm.ValidateCommit(validators.GetValidator("0").Address().Bytes(), wrongSignature, []byte{})
	require.ErrorContains(t, err, "incorrect commit signature from")
}

func TestFSM_ValidateCommit_Good(t *testing.T) {
	t.Parallel()

	const parentBlockNumber = 10

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"})
	validatorsMetadata := validators.GetPublicIdentities()

	parent := &types.Header{Number: parentBlockNumber, ExtraData: createTestExtra(validatorsMetadata, validator.AccountSet{}, 5, 3, 3)}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData, parent.Timestamp)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	validatorSet := validator.NewValidatorSet(validatorsMetadata, hclog.NewNullLogger())

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: testChainClockConfig(), backend: &blockchainMock{},
		validators: validatorSet,
		logger:     hclog.NewNullLogger()}

	proposal, err := fsm.BuildProposal(0)
	require.NoError(t, err)

	block := types.Block{}
	require.NoError(t, block.UnmarshalRLP(proposal))

	validator := validators.GetValidator("A")
	seal, err := validator.MustSign(block.Hash().Bytes(), signer.DomainCheckpointManager).Marshal()
	require.NoError(t, err)
	err = fsm.ValidateCommit(validator.Key().Address().Bytes(), seal, block.Hash().Bytes())
	require.NoError(t, err)
}

func TestFSM_Validate_ExitEventRootNotExpected(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parentExtra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	parentExtra.Validators = nil

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: parentExtra.MarshalRLPTo(nil),
	}
	parent.ComputeHash()

	innbcbftBackendMock := new(innbcbftBackendMock)
	innbcbftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.GetPublicIdentities(), nil).Once()

	extra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	extra.Validators = nil
	parentCheckpointHash, err := extra.Checkpoint.Hash(0, parentBlockNumber, parent.Hash)
	require.NoError(t, err)

	currentValSetHash, err := validators.GetPublicIdentities().Hash()
	require.NoError(t, err)

	extra.Parent = createSignature(t, validators.GetPrivateIdentities(), parentCheckpointHash, signer.DomainCheckpointManager)
	extra.Checkpoint.EpochNumber = 1
	extra.Checkpoint.CurrentValidatorsHash = currentValSetHash
	extra.Checkpoint.NextValidatorsHash = currentValSetHash

	stateBlock := createDummyStateBlock(parent.Number+1, types.Hash{100, 15}, extra.MarshalRLPTo(nil), parent.Timestamp)

	proposalHash, err := extra.Checkpoint.Hash(0, stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)

	stateBlock.Block.Header.Hash = proposalHash
	stateBlock.Block.Header.ParentHash = parent.Hash
	/* START ALPHA CHANGE 3320 -- retain a valid timestamp before exercising later validation -- */
	stateBlock.Block.Header.Timestamp = parent.Timestamp + 1
	/* FINISH ALPHA CHANGE 3320 -- retain a valid timestamp before exercising later validation -- */
	stateBlock.Block.Transactions = []*types.Transaction{}

	proposal := stateBlock.Block.MarshalRLP()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("ProcessBlock", mock.Anything, mock.Anything).Return(stateBlock, nil).Once()

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
	require.NoError(t, err)

	innbcbftBackendMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestFSM_Validate_EpochEndingBlock_MismatchInDeltas(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parentExtra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	parentExtra.Validators = nil

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: parentExtra.MarshalRLPTo(nil),
	}
	parent.ComputeHash()

	innbcbftBackendMock := new(innbcbftBackendMock)
	innbcbftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.GetPublicIdentities(), nil).Once()

	extra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	parentCheckpointHash, err := extra.Checkpoint.Hash(0, parentBlockNumber, parent.Hash)
	require.NoError(t, err)

	extra.Validators = &validator.ValidatorSetDelta{} // this will cause test to fail
	extra.Parent = createSignature(t, validators.GetPrivateIdentities(), parentCheckpointHash, signer.DomainCheckpointManager)

	stateBlock := createDummyStateBlock(parent.Number+1, types.Hash{100, 15}, extra.MarshalRLPTo(nil), parent.Timestamp)

	proposalHash, err := new(CheckpointData).Hash(0, stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)

	commitEpoch := createTestCommitEpochInput(t, 1, 10)
	commitEpochTxInput, err := commitEpoch.EncodeAbi()
	require.NoError(t, err)

	/* START ALPHA CHANGE 583 -- remove obsolete reward distribution tx input from proposal validation test -- */
	/* FINISH ALPHA CHANGE 583 -- remove obsolete reward distribution tx input from proposal validation test -- */

	stateBlock.Block.Header.Hash = proposalHash
	stateBlock.Block.Header.ParentHash = parent.Hash
	/* START ALPHA CHANGE 3320 -- retain a valid timestamp before exercising later validation -- */
	stateBlock.Block.Header.Timestamp = parent.Timestamp + 1
	/* FINISH ALPHA CHANGE 3320 -- retain a valid timestamp before exercising later validation -- */
	stateBlock.Block.Transactions = []*types.Transaction{
		createStateTransactionWithData(1, contracts.ValidatorSetContract, commitEpochTxInput),
		/* START ALPHA CHANGE 584 -- remove obsolete reward distribution state transaction from proposal validation test -- */
		/* FINISH ALPHA CHANGE 584 -- remove obsolete reward distribution state transaction from proposal validation test -- */
	}

	proposal := stateBlock.Block.MarshalRLP()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("ProcessBlock", mock.Anything, mock.Anything).
		Return(stateBlock, error(nil)).
		Maybe()

	// a new validator is added to delta which proposers block does not have
	privateKey, err := bls.GenerateBlsKey()
	require.NoError(t, err)

	newValidatorDelta := &validator.ValidatorSetDelta{
		Added: validator.AccountSet{&validator.ValidatorMetadata{
			Address:     types.BytesToAddress([]byte{0, 1, 2, 3}),
			BlsKey:      privateKey.PublicKey(),
			VotingPower: new(big.Int).SetUint64(1),
			IsActive:    true,
		}},
	}

	fsm := &fsm{
		parent:           parent,
		backend:          blockchainMock,
		validators:       validators.ToValidatorSet(),
		logger:           hclog.NewNullLogger(),
		isEndOfEpoch:     true,
		commitEpochInput: commitEpoch,
		/* START ALPHA CHANGE 585 -- remove obsolete reward distribution input from proposal validation FSM fixture -- */
		/* FINISH ALPHA CHANGE 585 -- remove obsolete reward distribution input from proposal validation FSM fixture -- */
		innbcbftBackend:    innbcbftBackendMock,
		newValidatorsDelta: newValidatorDelta,
		config:             testChainClockConfig(),
	}

	err = fsm.Validate(proposal)
	require.ErrorIs(t, err, errValidatorSetDeltaMismatch)

	innbcbftBackendMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestFSM_Validate_EpochEndingBlock_UpdatingValidatorSetInNonEpochEndingBlock(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parentExtra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	parentExtra.Validators = nil

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: parentExtra.MarshalRLPTo(nil),
	}
	parent.ComputeHash()

	innbcbftBackendMock := new(innbcbftBackendMock)
	innbcbftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.GetPublicIdentities(), nil).Once()

	// a new validator is added to delta which proposers block does not have
	privateKey, err := bls.GenerateBlsKey()
	require.NoError(t, err)

	newValidatorDelta := &validator.ValidatorSetDelta{
		Added: validator.AccountSet{&validator.ValidatorMetadata{
			Address:     types.BytesToAddress([]byte{0, 1, 2, 3}),
			BlsKey:      privateKey.PublicKey(),
			VotingPower: new(big.Int).SetUint64(1),
			IsActive:    true,
		}},
	}

	extra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	parentCheckpointHash, err := extra.Checkpoint.Hash(0, parentBlockNumber, parent.Hash)
	require.NoError(t, err)

	extra.Validators = newValidatorDelta // this will cause test to fail
	extra.Parent = createSignature(t, validators.GetPrivateIdentities(), parentCheckpointHash, signer.DomainCheckpointManager)

	stateBlock := createDummyStateBlock(parent.Number+1, types.Hash{100, 15}, extra.MarshalRLPTo(nil), parent.Timestamp)

	proposalHash, err := new(CheckpointData).Hash(0, stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)

	stateBlock.Block.Header.Hash = proposalHash
	stateBlock.Block.Header.ParentHash = parent.Hash
	/* START ALPHA CHANGE 3320 -- retain a valid timestamp before exercising later validation -- */
	stateBlock.Block.Header.Timestamp = parent.Timestamp + 1
	/* FINISH ALPHA CHANGE 3320 -- retain a valid timestamp before exercising later validation -- */

	proposal := stateBlock.Block.MarshalRLP()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("ProcessBlock", mock.Anything, mock.Anything).
		Return(stateBlock, error(nil)).
		Maybe()

	fsm := &fsm{
		parent:          parent,
		backend:         blockchainMock,
		validators:      validators.ToValidatorSet(),
		logger:          hclog.NewNullLogger(),
		innbcbftBackend: innbcbftBackendMock,
		config:          testChainClockConfig(),
	}

	err = fsm.Validate(proposal)
	require.ErrorIs(t, err, errValidatorsUpdateInNonEpochEnding)

	innbcbftBackendMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestFSM_Validate_IncorrectHeaderParentHash(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount),
	}
	parent.ComputeHash()

	fsm := &fsm{
		parent:     parent,
		backend:    &blockchainMock{},
		validators: validators.ToValidatorSet(),
		logger:     hclog.NewNullLogger(),
		config: &INNBCBFTConfig{
			/* START ALPHA CHANGE 3320 -- retain the explicit fixture interval -- */
			BlockTime: testChainClockConfig().BlockTime,
			/* FINISH ALPHA CHANGE 3320 -- retain the explicit fixture interval -- */
		},
	}

	stateBlock := createDummyStateBlock(parent.Number+1, types.Hash{100, 15}, parent.ExtraData, parent.Timestamp)

	hash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)

	stateBlock.Block.Header.Hash = hash
	proposal := stateBlock.Block.MarshalRLP()

	err = fsm.Validate(proposal)
	require.ErrorContains(t, err, "incorrect header parent hash")
}

func TestFSM_Validate_InvalidNumber(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount),
	}
	parent.ComputeHash()

	// try some invalid block numbers, parentBlockNumber + 1 should be correct
	for _, blockNum := range []uint64{parentBlockNumber - 1, parentBlockNumber, parentBlockNumber + 2} {
		stateBlock := createDummyStateBlock(blockNum, parent.Hash, parent.ExtraData, parent.Timestamp)
		mBlockBuilder := newBlockBuilderMock(stateBlock)
		fsm := &fsm{
			parent:       parent,
			blockBuilder: mBlockBuilder,
			backend:      &blockchainMock{},
			validators:   validators.ToValidatorSet(),
			logger:       hclog.NewNullLogger(),
			config:       testChainClockConfig(),
		}

		proposalHash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), stateBlock.Block.Number(), stateBlock.Block.Hash())
		require.NoError(t, err)

		stateBlock.Block.Header.Hash = proposalHash
		proposal := stateBlock.Block.MarshalRLP()

		err = fsm.Validate(proposal)
		require.ErrorContains(t, err, "invalid number")
	}
}

func TestFSM_Validate_TimestampOlder(t *testing.T) {
	t.Parallel()

	const parentBlockNumber = 10

	validators := validator.NewTestValidators(t, 5)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 4, 3, 3),
		Timestamp: uint64(time.Now().UTC().Unix()),
	}
	parent.ComputeHash()

	// try some invalid times
	for _, blockTime := range []uint64{parent.Timestamp - 1, parent.Timestamp} {
		header := &types.Header{
			Number:     parentBlockNumber + 1,
			ParentHash: parent.Hash,
			Timestamp:  blockTime,
			ExtraData:  parent.ExtraData,
		}
		stateBlock := &types.FullBlock{Block: consensus.BuildBlock(consensus.BuildBlockParams{Header: header})}
		fsm := &fsm{
			parent:     parent,
			backend:    &blockchainMock{},
			validators: validators.ToValidatorSet(),
			logger:     hclog.NewNullLogger(),
			config: &INNBCBFTConfig{
				/* START ALPHA CHANGE 3320 -- retain the explicit fixture interval -- */
				BlockTime: testChainClockConfig().BlockTime,
				/* FINISH ALPHA CHANGE 3320 -- retain the explicit fixture interval -- */
			}}

		checkpointHash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), header.Number, header.Hash)
		require.NoError(t, err)

		stateBlock.Block.Header.Hash = checkpointHash
		proposal := stateBlock.Block.MarshalRLP()

		err = fsm.Validate(proposal)
		/* START ALPHA CHANGE 3320 -- reject equal or decreasing timestamps through exact equality -- */
		assert.ErrorContains(t, err, "invalid block timestamp")
		/* FINISH ALPHA CHANGE 3320 -- reject equal or decreasing timestamps through exact equality -- */
	}
}

func TestFSM_Validate_IncorrectMixHash(t *testing.T) {
	t.Parallel()

	const parentBlockNumber = 10

	validators := validator.NewTestValidators(t, 5)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 4, 3, 3),
		Timestamp: uint64(100),
	}
	parent.ComputeHash()

	header := &types.Header{
		Number:     parentBlockNumber + 1,
		ParentHash: parent.Hash,
		Timestamp:  parent.Timestamp + 1,
		MixHash:    types.Hash{},
		ExtraData:  parent.ExtraData,
	}

	buildBlock := &types.FullBlock{Block: consensus.BuildBlock(consensus.BuildBlockParams{Header: header})}

	fsm := &fsm{
		parent:     parent,
		backend:    &blockchainMock{},
		validators: validators.ToValidatorSet(),
		logger:     hclog.NewNullLogger(),
		config: &INNBCBFTConfig{
			/* START ALPHA CHANGE 3320 -- retain the explicit fixture interval -- */
			BlockTime: testChainClockConfig().BlockTime,
			/* FINISH ALPHA CHANGE 3320 -- retain the explicit fixture interval -- */
		},
	}
	rlpBlock := buildBlock.Block.MarshalRLP()

	_, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), header.Number, header.Hash)
	require.NoError(t, err)

	err = fsm.Validate(rlpBlock)
	assert.ErrorContains(t, err, "mix digest is not correct")
}

func TestFSM_Insert_Good(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		parentBlockNumber = uint64(10)
		signaturesCount   = 3
	)

	setupFn := func() (*fsm, []*messages.CommittedSeal, *types.FullBlock, *blockchainMock) {
		validators := validator.NewTestValidators(t, accountCount)
		allAccounts := validators.GetPrivateIdentities()
		validatorsMetadata := validators.GetPublicIdentities()

		extraParent := createTestExtra(validatorsMetadata, validator.AccountSet{}, len(allAccounts)-1, signaturesCount, signaturesCount)
		parent := &types.Header{Number: parentBlockNumber, ExtraData: extraParent}
		extraBlock := createTestExtra(validatorsMetadata, validator.AccountSet{}, len(allAccounts)-1, signaturesCount, signaturesCount)
		block := consensus.BuildBlock(
			consensus.BuildBlockParams{
				Header: &types.Header{Number: parentBlockNumber + 1, ParentHash: parent.Hash, ExtraData: extraBlock},
			})

		builtBlock := &types.FullBlock{Block: block}

		builderMock := newBlockBuilderMock(builtBlock)
		chainMock := &blockchainMock{}
		chainMock.On("CommitBlock", mock.Anything).Return(error(nil)).Once()
		chainMock.On("ProcessBlock", mock.Anything, mock.Anything).
			Return(builtBlock, error(nil)).
			Maybe()

		f := &fsm{
			parent:       parent,
			blockBuilder: builderMock,
			target:       builtBlock,
			backend:      chainMock,
			validators:   validator.NewValidatorSet(validatorsMetadata[0:len(validatorsMetadata)-1], hclog.NewNullLogger()),
			logger:       hclog.NewNullLogger(),
		}

		seals := make([]*messages.CommittedSeal, signaturesCount)

		for i := 0; i < signaturesCount; i++ {
			sign, err := allAccounts[i].Bls.Sign(builtBlock.Block.Hash().Bytes(), signer.DomainCheckpointManager)
			require.NoError(t, err)
			sigRaw, err := sign.Marshal()
			require.NoError(t, err)

			seals[i] = &messages.CommittedSeal{
				Signer:    validatorsMetadata[i].Address.Bytes(),
				Signature: sigRaw,
			}
		}

		return f, seals, builtBlock, chainMock
	}

	t.Run("Insert with target block defined", func(t *testing.T) {
		t.Parallel()

		fsm, seals, builtBlock, chainMock := setupFn()
		proposal := builtBlock.Block.MarshalRLP()
		fullBlock, err := fsm.Insert(proposal, seals)

		require.NoError(t, err)
		require.Equal(t, parentBlockNumber+1, fullBlock.Block.Number())
		chainMock.AssertExpectations(t)
	})

	t.Run("Insert with target block undefined", func(t *testing.T) {
		t.Parallel()

		fsm, seals, builtBlock, _ := setupFn()
		fsm.target = nil
		proposal := builtBlock.Block.MarshalRLP()
		_, err := fsm.Insert(proposal, seals)

		require.ErrorIs(t, err, errProposalDontMatch)
	})

	t.Run("Insert with target block hash not match", func(t *testing.T) {
		t.Parallel()

		fsm, seals, builtBlock, _ := setupFn()
		proposal := builtBlock.Block.MarshalRLP()
		fsm.target = builtBlock
		fsm.target.Block.Header.Hash = types.BytesToHash(generateRandomBytes(t))
		_, err := fsm.Insert(proposal, seals)

		require.ErrorIs(t, err, errProposalDontMatch)
	})
}

func TestFSM_Insert_InvalidNode(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		parentBlockNumber = 10
		signaturesCount   = 3
	)

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"})
	validatorsMetadata := validators.GetPublicIdentities()

	parent := &types.Header{Number: parentBlockNumber}
	parent.ComputeHash()

	extraBlock := createTestExtra(validatorsMetadata, validator.AccountSet{}, len(validators.Validators)-1, signaturesCount, signaturesCount)
	finalBlock := consensus.BuildBlock(
		consensus.BuildBlockParams{
			Header: &types.Header{Number: parentBlockNumber + 1, ParentHash: parent.Hash, ExtraData: extraBlock},
		})

	buildBlock := &types.FullBlock{Block: finalBlock, Receipts: []*types.Receipt{}}
	mBlockBuilder := newBlockBuilderMock(buildBlock)

	validatorSet := validator.NewValidatorSet(validatorsMetadata[0:len(validatorsMetadata)-1], hclog.NewNullLogger())

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, backend: &blockchainMock{},
		validators: validatorSet,
	}

	proposal := buildBlock.Block.MarshalRLP()
	validatorA := validators.GetValidator("A")
	validatorB := validators.GetValidator("B")
	proposalHash := buildBlock.Block.Hash().Bytes()
	sigA, err := validatorA.MustSign(proposalHash, signer.DomainCheckpointManager).Marshal()
	require.NoError(t, err)

	sigB, err := validatorB.MustSign(proposalHash, signer.DomainCheckpointManager).Marshal()
	require.NoError(t, err)

	// create test account outside of validator set
	nonValidatorAccount := validator.NewTestValidator(t, "non_validator", 1)
	nonValidatorSignature, err := nonValidatorAccount.MustSign(proposalHash, signer.DomainCheckpointManager).Marshal()
	require.NoError(t, err)

	commitedSeals := []*messages.CommittedSeal{
		{Signer: validatorA.Address().Bytes(), Signature: sigA},
		{Signer: validatorB.Address().Bytes(), Signature: sigB},
		{Signer: nonValidatorAccount.Address().Bytes(), Signature: nonValidatorSignature}, // this one should fail
	}

	fsm.target = buildBlock

	_, err = fsm.Insert(proposal, commitedSeals)
	assert.ErrorContains(t, err, "invalid node id")
}

func TestFSM_Height(t *testing.T) {
	t.Parallel()

	parentNumber := uint64(3)
	parent := &types.Header{Number: parentNumber}
	fsm := &fsm{parent: parent}
	assert.Equal(t, parentNumber+1, fsm.Height())
}

/* START ALPHA CHANGE 360 -- remove obsolete bridge commitment decode test -- */
/* FINISH ALPHA CHANGE 360 -- remove obsolete bridge commitment decode test -- */

func TestFSM_DecodeCommitEpochStateTx(t *testing.T) {
	t.Parallel()

	commitEpoch := createTestCommitEpochInput(t, 0, 10)
	input, err := commitEpoch.EncodeAbi()
	require.NoError(t, err)
	require.NotNil(t, input)

	tx := createStateTransactionWithData(1, contracts.ValidatorSetContract, input)
	decodedInputData, err := decodeStateTransaction(tx.Input)
	require.NoError(t, err)

	decodedCommitEpoch, ok := decodedInputData.(*contractsapi.CommitEpochValidatorSetFn)
	require.True(t, ok)
	require.True(t, commitEpoch.ID.Cmp(decodedCommitEpoch.ID) == 0)
	require.NotNil(t, decodedCommitEpoch.Epoch)
	require.True(t, commitEpoch.Epoch.StartBlock.Cmp(decodedCommitEpoch.Epoch.StartBlock) == 0)
	require.True(t, commitEpoch.Epoch.EndBlock.Cmp(decodedCommitEpoch.Epoch.EndBlock) == 0)
}

/* START ALPHA CHANGE 365 -- remove obsolete bridge mixed state transaction test -- */
/* FINISH ALPHA CHANGE 365 -- remove obsolete bridge mixed state transaction test -- */

/* START ALPHA CHANGE 361 -- remove obsolete bridge state transaction type test -- */
/* FINISH ALPHA CHANGE 361 -- remove obsolete bridge state transaction type test -- */

/* START ALPHA CHANGE 362 -- remove obsolete bridge state transaction quorum test -- */
/* FINISH ALPHA CHANGE 362 -- remove obsolete bridge state transaction quorum test -- */

/* START ALPHA CHANGE 363 -- remove obsolete bridge state transaction signature test -- */
/* FINISH ALPHA CHANGE 363 -- remove obsolete bridge state transaction signature test -- */

/* START ALPHA CHANGE 364 -- remove obsolete duplicate bridge commitment test -- */
/* FINISH ALPHA CHANGE 364 -- remove obsolete duplicate bridge commitment test -- */

func TestFSM_Validate_FailToVerifySignatures(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	validatorsMetadata := validators.GetPublicIdentities()

	extra := createTestExtraObject(validatorsMetadata, validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	validatorsHash, err := validatorsMetadata.Hash()
	require.NoError(t, err)

	extra.Checkpoint = &CheckpointData{CurrentValidatorsHash: validatorsHash, NextValidatorsHash: validatorsHash}
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: extra.MarshalRLPTo(nil),
	}
	parent.ComputeHash()

	innbcbftBackendMock := new(innbcbftBackendMock)
	innbcbftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validatorsMetadata, nil).Once()

	validatorSet := validator.NewValidatorSet(validatorsMetadata, hclog.NewNullLogger())

	fsm := &fsm{
		parent:          parent,
		backend:         &blockchainMock{},
		innbcbftBackend: innbcbftBackendMock,
		validators:      validatorSet,
		logger:          hclog.NewNullLogger(),
		config: &INNBCBFTConfig{
			/* START ALPHA CHANGE 3320 -- retain the explicit fixture interval -- */
			BlockTime: testChainClockConfig().BlockTime,
			/* FINISH ALPHA CHANGE 3320 -- retain the explicit fixture interval -- */
		},
	}

	finalBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{
			Number:     parentBlockNumber + 1,
			ParentHash: parent.Hash,
			Timestamp:  parent.Timestamp + 1,
			MixHash:    INNBCBFTMixDigest,
			Difficulty: 1,
			ExtraData:  parent.ExtraData,
		},
	})

	checkpointHash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), finalBlock.Number(), finalBlock.Hash())
	require.NoError(t, err)

	finalBlock.Header.Hash = checkpointHash
	proposal := finalBlock.MarshalRLP()

	assert.ErrorContains(t, fsm.Validate(proposal), "failed to verify signatures")

	innbcbftBackendMock.AssertExpectations(t)
}

/* START ALPHA CHANGE 2577 -- test live checkpoint epochs and rejection before proposal execution -- */
func TestFSM_CheckpointEpochContext(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidators(t, 5)
	accounts := validators.GetPublicIdentities()
	currentHash, err := accounts.Hash()
	require.NoError(t, err)

	for _, tc := range []struct {
		name          string
		parentNumber  uint64
		parentEpoch   uint64
		expectedEpoch uint64
		proposedEpoch uint64
		ending        bool
		changed       bool
		embeddedRound uint64
		proposalRound uint64
		accepted      bool
	}{
		{"first live block", 0, 0, 1, 1, false, false, 0, 0, true},
		{"ordinary block", 65, 7, 7, 7, false, false, 0, 0, true},
		{"epoch end without validator changes", 69, 7, 7, 7, true, false, 0, 0, true},
		{"epoch end with validator changes", 69, 7, 7, 7, true, true, 0, 0, true},
		{"first block after epoch end", 70, 7, 8, 8, false, false, 0, 0, true},
		{"prepared block reused in a later round", 65, 7, 7, 7, false, false, 2, 7, true},
		{"premature epoch advance", 65, 7, 7, 8, false, false, 0, 0, false},
		{"old epoch after epoch end", 70, 7, 8, 7, false, false, 0, 0, false},
		{"epoch zero in block one", 0, 0, 1, 0, false, false, 0, 0, false},
		{"epoch end advances checkpoint too early", 69, 7, 7, 8, true, false, 0, 0, false},
		{"maximum unsigned epoch", 65, 7, 7, ^uint64(0), false, false, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parentExtra := &Extra{
				Checkpoint: &CheckpointData{
					EpochNumber:           tc.parentEpoch,
					CurrentValidatorsHash: currentHash,
					NextValidatorsHash:    currentHash,
				},
			}
			if tc.parentNumber == 0 {
				parentExtra = &Extra{
					Validators: &validator.ValidatorSetDelta{Added: accounts},
					Checkpoint: &CheckpointData{},
				}
			}

			parent := &types.Header{
				Number:    tc.parentNumber,
				Timestamp: tc.parentNumber,
				ExtraData: parentExtra.MarshalRLPTo(nil),
			}
			parent.ComputeHash()

			var delta *validator.ValidatorSetDelta
			nextAccounts := accounts
			if tc.ending {
				delta = &validator.ValidatorSetDelta{}
			}
			if tc.changed {
				nextAccounts = accounts[:4]
				delta, err = validator.CreateValidatorSetDelta(accounts, nextAccounts)
				require.NoError(t, err)
			}

			nextHash, err := nextAccounts.Hash()
			require.NoError(t, err)

			extra := &Extra{
				Validators: delta,
				Checkpoint: &CheckpointData{
					EpochNumber:           tc.proposedEpoch,
					BlockRound:            tc.embeddedRound,
					CurrentValidatorsHash: currentHash,
					NextValidatorsHash:    nextHash,
					EventRoot:             types.ZeroHash,
				},
			}
			if parent.Number > 0 {
				parentHash, err := parentExtra.Checkpoint.Hash(0, parent.Number, parent.Hash)
				require.NoError(t, err)

				extra.Parent = createSignature(
					t, validators.GetPrivateIdentities(), parentHash, signer.DomainCheckpointManager,
				)
			}

			chain := new(blockchainMock)
			consensusBackend := new(innbcbftBackendMock)
			f := &fsm{
				parent:             parent,
				backend:            chain,
				innbcbftBackend:    consensusBackend,
				validators:         validators.ToValidatorSet(),
				epochNumber:        tc.expectedEpoch,
				isEndOfEpoch:       tc.ending,
				newValidatorsDelta: delta,
				config:             testChainClockConfig(),
				logger:             hclog.NewNullLogger(),
			}

			var txs []*types.Transaction
			if tc.ending {
				f.commitEpochInput = createTestCommitEpochInput(t, tc.expectedEpoch, 10)
				tx, err := f.createCommitEpochTx()
				require.NoError(t, err)
				txs = []*types.Transaction{tx}
			}

			block := consensus.BuildBlock(consensus.BuildBlockParams{
				Header: &types.Header{
					Number:     parent.Number + 1,
					ParentHash: parent.Hash,
					Timestamp:  parent.Timestamp + 1,
					Difficulty: 1,
					MixHash:    INNBCBFTMixDigest,
					ExtraData:  extra.MarshalRLPTo(nil),
				},
				Txns: txs,
			})
			stateBlock := &types.FullBlock{Block: block}
			raw := block.MarshalRLP()

			runtime := &consensusRuntime{
				fsm:    f,
				logger: hclog.NewNullLogger(),
				config: &runtimeConfig{blockchain: chain},
			}
			proposalHash, err := extra.Checkpoint.Hash(0, block.Number(), block.Hash())
			require.NoError(t, err)

			/* A correctly matching hash does not establish that the epoch is valid. */
			require.True(t, runtime.IsValidProposalHash(
				&proto.Proposal{RawProposal: raw, Round: tc.proposalRound},
				proposalHash.Bytes(),
			))

			if tc.accepted {
				if parent.Number > 0 {
					consensusBackend.On("GetValidators", parent.Number-1, mock.Anything).
						Return(accounts, nil).Once()
				}
				chain.On("ProcessBlock", parent, mock.Anything).
					Return(stateBlock, nil).Once()

				require.NoError(t, f.Validate(raw))
				require.Same(t, stateBlock, f.target)
			} else {
				require.ErrorContains(t, f.Validate(raw), "invalid checkpoint epoch")
				require.False(t, runtime.IsValidProposal(raw))
				require.Nil(t, f.target)
				chain.AssertNotCalled(t, "ProcessBlock", mock.Anything, mock.Anything)
				consensusBackend.AssertNotCalled(t, "GetValidators", mock.Anything, mock.Anything)
			}

			chain.AssertExpectations(t)
			consensusBackend.AssertExpectations(t)
		})
	}
}

/* FINISH ALPHA CHANGE 2577 -- test live checkpoint epochs and rejection before proposal execution -- */

/* START ALPHA CHANGE 3320 -- construct test proposals from explicit parent timestamps -- */
func testChainClockConfig() *INNBCBFTConfig {
	return &INNBCBFTConfig{
		BlockTime: common.Duration{Duration: time.Second},
	}
}

func createDummyStateBlock(
	blockNumber uint64,
	parentHash types.Hash,
	extraData []byte,
	parentTimestamp uint64,
) *types.FullBlock {
	/* FINISH ALPHA CHANGE 3320 -- construct test proposals from explicit parent timestamps -- */
	finalBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{
			Number:     blockNumber,
			ParentHash: parentHash,
			/* START ALPHA CHANGE 3320 -- advance the fixture's one-second chain clock -- */
			Timestamp: parentTimestamp + 1,
			/* FINISH ALPHA CHANGE 3320 -- advance the fixture's one-second chain clock -- */
			Difficulty: 1,
			ExtraData:  extraData,
			MixHash:    INNBCBFTMixDigest,
		},
	})

	return &types.FullBlock{Block: finalBlock}
}

func createTestExtra(
	allAccounts,
	previousValidatorSet validator.AccountSet,
	validatorsCount,
	committedSignaturesCount,
	parentSignaturesCount int,
) []byte {
	extraData := createTestExtraObject(allAccounts, previousValidatorSet, validatorsCount, committedSignaturesCount, parentSignaturesCount)

	return extraData.MarshalRLPTo(nil)
}

/* START ALPHA CHANGE 367 -- remove obsolete bridge commitment test helper -- */
/* FINISH ALPHA CHANGE 367 -- remove obsolete bridge commitment test helper -- */

/* START ALPHA CHANGE 2159 -- provide the Fill error result in existing FSM expectations -- */
func newBlockBuilderMock(stateBlock *types.FullBlock) *blockBuilderMock {
	mBlockBuilder := new(blockBuilderMock)
	mBlockBuilder.On("Build", mock.Anything).Return(stateBlock).Once()
	mBlockBuilder.On("Fill").Return(error(nil)).Once()
	mBlockBuilder.On("Reset", mock.Anything).Return(error(nil)).Once()

	return mBlockBuilder
}

/* FINISH ALPHA CHANGE 2159 -- provide the Fill error result in existing FSM expectations -- */

func createTestExtraObject(allAccounts,
	previousValidatorSet validator.AccountSet,
	validatorsCount,
	committedSignaturesCount,
	parentSignaturesCount int) *Extra {
	accountCount := len(allAccounts)
	dummySignature := [64]byte{}
	bitmapCommitted, bitmapParent := bitmap.Bitmap{}, bitmap.Bitmap{}
	extraData := &Extra{}
	extraData.Validators = generateValidatorDelta(validatorsCount, allAccounts, previousValidatorSet)

	for j := range rand.Perm(accountCount)[:committedSignaturesCount] {
		bitmapCommitted.Set(uint64(j))
	}

	for j := range rand.Perm(accountCount)[:parentSignaturesCount] {
		bitmapParent.Set(uint64(j))
	}

	extraData.Parent = &Signature{Bitmap: bitmapCommitted, AggregatedSignature: dummySignature[:]}
	extraData.Committed = &Signature{Bitmap: bitmapParent, AggregatedSignature: dummySignature[:]}
	extraData.Checkpoint = &CheckpointData{}

	return extraData
}

func generateValidatorDelta(validatorCount int, allAccounts, previousValidatorSet validator.AccountSet) (vd *validator.ValidatorSetDelta) {
	oldMap := make(map[types.Address]int, previousValidatorSet.Len())
	for i, x := range previousValidatorSet {
		oldMap[x.Address] = i
	}

	vd = &validator.ValidatorSetDelta{}
	vd.Removed = bitmap.Bitmap{}

	for _, id := range rand.Perm(len(allAccounts))[:validatorCount] {
		_, exists := oldMap[allAccounts[id].Address]
		if !exists {
			vd.Added = append(vd.Added, allAccounts[id])
		}

		delete(oldMap, allAccounts[id].Address)
	}

	for _, v := range oldMap {
		vd.Removed.Set(uint64(v))
	}

	return
}
