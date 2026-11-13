package status

import (
	"context"

	/* START ALPHA CHANGE 3303 -- close status connections before reporting execution failure -- */
	"os"
	/* FINISH ALPHA CHANGE 3303 -- close status connections before reporting execution failure -- */

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/spf13/cobra"

	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
)

func GetCommand() *cobra.Command {
	statusCmd := &cobra.Command{
		Use: "status",
		/*START ALPHA CHANGE 7 -- rebrand status command short description --*/
		Short: "Returns the status of the INNBC Chain client",
		/*FINISH ALPHA CHANGE 7 -- rebrand status command short description --*/
		Args: cobra.NoArgs,
		/* START ALPHA CHANGE 3303 -- close status connections before reporting execution failure -- */
		Run: func(cmd *cobra.Command, args []string) {
			if exitCode := runCommand(cmd, args); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
		/* FINISH ALPHA CHANGE 3303 -- close status connections before reporting execution failure -- */
	}

	helper.RegisterGRPCAddressFlag(statusCmd)

	return statusCmd
}

/* START ALPHA CHANGE 3303 -- close status connections before reporting execution failure -- */
func runCommand(cmd *cobra.Command, _ []string) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	statusResponse, err := getSystemStatus(helper.GetGRPCAddress(cmd))
	if err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(&StatusResult{
		ChainID:            statusResponse.Network,
		CurrentBlockNumber: statusResponse.Current.Number,
		CurrentBlockHash:   statusResponse.Current.Hash,
		LibP2PAddress:      statusResponse.P2PAddr,
	})

	return 0
}

func getSystemStatus(grpcAddress string) (*proto.ServerStatus, error) {
	conn, err := helper.GetGRPCConnection(grpcAddress)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := proto.NewSystemClient(conn)

	return client.GetStatus(context.Background(), &empty.Empty{})
}

/* FINISH ALPHA CHANGE 3303 -- close status connections before reporting execution failure -- */
