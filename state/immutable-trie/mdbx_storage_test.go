/* START ALPHA CHANGE 748 -- add MDBX trie storage tests -- */
package itrie

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestMDBXStorage(t *testing.T) {
	t.Parallel()

	storage, err := NewMDBXStorage(t.TempDir(), hclog.NewNullLogger())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, storage.Close())
	}()

	key := []byte("node-key")
	value := []byte("node-value")

	require.NoError(t, storage.Put(key, value))

	result, ok, err := storage.Get(key)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, value, result)

	batch := storage.Batch()
	batch.Put([]byte("batch-key"), []byte("batch-value"))
	require.NoError(t, batch.Write())

	result, ok, err = storage.Get([]byte("batch-key"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []byte("batch-value"), result)

	codeHash := types.StringToHash("code-hash")
	code := []byte("contract-code")

	require.NoError(t, storage.SetCode(codeHash, code))

	/* START ALPHA CHANGE 2153 -- assert the MDBX bytecode read error result -- */
	result, ok, err = storage.GetCode(codeHash)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, code, result)
	/* FINISH ALPHA CHANGE 2153 -- assert the MDBX bytecode read error result -- */
}

/* START ALPHA CHANGE 3075 -- preserve trie data and explicit recent allowances across reopening -- */
func TestMDBXMapSize_TrieDefaultAndReopen(t *testing.T) {
	require.Equal(t, int64(1)<<40, int64(defaultMDBXMapSize))

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
				require.NoError(t, database.Put([]byte("old-key"), []byte("old-value")))
			}()

			expectedUpper := uint64(defaultMDBXMapSize)
			if test.mapSize != 0 {
				expectedUpper = uint64(test.mapSize)
			}

			func() {
				var database Storage
				var err error
				if test.mapSize == 0 {
					database, err = NewMDBXStorage(path, logger)
				} else {
					database, err = NewMDBXStorageWithMapSize(path, logger, test.mapSize)
				}
				require.NoError(t, err)
				defer func() { require.NoError(t, database.Close()) }()

				value, ok, err := database.Get([]byte("old-key"))
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, []byte("old-value"), value)
				require.NoError(t, database.Put([]byte("new-key"), []byte("new-value")))

				info, err := database.(*KVStorage).env.Info(nil)
				require.NoError(t, err)
				require.Equal(t, expectedUpper, info.Geo.Upper)
			}()

			database, err := NewMDBXStorageReadOnly(path, logger)
			require.NoError(t, err)
			defer func() { require.NoError(t, database.Close()) }()

			for key, expected := range map[string]string{
				"old-key": "old-value",
				"new-key": "new-value",
			} {
				value, ok, err := database.Get([]byte(key))
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, []byte(expected), value)
			}

			info, err := database.(*KVStorage).env.Info(nil)
			require.NoError(t, err)
			require.Equal(t, expectedUpper, info.Geo.Upper)
		})
	}
}

/* FINISH ALPHA CHANGE 3075 -- preserve trie data and explicit recent allowances across reopening -- */

/* FINISH ALPHA CHANGE 748 -- add MDBX trie storage tests -- */
