/* START ALPHA CHANGE 2240 -- bound public RPC admission and resource consumption -- */
package jsonrpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// AdmissionConfig controls this RPC server instance.
// HTTP requests and WebSocket messages share the same budgets.
type AdmissionConfig struct {
	RequestRate   int   `json:"request_rate" yaml:"request_rate"`
	RequestBurst  int   `json:"request_burst" yaml:"request_burst"`
	CallRate      int   `json:"call_rate" yaml:"call_rate"`
	CallBurst     int   `json:"call_burst" yaml:"call_burst"`
	MaxConcurrent int   `json:"max_concurrent" yaml:"max_concurrent"`
	MaxExpensive  int   `json:"max_expensive" yaml:"max_expensive"`
	MaxBodyBytes  int64 `json:"max_body_bytes" yaml:"max_body_bytes"`
	MaxWebSockets int   `json:"max_websockets" yaml:"max_websockets"`
	MaxFilters    int   `json:"max_filters" yaml:"max_filters"`
}

func DefaultAdmissionConfig() *AdmissionConfig {
	return &AdmissionConfig{
		RequestRate:   100,
		RequestBurst:  200,
		CallRate:      200,
		CallBurst:     400,
		MaxConcurrent: 32,
		MaxExpensive:  4,
		MaxBodyBytes:  1024 * 1024,
		MaxWebSockets: 64,
		MaxFilters:    1024,
	}
}

// A nil configuration selects the defaults. Explicit zero limits are invalid.
func (c *AdmissionConfig) Validate() error {
	if c == nil {
		return nil
	}

	for _, setting := range []struct {
		name  string
		value int
		max   int
	}{
		{"request_rate", c.RequestRate, 100000},
		{"request_burst", c.RequestBurst, 100000},
		{"call_rate", c.CallRate, 100000},
		{"call_burst", c.CallBurst, 100000},
		{"max_concurrent", c.MaxConcurrent, 1024},
		{"max_expensive", c.MaxExpensive, 1024},
		{"max_websockets", c.MaxWebSockets, 4096},
		{"max_filters", c.MaxFilters, 65536},
	} {
		if setting.value < 1 || setting.value > setting.max {
			return fmt.Errorf(
				"RPC admission %s must be between 1 and %d",
				setting.name, setting.max,
			)
		}
	}

	if c.MaxExpensive > c.MaxConcurrent {
		return fmt.Errorf("RPC max_expensive must not exceed max_concurrent")
	}
	if c.MaxBodyBytes < 1024 || c.MaxBodyBytes > 64*1024*1024 {
		return fmt.Errorf("RPC max_body_bytes must be between 1024 and 67108864")
	}

	return nil
}

type rpcAdmission struct {
	config AdmissionConfig
	now    func() time.Time

	requests  *rate.Limiter
	calls     *rate.Limiter
	active    chan struct{}
	expensive chan struct{}
	sockets   chan struct{}

	logMu      sync.Mutex
	logLimiter *rate.Limiter
	suppressed uint64
}

func newRPCAdmission(config *AdmissionConfig) *rpcAdmission {
	if config == nil {
		config = DefaultAdmissionConfig()
	}

	return &rpcAdmission{
		config:     *config,
		now:        time.Now,
		requests:   rate.NewLimiter(rate.Limit(config.RequestRate), config.RequestBurst),
		calls:      rate.NewLimiter(rate.Limit(config.CallRate), config.CallBurst),
		active:     make(chan struct{}, config.MaxConcurrent),
		expensive:  make(chan struct{}, config.MaxExpensive),
		sockets:    make(chan struct{}, config.MaxWebSockets),
		logLimiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Production constructors validate configuration before installing the controller.
// Lazy default initialization also supports existing internal test fixtures.
func (j *JSONRPC) rpcLimits() *rpcAdmission {
	j.admissionOnce.Do(func() {
		if j.admission == nil {
			j.admission = newRPCAdmission(nil)
		}
	})

	return j.admission
}

func (d *Dispatcher) rpcLimits() *rpcAdmission {
	d.admissionOnce.Do(func() {
		if d.admission == nil {
			d.admission = newRPCAdmission(nil)
		}
	})

	return d.admission
}

func acquireRPCSlot(slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
		return true
	default:
		return false
	}
}

// enter does not wait or queue work. Successful admission requires leave.
func (a *rpcAdmission) enter() int {
	if !a.requests.AllowN(a.now(), 1) {
		return http.StatusTooManyRequests
	}
	if !acquireRPCSlot(a.active) {
		return http.StatusServiceUnavailable
	}

	return 0
}

func (a *rpcAdmission) leave() {
	<-a.active
}

// Whole batches consume their call allowance before any member executes.
// A batch larger than CallBurst cannot be admitted.
func (a *rpcAdmission) allowCalls(count int) bool {
	return count >= 0 && a.calls.AllowN(a.now(), count)
}

func isExpensiveRPC(method string) bool {
	switch method {
	case "eth_call",
		"eth_estimateGas",
		"eth_getLogs",
		"eth_getFilterLogs",
		"eth_feeHistory":
		return true
	default:
		return false
	}
}

func (a *rpcAdmission) acquireExecution(method string) (func(), Error) {
	if !isExpensiveRPC(method) {
		return nil, nil
	}
	if !acquireRPCSlot(a.expensive) {
		return nil, &rpcLimitError{"RPC execution capacity exceeded"}
	}

	return func() { <-a.expensive }, nil
}

type rpcLimitError struct {
	message string
}

func (e *rpcLimitError) Error() string {
	return e.message
}

func (e *rpcLimitError) ErrorCode() int {
	return -32005
}

func rpcCallLimitResponse(requests BatchRequest, batch bool) ([]byte, error) {
	limitErr := &rpcLimitError{"RPC call rate exceeded"}
	if !batch {
		return NewRPCResponse(requests[0].ID, "2.0", nil, limitErr).Bytes()
	}

	responses := make([]Response, 0, len(requests))
	for _, request := range requests {
		responses = append(
			responses,
			NewRPCResponse(request.ID, "2.0", nil, limitErr),
		)
	}

	return json.Marshal(responses)
}

func writeRPCAdmissionError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "1")
	}

	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      nil,
		Error: struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{
			Code:    -32005,
			Message: message,
		},
	})
}

// A bounded category, not a map keyed by requests, addresses or error text.
func (a *rpcAdmission) allowErrorLog() (bool, uint64) {
	a.logMu.Lock()
	defer a.logMu.Unlock()

	if !a.logLimiter.AllowN(a.now(), 1) {
		a.suppressed++

		return false, 0
	}

	suppressed := a.suppressed
	a.suppressed = 0

	return true, suppressed
}

/* FINISH ALPHA CHANGE 2240 -- bound public RPC admission and resource consumption -- */
