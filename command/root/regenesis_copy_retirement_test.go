/* START ALPHA CHANGE 3260 -- verify retired trie-copy flags cannot open databases and diagnostics still route -- */
package root

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRegenesisCopyRetired_CommandRouting(t *testing.T) {
	workDir := t.TempDir()
	source := filepath.Join(workDir, "source")
	target := filepath.Join(workDir, "target")
	chain := filepath.Join(workDir, "blockchain")
	rootHash := "0x" + strings.Repeat("1", 64)
	setupBoundary := errors.New("retained diagnostic setup reached")

	tests := []struct {
		name      string
		args      []string
		wantError string
		wantHelp  bool
		wantSetup string
	}{
		{
			name:     "bare_group",
			args:     []string{"regenesis"},
			wantHelp: true,
		},
		{
			name:     "group_help",
			args:     []string{"regenesis", "--help"},
			wantHelp: true,
		},
		{
			name:      "unknown_subcommand",
			args:      []string{"regenesis", "copy"},
			wantError: `unknown command "copy"`,
		},
		{
			name:      "retired_source_flag",
			args:      []string{"regenesis", "--source-path", source},
			wantError: "unknown flag: --source-path",
		},
		{
			name:      "retired_target_flag",
			args:      []string{"regenesis", "--target-path", target},
			wantError: "unknown flag: --target-path",
		},
		{
			name:      "retired_root_flag",
			args:      []string{"regenesis", "--stateRoot", rootHash},
			wantError: "unknown flag: --stateRoot",
		},
		{
			name: "complete_retired_invocation",
			args: []string{
				"regenesis",
				"--source-path", source,
				"--target-path", target,
				"--stateRoot", rootHash,
			},
			wantError: "unknown flag: --source-path",
		},
		{
			name: "copy_flag_under_getroot",
			args: []string{
				"regenesis", "getroot", "--stateRoot", rootHash,
			},
			wantError: "unknown flag: --stateRoot",
		},
		{
			name: "copy_flag_under_history",
			args: []string{
				"regenesis", "history", "--target-path", target,
			},
			wantError: "unknown flag: --target-path",
		},
		{
			name: "retained_getroot",
			args: []string{
				"regenesis", "getroot",
				"--rpc", "http://127.0.0.1:1",
				"--block", "0",
			},
			wantSetup: "getroot",
		},
		/* START ALPHA CHANGE 3263 -- reject retired history invocations before diagnostic setup -- */
		{
			name:      "retired_history",
			args:      []string{"regenesis", "history"},
			wantError: `unknown command "history"`,
		},
		{
			name: "retired_history_flags",
			args: []string{
				"regenesis", "history",
				"--triedb", source,
				"--chaindb", chain,
				"--from", "0",
				"--to", "1",
			},
			wantError: "unknown flag: --triedb",
		},
		{
			name: "history_flags_under_parent",
			args: []string{
				"regenesis", "--triedb", source,
			},
			wantError: "unknown flag: --triedb",
		},
		{
			name: "history_flags_under_getroot",
			args: []string{
				"regenesis", "getroot", "--triedb", source,
			},
			wantError: "unknown flag: --triedb",
		},
		/* FINISH ALPHA CHANGE 3263 -- reject retired history invocations before diagnostic setup -- */
	}

	for _, jsonOutput := range []bool{false, true} {
		for _, tc := range tests {
			t.Run(fmt.Sprintf("%s/json=%t", tc.name, jsonOutput), func(t *testing.T) {
				root := NewRootCommand()

				group, remaining, err := root.baseCmd.Find([]string{"regenesis"})
				require.NoError(t, err)
				require.Empty(t, remaining)
				require.Equal(t, "regenesis", group.Name())
				require.Nil(t, group.Run)
				require.NotNil(t, group.RunE)

				/* START ALPHA CHANGE 3263 -- verify copy and history flags are absent from the group -- */
				for _, flag := range []string{
					"source-path", "target-path", "stateRoot",
					"triedb", "chaindb", "from", "to",
				} {
					require.Nil(t, group.Flags().Lookup(flag))
				}
				/* FINISH ALPHA CHANGE 3263 -- verify copy and history flags are absent from the group -- */

				var childNames []string
				for _, child := range group.Commands() {
					childNames = append(childNames, child.Name())
				}
				/* START ALPHA CHANGE 3263 -- retain only state-root lookup in the command group -- */
				require.ElementsMatch(t, []string{"getroot"}, childNames)
				/* FINISH ALPHA CHANGE 3263 -- retain only state-root lookup in the command group -- */

				setupCalls := 0
				setupCommand := ""
				runCalls := 0

				/* START ALPHA CHANGE 3263 -- preserve the root lookup routing control without history setup -- */
				// Keep real flag parsing, but stop the retained diagnostic
				// before any RPC request.
				for _, name := range []string{"getroot"} {
					child, remaining, err := group.Find([]string{name})
					require.NoError(t, err)
					require.Empty(t, remaining)
					require.Equal(t, name, child.Name())

					child.PreRunE = func(*cobra.Command, []string) error {
						setupCalls++
						setupCommand = name
						return setupBoundary
					}
					child.Run = nil
					child.RunE = func(*cobra.Command, []string) error {
						runCalls++
						return errors.New("unexpected diagnostic execution")
					}
				}
				/* FINISH ALPHA CHANGE 3263 -- preserve the root lookup routing control without history setup -- */

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
				switch {
				case tc.wantHelp:
					require.NoError(t, err)
					require.Zero(t, setupCalls)
					/* START ALPHA CHANGE 3263 -- exclude retired history commands and flags from group help -- */
					require.Contains(t, output.String(), "getroot")
					require.NotContains(t, output.String(), "history")
					for _, flag := range []string{
						"--source-path", "--target-path", "--stateRoot",
						"--triedb", "--chaindb", "--from", "--to",
					} {
						require.NotContains(t, output.String(), flag)
					}
					/* FINISH ALPHA CHANGE 3263 -- exclude retired history commands and flags from group help -- */
				case tc.wantSetup != "":
					require.ErrorIs(t, err, setupBoundary)
					require.Equal(t, 1, setupCalls)
					require.Equal(t, tc.wantSetup, setupCommand)
				default:
					require.ErrorContains(t, err, tc.wantError)
					require.NotErrorIs(t, err, setupBoundary)
					require.Zero(t, setupCalls)
				}
				require.Zero(t, runCalls)

				entries, err := os.ReadDir(workDir)
				require.NoError(t, err)
				require.Empty(t, entries,
					"command routing created source, destination or blockchain data")
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3260 -- verify retired trie-copy flags cannot open databases and diagnostics still route -- */
