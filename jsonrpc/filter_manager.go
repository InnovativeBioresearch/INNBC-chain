package jsonrpc

import (
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"

	/* START ALPHA CHANGE 2254 -- bound subscription failure diagnostics -- */
	"golang.org/x/time/rate"
	/* FINISH ALPHA CHANGE 2254 -- bound subscription failure diagnostics -- */
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
)

var (
	ErrFilterNotFound                   = errors.New("filter not found")
	ErrWSFilterDoesNotSupportGetChanges = errors.New("web socket Filter doesn't support to return a batch of the changes")
	ErrCastingFilterToLogFilter         = errors.New("casting filter object to logFilter error")
	ErrBlockNotFound                    = errors.New("block not found")
	ErrIncorrectBlockRange              = errors.New("incorrect range")
	ErrBlockRangeTooHigh                = errors.New("block range too high")
	/* START ALPHA CHANGE 846 -- expose deliberate RPC error for pruned log history -- */
	ErrPrunedHistory = errors.New("requested historical data has been pruned on this node")
	/* FINISH ALPHA CHANGE 846 -- expose deliberate RPC error for pruned log history -- */
	ErrNoWSConnection          = errors.New("no websocket connection")
	ErrUnknownSubscriptionType = errors.New("unknown subscription type")
)

// defaultTimeout is the timeout to remove the filters that don't have a web socket stream
var defaultTimeout = 1 * time.Minute

const (
	// The index in heap which is indicating the element is not in the heap
	NoIndexInHeap = -1
)

// subscriptionType determines which event type the filter is subscribed to
type subscriptionType byte

const (
	// Blocks represents subscription type for blockchain events
	Blocks subscriptionType = iota
	// PendingTransactions represents subscription type for tx pool events
	PendingTransactions
)

// filter is an interface that BlockFilter and LogFilter implement
type filter interface {
	// hasWSConn returns the flag indicating the filter has web socket stream
	hasWSConn() bool

	// getFilterBase returns filterBase that has common fields
	getFilterBase() *filterBase

	// getSubscriptionType returns the type of the event the filter is subscribed to
	getSubscriptionType() subscriptionType

	// getUpdates returns stored data in a JSON serializable form
	getUpdates() (interface{}, error)

	// sendUpdates write stored data to web socket stream
	sendUpdates() error
}

// filterBase is a struct for common fields between BlockFilter and LogFilter
type filterBase struct {
	// UUID, a key of filter for client
	id string

	// index in the timeouts heap, -1 for non-existing index
	heapIndex int

	// timestamp to be expired
	expiresAt time.Time

	// websocket connection
	ws wsConn
}

// newFilterBase initializes filterBase with unique ID
func newFilterBase(ws wsConn) filterBase {
	uuidObj := uuid.New()

	return filterBase{
		id:        string(encodeToHex(uuidObj[:])),
		ws:        ws,
		heapIndex: NoIndexInHeap,
	}
}

// getFilterBase returns its own reference so that child struct can return base
func (f *filterBase) getFilterBase() *filterBase {
	return f
}

// hasWSConn returns the flag indicating this filter has websocket connection
func (f *filterBase) hasWSConn() bool {
	return f.ws != nil
}

const ethSubscriptionTemplate = `{
	"jsonrpc": "2.0",
	"method": "eth_subscription",
	"params": {
		"subscription":"%s",
		"result": %s
	}
}`

// writeMessageToWs sends given message to websocket stream
func (f *filterBase) writeMessageToWs(msg string) error {
	if !f.hasWSConn() {
		return ErrNoWSConnection
	}

	return f.ws.WriteMessage(
		websocket.TextMessage,
		/* START ALPHA CHANGE 826 -- use fmt.Appendf for websocket subscription payload -- */
		fmt.Appendf(nil, ethSubscriptionTemplate, f.id, msg),
		/* FINISH ALPHA CHANGE 826 -- use fmt.Appendf for websocket subscription payload -- */
	)
}

// blockFilter is a filter to store the updates of block
type blockFilter struct {
	filterBase
	sync.Mutex

	block *headElem
}

/*
	START ALPHA CHANGE 2555 -- serialize block-filter cursor consumption --

Read the cursor, collect its updates, and advance it under the same lock.
Concurrent polls of this filter must not consume the same cursor or move
it backward after another poll has advanced it.

FINISH ALPHA CHANGE 2555 -- serialize block-filter cursor consumption --
*/
func (f *blockFilter) takeBlockUpdates() []*block {
	f.Lock()
	defer f.Unlock()

	updates, newHead := f.block.getUpdates()
	f.block = newHead

	return updates
}

/* getUpdates returns block updates as hash strings. */
func (f *blockFilter) getUpdates() (any, error) {
	headers := f.takeBlockUpdates()

	updates := make([]string, len(headers))
	for index, header := range headers {
		updates[index] = header.Hash.String()
	}

	return updates, nil
}

// sendUpdates writes the updates of blocks to web socket stream
func (f *blockFilter) sendUpdates() error {
	updates := f.takeBlockUpdates()

	for _, header := range updates {
		raw, err := json.Marshal(header)
		if err != nil {
			return err
		}

		if err := f.writeMessageToWs(string(raw)); err != nil {
			return err
		}
	}

	return nil
}

// getSubscriptionType returns the type of the event the filter is subscribed to
func (f *blockFilter) getSubscriptionType() subscriptionType {
	return Blocks
}

// logFilter is a filter to store logs that meet the conditions in query
type logFilter struct {
	filterBase
	sync.Mutex

	query *LogQuery
	logs  []*Log
}

// appendLog appends new log to logs
func (f *logFilter) appendLog(log *Log) {
	f.Lock()
	defer f.Unlock()

	f.logs = append(f.logs, log)
}

// takeLogUpdates returns all saved logs in filter and set new log slice
func (f *logFilter) takeLogUpdates() []*Log {
	f.Lock()
	defer f.Unlock()

	logs := f.logs
	f.logs = []*Log{} // create brand-new slice so that prevent new logs from being added to current logs

	return logs
}

// getUpdates returns stored logs in string
func (f *logFilter) getUpdates() (interface{}, error) {
	logs := f.takeLogUpdates()

	return logs, nil
}

// sendUpdates writes stored logs to web socket stream
func (f *logFilter) sendUpdates() error {
	logs := f.takeLogUpdates()

	for _, log := range logs {
		res, err := json.Marshal(log)
		if err != nil {
			return err
		}

		if err := f.writeMessageToWs(string(res)); err != nil {
			return err
		}
	}

	return nil
}

// getSubscriptionType returns the type of the event the filter is subscribed to
func (f *logFilter) getSubscriptionType() subscriptionType {
	return Blocks
}

// pendingTxFilter is a filter to store pending tx
type pendingTxFilter struct {
	filterBase
	sync.Mutex

	txHashes []string
}

// appendPendingTxHashes appends new pending tx hash to tx hashes
func (f *pendingTxFilter) appendPendingTxHashes(txHash string) {
	f.Lock()
	defer f.Unlock()

	f.txHashes = append(f.txHashes, txHash)
}

// takePendingTxsUpdates returns all saved pending tx hashes in filter and sets a new slice
func (f *pendingTxFilter) takePendingTxsUpdates() []string {
	f.Lock()
	defer f.Unlock()

	txHashes := f.txHashes
	f.txHashes = []string{}

	return txHashes
}

// getSubscriptionType returns the type of the event the filter is subscribed to
func (f *pendingTxFilter) getSubscriptionType() subscriptionType {
	return PendingTransactions
}

// getUpdates returns stored pending tx hashes
func (f *pendingTxFilter) getUpdates() (interface{}, error) {
	pendingTxHashes := f.takePendingTxsUpdates()

	return pendingTxHashes, nil
}

// sendUpdates write the hashes for all pending transactions to web socket stream
func (f *pendingTxFilter) sendUpdates() error {
	pendingTxHashes := f.takePendingTxsUpdates()

	for _, txHash := range pendingTxHashes {
		if err := f.writeMessageToWs(txHash); err != nil {
			return err
		}
	}

	return nil
}

// filterManagerStore provides methods required by FilterManager
type filterManagerStore interface {
	// Header returns the current header of the chain (genesis if empty)
	Header() *types.Header

	/* START ALPHA CHANGE 3293 -- expose existing history availability to historical log queries -- */
	// HistoryFrom returns the first continuously available full block.
	// Zero means history is continuous from genesis.
	HistoryFrom() uint64
	/* FINISH ALPHA CHANGE 3293 -- expose existing history availability to historical log queries -- */

	// SubscribeEvents subscribes for chain head events
	SubscribeEvents() blockchain.Subscription

	/* START ALPHA CHANGE 1412 -- let filter manager release blockchain subscriptions -- */
	// UnsubscribeEvents unsubscribes from chain head events
	UnsubscribeEvents(blockchain.Subscription)
	/* FINISH ALPHA CHANGE 1412 -- let filter manager release blockchain subscriptions -- */

	// GetReceiptsByHash returns the receipts for a block hash
	GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error)

	// GetBlockByHash returns the block using the block hash
	GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool)

	// GetBlockByNumber returns a block using the provided number
	GetBlockByNumber(num uint64, full bool) (*types.Block, bool)

	// TxPoolSubscribe subscribes for tx pool events
	TxPoolSubscribe(request *proto.SubscribeRequest) (<-chan *proto.TxPoolEvent, func(), error)
}

// FilterManager manages all running filters
type FilterManager struct {
	sync.RWMutex

	logger hclog.Logger

	timeout time.Duration

	store           filterManagerStore
	subscription    blockchain.Subscription
	blockStream     *blockStream
	blockRangeLimit uint64

	filters  map[string]filter
	timeouts timeHeapImpl

	/* START ALPHA CHANGE 2545 -- gate pending subscriptions during event-feed recovery -- */
	txEventsUnavailable bool
	/* FINISH ALPHA CHANGE 2545 -- gate pending subscriptions during event-feed recovery -- */

	/* START ALPHA CHANGE 2247 -- bound retained RPC filters and subscriptions -- */
	maxFilters int
	/* FINISH ALPHA CHANGE 2247 -- bound retained RPC filters and subscriptions -- */

	/* START ALPHA CHANGE 2254 -- retain bounded subscription logging state -- */
	flushLogLimiter *rate.Limiter
	flushSuppressed uint64
	/* FINISH ALPHA CHANGE 2254 -- retain bounded subscription logging state -- */

	updateCh chan struct{}
	closeCh  chan struct{}

	/* START ALPHA CHANGE 1413 -- coordinate filter manager startup and shutdown -- */
	runLock   sync.Mutex
	closeOnce sync.Once
	closeWG   sync.WaitGroup
	running   bool
	closing   bool
	/* FINISH ALPHA CHANGE 1413 -- coordinate filter manager startup and shutdown -- */
}

func NewFilterManager(logger hclog.Logger, store filterManagerStore, blockRangeLimit uint64) *FilterManager {
	/* START ALPHA CHANGE 2481 -- apply the safe log-range default during construction -- */
	blockRangeLimit = ResolveBlockRangeLimit(blockRangeLimit)
	/* FINISH ALPHA CHANGE 2481 -- apply the safe log-range default during construction -- */

	m := &FilterManager{
		logger:          logger.Named("filter"),
		timeout:         defaultTimeout,
		store:           store,
		blockRangeLimit: blockRangeLimit,
		filters:         make(map[string]filter),
		timeouts:        timeHeapImpl{},
		/* START ALPHA CHANGE 2247 -- apply a finite default filter capacity -- */
		maxFilters: DefaultAdmissionConfig().MaxFilters,
		/* FINISH ALPHA CHANGE 2247 -- apply a finite default filter capacity -- */
		/* START ALPHA CHANGE 2254 -- report subscription failures at bounded frequency -- */
		flushLogLimiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
		/* FINISH ALPHA CHANGE 2254 -- report subscription failures at bounded frequency -- */
		/* START ALPHA CHANGE 2538 -- retain a pending timeout-change notification -- */
		updateCh: make(chan struct{}, 1),
		/* FINISH ALPHA CHANGE 2538 -- retain a pending timeout-change notification -- */
		closeCh: make(chan struct{}),
	}

	// start blockstream with the current header
	header := store.Header()

	block := toBlock(&types.Block{Header: header}, false)
	m.blockStream = newBlockStream(block)

	// start the head watcher
	m.subscription = store.SubscribeEvents()

	return m
}

// Run starts worker process to handle events
func (f *FilterManager) Run() {
	/* START ALPHA CHANGE 1414 -- admit filter workers before shutdown wait begins -- */
	f.runLock.Lock()
	if f.closing || f.running {
		f.runLock.Unlock()

		return
	}

	f.running = true
	f.closeWG.Add(2)
	f.runLock.Unlock()

	defer func() {
		f.runLock.Lock()
		f.running = false
		f.runLock.Unlock()

		f.closeWG.Done()
	}()

	// watch for new events in the blockchain
	blockWatchCh := make(chan *blockchain.Event)

	go func() {
		defer f.closeWG.Done()
		defer close(blockWatchCh)

		if f.subscription == nil {
			return
		}

		for {
			evnt := f.subscription.GetEvent()
			if evnt == nil {
				return
			}

			select {
			case blockWatchCh <- evnt:
			case <-f.closeCh:
				return
			}
		}
	}()
	/* FINISH ALPHA CHANGE 1414 -- admit filter workers before shutdown wait begins -- */

	/* START ALPHA CHANGE 2545 -- recover the transaction feed without stopping filter processing -- */
	txFeed := newFilterTxFeed(f)
	defer txFeed.close()
	/* FINISH ALPHA CHANGE 2545 -- recover the transaction feed without stopping filter processing -- */

	/* START ALPHA CHANGE 2538 -- reschedule expiry from current heap state -- */
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	for {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		var timeoutCh <-chan time.Time
		filterID, filterExpiresAt := f.nextTimeoutFilter()
		if filterID != "" {
			timer.Reset(time.Until(filterExpiresAt))
			timeoutCh = timer.C
		}

		select {
		case evnt, ok := <-blockWatchCh:
			if !ok {
				return
			}

			if err := f.dispatchEvent(evnt); err != nil {
				f.logger.Error("failed to dispatch block event", "err", err)
			}

		/* START ALPHA CHANGE 2545 -- keep block delivery and expiry running after event-feed loss -- */
		case evnt, ok := <-txFeed.events:
			if !ok {
				txFeed.lost()
				continue
			}

			if err := f.dispatchEvent(evnt); err != nil {
				f.logger.Error("failed to dispatch tx pool event", "err", err)
			}

		case <-txFeed.retryCh:
			txFeed.connect()
		/* FINISH ALPHA CHANGE 2545 -- keep block delivery and expiry running after event-feed loss -- */

		case <-timeoutCh:
			f.expireFilters(time.Now().UTC())

		case <-f.updateCh:
			/* Recompute the deadline from the current heap. */

		case <-f.closeCh:
			return
		}
	}
	/* FINISH ALPHA CHANGE 2538 -- reschedule expiry from current heap state -- */
}

// Close closed closeCh so that terminate worker
func (f *FilterManager) Close() {
	/* START ALPHA CHANGE 1415 -- unsubscribe and join filter manager workers -- */
	f.closeOnce.Do(func() {
		f.runLock.Lock()
		f.closing = true
		close(f.closeCh)
		subscription := f.subscription
		f.runLock.Unlock()

		if subscription != nil {
			f.store.UnsubscribeEvents(subscription)
		}

		f.closeWG.Wait()
	})
	/* FINISH ALPHA CHANGE 1415 -- unsubscribe and join filter manager workers -- */
}

// NewBlockFilter adds new BlockFilter
func (f *FilterManager) NewBlockFilter(ws wsConn) string {
	filter := &blockFilter{
		filterBase: newFilterBase(ws),
		block:      f.blockStream.getHead(),
	}

	/* START ALPHA CHANGE 2247 -- publish filter metadata only after capacity admission -- */
	return f.addFilter(filter)
	/* FINISH ALPHA CHANGE 2247 -- publish filter metadata only after capacity admission -- */
}

/* START ALPHA CHANGE 2252 -- publish subscription metadata only after admission -- */
/* START ALPHA CHANGE 2513 -- prevent registration of nil log queries --

NewLogFilter returns an empty ID when the query is nil or filter capacity
is exhausted. RPC callers validate the query before registration so
invalid parameters remain distinct from capacity failures.

FINISH ALPHA CHANGE 2513 -- prevent registration of nil log queries -- */
func (f *FilterManager) NewLogFilter(logQuery *LogQuery, ws wsConn) string {
	if logQuery == nil {
		return ""
	}

	filter := &logFilter{
		filterBase: newFilterBase(ws),
		query:      logQuery,
	}

	return f.addFilter(filter)
}

// NewPendingTxFilter adds new PendingTxFilter.
func (f *FilterManager) NewPendingTxFilter(ws wsConn) string {
	filter := &pendingTxFilter{
		filterBase: newFilterBase(ws),
		txHashes:   []string{},
	}

	return f.addFilter(filter)
}

/* FINISH ALPHA CHANGE 2252 -- publish subscription metadata only after admission -- */

// Exists checks the filter with given ID exists
func (f *FilterManager) Exists(id string) bool {
	f.RLock()
	defer f.RUnlock()

	_, ok := f.filters[id]

	return ok
}

func (f *FilterManager) getLogsFromBlock(query *LogQuery, block *types.Block) ([]*Log, error) {
	receipts, err := f.store.GetReceiptsByHash(block.Header.Hash)
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 3519 -- reject incomplete historical receipt lists before log extraction -- */
	// Every transaction requires a receipt, including transactions whose
	// receipts contain no logs.
	if len(receipts) != len(block.Transactions) {
		return nil, fmt.Errorf(
			"receipt count mismatch for block %s: expected %d, got %d",
			block.Header.Hash.String(),
			len(block.Transactions),
			len(receipts),
		)
	}
	/* FINISH ALPHA CHANGE 3519 -- reject incomplete historical receipt lists before log extraction -- */

	logIdx := uint64(0)
	logs := make([]*Log, 0)

	for idx, receipt := range receipts {
		for _, log := range receipt.Logs {
			if query.Match(log) {
				logs = append(logs, toLog(log, logIdx, uint64(idx), block.Header, block.Transactions[idx].Hash))
			}

			logIdx++
		}
	}

	return logs, nil
}

func (f *FilterManager) getLogsFromBlocks(query *LogQuery) ([]*Log, error) {
	from, err := GetNumericBlockNumber(query.fromBlock, f.store)
	if err != nil {
		return nil, err
	}

	to, err := GetNumericBlockNumber(query.toBlock, f.store)
	if err != nil {
		return nil, err
	}

	if to < from {
		return nil, ErrIncorrectBlockRange
	}

	/* START ALPHA CHANGE 2502 -- handle an empty range after skipping genesis --

	Preserve the existing genesis exclusion. A genesis-only query leaves
	no blocks to scan and must return before unsigned range subtraction.

	FINISH ALPHA CHANGE 2502 -- handle an empty range after skipping genesis -- */
	if from == 0 {
		from = 1
	}
	if from > to {
		return []*Log{}, nil
	}

	/* START ALPHA CHANGE 2481 -- enforce a finite log range when the stored limit is zero -- */
	if to-from > ResolveBlockRangeLimit(f.blockRangeLimit) {
		return nil, ErrBlockRangeTooHigh
	}
	/* FINISH ALPHA CHANGE 2481 -- enforce a finite log range when the stored limit is zero -- */

	logs := make([]*Log, 0)

	for i := from; i <= to; i++ {
		block, ok := f.store.GetBlockByNumber(i, true)
		if !ok {
			/* START ALPHA CHANGE 848 -- report pruned blocks explicitly for historical log ranges -- */
			if _, headerOnly := f.store.GetBlockByNumber(i, false); headerOnly {
				return nil, ErrPrunedHistory
			}
			/* FINISH ALPHA CHANGE 848 -- report pruned blocks explicitly for historical log ranges -- */

			/* START ALPHA CHANGE 3294 -- distinguish removed history and unexpected gaps from future blocks -- */
			head := f.store.Header()
			if head == nil {
				return nil, ErrLatestNotFound
			}

			// Preserve normal termination when the requested range
			// extends beyond the node's current head.
			if i > head.Number {
				break
			}

			// Physical retirement removes headers and bodies together.
			// Consult existing availability metadata only on this failure path.
			if i < f.store.HistoryFrom() {
				return nil, ErrPrunedHistory
			}

			// A missing block inside advertised history must not turn
			// accumulated logs into a successful partial response.
			return nil, ErrBlockNotFound
			/* FINISH ALPHA CHANGE 3294 -- distinguish removed history and unexpected gaps from future blocks -- */
		}

		if len(block.Transactions) == 0 {
			// do not check logs if no txs
			continue
		}

		blockLogs, err := f.getLogsFromBlock(query, block)
		if err != nil {
			return nil, err
		}

		logs = append(logs, blockLogs...)
	}

	return logs, nil
}

// GetLogsForQuery return array of logs for given query
func (f *FilterManager) GetLogsForQuery(query *LogQuery) ([]*Log, error) {
	/* START ALPHA CHANGE 2513 -- reject nil queries at the filter manager boundary -- */
	if query == nil {
		return nil, NewInvalidParamsError("Log filter must be a non-null object")
	}
	/* FINISH ALPHA CHANGE 2513 -- reject nil queries at the filter manager boundary -- */

	if query.BlockHash != nil {
		// BlockHash is set -> fetch logs from this block only
		block, ok := f.store.GetBlockByHash(*query.BlockHash, true)
		if !ok {
			/* START ALPHA CHANGE 849 -- report pruned block hash log queries explicitly -- */
			if _, headerOnly := f.store.GetBlockByHash(*query.BlockHash, false); headerOnly {
				return nil, ErrPrunedHistory
			}
			/* FINISH ALPHA CHANGE 849 -- report pruned block hash log queries explicitly -- */

			return nil, ErrBlockNotFound
		}

		if len(block.Transactions) == 0 {
			// no txs in block, return empty response
			return []*Log{}, nil
		}

		return f.getLogsFromBlock(query, block)
	}

	// gets logs from a range of blocks
	return f.getLogsFromBlocks(query)
}

// getFilterByID fetches the filter by the ID
func (f *FilterManager) getFilterByID(filterID string) filter {
	f.RLock()
	defer f.RUnlock()

	return f.filters[filterID]
}

// GetLogFilterFromID return log filter for given filterID
func (f *FilterManager) GetLogFilterFromID(filterID string) (*logFilter, error) {
	filterRaw := f.getFilterByID(filterID)

	if filterRaw == nil {
		return nil, ErrFilterNotFound
	}

	logFilter, ok := filterRaw.(*logFilter)
	if !ok {
		return nil, ErrCastingFilterToLogFilter
	}

	return logFilter, nil
}

// GetFilterChanges returns the updates of the filter with given ID in string, and refreshes the timeout on the filter
func (f *FilterManager) GetFilterChanges(id string) (interface{}, error) {
	filter, res, err := f.getFilterAndChanges(id)

	if err == nil && !filter.hasWSConn() {
		// Refresh the timeout on this filter
		f.Lock()
		f.refreshFilterTimeout(filter.getFilterBase())
		f.Unlock()
	}

	return res, err
}

// getFilterAndChanges returns the updates of the filter with given ID in string (read lock only)
func (f *FilterManager) getFilterAndChanges(id string) (filter, interface{}, error) {
	f.RLock()
	defer f.RUnlock()

	filter, ok := f.filters[id]

	if !ok {
		return nil, nil, ErrFilterNotFound
	}

	// we cannot get updates from a ws filter with getFilterChanges
	if filter.hasWSConn() {
		return nil, nil, ErrWSFilterDoesNotSupportGetChanges
	}

	res, err := filter.getUpdates()
	if err != nil {
		return nil, nil, err
	}

	return filter, res, nil
}

// Uninstall removes the filter with given ID from list
func (f *FilterManager) Uninstall(id string) bool {
	f.Lock()
	defer f.Unlock()

	return f.removeFilterByID(id)
}

// removeFilterByID removes the filter with given ID [NOT Thread Safe]
func (f *FilterManager) removeFilterByID(id string) bool {
	// Make sure filter exists
	filter, ok := f.filters[id]
	if !ok {
		return false
	}

	delete(f.filters, id)

	if removed := f.timeouts.removeFilter(filter.getFilterBase()); removed {
		f.emitSignalToUpdateCh()
	}

	return true
}

/* START ALPHA CHANGE 2247 -- release every subscription owned by a closed WebSocket -- */
// RemoveFilterByWs removes all filters associated with the connection.
func (f *FilterManager) RemoveFilterByWs(ws wsConn) {
	if ws == nil {
		return
	}

	f.Lock()
	defer f.Unlock()

	for id, filter := range f.filters {
		if filter.getFilterBase().ws == ws {
			f.removeFilterByID(id)
		}
	}
}

/* FINISH ALPHA CHANGE 2247 -- release every subscription owned by a closed WebSocket -- */

/* START ALPHA CHANGE 2538 -- refresh only the currently registered filter --

The caller must hold the manager's write lock. A poll may retain a filter
pointer after uninstall, so verify both registration and object identity
before changing the heap.

FINISH ALPHA CHANGE 2538 -- refresh only the currently registered filter -- */

func (f *FilterManager) refreshFilterTimeout(base *filterBase) {
	current, ok := f.filters[base.id]
	if !ok || current.getFilterBase() != base {
		return
	}

	f.timeouts.removeFilter(base)
	f.addFilterTimeout(base)
}

// addFilterTimeout set timeout and add to heap
func (f *FilterManager) addFilterTimeout(filter *filterBase) {
	filter.expiresAt = time.Now().UTC().Add(f.timeout)
	f.timeouts.addFilter(filter)
	f.emitSignalToUpdateCh()
}

/* START ALPHA CHANGE 2247 -- atomically admit filters within configured capacity -- */
// addFilter returns an empty ID when capacity is exhausted.
// Production callers must report that condition as an RPC resource-limit error.
func (f *FilterManager) addFilter(filter filter) string {
	f.Lock()
	defer f.Unlock()

	/* START ALPHA CHANGE 2545 -- admit pending filters only while their event feed is available -- */
	if len(f.filters) >= f.maxFilters ||
		(f.txEventsUnavailable && filter.getSubscriptionType() == PendingTransactions) {
		return ""
	}
	/* FINISH ALPHA CHANGE 2545 -- admit pending filters only while their event feed is available -- */

	base := filter.getFilterBase()
	f.filters[base.id] = filter

	if filter.hasWSConn() {
		base.ws.SetFilterID(base.id)
	} else {
		f.addFilterTimeout(base)
	}

	return base.id
}

/* FINISH ALPHA CHANGE 2247 -- atomically admit filters within configured capacity -- */

func (f *FilterManager) emitSignalToUpdateCh() {
	select {
	// notify worker of new filter with timeout
	case f.updateCh <- struct{}{}:
	default:
	}
}

/* START ALPHA CHANGE 2538 -- remove expired filters and detached heap entries --

Read registration, identity, and the current deadline under one write lock.
A stale timer must not remove a refreshed filter. Detached heap entries
must be discarded even when their IDs are no longer registered.

FINISH ALPHA CHANGE 2538 -- remove expired filters and detached heap entries -- */

func (f *FilterManager) expireFilters(now time.Time) {
	f.Lock()
	defer f.Unlock()

	for len(f.timeouts) > 0 {
		base := f.timeouts[0]
		current, ok := f.filters[base.id]

		if !ok || current.getFilterBase() != base {
			heap.Pop(&f.timeouts)
			continue
		}

		if base.expiresAt.After(now) {
			return
		}

		heap.Pop(&f.timeouts)
		delete(f.filters, base.id)
	}
}

/* nextTimeoutFilter returns the earliest deadline without removing it. */
func (f *FilterManager) nextTimeoutFilter() (string, time.Time) {
	f.RLock()
	defer f.RUnlock()

	if len(f.timeouts) == 0 {
		return "", time.Time{}
	}

	// peek the first item
	base := f.timeouts[0]

	return base.id, base.expiresAt
}

// dispatchEvent is an event handler for new block event
func (f *FilterManager) dispatchEvent(evnt interface{}) error {
	var subType subscriptionType

	// store new event in each filters
	switch evt := evnt.(type) {
	case *blockchain.Event:
		f.processBlockEvent(evt)

		subType = Blocks
	case *proto.TxPoolEvent:
		f.processTxEvent(evt)

		subType = PendingTransactions

	default:
		return ErrUnknownSubscriptionType
	}

	// send data to web socket stream
	if err := f.flushWsFilters(subType); err != nil {
		return err
	}

	return nil
}

// processEvent makes each filter append the new data that interests them
func (f *FilterManager) processBlockEvent(evnt *blockchain.Event) {
	f.RLock()
	defer f.RUnlock()

	for _, header := range evnt.NewChain {
		block := toBlock(&types.Block{Header: header}, false)

		// first include all the new headers in the blockstream for BlockFilter
		f.blockStream.push(block)

		// process new chain to include new logs for LogFilter
		if processErr := f.appendLogsToFilters(block); processErr != nil {
			f.logger.Error(fmt.Sprintf("Unable to process block, %v", processErr))
		}
	}
}

// appendLogsToFilters makes each LogFilters append logs in the header
func (f *FilterManager) appendLogsToFilters(header *block) error {
	receipts, err := f.store.GetReceiptsByHash(header.Hash)
	if err != nil {
		return err
	}

	// Get logFilters from filters
	logFilters := make([]*logFilter, 0)

	for _, f := range f.filters {
		if logFilter, ok := f.(*logFilter); ok {
			logFilters = append(logFilters, logFilter)
		}
	}

	if len(logFilters) == 0 {
		return nil
	}

	block, ok := f.store.GetBlockByHash(header.Hash, true)
	if !ok {
		f.logger.Error("could not find block in store", "hash", header.Hash.String())

		return nil
	}

	/* START ALPHA CHANGE 3573 -- reject mismatched live receipt lists before indexing or appending logs -- */
	if len(receipts) != len(block.Transactions) {
		return fmt.Errorf(
			"receipt count mismatch for block %s: expected %d, got %d",
			header.Hash.String(),
			len(block.Transactions),
			len(receipts),
		)
	}
	/* FINISH ALPHA CHANGE 3573 -- reject mismatched live receipt lists before indexing or appending logs -- */

	logIndex := uint64(0)

	for indx, receipt := range receipts {
		if receipt.TxHash == types.ZeroHash {
			// Extract tx Hash
			receipt.TxHash = block.Transactions[indx].Hash
		}
		// check the logs with the filters
		for _, log := range receipt.Logs {
			for _, f := range logFilters {
				if f.query.Match(log) {
					f.appendLog(toLog(log, logIndex, uint64(indx), block.Header, receipt.TxHash))
				}
			}

			logIndex++
		}
	}

	return nil
}

// processTxEvent makes each filter refresh the pending tx hashes
func (f *FilterManager) processTxEvent(evnt *proto.TxPoolEvent) {
	f.RLock()
	defer f.RUnlock()

	for _, f := range f.filters {
		if txFilter, ok := f.(*pendingTxFilter); ok {
			txFilter.appendPendingTxHashes(evnt.TxHash)
		}
	}
}

/* START ALPHA CHANGE 2254 -- retire failed subscriptions without repeated flush logging -- */
// flushWsFilters delivers updates and removes all subscriptions belonging to
// connections whose delivery failed. Production write failures also close the
// underlying socket through wsWrapper.WriteMessage.
func (f *FilterManager) flushWsFilters(subType subscriptionType) error {
	failed := make(map[wsConn]error)

	f.RLock()

	for _, filter := range f.filters {
		if !filter.hasWSConn() || filter.getSubscriptionType() != subType {
			continue
		}

		connection := filter.getFilterBase().ws
		if _, alreadyFailed := failed[connection]; alreadyFailed {
			continue
		}

		if err := filter.sendUpdates(); err != nil {
			failed[connection] = err
		}
	}

	f.RUnlock()

	if len(failed) == 0 {
		return nil
	}

	var (
		firstUnexpected error
		unexpectedCount uint64
		removed         int
		logFailure      bool
		suppressed      uint64
	)

	for _, err := range failed {
		// Normal connection closure needs no warning.
		if errors.Is(err, websocket.ErrCloseSent) || errors.Is(err, net.ErrClosed) {
			continue
		}

		unexpectedCount++
		if firstUnexpected == nil {
			firstUnexpected = err
		}
	}

	f.Lock()

	// Remove every subscription for each failed connection, including those
	// belonging to another subscription type.
	for id, filter := range f.filters {
		if _, failedConnection := failed[filter.getFilterBase().ws]; failedConnection {
			if f.removeFilterByID(id) {
				removed++
			}
		}
	}

	if unexpectedCount != 0 {
		if f.flushLogLimiter.Allow() {
			logFailure = true
			suppressed = f.flushSuppressed
			f.flushSuppressed = 0
		} else {
			f.flushSuppressed += unexpectedCount
		}
	}

	f.Unlock()

	if logFailure {
		f.logger.Warn(
			"removed subscriptions after delivery failure",
			"err", firstUnexpected,
			"failedConnections", len(failed),
			"removedSubscriptions", removed,
			"suppressedFailures", suppressed,
		)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2254 -- retire failed subscriptions without repeated flush logging -- */

type timeHeapImpl []*filterBase

func (t *timeHeapImpl) addFilter(filter *filterBase) {
	heap.Push(t, filter)
}

func (t *timeHeapImpl) removeFilter(filter *filterBase) bool {
	if filter.heapIndex == NoIndexInHeap {
		return false
	}

	heap.Remove(t, filter.heapIndex)

	return true
}

func (t timeHeapImpl) Len() int { return len(t) }

func (t timeHeapImpl) Less(i, j int) bool {
	return t[i].expiresAt.Before(t[j].expiresAt)
}

func (t timeHeapImpl) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].heapIndex = i
	t[j].heapIndex = j
}

func (t *timeHeapImpl) Push(x interface{}) {
	n := len(*t)
	item := x.(*filterBase) //nolint: forcetypeassert
	item.heapIndex = n
	*t = append(*t, item)
}

func (t *timeHeapImpl) Pop() interface{} {
	old := *t
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.heapIndex = -1
	*t = old[0 : n-1]

	return item
}

// blockStream is used to keep the stream of new block hashes and allow subscriptions
// of the stream at any point
type blockStream struct {
	lock sync.Mutex
	head atomic.Value
}

func newBlockStream(head *block) *blockStream {
	b := &blockStream{}
	b.head.Store(&headElem{header: head})

	return b
}

func (b *blockStream) getHead() *headElem {
	head, _ := b.head.Load().(*headElem)

	return head
}

func (b *blockStream) push(header *block) {
	b.lock.Lock()
	defer b.lock.Unlock()

	newHead := &headElem{
		header: header.Copy(),
	}

	oldHead, _ := b.head.Load().(*headElem)
	oldHead.next.Store(newHead)

	b.head.Store(newHead)
}

type headElem struct {
	header *block
	next   atomic.Value
}

func (h *headElem) getUpdates() ([]*block, *headElem) {
	res := make([]*block, 0)
	cur := h

	for {
		next := cur.next.Load()
		if next == nil {
			// no more messages
			break
		} else {
			nextElem, _ := next.(*headElem)

			if nextElem.header != nil {
				res = append(res, nextElem.header)
			}
			cur = nextElem
		}
	}

	return res, cur
}
