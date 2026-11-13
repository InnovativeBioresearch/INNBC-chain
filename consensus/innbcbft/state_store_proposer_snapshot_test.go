package innbcbft

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestState_getProposerSnapshot_writeProposerSnapshot(t *testing.T) {
	t.Parallel()

	const (
		height = uint64(100)
		round  = uint64(5)
	)

	state := newTestState(t)

	snap, err := state.ProposerSnapshotStore.getProposerSnapshot(nil)
	require.NoError(t, err)
	require.Nil(t, snap)

	newSnapshot := &ProposerSnapshot{Height: height, Round: round}
	require.NoError(t, state.ProposerSnapshotStore.writeProposerSnapshot(newSnapshot, nil))

	snap, err = state.ProposerSnapshotStore.getProposerSnapshot(nil)
	require.NoError(t, err)
	require.Equal(t, newSnapshot, snap)
}

/* START ALPHA CHANGE 1552 -- verify cross-process read-only consensus MDBX access -- */
func TestStateMDBXConcurrentReadOnlyAccess(t *testing.T) {
	const (
		statePathEnv = "INNBC_TEST_CONSENSUS_MDBX_PATH"
		height       = uint64(123)
		round        = uint64(4)
	)

	if statePath := os.Getenv(statePathEnv); statePath != "" {
		reader, err := newStateReadOnly(statePath)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, reader.Close())
		}()

		actual, err := reader.ProposerSnapshotStore.getProposerSnapshot(nil)
		require.NoError(t, err)
		require.Equal(t, &ProposerSnapshot{
			Height: height,
			Round:  round,
		}, actual)

		return
	}

	statePath := filepath.Join(t.TempDir(), "consensus-state")
	writer, err := newState(statePath, make(chan struct{}))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, writer.Close())
	}()

	expected := &ProposerSnapshot{
		Height: height,
		Round:  round,
	}
	require.NoError(t, writer.ProposerSnapshotStore.writeProposerSnapshot(expected, nil))

	cmd := exec.Command(
		os.Args[0],
		"-test.run=^TestStateMDBXConcurrentReadOnlyAccess$",
	)
	cmd.Env = append(os.Environ(), statePathEnv+"="+statePath)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "read-only child process failed:\n%s", output)
}

/* FINISH ALPHA CHANGE 1552 -- verify cross-process read-only consensus MDBX access -- */

/* START ALPHA CHANGE 1689 -- verify fixed-checkpoint consensus-state export -- */
func TestExportSnapshotState(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source-consensus")
	targetDir := filepath.Join(t.TempDir(), "target-consensus")

	source, err := newState(
		filepath.Join(sourceDir, stateFileName),
		make(chan struct{}),
	)
	require.NoError(t, err)

	require.NoError(
		t,
		source.EpochStore.insertEpoch(17, nil),
	)
	require.NoError(
		t,
		source.ProposerSnapshotStore.writeProposerSnapshot(
			&ProposerSnapshot{
				Height: 501,
				Round:  3,
			},
			nil,
		),
	)
	require.NoError(t, source.Close())

	checkpoint, err := ExportSnapshotState(sourceDir, targetDir)
	require.NoError(t, err)
	require.Equal(t, uint64(500), checkpoint)

	/* START ALPHA CHANGE 1697 -- avoid reopening one MDBX environment concurrently in exporter test -- */
	exportedCheckpoint, err := SnapshotStateCheckpoint(targetDir)
	require.NoError(t, err)
	require.Equal(t, checkpoint, exportedCheckpoint)

	exported, err := newStateReadOnly(
		filepath.Join(targetDir, stateFileName),
	)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, exported.Close())
	}()

	snapshot, err := exported.ProposerSnapshotStore.getProposerSnapshot(nil)
	require.NoError(t, err)
	require.Equal(
		t,
		&ProposerSnapshot{
			Height: 501,
			Round:  3,
		},
		snapshot,
	)
	require.True(t, exported.EpochStore.isEpochInserted(17))
	/* FINISH ALPHA CHANGE 1697 -- avoid reopening one MDBX environment concurrently in exporter test -- */
}

/* FINISH ALPHA CHANGE 1689 -- verify fixed-checkpoint consensus-state export -- */
