/* START ALPHA CHANGE 3163 -- verify forced HTTP closure releases incomplete request bodies before later cleanup -- */
package jsonrpc

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

type shutdownBodyReader struct {
	io.ReadCloser
	started chan struct{}
	once    sync.Once
}

func (r *shutdownBodyReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.ReadCloser.Read(p)
}

type shutdownBodyDispatcher struct {
	rpcAdmissionCountingDispatcher
	closed chan struct{}
}

func (d *shutdownBodyDispatcher) Close() {
	close(d.closed)
}

func TestShutdownHTTP_IncompleteBodyAllowsLaterCleanup(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	dispatcher := &shutdownBodyDispatcher{closed: make(chan struct{})}
	bodyStarted := make(chan struct{})
	handlerDone := make(chan struct{})

	rpc := &JSONRPC{
		logger:       hclog.NewNullLogger(),
		config:       &Config{},
		dispatcher:   dispatcher,
		closeCh:      make(chan struct{}),
		httpListener: listener,
	}
	httpServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req.Body = &shutdownBodyReader{
				ReadCloser: req.Body,
				started:    bodyStarted,
			}
			rpc.handle(w, req)
			close(handlerDone)
		}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	rpc.httpServer = httpServer

	t.Cleanup(func() {
		// Also release connections if an assertion fails before normal close.
		_ = httpServer.Close()
		_ = listener.Close()
		_ = rpc.Close()
	})

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- httpServer.Serve(listener)
	}()

	connection, err := net.DialTimeout(
		"tcp", listener.Addr().String(), 3*time.Second,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	require.NoError(t, connection.SetWriteDeadline(time.Now().Add(3*time.Second)))

	// Advertise a longer body, send only one byte, and keep the connection open.
	_, err = io.WriteString(connection,
		"POST / HTTP/1.1\r\n"+
			"Host: localhost\r\n"+
			"Content-Type: application/json\r\n"+
			"Content-Length: 1024\r\n"+
			"Connection: close\r\n\r\n"+
			"{",
	)
	require.NoError(t, err)

	select {
	case <-bodyStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP handler did not start reading the incomplete body")
	}
	select {
	case <-handlerDone:
		t.Fatal("incomplete request unexpectedly finished")
	default:
	}

	laterCleanup := make(chan struct{})
	closeDone := make(chan error, 1)
	go func() {
		closeErr := rpc.Close()

		// Represent the next stage in the sequential server close operation.
		close(laterCleanup)
		closeDone <- closeErr
	}()

	select {
	case closeErr := <-closeDone:
		// The component's own graceful deadline is expected to expire.
		// Forced connection closure must then release and join the handler.
		require.ErrorIs(t, closeErr, context.DeadlineExceeded)
	case <-time.After(10 * time.Second):
		t.Fatal("JSON-RPC close did not release the incomplete request")
	}

	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP handler did not finish after forced connection closure")
	}
	select {
	case <-dispatcher.closed:
	default:
		t.Fatal("dispatcher cleanup was not completed")
	}
	select {
	case <-laterCleanup:
	default:
		t.Fatal("later shutdown stage was not reached")
	}

	// The incomplete body never reached application dispatch.
	require.Zero(t, dispatcher.calls.Load())

	select {
	case serveErr := <-serveDone:
		require.ErrorIs(t, serveErr, http.ErrServerClosed)
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP serving goroutine did not finish")
	}

	require.NoError(t, connection.SetReadDeadline(time.Now().Add(time.Second)))
	_, readErr := io.ReadAll(connection)
	if readErr != nil {
		var networkErr net.Error
		if errors.As(readErr, &networkErr) {
			require.False(t, networkErr.Timeout(),
				"client connection remained open after shutdown")
		}
	}

	// The listener is reusable after the shutdown sequence.
	reopened, err := net.Listen("tcp", listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, reopened.Close())
}

/* FINISH ALPHA CHANGE 3163 -- verify forced HTTP closure releases incomplete request bodies before later cleanup -- */
