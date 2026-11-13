/* START ALPHA CHANGE 745 -- add MDBX blockchain storage backend -- */
package mdbx

import (
	"fmt"
	"os"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"

	/* START ALPHA CHANGE 3453 -- configure and verify blockchain MDBX page size -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3453 -- configure and verify blockchain MDBX page size -- */)

const (
	/* START ALPHA CHANGE 3058 -- default blockchain MDBX storage to one TiB -- */
	DefaultMapSize = 1024 * 1024 * 1024 * 1024
	/* FINISH ALPHA CHANGE 3058 -- default blockchain MDBX storage to one TiB -- */
	/* START ALPHA CHANGE 3453 -- retain the shared four-KiB default -- */
	defaultPageSize = mdbxconfig.DefaultPageSizeBytes
	/* FINISH ALPHA CHANGE 3453 -- retain the shared four-KiB default -- */
)

// Factory creates MDBX blockchain storage.
/* START ALPHA CHANGE 3077 -- use the any alias in MDBX factory configuration -- */
func Factory(config map[string]any, logger hclog.Logger) (storage.Storage, error) {
	/* FINISH ALPHA CHANGE 3077 -- use the any alias in MDBX factory configuration -- */
	path, ok := config["path"]
	if !ok {
		return nil, fmt.Errorf("path not found")
	}

	pathStr, ok := path.(string)
	if !ok {
		return nil, fmt.Errorf("path is not a string")
	}

	return NewMDBXStorage(pathStr, logger)
}

// NewMDBXStorage creates the new storage reference with MDBX default options.
func NewMDBXStorage(path string, logger hclog.Logger) (storage.Storage, error) {
	return NewMDBXStorageWithMapSize(path, logger, DefaultMapSize)
}

// NewMDBXStorageWithMapSize creates the new storage reference with MDBX custom map size.
/* START ALPHA CHANGE 3453 -- accept an optional explicit page size -- */
func NewMDBXStorageWithMapSize(
	path string,
	logger hclog.Logger,
	mapSize int,
	pageSizes ...uint64,
) (storage.Storage, error) {
	return newMDBXStorage(path, logger, mapSize, false, pageSizes...)
}

/* FINISH ALPHA CHANGE 3453 -- accept an optional explicit page size -- */

/* START ALPHA CHANGE 1774 -- use stored MDBX geometry for read-only blockchain access -- */
func NewMDBXStorageReadOnly(path string, logger hclog.Logger) (storage.Storage, error) {
	return newMDBXStorage(path, logger, 0, true)
}

func newMDBXStorage(
	path string,
	logger hclog.Logger,
	mapSize int,
	readOnly bool,
	pageSizes ...uint64,
) (storage.Storage, error) {
	/* START ALPHA CHANGE 3453 -- reject incompatible existing geometry before writable opening -- */
	pageSize, err := mdbxconfig.Resolve(pageSizes...)
	if err != nil {
		return nil, err
	}
	if !readOnly {
		if err := mdbxconfig.CheckDatabase(path, uint64(pageSize)); err != nil {
			return nil, err
		}
	}
	/* FINISH ALPHA CHANGE 3453 -- reject incompatible existing geometry before writable opening -- */

	if !readOnly {
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, err
		}
	}

	env, err := mdbxgo.NewEnv(mdbxgo.Default)
	if err != nil {
		return nil, err
	}

	if !readOnly {
		if err := env.SetGeometry(
			-1,
			-1,
			mapSize,
			-1,
			-1,
			/* START ALPHA CHANGE 3453 -- apply the selected creation page size -- */
			pageSize,
			/* FINISH ALPHA CHANGE 3453 -- apply the selected creation page size -- */
		); err != nil {
			_ = env.Close()

			return nil, err
		}
	}

	flags := uint(0)
	if readOnly {
		flags = mdbxgo.Readonly
	}
	/* FINISH ALPHA CHANGE 1774 -- use stored MDBX geometry for read-only blockchain access -- */

	if err := env.Open(path, flags, 0600); err != nil {
		_ = env.Close()

		return nil, err
	}

	/* START ALPHA CHANGE 3453 -- verify the opened environment before root initialization -- */
	if !readOnly {
		if err := mdbxconfig.CheckOpen(env, path, uint64(pageSize)); err != nil {
			_ = env.Close()
			return nil, err
		}
	}
	/* FINISH ALPHA CHANGE 3453 -- verify the opened environment before root initialization -- */

	kv := &mdbxKV{env: env}

	openRoot := env.Update
	if readOnly {
		openRoot = env.View
	}

	if err := openRoot(func(txn *mdbxgo.Txn) error {
		dbi, err := txn.OpenRoot(0)
		if err != nil {
			return err
		}

		kv.dbi = dbi

		return nil
	}); err != nil {
		_ = env.Close()

		return nil, err
	}

	return storage.NewKeyValueStorage(logger.Named("mdbx"), kv), nil
}

/* FINISH ALPHA CHANGE 1664 -- open blockchain MDBX for an independent read-only builder -- */

// mdbxKV is the MDBX implementation of the kv storage.
type mdbxKV struct {
	env *mdbxgo.Env
	dbi mdbxgo.DBI
}

// Get retrieves the key-value pair in MDBX storage.
func (m *mdbxKV) Get(p []byte) ([]byte, bool, error) {
	var data []byte

	err := m.env.View(func(txn *mdbxgo.Txn) error {
		value, err := txn.Get(m.dbi, p)
		if err != nil {
			if mdbxgo.IsNotFound(err) {
				return nil
			}

			return err
		}

		data = copyBytes(value)

		return nil
	})
	if err != nil {
		return nil, false, err
	}

	if data == nil {
		return nil, false, nil
	}

	return data, true, nil
}

/* START ALPHA CHANGE 3167 -- determine MDBX emptiness from database statistics without scanning records -- */
func (m *mdbxKV) IsEmpty() (bool, error) {
	var empty bool

	err := m.env.View(func(txn *mdbxgo.Txn) error {
		stat, err := txn.StatDBI(m.dbi)
		if err != nil {
			return err
		}
		empty = stat.Entries == 0
		return nil
	})

	return empty, err
}

/* FINISH ALPHA CHANGE 3167 -- determine MDBX emptiness from database statistics without scanning records -- */

// Close closes the MDBX storage instance.
func (m *mdbxKV) Close() error {
	return m.env.Close()
}

func (m *mdbxKV) NewBatch() storage.Batch {
	return NewBatchMDBX(m.env, m.dbi)
}

func copyBytes(input []byte) []byte {
	if input == nil {
		return nil
	}

	output := make([]byte, len(input))
	copy(output, input)

	return output
}

/* FINISH ALPHA CHANGE 745 -- add MDBX blockchain storage backend -- */
