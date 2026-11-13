package innbcbft

import (
	"errors"
	"math/big"
)

/* START ALPHA CHANGE 2637 -- centralize the fixed genesis minimum stake rule --

The genesis configuration must start the registry at exactly one wei.
Governance may subsequently change the contract's live minimum.
This validation must not be applied to that mutable contract value.

FINISH ALPHA CHANGE 2637 -- centralize the fixed genesis minimum stake rule -- */

var ErrInvalidGenesisMinimumStake = errors.New(
	"validator registry genesis minimum stake must be exactly 1 wei",
)

/* START ALPHA CHANGE 2637 -- validate fixed genesis stake without mutating configuration -- */

func ValidateGenesisMinimumStake(minimumStake *big.Int) error {
	if minimumStake == nil || minimumStake.Cmp(big.NewInt(1)) != 0 {
		return ErrInvalidGenesisMinimumStake
	}

	return nil
}

func (c *ValidatorRegistryConfig) validateGenesisMinimumStake() error {
	if c == nil {
		return errors.New("missing INNBC validator registry config")
	}

	return ValidateGenesisMinimumStake(c.MinimumStake)
}

/* FINISH ALPHA CHANGE 2637 -- validate fixed genesis stake without mutating configuration -- */
