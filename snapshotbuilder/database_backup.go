/* START ALPHA CHANGE 2262 -- build standalone blockchain database archives without snapshot activation -- */
package snapshotbuilder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"

	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"

	/* START ALPHA CHANGE 3469 -- configure standalone backup destination pages -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3469 -- configure standalone backup destination pages -- */
	"github.com/innovativebioresearch/innbc-chain/types"
)

const databaseBackupFormatVersion uint64 = 1

type DatabaseBackupConfig struct {
	ChainPath     string
	DataDir       string
	OutputRoot    string
	MDBXMapSizeGB uint64
	/* START ALPHA CHANGE 3469 -- retain backup creation page size -- */
	MDBXPageSizeBytes uint64
	/* FINISH ALPHA CHANGE 3469 -- retain backup creation page size -- */
	Logger hclog.Logger
}

type DatabaseBackupManifest struct {
	FormatVersion uint64 `json:"format_version"`
	Kind          string `json:"kind"`
	CreatedUTC    string `json:"created_utc"`

	ChainID          int64      `json:"chain_id"`
	GenesisHash      types.Hash `json:"genesis_hash"`
	GenesisStateRoot types.Hash `json:"genesis_state_root"`
	BlockNumber      uint64     `json:"block_number"`
	BlockHash        types.Hash `json:"block_hash"`
	StateRoot        types.Hash `json:"state_root"`

	Records       uint64 `json:"records"`
	RecordsSHA256 string `json:"records_sha256"`

	GenesisFile    ArchiveFile `json:"genesis_file"`
	BlockchainFile ArchiveFile `json:"blockchain_file"`
}

type DatabaseBackupResult struct {
	Directory   string     `json:"directory"`
	BlockNumber uint64     `json:"block_number"`
	BlockHash   types.Hash `json:"block_hash"`
	StateRoot   types.Hash `json:"state_root"`
}

// BuildDatabaseBackup is a standalone command operation.
//
// Normal writes to the source database may continue in another process.
// Source replacement, pruning, restore and other maintenance must not overlap
// the export. This function initializes the INNBC header-hash function and is
// not intended to be invoked inside the running node.
//
// It verifies record-copy equality and readable continuous canonical history.
// It does not execute transactions or independently regenerate genesis state.
func BuildDatabaseBackup(
	ctx context.Context,
	config DatabaseBackupConfig,
) (result *DatabaseBackupResult, err error) {
	if ctx == nil {
		return nil, errors.New("backup context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.ChainPath) == "" ||
		strings.TrimSpace(config.DataDir) == "" ||
		strings.TrimSpace(config.OutputRoot) == "" {
		return nil, errors.New("chain, data directory and backup output root are required")
	}
	if config.MDBXMapSizeGB == 0 {
		return nil, errors.New("backup MDBX map size must be positive")
	}
	/* START ALPHA CHANGE 3469 -- validate destination pages before staging a backup -- */
	if _, err := mdbxconfig.Resolve(config.MDBXPageSizeBytes); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3469 -- validate destination pages before staging a backup -- */

	mapSize, err := mapSizeBytes(config.MDBXMapSizeGB)
	if err != nil {
		return nil, err
	}

	sourceRoot, err := filepath.EvalSymlinks(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve source data directory: %w", err)
	}
	sourceRoot, err = filepath.Abs(sourceRoot)
	if err != nil {
		return nil, err
	}
	sourcePath := datadir.Blockchain(sourceRoot)
	if err := requireArchiveDirectory(sourcePath); err != nil {
		return nil, err
	}
	if _, err := archiveRegularFileSize(
		filepath.Join(sourcePath, "mdbx.dat"),
		^uint64(0),
	); err != nil {
		return nil, fmt.Errorf(
			"backup requires a unified blockchain MDBX database: %w", err,
		)
	}

	outputRoot, err := filepath.Abs(config.OutputRoot)
	if err != nil {
		return nil, err
	}
	if backupPathWithin(sourceRoot, outputRoot) {
		return nil, errors.New("backup output must be outside the source node data directory")
	}
	outputRoot, err = canonicalBuildRoot(outputRoot)
	if err != nil {
		return nil, err
	}
	if backupPathWithin(sourceRoot, outputRoot) {
		return nil, errors.New("resolved backup output is inside the source node data directory")
	}

	// This lock serializes only backup publication into OutputRoot.
	// It does not acquire a node-process lock or stop source writes.
	writerLock, err := acquireBuildLock(outputRoot)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, writerLock.Close())
	}()

	stage, err := os.MkdirTemp(outputRoot, ".database-backup-building-")
	if err != nil {
		return nil, err
	}
	defer func() {
		// After publication the staging path no longer exists.
		err = errors.Join(err, os.RemoveAll(stage))
	}()

	// Parse the exact bytes that will be archived, not a second read of the
	// operator's file that could have changed during the operation.
	genesisBytes, err := os.ReadFile(config.ChainPath)
	if err != nil {
		return nil, fmt.Errorf("read backup genesis configuration: %w", err)
	}
	genesisPath := filepath.Join(stage, "genesis.json")
	if err := writeArchiveFileDurable(genesisPath, genesisBytes, true); err != nil {
		return nil, err
	}

	chainConfig, err := chain.ImportFromFile(genesisPath)
	if err != nil {
		return nil, fmt.Errorf("validate backup genesis configuration: %w", err)
	}
	if chainConfig.Params.GetEngine() != innbcbft.ConsensusName {
		return nil, errors.New("database backup requires INNBCBFT")
	}
	if chainConfig.Params.ChainID <= 0 {
		return nil, errors.New("backup chain ID must be positive")
	}
	innbcbft.SetupHeaderHashFunc()

	logger := config.Logger
	if logger == nil {
		logger = hclog.NewNullLogger()
	}
	logger.Info("copying blockchain database from a fixed MDBX read view")

	if err := os.MkdirAll(datadir.ChainData(stage), 0700); err != nil {
		return nil, err
	}
	destination := datadir.Blockchain(stage)
	copied, err := mdbxstorage.CopyBlockchainDatabase(
		/* START ALPHA CHANGE 3469 -- forward backup destination pages -- */
		ctx, sourcePath, destination, mapSize, config.MDBXPageSizeBytes,
		/* FINISH ALPHA CHANGE 3469 -- forward backup destination pages -- */
	)
	if err != nil {
		return nil, fmt.Errorf("copy blockchain database: %w", err)
	}

	logger.Info("checking archived canonical history")

	genesis, head, err := inspectDatabaseBackup(
		ctx, destination, chainConfig, logger,
	)
	if err != nil {
		return nil, err
	}

	// inspectDatabaseBackup has closed its read-only destination environment.
	if err := os.Remove(filepath.Join(destination, "mdbx.lck")); err != nil &&
		!errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	genesisFile, err := archiveFileIdentity(genesisPath)
	if err != nil {
		return nil, err
	}
	genesisFile.Path = "genesis.json"

	databaseFile, err := archiveFileIdentity(filepath.Join(destination, "mdbx.dat"))
	if err != nil {
		return nil, err
	}
	databaseFile.Path = "chain-data/blockchain/mdbx.dat"

	now := time.Now().UTC()
	manifest := &DatabaseBackupManifest{
		FormatVersion:    databaseBackupFormatVersion,
		Kind:             "blockchain-database-backup",
		CreatedUTC:       now.Format(time.RFC3339Nano),
		ChainID:          chainConfig.Params.ChainID,
		GenesisHash:      genesis.Hash,
		GenesisStateRoot: genesis.StateRoot,
		BlockNumber:      head.Number,
		BlockHash:        head.Hash,
		StateRoot:        head.StateRoot,
		Records:          copied.Records,
		RecordsSHA256:    copied.RecordsSHA256,
		GenesisFile:      genesisFile,
		BlockchainFile:   databaseFile,
	}
	if err := writeArchiveJSONDurable(
		filepath.Join(stage, "backup.json"), manifest,
	); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	suffix := strings.TrimPrefix(
		filepath.Base(stage), ".database-backup-building-",
	)
	name := fmt.Sprintf(
		"database-backup-%d-%s-%s",
		head.Number,
		now.Format("20060102T150405Z"),
		suffix,
	)
	finalDirectory := filepath.Join(outputRoot, name)

	// Reuse durable directory publication, but publish no snapshot manifest
	// or latest pointer. Backup directories are not startup artifacts.
	if err := publishArchiveSegment(outputRoot, stage, finalDirectory); err != nil {
		return nil, fmt.Errorf("publish database backup: %w", err)
	}

	return &DatabaseBackupResult{
		Directory:   finalDirectory,
		BlockNumber: head.Number,
		BlockHash:   head.Hash,
		StateRoot:   head.StateRoot,
	}, nil
}

func backupPathWithin(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}

	return relative == "." ||
		(relative != ".." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func inspectDatabaseBackup(
	ctx context.Context,
	path string,
	config *chain.Chain,
	logger hclog.Logger,
) (genesis *types.Header, head *types.Header, err error) {
	database, err := mdbxstorage.NewMDBXStorageReadOnly(path, logger)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		err = errors.Join(err, database.Close())
	}()

	number, ok := database.ReadHeadNumber()
	if !ok {
		return nil, nil, errors.New("backup database head number is missing")
	}
	head, err = readArchiveCanonicalHeader(database, number)
	if err != nil {
		return nil, nil, err
	}
	headHash, ok := database.ReadHeadHash()
	if !ok || headHash != head.Hash {
		return nil, nil, errors.New("backup database head records disagree")
	}

	genesis, err = readArchiveCanonicalHeader(database, 0)
	if err != nil {
		return nil, nil, err
	}

	// Check configured header identity against the stored genesis root.
	// Allocation execution is intentionally left to node trie-rebuild.
	expectedGenesis := config.Genesis.GenesisHeader()
	expectedGenesis.StateRoot = genesis.StateRoot
	expectedGenesis.ComputeHash()
	if expectedGenesis.Hash != genesis.Hash {
		return nil, nil, errors.New("backup genesis header differs from configuration")
	}

	previous := genesis
	for height := uint64(1); height <= number; height++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		header, err := readArchiveCanonicalHeader(database, height)
		if err != nil {
			return nil, nil, fmt.Errorf("backup requires complete history: %w", err)
		}
		if header.ParentHash != previous.Hash {
			return nil, nil, fmt.Errorf("backup parent mismatch at block %d", height)
		}

		body, err := database.ReadBody(header.Hash)
		if err != nil {
			return nil, nil, fmt.Errorf("read backup body %d: %w", height, err)
		}
		receipts, err := database.ReadReceipts(header.Hash)
		if err != nil {
			return nil, nil, fmt.Errorf("read backup receipts %d: %w", height, err)
		}
		if body == nil || len(body.Transactions) != len(receipts) {
			return nil, nil, fmt.Errorf("backup body/receipt count mismatch at %d", height)
		}
		for index, transaction := range body.Transactions {
			if transaction == nil || receipts[index] == nil {
				return nil, nil, fmt.Errorf("nil backup transaction or receipt at %d", height)
			}
		}

		previous = header
		if height == number {
			break // Also avoids overflow at the maximum uint64 height.
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	return genesis, head, nil
}

/* FINISH ALPHA CHANGE 2262 -- build standalone blockchain database archives without snapshot activation -- */
