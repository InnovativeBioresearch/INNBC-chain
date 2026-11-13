/* START ALPHA CHANGE 2065 -- encode bounded immutable transaction indexes -- */
package snapshotbuilder

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	"github.com/innovativebioresearch/innbc-chain/types"
)

const (
	transactionIndexMagic      = "INNBTXI1"
	transactionIndexHeaderSize = 32
	transactionIndexRecordSize = 44
)

type TransactionIndexRecord struct {
	Hash             types.Hash
	BlockNumber      uint64
	TransactionIndex uint32
}

/* START ALPHA CHANGE 2071 -- share index allocation policy across builders and readers -- */
func validateArchiveIndexLimit(maxRecords uint64) error {
	if maxRecords == 0 || maxRecords > MaximumArchiveIndexRecords {
		return fmt.Errorf(
			"archive maximum index records must be between 1 and %d, got %d",
			MaximumArchiveIndexRecords,
			maxRecords,
		)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2071 -- share index allocation policy across builders and readers -- */

func writeTransactionIndex(
	path string,
	records []TransactionIndexRecord,
	maxRecords uint64,
) error {
	/* START ALPHA CHANGE 2071 -- reject unsupported writer limits before sorting or writing -- */
	if err := validateArchiveIndexLimit(maxRecords); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2071 -- reject unsupported writer limits before sorting or writing -- */

	if uint64(len(records)) > maxRecords {
		return fmt.Errorf(
			"transaction index contains %d records, exceeding configured maximum %d",
			len(records),
			maxRecords,
		)
	}

	sort.Slice(records, func(left, right int) bool {
		return bytes.Compare(
			records[left].Hash[:],
			records[right].Hash[:],
		) < 0
	})

	for index := 1; index < len(records); index++ {
		if records[index-1].Hash == records[index].Hash {
			return fmt.Errorf(
				"duplicate archived transaction hash %s",
				records[index].Hash,
			)
		}
	}

	file, err := os.OpenFile(
		path,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0660,
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction index: %w", err)
	}

	removeFile := true
	defer func() {
		if removeFile {
			_ = os.Remove(path)
		}
	}()

	writer := bufio.NewWriterSize(file, 256*1024)

	header := make([]byte, transactionIndexHeaderSize)
	copy(header[:8], transactionIndexMagic)
	binary.BigEndian.PutUint64(header[8:16], TransactionIndexVersion)
	binary.BigEndian.PutUint64(header[16:24], uint64(len(records)))

	if _, err := writer.Write(header); err != nil {
		_ = file.Close()

		return fmt.Errorf("failed to write transaction index header: %w", err)
	}

	recordBuffer := make([]byte, transactionIndexRecordSize)

	for _, record := range records {
		copy(recordBuffer[:32], record.Hash[:])
		binary.BigEndian.PutUint64(recordBuffer[32:40], record.BlockNumber)
		binary.BigEndian.PutUint32(
			recordBuffer[40:44],
			record.TransactionIndex,
		)

		if _, err := writer.Write(recordBuffer); err != nil {
			_ = file.Close()

			return fmt.Errorf("failed to write transaction index record: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		_ = file.Close()

		return fmt.Errorf("failed to flush transaction index: %w", err)
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()

		return fmt.Errorf("failed to synchronize transaction index: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close transaction index: %w", err)
	}

	removeFile = false

	return nil
}

func readTransactionIndex(
	path string,
	maxRecords uint64,
) ([]TransactionIndexRecord, error) {
	/* START ALPHA CHANGE 2071 -- reject unsupported reader limits before allocating records -- */
	if err := validateArchiveIndexLimit(maxRecords); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2071 -- reject unsupported reader limits before allocating records -- */

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open transaction index: %w", err)
	}
	defer file.Close() //nolint:errcheck

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect transaction index: %w", err)
	}

	if info.Size() < transactionIndexHeaderSize {
		return nil, errors.New("transaction index is smaller than its header")
	}

	/* START ALPHA CHANGE 2072 -- buffer sequential transaction index reads -- */
	reader := bufio.NewReaderSize(file, 256*1024)

	header := make([]byte, transactionIndexHeaderSize)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, fmt.Errorf("failed to read transaction index header: %w", err)
	}
	/* FINISH ALPHA CHANGE 2072 -- buffer sequential transaction index reads -- */

	if string(header[:8]) != transactionIndexMagic {
		return nil, errors.New("transaction index magic is invalid")
	}

	version := binary.BigEndian.Uint64(header[8:16])
	if version != TransactionIndexVersion {
		return nil, fmt.Errorf(
			"unsupported transaction index version %d",
			version,
		)
	}

	recordCount := binary.BigEndian.Uint64(header[16:24])
	if recordCount > maxRecords {
		return nil, fmt.Errorf(
			"transaction index declares %d records, exceeding maximum %d",
			recordCount,
			maxRecords,
		)
	}

	if !bytes.Equal(header[24:32], make([]byte, 8)) {
		return nil, errors.New("transaction index reserved header bytes are not zero")
	}

	if recordCount > (math.MaxInt64-transactionIndexHeaderSize)/
		transactionIndexRecordSize {
		return nil, errors.New("transaction index size overflows int64")
	}

	expectedSize := int64(transactionIndexHeaderSize) +
		int64(recordCount)*transactionIndexRecordSize
	if info.Size() != expectedSize {
		return nil, fmt.Errorf(
			"transaction index size mismatch: expected %d, got %d",
			expectedSize,
			info.Size(),
		)
	}

	if recordCount > uint64(math.MaxInt) {
		return nil, errors.New("transaction index record count exceeds platform limit")
	}

	records := make([]TransactionIndexRecord, int(recordCount))
	recordBuffer := make([]byte, transactionIndexRecordSize)

	for index := range records {
		/* START ALPHA CHANGE 2072 -- consume index records through the shared buffer -- */
		if _, err := io.ReadFull(reader, recordBuffer); err != nil {
			return nil, fmt.Errorf(
				"failed to read transaction index record %d: %w",
				index,
				err,
			)
		}
		/* FINISH ALPHA CHANGE 2072 -- consume index records through the shared buffer -- */

		copy(records[index].Hash[:], recordBuffer[:32])
		records[index].BlockNumber =
			binary.BigEndian.Uint64(recordBuffer[32:40])
		records[index].TransactionIndex =
			binary.BigEndian.Uint32(recordBuffer[40:44])

		if index > 0 {
			comparison := bytes.Compare(
				records[index-1].Hash[:],
				records[index].Hash[:],
			)
			if comparison >= 0 {
				return nil, errors.New(
					"transaction index hashes are not strictly ordered",
				)
			}
		}
	}

	/* START ALPHA CHANGE 2072 -- check trailing bytes through the same buffered reader -- */
	var trailing [1]byte
	if count, err := reader.Read(trailing[:]); err != io.EOF || count != 0 {
		if err != nil {
			return nil, fmt.Errorf(
				"failed while checking transaction index trailing data: %w",
				err,
			)
		}

		return nil, errors.New("transaction index contains trailing data")
	}
	/* FINISH ALPHA CHANGE 2072 -- check trailing bytes through the same buffered reader -- */

	return records, nil
}

/* FINISH ALPHA CHANGE 2065 -- encode bounded immutable transaction indexes -- */
