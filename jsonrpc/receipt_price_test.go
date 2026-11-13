/* START ALPHA CHANGE 3198 -- verify full-width receipt prices and exact RPC quantities -- */
package jsonrpc

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type receiptPriceCase struct {
	name     string
	kind     types.TxType
	gasPrice string
	feeCap   string
	tipCap   string
	baseFee  uint64
	want     string
}

func receiptPriceCases() []receiptPriceCase {
	return []receiptPriceCase{
		{
			name:     "legacy_zero",
			kind:     types.LegacyTx,
			gasPrice: "0",
			want:     "0x0",
		},
		{
			name:     "legacy_small",
			kind:     types.LegacyTx,
			gasPrice: "8",
			baseFee:  7,
			want:     "0x8",
		},
		{
			name:     "legacy_max_uint64",
			kind:     types.LegacyTx,
			gasPrice: "ffffffffffffffff",
			baseFee:  7,
			want:     "0xffffffffffffffff",
		},
		{
			name:     "legacy_two_to_64",
			kind:     types.LegacyTx,
			gasPrice: "10000000000000000",
			baseFee:  7,
			want:     "0x10000000000000000",
		},
		{
			name:     "legacy_two_to_64_plus_seven",
			kind:     types.LegacyTx,
			gasPrice: "10000000000000007",
			baseFee:  7,
			want:     "0x10000000000000007",
		},
		{
			name:     "legacy_max_uint256_conversion",
			kind:     types.LegacyTx,
			gasPrice: strings.Repeat("f", 64),
			baseFee:  7,
			want:     "0x" + strings.Repeat("f", 64),
		},
		{
			name:    "dynamic_small",
			kind:    types.DynamicFeeTx,
			feeCap:  "f",
			tipCap:  "1",
			baseFee: 7,
			want:    "0x8",
		},
		{
			// min(2^64 + 100, 7 + 2^64) = 2^64 + 7.
			name:    "dynamic_base_fee_plus_tip",
			kind:    types.DynamicFeeTx,
			feeCap:  "10000000000000064",
			tipCap:  "10000000000000000",
			baseFee: 7,
			want:    "0x10000000000000007",
		},
		{
			// min(2^64 + 3, 7 + 2^64) = 2^64 + 3.
			name:    "dynamic_fee_cap",
			kind:    types.DynamicFeeTx,
			feeCap:  "10000000000000003",
			tipCap:  "10000000000000000",
			baseFee: 7,
			want:    "0x10000000000000003",
		},
	}
}

func receiptPriceFixture(
	t *testing.T,
	test receiptPriceCase,
) (*types.Transaction, *types.Receipt, *types.Header) {
	t.Helper()

	parse := func(text string) *big.Int {
		if text == "" {
			return nil
		}
		value, ok := new(big.Int).SetString(text, 16)
		require.True(t, ok)
		return value
	}

	to := types.BytesToAddress([]byte{2})
	tx := &types.Transaction{
		Type:      test.kind,
		Hash:      types.BytesToHash([]byte{0x11}),
		From:      types.BytesToAddress([]byte{1}),
		To:        &to,
		Gas:       21000,
		Value:     new(big.Int),
		GasPrice:  parse(test.gasPrice),
		GasFeeCap: parse(test.feeCap),
		GasTipCap: parse(test.tipCap),
	}

	status := types.ReceiptSuccess
	source := &types.Receipt{
		CumulativeGasUsed: 21000,
		GasUsed:           21000,
		Status:            &status,
		TxHash:            tx.Hash,
		TransactionType:   test.kind,
	}
	header := &types.Header{
		Number:  1,
		Hash:    types.BytesToHash([]byte{0x22}),
		BaseFee: test.baseFee,
	}

	return tx, source, header
}

func receiptPriceRequireJSON(
	t *testing.T,
	value any,
	want string,
) map[string]json.RawMessage {
	t.Helper()

	encoded, err := json.Marshal(value)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.Contains(t, fields, "effectiveGasPrice")
	require.Equal(t, `"`+want+`"`, string(fields["effectiveGasPrice"]))

	return fields
}

func TestReceiptPrice_FullWidthConversionAndJSON(t *testing.T) {
	for _, test := range receiptPriceCases() {
		for _, mode := range []string{"success", "failed", "legacy_root"} {
			if mode == "legacy_root" && test.kind != types.LegacyTx {
				continue
			}

			t.Run(test.name+"/"+mode, func(t *testing.T) {
				tx, source, header := receiptPriceFixture(t, test)

				switch mode {
				case "failed":
					status := types.ReceiptFailed
					source.Status = &status
				case "legacy_root":
					source.Status = nil
					source.Root = types.BytesToHash([]byte{0x33})
					header.BaseFee = 0
				}

				consensusBefore := source.MarshalRLPTo(nil)
				storageBefore := source.MarshalStoreRLPTo(nil)

				result := toReceipt(source, tx, 0, header, []*Log{})
				fields := receiptPriceRequireJSON(t, result, test.want)

				require.Equal(t, `"0x5208"`, string(fields["gasUsed"]))
				require.Equal(t, `"0x5208"`, string(fields["cumulativeGasUsed"]))

				if source.Status == nil {
					require.Contains(t, fields, "root")
					require.NotContains(t, fields, "status")
					require.NotNil(t, result.Root)
					require.Equal(t, source.Root, *result.Root)
				} else {
					require.NotContains(t, fields, "root")
					wantStatus := `"0x1"`
					if mode == "failed" {
						wantStatus = `"0x0"`
					}
					require.Equal(t, wantStatus, string(fields["status"]))
				}

				require.Equal(t, consensusBefore, source.MarshalRLPTo(nil))
				require.Equal(t, storageBefore, source.MarshalStoreRLPTo(nil))
			})
		}
	}
}

func TestReceiptPrice_IndependentOwnership(t *testing.T) {
	for _, test := range receiptPriceCases() {
		t.Run(test.name, func(t *testing.T) {
			tx, source, header := receiptPriceFixture(t, test)

			first := toReceipt(source, tx, 0, header, nil)
			second := toReceipt(source, tx, 0, header, nil)

			for _, value := range []*big.Int{
				tx.GasPrice,
				tx.GasFeeCap,
				tx.GasTipCap,
			} {
				if value != nil {
					value.SetInt64(99)
				}
			}
			header.BaseFee = 99

			receiptPriceRequireJSON(t, first, test.want)
			receiptPriceRequireJSON(t, second, test.want)

			// Mutating one converted receipt must not change the other
			// receipt or the transaction's independently held integers.
			(*big.Int)(&first.EffectiveGasPrice).SetInt64(23)
			receiptPriceRequireJSON(t, second, test.want)

			for _, value := range []*big.Int{
				tx.GasPrice,
				tx.GasFeeCap,
				tx.GasTipCap,
			} {
				if value != nil {
					require.Zero(t, big.NewInt(99).Cmp(value))
				}
			}
		})
	}
}

type receiptPriceStore struct {
	ethStore
	block    *types.Block
	receipts []*types.Receipt
	reads    [3]int
}

func (s *receiptPriceStore) ReadTxLookup(hash types.Hash) (types.Hash, bool) {
	s.reads[0]++
	if hash != s.block.Transactions[0].Hash {
		return types.ZeroHash, false
	}
	return s.block.Hash(), true
}

func (s *receiptPriceStore) GetBlockByHash(
	hash types.Hash,
	_ bool,
) (*types.Block, bool) {
	s.reads[1]++
	if hash != s.block.Hash() {
		return nil, false
	}
	return s.block, true
}

func (s *receiptPriceStore) GetReceiptsByHash(
	hash types.Hash,
) ([]*types.Receipt, error) {
	s.reads[2]++
	if hash != s.block.Hash() {
		return nil, nil
	}
	return s.receipts, nil
}

func TestReceiptPrice_ExactDispatcherJSON(t *testing.T) {
	for _, test := range receiptPriceCases() {
		t.Run(test.name, func(t *testing.T) {
			tx, source, header := receiptPriceFixture(t, test)
			consensusBefore := source.MarshalRLPTo(nil)
			storageBefore := source.MarshalStoreRLPTo(nil)

			store := &receiptPriceStore{
				block: &types.Block{
					Header:       header,
					Transactions: []*types.Transaction{tx},
				},
				receipts: []*types.Receipt{source},
			}

			logger := hclog.NewNullLogger()
			dispatcher := &Dispatcher{
				logger: logger,
				params: &dispatcherParams{},
			}
			t.Cleanup(dispatcher.Close)
			require.NoError(t, dispatcher.registerService(
				"eth",
				&Eth{logger: logger, store: store},
			))

			response := hexInputRequest(
				t, dispatcher, "eth_getTransactionReceipt",
				[]any{tx.Hash.String()},
			)
			require.Nil(t, response.Error)

			fields := receiptPriceRequireJSON(t, response.Result, test.want)
			require.Equal(t, `"0x0"`, string(fields["transactionIndex"]))
			require.Equal(t, `"0x1"`, string(fields["blockNumber"]))
			require.Equal(t, `"0x1"`, string(fields["status"]))
			require.NotContains(t, fields, "root")

			require.Equal(t, [3]int{1, 1, 1}, store.reads)
			require.Equal(t, consensusBefore, source.MarshalRLPTo(nil))
			require.Equal(t, storageBefore, source.MarshalStoreRLPTo(nil))
		})
	}
}

/* FINISH ALPHA CHANGE 3198 -- verify full-width receipt prices and exact RPC quantities -- */
