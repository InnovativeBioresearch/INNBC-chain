/* START ALPHA CHANGE 2266 -- test independent archival database backup publication -- */
package snapshotbuilder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/stretchr/testify/require"
)

func databaseBackupTestConfig(
	t *testing.T,
	fixture archiveRecoveryFixture,
) DatabaseBackupConfig {
	t.Helper()

	return DatabaseBackupConfig{
		ChainPath:     fixture.chainPath,
		DataDir:       fixture.source,
		OutputRoot:    filepath.Join(t.TempDir(), "backups"),
		MDBXMapSizeGB: 1,
		Logger:        fixture.logger,
	}
}

func TestDatabaseBackupPublication(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 4)
	config := databaseBackupTestConfig(t, fixture)

	// A database archive must not depend on trie, consensus or node keys.
	require.NoError(t, os.RemoveAll(datadir.Trie(fixture.source)))
	require.NoError(t, os.RemoveAll(datadir.Consensus(fixture.source)))

	originalGenesis, err := os.ReadFile(fixture.chainPath)
	require.NoError(t, err)
	sourceBefore, err := archiveFileIdentity(
		filepath.Join(datadir.Blockchain(fixture.source), "mdbx.dat"),
	)
	require.NoError(t, err)

	first, err := BuildDatabaseBackup(context.Background(), config)
	require.NoError(t, err)
	require.Equal(t, uint64(4), first.BlockNumber)

	genesisBytes, err := os.ReadFile(filepath.Join(first.Directory, "genesis.json"))
	require.NoError(t, err)
	require.Equal(t, originalGenesis, genesisBytes)

	raw, err := os.ReadFile(filepath.Join(first.Directory, "backup.json"))
	require.NoError(t, err)
	var manifest DatabaseBackupManifest
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "blockchain-database-backup", manifest.Kind)
	require.Equal(t, first.BlockHash, manifest.BlockHash)
	require.Equal(t, first.StateRoot, manifest.StateRoot)
	require.Positive(t, manifest.Records)
	require.Len(t, manifest.RecordsSHA256, 64)

	require.NoError(t, verifyArchiveFile(
		filepath.Join(first.Directory, manifest.GenesisFile.Path),
		manifest.GenesisFile,
	))
	require.NoError(t, verifyArchiveFile(
		filepath.Join(first.Directory, filepath.FromSlash(manifest.BlockchainFile.Path)),
		manifest.BlockchainFile,
	))

	require.NoDirExists(t, datadir.Trie(first.Directory))
	require.NoDirExists(t, datadir.Consensus(first.Directory))
	require.NoFileExists(t, filepath.Join(first.Directory, "manifest.json"))
	require.NoFileExists(t, filepath.Join(first.Directory, "latest.json"))
	require.NoFileExists(t, filepath.Join(config.OutputRoot, "latest.json"))
	require.NoFileExists(t, filepath.Join(datadir.Blockchain(first.Directory), "mdbx.lck"))

	database, err := mdbxstorage.NewMDBXStorageReadOnly(
		datadir.Blockchain(first.Directory), fixture.logger,
	)
	require.NoError(t, err)
	header, err := readArchiveCanonicalHeader(database, 1)
	require.NoError(t, err)
	body, err := database.ReadBody(header.Hash)
	require.NoError(t, err)
	require.NotEmpty(t, body.Transactions)
	for _, transaction := range body.Transactions {
		hash, found := database.ReadTxLookup(transaction.Hash)
		require.True(t, found)
		require.Equal(t, header.Hash, hash)
	}
	require.NoError(t, database.Close())

	second, err := BuildDatabaseBackup(context.Background(), config)
	require.NoError(t, err)
	require.NotEqual(t, first.Directory, second.Directory)
	require.DirExists(t, first.Directory)

	unchangedManifest, err := os.ReadFile(filepath.Join(first.Directory, "backup.json"))
	require.NoError(t, err)
	require.Equal(t, raw, unchangedManifest)

	sourceAfter, err := archiveFileIdentity(
		filepath.Join(datadir.Blockchain(fixture.source), "mdbx.dat"),
	)
	require.NoError(t, err)
	require.Equal(t, sourceBefore, sourceAfter)
}

func TestDatabaseBackupRejectsIncompleteHistory(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 4)
	config := databaseBackupTestConfig(t, fixture)

	database := openArchiveRecoveryDatabase(
		t, datadir.Blockchain(fixture.source), false, fixture.logger,
	)
	batch := database.NewBatch()
	key := append(
		append([]byte(nil), storage.CANONICAL...),
		common.EncodeUint64ToBytes(2)...,
	)
	batch.Delete(key)
	require.NoError(t, batch.Write())
	require.NoError(t, database.Close())

	_, err := BuildDatabaseBackup(context.Background(), config)
	require.ErrorContains(t, err, "complete history")
	assertNoDatabaseBackupPublished(t, config.OutputRoot)
}

func TestDatabaseBackupRejectsWrongGenesisHeader(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 2)
	config := databaseBackupTestConfig(t, fixture)

	changed, err := chain.ImportFromFile(fixture.chainPath)
	require.NoError(t, err)
	changed.Genesis.Timestamp++
	raw, err := json.Marshal(changed)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fixture.chainPath, raw, 0600))

	_, err = BuildDatabaseBackup(context.Background(), config)
	require.ErrorContains(t, err, "genesis header differs")
	assertNoDatabaseBackupPublished(t, config.OutputRoot)
}

func TestDatabaseBackupRejectsCancelledOperation(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 2)
	config := databaseBackupTestConfig(t, fixture)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := BuildDatabaseBackup(ctx, config)
	require.ErrorIs(t, err, context.Canceled)
	require.NoDirExists(t, config.OutputRoot)
}

func TestDatabaseBackupRejectsSourceOutputDirectory(t *testing.T) {
	fixture := newArchiveRecoveryFixture(t, 2)
	config := databaseBackupTestConfig(t, fixture)
	config.OutputRoot = filepath.Join(fixture.source, "backups")

	_, err := BuildDatabaseBackup(context.Background(), config)
	require.ErrorContains(t, err, "outside the source")
	require.NoDirExists(t, config.OutputRoot)
}

func assertNoDatabaseBackupPublished(t *testing.T, root string) {
	t.Helper()

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, strings.HasPrefix(entry.Name(), "database-backup-"))
		require.False(t, strings.HasPrefix(entry.Name(), ".database-backup-building-"))
	}
	require.NoFileExists(t, filepath.Join(root, "latest.json"))
}

/* FINISH ALPHA CHANGE 2266 -- test independent archival database backup publication -- */
