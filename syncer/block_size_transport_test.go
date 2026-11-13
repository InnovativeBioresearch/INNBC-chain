/* START ALPHA CHANGE 2894 -- measure block transfer around the unchanged gRPC receive boundary -- */

package syncer

import (
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
	protobuf "google.golang.org/protobuf/proto"
)

const sizeSyncReceiveLimit = 4 << 20

type sizeTransportBlockchain struct {
	Blockchain
	block *types.Block
}

func (chain *sizeTransportBlockchain) Header() *types.Header {
	return chain.block.Header
}

func (*sizeTransportBlockchain) HistoryFrom() uint64 {
	return 1
}

func (chain *sizeTransportBlockchain) GetBlockByNumber(
	number uint64,
	_ bool,
) (*types.Block, bool) {
	if number != chain.block.Number() {
		return nil, false
	}
	return chain.block, true
}

func sizeTransportBlock(t *testing.T, responseTarget int) *types.Block {
	t.Helper()

	const (
		transactionCount = 35
		maxInput         = 120_000
	)

	/*
		These are structurally decodable transport fixtures, not finalized
		consensus blocks. Do not claim signature or execution validation.
		Bound the input and adjustment iterations before allocating.
	*/
	require.Greater(t, responseTarget, sizeSyncReceiveLimit-4096)
	require.Less(t, responseTarget, sizeSyncReceiveLimit+4096)

	inputBytes := responseTarget - 4096
	to := types.StringToAddress("0x0000000000000000000000000000000000009700")

	for attempt := 0; attempt < 8; attempt++ {
		require.GreaterOrEqual(t, inputBytes, 0)
		require.LessOrEqual(t, inputBytes, transactionCount*maxInput)

		block := &types.Block{
			Header: &types.Header{
				Number:       1,
				Timestamp:    1002,
				Difficulty:   1,
				GasLimit:     20_000_000,
				StateRoot:    types.EmptyRootHash,
				TxRoot:       types.EmptyRootHash,
				ReceiptsRoot: types.EmptyRootHash,
				Sha3Uncles:   types.EmptyUncleHash,
			},
		}

		remaining := inputBytes
		for i := 0; i < transactionCount; i++ {
			length := remaining
			if length > maxInput {
				length = maxInput
			}
			remaining -= length

			gas := uint64(21_000 + 4*length)
			tx := &types.Transaction{
				Nonce:    uint64(i),
				To:       &to,
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(1),
				Gas:      gas,
				Input:    make([]byte, length),
				V:        big.NewInt(27),
				R:        big.NewInt(1),
				S:        big.NewInt(1),
			}
			require.LessOrEqual(t, len(tx.MarshalRLP()), 128*1024)
			block.Transactions = append(block.Transactions, tx)
			block.Header.GasUsed += gas
		}
		require.Zero(t, remaining)
		require.LessOrEqual(t, block.Header.GasUsed, block.Header.GasLimit)
		block.Header.ComputeHash()

		response := toProtoBlock(block)
		actualSize := protobuf.Size(response)
		if actualSize == responseTarget {
			require.LessOrEqual(t, len(response.Block), sizeSyncReceiveLimit+4096)

			decoded := new(types.Block)
			require.NoError(t, decoded.UnmarshalRLP(response.Block))
			require.Len(t, decoded.Transactions, transactionCount)
			require.Equal(t, uint64(1), decoded.Number())
			return block
		}

		inputBytes += responseTarget - actualSize
	}

	t.Fatal("bounded block fixture did not converge to the requested protobuf size")
	return nil
}

func TestSyncBlockSize_DefaultReceiveBoundary(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		target  int
		success bool
	}{
		{
			name:    "below_default_receive_limit",
			target:  sizeSyncReceiveLimit - 1024,
			success: true,
		},
		{
			name:    "above_default_receive_limit",
			target:  sizeSyncReceiveLimit + 1024,
			success: false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			block := sizeTransportBlock(t, testCase.target)
			blockchain := &sizeTransportBlockchain{block: block}

			/*
				Use the production serving handler and client over real gRPC.
				The existing fixture supplies no receive-size override.
				Authorization and libp2p establishment are separate test scopes.
			*/
			service := NewSyncPeerService(nil, blockchain).(*syncPeerService)
			networkStub, connections := newOwnershipRPCNetwork(t, service)
			client := NewSyncPeerClient(
				hclog.NewNullLogger(),
				networkStub,
				nil,
			).(*syncPeerClient)
			t.Cleanup(func() {
				closeSyncTimingClient(t, client)
			})

			blocks, terminal, err := client.GetBlocks("peer", 1, 30*time.Second)
			require.NoError(t, err)

			connectionTimer := time.NewTimer(syncTimingWatchdog)
			defer connectionTimer.Stop()

			select {
			case conn := <-connections:
				require.NotNil(t, conn)

				watchdog := time.NewTimer(30 * time.Second)
				defer watchdog.Stop()

				var received []*types.Block
				var result BlockStreamTerminal
				finished := false

				for !finished {
					select {
					case next, ok := <-blocks:
						if !ok {
							blocks = nil
							continue
						}
						require.NotNil(t, next)
						received = append(received, next)
					case next, ok := <-terminal:
						require.True(t, ok,
							"terminal channel closed without its required result")
						result = next
						finished = true
					case <-watchdog.C:
						t.Fatal("boundary-sized block transfer did not terminate")
					}
				}

				if testCase.success {
					require.NoError(t, result.Err)
					require.Len(t, received, 1)
					require.Equal(t, uint64(1), received[0].Number())
					require.Equal(t, block.MarshalRLP(), received[0].MarshalRLP())
				} else {
					require.Empty(t, received)
					require.Error(t, result.Err)
					require.Equal(t, codes.ResourceExhausted, status.Code(result.Err))
				}

				/* Completion must follow owned connection and registration cleanup. */
				require.Equal(t, connectivity.Shutdown, conn.GetState())
				client.blockStreamLock.Lock()
				retainedDownloads := len(client.blockStreamCancels)
				client.blockStreamLock.Unlock()
				require.Zero(t, retainedDownloads)

				t.Logf(
					"block_rlp=%d protobuf_response=%d receive_limit=%d "+
						"delivered_blocks=%d terminal_code=%s",
					len(block.MarshalRLP()), protobuf.Size(toProtoBlock(block)),
					sizeSyncReceiveLimit, len(received), status.Code(result.Err),
				)
				closeSyncTimingClient(t, client)

			case <-connectionTimer.C:
				t.Fatal("real gRPC client connection was not established")
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2894 -- measure block transfer around the unchanged gRPC receive boundary -- */
