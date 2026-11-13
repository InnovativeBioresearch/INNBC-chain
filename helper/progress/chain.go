package progress

import (
	"sync"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
)

type ChainSyncType string

/* START ALPHA CHANGE 3268 -- retain only the active bulk synchronization type -- */
const ChainSyncBulk ChainSyncType = "bulk-sync"

/* FINISH ALPHA CHANGE 3268 -- retain only the active bulk synchronization type -- */

// Progression defines the status of the sync
// progression of the node
type Progression struct {
	// SyncType is indicating the sync method
	SyncType ChainSyncType

	// StartingBlock is the initial block that the node is starting
	// the sync from. It is reset after every sync batch
	StartingBlock uint64

	// CurrentBlock is the last written block from the sync batch
	CurrentBlock uint64

	// HighestBlock is the target block in the sync batch
	HighestBlock uint64
}

type ProgressionWrapper struct {
	// progression is a reference to the ongoing batch sync.
	// Nil if no batch sync is currently in progress
	progression *Progression

	// stopCh is the channel for receiving stop signals
	// in progression tracking
	stopCh chan struct{}

	lock sync.RWMutex

	syncType ChainSyncType
}

func NewProgressionWrapper(syncType ChainSyncType) *ProgressionWrapper {
	return &ProgressionWrapper{
		progression: nil,
		stopCh:      make(chan struct{}),
		syncType:    syncType,
	}
}

// startProgression initializes the progression tracking
func (pw *ProgressionWrapper) StartProgression(
	startingBlock uint64,
	subscription blockchain.Subscription,
) {
	pw.lock.Lock()
	defer pw.lock.Unlock()

	pw.progression = &Progression{
		SyncType:      pw.syncType,
		StartingBlock: startingBlock,
	}

	go pw.RunUpdateLoop(subscription)
}

// runUpdateLoop starts the blockchain event monitoring loop and
// updates the currently written block in the batch sync
func (pw *ProgressionWrapper) RunUpdateLoop(subscription blockchain.Subscription) {
	eventCh := subscription.GetEventCh()

	for {
		select {
		case event := <-eventCh:
			/* START ALPHA CHANGE 784 -- progress only tracks canonical head events after fork pruning -- */
			if len(event.NewChain) == 0 {
				continue
			}
			/* FINISH ALPHA CHANGE 784 -- progress only tracks canonical head events after fork pruning -- */

			lastBlock := event.NewChain[len(event.NewChain)-1]
			pw.UpdateCurrentProgression(lastBlock.Number)
		case <-pw.stopCh:
			return
		}
	}
}

// StopProgression stops the progression tracking
func (pw *ProgressionWrapper) StopProgression() {
	pw.stopCh <- struct{}{}

	pw.lock.Lock()
	defer pw.lock.Unlock()

	pw.progression = nil
}

// UpdateCurrentProgression sets the currently written block in the bulk sync
func (pw *ProgressionWrapper) UpdateCurrentProgression(currentBlock uint64) {
	pw.lock.Lock()
	defer pw.lock.Unlock()

	pw.progression.CurrentBlock = currentBlock
}

// UpdateHighestProgression sets the highest-known target block in the bulk sync
func (pw *ProgressionWrapper) UpdateHighestProgression(highestBlock uint64) {
	pw.lock.Lock()
	defer pw.lock.Unlock()

	pw.progression.HighestBlock = highestBlock
}

/* START ALPHA CHANGE 2975 -- return an independent progression snapshot under the existing lock -- */
// GetProgression returns a snapshot of the latest sync progression.
func (pw *ProgressionWrapper) GetProgression() *Progression {
	pw.lock.RLock()
	defer pw.lock.RUnlock()

	if pw.progression == nil {
		return nil
	}

	snapshot := *pw.progression
	return &snapshot
}

/* FINISH ALPHA CHANGE 2975 -- return an independent progression snapshot under the existing lock -- */
