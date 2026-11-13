package syncer

/* START ALPHA CHANGE 2910 -- identify and join existing service test transports -- */
import (
	"context"
	"io"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/emptypb"
)

func newMockGrpcClient(t *testing.T, service *syncPeerService) proto.SyncPeerClient {
	t.Helper()

	transport := newBlockServingTransport(t, service)
	conn := blockServingDial(t, transport, "peer")

	return proto.NewSyncPeerClient(conn)
}

/* FINISH ALPHA CHANGE 2910 -- identify and join existing service test transports -- */

func Test_syncPeerService_GetBlocks(t *testing.T) {
	t.Parallel()

	blocks := createMockBlocks(10)

	tests := []struct {
		name           string
		from           uint64
		latest         uint64
		blocks         []*types.Block
		receivedBlocks []*types.Block
		err            error
	}{
		{
			name:           "should send the blocks to the latest",
			from:           5,
			latest:         10,
			blocks:         blocks,
			receivedBlocks: blocks[4:], // from 5
			err:            io.EOF,
		},
		{
			name:           "should return ErrBlockNotFound",
			from:           5,
			latest:         10,
			blocks:         blocks[:8],
			receivedBlocks: blocks[4:8], // from 5
			err:            ErrBlockNotFound,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			blockMap := make(map[uint64]*types.Block)

			for _, b := range test.blocks {
				blockMap[b.Number()] = b
			}

			service := &syncPeerService{
				blockchain: &mockBlockchain{
					headerHandler: newSimpleHeaderHandler(test.latest),
					getBlockByNumberHandler: func(u uint64, full bool) (*types.Block, bool) {
						block, ok := blockMap[u]
						if !ok {
							return nil, false
						}

						return block, true
					},
				},
			}

			client := newMockGrpcClient(t, service)

			stream, err := client.GetBlocks(context.Background(), &proto.GetBlocksRequest{
				From: test.from,
			})

			assert.NoError(t, err)

			count := 0

			for {
				protoBlock, err := stream.Recv()
				if err != nil {
					assert.Contains(t, err.Error(), test.err.Error())

					break
				}

				expected := test.receivedBlocks[count].MarshalRLP()

				assert.Equal(t, expected, protoBlock.Block)

				count++
			}
		})
	}
}

func TestGetStatus(t *testing.T) {
	t.Parallel()

	headerNumber := uint64(10)
	historyFrom := uint64(5)

	service := &syncPeerService{
		blockchain: &mockBlockchain{
			headerHandler: newSimpleHeaderHandler(headerNumber),
			/* START ALPHA CHANGE 1684 -- provide service history availability -- */
			historyFromHandler: func() uint64 {
				return historyFrom
			},
			/* FINISH ALPHA CHANGE 1684 -- provide service history availability -- */
		},
	}

	client := newMockGrpcClient(t, service)

	status, err := client.GetStatus(context.Background(), &emptypb.Empty{})

	assert.NoError(t, err)
	assert.Equal(t, headerNumber, status.Number)
	/* START ALPHA CHANGE 1685 -- verify direct history availability advertisement -- */
	assert.Equal(t, historyFrom, status.HistoryFrom)
	/* FINISH ALPHA CHANGE 1685 -- verify direct history availability advertisement -- */
}
