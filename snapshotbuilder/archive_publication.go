/* START ALPHA CHANGE 2069 -- publish immutable archive segment directories atomically -- */
package snapshotbuilder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func publishArchiveSegment(
	segmentsRoot string,
	buildDir string,
	finalDir string,
) error {
	if filepath.Dir(buildDir) != segmentsRoot {
		return errors.New(
			"archive staging directory is outside the segment publication root",
		)
	}
	if filepath.Dir(finalDir) != segmentsRoot {
		return errors.New(
			"archive final directory is outside the segment publication root",
		)
	}

	if err := syncArchiveTree(buildDir); err != nil {
		return fmt.Errorf("failed to synchronize archive segment staging: %w", err)
	}

	if _, err := os.Stat(finalDir); err == nil {
		return fmt.Errorf("archive segment directory already exists: %s", finalDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Rename(buildDir, finalDir); err != nil {
		return fmt.Errorf("failed to publish archive segment directory: %w", err)
	}

	if err := syncDirectory(segmentsRoot); err != nil {
		return fmt.Errorf(
			"failed to persist archive segment directory publication: %w",
			err,
		)
	}

	return nil
}

func syncArchiveTree(root string) error {
	var directories []string

	err := filepath.WalkDir(
		root,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			info, err := entry.Info()
			if err != nil {
				return err
			}

			switch {
			case info.Mode().IsRegular():
				file, err := os.Open(path)
				if err != nil {
					return err
				}

				syncErr := file.Sync()
				closeErr := file.Close()

				if syncErr != nil {
					return syncErr
				}

				return closeErr
			case entry.IsDir():
				directories = append(directories, path)

				return nil
			case info.Mode()&os.ModeSymlink != 0:
				return fmt.Errorf(
					"archive segment contains symbolic link: %s",
					path,
				)
			default:
				return fmt.Errorf(
					"archive segment contains unsupported filesystem entry: %s",
					path,
				)
			}
		},
	)
	if err != nil {
		return err
	}

	for index := len(directories) - 1; index >= 0; index-- {
		if err := syncDirectory(directories[index]); err != nil {
			return err
		}
	}

	return nil
}

/* FINISH ALPHA CHANGE 2069 -- publish immutable archive segment directories atomically -- */
