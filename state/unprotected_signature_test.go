/* START ALPHA CHANGE 3090 -- reject high-S unprotected execution before state gas or receipt changes -- */
package state

import (
	"errors"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestUnprotectedSignature_ExecutionRecovery(t *testing.T) {
	key, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)
	sender := crypto.PubKeyToAddress(&key.PublicKey)

	low, err := crypto.NewFrontierSigner(true).SignTx(&types.Transaction{
		Type:     types.LegacyTx,
		To:       issue7ATarget.Ptr(),
		Gas:      21_000,
		GasPrice: big.NewInt(2),
		Value:    big.NewInt(1),
	}, key)
	require.NoError(t, err)
	require.Equal(t, types.ZeroAddress, low.From)
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

	recovered, err := crypto.NewFrontierSigner(false).Sender(high)
	require.NoError(t, err)
	require.Equal(t, sender, recovered)
	require.Equal(t, original, low.MarshalRLP())

	variants := []struct {
		name   string
		tx     *types.Transaction
		reject bool
	}{
		{"low_s", low, false},
		{"high_s", high, true},
	}

	for _, london := range []bool{false, true} {
		route := "eip155"
		if london {
			route = "london"
		}

		for _, variant := range variants {
			t.Run(route+"/"+variant.name, func(t *testing.T) {
				_, backend, tr, _ := issue7AExecutionFixture(t, []byte{0x00})
				require.True(t, tr.config.Homestead)
				require.True(t, tr.config.EIP155)
				tr.config.London = london

				tr.state.SetBalance(sender, big.NewInt(1_000_000_000))
				require.NoError(t, tr.Error())

				raw := variant.tx.MarshalRLP()
				transaction := new(types.Transaction)
				require.NoError(t, transaction.UnmarshalRLP(raw))
				require.Equal(t, types.ZeroAddress, transaction.From)
				transaction.ComputeHash(1)
				hashBefore := transaction.Hash

				balanceBefore := new(big.Int).Set(tr.GetBalance(sender))
				targetBalanceBefore := new(big.Int).Set(tr.GetBalance(issue7ATarget))
				nonceBefore := tr.GetNonce(sender)
				gasBefore := tr.gasPool
				contextBefore := tr.ctx

				if variant.reject {
					// Execution must not reach the destination's code lookup.
					backend.snapshot.codeErr = errors.New(
						"execution must not start for a high-S signature",
					)
				}

				err := tr.Write(transaction)
				if variant.reject {
					var applicationErr *TransitionApplicationError
					require.ErrorAs(t, err, &applicationErr)
					require.False(t, applicationErr.IsRecoverable)
					require.Contains(t, applicationErr.Err.Error(), "invalid txn signature")

					require.Equal(t, types.ZeroAddress, transaction.From)
					require.Zero(t, balanceBefore.Cmp(tr.GetBalance(sender)))
					require.Zero(t, targetBalanceBefore.Cmp(
						tr.GetBalance(issue7ATarget),
					))
					require.Equal(t, nonceBefore, tr.GetNonce(sender))
					require.Equal(t, gasBefore, tr.gasPool)
					require.Equal(t, contextBefore, tr.ctx)
					require.Zero(t, tr.TotalGas())
					require.Empty(t, tr.Receipts())
					require.Empty(t, tr.state.Logs())
					require.Zero(t, tr.state.GetRefund())
					require.Zero(t, backend.snapshot.commits)
				} else {
					require.NoError(t, err)
					require.Equal(t, sender, transaction.From)
					require.Equal(t, nonceBefore+1, tr.GetNonce(sender))
					require.Equal(t, uint64(21_000), tr.TotalGas())
					require.Equal(t, gasBefore-21_000, tr.gasPool)
					require.Zero(t, tr.GetBalance(sender).Cmp(
						new(big.Int).Sub(balanceBefore, big.NewInt(42_001)),
					))
					require.Zero(t, tr.GetBalance(issue7ATarget).Cmp(
						new(big.Int).Add(targetBalanceBefore, big.NewInt(1)),
					))
					require.Len(t, tr.Receipts(), 1)
					receipt := tr.Receipts()[0]
					require.NotNil(t, receipt.Status)
					require.Equal(t, types.ReceiptSuccess, *receipt.Status)
					require.Equal(t, uint64(21_000), receipt.GasUsed)
					require.Equal(t, hashBefore, receipt.TxHash)
				}

				require.NoError(t, tr.Error())
				require.Equal(t, hashBefore, transaction.Hash)
				require.Equal(t, raw, transaction.MarshalRLP())
			})
		}
	}

	require.Equal(t, original, low.MarshalRLP())
}

/* FINISH ALPHA CHANGE 3090 -- reject high-S unprotected execution before state gas or receipt changes -- */
