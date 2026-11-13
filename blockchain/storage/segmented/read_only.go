/* START ALPHA CHANGE 3186 -- inspect existing segmented storage without writes or retirement maintenance -- */
package segmented

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/go-hclog"
	bcstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
)

// NewStorageReadOnly opens an existing segmented layout using each database's
// stored geometry. It does not create layout components or remove retired paths.
// Missing or incomplete layouts are errors; callers classify fresh paths before
// invoking this constructor.
func NewStorageReadOnly(
	path string,
	logger hclog.Logger,
	segmentBlocks uint64,
	retentionBlocks uint64,
) (bcstorage.Storage, error) {
	if _, err := RetainedSegmentCount(segmentBlocks, retentionBlocks); err != nil {
		return nil, err
	}

	if logger == nil {
		logger = hclog.NewNullLogger()
	}

	store := &Storage{
		logger:          logger.Named("segmented-blockchain"),
		path:            path,
		segmentBlocks:   segmentBlocks,
		retentionBlocks: retentionBlocks,
		segments:        make(map[uint64]bcstorage.Storage),
	}

	if err := store.loadReadOnlySegments(); err != nil {
		return nil, errors.Join(err, store.Close())
	}

	return &readOnlyStorage{Storage: store}, nil
}

func (s *Storage) loadReadOnlySegments() error {
	rootEntries, err := os.ReadDir(s.path)
	if err != nil {
		return fmt.Errorf("read existing segmented blockchain directory: %w", err)
	}

	haveGenesis := false
	haveSegments := false

	for _, entry := range rootEntries {
		if !entry.IsDir() {
			return fmt.Errorf(
				"segmented blockchain layout entry is not a directory: %s",
				entry.Name(),
			)
		}

		switch entry.Name() {
		case genesisDirectoryName:
			haveGenesis = true
		case segmentsDirectoryName:
			haveSegments = true
		default:
			return fmt.Errorf(
				"unexpected entry in segmented blockchain directory: %s",
				entry.Name(),
			)
		}
	}

	if !haveGenesis {
		return errors.New("existing segmented layout is missing its permanent genesis directory")
	}
	if !haveSegments {
		return errors.New("existing segmented layout is missing its segments directory")
	}

	genesis, err := mdbxstorage.NewMDBXStorageReadOnly(s.genesisPath(), s.logger)
	if err != nil {
		return fmt.Errorf("open permanent genesis database read-only: %w", err)
	}
	s.segments[0] = genesis
	s.starts = append(s.starts, 0)

	entries, err := os.ReadDir(s.segmentsPath())
	if err != nil {
		return fmt.Errorf("read existing blockchain segments directory: %w", err)
	}

	for _, entry := range entries {
		// Retired paths no longer belong to live history. Leave their cleanup
		// to the ordinary writable opener.
		if strings.HasPrefix(entry.Name(), deletingPrefix) {
			continue
		}

		if !entry.IsDir() {
			return fmt.Errorf(
				"unexpected file in blockchain segments directory: %s",
				entry.Name(),
			)
		}

		start, end, ok := parseSegmentName(entry.Name())
		if !ok {
			return fmt.Errorf(
				"unexpected directory in blockchain segments directory: %s",
				entry.Name(),
			)
		}

		// parseSegmentName establishes end >= start. Subtraction avoids
		// overflowing while checking an extreme segment's ending height.
		if s.segmentStart(start) != start || end-start != s.segmentBlocks-1 {
			return fmt.Errorf(
				"blockchain segment %q has incompatible segment geometry",
				entry.Name(),
			)
		}

		if _, exists := s.segments[start]; exists {
			return fmt.Errorf("duplicate blockchain segment start %d", start)
		}

		segment, err := mdbxstorage.NewMDBXStorageReadOnly(
			filepath.Join(s.segmentsPath(), entry.Name()),
			s.logger,
		)
		if err != nil {
			return fmt.Errorf(
				"open blockchain segment %q read-only: %w",
				entry.Name(),
				err,
			)
		}

		s.segments[start] = segment
		s.starts = append(s.starts, start)
	}

	/* START ALPHA CHANGE 3192 -- modernize ascending segment-start sorting without changing ordering -- */
	slices.Sort(s.starts)
	/* FINISH ALPHA CHANGE 3192 -- modernize ascending segment-start sorting without changing ordering -- */

	return nil
}

// The wrapper rejects writes without adding a branch to ordinary storage writes.
type readOnlyStorage struct {
	*Storage
}

func (*readOnlyStorage) NewBatch() bcstorage.Batch {
	return &readOnlyBatch{}
}

func (*readOnlyStorage) NewBatchForBlock(uint64) bcstorage.Batch {
	return &readOnlyBatch{}
}

type readOnlyBatch struct{}

func (*readOnlyBatch) Put([]byte, []byte) {}

func (*readOnlyBatch) Delete([]byte) {}

func (*readOnlyBatch) Write() error {
	return errors.New("segmented blockchain storage is read-only")
}

/* FINISH ALPHA CHANGE 3186 -- inspect existing segmented storage without writes or retirement maintenance -- */
