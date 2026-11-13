package itrie

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/fastrlp"
)

/* START ALPHA CHANGE 1292 -- bound snapshot trie copy batch memory and recursion depth -- */
var (
	defaultTrieCopyMaxDepth        = 4096
	defaultTrieCopyMaxPendingNodes = 10000
	defaultTrieCopyMaxPendingBytes = 64 * 1024 * 1024
	defaultTrieCheckMaxDepth       = 4096
)

type trieCopyContext struct {
	storage    Storage
	newStorage Storage

	batchWriter  Batch
	pendingNodes int
	pendingBytes int

	maxDepth        int
	maxPendingNodes int
	maxPendingBytes int
}

/* FINISH ALPHA CHANGE 1292 -- bound snapshot trie copy batch memory and recursion depth -- */

/* START ALPHA CHANGE 2100 -- verify persisted trie nodes before snapshot decoding -- */
func getCustomNode(hash []byte, storage Storage) (Node, []byte, error) {
	data, err := readSnapshotTrieNode(hash, storage)
	if err != nil {
		return nil, nil, err
	}

	p := parserPool.Get()
	defer parserPool.Put(p)

	v, err := parseSnapshotRLP(p, data)
	if err != nil {
		return nil, nil, err
	}
	if err := validateSnapshotNodeShape(v, 0); err != nil {
		return nil, nil, err
	}

	n, err := decodeNode(v, storage)

	return n, data, err
}

/* FINISH ALPHA CHANGE 2100 -- verify persisted trie nodes before snapshot decoding -- */

/* START ALPHA CHANGE 1292 -- bound snapshot trie copy batch memory and recursion depth -- */
// CopyTrie copies trie nodes from storage into newStorage.
//
// The copy is flushed in bounded batches. If a later traversal or write fails,
// newStorage may contain a partial copy. Callers that need all-or-nothing
// behavior should copy into a temporary destination and discard it on failure.
/* START ALPHA CHANGE 2100 -- carry account and storage interpretation through bounded trie copying -- */
func CopyTrie(nodeHash []byte, storage Storage, newStorage Storage, agg []byte, isStorage bool) error {
	if bytes.Equal(nodeHash, types.EmptyRootHash.Bytes()) {
		return nil
	}

	ctx := &trieCopyContext{
		storage:         storage,
		newStorage:      newStorage,
		batchWriter:     newStorage.Batch(),
		maxDepth:        defaultTrieCopyMaxDepth,
		maxPendingNodes: defaultTrieCopyMaxPendingNodes,
		maxPendingBytes: defaultTrieCopyMaxPendingBytes,
	}

	if err := ctx.copyTrieHash(nodeHash, agg, 0, isStorage); err != nil {
		return err
	}

	return ctx.flush()
}

func (c *trieCopyContext) copyTrieHash(
	nodeHash []byte,
	agg []byte,
	depth int,
	isStorage bool,
) error {
	if c.maxDepth > 0 && depth > c.maxDepth {
		return fmt.Errorf("trie copy exceeded maximum depth %d", c.maxDepth)
	}

	node, data, err := getCustomNode(nodeHash, c.storage)
	if err != nil {
		return err
	}
	if err := c.put(nodeHash, data); err != nil {
		return err
	}

	return c.copyTrieNode(node, agg, depth, isStorage)
}

func (c *trieCopyContext) copyTrieNode(
	node Node,
	agg []byte,
	depth int,
	isStorage bool,
) error {
	if c.maxDepth > 0 && depth > c.maxDepth {
		return fmt.Errorf("trie copy exceeded maximum depth %d", c.maxDepth)
	}

	switch n := node.(type) {
	case nil:
		return errors.New("trie copy encountered a nil referenced node")

	case *FullNode:
		/* START ALPHA CHANGE 2189 -- read shared branch hash caches through atomic access -- */
		if hash, found := n.Hash(); found {
			return c.copyTrieHash(hash, agg, depth+1, isStorage)
		}
		/* FINISH ALPHA CHANGE 2189 -- read shared branch hash caches through atomic access -- */

		for i, child := range n.children {
			if child == nil {
				continue
			}
			if err := c.copyTrieNode(
				child,
				append(agg, uint8(i)),
				depth+1,
				isStorage,
			); err != nil {
				return err
			}
		}

		if n.value != nil {
			return c.copyTrieNode(n.value, agg, depth+1, isStorage)
		}

	case *ValueNode:
		if n.hash {
			return c.copyTrieHash(n.buf, agg, depth+1, isStorage)
		}

		// Generic storage-mode copying preserves opaque values.
		// ValidateCompleteState checks EVM storage encoding before publication.
		if isStorage {
			return nil
		}

		account, err := decodeStrictSnapshotAccount(n.buf)
		if err != nil {
			return fmt.Errorf(
				"can't parse account %s: %w",
				hex.EncodeToString(encodeCompact(agg)),
				err,
			)
		}

		codeHash := types.BytesToHash(account.CodeHash)
		if codeHash != types.EmptyCodeHash {
			code, err := readVerifiedSnapshotCode(c.storage, codeHash)
			if err != nil {
				return err
			}
			if err := c.put(GetCodeKey(codeHash), code); err != nil {
				return err
			}
		}

		if account.Root != types.EmptyRootHash {
			return c.copyTrieHash(account.Root.Bytes(), nil, depth+1, true)
		}

	case *ShortNode:
		/* START ALPHA CHANGE 2189 -- read shared short-node hash caches through atomic access -- */
		if hash, found := n.Hash(); found {
			return c.copyTrieHash(hash, agg, depth+1, isStorage)
		}
		/* FINISH ALPHA CHANGE 2189 -- read shared short-node hash caches through atomic access -- */

		return c.copyTrieNode(
			n.child,
			append(agg, n.key...),
			depth+1,
			isStorage,
		)

	default:
		return fmt.Errorf("trie copy encountered unknown node type %T", node)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2100 -- carry account and storage interpretation through bounded trie copying -- */

func (c *trieCopyContext) put(key, value []byte) error {
	c.batchWriter.Put(key, value)

	c.pendingNodes++
	c.pendingBytes += len(key) + len(value)

	if c.shouldFlush() {
		return c.flush()
	}

	return nil
}

func (c *trieCopyContext) shouldFlush() bool {
	if c.maxPendingNodes > 0 && c.pendingNodes >= c.maxPendingNodes {
		return true
	}

	return c.maxPendingBytes > 0 && c.pendingBytes >= c.maxPendingBytes
}

func (c *trieCopyContext) flush() error {
	if c.pendingNodes == 0 {
		return nil
	}

	if err := c.batchWriter.Write(); err != nil {
		return err
	}

	c.batchWriter = c.newStorage.Batch()
	c.pendingNodes = 0
	c.pendingBytes = 0

	return nil
}

/* FINISH ALPHA CHANGE 1292 -- bound snapshot trie copy batch memory and recursion depth -- */

func HashChecker(stateRoot []byte, storage Storage) (types.Hash, error) {
	node, _, err := GetNode(stateRoot, storage)
	if err != nil {
		return types.Hash{}, err
	}

	h, ok := hasherPool.Get().(*hasher)
	if !ok {
		return types.Hash{}, errors.New("can't get hasher")
	}
	defer hasherPool.Put(h)

	arena, _ := h.AcquireArena()
	defer h.ReleaseArenas(0)

	val, err := hashChecker(node, h, arena, 0, storage)
	if err != nil {
		return types.Hash{}, err
	}

	if val == nil {
		return emptyStateHash, nil
	}

	return types.BytesToHash(val.Raw()), nil
}

func hashChecker(node Node, h *hasher, a *fastrlp.Arena, d int, storage Storage) (*fastrlp.Value, error) {
	if defaultTrieCheckMaxDepth > 0 && d > defaultTrieCheckMaxDepth {
		return nil, fmt.Errorf("trie hash check exceeded maximum depth %d", defaultTrieCheckMaxDepth)
	}

	var (
		val *fastrlp.Value
		aa  *fastrlp.Arena
		idx int
	)

	switch n := node.(type) {
	case nil:
		return nil, nil
	case *ValueNode:
		if n.hash {
			nd, _, err := GetNode(n.buf, storage)
			if err != nil {
				return nil, err
			}

			return hashChecker(nd, h, a, d+1, storage)
		}

		return a.NewCopyBytes(n.buf), nil

	case *ShortNode:
		child, err := hashChecker(n.child, h, a, d+1, storage)
		if err != nil {
			return nil, err
		}

		val = a.NewArray()
		val.Set(a.NewBytes(encodeCompact(n.key)))
		val.Set(child)

	case *FullNode:
		val = a.NewArray()

		aa, idx = h.AcquireArena()

		for _, i := range n.children {
			if i == nil {
				val.Set(a.NewNull())
			} else {
				v, err := hashChecker(i, h, aa, d+1, storage)
				if err != nil {
					return nil, err
				}
				val.Set(v)
			}
		}

		// Add the value
		if n.value == nil {
			val.Set(a.NewNull())
		} else {
			v, err := hashChecker(n.value, h, a, d+1, storage)
			if err != nil {
				return nil, err
			}
			val.Set(v)
		}

	default:
		return nil, fmt.Errorf("unknown node type %T", node)
	}

	if val.Len() < 32 {
		return val, nil
	}

	// marshal RLP value
	h.buf = val.MarshalTo(h.buf[:0])

	if aa != nil {
		h.ReleaseArenas(idx)
	}

	/* START ALPHA CHANGE 2139 -- propagate the shared hash helper error in offline checking -- */
	tmp, err := h.Hash(h.buf)
	if err != nil {
		return nil, err
	}
	hh := node.SetHash(tmp)

	return a.NewCopyBytes(hh), nil
	/* FINISH ALPHA CHANGE 2139 -- propagate the shared hash helper error in offline checking -- */
}

func NewTrieWithRoot(root Node) *Trie {
	return &Trie{
		root: root,
	}
}
