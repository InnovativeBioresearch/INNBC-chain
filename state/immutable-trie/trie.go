package itrie

import (
	"bytes"
	"fmt"

	/* START ALPHA CHANGE 2189 -- publish immutable trie cache values atomically -- */
	"sync/atomic"
	/* FINISH ALPHA CHANGE 2189 -- publish immutable trie cache values atomically -- */

	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"

	/* START ALPHA CHANGE 2133 -- retain classified trie failures -- */
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	/* FINISH ALPHA CHANGE 2133 -- retain classified trie failures -- */
	"github.com/innovativebioresearch/innbc-chain/types"
)

// Node represents a node reference
type Node interface {
	Hash() ([]byte, bool)
	SetHash(b []byte) []byte
}

/* START ALPHA CHANGE 2189 -- retain successful resolution without replacing shared child edges -- */
type resolvedTrieNode struct {
	node Node
}

// ValueNode is a leaf or a reference to a persisted trie node.
// hash and buf are immutable once the node becomes shared.
type ValueNode struct {
	hash bool
	buf  []byte

	resolved atomic.Pointer[resolvedTrieNode]
}

/* FINISH ALPHA CHANGE 2189 -- retain successful resolution without replacing shared child edges -- */

// Hash implements the node interface
func (v *ValueNode) Hash() ([]byte, bool) {
	return v.buf, v.hash
}

// SetHash implements the node interface
func (v *ValueNode) SetHash(b []byte) []byte {
	panic("We cannot set hash on value node") //nolint:gocritic
}

/* START ALPHA CHANGE 2189 -- publish immutable hash-cache bytes atomically -- */
type cachedTrieHash struct {
	bytes []byte
}

type common struct {
	hash atomic.Pointer[cachedTrieHash]
}

// Hash implements the node interface.
// Returned bytes belong to an immutable cache entry.
func (c *common) Hash() ([]byte, bool) {
	cached := c.hash.Load()
	if cached == nil {
		return nil, false
	}

	return cached.bytes, len(cached.bytes) != 0
}

// SetHash implements the node interface.
// Copy before publication so another hasher cannot overwrite shared bytes.
func (c *common) SetHash(b []byte) []byte {
	cached := &cachedTrieHash{
		bytes: append([]byte(nil), b...),
	}
	c.hash.Store(cached)

	return cached.bytes
}

/* FINISH ALPHA CHANGE 2189 -- publish immutable hash-cache bytes atomically -- */

// ShortNode is an extension or short node
type ShortNode struct {
	common
	key   []byte
	child Node
}

// FullNode is a node with several children
type FullNode struct {
	common
	epoch    uint32
	value    Node
	children [16]Node
}

func (f *FullNode) copy() *FullNode {
	nc := &FullNode{}
	nc.value = f.value
	copy(nc.children[:], f.children[:])

	return nc
}

func (f *FullNode) setEdge(idx byte, e Node) {
	if idx == 16 {
		f.value = e
	} else {
		f.children[idx] = e
	}
}

func (f *FullNode) getEdge(idx byte) Node {
	if idx == 16 {
		return f.value
	} else {
		return f.children[idx]
	}
}

type Trie struct {
	root  Node
	epoch uint32
}

func NewTrie() *Trie {
	return &Trie{}
}

/* START ALPHA CHANGE 2133 -- distinguish trie absence from lookup failure -- */
func (t *Trie) Get(k []byte, storage Storage) ([]byte, bool, error) {
	return t.Txn(storage).Lookup(k)
}

/* FINISH ALPHA CHANGE 2133 -- distinguish trie absence from lookup failure -- */

func hashit(k []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(k)

	return h.Sum(nil)
}

var (
	stateArenaPool fastrlp.ArenaPool
)

// Hash returns the root hash of the trie. It does not write to the
// database and can be used even if the trie doesn't have one.
func (t *Trie) Hash() types.Hash {
	if t.root == nil {
		return types.EmptyRootHash
	}

	hash := t.hashRoot()

	return types.BytesToHash(hash)
}

func (t *Trie) hashRoot() []byte {
	hash, _ := t.root.Hash()

	return hash
}

func (t *Trie) Txn(storage Storage) *Txn {
	return &Txn{root: t.root, epoch: t.epoch + 1, storage: storage}
}

type Putter interface {
	Put(k, v []byte)
}

/* START ALPHA CHANGE 2134 -- prevent trie commit from discarding a retained failure -- */
type Txn struct {
	root    Node
	epoch   uint32
	storage Storage
	batch   Putter
	err     error
}

func (t *Txn) setError(err error) {
	if err != nil && t.err == nil {
		t.err = statepkg.WrapLocalStateFailure("trie transaction", err)
	}
}

func (t *Txn) Error() error {
	return t.err
}

func (t *Txn) Commit() (*Trie, error) {
	if t.err != nil {
		return nil, t.err
	}

	return &Trie{epoch: t.epoch, root: t.root}, nil
}

func (t *Txn) loadNode(hash []byte, operation string) (Node, error) {
	if t.storage == nil {
		return nil, statepkg.WrapLocalStateFailure(
			operation,
			fmt.Errorf("cannot resolve trie node %x without storage", hash),
		)
	}

	node, found, err := GetNode(hash, t.storage)
	if err != nil {
		return nil, statepkg.WrapLocalStateFailure(operation, err)
	}
	if !found {
		return nil, statepkg.WrapLocalStateFailure(
			operation,
			fmt.Errorf("%w: %x", statepkg.ErrMissingTrieNode, hash),
		)
	}

	return node, nil
}

/* FINISH ALPHA CHANGE 2134 -- prevent trie commit from discarding a retained failure -- */

/* START ALPHA CHANGE 2190 -- distinguish matching empty leaves without mutating shared trie paths -- */
func (t *Txn) resolveNode(
	reference *ValueNode,
	operation string,
) (Node, error) {
	if cached := reference.resolved.Load(); cached != nil {
		return cached.node, nil
	}

	return t.loadNode(reference.buf, operation)
}

func (t *Txn) Lookup(key []byte) ([]byte, bool, error) {
	if t.err != nil {
		return nil, false, t.err
	}

	result, found, err := t.lookup(t.root, bytesToHexNibbles(key))
	if err != nil {
		t.setError(err)

		return nil, false, t.err
	}

	return result, found, nil
}

func (t *Txn) lookup(
	node Node,
	key []byte,
) ([]byte, bool, error) {
	switch n := node.(type) {
	case nil:
		return nil, false, nil

	case *ValueNode:
		if n.hash {
			child, err := t.resolveNode(n, "resolve trie lookup")
			if err != nil {
				return nil, false, err
			}

			result, found, err := t.lookup(child, key)
			if err != nil {
				return nil, false, err
			}

			// Both a found key and genuine absence represent successful
			// traversal. Failed traversal never publishes this resolution.
			if n.resolved.Load() == nil {
				n.resolved.CompareAndSwap(
					nil,
					&resolvedTrieNode{node: child},
				)
			}

			return result, found, nil
		}

		if len(key) == 0 {
			// Presence depends on reaching the matching leaf, not on
			// whether its payload is nil or empty.
			return n.buf, true, nil
		}

		return nil, false, nil

	case *ShortNode:
		pathLength := len(n.key)
		if pathLength > len(key) ||
			!bytes.Equal(key[:pathLength], n.key) {
			return nil, false, nil
		}

		return t.lookup(n.child, key[pathLength:])

	case *FullNode:
		if len(key) == 0 {
			return t.lookup(n.value, key)
		}
		if key[0] > 16 {
			return nil, false, fmt.Errorf(
				"%w: invalid lookup edge %d",
				statepkg.ErrMalformedTrieNode,
				key[0],
			)
		}

		return t.lookup(n.getEdge(key[0]), key[1:])

	default:
		return nil, false, fmt.Errorf(
			"%w: unknown lookup node type %T",
			statepkg.ErrMalformedTrieNode,
			n,
		)
	}
}

/* FINISH ALPHA CHANGE 2190 -- distinguish matching empty leaves without mutating shared trie paths -- */

func (t *Txn) writeNode(n *FullNode) *FullNode {
	if t.epoch == n.epoch {
		return n
	}

	nc := &FullNode{
		epoch: t.epoch,
		value: n.value,
	}
	copy(nc.children[:], n.children[:])

	return nc
}

/* START ALPHA CHANGE 2136 -- install insertion results only after successful mutation -- */
func (t *Txn) Insert(key, value []byte) {
	if t.err != nil {
		return
	}

	root := t.insert(t.root, bytesToHexNibbles(key), value)
	if t.err == nil && root != nil {
		t.root = root
	}
}

/* FINISH ALPHA CHANGE 2136 -- install insertion results only after successful mutation -- */

func (t *Txn) insert(node Node, search, value []byte) Node {
	switch n := node.(type) {
	case nil:
		// NOTE, this only happens with the full node
		if len(search) == 0 {
			v := &ValueNode{}
			v.buf = make([]byte, len(value))
			copy(v.buf, value)

			return v
		} else {
			return &ShortNode{
				key:   search,
				child: t.insert(nil, nil, value),
			}
		}

	case *ValueNode:
		/* START ALPHA CHANGE 2136 -- preserve insertion node-loading failures -- */
		if n.hash {
			/* START ALPHA CHANGE 2190 -- reuse successfully resolved insertion references -- */
			child, err := t.resolveNode(n, "resolve trie insertion")
			/* FINISH ALPHA CHANGE 2190 -- reuse successfully resolved insertion references -- */
			if err != nil {
				t.setError(err)

				return n
			}

			return t.insert(child, search, value)
		}
		/* FINISH ALPHA CHANGE 2136 -- preserve insertion node-loading failures -- */

		if len(search) == 0 {
			v := &ValueNode{}
			v.buf = make([]byte, len(value))
			copy(v.buf, value)

			return v
		} else {
			b := t.insert(&FullNode{epoch: t.epoch, value: n}, search, value)

			return b
		}

	case *ShortNode:
		plen := prefixLen(search, n.key)
		if plen == len(n.key) {
			// Keep this node as is and insert to child
			child := t.insert(n.child, search[plen:], value)

			return &ShortNode{key: n.key, child: child}
		} else {
			// Introduce a new branch
			b := FullNode{epoch: t.epoch}
			if len(n.key) > plen+1 {
				b.setEdge(n.key[plen], &ShortNode{key: n.key[plen+1:], child: n.child})
			} else {
				b.setEdge(n.key[plen], n.child)
			}

			child := t.insert(&b, search[plen:], value)

			if plen == 0 {
				return child
			} else {
				return &ShortNode{key: search[:plen], child: child}
			}
		}

	/* START ALPHA CHANGE 2136 -- mutate parent nodes only after their child insertion succeeds -- */
	case *FullNode:
		if len(search) == 0 {
			newValue := t.insert(n.value, nil, value)
			if t.err != nil {
				return n
			}

			b := t.writeNode(n)
			b.value = newValue

			return b
		}

		edge := search[0]
		if edge > 16 {
			t.setError(fmt.Errorf(
				"%w: invalid insertion edge %d",
				statepkg.ErrMalformedTrieNode,
				edge,
			))

			return n
		}

		newChild := t.insert(n.getEdge(edge), search[1:], value)
		if t.err != nil {
			return n
		}

		b := t.writeNode(n)
		b.setEdge(edge, newChild)

		return b

	default:
		t.setError(fmt.Errorf(
			"%w: unknown insertion node type %T",
			statepkg.ErrMalformedTrieNode,
			n,
		))

		return node
	}
	/* FINISH ALPHA CHANGE 2136 -- mutate parent nodes only after their child insertion succeeds -- */
}

/* START ALPHA CHANGE 2137 -- install deletion results only after successful mutation -- */
func (t *Txn) Delete(key []byte) {
	if t.err != nil {
		return
	}

	root, changed := t.delete(t.root, bytesToHexNibbles(key))
	if t.err == nil && changed {
		t.root = root
	}
}

/* FINISH ALPHA CHANGE 2137 -- install deletion results only after successful mutation -- */

func (t *Txn) delete(node Node, search []byte) (Node, bool) {
	switch n := node.(type) {
	case nil:
		return nil, false

	case *ShortNode:
		/* START ALPHA CHANGE 2137 -- leave shared short nodes unchanged during deletion -- */
		// Successful deletion constructs replacement path nodes below.
		// Do not invalidate the original node's cached hash.
		/* FINISH ALPHA CHANGE 2137 -- leave shared short nodes unchanged during deletion -- */

		plen := prefixLen(search, n.key)
		if plen == len(search) {
			return nil, true
		}

		if plen == 0 {
			return nil, false
		}

		child, ok := t.delete(n.child, search[plen:])
		if !ok {
			return nil, false
		}

		if child == nil {
			return nil, true
		}

		if short, ok := child.(*ShortNode); ok {
			// merge nodes
			return &ShortNode{key: concat(n.key, short.key), child: short.child}, true
		} else {
			// full node
			return &ShortNode{key: n.key, child: child}, true
		}

	case *ValueNode:
		/* START ALPHA CHANGE 2137 -- preserve deletion node-loading failures -- */
		if n.hash {
			/* START ALPHA CHANGE 2190 -- reuse successfully resolved deletion references -- */
			child, err := t.resolveNode(n, "resolve trie deletion")
			/* FINISH ALPHA CHANGE 2190 -- reuse successfully resolved deletion references -- */
			if err != nil {
				t.setError(err)

				return n, false
			}

			return t.delete(child, search)
		}
		/* FINISH ALPHA CHANGE 2137 -- preserve deletion node-loading failures -- */

		if len(search) != 0 {
			return nil, false
		}

		return nil, true

	case *FullNode:
		/* START ALPHA CHANGE 2137 -- reject invalid deletion traversal without indexing an empty path -- */
		if len(search) == 0 || search[0] > 16 {
			t.setError(fmt.Errorf(
				"%w: invalid full-node deletion path",
				statepkg.ErrMalformedTrieNode,
			))

			return n, false
		}
		/* FINISH ALPHA CHANGE 2137 -- reject invalid deletion traversal without indexing an empty path -- */

		/* START ALPHA CHANGE 2189 -- replacement branches start with an empty hash cache -- */
		n = n.copy()
		/* FINISH ALPHA CHANGE 2189 -- replacement branches start with an empty hash cache -- */

		key := search[0]
		newChild, ok := t.delete(n.getEdge(key), search[1:])

		if !ok {
			return nil, false
		}

		n.setEdge(key, newChild)

		indx := -1

		var notEmpty bool

		for edge, i := range n.children {
			if i != nil {
				if indx != -1 {
					notEmpty = true

					break
				} else {
					indx = edge
				}
			}
		}

		if indx != -1 && n.value != nil {
			// We have one children and value, set notEmpty to true
			notEmpty = true
		}

		if notEmpty {
			// The full node still has some other values
			return n, true
		}

		if indx == -1 {
			// There are no children nodes
			if n.value == nil {
				// Everything is empty, return nil
				return nil, true
			}
			// The value is the only left, return a short node with it
			return &ShortNode{key: []byte{0x10}, child: n.value}, true
		}

		// Only one value left at indx
		nc := n.children[indx]

		/* START ALPHA CHANGE 2137 -- retain failures while compacting a deleted branch -- */
		if valueNode, ok := nc.(*ValueNode); ok && valueNode.hash {
			/* START ALPHA CHANGE 2190 -- reuse successful resolution during branch compaction -- */
			child, err := t.resolveNode(
				valueNode,
				"resolve trie deletion compaction",
			)
			/* FINISH ALPHA CHANGE 2190 -- reuse successful resolution during branch compaction -- */
			if err != nil {
				t.setError(err)

				return n, false
			}

			nc = child
		}
		/* FINISH ALPHA CHANGE 2137 -- retain failures while compacting a deleted branch -- */

		obj, ok := nc.(*ShortNode)
		if !ok {
			obj := &ShortNode{}
			obj.key = []byte{byte(indx)}
			obj.child = nc

			return obj, true
		}

		ncc := &ShortNode{}
		ncc.key = concat([]byte{byte(indx)}, obj.key)
		ncc.child = obj.child

		return ncc, true
	}

	/* START ALPHA CHANGE 2137 -- return unsupported deletion-node failures through the latch -- */
	t.setError(fmt.Errorf(
		"%w: unknown deletion node type %T",
		statepkg.ErrMalformedTrieNode,
		node,
	))

	return node, false
	/* FINISH ALPHA CHANGE 2137 -- return unsupported deletion-node failures through the latch -- */
}

func prefixLen(k1, k2 []byte) int {
	max := len(k1)
	if l := len(k2); l < max {
		max = l
	}

	var i int

	for i = 0; i < max; i++ {
		if k1[i] != k2[i] {
			break
		}
	}

	return i
}

func concat(a, b []byte) []byte {
	c := make([]byte, len(a)+len(b))
	copy(c, a)
	copy(c[len(a):], b)

	return c
}
