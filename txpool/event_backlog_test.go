package txpool

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	/* START ALPHA CHANGE 2550 -- avoid the existing test status type -- */
	grpcstatus "google.golang.org/grpc/status"
	/* FINISH ALPHA CHANGE 2550 -- avoid the existing test status type -- */)

/* START ALPHA CHANGE 2548 -- test bounded event backlogs and stalled stream cleanup -- */

func TestEventBacklog_QueueBoundAndOrder(t *testing.T) {
	t.Parallel()

	queue := &eventQueue{}

	for cycle := range 3 {
		for i := range maxQueuedTxPoolEvents {
			require.True(t, queue.push(&proto.TxPoolEvent{
				TxHash: fmt.Sprintf("%d/%d", cycle, i),
			}))
		}
		require.False(t, queue.push(&proto.TxPoolEvent{}))

		for i := range maxQueuedTxPoolEvents {
			event := queue.pop()
			require.NotNil(t, event)
			require.Equal(t, fmt.Sprintf("%d/%d", cycle, i), event.TxHash)
		}
		require.Nil(t, queue.pop())
	}

	for _, event := range queue.events {
		require.Nil(t, event)
	}

	queue.close()
	require.Nil(t, queue.events)
	require.False(t, queue.push(&proto.TxPoolEvent{}))
}

func TestEventBacklog_OverflowReleasesQueue(t *testing.T) {
	t.Parallel()

	subscription := &eventSubscription{
		eventTypes: []proto.EventType{proto.EventType_ADDED},
		outputCh:   make(chan *proto.TxPoolEvent),
		doneCh:     make(chan struct{}),
		notifyCh:   make(chan struct{}, 1),
		eventStore: &eventQueue{},
	}

	for range maxQueuedTxPoolEvents {
		subscription.pushEvent(&proto.TxPoolEvent{Type: proto.EventType_ADDED})
	}

	require.Equal(t, maxQueuedTxPoolEvents, subscription.eventStore.count)
	require.NoError(t, subscription.err())

	subscription.pushEvent(&proto.TxPoolEvent{Type: proto.EventType_ADDED})

	require.ErrorIs(t, subscription.err(), errEventQueueOverflow)
	require.Nil(t, subscription.eventStore.events)
	require.Zero(t, subscription.eventStore.count)

	subscription.pushEvent(&proto.TxPoolEvent{Type: proto.EventType_ADDED})
	require.Nil(t, subscription.eventStore.events)

	go subscription.runLoop()
	select {
	case _, more := <-subscription.outputCh:
		require.False(t, more)
	case <-time.After(5 * time.Second):
		t.Fatal("overflowed subscription did not close its output")
	}
}

func TestEventBacklog_ConcurrentStopAndPublish(t *testing.T) {
	t.Parallel()

	subscription := &eventSubscription{
		eventTypes: []proto.EventType{proto.EventType_ADDED},
		outputCh:   make(chan *proto.TxPoolEvent),
		doneCh:     make(chan struct{}),
		notifyCh:   make(chan struct{}, 1),
		eventStore: &eventQueue{},
	}

	var workers sync.WaitGroup
	for range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range maxQueuedTxPoolEvents {
				subscription.pushEvent(&proto.TxPoolEvent{Type: proto.EventType_ADDED})
			}
		}()
	}

	subscription.close()
	workers.Wait()

	subscription.eventStore.Lock()
	retained := subscription.eventStore.count
	storageReleased := subscription.eventStore.events == nil
	subscription.eventStore.Unlock()

	require.Zero(t, retained)
	require.True(t, storageReleased)
}

type backlogBlockedStream struct {
	grpc.ServerStream
	ctx     context.Context
	entered chan struct{}
	release chan struct{}
}

func (s *backlogBlockedStream) Context() context.Context {
	return s.ctx
}

func (s *backlogBlockedStream) Send(*proto.TxPoolEvent) error {
	close(s.entered)
	<-s.release
	return nil
}

func TestEventBacklog_StalledGRPCSendDoesNotRetainQueue(t *testing.T) {
	t.Parallel()

	manager := newEventManager(hclog.NewNullLogger())
	t.Cleanup(manager.Close)

	pool := &TxPool{eventManager: manager}
	stream := &backlogBlockedStream{
		ctx:     context.Background(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(stream.release) })
	}
	t.Cleanup(release)

	result := make(chan error, 1)
	go func() {
		result <- pool.Subscribe(
			&proto.SubscribeRequest{Types: []proto.EventType{proto.EventType_ADDED}},
			stream,
		)
	}()

	/* START ALPHA CHANGE 2553 -- capture the ID retained by a stalled stream -- */
	var id subscriptionID
	var subscription *eventSubscription
	require.Eventually(t, func() bool {
		manager.subscriptionsLock.RLock()
		defer manager.subscriptionsLock.RUnlock()

		for currentID, current := range manager.subscriptions {
			id = currentID
			subscription = current
			return true
		}
		return false
	}, 5*time.Second, time.Millisecond)
	/* FINISH ALPHA CHANGE 2553 -- capture the ID retained by a stalled stream -- */

	hash := types.StringToHash("1")
	manager.signalEvent(proto.EventType_ADDED, hash)

	select {
	case <-stream.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("stream send did not start")
	}

	for range maxQueuedTxPoolEvents + 2 {
		manager.signalEvent(proto.EventType_ADDED, hash)
	}

	require.Eventually(t, func() bool {
		manager.subscriptionsLock.RLock()
		defer manager.subscriptionsLock.RUnlock()
		return len(manager.subscriptions) == 0
	}, 5*time.Second, time.Millisecond)

	require.ErrorIs(t, subscription.err(), errEventQueueOverflow)

	subscription.eventStore.Lock()
	retained := subscription.eventStore.count
	storageReleased := subscription.eventStore.events == nil
	subscription.eventStore.Unlock()

	require.Zero(t, retained)
	require.True(t, storageReleased)

	/* START ALPHA CHANGE 2553 -- reuse the retired ID before delayed stream cleanup -- */
	replacement := backlogInstallReusedID(t, manager, id)
	/* FINISH ALPHA CHANGE 2553 -- reuse the retired ID before delayed stream cleanup -- */

	release()
	select {
	case err := <-result:
		/* START ALPHA CHANGE 2550 -- inspect errors through the aliased package -- */
		require.Equal(t, codes.ResourceExhausted, grpcstatus.Code(err))
		/* FINISH ALPHA CHANGE 2550 -- inspect errors through the aliased package -- */
	case <-time.After(5 * time.Second):
		t.Fatal("gRPC handler did not finish after send resumed")
	}

	/* START ALPHA CHANGE 2553 -- confirm delayed stream cleanup preserves the replacement -- */
	backlogRequireCurrentSubscription(t, manager, id, replacement)

	manager.signalEvent(proto.EventType_ADDED, hash)
	event := replacement.eventStore.pop()
	require.NotNil(t, event)
	require.Equal(t, hash.String(), event.TxHash)
	/* FINISH ALPHA CHANGE 2553 -- confirm delayed stream cleanup preserves the replacement -- */
}

func TestEventBacklog_ManagerShutdownRejectsNewSubscription(t *testing.T) {
	t.Parallel()

	manager := newEventManager(hclog.NewNullLogger())
	manager.Close()

	pool := &TxPool{eventManager: manager}
	events, cancel, err := pool.TxPoolSubscribe(
		&proto.SubscribeRequest{Types: []proto.EventType{proto.EventType_ADDED}},
	)

	require.ErrorIs(t, err, errEventManagerClosed)
	require.Nil(t, events)
	require.Nil(t, cancel)
	require.Empty(t, manager.subscriptions)
}

/* FINISH ALPHA CHANGE 2548 -- test bounded event backlogs and stalled stream cleanup -- */
