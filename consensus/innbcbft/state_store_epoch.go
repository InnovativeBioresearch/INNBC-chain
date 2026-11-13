package innbcbft

import (
	"bytes"
	"encoding/json"
	"fmt"

	/* START ALPHA CHANGE 1499 -- use MDBX for epoch and validator snapshot storage -- */
	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	/* FINISH ALPHA CHANGE 1499 -- use MDBX for epoch and validator snapshot storage -- */
	"github.com/innovativebioresearch/innbc-chain/helper/common"
)

const (
	// validatorSnapshotLimit defines a maximum number of validator snapshots
	// that can be stored in cache (both memory and db)
	validatorSnapshotLimit = 100
	// numberOfSnapshotsToLeaveInMemory defines a number of validator snapshots to leave in memory
	numberOfSnapshotsToLeaveInMemory = 12
	// numberOfSnapshotsToLeaveInDB defines a number of validator snapshots to leave in db
	numberOfSnapshotsToLeaveInDB = 20
)

var (
	// bucket to store epochs and all its nested buckets (message votes and message pool events)
	epochsBucket = []byte("epochs")

	// bucket to store validator snapshots
	validatorSnapshotsBucket = []byte("validatorSnapshots")
)

/*
MDBX schema:

epochs:
|--> epochNumber -> marker

validatorSnapshots:
|--> epochNumber -> *validatorSnapshot (JSON)
*/

type EpochStore struct {
	/* START ALPHA CHANGE 1500 -- retain MDBX epoch database handles -- */
	state                 *State
	epochsDBI             mdbxgo.DBI
	validatorSnapshotsDBI mdbxgo.DBI
	/* FINISH ALPHA CHANGE 1500 -- retain MDBX epoch database handles -- */
}

// initialize opens the required MDBX named databases.
func (s *EpochStore) initialize(tx *stateTx, flags uint) error {
	/* START ALPHA CHANGE 1501 -- open MDBX epoch and validator snapshot databases -- */
	epochsDBI, err := tx.tx.OpenDBISimple(string(epochsBucket), flags)
	if err != nil {
		return fmt.Errorf("failed to open database=%s: %w", string(epochsBucket), err)
	}

	validatorSnapshotsDBI, err := tx.tx.OpenDBISimple(string(validatorSnapshotsBucket), flags)
	if err != nil {
		return fmt.Errorf("failed to open database=%s: %w", string(validatorSnapshotsBucket), err)
	}

	s.epochsDBI = epochsDBI
	s.validatorSnapshotsDBI = validatorSnapshotsDBI

	return nil
	/* FINISH ALPHA CHANGE 1501 -- open MDBX epoch and validator snapshot databases -- */
}

// insertValidatorSnapshot inserts a validator snapshot for the given block to its database.
func (s *EpochStore) insertValidatorSnapshot(validatorSnapshot *validatorSnapshot, dbTx *stateTx) error {
	/* START ALPHA CHANGE 1502 -- write validator snapshots through MDBX transactions -- */
	insertFn := func(tx *stateTx) error {
		raw, err := json.Marshal(validatorSnapshot)
		if err != nil {
			return err
		}

		return tx.tx.Put(
			s.validatorSnapshotsDBI,
			common.EncodeUint64ToBytes(validatorSnapshot.Epoch),
			raw,
			0,
		)
	}

	if dbTx == nil {
		return s.state.update(insertFn)
	}

	return insertFn(dbTx)
	/* FINISH ALPHA CHANGE 1502 -- write validator snapshots through MDBX transactions -- */
}

// getValidatorSnapshot queries the validator snapshot for the given epoch.
func (s *EpochStore) getValidatorSnapshot(epoch uint64) (*validatorSnapshot, error) {
	/* START ALPHA CHANGE 1503 -- read validator snapshots through MDBX transactions -- */
	var snapshot *validatorSnapshot

	err := s.state.view(func(tx *stateTx) error {
		raw, err := tx.tx.Get(s.validatorSnapshotsDBI, common.EncodeUint64ToBytes(epoch))
		if mdbxgo.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}

		return json.Unmarshal(raw, &snapshot)
	})

	return snapshot, err
	/* FINISH ALPHA CHANGE 1503 -- read validator snapshots through MDBX transactions -- */
}

// getLastSnapshot returns the last snapshot saved in db
// since they are stored by epoch number (uint64), they are sequentially stored,
// so the latest epoch will be the last snapshot in db
func (s *EpochStore) getLastSnapshot(dbTx *stateTx) (*validatorSnapshot, error) {
	/* START ALPHA CHANGE 1504 -- retrieve the latest validator snapshot with an MDBX cursor -- */
	var (
		snapshot *validatorSnapshot
		err      error
	)

	getFn := func(tx *stateTx) error {
		cursor, err := tx.tx.OpenCursor(s.validatorSnapshotsDBI)
		if err != nil {
			return err
		}
		defer cursor.Close()

		_, raw, err := cursor.Get(nil, nil, mdbxgo.Last)
		if mdbxgo.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}

		return json.Unmarshal(raw, &snapshot)
	}

	if dbTx == nil {
		err = s.state.view(getFn)
	} else {
		err = getFn(dbTx)
	}

	return snapshot, err
	/* FINISH ALPHA CHANGE 1504 -- retrieve the latest validator snapshot with an MDBX cursor -- */
}

/* START ALPHA CHANGE 970 -- prevent validator snapshot lookup from returning future epochs -- */
func (s *EpochStore) getLastSnapshotAtOrBefore(epoch uint64, dbTx *stateTx) (*validatorSnapshot, error) {
	var (
		snapshot *validatorSnapshot
		err      error
	)

	targetKey := common.EncodeUint64ToBytes(epoch)

	/* START ALPHA CHANGE 1505 -- seek bounded validator snapshots through MDBX -- */
	getFn := func(tx *stateTx) error {
		cursor, err := tx.tx.OpenCursor(s.validatorSnapshotsDBI)
		if err != nil {
			return err
		}
		defer cursor.Close()

		key, raw, err := cursor.Get(targetKey, nil, mdbxgo.SetRange)
		if mdbxgo.IsNotFound(err) {
			key, raw, err = cursor.Get(nil, nil, mdbxgo.Last)
		}
		if mdbxgo.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}

		if bytes.Compare(key, targetKey) > 0 {
			_, raw, err = cursor.Get(nil, nil, mdbxgo.Prev)
			if mdbxgo.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
		}

		return json.Unmarshal(raw, &snapshot)
	}

	if dbTx == nil {
		err = s.state.view(getFn)
	} else {
		err = getFn(dbTx)
	}
	/* FINISH ALPHA CHANGE 1505 -- seek bounded validator snapshots through MDBX -- */

	return snapshot, err
}

/* FINISH ALPHA CHANGE 970 -- prevent validator snapshot lookup from returning future epochs -- */

// insertEpoch inserts a new epoch marker into the database.
func (s *EpochStore) insertEpoch(epoch uint64, dbTx *stateTx) error {
	/* START ALPHA CHANGE 1506 -- represent initialized epochs as compact MDBX markers -- */
	insertFn := func(tx *stateTx) error {
		return tx.tx.Put(
			s.epochsDBI,
			common.EncodeUint64ToBytes(epoch),
			[]byte{1},
			0,
		)
	}

	if dbTx == nil {
		return s.state.update(insertFn)
	}

	return insertFn(dbTx)
	/* FINISH ALPHA CHANGE 1506 -- represent initialized epochs as compact MDBX markers -- */
}

// isEpochInserted checks if the given epoch is present in the database.
func (s *EpochStore) isEpochInserted(epoch uint64) bool {
	/* START ALPHA CHANGE 1507 -- check MDBX epoch markers directly -- */
	found := false

	err := s.state.view(func(tx *stateTx) error {
		_, err := tx.tx.Get(s.epochsDBI, common.EncodeUint64ToBytes(epoch))
		if mdbxgo.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}

		found = true

		return nil
	})

	return err == nil && found
	/* FINISH ALPHA CHANGE 1507 -- check MDBX epoch markers directly -- */
}

// cleanEpochsFromDB removes all epoch markers.
func (s *EpochStore) cleanEpochsFromDB(dbTx *stateTx) error {
	/* START ALPHA CHANGE 1508 -- empty the MDBX epoch database without replacing its handle -- */
	cleanFn := func(tx *stateTx) error {
		return tx.tx.Drop(s.epochsDBI, false)
	}

	if dbTx == nil {
		return s.state.update(cleanFn)
	}

	return cleanFn(dbTx)
	/* FINISH ALPHA CHANGE 1508 -- empty the MDBX epoch database without replacing its handle -- */
}

// cleanValidatorSnapshotsFromDB cleans validator snapshots if a limit is reached,
// while retaining the latest configured number of actual records.
func (s *EpochStore) cleanValidatorSnapshotsFromDB(_ uint64, dbTx *stateTx) error {
	/* START ALPHA CHANGE 1509 -- retain the latest validator snapshots with an MDBX cursor -- */
	cleanFn := func(tx *stateTx) error {
		cursor, err := tx.tx.OpenCursor(s.validatorSnapshotsDBI)
		if err != nil {
			return err
		}

		keys := make([][]byte, 0, numberOfSnapshotsToLeaveInDB)
		values := make([][]byte, 0, numberOfSnapshotsToLeaveInDB)
		op := uint(mdbxgo.Last)

		for len(keys) < numberOfSnapshotsToLeaveInDB {
			key, value, err := cursor.Get(nil, nil, op)
			if mdbxgo.IsNotFound(err) {
				break
			}
			if err != nil {
				cursor.Close()

				return err
			}

			keys = append(keys, append([]byte(nil), key...))
			values = append(values, append([]byte(nil), value...))
			op = mdbxgo.Prev
		}

		cursor.Close()

		if err := tx.tx.Drop(s.validatorSnapshotsDBI, false); err != nil {
			return err
		}

		for i := len(keys) - 1; i >= 0; i-- {
			if err := tx.tx.Put(s.validatorSnapshotsDBI, keys[i], values[i], 0); err != nil {
				return err
			}
		}

		return nil
	}

	if dbTx == nil {
		return s.state.update(cleanFn)
	}

	return cleanFn(dbTx)
	/* FINISH ALPHA CHANGE 1509 -- retain the latest validator snapshots with an MDBX cursor -- */
}

// removeAllValidatorSnapshots empties the validator snapshot database.
func (s *EpochStore) removeAllValidatorSnapshots() error {
	/* START ALPHA CHANGE 1510 -- empty validator snapshots through MDBX -- */
	return s.state.update(func(tx *stateTx) error {
		return tx.tx.Drop(s.validatorSnapshotsDBI, false)
	})
	/* FINISH ALPHA CHANGE 1510 -- empty validator snapshots through MDBX -- */
}

// epochsDBStats returns statistics for the epoch database.
func (s *EpochStore) epochsDBStats() (*mdbxgo.Stat, error) {
	return s.state.dbStats(s.epochsDBI)
}

// validatorSnapshotsDBStats returns statistics for the validator snapshot database.
func (s *EpochStore) validatorSnapshotsDBStats() (*mdbxgo.Stat, error) {
	return s.state.dbStats(s.validatorSnapshotsDBI)
}

/* START ALPHA CHANGE 410 -- remove unused nested epoch bucket helper -- */
/* FINISH ALPHA CHANGE 410 -- remove unused nested epoch bucket helper -- */
