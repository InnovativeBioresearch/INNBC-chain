/* START ALPHA CHANGE 3144 -- test retention overflow rejection before server startup effects -- */
package server

import (
	"math"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/stretchr/testify/require"
)

func TestRecentHistorySegments_StartupRejectsBeforeSideEffects(t *testing.T) {
	for _, retention := range []uint64{
		uint64(math.MaxInt) + 1,
		uint64(1) << 63,
		math.MaxUint64,
	} {
		root := t.TempDir()
		dataPath := filepath.Join(root, "data")
		logPath := filepath.Join(root, "node.log")

		config := &Config{
			Chain: &chain.Chain{
				Genesis: &chain.Genesis{},
				Params: &chain.Params{
					ChainID: 1982,
					Forks:   chain.AllForksEnabled.Copy(),
				},
			},
			GRPCAddr: &net.TCPAddr{
				IP: net.ParseIP("127.0.0.1"), Port: 9632,
			},
			JSONRPC:     &JSONRPC{},
			DataDir:     dataPath,
			LogFilePath: logPath,
			History: &HistoryConfig{
				Mode:                      "recent",
				RetentionBlocks:           retention,
				SegmentBlocks:             1,
				MDBXMapSizeGB:             128,
				SnapshotSourceDir:         filepath.Join(root, "snapshots"),
				SnapshotBootstrapRequired: true,
			},
		}

		instance, err := NewServer(config)
		require.Nil(t, instance)
		require.ErrorContains(t, err, "invalid recent-history configuration")
		require.ErrorContains(t, err, "exceeds platform maximum")

		entries, readErr := os.ReadDir(root)
		require.NoError(t, readErr)
		require.Empty(t, entries)

		for _, path := range []string{dataPath, logPath} {
			_, statErr := os.Stat(path)
			require.ErrorIs(t, statErr, os.ErrNotExist)
		}
	}
}

/* FINISH ALPHA CHANGE 3144 -- test retention overflow rejection before server startup effects -- */
