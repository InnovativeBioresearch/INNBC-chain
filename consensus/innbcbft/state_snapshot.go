/* START ALPHA CHANGE 1688 -- export fixed-checkpoint INNBCBFT consensus state -- */
package innbcbft

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
)

// ExportSnapshotState exports one transactionally consistent consensus-state
// view. The exported checkpoint is derived from the persisted proposer
// snapshot, whose height represents the next block to be built.
func ExportSnapshotState(sourceDir, targetDir string) (uint64, error) {
	sourcePath := filepath.Join(sourceDir, stateFileName)
	targetPath := filepath.Join(targetDir, stateFileName)

	if _, err := os.Stat(targetPath); err == nil {
		return 0, fmt.Errorf("target consensus snapshot already exists: %s", targetPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("failed to inspect target consensus snapshot: %w", err)
	}

	source, err := newStateReadOnly(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open source consensus state: %w", err)
	}
	defer source.Close() //nolint:errcheck

	target, err := newState(targetPath, make(chan struct{}))
	if err != nil {
		return 0, fmt.Errorf("failed to open target consensus state: %w", err)
	}

	closeTarget := func(exportErr error) error {
		if closeErr := target.Close(); closeErr != nil {
			return errors.Join(
				exportErr,
				fmt.Errorf("failed to close target consensus state: %w", closeErr),
			)
		}

		return exportErr
	}

	var checkpoint uint64

	err = source.view(func(sourceTx *stateTx) error {
		var checkpointErr error
		checkpoint, checkpointErr = snapshotCheckpointInTx(source, sourceTx)
		if checkpointErr != nil {
			return checkpointErr
		}

		return target.update(func(targetTx *stateTx) error {
			databases := []struct {
				name      string
				sourceDBI mdbxgo.DBI
				targetDBI mdbxgo.DBI
			}{
				{
					name:      string(epochsBucket),
					sourceDBI: source.EpochStore.epochsDBI,
					targetDBI: target.EpochStore.epochsDBI,
				},
				{
					name:      string(validatorSnapshotsBucket),
					sourceDBI: source.EpochStore.validatorSnapshotsDBI,
					targetDBI: target.EpochStore.validatorSnapshotsDBI,
				},
				{
					name:      string(proposerSnapshotBucket),
					sourceDBI: source.ProposerSnapshotStore.dbi,
					targetDBI: target.ProposerSnapshotStore.dbi,
				},
				{
					name:      string(validatorSetBucket),
					sourceDBI: source.StakeStore.dbi,
					targetDBI: target.StakeStore.dbi,
				},
			}

			for _, database := range databases {
				if err := copyConsensusDatabase(
					sourceTx,
					targetTx,
					database.sourceDBI,
					database.targetDBI,
				); err != nil {
					return fmt.Errorf(
						"failed to export consensus database %q: %w",
						database.name,
						err,
					)
				}
			}

			return nil
		})
	})
	if err != nil {
		return 0, closeTarget(err)
	}

	if err := target.Close(); err != nil {
		return 0, fmt.Errorf("failed to close exported consensus state: %w", err)
	}

	exportedCheckpoint, err := SnapshotStateCheckpoint(targetDir)
	if err != nil {
		return 0, fmt.Errorf("failed to validate exported consensus checkpoint: %w", err)
	}

	if exportedCheckpoint != checkpoint {
		return 0, fmt.Errorf(
			"exported consensus checkpoint mismatch: expected %d, got %d",
			checkpoint,
			exportedCheckpoint,
		)
	}

	return checkpoint, nil
}

func SnapshotStateCheckpoint(consensusDir string) (uint64, error) {
	state, err := newStateReadOnly(filepath.Join(consensusDir, stateFileName))
	if err != nil {
		return 0, fmt.Errorf("failed to open snapshot consensus state: %w", err)
	}
	defer state.Close() //nolint:errcheck

	var checkpoint uint64

	if err := state.view(func(tx *stateTx) error {
		var checkpointErr error
		checkpoint, checkpointErr = snapshotCheckpointInTx(state, tx)

		return checkpointErr
	}); err != nil {
		return 0, err
	}

	return checkpoint, nil
}

func snapshotCheckpointInTx(state *State, tx *stateTx) (uint64, error) {
	snapshot, err := state.ProposerSnapshotStore.getProposerSnapshot(tx)
	if err != nil {
		return 0, fmt.Errorf("failed to read proposer snapshot: %w", err)
	}

	if snapshot == nil {
		return 0, errors.New("consensus proposer snapshot is missing")
	}

	if snapshot.Height == 0 {
		return 0, errors.New("consensus proposer snapshot has invalid zero height")
	}

	return snapshot.Height - 1, nil
}

func copyConsensusDatabase(
	sourceTx *stateTx,
	targetTx *stateTx,
	sourceDBI mdbxgo.DBI,
	targetDBI mdbxgo.DBI,
) error {
	cursor, err := sourceTx.tx.OpenCursor(sourceDBI)
	if err != nil {
		return err
	}
	defer cursor.Close()

	operation := uint(mdbxgo.First)

	for {
		key, value, err := cursor.Get(nil, nil, operation)
		if mdbxgo.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}

		if err := targetTx.tx.Put(
			targetDBI,
			append([]byte(nil), key...),
			append([]byte(nil), value...),
			0,
		); err != nil {
			return err
		}

		operation = mdbxgo.Next
	}
}

/* FINISH ALPHA CHANGE 1688 -- export fixed-checkpoint INNBCBFT consensus state -- */
