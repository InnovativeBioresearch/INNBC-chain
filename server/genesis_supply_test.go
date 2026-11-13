/* START ALPHA CHANGE 3125 -- reject excessive genesis supply before startup side effects -- */
package server

import (
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisSupply_StartupRejectsBeforeSideEffects(t *testing.T) {
	maximum := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1),
	)
	root := t.TempDir()
	dataPath := filepath.Join(root, "data")
	logPath := filepath.Join(root, "node.log")

	config := &Config{
		Chain: &chain.Chain{
			Genesis: &chain.Genesis{
				Alloc: map[types.Address]*chain.GenesisAccount{
					types.StringToAddress("0xb301"): {Balance: maximum},
					types.StringToAddress("0xb302"): {Balance: big.NewInt(1)},
				},
			},
			Params: &chain.Params{
				ChainID: 1982,
				Forks:   chain.AllForksEnabled.Copy(),
			},
		},
		GRPCAddr:    &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9632},
		JSONRPC:     &JSONRPC{},
		DataDir:     dataPath,
		LogFilePath: logPath,
	}

	instance, err := NewServer(config)
	require.Nil(t, instance)
	require.ErrorContains(t, err, "unsupported chain configuration")
	require.ErrorContains(t, err,
		"initial native-token supply exceeds unsigned 256-bit range")

	for _, path := range []string{dataPath, logPath} {
		_, statErr := os.Stat(path)
		require.True(t, os.IsNotExist(statErr))
	}
}

/* FINISH ALPHA CHANGE 3125 -- reject excessive genesis supply before startup side effects -- */
