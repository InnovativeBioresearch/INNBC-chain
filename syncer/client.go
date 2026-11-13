package syncer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/armon/go-metrics"
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/network"
	"github.com/innovativebioresearch/innbc-chain/network/event"

	/* START ALPHA CHANGE 2706 -- import sync operation lifetime support -- */
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	/* FINISH ALPHA CHANGE 2706 -- import sync operation lifetime support -- */
	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	SyncPeerClientLoggerName = "sync-peer-client"
	statusTopicName          = "syncer/status/0.1"
	defaultTimeoutForStatus  = 10 * time.Second
)

/* START ALPHA CHANGE 1346 -- track local sync block stream cancellation -- */
type blockStreamCancelState struct {
	cancel context.CancelFunc
}

/* FINISH ALPHA CHANGE 1346 -- track local sync block stream cancellation -- */

type syncPeerClient struct {
	logger     hclog.Logger // logger used for console logging
	network    Network      // reference to the network module
	blockchain Blockchain   // reference to the blockchain module

	/* START ALPHA CHANGE 1380 -- track sync peer client worker shutdown -- */
	workerLock sync.Mutex
	workerWG   sync.WaitGroup
	/* START ALPHA CHANGE 2707 -- retain sync RPC admission and joined close ownership -- */
	rpcLifetime netgrpc.RPCLifetime
	closeOnce   sync.Once
	/* FINISH ALPHA CHANGE 2707 -- retain sync RPC admission and joined close ownership -- */
	/* FINISH ALPHA CHANGE 1380 -- track sync peer client worker shutdown -- */

	/* START ALPHA CHANGE 2801 -- carry owned status envelopes through client delivery -- */
	topic                  *network.Topic         // reference to the network topic
	id                     string                 // node id
	peerStatusUpdateCh     chan *PeerStatusUpdate // peer status update channel
	peerConnectionUpdateCh chan *event.PeerEvent  // peer connection update channel
	/* FINISH ALPHA CHANGE 2801 -- carry owned status envelopes through client delivery -- */

	shouldEmitBlocks bool // flag for emitting blocks in the topic
	closeCh          chan struct{}
	closed           atomic.Bool

	peerStatusUpdateChLock   sync.Mutex
	peerStatusUpdateChClosed bool

	/* START ALPHA CHANGE 1346 -- track local sync block stream cancellation -- */
	blockStreamLock    sync.Mutex
	blockStreamCancels map[peer.ID]*blockStreamCancelState
	/* FINISH ALPHA CHANGE 1346 -- track local sync block stream cancellation -- */
}

func NewSyncPeerClient(
	logger hclog.Logger,
	network Network,
	blockchain Blockchain,
) SyncPeerClient {
	return &syncPeerClient{
		logger:     logger.Named(SyncPeerClientLoggerName),
		network:    network,
		blockchain: blockchain,
		id:         network.AddrInfo().ID.String(),
		/* START ALPHA CHANGE 2802 -- initialize bounded owned status delivery -- */
		peerStatusUpdateCh: make(chan *PeerStatusUpdate, 1),
		/* FINISH ALPHA CHANGE 2802 -- initialize bounded owned status delivery -- */
		peerConnectionUpdateCh: make(chan *event.PeerEvent, 1),
		shouldEmitBlocks:       true,
		closeCh:                make(chan struct{}),

		peerStatusUpdateChLock:   sync.Mutex{},
		peerStatusUpdateChClosed: false,

		/* START ALPHA CHANGE 1346 -- track local sync block stream cancellation -- */
		blockStreamCancels: make(map[peer.ID]*blockStreamCancelState),
		/* FINISH ALPHA CHANGE 1346 -- track local sync block stream cancellation -- */
	}
}

/* START ALPHA CHANGE 2708 -- join startup without reopening closed sync admission -- */
func (m *syncPeerClient) Start() error {
	_, finish, err := m.rpcLifetime.Begin(context.Background())
	if err != nil {
		return err
	}
	defer finish()

	if err := m.startGossip(); err != nil {
		return err
	}

	if !m.startWorker(m.startNewBlockProcess) {
		/* Close owns cleanup after this admitted startup returns. */
		return errors.New("sync peer client closed before block worker startup")
	}

	if !m.startWorker(m.startPeerEventProcess) {
		return errors.New("sync peer client closed before peer event worker startup")
	}

	return nil
}

/* FINISH ALPHA CHANGE 2708 -- join startup without reopening closed sync admission -- */

/* START ALPHA CHANGE 2709 -- cancel admitted sync operations before joining workers -- */
func (m *syncPeerClient) Close() {
	m.closeOnce.Do(func() {
		m.workerLock.Lock()
		m.closed.Store(true)
		m.workerLock.Unlock()

		if m.closeCh != nil {
			close(m.closeCh)
		}

		m.rpcLifetime.Close()
		m.workerWG.Wait()

		if m.topic != nil {
			m.topic.Close()
			m.topic = nil
		}

		m.peerStatusUpdateChLock.Lock()
		if !m.peerStatusUpdateChClosed {
			m.peerStatusUpdateChClosed = true
			close(m.peerStatusUpdateCh)
		}
		m.peerStatusUpdateChLock.Unlock()
	})
}

/* FINISH ALPHA CHANGE 2709 -- cancel admitted sync operations before joining workers -- */

/* START ALPHA CHANGE 1383 -- register sync peer client workers safely -- */
func (m *syncPeerClient) startWorker(fn func()) bool {
	m.workerLock.Lock()
	defer m.workerLock.Unlock()

	if m.closed.Load() {
		return false
	}

	m.workerWG.Add(1)

	go func() {
		defer m.workerWG.Done()

		fn()
	}()

	return true
}

/* FINISH ALPHA CHANGE 1383 -- register sync peer client workers safely -- */

// DisablePublishingPeerStatus disables publishing own status via gossip
func (m *syncPeerClient) DisablePublishingPeerStatus() {
	m.shouldEmitBlocks = false
}

// EnablePublishingPeerStatus enables publishing own status via gossip
func (m *syncPeerClient) EnablePublishingPeerStatus() {
	m.shouldEmitBlocks = true
}

/* START ALPHA CHANGE 2803 -- capture query ownership before establishing or executing the status RPC -- */
func (m *syncPeerClient) GetPeerStatusUpdate(peerID peer.ID) (*PeerStatusUpdate, error) {
	session := m.network.PeerSession(peerID)
	if session == nil {
		return nil, fmt.Errorf("peer %s has no eligible status session", peerID)
	}

	status, err := m.GetPeerStatus(peerID)
	if err != nil {
		return nil, err
	}

	return &PeerStatusUpdate{
		Status:  status,
		Session: session,
	}, nil
}

/* FINISH ALPHA CHANGE 2803 -- capture query ownership before establishing or executing the status RPC -- */

// GetPeerStatus fetches peer status
func (m *syncPeerClient) GetPeerStatus(peerID peer.ID) (*NoForkPeer, error) {
	/* START ALPHA CHANGE 2710 -- cover status establishment and RPC with admitted cancellation and deadline -- */
	ctx, finish, err := m.rpcLifetime.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	defer finish()

	timeoutCtx, cancel := context.WithTimeout(ctx, defaultTimeoutForStatus)
	defer cancel()

	clt, closeClient, err := m.newSyncPeerClient(timeoutCtx, peerID)
	if err != nil {
		return nil, err
	}
	defer closeClient()

	status, err := clt.GetStatus(timeoutCtx, &emptypb.Empty{})
	/* FINISH ALPHA CHANGE 2710 -- cover status establishment and RPC with admitted cancellation and deadline -- */
	if err != nil {
		return nil, err
	}

	return &NoForkPeer{
		ID:     peerID,
		Number: status.Number,
		/* START ALPHA CHANGE 1665 -- retain directly queried peer history floor -- */
		HistoryFrom: status.HistoryFrom,
		/* FINISH ALPHA CHANGE 1665 -- retain directly queried peer history floor -- */
		Distance: m.network.GetPeerDistance(peerID),
	}, nil
}

/* START ALPHA CHANGE 2804 -- retain originating sessions through startup status collection -- */
// GetConnectedPeerStatuses fetches values without publication metadata.
func (m *syncPeerClient) GetConnectedPeerStatuses() []*NoForkPeer {
	updates := m.GetConnectedPeerStatusUpdates()
	statuses := make([]*NoForkPeer, 0, len(updates))
	for _, update := range updates {
		statuses = append(statuses, update.Status)
	}

	return statuses
}

func (m *syncPeerClient) GetConnectedPeerStatusUpdates() []*PeerStatusUpdate {
	var (
		ps            = m.network.Peers()
		syncPeers     = make([]*PeerStatusUpdate, 0, len(ps))
		syncPeersLock sync.Mutex
		wg            sync.WaitGroup
	)

	for _, p := range ps {
		p := p

		wg.Add(1)

		go func() {
			defer wg.Done()

			peerID := p.Info.ID
			update, err := m.GetPeerStatusUpdate(peerID)
			if err != nil {
				m.logger.Warn("failed to get status from a peer, skip", "id", peerID, "err", err)

				return
			}

			syncPeersLock.Lock()
			syncPeers = append(syncPeers, update)
			syncPeersLock.Unlock()
		}()
	}

	wg.Wait()

	return syncPeers
}

/* FINISH ALPHA CHANGE 2804 -- retain originating sessions through startup status collection -- */

/* START ALPHA CHANGE 2805 -- expose owned status envelopes to the syncer -- */
// GetPeerStatusUpdateCh returns a channel of peer's status update
func (m *syncPeerClient) GetPeerStatusUpdateCh() <-chan *PeerStatusUpdate {
	return m.peerStatusUpdateCh
}

/* FINISH ALPHA CHANGE 2805 -- expose owned status envelopes to the syncer -- */

// GetPeerConnectionUpdateEventCh returns peer's connection change event
func (m *syncPeerClient) GetPeerConnectionUpdateEventCh() <-chan *event.PeerEvent {
	return m.peerConnectionUpdateCh
}

// startGossip creates new topic and starts subscribing
/* START ALPHA CHANGE 2806 -- preserve validated status ownership through callback scheduling and delivery -- */
func (m *syncPeerClient) startGossip() error {
	topic, err := m.network.NewPeerSessionTopic(statusTopicName, &proto.SyncPeerStatus{})
	if err != nil {
		return err
	}

	/* START ALPHA CHANGE 2837 -- keep status delivery inside bounded topic workers -- */
	if err := topic.SubscribeWithPeerSession(m.handleStatusUpdate); err != nil {
		topic.Close()

		return fmt.Errorf("unable to subscribe to gossip topic, %w", err)
	}
	/* FINISH ALPHA CHANGE 2837 -- keep status delivery inside bounded topic workers -- */

	m.topic = topic

	return nil
}

func (m *syncPeerClient) handleStatusUpdate(
	obj any,
	from peer.ID,
	session *network.PeerSession,
) {
	status, ok := obj.(*proto.SyncPeerStatus)
	if !ok {
		m.logger.Error("failed to cast gossiped message to txn")

		return
	}

	if session == nil ||
		!m.network.IsPeerAllowed(from) ||
		!m.network.IsConnected(from) {
		if m.id != from.String() {
			m.logger.Debug("received status from unavailable peer, ignore", "id", from)
		}

		return
	}

	update := &PeerStatusUpdate{
		Status: &NoForkPeer{
			ID:          from,
			Number:      status.Number,
			HistoryFrom: status.HistoryFrom,
			Distance:    m.network.GetPeerDistance(from),
		},
		Session: session,
	}

	m.peerStatusUpdateChLock.Lock()
	defer m.peerStatusUpdateChLock.Unlock()

	if !m.peerStatusUpdateChClosed {
		select {
		case m.peerStatusUpdateCh <- update:
		case <-m.closeCh:
		}
	}
}

/* FINISH ALPHA CHANGE 2806 -- preserve validated status ownership through callback scheduling and delivery -- */

// startNewBlockProcess starts blockchain event subscription
func (m *syncPeerClient) startNewBlockProcess() {
	/* START ALPHA CHANGE 1386 -- keep blockchain subscription owned by worker -- */
	subscription := m.blockchain.SubscribeEvents()
	if subscription == nil {
		return
	}
	defer m.blockchain.UnsubscribeEvents(subscription)

	eventCh := subscription.GetEventCh()

	for {
		select {
		case <-m.closeCh:
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}

			if event == nil || !m.shouldEmitBlocks {
				continue
			}

			if l := len(event.NewChain); l > 0 {
				latest := event.NewChain[l-1]
				// Publish status
				if err := m.topic.Publish(&proto.SyncPeerStatus{
					Number: latest.Number,
					/* START ALPHA CHANGE 1667 -- advertise current full-block history floor -- */
					HistoryFrom: m.blockchain.HistoryFrom(),
					/* FINISH ALPHA CHANGE 1667 -- advertise current full-block history floor -- */
				}); err != nil {
					m.logger.Warn("failed to publish status", "err", err)
				}
			}
		}
	}
	/* FINISH ALPHA CHANGE 1386 -- keep blockchain subscription owned by worker -- */
}

// startPeerEventProcess starts subscribing peer connection change events and process them
func (m *syncPeerClient) startPeerEventProcess() {
	defer close(m.peerConnectionUpdateCh)

	/* START ALPHA CHANGE 2711 -- cancel the peer event subscription with its owning sync worker -- */
	ctx, finish, err := m.rpcLifetime.Begin(context.Background())
	if err != nil {
		return
	}
	defer finish()

	peerEventCh, err := m.network.SubscribeCh(ctx)
	/* FINISH ALPHA CHANGE 2711 -- cancel the peer event subscription with its owning sync worker -- */
	if err != nil {
		m.logger.Error("failed to subscribe", "err", err)

		return
	}

	/* START ALPHA CHANGE 1387 -- make peer connection event delivery shutdown-aware -- */
	for {
		select {
		case <-m.closeCh:
			return

		case e, ok := <-peerEventCh:
			if !ok {
				return
			}

			/* START ALPHA CHANGE 2752 -- reject stale membership events before forwarding -- */
			/* START ALPHA CHANGE 2880 -- forward session cleanup independently of handshake registration -- */
			if e.IsCurrent() &&
				(e.Type == event.PeerConnected ||
					e.Type == event.PeerDisconnected ||
					e.Type == event.PeerSessionEnded) {
				select {
				case m.peerConnectionUpdateCh <- e:
				case <-m.closeCh:
					return
				}
			}
			/* FINISH ALPHA CHANGE 2880 -- forward session cleanup independently of handshake registration -- */
			/* FINISH ALPHA CHANGE 2752 -- reject stale membership events before forwarding -- */
		}
	}
	/* FINISH ALPHA CHANGE 1387 -- make peer connection event delivery shutdown-aware -- */
}

/* START ALPHA CHANGE 2712 -- cancel a download without closing a status connection or replacement -- */
func (m *syncPeerClient) CloseStream(peerID peer.ID) error {
	m.blockStreamLock.Lock()
	streamState := m.blockStreamCancels[peerID]
	delete(m.blockStreamCancels, peerID)
	m.blockStreamLock.Unlock()

	if streamState != nil {
		streamState.cancel()
	}

	return nil
}

/* FINISH ALPHA CHANGE 2712 -- cancel a download without closing a status connection or replacement -- */

// GetBlocks returns a stream of blocks from given height to peer's latest
func (m *syncPeerClient) GetBlocks(
	peerID peer.ID,
	from uint64,
	timeoutPerBlock time.Duration,
) (<-chan *types.Block, <-chan BlockStreamTerminal, error) {
	/* START ALPHA CHANGE 2713 -- own download cancellation before establishment and retain it through forwarding -- */
	ctx, finish, err := m.rpcLifetime.Begin(context.Background())
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	streamState := &blockStreamCancelState{cancel: cancel}

	m.blockStreamLock.Lock()
	if m.blockStreamCancels == nil {
		m.blockStreamCancels = make(map[peer.ID]*blockStreamCancelState)
	}
	oldStreamState := m.blockStreamCancels[peerID]
	m.blockStreamCancels[peerID] = streamState
	m.blockStreamLock.Unlock()

	if oldStreamState != nil {
		oldStreamState.cancel()
	}

	closeClient := func() {}
	var releaseOnce sync.Once

	release := func() {
		releaseOnce.Do(func() {
			cancel()
			closeClient()

			m.blockStreamLock.Lock()
			if m.blockStreamCancels[peerID] == streamState {
				delete(m.blockStreamCancels, peerID)
			}
			m.blockStreamLock.Unlock()

			finish()
		})
	}

	/* This timer only covers establishment, not the download lifetime. */
	setupTimer := time.AfterFunc(netgrpc.DefaultProtocolTimeout, cancel)

	clt, ownedClose, err := m.newSyncPeerClient(ctx, peerID)
	if err != nil {
		setupTimer.Stop()
		release()

		return nil, nil, fmt.Errorf("failed to create sync peer client: %w", err)
	}
	closeClient = ownedClose

	stream, err := clt.GetBlocks(ctx, &proto.GetBlocksRequest{
		From: from,
	})

	setupFinished := setupTimer.Stop()
	if err == nil && !setupFinished {
		err = context.DeadlineExceeded
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		release()

		return nil, nil, fmt.Errorf("failed to open GetBlocks stream: %w", err)
	}

	forwardCtx := ctx
	/* FINISH ALPHA CHANGE 2713 -- own download cancellation before establishment and retain it through forwarding -- */

	/* START ALPHA CHANGE 1346 -- propagate sync stream terminal errors safely -- */
	streamBlockCh, streamTerminalCh := m.blockStreamToChannel(stream, forwardCtx.Done())

	blockCh := make(chan *types.Block)
	terminalCh := make(chan BlockStreamTerminal, 1)

	if !m.startWorker(func() {
		terminal := BlockStreamTerminal{}

		/* START ALPHA CHANGE 2714 -- release the selected download ownership before completion -- */
		defer func() {
			release()

			terminalCh <- terminal
			close(terminalCh)
			close(blockCh)
		}()
		/* FINISH ALPHA CHANGE 2714 -- release the selected download ownership before completion -- */

		timer := time.NewTimer(timeoutPerBlock)
		defer timer.Stop()

		for {
			select {
			case block, ok := <-streamBlockCh:
				if !ok {
					streamTerminal, ok := <-streamTerminalCh
					if !ok {
						terminal.Err = errors.New("sync block stream closed terminal channel without result")

						return
					}

					terminal = streamTerminal

					return
				}

				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}

				select {
				case blockCh <- block:
				case <-forwardCtx.Done():
					terminal.Err = forwardCtx.Err()

					return
				case <-m.closeCh:
					terminal.Err = errors.New("sync peer client closed while forwarding block stream")

					return
				}

				timer.Reset(timeoutPerBlock)

			case streamTerminal, ok := <-streamTerminalCh:
				if !ok {
					terminal.Err = errors.New("sync block stream closed terminal channel without result")

					return
				}

				terminal = streamTerminal

				return

			case <-timer.C:
				terminal.Err = fmt.Errorf("%w after %s", errTimeout, timeoutPerBlock)

				return

			case <-forwardCtx.Done():
				terminal.Err = forwardCtx.Err()

				return

			case <-m.closeCh:
				terminal.Err = errors.New("sync peer client closed while waiting for block stream")

				return
			}
		}
	}) {
		/* START ALPHA CHANGE 2715 -- release download ownership when forwarding admission is closed -- */
		release()
		/* FINISH ALPHA CHANGE 2715 -- release download ownership when forwarding admission is closed -- */

		terminalCh <- BlockStreamTerminal{
			Err: errors.New("sync peer client closed before forwarding block stream"),
		}
		close(terminalCh)
		close(blockCh)
	}
	/* FINISH ALPHA CHANGE 1346 -- propagate sync stream terminal errors safely -- */

	return blockCh, terminalCh, nil
}

/* START ALPHA CHANGE 2716 -- give status calls and downloads separate connection ownership -- */
func (m *syncPeerClient) newSyncPeerClient(
	ctx context.Context,
	peerID peer.ID,
) (proto.SyncPeerClient, func(), error) {
	conn, err := m.network.NewProtoConnectionContext(ctx, syncerProto, peerID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open a stream, err %w", err)
	}

	return proto.NewSyncPeerClient(conn), netgrpc.OwnClient(ctx, conn), nil
}

/* FINISH ALPHA CHANGE 2716 -- give status calls and downloads separate connection ownership -- */

// fromProto gets block from gRPC response data
func fromProto(protoBlock *proto.Block) (*types.Block, error) {
	block := &types.Block{}
	if err := block.UnmarshalRLP(protoBlock.Block); err != nil {
		return nil, err
	}

	return block, nil
}

func (m *syncPeerClient) blockStreamToChannel(
	stream proto.SyncPeer_GetBlocksClient,
	done <-chan struct{},
) (<-chan *types.Block, <-chan BlockStreamTerminal) {
	blockCh := make(chan *types.Block)
	terminalCh := make(chan BlockStreamTerminal, 1)

	if !m.startWorker(func() {
		terminal := BlockStreamTerminal{}

		defer func() {
			terminalCh <- terminal
			close(terminalCh)
			close(blockCh)
		}()

		for {
			protoBlock, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}

			if err != nil {
				metrics.IncrCounter([]string{syncerMetrics, "bad_message"}, 1)
				terminal.Err = err

				return
			}

			block, err := fromProto(protoBlock)
			if err != nil {
				metrics.IncrCounter([]string{syncerMetrics, "bad_block"}, 1)
				terminal.Err = err

				return
			}

			metrics.SetGauge([]string{syncerMetrics, "ingress_bytes"}, float32(len(protoBlock.Block)))

			select {
			case blockCh <- block:
			case <-done:
				terminal.Err = errors.New("sync block stream forwarding canceled")

				return
			case <-m.closeCh:
				terminal.Err = errors.New("sync peer client closed while reading block stream")

				return
			}
		}
	}) {
		terminalCh <- BlockStreamTerminal{
			Err: errors.New("sync peer client closed before reading block stream"),
		}
		close(terminalCh)
		close(blockCh)
	}

	return blockCh, terminalCh
}
