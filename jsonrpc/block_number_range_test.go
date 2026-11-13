/* START ALPHA CHANGE 3194 -- verify block-selector ranges and rejection before RPC work -- */
package jsonrpc

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/gasprice"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func blockNumberRangeRejectedInputs() []string {
	return []string{
		`"0x8000000000000000"`,
		`"9223372036854775808"`,
		`9223372036854775808`,
		`"0xfffffffffffffffd"`,
		`"18446744073709551613"`,
		`18446744073709551613`,
		`"0xfffffffffffffffe"`,
		`"18446744073709551614"`,
		`18446744073709551614`,
		`"0xffffffffffffffff"`,
		`"18446744073709551615"`,
		`18446744073709551615`,
		`"0x10000000000000000"`,
		`"18446744073709551616"`,
		`18446744073709551616`,
	}
}

func TestBlockNumberRange_AcceptedDecoding(t *testing.T) {
	tests := []struct {
		raw  string
		want BlockNumber
	}{
		{`"earliest"`, EarliestBlockNumber},
		{`"latest"`, LatestBlockNumber},
		{`"pending"`, PendingBlockNumber},
		{`"0x0"`, 0},
		{`0`, 0},
		{`"0x2a"`, 42},
		{`"42"`, 42},
		{`42`, 42},
		{`"00042"`, 42},
		{`"0x20000000000001"`, 9007199254740993},
		{`9007199254740993`, 9007199254740993},
		{`"0x7fffffffffffffff"`, BlockNumber(math.MaxInt64)},
		{`"9223372036854775807"`, BlockNumber(math.MaxInt64)},
		{`9223372036854775807`, BlockNumber(math.MaxInt64)},
	}

	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			parsed, err := stringToBlockNumber(test.raw)
			require.NoError(t, err)
			require.Equal(t, test.want, parsed)

			var number BlockNumber
			require.NoError(t, json.Unmarshal([]byte(test.raw), &number))
			require.Equal(t, test.want, number)

			for _, raw := range []string{
				test.raw,
				`{"blockNumber":` + test.raw + `}`,
			} {
				var selector BlockNumberOrHash
				require.NoError(t, json.Unmarshal([]byte(raw), &selector))
				require.NotNil(t, selector.BlockNumber)
				require.Equal(t, test.want, *selector.BlockNumber)
				require.Nil(t, selector.BlockHash)
			}
		})
	}
}

func TestBlockNumberRange_RejectionPreservesReceiver(t *testing.T) {
	for _, raw := range blockNumberRangeRejectedInputs() {
		t.Run(raw, func(t *testing.T) {
			parsed, err := stringToBlockNumber(raw)
			require.Error(t, err)
			require.Zero(t, parsed)

			number := BlockNumber(7)
			require.Error(t, json.Unmarshal([]byte(raw), &number))
			require.Equal(t, BlockNumber(7), number)

			for _, encoded := range []string{
				raw,
				`{"blockNumber":` + raw + `}`,
			} {
				original := BlockNumber(7)
				selector := BlockNumberOrHash{BlockNumber: &original}

				require.Error(t, json.Unmarshal([]byte(encoded), &selector))
				require.Same(t, &original, selector.BlockNumber)
				require.Equal(t, BlockNumber(7), original)
				require.Nil(t, selector.BlockHash)
			}
		})
	}
}

func TestBlockNumberRange_LogFilterAcceptedBounds(t *testing.T) {
	tests := []struct {
		input string
		want  BlockNumber
	}{
		{"", LatestBlockNumber},
		{"pending", LatestBlockNumber},
		{"latest", LatestBlockNumber},
		{"earliest", EarliestBlockNumber},
		{"0x0", 0},
		{"0x2a", 42},
		{"42", 42},
		{"0x7fffffffffffffff", BlockNumber(math.MaxInt64)},
		{"9223372036854775807", BlockNumber(math.MaxInt64)},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			raw, err := json.Marshal(map[string]string{
				"fromBlock": test.input,
				"toBlock":   test.input,
			})
			require.NoError(t, err)

			var query LogQuery
			require.NoError(t, json.Unmarshal(raw, &query))
			require.Equal(t, test.want, query.fromBlock)
			require.Equal(t, test.want, query.toBlock)
		})
	}
}

// Reuse the existing hexadecimal-validation counters and mock backend.
// Record selected heights and account roots to verify valid routing too.
type blockNumberRangeStore struct {
	*typedHexStore
	heights      []uint64
	accountRoots []types.Hash
}

func (s *blockNumberRangeStore) Header() *types.Header {
	header, _ := s.GetHeaderByNumber(s.header.Number)
	return header
}

func (s *blockNumberRangeStore) GetHeaderByNumber(
	number uint64,
) (*types.Header, bool) {
	s.counts.backend.Add(1)
	s.heights = append(s.heights, number)

	if number > s.header.Number {
		return nil, false
	}

	header := s.header.Copy()
	header.Number = number
	header.Hash = types.BytesToHash([]byte{byte(number + 1)})
	header.StateRoot = header.Hash
	return header, true
}

func (s *blockNumberRangeStore) GetAccount(
	root types.Hash,
	address types.Address,
) (*Account, error) {
	s.accountRoots = append(s.accountRoots, root)
	return s.hexInputStore.GetAccount(root, address)
}

func (s *blockNumberRangeStore) GetBlockByNumber(
	number uint64,
	_ bool,
) (*types.Block, bool) {
	header, found := s.GetHeaderByNumber(number)
	if !found {
		return nil, false
	}
	return &types.Block{Header: header}, true
}

func (s *blockNumberRangeStore) FeeHistory(
	_ uint64,
	newest uint64,
	_ []float64,
) (*gasprice.FeeHistoryReturn, error) {
	s.counts.backend.Add(1)
	s.heights = append(s.heights, newest)
	return &gasprice.FeeHistoryReturn{OldestBlock: newest}, nil
}

type blockNumberRangeEthProbe struct {
	*typedHexEthProbe
}

func (p *blockNumberRangeEthProbe) GetBlockByNumber(
	number BlockNumber,
	full bool,
) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.GetBlockByNumber(number, full)
}

func (p *blockNumberRangeEthProbe) FeeHistory(
	count argUint64,
	newest BlockNumber,
	percentiles []float64,
) (any, error) {
	p.counts.endpoints.Add(1)
	return p.Eth.FeeHistory(count, newest, percentiles)
}

func blockNumberRangeDispatcher(
	t *testing.T,
) (*Dispatcher, *blockNumberRangeStore) {
	t.Helper()

	counts := new(hexInputCounters)
	store := &blockNumberRangeStore{
		typedHexStore: &typedHexStore{
			hexInputStore: &hexInputStore{
				counts: counts,
				header: &types.Header{
					Number:   1,
					GasLimit: 1_000_000,
					BaseFee:  1,
				},
			},
		},
	}

	logger := hclog.NewNullLogger()
	dispatcher := &Dispatcher{
		logger: logger,
		params: &dispatcherParams{},
	}
	t.Cleanup(dispatcher.Close)

	eth := &Eth{
		logger:        logger,
		store:         store,
		chainID:       1982,
		filterManager: &FilterManager{store: store},
	}
	require.NoError(t, dispatcher.registerService(
		"eth",
		&blockNumberRangeEthProbe{
			typedHexEthProbe: &typedHexEthProbe{
				hexInputEthProbe: &hexInputEthProbe{
					Eth:    eth,
					counts: counts,
				},
			},
		},
	))
	require.NoError(t, dispatcher.registerService(
		"debug",
		&hexInputDebugProbe{
			Debug:  NewDebug(store, 4),
			counts: counts,
		},
	))

	return dispatcher, store
}

func TestBlockNumberRange_DispatchRejectsBeforeWork(t *testing.T) {
	address := "0x6eE8A7Dc1625B46e6474a69b504825858315620c"

	for _, raw := range blockNumberRangeRejectedInputs() {
		t.Run(raw, func(t *testing.T) {
			selector := json.RawMessage(raw)
			object := map[string]any{"blockNumber": selector}

			cases := []struct {
				name   string
				method string
				params []any
			}{
				{"balance", "eth_getBalance", []any{address, selector}},
				{"balance_object", "eth_getBalance", []any{address, object}},
				{"block", "eth_getBlockByNumber", []any{selector, false}},
				{"call", "eth_call",
					[]any{hexInputTransactionArgs(), selector}},
				{"call_object", "eth_call",
					[]any{hexInputTransactionArgs(), object}},
				{"estimate", "eth_estimateGas",
					[]any{hexInputTransactionArgs(), selector}},
				{"trace", "debug_traceCall",
					[]any{hexInputTransactionArgs(), selector, map[string]any{}}},
				{"trace_object", "debug_traceCall",
					[]any{hexInputTransactionArgs(), object, map[string]any{}}},
				{"fee_history", "eth_feeHistory",
					[]any{"0x1", selector, []float64{}}},
			}

			// Log-filter bounds currently accept JSON strings.
			var text string
			if err := json.Unmarshal([]byte(raw), &text); err == nil {
				for _, bound := range []string{"fromBlock", "toBlock"} {
					filter := map[string]any{
						"fromBlock": "0x1",
						"toBlock":   "0x1",
					}
					filter[bound] = text
					cases = append(cases, struct {
						name   string
						method string
						params []any
					}{
						"logs_" + bound, "eth_getLogs", []any{filter},
					})
				}
			}

			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					dispatcher, store := blockNumberRangeDispatcher(t)
					response := hexInputRequest(
						t, dispatcher, test.method, test.params,
					)

					hexInputRequireError(t, response, -32602)
					hexInputRequireNoWork(t, store.counts)
					require.Empty(t, store.heights)
					require.Empty(t, store.accountRoots)
				})
			}
		})
	}
}

func TestBlockNumberRange_ValidBalanceRouting(t *testing.T) {
	tests := []struct {
		raw    string
		height uint64
	}{
		{`"earliest"`, 0},
		{`"latest"`, 1},
		{`"pending"`, 1},
		{`"0x0"`, 0},
		{`"0x1"`, 1},
		{`"1"`, 1},
		{`1`, 1},
	}

	for _, test := range tests {
		for _, object := range []bool{false, true} {
			name := test.raw + "/direct"
			var selector any = json.RawMessage(test.raw)
			if object {
				name = test.raw + "/object"
				selector = map[string]any{"blockNumber": selector}
			}

			t.Run(name, func(t *testing.T) {
				dispatcher, store := blockNumberRangeDispatcher(t)
				response := hexInputRequest(
					t, dispatcher, "eth_getBalance",
					[]any{
						"0x6eE8A7Dc1625B46e6474a69b504825858315620c",
						selector,
					},
				)

				require.Nil(t, response.Error)
				require.Equal(t, int32(1), store.counts.endpoints.Load())
				require.Equal(t, []uint64{test.height}, store.heights)
				require.Equal(t, []types.Hash{
					types.BytesToHash([]byte{byte(test.height + 1)}),
				}, store.accountRoots)
				require.Zero(t, store.counts.apply.Load())
				require.Zero(t, store.counts.trace.Load())
			})
		}
	}
}

func TestBlockNumberRange_ValidEndpointControls(t *testing.T) {
	tests := []struct {
		method string
		params []any
	}{
		{"eth_getBlockByNumber", []any{"latest", false}},
		{"eth_call", []any{hexInputTransactionArgs(), "latest"}},
		{"eth_estimateGas", []any{hexInputTransactionArgs(), "latest"}},
		{"debug_traceCall",
			[]any{hexInputTransactionArgs(), "latest", map[string]any{}}},
		{"eth_feeHistory", []any{"0x1", "latest", []float64{}}},
		{"eth_getLogs", []any{map[string]any{
			"fromBlock": "0x1",
			"toBlock":   "latest",
		}}},
	}

	for _, test := range tests {
		t.Run(test.method, func(t *testing.T) {
			dispatcher, store := blockNumberRangeDispatcher(t)
			response := hexInputRequest(
				t, dispatcher, test.method, test.params,
			)

			require.Nil(t, response.Error)
			require.Equal(t, int32(1), store.counts.endpoints.Load())
			require.Positive(t, store.counts.backend.Load())
			require.NotEmpty(t, store.heights)
			for _, height := range store.heights {
				require.Equal(t, uint64(1), height)
			}

			switch test.method {
			case "eth_call", "eth_estimateGas":
				require.Positive(t, store.counts.apply.Load())
			case "debug_traceCall":
				require.Equal(t, int32(1), store.counts.trace.Load())
			}
		})
	}
}

func TestBlockNumberRange_MaximumParsesButNeedNotExist(t *testing.T) {
	dispatcher, store := blockNumberRangeDispatcher(t)
	response := hexInputRequest(
		t, dispatcher, "eth_getBalance",
		[]any{
			"0x6eE8A7Dc1625B46e6474a69b504825858315620c",
			json.RawMessage(`9223372036854775807`),
		},
	)

	// Parameter decoding succeeds. This fixture contains only blocks 0 and 1,
	// so the actual endpoint must report its ordinary missing-header error.
	require.NotNil(t, response.Error)
	require.NotEqual(t, -32602, response.Error.Code)
	require.Equal(t, int32(1), store.counts.endpoints.Load())
	require.Equal(t, []uint64{uint64(math.MaxInt64)}, store.heights)
	require.Empty(t, store.accountRoots)
	require.Zero(t, store.counts.apply.Load())
	require.Zero(t, store.counts.trace.Load())
}

/* FINISH ALPHA CHANGE 3194 -- verify block-selector ranges and rejection before RPC work -- */
