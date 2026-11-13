/* START ALPHA CHANGE 3004 -- validate runtime RLP shape before recursive parsing -- */
package types

import "fmt"

// These checks describe supported encodings, not consensus validity or
// transaction-pool size policy. All slices reference the original input.
type runtimeRLPItem struct {
	list    bool
	encoded []byte
	payload []byte
	rest    []byte
}

func runtimeRLPNext(input []byte) (runtimeRLPItem, error) {
	if len(input) == 0 {
		return runtimeRLPItem{}, ErrInvalidRLPStructure
	}

	list, start, end, err := rlpStructureItem(input, 0, len(input))
	if err != nil {
		return runtimeRLPItem{}, err
	}

	return runtimeRLPItem{
		list:    list,
		encoded: input[:end],
		payload: input[start:end],
		rest:    input[end:],
	}, nil
}

func runtimeRLPListPayload(input []byte) ([]byte, error) {
	item, err := runtimeRLPNext(input)
	if err != nil {
		return nil, err
	}
	if !item.list || len(item.rest) != 0 {
		return nil, ErrInvalidRLPStructure
	}

	return item.payload, nil
}

// Reject unsupported field shapes without descending into their lists.
// Existing access-list errors remain distinguishable.
func validateRuntimeTransactionShape(input []byte, txType TxType) error {
	payload, err := runtimeRLPListPayload(input)
	if err != nil {
		return err
	}

	var expected int
	switch txType {
	case LegacyTx:
		expected = 9
	case StateTx:
		expected = 10
	case DynamicFeeTx:
		expected = 12
	default:
		return fmt.Errorf("transaction type %d not found", txType)
	}

	count := 0
	for len(payload) != 0 {
		if count >= expected {
			return ErrInvalidRLPStructure
		}

		field, err := runtimeRLPNext(payload)
		if err != nil {
			return err
		}

		/* START ALPHA CHANGE 3022 -- reject noncanonical scalar fields before parsing -- */
		if txType == DynamicFeeTx && count == 8 {
			if !field.list {
				return ErrInvalidTxAccessList
			}
			if len(field.payload) != 0 {
				return ErrUnsupportedTxAccessList
			}
		} else {
			if field.list {
				return ErrInvalidRLPStructure
			}
			if err := validateTransactionRLPField(txType, count, field.payload); err != nil {
				return err
			}
		}
		/* FINISH ALPHA CHANGE 3022 -- reject noncanonical scalar fields before parsing -- */

		count++
		payload = field.rest
	}

	if count != expected {
		return ErrInvalidRLPStructure
	}

	return nil
}

func validateRuntimeTransactionRLP(input []byte, txType TxType) error {
	if err := validateRuntimeTransactionShape(input, txType); err != nil {
		return err
	}

	maxDepth := 1
	if txType == DynamicFeeTx {
		maxDepth = 2
	}

	return ValidateRLPStructure(input, maxDepth)
}

// Headers currently model 15 historical fields, plus optional base fee.
// Embedded extra-data bytes remain opaque here and are checked separately.
func validateRuntimeHeaderShape(input []byte) error {
	payload, err := runtimeRLPListPayload(input)
	if err != nil {
		return err
	}

	count := 0
	for len(payload) != 0 {
		if count >= 16 {
			return ErrInvalidRLPStructure
		}

		field, err := runtimeRLPNext(payload)
		if err != nil {
			return err
		}
		if field.list {
			return ErrInvalidRLPStructure
		}

		count++
		payload = field.rest
	}

	if count < 15 {
		return ErrInvalidRLPStructure
	}

	return nil
}

// Preserve the block representation's flattened type-marker/list pairs.
func validateRuntimeBlockTransactions(payload []byte) error {
	for len(payload) != 0 {
		item, err := runtimeRLPNext(payload)
		if err != nil {
			return err
		}
		payload = item.rest

		txType := LegacyTx
		if !item.list {
			if len(item.payload) != 1 {
				return ErrInvalidRLPStructure
			}

			txType, err = txTypeFromByte(item.payload[0])
			if err != nil {
				return err
			}
			if len(payload) == 0 {
				return fmt.Errorf(
					"missing payload after transaction type 0x%x",
					byte(txType),
				)
			}

			/* START ALPHA CHANGE 3023 -- reject redundant legacy markers in block transactions -- */
			if txType == LegacyTx {
				return fmt.Errorf(
					"%w: legacy block transaction has a redundant type marker",
					ErrNonCanonicalTxEncoding,
				)
			}
			/* FINISH ALPHA CHANGE 3023 -- reject redundant legacy markers in block transactions -- */

			item, err = runtimeRLPNext(payload)
			if err != nil {
				return err
			}
			payload = item.rest
		}

		if err := validateRuntimeTransactionShape(item.encoded, txType); err != nil {
			return err
		}
	}

	return nil
}

func validateRuntimeBlockRLP(input []byte) error {
	payload, err := runtimeRLPListPayload(input)
	if err != nil {
		return err
	}

	var fields [3]runtimeRLPItem
	for index := range fields {
		fields[index], err = runtimeRLPNext(payload)
		if err != nil {
			return err
		}
		payload = fields[index].rest
	}
	if len(payload) != 0 {
		return ErrInvalidRLPStructure
	}

	if err := validateRuntimeHeaderShape(fields[0].encoded); err != nil {
		return err
	}
	if !fields[1].list || !fields[2].list {
		return ErrInvalidRLPStructure
	}

	if err := validateRuntimeBlockTransactions(fields[1].payload); err != nil {
		return err
	}

	uncles := fields[2].payload
	for len(uncles) != 0 {
		uncle, err := runtimeRLPNext(uncles)
		if err != nil {
			return err
		}
		if err := validateRuntimeHeaderShape(uncle.encoded); err != nil {
			return err
		}
		uncles = uncle.rest
	}

	return ValidateRLPStructure(input, BlockRLPMaxDepth)
}

/* FINISH ALPHA CHANGE 3004 -- validate runtime RLP shape before recursive parsing -- */
