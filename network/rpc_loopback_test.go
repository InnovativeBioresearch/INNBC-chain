/* START ALPHA CHANGE 2734 -- test real loopback protocol establishment delay and recovery after timeout -- */

package network

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/network/dial"
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"
	rawGrpc "google.golang.org/grpc"
)

const (
	rpcLoopbackProtocol protocol.ID = "/innbc/test/rpc-timing/1.0.0"
	rpcLoopbackWatchdog             = 20 * time.Second
)

func waitRPCLoopbackSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()

	timer := time.NewTimer(rpcLoopbackWatchdog)
	defer timer.Stop()

	select {
	case <-signal:
	case <-timer.C:
		t.Fatal(message)
	}
}

type rpcLoopbackEcho struct {
	proto.UnimplementedTestServiceServer
}

func (*rpcLoopbackEcho) SayHello(
	_ context.Context,
	req *proto.GenericMessage,
) (*proto.GenericMessage, error) {
	return req, nil
}

type rpcLoopbackObservedStream struct {
	libnetwork.Stream
	closed    chan struct{}
	closeOnce sync.Once
}

func (s *rpcLoopbackObservedStream) Reset() error {
	err := s.Stream.Reset()
	s.closeOnce.Do(func() { close(s.closed) })

	return err
}

type rpcLoopbackAttempt struct {
	number  int32
	stream  *rpcLoopbackObservedStream
	release chan struct{}
	exited  chan struct{}
	cancel  context.CancelFunc
	once    sync.Once
}

func (a *rpcLoopbackAttempt) allow() {
	a.once.Do(func() { close(a.release) })
}

func (a *rpcLoopbackAttempt) abort() {
	a.cancel()
	_ = a.stream.Reset()
}

type rpcLoopbackFixture struct {
	server   *Server
	remote   host.Host
	service  *netgrpc.GrpcStream
	attempts chan *rpcLoopbackAttempt
	handlers netgrpc.RPCLifetime
	sequence atomic.Int32
}

func newRPCLoopbackFixture(t *testing.T) *rpcLoopbackFixture {
	t.Helper()

	local, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = local.Close() })

	remote, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = remote.Close() })

	service := netgrpc.NewGrpcStream()
	t.Cleanup(service.Stop)
	proto.RegisterTestServiceServer(service.GrpcServer(), &rpcLoopbackEcho{})
	service.Serve()

	server := &Server{
		logger:    hclog.NewNullLogger(),
		host:      local,
		closeCh:   make(chan struct{}),
		dialQueue: dial.NewDialQueue(),
		protocols: map[string]Protocol{
			string(rpcLoopbackProtocol): netgrpc.NewGrpcStream(),
		},
		peers: make(map[peer.ID]*PeerConnInfo),
	}
	t.Cleanup(func() { _ = server.Close() })

	fixture := &rpcLoopbackFixture{
		server:   server,
		remote:   remote,
		service:  service,
		attempts: make(chan *rpcLoopbackAttempt, 8),
	}

	handlerParent, cancelHandlers := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	t.Cleanup(func() {
		/* Release gated handlers before stopping services or closing their hosts. */
		cancelHandlers()
		fixture.handlers.Close()
	})

	remote.SetStreamHandler(rpcLoopbackProtocol, func(raw libnetwork.Stream) {
		ctx, finish, err := fixture.handlers.Begin(handlerParent)
		if err != nil {
			_ = raw.Reset()

			return
		}
		defer finish()

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		attempt := &rpcLoopbackAttempt{
			number: fixture.sequence.Add(1),
			stream: &rpcLoopbackObservedStream{
				Stream: raw,
				closed: make(chan struct{}),
			},
			release: make(chan struct{}),
			exited:  make(chan struct{}),
			cancel:  cancel,
		}
		defer close(attempt.exited)

		select {
		case fixture.attempts <- attempt:
		case <-ctx.Done():
			_ = attempt.stream.Reset()

			return
		}

		/* Protocol selection has completed; gRPC readiness is deliberately gated. */
		select {
		case <-attempt.release:
		case <-ctx.Done():
			_ = attempt.stream.Reset()

			return
		}

		if ctx.Err() != nil {
			_ = attempt.stream.Reset()

			return
		}

		service.Handler()(attempt.stream)
	})

	connectCtx, cancelConnect := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancelConnect()

	err = local.Connect(connectCtx, peer.AddrInfo{
		ID:    remote.ID(),
		Addrs: remote.Addrs(),
	})
	require.NoError(t, err)

	return fixture
}

type rpcLoopbackResult struct {
	conn      *rawGrpc.ClientConn
	err       error
	callerErr error
}

func startRPCLoopbackDial(
	t *testing.T,
	fixture *rpcLoopbackFixture,
	parent context.Context,
) <-chan rpcLoopbackResult {
	t.Helper()

	ctx, cancel := context.WithCancel(parent)
	results := make(chan rpcLoopbackResult, 1)
	done := make(chan struct{})

	t.Cleanup(func() {
		cancel()
		waitRPCLoopbackSignal(t, done, "loopback dial worker did not terminate")

		select {
		case result := <-results:
			if result.conn != nil {
				_ = result.conn.Close()
			}
		default:
		}
	})

	go func() {
		defer close(done)

		conn, err := fixture.server.NewProtoConnectionContext(
			ctx,
			string(rpcLoopbackProtocol),
			fixture.remote.ID(),
		)
		results <- rpcLoopbackResult{
			conn:      conn,
			err:       err,
			callerErr: ctx.Err(),
		}
	}()

	return results
}

func nextRPCLoopbackAttempt(
	t *testing.T,
	fixture *rpcLoopbackFixture,
) *rpcLoopbackAttempt {
	t.Helper()

	timer := time.NewTimer(rpcLoopbackWatchdog)
	defer timer.Stop()

	select {
	case attempt := <-fixture.attempts:
		return attempt
	case <-timer.C:
		t.Fatal("remote protocol handler did not receive a stream")

		return nil
	}
}

func takeRPCLoopbackResult(
	t *testing.T,
	results <-chan rpcLoopbackResult,
) rpcLoopbackResult {
	t.Helper()

	timer := time.NewTimer(rpcLoopbackWatchdog)
	defer timer.Stop()

	select {
	case result := <-results:
		if result.conn != nil {
			t.Cleanup(func() { _ = result.conn.Close() })
		}

		return result
	case <-timer.C:
		t.Fatal("loopback establishment did not return")

		return rpcLoopbackResult{}
	}
}

func requireRPCLoopbackEcho(t *testing.T, conn *rawGrpc.ClientConn, message string) {
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

func TestRPCProtocol_LoopbackDelayedReadinessSucceeds(t *testing.T) {
	fixture := newRPCLoopbackFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := startRPCLoopbackDial(t, fixture, ctx)
	attempt := nextRPCLoopbackAttempt(t, fixture)
	require.Equal(t, int32(1), attempt.number)

	delay := time.NewTimer(100 * time.Millisecond)
	defer delay.Stop()

	select {
	case result := <-results:
		if result.conn != nil {
			t.Cleanup(func() { _ = result.conn.Close() })
		}
		t.Fatalf("establishment returned before gRPC handoff: %v", result.err)
	case <-delay.C:
	}

	attempt.allow()
	result := takeRPCLoopbackResult(t, results)
	require.NoError(t, result.err)
	require.NotNil(t, result.conn)
	requireRPCLoopbackEcho(t, result.conn, "loopback delayed readiness")

	require.NoError(t, result.conn.Close())
	waitRPCLoopbackSignal(t, attempt.exited, "remote handoff handler did not exit")
	waitRPCLoopbackSignal(t, attempt.stream.closed, "RPC stream was not released")

	require.Equal(
		t,
		libnetwork.Connected,
		fixture.server.host.Network().Connectedness(fixture.remote.ID()),
		"closing an RPC stream disconnected the peer",
	)
}

func TestRPCProtocol_LoopbackTimeoutThenFreshAttemptSucceeds(t *testing.T) {
	fixture := newRPCLoopbackFixture(t)

	firstCtx, firstCancel := context.WithTimeout(
		context.Background(),
		2*time.Second,
	)
	defer firstCancel()

	firstResults := startRPCLoopbackDial(t, fixture, firstCtx)
	firstAttempt := nextRPCLoopbackAttempt(t, fixture)
	firstResult := takeRPCLoopbackResult(t, firstResults)

	require.Error(t, firstResult.err)
	require.Nil(t, firstResult.conn)
	require.True(
		t,
		errors.Is(firstResult.err, context.DeadlineExceeded) ||
			errors.Is(firstResult.callerErr, context.DeadlineExceeded),
		"failed attempt did not observe its deadline: %v",
		firstResult.err,
	)

	firstAttempt.abort()
	waitRPCLoopbackSignal(t, firstAttempt.exited, "failed attempt handler did not exit")
	waitRPCLoopbackSignal(t, firstAttempt.stream.closed, "failed attempt stream was not released")

	secondCtx, secondCancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer secondCancel()

	secondResults := startRPCLoopbackDial(t, fixture, secondCtx)
	secondAttempt := nextRPCLoopbackAttempt(t, fixture)

	require.Equal(t, int32(2), secondAttempt.number)
	require.NotEqual(t, firstAttempt.stream.ID(), secondAttempt.stream.ID())
	secondAttempt.allow()

	secondResult := takeRPCLoopbackResult(t, secondResults)
	require.NoError(t, secondResult.err)
	require.NotNil(t, secondResult.conn)
	requireRPCLoopbackEcho(t, secondResult.conn, "fresh attempt after timeout")

	require.NoError(t, secondResult.conn.Close())
	waitRPCLoopbackSignal(t, secondAttempt.exited, "successful attempt handler did not exit")
	waitRPCLoopbackSignal(t, secondAttempt.stream.closed, "successful RPC stream was not released")

	require.Equal(
		t,
		libnetwork.Connected,
		fixture.server.host.Network().Connectedness(fixture.remote.ID()),
	)

	shutdownResult := make(chan error, 1)
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		shutdownResult <- fixture.server.Close()
	}()

	timer := time.NewTimer(rpcLoopbackWatchdog)
	defer timer.Stop()

	select {
	case err := <-shutdownResult:
		require.NoError(t, err)
	case <-timer.C:
		t.Fatal("server shutdown did not complete after timeout recovery")
	}
	waitRPCLoopbackSignal(t, shutdownDone, "server shutdown worker did not exit")
}

/* FINISH ALPHA CHANGE 2734 -- test real loopback protocol establishment delay and recovery after timeout -- */
