package blockchain

import (
	"sync"

	"github.com/innovativebioresearch/innbc-chain/types"
)

type void struct{}

// Subscription is the blockchain subscription interface
type Subscription interface {
	GetEventCh() chan *Event
	GetEvent() *Event
}

// FOR TESTING PURPOSES //

type MockSubscription struct {
	*subscription

	/* START ALPHA CHANGE 1416 -- make mock subscription close idempotent -- */
	closeOnce sync.Once
	/* FINISH ALPHA CHANGE 1416 -- make mock subscription close idempotent -- */
}

func NewMockSubscription() *MockSubscription {
	return &MockSubscription{
		subscription: &subscription{
			updateCh: make(chan *Event),
			closeCh:  make(chan void),
		},
	}
}
func (m *MockSubscription) Push(e *Event) {
	if m == nil || m.subscription == nil {
		return
	}

	select {
	case m.updateCh <- e:
	case <-m.closeCh:
	}
}

/* START ALPHA CHANGE 1417 -- wake mock subscription consumers on close -- */
func (m *MockSubscription) Close() {
	if m == nil || m.subscription == nil {
		return
	}

	m.closeOnce.Do(func() {
		close(m.closeCh)
	})
}

/* FINISH ALPHA CHANGE 1417 -- wake mock subscription consumers on close -- */

// subscription is the Blockchain event subscription object
type subscription struct {
	updateCh chan *Event // Channel for update information
	closeCh  chan void   // Channel for close signals
}

// GetEventCh creates a new event channel, and returns it
func (s *subscription) GetEventCh() chan *Event {
	return s.updateCh
}

// GetEvent returns the event from the subscription (BLOCKING)
func (s *subscription) GetEvent() *Event {
	// Wait for an update
	select {
	case ev := <-s.updateCh:
		return ev
	case <-s.closeCh:
		return nil
	}
}

type EventType int

/* START ALPHA CHANGE 780 -- remove fork and reorg blockchain event types -- */
const (
	EventHead EventType = iota // New head event
)

/* FINISH ALPHA CHANGE 780 -- remove fork and reorg blockchain event types -- */

// Event is the blockchain event that gets passed to the listeners
type Event struct {
	/* START ALPHA CHANGE 781 -- keep blockchain events canonical-head only -- */
	// New part of the canonical chain
	NewChain []*types.Header

	// Type is the type of event
	Type EventType
	/* FINISH ALPHA CHANGE 781 -- keep blockchain events canonical-head only -- */

	// Source is the source that generated the blocks for the event
	// right now it can be either the Sealer or the Syncer
	Source string
}

// Header returns the latest block header for the event
func (e *Event) Header() *types.Header {
	return e.NewChain[len(e.NewChain)-1]
}

/* START ALPHA CHANGE 782 -- remove total-difficulty event payload -- */
// AddNewHeader appends a header to the event's NewChain array
func (e *Event) AddNewHeader(newHeader *types.Header) {
	/* FINISH ALPHA CHANGE 782 -- remove total-difficulty event payload -- */
	header := newHeader.Copy()

	if e.NewChain == nil {
		// Array doesn't exist yet, create it
		e.NewChain = []*types.Header{}
	}

	e.NewChain = append(e.NewChain, header)
}

/* START ALPHA CHANGE 783 -- remove old-chain event payload after reorg pruning -- */
// SubscribeEvents returns a blockchain event subscription
/* FINISH ALPHA CHANGE 783 -- remove old-chain event payload after reorg pruning -- */
func (b *Blockchain) SubscribeEvents() Subscription {
	return b.stream.subscribe()
}

// UnsubscribeEvents removes subscription from blockchain event stream
func (b *Blockchain) UnsubscribeEvents(sub Subscription) {
	if subPtr, ok := sub.(*subscription); ok {
		b.stream.unsubscribe(subPtr)
	} else {
		b.logger.Warn("Failed to unsubscribe from event stream. Invalid subscription.")
	}
}

// eventStream is the structure that contains the subscribers
// which it uses to notify of updates
type eventStream struct {
	sync.RWMutex
	subscriptions map[*subscription]struct{}
}

// newEventStream creates event stream and initializes subscriptions map
func newEventStream() *eventStream {
	return &eventStream{
		subscriptions: make(map[*subscription]struct{}),
	}
}

// subscribe creates a new blockchain event subscription
func (e *eventStream) subscribe() *subscription {
	sub := &subscription{
		updateCh: make(chan *Event, 5),
		closeCh:  make(chan void),
	}

	e.Lock()
	e.subscriptions[sub] = struct{}{}
	e.Unlock()

	return sub
}

func (e *eventStream) unsubscribe(sub *subscription) {
	e.Lock()
	defer e.Unlock()

	delete(e.subscriptions, sub)
	close(sub.closeCh)
}

/* START ALPHA CHANGE 1294 -- prevent slow event subscribers from blocking block commitment -- */
// push adds a new Event, and notifies listeners.
//
// Delivery is intentionally non-blocking. A subscriber that does not drain its
// small buffer may miss events, but it cannot stall canonical block writes and
// consensus progress.
func (e *eventStream) push(event *Event) {
	e.RLock()
	defer e.RUnlock()

	for sub := range e.subscriptions {
		select {
		case sub.updateCh <- event:
		default:
		}
	}
}

/* FINISH ALPHA CHANGE 1294 -- prevent slow event subscribers from blocking block commitment -- */
