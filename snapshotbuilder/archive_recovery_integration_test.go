/* START ALPHA CHANGE 2086 -- test historical archive preservation and recovery publication -- */
package snapshotbuilder

import (
	"encoding/json"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	"github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/innovativebioresearch/innbc-chain/types/buildroot"
	"github.com/stretchr/testify/require"
)

const archiveRecoveryTestMDBXMapSize = 1024 * 1024 * 1024

type archiveRecoveryFixture struct {
	source    string
	chainPath string
	archive   string
	snapshots string
	logger    hclog.Logger
}

type archiveRecoveryFileState struct {
	Size     uint64
	SHA256   string
	Modified time.Time
}

func TestArchiveRecoveryStorageLifecycle(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 5)

	first, err := BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)
	require.Equal(t, uint64(4), first.ArchivedThrough)
	require.Equal(t, uint64(2), first.SegmentsAdded)

	manifest, _, err := LoadPublishedArchive(fixture.archive)
	require.NoError(t, err)
	require.Len(t, manifest.Segments, 2)
	require.Zero(t, manifest.Segments[1].TransactionCount)
	assertArchiveRecoveryTransactions(t, fixture.archive, 4, fixture.logger)

	// Fix old modification times so rewriting identical bytes cannot hide
	// behind filesystem timestamp resolution during the immediate rerun.
	stampArchiveRecoveryFiles(t, fixture.archive)
	beforeNoOp := archiveRecoveryInventory(t, fixture.archive)

	noOp, err := BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)
	require.Equal(t, uint64(4), noOp.ArchivedThrough)
	require.Zero(t, noOp.SegmentsAdded)
	require.Zero(t, noOp.TransactionsAdded)
	require.Equal(t, first.ManifestPath, noOp.ManifestPath)
	require.Equal(t, beforeNoOp, archiveRecoveryInventory(t, fixture.archive))

	oldSegments := archiveRecoveryInventory(
		t,
		filepath.Join(fixture.archive, archiveSegmentsDir),
	)

	appendArchiveRecoveryBlocks(t, fixture, 8, 8)
	second, err := BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)
	require.Equal(t, uint64(4), second.PreviousArchivedThrough)
	require.Equal(t, uint64(8), second.ArchivedThrough)
	require.Equal(t, uint64(2), second.SegmentsAdded)

	newSegments := archiveRecoveryInventory(
		t,
		filepath.Join(fixture.archive, archiveSegmentsDir),
	)
	for path, expected := range oldSegments {
		actual, exists := newSegments[path]
		require.True(t, exists, path)
		require.Equal(t, expected, actual, path)
	}
	assertArchiveRecoveryTransactions(t, fixture.archive, 8, fixture.logger)

	recovery, err := BuildRecoveryCheckpoint(fixture.recoveryConfig())
	require.NoError(t, err)
	require.Equal(t, uint64(8), recovery.CheckpointNumber)
	require.Equal(t, uint64(6), recovery.SharedBoundaryNumber)
	require.Equal(t, uint64(3), recovery.ActualRetentionBlocks)
	require.Equal(t, uint64(8), recovery.ArchiveActualThrough)
	assertArchiveRecoveryRelationship(t, fixture, recovery)

	// Restore into an independent node directory. Do not advance the
	// published immutable checkpoint itself.
	restored := fixture
	restored.source = filepath.Join(t.TempDir(), "restored-node")
	copyArchiveRecoveryTree(
		t,
		datadir.ChainData(recovery.CheckpointDir),
		datadir.ChainData(restored.source),
	)

	database := openArchiveRecoveryDatabase(
		t,
		datadir.Blockchain(restored.source),
		true,
		restored.logger,
	)
	history, ok := database.(storage.HistoryAvailabilityStorage)
	require.True(t, ok)
	require.Equal(t, uint64(6), history.HistoryFrom())
	require.NoError(t, database.Close())

	appendArchiveRecoveryBlocks(t, restored, 12, 12)

	continued, err := BuildArchive(restored.archiveConfig())
	require.NoError(t, err)
	require.Equal(t, uint64(8), continued.PreviousArchivedThrough)
	require.Equal(t, uint64(12), continued.ArchivedThrough)
	assertArchiveRecoveryTransactions(t, restored.archive, 12, restored.logger)

	secondRecovery, err := BuildRecoveryCheckpoint(restored.recoveryConfig())
	require.NoError(t, err)
	require.Equal(t, uint64(12), secondRecovery.CheckpointNumber)
	require.Equal(t, uint64(10), secondRecovery.SharedBoundaryNumber)
	assertArchiveRecoveryRelationship(t, restored, secondRecovery)
	require.DirExists(t, recovery.CheckpointDir)
}

func TestArchiveContinuationSourceFloor(t *testing.T) {
	for _, test := range []struct {
		name    string
		from    uint64
		wantErr bool
	}{
		{name: "retains previous boundary", from: 4},
		{name: "starts at next required block", from: 5},
		{name: "starts after next required block", from: 6, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newArchiveRecoveryFixture(t, 4)
			_, err := BuildArchive(fixture.archiveConfig())
			require.NoError(t, err)

			appendArchiveRecoveryBlocks(t, fixture, 8, 8)

			source := openArchiveRecoveryDatabase(
				t,
				datadir.Blockchain(fixture.source),
				true,
				fixture.logger,
			)
			truncatedSource := filepath.Join(t.TempDir(), "checkpoint-origin")
			target := openArchiveRecoveryDatabase(
				t,
				datadir.Blockchain(truncatedSource),
				false,
				fixture.logger,
			)
			require.NoError(t, copyBlockchainSuffix(source, target, test.from, 8))
			require.NoError(t, target.Close())
			require.NoError(t, source.Close())

			config := fixture.archiveConfig()
			config.SourceDataDir = truncatedSource
			result, err := BuildArchive(config)
			if test.wantErr {
				require.ErrorContains(t, err, "next required archive block is 5")
				return
			}

			require.NoError(t, err)
			require.Equal(t, uint64(8), result.ArchivedThrough)
			assertArchiveRecoveryTransactions(t, fixture.archive, 8, fixture.logger)
		})
	}
}

func TestRecoveryRetryUsesPinnedArchiveManifest(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 6)
	_, err := BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)

	first, err := BuildRecoveryCheckpoint(fixture.recoveryConfig())
	require.NoError(t, err)
	pinned, err := loadRecoverySet(first.RecoverySetPath)
	require.NoError(t, err)

	beforeInfo, err := os.Stat(first.CheckpointDir)
	require.NoError(t, err)
	beforeRelationship, err := os.ReadFile(first.RecoverySetPath)
	require.NoError(t, err)

	// Advance archive history while keeping the exported consensus
	// checkpoint at six, so this is an exact same-height retry.
	appendArchiveRecoveryBlocks(t, fixture, 8, 6)
	_, err = BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)
	latestArchive, latestArchivePath, err := LoadPublishedArchive(fixture.archive)
	require.NoError(t, err)
	require.Equal(t, uint64(8), latestArchive.ArchivedThrough)
	require.NotEqual(t, pinned.Archive.Manifest.Path, filepath.Base(latestArchivePath))

	require.NoError(t, os.Remove(first.LatestPath))

	retried, err := BuildRecoveryCheckpoint(fixture.recoveryConfig())
	require.NoError(t, err)
	require.Equal(t, first.CheckpointDir, retried.CheckpointDir)
	require.Equal(t, uint64(6), retried.ArchiveActualThrough)
	require.FileExists(t, retried.LatestPath)

	afterInfo, err := os.Stat(retried.CheckpointDir)
	require.NoError(t, err)
	require.True(t, os.SameFile(beforeInfo, afterInfo))
	afterRelationship, err := os.ReadFile(retried.RecoverySetPath)
	require.NoError(t, err)
	require.Equal(t, beforeRelationship, afterRelationship)

	reloaded, err := loadRecoverySet(retried.RecoverySetPath)
	require.NoError(t, err)
	require.Equal(t, pinned.Archive.Manifest, reloaded.Archive.Manifest)
}

func TestRecoveryPointerWriteFailurePreservesCompletedCheckpoint(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 4)

	// Establish a previous ordinary checkpoint pointer.
	previous, err := Build(fixture.snapshotConfig())
	require.NoError(t, err)
	previousPointer, err := os.ReadFile(previous.LatestPath)
	require.NoError(t, err)

	appendArchiveRecoveryBlocks(t, fixture, 8, 8)
	_, err = BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)

	// An existing directory at the temporary pointer filename causes the
	// actual writeJSONAtomic open to fail after checkpoint publication.
	obstruction := previous.LatestPath + ".tmp"
	require.NoError(t, os.Mkdir(obstruction, 0750))

	_, err = BuildRecoveryCheckpoint(fixture.recoveryConfig())
	require.ErrorContains(t, err, "failed to publish latest snapshot pointer")

	unchangedPointer, err := os.ReadFile(previous.LatestPath)
	require.NoError(t, err)
	require.Equal(t, previousPointer, unchangedPointer)
	require.DirExists(t, previous.SnapshotDir)

	source := openArchiveRecoveryDatabase(
		t,
		datadir.Blockchain(fixture.source),
		true,
		fixture.logger,
	)
	header, err := readArchiveCanonicalHeader(source, 8)
	require.NoError(t, err)
	require.NoError(t, source.Close())

	completedDir := filepath.Join(
		fixture.snapshots,
		KindCheckpoint.directoryName(),
		snapshotDirectoryName(8, header.Hash),
	)
	require.DirExists(t, completedDir)
	require.FileExists(t, filepath.Join(completedDir, recoverySetFileName))

	beforeInfo, err := os.Stat(completedDir)
	require.NoError(t, err)

	require.NoError(t, os.Remove(obstruction))
	retried, err := BuildRecoveryCheckpoint(fixture.recoveryConfig())
	require.NoError(t, err)
	require.Equal(t, completedDir, retried.CheckpointDir)
	assertArchiveRecoveryRelationship(t, fixture, retried)

	afterInfo, err := os.Stat(completedDir)
	require.NoError(t, err)
	require.True(t, os.SameFile(beforeInfo, afterInfo))
}

func TestArchivePointerWriteFailureCanReusePublishedSegments(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 4)
	first, err := BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)
	oldPointer, err := os.ReadFile(first.LatestPath)
	require.NoError(t, err)

	appendArchiveRecoveryBlocks(t, fixture, 8, 8)
	obstruction := first.LatestPath + ".tmp"
	require.NoError(t, os.Mkdir(obstruction, 0750))

	_, err = BuildArchive(fixture.archiveConfig())
	require.ErrorContains(t, err, "publish archive latest pointer")

	currentPointer, err := os.ReadFile(first.LatestPath)
	require.NoError(t, err)
	require.Equal(t, oldPointer, currentPointer)

	entries, err := os.ReadDir(filepath.Join(fixture.archive, archiveSegmentsDir))
	require.NoError(t, err)
	require.Len(t, entries, 4)

	require.NoError(t, os.Remove(obstruction))
	retried, err := BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)
	require.Equal(t, uint64(8), retried.ArchivedThrough)
	require.Equal(t, uint64(2), retried.SegmentsAdded)
	assertArchiveRecoveryTransactions(t, fixture.archive, 8, fixture.logger)
}

func TestRecoveryRejectsConflictingExistingCheckpoint(t *testing.T) {
	t.Run("ordinary checkpoint at same height", func(t *testing.T) {
		fixture := newArchiveRecoveryFixture(t, 6)
		_, err := BuildArchive(fixture.archiveConfig())
		require.NoError(t, err)
		existing, err := Build(fixture.snapshotConfig())
		require.NoError(t, err)

		_, err = BuildRecoveryCheckpoint(fixture.recoveryConfig())
		require.ErrorContains(t, err, "existing checkpoint is not a valid requested recovery artifact")
		require.DirExists(t, existing.SnapshotDir)
	})

	t.Run("different requested retention boundary", func(t *testing.T) {
		fixture := newArchiveRecoveryFixture(t, 6)
		_, err := BuildArchive(fixture.archiveConfig())
		require.NoError(t, err)
		existing, err := BuildRecoveryCheckpoint(fixture.recoveryConfig())
		require.NoError(t, err)

		config := fixture.recoveryConfig()
		config.MinimumRetentionBlocks = 5
		_, err = BuildRecoveryCheckpoint(config)
		require.ErrorContains(t, err, "different boundary")
		require.DirExists(t, existing.CheckpointDir)
	})
}

func TestRecoveryRequiresPublishedArchive(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 6)
	require.NoError(t, os.MkdirAll(fixture.archive, 0750))

	_, err := BuildRecoveryCheckpoint(fixture.recoveryConfig())
	require.ErrorIs(t, err, ErrArchiveNotFound)
}

func TestRecoveryRejectsSameSizePayloadCorruption(t *testing.T) {
	for _, payload := range []string{
		archiveIndexFileName,
		filepath.Join(archiveBlockchainDir, archiveMDBXDataFile),
	} {
		t.Run(payload, func(t *testing.T) {
			fixture := newArchiveRecoveryFixture(t, 6)
			_, err := BuildArchive(fixture.archiveConfig())
			require.NoError(t, err)
			manifest, _, err := LoadPublishedArchive(fixture.archive)
			require.NoError(t, err)

			segment := manifest.Segments[1]
			path := filepath.Join(
				fixture.archive,
				archiveSegmentsDir,
				segment.Directory,
				payload,
			)
			file, err := os.OpenFile(path, os.O_RDWR, 0)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = file.Close()
			})

			var value [1]byte
			_, err = file.ReadAt(value[:], 0)
			require.NoError(t, err)
			value[0] ^= 0xff
			_, err = file.WriteAt(value[:], 0)
			require.NoError(t, err)
			require.NoError(t, file.Close())

			_, err = BuildRecoveryCheckpoint(fixture.recoveryConfig())
			require.ErrorContains(t, err, "checksum mismatch")
			require.NoFileExists(t, filepath.Join(
				fixture.snapshots,
				KindCheckpoint.directoryName(),
				latestFileName,
			))
		})
	}
}

func TestRecoveryReadsActualArchivedBoundary(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 6)
	_, err := BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)
	manifest, _, err := LoadPublishedArchive(fixture.archive)
	require.NoError(t, err)

	segment := &manifest.Segments[1]
	directory := filepath.Join(fixture.archive, archiveSegmentsDir, segment.Directory)

	database := openArchiveRecoveryDatabase(
		t,
		filepath.Join(directory, archiveBlockchainDir),
		false,
		fixture.logger,
	)
	header, err := readArchiveCanonicalHeader(database, segment.To)
	require.NoError(t, err)
	header.Timestamp++
	header.ComputeHash()

	batch := storage.NewBatchWriterForBlock(database, header.Number)
	batch.PutCanonicalHeader(header)
	require.NoError(t, batch.WriteBatch())
	require.NoError(t, database.Close())

	// Recommit the changed payload checksum while deliberately retaining
	// the old claimed boundary hash. Checksums alone must not satisfy this test.
	metadata, err := readArchiveSegmentMetadata(directory, segment)
	require.NoError(t, err)
	dataIdentity, err := archiveFileIdentity(
		filepath.Join(directory, archiveBlockchainDir, archiveMDBXDataFile),
	)
	require.NoError(t, err)
	dataIdentity.Path = archiveBlockchainDir + "/" + archiveMDBXDataFile
	metadata.BlockchainData = dataIdentity

	metadataPath := filepath.Join(directory, archiveSegmentFile)
	writeArchiveRecoveryJSON(t, metadataPath, metadata)
	metadataIdentity, err := archiveFileIdentity(metadataPath)
	require.NoError(t, err)
	metadataIdentity.Path = archiveSegmentFile
	segment.SegmentManifest = metadataIdentity

	_, err = verifyArchiveForRecovery(
		fixture.archive,
		manifest,
		segment.To,
		fixture.logger,
	)
	require.ErrorContains(t, err, "actual archived boundary header differs")
}

func TestRecoveryRejectsSegmentChainIdentityMismatch(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 6)
	_, err := BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)
	manifest, _, err := LoadPublishedArchive(fixture.archive)
	require.NoError(t, err)

	segment := &manifest.Segments[0]
	directory := filepath.Join(fixture.archive, archiveSegmentsDir, segment.Directory)
	metadata, err := readArchiveSegmentMetadata(directory, segment)
	require.NoError(t, err)
	metadata.ChainID++

	path := filepath.Join(directory, archiveSegmentFile)
	writeArchiveRecoveryJSON(t, path, metadata)
	identity, err := archiveFileIdentity(path)
	require.NoError(t, err)
	identity.Path = archiveSegmentFile
	segment.SegmentManifest = identity

	_, err = verifyArchiveForRecovery(
		fixture.archive,
		manifest,
		4,
		fixture.logger,
	)
	require.ErrorContains(t, err, "chain identity differs")
}

func TestRecoveryCheckpointBoundaryMismatch(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 6)
	_, err := BuildArchive(fixture.archiveConfig())
	require.NoError(t, err)
	result, err := BuildRecoveryCheckpoint(fixture.recoveryConfig())
	require.NoError(t, err)
	manifest, err := LoadManifest(filepath.Join(result.CheckpointDir, manifestFileName))
	require.NoError(t, err)

	err = verifyRecoveryCheckpointBoundary(
		result.CheckpointDir,
		manifest,
		RecoverySetBoundary{
			BlockNumber: result.SharedBoundaryNumber,
			BlockHash:   types.StringToHash("0xdeadbeef"),
		},
		fixture.logger,
	)
	require.ErrorContains(t, err, "actual checkpoint and archive boundary headers differ")
}

func TestOrdinaryCleanupPreservesRecoveryCheckpoints(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		name := "valid relationship"
		if corrupt {
			name = "malformed relationship"
		}

		t.Run(name, func(t *testing.T) {
			fixture := newArchiveRecoveryFixture(t, 6)
			_, err := BuildArchive(fixture.archiveConfig())
			require.NoError(t, err)
			recovery, err := BuildRecoveryCheckpoint(fixture.recoveryConfig())
			require.NoError(t, err)

			if corrupt {
				require.NoError(t, os.WriteFile(
					recovery.RecoverySetPath,
					[]byte("{invalid"),
					0600,
				))
			}

			appendArchiveRecoveryBlocks(t, fixture, 8, 8)
			ordinary, err := Build(fixture.snapshotConfig())
			require.NoError(t, err)
			require.DirExists(t, recovery.CheckpointDir)

			appendArchiveRecoveryBlocks(t, fixture, 10, 10)
			_, err = Build(fixture.snapshotConfig())
			require.NoError(t, err)
			require.NoDirExists(t, ordinary.SnapshotDir)
			require.DirExists(t, recovery.CheckpointDir)
		})
	}
}

func (f archiveRecoveryFixture) archiveConfig() ArchiveConfig {
	return ArchiveConfig{
		ChainPath:       f.chainPath,
		SourceDataDir:   f.source,
		OutputRoot:      f.archive,
		SegmentBlocks:   2,
		MaxIndexRecords: 100,
		MDBXMapSizeGB:   1,
		Logger:          f.logger,
	}
}

func (f archiveRecoveryFixture) recoveryConfig() RecoveryCheckpointConfig {
	return RecoveryCheckpointConfig{
		ChainPath:              f.chainPath,
		SourceDataDir:          f.source,
		SnapshotOutputRoot:     f.snapshots,
		ArchiveOutputRoot:      f.archive,
		MinimumRetentionBlocks: 3,
		MDBXMapSizeGB:          1,
		Logger:                 f.logger,
	}
}

func (f archiveRecoveryFixture) snapshotConfig() Config {
	return Config{
		Kind:            KindCheckpoint,
		ChainPath:       f.chainPath,
		SourceDataDir:   f.source,
		OutputRoot:      f.snapshots,
		RetentionBlocks: 3,
		MDBXMapSizeGB:   1,
		Logger:          f.logger,
	}
}

func newArchiveRecoveryFixture(t *testing.T, head uint64) archiveRecoveryFixture {
	t.Helper()

	root := t.TempDir()
	fixture := archiveRecoveryFixture{
		source:    filepath.Join(root, "source"),
		chainPath: filepath.Join(root, "genesis.json"),
		archive:   filepath.Join(root, "archive"),
		snapshots: filepath.Join(root, "snapshots"),
		logger:    hclog.NewNullLogger(),
	}

	innbcbft.SetupHeaderHashFunc()
	extra := (&innbcbft.Extra{}).MarshalRLPTo(nil)
	config := &chain.Chain{
		Name: "archive-storage-test",
		Genesis: &chain.Genesis{
			ExtraData:          extra,
			GasLimit:           chain.GenesisGasLimit,
			Difficulty:         chain.GenesisDifficulty,
			BaseFee:            chain.GenesisBaseFee,
			BaseFeeEM:          chain.GenesisBaseFeeEM,
			BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
			Alloc: map[types.Address]*chain.GenesisAccount{
				types.StringToAddress("0x1001"): {
					Balance: big.NewInt(1000000000),
					Nonce:   7,
				},
			},
		},
		Params: &chain.Params{
			Forks:   chain.AllForksEnabled,
			ChainID: 100,
			/* START ALPHA CHANGE 3325 -- preserve archive recovery coverage with valid clock configuration -- */
			Engine: map[string]interface{}{
				innbcbft.ConsensusName: map[string]interface{}{"blockTime": "1s"},
			},
			/* FINISH ALPHA CHANGE 3325 -- preserve archive recovery coverage with valid clock configuration -- */
		},
	}
	writeArchiveRecoveryJSON(t, fixture.chainPath, config)

	trieStorage, err := itrie.NewMDBXStorageWithMapSize(
		datadir.Trie(fixture.source),
		fixture.logger,
		archiveRecoveryTestMDBXMapSize,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = trieStorage.Close()
	})

	executor := statepkg.NewExecutor(
		config.Params,
		itrie.NewState(trieStorage),
		fixture.logger,
	)
	stateRoot, err := executor.WriteGenesis(config.Genesis.Alloc, types.ZeroHash)
	require.NoError(t, err)
	require.NoError(t, trieStorage.Close())

	database := openArchiveRecoveryDatabase(
		t,
		datadir.Blockchain(fixture.source),
		false,
		fixture.logger,
	)
	genesis := config.Genesis.GenesisHeader()
	genesis.StateRoot = stateRoot
	genesis.ComputeHash()
	batch := storage.NewBatchWriterForBlock(database, 0)
	batch.PutCanonicalHeader(genesis)
	require.NoError(t, batch.WriteBatch())
	require.NoError(t, database.Close())

	appendArchiveRecoveryBlocks(t, fixture, head, head)

	return fixture
}

func appendArchiveRecoveryBlocks(
	t *testing.T,
	fixture archiveRecoveryFixture,
	newHead uint64,
	consensusCheckpoint uint64,
) {
	t.Helper()

	database := openArchiveRecoveryDatabase(
		t,
		datadir.Blockchain(fixture.source),
		false,
		fixture.logger,
	)
	head, ok := database.ReadHeadNumber()
	require.True(t, ok)
	require.GreaterOrEqual(t, newHead, head)
	previous, err := readArchiveCanonicalHeader(database, head)
	require.NoError(t, err)
	genesis, err := readArchiveCanonicalHeader(database, 0)
	require.NoError(t, err)

	for number := head + 1; number <= newHead; number++ {
		body, receipts := archiveRecoveryBlockData(number)
		var gasUsed uint64
		if len(receipts) > 0 {
			gasUsed = receipts[len(receipts)-1].CumulativeGasUsed
		}

		header := (&types.Header{
			ParentHash:   previous.Hash,
			Number:       number,
			StateRoot:    genesis.StateRoot,
			TxRoot:       buildroot.CalculateTransactionsRoot(body.Transactions, number),
			ReceiptsRoot: buildroot.CalculateReceiptsRoot(receipts),
			Sha3Uncles:   buildroot.CalculateUncleRoot(body.Uncles),
			LogsBloom:    types.CreateBloom(receipts),
			GasLimit:     genesis.GasLimit,
			GasUsed:      gasUsed,
			Difficulty:   genesis.Difficulty,
			Timestamp:    number * 2,
			BaseFee:      genesis.BaseFee,
			ExtraData:    (&innbcbft.Extra{}).MarshalRLPTo(nil),
		}).ComputeHash()

		batch := storage.NewBatchWriterForBlock(database, number)
		batch.PutCanonicalHeader(header)
		batch.PutBody(header.Hash, body)
		batch.PutReceipts(header.Hash, receipts)
		for _, transaction := range body.Transactions {
			batch.PutTxLookup(transaction.Hash, header.Hash)
		}
		require.NoError(t, batch.WriteBatch())
		previous = header
	}
	require.NoError(t, database.Close())

	writeArchiveRecoveryConsensus(
		t,
		datadir.Consensus(fixture.source),
		consensusCheckpoint,
	)
}

func archiveRecoveryBlockData(number uint64) (*types.Body, []*types.Receipt) {
	// Preserve one completely empty segment as well as populated segments.
	if number == 3 || number == 4 {
		return &types.Body{}, []*types.Receipt{}
	}

	body := &types.Body{}
	var receipts []*types.Receipt
	for position, transactionType := range []types.TxType{
		types.LegacyTx,
		types.LegacyTx,
		types.DynamicFeeTx,
		types.StateTx,
	} {
		recipient := types.StringToAddress("0x2002")
		transaction := &types.Transaction{
			Nonce:     number*10 + uint64(position),
			GasPrice:  big.NewInt(1),
			GasTipCap: big.NewInt(1),
			GasFeeCap: big.NewInt(2),
			Gas:       100000,
			To:        &recipient,
			Value:     big.NewInt(int64(number)),
			Input:     []byte{byte(number), byte(position)},
			V:         big.NewInt(0),
			R:         big.NewInt(1),
			S:         big.NewInt(1),
			From:      types.StringToAddress("0x1001"),
			Type:      transactionType,
			ChainID:   big.NewInt(100),
		}
		if position == 1 {
			transaction.To = nil
		}
		transaction.ComputeHash(number)

		receipt := &types.Receipt{
			CumulativeGasUsed: uint64(position+1) * 21000,
			GasUsed:           21000,
			TxHash:            transaction.Hash,
			TransactionType:   transactionType,
		}
		receipt.SetStatus(types.ReceiptSuccess)
		if position == 2 {
			receipt.SetStatus(types.ReceiptFailed)
		} else {
			receipt.Logs = []*types.Log{{
				Address: types.StringToAddress("0x3003"),
				Topics:  []types.Hash{types.StringToHash("0x1234")},
				Data:    []byte{byte(number), byte(position), 0xaa},
			}}
		}
		if position == 1 {
			receipt.SetContractAddress(types.StringToAddress("0x4004"))
		}
		receipt.LogsBloom = types.CreateBloom([]*types.Receipt{receipt})

		body.Transactions = append(body.Transactions, transaction)
		receipts = append(receipts, receipt)
	}

	return body, receipts
}

func writeArchiveRecoveryConsensus(t *testing.T, directory string, checkpoint uint64) {
	t.Helper()

	path := filepath.Join(directory, "consensusState")
	require.NoError(t, os.MkdirAll(path, 0750))
	environment, err := mdbxgo.NewEnv(mdbxgo.Default)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = environment.Close()
	})

	require.NoError(t, environment.SetGeometry(
		-1, -1, archiveRecoveryTestMDBXMapSize, -1, -1, 4096,
	))
	require.NoError(t, environment.SetOption(mdbxgo.OptMaxDB, 4))
	require.NoError(t, environment.Open(path, 0, 0600))

	require.NoError(t, environment.Update(func(transaction *mdbxgo.Txn) error {
		var proposerDB mdbxgo.DBI
		for _, name := range []string{
			"epochs",
			"validatorSnapshots",
			"proposerSnapshot",
			"fullValidatorSetBucket",
		} {
			database, err := transaction.OpenDBISimple(name, mdbxgo.Create)
			if err != nil {
				return err
			}
			if name == "proposerSnapshot" {
				proposerDB = database
			}
		}

		raw, err := json.Marshal(&innbcbft.ProposerSnapshot{
			Height: checkpoint + 1,
		})
		if err != nil {
			return err
		}
		return transaction.Put(
			proposerDB,
			[]byte("proposerSnapshotKey"),
			raw,
			0,
		)
	}))
	require.NoError(t, environment.Close())
}

func openArchiveRecoveryDatabase(
	t *testing.T,
	path string,
	readOnly bool,
	logger hclog.Logger,
) storage.Storage {
	t.Helper()

	var database storage.Storage
	var err error
	if readOnly {
		database, err = mdbxstorage.NewMDBXStorageReadOnly(path, logger)
	} else {
		database, err = mdbxstorage.NewMDBXStorageWithMapSize(
			path,
			logger,
			archiveRecoveryTestMDBXMapSize,
		)
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = database.Close()
	})

	return database
}

func assertArchiveRecoveryTransactions(
	t *testing.T,
	root string,
	through uint64,
	logger hclog.Logger,
) {
	t.Helper()

	manifest, _, err := LoadPublishedArchive(root)
	require.NoError(t, err)
	require.Equal(t, through, manifest.ArchivedThrough)

	type location struct {
		record    TransactionIndexRecord
		directory string
	}
	locations := make(map[types.Hash]location)
	for _, segment := range manifest.Segments {
		directory := filepath.Join(root, archiveSegmentsDir, segment.Directory)
		records, err := readTransactionIndex(
			filepath.Join(directory, archiveIndexFileName),
			manifest.MaxIndexRecords,
		)
		require.NoError(t, err)
		for _, record := range records {
			_, duplicate := locations[record.Hash]
			require.False(t, duplicate)
			locations[record.Hash] = location{record: record, directory: directory}
		}
	}

	var expectedCount int
	for number := uint64(1); number <= through; number++ {
		expectedBody, expectedReceipts := archiveRecoveryBlockData(number)
		for position, expectedTransaction := range expectedBody.Transactions {
			expectedCount++
			found, exists := locations[expectedTransaction.Hash]
			require.True(t, exists)
			require.Equal(t, number, found.record.BlockNumber)
			require.Equal(t, uint32(position), found.record.TransactionIndex)

			database := openArchiveRecoveryDatabase(
				t,
				filepath.Join(found.directory, archiveBlockchainDir),
				true,
				logger,
			)
			header, err := readArchiveCanonicalHeader(database, found.record.BlockNumber)
			require.NoError(t, err)
			body, err := database.ReadBody(header.Hash)
			require.NoError(t, err)
			receipts, err := database.ReadReceipts(header.Hash)
			require.NoError(t, err)

			require.Greater(t, len(body.Transactions), int(found.record.TransactionIndex))
			require.Greater(t, len(receipts), int(found.record.TransactionIndex))
			actualTransaction := body.Transactions[found.record.TransactionIndex]
			actualReceipt := receipts[found.record.TransactionIndex]
			expectedReceipt := expectedReceipts[position]

			require.Equal(t, expectedTransaction.Hash, actualTransaction.Hash)
			require.Equal(t, expectedTransaction.Type, actualTransaction.Type)
			require.Equal(
				t,
				expectedTransaction.MarshalStoreRLPTo(nil),
				actualTransaction.MarshalStoreRLPTo(nil),
			)
			require.Equal(
				t,
				expectedReceipt.MarshalStoreRLPTo(nil),
				actualReceipt.MarshalStoreRLPTo(nil),
			)
			require.Equal(t, expectedReceipt.Status, actualReceipt.Status)
			require.Equal(t, expectedReceipt.ContractAddress, actualReceipt.ContractAddress)
			require.Len(t, actualReceipt.Logs, len(expectedReceipt.Logs))
			if expectedReceipt.ContractAddress != nil {
				require.Nil(t, actualTransaction.To)
			}

			lookup, ok := database.ReadTxLookup(expectedTransaction.Hash)
			require.True(t, ok)
			require.Equal(t, header.Hash, lookup)
			require.NoError(t, database.Close())
		}
	}

	require.Len(t, locations, expectedCount)
	require.Equal(t, uint64(expectedCount), manifest.TransactionCount)
}

func assertArchiveRecoveryRelationship(
	t *testing.T,
	fixture archiveRecoveryFixture,
	result *RecoveryCheckpointResult,
) {
	t.Helper()

	recovery, err := loadRecoverySet(result.RecoverySetPath)
	require.NoError(t, err)
	archive, archiveFile, err := loadRecoveryArchiveObject(
		fixture.archive,
		recovery.Archive.Manifest,
	)
	require.NoError(t, err)
	checkpointPath := filepath.Join(result.CheckpointDir, manifestFileName)
	checkpoint, err := LoadManifest(checkpointPath)
	require.NoError(t, err)
	checkpointFile, err := archiveFileIdentity(checkpointPath)
	require.NoError(t, err)
	checkpointFile.Path = manifestFileName

	require.NoError(t, ValidateRecoverySet(
		recovery, archive, checkpoint, archiveFile, checkpointFile,
	))
	archivedHeader, err := verifyArchiveForRecovery(
		fixture.archive,
		archive,
		recovery.SharedBoundary.BlockNumber,
		fixture.logger,
	)
	require.NoError(t, err)
	require.Equal(t, recovery.SharedBoundary.BlockHash, archivedHeader.Hash)
	require.NoError(t, verifyRecoveryCheckpointBoundary(
		result.CheckpointDir,
		checkpoint,
		recovery.SharedBoundary,
		fixture.logger,
	))

	latest, err := LoadLatest(result.LatestPath)
	require.NoError(t, err)
	require.Equal(t, result.CheckpointNumber, latest.BlockNumber)
	require.Equal(t, result.CheckpointHash, latest.BlockHash)
	require.Equal(t, filepath.Base(result.CheckpointDir), latest.SnapshotDir)
}

func writeArchiveRecoveryJSON(t *testing.T, path string, value any) {
	t.Helper()

	raw, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(raw, '\n'), 0600))
}

func stampArchiveRecoveryFiles(t *testing.T, root string) {
	t.Helper()

	stamp := time.Unix(123456789, 0)
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == buildLockFileName ||
			entry.Name() == archiveMDBXLockFile {
			return nil
		}
		return os.Chtimes(path, stamp, stamp)
	}))
}

func archiveRecoveryInventory(
	t *testing.T,
	root string,
) map[string]archiveRecoveryFileState {
	t.Helper()

	result := make(map[string]archiveRecoveryFileState)
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == buildLockFileName ||
			entry.Name() == archiveMDBXLockFile {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		identity, err := archiveFileIdentity(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[relative] = archiveRecoveryFileState{
			Size:     identity.Bytes,
			SHA256:   identity.SHA256,
			Modified: info.ModTime(),
		}
		return nil
	}))

	return result
}

func copyArchiveRecoveryTree(t *testing.T, source, destination string) {
	t.Helper()

	// All database handles for this immutable fixture are closed before copying.
	require.NoError(t, filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0750)
		}
		if entry.Name() == archiveMDBXLockFile {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		require.True(t, info.Mode().IsRegular(), path)

		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputErr := input.Close()
		outputErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputErr != nil {
			return inputErr
		}
		return outputErr
	}))
}

/* FINISH ALPHA CHANGE 2086 -- test historical archive preservation and recovery publication -- */
