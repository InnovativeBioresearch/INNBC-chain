/* START ALPHA CHANGE 3535 -- reject nonpositive chain IDs before fresh or existing-directory startup effects -- */
package server

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/stretchr/testify/require"
)

func TestChainIDStartup_RejectsNonPositiveBeforeSideEffects(t *testing.T) {
	for _, id := range []int64{0, -1, math.MinInt64} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/existing=%t", id, existing), func(t *testing.T) {
				root := t.TempDir()
				dataPath := filepath.Join(root, "data")
				logPath := filepath.Join(root, "node.log")
				markerPath := filepath.Join(dataPath, "preserve")
				marker := []byte("existing data must remain unchanged")

				if existing {
					require.NoError(t, os.Mkdir(dataPath, 0700))
					require.NoError(t, os.WriteFile(markerPath, marker, 0600))
				}

				config := &Config{
					Chain: &chain.Chain{
						Params: &chain.Params{
							ChainID: id,
							Forks:   chain.AllForksEnabled.Copy(),
						},
					},
					DataDir:     dataPath,
					LogFilePath: logPath,
				}

				var instance *Server
				var err error
				require.NotPanics(t, func() {
					instance, err = NewServer(config)
				})

				require.Nil(t, instance)
				require.EqualError(t, err, fmt.Sprintf(
					"unsupported chain configuration: chain ID must be positive, got %d",
					id,
				))
				require.Equal(t, id, config.Chain.Params.ChainID)
				require.NoFileExists(t, logPath)

				if existing {
					actual, readErr := os.ReadFile(markerPath)
					require.NoError(t, readErr)
					require.Equal(t, marker, actual)

					entries, readErr := os.ReadDir(dataPath)
					require.NoError(t, readErr)
					require.Len(t, entries, 1)
					require.Equal(t, "preserve", entries[0].Name())
				} else {
					require.NoDirExists(t, dataPath)
				}

				entries, readErr := os.ReadDir(root)
				require.NoError(t, readErr)
				if existing {
					require.Len(t, entries, 1)
					require.Equal(t, "data", entries[0].Name())
				} else {
					require.Empty(t, entries)
				}
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3535 -- reject nonpositive chain IDs before fresh or existing-directory startup effects -- */
