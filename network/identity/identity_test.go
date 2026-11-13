package identity

import (
	"context"
	/* START ALPHA CHANGE 2726 -- import identity shutdown regression support -- */
	"sync"
	"sync/atomic"

	/* FINISH ALPHA CHANGE 2726 -- import identity shutdown regression support -- */
	"testing"
	/* START ALPHA CHANGE 2726 -- import identity shutdown regression support -- */
	"time"
	/* FINISH ALPHA CHANGE 2726 -- import identity shutdown regression support -- */

	"github.com/hashicorp/go-hclog"
	/* START ALPHA CHANGE 2726 -- import identity shutdown regression support -- */
	"github.com/innovativebioresearch/innbc-chain/network/event"
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"

	/* FINISH ALPHA CHANGE 2726 -- import identity shutdown regression support -- */
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	networkTesting "github.com/innovativebioresearch/innbc-chain/network/testing"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"

	/* START ALPHA CHANGE 2726 -- import identity shutdown regression support -- */
	"github.com/stretchr/testify/require"
	/* FINISH ALPHA CHANGE 2726 -- import identity shutdown regression support -- */
	"google.golang.org/grpc"
)

// newIdentityService creates a new identity service instance
// with mock-able backends
func newIdentityService(
	networkingServerCallback func(server *networkTesting.MockNetworkingServer),
) *IdentityService {
	baseServer := networkTesting.NewMockNetworkingServer()

	if networkingServerCallback != nil {
		networkingServerCallback(baseServer)
	}

	return &IdentityService{
		baseServer: baseServer,
		logger:     hclog.NewNullLogger(),
	}
}

// TestTemporaryDial tests temporary peer connections,
// by making sure temporary dials aren't saved as persistent peers
func TestTemporaryDial(t *testing.T) {
	peersArray := make([]peer.ID, 0)

	// Create an instance of the identity service
	identityService := newIdentityService(
		// Set the relevant hook responses from the mock server
		func(server *networkTesting.MockNetworkingServer) {
			// Define the temporary dial hook
			server.HookIsTemporaryDial(func(peerID peer.ID) bool {
				return true
			})

			// Define the add peer hook
			server.HookAddPeer(func(
				id peer.ID,
				direction network.Direction,
			) {
				peersArray = append(peersArray, id)
			})

			// Define the mock IdentityClient response
			server.GetMockIdentityClient().HookHello(func(
				ctx context.Context,
				in *proto.Status,
				opts ...grpc.CallOption,
			) (*proto.Status, error) {
				return &proto.Status{
					Chain:         0,
					TemporaryDial: true, // make sure the dial is temporary
				}, nil
			})
		},
	)

	// Check that there was no error during handshaking
	assert.NoError(
		t,
		identityService.handleConnected("TestPeer", network.DirInbound),
	)

	// Make sure no peers have been  added to the base networking server
	assert.Len(t, peersArray, 0)
}

// TestHandshake_Errors tests peer connections errors
func TestHandshake_Errors(t *testing.T) {
	peersArray := make([]peer.ID, 0)
	requesterChainID := int64(1)
	responderChainID := requesterChainID + 1 // different chain ID

	// Create an instance of the identity service
	identityService := newIdentityService(
		// Set the relevant hook responses from the mock server
		func(server *networkTesting.MockNetworkingServer) {
			// Define the add peer hook
			server.HookAddPeer(func(
				id peer.ID,
				direction network.Direction,
			) {
				peersArray = append(peersArray, id)
			})

			// Define the mock IdentityClient response
			server.GetMockIdentityClient().HookHello(func(
				ctx context.Context,
				in *proto.Status,
				opts ...grpc.CallOption,
			) (*proto.Status, error) {
				return &proto.Status{
					Chain:         responderChainID,
					TemporaryDial: false,
				}, nil
			})
		},
	)

	// Set the requester chain ID
	identityService.chainID = requesterChainID

	// Check that there was a chain ID mismatch during handshaking
	connectErr := identityService.handleConnected("TestPeer", network.DirInbound)
	if connectErr == nil {
		t.Fatalf("no connection error occurred")
	}

	assert.ErrorIs(t, connectErr, ErrInvalidChainID)

	// Make sure no peers have been  added to the base networking server
	assert.Len(t, peersArray, 0)
}

/* START ALPHA CHANGE 2727 -- regress identity shutdown through RPC cleanup and final event delivery -- */

type rpcIdentityOwnerServer struct {
	*networkTesting.MockNetworkingServer
	cleanup func()
}

func (s *rpcIdentityOwnerServer) NewIdentityClient(
	ctx context.Context,
	id peer.ID,
) (proto.IdentityClient, func(), error) {
	client, _, err := s.MockNetworkingServer.NewIdentityClient(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	return client, s.cleanup, nil
}

type rpcIdentityNotifyConn struct {
	network.Conn
}

/* START ALPHA CHANGE 2747 -- identify the existing notification fixture by connection -- */
func (*rpcIdentityNotifyConn) ID() string          { return "identity-lifetime" }
func (*rpcIdentityNotifyConn) RemotePeer() peer.ID { return "peer" }

/* FINISH ALPHA CHANGE 2747 -- identify the existing notification fixture by connection -- */

func (*rpcIdentityNotifyConn) Stat() network.ConnStats {
	return network.ConnStats{
		Stats: network.Stats{Direction: network.DirOutbound},
	}
}

func waitIdentityRPCSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for identity RPC lifecycle signal")
	}
}

func TestIdentityRPCLifetime_CloseJoinsCleanupAndFinalEvent(t *testing.T) {
	mock := networkTesting.NewMockNetworkingServer()
	started := make(chan struct{})
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	releaseEvent := make(chan struct{})
	abort := make(chan struct{})
	events := make(chan *event.PeerEvent, 1)

	var helloCalls atomic.Int32
	var cleanupCalls atomic.Int32
	var peersAdded atomic.Int32
	var pending atomic.Int64
	var startOnce sync.Once
	var cleanupStartOnce sync.Once
	var releaseCleanupOnce sync.Once
	var releaseEventOnce sync.Once

	mock.GetMockIdentityClient().HookHello(func(
		ctx context.Context,
		_ *proto.Status,
		_ ...grpc.CallOption,
	) (*proto.Status, error) {
		helloCalls.Add(1)
		startOnce.Do(func() { close(started) })

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-abort:
			return nil, context.Canceled
		}
	})
	mock.HookAddPeer(func(peer.ID, network.Direction) {
		peersAdded.Add(1)
	})
	mock.HookUpdatePendingConnCount(func(delta int64, _ network.Direction) {
		pending.Add(delta)
	})
	mock.HookEmitEvent(func(ev *event.PeerEvent) {
		events <- ev
		<-releaseEvent
	})

	base := &rpcIdentityOwnerServer{
		MockNetworkingServer: mock,
		cleanup: func() {
			cleanupCalls.Add(1)
			cleanupStartOnce.Do(func() { close(cleanupStarted) })
			<-releaseCleanup
		},
	}
	service := NewIdentityService(base, hclog.NewNullLogger(), 1, "local")
	t.Cleanup(func() {
		close(abort)
		releaseCleanupOnce.Do(func() { close(releaseCleanup) })
		releaseEventOnce.Do(func() { close(releaseEvent) })
		service.Close()
	})

	notify := service.GetNotifyBundle()
	conn := &rpcIdentityNotifyConn{}
	notify.ConnectedF(nil, conn)
	waitIdentityRPCSignal(t, started)

	/* A second notification must not create a second pending handshake. */
	notify.ConnectedF(nil, conn)
	require.Equal(t, int32(1), helloCalls.Load())
	require.Equal(t, int64(1), pending.Load())

	closed := make(chan struct{})
	go func() {
		service.Close()
		close(closed)
	}()
	waitIdentityRPCSignal(t, cleanupStarted)

	select {
	case <-closed:
		t.Fatal("identity shutdown returned before connection cleanup")
	case <-time.After(20 * time.Millisecond):
	}

	releaseCleanupOnce.Do(func() { close(releaseCleanup) })

	select {
	case ev := <-events:
		require.Equal(t, peer.ID("peer"), ev.PeerID)
		require.Equal(t, event.PeerFailedToConnect, ev.Type)
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled handshake did not reach its final event")
	}

	select {
	case <-closed:
		t.Fatal("identity shutdown returned before final event delivery completed")
	case <-time.After(20 * time.Millisecond):
	}

	releaseEventOnce.Do(func() { close(releaseEvent) })
	waitIdentityRPCSignal(t, closed)

	require.Equal(t, int32(1), cleanupCalls.Load())
	require.Zero(t, peersAdded.Load())
	require.Zero(t, pending.Load())
	require.False(t, service.hasPendingStatus("peer"))

	err := service.handleConnected("peer", network.DirOutbound)
	require.ErrorIs(t, err, netgrpc.ErrRPCClosed)
	service.Close()
}

/* FINISH ALPHA CHANGE 2727 -- regress identity shutdown through RPC cleanup and final event delivery -- */
