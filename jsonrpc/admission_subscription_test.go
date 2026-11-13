/* START ALPHA CHANGE 2256 -- test rejected subscriptions and terminal delivery cleanup -- */
package jsonrpc

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestRPCAdmissionRejectedSubscriptionPreservesMetadata(t *testing.T) {
	for _, kind := range []string{"block", "log", "pending"} {
		t.Run(kind, func(t *testing.T) {
			manager := NewFilterManager(
				hclog.NewNullLogger(),
				newMockStore(),
				256,
			)
			defer manager.Close()
			manager.maxFilters = 1

			connection := &wsWrapper{}
			existing := manager.NewBlockFilter(connection)
			require.NotEmpty(t, existing)
			require.Equal(t, existing, connection.GetFilterID())

			var rejected string
			switch kind {
			case "block":
				rejected = manager.NewBlockFilter(connection)
			case "log":
				rejected = manager.NewLogFilter(&LogQuery{}, connection)
			case "pending":
				rejected = manager.NewPendingTxFilter(connection)
			}

			require.Empty(t, rejected)
			require.Equal(t, existing, connection.GetFilterID())
			require.True(t, manager.Exists(existing))

			manager.RemoveFilterByWs(connection)
			require.False(t, manager.Exists(existing))

			var admitted string
			switch kind {
			case "block":
				admitted = manager.NewBlockFilter(connection)
			case "log":
				admitted = manager.NewLogFilter(&LogQuery{}, connection)
			case "pending":
				admitted = manager.NewPendingTxFilter(connection)
			}

			require.NotEmpty(t, admitted)
			require.Equal(t, admitted, connection.GetFilterID())
			require.True(t, manager.Exists(admitted))
		})
	}
}

type rpcSubscriptionTestConnection struct {
	id     string
	err    error
	writes int
}

func (c *rpcSubscriptionTestConnection) SetFilterID(id string) {
	c.id = id
}

func (c *rpcSubscriptionTestConnection) GetFilterID() string {
	return c.id
}

func (c *rpcSubscriptionTestConnection) WriteMessage(int, []byte) error {
	c.writes++

	return c.err
}

type rpcSubscriptionTimeout struct{}

func (rpcSubscriptionTimeout) Error() string   { return "injected write timeout" }
func (rpcSubscriptionTimeout) Timeout() bool   { return true }
func (rpcSubscriptionTimeout) Temporary() bool { return true }

func TestRPCAdmissionFailedFlushRemovesAllConnectionSubscriptions(t *testing.T) {
	for _, cause := range []error{
		rpcSubscriptionTimeout{},
		errors.New("injected write failure"),
		net.ErrClosed,
		websocket.ErrCloseSent,
	} {
		t.Run(cause.Error(), func(t *testing.T) {
			manager := NewFilterManager(
				hclog.NewNullLogger(),
				newMockStore(),
				256,
			)
			defer manager.Close()

			bad := &rpcSubscriptionTestConnection{err: cause}
			good := &rpcSubscriptionTestConnection{}

			first := manager.NewPendingTxFilter(bad)
			second := manager.NewPendingTxFilter(bad)
			otherType := manager.NewBlockFilter(bad)
			healthy := manager.NewPendingTxFilter(good)
			require.NotEmpty(t, first)
			require.NotEmpty(t, second)
			require.NotEmpty(t, otherType)
			require.NotEmpty(t, healthy)

			// Queue notifications without starting background filter workers.
			for _, id := range []string{first, second, healthy} {
				manager.filters[id].(*pendingTxFilter).
					appendPendingTxHashes("0x01")
			}

			require.NoError(t, manager.flushWsFilters(PendingTransactions))

			require.False(t, manager.Exists(first))
			require.False(t, manager.Exists(second))
			require.False(t, manager.Exists(otherType))
			require.True(t, manager.Exists(healthy))

			// At most one attempt on the failed connection in this flush.
			require.Equal(t, 1, bad.writes)
			require.Equal(t, 1, good.writes)

			require.NoError(t, manager.flushWsFilters(PendingTransactions))
			require.Equal(t, 1, bad.writes)
		})
	}
}

func TestRPCAdmissionSubscriptionFailureLogsAreBounded(t *testing.T) {
	var output bytes.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &output,
		Level:  hclog.Warn,
	})
	manager := NewFilterManager(logger, newMockStore(), 256)
	defer manager.Close()

	// One token and no refill make this test independent of elapsed time.
	manager.flushLogLimiter = rate.NewLimiter(0, 1)

	for i := 0; i < 3; i++ {
		connection := &rpcSubscriptionTestConnection{
			err: errors.New("injected delivery failure"),
		}
		id := manager.NewPendingTxFilter(connection)
		require.NotEmpty(t, id)
		manager.filters[id].(*pendingTxFilter).appendPendingTxHashes("0x01")

		require.NoError(t, manager.flushWsFilters(PendingTransactions))
		require.False(t, manager.Exists(id))
	}

	require.Equal(t, 1, strings.Count(
		output.String(),
		"removed subscriptions after delivery failure",
	))
	require.Equal(t, uint64(2), manager.flushSuppressed)
}

type rpcFailingWriteConn struct {
	net.Conn
	fail   atomic.Bool
	closed atomic.Bool
}

func (c *rpcFailingWriteConn) Write(data []byte) (int, error) {
	if c.fail.Load() {
		return 0, rpcSubscriptionTimeout{}
	}

	return c.Conn.Write(data)
}

func (c *rpcFailingWriteConn) Close() error {
	c.closed.Store(true)

	return c.Conn.Close()
}

func TestRPCAdmissionWebSocketWriteFailureClosesTransport(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()

		_, _, _ = connection.ReadMessage()
	}))
	defer server.Close()

	var transport *rpcFailingWriteConn
	dialer := websocket.Dialer{
		NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			connection, err := (&net.Dialer{}).DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}

			transport = &rpcFailingWriteConn{Conn: connection}

			return transport, nil
		},
	}

	connection, response, err := dialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"),
		nil,
	)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	require.NoError(t, err)
	defer connection.Close()
	require.NotNil(t, transport)

	transport.fail.Store(true)
	wrapper := &wsWrapper{ws: connection}

	err = wrapper.WriteMessage(websocket.TextMessage, []byte("notification"))
	require.Error(t, err)

	var timeout net.Error
	require.ErrorAs(t, err, &timeout)
	require.True(t, timeout.Timeout())
	require.True(t, transport.closed.Load())
}

/* FINISH ALPHA CHANGE 2256 -- test rejected subscriptions and terminal delivery cleanup -- */
