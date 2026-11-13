package txpool

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2553 -- test cancellation after deterministic subscription ID reuse --

The replacement is registered without a worker so tests can inspect its queue
directly. This forces ID reuse without relying on random ID collisions.

FINISH ALPHA CHANGE 2553 -- test cancellation after deterministic subscription ID reuse -- */

func backlogInstallReusedID(
	t *testing.T,
	manager *eventManager,
	id subscriptionID,
) *eventSubscription {
	t.Helper()

	replacement := &eventSubscription{
		eventTypes: []proto.EventType{proto.EventType_ADDED},
		outputCh:   make(chan *proto.TxPoolEvent),
		doneCh:     make(chan struct{}),
		notifyCh:   make(chan struct{}, 1),
		eventStore: &eventQueue{},
	}

	manager.subscriptionsLock.Lock()
	if manager.closed || manager.subscriptions[id] != nil {
		manager.subscriptionsLock.Unlock()
		t.Fatal("replacement requires an open manager and a retired subscription ID")
	}

	manager.subscriptions[id] = replacement
	atomic.AddInt64(&manager.numSubscriptions, 1)
	manager.subscriptionsLock.Unlock()

	return replacement
}

func backlogRequireCurrentSubscription(
	t *testing.T,
	manager *eventManager,
	id subscriptionID,
	expected *eventSubscription,
) {
	t.Helper()

	manager.subscriptionsLock.RLock()
	current := manager.subscriptions[id]
	count := atomic.LoadInt64(&manager.numSubscriptions)
	manager.subscriptionsLock.RUnlock()

	require.Same(t, expected, current)
	require.Equal(t, int64(1), count)

	select {
	case <-expected.doneCh:
		t.Fatal("replacement subscription was unexpectedly stopped")
	default:
	}
}

func TestEventBacklog_StaleInternalCancelPreservesReusedID(t *testing.T) {
	t.Parallel()

	manager := newEventManager(hclog.NewNullLogger())
	t.Cleanup(manager.Close)

	pool := &TxPool{eventManager: manager}
	events, cancel, err := pool.TxPoolSubscribe(
		&proto.SubscribeRequest{
			Types: []proto.EventType{proto.EventType_ADDED},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, cancel)

	var id subscriptionID
	var original *eventSubscription

	manager.subscriptionsLock.RLock()
	for currentID, subscription := range manager.subscriptions {
		if subscription.outputCh == events {
			id = currentID
			original = subscription
			break
		}
	}
	manager.subscriptionsLock.RUnlock()
	require.NotNil(t, original)

	hash := types.StringToHash("1")
	for range maxQueuedTxPoolEvents + 2 {
		manager.signalEvent(proto.EventType_ADDED, hash)
	}

	require.Eventually(t, func() bool {
		manager.subscriptionsLock.RLock()
		defer manager.subscriptionsLock.RUnlock()

		return len(manager.subscriptions) == 0 &&
			atomic.LoadInt64(&manager.numSubscriptions) == 0
	}, 5*time.Second, time.Millisecond)
	require.ErrorIs(t, original.err(), errEventQueueOverflow)

	replacement := backlogInstallReusedID(t, manager, id)

	/* Delayed or repeated cancellation must leave the new owner untouched. */
	cancel()
	cancel()
	manager.cancelSubscription(id, original)
	manager.cancelSubscription(id, nil)

	backlogRequireCurrentSubscription(t, manager, id, replacement)

	manager.signalEvent(proto.EventType_ADDED, hash)
	event := replacement.eventStore.pop()
	require.NotNil(t, event)
	require.Equal(t, hash.String(), event.TxHash)

	/* Cancellation by the current owner still works and remains idempotent. */
	manager.cancelSubscription(id, replacement)
	manager.cancelSubscription(id, replacement)

	manager.subscriptionsLock.RLock()
	remaining := len(manager.subscriptions)
	count := atomic.LoadInt64(&manager.numSubscriptions)
	manager.subscriptionsLock.RUnlock()

	require.Zero(t, remaining)
	require.Zero(t, count)

	select {
	case <-replacement.doneCh:
	default:
		t.Fatal("current owner cancellation did not stop the subscription")
	}
}
