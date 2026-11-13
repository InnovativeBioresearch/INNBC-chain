/* START ALPHA CHANGE 2909 -- regress block serving admission ownership and recoverable overload -- */

package syncer

import (
	"context"
	"errors"
	"io"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	rawGrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const blockServingWatchdog = 20 * time.Second

func blockServingAwait[T any](t *testing.T, ch <-chan T, description string) T {
	t.Helper()

	timer := time.NewTimer(blockServingWatchdog)
	defer timer.Stop()

	select {
	case value := <-ch:
		return value
	case <-timer.C:
		t.Fatal(description)
		var zero T
		return zero
	}
}

func blockServingJoin(t *testing.T, workers *sync.WaitGroup) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	blockServingAwait(t, done, "block serving test workers did not finish")
}

func blockServingTestContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), blockServingWatchdog)
	t.Cleanup(cancel)
	return ctx
}

type blockServingConnInfo struct {
	libnetwork.Conn
	id   peer.ID
	addr multiaddr.Multiaddr
}

func (c *blockServingConnInfo) LocalPeer() peer.ID {
	return "local"
}

func (c *blockServingConnInfo) RemotePeer() peer.ID {
	return c.id
}

func (c *blockServingConnInfo) LocalMultiaddr() multiaddr.Multiaddr {
	return c.addr
}

func (c *blockServingConnInfo) RemoteMultiaddr() multiaddr.Multiaddr {
	return c.addr
}

type blockServingTestStream struct {
	libnetwork.Stream
	socket net.Conn
	info   *blockServingConnInfo
}

func (s *blockServingTestStream) Read(data []byte) (int, error) {
	return s.socket.Read(data)
}

func (s *blockServingTestStream) Write(data []byte) (int, error) {
	return s.socket.Write(data)
}

func (s *blockServingTestStream) Conn() libnetwork.Conn {
	return s.info
}

func (s *blockServingTestStream) Close() error {
	return s.socket.Close()
}

func (s *blockServingTestStream) Reset() error {
	return s.socket.Close()
}

func (s *blockServingTestStream) SetDeadline(deadline time.Time) error {
	return s.socket.SetDeadline(deadline)
}

func (s *blockServingTestStream) SetReadDeadline(deadline time.Time) error {
	return s.socket.SetReadDeadline(deadline)
}

func (s *blockServingTestStream) SetWriteDeadline(deadline time.Time) error {
	return s.socket.SetWriteDeadline(deadline)
}

type blockServingTransport struct {
	stream *netgrpc.GrpcStream

	lock        sync.Mutex
	closing     bool
	sockets     []net.Conn
	connections []*rawGrpc.ClientConn
	handoffs    sync.WaitGroup
}

func newBlockServingTransport(
	t *testing.T,
	service proto.SyncPeerServer,
) *blockServingTransport {
	t.Helper()

	transport := &blockServingTransport{stream: netgrpc.NewGrpcStream()}
	transport.stream.SetPeerAuthorizer(func(id peer.ID) bool {
		return id != ""
	})

	owned, ownsService := service.(*syncPeerService)
	if ownsService {
		owned.lifecycleLock.Lock()
		existing := owned.stream
		if existing == nil {
			owned.stream = transport.stream
		}
		owned.lifecycleLock.Unlock()
		require.Nil(t, existing)
	}

	proto.RegisterSyncPeerServer(transport.stream.GrpcServer(), service)
	transport.stream.Serve()

	t.Cleanup(func() {
		transport.lock.Lock()
		transport.closing = true
		connections := append([]*rawGrpc.ClientConn(nil), transport.connections...)
		sockets := append([]net.Conn(nil), transport.sockets...)
		transport.lock.Unlock()

		for _, conn := range connections {
			_ = conn.Close()
		}
		for _, socket := range sockets {
			_ = socket.Close()
		}

		done := make(chan error, 1)
		go func() {
			if ownsService {
				done <- owned.Close()
			} else {
				transport.stream.Stop()
				done <- nil
			}
		}()

		require.NoError(t, blockServingAwait(t, done,
			"block serving transport shutdown did not finish"))
		blockServingJoin(t, &transport.handoffs)
	})

	return transport
}

func (transport *blockServingTransport) dial(
	parent context.Context,
	id peer.ID,
) (*rawGrpc.ClientConn, error) {
	setup, cancel := context.WithTimeout(parent, blockServingWatchdog)
	defer cancel()

	clientSocket, serverSocket := net.Pipe()
	addr := multiaddr.StringCast("/ip4/127.0.0.1/tcp/1234")

	clientStream := &blockServingTestStream{
		socket: clientSocket,
		info:   &blockServingConnInfo{id: "server", addr: addr},
	}
	serverStream := &blockServingTestStream{
		socket: serverSocket,
		info:   &blockServingConnInfo{id: id, addr: addr},
	}

	transport.lock.Lock()
	if transport.closing {
		transport.lock.Unlock()
		_ = clientSocket.Close()
		_ = serverSocket.Close()
		return nil, net.ErrClosed
	}
	transport.sockets = append(transport.sockets, clientSocket, serverSocket)
	transport.handoffs.Add(1)
	transport.lock.Unlock()

	go func() {
		defer transport.handoffs.Done()
		transport.stream.Handler()(serverStream)
	}()

	conn, err := netgrpc.WrapClientContext(setup, clientStream)
	if err != nil {
		_ = clientSocket.Close()
		_ = serverSocket.Close()
		return nil, err
	}

	transport.lock.Lock()
	if transport.closing {
		transport.lock.Unlock()
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	transport.connections = append(transport.connections, conn)
	transport.lock.Unlock()

	return conn, nil
}

func blockServingDial(
	t *testing.T,
	transport *blockServingTransport,
	id peer.ID,
) *rawGrpc.ClientConn {
	t.Helper()

	conn, err := transport.dial(blockServingTestContext(t), id)
	require.NoError(t, err)
	return conn
}

/*
Production-handler fixtures must carry transport identity.
Other RPC lifetime fixtures can retain their anonymous bufconn scope.
*/
func newBlockServingRPCNetwork(
	t *testing.T,
	service *syncPeerService,
) (*rpcLifecycleNetwork, <-chan *rawGrpc.ClientConn) {
	t.Helper()

	transport := newBlockServingTransport(t, service)
	connections := make(chan *rawGrpc.ClientConn, 8)

	return &rpcLifecycleNetwork{
		dial: func(ctx context.Context) (*rawGrpc.ClientConn, error) {
			conn, err := transport.dial(ctx, "peer")
			if err != nil {
				return nil, err
			}

			select {
			case connections <- conn:
				return conn, nil
			case <-ctx.Done():
				_ = conn.Close()
				return nil, ctx.Err()
			}
		},
	}, connections
}

func blockServingCounts(service *syncPeerService) (int, map[peer.ID]int) {
	service.lifecycleLock.Lock()
	defer service.lifecycleLock.Unlock()

	peers := make(map[peer.ID]int, len(service.blockServingByPeer))
	for id, count := range service.blockServingByPeer {
		peers[id] = count
	}
	return service.activeBlockServing, peers
}

func blockServingEmpty(t *testing.T, service *syncPeerService) {
	t.Helper()

	active, peers := blockServingCounts(service)
	require.Zero(t, active)
	require.Empty(t, peers)
}

type blockServingIdentityService struct {
	proto.UnimplementedSyncPeerServer
	contexts chan context.Context
}

func (s *blockServingIdentityService) GetStatus(
	ctx context.Context,
	_ *emptypb.Empty,
) (*proto.SyncPeerStatus, error) {
	/*
		Retain transport values for isolated accounting tests.
		These tests explicitly control cancellation separately.
	*/
	s.contexts <- context.WithoutCancel(ctx)
	return &proto.SyncPeerStatus{Number: 1}, nil
}

func blockServingOwnedContexts(
	t *testing.T,
	ids ...peer.ID,
) map[peer.ID]context.Context {
	t.Helper()

	service := &blockServingIdentityService{
		contexts: make(chan context.Context, len(ids)),
	}
	transport := newBlockServingTransport(t, service)
	contexts := make(map[peer.ID]context.Context, len(ids))

	for _, id := range ids {
		conn := blockServingDial(t, transport, id)
		_, err := proto.NewSyncPeerClient(conn).GetStatus(
			blockServingTestContext(t), &emptypb.Empty{},
		)
		require.NoError(t, err)

		ctx := blockServingAwait(t, service.contexts, "transport identity was not captured")
		actual, err := netgrpc.PeerIDFromContext(ctx)
		require.NoError(t, err)
		require.Equal(t, id, actual)
		contexts[id] = ctx
	}
	return contexts
}

func TestBlockServing_AtomicLastSlots(t *testing.T) {
	require.Equal(t, 8, maxActiveBlockServing)
	require.Equal(t, 2, maxActiveBlockServingPerPeer)

	for _, global := range []bool{false, true} {
		name := "per_peer"
		if global {
			name = "global"
		}

		t.Run(name, func(t *testing.T) {
			contexts := blockServingOwnedContexts(t, "a", "b", "c", "d", "e")
			service := &syncPeerService{}

			prefill := []peer.ID{"a"}
			contenders := []peer.ID{"a", "a"}
			if global {
				prefill = []peer.ID{"a", "a", "b", "b", "c", "c", "d"}
				contenders = []peer.ID{"d", "e"}
			}

			var releases []func()
			for _, id := range prefill {
				release, err := service.beginBlockServing(contexts[id])
				require.NoError(t, err)
				t.Cleanup(release)
				releases = append(releases, release)
			}

			type result struct {
				release func()
				err     error
			}
			results := make(chan result, 2)
			start := make(chan struct{})
			var workers sync.WaitGroup

			for _, id := range contenders {
				workers.Add(1)
				go func(id peer.ID) {
					defer workers.Done()
					<-start
					release, err := service.beginBlockServing(contexts[id])
					results <- result{release: release, err: err}
				}(id)
			}
			close(start)

			accepted := 0
			rejected := 0
			for i := 0; i < 2; i++ {
				next := blockServingAwait(t, results, "concurrent admission did not finish")
				if next.err == nil {
					accepted++
					require.NotNil(t, next.release)
					t.Cleanup(next.release)
					releases = append(releases, next.release)
				} else {
					rejected++
					require.Nil(t, next.release)
					require.Equal(t, codes.ResourceExhausted, status.Code(next.err))
				}
			}
			blockServingJoin(t, &workers)
			require.Equal(t, 1, accepted)
			require.Equal(t, 1, rejected)

			for _, release := range releases {
				release()
			}
			blockServingEmpty(t, service)
		})
	}
}

func TestBlockServing_RepeatedCleanupPreservesLaterReservation(t *testing.T) {
	ctx := blockServingOwnedContexts(t, "a")["a"]
	service := &syncPeerService{}

	oldRelease, err := service.beginBlockServing(ctx)
	require.NoError(t, err)
	t.Cleanup(oldRelease)
	oldRelease()

	newRelease, err := service.beginBlockServing(ctx)
	require.NoError(t, err)
	t.Cleanup(newRelease)

	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			oldRelease()
		}()
	}
	blockServingJoin(t, &workers)

	active, peers := blockServingCounts(service)
	require.Equal(t, 1, active)
	require.Equal(t, 1, peers[peer.ID("a")])

	newRelease()
	blockServingEmpty(t, service)

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	release, err := service.beginBlockServing(cancelled)
	require.Nil(t, release)
	require.Equal(t, codes.Canceled, status.Code(err))
	blockServingEmpty(t, service)

	require.NoError(t, service.Close())
	release, err = service.beginBlockServing(ctx)
	require.Nil(t, release)
	require.Equal(t, codes.Unavailable, status.Code(err))
	blockServingEmpty(t, service)
}

type blockServingReadGate struct {
	gate         chan struct{}
	release      sync.Once
	entered      chan struct{}
	headerReads  atomic.Int32
	historyReads atomic.Int32
	blockReads   atomic.Int32
}

func newBlockServingReadGate() *blockServingReadGate {
	return &blockServingReadGate{
		gate:    make(chan struct{}),
		entered: make(chan struct{}, 32),
	}
}

func (g *blockServingReadGate) unblock() {
	g.release.Do(func() { close(g.gate) })
}

func (g *blockServingReadGate) chain() *mockBlockchain {
	return &mockBlockchain{
		headerHandler: func() *types.Header {
			g.headerReads.Add(1)
			return &types.Header{Number: 1}
		},
		historyFromHandler: func() uint64 {
			g.historyReads.Add(1)
			return 1
		},
		getBlockByNumberHandler: func(number uint64, _ bool) (*types.Block, bool) {
			g.blockReads.Add(1)
			g.entered <- struct{}{}
			<-g.gate
			return &types.Block{Header: &types.Header{Number: number}}, true
		},
	}
}

func (g *blockServingReadGate) reads() [3]int32 {
	return [3]int32{
		g.headerReads.Load(),
		g.historyReads.Load(),
		g.blockReads.Load(),
	}
}

func blockServingOpen(
	t *testing.T,
	conn *rawGrpc.ClientConn,
	ctx context.Context,
) proto.SyncPeer_GetBlocksClient {
	t.Helper()

	stream, err := proto.NewSyncPeerClient(conn).GetBlocks(
		ctx, &proto.GetBlocksRequest{From: 1},
	)
	require.NoError(t, err)
	return stream
}

func blockServingReject(
	t *testing.T,
	conn *rawGrpc.ClientConn,
	expected codes.Code,
) {
	t.Helper()

	stream, err := proto.NewSyncPeerClient(conn).GetBlocks(
		blockServingTestContext(t), &proto.GetBlocksRequest{From: 1},
	)
	if err == nil {
		_, err = stream.Recv()
	}
	require.Error(t, err)
	require.Equal(t, expected, status.Code(err))
}

func blockServingComplete(t *testing.T, stream proto.SyncPeer_GetBlocksClient) {
	t.Helper()

	block, err := stream.Recv()
	require.NoError(t, err)
	decoded := new(types.Block)
	require.NoError(t, decoded.UnmarshalRLP(block.Block))
	require.Equal(t, uint64(1), decoded.Number())

	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
}

func TestBlockServing_PeerBudgetSharedAcrossRPCsAndConnections(t *testing.T) {
	gate := newBlockServingReadGate()
	service := NewSyncPeerService(nil, gate.chain()).(*syncPeerService)
	transport := newBlockServingTransport(t, service)
	t.Cleanup(gate.unblock)

	firstConn := blockServingDial(t, transport, "a")
	first := blockServingOpen(t, firstConn, blockServingTestContext(t))
	blockServingAwait(t, gate.entered, "first download did not reach storage")

	second := blockServingOpen(t, firstConn, blockServingTestContext(t))
	blockServingAwait(t, gate.entered, "multiplexed download did not reach storage")

	before := gate.reads()
	blockServingReject(t, firstConn, codes.ResourceExhausted)
	require.Equal(t, before, gate.reads())

	otherConn := blockServingDial(t, transport, "a")
	blockServingReject(t, otherConn, codes.ResourceExhausted)
	require.Equal(t, before, gate.reads())

	differentPeer := blockServingDial(t, transport, "b")
	third := blockServingOpen(t, differentPeer, blockServingTestContext(t))
	blockServingAwait(t, gate.entered, "different peer could not use available capacity")

	response, err := proto.NewSyncPeerClient(firstConn).GetStatus(
		blockServingTestContext(t), &emptypb.Empty{},
	)
	require.NoError(t, err)
	require.Equal(t, uint64(1), response.Number)

	gate.unblock()
	blockServingComplete(t, first)
	blockServingComplete(t, second)
	blockServingComplete(t, third)
	blockServingEmpty(t, service)

	retry := blockServingOpen(t, firstConn, blockServingTestContext(t))
	blockServingComplete(t, retry)
	blockServingEmpty(t, service)
}

func TestBlockServing_GlobalBudgetRejectsBeforeReads(t *testing.T) {
	gate := newBlockServingReadGate()
	service := NewSyncPeerService(nil, gate.chain()).(*syncPeerService)
	transport := newBlockServingTransport(t, service)
	t.Cleanup(gate.unblock)

	var streams []proto.SyncPeer_GetBlocksClient
	for _, id := range []peer.ID{"a", "b", "c", "d"} {
		conn := blockServingDial(t, transport, id)
		for i := 0; i < 2; i++ {
			streams = append(streams,
				blockServingOpen(t, conn, blockServingTestContext(t)))
			blockServingAwait(t, gate.entered, "global serving fixture did not reach storage")
		}
	}

	fifthPeer := blockServingDial(t, transport, "e")
	before := gate.reads()
	blockServingReject(t, fifthPeer, codes.ResourceExhausted)
	require.Equal(t, before, gate.reads())

	active, peers := blockServingCounts(service)
	require.Equal(t, 8, active)
	require.Len(t, peers, 4)

	_, err := proto.NewSyncPeerClient(fifthPeer).GetStatus(
		blockServingTestContext(t), &emptypb.Empty{},
	)
	require.NoError(t, err)

	gate.unblock()
	for _, stream := range streams {
		blockServingComplete(t, stream)
	}
	blockServingEmpty(t, service)

	retry := blockServingOpen(t, fifthPeer, blockServingTestContext(t))
	blockServingComplete(t, retry)
	blockServingEmpty(t, service)
}

func TestBlockServing_MissingIdentityRejectsBeforeReads(t *testing.T) {
	gate := newBlockServingReadGate()
	t.Cleanup(gate.unblock)
	service := NewSyncPeerService(nil, gate.chain()).(*syncPeerService)

	/*
		This wrapper deliberately selects the anonymous bufconn fixture.
		There is no identity fallback in the production serving handler.
	*/
	anonymous := &struct{ proto.SyncPeerServer }{SyncPeerServer: service}
	networkStub, _ := newOwnershipRPCNetwork(t, anonymous)
	conn, err := networkStub.NewProtoConnectionContext(
		blockServingTestContext(t), syncerProto, "peer",
	)
	require.NoError(t, err)

	blockServingReject(t, conn, codes.Unauthenticated)
	require.Equal(t, [3]int32{}, gate.reads())
	blockServingEmpty(t, service)
}

type blockServingObservedService struct {
	*syncPeerService
	contexts chan context.Context
	returned chan struct{}
}

func (s *blockServingObservedService) GetBlocks(
	req *proto.GetBlocksRequest,
	stream proto.SyncPeer_GetBlocksServer,
) error {
	s.contexts <- stream.Context()
	defer func() { s.returned <- struct{}{} }()
	return s.syncPeerService.GetBlocks(req, stream)
}

func TestBlockServing_CancelledReadRetainsCapacityUntilReturn(t *testing.T) {
	gate := newBlockServingReadGate()
	service := NewSyncPeerService(nil, gate.chain()).(*syncPeerService)
	observer := &blockServingObservedService{
		syncPeerService: service,
		contexts:        make(chan context.Context, 16),
		returned:        make(chan struct{}, 16),
	}
	transport := newBlockServingTransport(t, observer)
	t.Cleanup(gate.unblock)
	conn := blockServingDial(t, transport, "a")

	ctx, cancel := context.WithCancel(blockServingTestContext(t))
	t.Cleanup(cancel)
	first := blockServingOpen(t, conn, ctx)
	remoteCtx := blockServingAwait(t, observer.contexts, "first handler did not start")
	blockServingAwait(t, gate.entered, "first handler did not enter its read")

	cancel()
	blockServingAwait(t, remoteCtx.Done(), "server did not observe download cancellation")

	second := blockServingOpen(t, conn, blockServingTestContext(t))
	blockServingAwait(t, observer.contexts, "second handler did not start")
	blockServingAwait(t, gate.entered, "second handler did not enter its read")

	active, _ := blockServingCounts(service)
	require.Equal(t, 2, active)

	before := gate.reads()
	blockServingReject(t, conn, codes.ResourceExhausted)
	require.Equal(t, before, gate.reads())

	_, err := first.Recv()
	require.Equal(t, codes.Canceled, status.Code(err))

	gate.unblock()
	blockServingComplete(t, second)
	for i := 0; i < 3; i++ {
		blockServingAwait(t, observer.returned, "serving handler did not return")
	}
	blockServingEmpty(t, service)
}

func TestBlockServing_ShutdownClosesAdmissionAndJoinsRead(t *testing.T) {
	gate := newBlockServingReadGate()
	service := NewSyncPeerService(nil, gate.chain()).(*syncPeerService)
	observer := &blockServingObservedService{
		syncPeerService: service,
		contexts:        make(chan context.Context, 1),
		returned:        make(chan struct{}, 1),
	}
	transport := newBlockServingTransport(t, observer)

	/* Bind the observed production handler to its actual service shutdown. */
	service.stream = transport.stream
	t.Cleanup(gate.unblock)
	conn := blockServingDial(t, transport, "a")
	_ = blockServingOpen(t, conn, blockServingTestContext(t))
	remoteCtx := blockServingAwait(t, observer.contexts, "shutdown handler did not start")
	blockServingAwait(t, gate.entered, "shutdown handler did not enter storage")

	closed := make(chan error, 1)
	go func() { closed <- service.Close() }()

	blockServingAwait(t, remoteCtx.Done(), "shutdown did not cancel the RPC")

	active, _ := blockServingCounts(service)
	require.Equal(t, 1, active)
	release, err := service.beginBlockServing(context.WithoutCancel(remoteCtx))
	require.Nil(t, release)
	require.Equal(t, codes.Unavailable, status.Code(err))

	select {
	case <-closed:
		t.Fatal("shutdown returned while the storage read still owned its allowance")
	default:
	}

	gate.unblock()
	require.NoError(t, blockServingAwait(t, closed, "shutdown did not join the handler"))
	blockServingAwait(t, observer.returned, "shutdown handler did not return")
	blockServingEmpty(t, service)
	require.NoError(t, service.Close())
}

type blockServingStubStream struct {
	rawGrpc.ServerStream
	ctx  context.Context
	send func(*proto.Block) error
}

func (s *blockServingStubStream) Context() context.Context {
	return s.ctx
}

func (s *blockServingStubStream) Send(block *proto.Block) error {
	return s.send(block)
}

func TestBlockServing_HandlerExitsReleaseAllowance(t *testing.T) {
	base := blockServingOwnedContexts(t, "a")["a"]

	for _, name := range []string{
		"success", "missing_header", "invalid_range",
		"missing_block", "send_failure", "cancelled_during_read",
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(base)
			defer cancel()

			sendError := errors.New("controlled send failure")
			var sends atomic.Int32
			chain := &mockBlockchain{
				headerHandler: func() *types.Header {
					if name == "missing_header" {
						return nil
					}
					return &types.Header{Number: 1}
				},
				historyFromHandler: func() uint64 { return 1 },
				getBlockByNumberHandler: func(number uint64, _ bool) (*types.Block, bool) {
					if name == "missing_block" {
						return nil, false
					}
					if name == "cancelled_during_read" {
						cancel()
					}
					return &types.Block{Header: &types.Header{Number: number}}, true
				},
			}
			service := NewSyncPeerService(nil, chain).(*syncPeerService)
			stream := &blockServingStubStream{
				ctx: ctx,
				send: func(*proto.Block) error {
					sends.Add(1)
					if name == "send_failure" {
						return sendError
					}
					return nil
				},
			}
			from := uint64(1)
			if name == "invalid_range" {
				from = 2
			}

			err := service.GetBlocks(&proto.GetBlocksRequest{From: from}, stream)
			switch name {
			case "success":
				require.NoError(t, err)
				require.Equal(t, int32(1), sends.Load())
			case "send_failure":
				require.ErrorIs(t, err, sendError)
			case "cancelled_during_read":
				require.ErrorIs(t, err, context.Canceled)
				require.Zero(t, sends.Load())
			default:
				require.ErrorIs(t, err, ErrBlockNotFound)
			}
			blockServingEmpty(t, service)
		})
	}
}

func TestBlockServing_BlockedSendOwnsAllowance(t *testing.T) {
	base := blockServingOwnedContexts(t, "a")["a"]
	ctx, cancel := context.WithCancel(base)
	t.Cleanup(cancel)

	service := NewSyncPeerService(nil, &mockBlockchain{
		headerHandler:      newSimpleHeaderHandler(1),
		historyFromHandler: func() uint64 { return 1 },
		getBlockByNumberHandler: func(number uint64, _ bool) (*types.Block, bool) {
			return &types.Block{Header: &types.Header{Number: number}}, true
		},
	}).(*syncPeerService)

	entered := make(chan struct{})
	done := make(chan error, 1)
	stream := &blockServingStubStream{
		ctx: ctx,
		send: func(*proto.Block) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	go func() {
		done <- service.GetBlocks(&proto.GetBlocksRequest{From: 1}, stream)
	}()

	blockServingAwait(t, entered, "handler did not reach the controlled blocked send")
	active, _ := blockServingCounts(service)
	require.Equal(t, 1, active)

	cancel()
	require.ErrorIs(t,
		blockServingAwait(t, done, "blocked send did not return after cancellation"),
		context.Canceled)
	blockServingEmpty(t, service)
}

type blockServingRouteNetwork struct {
	Network
	routes map[peer.ID]*blockServingTransport
}

func (*blockServingRouteNetwork) AddrInfo() *peer.AddrInfo {
	return &peer.AddrInfo{ID: "local"}
}

func (*blockServingRouteNetwork) IsPeerAllowed(peer.ID) bool {
	return true
}

func (n *blockServingRouteNetwork) NewProtoConnectionContext(
	ctx context.Context,
	_ string,
	id peer.ID,
) (*rawGrpc.ClientConn, error) {
	transport := n.routes[id]
	if transport == nil {
		return nil, errors.New("missing test serving route")
	}
	return transport.dial(ctx, "peer")
}

type blockServingObservedClient struct {
	SyncPeerClient
	overloads chan error
	forwards  sync.WaitGroup
}

func (c *blockServingObservedClient) GetBlocks(
	id peer.ID,
	from uint64,
	timeout time.Duration,
) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
	blocks, terminal, err := c.SyncPeerClient.GetBlocks(id, from, timeout)
	if err != nil {
		return blocks, terminal, err
	}

	forwarded := make(chan BlockStreamTerminal, 1)
	c.forwards.Add(1)
	go func() {
		defer c.forwards.Done()
		defer close(forwarded)
		for result := range terminal {
			if status.Code(result.Err) == codes.ResourceExhausted {
				c.overloads <- result.Err
			}
			forwarded <- result
		}
	}()
	return blocks, forwarded, nil
}

func (c *blockServingObservedClient) Close() {
	c.SyncPeerClient.Close()
	c.forwards.Wait()
}

func TestBlockServing_SyncRecoversFromStreamingOverload(t *testing.T) {
	for _, laterRetry := range []bool{false, true} {
		name := "another_peer"
		if laterRetry {
			name = "later_retry"
		}

		t.Run(name, func(t *testing.T) {
			gate := newBlockServingReadGate()
			busyService := NewSyncPeerService(nil, gate.chain()).(*syncPeerService)
			busy := newBlockServingTransport(t, busyService)
			t.Cleanup(gate.unblock)

			conn := blockServingDial(t, busy, "peer")
			first := blockServingOpen(t, conn, blockServingTestContext(t))
			blockServingAwait(t, gate.entered, "first occupied allowance did not start")
			second := blockServingOpen(t, conn, blockServingTestContext(t))
			blockServingAwait(t, gate.entered, "second occupied allowance did not start")

			routes := map[peer.ID]*blockServingTransport{"busy": busy}
			peers := []*NoForkPeer{
				{ID: "busy", Number: 1, HistoryFrom: 1, Distance: big.NewInt(0)},
			}

			if !laterRetry {
				healthyService := NewSyncPeerService(nil, &mockBlockchain{
					headerHandler:      newSimpleHeaderHandler(1),
					historyFromHandler: func() uint64 { return 1 },
					getBlockByNumberHandler: func(number uint64, _ bool) (*types.Block, bool) {
						return &types.Block{Header: &types.Header{Number: number}}, true
					},
				}).(*syncPeerService)
				routes["healthy"] = newBlockServingTransport(t, healthyService)
				peers = append(peers, &NoForkPeer{
					ID: "healthy", Number: 1, HistoryFrom: 1, Distance: big.NewInt(1),
				})
			}

			networkStub := &blockServingRouteNetwork{routes: routes}
			actualClient := NewSyncPeerClient(
				hclog.NewNullLogger(), networkStub, nil,
			)
			observed := &blockServingObservedClient{
				SyncPeerClient: actualClient,
				overloads:      make(chan error, 8),
			}

			var localHeight atomic.Uint64
			local := &mockBlockchain{
				headerHandler: func() *types.Header {
					return &types.Header{Number: localHeight.Load()}
				},
				verifyFinalizedBlockHandler: func(block *types.Block) (*types.FullBlock, error) {
					return &types.FullBlock{Block: block}, nil
				},
				writeFullBlockHandler: func(block *types.FullBlock) error {
					localHeight.Store(block.Block.Number())
					return nil
				},
			}

			s := &syncer{
				logger:            hclog.NewNullLogger(),
				blockchain:        local,
				syncProgression:   &mockProgression{},
				peerMap:           NewPeerMap(peers),
				syncPeerService:   &mockSyncPeerService{},
				syncPeerClient:    observed,
				blockTimeout:      5 * time.Second,
				peerRetryInterval: 50 * time.Millisecond,
				newStatusCh:       make(chan struct{}, 1),
				closeCh:           make(chan struct{}),
			}

			/*
				Only this test instance shortens its exhausted-peer retry.
				The production five-second default is unchanged.
			*/
			t.Cleanup(func() {
				done := make(chan error, 1)
				go func() { done <- s.Close() }()
				require.NoError(t, blockServingAwait(t, done,
					"sync recovery fixture did not shut down"))
			})

			result := make(chan error, 1)
			go func() {
				result <- s.Sync(func(*types.FullBlock) bool { return true })
			}()

			err := blockServingAwait(t, observed.overloads,
				"sync client did not observe the real streaming overload")
			require.Equal(t, codes.ResourceExhausted, status.Code(err))

			if laterRetry {
				gate.unblock()
				blockServingComplete(t, first)
				blockServingComplete(t, second)
			}

			require.NoError(t, blockServingAwait(t, result,
				"sync did not recover after streaming overload"))
			require.Equal(t, uint64(1), localHeight.Load())

			if !laterRetry {
				gate.unblock()
				blockServingComplete(t, first)
				blockServingComplete(t, second)
			}
			blockServingEmpty(t, busyService)
		})
	}
}

/* FINISH ALPHA CHANGE 2909 -- regress block serving admission ownership and recoverable overload -- */
