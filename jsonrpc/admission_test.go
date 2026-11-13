/* START ALPHA CHANGE 2251 -- test RPC admission and capacity recovery -- */
package jsonrpc

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func frozenRPCAdmission(config *AdmissionConfig) *rpcAdmission {
	a := newRPCAdmission(config)
	now := time.Now()
	a.now = func() time.Time { return now }

	return a
}

func TestRPCAdmissionConfiguration(t *testing.T) {
	var absent *AdmissionConfig
	require.NoError(t, absent.Validate())
	require.NoError(t, DefaultAdmissionConfig().Validate())

	config := DefaultAdmissionConfig()
	config.RequestRate = 0
	require.Error(t, config.Validate())

	config = DefaultAdmissionConfig()
	config.MaxExpensive = config.MaxConcurrent + 1
	require.Error(t, config.Validate())

	config = DefaultAdmissionConfig()
	config.MaxBodyBytes = 0
	require.Error(t, config.Validate())
}

func TestRPCAdmissionRateAndConcurrencyAreSeparate(t *testing.T) {
	config := DefaultAdmissionConfig()
	config.RequestBurst = 3
	config.MaxConcurrent = 1
	config.MaxExpensive = 1
	a := frozenRPCAdmission(config)

	require.Zero(t, a.enter())
	require.Equal(t, http.StatusServiceUnavailable, a.enter())

	a.leave()
	require.Zero(t, a.enter())
	a.leave()

	require.Equal(t, http.StatusTooManyRequests, a.enter())
	require.Empty(t, a.active)
}

func TestRPCAdmissionConcurrentSlotsAreBounded(t *testing.T) {
	config := DefaultAdmissionConfig()
	config.RequestBurst = 1000
	config.MaxConcurrent = 3
	config.MaxExpensive = 1
	a := frozenRPCAdmission(config)

	const workers = 32
	var attempted sync.WaitGroup
	var completed sync.WaitGroup
	var admitted atomic.Int32
	release := make(chan struct{})

	attempted.Add(workers)
	completed.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer completed.Done()

			status := a.enter()
			if status == 0 {
				admitted.Add(1)
			}
			attempted.Done()

			if status == 0 {
				<-release
				a.leave()
			}
		}()
	}

	attempted.Wait()
	require.Equal(t, int32(3), admitted.Load())
	close(release)
	completed.Wait()
	require.Empty(t, a.active)
}

type rpcAdmissionTestService struct {
	calls atomic.Int32
}

func (s *rpcAdmissionTestService) Read() (any, error) {
	s.calls.Add(1)

	return "healthy", nil
}

func (s *rpcAdmissionTestService) Call() (any, error) {
	s.calls.Add(1)

	return "executed", nil
}

func TestRPCAdmissionRejectsWholeBatchBeforeExecution(t *testing.T) {
	config := DefaultAdmissionConfig()
	config.CallBurst = 2

	d := &Dispatcher{
		logger:    hclog.NewNullLogger(),
		params:    &dispatcherParams{jsonRPCBatchLengthLimit: 20},
		admission: frozenRPCAdmission(config),
	}
	service := &rpcAdmissionTestService{}
	require.NoError(t, d.registerService("budget", service))

	batch := []byte(`[
		{"jsonrpc":"2.0","id":1,"method":"budget_read"},
		{"jsonrpc":"2.0","id":2,"method":"budget_read"},
		{"jsonrpc":"2.0","id":3,"method":"budget_read"}
	]`)

	body, err := d.Handle(batch)
	require.NoError(t, err)
	require.Zero(t, service.calls.Load())

	var rejected []struct {
		ID    int `json:"id"`
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &rejected))
	require.Len(t, rejected, 3)
	for index, response := range rejected {
		require.Equal(t, index+1, response.ID)
		require.Equal(t, -32005, response.Error.Code)
	}

	// A rejected oversized batch did not consume a partial allowance.
	_, err = d.Handle([]byte(`[
		{"jsonrpc":"2.0","id":4,"method":"budget_read"},
		{"jsonrpc":"2.0","id":5,"method":"budget_read"}
	]`))
	require.NoError(t, err)
	require.Equal(t, int32(2), service.calls.Load())

	// HTTP and WebSocket dispatcher entrypoints share the call budget.
	body, err = d.HandleWs(
		[]byte(`{"jsonrpc":"2.0","id":6,"method":"budget_read"}`),
		nil,
	)
	require.NoError(t, err)
	require.Contains(t, string(body), `"code":-32005`)
	require.Equal(t, int32(2), service.calls.Load())
}

func TestRPCAdmissionExpensiveExecutionDoesNotBlockCheapReads(t *testing.T) {
	config := DefaultAdmissionConfig()
	config.MaxExpensive = 1
	d := &Dispatcher{
		logger:    hclog.NewNullLogger(),
		params:    &dispatcherParams{},
		admission: frozenRPCAdmission(config),
	}
	service := &rpcAdmissionTestService{}
	require.NoError(t, d.registerService("eth", service))

	release, err := d.rpcLimits().acquireExecution("eth_call")
	require.NoError(t, err)
	require.NotNil(t, release)

	_, rpcErr := d.handleReq(Request{Method: "eth_call"})
	require.Error(t, rpcErr)
	require.Equal(t, -32005, rpcErr.ErrorCode())
	require.Zero(t, service.calls.Load())

	_, rpcErr = d.handleReq(Request{Method: "eth_read"})
	require.NoError(t, rpcErr)
	require.Equal(t, int32(1), service.calls.Load())

	release()

	_, rpcErr = d.handleReq(Request{Method: "eth_call"})
	require.NoError(t, rpcErr)
	require.Equal(t, int32(2), service.calls.Load())
	require.Empty(t, d.rpcLimits().expensive)
}

type rpcAdmissionCountingDispatcher struct {
	calls atomic.Int32
}

func (d *rpcAdmissionCountingDispatcher) Handle([]byte) ([]byte, error) {
	d.calls.Add(1)

	return []byte(`{"jsonrpc":"2.0","id":1,"result":true}`), nil
}

func (d *rpcAdmissionCountingDispatcher) HandleWs([]byte, wsConn) ([]byte, error) {
	return nil, nil
}

func (*rpcAdmissionCountingDispatcher) RemoveFilterByWs(wsConn) {}
func (*rpcAdmissionCountingDispatcher) Close()                  {}

func TestRPCAdmissionHTTPBodyLimitBeforeDispatch(t *testing.T) {
	for _, unknownLength := range []bool{false, true} {
		t.Run(map[bool]string{false: "known length", true: "streamed body"}[unknownLength],
			func(t *testing.T) {
				config := DefaultAdmissionConfig()
				config.MaxBodyBytes = 1024
				dispatcher := &rpcAdmissionCountingDispatcher{}
				server := &JSONRPC{
					logger:     hclog.NewNullLogger(),
					config:     &Config{},
					dispatcher: dispatcher,
					admission:  frozenRPCAdmission(config),
					closeCh:    make(chan struct{}),
				}

				request := httptest.NewRequest(
					http.MethodPost,
					"/rpc",
					strings.NewReader(strings.Repeat("x", 1025)),
				)
				if unknownLength {
					request.ContentLength = -1
				}
				response := httptest.NewRecorder()
				server.handle(response, request)

				require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
				require.Zero(t, dispatcher.calls.Load())
				require.Empty(t, server.rpcLimits().active)
			})
	}
}

type rpcAdmissionUnreadBody struct {
	read bool
}

func (b *rpcAdmissionUnreadBody) Read([]byte) (int, error) {
	b.read = true

	return 0, io.EOF
}

func (*rpcAdmissionUnreadBody) Close() error { return nil }

func TestRPCAdmissionRejectsOverloadBeforeReadingBody(t *testing.T) {
	config := DefaultAdmissionConfig()
	config.RequestBurst = 1
	dispatcher := &rpcAdmissionCountingDispatcher{}
	server := &JSONRPC{
		logger:     hclog.NewNullLogger(),
		config:     &Config{},
		dispatcher: dispatcher,
		admission:  frozenRPCAdmission(config),
		closeCh:    make(chan struct{}),
	}

	require.Zero(t, server.rpcLimits().enter())
	server.rpcLimits().leave()

	body := &rpcAdmissionUnreadBody{}
	request := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	request.Body = body
	response := httptest.NewRecorder()

	server.handle(response, request)

	require.Equal(t, http.StatusTooManyRequests, response.Code)
	require.Equal(t, "1", response.Header().Get("Retry-After"))
	require.False(t, body.read)
	require.Zero(t, dispatcher.calls.Load())
}

func TestRPCAdmissionFilterCapacityAndConnectionCleanup(t *testing.T) {
	manager := NewFilterManager(hclog.NewNullLogger(), newMockStore(), 256)
	defer manager.Close()
	manager.maxFilters = 2

	firstConnection := &wsWrapper{}
	secondConnection := &wsWrapper{}

	first := manager.NewBlockFilter(firstConnection)
	second := manager.NewLogFilter(&LogQuery{}, firstConnection)
	require.NotEmpty(t, first)
	require.NotEmpty(t, second)

	require.Empty(t, manager.NewBlockFilter(secondConnection))
	require.Empty(t, secondConnection.GetFilterID())

	manager.RemoveFilterByWs(firstConnection)
	require.False(t, manager.Exists(first))
	require.False(t, manager.Exists(second))

	require.NotEmpty(t, manager.NewBlockFilter(secondConnection))
}

func TestRPCAdmissionFilterCapacityReturnsRPCError(t *testing.T) {
	d, err := newDispatcher(
		hclog.NewNullLogger(),
		newMockStore(),
		&dispatcherParams{
			admissionConfig: func() *AdmissionConfig {
				config := DefaultAdmissionConfig()
				config.MaxFilters = 1

				return config
			}(),
		},
	)
	require.NoError(t, err)
	defer d.Close()

	request := []byte(`{"jsonrpc":"2.0","id":1,"method":"eth_newBlockFilter"}`)
	first, err := d.Handle(request)
	require.NoError(t, err)
	require.Contains(t, string(first), `"result"`)

	second, err := d.Handle(request)
	require.NoError(t, err)
	require.Contains(t, string(second), `"code":-32005`)

	var created struct {
		Result string `json:"result"`
	}
	require.NoError(t, json.Unmarshal(first, &created))
	require.True(t, d.filterManager.Uninstall(created.Result))

	third, err := d.Handle(request)
	require.NoError(t, err)
	require.Contains(t, string(third), `"result"`)
}

func TestRPCAdmissionErrorLogSuppression(t *testing.T) {
	a := frozenRPCAdmission(DefaultAdmissionConfig())

	allowed, count := a.allowErrorLog()
	require.True(t, allowed)
	require.Zero(t, count)

	for i := 0; i < 10; i++ {
		allowed, _ = a.allowErrorLog()
		require.False(t, allowed)
	}

	later := a.now().Add(5 * time.Second)
	a.now = func() time.Time { return later }

	allowed, count = a.allowErrorLog()
	require.True(t, allowed)
	require.Equal(t, uint64(10), count)
}

func TestRPCAdmissionWebSocketConnectionCapacity(t *testing.T) {
	config := DefaultAdmissionConfig()
	config.MaxWebSockets = 1

	server, err := NewJSONRPC(hclog.NewNullLogger(), &Config{
		Store:     newMockStore(),
		Addr:      &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0},
		Admission: config,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })

	url := "ws://" + server.httpListener.Addr().String() + "/ws"
	first, response, err := websocket.DefaultDialer.Dial(url, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	require.NoError(t, err)
	defer first.Close()

	second, response, err := websocket.DefaultDialer.Dial(url, nil)
	if second != nil {
		_ = second.Close()
	}
	require.Error(t, err)
	require.NotNil(t, response)
	require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	_ = response.Body.Close()

	require.NoError(t, first.Close())
	require.Eventually(t, func() bool {
		return len(server.rpcLimits().sockets) == 0
	}, time.Second, time.Millisecond)

	third, response, err := websocket.DefaultDialer.Dial(url, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	require.NoError(t, err)
	require.NoError(t, third.Close())
}

func TestRPCAdmissionHealthyHTTPResultUnchanged(t *testing.T) {
	server, err := NewJSONRPC(hclog.NewNullLogger(), &Config{
		Store:   newMockStore(),
		Addr:    &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0},
		ChainID: 100,
	})
	require.NoError(t, err)
	defer server.Close()

	request := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(
		`{"jsonrpc":"2.0","id":7,"method":"eth_chainId"}`,
	))
	response := httptest.NewRecorder()
	server.handle(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t,
		`{"jsonrpc":"2.0","id":7,"result":"0x64"}`,
		response.Body.String(),
	)
}

/* FINISH ALPHA CHANGE 2251 -- test RPC admission and capacity recovery -- */
