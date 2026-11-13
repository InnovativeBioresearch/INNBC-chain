/* START ALPHA CHANGE 3200 -- reject unavailable blocks and unsupported legacy root selectors without panics -- */
package regenesis

import (
	"fmt"
	"io"
	"math"

	/* START ALPHA CHANGE 3301 -- report root lookup failures after client cleanup and output -- */
	"os"
	/* FINISH ALPHA CHANGE 3301 -- report root lookup failures after client cleanup and output -- */

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
)

/*
innbc-chain regenesis getroot --rpc "http://localhost:10002"
*/
/* START ALPHA CHANGE 3301 -- report root lookup failures after client cleanup and output -- */
func GetRootCMD() *cobra.Command {
	return newGetRootCommand(command.InitializeOutputter, os.Exit)
}

// The public command uses the real formatter and process exit.
// Tests inject both functions to inspect failures inside the test process.
func newGetRootCommand(
	initializeOutputter func(*cobra.Command) command.OutputFormatter,
	exit func(int),
) *cobra.Command {
	/* FINISH ALPHA CHANGE 3301 -- report root lookup failures after client cleanup and output -- */
	var (
		jsonRPCAddress string
		blockNumber    int64
	)

	getRootCmd := &cobra.Command{
		Use:   "getroot",
		Short: "returns state root of old chain",
	}
	/* START ALPHA CHANGE 3301 -- report root lookup failures after client cleanup and output -- */
	run := func(cmd *cobra.Command) int {
		outputter := initializeOutputter(cmd)
		defer outputter.WriteOutput()

		// runLegacyGetRoot closes its client before output is flushed.
		if err := runLegacyGetRoot(jsonRPCAddress, blockNumber, outputter); err != nil {
			outputter.SetError(err)

			return 1
		}

		return 0
	}

	getRootCmd.Run = func(cmd *cobra.Command, _ []string) {
		if exitCode := run(cmd); exitCode != 0 {
			exit(exitCode)
		}
	}
	/* FINISH ALPHA CHANGE 3301 -- report root lookup failures after client cleanup and output -- */

	getRootCmd.Flags().StringVar(
		&jsonRPCAddress,
		"rpc",
		"",
		"the JSON RPC IP address for old chain",
	)
	getRootCmd.Flags().Int64Var(
		&blockNumber,
		"block",
		int64(ethgo.Latest),
		"Block number of trie snapshot",
	)

	return getRootCmd
}

func legacyRootBlockSelector(number int64) (ethgo.BlockNumber, error) {
	switch number {
	case int64(ethgo.Latest), int64(ethgo.Earliest), int64(ethgo.Pending):
		return ethgo.BlockNumber(number), nil
	}

	if number < 0 {
		return 0, fmt.Errorf(
			"invalid --block %d: negative value is not a supported block tag",
			number,
		)
	}

	// The pinned client's BlockNumber is int, while the CLI accepts int64.
	if number > int64(math.MaxInt) {
		return 0, fmt.Errorf(
			"invalid --block %d: exceeds client maximum %d",
			number,
			math.MaxInt,
		)
	}

	return ethgo.BlockNumber(number), nil
}

func runLegacyGetRoot(address string, number int64, output io.Writer) error {
	selector, err := legacyRootBlockSelector(number)
	if err != nil {
		return err
	}

	rpcClient, err := jsonrpc.NewClient(address)
	if err != nil {
		return fmt.Errorf("connect to client: %w", err)
	}
	defer rpcClient.Close()

	block, err := rpcClient.Eth().GetBlockByNumber(selector, false)
	if err != nil {
		return fmt.Errorf("get block %s: %w", selector.String(), err)
	}
	if block == nil {
		return fmt.Errorf("block unavailable: %s", selector.String())
	}

	/* START ALPHA CHANGE 749 -- use fmt.Appendf to avoid temporary formatted string allocation -- */
	data := fmt.Appendf(
		nil, "state root %s for block %d\n", block.StateRoot, block.Number,
	)
	/* FINISH ALPHA CHANGE 749 -- use fmt.Appendf to avoid temporary formatted string allocation -- */

	written, err := output.Write(data)
	if err != nil {
		return fmt.Errorf("write state root: %w", err)
	}
	if written != len(data) {
		return fmt.Errorf("write state root: %w", io.ErrShortWrite)
	}

	return nil
}

/* FINISH ALPHA CHANGE 3200 -- reject unavailable blocks and unsupported legacy root selectors without panics -- */
