/* START ALPHA CHANGE 3490 -- verify startup and snapshot mismatch preservation -- */
package server

import (
	/* START ALPHA CHANGE 3495 -- provide valid startup consensus parameters -- */
	"math/big"
	/* FINISH ALPHA CHANGE 3495 -- provide valid startup consensus parameters -- */
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	/* START ALPHA CHANGE 3500 -- exercise recovery defaults using actual MDBX databases -- */
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	/* FINISH ALPHA CHANGE 3500 -- exercise recovery defaults using actual MDBX databases -- */
	/* START ALPHA CHANGE 3495 -- validate startup fixture consensus configuration -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	/* FINISH ALPHA CHANGE 3495 -- validate startup fixture consensus configuration -- */
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	"github.com/stretchr/testify/require"
)

func TestMDBXPageSize_StartupMismatch(t *testing.T) {
	logger := hclog.NewNullLogger()
	source := filepath.Join(t.TempDir(), "source")
	chainPath := filepath.Join(t.TempDir(), "genesis.json")
	chainConfig, _, _ := writeSnapshotIntegrationSource(t, source, chainPath, 4, logger)

	/* START ALPHA CHANGE 3495 -- satisfy consensus validation before testing page incompatibility -- */
	engine, ok := chainConfig.Params.Engine[innbcbft.ConsensusName].(map[string]any)
	require.True(t, ok)

	engine["validatorRegistryConfig"] = &innbcbft.ValidatorRegistryConfig{
		MinimumStake: big.NewInt(1),
	}
	engine["nativeTokenConfig"] = innbcbft.DefaultTokenConfig
	engine["epochSize"] = uint64(2)

	_, configErr := innbcbft.GetINNBCBFTConfig(chainConfig)
	require.NoError(t, configErr)
	/* FINISH ALPHA CHANGE 3495 -- satisfy consensus validation before testing page incompatibility -- */

	data := filepath.Join(datadir.Blockchain(source), "mdbx.dat")
	before, err := os.ReadFile(data)
	require.NoError(t, err)
	logPath := filepath.Join(t.TempDir(), "must-not-exist.log")

	instance, err := NewServer(&Config{
		Chain:       chainConfig,
		DataDir:     source,
		LogFilePath: logPath,
		GRPCAddr:    &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9632},
		JSONRPC:     &JSONRPC{},
		History: &HistoryConfig{
			Mode:              "full",
			MDBXMapSizeGB:     1,
			MDBXPageSizeBytes: 8192,
		},
	})
	require.ErrorIs(t, err, mdbxconfig.ErrPageSizeMismatch)
	require.Nil(t, instance)
	require.NoFileExists(t, logPath)

	after, err := os.ReadFile(data)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestMDBXPageSize_SnapshotMismatchAndRecovery(t *testing.T) {
	logger := hclog.NewNullLogger()
	source := filepath.Join(t.TempDir(), "source")
	chainPath := filepath.Join(t.TempDir(), "genesis.json")
	chainConfig, _, _ := writeSnapshotIntegrationSource(t, source, chainPath, 4, logger)

	for _, kind := range []snapshotbuilder.Kind{
		snapshotbuilder.KindCheckpoint, snapshotbuilder.KindSegmented,
	} {
		t.Run(string(kind), func(t *testing.T) {
			output := t.TempDir()
			_, err := snapshotbuilder.Build(snapshotbuilder.Config{
				Kind:              kind,
				ChainPath:         chainPath,
				SourceDataDir:     source,
				OutputRoot:        output,
				RetentionBlocks:   2,
				SegmentBlocks:     1,
				MDBXMapSizeGB:     1,
				MDBXPageSizeBytes: 8192,
				Logger:            logger,
			})
			require.NoError(t, err)

			mode := "full"
			required := true
			if kind == snapshotbuilder.KindSegmented {
				mode = "recent"
				required = false
			}
			target := filepath.Join(t.TempDir(), "target")
			instance := &Server{
				logger: logger,
				config: &Config{
					Chain:   chainConfig,
					DataDir: target,
					History: &HistoryConfig{
						Mode:                      mode,
						RetentionBlocks:           2,
						SegmentBlocks:             1,
						SnapshotSourceDir:         filepath.Join(output, string(kind)),
						SnapshotBootstrapRequired: required,
						MDBXMapSizeGB:             1,
						MDBXPageSizeBytes:         4096,
					},
				},
			}
			_, err = instance.activateConfiguredSnapshot()
			require.ErrorIs(t, err, mdbxconfig.ErrPageSizeMismatch)
			require.NoDirExists(t, datadir.ChainData(target))
			require.NoDirExists(t, filepath.Join(target, snapshotActivationStageName))

			instance.config.History.MDBXPageSizeBytes = 8192
			_, err = instance.activateConfiguredSnapshot()
			require.NoError(t, err)
			require.NoError(t, mdbxconfig.CheckChainData(datadir.ChainData(target), 8192))

			backup := filepath.Join(target, snapshotActivationBackupName)
			require.NoError(t, os.Mkdir(backup, 0700))
			marker := filepath.Join(backup, "preserve")
			require.NoError(t, os.WriteFile(marker, []byte("preserve"), 0600))
			require.NoError(t, createSnapshotActivationMarker(target))

			instance.config.History.MDBXPageSizeBytes = 4096
			require.ErrorIs(t, instance.recoverInterruptedSnapshotActivation(),
				mdbxconfig.ErrPageSizeMismatch)
			require.FileExists(t, marker)
			require.DirExists(t, datadir.ChainData(target))
		})
	}
}

/* START ALPHA CHANGE 3500 -- verify omitted history defaults preserve incompatible recovery data -- */
func TestMDBXPageSize_RecoveryWithoutHistory(t *testing.T) {
	for _, storedSize := range []uint64{4096, 8192} {
		t.Run(mdbxconfig.PageSize(storedSize).String(), func(t *testing.T) {
			dataDir := t.TempDir()
			backup := filepath.Join(dataDir, snapshotActivationBackupName)
			databasePath := filepath.Join(backup, "blockchain")
			logger := hclog.NewNullLogger()

			db, err := mdbxstorage.NewMDBXStorageWithMapSize(
				databasePath, logger, 64*1024*1024, storedSize,
			)
			require.NoError(t, err)
			require.NoError(t, db.Close())

			before, err := os.ReadFile(filepath.Join(databasePath, "mdbx.dat"))
			require.NoError(t, err)

			stage := filepath.Join(dataDir, snapshotActivationStageName)
			require.NoError(t, os.Mkdir(stage, 0700))
			stageFile := filepath.Join(stage, "preserve")
			require.NoError(t, os.WriteFile(stageFile, []byte("staging"), 0600))
			require.NoError(t, createSnapshotActivationMarker(dataDir))

			instance := &Server{
				config: &Config{DataDir: dataDir},
				logger: logger,
			}

			err = instance.recoverInterruptedSnapshotActivation()
			if storedSize == 8192 {
				require.ErrorIs(t, err, mdbxconfig.ErrPageSizeMismatch)
				require.DirExists(t, backup)
				require.FileExists(t, stageFile)
				require.FileExists(t,
					filepath.Join(dataDir, snapshotActivationMarkerName))
				require.NoDirExists(t, datadir.ChainData(dataDir))

				after, readErr := os.ReadFile(filepath.Join(databasePath, "mdbx.dat"))
				require.NoError(t, readErr)
				require.Equal(t, before, after)
				return
			}

			require.NoError(t, err)
			require.NoDirExists(t, backup)
			require.NoDirExists(t, stage)
			require.NoFileExists(t,
				filepath.Join(dataDir, snapshotActivationMarkerName))
			require.NoError(t, mdbxconfig.CheckChainData(
				datadir.ChainData(dataDir), 4096,
			))

			after, readErr := os.ReadFile(filepath.Join(
				datadir.Blockchain(dataDir), "mdbx.dat",
			))
			require.NoError(t, readErr)
			require.Equal(t, before, after)
		})
	}
}

/* FINISH ALPHA CHANGE 3500 -- verify omitted history defaults preserve incompatible recovery data -- */

/* FINISH ALPHA CHANGE 3490 -- verify startup and snapshot mismatch preservation -- */
