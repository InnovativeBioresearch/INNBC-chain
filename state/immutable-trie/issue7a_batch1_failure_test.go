/* START ALPHA CHANGE 2151 -- test trie failures and buffered publication boundaries -- */
package itrie

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
)

func TestIssue7A_EmptyCompactPathReturnsError(t *testing.T) {
	storage := NewMemoryStorage()
	root := types.StringToHash("0x01")

	// RLP ["", ""]: parsing succeeds, but the short-node compact path
	// cannot be decoded. This must return an error, not panic.
	require.NoError(t, storage.Put(root.Bytes(), []byte{0xc2, 0x80, 0x80}))

	require.NotPanics(t, func() {
		snapshot, err := NewState(storage).NewSnapshotAt(root)
		require.Nil(t, snapshot)
		require.ErrorIs(t, err, statepkg.ErrLocalStateFailure)
		require.ErrorIs(t, err, statepkg.ErrMalformedTrieNode)
	})
}

func TestIssue7A_TrieMutationFailureCannotBeCommitted(t *testing.T) {
	for _, operation := range []string{"insert", "delete", "lookup"} {
		t.Run(operation, func(t *testing.T) {
			storage := NewMemoryStorage()
			missingHash := types.StringToHash("0x1234")
			originalRoot := &ValueNode{
				hash: true,
				buf:  missingHash.Bytes(),
			}
			trie := &Trie{root: originalRoot}
			txn := trie.Txn(storage)

			switch operation {
			case "insert":
				txn.Insert([]byte{1}, []byte{2})
			case "delete":
				txn.Delete([]byte{1})
			case "lookup":
				value, found, err := txn.Lookup([]byte{1})
				require.Nil(t, value)
				require.False(t, found)
				require.ErrorIs(t, err, statepkg.ErrMissingTrieNode)
			}

			first := txn.Error()
			require.ErrorIs(t, first, statepkg.ErrLocalStateFailure)
			require.ErrorIs(t, first, statepkg.ErrMissingTrieNode)
			require.Same(t, originalRoot, trie.root)
			require.Same(t, originalRoot, txn.root)

			// Further mutations must not clear the first failure.
			txn.Insert([]byte{3}, []byte{4})
			txn.Delete([]byte{5})
			require.Equal(t, first, txn.Error())

			root, err := txn.Hash()
			require.Nil(t, root)
			require.Equal(t, first, err)

			committed, err := txn.Commit()
			require.Nil(t, committed)
			require.Equal(t, first, err)
		})
	}
}

func TestIssue7A_HashFailureRemainsLatched(t *testing.T) {
	txn := (&Trie{
		root: &ShortNode{
			key:   []byte{1, 16},
			child: nil,
		},
	}).Txn(NewMemoryStorage())

	require.NotPanics(t, func() {
		root, err := txn.Hash()
		require.Nil(t, root)
		require.ErrorIs(t, err, statepkg.ErrLocalStateFailure)
		require.ErrorIs(t, err, statepkg.ErrMalformedTrieNode)
	})

	first := txn.Error()
	require.Error(t, first)

	root, err := txn.Hash()
	require.Nil(t, root)
	require.Equal(t, first, err)

	committed, err := txn.Commit()
	require.Nil(t, committed)
	require.Equal(t, first, err)
}

/*
	START ALPHA CHANGE 2665 -- describe buffered failure injection after memory publication is corrected --

This wrapper keeps the existing failed-publication regression independent
of the underlying storage implementation. Both production storage backends
now stage batch records until Write.
FINISH ALPHA CHANGE 2665 -- describe buffered failure injection after memory publication is corrected --
*/
type issue7ABufferedStorage struct {
	Storage
	writeErr error
}

type issue7ABufferedBatch struct {
	storage *issue7ABufferedStorage
	entries [][2][]byte
}

func (s *issue7ABufferedStorage) Batch() Batch {
	return &issue7ABufferedBatch{storage: s}
}

func (b *issue7ABufferedBatch) Put(key, value []byte) {
	b.entries = append(b.entries, [2][]byte{
		append([]byte(nil), key...),
		append([]byte(nil), value...),
	})
}

func (b *issue7ABufferedBatch) Write() error {
	if b.storage.writeErr != nil {
		return b.storage.writeErr
	}

	// The backing store in these tests is memory storage. Writes cannot
	// fail there; the injected failure occurs before publishing any entry.
	for _, entry := range b.entries {
		if err := b.storage.Storage.Put(entry[0], entry[1]); err != nil {
			return err
		}
	}

	return nil
}

func TestIssue7A_FailedBatchDoesNotPublishDerivedCaches(t *testing.T) {
	backing := NewMemoryStorage()
	storage := &issue7ABufferedStorage{Storage: backing}
	world := NewState(storage)
	initial := world.NewSnapshot()

	address := types.StringToAddress("0x4001")
	slot := types.StringToHash("0x01")
	oldValue := types.StringToHash("0x2a")
	newValue := types.StringToHash("0x2b")

	initialTxn := statepkg.NewTxn(initial)
	initialTxn.SetState(address, slot, oldValue)
	objects, err := initialTxn.Commit(false)
	require.NoError(t, err)

	committed, originalRoot, err := initial.Commit(objects)
	require.NoError(t, err)

	cacheKeys := append([]interface{}(nil), world.cache.Keys()...)
	memory := backing.(*memStorage)
	recordsBefore := make(map[string][]byte, len(memory.db))
	for key, value := range memory.db {
		recordsBefore[key] = append([]byte(nil), value...)
	}

	nextTxn := statepkg.NewTxn(committed)
	nextTxn.SetState(address, slot, newValue)
	nextObjects, err := nextTxn.Commit(false)
	require.NoError(t, err)

	injected := errors.New("injected batch publication failure")
	storage.writeErr = injected

	nextSnapshot, nextRoot, err := committed.Commit(nextObjects)
	require.Nil(t, nextSnapshot)
	require.Equal(t, types.ZeroHash[:], nextRoot)
	require.ErrorIs(t, err, injected)
	require.ErrorIs(t, err, statepkg.ErrLocalStateFailure)

	require.ElementsMatch(t, cacheKeys, world.cache.Keys())
	require.Equal(t, recordsBefore, memory.db)

	/* START ALPHA CHANGE 2180 -- verify previously shared tries after failed batch publication -- */
	// Check the original snapshot directly, rather than only reconstructing
	// a fresh trie from the backing database.
	originalAccount, err := committed.GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, originalAccount)

	originalValue, err := committed.GetStorage(address, originalAccount.Root, slot)
	require.NoError(t, err)
	require.Equal(t, oldValue, originalValue)

	// Also open through the same State instance and its existing cache.
	cachedSnapshot, err := world.NewSnapshotAt(types.BytesToHash(originalRoot))
	require.NoError(t, err)

	cachedAccount, err := cachedSnapshot.GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, cachedAccount)

	cachedValue, err := cachedSnapshot.GetStorage(address, cachedAccount.Root, slot)
	require.NoError(t, err)
	require.Equal(t, oldValue, cachedValue)
	/* FINISH ALPHA CHANGE 2180 -- verify previously shared tries after failed batch publication -- */

	// The previously published state must remain usable.
	reopened, err := NewState(storage).NewSnapshotAt(
		types.BytesToHash(originalRoot),
	)
	require.NoError(t, err)

	account, err := reopened.GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, account)

	value, err := reopened.GetStorage(address, account.Root, slot)
	require.NoError(t, err)
	require.Equal(t, oldValue, value)
}

/* FINISH ALPHA CHANGE 2151 -- test trie failures and buffered publication boundaries -- */
