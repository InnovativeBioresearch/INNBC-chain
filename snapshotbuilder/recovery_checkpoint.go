/* START ALPHA CHANGE 2075 -- build recovery checkpoints against verified archive boundaries -- */
package snapshotbuilder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"

	/* START ALPHA CHANGE 3468 -- validate recovery destination page policy -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3468 -- validate recovery destination page policy -- */
	"github.com/innovativebioresearch/innbc-chain/types"
)

type RecoveryCheckpointConfig struct {
	ChainPath          string
	SourceDataDir      string
	SnapshotOutputRoot string
	ArchiveOutputRoot  string

	MinimumRetentionBlocks uint64
	MDBXMapSizeGB          uint64
	/* START ALPHA CHANGE 3468 -- retain recovery checkpoint creation pages -- */
	MDBXPageSizeBytes uint64
	/* FINISH ALPHA CHANGE 3468 -- retain recovery checkpoint creation pages -- */

	Logger hclog.Logger
}

type RecoveryCheckpointResult struct {
	CheckpointNumber uint64
	CheckpointHash   types.Hash

	SharedBoundaryNumber uint64
	SharedBoundaryHash   types.Hash

	ActualRetentionBlocks uint64
	ArchiveActualThrough  uint64

	CheckpointDir   string
	RecoverySetPath string
	LatestPath      string
}

/* START ALPHA CHANGE 3501 -- allow deterministic recovery lock-ordering regressions -- */
func BuildRecoveryCheckpoint(
	config RecoveryCheckpointConfig,
) (*RecoveryCheckpointResult, error) {
	return buildRecoveryCheckpoint(config, acquireBuildLocks)
}

func buildRecoveryCheckpoint(
	config RecoveryCheckpointConfig,
	acquireLocks func(...string) (buildLock, error),
) (*RecoveryCheckpointResult, error) {
	if err := validateRecoveryCheckpointConfig(config); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3501 -- allow deterministic recovery lock-ordering regressions -- */
	/* START ALPHA CHANGE 3468 -- reject incompatible linked archive geometry -- */
	if _, err := mdbxconfig.Resolve(config.MDBXPageSizeBytes); err != nil {
		return nil, err
	}
	if err := mdbxconfig.CheckTree(config.ArchiveOutputRoot, config.MDBXPageSizeBytes); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3468 -- reject incompatible linked archive geometry -- */

	logger := config.Logger
	if logger == nil {
		logger = hclog.NewNullLogger()
	}

	chainConfig, err := chain.ImportFromFile(config.ChainPath)
	if err != nil {
		return nil, fmt.Errorf("load recovery chain configuration: %w", err)
	}
	if chainConfig.Genesis == nil || chainConfig.Params == nil {
		return nil, errors.New("recovery chain configuration is incomplete")
	}
	if chainConfig.Params.GetEngine() != innbcbft.ConsensusName ||
		chainConfig.Params.ChainID <= 0 {
		return nil, errors.New("recovery requires a valid INNBCBFT chain")
	}

	if err := requireArchiveDirectory(config.ArchiveOutputRoot); err != nil {
		return nil, fmt.Errorf("archive output root: %w", err)
	}
	checkpointRoot := filepath.Join(
		config.SnapshotOutputRoot,
		KindCheckpoint.directoryName(),
	)

	archiveRoot, err := canonicalBuildRoot(config.ArchiveOutputRoot)
	if err != nil {
		return nil, err
	}
	checkpointRoot, err = canonicalBuildRoot(checkpointRoot)
	if err != nil {
		return nil, err
	}

	archiveInfo, err := os.Stat(archiveRoot)
	if err != nil {
		return nil, err
	}
	checkpointInfo, err := os.Stat(checkpointRoot)
	if err != nil {
		return nil, err
	}
	if os.SameFile(archiveInfo, checkpointInfo) {
		return nil, errors.New("archive and checkpoint publication roots must differ")
	}

	/* START ALPHA CHANGE 3502 -- recheck linked archive pages under publication locks -- */
	writerLocks, err := acquireLocks(archiveRoot, checkpointRoot)
	if err != nil {
		return nil, err
	}
	defer writerLocks.Close() //nolint:errcheck

	// An archive writer may have completed after the initial compatibility
	// check. Hold both publication locks while checking the canonical root
	// again, before selecting or opening the linked archive.
	if err := mdbxconfig.CheckTree(archiveRoot, config.MDBXPageSizeBytes); err != nil {
		return nil, fmt.Errorf("linked archive page-size compatibility: %w", err)
	}
	/* FINISH ALPHA CHANGE 3502 -- recheck linked archive pages under publication locks -- */

	var (
		selectedArchive     *ArchiveManifest
		selectedArchiveFile ArchiveFile
		selectedBoundary    ArchiveSegment
		archivedBoundary    *types.Header
	)

	resolveRetainedFrom := func(
		checkpoint uint64,
		source storage.Storage,
		sourceHistoryFrom uint64,
	) (uint64, error) {
		checkpointHeader, err := readArchiveCanonicalHeader(source, checkpoint)
		if err != nil {
			return 0, err
		}

		finalDir := filepath.Join(
			checkpointRoot,
			snapshotDirectoryName(checkpoint, checkpointHeader.Hash),
		)
		var existingRecovery *RecoverySet

		if _, statErr := os.Lstat(finalDir); statErr == nil {
			if err := requireArchiveDirectory(finalDir); err != nil {
				return 0, err
			}
			existingRecovery, err = loadRecoverySet(
				filepath.Join(finalDir, recoverySetFileName),
			)
			if err != nil {
				return 0, fmt.Errorf(
					"existing checkpoint is not a valid requested recovery artifact: %w",
					err,
				)
			}
			if existingRecovery.Checkpoint.CheckpointNumber != checkpoint ||
				existingRecovery.Checkpoint.CheckpointHash != checkpointHeader.Hash {
				return 0, errors.New("existing recovery checkpoint identity differs")
			}

			// A retry remains pinned to the original immutable archive manifest,
			// even when archive latest.json has advanced.
			selectedArchive, selectedArchiveFile, err = loadRecoveryArchiveObject(
				archiveRoot,
				existingRecovery.Archive.Manifest,
			)
			if err != nil {
				return 0, err
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return 0, statErr
		} else {
			var manifestPath string
			selectedArchive, manifestPath, err = LoadPublishedArchive(archiveRoot)
			if err != nil {
				return 0, fmt.Errorf("load archive before recovery construction: %w", err)
			}
			selectedArchiveFile, err = archiveFileIdentity(manifestPath)
			if err != nil {
				return 0, err
			}
			selectedArchiveFile.Path = filepath.Base(manifestPath)
		}

		genesis, err := readArchiveCanonicalHeader(source, 0)
		if err != nil {
			return 0, err
		}
		if selectedArchive.ChainID != chainConfig.Params.ChainID ||
			selectedArchive.GenesisHash != genesis.Hash ||
			selectedArchive.GenesisStateRoot != genesis.StateRoot {
			return 0, errors.New("archive identity differs from recovery source")
		}

		selectedBoundary, err = selectSharedBoundary(
			checkpoint,
			config.MinimumRetentionBlocks,
			selectedArchive.Segments,
		)
		if err != nil {
			return 0, err
		}
		if existingRecovery != nil &&
			(existingRecovery.SharedBoundary.BlockNumber != selectedBoundary.To ||
				existingRecovery.SharedBoundary.BlockHash != selectedBoundary.LastBlockHash) {
			return 0, errors.New("existing recovery artifact uses a different boundary")
		}
		if sourceHistoryFrom > selectedBoundary.To {
			return 0, fmt.Errorf(
				"source history starts at %d, after recovery boundary %d",
				sourceHistoryFrom,
				selectedBoundary.To,
			)
		}

		archivedBoundary, err = verifyArchiveForRecovery(
			archiveRoot,
			selectedArchive,
			selectedBoundary.To,
			logger,
		)
		if err != nil {
			return 0, err
		}

		sourceBoundary, err := readArchiveCanonicalHeader(source, selectedBoundary.To)
		if err != nil {
			return 0, err
		}
		if sourceBoundary.Hash != archivedBoundary.Hash {
			return 0, errors.New("actual source and archived boundary headers differ")
		}

		return selectedBoundary.To, nil
	}

	beforePublish := func(buildDir string, manifest *Manifest) error {
		if selectedArchive == nil || archivedBoundary == nil {
			return errors.New("recovery archive boundary was not initialized")
		}

		boundary := RecoverySetBoundary{
			BlockNumber: archivedBoundary.Number,
			BlockHash:   archivedBoundary.Hash,
		}
		if err := verifyRecoveryCheckpointBoundary(
			buildDir,
			manifest,
			boundary,
			logger,
		); err != nil {
			return err
		}

		manifestPath := filepath.Join(buildDir, manifestFileName)
		if _, err := archiveRegularFileSize(
			manifestPath,
			recoveryCheckpointManifestLimit,
		); err != nil {
			return err
		}
		checkpointFile, err := archiveFileIdentity(manifestPath)
		if err != nil {
			return err
		}
		checkpointFile.Path = manifestFileName

		recovery := &RecoverySet{
			FormatVersion:  RecoverySetFormatVersion,
			CoverageMode:   recoverySetCoverageMode,
			ChainID:        selectedArchive.ChainID,
			GenesisHash:    selectedArchive.GenesisHash,
			SharedBoundary: boundary,
			Archive: RecoverySetArchive{
				Manifest: RecoverySetObject{
					Path:   selectedArchiveFile.Path,
					SHA256: selectedArchiveFile.SHA256,
					Bytes:  selectedArchiveFile.Bytes,
				},
				RequiredThrough: boundary.BlockNumber,
				ActualThrough:   selectedArchive.ArchivedThrough,
			},
			Checkpoint: RecoverySetCheckpoint{
				Manifest: RecoverySetObject{
					Path:   manifestFileName,
					SHA256: checkpointFile.SHA256,
					Bytes:  checkpointFile.Bytes,
				},
				CheckpointNumber: manifest.BlockNumber,
				CheckpointHash:   manifest.BlockHash,
				StateRoot:        manifest.StateRoot,
				RetainedFrom:     manifest.RetainedFrom,
				RetainedTo:       manifest.RetainedTo,
				RetentionBlocks:  manifest.RetentionBlocks,
			},
		}
		if err := ValidateRecoverySet(
			recovery,
			selectedArchive,
			manifest,
			selectedArchiveFile,
			checkpointFile,
		); err != nil {
			return err
		}

		raw, err := marshalArchiveJSON(recovery)
		if err != nil {
			return err
		}
		if len(raw) > recoverySetSizeLimit {
			return errors.New("constructed recovery document exceeds size limit")
		}
		if err := writeArchiveFileDurable(
			filepath.Join(buildDir, recoverySetFileName),
			raw,
			true,
		); err != nil {
			return err
		}

		return syncArchiveTree(buildDir)
	}

	/* START ALPHA CHANGE 2103 -- validate existing recovery metadata against its pinned archive and physical artifact -- */
	validateExisting := func(directory string, manifest *Manifest) error {
		if selectedArchive == nil || archivedBoundary == nil {
			return errors.New("recovery archive boundary was not initialized")
		}

		recovery, err := loadRecoverySet(filepath.Join(directory, recoverySetFileName))
		if err != nil {
			return err
		}

		manifestPath := filepath.Join(directory, manifestFileName)
		if _, err := archiveRegularFileSize(
			manifestPath,
			recoveryCheckpointManifestLimit,
		); err != nil {
			return err
		}

		identity, err := archiveFileIdentity(manifestPath)
		if err != nil {
			return err
		}
		identity.Path = manifestFileName

		if err := ValidateRecoverySet(
			recovery,
			selectedArchive,
			manifest,
			selectedArchiveFile,
			identity,
		); err != nil {
			return err
		}

		boundary := RecoverySetBoundary{
			BlockNumber: archivedBoundary.Number,
			BlockHash:   archivedBoundary.Hash,
		}
		if recovery.SharedBoundary != boundary {
			return errors.New("existing recovery artifact uses a different boundary")
		}

		return verifyRecoveryCheckpointBoundary(
			directory,
			manifest,
			boundary,
			logger,
		)
	}
	/* FINISH ALPHA CHANGE 2103 -- validate existing recovery metadata against its pinned archive and physical artifact -- */

	result, err := buildWithOptions(
		Config{
			Kind:            KindCheckpoint,
			ChainPath:       config.ChainPath,
			SourceDataDir:   config.SourceDataDir,
			OutputRoot:      config.SnapshotOutputRoot,
			RetentionBlocks: config.MinimumRetentionBlocks,
			MDBXMapSizeGB:   config.MDBXMapSizeGB,
			/* START ALPHA CHANGE 3468 -- forward recovery creation pages -- */
			MDBXPageSizeBytes: config.MDBXPageSizeBytes,
			/* FINISH ALPHA CHANGE 3468 -- forward recovery creation pages -- */
			Logger: logger,
		},
		buildOptions{
			locksAlreadyHeld:    true,
			resolveRetainedFrom: resolveRetainedFrom,
			beforePublish:       beforePublish,
			/* START ALPHA CHANGE 2103 -- require pinned relationship verification on retries -- */
			validateExisting: validateExisting,
			/* FINISH ALPHA CHANGE 2103 -- require pinned relationship verification on retries -- */
			preservePrevious: true,
			reuseExisting:    true,
		},
	)
	if err != nil {
		return nil, err
	}

	return &RecoveryCheckpointResult{
		CheckpointNumber:      result.BlockNumber,
		CheckpointHash:        result.BlockHash,
		SharedBoundaryNumber:  archivedBoundary.Number,
		SharedBoundaryHash:    archivedBoundary.Hash,
		ActualRetentionBlocks: result.RetainedTo - result.RetainedFrom + 1,
		ArchiveActualThrough:  selectedArchive.ArchivedThrough,
		CheckpointDir:         result.SnapshotDir,
		RecoverySetPath:       filepath.Join(result.SnapshotDir, recoverySetFileName),
		LatestPath:            result.LatestPath,
	}, nil
}

func validateRecoveryCheckpointConfig(config RecoveryCheckpointConfig) error {
	for name, value := range map[string]string{
		"chain path":            config.ChainPath,
		"source data directory": config.SourceDataDir,
		"snapshot output root":  config.SnapshotOutputRoot,
		"archive output root":   config.ArchiveOutputRoot,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if config.MinimumRetentionBlocks == 0 {
		return errors.New("minimum checkpoint retention must be greater than zero")
	}
	if config.MDBXMapSizeGB == 0 {
		return errors.New("recovery MDBX map size must be greater than zero")
	}
	if _, err := mapSizeBytes(config.MDBXMapSizeGB); err != nil {
		return err
	}

	return nil
}

func selectSharedBoundary(
	checkpoint uint64,
	minimumRetention uint64,
	segments []ArchiveSegment,
) (ArchiveSegment, error) {
	if minimumRetention == 0 || checkpoint < minimumRetention {
		return ArchiveSegment{}, errors.New("checkpoint is too early for requested recovery retention")
	}
	maximumBoundary := checkpoint - minimumRetention + 1

	for index := len(segments) - 1; index >= 0; index-- {
		if segments[index].To > 0 && segments[index].To <= maximumBoundary {
			return segments[index], nil
		}
	}

	return ArchiveSegment{}, errors.New(
		"archive has no completed segment boundary satisfying recovery retention",
	)
}

func loadRecoveryArchiveObject(
	root string,
	object RecoverySetObject,
) (*ArchiveManifest, ArchiveFile, error) {
	if err := requireArchiveDirectory(root); err != nil {
		return nil, ArchiveFile{}, err
	}
	if err := validateRecoverySetObject(
		object,
		archiveManifestName(object.SHA256),
		archiveManifestSizeLimit,
	); err != nil {
		return nil, ArchiveFile{}, err
	}

	path := filepath.Join(root, object.Path)
	size, err := archiveRegularFileSize(path, archiveManifestSizeLimit)
	if err != nil {
		return nil, ArchiveFile{}, err
	}
	if size != object.Bytes {
		return nil, ArchiveFile{}, errors.New("pinned archive manifest size differs")
	}

	expected := ArchiveFile{
		Path:   object.Path,
		SHA256: object.SHA256,
		Bytes:  object.Bytes,
	}
	if err := verifyArchiveFile(path, expected); err != nil {
		return nil, ArchiveFile{}, err
	}

	var manifest ArchiveManifest
	if err := readStrictJSON(path, archiveManifestSizeLimit, &manifest); err != nil {
		return nil, ArchiveFile{}, err
	}
	if err := ValidateArchiveManifest(&manifest); err != nil {
		return nil, ArchiveFile{}, err
	}

	return &manifest, expected, nil
}

// Verify committed payload bytes through the required boundary. Under the
// trusted-builder model, matching checksums preserve the validation performed
// when each segment was created. They do not authenticate an external source.
func verifyArchiveForRecovery(
	root string,
	manifest *ArchiveManifest,
	requiredThrough uint64,
	logger hclog.Logger,
) (*types.Header, error) {
	if err := ValidateArchiveManifest(manifest); err != nil {
		return nil, err
	}
	if requiredThrough == 0 || requiredThrough > manifest.ArchivedThrough {
		return nil, errors.New("required recovery archive boundary is invalid")
	}
	if err := requireArchiveDirectory(root); err != nil {
		return nil, err
	}

	segmentsRoot := filepath.Join(root, archiveSegmentsDir)
	if err := requireArchiveDirectory(segmentsRoot); err != nil {
		return nil, err
	}

	for index := range manifest.Segments {
		expected := &manifest.Segments[index]
		if expected.To > requiredThrough {
			break
		}

		directory := filepath.Join(segmentsRoot, expected.Directory)
		if err := verifyRecoveryArchiveSegment(directory, manifest, expected); err != nil {
			return nil, fmt.Errorf("recovery archive segment %s: %w", expected.Directory, err)
		}
		if expected.To != requiredThrough {
			continue
		}

		database, err := mdbxstorage.NewMDBXStorageReadOnly(
			filepath.Join(directory, archiveBlockchainDir),
			logger,
		)
		if err != nil {
			return nil, err
		}
		header, readErr := readArchiveCanonicalHeader(database, requiredThrough)
		closeErr := database.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return nil, err
		}
		if header.Hash != expected.LastBlockHash {
			return nil, errors.New("actual archived boundary header differs from manifest")
		}

		// Leave MDBX's runtime lock file in place after closing the reader.
		return header, nil
	}

	return nil, errors.New("recovery boundary is not a completed archive segment")
}

func verifyRecoveryArchiveSegment(
	directory string,
	archive *ArchiveManifest,
	expected *ArchiveSegment,
) error {
	actual, err := readArchiveSegmentMetadata(directory, expected)
	if err != nil {
		return err
	}
	if err := compareArchiveSegmentIdentity(
		actual,
		archive.ChainID,
		archive.GenesisHash,
		archive.GenesisStateRoot,
		expected,
		true,
	); err != nil {
		return err
	}
	if actual.TransactionCount > archive.MaxIndexRecords {
		return errors.New("recovery segment exceeds archive index limit")
	}
	if err := validateArchivePayloadSizes(directory, actual); err != nil {
		return err
	}

	for _, file := range []ArchiveFile{
		actual.BlockchainData,
		actual.TransactionIndex,
	} {
		if err := verifyArchiveFile(
			filepath.Join(directory, filepath.FromSlash(file.Path)),
			file,
		); err != nil {
			return err
		}
	}

	records, err := readTransactionIndex(
		filepath.Join(directory, archiveIndexFileName),
		archive.MaxIndexRecords,
	)
	if err != nil {
		return err
	}
	if uint64(len(records)) != actual.TransactionCount {
		return errors.New("recovery index count differs from segment metadata")
	}
	for index := range records {
		if records[index].BlockNumber < actual.From ||
			records[index].BlockNumber > actual.To {
			return fmt.Errorf("recovery index record %d is outside segment range", index)
		}
	}

	return nil
}

func verifyRecoveryCheckpointBoundary(
	directory string,
	manifest *Manifest,
	boundary RecoverySetBoundary,
	logger hclog.Logger,
) error {
	if manifest == nil || manifest.SnapshotKind != KindCheckpoint ||
		boundary.BlockNumber == 0 ||
		boundary.BlockNumber != manifest.RetainedFrom ||
		boundary.BlockNumber > manifest.BlockNumber ||
		boundary.BlockHash == types.ZeroHash {
		return errors.New("invalid checkpoint boundary verification request")
	}

	database, err := mdbxstorage.NewMDBXStorageReadOnly(
		datadir.Blockchain(directory),
		logger,
	)
	if err != nil {
		return err
	}

	verifyErr := func() error {
		header, err := readArchiveCanonicalHeader(database, boundary.BlockNumber)
		if err != nil {
			return err
		}
		if header.Hash != boundary.BlockHash {
			return errors.New("actual checkpoint and archive boundary headers differ")
		}

		if boundary.BlockNumber < manifest.BlockNumber {
			next, err := readArchiveCanonicalHeader(database, boundary.BlockNumber+1)
			if err != nil {
				return err
			}
			if next.ParentHash != header.Hash {
				return errors.New("checkpoint block after shared boundary has wrong parent")
			}
		}

		return nil
	}()

	return errors.Join(verifyErr, database.Close())
}

/* FINISH ALPHA CHANGE 2075 -- build recovery checkpoints against verified archive boundaries -- */
