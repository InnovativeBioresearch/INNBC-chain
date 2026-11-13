/* START ALPHA CHANGE 2299 -- expose the bulk import path only to integration tests -- */
package syncer

import (
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

// Issue12RunBulkSyncForTest allows the external test package to exercise the
// actual import loop without creating a production API or a network listener.
func Issue12RunBulkSyncForTest(
	t *testing.T,
	chain Blockchain,
	block *types.Block,
	callback func(*types.FullBlock) bool,
) (uint64, bool, error) {
	t.Helper()

	sourcePeer := peer.ID("issue12-test-source")
	client := &mockSyncPeerClient{
		getBlocksHandler: func(
			id peer.ID,
			start uint64,
			_ time.Duration,
		) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
			require.Equal(t, sourcePeer, id)
			require.Equal(t, chain.Header().Number+1, start)
			require.Equal(t, start, block.Number())

			blocks := make(chan *types.Block, 1)
			blocks <- block

			// Validation failure or the successful callback ends the attempt.
			// No terminal result races with delivery of the candidate block.
			return blocks, make(chan BlockStreamTerminal), nil
		},
	}

	s := NewTestSyncer(nil, chain, time.Second, client, &mockProgression{})
	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})

	return s.bulkSyncWithPeer(sourcePeer, block.Number(), callback)
}

/* FINISH ALPHA CHANGE 2299 -- expose the bulk import path only to integration tests -- */
