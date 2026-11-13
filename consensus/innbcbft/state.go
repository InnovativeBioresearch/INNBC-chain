package innbcbft

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"

	/* START ALPHA CHANGE 368 -- remove legacy event-progress encoding import -- */
	/* FINISH ALPHA CHANGE 368 -- remove legacy event-progress encoding import -- */

	/* START ALPHA CHANGE 1492 -- migrate INNBCBFT consensus persistence to MDBX -- */
	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	/* FINISH ALPHA CHANGE 1492 -- migrate INNBCBFT consensus persistence to MDBX -- */)

/* START ALPHA CHANGE 369 -- remove legacy edge event progress bucket -- */
/* FINISH ALPHA CHANGE 369 -- remove legacy edge event progress bucket -- */

/* START ALPHA CHANGE 1493 -- configure the INNBCBFT MDBX environment -- */
/* START ALPHA CHANGE 1797 -- make consensus MDBX byte units explicit -- */
const (
	consensusMDBXMapSizeBytes = 1 * 1024 * 1024 * 1024
	consensusMDBXPageSize     = 4096
	consensusMDBXMaxDBs       = 4
)

/* FINISH ALPHA CHANGE 1797 -- make consensus MDBX byte units explicit -- */

/* FINISH ALPHA CHANGE 1493 -- configure the INNBCBFT MDBX environment -- */

// MessageSignature encapsulates sender identifier and its signature
type MessageSignature struct {
	// Signer of the vote
	From string
	// Signature of the message
	Signature []byte
}

// TransportMessage represents the payload which is gossiped across the network
type TransportMessage struct {
	// Hash is encoded data
	Hash []byte
	// Message signature
	Signature []byte
	// From is the address of the message signer
	From string
	// Number of epoch
	EpochNumber uint64
}

// State represents a persistence layer which persists consensus data off-chain
type State struct {
	/* START ALPHA CHANGE 1494 -- retain the MDBX environment and access mode -- */
	db       *mdbxgo.Env
	readOnly bool
	/* FINISH ALPHA CHANGE 1494 -- retain the MDBX environment and access mode -- */

	close chan struct{}

	/* START ALPHA CHANGE 1426 -- make consensus state database close idempotent -- */
	closeOnce sync.Once
	closeErr  error
	/* FINISH ALPHA CHANGE 1426 -- make consensus state database close idempotent -- */

	/* START ALPHA CHANGE 370 -- remove legacy state-sync and checkpoint stores -- */
	EpochStore            *EpochStore
	ProposerSnapshotStore *ProposerSnapshotStore
	StakeStore            *StakeStore
	/* FINISH ALPHA CHANGE 370 -- remove legacy state-sync and checkpoint stores -- */
}

/* START ALPHA CHANGE 1495 -- safely manage explicit MDBX consensus transactions -- */
type stateTx struct {
	tx    *mdbxgo.Txn
	write bool
	done  bool
}

func (t *stateTx) Commit() error {
	if t == nil || t.done {
		return nil
	}

	_, err := t.tx.Commit()
	t.done = true

	if t.write {
		runtime.UnlockOSThread()
	}

	return err
}

func (t *stateTx) Rollback() error {
	if t == nil || t.done {
		return nil
	}

	t.tx.Abort()
	t.done = true

	if t.write {
		runtime.UnlockOSThread()
	}

	return nil
}

/* FINISH ALPHA CHANGE 1495 -- safely manage explicit MDBX consensus transactions -- */

// newState creates new instance of State
/*START ALPHA CHANGE 56 -- remove unused logger parameter from state constructor --*/
func newState(path string, closeCh chan struct{}) (*State, error) {
	/*FINISH ALPHA CHANGE 56 -- remove unused logger parameter from state constructor --*/
	return openState(path, closeCh, false)
}

/* START ALPHA CHANGE 1496 -- open writable or concurrently readable INNBCBFT MDBX state -- */
// newStateReadOnly opens consensus state for an independent read-only process.
// libmdbx intentionally rejects opening the same environment twice inside one
// process, but permits concurrent access from separate cooperating processes.
func newStateReadOnly(path string) (*State, error) {
	return openState(path, nil, true)
}

func openState(path string, closeCh chan struct{}, readOnly bool) (*State, error) {
	if !readOnly {
		if err := os.MkdirAll(path, 0750); err != nil {
			return nil, fmt.Errorf("failed to create consensus MDBX directory: %w", err)
		}
	}

	db, err := mdbxgo.NewEnv(mdbxgo.Default)
	if err != nil {
		return nil, fmt.Errorf("failed to create consensus MDBX environment: %w", err)
	}

	closeDBOnError := func(openErr error) error {
		if closeErr := db.Close(); closeErr != nil {
			return errors.Join(openErr, fmt.Errorf("failed to close consensus MDBX environment: %w", closeErr))
		}

		return openErr
	}

	if err := db.SetGeometry(
		-1,
		-1,
		consensusMDBXMapSizeBytes,
		-1,
		-1,
		consensusMDBXPageSize,
	); err != nil {
		return nil, closeDBOnError(fmt.Errorf("failed to configure consensus MDBX geometry: %w", err))
	}

	if err := db.SetOption(mdbxgo.OptMaxDB, consensusMDBXMaxDBs); err != nil {
		return nil, closeDBOnError(fmt.Errorf("failed to configure consensus MDBX named databases: %w", err))
	}

	flags := uint(0)
	if readOnly {
		flags = mdbxgo.Readonly
	}

	if err := db.Open(path, flags, 0600); err != nil {
		return nil, closeDBOnError(fmt.Errorf("failed to open consensus MDBX environment: %w", err))
	}

	s := &State{
		db:       db,
		readOnly: readOnly,
		close:    closeCh,
	}

	/* START ALPHA CHANGE 371 -- stop constructing legacy state-sync and checkpoint stores -- */
	s.EpochStore = &EpochStore{state: s}
	s.ProposerSnapshotStore = &ProposerSnapshotStore{state: s}
	s.StakeStore = &StakeStore{state: s}
	/* FINISH ALPHA CHANGE 371 -- stop constructing legacy state-sync and checkpoint stores -- */

	if err = s.initStorages(); err != nil {
		/* START ALPHA CHANGE 1427 -- close consensus database after storage initialization failure -- */
		if closeErr := s.Close(); closeErr != nil {
			return nil, errors.Join(
				err,
				fmt.Errorf("failed to close consensus state after initialization failure: %w", closeErr),
			)
		}
		/* FINISH ALPHA CHANGE 1427 -- close consensus database after storage initialization failure -- */

		return nil, err
	}

	return s, nil
}

/* FINISH ALPHA CHANGE 1496 -- open writable or concurrently readable INNBCBFT MDBX state -- */

/* START ALPHA CHANGE 1428 -- close consensus state database explicitly -- */
// Close releases the consensus state database.
func (s *State) Close() error {
	if s == nil {
		return nil
	}

	s.closeOnce.Do(func() {
		if s.db != nil {
			s.closeErr = s.db.Close()
		}
	})

	return s.closeErr
}

/* FINISH ALPHA CHANGE 1428 -- close consensus state database explicitly -- */

// initStorages initializes data storages
func (s *State) initStorages() error {
	/* START ALPHA CHANGE 1497 -- initialize MDBX named consensus databases -- */
	flags := uint(mdbxgo.Create)
	run := s.update

	if s.readOnly {
		flags = 0
		run = s.view
	}

	return run(func(tx *stateTx) error {
		/* START ALPHA CHANGE 372 -- initialize only INNBC-native consensus stores -- */
		if err := s.EpochStore.initialize(tx, flags); err != nil {
			return err
		}
		if err := s.ProposerSnapshotStore.initialize(tx, flags); err != nil {
			return err
		}
		if err := s.StakeStore.initialize(tx, flags); err != nil {
			return err
		}

		return nil
		/* FINISH ALPHA CHANGE 372 -- initialize only INNBC-native consensus stores -- */
	})
	/* FINISH ALPHA CHANGE 1497 -- initialize MDBX named consensus databases -- */
}

/* START ALPHA CHANGE 373 -- remove legacy edge event progress accessors -- */
/* FINISH ALPHA CHANGE 373 -- remove legacy edge event progress accessors -- */

/* START ALPHA CHANGE 1498 -- provide explicit MDBX consensus transaction helpers -- */
// beginDBTransaction creates and begins an MDBX transaction.
// The transaction must be rolled back or committed by its owner.
func (s *State) beginDBTransaction(isWriteTx bool) (*stateTx, error) {
	if isWriteTx && s.readOnly {
		return nil, errors.New("cannot begin writable transaction on read-only consensus state")
	}

	flags := uint(mdbxgo.Readonly)
	if isWriteTx {
		runtime.LockOSThread()
		flags = 0
	}

	tx, err := s.db.BeginTxn(nil, flags)
	if err != nil {
		if isWriteTx {
			runtime.UnlockOSThread()
		}

		return nil, err
	}

	return &stateTx{
		tx:    tx,
		write: isWriteTx,
	}, nil
}

func (s *State) view(fn func(*stateTx) error) error {
	tx, err := s.beginDBTransaction(false)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	return fn(tx)
}

func (s *State) update(fn func(*stateTx) error) error {
	tx, err := s.beginDBTransaction(true)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *State) dbStats(dbi mdbxgo.DBI) (*mdbxgo.Stat, error) {
	var stats *mdbxgo.Stat

	err := s.view(func(tx *stateTx) error {
		var err error
		stats, err = tx.tx.StatDBI(dbi)

		return err
	})
	if err != nil {
		return nil, err
	}

	return stats, nil
}

/* FINISH ALPHA CHANGE 1498 -- provide explicit MDBX consensus transaction helpers -- */
