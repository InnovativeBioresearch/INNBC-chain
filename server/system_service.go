package server

import (
	/* START ALPHA CHANGE 3269 -- remove imports exclusive to legacy archive streaming -- */
	"context"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
	"github.com/libp2p/go-libp2p/core/peer"

	/* FINISH ALPHA CHANGE 3269 -- remove imports exclusive to legacy archive streaming -- */

	/* START ALPHA CHANGE 3000 -- preserve operator subscription cancellation status -- */
	"google.golang.org/grpc/status"
	/* FINISH ALPHA CHANGE 3000 -- preserve operator subscription cancellation status -- */
	empty "google.golang.org/protobuf/types/known/emptypb"
)

type systemService struct {
	proto.UnimplementedSystemServer

	server *Server
}

// GetStatus returns the current system status, in the form of:
//
// Network: <chainID>
//
// Current: { Number: <blockNumber>; Hash: <headerHash> }
//
// P2PAddr: <libp2pAddress>
func (s *systemService) GetStatus(ctx context.Context, req *empty.Empty) (*proto.ServerStatus, error) {
	header := s.server.blockchain.Header()

	addr, err := common.AddrInfoToString(s.server.network.AddrInfo())
	if err != nil {
		return nil, err
	}

	status := &proto.ServerStatus{
		Network: s.server.chain.Params.ChainID,
		Current: &proto.ServerStatus_Block{
			Number: int64(header.Number),
			Hash:   header.Hash.String(),
		},
		P2PAddr: addr,
	}

	return status, nil
}

/* START ALPHA CHANGE 3000 -- cancel idle operator subscriptions and release their single owner -- */
// Subscribe implements the blockchain event subscription service.
func (s *systemService) Subscribe(req *empty.Empty, stream proto.System_SubscribeServer) error {
	ctx := stream.Context()
	sub := s.server.blockchain.SubscribeEvents()
	defer s.server.blockchain.UnsubscribeEvents(sub)

	eventCh := sub.GetEventCh()

	for {
		if err := ctx.Err(); err != nil {
			return status.FromContextError(err).Err()
		}

		var (
			evnt *blockchain.Event
			ok   bool
		)

		select {
		case <-ctx.Done():
			return status.FromContextError(ctx.Err()).Err()
		case evnt, ok = <-eventCh:
		}

		// Cancellation can race with delivery of an already-buffered event.
		if err := ctx.Err(); err != nil {
			return status.FromContextError(err).Err()
		}

		if !ok || evnt == nil {
			return nil
		}

		pEvent := &proto.BlockchainEvent{
			Added:   []*proto.BlockchainEvent_Header{},
			Removed: []*proto.BlockchainEvent_Header{},
		}

		for _, h := range evnt.NewChain {
			pEvent.Added = append(
				pEvent.Added,
				&proto.BlockchainEvent_Header{Hash: h.Hash.String(), Number: int64(h.Number)},
			)
		}

		if err := ctx.Err(); err != nil {
			return status.FromContextError(err).Err()
		}

		/* START ALPHA CHANGE 819 -- keep removed event list empty after reorg pruning -- */
		if err := stream.Send(pEvent); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 819 -- keep removed event list empty after reorg pruning -- */
	}
}

/* FINISH ALPHA CHANGE 3000 -- cancel idle operator subscriptions and release their single owner -- */

// PeersAdd implements the 'peers add' operator service
func (s *systemService) PeersAdd(_ context.Context, req *proto.PeersAddRequest) (*proto.PeersAddResponse, error) {
	if joinErr := s.server.JoinPeer(req.Id); joinErr != nil {
		return &proto.PeersAddResponse{
			Message: "Unable to successfully add peer",
		}, joinErr
	}

	return &proto.PeersAddResponse{
		Message: "Peer address marked ready for dialing",
	}, nil
}

// PeersStatus implements the 'peers status' operator service
func (s *systemService) PeersStatus(ctx context.Context, req *proto.PeersStatusRequest) (*proto.Peer, error) {
	peerID, err := peer.Decode(req.Id)
	if err != nil {
		return nil, err
	}

	peer, err := s.getPeer(peerID)
	if err != nil {
		return nil, err
	}

	return peer, nil
}

// getPeer returns a specific proto.Peer using the peer ID
func (s *systemService) getPeer(id peer.ID) (*proto.Peer, error) {
	protocols, err := s.server.network.GetProtocols(id)
	if err != nil {
		return nil, err
	}

	info := s.server.network.GetPeerInfo(id)

	addrs := []string{}
	for _, addr := range info.Addrs {
		addrs = append(addrs, addr.String())
	}

	peer := &proto.Peer{
		Id:        id.String(),
		Protocols: protocols,
		Addrs:     addrs,
	}

	return peer, nil
}

// PeersList implements the 'peers list' operator service
func (s *systemService) PeersList(
	ctx context.Context,
	req *empty.Empty,
) (*proto.PeersListResponse, error) {
	resp := &proto.PeersListResponse{
		Peers: []*proto.Peer{},
	}

	peers := s.server.network.Peers()
	for _, p := range peers {
		peer, err := s.getPeer(p.Info.ID)
		if err != nil {
			return nil, err
		}

		resp.Peers = append(resp.Peers, peer)
	}

	return resp, nil
}

/* START ALPHA CHANGE 1221 -- implement manual peer directory refresh grpc handler -- */
func (s *systemService) RefreshPeerDirectory(ctx context.Context, req *empty.Empty) (*proto.RefreshPeerDirectoryResponse, error) {
	result, err := s.server.RefreshPeerDirectory()
	if err != nil {
		return nil, err
	}

	return &proto.RefreshPeerDirectoryResponse{
		Peers: result.Peers,
	}, nil
}

/* FINISH ALPHA CHANGE 1221 -- implement manual peer directory refresh grpc handler -- */

/* START ALPHA CHANGE 3269 -- remove legacy archive lookup and streaming implementation -- */
/* FINISH ALPHA CHANGE 3269 -- remove legacy archive lookup and streaming implementation -- */
