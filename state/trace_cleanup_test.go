/* START ALPHA CHANGE 3155 -- verify shared cleanup resets refunds accounts and transaction logs -- */
package state

import (
	"errors"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestTraceCleanup_FinalizeTransaction(t *testing.T) {
	txn, source := newOriginalStorageTxn(1)
	transition := NewTransition(chain.AllForksEnabled.At(1), nil, txn)

	txn.AddRefund(15_000)
	txn.EmitLog(source.address, []types.Hash{types.ZeroHash}, []byte{0x2a})
	require.True(t, txn.Suicide(source.address))

	logs, err := transition.FinalizeTransaction()
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, source.address, logs[0].Address)
	require.Equal(t, []byte{0x2a}, logs[0].Data)
	require.Zero(t, transition.GetRefund())
	require.False(t, transition.AccountExists(source.address))
	require.Empty(t, txn.Logs())
	require.Empty(t, transition.Receipts())
	require.Zero(t, transition.TotalGas())

	// A second boundary must not return the previous transaction's logs.
	logs, err = transition.FinalizeTransaction()
	require.NoError(t, err)
	require.Empty(t, logs)
	require.Zero(t, transition.GetRefund())
	require.NoError(t, transition.Error())
}

func TestTraceCleanup_RetainedFailureStopsFinalization(t *testing.T) {
	txn, source := newOriginalStorageTxn(1)
	transition := NewTransition(chain.AllForksEnabled.At(1), nil, txn)

	txn.AddRefund(15_000)
	txn.EmitLog(source.address, nil, []byte{0x2a})
	cause := errors.New("injected failure before transaction cleanup")
	txn.setError(cause)

	logs, err := transition.FinalizeTransaction()
	require.Nil(t, logs)
	require.ErrorIs(t, err, ErrLocalStateFailure)
	require.ErrorIs(t, err, cause)

	// A poisoned transition must stop rather than consume its pending data.
	require.Equal(t, uint64(15_000), txn.GetRefund())
	_, exists := txn.txn.Get(logIndex)
	require.True(t, exists)
	require.Empty(t, transition.Receipts())
	require.Zero(t, transition.TotalGas())
}

/* FINISH ALPHA CHANGE 3155 -- verify shared cleanup resets refunds accounts and transaction logs -- */
