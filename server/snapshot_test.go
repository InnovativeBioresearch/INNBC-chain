/* START ALPHA CHANGE 1702 -- test whole-chain-data snapshot activation -- */
package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestSnapshotPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		history       *HistoryConfig
		hasLocalChain bool
		attempt       bool
		required      bool
		kind          snapshotbuilder.Kind
	}{
		{
			name: "recent optional empty node",
			history: &HistoryConfig{
				Mode: "recent",
			},
			attempt: true,
			kind:    snapshotbuilder.KindSegmented,
		},
		{
			name: "recent required empty node",
			history: &HistoryConfig{
				Mode:                      "recent",
				SnapshotBootstrapRequired: true,
			},
			attempt:  true,
			required: true,
			kind:     snapshotbuilder.KindSegmented,
		},
		{
			name: "recent required existing node",
			history: &HistoryConfig{
				Mode:                      "recent",
				SnapshotBootstrapRequired: true,
			},
			hasLocalChain: true,
			attempt:       true,
			kind:          snapshotbuilder.KindSegmented,
		},
		{
			name: "full optional ignores snapshot",
			history: &HistoryConfig{
				Mode: "full",
			},
			kind: snapshotbuilder.KindCheckpoint,
		},
		{
			name: "full required empty node",
			history: &HistoryConfig{
				Mode:                      "full",
				SnapshotBootstrapRequired: true,
			},
			attempt:  true,
			required: true,
			kind:     snapshotbuilder.KindCheckpoint,
		},
		{
			name: "full required existing node continues locally",
			history: &HistoryConfig{
				Mode:                      "full",
				SnapshotBootstrapRequired: true,
			},
			hasLocalChain: true,
			kind:          snapshotbuilder.KindCheckpoint,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			policy, err := snapshotPolicy(
				test.history,
				test.hasLocalChain,
			)
			require.NoError(t, err)
			require.Equal(t, test.attempt, policy.attempt)
			require.Equal(t, test.required, policy.required)
			require.Equal(t, test.kind, policy.expectedKind)
		})
	}
}

/* START ALPHA CHANGE 1963 -- test durable and ambiguity-safe snapshot recovery -- */
func TestRecoverInterruptedSnapshotActivation(t *testing.T) {
	t.Parallel()

	t.Run("restores backup when live chain data is absent", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()
		backup := filepath.Join(dataDir, snapshotActivationBackupName)
		require.NoError(t, os.MkdirAll(backup, 0770))
		require.NoError(t, os.WriteFile(
			filepath.Join(backup, "marker"),
			[]byte("old"),
			0600,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(dataDir, snapshotActivationMarkerName),
			nil,
			0600,
		))

		instance := &Server{
			config: &Config{DataDir: dataDir},
			logger: hclog.NewNullLogger(),
		}

		require.NoError(t, instance.recoverInterruptedSnapshotActivation())
		require.FileExists(t, filepath.Join(datadir.ChainData(dataDir), "marker"))
		require.NoDirExists(t, backup)
		require.NoFileExists(
			t,
			filepath.Join(dataDir, snapshotActivationMarkerName),
		)
	})

	t.Run("completes a marked live and backup activation", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()
		require.NoError(t, os.MkdirAll(datadir.ChainData(dataDir), 0770))

		backup := filepath.Join(dataDir, snapshotActivationBackupName)
		require.NoError(t, os.MkdirAll(backup, 0770))
		require.NoError(t, os.WriteFile(
			filepath.Join(dataDir, snapshotActivationMarkerName),
			nil,
			0600,
		))

		instance := &Server{
			config: &Config{DataDir: dataDir},
			logger: hclog.NewNullLogger(),
		}

		require.NoError(t, instance.recoverInterruptedSnapshotActivation())
		require.DirExists(t, datadir.ChainData(dataDir))
		require.NoDirExists(t, backup)
		require.NoFileExists(
			t,
			filepath.Join(dataDir, snapshotActivationMarkerName),
		)
	})

	t.Run("preserves ambiguous live and backup data", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()
		live := datadir.ChainData(dataDir)
		backup := filepath.Join(dataDir, snapshotActivationBackupName)

		require.NoError(t, os.MkdirAll(live, 0770))
		require.NoError(t, os.MkdirAll(backup, 0770))

		instance := &Server{
			config: &Config{DataDir: dataDir},
			logger: hclog.NewNullLogger(),
		}

		err := instance.recoverInterruptedSnapshotActivation()
		require.ErrorContains(
			t,
			err,
			"both exist without a snapshot activation marker",
		)
		require.DirExists(t, live)
		require.DirExists(t, backup)
	})

	t.Run("removes unpublished staging directory", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()
		stage := filepath.Join(dataDir, snapshotActivationStageName)
		require.NoError(t, os.MkdirAll(stage, 0770))
		require.NoError(t, os.WriteFile(
			filepath.Join(stage, "marker"),
			[]byte("partial"),
			0600,
		))

		instance := &Server{
			config: &Config{DataDir: dataDir},
			logger: hclog.NewNullLogger(),
		}

		require.NoError(t, instance.recoverInterruptedSnapshotActivation())
		require.NoDirExists(t, stage)
	})
}

/* FINISH ALPHA CHANGE 1963 -- test durable and ambiguity-safe snapshot recovery -- */

/* START ALPHA CHANGE 1965 -- cover remaining snapshot recovery states -- */
func TestSnapshotActivationRecoveryStateMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		createMarker     bool
		createLive       bool
		createBackup     bool
		createStage      bool
		expectedLiveFile string
	}{
		{
			name:             "clears marker and staging before the first rename",
			createMarker:     true,
			createLive:       true,
			createStage:      true,
			expectedLiveFile: "live-state",
		},
		{
			name:         "clears marker and staging when no previous chain exists",
			createMarker: true,
			createStage:  true,
		},
		{
			name:             "restores an unmarked backup when live data is absent",
			createBackup:     true,
			expectedLiveFile: "backup-state",
		},
		{
			name:             "removes stale staging while preserving live data",
			createLive:       true,
			createStage:      true,
			expectedLiveFile: "live-state",
		},
		{
			name:             "clears a marked completed activation without a backup",
			createMarker:     true,
			createLive:       true,
			expectedLiveFile: "live-state",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			live := datadir.ChainData(dataDir)
			backup := filepath.Join(
				dataDir,
				snapshotActivationBackupName,
			)
			stage := filepath.Join(
				dataDir,
				snapshotActivationStageName,
			)
			activationMarker := filepath.Join(
				dataDir,
				snapshotActivationMarkerName,
			)

			if test.createLive {
				require.NoError(t, os.MkdirAll(live, 0770))
				require.NoError(t, os.WriteFile(
					filepath.Join(live, "live-state"),
					[]byte("live"),
					0600,
				))
			}

			if test.createBackup {
				require.NoError(t, os.MkdirAll(backup, 0770))
				require.NoError(t, os.WriteFile(
					filepath.Join(backup, "backup-state"),
					[]byte("backup"),
					0600,
				))
			}

			if test.createStage {
				require.NoError(t, os.MkdirAll(stage, 0770))
				require.NoError(t, os.WriteFile(
					filepath.Join(stage, "stage-state"),
					[]byte("stage"),
					0600,
				))
			}

			if test.createMarker {
				require.NoError(t, os.WriteFile(
					activationMarker,
					nil,
					0600,
				))
			}

			instance := &Server{
				config: &Config{DataDir: dataDir},
				logger: hclog.NewNullLogger(),
			}

			require.NoError(
				t,
				instance.recoverInterruptedSnapshotActivation(),
			)

			if test.expectedLiveFile == "" {
				require.NoDirExists(t, live)
			} else {
				require.DirExists(t, live)
				require.FileExists(
					t,
					filepath.Join(live, test.expectedLiveFile),
				)
			}

			require.NoDirExists(t, backup)
			require.NoDirExists(t, stage)
			require.NoFileExists(t, activationMarker)
		})
	}
}

/* FINISH ALPHA CHANGE 1965 -- cover remaining snapshot recovery states -- */

func TestCopySnapshotDirectoryRejectsSymbolicLinks(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")

	require.NoError(t, os.WriteFile(
		filepath.Join(source, "data"),
		[]byte("data"),
		0600,
	))
	require.NoError(t, os.Symlink(
		filepath.Join(source, "data"),
		filepath.Join(source, "link"),
	))

	err := copySnapshotDirectory(source, target)
	require.Error(t, err)
	require.Contains(t, err.Error(), "symbolic link")
}

/* START ALPHA CHANGE 1716 -- test genesis-only classification and live activation -- */
func TestLocalBlockchainHeadRequiresBlockAboveGenesis(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	blockchainPath := datadir.Blockchain(dataDir)
	logger := hclog.NewNullLogger()

	database, err := mdbxstorage.NewMDBXStorageWithMapSize(
		blockchainPath,
		logger,
		1024*1024*1024,
	)
	require.NoError(t, err)

	/* START ALPHA CHANGE 3190 -- provide valid INNBCBFT extra data for the local-head fixture -- */
	validExtraData := (&innbcbft.Extra{}).MarshalRLPTo(nil)

	genesis := (&types.Header{
		Number:       0,
		StateRoot:    types.EmptyRootHash,
		TxRoot:       types.EmptyRootHash,
		ReceiptsRoot: types.EmptyRootHash,
		Sha3Uncles:   types.EmptyUncleHash,
		ExtraData:    validExtraData,
	}).ComputeHash()
	require.NotEqual(t, types.ZeroHash, genesis.Hash)
	/* FINISH ALPHA CHANGE 3190 -- provide valid INNBCBFT extra data for the local-head fixture -- */

	batch := storage.NewBatchWriterForBlock(database, 0)
	batch.PutCanonicalHeader(genesis)
	require.NoError(t, batch.WriteBatch())
	require.NoError(t, database.Close())

	instance := &Server{
		config: &Config{
			DataDir: dataDir,
			History: &HistoryConfig{
				Mode:          "full",
				MDBXMapSizeGB: 1,
			},
		},
		logger: logger,
	}

	/* START ALPHA CHANGE 3213 -- genesis-only storage retains its validated identity without becoming usable history -- */
	head, usable, genesisHash, err := instance.localBlockchainHead()
	require.NoError(t, err)
	require.Equal(t, uint64(0), head)
	require.False(t, usable)
	require.NotNil(t, genesisHash)
	require.Equal(t, genesis.Hash, *genesisHash)
	/* FINISH ALPHA CHANGE 3213 -- genesis-only storage retains its validated identity without becoming usable history -- */

	database, err = mdbxstorage.NewMDBXStorageWithMapSize(
		blockchainPath,
		logger,
		1024*1024*1024,
	)
	require.NoError(t, err)

	/* START ALPHA CHANGE 3190 -- ensure block one cannot overwrite the genesis header fixture -- */
	blockOne := (&types.Header{
		ParentHash:   genesis.Hash,
		Number:       1,
		StateRoot:    types.EmptyRootHash,
		TxRoot:       types.EmptyRootHash,
		ReceiptsRoot: types.EmptyRootHash,
		Sha3Uncles:   types.EmptyUncleHash,
		ExtraData:    validExtraData,
	}).ComputeHash()
	require.NotEqual(t, types.ZeroHash, blockOne.Hash)
	require.NotEqual(t, genesis.Hash, blockOne.Hash)
	/* FINISH ALPHA CHANGE 3190 -- ensure block one cannot overwrite the genesis header fixture -- */

	batch = storage.NewBatchWriterForBlock(database, 1)
	batch.PutCanonicalHeader(blockOne)
	require.NoError(t, batch.WriteBatch())
	require.NoError(t, database.Close())

	/* START ALPHA CHANGE 3213 -- advancing the local head preserves the original genesis identity -- */
	head, usable, genesisHash, err = instance.localBlockchainHead()
	require.NoError(t, err)
	require.Equal(t, uint64(1), head)
	require.True(t, usable)
	require.NotNil(t, genesisHash)
	require.Equal(t, genesis.Hash, *genesisHash)
	/* FINISH ALPHA CHANGE 3213 -- advancing the local head preserves the original genesis identity -- */
}

func TestActivateStagedChainDataReplacesWholeDatabase(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	liveChainData := datadir.ChainData(dataDir)
	stageRoot := filepath.Join(dataDir, snapshotActivationStageName)
	stagedChainData := datadir.ChainData(stageRoot)

	require.NoError(t, os.MkdirAll(liveChainData, 0770))
	require.NoError(t, os.WriteFile(
		filepath.Join(liveChainData, "old"),
		[]byte("old"),
		0600,
	))

	require.NoError(t, os.MkdirAll(stagedChainData, 0770))
	require.NoError(t, os.WriteFile(
		filepath.Join(stagedChainData, "new"),
		[]byte("new"),
		0600,
	))

	instance := &Server{
		config: &Config{DataDir: dataDir},
		logger: hclog.NewNullLogger(),
	}

	require.NoError(t, instance.activateStagedChainData(stageRoot))
	require.FileExists(t, filepath.Join(liveChainData, "new"))
	require.NoFileExists(t, filepath.Join(liveChainData, "old"))
	require.NoDirExists(t, stageRoot)
	require.NoDirExists(
		t,
		filepath.Join(dataDir, snapshotActivationBackupName),
	)
	/* START ALPHA CHANGE 1964 -- verify completed activation removes its transaction marker -- */
	require.NoFileExists(
		t,
		filepath.Join(dataDir, snapshotActivationMarkerName),
	)
	/* FINISH ALPHA CHANGE 1964 -- verify completed activation removes its transaction marker -- */
}

/* START ALPHA CHANGE 1754 -- test failed snapshot activation staging cleanup -- */
func TestCleanupSnapshotActivationStage(t *testing.T) {
	t.Parallel()

	stageRoot := filepath.Join(
		t.TempDir(),
		snapshotActivationStageName,
	)
	require.NoError(t, os.MkdirAll(stageRoot, 0770))
	require.NoError(t, os.WriteFile(
		filepath.Join(stageRoot, "partial"),
		[]byte("partial"),
		0600,
	))

	activationErr := errors.New("snapshot activation failed")
	resultErr := cleanupSnapshotActivationStage(
		stageRoot,
		activationErr,
	)

	require.ErrorIs(t, resultErr, activationErr)
	require.NoDirExists(t, stageRoot)
}

/* FINISH ALPHA CHANGE 1754 -- test failed snapshot activation staging cleanup -- */

func TestFailedSnapshotCopyRemovesPartialStaging(t *testing.T) {
	dataDir := t.TempDir()
	sourceRoot := t.TempDir()
	snapshotName := "block-00000000000000000010-test"
	sourceArtifact := filepath.Join(sourceRoot, snapshotName)

	require.NoError(t, os.MkdirAll(sourceArtifact, 0770))
	require.NoError(t, os.WriteFile(
		filepath.Join(sourceArtifact, "a-data"),
		[]byte("copied before link rejection"),
		0600,
	))
	require.NoError(t, os.Symlink(
		filepath.Join(sourceArtifact, "a-data"),
		filepath.Join(sourceArtifact, "z-link"),
	))

	genesisConfig := &chain.Genesis{}
	genesisHeader := genesisConfig.GenesisHeader()
	genesisHeader.ComputeHash()

	/* START ALPHA CHANGE 1818 -- include genesis state commitment in activation fixture -- */
	manifest := &snapshotbuilder.Manifest{
		FormatVersion:    snapshotbuilder.FormatVersion,
		SnapshotKind:     snapshotbuilder.KindSegmented,
		BlockNumber:      10,
		BlockHash:        types.StringToHash("0x01"),
		StateRoot:        types.EmptyRootHash,
		GenesisHash:      genesisHeader.Hash,
		GenesisStateRoot: genesisHeader.StateRoot,
		RetainedFrom:     1,
		RetainedTo:       10,
		RetentionBlocks:  10,
		SegmentBlocks:    5,
		/* START ALPHA CHANGE 2109 -- satisfy format validation before the deliberate staged-copy failure -- */
		ConsensusData: snapshotbuilder.SnapshotFile{
			Path:   "chain-data/consensus/consensusState/mdbx.dat",
			SHA256: "0101010101010101010101010101010101010101010101010101010101010101",
			Bytes:  1,
		},
		/* FINISH ALPHA CHANGE 2109 -- satisfy format validation before the deliberate staged-copy failure -- */
		SnapshotComplete: true,
	}
	/* FINISH ALPHA CHANGE 1818 -- include genesis state commitment in activation fixture -- */
	latest := &snapshotbuilder.Latest{
		FormatVersion: snapshotbuilder.FormatVersion,
		SnapshotKind:  snapshotbuilder.KindSegmented,
		BlockNumber:   manifest.BlockNumber,
		BlockHash:     manifest.BlockHash,
		StateRoot:     manifest.StateRoot,
		SnapshotDir:   snapshotName,
	}

	writeSnapshotTestJSON(
		t,
		filepath.Join(sourceArtifact, "manifest.json"),
		manifest,
	)
	writeSnapshotTestJSON(
		t,
		filepath.Join(sourceRoot, "latest.json"),
		latest,
	)

	instance := &Server{
		config: &Config{
			DataDir: dataDir,
			Chain: &chain.Chain{
				Genesis: genesisConfig,
			},
			History: &HistoryConfig{
				Mode:                      "recent",
				RetentionBlocks:           10,
				SegmentBlocks:             5,
				SnapshotSourceDir:         sourceRoot,
				SnapshotBootstrapRequired: false,
				MDBXMapSizeGB:             1,
			},
		},
		logger: hclog.NewNullLogger(),
	}

	activated, err := instance.activateConfiguredSnapshot()
	require.NoError(t, err)
	require.Nil(t, activated)
	require.NoDirExists(
		t,
		filepath.Join(dataDir, snapshotActivationStageName),
	)
}

func writeSnapshotTestJSON(
	t *testing.T,
	path string,
	value interface{},
) {
	t.Helper()

	raw, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0600))
}

/* FINISH ALPHA CHANGE 1716 -- test genesis-only classification and live activation -- */

/* FINISH ALPHA CHANGE 1702 -- test whole-chain-data snapshot activation -- */
