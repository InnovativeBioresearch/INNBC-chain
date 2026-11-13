/* START ALPHA CHANGE 3029 -- reject altered signed creation encodings before RPC admission -- */
package jsonrpc

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func canonicalRPCReplaceField(
	t *testing.T,
	txType types.TxType,
	raw []byte,
	index int,
	replacement []byte,
) []byte {
	t.Helper()

	payload := raw
	if txType != types.LegacyTx {
		payload = raw[1:]
	}

	parser := new(fastrlp.Parser)
	value, err := parser.Parse(payload)
	require.NoError(t, err)
	fields, err := value.GetElems()
	require.NoError(t, err)

	arena := new(fastrlp.Arena)
	list := arena.NewArray()
	for i, field := range fields {
		if i == index {
			list.Set(arena.NewCopyBytes(replacement))
		} else {
			list.Set(field)
		}
	}

	result := list.MarshalTo(nil)
	if txType != types.LegacyTx {
		result = append([]byte{byte(txType)}, result...)
	}

	return result
}

func TestTransactionCanonical_RPCSignedCreation(t *testing.T) {
	for _, txType := range []types.TxType{types.LegacyTx, types.DynamicFeeTx} {
		t.Run(txType.String(), func(t *testing.T) {
			endpoint, store, _ := dynamicFeeFieldsEndpoint(t)

			key, err := crypto.GenerateECDSAKey()
			require.NoError(t, err)
			signer := crypto.NewLondonSigner(
				100, true, crypto.NewEIP155Signer(100, true),
			)

			signed, err := signer.SignTx(&types.Transaction{
				Type:      txType,
				ChainID:   big.NewInt(100),
				GasPrice:  big.NewInt(1),
				GasTipCap: big.NewInt(1),
				GasFeeCap: big.NewInt(2),
				Gas:       100_000,
				Value:     big.NewInt(1),
				Input:     []byte{0x00, 0x00},
			}, key)
			require.NoError(t, err)
			require.Nil(t, signed.To)

			sender, err := signer.Sender(signed)
			require.NoError(t, err)
			require.Equal(t, crypto.PubKeyToAddress(&key.PublicKey), sender)

			raw := signed.MarshalRLP()
			toIndex, nonceIndex, priceIndex := 3, 0, 1
			if txType == types.DynamicFeeTx {
				toIndex, nonceIndex, priceIndex = 5, 1, 2
			}

			tests := []struct {
				name string
				raw  []byte
				want error
			}{
				{
					"one_byte_recipient",
					canonicalRPCReplaceField(t, txType, raw, toIndex, []byte{1}),
					types.ErrNonCanonicalTxEncoding,
				},
				{
					"short_recipient",
					canonicalRPCReplaceField(t, txType, raw, toIndex, make([]byte, 19)),
					types.ErrNonCanonicalTxEncoding,
				},
				{
					"long_recipient",
					canonicalRPCReplaceField(t, txType, raw, toIndex, make([]byte, 21)),
					types.ErrNonCanonicalTxEncoding,
				},
				{
					"nonce_zero_byte",
					canonicalRPCReplaceField(t, txType, raw, nonceIndex, []byte{0}),
					types.ErrNonCanonicalTxEncoding,
				},
				{
					"price_leading_zero",
					canonicalRPCReplaceField(t, txType, raw, priceIndex, []byte{0, 1}),
					types.ErrNonCanonicalTxEncoding,
				},
				{
					"trailing_data",
					append(append([]byte(nil), raw...), 0x80),
					types.ErrInvalidRLPStructure,
				},
			}
			if txType == types.LegacyTx {
				tests = append(tests, struct {
					name string
					raw  []byte
					want error
				}{
					"redundant_legacy_prefix",
					append([]byte{0}, raw...),
					types.ErrNonCanonicalTxEncoding,
				})
			}

			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					before := append([]byte(nil), tc.raw...)
					result, err := endpoint.SendRawTransaction(argBytes(tc.raw))
					require.ErrorIs(t, err, tc.want)
					require.Nil(t, result)
					require.Zero(t, store.calls)
					require.Nil(t, store.accepted)
					require.True(t, bytes.Equal(before, tc.raw))
				})
			}

			// The original signed creation still enters the real pool,
			// preserving its original canonical bytes and typed-fork hash.
			result, err := endpoint.SendRawTransaction(argBytes(raw))
			require.NoError(t, err)
			require.Equal(t, crypto.Keccak256Hash(raw).String(), result)
			require.Equal(t, 1, store.calls)
			require.NotNil(t, store.accepted)
			require.Equal(t, raw, store.accepted.MarshalRLP())
			require.Nil(t, store.accepted.To)
		})
	}
}

/* FINISH ALPHA CHANGE 3029 -- reject altered signed creation encodings before RPC admission -- */
