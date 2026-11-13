/* START ALPHA CHANGE 3028 -- cover canonical transaction fields across runtime and storage decoding -- */
package types

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

// replacement is a complete encoded RLP field.
func canonicalTxReplaceField(
	t *testing.T,
	txType TxType,
	raw []byte,
	index int,
	replacement []byte,
) []byte {
	t.Helper()

	payload := raw
	if txType != LegacyTx {
		payload = raw[1:]
	}

	parser := new(fastrlp.Parser)
	value, err := parser.Parse(payload)
	require.NoError(t, err)
	fields, err := value.GetElems()
	require.NoError(t, err)
	require.Less(t, index, len(fields))

	var body []byte
	for i, field := range fields {
		if i == index {
			body = append(body, replacement...)
		} else {
			body = append(body, parser.Raw(field)...)
		}
	}

	result := runtimeRLPTestList(body)
	if txType != LegacyTx {
		result = append([]byte{byte(txType)}, result...)
	}

	return result
}

func canonicalTxBlock(raw []byte) []byte {
	header := (&Header{Number: 1}).MarshalRLP()
	body := append([]byte(nil), header...)
	body = append(body, runtimeRLPTestList(raw)...)
	body = append(body, 0xc0)

	return runtimeRLPTestList(body)
}

func canonicalTxAssertRejected(
	t *testing.T,
	txType TxType,
	raw []byte,
	want error,
) {
	t.Helper()

	candidate := runtimeRLPTestTransaction(txType)
	before := candidate.Copy()
	require.ErrorIs(t, candidate.UnmarshalRLP(raw), want)
	require.Equal(t, before, candidate)

	sentinel := &Header{Number: 99}
	block := &Block{Header: sentinel}
	require.ErrorIs(t, block.UnmarshalRLP(canonicalTxBlock(raw)), want)
	require.Same(t, sentinel, block.Header)
	require.Empty(t, block.Transactions)

	// Exercise the shared semantic decoder directly, bypassing the public
	// transaction preflight while retaining a real parser/value pair.
	payload := raw
	if txType != LegacyTx {
		payload = raw[1:]
	}
	parser := new(fastrlp.Parser)
	value, err := parser.Parse(payload)
	require.NoError(t, err)

	candidate = runtimeRLPTestTransaction(txType)
	before = candidate.Copy()
	require.ErrorIs(t, candidate.unmarshalRLPFrom(parser, value), want)
	require.Equal(t, before, candidate)

	// Preserve the real stored transaction envelope, replacing only its
	// consensus transaction list with the malformed list.
	arena := new(fastrlp.Arena)
	stored := arena.NewArray()
	if txType != LegacyTx {
		stored.Set(arena.NewCopyBytes([]byte{byte(txType)}))
	}
	stored.Set(value)
	stored.Set(arena.NewCopyBytes(before.From.Bytes()))

	var decodedStored Transaction
	require.ErrorIs(t, decodedStored.UnmarshalStoreRLP(stored.MarshalTo(nil)), want)
}

func TestTransactionCanonical_RecipientAndStateSender(t *testing.T) {
	for _, txType := range []TxType{LegacyTx, DynamicFeeTx, StateTx} {
		toIndex := 3
		if txType == DynamicFeeTx {
			toIndex = 5
		}

		for _, size := range []int{1, 19, 21} {
			t.Run(fmt.Sprintf("%s/recipient_%d", txType, size), func(t *testing.T) {
				raw := runtimeRLPTestTransaction(txType).MarshalRLP()
				field := append(
					[]byte{0x80 + byte(size)},
					bytes.Repeat([]byte{0}, size)...,
				)
				if size == 1 {
					// Canonical RLP for the one-byte string 0x01.
					field = []byte{0x01}
				}

				raw = canonicalTxReplaceField(t, txType, raw, toIndex, field)
				canonicalTxAssertRejected(t, txType, raw, ErrNonCanonicalTxEncoding)
			})
		}

		t.Run(txType.String()+"/recipient_list", func(t *testing.T) {
			raw := runtimeRLPTestTransaction(txType).MarshalRLP()
			raw = canonicalTxReplaceField(t, txType, raw, toIndex, []byte{0xc0})
			canonicalTxAssertRejected(t, txType, raw, ErrInvalidRLPStructure)
		})
	}

	for _, size := range []int{0, 1, 19, 21} {
		t.Run(fmt.Sprintf("state_sender_%d", size), func(t *testing.T) {
			raw := runtimeRLPTestTransaction(StateTx).MarshalRLP()
			field := append(
				[]byte{0x80 + byte(size)},
				bytes.Repeat([]byte{0}, size)...,
			)
			if size == 1 {
				field = []byte{0x01}
			}

			raw = canonicalTxReplaceField(t, StateTx, raw, 9, field)
			canonicalTxAssertRejected(t, StateTx, raw, ErrNonCanonicalTxEncoding)
		})
	}
}

func TestTransactionCanonical_IntegerFields(t *testing.T) {
	for _, txType := range []TxType{LegacyTx, DynamicFeeTx, StateTx} {
		indices := []int{0, 1, 2, 4, 6, 7, 8}
		if txType == DynamicFeeTx {
			indices = []int{0, 1, 2, 3, 4, 6, 9, 10, 11}
		}

		for _, index := range indices {
			for _, encoding := range []struct {
				name string
				raw  []byte
			}{
				{"zero_byte", []byte{0x00}},
				{"leading_zero", []byte{0x82, 0x00, 0x01}},
			} {
				name := fmt.Sprintf("%s/field_%d/%s", txType, index, encoding.name)
				t.Run(name, func(t *testing.T) {
					raw := runtimeRLPTestTransaction(txType).MarshalRLP()
					raw = canonicalTxReplaceField(t, txType, raw, index, encoding.raw)
					canonicalTxAssertRejected(t, txType, raw, ErrNonCanonicalTxEncoding)
				})
			}
		}
	}
}

func TestTransactionCanonical_LegacyEnvelope(t *testing.T) {
	valid := runtimeRLPTestTransaction(LegacyTx).MarshalRLP()
	redundant := append([]byte{byte(LegacyTx)}, valid...)

	var transaction Transaction
	require.ErrorIs(t, transaction.UnmarshalRLP(redundant), ErrNonCanonicalTxEncoding)

	var block Block
	require.ErrorIs(t, block.UnmarshalRLP(canonicalTxBlock(redundant)), ErrNonCanonicalTxEncoding)

	require.NoError(t, transaction.UnmarshalRLP(valid))
	require.Equal(t, valid, transaction.MarshalRLP())
}

func TestTransactionCanonical_ValidRoundTrips(t *testing.T) {
	// These are encoding fixtures; signature validity is checked separately
	// by the signed RPC admission regressions.
	for _, txType := range []TxType{LegacyTx, DynamicFeeTx, StateTx} {
		for _, destination := range []string{"creation", "zero_address", "address"} {
			for _, zeroIntegers := range []bool{false, true} {
				name := fmt.Sprintf("%s/%s/zero_%t", txType, destination, zeroIntegers)
				t.Run(name, func(t *testing.T) {
					original := runtimeRLPTestTransaction(txType)
					original.Input = []byte{0x00, 0x00, 0xc0}
					original.From = ZeroAddress

					switch destination {
					case "creation":
						original.To = nil
					case "zero_address":
						zero := ZeroAddress
						original.To = &zero
					}

					if zeroIntegers {
						original.Nonce = 0
						original.Gas = 0
						original.ChainID = new(big.Int)
						original.GasPrice = new(big.Int)
						original.GasTipCap = new(big.Int)
						original.GasFeeCap = new(big.Int)
						original.Value = new(big.Int)
						original.V = new(big.Int)
						original.R = new(big.Int)
						original.S = new(big.Int)
					}

					raw := original.MarshalRLP()
					var decoded Transaction
					require.NoError(t, decoded.UnmarshalRLP(raw))
					require.Equal(t, raw, decoded.MarshalRLP())
					require.Equal(t, original.To, decoded.To)
					require.Equal(t, original.Input, decoded.Input)

					hasher := new(TransactionHashForkV2)
					hasher.ComputeHash(original)
					hasher.ComputeHash(&decoded)
					require.Equal(t, original.Hash, decoded.Hash)

					stored := original.MarshalStoreRLPTo(nil)
					var decodedStored Transaction
					require.NoError(t, decodedStored.UnmarshalStoreRLP(stored))
					require.Equal(t, stored, decodedStored.MarshalStoreRLPTo(nil))
					require.Equal(t, raw, decodedStored.MarshalRLP())
					require.Equal(t, original.From, decodedStored.From)
				})
			}
		}
	}
}

/* FINISH ALPHA CHANGE 3028 -- cover canonical transaction fields across runtime and storage decoding -- */
