package types

/* START ALPHA CHANGE 2434 -- cover missing typed payloads and unchanged valid encodings -- */

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func missingPayloadTestTransaction(txType TxType) *Transaction {
	to := StringToAddress("0x1234")

	return &Transaction{
		Type:      txType,
		ChainID:   big.NewInt(1),
		Nonce:     3,
		GasPrice:  big.NewInt(7),
		GasFeeCap: big.NewInt(9),
		GasTipCap: big.NewInt(2),
		Gas:       50000,
		To:        &to,
		From:      StringToAddress("0x5678"),
		Value:     big.NewInt(4),
		Input:     []byte{1, 2, 3},
		V:         big.NewInt(1),
		R:         big.NewInt(2),
		S:         big.NewInt(3),
	}
}

func missingPayloadTestReceipt(txType TxType) *Receipt {
	receipt := &Receipt{
		TransactionType:   txType,
		CumulativeGasUsed: 21000,
		Logs: []*Log{
			{
				Address: StringToAddress("0x1234"),
				Topics:  []Hash{StringToHash("0x01")},
				Data:    []byte{4, 5, 6},
			},
		},
	}
	receipt.SetStatus(ReceiptSuccess)

	return receipt
}

func TestRLPTypePayload_MissingPayload(t *testing.T) {
	for _, txType := range []TxType{LegacyTx, DynamicFeeTx, StateTx} {
		for _, prefix := range []bool{false, true} {
			for _, kind := range []string{"block", "receipts"} {
				name := fmt.Sprintf("%s/type_%02x/prefix_%t", kind, byte(txType), prefix)

				t.Run(name, func(t *testing.T) {
					arena := new(fastrlp.Arena)
					list := arena.NewArray()

					if prefix {
						list.Set(arena.NewCopyBytes([]byte{byte(DynamicFeeTx)}))

						if kind == "block" {
							list.Set(missingPayloadTestTransaction(DynamicFeeTx).MarshalRLPWith(arena))
						} else {
							list.Set(missingPayloadTestReceipt(DynamicFeeTx).MarshalRLPWith(arena))
						}
					}

					list.Set(arena.NewCopyBytes([]byte{byte(txType)}))

					var decoder RLPUnmarshaler
					var encoded []byte

					if kind == "block" {
						block := arena.NewArray()
						header := &Header{Number: 1, GasLimit: 1000000}
						block.Set(header.MarshalRLPWith(arena))
						block.Set(list)
						block.Set(arena.NewNullArray())

						encoded = block.MarshalTo(nil)
						decoder = new(Block)
					} else {
						encoded = list.MarshalTo(nil)
						decoder = new(Receipts)
					}

					var err error
					require.NotPanics(t, func() {
						err = decoder.UnmarshalRLP(encoded)
					})
					require.ErrorContains(t, err, "missing payload after transaction type")
				})
			}
		}
	}
}

func TestRLPTypePayload_ValidRoundTrips(t *testing.T) {
	cases := []struct {
		name  string
		types []TxType
	}{
		{name: "empty"},
		{name: "legacy", types: []TxType{LegacyTx}},
		{name: "dynamic", types: []TxType{DynamicFeeTx}},
		{name: "state", types: []TxType{StateTx}},
		{
			name:  "mixed",
			types: []TxType{LegacyTx, DynamicFeeTx, StateTx, LegacyTx, DynamicFeeTx},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			block := &Block{
				Header: &Header{Number: 1, GasLimit: 1000000},
			}
			var receipts Receipts

			for _, txType := range testCase.types {
				block.Transactions = append(
					block.Transactions,
					missingPayloadTestTransaction(txType),
				)
				receipts = append(receipts, missingPayloadTestReceipt(txType))
			}

			blockBytes := block.MarshalRLP()
			var decodedBlock Block
			require.NoError(t, decodedBlock.UnmarshalRLP(blockBytes))
			require.Equal(t, blockBytes, decodedBlock.MarshalRLP())
			require.Len(t, decodedBlock.Transactions, len(testCase.types))

			receiptBytes := receipts.MarshalRLPTo(nil)
			var decodedReceipts Receipts
			require.NoError(t, decodedReceipts.UnmarshalRLP(receiptBytes))
			require.Equal(t, receiptBytes, decodedReceipts.MarshalRLPTo(nil))
			require.Len(t, decodedReceipts, len(testCase.types))

			for index, txType := range testCase.types {
				require.Equal(t, txType, decodedBlock.Transactions[index].Type)
				require.Equal(t, txType, decodedReceipts[index].TransactionType)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2434 -- cover missing typed payloads and unchanged valid encodings -- */
