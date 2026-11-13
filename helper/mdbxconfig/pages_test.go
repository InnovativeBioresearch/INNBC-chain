/* START ALPHA CHANGE 3484 -- verify page-size parsing and metadata inspection -- */
package mdbxconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	"github.com/stretchr/testify/require"
)

func TestMDBXPageSize_Parsing(t *testing.T) {
	for _, text := range []string{"", " ", "\t", "4096", "8192", "16384", "32768", "65536"} {
		var size PageSize
		require.NoError(t, size.Set(text))
		value, err := Resolve(uint64(size))
		require.NoError(t, err)
		require.GreaterOrEqual(t, value, 4096)
	}
	for _, text := range []string{"0", "-1", "4097", "2048", "131072", "8kb", "999999999999999999999999"} {
		var size PageSize
		require.Error(t, size.Set(text), text)
	}

	for _, input := range []string{`4096`, `8192`, `""`, `"  "`} {
		var size PageSize
		require.NoError(t, json.Unmarshal([]byte(input), &size), input)
	}
	for _, input := range []string{`0`, `null`, `1.5`, `{}`, `"bad"`} {
		var size PageSize
		require.Error(t, json.Unmarshal([]byte(input), &size), input)
	}
}

func TestMDBXPageSize_MetadataAndPreservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database")
	require.NoError(t, CheckDatabase(path, 8192))
	require.NoDirExists(t, path)
	require.NoError(t, os.Mkdir(path, 0700))

	env, err := mdbxgo.NewEnv(mdbxgo.Default)
	require.NoError(t, err)
	require.NoError(t, env.SetGeometry(-1, -1, 64*1024*1024, -1, -1, 8192))
	require.NoError(t, env.Open(path, 0, 0600))
	require.NoError(t, env.Close())

	data := filepath.Join(path, "mdbx.dat")
	before, err := os.ReadFile(data)
	require.NoError(t, err)

	require.NoError(t, CheckDatabase(path, 8192))
	require.ErrorIs(t, CheckDatabase(path, 4096), ErrPageSizeMismatch)

	after, err := os.ReadFile(data)
	require.NoError(t, err)
	require.Equal(t, before, after)

	broken := filepath.Join(t.TempDir(), "broken")
	require.NoError(t, os.Mkdir(broken, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(broken, "mdbx.dat"), []byte("broken"), 0600))
	require.Error(t, CheckDatabase(broken, 4096))
}

/* FINISH ALPHA CHANGE 3484 -- verify page-size parsing and metadata inspection -- */
