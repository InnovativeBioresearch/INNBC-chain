/* START ALPHA CHANGE 2937 -- verify pool rejection and preservation of signed chain IDs -- */
package txpool

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestDynamicFeeFields_PoolAdmission(t *testing.T) {
	high := new(big.Int).Lsh(big.NewInt(1), 64)
	high.Add(high, big.NewInt(100))

	tests := []struct {
		name  string
		id    *big.Int
		valid bool
	}{
		{"matching", big.NewInt(100), true},
		{"missing", nil, false},
		{"explicit_zero", new(big.Int), false},
		{"different", big.NewInt(101), false},
		{"same_low_64_bits", high, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := mockHeader.Copy()
			header.BaseFee = 1

			pool, err := newTestPool(NewDefaultMockStore(header))
			require.NoError(t, err)
			t.Cleanup(pool.Close)

			signer := crypto.NewLondonSigner(
				100, true, crypto.NewEIP155Signer(100, true),
			)
			pool.SetSigner(signer)
			require.NoError(t, pool.SetBaseFee(header))
			pool.Start()

			key, err := crypto.GenerateECDSAKey()
			require.NoError(t, err)

			to := types.StringToAddress("0x9002")
			tx, err := signer.SignTx(&types.Transaction{
				Type:      types.DynamicFeeTx,
				ChainID:   big.NewInt(100),
				To:        &to,
				Gas:       21000,
				Value:     big.NewInt(1),
				GasPrice:  new(big.Int),
				GasTipCap: big.NewInt(1),
				GasFeeCap: big.NewInt(2),
			}, key)
			require.NoError(t, err)

			if tt.id == nil {
				tx.ChainID = nil
			} else {
				tx.ChainID = new(big.Int).Set(tt.id)
			}

			idBefore := tx.ChainID
			rawBefore := append([]byte(nil), tx.MarshalRLP()...)
			fromBefore := tx.From
			hashBefore := tx.Hash

			err = pool.addTx(local, tx)

			if tt.valid {
				require.NoError(t, err)
				require.Same(t, idBefore, tx.ChainID)
				require.Equal(t, rawBefore, tx.MarshalRLP())
				require.Equal(t, crypto.PubKeyToAddress(&key.PublicKey), tx.From)
				require.Equal(t, uint64(1), pool.gauge.read())
				return
			}

			require.ErrorIs(t, err, types.ErrInvalidTxChainID)
			require.Equal(t, rawBefore, tx.MarshalRLP())
			require.Equal(t, fromBefore, tx.From)
			require.Equal(t, hashBefore, tx.Hash)
			require.Zero(t, pool.gauge.read())

			if idBefore == nil {
				require.Nil(t, tx.ChainID)
			} else {
				require.Same(t, idBefore, tx.ChainID)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2937 -- verify pool rejection and preservation of signed chain IDs -- */
