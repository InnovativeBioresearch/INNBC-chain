/* START ALPHA CHANGE 2936 -- cover full-width chain IDs and strict access-list decoding -- */
package types

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func TestDynamicFeeFields_ChainIDValidation(t *testing.T) {
	high := new(big.Int).Lsh(big.NewInt(1), 64)
	high.Add(high, big.NewInt(100))

	tests := []struct {
		name     string
		actual   *big.Int
		expected *big.Int
		valid    bool
	}{
		{"matching", big.NewInt(100), big.NewInt(100), true},
		{"matching_zero", new(big.Int), new(big.Int), true},
		{"missing", nil, big.NewInt(100), false},
		{"zero_mismatch", new(big.Int), big.NewInt(100), false},
		{"different", big.NewInt(101), big.NewInt(100), false},
		{"negative", big.NewInt(-100), big.NewInt(100), false},
		{"same_low_64_bits", high, big.NewInt(100), false},
		{"missing_configuration", big.NewInt(100), nil, false},
		{"negative_configuration", big.NewInt(100), big.NewInt(-1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &Transaction{
				Type:    DynamicFeeTx,
				ChainID: tt.actual,
			}

			var actualBefore, expectedBefore *big.Int
			if tt.actual != nil {
				actualBefore = new(big.Int).Set(tt.actual)
			}
			if tt.expected != nil {
				expectedBefore = new(big.Int).Set(tt.expected)
			}

			err := ValidateDynamicFeeChainID(tx, tt.expected)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrInvalidTxChainID)
			}

			require.Equal(t, actualBefore, tx.ChainID)
			require.Equal(t, expectedBefore, tt.expected)
		})
	}

	for _, txType := range []TxType{LegacyTx, StateTx} {
		require.NoError(t, ValidateDynamicFeeChainID(
			&Transaction{Type: txType}, nil,
		))
	}
}

func TestDynamicFeeFields_AccessListDecoding(t *testing.T) {
	tests := []struct {
		name string
		want error
	}{
		{"empty_list", nil},
		{"empty_bytes", ErrInvalidTxAccessList},
		{"nonempty_bytes", ErrInvalidTxAccessList},
		{"nonempty_list", ErrUnsupportedTxAccessList},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &fastrlp.Arena{}

			var accessList *fastrlp.Value
			switch tt.name {
			case "empty_list":
				accessList = a.NewArray()
			case "empty_bytes":
				accessList = a.NewNull()
			case "nonempty_bytes":
				accessList = a.NewBytes([]byte{1})
			case "nonempty_list":
				entry := a.NewArray()
				entry.Set(a.NewBytes(make([]byte, 20)))

				keys := a.NewArray()
				keys.Set(a.NewBytes(make([]byte, 32)))
				entry.Set(keys)

				accessList = a.NewArray()
				accessList.Set(entry)
			}

			payload := a.NewArray()
			payload.Set(a.NewUint(100))
			payload.Set(a.NewUint(0))
			payload.Set(a.NewUint(1))
			payload.Set(a.NewUint(2))
			payload.Set(a.NewUint(21000))
			payload.Set(a.NewBytes(StringToAddress("0x9002").Bytes()))
			payload.Set(a.NewUint(1))
			payload.Set(a.NewNull())
			payload.Set(accessList)
			payload.Set(a.NewUint(0))
			payload.Set(a.NewUint(1))
			payload.Set(a.NewUint(1))

			raw := append(
				[]byte{byte(DynamicFeeTx)},
				payload.MarshalTo(nil)...,
			)

			tx := &Transaction{}
			err := tx.UnmarshalRLP(raw)

			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
				return
			}

			require.NoError(t, err)
			require.Equal(t, DynamicFeeTx, tx.Type)
			require.Equal(t, big.NewInt(100), tx.ChainID)
			require.Equal(t, raw, tx.MarshalRLP())
		})
	}
}

/* FINISH ALPHA CHANGE 2936 -- cover full-width chain IDs and strict access-list decoding -- */
