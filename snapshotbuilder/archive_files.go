/* START ALPHA CHANGE 2066 -- provide strict crash-safe archive file operations -- */
package snapshotbuilder

/* START ALPHA CHANGE 2066 -- retain only required archive file imports -- */
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

/* FINISH ALPHA CHANGE 2066 -- retain only required archive file imports -- */

func archiveFileIdentity(path string) (ArchiveFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return ArchiveFile{}, err
	}
	defer file.Close() //nolint:errcheck

	info, err := file.Stat()
	if err != nil {
		return ArchiveFile{}, err
	}
	if !info.Mode().IsRegular() {
		return ArchiveFile{}, fmt.Errorf(
			"archive path is not a regular file: %s",
			path,
		)
	}

	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return ArchiveFile{}, err
	}

	return ArchiveFile{
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
		Bytes:  uint64(size),
	}, nil
}

func verifyArchiveFile(path string, expected ArchiveFile) error {
	actual, err := archiveFileIdentity(path)
	if err != nil {
		return err
	}

	if actual.Bytes != expected.Bytes {
		return fmt.Errorf(
			"archive file size mismatch for %s: expected %d, got %d",
			path,
			expected.Bytes,
			actual.Bytes,
		)
	}

	if actual.SHA256 != expected.SHA256 {
		return fmt.Errorf(
			"archive file checksum mismatch for %s",
			path,
		)
	}

	return nil
}

func validateSHA256(value string) error {
	if len(value) != sha256.Size*2 {
		return errors.New(
			"SHA-256 digest must contain 64 hexadecimal characters",
		)
	}

	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return errors.New("SHA-256 digest is invalid")
	}

	return nil
}

/* START ALPHA CHANGE 2066 -- remove obsolete generic archive path validation -- */
/* Archive metadata paths are checked against exact permitted names in archive_validation.go. */
/* FINISH ALPHA CHANGE 2066 -- remove obsolete generic archive path validation -- */

func readStrictJSON(path string, sizeLimit int64, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck

	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("JSON path is not a regular file: %s", path)
	}
	if info.Size() > sizeLimit {
		return fmt.Errorf(
			"JSON file %s exceeds size limit %d",
			path,
			sizeLimit,
		)
	}

	decoder := json.NewDecoder(io.LimitReader(file, sizeLimit+1))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(value); err != nil {
		return err
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON file contains multiple values")
		}

		return err
	}

	return nil
}

func marshalArchiveJSON(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(raw, '\n'), nil
}

func writeArchiveFileDurable(
	path string,
	raw []byte,
	exclusive bool,
) error {
	flags := os.O_WRONLY | os.O_CREATE
	if exclusive {
		flags |= os.O_EXCL
	} else {
		flags |= os.O_TRUNC
	}

	file, err := os.OpenFile(path, flags, 0660)
	if err != nil {
		return err
	}

	removeFile := exclusive
	defer func() {
		if removeFile {
			_ = os.Remove(path)
		}
	}()

	if _, err := file.Write(raw); err != nil {
		_ = file.Close()

		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()

		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	removeFile = false

	return nil
}

func syncRegularFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()

		return err
	}

	return file.Close()
}

func publishImmutableArchiveManifest(
	root string,
	manifest *ArchiveManifest,
) (string, string, error) {
	raw, err := marshalArchiveJSON(manifest)
	if err != nil {
		return "", "", err
	}

	/* START ALPHA CHANGE 2066 -- clarify cumulative archive manifest size limit -- */
	if int64(len(raw)) > archiveManifestSizeLimit {
		return "", "", fmt.Errorf(
			"archive manifest size %d exceeds supported limit %d; "+
				"every manifest must retain the complete archive history; "+
				"increase the supported manifest size limit consistently "+
				"in builders and readers before retrying",
			len(raw),
			archiveManifestSizeLimit,
		)
	}
	/* FINISH ALPHA CHANGE 2066 -- clarify cumulative archive manifest size limit -- */

	digestBytes := sha256.Sum256(raw)
	digest := hex.EncodeToString(digestBytes[:])
	name := archiveManifestName(digest)
	finalPath := filepath.Join(root, name)

	if existing, err := os.ReadFile(finalPath); err == nil {
		if !bytes.Equal(existing, raw) {
			return "", "", fmt.Errorf(
				"immutable archive manifest path contains different data: %s",
				finalPath,
			)
		}

		if err := syncRegularFile(finalPath); err != nil {
			return "", "", fmt.Errorf(
				"failed to synchronize existing immutable archive manifest: %w",
				err,
			)
		}
		if err := syncDirectory(root); err != nil {
			return "", "", fmt.Errorf(
				"failed to synchronize archive manifest directory: %w",
				err,
			)
		}

		return name, digest, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}

	temporary, err := os.CreateTemp(root, ".manifest-publishing-")
	if err != nil {
		return "", "", err
	}

	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(0660); err != nil {
		_ = temporary.Close()

		return "", "", err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()

		return "", "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()

		return "", "", err
	}
	if err := temporary.Close(); err != nil {
		return "", "", err
	}

	// Link publishes without replacing an existing immutable name.
	if err := os.Link(temporaryPath, finalPath); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", "", err
		}

		existing, readErr := os.ReadFile(finalPath)
		if readErr != nil {
			return "", "", readErr
		}
		if !bytes.Equal(existing, raw) {
			return "", "", fmt.Errorf(
				"immutable archive manifest path contains different data: %s",
				finalPath,
			)
		}
	}

	if err := syncRegularFile(finalPath); err != nil {
		return "", "", err
	}
	if err := syncDirectory(root); err != nil {
		return "", "", err
	}

	if err := os.Remove(temporaryPath); err != nil {
		return "", "", err
	}
	removeTemporary = false

	if err := syncDirectory(root); err != nil {
		return "", "", err
	}

	return name, digest, nil
}

func writeArchiveJSONDurable(path string, value any) error {
	raw, err := marshalArchiveJSON(value)
	if err != nil {
		return err
	}

	if err := writeArchiveFileDurable(path, raw, true); err != nil {
		return err
	}

	return syncDirectory(filepath.Dir(path))
}

/* FINISH ALPHA CHANGE 2066 -- provide strict crash-safe archive file operations -- */
