/* START ALPHA CHANGE 3857 -- regress outer sync callback termination and preserve partial-download fallback -- */

package syncer

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type syncCallbackStopRequest struct {
	peerID peer.ID
	from   uint64
}

type syncCallbackStopClient struct {
	*mockSyncPeerClient
	closed []peer.ID
}

func (c *syncCallbackStopClient) CloseStream(id peer.ID) error {
	c.closed = append(c.closed, id)

	return nil
}

type syncCallbackStopProgression struct {
	*mockProgression
	stops int
}

func (p *syncCallbackStopProgression) StopProgression() {
	p.stops++
}

// These fixtures use small, finite ranges and fully buffered block channels.
// No producer goroutine can remain blocked when a callback stops consumption.
// Terminal delivery is controlled separately so it cannot race the blocks.
func syncCallbackStopBlocks(
	first uint64,
	last uint64,
) (<-chan *types.Block, chan BlockStreamTerminal) {
	blocks := make(chan *types.Block, int(last-first+1))

	for number := first; number <= last; number++ {
		blocks <- &types.Block{
			Header: &types.Header{Number: number},
		}
	}
	close(blocks)

	return blocks, make(chan BlockStreamTerminal, 1)
}

func TestSyncCallbackStop_TerminationTakesPriority(t *testing.T) {
	tests := []struct {
		name       string
		initial    uint64
		advertised uint64
		stopAt     uint64
	}{
		{
			name:       "below_head_after_genesis",
			initial:    0,
			advertised: 10,
			stopAt:     1,
		},
		{
			name:       "below_head_after_existing_history",
			initial:    100,
			advertised: 110,
			stopAt:     101,
		},
		{
			name:       "below_head_after_valid_prefix",
			initial:    100,
			advertised: 110,
			stopAt:     103,
		},
		{
			name:       "at_advertised_head",
			initial:    100,
			advertised: 103,
			stopAt:     103,
		},
		{
			name:       "beyond_earlier_advertised_head",
			initial:    100,
			advertised: 102,
			stopAt:     103,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			head := test.initial
			verified := make([]uint64, 0)
			written := make([]uint64, 0)
			callbacks := make([]uint64, 0)
			requests := make([]syncCallbackStopRequest, 0)

			backend := &mockBlockchain{
				headerHandler: func() *types.Header {
					return &types.Header{Number: head}
				},
				verifyFinalizedBlockHandler: func(block *types.Block) (*types.FullBlock, error) {
					verified = append(verified, block.Number())

					return &types.FullBlock{Block: block}, nil
				},
				writeFullBlockHandler: func(block *types.FullBlock) error {
					number := block.Block.Number()
					written = append(written, number)
					head = number

					return nil
				},
			}

			client := &syncCallbackStopClient{
				mockSyncPeerClient: &mockSyncPeerClient{
					getBlocksHandler: func(
						id peer.ID,
						from uint64,
						_ time.Duration,
					) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
						requests = append(requests, syncCallbackStopRequest{
							peerID: id,
							from:   from,
						})

						last := from
						if id == "primary" {
							// Leave one later block queued. Neither the
							// inner nor outer loop should process it after
							// the callback requests termination.
							last = test.stopAt + 1
						}

						blocks, terminal := syncCallbackStopBlocks(from, last)

						return blocks, terminal, nil
					},
				},
			}

			progression := &syncCallbackStopProgression{
				mockProgression: &mockProgression{},
			}

			s := NewTestSyncer(
				nil,
				backend,
				time.Second,
				client.mockSyncPeerClient,
				progression,
			)
			s.syncPeerClient = client

			t.Cleanup(func() {
				require.NoError(t, s.Close())
			})

			alternateHead := test.advertised
			if test.stopAt < test.advertised {
				// With the old ordering, this peer supplies exactly one
				// unwanted extra block and then reaches its advertised
				// head. The failing test therefore returns without
				// depending on a timeout or forced shutdown.
				alternateHead = test.stopAt + 1
			}

			s.peerMap.Put(
				&NoForkPeer{
					ID:       "primary",
					Number:   test.advertised,
					Distance: big.NewInt(0),
				},
				&NoForkPeer{
					ID:       "alternate",
					Number:   alternateHead,
					Distance: big.NewInt(1),
				},
			)

			err := s.Sync(func(block *types.FullBlock) bool {
				number := block.Block.Number()
				callbacks = append(callbacks, number)

				return number >= test.stopAt
			})

			require.NoError(t, err)
			require.Equal(t, []syncCallbackStopRequest{
				{peerID: "primary", from: test.initial + 1},
			}, requests)

			expected := make([]uint64, 0)
			for number := test.initial + 1; number <= test.stopAt; number++ {
				expected = append(expected, number)
			}

			require.Equal(t, expected, verified)
			require.Equal(t, expected, written)
			require.Equal(t, expected, callbacks)
			require.Equal(t, test.stopAt, head)

			require.Equal(t, []peer.ID{"primary"}, client.closed)
			require.Equal(t, 1, progression.stops)
			require.Equal(t, test.initial+1, progression.startingBlock)
			require.Equal(t, test.advertised, progression.highestBlock)
		})
	}
}

func TestSyncCallbackStop_FalsePreservesIncompleteDownloadFallback(t *testing.T) {
	tests := []struct {
		name        string
		terminalErr error
	}{
		{
			name: "short_clean_stream",
		},
		{
			name:        "stream_error_after_valid_prefix",
			terminalErr: errors.New("serving peer interrupted the stream"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			head := uint64(100)
			verified := make([]uint64, 0)
			written := make([]uint64, 0)
			callbacks := make([]uint64, 0)
			requests := make([]syncCallbackStopRequest, 0)

			firstBlocks, firstTerminal := syncCallbackStopBlocks(101, 101)

			backend := &mockBlockchain{
				headerHandler: func() *types.Header {
					return &types.Header{Number: head}
				},
				verifyFinalizedBlockHandler: func(block *types.Block) (*types.FullBlock, error) {
					verified = append(verified, block.Number())

					return &types.FullBlock{Block: block}, nil
				},
				writeFullBlockHandler: func(block *types.FullBlock) error {
					number := block.Block.Number()
					written = append(written, number)
					head = number

					return nil
				},
			}

			client := &syncCallbackStopClient{
				mockSyncPeerClient: &mockSyncPeerClient{
					getBlocksHandler: func(
						id peer.ID,
						from uint64,
						_ time.Duration,
					) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
						requests = append(requests, syncCallbackStopRequest{
							peerID: id,
							from:   from,
						})

						if id == "primary" {
							return firstBlocks, firstTerminal, nil
						}

						blocks, terminal := syncCallbackStopBlocks(from, from)

						return blocks, terminal, nil
					},
				},
			}

			progression := &syncCallbackStopProgression{
				mockProgression: &mockProgression{},
			}

			s := NewTestSyncer(
				nil,
				backend,
				time.Second,
				client.mockSyncPeerClient,
				progression,
			)
			s.syncPeerClient = client

			t.Cleanup(func() {
				require.NoError(t, s.Close())
			})

			s.peerMap.Put(
				&NoForkPeer{
					ID:       "primary",
					Number:   110,
					Distance: big.NewInt(0),
				},
				&NoForkPeer{
					ID:       "alternate",
					Number:   102,
					Distance: big.NewInt(1),
				},
			)

			err := s.Sync(func(block *types.FullBlock) bool {
				number := block.Block.Number()
				callbacks = append(callbacks, number)

				if number == 101 {
					// Publish completion only after the valid prefix
					// was imported and its callback was reached.
					firstTerminal <- BlockStreamTerminal{
						Err: test.terminalErr,
					}

					return false
				}

				return number == 102
			})

			require.NoError(t, err)
			require.Equal(t, []syncCallbackStopRequest{
				{peerID: "primary", from: 101},
				{peerID: "alternate", from: 102},
			}, requests)

			require.Equal(t, []uint64{101, 102}, verified)
			require.Equal(t, []uint64{101, 102}, written)
			require.Equal(t, []uint64{101, 102}, callbacks)
			require.Equal(t, uint64(102), head)

			require.Equal(t, []peer.ID{"primary", "alternate"}, client.closed)
			require.Equal(t, 2, progression.stops)
			require.Equal(t, uint64(102), progression.startingBlock)
			require.Equal(t, uint64(102), progression.highestBlock)
		})
	}
}

/* FINISH ALPHA CHANGE 3857 -- regress outer sync callback termination and preserve partial-download fallback -- */
