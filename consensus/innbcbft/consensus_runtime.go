package innbcbft

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	/* START ALPHA CHANGE 587 -- remove sort import used only by reward uptime accounting -- */
	/* FINISH ALPHA CHANGE 587 -- remove sort import used only by reward uptime accounting -- */
	"sync"
	"sync/atomic"
	"time"

	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"

	/* START ALPHA CHANGE 2583 -- reference the proposal execution handle -- */
	core "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftcore"
	/* FINISH ALPHA CHANGE 2583 -- reference the proposal execution handle -- */

	/* START ALPHA CHANGE 1873 -- use INNBC-owned IBFT message packages -- */
	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"

	/* FINISH ALPHA CHANGE 1873 -- use INNBC-owned IBFT message packages -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"

	/* START ALPHA CHANGE 2583 -- move proposal execution logging into proposal_execution.go -- */
	/* FINISH ALPHA CHANGE 2583 -- move proposal execution logging into proposal_execution.go -- */
	"github.com/innovativebioresearch/innbc-chain/types"

	/* START ALPHA CHANGE 1527 -- remove bbolt from consensus runtime -- */
	/* FINISH ALPHA CHANGE 1527 -- remove bbolt from consensus runtime -- */

	hcf "github.com/hashicorp/go-hclog"
)

const (
	/* START ALPHA CHANGE 407 -- remove unused bridge commitment size constant -- */
	/* START ALPHA CHANGE 1528 -- store consensus state in an MDBX directory -- */
	stateFileName = "consensusState"
	/* FINISH ALPHA CHANGE 1528 -- store consensus state in an MDBX directory -- */
	/* FINISH ALPHA CHANGE 407 -- remove unused bridge commitment size constant -- */
	/* START ALPHA CHANGE 588 -- remove reward uptime lookback constant -- */
	/* FINISH ALPHA CHANGE 588 -- remove reward uptime lookback constant -- */
)

var (
	// errNotAValidator represents "node is not a validator" error message
	errNotAValidator = errors.New("node is not a validator")
	/* START ALPHA CHANGE 408 -- remove unused bridge commitment quorum error -- */
	/* FINISH ALPHA CHANGE 408 -- remove unused bridge commitment quorum error -- */
)

// txPoolInterface is an abstraction of transaction pool
type txPoolInterface interface {
	/* START ALPHA CHANGE 2201 -- protect proposal selection against incomplete pool resets -- */
	WithReconciledPool(func() error) error
	/* FINISH ALPHA CHANGE 2201 -- protect proposal selection against incomplete pool resets -- */
	Prepare()
	Length() uint64
	Peek() *types.Transaction
	Pop(*types.Transaction)
	Drop(*types.Transaction)
	Demote(*types.Transaction)
	SetSealing(bool)
	ResetWithHeaders(...*types.Header)
}

// epochMetadata is the static info for epoch currently being processed
type epochMetadata struct {
	// Number is the number of the epoch
	Number uint64

	FirstBlockInEpoch uint64

	// Validators is the set of validators for the epoch
	Validators validator.AccountSet
}

type guardedDataDTO struct {
	// last built block header at the time of collecting data
	lastBuiltBlock *types.Header

	// epoch metadata at the time of collecting data
	epoch *epochMetadata

	// proposerSnapshot at the time of collecting data
	proposerSnapshot *ProposerSnapshot
}

// runtimeConfig is a struct that holds configuration data for given consensus runtime
type runtimeConfig struct {
	INNBCBFTConfig  *INNBCBFTConfig
	DataDir         string
	Key             *wallet.Key
	State           *State
	blockchain      blockchainBackend
	innbcbftBackend innbcbftBackend
	txPool          txPoolInterface

	/* START ALPHA CHANGE 1455 -- report fatal local runtime advancement errors -- */
	reportFatalError func(error)
	/* FINISH ALPHA CHANGE 1455 -- report fatal local runtime advancement errors -- */
	/* START ALPHA CHANGE 302 -- remove bridge gossip topic from runtime config -- */
	/* FINISH ALPHA CHANGE 302 -- remove bridge gossip topic from runtime config -- */
	numBlockConfirmations uint64
	consensusConfig       *consensus.Config
}

// consensusRuntime is a struct that provides consensus runtime features like epoch, state and event management
type consensusRuntime struct {
	// config represents wrapper around required parameters which are received from the outside
	config *runtimeConfig

	// state is reference to the struct which encapsulates bridge events persistence logic
	state *State

	// fsm instance which is created for each `runSequence`
	fsm *fsm

	// lock is a lock to access 'epoch' and `lastBuiltBlock`
	lock sync.RWMutex

	/* START ALPHA CHANGE 2632 -- serialize live proposer cache access independently of runtime publication -- */
	/*
		Acquire proposerLock before lock. Waiting proposer checks must not
		hold a runtime read lock while queued behind another calculation.
	*/
	proposerLock sync.Mutex
	/* FINISH ALPHA CHANGE 2632 -- serialize live proposer cache access independently of runtime publication -- */

	// epoch is the metadata for the current epoch
	epoch *epochMetadata

	// lastBuiltBlock is the header of the last processed block
	lastBuiltBlock *types.Header

	// activeValidatorFlag indicates whether the given node is amongst currently active validator set
	activeValidatorFlag atomic.Bool

	/* START ALPHA CHANGE 303 -- remove runtime bridge checkpoint, state-sync, and event-provider managers -- */
	// proposerCalculator is the object which manipulates with ProposerSnapshot
	proposerCalculator *ProposerCalculator

	// manager for handling validator stake change and updating validator set
	stakeManager StakeManager
	/* FINISH ALPHA CHANGE 303 -- remove runtime bridge checkpoint, state-sync, and event-provider managers -- */

	// logger instance
	logger hcf.Logger
}

// newConsensusRuntime creates and starts a new consensus runtime instance with event tracking
func newConsensusRuntime(log hcf.Logger, config *runtimeConfig) (*consensusRuntime, error) {
	dbTx, err := config.State.beginDBTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("could not begin dbTx to init consensus runtime: %w", err)
	}

	defer dbTx.Rollback() //nolint:errcheck

	proposerCalculator, err := NewProposerCalculator(config, log.Named("proposer_calculator"), dbTx)
	if err != nil {
		return nil, fmt.Errorf("failed to create consensus runtime, error while creating proposer calculator %w", err)
	}

	runtime := &consensusRuntime{
		state:              config.State,
		config:             config,
		lastBuiltBlock:     config.blockchain.CurrentHeader(),
		proposerCalculator: proposerCalculator,
		logger:             log.Named("consensus_runtime"),
		/* START ALPHA CHANGE 341 -- stop constructing legacy event provider -- */
		/* FINISH ALPHA CHANGE 341 -- stop constructing legacy event provider -- */
	}

	/* START ALPHA CHANGE 304 -- initialize only INNBC-native runtime managers -- */
	if err := runtime.initStakeManager(log, dbTx); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 304 -- initialize only INNBC-native runtime managers -- */

	// we need to call restart epoch on runtime to initialize epoch state
	runtime.epoch, err = runtime.restartEpoch(runtime.lastBuiltBlock, dbTx)
	if err != nil {
		return nil, fmt.Errorf("consensus runtime creation - restart epoch failed: %w", err)
	}

	if err := dbTx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit db tx to init consensus runtime: %w", err)
	}

	return runtime, nil
}

// close is used to tear down allocated resources
func (c *consensusRuntime) close() {
	/* START ALPHA CHANGE 305 -- remove bridge manager shutdown hooks -- */
	/* FINISH ALPHA CHANGE 305 -- remove bridge manager shutdown hooks -- */
}

// initStakeManager initializes stake manager
func (c *consensusRuntime) initStakeManager(
	logger hcf.Logger,
	/* START ALPHA CHANGE 1529 -- accept the MDBX initialization transaction -- */
	_ *stateTx,
	/* FINISH ALPHA CHANGE 1529 -- accept the MDBX initialization transaction -- */
) error {
	var err error

	/* START ALPHA CHANGE 140 -- initialize INNBC-native validator registry manager instead of rootchain stake manager -- */
	/* START ALPHA CHANGE 2014 -- construct the registry manager without repeating genesis proof verification -- */
	c.stakeManager, err = newValidatorRegistryManager(
		logger.Named("validator-registry-manager"),
		c.config.blockchain,
		c.config.INNBCBFTConfig.ValidatorRegistryConfig,
		int(c.config.INNBCBFTConfig.MaxValidatorSetSize),
	)
	/* FINISH ALPHA CHANGE 2014 -- construct the registry manager without repeating genesis proof verification -- */
	if err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 140 -- initialize INNBC-native validator registry manager instead of rootchain stake manager -- */

	/* START ALPHA CHANGE 342 -- remove legacy event subscription from stake manager -- */
	/* FINISH ALPHA CHANGE 342 -- remove legacy event subscription from stake manager -- */

	return nil
}

// getGuardedData returns last build block, proposer snapshot and current epochMetadata in a thread-safe manner.
func (c *consensusRuntime) getGuardedData() (guardedDataDTO, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	lastBuiltBlock := c.lastBuiltBlock.Copy()
	epoch := new(epochMetadata)
	*epoch = *c.epoch // shallow copy, don't need to make validators copy because AccountSet is immutable
	proposerSnapshot, ok := c.proposerCalculator.GetSnapshot()

	if !ok {
		return guardedDataDTO{}, errors.New("cannot collect shared data, snapshot is empty")
	}

	return guardedDataDTO{
		epoch:            epoch,
		lastBuiltBlock:   lastBuiltBlock,
		proposerSnapshot: proposerSnapshot,
	}, nil
}

/* START ALPHA CHANGE 1450 -- compare runtime and blockchain parents by number and hash -- */
var errRuntimeHeadChanged = errors.New("consensus runtime head changed while preparing FSM")

func sameBlockHeader(left, right *types.Header) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.Number == right.Number && left.Hash == right.Hash
}

func (c *consensusRuntime) IsAlignedWith(header *types.Header) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return sameBlockHeader(c.lastBuiltBlock, header)
}

func (c *consensusRuntime) PrepareFSM(expectedParent *types.Header) (uint64, error) {
	if expectedParent == nil {
		return 0, errors.New("cannot prepare FSM without expected parent")
	}

	if !c.IsAlignedWith(expectedParent) {
		return 0, errRuntimeHeadChanged
	}

	if err := c.FSM(); err != nil {
		return 0, err
	}

	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.fsm == nil ||
		!sameBlockHeader(c.lastBuiltBlock, expectedParent) ||
		!sameBlockHeader(c.fsm.parent, expectedParent) {
		return 0, errRuntimeHeadChanged
	}

	return c.fsm.Height(), nil
}

/* FINISH ALPHA CHANGE 1450 -- compare runtime and blockchain parents by number and hash -- */

/* START ALPHA CHANGE 306 -- remove runtime bridge-enabled switch -- */
/* FINISH ALPHA CHANGE 306 -- remove runtime bridge-enabled switch -- */

// OnBlockInserted is called whenever fsm or syncer inserts new block.
// The boolean result reports whether the runtime actually advanced.
func (c *consensusRuntime) OnBlockInserted(fullBlock *types.FullBlock) (bool, error) {
	if fullBlock == nil || fullBlock.Block == nil || fullBlock.Block.Header == nil {
		return false, errors.New("cannot advance consensus runtime with nil block")
	}

	startTime := time.Now().UTC()

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.lastBuiltBlock != nil && c.lastBuiltBlock.Number >= fullBlock.Block.Number() {
		c.logger.Debug("on block inserted already handled",
			"current", c.lastBuiltBlock.Number, "block", fullBlock.Block.Number())

		/* START ALPHA CHANGE 1470 -- report duplicate block without runtime advancement -- */
		return false, nil
		/* FINISH ALPHA CHANGE 1470 -- report duplicate block without runtime advancement -- */
	}

	if err := updateBlockMetrics(fullBlock.Block, c.lastBuiltBlock); err != nil {
		c.logger.Error("failed to update block metrics", "error", err)
	}

	// after the block has been written we reset the txpool so that the old transactions are removed
	c.config.txPool.ResetWithHeaders(fullBlock.Block.Header)

	var (
		epoch = c.epoch
		err   error
		// calculation of epoch and sprint end does not consider slashing currently

		isEndOfEpoch = c.isFixedSizeOfEpochMet(fullBlock.Block.Header.Number, epoch)
	)

	// begin DB transaction
	dbTx, err := c.state.beginDBTransaction(true)
	if err != nil {
		return false, fmt.Errorf(
			"failed to begin DB transaction on block finalization for block %d: %w",
			fullBlock.Block.Number(),
			err,
		)
	}

	defer dbTx.Rollback() //nolint:errcheck

	/* START ALPHA CHANGE 343 -- remove legacy event-provider block scan -- */
	/* FINISH ALPHA CHANGE 343 -- remove legacy event-provider block scan -- */

	postBlock := &PostBlockRequest{
		FullBlock:          fullBlock,
		Epoch:              epoch.Number,
		IsEpochEndingBlock: isEndOfEpoch,
		DBTx:               dbTx,
	}

	/* START ALPHA CHANGE 307 -- remove bridge state-sync post-block hook -- */
	// update proposer priorities
	/* FINISH ALPHA CHANGE 307 -- remove bridge state-sync post-block hook -- */
	/* START ALPHA CHANGE 1457 -- stage proposer state until database commit succeeds -- */
	nextProposerSnapshot, err := c.proposerCalculator.PreparePostBlock(postBlock)
	if err != nil {
		return false, fmt.Errorf(
			"could not prepare proposer calculator for block %d: %w",
			fullBlock.Block.Number(),
			err,
		)
	}
	/* FINISH ALPHA CHANGE 1457 -- stage proposer state until database commit succeeds -- */

	// handle transfer events that happened in block
	if err := c.stakeManager.PostBlock(postBlock); err != nil {
		return false, fmt.Errorf(
			"failed to post block in stake manager for block %d: %w",
			fullBlock.Block.Number(),
			err,
		)
	}

	/* START ALPHA CHANGE 3887 -- reconcile epochs when callbacks pass the current boundary -- */
	// A later callback can arrive before the epoch-ending callback.
	// Reconstruct from this block's state once the old boundary is covered.
	/* START ALPHA CHANGE 308 -- remove bridge state-sync relayer post-block hook -- */
	if number := fullBlock.Block.Number(); number >= epoch.FirstBlockInEpoch &&
		number-epoch.FirstBlockInEpoch >= c.config.INNBCBFTConfig.EpochSize-1 {
		/* FINISH ALPHA CHANGE 308 -- remove bridge state-sync relayer post-block hook -- */
		if epoch, err = c.restartEpoch(fullBlock.Block.Header, dbTx); err != nil {
			return false, fmt.Errorf(
				"failed to restart epoch after block %d: %w",
				fullBlock.Block.Number(),
				err,
			)
		}
	}
	/* FINISH ALPHA CHANGE 3887 -- reconcile epochs when callbacks pass the current boundary -- */

	/* START ALPHA CHANGE 344 -- remove legacy event-provider progress tracking -- */
	/* FINISH ALPHA CHANGE 344 -- remove legacy event-provider progress tracking -- */

	// commit DB transaction
	if err := dbTx.Commit(); err != nil {
		return false, fmt.Errorf(
			"failed to commit PostBlock transaction for block %d: %w",
			fullBlock.Block.Number(),
			err,
		)
	}

	// Publish in-memory runtime state only after the database transaction commits.
	c.proposerCalculator.ApplySnapshot(nextProposerSnapshot)
	c.epoch = epoch
	c.lastBuiltBlock = fullBlock.Block.Header

	/* START ALPHA CHANGE 309 -- remove rootchain checkpoint post-block hook -- */
	/* FINISH ALPHA CHANGE 309 -- remove rootchain checkpoint post-block hook -- */

	endTime := time.Now().UTC()

	c.logger.Debug("OnBlockInserted finished", "elapsedTime", endTime.Sub(startTime),
		"epoch", epoch.Number, "block", fullBlock.Block.Number())

	return true, nil
}

// FSM creates a new instance of fsm
func (c *consensusRuntime) FSM() error {
	sharedData, err := c.getGuardedData()
	if err != nil {
		return fmt.Errorf("cannot create fsm: %w", err)
	}

	parent, epoch, proposerSnapshot := sharedData.lastBuiltBlock, sharedData.epoch, sharedData.proposerSnapshot

	if !epoch.Validators.ContainsNodeID(c.config.Key.String()) {
		return errNotAValidator
	}

	blockBuilder, err := c.config.blockchain.NewBlockBuilder(
		parent,
		types.Address(c.config.Key.Address()),
		c.config.txPool,
		c.config.INNBCBFTConfig.BlockTime.Duration,
		c.logger,
	)

	if err != nil {
		return fmt.Errorf("cannot create block builder for fsm: %w", err)
	}

	pendingBlockNumber := parent.Number + 1
	// calculation of epoch end does not consider slashing currently
	/* START ALPHA CHANGE 337 -- remove bridge sprint calculation from FSM setup -- */
	isEndOfEpoch := c.isFixedSizeOfEpochMet(pendingBlockNumber, epoch)
	/* FINISH ALPHA CHANGE 337 -- remove bridge sprint calculation from FSM setup -- */

	valSet := validator.NewValidatorSet(epoch.Validators, c.logger)

	/* START ALPHA CHANGE 310 -- remove bridge exit event root from FSM creation -- */
	ff := &fsm{
		config:          c.config.INNBCBFTConfig,
		parent:          parent,
		backend:         c.config.blockchain,
		innbcbftBackend: c.config.innbcbftBackend,
		epochNumber:     epoch.Number,
		/* FINISH ALPHA CHANGE 310 -- remove bridge exit event root from FSM creation -- */
		blockBuilder: blockBuilder,
		validators:   valSet,
		isEndOfEpoch: isEndOfEpoch,
		/* START ALPHA CHANGE 338 -- remove bridge sprint flag from FSM setup -- */
		proposerSnapshot: proposerSnapshot,
		/* FINISH ALPHA CHANGE 338 -- remove bridge sprint flag from FSM setup -- */
		logger: c.logger.Named("fsm"),
	}

	/* START ALPHA CHANGE 311 -- remove bridge commitment creation from sprint handling -- */
	if isEndOfEpoch {
		/* FINISH ALPHA CHANGE 311 -- remove bridge commitment creation from sprint handling -- */
		/* START ALPHA CHANGE 280 -- validate INNBC registry validator set before epoch commit data -- */
		ff.newValidatorsDelta, err = c.stakeManager.UpdateValidatorSet(epoch.Number, epoch.Validators.Copy())
		if err != nil {
			return fmt.Errorf("cannot update validator set on epoch ending: %w", err)
		}
		/* FINISH ALPHA CHANGE 280 -- validate INNBC registry validator set before epoch commit data -- */

		/* START ALPHA CHANGE 575 -- stop assigning obsolete reward distribution input to FSM -- */
		ff.commitEpochInput, err = c.calculateCommitEpochInput(parent, epoch)
		/* FINISH ALPHA CHANGE 575 -- stop assigning obsolete reward distribution input to FSM -- */
		if err != nil {
			return fmt.Errorf("cannot calculate commit epoch info: %w", err)
		}
	}

	c.logger.Info(
		"[FSM built]",
		"epoch", epoch.Number,
		"endOfEpoch", isEndOfEpoch,
		/* START ALPHA CHANGE 339 -- remove bridge sprint log field -- */
		/* FINISH ALPHA CHANGE 339 -- remove bridge sprint log field -- */
	)

	/* START ALPHA CHANGE 1451 -- prevent publication of an FSM built from a stale runtime parent -- */
	c.lock.Lock()
	defer c.lock.Unlock()

	if !sameBlockHeader(c.lastBuiltBlock, parent) {
		return errRuntimeHeadChanged
	}

	c.fsm = ff

	return nil
	/* FINISH ALPHA CHANGE 1451 -- prevent publication of an FSM built from a stale runtime parent -- */
}

// restartEpoch resets the previously run epoch and moves to the next one
// returns *epochMetadata different from nil if the lastEpoch is not the current one and everything was successful
func (c *consensusRuntime) restartEpoch(
	header *types.Header,
	/* START ALPHA CHANGE 1530 -- use the MDBX consensus transaction during epoch restart -- */
	dbTx *stateTx,
	/* FINISH ALPHA CHANGE 1530 -- use the MDBX consensus transaction during epoch restart -- */
) (*epochMetadata, error) {
	lastEpoch := c.epoch

	systemState, err := c.getSystemState(header)
	if err != nil {
		return nil, fmt.Errorf("get system state: %w", err)
	}

	epochNumber, err := systemState.GetEpoch()
	if err != nil {
		return nil, fmt.Errorf("get epoch: %w", err)
	}

	if lastEpoch != nil {
		// Epoch might be already in memory, if its the same number do nothing -> just return provided last one
		// Otherwise, reset the epoch metadata and restart the async services
		if lastEpoch.Number == epochNumber {
			return lastEpoch, nil
		}
	}

	validatorSet, err := c.config.innbcbftBackend.GetValidatorsWithTx(header.Number, nil, dbTx)
	if err != nil {
		return nil, fmt.Errorf("restart epoch - cannot get validators: %w", err)
	}

	updateEpochMetrics(epochMetadata{
		Number:     epochNumber,
		Validators: validatorSet,
	})

	firstBlockInEpoch, err := c.getFirstBlockOfEpoch(epochNumber, header)
	if err != nil {
		return nil, err
	}

	if err := c.state.EpochStore.cleanEpochsFromDB(dbTx); err != nil {
		c.logger.Error("Could not clean previous epochs from db.", "error", err)
	}

	if err := c.state.EpochStore.insertEpoch(epochNumber, dbTx); err != nil {
		return nil, fmt.Errorf("an error occurred while inserting new epoch in db. Reason: %w", err)
	}

	c.logger.Info(
		"restartEpoch",
		"block number", header.Number,
		"epoch", epochNumber,
		"validators", validatorSet.Len(),
		"firstBlockInEpoch", firstBlockInEpoch,
	)

	/* START ALPHA CHANGE 312 -- remove bridge state-sync post-epoch hook -- */
	/* FINISH ALPHA CHANGE 312 -- remove bridge state-sync post-epoch hook -- */

	return &epochMetadata{
		Number:            epochNumber,
		Validators:        validatorSet,
		FirstBlockInEpoch: firstBlockInEpoch,
	}, nil
}

// calculateCommitEpochInput calculates commit epoch input data for the epoch-ending block.
func (c *consensusRuntime) calculateCommitEpochInput(
	currentBlock *types.Header,
	epoch *epochMetadata,
) (*contractsapi.CommitEpochValidatorSetFn, error) {
	/* START ALPHA CHANGE 589 -- remove reward uptime accounting from commit epoch input calculation -- */
	epochID := epoch.Number
	/* FINISH ALPHA CHANGE 589 -- remove reward uptime accounting from commit epoch input calculation -- */

	commitEpoch := &contractsapi.CommitEpochValidatorSetFn{
		ID: new(big.Int).SetUint64(epochID),
		Epoch: &contractsapi.Epoch{
			StartBlock: new(big.Int).SetUint64(epoch.FirstBlockInEpoch),
			EndBlock:   new(big.Int).SetUint64(currentBlock.Number + 1),
			EpochRoot:  types.Hash{},
		},
	}

	/* START ALPHA CHANGE 572 -- remove reward distribution input creation from consensus runtime -- */
	/* FINISH ALPHA CHANGE 572 -- remove reward distribution input creation from consensus runtime -- */

	return commitEpoch, nil
}

/* START ALPHA CHANGE 313 -- remove bridge proof endpoints from consensus runtime -- */
/* FINISH ALPHA CHANGE 313 -- remove bridge proof endpoints from consensus runtime -- */

// setIsActiveValidator updates the activeValidatorFlag field
func (c *consensusRuntime) setIsActiveValidator(isActiveValidator bool) {
	c.activeValidatorFlag.Store(isActiveValidator)
}

// isActiveValidator indicates if node is in validator set or not
func (c *consensusRuntime) IsActiveValidator() bool {
	return c.activeValidatorFlag.Load()
}

// isFixedSizeOfEpochMet checks if epoch reached its end that was configured by its default size
// this is only true if no slashing occurred in the given epoch
func (c *consensusRuntime) isFixedSizeOfEpochMet(blockNumber uint64, epoch *epochMetadata) bool {
	return epoch.FirstBlockInEpoch+c.config.INNBCBFTConfig.EpochSize-1 == blockNumber
}

// isFixedSizeOfSprintMet checks if an end of an sprint is reached with the current block
func (c *consensusRuntime) isFixedSizeOfSprintMet(blockNumber uint64, epoch *epochMetadata) bool {
	return (blockNumber-epoch.FirstBlockInEpoch+1)%c.config.INNBCBFTConfig.SprintSize == 0
}

// getSystemState builds SystemState instance for the most current block header
func (c *consensusRuntime) getSystemState(header *types.Header) (SystemState, error) {
	provider, err := c.config.blockchain.GetStateProviderForBlock(header)
	if err != nil {
		return nil, err
	}

	return c.config.blockchain.GetSystemState(provider), nil
}

/* START ALPHA CHANGE 2172 -- withhold votes without blaming proposals for local state failures -- */
/* START ALPHA CHANGE 2583 -- validate without selecting a shared insertion target -- */
func (c *consensusRuntime) IsValidProposal(rawProposal []byte) bool {
	return c.ExecuteProposal(rawProposal) != nil
}

/* FINISH ALPHA CHANGE 2583 -- validate without selecting a shared insertion target -- */

/* FINISH ALPHA CHANGE 2172 -- withhold votes without blaming proposals for local state failures -- */

func (c *consensusRuntime) IsValidValidator(msg *proto.Message) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	/* START ALPHA CHANGE 1842 -- reject invalid consensus senders without log amplification -- */
	if msg == nil || c.fsm == nil {
		recordRejectedConsensusMessage()

		return false
	}

	if err := c.fsm.ValidateSender(msg); err != nil {
		recordRejectedConsensusMessage()

		return false
	}
	/* FINISH ALPHA CHANGE 1842 -- reject invalid consensus senders without log amplification -- */

	return true
}

/* START ALPHA CHANGE 2633 -- synchronize proposer cache reads, calculation, and publication -- */
func (c *consensusRuntime) IsProposer(id []byte, height, round uint64) bool {
	/*
		Callers must not hold c.lock. Acquire the proposer gate first so
		queued checks leave runtime publication free to proceed.
	*/
	c.proposerLock.Lock()
	defer c.proposerLock.Unlock()

	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.fsm == nil || c.fsm.proposerSnapshot == nil {
		recordRejectedConsensusMessage()

		return false
	}

	/* START ALPHA CHANGE 1848 -- avoid network-triggerable proposer log amplification -- */
	nextProposer, err := c.fsm.proposerSnapshot.CalcProposer(round, height)
	if err != nil {
		recordRejectedConsensusMessage()

		return false
	}

	return bytes.Equal(id, nextProposer[:])
	/* FINISH ALPHA CHANGE 1848 -- avoid network-triggerable proposer log amplification -- */
}

/* FINISH ALPHA CHANGE 2633 -- synchronize proposer cache reads, calculation, and publication -- */

func (c *consensusRuntime) IsValidProposalHash(proposal *proto.Proposal, hash []byte) bool {
	/* START ALPHA CHANGE 1843 -- make proposal hash validation nil-safe and quiet -- */
	if proposal == nil || len(proposal.RawProposal) == 0 {
		recordRejectedConsensusMessage()

		return false
	}

	block := types.Block{}
	if err := block.UnmarshalRLP(proposal.RawProposal); err != nil {
		recordRejectedConsensusMessage()

		return false
	}

	extra, err := GetIbftExtra(block.Header.ExtraData)
	if err != nil {
		recordRejectedConsensusMessage()

		return false
	}

	/* START ALPHA CHANGE 2565 -- constrain checkpoint rounds to the proposal context -- */
	if extra.Checkpoint == nil || extra.Checkpoint.BlockRound > proposal.Round {
		recordRejectedConsensusMessage()

		return false
	}
	/* FINISH ALPHA CHANGE 2565 -- constrain checkpoint rounds to the proposal context -- */

	proposalHash, err := extra.Checkpoint.Hash(c.config.blockchain.GetChainID(), block.Number(), block.Hash())
	if err != nil {
		recordRejectedConsensusMessage()

		return false
	}

	if !bytes.Equal(proposalHash.Bytes(), hash) {
		recordRejectedConsensusMessage()

		return false
	}

	return true
	/* FINISH ALPHA CHANGE 1843 -- make proposal hash validation nil-safe and quiet -- */
}

func (c *consensusRuntime) IsValidCommittedSeal(proposalHash []byte, committedSeal *messages.CommittedSeal) bool {
	/* START ALPHA CHANGE 1844 -- make committed seal validation nil-safe and quiet -- */
	if committedSeal == nil {
		recordRejectedConsensusMessage()

		return false
	}

	if err := c.fsm.ValidateCommit(committedSeal.Signer, committedSeal.Signature, proposalHash); err != nil {
		recordRejectedConsensusMessage()

		return false
	}

	return true
	/* FINISH ALPHA CHANGE 1844 -- make committed seal validation nil-safe and quiet -- */
}

/* START ALPHA CHANGE 2583 -- construct proposals without publishing a shared target -- */
func (c *consensusRuntime) BuildProposal(view *proto.View) []byte {
	proposal, _ := c.BuildExecutedProposal(view)

	return proposal
}

/* FINISH ALPHA CHANGE 2583 -- construct proposals without publishing a shared target -- */

// InsertProposal inserts a proposal with the specified committed seals
/* START ALPHA CHANGE 2583 -- require the selected execution result and propagate insertion failures -- */
func (c *consensusRuntime) InsertProposal(
	proposal *proto.Proposal,
	committedSeals []*messages.CommittedSeal,
	execution *core.ProposalExecution,
) error {
	return c.insertExecutedProposal(
		proposal,
		committedSeals,
		execution,
		func(fullBlock *types.FullBlock) error {
			_, err := c.OnBlockInserted(fullBlock)

			return err
		},
	)
}

/* FINISH ALPHA CHANGE 2583 -- require the selected execution result and propagate insertion failures -- */

// ID return ID (address actually) of the current node
func (c *consensusRuntime) ID() []byte {
	return c.config.Key.Address().Bytes()
}

// GetVotingPowers returns map of validators addresses and their voting powers for the specified height.
func (c *consensusRuntime) GetVotingPowers(height uint64) (map[string]*big.Int, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.fsm == nil {
		return nil, errors.New("getting voting power failed - backend is not initialized")
	} else if c.fsm.Height() != height {
		return nil, fmt.Errorf("getting voting power failed - backend is not initialized for height %d, fsm height %d",
			height, c.fsm.Height())
	}

	return c.fsm.validators.GetVotingPowers(), nil
}

// BuildPrePrepareMessage builds a PREPREPARE message based on the passed in proposal
func (c *consensusRuntime) BuildPrePrepareMessage(
	rawProposal []byte,
	certificate *proto.RoundChangeCertificate,
	view *proto.View,
) *proto.Message {
	if len(rawProposal) == 0 {
		c.logger.Error("can not build pre-prepare message, since proposal is empty")

		return nil
	}

	block := types.Block{}
	if err := block.UnmarshalRLP(rawProposal); err != nil {
		c.logger.Error(fmt.Sprintf("cannot unmarshal RLP: %s", err))

		return nil
	}

	extra, err := GetIbftExtra(block.Header.ExtraData)
	if err != nil {
		c.logger.Error("failed to retrieve extra for block %d: %w", block.Number(), err)

		return nil
	}

	proposalHash, err := extra.Checkpoint.Hash(c.config.blockchain.GetChainID(), block.Number(), block.Hash())
	if err != nil {
		c.logger.Error("failed to calculate proposal hash", "block number", block.Number(), "error", err)

		return nil
	}

	proposal := &proto.Proposal{
		RawProposal: rawProposal,
		Round:       view.Round,
	}

	msg := proto.Message{
		View: view,
		From: c.ID(),
		Type: proto.MessageType_PREPREPARE,
		Payload: &proto.Message_PreprepareData{
			PreprepareData: &proto.PrePrepareMessage{
				Proposal:     proposal,
				ProposalHash: proposalHash.Bytes(),
				Certificate:  certificate,
			},
		},
	}

	message, err := c.config.Key.SignIBFTMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message", "error", err)

		return nil
	}

	return message
}

// BuildPrepareMessage builds a PREPARE message based on the passed in proposal
func (c *consensusRuntime) BuildPrepareMessage(proposalHash []byte, view *proto.View) *proto.Message {
	msg := proto.Message{
		View: view,
		From: c.ID(),
		Type: proto.MessageType_PREPARE,
		Payload: &proto.Message_PrepareData{
			PrepareData: &proto.PrepareMessage{
				ProposalHash: proposalHash,
			},
		},
	}

	message, err := c.config.Key.SignIBFTMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message.", "error", err)

		return nil
	}

	return message
}

// BuildCommitMessage builds a COMMIT message based on the passed in proposal
func (c *consensusRuntime) BuildCommitMessage(proposalHash []byte, view *proto.View) *proto.Message {
	committedSeal, err := c.config.Key.SignWithDomain(proposalHash, signer.DomainCheckpointManager)
	if err != nil {
		c.logger.Error("Cannot create committed seal message.", "error", err)

		return nil
	}

	msg := proto.Message{
		View: view,
		From: c.ID(),
		Type: proto.MessageType_COMMIT,
		Payload: &proto.Message_CommitData{
			CommitData: &proto.CommitMessage{
				ProposalHash:  proposalHash,
				CommittedSeal: committedSeal,
			},
		},
	}

	message, err := c.config.Key.SignIBFTMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message", "Error", err)

		return nil
	}

	return message
}

// BuildRoundChangeMessage builds a ROUND_CHANGE message based on the passed in proposal
func (c *consensusRuntime) BuildRoundChangeMessage(
	proposal *proto.Proposal,
	certificate *proto.PreparedCertificate,
	view *proto.View,
) *proto.Message {
	msg := proto.Message{
		View: view,
		From: c.ID(),
		Type: proto.MessageType_ROUND_CHANGE,
		Payload: &proto.Message_RoundChangeData{
			RoundChangeData: &proto.RoundChangeMessage{
				LastPreparedProposal:      proposal,
				LatestPreparedCertificate: certificate,
			}},
	}

	signedMsg, err := c.config.Key.SignIBFTMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message", "Error", err)

		return nil
	}

	return signedMsg
}

// getFirstBlockOfEpoch returns the first block of epoch in which provided header resides
func (c *consensusRuntime) getFirstBlockOfEpoch(epochNumber uint64, latestHeader *types.Header) (uint64, error) {
	if latestHeader.Number == 0 {
		// if we are starting the chain, we know that the first block is block 1
		return 1, nil
	}

	blockHeader := latestHeader

	blockExtra, err := GetIbftExtra(latestHeader.ExtraData)
	if err != nil {
		return 0, err
	}

	if epochNumber != blockExtra.Checkpoint.EpochNumber {
		// its a regular epoch ending. No out of sync happened
		return latestHeader.Number + 1, nil
	}

	// node was out of sync, so we need to figure out what was the first block of the given epoch
	epoch := blockExtra.Checkpoint.EpochNumber

	var firstBlockInEpoch uint64

	for blockExtra.Checkpoint.EpochNumber == epoch {
		firstBlockInEpoch = blockHeader.Number
		blockHeader, blockExtra, err = getBlockData(blockHeader.Number-1, c.config.blockchain)

		if err != nil {
			return 0, err
		}
	}

	return firstBlockInEpoch, nil
}

/*START ALPHA CHANGE 50 -- remove unused top-level getSealersForBlock helper --*/
/*FINISH ALPHA CHANGE 50 -- remove unused top-level getSealersForBlock helper --*/
