/* START ALPHA CHANGE 2117 -- test trie rebuild command registration and failure exit status -- */
package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/innovativebioresearch/innbc-chain/command/helper"
)

func TestTrieRebuildCommandRegistered(t *testing.T) {
	cmd, remaining, err := GetCommand().Find([]string{"trie-rebuild"})
	require.NoError(t, err)
	require.Empty(t, remaining)
	require.Equal(t, "trie-rebuild", cmd.Name())
	require.NotNil(t, cmd.Flags().Lookup("chain"))
	require.NotNil(t, cmd.Flags().Lookup("data-dir"))
	require.NotNil(t, cmd.Flags().Lookup("mdbx-map-size-gb"))
	require.Contains(t, cmd.Long, "MUST remain stopped")
}

func TestTrieRebuildCommandFailureStatus(t *testing.T) {
	for _, mode := range []string{"plain", "json"} {
		t.Run(mode, func(t *testing.T) {
			process := exec.Command(
				os.Args[0],
				"-test.run=^TestTrieRebuildCommandFailureHelper$",
			)
			process.Env = append(
				os.Environ(),
				"INNBC_TRIE_REBUILD_HELPER=1",
				"INNBC_TRIE_REBUILD_MODE="+mode,
				"INNBC_TRIE_REBUILD_ROOT="+t.TempDir(),
			)

			var stdout, stderr bytes.Buffer
			process.Stdout = &stdout
			process.Stderr = &stderr
			err := process.Run()

			var exitError *exec.ExitError
			require.True(t, errors.As(err, &exitError))
			require.Equal(t, 1, exitError.ExitCode())
			require.Empty(t, strings.TrimSpace(stdout.String()))

			output := strings.TrimSpace(stderr.String())
			require.NotEmpty(t, output)
			if mode == "json" {
				var result struct {
					Error string `json:"error"`
				}
				require.NoError(t, json.Unmarshal([]byte(output), &result))
				require.NotEmpty(t, result.Error)
			}
		})
	}
}

func TestTrieRebuildCommandFailureHelper(t *testing.T) {
	if os.Getenv("INNBC_TRIE_REBUILD_HELPER") != "1" {
		return
	}

	cmd := getTrieRebuildCommand()
	helper.RegisterJSONOutputFlag(cmd)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	root := os.Getenv("INNBC_TRIE_REBUILD_ROOT")
	args := []string{
		"--chain", filepath.Join(root, "genesis.json"),
		"--data-dir", filepath.Join(root, "missing-node"),
		"--mdbx-map-size-gb", "1",
	}
	if os.Getenv("INNBC_TRIE_REBUILD_MODE") == "json" {
		args = append(args, "--json")
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

/* FINISH ALPHA CHANGE 2117 -- test trie rebuild command registration and failure exit status -- */
