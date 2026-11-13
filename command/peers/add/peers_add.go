package add

import (
	/* START ALPHA CHANGE 3348 -- propagate complete and partial peer addition failures -- */
	"os"
	/* FINISH ALPHA CHANGE 3348 -- propagate complete and partial peer addition failures -- */

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	peersAddCmd := &cobra.Command{
		Use:     "add",
		Short:   "Adds new peers to the peer list, using the peer's libp2p address",
		PreRunE: runPreRun,
		/* START ALPHA CHANGE 3348 -- apply peer addition status after cleanup and detailed output -- */
		Run: func(cmd *cobra.Command, args []string) {
			if exitCode := runCommand(cmd, args); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
		/* FINISH ALPHA CHANGE 3348 -- apply peer addition status after cleanup and detailed output -- */
	}

	setFlags(peersAddCmd)
	helper.SetRequiredFlags(peersAddCmd, params.getRequiredFlags())

	return peersAddCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringArrayVar(
		&params.peerAddresses,
		addrFlag,
		[]string{},
		"the libp2p addresses of the peers",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

/* START ALPHA CHANGE 3348 -- preserve detailed peer addition results while reporting failure -- */
func runCommand(cmd *cobra.Command, _ []string) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.addPeersAt(helper.GetGRPCAddress(cmd)); err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(params.getResult())

	if len(params.addErrors) > 0 {
		return 1
	}

	return 0
}

/* FINISH ALPHA CHANGE 3348 -- preserve detailed peer addition results while reporting failure -- */
