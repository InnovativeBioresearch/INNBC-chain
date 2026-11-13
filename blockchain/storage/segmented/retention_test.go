/* START ALPHA CHANGE 3142 -- test retained segment boundaries and storage preservation -- */
package segmented

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestRecentHistorySegments_CountBoundaries(t *testing.T) {
	maximum := uint64(math.MaxInt)

	for _, tc := range []struct {
		name      string
		segment   uint64
		retention uint64
		want      int
		wantErr   string
	}{
		{"zero_segment", 0, 43200, 0, "segment blocks must be greater than zero"},
		{"zero_retention", 1, 0, 0, "retention blocks must be at least one segment"},
		{"less_than_segment", 10, 5, 0, "retention blocks must be at least one segment"},
		{"not_divisible", 6, 10, 0, "retention blocks must be divisible"},
		{"default", 1800, 43200, 24, ""},
		{"one_segment", 1, 1, 1, ""},
		{"maximum_count", 1, maximum, math.MaxInt, ""},
		{"first_unsupported_count", 1, maximum + 1, 0, "exceeds platform maximum"},
		{"reported_64_bit_input", 1, uint64(1) << 63, 0, "exceeds platform maximum"},
		{"maximum_unsigned_count", 1, math.MaxUint64, 0, "exceeds platform maximum"},
		{"large_retention_one_segment", math.MaxUint64, math.MaxUint64, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count, err := RetainedSegmentCount(tc.segment, tc.retention)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Zero(t, count)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, count)
			require.Positive(t, count)
		})
	}
}

func TestRecentHistorySegments_RejectsBeforeDirectoryCreation(t *testing.T) {
	for _, retention := range []uint64{
		uint64(math.MaxInt) + 1,
		uint64(1) << 63,
		math.MaxUint64,
	} {
		root := t.TempDir()
		path := filepath.Join(root, "blockchain")

		store, err := NewStorage(
			path, hclog.NewNullLogger(), 1, retention, 256*1024*1024,
		)
		require.Nil(t, store)
		require.ErrorContains(t, err, "exceeds platform maximum")

		_, statErr := os.Stat(path)
		require.ErrorIs(t, statErr, os.ErrNotExist)

		entries, readErr := os.ReadDir(root)
		require.NoError(t, readErr)
		require.Empty(t, entries)
	}
}

func TestRecentHistorySegments_GenesisWriteAndReopen(t *testing.T) {
	for _, tc := range []struct {
		name      string
		segment   uint64
		retention uint64
	}{
		{"default", 1800, 43200},
		{"maximum_count", 1, uint64(math.MaxInt)},
		{"large_retention_one_segment", math.MaxUint64, math.MaxUint64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "blockchain")
			store, err := NewStorage(
				path, hclog.NewNullLogger(),
				tc.segment, tc.retention, 256*1024*1024,
			)
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })

			require.NoError(t, writeTestCanonicalHeader(store, 0))
			require.NoError(t, writeTestCanonicalHeader(store, 1))
			require.NoError(t, writeTestCanonicalHeader(store, 2))

			head, ok := store.ReadHeadNumber()
			require.True(t, ok)
			require.Equal(t, uint64(2), head)

			for _, number := range []uint64{0, 1, 2} {
				_, exists := store.ReadCanonicalHash(number)
				require.True(t, exists)
			}

			require.NoError(t, store.Close())

			// An invalid attempted reopen must not alter existing database data.
			genesisFile := filepath.Join(path, genesisDirectoryName, "mdbx.dat")
			before, err := os.ReadFile(genesisFile)
			require.NoError(t, err)

			rejected, err := NewStorage(
				path, hclog.NewNullLogger(),
				1, uint64(math.MaxInt)+1, 256*1024*1024,
			)
			require.Nil(t, rejected)
			require.ErrorContains(t, err, "exceeds platform maximum")

			after, err := os.ReadFile(genesisFile)
			require.NoError(t, err)
			require.Equal(t, before, after)

			reopened, err := NewStorage(
				path, hclog.NewNullLogger(),
				tc.segment, tc.retention, 256*1024*1024,
			)
			require.NoError(t, err)
			t.Cleanup(func() { _ = reopened.Close() })

			head, ok = reopened.ReadHeadNumber()
			require.True(t, ok)
			require.Equal(t, uint64(2), head)

			for _, number := range []uint64{0, 1, 2} {
				_, exists := reopened.ReadCanonicalHash(number)
				require.True(t, exists)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3142 -- test retained segment boundaries and storage preservation -- */
