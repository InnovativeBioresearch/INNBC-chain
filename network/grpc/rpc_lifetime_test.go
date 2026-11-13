package grpc

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/network/proto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	/* START ALPHA CHANGE 2724 -- inspect owned client shutdown in RPC lifecycle regressions -- */
	"google.golang.org/grpc/connectivity"
	/* FINISH ALPHA CHANGE 2724 -- inspect owned client shutdown in RPC lifecycle regressions -- */)

/* START ALPHA CHANGE 2721 -- regress RPC admission transport cancellation and joined service shutdown -- */

func waitRPCSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RPC lifecycle signal")
	}
}

func TestRPCLifetime_CloseCancelsAndJoins(t *testing.T) {
	var lifetime RPCLifetime

	ctx, finish, err := lifetime.Begin(context.Background())
	require.NoError(t, err)
	t.Cleanup(finish)

	closed := make(chan struct{})
	go func() {
		lifetime.Close()
		close(closed)
	}()

	waitRPCSignal(t, ctx.Done())

	select {
	case <-closed:
		t.Fatal("Close returned before the operation completed")
	default:
	}

	_, _, err = lifetime.Begin(context.Background())
	require.ErrorIs(t, err, ErrRPCClosed)

	finish()
	finish()
	waitRPCSignal(t, closed)
	lifetime.Close()
}

type rpcTestConn struct {
	network.Conn
	addr multiaddr.Multiaddr
}

func (c *rpcTestConn) LocalMultiaddr() multiaddr.Multiaddr  { return c.addr }
func (c *rpcTestConn) RemoteMultiaddr() multiaddr.Multiaddr { return c.addr }
func (c *rpcTestConn) LocalPeer() peer.ID                   { return "local" }
func (c *rpcTestConn) RemotePeer() peer.ID                  { return "remote" }

type rpcTestStream struct {
	network.Stream
	socket net.Conn
	info   *rpcTestConn
	resets atomic.Int32

	lock     sync.Mutex
	deadline time.Time
}

func (s *rpcTestStream) Read(p []byte) (int, error)  { return s.socket.Read(p) }
func (s *rpcTestStream) Write(p []byte) (int, error) { return s.socket.Write(p) }
func (s *rpcTestStream) Conn() network.Conn          { return s.info }
func (s *rpcTestStream) Close() error                { return s.socket.Close() }

func (s *rpcTestStream) Reset() error {
	s.resets.Add(1)

	return s.socket.Close()
}

func (s *rpcTestStream) SetDeadline(deadline time.Time) error {
	s.lock.Lock()
	s.deadline = deadline
	s.lock.Unlock()

	return s.socket.SetDeadline(deadline)
}

func (s *rpcTestStream) SetReadDeadline(deadline time.Time) error {
	return s.socket.SetReadDeadline(deadline)
}

func (s *rpcTestStream) SetWriteDeadline(deadline time.Time) error {
	return s.socket.SetWriteDeadline(deadline)
}

func rpcStreamPair(t *testing.T) (*rpcTestStream, *rpcTestStream) {
	t.Helper()

	addr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1234")
	require.NoError(t, err)

	a, b := net.Pipe()
	info := &rpcTestConn{addr: addr}

	return &rpcTestStream{socket: a, info: info},
		&rpcTestStream{socket: b, info: info}
}

type rpcEchoService struct {
	proto.UnimplementedTestServiceServer
	hello func(context.Context, *proto.GenericMessage) (*proto.GenericMessage, error)
}

func (s *rpcEchoService) SayHello(
	ctx context.Context,
	req *proto.GenericMessage,
) (*proto.GenericMessage, error) {
	if s.hello != nil {
		return s.hello(ctx, req)
	}

	return req, nil
}

func TestRPCTransport_CancelsBlockedEstablishment(t *testing.T) {
	a, b := rpcStreamPair(t)
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})

	/* The remote endpoint never reads or completes the transport handshake. */
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	conn, err := WrapClientContext(ctx, a)
	require.Nil(t, conn)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Greater(t, a.resets.Load(), int32(0))
}

func TestRPCTransport_ClearsEstablishmentDeadline(t *testing.T) {
	service := NewGrpcStream()
	proto.RegisterTestServiceServer(service.GrpcServer(), &rpcEchoService{})
	t.Cleanup(service.Stop)
	service.Serve()

	a, b := rpcStreamPair(t)
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})
	go service.Handler()(b)

	setupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, err := WrapClientContext(setupCtx, a)
	cancel()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	a.lock.Lock()
	deadline := a.deadline
	a.lock.Unlock()
	require.True(t, deadline.IsZero())

	/* Setup cancellation must not become the transport's total lifetime. */
	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rpcCancel()

	reply, err := proto.NewTestServiceClient(conn).SayHello(
		rpcCtx,
		&proto.GenericMessage{Message: "still alive"},
	)
	require.NoError(t, err)
	require.Equal(t, "still alive", reply.Message)
}

func TestRPCListener_CloseUnblocksAcceptAndRejectsLateStreams(t *testing.T) {
	listener := NewGrpcStream()
	t.Cleanup(listener.Stop)

	accepted := make(chan struct{})
	var acceptErr error
	go func() {
		_, acceptErr = listener.Accept()
		close(accepted)
	}()

	require.NoError(t, listener.Close())
	require.NoError(t, listener.Close())
	waitRPCSignal(t, accepted)
	require.ErrorIs(t, acceptErr, io.EOF)

	a, b := rpcStreamPair(t)
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})

	handled := make(chan struct{})
	go func() {
		listener.Handler()(a)
		close(handled)
	}()

	waitRPCSignal(t, handled)
	require.Greater(t, a.resets.Load(), int32(0))
	listener.Stop()
	listener.Serve()
}

func TestRPCService_StopJoinsHandlers(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeClient := func() {}

	service := NewGrpcStream()
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		closeClient()
		service.Stop()
	})

	proto.RegisterTestServiceServer(service.GrpcServer(), &rpcEchoService{
		hello: func(ctx context.Context, _ *proto.GenericMessage) (*proto.GenericMessage, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			<-release

			return nil, ctx.Err()
		},
	})
	service.Serve()

	a, b := rpcStreamPair(t)
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})
	go service.Handler()(b)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := WrapClientContext(ctx, a)
	require.NoError(t, err)
	closeClient = func() { _ = conn.Close() }

	callDone := make(chan struct{})
	go func() {
		_, _ = proto.NewTestServiceClient(conn).SayHello(
			context.Background(),
			&proto.GenericMessage{Message: "wait"},
		)
		close(callDone)
	}()

	waitRPCSignal(t, started)

	stopped := make(chan struct{})
	go func() {
		service.Stop()
		close(stopped)
	}()

	waitRPCSignal(t, cancelled)

	select {
	case <-stopped:
		t.Fatal("Stop returned while the handler still owned its work")
	default:
	}

	releaseOnce.Do(func() { close(release) })
	waitRPCSignal(t, stopped)
	waitRPCSignal(t, callDone)
	service.Stop()
}

/* FINISH ALPHA CHANGE 2721 -- regress RPC admission transport cancellation and joined service shutdown -- */

/* START ALPHA CHANGE 2725 -- regress cancellation handoff concurrent cleanup and pending listener delivery -- */

type rpcCancelOnClearStream struct {
	*rpcTestStream
	cancel context.CancelFunc
}

func (s *rpcCancelOnClearStream) SetDeadline(deadline time.Time) error {
	err := s.rpcTestStream.SetDeadline(deadline)
	if err == nil && deadline.IsZero() {
		/* Readiness succeeded; cancellation now races ownership handoff. */
		s.cancel()
	}

	return err
}

func TestRPCOwnership_CancelAtReadinessHandoff(t *testing.T) {
	service := NewGrpcStream()
	proto.RegisterTestServiceServer(service.GrpcServer(), &rpcEchoService{})
	service.Serve()

	a, b := rpcStreamPair(t)
	closeClient := func() {}
	t.Cleanup(func() {
		closeClient()
		_ = a.Close()
		_ = b.Close()
		service.Stop()
	})
	go service.Handler()(b)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := &rpcCancelOnClearStream{
		rpcTestStream: a,
		cancel:        cancel,
	}

	conn, err := WrapClientContext(ctx, stream)
	require.NoError(t, err)
	require.NotNil(t, conn)
	closeClient = func() { _ = conn.Close() }
	require.ErrorIs(t, ctx.Err(), context.Canceled)

	/* An already-cancelled owner must release the returned connection. */
	cleanup := OwnClient(ctx, conn)
	closeClient = cleanup
	cleanup()
	cleanup()

	require.Equal(t, connectivity.Shutdown, conn.GetState())
	require.Greater(t, a.resets.Load(), int32(0))
}

type rpcGatedResetStream struct {
	*rpcTestStream
	enabled   atomic.Bool
	completed atomic.Bool
	entered   chan struct{}
	release   <-chan struct{}
	enterOnce sync.Once
}

func (s *rpcGatedResetStream) Reset() error {
	if s.enabled.Load() {
		s.enterOnce.Do(func() { close(s.entered) })
		<-s.release
	}

	err := s.rpcTestStream.Reset()
	s.completed.Store(true)

	return err
}

func TestRPCOwnership_CleanupJoinsCancellationClose(t *testing.T) {
	service := NewGrpcStream()
	proto.RegisterTestServiceServer(service.GrpcServer(), &rpcEchoService{})
	service.Serve()

	a, b := rpcStreamPair(t)
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeClient := func() {}

	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		closeClient()
		_ = a.Close()
		_ = b.Close()
		service.Stop()
	})
	go service.Handler()(b)

	stream := &rpcGatedResetStream{
		rpcTestStream: a,
		entered:       make(chan struct{}),
		release:       release,
	}

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer setupCancel()

	conn, err := WrapClientContext(setupCtx, stream)
	require.NoError(t, err)
	closeClient = func() { _ = conn.Close() }

	/* Only teardown is gated; failed establishment can still reset normally. */
	stream.enabled.Store(true)

	ownerCtx, ownerCancel := context.WithCancel(context.Background())
	defer ownerCancel()

	cleanup := OwnClient(ownerCtx, conn)
	closeClient = cleanup
	ownerCancel()
	waitRPCSignal(t, stream.entered)

	cleaned := make(chan struct{})
	go func() {
		cleanup()
		close(cleaned)
	}()

	/* Cleanup must join the callback already blocked inside transport closure. */
	select {
	case <-cleaned:
		t.Fatal("cleanup returned before the cancellation callback released the transport")
	case <-time.After(20 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	waitRPCSignal(t, cleaned)

	require.True(t, stream.completed.Load())
	require.Equal(t, connectivity.Shutdown, conn.GetState())
	cleanup()
}

func TestRPCListener_CloseReleasesPendingHandoff(t *testing.T) {
	listener := NewGrpcStream()
	a, b := rpcStreamPair(t)
	t.Cleanup(func() {
		listener.Stop()
		_ = a.Close()
		_ = b.Close()
	})

	started := make(chan struct{})
	handled := make(chan struct{})
	go func() {
		close(started)
		listener.Handler()(a)
		close(handled)
	}()
	waitRPCSignal(t, started)

	/* No Accept or Serve exists to receive the stream. */
	select {
	case <-handled:
		t.Fatal("stream handler returned before handoff or closure")
	case <-time.After(20 * time.Millisecond):
	}

	require.NoError(t, listener.Close())
	waitRPCSignal(t, handled)
	require.Greater(t, a.resets.Load(), int32(0))
}

/* FINISH ALPHA CHANGE 2725 -- regress cancellation handoff concurrent cleanup and pending listener delivery -- */
