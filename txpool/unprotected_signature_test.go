/* START ALPHA CHANGE 3089 -- verify pool admission rejects high-S unprotected signatures before insertion -- */
package txpool

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestUnprotectedSignature_PoolAdmission(t *testing.T) {
	key, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)
	expectedSender := crypto.PubKeyToAddress(&key.PublicKey)

	to := types.StringToAddress("0x9002")
	low, err := crypto.NewFrontierSigner(true).SignTx(&types.Transaction{
		Type:     types.LegacyTx,
		To:       &to,
		Gas:      21_000,
		GasPrice: big.NewInt(2),
		Value:    big.NewInt(1),
	}, key)
	require.NoError(t, err)
	require.True(t, low.V.Cmp(big.NewInt(27)) == 0 ||
		low.V.Cmp(big.NewInt(28)) == 0)

	order := new(big.Int).Set(crypto.S256.Params().N)
	halfOrder := new(big.Int).Rsh(new(big.Int).Set(order), 1)
	require.True(t, low.S.Cmp(halfOrder) <= 0)
	original := append([]byte(nil), low.MarshalRLP()...)

	high := low.Copy()
	high.S = new(big.Int).Sub(order, low.S)
	high.V = new(big.Int).Sub(big.NewInt(55), low.V)
	require.True(t, high.S.Cmp(halfOrder) > 0)

	sender, err := crypto.NewFrontierSigner(false).Sender(high)
	require.NoError(t, err)
	require.Equal(t, expectedSender, sender)
	require.Equal(t, original, low.MarshalRLP())

	routes := []struct {
		name   string
		signer crypto.TxSigner
	}{
		{
			"eip155",
			crypto.NewSigner(chain.ForksInTime{
				Homestead: true,
				EIP155:    true,
			}, 100),
		},
		{
			"london",
			crypto.NewSigner(chain.AllForksEnabled.At(0), 100),
		},
	}

	variants := []struct {
		name   string
		tx     *types.Transaction
		reject bool
	}{
		{"low_s", low, false},
		{"high_s", high, true},
	}

	for _, route := range routes {
		for _, variant := range variants {
			t.Run(route.name+"/"+variant.name, func(t *testing.T) {
				header := mockHeader.Copy()
				header.BaseFee = 1

				pool, err := newTestPool(NewDefaultMockStore(header))
				require.NoError(t, err)
				t.Cleanup(pool.Close)

				pool.SetSigner(route.signer)
				pool.SetSealing(true)
				require.NoError(t, pool.SetBaseFee(header))
				pool.Start()

				raw := variant.tx.MarshalRLP()
				transaction, err := DecodeRawTransaction(raw)
				require.NoError(t, err)
				require.Equal(t, types.ZeroAddress, transaction.From)
				transaction.ComputeHash(header.Number)
				hashBefore := transaction.Hash

				err = pool.AddTx(transaction)
				stored, found := pool.GetPendingTx(hashBefore)

				if variant.reject {
					require.ErrorIs(t, err, ErrExtractSignature)
					require.False(t, found)
					require.Nil(t, stored)
					require.Zero(t, pool.gauge.read())
					require.Equal(t, types.ZeroAddress, transaction.From)
				} else {
					require.NoError(t, err)
					require.True(t, found)
					require.NotNil(t, stored)
					require.Equal(t, expectedSender, stored.From)
					require.Equal(t, uint64(1), pool.gauge.read())
				}

				require.Equal(t, hashBefore, transaction.Hash)
				require.Equal(t, raw, transaction.MarshalRLP())
			})
		}
	}

	require.Equal(t, original, low.MarshalRLP())
}

/* FINISH ALPHA CHANGE 3089 -- verify pool admission rejects high-S unprotected signatures before insertion -- */
