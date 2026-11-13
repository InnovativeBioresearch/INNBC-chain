package memory

import (
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	"github.com/innovativebioresearch/innbc-chain/helper/hex"
)

// NewMemoryStorage creates the new storage reference with inmemory
func NewMemoryStorage(logger hclog.Logger) (storage.Storage, error) {
	db := &memoryKV{map[string][]byte{}}

	return storage.NewKeyValueStorage(logger, db), nil
}

// memoryKV is an in memory implementation of the kv storage
type memoryKV struct {
	db map[string][]byte
}

func (m *memoryKV) Set(p []byte, v []byte) error {
	m.db[hex.EncodeToHex(p)] = v

	return nil
}

func (m *memoryKV) Get(p []byte) ([]byte, bool, error) {
	v, ok := m.db[hex.EncodeToHex(p)]
	if !ok {
		return nil, false, nil
	}

	return v, true, nil
}

/* START ALPHA CHANGE 3167 -- determine memory database emptiness from its actual record count -- */
func (m *memoryKV) IsEmpty() (bool, error) {
	return len(m.db) == 0, nil
}

/* FINISH ALPHA CHANGE 3167 -- determine memory database emptiness from its actual record count -- */

func (m *memoryKV) Close() error {
	return nil
}

func (m *memoryKV) NewBatch() storage.Batch {
	return NewBatchMemory(m.db)
}
