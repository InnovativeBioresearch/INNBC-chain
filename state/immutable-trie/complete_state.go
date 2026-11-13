/* START ALPHA CHANGE 2101 -- verify complete snapshot state using persisted node commitments -- */
package itrie

import (
	"bytes"
	"errors"
	"fmt"

	lru "github.com/hashicorp/golang-lru"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/fastrlp"
)

const completeStateValidationCacheEntries = 4096

// parseSnapshotRLP requires one complete, canonically encoded RLP value.
func parseSnapshotRLP(p *fastrlp.Parser, raw []byte) (*fastrlp.Value, error) {
	value, err := p.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(p.Raw(value), raw) {
		return nil, errors.New("snapshot RLP contains trailing bytes")
	}
	if !bytes.Equal(value.MarshalTo(nil), raw) {
		return nil, errors.New("snapshot RLP is not canonical")
	}

	return value, nil
}

func readSnapshotTrieNode(hash []byte, storage Storage) ([]byte, error) {
	if len(hash) != types.HashLength {
		return nil, fmt.Errorf("snapshot trie reference must contain %d bytes", types.HashLength)
	}

	data, ok, err := storage.Get(hash)
	if err != nil {
		return nil, fmt.Errorf("read snapshot trie node %x: %w", hash, err)
	}
	if !ok || len(data) == 0 {
		return nil, fmt.Errorf("referenced snapshot trie node %x is missing", hash)
	}
	if !bytes.Equal(crypto.Keccak256(data), hash) {
		return nil, fmt.Errorf("snapshot trie node hash mismatch for %x", hash)
	}

	return data, nil
}

// validateSnapshotNodeShape validates inline structure before decodeNode.
// It does not interpret generic trie leaf values.
func validateSnapshotNodeShape(value *fastrlp.Value, depth int) error {
	if defaultTrieCheckMaxDepth > 0 && depth > defaultTrieCheckMaxDepth {
		return errors.New("snapshot inline trie exceeds maximum depth")
	}
	if value.Type() != fastrlp.TypeArray {
		return errors.New("snapshot trie node must be an RLP array")
	}

	elements, err := value.GetElems()
	if err != nil {
		return err
	}

	switch len(elements) {
	case 2:
		key, err := elements[0].GetBytes(nil)
		if err != nil || len(key) == 0 {
			return errors.New("snapshot trie compact key is missing or invalid")
		}
		if key[0]>>4 > 3 {
			return errors.New("snapshot trie compact key has invalid flags")
		}

		decoded := decodeCompact(key)
		if !bytes.Equal(encodeCompact(decoded), key) {
			return errors.New("snapshot trie compact key is not canonical")
		}

		if hasTerminator(decoded) {
			if elements[1].Type() != fastrlp.TypeBytes {
				return errors.New("snapshot trie leaf must contain bytes")
			}

			return nil
		}

		if len(decoded) == 0 {
			return errors.New("snapshot trie extension has an empty path")
		}

		return validateSnapshotChild(elements[1], depth+1, false)

	case 17:
		for _, child := range elements[:16] {
			if err := validateSnapshotChild(child, depth+1, true); err != nil {
				return err
			}
		}
		if elements[16].Type() != fastrlp.TypeBytes {
			return errors.New("snapshot trie branch value must contain bytes")
		}

		return nil

	default:
		return fmt.Errorf("snapshot trie node has %d fields", len(elements))
	}
}

func validateSnapshotChild(value *fastrlp.Value, depth int, allowEmpty bool) error {
	switch value.Type() {
	case fastrlp.TypeBytes:
		size := len(value.Raw())
		if size == types.HashLength || (allowEmpty && size == 0) {
			return nil
		}

		return errors.New("snapshot trie child reference has invalid length")

	case fastrlp.TypeArray:
		if len(value.MarshalTo(nil)) >= types.HashLength {
			return errors.New("snapshot inline trie child must be shorter than 32 bytes")
		}

		return validateSnapshotNodeShape(value, depth)

	default:
		return errors.New("snapshot trie child has invalid RLP type")
	}
}

type completeStateValidator struct {
	storage          Storage
	validatedCode    *lru.Cache
	validatedStorage *lru.Cache
}

// ValidateCompleteState verifies every persisted node hash and all account,
// contract-storage and bytecode objects reachable from root.
//
// Root nodes are verified against their actual stored RLP bytes, including
// roots whose encoding is shorter than 32 bytes.
func ValidateCompleteState(root types.Hash, storage Storage) error {
	if storage == nil {
		return errors.New("snapshot trie storage is nil")
	}
	if root == types.EmptyRootHash {
		return nil
	}

	codeCache, err := lru.New(completeStateValidationCacheEntries)
	if err != nil {
		return err
	}
	storageCache, err := lru.New(completeStateValidationCacheEntries)
	if err != nil {
		return err
	}

	validator := &completeStateValidator{
		storage:          storage,
		validatedCode:    codeCache,
		validatedStorage: storageCache,
	}

	if err := validator.walkHash(root.Bytes(), 0, 0, false, true); err != nil {
		return fmt.Errorf("validate snapshot account trie: %w", err)
	}

	return nil
}

func (v *completeStateValidator) walkHash(
	hash []byte,
	nibbles int,
	depth int,
	isStorage bool,
	isRoot bool,
) error {
	if defaultTrieCheckMaxDepth > 0 && depth > defaultTrieCheckMaxDepth {
		return errors.New("snapshot trie validation exceeded maximum depth")
	}

	node, raw, err := getCustomNode(hash, v.storage)
	if err != nil {
		return err
	}
	if !isRoot && len(raw) < types.HashLength {
		return errors.New("snapshot short trie child must be embedded")
	}

	return v.walkNode(node, nibbles, depth, isStorage)
}

func (v *completeStateValidator) walkNode(
	node Node,
	nibbles int,
	depth int,
	isStorage bool,
) error {
	if defaultTrieCheckMaxDepth > 0 && depth > defaultTrieCheckMaxDepth {
		return errors.New("snapshot trie validation exceeded maximum depth")
	}
	if nibbles > types.HashLength*2 {
		return errors.New("snapshot secure trie key exceeds 64 nibbles")
	}

	switch n := node.(type) {
	case *ValueNode:
		if n.hash {
			return v.walkHash(n.buf, nibbles, depth+1, isStorage, false)
		}
		if nibbles != types.HashLength*2 {
			return errors.New("snapshot secure trie leaf key must contain 64 nibbles")
		}
		if isStorage {
			return validateSnapshotStorageValue(n.buf)
		}

		return v.validateAccount(n.buf)

	case *ShortNode:
		length := len(n.key)
		if hasTerminator(n.key) {
			length--
		}

		return v.walkNode(n.child, nibbles+length, depth+1, isStorage)

	case *FullNode:
		for _, child := range n.children {
			if child == nil {
				continue
			}
			if err := v.walkNode(child, nibbles+1, depth+1, isStorage); err != nil {
				return err
			}
		}
		if n.value != nil {
			return v.walkNode(n.value, nibbles, depth+1, isStorage)
		}

		return nil

	default:
		return fmt.Errorf("snapshot trie contains invalid node %T", node)
	}
}

func (v *completeStateValidator) validateAccount(raw []byte) error {
	account, err := decodeStrictSnapshotAccount(raw)
	if err != nil {
		return err
	}

	codeHash := types.BytesToHash(account.CodeHash)
	if codeHash != types.EmptyCodeHash {
		if _, ok := v.validatedCode.Get(codeHash); !ok {
			if _, err := readVerifiedSnapshotCode(v.storage, codeHash); err != nil {
				return err
			}
			v.validatedCode.Add(codeHash, struct{}{})
		}
	}

	if account.Root == types.EmptyRootHash {
		return nil
	}
	if _, ok := v.validatedStorage.Get(account.Root); ok {
		return nil
	}

	if err := v.walkHash(account.Root.Bytes(), 0, 0, true, true); err != nil {
		return fmt.Errorf("validate snapshot storage trie %s: %w", account.Root, err)
	}

	v.validatedStorage.Add(account.Root, struct{}{})

	return nil
}

/* START ALPHA CHANGE 3883 -- permit canonical snapshot integers without a fixed width when requested -- */
// A maximum of zero disables the byte-width limit.
// RLP type and canonical integer encoding remain checked.
func validateSnapshotInteger(value *fastrlp.Value, maximum int) error {
	if value.Type() != fastrlp.TypeBytes {
		return errors.New("snapshot integer must be RLP bytes")
	}
	raw := value.Raw()
	if maximum > 0 && len(raw) > maximum {
		return fmt.Errorf("snapshot integer exceeds %d bytes", maximum)
	}
	if len(raw) > 0 && raw[0] == 0 {
		return errors.New("snapshot integer has a leading zero")
	}

	return nil
}

/* FINISH ALPHA CHANGE 3883 -- permit canonical snapshot integers without a fixed width when requested -- */

func decodeStrictSnapshotAccount(raw []byte) (*state.Account, error) {
	p := parserPool.Get()
	defer parserPool.Put(p)

	value, err := parseSnapshotRLP(p, raw)
	if err != nil {
		return nil, fmt.Errorf("decode snapshot account: %w", err)
	}
	if value.Type() != fastrlp.TypeArray {
		return nil, errors.New("snapshot account must be an RLP array")
	}

	elements, err := value.GetElems()
	if err != nil {
		return nil, err
	}
	if len(elements) != 4 {
		return nil, errors.New("snapshot account must contain exactly 4 fields")
	}
	if err := validateSnapshotInteger(elements[0], 8); err != nil {
		return nil, fmt.Errorf("snapshot account nonce: %w", err)
	}
	/* START ALPHA CHANGE 3884 -- preserve live arbitrary-precision balances during recovery -- */
	// Live account balances are serialized as arbitrary-precision big.Int values.
	if err := validateSnapshotInteger(elements[1], 0); err != nil {
		return nil, fmt.Errorf("snapshot account balance: %w", err)
	}
	/* FINISH ALPHA CHANGE 3884 -- preserve live arbitrary-precision balances during recovery -- */

	for _, index := range []int{2, 3} {
		field, err := elements[index].GetBytes(nil)
		if err != nil || len(field) != types.HashLength {
			return nil, fmt.Errorf("snapshot account field %d must contain 32 bytes", index)
		}
	}

	var account state.Account
	if err := account.UnmarshalRlp(raw); err != nil {
		return nil, err
	}

	return &account, nil
}

func validateSnapshotStorageValue(raw []byte) error {
	p := parserPool.Get()
	defer parserPool.Put(p)

	value, err := parseSnapshotRLP(p, raw)
	if err != nil {
		return fmt.Errorf("decode snapshot storage value: %w", err)
	}

	return validateSnapshotInteger(value, types.HashLength)
}

func readVerifiedSnapshotCode(storage Storage, hash types.Hash) ([]byte, error) {
	code, ok, err := storage.Get(GetCodeKey(hash))
	if err != nil {
		return nil, fmt.Errorf("read snapshot code %s: %w", hash, err)
	}
	if !ok {
		return nil, fmt.Errorf("snapshot code %s is missing", hash)
	}
	if !bytes.Equal(crypto.Keccak256(code), hash.Bytes()) {
		return nil, fmt.Errorf("snapshot code hash mismatch for %s", hash)
	}

	return code, nil
}

/* FINISH ALPHA CHANGE 2101 -- verify complete snapshot state using persisted node commitments -- */
