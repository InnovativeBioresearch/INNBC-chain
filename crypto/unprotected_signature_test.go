/* START ALPHA CHANGE 3088 -- verify unprotected signature canonicality through signer fallback and fork wiring -- */
package crypto

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func unprotectedSignaturePair(
	t *testing.T,
) (*types.Transaction, *types.Transaction, types.Address) {
	t.Helper()

	key, err := GenerateECDSAKey()
	require.NoError(t, err)

	to := types.StringToAddress("0x9002")
	low, err := NewFrontierSigner(true).SignTx(&types.Transaction{
		Type:     types.LegacyTx,
		To:       &to,
		Gas:      21_000,
		GasPrice: big.NewInt(2),
		Value:    big.NewInt(1),
	}, key)
	require.NoError(t, err)
	require.Equal(t, types.ZeroAddress, low.From)
	require.True(t, low.V.Cmp(big.NewInt(27)) == 0 ||
		low.V.Cmp(big.NewInt(28)) == 0)

	order := new(big.Int).Set(S256.Params().N)
	halfOrder := new(big.Int).Rsh(new(big.Int).Set(order), 1)
	require.True(t, low.S.Sign() > 0)
	require.True(t, low.S.Cmp(halfOrder) <= 0)

	low.ComputeHash(1)
	original := append([]byte(nil), low.MarshalRLP()...)

	high := low.Copy()
	high.S = new(big.Int).Sub(order, low.S)
	high.V = new(big.Int).Sub(big.NewInt(55), low.V)
	high.ComputeHash(1)

	require.True(t, high.S.Cmp(halfOrder) > 0)
	require.True(t, high.S.Cmp(order) < 0)
	require.Equal(t, low.R, high.R)
	require.NotEqual(t, low.Hash, high.Hash)
	require.NotEqual(t, low.MarshalRLP(), high.MarshalRLP())
	require.Equal(t, original, low.MarshalRLP())

	frontier := NewFrontierSigner(false)
	require.Equal(t, frontier.Hash(low), frontier.Hash(high))

	return low, high, PubKeyToAddress(&key.PublicKey)
}

func TestUnprotectedSignature_FallbackRules(t *testing.T) {
	low, high, expectedSender := unprotectedSignaturePair(t)
	lowRaw := append([]byte(nil), low.MarshalRLP()...)
	highRaw := append([]byte(nil), high.MarshalRLP()...)

	for _, homestead := range []bool{false, true} {
		phase := "before_homestead"
		if homestead {
			phase = "homestead"
		}

		routes := []struct {
			name   string
			signer TxSigner
		}{
			{
				"frontier",
				NewFrontierSigner(homestead),
			},
			{
				"eip155",
				NewEIP155Signer(1982, homestead),
			},
			{
				"london",
				NewLondonSigner(
					1982, homestead,
					NewEIP155Signer(1982, homestead),
				),
			},
			{
				"new_signer_eip155",
				NewSigner(chain.ForksInTime{
					Homestead: homestead,
					EIP155:    true,
				}, 1982),
			},
			{
				"new_signer_london",
				NewSigner(chain.ForksInTime{
					Homestead: homestead,
					EIP155:    true,
					London:    true,
				}, 1982),
			},
		}

		for _, route := range routes {
			t.Run(phase+"/"+route.name, func(t *testing.T) {
				sender, err := route.signer.Sender(low)
				require.NoError(t, err)
				require.Equal(t, expectedSender, sender)

				sender, err = route.signer.Sender(high)
				if homestead {
					require.Error(t, err)
					require.Contains(t, err.Error(), "invalid txn signature")
				} else {
					require.NoError(t, err)
					require.Equal(t, expectedSender, sender)
				}

				require.Equal(t, lowRaw, low.MarshalRLP())
				require.Equal(t, highRaw, high.MarshalRLP())
			})
		}
	}
}

func TestUnprotectedSignature_DefaultForkWiring(t *testing.T) {
	forks := chain.AllForksEnabled.At(0)
	require.True(t, forks.Homestead)
	require.True(t, forks.EIP155)
	require.True(t, forks.London)

	low, high, expectedSender := unprotectedSignaturePair(t)
	signer := NewSigner(forks, 1982)

	sender, err := signer.Sender(low)
	require.NoError(t, err)
	require.Equal(t, expectedSender, sender)

	_, err = signer.Sender(high)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid txn signature")
}

func TestUnprotectedSignature_ProtectedFormats(t *testing.T) {
	key, err := GenerateECDSAKey()
	require.NoError(t, err)
	expectedSender := PubKeyToAddress(&key.PublicKey)

	signer := NewSigner(chain.AllForksEnabled.At(0), 1982)
	to := types.StringToAddress("0x9002")

	for _, txType := range []types.TxType{
		types.LegacyTx,
		types.DynamicFeeTx,
	} {
		name := "protected_legacy"
		if txType == types.DynamicFeeTx {
			name = "dynamic_fee"
		}

		t.Run(name, func(t *testing.T) {
			transaction := &types.Transaction{
				Type:     txType,
				To:       &to,
				Gas:      21_000,
				GasPrice: big.NewInt(2),
				Value:    big.NewInt(1),
			}
			if txType == types.DynamicFeeTx {
				transaction.ChainID = big.NewInt(1982)
				transaction.GasPrice = new(big.Int)
				transaction.GasTipCap = big.NewInt(1)
				transaction.GasFeeCap = big.NewInt(2)
			}

			low, err := signer.SignTx(transaction, key)
			require.NoError(t, err)
			original := append([]byte(nil), low.MarshalRLP()...)

			sender, err := signer.Sender(low)
			require.NoError(t, err)
			require.Equal(t, expectedSender, sender)

			order := new(big.Int).Set(S256.Params().N)
			halfOrder := new(big.Int).Rsh(new(big.Int).Set(order), 1)
			require.True(t, low.S.Cmp(halfOrder) <= 0)

			high := low.Copy()
			high.S = new(big.Int).Sub(order, low.S)

			if txType == types.DynamicFeeTx {
				require.True(t, low.V.Sign() == 0 ||
					low.V.Cmp(big.NewInt(1)) == 0)
				high.V = new(big.Int).Sub(big.NewInt(1), low.V)
			} else {
				base := big.NewInt(2*1982 + 35)
				parity := new(big.Int).Sub(low.V, base)
				require.True(t, parity.Sign() == 0 ||
					parity.Cmp(big.NewInt(1)) == 0)
				high.V = new(big.Int).Add(
					base,
					new(big.Int).Sub(big.NewInt(1), parity),
				)
			}

			require.Equal(t, signer.Hash(low), signer.Hash(high))
			_, err = signer.Sender(high)
			require.Error(t, err)
			require.Contains(t, err.Error(), "invalid txn signature")
			require.Equal(t, original, low.MarshalRLP())
		})
	}
}

/* FINISH ALPHA CHANGE 3088 -- verify unprotected signature canonicality through signer fallback and fork wiring -- */
