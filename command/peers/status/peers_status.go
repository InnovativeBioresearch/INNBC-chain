package status

import (
	/* START ALPHA CHANGE 3346 -- propagate peer status execution failure -- */
	"os"
	/* FINISH ALPHA CHANGE 3346 -- propagate peer status execution failure -- */

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	peersStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Returns the status of the specified peer, using the libp2p ID of the peer node",
		/* START ALPHA CHANGE 3346 -- apply failure status after peer status cleanup and output -- */
		Run: func(cmd *cobra.Command, args []string) {
			if exitCode := runCommand(cmd, args); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
		/* FINISH ALPHA CHANGE 3346 -- apply failure status after peer status cleanup and output -- */
	}

	setFlags(peersStatusCmd)
	helper.SetRequiredFlags(peersStatusCmd, params.getRequiredFlags())

	return peersStatusCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.peerID,
		peerIDFlag,
		"",
		"libp2p node ID of a specific peer within p2p network",
	)
}

/* START ALPHA CHANGE 3346 -- return peer status execution outcome -- */
func runCommand(cmd *cobra.Command, _ []string) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.initPeerInfo(helper.GetGRPCAddress(cmd)); err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(params.getResult())

	return 0
}

/* FINISH ALPHA CHANGE 3346 -- return peer status execution outcome -- */
