/* START ALPHA CHANGE 2074 -- describe and validate recovery relationships inside checkpoints -- */
package snapshotbuilder

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/innovativebioresearch/innbc-chain/types"
)

const (
	RecoverySetFormatVersion uint64 = 1

	recoverySetCoverageMode         = "shared-boundary-overlap"
	recoverySetFileName             = "recovery-set.json"
	recoverySetSizeLimit            = 128 * 1024
	recoveryCheckpointManifestLimit = 128 * 1024
)

type RecoverySetObject struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
}

type RecoverySetBoundary struct {
	BlockNumber uint64     `json:"blockNumber"`
	BlockHash   types.Hash `json:"blockHash"`
}

type RecoverySetArchive struct {
	Manifest RecoverySetObject `json:"manifest"`

	RequiredThrough uint64 `json:"requiredThrough"`
	ActualThrough   uint64 `json:"actualThrough"`
}

type RecoverySetCheckpoint struct {
	Manifest RecoverySetObject `json:"manifest"`

	CheckpointNumber uint64     `json:"checkpointNumber"`
	CheckpointHash   types.Hash `json:"checkpointHash"`
	StateRoot        types.Hash `json:"stateRoot"`

	RetainedFrom    uint64 `json:"retainedFrom"`
	RetainedTo      uint64 `json:"retainedTo"`
	RetentionBlocks uint64 `json:"retentionBlocks"`
}

type RecoverySet struct {
	FormatVersion uint64 `json:"formatVersion"`
	CoverageMode  string `json:"coverageMode"`

	ChainID     int64      `json:"chainID"`
	GenesisHash types.Hash `json:"genesisHash"`

	SharedBoundary RecoverySetBoundary   `json:"sharedBoundary"`
	Archive        RecoverySetArchive    `json:"archive"`
	Checkpoint     RecoverySetCheckpoint `json:"checkpoint"`
}

func validateRecoverySetObject(
	object RecoverySetObject,
	expectedPath string,
	sizeLimit uint64,
) error {
	if object.Path != expectedPath {
		return fmt.Errorf(
			"recovery object path must be %q, got %q",
			expectedPath,
			object.Path,
		)
	}
	if object.Bytes == 0 || object.Bytes > sizeLimit {
		return errors.New("recovery object size is outside supported limits")
	}

	return validateSHA256(object.SHA256)
}

func validateRecoverySetShape(recovery *RecoverySet) error {
	if recovery == nil {
		return errors.New("recovery set is nil")
	}
	if recovery.FormatVersion != RecoverySetFormatVersion {
		return errors.New("unsupported recovery-set format version")
	}
	if recovery.CoverageMode != recoverySetCoverageMode {
		return errors.New("unsupported recovery-set coverage mode")
	}
	if recovery.ChainID <= 0 || recovery.GenesisHash == types.ZeroHash {
		return errors.New("recovery set has invalid chain identity")
	}
	if recovery.SharedBoundary.BlockNumber == 0 ||
		recovery.SharedBoundary.BlockHash == types.ZeroHash {
		return errors.New("recovery set has invalid shared boundary")
	}

	if err := validateRecoverySetObject(
		recovery.Archive.Manifest,
		archiveManifestName(recovery.Archive.Manifest.SHA256),
		archiveManifestSizeLimit,
	); err != nil {
		return fmt.Errorf("recovery archive object: %w", err)
	}
	if err := validateRecoverySetObject(
		recovery.Checkpoint.Manifest,
		manifestFileName,
		recoveryCheckpointManifestLimit,
	); err != nil {
		return fmt.Errorf("recovery checkpoint object: %w", err)
	}

	checkpoint := recovery.Checkpoint
	boundary := recovery.SharedBoundary.BlockNumber

	if checkpoint.CheckpointNumber == 0 ||
		checkpoint.CheckpointHash == types.ZeroHash ||
		checkpoint.StateRoot == types.ZeroHash {
		return errors.New("recovery checkpoint identity is invalid")
	}
	if checkpoint.RetainedFrom != boundary ||
		checkpoint.RetainedTo != checkpoint.CheckpointNumber ||
		checkpoint.RetainedFrom > checkpoint.RetainedTo {
		return errors.New("recovery checkpoint range is invalid")
	}
	// RetainedFrom is positive, so this addition cannot overflow.
	if checkpoint.RetentionBlocks !=
		checkpoint.RetainedTo-checkpoint.RetainedFrom+1 {
		return errors.New("recovery checkpoint retention length is incorrect")
	}
	if recovery.Archive.RequiredThrough != boundary ||
		recovery.Archive.ActualThrough < boundary {
		return errors.New("recovery archive does not cover the required boundary")
	}

	return nil
}

func ValidateRecoverySet(
	recovery *RecoverySet,
	archive *ArchiveManifest,
	checkpoint *Manifest,
	archiveManifest ArchiveFile,
	checkpointManifest ArchiveFile,
) error {
	if err := validateRecoverySetShape(recovery); err != nil {
		return err
	}
	if err := ValidateArchiveManifest(archive); err != nil {
		return fmt.Errorf("recovery archive manifest: %w", err)
	}
	if err := ValidateManifest(checkpoint); err != nil {
		return fmt.Errorf("recovery checkpoint manifest: %w", err)
	}
	if checkpoint.SnapshotKind != KindCheckpoint {
		return errors.New("recovery set must reference a checkpoint snapshot")
	}

	if recovery.Archive.Manifest.Path != archiveManifest.Path ||
		recovery.Archive.Manifest.SHA256 != archiveManifest.SHA256 ||
		recovery.Archive.Manifest.Bytes != archiveManifest.Bytes {
		return errors.New("recovery archive manifest identity differs")
	}
	if checkpointManifest.Path != manifestFileName ||
		recovery.Checkpoint.Manifest.SHA256 != checkpointManifest.SHA256 ||
		recovery.Checkpoint.Manifest.Bytes != checkpointManifest.Bytes {
		return errors.New("recovery checkpoint manifest identity differs")
	}

	if recovery.ChainID != archive.ChainID ||
		recovery.GenesisHash != archive.GenesisHash ||
		recovery.GenesisHash != checkpoint.GenesisHash ||
		archive.GenesisStateRoot != checkpoint.GenesisStateRoot {
		return errors.New("recovery chain identity differs between artifacts")
	}
	if recovery.Archive.ActualThrough != archive.ArchivedThrough {
		return errors.New("recovery archive watermark differs from manifest")
	}

	actual := recovery.Checkpoint
	if actual.CheckpointNumber != checkpoint.BlockNumber ||
		actual.CheckpointHash != checkpoint.BlockHash ||
		actual.StateRoot != checkpoint.StateRoot ||
		actual.RetainedFrom != checkpoint.RetainedFrom ||
		actual.RetainedTo != checkpoint.RetainedTo ||
		actual.RetentionBlocks != checkpoint.RetentionBlocks {
		return errors.New("recovery checkpoint differs from its manifest")
	}

	for index := range archive.Segments {
		segment := &archive.Segments[index]
		if segment.To == recovery.SharedBoundary.BlockNumber {
			if segment.LastBlockHash != recovery.SharedBoundary.BlockHash {
				return errors.New("recovery boundary hash differs from archive manifest")
			}
			return nil
		}
	}

	return errors.New("recovery boundary is not a completed archive segment boundary")
}

// Read only regular files, with a bound on both the initial size and actual read.
func readBoundedRecoveryFile(path string, limit uint64) ([]byte, error) {
	if limit == 0 || limit > archiveManifestSizeLimit {
		return nil, errors.New("invalid recovery metadata read limit")
	}
	if _, err := archiveRegularFileSize(path, limit); err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("recovery metadata is not a regular file")
	}

	raw, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || uint64(len(raw)) > limit {
		return nil, errors.New("recovery metadata size is outside supported limits")
	}

	return raw, nil
}

func loadRecoverySet(path string) (*RecoverySet, error) {
	raw, err := readBoundedRecoveryFile(path, recoverySetSizeLimit)
	if err != nil {
		return nil, fmt.Errorf("read recovery set: %w", err)
	}

	var recovery RecoverySet
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&recovery); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("recovery document contains multiple JSON values")
	}
	if err := validateRecoverySetShape(&recovery); err != nil {
		return nil, err
	}

	return &recovery, nil
}

/* FINISH ALPHA CHANGE 2074 -- describe and validate recovery relationships inside checkpoints -- */
