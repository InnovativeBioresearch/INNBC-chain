package status

import (
	"context"

	/* START ALPHA CHANGE 3347 -- propagate transaction pool status failure -- */
	"os"
	/* FINISH ALPHA CHANGE 3347 -- propagate transaction pool status failure -- */

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/spf13/cobra"

	txpoolOp "github.com/innovativebioresearch/innbc-chain/txpool/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

func GetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Returns the number of transactions in the transaction pool",
		/* START ALPHA CHANGE 3347 -- apply failure status after transaction pool status cleanup and output -- */
		Run: func(cmd *cobra.Command, args []string) {
			if exitCode := runCommand(cmd, args); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
		/* FINISH ALPHA CHANGE 3347 -- apply failure status after transaction pool status cleanup and output -- */
	}
}

/* START ALPHA CHANGE 3347 -- return transaction pool status and close its connection before output -- */
func runCommand(cmd *cobra.Command, _ []string) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	statusResponse, err := getTxPoolStatus(helper.GetGRPCAddress(cmd))
	if err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(&TxPoolStatusResult{
		Transactions: statusResponse.Length,
	})

	return 0
}

func getTxPoolStatus(grpcAddress string) (*txpoolOp.TxnPoolStatusResp, error) {
	conn, err := helper.GetGRPCConnection(grpcAddress)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := txpoolOp.NewTxnPoolOperatorClient(conn)

	return client.Status(context.Background(), &empty.Empty{})
}

/* FINISH ALPHA CHANGE 3347 -- return transaction pool status and close its connection before output -- */
