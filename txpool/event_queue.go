package txpool

import (
	"sync"

	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
)

/* START ALPHA CHANGE 2541 -- bound transaction-event queues --

The ring is allocated on first use. Consumed entries are cleared, and
closing the queue releases its storage and prevents further insertion.

FINISH ALPHA CHANGE 2541 -- bound transaction-event queues -- */

const maxQueuedTxPoolEvents = 4096

type eventQueue struct {
	sync.Mutex

	events []*proto.TxPoolEvent
	head   int
	count  int
	closed bool
}

func (es *eventQueue) push(event *proto.TxPoolEvent) bool {
	es.Lock()
	defer es.Unlock()

	if es.closed || es.count == maxQueuedTxPoolEvents {
		return false
	}

	if len(es.events) == 0 {
		es.events = make([]*proto.TxPoolEvent, maxQueuedTxPoolEvents)
	}

	index := (es.head + es.count) % maxQueuedTxPoolEvents
	es.events[index] = event
	es.count++

	return true
}

func (es *eventQueue) pop() *proto.TxPoolEvent {
	es.Lock()
	defer es.Unlock()

	if es.count == 0 {
		return nil
	}

	event := es.events[es.head]
	es.events[es.head] = nil
	es.head = (es.head + 1) % maxQueuedTxPoolEvents
	es.count--

	return event
}

func (es *eventQueue) close() {
	es.Lock()
	defer es.Unlock()

	es.closed = true
	es.events = nil
	es.head = 0
	es.count = 0
}
