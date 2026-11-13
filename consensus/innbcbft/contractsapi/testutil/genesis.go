/* START ALPHA CHANGE 3413 -- construct test allocations from current embedded contract artifacts -- */
package testutil

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

// GenesisAllocations returns independent allocations using the current
// embedded runtime bytecode. It contains no frozen genesis or storage values.
func GenesisAllocations(t testing.TB) map[types.Address]*chain.GenesisAccount {
	t.Helper()

	implementations := map[types.Address][]byte{
		/* START ALPHA CHANGE 3776 -- remove Merkle from shared genesis test allocations -- */
		contracts.BLSContractV1:          contractsapi.BLS.DeployedBytecode,
		contracts.ValidatorSetContractV1: contractsapi.ValidatorSet.DeployedBytecode,
		/* FINISH ALPHA CHANGE 3776 -- remove Merkle from shared genesis test allocations -- */
		contracts.INNBCValidatorRegistryContractV1:     contractsapi.INNBCValidatorRegistry.DeployedBytecode,
		contracts.INNBCTreasuryDAOContractV1:           contractsapi.INNBCTreasuryDAO.DeployedBytecode,
		contracts.INNBCValidatorGovernanceContractV1:   contractsapi.INNBCValidatorGovernance.DeployedBytecode,
		contracts.INNBCBlockRewardGovernanceContractV1: contractsapi.INNBCBlockRewardGovernance.DeployedBytecode,
		contracts.INNBCFeeBurnVaultContractV1:          contractsapi.INNBCFeeBurnVault.DeployedBytecode,
		contracts.INNBCCommunityRewardPoolContractV1:   contractsapi.INNBCCommunityRewardPool.DeployedBytecode,
		contracts.NativeERC20TokenContractV1:           contractsapi.INNBCNativeToken.DeployedBytecode,
		contracts.INNBCProxyAdminGovernanceContract:    contractsapi.INNBCProxyAdminGovernance.DeployedBytecode,
		contracts.INNBCProxyAdminExecutorContract:      contractsapi.INNBCProxyAdminExecutor.DeployedBytecode,
	}

	allocations := make(map[types.Address]*chain.GenesisAccount)

	for address, code := range implementations {
		require.NotEmpty(t, code, "missing current runtime bytecode for %s", address)

		allocations[address] = &chain.GenesisAccount{
			Balance: new(big.Int),
			Code:    append([]byte(nil), code...),
		}
	}

	require.NotEmpty(t, contractsapi.GenesisProxy.DeployedBytecode)

	for proxy, implementation := range contracts.GetProxyImplementationMapping() {
		require.Contains(t, allocations, implementation,
			"missing implementation allocation for proxy %s", proxy)

		allocations[proxy] = &chain.GenesisAccount{
			Balance: new(big.Int),
			Code:    append([]byte(nil), contractsapi.GenesisProxy.DeployedBytecode...),
		}
	}

	return allocations
}

/* FINISH ALPHA CHANGE 3413 -- construct test allocations from current embedded contract artifacts -- */
