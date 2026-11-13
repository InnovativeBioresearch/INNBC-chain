package itrie

import (
	"fmt"

	lru "github.com/hashicorp/golang-lru"

	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
)

type State struct {
	storage Storage
	cache   *lru.Cache
}

func NewState(storage Storage) *State {
	cache, _ := lru.New(128)

	s := &State{
		storage: storage,
		cache:   cache,
	}

	return s
}

func (s *State) NewSnapshot() state.Snapshot {
	return &Snapshot{state: s, trie: s.newTrie()}
}

func (s *State) NewSnapshotAt(root types.Hash) (state.Snapshot, error) {
	t, err := s.newTrieAt(root)
	if err != nil {
		return nil, err
	}

	return &Snapshot{state: s, trie: t}, nil
}

func (s *State) newTrie() *Trie {
	return NewTrie()
}

func (s *State) SetCode(hash types.Hash, code []byte) error {
	return s.storage.SetCode(hash, code)
}

/* START ALPHA CHANGE 2142 -- distinguish canonical empty code from missing referenced code -- */
func (s *State) GetCode(hash types.Hash) ([]byte, bool, error) {
	if hash == types.EmptyCodeHash {
		return []byte{}, true, nil
	}

	code, found, err := s.storage.GetCode(hash)
	if err != nil {
		return nil, false, state.WrapLocalStateFailure(
			fmt.Sprintf("read contract code %s", hash),
			err,
		)
	}
	/* START ALPHA CHANGE 2185 -- reject empty records for nonempty code hashes -- */
	if !found || len(code) == 0 {
		return nil, false, state.WrapLocalStateFailure(
			fmt.Sprintf("read contract code %s", hash),
			fmt.Errorf("%w: %s", state.ErrMissingCode, hash),
		)
	}
	/* FINISH ALPHA CHANGE 2185 -- reject empty records for nonempty code hashes -- */

	return code, true, nil
}

/* FINISH ALPHA CHANGE 2142 -- distinguish canonical empty code from missing referenced code -- */

// newTrieAt returns trie with root and if necessary locks state on a trie level
func (s *State) newTrieAt(root types.Hash) (*Trie, error) {
	if root == types.EmptyRootHash {
		// empty state
		return s.newTrie(), nil
	}

	tt, ok := s.cache.Get(root)
	if ok {
		t, ok := tt.(*Trie)
		/* START ALPHA CHANGE 2143 -- classify invalid cached trie state -- */
		if !ok || t == nil {
			return nil, state.WrapLocalStateFailure(
				fmt.Sprintf("read cached trie %s", root),
				fmt.Errorf("invalid cached trie value"),
			)
		}
		/* FINISH ALPHA CHANGE 2143 -- classify invalid cached trie state -- */

		return t, nil
	}

	/* START ALPHA CHANGE 2143 -- classify missing and unreadable trie roots -- */
	n, found, err := GetNode(root.Bytes(), s.storage)
	if err != nil {
		return nil, state.WrapLocalStateFailure(
			fmt.Sprintf("load trie root %s", root),
			err,
		)
	}
	if !found {
		return nil, state.WrapLocalStateFailure(
			fmt.Sprintf("load trie root %s", root),
			fmt.Errorf("%w: %s", state.ErrMissingTrieNode, root),
		)
	}
	/* FINISH ALPHA CHANGE 2143 -- classify missing and unreadable trie roots -- */

	t := &Trie{
		root: n,
	}

	return t, nil
}

func (s *State) AddState(root types.Hash, t *Trie) {
	s.cache.Add(root, t)
}
