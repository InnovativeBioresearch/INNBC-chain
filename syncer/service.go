package syncer

import (
	"context"
	"errors"

	/* START ALPHA CHANGE 2717 -- import service lifecycle synchronization -- */
	"sync"
	/* FINISH ALPHA CHANGE 2717 -- import service lifecycle synchronization -- */

	"github.com/armon/go-metrics"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/innovativebioresearch/innbc-chain/types"

	/* START ALPHA CHANGE 2903 -- import authenticated peer keys for serving accounting -- */
	"github.com/libp2p/go-libp2p/core/peer"
	/* FINISH ALPHA CHANGE 2903 -- import authenticated peer keys for serving accounting -- */)

/* START ALPHA CHANGE 1755 -- use block-not-found for unavailable segmented history -- */
var ErrBlockNotFound = errors.New("block not found")

/* FINISH ALPHA CHANGE 1755 -- use block-not-found for unavailable segmented history -- */

type syncPeerService struct {
	proto.UnimplementedSyncPeerServer

	blockchain Blockchain       // reference to the blockchain module
	network    Network          // reference to the network module
	stream     *grpc.GrpcStream // reference to the grpc stream

	/* START ALPHA CHANGE 2718 -- coordinate sync service startup and shutdown -- */
	lifecycleLock sync.Mutex
	closed        bool
	/* FINISH ALPHA CHANGE 2718 -- coordinate sync service startup and shutdown -- */

	/* START ALPHA CHANGE 2904 -- share serving accounting across all service connections -- */
	/* lifecycleLock protects these counters and orders admission against Close. */
	activeBlockServing int
	blockServingByPeer map[peer.ID]int
	/* FINISH ALPHA CHANGE 2904 -- share serving accounting across all service connections -- */
}

func NewSyncPeerService(
	network Network,
	blockchain Blockchain,
) SyncPeerService {
	return &syncPeerService{
		blockchain: blockchain,
		network:    network,
	}
}

/* START ALPHA CHANGE 2719 -- stop and join the owned sync RPC service outside its lifecycle lock -- */
func (s *syncPeerService) Start() {
	s.lifecycleLock.Lock()
	defer s.lifecycleLock.Unlock()

	if s.closed || s.stream != nil {
		return
	}

	s.setupGRPCServer()
}

func (s *syncPeerService) Close() error {
	s.lifecycleLock.Lock()
	s.closed = true
	stream := s.stream
	s.lifecycleLock.Unlock()

	if stream != nil {
		stream.Stop()
	}

	return nil
}

/* FINISH ALPHA CHANGE 2719 -- stop and join the owned sync RPC service outside its lifecycle lock -- */

// setupGRPCServer setup GRPC server
func (s *syncPeerService) setupGRPCServer() {
	s.stream = grpc.NewGrpcStream()

	proto.RegisterSyncPeerServer(s.stream.GrpcServer(), s)
	s.stream.Serve()
	s.network.RegisterProtocol(syncerProto, s.stream)
}

// GetBlocks is a gRPC endpoint to return blocks from the specific height via stream
func (s *syncPeerService) GetBlocks(
	req *proto.GetBlocksRequest,
	stream proto.SyncPeer_GetBlocksServer,
) error {
	/* START ALPHA CHANGE 2905 -- reserve serving capacity before any blockchain access -- */
	release, err := s.beginBlockServing(stream.Context())
	if err != nil {
		return err
	}
	defer release()
	/* FINISH ALPHA CHANGE 2905 -- reserve serving capacity before any blockchain access -- */

	/* START ALPHA CHANGE 1668 -- serve a fixed currently available block range -- */
	latestHeader := s.blockchain.Header()
	if latestHeader == nil {
		return ErrBlockNotFound
	}

	target := latestHeader.Number
	if req.From < s.blockchain.HistoryFrom() || req.From > target {
		return ErrBlockNotFound
	}

	for i := req.From; ; i++ {
		/* START ALPHA CHANGE 2720 -- stop cancelled download handlers before another storage read -- */
		if err := stream.Context().Err(); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 2720 -- stop cancelled download handlers before another storage read -- */

		block, ok := s.blockchain.GetBlockByNumber(i, true)
		if !ok {
			/* START ALPHA CHANGE 1756 -- reject unavailable complete block ranges uniformly -- */
			return ErrBlockNotFound
			/* FINISH ALPHA CHANGE 1756 -- reject unavailable complete block ranges uniformly -- */
		}

		/* START ALPHA CHANGE 2906 -- avoid serialization after cancellation during a storage read -- */
		if err := stream.Context().Err(); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 2906 -- avoid serialization after cancellation during a storage read -- */

		resp := toProtoBlock(block)
		metrics.SetGauge([]string{syncerMetrics, "egress_bytes"}, float32(len(resp.Block)))

		// if client closes stream, context.Canceled is given
		if err := stream.Send(resp); err != nil {
			return err
		}

		if i == target {
			return nil
		}
	}
	/* FINISH ALPHA CHANGE 1668 -- serve a fixed currently available block range -- */
}

// GetStatus is a gRPC endpoint to return the latest block number as a node status
func (s *syncPeerService) GetStatus(
	ctx context.Context,
	req *empty.Empty,
) (*proto.SyncPeerStatus, error) {
	var number uint64
	if header := s.blockchain.Header(); header != nil {
		number = header.Number
	}

	return &proto.SyncPeerStatus{
		Number: number,
		/* START ALPHA CHANGE 1669 -- return current history availability in direct status -- */
		HistoryFrom: s.blockchain.HistoryFrom(),
		/* FINISH ALPHA CHANGE 1669 -- return current history availability in direct status -- */
	}, nil
}

// toProtoBlock converts type.Block -> proto.Block
func toProtoBlock(block *types.Block) *proto.Block {
	return &proto.Block{
		Block: block.MarshalRLP(),
	}
}
