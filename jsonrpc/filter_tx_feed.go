package jsonrpc

import (
	"time"

	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
)

/* START ALPHA CHANGE 2545 -- recover internal transaction-event subscriptions --

This controller is owned by FilterManager.Run. Retrying the local event
feed does not query contracts or perform peer-directory exchanges.

FINISH ALPHA CHANGE 2545 -- recover internal transaction-event subscriptions -- */

const filterTxFeedRetryDelay = time.Second

type filterTxFeed struct {
	manager *FilterManager
	events  <-chan *proto.TxPoolEvent
	cancel  func()

	timer    *time.Timer
	retryCh  <-chan time.Time
	retrying bool
}

func newFilterTxFeed(manager *FilterManager) *filterTxFeed {
	feed := &filterTxFeed{manager: manager}
	feed.connect()
	return feed
}

func (feed *filterTxFeed) connect() {
	if feed.timer != nil {
		feed.timer.Stop()
		feed.timer = nil
	}
	feed.retryCh = nil

	select {
	case <-feed.manager.closeCh:
		return
	default:
	}

	events, cancel, err := feed.manager.store.TxPoolSubscribe(
		&proto.SubscribeRequest{Types: []proto.EventType{proto.EventType_ADDED}},
	)
	if err != nil || events == nil {
		if cancel != nil {
			cancel()
		}
		feed.lost()
		return
	}

	feed.events = events
	feed.cancel = cancel

	feed.manager.Lock()
	feed.manager.txEventsUnavailable = false
	feed.manager.Unlock()

	if feed.retrying {
		feed.manager.logger.Info("transaction-event feed restored")
	}
	feed.retrying = false
}

func (feed *filterTxFeed) lost() {
	if feed.cancel != nil {
		feed.cancel()
		feed.cancel = nil
	}
	feed.events = nil

	if !feed.retrying {
		feed.manager.invalidatePendingTxFilters()
		feed.manager.logger.Warn(
			"transaction-event feed interrupted; pending subscriptions must be recreated",
		)
	}
	feed.retrying = true

	if feed.timer != nil {
		feed.timer.Stop()
	}
	feed.timer = time.NewTimer(filterTxFeedRetryDelay)
	feed.retryCh = feed.timer.C
}

func (feed *filterTxFeed) close() {
	if feed.timer != nil {
		feed.timer.Stop()
	}
	if feed.cancel != nil {
		feed.cancel()
		feed.cancel = nil
	}
}

/* START ALPHA CHANGE 2545 -- make pending-event gaps visible to subscribers --

Remove pending filters and close affected WebSocket connections. Filters
sharing those connections are removed too. Other connections and polling
block/log filters remain available. Socket closure occurs outside the
manager lock.

FINISH ALPHA CHANGE 2545 -- make pending-event gaps visible to subscribers -- */

func (f *FilterManager) invalidatePendingTxFilters() {
	connections := make(map[wsConn]struct{})

	f.Lock()
	f.txEventsUnavailable = true

	for _, filter := range f.filters {
		if filter.getSubscriptionType() == PendingTransactions {
			if ws := filter.getFilterBase().ws; ws != nil {
				connections[ws] = struct{}{}
			}
		}
	}

	for id, filter := range f.filters {
		_, affectedConnection := connections[filter.getFilterBase().ws]
		if filter.getSubscriptionType() == PendingTransactions || affectedConnection {
			f.removeFilterByID(id)
		}
	}
	f.Unlock()

	for connection := range connections {
		if closer, ok := connection.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}
}
