package types

import (
	"github.com/umbracle/fastrlp"
)

type RLPStoreMarshaler interface {
	MarshalStoreRLPTo(dst []byte) []byte
}

/* START ALPHA CHANGE 1481 -- encode headers using a lossless compact storage representation -- */
const (
	headerStoreFlagEmptyUncles uint64 = 1 << iota
	headerStoreFlagEmptyTransactions
	headerStoreFlagEmptyReceipts
	headerStoreFlagZeroBloom
	headerStoreFlagDifficultyOne
	headerStoreFlagZeroGasUsed
	headerStoreFlagINNBCBFTMixDigest
	headerStoreFlagZeroNonce
	headerStoreFlagZeroMixHash
)

const headerStoreKnownFlags = headerStoreFlagEmptyUncles |
	headerStoreFlagEmptyTransactions |
	headerStoreFlagEmptyReceipts |
	headerStoreFlagZeroBloom |
	headerStoreFlagDifficultyOne |
	headerStoreFlagZeroGasUsed |
	headerStoreFlagINNBCBFTMixDigest |
	headerStoreFlagZeroNonce |
	headerStoreFlagZeroMixHash

// MarshalStoreRLPTo encodes a header for internal database storage.
//
// Canonical Header.MarshalRLPTo remains unchanged and continues to be used for
// hashing, networking, consensus, snapshots, archives, and RPC behavior.
func (h *Header) MarshalStoreRLPTo(dst []byte) []byte {
	return MarshalRLPTo(h.marshalStoreRLPWith, dst)
}

func (h *Header) marshalStoreRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	var flags uint64

	if h.Sha3Uncles == EmptyUncleHash {
		flags |= headerStoreFlagEmptyUncles
	}

	if h.TxRoot == EmptyRootHash {
		flags |= headerStoreFlagEmptyTransactions
	}

	if h.ReceiptsRoot == EmptyRootHash {
		flags |= headerStoreFlagEmptyReceipts
	}

	if h.LogsBloom == (Bloom{}) {
		flags |= headerStoreFlagZeroBloom
	}

	if h.Difficulty == 1 {
		flags |= headerStoreFlagDifficultyOne
	}

	if h.GasUsed == 0 {
		flags |= headerStoreFlagZeroGasUsed
	}

	switch h.MixHash {
	case INNBCBFTMixDigest:
		flags |= headerStoreFlagINNBCBFTMixDigest
	case ZeroHash:
		flags |= headerStoreFlagZeroMixHash
	}

	if h.Nonce == ZeroNonce {
		flags |= headerStoreFlagZeroNonce
	}

	stored := arena.NewArray()

	// Fields always required by the compact storage representation.
	stored.Set(arena.NewUint(flags))
	stored.Set(arena.NewCopyBytes(h.ParentHash.Bytes()))
	stored.Set(arena.NewCopyBytes(h.Miner))
	stored.Set(arena.NewCopyBytes(h.StateRoot.Bytes()))
	stored.Set(arena.NewUint(h.Number))
	stored.Set(arena.NewUint(h.GasLimit))
	stored.Set(arena.NewUint(h.Timestamp))
	stored.Set(arena.NewCopyBytes(h.ExtraData))
	stored.Set(arena.NewUint(h.BaseFee))

	// Conditionally stored fields follow one fixed order. Their flags indicate
	// whether their deterministic values were omitted entirely.
	if flags&headerStoreFlagEmptyUncles == 0 {
		stored.Set(arena.NewCopyBytes(h.Sha3Uncles.Bytes()))
	}

	if flags&headerStoreFlagEmptyTransactions == 0 {
		stored.Set(arena.NewCopyBytes(h.TxRoot.Bytes()))
	}

	if flags&headerStoreFlagEmptyReceipts == 0 {
		stored.Set(arena.NewCopyBytes(h.ReceiptsRoot.Bytes()))
	}

	if flags&headerStoreFlagZeroBloom == 0 {
		stored.Set(arena.NewCopyBytes(h.LogsBloom[:]))
	}

	if flags&headerStoreFlagDifficultyOne == 0 {
		stored.Set(arena.NewUint(h.Difficulty))
	}

	if flags&headerStoreFlagZeroGasUsed == 0 {
		stored.Set(arena.NewUint(h.GasUsed))
	}

	if flags&(headerStoreFlagINNBCBFTMixDigest|headerStoreFlagZeroMixHash) == 0 {
		stored.Set(arena.NewCopyBytes(h.MixHash.Bytes()))
	}

	if flags&headerStoreFlagZeroNonce == 0 {
		stored.Set(arena.NewCopyBytes(h.Nonce[:]))
	}

	return stored
}

/* FINISH ALPHA CHANGE 1481 -- encode headers using a lossless compact storage representation -- */

func (b *Body) MarshalRLPTo(dst []byte) []byte {
	return MarshalRLPTo(b.marshalRLPWith, dst)
}

func (b *Body) marshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()
	if len(b.Transactions) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		v0 := ar.NewArray()
		for _, tx := range b.Transactions {
			v0.Set(tx.marshalStoreRLPWith(ar))
		}
		vv.Set(v0)
	}

	if len(b.Uncles) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		v1 := ar.NewArray()
		for _, uncle := range b.Uncles {
			v1.Set(uncle.MarshalRLPWith(ar))
		}
		vv.Set(v1)
	}

	return vv
}

func (t *Transaction) MarshalStoreRLPTo(dst []byte) []byte {
	return MarshalRLPTo(t.marshalStoreRLPWith, dst)
}

func (t *Transaction) marshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()

	if t.Type != LegacyTx {
		vv.Set(a.NewBytes([]byte{byte(t.Type)}))
	}

	// consensus part
	vv.Set(t.MarshalRLPWith(a))
	// context part
	vv.Set(a.NewBytes(t.From.Bytes()))

	return vv
}

func (r Receipts) MarshalStoreRLPTo(dst []byte) []byte {
	return MarshalRLPTo(r.marshalStoreRLPWith, dst)
}

func (r *Receipts) marshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()
	for _, rr := range *r {
		vv.Set(rr.marshalStoreRLPWith(a))
	}

	return vv
}

func (r *Receipt) MarshalStoreRLPTo(dst []byte) []byte {
	return MarshalRLPTo(r.marshalStoreRLPWith, dst)
}

func (r *Receipt) marshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	// use the hash part
	vv := a.NewArray()

	if !r.IsLegacyTx() {
		vv.Set(a.NewBytes([]byte{byte(r.TransactionType)}))
	}

	vv.Set(r.MarshalRLPWith(a))

	if r.ContractAddress == nil {
		vv.Set(a.NewNull())
	} else {
		vv.Set(a.NewBytes(r.ContractAddress.Bytes()))
	}

	// gas used
	vv.Set(a.NewUint(r.GasUsed))

	// TxHash
	vv.Set(a.NewBytes(r.TxHash.Bytes()))

	return vv
}
