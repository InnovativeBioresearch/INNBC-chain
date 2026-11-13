/* START ALPHA CHANGE 2106 -- test snapshot commitments and recovery retries across independent MDBX exports -- */
package snapshotbuilder

/* START ALPHA CHANGE 3333 -- import snapshot history-floor regression dependencies -- */
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* FINISH ALPHA CHANGE 3333 -- import snapshot history-floor regression dependencies -- */

func TestSnapshotManifestRequiresConsensusCommitment(t *testing.T) {
	f := newArchiveRecoveryFixture(t, 4)
	result, err := Build(f.snapshotConfig())
	require.NoError(t, err)
	valid, err := LoadManifest(filepath.Join(result.SnapshotDir, manifestFileName))
	require.NoError(t, err)

	tests := []struct {
		name string
		edit func(*Manifest)
	}{
		{"missing", func(m *Manifest) { m.ConsensusData = SnapshotFile{} }},
		{"path", func(m *Manifest) { m.ConsensusData.Path = "../mdbx.dat" }},
		{"size", func(m *Manifest) { m.ConsensusData.Bytes = 0 }},
		{"checksum", func(m *Manifest) { m.ConsensusData.SHA256 = "invalid" }},
		{"old format", func(m *Manifest) { m.FormatVersion = 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := *valid
			test.edit(&candidate)
			require.Error(t, ValidateManifest(&candidate))
		})
	}
}

func TestSnapshotCheckpointHeaderBindsStateRoot(t *testing.T) {
	f := newArchiveRecoveryFixture(t, 4)
	result, err := Build(f.snapshotConfig())
	require.NoError(t, err)
	manifest, err := LoadManifest(filepath.Join(result.SnapshotDir, manifestFileName))
	require.NoError(t, err)

	database := openArchiveRecoveryDatabase(
		t, datadir.Blockchain(result.SnapshotDir), true, f.logger,
	)
	require.NoError(t, validateBlockchain(database, manifest))

	manifest.StateRoot = types.EmptyRootHash
	require.NotEqual(t, result.StateRoot, manifest.StateRoot)
	require.ErrorContains(
		t,
		validateBlockchain(database, manifest),
		"final retained header state root mismatch",
	)
}

func TestRecoveryRetryAcceptsDifferentPhysicalConsensusExport(t *testing.T) {
	f := newArchiveRecoveryFixture(t, 6)
	_, err := BuildArchive(f.archiveConfig())
	require.NoError(t, err)
	first, err := BuildRecoveryCheckpoint(f.recoveryConfig())
	require.NoError(t, err)

	manifestPath := filepath.Join(first.CheckpointDir, manifestFileName)
	originalManifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	originalRelationship, err := os.ReadFile(first.RecoverySetPath)
	require.NoError(t, err)
	existing, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	otherRoot := t.TempDir()
	checkpoint, err := innbcbft.ExportSnapshotState(
		datadir.Consensus(f.source),
		datadir.Consensus(otherRoot),
	)
	require.NoError(t, err)
	require.Equal(t, first.CheckpointNumber, checkpoint)

	otherIdentity, err := snapshotConsensusFileIdentity(otherRoot)
	require.NoError(t, err)
	require.NotEqual(t, existing.ConsensusData.SHA256, otherIdentity.SHA256)

	before, err := os.Stat(first.CheckpointDir)
	require.NoError(t, err)
	require.NoError(t, os.Remove(first.LatestPath))

	retried, err := BuildRecoveryCheckpoint(f.recoveryConfig())
	require.NoError(t, err)
	require.Equal(t, first.CheckpointDir, retried.CheckpointDir)

	after, err := os.Stat(retried.CheckpointDir)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after))

	raw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.Equal(t, originalManifest, raw)
	raw, err = os.ReadFile(first.RecoverySetPath)
	require.NoError(t, err)
	require.Equal(t, originalRelationship, raw)
	require.FileExists(t, retried.LatestPath)
}

func TestRecoveryRetryRejectsChangedCommittedConsensusBytes(t *testing.T) {
	f := newArchiveRecoveryFixture(t, 6)
	_, err := BuildArchive(f.archiveConfig())
	require.NoError(t, err)
	result, err := BuildRecoveryCheckpoint(f.recoveryConfig())
	require.NoError(t, err)
	pointer, err := os.ReadFile(result.LatestPath)
	require.NoError(t, err)

	path := filepath.Join(result.CheckpointDir, filepath.FromSlash(snapshotConsensusDataPath))
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	require.NoError(t, err)
	first := []byte{0}
	_, err = file.ReadAt(first, 0)
	require.NoError(t, err)
	first[0] ^= 1
	_, err = file.WriteAt(first, 0)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	_, err = BuildRecoveryCheckpoint(f.recoveryConfig())
	require.ErrorContains(t, err, "consensus data checksum mismatch")
	unchanged, err := os.ReadFile(result.LatestPath)
	require.NoError(t, err)
	require.Equal(t, pointer, unchanged)
	require.DirExists(t, result.CheckpointDir)
}

func TestRecoveryRetryRejectsChangedManifestCommitment(t *testing.T) {
	f := newArchiveRecoveryFixture(t, 6)
	_, err := BuildArchive(f.archiveConfig())
	require.NoError(t, err)
	result, err := BuildRecoveryCheckpoint(f.recoveryConfig())
	require.NoError(t, err)

	path := filepath.Join(result.CheckpointDir, manifestFileName)
	manifest, err := LoadManifest(path)
	require.NoError(t, err)
	manifest.ConsensusData.SHA256 = strings.Repeat("ab", 32)
	writeArchiveRecoveryJSON(t, path, manifest)

	_, err = BuildRecoveryCheckpoint(f.recoveryConfig())
	require.ErrorContains(t, err, "manifest identity differs")
	require.DirExists(t, result.CheckpointDir)
}

/* FINISH ALPHA CHANGE 2106 -- test snapshot commitments and recovery retries across independent MDBX exports -- */

/* START ALPHA CHANGE 3334 -- test genesis-contiguous snapshot history and failure preservation -- */

type snapshotHistoryFloorOverride struct {
	storage.Storage
	floor uint64
}

func (s snapshotHistoryFloorOverride) HistoryFrom() uint64 {
	return s.floor
}

func TestSnapshotHistoryFloor_BuildAndReopen(t *testing.T) {
	cases := []struct {
		name           string
		head           uint64
		retention      uint64
		segment        uint64
		checkpointFrom uint64
		segmentedFrom  uint64
	}{
		{"first_block", 1, 4, 2, 1, 1},
		{"retention_larger_than_checkpoint", 3, 4, 2, 1, 1},
		{"retention_equals_checkpoint", 4, 4, 2, 1, 1},
		{"first_truncated_range", 5, 4, 2, 2, 3},
		{"completed_truncated_segment", 6, 4, 2, 3, 3},
		{"default_retention_window", 4, 43200, 1800, 1, 1},
	}

	for _, kind := range []Kind{KindCheckpoint, KindSegmented} {
		for _, tc := range cases {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				fixture := newArchiveRecoveryFixture(t, tc.head)
				config := fixture.snapshotConfig()
				config.Kind = kind
				config.RetentionBlocks = tc.retention
				if kind == KindSegmented {
					config.SegmentBlocks = tc.segment
				}

				wantFrom := tc.checkpointFrom
				if kind == KindSegmented {
					wantFrom = tc.segmentedFrom
				}
				wantFloor := wantFrom
				if wantFrom == 1 {
					wantFloor = 0
				}

				result, err := Build(config)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tc.head, result.BlockNumber)
				require.Equal(t, wantFrom, result.RetainedFrom)
				require.Equal(t, tc.head, result.RetainedTo)

				manifest, err := ValidatePublishedArtifact(
					result.SnapshotDir,
					kind,
					config.MDBXMapSizeGB,
					fixture.logger,
				)
				require.NoError(t, err)
				require.Equal(t, wantFrom, manifest.RetainedFrom)
				require.Equal(t, tc.head, manifest.RetainedTo)

				latest, err := LoadLatest(result.LatestPath)
				require.NoError(t, err)
				require.NoError(t, ValidateLatestManifest(latest, manifest))

				database := snapshotHistoryFloorOpen(
					t, config, result.SnapshotDir,
				)
				history, ok := database.(storage.HistoryAvailabilityStorage)
				require.True(t, ok)
				require.Equal(t, wantFloor, history.HistoryFrom())
				require.NoError(t, validateBlockchain(database, manifest))

				genesisHash, ok := database.ReadCanonicalHash(0)
				require.True(t, ok)
				require.Equal(t, manifest.GenesisHash, genesisHash)

				if wantFrom > 1 {
					_, ok := database.ReadCanonicalHash(wantFrom - 1)
					require.False(t, ok)
				}

				// A genesis-contiguous artifact must reject floor 1.
				// A truncated artifact must reject floor 0.
				wrongFloor := uint64(1)
				if wantFloor != 0 {
					wrongFloor = 0
				}
				err = validateBlockchain(snapshotHistoryFloorOverride{
					Storage: database,
					floor:   wrongFloor,
				}, manifest)
				require.EqualError(t, err, fmt.Sprintf(
					"snapshot history floor mismatch: expected %d, got %d",
					wantFloor,
					wrongFloor,
				))

				require.NoError(t, database.Close())
				snapshotDurabilityAssertNoStaging(t, config)
			})
		}
	}
}

func TestSnapshotHistoryFloor_RejectsMissingCanonicalBlocks(t *testing.T) {
	for _, kind := range []Kind{KindCheckpoint, KindSegmented} {
		t.Run(string(kind), func(t *testing.T) {
			fixture := newArchiveRecoveryFixture(t, 4)
			config := fixture.snapshotConfig()
			config.Kind = kind
			config.RetentionBlocks = 6
			if kind == KindSegmented {
				config.SegmentBlocks = 2
			}

			result, err := Build(config)
			require.NoError(t, err)
			manifest, err := LoadManifest(
				filepath.Join(result.SnapshotDir, manifestFileName),
			)
			require.NoError(t, err)
			require.Equal(t, uint64(1), manifest.RetainedFrom)

			for _, number := range []uint64{1, 3, 4} {
				t.Run(fmt.Sprint(number), func(t *testing.T) {
					// Mutate an independent copy, preserving the publication.
					damagedRoot := t.TempDir()
					copyArchiveRecoveryTree(
						t, result.SnapshotDir, damagedRoot,
					)
					database := snapshotHistoryFloorOpen(
						t, config, damagedRoot,
					)

					batch := database.NewBatch()
					if blockStorage, ok := database.(storage.BlockBatchStorage); ok {
						batch = blockStorage.NewBatchForBlock(number)
					}
					key := append(
						append([]byte(nil), storage.CANONICAL...),
						common.EncodeUint64ToBytes(number)...,
					)
					batch.Delete(key)
					require.NoError(t, batch.Write())
					require.NoError(t, database.Close())

					// Reopen so history metadata is calculated from the
					// damaged database rather than a cached earlier value.
					reopened := snapshotHistoryFloorOpen(
						t, config, damagedRoot,
					)
					require.EqualError(t,
						validateBlockchain(reopened, manifest),
						fmt.Sprintf(
							"snapshot canonical block %d is missing",
							number,
						),
					)
					require.NoError(t, reopened.Close())
				})
			}

			snapshotDurabilityAssertPublished(t, config, result)
		})
	}
}

func TestSnapshotHistoryFloor_FailurePreservesPublication(t *testing.T) {
	for _, kind := range []Kind{KindCheckpoint, KindSegmented} {
		for _, failure := range []string{"genesis_parent", "staging_sync"} {
			t.Run(string(kind)+"/"+failure, func(t *testing.T) {
				fixture := newArchiveRecoveryFixture(t, 2)
				config := fixture.snapshotConfig()
				config.Kind = kind
				config.RetentionBlocks = 6
				if kind == KindSegmented {
					config.SegmentBlocks = 2
				}

				first, err := Build(config)
				require.NoError(t, err)
				require.Equal(t, uint64(1), first.RetainedFrom)

				originalLatest, err := os.ReadFile(first.LatestPath)
				require.NoError(t, err)
				kindRoot := filepath.Join(
					config.OutputRoot, kind.directoryName(),
				)
				before := archiveRecoveryInventory(t, kindRoot)

				appendArchiveRecoveryBlocks(t, fixture, 4, 4)
				if failure == "genesis_parent" {
					snapshotHistoryFloorBreakGenesisLink(t, fixture)
				}

				syncErr := errors.New("injected history-floor staging sync failure")
				syncCalls := 0
				result, err := buildWithOptions(config, buildOptions{
					syncStaging: func(string) error {
						syncCalls++
						return syncErr
					},
				})

				require.Nil(t, result)
				if failure == "genesis_parent" {
					require.ErrorContains(t, err,
						"snapshot artifact validation failed")
					require.ErrorContains(t, err,
						"snapshot parent hash mismatch at block 1")
					require.Zero(t, syncCalls,
						"invalid history reached staging synchronization")
				} else {
					require.ErrorIs(t, err, syncErr)
					require.ErrorContains(t, err,
						"failed to synchronize snapshot staging")
					require.Equal(t, 1, syncCalls)
				}

				currentLatest, readErr := os.ReadFile(first.LatestPath)
				require.NoError(t, readErr)
				require.Equal(t, originalLatest, currentLatest)
				require.Equal(t, before,
					archiveRecoveryInventory(t, kindRoot),
					"failure changed the previous snapshot publication",
				)
				require.DirExists(t, first.SnapshotDir)
				snapshotDurabilityAssertPublished(t, config, first)
				snapshotDurabilityAssertNoStaging(t, config)
			})
		}
	}
}

func snapshotHistoryFloorOpen(
	t *testing.T,
	config Config,
	root string,
) storage.Storage {
	t.Helper()

	database, err := openTargetBlockchain(
		config,
		datadir.Blockchain(root),
		archiveRecoveryTestMDBXMapSize,
		config.Logger,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = database.Close()
	})

	return database
}

func snapshotHistoryFloorBreakGenesisLink(
	t *testing.T,
	fixture archiveRecoveryFixture,
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
	require.Greater(t, head, uint64(0))

	genesisHash, ok := database.ReadCanonicalHash(0)
	require.True(t, ok)
	parentHash := types.StringToHash("0xdead")
	require.NotEqual(t, genesisHash, parentHash)

	// Rebuild every affected header and its associated records so the
	// candidate has consistent hashes, bodies, receipts and internal links.
	// Its deliberate defect is block 1's parent differing from genesis.
	for number := uint64(1); number <= head; number++ {
		header, err := readArchiveCanonicalHeader(database, number)
		require.NoError(t, err)
		body, err := database.ReadBody(header.Hash)
		require.NoError(t, err)
		receipts, err := database.ReadReceipts(header.Hash)
		require.NoError(t, err)

		header.ParentHash = parentHash
		header.ComputeHash()

		batch := storage.NewBatchWriterForBlock(database, number)
		batch.PutCanonicalHeader(header)
		batch.PutBody(header.Hash, body)
		batch.PutReceipts(header.Hash, receipts)
		for _, transaction := range body.Transactions {
			batch.PutTxLookup(transaction.Hash, header.Hash)
		}
		require.NoError(t, batch.WriteBatch())
		parentHash = header.Hash
	}

	require.NoError(t, database.Close())
}

/* FINISH ALPHA CHANGE 3334 -- test genesis-contiguous snapshot history and failure preservation -- */
