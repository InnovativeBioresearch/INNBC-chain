/* START ALPHA CHANGE 3151 -- verify transaction-start storage views resets overrides and rollback -- */
package state

import (
	"errors"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type originalStorageSnapshot struct {
	address    types.Address
	root       types.Hash
	initial    types.Hash
	storageErr error
	reads      int
}

func (s *originalStorageSnapshot) GetAccount(addr types.Address) (*Account, error) {
	if addr != s.address {
		return nil, nil
	}

	return &Account{
		Nonce:    1,
		Balance:  big.NewInt(1),
		Root:     s.root,
		CodeHash: types.EmptyCodeHash.Bytes(),
	}, nil
}

func (s *originalStorageSnapshot) GetStorage(
	addr types.Address,
	root types.Hash,
	key types.Hash,
) (types.Hash, error) {
	if root == emptyStateHash {
		return types.ZeroHash, nil
	}

	s.reads++
	if s.storageErr != nil {
		return types.ZeroHash, s.storageErr
	}
	if addr == s.address && root == s.root && key == types.ZeroHash {
		return s.initial, nil
	}

	return types.ZeroHash, nil
}

func (s *originalStorageSnapshot) GetCode(hash types.Hash) ([]byte, bool, error) {
	if hash == types.EmptyCodeHash {
		return []byte{}, true, nil
	}

	return nil, false, nil
}

func newOriginalStorageTxn(initial uint64) (*Txn, *originalStorageSnapshot) {
	source := &originalStorageSnapshot{
		address: types.StringToAddress("0x8101"),
		root:    types.StringToHash("0xa001"),
		initial: types.BytesToHash(new(big.Int).SetUint64(initial).Bytes()),
	}

	return newTxn(source), source
}

// Unit tests establish the same view that Apply obtains from its existing
// rollback snapshot. Execution tests separately exercise the real entry point.
func originalStorageBoundary(txn *Txn) func() {
	id := txn.Snapshot()
	previous := txn.originalState
	txn.originalState = txn.snapshots[id]

	return func() {
		txn.originalState = previous
	}
}

func TestOriginalStorage_TransactionBoundaries(t *testing.T) {
	config := chain.AllForksEnabled.At(1)
	tests := []struct {
		name        string
		initial     uint64
		first       uint64
		firstState  runtime.StorageStatus
		secondState runtime.StorageStatus
	}{
		{
			name:        "new_then_modify",
			initial:     0,
			first:       1,
			firstState:  runtime.StorageAdded,
			secondState: runtime.StorageModified,
		},
		{
			name:        "clear_then_add",
			initial:     1,
			first:       0,
			firstState:  runtime.StorageDeleted,
			secondState: runtime.StorageAdded,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			txn, source := newOriginalStorageTxn(tc.initial)
			first := types.BytesToHash(new(big.Int).SetUint64(tc.first).Bytes())

			finish := originalStorageBoundary(txn)
			require.Equal(t, tc.firstState,
				txn.SetStorage(source.address, types.ZeroHash, first, &config))
			finish()

			require.NoError(t, txn.CleanDeleteObjects(true))
			require.Zero(t, txn.GetRefund())

			finish = originalStorageBoundary(txn)
			defer finish()

			before := source.reads
			require.Equal(t, first,
				txn.GetCommittedState(source.address, types.ZeroHash))
			require.Equal(t, before, source.reads)

			require.Equal(t, tc.secondState, txn.SetStorage(
				source.address, types.ZeroHash, types.StringToHash("0x02"), &config,
			))
			require.Zero(t, txn.GetRefund())
			require.NoError(t, txn.Error())
		})
	}
}

func TestOriginalStorage_NestedRollback(t *testing.T) {
	txn, source := newOriginalStorageTxn(1)
	config := chain.AllForksEnabled.At(1)
	finish := originalStorageBoundary(txn)
	defer finish()
	original := txn.originalState

	require.Equal(t, runtime.StorageDeleted, txn.SetStorage(
		source.address, types.ZeroHash, types.ZeroHash, &config,
	))
	require.Equal(t, uint64(15_000), txn.GetRefund())

	child := txn.Snapshot()
	require.Equal(t, runtime.StorageModifiedAgain, txn.SetStorage(
		source.address, types.ZeroHash, types.StringToHash("0x02"), &config,
	))
	require.Zero(t, txn.GetRefund())

	require.NoError(t, txn.RevertToSnapshot(child))
	require.Same(t, original, txn.originalState)
	require.Equal(t, types.ZeroHash,
		txn.GetState(source.address, types.ZeroHash))
	require.Equal(t, uint64(15_000), txn.GetRefund())
	require.Equal(t, source.initial,
		txn.GetCommittedState(source.address, types.ZeroHash))

	require.Equal(t, runtime.StorageModifiedAgain, txn.SetStorage(
		source.address, types.ZeroHash, source.initial, &config,
	))
	require.Equal(t, uint64(4_200), txn.GetRefund())
	require.NoError(t, txn.Error())
}

func TestOriginalStorage_AccountResetAndDeletion(t *testing.T) {
	config := chain.AllForksEnabled.At(1)

	t.Run("creation_reset_copy_and_rollback", func(t *testing.T) {
		txn, source := newOriginalStorageTxn(1)
		finish := originalStorageBoundary(txn)
		defer finish()

		rollback := txn.Snapshot()
		txn.CreateAccount(source.address)
		// Exercise StateObject.Copy after the reset.
		txn.SetBalance(source.address, big.NewInt(2))

		require.Equal(t, types.ZeroHash,
			txn.GetCommittedState(source.address, types.ZeroHash))
		require.Equal(t, runtime.StorageAdded, txn.SetStorage(
			source.address, types.ZeroHash, types.StringToHash("0x02"), &config,
		))
		require.Zero(t, txn.GetRefund())

		require.NoError(t, txn.RevertToSnapshot(rollback))
		require.Equal(t, source.initial,
			txn.GetCommittedState(source.address, types.ZeroHash))
		require.Equal(t, runtime.StorageModified, txn.SetStorage(
			source.address, types.ZeroHash, types.StringToHash("0x02"), &config,
		))
		require.NoError(t, txn.Error())
	})

	t.Run("creation_from_previous_transaction", func(t *testing.T) {
		txn, source := newOriginalStorageTxn(1)
		finish := originalStorageBoundary(txn)
		txn.CreateAccount(source.address)
		txn.SetState(source.address, types.ZeroHash, types.StringToHash("0x02"))
		finish()

		require.NoError(t, txn.CleanDeleteObjects(true))
		finish = originalStorageBoundary(txn)
		defer finish()

		require.Equal(t, types.StringToHash("0x02"),
			txn.GetCommittedState(source.address, types.ZeroHash))
		require.Equal(t, runtime.StorageModified, txn.SetStorage(
			source.address, types.ZeroHash, types.StringToHash("0x03"), &config,
		))
		require.NoError(t, txn.Error())
	})

	t.Run("deleted_account_does_not_restore_persistent_storage", func(t *testing.T) {
		txn, source := newOriginalStorageTxn(1)
		require.True(t, txn.Suicide(source.address))
		require.NoError(t, txn.CleanDeleteObjects(true))

		finish := originalStorageBoundary(txn)
		defer finish()
		txn.AddBalance(source.address, big.NewInt(1))

		require.Equal(t, types.ZeroHash,
			txn.GetCommittedState(source.address, types.ZeroHash))
		require.Equal(t, runtime.StorageAdded, txn.SetStorage(
			source.address, types.ZeroHash, types.StringToHash("0x02"), &config,
		))
		require.NoError(t, txn.Error())
	})
}

func TestOriginalStorage_OverridesAndDirectWrites(t *testing.T) {
	t.Run("state_diff_and_later_direct_write", func(t *testing.T) {
		txn, source := newOriginalStorageTxn(1)
		txn.SetState(source.address, types.ZeroHash, types.StringToHash("0x02"))

		finish := originalStorageBoundary(txn)
		defer finish()
		before := source.reads

		txn.SetState(source.address, types.ZeroHash, types.StringToHash("0x03"))
		require.Equal(t, types.StringToHash("0x02"),
			txn.GetCommittedState(source.address, types.ZeroHash))
		require.Equal(t, before, source.reads)
		require.NoError(t, txn.Error())
	})

	t.Run("full_storage_override_hides_omitted_persistent_slot", func(t *testing.T) {
		txn, source := newOriginalStorageTxn(1)
		other := types.StringToHash("0x02")
		txn.SetFullStorage(source.address, map[types.Hash]types.Hash{
			other: types.StringToHash("0x03"),
		})

		finish := originalStorageBoundary(txn)
		defer finish()
		before := source.reads

		require.Equal(t, types.ZeroHash,
			txn.GetCommittedState(source.address, types.ZeroHash))
		require.Equal(t, types.StringToHash("0x03"),
			txn.GetCommittedState(source.address, other))
		require.Equal(t, before, source.reads)
		require.NoError(t, txn.Error())
	})

	t.Run("direct_setup_without_apply_keeps_persistent_original", func(t *testing.T) {
		txn, source := newOriginalStorageTxn(1)
		txn.SetState(source.address, types.ZeroHash, types.StringToHash("0x02"))

		require.Nil(t, txn.originalState)
		require.Equal(t, source.initial,
			txn.GetCommittedState(source.address, types.ZeroHash))
		require.NoError(t, txn.Error())
	})
}

func TestOriginalStorage_ReadFailureSurvivesRollback(t *testing.T) {
	txn, source := newOriginalStorageTxn(1)
	finish := originalStorageBoundary(txn)
	defer finish()

	cause := errors.New("original storage unavailable")
	source.storageErr = cause

	require.Equal(t, types.ZeroHash,
		txn.GetCommittedState(source.address, types.ZeroHash))
	require.ErrorIs(t, txn.Error(), ErrLocalStateFailure)
	require.ErrorIs(t, txn.Error(), cause)

	require.NoError(t, txn.RevertToSnapshot(0))
	require.ErrorIs(t, txn.Error(), cause)
}

func TestOriginalStorage_ApplyLifecycle(t *testing.T) {
	for _, name := range []string{"success", "invalid_nonce", "local_failure"} {
		t.Run(name, func(t *testing.T) {
			code := []byte{0x60, 0x01, 0x60, 0x00, 0x55, 0x00}
			_, backend, tr, tx := issue7AExecutionFixture(t, code)
			before := len(tr.state.snapshots)
			hookObserved := false

			tr.PostHook = func(current *Transition) {
				hookObserved = true
				require.NotNil(t, current.state.originalState)
				require.Same(t, current.state.snapshots[before],
					current.state.originalState)
			}

			cause := errors.New("storage read failed")
			switch name {
			case "invalid_nonce":
				tx.Nonce = 1
			case "local_failure":
				backend.snapshot.storageErr = cause
			}

			result, err := tr.Apply(tx.Copy())
			require.Nil(t, tr.state.originalState)
			require.Zero(t, backend.snapshot.commits)

			switch name {
			case "success":
				require.NoError(t, err)
				require.NotNil(t, result)
				require.NoError(t, result.Err)
				require.True(t, hookObserved)
				// One existing Apply snapshot and one existing call snapshot.
				require.Len(t, tr.state.snapshots, before+2)
			case "invalid_nonce":
				require.Error(t, err)
				require.NoError(t, tr.Error())
				require.True(t, hookObserved)
				require.Len(t, tr.state.snapshots, before+1)
			case "local_failure":
				require.ErrorIs(t, err, ErrLocalStateFailure)
				require.ErrorIs(t, err, cause)
				require.False(t, hookObserved)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3151 -- verify transaction-start storage views resets overrides and rollback -- */
