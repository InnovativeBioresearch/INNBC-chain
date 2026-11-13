package txpool

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/armon/go-metrics"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/grpc"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/forkmanager"
	"github.com/innovativebioresearch/innbc-chain/network"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
)

const (
	/* START ALPHA CHANGE 3007 -- expose the existing admission size policy without changing its allowance -- */
	MaxTransactionSize = 128 * 1024 // 128 KiB, transaction-pool admission only

	txSlotSize  = 32 * 1024
	txMaxSize   = MaxTransactionSize
	topicNameV1 = "txpool/0.1"
	/* FINISH ALPHA CHANGE 3007 -- expose the existing admission size policy without changing its allowance -- */

	// maximum allowed number of times an account
	// was excluded from block building (ibft.writeTransactions)
	maxAccountDemotions uint64 = 10

	// maximum allowed number of consecutive blocks that don't have the account's transaction
	maxAccountSkips = uint64(10)

	pruningCooldown = 5000 * time.Millisecond

	/* START ALPHA CHANGE 1936 -- require a meaningful tx replacement price bump -- */
	replacementBumpPercent int64 = 10
	/* FINISH ALPHA CHANGE 1936 -- require a meaningful tx replacement price bump -- */

	// txPoolMetrics is a prefix used for txpool-related metrics
	txPoolMetrics = "txpool"
)

// errors
var (
	ErrIntrinsicGas            = errors.New("intrinsic gas too low")
	ErrBlockLimitExceeded      = errors.New("exceeds block gas limit")
	ErrNegativeValue           = errors.New("negative value")
	ErrExtractSignature        = errors.New("cannot extract signature")
	ErrInvalidSender           = errors.New("invalid sender")
	ErrTxPoolOverflow          = errors.New("txpool is full")
	ErrUnderpriced             = errors.New("transaction underpriced")
	ErrNonceTooLow             = errors.New("nonce too low")
	ErrInsufficientFunds       = errors.New("insufficient funds for gas * price + value")
	ErrInvalidAccountState     = errors.New("invalid account state")
	ErrAlreadyKnown            = errors.New("already known")
	ErrOversizedData           = errors.New("oversized data")
	ErrMaxEnqueuedLimitReached = errors.New("maximum number of enqueued transactions reached")
	ErrRejectFutureTx          = errors.New("rejected future tx due to low slots")
	ErrInvalidTxType           = errors.New("invalid tx type")
	ErrTxTypeNotSupported      = types.ErrTxTypeNotSupported
	ErrTipAboveFeeCap          = errors.New("max priority fee per gas higher than max fee per gas")
	ErrTipVeryHigh             = errors.New("max priority fee per gas higher than 2^256-1")
	ErrFeeCapVeryHigh          = errors.New("max fee per gas higher than 2^256-1")
	ErrNonceExistsInPool       = errors.New("tx with the same nonce is already present")
	ErrReplacementUnderpriced  = errors.New("replacement tx underpriced")
	ErrDynamicTxNotAllowed     = errors.New("dynamic tx not allowed currently")
)

// indicates origin of a transaction
type txOrigin int

const (
	local  txOrigin = iota // json-RPC/gRPC endpoints
	gossip                 // gossip protocol
)

func (o txOrigin) String() (s string) {
	switch o {
	case local:
		s = "local"
	case gossip:
		s = "gossip"
	}

	return
}

// store interface defines State helper methods the TxPool should have access to
type store interface {
	Header() *types.Header
	/* START ALPHA CHANGE 2164 -- preserve state nonce read failures -- */
	GetNonce(root types.Hash, addr types.Address) (uint64, error)
	/* FINISH ALPHA CHANGE 2164 -- preserve state nonce read failures -- */
	GetBalance(root types.Hash, addr types.Address) (*big.Int, error)
	GetBlockByHash(types.Hash, bool) (*types.Block, bool)
	CalculateBaseFee(parent *types.Header) (uint64, error)
}

type signer interface {
	Sender(tx *types.Transaction) (types.Address, error)
}

type Config struct {
	PriceLimit         uint64
	MaxSlots           uint64
	MaxAccountEnqueued uint64
	ChainID            *big.Int
}

/* All requests are passed to the main loop
through their designated channels. */

// START ALPHA CHANGE 1024 -- remove unused txpool enqueue request type --
// FINISH ALPHA CHANGE 1024 -- remove unused txpool enqueue request type --

// A promoteRequest is created each time some account
// is eligible for promotion. This request is signaled
// on 2 occasions:
//
// 1. When an enqueued transaction's nonce is
// not greater than the expected (account's nextNonce).
// == nextNonce - transaction is expected (addTx)
// < nextNonce - transaction was demoted (Demote)
//
// 2. When an account's nextNonce is updated (during ResetWithHeader)
// and the first enqueued transaction matches the new nonce.
type promoteRequest struct {
	account types.Address
}

// TxPool is a module that handles pending transactions.
// All transactions are handled within their respective accounts.
// An account contains 2 queues a transaction needs to go through:
// - 1. Enqueued (entry point)
// - 2. Promoted (exit point)
// (both queues are min nonce ordered)
//
// When consensus needs to process promoted transactions,
// the pool generates a queue of "executable" transactions. These
// transactions are the first-in-line of some promoted queue,
// ready to be written to the state (primaries).
type TxPool struct {
	logger hclog.Logger
	signer signer
	forks  *chain.Forks
	store  store

	// map of all accounts registered by the pool
	accounts accountsMap

	// START ALPHA CHANGE 2340 -- track proposal selection without consuming shared queues --
	// These fields belong to the single proposal-selection consumer.
	// Prepare starts a new selection; admissions do not access them.
	executables    *pricedQueue
	proposalNonces map[types.Address]uint64
	// FINISH ALPHA CHANGE 2340 -- track proposal selection without consuming shared queues --

	// lookup map keeping track of all
	// transactions present in the pool
	index lookupMap

	// networking stack
	topic *network.Topic

	// gauge for measuring pool capacity
	gauge slotGauge

	// priceLimit is a lower threshold for gas price
	priceLimit uint64

	// channels on which the pool's event loop
	// does dispatching/handling requests.
	promoteReqCh chan promoteRequest
	pruneCh      chan struct{}

	// shutdown channel
	shutdownCh chan struct{}

	/* START ALPHA CHANGE 1064 -- make txpool shutdown idempotent -- */
	closeOnce sync.Once

	/* START ALPHA CHANGE 1392 -- gate and wait for txpool runtime work -- */
	closeLock sync.Mutex
	closeWG   sync.WaitGroup
	closing   atomic.Bool
	/* FINISH ALPHA CHANGE 1392 -- gate and wait for txpool runtime work -- */
	/* FINISH ALPHA CHANGE 1064 -- make txpool shutdown idempotent -- */

	// flag indicating if the current node is a sealer,
	// and should therefore gossip transactions
	sealing atomic.Bool

	// baseFee is the last successfully calculated fee.
	// This is needed to sort transactions by price.
	baseFee uint64

	/* START ALPHA CHANGE 2191 -- retain failed base-fee refreshes for transaction admission -- */
	baseFeeMu  sync.RWMutex
	baseFeeErr error
	/* FINISH ALPHA CHANGE 2191 -- retain failed base-fee refreshes for transaction admission -- */

	/* START ALPHA CHANGE 2196 -- retain incomplete pool reconciliation independently of fee refreshes -- */
	// resetMu serializes resets and protects resetErr/resetRetryAt.
	// Admission and pending-nonce queries hold its read lock while
	// consuming reconciled pool data.
	//
	// Lock order: resetMu precedes account locks and baseFeeMu.
	resetMu      sync.RWMutex
	resetErr     error
	resetRetryAt time.Time
	/* FINISH ALPHA CHANGE 2196 -- retain incomplete pool reconciliation independently of fee refreshes -- */

	// Event manager for txpool events
	eventManager *eventManager

	// indicates which txpool operator commands should be implemented
	proto.UnimplementedTxnPoolOperatorServer

	// pending is the list of pending and ready transactions. This variable
	// is accessed with atomics
	pending int64

	// chain id
	chainID *big.Int
}

// NewTxPool returns a new pool for processing incoming transactions.
func NewTxPool(
	logger hclog.Logger,
	forks *chain.Forks,
	store store,
	grpcServer *grpc.Server,
	network *network.Server,
	config *Config,
) (*TxPool, error) {
	pool := &TxPool{
		logger:      logger.Named("txpool"),
		forks:       forks,
		store:       store,
		executables: newPricesQueue(0, nil),
		accounts:    accountsMap{maxEnqueuedLimit: config.MaxAccountEnqueued},
		index:       lookupMap{all: make(map[types.Hash]*types.Transaction)},
		gauge:       slotGauge{height: 0, max: config.MaxSlots},
		priceLimit:  config.PriceLimit,
		chainID:     config.ChainID,

		//	main loop channels
		promoteReqCh: make(chan promoteRequest),
		pruneCh:      make(chan struct{}),
		shutdownCh:   make(chan struct{}),
	}

	// Attach the event manager
	pool.eventManager = newEventManager(pool.logger)

	if network != nil {
		// subscribe to the gossip protocol
		topic, err := network.NewTopic(topicNameV1, &proto.Txn{})
		if err != nil {
			return nil, err
		}

		if subscribeErr := topic.Subscribe(pool.addGossipTx); subscribeErr != nil {
			return nil, fmt.Errorf("unable to subscribe to gossip topic, %w", subscribeErr)
		}

		pool.topic = topic
	}

	if grpcServer != nil {
		proto.RegisterTxnPoolOperatorServer(grpcServer, pool)
	}

	return pool, nil
}

func (p *TxPool) updatePending(i int64) {
	newPending := atomic.AddInt64(&p.pending, i)
	metrics.SetGauge([]string{txPoolMetrics, "pending_transactions"}, float32(newPending))
}

/* START ALPHA CHANGE 1393 -- register txpool runtime work safely -- */
func (p *TxPool) enterRuntimeWork() bool {
	p.closeLock.Lock()
	defer p.closeLock.Unlock()

	if p.closing.Load() {
		return false
	}

	p.closeWG.Add(1)

	return true
}

func (p *TxPool) leaveRuntimeWork() {
	p.closeWG.Done()
}

/* FINISH ALPHA CHANGE 1393 -- register txpool runtime work safely -- */

// Start runs the pool's main loop in the background.
// On each request received, the appropriate handler
// is invoked in a separate goroutine.
func (p *TxPool) Start() {
	// set default value of txpool pending transactions gauge
	p.updatePending(0)

	//	run the handler for high gauge level pruning
	/* START ALPHA CHANGE 1394 -- track txpool pruning loop shutdown completion -- */
	if p.enterRuntimeWork() {
		go func() {
			defer p.leaveRuntimeWork()

			for {
				select {
				case <-p.shutdownCh:
					return
				case <-p.pruneCh:
					p.pruneAccountsWithNonceHoles()
				}

				timer := time.NewTimer(pruningCooldown)
				select {
				case <-p.shutdownCh:
					timer.Stop()

					return
				case <-timer.C:
				}
			}
		}()
	}
	/* FINISH ALPHA CHANGE 1394 -- track txpool pruning loop shutdown completion -- */

	//	run the handler for the tx pipeline
	/* START ALPHA CHANGE 1395 -- track txpool promote loop and workers -- */
	if p.enterRuntimeWork() {
		go func() {
			defer p.leaveRuntimeWork()

			for {
				select {
				case <-p.shutdownCh:
					return
				case req := <-p.promoteReqCh:
					if !p.enterRuntimeWork() {
						return
					}

					go func(req promoteRequest) {
						defer p.leaveRuntimeWork()

						p.handlePromoteRequest(req)
					}(req)
				}
			}
		}()
	}
	/* FINISH ALPHA CHANGE 1395 -- track txpool promote loop and workers -- */
}

// Close shuts down the pool's main loop.
func (p *TxPool) Close() {
	/* START ALPHA CHANGE 1065 -- close txpool channels only once -- */
	p.closeOnce.Do(func() {
		/* START ALPHA CHANGE 1396 -- stop txpool ingress before closing topic and event manager -- */
		p.closeLock.Lock()
		p.closing.Store(true)
		close(p.shutdownCh)
		p.closeLock.Unlock()

		p.sealing.Store(false)

		p.closeWG.Wait()

		if p.topic != nil {
			p.topic.Close()
		}

		p.eventManager.Close()
		/* FINISH ALPHA CHANGE 1396 -- stop txpool ingress before closing topic and event manager -- */
	})
	/* FINISH ALPHA CHANGE 1065 -- close txpool channels only once -- */
}

// SetSigner sets the signer the pool will use
// to validate a transaction's signature.
func (p *TxPool) SetSigner(s signer) {
	p.signer = s
}

// SetSealing sets the sealing flag
func (p *TxPool) SetSealing(sealing bool) {
	p.sealing.CompareAndSwap(p.sealing.Load(), sealing)
}

// AddTx adds a new transaction to the pool (sent from json-RPC/gRPC endpoints)
// and broadcasts it to the network (if enabled).
func (p *TxPool) AddTx(tx *types.Transaction) error {
	/* START ALPHA CHANGE 1397 -- reject local txpool ingress during shutdown -- */
	if !p.enterRuntimeWork() {
		return errors.New("txpool is shutting down")
	}
	defer p.leaveRuntimeWork()
	/* FINISH ALPHA CHANGE 1397 -- reject local txpool ingress during shutdown -- */

	if err := p.addTx(local, tx); err != nil {
		/* START ALPHA CHANGE 1942 -- return routine txpool rejections without noisy local logs -- */
		/* START ALPHA CHANGE 2173 -- bound local transaction admission state failure logs -- */
		if !state.LogLocalStateFailure(p.logger, state.LocalStateLogTxPool, err) &&
			!errors.Is(err, ErrAlreadyKnown) &&
			!errors.Is(err, ErrReplacementUnderpriced) &&
			!errors.Is(err, ErrUnderpriced) {
			p.logger.Error("failed to add tx", "err", err)
		}
		/* FINISH ALPHA CHANGE 2173 -- bound local transaction admission state failure logs -- */
		/* FINISH ALPHA CHANGE 1942 -- return routine txpool rejections without noisy local logs -- */

		return err
	}

	// broadcast the transaction only if a topic
	// subscription is present
	if p.topic != nil {
		tx := &proto.Txn{
			Raw: &any.Any{
				Value: tx.MarshalRLP(),
			},
		}

		if err := p.topic.Publish(tx); err != nil {
			p.logger.Error("failed to topic tx", "err", err)
		}
	}

	return nil
}

// START ALPHA CHANGE 2341 -- start each proposal from the current shared pool --
// Prepare starts a new proposal selection from the promoted queue heads.
// Transactions selected by earlier proposals remain available until reset
// confirms their nonces or an ordinary pool rejection removes them.
func (p *TxPool) Prepare() {
	primaries := p.accounts.getPrimaries()

	p.proposalNonces = make(map[types.Address]uint64)
	p.executables = newPricesQueue(p.GetBaseFee(), primaries)
}

// FINISH ALPHA CHANGE 2341 -- start each proposal from the current shared pool --

// Peek returns the best-price selected
// transaction ready for execution.
func (p *TxPool) Peek() *types.Transaction {
	// Popping the executables queue
	// does not remove the actual tx
	// from the pool.
	// The executables queue just provides
	// insight into which account has the
	// highest priced tx (head of promoted queue)
	return p.executables.pop()
}

// START ALPHA CHANGE 2342 -- advance proposal selection without removing unconfirmed transactions --
// Pop marks this nonce as executed in the current proposal and offers the
// sender's next promoted transaction to the proposal's price queue.
//
// Shared queues, indexes, pending counts and capacity remain unchanged.
// A failed or abandoned proposal therefore needs no restoration step.
// A concurrent replacement stays in the pool for the next proposal.
func (p *TxPool) Pop(tx *types.Transaction) {
	if last, exists := p.proposalNonces[tx.From]; exists && tx.Nonce <= last {
		return
	}

	account := p.accounts.get(tx.From)
	if account == nil {
		return
	}

	account.promoted.lock(false)
	account.nonceToTx.lock()

	defer func() {
		account.nonceToTx.unlock()
		account.promoted.unlock()
	}()

	// Admission may have replaced the selected hash. The nonce must
	// still belong to the promoted part of this account.
	current := account.nonceToTx.get(tx.Nonce)
	if current == nil || tx.Nonce >= account.getNonce() {
		return
	}

	if p.proposalNonces == nil {
		p.proposalNonces = make(map[types.Address]uint64)
	}
	p.proposalNonces[tx.From] = tx.Nonce
	account.resetDemotions()

	if tx.Nonce == ^uint64(0) {
		return
	}

	nextNonce := tx.Nonce + 1
	if nextNonce >= account.getNonce() {
		return
	}

	if next := account.nonceToTx.get(nextNonce); next != nil {
		p.executables.push(next)
	}
}

// FINISH ALPHA CHANGE 2342 -- advance proposal selection without removing unconfirmed transactions --

// START ALPHA CHANGE 2344 -- preserve successful proposal prefixes during ordinary transaction rejection --
// Drop removes the rejected nonce and its successors.
// Earlier transactions remain available if the proposal is abandoned.
func (p *TxPool) Drop(tx *types.Transaction) {
	account := p.accounts.get(tx.From)
	p.dropAccount(account, tx.Nonce, tx, true)
}

// dropAccount updates queue ownership, counters and the expected nonce.
// keepLowerNonces preserves the successful prefix during proposal handling.
// Maintenance cleanup passes false to retain its full-account drop behavior.
func (p *TxPool) dropAccount(
	account *account,
	nextNonce uint64,
	tx *types.Transaction,
	keepLowerNonces bool,
) {
	if account == nil || !account.lockActive() {
		return
	}

	defer func() {
		p.retireIdleAccountLocked(tx.From, account)
		account.unlockActive()
	}()

	droppedCount := 0

	clearAccountQueue := func(txs []*types.Transaction) {
		account.nonceToTx.remove(txs...)
		p.index.remove(txs...)
		p.gauge.decrease(slotsRequired(txs...))
		droppedCount += len(txs)
	}

	removeQueue := func(queue *accountQueue) []*types.Transaction {
		if keepLowerNonces {
			return queue.removeFrom(nextNonce)
		}

		return queue.clear()
	}

	account.setNonce(nextNonce)

	dropped := removeQueue(account.promoted)
	clearAccountQueue(dropped)
	p.updatePending(-1 * int64(len(dropped)))

	dropped = removeQueue(account.enqueued)
	clearAccountQueue(dropped)

	p.eventManager.signalEvent(proto.EventType_DROPPED, tx.Hash)

	if p.logger.IsDebug() {
		p.logger.Debug("dropped account txs",
			"num", droppedCount,
			"next_nonce", nextNonce,
			"address", tx.From.String(),
		)
	}
}

// FINISH ALPHA CHANGE 2344 -- preserve successful proposal prefixes during ordinary transaction rejection --

// Demote excludes an account from being further processed during block building
// due to a recoverable error. If an account has been demoted too many times (maxAccountDemotions),
// it is Dropped instead.
func (p *TxPool) Demote(tx *types.Transaction) {
	account := p.accounts.get(tx.From)
	if account.Demotions() >= maxAccountDemotions {
		if p.logger.IsDebug() {
			p.logger.Debug(
				"Demote: threshold reached - dropping account",
				"addr", tx.From.String(),
			)
		}

		p.Drop(tx)

		// reset the demotions counter
		account.resetDemotions()

		return
	}

	account.incrementDemotions()

	p.eventManager.signalEvent(proto.EventType_DEMOTED, tx.Hash)
}

// ResetWithHeaders processes the transactions from the new
// headers to sync the pool with the new state.
func (p *TxPool) ResetWithHeaders(headers ...*types.Header) {
	/* START ALPHA CHANGE 1398 -- skip txpool reset work during shutdown -- */
	if !p.enterRuntimeWork() {
		return
	}
	defer p.leaveRuntimeWork()
	/* FINISH ALPHA CHANGE 1398 -- skip txpool reset work during shutdown -- */

	// process the txs in the event
	// to make sure the pool is up-to-date
	p.processEvent(&blockchain.Event{
		NewChain: headers,
	})
}

// processEvent collects the latest nonces for each account contained
// in the received event. Resets all known accounts with the new nonce.
func (p *TxPool) processEvent(event *blockchain.Event) {
	/* START ALPHA CHANGE 2198 -- reconcile missed resets before resuming incremental maintenance -- */
	p.resetMu.Lock()
	defer p.resetMu.Unlock()

	if p.resetErr != nil {
		// A full reconciliation uses current state and covers this event
		// as well as accounts omitted from subsequent block headers.
		// A throttled or unsuccessful attempt leaves resetErr retained.
		_ = p.retryResetLocked()

		return
	}
	/* FINISH ALPHA CHANGE 2198 -- reconcile missed resets before resuming incremental maintenance -- */

	// Grab the latest state root now that the block has been inserted
	/* START ALPHA CHANGE 2164 -- stage mined lookup removals until event nonce reads succeed -- */
	stateRoot := p.store.Header().StateRoot
	stateNonces := make(map[types.Address]uint64)
	var minedTransactions []*types.Transaction
	/* FINISH ALPHA CHANGE 2164 -- stage mined lookup removals until event nonce reads succeed -- */

	// discover latest (next) nonces for all accounts
	for _, header := range event.NewChain {
		// START ALPHA CHANGE 2346 -- recover incomplete resets after an unavailable finalized block --
		block, ok := p.store.GetBlockByHash(header.Hash, true)
		if !ok || block == nil {
			err := state.WrapLocalStateFailure(
				"reset transaction pool",
				fmt.Errorf("finalized block %s is unavailable", header.Hash),
			)
			p.recordResetFailureLocked(err)
			state.LogLocalStateFailure(p.logger, state.LocalStateLogTxPool, err)

			return
		}
		// FINISH ALPHA CHANGE 2346 -- recover incomplete resets after an unavailable finalized block --

		/* START ALPHA CHANGE 2164 -- defer mined lookup removal until state reads succeed -- */
		minedTransactions = append(minedTransactions, block.Transactions...)
		/* FINISH ALPHA CHANGE 2164 -- defer mined lookup removal until state reads succeed -- */

		// Extract latest nonces
		for _, tx := range block.Transactions {
			var err error

			addr := tx.From
			if addr == types.ZeroAddress {
				// From field is not set, extract the signer
				if addr, err = p.signer.Sender(tx); err != nil {
					p.logger.Error(
						fmt.Sprintf("unable to extract signer for transaction, %v", err),
					)

					continue
				}
			}

			// skip already processed accounts
			if _, processed := stateNonces[addr]; processed {
				continue
			}

			/* START ALPHA CHANGE 2164 -- abort pool reset when a required nonce cannot be read -- */
			latestNonce, err := p.store.GetNonce(stateRoot, addr)
			if err != nil {
				/* START ALPHA CHANGE 2173 -- bound txpool reset state failure logs -- */
				state.LogLocalStateFailure(
					p.logger,
					state.LocalStateLogTxPool,
					state.WrapLocalStateFailure(
						fmt.Sprintf("read reset nonce for %s", addr),
						err,
					),
				)
				/* FINISH ALPHA CHANGE 2173 -- bound txpool reset state failure logs -- */

				/* START ALPHA CHANGE 2198 -- retain missed account reconciliation after nonce failure -- */
				p.recordResetFailureLocked(state.WrapLocalStateFailure(
					fmt.Sprintf("read reset nonce for %s", addr),
					err,
				))
				/* FINISH ALPHA CHANGE 2198 -- retain missed account reconciliation after nonce failure -- */

				return
			}

			stateNonces[addr] = latestNonce
			/* FINISH ALPHA CHANGE 2164 -- abort pool reset when a required nonce cannot be read -- */
		}
	}

	/* START ALPHA CHANGE 2192 -- require fee refresh before publishing pool reset changes -- */
	if ln := len(event.NewChain); ln > 0 {
		if err := p.SetBaseFee(event.NewChain[ln-1]); err != nil {
			/* START ALPHA CHANGE 2198 -- retain the missed reset independently of the fee error -- */
			// SetBaseFee reports the failure. Retain the incomplete
			// account/index reconciliation until it succeeds too.
			p.recordResetFailureLocked(err)
			/* FINISH ALPHA CHANGE 2198 -- retain the missed reset independently of the fee error -- */

			return
		}
	}

	p.index.remove(minedTransactions...)
	/* FINISH ALPHA CHANGE 2192 -- require fee refresh before publishing pool reset changes -- */

	// reset accounts with the new state
	p.resetAccounts(stateNonces)

	if !p.sealing.Load() {
		// only non-validator cleanup inactive accounts
		p.updateAccountSkipsCounts(stateNonces)
	}
}

// validateTx ensures the transaction conforms to specific
// constraints before entering the pool.
/* START ALPHA CHANGE 1937 -- permit authenticated pre-state rejection without repeating signature recovery -- */
func (p *TxPool) validateTx(tx *types.Transaction) error {
	return p.validateTxWithPreStateCheck(tx, nil)
}

func (p *TxPool) validateTxWithPreStateCheck(
	tx *types.Transaction,
	preStateCheck func(uint64) error,
) error {
	/* FINISH ALPHA CHANGE 1937 -- permit authenticated pre-state rejection without repeating signature recovery -- */
	// Check the transaction type. State transactions are not expected to be added to the pool
	if tx.Type == types.StateTx {
		metrics.IncrCounter([]string{txPoolMetrics, "invalid_tx_type"}, 1)

		return fmt.Errorf("%w: type %d rejected, state transactions are not expected to be added to the pool",
			ErrInvalidTxType, tx.Type)
	}

	/* START ALPHA CHANGE 2929 -- reject invalid chain IDs before pool serialization and recovery -- */
	if err := types.ValidateDynamicFeeChainID(tx, p.chainID); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2929 -- reject invalid chain IDs before pool serialization and recovery -- */

	/* START ALPHA CHANGE 3895 -- reject terminal nonces before pool admission and promotion -- */
	if tx.Nonce == ^uint64(0) {
		return state.ErrNonceUintOverflow
	}
	/* FINISH ALPHA CHANGE 3895 -- reject terminal nonces before pool admission and promotion -- */

	// Check the transaction size to overcome DOS Attacks
	if uint64(len(tx.MarshalRLP())) > txMaxSize {
		metrics.IncrCounter([]string{txPoolMetrics, "oversized_data_txs"}, 1)

		return ErrOversizedData
	}

	// Check if the transaction has a strictly positive value
	if tx.Value.Sign() < 0 {
		metrics.IncrCounter([]string{txPoolMetrics, "negative_value_tx"}, 1)

		return ErrNegativeValue
	}

	// Check if the transaction is signed properly

	// Extract the sender
	from, signerErr := p.signer.Sender(tx)
	if signerErr != nil {
		metrics.IncrCounter([]string{txPoolMetrics, "invalid_signature_txs"}, 1)

		return ErrExtractSignature
	}

	// If the from field is set, check that
	// it matches the signer
	if tx.From != types.ZeroAddress &&
		tx.From != from {
		metrics.IncrCounter([]string{txPoolMetrics, "invalid_sender_txs"}, 1)

		return ErrInvalidSender
	}

	// If no address was set, update it
	if tx.From == types.ZeroAddress {
		tx.From = from
	}

	// Grab current block number
	currentHeader := p.store.Header()
	currentBlockNumber := currentHeader.Number

	// Get forks state for the current block
	forks := p.forks.At(currentBlockNumber)

	// Check if transaction can deploy smart contract
	if tx.IsContractCreation() && forks.EIP158 && len(tx.Input) > state.TxPoolMaxInitCodeSize {
		metrics.IncrCounter([]string{txPoolMetrics, "contract_deploy_too_large_txs"}, 1)

		return runtime.ErrMaxCodeSizeExceeded
	}

	// Grab the state root, and block gas limit for the latest block
	stateRoot := currentHeader.StateRoot
	latestBlockGasLimit := currentHeader.GasLimit

	/* START ALPHA CHANGE 2191 -- do not classify transaction prices using a failed fee refresh -- */
	baseFee, baseFeeErr := p.admissionBaseFee()
	if baseFeeErr != nil {
		return baseFeeErr
	}
	/* FINISH ALPHA CHANGE 2191 -- do not classify transaction prices using a failed fee refresh -- */

	if tx.Type == types.DynamicFeeTx {
		// Reject dynamic fee tx if london hardfork is not enabled
		if !forks.London {
			metrics.IncrCounter([]string{txPoolMetrics, "tx_type"}, 1)

			return fmt.Errorf("%w: type %d rejected, london hardfork is not enabled", ErrTxTypeNotSupported, tx.Type)
		}

		// DynamicFeeTx should be rejected if TxHashWithType fork is registered but not enabled for current block
		blockNumber, err := forkmanager.GetInstance().GetForkBlock(chain.TxHashWithType)
		if err == nil && blockNumber > currentBlockNumber {
			metrics.IncrCounter([]string{txPoolMetrics, "dynamic_tx_not_allowed"}, 1)

			return ErrDynamicTxNotAllowed
		}

		// Check EIP-1559-related fields and make sure they are correct
		if tx.GasFeeCap == nil || tx.GasTipCap == nil {
			metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

			return ErrUnderpriced
		}

		if tx.GasFeeCap.BitLen() > 256 {
			metrics.IncrCounter([]string{txPoolMetrics, "fee_cap_too_high_dynamic_tx"}, 1)

			return ErrFeeCapVeryHigh
		}

		if tx.GasTipCap.BitLen() > 256 {
			metrics.IncrCounter([]string{txPoolMetrics, "tip_too_high_dynamic_tx"}, 1)

			return ErrTipVeryHigh
		}

		if tx.GasFeeCap.Cmp(tx.GasTipCap) < 0 {
			metrics.IncrCounter([]string{txPoolMetrics, "tip_above_fee_cap_dynamic_tx"}, 1)

			return ErrTipAboveFeeCap
		}

		// Reject underpriced transactions
		if tx.GasFeeCap.Cmp(new(big.Int).SetUint64(baseFee)) < 0 {
			metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

			return ErrUnderpriced
		}
	} else {
		// Legacy approach to check if the given tx is not underpriced when london hardfork is enabled
		if forks.London && tx.GasPrice.Cmp(new(big.Int).SetUint64(baseFee)) < 0 {
			metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

			return ErrUnderpriced
		}
	}

	// Check if the given tx is not underpriced
	if tx.GetGasPrice(baseFee).Cmp(new(big.Int).SetUint64(p.priceLimit)) < 0 {
		metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

		return ErrUnderpriced
	}

	/* START ALPHA CHANGE 1938 -- reject insufficient authenticated replacements before state work -- */
	if preStateCheck != nil {
		if err := preStateCheck(currentBlockNumber); err != nil {
			return err
		}
	}
	/* FINISH ALPHA CHANGE 1938 -- reject insufficient authenticated replacements before state work -- */

	/* START ALPHA CHANGE 2164 -- do not attribute unavailable account state to a transaction -- */
	stateNonce, nonceErr := p.store.GetNonce(stateRoot, tx.From)
	if nonceErr != nil {
		return state.WrapLocalStateFailure("read txpool admission nonce", nonceErr)
	}

	// Check nonce ordering only after a successful read.
	if stateNonce > tx.Nonce {
		metrics.IncrCounter([]string{txPoolMetrics, "nonce_too_low_tx"}, 1)

		return ErrNonceTooLow
	}

	accountBalance, balanceErr := p.store.GetBalance(stateRoot, tx.From)
	if balanceErr != nil {
		return state.WrapLocalStateFailure("read txpool admission balance", balanceErr)
	}
	/* FINISH ALPHA CHANGE 2164 -- do not attribute unavailable account state to a transaction -- */

	// Check if the sender has enough funds to execute the transaction
	if accountBalance.Cmp(tx.Cost()) < 0 {
		metrics.IncrCounter([]string{txPoolMetrics, "insufficient_funds_tx"}, 1)

		return ErrInsufficientFunds
	}

	// Make sure the transaction has more gas than the basic transaction fee
	intrinsicGas, err := state.TransactionGasCost(tx, forks.Homestead, forks.Istanbul)
	if err != nil {
		metrics.IncrCounter([]string{txPoolMetrics, "invalid_intrinsic_gas_tx"}, 1)

		return err
	}

	if tx.Gas < intrinsicGas {
		metrics.IncrCounter([]string{txPoolMetrics, "intrinsic_gas_low_tx"}, 1)

		return ErrIntrinsicGas
	}

	if tx.Gas > latestBlockGasLimit {
		metrics.IncrCounter([]string{txPoolMetrics, "block_gas_limit_exceeded_tx"}, 1)

		return ErrBlockLimitExceeded
	}

	return nil
}

/* START ALPHA CHANGE 1939 -- enforce rounded replacement bumps and authenticated early rejection -- */
// A replacement must increase the old price by at least 10%, rounded up,
// and by at least one wei. big.Int keeps the calculation overflow-safe.
func replacementPriceHasBump(oldPrice, newPrice *big.Int) bool {
	bump := new(big.Int).Mul(oldPrice, big.NewInt(replacementBumpPercent))
	bump.Add(bump, big.NewInt(99))
	bump.Div(bump, big.NewInt(100))

	if bump.Sign() == 0 {
		bump.SetInt64(1)
	}

	requiredPrice := new(big.Int).Add(oldPrice, bump)

	return newPrice.Cmp(requiredPrice) >= 0
}

func replacementPriceValid(oldTx, newTx *types.Transaction, baseFee uint64) bool {
	if oldTx.Type == types.DynamicFeeTx && newTx.Type == types.DynamicFeeTx {
		return replacementPriceHasBump(oldTx.GasFeeCap, newTx.GasFeeCap) &&
			replacementPriceHasBump(oldTx.GasTipCap, newTx.GasTipCap)
	}

	if oldTx.Type == types.LegacyTx && newTx.Type == types.LegacyTx {
		return replacementPriceHasBump(oldTx.GasPrice, newTx.GasPrice)
	}

	// Preserve cross-type replacement support using effective gas prices.
	return replacementPriceHasBump(
		oldTx.GetGasPrice(baseFee),
		newTx.GetGasPrice(baseFee),
	)
}

// This check only rejects. It never admits or replaces a transaction.
// Cross-type prices depend on the changing base fee, so their authoritative
// comparison is deferred to the final locked insertion check.
func (p *TxPool) rejectEarlyReplacement(tx *types.Transaction) error {
	account := p.accounts.get(tx.From)
	if account == nil {
		return nil
	}

	account.nonceToTx.lock()
	defer account.nonceToTx.unlock()

	oldTx := account.nonceToTx.get(tx.Nonce)
	if oldTx == nil {
		return nil
	}

	if oldTx.Hash == tx.Hash {
		metrics.IncrCounter([]string{txPoolMetrics, "already_known_tx"}, 1)

		return ErrAlreadyKnown
	}

	if oldTx.Type != tx.Type {
		return nil
	}

	if !replacementPriceValid(oldTx, tx, 0) {
		metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

		return ErrReplacementUnderpriced
	}

	return nil
}

/* FINISH ALPHA CHANGE 1939 -- enforce rounded replacement bumps and authenticated early rejection -- */

func (p *TxPool) signalPruning() {
	select {
	case p.pruneCh <- struct{}{}:
	default: //	pruning handler is active or in cooldown
	}
}

func (p *TxPool) pruneAccountsWithNonceHoles() {
	/* START ALPHA CHANGE 2198 -- preserve queued transactions while nonce reconciliation is incomplete -- */
	if err := p.lockReconciled(); err != nil {
		// The failed reset or retry already reported the error through
		// the existing logging policy.
		return
	}
	defer p.resetMu.RUnlock()
	/* FINISH ALPHA CHANGE 2198 -- preserve queued transactions while nonce reconciliation is incomplete -- */

	// START ALPHA CHANGE 2310 -- prune and retire under the account mutation locks --
	p.accounts.Range(
		func(key, value interface{}) bool {
			address := key.(types.Address)
			account := value.(*account)

			if !account.lockActive() {
				return true
			}

			defer func() {
				p.retireIdleAccountLocked(address, account)
				account.unlockActive()
			}()

			firstTx := account.enqueued.peek()
			if firstTx == nil {
				return true
			}

			if firstTx.Nonce == account.getNonce() {
				return true
			}

			removed := account.enqueued.clear()

			account.nonceToTx.remove(removed...)
			p.index.remove(removed...)
			p.gauge.decrease(slotsRequired(removed...))

			return true
		},
	)
	// FINISH ALPHA CHANGE 2310 -- prune and retire under the account mutation locks --
}

// addTx is the main entry point to the pool
// for all new transactions. If the call is
// successful, an account is created for this address
// once and the transaction is added to the account queue.
// START ALPHA CHANGE 1024 -- update txpool addTx comment after enqueue request removal --
// FINISH ALPHA CHANGE 1024 -- update txpool addTx comment after enqueue request removal --
func (p *TxPool) addTx(origin txOrigin, tx *types.Transaction) error {
	/* START ALPHA CHANGE 2198 -- prevent admission against an incomplete pool reset -- */
	if err := p.lockReconciled(); err != nil {
		return err
	}
	defer p.resetMu.RUnlock()
	/* FINISH ALPHA CHANGE 2198 -- prevent admission against an incomplete pool reset -- */

	if p.logger.IsDebug() {
		p.logger.Debug("add tx", "origin", origin.String(), "hash", tx.Hash.String())
	}

	/* START ALPHA CHANGE 1940 -- hash and reject weak replacements before state validation -- */
	// The callback runs only after signature recovery and fee-field validation.
	// Passing it does not allow a transaction into the pool.
	if err := p.validateTxWithPreStateCheck(tx, func(currentBlockNumber uint64) error {
		/* START ALPHA CHANGE 2930 -- preserve the submitted chain ID during transaction hashing -- */
		tx.ComputeHash(currentBlockNumber)
		/* FINISH ALPHA CHANGE 2930 -- preserve the submitted chain ID during transaction hashing -- */

		return p.rejectEarlyReplacement(tx)
	}); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 1940 -- hash and reject weak replacements before state validation -- */

	// START ALPHA CHANGE 2308 -- retire empty admission records and retry detached pointers --
	account, err := p.lockAccountForAdmission(tx.From)
	if err != nil {
		return err
	}

	defer func() {
		// Successful admission retains queue/index ownership.
		// Failed admission must not leave an unused account behind.
		p.retireIdleAccountLocked(tx.From, account)
		account.unlockActive()
	}()
	// FINISH ALPHA CHANGE 2308 -- retire empty admission records and retry detached pointers --

	/* START ALPHA CHANGE 2191 -- recheck fee availability before publishing admission changes -- */
	p.baseFeeMu.RLock()
	defer p.baseFeeMu.RUnlock()

	if p.baseFeeErr != nil {
		return p.baseFeeErr
	}
	admissionBaseFee := p.GetBaseFee()
	/* FINISH ALPHA CHANGE 2191 -- recheck fee availability before publishing admission changes -- */

	accountNonce := account.getNonce()

	//	only accept transactions with expected nonce
	if p.gauge.highPressure() {
		p.signalPruning()

		if tx.Nonce > accountNonce {
			metrics.IncrCounter([]string{txPoolMetrics, "rejected_future_tx"}, 1)

			return ErrRejectFutureTx
		}
	}

	// try to find if there is transaction with same nonce for this account
	oldTxWithSameNonce := account.nonceToTx.get(tx.Nonce)
	if oldTxWithSameNonce != nil {
		if oldTxWithSameNonce.Hash == tx.Hash {
			metrics.IncrCounter([]string{txPoolMetrics, "already_known_tx"}, 1)

			return ErrAlreadyKnown
			/* START ALPHA CHANGE 1941 -- recheck the replacement bump under insertion locks -- */
			/* START ALPHA CHANGE 2191 -- use the successfully checked admission fee for replacement pricing -- */
		} else if !replacementPriceValid(oldTxWithSameNonce, tx, admissionBaseFee) {
			/* FINISH ALPHA CHANGE 2191 -- use the successfully checked admission fee for replacement pricing -- */
			metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

			return ErrReplacementUnderpriced
			/* FINISH ALPHA CHANGE 1941 -- recheck the replacement bump under insertion locks -- */
		}
	} else {
		/* START ALPHA CHANGE 3874 -- enforce the future transaction limit at and above capacity -- */
		if account.enqueued.length() >= account.maxEnqueued && tx.Nonce != accountNonce {
			return ErrMaxEnqueuedLimitReached
		}
		/* FINISH ALPHA CHANGE 3874 -- enforce the future transaction limit at and above capacity -- */

		// reject low nonce tx
		if tx.Nonce < accountNonce {
			metrics.IncrCounter([]string{txPoolMetrics, "nonce_too_low_tx"}, 1)

			return ErrNonceTooLow
		}
	}

	slotsAllocated := slotsRequired(tx)

	var slotsFreed uint64
	if oldTxWithSameNonce != nil {
		slotsFreed = slotsRequired(oldTxWithSameNonce)
	}

	var slotsIncreased uint64
	if slotsAllocated > slotsFreed {
		slotsIncreased = slotsAllocated - slotsFreed
		if !p.gauge.increaseWithinLimit(slotsIncreased) {
			return ErrTxPoolOverflow
		}
	}

	// add to index
	if ok := p.index.add(tx); !ok {
		metrics.IncrCounter([]string{txPoolMetrics, "already_known_tx"}, 1)

		if slotsIncreased > 0 {
			p.gauge.decrease(slotsIncreased)
		}

		return ErrAlreadyKnown
	}

	if slotsFreed > slotsAllocated {
		p.gauge.decrease(slotsFreed - slotsAllocated)
	}

	if oldTxWithSameNonce != nil {
		p.index.remove(oldTxWithSameNonce)
	} else {
		metrics.SetGauge([]string{txPoolMetrics, "added_tx"}, 1)
	}

	account.enqueue(tx, oldTxWithSameNonce != nil) // add or replace tx into account

	/* START ALPHA CHANGE 1399 -- track asynchronous txpool promotion notification -- */
	if p.enterRuntimeWork() {
		go func() {
			defer p.leaveRuntimeWork()

			p.invokePromotion(tx, tx.Nonce <= accountNonce) // don't signal promotion for higher nonce txs
		}()
	}
	/* FINISH ALPHA CHANGE 1399 -- track asynchronous txpool promotion notification -- */

	return nil
}

func (p *TxPool) invokePromotion(tx *types.Transaction, callPromote bool) {
	p.eventManager.signalEvent(proto.EventType_ADDED, tx.Hash)

	if p.logger.IsDebug() {
		p.logger.Debug("enqueue request", "hash", tx.Hash.String())
	}

	p.eventManager.signalEvent(proto.EventType_ENQUEUED, tx.Hash)

	if callPromote {
		select {
		case <-p.shutdownCh:
		case p.promoteReqCh <- promoteRequest{account: tx.From}: // BLOCKING
		}
	}
}

// handlePromoteRequest handles moving promotable transactions
// of some account from enqueued to promoted. Can only be
// invoked by handleEnqueueRequest or resetAccount.
func (p *TxPool) handlePromoteRequest(req promoteRequest) {
	addr := req.account
	account := p.accounts.get(addr)

	// START ALPHA CHANGE 2311 -- tolerate promotion requests for reclaimed accounts --
	if account == nil {
		return
	}
	// FINISH ALPHA CHANGE 2311 -- tolerate promotion requests for reclaimed accounts --

	// promote enqueued txs
	promoted, pruned := account.promote()
	if p.logger.IsDebug() {
		p.logger.Debug("promote request", "promoted", promoted, "addr", addr.String())
	}

	p.index.remove(pruned...)
	p.gauge.decrease(slotsRequired(pruned...))

	// update metrics
	p.updatePending(int64(len(promoted)))

	p.eventManager.signalEvent(proto.EventType_PROMOTED, toHash(promoted...)...)
}

// addGossipTx handles receiving transactions
// gossiped by the network.
func (p *TxPool) addGossipTx(obj interface{}, _ peer.ID) {
	/* START ALPHA CHANGE 1400 -- gate gossip txpool ingress during shutdown -- */
	if !p.enterRuntimeWork() {
		return
	}
	defer p.leaveRuntimeWork()
	/* FINISH ALPHA CHANGE 1400 -- gate gossip txpool ingress during shutdown -- */

	if !p.sealing.Load() {
		return
	}

	raw, ok := obj.(*proto.Txn)
	if !ok {
		p.logger.Error("failed to cast gossiped message to txn")

		return
	}

	// Verify that the gossiped transaction message is not empty
	if raw == nil || raw.Raw == nil {
		p.logger.Error("malformed gossip transaction message received")

		return
	}

	/* START ALPHA CHANGE 3008 -- enforce raw gossip size before transaction decoding -- */
	tx, err := DecodeRawTransaction(raw.Raw.Value)
	if err != nil {
		if !errors.Is(err, ErrOversizedData) {
			p.logger.Error("failed to decode broadcast tx", "err", err)
		}

		return
	}
	/* FINISH ALPHA CHANGE 3008 -- enforce raw gossip size before transaction decoding -- */

	// add tx
	if err := p.addTx(gossip, tx); err != nil {
		/* START ALPHA CHANGE 1943 -- keep expected replacement gossip rejection out of logs -- */
		if errors.Is(err, ErrAlreadyKnown) ||
			errors.Is(err, ErrReplacementUnderpriced) ||
			errors.Is(err, ErrUnderpriced) {
			return
		}
		/* FINISH ALPHA CHANGE 1943 -- keep expected replacement gossip rejection out of logs -- */

		/* START ALPHA CHANGE 2173 -- bound gossiped transaction state failure logs -- */
		if !state.LogLocalStateFailure(p.logger, state.LocalStateLogTxPool, err) {
			p.logger.Error("failed to add broadcast tx", "err", err, "hash", tx.Hash.String())
		}
		/* FINISH ALPHA CHANGE 2173 -- bound gossiped transaction state failure logs -- */
	}
}

// resetAccounts updates existing accounts with the new nonce and prunes stale transactions.
func (p *TxPool) resetAccounts(stateNonces map[types.Address]uint64) {
	if len(stateNonces) == 0 {
		return
	}

	var (
		allPrunedPromoted []*types.Transaction
		allPrunedEnqueued []*types.Transaction
	)

	// clear all accounts of stale txs
	for addr, newNonce := range stateNonces {
		account := p.accounts.get(addr)

		if account == nil {
			// no updates for this account
			continue
		}

		prunedPromoted, prunedEnqueued := account.reset(
			newNonce,
			p.promoteReqCh,
			p.shutdownCh,
		)

		// append pruned
		allPrunedPromoted = append(allPrunedPromoted, prunedPromoted...)
		allPrunedEnqueued = append(allPrunedEnqueued, prunedEnqueued...)

		// new state for account -> demotions are reset to 0
		account.resetDemotions()
	}

	// pool cleanup callback
	cleanup := func(stale []*types.Transaction) {
		p.index.remove(stale...)
		p.gauge.decrease(slotsRequired(stale...))
	}

	// prune pool state
	if len(allPrunedPromoted) > 0 {
		cleanup(allPrunedPromoted)

		p.eventManager.signalEvent(
			proto.EventType_PRUNED_PROMOTED,
			toHash(allPrunedPromoted...)...,
		)

		p.updatePending(int64(-1 * len(allPrunedPromoted)))
	}

	if len(allPrunedEnqueued) > 0 {
		cleanup(allPrunedEnqueued)

		p.eventManager.signalEvent(
			proto.EventType_PRUNED_ENQUEUED,
			toHash(allPrunedEnqueued...)...,
		)
	}

	// START ALPHA CHANGE 2335 -- clear consumed index-only transactions before account retirement --
	// START ALPHA CHANGE 2366 -- clarify finalized ownership cleanup before retirement --
	// Both incremental reset and missed-reset recovery reach this point
	// only after their required state reads and fee refresh succeed.
	//
	// Proposal selection retains queued transactions. Finalization may
	// consume a nonce using a different hash, so cleanup uses confirmed
	// account nonces rather than relying only on exact mined-hash matches.
	//
	// Queue cleanup above owns slot and pending-counter updates.
	// Clear any remaining consumed index entries before testing account
	// retirement, without subtracting those counters again.
	// FINISH ALPHA CHANGE 2366 -- clarify finalized ownership cleanup before retirement --
	p.index.removeBelowNonces(stateNonces)

	for address := range stateNonces {
		p.retireIdleAccount(address, p.accounts.get(address))
	}
	// FINISH ALPHA CHANGE 2335 -- clear consumed index-only transactions before account retirement --
}

// updateAccountSkipsCounts update the accounts' skips,
// the number of the consecutive blocks that doesn't have the account's transactions
func (p *TxPool) updateAccountSkipsCounts(latestActiveAccounts map[types.Address]uint64) {
	stateRoot := p.store.Header().StateRoot
	p.accounts.Range(
		func(key, value interface{}) bool {
			address, _ := key.(types.Address)
			account, _ := value.(*account)

			if _, ok := latestActiveAccounts[address]; ok {
				account.resetSkips()

				return true
			}

			firstTx := account.getLowestTx()
			if firstTx == nil {
				// no need to increment anything,
				// account has no txs
				return true
			}

			if account.incrementSkips() < maxAccountSkips {
				return true
			}

			/* START ALPHA CHANGE 2164 -- preserve queued transactions when cleanup cannot read state -- */
			// Account has been skipped too many times.
			nextNonce, err := p.store.GetNonce(stateRoot, firstTx.From)
			if err != nil {
				/* START ALPHA CHANGE 2173 -- stop the cleanup pass after unavailable local state -- */
				state.LogLocalStateFailure(
					p.logger,
					state.LocalStateLogTxPool,
					state.WrapLocalStateFailure(
						fmt.Sprintf("read cleanup nonce for %s", firstTx.From),
						err,
					),
				)

				// Preserve this account and stop this pass instead of
				// repeatedly reading unavailable state for other accounts.
				return false
				/* FINISH ALPHA CHANGE 2173 -- stop the cleanup pass after unavailable local state -- */
			}

			// START ALPHA CHANGE 2345 -- retain full-account removal for inactive-account cleanup --
			p.dropAccount(account, nextNonce, firstTx, false)
			account.resetSkips()
			// FINISH ALPHA CHANGE 2345 -- retain full-account removal for inactive-account cleanup --
			/* FINISH ALPHA CHANGE 2164 -- preserve queued transactions when cleanup cannot read state -- */

			return true
		},
	)
}

// getOrCreateAccount creates an account and
// ensures it is only initialized once.
/* START ALPHA CHANGE 2164 -- never initialize a pool account from a failed nonce read -- */
func (p *TxPool) getOrCreateAccount(newAddr types.Address) (*account, error) {
	if account := p.accounts.get(newAddr); account != nil {
		return account, nil
	}

	stateRoot := p.store.Header().StateRoot
	stateNonce, err := p.store.GetNonce(stateRoot, newAddr)
	if err != nil {
		return nil, state.WrapLocalStateFailure("initialize txpool account nonce", err)
	}

	return p.accounts.initOnce(newAddr, stateNonce), nil
}

/* FINISH ALPHA CHANGE 2164 -- never initialize a pool account from a failed nonce read -- */

// Length returns the total number of all promoted transactions.
func (p *TxPool) Length() uint64 {
	return p.accounts.promoted()
}

// toHash returns the hash(es) of given transaction(s)
func toHash(txs ...*types.Transaction) (hashes []types.Hash) {
	for _, tx := range txs {
		hashes = append(hashes, tx.Hash)
	}

	return
}
