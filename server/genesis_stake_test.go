package server

import (
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"

	/* START ALPHA CHANGE 3321 -- supply an explicit interval in the startup stake fixture -- */
	"time"

	"github.com/innovativebioresearch/innbc-chain/helper/common"
	/* FINISH ALPHA CHANGE 3321 -- supply an explicit interval in the startup stake fixture -- */

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2650 -- test genesis stake validation before server startup effects -- */

func TestGenesisMinimumStake_NewServer(t *testing.T) {
	cases := []struct {
		name     string
		registry *innbcbft.ValidatorRegistryConfig
		valid    bool
	}{
		{"missing registry", nil, false},
		{"missing minimum", &innbcbft.ValidatorRegistryConfig{}, false},
		{"negative", &innbcbft.ValidatorRegistryConfig{MinimumStake: big.NewInt(-1)}, false},
		{"zero", &innbcbft.ValidatorRegistryConfig{MinimumStake: big.NewInt(0)}, false},
		{"one wei", &innbcbft.ValidatorRegistryConfig{MinimumStake: big.NewInt(1)}, true},
		{"two wei", &innbcbft.ValidatorRegistryConfig{MinimumStake: big.NewInt(2)}, false},
		{"larger minimum", &innbcbft.ValidatorRegistryConfig{MinimumStake: big.NewInt(100)}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			logPath := filepath.Join(root, "node.log")
			dataPath := filepath.Join(root, "node")

			instance, err := NewServer(&Config{
				Chain: &chain.Chain{
					Params: &chain.Params{
						/* START ALPHA CHANGE 3545 -- reach startup minimum-stake validation with a valid chain ID -- */
						ChainID: 1982,
						/* FINISH ALPHA CHANGE 3545 -- reach startup minimum-stake validation with a valid chain ID -- */
						Forks: &chain.Forks{
							chain.London:    chain.NewFork(0),
							chain.LondonFix: chain.NewFork(0),
						},
						Engine: map[string]any{
							innbcbft.ConsensusName: &innbcbft.INNBCBFTConfig{
								/* START ALPHA CHANGE 3321 -- keep the startup stake check isolated -- */
								BlockTime: common.Duration{Duration: time.Second},
								/* FINISH ALPHA CHANGE 3321 -- keep the startup stake check isolated -- */
								/* START ALPHA CHANGE 3341 -- preserve startup stake coverage with required native-token configuration -- */
								NativeTokenConfig: innbcbft.DefaultTokenConfig,
								/* FINISH ALPHA CHANGE 3341 -- preserve startup stake coverage with required native-token configuration -- */
								/* START ALPHA CHANGE 3442 -- keep startup stake coverage independent of epoch validation -- */
								EpochSize: 2,
								/* FINISH ALPHA CHANGE 3442 -- keep startup stake coverage independent of epoch validation -- */
								ValidatorRegistryConfig: tc.registry,
							},
						},
					},
				},
				GRPCAddr: &net.TCPAddr{
					IP:   net.ParseIP("127.0.0.1"),
					Port: 9632,
				},
				LogFilePath: logPath,
				DataDir:     dataPath,
			})

			require.Nil(t, instance)

			if tc.valid {
				/*
					Passing the stake check reaches the next configuration
					boundary without starting a node.
				*/
				require.ErrorContains(t, err, "JSON-RPC configuration is missing")
			} else if tc.registry == nil {
				require.ErrorContains(t, err, "missing INNBC validator registry config")
			} else {
				require.ErrorIs(t, err, innbcbft.ErrInvalidGenesisMinimumStake)
			}

			for _, path := range []string{logPath, dataPath} {
				_, statErr := os.Stat(path)
				require.True(t, os.IsNotExist(statErr))
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2650 -- test genesis stake validation before server startup effects -- */

/* START ALPHA CHANGE 3341 -- reject missing native-token configuration before server startup side effects -- */

func TestNativeTokenConfig_NewServer(t *testing.T) {
	for _, mode := range []string{"omitted", "explicit_null"} {
		t.Run(mode, func(t *testing.T) {
			/* START ALPHA CHANGE 3413 -- reject missing native-token configuration using current genesis fixtures -- */
			config := loadCurrentTestGenesis(t)
			/* FINISH ALPHA CHANGE 3413 -- reject missing native-token configuration using current genesis fixtures -- */
			engine, ok := config.Params.Engine[innbcbft.ConsensusName].(map[string]any)
			require.True(t, ok)

			if mode == "omitted" {
				delete(engine, "nativeTokenConfig")
			} else {
				engine["nativeTokenConfig"] = nil
			}

			root := t.TempDir()
			dataPath := filepath.Join(root, "node")
			logPath := filepath.Join(root, "node.log")

			var instance *Server
			var err error
			require.NotPanics(t, func() {
				instance, err = NewServer(&Config{
					Chain: config,
					GRPCAddr: &net.TCPAddr{
						IP:   net.ParseIP("127.0.0.1"),
						Port: 9632,
					},
					DataDir:     dataPath,
					LogFilePath: logPath,
				})
			})

			require.Nil(t, instance)
			require.ErrorIs(t, err, innbcbft.ErrMissingNativeTokenConfig)
			require.EqualError(t, err,
				"invalid INNBCBFT configuration: nativeTokenConfig is required and must not be null")

			require.NoDirExists(t, dataPath)
			require.NoFileExists(t, logPath)

			entries, readErr := os.ReadDir(root)
			require.NoError(t, readErr)
			require.Empty(t, entries,
				"invalid native-token configuration created runtime files")
		})
	}
}

/* FINISH ALPHA CHANGE 3341 -- reject missing native-token configuration before server startup side effects -- */

/* START ALPHA CHANGE 3442 -- validate epoch sizes before fresh or existing-data startup effects -- */

func TestEpochSize_NewServer(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		omit    bool
		wantErr bool
	}{
		{name: "omitted", omit: true, wantErr: true},
		{name: "explicit_null", wantErr: true},
		{name: "zero", value: uint64(0), wantErr: true},
		{name: "one", value: uint64(1), wantErr: true},
		{name: "minimum", value: uint64(2)},
		{name: "production_default", value: uint64(30)},
	}

	for _, tc := range cases {
		for _, storage := range []string{"fresh", "existing_directory"} {
			t.Run(tc.name+"/"+storage, func(t *testing.T) {
				config := loadCurrentTestGenesis(t)
				engine, ok := config.Params.Engine[innbcbft.ConsensusName].(map[string]any)
				require.True(t, ok)

				if tc.omit {
					delete(engine, "epochSize")
				} else {
					engine["epochSize"] = tc.value
				}

				root := t.TempDir()
				dataPath := filepath.Join(root, "node")
				logPath := filepath.Join(root, "node.log")
				markerPath := filepath.Join(dataPath, "keep")
				marker := []byte("existing node data must be preserved")

				if storage == "existing_directory" {
					require.NoError(t, os.Mkdir(dataPath, 0700))
					require.NoError(t, os.WriteFile(markerPath, marker, 0600))
				}

				var instance *Server
				var err error
				require.NotPanics(t, func() {
					instance, err = NewServer(&Config{
						Chain: config,
						GRPCAddr: &net.TCPAddr{
							IP:   net.ParseIP("127.0.0.1"),
							Port: 0,
						},
						DataDir:     dataPath,
						LogFilePath: logPath,
					})
				})

				require.Nil(t, instance)

				if tc.wantErr {
					require.ErrorIs(t, err, innbcbft.ErrInvalidEpochSize)
					require.EqualError(t, err,
						"invalid INNBCBFT configuration: epochSize must be at least 2")
				} else {
					// Deliberately omit JSONRPC so acceptance of the epoch
					// reaches the next boundary without starting a server.
					require.ErrorContains(t, err,
						"JSON-RPC configuration is missing")
				}

				require.NoFileExists(t, logPath)

				if storage == "existing_directory" {
					contents, readErr := os.ReadFile(markerPath)
					require.NoError(t, readErr)
					require.Equal(t, marker, contents)

					entries, readErr := os.ReadDir(dataPath)
					require.NoError(t, readErr)
					require.Len(t, entries, 1)
					require.Equal(t, "keep", entries[0].Name())
				} else {
					require.NoDirExists(t, dataPath)
				}

				entries, readErr := os.ReadDir(root)
				require.NoError(t, readErr)
				if storage == "existing_directory" {
					require.Len(t, entries, 1)
					require.Equal(t, "node", entries[0].Name())
				} else {
					require.Empty(t, entries)
				}
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3442 -- validate epoch sizes before fresh or existing-data startup effects -- */
