/* START ALPHA CHANGE 3289 -- verify ordinary snapshot durability ordering and failure preservation -- */
package snapshotbuilder

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/stretchr/testify/require"
)

func TestSnapshotDurability_PublicationOrdering(t *testing.T) {
	for _, kind := range []Kind{KindCheckpoint, KindSegmented} {
		t.Run(string(kind), func(t *testing.T) {
			fixture := newArchiveRecoveryFixture(t, 4)
			config := snapshotDurabilityConfig(fixture, kind)

			first, err := Build(config)
			require.NoError(t, err)

			originalLatest, err := os.ReadFile(first.LatestPath)
			require.NoError(t, err)
			originalFiles := archiveRecoveryInventory(t, first.SnapshotDir)

			appendArchiveRecoveryBlocks(t, fixture, 6, 6)

			var (
				syncCalls   int
				stagedPath  string
				finalPath   string
				stagedFiles map[string]archiveRecoveryFileState
			)

			second, err := buildWithOptions(config, buildOptions{
				syncStaging: func(directory string) error {
					syncCalls++
					stagedPath = directory

					require.Equal(t,
						filepath.Join(config.OutputRoot, kind.directoryName()),
						filepath.Dir(directory),
					)
					require.True(t,
						strings.HasPrefix(filepath.Base(directory), ".building-"),
					)

					// The complete candidate is readable before synchronization.
					// This extra validation runs only in the regression test.
					manifest, err := ValidatePublishedArtifact(
						directory, kind, config.MDBXMapSizeGB, fixture.logger,
					)
					require.NoError(t, err)
					require.Equal(t, uint64(6), manifest.BlockNumber)

					finalPath = filepath.Join(
						filepath.Dir(directory),
						snapshotDirectoryName(manifest.BlockNumber, manifest.BlockHash),
					)
					require.NoDirExists(t, finalPath)

					// Publication and retirement must not precede this barrier.
					currentLatest, err := os.ReadFile(first.LatestPath)
					require.NoError(t, err)
					require.Equal(t, originalLatest, currentLatest)
					require.Equal(t, originalFiles,
						archiveRecoveryInventory(t, first.SnapshotDir),
					)

					stagedFiles = archiveRecoveryInventory(t, directory)
					return syncArchiveTree(directory)
				},
			})
			require.NoError(t, err)
			require.NotNil(t, second)
			require.Equal(t, 1, syncCalls)
			require.Equal(t, uint64(6), second.BlockNumber)
			require.Equal(t, finalPath, second.SnapshotDir)

			// The synchronized candidate is the artifact actually published.
			require.Equal(t, stagedFiles,
				archiveRecoveryInventory(t, second.SnapshotDir),
			)
			require.NoDirExists(t, stagedPath)
			require.NoDirExists(t, first.SnapshotDir)

			snapshotDurabilityAssertPublished(t, config, second)
			snapshotDurabilityAssertNoStaging(t, config)
		})
	}
}

func TestSnapshotDurability_SyncFailurePreservesPrevious(t *testing.T) {
	for _, kind := range []Kind{KindCheckpoint, KindSegmented} {
		t.Run(string(kind), func(t *testing.T) {
			fixture := newArchiveRecoveryFixture(t, 4)
			config := snapshotDurabilityConfig(fixture, kind)

			first, err := Build(config)
			require.NoError(t, err)

			kindRoot := filepath.Join(config.OutputRoot, kind.directoryName())
			before := archiveRecoveryInventory(t, kindRoot)

			appendArchiveRecoveryBlocks(t, fixture, 6, 6)

			syncErr := errors.New("injected staging synchronization failure")
			var (
				syncCalls  int
				stagedPath string
				finalPath  string
			)

			result, err := buildWithOptions(config, buildOptions{
				syncStaging: func(directory string) error {
					syncCalls++
					stagedPath = directory

					manifest, err := LoadManifest(
						filepath.Join(directory, manifestFileName),
					)
					require.NoError(t, err)
					require.Equal(t, uint64(6), manifest.BlockNumber)

					finalPath = filepath.Join(
						kindRoot,
						snapshotDirectoryName(manifest.BlockNumber, manifest.BlockHash),
					)
					require.NoDirExists(t, finalPath)

					return syncErr
				},
			})

			require.Nil(t, result)
			require.ErrorIs(t, err, syncErr)
			require.ErrorContains(t, err, "failed to synchronize snapshot staging")
			require.Equal(t, 1, syncCalls)

			require.NoDirExists(t, stagedPath)
			require.NoDirExists(t, finalPath)
			require.Equal(t, before, archiveRecoveryInventory(t, kindRoot),
				"synchronization failure changed the existing publication",
			)

			snapshotDurabilityAssertPublished(t, config, first)
			snapshotDurabilityAssertNoStaging(t, config)
		})
	}
}

func TestSnapshotDurability_ValidationFailureBeforeSync(t *testing.T) {
	for _, kind := range []Kind{KindCheckpoint, KindSegmented} {
		t.Run(string(kind), func(t *testing.T) {
			fixture := newArchiveRecoveryFixture(t, 4)
			config := snapshotDurabilityConfig(fixture, kind)

			first, err := Build(config)
			require.NoError(t, err)

			kindRoot := filepath.Join(config.OutputRoot, kind.directoryName())
			before := archiveRecoveryInventory(t, kindRoot)

			appendArchiveRecoveryBlocks(t, fixture, 6, 6)

			// Preserve readable source records, but make the copied receipts
			// disagree with their header commitment. Artifact validation,
			// rather than source copying, must reject this candidate.
			database := openArchiveRecoveryDatabase(
				t, datadir.Blockchain(fixture.source), false, fixture.logger,
			)
			header, err := readArchiveCanonicalHeader(database, 6)
			require.NoError(t, err)

			receipts, err := database.ReadReceipts(header.Hash)
			require.NoError(t, err)
			require.NotEmpty(t, receipts)
			receipts[0].CumulativeGasUsed++

			batch := storage.NewBatchWriterForBlock(database, 6)
			batch.PutReceipts(header.Hash, receipts)
			require.NoError(t, batch.WriteBatch())
			require.NoError(t, database.Close())

			syncCalls := 0
			result, err := buildWithOptions(config, buildOptions{
				syncStaging: func(string) error {
					syncCalls++
					return errors.New("synchronization reached before validation")
				},
			})

			require.Nil(t, result)
			require.ErrorContains(t, err, "snapshot artifact validation failed")
			require.ErrorContains(t, err, "snapshot receipt root mismatch at block 6")
			require.Zero(t, syncCalls)

			require.Equal(t, before, archiveRecoveryInventory(t, kindRoot),
				"validation failure changed the existing publication",
			)
			snapshotDurabilityAssertPublished(t, config, first)
			snapshotDurabilityAssertNoStaging(t, config)
		})
	}
}

/* START ALPHA CHANGE 3290 -- isolate durability regressions from genesis-contiguous history-floor validation -- */
func snapshotDurabilityConfig(fixture archiveRecoveryFixture, kind Kind) Config {
	config := fixture.snapshotConfig()
	config.Kind = kind

	// At heads 4 and 6, both formats retain a truncated suffix.
	// Genesis-contiguous history-floor handling is a separate validation case.
	config.RetentionBlocks = 2
	if kind == KindSegmented {
		config.SegmentBlocks = 1
	}
	return config
}

/* FINISH ALPHA CHANGE 3290 -- isolate durability regressions from genesis-contiguous history-floor validation -- */

func snapshotDurabilityAssertPublished(
	t *testing.T,
	config Config,
	result *Result,
) {
	t.Helper()

	latest, err := LoadLatest(result.LatestPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Base(result.SnapshotDir), latest.SnapshotDir)
	require.Equal(t, result.BlockNumber, latest.BlockNumber)

	manifest, err := ValidatePublishedArtifact(
		result.SnapshotDir, config.Kind, config.MDBXMapSizeGB, config.Logger,
	)
	require.NoError(t, err)
	require.NoError(t, ValidateLatestManifest(latest, manifest))
}

func snapshotDurabilityAssertNoStaging(t *testing.T, config Config) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(
		config.OutputRoot, config.Kind.directoryName(), ".building-*",
	))
	require.NoError(t, err)
	require.Empty(t, matches, "unpublished staging directories remain")
}

/* FINISH ALPHA CHANGE 3289 -- verify ordinary snapshot durability ordering and failure preservation -- */
