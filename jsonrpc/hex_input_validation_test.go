/* START ALPHA CHANGE 3093 -- verify hexadecimal errors stop RPC dispatch before endpoint and backend work -- */
package jsonrpc

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/tracer"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type hexInputFormat struct {
	name  string
	input string
	want  []byte
}

func hexInputAcceptedFormats() []hexInputFormat {
	return []hexInputFormat{
		{"empty_prefixed", "0x", []byte{}},
		{"empty_prefixless", "", []byte{}},
		{"prefixed_even", "0x1234", []byte{0x12, 0x34}},
		{"prefixed_odd", "0xabc", []byte{0x0a, 0xbc}},
		{"prefixless_even", "1234", []byte{0x12, 0x34}},
		{"prefixless_odd", "abc", []byte{0x0a, 0xbc}},
		{"uppercase_digits", "0xAbCd", []byte{0xab, 0xcd}},
		{"leading_zero_bytes", "0x0001", []byte{0x00, 0x01}},
		{"single_zero_digit", "0x0", []byte{0x00}},
	}
}

func hexInputMalformedFormats() []string {
	return []string{
		"0xzz",
		"0x1234zz",
		"12zz",
		"0x12 34",
	}
}

func TestHexInputValidation_AcceptedFormats(t *testing.T) {
	for _, tc := range hexInputAcceptedFormats() {
		t.Run(tc.name, func(t *testing.T) {
			value := argBytes{0xaa, 0xbb}
			require.NoError(t, value.UnmarshalText([]byte(tc.input)))
			require.Equal(t, tc.want, []byte(value))

			encoded, err := json.Marshal(tc.input)
			require.NoError(t, err)

			value = argBytes{0xaa, 0xbb}
			require.NoError(t, json.Unmarshal(encoded, &value))
			require.Equal(t, tc.want, []byte(value))

			roundTrip, err := json.Marshal(value)
			require.NoError(t, err)
			require.Equal(t,
				`"0x`+hex.EncodeToString(tc.want)+`"`,
				string(roundTrip),
			)
		})
	}
}

func TestHexInputValidation_ErrorPreservesReceiver(t *testing.T) {
	for _, input := range hexInputMalformedFormats() {
		for _, throughJSON := range []bool{false, true} {
			mode := "text"
			if throughJSON {
				mode = "json"
			}

			t.Run(mode+"/"+input, func(t *testing.T) {
				for _, initial := range []argBytes{
					nil,
					{0xaa, 0xbb},
				} {
					value := append(argBytes(nil), initial...)
					before := append(argBytes(nil), value...)

					var err error
					if throughJSON {
						encoded, marshalErr := json.Marshal(input)
						require.NoError(t, marshalErr)
						err = json.Unmarshal(encoded, &value)
					} else {
						err = value.UnmarshalText([]byte(input))
					}

					require.Error(t, err)
					require.Equal(t, before, value)
				}
			})
		}
	}
}

type hexInputCounters struct {
	endpoints atomic.Int32
	backend   atomic.Int32
	apply     atomic.Int32
	trace     atomic.Int32
	pool      atomic.Int32
}

type hexInputStore struct {
	JSONRPCStore
	counts *hexInputCounters
	header *types.Header
}

func (s *hexInputStore) Header() *types.Header {
	s.counts.backend.Add(1)
	return s.header.Copy()
}

func (s *hexInputStore) GetHeaderByNumber(number uint64) (*types.Header, bool) {
	s.counts.backend.Add(1)
	if number != s.header.Number {
		return nil, false
	}
	return s.header.Copy(), true
}

func (s *hexInputStore) GetAccount(types.Hash, types.Address) (*Account, error) {
	s.counts.backend.Add(1)
	return &Account{
		Balance: big.NewInt(1_000_000_000),
		Nonce:   0,
	}, nil
}

func (s *hexInputStore) GetNonce(types.Address) (uint64, error) {
	s.counts.backend.Add(1)
	return 0, nil
}

func (s *hexInputStore) GetBaseFee() uint64 {
	s.counts.backend.Add(1)
	return s.header.BaseFee
}

func (s *hexInputStore) GetForksInTime(number uint64) chain.ForksInTime {
	s.counts.backend.Add(1)
	return chain.AllForksEnabled.At(number)
}

func (s *hexInputStore) MaxPriorityFeePerGas() (*big.Int, error) {
	s.counts.backend.Add(1)
	return big.NewInt(1), nil
}

func (s *hexInputStore) GetAvgGasPrice() *big.Int {
	s.counts.backend.Add(1)
	return big.NewInt(2)
}

func (s *hexInputStore) ApplyTxn(
	_ *types.Header,
	tx *types.Transaction,
	_ types.StateOverride,
	_ bool,
) (*runtime.ExecutionResult, error) {
	s.counts.backend.Add(1)
	s.counts.apply.Add(1)
	return &runtime.ExecutionResult{
		ReturnValue: append([]byte(nil), tx.Input...),
		GasUsed:     tx.Gas,
		GasLeft:     0,
	}, nil
}

func (s *hexInputStore) TraceCall(
	tx *types.Transaction,
	_ *types.Header,
	_ tracer.Tracer,
) (any, error) {
	s.counts.backend.Add(1)
	s.counts.trace.Add(1)
	return map[string]any{
		"input": argBytes(append([]byte(nil), tx.Input...)),
	}, nil
}

func (s *hexInputStore) AddTx(tx *types.Transaction) error {
	s.counts.backend.Add(1)
	s.counts.pool.Add(1)
	tx.ComputeHash(s.header.Number)
	return nil
}

type hexInputEthProbe struct {
	*Eth
	counts *hexInputCounters
}

func (p *hexInputEthProbe) Call(
	arg *txnArgs,
	filter BlockNumberOrHash,
	override *stateOverride,
) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.Call(arg, filter, override)
}

func (p *hexInputEthProbe) EstimateGas(
	arg *txnArgs,
	number *BlockNumber,
) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.EstimateGas(arg, number)
}

func (p *hexInputEthProbe) SendRawTransaction(raw argBytes) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.SendRawTransaction(raw)
}

type hexInputDebugProbe struct {
	*Debug
	counts *hexInputCounters
}

func (p *hexInputDebugProbe) TraceCall(
	arg *txnArgs,
	filter BlockNumberOrHash,
	config *TraceConfig,
) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Debug.TraceCall(arg, filter, config)
}

type hexInputWeb3Probe struct {
	*Web3
	counts *hexInputCounters
}

func (p *hexInputWeb3Probe) Sha3(input argBytes) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Web3.Sha3(input)
}

func hexInputDispatcher(t *testing.T) (*Dispatcher, *hexInputCounters) {
	t.Helper()

	counts := new(hexInputCounters)
	store := &hexInputStore{
		counts: counts,
		header: &types.Header{
			Number:    1,
			GasLimit:  1_000_000,
			BaseFee:   1,
			StateRoot: types.EmptyRootHash,
		},
	}
	logger := hclog.NewNullLogger()
	dispatcher := &Dispatcher{
		logger: logger,
		params: &dispatcherParams{},
	}
	t.Cleanup(dispatcher.Close)

	require.NoError(t, dispatcher.registerService("eth", &hexInputEthProbe{
		Eth: &Eth{
			logger:  logger,
			store:   store,
			chainID: 1982,
		},
		counts: counts,
	}))
	require.NoError(t, dispatcher.registerService("debug", &hexInputDebugProbe{
		Debug:  NewDebug(store, 4),
		counts: counts,
	}))
	require.NoError(t, dispatcher.registerService("web3", &hexInputWeb3Probe{
		Web3:   &Web3{chainName: "innbc-chain"},
		counts: counts,
	}))

	return dispatcher, counts
}

type hexInputRPCResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func hexInputRequest(
	t *testing.T,
	dispatcher *Dispatcher,
	method string,
	params []any,
) hexInputRPCResponse {
	t.Helper()

	request, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	require.NoError(t, err)

	body, err := dispatcher.Handle(request)
	require.NoError(t, err)

	var response hexInputRPCResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Equal(t, 1, response.ID)
	return response
}

func hexInputRequireError(t *testing.T, response hexInputRPCResponse, code int) {
	t.Helper()

	require.NotNil(t, response.Error)
	require.Equal(t, code, response.Error.Code)
	if code == -32602 {
		require.Equal(t, "Invalid Params", response.Error.Message)
	}
	require.True(t,
		len(response.Result) == 0 || string(response.Result) == "null",
	)
}

func hexInputRequireNoWork(t *testing.T, counts *hexInputCounters) {
	t.Helper()

	require.Zero(t, counts.endpoints.Load())
	require.Zero(t, counts.backend.Load())
	require.Zero(t, counts.apply.Load())
	require.Zero(t, counts.trace.Load())
	require.Zero(t, counts.pool.Load())
}

func hexInputTransactionArgs() map[string]any {
	return map[string]any{
		"from":      types.StringToAddress("0x9001").String(),
		"to":        types.StringToAddress("0x9002").String(),
		"gas":       "0xc350",
		"value":     "0x1",
		"gasPrice":  "0x2",
		"gasTipCap": "0x1",
		"gasFeeCap": "0x2",
		"data":      "0x",
	}
}

func hexInputSimulationParams(method string, args map[string]any) []any {
	if method == "debug_traceCall" {
		return []any{args, "latest", map[string]any{}}
	}
	return []any{args, "latest"}
}

func TestHexInputValidation_TransactionFieldsBeforeExecution(t *testing.T) {
	for _, method := range []string{
		"eth_call",
		"eth_estimateGas",
		"debug_traceCall",
	} {
		for _, field := range []string{
			"data",
			"input",
			"value",
			"gasPrice",
			"gasTipCap",
			"gasFeeCap",
		} {
			for _, malformed := range hexInputMalformedFormats() {
				t.Run(method+"/"+field+"/"+malformed, func(t *testing.T) {
					dispatcher, counts := hexInputDispatcher(t)
					args := hexInputTransactionArgs()
					args[field] = malformed

					response := hexInputRequest(t, dispatcher, method,
						hexInputSimulationParams(method, args),
					)
					hexInputRequireError(t, response, -32602)
					hexInputRequireNoWork(t, counts)

					// A healthy counterpart must reach the registered endpoint
					// and the applicable backend path.
					args[field] = "0x1"
					response = hexInputRequest(t, dispatcher, method,
						hexInputSimulationParams(method, args),
					)
					require.Nil(t, response.Error)
					require.Equal(t, int32(1), counts.endpoints.Load())
					require.True(t, counts.backend.Load() > 0)

					if method == "debug_traceCall" {
						require.Equal(t, int32(1), counts.trace.Load())
						require.Zero(t, counts.apply.Load())
					} else {
						require.True(t, counts.apply.Load() > 0)
						require.Zero(t, counts.trace.Load())
					}
					require.Zero(t, counts.pool.Load())
				})
			}
		}
	}
}

func TestHexInputValidation_OverrideCodeBeforeExecution(t *testing.T) {
	address := types.StringToAddress("0x9002").String()

	for _, malformed := range hexInputMalformedFormats() {
		t.Run(malformed, func(t *testing.T) {
			dispatcher, counts := hexInputDispatcher(t)
			account := map[string]any{"code": malformed}
			override := map[string]any{address: account}
			params := []any{hexInputTransactionArgs(), "latest", override}

			response := hexInputRequest(t, dispatcher, "eth_call", params)
			hexInputRequireError(t, response, -32602)
			hexInputRequireNoWork(t, counts)

			account["code"] = "0x00"
			response = hexInputRequest(t, dispatcher, "eth_call", params)
			require.Nil(t, response.Error)
			require.Equal(t, int32(1), counts.endpoints.Load())
			require.Equal(t, int32(1), counts.apply.Load())
			require.True(t, counts.backend.Load() > 0)
			require.Zero(t, counts.trace.Load())
			require.Zero(t, counts.pool.Load())
		})
	}
}

func TestHexInputValidation_RawSubmissionBeforeAdmission(t *testing.T) {
	key, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	to := types.StringToAddress("0x9002")
	signed, err := crypto.NewFrontierSigner(true).SignTx(&types.Transaction{
		Type:     types.LegacyTx,
		To:       &to,
		Gas:      21_000,
		GasPrice: big.NewInt(2),
		Value:    big.NewInt(1),
	}, key)
	require.NoError(t, err)
	validRaw := "0x" + hex.EncodeToString(signed.MarshalRLP())

	for _, malformed := range hexInputMalformedFormats() {
		t.Run(malformed, func(t *testing.T) {
			dispatcher, counts := hexInputDispatcher(t)

			response := hexInputRequest(t, dispatcher,
				"eth_sendRawTransaction", []any{malformed},
			)
			hexInputRequireError(t, response, -32602)
			hexInputRequireNoWork(t, counts)

			response = hexInputRequest(t, dispatcher,
				"eth_sendRawTransaction", []any{validRaw},
			)
			require.Nil(t, response.Error)
			require.Equal(t, int32(1), counts.endpoints.Load())
			require.Equal(t, int32(1), counts.pool.Load())
			require.Equal(t, int32(1), counts.backend.Load())
			require.Zero(t, counts.apply.Load())
			require.Zero(t, counts.trace.Load())
		})
	}
}

func TestHexInputValidation_SHA3BeforeExecution(t *testing.T) {
	for _, malformed := range hexInputMalformedFormats() {
		t.Run("invalid/"+malformed, func(t *testing.T) {
			dispatcher, counts := hexInputDispatcher(t)
			response := hexInputRequest(t, dispatcher,
				"web3_sha3", []any{malformed},
			)
			hexInputRequireError(t, response, -32602)
			hexInputRequireNoWork(t, counts)
		})
	}

	for _, tc := range hexInputAcceptedFormats() {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			dispatcher, counts := hexInputDispatcher(t)
			response := hexInputRequest(t, dispatcher,
				"web3_sha3", []any{tc.input},
			)
			require.Nil(t, response.Error)

			var result string
			require.NoError(t, json.Unmarshal(response.Result, &result))
			require.Equal(t,
				"0x"+hex.EncodeToString(crypto.Keccak256(tc.want)),
				result,
			)
			require.Equal(t, int32(1), counts.endpoints.Load())
			require.Zero(t, counts.backend.Load())
			require.Zero(t, counts.apply.Load())
			require.Zero(t, counts.trace.Load())
			require.Zero(t, counts.pool.Load())
		})
	}
}

func TestHexInputValidation_AdmissionPrecedence(t *testing.T) {
	dispatcher, counts := hexInputDispatcher(t)
	config := DefaultAdmissionConfig()
	config.MaxExpensive = 1
	require.NoError(t, config.Validate())
	dispatcher.admission = newRPCAdmission(config)

	args := hexInputTransactionArgs()
	args["data"] = "0xzz"
	params := []any{args, "latest"}

	func() {
		release, err := dispatcher.rpcLimits().acquireExecution("eth_call")
		require.NoError(t, err)
		require.NotNil(t, release)
		defer release()

		response := hexInputRequest(t, dispatcher, "eth_call", params)
		hexInputRequireError(t, response, -32005)
		hexInputRequireNoWork(t, counts)
	}()

	response := hexInputRequest(t, dispatcher, "eth_call", params)
	hexInputRequireError(t, response, -32602)
	hexInputRequireNoWork(t, counts)
}

/* FINISH ALPHA CHANGE 3093 -- verify hexadecimal errors stop RPC dispatch before endpoint and backend work -- */
