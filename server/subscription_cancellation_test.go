/* START ALPHA CHANGE 3002 -- verify operator subscription cancellation and transport cleanup -- */
package server

import (
	"context"
	"errors"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	gproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const operatorSubscribeTestTimeout = 5 * time.Second

type operatorSubscribeRun struct {
	result   chan error
	finished chan struct{}
}

func newOperatorSubscribeRun() *operatorSubscribeRun {
	return &operatorSubscribeRun{
		result:   make(chan error, 1),
		finished: make(chan struct{}),
	}
}

func operatorSubscribeAwait[T any](
	t *testing.T,
	ch <-chan T,
	description string,
) T {
	t.Helper()

	select {
	case value := <-ch:
		return value
	case <-time.After(operatorSubscribeTestTimeout):
		t.Fatalf("timed out waiting for %s", description)
		var zero T
		return zero
	}
}

// Observe the actual blocked goroutine, rather than treating handler entry or
// client-side cancellation as proof that the server reached the relevant wait.
// Stack storage is bounded. These tests intentionally do not run in parallel.
func operatorSubscribeBlocked(frames ...string) bool {
	size := 64 * 1024

	for size <= 2*1024*1024 {
		buffer := make([]byte, size)
		n := runtime.Stack(buffer, true)
		if n == len(buffer) {
			size *= 2
			continue
		}

		for _, goroutine := range strings.Split(string(buffer[:n]), "\n\n") {
			if !strings.Contains(goroutine, "[select") {
				continue
			}

			matches := true
			for _, frame := range frames {
				if !strings.Contains(goroutine, frame) {
					matches = false
					break
				}
			}

			if matches {
				return true
			}
		}

		return false
	}

	return false
}

func operatorSubscribeRequireIdle(t *testing.T, count func() int) {
	t.Helper()

	require.Eventually(t, func() bool {
		return count() == 1 &&
			operatorSubscribeBlocked("(*systemService).Subscribe")
	}, operatorSubscribeTestTimeout, time.Millisecond,
		"handler did not reach its idle subscription wait")
}

func operatorSubscribeRequireFinished(
	t *testing.T,
	run *operatorSubscribeRun,
	count func() int,
) error {
	t.Helper()

	err := operatorSubscribeAwait(t, run.result, "handler result")
	operatorSubscribeAwait(t, run.finished, "handler completion")
	require.Zero(t, count(), "subscription remains registered after handler return")

	return err
}

func operatorSubscribeEvent() *blockchain.Event {
	return &blockchain.Event{
		NewChain: []*types.Header{
			{
				Number: 1,
				Hash:   types.StringToHash("0x01"),
			},
		},
	}
}

type operatorSubscribeUnitStream struct {
	proto.System_SubscribeServer
	ctx  context.Context
	send func(*proto.BlockchainEvent) error
}

func (s *operatorSubscribeUnitStream) Context() context.Context {
	return s.ctx
}

func (s *operatorSubscribeUnitStream) Send(event *proto.BlockchainEvent) error {
	return s.send(event)
}

func operatorSubscribeStartUnit(
	t *testing.T,
	ctx context.Context,
	cancel context.CancelFunc,
	send func(*proto.BlockchainEvent) error,
) (*operatorSubscribeRun, func(*blockchain.Event), func() int) {
	t.Helper()

	bc, publish, count := blockchain.NewTestSubscriptionBlockchain()
	service := &systemService{server: &Server{blockchain: bc}}
	run := newOperatorSubscribeRun()
	stream := &operatorSubscribeUnitStream{
		ctx:  ctx,
		send: send,
	}

	t.Cleanup(func() {
		cancel()

		// This also wakes the original GetEvent implementation if a regression
		// causes the cancellation assertion to fail.
		publish(nil)

		select {
		case <-run.finished:
		case <-time.After(operatorSubscribeTestTimeout):
			t.Error("unit subscription handler did not finish during cleanup")
		}

		if active := count(); active != 0 {
			t.Errorf("unit cleanup retained %d subscriptions", active)
		}
	})

	go func() {
		defer close(run.finished)
		run.result <- service.Subscribe(&emptypb.Empty{}, stream)
	}()

	return run, publish, count
}

func TestOperatorSubscribe_IdleCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sent := make(chan *proto.BlockchainEvent, 1)

	run, _, count := operatorSubscribeStartUnit(t, ctx, cancel,
		func(event *proto.BlockchainEvent) error {
			sent <- event
			return nil
		})

	operatorSubscribeRequireIdle(t, count)
	require.Empty(t, sent)

	// No event is published to release the handler.
	cancel()

	err := operatorSubscribeRequireFinished(t, run, count)
	require.Equal(t, codes.Canceled, status.Code(err))
	require.Empty(t, sent)
}

func TestOperatorSubscribe_ContextStatus(t *testing.T) {
	for _, tc := range []struct {
		name     string
		deadline bool
		code     codes.Code
	}{
		{"already_canceled", false, codes.Canceled},
		{"already_expired", true, codes.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				ctx    context.Context
				cancel context.CancelFunc
			)

			if tc.deadline {
				ctx, cancel = context.WithDeadline(
					context.Background(),
					time.Now().Add(-time.Second),
				)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}

			sent := make(chan *proto.BlockchainEvent, 1)
			run, _, count := operatorSubscribeStartUnit(t, ctx, cancel,
				func(event *proto.BlockchainEvent) error {
					sent <- event
					return nil
				})

			err := operatorSubscribeRequireFinished(t, run, count)
			require.Equal(t, tc.code, status.Code(err))
			require.Empty(t, sent)
		})
	}
}

func TestOperatorSubscribe_NormalDeliveryAndContinuedIdle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sent := make(chan *proto.BlockchainEvent, 1)

	run, publish, count := operatorSubscribeStartUnit(t, ctx, cancel,
		func(event *proto.BlockchainEvent) error {
			sent <- event
			return nil
		})

	operatorSubscribeRequireIdle(t, count)

	event := operatorSubscribeEvent()
	publish(event)

	got := operatorSubscribeAwait(t, sent, "normal event delivery")
	require.Len(t, got.Added, 1)
	require.Equal(t, int64(1), got.Added[0].Number)
	require.Equal(t, event.NewChain[0].Hash.String(), got.Added[0].Hash)
	require.Empty(t, got.Removed)

	// Delivery does not terminate a healthy subscription.
	operatorSubscribeRequireIdle(t, count)
	select {
	case <-run.finished:
		t.Fatal("healthy subscription ended after normal delivery")
	default:
	}

	cancel()
	err := operatorSubscribeRequireFinished(t, run, count)
	require.Equal(t, codes.Canceled, status.Code(err))
}

func TestOperatorSubscribe_SendErrorAndNilEvent(t *testing.T) {
	t.Run("send_error_returned_directly", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		sendErr := errors.New("injected subscription send failure")

		run, publish, count := operatorSubscribeStartUnit(t, ctx, cancel,
			func(*proto.BlockchainEvent) error {
				return sendErr
			})

		operatorSubscribeRequireIdle(t, count)
		publish(operatorSubscribeEvent())

		err := operatorSubscribeRequireFinished(t, run, count)
		require.True(t, err == sendErr, "handler did not return the original Send error")
	})

	t.Run("nil_event_completes", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		sent := make(chan *proto.BlockchainEvent, 1)

		run, publish, count := operatorSubscribeStartUnit(t, ctx, cancel,
			func(event *proto.BlockchainEvent) error {
				sent <- event
				return nil
			})

		operatorSubscribeRequireIdle(t, count)
		publish(nil)

		require.NoError(t, operatorSubscribeRequireFinished(t, run, count))
		require.Empty(t, sent)
	})
}

func TestOperatorSubscribe_RepeatedCancellation(t *testing.T) {
	for i := 0; i < 25; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		run, _, count := operatorSubscribeStartUnit(t, ctx, cancel,
			func(*proto.BlockchainEvent) error { return nil })

		operatorSubscribeRequireIdle(t, count)
		cancel()

		err := operatorSubscribeRequireFinished(t, run, count)
		require.Equal(t, codes.Canceled, status.Code(err))
	}
}

func TestOperatorSubscribe_CancellationRacesWithPublication(t *testing.T) {
	for i := 0; i < 25; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		run, publish, count := operatorSubscribeStartUnit(t, ctx, cancel,
			func(*proto.BlockchainEvent) error { return nil })

		operatorSubscribeRequireIdle(t, count)

		start := make(chan struct{})
		published := make(chan struct{})
		canceled := make(chan struct{})

		go func() {
			defer close(published)
			<-start
			publish(operatorSubscribeEvent())
		}()

		go func() {
			defer close(canceled)
			<-start
			cancel()
		}()

		close(start)
		operatorSubscribeAwait(t, published, "racing publisher completion")
		operatorSubscribeAwait(t, canceled, "racing cancellation completion")

		err := operatorSubscribeRequireFinished(t, run, count)
		require.Equal(t, codes.Canceled, status.Code(err))
	}
}

type operatorSubscribeRPCStream struct {
	proto.System_SubscribeServer
	sendEntered  chan *proto.BlockchainEvent
	sendReturned chan error
}

func (s *operatorSubscribeRPCStream) Send(event *proto.BlockchainEvent) error {
	s.sendEntered <- event
	err := s.System_SubscribeServer.Send(event)
	s.sendReturned <- err
	return err
}

type operatorSubscribeRPCService struct {
	*systemService
	run          *operatorSubscribeRun
	entered      chan struct{}
	sendEntered  chan *proto.BlockchainEvent
	sendReturned chan error
}

func (s *operatorSubscribeRPCService) Subscribe(
	req *emptypb.Empty,
	stream proto.System_SubscribeServer,
) error {
	close(s.entered)
	defer close(s.run.finished)

	err := s.systemService.Subscribe(req, &operatorSubscribeRPCStream{
		System_SubscribeServer: stream,
		sendEntered:            s.sendEntered,
		sendReturned:           s.sendReturned,
	})
	s.run.result <- err

	return err
}

type operatorSubscribeRPCFixture struct {
	run          *operatorSubscribeRun
	publish      func(*blockchain.Event)
	count        func() int
	cancel       context.CancelFunc
	conn         *grpc.ClientConn
	server       *grpc.Server
	stream       proto.System_SubscribeClient
	sendEntered  chan *proto.BlockchainEvent
	sendReturned chan error
}

func operatorSubscribeNewRPCFixture(t *testing.T) *operatorSubscribeRPCFixture {
	t.Helper()

	bc, publish, count := blockchain.NewTestSubscriptionBlockchain()
	service := &operatorSubscribeRPCService{
		systemService: &systemService{server: &Server{blockchain: bc}},
		run:           newOperatorSubscribeRun(),
		entered:       make(chan struct{}),
		sendEntered:   make(chan *proto.BlockchainEvent, 4),
		sendReturned:  make(chan error, 4),
	}

	listener := bufconn.Listen(1024 * 1024)
	rpcServer := grpc.NewServer()
	proto.RegisterSystemServer(rpcServer, service)

	ctx, cancel := context.WithCancel(context.Background())
	fixture := &operatorSubscribeRPCFixture{
		run:          service.run,
		publish:      publish,
		count:        count,
		cancel:       cancel,
		server:       rpcServer,
		sendEntered:  service.sendEntered,
		sendReturned: service.sendReturned,
	}
	serveDone := make(chan struct{})

	t.Cleanup(func() {
		cancel()
		if fixture.conn != nil {
			_ = fixture.conn.Close()
		}

		// Failing tests against the old idle handler still have an escape path.
		publish(nil)
		rpcServer.Stop()
		_ = listener.Close()

		select {
		case <-service.entered:
			select {
			case <-service.run.finished:
			case <-time.After(operatorSubscribeTestTimeout):
				t.Error("transport subscription handler did not finish during cleanup")
			}
		default:
		}

		select {
		case <-serveDone:
		case <-time.After(operatorSubscribeTestTimeout):
			t.Error("subscription test gRPC server did not stop")
		}

		if active := count(); active != 0 {
			t.Errorf("transport cleanup retained %d subscriptions", active)
		}
	})

	go func() {
		defer close(serveDone)
		_ = rpcServer.Serve(listener)
	}()

	setupCtx, cancelSetup := context.WithTimeout(
		context.Background(),
		operatorSubscribeTestTimeout,
	)
	defer cancelSetup()

	conn, err := grpc.DialContext(
		setupCtx,
		"passthrough:///operator-subscription",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		// Fixed receive windows make the non-reading client fixture bounded
		// and prevent dynamic window growth from masking backpressure.
		grpc.WithStaticStreamWindowSize(65535),
		grpc.WithStaticConnWindowSize(65535),
	)
	require.NoError(t, err)
	fixture.conn = conn

	fixture.stream, err = proto.NewSystemClient(conn).Subscribe(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	operatorSubscribeAwait(t, service.entered, "production RPC handler entry")

	require.Eventually(t, func() bool {
		return count() == 1 &&
			operatorSubscribeBlocked(
				"(*systemService).Subscribe",
				"(*operatorSubscribeRPCService).Subscribe",
			)
	}, operatorSubscribeTestTimeout, time.Millisecond,
		"production RPC handler did not reach its idle event wait")

	return fixture
}

func operatorSubscribeRecv(
	t *testing.T,
	fixture *operatorSubscribeRPCFixture,
) (*proto.BlockchainEvent, error) {
	t.Helper()

	type result struct {
		event *proto.BlockchainEvent
		err   error
	}

	done := make(chan result, 1)
	go func() {
		event, err := fixture.stream.Recv()
		done <- result{event: event, err: err}
	}()

	select {
	case received := <-done:
		return received.event, received.err
	case <-time.After(operatorSubscribeTestTimeout):
		fixture.cancel()
		select {
		case <-done:
		case <-time.After(operatorSubscribeTestTimeout):
			t.Error("client Recv did not finish after cancellation")
		}
		t.Fatal("client Recv exceeded the test deadline")
		return nil, nil
	}
}

func TestOperatorSubscribe_TransportIdle(t *testing.T) {
	for _, action := range []string{
		"client_cancellation",
		"connection_close",
		"forced_server_stop",
	} {
		t.Run(action, func(t *testing.T) {
			fixture := operatorSubscribeNewRPCFixture(t)
			require.Empty(t, fixture.sendEntered)

			// The fixture has already confirmed the actual idle handler wait.
			// No event is published before checking completion.
			switch action {
			case "client_cancellation":
				fixture.cancel()
			case "connection_close":
				require.NoError(t, fixture.conn.Close())
			case "forced_server_stop":
				fixture.server.Stop()
			}

			err := operatorSubscribeRequireFinished(t, fixture.run, fixture.count)
			require.Equal(t, codes.Canceled, status.Code(err))
			require.Empty(t, fixture.sendEntered)

			_, recvErr := operatorSubscribeRecv(t, fixture)
			require.Error(t, recvErr)
			if action == "client_cancellation" {
				require.Equal(t, codes.Canceled, status.Code(recvErr))
			}
		})
	}
}

func TestOperatorSubscribe_TransportNormalDelivery(t *testing.T) {
	fixture := operatorSubscribeNewRPCFixture(t)
	event := operatorSubscribeEvent()
	fixture.publish(event)

	got, err := operatorSubscribeRecv(t, fixture)
	require.NoError(t, err)
	require.Len(t, got.Added, 1)
	require.Equal(t, event.NewChain[0].Hash.String(), got.Added[0].Hash)
	require.Equal(t, int64(1), got.Added[0].Number)
	require.Empty(t, got.Removed)
	require.NoError(t,
		operatorSubscribeAwait(t, fixture.sendReturned, "successful transport send"))

	require.Eventually(t, func() bool {
		return fixture.count() == 1 &&
			operatorSubscribeBlocked(
				"(*systemService).Subscribe",
				"(*operatorSubscribeRPCService).Subscribe",
			)
	}, operatorSubscribeTestTimeout, time.Millisecond,
		"healthy RPC subscription did not resume its idle wait")

	fixture.cancel()
	err = operatorSubscribeRequireFinished(t, fixture.run, fixture.count)
	require.Equal(t, codes.Canceled, status.Code(err))
}

func TestOperatorSubscribe_TransportBackpressure(t *testing.T) {
	for _, action := range []string{
		"client_cancellation",
		"forced_server_stop",
	} {
		t.Run(action, func(t *testing.T) {
			fixture := operatorSubscribeNewRPCFixture(t)

			// A bounded synthetic notification exceeds the fixed receive
			// window and write quota. This tests event transport, not block
			// validity or consensus.
			header := operatorSubscribeEvent().NewChain[0]
			event := &blockchain.Event{
				NewChain: make([]*types.Header, 4096),
			}
			for i := range event.NewChain {
				event.NewChain[i] = header
			}

			// The first Send can enqueue its message and return. The client
			// deliberately does not call Recv, so its stream window remains
			// exhausted and the next Send must wait for write quota.
			fixture.publish(event)
			first := operatorSubscribeAwait(t, fixture.sendEntered, "first Send entry")
			require.Greater(t, gproto.Size(first), 4*65535)
			require.Less(t, gproto.Size(first), 1024*1024)
			require.NoError(t,
				operatorSubscribeAwait(t, fixture.sendReturned, "first Send return"))

			fixture.publish(event)
			operatorSubscribeAwait(t, fixture.sendEntered, "second Send entry")

			require.Eventually(t, func() bool {
				return fixture.count() == 1 &&
					operatorSubscribeBlocked(
						"(*writeQuota).get",
						"(*operatorSubscribeRPCStream).Send",
						"(*systemService).Subscribe",
					)
			}, operatorSubscribeTestTimeout, time.Millisecond,
				"second Send did not block in production transport write quota")

			select {
			case err := <-fixture.sendReturned:
				t.Fatalf("second Send returned before cancellation: %v", err)
			default:
			}
			select {
			case <-fixture.run.finished:
				t.Fatal("handler returned before backpressure was released")
			default:
			}
			require.Equal(t, 1, fixture.count())

			switch action {
			case "client_cancellation":
				fixture.cancel()
			case "forced_server_stop":
				fixture.server.Stop()
			}

			sendErr := operatorSubscribeAwait(
				t, fixture.sendReturned, "blocked Send cancellation")
			require.Error(t, sendErr)

			handlerErr := operatorSubscribeRequireFinished(
				t, fixture.run, fixture.count)
			require.True(t, handlerErr == sendErr,
				"handler did not propagate its transport Send error directly")

			_, recvErr := operatorSubscribeRecv(t, fixture)
			require.Error(t, recvErr)
			if action == "client_cancellation" {
				require.Equal(t, codes.Canceled, status.Code(recvErr))
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3002 -- verify operator subscription cancellation and transport cleanup -- */
