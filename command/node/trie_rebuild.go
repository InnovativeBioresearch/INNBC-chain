/* START ALPHA CHANGE 2114 -- expose explicit offline trie database rebuilding -- */
package node

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"

	"github.com/innovativebioresearch/innbc-chain/command"
	serverconfig "github.com/innovativebioresearch/innbc-chain/command/server/config"
	nodeserver "github.com/innovativebioresearch/innbc-chain/server"
)

type trieRebuildParams struct {
	chainPath     string
	dataDir       string
	mdbxMapSizeGB uint64
	/* START ALPHA CHANGE 3475 -- retain replacement trie page flag -- */
	mdbxPageSizeBytes serverconfig.MDBXPageSize
	/* FINISH ALPHA CHANGE 3475 -- retain replacement trie page flag -- */
}

type trieRebuildCommandResult struct {
	*nodeserver.TrieRebuildResult
}

func (r *trieRebuildCommandResult) GetOutput() string {
	return fmt.Sprintf(
		"\n[TRIE DATABASE REBUILT]\n"+
			"Block: %d\nBlock hash: %s\nState root: %s\nTrie: %s\n",
		r.BlockNumber,
		r.BlockHash,
		r.StateRoot,
		r.TriePath,
	)
}

func getTrieRebuildCommand() *cobra.Command {
	params := &trieRebuildParams{
		chainPath:     defaultChainPath,
		mdbxMapSizeGB: serverconfig.DefaultMDBXMapSizeGB,
	}

	cmd := &cobra.Command{
		Use:   "trie-rebuild",
		Short: "Rebuilds an offline full-history node's empty trie database",
		Long: "Replays the local canonical block history from genesis into a new trie database.\n" +
			"The node and all other database maintenance processes MUST remain stopped.\n" +
			"The target trie directory must be absent or completely empty.\n" +
			"No process lock is acquired. Blockchain, consensus data, and keys are preserved.",
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			if status := runTrieRebuildCommand(cmd, params); status != 0 {
				os.Exit(status)
			}
		},
	}

	cmd.Flags().StringVar(
		&params.chainPath,
		"chain",
		params.chainPath,
		"the original chain genesis configuration",
	)
	cmd.Flags().StringVar(
		&params.dataDir,
		"data-dir",
		"",
		"the offline full-history node data directory",
	)
	cmd.Flags().Uint64Var(
		&params.mdbxMapSizeGB,
		"mdbx-map-size-gb",
		params.mdbxMapSizeGB,
		"the replacement trie MDBX map size in GiB",
	)
	/* START ALPHA CHANGE 3475 -- expose replacement trie creation pages -- */
	cmd.Flags().Var(
		&params.mdbxPageSizeBytes,
		"mdbx-page-size-bytes",
		"replacement trie page size in bytes; blank selects 4096; must match blockchain storage",
	)
	/* FINISH ALPHA CHANGE 3475 -- expose replacement trie creation pages -- */

	return cmd
}

func runTrieRebuildCommand(cmd *cobra.Command, params *trieRebuildParams) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ctx, cancel := signal.NotifyContext(
		cmd.Context(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	result, err := runTrieRebuild(ctx, params)
	if err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(&trieRebuildCommandResult{
		TrieRebuildResult: result,
	})

	return 0
}

func runTrieRebuild(
	ctx context.Context,
	params *trieRebuildParams,
) (*nodeserver.TrieRebuildResult, error) {
	if params.dataDir == "" {
		return nil, fmt.Errorf("--data-dir is required")
	}

	return nodeserver.RebuildTrieDatabase(ctx, nodeserver.TrieRebuildConfig{
		ChainPath:     params.chainPath,
		DataDir:       params.dataDir,
		MDBXMapSizeGB: params.mdbxMapSizeGB,
		/* START ALPHA CHANGE 3475 -- forward replacement trie creation pages -- */
		MDBXPageSizeBytes: uint64(params.mdbxPageSizeBytes),
		/* FINISH ALPHA CHANGE 3475 -- forward replacement trie creation pages -- */
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   "trie-rebuild",
			Level:  hclog.Info,
			Output: os.Stderr,
		}),
	})
}

/* FINISH ALPHA CHANGE 2114 -- expose explicit offline trie database rebuilding -- */
