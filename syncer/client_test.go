package syncer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/network"
	"github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
)

var (
	networkConfig = func(c *network.Config) {
		c.NoDiscover = true
	}
)

/* START ALPHA CHANGE 2736 -- close sync test network fixtures and expose peer establishment diagnostics -- */
func newTestNetwork(t *testing.T) *network.Server {
	t.Helper()

	srv, err := network.CreateServer(&network.CreateServerParams{
		ConfigCallback: networkConfig,
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:  t.Name(),
			Level: hclog.Debug,
		}),
	})

	if srv != nil {
		t.Cleanup(func() {
			assert.NoError(t, srv.Close())
		})
	}

	require.NoError(t, err)
	require.NotNil(t, srv)

	return srv
}

/* FINISH ALPHA CHANGE 2736 -- close sync test network fixtures and expose peer establishment diagnostics -- */

func newTestSyncPeerClient(network Network, blockchain Blockchain) *syncPeerClient {
	client := &syncPeerClient{
		logger:     hclog.NewNullLogger(),
		network:    network,
		blockchain: blockchain,
		id:         network.AddrInfo().ID.String(),
		/* START ALPHA CHANGE 2820 -- initialize owned status channels in client fixtures -- */
		peerStatusUpdateCh: make(chan *PeerStatusUpdate, 1),
		/* FINISH ALPHA CHANGE 2820 -- initialize owned status channels in client fixtures -- */
		peerConnectionUpdateCh: make(chan *event.PeerEvent, 1),
		closeCh:                make(chan struct{}),

		/* START ALPHA CHANGE 1352 -- initialize sync block stream cancellation in tests -- */
		blockStreamCancels: make(map[peer.ID]*blockStreamCancelState),
		/* FINISH ALPHA CHANGE 1352 -- initialize sync block stream cancellation in tests -- */
	}

	// need to register protocol
	network.RegisterProtocol(syncerProto, grpc.NewGrpcStream())

	return client
}

func createTestSyncerService(t *testing.T, chain Blockchain) (*syncPeerService, *network.Server) {
	t.Helper()

	srv := newTestNetwork(t)

	service := &syncPeerService{
		blockchain: chain,
		network:    srv,
	}

	service.Start()

	return service, srv
}

func TestGetPeerStatus(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)

	peerLatest := uint64(10)
	peerHistoryFrom := uint64(5)
	_, peerSrv := createTestSyncerService(t, &mockBlockchain{
		headerHandler: newSimpleHeaderHandler(peerLatest),
		/* START ALPHA CHANGE 1686 -- advertise peer history in direct client test -- */
		historyFromHandler: func() uint64 {
			return peerHistoryFrom
		},
		/* FINISH ALPHA CHANGE 1686 -- advertise peer history in direct client test -- */
	})

	err := network.JoinAndWait(
		clientSrv,
		peerSrv,
		network.DefaultBufferTimeout,
		network.DefaultJoinTimeout,
	)

	assert.NoError(t, err)

	status, err := client.GetPeerStatus(peerSrv.AddrInfo().ID)
	assert.NoError(t, err)

	expected := &NoForkPeer{
		ID:     peerSrv.AddrInfo().ID,
		Number: peerLatest,
		/* START ALPHA CHANGE 1687 -- verify decoded direct history availability -- */
		HistoryFrom: peerHistoryFrom,
		/* FINISH ALPHA CHANGE 1687 -- verify decoded direct history availability -- */
		Distance: clientSrv.GetPeerDistance(peerSrv.AddrInfo().ID),
	}

	assert.Equal(t, expected, status)
}

func TestGetConnectedPeerStatuses(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)

	var (
		peerLatests = []uint64{
			30,
			20,
			10,
		}

		mutex        = sync.Mutex{}
		peerJoinErrs = make([]error, len(peerLatests))
		expected     = make([]*NoForkPeer, len(peerLatests))

		wg sync.WaitGroup
	)

	for idx, latest := range peerLatests {
		idx, latest := idx, latest

		_, peerSrv := createTestSyncerService(t, &mockBlockchain{
			headerHandler: newSimpleHeaderHandler(latest),
		})

		peerID := peerSrv.AddrInfo().ID

		wg.Add(1)

		go func() {
			defer wg.Done()

			mutex.Lock()
			defer mutex.Unlock()

			peerJoinErrs[idx] = network.JoinAndWait(
				clientSrv,
				peerSrv,
				network.DefaultBufferTimeout,
				network.DefaultJoinTimeout,
			)

			expected[idx] = &NoForkPeer{
				ID:       peerID,
				Number:   latest,
				Distance: clientSrv.GetPeerDistance(peerID),
			}
		}()
	}

	wg.Wait()

	for _, err := range peerJoinErrs {
		assert.NoError(t, err)
	}

	statuses := client.GetConnectedPeerStatuses()

	// no need to check order
	assert.Equal(t, expected, sortNoForkPeers(statuses))
}

/* START ALPHA CHANGE 2093 -- await forwarded peer events before closing the sync client -- */
func TestStatusPubSub(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)
	t.Cleanup(client.Close)

	_, peerSrv := createTestSyncerService(t, &mockBlockchain{})
	peerID := peerSrv.AddrInfo().ID

	require.True(t, client.startWorker(client.startPeerEventProcess))

	eventCh := client.GetPeerConnectionUpdateEventCh()

	waitForEvent := func(expectedType event.PeerEventType) {
		t.Helper()

		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()

		select {
		case received, open := <-eventCh:
			require.True(t, open, "peer event channel closed before expected event")
			require.NotNil(t, received)
			/* START ALPHA CHANGE 2754 -- compare observable event identity without comparing callbacks -- */
			require.Equal(t, peerID, received.PeerID)
			require.Equal(t, expectedType, received.Type)
			/* FINISH ALPHA CHANGE 2754 -- compare observable event identity without comparing callbacks -- */

		case <-timer.C:
			t.Fatalf("timed out waiting for forwarded peer event %v", expectedType)
		}
	}

	err := network.JoinAndWait(
		clientSrv,
		peerSrv,
		network.DefaultBufferTimeout,
		network.DefaultJoinTimeout,
	)
	require.NoError(t, err)

	waitForEvent(event.PeerConnected)

	err = network.DisconnectAndWait(
		clientSrv,
		peerID,
		network.DefaultLeaveTimeout,
	)
	require.NoError(t, err)

	/* START ALPHA CHANGE 2886 -- verify session cleanup followed by the preserved application disconnect event -- */
	waitForEvent(event.PeerSessionEnded)
	waitForEvent(event.PeerDisconnected)
	/* FINISH ALPHA CHANGE 2886 -- verify session cleanup followed by the preserved application disconnect event -- */

	// Close joins the forwarding worker and closes its output channel.
	client.Close()

	select {
	case unexpected, open := <-eventCh:
		require.False(t, open, "unexpected additional peer event: %v", unexpected)
	default:
		t.Fatal("peer event channel remains open after client.Close")
	}
}

/* FINISH ALPHA CHANGE 2093 -- await forwarded peer events before closing the sync client -- */

func TestPeerConnectionUpdateEventCh(t *testing.T) {
	t.Skip()
	t.Parallel()

	var (
		// network layer
		clientSrv = newTestNetwork(t)
		peerSrv1  = newTestNetwork(t)
		peerSrv2  = newTestNetwork(t)
		peerSrv3  = newTestNetwork(t) // to wait for gossipped message

		// latest block height
		peerLatest1 = uint64(10)
		peerLatest2 = uint64(20)

		// blockchain subscription
		subscription1 = blockchain.NewMockSubscription()
		subscription2 = blockchain.NewMockSubscription()

		// syncer client
		client = newTestSyncPeerClient(clientSrv, &mockBlockchain{
			subscription: &blockchain.MockSubscription{},
		})
		peerClient1 = newTestSyncPeerClient(peerSrv1, &mockBlockchain{
			subscription:  subscription1,
			headerHandler: newSimpleHeaderHandler(peerLatest1),
		})
		peerClient2 = newTestSyncPeerClient(peerSrv2, &mockBlockchain{
			subscription:  subscription2,
			headerHandler: newSimpleHeaderHandler(peerLatest2),
		})
	)

	t.Cleanup(func() {
		// no need to call Close of Client because test closes it manually
		peerClient1.Close()
		peerClient2.Close()

		clientSrv.Close()
		peerSrv1.Close()
		peerSrv2.Close()
		peerSrv3.Close()
	})

	// client <-> peer1
	// peer1  <-> peer2
	err := network.JoinAndWaitMultiple(
		network.DefaultJoinTimeout,
		clientSrv,
		peerSrv1,
		peerSrv1,
		peerSrv2,
		peerSrv2,
		peerSrv3,
	)

	assert.NoError(t, err)

	// start gossip
	assert.NoError(t, client.startGossip())
	assert.NoError(t, peerClient1.startGossip())
	assert.NoError(t, peerClient2.startGossip())

	// create topic
	topic, err := peerSrv3.NewTopic(statusTopicName, &proto.SyncPeerStatus{})
	assert.NoError(t, err)

	var wgForGossip sync.WaitGroup

	// 2 messages should be gossipped
	wgForGossip.Add(2)

	handler := func(_ interface{}, _ peer.ID) {
		wgForGossip.Done()
	}

	assert.NoError(t, topic.Subscribe(handler))

	// need to wait for a few seconds to propagate subscribing
	time.Sleep(2 * time.Second)

	// enable peers to send own status via gossip
	peerClient1.EnablePublishingPeerStatus()
	peerClient2.EnablePublishingPeerStatus()

	// start to subscribe blockchain events
	assert.True(t, peerClient1.startWorker(peerClient1.startNewBlockProcess))
	assert.True(t, peerClient2.startWorker(peerClient2.startNewBlockProcess))

	// collect peer status changes
	var (
		wgForConnectingStatus sync.WaitGroup
		newStatuses           []*NoForkPeer
	)

	wgForConnectingStatus.Add(1)

	go func() {
		defer wgForConnectingStatus.Done()

		/* START ALPHA CHANGE 2821 -- compare peer values independently of local publication ownership -- */
		for update := range client.GetPeerStatusUpdateCh() {
			newStatuses = append(newStatuses, update.Status)
		}
		/* FINISH ALPHA CHANGE 2821 -- compare peer values independently of local publication ownership -- */
	}()

	// push latest block number to blockchain subscription
	pushSubscription := func(sub *blockchain.MockSubscription, latest uint64) {
		sub.Push(&blockchain.Event{
			NewChain: []*types.Header{
				{
					Number: latest,
				},
			},
		})
	}

	// peer1 and peer2 emit Blockchain event
	// they should publish their status via gossip
	pushSubscription(subscription1, peerLatest1)
	pushSubscription(subscription2, peerLatest2)

	// wait until 2 messages are propagated
	wgForGossip.Wait()

	// close to terminate goroutine
	client.Close()

	// wait until collecting routine is done
	wgForConnectingStatus.Wait()

	// client connects to only peer1, then expects to have a status from peer1
	expected := []*NoForkPeer{
		{
			ID:       peerSrv1.AddrInfo().ID,
			Number:   peerLatest1,
			Distance: clientSrv.GetPeerDistance(peerSrv1.AddrInfo().ID),
		},
	}

	assert.Equal(t, expected, newStatuses)
}

// Make sure the peer shouldn't emit status if the shouldEmitBlocks flag is set.
// The subtests cannot contain t.Parallel() due to how
// the test is organized
//
//nolint:tparallel
func Test_shouldEmitBlocks(t *testing.T) {
	t.Parallel()

	var (
		// network layer
		clientSrv = newTestNetwork(t)
		peerSrv   = newTestNetwork(t)

		clientLatest = uint64(10)

		subscription = blockchain.NewMockSubscription()

		client = newTestSyncPeerClient(clientSrv, &mockBlockchain{
			subscription:  subscription,
			headerHandler: newSimpleHeaderHandler(clientLatest),
		})
	)

	t.Cleanup(func() {
		client.Close()
		clientSrv.Close()
		peerSrv.Close()
	})

	err := network.JoinAndWaitMultiple(
		network.DefaultJoinTimeout,
		clientSrv,
		peerSrv,
	)

	assert.NoError(t, err)

	// start gossip
	assert.NoError(t, client.startGossip())

	// start to subscribe blockchain events
	assert.True(t, client.startWorker(client.startNewBlockProcess))

	// push latest block number to blockchain subscription
	pushSubscription := func(sub *blockchain.MockSubscription, latest uint64) {
		sub.Push(&blockchain.Event{
			NewChain: []*types.Header{
				{
					Number: latest,
				},
			},
		})
	}

	waitForContext := func(ctx context.Context) bool {
		select {
		case <-ctx.Done():
			return true
		case <-time.After(5 * time.Second):
			return false
		}
	}

	// create topic & subscribe in peer
	topic, err := peerSrv.NewTopic(statusTopicName, &proto.SyncPeerStatus{})
	assert.NoError(t, err)

	testGossip := func(t *testing.T, shouldEmit bool) {
		t.Helper()

		// context to be canceled when receiving status
		receiveContext, cancelContext := context.WithCancel(context.Background())
		defer cancelContext()

		assert.NoError(t, topic.Subscribe(func(_ interface{}, id peer.ID) {
			cancelContext()
		}))

		// need to wait for a few seconds to propagate subscribing
		time.Sleep(2 * time.Second)

		if shouldEmit {
			client.EnablePublishingPeerStatus()
		} else {
			client.DisablePublishingPeerStatus()
		}

		pushSubscription(subscription, clientLatest)

		canceled := waitForContext(receiveContext)

		assert.Equal(t, shouldEmit, canceled)
	}

	t.Run("should send own status via gossip if shouldEmitBlocks is set", func(t *testing.T) {
		testGossip(t, true)
	})

	t.Run("shouldn't send own status via gossip if shouldEmitBlocks is reset", func(t *testing.T) {
		testGossip(t, false)
	})
}

func Test_syncPeerClient_GetBlocks(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)

	var (
		peerLatest = uint64(10)
		syncFrom   = uint64(1)
	)

	_, peerSrv := createTestSyncerService(t, &mockBlockchain{
		headerHandler: newSimpleHeaderHandler(peerLatest),
		getBlockByNumberHandler: func(u uint64, b bool) (*types.Block, bool) {
			if u <= 10 {
				return &types.Block{
					Header: &types.Header{
						Number: u,
					},
				}, true
			}

			return nil, false
		},
	})

	err := network.JoinAndWait(
		clientSrv,
		peerSrv,
		network.DefaultBufferTimeout,
		network.DefaultJoinTimeout,
	)

	require.NoError(t, err)

	blockStream, terminalCh, err := client.GetBlocks(
		peerSrv.AddrInfo().ID,
		syncFrom,
		5*time.Second,
	)
	require.NoError(t, err)

	blocks := make([]*types.Block, 0, peerLatest)
	for block := range blockStream {
		blocks = append(blocks, block)
	}

	terminal, ok := <-terminalCh
	require.True(t, ok)
	require.NoError(t, terminal.Err)

	_, ok = <-terminalCh
	require.False(t, ok)

	// hash is calculated on unmarshaling
	expected := createMockBlocks(10)
	for _, b := range expected {
		b.Header.ComputeHash()
	}

	assert.Equal(t, expected, blocks)
}

/* START ALPHA CHANGE 1353 -- test sync peer client terminal error and ordering delivery -- */
func Test_syncPeerClient_GetBlocksDeliversFinalBlockBeforeTerminal(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)

	_, peerSrv := createTestSyncerService(t, &mockBlockchain{
		headerHandler: newSimpleHeaderHandler(1),
		getBlockByNumberHandler: func(u uint64, full bool) (*types.Block, bool) {
			return &types.Block{
				Header: &types.Header{
					Number: u,
				},
			}, true
		},
	})

	err := network.JoinAndWait(
		clientSrv,
		peerSrv,
		network.DefaultBufferTimeout,
		network.DefaultJoinTimeout,
	)
	require.NoError(t, err)

	blockStream, terminalCh, err := client.GetBlocks(
		peerSrv.AddrInfo().ID,
		1,
		5*time.Second,
	)
	require.NoError(t, err)

	select {
	case terminal, ok := <-terminalCh:
		if !ok {
			t.Fatal("terminal channel closed before final block delivery")
		}

		t.Fatalf("terminal result arrived before final block: %v", terminal.Err)
	case <-time.After(50 * time.Millisecond):
	}

	block, ok := <-blockStream
	require.True(t, ok)
	require.NotNil(t, block)
	require.Equal(t, uint64(1), block.Number())

	terminal, ok := <-terminalCh
	require.True(t, ok)
	require.NoError(t, terminal.Err)

	_, ok = <-terminalCh
	require.False(t, ok)

	_, ok = <-blockStream
	require.False(t, ok)
}

func Test_syncPeerClient_GetBlocksReportsMissingBlockTerminalError(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)

	_, peerSrv := createTestSyncerService(t, &mockBlockchain{
		headerHandler: newSimpleHeaderHandler(1),
		getBlockByNumberHandler: func(u uint64, full bool) (*types.Block, bool) {
			if u != 1 {
				return nil, false
			}

			if full {
				return nil, false
			}

			return &types.Block{
				Header: &types.Header{
					Number: u,
				},
			}, true
		},
	})

	err := network.JoinAndWait(
		clientSrv,
		peerSrv,
		network.DefaultBufferTimeout,
		network.DefaultJoinTimeout,
	)
	require.NoError(t, err)

	blockStream, terminalCh, err := client.GetBlocks(
		peerSrv.AddrInfo().ID,
		1,
		5*time.Second,
	)
	require.NoError(t, err)

	_, ok := <-blockStream
	require.False(t, ok)

	terminal, ok := <-terminalCh
	require.True(t, ok)
	require.Error(t, terminal.Err)
	require.ErrorContains(t, terminal.Err, ErrBlockNotFound.Error())

	_, ok = <-terminalCh
	require.False(t, ok)
}

func Test_syncPeerClient_GetBlocksReportsTimeoutTerminal(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)

	_, peerSrv := createTestSyncerService(t, &mockBlockchain{
		headerHandler: newSimpleHeaderHandler(1),
		getBlockByNumberHandler: func(u uint64, full bool) (*types.Block, bool) {
			time.Sleep(200 * time.Millisecond)

			return &types.Block{
				Header: &types.Header{
					Number: u,
				},
			}, true
		},
	})

	err := network.JoinAndWait(
		clientSrv,
		peerSrv,
		network.DefaultBufferTimeout,
		network.DefaultJoinTimeout,
	)
	require.NoError(t, err)

	blockStream, terminalCh, err := client.GetBlocks(
		peerSrv.AddrInfo().ID,
		1,
		20*time.Millisecond,
	)
	require.NoError(t, err)

	_, ok := <-blockStream
	require.False(t, ok)

	terminal, ok := <-terminalCh
	require.True(t, ok)
	require.ErrorIs(t, terminal.Err, errTimeout)

	_, ok = <-terminalCh
	require.False(t, ok)
}

/* FINISH ALPHA CHANGE 1353 -- test sync peer client terminal error and ordering delivery -- */

func Test_EmitMultipleBlocks(t *testing.T) {
	t.Parallel()

	var (
		// network layer
		clientSrv = newTestNetwork(t)
		peerSrv   = newTestNetwork(t)

		clientLatest = uint64(10)

		subscription = blockchain.NewMockSubscription()

		client = newTestSyncPeerClient(clientSrv, &mockBlockchain{
			subscription:  subscription,
			headerHandler: newSimpleHeaderHandler(clientLatest),
		})
	)

	t.Cleanup(func() {
		client.Close()
		clientSrv.Close()
		peerSrv.Close()
	})

	err := network.JoinAndWaitMultiple(
		network.DefaultJoinTimeout,
		clientSrv,
		peerSrv,
	)

	require.NoError(t, err)

	// start gossip
	require.NoError(t, client.startGossip())

	// start to subscribe blockchain events
	require.True(t, client.startWorker(client.startNewBlockProcess))

	// push latest block number to blockchain subscription
	pushSubscription := func(sub *blockchain.MockSubscription, latest uint64) {
		sub.Push(&blockchain.Event{
			NewChain: []*types.Header{
				{
					Number: latest,
				},
			},
		})
	}

	waitForGossip := func(wg *sync.WaitGroup) bool {
		c := make(chan struct{})

		go func() {
			defer close(c)
			wg.Wait()
		}()

		select {
		case <-c:
			return true
		case <-time.After(5 * time.Second):
			return false
		}
	}

	// create topic & subscribe in peer
	topic, err := peerSrv.NewTopic(statusTopicName, &proto.SyncPeerStatus{})
	assert.NoError(t, err)

	testGossip := func(t *testing.T, blocksNum int) {
		t.Helper()

		var wgForGossip sync.WaitGroup

		wgForGossip.Add(blocksNum)

		require.NoError(t, topic.Subscribe(func(_ interface{}, _ peer.ID) {
			wgForGossip.Done()
		}))

		// need to wait for a few seconds to propagate subscribing
		time.Sleep(2 * time.Second)
		client.EnablePublishingPeerStatus()

		go func() {
			for i := 0; i < blocksNum; i++ {
				pushSubscription(subscription, clientLatest+uint64(i))
			}
		}()

		gossiped := waitForGossip(&wgForGossip)

		require.Equal(t, true, gossiped)
	}

	t.Run("should receive all blocks", func(t *testing.T) {
		t.Parallel()
		testGossip(t, 4)
	})
}
