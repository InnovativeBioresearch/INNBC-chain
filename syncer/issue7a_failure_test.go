/* START ALPHA CHANGE 2176 -- test local sync failures without peer exclusion or accelerated retries -- */
package syncer

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestIssue7A_SyncLocalFailureRetriesSamePeer(t *testing.T) {
	for _, phase := range []string{"verify", "write"} {
		t.Run(phase, func(t *testing.T) {
			block := createMockBlocks(1)[0]
			local := state.WrapLocalStateFailure(
				"sync execution",
				errors.New("injected local state failure"),
			)

			const retryDelay = 20 * time.Millisecond
			var failedAt time.Time
			var failed bool
			var head uint64
			var attempts int
			var writes int
			var callbacks int

			backend := &mockBlockchain{
				headerHandler: func() *types.Header {
					return &types.Header{Number: head}
				},
				verifyFinalizedBlockHandler: func(b *types.Block) (*types.FullBlock, error) {
					if phase == "verify" && !failed {
						failed = true
						failedAt = time.Now()

						return nil, local
					}

					return &types.FullBlock{Block: b}, nil
				},
				writeFullBlockHandler: func(b *types.FullBlock) error {
					if phase == "write" && !failed {
						failed = true
						failedAt = time.Now()

						return local
					}

					writes++
					head = b.Block.Number()

					return nil
				},
			}

			client := &mockSyncPeerClient{
				getBlocksHandler: func(
					id peer.ID,
					start uint64,
					_ time.Duration,
				) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
					attempts++
					require.LessOrEqual(t, attempts, 2)
					require.Equal(t, peer.ID("A"), id)
					require.Equal(t, uint64(1), start)

					if attempts == 2 {
						require.False(t, failedAt.IsZero())
						require.GreaterOrEqual(t, time.Since(failedAt), retryDelay)
					}

					blocks := make(chan *types.Block, 1)
					blocks <- block

					// No terminal result can race with the block.
					// Failure or the successful callback ends this attempt.
					return blocks, make(chan BlockStreamTerminal), nil
				},
			}

			s := NewTestSyncer(nil, backend, time.Second, client, &mockProgression{})
			s.peerRetryInterval = retryDelay

			// A ready status notification must not bypass the local-failure delay.
			s.newStatusCh = make(chan struct{}, 1)
			s.newStatusCh <- struct{}{}

			s.peerMap.Put(
				&NoForkPeer{ID: peer.ID("A"), Number: 1, Distance: big.NewInt(0)},
				&NoForkPeer{ID: peer.ID("B"), Number: 1, Distance: big.NewInt(1)},
			)

			err := s.Sync(func(b *types.FullBlock) bool {
				callbacks++
				require.Equal(t, uint64(1), b.Block.Number())

				return true
			})
			require.NoError(t, err)
			require.Equal(t, 2, attempts)
			require.Equal(t, 1, writes)
			require.Equal(t, 1, callbacks)
			require.Equal(t, uint64(1), head)
		})
	}
}

func TestIssue7A_LocalSyncRetryWaitHonorsShutdown(t *testing.T) {
	s := &syncer{
		peerRetryInterval: time.Hour,
		closeCh:           make(chan struct{}),
	}
	close(s.closeCh)

	require.False(t, s.waitForLocalStateRetry())
}

/* FINISH ALPHA CHANGE 2176 -- test local sync failures without peer exclusion or accelerated retries -- */
