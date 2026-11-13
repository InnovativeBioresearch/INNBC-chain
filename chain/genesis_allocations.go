/* START ALPHA CHANGE 3100 -- validate genesis allocation accounts without changing omitted balances -- */
/* START ALPHA CHANGE 3121 -- reject initial native-token supply exceeding its uint256 representation -- */
package chain

import (
	"fmt"
	"math/big"

	"github.com/innovativebioresearch/innbc-chain/types"
)

// ValidateGenesisAllocations checks allocation input without modifying it.
// An absent balance means no balance allocation, not an invalid account.
// The initial native-token supply excludes the zero address.
func ValidateGenesisAllocations(alloc map[types.Address]*GenesisAccount) error {
	initialTotalSupply := new(big.Int)

	for address, account := range alloc {
		if account == nil {
			return fmt.Errorf("genesis allocation %s: account must not be null", address)
		}

		if account.Balance == nil {
			continue
		}

		if account.Balance.Sign() < 0 {
			return fmt.Errorf("genesis allocation %s: balance must be nonnegative", address)
		}

		if account.Balance.BitLen() > 256 {
			return fmt.Errorf("genesis allocation %s: balance exceeds unsigned 256-bit range", address)
		}

		if address != types.ZeroAddress {
			initialTotalSupply.Add(initialTotalSupply, account.Balance)
			if initialTotalSupply.BitLen() > 256 {
				return fmt.Errorf("genesis initial native-token supply exceeds unsigned 256-bit range")
			}
		}
	}

	return nil
}

// ValidateGenesisAllocations protects callers using a complete chain config.
func (c *Chain) ValidateGenesisAllocations() error {
	if c == nil {
		return fmt.Errorf("chain config is empty")
	}

	if c.Genesis == nil {
		return fmt.Errorf("chain genesis config is missing")
	}

	return ValidateGenesisAllocations(c.Genesis.Alloc)
}

/* FINISH ALPHA CHANGE 3121 -- reject initial native-token supply exceeding its uint256 representation -- */
/* FINISH ALPHA CHANGE 3100 -- validate genesis allocation accounts without changing omitted balances -- */
