package storage

import (
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/types"
)

/* START ALPHA CHANGE 3165 -- require explicit startup storage inspection -- */

// StartupState describes one database's validated startup metadata.
// Empty is true only when the backend confirms there are no stored records.
// Individual non-genesis segment databases do not contain a genesis mapping.
type StartupState struct {
	Empty       bool
	Head        *types.Header
	GenesisHash types.Hash
	HasGenesis  bool
}

// Storage is a generic blockchain storage
type Storage interface {
	ReadStartupState() (StartupState, error)
	/* FINISH ALPHA CHANGE 3165 -- require explicit startup storage inspection -- */

	ReadCanonicalHash(n uint64) (types.Hash, bool)

	ReadHeadHash() (types.Hash, bool)
	ReadHeadNumber() (uint64, bool)

	/* START ALPHA CHANGE 779 -- remove fork and total-difficulty storage interface methods -- */
	ReadHeader(hash types.Hash) (*types.Header, error)
	/* FINISH ALPHA CHANGE 779 -- remove fork and total-difficulty storage interface methods -- */

	ReadBody(hash types.Hash) (*types.Body, error)

	ReadReceipts(hash types.Hash) ([]*types.Receipt, error)

	ReadTxLookup(hash types.Hash) (types.Hash, bool)

	NewBatch() Batch

	Close() error
}

/* START ALPHA CHANGE 1553 -- support block-targeted batches for segmented recent history -- */
// BlockBatchStorage is implemented by storage backends that need the canonical
// block number before selecting the physical database that owns a write.
type BlockBatchStorage interface {
	NewBatchForBlock(blockNumber uint64) Batch
}

/* FINISH ALPHA CHANGE 1553 -- support block-targeted batches for segmented recent history -- */

/* START ALPHA CHANGE 1658 -- expose the earliest continuously servable block -- */
// HistoryAvailabilityStorage is implemented by storage backends whose retained
// canonical history does not necessarily begin at genesis.
type HistoryAvailabilityStorage interface {
	HistoryFrom() uint64
}

/* FINISH ALPHA CHANGE 1658 -- expose the earliest continuously servable block -- */

// Factory is a factory method to create a blockchain storage
type Factory func(config map[string]interface{}, logger hclog.Logger) (Storage, error)
