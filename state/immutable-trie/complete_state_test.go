/* START ALPHA CHANGE 2105 -- test executable-state completeness and strict snapshot decoding -- */
package itrie

import (
	"bytes"
	"errors"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

type executableStateFixture struct {
	storage Storage
	root    types.Hash
	address types.Address
	key     types.Hash
	value   types.Hash
	account *statepkg.Account
	code    []byte
}

func newExecutableStateFixture(t *testing.T) executableStateFixture {
	t.Helper()

	storage := NewMemoryStorage()
	snapshot := NewState(storage).NewSnapshot()
	txn := statepkg.NewTxn(snapshot)
	address := types.StringToAddress("0x1001")
	key := types.StringToHash("0x01")
	value := types.StringToHash("0x1234")
	code := []byte{0x60, 0x00, 0x60, 0x00, 0xf3}

	txn.SetState(address, key, value)
	txn.SetCode(address, code)
	objects, err := txn.Commit(false)
	require.NoError(t, err)

	committed, root, err := snapshot.Commit(objects)
	require.NoError(t, err)
	account, err := committed.GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, account)

	return executableStateFixture{
		storage: storage,
		root:    types.BytesToHash(root),
		address: address,
		key:     key,
		value:   value,
		account: account,
		code:    code,
	}
}

type snapshotFaultStorage struct {
	Storage
	key []byte
	err error
}

func (s *snapshotFaultStorage) Get(key []byte) ([]byte, bool, error) {
	if bytes.Equal(key, s.key) {
		return nil, false, s.err
	}

	return s.Storage.Get(key)
}

func TestCopyTrieCopiesCompleteExecutableState(t *testing.T) {
	f := newExecutableStateFixture(t)
	target := NewMemoryStorage()

	require.NoError(t, CopyTrie(f.root.Bytes(), f.storage, target, nil, false))
	require.NoError(t, ValidateCompleteState(f.root, target))

	snapshot, err := NewState(target).NewSnapshotAt(f.root)
	require.NoError(t, err)
	account, err := snapshot.GetAccount(f.address)
	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, f.account.Root, account.Root)
	/* START ALPHA CHANGE 2153 -- check complete-state fixture storage and code reads -- */
	checkedStorage1, storageErr1 := snapshot.GetStorage(f.address, account.Root, f.key)
	require.NoError(t, storageErr1)
	require.Equal(t, f.value, checkedStorage1)

	code, ok, codeErr := snapshot.GetCode(types.BytesToHash(account.CodeHash))
	require.NoError(t, codeErr)
	require.True(t, ok)
	require.Equal(t, f.code, code)
	/* FINISH ALPHA CHANGE 2153 -- check complete-state fixture storage and code reads -- */
}

func TestCompleteStateRejectsMissingReferencesAndReadErrors(t *testing.T) {
	f := newExecutableStateFixture(t)
	tests := []struct {
		name string
		key  []byte
	}{
		{"account root", f.root.Bytes()},
		{"storage root", f.account.Root.Bytes()},
		{"code", GetCodeKey(types.BytesToHash(f.account.CodeHash))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			missing := &snapshotFaultStorage{Storage: f.storage, key: test.key}
			require.ErrorContains(t, ValidateCompleteState(f.root, missing), "missing")
			require.Error(t, CopyTrie(f.root.Bytes(), missing, NewMemoryStorage(), nil, false))

			readErr := errors.New("forced snapshot read failure")
			failing := &snapshotFaultStorage{
				Storage: f.storage,
				key:     test.key,
				err:     readErr,
			}
			require.ErrorIs(t, ValidateCompleteState(f.root, failing), readErr)
		})
	}
}

func TestCompleteStateRejectsHashMismatches(t *testing.T) {
	for _, kind := range []string{"account", "storage", "code"} {
		t.Run(kind, func(t *testing.T) {
			f := newExecutableStateFixture(t)
			key := f.root.Bytes()
			if kind == "storage" {
				key = f.account.Root.Bytes()
			}
			if kind == "code" {
				key = GetCodeKey(types.BytesToHash(f.account.CodeHash))
			}

			require.NoError(t, f.storage.Put(key, []byte{0xc0}))
			require.ErrorContains(t, ValidateCompleteState(f.root, f.storage), "hash mismatch")
		})
	}
}

func TestCompleteStateAcceptsEmptyStateAndAccount(t *testing.T) {
	require.NoError(t, ValidateCompleteState(types.EmptyRootHash, NewMemoryStorage()))

	storage := NewMemoryStorage()
	snapshot := NewState(storage).NewSnapshot()
	txn := statepkg.NewTxn(snapshot)
	txn.SetNonce(types.StringToAddress("0x1001"), 1)
	objects, err := txn.Commit(false)
	require.NoError(t, err)
	_, root, err := snapshot.Commit(objects)
	require.NoError(t, err)
	require.NoError(t, ValidateCompleteState(types.BytesToHash(root), storage))
}

func snapshotTestAccountRLP(
	nonce []byte,
	balance []byte,
	root []byte,
	code []byte,
	extra bool,
) []byte {
	arena := &fastrlp.Arena{}
	value := arena.NewArray()
	value.Set(arena.NewBytes(nonce))
	value.Set(arena.NewBytes(balance))
	value.Set(arena.NewBytes(root))
	value.Set(arena.NewBytes(code))
	if extra {
		value.Set(arena.NewUint(1))
	}

	return value.MarshalTo(nil)
}

func TestStrictSnapshotAccountEncoding(t *testing.T) {
	valid := snapshotTestAccountRLP(
		nil, nil, types.EmptyRootHash.Bytes(), types.EmptyCodeHash.Bytes(), false,
	)
	_, err := decodeStrictSnapshotAccount(valid)
	require.NoError(t, err)

	tests := [][]byte{
		append(append([]byte(nil), valid...), 0),
		snapshotTestAccountRLP(nil, nil, types.EmptyRootHash.Bytes(), types.EmptyCodeHash.Bytes(), true),
		snapshotTestAccountRLP([]byte{0}, nil, types.EmptyRootHash.Bytes(), types.EmptyCodeHash.Bytes(), false),
		snapshotTestAccountRLP(nil, []byte{0, 1}, types.EmptyRootHash.Bytes(), types.EmptyCodeHash.Bytes(), false),
		snapshotTestAccountRLP(nil, nil, []byte{1}, types.EmptyCodeHash.Bytes(), false),
		snapshotTestAccountRLP(nil, nil, types.EmptyRootHash.Bytes(), []byte{1}, false),
	}
	for _, raw := range tests {
		_, err := decodeStrictSnapshotAccount(raw)
		require.Error(t, err)
	}
}

func TestStrictSnapshotStorageEncoding(t *testing.T) {
	require.NoError(t, validateSnapshotStorageValue([]byte{0x01}))
	require.NoError(t, validateSnapshotStorageValue([]byte{0x80}))

	for _, raw := range [][]byte{
		{0x01, 0x00},
		{0xc0},
		{0x00},
		{0x82, 0x00, 0x01},
		append([]byte{0xa1}, bytes.Repeat([]byte{1}, 33)...),
	} {
		require.Error(t, validateSnapshotStorageValue(raw))
	}
}

func TestSnapshotNodeLoadingChecksShortRootAndTrailingData(t *testing.T) {
	arena := &fastrlp.Arena{}
	leaf := arena.NewArray()
	leaf.Set(arena.NewBytes(encodeCompact([]byte{16})))
	leaf.Set(arena.NewBytes([]byte{1}))
	raw := leaf.MarshalTo(nil)
	require.Less(t, len(raw), types.HashLength)

	storage := NewMemoryStorage()
	hash := crypto.Keccak256(raw)
	require.NoError(t, storage.Put(hash, raw))
	_, stored, err := getCustomNode(hash, storage)
	require.NoError(t, err)
	require.Equal(t, raw, stored)

	// A matching hash must not make trailing RLP bytes acceptable.
	withTrailing := append(append([]byte(nil), raw...), 0)
	trailingHash := crypto.Keccak256(withTrailing)
	require.NoError(t, storage.Put(trailingHash, withTrailing))
	_, _, err = getCustomNode(trailingHash, storage)
	require.ErrorContains(t, err, "trailing bytes")
}

/* START ALPHA CHANGE 2111 -- exercise the depth bound with a branching account trie -- */
func TestCompleteStateHonorsDepthBound(t *testing.T) {
	storage := NewMemoryStorage()
	snapshot := NewState(storage).NewSnapshot()
	txn := statepkg.NewTxn(snapshot)

	txn.SetNonce(types.StringToAddress("0x1001"), 1)
	txn.SetNonce(types.StringToAddress("0x1002"), 1)

	objects, err := txn.Commit(false)
	require.NoError(t, err)

	_, rootBytes, err := snapshot.Commit(objects)
	require.NoError(t, err)
	root := types.BytesToHash(rootBytes)

	// Establish that the fixture is valid before restricting traversal.
	require.NoError(t, ValidateCompleteState(root, storage))

	old := defaultTrieCheckMaxDepth
	t.Cleanup(func() {
		defaultTrieCheckMaxDepth = old
	})
	defaultTrieCheckMaxDepth = 1

	require.ErrorContains(
		t,
		ValidateCompleteState(root, storage),
		"maximum depth",
	)
}

/* FINISH ALPHA CHANGE 2111 -- exercise the depth bound with a branching account trie -- */

/* FINISH ALPHA CHANGE 2105 -- test executable-state completeness and strict snapshot decoding -- */
