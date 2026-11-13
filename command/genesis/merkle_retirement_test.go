/* START ALPHA CHANGE 3777 -- test Merkle retirement from production and shared genesis allocations -- */
package genesis

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi/testutil"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestMerkleRetirement_GenesisAllocations(t *testing.T) {
	production, err := (&genesisParams{}).deployContracts()
	require.NoError(t, err)

	proxies := contracts.GetProxyImplementationMapping()
	require.Len(t, proxies, 9)

	retiredAddresses := []types.Address{
		types.StringToAddress("0x103"),
		types.StringToAddress("0x1031"),
	}

	for _, retired := range retiredAddresses {
		require.NotContains(t, proxies, retired)

		for _, implementation := range proxies {
			require.NotEqual(t, retired, implementation)
		}
	}

	cases := []struct {
		name        string
		allocations map[types.Address]*chain.GenesisAccount
	}{
		{
			name:        "production",
			allocations: production,
		},
		{
			name:        "shared_test_fixture",
			allocations: testutil.GenesisAllocations(t),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, retired := range retiredAddresses {
				require.NotContains(t, tc.allocations, retired)
			}

			for proxy, implementation := range proxies {
				require.Contains(t, tc.allocations, proxy)
				require.NotNil(t, tc.allocations[proxy])
				require.NotEmpty(t, tc.allocations[proxy].Code)

				require.Contains(t, tc.allocations, implementation)
				require.NotNil(t, tc.allocations[implementation])
				require.NotEmpty(t, tc.allocations[implementation].Code)
			}

			for _, controller := range []types.Address{
				contracts.INNBCProxyAdminGovernanceContract,
				contracts.INNBCProxyAdminExecutorContract,
			} {
				require.Contains(t, tc.allocations, controller)
				require.NotNil(t, tc.allocations[controller])
				require.NotEmpty(t, tc.allocations[controller].Code)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3777 -- test Merkle retirement from production and shared genesis allocations -- */
