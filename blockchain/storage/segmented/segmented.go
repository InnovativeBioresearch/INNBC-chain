/* START ALPHA CHANGE 1560 -- add physically reclaimable segmented recent-history storage -- */
package segmented

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	bcstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"

	/* START ALPHA CHANGE 3455 -- enforce consistent geometry across active segments -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3455 -- enforce consistent geometry across active segments -- */
	"github.com/innovativebioresearch/innbc-chain/types"
)

const (
	/* START ALPHA CHANGE 1647 -- separate permanent genesis from disposable history segments -- */
	genesisDirectoryName  = "genesis"
	segmentsDirectoryName = "segments"
	/* FINISH ALPHA CHANGE 1647 -- separate permanent genesis from disposable history segments -- */

	segmentPrefix     = "segment-"
	deletingPrefix    = ".deleting-segment-"
	minSegmentMapSize = 64 * 1024 * 1024
)

var errBlockTargetRequired = errors.New(
	"segmented blockchain storage requires a block-targeted batch",
)

type Storage struct {
	lock sync.RWMutex

	logger          hclog.Logger
	path            string
	segmentBlocks   uint64
	retentionBlocks uint64
	segmentMapSize  int
	/* START ALPHA CHANGE 3455 -- retain creation geometry for future segments -- */
	pageSizeBytes uint64
	/* FINISH ALPHA CHANGE 3455 -- retain creation geometry for future segments -- */

	segments map[uint64]bcstorage.Storage
	starts   []uint64
	closed   bool

	/* START ALPHA CHANGE 1605 -- retain failed segment deletions for live retry -- */
	pendingDeletes map[string]struct{}
	removeAll      func(string) error
	/* FINISH ALPHA CHANGE 1605 -- retain failed segment deletions for live retry -- */
}

type batchOperation struct {
	delete bool
	key    []byte
	value  []byte
}

type segmentBatch struct {
	storage     *Storage
	blockNumber uint64
	operations  []batchOperation
}

type rejectedBatch struct{}

var _ bcstorage.Storage = (*Storage)(nil)
var _ bcstorage.BlockBatchStorage = (*Storage)(nil)
var _ bcstorage.Batch = (*segmentBatch)(nil)

func NewStorage(
	path string,
	logger hclog.Logger,
	segmentBlocks uint64,
	retentionBlocks uint64,
	totalMapSize int,
	pageSizes ...uint64,
) (*Storage, error) {
	/* START ALPHA CHANGE 3455 -- inspect every active database before writable maintenance -- */
	pageSize, err := mdbxconfig.Resolve(pageSizes...)
	if err != nil {
		return nil, err
	}
	if err := mdbxconfig.CheckTree(path, uint64(pageSize)); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3455 -- inspect every active database before writable maintenance -- */
	/* START ALPHA CHANGE 3138 -- reject unrepresentable retention before creating storage -- */
	retainedSegments, err := RetainedSegmentCount(segmentBlocks, retentionBlocks)
	if err != nil {
		return nil, err
	}

	if totalMapSize <= 0 {
		return nil, errors.New("blockchain MDBX map size must be greater than zero")
	}

	segmentMapSize := totalMapSize / retainedSegments
	if segmentMapSize < minSegmentMapSize {
		segmentMapSize = minSegmentMapSize
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create segmented blockchain directory: %w", err)
	}
	/* FINISH ALPHA CHANGE 3138 -- reject unrepresentable retention before creating storage -- */

	storage := &Storage{
		logger:          logger.Named("segmented-blockchain"),
		path:            path,
		segmentBlocks:   segmentBlocks,
		retentionBlocks: retentionBlocks,
		segmentMapSize:  segmentMapSize,
		/* START ALPHA CHANGE 3455 -- retain the resolved page size -- */
		pageSizeBytes: uint64(pageSize),
		/* FINISH ALPHA CHANGE 3455 -- retain the resolved page size -- */
		segments: make(map[uint64]bcstorage.Storage),
		/* START ALPHA CHANGE 1606 -- initialize live segment deletion maintenance -- */
		pendingDeletes: make(map[string]struct{}),
		removeAll:      os.RemoveAll,
		/* FINISH ALPHA CHANGE 1606 -- initialize live segment deletion maintenance -- */
	}

	if err := storage.loadSegments(); err != nil {
		_ = storage.Close()

		return nil, err
	}

	return storage, nil
}

/* START ALPHA CHANGE 1648 -- load permanent genesis and disposable segments from separate directories -- */
func (s *Storage) loadSegments() error {
	rootEntries, err := os.ReadDir(s.path)
	if err != nil {
		return fmt.Errorf("failed to read segmented blockchain directory: %w", err)
	}

	for _, entry := range rootEntries {
		if entry.Name() != genesisDirectoryName && entry.Name() != segmentsDirectoryName {
			return fmt.Errorf(
				"unexpected entry in segmented blockchain directory: %s",
				entry.Name(),
			)
		}

		if !entry.IsDir() {
			return fmt.Errorf(
				"segmented blockchain layout entry is not a directory: %s",
				entry.Name(),
			)
		}
	}

	if err := os.MkdirAll(s.genesisPath(), 0755); err != nil {
		return fmt.Errorf("failed to create permanent genesis database directory: %w", err)
	}

	if err := os.MkdirAll(s.segmentsPath(), 0755); err != nil {
		return fmt.Errorf("failed to create blockchain segments directory: %w", err)
	}

	/* START ALPHA CHANGE 3455 -- apply selected geometry to permanent genesis storage -- */
	genesisStorage, err := mdbxstorage.NewMDBXStorageWithMapSize(
		s.genesisPath(),
		s.logger,
		s.segmentMapSize,
		s.pageSizeBytes,
	)
	/* FINISH ALPHA CHANGE 3455 -- apply selected geometry to permanent genesis storage -- */
	if err != nil {
		return fmt.Errorf("failed to open permanent genesis database: %w", err)
	}

	s.segments[0] = genesisStorage
	s.starts = append(s.starts, 0)

	entries, err := os.ReadDir(s.segmentsPath())
	if err != nil {
		return fmt.Errorf("failed to read blockchain segments directory: %w", err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), deletingPrefix) {
			if err := s.removeAll(filepath.Join(s.segmentsPath(), entry.Name())); err != nil {
				return fmt.Errorf("failed to remove interrupted retired segment %q: %w", entry.Name(), err)
			}

			continue
		}

		if !entry.IsDir() {
			return fmt.Errorf("unexpected file in blockchain segments directory: %s", entry.Name())
		}

		start, end, ok := parseSegmentName(entry.Name())
		if !ok {
			return fmt.Errorf("unexpected directory in blockchain segments directory: %s", entry.Name())
		}

		/* START ALPHA CHANGE 1583 -- reject segments incompatible with configured geometry -- */
		if s.segmentStart(start) != start {
			return fmt.Errorf(
				"blockchain segment %q is incompatible with configured segment size %d",
				entry.Name(),
				s.segmentBlocks,
			)
		}

		if end != s.segmentEnd(start) {
			return fmt.Errorf(
				"blockchain segment %q has invalid end block: expected %d",
				entry.Name(),
				s.segmentEnd(start),
			)
		}
		/* FINISH ALPHA CHANGE 1583 -- reject segments incompatible with configured geometry -- */

		if _, exists := s.segments[start]; exists {
			return fmt.Errorf("duplicate blockchain segment start %d", start)
		}

		/* START ALPHA CHANGE 3455 -- enforce selected geometry when reopening segments -- */
		segment, err := mdbxstorage.NewMDBXStorageWithMapSize(
			s.segmentPath(start),
			s.logger,
			s.segmentMapSize,
			s.pageSizeBytes,
		)
		/* FINISH ALPHA CHANGE 3455 -- enforce selected geometry when reopening segments -- */
		if err != nil {
			return fmt.Errorf("failed to open blockchain segment %q: %w", entry.Name(), err)
		}

		s.segments[start] = segment
		s.starts = append(s.starts, start)
	}

	sort.Slice(s.starts, func(i, j int) bool {
		return s.starts[i] < s.starts[j]
	})

	return nil
}

/* FINISH ALPHA CHANGE 1648 -- load permanent genesis and disposable segments from separate directories -- */

func (s *Storage) ReadCanonicalHash(number uint64) (types.Hash, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	segment := s.segments[s.segmentStart(number)]
	if segment == nil {
		return types.Hash{}, false
	}

	return segment.ReadCanonicalHash(number)
}

/* START ALPHA CHANGE 1659 -- derive advertised history floor from retained segments -- */
func (s *Storage) HistoryFrom() uint64 {
	s.lock.RLock()
	defer s.lock.RUnlock()

	for _, start := range s.starts {
		if start == 0 {
			continue
		}

		// A first segment beginning at block 1 is continuous with the
		// separately retained genesis block.
		if start == 1 {
			return 0
		}

		return start
	}

	// A node containing only genesis cannot serve block 1, but its advertised
	// head is also zero, so it will not qualify for that request.
	return 0
}

/* FINISH ALPHA CHANGE 1659 -- derive advertised history floor from retained segments -- */

func (s *Storage) ReadHeadHash() (types.Hash, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	segment, _, ok := s.headSegmentLocked()
	if !ok {
		return types.Hash{}, false
	}

	return segment.ReadHeadHash()
}

func (s *Storage) ReadHeadNumber() (uint64, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, number, ok := s.headSegmentLocked()

	return number, ok
}

func (s *Storage) ReadHeader(hash types.Hash) (*types.Header, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	segment, err := s.findHeaderSegmentLocked(hash)
	if err != nil {
		return nil, err
	}

	return segment.ReadHeader(hash)
}

func (s *Storage) ReadBody(hash types.Hash) (*types.Body, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	segment, err := s.findHeaderSegmentLocked(hash)
	if err != nil {
		return nil, err
	}

	return segment.ReadBody(hash)
}

func (s *Storage) ReadReceipts(hash types.Hash) ([]*types.Receipt, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	segment, err := s.findHeaderSegmentLocked(hash)
	if err != nil {
		return nil, err
	}

	return segment.ReadReceipts(hash)
}

func (s *Storage) ReadTxLookup(hash types.Hash) (types.Hash, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	for index := len(s.starts) - 1; index >= 0; index-- {
		segment := s.segments[s.starts[index]]
		if blockHash, ok := segment.ReadTxLookup(hash); ok {
			return blockHash, true
		}
	}

	return types.Hash{}, false
}

func (s *Storage) NewBatch() bcstorage.Batch {
	return &rejectedBatch{}
}

func (s *Storage) NewBatchForBlock(blockNumber uint64) bcstorage.Batch {
	return &segmentBatch{
		storage:     s,
		blockNumber: blockNumber,
	}
}

func (s *Storage) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	var closeErr error
	for _, start := range s.starts {
		if err := s.segments[start].Close(); err != nil {
			closeErr = errors.Join(
				closeErr,
				fmt.Errorf("failed to close blockchain segment %d: %w", start, err),
			)
		}
	}

	s.segments = nil
	s.starts = nil

	return closeErr
}

func (s *Storage) segmentStart(blockNumber uint64) uint64 {
	if blockNumber == 0 {
		return 0
	}

	return ((blockNumber - 1) / s.segmentBlocks * s.segmentBlocks) + 1
}

func (s *Storage) segmentEnd(start uint64) uint64 {
	if start == 0 {
		return 0
	}

	return start + s.segmentBlocks - 1
}

func (s *Storage) segmentName(start uint64) string {
	return fmt.Sprintf(
		"%s%020d-%020d",
		segmentPrefix,
		start,
		s.segmentEnd(start),
	)
}

/* START ALPHA CHANGE 1649 -- resolve permanent genesis and disposable segment paths -- */
func (s *Storage) genesisPath() string {
	return filepath.Join(s.path, genesisDirectoryName)
}

func (s *Storage) segmentsPath() string {
	return filepath.Join(s.path, segmentsDirectoryName)
}

func (s *Storage) segmentPath(start uint64) string {
	if start == 0 {
		return s.genesisPath()
	}

	return filepath.Join(s.segmentsPath(), s.segmentName(start))
}

/* FINISH ALPHA CHANGE 1649 -- resolve permanent genesis and disposable segment paths -- */

func parseSegmentName(name string) (uint64, uint64, bool) {
	var start uint64
	var end uint64

	count, err := fmt.Sscanf(name, segmentPrefix+"%020d-%020d", &start, &end)
	if err != nil || count != 2 {
		return 0, 0, false
	}

	if fmt.Sprintf(segmentPrefix+"%020d-%020d", start, end) != name {
		return 0, 0, false
	}

	/* START ALPHA CHANGE 1650 -- reserve block zero for dedicated genesis storage -- */
	return start, end, start > 0 && end >= start
	/* FINISH ALPHA CHANGE 1650 -- reserve block zero for dedicated genesis storage -- */
}

func (s *Storage) headSegmentLocked() (bcstorage.Storage, uint64, bool) {
	var headSegment bcstorage.Storage
	var headNumber uint64
	found := false

	for _, start := range s.starts {
		segment := s.segments[start]
		number, ok := segment.ReadHeadNumber()
		if !ok {
			continue
		}

		if !found || number > headNumber {
			headSegment = segment
			headNumber = number
			found = true
		}
	}

	return headSegment, headNumber, found
}

func (s *Storage) findHeaderSegmentLocked(hash types.Hash) (bcstorage.Storage, error) {
	for index := len(s.starts) - 1; index >= 0; index-- {
		segment := s.segments[s.starts[index]]

		if _, err := segment.ReadHeader(hash); err == nil {
			return segment, nil
		} else if !errors.Is(err, bcstorage.ErrNotFound) {
			return nil, err
		}
	}

	return nil, bcstorage.ErrNotFound
}

func (s *Storage) openSegmentLocked(start uint64) (bcstorage.Storage, error) {
	if segment := s.segments[start]; segment != nil {
		return segment, nil
	}

	/* START ALPHA CHANGE 1651 -- create genesis and history databases at their designated paths -- */
	/* START ALPHA CHANGE 3455 -- inherit page size when allocating subsequent segments -- */
	segment, err := mdbxstorage.NewMDBXStorageWithMapSize(
		s.segmentPath(start),
		s.logger,
		s.segmentMapSize,
		s.pageSizeBytes,
	)
	/* FINISH ALPHA CHANGE 3455 -- inherit page size when allocating subsequent segments -- */
	/* FINISH ALPHA CHANGE 1651 -- create genesis and history databases at their designated paths -- */
	if err != nil {
		return nil, fmt.Errorf("failed to create blockchain segment %d: %w", start, err)
	}

	s.segments[start] = segment
	s.starts = append(s.starts, start)

	sort.Slice(s.starts, func(i, j int) bool {
		return s.starts[i] < s.starts[j]
	})

	return segment, nil
}

/* START ALPHA CHANGE 1607 -- retry failed retired segment deletion while online -- */
func (s *Storage) cleanupPendingDeletesLocked() {
	for path := range s.pendingDeletes {
		if err := s.removeAll(path); err != nil {
			// The original failure is logged by pruneLocked. Subsequent
			// failures stay at debug level to prevent one filesystem problem
			// from producing an error log for every committed block.
			s.logger.Debug(
				"retired blockchain segment still awaiting physical deletion",
				"path", path,
				"err", err,
			)

			continue
		}

		delete(s.pendingDeletes, path)

		s.logger.Info(
			"retired blockchain segment physical deletion completed",
			"path", path,
		)
	}
}

/* FINISH ALPHA CHANGE 1607 -- retry failed retired segment deletion while online -- */

func (s *Storage) pruneLocked(activeStart uint64) {
	maxSegments := int(s.retentionBlocks / s.segmentBlocks)

	for {
		nonGenesis := make([]uint64, 0, len(s.starts))
		for _, start := range s.starts {
			if start != 0 {
				nonGenesis = append(nonGenesis, start)
			}
		}

		if len(nonGenesis) <= maxSegments {
			return
		}

		oldest := nonGenesis[0]
		if oldest == activeStart {
			return
		}

		if err := s.retireSegmentLocked(oldest); err != nil {
			s.logger.Error(
				"failed to retire expired blockchain segment",
				"segmentStart", oldest,
				"err", err,
			)

			return
		}
	}
}

func (s *Storage) retireSegmentLocked(start uint64) error {
	segment := s.segments[start]
	if segment == nil {
		return nil
	}

	name := s.segmentName(start)

	/* START ALPHA CHANGE 1652 -- retire expired history only inside segments directory -- */
	path := s.segmentPath(start)
	retiredPath := filepath.Join(s.segmentsPath(), deletingPrefix+name)
	/* FINISH ALPHA CHANGE 1652 -- retire expired history only inside segments directory -- */

	if err := os.Rename(path, retiredPath); err != nil {
		return fmt.Errorf("failed to mark segment for deletion: %w", err)
	}

	if err := segment.Close(); err != nil {
		if renameErr := os.Rename(retiredPath, path); renameErr != nil {
			return fmt.Errorf(
				"failed to close retired segment: %w; additionally failed to restore its path: %v",
				err,
				renameErr,
			)
		}

		return fmt.Errorf("failed to close retired segment: %w", err)
	}

	delete(s.segments, start)

	for index, existingStart := range s.starts {
		if existingStart == start {
			s.starts = append(s.starts[:index], s.starts[index+1:]...)

			break
		}
	}

	if err := s.removeAll(retiredPath); err != nil {
		/* START ALPHA CHANGE 1608 -- schedule failed physical deletion for live retry -- */
		s.pendingDeletes[retiredPath] = struct{}{}
		/* FINISH ALPHA CHANGE 1608 -- schedule failed physical deletion for live retry -- */

		return fmt.Errorf("failed to remove retired segment directory: %w", err)
	}

	s.logger.Info(
		"expired blockchain history segment removed",
		"segmentStart", start,
		"segmentEnd", s.segmentEnd(start),
	)

	return nil
}

func (b *segmentBatch) Put(key []byte, value []byte) {
	b.operations = append(b.operations, batchOperation{
		key:   append([]byte(nil), key...),
		value: append([]byte(nil), value...),
	})
}

func (b *segmentBatch) Delete(key []byte) {
	b.operations = append(b.operations, batchOperation{
		delete: true,
		key:    append([]byte(nil), key...),
	})
}

func (b *segmentBatch) Write() error {
	b.storage.lock.Lock()
	defer b.storage.lock.Unlock()

	if b.storage.closed {
		return errors.New("segmented blockchain storage is closed")
	}

	start := b.storage.segmentStart(b.blockNumber)

	segment, err := b.storage.openSegmentLocked(start)
	if err != nil {
		return err
	}

	batch := segment.NewBatch()

	for _, operation := range b.operations {
		if operation.delete {
			batch.Delete(operation.key)

			continue
		}

		batch.Put(operation.key, operation.value)
	}

	if err := batch.Write(); err != nil {
		return fmt.Errorf("failed to write blockchain segment %d: %w", start, err)
	}

	/* START ALPHA CHANGE 1609 -- retry pending removals after successful live commits -- */
	b.storage.cleanupPendingDeletesLocked()
	/* FINISH ALPHA CHANGE 1609 -- retry pending removals after successful live commits -- */

	// Segment retirement is maintenance after the canonical block commit. A
	// failed directory cleanup must not report the already-committed block as
	// failed. Failed physical removals remain registered and are retried after
	// subsequent successful block commits without restarting the node.
	b.storage.pruneLocked(start)

	return nil
}

func (b *rejectedBatch) Put([]byte, []byte) {}

func (b *rejectedBatch) Delete([]byte) {}

func (b *rejectedBatch) Write() error {
	return errBlockTargetRequired
}

/* FINISH ALPHA CHANGE 1560 -- add physically reclaimable segmented recent-history storage -- */
