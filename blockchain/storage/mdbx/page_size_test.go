/* START ALPHA CHANGE 3485 -- verify actual blockchain database page enforcement -- */
package mdbx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	"github.com/stretchr/testify/require"
)

func TestMDBXPageSize_Blockchain(t *testing.T) {
	for _, size := range []uint64{4096, 8192} {
		t.Run(mdbxconfig.PageSize(size).String(), func(t *testing.T) {
			path := t.TempDir()
			logger := hclog.NewNullLogger()

			db, err := NewMDBXStorageWithMapSize(path, logger, 64*1024*1024, size)
			require.NoError(t, err)
			require.NoError(t, db.Close())
			require.NoError(t, mdbxconfig.CheckDatabase(path, size))

			reader, err := NewMDBXStorageReadOnly(path, logger)
			require.NoError(t, err)
			/* START ALPHA CHANGE 3496 -- retain reader cleanup while testing mismatch rejection -- */
			readerOpen := true
			defer func() {
				if readerOpen {
					require.NoError(t, reader.Close())
				}
			}()
			/* FINISH ALPHA CHANGE 3496 -- retain reader cleanup while testing mismatch rejection -- */

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

			/* START ALPHA CHANGE 3496 -- release the reader before successful writable reopening -- */
			require.NoError(t, reader.Close())
			readerOpen = false
			/* FINISH ALPHA CHANGE 3496 -- release the reader before successful writable reopening -- */

			db, err = NewMDBXStorageWithMapSize(path, logger, 64*1024*1024, size)
			require.NoError(t, err)
			require.NoError(t, db.Close())
		})
	}
}

/* FINISH ALPHA CHANGE 3485 -- verify actual blockchain database page enforcement -- */
