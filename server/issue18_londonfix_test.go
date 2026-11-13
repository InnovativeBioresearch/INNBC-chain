/* START ALPHA CHANGE 2396 -- test London correction startup and replay boundaries -- */
package server

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/forkmanager"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/state"
)

func issue18UnsupportedForkSchedules() map[string]*chain.Forks {
	return map[string]*chain.Forks{
		"missing forks": nil,
		"empty forks":   {},
		"missing correction": {
			chain.London: chain.NewFork(0),
		},
		"delayed correction": {
			chain.London:    chain.NewFork(0),
			chain.LondonFix: chain.NewFork(1),
		},
	}
}

func TestIssue18_NewServerRejectsLegacyScheduleBeforeStartup(t *testing.T) {
	for name, forks := range issue18UnsupportedForkSchedules() {
		t.Run(name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "node.log")

			instance, err := NewServer(&Config{
				Chain: &chain.Chain{
					Params: &chain.Params{Forks: forks},
				},
				LogFilePath: logPath,
			})

			require.ErrorContains(
				t, err,
				"INNBC requires londonfix activation at block 0",
			)
			require.Nil(t, instance)

			_, statErr := os.Stat(logPath)
			require.True(t, os.IsNotExist(statErr))
		})
	}
}

func TestIssue18_ForkManagerRejectsLegacyScheduleAndSelectsV2(t *testing.T) {
	/*
		Fork handlers are process-global. This test must not run
		in parallel with other fork-manager initialization tests.
	*/
	manager := forkmanager.GetInstance()
	t.Cleanup(manager.Clear)

	/* START ALPHA CHANGE 3413 -- validate fork selection with current genesis configuration -- */
	config := loadCurrentTestGenesis(t)
	/* FINISH ALPHA CHANGE 3413 -- validate fork selection with current genesis configuration -- */
	require.NoError(t, initForkManager(innbcbft.ConsensusName, config))

	activeHandler := manager.GetHandler(state.LondonFixHandler, 0)
	require.IsType(t, &state.LondonFixForkV2{}, activeHandler)

	for name, forks := range issue18UnsupportedForkSchedules() {
		t.Run(name, func(t *testing.T) {
			candidate := *config
			params := *config.Params
			params.Forks = forks
			candidate.Params = &params

			err := initForkManager(innbcbft.ConsensusName, &candidate)
			require.ErrorContains(
				t, err,
				"INNBC requires londonfix activation at block 0",
			)

			require.Same(
				t,
				activeHandler,
				manager.GetHandler(state.LondonFixHandler, 0),
			)
		})
	}

	for _, number := range []uint64{0, 1, 100, math.MaxUint64} {
		require.IsType(
			t,
			&state.LondonFixForkV2{},
			state.GetLondonFixHandler(number),
		)
	}
}

func TestIssue18_TrieRebuildRejectsLegacyScheduleBeforeOpeningDatabases(t *testing.T) {
	for name, forks := range issue18UnsupportedForkSchedules() {
		t.Run(name, func(t *testing.T) {
			/* START ALPHA CHANGE 3413 -- preserve replay-boundary coverage with current configuration -- */
			config := loadCurrentTestGenesis(t)
			/* FINISH ALPHA CHANGE 3413 -- preserve replay-boundary coverage with current configuration -- */
			config.Params.Forks = forks

			root := t.TempDir()
			dataDir := filepath.Join(root, "node")
			chainPath := filepath.Join(root, "genesis.json")

			require.NoError(
				t,
				os.MkdirAll(datadir.ChainData(dataDir), 0o700),
			)

			payload, err := json.Marshal(config)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(chainPath, payload, 0o600))

			result, err := RebuildTrieDatabase(
				context.Background(),
				TrieRebuildConfig{
					ChainPath:     chainPath,
					DataDir:       dataDir,
					MDBXMapSizeGB: 128,
				},
			)

			require.ErrorContains(
				t, err,
				"INNBC requires londonfix activation at block 0",
			)
			require.Nil(t, result)

			entries, err := os.ReadDir(datadir.ChainData(dataDir))
			require.NoError(t, err)
			require.Empty(t, entries)

			onDisk, err := os.ReadFile(chainPath)
			require.NoError(t, err)
			require.Equal(t, payload, onDisk)
		})
	}
}

/* FINISH ALPHA CHANGE 2396 -- test London correction startup and replay boundaries -- */
