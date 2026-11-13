package txpool

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

/* START ALPHA CHANGE 2559 -- test global event subscription admission and reservation lifetimes -- */

func eventLimitOperatorCount(manager *eventManager) int {
	manager.subscriptionsLock.RLock()
	defer manager.subscriptionsLock.RUnlock()

	return manager.operatorStreams
}

func TestEventLimits_ConcurrentOperatorReservations(t *testing.T) {
	t.Parallel()

	manager := newEventManager(hclog.NewNullLogger())
	t.Cleanup(manager.Close)

	type reservationResult struct {
		release func()
		err     error
	}

	const attempts = maxOperatorEventSubscriptions * 2
	results := make(chan reservationResult, attempts)
	start := make(chan struct{})

	for range attempts {
		go func() {
			<-start
			release, err := manager.reserveOperatorStream()
			results <- reservationResult{release: release, err: err}
		}()
	}
	close(start)

	releases := make([]func(), 0, maxOperatorEventSubscriptions)
	rejected := 0

	for range attempts {
		select {
		case result := <-results:
			if result.err != nil {
				require.ErrorIs(t, result.err, errEventSubscriptionLimit)
				require.Nil(t, result.release)
				rejected++
				continue
			}

			require.NotNil(t, result.release)
			releases = append(releases, result.release)
			t.Cleanup(result.release)

		case <-time.After(5 * time.Second):
			t.Fatal("operator admission did not finish")
		}
	}

	require.Len(t, releases, maxOperatorEventSubscriptions)
	require.Equal(t, maxOperatorEventSubscriptions, rejected)
	require.Equal(t, maxOperatorEventSubscriptions, eventLimitOperatorCount(manager))

	/* Releasing twice must free only one slot. */
	releases[0]()
	releases[0]()
	require.Equal(t, maxOperatorEventSubscriptions-1, eventLimitOperatorCount(manager))

	replacementRelease, err := manager.reserveOperatorStream()
	require.NoError(t, err)
	t.Cleanup(replacementRelease)
	require.Equal(t, maxOperatorEventSubscriptions, eventLimitOperatorCount(manager))

	/* Shutdown must not reset accounting for handlers that still own slots. */
	manager.Close()
	require.Equal(t, maxOperatorEventSubscriptions, eventLimitOperatorCount(manager))

	for _, release := range releases {
		release()
	}
	replacementRelease()
	replacementRelease()
	require.Zero(t, eventLimitOperatorCount(manager))

	release, err := manager.reserveOperatorStream()
	require.ErrorIs(t, err, errEventManagerClosed)
	require.Nil(t, release)
}

func TestEventLimits_ConcurrentQueuesPreserveInternalReserve(t *testing.T) {
	t.Parallel()

	manager := newEventManager(hclog.NewNullLogger())
	t.Cleanup(manager.Close)

	const attempts = maxOperatorEventSubscriptions * 2
	results := make(chan *subscribeResult, attempts)
	start := make(chan struct{})

	for range attempts {
		go func() {
			<-start
			results <- manager.subscribe([]proto.EventType{proto.EventType_ADDED})
		}()
	}
	close(start)

	admitted := make([]*subscribeResult, 0, maxOperatorEventSubscriptions)
	rejected := 0

	for range attempts {
		select {
		case result := <-results:
			if err := result.subscription.err(); err != nil {
				require.ErrorIs(t, err, errEventSubscriptionLimit)

				select {
				case _, open := <-result.subscriptionChannel:
					require.False(t, open)
				default:
					t.Fatal("rejected subscription channel remained open")
				}

				rejected++
				continue
			}

			admitted = append(admitted, result)

		case <-time.After(5 * time.Second):
			t.Fatal("queue admission did not finish")
		}
	}

	require.Len(t, admitted, maxOperatorEventSubscriptions)
	require.Equal(t, maxOperatorEventSubscriptions, rejected)

	pool := &TxPool{eventManager: manager}
	request := &proto.SubscribeRequest{
		Types: []proto.EventType{proto.EventType_ADDED},
	}

	/* A full ordinary pool must still admit the internal RPC feed. */
	events, cancel, err := pool.TxPoolSubscribe(request)
	require.NoError(t, err)
	require.NotNil(t, events)
	require.NotNil(t, cancel)
	t.Cleanup(cancel)

	manager.subscriptionsLock.RLock()
	total := len(manager.subscriptions)
	manager.subscriptionsLock.RUnlock()
	require.Equal(t, maxOperatorEventSubscriptions+1, total)

	duplicateEvents, duplicateCancel, err := pool.TxPoolSubscribe(request)
	require.ErrorIs(t, err, errEventSubscriptionLimit)
	require.Nil(t, duplicateEvents)
	require.Nil(t, duplicateCancel)

	/* Internal cancellation allows reconnection while ordinary capacity is full. */
	cancel()
	cancel()

	replacementEvents, replacementCancel, err := pool.TxPoolSubscribe(request)
	require.NoError(t, err)
	require.NotNil(t, replacementEvents)
	require.NotNil(t, replacementCancel)
	t.Cleanup(replacementCancel)

	cancel()
	select {
	case <-replacementEvents:
		t.Fatal("stale cancellation closed the replacement internal feed")
	default:
	}

	/* Ordinary cancellation also restores exactly one queue slot. */
	original := admitted[0]
	manager.cancelSubscription(original.subscriptionID, original.subscription)

	replacement := manager.subscribe([]proto.EventType{proto.EventType_ADDED})
	require.NoError(t, replacement.subscription.err())

	excess := manager.subscribe([]proto.EventType{proto.EventType_ADDED})
	require.ErrorIs(t, excess.subscription.err(), errEventSubscriptionLimit)

	manager.Close()

	events, cancel, err = pool.TxPoolSubscribe(request)
	require.ErrorIs(t, err, errEventManagerClosed)
	require.Nil(t, events)
	require.Nil(t, cancel)
}

func TestEventLimits_OverflowKeepsStalledOperatorSlot(t *testing.T) {
	t.Parallel()

	manager := newEventManager(hclog.NewNullLogger())
	t.Cleanup(manager.Close)

	/* Reserve all but one operator slot without creating extra event queues. */
	for range maxOperatorEventSubscriptions - 1 {
		release, err := manager.reserveOperatorStream()
		require.NoError(t, err)
		t.Cleanup(release)
	}

	pool := &TxPool{eventManager: manager}
	request := &proto.SubscribeRequest{
		Types: []proto.EventType{proto.EventType_ADDED},
	}
	stream := &backlogBlockedStream{
		ctx:     context.Background(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}

	var releaseOnce sync.Once
	releaseSend := func() {
		releaseOnce.Do(func() { close(stream.release) })
	}
	t.Cleanup(releaseSend)

	result := make(chan error, 1)
	go func() {
		result <- pool.Subscribe(request, stream)
	}()

	require.Eventually(t, func() bool {
		manager.subscriptionsLock.RLock()
		defer manager.subscriptionsLock.RUnlock()

		return len(manager.subscriptions) == 1 &&
			manager.operatorStreams == maxOperatorEventSubscriptions
	}, 5*time.Second, time.Millisecond)

	hash := types.StringToHash("1")
	manager.signalEvent(proto.EventType_ADDED, hash)

	select {
	case <-stream.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("operator send did not start")
	}

	for range maxQueuedTxPoolEvents + 2 {
		manager.signalEvent(proto.EventType_ADDED, hash)
	}

	require.Eventually(t, func() bool {
		manager.subscriptionsLock.RLock()
		defer manager.subscriptionsLock.RUnlock()

		return len(manager.subscriptions) == 0
	}, 5*time.Second, time.Millisecond)

	require.Equal(t, maxOperatorEventSubscriptions, eventLimitOperatorCount(manager))

	/* A cancelled context keeps a faulty admission path from hanging this test. */
	rejectedContext, cancelContext := context.WithCancel(context.Background())
	cancelContext()

	err := pool.Subscribe(request, &backlogBlockedStream{ctx: rejectedContext})
	require.Equal(t, codes.ResourceExhausted, grpcstatus.Code(err))

	/* Operator saturation must not consume the internal reservation. */
	events, cancelInternal, err := pool.TxPoolSubscribe(request)
	require.NoError(t, err)
	require.NotNil(t, events)
	require.NotNil(t, cancelInternal)
	t.Cleanup(cancelInternal)
	cancelInternal()

	releaseSend()

	select {
	case err := <-result:
		require.Equal(t, codes.ResourceExhausted, grpcstatus.Code(err))
	case <-time.After(5 * time.Second):
		t.Fatal("overflowed operator handler did not return")
	}

	require.Equal(t, maxOperatorEventSubscriptions-1, eventLimitOperatorCount(manager))

	release, err := manager.reserveOperatorStream()
	require.NoError(t, err)
	t.Cleanup(release)
	require.Equal(t, maxOperatorEventSubscriptions, eventLimitOperatorCount(manager))
}

/* FINISH ALPHA CHANGE 2559 -- test global event subscription admission and reservation lifetimes -- */
