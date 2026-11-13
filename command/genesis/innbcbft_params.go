package genesis

import (
	/* START ALPHA CHANGE 549 -- remove custom reward-token bytecode decoding import -- */
	/* FINISH ALPHA CHANGE 549 -- remove custom reward-token bytecode decoding import -- */
	/* START ALPHA CHANGE 2018 -- strictly decode explicit genesis operator addresses -- */
	"encoding/hex"
	/* FINISH ALPHA CHANGE 2018 -- strictly decode explicit genesis operator addresses -- */
	"errors"
	"fmt"
	"math/big"
	"path"
	"strings"
	"time"

	/* START ALPHA CHANGE 3841 -- share peer identity parsing with directory bootstrap -- */
	networkcommon "github.com/innovativebioresearch/innbc-chain/network/common"
	/* FINISH ALPHA CHANGE 3841 -- share peer identity parsing with directory bootstrap -- */

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi/artifact"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/server"
	"github.com/innovativebioresearch/innbc-chain/types"
)

const (
	sprintSizeFlag = "sprint-size"
	blockTimeFlag  = "block-time"
	trieRootFlag   = "trieroot"

	/* START ALPHA CHANGE 3319 -- retire the genesis drift flag -- */
	/* FINISH ALPHA CHANGE 3319 -- retire the genesis drift flag -- */

	/*START ALPHA CHANGE 19 -- remove unused INNBCBFT genesis default constants reported by analyzer --*/
	defaultSprintSize = uint64(5)
	defaultBlockTime  = 2 * time.Second
	/* START ALPHA CHANGE 550 -- remove pre-funded epoch reward default -- */
	/* FINISH ALPHA CHANGE 550 -- remove pre-funded epoch reward default -- */
	/* START ALPHA CHANGE 3319 -- retain polling without a drift default -- */
	defaultBlockTrackerPollInterval = time.Second
	/* FINISH ALPHA CHANGE 3319 -- retain polling without a drift default -- */
	/*FINISH ALPHA CHANGE 19 -- remove unused INNBCBFT genesis default constants reported by analyzer --*/

	contractDeployerAllowListAdminFlag   = "contract-deployer-allow-list-admin"
	contractDeployerAllowListEnabledFlag = "contract-deployer-allow-list-enabled"
	contractDeployerBlockListAdminFlag   = "contract-deployer-block-list-admin"
	contractDeployerBlockListEnabledFlag = "contract-deployer-block-list-enabled"
	transactionsAllowListAdminFlag       = "transactions-allow-list-admin"
	transactionsAllowListEnabledFlag     = "transactions-allow-list-enabled"
	transactionsBlockListAdminFlag       = "transactions-block-list-admin"
	transactionsBlockListEnabledFlag     = "transactions-block-list-enabled"
	/* START ALPHA CHANGE 112 -- remove bridge allow/block list genesis flags from INNBCBFT -- */
	/* FINISH ALPHA CHANGE 112 -- remove bridge allow/block list genesis flags from INNBCBFT -- */

	/* START ALPHA CHANGE 638 -- remove legacy bootnode port seed from INNBCBFT genesis -- */
	/* FINISH ALPHA CHANGE 638 -- remove legacy bootnode port seed from INNBCBFT genesis -- */

	/* START ALPHA CHANGE 2019 -- define explicit genesis identity and proof lengths -- */
	ecdsaAddressLength = 40
	blsKeyLength       = 256
	blsProofLength     = 128
	/* FINISH ALPHA CHANGE 2019 -- define explicit genesis identity and proof lengths -- */
)

var (
	errNoGenesisValidators = errors.New("genesis validators aren't provided")
	/* START ALPHA CHANGE 781 -- remove obsolete native-token mintable premine restriction -- */
	/* FINISH ALPHA CHANGE 781 -- remove obsolete native-token mintable premine restriction -- */
)

type contractInfo struct {
	artifact *artifact.Artifact
	address  types.Address
}

// generatePolyBftChainConfig creates and persists innbcbft chain configuration to the provided file path
func (p *genesisParams) generatePolyBftChainConfig(o command.OutputFormatter) error {
	/* START ALPHA CHANGE 3050 -- reject oversized chain IDs before INNBC genesis construction -- */
	if err := p.validateChainID(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3050 -- reject oversized chain IDs before INNBC genesis construction -- */

	/* START ALPHA CHANGE 3179 -- reject malformed access-list input before INNBC genesis construction -- */
	if err := p.validateAccessListAddresses(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3179 -- reject malformed access-list input before INNBC genesis construction -- */

	/* START ALPHA CHANGE 3183 -- parse the external trie root before INNBC genesis construction -- */
	initialTrieRoot, err := p.parseInitialTrieRoot()
	if err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3183 -- parse the external trie root before INNBC genesis construction -- */

	// populate premine balance map
	premineBalances := make(map[types.Address]*helper.PremineInfo, len(p.premine))

	for _, premine := range p.premineInfos {
		premineBalances[premine.Address] = premine
	}

	/* START ALPHA CHANGE 782 -- allow premine with mandatory INNBC native token -- */
	/* FINISH ALPHA CHANGE 782 -- allow premine with mandatory INNBC native token -- */

	initialValidators, err := p.getValidatorAccounts()
	if err != nil {
		return fmt.Errorf("failed to retrieve genesis validators: %w", err)
	}

	if len(initialValidators) == 0 {
		return errNoGenesisValidators
	}

	/* START ALPHA CHANGE 3871 -- reject duplicate peer identities during genesis generation only -- */
	peerOwners := make(map[string]types.Address, len(initialValidators))

	for _, entry := range initialValidators {
		info, err := networkcommon.StringToAddrInfo(entry.MultiAddr)
		if err != nil {
			return fmt.Errorf(
				"invalid P2P multi address for genesis validator %s: %w",
				entry.Address, err,
			)
		}

		peerID := info.ID.String()
		if previousOwner, exists := peerOwners[peerID]; exists {
			return fmt.Errorf(
				"duplicate genesis peer ID %s used by validators %s and %s",
				peerID, previousOwner, entry.Address,
			)
		}

		peerOwners[peerID] = entry.Address
	}
	/* FINISH ALPHA CHANGE 3871 -- reject duplicate peer identities during genesis generation only -- */

	if _, err := o.Write([]byte("[GENESIS VALIDATORS]\n")); err != nil {
		return err
	}

	for _, v := range initialValidators {
		/* START ALPHA CHANGE 793 -- use fmt.Appendf for genesis validator output -- */
		if _, err := o.Write(fmt.Appendf(nil, "%v\n", v)); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 793 -- use fmt.Appendf for genesis validator output -- */
	}

	innbcBFTConfig := &innbcbft.INNBCBFTConfig{
		InitialValidatorSet: initialValidators,
		BlockTime:           common.Duration{Duration: p.blockTime},
		EpochSize:           p.epochSize,
		SprintSize:          p.sprintSize,
		/* START ALPHA CHANGE 552 -- remove pre-funded epoch reward from INNBCBFT genesis config -- */
		/* FINISH ALPHA CHANGE 552 -- remove pre-funded epoch reward from INNBCBFT genesis config -- */
		// use 1st account as governance address
		Governance: types.ZeroAddress,
		/* START ALPHA CHANGE 3183 -- serialize the validated external trie root without permissive conversion -- */
		InitialTrieRoot: initialTrieRoot,
		/* FINISH ALPHA CHANGE 3183 -- serialize the validated external trie root without permissive conversion -- */
		NativeTokenConfig:   p.nativeTokenConfig,
		MinValidatorSetSize: p.minNumValidators,
		MaxValidatorSetSize: p.maxNumValidators,
		/* START ALPHA CHANGE 553 -- remove reward wallet and custom reward-token config from INNBCBFT genesis -- */
		/* FINISH ALPHA CHANGE 553 -- remove reward wallet and custom reward-token config from INNBCBFT genesis -- */
		/* START ALPHA CHANGE 74 -- write INNBC validator registry config into genesis -- */
		/* START ALPHA CHANGE 1911 -- omit fixed system addresses from generated consensus configuration -- */
		ValidatorRegistryConfig: &innbcbft.ValidatorRegistryConfig{
			GenesisValidator: initialValidators[0].Address,
			MinimumStake:     p.validatorRegistryMinimumStake,
			/* START ALPHA CHANGE 269 -- write INNBC governance module addresses into genesis config -- */
			/* START ALPHA CHANGE 959 -- write INNBC community reward pool addresses into genesis config -- */
			/* FINISH ALPHA CHANGE 959 -- write INNBC community reward pool addresses into genesis config -- */
			/* FINISH ALPHA CHANGE 269 -- write INNBC governance module addresses into genesis config -- */
			ProposalDurationPreset: p.validatorRegistryProposalDurationPreset,
			/* START ALPHA CHANGE 293 -- set fixed registry activation epoch in genesis config -- */
			ActivationEpoch: 2,
			/* FINISH ALPHA CHANGE 293 -- set fixed registry activation epoch in genesis config -- */
		},
		/* FINISH ALPHA CHANGE 1911 -- omit fixed system addresses from generated consensus configuration -- */
		/* FINISH ALPHA CHANGE 74 -- write INNBC validator registry config into genesis -- */
		/* START ALPHA CHANGE 3319 -- omit retired drift from generated consensus configuration -- */
		BlockTrackerPollInterval: common.Duration{Duration: p.blockTrackerPollInterval},
		/* FINISH ALPHA CHANGE 3319 -- omit retired drift from generated consensus configuration -- */
		/* START ALPHA CHANGE 988 -- use isolated executor as fixed transparent proxy admin -- */
		/* FINISH ALPHA CHANGE 988 -- use isolated executor as fixed transparent proxy admin -- */
		/* START ALPHA CHANGE 1911 -- stop serializing the fixed proxy administrator -- */
		/* FINISH ALPHA CHANGE 1911 -- stop serializing the fixed proxy administrator -- */
	}

	/* START ALPHA CHANGE 505 -- keep London fork independent from legacy burn-contract flag -- */
	enabledForks := chain.AllForksEnabled
	/* FINISH ALPHA CHANGE 505 -- keep London fork independent from legacy burn-contract flag -- */

	chainConfig := &chain.Chain{
		Name: p.name,
		Params: &chain.Params{
			ChainID: int64(p.chainID),
			Forks:   enabledForks,
			Engine: map[string]interface{}{
				string(server.INNBCBFTConsensus): innbcBFTConfig,
			},
		},
		/* START ALPHA CHANGE 639 -- stop writing legacy bootnodes to INNBCBFT genesis -- */
		/* FINISH ALPHA CHANGE 639 -- stop writing legacy bootnodes to INNBCBFT genesis -- */
	}

	/* START ALPHA CHANGE 506 -- remove legacy burn-contract genesis config for INNBCBFT -- */
	/* FINISH ALPHA CHANGE 506 -- remove legacy burn-contract genesis config for INNBCBFT -- */

	// deploy genesis contracts
	/* START ALPHA CHANGE 554 -- remove custom reward-token bytecode from genesis contract deployment -- */
	allocs, err := p.deployContracts()
	/* FINISH ALPHA CHANGE 554 -- remove custom reward-token bytecode from genesis contract deployment -- */
	if err != nil {
		return err
	}

	/* START ALPHA CHANGE 959 -- allow premine balances on genesis contract allocations -- */
	// premine accounts, preserving code/storage for predeployed system contracts
	for _, premine := range premineBalances {
		if alloc, ok := allocs[premine.Address]; ok {
			alloc.Balance = new(big.Int).Set(premine.Amount)

			continue
		}

		allocs[premine.Address] = &chain.GenesisAccount{
			Balance: new(big.Int).Set(premine.Amount),
		}
	}
	/* FINISH ALPHA CHANGE 959 -- allow premine balances on genesis contract allocations -- */

	/* START ALPHA CHANGE 2020 -- validate genesis proofs uniqueness and set bounds before serialization -- */
	validatorMetadata, err := innbcbft.BuildGenesisValidatorSet(innbcBFTConfig)
	if err != nil {
		return fmt.Errorf("invalid genesis validator set: %w", err)
	}
	/* FINISH ALPHA CHANGE 2020 -- validate genesis proofs uniqueness and set bounds before serialization -- */

	genesisExtraData, err := GenerateExtraDataPolyBft(validatorMetadata)
	if err != nil {
		return err
	}

	// populate genesis parameters
	chainConfig.Genesis = &chain.Genesis{
		/* START ALPHA CHANGE 2049 -- anchor INNBCBFT genesis to its generation time -- */
		Timestamp: uint64(time.Now().Unix()),
		/* FINISH ALPHA CHANGE 2049 -- anchor INNBCBFT genesis to its generation time -- */
		GasLimit:   p.blockGasLimit,
		Difficulty: 0,
		Alloc:      allocs,
		ExtraData:  genesisExtraData,
		GasUsed:    command.DefaultGenesisGasUsed,
		Mixhash:    innbcbft.INNBCBFTMixDigest,
	}

	if len(p.contractDeployerAllowListAdmin) != 0 {
		// only enable allow list if there is at least one address as **admin**, otherwise
		// the allow list could never be updated
		chainConfig.Params.ContractDeployerAllowList = &chain.AddressListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.contractDeployerAllowListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.contractDeployerAllowListEnabled),
		}
	}

	if len(p.contractDeployerBlockListAdmin) != 0 {
		// only enable block list if there is at least one address as **admin**, otherwise
		// the block list could never be updated
		chainConfig.Params.ContractDeployerBlockList = &chain.AddressListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.contractDeployerBlockListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.contractDeployerBlockListEnabled),
		}
	}

	if len(p.transactionsAllowListAdmin) != 0 {
		// only enable allow list if there is at least one address as **admin**, otherwise
		// the allow list could never be updated
		chainConfig.Params.TransactionsAllowList = &chain.AddressListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.transactionsAllowListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.transactionsAllowListEnabled),
		}
	}

	if len(p.transactionsBlockListAdmin) != 0 {
		// only enable block list if there is at least one address as **admin**, otherwise
		// the block list could never be updated
		chainConfig.Params.TransactionsBlockList = &chain.AddressListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.transactionsBlockListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.transactionsBlockListEnabled),
		}
	}

	/* START ALPHA CHANGE 113 -- remove bridge allow/block list config from genesis output -- */
	/* FINISH ALPHA CHANGE 113 -- remove bridge allow/block list config from genesis output -- */

	/* START ALPHA CHANGE 507 -- always write INNBCBFT base-fee genesis config without burn-contract gate -- */
	chainConfig.Genesis.BaseFee = p.parsedBaseFeeConfig.baseFee
	chainConfig.Genesis.BaseFeeEM = p.parsedBaseFeeConfig.baseFeeEM
	chainConfig.Genesis.BaseFeeChangeDenom = p.parsedBaseFeeConfig.baseFeeChangeDenom
	/* FINISH ALPHA CHANGE 507 -- always write INNBCBFT base-fee genesis config without burn-contract gate -- */

	return helper.WriteGenesisConfigToDisk(chainConfig, params.genesisPath)
}

/* START ALPHA CHANGE 555 -- remove custom reward-token bytecode parameter from genesis deployment -- */
func (p *genesisParams) deployContracts() (map[types.Address]*chain.GenesisAccount, error) {
	/* FINISH ALPHA CHANGE 555 -- remove custom reward-token bytecode parameter from genesis deployment -- */
	proxyToImplAddrMap := contracts.GetProxyImplementationMapping()
	proxyAddresses := make([]types.Address, 0, len(proxyToImplAddrMap))

	for proxyAddr := range proxyToImplAddrMap {
		proxyAddresses = append(proxyAddresses, proxyAddr)
	}

	genesisContracts := []*contractInfo{
		/* START ALPHA CHANGE 449 -- remove state receiver bridge implementation from genesis allocation -- */
		/* FINISH ALPHA CHANGE 449 -- remove state receiver bridge implementation from genesis allocation -- */
		/* START ALPHA CHANGE 458 -- remove legacy child token template genesis allocations -- */
		/* FINISH ALPHA CHANGE 458 -- remove legacy child token template genesis allocations -- */
		{
			// BLS contract
			artifact: contractsapi.BLS,
			address:  contracts.BLSContractV1,
		},
		/* START ALPHA CHANGE 3771 -- stop allocating the unused Merkle implementation at genesis -- */
		/* FINISH ALPHA CHANGE 3771 -- stop allocating the unused Merkle implementation at genesis -- */
		/* START ALPHA CHANGE 114 -- stop deploying L2 state sender bridge contract in genesis -- */
		/* FINISH ALPHA CHANGE 114 -- stop deploying L2 state sender bridge contract in genesis -- */
		{
			artifact: contractsapi.ValidatorSet,
			address:  contracts.ValidatorSetContractV1,
		},
		/* START ALPHA CHANGE 535 -- stop allocating pre-funded epoch reward pool implementation in genesis -- */
		/* FINISH ALPHA CHANGE 535 -- stop allocating pre-funded epoch reward pool implementation in genesis -- */
		/* START ALPHA CHANGE 97 -- include INNBC validator registry implementation in genesis allocation -- */
		{
			artifact: contractsapi.INNBCValidatorRegistry,
			address:  contracts.INNBCValidatorRegistryContractV1,
		},
		/* FINISH ALPHA CHANGE 97 -- include INNBC validator registry implementation in genesis allocation -- */
		/* START ALPHA CHANGE 244 -- include INNBC treasury DAO implementation in genesis allocation -- */
		{
			artifact: contractsapi.INNBCTreasuryDAO,
			address:  contracts.INNBCTreasuryDAOContractV1,
		},
		/* FINISH ALPHA CHANGE 244 -- include INNBC treasury DAO implementation in genesis allocation -- */
		/* START ALPHA CHANGE 270 -- include INNBC governance module implementations in genesis allocation -- */
		{
			artifact: contractsapi.INNBCValidatorGovernance,
			address:  contracts.INNBCValidatorGovernanceContractV1,
		},
		{
			artifact: contractsapi.INNBCBlockRewardGovernance,
			address:  contracts.INNBCBlockRewardGovernanceContractV1,
		},
		/* START ALPHA CHANGE 957 -- include INNBC community reward pool implementation in genesis allocation -- */
		{
			artifact: contractsapi.INNBCCommunityRewardPool,
			address:  contracts.INNBCCommunityRewardPoolContractV1,
		},
		/* FINISH ALPHA CHANGE 957 -- include INNBC community reward pool implementation in genesis allocation -- */
		/* FINISH ALPHA CHANGE 270 -- include INNBC governance module implementations in genesis allocation -- */
		/* START ALPHA CHANGE 803 -- include INNBC fee burn vault implementation in genesis allocation -- */
		{
			artifact: contractsapi.INNBCFeeBurnVault,
			address:  contracts.INNBCFeeBurnVaultContractV1,
		},
		/* FINISH ALPHA CHANGE 803 -- include INNBC fee burn vault implementation in genesis allocation -- */
		/* START ALPHA CHANGE 989 -- include split proxy admin governance and executor in genesis allocation -- */
		{
			artifact: contractsapi.INNBCProxyAdminGovernance,
			address:  contracts.INNBCProxyAdminGovernanceContract,
		},
		{
			artifact: contractsapi.INNBCProxyAdminExecutor,
			address:  contracts.INNBCProxyAdminExecutorContract,
		},
		/* FINISH ALPHA CHANGE 989 -- include split proxy admin governance and executor in genesis allocation -- */
	}

	/* START ALPHA CHANGE 783 -- always allocate INNBC native token implementation in genesis -- */
	genesisContracts = append(genesisContracts,
		&contractInfo{
			artifact: contractsapi.INNBCNativeToken,
			address:  contracts.NativeERC20TokenContractV1,
		})
	/* FINISH ALPHA CHANGE 783 -- always allocate INNBC native token implementation in genesis -- */

	/* START ALPHA CHANGE 115 -- stop deploying Polygon bridge predicate contracts in genesis -- */
	/* FINISH ALPHA CHANGE 115 -- stop deploying Polygon bridge predicate contracts in genesis -- */

	allocations := make(map[types.Address]*chain.GenesisAccount, len(genesisContracts))

	/* START ALPHA CHANGE 556 -- remove custom reward-token genesis allocation and proxy deployment -- */
	/* FINISH ALPHA CHANGE 556 -- remove custom reward-token genesis allocation and proxy deployment -- */

	genesisContracts = append(genesisContracts, getProxyContractsInfo(proxyAddresses)...)

	for _, contract := range genesisContracts {
		allocations[contract.address] = &chain.GenesisAccount{
			Balance: big.NewInt(0),
			Code:    contract.artifact.DeployedBytecode,
		}
	}

	return allocations, nil
}

// getValidatorAccounts gathers validator accounts info either from CLI or from provided local storage
func (p *genesisParams) getValidatorAccounts() ([]*validator.GenesisValidator, error) {
	// populate validators premine info
	if len(p.validators) > 0 {
		validators := make([]*validator.GenesisValidator, len(p.validators))
		/* START ALPHA CHANGE 2021 -- parse mandatory genesis proofs without splitting IPv6 multiaddresses -- */
		for i, val := range p.validators {
			var parts [4]string
			remainder := val

			for field := 3; field > 0; field-- {
				separator := strings.LastIndexByte(remainder, ':')
				if separator < 0 {
					return nil, fmt.Errorf(
						"expected <P2P multi address:ECDSA address:public BLS key:BLS proof>",
					)
				}
				parts[field] = remainder[separator+1:]
				remainder = remainder[:separator]
			}
			parts[0] = remainder

			/* START ALPHA CHANGE 3842 -- reject explicit genesis addresses without a valid peer identity -- */
			if _, err := networkcommon.StringToAddrInfo(parts[0]); err != nil {
				return nil, fmt.Errorf("invalid P2P multi address %q: %w", parts[0], err)
			}
			/* FINISH ALPHA CHANGE 3842 -- reject explicit genesis addresses without a valid peer identity -- */

			trimmedAddress := strings.TrimPrefix(parts[1], "0x")
			if len(trimmedAddress) != ecdsaAddressLength {
				return nil, fmt.Errorf("invalid ECDSA address length")
			}
			addressBytes, err := hex.DecodeString(trimmedAddress)
			if err != nil {
				return nil, fmt.Errorf("invalid ECDSA address encoding: %w", err)
			}

			trimmedBLSKey := strings.TrimPrefix(parts[2], "0x")
			if len(trimmedBLSKey) != blsKeyLength {
				return nil, fmt.Errorf("invalid BLS public key length")
			}
			if _, err := hex.DecodeString(trimmedBLSKey); err != nil {
				return nil, fmt.Errorf("invalid BLS public key encoding: %w", err)
			}

			trimmedBLSProof := strings.TrimPrefix(parts[3], "0x")
			if len(trimmedBLSProof) != blsProofLength {
				return nil, fmt.Errorf("invalid BLS proof length")
			}
			if _, err := hex.DecodeString(trimmedBLSProof); err != nil {
				return nil, fmt.Errorf("invalid BLS proof encoding: %w", err)
			}

			/* START ALPHA CHANGE 3422 -- parse genesis identities without obsolete VSET amounts -- */
			validators[i] = &validator.GenesisValidator{
				MultiAddr: parts[0],
				Address:   types.BytesToAddress(addressBytes),
				BlsKey:    trimmedBLSKey,
				BlsProof:  trimmedBLSProof,
			}
			/* FINISH ALPHA CHANGE 3422 -- parse genesis identities without obsolete VSET amounts -- */
		}
		/* FINISH ALPHA CHANGE 2021 -- parse mandatory genesis proofs without splitting IPv6 multiaddresses -- */

		return validators, nil
	}

	validatorsPath := p.validatorsPath
	if validatorsPath == "" {
		validatorsPath = path.Dir(p.genesisPath)
	}

	validators, err := ReadValidatorsByPrefix(validatorsPath, p.validatorsPrefixPath)
	if err != nil {
		return nil, err
	}

	return validators, nil
}

func stringSliceToAddressSlice(addrs []string) []types.Address {
	res := make([]types.Address, len(addrs))
	for indx, addr := range addrs {
		res[indx] = types.StringToAddress(addr)
	}

	return res
}

func getProxyContractsInfo(addresses []types.Address) []*contractInfo {
	result := make([]*contractInfo, len(addresses))

	for i, proxyAddress := range addresses {
		result[i] = &contractInfo{
			artifact: contractsapi.GenesisProxy,
			address:  proxyAddress,
		}
	}

	return result
}
