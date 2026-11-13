package itrie

import (
	"fmt"
	"os"
	"sync"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/helper/hex"

	/* START ALPHA CHANGE 3454 -- share trie MDBX page-size validation -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3454 -- share trie MDBX page-size validation -- */

	/* START ALPHA CHANGE 2129 -- classify existing trie-decoding failures -- */
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	/* FINISH ALPHA CHANGE 2129 -- classify existing trie-decoding failures -- */
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/fastrlp"
)

var parserPool fastrlp.ParserPool

var (
	// codePrefix is the code prefix for trie code storage
	codePrefix = []byte("code")
)

const (
	/* START ALPHA CHANGE 3059 -- default trie MDBX storage to one TiB -- */
	defaultMDBXMapSize = 1024 * 1024 * 1024 * 1024
	/* FINISH ALPHA CHANGE 3059 -- default trie MDBX storage to one TiB -- */
	/* START ALPHA CHANGE 3454 -- retain the shared four-KiB default -- */
	defaultMDBXPageSize = mdbxconfig.DefaultPageSizeBytes
	/* FINISH ALPHA CHANGE 3454 -- retain the shared four-KiB default -- */
)

// Batch is batch write interface
type Batch interface {
	// Put puts key and value into batch. It can not return error because actual writing is done with Write method
	Put(k, v []byte)
	// Write writes all the key values pair previosly putted with Put method to the database
	Write() error
}

// Storage stores the trie
/* START ALPHA CHANGE 2129 -- preserve backend errors in bytecode reads -- */
type Storage interface {
	Put(k, v []byte) error
	Get(k []byte) ([]byte, bool, error)
	Batch() Batch
	SetCode(hash types.Hash, code []byte) error
	GetCode(hash types.Hash) ([]byte, bool, error)

	Close() error
}

/* FINISH ALPHA CHANGE 2129 -- preserve backend errors in bytecode reads -- */

// KVStorage is a k/v trie storage backed by MDBX.
type KVStorage struct {
	env *mdbxgo.Env
	dbi mdbxgo.DBI
}

// KVBatch is a batch write for MDBX trie storage.
type KVBatch struct {
	env         *mdbxgo.Env
	dbi         mdbxgo.DBI
	valuesToPut [][2][]byte
}

func (b *KVBatch) Put(k, v []byte) {
	b.valuesToPut = append(b.valuesToPut, [2][]byte{
		copyBytes(k),
		copyBytes(v),
	})
}

func (b *KVBatch) Write() error {
	return b.env.Update(func(txn *mdbxgo.Txn) error {
		for _, value := range b.valuesToPut {
			if err := txn.Put(b.dbi, value[0], value[1], 0); err != nil {
				return err
			}
		}

		return nil
	})
}

func (kv *KVStorage) SetCode(hash types.Hash, code []byte) error {
	return kv.Put(GetCodeKey(hash), code)
}

/* START ALPHA CHANGE 2130 -- propagate MDBX bytecode read errors -- */
func (kv *KVStorage) GetCode(hash types.Hash) ([]byte, bool, error) {
	return kv.Get(GetCodeKey(hash))
}

/* FINISH ALPHA CHANGE 2130 -- propagate MDBX bytecode read errors -- */

func (kv *KVStorage) Batch() Batch {
	return &KVBatch{env: kv.env, dbi: kv.dbi}
}

func (kv *KVStorage) Put(k, v []byte) error {
	return kv.env.Update(func(txn *mdbxgo.Txn) error {
		return txn.Put(kv.dbi, k, v, 0)
	})
}

func (kv *KVStorage) Get(k []byte) ([]byte, bool, error) {
	var data []byte

	err := kv.env.View(func(txn *mdbxgo.Txn) error {
		value, err := txn.Get(kv.dbi, k)
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

func (kv *KVStorage) Close() error {
	return kv.env.Close()
}

func NewMDBXStorage(path string, logger hclog.Logger) (Storage, error) {
	return NewMDBXStorageWithMapSize(path, logger, defaultMDBXMapSize)
}

/* START ALPHA CHANGE 1058 -- add configurable MDBX trie storage map size -- */
/* START ALPHA CHANGE 3454 -- accept an optional destination page size -- */
func NewMDBXStorageWithMapSize(
	path string,
	logger hclog.Logger,
	mapSize int,
	pageSizes ...uint64,
) (Storage, error) {
	return newMDBXStorage(path, logger, false, mapSize, pageSizes...)
}

/* FINISH ALPHA CHANGE 3454 -- accept an optional destination page size -- */

/* FINISH ALPHA CHANGE 1058 -- add configurable MDBX trie storage map size -- */

/* START ALPHA CHANGE 1775 -- use stored MDBX geometry for read-only trie access -- */
func NewMDBXStorageReadOnly(path string, logger hclog.Logger) (Storage, error) {
	return newMDBXStorage(path, logger, true, 0)
}

func newMDBXStorage(
	path string,
	_ hclog.Logger,
	readOnly bool,
	mapSize int,
	pageSizes ...uint64,
) (Storage, error) {
	/* START ALPHA CHANGE 3454 -- reject incompatible geometry before writable trie opening -- */
	pageSize, err := mdbxconfig.Resolve(pageSizes...)
	if err != nil {
		return nil, err
	}
	if !readOnly {
		if err := mdbxconfig.CheckDatabase(path, uint64(pageSize)); err != nil {
			return nil, err
		}
	}
	/* FINISH ALPHA CHANGE 3454 -- reject incompatible geometry before writable trie opening -- */

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
			/* START ALPHA CHANGE 3454 -- apply the selected trie creation page size -- */
			pageSize,
			/* FINISH ALPHA CHANGE 3454 -- apply the selected trie creation page size -- */
		); err != nil {
			_ = env.Close()

			return nil, err
		}
	}

	flags := uint(0)
	if readOnly {
		flags = mdbxgo.Readonly
	}
	/* FINISH ALPHA CHANGE 1775 -- use stored MDBX geometry for read-only trie access -- */

	if err := env.Open(path, flags, 0600); err != nil {
		_ = env.Close()

		return nil, err
	}

	/* START ALPHA CHANGE 3454 -- verify the opened trie before application writes -- */
	if !readOnly {
		if err := mdbxconfig.CheckOpen(env, path, uint64(pageSize)); err != nil {
			_ = env.Close()
			return nil, err
		}
	}
	/* FINISH ALPHA CHANGE 3454 -- verify the opened trie before application writes -- */

	kv := &KVStorage{env: env}

	openRoot := func(txn *mdbxgo.Txn) error {
		dbi, err := txn.OpenRoot(0)
		if err != nil {
			return err
		}

		kv.dbi = dbi

		return nil
	}

	if readOnly {
		err = env.View(openRoot)
	} else {
		err = env.Update(openRoot)
	}

	if err != nil {
		_ = env.Close()

		return nil, err
	}

	return kv, nil
}

func copyBytes(input []byte) []byte {
	if input == nil {
		return nil
	}

	output := make([]byte, len(input))
	copy(output, input)

	return output
}

type memStorage struct {
	l    *sync.Mutex
	db   map[string][]byte
	code map[string][]byte
}

/* START ALPHA CHANGE 2659 -- retain memory batch records until publication -- */
type memBatch struct {
	l           *sync.Mutex
	db          *map[string][]byte
	valuesToPut [][2][]byte
}

/* FINISH ALPHA CHANGE 2659 -- retain memory batch records until publication -- */

// NewMemoryStorage creates an inmemory trie storage
func NewMemoryStorage() Storage {
	return &memStorage{db: map[string][]byte{}, code: map[string][]byte{}, l: new(sync.Mutex)}
}

func (m *memStorage) Put(p []byte, v []byte) error {
	m.l.Lock()
	defer m.l.Unlock()

	buf := make([]byte, len(v))
	copy(buf[:], v[:])
	m.db[hex.EncodeToHex(p)] = buf

	return nil
}

func (m *memStorage) Get(p []byte) ([]byte, bool, error) {
	m.l.Lock()
	defer m.l.Unlock()

	v, ok := m.db[hex.EncodeToHex(p)]
	if !ok {
		return []byte{}, false, nil
	}

	return v, true, nil
}

func (m *memStorage) SetCode(hash types.Hash, code []byte) error {
	return m.Put(append(codePrefix, hash.Bytes()...), code)
}

/* START ALPHA CHANGE 2131 -- propagate memory-storage bytecode read errors -- */
func (m *memStorage) GetCode(hash types.Hash) ([]byte, bool, error) {
	return m.Get(GetCodeKey(hash))
}

/* FINISH ALPHA CHANGE 2131 -- propagate memory-storage bytecode read errors -- */

/* START ALPHA CHANGE 2660 -- serialize memory batch publication with storage reads and writes -- */
func (m *memStorage) Batch() Batch {
	return &memBatch{db: &m.db, l: m.l}
}

/* FINISH ALPHA CHANGE 2660 -- serialize memory batch publication with storage reads and writes -- */

func (m *memStorage) Close() error {
	return nil
}

/* START ALPHA CHANGE 2661 -- stage memory records and publish them under the shared storage lock -- */
/*
	Like KVBatch, a memory batch belongs to one caller while it is assembled.
	Put copies its inputs without publishing them.
*/
func (m *memBatch) Put(p, v []byte) {
	m.valuesToPut = append(m.valuesToPut, [2][]byte{
		copyBytes(p),
		copyBytes(v),
	})
}

func (m *memBatch) Write() error {
	m.l.Lock()
	defer m.l.Unlock()

	for _, value := range m.valuesToPut {
		(*m.db)[hex.EncodeToHex(value[0])] = value[1]
	}

	return nil
}

/* FINISH ALPHA CHANGE 2661 -- stage memory records and publish them under the shared storage lock -- */

// GetNode retrieves a node from storage
func GetNode(root []byte, storage Storage) (Node, bool, error) {
	data, ok, err := storage.Get(root)
	if err != nil || !ok || len(data) == 0 {
		return nil, false, err
	}

	// NOTE. We dont need to make copies of the bytes because the nodes
	// take the reference from data itself which is a safe copy.
	p := parserPool.Get()
	defer parserPool.Put(p)

	/* START ALPHA CHANGE 2132 -- preserve and classify trie-decoding errors -- */
	v, err := p.Parse(data)
	if err != nil {
		return nil, false, statepkg.WrapLocalStateFailure(
			fmt.Sprintf("decode trie node %x", root),
			fmt.Errorf("%w: %w", statepkg.ErrMalformedTrieNode, err),
		)
	}

	if v.Type() != fastrlp.TypeArray {
		return nil, false, statepkg.WrapLocalStateFailure(
			fmt.Sprintf("decode trie node %x", root),
			fmt.Errorf(
				"%w: storage item should be an array",
				statepkg.ErrMalformedTrieNode,
			),
		)
	}

	n, err := decodeNode(v, storage)
	if err != nil {
		return nil, false, statepkg.WrapLocalStateFailure(
			fmt.Sprintf("decode trie node %x", root),
			fmt.Errorf("%w: %w", statepkg.ErrMalformedTrieNode, err),
		)
	}

	return n, true, nil
	/* FINISH ALPHA CHANGE 2132 -- preserve and classify trie-decoding errors -- */
}

func decodeNode(v *fastrlp.Value, s Storage) (Node, error) {
	if v.Type() == fastrlp.TypeBytes {
		vv := &ValueNode{
			hash: true,
		}
		vv.buf = append(vv.buf[:0], v.Raw()...)

		return vv, nil
	}

	var err error

	ll := v.Elems()
	if ll == 2 {
		key := v.Get(0)
		if key.Type() != fastrlp.TypeBytes {
			return nil, fmt.Errorf("short key expected to be bytes")
		}

		/* START ALPHA CHANGE 2132 -- return an error instead of panicking on an empty compact path -- */
		if len(key.Raw()) == 0 {
			return nil, fmt.Errorf("short node compact path is empty")
		}
		/* FINISH ALPHA CHANGE 2132 -- return an error instead of panicking on an empty compact path -- */

		// this can be either an array (extension node)
		// or bytes (leaf node)
		nc := &ShortNode{}
		nc.key = decodeCompact(key.Raw())

		if hasTerminator(nc.key) {
			// value node
			if v.Get(1).Type() != fastrlp.TypeBytes {
				return nil, fmt.Errorf("short leaf value expected to be bytes")
			}

			vv := &ValueNode{}
			vv.buf = append(vv.buf, v.Get(1).Raw()...)
			nc.child = vv
		} else {
			nc.child, err = decodeNode(v.Get(1), s)
			if err != nil {
				return nil, err
			}
		}

		return nc, nil
	} else if ll == 17 {
		// full node
		nc := &FullNode{}
		for i := 0; i < 16; i++ {
			if v.Get(i).Type() == fastrlp.TypeBytes && len(v.Get(i).Raw()) == 0 {
				// empty
				continue
			}
			nc.children[i], err = decodeNode(v.Get(i), s)
			if err != nil {
				return nil, err
			}
		}

		if v.Get(16).Type() != fastrlp.TypeBytes {
			return nil, fmt.Errorf("full node value expected to be bytes")
		}
		if len(v.Get(16).Raw()) != 0 {
			vv := &ValueNode{}
			vv.buf = append(vv.buf[:0], v.Get(16).Raw()...)
			nc.value = vv
		}

		return nc, nil
	}

	return nil, fmt.Errorf("node has incorrect number of leafs")
}

func GetCodeKey(hash types.Hash) []byte {
	return append(codePrefix, hash.Bytes()...)
}
