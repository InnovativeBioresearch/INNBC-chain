/* START ALPHA CHANGE 2259 -- test WebSocket admission cleanup and handshake deadlines -- */
package jsonrpc

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

type rpcLifecycleDispatcher struct {
	rpcAdmissionCountingDispatcher
	dispatchErr error
	panicValue  any
}

func (d *rpcLifecycleDispatcher) HandleWs([]byte, wsConn) ([]byte, error) {
	if d.panicValue != nil {
		panic(d.panicValue)
	}

	return []byte(`{"jsonrpc":"2.0","id":1,"result":true}`), d.dispatchErr
}

func TestRPCAdmissionWebSocketDispatchAlwaysReleasesSlot(t *testing.T) {
	for _, mode := range []string{"success", "dispatch error", "write error", "panic"} {
		t.Run(mode, func(t *testing.T) {
			dispatcher := &rpcLifecycleDispatcher{}
			connection := &rpcSubscriptionTestConnection{}

			switch mode {
			case "dispatch error":
				dispatcher.dispatchErr = errors.New("injected dispatch failure")
			case "write error":
				connection.err = errors.New("injected write failure")
			case "panic":
				dispatcher.panicValue = "injected dispatch panic"
			}

			server := &JSONRPC{
				dispatcher: dispatcher,
				admission:  frozenRPCAdmission(DefaultAdmissionConfig()),
			}

			require.Zero(t, server.rpcLimits().enter())

			if mode == "panic" {
				require.PanicsWithValue(t, dispatcher.panicValue, func() {
					_ = server.dispatchAdmittedWs(
						[]byte(`{}`),
						websocket.TextMessage,
						connection,
					)
				})
			} else {
				err := server.dispatchAdmittedWs(
					[]byte(`{}`),
					websocket.TextMessage,
					connection,
				)
				if mode == "success" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			}

			require.Empty(t, server.rpcLimits().active)

			// Capacity remains usable after the preceding operation.
			require.Zero(t, server.rpcLimits().enter())
			server.rpcLimits().leave()
		})
	}
}

type rpcHandshakeDeadlineConn struct {
	net.Conn
	writeDeadline     time.Time
	writeAttempted    bool
	deadlineAtAttempt bool
}

func (c *rpcHandshakeDeadlineConn) SetDeadline(deadline time.Time) error {
	c.writeDeadline = deadline

	return c.Conn.SetDeadline(deadline)
}

func (c *rpcHandshakeDeadlineConn) SetWriteDeadline(deadline time.Time) error {
	c.writeDeadline = deadline

	return c.Conn.SetWriteDeadline(deadline)
}

func (c *rpcHandshakeDeadlineConn) Write([]byte) (int, error) {
	c.writeAttempted = true
	c.deadlineAtAttempt = !c.writeDeadline.IsZero()

	// Fail immediately instead of making this test wait for the timeout.
	return 0, errors.New("injected handshake write failure")
}

type rpcHandshakeResponseWriter struct {
	*httptest.ResponseRecorder
	connection net.Conn
}

func (w *rpcHandshakeResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.connection, bufio.NewReadWriter(
		bufio.NewReader(w.connection),
		bufio.NewWriter(w.connection),
	), nil
}

func TestRPCAdmissionHandshakeHasDeadlineAndReleasesCapacity(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	connection := &rpcHandshakeDeadlineConn{Conn: local}
	writer := &rpcHandshakeResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		connection:       connection,
	}

	request := httptest.NewRequest(http.MethodGet, "http://localhost/ws", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	server := &JSONRPC{
		logger:    hclog.NewNullLogger(),
		config:    &Config{},
		admission: frozenRPCAdmission(DefaultAdmissionConfig()),
		closeCh:   make(chan struct{}),
	}

	server.handleWs(writer, request)

	require.True(t, connection.writeAttempted)
	require.True(t, connection.deadlineAtAttempt)
	require.Empty(t, server.rpcLimits().active)
	require.Empty(t, server.rpcLimits().sockets)
}

/* FINISH ALPHA CHANGE 2259 -- test WebSocket admission cleanup and handshake deadlines -- */
