package innbcbft

import (
	"fmt"
	"math/big"
	"math/rand"

	/* START ALPHA CHANGE 2634 -- coordinate concurrent proposer checks and publication regressions -- */
	"sync"
	/* FINISH ALPHA CHANGE 2634 -- coordinate concurrent proposer checks and publication regressions -- */

	/* START ALPHA CHANGE 1549 -- remove obsolete global BoltDB test-file import -- */
	/* FINISH ALPHA CHANGE 1549 -- remove obsolete global BoltDB test-file import -- */
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/consensus"

	/* START ALPHA CHANGE 1874 -- test with INNBC-owned IBFT wire types -- */
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	/* FINISH ALPHA CHANGE 1874 -- test with INNBC-owned IBFT wire types -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/bitmap"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func testValidatorRegistryConfig(genesisValidator types.Address) *ValidatorRegistryConfig {
	/* START ALPHA CHANGE 1914 -- keep test registry configuration free of fixed system addresses -- */
	return &ValidatorRegistryConfig{
		GenesisValidator:       genesisValidator,
		MinimumStake:           big.NewInt(1),
		ProposalDurationPreset: ValidatorRegistryProposalDurationStandard,
		ActivationEpoch:        1,
	}
	/* FINISH ALPHA CHANGE 1914 -- keep test registry configuration free of fixed system addresses -- */
}

func TestConsensusRuntime_isFixedSizeOfEpochMet_NotReachedEnd(t *testing.T) {
	t.Parallel()

	// because of slashing, we can assume some epochs started at random numbers
	var cases = []struct {
		epochSize, firstBlockInEpoch, parentBlockNumber uint64
	}{
		{4, 1, 2},
		{5, 1, 3},
		{6, 0, 6},
		{7, 0, 4},
		{8, 0, 5},
		{9, 4, 9},
		{10, 7, 10},
		{10, 1, 1},
	}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			INNBCBFTConfig: &INNBCBFTConfig{},
		},
		lastBuiltBlock: &types.Header{},
		epoch:          &epochMetadata{},
	}

	for _, c := range cases {
		runtime.config.INNBCBFTConfig.EpochSize = c.epochSize
		runtime.epoch.FirstBlockInEpoch = c.firstBlockInEpoch
		assert.False(
			t,
			runtime.isFixedSizeOfEpochMet(c.parentBlockNumber+1, runtime.epoch),
			fmt.Sprintf(
				"Not expected end of epoch for epoch size=%v and parent block number=%v",
				c.epochSize,
				c.parentBlockNumber),
		)
	}
}

func TestConsensusRuntime_isFixedSizeOfEpochMet_ReachedEnd(t *testing.T) {
	t.Parallel()

	// because of slashing, we can assume some epochs started at random numbers
	var cases = []struct {
		epochSize, firstBlockInEpoch, blockNumber uint64
	}{
		{4, 1, 4},
		{5, 1, 5},
		{6, 0, 5},
		{7, 0, 6},
		{8, 0, 7},
		{9, 4, 12},
		{10, 7, 16},
		{10, 1, 10},
	}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			INNBCBFTConfig: &INNBCBFTConfig{},
		},
		epoch: &epochMetadata{},
	}

	for _, c := range cases {
		runtime.config.INNBCBFTConfig.EpochSize = c.epochSize
		runtime.epoch.FirstBlockInEpoch = c.firstBlockInEpoch
		assert.True(
			t,
			runtime.isFixedSizeOfEpochMet(c.blockNumber, runtime.epoch),
			fmt.Sprintf(
				"Not expected end of epoch for epoch size=%v and parent block number=%v",
				c.epochSize,
				c.blockNumber),
		)
	}
}

func TestConsensusRuntime_isFixedSizeOfSprintMet_NotReachedEnd(t *testing.T) {
	t.Parallel()

	// because of slashing, we can assume some epochs started at random numbers
	var cases = []struct {
		sprintSize, firstBlockInEpoch, blockNumber uint64
	}{
		{4, 1, 2},
		{5, 1, 3},
		{6, 0, 6},
		{7, 0, 4},
		{8, 0, 5},
		{9, 4, 9},
		{10, 7, 10},
		{10, 1, 1},
	}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			INNBCBFTConfig: &INNBCBFTConfig{},
		},
		epoch: &epochMetadata{},
	}

	for _, c := range cases {
		runtime.config.INNBCBFTConfig.SprintSize = c.sprintSize
		runtime.epoch.FirstBlockInEpoch = c.firstBlockInEpoch
		assert.False(t,
			runtime.isFixedSizeOfSprintMet(c.blockNumber, runtime.epoch),
			fmt.Sprintf(
				"Not expected end of sprint for sprint size=%v and parent block number=%v",
				c.sprintSize,
				c.blockNumber),
		)
	}
}

func TestConsensusRuntime_isFixedSizeOfSprintMet_ReachedEnd(t *testing.T) {
	t.Parallel()

	// because of slashing, we can assume some epochs started at random numbers
	var cases = []struct {
		sprintSize, firstBlockInEpoch, blockNumber uint64
	}{
		{4, 1, 4},
		{5, 1, 5},
		{6, 0, 5},
		{7, 0, 6},
		{8, 0, 7},
		{9, 4, 12},
		{10, 7, 16},
		{10, 1, 10},
		{5, 1, 10},
		{3, 3, 5},
	}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			INNBCBFTConfig: &INNBCBFTConfig{},
		},
		epoch: &epochMetadata{},
	}

	for _, c := range cases {
		runtime.config.INNBCBFTConfig.SprintSize = c.sprintSize
		runtime.epoch.FirstBlockInEpoch = c.firstBlockInEpoch
		assert.True(t,
			runtime.isFixedSizeOfSprintMet(c.blockNumber, runtime.epoch),
			fmt.Sprintf(
				"Not expected end of sprint for sprint size=%v and parent block number=%v",
				c.sprintSize,
				c.blockNumber),
		)
	}
}

func TestConsensusRuntime_OnBlockInserted_EndOfEpoch(t *testing.T) {
	t.Parallel()

	const (
		epochSize       = uint64(10)
		validatorsCount = 7
	)

	currentEpochNumber := getEpochNumber(t, epochSize, epochSize)
	validatorSet := validator.NewTestValidators(t, validatorsCount).GetPublicIdentities()
	header, headerMap := createTestBlocks(t, epochSize, epochSize, validatorSet)
	builtBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: header,
	})

	newEpochNumber := currentEpochNumber + 1
	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetEpoch").Return(newEpochNumber).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	innbcbftBackendMock := new(innbcbftBackendMock)
	innbcbftBackendMock.On("GetValidatorsWithTx", mock.Anything, mock.Anything, mock.Anything).Return(validatorSet).Times(3)

	txPool := new(txPoolMock)
	txPool.On("ResetWithHeaders", mock.Anything).Once()

	snapshot := NewProposerSnapshot(epochSize-1, validatorSet)
	config := &runtimeConfig{
		INNBCBFTConfig: &INNBCBFTConfig{
			EpochSize: epochSize,
		},
		blockchain:      blockchainMock,
		innbcbftBackend: innbcbftBackendMock,
		txPool:          txPool,
		State:           newTestState(t),
	}
	/* START ALPHA CHANGE 351 -- remove legacy event-provider test setup -- */
	/* FINISH ALPHA CHANGE 351 -- remove legacy event-provider test setup -- */

	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:             hclog.NewNullLogger(),
		state:              config.State,
		config:             config,
		epoch: &epochMetadata{
			Number:            currentEpochNumber,
			FirstBlockInEpoch: header.Number - epochSize + 1,
		},
		lastBuiltBlock: &types.Header{Number: header.Number - 1},
		/* START ALPHA CHANGE 316 -- remove bridge dummy managers from runtime epoch test -- */
		stakeManager: &dummyStakeManager{},
		/* FINISH ALPHA CHANGE 316 -- remove bridge dummy managers from runtime epoch test -- */
	}
	/* START ALPHA CHANGE 1465 -- verify successful runtime advancement result -- */
	advanced, err := runtime.OnBlockInserted(&types.FullBlock{Block: builtBlock})

	require.NoError(t, err)
	require.True(t, advanced)
	/* FINISH ALPHA CHANGE 1465 -- verify successful runtime advancement result -- */

	require.True(t, runtime.state.EpochStore.isEpochInserted(currentEpochNumber+1))
	require.Equal(t, newEpochNumber, runtime.epoch.Number)

	blockchainMock.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
}

func TestConsensusRuntime_OnBlockInserted_MiddleOfEpoch(t *testing.T) {
	t.Parallel()

	const (
		epoch             = 2
		epochSize         = uint64(10)
		firstBlockInEpoch = epochSize + 1
		blockNumber       = epochSize + 2
	)

	header := &types.Header{Number: blockNumber}
	builtBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: header,
	})

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(builtBlock.Header, true).Once()

	innbcbftBackendMock := new(innbcbftBackendMock)
	innbcbftBackendMock.On("GetValidatorsWithTx", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	txPool := new(txPoolMock)
	txPool.On("ResetWithHeaders", mock.Anything).Once()

	snapshot := NewProposerSnapshot(blockNumber, []*validator.ValidatorMetadata{})
	config := &runtimeConfig{
		INNBCBFTConfig: &INNBCBFTConfig{EpochSize: epochSize},
		blockchain:     blockchainMock,
		txPool:         txPool,
	}

	runtime := &consensusRuntime{
		lastBuiltBlock: header,
		config: &runtimeConfig{
			INNBCBFTConfig: &INNBCBFTConfig{EpochSize: epochSize},
			blockchain:     blockchainMock,
			txPool:         txPool,
		},
		epoch: &epochMetadata{
			Number:            epoch,
			FirstBlockInEpoch: firstBlockInEpoch,
		},
		logger:             hclog.NewNullLogger(),
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
	}
	/* START ALPHA CHANGE 1466 -- verify duplicate blocks do not report runtime advancement -- */
	advanced, err := runtime.OnBlockInserted(&types.FullBlock{Block: builtBlock})

	require.NoError(t, err)
	require.False(t, advanced)
	/* FINISH ALPHA CHANGE 1466 -- verify duplicate blocks do not report runtime advancement -- */

	require.Equal(t, header.Number, runtime.lastBuiltBlock.Number)
}

/* START ALPHA CHANGE 1467 -- test runtime alignment includes canonical block hash -- */
func TestConsensusRuntime_IsAlignedWithRequiresMatchingHash(t *testing.T) {
	t.Parallel()

	runtimeHeader := &types.Header{
		Number: 10,
		Hash:   types.StringToHash("runtime"),
	}

	runtime := &consensusRuntime{
		lastBuiltBlock: runtimeHeader,
	}

	require.True(t, runtime.IsAlignedWith(runtimeHeader.Copy()))
	require.False(t, runtime.IsAlignedWith(&types.Header{
		Number: 10,
		Hash:   types.StringToHash("different"),
	}))
	require.False(t, runtime.IsAlignedWith(&types.Header{
		Number: 11,
		Hash:   runtimeHeader.Hash,
	}))
}

/* FINISH ALPHA CHANGE 1467 -- test runtime alignment includes canonical block hash -- */

func TestConsensusRuntime_FSM_NotInValidatorSet(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D"})

	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		INNBCBFTConfig: &INNBCBFTConfig{
			EpochSize: 1,
		},
		Key: createTestKey(t),
	}
	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		config:             config,
		epoch: &epochMetadata{
			Number:     1,
			Validators: validators.GetPublicIdentities(),
		},
		lastBuiltBlock: &types.Header{},
	}
	runtime.setIsActiveValidator(true)

	err := runtime.FSM()
	assert.ErrorIs(t, err, errNotAValidator)
}

func TestConsensusRuntime_FSM_NotEndOfEpoch_NotEndOfSprint(t *testing.T) {
	t.Parallel()

	extra := &Extra{
		Checkpoint: &CheckpointData{},
	}
	lastBlock := &types.Header{
		Number:    1,
		ExtraData: extra.MarshalRLPTo(nil),
	}

	validators := validator.NewTestValidators(t, 3)
	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()

	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		INNBCBFTConfig: &INNBCBFTConfig{
			EpochSize:  10,
			SprintSize: 5,
		},
		Key:        wallet.NewKey(validators.GetPrivateIdentities()[0]),
		blockchain: blockchainMock,
	}
	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:             hclog.NewNullLogger(),
		config:             config,
		epoch: &epochMetadata{
			Number:            1,
			Validators:        validators.GetPublicIdentities(),
			FirstBlockInEpoch: 1,
		},
		lastBuiltBlock: lastBlock,
		state:          newTestState(t),
		/* START ALPHA CHANGE 317 -- remove bridge dummy managers from FSM test runtime -- */
		/* FINISH ALPHA CHANGE 317 -- remove bridge dummy managers from FSM test runtime -- */
	}
	runtime.setIsActiveValidator(true)

	err := runtime.FSM()
	require.NoError(t, err)

	assert.True(t, runtime.IsActiveValidator())
	assert.False(t, runtime.fsm.isEndOfEpoch)
	/* START ALPHA CHANGE 341 -- remove bridge sprint assertion from FSM test -- */
	/* FINISH ALPHA CHANGE 341 -- remove bridge sprint assertion from FSM test -- */
	assert.Equal(t, lastBlock.Number, runtime.fsm.parent.Number)

	address := types.Address(runtime.config.Key.Address())
	assert.True(t, runtime.fsm.ValidatorSet().Includes(address))

	assert.NotNil(t, runtime.fsm.blockBuilder)
	assert.NotNil(t, runtime.fsm.backend)

	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_FSM_EndOfEpoch_BuildCommitEpoch(t *testing.T) {
	t.Parallel()

	const (
		epoch             = 0
		epochSize         = uint64(10)
		sprintSize        = uint64(3)
		firstBlockInEpoch = uint64(1)
		fromIndex         = uint64(0)
		toIndex           = uint64(9)
	)

	validatorAccounts := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	validators := validatorAccounts.GetPublicIdentities()

	lastBuiltBlock, _ := createTestBlocks(t, 9, epochSize, validators)

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()

	state := newTestState(t)
	require.NoError(t, state.EpochStore.insertEpoch(epoch, nil))

	metadata := &epochMetadata{
		Validators:        validators,
		Number:            epoch,
		FirstBlockInEpoch: firstBlockInEpoch,
	}

	genesisValidator := types.Address(validatorAccounts.GetValidator("A").Address())

	config := &runtimeConfig{
		INNBCBFTConfig: &INNBCBFTConfig{
			EpochSize:               epochSize,
			SprintSize:              sprintSize,
			ValidatorRegistryConfig: testValidatorRegistryConfig(genesisValidator),
		},
		Key:        validatorAccounts.GetValidator("A").Key(),
		blockchain: blockchainMock,
	}

	snapshot := NewProposerSnapshot(1, nil)
	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:             hclog.NewNullLogger(),
		state:              state,
		epoch:              metadata,
		config:             config,
		lastBuiltBlock:     lastBuiltBlock,
		/* START ALPHA CHANGE 318 -- remove bridge dummy managers from transition test runtime -- */
		stakeManager: &dummyStakeManager{},
		/* FINISH ALPHA CHANGE 318 -- remove bridge dummy managers from transition test runtime -- */
	}

	err := runtime.FSM()
	fsm := runtime.fsm

	assert.NoError(t, err)
	assert.True(t, fsm.isEndOfEpoch)
	assert.NotNil(t, fsm.commitEpochInput)
	assert.NotEmpty(t, fsm.commitEpochInput)

	blockchainMock.AssertExpectations(t)
}

func Test_NewConsensusRuntime(t *testing.T) {
	t.Parallel()

	/* START ALPHA CHANGE 1550 -- rely on isolated per-test MDBX consensus state -- */
	/* FINISH ALPHA CHANGE 1550 -- rely on isolated per-test MDBX consensus state -- */

	innbcBFTConfig := &INNBCBFTConfig{
		/* START ALPHA CHANGE 319 -- remove bridge config from runtime initialization test -- */
		EpochSize:               10,
		SprintSize:              10,
		BlockTime:               common.Duration{Duration: 2 * time.Second},
		ValidatorRegistryConfig: testValidatorRegistryConfig(types.ZeroAddress),
		/* FINISH ALPHA CHANGE 319 -- remove bridge config from runtime initialization test -- */
	}

	validators := validator.NewTestValidators(t, 3).GetPublicIdentities()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetEpoch").Return(uint64(1)).Once()
	/* START ALPHA CHANGE 320 -- remove bridge committed-index expectation from runtime initialization test -- */
	/* FINISH ALPHA CHANGE 320 -- remove bridge committed-index expectation from runtime initialization test -- */

	blockchainMock := &blockchainMock{}
	blockchainMock.On("CurrentHeader").Return(&types.Header{Number: 1, ExtraData: createTestExtraForAccounts(t, 1, validators, nil)})
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()
	blockchainMock.On("GetHeaderByNumber", uint64(0)).Return(&types.Header{Number: 0, ExtraData: createTestExtraForAccounts(t, 0, validators, nil)})
	blockchainMock.On("GetHeaderByNumber", uint64(1)).Return(&types.Header{Number: 1, ExtraData: createTestExtraForAccounts(t, 1, validators, nil)})

	innbcbftBackendMock := new(innbcbftBackendMock)
	innbcbftBackendMock.On("GetValidatorsWithTx", mock.Anything, mock.Anything, mock.Anything).Return(validators).Times(3)

	tmpDir := t.TempDir()
	config := &runtimeConfig{
		innbcbftBackend: innbcbftBackendMock,
		State:           newTestState(t),
		INNBCBFTConfig:  innbcBFTConfig,
		DataDir:         tmpDir,
		Key:             createTestKey(t),
		blockchain:      blockchainMock,
		/* START ALPHA CHANGE 321 -- remove bridge topic from runtime config test literal -- */
		consensusConfig: &consensus.Config{},
		/* FINISH ALPHA CHANGE 321 -- remove bridge topic from runtime config test literal -- */
	}

	require.NoError(t, config.State.StakeStore.insertFullValidatorSet(validatorSetState{
		BlockNumber: 1,
	}, nil))

	runtime, err := newConsensusRuntime(hclog.NewNullLogger(), config)
	require.NoError(t, err)

	assert.False(t, runtime.IsActiveValidator())
	assert.Equal(t, runtime.config.DataDir, tmpDir)
	assert.Equal(t, uint64(10), runtime.config.INNBCBFTConfig.SprintSize)
	assert.Equal(t, uint64(10), runtime.config.INNBCBFTConfig.EpochSize)
	assert.Equal(t, "0x0000000000000000000000000000000000000101", contracts.ValidatorSetContract.String())
	/* START ALPHA CHANGE 322 -- remove bridge runtime assertions -- */
	/* FINISH ALPHA CHANGE 322 -- remove bridge runtime assertions -- */
	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
	innbcbftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_restartEpoch_SameEpochNumberAsTheLastOne(t *testing.T) {
	t.Parallel()

	const originalBlockNumber = uint64(5)

	newCurrentHeader := &types.Header{Number: originalBlockNumber + 1}
	validatorSet := validator.NewTestValidators(t, 3).GetPublicIdentities()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetEpoch").Return(uint64(1), nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		blockchain: blockchainMock,
	}
	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		config:             config,
		epoch: &epochMetadata{
			Number:            1,
			Validators:        validatorSet,
			FirstBlockInEpoch: 1,
		},
		lastBuiltBlock: &types.Header{
			Number: originalBlockNumber,
		},
	}
	runtime.setIsActiveValidator(true)

	epoch, err := runtime.restartEpoch(newCurrentHeader, nil)

	require.NoError(t, err)

	for _, a := range validatorSet.GetAddresses() {
		assert.True(t, epoch.Validators.ContainsAddress(a))
	}

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_calculateCommitEpochInput_SecondEpoch(t *testing.T) {
	t.Parallel()

	const (
		epoch           = 2
		epochSize       = 10
		epochStartBlock = 11
		epochEndBlock   = 20
		sprintSize      = 5
	)

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"})
	genesisValidator := types.Address(validators.GetValidator("A").Address())

	innbcbftConfig := &INNBCBFTConfig{
		EpochSize:               epochSize,
		SprintSize:              sprintSize,
		ValidatorRegistryConfig: testValidatorRegistryConfig(genesisValidator),
	}

	lastBuiltBlock, _ := createTestBlocks(t, 19, epochSize, validators.GetPublicIdentities())

	blockchainMock := new(blockchainMock)

	innbcbftBackendMock := new(innbcbftBackendMock)

	config := &runtimeConfig{
		INNBCBFTConfig:  innbcbftConfig,
		blockchain:      blockchainMock,
		innbcbftBackend: innbcbftBackendMock,
		Key:             validators.GetValidator("A").Key(),
	}

	consensusRuntime := &consensusRuntime{
		config: config,
		epoch: &epochMetadata{
			Number:            epoch,
			Validators:        validators.GetPublicIdentities(),
			FirstBlockInEpoch: epochStartBlock,
		},
		lastBuiltBlock: lastBuiltBlock,
	}

	/* START ALPHA CHANGE 576 -- remove obsolete reward distribution assertion from commit epoch test -- */
	commitEpochInput, err := consensusRuntime.calculateCommitEpochInput(lastBuiltBlock, consensusRuntime.epoch)
	/* FINISH ALPHA CHANGE 576 -- remove obsolete reward distribution assertion from commit epoch test -- */
	assert.NoError(t, err)
	assert.NotEmpty(t, commitEpochInput)
	assert.Equal(t, uint64(epoch), commitEpochInput.ID.Uint64())
	assert.Equal(t, uint64(epochStartBlock), commitEpochInput.Epoch.StartBlock.Uint64())
	assert.Equal(t, uint64(epochEndBlock), commitEpochInput.Epoch.EndBlock.Uint64())

	blockchainMock.AssertExpectations(t)
	innbcbftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_IsValidValidator_BasicCases(t *testing.T) {
	t.Parallel()

	setupFn := func(t *testing.T) (*consensusRuntime, *validator.TestValidators) {
		t.Helper()

		validatorAccounts := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
		epoch := &epochMetadata{
			Validators: validatorAccounts.GetPublicIdentities("A", "B", "C", "D"),
		}
		runtime := &consensusRuntime{
			epoch:  epoch,
			logger: hclog.NewNullLogger(),
			fsm:    &fsm{validators: validator.NewValidatorSet(epoch.Validators, hclog.NewNullLogger())},
		}

		return runtime, validatorAccounts
	}

	cases := []struct {
		name          string
		signerAlias   string
		senderAlias   string
		isValidSender bool
	}{
		{
			name:          "Valid sender",
			signerAlias:   "A",
			senderAlias:   "A",
			isValidSender: true,
		},
		{
			name:          "Sender not amongst current validators",
			signerAlias:   "F",
			senderAlias:   "F",
			isValidSender: false,
		},
		{
			name:          "Sender and signer accounts mismatch",
			signerAlias:   "A",
			senderAlias:   "B",
			isValidSender: false,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			runtime, validatorAccounts := setupFn(t)
			signer := validatorAccounts.GetValidator(c.signerAlias)
			sender := validatorAccounts.GetValidator(c.senderAlias)
			msg, err := signer.Key().SignIBFTMessage(&proto.Message{From: sender.Address().Bytes()})

			require.NoError(t, err)
			require.Equal(t, c.isValidSender, runtime.IsValidValidator(msg))
		})
	}
}

func TestConsensusRuntime_IsValidValidator_TamperSignature(t *testing.T) {
	t.Parallel()

	validatorAccounts := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	epoch := &epochMetadata{
		Validators: validatorAccounts.GetPublicIdentities("A", "B", "C", "D"),
	}
	runtime := &consensusRuntime{
		epoch:  epoch,
		logger: hclog.NewNullLogger(),
		fsm:    &fsm{validators: validator.NewValidatorSet(epoch.Validators, hclog.NewNullLogger())},
	}

	// provide invalid signature
	sender := validatorAccounts.GetValidator("A")
	msg := &proto.Message{
		From:      sender.Address().Bytes(),
		Signature: []byte{1, 2, 3, 4, 5},
	}
	require.False(t, runtime.IsValidValidator(msg))
}

func TestConsensusRuntime_TamperMessageContent(t *testing.T) {
	t.Parallel()

	validatorAccounts := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	epoch := &epochMetadata{
		Validators: validatorAccounts.GetPublicIdentities("A", "B", "C", "D"),
	}
	runtime := &consensusRuntime{
		epoch:  epoch,
		logger: hclog.NewNullLogger(),
		fsm:    &fsm{validators: validator.NewValidatorSet(epoch.Validators, hclog.NewNullLogger())},
	}
	sender := validatorAccounts.GetValidator("A")
	proposalHash := []byte{2, 4, 6, 8, 10}
	proposalSignature, err := sender.Key().SignWithDomain(proposalHash, signer.DomainCheckpointManager)
	require.NoError(t, err)

	msg := &proto.Message{
		View: &proto.View{},
		From: sender.Address().Bytes(),
		Type: proto.MessageType_COMMIT,
		Payload: &proto.Message_CommitData{
			CommitData: &proto.CommitMessage{
				ProposalHash:  proposalHash,
				CommittedSeal: proposalSignature,
			},
		},
	}
	// sign the message itself
	msg, err = sender.Key().SignIBFTMessage(msg)
	assert.NoError(t, err)
	// signature verification works
	assert.True(t, runtime.IsValidValidator(msg))

	// modify message without signing it again
	msg.Payload = &proto.Message_CommitData{
		CommitData: &proto.CommitMessage{
			ProposalHash:  []byte{1, 3, 5, 7, 9}, // modification
			CommittedSeal: proposalSignature,
		},
	}
	// signature isn't valid, because message was tampered
	assert.False(t, runtime.IsValidValidator(msg))
}

func TestConsensusRuntime_IsValidProposalHash(t *testing.T) {
	t.Parallel()

	extra := &Extra{
		Checkpoint: &CheckpointData{
			EpochNumber: 1,
			BlockRound:  1,
		},
	}
	block := &types.Block{
		Header: &types.Header{
			Number:    10,
			ExtraData: extra.MarshalRLPTo(nil),
		},
	}
	block.Header.ComputeHash()

	proposalHash, err := extra.Checkpoint.Hash(0, block.Number(), block.Hash())
	require.NoError(t, err)

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{blockchain: new(blockchainMock)},
	}

	/* START ALPHA CHANGE 2569 -- provide the matching round in the valid proposal fixture -- */
	require.True(t, runtime.IsValidProposalHash(
		&proto.Proposal{RawProposal: block.MarshalRLP(), Round: 1},
		proposalHash.Bytes(),
	))
	/* FINISH ALPHA CHANGE 2569 -- provide the matching round in the valid proposal fixture -- */
}

func TestConsensusRuntime_IsValidProposalHash_InvalidProposalHash(t *testing.T) {
	t.Parallel()

	extra := &Extra{
		Checkpoint: &CheckpointData{
			EpochNumber: 1,
			BlockRound:  1,
		},
	}

	block := &types.Block{
		Header: &types.Header{
			Number:    10,
			ExtraData: extra.MarshalRLPTo(nil),
		},
	}

	proposalHash, err := extra.Checkpoint.Hash(0, block.Number(), block.Hash())
	require.NoError(t, err)

	extra.Checkpoint.BlockRound = 2 // change it so it is not the same as in proposal hash
	block.Header.ExtraData = extra.MarshalRLPTo(nil)
	block.Header.ComputeHash()

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{blockchain: new(blockchainMock)},
	}

	/* START ALPHA CHANGE 2569 -- retain hash-mismatch coverage with an admissible round -- */
	require.False(t, runtime.IsValidProposalHash(
		&proto.Proposal{RawProposal: block.MarshalRLP(), Round: 2},
		proposalHash.Bytes(),
	))
	/* FINISH ALPHA CHANGE 2569 -- retain hash-mismatch coverage with an admissible round -- */
}

func TestConsensusRuntime_IsValidProposalHash_InvalidExtra(t *testing.T) {
	t.Parallel()

	extra := &Extra{
		Checkpoint: &CheckpointData{
			EpochNumber: 1,
			BlockRound:  1,
		},
	}

	block := &types.Block{
		Header: &types.Header{
			Number:    10,
			ExtraData: []byte{1, 2, 3}, // invalid extra in block
		},
	}
	block.Header.ComputeHash()

	proposalHash, err := extra.Checkpoint.Hash(0, block.Number(), block.Hash())
	require.NoError(t, err)

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{blockchain: new(blockchainMock)},
	}

	require.False(t, runtime.IsValidProposalHash(&proto.Proposal{RawProposal: block.MarshalRLP()}, proposalHash.Bytes()))
}

func TestConsensusRuntime_BuildProposal_InvalidParent(t *testing.T) {
	config := &runtimeConfig{}
	snapshot := NewProposerSnapshot(1, nil)
	runtime := &consensusRuntime{
		logger:             hclog.NewNullLogger(),
		lastBuiltBlock:     &types.Header{Number: 2},
		epoch:              &epochMetadata{Number: 1},
		config:             config,
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
	}

	require.Nil(t, runtime.BuildProposal(&proto.View{Round: 5}))
}

func TestConsensusRuntime_ID(t *testing.T) {
	t.Parallel()

	key1, key2 := createTestKey(t), createTestKey(t)
	runtime := &consensusRuntime{
		config: &runtimeConfig{Key: key1},
	}

	require.Equal(t, runtime.ID(), key1.Address().Bytes())
	require.NotEqual(t, runtime.ID(), key2.Address().Bytes())
}

/* START ALPHA CHANGE 2635 -- exercise concurrent proposer authorization and lock ordering -- */
func TestConsensusRuntime_IsProposer_ConcurrentRounds(t *testing.T) {
	const height uint64 = 100

	validators := validator.NewTestValidatorsWithAliases(
		t, []string{"A", "B", "C", "D", "E"}, []uint64{1, 2, 3, 4, 5},
	)
	accounts := validators.GetPublicIdentities("A", "B", "C", "D", "E")
	snapshot := NewProposerSnapshot(height, accounts)
	runtime := &consensusRuntime{fsm: &fsm{proposerSnapshot: snapshot}}

	/*
		Calculate each reference round on an independent copy before
		starting workers. Include rounds beyond the catch-up window to
		ensure this synchronization change introduces no round ceiling.
	*/
	rounds := []uint64{0, 1, 2, 3, 5, 6, 8, 16, 32}
	expected := make(map[uint64]types.Address, len(rounds))
	for _, round := range rounds {
		address, err := snapshot.Copy().CalcProposer(round, height)
		require.NoError(t, err)
		expected[round] = address
	}
	require.Equal(t, accounts[4].Address, expected[0])
	require.Equal(t, accounts[3].Address, expected[1])
	require.Equal(t, accounts[0].Address, expected[6])

	priorities := make([]*big.Int, len(snapshot.Validators))
	for i, entry := range snapshot.Validators {
		priorities[i] = new(big.Int).Set(entry.ProposerPriority)
	}

	const workers, iterations = 8, 64
	start := make(chan struct{})
	failures := make(chan string, workers)
	var wg sync.WaitGroup
	wg.Add(workers)

	for worker := 0; worker < workers; worker++ {
		go func(worker int) {
			defer wg.Done()
			<-start

			for iteration := 0; iteration < iterations; iteration++ {
				round := rounds[(worker+iteration)%len(rounds)]
				for _, account := range accounts {
					want := account.Address == expected[round]
					got := runtime.IsProposer(account.Address.Bytes(), height, round)
					if got != want {
						failures <- fmt.Sprintf(
							"worker %d, iteration %d, round %d, validator %s: got %t, want %t",
							worker, iteration, round, account.Address.String(), got, want,
						)

						return
					}
				}

				if !runtime.IsProposer(expected[round].Bytes(), height, round) {
					failures <- fmt.Sprintf("cached authorization failed for round %d", round)

					return
				}
			}
		}(worker)
	}

	close(start)
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}

	/* Workers have joined, so cache inspection here needs no synchronization. */
	require.NotNil(t, snapshot.Proposer)
	require.Equal(t, expected[snapshot.Round], snapshot.Proposer.Metadata.Address)
	require.Equal(t, height, snapshot.Height)
	for i, entry := range snapshot.Validators {
		require.Equal(t, priorities[i], entry.ProposerPriority)
	}
}

func TestConsensusRuntime_IsProposer_LockOrderAndFSMReplacement(t *testing.T) {
	validators := validator.NewTestValidatorsWithAliases(
		t, []string{"A", "B", "C"}, []uint64{1, 2, 3},
	)
	accounts := validators.GetPublicIdentities("A", "B", "C")
	oldSnapshot := NewProposerSnapshot(100, accounts)
	newSnapshot := NewProposerSnapshot(101, accounts)
	const round uint64 = 6

	address, err := newSnapshot.Copy().CalcProposer(round, newSnapshot.Height)
	require.NoError(t, err)
	runtime := &consensusRuntime{fsm: &fsm{proposerSnapshot: oldSnapshot}}

	/*
		Hold the runtime write lock. A correctly ordered check must acquire
		the proposer gate before waiting for the runtime read lock.
	*/
	runtime.lock.Lock()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(runtime.lock.Unlock) }
	result := make(chan bool, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		release()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("proposer check did not finish after releasing the runtime lock")
		}
	})

	go func() {
		defer close(finished)
		result <- runtime.IsProposer(address.Bytes(), newSnapshot.Height, round)
	}()

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()

	for {
		/*
			Only the worker can hold this gate between probes. TryLock is
			nonblocking, so the test never waits for it while holding c.lock.
		*/
		if !runtime.proposerLock.TryLock() {
			break
		}
		runtime.proposerLock.Unlock()

		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("proposer gate was not acquired before the runtime read lock")
		}
	}

	/* Publish the new FSM while the worker is waiting for the read lock. */
	runtime.fsm = &fsm{proposerSnapshot: newSnapshot}
	release()

	select {
	case valid := <-result:
		require.True(t, valid)
	case <-time.After(5 * time.Second):
		t.Fatal("proposer check did not complete against the replacement FSM")
	}
	<-finished
	require.Nil(t, oldSnapshot.Proposer)
	require.NotNil(t, newSnapshot.Proposer)
	require.Equal(t, round, newSnapshot.Round)
	require.Equal(t, address, newSnapshot.Proposer.Metadata.Address)
}

func TestConsensusRuntime_IsProposer_UnavailableOrWrongHeight(t *testing.T) {
	validators := validator.NewTestValidatorsWithAliases(
		t, []string{"A", "B", "C"}, []uint64{1, 2, 3},
	)
	accounts := validators.GetPublicIdentities("A", "B", "C")

	for _, test := range []struct {
		name    string
		machine *fsm
	}{
		{"missing FSM", nil},
		{"missing snapshot", &fsm{}},
		{"empty validators", &fsm{proposerSnapshot: NewProposerSnapshot(100, nil)}},
		{"wrong height", &fsm{proposerSnapshot: NewProposerSnapshot(101, accounts)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := &consensusRuntime{fsm: test.machine}
			require.False(t, runtime.IsProposer(accounts[0].Address.Bytes(), 100, 0))
		})
	}
}

/* FINISH ALPHA CHANGE 2635 -- exercise concurrent proposer authorization and lock ordering -- */

func TestConsensusRuntime_GetVotingPowers(t *testing.T) {
	t.Parallel()

	const height = 100

	validatorAccounts := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C"}, []uint64{1, 3, 2})
	runtime := &consensusRuntime{}

	_, err := runtime.GetVotingPowers(height)
	require.Error(t, err)

	runtime.fsm = &fsm{
		validators: validatorAccounts.ToValidatorSet(),
		parent:     &types.Header{Number: height},
	}

	_, err = runtime.GetVotingPowers(height)
	require.Error(t, err)

	runtime.fsm.parent.Number = height - 1

	val, err := runtime.GetVotingPowers(height)
	require.NoError(t, err)

	addresses := validatorAccounts.GetPublicIdentities([]string{"A", "B", "C"}...).GetAddresses()

	assert.Equal(t, uint64(1), val[types.AddressToString(addresses[0])].Uint64())
	assert.Equal(t, uint64(3), val[types.AddressToString(addresses[1])].Uint64())
	assert.Equal(t, uint64(2), val[types.AddressToString(addresses[2])].Uint64())
}

func TestConsensusRuntime_BuildRoundChangeMessage(t *testing.T) {
	t.Parallel()

	key := createTestKey(t)
	view, rawProposal, certificate := &proto.View{}, []byte{1}, &proto.PreparedCertificate{}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			Key: key,
		},
	}

	proposal := &proto.Proposal{
		RawProposal: rawProposal,
		Round:       view.Round,
	}

	expected := proto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: proto.MessageType_ROUND_CHANGE,
		Payload: &proto.Message_RoundChangeData{RoundChangeData: &proto.RoundChangeMessage{
			LatestPreparedCertificate: certificate,
			LastPreparedProposal:      proposal,
		}},
	}

	signedMsg, err := key.SignIBFTMessage(&expected)
	require.NoError(t, err)

	assert.Equal(t, signedMsg, runtime.BuildRoundChangeMessage(proposal, certificate, view))
}

func TestConsensusRuntime_BuildCommitMessage(t *testing.T) {
	t.Parallel()

	key := createTestKey(t)
	view, proposalHash := &proto.View{}, []byte{1, 2, 4}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			Key: key,
		},
	}

	committedSeal, err := key.SignWithDomain(proposalHash, signer.DomainCheckpointManager)
	require.NoError(t, err)

	expected := proto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: proto.MessageType_COMMIT,
		Payload: &proto.Message_CommitData{
			CommitData: &proto.CommitMessage{
				ProposalHash:  proposalHash,
				CommittedSeal: committedSeal,
			},
		},
	}

	signedMsg, err := key.SignIBFTMessage(&expected)
	require.NoError(t, err)

	assert.Equal(t, signedMsg, runtime.BuildCommitMessage(proposalHash, view))
}

func TestConsensusRuntime_BuildPrePrepareMessage_EmptyProposal(t *testing.T) {
	t.Parallel()

	runtime := &consensusRuntime{logger: hclog.NewNullLogger()}

	assert.Nil(t, runtime.BuildPrePrepareMessage(nil, &proto.RoundChangeCertificate{}, &proto.View{Height: 1, Round: 0}))
}

func TestConsensusRuntime_IsValidProposalHash_EmptyProposal(t *testing.T) {
	t.Parallel()

	runtime := &consensusRuntime{logger: hclog.NewNullLogger()}

	assert.False(t, runtime.IsValidProposalHash(&proto.Proposal{}, []byte("hash")))
}

/* START ALPHA CHANGE 1846 -- reject nil consensus proposals without panicking -- */
func TestConsensusRuntime_IsValidProposalHash_NilProposal(t *testing.T) {
	t.Parallel()

	runtime := &consensusRuntime{logger: hclog.NewNullLogger()}

	assert.False(t, runtime.IsValidProposalHash(nil, []byte("hash")))
}

/* FINISH ALPHA CHANGE 1846 -- reject nil consensus proposals without panicking -- */

func TestConsensusRuntime_BuildPrepareMessage(t *testing.T) {
	t.Parallel()

	key := createTestKey(t)
	view, proposalHash := &proto.View{}, []byte{1, 2, 4}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			Key: key,
		},
	}

	expected := proto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: proto.MessageType_PREPARE,
		Payload: &proto.Message_PrepareData{
			PrepareData: &proto.PrepareMessage{
				ProposalHash: proposalHash,
			},
		},
	}

	signedMsg, err := key.SignIBFTMessage(&expected)
	require.NoError(t, err)

	assert.Equal(t, signedMsg, runtime.BuildPrepareMessage(proposalHash, view))
}

func createTestBlocks(t *testing.T, numberOfBlocks, defaultEpochSize uint64,
	validatorSet validator.AccountSet) (*types.Header, *testHeadersMap) {
	t.Helper()

	headerMap := &testHeadersMap{}
	bitmaps := createTestBitmaps(t, validatorSet, numberOfBlocks)

	extra := &Extra{
		Checkpoint: &CheckpointData{EpochNumber: 0},
	}

	genesisBlock := &types.Header{
		Number:    0,
		ExtraData: extra.MarshalRLPTo(nil),
	}
	parentHash := types.BytesToHash(big.NewInt(0).Bytes())

	headerMap.addHeader(genesisBlock)

	var hash types.Hash

	var blockHeader *types.Header

	for i := uint64(1); i <= numberOfBlocks; i++ {
		big := big.NewInt(int64(i))
		hash = types.BytesToHash(big.Bytes())

		header := &types.Header{
			Number:     i,
			ParentHash: parentHash,
			ExtraData:  createTestExtraForAccounts(t, getEpochNumber(t, i, defaultEpochSize), validatorSet, bitmaps[i]),
			GasLimit:   types.StateTransactionGasLimit,
		}

		headerMap.addHeader(header)

		parentHash = hash
		blockHeader = header
	}

	return blockHeader, headerMap
}

func createTestBitmaps(t *testing.T, validators validator.AccountSet, numberOfBlocks uint64) map[uint64]bitmap.Bitmap {
	t.Helper()

	bitmaps := make(map[uint64]bitmap.Bitmap, numberOfBlocks)

	rand.Seed(time.Now().UTC().Unix())

	for i := numberOfBlocks; i > 1; i-- {
		bitmap := bitmap.Bitmap{}
		j := 0

		for j != 3 {
			validator := validators[rand.Intn(validators.Len())]
			index := uint64(validators.Index(validator.Address))

			if !bitmap.IsSet(index) {
				bitmap.Set(index)
				j++
			}
		}

		bitmaps[i] = bitmap
	}

	return bitmaps
}

func createTestExtraForAccounts(t *testing.T, epoch uint64, validators validator.AccountSet, b bitmap.Bitmap) []byte {
	t.Helper()

	dummySignature := [64]byte{}
	extraData := Extra{
		Validators: &validator.ValidatorSetDelta{
			Added:   validators,
			Removed: bitmap.Bitmap{},
		},
		Parent:     &Signature{Bitmap: b, AggregatedSignature: dummySignature[:]},
		Committed:  &Signature{Bitmap: b, AggregatedSignature: dummySignature[:]},
		Checkpoint: &CheckpointData{EpochNumber: epoch},
	}

	return extraData.MarshalRLPTo(nil)
}

/* START ALPHA CHANGE 388 -- remove obsolete bridge exit event encoder test helper -- */
/* FINISH ALPHA CHANGE 388 -- remove obsolete bridge exit event encoder test helper -- */
