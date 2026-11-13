package innbcbft

import (
	"fmt"
	"math/big"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/ethgo/abi"
)

const (
	contractCallGasLimit = 100_000_000
)

/* START ALPHA CHANGE 3419 -- initialize ValidatorSet using only its epoch size -- */
// initValidatorSet initializes ValidatorSet epoch tracking.
func initValidatorSet(innbcBFTConfig INNBCBFTConfig, transition *state.Transition) error {
	initFn := &contractsapi.InitializeValidatorSetFn{
		NewEpochSize: new(big.Int).SetUint64(innbcBFTConfig.EpochSize),
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("ValidatorSet.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.ValidatorSetContract, input, "ValidatorSet.initialize", transition)
}

/* FINISH ALPHA CHANGE 3419 -- initialize ValidatorSet using only its epoch size -- */

/* START ALPHA CHANGE 560 -- remove pre-funded reward pool initializer -- */
/* FINISH ALPHA CHANGE 560 -- remove pre-funded reward pool initializer -- */

/* START ALPHA CHANGE 101 -- initialize INNBC validator registry during genesis -- */
func initValidatorRegistry(innbcbftConfig INNBCBFTConfig, transition *state.Transition) error {
	registryConfig := innbcbftConfig.ValidatorRegistryConfig

	/* START ALPHA CHANGE 2643 -- reject unsupported genesis stake before contract initialization -- */
	if err := registryConfig.validateGenesisMinimumStake(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2643 -- reject unsupported genesis stake before contract initialization -- */

	proposalDurationPreset, err := ParseValidatorRegistryProposalDurationPreset(registryConfig.ProposalDurationPreset)
	if err != nil {
		return fmt.Errorf("invalid INNBC validator registry proposal duration preset: %w", err)
	}

	/* START ALPHA CHANGE 1912 -- initialize the registry exclusively through fixed system addresses -- */
	initFn := &contractsapi.InitializeINNBCValidatorRegistryFn{
		GenesisValidator:       registryConfig.GenesisValidator,
		MinimumStake:           registryConfig.MinimumStake,
		ProposalDurationPreset: proposalDurationPreset,
		/* START ALPHA CHANGE 272 -- initialize registry with validator governance controller -- */
		ValidatorGovernance: contracts.INNBCValidatorGovernanceContract,
		/* FINISH ALPHA CHANGE 272 -- initialize registry with validator governance controller -- */
		/* START ALPHA CHANGE 292 -- initialize registry native staking token and activation epoch -- */
		NativeToken:     contracts.NativeERC20TokenContract,
		ActivationEpoch: new(big.Int).SetUint64(registryConfig.ActivationEpoch),
		/* FINISH ALPHA CHANGE 292 -- initialize registry native staking token and activation epoch -- */
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("INNBCValidatorRegistry.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.INNBCValidatorRegistryContract, input, "INNBCValidatorRegistry.initialize", transition)
	/* FINISH ALPHA CHANGE 1912 -- initialize the registry exclusively through fixed system addresses -- */
}

/* FINISH ALPHA CHANGE 101 -- initialize INNBC validator registry during genesis -- */

/* START ALPHA CHANGE 273 -- initialize INNBC validator governance during genesis -- */
func initValidatorGovernance(innbcbftConfig INNBCBFTConfig, transition *state.Transition) error {
	registryConfig := innbcbftConfig.ValidatorRegistryConfig
	if registryConfig == nil {
		return fmt.Errorf("missing INNBC validator registry config")
	}

	proposalDurationPreset, err := ParseValidatorRegistryProposalDurationPreset(registryConfig.ProposalDurationPreset)
	if err != nil {
		return fmt.Errorf("invalid INNBC validator governance proposal duration preset: %w", err)
	}

	/* START ALPHA CHANGE 1912 -- initialize validator governance through fixed system addresses -- */
	initFn := &contractsapi.InitializeINNBCValidatorGovernanceFn{
		ValidatorRegistry_:      contracts.INNBCValidatorRegistryContract,
		ProposalDurationPreset_: proposalDurationPreset,
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("INNBCValidatorGovernance.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.INNBCValidatorGovernanceContract, input, "INNBCValidatorGovernance.initialize", transition)
	/* FINISH ALPHA CHANGE 1912 -- initialize validator governance through fixed system addresses -- */
}

/* FINISH ALPHA CHANGE 273 -- initialize INNBC validator governance during genesis -- */

/* START ALPHA CHANGE 274 -- initialize INNBC block reward governance during genesis -- */
func initBlockRewardGovernance(innbcbftConfig INNBCBFTConfig, transition *state.Transition) error {
	registryConfig := innbcbftConfig.ValidatorRegistryConfig
	if registryConfig == nil {
		return fmt.Errorf("missing INNBC validator registry config")
	}

	/* START ALPHA CHANGE 1912 -- initialize block reward governance through fixed system addresses -- */
	initFn := &contractsapi.InitializeINNBCBlockRewardGovernanceFn{
		ValidatorRegistry_: contracts.INNBCValidatorRegistryContract,
		NativeToken_:       contracts.NativeERC20TokenContract,
		/* START ALPHA CHANGE 960 -- initialize block reward governance with community reward pool -- */
		CommunityRewardPool_: contracts.INNBCCommunityRewardPoolContract,
		/* FINISH ALPHA CHANGE 960 -- initialize block reward governance with community reward pool -- */
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("INNBCBlockRewardGovernance.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.INNBCBlockRewardGovernanceContract, input, "INNBCBlockRewardGovernance.initialize", transition)
	/* FINISH ALPHA CHANGE 1912 -- initialize block reward governance through fixed system addresses -- */
}

/* FINISH ALPHA CHANGE 274 -- initialize INNBC block reward governance during genesis -- */

/* START ALPHA CHANGE 961 -- initialize INNBC community reward pool during genesis -- */
func initCommunityRewardPool(innbcbftConfig INNBCBFTConfig, transition *state.Transition) error {
	registryConfig := innbcbftConfig.ValidatorRegistryConfig
	if registryConfig == nil {
		return fmt.Errorf("missing INNBC validator registry config")
	}

	/* START ALPHA CHANGE 1912 -- initialize the community pool through fixed system addresses -- */
	initFn := &contractsapi.InitializeINNBCCommunityRewardPoolFn{
		ValidatorRegistry_:     contracts.INNBCValidatorRegistryContract,
		BlockRewardGovernance_: contracts.INNBCBlockRewardGovernanceContract,
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("INNBCCommunityRewardPool.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.INNBCCommunityRewardPoolContract, input, "INNBCCommunityRewardPool.initialize", transition)
	/* FINISH ALPHA CHANGE 1912 -- initialize the community pool through fixed system addresses -- */
}

/* FINISH ALPHA CHANGE 961 -- initialize INNBC community reward pool during genesis -- */

/* START ALPHA CHANGE 993 -- initialize split proxy admin governance and executor during genesis -- */
func initProxyAdminExecutor(transition *state.Transition) error {
	initFn := &contractsapi.InitializeINNBCProxyAdminExecutorFn{
		Governance_: contracts.INNBCProxyAdminGovernanceContract,
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("INNBCProxyAdminExecutor.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.INNBCProxyAdminExecutorContract, input, "INNBCProxyAdminExecutor.initialize", transition)
}

func initProxyAdminGovernance(transition *state.Transition) error {
	initFn := &contractsapi.InitializeINNBCProxyAdminGovernanceFn{
		ValidatorRegistry_:  contracts.INNBCValidatorRegistryContract,
		ProxyAdminExecutor_: contracts.INNBCProxyAdminExecutorContract,
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("INNBCProxyAdminGovernance.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.INNBCProxyAdminGovernanceContract, input, "INNBCProxyAdminGovernance.initialize", transition)
}

/* FINISH ALPHA CHANGE 993 -- initialize split proxy admin governance and executor during genesis -- */

/* START ALPHA CHANGE 804 -- initialize INNBC fee burn vault during genesis -- */
func initFeeBurnVault(transition *state.Transition) error {
	initFn := &contractsapi.InitializeINNBCFeeBurnVaultFn{
		NativeToken_: contracts.NativeERC20TokenContract,
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("INNBCFeeBurnVault.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.INNBCFeeBurnVaultContract, input, "INNBCFeeBurnVault.initialize", transition)
}

/* FINISH ALPHA CHANGE 804 -- initialize INNBC fee burn vault during genesis -- */

/* START ALPHA CHANGE 245 -- initialize INNBC treasury DAO during genesis -- */
func initTreasuryDAO(transition *state.Transition) error {
	initFn := &contractsapi.InitializeINNBCTreasuryDAOFn{
		/* START ALPHA CHANGE 247 -- use generated TreasuryDAO initializer field names -- */
		ValidatorRegistry_: contracts.INNBCValidatorRegistryContract,
		NativeToken_:       contracts.NativeERC20TokenContract,
		/* FINISH ALPHA CHANGE 247 -- use generated TreasuryDAO initializer field names -- */
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("INNBCTreasuryDAO.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.INNBCTreasuryDAOContract, input, "INNBCTreasuryDAO.initialize", transition)
}

/* FINISH ALPHA CHANGE 245 -- initialize INNBC treasury DAO during genesis -- */

/* START ALPHA CHANGE 111 -- remove bridge predicate initializer helpers from sovereign consensus -- */
/* FINISH ALPHA CHANGE 111 -- remove bridge predicate initializer helpers from sovereign consensus -- */

/* START ALPHA CHANGE 561 -- remove reward-token mint and reward-pool approval helpers -- */
/* FINISH ALPHA CHANGE 561 -- remove reward-token mint and reward-pool approval helpers -- */

// callContract calls given smart contract function, encoded in input parameter
/* START ALPHA CHANGE 2158 -- check system contract state errors before interpreting return data -- */
func callContract(from, to types.Address, input []byte, contractName string, transition *state.Transition) error {
	result := transition.Call2(from, to, input, big.NewInt(0), contractCallGasLimit)
	if err := transition.Error(); err != nil {
		return fmt.Errorf("%s contract state access failed: %w", contractName, err)
	}
	if result.Failed() {
		if result.Reverted() {
			if revertReason, err := abi.UnpackRevertError(result.ReturnValue); err == nil {
				return fmt.Errorf("%s contract call was reverted: %s", contractName, revertReason)
			}
		}

		return fmt.Errorf("%s contract call failed: %w", contractName, result.Err)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2158 -- check system contract state errors before interpreting return data -- */

/* START ALPHA CHANGE 562 -- remove native reward-token classifier after reward pool prune -- */
/* FINISH ALPHA CHANGE 562 -- remove native reward-token classifier after reward pool prune -- */
