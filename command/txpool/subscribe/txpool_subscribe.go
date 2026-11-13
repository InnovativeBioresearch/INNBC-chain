package subscribe

/* START ALPHA CHANGE 3350 -- own transaction pool subscription cancellation and terminal errors -- */
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
	txpoolProto "github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

/* FINISH ALPHA CHANGE 3350 -- own transaction pool subscription cancellation and terminal errors -- */

func GetCommand() *cobra.Command {
	txPoolSubscribeCmd := &cobra.Command{
		Use:   "subscribe",
		Short: "Logs specific TxPool events",
		/* START ALPHA CHANGE 3350 -- apply transaction pool subscription failure after cleanup -- */
		Run: func(cmd *cobra.Command, args []string) {
			if exitCode := runCommand(cmd, args); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
		/* FINISH ALPHA CHANGE 3350 -- apply transaction pool subscription failure after cleanup -- */
	}

	setFlags(txPoolSubscribeCmd)

	return txPoolSubscribeCmd
}

func setFlags(cmd *cobra.Command) {
	params.initEventMap()

	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_ADDED],
		addedFlag,
		false,
		"should subscribe to added events",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_PROMOTED],
		promotedFlag,
		false,
		"should subscribe to promoted events",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_ENQUEUED],
		enqueuedFlag,
		false,
		"should subscribe to enqueued events",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_DROPPED],
		droppedFlag,
		false,
		"should subscribe to dropped events",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_DEMOTED],
		demotedFlag,
		false,
		"should subscribe to demoted events",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_PRUNED_PROMOTED],
		prunedPromotedFlag,
		false,
		"should subscribe to pruned promoted tx events in the TxPool",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_PRUNED_ENQUEUED],
		prunedEnqueuedFlag,
		false,
		"should subscribe to pruned enqueued tx events in the TxPool",
	)
}

/* START ALPHA CHANGE 3350 -- receive transaction pool events synchronously and report terminal failure once -- */
func runCommand(cmd *cobra.Command, _ []string) int {
	outputter := command.InitializeOutputter(cmd)

	params.init()

	if err := subscribeToEvents(
		cmd.Context(),
		outputter,
		&txpoolProto.SubscribeRequest{
			Types: params.supportedEvents,
		},
		helper.GetGRPCAddress(cmd),
	); err != nil {
		// Subscription resources have been released before final output.
		outputter.SetError(err)
		outputter.WriteOutput()

		return 1
	}

	return 0
}

func subscribeToEvents(
	parent context.Context,
	outputter command.OutputFormatter,
	subscribeRequest *txpoolProto.SubscribeRequest,
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

	client := txpoolProto.NewTxnPoolOperatorClient(conn)
	stream, err := client.Subscribe(ctx, subscribeRequest)
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
	stream txpoolProto.TxnPoolOperator_SubscribeClient,
	outputter command.OutputFormatter,
) error {
	for {
		streamEvent, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			if ctx.Err() != nil &&
				(errors.Is(err, context.Canceled) ||
					grpcstatus.Code(err) == codes.Canceled) {
				return nil
			}

			return fmt.Errorf("failed to read event: %w", err)
		}

		outputter.WriteCommandResult(&TxPoolEventResult{
			EventType: streamEvent.Type,
			TxHash:    streamEvent.TxHash,
		})
	}
}

/* FINISH ALPHA CHANGE 3350 -- receive transaction pool events synchronously and report terminal failure once -- */
