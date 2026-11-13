/* START ALPHA CHANGE 3278 -- reject the retired CLI wrapper while preserving active consensus selection -- */
package root

import (
	"bytes"
	"fmt"
	"testing"

	runtimeserver "github.com/innovativebioresearch/innbc-chain/server"
	"github.com/stretchr/testify/require"
)

func TestINNBCBFTWrapperRetired_CommandRouting(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		for _, explicitHelp := range []bool{false, true} {
			t.Run(fmt.Sprintf(
				"help=%t/json=%t", explicitHelp, jsonOutput,
			), func(t *testing.T) {
				root := NewRootCommand()

				for _, child := range root.baseCmd.Commands() {
					require.NotEqual(t, "innbcbft", child.Name())
					require.NotContains(t, child.Aliases, "innbcbft")
				}

				var output bytes.Buffer
				root.baseCmd.SetOut(&output)
				root.baseCmd.SetErr(&output)
				root.baseCmd.SilenceErrors = true
				root.baseCmd.SilenceUsage = true

				args := []string{"innbcbft"}
				if explicitHelp {
					args = append(args, "--help")
				}
				if jsonOutput {
					args = append([]string{"--json"}, args...)
				}
				root.baseCmd.SetArgs(args)

				err := root.baseCmd.Execute()
				require.ErrorContains(t, err, `unknown command "innbcbft"`)
			})
		}
	}
}

func TestINNBCBFTWrapperRetired_RetainsConsensusAndCommands(t *testing.T) {
	require.True(t, runtimeserver.ConsensusSupported("innbcbft"))

	root := NewRootCommand()
	for _, path := range [][]string{
		{"genesis"},
		{"server"},
		{"node", "create"},
		{"node", "info"},
	} {
		cmd, remaining, err := root.baseCmd.Find(path)
		require.NoError(t, err)
		require.Empty(t, remaining)
		require.Equal(t, path[len(path)-1], cmd.Name())
	}

	genesisCmd, remaining, err := root.baseCmd.Find([]string{"genesis"})
	require.NoError(t, err)
	require.Empty(t, remaining)

	consensusFlag := genesisCmd.Flags().Lookup("consensus")
	require.NotNil(t, consensusFlag)
	require.Equal(t, "innbcbft", consensusFlag.DefValue)
}

func TestINNBCBFTWrapperRetired_Help(t *testing.T) {
	root := NewRootCommand()

	var output bytes.Buffer
	root.baseCmd.SetOut(&output)
	root.baseCmd.SetErr(&output)
	root.baseCmd.SetArgs([]string{"--help"})

	require.NoError(t, root.baseCmd.Execute())
	require.NotContains(t, output.String(), "\n  innbcbft ")
}

/* FINISH ALPHA CHANGE 3278 -- reject the retired CLI wrapper while preserving active consensus selection -- */
