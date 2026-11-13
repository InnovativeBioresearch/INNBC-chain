package state

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/addresslist"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/evm"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/precompiled"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/tracer"
	"github.com/innovativebioresearch/innbc-chain/types"
)

const (
	SpuriousDragonMaxCodeSize = 24576
	TxPoolMaxInitCodeSize     = 2 * SpuriousDragonMaxCodeSize

	TxGas                 uint64 = 21000 // Per transaction not creating a contract
	TxGasContractCreation uint64 = 53000 // Per transaction that creates a contract

	/* START ALPHA CHANGE 808 -- define runtime fee burn basis points denominator -- */
	feeBurnBasisPointsDenominator uint64 = 10000
	/* FINISH ALPHA CHANGE 808 -- define runtime fee burn basis points denominator -- */
)

/* START ALPHA CHANGE 809 -- define governance fee burn rate storage slot for runtime reads -- */
var feeBurnRateBpsSlot = crypto.Keccak256Hash([]byte("innbc.blockRewardGovernance.feeBurnRateBps"))

/* START ALPHA CHANGE 1140 -- define governance max base fee storage slot for runtime reads -- */
var maxBaseFeeWeiSlot = crypto.Keccak256Hash([]byte("innbc.blockRewardGovernance.maxBaseFeeWei"))

/* FINISH ALPHA CHANGE 1140 -- define governance max base fee storage slot for runtime reads -- */

/* START ALPHA CHANGE 964 -- read community reward rate from explicit runtime storage slot -- */
var communityRewardRateBpsSlot = crypto.Keccak256Hash([]byte("innbc.communityRewardPool.communityRewardRateBps"))

/* FINISH ALPHA CHANGE 964 -- read community reward rate from explicit runtime storage slot -- */

/* FINISH ALPHA CHANGE 809 -- define governance fee burn rate storage slot for runtime reads -- */

/* START ALPHA CHANGE 3807 -- make execution block-hash callbacks preserve read failures -- */

// GetHashByNumber looks up a block hash and preserves ancestor-read failures.
type GetHashByNumber = func(i uint64) (types.Hash, error)

type GetHashByNumberHelper = func(*types.Header) GetHashByNumber

/* FINISH ALPHA CHANGE 3807 -- make execution block-hash callbacks preserve read failures -- */

// Executor is the main entity
type Executor struct {
	logger  hclog.Logger
	config  *chain.Params
	state   State
	GetHash GetHashByNumberHelper

	PostHook        func(txn *Transition)
	GenesisPostHook func(*Transition) error
}

// NewExecutor creates a new executor
func NewExecutor(config *chain.Params, s State, logger hclog.Logger) *Executor {
	return &Executor{
		logger: logger,
		config: config,
		state:  s,
	}
}

/* START ALPHA CHANGE 1141 -- read max base fee cap from parent state -- */
/* START ALPHA CHANGE 2154 -- classify governance snapshot opening failures -- */
func (e *Executor) ReadMaxBaseFeeWei(root types.Hash) (uint64, error) {
	snap, err := e.state.NewSnapshotAt(root)
	if err != nil {
		return 0, WrapLocalStateFailure("open max-base-fee snapshot", err)
	}

	return readMaxBaseFeeWei(NewTxn(snap))
}

/* FINISH ALPHA CHANGE 2154 -- classify governance snapshot opening failures -- */

/* FINISH ALPHA CHANGE 1141 -- read max base fee cap from parent state -- */

func (e *Executor) WriteGenesis(
	alloc map[types.Address]*chain.GenesisAccount,
	initialStateRoot types.Hash) (types.Hash, error) {
	/* START ALPHA CHANGE 3104 -- reject malformed allocations before opening or modifying genesis state -- */
	if err := chain.ValidateGenesisAllocations(alloc); err != nil {
		return types.Hash{}, err
	}
	/* FINISH ALPHA CHANGE 3104 -- reject malformed allocations before opening or modifying genesis state -- */

	var (
		snap Snapshot
		err  error
	)

	/* START ALPHA CHANGE 2160 -- classify genesis snapshot opening failures -- */
	if initialStateRoot == types.ZeroHash {
		snap = e.state.NewSnapshot()
	} else {
		snap, err = e.state.NewSnapshotAt(initialStateRoot)
	}

	if err != nil {
		return types.Hash{}, WrapLocalStateFailure("open genesis snapshot", err)
	}
	/* FINISH ALPHA CHANGE 2160 -- classify genesis snapshot opening failures -- */

	txn := NewTxn(snap)
	config := e.config.Forks.At(0)

	env := runtime.TxContext{
		ChainID: e.config.ChainID,
	}

	transition := &Transition{
		logger:      e.logger,
		ctx:         env,
		state:       txn,
		auxState:    e.state,
		gasPool:     uint64(env.GasLimit),
		config:      config,
		precompiles: precompiled.NewPrecompiled(),
	}

	for addr, account := range alloc {
		if account.Balance != nil {
			txn.AddBalance(addr, account.Balance)
		}

		if account.Nonce != 0 {
			txn.SetNonce(addr, account.Nonce)
		}

		if len(account.Code) != 0 {
			txn.SetCode(addr, account.Code)
		}

		for key, value := range account.Storage {
			txn.SetState(addr, key, value)
		}
	}

	/* START ALPHA CHANGE 2160 -- stop genesis publication after allocation or hook state failures -- */
	if stateErr := txn.Error(); stateErr != nil {
		return types.Hash{}, stateErr
	}
	if e.GenesisPostHook != nil {
		hookErr := e.GenesisPostHook(transition)
		if stateErr := txn.Error(); stateErr != nil {
			return types.Hash{}, stateErr
		}
		if hookErr != nil {
			return types.Hash{}, fmt.Errorf("Error writing genesis block: %w", hookErr)
		}
	}
	/* FINISH ALPHA CHANGE 2160 -- stop genesis publication after allocation or hook state failures -- */

	objs, err := txn.Commit(false)
	if err != nil {
		return types.Hash{}, err
	}

	/* START ALPHA CHANGE 2160 -- retain genesis database publication failures -- */
	_, root, err := snap.Commit(objs)
	if err != nil {
		txn.setError(err)
		return types.Hash{}, txn.Error()
	}
	/* FINISH ALPHA CHANGE 2160 -- retain genesis database publication failures -- */

	return types.BytesToHash(root), nil
}

type BlockResult struct {
	Root     types.Hash
	Receipts []*types.Receipt
	TotalGas uint64
}

// ProcessBlock already does all the handling of the whole process
/* START ALPHA CHANGE 2157 -- check the retained failure even for an empty block -- */
func (e *Executor) ProcessBlock(
	parentRoot types.Hash,
	block *types.Block,
	blockCreator types.Address,
) (*Transition, error) {
	txn, err := e.BeginTxn(parentRoot, block.Header, blockCreator)
	if err != nil {
		return nil, err
	}

	for _, t := range block.Transactions {
		if t.Gas > block.Header.GasLimit {
			continue
		}

		if err = txn.Write(t); err != nil {
			return nil, err
		}
	}

	if err := txn.Error(); err != nil {
		return nil, err
	}

	return txn, nil
}

/* FINISH ALPHA CHANGE 2157 -- check the retained failure even for an empty block -- */

// StateAt returns snapshot at given root
func (e *Executor) State() State {
	return e.state
}

// StateAt returns snapshot at given root
func (e *Executor) StateAt(root types.Hash) (Snapshot, error) {
	return e.state.NewSnapshotAt(root)
}

// GetForksInTime returns the active forks at the given block height
func (e *Executor) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	return e.config.Forks.At(blockNumber)
}

func (e *Executor) BeginTxn(
	parentRoot types.Hash,
	header *types.Header,
	coinbaseReceiver types.Address,
) (*Transition, error) {
	forkConfig := e.config.Forks.At(header.Number)

	/* START ALPHA CHANGE 2154 -- classify execution snapshot opening failures -- */
	auxSnap2, err := e.state.NewSnapshotAt(parentRoot)
	if err != nil {
		return nil, WrapLocalStateFailure("open execution snapshot", err)
	}
	/* FINISH ALPHA CHANGE 2154 -- classify execution snapshot opening failures -- */

	newTxn := NewTxn(auxSnap2)

	/* START ALPHA CHANGE 810 -- read governance fee burn rate once per block from explicit storage slot -- */
	feeBurnRateBps, err := readFeeBurnRateBps(newTxn)
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 965 -- read community reward rate once per block and validate total fee split -- */
	communityRewardRateBps, err := readCommunityRewardRateBps(newTxn)
	if err != nil {
		return nil, err
	}

	if feeBurnRateBps+communityRewardRateBps > feeBurnBasisPointsDenominator {
		return nil, fmt.Errorf(
			"INNBC fee split exceeds basis points denominator: burn=%d community=%d denominator=%d",
			feeBurnRateBps,
			communityRewardRateBps,
			feeBurnBasisPointsDenominator,
		)
	}
	/* FINISH ALPHA CHANGE 965 -- read community reward rate once per block and validate total fee split -- */

	burnContract := contracts.INNBCFeeBurnVaultContract
	communityRewardContract := contracts.INNBCCommunityRewardPoolContract
	/* FINISH ALPHA CHANGE 810 -- read governance fee burn rate once per block from explicit storage slot -- */

	txCtx := runtime.TxContext{
		Coinbase:       coinbaseReceiver,
		Timestamp:      int64(header.Timestamp),
		Number:         int64(header.Number),
		Difficulty:     types.BytesToHash(new(big.Int).SetUint64(header.Difficulty).Bytes()),
		BaseFee:        new(big.Int).SetUint64(header.BaseFee),
		GasLimit:       int64(header.GasLimit),
		ChainID:        e.config.ChainID,
		BurnContract:   burnContract,
		FeeBurnRateBps: feeBurnRateBps,
		/* START ALPHA CHANGE 966 -- cache community reward fee settings in transaction context -- */
		CommunityRewardContract: communityRewardContract,
		CommunityRewardRateBps:  communityRewardRateBps,
		/* FINISH ALPHA CHANGE 966 -- cache community reward fee settings in transaction context -- */
	}

	txn := &Transition{
		logger:   e.logger,
		ctx:      txCtx,
		state:    newTxn,
		snap:     auxSnap2,
		getHash:  e.GetHash(header),
		auxState: e.state,
		config:   forkConfig,
		gasPool:  uint64(txCtx.GasLimit),

		receipts: []*types.Receipt{},
		totalGas: 0,

		evm:         evm.NewEVM(),
		precompiles: precompiled.NewPrecompiled(),
		PostHook:    e.PostHook,
	}

	// enable contract deployment allow list (if any)
	if e.config.ContractDeployerAllowList != nil {
		txn.deploymentAllowList = addresslist.NewAddressList(txn, contracts.AllowListContractsAddr)
	}

	if e.config.ContractDeployerBlockList != nil {
		txn.deploymentBlockList = addresslist.NewAddressList(txn, contracts.BlockListContractsAddr)
	}

	// enable transactions allow list (if any)
	if e.config.TransactionsAllowList != nil {
		txn.txnAllowList = addresslist.NewAddressList(txn, contracts.AllowListTransactionsAddr)
	}

	if e.config.TransactionsBlockList != nil {
		txn.txnBlockList = addresslist.NewAddressList(txn, contracts.BlockListTransactionsAddr)
	}

	/* START ALPHA CHANGE 439 -- remove bridge allow/block list transition runtime initialization -- */
	/* FINISH ALPHA CHANGE 439 -- remove bridge allow/block list transition runtime initialization -- */

	return txn, nil
}

type Transition struct {
	logger hclog.Logger

	// dummy
	auxState State
	snap     Snapshot

	config  chain.ForksInTime
	state   *Txn
	getHash GetHashByNumber
	ctx     runtime.TxContext
	gasPool uint64

	// result
	receipts []*types.Receipt
	totalGas uint64

	/* START ALPHA CHANGE 2655 -- retain commitment rejection without classifying it as local state failure -- */
	commitValidationErr error
	/* FINISH ALPHA CHANGE 2655 -- retain commitment rejection without classifying it as local state failure -- */

	PostHook func(t *Transition)

	// runtimes
	evm         *evm.EVM
	precompiles *precompiled.Precompiled

	// allow list runtimes
	deploymentAllowList *addresslist.AddressList
	deploymentBlockList *addresslist.AddressList
	txnAllowList        *addresslist.AddressList
	txnBlockList        *addresslist.AddressList
	/* START ALPHA CHANGE 440 -- remove bridge allow/block list transition fields -- */
	/* FINISH ALPHA CHANGE 440 -- remove bridge allow/block list transition fields -- */
}

func NewTransition(config chain.ForksInTime, snap Snapshot, radix *Txn) *Transition {
	return &Transition{
		config:      config,
		state:       radix,
		snap:        snap,
		evm:         evm.NewEVM(),
		precompiles: precompiled.NewPrecompiled(),
	}
}

/* START ALPHA CHANGE 2155 -- propagate state failures from execution overrides -- */
func (t *Transition) WithStateOverride(override types.StateOverride) error {
	if err := t.Error(); err != nil {
		return err
	}

	for addr, o := range override {
		if err := t.Error(); err != nil {
			return err
		}
		if o.State != nil && o.StateDiff != nil {
			return fmt.Errorf("cannot override both state and state diff")
		}

		if o.Nonce != nil {
			t.state.SetNonce(addr, *o.Nonce)
		}

		if o.Balance != nil {
			t.state.SetBalance(addr, o.Balance)
		}

		if o.Code != nil {
			t.state.SetCode(addr, o.Code)
		}

		if o.State != nil {
			t.state.SetFullStorage(addr, o.State)
		}

		for k, v := range o.StateDiff {
			t.state.SetState(addr, k, v)
		}
	}

	return t.Error()
}

/* FINISH ALPHA CHANGE 2155 -- propagate state failures from execution overrides -- */

func (t *Transition) TotalGas() uint64 {
	return t.totalGas
}

func (t *Transition) Receipts() []*types.Receipt {
	return t.receipts
}

var emptyFrom = types.Address{}

/* START ALPHA CHANGE 3153 -- share transaction cleanup between block execution and trace replay -- */

// FinalizeTransaction performs the existing transaction-boundary cleanup
// and returns the transaction's logs, removing them from transient state.
//
// Call it after Apply returns without an application error, including when
// the execution result contains an ordinary EVM failure. Capture any trace
// result before calling it. This method does not publish state or add receipts.
func (t *Transition) FinalizeTransaction() ([]*types.Log, error) {
	if err := t.Error(); err != nil {
		return nil, err
	}

	if err := t.state.CleanDeleteObjects(true); err != nil {
		t.state.setError(fmt.Errorf("failed to clean deleted objects: %w", err))
		return nil, t.Error()
	}
	if err := t.Error(); err != nil {
		return nil, err
	}

	return t.state.Logs(), nil
}

/* FINISH ALPHA CHANGE 3153 -- share transaction cleanup between block execution and trace replay -- */

// Write writes another transaction to the executor
/* START ALPHA CHANGE 2157 -- accept failed receipts only for ordinary EVM failures -- */
func (t *Transition) Write(txn *types.Transaction) error {
	if err := t.Error(); err != nil {
		return err
	}

	/* START ALPHA CHANGE 2931 -- validate signed chain IDs independently of cached sender metadata -- */
	if err := types.ValidateDynamicFeeChainID(
		txn,
		new(big.Int).SetUint64(uint64(t.ctx.ChainID)),
	); err != nil {
		return NewTransitionApplicationError(err, false)
	}
	/* FINISH ALPHA CHANGE 2931 -- validate signed chain IDs independently of cached sender metadata -- */

	var err error

	if txn.From == emptyFrom &&
		(txn.Type == types.LegacyTx || txn.Type == types.DynamicFeeTx) {
		// Decrypt the from address
		signer := crypto.NewSigner(t.config, uint64(t.ctx.ChainID))

		txn.From, err = signer.Sender(txn)
		if err != nil {
			return NewTransitionApplicationError(err, false)
		}
	}

	// Make a local copy and apply the transaction
	msg := txn.Copy()

	result, e := t.Apply(msg)
	if e != nil {
		/* START ALPHA CHANGE 2173 -- let operation boundaries report local state failures once -- */
		if !errors.Is(e, ErrLocalStateFailure) {
			t.logger.Error("failed to apply tx", "err", e)
		}
		/* FINISH ALPHA CHANGE 2173 -- let operation boundaries report local state failures once -- */

		return e
	}

	/* START ALPHA CHANGE 3153 -- retain Write cleanup and log collection through the shared boundary -- */
	// Cleanup must succeed before gas, logs or a receipt are accepted.
	logs, err := t.FinalizeTransaction()
	if err != nil {
		return err
	}

	t.totalGas += result.GasUsed
	/* FINISH ALPHA CHANGE 3153 -- retain Write cleanup and log collection through the shared boundary -- */

	receipt := &types.Receipt{
		CumulativeGasUsed: t.totalGas,
		TransactionType:   txn.Type,
		TxHash:            txn.Hash,
		GasUsed:           result.GasUsed,
	}

	if result.Failed() {
		receipt.SetStatus(types.ReceiptFailed)
	} else {
		receipt.SetStatus(types.ReceiptSuccess)
	}

	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To == nil {
		receipt.ContractAddress = crypto.CreateAddress(msg.From, txn.Nonce).Ptr()
	}

	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = logs
	receipt.LogsBloom = types.CreateBloom([]*types.Receipt{receipt})
	t.receipts = append(t.receipts, receipt)

	return nil
}

/* FINISH ALPHA CHANGE 2157 -- accept failed receipts only for ordinary EVM failures -- */

/* START ALPHA CHANGE 2150 -- retain commit failures for the lifetime of the execution transition -- */
/* START ALPHA CHANGE 2656 -- validate staged state before publication and retain rejection separately -- */
// Commit commits the final result.
func (t *Transition) Commit() (Snapshot, types.Hash, error) {
	return t.commit(nil)
}

/*
CommitWithValidation calculates the root once, validates it, and publishes
only after validation succeeds. Ordinary validation errors are retained
separately from local state failures.
*/
func (t *Transition) CommitWithValidation(
	validate func(types.Hash) error,
) (Snapshot, types.Hash, error) {
	return t.commit(validate)
}

func (t *Transition) commit(
	validate func(types.Hash) error,
) (Snapshot, types.Hash, error) {
	if err := t.state.Error(); err != nil {
		return nil, types.ZeroHash, err
	}

	if t.commitValidationErr != nil {
		return nil, types.ZeroHash, t.commitValidationErr
	}

	var validatingSnapshot ValidatingSnapshot
	if validate != nil {
		var ok bool
		validatingSnapshot, ok = t.snap.(ValidatingSnapshot)
		if !ok {
			t.state.setError(fmt.Errorf(
				"snapshot %T does not support guarded commit",
				t.snap,
			))

			return nil, types.ZeroHash, t.state.Error()
		}
	}

	objs, err := t.state.Commit(t.config.EIP155)
	if err != nil {
		t.state.setError(err)

		return nil, types.ZeroHash, t.state.Error()
	}

	var (
		snapshot      Snapshot
		root          []byte
		validationErr error
	)

	if validate == nil {
		snapshot, root, err = t.snap.Commit(objs)
	} else {
		snapshot, root, err = validatingSnapshot.CommitWithValidation(
			objs,
			func(candidateRoot types.Hash) error {
				validationErr = validate(candidateRoot)

				return validationErr
			},
		)
	}

	if validationErr != nil {
		t.commitValidationErr = validationErr

		return nil, types.ZeroHash, validationErr
	}

	if err != nil {
		t.state.setError(err)

		return nil, types.ZeroHash, t.state.Error()
	}

	return snapshot, types.BytesToHash(root), nil
}

/* FINISH ALPHA CHANGE 2656 -- validate staged state before publication and retain rejection separately -- */
/* FINISH ALPHA CHANGE 2150 -- retain commit failures for the lifetime of the execution transition -- */

func (t *Transition) subGasPool(amount uint64) error {
	if t.gasPool < amount {
		return ErrBlockLimitReached
	}

	t.gasPool -= amount

	return nil
}

func (t *Transition) addGasPool(amount uint64) {
	t.gasPool += amount
}

/* START ALPHA CHANGE 2153 -- expose the retained transition state failure -- */
func (t *Transition) Txn() *Txn {
	return t.state
}

// Error returns the first local state failure; EVM rollback does not clear it.
func (t *Transition) Error() error {
	return t.state.Error()
}

/* FINISH ALPHA CHANGE 2153 -- expose the retained transition state failure -- */

// Apply applies a new transaction
/* START ALPHA CHANGE 2155 -- abort local failures while preserving ordinary post-hook behavior -- */
func (t *Transition) Apply(msg *types.Transaction) (*runtime.ExecutionResult, error) {
	if err := t.Error(); err != nil {
		return nil, err
	}

	s := t.state.Snapshot()

	/* START ALPHA CHANGE 3150 -- reuse the existing Apply rollback snapshot throughout this transaction -- */
	previousOriginalState := t.state.originalState
	t.state.originalState = t.state.snapshots[s]
	defer func() {
		t.state.originalState = previousOriginalState
	}()
	/* FINISH ALPHA CHANGE 3150 -- reuse the existing Apply rollback snapshot throughout this transaction -- */

	result, err := t.apply(msg)
	if stateErr := t.Error(); stateErr != nil {
		// Preserve the original local failure even if rollback also fails.
		_ = t.state.RevertToSnapshot(s)
		return nil, stateErr
	}

	if err != nil {
		if revertErr := t.state.RevertToSnapshot(s); revertErr != nil {
			return nil, revertErr
		}
	}

	// Preserve the existing hook behavior for ordinary transaction errors.
	if t.PostHook != nil {
		t.PostHook(t)
	}
	if stateErr := t.Error(); stateErr != nil {
		_ = t.state.RevertToSnapshot(s)
		return nil, stateErr
	}

	return result, err
}

/* FINISH ALPHA CHANGE 2155 -- abort local failures while preserving ordinary post-hook behavior -- */

// ContextPtr returns reference of context
// This method is called only by test
func (t *Transition) ContextPtr() *runtime.TxContext {
	return &t.ctx
}

/* START ALPHA CHANGE 2155 -- give state errors precedence over derived balance errors -- */
func (t *Transition) subGasLimitPrice(msg *types.Transaction) error {
	if err := t.Error(); err != nil {
		return err
	}

	upfrontGasCost := GetLondonFixHandler(uint64(t.ctx.Number)).getUpfrontGasCost(msg, t.ctx.BaseFee)

	err := t.state.SubBalance(msg.From, upfrontGasCost)
	if stateErr := t.Error(); stateErr != nil {
		return stateErr
	}
	if err != nil {
		if errors.Is(err, runtime.ErrNotEnoughFunds) {
			return ErrNotEnoughFundsForGas
		}

		return err
	}

	return nil
}

/* FINISH ALPHA CHANGE 2155 -- give state errors precedence over derived balance errors -- */

/* START ALPHA CHANGE 2155 -- give state errors precedence over nonce mismatch -- */
func (t *Transition) nonceCheck(msg *types.Transaction) error {
	if err := t.Error(); err != nil {
		return err
	}

	nonce := t.state.GetNonce(msg.From)
	if err := t.Error(); err != nil {
		return err
	}

	if nonce != msg.Nonce {
		return ErrNonceIncorrect
	}

	return nil
}

/* FINISH ALPHA CHANGE 2155 -- give state errors precedence over nonce mismatch -- */

/* START ALPHA CHANGE 3040 -- validate maximum gas obligation plus value before gas debit -- */

// checkTxBalance validates affordability without debiting the sender.
// GetGasFeeCap selects the maximum fee cap for dynamic-fee transactions
// and the gas price for legacy transactions.
func (t *Transition) checkTxBalance(msg *types.Transaction) error {
	if err := t.Error(); err != nil {
		return err
	}

	maximumGasCost := new(big.Int)
	if fee := msg.GetGasFeeCap(); fee != nil {
		maximumGasCost.Mul(new(big.Int).SetUint64(msg.Gas), fee)
	}

	requiredBalance := new(big.Int).Set(maximumGasCost)
	if msg.Value != nil {
		requiredBalance.Add(requiredBalance, msg.Value)
	}

	balance := t.state.GetBalance(msg.From)
	if stateErr := t.Error(); stateErr != nil {
		return stateErr
	}

	if balance.Cmp(maximumGasCost) < 0 {
		return ErrNotEnoughFundsForGas
	}
	if balance.Cmp(requiredBalance) < 0 {
		return ErrNotEnoughFundsForGasAndValue
	}

	return nil
}

/* FINISH ALPHA CHANGE 3040 -- validate maximum gas obligation plus value before gas debit -- */

// checkDynamicFees checks correctness of the EIP-1559 feature-related fields.
// Basically, makes sure gas tip cap and gas fee cap are good for dynamic and legacy transactions
func (t *Transition) checkDynamicFees(msg *types.Transaction) error {
	return GetLondonFixHandler(uint64(t.ctx.Number)).checkDynamicFees(msg, t)
}

// errors that can originate in the consensus rules checks of the apply method below
// surfacing of these errors reject the transaction thus not including it in the block

var (
	ErrNonceIncorrect       = errors.New("incorrect nonce")
	ErrNotEnoughFundsForGas = errors.New("not enough funds to cover gas costs")
	/* START ALPHA CHANGE 3041 -- distinguish insufficient combined gas and value balance -- */
	ErrNotEnoughFundsForGasAndValue = errors.New("not enough funds to cover maximum gas costs and value")
	/* FINISH ALPHA CHANGE 3041 -- distinguish insufficient combined gas and value balance -- */
	ErrBlockLimitReached     = errors.New("gas limit reached in the pool")
	ErrIntrinsicGasOverflow  = errors.New("overflow in intrinsic gas calculation")
	ErrNotEnoughIntrinsicGas = errors.New("not enough gas supplied for intrinsic gas costs")

	// ErrTipAboveFeeCap is a sanity error to ensure no one is able to specify a
	// transaction with a tip higher than the total fee cap.
	ErrTipAboveFeeCap = errors.New("max priority fee per gas higher than max fee per gas")

	// ErrTipVeryHigh is a sanity error to avoid extremely big numbers specified
	// in the tip field.
	ErrTipVeryHigh = errors.New("max priority fee per gas higher than 2^256-1")

	// ErrFeeCapVeryHigh is a sanity error to avoid extremely big numbers specified
	// in the fee cap field.
	ErrFeeCapVeryHigh = errors.New("max fee per gas higher than 2^256-1")

	// ErrFeeCapTooLow is returned if the transaction fee cap is less than the
	// the base fee of the block.
	ErrFeeCapTooLow = errors.New("max fee per gas less than block base fee")

	// ErrNonceUintOverflow is returned if uint64 overflow happens
	ErrNonceUintOverflow = errors.New("nonce uint64 overflow")
)

type TransitionApplicationError struct {
	Err           error
	IsRecoverable bool // Should the transaction be discarded, or put back in the queue.
}

func (e *TransitionApplicationError) Error() string {
	return e.Err.Error()
}

func NewTransitionApplicationError(err error, isRecoverable bool) *TransitionApplicationError {
	return &TransitionApplicationError{
		Err:           err,
		IsRecoverable: isRecoverable,
	}
}

type GasLimitReachedTransitionApplicationError struct {
	TransitionApplicationError
}

func NewGasLimitReachedTransitionApplicationError(err error) *GasLimitReachedTransitionApplicationError {
	return &GasLimitReachedTransitionApplicationError{
		*NewTransitionApplicationError(err, true),
	}
}

func (t *Transition) apply(msg *types.Transaction) (*runtime.ExecutionResult, error) {
	/* START ALPHA CHANGE 2155 -- check retained errors before transaction processing -- */
	if err := t.Error(); err != nil {
		return nil, err
	}

	var err error

	if msg.Type == types.StateTx {
		err = checkAndProcessStateTx(msg)
	} else {
		err = checkAndProcessTx(msg, t)
	}

	if err := t.Error(); err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2155 -- check retained errors before transaction processing -- */

	// the amount of gas required is available in the block
	if err = t.subGasPool(msg.Gas); err != nil {
		return nil, NewGasLimitReachedTransitionApplicationError(err)
	}

	if t.ctx.Tracer != nil {
		t.ctx.Tracer.TxStart(msg.Gas)
	}

	// 4. there is no overflow when calculating intrinsic gas
	intrinsicGasCost, err := TransactionGasCost(msg, t.config.Homestead, t.config.Istanbul)
	if err != nil {
		return nil, NewTransitionApplicationError(err, false)
	}

	// the purchased gas is enough to cover intrinsic usage
	gasLeft := msg.Gas - intrinsicGasCost
	// because we are working with unsigned integers for gas, the `>` operator is used instead of the more intuitive `<`
	if gasLeft > msg.Gas {
		return nil, NewTransitionApplicationError(ErrNotEnoughIntrinsicGas, false)
	}

	gasPrice := msg.GetGasPrice(t.ctx.BaseFee.Uint64())
	value := new(big.Int).Set(msg.Value)

	// set the specific transaction fields in the context
	t.ctx.GasPrice = types.BytesToHash(gasPrice.Bytes())
	t.ctx.Origin = msg.From

	var result *runtime.ExecutionResult
	if msg.IsContractCreation() {
		result = t.Create2(msg.From, msg.Input, value, gasLeft)
	} else {
		if err := t.state.IncrNonce(msg.From); err != nil {
			return nil, err
		}
		result = t.Call2(msg.From, *msg.To, msg.Input, value, gasLeft)
	}

	/* START ALPHA CHANGE 2155 -- reject local execution failure before refund processing -- */
	if err := t.Error(); err != nil {
		return nil, err
	}

	refund := t.state.GetRefund()
	result.UpdateGasUsed(msg.Gas, refund)
	/* FINISH ALPHA CHANGE 2155 -- reject local execution failure before refund processing -- */

	if t.ctx.Tracer != nil {
		t.ctx.Tracer.TxEnd(result.GasLeft)
	}

	// Refund the sender
	/* START ALPHA CHANGE 2155 -- propagate sender refund state failure -- */
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(result.GasLeft), gasPrice)
	t.state.AddBalance(msg.From, remaining)
	if err := t.Error(); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2155 -- propagate sender refund state failure -- */

	// Spec: https://eips.ethereum.org/EIPS/eip-1559#specification
	// Define effective tip based on tx type.
	// We use EIP-1559 fields of the tx if the london hardfork is enabled.
	// Effective tip became to be either gas tip cap or (gas fee cap - current base fee)
	effectiveTip := GetLondonFixHandler(uint64(t.ctx.Number)).getEffectiveTip(
		msg, gasPrice, t.ctx.BaseFee, t.config.London,
	)

	/* START ALPHA CHANGE 811 -- split EIP-1559 base fee by governance burn rate and keep tip for validator -- */
	gasUsed := new(big.Int).SetUint64(result.GasUsed)

	tipAmount := new(big.Int).Mul(gasUsed, effectiveTip)
	validatorFee := new(big.Int).Set(tipAmount)

	if t.config.London && msg.Type != types.StateTx {
		baseFeeAmount := new(big.Int).Mul(gasUsed, t.ctx.BaseFee)
		burnAmount := new(big.Int).Mul(baseFeeAmount, new(big.Int).SetUint64(t.ctx.FeeBurnRateBps))
		burnAmount.Div(burnAmount, new(big.Int).SetUint64(feeBurnBasisPointsDenominator))

		/* START ALPHA CHANGE 967 -- split base fee between burn vault community pool and validator -- */
		communityAmount := new(big.Int).Mul(baseFeeAmount, new(big.Int).SetUint64(t.ctx.CommunityRewardRateBps))
		communityAmount.Div(communityAmount, new(big.Int).SetUint64(feeBurnBasisPointsDenominator))

		validatorBaseFee := new(big.Int).Sub(baseFeeAmount, burnAmount)
		validatorBaseFee.Sub(validatorBaseFee, communityAmount)
		validatorFee.Add(validatorFee, validatorBaseFee)
		/* FINISH ALPHA CHANGE 967 -- split base fee between burn vault community pool and validator -- */

		if burnAmount.Sign() > 0 {
			t.state.AddBalance(t.ctx.BurnContract, burnAmount)
		}

		/* START ALPHA CHANGE 968 -- credit community reward pool with protocol fee share -- */
		if communityAmount.Sign() > 0 {
			t.state.AddBalance(t.ctx.CommunityRewardContract, communityAmount)
		}
		/* FINISH ALPHA CHANGE 968 -- credit community reward pool with protocol fee share -- */
	}

	if validatorFee.Sign() > 0 {
		t.state.AddBalance(t.ctx.Coinbase, validatorFee)
	}
	/* FINISH ALPHA CHANGE 811 -- split EIP-1559 base fee by governance burn rate and keep tip for validator -- */

	/* START ALPHA CHANGE 2155 -- reject fee-recipient state failures before accepting execution -- */
	if err := t.Error(); err != nil {
		return nil, err
	}

	// return gas to the pool
	t.addGasPool(result.GasLeft)

	return result, nil
	/* FINISH ALPHA CHANGE 2155 -- reject fee-recipient state failures before accepting execution -- */
}

/* START ALPHA CHANGE 1142 -- decode governance max base fee cap from explicit storage slot -- */
/* START ALPHA CHANGE 2154 -- check governance read errors before interpreting values -- */
func readMaxBaseFeeWei(txn *Txn) (uint64, error) {
	if err := txn.Error(); err != nil {
		return 0, err
	}

	value := new(big.Int).SetBytes(txn.GetState(contracts.INNBCBlockRewardGovernanceContract, maxBaseFeeWeiSlot).Bytes())
	if err := txn.Error(); err != nil {
		return 0, err
	}

	if !value.IsUint64() {
		return 0, fmt.Errorf("INNBC max base fee exceeds uint64: %s", value.String())
	}

	return value.Uint64(), nil
}

/* FINISH ALPHA CHANGE 2154 -- check governance read errors before interpreting values -- */

/* FINISH ALPHA CHANGE 1142 -- decode governance max base fee cap from explicit storage slot -- */

/* START ALPHA CHANGE 812 -- decode governance fee burn rate from explicit runtime storage slot -- */
/* START ALPHA CHANGE 2154 -- check governance read errors before interpreting values -- */
func readFeeBurnRateBps(txn *Txn) (uint64, error) {
	if err := txn.Error(); err != nil {
		return 0, err
	}

	value := new(big.Int).SetBytes(txn.GetState(contracts.INNBCBlockRewardGovernanceContract, feeBurnRateBpsSlot).Bytes())
	if err := txn.Error(); err != nil {
		return 0, err
	}

	if value.Cmp(new(big.Int).SetUint64(feeBurnBasisPointsDenominator)) > 0 {
		return 0, fmt.Errorf("INNBC fee burn rate exceeds basis points denominator: %s", value.String())
	}

	return value.Uint64(), nil
}

/* FINISH ALPHA CHANGE 2154 -- check governance read errors before interpreting values -- */

/* START ALPHA CHANGE 969 -- decode community reward rate from explicit runtime storage slot -- */
/* START ALPHA CHANGE 2154 -- check governance read errors before interpreting values -- */
func readCommunityRewardRateBps(txn *Txn) (uint64, error) {
	if err := txn.Error(); err != nil {
		return 0, err
	}

	value := new(big.Int).SetBytes(txn.GetState(contracts.INNBCCommunityRewardPoolContract, communityRewardRateBpsSlot).Bytes())
	if err := txn.Error(); err != nil {
		return 0, err
	}

	if value.Cmp(new(big.Int).SetUint64(feeBurnBasisPointsDenominator)) > 0 {
		return 0, fmt.Errorf("INNBC community reward rate exceeds basis points denominator: %s", value.String())
	}

	return value.Uint64(), nil
}

/* FINISH ALPHA CHANGE 2154 -- check governance read errors before interpreting values -- */

/* FINISH ALPHA CHANGE 969 -- decode community reward rate from explicit runtime storage slot -- */

/* FINISH ALPHA CHANGE 812 -- decode governance fee burn rate from explicit runtime storage slot -- */

/* START ALPHA CHANGE 2156 -- check creation nonce access before constructing a contract -- */
func (t *Transition) Create2(
	caller types.Address,
	code []byte,
	value *big.Int,
	gas uint64,
) *runtime.ExecutionResult {
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	nonce := t.state.GetNonce(caller)
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	address := crypto.CreateAddress(caller, nonce)
	contract := runtime.NewContractCreation(1, caller, caller, address, value, gas, code)

	return t.applyCreate(contract, t)
}

/* FINISH ALPHA CHANGE 2156 -- check creation nonce access before constructing a contract -- */

/* START ALPHA CHANGE 2156 -- check bytecode access before direct contract execution -- */
func (t *Transition) Call2(
	caller types.Address,
	to types.Address,
	input []byte,
	value *big.Int,
	gas uint64,
) *runtime.ExecutionResult {
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	code := t.state.GetCode(to)
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	c := runtime.NewContractCall(1, caller, caller, to, value, gas, code, input)

	return t.applyCall(c, runtime.Call, t)
}

/* FINISH ALPHA CHANGE 2156 -- check bytecode access before direct contract execution -- */

func (t *Transition) run(contract *runtime.Contract, host runtime.Host) *runtime.ExecutionResult {
	/* START ALPHA CHANGE 2156 -- check access-list update state failures -- */
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	result := t.handleAllowBlockListsUpdate(contract, host)
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	if result != nil {
		return result
	}
	/* FINISH ALPHA CHANGE 2156 -- check access-list update state failures -- */

	// check txns access lists, allow list takes precedence over block list
	if t.txnAllowList != nil {
		if contract.Caller != contracts.SystemCaller {
			/* START ALPHA CHANGE 2156 -- propagate transaction allow-list read failures -- */
			role := t.txnAllowList.GetRole(contract.Caller)
			if err := t.Error(); err != nil {
				return &runtime.ExecutionResult{Err: err}
			}
			/* FINISH ALPHA CHANGE 2156 -- propagate transaction allow-list read failures -- */

			if !role.Enabled() {
				t.logger.Debug(
					"Failing transaction. Caller is not in the transaction allowlist",
					"contract.Caller", contract.Caller,
					"contract.Address", contract.Address,
				)

				return &runtime.ExecutionResult{
					GasLeft: 0,
					Err:     runtime.ErrNotAuth,
				}
			}
		}
	} else if t.txnBlockList != nil {
		if contract.Caller != contracts.SystemCaller {
			/* START ALPHA CHANGE 2156 -- propagate transaction block-list read failures -- */
			role := t.txnBlockList.GetRole(contract.Caller)
			if err := t.Error(); err != nil {
				return &runtime.ExecutionResult{Err: err}
			}
			/* FINISH ALPHA CHANGE 2156 -- propagate transaction block-list read failures -- */

			if role == addresslist.EnabledRole {
				t.logger.Debug(
					"Failing transaction. Caller is in the transaction blocklist",
					"contract.Caller", contract.Caller,
					"contract.Address", contract.Address,
				)

				return &runtime.ExecutionResult{
					GasLeft: 0,
					Err:     runtime.ErrNotAuth,
				}
			}
		}
	}

	// check the precompiles
	if t.precompiles.CanRun(contract, host, &t.config) {
		return t.precompiles.Run(contract, host, &t.config)
	}
	// check the evm
	if t.evm.CanRun(contract, host, &t.config) {
		return t.evm.Run(contract, host, &t.config)
	}

	return &runtime.ExecutionResult{
		Err: fmt.Errorf("runtime not found"),
	}
}

/* START ALPHA CHANGE 2155 -- give state errors precedence over derived balance errors -- */
func (t *Transition) Transfer(from, to types.Address, amount *big.Int) error {
	if err := t.Error(); err != nil {
		return err
	}

	if amount == nil {
		return nil
	}

	err := t.state.SubBalance(from, amount)
	if stateErr := t.Error(); stateErr != nil {
		return stateErr
	}
	if err != nil {
		if errors.Is(err, runtime.ErrNotEnoughFunds) {
			return runtime.ErrInsufficientBalance
		}

		return err
	}

	t.state.AddBalance(to, amount)

	return t.Error()
}

/* FINISH ALPHA CHANGE 2155 -- give state errors precedence over derived balance errors -- */

/* START ALPHA CHANGE 950 -- allow trusted runtime paths to mint native balance -- */
func (t *Transition) AddBalance(addr types.Address, amount *big.Int) {
	if amount == nil {
		return
	}

	t.state.AddBalance(addr, amount)
}

/* FINISH ALPHA CHANGE 950 -- allow trusted runtime paths to mint native balance -- */

/* START ALPHA CHANGE 2156 -- preserve fatal state errors across call rollback -- */
func (t *Transition) applyCall(
	c *runtime.Contract,
	callType runtime.CallType,
	host runtime.Host,
) *runtime.ExecutionResult {
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	if c.Depth > int(1024)+1 {
		return &runtime.ExecutionResult{
			GasLeft: c.Gas,
			Err:     runtime.ErrDepth,
		}
	}

	snapshot := t.state.Snapshot()
	t.state.TouchAccount(c.Address)
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	if callType == runtime.Call {
		// Transfers only allowed on calls
		if err := t.Transfer(c.Caller, c.Address, c.Value); err != nil {
			return &runtime.ExecutionResult{
				GasLeft: c.Gas,
				Err:     err,
			}
		}
	}

	var result *runtime.ExecutionResult

	t.captureCallStart(c, callType)

	result = t.run(c, host)
	if stateErr := t.Error(); stateErr != nil {
		_ = t.state.RevertToSnapshot(snapshot)
		return &runtime.ExecutionResult{Err: stateErr}
	}
	if result.Failed() {
		if err := t.state.RevertToSnapshot(snapshot); err != nil {
			return &runtime.ExecutionResult{
				GasLeft: c.Gas,
				Err:     err,
			}
		}
	}

	t.captureCallEnd(c, result)

	return result
}

/* FINISH ALPHA CHANGE 2156 -- preserve fatal state errors across call rollback -- */

func (t *Transition) hasCodeOrNonce(addr types.Address) bool {
	if t.state.GetNonce(addr) != 0 {
		return true
	}

	codeHash := t.state.GetCodeHash(addr)

	return codeHash != types.EmptyCodeHash && codeHash != types.ZeroHash
}

/* START ALPHA CHANGE 3035 -- revert failed creation changes and retain rollback failures -- */

// revertCreation restores the creation snapshot without reverting the caller's
// preceding nonce increment. Ordinary execution results retain their original
// error and gas; a local state failure takes precedence and prevents acceptance.
func (t *Transition) revertCreation(
	snapshot int,
	result *runtime.ExecutionResult,
) *runtime.ExecutionResult {
	if err := t.state.RevertToSnapshot(snapshot); err != nil {
		t.state.setError(fmt.Errorf("revert contract creation: %w", err))
	}

	if stateErr := t.Error(); stateErr != nil {
		return &runtime.ExecutionResult{Err: stateErr}
	}

	return result
}

func (t *Transition) applyCreate(c *runtime.Contract, host runtime.Host) (result *runtime.ExecutionResult) {
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	gasLimit := c.Gas

	if c.Depth > int(1024)+1 {
		return &runtime.ExecutionResult{
			GasLeft: gasLimit,
			Err:     runtime.ErrDepth,
		}
	}

	// Increment the nonce before the creation snapshot.
	nonceErr := t.state.IncrNonce(c.Caller)
	if stateErr := t.Error(); stateErr != nil {
		return &runtime.ExecutionResult{Err: stateErr}
	}
	if nonceErr != nil {
		return &runtime.ExecutionResult{Err: nonceErr}
	}

	collision := t.hasCodeOrNonce(c.Address)
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	if collision {
		return &runtime.ExecutionResult{
			GasLeft: 0,
			Err:     runtime.ErrContractAddressCollision,
		}
	}

	// Ordinary creation failure preserves the preceding caller nonce increment.
	snapshot := t.state.Snapshot()

	if t.config.EIP158 {
		t.state.CreateAccount(c.Address)
		if stateErr := t.Error(); stateErr != nil {
			return t.revertCreation(snapshot, &runtime.ExecutionResult{
				Err: stateErr,
			})
		}

		if err := t.state.IncrNonce(c.Address); err != nil {
			return t.revertCreation(snapshot, &runtime.ExecutionResult{
				Err: err,
			})
		}
		if stateErr := t.Error(); stateErr != nil {
			return t.revertCreation(snapshot, &runtime.ExecutionResult{
				Err: stateErr,
			})
		}
	}

	if err := t.Transfer(c.Caller, c.Address, c.Value); err != nil {
		return t.revertCreation(snapshot, &runtime.ExecutionResult{
			GasLeft: gasLimit,
			Err:     err,
		})
	}

	t.captureCallStart(c, evm.CREATE)

	defer func() {
		t.captureCallEnd(c, result)
	}()

	if t.deploymentAllowList != nil {
		role := t.deploymentAllowList.GetRole(c.Caller)
		if stateErr := t.Error(); stateErr != nil {
			return t.revertCreation(snapshot, &runtime.ExecutionResult{
				Err: stateErr,
			})
		}

		if !role.Enabled() {
			t.logger.Debug(
				"Failing contract deployment. Caller is not in the deployment allowlist",
				"contract.Caller", c.Caller,
				"contract.Address", c.Address,
			)

			return t.revertCreation(snapshot, &runtime.ExecutionResult{
				GasLeft: 0,
				Err:     runtime.ErrNotAuth,
			})
		}
	} else if t.deploymentBlockList != nil {
		role := t.deploymentBlockList.GetRole(c.Caller)
		if stateErr := t.Error(); stateErr != nil {
			return t.revertCreation(snapshot, &runtime.ExecutionResult{
				Err: stateErr,
			})
		}

		if role == addresslist.EnabledRole {
			t.logger.Debug(
				"Failing contract deployment. Caller is in the deployment blocklist",
				"contract.Caller", c.Caller,
				"contract.Address", c.Address,
			)

			return t.revertCreation(snapshot, &runtime.ExecutionResult{
				GasLeft: 0,
				Err:     runtime.ErrNotAuth,
			})
		}
	}

	result = t.run(c, host)
	if stateErr := t.Error(); stateErr != nil {
		return t.revertCreation(snapshot, &runtime.ExecutionResult{
			Err: stateErr,
		})
	}
	if result.Failed() {
		return t.revertCreation(snapshot, result)
	}

	if t.config.EIP158 && len(result.ReturnValue) > SpuriousDragonMaxCodeSize {
		return t.revertCreation(snapshot, &runtime.ExecutionResult{
			GasLeft: 0,
			Err:     runtime.ErrMaxCodeSizeExceeded,
		})
	}

	gasCost := uint64(len(result.ReturnValue)) * 200

	if result.GasLeft < gasCost {
		result.Err = runtime.ErrCodeStoreOutOfGas
		result.ReturnValue = nil

		// Preserve the pre-Homestead code-deposit exception.
		if t.config.Homestead {
			result.GasLeft = 0
			return t.revertCreation(snapshot, result)
		}

		return result
	}

	result.GasLeft -= gasCost
	result.Address = c.Address
	t.state.SetCode(c.Address, result.ReturnValue)
	if stateErr := t.Error(); stateErr != nil {
		return t.revertCreation(snapshot, &runtime.ExecutionResult{
			Err: stateErr,
		})
	}

	return result
}

/* FINISH ALPHA CHANGE 3035 -- revert failed creation changes and retain rollback failures -- */

func (t *Transition) handleAllowBlockListsUpdate(contract *runtime.Contract,
	host runtime.Host) *runtime.ExecutionResult {
	// check contract deployment allow list (if any)
	if t.deploymentAllowList != nil && t.deploymentAllowList.Addr() == contract.CodeAddress {
		return t.deploymentAllowList.Run(contract, host, &t.config)
	}

	// check contract deployment block list (if any)
	if t.deploymentBlockList != nil && t.deploymentBlockList.Addr() == contract.CodeAddress {
		return t.deploymentBlockList.Run(contract, host, &t.config)
	}

	/* START ALPHA CHANGE 441 -- remove bridge allow/block list execution hooks -- */
	/* FINISH ALPHA CHANGE 441 -- remove bridge allow/block list execution hooks -- */

	// check transaction allow list (if any)
	if t.txnAllowList != nil && t.txnAllowList.Addr() == contract.CodeAddress {
		return t.txnAllowList.Run(contract, host, &t.config)
	}

	// check transaction block list (if any)
	if t.txnBlockList != nil && t.txnBlockList.Addr() == contract.CodeAddress {
		return t.txnBlockList.Run(contract, host, &t.config)
	}

	return nil
}

func (t *Transition) SetState(addr types.Address, key types.Hash, value types.Hash) {
	t.state.SetState(addr, key, value)
}

func (t *Transition) SetStorage(
	addr types.Address,
	key types.Hash,
	value types.Hash,
	config *chain.ForksInTime,
) runtime.StorageStatus {
	return t.state.SetStorage(addr, key, value, config)
}

func (t *Transition) GetTxContext() runtime.TxContext {
	return t.ctx
}

/* START ALPHA CHANGE 3808 -- retain BLOCKHASH read failures across execution rollback -- */

func (t *Transition) GetBlockHash(number int64) (res types.Hash) {
	if t.Error() != nil {
		return
	}

	hash, err := t.getHash(uint64(number))
	if err != nil {
		t.state.setError(fmt.Errorf("read BLOCKHASH %d: %w", number, err))

		return
	}

	return hash
}

/* FINISH ALPHA CHANGE 3808 -- retain BLOCKHASH read failures across execution rollback -- */

func (t *Transition) EmitLog(addr types.Address, topics []types.Hash, data []byte) {
	t.state.EmitLog(addr, topics, data)
}

func (t *Transition) GetCodeSize(addr types.Address) int {
	return t.state.GetCodeSize(addr)
}

func (t *Transition) GetCodeHash(addr types.Address) (res types.Hash) {
	return t.state.GetCodeHash(addr)
}

func (t *Transition) GetCode(addr types.Address) []byte {
	return t.state.GetCode(addr)
}

func (t *Transition) GetBalance(addr types.Address) *big.Int {
	return t.state.GetBalance(addr)
}

func (t *Transition) GetStorage(addr types.Address, key types.Hash) types.Hash {
	return t.state.GetState(addr, key)
}

func (t *Transition) AccountExists(addr types.Address) bool {
	return t.state.Exist(addr)
}

func (t *Transition) Empty(addr types.Address) bool {
	return t.state.Empty(addr)
}

func (t *Transition) GetNonce(addr types.Address) uint64 {
	return t.state.GetNonce(addr)
}

func (t *Transition) Selfdestruct(addr types.Address, beneficiary types.Address) {
	if !t.state.HasSuicided(addr) {
		t.state.AddRefund(24000)
	}

	t.state.AddBalance(beneficiary, t.state.GetBalance(addr))
	t.state.Suicide(addr)
}

/* START ALPHA CHANGE 2156 -- stop nested calls after a retained state failure -- */
func (t *Transition) Callx(c *runtime.Contract, h runtime.Host) *runtime.ExecutionResult {
	if err := t.Error(); err != nil {
		return &runtime.ExecutionResult{Err: err}
	}

	if c.Type == runtime.Create {
		return t.applyCreate(c, h)
	}

	return t.applyCall(c, c.Type, h)
}

/* FINISH ALPHA CHANGE 2156 -- stop nested calls after a retained state failure -- */

// SetAccountDirectly sets an account to the given address
// NOTE: SetAccountDirectly changes the world state without a transaction
/* START ALPHA CHANGE 2155 -- check direct account helpers before using placeholder existence -- */
func (t *Transition) SetAccountDirectly(addr types.Address, account *chain.GenesisAccount) error {
	if err := t.Error(); err != nil {
		return err
	}

	exists := t.AccountExists(addr)
	if err := t.Error(); err != nil {
		return err
	}

	if exists {
		return fmt.Errorf("can't add account to %+v because an account exists already", addr)
	}

	t.state.SetCode(addr, account.Code)

	for key, value := range account.Storage {
		t.state.SetStorage(addr, key, value, &t.config)
	}

	t.state.SetBalance(addr, account.Balance)
	t.state.SetNonce(addr, account.Nonce)

	return t.Error()
}

/* FINISH ALPHA CHANGE 2155 -- check direct account helpers before using placeholder existence -- */

// SetCodeDirectly sets new code into the account with the specified address
// NOTE: SetCodeDirectly changes the world state without a transaction
/* START ALPHA CHANGE 2155 -- check direct account helpers before using placeholder existence -- */
func (t *Transition) SetCodeDirectly(addr types.Address, code []byte) error {
	if err := t.Error(); err != nil {
		return err
	}

	exists := t.AccountExists(addr)
	if err := t.Error(); err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("account doesn't exist at %s", addr)
	}

	t.state.SetCode(addr, code)

	return t.Error()
}

/* FINISH ALPHA CHANGE 2155 -- check direct account helpers before using placeholder existence -- */

// SetNonPayable deactivates the check of tx cost against tx executor balance.
func (t *Transition) SetNonPayable(nonPayable bool) {
	t.ctx.NonPayable = nonPayable
}

// SetTracer sets tracer to the context in order to enable it
func (t *Transition) SetTracer(tracer tracer.Tracer) {
	t.ctx.Tracer = tracer
}

// GetTracer returns a tracer in context
func (t *Transition) GetTracer() runtime.VMTracer {
	return t.ctx.Tracer
}

func (t *Transition) GetRefund() uint64 {
	return t.state.GetRefund()
}

func TransactionGasCost(msg *types.Transaction, isHomestead, isIstanbul bool) (uint64, error) {
	cost := uint64(0)

	// Contract creation is only paid on the homestead fork
	if msg.IsContractCreation() && isHomestead {
		cost += TxGasContractCreation
	} else {
		cost += TxGas
	}

	payload := msg.Input
	if len(payload) > 0 {
		zeros := uint64(0)

		for i := 0; i < len(payload); i++ {
			if payload[i] == 0 {
				zeros++
			}
		}

		nonZeros := uint64(len(payload)) - zeros
		nonZeroCost := uint64(68)

		if isIstanbul {
			nonZeroCost = 16
		}

		if (math.MaxUint64-cost)/nonZeroCost < nonZeros {
			return 0, ErrIntrinsicGasOverflow
		}

		cost += nonZeros * nonZeroCost

		if (math.MaxUint64-cost)/4 < zeros {
			return 0, ErrIntrinsicGasOverflow
		}

		cost += zeros * 4
	}

	return cost, nil
}

/* START ALPHA CHANGE 3042 -- check full affordability before preserving the existing gas debit -- */
// checkAndProcessTx validates the nonce and, for payable execution, the fee
// fields and balance required for maximum gas costs plus transferred value.
// Actual upfront gas charging remains delegated to the existing fork handler.
/* FINISH ALPHA CHANGE 3042 -- check full affordability before preserving the existing gas debit -- */
/* START ALPHA CHANGE 2155 -- do not classify local state failures as transaction errors -- */
func checkAndProcessTx(msg *types.Transaction, t *Transition) error {
	if err := t.Error(); err != nil {
		return err
	}

	/* START ALPHA CHANGE 3894 -- reject exhausted transaction nonces before gas reservation -- */
	if msg.Nonce == math.MaxUint64 {
		return NewTransitionApplicationError(ErrNonceUintOverflow, false)
	}
	/* FINISH ALPHA CHANGE 3894 -- reject exhausted transaction nonces before gas reservation -- */

	// 1. the nonce of the message caller is correct
	if err := t.nonceCheck(msg); err != nil {
		if stateErr := t.Error(); stateErr != nil {
			return stateErr
		}
		return NewTransitionApplicationError(err, true)
	}

	if !t.ctx.NonPayable {
		// 2. check dynamic fees of the transaction
		if err := t.checkDynamicFees(msg); err != nil {
			if stateErr := t.Error(); stateErr != nil {
				return stateErr
			}
			return NewTransitionApplicationError(err, true)
		}

		/* START ALPHA CHANGE 3042 -- reject insufficient gas-plus-value balance before debiting gas -- */
		if err := t.checkTxBalance(msg); err != nil {
			if stateErr := t.Error(); stateErr != nil {
				return stateErr
			}
			return NewTransitionApplicationError(err, true)
		}

		// Affordability uses the maximum obligation. The existing fork handler
		// still determines the actual upfront gas debit.
		if err := t.subGasLimitPrice(msg); err != nil {
			if stateErr := t.Error(); stateErr != nil {
				return stateErr
			}
			return NewTransitionApplicationError(err, true)
		}
		/* FINISH ALPHA CHANGE 3042 -- reject insufficient gas-plus-value balance before debiting gas -- */
	}

	return t.Error()
}

/* FINISH ALPHA CHANGE 2155 -- do not classify local state failures as transaction errors -- */

func checkAndProcessStateTx(msg *types.Transaction) error {
	if msg.GasPrice.Cmp(big.NewInt(0)) != 0 {
		return NewTransitionApplicationError(
			errors.New("gasPrice of state transaction must be zero"),
			true,
		)
	}

	if msg.Gas != types.StateTransactionGasLimit {
		return NewTransitionApplicationError(
			fmt.Errorf("gas of state transaction must be %d", types.StateTransactionGasLimit),
			true,
		)
	}

	if msg.From != contracts.SystemCaller {
		return NewTransitionApplicationError(
			fmt.Errorf("state transaction sender must be %v, but got %v", contracts.SystemCaller, msg.From),
			true,
		)
	}

	if msg.To == nil || *msg.To == types.ZeroAddress {
		return NewTransitionApplicationError(
			errors.New("to of state transaction must be specified"),
			true,
		)
	}

	return nil
}

// captureCallStart calls CallStart in Tracer if context has the tracer
func (t *Transition) captureCallStart(c *runtime.Contract, callType runtime.CallType) {
	if t.ctx.Tracer == nil {
		return
	}

	t.ctx.Tracer.CallStart(
		c.Depth,
		c.Caller,
		c.Address,
		int(callType),
		c.Gas,
		c.Value,
		c.Input,
	)
}

// captureCallEnd calls CallEnd in Tracer if context has the tracer
func (t *Transition) captureCallEnd(c *runtime.Contract, result *runtime.ExecutionResult) {
	if t.ctx.Tracer == nil {
		return
	}

	t.ctx.Tracer.CallEnd(
		c.Depth,
		result.ReturnValue,
		result.Err,
	)
}
