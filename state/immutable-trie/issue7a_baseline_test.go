/* START ALPHA CHANGE 2120 -- pin legitimate empty and absent state behavior for Issue 7A -- */
package itrie

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
)

func TestIssue7A_LegitimateAbsenceBaseline(t *testing.T) {
	storage := NewMemoryStorage()
	worldState := NewState(storage)
	snapshot := worldState.NewSnapshot()

	address := types.StringToAddress("0x1001")
	missingAddress := types.StringToAddress("0x1002")
	slot := types.StringToHash("0x01")

	account, err := snapshot.GetAccount(address)
	require.NoError(t, err)
	require.Nil(t, account)

	/* START ALPHA CHANGE 2153 -- check legitimate empty storage and code reads -- */
	checkedStorage1, storageErr1 := snapshot.GetStorage(address, emptyStateHash, slot)
	require.NoError(t, storageErr1)
	require.Equal(
		t,
		types.ZeroHash,
		checkedStorage1,
	)

	code, found, codeErr := snapshot.GetCode(types.EmptyCodeHash)
	require.NoError(t, codeErr)
	require.True(t, found)
	require.Empty(t, code)
	/* FINISH ALPHA CHANGE 2153 -- check legitimate empty storage and code reads -- */

	txn := state.NewTxn(snapshot)
	txn.CreateAccount(address)

	objects, err := txn.Commit(false)
	require.NoError(t, err)

	committed, root, err := snapshot.Commit(objects)
	require.NoError(t, err)
	require.NotEqual(t, types.EmptyRootHash, types.BytesToHash(root))

	account, err = committed.GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, uint64(0), account.Nonce)
	require.Zero(t, account.Balance.Cmp(big.NewInt(0)))
	require.Equal(t, emptyStateHash, account.Root)
	require.Equal(t, types.EmptyCodeHash.Bytes(), account.CodeHash)

	missing, err := committed.GetAccount(missingAddress)
	require.NoError(t, err)
	require.Nil(t, missing)

	/* START ALPHA CHANGE 2153 -- check committed absent storage slot -- */
	checkedStorage2, storageErr2 := committed.GetStorage(address, account.Root, slot)
	require.NoError(t, storageErr2)
	require.Equal(
		t,
		types.ZeroHash,
		checkedStorage2,
	)
	/* FINISH ALPHA CHANGE 2153 -- check committed absent storage slot -- */
}

func TestIssue7A_HealthyRollbackBaseline(t *testing.T) {
	storage := NewMemoryStorage()
	worldState := NewState(storage)
	snapshot := worldState.NewSnapshot()

	address := types.StringToAddress("0x2001")
	slot := types.StringToHash("0x01")
	original := types.StringToHash("0x2a")
	reverted := types.StringToHash("0x2b")

	txn := state.NewTxn(snapshot)
	txn.SetState(address, slot, original)

	checkpoint := txn.Snapshot()
	txn.SetState(address, slot, reverted)
	require.Equal(t, reverted, txn.GetState(address, slot))

	require.NoError(t, txn.RevertToSnapshot(checkpoint))
	require.Equal(t, original, txn.GetState(address, slot))

	objects, err := txn.Commit(false)
	require.NoError(t, err)

	committed, _, err := snapshot.Commit(objects)
	require.NoError(t, err)

	account, err := committed.GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, account)
	/* START ALPHA CHANGE 2153 -- check healthy rollback storage read -- */
	checkedStorage1, storageErr1 := committed.GetStorage(address, account.Root, slot)
	require.NoError(t, storageErr1)
	require.Equal(
		t,
		original,
		checkedStorage1,
	)
	/* FINISH ALPHA CHANGE 2153 -- check healthy rollback storage read -- */
}

/* START ALPHA CHANGE 2124 -- distinguish an absent slot from an empty storage trie -- */
func TestIssue7A_AbsentSlotInPersistedNonemptyStorageIsValid(t *testing.T) {
	storage := NewMemoryStorage()
	worldState := NewState(storage)
	snapshot := worldState.NewSnapshot()

	address := types.StringToAddress("0x3001")
	missingAddress := types.StringToAddress("0x3002")
	presentSlot := types.StringToHash("0x01")
	missingSlot := types.StringToHash("0x02")
	presentValue := types.StringToHash("0x2a")

	txn := state.NewTxn(snapshot)
	txn.SetState(address, presentSlot, presentValue)

	objects, err := txn.Commit(false)
	require.NoError(t, err)

	_, root, err := snapshot.Commit(objects)
	require.NoError(t, err)

	// Reopen through a fresh State so reads cannot rely on the previous
	// State's decoded-trie cache.
	reopened, err := NewState(storage).NewSnapshotAt(types.BytesToHash(root))
	require.NoError(t, err)

	account, err := reopened.GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, account)
	require.NotEqual(t, emptyStateHash, account.Root)

	/* START ALPHA CHANGE 2153 -- distinguish successful persisted reads from read failures -- */
	checkedStorage1, storageErr1 := reopened.GetStorage(address, account.Root, presentSlot)
	require.NoError(t, storageErr1)
	require.Equal(
		t,
		presentValue,
		checkedStorage1,
	)

	checkedStorage2, storageErr2 := reopened.GetStorage(address, account.Root, missingSlot)
	require.NoError(t, storageErr2)
	require.Equal(
		t,
		types.ZeroHash,
		checkedStorage2,
	)
	/* FINISH ALPHA CHANGE 2153 -- distinguish successful persisted reads from read failures -- */

	missing, err := reopened.GetAccount(missingAddress)
	require.NoError(t, err)
	require.Nil(t, missing)
}

/* FINISH ALPHA CHANGE 2124 -- distinguish an absent slot from an empty storage trie -- */

/* FINISH ALPHA CHANGE 2120 -- pin legitimate empty and absent state behavior for Issue 7A -- */
