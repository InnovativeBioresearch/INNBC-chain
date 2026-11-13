/* START ALPHA CHANGE 1582 -- test physical recent-history segment retirement -- */
package segmented

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	bcstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 1610 -- add segmented canonical header test writer -- */
func writeTestCanonicalHeader(store *Storage, number uint64) error {
	header := &types.Header{
		Number:       number,
		TxRoot:       types.EmptyRootHash,
		Sha3Uncles:   types.EmptyUncleHash,
		ReceiptsRoot: types.EmptyRootHash,
	}
	header.ComputeHash()

	batch := bcstorage.NewBatchWriterForBlock(store, number)
	batch.PutCanonicalHeader(header)

	return batch.WriteBatch()
}

/* FINISH ALPHA CHANGE 1610 -- add segmented canonical header test writer -- */

func TestSegmentedStorageRetiresExpiredDirectories(t *testing.T) {
	t.Parallel()

	path := t.TempDir()

	store, err := NewStorage(
		path,
		hclog.NewNullLogger(),
		2,
		4,
		256*1024*1024,
	)
	require.NoError(t, err)

	parentHash := types.ZeroHash

	for number := uint64(0); number <= 5; number++ {
		header := &types.Header{
			Number:       number,
			ParentHash:   parentHash,
			TxRoot:       types.EmptyRootHash,
			Sha3Uncles:   types.EmptyUncleHash,
			ReceiptsRoot: types.EmptyRootHash,
		}
		header.ComputeHash()

		batch := bcstorage.NewBatchWriterForBlock(store, number)
		batch.PutCanonicalHeader(header)
		require.NoError(t, batch.WriteBatch())

		parentHash = header.Hash
	}

	/* START ALPHA CHANGE 1653 -- verify separated genesis and disposable segment layout -- */
	/* START ALPHA CHANGE 1682 -- verify live advertised history floor after retirement -- */
	require.Equal(t, uint64(3), store.HistoryFrom())
	/* FINISH ALPHA CHANGE 1682 -- verify live advertised history floor after retirement -- */

	_, err = os.Stat(filepath.Join(path, segmentsDirectoryName, store.segmentName(1)))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, err = os.Stat(filepath.Join(path, segmentsDirectoryName, store.segmentName(3)))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(path, segmentsDirectoryName, store.segmentName(5)))
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(path, genesisDirectoryName))
	require.DirExists(t, filepath.Join(path, segmentsDirectoryName))
	/* FINISH ALPHA CHANGE 1653 -- verify separated genesis and disposable segment layout -- */

	_, ok := store.ReadCanonicalHash(1)
	require.False(t, ok)

	_, ok = store.ReadCanonicalHash(3)
	require.True(t, ok)

	head, ok := store.ReadHeadNumber()
	require.True(t, ok)
	require.Equal(t, uint64(5), head)

	require.NoError(t, store.Close())

	reopened, err := NewStorage(
		path,
		hclog.NewNullLogger(),
		2,
		4,
		256*1024*1024,
	)
	require.NoError(t, err)
	defer reopened.Close()

	head, ok = reopened.ReadHeadNumber()
	require.True(t, ok)
	require.Equal(t, uint64(5), head)

	/* START ALPHA CHANGE 1611 -- verify live writing and rotation after restart -- */
	require.NoError(t, writeTestCanonicalHeader(reopened, 6))
	require.NoError(t, writeTestCanonicalHeader(reopened, 7))

	head, ok = reopened.ReadHeadNumber()
	require.True(t, ok)
	require.Equal(t, uint64(7), head)

	/* START ALPHA CHANGE 1654 -- verify post-restart rotation in segments directory -- */
	/* START ALPHA CHANGE 1683 -- verify recovered history floor advances after rotation -- */
	require.Equal(t, uint64(5), reopened.HistoryFrom())
	/* FINISH ALPHA CHANGE 1683 -- verify recovered history floor advances after rotation -- */

	_, err = os.Stat(filepath.Join(path, segmentsDirectoryName, reopened.segmentName(3)))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, err = os.Stat(filepath.Join(path, segmentsDirectoryName, reopened.segmentName(5)))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(path, segmentsDirectoryName, reopened.segmentName(7)))
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(path, genesisDirectoryName))
	/* FINISH ALPHA CHANGE 1654 -- verify post-restart rotation in segments directory -- */
	/* FINISH ALPHA CHANGE 1611 -- verify live writing and rotation after restart -- */
}

/* START ALPHA CHANGE 1584 -- reject persisted segments with incompatible boundaries -- */
func TestSegmentedStorageRejectsIncompatibleSegmentGeometry(t *testing.T) {
	t.Parallel()

	path := t.TempDir()
	incompatibleSegment := fmt.Sprintf("%s%020d-%020d", segmentPrefix, 2, 3)

	/* START ALPHA CHANGE 1655 -- place incompatible fixture in disposable segments directory -- */
	require.NoError(
		t,
		os.MkdirAll(
			filepath.Join(path, segmentsDirectoryName, incompatibleSegment),
			0755,
		),
	)
	/* FINISH ALPHA CHANGE 1655 -- place incompatible fixture in disposable segments directory -- */

	_, err := NewStorage(
		path,
		hclog.NewNullLogger(),
		2,
		4,
		256*1024*1024,
	)
	require.ErrorContains(t, err, "incompatible with configured segment size")
}

/* FINISH ALPHA CHANGE 1584 -- reject persisted segments with incompatible boundaries -- */

/* START ALPHA CHANGE 1612 -- test retired segment deletion retry without restart -- */
func TestSegmentedStorageRetriesFailedDeletionWhileOnline(t *testing.T) {
	t.Parallel()

	path := t.TempDir()

	store, err := NewStorage(
		path,
		hclog.NewNullLogger(),
		1,
		2,
		256*1024*1024,
	)
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, writeTestCanonicalHeader(store, 0))
	require.NoError(t, writeTestCanonicalHeader(store, 1))
	require.NoError(t, writeTestCanonicalHeader(store, 2))

	/* START ALPHA CHANGE 1656 -- track failed deletion inside segments directory -- */
	retiredPath := filepath.Join(
		path,
		segmentsDirectoryName,
		deletingPrefix+store.segmentName(1),
	)
	/* FINISH ALPHA CHANGE 1656 -- track failed deletion inside segments directory -- */

	deletionFailed := false
	store.removeAll = func(path string) error {
		if path == retiredPath && !deletionFailed {
			deletionFailed = true

			return fmt.Errorf("injected retired segment deletion failure")
		}

		return os.RemoveAll(path)
	}

	// Block 3 remains committed even though segment 1 cannot complete its
	// final physical removal.
	require.NoError(t, writeTestCanonicalHeader(store, 3))
	require.True(t, deletionFailed)
	require.DirExists(t, retiredPath)
	require.Contains(t, store.pendingDeletes, retiredPath)

	store.removeAll = os.RemoveAll

	// The next successful commit retries the pending removal without closing
	// or reopening segmented storage.
	require.NoError(t, writeTestCanonicalHeader(store, 4))

	_, err = os.Stat(retiredPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NotContains(t, store.pendingDeletes, retiredPath)
}

/* FINISH ALPHA CHANGE 1612 -- test retired segment deletion retry without restart -- */

/* START ALPHA CHANGE 1613 -- test bounded physical segment retention -- */
func TestSegmentedStorageKeepsSegmentCountBounded(t *testing.T) {
	t.Parallel()

	path := t.TempDir()

	store, err := NewStorage(
		path,
		hclog.NewNullLogger(),
		2,
		6,
		256*1024*1024,
	)
	require.NoError(t, err)
	defer store.Close()

	for number := uint64(0); number <= 40; number++ {
		require.NoError(t, writeTestCanonicalHeader(store, number))
	}

	/* START ALPHA CHANGE 1657 -- count bounded disposable segments separately from genesis -- */
	entries, err := os.ReadDir(filepath.Join(path, segmentsDirectoryName))
	require.NoError(t, err)

	segmentCount := 0
	for _, entry := range entries {
		if _, _, ok := parseSegmentName(entry.Name()); ok {
			segmentCount++
		}

		require.False(
			t,
			len(entry.Name()) >= len(deletingPrefix) &&
				entry.Name()[:len(deletingPrefix)] == deletingPrefix,
			"retired segment remains pending: %s",
			entry.Name(),
		)
	}

	require.Equal(t, 3, segmentCount)
	require.DirExists(t, filepath.Join(path, genesisDirectoryName))
	/* FINISH ALPHA CHANGE 1657 -- count bounded disposable segments separately from genesis -- */

	head, ok := store.ReadHeadNumber()
	require.True(t, ok)
	require.Equal(t, uint64(40), head)
}

/* FINISH ALPHA CHANGE 1613 -- test bounded physical segment retention -- */

/* FINISH ALPHA CHANGE 1582 -- test physical recent-history segment retirement -- */
