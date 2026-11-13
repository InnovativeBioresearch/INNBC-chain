package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/innovativebioresearch/innbc-chain/types"
)

/* START ALPHA CHANGE 932 -- test local history bootstrap config validation -- */

func TestInitHistoryConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		history     *config.History
		expectedErr string
	}{
		{
			name: "nil history uses defaults",
		},
		{
			name: "full mode with valid defaults",
			history: &config.History{
				Mode:            "full",
				RetentionBlocks: 43200,
				SegmentBlocks:   1800,
				/* START ALPHA CHANGE 1730 -- remove obsolete snapshot interval fixture -- */
				/* FINISH ALPHA CHANGE 1730 -- remove obsolete snapshot interval fixture -- */
				MDBXMapSizeGB: config.DefaultMDBXMapSizeGB,
			},
		},
		{
			name: "recent mode with snapshot source is valid",
			history: &config.History{
				Mode:            "recent",
				RetentionBlocks: 10,
				SegmentBlocks:   5,
				/* START ALPHA CHANGE 1730 -- remove obsolete snapshot interval fixture -- */
				/* FINISH ALPHA CHANGE 1730 -- remove obsolete snapshot interval fixture -- */
				SnapshotSourceDir: "snapshots",
				MDBXMapSizeGB:     config.DefaultMDBXMapSizeGB,
			},
		},
		/* START ALPHA CHANGE 1666 -- accept a dormant checkpoint source in full mode -- */
		{
			name: "full mode with dormant checkpoint snapshot source is valid",
			history: &config.History{
				Mode:            "full",
				RetentionBlocks: 10,
				SegmentBlocks:   5,
				/* START ALPHA CHANGE 1741 -- remove obsolete dormant-source snapshot interval -- */
				/* FINISH ALPHA CHANGE 1741 -- remove obsolete dormant-source snapshot interval -- */
				SnapshotSourceDir: "snapshots",
				MDBXMapSizeGB:     config.DefaultMDBXMapSizeGB,
			},
		},
		/* FINISH ALPHA CHANGE 1666 -- accept a dormant checkpoint source in full mode -- */
		{
			name: "invalid mode is rejected",
			history: &config.History{
				Mode:            "archive",
				RetentionBlocks: 10,
				SegmentBlocks:   5,
				/* START ALPHA CHANGE 1730 -- remove obsolete snapshot interval fixture -- */
				/* FINISH ALPHA CHANGE 1730 -- remove obsolete snapshot interval fixture -- */
			},
			expectedErr: `invalid history mode "archive": expected full or recent`,
		},
		{
			name: "zero retention is rejected",
			history: &config.History{
				Mode:          "recent",
				SegmentBlocks: 5,
				/* START ALPHA CHANGE 1742 -- remove obsolete zero-retention snapshot interval -- */
				/* FINISH ALPHA CHANGE 1742 -- remove obsolete zero-retention snapshot interval -- */
			},
			expectedErr: "history retention blocks must be greater than zero",
		},
		{
			name: "zero segment size is rejected",
			history: &config.History{
				Mode:            "recent",
				RetentionBlocks: 10,
				/* START ALPHA CHANGE 1743 -- remove obsolete zero-segment snapshot interval -- */
				/* FINISH ALPHA CHANGE 1743 -- remove obsolete zero-segment snapshot interval -- */
			},
			expectedErr: "history segment blocks must be greater than zero",
		},
		{
			name: "retention must contain a complete segment",
			history: &config.History{
				Mode:            "recent",
				RetentionBlocks: 5,
				SegmentBlocks:   10,
				/* START ALPHA CHANGE 1744 -- remove obsolete minimum-segment snapshot interval -- */
				/* FINISH ALPHA CHANGE 1744 -- remove obsolete minimum-segment snapshot interval -- */
			},
			expectedErr: "history retention blocks must be at least one segment",
		},
		{
			name: "retention must be divisible by segment size",
			history: &config.History{
				Mode:            "recent",
				RetentionBlocks: 10,
				SegmentBlocks:   6,
				/* START ALPHA CHANGE 1745 -- remove obsolete geometry snapshot interval -- */
				/* FINISH ALPHA CHANGE 1745 -- remove obsolete geometry snapshot interval -- */
				MDBXMapSizeGB: config.DefaultMDBXMapSizeGB,
			},
			expectedErr: "history retention blocks must be divisible by history segment blocks",
		},
		/* START ALPHA CHANGE 1729 -- remove obsolete snapshot interval validation test -- */
		/* FINISH ALPHA CHANGE 1729 -- remove obsolete snapshot interval validation test -- */
		/* START ALPHA CHANGE 1803 -- reject a zero unified MDBX map size -- */
		{
			name: "zero MDBX map size is rejected",
			history: &config.History{
				Mode:            "recent",
				RetentionBlocks: 10,
				SegmentBlocks:   5,
			},
			expectedErr: "MDBX map size must be greater than zero",
		},
		/* FINISH ALPHA CHANGE 1803 -- reject a zero unified MDBX map size -- */
		{
			name: "strict snapshot bootstrap with recent source is valid",
			history: &config.History{
				Mode:            "recent",
				RetentionBlocks: 10,
				SegmentBlocks:   5,
				/* START ALPHA CHANGE 1746 -- remove obsolete required-recent snapshot interval -- */
				/* FINISH ALPHA CHANGE 1746 -- remove obsolete required-recent snapshot interval -- */
				SnapshotSourceDir:         "snapshots",
				SnapshotBootstrapRequired: true,
				MDBXMapSizeGB:             config.DefaultMDBXMapSizeGB,
			},
		},
		/* START ALPHA CHANGE 1667 -- accept full-mode Plan B checkpoint bootstrap -- */
		{
			name: "strict snapshot bootstrap with full checkpoint source is valid",
			history: &config.History{
				Mode:            "full",
				RetentionBlocks: 10,
				SegmentBlocks:   5,
				/* START ALPHA CHANGE 1747 -- remove obsolete required-checkpoint snapshot interval -- */
				/* FINISH ALPHA CHANGE 1747 -- remove obsolete required-checkpoint snapshot interval -- */
				SnapshotSourceDir:         "snapshots",
				SnapshotBootstrapRequired: true,
				MDBXMapSizeGB:             config.DefaultMDBXMapSizeGB,
			},
		},
		/* FINISH ALPHA CHANGE 1667 -- accept full-mode Plan B checkpoint bootstrap -- */
		{
			name: "strict recent snapshot bootstrap requires source directory",
			history: &config.History{
				Mode:            "recent",
				RetentionBlocks: 10,
				SegmentBlocks:   5,
				/* START ALPHA CHANGE 1748 -- remove obsolete missing-recent-source snapshot interval -- */
				/* FINISH ALPHA CHANGE 1748 -- remove obsolete missing-recent-source snapshot interval -- */
				SnapshotBootstrapRequired: true,
				MDBXMapSizeGB:             config.DefaultMDBXMapSizeGB,
			},
			expectedErr: "snapshot bootstrap required needs a snapshot source directory",
		},
		/* START ALPHA CHANGE 1668 -- require a source for full-mode Plan B -- */
		{
			name: "strict full checkpoint bootstrap requires source directory",
			history: &config.History{
				Mode:            "full",
				RetentionBlocks: 10,
				SegmentBlocks:   5,
				/* START ALPHA CHANGE 1749 -- remove obsolete missing-checkpoint-source snapshot interval -- */
				/* FINISH ALPHA CHANGE 1749 -- remove obsolete missing-checkpoint-source snapshot interval -- */
				SnapshotBootstrapRequired: true,
				MDBXMapSizeGB:             config.DefaultMDBXMapSizeGB,
			},
			expectedErr: "snapshot bootstrap required needs a snapshot source directory",
		},
		/* FINISH ALPHA CHANGE 1668 -- require a source for full-mode Plan B -- */
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			params := &serverParams{
				rawConfig: &config.Config{
					History: test.history,
				},
			}

			err := params.initHistoryConfig()
			if test.expectedErr == "" {
				require.NoError(t, err)
				require.NotNil(t, params.rawConfig.History)

				/* START ALPHA CHANGE 1731 -- remove obsolete producer default assertion -- */
				/* FINISH ALPHA CHANGE 1731 -- remove obsolete producer default assertion -- */

				return
			}

			require.Error(t, err)
			require.Equal(t, test.expectedErr, err.Error())
		})
	}
}

/* START ALPHA CHANGE 1767 -- test mode-specific history segment geometry -- */
func TestInitHistoryConfigNormalizesSegmentGeometryByMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		mode              string
		retentionBlocks   uint64
		segmentBlocks     uint64
		expectedRetention uint64
		expectedSegment   uint64
	}{
		{
			name:              "full mode disables segmented retention",
			mode:              "full",
			retentionBlocks:   43200,
			segmentBlocks:     1800,
			expectedRetention: 0,
			expectedSegment:   0,
		},
		{
			name:              "recent mode preserves segmented retention",
			mode:              "recent",
			retentionBlocks:   43200,
			segmentBlocks:     1800,
			expectedRetention: 43200,
			expectedSegment:   1800,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			params := &serverParams{
				rawConfig: &config.Config{
					History: &config.History{
						Mode:            test.mode,
						RetentionBlocks: test.retentionBlocks,
						SegmentBlocks:   test.segmentBlocks,
						MDBXMapSizeGB:   config.DefaultMDBXMapSizeGB,
					},
				},
			}

			require.NoError(t, params.initHistoryConfig())
			require.Equal(
				t,
				test.expectedRetention,
				params.rawConfig.History.RetentionBlocks,
			)
			require.Equal(
				t,
				test.expectedSegment,
				params.rawConfig.History.SegmentBlocks,
			)
		})
	}
}

/* FINISH ALPHA CHANGE 1767 -- test mode-specific history segment geometry -- */

/* START ALPHA CHANGE 2056 -- test configured and inherited block gas target validation -- */
func TestInitBlockGasTargetCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rawTarget     string
		expectedError bool
	}{
		{
			name:      "zero target remains valid",
			rawTarget: "0x0",
		},
		{
			name:          "target below mandatory transaction requirement is rejected",
			rawTarget:     "999999",
			expectedError: true,
		},
		{
			name:      "exact mandatory transaction target is accepted",
			rawTarget: "1000000",
		},
		{
			name:      "generated INNBC target is accepted",
			rawTarget: "5242880",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			params := &serverParams{
				rawConfig: &config.Config{
					BlockGasTarget: test.rawTarget,
				},
			}

			err := params.initBlockGasTarget()
			if test.expectedError {
				require.ErrorContains(
					t,
					err,
					"block gas target must be zero or at least mandatory state transaction gas limit 1000000",
				)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestInitGenesisConfigValidatesEffectiveBlockGasTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		genesisTarget       uint64
		overrideTarget      uint64
		expectedError       bool
		expectedFinalTarget uint64
	}{
		{
			name:           "rejects below-floor target inherited with zero override",
			genesisTarget:  types.StateTransactionGasLimit - 1,
			overrideTarget: 0,
			expectedError:  true,
		},
		/* START ALPHA CHANGE 2059 -- make imported-target precedence and inheritance explicit -- */
		{
			name:           "rejects invalid imported target before applying valid override",
			genesisTarget:  types.StateTransactionGasLimit - 1,
			overrideTarget: 2_000_000,
			expectedError:  true,
		},
		/* FINISH ALPHA CHANGE 2059 -- make imported-target precedence and inheritance explicit -- */
		{
			name:                "zero inherited target remains zero",
			genesisTarget:       0,
			overrideTarget:      0,
			expectedFinalTarget: 0,
		},
		/* START ALPHA CHANGE 2059 -- make imported-target precedence and inheritance explicit -- */
		{
			name:                "valid nonzero inherited target remains unchanged with zero override",
			genesisTarget:       2_000_000,
			overrideTarget:      0,
			expectedFinalTarget: 2_000_000,
		},
		/* FINISH ALPHA CHANGE 2059 -- make imported-target precedence and inheritance explicit -- */
		{
			name:                "exact mandatory transaction override is accepted",
			genesisTarget:       0,
			overrideTarget:      types.StateTransactionGasLimit,
			expectedFinalTarget: types.StateTransactionGasLimit,
		},
		{
			name:           "below-floor override is rejected",
			genesisTarget:  types.StateTransactionGasLimit,
			overrideTarget: types.StateTransactionGasLimit - 1,
			expectedError:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			chainConfig := &chain.Chain{
				Name: "gas-target-test",
				Genesis: &chain.Genesis{
					GasLimit:           5_242_880,
					BaseFee:            1,
					BaseFeeEM:          chain.GenesisBaseFeeEM,
					BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
				},
				Params: &chain.Params{
					/* START ALPHA CHANGE 3542 -- preserve imported gas-target coverage with a valid chain ID -- */
					ChainID: 1982,
					/* FINISH ALPHA CHANGE 3542 -- preserve imported gas-target coverage with a valid chain ID -- */
					/* START ALPHA CHANGE 2394 -- retain a supported fork schedule in imported gas target fixtures -- */
					Forks: chain.AllForksEnabled.Copy(),
					/* FINISH ALPHA CHANGE 2394 -- retain a supported fork schedule in imported gas target fixtures -- */
					/* START ALPHA CHANGE 3325 -- preserve imported gas-target validation with valid clock configuration -- */
					Engine: map[string]any{
						"innbcbft": map[string]any{"blockTime": "1s"},
					},
					/* FINISH ALPHA CHANGE 3325 -- preserve imported gas-target validation with valid clock configuration -- */
					BlockGasTarget: test.genesisTarget,
				},
			}

			encodedConfig, err := json.Marshal(chainConfig)
			require.NoError(t, err)

			genesisPath := filepath.Join(t.TempDir(), "genesis.json")
			require.NoError(t, os.WriteFile(genesisPath, encodedConfig, 0o600))

			params := &serverParams{
				rawConfig: &config.Config{
					GenesisPath: genesisPath,
				},
				blockGasTarget: test.overrideTarget,
			}

			err = params.initGenesisConfig()
			if test.expectedError {
				require.ErrorContains(
					t,
					err,
					"block gas target must be zero or at least mandatory state transaction gas limit 1000000",
				)

				return
			}

			require.NoError(t, err)
			require.Equal(
				t,
				test.expectedFinalTarget,
				params.genesisConfig.Params.BlockGasTarget,
			)
		})
	}
}

/* FINISH ALPHA CHANGE 2056 -- test configured and inherited block gas target validation -- */

/* FINISH ALPHA CHANGE 932 -- test local history bootstrap config validation -- */
