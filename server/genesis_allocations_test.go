/* START ALPHA CHANGE 3108 -- test malformed allocation rejection before server startup effects -- */
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

func TestGenesisAllocations_StartupRejectsBeforeSideEffects(t *testing.T) {
	address := types.StringToAddress("0xa201")

	for _, tc := range []struct {
		name    string
		genesis *chain.Genesis
		wantErr string
	}{
		{"missing_genesis", nil, "chain genesis config is missing"},
		{
			"null_account",
			&chain.Genesis{Alloc: map[types.Address]*chain.GenesisAccount{address: nil}},
			"account must not be null",
		},
		{
			"null_zero_address_account",
			&chain.Genesis{Alloc: map[types.Address]*chain.GenesisAccount{types.ZeroAddress: nil}},
			"account must not be null",
		},
		{
			"negative_balance",
			&chain.Genesis{Alloc: map[types.Address]*chain.GenesisAccount{
				address: {Balance: big.NewInt(-1)},
			}},
			"balance must be nonnegative",
		},
		{
			"oversized_balance",
			&chain.Genesis{Alloc: map[types.Address]*chain.GenesisAccount{
				address: {Balance: new(big.Int).Lsh(big.NewInt(1), 256)},
			}},
			"balance exceeds unsigned 256-bit range",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dataPath := filepath.Join(root, "data")
			logPath := filepath.Join(root, "node.log")
			config := &Config{
				Chain: &chain.Chain{
					Genesis: tc.genesis,
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
			require.ErrorContains(t, err, tc.wantErr)

			for _, path := range []string{dataPath, logPath} {
				_, statErr := os.Stat(path)
				require.True(t, os.IsNotExist(statErr))
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3108 -- test malformed allocation rejection before server startup effects -- */
