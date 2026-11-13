/* START ALPHA CHANGE 3119 -- verify storage rollback and continued processing after memory expansion failure -- */
package state

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestMemoryExpansion_TransitionRollback(t *testing.T) {
	const (
		transactionGas = uint64(13_000_000_000)
		blockGas       = transactionGas + 21_000
		senderBalance  = int64(30_000_000_000)
	)

	// SSTORE(slot 0, value 1).
	prefix := []byte{0x60, 0x01, 0x60, 0x00, 0x55}

	// Then attempt MSTORE8 at offset 2^37-1.
	failingCode := append(append([]byte(nil), prefix...), []byte{
		0x60, 0xab,
		0x64, 0x1f, 0xff, 0xff, 0xff, 0xff,
		0x53,
		0x00,
	}...)

	fixture := func(t *testing.T, code []byte) (*Transition, *types.Transaction) {
		t.Helper()

		executor, _, _, transaction := issue7AExecutionFixture(t, code)
		tr, err := executor.BeginTxn(
			types.EmptyRootHash,
			&types.Header{
				Number:   1,
				GasLimit: blockGas,
				BaseFee:  1,
			},
			types.ZeroAddress,
		)
		require.NoError(t, err)

		tr.state.SetBalance(issue7ASender, big.NewInt(senderBalance))
		require.NoError(t, tr.Error())

		transaction.Gas = transactionGas
		transaction.ComputeHash(1)

		return tr, transaction
	}

	t.Run("successful_storage_control", func(t *testing.T) {
		code := append(append([]byte(nil), prefix...), 0x00)
		tr, transaction := fixture(t, code)
		transaction.Gas = 100_000
		transaction.ComputeHash(1)

		require.NoError(t, tr.Write(transaction))
		require.NoError(t, tr.Error())
		require.Equal(t, types.StringToHash("0x01"),
			tr.GetStorage(issue7ATarget, types.ZeroHash))
		require.Len(t, tr.Receipts(), 1)
		require.Equal(t, types.ReceiptSuccess, *tr.Receipts()[0].Status)
	})

	t.Run("apply_returns_ordinary_out_of_gas_and_rolls_back", func(t *testing.T) {
		tr, transaction := fixture(t, failingCode)
		result, err := tr.Apply(transaction.Copy())

		require.NoError(t, err)
		require.NotNil(t, result)
		require.ErrorIs(t, result.Err, runtime.ErrOutOfGas)
		require.Zero(t, result.GasLeft)
		require.Equal(t, transactionGas, result.GasUsed)
		require.NoError(t, tr.Error())

		require.Equal(t, types.ZeroHash,
			tr.GetStorage(issue7ATarget, types.ZeroHash))
		require.Equal(t, uint64(1), tr.GetNonce(issue7ASender))
		require.Zero(t, tr.GetBalance(issue7ASender).Cmp(
			big.NewInt(senderBalance-int64(transactionGas)),
		))
		require.Equal(t, uint64(21_000), tr.gasPool)
		require.Empty(t, tr.state.Logs())
		require.Zero(t, tr.state.GetRefund())
	})

	t.Run("failed_receipt_then_next_transaction_succeeds", func(t *testing.T) {
		tr, transaction := fixture(t, failingCode)

		require.NoError(t, tr.Write(transaction))
		require.NoError(t, tr.Error())
		require.Equal(t, types.ZeroHash,
			tr.GetStorage(issue7ATarget, types.ZeroHash))
		require.Equal(t, uint64(1), tr.GetNonce(issue7ASender))

		require.Len(t, tr.Receipts(), 1)
		failed := tr.Receipts()[0]
		require.NotNil(t, failed.Status)
		require.Equal(t, types.ReceiptFailed, *failed.Status)
		require.Equal(t, transactionGas, failed.GasUsed)
		require.Empty(t, failed.Logs)
		require.Equal(t, transactionGas, tr.TotalGas())
		require.Equal(t, uint64(21_000), tr.gasPool)

		recipient := types.StringToAddress("0x7003")
		next := &types.Transaction{
			Type:     types.LegacyTx,
			From:     issue7ASender,
			To:       recipient.Ptr(),
			Nonce:    1,
			Gas:      21_000,
			GasPrice: big.NewInt(1),
			Value:    big.NewInt(1),
		}
		next.ComputeHash(1)

		require.NoError(t, tr.Write(next))
		require.NoError(t, tr.Error())
		require.Len(t, tr.Receipts(), 2)
		require.Equal(t, types.ReceiptSuccess, *tr.Receipts()[1].Status)
		require.Equal(t, blockGas, tr.TotalGas())
		require.Zero(t, tr.gasPool)
		require.Equal(t, uint64(2), tr.GetNonce(issue7ASender))
		require.Zero(t, tr.GetBalance(recipient).Cmp(big.NewInt(1)))
		require.Zero(t, tr.GetBalance(issue7ASender).Cmp(
			big.NewInt(senderBalance-int64(blockGas)-1),
		))
		require.Equal(t, types.ZeroHash,
			tr.GetStorage(issue7ATarget, types.ZeroHash))
	})
}

/* FINISH ALPHA CHANGE 3119 -- verify storage rollback and continued processing after memory expansion failure -- */
