package innbcbft

import (
	"encoding/json"
	"fmt"

	/* START ALPHA CHANGE 1511 -- use MDBX for proposer snapshot storage -- */
	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	/* FINISH ALPHA CHANGE 1511 -- use MDBX for proposer snapshot storage -- */)

/*
MDBX schema:

proposerSnapshot:
|--> proposerSnapshotKey -> *ProposerSnapshot (JSON)
*/
var (
	// bucket to store proposer calculator snapshot
	proposerSnapshotBucket = []byte("proposerSnapshot")
	// proposerSnapshotKey is a static key which is used to save latest proposer snapshot.
	// (there will always be one object in bucket)
	proposerSnapshotKey = []byte("proposerSnapshotKey")
)

type ProposerSnapshotStore struct {
	/* START ALPHA CHANGE 1512 -- retain the proposer snapshot MDBX handle -- */
	state *State
	dbi   mdbxgo.DBI
	/* FINISH ALPHA CHANGE 1512 -- retain the proposer snapshot MDBX handle -- */
}

// initialize opens the proposer snapshot MDBX database.
func (s *ProposerSnapshotStore) initialize(tx *stateTx, flags uint) error {
	/* START ALPHA CHANGE 1513 -- open the proposer snapshot MDBX database -- */
	dbi, err := tx.tx.OpenDBISimple(string(proposerSnapshotBucket), flags)
	if err != nil {
		return fmt.Errorf("failed to open database=%s: %w", string(proposerSnapshotBucket), err)
	}

	s.dbi = dbi

	return nil
	/* FINISH ALPHA CHANGE 1513 -- open the proposer snapshot MDBX database -- */
}

// getProposerSnapshot gets the latest proposer snapshot.
func (s *ProposerSnapshotStore) getProposerSnapshot(dbTx *stateTx) (*ProposerSnapshot, error) {
	/* START ALPHA CHANGE 1514 -- read proposer snapshots through MDBX -- */
	var (
		snapshot *ProposerSnapshot
		err      error
	)

	getFn := func(tx *stateTx) error {
		value, err := tx.tx.Get(s.dbi, proposerSnapshotKey)
		if mdbxgo.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}

		return json.Unmarshal(value, &snapshot)
	}

	if dbTx == nil {
		err = s.state.view(getFn)
	} else {
		err = getFn(dbTx)
	}

	return snapshot, err
	/* FINISH ALPHA CHANGE 1514 -- read proposer snapshots through MDBX -- */
}

// writeProposerSnapshot writes the proposer snapshot.
func (s *ProposerSnapshotStore) writeProposerSnapshot(snapshot *ProposerSnapshot, dbTx *stateTx) error {
	/* START ALPHA CHANGE 1515 -- write proposer snapshots through MDBX -- */
	insertFn := func(tx *stateTx) error {
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}

		return tx.tx.Put(s.dbi, proposerSnapshotKey, raw, 0)
	}

	if dbTx == nil {
		return s.state.update(insertFn)
	}

	return insertFn(dbTx)
	/* FINISH ALPHA CHANGE 1515 -- write proposer snapshots through MDBX -- */
}
