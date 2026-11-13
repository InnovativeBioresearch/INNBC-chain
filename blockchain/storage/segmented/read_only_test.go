/* START ALPHA CHANGE 3188 -- verify read-only segmented inspection preserves databases and incomplete layouts -- */
package segmented

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	bcstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/stretchr/testify/require"
)

func readOnlyFixture(t *testing.T) *Storage {
	t.Helper()

	store, err := NewStorage(
		t.TempDir(), hclog.NewNullLogger(), 2, 4, 256*1024*1024,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	for number := range 6 {
		require.NoError(t, writeTestCanonicalHeader(store, uint64(number)))
	}
	require.NoError(t, store.Close())

	return store
}

func readOnlyFiles(t *testing.T, root string) map[string][sha256.Size]byte {
	t.Helper()

	result := make(map[string][sha256.Size]byte)
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return result
	} else {
		require.NoError(t, err)
	}

	require.NoError(t, filepath.WalkDir(root, func(
		path string, entry fs.DirEntry, walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			result[relative+"/"] = [sha256.Size]byte{}
			return nil
		}
		// MDBX reader-lock bookkeeping is separate from database contents.
		if entry.Name() == "mdbx.lck" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[relative] = sha256.Sum256(raw)
		return nil
	}))

	return result
}

func TestSegmentedReadOnly_PreservesSourceAndRejectsWrites(t *testing.T) {
	fixture := readOnlyFixture(t)

	retired := filepath.Join(
		fixture.segmentsPath(), ".deleting-"+fixture.segmentName(1),
	)
	require.NoError(t, os.Mkdir(retired, 0700))
	require.NoError(t, os.WriteFile(
		filepath.Join(retired, "retired-data"), []byte("preserve"), 0600,
	))

	before := readOnlyFiles(t, fixture.path)

	// A smaller requested retention must not prune existing segments during
	// inspection or reconfigure their stored MDBX geometry.
	reader, err := NewStorageReadOnly(
		fixture.path, hclog.NewNullLogger(), 2, 2,
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, reader.Close()) }()

	startup, err := reader.ReadStartupState()
	require.NoError(t, err)
	require.False(t, startup.Empty)
	require.True(t, startup.HasGenesis)
	require.Equal(t, uint64(5), startup.Head.Number)

	availability, ok := reader.(bcstorage.HistoryAvailabilityStorage)
	require.True(t, ok)
	require.Equal(t, uint64(3), availability.HistoryFrom())

	targeted, ok := reader.(bcstorage.BlockBatchStorage)
	require.True(t, ok)
	for _, number := range []uint64{0, 5, 7} {
		batch := targeted.NewBatchForBlock(number)
		batch.Put([]byte("forbidden"), []byte("write"))
		batch.Delete([]byte("another-key"))
		require.ErrorContains(t, batch.Write(), "read-only")
	}
	require.ErrorContains(t, reader.NewBatch().Write(), "read-only")

	require.Equal(t, before, readOnlyFiles(t, fixture.path))
	require.NoDirExists(t, fixture.segmentPath(7))
	require.FileExists(t, filepath.Join(retired, "retired-data"))
}

func TestSegmentedReadOnly_RejectsIncompleteLayouts(t *testing.T) {
	for _, scenario := range []string{
		"absent",
		"empty",
		"missing_genesis",
		"missing_segments",
		"missing_database",
	} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "blockchain")

			switch scenario {
			case "empty":
				require.NoError(t, os.Mkdir(path, 0700))
			case "missing_genesis":
				require.NoError(t, os.MkdirAll(
					filepath.Join(path, segmentsDirectoryName), 0700,
				))
			case "missing_segments":
				require.NoError(t, os.MkdirAll(
					filepath.Join(path, genesisDirectoryName), 0700,
				))
			case "missing_database":
				require.NoError(t, os.MkdirAll(
					filepath.Join(path, genesisDirectoryName), 0700,
				))
				require.NoError(t, os.MkdirAll(
					filepath.Join(path, segmentsDirectoryName), 0700,
				))
			}

			before := readOnlyFiles(t, path)
			reader, err := NewStorageReadOnly(
				path, hclog.NewNullLogger(), 2, 4,
			)
			require.Error(t, err)
			require.Nil(t, reader)
			require.Equal(t, before, readOnlyFiles(t, path))
			require.NoFileExists(t, filepath.Join(
				path, genesisDirectoryName, "mdbx.dat",
			))
		})
	}
}

func TestSegmentedReadOnly_RejectsIncompatibleGeometry(t *testing.T) {
	fixture := readOnlyFixture(t)
	before := readOnlyFiles(t, fixture.path)

	reader, err := NewStorageReadOnly(
		fixture.path, hclog.NewNullLogger(), 3, 6,
	)
	require.ErrorContains(t, err, "geometry")
	require.Nil(t, reader)
	require.Equal(t, before, readOnlyFiles(t, fixture.path))
}

func TestSegmentedReadOnly_ClosesPartialOpen(t *testing.T) {
	fixture := readOnlyFixture(t)

	// Genesis and segment 3 open before the missing data file in segment 5
	// causes construction to fail.
	require.NoError(t, os.Remove(filepath.Join(
		fixture.segmentPath(5), "mdbx.dat",
	)))
	before := readOnlyFiles(t, fixture.path)

	reader, err := NewStorageReadOnly(
		fixture.path, hclog.NewNullLogger(), 2, 4,
	)
	require.Error(t, err)
	require.Nil(t, reader)
	require.Equal(t, before, readOnlyFiles(t, fixture.path))

	// Previously opened components must be available for a subsequent open.
	for _, number := range []uint64{0, 3} {
		database, err := mdbxstorage.NewMDBXStorageReadOnly(
			fixture.segmentPath(number), hclog.NewNullLogger(),
		)
		require.NoError(t, err)
		require.NoError(t, database.Close())
	}
}

/* FINISH ALPHA CHANGE 3188 -- verify read-only segmented inspection preserves databases and incomplete layouts -- */
