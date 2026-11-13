/* START ALPHA CHANGE 3282 -- verify snapshot process status, result formatting and cleanup after failure -- */
package root

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	snapshotbuild "github.com/innovativebioresearch/innbc-chain/command/snapshot/build"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	"github.com/stretchr/testify/require"
)

const snapshotBuildExitChild = "INNBC_SNAPSHOT_BUILD_EXIT_CHILD"

func TestSnapshotBuildExit_Subprocess(t *testing.T) {
	if os.Getenv(snapshotBuildExitChild) != "1" {
		return
	}

	separator := slices.Index(os.Args, "--")
	if separator < 0 {
		fmt.Fprintln(os.Stderr, "missing snapshot subprocess argument separator")
		os.Exit(2)
	}

	root := NewRootCommand()
	root.baseCmd.SetArgs(os.Args[separator+1:])
	root.Execute()

	// A normal return must remain success. The parent detects the bug
	// if an expected failure reaches this point.
	os.Exit(0)
}

func snapshotBuildExitRun(
	t *testing.T,
	dir string,
	args ...string,
) (string, string, error) {
	t.Helper()

	executable, err := os.Executable()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	childArgs := append(
		[]string{"-test.run=^TestSnapshotBuildExit_Subprocess$", "--"},
		args...,
	)
	process := exec.CommandContext(ctx, executable, childArgs...)
	process.Dir = dir
	process.Env = append(os.Environ(), snapshotBuildExitChild+"=1")

	var stdout, stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr

	err = process.Run()
	require.NoError(t, ctx.Err(), "snapshot command timed out")

	return stdout.String(), stderr.String(), err
}

func snapshotBuildExitAssertFailure(
	t *testing.T,
	stdout string,
	stderr string,
	err error,
	jsonOutput bool,
	wantMessage string,
) {
	t.Helper()

	var exitError *exec.ExitError
	require.True(t, errors.As(err, &exitError),
		"expected a failed subprocess; err=%v\nstdout:\n%s\nstderr:\n%s",
		err, stdout, stderr)
	require.Equal(t, 1, exitError.ExitCode())
	require.Empty(t, strings.TrimSpace(stdout))
	require.NotContains(t, stderr, "[SNAPSHOT BUILT]")
	require.Contains(t, stderr, wantMessage)

	if jsonOutput {
		// MDBX can write native diagnostics before the formatted result.
		// The final nonempty line must be the unchanged JSON error object.
		lines := strings.Split(strings.TrimSpace(stderr), "\n")
		errorLine := lines[len(lines)-1]

		var response map[string]string
		require.NoError(t, json.Unmarshal([]byte(errorLine), &response))
		require.Len(t, response, 1)
		require.Contains(t, response["error"], wantMessage)
	}
}

func TestSnapshotBuildExit_EarlyFailures(t *testing.T) {
	tests := []struct {
		kind    string
		message string
	}{
		{"unsupported", "unsupported snapshot type"},
		{"checkpoint", "failed to load chain configuration"},
		{"segmented", "failed to load chain configuration"},
	}

	for _, tc := range tests {
		for _, jsonOutput := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/json=%t", tc.kind, jsonOutput), func(t *testing.T) {
				// Deliberately leave the configuration and source absent.
				f := newSnapshotBuildExitPaths(t)
				stdout, stderr, err := snapshotBuildExitRun(
					t, f.root, f.args(tc.kind, jsonOutput)...,
				)

				snapshotBuildExitAssertFailure(
					t, stdout, stderr, err, jsonOutput, tc.message,
				)
				require.NoDirExists(t, f.output)
				require.NoDirExists(t, f.source)

				// These early failures never open MDBX, so JSON mode
				// must produce exactly one error object.
				if jsonOutput {
					var response map[string]string
					require.NoError(t, json.Unmarshal(
						[]byte(strings.TrimSpace(stderr)), &response,
					))
					require.Len(t, response, 1)
				}
			})
		}
	}
}

func TestSnapshotBuildExit_PublicationAndCleanup(t *testing.T) {
	for _, kind := range []string{"checkpoint", "segmented"} {
		for _, jsonOutput := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/json=%t", kind, jsonOutput), func(t *testing.T) {
				f := newSnapshotBuildExitFixture(t)

				stdout, stderr, err := snapshotBuildExitRun(
					t, f.root, f.args(kind, jsonOutput)...,
				)
				require.NoError(t, err, "stdout:\n%s\nstderr:\n%s", stdout, stderr)

				kindRoot := filepath.Join(f.output, kind)
				latestPath := filepath.Join(kindRoot, "latest.json")
				latest, err := snapshotbuilder.LoadLatest(latestPath)
				require.NoError(t, err)
				require.Equal(t, snapshotbuilder.Kind(kind), latest.SnapshotKind)
				require.Equal(t, uint64(4), latest.BlockNumber)
				require.Equal(t, f.blockHash, latest.BlockHash)
				require.Equal(t, f.stateRoot, latest.StateRoot)

				artifactDir := filepath.Join(kindRoot, latest.SnapshotDir)
				manifest, err := snapshotbuilder.ValidatePublishedArtifact(
					artifactDir, snapshotbuilder.Kind(kind), 1,
					hclog.NewNullLogger(),
				)
				require.NoError(t, err)
				require.Equal(t, uint64(3), manifest.RetainedFrom)
				require.Equal(t, uint64(4), manifest.RetainedTo)

				if jsonOutput {
					var result snapshotbuild.SnapshotBuildResult
					require.NoError(t, json.Unmarshal([]byte(stdout), &result))
					require.Equal(t, kind, result.SnapshotKind)
					require.Equal(t, uint64(4), result.BlockNumber)
					require.Equal(t, f.blockHash.String(), result.BlockHash)
					require.Equal(t, f.stateRoot.String(), result.StateRoot)
					require.Equal(t, uint64(3), result.RetainedFrom)
					require.Equal(t, uint64(4), result.RetainedTo)
					require.Equal(t, artifactDir, result.SnapshotDir)
					require.Equal(t, latestPath, result.LatestPath)
					_, err := time.ParseDuration(result.Duration)
					require.NoError(t, err)

					var fields map[string]json.RawMessage
					require.NoError(t, json.Unmarshal([]byte(stdout), &fields))
					require.Len(t, fields, 9)
					for _, name := range []string{
						"snapshot_kind", "block_number", "block_hash",
						"state_root", "retained_from", "retained_to",
						"snapshot_dir", "latest_path", "duration",
					} {
						require.Contains(t, fields, name)
					}
				} else {
					for _, value := range []string{
						"[SNAPSHOT BUILT]", "Snapshot Kind",
						kind, f.blockHash.String(), f.stateRoot.String(),
						artifactDir, latestPath,
					} {
						require.Contains(t, stdout, value)
					}
				}

				snapshotBuildExitAssertNoStaging(t, kindRoot)
				before := snapshotBuildExitFiles(t, kindRoot)

				// Rebuilding this exact checkpoint reaches the existing
				// directory rejection after construction and validation
				// of a fresh staged artifact.
				stdout, stderr, err = snapshotBuildExitRun(
					t, f.root, f.args(kind, jsonOutput)...,
				)
				snapshotBuildExitAssertFailure(
					t, stdout, stderr, err, jsonOutput,
					"snapshot directory already exists",
				)

				snapshotBuildExitAssertNoStaging(t, kindRoot)
				require.Equal(t, before, snapshotBuildExitFiles(t, kindRoot),
					"failed build changed the existing publication")

				_, err = snapshotbuilder.ValidatePublishedArtifact(
					artifactDir, snapshotbuilder.Kind(kind), 1,
					hclog.NewNullLogger(),
				)
				require.NoError(t, err)
			})
		}
	}
}

func snapshotBuildExitAssertNoStaging(t *testing.T, root string) {
	t.Helper()

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, strings.HasPrefix(entry.Name(), ".building-"),
			"unpublished staging remains: %s", entry.Name())
	}
}

func snapshotBuildExitFiles(
	t *testing.T,
	root string,
) map[string][sha256.Size]byte {
	t.Helper()

	files := make(map[string][sha256.Size]byte)
	err := filepath.WalkDir(root, func(
		path string,
		entry fs.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		// Reader-lock bookkeeping is mutable and is not published data.
		if entry.IsDir() || entry.Name() == "mdbx.lck" {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[relative] = sha256.Sum256(raw)
		return nil
	})
	require.NoError(t, err)

	return files
}

/* FINISH ALPHA CHANGE 3282 -- verify snapshot process status, result formatting and cleanup after failure -- */
