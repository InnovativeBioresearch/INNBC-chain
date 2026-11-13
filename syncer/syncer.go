package syncer

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/armon/go-metrics"
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/helper/progress"
	"github.com/innovativebioresearch/innbc-chain/network/event"

	/* START ALPHA CHANGE 2171 -- distinguish local state failures during synchronization -- */
	"github.com/innovativebioresearch/innbc-chain/state"
	/* FINISH ALPHA CHANGE 2171 -- distinguish local state failures during synchronization -- */
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	syncerName  = "syncer"
	syncerProto = "/syncer/0.2"

	/* START ALPHA CHANGE 1670 -- bound exhausted-peer retry wait -- */
	defaultPeerRetryInterval = 5 * time.Second
	/* FINISH ALPHA CHANGE 1670 -- bound exhausted-peer retry wait -- */
)

var (
	errTimeout = errors.New("timeout awaiting block from peer")
)

// XXX: Don't use this syncer for the consensus that may cause fork.
// This syncer doesn't assume forks
type syncer struct {
	logger          hclog.Logger
	blockchain      Blockchain
	syncProgression Progression

	peerMap         *PeerMap
	syncPeerService SyncPeerService
	syncPeerClient  SyncPeerClient

	/* START ALPHA CHANGE 2766 -- coordinate peer status publication with membership removal -- */
	network       Network
	peerStateLock sync.Mutex
	/* FINISH ALPHA CHANGE 2766 -- coordinate peer status publication with membership removal -- */

	// Timeout for syncing a block
	blockTimeout time.Duration

	/* START ALPHA CHANGE 1671 -- configure exhausted-peer retry interval -- */
	peerRetryInterval time.Duration
	/* FINISH ALPHA CHANGE 1671 -- configure exhausted-peer retry interval -- */

	// Channel to notify Sync that a new status arrived
	newStatusCh chan struct{}

	/* START ALPHA CHANGE 1084 -- separate syncer shutdown from status notifications -- */
	closeCh chan struct{}
	/* FINISH ALPHA CHANGE 1084 -- separate syncer shutdown from status notifications -- */

	/* START ALPHA CHANGE 1067 -- make syncer shutdown idempotent -- */
	closeOnce sync.Once
	closeErr  error

	/* START ALPHA CHANGE 1388 -- gate and wait for syncer background work -- */
	closeLock sync.Mutex
	closeWG   sync.WaitGroup
	/* FINISH ALPHA CHANGE 1388 -- gate and wait for syncer background work -- */
	/* FINISH ALPHA CHANGE 1067 -- make syncer shutdown idempotent -- */
}

func NewSyncer(
	logger hclog.Logger,
	network Network,
	blockchain Blockchain,
	blockTimeout time.Duration,
) Syncer {
	return &syncer{
		logger:     logger.Named(syncerName),
		blockchain: blockchain,
		/* START ALPHA CHANGE 2767 -- retain current network membership for status publication -- */
		network: network,
		/* FINISH ALPHA CHANGE 2767 -- retain current network membership for status publication -- */
		syncProgression: progress.NewProgressionWrapper(progress.ChainSyncBulk),
		syncPeerService: NewSyncPeerService(network, blockchain),
		syncPeerClient:  NewSyncPeerClient(logger, network, blockchain),
		blockTimeout:    blockTimeout,
		/* START ALPHA CHANGE 1672 -- initialize exhausted-peer retry interval -- */
		peerRetryInterval: defaultPeerRetryInterval,
		/* FINISH ALPHA CHANGE 1672 -- initialize exhausted-peer retry interval -- */
		newStatusCh: make(chan struct{}),
		/* START ALPHA CHANGE 1085 -- initialize syncer shutdown channel -- */
		closeCh: make(chan struct{}),
		/* FINISH ALPHA CHANGE 1085 -- initialize syncer shutdown channel -- */
		peerMap: new(PeerMap),
	}
}

/* START ALPHA CHANGE 1389 -- register syncer runtime work safely -- */
func (s *syncer) enterRuntimeWork() bool {
	s.closeLock.Lock()
	defer s.closeLock.Unlock()

	select {
	case <-s.closeCh:
		return false
	default:
		s.closeWG.Add(1)

		return true
	}
}

func (s *syncer) leaveRuntimeWork() {
	s.closeWG.Done()
}

func (s *syncer) startWorker(fn func()) bool {
	if !s.enterRuntimeWork() {
		return false
	}

	go func() {
		defer s.leaveRuntimeWork()

		fn()
	}()

	return true
}

/* FINISH ALPHA CHANGE 1389 -- register syncer runtime work safely -- */

// Start starts goroutine processes
func (s *syncer) Start() error {
	if err := s.syncPeerClient.Start(); err != nil {
		return err
	}

	s.syncPeerService.Start()

	s.initializePeerMap()

	/* START ALPHA CHANGE 1390 -- track syncer status workers -- */
	if !s.startWorker(s.startPeerStatusUpdateProcess) {
		return errors.New("syncer closed before peer status worker startup")
	}

	if !s.startWorker(s.startPeerConnectionEventProcess) {
		return errors.New("syncer closed before peer connection worker startup")
	}
	/* FINISH ALPHA CHANGE 1390 -- track syncer status workers -- */

	return nil
}

// Close terminates goroutine processes
func (s *syncer) Close() error {
	/* START ALPHA CHANGE 1068 -- close syncer resources only once -- */
	s.closeOnce.Do(func() {
		/* START ALPHA CHANGE 1402 -- complete syncer cleanup and wait for admitted work -- */
		/* START ALPHA CHANGE 2807 -- order shutdown admission against final status publication -- */
		s.closeLock.Lock()
		s.peerStateLock.Lock()
		close(s.closeCh)
		s.peerStateLock.Unlock()
		s.closeLock.Unlock()
		/* FINISH ALPHA CHANGE 2807 -- order shutdown admission against final status publication -- */

		if err := s.syncPeerService.Close(); err != nil {
			s.closeErr = errors.Join(s.closeErr, err)
		}

		s.syncPeerClient.Close()
		s.closeWG.Wait()
		/* FINISH ALPHA CHANGE 1402 -- complete syncer cleanup and wait for admitted work -- */
	})

	return s.closeErr
	/* FINISH ALPHA CHANGE 1068 -- close syncer resources only once -- */
}

/* START ALPHA CHANGE 2768 -- preserve query ownership and serialize status publication with removal -- */

/* START ALPHA CHANGE 2808 -- publish only owned startup and queued status results -- */
// initializePeerMap fetches peer statuses and initializes map
func (s *syncer) initializePeerMap() {
	for _, update := range s.syncPeerClient.GetConnectedPeerStatusUpdates() {
		s.publishPeerStatus(update)
	}
}

// startPeerStatusUpdateProcess subscribes peer status change event and updates peer map
func (s *syncer) startPeerStatusUpdateProcess() {
	for update := range s.syncPeerClient.GetPeerStatusUpdateCh() {
		s.publishPeerStatus(update)
	}
}

/* FINISH ALPHA CHANGE 2808 -- publish only owned startup and queued status results -- */

// startPeerConnectionEventProcess processes peer connection change events
func (s *syncer) startPeerConnectionEventProcess() {
	for e := range s.syncPeerClient.GetPeerConnectionUpdateEventCh() {
		if !e.IsCurrent() {
			continue
		}

		peerID := e.PeerID

		switch e.Type {
		case event.PeerConnected:
			/* Capture the originating event before launching the worker. */
			current := e.IsCurrent
			s.startWorker(func() {
				s.initNewPeerStatus(peerID, current)
			})
		case event.PeerDisconnected:
			s.removeFromPeerMap(peerID, e.IsCurrent)
		/* START ALPHA CHANGE 2881 -- atomically reconsider status after an eligible session ends -- */
		case event.PeerSessionEnded:
			/*
				No application-registration predicate belongs here.
				WithNoPeerSession preserves any eligible replacement.
			*/
			s.removeFromPeerMap(peerID)
			/* FINISH ALPHA CHANGE 2881 -- atomically reconsider status after an eligible session ends -- */
		}
	}
}

/* START ALPHA CHANGE 2809 -- preserve captured query ownership alongside connection event checks -- */
// initNewPeerStatus fetches status of the peer and put to peer map
func (s *syncer) initNewPeerStatus(peerID peer.ID, current ...func() bool) {
	update, err := s.syncPeerClient.GetPeerStatusUpdate(peerID)
	if err != nil {
		s.logger.Warn("failed to get peer status, skip", "id", peerID, "err", err)

		return
	}

	s.publishPeerStatus(update, current...)
}

/* FINISH ALPHA CHANGE 2809 -- preserve captured query ownership alongside connection event checks -- */

/* START ALPHA CHANGE 2810 -- validate session ownership atomically with peer map publication -- */
func (s *syncer) publishPeerStatus(update *PeerStatusUpdate, current ...func() bool) {
	if update == nil ||
		update.Status == nil ||
		update.Session == nil ||
		s.network == nil {
		return
	}

	s.peerStateLock.Lock()

	/* These callbacks may acquire admission locks; run them before the helper. */
	for _, check := range current {
		if check != nil && !check() {
			s.peerStateLock.Unlock()

			return
		}
	}

	select {
	case <-s.closeCh:
		s.peerStateLock.Unlock()

		return
	default:
	}

	published := s.network.WithPeerSession(
		update.Status.ID,
		update.Session,
		func() {
			/* No networking or notifications while the admission lock is held. */
			s.peerMap.Put(update.Status)
		},
	)
	s.peerStateLock.Unlock()

	if published {
		s.notifyNewStatusEvent()
	}
}

/* FINISH ALPHA CHANGE 2810 -- validate session ownership atomically with peer map publication -- */

// removeFromPeerMap removes the peer from peer map
/* START ALPHA CHANGE 2838 -- preserve status while an eligible replacement session exists -- */
func (s *syncer) removeFromPeerMap(peerID peer.ID, current ...func() bool) {
	if s.network == nil {
		return
	}

	s.peerStateLock.Lock()
	defer s.peerStateLock.Unlock()

	/* Event checks may acquire admission locks; run them before the helper. */
	for _, check := range current {
		if check != nil && !check() {
			return
		}
	}

	s.network.WithNoPeerSession(peerID, func() {
		/* Only the map update runs while the admission read lock is held. */
		s.peerMap.Remove(peerID)
	})
}

/* FINISH ALPHA CHANGE 2838 -- preserve status while an eligible replacement session exists -- */

/* FINISH ALPHA CHANGE 2768 -- preserve query ownership and serialize status publication with removal -- */

// notifyNewStatusEvent emits signal to newStatusCh
func (s *syncer) notifyNewStatusEvent() {
	/* START ALPHA CHANGE 1087 -- avoid sending status notifications after syncer shutdown -- */
	select {
	case <-s.closeCh:
		return
	case s.newStatusCh <- struct{}{}:
	default:
	}
	/* FINISH ALPHA CHANGE 1087 -- avoid sending status notifications after syncer shutdown -- */
}

// GetSyncProgression returns progression
func (s *syncer) GetSyncProgression() *progress.Progression {
	return s.syncProgression.GetProgression()
}

// HasSyncPeer returns whether syncer has the peer to syncs blocks
// return false if syncer has no peer whose latest block height doesn't exceed local height
func (s *syncer) HasSyncPeer() bool {
	header := s.blockchain.Header()
	if header == nil {
		return false
	}

	requestedFrom := header.Number + 1
	bestPeer := s.peerMap.BestPeerForBlock(requestedFrom, nil)

	return bestPeer != nil
}

/* START ALPHA CHANGE 1673 -- wait for status or bounded exhausted-peer retry -- */
func (s *syncer) waitForStatusOrRetry() bool {
	timer := time.NewTimer(s.peerRetryInterval)
	defer timer.Stop()

	select {
	case <-s.closeCh:
		return false
	case <-s.newStatusCh:
		return true
	case <-timer.C:
		return true
	}
}

/* FINISH ALPHA CHANGE 1673 -- wait for status or bounded exhausted-peer retry -- */

/* START ALPHA CHANGE 2171 -- prevent status notifications from accelerating local failure retries -- */
func (s *syncer) waitForLocalStateRetry() bool {
	delay := s.peerRetryInterval
	if delay <= 0 {
		delay = defaultPeerRetryInterval
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-s.closeCh:
		return false
	case <-timer.C:
		return true
	}
}

/* FINISH ALPHA CHANGE 2171 -- prevent status notifications from accelerating local failure retries -- */

// Sync syncs block with the best peer until callback returns true
func (s *syncer) Sync(callback func(*types.FullBlock) bool) error {
	/* START ALPHA CHANGE 1403 -- track active sync calls during shutdown -- */
	if !s.enterRuntimeWork() {
		return nil
	}
	defer s.leaveRuntimeWork()
	/* FINISH ALPHA CHANGE 1403 -- track active sync calls during shutdown -- */

	localLatest := s.blockchain.Header().Number
	skipList := make(map[peer.ID]bool)

	/* START ALPHA CHANGE 1079 -- let sync evaluate initialized peers immediately -- */
	waitForStatus := false
	/* FINISH ALPHA CHANGE 1079 -- let sync evaluate initialized peers immediately -- */

	for {
		// Wait for a new event to arrive
		/* START ALPHA CHANGE 1055 -- exit sync loop when status channel is closed -- */
		/* START ALPHA CHANGE 1080 -- skip first status wait after peer map initialization -- */
		if waitForStatus {
			/* START ALPHA CHANGE 1088 -- let sync exit through shutdown channel -- */
			/* START ALPHA CHANGE 1674 -- retry even without another status notification -- */
			if !s.waitForStatusOrRetry() {
				return nil
			}
			/* FINISH ALPHA CHANGE 1674 -- retry even without another status notification -- */
			/* FINISH ALPHA CHANGE 1088 -- let sync exit through shutdown channel -- */
		}
		waitForStatus = true
		/* FINISH ALPHA CHANGE 1080 -- skip first status wait after peer map initialization -- */
		/* FINISH ALPHA CHANGE 1055 -- exit sync loop when status channel is closed -- */

		// fetch local latest block
		if header := s.blockchain.Header(); header != nil {
			localLatest = header.Number
		}

		/* START ALPHA CHANGE 1675 -- select only peers containing the next required block -- */
		requestedFrom := localLatest + 1

		// Pick the most up-to-date peer among those that can serve the next block.
		bestPeer := s.peerMap.BestPeerForBlock(requestedFrom, skipList)
		if bestPeer == nil {
			// Reset temporary exclusions. The next loop waits for either a
			// status update or the bounded retry interval.
			skipList = make(map[peer.ID]bool)

			continue
		}
		/* FINISH ALPHA CHANGE 1675 -- select only peers containing the next required block -- */

		// if the bestPeer does not have a new block continue
		if bestPeer.Number <= localLatest {
			continue
		}

		// fetch block from the peer
		lastNumber, shouldTerminate, err := s.bulkSyncWithPeer(bestPeer.ID, bestPeer.Number, callback)

		/* START ALPHA CHANGE 2171 -- retry local state failures without excluding the serving peer -- */
		if errors.Is(err, state.ErrLocalStateFailure) {
			state.LogLocalStateFailure(s.logger, state.LocalStateLogSync, err)

			if !s.waitForLocalStateRetry() {
				return nil
			}

			// The timed wait has already completed. Re-evaluate the current
			// head and peers without adding the serving peer to skipList.
			waitForStatus = false

			continue
		}
		/* FINISH ALPHA CHANGE 2171 -- retry local state failures without excluding the serving peer -- */

		if err != nil {
			/* START ALPHA CHANGE 1347 -- log sync peer failures with structured error context -- */
			s.logger.Warn(
				"failed to complete bulk sync with peer, trying next one",
				"peerID", bestPeer.ID,
				"err", err,
			)
			/* FINISH ALPHA CHANGE 1347 -- log sync peer failures with structured error context -- */
		}

		/* START ALPHA CHANGE 3856 -- honor callback termination before incomplete-download fallback -- */
		if shouldTerminate {
			break
		}
		/* FINISH ALPHA CHANGE 3856 -- honor callback termination before incomplete-download fallback -- */

		if lastNumber < bestPeer.Number {
			skipList[bestPeer.ID] = true

			/* START ALPHA CHANGE 1082 -- immediately try another known peer after partial sync failure -- */
			waitForStatus = false
			/* FINISH ALPHA CHANGE 1082 -- immediately try another known peer after partial sync failure -- */

			// continue to next peer
			continue
		}
	}

	return nil
}

// bulkSyncWithPeer syncs block with a given peer
func (s *syncer) bulkSyncWithPeer(peerID peer.ID, peerLatestBlock uint64,
	newBlockCallback func(*types.FullBlock) bool) (uint64, bool, error) {
	localLatest := s.blockchain.Header().Number
	shouldTerminate := false

	/* START ALPHA CHANGE 1348 -- consume sync stream terminal result explicitly -- */
	blockCh, terminalCh, err := s.syncPeerClient.GetBlocks(peerID, localLatest+1, s.blockTimeout)
	if err != nil {
		return localLatest, false, err
	}
	/* FINISH ALPHA CHANGE 1348 -- consume sync stream terminal result explicitly -- */

	// Create a blockchain subscription for the sync progression and start tracking
	subscription := s.blockchain.SubscribeEvents()
	s.syncProgression.StartProgression(localLatest+1, subscription)
	s.syncProgression.UpdateHighestProgression(peerLatestBlock)

	defer func() {
		err := s.syncPeerClient.CloseStream(peerID)
		if err != nil {
			s.logger.Error("Failed to close stream: ", err)
		}

		// Stop monitoring the sync progression upon exit
		s.syncProgression.StopProgression()
		s.blockchain.UnsubscribeEvents(subscription)
	}()

	/* START ALPHA CHANGE 1349 -- require explicit sync stream completion status -- */
	lastReceivedNumber := localLatest

	for {
		select {
		case block, ok := <-blockCh:
			if !ok {
				blockCh = nil

				continue
			}

			/* START ALPHA CHANGE 3833 -- reject non-contiguous sync block streams before verification -- */
			number := block.Number()
			if number == 0 || number-1 != lastReceivedNumber {
				return lastReceivedNumber, false, fmt.Errorf(
					"non-contiguous block stream from peer %s: block %d follows %d",
					peerID,
					number,
					lastReceivedNumber,
				)
			}
			/* FINISH ALPHA CHANGE 3833 -- reject non-contiguous sync block streams before verification -- */

			/* START ALPHA CHANGE 2171 -- do not count local state failures as bad remote blocks -- */
			fullBlock, err := s.blockchain.VerifyFinalizedBlock(block)
			if err != nil {
				if errors.Is(err, state.ErrLocalStateFailure) {
					metrics.IncrCounter([]string{syncerMetrics, "local_state_failure"}, 1)
				} else {
					metrics.IncrCounter([]string{syncerMetrics, "bad_block"}, 1)
				}

				return lastReceivedNumber, false, fmt.Errorf("unable to verify block, %w", err)
			}

			if err := s.blockchain.WriteFullBlock(fullBlock, syncerName); err != nil {
				if errors.Is(err, state.ErrLocalStateFailure) {
					metrics.IncrCounter([]string{syncerMetrics, "local_state_failure"}, 1)
				} else {
					metrics.IncrCounter([]string{syncerMetrics, "bad_block"}, 1)
				}

				return lastReceivedNumber, false, fmt.Errorf("failed to write block while bulk syncing: %w", err)
			}
			/* FINISH ALPHA CHANGE 2171 -- do not count local state failures as bad remote blocks -- */

			updateMetrics(fullBlock)
			shouldTerminate = newBlockCallback(fullBlock)

			lastReceivedNumber = block.Number()

			/* START ALPHA CHANGE 1448 -- stop bulk sync immediately when callback requests termination -- */
			if shouldTerminate {
				return lastReceivedNumber, true, nil
			}
			/* FINISH ALPHA CHANGE 1448 -- stop bulk sync immediately when callback requests termination -- */

		case terminal, ok := <-terminalCh:
			if !ok {
				return lastReceivedNumber, false, fmt.Errorf(
					"sync block stream from peer %s closed terminal channel without result: start block %d, advertised head %d, last received block %d",
					peerID,
					localLatest+1,
					peerLatestBlock,
					lastReceivedNumber,
				)
			}

			if terminal.Err != nil {
				return lastReceivedNumber, false, fmt.Errorf(
					"sync block stream failed from peer %s starting at block %d toward advertised head %d after last received block %d: %w",
					peerID,
					localLatest+1,
					peerLatestBlock,
					lastReceivedNumber,
					terminal.Err,
				)
			}

			if lastReceivedNumber < peerLatestBlock {
				return lastReceivedNumber, false, fmt.Errorf(
					"peer %s ended block stream before advertised head: start block %d, advertised head %d, last received block %d",
					peerID,
					localLatest+1,
					peerLatestBlock,
					lastReceivedNumber,
				)
			}

			return lastReceivedNumber, shouldTerminate, nil
		}
	}
	/* FINISH ALPHA CHANGE 1349 -- require explicit sync stream completion status -- */
}

func updateMetrics(fullBlock *types.FullBlock) {
	metrics.SetGauge([]string{syncerMetrics, "tx_num"}, float32(len(fullBlock.Block.Transactions)))
	metrics.SetGauge([]string{syncerMetrics, "receipts_num"}, float32(len(fullBlock.Receipts)))
	metrics.SetGauge([]string{syncerMetrics, "blocks_num"}, 1)
}
