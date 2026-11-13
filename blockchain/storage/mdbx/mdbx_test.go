/* START ALPHA CHANGE 747 -- add MDBX blockchain storage tests -- */
package mdbx

import (
	"os"
	"testing"

	/* START ALPHA CHANGE 3074 -- inspect persisted blockchain MDBX geometry -- */
	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	/* FINISH ALPHA CHANGE 3074 -- inspect persisted blockchain MDBX geometry -- */
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"

	/* START ALPHA CHANGE 3074 -- assert default and explicit MDBX allowances -- */
	"github.com/stretchr/testify/require"
	/* FINISH ALPHA CHANGE 3074 -- assert default and explicit MDBX allowances -- */)

func newStorage(t *testing.T) (storage.Storage, func()) {
	t.Helper()

	path, err := os.MkdirTemp("/tmp", "mdbx_storage")
	if err != nil {
		t.Fatal(err)
	}

	s, err := NewMDBXStorage(path, hclog.NewNullLogger())
	if err != nil {
		t.Fatal(err)
	}

	closeFn := func() {
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}

		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}

	return s, closeFn
}

func TestStorage(t *testing.T) {
	storage.TestStorage(t, newStorage)
}

/* START ALPHA CHANGE 3074 -- preserve blockchain data when increasing the MDBX allowance -- */
func TestMDBXMapSize_BlockchainDefaultAndReopen(t *testing.T) {
	require.Equal(t, int64(1)<<40, int64(DefaultMapSize))

	for _, test := range []struct {
		name    string
		mapSize int
	}{
		{name: "full_default"},
		{name: "explicit_recent", mapSize: 128 * 1024 * 1024 * 1024},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir()
			logger := hclog.NewNullLogger()

			func() {
				database, err := NewMDBXStorageWithMapSize(path, logger, 64*1024*1024)
				require.NoError(t, err)
				defer func() { require.NoError(t, database.Close()) }()

				writer := storage.NewBatchWriter(database)
				writer.PutHeadNumber(42)
				require.NoError(t, writer.WriteBatch())
			}()

			expectedUpper := uint64(DefaultMapSize)
			if test.mapSize != 0 {
				expectedUpper = uint64(test.mapSize)
			}

			func() {
				var database storage.Storage
				var err error
				if test.mapSize == 0 {
					database, err = NewMDBXStorage(path, logger)
				} else {
					database, err = NewMDBXStorageWithMapSize(path, logger, test.mapSize)
				}
				require.NoError(t, err)
				defer func() { require.NoError(t, database.Close()) }()

				number, ok := database.ReadHeadNumber()
				require.True(t, ok)
				require.Equal(t, uint64(42), number)

				writer := storage.NewBatchWriter(database)
				writer.PutHeadNumber(43)
				require.NoError(t, writer.WriteBatch())
			}()

			func() {
				database, err := NewMDBXStorageReadOnly(path, logger)
				require.NoError(t, err)
				defer func() { require.NoError(t, database.Close()) }()

				number, ok := database.ReadHeadNumber()
				require.True(t, ok)
				require.Equal(t, uint64(43), number)
			}()

			env, err := mdbxgo.NewEnv(mdbxgo.Default)
			require.NoError(t, err)
			defer func() { require.NoError(t, env.Close()) }()
			require.NoError(t, env.Open(path, mdbxgo.Readonly, 0600))

			info, err := env.Info(nil)
			require.NoError(t, err)
			require.Equal(t, expectedUpper, info.Geo.Upper)
		})
	}
}

/* FINISH ALPHA CHANGE 3074 -- preserve blockchain data when increasing the MDBX allowance -- */

/* FINISH ALPHA CHANGE 747 -- add MDBX blockchain storage tests -- */
