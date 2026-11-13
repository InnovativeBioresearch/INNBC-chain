/* START ALPHA CHANGE 3176 -- reject malformed typed RPC identifiers before endpoint and backend work -- */
package jsonrpc

import (
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type typedHexStore struct {
	*hexInputStore
}

func (s *typedHexStore) GetBlockByHash(hash types.Hash, _ bool) (*types.Block, bool) {
	s.counts.backend.Add(1)
	header := s.header.Copy()
	header.Hash = hash
	return &types.Block{Header: header}, true
}

func (s *typedHexStore) ReadTxLookup(types.Hash) (types.Hash, bool) {
	s.counts.backend.Add(1)
	return types.ZeroHash, false
}

func (s *typedHexStore) GetPendingTx(types.Hash) (*types.Transaction, bool) {
	s.counts.backend.Add(1)
	return nil, false
}

func (s *typedHexStore) GetStorage(types.Hash, types.Address, types.Hash) ([]byte, error) {
	s.counts.backend.Add(1)
	return make([]byte, types.HashLength), nil
}

func (s *typedHexStore) GetCode(types.Hash, types.Address) ([]byte, error) {
	s.counts.backend.Add(1)
	return []byte{0}, nil
}

func (s *typedHexStore) FilterExtra(extra []byte) ([]byte, error) {
	s.counts.backend.Add(1)
	return append([]byte(nil), extra...), nil
}

type typedHexEthProbe struct {
	*hexInputEthProbe
}

func (p *typedHexEthProbe) GetBlockByHash(hash types.Hash, full bool) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.GetBlockByHash(hash, full)
}

func (p *typedHexEthProbe) GetTransactionByHash(hash types.Hash) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.GetTransactionByHash(hash)
}

func (p *typedHexEthProbe) GetTransactionReceipt(hash types.Hash) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.GetTransactionReceipt(hash)
}

func (p *typedHexEthProbe) GetBalance(address types.Address, filter BlockNumberOrHash) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.GetBalance(address, filter)
}

func (p *typedHexEthProbe) GetTransactionCount(address types.Address, filter BlockNumberOrHash) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.GetTransactionCount(address, filter)
}

func (p *typedHexEthProbe) GetCode(address types.Address, filter BlockNumberOrHash) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.GetCode(address, filter)
}

func (p *typedHexEthProbe) GetStorageAt(
	address types.Address,
	index types.Hash,
	filter BlockNumberOrHash,
) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.GetStorageAt(address, index, filter)
}

func (p *typedHexEthProbe) GetLogs(query *LogQuery) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.GetLogs(query)
}

func typedHexDispatcher(t *testing.T) (*Dispatcher, *hexInputCounters) {
	t.Helper()

	counts := new(hexInputCounters)
	store := &typedHexStore{
		hexInputStore: &hexInputStore{
			counts: counts,
			header: &types.Header{
				Number:    1,
				Hash:      types.StringToHash("0x01"),
				GasLimit:  1_000_000,
				BaseFee:   1,
				StateRoot: types.EmptyRootHash,
			},
		},
	}

	logger := hclog.NewNullLogger()
	dispatcher := &Dispatcher{
		logger: logger,
		params: &dispatcherParams{},
	}
	t.Cleanup(dispatcher.Close)

	// All log-query cases below use blockHash. This fixture exercises the
	// synchronous query path without starting filter-manager workers.
	eth := &Eth{
		logger:        logger,
		store:         store,
		chainID:       1982,
		filterManager: &FilterManager{store: store},
	}

	require.NoError(t, dispatcher.registerService("eth", &typedHexEthProbe{
		hexInputEthProbe: &hexInputEthProbe{
			Eth:    eth,
			counts: counts,
		},
	}))
	require.NoError(t, dispatcher.registerService("debug", &hexInputDebugProbe{
		Debug:  NewDebug(store, 4),
		counts: counts,
	}))

	return dispatcher, counts
}

type typedHexRequestCase struct {
	name        string
	method      string
	params      []any
	validParams []any
}

func typedHexRequestCases(malformed string) []typedHexRequestCase {
	address := "0x" + strings.Repeat("11", types.AddressLength)
	hash := "0x" + strings.Repeat("22", types.HashLength)

	cases := []typedHexRequestCase{
		{
			"block_hash", "eth_getBlockByHash",
			[]any{malformed, false},
			[]any{hash, false},
		},
		{
			"transaction_hash", "eth_getTransactionByHash",
			[]any{malformed},
			[]any{hash},
		},
		{
			"receipt_hash", "eth_getTransactionReceipt",
			[]any{malformed},
			[]any{hash},
		},
		{
			"balance_address", "eth_getBalance",
			[]any{malformed, "latest"},
			[]any{address, "latest"},
		},
		{
			"nonce_address", "eth_getTransactionCount",
			[]any{malformed, "latest"},
			[]any{address, "latest"},
		},
		{
			"code_address", "eth_getCode",
			[]any{malformed, "latest"},
			[]any{address, "latest"},
		},
		{
			"storage_address", "eth_getStorageAt",
			[]any{malformed, hash, "latest"},
			[]any{address, hash, "latest"},
		},
		{
			"storage_slot", "eth_getStorageAt",
			[]any{address, malformed, "latest"},
			[]any{address, hash, "latest"},
		},
		{
			"balance_object_selector", "eth_getBalance",
			[]any{address, map[string]any{"blockHash": malformed}},
			[]any{address, map[string]any{"blockHash": hash}},
		},
		{
			"nonce_object_selector", "eth_getTransactionCount",
			[]any{address, map[string]any{"blockHash": malformed}},
			[]any{address, map[string]any{"blockHash": hash}},
		},
		{
			"code_object_selector", "eth_getCode",
			[]any{address, map[string]any{"blockHash": malformed}},
			[]any{address, map[string]any{"blockHash": hash}},
		},
		{
			"storage_object_selector", "eth_getStorageAt",
			[]any{address, hash, map[string]any{"blockHash": malformed}},
			[]any{address, hash, map[string]any{"blockHash": hash}},
		},
	}

	for _, method := range []string{
		"eth_call",
		"eth_estimateGas",
		"debug_traceCall",
	} {
		for _, field := range []string{"from", "to"} {
			args := hexInputTransactionArgs()
			args[field] = malformed

			cases = append(cases, typedHexRequestCase{
				name:        method + "/" + field,
				method:      method,
				params:      hexInputSimulationParams(method, args),
				validParams: hexInputSimulationParams(method, hexInputTransactionArgs()),
			})
		}
	}

	for _, method := range []string{"eth_call", "debug_traceCall"} {
		params := []any{
			hexInputTransactionArgs(),
			map[string]any{"blockHash": malformed},
		}
		validParams := []any{
			hexInputTransactionArgs(),
			map[string]any{"blockHash": hash},
		}
		if method == "debug_traceCall" {
			params = append(params, map[string]any{})
			validParams = append(validParams, map[string]any{})
		}

		cases = append(cases, typedHexRequestCase{
			name:        method + "/object_selector",
			method:      method,
			params:      params,
			validParams: validParams,
		})
	}

	cases = append(cases,
		typedHexRequestCase{
			"logs_block_hash", "eth_getLogs",
			[]any{map[string]any{"blockHash": malformed}},
			[]any{map[string]any{"blockHash": hash}},
		},
		typedHexRequestCase{
			"logs_address", "eth_getLogs",
			[]any{map[string]any{"blockHash": hash, "address": malformed}},
			[]any{map[string]any{"blockHash": hash, "address": address}},
		},
		typedHexRequestCase{
			"logs_address_array", "eth_getLogs",
			[]any{map[string]any{"blockHash": hash, "address": []any{malformed}}},
			[]any{map[string]any{"blockHash": hash, "address": []any{address}}},
		},
		typedHexRequestCase{
			"logs_topic", "eth_getLogs",
			[]any{map[string]any{"blockHash": hash, "topics": []any{malformed}}},
			[]any{map[string]any{"blockHash": hash, "topics": []any{hash}}},
		},
		typedHexRequestCase{
			"logs_topic_array", "eth_getLogs",
			[]any{map[string]any{"blockHash": hash, "topics": []any{[]any{malformed}}}},
			[]any{map[string]any{"blockHash": hash, "topics": []any{[]any{hash}}}},
		},
		typedHexRequestCase{
			"override_address_key", "eth_call",
			[]any{
				hexInputTransactionArgs(), "latest",
				map[string]any{malformed: map[string]any{"balance": "0x1"}},
			},
			[]any{
				hexInputTransactionArgs(), "latest",
				map[string]any{address: map[string]any{"balance": "0x1"}},
			},
		},
	)

	for _, field := range []string{"state", "stateDiff"} {
		cases = append(cases,
			typedHexRequestCase{
				name:   "override_" + field + "_key",
				method: "eth_call",
				params: []any{
					hexInputTransactionArgs(), "latest",
					map[string]any{
						address: map[string]any{
							field: map[string]string{malformed: hash},
						},
					},
				},
				validParams: []any{
					hexInputTransactionArgs(), "latest",
					map[string]any{
						address: map[string]any{
							field: map[string]string{hash: hash},
						},
					},
				},
			},
			typedHexRequestCase{
				name:   "override_" + field + "_value",
				method: "eth_call",
				params: []any{
					hexInputTransactionArgs(), "latest",
					map[string]any{
						address: map[string]any{
							field: map[string]string{hash: malformed},
						},
					},
				},
				validParams: []any{
					hexInputTransactionArgs(), "latest",
					map[string]any{
						address: map[string]any{
							field: map[string]string{hash: hash},
						},
					},
				},
			},
		)
	}

	return cases
}

func TestTypedHex_DispatchRejectsBeforeWork(t *testing.T) {
	malformed := []struct {
		name  string
		input string
	}{
		{"invalid", "0xzz"},
		{"partial", "0x1234zz"},
		{"prefixless_partial", "12zz"},
		{"underscore", "0x12_34"},
		{"address_suffix", "0x" + strings.Repeat("11", types.AddressLength) + "zz"},
		{"hash_suffix", "0x" + strings.Repeat("11", types.HashLength) + "zz"},
		{"oversized_invalid_leading", "0xzz" + strings.Repeat("11", types.HashLength+1)},
	}

	for _, input := range malformed {
		for _, tc := range typedHexRequestCases(input.input) {
			t.Run(input.name+"/"+tc.name, func(t *testing.T) {
				dispatcher, counts := typedHexDispatcher(t)

				response := hexInputRequest(t, dispatcher, tc.method, tc.params)
				hexInputRequireError(t, response, -32602)
				hexInputRequireNoWork(t, counts)

				// A valid counterpart must reach the real endpoint and backend.
				response = hexInputRequest(t, dispatcher, tc.method, tc.validParams)
				require.Nil(t, response.Error)
				require.Equal(t, int32(1), counts.endpoints.Load())
				require.True(t, counts.backend.Load() > 0)
				require.Zero(t, counts.pool.Load())

				switch tc.method {
				case "eth_call", "eth_estimateGas":
					require.True(t, counts.apply.Load() > 0)
					require.Zero(t, counts.trace.Load())
				case "debug_traceCall":
					require.Equal(t, int32(1), counts.trace.Load())
					require.Zero(t, counts.apply.Load())
				default:
					require.Zero(t, counts.apply.Load())
					require.Zero(t, counts.trace.Load())
				}
			})
		}
	}
}

func TestTypedHex_AdmissionPrecedence(t *testing.T) {
	dispatcher, counts := typedHexDispatcher(t)

	config := DefaultAdmissionConfig()
	config.MaxExpensive = 1
	require.NoError(t, config.Validate())
	dispatcher.admission = newRPCAdmission(config)

	args := hexInputTransactionArgs()
	args["to"] = "0x" + strings.Repeat("11", types.AddressLength) + "zz"
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

/* FINISH ALPHA CHANGE 3176 -- reject malformed typed RPC identifiers before endpoint and backend work -- */
