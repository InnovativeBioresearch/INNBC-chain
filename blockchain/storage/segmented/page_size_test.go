/* START ALPHA CHANGE 3487 -- verify segment creation pages and rejection before cleanup -- */
package segmented

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	"github.com/stretchr/testify/require"
)

func TestMDBXPageSize_Segments(t *testing.T) {
	path := t.TempDir()
	logger := hclog.NewNullLogger()

	store, err := NewStorage(path, logger, 2, 4, 256*1024*1024, 8192)
	require.NoError(t, err)
	for number := uint64(0); number <= 5; number++ {
		require.NoError(t, writeTestCanonicalHeader(store, number))
	}
	require.NoError(t, store.Close())
	require.NoError(t, mdbxconfig.CheckTree(path, 8192))

	retired := filepath.Join(path, segmentsDirectoryName, deletingPrefix+"preserve")
	require.NoError(t, os.Mkdir(retired, 0700))
	marker := filepath.Join(retired, "marker")
	require.NoError(t, os.WriteFile(marker, []byte("preserve"), 0600))

	store, err = NewStorage(path, logger, 2, 4, 256*1024*1024, 4096)
	require.ErrorIs(t, err, mdbxconfig.ErrPageSizeMismatch)
	require.Nil(t, store)
	require.FileExists(t, marker)

	store, err = NewStorage(path, logger, 2, 4, 256*1024*1024, 8192)
	require.NoError(t, err)
	require.NoError(t, store.Close())
}

/* FINISH ALPHA CHANGE 3487 -- verify segment creation pages and rejection before cleanup -- */
