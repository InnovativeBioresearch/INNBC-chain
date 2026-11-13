/* START ALPHA CHANGE 3021 -- validate canonical transaction field representations -- */
package types

import (
	"errors"
	"fmt"
)

var ErrNonCanonicalTxEncoding = errors.New("noncanonical transaction encoding")

// validateTransactionRLPField validates a scalar transaction field.
// Field types and counts are checked by the surrounding decoder.
// Addresses and calldata intentionally retain their leading zero bytes.
func validateTransactionRLPField(txType TxType, index int, payload []byte) error {
	toIndex, inputIndex := 3, 5

	switch txType {
	case LegacyTx, StateTx:
	case DynamicFeeTx:
		toIndex, inputIndex = 5, 7
	default:
		return fmt.Errorf("unknown transaction type: %d", txType)
	}

	switch {
	case index == toIndex:
		if len(payload) != 0 && len(payload) != AddressLength {
			return fmt.Errorf(
				"%w: recipient has %d bytes; expected zero or %d",
				ErrNonCanonicalTxEncoding,
				len(payload),
				AddressLength,
			)
		}

	case index == inputIndex:
		// Calldata is an opaque byte string.

	case txType == DynamicFeeTx && index == 8:
		// The surrounding decoder independently requires an empty list.

	case txType == StateTx && index == 9:
		if len(payload) != AddressLength {
			return fmt.Errorf(
				"%w: state sender has %d bytes; expected %d",
				ErrNonCanonicalTxEncoding,
				len(payload),
				AddressLength,
			)
		}

	default:
		// Integer zero is the empty byte string. Positive integers must
		// not contain a leading zero byte.
		if len(payload) != 0 && payload[0] == 0 {
			return fmt.Errorf(
				"%w: integer field %d has a leading zero byte",
				ErrNonCanonicalTxEncoding,
				index,
			)
		}
	}

	return nil
}

/* FINISH ALPHA CHANGE 3021 -- validate canonical transaction field representations -- */
