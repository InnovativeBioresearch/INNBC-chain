package innbcbft

import (
	"errors"
	"fmt"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
	/* START ALPHA CHANGE 1537 -- remove bbolt from validator snapshot caching -- */ /* FINISH ALPHA CHANGE 1537 -- remove bbolt from validator snapshot caching -- */)

type validatorSnapshot struct {
	Epoch            uint64               `json:"epoch"`
	EpochEndingBlock uint64               `json:"epochEndingBlock"`
	Snapshot         validator.AccountSet `json:"snapshot"`
}

func (vs *validatorSnapshot) copy() *validatorSnapshot {
	copiedAccountSet := vs.Snapshot.Copy()

	return &validatorSnapshot{
		Epoch:            vs.Epoch,
		EpochEndingBlock: vs.EpochEndingBlock,
		Snapshot:         copiedAccountSet,
	}
}

type validatorsSnapshotCache struct {
	snapshots  map[uint64]*validatorSnapshot
	state      *State
	blockchain blockchainBackend
	lock       sync.Mutex
	logger     hclog.Logger
}

// newValidatorsSnapshotCache initializes a new instance of validatorsSnapshotCache
func newValidatorsSnapshotCache(
	logger hclog.Logger, state *State, blockchain blockchainBackend,
) *validatorsSnapshotCache {
	return &validatorsSnapshotCache{
		snapshots:  map[uint64]*validatorSnapshot{},
		state:      state,
		blockchain: blockchain,
		logger:     logger.Named("validators_snapshot"),
	}
}

// GetSnapshot tries to retrieve the most recent cached snapshot (if any) and
// applies pending validator set deltas to it.
// Otherwise, it builds a snapshot from scratch and applies pending validator set deltas.
func (v *validatorsSnapshotCache) GetSnapshot(
	blockNumber uint64,
	parents []*types.Header,
	/* START ALPHA CHANGE 1538 -- use MDBX transactions for validator snapshot calculation -- */
	dbTx *stateTx,
	/* FINISH ALPHA CHANGE 1538 -- use MDBX transactions for validator snapshot calculation -- */
) (validator.AccountSet, error) {
	tx := dbTx
	isPassedTxNil := dbTx == nil

	if isPassedTxNil {
		// if no tx is passed, we need to open one, because,
		// GetSnapshot is called from multiple routines (new sequence, fsm, OnBlockInserted, etc)
		// and what can happen is that one routine takes its lock, and without a global tx,
		// a deadlock can happen between that routine and OnBlockInserted, because it to
		// is calling GetSnapshot, but it also opens a global db tx, resulting in a deadlock
		// between one routine waiting to get a transaction on db, and OnBlockInserted waiting
		// to get the validatorsSnapshotCache lock
		t, err := v.state.beginDBTransaction(true)
		if err != nil {
			return nil, err
		}

		tx = t
		defer tx.Rollback() //nolint:errcheck
	}

	v.lock.Lock()
	defer func() {
		v.lock.Unlock()
	}()

	/* START ALPHA CHANGE 1460 -- keep external transaction cache updates unpublished -- */
	if !isPassedTxNil {
		committedSnapshots := make(map[uint64]*validatorSnapshot, len(v.snapshots))
		for epoch, snapshot := range v.snapshots {
			committedSnapshots[epoch] = snapshot.copy()
		}

		// GetSnapshot may temporarily populate or prune v.snapshots while it
		// evaluates the caller-owned transaction. Restore the committed
		// in-memory view before releasing the cache lock. Database writes remain
		// staged in the external transaction and become visible after commit.
		defer func() {
			v.snapshots = committedSnapshots
		}()
	}
	/* FINISH ALPHA CHANGE 1460 -- keep external transaction cache updates unpublished -- */

	_, extra, err := getBlockData(blockNumber, v.blockchain)
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 2573 -- reject missing checkpoints and non-genesis epoch zero before snapshot calculation -- */
	if extra.Checkpoint == nil {
		return nil, fmt.Errorf("checkpoint data for block %d is missing", blockNumber)
	}

	if blockNumber != 0 && extra.Checkpoint.EpochNumber == 0 {
		return nil, fmt.Errorf("invalid checkpoint epoch for non-genesis block %d: got zero", blockNumber)
	}
	/* FINISH ALPHA CHANGE 2573 -- reject missing checkpoints and non-genesis epoch zero before snapshot calculation -- */

	isEpochEndingBlock, err := isEpochEndingBlock(blockNumber, extra, v.blockchain)
	if err != nil && !errors.Is(err, blockchain.ErrNoBlock) {
		// if there is no block after given block, we assume its not epoch ending block
		// but, it's a regular use case, and we should not stop the snapshot calculation
		// because there are cases we need the snapshot for the latest block in chain
		return nil, err
	}

	epochToGetSnapshot := extra.Checkpoint.EpochNumber
	/* START ALPHA CHANGE 2573 -- prevent snapshot epoch subtraction from wrapping -- */
	if !isEpochEndingBlock {
		if epochToGetSnapshot == 0 {
			return nil, fmt.Errorf("cannot derive validator snapshot epoch for non-epoch-ending block %d", blockNumber)
		}

		epochToGetSnapshot--
	}
	/* FINISH ALPHA CHANGE 2573 -- prevent snapshot epoch subtraction from wrapping -- */

	v.logger.Trace("Retrieving snapshot started...", "Block", blockNumber, "Epoch", epochToGetSnapshot)

	latestValidatorSnapshot, err := v.getLastCachedSnapshot(epochToGetSnapshot, tx)
	if err != nil {
		return nil, err
	}

	if latestValidatorSnapshot != nil && latestValidatorSnapshot.Epoch == epochToGetSnapshot {
		// we have snapshot for required block (epoch) in cache
		return latestValidatorSnapshot.Snapshot, nil
	}

	if latestValidatorSnapshot == nil {
		// Haven't managed to retrieve snapshot for any epoch from the cache.
		// Build snapshot from the scratch, by applying delta from the genesis block.
		genesisBlockSnapshot, err := v.computeSnapshot(nil, 0, parents)
		if err != nil {
			return nil, fmt.Errorf("failed to compute snapshot for epoch 0: %w", err)
		}

		err = v.storeSnapshot(genesisBlockSnapshot, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to store validators snapshot for epoch 0: %w", err)
		}

		latestValidatorSnapshot = genesisBlockSnapshot

		v.logger.Trace("Built validators snapshot for genesis block")
	}

	deltasCount := 0

	v.logger.Trace("Applying deltas started...", "LatestSnapshotEpoch", latestValidatorSnapshot.Epoch)

	// Create the snapshot for the desired block (epoch) by incrementally applying deltas to the latest stored snapshot
	for latestValidatorSnapshot.Epoch < epochToGetSnapshot {
		nextEpochEndBlockNumber, err := v.getNextEpochEndingBlock(latestValidatorSnapshot.EpochEndingBlock)
		if err != nil {
			return nil, fmt.Errorf("failed to get the epoch ending block for epoch: %d. Error: %w",
				latestValidatorSnapshot.Epoch+1, err)
		}

		v.logger.Trace("Applying delta", "epochEndBlock", nextEpochEndBlockNumber)

		intermediateSnapshot, err := v.computeSnapshot(latestValidatorSnapshot, nextEpochEndBlockNumber, parents)
		if err != nil {
			return nil, fmt.Errorf("failed to compute snapshot for epoch %d: %w", latestValidatorSnapshot.Epoch+1, err)
		}

		latestValidatorSnapshot = intermediateSnapshot
		if err = v.storeSnapshot(latestValidatorSnapshot, tx); err != nil {
			return nil, fmt.Errorf("failed to store validators snapshot for epoch %d: %w", latestValidatorSnapshot.Epoch, err)
		}

		deltasCount++
	}

	v.logger.Trace(
		fmt.Sprintf("Applied %d delta(s) to the validators snapshot", deltasCount),
		"Epoch", latestValidatorSnapshot.Epoch,
	)

	if err := v.cleanup(tx); err != nil {
		// error on cleanup should not block or fail any action
		v.logger.Error("could not clean validator snapshots from cache and db", "err", err)
	}

	if isPassedTxNil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	return latestValidatorSnapshot.Snapshot, nil
}

// computeSnapshot gets desired block header by block number, extracts its extra and applies given delta to the snapshot
func (v *validatorsSnapshotCache) computeSnapshot(
	existingSnapshot *validatorSnapshot,
	nextEpochEndBlockNumber uint64,
	parents []*types.Header,
) (*validatorSnapshot, error) {
	var header *types.Header

	v.logger.Trace("Compute snapshot started...", "BlockNumber", nextEpochEndBlockNumber)

	if len(parents) > 0 {
		for i := len(parents) - 1; i >= 0; i-- {
			parentHeader := parents[i]
			if parentHeader.Number == nextEpochEndBlockNumber {
				v.logger.Trace("Compute snapshot. Found header in parents", "Header", parentHeader.Number)
				header = parentHeader

				break
			}
		}
	}

	if header == nil {
		var ok bool

		header, ok = v.blockchain.GetHeaderByNumber(nextEpochEndBlockNumber)
		if !ok {
			return nil, fmt.Errorf("unknown block. Block number=%v", nextEpochEndBlockNumber)
		}
	}

	extra, err := GetIbftExtra(header.ExtraData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode extra from the block#%d: %w", header.Number, err)
	}

	var (
		snapshot      validator.AccountSet
		snapshotEpoch uint64
	)

	if existingSnapshot == nil {
		snapshot = validator.AccountSet{}
	} else {
		snapshot = existingSnapshot.Snapshot
		snapshotEpoch = existingSnapshot.Epoch + 1
	}

	snapshot, err = snapshot.ApplyDelta(extra.Validators)
	if err != nil {
		return nil, fmt.Errorf("failed to apply delta to the validators snapshot, block#%d: %w", header.Number, err)
	}

	if len(snapshot) == 0 {
		return nil, fmt.Errorf("validator snapshot is empty for block: %d", header.Number)
	}

	v.logger.Debug("Computed snapshot",
		"blockNumber", nextEpochEndBlockNumber,
		"snapshot", snapshot.String(),
		"delta", extra.Validators)

	return &validatorSnapshot{
		Epoch:            snapshotEpoch,
		EpochEndingBlock: nextEpochEndBlockNumber,
		Snapshot:         snapshot,
	}, nil
}

// storeSnapshot stores given snapshot to the in-memory cache and database
func (v *validatorsSnapshotCache) storeSnapshot(
	snapshot *validatorSnapshot,
	/* START ALPHA CHANGE 1539 -- store validator snapshots through MDBX -- */
	dbTx *stateTx,
	/* FINISH ALPHA CHANGE 1539 -- store validator snapshots through MDBX -- */
) error {
	copySnap := snapshot.copy()
	v.snapshots[copySnap.Epoch] = copySnap

	if err := v.state.EpochStore.insertValidatorSnapshot(copySnap, dbTx); err != nil {
		return fmt.Errorf("failed to insert validator snapshot for epoch %d to the database: %w", copySnap.Epoch, err)
	}

	v.logger.Trace("Store snapshot", "Snapshots", v.snapshots)

	return nil
}

// Cleanup cleans the validators cache in memory and db
func (v *validatorsSnapshotCache) cleanup(
	/* START ALPHA CHANGE 1540 -- clean validator snapshots through MDBX -- */
	dbTx *stateTx,
	/* FINISH ALPHA CHANGE 1540 -- clean validator snapshots through MDBX -- */
) error {
	if len(v.snapshots) >= validatorSnapshotLimit {
		latestEpoch := uint64(0)

		for e := range v.snapshots {
			if e > latestEpoch {
				latestEpoch = e
			}
		}

		startEpoch := latestEpoch
		cache := make(map[uint64]*validatorSnapshot, numberOfSnapshotsToLeaveInMemory)

		for i := 0; i < numberOfSnapshotsToLeaveInMemory; i++ {
			if snapshot, exists := v.snapshots[startEpoch]; exists {
				cache[startEpoch] = snapshot
			}

			startEpoch--
		}

		v.snapshots = cache

		return v.state.EpochStore.cleanValidatorSnapshotsFromDB(latestEpoch, dbTx)
	}

	return nil
}

// getLastCachedSnapshot gets the latest snapshot cached
// If it doesn't have snapshot cached for desired epoch, it will return the latest one it has
func (v *validatorsSnapshotCache) getLastCachedSnapshot(
	currentEpoch uint64,
	/* START ALPHA CHANGE 1541 -- retrieve cached validator snapshots through MDBX -- */
	dbTx *stateTx,
	/* FINISH ALPHA CHANGE 1541 -- retrieve cached validator snapshots through MDBX -- */
) (*validatorSnapshot, error) {
	requestedEpoch := currentEpoch

	cachedSnapshot := v.snapshots[currentEpoch]
	if cachedSnapshot != nil {
		return cachedSnapshot, nil
	}

	/* START ALPHA CHANGE 2574 -- select the latest eligible cached snapshot without scanning numeric epoch gaps -- */
	var cachedEpoch uint64

	for epoch, snapshot := range v.snapshots {
		if snapshot == nil || epoch > requestedEpoch {
			continue
		}

		if cachedSnapshot == nil || epoch > cachedEpoch {
			cachedSnapshot = snapshot
			cachedEpoch = epoch
		}
	}

	if cachedSnapshot != nil {
		v.logger.Trace("Found snapshot in memory cache", "Epoch", cachedEpoch)
	}
	/* FINISH ALPHA CHANGE 2574 -- select the latest eligible cached snapshot without scanning numeric epoch gaps -- */

	/* START ALPHA CHANGE 971 -- only use database validator snapshots at or before requested epoch -- */
	dbSnapshot, err := v.state.EpochStore.getLastSnapshotAtOrBefore(requestedEpoch, dbTx)
	if err != nil {
		return nil, err
	}

	if dbSnapshot != nil {
		// if we do not have any snapshot in memory, or db snapshot is newer than the one in memory,
		// return the newest snapshot that is still not newer than the requested epoch.
		if cachedSnapshot == nil || dbSnapshot.Epoch > cachedSnapshot.Epoch {
			cachedSnapshot = dbSnapshot
			v.snapshots[dbSnapshot.Epoch] = dbSnapshot.copy()
		}
	}
	/* FINISH ALPHA CHANGE 971 -- only use database validator snapshots at or before requested epoch -- */

	return cachedSnapshot, nil
}

// getNextEpochEndingBlock gets the epoch ending block of a newer epoch
// It start checking the blocks from the provided epoch ending block of the previous epoch
func (v *validatorsSnapshotCache) getNextEpochEndingBlock(latestEpochEndingBlock uint64) (uint64, error) {
	blockNumber := latestEpochEndingBlock + 1 // get next block

	_, extra, err := getBlockData(blockNumber, v.blockchain)
	if err != nil {
		return 0, err
	}

	startEpoch := extra.Checkpoint.EpochNumber
	epoch := startEpoch

	for startEpoch == epoch {
		blockNumber++

		_, extra, err = getBlockData(blockNumber, v.blockchain)
		if err != nil {
			if errors.Is(err, blockchain.ErrNoBlock) {
				return blockNumber - 1, nil
			}

			return 0, err
		}

		epoch = extra.Checkpoint.EpochNumber
	}

	return blockNumber - 1, nil
}
