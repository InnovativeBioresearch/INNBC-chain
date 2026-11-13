package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/versioning"
)

type serverType int

const (
	serverIPC serverType = iota
	serverHTTP
	serverWS
)

func (s serverType) String() string {
	switch s {
	case serverIPC:
		return "ipc"
	case serverHTTP:
		return "http"
	case serverWS:
		return "ws"
	default:
		panic("BUG: Not expected") //nolint:gocritic
	}
}

// JSONRPC is an API consensus
type JSONRPC struct {
	logger     hclog.Logger
	config     *Config
	dispatcher dispatcher

	/* START ALPHA CHANGE 2241 -- share RPC transport admission state -- */
	admissionOnce sync.Once
	admission     *rpcAdmission
	/* FINISH ALPHA CHANGE 2241 -- share RPC transport admission state -- */

	/* START ALPHA CHANGE 1404 -- track JSON-RPC shutdown lifecycle -- */
	httpServer   *http.Server
	httpListener net.Listener
	closeOnce    sync.Once
	closeCh      chan struct{}
	closeErr     error

	workerLock sync.Mutex
	workerWG   sync.WaitGroup
	wsConns    map[*websocket.Conn]struct{}
	/* FINISH ALPHA CHANGE 1404 -- track JSON-RPC shutdown lifecycle -- */
}

type dispatcher interface {
	RemoveFilterByWs(conn wsConn)
	HandleWs(reqBody []byte, conn wsConn) ([]byte, error)
	Handle(reqBody []byte) ([]byte, error)
	Close()
}

// JSONRPCStore defines all the methods required
// by all the JSON RPC endpoints
type JSONRPCStore interface {
	ethStore
	networkStore
	txPoolStore
	filterManagerStore
	/* START ALPHA CHANGE 424 -- remove bridge store from JSON-RPC store interface -- */
	/* FINISH ALPHA CHANGE 424 -- remove bridge store from JSON-RPC store interface -- */
	debugStore
}

type Config struct {
	Store                    JSONRPCStore
	Addr                     *net.TCPAddr
	ChainID                  uint64
	ChainName                string
	AccessControlAllowOrigin []string
	PriceLimit               uint64
	BatchLengthLimit         uint64
	BlockRangeLimit          uint64

	/* START ALPHA CHANGE 2457 -- configure early fee-history percentile validation -- */
	FeeHistoryPercentileLimit uint64
	/* FINISH ALPHA CHANGE 2457 -- configure early fee-history percentile validation -- */

	ConcurrentRequestsDebug uint64
	WebSocketReadLimit      uint64

	/* START ALPHA CHANGE 2241 -- configure native RPC admission -- */
	Admission *AdmissionConfig
	/* FINISH ALPHA CHANGE 2241 -- configure native RPC admission -- */
}

// NewJSONRPC returns the JSONRPC http server
func NewJSONRPC(logger hclog.Logger, config *Config) (*JSONRPC, error) {
	d, err := newDispatcher(
		logger,
		config.Store,
		&dispatcherParams{
			chainID:                 config.ChainID,
			chainName:               config.ChainName,
			priceLimit:              config.PriceLimit,
			jsonRPCBatchLengthLimit: config.BatchLengthLimit,
			blockRangeLimit:         config.BlockRangeLimit,
			/* START ALPHA CHANGE 2457 -- pass the configured percentile limit to the dispatcher -- */
			feeHistoryPercentileLimit: config.FeeHistoryPercentileLimit,
			/* FINISH ALPHA CHANGE 2457 -- pass the configured percentile limit to the dispatcher -- */
			concurrentRequestsDebug: config.ConcurrentRequestsDebug,
			/* START ALPHA CHANGE 2241 -- construct one admission controller per RPC server -- */
			admissionConfig: config.Admission,
			/* FINISH ALPHA CHANGE 2241 -- construct one admission controller per RPC server -- */
		},
	)

	if err != nil {
		return nil, err
	}

	srv := &JSONRPC{
		logger:     logger.Named("jsonrpc"),
		config:     config,
		dispatcher: d,
		/* START ALPHA CHANGE 2241 -- use shared HTTP and WebSocket budgets -- */
		admission: d.rpcLimits(),
		/* FINISH ALPHA CHANGE 2241 -- use shared HTTP and WebSocket budgets -- */
		/* START ALPHA CHANGE 1405 -- initialize JSON-RPC shutdown state -- */
		closeCh: make(chan struct{}),
		wsConns: make(map[*websocket.Conn]struct{}),
		/* FINISH ALPHA CHANGE 1405 -- initialize JSON-RPC shutdown state -- */
	}

	// start http server
	if err := srv.setupHTTP(); err != nil {
		d.Close()

		return nil, err
	}

	return srv, nil
}

func (j *JSONRPC) setupHTTP() error {
	j.logger.Info("http server started", "addr", j.config.Addr.String())

	lis, err := net.Listen("tcp", j.config.Addr.String())
	if err != nil {
		return err
	}

	// NewServeMux must be used, as it disables all debug features.
	// For some strange reason, with DefaultServeMux debug/vars is always enabled (but not debug/pprof).
	// If pprof need to be enabled, this should be DefaultServeMux
	mux := http.NewServeMux()

	// The middleware factory returns a handler, so we need to wrap the handler function properly.
	jsonRPCHandler := http.HandlerFunc(j.handle)
	mux.Handle("/", middlewareFactory(j.config)(jsonRPCHandler))

	mux.HandleFunc("/ws", j.handleWs)

	/* START ALPHA CHANGE 1406 -- retain JSON-RPC HTTP server for shutdown -- */
	/* START ALPHA CHANGE 2242 -- bound HTTP header body and response network waits -- */
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	/* FINISH ALPHA CHANGE 2242 -- bound HTTP header body and response network waits -- */

	j.httpServer = srv

	/* START ALPHA CHANGE 1424 -- retain listener across asynchronous Serve startup -- */
	j.httpListener = lis
	/* FINISH ALPHA CHANGE 1424 -- retain listener across asynchronous Serve startup -- */

	go func() {
		if err := srv.Serve(lis); err != nil &&
			!errors.Is(err, http.ErrServerClosed) &&
			!errors.Is(err, net.ErrClosed) {
			j.logger.Error("closed http connection", "err", err)
		}
	}()
	/* FINISH ALPHA CHANGE 1406 -- retain JSON-RPC HTTP server for shutdown -- */

	return nil
}

// The middlewareFactory builds a middleware which enables CORS using the provided config.
func middlewareFactory(config *Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			for _, allowedOrigin := range config.AccessControlAllowOrigin {
				if allowedOrigin == "*" {
					w.Header().Set("Access-Control-Allow-Origin", "*")

					break
				}

				if allowedOrigin == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)

					break
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// wsUpgrader defines upgrade parameters for the WS connection
var wsUpgrader = websocket.Upgrader{
	// Uses the default HTTP buffer sizes for Read / Write buffers.
	// Documentation specifies that they are 4096B in size.
	// There is no need to have them be 4x in size when requests / responses
	// shouldn't exceed 1024B
	/* START ALPHA CHANGE 2257 -- bound WebSocket handshake writes -- */
	ReadBufferSize:   1024,
	WriteBufferSize:  1024,
	HandshakeTimeout: 10 * time.Second,
	/* FINISH ALPHA CHANGE 2257 -- bound WebSocket handshake writes -- */
}

// wsWrapper is a wrapping object for the web socket connection and logger
type wsWrapper struct {
	sync.Mutex

	ws       *websocket.Conn // the actual WS connection
	logger   hclog.Logger    // module logger
	filterID string          // filter ID
}

/* START ALPHA CHANGE 2243 -- synchronize WebSocket metadata and bound writes -- */
func (w *wsWrapper) SetFilterID(filterID string) {
	w.Lock()
	defer w.Unlock()

	w.filterID = filterID
}

func (w *wsWrapper) GetFilterID() string {
	w.Lock()
	defer w.Unlock()

	return w.filterID
}

/* START ALPHA CHANGE 2546 -- close affected WebSockets without waiting for the write mutex --

The connection pointer is fixed when the wrapper is constructed.
Transport closure can interrupt a concurrent blocked write.

FINISH ALPHA CHANGE 2546 -- close affected WebSockets without waiting for the write mutex -- */

func (w *wsWrapper) Close() error {
	if w.ws == nil {
		return nil
	}

	return w.ws.Close()
}

/* START ALPHA CHANGE 2253 -- close WebSockets after terminal write failures -- */
// WriteMessage serializes responses and subscription notifications.
// A failed write closes the socket so the read handler can release its resources.
func (w *wsWrapper) WriteMessage(messageType int, data []byte) error {
	w.Lock()
	defer w.Unlock()

	if err := w.ws.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		_ = w.ws.Close()

		return err
	}

	if err := w.ws.WriteMessage(messageType, data); err != nil {
		_ = w.ws.Close()

		return err
	}

	return nil
}

/* FINISH ALPHA CHANGE 2253 -- close WebSockets after terminal write failures -- */

/* FINISH ALPHA CHANGE 2243 -- synchronize WebSocket metadata and bound writes -- */

// isSupportedWSType returns a status indicating if the message type is supported
func isSupportedWSType(messageType int) bool {
	return messageType == websocket.TextMessage ||
		messageType == websocket.BinaryMessage
}

/* START ALPHA CHANGE 2243 -- bound WebSocket connections and message execution -- */
func (j *JSONRPC) handleWs(w http.ResponseWriter, req *http.Request) {
	if !j.enterWorker() {
		writeRPCAdmissionError(w, http.StatusServiceUnavailable, "RPC server is shutting down")

		return
	}
	defer j.leaveWorker()

	limits := j.rpcLimits()
	if status := limits.enter(); status != 0 {
		writeRPCAdmissionError(w, status, "RPC admission capacity exceeded")

		return
	}

	if !acquireRPCSlot(limits.sockets) {
		limits.leave()
		writeRPCAdmissionError(w, http.StatusServiceUnavailable, "RPC WebSocket capacity exceeded")

		return
	}
	defer func() { <-limits.sockets }()

	// Preserve the existing origin policy without mutating a shared upgrader.
	upgrader := wsUpgrader
	upgrader.CheckOrigin = func(*http.Request) bool { return true }

	ws, err := upgrader.Upgrade(w, req, nil)
	limits.leave()
	if err != nil {
		return
	}

	if !j.registerWsConn(ws) {
		_ = ws.Close()

		return
	}

	// The configured WS limit can be stricter than the transport body cap.
	// A zero WS-specific limit does not disable the shared body cap.
	readLimit := limits.config.MaxBodyBytes
	if j.config.WebSocketReadLimit != 0 &&
		j.config.WebSocketReadLimit < uint64(readLimit) {
		readLimit = int64(j.config.WebSocketReadLimit)
	}
	ws.SetReadLimit(readLimit)

	wrapConn := &wsWrapper{ws: ws, logger: j.logger}
	defer func() {
		j.dispatcher.RemoveFilterByWs(wrapConn)
		_ = ws.Close()
		j.unregisterWsConn(ws)
	}()

	for {
		msgType, message, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if !isSupportedWSType(msgType) {
			continue
		}

		select {
		case <-j.closeCh:
			return
		default:
		}

		if status := limits.enter(); status != 0 {
			_ = ws.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(
					websocket.CloseTryAgainLater,
					"RPC temporarily at capacity",
				),
				time.Now().Add(time.Second),
			)

			return
		}

		/* START ALPHA CHANGE 2258 -- release WebSocket message admission during unwinding -- */
		// One in-flight message per connection. There is no goroutine queue.
		if err := j.dispatchAdmittedWs(message, msgType, wrapConn); err != nil {
			return
		}
		/* FINISH ALPHA CHANGE 2258 -- release WebSocket message admission during unwinding -- */
	}
}

/* FINISH ALPHA CHANGE 2243 -- bound WebSocket connections and message execution -- */

/* START ALPHA CHANGE 2258 -- pair admitted WebSocket dispatch with deferred release -- */
// dispatchAdmittedWs requires one already-acquired transport admission slot.
// It releases that slot on success, error, or panic without swallowing panics.
func (j *JSONRPC) dispatchAdmittedWs(message []byte, messageType int, connection wsConn) error {
	defer j.rpcLimits().leave()

	response, err := j.dispatcher.HandleWs(message, connection)
	if err != nil {
		return err
	}

	return connection.WriteMessage(messageType, response)
}

/* FINISH ALPHA CHANGE 2258 -- pair admitted WebSocket dispatch with deferred release -- */

func (j *JSONRPC) handle(w http.ResponseWriter, req *http.Request) {
	/* START ALPHA CHANGE 1408 -- reject and join HTTP handlers during shutdown -- */
	if !j.enterWorker() {
		http.Error(w, "json-rpc server is shutting down", http.StatusServiceUnavailable)

		return
	}
	defer j.leaveWorker()
	/* FINISH ALPHA CHANGE 1408 -- reject and join HTTP handlers during shutdown -- */

	/* START ALPHA CHANGE 2242 -- reject HTTP overload without waiting -- */
	limits := j.rpcLimits()
	if status := limits.enter(); status != 0 {
		writeRPCAdmissionError(w, status, "RPC admission capacity exceeded")

		return
	}
	defer limits.leave()

	if req.Context().Err() != nil {
		return
	}
	/* FINISH ALPHA CHANGE 2242 -- reject HTTP overload without waiting -- */

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set(
		"Access-Control-Allow-Headers",
		"Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization",
	)

	switch req.Method {
	case "POST":
		j.handleJSONRPCRequest(w, req)
	case "GET":
		j.handleGetRequest(w)
	case "OPTIONS":
		// nothing to return
	default:
		_, _ = w.Write([]byte("method " + req.Method + " not allowed"))
	}
}

/* START ALPHA CHANGE 1409 -- close JSON-RPC intake and wait for handlers -- */
func (j *JSONRPC) Close() error {
	j.closeOnce.Do(func() {
		j.workerLock.Lock()
		close(j.closeCh)
		j.workerLock.Unlock()

		if j.httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			shutdownErr := j.httpServer.Shutdown(ctx)
			cancel()

			if shutdownErr != nil {
				j.closeErr = errors.Join(
					j.closeErr,
					fmt.Errorf("JSON-RPC HTTP shutdown failed: %w", shutdownErr),
				)

				if forceErr := j.httpServer.Close(); forceErr != nil &&
					!errors.Is(forceErr, http.ErrServerClosed) {
					j.closeErr = errors.Join(
						j.closeErr,
						fmt.Errorf("JSON-RPC forced HTTP close failed: %w", forceErr),
					)
				}
			}
		}

		/* START ALPHA CHANGE 1425 -- close listener even if Shutdown raced Serve startup -- */
		if j.httpListener != nil {
			if listenerErr := j.httpListener.Close(); listenerErr != nil &&
				!errors.Is(listenerErr, net.ErrClosed) {
				j.closeErr = errors.Join(
					j.closeErr,
					fmt.Errorf("JSON-RPC listener close failed: %w", listenerErr),
				)
			}
		}
		/* FINISH ALPHA CHANGE 1425 -- close listener even if Shutdown raced Serve startup -- */

		j.closeWsConns()
		j.workerWG.Wait()

		if j.dispatcher != nil {
			j.dispatcher.Close()
		}
	})

	return j.closeErr
}

func (j *JSONRPC) enterWorker() bool {
	j.workerLock.Lock()
	defer j.workerLock.Unlock()

	select {
	case <-j.closeCh:
		return false
	default:
		j.workerWG.Add(1)

		return true
	}
}

func (j *JSONRPC) leaveWorker() {
	j.workerWG.Done()
}

/* START ALPHA CHANGE 2243 -- WebSocket messages execute within their tracked connection handler -- */
/* FINISH ALPHA CHANGE 2243 -- WebSocket messages execute within their tracked connection handler -- */

func (j *JSONRPC) registerWsConn(ws *websocket.Conn) bool {
	j.workerLock.Lock()
	defer j.workerLock.Unlock()

	select {
	case <-j.closeCh:
		return false
	default:
		j.wsConns[ws] = struct{}{}
		j.workerWG.Add(1)

		return true
	}
}

func (j *JSONRPC) unregisterWsConn(ws *websocket.Conn) {
	j.workerLock.Lock()
	delete(j.wsConns, ws)
	j.workerLock.Unlock()

	j.workerWG.Done()
}

func (j *JSONRPC) closeWsConns() {
	j.workerLock.Lock()
	conns := make([]*websocket.Conn, 0, len(j.wsConns))
	for ws := range j.wsConns {
		conns = append(conns, ws)
	}
	j.workerLock.Unlock()

	for _, ws := range conns {
		_ = ws.Close()
	}
}

/* FINISH ALPHA CHANGE 1409 -- close JSON-RPC intake and wait for handlers -- */

/* START ALPHA CHANGE 2242 -- limit RPC body allocation and avoid payload logging -- */
func (j *JSONRPC) handleJSONRPCRequest(w http.ResponseWriter, req *http.Request) {
	maxBody := j.rpcLimits().config.MaxBodyBytes
	if req.ContentLength > maxBody {
		writeRPCAdmissionError(w, http.StatusRequestEntityTooLarge, "RPC request body too large")

		return
	}

	req.Body = http.MaxBytesReader(w, req.Body, maxBody)
	data, err := io.ReadAll(req.Body)
	if err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			writeRPCAdmissionError(w, http.StatusRequestEntityTooLarge, "RPC request body too large")
		} else {
			http.Error(w, "unable to read RPC request", http.StatusBadRequest)
		}

		return
	}

	if req.Context().Err() != nil {
		return
	}

	resp, err := j.dispatcher.Handle(data)
	if err != nil {
		http.Error(w, "unable to encode RPC response", http.StatusInternalServerError)

		return
	}

	_, _ = w.Write(resp)
}

/* FINISH ALPHA CHANGE 2242 -- limit RPC body allocation and avoid payload logging -- */

type GetResponse struct {
	Name    string `json:"name"`
	ChainID uint64 `json:"chain_id"`
	Version string `json:"version"`
}

func (j *JSONRPC) handleGetRequest(writer io.Writer) {
	data := &GetResponse{
		Name:    j.config.ChainName,
		ChainID: j.config.ChainID,
		Version: versioning.Version,
	}

	resp, err := json.Marshal(data)
	if err != nil {
		_, _ = writer.Write([]byte(err.Error()))
	}

	if _, err = writer.Write(resp); err != nil {
		_, _ = writer.Write([]byte(err.Error()))
	}
}
