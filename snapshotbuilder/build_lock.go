/* START ALPHA CHANGE 2073 -- serialize snapshot and archive writers -- */
package snapshotbuilder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const buildLockFileName = ".innbc-builder.lock"

type buildLock interface {
	Close() error
}

type buildLockSet struct {
	locks []buildLock
}

func acquireBuildLock(root string) (buildLock, error) {
	return acquireBuildLocks(root)
}

func acquireBuildLocks(roots ...string) (buildLock, error) {
	if len(roots) == 0 {
		return nil, errors.New("at least one build-lock root is required")
	}

	canonicalRoots := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))

	for _, root := range roots {
		canonical, err := canonicalBuildRoot(root)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[canonical]; exists {
			continue
		}

		seen[canonical] = struct{}{}
		canonicalRoots = append(canonicalRoots, canonical)
	}
	sort.Strings(canonicalRoots)

	result := &buildLockSet{
		locks: make([]buildLock, 0, len(canonicalRoots)),
	}

	for _, root := range canonicalRoots {
		lock, err := acquirePlatformBuildLock(
			filepath.Join(root, buildLockFileName),
		)
		if err != nil {
			return nil, errors.Join(err, result.Close())
		}
		result.locks = append(result.locks, lock)
	}

	return result, nil
}

func canonicalBuildRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("build-lock root is required")
	}
	if err := os.MkdirAll(root, 0770); err != nil {
		return "", fmt.Errorf("create build-lock root: %w", err)
	}

	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve build-lock root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve build-lock root links: %w", err)
	}
	if err := requireArchiveDirectory(canonical); err != nil {
		return "", err
	}

	return filepath.Clean(canonical), nil
}

func openBuildLockFile(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("build lock is not a regular file: %s", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		return nil, fmt.Errorf("open build lock %s: %w", path, err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("build lock is not a regular file: %s", path)
	}

	return file, nil
}

func (s *buildLockSet) Close() error {
	if s == nil {
		return nil
	}

	var result error
	for index := len(s.locks) - 1; index >= 0; index-- {
		result = errors.Join(result, s.locks[index].Close())
	}
	s.locks = nil

	return result
}

/* FINISH ALPHA CHANGE 2073 -- serialize snapshot and archive writers -- */
