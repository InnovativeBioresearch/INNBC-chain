package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/innovativebioresearch/innbc-chain/network/discovery"
	"github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	kb "github.com/libp2p/go-libp2p-kbucket"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	rawGrpc "google.golang.org/grpc"
)

/* START ALPHA CHANGE 651 -- remove legacy bootnode selectors from network discovery server -- */
/* FINISH ALPHA CHANGE 651 -- remove legacy bootnode selectors from network discovery server -- */

/* START ALPHA CHANGE 2695 -- give each discovery request its own cancellable connection -- */
func (s *Server) NewDiscoveryClient(
	ctx context.Context,
	peerID peer.ID,
) (proto.DiscoveryClient, func(), error) {
	if !s.IsConnected(peerID) && !s.IsTemporaryDial(peerID) {
		return nil, nil, fmt.Errorf(
			"could not initialize new discovery client - peer [%s] not connected",
			peerID.String(),
		)
	}

	conn, err := s.NewProtoConnectionContext(ctx, common.DiscProto, peerID)
	if err != nil {
		return nil, nil, err
	}

	return proto.NewDiscoveryClient(conn), grpc.OwnClient(ctx, conn), nil
}

/* FINISH ALPHA CHANGE 2695 -- give each discovery request its own cancellable connection -- */

// SaveProtocolStream saves the protocol stream to the peer
// protocol stream reference [Thread safe]
/* START ALPHA CHANGE 2696 -- close replaced and abandoned saved clients outside peer locks -- */
func (s *Server) SaveProtocolStream(
	protocol string,
	stream *rawGrpc.ClientConn,
	peerID peer.ID,
) {
	if stream == nil {
		return
	}

	s.peersLock.Lock()

	select {
	case <-s.closeCh:
		s.peersLock.Unlock()
		_ = stream.Close()

		return
	default:
	}

	connectionInfo, ok := s.peers[peerID]
	if !ok {
		s.peersLock.Unlock()
		_ = stream.Close()

		return
	}

	previous := connectionInfo.addProtocolStream(protocol, stream)
	s.peersLock.Unlock()

	if previous != nil && previous != stream {
		_ = previous.Close()
	}
}

func (s *Server) CloseProtocolStream(protocol string, peerID peer.ID) error {
	s.peersLock.Lock()

	connectionInfo, ok := s.peers[peerID]
	if !ok {
		s.peersLock.Unlock()

		return nil
	}

	stream := connectionInfo.removeProtocolStream(protocol)
	s.peersLock.Unlock()

	if stream != nil {
		return stream.Close()
	}

	return nil
}

/* FINISH ALPHA CHANGE 2696 -- close replaced and abandoned saved clients outside peer locks -- */

// AddToPeerStore adds peer information to the node's peer store
func (s *Server) AddToPeerStore(peerInfo *peer.AddrInfo) {
	/* START ALPHA CHANGE 2830 -- reject missing peerstore addresses before dereferencing -- */
	if peerInfo == nil || len(peerInfo.Addrs) == 0 || peerInfo.Addrs[0] == nil {
		return
	}
	/* FINISH ALPHA CHANGE 2830 -- reject missing peerstore addresses before dereferencing -- */

	/* START ALPHA CHANGE 689 -- avoid storing non-allowed validator peers -- */
	if !s.IsPeerAllowed(peerInfo.ID) {
		return
	}
	/* FINISH ALPHA CHANGE 689 -- avoid storing non-allowed validator peers -- */

	s.host.Peerstore().AddAddr(peerInfo.ID, peerInfo.Addrs[0], peerstore.AddressTTL)
}

// RemoveFromPeerStore removes peer information from the node's peer store
func (s *Server) RemoveFromPeerStore(peerInfo *peer.AddrInfo) {
	s.host.Peerstore().RemovePeer(peerInfo.ID)
}

// GetPeerInfo fetches the information of a peer
func (s *Server) GetPeerInfo(peerID peer.ID) *peer.AddrInfo {
	info := s.host.Peerstore().PeerInfo(peerID)

	return &info
}

// GetRandomPeer fetches a random peer from the peers list
func (s *Server) GetRandomPeer() *peer.ID {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	if len(s.peers) < 1 {
		return nil
	}

	randNum, _ := rand.Int(
		rand.Reader,
		big.NewInt(int64(len(s.peers))),
	)

	randomPeerIndx := int(randNum.Int64())

	counter := 0
	for peerID := range s.peers {
		if randomPeerIndx == counter {
			return &peerID
		}

		counter++
	}

	return nil
}

// FetchOrSetTemporaryDial loads the temporary status of a peer connection, and
// sets a new value [Thread safe]
func (s *Server) FetchOrSetTemporaryDial(peerID peer.ID, newValue bool) bool {
	_, loaded := s.temporaryDials.LoadOrStore(peerID, newValue)

	return loaded
}

// RemoveTemporaryDial removes a peer connection as temporary [Thread safe]
func (s *Server) RemoveTemporaryDial(peerID peer.ID) {
	s.temporaryDials.Delete(peerID)
}

/* START ALPHA CHANGE 2854 -- separate discovery routing eviction from dial request ownership -- */
func (s *Server) newDiscoveryRoutingTable() (*kb.RoutingTable, error) {
	keyID := kb.ConvertPeerID(s.host.ID())

	routingTable, err := kb.NewRoutingTable(
		defaultBucketSize,
		keyID,
		time.Minute,
		s.host.Peerstore(),
		10*time.Second,
		nil,
	)
	if err != nil {
		return nil, err
	}

	routingTable.PeerAdded = func(p peer.ID) {
		/*
			PeerAdded runs under the routing-table lock. Queue insertion and
			event delivery remain outside that lock.
		*/
		go func() {
			info := s.host.Peerstore().PeerInfo(p)
			s.addToDialQueue(&info, common.PriorityRandomDial)
		}()
	}

	routingTable.PeerRemoved = func(peer.ID) {
		/*
			Routing-table eviction does not own independently queued dial
			requests. A delayed disconnect or failure must not discard a
			new reconnect request for the same identity.

			The dial loop retains its authorization, connectivity, capacity,
			and shutdown checks.
		*/
	}

	return routingTable, nil
}

/* FINISH ALPHA CHANGE 2854 -- separate discovery routing eviction from dial request ownership -- */

// setupDiscovery Sets up the discovery service for the node
func (s *Server) setupDiscovery() error {
	/* START ALPHA CHANGE 2855 -- construct discovery with the shared production routing callbacks -- */
	routingTable, err := s.newDiscoveryRoutingTable()
	if err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2855 -- construct discovery with the shared production routing callbacks -- */

	// Create an instance of the discovery service
	discoveryService := discovery.NewDiscoveryService(
		s,
		routingTable,
		s.logger,
	)

	// Register a network event handler
	if err := s.Subscribe(context.Background(), discoveryService.HandleNetworkEvent); err != nil {
		return fmt.Errorf("unable to subscribe to network events, %w", err)
	}

	// Register the actual discovery service as a valid protocol
	s.registerDiscoveryService(discoveryService)

	/* START ALPHA CHANGE 652 -- stop seeding discovery from legacy bootnodes -- */
	/* FINISH ALPHA CHANGE 652 -- stop seeding discovery from legacy bootnodes -- */

	// Start the discovery service
	discoveryService.Start()

	// Set the discovery service reference
	s.discovery = discoveryService

	return nil
}

func (s *Server) TemporaryDialPeer(peerAddrInfo *peer.AddrInfo) {
	/* START ALPHA CHANGE 690 -- avoid temporary dials to non-allowed validator peers -- */
	if !s.IsPeerAllowed(peerAddrInfo.ID) {
		return
	}
	/* FINISH ALPHA CHANGE 690 -- avoid temporary dials to non-allowed validator peers -- */

	s.logger.Debug("creating new temporary dial to peer", "peer", peerAddrInfo.ID)
	s.addToDialQueue(peerAddrInfo, common.PriorityRandomDial)
}

// registerDiscoveryService registers the discovery protocol to be available
func (s *Server) registerDiscoveryService(discovery *discovery.DiscoveryService) {
	grpcStream := grpc.NewGrpcStream()
	proto.RegisterDiscoveryServer(grpcStream.GrpcServer(), discovery)
	grpcStream.Serve()

	s.RegisterProtocol(common.DiscProto, grpcStream)
}
