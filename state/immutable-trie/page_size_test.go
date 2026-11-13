/* START ALPHA CHANGE 3486 -- verify trie page enforcement and retained records -- */
package itrie

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	"github.com/stretchr/testify/require"
)

func TestMDBXPageSize_Trie(t *testing.T) {
	for _, size := range []uint64{4096, 8192} {
		t.Run(mdbxconfig.PageSize(size).String(), func(t *testing.T) {
			path := t.TempDir()
			logger := hclog.NewNullLogger()

			db, err := NewMDBXStorageWithMapSize(path, logger, 64*1024*1024, size)
			require.NoError(t, err)
			require.NoError(t, db.Put([]byte("key"), []byte("value")))
			require.NoError(t, db.Close())

			before, err := os.ReadFile(filepath.Join(path, "mdbx.dat"))
			require.NoError(t, err)

			other := uint64(4096)
			if size == other {
				other = 8192
			}
			db, err = NewMDBXStorageWithMapSize(path, logger, 64*1024*1024, other)
			require.ErrorIs(t, err, mdbxconfig.ErrPageSizeMismatch)
			require.Nil(t, db)

			after, err := os.ReadFile(filepath.Join(path, "mdbx.dat"))
			require.NoError(t, err)
			require.Equal(t, before, after)

			db, err = NewMDBXStorageWithMapSize(path, logger, 64*1024*1024, size)
			require.NoError(t, err)
			value, found, err := db.Get([]byte("key"))
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, []byte("value"), value)
			require.NoError(t, db.Close())
		})
	}
}

/* FINISH ALPHA CHANGE 3486 -- verify trie page enforcement and retained records -- */
