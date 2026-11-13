/* START ALPHA CHANGE 3255 -- verify the retired INNBCBFT identity command is absent and rejected -- */
package root

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestINNBCBFTSecretsRetired_CommandRouting(t *testing.T) {
	workDir := t.TempDir()
	nodePath := filepath.Join(workDir, "node")
	remotePath := filepath.Join(workDir, "remote.json")

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "bare_command",
			args: []string{"innbcbft-secrets"},
		},
		{
			name: "local_generation",
			args: []string{
				"innbcbft-secrets",
				"--data-dir", nodePath,
				"--num", "4",
				"--insecure",
			},
		},
		{
			name: "private_output",
			args: []string{
				"innbcbft-secrets",
				"--data-dir", nodePath,
				"--output",
				"--private",
				"--insecure",
			},
		},
		{
			name: "remote_configuration",
			args: []string{
				"innbcbft-secrets",
				"--config", remotePath,
			},
		},
	}

	for _, jsonOutput := range []bool{false, true} {
		for _, tc := range tests {
			t.Run(fmt.Sprintf("%s/json=%t", tc.name, jsonOutput), func(t *testing.T) {
				root := NewRootCommand()

				for _, child := range root.baseCmd.Commands() {
					require.NotEqual(t, "innbcbft-secrets", child.Name())
					require.NotContains(t, child.Aliases, "innbcbft-secrets")
				}

				// Retained commands must still resolve normally.
				for _, path := range [][]string{
					{"node", "create"},
					{"node", "info"},
					{"genesis"},
					{"server"},
				} {
					cmd, remaining, err := root.baseCmd.Find(path)
					require.NoError(t, err)
					require.Empty(t, remaining)
					require.Equal(t, path[len(path)-1], cmd.Name())
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

				// Exercise the actual dispatcher without its os.Exit wrapper.
				err := root.baseCmd.Execute()
				require.ErrorContains(t, err, `unknown command "innbcbft-secrets"`)

				entries, err := os.ReadDir(workDir)
				require.NoError(t, err)
				require.Empty(t, entries,
					"retired command created identity or configuration files")
			})
		}
	}
}

func TestINNBCBFTSecretsRetired_Help(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("json=%t", jsonOutput), func(t *testing.T) {
			root := NewRootCommand()

			var output bytes.Buffer
			root.baseCmd.SetOut(&output)
			root.baseCmd.SetErr(&output)

			args := []string{"--help"}
			if jsonOutput {
				args = append([]string{"--json"}, args...)
			}
			root.baseCmd.SetArgs(args)

			require.NoError(t, root.baseCmd.Execute())
			require.Contains(t, output.String(), "Available Commands:")
			require.NotContains(t, output.String(), "innbcbft-secrets")
		})
	}
}

/* FINISH ALPHA CHANGE 3255 -- verify the retired INNBCBFT identity command is absent and rejected -- */
