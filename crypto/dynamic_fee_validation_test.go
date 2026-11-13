/* START ALPHA CHANGE 2935 -- cover dynamic-fee signing and recovery field ownership -- */
package crypto

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func dynamicFeeFieldsUnsignedTx() *types.Transaction {
	to := types.StringToAddress("0x9002")

	return &types.Transaction{
		Type:      types.DynamicFeeTx,
		To:        &to,
		Gas:       21000,
		Value:     big.NewInt(1),
		GasPrice:  new(big.Int),
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
	}
}

func TestDynamicFeeFields_LocalSigning(t *testing.T) {
	key, err := GenerateECDSAKey()
	require.NoError(t, err)

	for _, chainID := range []uint64{0, 100, ^uint64(0)} {
		expected := new(big.Int).SetUint64(chainID)

		t.Run(expected.String(), func(t *testing.T) {
			signer := NewLondonSigner(
				chainID, true, NewEIP155Signer(chainID, true),
			)

			for _, supplied := range []bool{false, true} {
				tx := dynamicFeeFieldsUnsignedTx()
				if supplied {
					tx.ChainID = new(big.Int).Set(expected)
				}

				before := append([]byte(nil), tx.MarshalRLP()...)
				signed, err := signer.SignTx(tx, key)
				require.NoError(t, err)
				require.Equal(t, 0, signed.ChainID.Cmp(expected))
				require.Equal(t, before, tx.MarshalRLP())
				require.Nil(t, tx.R)
				require.Nil(t, tx.S)
				require.Nil(t, tx.V)

				if supplied {
					require.NotSame(t, tx.ChainID, signed.ChainID)
					require.Equal(t, 0, tx.ChainID.Cmp(expected))
				} else {
					require.Nil(t, tx.ChainID)
				}

				sender, err := signer.Sender(signed)
				require.NoError(t, err)
				require.Equal(t, PubKeyToAddress(&key.PublicKey), sender)
			}
		})
	}
}

func TestDynamicFeeFields_RejectMismatchedIDs(t *testing.T) {
	key, err := GenerateECDSAKey()
	require.NoError(t, err)

	signer := NewLondonSigner(100, true, NewEIP155Signer(100, true))
	valid, err := signer.SignTx(dynamicFeeFieldsUnsignedTx(), key)
	require.NoError(t, err)

	high := new(big.Int).Lsh(big.NewInt(1), 64)
	high.Add(high, big.NewInt(100))

	tests := []struct {
		name string
		id   *big.Int
	}{
		{"missing", nil},
		{"explicit_zero", new(big.Int)},
		{"different", big.NewInt(101)},
		{"negative", big.NewInt(-100)},
		{"same_low_64_bits", high},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := valid.Copy()
			if tt.id == nil {
				tx.ChainID = nil
			} else {
				tx.ChainID = new(big.Int).Set(tt.id)
			}

			before := append([]byte(nil), tx.MarshalRLP()...)

			_, err := signer.Sender(tx)
			require.ErrorIs(t, err, types.ErrInvalidTxChainID)
			require.Equal(t, before, tx.MarshalRLP())

			// Nil is deliberately filled during local signing.
			// Explicit invalid values must instead be rejected.
			if tt.id != nil {
				signed, err := signer.SignTx(tx, key)
				require.ErrorIs(t, err, types.ErrInvalidTxChainID)
				require.Nil(t, signed)
				require.Equal(t, before, tx.MarshalRLP())
			}
		})
	}
}

func TestDynamicFeeFields_HashUsesActualID(t *testing.T) {
	signer100 := NewLondonSigner(100, true, NewEIP155Signer(100, true))
	signer101 := NewLondonSigner(101, true, NewEIP155Signer(101, true))

	tx := dynamicFeeFieldsUnsignedTx()
	tx.ChainID = big.NewInt(100)

	original := signer100.Hash(tx)

	// Hash follows transaction fields, not signer configuration.
	require.Equal(t, original, signer101.Hash(tx))

	tx.ChainID = big.NewInt(101)
	require.NotEqual(t, original, signer100.Hash(tx))

	/* START ALPHA CHANGE 2942 -- verify signing hashes retain the full encoded chain ID -- */
	high := new(big.Int).Lsh(big.NewInt(1), 64)
	high.Add(high, big.NewInt(100))
	tx.ChainID = high

	require.NotEqual(t, original, signer100.Hash(tx))
	/* FINISH ALPHA CHANGE 2942 -- verify signing hashes retain the full encoded chain ID -- */

	tx.ChainID = nil
	nilHash := signer100.Hash(tx)
	require.Nil(t, tx.ChainID)

	tx.ChainID = new(big.Int)
	require.Equal(t, nilHash, signer100.Hash(tx))
}

func TestDynamicFeeFields_LegacyFallbackPreserved(t *testing.T) {
	key, err := GenerateECDSAKey()
	require.NoError(t, err)

	fallback := NewEIP155Signer(100, true)
	london := NewLondonSigner(100, true, fallback)

	tx := dynamicFeeFieldsUnsignedTx()
	tx.Type = types.LegacyTx
	tx.GasPrice = big.NewInt(2)

	expected, err := fallback.SignTx(tx, key)
	require.NoError(t, err)

	actual, err := london.SignTx(tx, key)
	require.NoError(t, err)

	require.Equal(t, expected.MarshalRLP(), actual.MarshalRLP())
	require.Equal(t, fallback.Hash(tx), london.Hash(tx))

	sender, err := london.Sender(actual)
	require.NoError(t, err)
	require.Equal(t, PubKeyToAddress(&key.PublicKey), sender)
}

/* FINISH ALPHA CHANGE 2935 -- cover dynamic-fee signing and recovery field ownership -- */
