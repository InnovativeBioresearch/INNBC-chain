//go:build linux

/* START ALPHA CHANGE 2085 -- test builder locks across processes and canonical paths -- */
package snapshotbuilder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const buildLockHelperEnvironment = "INNBC_BUILD_LOCK_TEST_HELPER"

func TestBuildLockCrossProcessContention(t *testing.T) {
	root := filepath.Join(t.TempDir(), "publication-root")
	lock, err := acquireBuildLock(root)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = lock.Close()
	})

	runBuildLockChild(t, root, 23)

	require.NoError(t, lock.Close())
	runBuildLockChild(t, root, 0)
}

func runBuildLockChild(t *testing.T, root string, expectedStatus int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	process := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run=^TestBuildLockProcessHelper$",
	)
	process.Env = append(
		os.Environ(),
		buildLockHelperEnvironment+"=1",
		"INNBC_BUILD_LOCK_TEST_ROOT="+root,
	)

	output, err := process.CombinedOutput()
	require.NoError(t, ctx.Err(), "lock subprocess timed out: %s", output)

	if expectedStatus == 0 {
		require.NoError(t, err, "%s", output)
		return
	}

	var exitError *exec.ExitError
	require.True(t, errors.As(err, &exitError), "%v: %s", err, output)
	require.Equal(t, expectedStatus, exitError.ExitCode(), "%s", output)
	require.Contains(t, string(output), "cannot acquire builder lock")
}

func TestBuildLockProcessHelper(t *testing.T) {
	if os.Getenv(buildLockHelperEnvironment) != "1" {
		return
	}

	lock, err := acquireBuildLock(os.Getenv("INNBC_BUILD_LOCK_TEST_ROOT"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(23)
	}
	if err := lock.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(24)
	}

	os.Exit(0)
}

func TestBuildLocksCanonicalizeSymlinkAliases(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "publication-root")
	alias := filepath.Join(parent, "publication-alias")

	require.NoError(t, os.MkdirAll(root, 0750))
	require.NoError(t, os.Symlink(root, alias))

	locks, err := acquireBuildLocks(alias, root, alias)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = locks.Close()
	})

	set, ok := locks.(*buildLockSet)
	require.True(t, ok)
	require.Len(t, set.locks, 1)

	_, err = acquireBuildLock(root)
	require.ErrorContains(t, err, "cannot acquire builder lock")

	require.NoError(t, locks.Close())

	retry, err := acquireBuildLock(alias)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = retry.Close()
	})
	require.NoError(t, retry.Close())
}

func TestBuildLocksSortAndReleasePartialAcquisition(t *testing.T) {
	parent := t.TempDir()
	firstRoot := filepath.Join(parent, "a-first")
	secondRoot := filepath.Join(parent, "z-second")

	blocker, err := acquireBuildLock(secondRoot)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = blocker.Close()
	})

	// Reverse input order. Sorted acquisition must still open a-first before
	// attempting the already-held z-second lock.
	_, err = acquireBuildLocks(secondRoot, firstRoot)
	require.ErrorContains(t, err, "cannot acquire builder lock")
	require.FileExists(t, filepath.Join(firstRoot, buildLockFileName))

	retry, err := acquireBuildLock(firstRoot)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = retry.Close()
	})
	require.NoError(t, retry.Close())
	require.NoError(t, blocker.Close())
}

/* FINISH ALPHA CHANGE 2085 -- test builder locks across processes and canonical paths -- */
