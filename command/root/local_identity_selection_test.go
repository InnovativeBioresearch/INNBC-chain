/* START ALPHA CHANGE 3275 -- verify legacy secrets CLI retirement and preserve current identity routing -- */
package root

import (
	"bytes"
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

func TestLocalIdentity_CommandRouting(t *testing.T) {
	dir := t.TempDir()
	remotePath := filepath.Join(dir, "remote.json")
	localPath := filepath.Join(dir, "nodes")
	configPath := filepath.Join(dir, "config")
	setupBoundary := errors.New("local command setup boundary reached")

	tests := []struct {
		name      string
		path      []string
		args      []string
		wantError string
		wantSetup bool
	}{
		{
			name:      "server_remote_flag",
			path:      []string{"server"},
			args:      []string{"--secrets-config", remotePath},
			wantError: "unknown flag: --secrets-config",
		},
		{
			name: "retained_node_create",
			path: []string{"node", "create"},
			args: []string{
				"1",
				"--base-dir", localPath,
				"--config-dir", configPath,
			},
			wantSetup: true,
		},
		{
			name:      "retained_node_info",
			path:      []string{"node", "info"},
			args:      []string{"node1", "--base-dir", localPath},
			wantSetup: true,
		},
	}

	for _, jsonOutput := range []bool{false, true} {
		for _, tc := range tests {
			t.Run(fmt.Sprintf("%s/json=%t", tc.name, jsonOutput), func(t *testing.T) {
				root := NewRootCommand()
				cmd, remaining, err := root.baseCmd.Find(tc.path)
				require.NoError(t, err)
				require.Empty(t, remaining)
				require.Equal(t, tc.path[len(tc.path)-1], cmd.Name())

				// Preserve actual flag and argument validation, then stop
				// before the command can read or create identity files.
				setupCalls := 0
				runCalls := 0
				cmd.PreRunE = func(*cobra.Command, []string) error {
					setupCalls++
					return setupBoundary
				}
				cmd.Run = nil
				cmd.RunE = func(*cobra.Command, []string) error {
					runCalls++
					return errors.New("unexpected command execution")
				}

				var output bytes.Buffer
				root.baseCmd.SetOut(&output)
				root.baseCmd.SetErr(&output)
				root.baseCmd.SilenceErrors = true
				root.baseCmd.SilenceUsage = true

				args := append([]string(nil), tc.path...)
				args = append(args, tc.args...)
				if jsonOutput {
					args = append([]string{"--json"}, args...)
				}
				root.baseCmd.SetArgs(args)

				err = root.baseCmd.Execute()
				if tc.wantSetup {
					require.ErrorIs(t, err, setupBoundary)
					require.Equal(t, 1, setupCalls)
				} else {
					require.ErrorContains(t, err, tc.wantError)
					require.Zero(t, setupCalls)
				}
				require.Zero(t, runCalls)

				entries, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Empty(t, entries)
			})
		}
	}
}

func TestLocalIdentity_ServerConfigurationSurface(t *testing.T) {
	_, rawSelectorExists := reflect.TypeOf(serverconfig.Config{}).
		FieldByName("SecretsConfigPath")
	require.False(t, rawSelectorExists)

	_, runtimeSelectorExists := reflect.TypeOf(runtimeserver.Config{}).
		FieldByName("SecretsManager")
	require.False(t, runtimeSelectorExists)

	root := NewRootCommand()
	cmd, remaining, err := root.baseCmd.Find([]string{"server"})
	require.NoError(t, err)
	require.Empty(t, remaining)

	require.Nil(t, cmd.Flags().Lookup("secrets-config"))
	require.NotNil(t, cmd.Flags().Lookup("config"))
	require.NotNil(t, cmd.Flags().Lookup("data-dir"))
}

func TestSecretsCLIRetired_CommandRouting(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "node")
	remotePath := filepath.Join(dir, "remote.json")

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "parent",
			args: []string{"secrets"},
		},
		{
			name: "output",
			args: []string{"secrets", "output"},
		},
		{
			name: "public_output_flags",
			args: []string{
				"secrets", "output",
				"--data-dir", localPath,
				"--validator",
			},
		},
		{
			name: "retired_init",
			args: []string{"secrets", "init"},
		},
		{
			name: "retired_generation_flags",
			args: []string{
				"secrets", "init",
				"--data-dir", localPath,
				"--num", "2",
				"--insecure",
			},
		},
		{
			name: "retired_generator",
			args: []string{"secrets", "generate"},
		},
		{
			name: "remote_configuration",
			args: []string{
				"secrets", "output", "--config", remotePath,
			},
		},
		{
			name: "retired_group_help",
			args: []string{"secrets", "--help"},
		},
	}

	for _, jsonOutput := range []bool{false, true} {
		for _, tc := range tests {
			t.Run(fmt.Sprintf("%s/json=%t", tc.name, jsonOutput), func(t *testing.T) {
				root := NewRootCommand()

				for _, child := range root.baseCmd.Commands() {
					require.NotEqual(t, "secrets", child.Name())
					require.NotContains(t, child.Aliases, "secrets")
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

				err := root.baseCmd.Execute()
				require.ErrorContains(t, err, `unknown command "secrets"`)

				entries, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Empty(t, entries,
					"retired command created identity or configuration files")
			})
		}
	}
}

func TestSecretsCLIRetired_Help(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("json=%t", jsonOutput), func(t *testing.T) {
			root := NewRootCommand()

			var output bytes.Buffer
			root.baseCmd.SetOut(&output)
			root.baseCmd.SetErr(&output)
			root.baseCmd.SilenceErrors = true
			root.baseCmd.SilenceUsage = true

			args := []string{"--help"}
			if jsonOutput {
				args = append([]string{"--json"}, args...)
			}
			root.baseCmd.SetArgs(args)

			require.NoError(t, root.baseCmd.Execute())
			require.NotContains(t, output.String(), "\n  secrets ")
		})
	}
}

/* FINISH ALPHA CHANGE 3275 -- verify legacy secrets CLI retirement and preserve current identity routing -- */
