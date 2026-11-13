/* START ALPHA CHANGE 3107 -- test allocation validation before genesis file publication -- */
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

func TestGenesisAllocations_WriterRejectsBeforePublication(t *testing.T) {
	address := types.StringToAddress("0xa101")

	for _, tc := range []struct {
		name    string
		account *chain.GenesisAccount
		wantErr string
	}{
		{"null_account", nil, "account must not be null"},
		{
			"negative_balance",
			&chain.GenesisAccount{Balance: big.NewInt(-1)},
			"balance must be nonnegative",
		},
		{
			"oversized_balance",
			&chain.GenesisAccount{Balance: new(big.Int).Lsh(big.NewInt(1), 256)},
			"balance exceeds unsigned 256-bit range",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &chain.Chain{Genesis: &chain.Genesis{
				Alloc: map[types.Address]*chain.GenesisAccount{
					address: tc.account,
				},
			}}
			for _, existing := range []bool{false, true} {
				path := filepath.Join(t.TempDir(), "genesis.json")
				original := []byte("existing file must remain unchanged")
				if existing {
					require.NoError(t, os.WriteFile(path, original, 0o600))
				}

				err := WriteGenesisConfigToDisk(config, path)
				require.ErrorContains(t, err, tc.wantErr)
				require.ErrorContains(t, err, address.String())

				if existing {
					content, readErr := os.ReadFile(path)
					require.NoError(t, readErr)
					require.Equal(t, original, content)
				} else {
					_, statErr := os.Stat(path)
					require.True(t, os.IsNotExist(statErr))
				}
			}
		})
	}

	path := filepath.Join(t.TempDir(), "genesis.json")
	require.ErrorContains(t, WriteGenesisConfigToDisk(nil, path), "chain config is empty")
	require.ErrorContains(t, WriteGenesisConfigToDisk(&chain.Chain{}, path),
		"chain genesis config is missing")
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

func TestGenesisAllocations_WriterPreservesOmittedBalance(t *testing.T) {
	address := types.StringToAddress("0xa102")
	account := &chain.GenesisAccount{Code: []byte{0x00}, Nonce: 3}
	config := &chain.Chain{
		Genesis: &chain.Genesis{
			GasLimit:           chain.GenesisGasLimit,
			BaseFee:            1,
			BaseFeeEM:          chain.GenesisBaseFeeEM,
			BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
			Alloc: map[types.Address]*chain.GenesisAccount{
				address: account,
			},
		},
		Params: &chain.Params{
			ChainID: 1982,
			Forks:   chain.AllForksEnabled.Copy(),
			/* START ALPHA CHANGE 3325 -- preserve genesis publication coverage with valid clock configuration -- */
			Engine: map[string]any{
				"innbcbft": map[string]any{"blockTime": "1s"},
			},
			/* FINISH ALPHA CHANGE 3325 -- preserve genesis publication coverage with valid clock configuration -- */
		},
	}
	before, err := json.Marshal(config)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "genesis.json")
	require.NoError(t, WriteGenesisConfigToDisk(config, path))
	imported, err := chain.ImportFromFile(path)
	require.NoError(t, err)
	require.Nil(t, imported.Genesis.Alloc[address].Balance)
	require.Equal(t, account.Code, imported.Genesis.Alloc[address].Code)
	require.Equal(t, account.Nonce, imported.Genesis.Alloc[address].Nonce)

	after, err := json.Marshal(config)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Nil(t, account.Balance)
}

/* FINISH ALPHA CHANGE 3107 -- test allocation validation before genesis file publication -- */
