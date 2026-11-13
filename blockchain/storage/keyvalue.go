//nolint:stylecheck
package storage

import (
	"errors"
	"fmt"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/fastrlp"
)

// Prefixes for the key-value store
var (
	/* START ALPHA CHANGE 777 -- remove total-difficulty and fork storage prefixes -- */
	// HEADER is the header prefix
	HEADER = []byte("h")

	// HEAD is the chain head prefix
	HEAD = []byte("o")
	/* FINISH ALPHA CHANGE 777 -- remove total-difficulty and fork storage prefixes -- */

	// CANONICAL is the prefix for the canonical chain numbers
	CANONICAL = []byte("c")

	// BODY is the prefix for bodies
	BODY = []byte("b")

	// RECEIPTS is the prefix for receipts
	RECEIPTS = []byte("r")

	/* START ALPHA CHANGE 861 -- remove unused legacy snapshot storage prefix -- */
	/* FINISH ALPHA CHANGE 861 -- remove unused legacy snapshot storage prefix -- */

	// TX_LOOKUP_PREFIX is the prefix for transaction lookups
	TX_LOOKUP_PREFIX = []byte("l")
)

// Sub-prefixes
var (
	HASH   = []byte("hash")
	NUMBER = []byte("number")
	EMPTY  = []byte("empty")
)

// KV is a key value storage interface.
//
// KV = Key-Value
type KV interface {
	Close() error
	Get(p []byte) ([]byte, bool, error)

	/* START ALPHA CHANGE 3165 -- distinguish an empty database from missing metadata -- */
	IsEmpty() (bool, error)
	/* FINISH ALPHA CHANGE 3165 -- distinguish an empty database from missing metadata -- */

	NewBatch() Batch
}

// KeyValueStorage is a generic storage for kv databases
type KeyValueStorage struct {
	logger hclog.Logger
	db     KV
	Db     KV

	/* START ALPHA CHANGE 1661 -- cache database-derived standard history availability -- */
	historyFromOnce sync.Once
	historyFrom     uint64
	/* FINISH ALPHA CHANGE 1661 -- cache database-derived standard history availability -- */
}

func NewKeyValueStorage(logger hclog.Logger, db KV) Storage {
	storage := &KeyValueStorage{logger: logger, db: db}

	/* START ALPHA CHANGE 1662 -- initialize standard history availability when storage opens -- */
	storage.initializeHistoryFrom()
	/* FINISH ALPHA CHANGE 1662 -- initialize standard history availability when storage opens -- */

	return storage
}

// -- canonical hash --

// ReadCanonicalHash gets the hash from the number of the canonical chain
func (s *KeyValueStorage) ReadCanonicalHash(n uint64) (types.Hash, bool) {
	data, ok := s.get(CANONICAL, common.EncodeUint64ToBytes(n))
	if !ok {
		return types.Hash{}, false
	}

	return types.BytesToHash(data), true
}

/* START ALPHA CHANGE 1663 -- derive the continuous standard-storage history floor -- */
func (s *KeyValueStorage) initializeHistoryFrom() {
	s.historyFromOnce.Do(func() {
		s.historyFrom = s.calculateHistoryFrom()
	})
}

func (s *KeyValueStorage) calculateHistoryFrom() uint64 {
	head, ok := s.ReadHeadNumber()
	if !ok || head == 0 {
		return 0
	}

	// A canonical block 1 identifies an ordinary genesis-origin database.
	// Standard storage never deletes individual canonical blocks.
	if _, ok := s.ReadCanonicalHash(1); ok {
		return 0
	}

	if head == 1 {
		return 1
	}

	// A checkpoint-origin database contains genesis separately, followed by one
	// missing range and then one continuous retained suffix ending at the head.
	// Find the first block in that suffix without scanning the complete chain.
	low := uint64(2)
	high := head

	for low < high {
		mid := low + (high-low)/2

		if _, ok := s.ReadCanonicalHash(mid); ok {
			high = mid
		} else {
			low = mid + 1
		}
	}

	if _, ok := s.ReadCanonicalHash(low); !ok {
		// The recorded head must itself have a canonical mapping. Returning the
		// head is safer than advertising unavailable history from genesis.
		return head
	}

	return low
}

func (s *KeyValueStorage) HistoryFrom() uint64 {
	// The constructor initializes this eagerly. Keeping the same guarded call
	// here provides a safe lazy fallback for any directly constructed storage.
	s.initializeHistoryFrom()

	return s.historyFrom
}

/* FINISH ALPHA CHANGE 1663 -- derive the continuous standard-storage history floor -- */

// HEAD //

// ReadHeadHash returns the hash of the head
func (s *KeyValueStorage) ReadHeadHash() (types.Hash, bool) {
	data, ok := s.get(HEAD, HASH)
	if !ok {
		return types.Hash{}, false
	}

	return types.BytesToHash(data), true
}

// ReadHeadNumber returns the number of the head
func (s *KeyValueStorage) ReadHeadNumber() (uint64, bool) {
	data, ok := s.get(HEAD, NUMBER)
	if !ok {
		return 0, false
	}

	if len(data) != 8 {
		return 0, false
	}

	return common.EncodeBytesToUint64(data), true
}

/* START ALPHA CHANGE 778 -- prune fork and total-difficulty readers -- */
// HEADER //
/* FINISH ALPHA CHANGE 778 -- prune fork and total-difficulty readers -- */

// ReadHeader reads the header
func (s *KeyValueStorage) ReadHeader(hash types.Hash) (*types.Header, error) {
	header := &types.Header{}
	if err := s.readRLP(HEADER, hash.Bytes(), header); err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 1484 -- verify reconstructed compact header against its storage key -- */
	if header.Hash != hash {
		return nil, fmt.Errorf(
			"%w: requested=%s reconstructed=%s",
			ErrHeaderHashMismatch,
			hash,
			header.Hash,
		)
	}
	/* FINISH ALPHA CHANGE 1484 -- verify reconstructed compact header against its storage key -- */

	return header, nil
}

// BODY //

// ReadBody reads the body
func (s *KeyValueStorage) ReadBody(hash types.Hash) (*types.Body, error) {
	body := &types.Body{}
	bodyErr := s.readRLP(BODY, hash.Bytes(), body)

	/* START ALPHA CHANGE 1473 -- reconstruct omitted empty block bodies from header commitments -- */
	if bodyErr != nil && !errors.Is(bodyErr, ErrNotFound) {
		return nil, bodyErr
	}

	// The header is required both to distinguish an intentionally omitted empty
	// body from missing non-empty data and to calculate transaction hashes.
	/* START ALPHA CHANGE 1485 -- use hash-verified compact header for body reconstruction -- */
	header, err := s.ReadHeader(hash)
	if err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 1485 -- use hash-verified compact header for body reconstruction -- */

	if errors.Is(bodyErr, ErrNotFound) {
		if header.HasBody() {
			return nil, bodyErr
		}

		return &types.Body{}, nil
	}
	/* FINISH ALPHA CHANGE 1473 -- reconstruct omitted empty block bodies from header commitments -- */

	for _, tx := range body.Transactions {
		tx.ComputeHash(header.Number)
	}

	return body, nil
}

// RECEIPTS //

// ReadReceipts reads the receipts
func (s *KeyValueStorage) ReadReceipts(hash types.Hash) ([]*types.Receipt, error) {
	receipts := &types.Receipts{}
	receiptsErr := s.readRLP(RECEIPTS, hash.Bytes(), receipts)

	/* START ALPHA CHANGE 1474 -- reconstruct omitted empty receipts from header commitments -- */
	if receiptsErr == nil {
		return *receipts, nil
	}

	if !errors.Is(receiptsErr, ErrNotFound) {
		return nil, receiptsErr
	}

	/* START ALPHA CHANGE 1486 -- use hash-verified compact header for receipt reconstruction -- */
	header, err := s.ReadHeader(hash)
	if err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 1486 -- use hash-verified compact header for receipt reconstruction -- */

	if header.HasReceipts() {
		return nil, receiptsErr
	}

	return []*types.Receipt{}, nil
	/* FINISH ALPHA CHANGE 1474 -- reconstruct omitted empty receipts from header commitments -- */
}

// TX LOOKUP //

// ReadTxLookup reads the block hash using the transaction hash
func (s *KeyValueStorage) ReadTxLookup(hash types.Hash) (types.Hash, bool) {
	parser := &fastrlp.Parser{}

	v := s.read2(TX_LOOKUP_PREFIX, hash.Bytes(), parser)
	if v == nil {
		return types.Hash{}, false
	}

	blockHash := []byte{}
	blockHash, err := v.GetBytes(blockHash[:0], 32)

	if err != nil {
		return types.Hash{}, false
	}

	return types.BytesToHash(blockHash), true
}

var (
	ErrNotFound = fmt.Errorf("not found")

	/* START ALPHA CHANGE 1483 -- identify compact stored header hash mismatches -- */
	ErrHeaderHashMismatch = fmt.Errorf("stored header hash mismatch")
	/* FINISH ALPHA CHANGE 1483 -- identify compact stored header hash mismatches -- */
)

func (s *KeyValueStorage) readRLP(p, k []byte, raw types.RLPUnmarshaler) error {
	p = append(p, k...)
	data, ok, err := s.db.Get(p)

	if err != nil {
		return err
	}

	if !ok {
		return ErrNotFound
	}

	if obj, ok := raw.(types.RLPStoreUnmarshaler); ok {
		// decode in the store format
		if err := obj.UnmarshalStoreRLP(data); err != nil {
			return err
		}
	} else {
		// normal rlp decoding
		if err := raw.UnmarshalRLP(data); err != nil {
			return err
		}
	}

	return nil
}

func (s *KeyValueStorage) read2(p, k []byte, parser *fastrlp.Parser) *fastrlp.Value {
	data, ok := s.get(p, k)
	if !ok {
		return nil
	}

	v, err := parser.Parse(data)
	if err != nil {
		return nil
	}

	return v
}

func (s *KeyValueStorage) get(p []byte, k []byte) ([]byte, bool) {
	p = append(p, k...)
	data, ok, err := s.db.Get(p)

	if err != nil {
		return nil, false
	}

	return data, ok
}

// Close closes the connection with the db
func (s *KeyValueStorage) Close() error {
	return s.db.Close()
}

// NewBatch creates batch used for write/update/delete operations
func (s *KeyValueStorage) NewBatch() Batch {
	return s.db.NewBatch()
}
