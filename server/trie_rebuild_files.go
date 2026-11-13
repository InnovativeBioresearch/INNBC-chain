/* START ALPHA CHANGE 2113 -- publish rebuilt trie only into an absent or empty target -- */
package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func requireTrieRebuildDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect directory %s: %w", path, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("expected a real directory: %s", path)
	}

	return nil
}

func requireEmptyTrieRebuildTarget(path string) error {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := requireTrieRebuildDirectory(path); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf(
			"trie rebuild requires an absent or completely empty directory: %s",
			path,
		)
	}

	return nil
}

func syncTrieRebuildDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}

	return errors.Join(directory.Sync(), directory.Close())
}

// An MDBX trie environment stores its files directly in this directory.
// Refuse unexpected directories, symlinks, and special files.
func syncTrieRebuildStage(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected rebuilt trie entry: %s", entry.Name())
		}
		file, err := os.OpenFile(filepath.Join(path, entry.Name()), os.O_RDWR, 0)
		if err != nil {
			return err
		}
		if err := errors.Join(file.Sync(), file.Close()); err != nil {
			return err
		}
	}

	return syncTrieRebuildDirectory(path)
}

func publishTrieRebuild(stage, target string) error {
	if err := requireEmptyTrieRebuildTarget(target); err != nil {
		return err
	}

	// Remove only an empty directory. This never recursively removes data.
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove empty trie target: %w", err)
	}

	if err := os.Rename(stage, target); err != nil {
		return fmt.Errorf("publish rebuilt trie: %w", err)
	}
	if err := syncTrieRebuildDirectory(filepath.Dir(target)); err != nil {
		return fmt.Errorf(
			"rebuilt trie was installed at %s, but parent-directory synchronization failed; "+
				"keep the node stopped and inspect the filesystem: %w",
			target,
			err,
		)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2113 -- publish rebuilt trie only into an absent or empty target -- */
