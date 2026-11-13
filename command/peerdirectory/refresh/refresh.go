package refresh

import (
	"context"
	/* START ALPHA CHANGE 3526 -- propagate refresh execution failure after cleanup and output -- */
	"os"
	/* FINISH ALPHA CHANGE 3526 -- propagate refresh execution failure after cleanup and output -- */

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
	"github.com/spf13/cobra"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

/* START ALPHA CHANGE 1223 -- add manual peer directory refresh command -- */
func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Runs one peer-directory refresh from the running node",
		Args:  cobra.NoArgs,
		/* START ALPHA CHANGE 3526 -- apply refresh failure status after the execution helper returns -- */
		Run: func(cmd *cobra.Command, args []string) {
			if exitCode := runCommand(cmd, args); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
		/* FINISH ALPHA CHANGE 3526 -- apply refresh failure status after the execution helper returns -- */
	}

	helper.RegisterGRPCAddressFlag(cmd)

	return cmd
}

/* START ALPHA CHANGE 3526 -- return refresh status and release its connection before final output -- */
func runCommand(cmd *cobra.Command, _ []string) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	result, err := refreshPeerDirectory(helper.GetGRPCAddress(cmd))
	if err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(result)

	return 0
}

func refreshPeerDirectory(grpcAddress string) (*PeerDirectoryRefreshResult, error) {
	conn, err := helper.GetGRPCConnection(grpcAddress)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := proto.NewSystemClient(conn)

	response, err := client.RefreshPeerDirectory(context.Background(), &empty.Empty{})
	if err != nil {
		return nil, err
	}

	return peerDirectoryRefreshResultFromProto(response), nil
}

/* FINISH ALPHA CHANGE 3526 -- return refresh status and release its connection before final output -- */

func peerDirectoryRefreshResultFromProto(response *proto.RefreshPeerDirectoryResponse) *PeerDirectoryRefreshResult {
	return &PeerDirectoryRefreshResult{
		Peers: response.Peers,
	}
}

/* FINISH ALPHA CHANGE 1223 -- add manual peer directory refresh command -- */
