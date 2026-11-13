/* START ALPHA CHANGE 2067 -- build consistent append-only historical archives -- */
package snapshotbuilder

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"

	/* START ALPHA CHANGE 3467 -- preserve uniform archive database pages -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3467 -- preserve uniform archive database pages -- */
	"github.com/innovativebioresearch/innbc-chain/types"
)

// BuildArchive exports complete canonical block ranges.
//
// The caller must serialize writers to OutputRoot. Cross-process locking is
// integrated separately. Source replacement, history deletion, and snapshot
// activation must not run concurrently with export.
func BuildArchive(config ArchiveConfig) (*ArchiveResult, error) {
	if err := validateArchiveConfig(config); err != nil {
		return nil, err
	}
	/* START ALPHA CHANGE 3467 -- reject incompatible existing archives before extending them -- */
	if _, err := mdbxconfig.Resolve(config.MDBXPageSizeBytes); err != nil {
		return nil, err
	}
	if err := mdbxconfig.CheckTree(config.OutputRoot, config.MDBXPageSizeBytes); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3467 -- reject incompatible existing archives before extending them -- */

	/* START ALPHA CHANGE 2070 -- preserve the configured logger and default only when nil -- */
	logger := config.Logger
	if logger == nil {
		logger = hclog.NewNullLogger()
	}
	/* FINISH ALPHA CHANGE 2070 -- preserve the configured logger and default only when nil -- */

	/* START ALPHA CHANGE 2073 -- hold the archive writer lock through publication -- */
	if err := os.MkdirAll(config.OutputRoot, 0770); err != nil {
		return nil, fmt.Errorf("create archive output root: %w", err)
	}
	if err := requireArchiveDirectory(config.OutputRoot); err != nil {
		return nil, err
	}

	writerLock, err := acquireBuildLock(config.OutputRoot)
	if err != nil {
		return nil, err
	}
	defer writerLock.Close() //nolint:errcheck
	/* START ALPHA CHANGE 3467 -- verify archive pages while holding the publication lock -- */
	if err := mdbxconfig.CheckTree(config.OutputRoot, config.MDBXPageSizeBytes); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3467 -- verify archive pages while holding the publication lock -- */
	/* FINISH ALPHA CHANGE 2073 -- hold the archive writer lock through publication -- */

	chainConfig, err := chain.ImportFromFile(config.ChainPath)
	if err != nil {
		return nil, fmt.Errorf("load archive chain configuration: %w", err)
	}
	if chainConfig.Genesis == nil || chainConfig.Params == nil {
		return nil, errors.New("archive chain configuration is incomplete")
	}
	if chainConfig.Params.GetEngine() != innbcbft.ConsensusName {
		return nil, fmt.Errorf(
			"archive builder requires %q consensus",
			innbcbft.ConsensusName,
		)
	}
	if chainConfig.Params.ChainID <= 0 {
		return nil, errors.New("archive chain ID must be greater than zero")
	}

	innbcbft.SetupHeaderHashFunc()

	mapSize, err := mapSizeBytes(config.MDBXMapSizeGB)
	if err != nil {
		return nil, fmt.Errorf("invalid archive MDBX map size: %w", err)
	}

	source, err := mdbxstorage.NewMDBXStorageReadOnly(
		datadir.Blockchain(config.SourceDataDir),
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("open archive source: %w", err)
	}
	defer source.Close() //nolint:errcheck

	history, ok := source.(storage.HistoryAvailabilityStorage)
	if !ok {
		return nil, errors.New("archive source does not expose history availability")
	}
	sourceHistoryFrom := history.HistoryFrom()

	// Sample the canonical hash for the captured number, rather than combining
	// independently sampled head-number and head-hash fields.
	sourceHeadNumber, ok := source.ReadHeadNumber()
	if !ok {
		return nil, errors.New("archive source head number is missing")
	}
	sourceHead, err := readArchiveCanonicalHeader(source, sourceHeadNumber)
	if err != nil {
		return nil, err
	}
	sourceHeadHash := sourceHead.Hash

	genesis, err := readArchiveCanonicalHeader(source, 0)
	if err != nil {
		return nil, fmt.Errorf("read archive source genesis: %w", err)
	}

	// This follows the existing trusted-source bootstrap model. It checks
	// configured header fields against the stored genesis root; it does not
	// independently execute configured genesis allocations.
	expectedGenesis := chainConfig.Genesis.GenesisHeader()
	expectedGenesis.StateRoot = genesis.StateRoot
	expectedGenesis.ComputeHash()
	if expectedGenesis.Hash != genesis.Hash {
		return nil, errors.New("archive source genesis does not match configuration")
	}

	previous, previousPath, err := LoadPublishedArchive(config.OutputRoot)
	if err != nil && !errors.Is(err, ErrArchiveNotFound) {
		return nil, err
	}

	var (
		previousThrough   uint64
		totalTransactions uint64
		segments          []ArchiveSegment
	)
	effectiveIndexLimit := config.MaxIndexRecords
	boundaryHash := genesis.Hash

	if previous != nil {
		if previous.ChainID != chainConfig.Params.ChainID ||
			previous.GenesisHash != genesis.Hash ||
			previous.GenesisStateRoot != genesis.StateRoot {
			return nil, errors.New("existing archive identity does not match source")
		}
		if previous.SegmentBlocks != config.SegmentBlocks {
			return nil, errors.New("existing archive segment size differs from configuration")
		}

		// Routine continuation verifies committed metadata and payload sizes.
		// Full historical payload checksums belong to recovery verification.
		if err := ValidatePublishedArchiveReferences(
			config.OutputRoot,
			previous,
		); err != nil {
			return nil, fmt.Errorf("existing archive references: %w", err)
		}

		previousThrough = previous.ArchivedThrough
		totalTransactions = previous.TransactionCount
		segments = append(segments, previous.Segments...)
		boundaryHash = previous.Segments[len(previous.Segments)-1].LastBlockHash

		// An operational ceiling is not immutable segment geometry.
		// Preserve the ceiling needed to read already-published segments.
		if previous.MaxIndexRecords > effectiveIndexLimit {
			effectiveIndexLimit = previous.MaxIndexRecords
		}
	}

	if previousThrough == math.MaxUint64 {
		return nil, errors.New("archive watermark cannot be incremented")
	}
	if sourceHeadNumber < previousThrough {
		return nil, errors.New("archive source head is behind archive watermark")
	}

	nextRequired := previousThrough + 1
	if sourceHistoryFrom > nextRequired {
		return nil, fmt.Errorf(
			"source history starts at %d; next required archive block is %d",
			sourceHistoryFrom,
			nextRequired,
		)
	}

	if previous != nil && sourceHistoryFrom <= previousThrough {
		header, err := readArchiveCanonicalHeader(source, previousThrough)
		if err != nil {
			return nil, err
		}
		if header.Hash != boundaryHash {
			return nil, errors.New("source disagrees with the existing archive boundary")
		}
	} else if sourceHeadNumber >= nextRequired {
		header, err := readArchiveCanonicalHeader(source, nextRequired)
		if err != nil {
			return nil, err
		}
		if header.ParentHash != boundaryHash {
			return nil, errors.New("source continuation does not connect to archive boundary")
		}
	} else if previous != nil {
		// P is unavailable and N does not yet exist: continuity cannot be
		// established from either permitted boundary check.
		return nil, errors.New("source does not retain an archive continuation boundary")
	}

	completeThrough := sourceHeadNumber - sourceHeadNumber%config.SegmentBlocks
	latestPath := filepath.Join(config.OutputRoot, archiveLatestFileName)

	if completeThrough < nextRequired {
		if err := verifyFixedSourceHead(
			source,
			sourceHeadNumber,
			sourceHeadHash,
		); err != nil {
			return nil, err
		}

		return &ArchiveResult{
			PreviousArchivedThrough: previousThrough,
			ArchivedThrough:         previousThrough,
			ManifestPath:            previousPath,
			LatestPath:              latestPath,
		}, nil
	}

	segmentsRoot := filepath.Join(config.OutputRoot, archiveSegmentsDir)
	if err := os.MkdirAll(segmentsRoot, 0770); err != nil {
		return nil, fmt.Errorf("create archive segment root: %w", err)
	}
	if err := requireArchiveDirectory(config.OutputRoot); err != nil {
		return nil, err
	}
	if err := requireArchiveDirectory(segmentsRoot); err != nil {
		return nil, err
	}

	var segmentsAdded, transactionsAdded uint64

	for from := nextRequired; from <= completeThrough; {
		if config.SegmentBlocks-1 > math.MaxUint64-from {
			return nil, errors.New("archive segment range overflows uint64")
		}
		to := from + (config.SegmentBlocks - 1)

		segment, _, err := buildArchiveSegment(
			config,
			chainConfig.Params.ChainID,
			genesis.Hash,
			genesis.StateRoot,
			source,
			segmentsRoot,
			from,
			to,
			mapSize,
			logger,
		)
		if err != nil {
			return nil, err
		}
		if segment.ParentHash != boundaryHash {
			return nil, fmt.Errorf("archive segment beginning at %d breaks continuity", from)
		}
		if segment.TransactionCount > math.MaxUint64-totalTransactions {
			return nil, errors.New("archive transaction count overflows uint64")
		}
		if segment.TransactionCount > math.MaxUint64-transactionsAdded {
			return nil, errors.New("new archive transaction count overflows uint64")
		}

		segments = append(segments, *segment)
		boundaryHash = segment.LastBlockHash
		totalTransactions += segment.TransactionCount
		transactionsAdded += segment.TransactionCount

		// Count additions to the manifest, including an unreferenced segment
		// successfully recovered from an interrupted earlier invocation.
		segmentsAdded++

		if to == completeThrough {
			break
		}
		from = to + 1
	}

	// Also retries directory durability when an earlier invocation completed
	// a segment rename but failed before publishing its manifest.
	if err := syncDirectory(segmentsRoot); err != nil {
		return nil, fmt.Errorf("persist archive segment directory: %w", err)
	}
	if err := verifyFixedSourceHead(
		source,
		sourceHeadNumber,
		sourceHeadHash,
	); err != nil {
		return nil, err
	}

	manifest := &ArchiveManifest{
		FormatVersion:           ArchiveFormatVersion,
		StorageEncodingVersion:  ArchiveSegmentVersion,
		TransactionIndexVersion: TransactionIndexVersion,
		ChainID:                 chainConfig.Params.ChainID,
		GenesisHash:             genesis.Hash,
		GenesisStateRoot:        genesis.StateRoot,
		SegmentBlocks:           config.SegmentBlocks,
		MaxIndexRecords:         effectiveIndexLimit,
		ArchivedThrough:         completeThrough,
		SourceHeadNumber:        sourceHeadNumber,
		SourceHeadHash:          sourceHeadHash,
		TransactionCount:        totalTransactions,
		Segments:                segments,
	}
	if err := ValidateArchiveManifest(manifest); err != nil {
		return nil, fmt.Errorf("constructed archive manifest: %w", err)
	}

	manifestName, digest, err := publishImmutableArchiveManifest(
		config.OutputRoot,
		manifest,
	)
	if err != nil {
		return nil, err
	}

	latest := &ArchiveLatest{
		FormatVersion:   ArchiveFormatVersion,
		ArchivedThrough: completeThrough,
		ManifestPath:    manifestName,
		ManifestSHA256:  digest,
	}
	if err := writeJSONAtomic(latestPath, latest); err != nil {
		return nil, fmt.Errorf("publish archive latest pointer: %w", err)
	}
	if err := syncDirectory(config.OutputRoot); err != nil {
		return nil, fmt.Errorf("persist archive latest pointer: %w", err)
	}

	return &ArchiveResult{
		PreviousArchivedThrough: previousThrough,
		ArchivedThrough:         completeThrough,
		SegmentsAdded:           segmentsAdded,
		TransactionsAdded:       transactionsAdded,
		ManifestPath:            filepath.Join(config.OutputRoot, manifestName),
		LatestPath:              latestPath,
	}, nil
}

func validateArchiveConfig(config ArchiveConfig) error {
	if strings.TrimSpace(config.ChainPath) == "" {
		return errors.New("archive chain path is required")
	}
	if strings.TrimSpace(config.SourceDataDir) == "" {
		return errors.New("archive source data directory is required")
	}
	if strings.TrimSpace(config.OutputRoot) == "" {
		return errors.New("archive output root is required")
	}
	if config.SegmentBlocks == 0 {
		return errors.New("archive segment size must be greater than zero")
	}
	/* START ALPHA CHANGE 2071 -- enforce index limits for direct builder callers -- */
	if err := validateArchiveIndexLimit(config.MaxIndexRecords); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2071 -- enforce index limits for direct builder callers -- */
	if config.MDBXMapSizeGB == 0 {
		return errors.New("archive MDBX map size must be greater than zero")
	}

	return nil
}

func readArchiveCanonicalHeader(
	source storage.Storage,
	number uint64,
) (*types.Header, error) {
	hash, ok := source.ReadCanonicalHash(number)
	if !ok || hash == types.ZeroHash {
		return nil, fmt.Errorf("canonical archive source hash %d is missing", number)
	}

	header, err := source.ReadHeader(hash)
	if err != nil {
		return nil, fmt.Errorf("read canonical header %d: %w", number, err)
	}
	if header == nil || header.Number != number || header.Hash != hash {
		return nil, fmt.Errorf("canonical header identity mismatch at %d", number)
	}

	return header, nil
}

func verifyFixedSourceHead(
	source storage.Storage,
	number uint64,
	hash types.Hash,
) error {
	currentHead, ok := source.ReadHeadNumber()
	if !ok || currentHead < number {
		return errors.New("archive source head regressed during export")
	}

	header, err := readArchiveCanonicalHeader(source, number)
	if err != nil {
		return err
	}
	if header.Hash != hash {
		return errors.New("fixed archive source head changed during export")
	}

	return nil
}

func buildArchiveSegment(
	config ArchiveConfig,
	chainID int64,
	genesisHash types.Hash,
	genesisStateRoot types.Hash,
	source storage.Storage,
	segmentsRoot string,
	from uint64,
	to uint64,
	mapSize int,
	logger hclog.Logger,
) (*ArchiveSegment, bool, error) {
	first, err := readArchiveCanonicalHeader(source, from)
	if err != nil {
		return nil, false, err
	}
	last, err := readArchiveCanonicalHeader(source, to)
	if err != nil {
		return nil, false, err
	}

	name := archiveSegmentDirectoryName(from, to, last.Hash)
	finalDir := filepath.Join(segmentsRoot, name)
	expected := &ArchiveSegment{
		From:           from,
		To:             to,
		ParentHash:     first.ParentHash,
		FirstBlockHash: first.Hash,
		LastBlockHash:  last.Hash,
		Directory:      name,
	}

	if _, err := os.Lstat(finalDir); err == nil {
		segment, err := loadAndValidateUnreferencedArchiveSegment(
			finalDir,
			chainID,
			genesisHash,
			genesisStateRoot,
			expected,
			config.MaxIndexRecords,
			logger,
		)
		if err != nil {
			return nil, false, fmt.Errorf("existing archive segment %s: %w", name, err)
		}

		return segment, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}

	buildDir, err := os.MkdirTemp(segmentsRoot, ".building-")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(buildDir) //nolint:errcheck

	blockchainDir := filepath.Join(buildDir, archiveBlockchainDir)
	/* START ALPHA CHANGE 3467 -- apply archive destination page size -- */
	target, err := mdbxstorage.NewMDBXStorageWithMapSize(
		blockchainDir,
		logger,
		mapSize,
		config.MDBXPageSizeBytes,
	)
	/* FINISH ALPHA CHANGE 3467 -- apply archive destination page size -- */
	if err != nil {
		return nil, false, err
	}

	records, copyErr := copyArchiveRange(
		source,
		target,
		from,
		to,
		config.MaxIndexRecords,
	)
	closeErr := target.Close()
	if copyErr != nil {
		return nil, false, copyErr
	}
	if closeErr != nil {
		return nil, false, closeErr
	}

	indexPath := filepath.Join(buildDir, archiveIndexFileName)
	if err := writeTransactionIndex(
		indexPath,
		records,
		config.MaxIndexRecords,
	); err != nil {
		return nil, false, err
	}
	transactionCount := uint64(len(records))
	records = nil

	databaseIdentity, err := archiveFileIdentity(
		filepath.Join(blockchainDir, archiveMDBXDataFile),
	)
	if err != nil {
		return nil, false, err
	}
	databaseIdentity.Path = archiveBlockchainDir + "/" + archiveMDBXDataFile

	indexIdentity, err := archiveFileIdentity(indexPath)
	if err != nil {
		return nil, false, err
	}
	indexIdentity.Path = archiveIndexFileName

	segmentManifest := &ArchiveSegmentManifest{
		FormatVersion:           ArchiveFormatVersion,
		StorageEncodingVersion:  ArchiveSegmentVersion,
		TransactionIndexVersion: TransactionIndexVersion,
		ChainID:                 chainID,
		GenesisHash:             genesisHash,
		GenesisStateRoot:        genesisStateRoot,
		From:                    from,
		To:                      to,
		ParentHash:              first.ParentHash,
		FirstBlockHash:          first.Hash,
		LastBlockHash:           last.Hash,
		TransactionCount:        transactionCount,
		BlockchainData:          databaseIdentity,
		TransactionIndex:        indexIdentity,
	}
	if err := validateArchiveSegmentManifest(segmentManifest); err != nil {
		return nil, false, err
	}

	manifestPath := filepath.Join(buildDir, archiveSegmentFile)
	if err := writeArchiveJSONDurable(manifestPath, segmentManifest); err != nil {
		return nil, false, err
	}

	if _, err := validateArchiveSegmentDirectory(
		buildDir,
		config.MaxIndexRecords,
		logger,
		nil,
	); err != nil {
		return nil, false, fmt.Errorf("constructed archive segment: %w", err)
	}

	// Only this private staging directory is cleaned. Validation has closed
	// its MDBX handle before returning. Never remove a published DB's lock.
	lockPath := filepath.Join(blockchainDir, archiveMDBXLockFile)
	if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("remove staging MDBX runtime lock: %w", err)
	}

	manifestIdentity, err := archiveFileIdentity(manifestPath)
	if err != nil {
		return nil, false, err
	}
	manifestIdentity.Path = archiveSegmentFile

	expected.TransactionCount = transactionCount
	expected.SegmentManifest = manifestIdentity

	if err := publishArchiveSegment(segmentsRoot, buildDir, finalDir); err != nil {
		return nil, false, err
	}

	return expected, false, nil
}

func copyArchiveRange(
	source storage.Storage,
	target storage.Storage,
	from uint64,
	to uint64,
	maxRecords uint64,
) ([]TransactionIndexRecord, error) {
	/* START ALPHA CHANGE 2071 -- enforce the record ceiling before collecting transactions -- */
	if from == 0 || to < from {
		return nil, errors.New("invalid archive copy range")
	}
	if err := validateArchiveIndexLimit(maxRecords); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2071 -- enforce the record ceiling before collecting transactions -- */

	var records []TransactionIndexRecord

	for number := from; ; number++ {
		header, err := readArchiveCanonicalHeader(source, number)
		if err != nil {
			return nil, err
		}
		hash := header.Hash

		body, err := source.ReadBody(hash)
		if err != nil {
			return nil, fmt.Errorf("read source body %d: %w", number, err)
		}
		receipts, err := source.ReadReceipts(hash)
		if err != nil {
			return nil, fmt.Errorf("read source receipts %d: %w", number, err)
		}
		if body == nil || len(body.Transactions) != len(receipts) {
			return nil, fmt.Errorf("source body/receipt count mismatch at %d", number)
		}
		if uint64(len(records)) > maxRecords ||
			uint64(len(body.Transactions)) > maxRecords-uint64(len(records)) {
			/* START ALPHA CHANGE 2071 -- qualify index-limit guidance with the supported ceiling -- */
			return nil, fmt.Errorf(
				"archive segment exceeds %d index records; "+
					"--max-index-records may be increased only up to %d; "+
					"if already at that maximum, this segment exceeds supported capacity; "+
					"do not change segment size within an existing archive",
				maxRecords,
				MaximumArchiveIndexRecords,
			)
			/* FINISH ALPHA CHANGE 2071 -- qualify index-limit guidance with the supported ceiling -- */
		}

		currentHash, ok := source.ReadCanonicalHash(number)
		if !ok || currentHash != hash {
			return nil, fmt.Errorf("source canonical block %d changed during export", number)
		}

		batch := storage.NewBatchWriterForBlock(target, number)
		batch.PutCanonicalHeader(header)
		batch.PutBody(hash, body)
		batch.PutReceipts(hash, receipts)

		for position, transaction := range body.Transactions {
			if transaction == nil || receipts[position] == nil {
				return nil, fmt.Errorf("nil source transaction or receipt at block %d", number)
			}
			if uint64(position) > math.MaxUint32 {
				return nil, fmt.Errorf("transaction position exceeds uint32 at %d", number)
			}

			lookup, ok := source.ReadTxLookup(transaction.Hash)
			if !ok || lookup != hash {
				return nil, fmt.Errorf(
					"source transaction lookup mismatch at block %d position %d",
					number,
					position,
				)
			}

			batch.PutTxLookup(transaction.Hash, hash)
			records = append(records, TransactionIndexRecord{
				Hash:             transaction.Hash,
				BlockNumber:      number,
				TransactionIndex: uint32(position),
			})
		}

		if err := batch.WriteBatch(); err != nil {
			return nil, fmt.Errorf("write archive block %d: %w", number, err)
		}
		currentHash, ok = source.ReadCanonicalHash(number)
		if !ok || currentHash != hash {
			return nil, fmt.Errorf("source canonical block %d changed while copied", number)
		}

		if number == to {
			break
		}
	}

	return records, nil
}

/* FINISH ALPHA CHANGE 2067 -- build consistent append-only historical archives -- */
