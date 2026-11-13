/* START ALPHA CHANGE 3270 -- verify legacy archive retirement and retained maintenance routing -- */
package root

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	serverconfig "github.com/innovativebioresearch/innbc-chain/command/server/config"
	runtimeserver "github.com/innovativebioresearch/innbc-chain/server"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestLegacyArchiveRetired_CommandRouting(t *testing.T) {
	workDir := t.TempDir()
	archivePath := filepath.Join(workDir, "legacy.rlp")
	setupBoundary := errors.New("server setup boundary reached")

	tests := []struct {
		name      string
		args      []string
		wantError string
	}{
		{
			name:      "bare_backup",
			args:      []string{"backup"},
			wantError: `unknown command "backup"`,
		},
		{
			name: "backup_with_output",
			args: []string{
				"backup", "--out", archivePath, "--from", "0", "--to", "1",
			},
			wantError: `unknown command "backup"`,
		},
		{
			name:      "restore",
			args:      []string{"server", "--restore", archivePath},
			wantError: "unknown flag: --restore",
		},
		{
			name: "restore_allowance",
			args: []string{
				"server", "--restore-max-block-bytes", "16777216",
			},
			wantError: "unknown flag: --restore-max-block-bytes",
		},
	}

	for _, jsonOutput := range []bool{false, true} {
		for _, tc := range tests {
			t.Run(fmt.Sprintf("%s/json=%t", tc.name, jsonOutput), func(t *testing.T) {
				root := NewRootCommand()

				for _, child := range root.baseCmd.Commands() {
					require.NotEqual(t, "backup", child.Name())
					require.NotContains(t, child.Aliases, "backup")
				}

				serverCmd, remaining, err := root.baseCmd.Find([]string{"server"})
				require.NoError(t, err)
				require.Empty(t, remaining)
				require.Equal(t, "server", serverCmd.Name())
				require.Nil(t, serverCmd.Flags().Lookup("restore"))
				require.Nil(t, serverCmd.Flags().Lookup("restore-max-block-bytes"))

				setupCalls := 0
				runCalls := 0
				serverCmd.PreRunE = func(*cobra.Command, []string) error {
					setupCalls++
					return setupBoundary
				}
				serverCmd.Run = nil
				serverCmd.RunE = func(*cobra.Command, []string) error {
					runCalls++
					return errors.New("unexpected server execution")
				}

				var output bytes.Buffer
				root.baseCmd.SetOut(&output)
				root.baseCmd.SetErr(&output)
				root.baseCmd.SilenceErrors = true
				root.baseCmd.SilenceUsage = true

				args := append([]string(nil), tc.args...)
				if jsonOutput {
					args = append([]string{"--json"}, args...)
				}
				root.baseCmd.SetArgs(args)

				err = root.baseCmd.Execute()
				require.ErrorContains(t, err, tc.wantError)
				require.NotErrorIs(t, err, setupBoundary)
				require.Zero(t, setupCalls)
				require.Zero(t, runCalls)

				entries, err := os.ReadDir(workDir)
				require.NoError(t, err)
				require.Empty(t, entries)
			})
		}
	}
}

func TestLegacyArchiveRetired_RetainedCommands(t *testing.T) {
	for _, path := range [][]string{
		{"snapshot", "backup"},
		{"snapshot", "build"},
		{"snapshot", "archive"},
		{"snapshot", "recovery-checkpoint"},
		{"node", "trie-rebuild"},
		{"server", "export"},
	} {
		t.Run(fmt.Sprint(path), func(t *testing.T) {
			root := NewRootCommand()
			cmd, remaining, err := root.baseCmd.Find(path)
			require.NoError(t, err)
			require.Empty(t, remaining)
			require.Equal(t, path[len(path)-1], cmd.Name())
		})
	}
}

func TestLegacyArchiveRetired_ConfigurationSurface(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(serverconfig.Config{}),
		reflect.TypeOf(runtimeserver.Config{}),
	} {
		for _, name := range []string{"RestoreFile", "RestoreMaxBlockBytes"} {
			_, exists := typ.FieldByName(name)
			require.False(t, exists, "%s still exposes %s", typ, name)
		}
	}

	data, err := json.Marshal(serverconfig.DefaultConfig())
	require.NoError(t, err)
	require.NotContains(t, string(data), `"restore_file"`)
	require.NotContains(t, string(data), `"restore_max_block_bytes"`)
}

/* FINISH ALPHA CHANGE 3270 -- verify legacy archive retirement and retained maintenance routing -- */
