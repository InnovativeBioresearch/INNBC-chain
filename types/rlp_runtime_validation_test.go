/* START ALPHA CHANGE 3013 -- cover runtime preflight rejection and supported encoding compatibility -- */
package types

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func runtimeRLPTestTransaction(txType TxType) *Transaction {
	to := StringToAddress("0x1234")
	return &Transaction{
		Type:      txType,
		ChainID:   big.NewInt(100),
		Nonce:     3,
		GasPrice:  big.NewInt(7),
		GasTipCap: big.NewInt(2),
		GasFeeCap: big.NewInt(9),
		Gas:       1_000_000,
		To:        &to,
		From:      StringToAddress("0x5678"),
		Value:     big.NewInt(4),
		Input:     bytes.Repeat([]byte{0xff}, 32),
		V:         big.NewInt(1),
		R:         big.NewInt(2),
		S:         big.NewInt(3),
	}
}

// Construct canonical nested lists without a recursive fixture encoder.
func runtimeRLPTestList(payload []byte) []byte {
	if len(payload) <= 55 {
		return append([]byte{0xc0 + byte(len(payload))}, payload...)
	}

	var length [8]byte
	size := uint64(len(payload))
	start := len(length)
	for size != 0 {
		start--
		length[start] = byte(size)
		size >>= 8
	}

	result := []byte{0xf7 + byte(len(length)-start)}
	result = append(result, length[start:]...)
	return append(result, payload...)
}

func TestRuntimeRLP_TransactionRoundTrips(t *testing.T) {
	for _, txType := range []TxType{LegacyTx, DynamicFeeTx, StateTx} {
		t.Run(txType.String(), func(t *testing.T) {
			original := runtimeRLPTestTransaction(txType)
			raw := original.MarshalRLP()
			before := append([]byte(nil), raw...)

			var decoded Transaction
			require.NoError(t, decoded.UnmarshalRLP(raw))
			require.Equal(t, before, raw)
			require.Equal(t, raw, decoded.MarshalRLP())
			require.Equal(t, original.Type, decoded.Type)
			require.Equal(t, original.Nonce, decoded.Nonce)
			require.Equal(t, original.To, decoded.To)
			require.Equal(t, original.Value, decoded.Value)
			require.Equal(t, original.Input, decoded.Input)

			if txType == StateTx {
				require.Equal(t, original.From, decoded.From)
			}

			original.ComputeHash(1)
			decoded.ComputeHash(1)
			require.Equal(t, original.Hash, decoded.Hash)
		})
	}
}

func TestRuntimeRLP_TransactionRejection(t *testing.T) {
	valid := runtimeRLPTestTransaction(LegacyTx).MarshalRLP()

	deep := []byte{0xc0}
	for level := 0; level < 256; level++ {
		deep = runtimeRLPTestList(deep)
	}
	require.ErrorIs(t, ValidateRLPStructure(deep, 2), ErrRLPNestingTooDeep)

	tests := []struct {
		name  string
		input []byte
		want  error
	}{
		{"empty", nil, ErrInvalidRLPStructure},
		{"byte_root", []byte{0x80}, ErrInvalidRLPStructure},
		{"deep_canonical_lists", deep, ErrInvalidRLPStructure},
		{"missing_long_length", []byte{0xf8}, ErrInvalidRLPStructure},
		{"missing_payload", []byte{0xc1}, ErrInvalidRLPStructure},
		{"child_crosses_parent", []byte{0xc1, 0x81, 0x80}, ErrInvalidRLPStructure},
		{
			"maximum_declared_length",
			[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			ErrInvalidRLPStructure,
		},
		{"trailing_data", append(append([]byte(nil), valid...), 0x80), ErrInvalidRLPStructure},
		{"missing_dynamic_payload", []byte{byte(DynamicFeeTx)}, ErrInvalidRLPStructure},
		{"unknown_type", append([]byte{0x01}, valid...), nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]byte(nil), tc.input...)
			tx := runtimeRLPTestTransaction(StateTx)
			snapshot := tx.Copy()

			var err error
			require.NotPanics(t, func() {
				err = tx.UnmarshalRLP(tc.input)
			})
			require.Error(t, err)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			}

			require.Equal(t, snapshot, tx)
			require.True(t, bytes.Equal(before, tc.input))
		})
	}
}

func TestRuntimeRLP_ShallowTransactionShapeIsBounded(t *testing.T) {
	raw := runtimeRLPTestList(bytes.Repeat([]byte{0x80}, 100_000))

	var err error
	allocations := testing.AllocsPerRun(10, func() {
		err = validateRuntimeTransactionRLP(raw, LegacyTx)
	})

	require.ErrorIs(t, err, ErrInvalidRLPStructure)
	require.Zero(t, allocations)
}

func TestRuntimeRLP_UnsupportedNestedFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index int
		want  error
	}{
		{"nested_nonce", 1, ErrInvalidRLPStructure},
		{"nested_calldata", 7, ErrInvalidRLPStructure},
		{"nested_access_list", 8, ErrUnsupportedTxAccessList},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := runtimeRLPTestTransaction(DynamicFeeTx).MarshalRLP()
			parser := new(fastrlp.Parser)
			value, err := parser.Parse(raw[1:])
			require.NoError(t, err)
			fields, err := value.GetElems()
			require.NoError(t, err)

			arena := new(fastrlp.Arena)
			nested := arena.NewArray()
			inner := arena.NewArray()
			inner.Set(arena.NewNullArray())
			nested.Set(inner)

			payload := arena.NewArray()
			for index, field := range fields {
				if index == tc.index {
					payload.Set(nested)
				} else {
					payload.Set(field)
				}
			}

			modified := append([]byte{byte(DynamicFeeTx)}, payload.MarshalTo(nil)...)
			var decoded Transaction
			require.ErrorIs(t, decoded.UnmarshalRLP(modified), tc.want)
		})
	}
}

func TestRuntimeRLP_BlockRoundTripAndOpaqueBytes(t *testing.T) {
	block := &Block{
		Header: &Header{
			Number:    1,
			GasLimit:  5_242_880,
			ExtraData: bytes.Repeat([]byte{0xff}, 32),
		},
		Uncles: []*Header{{Number: 0}},
	}

	for _, txType := range []TxType{LegacyTx, DynamicFeeTx, StateTx} {
		block.Transactions = append(block.Transactions, runtimeRLPTestTransaction(txType))
	}

	block.Header.ComputeHash()
	raw := block.MarshalRLP()
	var decoded Block

	require.NoError(t, decoded.UnmarshalRLP(raw))
	require.Equal(t, raw, decoded.MarshalRLP())
	require.Equal(t, block.Hash(), decoded.Hash())
	require.Equal(t, block.Header.ExtraData, decoded.Header.ExtraData)
	require.Len(t, decoded.Transactions, 3)
	require.Len(t, decoded.Uncles, 1)

	for index, original := range block.Transactions {
		require.Equal(t, original.Type, decoded.Transactions[index].Type)
		require.Equal(t, original.Input, decoded.Transactions[index].Input)
	}
}

func TestRuntimeRLP_BlockRejectsUnsupportedShape(t *testing.T) {
	arena := new(fastrlp.Arena)
	header := (&Header{Number: 1}).MarshalRLPWith(arena)

	extraField := arena.NewArray()
	extraField.Set(header)
	extraField.Set(arena.NewNullArray())
	extraField.Set(arena.NewNullArray())
	extraField.Set(arena.NewNull())

	wrongTransactions := arena.NewArray()
	wrongTransactions.Set(header)
	wrongTransactions.Set(arena.NewNull())
	wrongTransactions.Set(arena.NewNullArray())

	valid := (&Block{Header: &Header{Number: 1}}).MarshalRLP()
	for _, raw := range [][]byte{
		{0xc5, 0xc4, 0xc3, 0xc2, 0xc1, 0xc0},
		{0xc3, 0xc0, 0xc0, 0xc0},
		extraField.MarshalTo(nil),
		wrongTransactions.MarshalTo(nil),
		append(append([]byte(nil), valid...), 0x80),
	} {
		sentinel := &Header{Number: 99}
		block := &Block{Header: sentinel}

		require.ErrorIs(t, block.UnmarshalRLP(raw), ErrInvalidRLPStructure)
		require.Same(t, sentinel, block.Header)
		require.Empty(t, block.Transactions)
		require.Empty(t, block.Uncles)
	}
}

func TestRuntimeRLP_HistoricalHeaderWithoutBaseFee(t *testing.T) {
	arena := new(fastrlp.Arena)
	headerValue := (&Header{Number: 1}).MarshalRLPWith(arena)
	fields, err := headerValue.GetElems()
	require.NoError(t, err)
	require.Len(t, fields, 16)

	historicalHeader := arena.NewArray()
	for _, field := range fields[:15] {
		historicalHeader.Set(field)
	}

	block := arena.NewArray()
	block.Set(historicalHeader)
	block.Set(arena.NewNullArray())
	block.Set(arena.NewNullArray())

	var decoded Block
	require.NoError(t, decoded.UnmarshalRLP(block.MarshalTo(nil)))
	require.Equal(t, uint64(1), decoded.Number())
	require.Zero(t, decoded.Header.BaseFee)
}

func TestRuntimeRLP_BlockTransactionsDoNotUsePoolSizePolicy(t *testing.T) {
	tx := runtimeRLPTestTransaction(LegacyTx)
	tx.Input = make([]byte, 128*1024+1)
	raw := tx.MarshalRLP()

	var decoded Transaction
	require.NoError(t, decoded.UnmarshalRLP(raw))
	require.Equal(t, raw, decoded.MarshalRLP())

	block := &Block{
		Header:       &Header{Number: 1},
		Transactions: []*Transaction{tx},
	}
	var decodedBlock Block
	require.NoError(t, decodedBlock.UnmarshalRLP(block.MarshalRLP()))
	require.Equal(t, tx.Input, decodedBlock.Transactions[0].Input)
}

func TestRuntimeRLP_GenericReceiptsRemainUnchanged(t *testing.T) {
	receipts := Receipts{missingPayloadTestReceipt(LegacyTx)}
	raw := receipts.MarshalRLPTo(nil)

	// Receipts with logs/topics can legitimately exceed the block depth.
	require.ErrorIs(t, ValidateRLPStructure(raw, BlockRLPMaxDepth), ErrRLPNestingTooDeep)

	var decoded Receipts
	require.NoError(t, decoded.UnmarshalRLP(raw))
	require.Equal(t, raw, decoded.MarshalRLPTo(nil))
}

/* FINISH ALPHA CHANGE 3013 -- cover runtime preflight rejection and supported encoding compatibility -- */
