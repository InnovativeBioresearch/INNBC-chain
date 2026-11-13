/* START ALPHA CHANGE 2261 -- export a fixed blockchain MDBX view and verify every copied record -- */
package mdbx

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	/* START ALPHA CHANGE 3456 -- configure database-copy destination pages -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3456 -- configure database-copy destination pages -- */)

const (
	backupBatchRecords = 1024
	backupBatchBytes   = 4 * 1024 * 1024
)

type DatabaseCopyResult struct {
	Records       uint64 `json:"records"`
	RecordsSHA256 string `json:"records_sha256"`
}

// CopyBlockchainDatabase copies the root database from one fixed source read
// transaction. Normal source writes may continue in another process.
//
// The caller must create the target's parent directory. Target itself must not
// exist. Source replacement, deletion and database maintenance must not overlap
// this operation. This function does not copy the source MDBX lock file.
func CopyBlockchainDatabase(
	ctx context.Context,
	sourcePath string,
	targetPath string,
	mapSize int,
	pageSizes ...uint64,
) (result *DatabaseCopyResult, err error) {
	/* START ALPHA CHANGE 3456 -- reject invalid destination settings before creating files -- */
	if _, err := mdbxconfig.Resolve(pageSizes...); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3456 -- reject invalid destination settings before creating files -- */
	if ctx == nil {
		return nil, errors.New("backup context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if mapSize <= 0 {
		return nil, errors.New("backup MDBX map size must be positive")
	}

	info, err := os.Lstat(filepath.Join(sourcePath, "mdbx.dat"))
	if err != nil {
		return nil, fmt.Errorf("inspect source MDBX data: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("source MDBX data must be a regular file")
	}

	if err := os.Mkdir(targetPath, 0700); err != nil {
		return nil, fmt.Errorf("create new backup database directory: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(targetPath))
		}
	}()

	/* START ALPHA CHANGE 3456 -- pass destination geometry into record copying -- */
	expected, err := copyBackupDatabase(ctx, sourcePath, targetPath, mapSize, pageSizes...)
	/* FINISH ALPHA CHANGE 3456 -- pass destination geometry into record copying -- */
	if err != nil {
		return nil, err
	}

	actual, err := inspectBackupDatabase(ctx, targetPath)
	if err != nil {
		return nil, fmt.Errorf("verify copied MDBX records: %w", err)
	}
	if actual != expected {
		return nil, errors.New("copied MDBX records differ from captured source view")
	}

	// All destination environments have closed. Its runtime lock is not
	// archival data and will be recreated when the database is opened.
	if err := os.Remove(filepath.Join(targetPath, "mdbx.lck")); err != nil &&
		!errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove destination runtime lock: %w", err)
	}

	return &actual, nil
}

func copyBackupDatabase(
	ctx context.Context,
	sourcePath string,
	targetPath string,
	mapSize int,
	pageSizes ...uint64,
) (result DatabaseCopyResult, err error) {
	source, sourceDBI, err := openBackupEnvironment(sourcePath, 0, true)
	if err != nil {
		return result, fmt.Errorf("open backup source: %w", err)
	}
	defer func() {
		err = errors.Join(err, source.Close())
	}()

	/* START ALPHA CHANGE 3456 -- apply destination pages without constraining the source -- */
	target, targetDBI, err := openBackupEnvironment(targetPath, mapSize, false, pageSizes...)
	/* FINISH ALPHA CHANGE 3456 -- apply destination pages without constraining the source -- */
	if err != nil {
		return result, fmt.Errorf("open backup destination: %w", err)
	}
	defer func() {
		err = errors.Join(err, target.Close())
	}()

	err = source.View(func(txn *mdbxgo.Txn) error {
		var copyErr error
		result, copyErr = copyBackupRecords(
			ctx, txn, sourceDBI, target, targetDBI,
		)

		return copyErr
	})

	return result, err
}

func copyBackupRecords(
	ctx context.Context,
	sourceTxn *mdbxgo.Txn,
	sourceDBI mdbxgo.DBI,
	target *mdbxgo.Env,
	targetDBI mdbxgo.DBI,
) (DatabaseCopyResult, error) {
	var batch [][2][]byte
	var batchBytes uint64

	flush := func() error {
		if len(batch) == 0 {
			return ctx.Err()
		}

		err := target.Update(func(txn *mdbxgo.Txn) error {
			for _, record := range batch {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := txn.Put(targetDBI, record[0], record[1], 0); err != nil {
					return err
				}
			}

			return nil
		})

		// Release references to copied values after each bounded write batch.
		batch = nil
		batchBytes = 0

		return err
	}

	result, err := scanBackupRecords(
		ctx,
		sourceTxn,
		sourceDBI,
		func(key, value []byte) error {
			batch = append(batch, [2][]byte{
				copyBytes(key),
				copyBytes(value),
			})
			batchBytes += uint64(len(key)) + uint64(len(value))

			// One individual record may exceed the byte target.
			if len(batch) >= backupBatchRecords || batchBytes >= backupBatchBytes {
				return flush()
			}

			return nil
		},
	)
	if err != nil {
		return DatabaseCopyResult{}, err
	}
	if err := flush(); err != nil {
		return DatabaseCopyResult{}, err
	}

	return result, nil
}

func scanBackupRecords(
	ctx context.Context,
	txn *mdbxgo.Txn,
	dbi mdbxgo.DBI,
	visit func(key, value []byte) error,
) (DatabaseCopyResult, error) {
	var result DatabaseCopyResult

	cursor, err := txn.OpenCursor(dbi)
	if err != nil {
		return result, err
	}
	defer cursor.Close()

	digest := sha256.New()
	var sizes [16]byte
	operation := uint(mdbxgo.First)

	for {
		if err := ctx.Err(); err != nil {
			return DatabaseCopyResult{}, err
		}

		key, value, err := cursor.Get(nil, nil, operation)
		if mdbxgo.IsNotFound(err) {
			break
		}
		if err != nil {
			return DatabaseCopyResult{}, err
		}
		operation = mdbxgo.Next

		// Length framing makes the digest unambiguous across record boundaries.
		binary.BigEndian.PutUint64(sizes[:8], uint64(len(key)))
		binary.BigEndian.PutUint64(sizes[8:], uint64(len(value)))
		_, _ = digest.Write(sizes[:])
		_, _ = digest.Write(key)
		_, _ = digest.Write(value)

		if visit != nil {
			if err := visit(key, value); err != nil {
				return DatabaseCopyResult{}, err
			}
		}

		result.Records++
	}

	result.RecordsSHA256 = fmt.Sprintf("%x", digest.Sum(nil))

	return result, nil
}

func inspectBackupDatabase(
	ctx context.Context,
	path string,
) (result DatabaseCopyResult, err error) {
	env, dbi, err := openBackupEnvironment(path, 0, true)
	if err != nil {
		return result, err
	}
	defer func() {
		err = errors.Join(err, env.Close())
	}()

	err = env.View(func(txn *mdbxgo.Txn) error {
		var scanErr error
		result, scanErr = scanBackupRecords(ctx, txn, dbi, nil)

		return scanErr
	})

	return result, err
}

func openBackupEnvironment(
	path string,
	mapSize int,
	readOnly bool,
	pageSizes ...uint64,
) (*mdbxgo.Env, mdbxgo.DBI, error) {
	/* START ALPHA CHANGE 3456 -- validate backup destination geometry before opening -- */
	pageSize, err := mdbxconfig.Resolve(pageSizes...)
	if err != nil {
		return nil, 0, err
	}
	if !readOnly {
		if err := mdbxconfig.CheckDatabase(path, uint64(pageSize)); err != nil {
			return nil, 0, err
		}
	}
	/* FINISH ALPHA CHANGE 3456 -- validate backup destination geometry before opening -- */

	env, err := mdbxgo.NewEnv(mdbxgo.Default)
	if err != nil {
		return nil, 0, err
	}

	fail := func(err error) (*mdbxgo.Env, mdbxgo.DBI, error) {
		return nil, 0, errors.Join(err, env.Close())
	}

	if !readOnly {
		if err := env.SetGeometry(
			/* START ALPHA CHANGE 3456 -- use the selected backup creation page size -- */
			-1, -1, mapSize, -1, -1, pageSize,
			/* FINISH ALPHA CHANGE 3456 -- use the selected backup creation page size -- */
		); err != nil {
			return fail(err)
		}
	}

	flags := uint(0)
	if readOnly {
		flags = mdbxgo.Readonly
	}
	if err := env.Open(path, flags, 0600); err != nil {
		return fail(err)
	}

	/* START ALPHA CHANGE 3456 -- verify the opened backup destination before writes -- */
	if !readOnly {
		if err := mdbxconfig.CheckOpen(env, path, uint64(pageSize)); err != nil {
			return fail(err)
		}
	}
	/* FINISH ALPHA CHANGE 3456 -- verify the opened backup destination before writes -- */

	var dbi mdbxgo.DBI
	openRoot := env.Update
	if readOnly {
		openRoot = env.View
	}
	if err := openRoot(func(txn *mdbxgo.Txn) error {
		var openErr error
		dbi, openErr = txn.OpenRoot(0)

		return openErr
	}); err != nil {
		return fail(err)
	}

	return env, dbi, nil
}

/* FINISH ALPHA CHANGE 2261 -- export a fixed blockchain MDBX view and verify every copied record -- */
