package discovery

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	kb "github.com/libp2p/go-libp2p-kbucket"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// maxDiscoveryPeerReqCount is the max peer count that
	// can be requested from other peers
	maxDiscoveryPeerReqCount = 16

	// peerDiscoveryInterval is the interval at which other
	// peers are queried for their peer sets
	peerDiscoveryInterval = 5 * time.Second

	/* START ALPHA CHANGE 656 -- remove legacy bootnode discovery interval -- */
	/* FINISH ALPHA CHANGE 656 -- remove legacy bootnode discovery interval -- */
)

// networkingServer defines the base communication interface between
// any networking server implementation and the DiscoveryService
type networkingServer interface {
	/* START ALPHA CHANGE 657 -- remove legacy bootnode discovery server hooks -- */
	/* FINISH ALPHA CHANGE 657 -- remove legacy bootnode discovery server hooks -- */

	// PROTOCOL MANIPULATION //

	/* START ALPHA CHANGE 2697 -- require operation-owned discovery connections -- */
	NewDiscoveryClient(
		context.Context,
		peer.ID,
	) (proto.DiscoveryClient, func(), error)
	/* FINISH ALPHA CHANGE 2697 -- require operation-owned discovery connections -- */

	// PEER MANIPULATION //

	// DisconnectFromPeer attempts to disconnect from the specified peer
	DisconnectFromPeer(peerID peer.ID, reason string)

	// AddToPeerStore adds a peer to the networking server's peer store
	AddToPeerStore(peerInfo *peer.AddrInfo)

	// RemoveFromPeerStore removes peer information from the server's peer store
	RemoveFromPeerStore(peerInfo *peer.AddrInfo)

	// GetPeerInfo fetches the peer information from the server's peer store
	GetPeerInfo(peerID peer.ID) *peer.AddrInfo

	// GetRandomPeer fetches a random peer from the server's peer store
	GetRandomPeer() *peer.ID

	// TEMPORARY DIALING //

	// FetchOrSetTemporaryDial checks if the peer connection is a temporary dial,
	// and sets a new value accordingly
	FetchOrSetTemporaryDial(peerID peer.ID, newValue bool) bool

	// RemoveTemporaryDial removes a peer from the temporary dial map
	RemoveTemporaryDial(peerID peer.ID)

	// TemporaryDialPeer dials the peer temporarily
	TemporaryDialPeer(peerAddrInfo *peer.AddrInfo)

	// CONNECTION INFORMATION //

	// HasFreeConnectionSlot checks if there is an available connection slot for the set direction [Thread safe]
	HasFreeConnectionSlot(direction network.Direction) bool

	/* START ALPHA CHANGE 685 -- expose validator peer allowlist to discovery service -- */
	IsPeerAllowed(peerID peer.ID) bool
	/* FINISH ALPHA CHANGE 685 -- expose validator peer allowlist to discovery service -- */
}

// DiscoveryService is a service that finds other peers in the network
// and connects them to the current running node
type DiscoveryService struct {
	proto.UnimplementedDiscoveryServer

	baseServer   networkingServer // The interface towards the base networking server
	logger       hclog.Logger     // The DiscoveryService logger
	routingTable *kb.RoutingTable // Kademlia 'k-bucket' routing table that contains connected nodes info

	closeCh chan struct{} // Channel used for stopping the DiscoveryService

	/* START ALPHA CHANGE 2698 -- track discovery operations and workers -- */
	rpcLifetime grpc.RPCLifetime
	/* FINISH ALPHA CHANGE 2698 -- track discovery operations and workers -- */

	/* START ALPHA CHANGE 1136 -- make discovery service close idempotent -- */
	closeOnce sync.Once
	/* FINISH ALPHA CHANGE 1136 -- make discovery service close idempotent -- */
}

// NewDiscoveryService creates a new instance of the discovery service
func NewDiscoveryService(
	server networkingServer,
	routingTable *kb.RoutingTable,
	logger hclog.Logger,
) *DiscoveryService {
	return &DiscoveryService{
		logger:       logger.Named("discovery"),
		baseServer:   server,
		routingTable: routingTable,
		closeCh:      make(chan struct{}),
	}
}

/* START ALPHA CHANGE 2699 -- admit discovery workers and join them after cancellation -- */
func (d *DiscoveryService) Start() {
	_, finish, err := d.rpcLifetime.Begin(context.Background())
	if err != nil {
		return
	}

	go func() {
		defer finish()

		d.startDiscovery()
	}()
}

func (d *DiscoveryService) Close() {
	d.closeOnce.Do(func() {
		close(d.closeCh)
		d.rpcLifetime.Close()
	})
}

/* FINISH ALPHA CHANGE 2699 -- admit discovery workers and join them after cancellation -- */

// RoutingTableSize returns the size of the routing table
func (d *DiscoveryService) RoutingTableSize() int {
	return d.routingTable.Size()
}

// RoutingTablePeers fetches the peers from the routing table
func (d *DiscoveryService) RoutingTablePeers() []peer.ID {
	return d.routingTable.ListPeers()
}

// HandleNetworkEvent handles base network events for the DiscoveryService
func (d *DiscoveryService) HandleNetworkEvent(peerEvent *event.PeerEvent) {
	/* START ALPHA CHANGE 2751 -- preserve replacement peers when old cleanup arrives late -- */
	if !peerEvent.IsCurrent() {
		return
	}
	/* FINISH ALPHA CHANGE 2751 -- preserve replacement peers when old cleanup arrives late -- */

	peerID := peerEvent.PeerID

	switch peerEvent.Type {
	case event.PeerConnected:
		// Add peer to the routing table and to our local peer table
		_, err := d.routingTable.TryAddPeer(peerID, false, false)
		if err != nil {
			d.logger.Error("failed to add peer to routing table", "err", err)

			return
		}
	case event.PeerDisconnected, event.PeerFailedToConnect:
		// Run cleanup for the local routing / reference peers table
		d.routingTable.RemovePeer(peerID)
	}
}

/* START ALPHA CHANGE 665 -- remove legacy bootnode table seeding helper -- */
/* FINISH ALPHA CHANGE 665 -- remove legacy bootnode table seeding helper -- */

// addToTable adds the node to the peer store and the routing table
func (d *DiscoveryService) addToTable(node *peer.AddrInfo) error {
	// before we include peers on the routing table -> dial queue
	// we have to add them to the peer store so that they are
	// available to all the libp2p services
	d.baseServer.AddToPeerStore(node)

	if _, err := d.routingTable.TryAddPeer(
		node.ID,
		false,
		false,
	); err != nil {
		// Since the routing table addition failed,
		// the peer can be removed from the libp2p peer store
		// in the base networking server
		d.baseServer.RemoveFromPeerStore(node)

		return err
	}

	return nil
}

// addPeersToTable adds the passed in peers to the peer store and the routing table
func (d *DiscoveryService) addPeersToTable(nodeAddrStrs []string) {
	for _, nodeAddrStr := range nodeAddrStrs {
		// Convert the string address info to a working type
		nodeInfo, err := common.StringToAddrInfo(nodeAddrStr)
		if err != nil {
			d.logger.Error(
				"Failed to parse address",
				"err",
				err,
			)

			continue
		}

		/* START ALPHA CHANGE 2834 -- discard addressless discovery records before table insertion -- */
		if len(nodeInfo.Addrs) == 0 {
			continue
		}
		/* FINISH ALPHA CHANGE 2834 -- discard addressless discovery records before table insertion -- */

		/* START ALPHA CHANGE 686 -- ignore discovered peers outside validator allowlist -- */
		if !d.baseServer.IsPeerAllowed(nodeInfo.ID) {
			continue
		}
		/* FINISH ALPHA CHANGE 686 -- ignore discovered peers outside validator allowlist -- */

		if err := d.addToTable(nodeInfo); err != nil {
			d.logger.Error(
				"Failed to add new peer to routing table",
				"peer",
				nodeInfo.ID,
				"err",
				err,
			)
		}
	}
}

// attemptToFindPeers dials the specified peer and requests
// to see their peer list
func (d *DiscoveryService) attemptToFindPeers(peerID peer.ID) error {
	d.logger.Debug("Querying a peer for near peers", "peer", peerID)
	/* START ALPHA CHANGE 2700 -- use discovery request ownership consistently -- */
	nodes, err := d.findPeersCall(peerID)
	/* FINISH ALPHA CHANGE 2700 -- use discovery request ownership consistently -- */

	if err != nil {
		return err
	}

	d.logger.Debug("Found new near peers", "peer", len(nodes))
	d.addPeersToTable(nodes)

	return nil
}

/* START ALPHA CHANGE 2701 -- bound discovery dialing and RPC before either can block -- */
func (d *DiscoveryService) findPeersCall(peerID peer.ID) ([]string, error) {
	ctx, finish, err := d.rpcLifetime.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	defer finish()

	ctx, cancel := context.WithTimeout(ctx, grpc.DefaultProtocolTimeout)
	defer cancel()

	clt, closeClient, err := d.baseServer.NewDiscoveryClient(ctx, peerID)
	if err != nil {
		return nil, fmt.Errorf("unable to create new discovery client connection, %w", err)
	}
	defer closeClient()

	resp, err := clt.FindPeers(ctx, &proto.FindPeersReq{
		Count: maxDiscoveryPeerReqCount,
	})
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if resp == nil {
		return nil, errors.New("discovery returned a nil response")
	}

	return resp.Nodes, nil
}

/* FINISH ALPHA CHANGE 2701 -- bound discovery dialing and RPC before either can block -- */

// startDiscovery starts the DiscoveryService loop,
// in which random peers are dialed for their peer sets.
func (d *DiscoveryService) startDiscovery() {
	peerDiscoveryTicker := time.NewTicker(peerDiscoveryInterval)

	defer func() {
		peerDiscoveryTicker.Stop()
	}()

	for {
		select {
		case <-d.closeCh:
			return
		case <-peerDiscoveryTicker.C:
			/* START ALPHA CHANGE 2702 -- register periodic discovery work before launching it -- */
			_, finish, err := d.rpcLifetime.Begin(context.Background())
			if err != nil {
				return
			}

			go func() {
				defer finish()

				d.regularPeerDiscovery()
			}()
			/* FINISH ALPHA CHANGE 2702 -- register periodic discovery work before launching it -- */
		}
	}
}

// regularPeerDiscovery grabs a random peer from the list of
// connected peers, and attempts to find / connect to their peer set
func (d *DiscoveryService) regularPeerDiscovery() {
	if !d.baseServer.HasFreeConnectionSlot(network.DirOutbound) {
		// No need to do peer discovery if no open connection slots
		// are available
		return
	}

	// Grab a random peer from the base server's peer store
	peerID := d.baseServer.GetRandomPeer()
	if peerID == nil {
		// The node cannot find a random peer to query
		// from the current peer set
		return
	}

	d.logger.Debug("running regular peer discovery", "peer", peerID.String())
	// Try to discover the peers connected to the reference peer
	if err := d.attemptToFindPeers(*peerID); err != nil {
		d.logger.Error(
			"Failed to find new peers",
			"peer",
			peerID,
			"err",
			err,
		)
	}
}

// bootnodePeerDiscovery queries a random (unconnected) bootnode for new peers
// and adds them to the routing table

/* START ALPHA CHANGE 658 -- remove legacy bootnode peer discovery loop -- */
/* FINISH ALPHA CHANGE 658 -- remove legacy bootnode peer discovery loop -- */

// FindPeers implements the proto service for finding the target's peers
func (d *DiscoveryService) FindPeers(
	ctx context.Context,
	req *proto.FindPeersReq,
) (*proto.FindPeersResp, error) {
	// Extract the requesting peer ID from the gRPC context
	grpcContext, ok := ctx.(*grpc.Context)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	from := grpcContext.PeerID

	/* START ALPHA CHANGE 687 -- reject peer discovery requests from non-allowed validators -- */
	if !d.baseServer.IsPeerAllowed(from) {
		return nil, errors.New("peer is not allowed")
	}
	/* FINISH ALPHA CHANGE 687 -- reject peer discovery requests from non-allowed validators -- */

	// Sanity check for result set size
	if req.Count > maxDiscoveryPeerReqCount {
		req.Count = maxDiscoveryPeerReqCount
	}

	// The request Key is used for finding the closest peers
	// by utilizing Kademlia's distance calculation.
	// This way, the peer that's being queried for its peers delivers
	// only the closest ones to the requested key (peer)
	if req.GetKey() == "" {
		req.Key = from.String()
	}

	nearestPeers := d.routingTable.NearestPeers(
		kb.ConvertKey(req.GetKey()),
		int(req.Count),
	)

	// The peer that's initializing this request
	// doesn't need to be a part of the resulting set
	filteredPeers := make([]string, 0)

	for _, id := range nearestPeers {
		if id == from {
			// Skip the peer that's initializing the request
			continue
		}

		/* START ALPHA CHANGE 688 -- do not leak non-allowed peers through discovery -- */
		if !d.baseServer.IsPeerAllowed(id) {
			continue
		}
		/* FINISH ALPHA CHANGE 688 -- do not leak non-allowed peers through discovery -- */

		if info := d.baseServer.GetPeerInfo(id); len(info.Addrs) > 0 {
			addr, err := common.AddrInfoToString(info)
			if err != nil {
				return nil, err
			}

			filteredPeers = append(filteredPeers, addr)
		}
	}

	return &proto.FindPeersResp{
		Nodes: filteredPeers,
	}, nil
}
