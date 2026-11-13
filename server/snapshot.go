/* START ALPHA CHANGE 1701 -- activate complete snapshot chain-data artifacts -- */
package server

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	segmentedstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/segmented"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"

	/* START ALPHA CHANGE 3463 -- enforce snapshot page-size compatibility before activation -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3463 -- enforce snapshot page-size compatibility before activation -- */
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	"github.com/innovativebioresearch/innbc-chain/types"
)

const (
	snapshotActivationStageName  = ".snapshot-activation"
	snapshotActivationBackupName = "chain-data.snapshot-backup"
	/* START ALPHA CHANGE 1959 -- record snapshot activation transactions durably -- */
	snapshotActivationMarkerName = ".snapshot-activation-in-progress"
	/* FINISH ALPHA CHANGE 1959 -- record snapshot activation transactions durably -- */
)

type snapshotBootstrapPolicy struct {
	attempt      bool
	required     bool
	expectedKind snapshotbuilder.Kind
}

/* START ALPHA CHANGE 1708 -- distinguish usable local history from genesis-only storage -- */
func snapshotPolicy(
	history *HistoryConfig,
	hasUsableLocalChain bool,
) (snapshotBootstrapPolicy, error) {
	if history == nil {
		return snapshotBootstrapPolicy{}, errors.New("history configuration is missing")
	}

	switch history.Mode {
	case "recent":
		return snapshotBootstrapPolicy{
			attempt:      true,
			required:     history.SnapshotBootstrapRequired && !hasUsableLocalChain,
			expectedKind: snapshotbuilder.KindSegmented,
		}, nil
	case "full":
		if hasUsableLocalChain || !history.SnapshotBootstrapRequired {
			return snapshotBootstrapPolicy{
				expectedKind: snapshotbuilder.KindCheckpoint,
			}, nil
		}

		return snapshotBootstrapPolicy{
			attempt:      true,
			required:     true,
			expectedKind: snapshotbuilder.KindCheckpoint,
		}, nil
	default:
		return snapshotBootstrapPolicy{}, fmt.Errorf(
			"unsupported history mode %q",
			history.Mode,
		)
	}
}

/* FINISH ALPHA CHANGE 1708 -- distinguish usable local history from genesis-only storage -- */

func (s *Server) activateConfiguredSnapshot() (*snapshotbuilder.Manifest, error) {
	if s.config == nil || s.config.History == nil {
		return nil, errors.New("snapshot activation requires history configuration")
	}

	/* START ALPHA CHANGE 1709 -- log in-memory snapshot startup decisions -- */
	if s.config.DataDir == "" {
		if s.config.History.SnapshotBootstrapRequired {
			err := errors.New(
				"snapshot bootstrap cannot be required with in-memory storage",
			)

			s.logger.Error(
				"required snapshot bootstrap failed; node startup will stop",
				"historyMode", s.config.History.Mode,
				"err", err,
			)

			return nil, err
		}

		s.logger.Info(
			"snapshot bootstrap disabled for in-memory node startup",
			"historyMode", s.config.History.Mode,
		)

		return nil, nil
	}
	/* FINISH ALPHA CHANGE 1709 -- log in-memory snapshot startup decisions -- */

	if err := os.MkdirAll(s.config.DataDir, 0770); err != nil {
		return nil, fmt.Errorf("failed to create node data directory: %w", err)
	}

	if err := s.recoverInterruptedSnapshotActivation(); err != nil {
		return nil, fmt.Errorf(
			"failed to recover interrupted snapshot activation: %w",
			err,
		)
	}

	/* START ALPHA CHANGE 1710 -- log the resolved snapshot startup mode -- */
	/* START ALPHA CHANGE 3211 -- retain the validated local genesis identity during snapshot selection -- */
	localHead, hasUsableLocalChain, localGenesisHash, err := s.localBlockchainHead()
	/* FINISH ALPHA CHANGE 3211 -- retain the validated local genesis identity during snapshot selection -- */
	if err != nil {
		s.logger.Error(
			"snapshot startup policy resolution failed; node startup will stop",
			"historyMode", s.config.History.Mode,
			"err", err,
		)

		return nil, err
	}

	policy, err := snapshotPolicy(
		s.config.History,
		hasUsableLocalChain,
	)
	if err != nil {
		s.logger.Error(
			"snapshot startup policy resolution failed; node startup will stop",
			"historyMode", s.config.History.Mode,
			"err", err,
		)

		return nil, err
	}

	s.logger.Info(
		"snapshot startup mode selected",
		"historyMode", s.config.History.Mode,
		"configuredRequired", s.config.History.SnapshotBootstrapRequired,
		"hasUsableLocalChain", hasUsableLocalChain,
		"localHead", localHead,
		"attemptSnapshot", policy.attempt,
		"snapshotRequired", policy.required,
		"expectedKind", policy.expectedKind,
	)

	if !policy.attempt {
		if s.config.History.Mode == "full" &&
			s.config.History.SnapshotBootstrapRequired &&
			hasUsableLocalChain {
			s.logger.Warn(
				"checkpoint bootstrap requested but usable local chain exists; preserving local chain data",
				"localHead", localHead,
			)
		} else {
			s.logger.Info(
				"snapshot bootstrap not requested for this startup mode",
				"historyMode", s.config.History.Mode,
				"localHead", localHead,
			)
		}

		return nil, nil
	}

	s.logger.Info(
		"snapshot bootstrap attempt started",
		"historyMode", s.config.History.Mode,
		"required", policy.required,
		"expectedKind", policy.expectedKind,
		"sourceDir", strings.TrimSpace(
			s.config.History.SnapshotSourceDir,
		),
	)
	/* FINISH ALPHA CHANGE 1710 -- log the resolved snapshot startup mode -- */

	sourceRoot := strings.TrimSpace(s.config.History.SnapshotSourceDir)
	if sourceRoot == "" {
		return s.handleSnapshotBootstrapFailure(
			policy.required,
			errors.New("snapshot source directory is not configured"),
		)
	}

	latest, err := snapshotbuilder.LoadLatest(
		filepath.Join(sourceRoot, "latest.json"),
	)
	if err != nil {
		return s.handleSnapshotBootstrapFailure(
			policy.required,
			fmt.Errorf("failed to load latest snapshot pointer: %w", err),
		)
	}

	if latest.SnapshotKind != policy.expectedKind {
		return s.handleSnapshotBootstrapFailure(
			policy.required,
			fmt.Errorf(
				"snapshot kind mismatch: history mode %q requires %q, got %q",
				s.config.History.Mode,
				policy.expectedKind,
				latest.SnapshotKind,
			),
		)
	}

	sourceArtifact := filepath.Join(sourceRoot, latest.SnapshotDir)
	sourceManifest, err := snapshotbuilder.LoadManifest(
		filepath.Join(sourceArtifact, "manifest.json"),
	)
	if err != nil {
		return s.handleSnapshotBootstrapFailure(
			policy.required,
			fmt.Errorf("failed to load snapshot manifest: %w", err),
		)
	}

	if err := snapshotbuilder.ValidateLatestManifest(
		latest,
		sourceManifest,
	); err != nil {
		return s.handleSnapshotBootstrapFailure(
			policy.required,
			fmt.Errorf("snapshot pointer validation failed: %w", err),
		)
	}

	if err := s.validateSnapshotManifestForNode(
		sourceManifest,
		policy.expectedKind,
	); err != nil {
		return s.handleSnapshotBootstrapFailure(policy.required, err)
	}

	/* START ALPHA CHANGE 3212 -- require snapshot genesis identity to match initialized local storage before staging -- */
	if localGenesisHash != nil && sourceManifest.GenesisHash != *localGenesisHash {
		return s.handleSnapshotBootstrapFailure(
			policy.required,
			fmt.Errorf(
				"snapshot genesis hash %s does not match existing local genesis hash %s",
				sourceManifest.GenesisHash,
				*localGenesisHash,
			),
		)
	}
	/* FINISH ALPHA CHANGE 3212 -- require snapshot genesis identity to match initialized local storage before staging -- */

	/* START ALPHA CHANGE 1750 -- use usable-local-chain classification consistently -- */
	if hasUsableLocalChain && sourceManifest.BlockNumber <= localHead {
		s.logger.Info(
			"configured snapshot is not newer than local chain",
			"snapshotBlock", sourceManifest.BlockNumber,
			"localHead", localHead,
		)

		return nil, nil
	}
	/* FINISH ALPHA CHANGE 1750 -- use usable-local-chain classification consistently -- */

	/* START ALPHA CHANGE 3463 -- reject incompatible source snapshots before copying -- */
	if err := mdbxconfig.CheckChainData(
		datadir.ChainData(sourceArtifact),
		s.config.History.MDBXPageSizeBytes,
	); err != nil {
		return s.handleSnapshotBootstrapFailure(policy.required, err)
	}
	/* FINISH ALPHA CHANGE 3463 -- reject incompatible source snapshots before copying -- */

	stageRoot := filepath.Join(s.config.DataDir, snapshotActivationStageName)
	if err := os.RemoveAll(stageRoot); err != nil {
		return nil, fmt.Errorf("failed to clear snapshot activation staging: %w", err)
	}

	/* START ALPHA CHANGE 1711 -- remove incomplete snapshot staging immediately -- */
	if err := copySnapshotDirectory(sourceArtifact, stageRoot); err != nil {
		stageErr := fmt.Errorf(
			"failed to stage snapshot artifact: %w",
			err,
		)

		if cleanupErr := os.RemoveAll(stageRoot); cleanupErr != nil {
			stageErr = errors.Join(
				stageErr,
				fmt.Errorf(
					"failed to remove incomplete snapshot staging: %w",
					cleanupErr,
				),
			)
		}

		return s.handleSnapshotBootstrapFailure(
			policy.required,
			stageErr,
		)
	}
	/* FINISH ALPHA CHANGE 1711 -- remove incomplete snapshot staging immediately -- */

	/* START ALPHA CHANGE 1785 -- validate staged storage with the node-wide MDBX size -- */
	/* START ALPHA CHANGE 3463 -- verify staged databases against the receiving node setting -- */
	stagedManifest, err := snapshotbuilder.ValidatePublishedArtifact(
		stageRoot,
		policy.expectedKind,
		s.config.History.MDBXMapSizeGB,
		s.logger,
		s.config.History.MDBXPageSizeBytes,
	)
	/* FINISH ALPHA CHANGE 3463 -- verify staged databases against the receiving node setting -- */
	/* FINISH ALPHA CHANGE 1785 -- validate staged storage with the node-wide MDBX size -- */
	if err != nil {
		validationErr := cleanupSnapshotActivationStage(
			stageRoot,
			fmt.Errorf("staged snapshot validation failed: %w", err),
		)

		return s.handleSnapshotBootstrapFailure(
			policy.required,
			validationErr,
		)
	}

	if *stagedManifest != *sourceManifest {
		validationErr := cleanupSnapshotActivationStage(
			stageRoot,
			errors.New("staged snapshot manifest changed during staging"),
		)

		return s.handleSnapshotBootstrapFailure(
			policy.required,
			validationErr,
		)
	}

	if err := s.activateStagedChainData(stageRoot); err != nil {
		activationErr := cleanupSnapshotActivationStage(
			stageRoot,
			fmt.Errorf("failed to activate staged snapshot: %w", err),
		)

		s.logger.Error(
			"snapshot activation failed; node startup will stop",
			"historyMode", s.config.History.Mode,
			"err", activationErr,
		)

		return nil, activationErr
	}

	s.logger.Info(
		"complete snapshot chain data activated",
		"kind", stagedManifest.SnapshotKind,
		"block", stagedManifest.BlockNumber,
		"hash", stagedManifest.BlockHash.String(),
		"stateRoot", stagedManifest.StateRoot.String(),
		"retainedFrom", stagedManifest.RetainedFrom,
		"retainedTo", stagedManifest.RetainedTo,
	)

	return stagedManifest, nil
}

/* START ALPHA CHANGE 1712 -- log required and optional snapshot failures explicitly -- */
func (s *Server) handleSnapshotBootstrapFailure(
	required bool,
	activationErr error,
) (*snapshotbuilder.Manifest, error) {
	/* START ALPHA CHANGE 3463 -- never silently bypass a page-size mismatch -- */
	if required || errors.Is(activationErr, mdbxconfig.ErrPageSizeMismatch) {
		/* FINISH ALPHA CHANGE 3463 -- never silently bypass a page-size mismatch -- */
		s.logger.Error(
			"required snapshot bootstrap failed; node startup will stop",
			"historyMode", s.config.History.Mode,
			"err", activationErr,
		)

		return nil, activationErr
	}

	s.logger.Warn(
		"optional snapshot bootstrap unavailable; continuing with normal synchronization",
		"historyMode", s.config.History.Mode,
		"err", activationErr,
	)

	return nil, nil
}

/* FINISH ALPHA CHANGE 1712 -- log required and optional snapshot failures explicitly -- */

/* START ALPHA CHANGE 1753 -- clean failed snapshot activation staging with complete error reporting -- */
func cleanupSnapshotActivationStage(
	stageRoot string,
	activationErr error,
) error {
	if cleanupErr := os.RemoveAll(stageRoot); cleanupErr != nil {
		return errors.Join(
			activationErr,
			fmt.Errorf(
				"failed to remove snapshot activation staging: %w",
				cleanupErr,
			),
		)
	}

	return activationErr
}

/* FINISH ALPHA CHANGE 1753 -- clean failed snapshot activation staging with complete error reporting -- */

func (s *Server) validateSnapshotManifestForNode(
	manifest *snapshotbuilder.Manifest,
	expectedKind snapshotbuilder.Kind,
) error {
	if manifest.SnapshotKind != expectedKind {
		return fmt.Errorf(
			"snapshot kind mismatch: expected %q, got %q",
			expectedKind,
			manifest.SnapshotKind,
		)
	}

	/* START ALPHA CHANGE 1815 -- validate snapshot against allocation-derived genesis -- */
	genesis := s.config.Chain.Genesis.GenesisHeader()
	genesis.StateRoot = manifest.GenesisStateRoot
	genesis.ComputeHash()

	if manifest.GenesisHash != genesis.Hash {
		return fmt.Errorf(
			"snapshot genesis hash mismatch: expected %s, got %s",
			genesis.Hash,
			manifest.GenesisHash,
		)
	}
	/* FINISH ALPHA CHANGE 1815 -- validate snapshot against allocation-derived genesis -- */

	if expectedKind == snapshotbuilder.KindSegmented {
		if manifest.RetentionBlocks != s.config.History.RetentionBlocks {
			return fmt.Errorf(
				"snapshot retention mismatch: expected %d, got %d",
				s.config.History.RetentionBlocks,
				manifest.RetentionBlocks,
			)
		}

		if manifest.SegmentBlocks != s.config.History.SegmentBlocks {
			return fmt.Errorf(
				"snapshot segment size mismatch: expected %d, got %d",
				s.config.History.SegmentBlocks,
				manifest.SegmentBlocks,
			)
		}
	}

	return nil
}

/* START ALPHA CHANGE 3187 -- validate existing local storage before snapshot eligibility and age comparisons -- */
/* START ALPHA CHANGE 3211 -- retain validated genesis identity without additional storage reads -- */
func (s *Server) localBlockchainHead() (uint64, bool, *types.Hash, error) {
	if s.config == nil || s.config.History == nil {
		return 0, false, nil, errors.New("local blockchain inspection requires history configuration")
	}

	blockchainPath := datadir.Blockchain(s.config.DataDir)

	entries, err := os.ReadDir(blockchainPath)
	if errors.Is(err, os.ErrNotExist) {
		return s.emptyLocalBlockchainHead()
	}
	if err != nil {
		return 0, false, nil, fmt.Errorf(
			"failed to inspect local blockchain storage: %w",
			err,
		)
	}
	if len(entries) == 0 {
		return s.emptyLocalBlockchainHead()
	}

	var database storage.Storage

	switch s.config.History.Mode {
	case "recent":
		database, err = segmentedstorage.NewStorageReadOnly(
			blockchainPath,
			s.logger,
			s.config.History.SegmentBlocks,
			s.config.History.RetentionBlocks,
		)
	case "full":
		database, err = mdbxstorage.NewMDBXStorageReadOnly(
			blockchainPath,
			s.logger,
		)
	default:
		return 0, false, nil, fmt.Errorf(
			"unsupported history mode %q",
			s.config.History.Mode,
		)
	}
	if err != nil {
		return 0, false, nil, fmt.Errorf(
			"failed to inspect local blockchain head: %w",
			err,
		)
	}

	return inspectSnapshotLocalHead(database)
}

// An absent or empty blockchain path is fresh only when other chain-data
// components contain no files or nested layout. Preserve incomplete existing
// chain data instead of allowing a snapshot to replace it automatically.
func (s *Server) emptyLocalBlockchainHead() (uint64, bool, *types.Hash, error) {
	chainData := datadir.ChainData(s.config.DataDir)

	entries, err := os.ReadDir(chainData)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil, nil
	}
	if err != nil {
		return 0, false, nil, fmt.Errorf("failed to inspect local chain data: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			return 0, false, nil, fmt.Errorf(
				"local blockchain storage is absent or empty but chain data contains %q",
				entry.Name(),
			)
		}

		children, err := os.ReadDir(filepath.Join(chainData, entry.Name()))
		if err != nil {
			return 0, false, nil, fmt.Errorf(
				"failed to inspect local chain-data component %q: %w",
				entry.Name(),
				err,
			)
		}
		if len(children) != 0 {
			return 0, false, nil, fmt.Errorf(
				"local blockchain storage is absent or empty but %q contains data",
				entry.Name(),
			)
		}
	}

	return 0, false, nil, nil
}

// Consume and close the inspection handle before returning a classification.
// A local read or close failure must propagate directly to the startup caller.
// A non-nil genesis hash also anchors genesis-only initialized storage.
func inspectSnapshotLocalHead(database storage.Storage) (uint64, bool, *types.Hash, error) {
	startup, readErr := database.ReadStartupState()
	if readErr != nil {
		readErr = fmt.Errorf(
			"failed to validate local blockchain startup state: %w",
			readErr,
		)
	}

	if closeErr := database.Close(); closeErr != nil {
		readErr = errors.Join(
			readErr,
			fmt.Errorf("failed to close inspected blockchain storage: %w", closeErr),
		)
	}
	if readErr != nil {
		return 0, false, nil, readErr
	}

	if startup.Empty {
		if startup.Head != nil || startup.HasGenesis {
			return 0, false, nil, errors.New("inconsistent empty local blockchain startup state")
		}
		return 0, false, nil, nil
	}

	if startup.Head == nil {
		return 0, false, nil, errors.New("local blockchain startup state has no validated head")
	}
	if !startup.HasGenesis {
		return 0, false, nil, errors.New("local blockchain startup state has no genesis mapping")
	}

	return startup.Head.Number, startup.Head.Number > 0, &startup.GenesisHash, nil
}

/* FINISH ALPHA CHANGE 3211 -- retain validated genesis identity without additional storage reads -- */
/* FINISH ALPHA CHANGE 3187 -- validate existing local storage before snapshot eligibility and age comparisons -- */

/* START ALPHA CHANGE 1960 -- persist snapshot activation directory mutations -- */
func syncSnapshotDirectory(path string) error {
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

func syncSnapshotDirectoryTree(root string) error {
	directories := make([]string, 0)

	if err := filepath.WalkDir(
		root,
		func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if entry.IsDir() {
				directories = append(directories, path)
			}

			return nil
		},
	); err != nil {
		return err
	}

	for index := len(directories) - 1; index >= 0; index-- {
		if err := syncSnapshotDirectory(directories[index]); err != nil {
			return fmt.Errorf(
				"failed to sync snapshot directory %s: %w",
				directories[index],
				err,
			)
		}
	}

	return nil
}

func removeSnapshotPathAndSync(parentPath, targetPath string) error {
	if err := os.RemoveAll(targetPath); err != nil {
		return err
	}

	return syncSnapshotDirectory(parentPath)
}

func createSnapshotActivationMarker(dataDir string) error {
	markerPath := filepath.Join(dataDir, snapshotActivationMarkerName)

	marker, err := os.OpenFile(
		markerPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0600,
	)
	if err != nil {
		return err
	}

	if err := marker.Sync(); err != nil {
		_ = marker.Close()

		return err
	}

	if err := marker.Close(); err != nil {
		return err
	}

	return syncSnapshotDirectory(dataDir)
}

/* FINISH ALPHA CHANGE 1960 -- persist snapshot activation directory mutations -- */

/* START ALPHA CHANGE 1961 -- make snapshot chain-data activation power-loss durable -- */
func (s *Server) activateStagedChainData(stageRoot string) error {
	stagedChainData := datadir.ChainData(stageRoot)
	liveChainData := datadir.ChainData(s.config.DataDir)
	backupChainData := filepath.Join(
		s.config.DataDir,
		snapshotActivationBackupName,
	)
	activationMarker := filepath.Join(
		s.config.DataDir,
		snapshotActivationMarkerName,
	)

	stagedInfo, err := os.Stat(stagedChainData)
	if err != nil {
		return fmt.Errorf("failed to inspect staged chain data: %w", err)
	}
	if !stagedInfo.IsDir() {
		return errors.New("staged chain-data path is not a directory")
	}

	// Snapshot files are synced while they are copied. Sync every staging
	// directory from the leaves upward before publishing the tree.
	if err := syncSnapshotDirectoryTree(stageRoot); err != nil {
		return fmt.Errorf(
			"failed to persist staged snapshot directory tree: %w",
			err,
		)
	}

	if err := removeSnapshotPathAndSync(
		s.config.DataDir,
		backupChainData,
	); err != nil {
		return fmt.Errorf("failed to clear old chain-data backup: %w", err)
	}

	if err := createSnapshotActivationMarker(s.config.DataDir); err != nil {
		return fmt.Errorf(
			"failed to create snapshot activation marker: %w",
			err,
		)
	}

	hadLiveChainData := false
	if liveInfo, err := os.Stat(liveChainData); err == nil {
		if !liveInfo.IsDir() {
			return errors.New("live chain-data path is not a directory")
		}

		if err := os.Rename(liveChainData, backupChainData); err != nil {
			return fmt.Errorf("failed to preserve live chain data: %w", err)
		}

		if err := syncSnapshotDirectory(s.config.DataDir); err != nil {
			return fmt.Errorf(
				"failed to persist preserved live chain data: %w",
				err,
			)
		}

		hadLiveChainData = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect live chain data: %w", err)
	}

	if err := os.Rename(stagedChainData, liveChainData); err != nil {
		activationErr := fmt.Errorf(
			"failed to activate staged chain data: %w",
			err,
		)

		if hadLiveChainData {
			if restoreErr := os.Rename(
				backupChainData,
				liveChainData,
			); restoreErr != nil {
				return errors.Join(
					activationErr,
					fmt.Errorf(
						"failed to restore previous chain data: %w",
						restoreErr,
					),
				)
			}

			if restoreSyncErr := syncSnapshotDirectory(
				s.config.DataDir,
			); restoreSyncErr != nil {
				return errors.Join(
					activationErr,
					fmt.Errorf(
						"failed to persist restored chain data: %w",
						restoreSyncErr,
					),
				)
			}
		}

		if markerErr := removeSnapshotPathAndSync(
			s.config.DataDir,
			activationMarker,
		); markerErr != nil {
			return errors.Join(
				activationErr,
				fmt.Errorf(
					"failed to clear snapshot activation marker: %w",
					markerErr,
				),
			)
		}

		return activationErr
	}

	// The rename changes both the source and destination parent directories.
	if err := syncSnapshotDirectory(stageRoot); err != nil {
		return fmt.Errorf(
			"failed to persist removal of staged chain data: %w",
			err,
		)
	}

	if err := syncSnapshotDirectory(s.config.DataDir); err != nil {
		return fmt.Errorf(
			"failed to persist activated chain data: %w",
			err,
		)
	}

	if err := removeSnapshotPathAndSync(
		s.config.DataDir,
		stageRoot,
	); err != nil {
		s.logger.Warn(
			"snapshot activated but staging directory cleanup was not persisted",
			"path", stageRoot,
			"err", err,
		)
	}

	cleanupComplete := true

	if hadLiveChainData {
		if err := removeSnapshotPathAndSync(
			s.config.DataDir,
			backupChainData,
		); err != nil {
			cleanupComplete = false
			s.logger.Warn(
				"snapshot activated but previous chain data cleanup was not persisted",
				"path", backupChainData,
				"err", err,
			)
		}
	}

	// Keep the marker when backup cleanup was not durable. Recovery can then
	// distinguish this known activation from an unexplained live-plus-backup state.
	if cleanupComplete {
		if err := removeSnapshotPathAndSync(
			s.config.DataDir,
			activationMarker,
		); err != nil {
			s.logger.Warn(
				"snapshot activated but activation marker cleanup was not persisted",
				"path", activationMarker,
				"err", err,
			)
		}
	}

	return nil
}

/* FINISH ALPHA CHANGE 1961 -- make snapshot chain-data activation power-loss durable -- */

/* START ALPHA CHANGE 1962 -- recover snapshot activation using a durable transaction marker -- */
func (s *Server) recoverInterruptedSnapshotActivation() error {
	stageRoot := filepath.Join(
		s.config.DataDir,
		snapshotActivationStageName,
	)
	liveChainData := datadir.ChainData(s.config.DataDir)
	backupChainData := filepath.Join(
		s.config.DataDir,
		snapshotActivationBackupName,
	)
	activationMarker := filepath.Join(
		s.config.DataDir,
		snapshotActivationMarkerName,
	)

	markerExists := false
	if markerInfo, err := os.Stat(activationMarker); err == nil {
		if !markerInfo.Mode().IsRegular() {
			return errors.New("snapshot activation marker is not a regular file")
		}

		markerExists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"failed to inspect snapshot activation marker: %w",
			err,
		)
	}

	backupInfo, backupErr := os.Stat(backupChainData)
	backupExists := backupErr == nil
	if backupErr != nil && !errors.Is(backupErr, os.ErrNotExist) {
		return fmt.Errorf(
			"failed to inspect chain-data backup: %w",
			backupErr,
		)
	}
	if backupExists && !backupInfo.IsDir() {
		return errors.New("chain-data backup path is not a directory")
	}

	liveInfo, liveErr := os.Stat(liveChainData)
	liveExists := liveErr == nil
	if liveErr != nil && !errors.Is(liveErr, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect live chain data: %w", liveErr)
	}
	if liveExists && !liveInfo.IsDir() {
		return errors.New("live chain-data path is not a directory")
	}

	// Without the durable marker, two competing trees are ambiguous.
	// Preserve both and refuse to guess which one should be deleted.
	if liveExists && backupExists && !markerExists {
		return errors.New(
			"live and backup chain data both exist without a snapshot activation marker",
		)
	}

	/* START ALPHA CHANGE 3499 -- resolve omitted recovery page settings without dereferencing nil history -- */
	var configuredPageSize uint64
	if s.config.History != nil {
		configuredPageSize = s.config.History.MDBXPageSizeBytes
	}
	pageSize, err := mdbxconfig.Resolve(configuredPageSize)
	if err != nil {
		return err
	}

	switch {
	case liveExists:
		if err := mdbxconfig.CheckChainData(
			liveChainData, uint64(pageSize),
		); err != nil {
			return err
		}
	case backupExists:
		if err := mdbxconfig.CheckChainData(
			backupChainData, uint64(pageSize),
		); err != nil {
			return err
		}
	}
	/* FINISH ALPHA CHANGE 3499 -- resolve omitted recovery page settings without dereferencing nil history -- */

	if stageInfo, err := os.Stat(stageRoot); err == nil {
		if !stageInfo.IsDir() {
			return errors.New("snapshot activation staging path is not a directory")
		}

		if err := removeSnapshotPathAndSync(
			s.config.DataDir,
			stageRoot,
		); err != nil {
			return fmt.Errorf(
				"failed to durably remove interrupted snapshot staging: %w",
				err,
			)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"failed to inspect snapshot activation staging: %w",
			err,
		)
	}

	switch {
	case backupExists && !liveExists:
		if err := os.Rename(backupChainData, liveChainData); err != nil {
			return fmt.Errorf(
				"failed to restore interrupted chain-data activation: %w",
				err,
			)
		}

		if err := syncSnapshotDirectory(s.config.DataDir); err != nil {
			return fmt.Errorf(
				"failed to persist restored chain data: %w",
				err,
			)
		}

		if markerExists {
			if err := removeSnapshotPathAndSync(
				s.config.DataDir,
				activationMarker,
			); err != nil {
				return fmt.Errorf(
					"failed to clear recovered activation marker: %w",
					err,
				)
			}
		}

		s.logger.Warn(
			"restored chain data after interrupted snapshot activation",
			"path", liveChainData,
		)

		return nil

	case backupExists && liveExists:
		if err := removeSnapshotPathAndSync(
			s.config.DataDir,
			backupChainData,
		); err != nil {
			return fmt.Errorf(
				"failed to durably remove completed activation backup: %w",
				err,
			)
		}

		if err := removeSnapshotPathAndSync(
			s.config.DataDir,
			activationMarker,
		); err != nil {
			return fmt.Errorf(
				"failed to clear completed activation marker: %w",
				err,
			)
		}

		return nil

	default:
		if markerExists {
			if err := removeSnapshotPathAndSync(
				s.config.DataDir,
				activationMarker,
			); err != nil {
				return fmt.Errorf(
					"failed to clear stale snapshot activation marker: %w",
					err,
				)
			}
		}

		return nil
	}
}

/* FINISH ALPHA CHANGE 1962 -- recover snapshot activation using a durable transaction marker -- */

func copySnapshotDirectory(source, target string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !sourceInfo.IsDir() {
		return errors.New("snapshot artifact path is not a directory")
	}

	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("snapshot staging path already exists: %s", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Mkdir(target, sourceInfo.Mode().Perm()); err != nil {
		return err
	}

	return filepath.WalkDir(
		source,
		func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if path == source {
				return nil
			}

			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf(
					"snapshot artifact contains symbolic link %s",
					path,
				)
			}

			relativePath, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}

			targetPath := filepath.Join(target, relativePath)

			if entry.IsDir() {
				info, err := entry.Info()
				if err != nil {
					return err
				}

				return os.Mkdir(targetPath, info.Mode().Perm())
			}

			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf(
					"snapshot artifact contains unsupported file %s",
					path,
				)
			}

			return copySnapshotFile(path, targetPath, info.Mode().Perm())
		},
	)
}

func copySnapshotFile(source, target string, mode os.FileMode) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close() //nolint:errcheck

	targetFile, err := os.OpenFile(
		target,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		mode,
	)
	if err != nil {
		return err
	}

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		_ = targetFile.Close()

		return err
	}

	if err := targetFile.Sync(); err != nil {
		_ = targetFile.Close()

		return err
	}

	return targetFile.Close()
}

func storedGenesisStateRoot(
	database storage.Storage,
) (types.Hash, bool, error) {
	if _, ok := database.ReadHeadNumber(); !ok {
		return types.ZeroHash, false, nil
	}

	genesisHash, ok := database.ReadCanonicalHash(0)
	if !ok {
		return types.ZeroHash, false, errors.New(
			"existing blockchain is missing its genesis mapping",
		)
	}

	genesisHeader, err := database.ReadHeader(genesisHash)
	if err != nil {
		return types.ZeroHash, false, fmt.Errorf(
			"failed to read existing genesis header: %w",
			err,
		)
	}

	return genesisHeader.StateRoot, true, nil
}

/* FINISH ALPHA CHANGE 1701 -- activate complete snapshot chain-data artifacts -- */
