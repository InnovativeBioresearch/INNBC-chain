/* START ALPHA CHANGE 2265 -- test fixed-view database backup across concurrent source writes -- */
package mdbx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	"github.com/stretchr/testify/require"
)

const backupTestMapSize = 64 * 1024 * 1024

func TestDatabaseBackupFixedReadView(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source")
	targetPath := filepath.Join(root, "target")
	require.NoError(t, os.Mkdir(sourcePath, 0700))
	require.NoError(t, os.Mkdir(targetPath, 0700))

	source, sourceDBI, err := openBackupEnvironment(
		sourcePath, backupTestMapSize, false,
	)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, source.Close())
	}()

	target, targetDBI, err := openBackupEnvironment(
		targetPath, backupTestMapSize, false,
	)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, target.Close())
	}()

	const records = 2500 // Exercises multiple destination write batches.
	require.NoError(t, source.Update(func(txn *mdbxgo.Txn) error {
		/* START ALPHA CHANGE 2270 -- use integer range for backup test records -- */
		for index := range records {
			/* FINISH ALPHA CHANGE 2270 -- use integer range for backup test records -- */
			/* START ALPHA CHANGE 2269 -- format backup test keys directly into bytes -- */
			key := fmt.Appendf(nil, "key-%06d", index)
			/* FINISH ALPHA CHANGE 2269 -- format backup test keys directly into bytes -- */
			if err := txn.Put(sourceDBI, key, []byte("original"), 0); err != nil {
				return err
			}
		}

		return nil
	}))

	var captured DatabaseCopyResult
	require.NoError(t, source.View(func(txn *mdbxgo.Txn) error {
		// Commit a newer source version after this read transaction begins.
		// The copy must still contain the older, internally consistent view.
		done := make(chan error, 1)
		go func() {
			done <- source.Update(func(writeTxn *mdbxgo.Txn) error {
				if err := writeTxn.Put(
					sourceDBI, []byte("key-000000"), []byte("changed"), 0,
				); err != nil {
					return err
				}

				return writeTxn.Put(sourceDBI, []byte("new-key"), []byte("new"), 0)
			})
		}()
		if err := <-done; err != nil {
			return err
		}

		var copyErr error
		captured, copyErr = copyBackupRecords(
			ctx, txn, sourceDBI, target, targetDBI,
		)

		return copyErr
	}))
	require.Equal(t, uint64(records), captured.Records)

	require.NoError(t, target.View(func(txn *mdbxgo.Txn) error {
		actual, err := scanBackupRecords(ctx, txn, targetDBI, nil)
		if err != nil {
			return err
		}
		require.Equal(t, captured, actual)

		value, err := txn.Get(targetDBI, []byte("key-000000"))
		if err != nil {
			return err
		}
		require.Equal(t, "original", string(value))

		_, err = txn.Get(targetDBI, []byte("new-key"))
		require.True(t, mdbxgo.IsNotFound(err))

		return nil
	}))

	require.NoError(t, source.View(func(txn *mdbxgo.Txn) error {
		value, err := txn.Get(sourceDBI, []byte("key-000000"))
		if err != nil {
			return err
		}
		require.Equal(t, "changed", string(value))

		return nil
	}))
}

func TestDatabaseBackupCopyAndCancellation(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source")
	require.NoError(t, os.Mkdir(sourcePath, 0700))

	env, dbi, err := openBackupEnvironment(sourcePath, backupTestMapSize, false)
	require.NoError(t, err)
	require.NoError(t, env.Update(func(txn *mdbxgo.Txn) error {
		if err := txn.Put(dbi, []byte("first"), []byte("one"), 0); err != nil {
			return err
		}

		return txn.Put(dbi, []byte("second"), []byte("two"), 0)
	}))
	require.NoError(t, env.Close())

	targetPath := filepath.Join(root, "backup")
	copied, err := CopyBlockchainDatabase(
		context.Background(), sourcePath, targetPath, backupTestMapSize,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(2), copied.Records)
	require.FileExists(t, filepath.Join(targetPath, "mdbx.dat"))
	require.NoFileExists(t, filepath.Join(targetPath, "mdbx.lck"))

	_, err = CopyBlockchainDatabase(
		context.Background(), sourcePath, targetPath, backupTestMapSize,
	)
	require.Error(t, err)
	actual, err := inspectBackupDatabase(context.Background(), targetPath)
	require.NoError(t, err)
	require.Equal(t, *copied, actual)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelledPath := filepath.Join(root, "cancelled")
	_, err = CopyBlockchainDatabase(ctx, sourcePath, cancelledPath, backupTestMapSize)
	require.ErrorIs(t, err, context.Canceled)
	_, err = os.Stat(cancelledPath)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestDatabaseBackupScanStopsOnCancellation(t *testing.T) {
	path := t.TempDir()
	env, dbi, err := openBackupEnvironment(path, backupTestMapSize, false)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, env.Close())
	}()

	require.NoError(t, env.Update(func(txn *mdbxgo.Txn) error {
		for _, key := range []string{"a", "b", "c"} {
			if err := txn.Put(dbi, []byte(key), []byte("value"), 0); err != nil {
				return err
			}
		}

		return nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	visited := 0

	err = env.View(func(txn *mdbxgo.Txn) error {
		_, err := scanBackupRecords(ctx, txn, dbi, func(_, _ []byte) error {
			visited++
			cancel()

			return nil
		})

		return err
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, visited)
}

/* FINISH ALPHA CHANGE 2265 -- test fixed-view database backup across concurrent source writes -- */
