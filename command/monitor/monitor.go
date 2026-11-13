package monitor

/* START ALPHA CHANGE 3349 -- own monitor cancellation and preserve terminal RPC errors -- */
import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

/* FINISH ALPHA CHANGE 3349 -- own monitor cancellation and preserve terminal RPC errors -- */

func GetCommand() *cobra.Command {
	monitorCmd := &cobra.Command{
		Use:   "monitor",
		Short: "Starts logging block add / remove events on the blockchain",
		/* START ALPHA CHANGE 3349 -- apply monitor failure status after subscription cleanup -- */
		Run: func(cmd *cobra.Command, args []string) {
			if exitCode := runCommand(cmd, args); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
		/* FINISH ALPHA CHANGE 3349 -- apply monitor failure status after subscription cleanup -- */
	}

	helper.RegisterGRPCAddressFlag(monitorCmd)

	return monitorCmd
}

/* START ALPHA CHANGE 3349 -- receive monitor events synchronously and report terminal failure once -- */
func runCommand(cmd *cobra.Command, _ []string) int {
	outputter := command.InitializeOutputter(cmd)

	if err := subscribeToEvents(
		cmd.Context(),
		outputter,
		helper.GetGRPCAddress(cmd),
	); err != nil {
		// The subscription has already released its connection and signal
		// registration before either formatter can terminate the process.
		outputter.SetError(err)
		outputter.WriteOutput()

		return 1
	}

	return 0
}

func subscribeToEvents(
	parent context.Context,
	outputter command.OutputFormatter,
	grpcAddress string,
) error {
	ctx, stop := signal.NotifyContext(
		parent,
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)
	defer stop()

	conn, err := helper.GetGRPCConnection(grpcAddress)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := proto.NewSystemClient(conn)
	stream, err := client.Subscribe(ctx, &empty.Empty{})
	if err != nil {
		if ctx.Err() != nil &&
			(errors.Is(err, context.Canceled) ||
				grpcstatus.Code(err) == codes.Canceled) {
			return nil
		}

		return err
	}

	return runSubscribeLoop(ctx, stream, outputter)
}

func runSubscribeLoop(
	ctx context.Context,
	stream proto.System_SubscribeClient,
	outputter command.OutputFormatter,
) error {
	for {
		streamEvent, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			// Only cancellation of our own context is intentional.
			// A remote Canceled status with a live context remains a failure.
			if ctx.Err() != nil &&
				(errors.Is(err, context.Canceled) ||
					grpcstatus.Code(err) == codes.Canceled) {
				return nil
			}

			return fmt.Errorf("failed to read event: %w", err)
		}

		// Write each event once without retaining it for final output.
		outputter.WriteCommandResult(NewBlockEventResult(streamEvent))
	}
}

/* FINISH ALPHA CHANGE 3349 -- receive monitor events synchronously and report terminal failure once -- */
