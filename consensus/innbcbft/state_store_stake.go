package innbcbft

import (
	"errors"
	"fmt"

	/* START ALPHA CHANGE 1516 -- use MDBX for the temporarily retained stake store -- */
	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	/* FINISH ALPHA CHANGE 1516 -- use MDBX for the temporarily retained stake store -- */)

var (
	// bucket to store full validator set
	validatorSetBucket = []byte("fullValidatorSetBucket")
	// key of the full validator set in bucket
	fullValidatorSetKey = []byte("fullValidatorSet")
	// error returned if full validator set does not exists in db
	errNoFullValidatorSet = errors.New("full validator set not in db")
)

type StakeStore struct {
	/* START ALPHA CHANGE 1517 -- retain the validator-set MDBX handle -- */
	state *State
	dbi   mdbxgo.DBI
	/* FINISH ALPHA CHANGE 1517 -- retain the validator-set MDBX handle -- */
}

// initialize opens the validator-set MDBX database.
func (s *StakeStore) initialize(tx *stateTx, flags uint) error {
	/* START ALPHA CHANGE 1518 -- open the validator-set MDBX database -- */
	dbi, err := tx.tx.OpenDBISimple(string(validatorSetBucket), flags)
	if err != nil {
		return fmt.Errorf("failed to open database=%s: %w", string(validatorSetBucket), err)
	}

	s.dbi = dbi

	return nil
	/* FINISH ALPHA CHANGE 1518 -- open the validator-set MDBX database -- */
}

// insertFullValidatorSet inserts full validator set to its bucket (or updates it if exists)
// If the passed tx is already open (not nil), it will use it to insert full validator set
// If the passed tx is not open (it is nil), it will open a new transaction on db and insert full validator set
func (s *StakeStore) insertFullValidatorSet(fullValidatorSet validatorSetState, dbTx *stateTx) error {
	/* START ALPHA CHANGE 1519 -- write the retained validator set through MDBX -- */
	insertFn := func(tx *stateTx) error {
		raw, err := fullValidatorSet.Marshal()
		if err != nil {
			return err
		}

		return tx.tx.Put(s.dbi, fullValidatorSetKey, raw, 0)
	}

	if dbTx == nil {
		return s.state.update(insertFn)
	}

	return insertFn(dbTx)
	/* FINISH ALPHA CHANGE 1519 -- write the retained validator set through MDBX -- */
}

// getFullValidatorSet returns full validator set from its bucket if exists
// If the passed tx is already open (not nil), it will use it to get full validator set
// If the passed tx is not open (it is nil), it will open a new transaction on db and get full validator set
func (s *StakeStore) getFullValidatorSet(dbTx *stateTx) (validatorSetState, error) {
	/* START ALPHA CHANGE 1520 -- read the retained validator set through MDBX -- */
	var (
		fullValidatorSet validatorSetState
		err              error
	)

	getFn := func(tx *stateTx) error {
		raw, err := tx.tx.Get(s.dbi, fullValidatorSetKey)
		if mdbxgo.IsNotFound(err) {
			return errNoFullValidatorSet
		}
		if err != nil {
			return err
		}

		return fullValidatorSet.Unmarshal(raw)
	}

	if dbTx == nil {
		err = s.state.view(getFn)
	} else {
		err = getFn(dbTx)
	}

	return fullValidatorSet, err
	/* FINISH ALPHA CHANGE 1520 -- read the retained validator set through MDBX -- */
}
