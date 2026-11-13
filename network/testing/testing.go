package testing

import (
	"context"
	/* START ALPHA CHANGE 2869 -- support atomic reservation in isolated identity mocks -- */
	"sync"
	/* FINISH ALPHA CHANGE 2869 -- support atomic reservation in isolated identity mocks -- */
	"time"

	"github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/grpc"
)

type MockNetworkingServer struct {
	/* START ALPHA CHANGE 2870 -- coordinate mock capacity inspection and pending accounting -- */
	reservationLock sync.Mutex
	/* FINISH ALPHA CHANGE 2870 -- coordinate mock capacity inspection and pending accounting -- */

	// Mock identity client that simulates another peer
	mockIdentityClient *MockIdentityClient

	// Mock discovery client that simulates another peer
	mockDiscoveryClient *MockDiscoveryClient

	// Mock libp2p peer metrics
	mockPeerMetrics *MockPeerMetrics

	// Hooks that the test can set //
	// Identity Hooks
	newIdentityClientFn      newIdentityClientDelegate
	disconnectFromPeerFn     disconnectFromPeerDelegate
	addPeerFn                addPeerDelegate
	updatePendingConnCountFn updatePendingConnCountDelegate
	emitEventFn              emitEventDelegate
	isTemporaryDialFn        isTemporaryDialDelegate
	hasFreeConnectionSlotFn  hasFreeConnectionSlotDelegate
	/* START ALPHA CHANGE 691 -- add validator peer allowlist mock hook -- */
	isPeerAllowedFn isPeerAllowedDelegate
	/* FINISH ALPHA CHANGE 691 -- add validator peer allowlist mock hook -- */

	// Discovery Hooks
	newDiscoveryClientFn newDiscoveryClientDelegate
	/* START ALPHA CHANGE 666 -- remove legacy bootnode mock hooks -- */
	/* FINISH ALPHA CHANGE 666 -- remove legacy bootnode mock hooks -- */
	closeProtocolStreamFn      closeProtocolStreamDelegate
	addToPeerStoreFn           addToPeerStoreDelegate
	removeFromPeerStoreFn      removeFromPeerStoreDelegate
	getPeerInfoFn              getPeerInfoDelegate
	getRandomPeerFn            getRandomPeerDelegate
	fetchAndSetTemporaryDialFn fetchAndSetTemporaryDialDelegate
	removeTemporaryDialFn      removeTemporaryDialDelegate
	temporaryDialPeerFn        temporaryDialPeerDelegate
}

func NewMockNetworkingServer() *MockNetworkingServer {
	return &MockNetworkingServer{
		mockIdentityClient:  &MockIdentityClient{},
		mockDiscoveryClient: &MockDiscoveryClient{},
		mockPeerMetrics:     &MockPeerMetrics{},
	}
}

func (m *MockNetworkingServer) GetMockIdentityClient() *MockIdentityClient {
	return m.mockIdentityClient
}

func (m *MockNetworkingServer) GetMockDiscoveryClient() *MockDiscoveryClient {
	return m.mockDiscoveryClient
}

func (m *MockNetworkingServer) GetMockPeerMetrics() *MockPeerMetrics {
	return m.mockPeerMetrics
}

// Define the mock hooks //
// Required for Identity
type newIdentityClientDelegate func(peer.ID) (proto.IdentityClient, error)
type disconnectFromPeerDelegate func(peer.ID, string)
type addPeerDelegate func(peer.ID, network.Direction)
type updatePendingConnCountDelegate func(int64, network.Direction)
type emitEventDelegate func(*event.PeerEvent)
type isTemporaryDialDelegate func(peer.ID) bool
type hasFreeConnectionSlotDelegate func(network.Direction) bool

/* START ALPHA CHANGE 692 -- add validator peer allowlist mock delegate -- */
type isPeerAllowedDelegate func(peer.ID) bool

/* FINISH ALPHA CHANGE 692 -- add validator peer allowlist mock delegate -- */

// Required for Discovery
/* START ALPHA CHANGE 667 -- remove legacy bootnode mock delegate types -- */
/* FINISH ALPHA CHANGE 667 -- remove legacy bootnode mock delegate types -- */
type newDiscoveryClientDelegate func(peer.ID) (proto.DiscoveryClient, error)
type closeProtocolStreamDelegate func(string, peer.ID) error
type addToPeerStoreDelegate func(*peer.AddrInfo)
type removeFromPeerStoreDelegate func(peerInfo *peer.AddrInfo)
type getPeerInfoDelegate func(peer.ID) *peer.AddrInfo
type getRandomPeerDelegate func() *peer.ID
type fetchAndSetTemporaryDialDelegate func(peer.ID, bool) bool
type removeTemporaryDialDelegate func(peer.ID)
type temporaryDialPeerDelegate func(peerAddrInfo *peer.AddrInfo)

func (m *MockNetworkingServer) TemporaryDialPeer(peerAddrInfo *peer.AddrInfo) {
	if m.temporaryDialPeerFn != nil {
		m.temporaryDialPeerFn(peerAddrInfo)
	}
}

func (m *MockNetworkingServer) HookTemporaryDialPeer(fn temporaryDialPeerDelegate) {
	m.temporaryDialPeerFn = fn
}

/* START ALPHA CHANGE 2703 -- adapt identity mock to operation context and cleanup ownership -- */
func (m *MockNetworkingServer) NewIdentityClient(
	ctx context.Context,
	peerID peer.ID,
) (proto.IdentityClient, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	if m.newIdentityClientFn != nil {
		client, err := m.newIdentityClientFn(peerID)

		return client, func() {}, err
	}

	return m.mockIdentityClient, func() {}, nil
}

/* FINISH ALPHA CHANGE 2703 -- adapt identity mock to operation context and cleanup ownership -- */

func (m *MockNetworkingServer) HookNewIdentityClient(fn newIdentityClientDelegate) {
	m.newIdentityClientFn = fn
}

func (m *MockNetworkingServer) DisconnectFromPeer(peerID peer.ID, reason string) {
	if m.disconnectFromPeerFn != nil {
		m.disconnectFromPeerFn(peerID, reason)
	}
}

func (m *MockNetworkingServer) HookDisconnectFromPeer(fn disconnectFromPeerDelegate) {
	m.disconnectFromPeerFn = fn
}

func (m *MockNetworkingServer) AddPeer(id peer.ID, direction network.Direction) {
	if m.addPeerFn != nil {
		m.addPeerFn(id, direction)
	}
}

func (m *MockNetworkingServer) HookAddPeer(fn addPeerDelegate) {
	m.addPeerFn = fn
}

/* START ALPHA CHANGE 2746 -- adapt isolated identity backends to connection ownership -- */
func (m *MockNetworkingServer) ConnectionContext(
	conn network.Conn,
) (context.Context, error) {
	return context.Background(), nil
}

func (m *MockNetworkingServer) AddPeerConnection(conn network.Conn) error {
	m.AddPeer(conn.RemotePeer(), conn.Stat().Direction)

	return nil
}

func (m *MockNetworkingServer) DisconnectConnection(conn network.Conn, reason string) {
	m.DisconnectFromPeer(conn.RemotePeer(), reason)
}

func (m *MockNetworkingServer) EmitConnectionEvent(
	conn network.Conn,
	kind event.PeerEventType,
) {
	m.EmitEvent(&event.PeerEvent{
		PeerID: conn.RemotePeer(),
		Type:   kind,
	})
}

/* FINISH ALPHA CHANGE 2746 -- adapt isolated identity backends to connection ownership -- */

/* START ALPHA CHANGE 2871 -- adapt isolated identity fixtures to owned reservation cleanup -- */
func (m *MockNetworkingServer) ReserveConnectionSlot(conn network.Conn) (func(), bool) {
	m.reservationLock.Lock()
	defer m.reservationLock.Unlock()

	direction := conn.Stat().Direction
	if !m.HasFreeConnectionSlot(direction) {
		return nil, false
	}

	m.UpdatePendingConnCount(1, direction)

	var once sync.Once

	return func() {
		once.Do(func() {
			m.reservationLock.Lock()
			defer m.reservationLock.Unlock()

			m.UpdatePendingConnCount(-1, direction)
		})
	}, true
}

/* FINISH ALPHA CHANGE 2871 -- adapt isolated identity fixtures to owned reservation cleanup -- */

func (m *MockNetworkingServer) UpdatePendingConnCount(delta int64, direction network.Direction) {
	if m.updatePendingConnCountFn != nil {
		m.updatePendingConnCountFn(delta, direction)
	}
}

func (m *MockNetworkingServer) HookUpdatePendingConnCount(fn updatePendingConnCountDelegate) {
	m.updatePendingConnCountFn = fn
}

func (m *MockNetworkingServer) EmitEvent(event *event.PeerEvent) {
	if m.emitEventFn != nil {
		m.emitEventFn(event)
	}
}

func (m *MockNetworkingServer) HookEmitEvent(fn emitEventDelegate) {
	m.emitEventFn = fn
}

func (m *MockNetworkingServer) IsTemporaryDial(peerID peer.ID) bool {
	if m.isTemporaryDialFn != nil {
		return m.isTemporaryDialFn(peerID)
	}

	return false
}

func (m *MockNetworkingServer) HookIsTemporaryDial(fn isTemporaryDialDelegate) {
	m.isTemporaryDialFn = fn
}

func (m *MockNetworkingServer) HasFreeConnectionSlot(direction network.Direction) bool {
	if m.hasFreeConnectionSlotFn != nil {
		return m.hasFreeConnectionSlotFn(direction)
	}

	return true
}

func (m *MockNetworkingServer) HookHasFreeConnectionSlot(fn hasFreeConnectionSlotDelegate) {
	m.hasFreeConnectionSlotFn = fn
}

/* START ALPHA CHANGE 693 -- add validator peer allowlist mock method -- */
func (m *MockNetworkingServer) IsPeerAllowed(peerID peer.ID) bool {
	if m.isPeerAllowedFn != nil {
		return m.isPeerAllowedFn(peerID)
	}

	return true
}

func (m *MockNetworkingServer) HookIsPeerAllowed(fn isPeerAllowedDelegate) {
	m.isPeerAllowedFn = fn
}

/* FINISH ALPHA CHANGE 693 -- add validator peer allowlist mock method -- */

/* START ALPHA CHANGE 668 -- remove legacy bootnode mock methods -- */
/* FINISH ALPHA CHANGE 668 -- remove legacy bootnode mock methods -- */

/* START ALPHA CHANGE 2704 -- adapt discovery mock to operation context and cleanup ownership -- */
func (m *MockNetworkingServer) NewDiscoveryClient(
	ctx context.Context,
	peerID peer.ID,
) (proto.DiscoveryClient, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	if m.newDiscoveryClientFn != nil {
		client, err := m.newDiscoveryClientFn(peerID)

		return client, func() {}, err
	}

	return m.mockDiscoveryClient, func() {}, nil
}

/* FINISH ALPHA CHANGE 2704 -- adapt discovery mock to operation context and cleanup ownership -- */

func (m *MockNetworkingServer) HookNewDiscoveryClient(fn newDiscoveryClientDelegate) {
	m.newDiscoveryClientFn = fn
}

func (m *MockNetworkingServer) CloseProtocolStream(protocol string, peerID peer.ID) error {
	if m.closeProtocolStreamFn != nil {
		return m.closeProtocolStreamFn(protocol, peerID)
	}

	return nil
}

func (m *MockNetworkingServer) HookCloseProtocolStream(fn closeProtocolStreamDelegate) {
	m.closeProtocolStreamFn = fn
}

func (m *MockNetworkingServer) AddToPeerStore(peerInfo *peer.AddrInfo) {
	if m.addToPeerStoreFn != nil {
		m.addToPeerStoreFn(peerInfo)
	}
}

func (m *MockNetworkingServer) HookAddToPeerStore(fn addToPeerStoreDelegate) {
	m.addToPeerStoreFn = fn
}

func (m *MockNetworkingServer) RemoveFromPeerStore(peerInfo *peer.AddrInfo) {
	if m.removeFromPeerStoreFn != nil {
		m.removeFromPeerStoreFn(peerInfo)
	}
}

func (m *MockNetworkingServer) HookRemoveFromPeerStore(fn removeFromPeerStoreDelegate) {
	m.removeFromPeerStoreFn = fn
}

func (m *MockNetworkingServer) GetPeerInfo(peerID peer.ID) *peer.AddrInfo {
	if m.getPeerInfoFn != nil {
		return m.getPeerInfoFn(peerID)
	}

	return nil
}

func (m *MockNetworkingServer) HookGetPeerInfo(fn getPeerInfoDelegate) {
	m.getPeerInfoFn = fn
}

func (m *MockNetworkingServer) GetRandomPeer() *peer.ID {
	if m.getRandomPeerFn != nil {
		return m.getRandomPeerFn()
	}

	return nil
}

func (m *MockNetworkingServer) HookGetRandomPeer(fn getRandomPeerDelegate) {
	m.getRandomPeerFn = fn
}

func (m *MockNetworkingServer) FetchOrSetTemporaryDial(peerID peer.ID, newValue bool) bool {
	if m.fetchAndSetTemporaryDialFn != nil {
		return m.fetchAndSetTemporaryDialFn(peerID, newValue)
	}

	return false
}

func (m *MockNetworkingServer) HookFetchAndSetTemporaryDial(fn fetchAndSetTemporaryDialDelegate) {
	m.fetchAndSetTemporaryDialFn = fn
}

func (m *MockNetworkingServer) RemoveTemporaryDial(peerID peer.ID) {
	if m.removeTemporaryDialFn != nil {
		m.removeTemporaryDialFn(peerID)
	}
}

func (m *MockNetworkingServer) HookRemoveTemporaryDial(fn removeTemporaryDialDelegate) {
	m.removeTemporaryDialFn = fn
}

// MockIdentityClient mocks an identity client (other peer in the communication)
type MockIdentityClient struct {
	// Hooks that the test can set
	helloFn helloDelegate
}

type helloDelegate func(
	ctx context.Context,
	in *proto.Status,
	opts ...grpc.CallOption,
) (*proto.Status, error)

func (mic *MockIdentityClient) HookHello(fn helloDelegate) {
	mic.helloFn = fn
}

func (mic *MockIdentityClient) Hello(
	ctx context.Context,
	in *proto.Status,
	opts ...grpc.CallOption,
) (*proto.Status, error) {
	if mic.helloFn != nil {
		return mic.helloFn(ctx, in, opts...)
	}

	return nil, nil
}

// MockDiscoveryClient mocks a discovery client (other peer in the communication)
type MockDiscoveryClient struct {
	// Hooks that the test can set
	findPeersFn findPeersDelegate
}

type findPeersDelegate func(
	ctx context.Context,
	in *proto.FindPeersReq,
	opts ...grpc.CallOption,
) (*proto.FindPeersResp, error)

func (mdc *MockDiscoveryClient) HookFindPeers(fn findPeersDelegate) {
	mdc.findPeersFn = fn
}

func (mdc *MockDiscoveryClient) FindPeers(
	ctx context.Context,
	in *proto.FindPeersReq,
	opts ...grpc.CallOption,
) (*proto.FindPeersResp, error) {
	if mdc.findPeersFn != nil {
		return mdc.findPeersFn(ctx, in, opts...)
	}

	return nil, nil
}

// MockPeerMetrics is a mock used by the Kademlia routing table
type MockPeerMetrics struct {
	recordLatencyFn     recordLatencyDelegate
	latencyEWMAFn       latencyEWMADelegate
	removeMetricsPeerFn removeMetricsPeerDelegate
}

type recordLatencyDelegate func(id peer.ID, duration time.Duration)
type latencyEWMADelegate func(id peer.ID) time.Duration
type removeMetricsPeerDelegate func(id peer.ID)

func (m *MockPeerMetrics) RecordLatency(id peer.ID, duration time.Duration) {
	if m.recordLatencyFn != nil {
		m.recordLatencyFn(id, duration)
	}
}

func (m *MockPeerMetrics) HookRecordLatency(fn recordLatencyDelegate) {
	m.recordLatencyFn = fn
}

func (m *MockPeerMetrics) LatencyEWMA(id peer.ID) time.Duration {
	if m.latencyEWMAFn != nil {
		return m.latencyEWMAFn(id)
	}

	return 0
}

func (m *MockPeerMetrics) HookLatencyEWMA(fn latencyEWMADelegate) {
	m.latencyEWMAFn = fn
}

func (m *MockPeerMetrics) RemovePeer(id peer.ID) {
	if m.removeMetricsPeerFn != nil {
		m.removeMetricsPeerFn(id)
	}
}

func (m *MockPeerMetrics) HookRemoveMetricsPeer(fn removeMetricsPeerDelegate) {
	m.removeMetricsPeerFn = fn
}
