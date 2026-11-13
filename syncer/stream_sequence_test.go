/* START ALPHA CHANGE 3835 -- regress sync stream progression and recovery from non-progressing peers -- */

package syncer

import (
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func syncSequenceBlocks(numbers []uint64) []*types.Block {
	blocks := make([]*types.Block, len(numbers))
	for i, number := range numbers {
		blocks[i] = &types.Block{
			Header: &types.Header{Number: number},
		}
	}

	return blocks
}

func TestSyncStreamSequence_RejectsMalformed(t *testing.T) {
	const maximum = ^uint64(0)

	tests := []struct {
		name    string
		initial uint64
		numbers []uint64
	}{
		{
			name:    "genesis_instead_of_first_block",
			initial: 0,
			numbers: []uint64{0},
		},
		{
			name:    "genesis_after_nonzero_head",
			initial: 100,
			numbers: []uint64{0},
		},
		{
			name:    "stale_first_block",
			initial: 100,
			numbers: []uint64{100},
		},
		{
			name:    "skipped_first_block",
			initial: 100,
			numbers: []uint64{102},
		},
		{
			name:    "duplicate_after_valid_prefix",
			initial: 100,
			numbers: []uint64{101, 101},
		},
		{
			name:    "backward_after_valid_prefix",
			initial: 100,
			numbers: []uint64{101, 100},
		},
		{
			name:    "gap_after_valid_prefix",
			initial: 100,
			numbers: []uint64{101, 103},
		},
		{
			name:    "genesis_after_valid_prefix",
			initial: 100,
			numbers: []uint64{101, 0},
		},
		{
			name:    "zero_after_maximum_height",
			initial: maximum - 1,
			numbers: []uint64{maximum, 0},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			head := test.initial
			var requested uint64
			verified := make([]uint64, 0)
			written := make([]uint64, 0)
			callbacks := make([]uint64, 0)

			backend := &mockBlockchain{
				headerHandler: func() *types.Header {
					return &types.Header{Number: head}
				},
				verifyFinalizedBlockHandler: func(block *types.Block) (*types.FullBlock, error) {
					verified = append(verified, block.Number())

					return &types.FullBlock{Block: block}, nil
				},
				writeFullBlockHandler: func(block *types.FullBlock) error {
					written = append(written, block.Block.Number())
					head = block.Block.Number()

					return nil
				},
			}

			client := &mockSyncPeerClient{
				getBlocksHandler: func(
					_ peer.ID,
					start uint64,
					_ time.Duration,
				) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
					requested = start

					// The malformed frame is deliberately last. Once it is
					// received, this finite producer has no further block sends.
					blocks, terminal := blocksToStream(
						syncSequenceBlocks(test.numbers),
						0,
					)

					return blocks, terminal, nil
				},
			}

			s := NewTestSyncer(nil, backend, time.Second, client, &mockProgression{})
			t.Cleanup(func() {
				require.NoError(t, s.Close())
			})

			last, stop, err := s.bulkSyncWithPeer(
				"sequence-peer",
				maximum,
				func(block *types.FullBlock) bool {
					callbacks = append(callbacks, block.Block.Number())

					return false
				},
			)

			require.ErrorContains(t, err, "non-contiguous block stream")
			require.False(t, stop)
			require.Equal(t, test.initial+1, requested)

			prefix := test.numbers[:len(test.numbers)-1]
			expectedLast := test.initial
			if len(prefix) != 0 {
				expectedLast = prefix[len(prefix)-1]
			}

			require.Equal(t, expectedLast, last)
			require.Equal(t, expectedLast, head)
			require.Equal(t, prefix, verified)
			require.Equal(t, prefix, written)
			require.Equal(t, prefix, callbacks)
		})
	}
}

func TestSyncStreamSequence_PreservesValidDownloads(t *testing.T) {
	const maximum = ^uint64(0)

	tests := []struct {
		name       string
		initial    uint64
		advertised uint64
		advanceTo  uint64
		numbers    []uint64
	}{
		{
			name:       "starts_after_genesis",
			initial:    0,
			advertised: 3,
			numbers:    []uint64{1, 2, 3},
		},
		{
			name:       "starts_after_existing_head",
			initial:    100,
			advertised: 103,
			numbers:    []uint64{101, 102, 103},
		},
		{
			name:       "extends_beyond_earlier_advertised_head",
			initial:    100,
			advertised: 101,
			numbers:    []uint64{101, 102, 103},
		},
		{
			name:       "local_import_advances_head_after_request",
			initial:    100,
			advertised: 103,
			advanceTo:  103,
			numbers:    []uint64{101, 102, 103},
		},
		{
			name:       "maximum_height",
			initial:    maximum - 1,
			advertised: maximum,
			numbers:    []uint64{maximum},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			head := test.initial
			var requested uint64
			verified := make([]uint64, 0)
			written := make([]uint64, 0)
			callbacks := make([]uint64, 0)

			backend := &mockBlockchain{
				headerHandler: func() *types.Header {
					return &types.Header{Number: head}
				},
				verifyFinalizedBlockHandler: func(block *types.Block) (*types.FullBlock, error) {
					verified = append(verified, block.Number())

					return &types.FullBlock{Block: block}, nil
				},
				writeFullBlockHandler: func(block *types.FullBlock) error {
					written = append(written, block.Block.Number())

					// Model successful handling of a height already imported
					// through the concurrent consensus path.
					if block.Block.Number() > head {
						head = block.Block.Number()
					}

					return nil
				},
			}

			client := &mockSyncPeerClient{
				getBlocksHandler: func(
					_ peer.ID,
					start uint64,
					_ time.Duration,
				) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
					requested = start

					// The request has already captured its starting height.
					// Model consensus importing these heights before delivery.
					if test.advanceTo != 0 {
						head = test.advanceTo
					}

					blocks, terminal := blocksToStream(
						syncSequenceBlocks(test.numbers),
						0,
					)

					return blocks, terminal, nil
				},
			}

			s := NewTestSyncer(nil, backend, time.Second, client, &mockProgression{})
			t.Cleanup(func() {
				require.NoError(t, s.Close())
			})

			last, stop, err := s.bulkSyncWithPeer(
				"sequence-peer",
				test.advertised,
				func(block *types.FullBlock) bool {
					callbacks = append(callbacks, block.Block.Number())

					return false
				},
			)

			require.NoError(t, err)
			require.False(t, stop)
			require.Equal(t, test.initial+1, requested)
			require.Equal(t, test.numbers[len(test.numbers)-1], last)
			require.Equal(t, last, head)
			require.Equal(t, test.numbers, verified)
			require.Equal(t, test.numbers, written)
			require.Equal(t, test.numbers, callbacks)
		})
	}
}

func TestSyncStreamSequence_OpenMalformedRPCFallsBack(t *testing.T) {
	badRequests := make(chan uint64, 1)
	healthyRequests := make(chan uint64, 1)
	badExited := make(chan struct{})

	badService := &rpcOwnershipSyncService{
		blocks: func(
			req *proto.GetBlocksRequest,
			stream proto.SyncPeer_GetBlocksServer,
		) error {
			defer close(badExited)

			select {
			case badRequests <- req.From:
			case <-stream.Context().Done():
				return stream.Context().Err()
			}

			genesis := &types.Block{Header: &types.Header{Number: 0}}
			if err := stream.Send(toProtoBlock(genesis)); err != nil {
				return err
			}

			// Withhold EOF. Recovery must cancel this stream rather than
			// depend on normal completion or the operational block timeout.
			<-stream.Context().Done()

			return stream.Context().Err()
		},
	}

	healthyService := &rpcOwnershipSyncService{
		blocks: func(
			req *proto.GetBlocksRequest,
			stream proto.SyncPeer_GetBlocksServer,
		) error {
			select {
			case healthyRequests <- req.From:
			case <-stream.Context().Done():
				return stream.Context().Err()
			}

			for number := req.From; number <= 2; number++ {
				block := &types.Block{Header: &types.Header{Number: number}}
				if err := stream.Send(toProtoBlock(block)); err != nil {
					return err
				}
			}

			return nil
		},
	}

	networkStub := &blockServingRouteNetwork{
		routes: map[peer.ID]*blockServingTransport{
			"bad":     newBlockServingTransport(t, badService),
			"healthy": newBlockServingTransport(t, healthyService),
		},
	}

	var head atomic.Uint64
	verified := make([]uint64, 0)
	written := make([]uint64, 0)
	callbacks := make([]uint64, 0)

	backend := &mockBlockchain{
		headerHandler: func() *types.Header {
			return &types.Header{Number: head.Load()}
		},
		verifyFinalizedBlockHandler: func(block *types.Block) (*types.FullBlock, error) {
			verified = append(verified, block.Number())

			return &types.FullBlock{Block: block}, nil
		},
		writeFullBlockHandler: func(block *types.FullBlock) error {
			written = append(written, block.Block.Number())
			head.Store(block.Block.Number())

			return nil
		},
	}

	// A one-hour operational timeout makes the short test watchdog
	// distinguish immediate rejection from ordinary timeout recovery.
	s := NewTestSyncer(
		nil,
		backend,
		time.Hour,
		&mockSyncPeerClient{},
		&mockProgression{},
	)
	s.syncPeerClient = NewSyncPeerClient(
		hclog.NewNullLogger(),
		networkStub,
		nil,
	)

	// The malformed peer is deterministically preferred initially.
	s.peerMap.Put(
		&NoForkPeer{
			ID:       "bad",
			Number:   3,
			Distance: big.NewInt(0),
		},
		&NoForkPeer{
			ID:       "healthy",
			Number:   2,
			Distance: big.NewInt(1),
		},
	)

	result := make(chan error, 1)
	done := make(chan struct{})

	t.Cleanup(func() {
		// Cancel active RPCs before joining the Sync caller. This also
		// permits cleanup when the test runs against the unfixed code.
		closed := make(chan error, 1)
		go func() {
			closed <- s.Close()
		}()

		require.NoError(t, blockServingAwait(
			t,
			closed,
			"sync sequence fixture did not close",
		))
		blockServingAwait(t, done, "sync sequence caller did not exit")
	})

	go func() {
		defer close(done)

		result <- s.Sync(func(block *types.FullBlock) bool {
			callbacks = append(callbacks, block.Block.Number())

			return block.Block.Number() == 2
		})
	}()

	require.NoError(t, blockServingAwait(
		t,
		result,
		"sync did not reject the open malformed stream and use the healthy peer",
	))
	blockServingAwait(t, done, "successful sync caller did not exit")
	blockServingAwait(t, badExited, "malformed RPC handler was not cancelled")

	require.Equal(t, uint64(1), blockServingAwait(
		t,
		badRequests,
		"malformed peer did not receive the initial request",
	))
	require.Equal(t, uint64(1), blockServingAwait(
		t,
		healthyRequests,
		"healthy peer did not receive the fallback request",
	))

	require.Equal(t, uint64(2), head.Load())
	require.Equal(t, []uint64{1, 2}, verified)
	require.Equal(t, []uint64{1, 2}, written)
	require.Equal(t, []uint64{1, 2}, callbacks)
}

/* FINISH ALPHA CHANGE 3835 -- regress sync stream progression and recovery from non-progressing peers -- */
