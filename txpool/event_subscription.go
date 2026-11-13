package txpool

import (
	/* START ALPHA CHANGE 2542 -- report terminal event subscription failures -- */
	"errors"
	/* FINISH ALPHA CHANGE 2542 -- report terminal event subscription failures -- */
	"sync"

	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
)

/* START ALPHA CHANGE 2542 -- distinguish overflow from manager shutdown -- */
var (
	errEventQueueOverflow = errors.New("transaction-event subscription queue capacity exceeded")
	errEventManagerClosed = errors.New("transaction-event manager is closed")

	/* START ALPHA CHANGE 2558 -- report subscription admission limits -- */
	errEventSubscriptionLimit = errors.New("transaction-event subscription capacity exceeded")
	/* FINISH ALPHA CHANGE 2558 -- report subscription admission limits -- */
)

/* FINISH ALPHA CHANGE 2542 -- distinguish overflow from manager shutdown -- */

type subscriptionID int32

type eventSubscription struct {
	// eventTypes is the list of subscribed event types
	eventTypes []proto.EventType

	// outputCh is the update channel for the subscriber
	outputCh chan *proto.TxPoolEvent

	// doneCh is the channel for handling stop signals
	doneCh chan struct{}

	/* START ALPHA CHANGE 2542 -- publish terminal state before closing doneCh -- */
	closeOnce sync.Once
	stopErr   error
	/* FINISH ALPHA CHANGE 2542 -- publish terminal state before closing doneCh -- */

	// notifyCh is the channel for receiving event requests
	notifyCh chan struct{}

	// eventStore is used for temporary concurrent event storage,
	// required in order to preserve the chronological order of events
	eventStore *eventQueue
}

// eventSupported checks if the event is supported by the subscription
func (es *eventSubscription) eventSupported(eventType proto.EventType) bool {
	for _, supportedType := range es.eventTypes {
		if supportedType == eventType {
			return true
		}
	}

	return false
}

/* START ALPHA CHANGE 2542 -- release queued events on terminal subscription closure -- */

func (es *eventSubscription) close() {
	es.stop(nil)
}

func (es *eventSubscription) stop(err error) {
	es.closeOnce.Do(func() {
		if es.eventStore != nil {
			es.eventStore.close()
		}

		es.stopErr = err
		close(es.doneCh)
	})
}

func (es *eventSubscription) err() error {
	select {
	case <-es.doneCh:
		return es.stopErr
	default:
		return nil
	}
}

/* FINISH ALPHA CHANGE 2542 -- release queued events on terminal subscription closure -- */

// runLoop is the main loop that listens for notifications and handles the event / close signals
func (es *eventSubscription) runLoop() {
	defer close(es.outputCh)

	for {
		select {
		case <-es.doneCh: // Break if a close signal has been received
			return
		case <-es.notifyCh: // Listen for new events to appear
			for {
				// Grab the next event to be processed by order of sending
				event := es.eventStore.pop()
				if event == nil {
					break
				}

				select {
				case <-es.doneCh: // Break if a close signal has been received
					return
				case es.outputCh <- event: // Pass the event to the output
				}
			}
		}
	}
}

/* START ALPHA CHANGE 2542 -- terminate overflowing subscriptions without blocking producers -- */

func (es *eventSubscription) pushEvent(event *proto.TxPoolEvent) {
	if !es.eventSupported(event.Type) {
		return
	}

	select {
	case <-es.doneCh:
		return
	default:
	}

	if !es.eventStore.push(event) {
		es.stop(errEventQueueOverflow)
		return
	}

	select {
	case es.notifyCh <- struct{}{}:
	default:
	}
}

/* FINISH ALPHA CHANGE 2542 -- terminate overflowing subscriptions without blocking producers -- */
