package jsonrpc

import (
	"container/heap"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2549 -- test RPC survival and recovery after transaction-feed loss -- */

type recoveryEventStream struct {
	events chan *proto.TxPoolEvent
	once   sync.Once
}

func (s *recoveryEventStream) close() {
	s.once.Do(func() { close(s.events) })
}

type recoveryEventStore struct {
	*mockStore
	first    *recoveryEventStream
	second   *recoveryEventStream
	attempts atomic.Int32
	fail     atomic.Bool
}

func (s *recoveryEventStore) TxPoolSubscribe(
	*proto.SubscribeRequest,
) (<-chan *proto.TxPoolEvent, func(), error) {
	if s.attempts.Add(1) == 1 {
		return s.first.events, s.first.close, nil
	}

	if s.fail.Load() {
		return nil, nil, errors.New("injected subscription failure")
	}

	return s.second.events, s.second.close, nil
}

type recoveryWSConnection struct {
	mu        sync.Mutex
	id        string
	closed    chan struct{}
	delivered chan struct{}
	once      sync.Once
}

func newRecoveryWSConnection() *recoveryWSConnection {
	return &recoveryWSConnection{
		closed:    make(chan struct{}),
		delivered: make(chan struct{}, 4),
	}
}

func (c *recoveryWSConnection) SetFilterID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.id = id
}

func (c *recoveryWSConnection) GetFilterID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.id
}

func (c *recoveryWSConnection) WriteMessage(int, []byte) error {
	select {
	case c.delivered <- struct{}{}:
	default:
	}
	return nil
}

func (c *recoveryWSConnection) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func TestTxEventRecovery_KeepsExpiryAndRestoresPendingDelivery(t *testing.T) {
	t.Parallel()

	store := &recoveryEventStore{
		mockStore: newMockStore(),
		first: &recoveryEventStream{
			events: make(chan *proto.TxPoolEvent),
		},
		second: &recoveryEventStream{
			events: make(chan *proto.TxPoolEvent),
		},
	}
	store.fail.Store(true)

	manager := NewFilterManager(hclog.NewNullLogger(), store, 256)
	t.Cleanup(manager.Close)

	affected := newRecoveryWSConnection()
	pendingID := manager.NewPendingTxFilter(affected)
	sharedBlockID := manager.NewBlockFilter(affected)
	unrelatedID := manager.NewLogFilter(&LogQuery{}, nil)

	require.NotEmpty(t, pendingID)
	require.NotEmpty(t, sharedBlockID)
	require.NotEmpty(t, unrelatedID)

	go manager.Run()

	require.Eventually(t, func() bool {
		return store.attempts.Load() >= 1
	}, 5*time.Second, time.Millisecond)

	store.first.close()

	select {
	case <-affected.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("affected pending-transaction connection was not closed")
	}

	require.False(t, manager.Exists(pendingID))
	require.False(t, manager.Exists(sharedBlockID))
	require.True(t, manager.Exists(unrelatedID))
	require.Empty(t, manager.NewPendingTxFilter(nil))

	require.Eventually(t, func() bool {
		return store.attempts.Load() >= 2
	}, 5*time.Second, 10*time.Millisecond)

	/* Filter expiry must continue while reconnect attempts fail. */
	expiringID := manager.NewBlockFilter(nil)
	require.NotEmpty(t, expiringID)

	manager.Lock()
	base := manager.filters[expiringID].getFilterBase()
	base.expiresAt = time.Now().UTC().Add(-time.Second)
	heap.Fix(&manager.timeouts, base.heapIndex)
	manager.emitSignalToUpdateCh()
	manager.Unlock()

	require.Eventually(t, func() bool {
		return !manager.Exists(expiringID)
	}, 5*time.Second, 10*time.Millisecond)
	require.True(t, manager.Exists(unrelatedID))

	store.fail.Store(false)

	require.Eventually(t, func() bool {
		manager.RLock()
		defer manager.RUnlock()
		return !manager.txEventsUnavailable
	}, 5*time.Second, 10*time.Millisecond)

	recovered := newRecoveryWSConnection()
	require.NotEmpty(t, manager.NewPendingTxFilter(recovered))

	select {
	case store.second.events <- &proto.TxPoolEvent{
		Type:   proto.EventType_ADDED,
		TxHash: "0x01",
	}:
	case <-time.After(5 * time.Second):
		t.Fatal("recovered worker did not consume transaction event")
	}

	select {
	case <-recovered.delivered:
	case <-time.After(5 * time.Second):
		t.Fatal("pending notification was not delivered after recovery")
	}

	require.True(t, manager.Exists(unrelatedID))
}

func TestTxEventRecovery_ShutdownCancelsRetry(t *testing.T) {
	t.Parallel()

	store := &recoveryEventStore{
		mockStore: newMockStore(),
		first: &recoveryEventStream{
			events: make(chan *proto.TxPoolEvent),
		},
		second: &recoveryEventStream{
			events: make(chan *proto.TxPoolEvent),
		},
	}

	manager := NewFilterManager(hclog.NewNullLogger(), store, 256)
	t.Cleanup(manager.Close)

	feed := newFilterTxFeed(manager)
	t.Cleanup(feed.close)

	feed.lost()
	require.NotNil(t, feed.retryCh)

	manager.Close()
	attempts := store.attempts.Load()
	feed.connect()

	require.Equal(t, attempts, store.attempts.Load())
	require.Nil(t, feed.retryCh)
}

/* FINISH ALPHA CHANGE 2549 -- test RPC survival and recovery after transaction-feed loss -- */
