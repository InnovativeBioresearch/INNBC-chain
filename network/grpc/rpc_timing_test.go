/* START ALPHA CHANGE 2733 -- test delayed transport readiness and operation after setup deadline expiry -- */

package grpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/network/proto"
	"github.com/stretchr/testify/require"
	rawGrpc "google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

const rpcTimingWatchdog = 15 * time.Second

func waitRPCTimingSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()

	timer := time.NewTimer(rpcTimingWatchdog)
	defer timer.Stop()

	select {
	case <-signal:
	case <-timer.C:
		t.Fatal(message)
	}
}

type rpcTimingResult struct {
	conn *rawGrpc.ClientConn
	err  error
}

type rpcTimingWriteStream struct {
	*rpcTestStream
	started chan struct{}
	once    sync.Once
}

func (s *rpcTimingWriteStream) Write(data []byte) (int, error) {
	s.once.Do(func() { close(s.started) })

	return s.rpcTestStream.Write(data)
}

func newRPCTimingService(
	t *testing.T,
) (*GrpcStream, *rpcTestStream, *rpcTestStream) {
	t.Helper()

	service := NewGrpcStream()
	t.Cleanup(service.Stop)
	proto.RegisterTestServiceServer(service.GrpcServer(), &rpcEchoService{})
	service.Serve()

	left, right := rpcStreamPair(t)
	t.Cleanup(func() {
		_ = left.Reset()
		_ = right.Reset()
	})

	return service, left, right
}

func requireRPCTimingEcho(
	t *testing.T,
	conn *rawGrpc.ClientConn,
	message string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reply, err := proto.NewTestServiceClient(conn).SayHello(
		ctx,
		&proto.GenericMessage{Message: message},
	)
	require.NoError(t, err)
	require.NotNil(t, reply)
	require.Equal(t, message, reply.Message)
}

func requireRPCTimingDeadlineCleared(t *testing.T, stream *rpcTestStream) {
	t.Helper()

	stream.lock.Lock()
	deadline := stream.deadline
	stream.lock.Unlock()

	require.True(t, deadline.IsZero(), "establishment left a transport deadline")
}

func TestRPCTransport_DelayedReadinessSucceedsBeforeDeadline(t *testing.T) {
	service, left, right := newRPCTimingService(t)

	releaseHandshake := make(chan struct{})
	handoffEntered := make(chan struct{})
	handoffDone := make(chan struct{})
	dialDone := make(chan struct{})
	results := make(chan rpcTimingResult, 1)
	var releaseOnce sync.Once
	var opened *rawGrpc.ClientConn

	handoffCtx, handoffCancel := context.WithTimeout(
		context.Background(),
		rpcTimingWatchdog,
	)
	setupCtx, setupCancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)

	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseHandshake) })
		handoffCancel()
		setupCancel()
		_ = left.Reset()
		_ = right.Reset()

		waitRPCTimingSignal(t, handoffDone, "remote handoff did not terminate")
		waitRPCTimingSignal(t, dialDone, "transport establishment did not terminate")

		if opened != nil {
			_ = opened.Close()
		}

		select {
		case result := <-results:
			if result.conn != nil {
				_ = result.conn.Close()
			}
		default:
		}
	})

	go func() {
		defer close(handoffDone)
		close(handoffEntered)

		select {
		case <-releaseHandshake:
		case <-handoffCtx.Done():
			return
		}

		if handoffCtx.Err() != nil {
			return
		}

		service.Handler()(right)
	}()

	stream := &rpcTimingWriteStream{
		rpcTestStream: left,
		started:       make(chan struct{}),
	}
	go func() {
		defer close(dialDone)

		conn, err := WrapClientContext(setupCtx, stream)
		results <- rpcTimingResult{conn: conn, err: err}
	}()

	waitRPCTimingSignal(t, handoffEntered, "remote handoff gate was not reached")
	waitRPCTimingSignal(t, stream.started, "client transport I/O did not start")

	/* The deliberate delay starts only after client establishment performs I/O. */
	delay := time.NewTimer(100 * time.Millisecond)
	defer delay.Stop()

	select {
	case result := <-results:
		opened = result.conn
		t.Fatalf("establishment returned before remote handoff: %v", result.err)
	case <-delay.C:
	}

	releaseOnce.Do(func() { close(releaseHandshake) })

	watchdog := time.NewTimer(rpcTimingWatchdog)
	defer watchdog.Stop()

	select {
	case result := <-results:
		opened = result.conn
		require.NoError(t, result.err)
		require.NotNil(t, opened)
	case <-watchdog.C:
		t.Fatal("released transport did not become ready")
	}

	waitRPCTimingSignal(t, dialDone, "establishment worker did not exit")
	waitRPCTimingSignal(t, handoffDone, "remote handoff worker did not exit")

	require.NoError(t, setupCtx.Err())
	requireRPCTimingDeadlineCleared(t, left)
	requireRPCTimingEcho(t, opened, "delayed readiness succeeded")
}

func TestRPCTransport_RemainsUsableAfterSetupDeadlineExpires(t *testing.T) {
	service, left, right := newRPCTimingService(t)
	handoffDone := make(chan struct{})
	var opened *rawGrpc.ClientConn

	setupCtx, setupCancel := context.WithTimeout(
		context.Background(),
		2*time.Second,
	)
	t.Cleanup(func() {
		setupCancel()

		if opened != nil {
			_ = opened.Close()
		}

		_ = left.Reset()
		_ = right.Reset()
		waitRPCTimingSignal(t, handoffDone, "remote handoff did not terminate")
	})

	go func() {
		defer close(handoffDone)
		service.Handler()(right)
	}()

	conn, err := WrapClientContext(setupCtx, left)
	opened = conn
	require.NoError(t, err)
	require.NotNil(t, conn)

	deadline, hasDeadline := setupCtx.Deadline()
	require.True(t, hasDeadline)
	requireRPCTimingEcho(t, conn, "before setup deadline")
	requireRPCTimingDeadlineCleared(t, left)

	/* Let the setup deadline expire naturally instead of cancelling it manually. */
	waitRPCTimingSignal(t, setupCtx.Done(), "setup deadline did not expire")
	require.ErrorIs(t, setupCtx.Err(), context.DeadlineExceeded)
	require.False(t, time.Now().Before(deadline))
	require.NotEqual(t, connectivity.Shutdown, conn.GetState())

	/* Setup owns establishment only; this RPC uses a fresh operation context. */
	requireRPCTimingEcho(t, conn, "after setup deadline")
	requireRPCTimingDeadlineCleared(t, left)
	require.NotEqual(t, connectivity.Shutdown, conn.GetState())

	waitRPCTimingSignal(t, handoffDone, "remote handoff worker did not exit")
}

/* FINISH ALPHA CHANGE 2733 -- test delayed transport readiness and operation after setup deadline expiry -- */
