package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	/* START ALPHA CHANGE 2244 -- initialize dispatcher admission safely -- */
	"sync"
	/* FINISH ALPHA CHANGE 2244 -- initialize dispatcher admission safely -- */
	"time"
	"unicode"

	"github.com/armon/go-metrics"
	"github.com/hashicorp/go-hclog"

	/* START ALPHA CHANGE 2458 -- validate configured RPC percentile limits -- */
	"github.com/innovativebioresearch/innbc-chain/gasprice"
	/* FINISH ALPHA CHANGE 2458 -- validate configured RPC percentile limits -- */

	/* START ALPHA CHANGE 2170 -- classify local state failures at RPC dispatch -- */
	"github.com/innovativebioresearch/innbc-chain/state"
	/* FINISH ALPHA CHANGE 2170 -- classify local state failures at RPC dispatch -- */)

type serviceData struct {
	sv      reflect.Value
	funcMap map[string]*funcData
}

type funcData struct {
	inNum int
	reqt  []reflect.Type
	fv    reflect.Value
	isDyn bool
}

func (f *funcData) numParams() int {
	return f.inNum - 1
}

type endpoints struct {
	Eth  *Eth
	Web3 *Web3
	Net  *Net
	/* START ALPHA CHANGE 1139 -- remove operator-only JSON-RPC endpoint handles from public dispatcher -- */
	/* FINISH ALPHA CHANGE 1139 -- remove operator-only JSON-RPC endpoint handles from public dispatcher -- */
	/* START ALPHA CHANGE 425 -- remove bridge JSON-RPC endpoint handle -- */
	/* FINISH ALPHA CHANGE 425 -- remove bridge JSON-RPC endpoint handle -- */
}

// Dispatcher handles all json rpc requests by delegating
// the execution flow to the corresponding service
type Dispatcher struct {
	logger        hclog.Logger
	serviceMap    map[string]*serviceData
	filterManager *FilterManager
	endpoints     endpoints

	params *dispatcherParams

	/* START ALPHA CHANGE 2244 -- retain dispatcher admission state -- */
	admissionOnce sync.Once
	admission     *rpcAdmission
	/* FINISH ALPHA CHANGE 2244 -- retain dispatcher admission state -- */
}

type dispatcherParams struct {
	chainID   uint64
	chainName string

	priceLimit              uint64
	jsonRPCBatchLengthLimit uint64
	blockRangeLimit         uint64

	/* START ALPHA CHANGE 2458 -- retain the RPC percentile limit -- */
	feeHistoryPercentileLimit uint64
	/* FINISH ALPHA CHANGE 2458 -- retain the RPC percentile limit -- */

	concurrentRequestsDebug uint64

	/* START ALPHA CHANGE 2244 -- receive native RPC admission configuration -- */
	admissionConfig *AdmissionConfig
	/* FINISH ALPHA CHANGE 2244 -- receive native RPC admission configuration -- */
}

func (dp dispatcherParams) isExceedingBatchLengthLimit(value uint64) bool {
	return dp.jsonRPCBatchLengthLimit != 0 && value > dp.jsonRPCBatchLengthLimit
}

func newDispatcher(
	logger hclog.Logger,
	store JSONRPCStore,
	params *dispatcherParams,
) (*Dispatcher, error) {
	/* START ALPHA CHANGE 2458 -- normalize percentile configuration before creating RPC resources -- */
	resolved, err := gasprice.ResolveFeeHistoryConfig(&gasprice.Config{
		FeeHistoryPercentileLimit: params.feeHistoryPercentileLimit,
	})
	if err != nil {
		return nil, err
	}
	params.feeHistoryPercentileLimit = resolved.FeeHistoryPercentileLimit
	/* FINISH ALPHA CHANGE 2458 -- normalize percentile configuration before creating RPC resources -- */

	/* START ALPHA CHANGE 2244 -- validate budgets before creating RPC resources -- */
	if err := params.admissionConfig.Validate(); err != nil {
		return nil, err
	}

	d := &Dispatcher{
		logger:    logger.Named("dispatcher"),
		params:    params,
		admission: newRPCAdmission(params.admissionConfig),
	}

	if store != nil {
		d.filterManager = NewFilterManager(logger, store, params.blockRangeLimit)
		d.filterManager.maxFilters = d.rpcLimits().config.MaxFilters
	}
	/* FINISH ALPHA CHANGE 2244 -- validate budgets before creating RPC resources -- */

	if err := d.registerEndpoints(store); err != nil {
		d.Close()

		return nil, err
	}

	/* START ALPHA CHANGE 1410 -- start and close dispatcher-owned filter manager safely -- */
	if d.filterManager != nil {
		go d.filterManager.Run()
	}
	/* FINISH ALPHA CHANGE 1410 -- start and close dispatcher-owned filter manager safely -- */

	return d, nil
}

/* START ALPHA CHANGE 1411 -- close dispatcher-owned filter manager -- */
func (d *Dispatcher) Close() {
	if d.filterManager != nil {
		d.filterManager.Close()
	}
}

/* FINISH ALPHA CHANGE 1411 -- close dispatcher-owned filter manager -- */

func (d *Dispatcher) registerEndpoints(store JSONRPCStore) error {
	/* START ALPHA CHANGE 2458 -- configure the endpoint with named fields -- */
	d.endpoints.Eth = &Eth{
		logger:                    d.logger,
		store:                     store,
		chainID:                   d.params.chainID,
		filterManager:             d.filterManager,
		priceLimit:                d.params.priceLimit,
		feeHistoryPercentileLimit: d.params.feeHistoryPercentileLimit,
	}
	/* FINISH ALPHA CHANGE 2458 -- configure the endpoint with named fields -- */
	d.endpoints.Net = &Net{
		store,
		d.params.chainID,
	}
	d.endpoints.Web3 = &Web3{
		d.params.chainName,
	}
	/* START ALPHA CHANGE 1140 -- stop constructing operator-only JSON-RPC endpoints -- */
	/* FINISH ALPHA CHANGE 1140 -- stop constructing operator-only JSON-RPC endpoints -- */

	/* START ALPHA CHANGE 426 -- stop constructing bridge JSON-RPC endpoint -- */
	/* FINISH ALPHA CHANGE 426 -- stop constructing bridge JSON-RPC endpoint -- */

	var err error

	if err = d.registerService("eth", d.endpoints.Eth); err != nil {
		return err
	}

	if err = d.registerService("net", d.endpoints.Net); err != nil {
		return err
	}

	if err = d.registerService("web3", d.endpoints.Web3); err != nil {
		return err
	}

	/* START ALPHA CHANGE 1141 -- stop exposing operator-only JSON-RPC namespaces -- */
	/* FINISH ALPHA CHANGE 1141 -- stop exposing operator-only JSON-RPC namespaces -- */

	/* START ALPHA CHANGE 427 -- stop registering bridge JSON-RPC namespace -- */
	/* FINISH ALPHA CHANGE 427 -- stop registering bridge JSON-RPC namespace -- */

	return nil
}

func (d *Dispatcher) getFnHandler(req Request) (*serviceData, *funcData, Error) {
	callName := strings.SplitN(req.Method, "_", 2)
	if len(callName) != 2 {
		return nil, nil, NewMethodNotFoundError(req.Method)
	}

	serviceName, funcName := callName[0], callName[1]

	service, ok := d.serviceMap[serviceName]
	if !ok {
		return nil, nil, NewMethodNotFoundError(req.Method)
	}

	fd, ok := service.funcMap[funcName]

	if !ok {
		return nil, nil, NewMethodNotFoundError(req.Method)
	}

	return service, fd, nil
}

type wsConn interface {
	WriteMessage(messageType int, data []byte) error
	GetFilterID() string
	SetFilterID(string)
}

// as per https://www.jsonrpc.org/specification, the `id` in JSON-RPC 2.0
// can only be a string or a non-decimal integer
/* START ALPHA CHANGE 2181 -- use the any alias for empty interfaces -- */
func formatID(id any) (any, Error) {
	/* FINISH ALPHA CHANGE 2181 -- use the any alias for empty interfaces -- */
	switch t := id.(type) {
	case string:
		return t, nil
	case float64:
		if t == math.Trunc(t) {
			return int(t), nil
		} else {
			return "", NewInvalidRequestError("Invalid json request")
		}
	case nil:
		return nil, nil
	default:
		return "", NewInvalidRequestError("Invalid json request")
	}
}

func (d *Dispatcher) handleSubscribe(req Request, conn wsConn) (string, Error) {
	/* START ALPHA CHANGE 2181 -- use the any alias for subscription parameters -- */
	var params []any
	/* FINISH ALPHA CHANGE 2181 -- use the any alias for subscription parameters -- */
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return "", NewInvalidRequestError("Invalid json request")
	}

	if len(params) == 0 {
		return "", NewInvalidParamsError("Invalid params")
	}

	subscribeMethod, ok := params[0].(string)
	if !ok {
		return "", NewSubscriptionNotFoundError(subscribeMethod)
	}

	/* START ALPHA CHANGE 436 -- use tagged switch for subscription method dispatch -- */
	var filterID string
	switch subscribeMethod {
	case "newHeads":
		filterID = d.filterManager.NewBlockFilter(conn)
	case "logs":
		/* START ALPHA CHANGE 2184 -- reject missing logs filter before indexing subscription parameters -- */
		if len(params) < 2 {
			return "", NewInvalidParamsError("Missing logs filter parameter")
		}
		/* FINISH ALPHA CHANGE 2184 -- reject missing logs filter before indexing subscription parameters -- */

		/* START ALPHA CHANGE 2512 -- classify invalid subscription filters as invalid parameters -- */
		logQuery, err := decodeLogQueryFromInterface(params[1])
		if err != nil {
			return "", NewInvalidParamsError(err.Error())
		}
		filterID = d.filterManager.NewLogFilter(logQuery, conn)
		/* FINISH ALPHA CHANGE 2512 -- classify invalid subscription filters as invalid parameters -- */
	case "newPendingTransactions":
		filterID = d.filterManager.NewPendingTxFilter(conn)
	default:
		return "", NewSubscriptionNotFoundError(subscribeMethod)
	}
	/* FINISH ALPHA CHANGE 436 -- use tagged switch for subscription method dispatch -- */

	/* START ALPHA CHANGE 2547 -- report subscription capacity or feed unavailability -- */
	if filterID == "" {
		return "", &rpcLimitError{
			"RPC subscription capacity exceeded or event feed temporarily unavailable",
		}
	}
	/* FINISH ALPHA CHANGE 2547 -- report subscription capacity or feed unavailability -- */

	return filterID, nil
}

func (d *Dispatcher) handleUnsubscribe(req Request) (bool, Error) {
	/* START ALPHA CHANGE 2181 -- use the any alias for unsubscription parameters -- */
	var params []any
	/* FINISH ALPHA CHANGE 2181 -- use the any alias for unsubscription parameters -- */
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return false, NewInvalidRequestError("Invalid json request")
	}

	if len(params) != 1 {
		return false, NewInvalidParamsError("Invalid params")
	}

	filterID, ok := params[0].(string)
	if !ok {
		return false, NewSubscriptionNotFoundError(filterID)
	}

	return d.filterManager.Uninstall(filterID), nil
}

func (d *Dispatcher) RemoveFilterByWs(conn wsConn) {
	d.filterManager.RemoveFilterByWs(conn)
}

func (d *Dispatcher) HandleWs(reqBody []byte, conn wsConn) ([]byte, error) {
	const (
		openSquareBracket  byte = '['
		closeSquareBracket byte = ']'
		comma              byte = ','
	)

	reqBody = bytes.TrimLeft(reqBody, " \t\r\n")

	// if body begins with [ consider it as a batch request
	if len(reqBody) > 0 && reqBody[0] == openSquareBracket {
		var batchReq BatchRequest

		err := json.Unmarshal(reqBody, &batchReq)
		if err != nil {
			return NewRPCResponse(nil, "2.0", nil,
				NewInvalidRequestError("Invalid json batch request")).Bytes()
		}

		// if not disabled, avoid handling long batch requests
		if d.params.isExceedingBatchLengthLimit(uint64(len(batchReq))) {
			return NewRPCResponse(
				nil,
				"2.0",
				nil,
				NewInvalidRequestError("Batch request length too long"),
			).Bytes()
		}

		/* START ALPHA CHANGE 2245 -- admit complete WebSocket batches before execution -- */
		if !d.rpcLimits().allowCalls(len(batchReq)) {
			return rpcCallLimitResponse(batchReq, true)
		}
		/* FINISH ALPHA CHANGE 2245 -- admit complete WebSocket batches before execution -- */

		responses := make([][]byte, len(batchReq))

		for i, req := range batchReq {
			responses[i], err = d.handleSingleWs(req, conn).Bytes()
			if err != nil {
				return nil, err
			}
		}

		var buf bytes.Buffer

		// batch output should look like:
		// [ { "requestId": "1", "status": 200 }, { "requestId": "2", "status": 200 } ]
		buf.WriteByte(openSquareBracket)                // [
		buf.Write(bytes.Join(responses, []byte{comma})) // join responses with the comma separator
		buf.WriteByte(closeSquareBracket)               // ]

		return buf.Bytes(), nil
	}

	var req Request
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return NewRPCResponse(req.ID, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
	}

	/* START ALPHA CHANGE 2245 -- charge single WebSocket RPC calls -- */
	if !d.rpcLimits().allowCalls(1) {
		return rpcCallLimitResponse(BatchRequest{req}, false)
	}
	/* FINISH ALPHA CHANGE 2245 -- charge single WebSocket RPC calls -- */

	return d.handleSingleWs(req, conn).Bytes()
}

func (d *Dispatcher) handleSingleWs(req Request, conn wsConn) Response {
	id, err := formatID(req.ID)
	if err != nil {
		return NewRPCResponse(nil, "2.0", nil, err)
	}

	var response []byte

	switch req.Method {
	case "eth_subscribe":
		var filterID string

		// if the request method is eth_subscribe we need to create a new filter with ws connection
		if filterID, err = d.handleSubscribe(req, conn); err == nil {
			/* START ALPHA CHANGE 824 -- use fmt.Appendf for websocket subscribe response -- */
			response = fmt.Appendf(nil, "\"%s\"", filterID)
			/* FINISH ALPHA CHANGE 824 -- use fmt.Appendf for websocket subscribe response -- */
		}
	case "eth_unsubscribe":
		var ok bool

		if ok, err = d.handleUnsubscribe(req); err == nil {
			response = []byte(strconv.FormatBool(ok))
		}
	default:
		// its a normal query that we handle with the dispatcher
		response, err = d.handleReq(req)
	}

	return NewRPCResponse(id, "2.0", response, err)
}

func (d *Dispatcher) Handle(reqBody []byte) ([]byte, error) {
	x := bytes.TrimLeft(reqBody, " \t\r\n")
	if len(x) == 0 {
		return NewRPCResponse(nil, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
	}

	if x[0] == '{' {
		var req Request
		if err := json.Unmarshal(reqBody, &req); err != nil {
			return NewRPCResponse(nil, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
		}

		if req.Method == "" {
			return NewRPCResponse(req.ID, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
		}

		/* START ALPHA CHANGE 2245 -- charge single HTTP RPC calls -- */
		if !d.rpcLimits().allowCalls(1) {
			return rpcCallLimitResponse(BatchRequest{req}, false)
		}
		/* FINISH ALPHA CHANGE 2245 -- charge single HTTP RPC calls -- */

		resp, err := d.handleReq(req)

		return NewRPCResponse(req.ID, "2.0", resp, err).Bytes()
	}

	// handle batch requests
	var requests BatchRequest
	if err := json.Unmarshal(reqBody, &requests); err != nil {
		return NewRPCResponse(
			nil,
			"2.0",
			nil,
			NewInvalidRequestError("Invalid json request"),
		).Bytes()
	}

	// if not disabled, avoid handling long batch requests
	if d.params.isExceedingBatchLengthLimit(uint64(len(requests))) {
		return NewRPCResponse(
			nil,
			"2.0",
			nil,
			NewInvalidRequestError("Batch request length too long"),
		).Bytes()
	}

	/* START ALPHA CHANGE 2245 -- admit complete HTTP batches before execution -- */
	if !d.rpcLimits().allowCalls(len(requests)) {
		return rpcCallLimitResponse(requests, true)
	}
	/* FINISH ALPHA CHANGE 2245 -- admit complete HTTP batches before execution -- */

	responses := make([]Response, 0)

	for _, req := range requests {
		var response, err = d.handleReq(req)
		if err != nil {
			errorResponse := NewRPCResponse(req.ID, "2.0", response, err)
			responses = append(responses, errorResponse)

			continue
		}

		resp := NewRPCResponse(req.ID, "2.0", response, nil)
		responses = append(responses, resp)
	}

	respBytes, err := json.Marshal(responses)
	if err != nil {
		return NewRPCResponse(nil, "2.0", nil, NewInternalError("Internal error")).Bytes()
	}

	return respBytes, nil
}

func (d *Dispatcher) handleReq(req Request) ([]byte, Error) {
	d.logger.Debug("request", "method", req.Method, "id", req.ID)

	service, fd, ferr := d.getFnHandler(req)
	if ferr != nil {
		return nil, ferr
	}

	/* START ALPHA CHANGE 2246 -- bound expensive RPC execution without queueing -- */
	release, admissionErr := d.rpcLimits().acquireExecution(req.Method)
	if admissionErr != nil {
		return nil, admissionErr
	}
	if release != nil {
		defer release()
	}
	/* FINISH ALPHA CHANGE 2246 -- bound expensive RPC execution without queueing -- */

	inArgs := make([]reflect.Value, fd.inNum)
	inArgs[0] = service.sv

	/* START ALPHA CHANGE 2181 -- use the any alias for decoded RPC inputs -- */
	inputs := make([]any, fd.numParams())
	/* FINISH ALPHA CHANGE 2181 -- use the any alias for decoded RPC inputs -- */

	for i := 0; i < fd.inNum-1; i++ {
		val := reflect.New(fd.reqt[i+1])
		inputs[i] = val.Interface()
		inArgs[i+1] = val.Elem()
	}

	if fd.numParams() > 0 {
		if err := json.Unmarshal(req.Params, &inputs); err != nil {
			return nil, NewInvalidParamsError("Invalid Params")
		}
	}

	var (
		data []byte
		err  error
		ok   bool
	)

	start := time.Now().UTC()
	output := fd.fv.Call(inArgs) // call rpc endpoint function
	// measure execution time of rpc endpoint function
	metrics.SetGauge([]string{jsonRPCMetric, req.Method + "_time"}, float32(time.Now().UTC().Sub(start).Seconds()))

	if err := getError(output[1]); err != nil {
		/* START ALPHA CHANGE 2512 -- preserve explicit endpoint parameter errors without warning spam -- */
		var paramsErr *invalidParamsError
		if errors.As(err, &paramsErr) {
			return nil, paramsErr
		}
		/* FINISH ALPHA CHANGE 2512 -- preserve explicit endpoint parameter errors without warning spam -- */

		/* START ALPHA CHANGE 2246 -- report resource limits without per-rejection warnings -- */
		var limitErr *rpcLimitError
		if errors.As(err, &limitErr) {
			return nil, limitErr
		}
		/* FINISH ALPHA CHANGE 2246 -- report resource limits without per-rejection warnings -- */

		// measure error on the rpc endpoint function
		metrics.IncrCounter([]string{jsonRPCMetric, req.Method + "_errors"}, 1)
		d.logInternalError(req.Method, err)

		/* START ALPHA CHANGE 2170 -- return internal errors without unreliable result data -- */
		if errors.Is(err, state.ErrLocalStateFailure) {
			return nil, NewInternalError(err.Error())
		}
		/* FINISH ALPHA CHANGE 2170 -- return internal errors without unreliable result data -- */

		if res := output[0].Interface(); res != nil {
			data, ok = res.([]byte)

			if !ok {
				return nil, NewInternalError(err.Error())
			}
		}

		return data, NewInvalidRequestError(err.Error())
	}

	if res := output[0].Interface(); res != nil {
		data, err = json.Marshal(res)
		if err != nil {
			d.logInternalError(req.Method, err)

			return nil, NewInternalError("Internal error")
		}
	}

	return data, nil
}

/* START ALPHA CHANGE 2170 -- bound repeated RPC local state failure messages -- */
/* START ALPHA CHANGE 2246 -- bound ordinary RPC endpoint error logging -- */
func (d *Dispatcher) logInternalError(method string, err error) {
	if state.LogLocalStateFailure(d.logger, state.LocalStateLogRPC, err) {
		return
	}

	allowed, suppressed := d.rpcLimits().allowErrorLog()
	if !allowed {
		return
	}

	d.logger.Warn(
		"failed to dispatch",
		"method", method,
		"err", err,
		"suppressed", suppressed,
	)
}

/* FINISH ALPHA CHANGE 2246 -- bound ordinary RPC endpoint error logging -- */

/* FINISH ALPHA CHANGE 2170 -- bound repeated RPC local state failure messages -- */

/* START ALPHA CHANGE 2181 -- use the any alias for the registered service -- */
func (d *Dispatcher) registerService(serviceName string, service any) error {
	/* FINISH ALPHA CHANGE 2181 -- use the any alias for the registered service -- */
	if d.serviceMap == nil {
		d.serviceMap = map[string]*serviceData{}
	}

	if serviceName == "" {
		return errors.New("jsonrpc: serviceName cannot be empty")
	}

	st := reflect.TypeOf(service)
	if st.Kind() == reflect.Struct {
		/*START ALPHA CHANGE 35 -- use fmt.Errorf instead of errors.New(fmt.Sprintf(...)) --*/
		return fmt.Errorf("jsonrpc: service '%s' must be a pointer to struct", serviceName)
		/*FINISH ALPHA CHANGE 35 -- use fmt.Errorf instead of errors.New(fmt.Sprintf(...)) --*/
	}

	funcMap := make(map[string]*funcData)

	/* START ALPHA CHANGE 2182 -- iterate reflected type methods directly -- */
	for mv := range st.Methods() {
		/* FINISH ALPHA CHANGE 2182 -- iterate reflected type methods directly -- */
		if mv.PkgPath != "" {
			// skip unexported methods
			continue
		}

		name := lowerCaseFirst(mv.Name)
		funcName := serviceName + "_" + name
		fd := &funcData{
			fv: mv.Func,
		}

		var err error

		if fd.inNum, fd.reqt, err = validateFunc(funcName, fd.fv, true); err != nil {
			return fmt.Errorf("jsonrpc: %w", err)
		}
		// check if last item is a pointer
		if fd.numParams() != 0 {
			last := fd.reqt[fd.numParams()]
			if last.Kind() == reflect.Ptr {
				fd.isDyn = true
			}
		}

		funcMap[name] = fd
	}

	d.serviceMap[serviceName] = &serviceData{
		sv:      reflect.ValueOf(service),
		funcMap: funcMap,
	}

	return nil
}

func validateFunc(funcName string, fv reflect.Value, _ bool) (inNum int, reqt []reflect.Type, err error) {
	if funcName == "" {
		err = fmt.Errorf("funcName cannot be empty")

		return
	}

	ft := fv.Type()
	if ft.Kind() != reflect.Func {
		err = fmt.Errorf("function '%s' must be a function instead of %s", funcName, ft)

		return
	}

	inNum = ft.NumIn()

	if outNum := ft.NumOut(); ft.NumOut() != 2 {
		err = fmt.Errorf("unexpected number of output arguments in the function '%s': %d. Expected 2", funcName, outNum)

		return
	}

	if !isErrorType(ft.Out(1)) {
		err = fmt.Errorf(
			"unexpected type for the second return value of the function '%s': '%s'. Expected '%s'",
			funcName,
			ft.Out(1),
			errt,
		)

		return
	}

	reqt = make([]reflect.Type, inNum)
	for i := 0; i < inNum; i++ {
		reqt[i] = ft.In(i)
	}

	return
}

/* START ALPHA CHANGE 2183 -- obtain the error interface type directly -- */
var errt = reflect.TypeFor[error]()

/* FINISH ALPHA CHANGE 2183 -- obtain the error interface type directly -- */

func isErrorType(t reflect.Type) bool {
	return t.Implements(errt)
}

func getError(v reflect.Value) error {
	if v.IsNil() {
		return nil
	}

	extractedErr, ok := v.Interface().(error)
	if !ok {
		return errors.New("invalid type assertion, unable to extract error")
	}

	return extractedErr
}

func lowerCaseFirst(str string) string {
	for i, v := range str {
		return string(unicode.ToLower(v)) + str[i+1:]
	}

	return ""
}
