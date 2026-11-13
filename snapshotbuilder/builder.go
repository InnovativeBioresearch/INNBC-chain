/* START ALPHA CHANGE 1690 -- build independent segmented and checkpoint snapshots -- */
package snapshotbuilder

/* START ALPHA CHANGE 2076 -- support exact recovery metadata comparison -- */
import (
	"encoding/json"
	"errors"

	/* FINISH ALPHA CHANGE 2076 -- support exact recovery metadata comparison -- */
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	segmentedstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/segmented"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"

	/* START ALPHA CHANGE 3464 -- configure snapshot destination geometry -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3464 -- configure snapshot destination geometry -- */
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/innovativebioresearch/innbc-chain/types/buildroot"
)

type Kind string

/* START ALPHA CHANGE 2102 -- require executable-state and consensus commitments in fresh snapshots -- */
const (
	KindSegmented  Kind = "segmented"
	KindCheckpoint Kind = "checkpoint"

	FormatVersion uint64 = 2

	manifestFileName = "manifest.json"
	latestFileName   = "latest.json"

	snapshotConsensusDataPath = "chain-data/consensus/consensusState/mdbx.dat"
)

type SnapshotFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
}

/* FINISH ALPHA CHANGE 2102 -- require executable-state and consensus commitments in fresh snapshots -- */

type Config struct {
	Kind          Kind
	ChainPath     string
	SourceDataDir string
	OutputRoot    string

	RetentionBlocks uint64
	SegmentBlocks   uint64

	/* START ALPHA CHANGE 1776 -- use one destination MDBX map size -- */
	MDBXMapSizeGB uint64
	/* START ALPHA CHANGE 3464 -- select destination pages independently of source pages -- */
	MDBXPageSizeBytes uint64
	/* FINISH ALPHA CHANGE 3464 -- select destination pages independently of source pages -- */
	/* FINISH ALPHA CHANGE 1776 -- use one destination MDBX map size -- */

	Logger hclog.Logger
}

type Manifest struct {
	FormatVersion uint64 `json:"formatVersion"`
	SnapshotKind  Kind   `json:"snapshotKind"`

	/* START ALPHA CHANGE 1810 -- retain allocation-derived genesis identity -- */
	BlockNumber      uint64     `json:"blockNumber"`
	BlockHash        types.Hash `json:"blockHash"`
	StateRoot        types.Hash `json:"stateRoot"`
	GenesisHash      types.Hash `json:"genesisHash"`
	GenesisStateRoot types.Hash `json:"genesisStateRoot"`
	/* FINISH ALPHA CHANGE 1810 -- retain allocation-derived genesis identity -- */

	RetainedFrom uint64 `json:"retainedFrom"`
	RetainedTo   uint64 `json:"retainedTo"`

	RetentionBlocks uint64 `json:"retentionBlocks"`
	SegmentBlocks   uint64 `json:"segmentBlocks"`

	/* START ALPHA CHANGE 2102 -- bind the exported consensus database -- */
	ConsensusData SnapshotFile `json:"consensusData"`
	/* FINISH ALPHA CHANGE 2102 -- bind the exported consensus database -- */

	SnapshotComplete bool `json:"snapshotComplete"`
}

type Latest struct {
	FormatVersion uint64 `json:"formatVersion"`
	SnapshotKind  Kind   `json:"snapshotKind"`

	BlockNumber uint64     `json:"blockNumber"`
	BlockHash   types.Hash `json:"blockHash"`
	StateRoot   types.Hash `json:"stateRoot"`

	SnapshotDir string `json:"snapshotDir"`
}

type Result struct {
	Kind         Kind
	BlockNumber  uint64
	BlockHash    types.Hash
	StateRoot    types.Hash
	RetainedFrom uint64
	RetainedTo   uint64
	SnapshotDir  string
	LatestPath   string
	Duration     time.Duration
}

/* START ALPHA CHANGE 2076 -- keep recovery construction behind internal builder options -- */
type buildOptions struct {
	locksAlreadyHeld bool

	resolveRetainedFrom func(
		checkpoint uint64,
		source storage.Storage,
		sourceHistoryFrom uint64,
	) (uint64, error)

	beforePublish func(
		buildDir string,
		manifest *Manifest,
	) error

	/* START ALPHA CHANGE 2103 -- validate pinned recovery relationships before artifact reuse -- */
	validateExisting func(
		artifactDir string,
		manifest *Manifest,
	) error
	/* FINISH ALPHA CHANGE 2103 -- validate pinned recovery relationships before artifact reuse -- */

	preservePrevious bool
	reuseExisting    bool

	/* START ALPHA CHANGE 3287 -- support isolated staging synchronization failure tests -- */
	// Nil selects syncArchiveTree. The override is internal and per invocation.
	syncStaging func(string) error
	/* FINISH ALPHA CHANGE 3287 -- support isolated staging synchronization failure tests -- */
}

func Build(config Config) (*Result, error) {
	return buildWithOptions(config, buildOptions{})
}

func buildWithOptions(config Config, options buildOptions) (*Result, error) {
	started := time.Now()

	if err := validateConfig(config); err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 2103 -- require validation when reusing recovery artifacts -- */
	recoveryBuild := options.resolveRetainedFrom != nil
	if recoveryBuild {
		if config.Kind != KindCheckpoint || options.beforePublish == nil {
			return nil, errors.New("recovery construction requires checkpoint mode and metadata finalization")
		}
		if options.reuseExisting && options.validateExisting == nil {
			return nil, errors.New("recovery reuse requires existing relationship validation")
		}
	} else if options.beforePublish != nil ||
		options.validateExisting != nil ||
		options.reuseExisting {
		return nil, errors.New("recovery publication options require a recovery range resolver")
	}
	/* FINISH ALPHA CHANGE 2103 -- require validation when reusing recovery artifacts -- */
	/* FINISH ALPHA CHANGE 2076 -- keep recovery construction behind internal builder options -- */

	logger := config.Logger
	if logger == nil {
		logger = hclog.NewNullLogger()
	}

	chainConfig, err := chain.ImportFromFile(config.ChainPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chain configuration: %w", err)
	}

	if chainConfig.Params.GetEngine() != innbcbft.ConsensusName {
		return nil, fmt.Errorf(
			"snapshot builder requires %q consensus",
			innbcbft.ConsensusName,
		)
	}

	innbcbft.SetupHeaderHashFunc()

	/* START ALPHA CHANGE 1777 -- calculate one destination MDBX map size -- */
	mdbxMapSize, err := mapSizeBytes(config.MDBXMapSizeGB)
	if err != nil {
		return nil, fmt.Errorf("invalid destination MDBX map size: %w", err)
	}
	/* FINISH ALPHA CHANGE 1777 -- calculate one destination MDBX map size -- */

	kindRoot := filepath.Join(config.OutputRoot, config.Kind.directoryName())
	if err := os.MkdirAll(kindRoot, 0770); err != nil {
		return nil, fmt.Errorf("failed to create snapshot-kind directory: %w", err)
	}

	/* START ALPHA CHANGE 2076 -- serialize ordinary and coordinated checkpoint publication -- */
	if !options.locksAlreadyHeld {
		writerLock, err := acquireBuildLock(kindRoot)
		if err != nil {
			return nil, err
		}
		defer writerLock.Close() //nolint:errcheck
	}
	/* FINISH ALPHA CHANGE 2076 -- serialize ordinary and coordinated checkpoint publication -- */

	buildDir, err := os.MkdirTemp(kindRoot, ".building-")
	if err != nil {
		return nil, fmt.Errorf("failed to create unpublished snapshot directory: %w", err)
	}

	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(buildDir)
		}
	}()

	targetConsensusDir := datadir.Consensus(buildDir)

	checkpoint, err := innbcbft.ExportSnapshotState(
		datadir.Consensus(config.SourceDataDir),
		targetConsensusDir,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to export consensus checkpoint: %w", err)
	}

	if checkpoint == 0 {
		return nil, errors.New("refusing to publish a genesis-only snapshot")
	}

	/* START ALPHA CHANGE 2102 -- hash the closed exported consensus database -- */
	consensusData, err := snapshotConsensusFileIdentity(buildDir)
	if err != nil {
		return nil, fmt.Errorf("identify exported consensus database: %w", err)
	}
	/* FINISH ALPHA CHANGE 2102 -- hash the closed exported consensus database -- */

	/* START ALPHA CHANGE 1778 -- discover source blockchain MDBX geometry -- */
	sourceBlockchain, err := mdbxstorage.NewMDBXStorageReadOnly(
		datadir.Blockchain(config.SourceDataDir),
		logger,
	)
	/* FINISH ALPHA CHANGE 1778 -- discover source blockchain MDBX geometry -- */
	if err != nil {
		return nil, fmt.Errorf("failed to open source blockchain storage: %w", err)
	}
	defer sourceBlockchain.Close() //nolint:errcheck

	historyStorage, ok := sourceBlockchain.(storage.HistoryAvailabilityStorage)
	if !ok {
		return nil, errors.New("source blockchain does not expose history availability")
	}

	/* START ALPHA CHANGE 2076 -- allow nonzero source history only during recovery construction -- */
	sourceHistoryFrom := historyStorage.HistoryFrom()
	if !recoveryBuild && sourceHistoryFrom != 0 {
		return nil, fmt.Errorf(
			"snapshot source is not a genesis-origin full-history database: history starts at %d",
			sourceHistoryFrom,
		)
	}
	/* FINISH ALPHA CHANGE 2076 -- allow nonzero source history only during recovery construction -- */

	/* START ALPHA CHANGE 1811 -- validate persisted allocation-derived genesis -- */
	genesisHash, ok := sourceBlockchain.ReadCanonicalHash(0)
	if !ok {
		return nil, errors.New("source blockchain genesis mapping is missing")
	}

	genesisHeader, err := sourceBlockchain.ReadHeader(genesisHash)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read source genesis header: %w",
			err,
		)
	}

	if genesisHeader.Number != 0 {
		return nil, fmt.Errorf(
			"source genesis header number mismatch: expected 0, got %d",
			genesisHeader.Number,
		)
	}

	if genesisHeader.Hash != genesisHash {
		return nil, fmt.Errorf(
			"source genesis header hash mismatch: expected %s, got %s",
			genesisHash,
			genesisHeader.Hash,
		)
	}

	expectedGenesis := chainConfig.Genesis.GenesisHeader()
	expectedGenesis.StateRoot = genesisHeader.StateRoot
	expectedGenesis.ComputeHash()

	if genesisHash != expectedGenesis.Hash {
		return nil, fmt.Errorf(
			"source genesis hash mismatch: expected %s, got %s",
			expectedGenesis.Hash,
			genesisHash,
		)
	}
	/* FINISH ALPHA CHANGE 1811 -- validate persisted allocation-derived genesis -- */

	checkpointHash, ok := sourceBlockchain.ReadCanonicalHash(checkpoint)
	if !ok {
		return nil, fmt.Errorf(
			"source canonical checkpoint %d is missing",
			checkpoint,
		)
	}

	checkpointHeader, err := sourceBlockchain.ReadHeader(checkpointHash)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read source checkpoint header %d: %w",
			checkpoint,
			err,
		)
	}

	retainedFrom := calculateRetainedFrom(
		config.Kind,
		checkpoint,
		config.RetentionBlocks,
		config.SegmentBlocks,
	)

	/* START ALPHA CHANGE 2076 -- enforce recovery range and minimum retention before copying -- */
	manifestRetentionBlocks := config.RetentionBlocks

	if recoveryBuild {
		retainedFrom, err = options.resolveRetainedFrom(
			checkpoint,
			sourceBlockchain,
			sourceHistoryFrom,
		)
		if err != nil {
			return nil, err
		}
		if retainedFrom == 0 || retainedFrom > checkpoint {
			return nil, fmt.Errorf(
				"resolved recovery range %d...%d is invalid",
				retainedFrom,
				checkpoint,
			)
		}
		if sourceHistoryFrom > retainedFrom {
			return nil, fmt.Errorf(
				"source history starts at %d, after recovery boundary %d",
				sourceHistoryFrom,
				retainedFrom,
			)
		}

		// retainedFrom is positive, so the addition cannot overflow.
		manifestRetentionBlocks = checkpoint - retainedFrom + 1
		if manifestRetentionBlocks < config.RetentionBlocks {
			return nil, errors.New("resolved recovery range is shorter than minimum retention")
		}
	}
	/* FINISH ALPHA CHANGE 2076 -- enforce recovery range and minimum retention before copying -- */

	/* START ALPHA CHANGE 1812 -- publish allocation-derived genesis state root -- */
	manifest := &Manifest{
		FormatVersion:    FormatVersion,
		SnapshotKind:     config.Kind,
		BlockNumber:      checkpoint,
		BlockHash:        checkpointHeader.Hash,
		StateRoot:        checkpointHeader.StateRoot,
		GenesisHash:      genesisHash,
		GenesisStateRoot: genesisHeader.StateRoot,
		RetainedFrom:     retainedFrom,
		RetainedTo:       checkpoint,
		/* START ALPHA CHANGE 2076 -- record the actual recovery retention length -- */
		RetentionBlocks: manifestRetentionBlocks,
		/* FINISH ALPHA CHANGE 2076 -- record the actual recovery retention length -- */
		/* START ALPHA CHANGE 2102 -- include the consensus database commitment -- */
		ConsensusData: consensusData,
		/* FINISH ALPHA CHANGE 2102 -- include the consensus database commitment -- */
		SnapshotComplete: true,
	}
	/* FINISH ALPHA CHANGE 1812 -- publish allocation-derived genesis state root -- */

	if config.Kind == KindSegmented {
		manifest.SegmentBlocks = config.SegmentBlocks
	}

	/* START ALPHA CHANGE 2103 -- validate and reuse a completed recovery artifact before rebuilding state -- */
	if options.reuseExisting {
		reused, err := reusePublishedRecovery(
			kindRoot,
			manifest,
			mdbxMapSize,
			logger,
			options.validateExisting,
			/* START ALPHA CHANGE 3464 -- include geometry in recovery-artifact reuse validation -- */
			config.MDBXPageSizeBytes,
		/* FINISH ALPHA CHANGE 3464 -- include geometry in recovery-artifact reuse validation -- */
		)
		if err != nil {
			return nil, err
		}
		if reused != nil {
			reused.Duration = time.Since(started)

			// published remains false so the temporary consensus export is removed.
			return reused, nil
		}
	}
	/* FINISH ALPHA CHANGE 2103 -- validate and reuse a completed recovery artifact before rebuilding state -- */

	targetBlockchain, err := openTargetBlockchain(
		config,
		datadir.Blockchain(buildDir),
		mdbxMapSize,
		logger,
	)
	if err != nil {
		return nil, err
	}

	copyErr := copyBlockchainSuffix(
		sourceBlockchain,
		targetBlockchain,
		retainedFrom,
		checkpoint,
	)
	closeErr := targetBlockchain.Close()

	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("failed to close snapshot blockchain storage: %w", closeErr)
	}

	/* START ALPHA CHANGE 1779 -- read source geometry and apply one destination geometry -- */
	sourceTrie, err := itrie.NewMDBXStorageReadOnly(
		datadir.Trie(config.SourceDataDir),
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open source trie storage: %w", err)
	}
	defer sourceTrie.Close() //nolint:errcheck

	/* START ALPHA CHANGE 3464 -- create the snapshot trie using destination geometry -- */
	targetTrie, err := itrie.NewMDBXStorageWithMapSize(
		datadir.Trie(buildDir),
		logger,
		mdbxMapSize,
		config.MDBXPageSizeBytes,
	)
	/* FINISH ALPHA CHANGE 3464 -- create the snapshot trie using destination geometry -- */
	/* FINISH ALPHA CHANGE 1779 -- read source geometry and apply one destination geometry -- */
	if err != nil {
		return nil, fmt.Errorf("failed to open snapshot trie storage: %w", err)
	}

	copyErr = itrie.CopyTrie(
		checkpointHeader.StateRoot.Bytes(),
		sourceTrie,
		targetTrie,
		nil,
		false,
	)
	/* START ALPHA CHANGE 2104 -- validate complete state once after closing and reopening the staged artifact -- */
	closeErr = targetTrie.Close()
	/* FINISH ALPHA CHANGE 2104 -- validate complete state once after closing and reopening the staged artifact -- */
	if copyErr != nil {
		return nil, fmt.Errorf("failed to copy checkpoint trie: %w", copyErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("failed to close snapshot trie storage: %w", closeErr)
	}

	manifestPath := filepath.Join(buildDir, manifestFileName)
	if err := writeJSONFile(manifestPath, manifest); err != nil {
		return nil, fmt.Errorf("failed to write snapshot manifest: %w", err)
	}

	/* START ALPHA CHANGE 3464 -- verify constructed geometry before publication -- */
	if err := validateArtifact(
		buildDir,
		manifest,
		mdbxMapSize,
		logger,
		config.MDBXPageSizeBytes,
	); err != nil {
		/* FINISH ALPHA CHANGE 3464 -- verify constructed geometry before publication -- */
		return nil, fmt.Errorf("snapshot artifact validation failed: %w", err)
	}

	/* START ALPHA CHANGE 2076 -- complete and synchronize recovery metadata before publication -- */
	if options.beforePublish != nil {
		if err := options.beforePublish(buildDir, manifest); err != nil {
			return nil, fmt.Errorf("finalize recovery checkpoint staging: %w", err)
		}
	}
	/* FINISH ALPHA CHANGE 2076 -- complete and synchronize recovery metadata before publication -- */

	finalName := snapshotDirectoryName(checkpoint, checkpointHeader.Hash)
	finalDir := filepath.Join(kindRoot, finalName)

	/* START ALPHA CHANGE 2076 -- validate exact retries and retain completed recovery checkpoints -- */
	/* START ALPHA CHANGE 2103 -- publish only new artifacts after early recovery reuse -- */
	if _, err := os.Lstat(finalDir); err == nil {
		return nil, fmt.Errorf("snapshot directory already exists: %s", finalDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to inspect final snapshot directory: %w", err)
	}

	previousLatest, err := loadOptionalLatest(filepath.Join(kindRoot, latestFileName))
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 3288 -- synchronize validated ordinary snapshots before publication -- */
	// Recovery construction already synchronizes its completed staging tree
	// through beforePublish. Ordinary builds need the same durability barrier.
	if !recoveryBuild {
		syncStaging := syncArchiveTree
		if options.syncStaging != nil {
			syncStaging = options.syncStaging
		}

		if err := syncStaging(buildDir); err != nil {
			return nil, fmt.Errorf("failed to synchronize snapshot staging: %w", err)
		}
	}
	/* FINISH ALPHA CHANGE 3288 -- synchronize validated ordinary snapshots before publication -- */

	if err := os.Rename(buildDir, finalDir); err != nil {
		return nil, fmt.Errorf("failed to publish snapshot directory: %w", err)
	}
	createdFinal := true
	/* FINISH ALPHA CHANGE 2103 -- publish only new artifacts after early recovery reuse -- */

	if err := syncDirectory(kindRoot); err != nil {
		// Recovery artifacts remain available for validation and retry.
		// Preserve the existing cleanup policy for ordinary snapshots.
		if createdFinal && !recoveryBuild {
			cleanupErr := removeDirectoryAndSync(kindRoot, finalDir)
			if cleanupErr != nil {
				return nil, errors.Join(
					fmt.Errorf("failed to persist published snapshot directory: %w", err),
					fmt.Errorf("failed to remove unpublished snapshot directory: %w", cleanupErr),
				)
			}
		}

		return nil, fmt.Errorf("failed to persist published snapshot directory: %w", err)
	}
	/* FINISH ALPHA CHANGE 2076 -- validate exact retries and retain completed recovery checkpoints -- */

	latest := &Latest{
		FormatVersion: FormatVersion,
		SnapshotKind:  config.Kind,
		BlockNumber:   checkpoint,
		BlockHash:     checkpointHeader.Hash,
		StateRoot:     checkpointHeader.StateRoot,
		SnapshotDir:   finalName,
	}

	latestPath := filepath.Join(kindRoot, latestFileName)
	if err := writeJSONAtomic(latestPath, latest); err != nil {
		/* START ALPHA CHANGE 2076 -- retain completed recovery artifacts when pointer publication fails -- */
		if createdFinal && !recoveryBuild {
			cleanupErr := removeDirectoryAndSync(kindRoot, finalDir)
			if cleanupErr != nil {
				return nil, errors.Join(
					fmt.Errorf("failed to publish latest snapshot pointer: %w", err),
					fmt.Errorf("failed to remove unpublished snapshot directory: %w", cleanupErr),
				)
			}
		}
		/* FINISH ALPHA CHANGE 2076 -- retain completed recovery artifacts when pointer publication fails -- */

		return nil, fmt.Errorf("failed to publish latest snapshot pointer: %w", err)
	}

	/* START ALPHA CHANGE 1837 -- persist latest snapshot pointer rename -- */
	if err := syncDirectory(kindRoot); err != nil {
		return nil, fmt.Errorf("failed to persist latest snapshot pointer: %w", err)
	}
	/* FINISH ALPHA CHANGE 1837 -- persist latest snapshot pointer rename -- */

	/* START ALPHA CHANGE 2103 -- early retries clean their temporary export through the existing defer -- */
	published = true
	/* FINISH ALPHA CHANGE 2103 -- early retries clean their temporary export through the existing defer -- */

	/* START ALPHA CHANGE 2077 -- preserve recovery-linked checkpoints and report ambiguous metadata -- */
	if previousLatest != nil &&
		previousLatest.SnapshotDir != finalName &&
		!options.preservePrevious {
		previousDir := filepath.Join(kindRoot, previousLatest.SnapshotDir)
		recoveryPath := filepath.Join(previousDir, recoverySetFileName)

		_, recoveryErr := os.Lstat(recoveryPath)
		switch {
		case recoveryErr == nil:
			if _, err := loadRecoverySet(recoveryPath); err != nil {
				logger.Error(
					"preserving previous snapshot with invalid or unreadable recovery metadata",
					"path", previousDir,
					"err", err,
				)
			} else {
				logger.Info(
					"preserving previous snapshot containing a recovery relationship",
					"path", previousDir,
				)
			}

		case errors.Is(recoveryErr, os.ErrNotExist):
			if err := removeDirectoryAndSync(kindRoot, previousDir); err != nil {
				logger.Warn(
					"new snapshot published but previous snapshot could not be removed durably",
					"path", previousDir,
					"err", err,
				)
			}

		default:
			logger.Error(
				"preserving previous snapshot because recovery metadata could not be inspected",
				"path", previousDir,
				"err", recoveryErr,
			)
		}
	}
	/* FINISH ALPHA CHANGE 2077 -- preserve recovery-linked checkpoints and report ambiguous metadata -- */

	return &Result{
		Kind:         config.Kind,
		BlockNumber:  checkpoint,
		BlockHash:    checkpointHeader.Hash,
		StateRoot:    checkpointHeader.StateRoot,
		RetainedFrom: retainedFrom,
		RetainedTo:   checkpoint,
		SnapshotDir:  finalDir,
		LatestPath:   latestPath,
		Duration:     time.Since(started),
	}, nil
}

func validateConfig(config Config) error {
	if config.Kind != KindSegmented && config.Kind != KindCheckpoint {
		return fmt.Errorf("unsupported snapshot kind %q", config.Kind)
	}

	if strings.TrimSpace(config.ChainPath) == "" {
		return errors.New("chain configuration path is required")
	}

	if strings.TrimSpace(config.SourceDataDir) == "" {
		return errors.New("source data directory is required")
	}

	if strings.TrimSpace(config.OutputRoot) == "" {
		return errors.New("snapshot output root is required")
	}

	if config.RetentionBlocks == 0 {
		return errors.New("snapshot retention blocks must be greater than zero")
	}

	/* START ALPHA CHANGE 3464 -- validate destination page size before construction -- */
	if _, err := mdbxconfig.Resolve(config.MDBXPageSizeBytes); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3464 -- validate destination page size before construction -- */

	/* START ALPHA CHANGE 1780 -- validate one destination MDBX map size -- */
	if config.MDBXMapSizeGB == 0 {
		return errors.New("destination MDBX map size must be greater than zero")
	}
	/* FINISH ALPHA CHANGE 1780 -- validate one destination MDBX map size -- */

	/* START ALPHA CHANGE 3141 -- validate segmented snapshot counts before construction -- */
	if config.Kind == KindSegmented {
		if _, err := segmentedstorage.RetainedSegmentCount(
			config.SegmentBlocks,
			config.RetentionBlocks,
		); err != nil {
			return err
		}
	}
	/* FINISH ALPHA CHANGE 3141 -- validate segmented snapshot counts before construction -- */

	return nil
}

func mapSizeBytes(sizeGB uint64) (int, error) {
	const gib = uint64(1024 * 1024 * 1024)

	if sizeGB > uint64(math.MaxInt)/gib {
		return 0, errors.New("configured size exceeds platform integer range")
	}

	return int(sizeGB * gib), nil
}

func calculateRetainedFrom(
	kind Kind,
	checkpoint uint64,
	retentionBlocks uint64,
	segmentBlocks uint64,
) uint64 {
	if checkpoint <= retentionBlocks {
		return 1
	}

	if kind == KindCheckpoint {
		return checkpoint - retentionBlocks + 1
	}

	activeSegmentStart := ((checkpoint - 1) / segmentBlocks * segmentBlocks) + 1
	retainedSegments := retentionBlocks / segmentBlocks
	precedingSegments := retainedSegments - 1
	offset := precedingSegments * segmentBlocks

	if activeSegmentStart <= offset {
		return 1
	}

	return activeSegmentStart - offset
}

func openTargetBlockchain(
	config Config,
	path string,
	mapSize int,
	logger hclog.Logger,
) (storage.Storage, error) {
	if config.Kind == KindSegmented {
		target, err := segmentedstorage.NewStorage(
			path,
			logger,
			config.SegmentBlocks,
			config.RetentionBlocks,
			mapSize,
			/* START ALPHA CHANGE 3464 -- configure destination segment pages -- */
			config.MDBXPageSizeBytes,
			/* FINISH ALPHA CHANGE 3464 -- configure destination segment pages -- */
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create segmented snapshot storage: %w", err)
		}

		return target, nil
	}

	/* START ALPHA CHANGE 3464 -- configure checkpoint blockchain pages -- */
	target, err := mdbxstorage.NewMDBXStorageWithMapSize(
		path, logger, mapSize, config.MDBXPageSizeBytes,
	)
	/* FINISH ALPHA CHANGE 3464 -- configure checkpoint blockchain pages -- */
	if err != nil {
		return nil, fmt.Errorf("failed to create checkpoint snapshot storage: %w", err)
	}

	return target, nil
}

func copyBlockchainSuffix(
	source storage.Storage,
	target storage.Storage,
	retainedFrom uint64,
	checkpoint uint64,
) error {
	if err := copyCanonicalBlock(source, target, 0); err != nil {
		return fmt.Errorf("failed to copy genesis: %w", err)
	}

	for number := retainedFrom; number <= checkpoint; number++ {
		if err := copyCanonicalBlock(source, target, number); err != nil {
			return fmt.Errorf("failed to copy canonical block %d: %w", number, err)
		}
	}

	return nil
}

func copyCanonicalBlock(
	source storage.Storage,
	target storage.Storage,
	number uint64,
) error {
	hash, ok := source.ReadCanonicalHash(number)
	if !ok {
		return errors.New("canonical hash is missing")
	}

	header, err := source.ReadHeader(hash)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	batch := storage.NewBatchWriterForBlock(target, number)
	batch.PutCanonicalHeader(header)

	if number > 0 {
		body, err := source.ReadBody(hash)
		if err != nil {
			return fmt.Errorf("failed to read body: %w", err)
		}

		receipts, err := source.ReadReceipts(hash)
		if err != nil {
			return fmt.Errorf("failed to read receipts: %w", err)
		}

		batch.PutBody(hash, body)
		batch.PutReceipts(hash, receipts)

		for _, transaction := range body.Transactions {
			batch.PutTxLookup(transaction.Hash, hash)
		}
	}

	if err := batch.WriteBatch(); err != nil {
		return fmt.Errorf("failed to write snapshot block: %w", err)
	}

	return nil
}

/* START ALPHA CHANGE 3464 -- inspect artifact geometry before reopening components -- */
func validateArtifact(
	artifactDir string,
	expected *Manifest,
	mdbxMapSize int,
	logger hclog.Logger,
	pageSizes ...uint64,
) error {
	pageSize, err := mdbxconfig.Resolve(pageSizes...)
	if err != nil {
		return err
	}
	if err := mdbxconfig.CheckChainData(
		datadir.ChainData(artifactDir), uint64(pageSize),
	); err != nil {
		return err
	}

	manifest, err := LoadManifest(filepath.Join(artifactDir, manifestFileName))
	/* FINISH ALPHA CHANGE 3464 -- inspect artifact geometry before reopening components -- */
	if err != nil {
		return err
	}

	if *manifest != *expected {
		return errors.New("persisted manifest does not match constructed manifest")
	}

	var blockchainStorage storage.Storage

	if manifest.SnapshotKind == KindSegmented {
		blockchainStorage, err = segmentedstorage.NewStorage(
			datadir.Blockchain(artifactDir),
			logger,
			manifest.SegmentBlocks,
			manifest.RetentionBlocks,
			mdbxMapSize,
			/* START ALPHA CHANGE 3464 -- preserve artifact pages during segmented validation -- */
			uint64(pageSize),
			/* FINISH ALPHA CHANGE 3464 -- preserve artifact pages during segmented validation -- */
		)
	} else {
		blockchainStorage, err = mdbxstorage.NewMDBXStorageReadOnly(
			datadir.Blockchain(artifactDir),
			logger,
		)
	}
	if err != nil {
		return fmt.Errorf("failed to reopen snapshot blockchain: %w", err)
	}

	if err := validateBlockchain(blockchainStorage, manifest); err != nil {
		_ = blockchainStorage.Close()

		return err
	}

	if err := blockchainStorage.Close(); err != nil {
		return fmt.Errorf("failed to close validated snapshot blockchain: %w", err)
	}

	trieStorage, err := itrie.NewMDBXStorageReadOnly(
		datadir.Trie(artifactDir),
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to reopen snapshot trie: %w", err)
	}

	/* START ALPHA CHANGE 2104 -- validate complete executable state and committed consensus bytes -- */
	stateErr := itrie.ValidateCompleteState(manifest.StateRoot, trieStorage)
	closeErr := trieStorage.Close()

	if err := errors.Join(stateErr, closeErr); err != nil {
		return fmt.Errorf("failed to validate complete snapshot state: %w", err)
	}

	if err := verifySnapshotConsensusFile(artifactDir, manifest.ConsensusData); err != nil {
		return fmt.Errorf("failed to validate snapshot consensus data: %w", err)
	}
	/* FINISH ALPHA CHANGE 2104 -- validate complete executable state and committed consensus bytes -- */

	consensusCheckpoint, err := innbcbft.SnapshotStateCheckpoint(
		datadir.Consensus(artifactDir),
	)
	if err != nil {
		return fmt.Errorf("failed to validate snapshot consensus state: %w", err)
	}

	if consensusCheckpoint != manifest.BlockNumber {
		return fmt.Errorf(
			"snapshot consensus checkpoint mismatch: expected %d, got %d",
			manifest.BlockNumber,
			consensusCheckpoint,
		)
	}

	return nil
}

func validateBlockchain(
	blockchainStorage storage.Storage,
	manifest *Manifest,
) error {
	/* START ALPHA CHANGE 1813 -- validate snapshot genesis header commitment -- */
	genesisHash, ok := blockchainStorage.ReadCanonicalHash(0)
	if !ok {
		return errors.New("snapshot genesis mapping is missing")
	}

	if genesisHash != manifest.GenesisHash {
		return fmt.Errorf(
			"snapshot genesis hash mismatch: expected %s, got %s",
			manifest.GenesisHash,
			genesisHash,
		)
	}

	genesisHeader, err := blockchainStorage.ReadHeader(genesisHash)
	if err != nil {
		return fmt.Errorf(
			"failed to read snapshot genesis header: %w",
			err,
		)
	}

	if genesisHeader.Number != 0 {
		return fmt.Errorf(
			"snapshot genesis header number mismatch: expected 0, got %d",
			genesisHeader.Number,
		)
	}

	if genesisHeader.Hash != genesisHash {
		return fmt.Errorf(
			"snapshot genesis header hash mismatch: expected %s, got %s",
			genesisHash,
			genesisHeader.Hash,
		)
	}

	if genesisHeader.StateRoot != manifest.GenesisStateRoot {
		return fmt.Errorf(
			"snapshot genesis state root mismatch: expected %s, got %s",
			manifest.GenesisStateRoot,
			genesisHeader.StateRoot,
		)
	}
	/* FINISH ALPHA CHANGE 1813 -- validate snapshot genesis header commitment -- */

	/* START ALPHA CHANGE 3332 -- verify the genesis link when retained history starts at block one -- */
	var previous *types.Header
	if manifest.RetainedFrom == 1 {
		previous = genesisHeader
	}
	/* FINISH ALPHA CHANGE 3332 -- verify the genesis link when retained history starts at block one -- */

	for number := manifest.RetainedFrom; number <= manifest.RetainedTo; number++ {
		hash, ok := blockchainStorage.ReadCanonicalHash(number)
		if !ok {
			return fmt.Errorf("snapshot canonical block %d is missing", number)
		}

		header, err := blockchainStorage.ReadHeader(hash)
		if err != nil {
			return fmt.Errorf("failed to read snapshot header %d: %w", number, err)
		}

		if header.Number != number {
			return fmt.Errorf(
				"snapshot header number mismatch: expected %d, got %d",
				number,
				header.Number,
			)
		}

		/* START ALPHA CHANGE 2104 -- bind retained headers to canonical mappings -- */
		if header.Hash != hash {
			return fmt.Errorf("snapshot header hash mismatch at block %d", number)
		}
		/* FINISH ALPHA CHANGE 2104 -- bind retained headers to canonical mappings -- */

		if previous != nil && header.ParentHash != previous.Hash {
			return fmt.Errorf("snapshot parent hash mismatch at block %d", number)
		}

		body, err := blockchainStorage.ReadBody(hash)
		if err != nil {
			return fmt.Errorf("failed to read snapshot body %d: %w", number, err)
		}

		if root := buildroot.CalculateTransactionsRoot(
			body.Transactions,
			number,
		); root != header.TxRoot {
			return fmt.Errorf("snapshot transaction root mismatch at block %d", number)
		}

		if root := buildroot.CalculateUncleRoot(body.Uncles); root != header.Sha3Uncles {
			return fmt.Errorf("snapshot uncle root mismatch at block %d", number)
		}

		receipts, err := blockchainStorage.ReadReceipts(hash)
		if err != nil {
			return fmt.Errorf("failed to read snapshot receipts %d: %w", number, err)
		}

		if root := buildroot.CalculateReceiptsRoot(receipts); root != header.ReceiptsRoot {
			return fmt.Errorf("snapshot receipt root mismatch at block %d", number)
		}

		for _, transaction := range body.Transactions {
			blockHash, ok := blockchainStorage.ReadTxLookup(transaction.Hash)
			if !ok || blockHash != hash {
				return fmt.Errorf(
					"snapshot transaction lookup mismatch at block %d",
					number,
				)
			}
		}

		previous = header
	}

	/* START ALPHA CHANGE 2104 -- bind final retained header to manifest checkpoint and state -- */
	if previous == nil {
		return errors.New("snapshot retained range contains no checkpoint header")
	}
	if previous.Number != manifest.BlockNumber {
		return errors.New("final retained header number mismatch")
	}
	if previous.Hash != manifest.BlockHash {
		return errors.New("final retained header hash mismatch")
	}
	if previous.StateRoot != manifest.StateRoot {
		return errors.New("final retained header state root mismatch")
	}
	/* FINISH ALPHA CHANGE 2104 -- bind final retained header to manifest checkpoint and state -- */

	headNumber, ok := blockchainStorage.ReadHeadNumber()
	if !ok || headNumber != manifest.BlockNumber {
		return fmt.Errorf(
			"snapshot head number mismatch: expected %d, got %d",
			manifest.BlockNumber,
			headNumber,
		)
	}

	headHash, ok := blockchainStorage.ReadHeadHash()
	if !ok || headHash != manifest.BlockHash {
		return fmt.Errorf(
			"snapshot head hash mismatch: expected %s, got %s",
			manifest.BlockHash,
			headHash,
		)
	}

	historyStorage, ok := blockchainStorage.(storage.HistoryAvailabilityStorage)
	if !ok {
		return errors.New("snapshot blockchain does not expose history availability")
	}

	/* START ALPHA CHANGE 3332 -- translate the retained non-genesis range into the expected storage history floor -- */
	expectedHistoryFrom := manifest.RetainedFrom
	if expectedHistoryFrom == 1 {
		expectedHistoryFrom = 0
	}

	actualHistoryFrom := historyStorage.HistoryFrom()
	if actualHistoryFrom != expectedHistoryFrom {
		return fmt.Errorf(
			"snapshot history floor mismatch: expected %d, got %d",
			expectedHistoryFrom,
			actualHistoryFrom,
		)
	}
	/* FINISH ALPHA CHANGE 3332 -- translate the retained non-genesis range into the expected storage history floor -- */

	return nil
}

func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("failed to decode snapshot manifest: %w", err)
	}

	if err := ValidateManifest(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func ValidateManifest(manifest *Manifest) error {
	if manifest == nil {
		return errors.New("snapshot manifest is nil")
	}

	if manifest.FormatVersion != FormatVersion {
		return fmt.Errorf(
			"unsupported snapshot format version %d",
			manifest.FormatVersion,
		)
	}

	if manifest.SnapshotKind != KindSegmented &&
		manifest.SnapshotKind != KindCheckpoint {
		return fmt.Errorf("unsupported snapshot kind %q", manifest.SnapshotKind)
	}

	if !manifest.SnapshotComplete {
		return errors.New("snapshot manifest is not complete")
	}

	if manifest.BlockNumber == 0 {
		return errors.New("snapshot block number must be greater than zero")
	}

	if manifest.BlockHash == types.ZeroHash {
		return errors.New("snapshot block hash must not be zero")
	}

	if manifest.StateRoot == types.ZeroHash {
		return errors.New("snapshot state root must not be zero")
	}

	if manifest.GenesisHash == types.ZeroHash {
		return errors.New("snapshot genesis hash must not be zero")
	}

	/* START ALPHA CHANGE 1814 -- require persisted genesis state commitment -- */
	if manifest.GenesisStateRoot == types.ZeroHash {
		return errors.New("snapshot genesis state root must not be zero")
	}
	/* FINISH ALPHA CHANGE 1814 -- require persisted genesis state commitment -- */

	if manifest.RetainedFrom == 0 {
		return errors.New("snapshot retained range must start after genesis")
	}

	if manifest.RetainedTo != manifest.BlockNumber {
		return errors.New("snapshot retained range must end at the checkpoint")
	}

	if manifest.RetainedFrom > manifest.RetainedTo {
		return errors.New("snapshot retained range is invalid")
	}

	if manifest.RetentionBlocks == 0 {
		return errors.New("snapshot retention blocks must be greater than zero")
	}

	/* START ALPHA CHANGE 2102 -- require the canonical consensus file commitment -- */
	if manifest.ConsensusData.Path != snapshotConsensusDataPath {
		return errors.New("snapshot consensus data path is invalid")
	}
	if manifest.ConsensusData.Bytes == 0 {
		return errors.New("snapshot consensus data size must be greater than zero")
	}
	if err := validateSHA256(manifest.ConsensusData.SHA256); err != nil {
		return fmt.Errorf("snapshot consensus data checksum is invalid: %w", err)
	}
	/* FINISH ALPHA CHANGE 2102 -- require the canonical consensus file commitment -- */

	if manifest.SnapshotKind == KindSegmented {
		if manifest.SegmentBlocks == 0 {
			return errors.New("segmented snapshot segment size must be greater than zero")
		}

		if manifest.RetentionBlocks%manifest.SegmentBlocks != 0 {
			return errors.New(
				"segmented snapshot retention must be divisible by segment size",
			)
		}
	} else if manifest.SegmentBlocks != 0 {
		return errors.New("checkpoint snapshot must not define segment geometry")
	}

	/* START ALPHA CHANGE 1714 -- require the exact retained snapshot range -- */
	expectedRetainedFrom := calculateRetainedFrom(
		manifest.SnapshotKind,
		manifest.BlockNumber,
		manifest.RetentionBlocks,
		manifest.SegmentBlocks,
	)
	if manifest.RetainedFrom != expectedRetainedFrom {
		return fmt.Errorf(
			"snapshot retained range must start at %d, got %d",
			expectedRetainedFrom,
			manifest.RetainedFrom,
		)
	}
	/* FINISH ALPHA CHANGE 1714 -- require the exact retained snapshot range -- */

	return nil
}

/* START ALPHA CHANGE 1700 -- expose published snapshot loading and validation -- */
func LoadLatest(path string) (*Latest, error) {
	latest, err := loadOptionalLatest(path)
	if err != nil {
		return nil, err
	}

	if latest == nil {
		return nil, errors.New("latest snapshot pointer is missing")
	}

	if latest.BlockNumber == 0 {
		return nil, errors.New("latest snapshot block number must be greater than zero")
	}

	if latest.BlockHash == types.ZeroHash {
		return nil, errors.New("latest snapshot block hash must not be zero")
	}

	if latest.StateRoot == types.ZeroHash {
		return nil, errors.New("latest snapshot state root must not be zero")
	}

	return latest, nil
}

func ValidateLatestManifest(latest *Latest, manifest *Manifest) error {
	if latest == nil {
		return errors.New("latest snapshot pointer is nil")
	}

	if err := ValidateManifest(manifest); err != nil {
		return err
	}

	if latest.FormatVersion != manifest.FormatVersion {
		return errors.New("latest pointer and manifest format versions do not match")
	}

	if latest.SnapshotKind != manifest.SnapshotKind {
		return errors.New("latest pointer and manifest snapshot kinds do not match")
	}

	if latest.BlockNumber != manifest.BlockNumber {
		return errors.New("latest pointer and manifest block numbers do not match")
	}

	if latest.BlockHash != manifest.BlockHash {
		return errors.New("latest pointer and manifest block hashes do not match")
	}

	if latest.StateRoot != manifest.StateRoot {
		return errors.New("latest pointer and manifest state roots do not match")
	}

	expectedDir := snapshotDirectoryName(manifest.BlockNumber, manifest.BlockHash)
	if latest.SnapshotDir != expectedDir {
		return fmt.Errorf(
			"latest snapshot directory %q does not match expected %q",
			latest.SnapshotDir,
			expectedDir,
		)
	}

	return nil
}

/* START ALPHA CHANGE 3464 -- accept expected pages while preserving existing validation callers -- */
func ValidatePublishedArtifact(
	artifactDir string,
	expectedKind Kind,
	mdbxMapSizeGB uint64,
	logger hclog.Logger,
	pageSizes ...uint64,
) (*Manifest, error) {
	/* FINISH ALPHA CHANGE 3464 -- accept expected pages while preserving existing validation callers -- */
	manifest, err := LoadManifest(filepath.Join(artifactDir, manifestFileName))
	if err != nil {
		return nil, err
	}

	if manifest.SnapshotKind != expectedKind {
		return nil, fmt.Errorf(
			"snapshot kind mismatch: expected %q, got %q",
			expectedKind,
			manifest.SnapshotKind,
		)
	}

	mdbxMapSize, err := mapSizeBytes(mdbxMapSizeGB)
	if err != nil {
		return nil, fmt.Errorf("invalid MDBX map size: %w", err)
	}

	if logger == nil {
		logger = hclog.NewNullLogger()
	}

	/* START ALPHA CHANGE 3464 -- propagate the expected artifact page size -- */
	if err := validateArtifact(
		artifactDir,
		manifest,
		mdbxMapSize,
		logger,
		pageSizes...,
	); err != nil {
		/* FINISH ALPHA CHANGE 3464 -- propagate the expected artifact page size -- */
		return nil, err
	}

	return manifest, nil
}

/* FINISH ALPHA CHANGE 1700 -- expose published snapshot loading and validation -- */

/* START ALPHA CHANGE 1751 -- align snapshot builder directories with consumer paths -- */
func (kind Kind) directoryName() string {
	if kind == KindSegmented {
		return "segmented"
	}

	return "checkpoint"
}

/* FINISH ALPHA CHANGE 1751 -- align snapshot builder directories with consumer paths -- */

func snapshotDirectoryName(blockNumber uint64, blockHash types.Hash) string {
	hashText := strings.TrimPrefix(blockHash.String(), "0x")

	return fmt.Sprintf(
		"block-%020d-%s",
		blockNumber,
		hashText[:12],
	)
}

func loadOptionalLatest(path string) (*Latest, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read latest snapshot pointer: %w", err)
	}

	var latest Latest
	if err := json.Unmarshal(raw, &latest); err != nil {
		return nil, fmt.Errorf("failed to decode latest snapshot pointer: %w", err)
	}

	if latest.FormatVersion != FormatVersion {
		return nil, fmt.Errorf(
			"unsupported latest snapshot format version %d",
			latest.FormatVersion,
		)
	}

	if latest.SnapshotKind != KindSegmented &&
		latest.SnapshotKind != KindCheckpoint {
		return nil, fmt.Errorf(
			"unsupported latest snapshot kind %q",
			latest.SnapshotKind,
		)
	}

	expectedDir := snapshotDirectoryName(latest.BlockNumber, latest.BlockHash)
	if latest.SnapshotDir != expectedDir {
		return nil, fmt.Errorf(
			"latest snapshot directory %q does not match expected %q",
			latest.SnapshotDir,
			expectedDir,
		)
	}

	return &latest, nil
}

func writeJSONFile(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	raw = append(raw, '\n')

	return os.WriteFile(path, raw, 0660)
}

func writeJSONAtomic(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	raw = append(raw, '\n')
	tmpPath := path + ".tmp"

	file, err := os.OpenFile(
		tmpPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0660,
	)
	if err != nil {
		return err
	}

	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)

		return err
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)

		return err
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)

		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)

		return err
	}

	return nil
}

/* START ALPHA CHANGE 1838 -- persist snapshot directory mutations -- */
func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}

	if err := directory.Sync(); err != nil {
		_ = directory.Close()

		return err
	}

	return directory.Close()
}

func removeDirectoryAndSync(parentPath string, targetPath string) error {
	if err := os.RemoveAll(targetPath); err != nil {
		return err
	}

	return syncDirectory(parentPath)
}

/* FINISH ALPHA CHANGE 1838 -- persist snapshot directory mutations -- */

/* FINISH ALPHA CHANGE 1690 -- build independent segmented and checkpoint snapshots -- */
