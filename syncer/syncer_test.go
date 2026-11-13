package syncer

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/helper/progress"

	/* START ALPHA CHANGE 2812 -- import explicit mock session ownership -- */
	chainnetwork "github.com/innovativebioresearch/innbc-chain/network"
	/* FINISH ALPHA CHANGE 2812 -- import explicit mock session ownership -- */
	"github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProgression struct {
	startingBlock uint64
	highestBlock  uint64
}

func (m *mockProgression) StartProgression(startingBlock uint64, subscription blockchain.Subscription) {
	m.startingBlock = startingBlock
}

func (m *mockProgression) UpdateHighestProgression(highestBlock uint64) {
	m.highestBlock = highestBlock
}

func (m *mockProgression) GetProgression() *progress.Progression {
	// Syncer doesn't use this method. It just exports
	return nil
}

type mockBlockchain struct {
	subscription  blockchain.Subscription
	headerHandler func() *types.Header
	/* START ALPHA CHANGE 1676 -- model advertised history availability -- */
	historyFromHandler func() uint64
	/* FINISH ALPHA CHANGE 1676 -- model advertised history availability -- */
	getBlockByNumberHandler     func(uint64, bool) (*types.Block, bool)
	verifyFinalizedBlockHandler func(*types.Block) (*types.FullBlock, error)
	writeBlockHandler           func(*types.Block) error
	writeFullBlockHandler       func(*types.FullBlock) error
}

func (m *mockBlockchain) SubscribeEvents() blockchain.Subscription {
	return m.subscription
}

func (m *mockBlockchain) UnsubscribeEvents(blockchain.Subscription) {
}

func (m *mockBlockchain) Header() *types.Header {
	return m.headerHandler()
}

/* START ALPHA CHANGE 1677 -- return mocked history availability -- */
func (m *mockBlockchain) HistoryFrom() uint64 {
	if m.historyFromHandler != nil {
		return m.historyFromHandler()
	}

	return 0
}

/* FINISH ALPHA CHANGE 1677 -- return mocked history availability -- */

func (m *mockBlockchain) GetBlockByNumber(number uint64, full bool) (*types.Block, bool) {
	return m.getBlockByNumberHandler(number, full)
}

func (m *mockBlockchain) VerifyFinalizedBlock(b *types.Block) (*types.FullBlock, error) {
	return m.verifyFinalizedBlockHandler(b)
}

func (m *mockBlockchain) WriteBlock(b *types.Block, s string) error {
	return m.writeBlockHandler(b)
}

func (m *mockBlockchain) WriteFullBlock(b *types.FullBlock, s string) error {
	return m.writeFullBlockHandler(b)
}

func newSimpleHeaderHandler(num uint64) func() *types.Header {
	return func() *types.Header {
		return &types.Header{
			Number: num,
		}
	}
}

type mockSyncPeerService struct{}

func (m *mockSyncPeerService) Start() {}

func (m *mockSyncPeerService) Close() error {
	return nil
}

func (m *mockProgression) StopProgression() {}

/* START ALPHA CHANGE 2811 -- supply explicit ownership in sync client mocks -- */
type mockSyncPeerClient struct {
	getPeerStatusHandler                  func(peer.ID) (*NoForkPeer, error)
	getConnectedPeerStatusesHandler       func() []*NoForkPeer
	getBlocksHandler                      func(peer.ID, uint64, time.Duration) (<-chan *types.Block, <-chan BlockStreamTerminal, error)
	getPeerStatusUpdateChHandler          func() <-chan *PeerStatusUpdate
	getPeerConnectionUpdateEventChHandler func() <-chan *event.PeerEvent

	statusSession                        func(peer.ID) *chainnetwork.PeerSession
	getConnectedPeerStatusUpdatesHandler func() []*PeerStatusUpdate
}

/* FINISH ALPHA CHANGE 2811 -- supply explicit ownership in sync client mocks -- */

func (m *mockSyncPeerClient) DisablePublishingPeerStatus() {}

func (m *mockSyncPeerClient) EnablePublishingPeerStatus() {}

func (m *mockSyncPeerClient) Start() error {
	return nil
}

func (m *mockSyncPeerClient) Close() {}

func (m *mockSyncPeerClient) GetPeerStatus(id peer.ID) (*NoForkPeer, error) {
	return m.getPeerStatusHandler(id)
}

func (m *mockSyncPeerClient) GetConnectedPeerStatuses() []*NoForkPeer {
	return m.getConnectedPeerStatusesHandler()
}

/* START ALPHA CHANGE 2813 -- capture mock ownership before delayed query handlers -- */
func (m *mockSyncPeerClient) GetPeerStatusUpdate(id peer.ID) (*PeerStatusUpdate, error) {
	if m.statusSession == nil {
		return nil, fmt.Errorf("status mock has no ownership provider")
	}

	session := m.statusSession(id)
	if session == nil {
		return nil, fmt.Errorf("status mock has no eligible session for %s", id)
	}

	status, err := m.GetPeerStatus(id)
	if err != nil {
		return nil, err
	}

	return &PeerStatusUpdate{Status: status, Session: session}, nil
}

func (m *mockSyncPeerClient) GetConnectedPeerStatusUpdates() []*PeerStatusUpdate {
	if m.getConnectedPeerStatusUpdatesHandler != nil {
		return m.getConnectedPeerStatusUpdatesHandler()
	}

	if m.getConnectedPeerStatusesHandler == nil || m.statusSession == nil {
		return nil
	}

	/* These legacy mock handlers provide immediate fixture values, not RPCs. */
	statuses := m.getConnectedPeerStatusesHandler()
	updates := make([]*PeerStatusUpdate, 0, len(statuses))
	for _, status := range statuses {
		if status != nil {
			updates = append(updates, &PeerStatusUpdate{
				Status:  status,
				Session: m.statusSession(status.ID),
			})
		}
	}

	return updates
}

/* FINISH ALPHA CHANGE 2813 -- capture mock ownership before delayed query handlers -- */

func (m *mockSyncPeerClient) GetBlocks(
	id peer.ID,
	start uint64,
	timeoutPerBlock time.Duration,
) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
	return m.getBlocksHandler(id, start, timeoutPerBlock)
}

/* START ALPHA CHANGE 2814 -- return owned envelopes from the mock status channel -- */
func (m *mockSyncPeerClient) GetPeerStatusUpdateCh() <-chan *PeerStatusUpdate {
	return m.getPeerStatusUpdateChHandler()
}

/* FINISH ALPHA CHANGE 2814 -- return owned envelopes from the mock status channel -- */

func (m *mockSyncPeerClient) GetPeerConnectionUpdateEventCh() <-chan *event.PeerEvent {
	return m.getPeerConnectionUpdateEventChHandler()
}

func (m *mockSyncPeerClient) CloseStream(peerID peer.ID) error {
	return nil
}

func GetAllElementsFromPeerMap(t *testing.T, p *PeerMap) []*NoForkPeer {
	t.Helper()

	peers := make([]*NoForkPeer, 0, 3)

	p.Range(func(key, value interface{}) bool {
		peer, ok := value.(*NoForkPeer)
		assert.True(t, ok)

		peers = append(peers, peer)

		return true
	})

	return peers
}

func sortPeerStatuses(peerStatuses []*NoForkPeer) []*NoForkPeer {
	sort.Slice(peerStatuses, func(p, q int) bool {
		return peerStatuses[p].Number < peerStatuses[q].Number
	})

	return peerStatuses
}

func NewTestSyncer(
	network Network,
	blockchain Blockchain,
	blockTimeout time.Duration,
	mockSyncPeerClient *mockSyncPeerClient,
	mockProgression Progression,
) *syncer {
	/* START ALPHA CHANGE 2815 -- initialize explicit ownership for isolated sync fixtures -- */
	if network == nil {
		network = &testStatusNetwork{}
	}
	if mockSyncPeerClient.statusSession == nil {
		mockSyncPeerClient.statusSession = network.PeerSession
	}
	/* FINISH ALPHA CHANGE 2815 -- initialize explicit ownership for isolated sync fixtures -- */

	return &syncer{
		/* START ALPHA CHANGE 2816 -- retain the fixture ownership provider for publication -- */
		network: network,
		/* FINISH ALPHA CHANGE 2816 -- retain the fixture ownership provider for publication -- */
		logger:          hclog.NewNullLogger(),
		blockchain:      blockchain,
		syncProgression: mockProgression,
		syncPeerService: &mockSyncPeerService{},
		syncPeerClient:  mockSyncPeerClient,
		blockTimeout:    blockTimeout,
		/* START ALPHA CHANGE 1678 -- initialize test retry interval -- */
		peerRetryInterval: defaultPeerRetryInterval,
		/* FINISH ALPHA CHANGE 1678 -- initialize test retry interval -- */
		newStatusCh: make(chan struct{}),
		/* START ALPHA CHANGE 1089 -- initialize test syncer shutdown channel -- */
		closeCh: make(chan struct{}),
		/* FINISH ALPHA CHANGE 1089 -- initialize test syncer shutdown channel -- */
		peerMap: new(PeerMap),
	}
}

var (
	peerStatuses = []*NoForkPeer{
		{
			ID:       peer.ID("A"),
			Number:   10,
			Distance: big.NewInt(10),
		},
		{
			ID:       peer.ID("B"),
			Number:   20,
			Distance: big.NewInt(20),
		},
		{
			ID:       peer.ID("C"),
			Number:   30,
			Distance: big.NewInt(30),
		},
	}
)

func Test_initializePeerMap(t *testing.T) {
	t.Parallel()

	syncer := NewTestSyncer(
		nil,
		nil,
		0,
		&mockSyncPeerClient{
			getConnectedPeerStatusesHandler: func() []*NoForkPeer {
				return peerStatuses
			},
			/* START ALPHA CHANGE 2817 -- use the owned channel type in the startup fixture -- */
			getPeerStatusUpdateChHandler: func() <-chan *PeerStatusUpdate {
				return nil
			},
			/* FINISH ALPHA CHANGE 2817 -- use the owned channel type in the startup fixture -- */
		},
		&mockProgression{},
	)

	syncer.initializePeerMap()

	peerMapStatuses := sortPeerStatuses(
		GetAllElementsFromPeerMap(t, syncer.peerMap),
	)

	assert.Equal(t, peerStatuses, peerMapStatuses)
}

func Test_startPeerStatusUpdateProcess(t *testing.T) {
	t.Parallel()

	/* START ALPHA CHANGE 2818 -- capture queued fixture ownership at its source -- */
	statusNetwork := &testStatusNetwork{}
	/* FINISH ALPHA CHANGE 2818 -- capture queued fixture ownership at its source -- */

	syncer := NewTestSyncer(
		statusNetwork,
		nil,
		0,
		&mockSyncPeerClient{
			getConnectedPeerStatusesHandler: func() []*NoForkPeer {
				return nil
			},
			/* START ALPHA CHANGE 2819 -- queue owned fixture envelopes instead of bare values -- */
			getPeerStatusUpdateChHandler: func() <-chan *PeerStatusUpdate {
				ch := make(chan *PeerStatusUpdate, len(peerStatuses))

				for _, status := range peerStatuses {
					ch <- &PeerStatusUpdate{
						Status:  status,
						Session: statusNetwork.PeerSession(status.ID),
					}
				}

				close(ch)

				return ch
			},
			/* FINISH ALPHA CHANGE 2819 -- queue owned fixture envelopes instead of bare values -- */
		},
		&mockProgression{},
	)

	syncer.startPeerStatusUpdateProcess()

	peerMapStatuses := sortPeerStatuses(
		GetAllElementsFromPeerMap(t, syncer.peerMap),
	)

	assert.Equal(t, peerStatuses, peerMapStatuses)
}

func Test_startPeerDisconnectEventProcess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		events          []*event.PeerEvent
		statuses        map[peer.ID]*NoForkPeer
		expectedPeerMap []*NoForkPeer
	}{
		{
			name: "should add peer to PeerMap after PeerConnected",
			events: []*event.PeerEvent{
				{
					PeerID: peer.ID("A"),
					Type:   event.PeerConnected,
				},
				{
					PeerID: peer.ID("B"),
					Type:   event.PeerConnected,
				},
			},
			statuses: map[peer.ID]*NoForkPeer{
				peer.ID("A"): {
					ID:       peer.ID("A"),
					Number:   10,
					Distance: big.NewInt(10),
				},
				peer.ID("B"): {
					ID:       peer.ID("B"),
					Number:   20,
					Distance: big.NewInt(20),
				},
			},
			expectedPeerMap: []*NoForkPeer{
				{
					ID:       peer.ID("A"),
					Number:   10,
					Distance: big.NewInt(10),
				},
				{
					ID:       peer.ID("B"),
					Number:   20,
					Distance: big.NewInt(20),
				},
			},
		},
		{
			name: "should remove peer to PeerMap after PeerDisconnected",
			events: []*event.PeerEvent{
				{
					PeerID: peer.ID("A"),
					Type:   event.PeerConnected,
				},
				{
					PeerID: peer.ID("A"),
					Type:   event.PeerDisconnected,
				},
			},
			statuses: map[peer.ID]*NoForkPeer{
				peer.ID("A"): {
					ID:       peer.ID("A"),
					Number:   10,
					Distance: big.NewInt(10),
				},
			},
			expectedPeerMap: []*NoForkPeer{},
		},
		{
			name: "should happen nothing in case of PeerFailedToConnect, PeerDialCompleted, PeerAddedToDialQueue",
			events: []*event.PeerEvent{
				{
					PeerID: peer.ID("A"),
					Type:   event.PeerFailedToConnect,
				},
				{
					PeerID: peer.ID("B"),
					Type:   event.PeerDialCompleted,
				},
				{
					PeerID: peer.ID("C"),
					Type:   event.PeerAddedToDialQueue,
				},
			},
			statuses:        map[peer.ID]*NoForkPeer{},
			expectedPeerMap: []*NoForkPeer{},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			/* START ALPHA CHANGE 2844 -- represent connection session termination in event fixtures -- */
			statusNetwork := &testStatusNetwork{}
			syncer := NewTestSyncer(
				statusNetwork,
				nil,
				0,
				&mockSyncPeerClient{
					getPeerConnectionUpdateEventChHandler: func() <-chan *event.PeerEvent {
						ch := make(chan *event.PeerEvent, len(test.events))

						go func() {
							for _, e := range test.events {
								if e.Type == event.PeerDisconnected {
									statusNetwork.endSession(e.PeerID)
								}

								ch <- e

								/* Preserve the existing simulated event interval. */
								time.Sleep(500 * time.Millisecond)
							}

							close(ch)
						}()

						return ch
					},
					getPeerStatusHandler: func(i peer.ID) (*NoForkPeer, error) {
						status, ok := test.statuses[i]
						if !ok {
							return nil, fmt.Errorf("peer %s didn't return status", i)
						}

						return status, nil
					},
				},
				&mockProgression{},
			)
			/* FINISH ALPHA CHANGE 2844 -- represent connection session termination in event fixtures -- */

			syncer.startPeerConnectionEventProcess()

			peerMapStatuses := GetAllElementsFromPeerMap(t, syncer.peerMap)

			// no need to check order
			peerMapStatuses = sortPeerStatuses(peerMapStatuses)

			assert.Equal(t, test.expectedPeerMap, peerMapStatuses)
		})
	}
}

func TestHasSyncPeer(t *testing.T) {
	t.Parallel()

	peerStatuses := []*NoForkPeer{
		{
			ID:       peer.ID("A"),
			Number:   10,
			Distance: big.NewInt(10),
		},
		{
			ID:       peer.ID("B"),
			Number:   20,
			Distance: big.NewInt(20),
		},
	}

	tests := []struct {
		name        string
		localLatest uint64
		peers       []*NoForkPeer
		result      bool
	}{
		{
			name:        "should return true when peerMap has elements",
			localLatest: 0,
			peers:       peerStatuses,
			result:      true,
		},
		{
			name:        "should return false when peerMap is empty",
			localLatest: 0,
			peers:       nil,
			result:      false,
		},
		{
			name:        "should return false when local latest is greater than any peers in peerMap",
			localLatest: 30,
			peers:       peerStatuses,
			result:      false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			syncer := NewTestSyncer(
				nil,
				&mockBlockchain{
					headerHandler: newSimpleHeaderHandler(test.localLatest),
				},
				0,
				&mockSyncPeerClient{},
				&mockProgression{},
			)

			syncer.peerMap.Put(test.peers...)

			assert.Equal(t, test.result, syncer.HasSyncPeer())
		})
	}
}

/* START ALPHA CHANGE 1350 -- model explicit sync block stream terminal results in tests -- */
func blocksToStream(
	blocks []*types.Block,
	delay time.Duration,
) (<-chan *types.Block, <-chan BlockStreamTerminal) {
	return blocksToTerminalStream(blocks, delay, nil)
}

func blocksToTerminalStream(
	blocks []*types.Block,
	delay time.Duration,
	terminalErr error,
) (<-chan *types.Block, <-chan BlockStreamTerminal) {
	blockCh := make(chan *types.Block)
	terminalCh := make(chan BlockStreamTerminal, 1)

	go func() {
		defer close(blockCh)
		defer close(terminalCh)

		for _, block := range blocks {
			time.Sleep(delay)

			blockCh <- block
		}

		terminalCh <- BlockStreamTerminal{Err: terminalErr}
	}()

	return blockCh, terminalCh
}

/* FINISH ALPHA CHANGE 1350 -- model explicit sync block stream terminal results in tests -- */

func createMockBlocks(num int) []*types.Block {
	blocks := make([]*types.Block, num)
	for i := 0; i < num; i++ {
		blocks[i] = &types.Block{
			Header: &types.Header{
				Number: uint64(i + 1),
			},
		}
	}

	return blocks
}

func TestSync(t *testing.T) {
	t.Parallel()

	blocks := createMockBlocks(10)

	tests := []struct {
		name string

		// local
		beginningHeight     uint64
		createBlockCallback func() func(*types.FullBlock) bool

		// peers
		peerStatuses []*NoForkPeer

		peerBlocks     map[peer.ID][]*types.Block
		newStatusDelay time.Duration

		// handlers
		// a function to return a callback to use closure
		createVerifyFinalizedBlockHandler func() func(*types.Block) (*types.FullBlock, error)

		// results
		blocks             []*types.Block
		progressionStart   uint64
		progressionHighest uint64
		err                error
	}{
		{
			name:            "should sync blocks to the latest successfully",
			beginningHeight: 0,
			createBlockCallback: func() func(*types.FullBlock) bool {
				return func(b *types.FullBlock) bool {
					return b.Block.Number() >= 10
				}
			},
			peerStatuses: []*NoForkPeer{
				{
					ID:       peer.ID("A"),
					Number:   10,
					Distance: big.NewInt(0),
				},
			},
			newStatusDelay: 0,
			peerBlocks: map[peer.ID][]*types.Block{
				peer.ID("A"): blocks[:10],
			},
			createVerifyFinalizedBlockHandler: func() func(*types.Block) (*types.FullBlock, error) {
				return func(b *types.Block) (*types.FullBlock, error) {
					return &types.FullBlock{Block: b}, nil
				}
			},
			blocks:             blocks[:10],
			progressionStart:   1,
			progressionHighest: 10,
			err:                nil,
		},
		{
			name:            "should sync blocks with multiple peers",
			beginningHeight: 0,
			createBlockCallback: func() func(*types.FullBlock) bool {
				return func(b *types.FullBlock) bool {
					return b.Block.Number() >= 10
				}
			},
			peerStatuses: []*NoForkPeer{
				{
					ID:       peer.ID("A"),
					Number:   10,
					Distance: big.NewInt(0),
				},
				{
					ID:       peer.ID("B"),
					Number:   10,
					Distance: big.NewInt(1),
				},
			},
			newStatusDelay: 0,
			peerBlocks: map[peer.ID][]*types.Block{
				peer.ID("A"): blocks[:5],
				peer.ID("B"): blocks[4:10],
			},
			createVerifyFinalizedBlockHandler: func() func(*types.Block) (*types.FullBlock, error) {
				count := 0

				return func(b *types.Block) (*types.FullBlock, error) {
					if b.Number() == 5 {
						count++

						if count == 1 {
							return nil, errors.New("block verification failed")
						}
					}

					return &types.FullBlock{Block: b}, nil
				}
			},
			blocks: blocks[:10],
			/* START ALPHA CHANGE 3836 -- expect progression to start at the resumed sync batch -- */
			progressionStart: 5,
			/* FINISH ALPHA CHANGE 3836 -- expect progression to start at the resumed sync batch -- */
			progressionHighest: 10,
			err:                nil,
		},
	}

	// ALPHA CHANGE 3837 -- remove redundant copy of the per-iteration range variable.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var (
				syncedBlocks      = make([]*types.Block, 0, len(test.blocks))
				latestBlockNumber = test.beginningHeight
				progression       = &mockProgression{}

				syncer = NewTestSyncer(
					nil,
					&mockBlockchain{
						/* START ALPHA CHANGE 3834 -- expose imported head advancement in multi-peer sync tests -- */
						headerHandler: func() *types.Header {
							return &types.Header{Number: latestBlockNumber}
						},
						/* FINISH ALPHA CHANGE 3834 -- expose imported head advancement in multi-peer sync tests -- */
						verifyFinalizedBlockHandler: test.createVerifyFinalizedBlockHandler(),
						writeFullBlockHandler: func(b *types.FullBlock) error {
							syncedBlocks = append(syncedBlocks, b.Block)
							latestBlockNumber = b.Block.Number()

							return nil
						},
					},
					time.Second,
					&mockSyncPeerClient{
						getBlocksHandler: func(
							i peer.ID,
							u uint64,
							_ time.Duration,
						) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
							// should not panic
							peerBlocks := test.peerBlocks[i]
							blockCh, terminalCh := blocksToStream(peerBlocks, 0)

							return blockCh, terminalCh, nil
						},
					},
					progression,
				)
			)

			/* START ALPHA CHANGE 1083 -- seed sync test peers before immediate peer-map evaluation -- */
			for _, p := range test.peerStatuses {
				syncer.peerMap.Put(p)
			}
			/* FINISH ALPHA CHANGE 1083 -- seed sync test peers before immediate peer-map evaluation -- */

			errCh := make(chan error, 1)

			go func() {
				errCh <- syncer.Sync(test.createBlockCallback())
			}()

			err := <-errCh

			assert.Equal(t, test.blocks, syncedBlocks)
			assert.Equal(t, test.progressionStart, progression.startingBlock)
			assert.Equal(t, test.progressionHighest, progression.highestBlock)
			assert.ErrorIs(t, err, test.err)
		})
	}
}

/* START ALPHA CHANGE 1081 -- verify sync uses initialized peer map without waiting for a fresh status event -- */
func TestSync_UsesInitializedPeerMapImmediately(t *testing.T) {
	t.Parallel()

	blocks := createMockBlocks(1)
	syncedBlocks := make([]*types.Block, 0, 1)
	latestBlockNumber := uint64(0)

	syncer := NewTestSyncer(
		nil,
		&mockBlockchain{
			headerHandler: newSimpleHeaderHandler(latestBlockNumber),
			verifyFinalizedBlockHandler: func(b *types.Block) (*types.FullBlock, error) {
				return &types.FullBlock{Block: b}, nil
			},
			writeFullBlockHandler: func(b *types.FullBlock) error {
				syncedBlocks = append(syncedBlocks, b.Block)
				latestBlockNumber = b.Block.Number()

				return nil
			},
		},
		time.Second,
		&mockSyncPeerClient{
			getBlocksHandler: func(
				i peer.ID,
				u uint64,
				_ time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				assert.Equal(t, peer.ID("A"), i)
				assert.Equal(t, uint64(1), u)

				blockCh, terminalCh := blocksToStream(blocks, 0)

				return blockCh, terminalCh, nil
			},
		},
		&mockProgression{},
	)

	syncer.peerMap.Put(&NoForkPeer{
		ID:       peer.ID("A"),
		Number:   1,
		Distance: big.NewInt(0),
	})

	err := syncer.Sync(func(b *types.FullBlock) bool {
		return b.Block.Number() >= 1
	})

	assert.NoError(t, err)
	assert.Equal(t, blocks, syncedBlocks)
}

/* START ALPHA CHANGE 1679 -- retry exhausted peers without requiring new status -- */
func TestSyncRetriesExhaustedPeersAfterBoundedWait(t *testing.T) {
	t.Parallel()

	blocks := createMockBlocks(1)
	latestBlockNumber := uint64(0)
	attempts := 0

	syncer := NewTestSyncer(
		nil,
		&mockBlockchain{
			headerHandler: func() *types.Header {
				return &types.Header{Number: latestBlockNumber}
			},
			verifyFinalizedBlockHandler: func(block *types.Block) (*types.FullBlock, error) {
				return &types.FullBlock{Block: block}, nil
			},
			writeFullBlockHandler: func(block *types.FullBlock) error {
				latestBlockNumber = block.Block.Number()

				return nil
			},
		},
		time.Second,
		&mockSyncPeerClient{
			getBlocksHandler: func(
				id peer.ID,
				start uint64,
				_ time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				attempts++
				if attempts == 1 {
					blockCh, terminalCh := blocksToTerminalStream(
						nil,
						0,
						errors.New("injected stream failure"),
					)

					return blockCh, terminalCh, nil
				}

				blockCh, terminalCh := blocksToStream(blocks, 0)

				return blockCh, terminalCh, nil
			},
		},
		&mockProgression{},
	)
	syncer.peerRetryInterval = 10 * time.Millisecond

	syncer.peerMap.Put(&NoForkPeer{
		ID:          peer.ID("A"),
		Number:      1,
		HistoryFrom: 0,
		Distance:    big.NewInt(0),
	})

	err := syncer.Sync(func(block *types.FullBlock) bool {
		return block.Block.Number() == 1
	})

	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.Equal(t, uint64(1), latestBlockNumber)
}

/* FINISH ALPHA CHANGE 1679 -- retry exhausted peers without requiring new status -- */

/* FINISH ALPHA CHANGE 1081 -- verify sync uses initialized peer map without waiting for a fresh status event -- */

/* START ALPHA CHANGE 1468 -- test immediate bulk sync callback termination -- */
func TestBulkSyncWithPeerStopsImmediatelyWhenCallbackRequestsTermination(t *testing.T) {
	t.Parallel()

	blocks := createMockBlocks(2)

	blockCh := make(chan *types.Block, len(blocks))
	for _, block := range blocks {
		blockCh <- block
	}
	close(blockCh)

	// This channel intentionally remains open and unreadable. The callback must
	// terminate bulk sync without waiting for the stream terminal result.
	terminalCh := make(chan BlockStreamTerminal)

	writtenBlocks := make([]*types.Block, 0, 1)
	latestBlockNumber := uint64(0)

	syncer := NewTestSyncer(
		nil,
		&mockBlockchain{
			headerHandler: func() *types.Header {
				return &types.Header{Number: latestBlockNumber}
			},
			verifyFinalizedBlockHandler: func(block *types.Block) (*types.FullBlock, error) {
				return &types.FullBlock{Block: block}, nil
			},
			writeFullBlockHandler: func(fullBlock *types.FullBlock) error {
				writtenBlocks = append(writtenBlocks, fullBlock.Block)
				latestBlockNumber = fullBlock.Block.Number()

				return nil
			},
		},
		time.Second,
		&mockSyncPeerClient{
			getBlocksHandler: func(
				peer.ID,
				uint64,
				time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				return blockCh, terminalCh, nil
			},
		},
		&mockProgression{},
	)

	lastNumber, shouldTerminate, err := syncer.bulkSyncWithPeer(
		peer.ID("peer"),
		2,
		func(*types.FullBlock) bool {
			return true
		},
	)

	require.NoError(t, err)
	require.True(t, shouldTerminate)
	require.Equal(t, uint64(1), lastNumber)
	require.Equal(t, blocks[:1], writtenBlocks)
}

/* FINISH ALPHA CHANGE 1468 -- test immediate bulk sync callback termination -- */

/* START ALPHA CHANGE 1069 -- verify syncer close is idempotent -- */
func TestSyncer_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	syncer := NewTestSyncer(
		nil,
		&mockBlockchain{
			headerHandler: newSimpleHeaderHandler(0),
		},
		time.Second,
		&mockSyncPeerClient{},
		&mockProgression{},
	)

	assert.NoError(t, syncer.Close())
	assert.NoError(t, syncer.Close())
}

/* FINISH ALPHA CHANGE 1069 -- verify syncer close is idempotent -- */

/* START ALPHA CHANGE 1076 -- verify sync exits when syncer closes -- */
func TestSync_ReturnsWhenSyncerCloses(t *testing.T) {
	t.Parallel()

	syncer := NewTestSyncer(
		nil,
		&mockBlockchain{
			headerHandler: newSimpleHeaderHandler(0),
		},
		time.Second,
		&mockSyncPeerClient{},
		&mockProgression{},
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- syncer.Sync(func(*types.FullBlock) bool {
			return false
		})
	}()

	assert.NoError(t, syncer.Close())

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("sync did not return after syncer close")
	}
}

/* FINISH ALPHA CHANGE 1076 -- verify sync exits when syncer closes -- */

/* START ALPHA CHANGE 1090 -- verify status notifications after close do not panic -- */
func TestSyncer_NotifyNewStatusAfterCloseDoesNotPanic(t *testing.T) {
	t.Parallel()

	syncer := NewTestSyncer(
		nil,
		&mockBlockchain{
			headerHandler: newSimpleHeaderHandler(0),
		},
		time.Second,
		&mockSyncPeerClient{},
		&mockProgression{},
	)

	assert.NoError(t, syncer.Close())
	assert.NotPanics(t, syncer.notifyNewStatusEvent)
}

/* FINISH ALPHA CHANGE 1090 -- verify status notifications after close do not panic -- */

func Test_bulkSyncWithPeer(t *testing.T) {
	t.Parallel()

	blockNum := 30
	blocks := make([]*types.Block, blockNum) // 1 to 30

	for i := 0; i < blockNum; i++ {
		blocks[i] = &types.Block{
			Header: &types.Header{
				Number: uint64(i + 1),
			},
		}
	}

	var (
		// mock errors
		errPeerNoResponse       = errors.New("peer is not responding")
		errInvalidBlock         = errors.New("invalid block")
		errBlockInsertionFailed = errors.New("failed to insert block")
	)

	tests := []struct {
		name string

		// local
		beginningHeight uint64
		blockTimeout    time.Duration
		blockCallback   func(*types.FullBlock) bool

		// peers
		getBlocksHandler func(
			id peer.ID,
			start uint64,
			timeoutPerBlock time.Duration,
		) (<-chan *types.Block, <-chan BlockStreamTerminal, error)
		peerLatestBlock uint64

		// handlers
		verifyFinalizedBlockHandler func(*types.Block) (*types.FullBlock, error)
		writeFullBlockHandler       func(*types.FullBlock) error

		// results
		blocks                []*types.Block
		lastSyncedBlockNumber uint64
		shouldTerminate       bool
		err                   error
	}{
		{
			name:            "should sync blocks to the latest successfully",
			beginningHeight: 0,
			blockTimeout:    time.Second,
			blockCallback: func(b *types.FullBlock) bool {
				return false
			},
			getBlocksHandler: func(
				id peer.ID,
				start uint64,
				_ time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				blockCh, terminalCh := blocksToStream(blocks[:10], 0)

				return blockCh, terminalCh, nil
			},
			peerLatestBlock: 10,
			verifyFinalizedBlockHandler: func(b *types.Block) (*types.FullBlock, error) {
				return &types.FullBlock{Block: b}, nil
			},
			writeFullBlockHandler: func(b *types.FullBlock) error {
				return nil
			},
			blocks:                blocks[:10],
			lastSyncedBlockNumber: 10,
			shouldTerminate:       false,
			err:                   nil,
		},
		{
			name:            "should return error if GetBlocks returns error",
			beginningHeight: 0,
			blockTimeout:    time.Second,
			blockCallback: func(b *types.FullBlock) bool {
				return false
			},
			getBlocksHandler: func(
				id peer.ID,
				start uint64,
				_ time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				return nil, nil, errPeerNoResponse
			},
			peerLatestBlock: 10,
			verifyFinalizedBlockHandler: func(b *types.Block) (*types.FullBlock, error) {
				return &types.FullBlock{Block: b}, nil
			},
			writeFullBlockHandler: func(b *types.FullBlock) error {
				return nil
			},
			blocks:                []*types.Block{},
			lastSyncedBlockNumber: 0,
			shouldTerminate:       false,
			err:                   errPeerNoResponse,
		},
		{
			name:            "should return error if verification is failed",
			beginningHeight: 0,
			blockTimeout:    time.Second,
			blockCallback: func(b *types.FullBlock) bool {
				return false
			},
			getBlocksHandler: func(
				id peer.ID,
				start uint64,
				_ time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				blockCh, terminalCh := blocksToStream(blocks[:6], 0)

				return blockCh, terminalCh, nil
			},
			peerLatestBlock: 10,
			verifyFinalizedBlockHandler: func(b *types.Block) (*types.FullBlock, error) {
				if b.Number() > 5 {
					return nil, errInvalidBlock
				}

				return &types.FullBlock{Block: b}, nil
			},
			writeFullBlockHandler: func(b *types.FullBlock) error {
				return nil
			},
			blocks:                blocks[:5],
			lastSyncedBlockNumber: 5,
			shouldTerminate:       false,
			err:                   errInvalidBlock,
		},
		{
			name:            "should return error if block insertion is failed",
			beginningHeight: 0,
			blockTimeout:    time.Second,
			blockCallback: func(b *types.FullBlock) bool {
				return false
			},
			getBlocksHandler: func(
				id peer.ID,
				start uint64,
				_ time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				blockCh, terminalCh := blocksToStream(blocks[:6], 0)

				return blockCh, terminalCh, nil
			},
			peerLatestBlock: 10,
			verifyFinalizedBlockHandler: func(b *types.Block) (*types.FullBlock, error) {
				return &types.FullBlock{Block: b}, nil
			},
			writeFullBlockHandler: func(b *types.FullBlock) error {
				if b.Block.Number() > 5 {
					return errBlockInsertionFailed
				}

				return nil
			},
			blocks:                blocks[:5],
			lastSyncedBlockNumber: 5,
			shouldTerminate:       false,
			err:                   errBlockInsertionFailed,
		},
		{
			name:            "should return error in case of timeout",
			beginningHeight: 0,
			blockTimeout:    500 * time.Millisecond,
			blockCallback: func(b *types.FullBlock) bool {
				return false
			},
			getBlocksHandler: func(
				id peer.ID,
				start uint64,
				_ time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				blockCh, terminalCh := blocksToTerminalStream(nil, 0, errTimeout)

				return blockCh, terminalCh, nil
			},
			peerLatestBlock: 10,
			verifyFinalizedBlockHandler: func(b *types.Block) (*types.FullBlock, error) {
				return &types.FullBlock{Block: b}, nil
			},
			writeFullBlockHandler: func(b *types.FullBlock) error {
				return nil
			},
			blocks:                []*types.Block{},
			lastSyncedBlockNumber: 0,
			shouldTerminate:       false,
			err:                   errTimeout,
		},
		/* START ALPHA CHANGE 1351 -- test explicit sync stream terminal failures -- */
		{
			name:            "should return error if peer ends stream before advertised head",
			beginningHeight: 0,
			blockTimeout:    time.Second,
			blockCallback: func(b *types.FullBlock) bool {
				return false
			},
			getBlocksHandler: func(
				id peer.ID,
				start uint64,
				_ time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				blockCh, terminalCh := blocksToStream(blocks[:5], 0)

				return blockCh, terminalCh, nil
			},
			peerLatestBlock: 10,
			verifyFinalizedBlockHandler: func(b *types.Block) (*types.FullBlock, error) {
				return &types.FullBlock{Block: b}, nil
			},
			writeFullBlockHandler: func(b *types.FullBlock) error {
				return nil
			},
			blocks:                blocks[:5],
			lastSyncedBlockNumber: 5,
			shouldTerminate:       false,
			err:                   errors.New("ended block stream before advertised head"),
		},
		{
			name:            "should return not-found terminal error before any block",
			beginningHeight: 0,
			blockTimeout:    time.Second,
			blockCallback: func(b *types.FullBlock) bool {
				return false
			},
			getBlocksHandler: func(
				id peer.ID,
				start uint64,
				_ time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				blockCh, terminalCh := blocksToTerminalStream(nil, 0, ErrBlockNotFound)

				return blockCh, terminalCh, nil
			},
			peerLatestBlock: 10,
			verifyFinalizedBlockHandler: func(b *types.Block) (*types.FullBlock, error) {
				return &types.FullBlock{Block: b}, nil
			},
			writeFullBlockHandler: func(b *types.FullBlock) error {
				return nil
			},
			blocks:                []*types.Block{},
			lastSyncedBlockNumber: 0,
			shouldTerminate:       false,
			err:                   ErrBlockNotFound,
		},
		{
			name:            "should return not-found terminal error after partial delivery",
			beginningHeight: 0,
			blockTimeout:    time.Second,
			blockCallback: func(b *types.FullBlock) bool {
				return false
			},
			getBlocksHandler: func(
				id peer.ID,
				start uint64,
				_ time.Duration,
			) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
				blockCh, terminalCh := blocksToTerminalStream(blocks[:5], 0, ErrBlockNotFound)

				return blockCh, terminalCh, nil
			},
			peerLatestBlock: 10,
			verifyFinalizedBlockHandler: func(b *types.Block) (*types.FullBlock, error) {
				return &types.FullBlock{Block: b}, nil
			},
			writeFullBlockHandler: func(b *types.FullBlock) error {
				return nil
			},
			blocks:                blocks[:5],
			lastSyncedBlockNumber: 5,
			shouldTerminate:       false,
			err:                   ErrBlockNotFound,
		},
		/* FINISH ALPHA CHANGE 1351 -- test explicit sync stream terminal failures -- */
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var (
				syncedBlocks = make([]*types.Block, 0, len(test.blocks))

				syncer = NewTestSyncer(
					nil,
					&mockBlockchain{
						headerHandler:               newSimpleHeaderHandler(test.beginningHeight),
						verifyFinalizedBlockHandler: test.verifyFinalizedBlockHandler,
						writeFullBlockHandler: func(b *types.FullBlock) error {
							if err := test.writeFullBlockHandler(b); err != nil {
								return err
							}

							syncedBlocks = append(syncedBlocks, b.Block)

							return nil
						},
					},
					test.blockTimeout,
					&mockSyncPeerClient{
						getBlocksHandler: test.getBlocksHandler,
					},
					&mockProgression{},
				)
			)

			lastSynced, shouldTerminate, err := syncer.bulkSyncWithPeer(
				peer.ID("X"),
				test.peerLatestBlock,
				test.blockCallback,
			)

			assert.Equal(t, test.lastSyncedBlockNumber, lastSynced)
			assert.Equal(t, test.shouldTerminate, shouldTerminate)

			if test.err == nil {
				assert.NoError(t, err)
			} else if errors.Is(err, test.err) {
				assert.ErrorIs(t, err, test.err)
			} else {
				assert.ErrorContains(t, err, test.err.Error())
			}

			assert.Equal(t, test.blocks, syncedBlocks)
		})
	}
}
