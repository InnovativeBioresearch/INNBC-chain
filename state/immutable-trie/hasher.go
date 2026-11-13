package itrie

import (
	"errors"
	"fmt"
	"hash"
	"sync"

	/* START ALPHA CHANGE 2138 -- classify root-calculation failures -- */
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	/* FINISH ALPHA CHANGE 2138 -- classify root-calculation failures -- */
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"
)

var arenaPool fastrlp.ArenaPool

var (
	emptyRoot = types.StringToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421").Bytes()
)

var hasherPool = sync.Pool{
	New: func() interface{} {
		impl, ok := sha3.NewLegacyKeccak256().(hashImpl)
		if !ok {
			return nil
		}

		return &hasher{
			hash: impl,
		}
	},
}

type hashImpl interface {
	hash.Hash
	Read([]byte) (int, error)
}

type hasher struct {
	arena []*fastrlp.Arena
	buf   []byte
	hash  hashImpl
	tmp   [32]byte
}

func (h *hasher) Reset() {
	h.buf = h.buf[:0]
	h.hash.Reset()
	h.tmp = [32]byte{}
}

func (h *hasher) ReleaseArenas(idx int) {
	for i := idx; i < len(h.arena); i++ {
		arenaPool.Put(h.arena[i])
	}

	h.arena = h.arena[:idx]
}

func (h *hasher) ReleaseArena(a *fastrlp.Arena) {
	a.Reset()
	arenaPool.Put(a)
}

func (h *hasher) AcquireArena() (*fastrlp.Arena, int) {
	v := arenaPool.Get()
	idx := len(h.arena)
	h.arena = append(h.arena, v)

	return v, idx
}

/* START ALPHA CHANGE 2138 -- propagate hash-engine failures instead of panicking -- */
func (h *hasher) Hash(data []byte) ([]byte, error) {
	h.hash.Reset()

	if _, err := h.hash.Write(data); err != nil {
		return nil, err
	}

	n, err := h.hash.Read(h.tmp[:])
	if err != nil {
		return nil, err
	}
	if n != len(h.tmp) {
		return nil, fmt.Errorf(
			"incorrect hash length: got %d, want %d",
			n,
			len(h.tmp),
		)
	}

	return h.tmp[:], nil
}

/* FINISH ALPHA CHANGE 2138 -- propagate hash-engine failures instead of panicking -- */

func (t *Txn) Hash() ([]byte, error) {
	/* START ALPHA CHANGE 2138 -- retain root failures for all later uses of the trie transaction -- */
	if t.err != nil {
		return nil, t.err
	}
	if t.root == nil {
		return emptyRoot, nil
	}

	h, ok := hasherPool.Get().(*hasher)
	if !ok || h == nil {
		t.setError(errors.New("invalid hasher pool value"))

		return nil, t.err
	}

	defer func() {
		h.ReleaseArenas(0)
		hasherPool.Put(h)
	}()

	var root []byte

	arena, _ := h.AcquireArena()
	val := t.hash(t.root, h, arena, 0)
	if t.err != nil {
		return nil, t.err
	}
	/* FINISH ALPHA CHANGE 2138 -- retain root failures for all later uses of the trie transaction -- */

	// REDO
	if val.Type() == fastrlp.TypeBytes {
		if val.Len() != 32 {
			h.hash.Reset()
			h.hash.Write(val.Raw())

			root = h.hash.Sum(nil)

			if t.batch != nil {
				t.batch.Put(root, val.Raw())
			}
		} else {
			root = make([]byte, 32)
			copy(root, val.Raw())
		}
	} else {
		tmp := val.MarshalTo(nil)
		h.hash.Reset()
		h.hash.Write(tmp)

		root = h.hash.Sum(nil)

		if t.batch != nil {
			t.batch.Put(root, tmp)
		}
	}

	return root, nil
}

func (t *Txn) hash(node Node, h *hasher, a *fastrlp.Arena, d int) *fastrlp.Value {
	/* START ALPHA CHANGE 2138 -- stop recursive hashing after a retained failure -- */
	if t.err != nil {
		return nil
	}
	if node == nil {
		t.setError(fmt.Errorf(
			"%w: nil node in required hashing path",
			statepkg.ErrMalformedTrieNode,
		))

		return nil
	}
	/* FINISH ALPHA CHANGE 2138 -- stop recursive hashing after a retained failure -- */

	var val *fastrlp.Value

	var aa *fastrlp.Arena

	var idx int

	if h, ok := node.Hash(); ok {
		return a.NewCopyBytes(h)
	}

	switch n := node.(type) {
	case *ValueNode:
		return a.NewCopyBytes(n.buf)

	case *ShortNode:
		child := t.hash(n.child, h, a, d+1)
		/* START ALPHA CHANGE 2138 -- propagate a failed short-node child hash -- */
		if t.err != nil {
			return nil
		}
		/* FINISH ALPHA CHANGE 2138 -- propagate a failed short-node child hash -- */

		val = a.NewArray()
		val.Set(a.NewBytes(encodeCompact(n.key)))
		val.Set(child)

	case *FullNode:
		val = a.NewArray()

		aa, idx = h.AcquireArena()

		/* START ALPHA CHANGE 2138 -- propagate failed full-node hashing paths -- */
		for _, child := range n.children {
			if child == nil {
				val.Set(a.NewNull())

				continue
			}

			childValue := t.hash(child, h, aa, d+1)
			if t.err != nil {
				return nil
			}

			val.Set(childValue)
		}

		if n.value == nil {
			val.Set(a.NewNull())
		} else {
			value := t.hash(n.value, h, a, d+1)
			if t.err != nil {
				return nil
			}

			val.Set(value)
		}

	default:
		t.setError(fmt.Errorf(
			"%w: unknown hashing node type %T",
			statepkg.ErrMalformedTrieNode,
			n,
		))

		return nil
	}
	/* FINISH ALPHA CHANGE 2138 -- propagate failed full-node hashing paths -- */

	if val.Len() < 32 {
		return val
	}

	// marshal RLP value
	h.buf = val.MarshalTo(h.buf[:0])

	if aa != nil {
		h.ReleaseArenas(idx)
	}

	/* START ALPHA CHANGE 2138 -- retain hash-engine failures before caching a node hash -- */
	tmp, err := h.Hash(h.buf)
	if err != nil {
		t.setError(err)

		return nil
	}
	hh := node.SetHash(tmp)
	/* FINISH ALPHA CHANGE 2138 -- retain hash-engine failures before caching a node hash -- */

	// Write data
	if t.batch != nil {
		t.batch.Put(tmp, h.buf)
	}

	return a.NewCopyBytes(hh)
}
