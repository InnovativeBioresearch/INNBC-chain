/* START ALPHA CHANGE 1691 -- test snapshot builder geometry and manifest rules -- */
package snapshotbuilder

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestCalculateRetainedFrom(t *testing.T) {
	t.Parallel()

	require.Equal(
		t,
		uint64(1),
		calculateRetainedFrom(KindSegmented, 100, 43200, 1800),
	)

	require.Equal(
		t,
		uint64(190801),
		calculateRetainedFrom(KindSegmented, 233230, 43200, 1800),
	)

	require.Equal(
		t,
		uint64(190031),
		calculateRetainedFrom(KindCheckpoint, 233230, 43200, 0),
	)
}

func TestValidateManifest(t *testing.T) {
	t.Parallel()

	/* START ALPHA CHANGE 1816 -- include genesis state commitment in manifest tests -- */
	valid := &Manifest{
		FormatVersion:    FormatVersion,
		SnapshotKind:     KindSegmented,
		BlockNumber:      233230,
		BlockHash:        types.StringToHash("0x01"),
		StateRoot:        types.StringToHash("0x02"),
		GenesisHash:      types.StringToHash("0x03"),
		GenesisStateRoot: types.StringToHash("0x04"),
		RetainedFrom:     190801,
		RetainedTo:       233230,
		RetentionBlocks:  43200,
		SegmentBlocks:    1800,
		/* START ALPHA CHANGE 2106 -- include the required consensus commitment in valid fixtures -- */
		ConsensusData: SnapshotFile{
			Path:   snapshotConsensusDataPath,
			SHA256: "0101010101010101010101010101010101010101010101010101010101010101",
			Bytes:  1,
		},
		/* FINISH ALPHA CHANGE 2106 -- include the required consensus commitment in valid fixtures -- */
		SnapshotComplete: true,
	}
	/* FINISH ALPHA CHANGE 1816 -- include genesis state commitment in manifest tests -- */

	require.NoError(t, ValidateManifest(valid))

	checkpoint := *valid
	checkpoint.SnapshotKind = KindCheckpoint
	checkpoint.SegmentBlocks = 0
	checkpoint.RetainedFrom = 190031

	require.NoError(t, ValidateManifest(&checkpoint))

	incomplete := *valid
	incomplete.SnapshotComplete = false
	require.ErrorContains(t, ValidateManifest(&incomplete), "not complete")

	/* START ALPHA CHANGE 1817 -- reject missing genesis state commitment -- */
	missingGenesisStateRoot := *valid
	missingGenesisStateRoot.GenesisStateRoot = types.ZeroHash
	require.ErrorContains(
		t,
		ValidateManifest(&missingGenesisStateRoot),
		"genesis state root must not be zero",
	)
	/* FINISH ALPHA CHANGE 1817 -- reject missing genesis state commitment -- */

	wrongKind := *valid
	wrongKind.SnapshotKind = Kind("unknown")
	require.ErrorContains(t, ValidateManifest(&wrongKind), "unsupported snapshot kind")

	wrongGeometry := *valid
	wrongGeometry.RetentionBlocks = 43201
	require.ErrorContains(
		t,
		ValidateManifest(&wrongGeometry),
		"retention must be divisible",
	)

	checkpointWithSegments := checkpoint
	checkpointWithSegments.SegmentBlocks = 1800
	require.ErrorContains(
		t,
		ValidateManifest(&checkpointWithSegments),
		"must not define segment geometry",
	)

	/* START ALPHA CHANGE 1715 -- reject fabricated snapshot retention floors -- */
	wrongSegmentedRetainedFrom := *valid
	wrongSegmentedRetainedFrom.RetainedFrom++
	require.ErrorContains(
		t,
		ValidateManifest(&wrongSegmentedRetainedFrom),
		"snapshot retained range must start at 190801",
	)

	wrongCheckpointRetainedFrom := checkpoint
	wrongCheckpointRetainedFrom.RetainedFrom--
	require.ErrorContains(
		t,
		ValidateManifest(&wrongCheckpointRetainedFrom),
		"snapshot retained range must start at 190031",
	)
	/* FINISH ALPHA CHANGE 1715 -- reject fabricated snapshot retention floors -- */
}

func TestSnapshotDirectoryName(t *testing.T) {
	t.Parallel()

	hash := types.StringToHash(
		"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
	)

	require.Equal(
		t,
		"block-00000000000000233230-1234567890ab",
		snapshotDirectoryName(233230, hash),
	)
}

/* START ALPHA CHANGE 1752 -- test role-specific snapshot output directories -- */
func TestKindDirectoryName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "segmented", KindSegmented.directoryName())
	require.Equal(t, "checkpoint", KindCheckpoint.directoryName())
}

/* FINISH ALPHA CHANGE 1752 -- test role-specific snapshot output directories -- */

/* START ALPHA CHANGE 1836 -- test snapshot parent-directory synchronization -- */
func TestSyncDirectory(t *testing.T) {
	t.Parallel()

	require.NoError(t, syncDirectory(t.TempDir()))
}

/* FINISH ALPHA CHANGE 1836 -- test snapshot parent-directory synchronization -- */

/* FINISH ALPHA CHANGE 1691 -- test snapshot builder geometry and manifest rules -- */
