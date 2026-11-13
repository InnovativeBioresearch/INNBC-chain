package discovery

import (
	/* START ALPHA CHANGE 2728 -- import discovery RPC shutdown regression support -- */
	"context"
	/* FINISH ALPHA CHANGE 2728 -- import discovery RPC shutdown regression support -- */
	"errors"
	/* START ALPHA CHANGE 2728 -- import discovery RPC shutdown regression support -- */
	"sync"
	/* FINISH ALPHA CHANGE 2728 -- import discovery RPC shutdown regression support -- */
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/helper/tests"

	/* START ALPHA CHANGE 2728 -- import discovery RPC shutdown regression support -- */
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	/* FINISH ALPHA CHANGE 2728 -- import discovery RPC shutdown regression support -- */
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	networkTesting "github.com/innovativebioresearch/innbc-chain/network/testing"
	kb "github.com/libp2p/go-libp2p-kbucket"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"

	/* START ALPHA CHANGE 2728 -- import discovery RPC shutdown regression support -- */
	"github.com/stretchr/testify/require"
	rawGrpc "google.golang.org/grpc"
	/* FINISH ALPHA CHANGE 2728 -- import discovery RPC shutdown regression support -- */)

// newDiscoveryService creates a new discovery service instance
// with mock-able backends
func newDiscoveryService(
	networkingServerCallback func(server *networkTesting.MockNetworkingServer),
) (*DiscoveryService, error) {
	// Setup the mock networking server
	baseServer := networkTesting.NewMockNetworkingServer()

	if networkingServerCallback != nil {
		networkingServerCallback(baseServer)
	}

	// Setup the kademlia routing table
	routingTable, routingErr := kb.NewRoutingTable(
		10,
		kb.ConvertPeerID("ExampleID"),
		time.Minute,
		baseServer.GetMockPeerMetrics(),
		10*time.Second,
		nil,
	)
	if routingErr != nil {
		return nil, routingErr
	}

	return &DiscoveryService{
		baseServer:   baseServer,
		logger:       hclog.NewNullLogger(),
		routingTable: routingTable,
		/* START ALPHA CHANGE 1141 -- initialize discovery test close channel -- */
		closeCh: make(chan struct{}),
		/* FINISH ALPHA CHANGE 1141 -- initialize discovery test close channel -- */
	}, nil
}

// getRandomPeers returns random peers, generated on the fly
func getRandomPeers(t *testing.T, count int) []*peer.AddrInfo {
	t.Helper()

	peersInfo := make([]*peer.AddrInfo, 0)

	for i := 0; i < count; i++ {
		info, err := peer.AddrInfoFromP2pAddr(
			tests.GenerateTestMultiAddr(t),
		)
		if err != nil {
			t.Fatalf("unable to generate peer info, %v", err)
		}

		peersInfo = append(peersInfo, info)
	}

	return peersInfo
}

// TestDiscoveryService_BootnodePeerDiscovery makes sure the
// discovery service's peer discovery mechanism through the bootnode works as
// expected
/* START ALPHA CHANGE 664 -- remove legacy bootnode peer discovery unit test -- */
/* FINISH ALPHA CHANGE 664 -- remove legacy bootnode peer discovery unit test -- */

/* START ALPHA CHANGE 1142 -- test discovery service close idempotency -- */
func TestDiscoveryService_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	discoveryService, setupErr := newDiscoveryService(nil)
	if setupErr != nil {
		t.Fatalf("Unable to setup the discovery service")
	}

	assert.NotPanics(t, func() {
		discoveryService.Close()
		discoveryService.Close()
	})
}

/* FINISH ALPHA CHANGE 1142 -- test discovery service close idempotency -- */

// TestDiscoveryService_AddToTable tests that peers are added correctly
// both to the discovery routing table, and the networking server's peer store
func TestDiscoveryService_AddToTable(t *testing.T) {
	randomPeer := getRandomPeers(t, 1)[0]
	peerStore := make(map[peer.ID]*peer.AddrInfo)

	addToPeerStoreHook := func(info *peer.AddrInfo) {
		peerStore[info.ID] = info
	}

	removeFromPeerStoreHook := func(info *peer.AddrInfo) {
		delete(peerStore, info.ID)
	}

	highLatencyHook := func(id peer.ID) time.Duration {
		// Set a high latency value to trigger an error
		return 24 * time.Hour
	}

	lowLatencyHook := func(id peer.ID) time.Duration {
		// Set a non-existing latency value
		return 0
	}

	testTable := []struct {
		name          string
		latencyHook   func(id peer.ID) time.Duration
		shouldSucceed bool
	}{
		{
			"routing table and peer store updated",
			lowLatencyHook,
			true,
		},
		{
			"peer store should not be updated due to error",
			highLatencyHook,
			false,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			// Create an instance of the identity service
			discoveryService, setupErr := newDiscoveryService(
				// Set the relevant hook responses from the mock server
				func(server *networkTesting.MockNetworkingServer) {
					// Define the peer store addition
					server.HookAddToPeerStore(addToPeerStoreHook)

					// Define the peer store deletion
					server.HookRemoveFromPeerStore(removeFromPeerStoreHook)

					// Define the routing table latency hook
					server.GetMockPeerMetrics().HookLatencyEWMA(testCase.latencyHook)
				},
			)
			if setupErr != nil {
				t.Fatalf("Unable to setup the discovery service")
			}

			// Run the main method
			additionErr := discoveryService.addToTable(randomPeer)

			if testCase.shouldSucceed {
				assert.NoError(t, additionErr)
				assert.Len(t, peerStore, 1)
				assert.Equal(t, randomPeer, peerStore[randomPeer.ID])
			} else {
				assert.Error(t, additionErr)
				assert.Len(t, peerStore, 0)
			}
		})
	}
}

// TestDiscoveryService_RegularPeerDiscoveryUnconnected makes sure the peers who disconnected
// in the middle of peer discovery are not queried for their peer sets
func TestDiscoveryService_RegularPeerDiscoveryUnconnected(t *testing.T) {
	randomPeer := getRandomPeers(t, 1)[0]
	peerStore := make(map[peer.ID]*peer.AddrInfo)

	// Create an instance of the identity service
	discoveryService, setupErr := newDiscoveryService(
		// Set the relevant hook responses from the mock server
		func(server *networkTesting.MockNetworkingServer) {
			// Define the random peer hook
			server.HookGetRandomPeer(func() *peer.ID {
				return &randomPeer.ID
			})

			// Define the new discovery client creation
			server.HookNewDiscoveryClient(func(id peer.ID) (proto.DiscoveryClient, error) {
				return nil, errors.New("peer is not connected anymore")
			})

			// Define the peer store addition
			server.HookAddToPeerStore(func(info *peer.AddrInfo) {
				peerStore[info.ID] = info
			})
		},
	)
	if setupErr != nil {
		t.Fatalf("Unable to setup the discovery service")
	}

	// Run the regular peer discovery method
	discoveryService.regularPeerDiscovery()

	// Make sure that no peers were added to the peer store
	assert.Len(t, peerStore, 0)
}

/* START ALPHA CHANGE 2729 -- regress discovery cancellation after establishment and joined connection cleanup -- */

type rpcDiscoveryOwnerServer struct {
	*networkTesting.MockNetworkingServer
	cleanup func()
}

func (s *rpcDiscoveryOwnerServer) NewDiscoveryClient(
	ctx context.Context,
	id peer.ID,
) (proto.DiscoveryClient, func(), error) {
	client, _, err := s.MockNetworkingServer.NewDiscoveryClient(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	return client, s.cleanup, nil
}

func waitDiscoveryRPCSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for discovery RPC lifecycle signal")
	}
}

func TestDiscoveryRPCLifetime_CloseJoinsEstablishedRPCCleanup(t *testing.T) {
	mock := networkTesting.NewMockNetworkingServer()
	started := make(chan struct{})
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	abort := make(chan struct{})
	var releaseOnce sync.Once

	mock.GetMockDiscoveryClient().HookFindPeers(func(
		ctx context.Context,
		_ *proto.FindPeersReq,
		_ ...rawGrpc.CallOption,
	) (*proto.FindPeersResp, error) {
		close(started)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-abort:
			return nil, context.Canceled
		}
	})

	base := &rpcDiscoveryOwnerServer{
		MockNetworkingServer: mock,
		cleanup: func() {
			close(cleanupStarted)
			<-releaseCleanup
		},
	}
	service := &DiscoveryService{
		baseServer: base,
		logger:     hclog.NewNullLogger(),
		closeCh:    make(chan struct{}),
	}
	t.Cleanup(func() {
		close(abort)
		releaseOnce.Do(func() { close(releaseCleanup) })
		service.Close()
	})

	result := make(chan error, 1)
	go func() {
		_, err := service.findPeersCall("peer")
		result <- err
	}()
	waitDiscoveryRPCSignal(t, started)

	closed := make(chan struct{})
	go func() {
		service.Close()
		close(closed)
	}()
	waitDiscoveryRPCSignal(t, cleanupStarted)

	select {
	case <-closed:
		t.Fatal("discovery shutdown returned while connection cleanup was pending")
	case <-time.After(20 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(releaseCleanup) })
	waitDiscoveryRPCSignal(t, closed)

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled discovery RPC did not return")
	}

	_, err := service.findPeersCall("peer")
	require.ErrorIs(t, err, netgrpc.ErrRPCClosed)
	service.Close()
}

/* FINISH ALPHA CHANGE 2729 -- regress discovery cancellation after establishment and joined connection cleanup -- */
