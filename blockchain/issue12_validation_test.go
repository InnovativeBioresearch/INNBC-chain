/* START ALPHA CHANGE 2296 -- test block bloom verification using execution receipts -- */
package blockchain

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/innovativebioresearch/innbc-chain/types/buildroot"
	"github.com/stretchr/testify/require"
)

func TestIssue12_BlockResultChecksLogsBloom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		receipts []*types.Receipt
	}{
		{
			name: "empty block",
		},
		{
			name: "receipts without logs",
			receipts: []*types.Receipt{
				{CumulativeGasUsed: 1},
			},
		},
		{
			name: "multiple receipts including an ordinary failed transaction",
			receipts: []*types.Receipt{
				{
					CumulativeGasUsed: 1,
					Logs: []*types.Log{
						{
							Address: types.StringToAddress("0x1234"),
							Topics: []types.Hash{
								types.BytesToHash([]byte{1}),
								types.BytesToHash([]byte{2}),
							},
						},
					},
				},
				{
					CumulativeGasUsed: 2,
					Logs: []*types.Log{
						{
							Address: types.StringToAddress("0x5678"),
							Topics: []types.Hash{
								types.BytesToHash([]byte{3}),
							},
						},
					},
				},
				{
					CumulativeGasUsed: 3,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transactions := make([]*types.Transaction, len(test.receipts))
			for index, receipt := range test.receipts {
				receipt.SetStatus(types.ReceiptSuccess)
				receipt.LogsBloom = types.CreateBloom([]*types.Receipt{receipt})
				transactions[index] = &types.Transaction{}
			}

			if len(test.receipts) == 3 {
				test.receipts[2].SetStatus(types.ReceiptFailed)
			}

			root := types.BytesToHash([]byte{9})
			gas := uint64(len(test.receipts))
			block := &types.Block{
				Header: &types.Header{
					StateRoot:    root,
					GasUsed:      gas,
					ReceiptsRoot: buildroot.CalculateReceiptsRoot(test.receipts),
					LogsBloom:    types.CreateBloom(test.receipts),
				},
				Transactions: transactions,
			}
			result := &BlockResult{
				Root:     root,
				TotalGas: gas,
				Receipts: test.receipts,
			}

			// The independent log-based bloom must match the combination of
			// locally computed per-receipt blooms.
			require.NoError(t, result.verifyBlockResult(block))

			// Keep all other execution commitments unchanged.
			block.Header.LogsBloom[0] ^= 1
			require.ErrorIs(t, result.verifyBlockResult(block), ErrInvalidLogsBloom)
		})
	}
}

/* FINISH ALPHA CHANGE 2296 -- test block bloom verification using execution receipts -- */
