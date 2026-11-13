package state

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/umbracle/fastrlp"

	"github.com/innovativebioresearch/innbc-chain/types"
)

/* START ALPHA CHANGE 2128 -- classify local state failures without changing valid-state rules -- */
var (
	ErrLocalStateFailure = errors.New("local state failure")
	ErrMissingTrieNode   = errors.New("referenced trie node is missing")
	ErrMissingCode       = errors.New("referenced contract code is missing")

	ErrMalformedTrieNode = errors.New("malformed stored trie node")
	ErrMalformedAccount  = errors.New("malformed stored account")
	ErrMalformedStorage  = errors.New("malformed stored storage value")
)

// WrapLocalStateFailure preserves the original cause and adds the general
// classification used by execution, block construction, and RPC boundaries.
func WrapLocalStateFailure(operation string, cause error) error {
	if cause == nil {
		return nil
	}

	return fmt.Errorf(
		"%w: %s: %w",
		ErrLocalStateFailure,
		operation,
		cause,
	)
}

/* FINISH ALPHA CHANGE 2128 -- classify local state failures without changing valid-state rules -- */

type State interface {
	NewSnapshotAt(types.Hash) (Snapshot, error)
	NewSnapshot() Snapshot
	GetCode(hash types.Hash) ([]byte, bool, error)
}

type Snapshot interface {
	readSnapshot

	Commit(objs []*Object) (Snapshot, []byte, error)
}

/* START ALPHA CHANGE 2654 -- expose validation before snapshot publication without changing legacy snapshots -- */
/*
	ValidatingSnapshot calculates the resulting root and stages changes before
	calling validate. A validation error must prevent storage and derived-cache
	publication and must be returned without local-state failure wrapping.

	The callback validates the supplied root; it must not publish or mutate
	the snapshot.
*/
type ValidatingSnapshot interface {
	Snapshot

	CommitWithValidation(
		objs []*Object,
		validate func(types.Hash) error,
	) (Snapshot, []byte, error)
}

/* FINISH ALPHA CHANGE 2654 -- expose validation before snapshot publication without changing legacy snapshots -- */

// Account is the account reference in the ethereum state
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     types.Hash
	CodeHash []byte
}

func (a *Account) MarshalWith(ar *fastrlp.Arena) *fastrlp.Value {
	v := ar.NewArray()
	v.Set(ar.NewUint(a.Nonce))
	v.Set(ar.NewBigInt(a.Balance))
	v.Set(ar.NewBytes(a.Root.Bytes()))
	v.Set(ar.NewBytes(a.CodeHash))

	return v
}

var accountParserPool fastrlp.ParserPool

func (a *Account) UnmarshalRlp(b []byte) error {
	p := accountParserPool.Get()
	defer accountParserPool.Put(p)

	v, err := p.Parse(b)
	if err != nil {
		return err
	}

	elems, err := v.GetElems()

	if err != nil {
		return err
	}

	if len(elems) < 4 {
		return fmt.Errorf("incorrect number of elements to decode account, expected 4 but found %d", len(elems))
	}

	// nonce
	if a.Nonce, err = elems[0].GetUint64(); err != nil {
		return err
	}
	// balance
	if a.Balance == nil {
		a.Balance = new(big.Int)
	}

	if err = elems[1].GetBigInt(a.Balance); err != nil {
		return err
	}
	// root
	if err = elems[2].GetHash(a.Root[:]); err != nil {
		return err
	}
	// codeHash
	if a.CodeHash, err = elems[3].GetBytes(a.CodeHash[:0]); err != nil {
		return err
	}

	return nil
}

func (a *Account) String() string {
	return fmt.Sprintf("%d %s", a.Nonce, a.Balance.String())
}

func (a *Account) Copy() *Account {
	aa := new(Account)

	aa.Balance = big.NewInt(1).SetBytes(a.Balance.Bytes())
	aa.Nonce = a.Nonce
	aa.CodeHash = a.CodeHash
	aa.Root = a.Root

	return aa
}

// StateObject is the internal representation of the account
type StateObject struct {
	Account   *Account
	Code      []byte
	Suicide   bool
	Deleted   bool
	DirtyCode bool
	Txn       *iradix.Txn

	// withFakeStorage signals whether the state object
	// is using the override full state
	withFakeStorage bool

	/* START ALPHA CHANGE 3148 -- identify storage resets within the active transaction without a slot cache -- */
	storageResetAt *iradix.Tree
	/* FINISH ALPHA CHANGE 3148 -- identify storage resets within the active transaction without a slot cache -- */
}

func (s *StateObject) Empty() bool {
	return s.Account.Nonce == 0 &&
		s.Account.Balance.Sign() == 0 &&
		bytes.Equal(s.Account.CodeHash, types.EmptyCodeHash.Bytes())
}

// Copy makes a copy of the state object
func (s *StateObject) Copy() *StateObject {
	ss := new(StateObject)

	// copy account
	ss.Account = s.Account.Copy()

	ss.Suicide = s.Suicide
	ss.Deleted = s.Deleted
	ss.DirtyCode = s.DirtyCode
	ss.Code = s.Code
	ss.withFakeStorage = s.withFakeStorage

	/* START ALPHA CHANGE 3148 -- preserve storage-reset identity through account copies and rollback -- */
	ss.storageResetAt = s.storageResetAt
	/* FINISH ALPHA CHANGE 3148 -- preserve storage-reset identity through account copies and rollback -- */

	if s.Txn != nil {
		ss.Txn = s.Txn.CommitOnly().Txn()
	}

	return ss
}

// Object is the serialization of the radix object (can be merged to StateObject?).
type Object struct {
	Address  types.Address
	CodeHash types.Hash
	Balance  *big.Int
	Root     types.Hash
	Nonce    uint64
	Deleted  bool

	//nolint:godox
	// TODO: Move this to executor (to be fixed in EVM-527)
	DirtyCode bool
	Code      []byte

	Storage []*StorageObject
}

// StorageObject is an entry in the storage
type StorageObject struct {
	Deleted bool
	Key     []byte
	Val     []byte
}
