/* START ALPHA CHANGE 2924 -- share signed dynamic-fee field validation -- */
package types

import (
	"errors"
	"fmt"
	"math/big"
)

var (
	ErrInvalidTxChainID = errors.New("invalid dynamic fee transaction chain ID")

	ErrInvalidTxAccessList = errors.New("invalid transaction access list")

	ErrUnsupportedTxAccessList = errors.New(
		"nonempty transaction access lists are not supported",
	)
)

// ValidateDynamicFeeChainID validates the submitted ID without modifying it.
// Legacy and state transactions retain their existing validation paths.
func ValidateDynamicFeeChainID(tx *Transaction, expected *big.Int) error {
	if tx.Type != DynamicFeeTx {
		return nil
	}

	if expected == nil || expected.Sign() < 0 {
		return fmt.Errorf(
			"%w: configured chain ID is unavailable or negative",
			ErrInvalidTxChainID,
		)
	}

	if tx.ChainID == nil || tx.ChainID.Cmp(expected) != 0 {
		return fmt.Errorf(
			"%w: got %v, expected %s",
			ErrInvalidTxChainID,
			tx.ChainID,
			expected.String(),
		)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2924 -- share signed dynamic-fee field validation -- */
