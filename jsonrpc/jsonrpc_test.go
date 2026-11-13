package jsonrpc

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/innovativebioresearch/innbc-chain/helper/tests"
	"github.com/innovativebioresearch/innbc-chain/versioning"
)

/* START ALPHA CHANGE 1420 -- test joined JSON-RPC shutdown behavior -- */
type blockingTestDispatcher struct {
	handleStarted chan struct{}
	handleRelease chan struct{}
	closeCh       chan struct{}
	closeOnce     sync.Once
}

func newBlockingTestDispatcher() *blockingTestDispatcher {
	return &blockingTestDispatcher{
		handleStarted: make(chan struct{}),
		handleRelease: make(chan struct{}),
		closeCh:       make(chan struct{}),
	}
}

func (d *blockingTestDispatcher) RemoveFilterByWs(wsConn) {
}

func (d *blockingTestDispatcher) HandleWs([]byte, wsConn) ([]byte, error) {
	return nil, nil
}

func (d *blockingTestDispatcher) Handle([]byte) ([]byte, error) {
	close(d.handleStarted)
	<-d.handleRelease

	return []byte(`{}`), nil
}

func (d *blockingTestDispatcher) Close() {
	d.closeOnce.Do(func() {
		close(d.closeCh)
	})
}

/* FINISH ALPHA CHANGE 1420 -- test joined JSON-RPC shutdown behavior -- */

func TestHTTPServer(t *testing.T) {
	_, err := newTestJSONRPC(t)
	require.NoError(t, err)
}

func Test_handleGetRequest(t *testing.T) {
	var (
		chainName = "polygon-edge-test"
		chainID   = uint64(200)
	)

	jsonRPC := &JSONRPC{
		config: &Config{
			ChainName: chainName,
			ChainID:   chainID,
		},
	}

	mockWriter := bytes.NewBuffer(nil)

	jsonRPC.handleGetRequest(mockWriter)

	response := &GetResponse{}

	require.NoError(
		t,
		json.Unmarshal(mockWriter.Bytes(), response),
	)

	require.Equal(
		t,
		&GetResponse{
			Name:    chainName,
			ChainID: chainID,
			Version: versioning.Version,
		},
		response,
	)
}

func TestJSONRPC_handleJSONRPCRequest(t *testing.T) {
	t.Parallel()

	j, err := newTestJSONRPC(t)
	require.NoError(t, err)

	cases := []struct {
		name             string
		request          string
		expectedResponse string
	}{
		{
			name: "malicious input (XSS attack)",
			request: `{"jsonrpc":"2.0","id":0,"method":"eth_getBlockByNumber","params":["latest",false]}
<script>alert("XSS attack");</script>`,
			expectedResponse: `{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid json request"}}`,
		},
		{
			name:             "valid input",
			request:          `{"jsonrpc":"2.0","id":0,"method":"eth_getBlockByNumber","params":["latest",false]}`,
			expectedResponse: `{"jsonrpc":"2.0","id":0,"result":{`,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("POST", "/eth_getBlockByNumber", strings.NewReader(c.request))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()

			j.handleJSONRPCRequest(w, req)

			response := w.Body.String()
			require.Contains(t, response, c.expectedResponse)
		})
	}
}

/* START ALPHA CHANGE 1421 -- verify listener, HTTP worker, and websocket shutdown -- */
func TestJSONRPCCloseIsIdempotentAndReleasesListener(t *testing.T) {
	srv, err := newTestJSONRPC(t)
	require.NoError(t, err)

	addr := srv.config.Addr.String()

	require.NoError(t, srv.Close())
	require.NoError(t, srv.Close())

	listener, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	require.NoError(t, listener.Close())
}

func TestJSONRPCCloseWaitsForActiveHTTPHandler(t *testing.T) {
	dispatcher := newBlockingTestDispatcher()
	srv := &JSONRPC{
		logger:     hclog.NewNullLogger(),
		config:     &Config{},
		dispatcher: dispatcher,
		closeCh:    make(chan struct{}),
		wsConns:    make(map[*websocket.Conn]struct{}),
	}

	request := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	handlerDone := make(chan struct{})

	go func() {
		defer close(handlerDone)

		srv.handle(response, request)
	}()

	select {
	case <-dispatcher.handleStarted:
	case <-time.After(time.Second):
		t.Fatal("HTTP handler did not enter dispatcher")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- srv.Close()
	}()

	select {
	case closeErr := <-closeDone:
		close(dispatcher.handleRelease)
		t.Fatalf("JSON-RPC close returned before HTTP handler completed: %v", closeErr)
	case <-time.After(50 * time.Millisecond):
	}

	close(dispatcher.handleRelease)

	select {
	case closeErr := <-closeDone:
		require.NoError(t, closeErr)
	case <-time.After(time.Second):
		t.Fatal("JSON-RPC close did not finish after HTTP handler completed")
	}

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("HTTP handler did not finish")
	}

	select {
	case <-dispatcher.closeCh:
	case <-time.After(time.Second):
		t.Fatal("dispatcher was not closed")
	}
}

func TestJSONRPCCloseClosesActiveWebSocket(t *testing.T) {
	srv, err := newTestJSONRPC(t)
	require.NoError(t, err)

	ws, _, err := websocket.DefaultDialer.Dial(
		"ws://"+srv.config.Addr.String()+"/ws",
		nil,
	)
	require.NoError(t, err)
	defer ws.Close()

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- srv.Close()
	}()

	select {
	case closeErr := <-closeDone:
		require.NoError(t, closeErr)
	case <-time.After(10 * time.Second):
		t.Fatal("JSON-RPC close did not finish after closing websocket connections")
	}

	require.NoError(t, ws.SetReadDeadline(time.Now().Add(time.Second)))

	_, _, readErr := ws.ReadMessage()
	require.Error(t, readErr)
}

/* FINISH ALPHA CHANGE 1421 -- verify listener, HTTP worker, and websocket shutdown -- */

func newTestJSONRPC(t *testing.T) (*JSONRPC, error) {
	t.Helper()

	store := newMockStore()

	port, err := tests.GetFreePort()
	require.NoError(t, err, "Unable to fetch free port, %v", err)

	config := &Config{
		Store: store,
		Addr:  &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port},
	}

	srv, err := NewJSONRPC(hclog.NewNullLogger(), config)
	if err == nil {
		t.Cleanup(func() {
			require.NoError(t, srv.Close())
		})
	}

	return srv, err
}
