/* START ALPHA CHANGE 3043 -- verify upfront affordability without execution side effects -- */
package state

import (
	"errors"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func txAffordabilityFixture(
	t *testing.T,
	txType types.TxType,
	balance *big.Int,
	zeroFee bool,
	creation bool,
) (*issue7AExecutionState, *Transition, *types.Transaction, types.Address) {
	t.Helper()

	_, backend, tr, tx := issue7AExecutionFixture(t, []byte{0x00})
	key, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	sender := crypto.PubKeyToAddress(&key.PublicKey)
	backend.snapshot.accounts[sender] = &Account{
		Balance:  new(big.Int).Set(balance),
		Root:     types.EmptyRootHash,
		CodeHash: types.EmptyCodeHash.Bytes(),
	}

	tx.Type = txType
	tx.From = types.ZeroAddress
	tx.Nonce = 0
	tx.Gas = 100_000
	tx.GasPrice = big.NewInt(2)
	tx.Value = big.NewInt(11)

	if txType == types.DynamicFeeTx {
		tx.ChainID = big.NewInt(100)
		tx.GasPrice = new(big.Int)
		tx.GasFeeCap = big.NewInt(10)
		tx.GasTipCap = big.NewInt(1)
	}

	if zeroFee {
		tr.ctx.BaseFee = new(big.Int)
		tx.GasPrice = new(big.Int)
		tx.GasFeeCap = new(big.Int)
		tx.GasTipCap = new(big.Int)
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

	return backend, tr, signed, sender
}

func TestTxAffordability_WriteRejectsWithoutExecution(t *testing.T) {
	tests := []struct {
		name     string
		txType   types.TxType
		balance  int64
		zeroFee  bool
		creation bool
		wantErr  error
	}{
		{
			name:    "legacy_gas_unaffordable",
			txType:  types.LegacyTx,
			balance: 199_999,
			wantErr: ErrNotEnoughFundsForGas,
		},
		{
			name:    "legacy_gas_plus_value_missing_one",
			txType:  types.LegacyTx,
			balance: 200_010,
			wantErr: ErrNotEnoughFundsForGasAndValue,
		},
		{
			name:    "dynamic_effective_price_affordable_but_cap_unaffordable",
			txType:  types.DynamicFeeTx,
			balance: 200_011,
			wantErr: ErrNotEnoughFundsForGas,
		},
		{
			name:    "dynamic_cap_plus_value_missing_one",
			txType:  types.DynamicFeeTx,
			balance: 1_000_010,
			wantErr: ErrNotEnoughFundsForGasAndValue,
		},
		{
			name:    "zero_legacy_fee_still_requires_value",
			txType:  types.LegacyTx,
			balance: 10,
			zeroFee: true,
			wantErr: ErrNotEnoughFundsForGasAndValue,
		},
		{
			name:    "zero_dynamic_cap_still_requires_value",
			txType:  types.DynamicFeeTx,
			balance: 10,
			zeroFee: true,
			wantErr: ErrNotEnoughFundsForGasAndValue,
		},
		{
			name:     "creation_gas_plus_value_missing_one",
			txType:   types.LegacyTx,
			balance:  200_010,
			creation: true,
			wantErr:  ErrNotEnoughFundsForGasAndValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, cachedSender := range []bool{false, true} {
				name := "recover_sender"
				if cachedSender {
					name = "cached_sender"
				}

				t.Run(name, func(t *testing.T) {
					backend, tr, tx, sender := txAffordabilityFixture(
						t, tt.txType, big.NewInt(tt.balance),
						tt.zeroFee, tt.creation,
					)
					if cachedSender {
						tx.From = sender
					}

					target := crypto.CreateAddress(sender, tx.Nonce)
					if tx.To != nil {
						target = *tx.To
					}

					senderBalance := new(big.Int).Set(tr.GetBalance(sender))
					targetBalance := new(big.Int).Set(tr.GetBalance(target))
					senderNonce := tr.GetNonce(sender)
					targetNonce := tr.GetNonce(target)
					targetExists := tr.AccountExists(target)
					targetCodeHash := tr.GetCodeHash(target)
					gasPool := tr.gasPool
					contextBefore := tr.ctx
					rawBefore := append([]byte(nil), tx.MarshalRLP()...)

					// A contract code read would prove execution started.
					backend.snapshot.codeErr = errors.New(
						"execution must not start for an unaffordable transaction",
					)

					err := tr.Write(tx)
					var applicationErr *TransitionApplicationError
					require.ErrorAs(t, err, &applicationErr)
					require.ErrorIs(t, applicationErr.Err, tt.wantErr)
					require.True(t, applicationErr.IsRecoverable)
					require.NoError(t, tr.Error())

					require.Zero(t, senderBalance.Cmp(tr.GetBalance(sender)))
					require.Zero(t, targetBalance.Cmp(tr.GetBalance(target)))
					require.Equal(t, senderNonce, tr.GetNonce(sender))
					require.Equal(t, targetNonce, tr.GetNonce(target))
					require.Equal(t, targetExists, tr.AccountExists(target))
					require.Equal(t, targetCodeHash, tr.GetCodeHash(target))
					require.Equal(t, gasPool, tr.gasPool)
					require.Equal(t, contextBefore, tr.ctx)
					require.Zero(t, tr.TotalGas())
					require.Empty(t, tr.Receipts())
					require.Empty(t, tr.state.Logs())
					require.Zero(t, tr.state.GetRefund())
					require.Equal(t, rawBefore, tx.MarshalRLP())
				})
			}
		})
	}
}

func TestTxAffordability_ExactBalanceAndActualCharging(t *testing.T) {
	for _, txType := range []types.TxType{
		types.LegacyTx, types.DynamicFeeTx,
	} {
		name := "legacy"
		if txType == types.DynamicFeeTx {
			name = "dynamic"
		}

		t.Run(name, func(t *testing.T) {
			for _, zeroFee := range []bool{false, true} {
				name := "positive_fee"
				if zeroFee {
					name = "zero_fee"
				}

				t.Run(name, func(t *testing.T) {
					requiredBalance := int64(200_011)
					if txType == types.DynamicFeeTx {
						requiredBalance = 1_000_011
					}
					chargedPrice := int64(2)
					if zeroFee {
						requiredBalance = 11
						chargedPrice = 0
					}

					_, tr, tx, sender := txAffordabilityFixture(
						t, txType, big.NewInt(requiredBalance), zeroFee, false,
					)
					target := *tx.To
					gasPool := tr.gasPool

					require.NoError(t, tr.Write(tx))
					require.NoError(t, tr.Error())
					require.Len(t, tr.Receipts(), 1)

					receipt := tr.Receipts()[0]
					require.NotNil(t, receipt.Status)
					require.Equal(t, types.ReceiptSuccess, *receipt.Status)
					require.Equal(t, uint64(21_000), receipt.GasUsed)
					require.Equal(t, uint64(21_000), tr.TotalGas())
					require.Equal(t, gasPool-21_000, tr.gasPool)
					require.Equal(t, uint64(1), tr.GetNonce(sender))
					require.Zero(t, tr.GetBalance(target).Cmp(big.NewInt(11)))

					actualCost := int64(21_000)*chargedPrice + 11
					require.Zero(t, tr.GetBalance(sender).Cmp(
						big.NewInt(requiredBalance-actualCost),
					))
				})
			}
		})
	}
}

func TestTxAffordability_FullWidthArithmetic(t *testing.T) {
	_, _, tr, _ := issue7AExecutionFixture(t, []byte{0x00})

	fee := new(big.Int).Lsh(big.NewInt(1), 128)
	value := new(big.Int).Lsh(big.NewInt(1), 190)
	msg := &types.Transaction{
		Type:      types.DynamicFeeTx,
		From:      issue7ASender,
		Gas:       ^uint64(0),
		GasPrice:  new(big.Int),
		GasFeeCap: new(big.Int).Set(fee),
		Value:     new(big.Int).Set(value),
	}
	gasCost := new(big.Int).Mul(
		new(big.Int).SetUint64(msg.Gas), fee,
	)
	required := new(big.Int).Add(new(big.Int).Set(gasCost), value)

	tests := []struct {
		name    string
		balance *big.Int
		wantErr error
	}{
		{
			name:    "below_maximum_gas",
			balance: new(big.Int).Sub(new(big.Int).Set(gasCost), big.NewInt(1)),
			wantErr: ErrNotEnoughFundsForGas,
		},
		{
			name:    "below_combined_requirement",
			balance: new(big.Int).Sub(new(big.Int).Set(required), big.NewInt(1)),
			wantErr: ErrNotEnoughFundsForGasAndValue,
		},
		{
			name:    "exact_combined_requirement",
			balance: new(big.Int).Set(required),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr.state.SetBalance(msg.From, tt.balance)
			before := new(big.Int).Set(tr.GetBalance(msg.From))

			err := tr.checkTxBalance(msg)
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}

			require.NoError(t, tr.Error())
			require.Zero(t, before.Cmp(tr.GetBalance(msg.From)))
			require.Zero(t, fee.Cmp(msg.GasFeeCap))
			require.Zero(t, value.Cmp(msg.Value))
		})
	}
}

func TestTxAffordability_LocalFailureTakesPrecedence(t *testing.T) {
	backend, tr, tx, sender := txAffordabilityFixture(
		t, types.LegacyTx, big.NewInt(0), false, false,
	)
	tx.From = sender

	cause := errors.New("injected affordability account read failure")
	backend.snapshot.accountFailureAddress = sender
	backend.snapshot.accountErr = cause

	err := tr.checkTxBalance(tx)
	require.ErrorIs(t, err, ErrLocalStateFailure)
	require.ErrorIs(t, err, cause)
	require.NotErrorIs(t, err, ErrNotEnoughFundsForGas)
	require.NotErrorIs(t, err, ErrNotEnoughFundsForGasAndValue)
	require.ErrorIs(t, tr.Error(), cause)

	require.ErrorIs(t, tr.Write(tx), cause)
	require.Empty(t, tr.Receipts())
	require.Zero(t, tr.TotalGas())

	snapshot, root, err := tr.Commit()
	require.Nil(t, snapshot)
	require.Equal(t, types.ZeroHash, root)
	require.ErrorIs(t, err, cause)
	require.Zero(t, backend.snapshot.commits)
}

func TestTxAffordability_SimulationAndStateTransactions(t *testing.T) {
	t.Run("nonpayable_simulation_preserves_fee_check_bypass", func(t *testing.T) {
		_, tr, tx, sender := txAffordabilityFixture(
			t, types.DynamicFeeTx, big.NewInt(11), false, false,
		)
		msg := tx.Copy()
		msg.From = sender
		tr.SetNonPayable(true)

		result, err := tr.Apply(msg)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NoError(t, result.Err)
		require.NoError(t, tr.Error())
		require.Equal(t, uint64(21_000), result.GasUsed)
		require.Zero(t, tr.GetBalance(*msg.To).Cmp(big.NewInt(11)))
		require.Empty(t, tr.Receipts())
	})

	t.Run("system_transaction_retains_separate_validation", func(t *testing.T) {
		_, _, tr, _ := issue7AExecutionFixture(t, []byte{0x00})
		msg := &types.Transaction{
			Type:     types.StateTx,
			From:     contracts.SystemCaller,
			To:       issue7ATarget.Ptr(),
			Gas:      types.StateTransactionGasLimit,
			GasPrice: new(big.Int),
			Value:    new(big.Int),
		}

		require.Zero(t, tr.GetBalance(msg.From).Sign())
		result, err := tr.Apply(msg)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NoError(t, result.Err)
		require.NoError(t, tr.Error())
		require.Equal(t, uint64(21_000), result.GasUsed)
		require.Empty(t, tr.Receipts())
	})
}

/* FINISH ALPHA CHANGE 3043 -- verify upfront affordability without execution side effects -- */
