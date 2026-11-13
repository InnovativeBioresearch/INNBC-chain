/* START ALPHA CHANGE 3044 -- verify sequential affordability and imported-block rejection with MDBX -- */
package itrie

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func signTxAffordability(
	t *testing.T,
	fixture *creationRollbackFixture,
	txType types.TxType,
	target types.Address,
	nonce uint64,
) *types.Transaction {
	t.Helper()

	tx := &types.Transaction{
		Type:     txType,
		Nonce:    nonce,
		To:       target.Ptr(),
		Gas:      30_000,
		GasPrice: big.NewInt(1),
		Value:    big.NewInt(15_000),
	}
	if txType == types.DynamicFeeTx {
		tx.ChainID = big.NewInt(100)
		tx.GasPrice = new(big.Int)
		tx.GasFeeCap = big.NewInt(10)
		tx.GasTipCap = big.NewInt(1)
	}

	signer := crypto.NewLondonSigner(
		100, true, crypto.NewEIP155Signer(100, true),
	)
	signed, err := signer.SignTx(tx, fixture.key)
	require.NoError(t, err)
	signed.ComputeHash(1)

	return signed
}

func TestTxAffordability_SequentialTransactionsAndPersistence(t *testing.T) {
	for _, txType := range []types.TxType{
		types.LegacyTx, types.DynamicFeeTx,
	} {
		name := "legacy"
		if txType == types.DynamicFeeTx {
			name = "dynamic"
		}

		t.Run(name, func(t *testing.T) {
			fixture := newCreationRollbackFixture(t)
			target := types.StringToAddress("0x9301")
			coinbase := types.StringToAddress("0x9302")

			first := signTxAffordability(t, fixture, txType, target, 0)
			second := signTxAffordability(t, fixture, txType, target, 1)

			required := new(big.Int).Set(second.Cost())
			actualPrice := int64(1)
			if txType == types.DynamicFeeTx {
				actualPrice = 2
			}
			firstActualCost := big.NewInt(21_000*actualPrice + 15_000)

			// Both transactions are individually affordable initially.
			// The first leaves exactly one less than the second requires.
			initialBalance := new(big.Int).Add(
				new(big.Int).Set(required), firstActualCost,
			)
			initialBalance.Sub(initialBalance, big.NewInt(1))
			require.LessOrEqual(t, first.Cost().Cmp(initialBalance), 0)
			require.LessOrEqual(t, second.Cost().Cmp(initialBalance), 0)

			root, err := fixture.executor.WriteGenesis(
				map[types.Address]*chain.GenesisAccount{
					fixture.sender: {
						Balance: new(big.Int).Set(initialBalance),
					},
				},
				types.ZeroHash,
			)
			require.NoError(t, err)

			header := &types.Header{
				Number:   1,
				GasLimit: 1_000_000,
				BaseFee:  1,
			}

			// The production block-execution entry point must reject the
			// second transaction and leave the persisted parent state intact.
			rejected, err := fixture.executor.ProcessBlock(
				root,
				&types.Block{
					Header: header.Copy(),
					Transactions: []*types.Transaction{
						first.Copy(), second.Copy(),
					},
				},
				coinbase,
			)
			require.Nil(t, rejected)
			var applicationErr *statepkg.TransitionApplicationError
			require.ErrorAs(t, err, &applicationErr)
			require.ErrorIs(
				t, applicationErr.Err, statepkg.ErrNotEnoughFundsForGasAndValue,
			)

			parentSnapshot, err := NewState(fixture.storage).NewSnapshotAt(root)
			require.NoError(t, err)
			parentSender, err := parentSnapshot.GetAccount(fixture.sender)
			require.NoError(t, err)
			require.NotNil(t, parentSender)
			require.Zero(t, parentSender.Nonce)
			require.Zero(t, parentSender.Balance.Cmp(initialBalance))
			parentTarget, err := parentSnapshot.GetAccount(target)
			require.NoError(t, err)
			require.Nil(t, parentTarget)

			// A block builder can retain the first successful transaction
			// while rejecting the now-unaffordable second transaction.
			tr, err := fixture.executor.BeginTxn(root, header.Copy(), coinbase)
			require.NoError(t, err)
			require.NoError(t, tr.Write(first.Copy()))
			require.Len(t, tr.Receipts(), 1)
			require.NotNil(t, tr.Receipts()[0].Status)
			require.Equal(t, types.ReceiptSuccess, *tr.Receipts()[0].Status)
			require.Equal(t, uint64(21_000), tr.TotalGas())

			remaining := new(big.Int).Sub(
				new(big.Int).Set(required), big.NewInt(1),
			)
			require.Zero(t, tr.GetBalance(fixture.sender).Cmp(remaining))

			// The remaining balance covers the maximum gas obligation.
			// The combined maximum gas plus value requirement fails.
			maximumGasCost := new(big.Int).Mul(
				new(big.Int).SetUint64(second.Gas), second.GetGasFeeCap(),
			)
			require.GreaterOrEqual(t, remaining.Cmp(maximumGasCost), 0)

			nonceBefore := tr.GetNonce(fixture.sender)
			balanceBefore := new(big.Int).Set(tr.GetBalance(fixture.sender))
			targetBefore := new(big.Int).Set(tr.GetBalance(target))
			totalGasBefore := tr.TotalGas()
			receiptBefore := tr.Receipts()[0]

			err = tr.Write(second.Copy())
			applicationErr = nil
			require.ErrorAs(t, err, &applicationErr)
			require.ErrorIs(
				t, applicationErr.Err, statepkg.ErrNotEnoughFundsForGasAndValue,
			)
			require.True(t, applicationErr.IsRecoverable)
			require.NoError(t, tr.Error())
			require.Equal(t, nonceBefore, tr.GetNonce(fixture.sender))
			require.Zero(t, balanceBefore.Cmp(tr.GetBalance(fixture.sender)))
			require.Zero(t, targetBefore.Cmp(tr.GetBalance(target)))
			require.Equal(t, totalGasBefore, tr.TotalGas())
			require.Len(t, tr.Receipts(), 1)
			require.Same(t, receiptBefore, tr.Receipts()[0])

			snapshot := fixture.reopen(t, tr)
			sender, err := snapshot.GetAccount(fixture.sender)
			require.NoError(t, err)
			require.NotNil(t, sender)
			require.Equal(t, uint64(1), sender.Nonce)
			require.Zero(t, sender.Balance.Cmp(remaining))

			recipient, err := snapshot.GetAccount(target)
			require.NoError(t, err)
			require.NotNil(t, recipient)
			require.Zero(t, recipient.Balance.Cmp(big.NewInt(15_000)))
		})
	}
}

/* FINISH ALPHA CHANGE 3044 -- verify sequential affordability and imported-block rejection with MDBX -- */
