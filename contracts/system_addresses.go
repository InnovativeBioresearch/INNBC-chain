package contracts

import "github.com/innovativebioresearch/innbc-chain/types"

var (
	// ValidatorSetContract is an address of validator set proxy contract deployed to child chain
	ValidatorSetContract = types.StringToAddress("0x101")
	// ValidatorSetContractV1 is an address of validator set implementation contract deployed to child chain
	ValidatorSetContractV1 = types.StringToAddress("0x1011")
	// BLSContract is an address of BLS proxy contract on the child chain
	BLSContract = types.StringToAddress("0x102")
	// BLSContractV1 is an address of BLS contract on the child chain
	BLSContractV1 = types.StringToAddress("0x1021")
	/* START ALPHA CHANGE 3770 -- remove retired Merkle system addresses and proxy mapping -- */
	/* FINISH ALPHA CHANGE 3770 -- remove retired Merkle system addresses and proxy mapping -- */
	/* START ALPHA CHANGE 610 -- remove custom reward-token system addresses -- */
	/* FINISH ALPHA CHANGE 610 -- remove custom reward-token system addresses -- */
	/* START ALPHA CHANGE 533 -- remove pre-funded epoch reward pool system addresses -- */
	/* FINISH ALPHA CHANGE 533 -- remove pre-funded epoch reward pool system addresses -- */
	/* START ALPHA CHANGE 487 -- remove legacy EIP1559 bridge burn proxy address -- */
	/* FINISH ALPHA CHANGE 487 -- remove legacy EIP1559 bridge burn proxy address -- */
	/* START ALPHA CHANGE 65 -- reserve INNBC validator registry system contract addresses -- */
	// INNBCValidatorRegistryContract is an address of INNBC validator registry proxy contract on the child chain
	INNBCValidatorRegistryContract = types.StringToAddress("0x107")
	// INNBCValidatorRegistryContractV1 is an address of INNBC validator registry implementation contract on the child chain
	INNBCValidatorRegistryContractV1 = types.StringToAddress("0x1071")
	/* FINISH ALPHA CHANGE 65 -- reserve INNBC validator registry system contract addresses -- */
	/* START ALPHA CHANGE 238 -- reserve INNBC treasury DAO system contract addresses -- */
	// INNBCTreasuryDAOContract is an address of INNBC treasury DAO proxy contract on the child chain
	INNBCTreasuryDAOContract = types.StringToAddress("0x108")
	// INNBCTreasuryDAOContractV1 is an address of INNBC treasury DAO implementation contract on the child chain
	INNBCTreasuryDAOContractV1 = types.StringToAddress("0x1081")
	/* FINISH ALPHA CHANGE 238 -- reserve INNBC treasury DAO system contract addresses -- */
	/* START ALPHA CHANGE 262 -- reserve INNBC governance module system contract addresses -- */
	// INNBCValidatorGovernanceContract is an address of INNBC validator governance proxy contract on the child chain
	INNBCValidatorGovernanceContract = types.StringToAddress("0x109")
	// INNBCValidatorGovernanceContractV1 is an address of INNBC validator governance implementation contract on the child chain
	INNBCValidatorGovernanceContractV1 = types.StringToAddress("0x1091")
	// INNBCBlockRewardGovernanceContract is an address of INNBC block reward governance proxy contract on the child chain
	INNBCBlockRewardGovernanceContract = types.StringToAddress("0x10a")
	// INNBCBlockRewardGovernanceContractV1 is an address of INNBC block reward governance implementation contract on the child chain
	INNBCBlockRewardGovernanceContractV1 = types.StringToAddress("0x10a1")
	/* FINISH ALPHA CHANGE 262 -- reserve INNBC governance module system contract addresses -- */
	/* START ALPHA CHANGE 801 -- reserve INNBC fee burn vault system contract addresses -- */
	// INNBCFeeBurnVaultContract is an address of INNBC fee burn vault proxy contract on the child chain
	INNBCFeeBurnVaultContract = types.StringToAddress("0x10b")
	// INNBCFeeBurnVaultContractV1 is an address of INNBC fee burn vault implementation contract on the child chain
	INNBCFeeBurnVaultContractV1 = types.StringToAddress("0x10b1")
	/* FINISH ALPHA CHANGE 801 -- reserve INNBC fee burn vault system contract addresses -- */
	/* START ALPHA CHANGE 945 -- reserve INNBC community reward pool system contract addresses -- */
	// INNBCCommunityRewardPoolContract is an address of INNBC community reward pool proxy contract on the child chain
	INNBCCommunityRewardPoolContract = types.StringToAddress("0x10d")
	// INNBCCommunityRewardPoolContractV1 is an address of INNBC community reward pool implementation contract on the child chain
	INNBCCommunityRewardPoolContractV1 = types.StringToAddress("0x10d1")
	/* FINISH ALPHA CHANGE 945 -- reserve INNBC community reward pool system contract addresses -- */
	/* START ALPHA CHANGE 987 -- split proxy admin governance from transparent proxy executor -- */
	// INNBCProxyAdminGovernanceContract is the non-upgradeable governance controller for system proxy upgrades.
	INNBCProxyAdminGovernanceContract = types.StringToAddress("0x10c")
	// INNBCProxyAdminExecutorContract is the non-upgradeable executor that owns transparent proxy admin slots.
	INNBCProxyAdminExecutorContract = types.StringToAddress("0x10e")
	/* FINISH ALPHA CHANGE 987 -- split proxy admin governance from transparent proxy executor -- */
	/* START ALPHA CHANGE 833 -- remove duplicated fee burn vault system address declarations -- */
	/* FINISH ALPHA CHANGE 833 -- remove duplicated fee burn vault system address declarations -- */
	/* START ALPHA CHANGE 446 -- remove state receiver bridge system addresses -- */
	/* FINISH ALPHA CHANGE 446 -- remove state receiver bridge system addresses -- */
	/* START ALPHA CHANGE 527 -- describe native token system addresses without bridge wording -- */
	// NativeERC20TokenContract is the INNBC native token proxy system contract address.
	NativeERC20TokenContract = types.StringToAddress("0x1010")
	// NativeERC20TokenContractV1 is the INNBC native token implementation system contract address.
	NativeERC20TokenContractV1 = types.StringToAddress("0x10101")
	/* FINISH ALPHA CHANGE 527 -- describe native token system addresses without bridge wording -- */
	/* START ALPHA CHANGE 447 -- remove L2 state sender bridge system addresses -- */
	/* FINISH ALPHA CHANGE 447 -- remove L2 state sender bridge system addresses -- */

	/* START ALPHA CHANGE 457 -- remove legacy child token and predicate system addresses -- */
	/* FINISH ALPHA CHANGE 457 -- remove legacy child token and predicate system addresses -- */
	/* START ALPHA CHANGE 297 -- prune legacy bridge mintable predicate system addresses -- */
	/* FINISH ALPHA CHANGE 297 -- prune legacy bridge mintable predicate system addresses -- */

	// SystemCaller is address of account, used for system calls to smart contracts
	SystemCaller = types.StringToAddress("0xffffFFFfFFffffffffffffffFfFFFfffFFFfFFfE")

	// NativeTransferPrecompile is an address of native transfer precompile
	NativeTransferPrecompile = types.StringToAddress("0x2020")
	/* START ALPHA CHANGE 2914 -- remove retired aggregate BLS precompile address declaration -- */
	/* FINISH ALPHA CHANGE 2914 -- remove retired aggregate BLS precompile address declaration -- */
	/* START ALPHA CHANGE 3781 -- remove the unused Hardhat console precompile address -- */
	/* FINISH ALPHA CHANGE 3781 -- remove the unused Hardhat console precompile address -- */
	// AllowListContractsAddr is the address of the contract deployer allow list
	AllowListContractsAddr = types.StringToAddress("0x0200000000000000000000000000000000000000")
	// BlockListContractsAddr is the address of the contract deployer block list
	BlockListContractsAddr = types.StringToAddress("0x0300000000000000000000000000000000000000")
	// AllowListTransactionsAddr is the address of the transactions allow list
	AllowListTransactionsAddr = types.StringToAddress("0x0200000000000000000000000000000000000002")
	// BlockListTransactionsAddr is the address of the transactions block list
	BlockListTransactionsAddr = types.StringToAddress("0x0300000000000000000000000000000000000002")
	/* START ALPHA CHANGE 443 -- remove bridge allow/block list system addresses -- */
	/* FINISH ALPHA CHANGE 443 -- remove bridge allow/block list system addresses -- */
)

// GetProxyImplementationMapping retrieves the addresses of proxy contracts that should be deployed unconditionally
func GetProxyImplementationMapping() map[types.Address]types.Address {
	return map[types.Address]types.Address{
		/* START ALPHA CHANGE 448 -- remove state receiver from unconditional proxy deployment -- */
		/* FINISH ALPHA CHANGE 448 -- remove state receiver from unconditional proxy deployment -- */
		/* START ALPHA CHANGE 3770 -- remove retired Merkle system addresses and proxy mapping -- */
		BLSContract: BLSContractV1,
		/* FINISH ALPHA CHANGE 3770 -- remove retired Merkle system addresses and proxy mapping -- */
		/* START ALPHA CHANGE 116 -- remove bridge L2 state sender from unconditional proxy deployment -- */
		/* FINISH ALPHA CHANGE 116 -- remove bridge L2 state sender from unconditional proxy deployment -- */
		ValidatorSetContract: ValidatorSetContractV1,
		/* START ALPHA CHANGE 534 -- stop deploying pre-funded epoch reward pool proxy -- */
		/* FINISH ALPHA CHANGE 534 -- stop deploying pre-funded epoch reward pool proxy -- */
		/* START ALPHA CHANGE 96 -- deploy INNBC validator registry through genesis proxy mapping -- */
		INNBCValidatorRegistryContract: INNBCValidatorRegistryContractV1,
		/* FINISH ALPHA CHANGE 96 -- deploy INNBC validator registry through genesis proxy mapping -- */
		/* START ALPHA CHANGE 239 -- deploy INNBC treasury DAO through genesis proxy mapping -- */
		INNBCTreasuryDAOContract: INNBCTreasuryDAOContractV1,
		/* FINISH ALPHA CHANGE 239 -- deploy INNBC treasury DAO through genesis proxy mapping -- */
		/* START ALPHA CHANGE 263 -- deploy INNBC governance modules through genesis proxy mapping -- */
		INNBCValidatorGovernanceContract:   INNBCValidatorGovernanceContractV1,
		INNBCBlockRewardGovernanceContract: INNBCBlockRewardGovernanceContractV1,
		/* FINISH ALPHA CHANGE 263 -- deploy INNBC governance modules through genesis proxy mapping -- */
		/* START ALPHA CHANGE 802 -- deploy INNBC fee burn vault through genesis proxy mapping -- */
		INNBCFeeBurnVaultContract: INNBCFeeBurnVaultContractV1,
		/* FINISH ALPHA CHANGE 802 -- deploy INNBC fee burn vault through genesis proxy mapping -- */
		/* START ALPHA CHANGE 946 -- deploy INNBC community reward pool through genesis proxy mapping -- */
		INNBCCommunityRewardPoolContract: INNBCCommunityRewardPoolContractV1,
		/* FINISH ALPHA CHANGE 946 -- deploy INNBC community reward pool through genesis proxy mapping -- */
		NativeERC20TokenContract: NativeERC20TokenContractV1,
		/* START ALPHA CHANGE 117 -- remove bridge predicates from unconditional proxy deployment -- */
		/* FINISH ALPHA CHANGE 117 -- remove bridge predicates from unconditional proxy deployment -- */
	}
}
