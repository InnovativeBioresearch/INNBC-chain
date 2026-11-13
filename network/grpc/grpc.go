package grpc

import (
	"context"
	"errors"
	"io"
	"net"

	/* START ALPHA CHANGE 2671 -- import synchronization and transport deadline support -- */
	"sync"
	"time"

	/* FINISH ALPHA CHANGE 2671 -- import synchronization and transport deadline support -- */

	"google.golang.org/grpc/credentials/insecure"

	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/innovativebioresearch/innbc-chain/validate"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/grpc"
	grpcPeer "google.golang.org/grpc/peer"
)

/* START ALPHA CHANGE 2672 -- retain listener cancellation and serving lifecycle -- */
type GrpcStream struct {
	ctx      context.Context
	cancel   context.CancelFunc
	streamCh chan network.Stream

	grpcServer *grpc.Server

	lifecycleLock sync.Mutex
	started       bool
	stopped       bool
	serveDone     chan struct{}
	stopOnce      sync.Once

	/* START ALPHA CHANGE 2748 -- retain authorization independently of serving lifecycle -- */
	authorizationLock sync.RWMutex
	authorizePeer     func(peer.ID) bool
	/* FINISH ALPHA CHANGE 2748 -- retain authorization independently of serving lifecycle -- */
}

func NewGrpcStream() *GrpcStream {
	ctx, cancel := context.WithCancel(context.Background())

	/* START ALPHA CHANGE 2748 -- authorize unary and streaming calls on reused transports -- */
	service := &GrpcStream{
		ctx:       ctx,
		cancel:    cancel,
		streamCh:  make(chan network.Stream),
		serveDone: make(chan struct{}),
	}
	service.grpcServer = grpc.NewServer(
		grpc.ChainUnaryInterceptor(service.authorizeUnary, interceptor),
		grpc.StreamInterceptor(service.authorizeStream),
		grpc.WaitForHandlers(true),
	)

	return service
	/* FINISH ALPHA CHANGE 2748 -- authorize unary and streaming calls on reused transports -- */
}

/* FINISH ALPHA CHANGE 2672 -- retain listener cancellation and serving lifecycle -- */

type Context struct {
	context.Context
	PeerID peer.ID
}

/* START ALPHA CHANGE 2902 -- expose authenticated transport identity for serving admission -- */
/* PeerIDFromContext reads only the identity supplied by the libp2p transport. */
func PeerIDFromContext(ctx context.Context) (peer.ID, error) {
	if ctx == nil {
		return "", errors.New("missing authenticated P2P peer")
	}

	remote, ok := grpcPeer.FromContext(ctx)
	if !ok || remote == nil {
		return "", errors.New("missing authenticated P2P peer")
	}

	addr, ok := remote.Addr.(*wrapLibp2pAddr)
	if !ok || addr == nil || addr.id == "" {
		return "", errors.New("missing authenticated P2P peer")
	}

	return addr.id, nil
}

/* FINISH ALPHA CHANGE 2902 -- expose authenticated transport identity for serving admission -- */

/* START ALPHA CHANGE 2748 -- extract RPC identity solely from authenticated transport data -- */
func (g *GrpcStream) SetPeerAuthorizer(authorize func(peer.ID) bool) {
	g.authorizationLock.Lock()
	g.authorizePeer = authorize
	g.authorizationLock.Unlock()
}

func (g *GrpcStream) authorize(ctx context.Context) error {
	g.authorizationLock.RLock()
	authorize := g.authorizePeer
	g.authorizationLock.RUnlock()

	/* Standalone transports retain their existing behavior until registered. */
	if authorize == nil {
		return nil
	}

	remote, ok := grpcPeer.FromContext(ctx)
	if !ok {
		return errors.New("missing authenticated P2P peer")
	}
	addr, ok := remote.Addr.(*wrapLibp2pAddr)
	if !ok || !authorize(addr.id) {
		return errors.New("P2P peer is not authorized")
	}

	return nil
}

func (g *GrpcStream) authorizeUnary(
	ctx context.Context,
	req interface{},
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	if err := g.authorize(ctx); err != nil {
		return nil, err
	}

	return handler(ctx, req)
}

func (g *GrpcStream) authorizeStream(
	service interface{},
	stream grpc.ServerStream,
	_ *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	if err := g.authorize(stream.Context()); err != nil {
		return err
	}

	return handler(service, stream)
}

/* FINISH ALPHA CHANGE 2748 -- extract RPC identity solely from authenticated transport data -- */

// interceptor is the middleware function that wraps
// gRPC peer data to custom Polygon Edge structures
func interceptor(
	ctx context.Context,
	req interface{},
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	// Validate request
	if err := validate.ValidateRequest(req); err != nil {
		return nil, err
	}

	// Grab the peer info from the connection
	contextPeer, ok := grpcPeer.FromContext(ctx)
	if !ok {
		return nil, errors.New("invalid type assertion for peer context")
	}

	// The peer address is expected to be wrapped in a custom
	// structure that contains the PeerID
	addr, ok := contextPeer.Addr.(*wrapLibp2pAddr)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	// Wrap the extracted PeerID and the context
	// so the stream handler has access to the PeerID
	return handler(
		&Context{
			Context: ctx,
			PeerID:  addr.id,
		},
		req,
	)
}

/* START ALPHA CHANGE 2673 -- establish clients with context and reject streams during shutdown -- */
func (g *GrpcStream) Client(stream network.Stream) (*grpc.ClientConn, error) {
	return WrapClient(stream)
}

func (g *GrpcStream) ClientContext(
	ctx context.Context,
	stream network.Stream,
) (*grpc.ClientConn, error) {
	return WrapClientContext(ctx, stream)
}

func (g *GrpcStream) Serve() {
	g.lifecycleLock.Lock()
	defer g.lifecycleLock.Unlock()

	if g.started || g.stopped {
		return
	}

	g.started = true

	go func() {
		defer close(g.serveDone)

		_ = g.grpcServer.Serve(g)
	}()
}

func (g *GrpcStream) Handler() func(network.Stream) {
	return func(stream network.Stream) {
		if g.ctx.Err() != nil {
			_ = stream.Reset()

			return
		}

		select {
		case <-g.ctx.Done():
			_ = stream.Reset()
		case g.streamCh <- stream:
			/* Accept now owns this stream. */
		}
	}
}

/* FINISH ALPHA CHANGE 2673 -- establish clients with context and reject streams during shutdown -- */

func (g *GrpcStream) RegisterService(sd *grpc.ServiceDesc, ss interface{}) {
	g.grpcServer.RegisterService(sd, ss)
}

func (g *GrpcStream) GrpcServer() *grpc.Server {
	return g.grpcServer
}

// --- listener ---

/* START ALPHA CHANGE 2674 -- release streams handed off while listener closure races acceptance -- */
func (g *GrpcStream) Accept() (net.Conn, error) {
	if g.ctx.Err() != nil {
		return nil, io.EOF
	}

	select {
	case <-g.ctx.Done():
		return nil, io.EOF
	case stream := <-g.streamCh:
		if g.ctx.Err() != nil {
			_ = stream.Reset()

			return nil, io.EOF
		}

		return &streamConn{Stream: stream}, nil
	}
}

/* FINISH ALPHA CHANGE 2674 -- release streams handed off while listener closure races acceptance -- */

// Addr implements the net.Listener interface
func (g *GrpcStream) Addr() net.Addr {
	return fakeLocalAddr()
}

/* START ALPHA CHANGE 2675 -- separate listener closure from joined gRPC service shutdown -- */
func (g *GrpcStream) Close() error {
	/* gRPC itself calls this listener method while stopping. */
	g.cancel()

	return nil
}

func (g *GrpcStream) Stop() {
	g.stopOnce.Do(func() {
		g.lifecycleLock.Lock()
		g.stopped = true
		started := g.started
		g.lifecycleLock.Unlock()

		_ = g.Close()
		g.grpcServer.Stop()

		if started {
			<-g.serveDone
		}
	})
}

/* FINISH ALPHA CHANGE 2675 -- separate listener closure from joined gRPC service shutdown -- */

// --- conn ---

/* START ALPHA CHANGE 2676 -- bound transport readiness and release stalled negotiation -- */
func WrapClient(s network.Stream) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		DefaultProtocolTimeout,
	)
	defer cancel()

	return WrapClientContext(ctx, s)
}

func WrapClientContext(
	ctx context.Context,
	s network.Stream,
) (*grpc.ClientConn, error) {
	if err := ctx.Err(); err != nil {
		_ = s.Reset()

		return nil, err
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := s.SetDeadline(deadline); err != nil {
			_ = s.Reset()

			return nil, err
		}
	}

	resetDone := make(chan struct{})
	stopReset := context.AfterFunc(ctx, func() {
		_ = s.Reset()
		close(resetDone)
	})

	var dialLock sync.Mutex
	used := false

	opts := grpc.WithContextDialer(func(
		dialCtx context.Context,
		_ string,
	) (net.Conn, error) {
		if err := dialCtx.Err(); err != nil {
			return nil, err
		}

		dialLock.Lock()
		defer dialLock.Unlock()

		if used {
			/* A closed libp2p stream cannot be reused by a reconnect. */
			return nil, net.ErrClosed
		}

		used = true

		return &streamConn{Stream: s}, nil
	})

	conn, err := grpc.DialContext(
		ctx,
		"",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		opts,
	)

	if !stopReset() {
		<-resetDone

		if conn != nil {
			_ = conn.Close()
		}

		return nil, ctx.Err()
	}

	if err != nil {
		_ = s.Reset()

		return nil, err
	}

	if err := ctx.Err(); err != nil {
		_ = s.Reset()
		_ = conn.Close()

		return nil, err
	}

	/* Establishment must not impose a total lifetime on a download. */
	if err := s.SetDeadline(time.Time{}); err != nil {
		_ = s.Reset()
		_ = conn.Close()

		return nil, err
	}

	return conn, nil
}

/* FINISH ALPHA CHANGE 2676 -- bound transport readiness and release stalled negotiation -- */

// streamConn represents a net.Conn wrapped to be compatible with net.conn
type streamConn struct {
	network.Stream
}

/* START ALPHA CHANGE 2677 -- reset an owned stream when its gRPC transport closes -- */
func (c *streamConn) Close() error {
	return c.Stream.Reset()
}

/* FINISH ALPHA CHANGE 2677 -- reset an owned stream when its gRPC transport closes -- */

type wrapLibp2pAddr struct {
	id peer.ID
	net.Addr
}

// LocalAddr returns the local address.
func (c *streamConn) LocalAddr() net.Addr {
	addr, err := manet.ToNetAddr(c.Stream.Conn().LocalMultiaddr())
	if err != nil {
		return fakeRemoteAddr()
	}

	return &wrapLibp2pAddr{Addr: addr, id: c.Stream.Conn().LocalPeer()}
}

// RemoteAddr returns the remote address.
func (c *streamConn) RemoteAddr() net.Addr {
	addr, err := manet.ToNetAddr(c.Stream.Conn().RemoteMultiaddr())
	if err != nil {
		return fakeRemoteAddr()
	}

	return &wrapLibp2pAddr{Addr: addr, id: c.Stream.Conn().RemotePeer()}
}

var _ net.Conn = &streamConn{}

// fakeLocalAddr returns a dummy local address.
func fakeLocalAddr() net.Addr {
	return &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	}
}

// fakeRemoteAddr returns a dummy remote address.
func fakeRemoteAddr() net.Addr {
	return &net.TCPAddr{
		IP:   net.ParseIP("127.1.0.1"),
		Port: 0,
	}
}
