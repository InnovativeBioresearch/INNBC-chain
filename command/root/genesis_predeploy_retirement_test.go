/* START ALPHA CHANGE 3217 -- verify retired predeploy invocations cannot reach genesis generation or alter files -- */
package root

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestGenesisPredeployRetired_CommandRouting(t *testing.T) {
	workDir := t.TempDir()
	genesisPath := filepath.Join(workDir, "genesis.json")
	artifactPath := filepath.Join(workDir, "artifacts.json")
	newGenesisPath := filepath.Join(workDir, "new-genesis.json")

	originalGenesis := []byte("{\"marker\":\"preserve-original-genesis\"}\n")
	originalArtifact := []byte("{\"marker\":\"preserve-original-artifact\"}\n")

	require.NoError(t, os.WriteFile(genesisPath, originalGenesis, 0640))
	require.NoError(t, os.WriteFile(artifactPath, originalArtifact, 0640))

	setupFailure := errors.New("ordinary genesis setup boundary reached")

	tests := []struct {
		name        string
		args        []string
		wantMessage string
		wantSetup   bool
		help        bool
	}{
		{
			name:        "bare_retired_command",
			args:        []string{"predeploy"},
			wantMessage: `unknown command "predeploy"`,
		},
		{
			name: "retired_command_before_current_flags",
			args: []string{
				"predeploy",
				"--dir", genesisPath,
			},
			wantMessage: `unknown command "predeploy"`,
		},
		{
			name: "retired_command_after_current_flags",
			args: []string{
				"--dir", genesisPath,
				"predeploy",
			},
			wantMessage: `unknown command "predeploy"`,
		},
		{
			name: "retired_command_cannot_create_genesis",
			args: []string{
				"predeploy",
				"--dir", newGenesisPath,
			},
			wantMessage: `unknown command "predeploy"`,
		},
		{
			name: "retired_command_flags",
			args: []string{
				"predeploy",
				"--chain", genesisPath,
				"--predeploy-address",
				"0x0000000000000000000000000000000000001100",
				"--artifacts-path", artifactPath,
			},
			wantMessage: "unknown flag",
		},
		{
			name: "ordinary_flags_reach_setup",
			args: []string{
				"--dir", genesisPath,
			},
			wantSetup: true,
		},
		{
			name: "help",
			args: []string{"--help"},
			help: true,
		},
	}

	for _, jsonOutput := range []bool{false, true} {
		for _, test := range tests {
			t.Run(fmt.Sprintf(
				"%s/json=%t", test.name, jsonOutput,
			), func(t *testing.T) {
				root := NewRootCommand()

				genesisCmd, remaining, err := root.baseCmd.Find(
					[]string{"genesis"},
				)
				require.NoError(t, err)
				require.Empty(t, remaining)
				require.Equal(t, "genesis", genesisCmd.Name())

				for _, child := range genesisCmd.Commands() {
					require.NotEqual(t, "predeploy", child.Name())
				}

				// Stop at setup if dispatch incorrectly reaches generation.
				// This also provides a control for ordinary flag-only input.
				setupCalls := 0
				runCalls := 0
				genesisCmd.PreRunE = func(
					_ *cobra.Command,
					_ []string,
				) error {
					setupCalls++
					return setupFailure
				}
				genesisCmd.Run = func(
					_ *cobra.Command,
					_ []string,
				) {
					runCalls++
				}

				var output bytes.Buffer
				root.baseCmd.SetOut(&output)
				root.baseCmd.SetErr(&output)
				root.baseCmd.SilenceErrors = true
				root.baseCmd.SilenceUsage = true

				args := append([]string{"genesis"}, test.args...)
				if jsonOutput {
					args = append([]string{"--json"}, args...)
				}
				root.baseCmd.SetArgs(args)

				// Use the real root dispatcher without its os.Exit wrapper.
				err = root.baseCmd.Execute()

				switch {
				case test.help:
					require.NoError(t, err)
					require.Zero(t, setupCalls)
					require.Contains(t, output.String(), "--dir")
					require.NotContains(t, output.String(), "predeploy")
				case test.wantSetup:
					require.ErrorIs(t, err, setupFailure)
					require.Equal(t, 1, setupCalls)
				default:
					require.ErrorContains(t, err, test.wantMessage)
					require.NotErrorIs(t, err, setupFailure)
					require.Zero(t, setupCalls)
				}
				require.Zero(t, runCalls)

				genesisBytes, err := os.ReadFile(genesisPath)
				require.NoError(t, err)
				require.Equal(t, originalGenesis, genesisBytes)

				artifactBytes, err := os.ReadFile(artifactPath)
				require.NoError(t, err)
				require.Equal(t, originalArtifact, artifactBytes)

				require.NoFileExists(t, newGenesisPath)

				entries, err := os.ReadDir(workDir)
				require.NoError(t, err)
				require.Len(t, entries, 2)
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3217 -- verify retired predeploy invocations cannot reach genesis generation or alter files -- */
