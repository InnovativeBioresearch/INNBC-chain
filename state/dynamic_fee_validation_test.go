/* START ALPHA CHANGE 2939 -- reject invalid block chain IDs before execution while preserving simulation -- */
package state

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func dynamicFeeFieldsTransition(
	t *testing.T,
) (*Transition, *types.Transaction, types.Address) {
	t.Helper()

	_, _, tr, tx := issue7AExecutionFixture(t, []byte{0x00})

	key, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	sender := crypto.PubKeyToAddress(&key.PublicKey)
	tr.state.SetBalance(sender, big.NewInt(1_000_000_000))

	tx.Type = types.DynamicFeeTx
	tx.ChainID = big.NewInt(100)
	tx.From = types.ZeroAddress
	tx.Gas = 21000
	tx.GasPrice = new(big.Int)
	tx.GasTipCap = big.NewInt(1)
	tx.GasFeeCap = big.NewInt(2)

	signer := crypto.NewLondonSigner(
		100, true, crypto.NewEIP155Signer(100, true),
	)
	signed, err := signer.SignTx(tx, key)
	require.NoError(t, err)

	return tr, signed, sender
}

func TestDynamicFeeFields_WriteRejectsWithoutExecution(t *testing.T) {
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
			for _, cachedFrom := range []bool{false, true} {
				name := "recover_sender"
				if cachedFrom {
					name = "cached_sender"
				}

				t.Run(name, func(t *testing.T) {
					tr, tx, sender := dynamicFeeFieldsTransition(t)

					if cachedFrom {
						tx.From = sender
					}

					if tt.id == nil {
						tx.ChainID = nil
					} else {
						tx.ChainID = new(big.Int).Set(tt.id)
					}

					slot := types.StringToHash("0x01")
					stored := types.StringToHash("0x02")
					tr.state.SetState(*tx.To, slot, stored)

					senderBalance := new(big.Int).Set(
						tr.state.GetBalance(sender),
					)
					targetBalance := new(big.Int).Set(
						tr.state.GetBalance(*tx.To),
					)
					nonce := tr.state.GetNonce(sender)
					gasPool := tr.gasPool
					contextBefore := tr.ctx
					fromBefore := tx.From
					hashBefore := tx.Hash
					rawBefore := append([]byte(nil), tx.MarshalRLP()...)

					err := tr.Write(tx)

					var applicationErr *TransitionApplicationError
					require.ErrorAs(t, err, &applicationErr)
					require.ErrorIs(
						t, applicationErr.Err, types.ErrInvalidTxChainID,
					)
					require.False(t, applicationErr.IsRecoverable)

					/* START ALPHA CHANGE 2944 -- compare preserved balances numerically -- */
					require.Zero(
						t,
						senderBalance.Cmp(tr.state.GetBalance(sender)),
						"sender balance changed after transaction rejection",
					)
					require.Zero(
						t,
						targetBalance.Cmp(tr.state.GetBalance(*tx.To)),
						"target balance changed after transaction rejection",
					)
					/* FINISH ALPHA CHANGE 2944 -- compare preserved balances numerically -- */
					require.Equal(t, nonce, tr.state.GetNonce(sender))
					require.Equal(t, stored, tr.state.GetState(*tx.To, slot))
					require.Equal(t, gasPool, tr.gasPool)
					require.Equal(t, contextBefore, tr.ctx)
					require.Zero(t, tr.TotalGas())
					require.Empty(t, tr.Receipts())
					require.Empty(t, tr.state.Logs())
					require.NoError(t, tr.Error())

					require.Equal(t, fromBefore, tx.From)
					require.Equal(t, hashBefore, tx.Hash)
					require.Equal(t, rawBefore, tx.MarshalRLP())
				})
			}
		})
	}
}

/* START ALPHA CHANGE 2941 -- complete execution and unsigned simulation regressions -- */
func TestDynamicFeeFields_WriteAcceptsMatchingID(t *testing.T) {
	for _, cachedFrom := range []bool{false, true} {
		name := "recover_sender"
		if cachedFrom {
			name = "cached_sender"
		}

		t.Run(name, func(t *testing.T) {
			tr, tx, sender := dynamicFeeFieldsTransition(t)

			if cachedFrom {
				tx.From = sender
			}

			rawBefore := append([]byte(nil), tx.MarshalRLP()...)
			idBefore := tx.ChainID

			require.NoError(t, tr.Write(tx))
			require.NoError(t, tr.Error())
			require.Equal(t, sender, tx.From)
			require.Same(t, idBefore, tx.ChainID)
			require.Equal(t, rawBefore, tx.MarshalRLP())
			require.Equal(t, uint64(1), tr.state.GetNonce(sender))
			require.Equal(t, uint64(21000), tr.TotalGas())
			require.Len(t, tr.Receipts(), 1)
		})
	}
}

func TestDynamicFeeFields_ApplyStillSupportsUnsignedSimulation(t *testing.T) {
	tr, tx, sender := dynamicFeeFieldsTransition(t)

	tx.From = sender
	tx.ChainID = nil
	tx.V = nil
	tx.R = nil
	tx.S = nil

	result, err := tr.Apply(tx)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NoError(t, result.Err)
	require.Nil(t, tx.ChainID)
}

/* FINISH ALPHA CHANGE 2941 -- complete execution and unsigned simulation regressions -- */

/* FINISH ALPHA CHANGE 2939 -- reject invalid block chain IDs before execution while preserving simulation -- */
