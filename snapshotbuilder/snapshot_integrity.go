/* START ALPHA CHANGE 2103 -- verify physical consensus commitments and reuse immutable recovery artifacts -- */
package snapshotbuilder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-hclog"
)

func snapshotConsensusFileIdentity(artifactDir string) (SnapshotFile, error) {
	path := filepath.Join(artifactDir, filepath.FromSlash(snapshotConsensusDataPath))

	info, err := os.Lstat(path)
	if err != nil {
		return SnapshotFile{}, err
	}
	if !info.Mode().IsRegular() {
		return SnapshotFile{}, errors.New("snapshot consensus data is not a regular file")
	}

	identity, err := archiveFileIdentity(path)
	if err != nil {
		return SnapshotFile{}, err
	}

	return SnapshotFile{
		Path:   snapshotConsensusDataPath,
		SHA256: identity.SHA256,
		Bytes:  identity.Bytes,
	}, nil
}

func verifySnapshotConsensusFile(artifactDir string, expected SnapshotFile) error {
	if expected.Path != snapshotConsensusDataPath {
		return errors.New("snapshot consensus data path is invalid")
	}
	if expected.Bytes == 0 {
		return errors.New("snapshot consensus data size is invalid")
	}
	if err := validateSHA256(expected.SHA256); err != nil {
		return fmt.Errorf("snapshot consensus data checksum is invalid: %w", err)
	}

	actual, err := snapshotConsensusFileIdentity(artifactDir)
	if err != nil {
		return err
	}
	if actual.Bytes != expected.Bytes {
		return fmt.Errorf(
			"snapshot consensus data size mismatch: expected %d, got %d",
			expected.Bytes,
			actual.Bytes,
		)
	}
	if actual.SHA256 != expected.SHA256 {
		return errors.New("snapshot consensus data checksum mismatch")
	}

	return nil
}

// reusePublishedRecovery runs under the existing archive/checkpoint locks.
// A nil result means no published artifact exists.
//
// The existing file commitment is verified against the existing artifact.
// A fresh MDBX export is not required to have identical physical bytes.
func reusePublishedRecovery(
	kindRoot string,
	requested *Manifest,
	mdbxMapSize int,
	logger hclog.Logger,
	validateExisting func(string, *Manifest) error,
	pageSizes ...uint64,
) (*Result, error) {
	/* START ALPHA CHANGE 3465 -- retain expected geometry when reusing recovery publications -- */
	/* FINISH ALPHA CHANGE 3465 -- retain expected geometry when reusing recovery publications -- */
	name := snapshotDirectoryName(requested.BlockNumber, requested.BlockHash)
	directory := filepath.Join(kindRoot, name)

	if _, err := os.Lstat(directory); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if err := requireArchiveDirectory(directory); err != nil {
		return nil, err
	}
	if validateExisting == nil {
		return nil, errors.New("existing recovery validation is unavailable")
	}

	manifestPath := filepath.Join(directory, manifestFileName)
	var existing Manifest
	if err := readStrictJSON(
		manifestPath,
		recoveryCheckpointManifestLimit,
		&existing,
	); err != nil {
		return nil, err
	}
	if err := ValidateManifest(&existing); err != nil {
		return nil, err
	}

	// Compare all logical manifest fields while retaining the existing
	// physical consensus commitment for actual artifact verification.
	expected := *requested
	expected.ConsensusData = existing.ConsensusData
	if existing != expected {
		return nil, errors.New("existing checkpoint differs from requested recovery checkpoint")
	}

	if err := validateExisting(directory, &existing); err != nil {
		return nil, fmt.Errorf("existing recovery relationship: %w", err)
	}
	/* START ALPHA CHANGE 3465 -- reject recovery reuse with incompatible database pages -- */
	if err := validateArtifact(directory, &existing, mdbxMapSize, logger, pageSizes...); err != nil {
		/* FINISH ALPHA CHANGE 3465 -- reject recovery reuse with incompatible database pages -- */
		return nil, fmt.Errorf("existing recovery checkpoint validation: %w", err)
	}

	// Preserve the prior behavior of refusing an invalid current pointer.
	if _, err := loadOptionalLatest(filepath.Join(kindRoot, latestFileName)); err != nil {
		return nil, err
	}

	if err := syncArchiveTree(directory); err != nil {
		return nil, fmt.Errorf("synchronize existing recovery checkpoint: %w", err)
	}
	if err := syncDirectory(kindRoot); err != nil {
		return nil, err
	}

	latestPath := filepath.Join(kindRoot, latestFileName)
	latest := &Latest{
		FormatVersion: existing.FormatVersion,
		SnapshotKind:  existing.SnapshotKind,
		BlockNumber:   existing.BlockNumber,
		BlockHash:     existing.BlockHash,
		StateRoot:     existing.StateRoot,
		SnapshotDir:   name,
	}
	if err := writeJSONAtomic(latestPath, latest); err != nil {
		return nil, fmt.Errorf("failed to publish latest snapshot pointer: %w", err)
	}
	if err := syncDirectory(kindRoot); err != nil {
		return nil, fmt.Errorf("failed to persist latest snapshot pointer: %w", err)
	}

	return &Result{
		Kind:         existing.SnapshotKind,
		BlockNumber:  existing.BlockNumber,
		BlockHash:    existing.BlockHash,
		StateRoot:    existing.StateRoot,
		RetainedFrom: existing.RetainedFrom,
		RetainedTo:   existing.RetainedTo,
		SnapshotDir:  directory,
		LatestPath:   latestPath,
	}, nil
}

/* FINISH ALPHA CHANGE 2103 -- verify physical consensus commitments and reuse immutable recovery artifacts -- */
