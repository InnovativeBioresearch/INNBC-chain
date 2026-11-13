/* START ALPHA CHANGE 3145 -- test segmented snapshot rejection before construction effects -- */
package snapshotbuilder

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecentHistorySegments_SnapshotGeometry(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      Kind
		retention uint64
		segment   uint64
		wantErr   string
	}{
		{"default_segmented", KindSegmented, 43200, 1800, ""},
		{"maximum_count", KindSegmented, uint64(math.MaxInt), 1, ""},
		{"first_unsupported_count", KindSegmented, uint64(math.MaxInt) + 1, 1, "exceeds platform maximum"},
		{"maximum_unsigned_count", KindSegmented, math.MaxUint64, 1, "exceeds platform maximum"},
		{"large_retention_one_segment", KindSegmented, math.MaxUint64, math.MaxUint64, ""},
		{"checkpoint_has_no_segment_count", KindCheckpoint, math.MaxUint64, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{
				Kind:            tc.kind,
				ChainPath:       "genesis.json",
				SourceDataDir:   "source",
				OutputRoot:      "output",
				RetentionBlocks: tc.retention,
				SegmentBlocks:   tc.segment,
				MDBXMapSizeGB:   128,
			}

			err := validateConfig(config)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestRecentHistorySegments_SnapshotRejectsBeforeSideEffects(t *testing.T) {
	root := t.TempDir()
	config := Config{
		Kind:            KindSegmented,
		ChainPath:       filepath.Join(root, "genesis.json"),
		SourceDataDir:   filepath.Join(root, "source"),
		OutputRoot:      filepath.Join(root, "output"),
		RetentionBlocks: uint64(math.MaxInt) + 1,
		SegmentBlocks:   1,
		MDBXMapSizeGB:   128,
	}

	result, err := Build(config)
	require.Nil(t, result)
	require.ErrorContains(t, err, "exceeds platform maximum")

	entries, readErr := os.ReadDir(root)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

/* FINISH ALPHA CHANGE 3145 -- test segmented snapshot rejection before construction effects -- */
