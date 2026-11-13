/* START ALPHA CHANGE 3896 -- verify nonce exhaustion preserves execution capacity and valid boundaries -- */

package state

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func nonceLimitExecutionFixture(
	t *testing.T,
	nonce uint64,
) (*Transition, *ecdsa.PrivateKey, types.Address) {
	t.Helper()

	_, _, tr, _ := issue7AExecutionFixture(t, []byte{0x00})

	key, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	sender := crypto.PubKeyToAddress(&key.PublicKey)
	tr.state.SetBalance(sender, big.NewInt(1_000_000_000))
	tr.state.SetNonce(sender, nonce)
	require.NoError(t, tr.Error())

	return tr, key, sender
}

func nonceLimitExecutionTransaction(
	t *testing.T,
	key *ecdsa.PrivateKey,
	txType types.TxType,
	nonce uint64,
	gas uint64,
	creation bool,
) *types.Transaction {
	t.Helper()

	recipient := types.StringToAddress("0x7303")
	tx := &types.Transaction{
		Type:     txType,
		From:     types.ZeroAddress,
		To:       recipient.Ptr(),
		Nonce:    nonce,
		Gas:      gas,
		GasPrice: big.NewInt(2),
		Value:    big.NewInt(7),
	}

	if txType == types.DynamicFeeTx {
		tx.ChainID = big.NewInt(100)
		tx.GasPrice = new(big.Int)
		tx.GasFeeCap = big.NewInt(10)
		tx.GasTipCap = big.NewInt(1)
	}

	if creation {
		tx.To = nil
		tx.Input = []byte{0x00}
	}

	signer := crypto.NewLondonSigner(
		100, true, crypto.NewEIP155Signer(100, true),
	)
	signed, err := signer.SignTx(tx, key)
	require.NoError(t, err)
	signed.ComputeHash(1)

	return signed
}

func TestNonceLimit_ExecutionRejectsMaximum(t *testing.T) {
	for _, txType := range []types.TxType{
		types.LegacyTx, types.DynamicFeeTx,
	} {
		for _, creation := range []bool{false, true} {
			for _, write := range []bool{false, true} {
				t.Run(
					fmt.Sprintf("%s/creation_%t/write_%t", txType, creation, write),
					func(t *testing.T) {
						tr, key, sender := nonceLimitExecutionFixture(
							t, math.MaxUint64,
						)
						tx := nonceLimitExecutionTransaction(
							t, key, txType, math.MaxUint64, tr.gasPool, creation,
						)

						target := crypto.CreateAddress(sender, tx.Nonce)
						if tx.To != nil {
							target = *tx.To
						}

						senderBefore := new(big.Int).Set(tr.GetBalance(sender))
						targetBefore := new(big.Int).Set(tr.GetBalance(target))
						existsBefore := tr.AccountExists(target)
						gasBefore := tr.gasPool
						contextBefore := tr.ctx

						var rejection error
						if write {
							rejection = tr.Write(tx)
						} else {
							request := tx.Copy()
							request.From = sender
							result, err := tr.Apply(request)
							require.Nil(t, result)
							rejection = err
						}

						var applicationErr *TransitionApplicationError
						require.ErrorAs(t, rejection, &applicationErr)
						require.ErrorIs(t, applicationErr.Err, ErrNonceUintOverflow)
						require.False(t, applicationErr.IsRecoverable)

						require.NoError(t, tr.Error())
						require.Equal(t, uint64(math.MaxUint64), tr.GetNonce(sender))
						require.Zero(t, senderBefore.Cmp(tr.GetBalance(sender)))
						require.Zero(t, targetBefore.Cmp(tr.GetBalance(target)))
						require.Equal(t, existsBefore, tr.AccountExists(target))
						require.Equal(t, gasBefore, tr.gasPool)
						require.Equal(t, contextBefore, tr.ctx)
						require.Zero(t, tr.TotalGas())
						require.Empty(t, tr.Receipts())
						require.Empty(t, tr.state.Logs())
						require.Zero(t, tr.state.GetRefund())

						// The rejected full-block reservation must leave room
						// for a different sender on this same transition.
						nextKey, err := crypto.GenerateECDSAKey()
						require.NoError(t, err)
						nextSender := crypto.PubKeyToAddress(&nextKey.PublicKey)
						require.NotEqual(t, sender, nextSender)
						tr.state.SetBalance(nextSender, big.NewInt(1_000_000_000))

						next := nonceLimitExecutionTransaction(
							t, nextKey, txType, 0, 21_000, false,
						)
						require.NoError(t, tr.Write(next))
						require.NoError(t, tr.Error())

						require.Len(t, tr.Receipts(), 1)
						receipt := tr.Receipts()[0]
						require.NotNil(t, receipt.Status)
						require.Equal(t, types.ReceiptSuccess, *receipt.Status)
						require.Equal(t, next.Hash, receipt.TxHash)
						require.Equal(t, uint64(21_000), receipt.GasUsed)
						require.Equal(t, uint64(21_000), tr.TotalGas())
						require.Equal(t, gasBefore-21_000, tr.gasPool)
						require.Equal(t, uint64(1), tr.GetNonce(nextSender))
						require.Zero(t, tr.GetBalance(*next.To).Cmp(big.NewInt(7)))
						require.Zero(t, tr.GetBalance(nextSender).Cmp(
							big.NewInt(1_000_000_000-42_000-7),
						))
					},
				)
			}
		}
	}
}

func TestNonceLimit_ExecutionBoundary(t *testing.T) {
	for _, txType := range []types.TxType{
		types.LegacyTx, types.DynamicFeeTx,
	} {
		for _, creation := range []bool{false, true} {
			for _, initialNonce := range []uint64{0, math.MaxUint64 - 1} {
				t.Run(
					fmt.Sprintf("%s/creation_%t/nonce_%d", txType, creation, initialNonce),
					func(t *testing.T) {
						tr, key, sender := nonceLimitExecutionFixture(t, initialNonce)
						gasBefore := tr.gasPool
						tx := nonceLimitExecutionTransaction(
							t, key, txType, initialNonce, gasBefore, creation,
						)

						target := crypto.CreateAddress(sender, initialNonce)
						if tx.To != nil {
							target = *tx.To
						}

						require.NoError(t, tr.Write(tx))
						require.NoError(t, tr.Error())
						require.Equal(t, initialNonce+1, tr.GetNonce(sender))

						wantGas := uint64(21_000)
						if creation {
							// Creation intrinsic gas plus one zero initcode byte.
							wantGas = 53_004
						}

						require.Len(t, tr.Receipts(), 1)
						receipt := tr.Receipts()[0]
						require.NotNil(t, receipt.Status)
						require.Equal(t, types.ReceiptSuccess, *receipt.Status)
						require.Equal(t, tx.Hash, receipt.TxHash)
						require.Equal(t, wantGas, receipt.GasUsed)
						require.Equal(t, wantGas, tr.TotalGas())
						require.Equal(t, gasBefore-wantGas, tr.gasPool)
						require.Zero(t, tr.GetBalance(target).Cmp(big.NewInt(7)))

						wantBalance := new(big.Int).Sub(
							big.NewInt(1_000_000_000),
							new(big.Int).SetUint64(2*wantGas+7),
						)
						require.Zero(t, wantBalance.Cmp(tr.GetBalance(sender)))

						if initialNonce == math.MaxUint64-1 {
							// Reject the next nonce without changing the
							// already successful transaction's effects.
							gasAfter := tr.gasPool
							balanceAfter := new(big.Int).Set(tr.GetBalance(sender))
							targetAfter := new(big.Int).Set(tr.GetBalance(target))

							terminal := nonceLimitExecutionTransaction(
								t, key, txType, math.MaxUint64, gasAfter, creation,
							)
							err := tr.Write(terminal)

							var applicationErr *TransitionApplicationError
							require.ErrorAs(t, err, &applicationErr)
							require.ErrorIs(t, applicationErr.Err, ErrNonceUintOverflow)
							require.False(t, applicationErr.IsRecoverable)

							require.NoError(t, tr.Error())
							require.Equal(t, uint64(math.MaxUint64), tr.GetNonce(sender))
							require.Equal(t, gasAfter, tr.gasPool)
							require.Equal(t, wantGas, tr.TotalGas())
							require.Zero(t, balanceAfter.Cmp(tr.GetBalance(sender)))
							require.Zero(t, targetAfter.Cmp(tr.GetBalance(target)))
							require.Len(t, tr.Receipts(), 1)
							require.Equal(t, tx.Hash, tr.Receipts()[0].TxHash)
						}
					},
				)
			}
		}
	}
}

func TestNonceLimit_RetainedStateFailurePriority(t *testing.T) {
	_, _, tr, _ := issue7AExecutionFixture(t, []byte{0x00})
	cause := errors.New("retained storage failure")
	tr.state.setError(cause)

	err := checkAndProcessTx(
		&types.Transaction{Nonce: math.MaxUint64},
		tr,
	)

	require.ErrorIs(t, err, ErrLocalStateFailure)
	require.ErrorIs(t, err, cause)
}

/* FINISH ALPHA CHANGE 3896 -- verify nonce exhaustion preserves execution capacity and valid boundaries -- */
