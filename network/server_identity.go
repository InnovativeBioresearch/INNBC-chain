package network

import (
	/* START ALPHA CHANGE 2686 -- import identity operation context -- */
	"context"
	/* FINISH ALPHA CHANGE 2686 -- import identity operation context -- */
	"math/big"

	"github.com/armon/go-metrics"
	"github.com/innovativebioresearch/innbc-chain/network/common"
	peerEvent "github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/innovativebioresearch/innbc-chain/network/identity"
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	kbucket "github.com/libp2p/go-libp2p-kbucket"
	"github.com/libp2p/go-libp2p-kbucket/keyspace"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	rawGrpc "google.golang.org/grpc"
)

/* START ALPHA CHANGE 2687 -- return explicit identity connection ownership -- */
func (s *Server) NewIdentityClient(
	ctx context.Context,
	peerID peer.ID,
) (proto.IdentityClient, func(), error) {
	conn, err := s.NewProtoConnectionContext(ctx, common.IdentityProto, peerID)
	if err != nil {
		return nil, nil, err
	}

	return proto.NewIdentityClient(conn), grpc.OwnClient(ctx, conn), nil
}

/* FINISH ALPHA CHANGE 2687 -- return explicit identity connection ownership -- */

// AddPeer adds a new peer to the networking server's peer list,
// and updates relevant counters and metrics
func (s *Server) AddPeer(id peer.ID, direction network.Direction) {
	s.logger.Info("Peer connected", "id", id.String())

	// Update the peer connection info
	if connectionExists := s.addPeerInfo(id, direction); connectionExists {
		// The peer connection information was already present in the networking
		// server, so no connection metrics should be updated further
		return
	}

	// Emit the event alerting listeners
	// WARNING: THIS CALL IS POTENTIALLY BLOCKING
	s.emitEvent(id, peerEvent.PeerConnected)
}

// addPeerInfo updates the networking server's internal peer info table
// and returns a flag indicating if the same peer connection previously existed.
// In case the peer connection previously existed, this is a noop
func (s *Server) addPeerInfo(id peer.ID, direction network.Direction) bool {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	connectionInfo, connectionExists := s.peers[id]
	if connectionExists && connectionInfo.connDirections[direction] {
		// Check if this peer already has an active connection status (saved info).
		// There is no need to do further processing
		return true
	}

	// Check if the connection info is already initialized
	if !connectionExists {
		// Create a new record for the connection info
		connectionInfo = &PeerConnInfo{
			Info:            s.host.Peerstore().PeerInfo(id),
			connDirections:  make(map[network.Direction]bool),
			protocolStreams: make(map[string]*rawGrpc.ClientConn),
		}
	}

	// Save the connection info to the networking server
	connectionInfo.connDirections[direction] = true

	s.peers[id] = connectionInfo

	// Update connection counters
	s.connectionCounts.UpdateConnCountByDirection(1, direction)
	s.updateConnCountMetrics(direction)
	/* START ALPHA CHANGE 654 -- stop tracking legacy bootnode connection count on peer add -- */
	/* FINISH ALPHA CHANGE 654 -- stop tracking legacy bootnode connection count on peer add -- */

	// Update the metric stats
	metrics.SetGauge([]string{networkMetrics, "peers"}, float32(len(s.peers)))

	return false
}

// UpdatePendingConnCount updates the pending connection count in the specified direction [Thread safe]
func (s *Server) UpdatePendingConnCount(delta int64, direction network.Direction) {
	s.connectionCounts.UpdatePendingConnCountByDirection(delta, direction)

	s.updatePendingConnCountMetrics(direction)
}

// EmitEvent emits a specified event to the networking server's event bus
func (s *Server) EmitEvent(event *peerEvent.PeerEvent) {
	s.emitEvent(event.PeerID, event.Type)
}

// IsTemporaryDial checks if a peer connection is temporary [Thread safe]
func (s *Server) IsTemporaryDial(peerID peer.ID) bool {
	_, ok := s.temporaryDials.Load(peerID)

	return ok
}

// setupIdentity sets up the identity service for the node
func (s *Server) setupIdentity() error {
	// Create an instance of the identity service
	identityService := identity.NewIdentityService(
		s,
		s.logger,
		s.config.Chain.Params.ChainID,
		s.host.ID(),
	)

	/* START ALPHA CHANGE 2688 -- retain the identity service for joined shutdown -- */
	s.identityService = identityService
	/* FINISH ALPHA CHANGE 2688 -- retain the identity service for joined shutdown -- */

	// Register the identity service protocol
	s.registerIdentityService(identityService)

	// Register the network notify bundle handlers
	s.host.Network().Notify(identityService.GetNotifyBundle())

	return nil
}

// registerIdentityService registers the identity service
func (s *Server) registerIdentityService(identityService *identity.IdentityService) {
	grpcStream := grpc.NewGrpcStream()
	proto.RegisterIdentityServer(grpcStream.GrpcServer(), identityService)
	grpcStream.Serve()

	s.RegisterProtocol(common.IdentityProto, grpcStream)
}

func (s *Server) GetPeerDistance(peerID peer.ID) *big.Int {
	nodeKey := keyspace.Key{Space: keyspace.XORKeySpace, Bytes: kbucket.ConvertPeerID(s.AddrInfo().ID)}
	peerKey := keyspace.Key{Space: keyspace.XORKeySpace, Bytes: kbucket.ConvertPeerID(peerID)}

	return nodeKey.Distance(peerKey)
}
