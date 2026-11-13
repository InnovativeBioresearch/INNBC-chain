/* START ALPHA CHANGE 3124 -- reject excessive aggregate supply before genesis file publication -- */
package helper

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisSupply_WriterRejectsBeforePublication(t *testing.T) {
	maximum := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1),
	)
	config := &chain.Chain{Genesis: &chain.Genesis{
		Alloc: map[types.Address]*chain.GenesisAccount{
			types.StringToAddress("0xb201"): {Balance: maximum},
			types.StringToAddress("0xb202"): {Balance: big.NewInt(1)},
		},
	}}
	before, err := json.Marshal(config)
	require.NoError(t, err)

	for _, existing := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "genesis.json")
		original := []byte("existing genesis must remain unchanged")
		if existing {
			require.NoError(t, os.WriteFile(path, original, 0o600))
		}

		err := WriteGenesisConfigToDisk(config, path)
		require.ErrorContains(t, err,
			"initial native-token supply exceeds unsigned 256-bit range")

		if existing {
			content, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Equal(t, original, content)
		} else {
			_, statErr := os.Stat(path)
			require.True(t, os.IsNotExist(statErr))
		}
	}

	after, err := json.Marshal(config)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

/* FINISH ALPHA CHANGE 3124 -- reject excessive aggregate supply before genesis file publication -- */
