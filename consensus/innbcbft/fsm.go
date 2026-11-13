package innbcbft

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/armon/go-metrics"
	hcf "github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/bitmap"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"

	/* START ALPHA CHANGE 1871 -- use INNBC-owned IBFT message packages -- */
	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"

	/* FINISH ALPHA CHANGE 1871 -- use INNBC-owned IBFT message packages -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/contracts"

	/* START ALPHA CHANGE 3313 -- validate timestamps with shared chain-clock arithmetic -- */
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	/* FINISH ALPHA CHANGE 3313 -- validate timestamps with shared chain-clock arithmetic -- */
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
)

type blockBuilder interface {
	Reset() error
	WriteTx(*types.Transaction) error
	/* START ALPHA CHANGE 2159 -- migrate block builder Fill interface -- */
	// Fill returns a fatal local execution error without consuming its transaction.
	Fill() error
	/* FINISH ALPHA CHANGE 2159 -- migrate block builder Fill interface -- */
	Build(func(h *types.Header)) (*types.FullBlock, error)
	GetState() *state.Transition
	Receipts() []*types.Receipt
}

var (
	errCommitEpochTxDoesNotExist   = errors.New("commit epoch transaction is not found in the epoch ending block")
	errCommitEpochTxNotExpected    = errors.New("didn't expect commit epoch transaction in a non epoch ending block")
	errCommitEpochTxSingleExpected = errors.New("only one commit epoch transaction is allowed " +
		"in an epoch ending block")
	/* START ALPHA CHANGE 563 -- remove pre-funded reward distribution transaction errors -- */
	/* FINISH ALPHA CHANGE 563 -- remove pre-funded reward distribution transaction errors -- */
	errProposalDontMatch = errors.New("failed to insert proposal, because the validated proposal " +
		"is either nil or it does not match the received one")
	errValidatorSetDeltaMismatch           = errors.New("validator set delta mismatch")
	errValidatorsUpdateInNonEpochEnding    = errors.New("trying to update validator set in a non epoch ending block")
	errValidatorDeltaNilInEpochEndingBlock = errors.New("validator set delta is nil in epoch ending block")
)

type fsm struct {
	// INNBCBFT consensus protocol configuration
	config *INNBCBFTConfig

	// parent block header
	parent *types.Header

	// backend implements methods for retrieving data from block chain
	backend blockchainBackend

	// innbcbftBackend implements methods needed from the innbcbft
	innbcbftBackend innbcbftBackend

	// validators is the list of validators for this round
	validators validator.ValidatorSet

	// proposerSnapshot keeps information about new proposer
	proposerSnapshot *ProposerSnapshot

	// blockBuilder is the block builder for proposers
	blockBuilder blockBuilder

	// epochNumber denotes current epoch number
	epochNumber uint64

	// commitEpochInput holds info about a single epoch
	// It is populated only for epoch-ending blocks.
	commitEpochInput *contractsapi.CommitEpochValidatorSetFn

	/* START ALPHA CHANGE 564 -- remove pre-funded reward distribution input from FSM -- */
	/* FINISH ALPHA CHANGE 564 -- remove pre-funded reward distribution input from FSM -- */

	// isEndOfEpoch indicates if epoch reached its end
	isEndOfEpoch bool

	/* START ALPHA CHANGE 329 -- remove bridge sprint commitment fields from FSM -- */
	// logger instance
	logger hcf.Logger

	/* START ALPHA CHANGE 2582 -- keep execution targets on proposal-owned FSM copies -- */
	// target belongs to a single execution; runtime selection carries its result.
	target *types.FullBlock
	/* FINISH ALPHA CHANGE 2582 -- keep execution targets on proposal-owned FSM copies -- */
	/* FINISH ALPHA CHANGE 329 -- remove bridge sprint commitment fields from FSM -- */

	// newValidatorsDelta carries the updates of validator set on epoch ending block
	newValidatorsDelta *validator.ValidatorSetDelta
}

// BuildProposal builds a proposal for the current round (used if proposer)
func (f *fsm) BuildProposal(currentRound uint64) ([]byte, error) {
	start := time.Now().UTC()
	defer metrics.SetGauge([]string{consensusMetricsPrefix, "block_building_time"},
		float32(time.Now().UTC().Sub(start).Seconds()))

	parent := f.parent

	extraParent, err := GetIbftExtra(parent.ExtraData)
	if err != nil {
		return nil, err
	}

	extra := &Extra{Parent: extraParent.Committed}
	// for non-epoch ending blocks, currentValidatorsHash is the same as the nextValidatorsHash
	nextValidators := f.validators.Accounts()

	if err := f.blockBuilder.Reset(); err != nil {
		return nil, fmt.Errorf("failed to initialize block builder: %w", err)
	}

	if f.isEndOfEpoch {
		tx, err := f.createCommitEpochTx()
		if err != nil {
			return nil, err
		}

		if err := f.blockBuilder.WriteTx(tx); err != nil {
			return nil, fmt.Errorf("failed to apply commit epoch transaction: %w", err)
		}

		/* START ALPHA CHANGE 565 -- stop adding pre-funded reward distribution transaction to proposals -- */
		/* FINISH ALPHA CHANGE 565 -- stop adding pre-funded reward distribution transaction to proposals -- */
	}

	/* START ALPHA CHANGE 330 -- remove bridge commitment transaction from proposal build -- */
	/* FINISH ALPHA CHANGE 330 -- remove bridge commitment transaction from proposal build -- */

	/* START ALPHA CHANGE 2159 -- abort proposal construction when block fill fails -- */
	// fill the block with transactions
	if err := f.blockBuilder.Fill(); err != nil {
		return nil, fmt.Errorf("failed to fill proposal: %w", err)
	}
	/* FINISH ALPHA CHANGE 2159 -- abort proposal construction when block fill fails -- */

	if f.isEndOfEpoch {
		nextValidators, err = nextValidators.ApplyDelta(f.newValidatorsDelta)
		if err != nil {
			return nil, err
		}

		extra.Validators = f.newValidatorsDelta
	}

	currentValidatorsHash, err := f.validators.Accounts().Hash()
	if err != nil {
		return nil, err
	}

	nextValidatorsHash, err := nextValidators.Hash()
	if err != nil {
		return nil, err
	}

	extra.Checkpoint = &CheckpointData{
		BlockRound:            currentRound,
		EpochNumber:           f.epochNumber,
		CurrentValidatorsHash: currentValidatorsHash,
		NextValidatorsHash:    nextValidatorsHash,
		/* START ALPHA CHANGE 331 -- use zero event root after bridge exit tree pruning -- */
		EventRoot: types.ZeroHash,
		/* FINISH ALPHA CHANGE 331 -- use zero event root after bridge exit tree pruning -- */
	}

	f.logger.Debug("[Build Proposal]", "Current validators hash", currentValidatorsHash,
		"Next validators hash", nextValidatorsHash)

	stateBlock, err := f.blockBuilder.Build(func(h *types.Header) {
		h.ExtraData = extra.MarshalRLPTo(nil)
		h.MixHash = INNBCBFTMixDigest
	})

	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 815 -- self-validate locally built proposal header -- */
	/* START ALPHA CHANGE 3313 -- self-validate against the production interval -- */
	if err := validateHeaderFields(parent, stateBlock.Block.Header, f.config.BlockTime.Duration); err != nil {
		/* FINISH ALPHA CHANGE 3313 -- self-validate against the production interval -- */
		return nil, fmt.Errorf(
			"locally built proposal failed header validation (parent header# %d, current header#%d): %w",
			parent.Number,
			stateBlock.Block.Number(),
			err,
		)
	}
	/* FINISH ALPHA CHANGE 815 -- self-validate locally built proposal header -- */

	if f.logger.IsDebug() {
		checkpointHash, err := extra.Checkpoint.Hash(f.backend.GetChainID(), f.Height(), stateBlock.Block.Hash())
		if err != nil {
			return nil, fmt.Errorf("failed to calculate proposal hash: %w", err)
		}

		f.logger.Debug("[FSM Build Proposal]",
			"txs", len(stateBlock.Block.Transactions),
			"proposal hash", checkpointHash.String())
	}

	f.target = stateBlock

	return stateBlock.Block.MarshalRLP(), nil
}

/* START ALPHA CHANGE 332 -- remove bridge commitment state transaction helpers -- */
/* FINISH ALPHA CHANGE 332 -- remove bridge commitment state transaction helpers -- */

// getValidatorsTransition applies delta to the current validators,
func (f *fsm) getValidatorsTransition(delta *validator.ValidatorSetDelta) (validator.AccountSet, error) {
	nextValidators, err := f.validators.Accounts().ApplyDelta(delta)
	if err != nil {
		return nil, err
	}

	f.logger.Debug("getValidatorsTransition", "Next validators", nextValidators)

	return nextValidators, nil
}

// createCommitEpochTx create a StateTransaction, which invokes ValidatorSet smart contract
// and sends all the necessary metadata to it.
func (f *fsm) createCommitEpochTx() (*types.Transaction, error) {
	input, err := f.commitEpochInput.EncodeAbi()
	if err != nil {
		return nil, err
	}

	return createStateTransactionWithData(f.Height(), contracts.ValidatorSetContract, input), nil
}

/* START ALPHA CHANGE 566 -- remove pre-funded reward distribution state transaction builder -- */
/* FINISH ALPHA CHANGE 566 -- remove pre-funded reward distribution state transaction builder -- */

// ValidateCommit is used to validate that a given commit is valid
func (f *fsm) ValidateCommit(signerAddr []byte, seal []byte, proposalHash []byte) error {
	from := types.BytesToAddress(signerAddr)

	validator := f.validators.Accounts().GetValidatorMetadata(from)
	if validator == nil {
		return fmt.Errorf("unable to resolve validator %s", from)
	}

	signature, err := bls.UnmarshalSignature(seal)
	if err != nil {
		return fmt.Errorf("failed to unmarshall signature: %w", err)
	}

	if !signature.Verify(validator.BlsKey, proposalHash, signer.DomainCheckpointManager) {
		return fmt.Errorf("incorrect commit signature from %s", from)
	}

	return nil
}

// Validate validates a raw proposal (used if non-proposer)
func (f *fsm) Validate(proposal []byte) error {
	var block types.Block
	if err := block.UnmarshalRLP(proposal); err != nil {
		return fmt.Errorf("failed to validate, cannot decode block data. Error: %w", err)
	}

	// validate header fields
	/* START ALPHA CHANGE 3313 -- reject incorrect chain timestamps before execution -- */
	if err := validateHeaderFields(f.parent, block.Header, f.config.BlockTime.Duration); err != nil {
		/* FINISH ALPHA CHANGE 3313 -- reject incorrect chain timestamps before execution -- */
		return fmt.Errorf(
			"failed to validate header (parent header# %d, current header#%d): %w",
			f.parent.Number,
			block.Number(),
			err,
		)
	}

	extra, err := GetIbftExtra(block.Header.ExtraData)
	if err != nil {
		return fmt.Errorf("cannot get extra data:%w", err)
	}

	parentExtra, err := GetIbftExtra(f.parent.ExtraData)
	if err != nil {
		return err
	}

	if extra.Checkpoint == nil {
		return fmt.Errorf("checkpoint data for block %d is missing", block.Number())
	}

	/* START ALPHA CHANGE 2572 -- bind live checkpoint epochs to the FSM epoch before execution -- */
	if extra.Checkpoint.EpochNumber != f.epochNumber {
		return fmt.Errorf("invalid checkpoint epoch for block %d: expected %d, got %d",
			block.Number(), f.epochNumber, extra.Checkpoint.EpochNumber)
	}
	/* FINISH ALPHA CHANGE 2572 -- bind live checkpoint epochs to the FSM epoch before execution -- */

	if parentExtra.Checkpoint == nil {
		return fmt.Errorf("checkpoint data for parent block %d is missing", f.parent.Number)
	}

	if err := extra.ValidateParentSignatures(block.Number(), f.innbcbftBackend, nil, f.parent, parentExtra,
		f.backend.GetChainID(), signer.DomainCheckpointManager, f.logger); err != nil {
		return err
	}

	if err := f.VerifyStateTransactions(block.Transactions); err != nil {
		return err
	}

	currentValidators := f.validators.Accounts()

	// validate validators delta
	if f.isEndOfEpoch {
		if extra.Validators == nil {
			return errValidatorDeltaNilInEpochEndingBlock
		}

		if !extra.Validators.Equals(f.newValidatorsDelta) {
			return errValidatorSetDeltaMismatch
		}
	} else if extra.Validators != nil {
		// delta should be nil in non epoch ending blocks
		return errValidatorsUpdateInNonEpochEnding
	}

	nextValidators, err := f.getValidatorsTransition(extra.Validators)
	if err != nil {
		return err
	}

	// validate checkpoint data
	if err := extra.Checkpoint.Validate(parentExtra.Checkpoint,
		/* START ALPHA CHANGE 333 -- validate consensus checkpoint with zero bridge event root -- */
		currentValidators, nextValidators, types.ZeroHash); err != nil {
		/* FINISH ALPHA CHANGE 333 -- validate consensus checkpoint with zero bridge event root -- */
		return err
	}

	if f.logger.IsTrace() && block.Number() > 1 {
		validators, err := f.innbcbftBackend.GetValidators(block.Number()-2, nil)
		if err != nil {
			return fmt.Errorf("failed to retrieve validators:%w", err)
		}

		f.logger.Trace("[FSM Validate]", "Block", block.Number(), "parent validators", validators)
	}

	stateBlock, err := f.backend.ProcessBlock(f.parent, &block)
	if err != nil {
		return err
	}

	if f.logger.IsDebug() {
		checkpointHash, err := extra.Checkpoint.Hash(f.backend.GetChainID(), block.Number(), block.Hash())
		if err != nil {
			return fmt.Errorf("failed to calculate proposal hash: %w", err)
		}

		f.logger.Debug("[FSM Validate]", "txs", len(block.Transactions), "proposal hash", checkpointHash)
	}

	f.target = stateBlock

	return nil
}

// ValidateSender validates sender address and signature
func (f *fsm) ValidateSender(msg *proto.Message) error {
	msgNoSig, err := msg.PayloadNoSig()
	if err != nil {
		return err
	}

	signerAddress, err := wallet.RecoverAddressFromSignature(msg.Signature, msgNoSig)
	if err != nil {
		return fmt.Errorf("failed to recover address from signature: %w", err)
	}

	// verify the signature came from the sender
	if !bytes.Equal(msg.From, signerAddress.Bytes()) {
		return fmt.Errorf("signer address %s doesn't match From field", signerAddress.String())
	}

	// verify the sender is in the active validator set
	if !f.validators.Includes(signerAddress) {
		return fmt.Errorf("signer address %s is not included in validator set", signerAddress.String())
	}

	return nil
}

/* START ALPHA CHANGE 2292 -- retain live epoch requirements around shared state transaction checks -- */
func (f *fsm) VerifyStateTransactions(transactions []*types.Transaction) error {
	commitEpochTx, err := findCommitEpochTransaction(transactions)
	if err != nil {
		return err
	}

	if commitEpochTx != nil {
		if err := f.verifyCommitEpochTx(commitEpochTx); err != nil {
			return fmt.Errorf("error while verifying commit epoch transaction. error: %w", err)
		}
	} else if f.isEndOfEpoch {
		return errCommitEpochTxDoesNotExist
	}

	return nil
}

/* FINISH ALPHA CHANGE 2292 -- retain live epoch requirements around shared state transaction checks -- */

// Insert inserts the sealed proposal
func (f *fsm) Insert(proposal []byte, committedSeals []*messages.CommittedSeal) (*types.FullBlock, error) {
	newBlock := f.target

	var proposedBlock types.Block
	if err := proposedBlock.UnmarshalRLP(proposal); err != nil {
		return nil, fmt.Errorf("failed to insert proposal, block unmarshaling failed: %w", err)
	}

	/* START ALPHA CHANGE 2582 -- reject incomplete or mismatched insertion results -- */
	if newBlock == nil || newBlock.Block == nil || newBlock.Block.Header == nil ||
		newBlock.Block.Hash() != proposedBlock.Hash() {
		return nil, errProposalDontMatch
	}
	/* FINISH ALPHA CHANGE 2582 -- reject incomplete or mismatched insertion results -- */

	// In this function we should try to return little to no errors since
	// at this point everything we have to do is just commit something that
	// we should have already computed beforehand.
	extra, err := GetIbftExtra(newBlock.Block.Header.ExtraData)
	if err != nil {
		return nil, fmt.Errorf("failed to insert proposal, due to not being able to extract extra data: %w", err)
	}

	// create map for faster access to indexes
	nodeIDIndexMap := make(map[types.Address]int, f.validators.Len())
	for i, addr := range f.validators.Accounts().GetAddresses() {
		nodeIDIndexMap[addr] = i
	}

	// populated bitmap according to nodeId from validator set and committed seals
	// also populate slice of signatures
	bitmap := bitmap.Bitmap{}
	signatures := make(bls.Signatures, 0, len(committedSeals))

	for _, commSeal := range committedSeals {
		signerAddr := types.BytesToAddress(commSeal.Signer)

		index, exists := nodeIDIndexMap[signerAddr]
		if !exists {
			return nil, fmt.Errorf("invalid node id = %s", signerAddr.String())
		}

		s, err := bls.UnmarshalSignature(commSeal.Signature)
		if err != nil {
			return nil, fmt.Errorf("invalid signature = %s", commSeal.Signature)
		}

		signatures = append(signatures, s)

		bitmap.Set(uint64(index))
	}

	aggregatedSignature, err := signatures.Aggregate().Marshal()
	if err != nil {
		return nil, fmt.Errorf("could not aggregate seals: %w", err)
	}

	// include aggregated signature of all committed seals
	// also includes bitmap which contains all indexes from validator set which provides there seals
	extra.Committed = &Signature{
		AggregatedSignature: aggregatedSignature,
		Bitmap:              bitmap,
	}

	// Write extra data to header
	newBlock.Block.Header.ExtraData = extra.MarshalRLPTo(nil)

	if err := f.backend.CommitBlock(newBlock); err != nil {
		return nil, err
	}

	return newBlock, nil
}

// Height returns the height for the current round
func (f *fsm) Height() uint64 {
	return f.parent.Number + 1
}

// ValidatorSet returns the validator set for the current round
func (f *fsm) ValidatorSet() validator.ValidatorSet {
	return f.validators
}

// verifyCommitEpochTx creates commit epoch transaction and compares its hash with the one extracted from the block.
func (f *fsm) verifyCommitEpochTx(commitEpochTx *types.Transaction) error {
	if f.isEndOfEpoch {
		localCommitEpochTx, err := f.createCommitEpochTx()
		if err != nil {
			return err
		}

		if commitEpochTx.Hash != localCommitEpochTx.Hash {
			return fmt.Errorf(
				"invalid commit epoch transaction. Expected '%s', but got '%s' commit epoch transaction hash",
				localCommitEpochTx.Hash,
				commitEpochTx.Hash,
			)
		}

		return nil
	}

	return errCommitEpochTxNotExpected
}

/* START ALPHA CHANGE 570 -- remove pre-funded reward distribution verifier -- */
/* FINISH ALPHA CHANGE 570 -- remove pre-funded reward distribution verifier -- */

/* START ALPHA CHANGE 336 -- remove bridge commitment verifier -- */
/* FINISH ALPHA CHANGE 336 -- remove bridge commitment verifier -- */

/* START ALPHA CHANGE 3313 -- require a deterministic chain-clock successor -- */
func validateHeaderFields(parent *types.Header, header *types.Header, blockTime time.Duration) error {
	if parent == nil || header == nil {
		return fmt.Errorf("cannot validate a nil parent or block header")
	}
	/* FINISH ALPHA CHANGE 3313 -- require a deterministic chain-clock successor -- */
	// header extra data must be higher or equal to ExtraVanity = 32 in order to be compliant with Ethereum blocks
	if len(header.ExtraData) < ExtraVanity {
		return fmt.Errorf("extra-data shorter than %d bytes (%d)", ExtraVanity, len(header.ExtraData))
	}
	// verify parent hash
	if parent.Hash != header.ParentHash {
		return fmt.Errorf("incorrect header parent hash (parent=%s, header parent=%s)", parent.Hash, header.ParentHash)
	}
	// verify parent number
	if header.Number != parent.Number+1 {
		return fmt.Errorf("invalid number")
	}
	/* START ALPHA CHANGE 3313 -- enforce exact timestamp equality without a wall-clock allowance -- */
	expectedTimestamp, err := common.ExpectedBlockTimestamp(parent.Timestamp, blockTime)
	if err != nil {
		return fmt.Errorf("invalid block timestamp configuration: %w", err)
	}
	if header.Timestamp != expectedTimestamp {
		return fmt.Errorf(
			"invalid block timestamp: expected %d, got %d",
			expectedTimestamp,
			header.Timestamp,
		)
	}
	/* FINISH ALPHA CHANGE 3313 -- enforce exact timestamp equality without a wall-clock allowance -- */
	// verify header nonce is zero
	if header.Nonce != types.ZeroNonce {
		return fmt.Errorf("invalid nonce")
	}
	// verify that the gasUsed is <= gasLimit
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gas limit: have %v, max %v", header.GasUsed, header.GasLimit)
	}
	/* START ALPHA CHANGE 3313 -- exact positive-interval equality already guarantees increasing time -- */
	/* FINISH ALPHA CHANGE 3313 -- exact positive-interval equality already guarantees increasing time -- */
	// verify mix digest
	if header.MixHash != INNBCBFTMixDigest {
		return fmt.Errorf("mix digest is not correct")
	}
	/* START ALPHA CHANGE 811 -- require fixed INNBCBFT header difficulty -- */
	// INNBCBFT keeps the Ethereum-compatible header difficulty field, but it is fixed.
	if header.Difficulty != 1 {
		return fmt.Errorf("difficulty should be 1")
	}
	/* FINISH ALPHA CHANGE 811 -- require fixed INNBCBFT header difficulty -- */
	// calculated header hash must be correct
	if header.Hash != types.HeaderHash(header) {
		return fmt.Errorf("invalid header hash")
	}

	return nil
}

// createStateTransactionWithData creates a state transaction
// with provided target address and inputData parameter which is ABI encoded byte array.
func createStateTransactionWithData(blockNumber uint64, target types.Address, inputData []byte) *types.Transaction {
	tx := &types.Transaction{
		From:     contracts.SystemCaller,
		To:       &target,
		Type:     types.StateTx,
		Input:    inputData,
		Gas:      types.StateTransactionGasLimit,
		GasPrice: big.NewInt(0),
	}

	return tx.ComputeHash(blockNumber)
}
