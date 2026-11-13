package types

import (
	"errors"
	"fmt"

	"github.com/umbracle/fastrlp"
)

type RLPStoreUnmarshaler interface {
	UnmarshalStoreRLP(input []byte) error
}

/* START ALPHA CHANGE 1482 -- reconstruct canonical headers from compact storage -- */
// UnmarshalStoreRLP decodes the internal compact header representation and
// reconstructs the complete canonical header.
func (h *Header) UnmarshalStoreRLP(input []byte) error {
	return UnmarshalRlp(h.unmarshalStoreRLPFrom, input)
}

func (h *Header) unmarshalStoreRLPFrom(_ *fastrlp.Parser, value *fastrlp.Value) error {
	elems, err := value.GetElems()
	if err != nil {
		return err
	}

	const requiredStoredHeaderElements = 9

	if len(elems) < requiredStoredHeaderElements {
		return fmt.Errorf(
			"incorrect number of elements to decode stored header, expected at least %d but found %d",
			requiredStoredHeaderElements,
			len(elems),
		)
	}

	flags, err := elems[0].GetUint64()
	if err != nil {
		return fmt.Errorf("failed to decode stored header flags: %w", err)
	}

	if unknownFlags := flags & ^headerStoreKnownFlags; unknownFlags != 0 {
		return fmt.Errorf("stored header contains unknown flags 0x%x", unknownFlags)
	}

	if flags&headerStoreFlagINNBCBFTMixDigest != 0 &&
		flags&headerStoreFlagZeroMixHash != 0 {
		return fmt.Errorf("stored header contains conflicting mix hash flags")
	}

	*h = Header{}

	if err = elems[1].GetHash(h.ParentHash[:]); err != nil {
		return fmt.Errorf("failed to decode stored header parent hash: %w", err)
	}

	if h.Miner, err = elems[2].GetBytes(h.Miner[:0]); err != nil {
		return fmt.Errorf("failed to decode stored header miner: %w", err)
	}

	if err = elems[3].GetHash(h.StateRoot[:]); err != nil {
		return fmt.Errorf("failed to decode stored header state root: %w", err)
	}

	if h.Number, err = elems[4].GetUint64(); err != nil {
		return fmt.Errorf("failed to decode stored header number: %w", err)
	}

	if h.GasLimit, err = elems[5].GetUint64(); err != nil {
		return fmt.Errorf("failed to decode stored header gas limit: %w", err)
	}

	if h.Timestamp, err = elems[6].GetUint64(); err != nil {
		return fmt.Errorf("failed to decode stored header timestamp: %w", err)
	}

	if h.ExtraData, err = elems[7].GetBytes(h.ExtraData[:0]); err != nil {
		return fmt.Errorf("failed to decode stored header extra data: %w", err)
	}

	if h.BaseFee, err = elems[8].GetUint64(); err != nil {
		return fmt.Errorf("failed to decode stored header base fee: %w", err)
	}

	nextIndex := requiredStoredHeaderElements

	nextElement := func(field string) (*fastrlp.Value, error) {
		if nextIndex >= len(elems) {
			return nil, fmt.Errorf("stored header is missing %s", field)
		}

		elem := elems[nextIndex]
		nextIndex++

		return elem, nil
	}

	decodeHash := func(flag uint64, implied Hash, destination []byte, field string) error {
		if flags&flag != 0 {
			copy(destination, implied[:])

			return nil
		}

		elem, nextErr := nextElement(field)
		if nextErr != nil {
			return nextErr
		}

		if nextErr = elem.GetHash(destination); nextErr != nil {
			return fmt.Errorf("failed to decode stored header %s: %w", field, nextErr)
		}

		return nil
	}

	if err = decodeHash(
		headerStoreFlagEmptyUncles,
		EmptyUncleHash,
		h.Sha3Uncles[:],
		"uncle hash",
	); err != nil {
		return err
	}

	if err = decodeHash(
		headerStoreFlagEmptyTransactions,
		EmptyRootHash,
		h.TxRoot[:],
		"transaction root",
	); err != nil {
		return err
	}

	if err = decodeHash(
		headerStoreFlagEmptyReceipts,
		EmptyRootHash,
		h.ReceiptsRoot[:],
		"receipts root",
	); err != nil {
		return err
	}

	if flags&headerStoreFlagZeroBloom != 0 {
		h.LogsBloom = Bloom{}
	} else {
		elem, nextErr := nextElement("logs bloom")
		if nextErr != nil {
			return nextErr
		}

		if _, nextErr = elem.GetBytes(h.LogsBloom[:0], len(h.LogsBloom)); nextErr != nil {
			return fmt.Errorf("failed to decode stored header logs bloom: %w", nextErr)
		}
	}

	if flags&headerStoreFlagDifficultyOne != 0 {
		h.Difficulty = 1
	} else {
		elem, nextErr := nextElement("difficulty")
		if nextErr != nil {
			return nextErr
		}

		if h.Difficulty, nextErr = elem.GetUint64(); nextErr != nil {
			return fmt.Errorf("failed to decode stored header difficulty: %w", nextErr)
		}
	}

	if flags&headerStoreFlagZeroGasUsed != 0 {
		h.GasUsed = 0
	} else {
		elem, nextErr := nextElement("gas used")
		if nextErr != nil {
			return nextErr
		}

		if h.GasUsed, nextErr = elem.GetUint64(); nextErr != nil {
			return fmt.Errorf("failed to decode stored header gas used: %w", nextErr)
		}
	}

	switch {
	case flags&headerStoreFlagINNBCBFTMixDigest != 0:
		h.MixHash = INNBCBFTMixDigest
	case flags&headerStoreFlagZeroMixHash != 0:
		h.MixHash = ZeroHash
	default:
		elem, nextErr := nextElement("mix hash")
		if nextErr != nil {
			return nextErr
		}

		if nextErr = elem.GetHash(h.MixHash[:]); nextErr != nil {
			return fmt.Errorf("failed to decode stored header mix hash: %w", nextErr)
		}
	}

	if flags&headerStoreFlagZeroNonce != 0 {
		h.Nonce = ZeroNonce
	} else {
		elem, nextErr := nextElement("nonce")
		if nextErr != nil {
			return nextErr
		}

		nonce, nonceErr := elem.GetUint64()
		if nonceErr != nil {
			return fmt.Errorf("failed to decode stored header nonce: %w", nonceErr)
		}

		h.SetNonce(nonce)
	}

	if nextIndex != len(elems) {
		return fmt.Errorf(
			"stored header contains %d unexpected trailing elements",
			len(elems)-nextIndex,
		)
	}

	h.ComputeHash()

	return nil
}

/* FINISH ALPHA CHANGE 1482 -- reconstruct canonical headers from compact storage -- */

// UnmarshalRLP unmarshals body from byte slice.
// Caution: Hash for each tx must be computed manually after!
func (b *Body) UnmarshalRLP(input []byte) error {
	return UnmarshalRlp(b.unmarshalRLPFrom, input)
}

func (b *Body) unmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	tuple, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(tuple) < 2 {
		return fmt.Errorf("incorrect number of elements to decode header, expected 2 but found %d", len(tuple))
	}

	// transactions
	if err = unmarshalRLPFrom(p, tuple[0], func(txType TxType, p *fastrlp.Parser, v *fastrlp.Value) error {
		bTxn := &Transaction{
			Type: txType,
		}

		if err = bTxn.unmarshalStoreRLPFrom(p, v); err != nil {
			return err
		}

		b.Transactions = append(b.Transactions, bTxn)

		return nil
	}); err != nil {
		return err
	}

	// uncles
	uncles, err := tuple[1].GetElems()
	if err != nil {
		return err
	}

	for _, uncle := range uncles {
		bUncle := &Header{}
		if err = bUncle.unmarshalRLPFrom(p, uncle); err != nil {
			return err
		}

		b.Uncles = append(b.Uncles, bUncle)
	}

	return nil
}

// UnmarshalStoreRLP unmarshals transaction from byte slice. Hash must be computed manually after!
func (t *Transaction) UnmarshalStoreRLP(input []byte) error {
	t.Type = LegacyTx

	if len(input) > 0 && input[0] <= RLPSingleByteUpperLimit {
		var err error
		if t.Type, err = txTypeFromByte(input[0]); err != nil {
			return err
		}
	}

	return UnmarshalRlp(t.unmarshalStoreRLPFrom, input)
}

func (t *Transaction) unmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	// come TransactionType first if exist
	if len(elems) != 2 && len(elems) != 3 {
		return fmt.Errorf("incorrect number of elements, expected 2 or 3 but found %d", len(elems))
	}

	if len(elems) == 3 {
		if err = t.Type.unmarshalRLPFrom(p, elems[0]); err != nil {
			return err
		}

		elems = elems[1:]
	}

	// consensus part
	if err = t.unmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// context part
	if err = elems[1].GetAddr(t.From[:]); err != nil {
		return err
	}

	return nil
}

func (r *Receipts) UnmarshalStoreRLP(input []byte) error {
	return UnmarshalRlp(r.unmarshalStoreRLPFrom, input)
}

func (r *Receipts) unmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	return unmarshalRLPFrom(p, v, func(txType TxType, p *fastrlp.Parser, v *fastrlp.Value) error {
		obj := &Receipt{
			TransactionType: txType,
		}

		if err := obj.unmarshalStoreRLPFrom(p, v); err != nil {
			return err
		}

		*r = append(*r, obj)

		return nil
	})
}

func (r *Receipt) UnmarshalStoreRLP(input []byte) error {
	r.TransactionType = LegacyTx

	if len(input) > 0 && input[0] <= RLPSingleByteUpperLimit {
		var err error
		if r.TransactionType, err = txTypeFromByte(input[0]); err != nil {
			return err
		}
	}

	return UnmarshalRlp(r.unmarshalStoreRLPFrom, input)
}

func (r *Receipt) unmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 4 {
		return errors.New("expected at least 4 elements")
	}

	// come TransactionType first if exist
	if len(elems) == 5 {
		if err = r.TransactionType.unmarshalRLPFrom(p, elems[0]); err != nil {
			return err
		}

		elems = elems[1:]
	}

	if err = r.unmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// contract address
	vv, err := elems[1].Bytes()
	if err != nil {
		return err
	}

	if len(vv) == 20 {
		// address
		r.SetContractAddress(BytesToAddress(vv))
	}

	// gas used
	if r.GasUsed, err = elems[2].GetUint64(); err != nil {
		return err
	}

	// tx hash
	// backwards compatibility, old receipts did not marshal a TxHash
	if len(elems) == 4 {
		vv, err = elems[3].Bytes()
		if err != nil {
			return err
		}

		r.TxHash = BytesToHash(vv)
	}

	return nil
}
