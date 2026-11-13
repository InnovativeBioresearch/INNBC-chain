package syncer

import (
	"context"
	/* START ALPHA CHANGE 2730 -- import real RPC transport support for sync ownership regressions -- */
	"math/big"
	"net"

	/* FINISH ALPHA CHANGE 2730 -- import real RPC transport support for sync ownership regressions -- */
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	/* START ALPHA CHANGE 2730 -- import real RPC transport support for sync ownership regressions -- */
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/innovativebioresearch/innbc-chain/types"

	/* FINISH ALPHA CHANGE 2730 -- import real RPC transport support for sync ownership regressions -- */
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	rawGrpc "google.golang.org/grpc"

	/* START ALPHA CHANGE 2730 -- import real RPC transport support for sync ownership regressions -- */
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	/* FINISH ALPHA CHANGE 2730 -- import real RPC transport support for sync ownership regressions -- */)

/* START ALPHA CHANGE 2723 -- regress sync establishment ownership cancellation and replacement cleanup -- */

type rpcLifecycleNetwork struct {
	Network
	dial func(context.Context) (*rawGrpc.ClientConn, error)
}

func (n *rpcLifecycleNetwork) AddrInfo() *peer.AddrInfo {
	return &peer.AddrInfo{ID: "local"}
}

/* START ALPHA CHANGE 2755 -- declare unrestricted membership for isolated RPC lifetime fixtures -- */
func (*rpcLifecycleNetwork) IsPeerAllowed(peer.ID) bool {
	return true
}

/* FINISH ALPHA CHANGE 2755 -- declare unrestricted membership for isolated RPC lifetime fixtures -- */

func (n *rpcLifecycleNetwork) NewProtoConnectionContext(
	ctx context.Context,
	_ string,
	_ peer.ID,
) (*rawGrpc.ClientConn, error) {
	return n.dial(ctx)
}

func waitSyncRPCSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for sync RPC lifecycle signal")
	}
}

func TestSyncRPCLifetime_CloseCancelsStatusEstablishment(t *testing.T) {
	started := make(chan struct{})
	net := &rpcLifecycleNetwork{
		dial: func(ctx context.Context) (*rawGrpc.ClientConn, error) {
			close(started)
			<-ctx.Done()

			return nil, ctx.Err()
		},
	}
	client := NewSyncPeerClient(hclog.NewNullLogger(), net, nil).(*syncPeerClient)
	t.Cleanup(client.Close)

	result := make(chan error, 1)
	go func() {
		_, err := client.GetPeerStatus("peer")
		result <- err
	}()

	waitSyncRPCSignal(t, started)
	client.Close()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("status establishment survived client closure")
	}

	client.Close()
}

func TestSyncRPCLifetime_CloseStreamPreservesStatusOperation(t *testing.T) {
	statusStarted := make(chan struct{})
	downloadStarted := make(chan struct{})
	var calls atomic.Int32
	var statusCtx context.Context

	net := &rpcLifecycleNetwork{
		dial: func(ctx context.Context) (*rawGrpc.ClientConn, error) {
			if calls.Add(1) == 1 {
				statusCtx = ctx
				close(statusStarted)
			} else {
				/* The download lifetime itself must have no setup deadline. */
				if _, hasDeadline := ctx.Deadline(); hasDeadline {
					return nil, context.DeadlineExceeded
				}
				close(downloadStarted)
			}

			<-ctx.Done()

			return nil, ctx.Err()
		},
	}
	client := NewSyncPeerClient(hclog.NewNullLogger(), net, nil).(*syncPeerClient)
	t.Cleanup(client.Close)

	statusDone := make(chan struct{})
	go func() {
		_, _ = client.GetPeerStatus("peer")
		close(statusDone)
	}()
	waitSyncRPCSignal(t, statusStarted)

	downloadDone := make(chan struct{})
	go func() {
		_, _, _ = client.GetBlocks("peer", 1, time.Second)
		close(downloadDone)
	}()
	waitSyncRPCSignal(t, downloadStarted)

	require.NoError(t, client.CloseStream("peer"))
	waitSyncRPCSignal(t, downloadDone)

	select {
	case <-statusCtx.Done():
		t.Fatal("download cancellation cancelled the independent status operation")
	default:
	}

	client.Close()
	waitSyncRPCSignal(t, statusDone)
}

func TestSyncRPCLifetime_LateCleanupPreservesReplacement(t *testing.T) {
	firstStarted := make(chan struct{})
	firstCancelled := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	var releaseOnce sync.Once
	var calls atomic.Int32

	net := &rpcLifecycleNetwork{
		dial: func(ctx context.Context) (*rawGrpc.ClientConn, error) {
			if calls.Add(1) == 1 {
				close(firstStarted)
				<-ctx.Done()
				close(firstCancelled)
				<-releaseFirst
			} else {
				close(secondStarted)
				<-ctx.Done()
			}

			return nil, ctx.Err()
		},
	}
	client := NewSyncPeerClient(hclog.NewNullLogger(), net, nil).(*syncPeerClient)
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseFirst) })
		client.Close()
	})

	firstDone := make(chan struct{})
	go func() {
		_, _, _ = client.GetBlocks("peer", 1, time.Second)
		close(firstDone)
	}()
	waitSyncRPCSignal(t, firstStarted)

	secondDone := make(chan struct{})
	go func() {
		_, _, _ = client.GetBlocks("peer", 1, time.Second)
		close(secondDone)
	}()
	waitSyncRPCSignal(t, secondStarted)
	waitSyncRPCSignal(t, firstCancelled)

	client.blockStreamLock.Lock()
	replacement := client.blockStreamCancels["peer"]
	client.blockStreamLock.Unlock()
	require.NotNil(t, replacement)

	releaseOnce.Do(func() { close(releaseFirst) })
	waitSyncRPCSignal(t, firstDone)

	client.blockStreamLock.Lock()
	current := client.blockStreamCancels["peer"]
	client.blockStreamLock.Unlock()
	require.Same(t, replacement, current)

	require.NoError(t, client.CloseStream("peer"))
	waitSyncRPCSignal(t, secondDone)
	client.Close()
}

/* FINISH ALPHA CHANGE 2723 -- regress sync establishment ownership cancellation and replacement cleanup -- */

/* START ALPHA CHANGE 2731 -- regress established sync connection isolation and active receive shutdown -- */

func (*rpcLifecycleNetwork) GetPeerDistance(peer.ID) *big.Int {
	return new(big.Int)
}

type rpcOwnershipSyncService struct {
	proto.UnimplementedSyncPeerServer
	blocks func(*proto.GetBlocksRequest, proto.SyncPeer_GetBlocksServer) error
}

func (s *rpcOwnershipSyncService) GetBlocks(
	req *proto.GetBlocksRequest,
	stream proto.SyncPeer_GetBlocksServer,
) error {
	return s.blocks(req, stream)
}

func (*rpcOwnershipSyncService) GetStatus(
	context.Context,
	*emptypb.Empty,
) (*proto.SyncPeerStatus, error) {
	return &proto.SyncPeerStatus{Number: 2, HistoryFrom: 1}, nil
}

func newOwnershipRPCNetwork(
	t *testing.T,
	service proto.SyncPeerServer,
) (*rpcLifecycleNetwork, <-chan *rawGrpc.ClientConn) {
	t.Helper()

	/* START ALPHA CHANGE 2911 -- supply identified transports to production block serving fixtures -- */
	if production, ok := service.(*syncPeerService); ok {
		return newBlockServingRPCNetwork(t, production)
	}
	/* FINISH ALPHA CHANGE 2911 -- supply identified transports to production block serving fixtures -- */

	listener := bufconn.Listen(1024 * 1024)
	server := rawGrpc.NewServer(rawGrpc.WaitForHandlers(true))
	proto.RegisterSyncPeerServer(server, service)

	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		_ = server.Serve(listener)
	}()

	connections := make(chan *rawGrpc.ClientConn, 8)
	var connectionsLock sync.Mutex
	var allConnections []*rawGrpc.ClientConn

	t.Cleanup(func() {
		connectionsLock.Lock()
		cached := append([]*rawGrpc.ClientConn(nil), allConnections...)
		connectionsLock.Unlock()

		for _, conn := range cached {
			_ = conn.Close()
		}

		server.Stop()
		_ = listener.Close()
		waitSyncRPCSignal(t, serveDone)
	})

	networkStub := &rpcLifecycleNetwork{
		dial: func(ctx context.Context) (*rawGrpc.ClientConn, error) {
			/* This child deadline bounds test establishment only. */
			setupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			conn, err := rawGrpc.DialContext(
				setupCtx,
				"passthrough:///rpc-ownership",
				rawGrpc.WithTransportCredentials(insecure.NewCredentials()),
				rawGrpc.WithBlock(),
				rawGrpc.WithContextDialer(func(
					dialCtx context.Context,
					_ string,
				) (net.Conn, error) {
					return listener.DialContext(dialCtx)
				}),
			)
			if err != nil {
				return nil, err
			}

			connectionsLock.Lock()
			allConnections = append(allConnections, conn)
			connectionsLock.Unlock()
			connections <- conn

			return conn, nil
		},
	}

	return networkStub, connections
}

func TestSyncRPCOwnership_StatusCleanupPreservesEstablishedDownload(t *testing.T) {
	releaseNext := make(chan struct{})
	var releaseOnce sync.Once
	deadlineSeen := make(chan bool, 1)

	service := &rpcOwnershipSyncService{
		blocks: func(
			req *proto.GetBlocksRequest,
			stream proto.SyncPeer_GetBlocksServer,
		) error {
			_, hasDeadline := stream.Context().Deadline()
			deadlineSeen <- hasDeadline

			first := &types.Block{Header: &types.Header{Number: req.From}}
			if err := stream.Send(toProtoBlock(first)); err != nil {
				return err
			}

			select {
			case <-releaseNext:
			case <-stream.Context().Done():
				return stream.Context().Err()
			}

			second := &types.Block{Header: &types.Header{Number: req.From + 1}}

			return stream.Send(toProtoBlock(second))
		},
	}
	networkStub, connections := newOwnershipRPCNetwork(t, service)
	client := NewSyncPeerClient(hclog.NewNullLogger(), networkStub, nil).(*syncPeerClient)
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseNext) })
		client.Close()
	})

	blocks, terminal, err := client.GetBlocks("peer", 1, 30*time.Second)
	require.NoError(t, err)
	downloadConn := <-connections

	select {
	case hasDeadline := <-deadlineSeen:
		require.False(t, hasDeadline, "download inherited a total establishment deadline")
	case <-time.After(5 * time.Second):
		t.Fatal("download handler did not start")
	}

	readBlock := func() *types.Block {
		t.Helper()

		select {
		case block, open := <-blocks:
			require.True(t, open, "download closed before delivering the expected block")
			require.NotNil(t, block)

			return block
		case <-time.After(5 * time.Second):
			t.Fatal("download did not deliver the expected block")

			return nil
		}
	}
	require.Equal(t, uint64(1), readBlock().Number())

	status, err := client.GetPeerStatus("peer")
	require.NoError(t, err)
	require.Equal(t, uint64(2), status.Number)
	statusConn := <-connections

	require.NotSame(t, downloadConn, statusConn)
	require.Equal(t, connectivity.Shutdown, statusConn.GetState())
	require.Equal(t, connectivity.Ready, downloadConn.GetState())

	/* Delivery after status cleanup demonstrates that the download survives. */
	releaseOnce.Do(func() { close(releaseNext) })
	require.Equal(t, uint64(2), readBlock().Number())

	select {
	case result, open := <-terminal:
		require.True(t, open)
		require.NoError(t, result.Err)
	case <-time.After(5 * time.Second):
		t.Fatal("successful download did not publish completion")
	}

	require.Equal(t, connectivity.Shutdown, downloadConn.GetState())
	client.Close()
}

func TestSyncRPCOwnership_CloseCancelsActiveReceiveAndJoinsReaders(t *testing.T) {
	started := make(chan struct{})
	handlerDone := make(chan struct{})
	abort := make(chan struct{})

	service := &rpcOwnershipSyncService{
		blocks: func(
			_ *proto.GetBlocksRequest,
			stream proto.SyncPeer_GetBlocksServer,
		) error {
			defer close(handlerDone)
			close(started)

			/* No response arrives: the client reader remains inside Recv. */
			select {
			case <-stream.Context().Done():
				return stream.Context().Err()
			case <-abort:
				return context.Canceled
			}
		},
	}
	networkStub, connections := newOwnershipRPCNetwork(t, service)
	client := NewSyncPeerClient(hclog.NewNullLogger(), networkStub, nil).(*syncPeerClient)
	t.Cleanup(func() {
		close(abort)
		client.Close()
	})

	blocks, terminal, err := client.GetBlocks("peer", 1, 30*time.Second)
	require.NoError(t, err)
	conn := <-connections
	waitSyncRPCSignal(t, started)

	closed := make(chan struct{})
	go func() {
		client.Close()
		close(closed)
	}()

	waitSyncRPCSignal(t, handlerDone)
	waitSyncRPCSignal(t, closed)
	require.Equal(t, connectivity.Shutdown, conn.GetState())

	select {
	case result, open := <-terminal:
		require.True(t, open)
		require.Error(t, result.Err)
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled download did not publish completion")
	}

	select {
	case block, open := <-blocks:
		require.False(t, open)
		require.Nil(t, block)
	case <-time.After(5 * time.Second):
		t.Fatal("download output survived joined shutdown")
	}

	client.blockStreamLock.Lock()
	remaining := len(client.blockStreamCancels)
	client.blockStreamLock.Unlock()
	require.Zero(t, remaining)

	_, _, err = client.GetBlocks("peer", 1, time.Second)
	require.ErrorIs(t, err, netgrpc.ErrRPCClosed)
	client.Close()
}

/* FINISH ALPHA CHANGE 2731 -- regress established sync connection isolation and active receive shutdown -- */
