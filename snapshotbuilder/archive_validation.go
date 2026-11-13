/* START ALPHA CHANGE 2068 -- validate archive metadata payloads and transaction coverage -- */
package snapshotbuilder

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/innovativebioresearch/innbc-chain/types/buildroot"
)

var ErrArchiveNotFound = errors.New("published archive does not exist")

func ValidateArchiveManifest(manifest *ArchiveManifest) error {
	if manifest == nil {
		return errors.New("archive manifest is nil")
	}
	if manifest.FormatVersion != ArchiveFormatVersion ||
		manifest.StorageEncodingVersion != ArchiveSegmentVersion ||
		manifest.TransactionIndexVersion != TransactionIndexVersion {
		return errors.New("unsupported archive manifest version")
	}
	if manifest.ChainID <= 0 ||
		manifest.GenesisHash == types.ZeroHash ||
		manifest.GenesisStateRoot == types.ZeroHash {
		return errors.New("archive manifest has invalid chain identity")
	}
	/* START ALPHA CHANGE 2071 -- validate published archive index limits consistently -- */
	if manifest.SegmentBlocks == 0 {
		return errors.New("archive manifest has invalid segment size")
	}
	if err := validateArchiveIndexLimit(manifest.MaxIndexRecords); err != nil {
		return fmt.Errorf("archive manifest: %w", err)
	}
	/* FINISH ALPHA CHANGE 2071 -- validate published archive index limits consistently -- */
	if len(manifest.Segments) == 0 {
		return errors.New("archive manifest has no segments")
	}
	if manifest.SourceHeadHash == types.ZeroHash ||
		manifest.SourceHeadNumber < manifest.ArchivedThrough {
		return errors.New("archive manifest has invalid source head")
	}

	expectedFrom := uint64(1)
	expectedParent := manifest.GenesisHash
	var transactionCount uint64
	directories := make(map[string]struct{}, len(manifest.Segments))

	for index := range manifest.Segments {
		segment := &manifest.Segments[index]

		if segment.From != expectedFrom ||
			segment.From == 0 ||
			segment.To < segment.From ||
			segment.To-segment.From != manifest.SegmentBlocks-1 {
			return fmt.Errorf("archive segment %d has invalid range or geometry", index)
		}
		if segment.ParentHash != expectedParent ||
			segment.FirstBlockHash == types.ZeroHash ||
			segment.LastBlockHash == types.ZeroHash {
			return fmt.Errorf("archive segment %d has invalid block continuity", index)
		}
		if segment.From == segment.To &&
			segment.FirstBlockHash != segment.LastBlockHash {
			return fmt.Errorf("single-block archive segment %d has inconsistent hashes", index)
		}

		expectedName := archiveSegmentDirectoryName(
			segment.From,
			segment.To,
			segment.LastBlockHash,
		)
		if segment.Directory != expectedName {
			return fmt.Errorf("archive segment %d has an invalid directory name", index)
		}
		if _, exists := directories[segment.Directory]; exists {
			return fmt.Errorf("duplicate archive segment directory %q", segment.Directory)
		}
		directories[segment.Directory] = struct{}{}

		if err := validateArchiveSegmentManifestReference(segment.SegmentManifest); err != nil {
			return fmt.Errorf("archive segment %d manifest reference: %w", index, err)
		}
		if segment.TransactionCount > manifest.MaxIndexRecords {
			return fmt.Errorf("archive segment %d exceeds the declared index limit", index)
		}
		if segment.TransactionCount > math.MaxUint64-transactionCount {
			return errors.New("archive transaction count overflows uint64")
		}
		transactionCount += segment.TransactionCount
		expectedParent = segment.LastBlockHash

		if index != len(manifest.Segments)-1 {
			if segment.To == math.MaxUint64 {
				return errors.New("archive segment range cannot advance")
			}
			expectedFrom = segment.To + 1
		}
	}

	last := manifest.Segments[len(manifest.Segments)-1]
	if manifest.ArchivedThrough != last.To {
		return errors.New("archive watermark differs from final segment end")
	}
	if manifest.TransactionCount != transactionCount {
		return errors.New("archive aggregate transaction count is incorrect")
	}

	return nil
}

func validateArchiveSegmentManifestReference(reference ArchiveFile) error {
	if reference.Path != archiveSegmentFile {
		return errors.New("segment manifest path must be segment.json")
	}
	if reference.Bytes == 0 || reference.Bytes > archiveSegmentSizeLimit {
		return errors.New("segment manifest size is invalid")
	}

	return validateSHA256(reference.SHA256)
}

// Validate metadata before using any embedded payload path.
func validateArchiveSegmentManifest(manifest *ArchiveSegmentManifest) error {
	if manifest == nil {
		return errors.New("archive segment manifest is nil")
	}
	if manifest.FormatVersion != ArchiveFormatVersion ||
		manifest.StorageEncodingVersion != ArchiveSegmentVersion ||
		manifest.TransactionIndexVersion != TransactionIndexVersion {
		return errors.New("unsupported archive segment version")
	}
	if manifest.ChainID <= 0 ||
		manifest.GenesisHash == types.ZeroHash ||
		manifest.GenesisStateRoot == types.ZeroHash {
		return errors.New("archive segment has invalid chain identity")
	}
	if manifest.From == 0 || manifest.To < manifest.From {
		return errors.New("archive segment range is invalid")
	}
	if manifest.ParentHash == types.ZeroHash ||
		manifest.FirstBlockHash == types.ZeroHash ||
		manifest.LastBlockHash == types.ZeroHash {
		return errors.New("archive segment contains a zero block hash")
	}
	if manifest.From == 1 && manifest.ParentHash != manifest.GenesisHash {
		return errors.New("first archive segment does not connect to genesis")
	}
	if manifest.From == manifest.To &&
		manifest.FirstBlockHash != manifest.LastBlockHash {
		return errors.New("single-block archive segment hashes differ")
	}

	if manifest.BlockchainData.Path != archiveBlockchainDir+"/"+archiveMDBXDataFile {
		return errors.New("archive database path is invalid")
	}
	if manifest.TransactionIndex.Path != archiveIndexFileName {
		return errors.New("archive transaction index path is invalid")
	}
	if err := validateSHA256(manifest.BlockchainData.SHA256); err != nil {
		return fmt.Errorf("archive database checksum: %w", err)
	}
	if err := validateSHA256(manifest.TransactionIndex.SHA256); err != nil {
		return fmt.Errorf("archive index checksum: %w", err)
	}
	if manifest.BlockchainData.Bytes == 0 ||
		manifest.BlockchainData.Bytes > math.MaxInt64 {
		return errors.New("archive database size is invalid")
	}

	if manifest.TransactionCount >
		(math.MaxInt64-transactionIndexHeaderSize)/transactionIndexRecordSize {
		return errors.New("archive index size overflows supported file size")
	}
	expectedIndexBytes := uint64(transactionIndexHeaderSize) +
		manifest.TransactionCount*uint64(transactionIndexRecordSize)
	if manifest.TransactionIndex.Bytes != expectedIndexBytes {
		return errors.New("archive index size does not match its transaction count")
	}

	return nil
}

func LoadPublishedArchive(root string) (*ArchiveManifest, string, error) {
	latestPath := filepath.Join(root, archiveLatestFileName)
	if _, err := os.Lstat(latestPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", ErrArchiveNotFound
		}
		return nil, "", err
	}
	if err := requireArchiveDirectory(root); err != nil {
		return nil, "", err
	}
	if _, err := archiveRegularFileSize(latestPath, archiveLatestSizeLimit); err != nil {
		return nil, "", err
	}

	var latest ArchiveLatest
	if err := readStrictJSON(latestPath, archiveLatestSizeLimit, &latest); err != nil {
		return nil, "", fmt.Errorf("read archive latest pointer: %w", err)
	}
	if latest.FormatVersion != ArchiveFormatVersion {
		return nil, "", errors.New("unsupported archive latest version")
	}
	if err := validateSHA256(latest.ManifestSHA256); err != nil {
		return nil, "", err
	}
	if latest.ManifestPath != archiveManifestName(latest.ManifestSHA256) {
		return nil, "", errors.New("archive manifest name does not match its checksum")
	}

	manifestPath := filepath.Join(root, latest.ManifestPath)
	if _, err := archiveRegularFileSize(
		manifestPath,
		archiveManifestSizeLimit,
	); err != nil {
		return nil, "", err
	}
	identity, err := archiveFileIdentity(manifestPath)
	if err != nil {
		return nil, "", err
	}
	if identity.SHA256 != latest.ManifestSHA256 {
		return nil, "", errors.New("archive manifest checksum differs from latest pointer")
	}

	var manifest ArchiveManifest
	if err := readStrictJSON(
		manifestPath,
		archiveManifestSizeLimit,
		&manifest,
	); err != nil {
		return nil, "", err
	}
	if err := ValidateArchiveManifest(&manifest); err != nil {
		return nil, "", err
	}
	if latest.ArchivedThrough != manifest.ArchivedThrough {
		return nil, "", errors.New("archive latest and manifest watermarks differ")
	}

	return &manifest, manifestPath, nil
}

// ValidatePublishedArchiveReferences is the inexpensive continuation check.
//
// It verifies every committed segment.json checksum, its metadata and identity,
// and the existence, regular-file type, and exact sizes of both payload files.
// It deliberately does not hash historical MDBX databases or index payloads.
// Recovery verification must additionally verify full payload checksums.
func ValidatePublishedArchiveReferences(
	root string,
	manifest *ArchiveManifest,
) error {
	if err := ValidateArchiveManifest(manifest); err != nil {
		return err
	}
	if err := requireArchiveDirectory(root); err != nil {
		return err
	}

	segmentsRoot := filepath.Join(root, archiveSegmentsDir)
	if err := requireArchiveDirectory(segmentsRoot); err != nil {
		return err
	}

	for index := range manifest.Segments {
		expected := &manifest.Segments[index]
		directory := filepath.Join(segmentsRoot, expected.Directory)

		actual, err := readArchiveSegmentMetadata(directory, expected)
		if err != nil {
			return fmt.Errorf("archive segment %s: %w", expected.Directory, err)
		}
		if err := compareArchiveSegmentIdentity(
			actual,
			manifest.ChainID,
			manifest.GenesisHash,
			manifest.GenesisStateRoot,
			expected,
			true,
		); err != nil {
			return fmt.Errorf("archive segment %s: %w", expected.Directory, err)
		}
		if actual.TransactionCount > manifest.MaxIndexRecords {
			return errors.New("archive segment exceeds manifest index limit")
		}
		if err := validateArchivePayloadSizes(directory, actual); err != nil {
			return fmt.Errorf("archive segment %s: %w", expected.Directory, err)
		}
	}

	return nil
}

func readArchiveSegmentMetadata(
	directory string,
	expected *ArchiveSegment,
) (*ArchiveSegmentManifest, error) {
	if err := validateArchiveSegmentFilesystem(directory); err != nil {
		return nil, err
	}

	manifestPath := filepath.Join(directory, archiveSegmentFile)
	if _, err := archiveRegularFileSize(
		manifestPath,
		archiveSegmentSizeLimit,
	); err != nil {
		return nil, err
	}

	if expected != nil {
		if err := validateArchiveSegmentManifestReference(expected.SegmentManifest); err != nil {
			return nil, err
		}
		if err := verifyArchiveFile(manifestPath, expected.SegmentManifest); err != nil {
			return nil, fmt.Errorf("segment manifest commitment mismatch: %w", err)
		}
	}

	var manifest ArchiveSegmentManifest
	if err := readStrictJSON(
		manifestPath,
		archiveSegmentSizeLimit,
		&manifest,
	); err != nil {
		return nil, err
	}
	if err := validateArchiveSegmentManifest(&manifest); err != nil {
		return nil, err
	}

	if expected != nil {
		// Zero is a real committed transaction count, never an unspecified
		// sentinel. This comparison is unconditional for referenced segments.
		if err := compareArchiveSegmentIdentity(
			&manifest,
			manifest.ChainID,
			manifest.GenesisHash,
			manifest.GenesisStateRoot,
			expected,
			true,
		); err != nil {
			return nil, err
		}
	}

	return &manifest, nil
}

func validateArchivePayloadSizes(
	directory string,
	manifest *ArchiveSegmentManifest,
) error {
	for _, file := range []ArchiveFile{
		manifest.BlockchainData,
		manifest.TransactionIndex,
	} {
		path := filepath.Join(directory, filepath.FromSlash(file.Path))
		size, err := archiveRegularFileSize(path, math.MaxInt64)
		if err != nil {
			return err
		}
		if size != file.Bytes {
			return fmt.Errorf("archive payload size mismatch for %s", file.Path)
		}
	}

	return nil
}

// validateArchiveSegmentDirectory performs full payload and database validation.
// It may create MDBX's runtime lock file. It never removes a published lock.
func validateArchiveSegmentDirectory(
	directory string,
	maxRecords uint64,
	logger hclog.Logger,
	expected *ArchiveSegment,
) (*ArchiveSegmentManifest, error) {
	/* START ALPHA CHANGE 2071 -- enforce supported limits before full segment validation -- */
	if err := validateArchiveIndexLimit(maxRecords); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2071 -- enforce supported limits before full segment validation -- */

	manifest, err := readArchiveSegmentMetadata(directory, expected)
	if err != nil {
		return nil, err
	}
	if manifest.TransactionCount > maxRecords {
		return nil, errors.New("archive segment exceeds configured index limit")
	}
	if err := validateArchivePayloadSizes(directory, manifest); err != nil {
		return nil, err
	}

	for _, file := range []ArchiveFile{
		manifest.BlockchainData,
		manifest.TransactionIndex,
	} {
		path := filepath.Join(directory, filepath.FromSlash(file.Path))
		if err := verifyArchiveFile(path, file); err != nil {
			return nil, fmt.Errorf("archive payload %s: %w", file.Path, err)
		}
	}

	records, err := readTransactionIndex(
		filepath.Join(directory, archiveIndexFileName),
		maxRecords,
	)
	if err != nil {
		return nil, err
	}
	if uint64(len(records)) != manifest.TransactionCount {
		return nil, errors.New("archive decoded index count differs from metadata")
	}

	database, err := mdbxstorage.NewMDBXStorageReadOnly(
		filepath.Join(directory, archiveBlockchainDir),
		logger,
	)
	if err != nil {
		return nil, err
	}

	validationErr := validateArchiveBlockchain(database, manifest, records)
	closeErr := database.Close()
	if validationErr != nil {
		return nil, validationErr
	}
	if closeErr != nil {
		return nil, closeErr
	}

	return manifest, nil
}

func loadAndValidateUnreferencedArchiveSegment(
	directory string,
	chainID int64,
	genesisHash types.Hash,
	genesisStateRoot types.Hash,
	expected *ArchiveSegment,
	maxRecords uint64,
	logger hclog.Logger,
) (*ArchiveSegment, error) {
	if expected == nil {
		return nil, errors.New("expected unreferenced segment identity is missing")
	}

	// No earlier manifest commits this segment's count or manifest digest.
	// Establish them through full validation, not a zero-count sentinel.
	manifest, err := validateArchiveSegmentDirectory(
		directory,
		maxRecords,
		logger,
		nil,
	)
	if err != nil {
		return nil, err
	}
	if err := compareArchiveSegmentIdentity(
		manifest,
		chainID,
		genesisHash,
		genesisStateRoot,
		expected,
		false,
	); err != nil {
		return nil, err
	}

	identity, err := archiveFileIdentity(filepath.Join(directory, archiveSegmentFile))
	if err != nil {
		return nil, err
	}
	identity.Path = archiveSegmentFile

	result := *expected
	result.TransactionCount = manifest.TransactionCount
	result.SegmentManifest = identity

	return &result, nil
}

func compareArchiveSegmentIdentity(
	actual *ArchiveSegmentManifest,
	chainID int64,
	genesisHash types.Hash,
	genesisStateRoot types.Hash,
	expected *ArchiveSegment,
	compareCount bool,
) error {
	if actual == nil || expected == nil {
		return errors.New("archive segment identity is missing")
	}
	if actual.ChainID != chainID ||
		actual.GenesisHash != genesisHash ||
		actual.GenesisStateRoot != genesisStateRoot {
		return errors.New("archive segment chain identity differs")
	}
	if actual.From != expected.From ||
		actual.To != expected.To ||
		actual.ParentHash != expected.ParentHash ||
		actual.FirstBlockHash != expected.FirstBlockHash ||
		actual.LastBlockHash != expected.LastBlockHash {
		return errors.New("archive segment block identity differs")
	}
	if compareCount && actual.TransactionCount != expected.TransactionCount {
		return errors.New("archive segment transaction count differs")
	}

	return nil
}

func requireArchiveDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("archive path is not a real directory: %s", path)
	}

	return nil
}

func archiveRegularFileSize(path string, limit uint64) (uint64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 {
		return 0, fmt.Errorf("archive path is not a regular file: %s", path)
	}

	size := uint64(info.Size())
	if size > limit {
		return 0, fmt.Errorf("archive file exceeds size limit: %s", path)
	}

	return size, nil
}

func validateArchiveSegmentFilesystem(directory string) error {
	if err := requireArchiveDirectory(directory); err != nil {
		return err
	}

	// Values indicate whether the required entry is a directory.
	required := map[string]bool{
		".":                  true,
		archiveBlockchainDir: true,
		archiveIndexFileName: false,
		archiveSegmentFile:   false,
		filepath.Join(archiveBlockchainDir, archiveMDBXDataFile): false,
	}
	seen := make(map[string]bool, len(required))
	lockName := filepath.Join(archiveBlockchainDir, archiveMDBXLockFile)

	err := filepath.WalkDir(
		directory,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(directory, path)
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("archive segment contains symlink: %s", relative)
			}

			if relative == lockName {
				if !info.Mode().IsRegular() {
					return errors.New("archive MDBX runtime lock is not a regular file")
				}
				return nil
			}

			directoryExpected, allowed := required[relative]
			if !allowed {
				return fmt.Errorf("unexpected archive segment entry: %s", relative)
			}
			if directoryExpected {
				if !info.IsDir() {
					return fmt.Errorf("archive entry must be a directory: %s", relative)
				}
			} else if !info.Mode().IsRegular() {
				return fmt.Errorf("archive entry must be a regular file: %s", relative)
			}
			seen[relative] = true

			return nil
		},
	)
	if err != nil {
		return err
	}

	for path := range required {
		if !seen[path] {
			return fmt.Errorf("required archive segment entry is missing: %s", path)
		}
	}

	return nil
}

func validateArchiveBlockchain(
	database storage.Storage,
	manifest *ArchiveSegmentManifest,
	records []TransactionIndexRecord,
) error {
	if err := validateArchiveSegmentManifest(manifest); err != nil {
		return err
	}
	if uint64(len(records)) != manifest.TransactionCount {
		return errors.New("archive index record count differs from manifest")
	}
	for index := range records {
		record := &records[index]
		if record.BlockNumber < manifest.From || record.BlockNumber > manifest.To {
			return fmt.Errorf("archive index record %d is outside segment range", index)
		}
		if index > 0 &&
			bytes.Compare(records[index-1].Hash[:], record.Hash[:]) >= 0 {
			return errors.New("archive index hashes are not strictly increasing")
		}
	}

	usedRecords := make([]bool, len(records))
	expectedParent := manifest.ParentHash
	var transactionCount uint64

	for number := manifest.From; ; number++ {
		header, err := readArchiveCanonicalHeader(database, number)
		if err != nil {
			return err
		}
		hash := header.Hash
		if header.ParentHash != expectedParent {
			return fmt.Errorf("archive parent mismatch at block %d", number)
		}
		if number == manifest.From && hash != manifest.FirstBlockHash {
			return errors.New("archive first block hash differs from manifest")
		}

		body, err := database.ReadBody(hash)
		if err != nil {
			return fmt.Errorf("read archive body %d: %w", number, err)
		}
		receipts, err := database.ReadReceipts(hash)
		if err != nil {
			return fmt.Errorf("read archive receipts %d: %w", number, err)
		}
		if body == nil || len(body.Transactions) != len(receipts) {
			return fmt.Errorf("archive body/receipt count mismatch at %d", number)
		}

		for position, transaction := range body.Transactions {
			if transaction == nil || receipts[position] == nil {
				return fmt.Errorf("nil archive transaction or receipt at block %d", number)
			}
			for _, log := range receipts[position].Logs {
				if log == nil {
					return fmt.Errorf("nil archive receipt log at block %d", number)
				}
			}
		}
		for _, uncle := range body.Uncles {
			if uncle == nil {
				return fmt.Errorf("nil archive uncle at block %d", number)
			}
		}

		if buildroot.CalculateTransactionsRoot(body.Transactions, number) != header.TxRoot {
			return fmt.Errorf("archive transaction root mismatch at block %d", number)
		}
		if buildroot.CalculateReceiptsRoot(receipts) != header.ReceiptsRoot {
			return fmt.Errorf("archive receipt root mismatch at block %d", number)
		}
		if buildroot.CalculateUncleRoot(body.Uncles) != header.Sha3Uncles {
			return fmt.Errorf("archive uncle root mismatch at block %d", number)
		}

		for position, transaction := range body.Transactions {
			if uint64(position) > math.MaxUint32 {
				return fmt.Errorf("archive transaction position exceeds uint32 at %d", number)
			}

			receipt := receipts[position]
			if receipt.TxHash != transaction.Hash ||
				receipt.TransactionType != transaction.Type {
				return fmt.Errorf(
					"archive receipt identity mismatch at block %d position %d",
					number,
					position,
				)
			}
			blockHash, ok := database.ReadTxLookup(transaction.Hash)
			if !ok || blockHash != hash {
				return fmt.Errorf("archive transaction lookup mismatch at block %d", number)
			}

			recordIndex := sort.Search(len(records), func(index int) bool {
				return bytes.Compare(records[index].Hash[:], transaction.Hash[:]) >= 0
			})
			if recordIndex == len(records) ||
				records[recordIndex].Hash != transaction.Hash {
				return fmt.Errorf("archive index is missing a transaction from block %d", number)
			}

			record := records[recordIndex]
			if record.BlockNumber != number ||
				record.TransactionIndex != uint32(position) {
				return fmt.Errorf(
					"archive index position mismatch at block %d position %d",
					number,
					position,
				)
			}
			if usedRecords[recordIndex] {
				return errors.New("archive index record resolves more than once")
			}

			usedRecords[recordIndex] = true
			transactionCount++
		}

		expectedParent = hash
		if number == manifest.To {
			break
		}
	}

	if expectedParent != manifest.LastBlockHash {
		return errors.New("archive final block hash differs from manifest")
	}
	if transactionCount != manifest.TransactionCount {
		return errors.New("archive transaction coverage differs from manifest")
	}
	for index, used := range usedRecords {
		if !used {
			return fmt.Errorf("archive index contains unresolved record %d", index)
		}
	}

	headNumber, ok := database.ReadHeadNumber()
	if !ok || headNumber != manifest.To {
		return errors.New("archive segment head number is incorrect")
	}
	headHash, ok := database.ReadHeadHash()
	if !ok || headHash != manifest.LastBlockHash {
		return errors.New("archive segment head hash is incorrect")
	}

	return nil
}

/* FINISH ALPHA CHANGE 2068 -- validate archive metadata payloads and transaction coverage -- */
