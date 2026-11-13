package jsonrpc

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/gasprice"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/helper/progress"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"

	/* START ALPHA CHANGE 3010 -- use the shared raw transaction admission decoder -- */
	"github.com/innovativebioresearch/innbc-chain/txpool"
	/* FINISH ALPHA CHANGE 3010 -- use the shared raw transaction admission decoder -- */
	"github.com/innovativebioresearch/innbc-chain/types"
)

type ethTxPoolStore interface {
	// AddTx adds a new transaction to the tx pool
	AddTx(tx *types.Transaction) error

	// GetPendingTx gets the pending transaction from the transaction pool, if it's present
	GetPendingTx(txHash types.Hash) (*types.Transaction, bool)

	/* START ALPHA CHANGE 2163 -- allow pending nonce queries to fail explicitly -- */
	// GetNonce returns the next nonce for this address.
	GetNonce(addr types.Address) (uint64, error)
	/* FINISH ALPHA CHANGE 2163 -- allow pending nonce queries to fail explicitly -- */

	// GetBaseFee returns the current base fee of TxPool
	GetBaseFee() uint64
}

type Account struct {
	Balance *big.Int
	Nonce   uint64
}

type ethStateStore interface {
	GetAccount(root types.Hash, addr types.Address) (*Account, error)
	GetStorage(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error)
	GetForksInTime(blockNumber uint64) chain.ForksInTime
	GetCode(root types.Hash, addr types.Address) ([]byte, error)
}

type ethBlockchainStore interface {
	// Header returns the current header of the chain (genesis if empty)
	Header() *types.Header

	// GetHeaderByNumber gets a header using the provided number
	GetHeaderByNumber(uint64) (*types.Header, bool)

	// GetBlockByHash gets a block using the provided hash
	GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool)

	// GetBlockByNumber returns a block using the provided number
	GetBlockByNumber(num uint64, full bool) (*types.Block, bool)

	// ReadTxLookup returns a block hash in which a given txn was mined
	ReadTxLookup(txnHash types.Hash) (types.Hash, bool)

	// GetReceiptsByHash returns the receipts for a block hash
	GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error)

	// GetAvgGasPrice returns the average gas price
	GetAvgGasPrice() *big.Int

	// ApplyTxn applies a transaction object to the blockchain
	ApplyTxn(
		header *types.Header,
		txn *types.Transaction,
		override types.StateOverride,
		nonPayable bool,
	) (*runtime.ExecutionResult, error)

	// GetSyncProgression retrieves the current sync progression, if any
	GetSyncProgression() *progress.Progression
}

type ethFilter interface {
	// FilterExtra filters extra data from header extra that is not included in block hash
	FilterExtra(extra []byte) ([]byte, error)
}

// ethStore provides access to the methods needed by eth endpoint
type ethStore interface {
	ethTxPoolStore
	ethStateStore
	ethBlockchainStore
	ethFilter
	gasprice.GasStore
}

// Eth is the eth jsonrpc endpoint
/* START ALPHA CHANGE 2459 -- retain the endpoint percentile budget -- */
type Eth struct {
	logger                    hclog.Logger
	store                     ethStore
	chainID                   uint64
	filterManager             *FilterManager
	priceLimit                uint64
	feeHistoryPercentileLimit uint64
}

/* FINISH ALPHA CHANGE 2459 -- retain the endpoint percentile budget -- */

var (
	ErrInsufficientFunds = errors.New("insufficient funds for execution")
)

// ChainId returns the chain id of the client
//
//nolint:stylecheck
func (e *Eth) ChainId() (interface{}, error) {
	return argUintPtr(e.chainID), nil
}

func (e *Eth) Syncing() (interface{}, error) {
	if syncProgression := e.store.GetSyncProgression(); syncProgression != nil {
		// Node is bulk syncing, return the status
		return progression{
			Type:          string(syncProgression.SyncType),
			StartingBlock: argUint64(syncProgression.StartingBlock),
			CurrentBlock:  argUint64(syncProgression.CurrentBlock),
			HighestBlock:  argUint64(syncProgression.HighestBlock),
		}, nil
	}

	// Node is not bulk syncing
	return false, nil
}

// GetBlockByNumber returns information about a block by block number
func (e *Eth) GetBlockByNumber(number BlockNumber, fullTx bool) (interface{}, error) {
	num, err := GetNumericBlockNumber(number, e.store)
	if err != nil {
		return nil, err
	}

	block, ok := e.store.GetBlockByNumber(num, true)
	if !ok {
		/* START ALPHA CHANGE 844 -- return header-only block data when recent mode pruned heavy history -- */
		if fullTx {
			return nil, nil
		}

		block, ok = e.store.GetBlockByNumber(num, false)
		if !ok {
			return nil, nil
		}
		/* FINISH ALPHA CHANGE 844 -- return header-only block data when recent mode pruned heavy history -- */
	}

	if err := e.filterExtra(block); err != nil {
		return nil, err
	}

	return toBlock(block, fullTx), nil
}

// GetBlockByHash returns information about a block by hash
func (e *Eth) GetBlockByHash(hash types.Hash, fullTx bool) (interface{}, error) {
	block, ok := e.store.GetBlockByHash(hash, true)
	if !ok {
		/* START ALPHA CHANGE 845 -- return header-only block data by hash when recent mode pruned heavy history -- */
		if fullTx {
			return nil, nil
		}

		block, ok = e.store.GetBlockByHash(hash, false)
		if !ok {
			return nil, nil
		}
		/* FINISH ALPHA CHANGE 845 -- return header-only block data by hash when recent mode pruned heavy history -- */
	}

	if err := e.filterExtra(block); err != nil {
		return nil, err
	}

	return toBlock(block, fullTx), nil
}

func (e *Eth) filterExtra(block *types.Block) error {
	// we need to copy it because the store returns header from storage directly
	// and not a copy, so changing it, actually changes it in storage as well
	headerCopy := block.Header.Copy()

	filteredExtra, err := e.store.FilterExtra(headerCopy.ExtraData)
	if err != nil {
		return err
	}

	headerCopy.ExtraData = filteredExtra
	// no need to recompute hash (filtered out data is not in the hash in the first place)
	block.Header = headerCopy

	return nil
}

func (e *Eth) GetBlockTransactionCountByNumber(number BlockNumber) (interface{}, error) {
	num, err := GetNumericBlockNumber(number, e.store)
	if err != nil {
		return nil, err
	}

	block, ok := e.store.GetBlockByNumber(num, true)

	if !ok {
		/* START ALPHA CHANGE 910 -- report pruned transaction count requests explicitly -- */
		if _, headerOnly := e.store.GetBlockByNumber(num, false); headerOnly {
			return nil, ErrPrunedHistory
		}
		/* FINISH ALPHA CHANGE 910 -- report pruned transaction count requests explicitly -- */

		return nil, nil
	}

	return *common.EncodeUint64(uint64(len(block.Transactions))), nil
}

// BlockNumber returns current block number
func (e *Eth) BlockNumber() (interface{}, error) {
	h := e.store.Header()
	if h == nil {
		return nil, fmt.Errorf("header has a nil value")
	}

	return argUintPtr(h.Number), nil
}

// SendRawTransaction sends a raw transaction
func (e *Eth) SendRawTransaction(buf argBytes) (interface{}, error) {
	/* START ALPHA CHANGE 3011 -- enforce raw RPC size before transaction decoding -- */
	tx, err := txpool.DecodeRawTransaction(buf)
	if err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3011 -- enforce raw RPC size before transaction decoding -- */

	// tx hash will be calculated inside e.store.AddTx
	if err := e.store.AddTx(tx); err != nil {
		return nil, err
	}

	return tx.Hash.String(), nil
}

// SendTransaction rejects eth_sendTransaction json-rpc call as we don't support wallet management
func (e *Eth) SendTransaction(_ *txnArgs) (interface{}, error) {
	return nil, fmt.Errorf("request calls to eth_sendTransaction method are not supported," +
		" use eth_sendRawTransaction instead")
}

// GetTransactionByHash returns a transaction by its hash.
// If the transaction is still pending -> return the txn with some fields omitted
// If the transaction is sealed into a block -> return the whole txn with all fields
func (e *Eth) GetTransactionByHash(hash types.Hash) (interface{}, error) {
	// findSealedTx is a helper method for checking the world state
	// for the transaction with the provided hash
	findSealedTx := func() *transaction {
		// Check the chain state for the transaction
		blockHash, ok := e.store.ReadTxLookup(hash)
		if !ok {
			// Block not found in storage
			return nil
		}

		block, ok := e.store.GetBlockByHash(blockHash, true)
		if !ok {
			// Block receipts not found in storage
			return nil
		}

		// Find the transaction within the block
		if txn, idx := types.FindTxByHash(block.Transactions, hash); txn != nil {
			txn.GasPrice = txn.GetGasPrice(block.Header.BaseFee)

			return toTransaction(
				txn,
				argUintPtr(block.Number()),
				argHashPtr(block.Hash()),
				&idx,
			)
		}

		return nil
	}

	// findPendingTx is a helper method for checking the TxPool
	// for the pending transaction with the provided hash
	findPendingTx := func() *transaction {
		// Check the TxPool for the transaction if it's pending
		if pendingTx, pendingFound := e.store.GetPendingTx(hash); pendingFound {
			return toPendingTransaction(pendingTx)
		}

		// Transaction not found in the TxPool
		return nil
	}

	// 1. Check the chain state for the txn
	if resultTxn := findSealedTx(); resultTxn != nil {
		return resultTxn, nil
	}

	// 2. Check the TxPool for the txn
	if resultTxn := findPendingTx(); resultTxn != nil {
		return resultTxn, nil
	}

	// Transaction not found in state or TxPool
	e.logger.Warn(
		fmt.Sprintf("Transaction with hash [%s] not found", hash),
	)

	return nil, nil
}

// GetTransactionReceipt returns a transaction receipt by his hash
func (e *Eth) GetTransactionReceipt(hash types.Hash) (interface{}, error) {
	blockHash, ok := e.store.ReadTxLookup(hash)
	if !ok {
		// txn not found
		return nil, nil
	}

	block, ok := e.store.GetBlockByHash(blockHash, true)
	if !ok {
		/* START ALPHA CHANGE 911 -- report pruned transaction receipt block body explicitly -- */
		if _, headerOnly := e.store.GetBlockByHash(blockHash, false); headerOnly {
			return nil, ErrPrunedHistory
		}
		/* FINISH ALPHA CHANGE 911 -- report pruned transaction receipt block body explicitly -- */

		// block not found
		e.logger.Warn(
			fmt.Sprintf("Block with hash [%s] not found", blockHash.String()),
		)

		return nil, nil
	}

	/* START ALPHA CHANGE 3569 -- preserve receipt-read failures and reject incomplete lists before indexing -- */
	txn, txIndex := types.FindTxByHash(block.Transactions, hash)
	if txIndex == -1 {
		// Transaction not found in the loaded block.
		return nil, nil
	}

	receipts, err := e.store.GetReceiptsByHash(blockHash)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read receipts for block %s: %w",
			blockHash.String(), err,
		)
	}

	if len(receipts) != len(block.Transactions) {
		return nil, fmt.Errorf(
			"receipt count mismatch for block %s: expected %d, got %d",
			blockHash.String(),
			len(block.Transactions),
			len(receipts),
		)
	}

	logIndex := 0
	/* FINISH ALPHA CHANGE 3569 -- preserve receipt-read failures and reject incomplete lists before indexing -- */

	/* START ALPHA CHANGE 3571 -- use integer range for preceding receipt log indices -- */
	for i := range txIndex {
		// accumulate receipt logs indexes from block transactions
		// that are before the desired transaction
		logIndex += len(receipts[i].Logs)
	}
	/* FINISH ALPHA CHANGE 3571 -- use integer range for preceding receipt log indices -- */

	raw := receipts[txIndex]
	logs := toLogs(raw.Logs, uint64(logIndex), uint64(txIndex), block.Header, hash)

	return toReceipt(raw, txn, uint64(txIndex), block.Header, logs), nil
}

// GetStorageAt returns the contract storage at the index position
func (e *Eth) GetStorageAt(
	address types.Address,
	index types.Hash,
	filter BlockNumberOrHash,
) (interface{}, error) {
	header, err := GetHeaderFromBlockNumberOrHash(filter, e.store)
	if err != nil {
		return nil, err
	}

	// Get the storage for the passed in location
	/* START ALPHA CHANGE 2163 -- preserve local storage failures before absence handling -- */
	result, err := e.store.GetStorage(header.StateRoot, address, index)
	if err != nil {
		if isAbsentState(err) {
			return argBytesPtr(types.ZeroHash[:]), nil
		}

		return nil, err
	}
	/* FINISH ALPHA CHANGE 2163 -- preserve local storage failures before absence handling -- */

	return argBytesPtr(result), nil
}

// GasPrice exposes "getGasPrice"'s function logic to public RPC interface
func (e *Eth) GasPrice() (interface{}, error) {
	gasPrice, err := e.getGasPrice()
	if err != nil {
		return nil, err
	}

	return argUint64(gasPrice), nil
}

// getGasPrice returns the average gas price based on the last x blocks
// taking into consideration operator defined price limit
func (e *Eth) getGasPrice() (uint64, error) {
	// START ALPHA CHANGE 1007 -- use floored London base fee for RPC gas price recommendation --
	// Return --price-limit flag defined value if it is greater than avgGasPrice/baseFee+priorityFee
	if e.store.GetForksInTime(e.store.Header().Number).London {
		priorityFee, err := e.store.MaxPriorityFeePerGas()
		if err != nil {
			return 0, err
		}

		baseFee := common.Max(e.store.GetBaseFee(), 1)

		/* START ALPHA CHANGE 1053 -- saturate London gasPrice RPC addition to avoid uint64 wraparound -- */
		if priorityFee.BitLen() > 64 || math.MaxUint64-baseFee < priorityFee.Uint64() {
			return math.MaxUint64, nil
		}

		return common.Max(e.priceLimit, priorityFee.Uint64()+baseFee), nil
		/* FINISH ALPHA CHANGE 1053 -- saturate London gasPrice RPC addition to avoid uint64 wraparound -- */
	}
	// FINISH ALPHA CHANGE 1007 -- use floored London base fee for RPC gas price recommendation --

	// Fetch average gas price in uint64
	avgGasPrice := e.store.GetAvgGasPrice().Uint64()

	return common.Max(e.priceLimit, avgGasPrice), nil
}

// fillTransactionGasPrice fills transaction gas price if no provided
func (e *Eth) fillTransactionGasPrice(tx *types.Transaction) error {
	if tx.GetGasPrice(e.store.GetBaseFee()).BitLen() > 0 {
		return nil
	}

	estimatedGasPrice, err := e.getGasPrice()
	if err != nil {
		return err
	}

	if tx.Type == types.DynamicFeeTx {
		tx.GasFeeCap = new(big.Int).SetUint64(estimatedGasPrice)
	} else {
		tx.GasPrice = new(big.Int).SetUint64(estimatedGasPrice)
	}

	return nil
}

/* START ALPHA CHANGE 3095 -- preserve full-width validated balances during RPC override conversion -- */
type overrideAccount struct {
	Nonce     *argUint64                 `json:"nonce"`
	Code      *argBytes                  `json:"code"`
	Balance   *argUint256                `json:"balance"`
	State     *map[types.Hash]types.Hash `json:"state"`
	StateDiff *map[types.Hash]types.Hash `json:"stateDiff"`
}

func (o *overrideAccount) ToType() types.OverrideAccount {
	res := types.OverrideAccount{}

	if o.Nonce != nil {
		res.Nonce = (*uint64)(o.Nonce)
	}

	if o.Code != nil {
		res.Code = *o.Code
	}

	if o.Balance != nil {
		res.Balance = new(big.Int).Set((*big.Int)(o.Balance))
	}

	if o.State != nil {
		res.State = *o.State
	}

	if o.StateDiff != nil {
		res.StateDiff = *o.StateDiff
	}

	return res
}

/* FINISH ALPHA CHANGE 3095 -- preserve full-width validated balances during RPC override conversion -- */

// StateOverride is the collection of overridden accounts.
type stateOverride map[types.Address]overrideAccount

// Call executes a smart contract call using the transaction object data
func (e *Eth) Call(arg *txnArgs, filter BlockNumberOrHash, apiOverride *stateOverride) (interface{}, error) {
	/* START ALPHA CHANGE 2995 -- resolve call nonces from the captured state and effective sender override -- */
	if arg == nil {
		return nil, errors.New("missing value for required argument 0")
	}

	header, err := GetHeaderFromBlockNumberOrHash(filter, e.store)
	if err != nil {
		return nil, err
	}

	transaction, err := decodeSimulationTxn(arg, header, e.store, apiOverride)
	if err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2995 -- resolve call nonces from the captured state and effective sender override -- */

	// If the caller didn't supply the gas limit in the message, then we set it to maximum possible => block gas limit
	if transaction.Gas == 0 {
		transaction.Gas = header.GasLimit
	}

	// Force transaction gas price if empty
	if err = e.fillTransactionGasPrice(transaction); err != nil {
		return nil, err
	}

	var override types.StateOverride
	if apiOverride != nil {
		override = types.StateOverride{}
		for addr, o := range *apiOverride {
			override[addr] = o.ToType()
		}
	}

	// The return value of the execution is saved in the transition (returnValue field)
	/* START ALPHA CHANGE 2163 -- prioritize local failures over call return data and EVM errors -- */
	result, err := e.store.ApplyTxn(header, transaction, override, true)
	if errors.Is(err, state.ErrLocalStateFailure) {
		return nil, err
	}
	if result != nil && errors.Is(result.Err, state.ErrLocalStateFailure) {
		return nil, result.Err
	}
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 2995 -- reject a missing call simulation result -- */
	if result == nil {
		return nil, errors.New("call simulation returned no execution result")
	}
	/* FINISH ALPHA CHANGE 2995 -- reject a missing call simulation result -- */

	// Check if an EVM revert happened
	/* FINISH ALPHA CHANGE 2163 -- prioritize local failures over call return data and EVM errors -- */
	if result.Reverted() {
		return []byte(hex.EncodeToString(result.ReturnValue)), constructErrorFromRevert(result)
	}

	if result.Failed() {
		return nil, fmt.Errorf("unable to execute call: %w", result.Err)
	}

	return argBytesPtr(result.ReturnValue), nil
}

// EstimateGas estimates the gas needed to execute a transaction
func (e *Eth) EstimateGas(arg *txnArgs, rawNum *BlockNumber) (interface{}, error) {
	/* START ALPHA CHANGE 2986 -- establish estimation ceilings before execution -- */
	if arg == nil {
		return nil, errors.New("missing value for required argument 0")
	}

	// DecodeTxn supplies defaults. Preserve which fields the caller supplied
	// and decode a copy so repeated calls do not change that distinction.
	gasProvided := arg.Gas != nil
	fromProvided := arg.From != nil
	gasPriceProvided := arg.GasPrice != nil
	feeCapProvided := arg.GasFeeCap != nil
	request := *arg

	number := LatestBlockNumber
	if rawNum != nil {
		number = *rawNum
	}

	header, err := GetBlockHeader(number, e.store)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, errors.New("missing header for gas estimation")
	}

	/* START ALPHA CHANGE 2991 -- bind estimation nonce and balance to the selected state -- */
	var senderAccount *Account
	if fromProvided {
		var accountErr error
		senderAccount, accountErr = e.store.GetAccount(
			header.StateRoot,
			*request.From,
		)
		if accountErr != nil {
			if !isAbsentState(accountErr) {
				return nil, accountErr
			}

			// A genuinely absent account has nonce zero and balance zero.
			senderAccount = nil
		} else if senderAccount == nil ||
			senderAccount.Balance == nil ||
			senderAccount.Balance.Sign() < 0 {
			return nil, state.WrapLocalStateFailure(
				"read gas-estimation sender account",
				state.ErrMalformedAccount,
			)
		}

		// Estimation uses the nonce expected by the selected state,
		// overriding a caller-supplied nonce only on the request copy.
		request.Nonce = argUintPtr(0)
		if senderAccount != nil {
			request.Nonce = argUintPtr(senderAccount.Nonce)
		}
	}

	// A supplied sender already has its selected-state nonce. With no
	// sender, DecodeTxn retains its default zero simulation caller.
	// Neither case needs a fresh latest-state nonce lookup.
	transaction, err := DecodeTxn(&request, header.Number, e.store, false)
	if err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2991 -- bind estimation nonce and balance to the selected state -- */

	forksInTime := e.store.GetForksInTime(header.Number)
	intrinsicGas, err := state.TransactionGasCost(
		transaction,
		forksInTime.Homestead,
		forksInTime.Istanbul,
	)
	if err != nil {
		return nil, err
	}

	lowEnd := intrinsicGas
	highEnd := header.GasLimit

	if gasProvided {
		if transaction.Gas == 0 {
			return nil, fmt.Errorf(
				"explicit gas ceiling is zero: %w",
				state.ErrNotEnoughIntrinsicGas,
			)
		}
		if transaction.Gas < highEnd {
			highEnd = transaction.Gas
		}
	}

	// Default only an omitted fee field. A supplied zero remains zero.
	feeProvided := gasPriceProvided
	if transaction.Type == types.DynamicFeeTx {
		feeProvided = feeCapProvided
	}
	if !feeProvided {
		recommendedFee, err := e.getGasPrice()
		if err != nil {
			return nil, err
		}

		if transaction.Type == types.DynamicFeeTx {
			transaction.GasFeeCap.SetUint64(recommendedFee)
		} else {
			transaction.GasPrice.SetUint64(recommendedFee)
		}
	}

	// An explicitly supplied zero address is still a supplied sender.
	// With no sender, retain the default simulation caller and leave value
	// affordability to execution; do not manufacture an account balance.
	if fromProvided {
		/* START ALPHA CHANGE 2991 -- reuse the selected-state account for affordability -- */
		accountBalance := new(big.Int)
		if senderAccount != nil {
			accountBalance.Set(senderAccount.Balance)
		}
		/* FINISH ALPHA CHANGE 2991 -- reuse the selected-state account for affordability -- */

		if accountBalance.Cmp(transaction.Value) < 0 {
			return nil, fmt.Errorf(
				"%w: balance %s is below transfer value %s",
				runtime.ErrInsufficientBalance,
				accountBalance,
				transaction.Value,
			)
		}

		available := new(big.Int).Sub(accountBalance, transaction.Value)

		// Dynamic-fee affordability uses the maximum cap, not the
		// effective price observed during a nonpayable simulation.
		fee := transaction.GetGasFeeCap()
		if fee.Sign() > 0 {
			allowance := new(big.Int).Div(available, fee)
			ceiling := new(big.Int).SetUint64(highEnd)

			// Convert only after proving the allowance is below a
			// uint64 ceiling. A larger allowance cannot wrap.
			if allowance.Cmp(ceiling) < 0 {
				highEnd = allowance.Uint64()
			}
		}
	}

	if highEnd < intrinsicGas {
		return nil, fmt.Errorf(
			"gas ceiling %d is below intrinsic gas %d: %w",
			highEnd,
			intrinsicGas,
			state.ErrNotEnoughIntrinsicGas,
		)
	}
	/* FINISH ALPHA CHANGE 2986 -- establish estimation ceilings before execution -- */

	// Checks if executor level valid gas errors occurred
	isGasApplyError := func(err error) bool {
		if errors.Is(err, state.ErrNotEnoughIntrinsicGas) {
			return true
		}

		var expected *state.TransitionApplicationError
		if errors.As(err, &expected) {
			return errors.Is(expected.Err, state.ErrNotEnoughIntrinsicGas)
		}

		return false
	}

	// Checks if EVM level valid gas errors occurred
	isGasEVMError := func(err error) bool {
		return errors.Is(err, runtime.ErrOutOfGas) || errors.Is(err, runtime.ErrCodeStoreOutOfGas)
	}

	// Checks if the EVM reverted during execution
	isEVMRevertError := func(err error) bool {
		return errors.Is(err, runtime.ErrExecutionReverted)
	}

	// Run the transaction with the specified gas value.
	// Returns a status indicating if the transaction failed, return value (data), and the accompanying error
	testTransaction := func(gas uint64, shouldOmitErr bool) (bool, interface{}, error) {
		var data interface{}

		transaction.Gas = gas

		/* START ALPHA CHANGE 2163 -- stop estimation before classifying failed state reads as gas errors -- */
		result, applyErr := e.store.ApplyTxn(header, transaction, nil, true)
		if errors.Is(applyErr, state.ErrLocalStateFailure) {
			return true, nil, applyErr
		}
		if result != nil && errors.Is(result.Err, state.ErrLocalStateFailure) {
			return true, nil, result.Err
		}

		if result != nil {
			data = []byte(hex.EncodeToString(result.ReturnValue))
		}
		/* FINISH ALPHA CHANGE 2163 -- stop estimation before classifying failed state reads as gas errors -- */

		if applyErr != nil {
			// Check the application error.
			// Gas apply errors are valid, and should be ignored
			if isGasApplyError(applyErr) && shouldOmitErr {
				// Specifying the transaction failed, but not providing an error
				// is an indication that a valid error occurred due to low gas,
				// which will increase the lower bound for the search
				return true, data, nil
			}

			return true, data, applyErr
		}

		/* START ALPHA CHANGE 2987 -- reject a missing simulation result -- */
		if result == nil {
			return true, nil, errors.New("gas estimation returned no execution result")
		}
		/* FINISH ALPHA CHANGE 2987 -- reject a missing simulation result -- */

		// Check if an out of gas error happened during EVM execution
		if result.Failed() {
			if isGasEVMError(result.Err) && shouldOmitErr {
				// Specifying the transaction failed, but not providing an error
				// is an indication that a valid error occurred due to low gas,
				// which will increase the lower bound for the search
				return true, data, nil
			}

			if isEVMRevertError(result.Err) {
				// The EVM reverted during execution, attempt to extract the
				// error message and return it
				return true, data, constructErrorFromRevert(result)
			}

			return true, data, result.Err
		}

		return false, nil, nil
	}

	/* START ALPHA CHANGE 2987 -- simulate candidate transfers before returning intrinsic gas -- */
	if transaction.IsValueTransfer() {
		failed, retVal, probeErr := testTransaction(intrinsicGas, false)
		if errors.Is(probeErr, state.ErrLocalStateFailure) {
			return nil, probeErr
		}
		if !failed {
			return argUint64(intrinsicGas), nil
		}

		// Low-gas execution can exhaust gas or revert. Neither proves
		// that the transaction fails at every permitted allowance.
		if !isGasApplyError(probeErr) &&
			!isGasEVMError(probeErr) &&
			!isEVMRevertError(probeErr) {
			return retVal, probeErr
		}

		if highEnd == intrinsicGas {
			return retVal, fmt.Errorf(
				"unable to apply transaction even for the highest gas limit %d: %w",
				highEnd,
				probeErr,
			)
		}

		// The intrinsic allowance already failed. highEnd is strictly
		// greater, so this increment cannot overflow.
		lowEnd = intrinsicGas + 1
	}
	/* FINISH ALPHA CHANGE 2987 -- simulate candidate transfers before returning intrinsic gas -- */

	// Start the binary search for the lowest successful gas allowance
	for lowEnd < highEnd {
		mid := lowEnd + ((highEnd - lowEnd) >> 1) // (lowEnd + highEnd) / 2 can overflow

		/* START ALPHA CHANGE 2163 -- abort gas search immediately on local state failure -- */
		failed, retVal, testErr := testTransaction(mid, true)
		if errors.Is(testErr, state.ErrLocalStateFailure) {
			return nil, testErr
		}
		if testErr != nil && !isEVMRevertError(testErr) {
			/* FINISH ALPHA CHANGE 2163 -- abort gas search immediately on local state failure -- */
			// Reverts are ignored in the binary search, but are checked later on
			// during the execution for the optimal gas limit found
			return retVal, testErr
		}

		if failed {
			// If the transaction failed => increase the gas
			lowEnd = mid + 1
		} else {
			// If the transaction didn't fail => make this ok value the high end
			highEnd = mid
		}
	}

	// Check if the highEnd is a good value to make the transaction pass
	/* START ALPHA CHANGE 2163 -- preserve local failure identity at the final gas probe -- */
	failed, retVal, err := testTransaction(highEnd, false)
	if errors.Is(err, state.ErrLocalStateFailure) {
		return nil, err
	}
	if failed {
		/* FINISH ALPHA CHANGE 2163 -- preserve local failure identity at the final gas probe -- */
		// The transaction shouldn't fail, for whatever reason, at highEnd
		return retVal, fmt.Errorf(
			"unable to apply transaction even for the highest gas limit %d: %w",
			highEnd,
			err,
		)
	}

	return argUint64(highEnd), nil
}

// GetFilterLogs returns an array of logs for the specified filter
func (e *Eth) GetFilterLogs(id string) (interface{}, error) {
	logFilter, err := e.filterManager.GetLogFilterFromID(id)
	if err != nil {
		return nil, err
	}

	return e.filterManager.GetLogsForQuery(logFilter.query)
}

// GetLogs returns an array of logs matching the filter options
func (e *Eth) GetLogs(query *LogQuery) (interface{}, error) {
	/* START ALPHA CHANGE 2511 -- reject null historical log filters -- */
	if query == nil {
		return nil, NewInvalidParamsError("Log filter must be a non-null object")
	}
	/* FINISH ALPHA CHANGE 2511 -- reject null historical log filters -- */

	return e.filterManager.GetLogsForQuery(query)
}

// GetBalance returns the account's balance at the referenced block.
func (e *Eth) GetBalance(address types.Address, filter BlockNumberOrHash) (interface{}, error) {
	header, err := GetHeaderFromBlockNumberOrHash(filter, e.store)
	if err != nil {
		return nil, err
	}

	// Extract the account balance
	acc, err := e.store.GetAccount(header.StateRoot, address)
	/* START ALPHA CHANGE 2163 -- never convert a local state failure into an empty RPC value -- */
	if isAbsentState(err) {
		/* FINISH ALPHA CHANGE 2163 -- never convert a local state failure into an empty RPC value -- */
		// Account not found, return an empty account
		return argUintPtr(0), nil
	} else if err != nil {
		return nil, err
	}

	return argBigPtr(acc.Balance), nil
}

// GetTransactionCount returns account nonce
func (e *Eth) GetTransactionCount(address types.Address, filter BlockNumberOrHash) (interface{}, error) {
	var (
		blockNumber BlockNumber
		header      *types.Header
		err         error
	)

	// The filter is empty, use the latest block by default
	if filter.BlockNumber == nil && filter.BlockHash == nil {
		filter.BlockNumber, _ = createBlockNumberPointer(latest)
	}

	if filter.BlockNumber == nil {
		header, err = GetHeaderFromBlockNumberOrHash(filter, e.store)
		if err != nil {
			return nil, fmt.Errorf("failed to get header from block hash or block number: %w", err)
		}

		blockNumber = BlockNumber(header.Number)
	} else {
		blockNumber = *filter.BlockNumber
	}

	/* START ALPHA CHANGE 2163 -- preserve local nonce failures before absence handling -- */
	nonce, err := GetNextNonce(address, blockNumber, e.store)
	if err != nil {
		if isAbsentState(err) {
			return argUintPtr(0), nil
		}

		return nil, err
	}
	/* FINISH ALPHA CHANGE 2163 -- preserve local nonce failures before absence handling -- */

	return argUintPtr(nonce), nil
}

// GetCode returns account code at given block number
func (e *Eth) GetCode(address types.Address, filter BlockNumberOrHash) (interface{}, error) {
	header, err := GetHeaderFromBlockNumberOrHash(filter, e.store)
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 2163 -- return no bytecode result on a failed read -- */
	code, err := e.store.GetCode(header.StateRoot, address)
	/* FINISH ALPHA CHANGE 2163 -- return no bytecode result on a failed read -- */

	/* START ALPHA CHANGE 2163 -- preserve local bytecode failures before absence handling -- */
	if isAbsentState(err) {
		// If the account doesn't exist / is not initialized yet,
		// return the default value.
		return "0x", nil
	} else if err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2163 -- preserve local bytecode failures before absence handling -- */

	return argBytesPtr(code), nil
}

/* START ALPHA CHANGE 2248 -- return explicit RPC filter capacity errors -- */
// NewFilter creates a filter object for matching logs.
func (e *Eth) NewFilter(filter *LogQuery) (interface{}, error) {
	/* START ALPHA CHANGE 2511 -- reject null filters before registration -- */
	if filter == nil {
		return nil, NewInvalidParamsError("Log filter must be a non-null object")
	}
	/* FINISH ALPHA CHANGE 2511 -- reject null filters before registration -- */

	id := e.filterManager.NewLogFilter(filter, nil)
	if id == "" {
		return nil, &rpcLimitError{"RPC filter capacity exceeded"}
	}

	return id, nil
}

// NewBlockFilter creates a filter for new blocks.
func (e *Eth) NewBlockFilter() (interface{}, error) {
	id := e.filterManager.NewBlockFilter(nil)
	if id == "" {
		return nil, &rpcLimitError{"RPC filter capacity exceeded"}
	}

	return id, nil
}

/* FINISH ALPHA CHANGE 2248 -- return explicit RPC filter capacity errors -- */

// GetFilterChanges is a polling method for a filter, which returns an array of logs which occurred since last poll.
func (e *Eth) GetFilterChanges(id string) (interface{}, error) {
	return e.filterManager.GetFilterChanges(id)
}

// UninstallFilter uninstalls a filter with given ID
func (e *Eth) UninstallFilter(id string) (bool, error) {
	return e.filterManager.Uninstall(id), nil
}

// Unsubscribe uninstalls a filter in a websocket
func (e *Eth) Unsubscribe(id string) (bool, error) {
	return e.filterManager.Uninstall(id), nil
}

// MaxPriorityFeePerGas calculates the priority fee needed for transaction to be included in a block
func (e *Eth) MaxPriorityFeePerGas() (interface{}, error) {
	priorityFee, err := e.store.MaxPriorityFeePerGas()
	if err != nil {
		return nil, err
	}

	return argBigPtr(priorityFee), nil
}

func (e *Eth) FeeHistory(blockCount argUint64, newestBlock BlockNumber,
	rewardPercentiles []float64) (interface{}, error) {
	/* START ALPHA CHANGE 2459 -- reject excess percentiles using the effective RPC limit -- */
	limit := e.feeHistoryPercentileLimit
	if limit == 0 {
		limit = gasprice.DefaultFeeHistoryPercentileLimit
	}

	if uint64(len(rewardPercentiles)) > limit {
		return nil, &rpcLimitError{
			message: fmt.Sprintf(
				"%s: maximum %d, received %d",
				gasprice.ErrTooManyRewardPercentiles,
				limit,
				len(rewardPercentiles),
			),
		}
	}
	/* FINISH ALPHA CHANGE 2459 -- reject excess percentiles using the effective RPC limit -- */

	block, err := GetNumericBlockNumber(newestBlock, e.store)
	if err != nil {
		return nil, fmt.Errorf("could not parse newest block argument. Error: %w", err)
	}

	// Retrieve oldestBlock, baseFeePerGas, gasUsedRatio, and reward synchronously
	history, err := e.store.FeeHistory(uint64(blockCount), block, rewardPercentiles)
	if err != nil {
		/* START ALPHA CHANGE 2441 -- explicitly map both core fee-history resource errors -- */
		if errors.Is(err, gasprice.ErrTooManyRewardPercentiles) ||
			errors.Is(err, gasprice.ErrFeeHistoryResultTooLarge) {
			return nil, &rpcLimitError{message: err.Error()}
		}
		/* FINISH ALPHA CHANGE 2441 -- explicitly map both core fee-history resource errors -- */

		return nil, err
	}

	// Create channels to receive the processed slices asynchronously
	/* START ALPHA CHANGE 2379 -- carry full-width rewards through RPC conversion -- */
	baseFeePerGasCh := make(chan []argUint64)
	gasUsedRatioCh := make(chan []float64)
	rewardCh := make(chan [][]argBig)
	/* FINISH ALPHA CHANGE 2379 -- carry full-width rewards through RPC conversion -- */

	// Process baseFeePerGas asynchronously
	go func() {
		baseFeePerGasCh <- convertToArgUint64Slice(history.BaseFeePerGas)
	}()

	// Process gasUsedRatio asynchronously
	go func() {
		gasUsedRatioCh <- history.GasUsedRatio
	}()

	/* START ALPHA CHANGE 2380 -- convert rewards without narrowing to uint64 -- */
	go func() {
		rewardCh <- convertToArgBigSliceSlice(history.Reward)
	}()
	/* FINISH ALPHA CHANGE 2380 -- convert rewards without narrowing to uint64 -- */

	// Wait for the processed slices from goroutines
	baseFeePerGasResult := <-baseFeePerGasCh
	gasUsedRatioResult := <-gasUsedRatioCh
	rewardResult := <-rewardCh

	result := &feeHistoryResult{
		OldestBlock:   *argUintPtr(history.OldestBlock),
		BaseFeePerGas: baseFeePerGasResult,
		GasUsedRatio:  gasUsedRatioResult,
		Reward:        rewardResult,
	}

	return result, nil
}
