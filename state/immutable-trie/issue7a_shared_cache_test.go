/* START ALPHA CHANGE 2194 -- test explicit leaf presence and concurrent immutable trie caches -- */
package itrie

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"

	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestIssue7A_EmptyLeafIsFoundBeforeValueDecoding(t *testing.T) {
	storage := NewMemoryStorage()
	key := []byte{0x01}
	root := issue7APersistRawValue(t, storage, key, nil)

	node, found, err := GetNode(root.Bytes(), storage)
	require.NoError(t, err)
	require.True(t, found)

	trie := &Trie{root: node}
	value, found, err := trie.Get(key, storage)
	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, value)

	value, found, err = trie.Get([]byte{0x02}, storage)
	require.NoError(t, err)
	require.False(t, found)
	require.Nil(t, value)
}

type issue7AResolutionStorage struct {
	Storage
	readErr error
	reads   int
}

func (s *issue7AResolutionStorage) Get(key []byte) ([]byte, bool, error) {
	s.reads++
	if s.readErr != nil {
		return nil, false, s.readErr
	}

	return s.Storage.Get(key)
}

func TestIssue7A_ResolutionFailureIsNotCached(t *testing.T) {
	backing := NewMemoryStorage()
	key := []byte{0x01}
	value := bytes.Repeat([]byte{0x2a}, 40)
	root := issue7APersistRawValue(t, backing, key, value)

	cause := errors.New("injected resolution failure")
	storage := &issue7AResolutionStorage{
		Storage: backing,
		readErr: cause,
	}
	reference := &ValueNode{hash: true, buf: root.Bytes()}
	trie := &Trie{root: reference}

	result, found, err := trie.Get(key, storage)
	require.Nil(t, result)
	require.False(t, found)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, statepkg.ErrLocalStateFailure)
	require.Nil(t, reference.resolved.Load())

	// A new lookup may retry a previously unsuccessful resolution.
	storage.readErr = nil
	result, found, err = trie.Get(key, storage)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, value, result)
	require.NotNil(t, reference.resolved.Load())

	readsAfterSuccess := storage.reads
	storage.readErr = cause

	// Successfully cached data remains usable without a new backend read.
	result, found, err = trie.Get(key, storage)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, value, result)
	require.Equal(t, readsAfterSuccess, storage.reads)

	// Mutation can also reuse the successful resolution. Copy-on-write
	// must preserve the original shared trie.
	mutation := trie.Txn(storage)
	replacement := bytes.Repeat([]byte{0x2b}, 40)
	mutation.Insert(key, replacement)
	require.NoError(t, mutation.Error())
	_, err = mutation.Hash()
	require.NoError(t, err)

	result, found, err = trie.Get(key, storage)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, value, result)
	require.Equal(t, readsAfterSuccess, storage.reads)
}

func TestIssue7A_SharedTrieConcurrentReadsAndHashing(t *testing.T) {
	storage := NewMemoryStorage()
	key := []byte{0x01}
	otherKey := []byte{0x02}
	original := bytes.Repeat([]byte{0x2a}, 40)
	otherValue := bytes.Repeat([]byte{0x3b}, 40)

	batch := storage.Batch()
	writer := NewTrie().Txn(storage)
	writer.batch = batch
	writer.Insert(key, original)
	writer.Insert(otherKey, otherValue)

	root, err := writer.Hash()
	require.NoError(t, err)
	require.NoError(t, batch.Write())

	node, found, err := GetNode(root, storage)
	require.NoError(t, err)
	require.True(t, found)

	// Publish the same decoded root through the actual State cache.
	world := NewState(storage)
	rootHash := types.BytesToHash(root)
	world.AddState(rootHash, &Trie{root: node})

	const workers = 12
	const iterations = 50

	start := make(chan struct{})
	results := make(chan error, workers)
	var wg sync.WaitGroup

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			results <- func() error {
				shared, err := world.newTrieAt(rootHash)
				if err != nil {
					return err
				}

				for iteration := 0; iteration < iterations; iteration++ {
					value, found, err := shared.Get(key, storage)
					if err != nil {
						return err
					}
					if !found || !bytes.Equal(value, original) {
						return errors.New("shared read changed the original value")
					}

					// Independent transactions hash the same shared nodes.
					hashTxn := shared.Txn(storage)
					actualRoot, err := hashTxn.Hash()
					if err != nil {
						return err
					}
					if !bytes.Equal(root, actualRoot) {
						return errors.New("shared trie root changed")
					}

					// Mutations use private paths and do not publish a batch.
					mutation := shared.Txn(storage)
					mutation.Insert(key, bytes.Repeat([]byte{0x4c}, 40))
					if err := mutation.Error(); err != nil {
						return err
					}
					mutation.Delete(otherKey)
					if err := mutation.Error(); err != nil {
						return err
					}
					if _, err := mutation.Hash(); err != nil {
						return err
					}
					if _, err := mutation.Commit(); err != nil {
						return err
					}

					value, found, err = shared.Get(otherKey, storage)
					if err != nil {
						return err
					}
					if !found || !bytes.Equal(value, otherValue) {
						return fmt.Errorf(
							"private mutation changed shared state at iteration %d",
							iteration,
						)
					}
				}

				return nil
			}()
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	for err := range results {
		require.NoError(t, err)
	}
}

func TestIssue7A_HashCacheOwnsPublishedBytes(t *testing.T) {
	node := &ShortNode{}
	input := bytes.Repeat([]byte{0x5d}, 32)

	first := node.SetHash(input)
	input[0] = 0xff

	require.Equal(t, byte(0x5d), first[0])

	secondInput := bytes.Repeat([]byte{0x6e}, 32)
	node.SetHash(secondInput)

	// Later publication must not overwrite bytes previously returned.
	require.Equal(t, bytes.Repeat([]byte{0x5d}, 32), first)

	current, found := node.Hash()
	require.True(t, found)
	require.Equal(t, secondInput, current)
}

/* FINISH ALPHA CHANGE 2194 -- test explicit leaf presence and concurrent immutable trie caches -- */
