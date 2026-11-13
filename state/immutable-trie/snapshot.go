package itrie

import (
	"bytes"
	"fmt"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/fastrlp"
)

type Snapshot struct {
	state *State
	trie  *Trie
}

var emptyStateHash = types.StringToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

/* START ALPHA CHANGE 2140 -- preserve legitimate storage absence and propagate failed reads -- */
func (s *Snapshot) GetStorage(
	addr types.Address,
	root types.Hash,
	rawkey types.Hash,
) (types.Hash, error) {
	if root == emptyStateHash {
		return types.ZeroHash, nil
	}

	trie, err := s.state.newTrieAt(root)
	if err != nil {
		return types.ZeroHash, state.WrapLocalStateFailure(
			fmt.Sprintf("open storage trie %s for account %s", root, addr),
			err,
		)
	}

	key := crypto.Keccak256(rawkey.Bytes())
	encoded, found, err := trie.Get(key, s.state.storage)
	if err != nil {
		return types.ZeroHash, state.WrapLocalStateFailure(
			fmt.Sprintf("read storage slot %s for account %s", rawkey, addr),
			err,
		)
	}
	if !found {
		return types.ZeroHash, nil
	}

	parser := &fastrlp.Parser{}
	value, err := parser.Parse(encoded)
	if err != nil {
		return types.ZeroHash, state.WrapLocalStateFailure(
			fmt.Sprintf("decode storage slot %s for account %s", rawkey, addr),
			fmt.Errorf("%w: %w", state.ErrMalformedStorage, err),
		)
	}

	rawValue, err := value.GetBytes(nil)
	if err != nil {
		return types.ZeroHash, state.WrapLocalStateFailure(
			fmt.Sprintf("decode storage slot %s for account %s", rawkey, addr),
			fmt.Errorf("%w: %w", state.ErrMalformedStorage, err),
		)
	}

	return types.BytesToHash(rawValue), nil
}

/* FINISH ALPHA CHANGE 2140 -- preserve legitimate storage absence and propagate failed reads -- */

/* START ALPHA CHANGE 2141 -- propagate account and code read failures -- */
func (s *Snapshot) GetAccount(addr types.Address) (*state.Account, error) {
	key := crypto.Keccak256(addr.Bytes())

	data, found, err := s.trie.Get(key, s.state.storage)
	if err != nil {
		return nil, state.WrapLocalStateFailure(
			fmt.Sprintf("read account %s", addr),
			err,
		)
	}
	if !found {
		return nil, nil
	}

	var account state.Account
	if err := account.UnmarshalRlp(data); err != nil {
		return nil, state.WrapLocalStateFailure(
			fmt.Sprintf("decode account %s", addr),
			fmt.Errorf("%w: %w", state.ErrMalformedAccount, err),
		)
	}

	return &account, nil
}

func (s *Snapshot) GetCode(hash types.Hash) ([]byte, bool, error) {
	return s.state.GetCode(hash)
}

/* FINISH ALPHA CHANGE 2141 -- propagate account and code read failures -- */

/* START ALPHA CHANGE 2149 -- publish derived tries only after the existing database batch succeeds -- */
/* START ALPHA CHANGE 2657 -- share trie preparation between ordinary and guarded commits -- */
func (s *Snapshot) Commit(
	objs []*state.Object,
) (state.Snapshot, []byte, error) {
	return s.commit(objs, nil)
}

func (s *Snapshot) CommitWithValidation(
	objs []*state.Object,
	validate func(types.Hash) error,
) (state.Snapshot, []byte, error) {
	return s.commit(objs, validate)
}

func (s *Snapshot) commit(
	objs []*state.Object,
	validate func(types.Hash) error,
) (state.Snapshot, []byte, error) {
	batch := s.state.storage.Batch()
	accountTxn := s.trie.Txn(s.state.storage)
	accountTxn.batch = batch

	type pendingTrie struct {
		root types.Hash
		trie *Trie
	}

	var pendingStorageTries []pendingTrie

	arena := stateArenaPool.Get()
	defer stateArenaPool.Put(arena)

	for _, obj := range objs {
		if obj.Deleted {
			accountTxn.Delete(hashit(obj.Address.Bytes()))
			if err := accountTxn.Error(); err != nil {
				return nil, types.ZeroHash[:], err
			}

			continue
		}

		account := state.Account{
			Balance:  obj.Balance,
			Nonce:    obj.Nonce,
			CodeHash: obj.CodeHash.Bytes(),
			Root:     obj.Root,
		}

		if len(obj.Storage) != 0 {
			storageTrie, err := s.state.newTrieAt(obj.Root)
			if err != nil {
				return nil, types.ZeroHash[:], state.WrapLocalStateFailure(
					fmt.Sprintf("open storage for account %s during commit", obj.Address),
					err,
				)
			}

			storageTxn := storageTrie.Txn(s.state.storage)
			storageTxn.batch = batch

			for _, entry := range obj.Storage {
				key := hashit(entry.Key)
				if entry.Deleted {
					storageTxn.Delete(key)
				} else {
					value := arena.NewBytes(bytes.TrimLeft(entry.Val, "\x00"))
					storageTxn.Insert(key, value.MarshalTo(nil))
				}

				if err := storageTxn.Error(); err != nil {
					return nil, types.ZeroHash[:], err
				}
			}

			storageRoot, err := storageTxn.Hash()
			if err != nil {
				return nil, types.ZeroHash[:], state.WrapLocalStateFailure(
					fmt.Sprintf("calculate storage root for account %s", obj.Address),
					err,
				)
			}

			newStorageTrie, err := storageTxn.Commit()
			if err != nil {
				return nil, types.ZeroHash[:], err
			}

			account.Root = types.BytesToHash(storageRoot)
			pendingStorageTries = append(
				pendingStorageTries,
				pendingTrie{
					root: account.Root,
					trie: newStorageTrie,
				},
			)
		}

		if obj.DirtyCode {
			batch.Put(GetCodeKey(obj.CodeHash), obj.Code)
		}

		value := account.MarshalWith(arena)
		accountTxn.Insert(
			hashit(obj.Address.Bytes()),
			value.MarshalTo(nil),
		)
		if err := accountTxn.Error(); err != nil {
			return nil, types.ZeroHash[:], err
		}

		arena.Reset()
	}

	root, err := accountTxn.Hash()
	if err != nil {
		return nil, types.ZeroHash[:], state.WrapLocalStateFailure(
			"calculate account trie root",
			err,
		)
	}

	newAccountTrie, err := accountTxn.Commit()
	if err != nil {
		return nil, types.ZeroHash[:], err
	}

	/* START ALPHA CHANGE 2658 -- reject incorrect commitments before storage or derived-cache publication -- */
	if validate != nil {
		if err := validate(types.BytesToHash(root)); err != nil {
			return nil, types.ZeroHash[:], err
		}
	}
	/* FINISH ALPHA CHANGE 2658 -- reject incorrect commitments before storage or derived-cache publication -- */

	// Use the existing batch publication. No additional synchronization
	// or MDBX durability operation is introduced.
	if err := batch.Write(); err != nil {
		return nil, types.ZeroHash[:], state.WrapLocalStateFailure(
			"publish state database batch",
			err,
		)
	}

	for _, pending := range pendingStorageTries {
		s.state.AddState(pending.root, pending.trie)
	}
	s.state.AddState(types.BytesToHash(root), newAccountTrie)

	return &Snapshot{
		trie:  newAccountTrie,
		state: s.state,
	}, root, nil
}

/* FINISH ALPHA CHANGE 2657 -- share trie preparation between ordinary and guarded commits -- */
/* FINISH ALPHA CHANGE 2149 -- publish derived tries only after the existing database batch succeeds -- */
