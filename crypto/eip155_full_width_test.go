/* START ALPHA CHANGE 3602 -- verify full-width legacy signature recovery and preserved unprotected rules -- */
package crypto

import (
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type eip155FullWidthRoute struct {
	name   string
	signer TxSigner
}

func eip155FullWidthRoutes(
	chainID uint64,
	homestead bool,
) []eip155FullWidthRoute {
	return []eip155FullWidthRoute{
		{
			"eip155",
			NewEIP155Signer(chainID, homestead),
		},
		{
			"london",
			NewLondonSigner(
				chainID, homestead,
				NewEIP155Signer(chainID, homestead),
			),
		},
		{
			"new_signer",
			NewSigner(chain.ForksInTime{
				Homestead: homestead,
				EIP155:    true,
				London:    true,
			}, chainID),
		},
	}
}

func eip155FullWidthFixtures(
	t *testing.T,
	chainID uint64,
) ([2]*types.Transaction, types.Address) {
	t.Helper()

	// Fixed test key; vary the nonce to obtain both recovery parities.
	keyBytes := make([]byte, 32)
	keyBytes[31] = 1
	key, err := ParseECDSAPrivateKey(keyBytes)
	require.NoError(t, err)

	to := types.StringToAddress(
		"0x1000000000000000000000000000000000000002",
	)
	signer := NewEIP155Signer(chainID, true)

	baseV := new(big.Int).Lsh(new(big.Int).SetUint64(chainID), 1)
	baseV.Add(baseV, big.NewInt(35))

	var fixtures [2]*types.Transaction
	found := 0

	// Bounded fixture search: never silently omit either parity.
	for nonce := uint64(0); nonce < 128 && found < 2; nonce++ {
		tx, err := signer.SignTx(&types.Transaction{
			Type:     types.LegacyTx,
			Nonce:    nonce,
			To:       &to,
			Gas:      21000,
			GasPrice: big.NewInt(2),
			Value:    big.NewInt(1),
		}, key)
		require.NoError(t, err)
		require.True(t, tx.S.Cmp(secp256k1NHalf) <= 0)

		parity := new(big.Int).Sub(tx.V, baseV)
		require.True(t,
			parity.Sign() >= 0 && parity.Cmp(big.NewInt(1)) <= 0,
		)

		index := parity.Uint64()
		if fixtures[index] == nil {
			fixtures[index] = tx
			found++
		}
	}

	require.Equal(t, 2, found, "fixtures must include both recovery parities")
	return fixtures, PubKeyToAddress(&key.PublicKey)
}

func TestEIP155FullWidth_ProtectedRecovery(t *testing.T) {
	for _, chainID := range []uint64{
		1,
		1982,
		uint64(math.MaxInt64) - 3,
		uint64(math.MaxInt64),
	} {
		t.Run(fmt.Sprintf("chain_%d", chainID), func(t *testing.T) {
			fixtures, expectedSender := eip155FullWidthFixtures(t, chainID)

			wrongChainID := chainID - 1
			if chainID == 1 {
				wrongChainID = 2
			}

			for parity, tx := range fixtures {
				t.Run(fmt.Sprintf("parity_%d", parity), func(t *testing.T) {
					if chainID == uint64(math.MaxInt64)-3 {
						require.Greater(t, tx.V.BitLen(), 64)
						require.Equal(t, uint64(27+parity), tx.V.Uint64())
					}

					raw := append([]byte(nil), tx.MarshalRLP()...)

					var decoded types.Transaction
					require.NoError(t, decoded.UnmarshalRLP(raw))
					require.Zero(t, tx.V.Cmp(decoded.V))
					require.Equal(t, raw, decoded.MarshalRLP())

					for _, variant := range []struct {
						name string
						tx   *types.Transaction
					}{
						{"original", tx},
						{"rlp_round_trip", &decoded},
					} {
						t.Run(variant.name, func(t *testing.T) {
							require.Equal(t, types.ZeroAddress, variant.tx.From)

							for _, route := range eip155FullWidthRoutes(chainID, true) {
								t.Run(route.name, func(t *testing.T) {
									sender, err := route.signer.Sender(variant.tx)
									require.NoError(t, err)
									require.Equal(t, expectedSender, sender)
								})
							}

							for _, route := range eip155FullWidthRoutes(wrongChainID, true) {
								t.Run("mismatched_"+route.name, func(t *testing.T) {
									sender, err := route.signer.Sender(variant.tx)
									require.Error(t, err)
									require.Equal(t, types.ZeroAddress, sender)
								})
							}

							require.Equal(t, raw, variant.tx.MarshalRLP())
						})
					}

					require.Equal(t, raw, tx.MarshalRLP())
				})
			}
		})
	}
}

func TestEIP155FullWidth_UnprotectedCompatibility(t *testing.T) {
	// Reuse the existing fixture containing equivalent low-S/high-S
	// unprotected signatures with their recovery values swapped.
	low, high, expectedSender := unprotectedSignaturePair(t)
	lowRaw := append([]byte(nil), low.MarshalRLP()...)
	highRaw := append([]byte(nil), high.MarshalRLP()...)

	for _, chainID := range []uint64{
		1982,
		uint64(math.MaxInt64) - 3,
		uint64(math.MaxInt64),
	} {
		for _, homestead := range []bool{false, true} {
			for _, route := range eip155FullWidthRoutes(chainID, homestead) {
				name := fmt.Sprintf(
					"chain_%d/homestead_%t/%s",
					chainID, homestead, route.name,
				)
				t.Run(name, func(t *testing.T) {
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
}

/* FINISH ALPHA CHANGE 3602 -- verify full-width legacy signature recovery and preserved unprotected rules -- */
