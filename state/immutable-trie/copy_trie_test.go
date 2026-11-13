package itrie

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

/* START ALPHA CHANGE 1293 -- test bounded persisted trie copy behavior -- */
type countingStorage struct {
	Storage
	writes int
}

func (s *countingStorage) Batch() Batch {
	return &countingBatch{
		Batch:  s.Storage.Batch(),
		writes: &s.writes,
	}
}

type countingBatch struct {
	Batch
	writes *int
}

func (b *countingBatch) Write() error {
	*b.writes++

	return b.Batch.Write()
}

type failingBatchStorage struct {
	Storage
	writes int
	failAt int
}

func (s *failingBatchStorage) Batch() Batch {
	return &failingBatch{
		Batch:  s.Storage.Batch(),
		parent: s,
	}
}

type failingBatch struct {
	Batch
	parent *failingBatchStorage
}

func (b *failingBatch) Write() error {
	b.parent.writes++
	if b.parent.writes == b.parent.failAt {
		return errors.New("forced batch write failure")
	}

	return b.Batch.Write()
}

func buildPersistedStorageTrie(t *testing.T, kv Storage, entries int, valueLen int) types.Hash {
	t.Helper()

	state := NewState(kv)
	trie := state.newTrie()
	tx := trie.Txn(kv)

	batch := kv.Batch()
	tx.batch = batch

	for i := 0; i < entries; i++ {
		key := make([]byte, 32)
		binary.BigEndian.PutUint64(key[24:], uint64(i))

		value := bytes.Repeat([]byte{byte(i%251 + 1)}, valueLen)
		tx.Insert(key, value)
	}

	root, err := tx.Hash()
	require.NoError(t, err)
	require.NoError(t, batch.Write())

	return types.BytesToHash(root)
}

func TestCompareModelOfTrieCopy(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(tt *rapid.T) {
		kv := NewMemoryStorage()
		newKV := NewMemoryStorage()

		state := NewState(kv)
		trie := state.newTrie()
		tx := trie.Txn(kv)

		batch := kv.Batch()
		tx.batch = batch

		n := rapid.IntRange(1, 1000).Draw(tt, "n")
		for i := 0; i < n; i++ {
			key := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(tt, "key")
			value := rapid.SliceOfN(rapid.Byte(), 64, 96).Draw(tt, "value")
			tx.Insert(key, value)
		}

		root, err := tx.Hash()
		if err != nil {
			t.Fatal(err)
		}

		if err := batch.Write(); err != nil {
			t.Fatal(err)
		}

		stateRoot := types.BytesToHash(root)
		result, err := HashChecker(stateRoot.Bytes(), kv)
		if err != nil {
			t.Fatal(err)
		}
		if stateRoot != result {
			t.Fatal("Hashes are not equal", stateRoot, result)
		}

		err = CopyTrie(stateRoot.Bytes(), kv, newKV, []byte{}, true)
		if err != nil {
			t.Fatal(err)
		}

		result, err = HashChecker(stateRoot.Bytes(), newKV)
		if err != nil {
			t.Error(err)
		}
		if stateRoot != result {
			t.Error("Hashes are not equal", stateRoot, result)
		}
	})
}

func TestCopyTrieFlushesMultipleBatches(t *testing.T) {
	oldMaxPendingNodes := defaultTrieCopyMaxPendingNodes
	oldMaxPendingBytes := defaultTrieCopyMaxPendingBytes
	t.Cleanup(func() {
		defaultTrieCopyMaxPendingNodes = oldMaxPendingNodes
		defaultTrieCopyMaxPendingBytes = oldMaxPendingBytes
	})

	defaultTrieCopyMaxPendingNodes = 2
	defaultTrieCopyMaxPendingBytes = 0

	source := NewMemoryStorage()
	root := buildPersistedStorageTrie(t, source, 12, 64)

	target := &countingStorage{Storage: NewMemoryStorage()}

	require.NoError(t, CopyTrie(root.Bytes(), source, target, nil, true))
	require.GreaterOrEqual(t, target.writes, 2)

	checkedRoot, err := HashChecker(root.Bytes(), target)
	require.NoError(t, err)
	require.Equal(t, root, checkedRoot)
}

func TestCopyTrieReturnsIntermediateBatchWriteError(t *testing.T) {
	oldMaxPendingNodes := defaultTrieCopyMaxPendingNodes
	oldMaxPendingBytes := defaultTrieCopyMaxPendingBytes
	t.Cleanup(func() {
		defaultTrieCopyMaxPendingNodes = oldMaxPendingNodes
		defaultTrieCopyMaxPendingBytes = oldMaxPendingBytes
	})

	defaultTrieCopyMaxPendingNodes = 1
	defaultTrieCopyMaxPendingBytes = 0

	source := NewMemoryStorage()
	root := buildPersistedStorageTrie(t, source, 12, 64)

	target := &failingBatchStorage{
		Storage: NewMemoryStorage(),
		failAt:  1,
	}

	err := CopyTrie(root.Bytes(), source, target, nil, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "forced batch write failure")
}

func TestCopyTrieReturnsFinalBatchWriteError(t *testing.T) {
	oldMaxPendingNodes := defaultTrieCopyMaxPendingNodes
	oldMaxPendingBytes := defaultTrieCopyMaxPendingBytes
	t.Cleanup(func() {
		defaultTrieCopyMaxPendingNodes = oldMaxPendingNodes
		defaultTrieCopyMaxPendingBytes = oldMaxPendingBytes
	})

	defaultTrieCopyMaxPendingNodes = 0
	defaultTrieCopyMaxPendingBytes = 0

	source := NewMemoryStorage()
	root := buildPersistedStorageTrie(t, source, 3, 64)

	target := &failingBatchStorage{
		Storage: NewMemoryStorage(),
		failAt:  1,
	}

	err := CopyTrie(root.Bytes(), source, target, nil, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "forced batch write failure")
}

func TestCopyTrieRejectsExcessiveDepth(t *testing.T) {
	oldMaxDepth := defaultTrieCopyMaxDepth
	t.Cleanup(func() {
		defaultTrieCopyMaxDepth = oldMaxDepth
	})

	defaultTrieCopyMaxDepth = 1

	source := NewMemoryStorage()
	root := buildPersistedStorageTrie(t, source, 3, 64)

	err := CopyTrie(root.Bytes(), source, NewMemoryStorage(), nil, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "trie copy exceeded maximum depth")
}

func TestHashCheckerRejectsExcessiveDepth(t *testing.T) {
	oldMaxDepth := defaultTrieCheckMaxDepth
	t.Cleanup(func() {
		defaultTrieCheckMaxDepth = oldMaxDepth
	})

	defaultTrieCheckMaxDepth = 1

	source := NewMemoryStorage()
	root := buildPersistedStorageTrie(t, source, 3, 64)

	_, err := HashChecker(root.Bytes(), source)
	require.Error(t, err)
	require.Contains(t, err.Error(), "trie hash check exceeded maximum depth")
}

/* FINISH ALPHA CHANGE 1293 -- test bounded persisted trie copy behavior -- */
