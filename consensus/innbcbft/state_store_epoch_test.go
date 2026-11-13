package innbcbft

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_insertAndGetValidatorSnapshot(t *testing.T) {
	t.Parallel()

	const (
		epoch            = uint64(1)
		epochEndingBlock = uint64(100)
	)

	state := newTestState(t)
	keys, err := bls.CreateRandomBlsKeys(3)

	require.NoError(t, err)

	snapshot := validator.AccountSet{
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x18}), BlsKey: keys[0].PublicKey()},
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x23}), BlsKey: keys[1].PublicKey()},
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x37}), BlsKey: keys[2].PublicKey()},
	}

	assert.NoError(t, state.EpochStore.insertValidatorSnapshot(
		&validatorSnapshot{epoch, epochEndingBlock, snapshot}, nil))

	snapshotFromDB, err := state.EpochStore.getValidatorSnapshot(epoch)

	assert.NoError(t, err)
	assert.Equal(t, snapshot.Len(), snapshotFromDB.Snapshot.Len())
	assert.Equal(t, epoch, snapshotFromDB.Epoch)
	assert.Equal(t, epochEndingBlock, snapshotFromDB.EpochEndingBlock)

	for i, v := range snapshot {
		assert.Equal(t, v.Address, snapshotFromDB.Snapshot[i].Address)
		assert.Equal(t, v.BlsKey, snapshotFromDB.Snapshot[i].BlsKey)
	}
}

func TestState_cleanValidatorSnapshotsFromDb(t *testing.T) {
	t.Parallel()

	fixedEpochSize := uint64(10)
	state := newTestState(t)
	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	snapshot := validator.AccountSet{
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x18}), BlsKey: keys[0].PublicKey()},
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x23}), BlsKey: keys[1].PublicKey()},
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x37}), BlsKey: keys[2].PublicKey()},
	}

	var epoch uint64
	// add a couple of more snapshots above limit just to make sure we reached it
	for i := 1; i <= validatorSnapshotLimit+2; i++ {
		epoch = uint64(i)
		assert.NoError(t, state.EpochStore.insertValidatorSnapshot(
			&validatorSnapshot{epoch, epoch * fixedEpochSize, snapshot}, nil))
	}

	snapshotFromDB, err := state.EpochStore.getValidatorSnapshot(epoch)

	assert.NoError(t, err)
	assert.Equal(t, snapshot.Len(), snapshotFromDB.Snapshot.Len())
	assert.Equal(t, epoch, snapshotFromDB.Epoch)
	assert.Equal(t, epoch*fixedEpochSize, snapshotFromDB.EpochEndingBlock)

	for i, v := range snapshot {
		assert.Equal(t, v.Address, snapshotFromDB.Snapshot[i].Address)
		assert.Equal(t, v.BlsKey, snapshotFromDB.Snapshot[i].BlsKey)
	}

	assert.NoError(t, state.EpochStore.cleanValidatorSnapshotsFromDB(epoch, nil))

	// test that last (numberOfSnapshotsToLeaveInDb) of snapshots are left in db after cleanup
	validatorSnapshotsBucketStats, err := state.EpochStore.validatorSnapshotsDBStats()
	require.NoError(t, err)

	/* START ALPHA CHANGE 1545 -- assert MDBX validator snapshot entry count -- */
	assert.Equal(t, uint64(numberOfSnapshotsToLeaveInDB), validatorSnapshotsBucketStats.Entries)
	/* FINISH ALPHA CHANGE 1545 -- assert MDBX validator snapshot entry count -- */

	for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ {
		snapshotFromDB, err = state.EpochStore.getValidatorSnapshot(epoch)
		assert.NoError(t, err)
		assert.NotNil(t, snapshotFromDB)
		epoch--
	}
}

/* START ALPHA CHANGE 379 -- remove obsolete state-sync vote concurrency test -- */
/* FINISH ALPHA CHANGE 379 -- remove obsolete state-sync vote concurrency test -- */

func TestState_Insert_And_Cleanup(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	/* START ALPHA CHANGE 380 -- remove state-sync vote hash from epoch cleanup test -- */
	/* FINISH ALPHA CHANGE 380 -- remove state-sync vote hash from epoch cleanup test -- */

	for i := uint64(1); i <= 500; i++ {
		epoch := i
		err := state.EpochStore.insertEpoch(epoch, nil)

		assert.NoError(t, err)

		/* START ALPHA CHANGE 381 -- remove state-sync vote write from epoch cleanup test -- */
		/* FINISH ALPHA CHANGE 381 -- remove state-sync vote write from epoch cleanup test -- */
	}

	stats, err := state.EpochStore.epochsDBStats()
	require.NoError(t, err)

	/* START ALPHA CHANGE 1546 -- assert compact MDBX epoch marker counts -- */
	assert.Equal(t, uint64(500), stats.Entries)

	assert.NoError(t, state.EpochStore.cleanEpochsFromDB(nil))

	stats, err = state.EpochStore.epochsDBStats()
	require.NoError(t, err)

	assert.Equal(t, uint64(0), stats.Entries)
	/* FINISH ALPHA CHANGE 1546 -- assert compact MDBX epoch marker counts -- */

	/* START ALPHA CHANGE 382 -- remove state-sync vote cleanup assertion from epoch cleanup test -- */
	/* FINISH ALPHA CHANGE 382 -- remove state-sync vote cleanup assertion from epoch cleanup test -- */

	for i := uint64(501); i <= 1000; i++ {
		epoch := i
		err := state.EpochStore.insertEpoch(epoch, nil)
		assert.NoError(t, err)

		/* START ALPHA CHANGE 383 -- remove second state-sync vote write from epoch cleanup test -- */
		/* FINISH ALPHA CHANGE 383 -- remove second state-sync vote write from epoch cleanup test -- */
	}

	stats, err = state.EpochStore.epochsDBStats()
	require.NoError(t, err)

	/* START ALPHA CHANGE 1547 -- assert repopulated MDBX epoch marker count -- */
	assert.Equal(t, uint64(500), stats.Entries)
	/* FINISH ALPHA CHANGE 1547 -- assert repopulated MDBX epoch marker count -- */

	/* START ALPHA CHANGE 384 -- remove final state-sync vote assertion from epoch cleanup test -- */
	/* FINISH ALPHA CHANGE 384 -- remove final state-sync vote assertion from epoch cleanup test -- */
}

func TestState_getLastSnapshot(t *testing.T) {
	t.Parallel()

	const (
		lastEpoch          = uint64(10)
		fixedEpochSize     = uint64(10)
		numberOfValidators = 3
	)

	state := newTestState(t)

	for i := uint64(1); i <= lastEpoch; i++ {
		keys, err := bls.CreateRandomBlsKeys(numberOfValidators)

		require.NoError(t, err)

		var snapshot validator.AccountSet
		for j := 0; j < numberOfValidators; j++ {
			snapshot = append(snapshot, &validator.ValidatorMetadata{Address: types.BytesToAddress(generateRandomBytes(t)), BlsKey: keys[j].PublicKey()})
		}

		require.NoError(t, state.EpochStore.insertValidatorSnapshot(
			&validatorSnapshot{i, i * fixedEpochSize, snapshot}, nil))
	}

	snapshotFromDB, err := state.EpochStore.getLastSnapshot(nil)

	assert.NoError(t, err)
	assert.Equal(t, numberOfValidators, snapshotFromDB.Snapshot.Len())
	assert.Equal(t, lastEpoch, snapshotFromDB.Epoch)
	assert.Equal(t, lastEpoch*fixedEpochSize, snapshotFromDB.EpochEndingBlock)
}

/* START ALPHA CHANGE 972 -- test validator snapshot lookup at or before requested epoch -- */
func TestState_getLastSnapshotAtOrBefore(t *testing.T) {
	t.Parallel()

	const fixedEpochSize = uint64(10)

	state := newTestState(t)
	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	snapshot := validator.AccountSet{
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x18}), BlsKey: keys[0].PublicKey()},
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x23}), BlsKey: keys[1].PublicKey()},
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x37}), BlsKey: keys[2].PublicKey()},
	}

	require.NoError(t, state.EpochStore.insertValidatorSnapshot(&validatorSnapshot{2, 2 * fixedEpochSize, snapshot}, nil))
	require.NoError(t, state.EpochStore.insertValidatorSnapshot(&validatorSnapshot{4, 4 * fixedEpochSize, snapshot}, nil))
	require.NoError(t, state.EpochStore.insertValidatorSnapshot(&validatorSnapshot{7, 7 * fixedEpochSize, snapshot}, nil))

	snapshotFromDB, err := state.EpochStore.getLastSnapshotAtOrBefore(6, nil)
	require.NoError(t, err)
	require.NotNil(t, snapshotFromDB)
	require.Equal(t, uint64(4), snapshotFromDB.Epoch)

	snapshotFromDB, err = state.EpochStore.getLastSnapshotAtOrBefore(7, nil)
	require.NoError(t, err)
	require.NotNil(t, snapshotFromDB)
	require.Equal(t, uint64(7), snapshotFromDB.Epoch)

	snapshotFromDB, err = state.EpochStore.getLastSnapshotAtOrBefore(1, nil)
	require.NoError(t, err)
	require.Nil(t, snapshotFromDB)
}

/* FINISH ALPHA CHANGE 972 -- test validator snapshot lookup at or before requested epoch -- */
