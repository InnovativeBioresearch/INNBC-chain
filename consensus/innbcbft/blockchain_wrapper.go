package innbcbft

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"

	/* START ALPHA CHANGE 1252 -- validate proposal roots before consensus execution -- */
	"github.com/innovativebioresearch/innbc-chain/types/buildroot"
	/* FINISH ALPHA CHANGE 1252 -- validate proposal roots before consensus execution -- */
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
)

const (
	consensusSource = "consensus"
)

var (
	errSendTxnUnsupported = errors.New("system state does not support send transactions")
)

// blockchain is an interface that wraps the methods called on blockchain
type blockchainBackend interface {
	// CurrentHeader returns the header of blockchain block head
	CurrentHeader() *types.Header

	/* START ALPHA CHANGE 2592 -- require confirmation of the selected canonical block -- */
	/* START ALPHA CHANGE 2608 -- expose canonical confirmation without rewriting a block -- */
	/*
		CommitBlock returns nil only after writing the block or confirming
		its identity at its canonical height.
	*/
	CommitBlock(block *types.FullBlock) error

	/* ConfirmCanonicalBlock performs identity confirmation without writing. */
	ConfirmCanonicalBlock(number uint64, hash types.Hash) error
	/* FINISH ALPHA CHANGE 2608 -- expose canonical confirmation without rewriting a block -- */
	/* FINISH ALPHA CHANGE 2592 -- require confirmation of the selected canonical block -- */

	// NewBlockBuilder is a factory method that returns a block builder on top of 'parent'.
	NewBlockBuilder(parent *types.Header, coinbase types.Address,
		txPool txPoolInterface, blockTime time.Duration, logger hclog.Logger) (blockBuilder, error)

	// ProcessBlock builds a final block from given 'block' on top of 'parent'.
	ProcessBlock(parent *types.Header, block *types.Block) (*types.FullBlock, error)

	// GetStateProviderForBlock returns a reference to make queries to the state at 'block'.
	GetStateProviderForBlock(block *types.Header) (contract.Provider, error)

	// GetStateProvider returns a reference to make queries to the provided state.
	GetStateProvider(transition *state.Transition) contract.Provider

	// GetHeaderByNumber returns a reference to block header for the given block number.
	GetHeaderByNumber(number uint64) (*types.Header, bool)

	// GetHeaderByHash returns a reference to block header for the given block hash
	GetHeaderByHash(hash types.Hash) (*types.Header, bool)

	// GetSystemState creates a new instance of SystemState interface
	GetSystemState(provider contract.Provider) SystemState

	// SubscribeEvents subscribes to blockchain events
	SubscribeEvents() blockchain.Subscription

	// UnubscribeEvents unsubscribes from blockchain events
	UnubscribeEvents(subscription blockchain.Subscription)

	// GetChainID returns chain id of the current blockchain
	GetChainID() uint64

	// GetReceiptsByHash retrieves receipts by hash
	GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error)
}

var _ blockchainBackend = &blockchainWrapper{}

type blockchainWrapper struct {
	executor   *state.Executor
	blockchain *blockchain.Blockchain
}

// CurrentHeader returns the header of blockchain block head
func (p *blockchainWrapper) CurrentHeader() *types.Header {
	return p.blockchain.Header()
}

/* START ALPHA CHANGE 2593 -- use identity-confirmed canonical writes for consensus insertion -- */
/* CommitBlock commits or confirms the exact selected block. */
func (p *blockchainWrapper) CommitBlock(block *types.FullBlock) error {
	return p.blockchain.WriteFullBlockConfirmed(block, consensusSource)
}

/* START ALPHA CHANGE 2609 -- delegate identity confirmation to the locked canonical reader -- */
func (p *blockchainWrapper) ConfirmCanonicalBlock(number uint64, hash types.Hash) error {
	return p.blockchain.ConfirmCanonicalBlock(number, hash)
}

/* FINISH ALPHA CHANGE 2609 -- delegate identity confirmation to the locked canonical reader -- */

/* FINISH ALPHA CHANGE 2593 -- use identity-confirmed canonical writes for consensus insertion -- */

// ProcessBlock builds a final block from given 'block' on top of 'parent'
func (p *blockchainWrapper) ProcessBlock(parent *types.Header, block *types.Block) (*types.FullBlock, error) {
	/* START ALPHA CHANGE 1253 -- reject malformed consensus proposal headers before execution -- */
	expectedBaseFee, err := p.blockchain.CalculateBaseFee(parent)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate expected base fee: %w", err)
	}

	if block.Header.BaseFee != expectedBaseFee {
		return nil, fmt.Errorf("%w: have %d, want %d", blockchain.ErrInvalidBaseFee, block.Header.BaseFee, expectedBaseFee)
	}

	if err := p.blockchain.VerifyGasLimit(block.Header, parent); err != nil {
		return nil, fmt.Errorf("invalid gas limit, %w", err)
	}

	if block.Header.Sha3Uncles != types.EmptyUncleHash {
		return nil, fmt.Errorf("%w: have %s, want %s",
			blockchain.ErrInvalidSha3Uncles, block.Header.Sha3Uncles, types.EmptyUncleHash)
	}

	expectedTxRoot := types.EmptyRootHash
	if len(block.Transactions) != 0 {
		expectedTxRoot = buildroot.CalculateTransactionsRoot(block.Transactions, block.Number())
	}

	if block.Header.TxRoot != expectedTxRoot {
		return nil, fmt.Errorf("%w: have %s, want %s",
			blockchain.ErrInvalidTxRoot, block.Header.TxRoot, expectedTxRoot)
	}
	/* FINISH ALPHA CHANGE 1253 -- reject malformed consensus proposal headers before execution -- */

	/* START ALPHA CHANGE 2662 -- validate every reconstructed proposal commitment before trie publication -- */
	header := block.Header.Copy()
	start := time.Now().UTC()

	transition, err := p.executor.BeginTxn(parent.StateRoot, header, types.BytesToAddress(header.Miner))
	if err != nil {
		return nil, err
	}

	// apply transactions from block
	for _, tx := range block.Transactions {
		if err = transition.Write(tx); err != nil {
			return nil, fmt.Errorf("process block tx error, tx = %v, err = %w", tx.Hash, err)
		}
	}

	/* START ALPHA CHANGE 2157 -- stop proposal execution before commitment comparison on local failure -- */
	if stateErr := transition.Error(); stateErr != nil {
		return nil, stateErr
	}
	/* FINISH ALPHA CHANGE 2157 -- stop proposal execution before commitment comparison on local failure -- */

	receipts := transition.Receipts()
	var builtBlock *types.Block

	/* START ALPHA CHANGE 1254 -- compare proposal header against single-pass execution result -- */
	_, _, err = transition.CommitWithValidation(func(root types.Hash) error {
		header.StateRoot = root
		header.GasUsed = transition.TotalGas()
		header.LogsBloom = types.CreateBloom(receipts)

		builtBlock = consensus.BuildBlock(consensus.BuildBlockParams{
			Header:   header,
			Txns:     block.Transactions,
			Receipts: receipts,
		})

		if builtBlock.Header.StateRoot != block.Header.StateRoot {
			return fmt.Errorf("%w: have %s, want %s",
				blockchain.ErrInvalidStateRoot, block.Header.StateRoot, builtBlock.Header.StateRoot)
		}

		if builtBlock.Header.GasUsed != block.Header.GasUsed {
			return fmt.Errorf("%w: have %d, want %d",
				blockchain.ErrInvalidGasUsed, block.Header.GasUsed, builtBlock.Header.GasUsed)
		}

		if builtBlock.Header.ReceiptsRoot != block.Header.ReceiptsRoot {
			return fmt.Errorf("%w: have %s, want %s",
				blockchain.ErrInvalidReceiptsRoot, block.Header.ReceiptsRoot, builtBlock.Header.ReceiptsRoot)
		}

		if builtBlock.Header.LogsBloom != block.Header.LogsBloom {
			return fmt.Errorf("%w: have %s, want %s",
				blockchain.ErrInvalidLogsBloom, block.Header.LogsBloom, builtBlock.Header.LogsBloom)
		}

		if builtBlock.Header.TxRoot != block.Header.TxRoot {
			return fmt.Errorf("%w: have %s, want %s",
				blockchain.ErrInvalidTxRoot, block.Header.TxRoot, builtBlock.Header.TxRoot)
		}

		if builtBlock.Header.Sha3Uncles != block.Header.Sha3Uncles {
			return fmt.Errorf("%w: have %s, want %s",
				blockchain.ErrInvalidSha3Uncles, block.Header.Sha3Uncles, builtBlock.Header.Sha3Uncles)
		}

		if builtBlock.Header.Hash != block.Header.Hash {
			return fmt.Errorf("incorrect block hash: have %s, want %s",
				block.Header.Hash, builtBlock.Header.Hash)
		}

		return nil
	})
	/* FINISH ALPHA CHANGE 1254 -- compare proposal header against single-pass execution result -- */
	if err != nil {
		return nil, fmt.Errorf("failed to validate or commit the state changes: %w", err)
	}

	updateBlockExecutionMetric(start)

	return &types.FullBlock{
		Block:    builtBlock,
		Receipts: receipts,
	}, nil
	/* FINISH ALPHA CHANGE 2662 -- validate every reconstructed proposal commitment before trie publication -- */
}

// GetStateProviderForBlock is an implementation of blockchainBackend interface
func (p *blockchainWrapper) GetStateProviderForBlock(header *types.Header) (contract.Provider, error) {
	transition, err := p.executor.BeginTxn(header.StateRoot, header, types.ZeroAddress)
	if err != nil {
		return nil, err
	}

	return NewStateProvider(transition), nil
}

// GetStateProvider returns a reference to make queries to the provided state
func (p *blockchainWrapper) GetStateProvider(transition *state.Transition) contract.Provider {
	return NewStateProvider(transition)
}

// GetHeaderByNumber is an implementation of blockchainBackend interface
func (p *blockchainWrapper) GetHeaderByNumber(number uint64) (*types.Header, bool) {
	return p.blockchain.GetHeaderByNumber(number)
}

// GetHeaderByHash is an implementation of blockchainBackend interface
func (p *blockchainWrapper) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	return p.blockchain.GetHeaderByHash(hash)
}

// NewBlockBuilder is an implementation of blockchainBackend interface
func (p *blockchainWrapper) NewBlockBuilder(
	parent *types.Header, coinbase types.Address,
	txPool txPoolInterface, blockTime time.Duration, logger hclog.Logger) (blockBuilder, error) {
	/* START ALPHA CHANGE 163 -- calculate block gas limit from already-loaded parent header -- */
	gasLimit := p.blockchain.CalculateGasLimitFromParent(parent)
	/* FINISH ALPHA CHANGE 163 -- calculate block gas limit from already-loaded parent header -- */

	/* START ALPHA CHANGE 1145 -- build blocks with governance-capped dynamic base fee -- */
	baseFee, err := p.blockchain.CalculateBaseFee(parent)
	if err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 1145 -- build blocks with governance-capped dynamic base fee -- */

	return NewBlockBuilder(&BlockBuilderParams{
		BlockTime: blockTime,
		Parent:    parent,
		Coinbase:  coinbase,
		Executor:  p.executor,
		GasLimit:  gasLimit,
		BaseFee:   baseFee,
		TxPool:    txPool,
		Logger:    logger,
	}), nil
}

// GetSystemState is an implementation of blockchainBackend interface
func (p *blockchainWrapper) GetSystemState(provider contract.Provider) SystemState {
	/* START ALPHA CHANGE 450 -- build validator-set-only system state -- */
	return NewSystemState(contracts.ValidatorSetContract, provider)
	/* FINISH ALPHA CHANGE 450 -- build validator-set-only system state -- */
}

func (p *blockchainWrapper) SubscribeEvents() blockchain.Subscription {
	return p.blockchain.SubscribeEvents()
}

func (p *blockchainWrapper) UnubscribeEvents(subscription blockchain.Subscription) {
	p.blockchain.UnsubscribeEvents(subscription)
}

func (p *blockchainWrapper) GetChainID() uint64 {
	return uint64(p.blockchain.Config().ChainID)
}

func (p *blockchainWrapper) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	return p.blockchain.GetReceiptsByHash(hash)
}

var _ contract.Provider = &stateProvider{}

type stateProvider struct {
	transition *state.Transition
}

// NewStateProvider initializes EVM against given state and chain config and returns stateProvider instance
// which is an abstraction for smart contract calls
func NewStateProvider(transition *state.Transition) contract.Provider {
	return &stateProvider{transition: transition}
}

// Call implements the contract.Provider interface to make contract calls directly to the state
/* START ALPHA CHANGE 2158 -- check system contract state errors before interpreting return data -- */
func (s *stateProvider) Call(addr ethgo.Address, input []byte, opts *contract.CallOpts) ([]byte, error) {
	result := s.transition.Call2(contracts.SystemCaller, types.Address(addr), input, big.NewInt(0), 10000000)
	if err := s.transition.Error(); err != nil {
		return nil, err
	}
	if result.Failed() {
		return nil, result.Err
	}

	return result.ReturnValue, nil
}

/* FINISH ALPHA CHANGE 2158 -- check system contract state errors before interpreting return data -- */

// Txn is part of the contract.Provider interface to make Ethereum transactions. We disable this function
// since the system state does not make any transaction
func (s *stateProvider) Txn(ethgo.Address, ethgo.Key, []byte) (contract.Txn, error) {
	return nil, errSendTxnUnsupported
}
