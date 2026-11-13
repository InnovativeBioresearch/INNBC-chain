package txpool

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
)

/* START ALPHA CHANGE 2558 -- bound operator subscriptions and reserve the internal feed -- */

const maxOperatorEventSubscriptions = 32

type eventManager struct {
	subscriptions     map[subscriptionID]*eventSubscription
	subscriptionsLock sync.RWMutex
	numSubscriptions  int64
	logger            hclog.Logger

	closed bool

	/* Protected by subscriptionsLock. */
	operatorStreams      int
	internalSubscription *eventSubscription
}

/* FINISH ALPHA CHANGE 2558 -- bound operator subscriptions and reserve the internal feed -- */

func newEventManager(logger hclog.Logger) *eventManager {
	return &eventManager{
		logger:           logger.Named("event-manager"),
		subscriptions:    make(map[subscriptionID]*eventSubscription),
		numSubscriptions: 0,
	}
}

/* START ALPHA CHANGE 2543 -- expose subscription completion to its consumers -- */
type subscribeResult struct {
	subscriptionID      subscriptionID
	subscriptionChannel chan *proto.TxPoolEvent
	subscription        *eventSubscription
}

/* FINISH ALPHA CHANGE 2543 -- expose subscription completion to its consumers -- */

/* START ALPHA CHANGE 2558 -- enforce queue admission and operator handler lifetime limits -- */

func (em *eventManager) reserveOperatorStream() (func(), error) {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	if em.closed {
		return nil, errEventManagerClosed
	}

	if em.operatorStreams >= maxOperatorEventSubscriptions {
		return nil, errEventSubscriptionLimit
	}

	em.operatorStreams++

	var once sync.Once
	release := func() {
		once.Do(func() {
			em.subscriptionsLock.Lock()
			defer em.subscriptionsLock.Unlock()

			em.operatorStreams--
		})
	}

	return release, nil
}

func (em *eventManager) subscribe(eventTypes []proto.EventType) *subscribeResult {
	return em.subscribeWithRole(eventTypes, false)
}

func (em *eventManager) subscribeInternal(eventTypes []proto.EventType) *subscribeResult {
	return em.subscribeWithRole(eventTypes, true)
}

func (em *eventManager) subscribeWithRole(
	eventTypes []proto.EventType,
	internal bool,
) *subscribeResult {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	subscription := &eventSubscription{
		eventTypes: append([]proto.EventType(nil), eventTypes...),
		outputCh:   make(chan *proto.TxPoolEvent),
		doneCh:     make(chan struct{}),
		notifyCh:   make(chan struct{}, 1),
		eventStore: &eventQueue{},
	}

	result := &subscribeResult{
		subscriptionChannel: subscription.outputCh,
		subscription:        subscription,
	}

	var admissionErr error

	switch {
	case em.closed:
		admissionErr = errEventManagerClosed

	case internal:
		if em.internalSubscription != nil {
			admissionErr = errEventSubscriptionLimit
		}

	default:
		ordinaryCount := len(em.subscriptions)
		if em.internalSubscription != nil {
			ordinaryCount--
		}

		if ordinaryCount >= maxOperatorEventSubscriptions {
			admissionErr = errEventSubscriptionLimit
		}
	}

	if admissionErr != nil {
		subscription.stop(admissionErr)
		close(subscription.outputCh)
		return result
	}

	id := subscriptionID(uuid.New().ID())
	for em.subscriptions[id] != nil {
		id = subscriptionID(uuid.New().ID())
	}

	result.subscriptionID = id
	em.subscriptions[id] = subscription
	if internal {
		em.internalSubscription = subscription
	}
	atomic.AddInt64(&em.numSubscriptions, 1)

	go func() {
		defer em.cancelSubscription(id, subscription)
		subscription.runLoop()
	}()

	em.logger.Info(fmt.Sprintf("Added new subscription %d", id))

	return result
}

/* FINISH ALPHA CHANGE 2558 -- enforce queue admission and operator handler lifetime limits -- */

/*
	START ALPHA CHANGE 2551 -- cancel only the subscription owned by the caller --

An ID can be reused after its previous subscription finishes. Delayed cleanup
must not close or remove the newer subscription registered under that ID.

FINISH ALPHA CHANGE 2551 -- cancel only the subscription owned by the caller --
*/
func (em *eventManager) cancelSubscription(
	id subscriptionID,
	expected *eventSubscription,
) {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	subscription, ok := em.subscriptions[id]
	if !ok || expected == nil || subscription != expected {
		return
	}

	subscription.close()
	em.logger.Info(fmt.Sprintf("Canceled subscription %d", id))
	delete(em.subscriptions, id)

	/* START ALPHA CHANGE 2558 -- release only the matching internal reservation -- */
	if em.internalSubscription == subscription {
		em.internalSubscription = nil
	}
	/* FINISH ALPHA CHANGE 2558 -- release only the matching internal reservation -- */

	atomic.AddInt64(&em.numSubscriptions, -1)
}

/* START ALPHA CHANGE 2543 -- make event-manager shutdown terminal -- */

func (em *eventManager) Close() {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	em.closed = true

	for id, subscription := range em.subscriptions {
		subscription.stop(errEventManagerClosed)
		delete(em.subscriptions, id)
	}

	/* START ALPHA CHANGE 2558 -- preserve operator reservations until handlers return --

	Shutdown clears queue registrations. Operator reservations remain owned
	by their handlers and are released by those handlers' deferred cleanup.

	FINISH ALPHA CHANGE 2558 -- preserve operator reservations until handlers return -- */
	em.internalSubscription = nil

	atomic.StoreInt64(&em.numSubscriptions, 0)
}

/* FINISH ALPHA CHANGE 2543 -- make event-manager shutdown terminal -- */

// signalEvent is a helper method for alerting listeners of a new TxPool event
func (em *eventManager) signalEvent(eventType proto.EventType, txHashes ...types.Hash) {
	if atomic.LoadInt64(&em.numSubscriptions) < 1 {
		// No reason to lock the subscriptions map
		// if no subscriptions exist
		return
	}

	em.subscriptionsLock.RLock()
	defer em.subscriptionsLock.RUnlock()

	for _, txHash := range txHashes {
		for _, subscription := range em.subscriptions {
			subscription.pushEvent(&proto.TxPoolEvent{
				Type:   eventType,
				TxHash: txHash.String(),
			})
		}
	}
}
