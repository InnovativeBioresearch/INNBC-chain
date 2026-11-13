/* START ALPHA CHANGE 2384 -- test full-width rewards through fee-history RPC encoding -- */
package jsonrpc

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/gasprice"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type issue16RPCStore struct {
	ethStore
	block    *types.Block
	receipts []*types.Receipt
	helper   *gasprice.GasHelper
}

func (s *issue16RPCStore) Header() *types.Header {
	return s.block.Header
}

func (s *issue16RPCStore) GetBlockByNumber(
	number uint64,
	_ bool,
) (*types.Block, bool) {
	if number != s.block.Number() {
		return nil, false
	}

	return s.block, true
}

func (s *issue16RPCStore) GetReceiptsByHash(
	_ types.Hash,
) ([]*types.Receipt, error) {
	return s.receipts, nil
}

func (s *issue16RPCStore) CalculateBaseFee(
	parent *types.Header,
) (uint64, error) {
	return parent.BaseFee, nil
}

func (s *issue16RPCStore) FeeHistory(
	count uint64,
	newest uint64,
	percentiles []float64,
) (*gasprice.FeeHistoryReturn, error) {
	return s.helper.FeeHistory(count, newest, percentiles)
}

func TestIssue16_FeeHistoryRPCPreservesLargeRewards(t *testing.T) {
	largeTip := new(big.Int).Lsh(big.NewInt(1), 64)

	store := &issue16RPCStore{
		block: &types.Block{
			Header: &types.Header{
				Number:   1,
				BaseFee:  100,
				GasUsed:  1000,
				GasLimit: 2000,
			},
			Transactions: []*types.Transaction{
				{Type: types.StateTx, GasPrice: new(big.Int)},
				{Type: types.LegacyTx, GasPrice: big.NewInt(100)},
				{
					Type: types.LegacyTx,
					GasPrice: new(big.Int).Add(
						new(big.Int).Set(largeTip), big.NewInt(100),
					),
				},
			},
		},
		receipts: []*types.Receipt{
			{GasUsed: 900, CumulativeGasUsed: 900},
			{GasUsed: 10, CumulativeGasUsed: 910},
			{GasUsed: 90, CumulativeGasUsed: 1000},
		},
	}

	helper, err := gasprice.NewGasHelper(gasprice.DefaultGasHelperConfig, store)
	require.NoError(t, err)
	store.helper = helper

	endpoint := &Eth{store: store}

	for range 2 {
		result, err := endpoint.FeeHistory(
			argUint64(1),
			BlockNumber(1),
			[]float64{0, 50, 100},
		)
		require.NoError(t, err)

		encoded, err := json.Marshal(result)
		require.NoError(t, err)

		var decoded struct {
			OldestBlock   string     `json:"oldestBlock"`
			BaseFeePerGas []string   `json:"baseFeePerGas"`
			GasUsedRatio  []float64  `json:"gasUsedRatio"`
			Reward        [][]string `json:"reward"`
		}
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.Equal(t, "0x1", decoded.OldestBlock)
		require.Equal(t, []string{"0x64", "0x64"}, decoded.BaseFeePerGas)
		require.Equal(t, []float64{0.5}, decoded.GasUsedRatio)
		require.Equal(t, [][]string{{
			"0x0",
			"0x10000000000000000",
			"0x10000000000000000",
		}}, decoded.Reward)
	}
}

func TestIssue16_FeeHistoryRPCConversionOwnsValues(t *testing.T) {
	source := [][]*big.Int{{
		new(big.Int).Lsh(big.NewInt(1), 64),
		new(big.Int),
	}}

	converted := convertToArgBigSliceSlice(source)
	source[0][0].SetInt64(7)

	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, `[["0x10000000000000000","0x0"]]`, string(encoded))
}

/* FINISH ALPHA CHANGE 2384 -- test full-width rewards through fee-history RPC encoding -- */
