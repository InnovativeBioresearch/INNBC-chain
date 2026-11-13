/* START ALPHA CHANGE 3489 -- verify destination geometry across snapshot and archive workflows -- */
package snapshotbuilder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	"github.com/stretchr/testify/require"
)

func TestMDBXPageSize_SnapshotDestinations(t *testing.T) {
	for _, kind := range []Kind{KindCheckpoint, KindSegmented} {
		t.Run(string(kind), func(t *testing.T) {
			fixture := newArchiveRecoveryFixture(t, 4)
			config := snapshotDurabilityConfig(fixture, kind)
			config.MDBXPageSizeBytes = 8192

			sourceBefore := archiveRecoveryInventory(t, datadir.ChainData(fixture.source))
			result, err := Build(config)
			require.NoError(t, err)

			require.NoError(t, mdbxconfig.CheckChainData(
				datadir.ChainData(result.SnapshotDir), 8192,
			))
			_, err = ValidatePublishedArtifact(
				result.SnapshotDir, kind, config.MDBXMapSizeGB, fixture.logger, 8192,
			)
			require.NoError(t, err)

			_, err = ValidatePublishedArtifact(
				result.SnapshotDir, kind, config.MDBXMapSizeGB, fixture.logger, 4096,
			)
			require.ErrorIs(t, err, mdbxconfig.ErrPageSizeMismatch)

			require.Equal(t, sourceBefore,
				archiveRecoveryInventory(t, datadir.ChainData(fixture.source)))

			latestBefore, err := os.ReadFile(result.LatestPath)
			require.NoError(t, err)
			config.MDBXPageSizeBytes = 12345
			_, err = Build(config)
			require.Error(t, err)
			latestAfter, err := os.ReadFile(result.LatestPath)
			require.NoError(t, err)
			require.Equal(t, latestBefore, latestAfter)
		})
	}
}

func TestMDBXPageSize_ArchiveRecoveryAndBackup(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 5)

	archiveConfig := fixture.archiveConfig()
	archiveConfig.MDBXPageSizeBytes = 8192
	_, err := BuildArchive(archiveConfig)
	require.NoError(t, err)
	require.NoError(t, mdbxconfig.CheckTree(fixture.archive, 8192))

	recoveryConfig := fixture.recoveryConfig()
	recoveryConfig.MDBXPageSizeBytes = 8192
	recovery, err := BuildRecoveryCheckpoint(recoveryConfig)
	require.NoError(t, err)
	require.NoError(t, mdbxconfig.CheckChainData(
		datadir.ChainData(recovery.CheckpointDir), 8192,
	))

	reused, err := BuildRecoveryCheckpoint(recoveryConfig)
	require.NoError(t, err)
	require.Equal(t, recovery.CheckpointDir, reused.CheckpointDir)

	archiveConfig.MDBXPageSizeBytes = 4096
	_, err = BuildArchive(archiveConfig)
	require.ErrorIs(t, err, mdbxconfig.ErrPageSizeMismatch)

	recoveryConfig.MDBXPageSizeBytes = 4096
	_, err = BuildRecoveryCheckpoint(recoveryConfig)
	require.ErrorIs(t, err, mdbxconfig.ErrPageSizeMismatch)

	backup, err := BuildDatabaseBackup(context.Background(), DatabaseBackupConfig{
		ChainPath:         fixture.chainPath,
		DataDir:           fixture.source,
		OutputRoot:        filepath.Join(t.TempDir(), "backups"),
		MDBXMapSizeGB:     1,
		MDBXPageSizeBytes: 8192,
		Logger:            fixture.logger,
	})
	require.NoError(t, err)
	require.NoError(t, mdbxconfig.CheckTree(backup.Directory, 8192))
}

/* START ALPHA CHANGE 3503 -- verify recovery archive compatibility after lock acquisition -- */
func TestMDBXPageSize_RecoveryRechecksArchiveAfterLock(t *testing.T) {
	for _, tc := range []struct {
		name            string
		checkpointPages uint64
		archivePages    uint64
		wantMismatch    bool
	}{
		{
			name:            "4096_matches",
			checkpointPages: 4096,
			archivePages:    4096,
		},
		{
			name:            "8192_matches",
			checkpointPages: 8192,
			archivePages:    8192,
		},
		{
			name:            "4096_rejects_8192",
			checkpointPages: 4096,
			archivePages:    8192,
			wantMismatch:    true,
		},
		{
			name:            "8192_rejects_4096",
			checkpointPages: 8192,
			archivePages:    4096,
			wantMismatch:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newArchiveRecoveryFixture(t, 3)

			// Preserve a previously published snapshot while attempting
			// recovery construction at a later checkpoint.
			snapshotConfig := fixture.snapshotConfig()
			snapshotConfig.MDBXPageSizeBytes = tc.checkpointPages
			previous, err := Build(snapshotConfig)
			require.NoError(t, err)

			checkpointRoot := filepath.Dir(previous.SnapshotDir)
			checkpointBefore := archiveRecoveryInventory(t, checkpointRoot)

			appendArchiveRecoveryBlocks(t, fixture, 5, 5)
			sourceBefore := archiveRecoveryInventory(
				t, datadir.ChainData(fixture.source),
			)

			// The initial compatibility check sees an empty archive root.
			require.NoError(t, os.MkdirAll(fixture.archive, 0770))

			recoveryConfig := fixture.recoveryConfig()
			recoveryConfig.MDBXPageSizeBytes = tc.checkpointPages

			var archiveBefore map[string]archiveRecoveryFileState
			acquireCalls := 0

			result, err := buildRecoveryCheckpoint(
				recoveryConfig,
				func(roots ...string) (buildLock, error) {
					acquireCalls++

					entries, readErr := os.ReadDir(fixture.archive)
					if readErr != nil {
						return nil, readErr
					}
					require.Empty(t, entries)

					// Complete an archive publication after preflight
					// but before recovery acquires its real locks.
					archiveConfig := fixture.archiveConfig()
					archiveConfig.MDBXPageSizeBytes = tc.archivePages
					if _, buildErr := BuildArchive(archiveConfig); buildErr != nil {
						return nil, buildErr
					}

					archiveBefore = archiveRecoveryInventory(t, fixture.archive)

					return acquireBuildLocks(roots...)
				},
			)

			require.Equal(t, 1, acquireCalls)
			require.NotEmpty(t, archiveBefore)

			if tc.wantMismatch {
				require.ErrorIs(t, err, mdbxconfig.ErrPageSizeMismatch)
				require.ErrorContains(t, err, "linked archive page-size compatibility")
				require.Nil(t, result)
				require.Equal(t, checkpointBefore,
					archiveRecoveryInventory(t, checkpointRoot),
					"compatibility rejection changed the previous publication",
				)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, uint64(5), result.CheckpointNumber)
				require.NoError(t, mdbxconfig.CheckChainData(
					datadir.ChainData(result.CheckpointDir),
					tc.checkpointPages,
				))
				assertArchiveRecoveryRelationship(t, fixture, result)
			}

			snapshotDurabilityAssertNoStaging(t, snapshotConfig)

			require.Equal(t, archiveBefore,
				archiveRecoveryInventory(t, fixture.archive),
				"recovery construction changed the published archive",
			)
			require.Equal(t, sourceBefore,
				archiveRecoveryInventory(t, datadir.ChainData(fixture.source)),
				"recovery construction changed the source databases",
			)

			// Success and compatibility rejection must both release
			// the archive and checkpoint publication locks.
			locks, lockErr := acquireBuildLocks(fixture.archive, checkpointRoot)
			require.NoError(t, lockErr)
			t.Cleanup(func() {
				_ = locks.Close()
			})
			require.NoError(t, locks.Close())
		})
	}
}

/* FINISH ALPHA CHANGE 3503 -- verify recovery archive compatibility after lock acquisition -- */

/* FINISH ALPHA CHANGE 3489 -- verify destination geometry across snapshot and archive workflows -- */
