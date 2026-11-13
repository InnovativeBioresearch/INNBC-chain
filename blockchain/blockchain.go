package blockchain

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/innovativebioresearch/innbc-chain/types/buildroot"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
)

const (
	// blockGasTargetDivisor is the bound divisor of the gas limit, used in update calculations
	blockGasTargetDivisor uint64 = 1024

	// defaultCacheSize is the default size for Blockchain LRU cache structures
	defaultCacheSize int = 100
)

var (
	ErrNoBlock              = errors.New("no block data passed in")
	ErrParentNotFound       = errors.New("parent block not found")
	ErrInvalidParentHash    = errors.New("parent block hash is invalid")
	ErrParentHashMismatch   = errors.New("invalid parent block hash")
	ErrInvalidBlockSequence = errors.New("invalid block sequence")
	ErrInvalidSha3Uncles    = errors.New("invalid block sha3 uncles root")
	ErrInvalidTxRoot        = errors.New("invalid block transactions root")
	ErrInvalidReceiptsSize  = errors.New("invalid number of receipts")
	ErrInvalidStateRoot     = errors.New("invalid block state root")
	ErrInvalidGasUsed       = errors.New("invalid block gas used")
	ErrInvalidReceiptsRoot  = errors.New("invalid block receipts root")
	/* START ALPHA CHANGE 1250 -- expose logs bloom validation error for consensus proposal checks -- */
	ErrInvalidLogsBloom = errors.New("invalid block logs bloom")
	/* FINISH ALPHA CHANGE 1250 -- expose logs bloom validation error for consensus proposal checks -- */
	ErrInvalidBaseFee = errors.New("invalid block base fee")
)

// Blockchain is a blockchain reference
type Blockchain struct {
	logger hclog.Logger // The logger object

	db        storage.Storage // The Storage object (database)
	consensus Verifier
	executor  Executor
	txSigner  TxSigner

	config  *chain.Chain // Config containing chain information
	genesis types.Hash   // The hash of the genesis block

	/* START ALPHA CHANGE 785 -- remove total-difficulty cache from blockchain state -- */
	headersCache *lru.Cache // LRU cache for the headers
	/* FINISH ALPHA CHANGE 785 -- remove total-difficulty cache from blockchain state -- */

	// We need to keep track of block receipts between the verification phase
	// and the insertion phase of a new block coming in. To avoid having to
	// execute the transactions twice, we save the receipts from the initial execution
	// in a cache, so we can grab it later when inserting the block.
	// This is of course not an optimal solution - a better one would be to add
	// the receipts to the proposed block (like we do with Transactions and Uncles), but
	// that is currently not possible because it would break backwards compatibility due to
	// insane conditionals in the RLP unmarshal methods for the Block structure, which prevent
	// any new fields from being added
	receiptsCache *lru.Cache // LRU cache for the block receipts

	/* START ALPHA CHANGE 786 -- track current canonical header only -- */
	currentHeader atomic.Pointer[types.Header] // The current header
	/* FINISH ALPHA CHANGE 786 -- track current canonical header only -- */

	stream *eventStream // Event subscriptions

	gpAverage *gasPriceAverage // A reference to the average gas price

	writeLock sync.Mutex
}

// gasPriceAverage keeps track of the average gas price (rolling average)
type gasPriceAverage struct {
	sync.RWMutex

	price *big.Int // The average gas price that gets queried
	count *big.Int // Param used in the avg. gas price calculation
}

type Verifier interface {
	VerifyHeader(header *types.Header) error
	ProcessHeaders(headers []*types.Header) error
	GetBlockCreator(header *types.Header) (types.Address, error)
	PreCommitState(block *types.Block, txn *state.Transition) error
}

type Executor interface {
	ProcessBlock(parentRoot types.Hash, block *types.Block, blockCreator types.Address) (*state.Transition, error)
	ReadMaxBaseFeeWei(root types.Hash) (uint64, error)
}

type TxSigner interface {
	// Sender returns the sender of the transaction
	Sender(tx *types.Transaction) (types.Address, error)
}

type BlockResult struct {
	Root     types.Hash
	Receipts []*types.Receipt
	TotalGas uint64
}

// updateGasPriceAvg updates the rolling average value of the gas price
func (b *Blockchain) updateGasPriceAvg(newValues []*big.Int) {
	/* START ALPHA CHANGE 1947 -- safely ignore empty gas-price average updates -- */
	if len(newValues) == 0 {
		return
	}
	/* FINISH ALPHA CHANGE 1947 -- safely ignore empty gas-price average updates -- */

	b.gpAverage.Lock()
	defer b.gpAverage.Unlock()

	// Sum the values for quick reference
	sum := big.NewInt(0)
	for _, val := range newValues {
		sum = sum.Add(sum, val)
	}

	// There is no previous average data,
	// so this new value set will instantiate it
	if b.gpAverage.count.Uint64() == 0 {
		b.calcArithmeticAverage(newValues, sum)

		return
	}

	// There is existing average data,
	// use it to generate a new average
	b.calcRollingAverage(newValues, sum)
}

// calcArithmeticAverage calculates and sets the arithmetic average
// of the passed in data set
func (b *Blockchain) calcArithmeticAverage(newValues []*big.Int, sum *big.Int) {
	newAverageCount := big.NewInt(int64(len(newValues)))
	newAverage := sum.Div(sum, newAverageCount)

	b.gpAverage.price = newAverage
	b.gpAverage.count = newAverageCount
}

/* START ALPHA CHANGE 1948 -- correct cumulative gas-price average arithmetic -- */
// calcRollingAverage combines the previously observed prices with the new
// values using:
// (old average * old count + sum of new values) / (old count + new count).
func (b *Blockchain) calcRollingAverage(newValues []*big.Int, sum *big.Int) {
	oldWeightedSum := new(big.Int).Mul(
		b.gpAverage.price,
		b.gpAverage.count,
	)
	oldWeightedSum.Add(oldWeightedSum, sum)

	inputSetCount := new(big.Int).SetInt64(int64(len(newValues)))
	newCount := new(big.Int).Add(
		b.gpAverage.count,
		inputSetCount,
	)

	b.gpAverage.price = new(big.Int).Div(oldWeightedSum, newCount)
	b.gpAverage.count = newCount
}

/* FINISH ALPHA CHANGE 1948 -- correct cumulative gas-price average arithmetic -- */

// GetAvgGasPrice returns the average gas price for the chain
func (b *Blockchain) GetAvgGasPrice() *big.Int {
	b.gpAverage.RLock()
	defer b.gpAverage.RUnlock()

	return b.gpAverage.price
}

// NewBlockchain creates a new blockchain object
func NewBlockchain(
	logger hclog.Logger,
	db storage.Storage,
	config *chain.Chain,
	consensus Verifier,
	executor Executor,
	txSigner TxSigner,
) (*Blockchain, error) {
	b := &Blockchain{
		logger:    logger.Named("blockchain"),
		config:    config,
		consensus: consensus,
		db:        db,
		executor:  executor,
		txSigner:  txSigner,
		stream:    newEventStream(),
		gpAverage: &gasPriceAverage{
			price: big.NewInt(0),
			count: big.NewInt(0),
		},
	}

	if err := b.initCaches(defaultCacheSize); err != nil {
		return nil, err
	}

	// Push the initial event to the stream
	b.stream.push(&Event{})

	return b, nil
}

// initCaches initializes the blockchain caches with the specified size
func (b *Blockchain) initCaches(size int) error {
	var err error

	b.headersCache, err = lru.New(size)
	if err != nil {
		return fmt.Errorf("unable to create headers cache, %w", err)
	}

	/* START ALPHA CHANGE 787 -- remove total-difficulty cache initialization -- */
	b.receiptsCache, err = lru.New(size)
	/* FINISH ALPHA CHANGE 787 -- remove total-difficulty cache initialization -- */
	if err != nil {
		return fmt.Errorf("unable to create receipts cache, %w", err)
	}

	return nil
}

/* START ALPHA CHANGE 3169 -- initialize genesis only after storage is confirmed empty -- */
// ComputeGenesis validates existing startup metadata or initializes empty storage.
func (b *Blockchain) ComputeGenesis() error {
	b.writeLock.Lock()
	defer b.writeLock.Unlock()

	if b.config == nil || b.config.Genesis == nil {
		return fmt.Errorf("genesis configuration is missing")
	}

	startup, err := b.db.ReadStartupState()
	if err != nil {
		return fmt.Errorf("failed to inspect blockchain startup state: %w", err)
	}

	if startup.Empty {
		if startup.Head != nil || startup.HasGenesis {
			return fmt.Errorf("inconsistent empty blockchain startup state")
		}
		if err := b.writeGenesis(b.config.Genesis); err != nil {
			return err
		}
	} else {
		if !startup.HasGenesis {
			return fmt.Errorf("initialized blockchain storage has no genesis mapping")
		}
		if startup.Head == nil {
			return fmt.Errorf("initialized blockchain storage has no validated head")
		}
		if startup.GenesisHash != b.config.Genesis.Hash() {
			return fmt.Errorf("genesis file does not match current genesis")
		}

		b.genesis = startup.GenesisHash
		header := startup.Head

		b.logger.Info(
			"Current header",
			"hash",
			header.Hash.String(),
			"number",
			header.Number,
		)
		b.setCurrentHeader(header)
	}

	b.logger.Info("genesis", "hash", b.config.Genesis.Hash())
	return nil
}

/* FINISH ALPHA CHANGE 3169 -- initialize genesis only after storage is confirmed empty -- */

func (b *Blockchain) GetConsensus() Verifier {
	return b.consensus
}

// SetConsensus sets the consensus
func (b *Blockchain) SetConsensus(c Verifier) {
	b.consensus = c
}

// setCurrentHeader sets the current header
/* START ALPHA CHANGE 790 -- remove current total-difficulty tracking -- */
func (b *Blockchain) setCurrentHeader(h *types.Header) {
	// Update the header (atomic)
	header := h.Copy()
	b.currentHeader.Store(header)
}

/* FINISH ALPHA CHANGE 790 -- remove current total-difficulty tracking -- */

// Header returns the current header (atomic)
func (b *Blockchain) Header() *types.Header {
	return b.currentHeader.Load()
}

/* START ALPHA CHANGE 791 -- remove current total-difficulty accessor -- */
// Config returns the blockchain configuration
/* FINISH ALPHA CHANGE 791 -- remove current total-difficulty accessor -- */
func (b *Blockchain) Config() *chain.Params {
	return b.config.Params
}

// GetHeader returns the block header using the hash
func (b *Blockchain) GetHeader(hash types.Hash, number uint64) (*types.Header, bool) {
	return b.GetHeaderByHash(hash)
}

// GetBlock returns the block using the hash
func (b *Blockchain) GetBlock(hash types.Hash, number uint64, full bool) (*types.Block, bool) {
	return b.GetBlockByHash(hash, full)
}

// GetParent returns the parent header
func (b *Blockchain) GetParent(header *types.Header) (*types.Header, bool) {
	return b.readHeader(header.ParentHash)
}

// Genesis returns the genesis block
func (b *Blockchain) Genesis() types.Hash {
	return b.genesis
}

// CalculateGasLimit returns the gas limit of the next block after parent
func (b *Blockchain) CalculateGasLimit(number uint64) (uint64, error) {
	parent, ok := b.GetHeaderByNumber(number - 1)
	if !ok {
		return 0, fmt.Errorf("parent of block %d not found", number)
	}

	return b.CalculateGasLimitFromParent(parent), nil
}

/* START ALPHA CHANGE 162 -- expose gas-limit calculation from known parent header for genesis bootstrap -- */
func (b *Blockchain) CalculateGasLimitFromParent(parent *types.Header) uint64 {
	return b.calculateGasLimit(parent.GasLimit)
}

/* FINISH ALPHA CHANGE 162 -- expose gas-limit calculation from known parent header for genesis bootstrap -- */

// calculateGasLimit calculates gas limit in reference to the block gas target
func (b *Blockchain) calculateGasLimit(parentGasLimit uint64) uint64 {
	/* START ALPHA CHANGE 2057 -- preserve mandatory state transaction capacity in gas-limit calculation -- */
	minimumGasLimit := uint64(types.StateTransactionGasLimit)
	clampToMinimum := func(gasLimit uint64) uint64 {
		return common.Max(gasLimit, minimumGasLimit)
	}

	// The gas limit cannot move more than 1/1024 * parentGasLimit
	// in either direction per block.
	blockGasTarget := b.Config().BlockGasTarget
	if blockGasTarget != 0 {
		blockGasTarget = clampToMinimum(blockGasTarget)
	}

	// Check if the gas limit target has been set.
	if blockGasTarget == 0 {
		// The gas limit target has not been set,
		// so it should use the parent gas limit.
		return clampToMinimum(parentGasLimit)
	}

	// Check if the gas limit is already at the target.
	if parentGasLimit == blockGasTarget {
		// The gas limit is already at the target, no need to move it.
		return clampToMinimum(blockGasTarget)
	}

	delta := parentGasLimit / blockGasTargetDivisor
	if parentGasLimit < blockGasTarget {
		// Bound the increase by the remaining distance before addition.
		// This avoids overflowing parentGasLimit + delta.
		increase := common.Min(delta, blockGasTarget-parentGasLimit)

		return clampToMinimum(parentGasLimit + increase)
	}

	// The gas limit is higher than the gas target, so it should
	// decrease towards the target without crossing the mandatory floor.
	return clampToMinimum(common.Max(blockGasTarget, parentGasLimit-delta))
	/* FINISH ALPHA CHANGE 2057 -- preserve mandatory state transaction capacity in gas-limit calculation -- */
}

// writeGenesis wrapper for the genesis write function
func (b *Blockchain) writeGenesis(genesis *chain.Genesis) error {
	header := genesis.GenesisHeader()
	header.ComputeHash()

	if err := b.writeGenesisImpl(header); err != nil {
		return err
	}

	return nil
}

// writeGenesisImpl writes the genesis file to the DB + blockchain reference
func (b *Blockchain) writeGenesisImpl(header *types.Header) error {
	/* START ALPHA CHANGE 1555 -- write genesis into its dedicated permanent segment -- */
	batchWriter := storage.NewBatchWriterForBlock(b.db, header.Number)
	/* FINISH ALPHA CHANGE 1555 -- write genesis into its dedicated permanent segment -- */

	/* START ALPHA CHANGE 792 -- write genesis canonical header without total difficulty -- */
	batchWriter.PutCanonicalHeader(header)

	if err := b.writeBatchAndUpdate(batchWriter, header, true); err != nil {
		/* FINISH ALPHA CHANGE 792 -- write genesis canonical header without total difficulty -- */
		return err
	}

	// Update the reference
	b.genesis = header.Hash

	// Create an event and send it to the stream
	event := &Event{}
	event.AddNewHeader(header)
	b.stream.push(event)

	return nil
}

// Empty checks if the blockchain is empty
func (b *Blockchain) Empty() bool {
	_, ok := b.db.ReadHeadHash()

	return !ok
}

/* START ALPHA CHANGE 1660 -- expose continuous full-block history availability -- */
func (b *Blockchain) HistoryFrom() uint64 {
	if historyStorage, ok := b.db.(storage.HistoryAvailabilityStorage); ok {
		return historyStorage.HistoryFrom()
	}

	// Standard full-history storage retains the canonical chain from genesis.
	return 0
}

/* FINISH ALPHA CHANGE 1660 -- expose continuous full-block history availability -- */

/* START ALPHA CHANGE 793 -- remove total-difficulty blockchain APIs -- */
// GetReceiptsByHash returns the receipts by their hash
/* FINISH ALPHA CHANGE 793 -- remove total-difficulty blockchain APIs -- */
func (b *Blockchain) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	return b.db.ReadReceipts(hash)
}

// GetBodyByHash returns the body by their hash
func (b *Blockchain) GetBodyByHash(hash types.Hash) (*types.Body, bool) {
	return b.readBody(hash)
}

// GetHeaderByHash returns the header by his hash
func (b *Blockchain) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	return b.readHeader(hash)
}

/* START ALPHA CHANGE 3805 -- preserve cached header read errors for execution callers -- */

// readHeader retains the existing boolean interface for its current callers.
func (b *Blockchain) readHeader(hash types.Hash) (*types.Header, bool) {
	header, err := b.readHeaderWithError(hash)

	return header, err == nil
}

// readHeaderWithError preserves storage failures while sharing the header cache.
func (b *Blockchain) readHeaderWithError(hash types.Hash) (*types.Header, error) {
	if cached, ok := b.headersCache.Get(hash); ok {
		header, ok := cached.(*types.Header)
		if !ok || header == nil {
			return nil, fmt.Errorf("invalid cached header %s", hash.String())
		}

		return header, nil
	}

	header, err := b.db.ReadHeader(hash)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, fmt.Errorf("missing header %s", hash.String())
	}

	header.ComputeHash()
	b.headersCache.Add(hash, header)

	return header, nil
}

/* FINISH ALPHA CHANGE 3805 -- preserve cached header read errors for execution callers -- */

// readBody reads the block's body, using the block hash
func (b *Blockchain) readBody(hash types.Hash) (*types.Body, bool) {
	bb, err := b.db.ReadBody(hash)
	if err != nil {
		b.logger.Error("failed to read body", "err", err)

		return nil, false
	}

	/* START ALPHA CHANGE 1556 -- keep completed history segments immutable during reads -- */
	// Sender recovery is an in-memory compatibility operation. New writes already
	// persist recovered sender fields, so reading an old block must not rewrite
	// the completed segment that owns it.
	b.recoverFromFieldsInTransactions(bb.Transactions)
	/* FINISH ALPHA CHANGE 1556 -- keep completed history segments immutable during reads -- */

	return bb, true
}

/* START ALPHA CHANGE 794 -- remove total-difficulty storage reader -- */
// GetHeaderByNumber returns the header using the block number
/* FINISH ALPHA CHANGE 794 -- remove total-difficulty storage reader -- */
func (b *Blockchain) GetHeaderByNumber(n uint64) (*types.Header, bool) {
	hash, ok := b.db.ReadCanonicalHash(n)
	if !ok {
		return nil, false
	}

	h, ok := b.readHeader(hash)
	if !ok {
		return nil, false
	}

	return h, true
}

// WriteHeadersWithBodies writes a batch of headers
func (b *Blockchain) WriteHeadersWithBodies(headers []*types.Header) error {
	// Check the size
	if len(headers) == 0 {
		return fmt.Errorf("passed in headers array is empty")
	}

	// Validate the chain
	for i := 1; i < len(headers); i++ {
		// Check the sequence
		if headers[i].Number-1 != headers[i-1].Number {
			return fmt.Errorf(
				"number sequence not correct at %d, %d and %d",
				i,
				headers[i].Number,
				headers[i-1].Number,
			)
		}

		// Check if the parent hashes match
		if headers[i].ParentHash != headers[i-1].Hash {
			return fmt.Errorf("parent hash not correct")
		}
	}

	// Write the actual headers in separate batches for now
	for _, header := range headers {
		event := &Event{}

		/* START ALPHA CHANGE 1557 -- route restored headers into their owning segments -- */
		batchWriter := storage.NewBatchWriterForBlock(b.db, header.Number)
		/* FINISH ALPHA CHANGE 1557 -- route restored headers into their owning segments -- */

		/* START ALPHA CHANGE 795 -- write only direct canonical descendants -- */
		if err := b.writeHeaderImpl(batchWriter, event, header); err != nil {
			return err
		}

		if err := b.writeBatchAndUpdate(batchWriter, header, true); err != nil {
			/* FINISH ALPHA CHANGE 795 -- write only direct canonical descendants -- */
			return err
		}

		// Notify the event stream
		b.dispatchEvent(event)
	}

	return nil
}

// VerifyPotentialBlock does the minimal block verification without consulting the
// consensus layer. Should only be used if consensus checks are done
// outside the method call
func (b *Blockchain) VerifyPotentialBlock(block *types.Block) error {
	// Do just the initial block verification
	_, err := b.verifyBlock(block)

	return err
}

// VerifyFinalizedBlock verifies that the block is valid by performing a series of checks.
// It is assumed that the block status is sealed (committed)
func (b *Blockchain) VerifyFinalizedBlock(block *types.Block) (*types.FullBlock, error) {
	// Make sure the consensus layer verifies this block header
	if err := b.consensus.VerifyHeader(block.Header); err != nil {
		return nil, fmt.Errorf("failed to verify the header: %w", err)
	}

	// Do the initial block verification
	receipts, err := b.verifyBlock(block)
	if err != nil {
		return nil, err
	}

	return &types.FullBlock{Block: block, Receipts: receipts}, nil
}

// verifyBlock does the base (common) block verification steps by
// verifying the block body as well as the parent information
func (b *Blockchain) verifyBlock(block *types.Block) ([]*types.Receipt, error) {
	// Make sure the block is present
	if block == nil {
		return nil, ErrNoBlock
	}

	// Make sure the block is in line with the parent block
	if err := b.verifyBlockParent(block); err != nil {
		return nil, err
	}

	// Make sure the block body data is valid
	return b.verifyBlockBody(block)
}

// verifyBlockParent makes sure that the child block is in line
// with the locally saved parent block. This means checking:
// - The parent exists
// - The hashes match up
// - The block numbers match up
// - The block gas limit / used matches up
func (b *Blockchain) verifyBlockParent(childBlock *types.Block) error {
	// Grab the parent block
	parentHash := childBlock.ParentHash()
	parent, ok := b.readHeader(parentHash)

	if !ok {
		b.logger.Error(fmt.Sprintf(
			"parent of %s (%d) not found: %s",
			childBlock.Hash().String(),
			childBlock.Number(),
			parentHash,
		))

		return ErrParentNotFound
	}

	// Make sure the hash is valid
	if parent.Hash == types.ZeroHash {
		return ErrInvalidParentHash
	}

	// Make sure the hashes match up
	if parentHash != parent.Hash {
		return ErrParentHashMismatch
	}

	// Make sure the block numbers are correct
	if childBlock.Number()-1 != parent.Number {
		b.logger.Error(fmt.Sprintf(
			"number sequence not correct at %d and %d",
			childBlock.Number(),
			parent.Number,
		))

		return ErrInvalidBlockSequence
	}

	// Make sure the gas limit is within correct bounds
	if gasLimitErr := b.verifyGasLimit(childBlock.Header, parent); gasLimitErr != nil {
		return fmt.Errorf("invalid gas limit, %w", gasLimitErr)
	}

	/* START ALPHA CHANGE 1143 -- verify child base fee against dynamic governance-capped value -- */
	expectedBaseFee, err := b.CalculateBaseFee(parent)
	if err != nil {
		return fmt.Errorf("failed to calculate expected base fee: %w", err)
	}

	if childBlock.Header.BaseFee != expectedBaseFee {
		return fmt.Errorf("%w: have %d, want %d", ErrInvalidBaseFee, childBlock.Header.BaseFee, expectedBaseFee)
	}
	/* FINISH ALPHA CHANGE 1143 -- verify child base fee against dynamic governance-capped value -- */

	return nil
}

// verifyBlockBody verifies that the block body is valid. This means checking:
// - The trie roots match up (state, transactions, receipts, uncles)
// - The receipts match up
// - The execution result matches up
func (b *Blockchain) verifyBlockBody(block *types.Block) ([]*types.Receipt, error) {
	// Make sure the Uncles root matches up
	if hash := buildroot.CalculateUncleRoot(block.Uncles); hash != block.Header.Sha3Uncles {
		b.logger.Error(fmt.Sprintf(
			"uncle root hash mismatch: have %s, want %s",
			hash,
			block.Header.Sha3Uncles,
		))

		return nil, ErrInvalidSha3Uncles
	}

	// Make sure the transactions root matches up
	if hash := buildroot.CalculateTransactionsRoot(block.Transactions, block.Number()); hash != block.Header.TxRoot {
		b.logger.Error(fmt.Sprintf(
			"incorrect tx root (expected: %s, actual: %s)",
			hash,
			block.Header.TxRoot,
		))

		return nil, ErrInvalidTxRoot
	}

	/* START ALPHA CHANGE 2663 -- rely on execution validation before publication without repeating comparisons -- */
	// Execute and validate before publishing state or caching receipts.
	blockResult, executeErr := b.executeBlockTransactions(block)
	if executeErr != nil {
		return nil, fmt.Errorf("unable to execute block transactions, %w", executeErr)
	}

	return blockResult.Receipts, nil
	/* FINISH ALPHA CHANGE 2663 -- rely on execution validation before publication without repeating comparisons -- */
}

// verifyBlockResult verifies that the block transaction execution result
// matches up to the expected values
func (br *BlockResult) verifyBlockResult(referenceBlock *types.Block) error {
	// Make sure the number of receipts matches the number of transactions
	if len(br.Receipts) != len(referenceBlock.Transactions) {
		return ErrInvalidReceiptsSize
	}

	// Make sure the world state root matches up
	if br.Root != referenceBlock.Header.StateRoot {
		return ErrInvalidStateRoot
	}

	// Make sure the gas used is valid
	if br.TotalGas != referenceBlock.Header.GasUsed {
		return ErrInvalidGasUsed
	}

	// Make sure the receipts root matches up
	receiptsRoot := buildroot.CalculateReceiptsRoot(br.Receipts)
	if receiptsRoot != referenceBlock.Header.ReceiptsRoot {
		return ErrInvalidReceiptsRoot
	}

	/* START ALPHA CHANGE 2289 -- verify block bloom using locally computed receipt blooms -- */
	var logsBloom types.Bloom
	for _, receipt := range br.Receipts {
		for index, value := range receipt.LogsBloom {
			logsBloom[index] |= value
		}
	}

	if logsBloom != referenceBlock.Header.LogsBloom {
		return ErrInvalidLogsBloom
	}
	/* FINISH ALPHA CHANGE 2289 -- verify block bloom using locally computed receipt blooms -- */

	return nil
}

// executeBlockTransactions executes the transactions in the block locally,
// and reports back the block execution result
func (b *Blockchain) executeBlockTransactions(block *types.Block) (*BlockResult, error) {
	header := block.Header

	parent, ok := b.readHeader(header.ParentHash)
	if !ok {
		return nil, ErrParentNotFound
	}

	blockCreator, err := b.consensus.GetBlockCreator(header)
	if err != nil {
		return nil, err
	}

	txn, err := b.executor.ProcessBlock(parent.StateRoot, block, blockCreator)
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 2157 -- give pre-commit state failures precedence over hook errors -- */
	hookErr := b.consensus.PreCommitState(block, txn)
	if stateErr := txn.Error(); stateErr != nil {
		return nil, fmt.Errorf("pre-commit block %d state: %w", block.Number(), stateErr)
	}
	if hookErr != nil {
		return nil, hookErr
	}
	/* FINISH ALPHA CHANGE 2157 -- give pre-commit state failures precedence over hook errors -- */

	/* START ALPHA CHANGE 2664 -- validate imported execution before publishing state and receipts -- */
	result := &BlockResult{
		Receipts: txn.Receipts(),
		TotalGas: txn.TotalGas(),
	}

	var validationErr error
	_, _, err = txn.CommitWithValidation(func(root types.Hash) error {
		result.Root = root
		validationErr = result.verifyBlockResult(block)

		return validationErr
	})
	if err != nil {
		if validationErr != nil {
			return nil, fmt.Errorf("unable to verify block execution result, %w", validationErr)
		}

		return nil, fmt.Errorf("failed to commit the state changes: %w", err)
	}

	// Cache only validated execution whose state publication succeeded.
	b.receiptsCache.Add(header.Hash, result.Receipts)

	return result, nil
	/* FINISH ALPHA CHANGE 2664 -- validate imported execution before publishing state and receipts -- */
}

/* START ALPHA CHANGE 2591 -- confirm consensus block identity under the canonical write lock -- */
/* START ALPHA CHANGE 2605 -- confirm selected identity at a retained canonical height -- */
/* ErrCanonicalBlockNotConfirmed means the candidate's canonical identity could not be established. */
var ErrCanonicalBlockNotConfirmed = errors.New("canonical block identity could not be confirmed")

/*
ConfirmCanonicalBlock confirms identity without writing a block.
The write lock serializes confirmation with canonical block insertion.
*/
func (b *Blockchain) ConfirmCanonicalBlock(number uint64, hash types.Hash) error {
	b.writeLock.Lock()
	defer b.writeLock.Unlock()

	return b.confirmCanonicalBlock(number, hash)
}

/* The caller holds b.writeLock. */
func (b *Blockchain) confirmCanonicalBlock(number uint64, hash types.Hash) error {
	head := b.Header()
	if head == nil || number > head.Number {
		return fmt.Errorf("%w: height %d is not covered by the canonical head",
			ErrCanonicalBlockNotConfirmed, number)
	}

	canonicalHash := head.Hash
	if number < head.Number {
		if b.db == nil {
			return fmt.Errorf("%w: canonical storage is unavailable",
				ErrCanonicalBlockNotConfirmed)
		}

		var ok bool
		canonicalHash, ok = b.db.ReadCanonicalHash(number)
		if !ok {
			return fmt.Errorf("%w: canonical hash is unavailable at height %d",
				ErrCanonicalBlockNotConfirmed, number)
		}
	}

	if canonicalHash != hash {
		return fmt.Errorf("%w: candidate %s, canonical %s at height %d",
			ErrCanonicalBlockNotConfirmed, hash.String(), canonicalHash.String(), number)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2605 -- confirm selected identity at a retained canonical height -- */

// WriteFullBlock writes a single block to the local blockchain.
// It doesn't do any kind of verification, only commits the block to the DB
// This function is a copy of WriteBlock but with a full block which does not
// require to compute again the Receipts.
func (b *Blockchain) WriteFullBlock(fblock *types.FullBlock, source string) error {
	return b.writeFullBlock(fblock, source, false)
}

/* START ALPHA CHANGE 2606 -- describe confirmation of already canonical blocks -- */
/*
WriteFullBlockConfirmed succeeds only after writing the candidate or
confirming its hash at the candidate's canonical height.
A higher head uses one canonical-hash lookup; missing history is not
treated as proof of insertion.
*/
/* FINISH ALPHA CHANGE 2606 -- describe confirmation of already canonical blocks -- */
func (b *Blockchain) WriteFullBlockConfirmed(fblock *types.FullBlock, source string) error {
	return b.writeFullBlock(fblock, source, true)
}

func (b *Blockchain) writeFullBlock(
	fblock *types.FullBlock,
	source string,
	confirmIdentity bool,
) error {
	b.writeLock.Lock()
	defer b.writeLock.Unlock()

	if confirmIdentity && (fblock == nil || fblock.Block == nil || fblock.Block.Header == nil) {
		return fmt.Errorf("%w: missing candidate header", ErrCanonicalBlockNotConfirmed)
	}

	block := fblock.Block
	head := b.Header()

	if confirmIdentity && head == nil {
		return fmt.Errorf("%w: current canonical head is not initialized", ErrCanonicalBlockNotConfirmed)
	}

	if block.Number() <= head.Number {
		/* START ALPHA CHANGE 2607 -- confirm covered consensus blocks instead of rejecting a higher head -- */
		if confirmIdentity {
			if err := b.confirmCanonicalBlock(block.Number(), block.Hash()); err != nil {
				return err
			}
		}
		/* FINISH ALPHA CHANGE 2607 -- confirm covered consensus blocks instead of rejecting a higher head -- */

		b.logger.Info("block already inserted", "block", block.Number(), "source", source)

		return nil
	}

	header := block.Header

	/* START ALPHA CHANGE 1558 -- route full-block writes into their owning segment -- */
	batchWriter := storage.NewBatchWriterForBlock(b.db, header.Number)
	/* FINISH ALPHA CHANGE 1558 -- route full-block writes into their owning segment -- */

	if err := b.writeBody(batchWriter, block); err != nil {
		return err
	}

	// Write the header to the chain
	evnt := &Event{Source: source}

	/* START ALPHA CHANGE 813 -- write full blocks as direct canonical descendants -- */
	if err := b.writeHeaderImpl(batchWriter, evnt, header); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 813 -- write full blocks as direct canonical descendants -- */

	// write the receipts, do it only after the header has been written.
	// Otherwise, a client might ask for a header once the receipt is valid,
	// but before it is written into the storage
	batchWriter.PutReceipts(block.Hash(), fblock.Receipts)

	/* START ALPHA CHANGE 863 -- clarify archive restore consensus snapshot update -- */
	// Update consensus validator snapshot metadata for the restored canonical header.
	if err := b.consensus.ProcessHeaders([]*types.Header{header}); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 863 -- clarify archive restore consensus snapshot update -- */

	// Update the average gas price
	b.updateGasPriceAvgWithBlock(block)

	/* START ALPHA CHANGE 814 -- update full block canonical head without total difficulty -- */
	if err := b.writeBatchAndUpdate(batchWriter, header, true); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 814 -- update full block canonical head without total difficulty -- */

	b.dispatchEvent(evnt)

	logArgs := []interface{}{
		"number", header.Number,
		"txs", len(block.Transactions),
		"hash", header.Hash,
		"parent", header.ParentHash,
		"source", source,
	}

	if prevHeader, ok := b.GetHeaderByNumber(header.Number - 1); ok {
		diff := header.Timestamp - prevHeader.Timestamp
		logArgs = append(logArgs, "generation_time_in_seconds", diff)
	}

	b.logger.Info("new block", logArgs...)

	return nil
}

/* FINISH ALPHA CHANGE 2591 -- confirm consensus block identity under the canonical write lock -- */

// WriteBlock writes a single block to the local blockchain.
// It doesn't do any kind of verification, only commits the block to the DB
func (b *Blockchain) WriteBlock(block *types.Block, source string) error {
	b.writeLock.Lock()
	defer b.writeLock.Unlock()

	if block.Number() <= b.Header().Number {
		b.logger.Info("block already inserted", "block", block.Number(), "source", source)

		return nil
	}

	header := block.Header

	/* START ALPHA CHANGE 1559 -- route executed-block writes into their owning segment -- */
	batchWriter := storage.NewBatchWriterForBlock(b.db, header.Number)
	/* FINISH ALPHA CHANGE 1559 -- route executed-block writes into their owning segment -- */

	if err := b.writeBody(batchWriter, block); err != nil {
		return err
	}

	// Write the header to the chain
	evnt := &Event{Source: source}

	/* START ALPHA CHANGE 813 -- write full blocks as direct canonical descendants -- */
	if err := b.writeHeaderImpl(batchWriter, evnt, header); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 813 -- write full blocks as direct canonical descendants -- */

	// Fetch the block receipts
	blockReceipts, receiptsErr := b.extractBlockReceipts(block)
	if receiptsErr != nil {
		return receiptsErr
	}

	// write the receipts, do it only after the header has been written.
	// Otherwise, a client might ask for a header once the receipt is valid,
	// but before it is written into the storage
	batchWriter.PutReceipts(block.Hash(), blockReceipts)

	/* START ALPHA CHANGE 862 -- clarify consensus snapshot update is not state snapshot sync -- */
	// Update consensus validator snapshot metadata for the new canonical header.
	if err := b.consensus.ProcessHeaders([]*types.Header{header}); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 862 -- clarify consensus snapshot update is not state snapshot sync -- */

	// Update the average gas price
	b.updateGasPriceAvgWithBlock(block)

	/* START ALPHA CHANGE 814 -- update full block canonical head without total difficulty -- */
	if err := b.writeBatchAndUpdate(batchWriter, header, true); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 814 -- update full block canonical head without total difficulty -- */

	b.dispatchEvent(evnt)

	logArgs := []interface{}{
		"number", header.Number,
		"txs", len(block.Transactions),
		"hash", header.Hash,
		"parent", header.ParentHash,
		"source", source,
	}

	if prevHeader, ok := b.GetHeaderByNumber(header.Number - 1); ok {
		diff := header.Timestamp - prevHeader.Timestamp
		logArgs = append(logArgs, "generation_time_in_seconds", diff)
	}

	b.logger.Info("new block", logArgs...)

	return nil
}

// GetCachedReceipts retrieves cached receipts for given headerHash
func (b *Blockchain) GetCachedReceipts(headerHash types.Hash) ([]*types.Receipt, error) {
	receipts, found := b.receiptsCache.Get(headerHash)
	if !found {
		return nil, fmt.Errorf("failed to retrieve receipts for header hash: %s", headerHash)
	}

	extractedReceipts, ok := receipts.([]*types.Receipt)
	if !ok {
		return nil, errors.New("invalid type assertion for receipts")
	}

	return extractedReceipts, nil
}

// extractBlockReceipts extracts the receipts from the passed in block
func (b *Blockchain) extractBlockReceipts(block *types.Block) ([]*types.Receipt, error) {
	// Check the cache for the block receipts
	receipts, ok := b.receiptsCache.Get(block.Header.Hash)
	if !ok {
		// No receipts found in the cache, execute the transactions from the block
		// and fetch them
		blockResult, err := b.executeBlockTransactions(block)
		if err != nil {
			return nil, err
		}

		return blockResult.Receipts, nil
	}

	extractedReceipts, ok := receipts.([]*types.Receipt)
	if !ok {
		return nil, errors.New("invalid type assertion for receipts")
	}

	return extractedReceipts, nil
}

// updateGasPriceAvgWithBlock extracts the gas price information from the
// block, and updates the average gas price for the chain accordingly
func (b *Blockchain) updateGasPriceAvgWithBlock(block *types.Block) {
	if len(block.Transactions) < 1 {
		// No transactions in the block,
		// so no gas price average to update
		return
	}

	gasPrices := make([]*big.Int, len(block.Transactions))
	for i, transaction := range block.Transactions {
		gasPrices[i] = transaction.GetGasPrice(block.Header.BaseFee)
	}

	b.updateGasPriceAvg(gasPrices)
}

// writeBody writes the block body to the DB.
// Additionally, it also updates the txn lookup, for txnHash -> block lookups
func (b *Blockchain) writeBody(batchWriter *storage.BatchWriter, block *types.Block) error {
	// Recover 'from' field in tx before saving
	// Because the block passed from the consensus layer doesn't have from field in tx,
	// due to missing encoding in RLP
	if err := b.recoverFromFieldsInBlock(block); err != nil {
		return err
	}

	// Write the full body (txns + receipts)
	batchWriter.PutBody(block.Header.Hash, block.Body())

	// Write txn lookups (txHash -> block)
	for _, txn := range block.Transactions {
		batchWriter.PutTxLookup(txn.Hash, block.Hash())
	}

	return nil
}

// ReadTxLookup returns the block hash using the transaction hash
func (b *Blockchain) ReadTxLookup(hash types.Hash) (types.Hash, bool) {
	v, ok := b.db.ReadTxLookup(hash)

	return v, ok
}

/* START ALPHA CHANGE 1586 -- remove superseded record-level history pruning -- */
/* FINISH ALPHA CHANGE 1586 -- remove superseded record-level history pruning -- */

// recoverFromFieldsInBlock recovers 'from' fields in the transactions of the given block
// return error if the invalid signature found
func (b *Blockchain) recoverFromFieldsInBlock(block *types.Block) error {
	for _, tx := range block.Transactions {
		if tx.From != types.ZeroAddress || tx.Type == types.StateTx {
			continue
		}

		sender, err := b.txSigner.Sender(tx)
		if err != nil {
			return err
		}

		tx.From = sender
	}

	return nil
}

// recoverFromFieldsInTransactions recovers 'from' fields in the transactions
// log as warning if failing to recover one address
func (b *Blockchain) recoverFromFieldsInTransactions(transactions []*types.Transaction) bool {
	updated := false

	for _, tx := range transactions {
		if tx.From != types.ZeroAddress || tx.Type == types.StateTx {
			continue
		}

		sender, err := b.txSigner.Sender(tx)
		if err != nil {
			b.logger.Warn("failed to recover from address in Tx", "hash", tx.Hash, "err", err)

			continue
		}

		tx.From = sender
		updated = true
	}

	return updated
}

/* START ALPHA CHANGE 1251 -- harden and expose gas-limit validation for consensus proposals -- */
// VerifyGasLimit validates a child header gas limit against its parent.
func (b *Blockchain) VerifyGasLimit(header *types.Header, parentHeader *types.Header) error {
	return b.verifyGasLimit(header, parentHeader)
}

// verifyGasLimit is a helper function for validating a gas limit in a header
func (b *Blockchain) verifyGasLimit(header *types.Header, parentHeader *types.Header) error {
	/* START ALPHA CHANGE 2058 -- reject block gas limits below mandatory state transaction capacity -- */
	if header.GasLimit < types.StateTransactionGasLimit {
		return fmt.Errorf(
			"invalid gas limit, limit = %d, minimum = %d",
			header.GasLimit,
			types.StateTransactionGasLimit,
		)
	}
	/* FINISH ALPHA CHANGE 2058 -- reject block gas limits below mandatory state transaction capacity -- */

	if header.GasUsed > header.GasLimit {
		return fmt.Errorf(
			"block gas used exceeds gas limit, limit = %d, used=%d",
			header.GasLimit,
			header.GasUsed,
		)
	}

	// Skip block limit difference check for genesis
	if header.Number == 0 {
		return nil
	}

	var diff uint64
	if header.GasLimit >= parentHeader.GasLimit {
		diff = header.GasLimit - parentHeader.GasLimit
	} else {
		diff = parentHeader.GasLimit - header.GasLimit
	}

	limit := parentHeader.GasLimit / blockGasTargetDivisor
	if diff > limit {
		return fmt.Errorf(
			"invalid gas limit, limit = %d, want %d +- %d",
			header.GasLimit,
			parentHeader.GasLimit,
			limit,
		)
	}

	return nil
}

/* FINISH ALPHA CHANGE 1251 -- harden and expose gas-limit validation for consensus proposals -- */

/* START ALPHA CHANGE 3802 -- cache bounded ancestor hashes within each execution transition -- */
/* START ALPHA CHANGE 3806 -- propagate ancestor read failures through the bounded hash helper -- */

// GetHashHelper returns an ancestor-hash lookup anchored to the supplied header.
// Each execution transition owns its helper and invokes it serially.
func (b *Blockchain) GetHashHelper(header *types.Header) func(uint64) (types.Hash, error) {
	number, parentHash := header.Number, header.ParentHash

	// Allocate only when a valid ancestor is requested. Index zero contains
	// the parent hash; subsequent entries contain progressively older hashes.
	var hashes []types.Hash

	return func(i uint64) (types.Hash, error) {
		if i >= number || number-i > 256 {
			return types.ZeroHash, nil
		}

		index := number - i - 1

		if len(hashes) == 0 {
			hashes = append(hashes, parentHash)
		}

		// Preserve the verified prefix if extending the ancestry fails.
		for uint64(len(hashes)) <= index {
			ancestorHash := hashes[len(hashes)-1]

			ancestor, err := b.readHeaderWithError(ancestorHash)
			if err != nil {
				return types.ZeroHash, fmt.Errorf(
					"read ancestor %s for BLOCKHASH(%d): %w",
					ancestorHash.String(),
					i,
					err,
				)
			}

			hashes = append(hashes, ancestor.ParentHash)
		}

		return hashes[index], nil
	}
}

/* FINISH ALPHA CHANGE 3806 -- propagate ancestor read failures through the bounded hash helper -- */
/* FINISH ALPHA CHANGE 3802 -- cache bounded ancestor hashes within each execution transition -- */

// GetHashByNumber returns the block hash using the block number
func (b *Blockchain) GetHashByNumber(blockNumber uint64) types.Hash {
	block, ok := b.GetBlockByNumber(blockNumber, false)
	if !ok {
		return types.Hash{}
	}

	return block.Hash()
}

// dispatchEvent pushes a new event to the stream
func (b *Blockchain) dispatchEvent(evnt *Event) {
	b.stream.push(evnt)
}

/* START ALPHA CHANGE 796 -- replace TD fork choice with INNBCBFT canonical append rule -- */
// writeHeaderImpl writes a header as the next canonical block.
//
// INNBCBFT finality does not use PoW total difficulty or competing fork heads.
// A block is accepted into storage only if it directly extends the current
// canonical head. Consensus and execution validation happen before this point.
func (b *Blockchain) writeHeaderImpl(
	batchWriter *storage.BatchWriter,
	evnt *Event,
	header *types.Header,
) error {
	currentHeader := b.Header()
	if currentHeader == nil {
		return errors.New("current canonical header is not initialized")
	}

	if header.ParentHash != currentHeader.Hash {
		return fmt.Errorf(
			"non-canonical header %s (%d): parent %s does not match current head %s (%d)",
			header.Hash.String(),
			header.Number,
			header.ParentHash.String(),
			currentHeader.Hash.String(),
			currentHeader.Number,
		)
	}

	batchWriter.PutCanonicalHeader(header)

	evnt.Type = EventHead
	evnt.AddNewHeader(header)

	return nil
}

/* FINISH ALPHA CHANGE 796 -- replace TD fork choice with INNBCBFT canonical append rule -- */

// GetBlockByHash returns the block using the block hash
func (b *Blockchain) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	header, ok := b.readHeader(hash)
	if !ok {
		return nil, false
	}

	block := &types.Block{
		Header: header,
	}

	if !full || header.Number == 0 {
		return block, true
	}

	// Load the entire block body
	body, ok := b.readBody(hash)
	if !ok {
		return block, false
	}

	// Set the transactions and uncles
	block.Transactions = body.Transactions
	block.Uncles = body.Uncles

	return block, true
}

// GetBlockByNumber returns the block using the block number
func (b *Blockchain) GetBlockByNumber(blockNumber uint64, full bool) (*types.Block, bool) {
	blockHash, ok := b.db.ReadCanonicalHash(blockNumber)
	if !ok {
		return nil, false
	}

	// if blockNumber 0 (genesis block), do not try and get the full block
	if blockNumber == uint64(0) {
		full = false
	}

	return b.GetBlockByHash(blockHash, full)
}

// Close closes the DB connection
func (b *Blockchain) Close() error {
	return b.db.Close()
}

// CalculateBaseFee calculates the basefee of the header.
func (b *Blockchain) CalculateBaseFee(parent *types.Header) (uint64, error) {
	// Return zero base fee is a london hardfork is not enabled
	if !b.config.Params.Forks.IsActive(chain.London, parent.Number) {
		return 0, nil
	}

	var baseFee uint64

	// Check if this is the first London hardfork block.
	// Should return chain.GenesisBaseFee ins this case.
	if parent.BaseFee == 0 {
		if b.config.Genesis.BaseFee > 0 {
			baseFee = b.config.Genesis.BaseFee
		} else {
			baseFee = chain.GenesisBaseFee
		}

		return b.applyMaxBaseFee(parent, baseFee)
	}

	parentGasTarget := parent.GasLimit / b.config.Genesis.BaseFeeEM
	if parentGasTarget == 0 {
		return b.applyMaxBaseFee(parent, common.Max(parent.BaseFee, 1))
	}

	// If the parent gasUsed is the same as the target, the baseFee remains unchanged.
	if parent.GasUsed == parentGasTarget {
		return b.applyMaxBaseFee(parent, parent.BaseFee)
	}

	// If the parent block used more gas than its target, the baseFee should increase.
	if parent.GasUsed > parentGasTarget {
		gasUsedDelta := parent.GasUsed - parentGasTarget
		baseFeeDelta := b.calcBaseFeeDelta(gasUsedDelta, parentGasTarget, parent.BaseFee)
		baseFeeIncrease := common.Max(baseFeeDelta, 1)

		if math.MaxUint64-parent.BaseFee < baseFeeIncrease {
			return b.applyMaxBaseFee(parent, math.MaxUint64)
		}

		return b.applyMaxBaseFee(parent, parent.BaseFee+baseFeeIncrease)
	}

	// START ALPHA CHANGE 1001 -- keep London base fee floored at one wei --
	// Otherwise, if the parent block used less gas than its target, the baseFee should decrease.
	gasUsedDelta := parentGasTarget - parent.GasUsed
	baseFeeDelta := b.calcBaseFeeDelta(gasUsedDelta, parentGasTarget, parent.BaseFee)

	if baseFeeDelta >= parent.BaseFee {
		return b.applyMaxBaseFee(parent, 1)
	}

	return b.applyMaxBaseFee(parent, common.Max(parent.BaseFee-baseFeeDelta, 1))
	// FINISH ALPHA CHANGE 1001 -- keep London base fee floored at one wei --
}

/* START ALPHA CHANGE 1144 -- apply optional governance max base fee cap -- */
func (b *Blockchain) applyMaxBaseFee(parent *types.Header, baseFee uint64) (uint64, error) {
	baseFee = common.Max(baseFee, 1)

	maxBaseFeeWei, err := b.executor.ReadMaxBaseFeeWei(parent.StateRoot)
	if err != nil {
		return 0, err
	}

	if maxBaseFeeWei > 0 && baseFee > maxBaseFeeWei {
		return common.Max(maxBaseFeeWei, 1), nil
	}

	return baseFee, nil
}

/* FINISH ALPHA CHANGE 1144 -- apply optional governance max base fee cap -- */

/* START ALPHA CHANGE 1070 -- calculate base fee delta without uint64 overflow -- */
func (b *Blockchain) calcBaseFeeDelta(gasUsedDelta, parentGasTarget, baseFee uint64) uint64 {
	if parentGasTarget == 0 || b.config.Genesis.BaseFeeChangeDenom == 0 {
		return 0
	}

	delta := new(big.Int).SetUint64(baseFee)
	delta.Mul(delta, new(big.Int).SetUint64(gasUsedDelta))
	delta.Div(delta, new(big.Int).SetUint64(parentGasTarget))
	delta.Div(delta, new(big.Int).SetUint64(b.config.Genesis.BaseFeeChangeDenom))

	if !delta.IsUint64() {
		return math.MaxUint64
	}

	return delta.Uint64()
}

/* FINISH ALPHA CHANGE 1070 -- calculate base fee delta without uint64 overflow -- */

/* START ALPHA CHANGE 797 -- update canonical head without total difficulty -- */
func (b *Blockchain) writeBatchAndUpdate(
	batchWriter *storage.BatchWriter,
	header *types.Header,
	isCanonnical bool) error {
	/* FINISH ALPHA CHANGE 797 -- update canonical head without total difficulty -- */
	if err := batchWriter.WriteBatch(); err != nil {
		return err
	}

	if isCanonnical {
		/* START ALPHA CHANGE 798 -- set canonical head only after batch write -- */
		b.headersCache.Add(header.Hash, header)
		b.setCurrentHeader(header) // Update the blockchain reference
		/* FINISH ALPHA CHANGE 798 -- set canonical head only after batch write -- */
	}

	return nil
}
