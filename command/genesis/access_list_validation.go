/* START ALPHA CHANGE 3178 -- validate every supplied genesis access-list address before conversion -- */
package genesis

import (
	"fmt"

	"github.com/innovativebioresearch/innbc-chain/types"
)

// validateAccessListAddresses validates supplied entries without changing
// list activation, ordering, duplicates or zero-address policy.
func (p *genesisParams) validateAccessListAddresses() error {
	lists := []struct {
		flag      string
		addresses []string
	}{
		{contractDeployerAllowListAdminFlag, p.contractDeployerAllowListAdmin},
		{contractDeployerAllowListEnabledFlag, p.contractDeployerAllowListEnabled},
		{contractDeployerBlockListAdminFlag, p.contractDeployerBlockListAdmin},
		{contractDeployerBlockListEnabledFlag, p.contractDeployerBlockListEnabled},
		{transactionsAllowListAdminFlag, p.transactionsAllowListAdmin},
		{transactionsAllowListEnabledFlag, p.transactionsAllowListEnabled},
		{transactionsBlockListAdminFlag, p.transactionsBlockListAdmin},
		{transactionsBlockListEnabledFlag, p.transactionsBlockListEnabled},
	}

	for _, list := range lists {
		for _, address := range list.addresses {
			if err := types.IsValidAddress(address); err != nil {
				return fmt.Errorf(
					"invalid --%s address %q: %w",
					list.flag, address, err,
				)
			}
		}
	}

	return nil
}

/* FINISH ALPHA CHANGE 3178 -- validate every supplied genesis access-list address before conversion -- */
