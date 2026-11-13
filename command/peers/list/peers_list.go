package list

import (
	"context"

	/* START ALPHA CHANGE 3345 -- propagate peers list execution failure -- */
	"os"
	/* FINISH ALPHA CHANGE 3345 -- propagate peers list execution failure -- */

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
	"github.com/spf13/cobra"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

func GetCommand() *cobra.Command {
	peersListCmd := &cobra.Command{
		Use:   "list",
		Short: "Returns the list of connected peers, including the current node",
		/* START ALPHA CHANGE 3345 -- apply failure status after peers list cleanup and output -- */
		Run: func(cmd *cobra.Command, args []string) {
			if exitCode := runCommand(cmd, args); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
		/* FINISH ALPHA CHANGE 3345 -- apply failure status after peers list cleanup and output -- */
	}

	return peersListCmd
}

/* START ALPHA CHANGE 3345 -- return peers list status and close its connection before output -- */
func runCommand(cmd *cobra.Command, _ []string) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	peersList, err := getPeersList(helper.GetGRPCAddress(cmd))
	if err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(
		newPeersListResult(peersList.Peers),
	)

	return 0
}

func getPeersList(grpcAddress string) (*proto.PeersListResponse, error) {
	conn, err := helper.GetGRPCConnection(grpcAddress)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := proto.NewSystemClient(conn)

	return client.PeersList(context.Background(), &empty.Empty{})
}

/* FINISH ALPHA CHANGE 3345 -- return peers list status and close its connection before output -- */
