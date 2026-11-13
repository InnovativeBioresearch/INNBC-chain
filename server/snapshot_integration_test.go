/* START ALPHA CHANGE 1819 -- test allocated-genesis snapshot lifecycle -- */
package server

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	segmentedstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/segmented"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

const snapshotIntegrationMDBXMapSize = 1024 * 1024 * 1024

/* START ALPHA CHANGE 2107 -- include executable contract state in snapshot lifecycle fixtures -- */
var (
	snapshotIntegrationContractAddress = types.StringToAddress("0x1002")
	snapshotIntegrationStorageKey      = types.StringToHash("0x01")
	snapshotIntegrationStorageValue    = types.StringToHash("0x1234")
	snapshotIntegrationContractCode    = []byte{0x60, 0x00, 0x60, 0x00, 0xf3}
)

/* FINISH ALPHA CHANGE 2107 -- include executable contract state in snapshot lifecycle fixtures -- */

func TestAllocatedGenesisSnapshotBuildActivationAndContinuation(t *testing.T) {
	// START ALPHA CHANGE 3796 -- cover early snapshots with retention exceeding chain age --
	tests := []struct {
		name                       string
		kind                       snapshotbuilder.Kind
		historyMode                string
		bootstrapRequired          bool
		checkpoint                 uint64
		retentionBlocks            uint64
		expectedRetainedFrom       uint64
		expectedHistoryBeforeWrite uint64
		expectedHistoryAfterWrite  uint64
	}{
		{
			name:                       "segmented recent history",
			kind:                       snapshotbuilder.KindSegmented,
			historyMode:                "recent",
			bootstrapRequired:          false,
			checkpoint:                 4,
			retentionBlocks:            2,
			expectedRetainedFrom:       3,
			expectedHistoryBeforeWrite: 3,
			expectedHistoryAfterWrite:  4,
		},
		{
			name:                       "checkpoint full history plan B",
			kind:                       snapshotbuilder.KindCheckpoint,
			historyMode:                "full",
			bootstrapRequired:          true,
			checkpoint:                 4,
			retentionBlocks:            2,
			expectedRetainedFrom:       3,
			expectedHistoryBeforeWrite: 3,
			expectedHistoryAfterWrite:  3,
		},
		{
			name:                       "early segmented snapshot retains full history",
			kind:                       snapshotbuilder.KindSegmented,
			historyMode:                "recent",
			bootstrapRequired:          false,
			checkpoint:                 1,
			retentionBlocks:            4,
			expectedRetainedFrom:       1,
			expectedHistoryBeforeWrite: 0,
			expectedHistoryAfterWrite:  0,
		},
		{
			name:                       "early checkpoint snapshot retains full history",
			kind:                       snapshotbuilder.KindCheckpoint,
			historyMode:                "full",
			bootstrapRequired:          true,
			checkpoint:                 1,
			retentionBlocks:            4,
			expectedRetainedFrom:       1,
			expectedHistoryBeforeWrite: 0,
			expectedHistoryAfterWrite:  0,
		},
	}
	// FINISH ALPHA CHANGE 3796 -- cover early snapshots with retention exceeding chain age --

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// START ALPHA CHANGE 3796 -- select snapshot boundaries per case --
			checkpoint := test.checkpoint
			retentionBlocks := test.retentionBlocks
			const (
				segmentBlocks    = uint64(1)
				destinationMapGB = uint64(1)
			)
			// FINISH ALPHA CHANGE 3796 -- select snapshot boundaries per case --

			logger := hclog.NewNullLogger()
			sourceDataDir := filepath.Join(t.TempDir(), "source-node")
			chainPath := filepath.Join(t.TempDir(), "genesis.json")

			chainConfig, genesisStateRoot, checkpointHeader := writeSnapshotIntegrationSource(
				t,
				sourceDataDir,
				chainPath,
				checkpoint,
				logger,
			)

			require.Equal(t, types.ZeroHash, chainConfig.Genesis.StateRoot)
			require.NotEqual(t, types.EmptyRootHash, genesisStateRoot)

			outputRoot := t.TempDir()
			buildConfig := snapshotbuilder.Config{
				Kind:            test.kind,
				ChainPath:       chainPath,
				SourceDataDir:   sourceDataDir,
				OutputRoot:      outputRoot,
				RetentionBlocks: retentionBlocks,
				MDBXMapSizeGB:   destinationMapGB,
				Logger:          logger,
			}
			if test.kind == snapshotbuilder.KindSegmented {
				buildConfig.SegmentBlocks = segmentBlocks
			}

			result, err := snapshotbuilder.Build(buildConfig)
			require.NoError(t, err)
			require.Equal(t, checkpoint, result.BlockNumber)
			require.Equal(t, checkpointHeader.Hash, result.BlockHash)
			require.Equal(t, genesisStateRoot, result.StateRoot)

			manifest, err := snapshotbuilder.LoadManifest(
				filepath.Join(result.SnapshotDir, "manifest.json"),
			)
			require.NoError(t, err)
			require.Equal(t, genesisStateRoot, manifest.GenesisStateRoot)
			// START ALPHA CHANGE 3796 -- check the selected retained boundary --
			require.Equal(t, test.expectedRetainedFrom, manifest.RetainedFrom)
			// FINISH ALPHA CHANGE 3796 -- check the selected retained boundary --

			targetDataDir := filepath.Join(t.TempDir(), "target-node")
			instance := &Server{
				config: &Config{
					DataDir: targetDataDir,
					Chain:   chainConfig,
					History: &HistoryConfig{
						Mode:            test.historyMode,
						RetentionBlocks: retentionBlocks,
						SegmentBlocks:   segmentBlocks,
						SnapshotSourceDir: filepath.Join(
							outputRoot,
							string(test.kind),
						),
						SnapshotBootstrapRequired: test.bootstrapRequired,
						MDBXMapSizeGB:             destinationMapGB,
					},
				},
				logger: logger,
			}

			activatedManifest, err := instance.activateConfiguredSnapshot()
			require.NoError(t, err)
			require.NotNil(t, activatedManifest)
			require.Equal(t, manifest.GenesisHash, activatedManifest.GenesisHash)
			require.Equal(
				t,
				manifest.GenesisStateRoot,
				activatedManifest.GenesisStateRoot,
			)

			database := openActivatedSnapshotBlockchain(
				t,
				targetDataDir,
				test.historyMode,
				segmentBlocks,
				retentionBlocks,
				logger,
			)

			headNumber, ok := database.ReadHeadNumber()
			require.True(t, ok)
			require.Equal(t, checkpoint, headNumber)

			headHash, ok := database.ReadHeadHash()
			require.True(t, ok)
			require.Equal(t, checkpointHeader.Hash, headHash)

			historyStorage, ok := database.(storage.HistoryAvailabilityStorage)
			require.True(t, ok)
			// START ALPHA CHANGE 3796 -- check history availability before continuation --
			require.Equal(t,
				test.expectedHistoryBeforeWrite,
				historyStorage.HistoryFrom())
			// FINISH ALPHA CHANGE 3796 -- check history availability before continuation --

			genesisHash, ok := database.ReadCanonicalHash(0)
			require.True(t, ok)

			genesisHeader, err := database.ReadHeader(genesisHash)
			require.NoError(t, err)
			require.Equal(t, genesisStateRoot, genesisHeader.StateRoot)

			nextHeader := (&types.Header{
				ParentHash:   checkpointHeader.Hash,
				Number:       checkpoint + 1,
				StateRoot:    checkpointHeader.StateRoot,
				TxRoot:       types.EmptyRootHash,
				ReceiptsRoot: types.EmptyRootHash,
				Sha3Uncles:   types.EmptyUncleHash,
				GasLimit:     checkpointHeader.GasLimit,
				Difficulty:   checkpointHeader.Difficulty,
				BaseFee:      checkpointHeader.BaseFee,
			}).ComputeHash()

			batch := storage.NewBatchWriterForBlock(
				database,
				nextHeader.Number,
			)
			batch.PutCanonicalHeader(nextHeader)
			require.NoError(t, batch.WriteBatch())
			require.NoError(t, database.Close())

			database = openActivatedSnapshotBlockchain(
				t,
				targetDataDir,
				test.historyMode,
				segmentBlocks,
				retentionBlocks,
				logger,
			)

			headNumber, ok = database.ReadHeadNumber()
			require.True(t, ok)
			require.Equal(t, checkpoint+1, headNumber)

			historyStorage, ok = database.(storage.HistoryAvailabilityStorage)
			require.True(t, ok)
			require.Equal(
				t,
				test.expectedHistoryAfterWrite,
				historyStorage.HistoryFrom(),
			)
			require.NoError(t, database.Close())

			trieStorage, err := itrie.NewMDBXStorageReadOnly(
				datadir.Trie(targetDataDir),
				logger,
			)
			require.NoError(t, err)

			/* START ALPHA CHANGE 2107 -- verify executable state after snapshot activation -- */
			require.NoError(t, itrie.ValidateCompleteState(
				checkpointHeader.StateRoot,
				trieStorage,
			))

			stateSnapshot, err := itrie.NewState(trieStorage).NewSnapshotAt(
				checkpointHeader.StateRoot,
			)
			require.NoError(t, err)
			contract, err := stateSnapshot.GetAccount(snapshotIntegrationContractAddress)
			require.NoError(t, err)
			require.NotNil(t, contract)

			/* START ALPHA CHANGE 2153 -- check activated snapshot storage and code reads -- */
			checkedStorage1, storageErr1 := stateSnapshot.GetStorage(
				snapshotIntegrationContractAddress,
				contract.Root,
				snapshotIntegrationStorageKey,
			)
			require.NoError(t, storageErr1)
			require.Equal(
				t,
				snapshotIntegrationStorageValue,
				checkedStorage1,
			)

			code, ok, codeErr := stateSnapshot.GetCode(types.BytesToHash(contract.CodeHash))
			require.NoError(t, codeErr)
			require.True(t, ok)
			require.Equal(t, snapshotIntegrationContractCode, code)
			/* FINISH ALPHA CHANGE 2153 -- check activated snapshot storage and code reads -- */
			require.NoError(t, trieStorage.Close())
			/* FINISH ALPHA CHANGE 2107 -- verify executable state after snapshot activation -- */

			consensusCheckpoint, err := innbcbft.SnapshotStateCheckpoint(
				datadir.Consensus(targetDataDir),
			)
			require.NoError(t, err)
			require.Equal(t, checkpoint, consensusCheckpoint)
		})
	}
}

func writeSnapshotIntegrationSource(
	t *testing.T,
	sourceDataDir string,
	chainPath string,
	checkpoint uint64,
	logger hclog.Logger,
) (*chain.Chain, types.Hash, *types.Header) {
	t.Helper()

	/* START ALPHA CHANGE 1821 -- construct valid INNBCBFT integration headers -- */
	validExtraData := (&innbcbft.Extra{}).MarshalRLPTo(nil)

	chainConfig := &chain.Chain{
		Name: "snapshot-integration",
		Genesis: &chain.Genesis{
			ExtraData:          validExtraData,
			GasLimit:           chain.GenesisGasLimit,
			Difficulty:         chain.GenesisDifficulty,
			BaseFee:            chain.GenesisBaseFee,
			BaseFeeEM:          chain.GenesisBaseFeeEM,
			BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
			Alloc: map[types.Address]*chain.GenesisAccount{
				/* FINISH ALPHA CHANGE 1821 -- construct valid INNBCBFT integration headers -- */
				types.StringToAddress("0x1001"): {
					Balance: big.NewInt(1000000000),
					Nonce:   7,
				},
				/* START ALPHA CHANGE 2107 -- include contract bytecode and populated storage -- */
				snapshotIntegrationContractAddress: {
					Balance: big.NewInt(0),
					Code:    snapshotIntegrationContractCode,
					Storage: map[types.Hash]types.Hash{
						snapshotIntegrationStorageKey: snapshotIntegrationStorageValue,
					},
				},
				/* FINISH ALPHA CHANGE 2107 -- include contract bytecode and populated storage -- */
			},
		},
		Params: &chain.Params{
			Forks:   chain.AllForksEnabled,
			ChainID: 100,
			/* START ALPHA CHANGE 3325 -- preserve snapshot activation coverage with valid clock configuration -- */
			Engine: map[string]interface{}{
				innbcbft.ConsensusName: map[string]interface{}{"blockTime": "1s"},
			},
			/* FINISH ALPHA CHANGE 3325 -- preserve snapshot activation coverage with valid clock configuration -- */
		},
	}

	rawChain, err := json.MarshalIndent(chainConfig, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(chainPath, rawChain, 0600))

	innbcbft.SetupHeaderHashFunc()

	trieStorage, err := itrie.NewMDBXStorageWithMapSize(
		datadir.Trie(sourceDataDir),
		logger,
		snapshotIntegrationMDBXMapSize,
	)
	require.NoError(t, err)

	executor := statepkg.NewExecutor(
		chainConfig.Params,
		itrie.NewState(trieStorage),
		logger,
	)
	genesisStateRoot, err := executor.WriteGenesis(
		chainConfig.Genesis.Alloc,
		types.ZeroHash,
	)
	require.NoError(t, err)
	require.NoError(t, trieStorage.Close())

	database, err := mdbxstorage.NewMDBXStorageWithMapSize(
		datadir.Blockchain(sourceDataDir),
		logger,
		snapshotIntegrationMDBXMapSize,
	)
	require.NoError(t, err)

	genesisHeader := chainConfig.Genesis.GenesisHeader()
	genesisHeader.StateRoot = genesisStateRoot
	genesisHeader.ComputeHash()

	batch := storage.NewBatchWriterForBlock(database, 0)
	batch.PutCanonicalHeader(genesisHeader)
	require.NoError(t, batch.WriteBatch())

	previousHeader := genesisHeader
	for number := uint64(1); number <= checkpoint; number++ {
		/* START ALPHA CHANGE 1822 -- hash test blocks with valid INNBCBFT extra data -- */
		header := (&types.Header{
			ParentHash:   previousHeader.Hash,
			Number:       number,
			StateRoot:    genesisStateRoot,
			TxRoot:       types.EmptyRootHash,
			ReceiptsRoot: types.EmptyRootHash,
			Sha3Uncles:   types.EmptyUncleHash,
			GasLimit:     genesisHeader.GasLimit,
			Difficulty:   genesisHeader.Difficulty,
			BaseFee:      genesisHeader.BaseFee,
			ExtraData:    validExtraData,
		}).ComputeHash()
		/* FINISH ALPHA CHANGE 1822 -- hash test blocks with valid INNBCBFT extra data -- */

		batch = storage.NewBatchWriterForBlock(database, number)
		batch.PutCanonicalHeader(header)
		require.NoError(t, batch.WriteBatch())

		previousHeader = header
	}
	require.NoError(t, database.Close())

	writeSnapshotIntegrationConsensusState(
		t,
		datadir.Consensus(sourceDataDir),
		checkpoint,
	)

	return chainConfig, genesisStateRoot, previousHeader
}

func writeSnapshotIntegrationConsensusState(
	t *testing.T,
	consensusDir string,
	checkpoint uint64,
) {
	t.Helper()

	/* START ALPHA CHANGE 1820 -- use current consensus MDBX directory name -- */
	consensusPath := filepath.Join(consensusDir, "consensusState")
	require.NoError(t, os.MkdirAll(consensusPath, 0750))
	/* FINISH ALPHA CHANGE 1820 -- use current consensus MDBX directory name -- */

	environment, err := mdbxgo.NewEnv(mdbxgo.Default)
	require.NoError(t, err)

	require.NoError(t, environment.SetGeometry(
		-1,
		-1,
		snapshotIntegrationMDBXMapSize,
		-1,
		-1,
		4096,
	))
	require.NoError(t, environment.SetOption(mdbxgo.OptMaxDB, 4))
	require.NoError(t, environment.Open(consensusPath, 0, 0600))

	require.NoError(t, environment.Update(func(transaction *mdbxgo.Txn) error {
		databaseNames := []string{
			"epochs",
			"validatorSnapshots",
			"proposerSnapshot",
			"fullValidatorSetBucket",
		}

		var proposerSnapshotDB mdbxgo.DBI
		for _, databaseName := range databaseNames {
			database, openErr := transaction.OpenDBISimple(
				databaseName,
				mdbxgo.Create,
			)
			if openErr != nil {
				return openErr
			}

			if databaseName == "proposerSnapshot" {
				proposerSnapshotDB = database
			}
		}

		proposerSnapshot, marshalErr := json.Marshal(map[string]interface{}{
			"Height":     checkpoint + 1,
			"Round":      uint64(0),
			"Proposer":   nil,
			"Validators": []interface{}{},
		})
		if marshalErr != nil {
			return marshalErr
		}

		return transaction.Put(
			proposerSnapshotDB,
			[]byte("proposerSnapshotKey"),
			proposerSnapshot,
			0,
		)
	}))
	require.NoError(t, environment.Close())
}

func openActivatedSnapshotBlockchain(
	t *testing.T,
	dataDir string,
	historyMode string,
	segmentBlocks uint64,
	retentionBlocks uint64,
	logger hclog.Logger,
) storage.Storage {
	t.Helper()

	if historyMode == "recent" {
		database, err := segmentedstorage.NewStorage(
			datadir.Blockchain(dataDir),
			logger,
			segmentBlocks,
			retentionBlocks,
			snapshotIntegrationMDBXMapSize,
		)
		require.NoError(t, err)

		return database
	}

	database, err := mdbxstorage.NewMDBXStorageWithMapSize(
		datadir.Blockchain(dataDir),
		logger,
		snapshotIntegrationMDBXMapSize,
	)
	require.NoError(t, err)

	return database
}

/* FINISH ALPHA CHANGE 1819 -- test allocated-genesis snapshot lifecycle -- */
