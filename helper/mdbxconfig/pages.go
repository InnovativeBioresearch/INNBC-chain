/* START ALPHA CHANGE 3452 -- share MDBX page-size configuration and compatibility checks -- */
package mdbxconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
)

const DefaultPageSizeBytes = 4096

var ErrPageSizeMismatch = errors.New("MDBX page-size mismatch")

// PageSize implements pflag.Value and configuration decoding.
// Its zero value represents an omitted programmatic setting.
type PageSize uint64

func (p PageSize) String() string {
	if p == 0 {
		return strconv.Itoa(DefaultPageSizeBytes)
	}
	return strconv.FormatUint(uint64(p), 10)
}

func (*PageSize) Type() string {
	return "mdbx-page-size"
}

func (p *PageSize) Set(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		*p = DefaultPageSizeBytes
		return nil
	}

	size, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid MDBX page size %q: %w", text, err)
	}
	if err := Validate(size); err != nil {
		return err
	}

	*p = PageSize(size)
	return nil
}

func (p *PageSize) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, `"`) {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
	}
	return p.Set(text)
}

func (p *PageSize) UnmarshalYAML(unmarshal func(any) error) error {
	var text string
	if err := unmarshal(&text); err != nil {
		return err
	}
	return p.Set(text)
}

// Validate checks an explicitly supplied numeric value.
func Validate(size uint64) error {
	if size < 4096 || size > 65536 || size&(size-1) != 0 {
		return fmt.Errorf(
			"invalid MDBX page size %d: expected 4096, 8192, 16384, 32768 or 65536 bytes",
			size,
		)
	}
	return nil
}

// Resolve preserves existing programmatic callers that omit the new option.
func Resolve(options ...uint64) (int, error) {
	if len(options) > 1 {
		return 0, errors.New("only one MDBX page-size option is permitted")
	}

	size := uint64(DefaultPageSizeBytes)
	if len(options) == 1 && options[0] != 0 {
		size = options[0]
	}
	if err := Validate(size); err != nil {
		return 0, err
	}
	return int(size), nil
}

func compare(path string, expected uint64, actual uint) error {
	if uint64(actual) != expected {
		return fmt.Errorf(
			"%w at %q: configured %d bytes, stored %d bytes; "+
				"existing databases cannot change page size in place",
			ErrPageSizeMismatch, path, expected, actual,
		)
	}
	return nil
}

// CheckDatabase permits an absent or empty destination directory.
// Existing data must have readable MDBX metadata.
func CheckDatabase(path string, size uint64) error {
	expected, err := Resolve(size)
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect MDBX directory %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("MDBX path is not a directory: %q", path)
	}

	dataPath := filepath.Join(path, "mdbx.dat")
	dataInfo, err := os.Lstat(dataPath)
	if errors.Is(err, os.ErrNotExist) {
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return readErr
		}
		if len(entries) == 0 {
			return nil
		}
		return fmt.Errorf(
			"existing MDBX directory %q is nonempty but mdbx.dat is missing",
			path,
		)
	}
	if err != nil {
		return err
	}
	if !dataInfo.Mode().IsRegular() {
		return fmt.Errorf("MDBX data is not a regular file: %q", dataPath)
	}

	stored, err := mdbxgo.PreOpenSnapInfo(path)
	if err != nil {
		return fmt.Errorf("read MDBX geometry at %q: %w", path, err)
	}
	return compare(path, uint64(expected), stored.PageSize)
}

// CheckOpen verifies the actual opened handle before application writes.
func CheckOpen(env *mdbxgo.Env, path string, size uint64) error {
	expected, err := Resolve(size)
	if err != nil {
		return err
	}
	info, err := env.Info(nil)
	if err != nil {
		return fmt.Errorf("inspect opened MDBX geometry at %q: %w", path, err)
	}
	return compare(path, uint64(expected), info.PageSize)
}

// CheckTree checks database metadata only. Retired segment directories are
// outside active history and remain subject to existing retirement cleanup.
func CheckTree(root string, size uint64) error {
	if _, err := Resolve(size); err != nil {
		return err
	}

	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}

	return filepath.WalkDir(resolved, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != resolved &&
				strings.HasPrefix(entry.Name(), ".deleting-segment-") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link in database tree: %q", path)
		}
		if entry.Name() == "mdbx.dat" {
			return CheckDatabase(filepath.Dir(path), size)
		}
		return nil
	})
}

// CheckChainData deliberately excludes the separately configured consensus DB.
func CheckChainData(chainData string, size uint64) error {
	for _, name := range []string{"blockchain", "trie"} {
		if err := CheckTree(filepath.Join(chainData, name), size); err != nil {
			return err
		}
	}
	return nil
}

/* FINISH ALPHA CHANGE 3452 -- share MDBX page-size configuration and compatibility checks -- */
