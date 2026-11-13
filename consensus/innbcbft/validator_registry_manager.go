package innbcbft

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"

	/* START ALPHA CHANGE 1913 -- read validator registry state from its fixed system address -- */
	/* START ALPHA CHANGE 391 -- remove unused types import after event subscriber pruning -- */
	"github.com/innovativebioresearch/innbc-chain/contracts"
)

/* FINISH ALPHA CHANGE 1913 -- read validator registry state from its fixed system address -- */
/* FINISH ALPHA CHANGE 391 -- remove unused types import after event subscriber pruning -- */

/* START ALPHA CHANGE 139 -- add INNBC-native validator registry stake manager -- */
var _ StakeManager = (*validatorRegistryManager)(nil)

/* START ALPHA CHANGE 2012 -- remove the unused initial set copy after centralized genesis validation -- */
type validatorRegistryManager struct {
	logger              hclog.Logger
	blockchain          blockchainBackend
	registryConfig      *ValidatorRegistryConfig
	maxValidatorSetSize int
}

func newValidatorRegistryManager(
	logger hclog.Logger,
	blockchain blockchainBackend,
	registryConfig *ValidatorRegistryConfig,
	maxValidatorSetSize int,
) (*validatorRegistryManager, error) {
	/* START ALPHA CHANGE 2644 -- enforce the one-wei floor for direct registry manager construction -- */
	if err := registryConfig.validateGenesisMinimumStake(); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2644 -- enforce the one-wei floor for direct registry manager construction -- */

	return &validatorRegistryManager{
		logger:              logger,
		blockchain:          blockchain,
		registryConfig:      registryConfig,
		maxValidatorSetSize: maxValidatorSetSize,
	}, nil
}

/* FINISH ALPHA CHANGE 2012 -- remove the unused initial set copy after centralized genesis validation -- */

func (v *validatorRegistryManager) PostBlock(req *PostBlockRequest) error {
	return nil
}

func (v *validatorRegistryManager) UpdateValidatorSet(
	epoch uint64,
	currentValidatorSet validator.AccountSet,
) (*validator.ValidatorSetDelta, error) {
	v.logger.Info("Calculating INNBC validator registry set update...", "epoch", epoch)

	registryState, err := v.getRegistryState()
	if err != nil {
		return nil, err
	}

	registryActivated, err := registryState.IsRegistryActivated()
	if err != nil {
		return nil, fmt.Errorf("failed to read validator registry activation status: %w", err)
	}

	if !registryActivated {
		v.logger.Debug("Validator registry is not active; keeping current genesis-derived validator set", "epoch", epoch)

		return &validator.ValidatorSetDelta{}, nil
	}

	activationEpoch, err := registryState.GetActivationEpoch()
	if err != nil {
		return nil, fmt.Errorf("failed to read validator registry activation epoch: %w", err)
	}

	if epoch < activationEpoch {
		v.logger.Debug(
			"Validator registry is active but not authoritative for this epoch yet",
			"epoch", epoch,
			"activationEpoch", activationEpoch,
		)

		return &validator.ValidatorSetDelta{}, nil
	}

	newValidatorSet, err := v.getRegistryValidatorSet(registryState)
	if err != nil {
		return nil, err
	}

	delta, err := validator.CreateValidatorSetDelta(currentValidatorSet, newValidatorSet)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate INNBC validator registry set delta: %w", err)
	}

	v.logger.Info(
		"Calculating INNBC validator registry set update finished.",
		"epoch", epoch,
		"activeValidators", len(newValidatorSet),
	)

	return delta, nil
}

/* START ALPHA CHANGE 378 -- remove legacy event subscriber methods from validator registry manager -- */
/* FINISH ALPHA CHANGE 378 -- remove legacy event subscriber methods from validator registry manager -- */

func (v *validatorRegistryManager) getRegistryState() (ValidatorRegistryState, error) {
	currentHeader := v.blockchain.CurrentHeader()
	if currentHeader == nil {
		return nil, fmt.Errorf("current header is not available")
	}

	provider, err := v.blockchain.GetStateProviderForBlock(currentHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to get validator registry state provider: %w", err)
	}

	/* START ALPHA CHANGE 1913 -- read validator registry state from its fixed system address -- */
	return NewValidatorRegistryState(contracts.INNBCValidatorRegistryContract, provider), nil
	/* FINISH ALPHA CHANGE 1913 -- read validator registry state from its fixed system address -- */
}

func (v *validatorRegistryManager) getRegistryValidatorSet(
	registryState ValidatorRegistryState,
) (validator.AccountSet, error) {
	activeValidators, err := registryState.GetActiveValidators()
	if err != nil {
		return nil, fmt.Errorf("failed to read active validator registry list: %w", err)
	}

	newValidatorSet := make(validator.AccountSet, 0, len(activeValidators))

	for _, operator := range activeValidators {
		registryValidator, err := registryState.GetValidator(operator)
		if err != nil {
			return nil, fmt.Errorf("failed to read validator registry record %s: %w", operator, err)
		}

		if !registryValidator.Registered || !registryValidator.Whitelisted {
			continue
		}

		if registryValidator.Stake == nil || registryValidator.Stake.Cmp(v.registryConfig.MinimumStake) < 0 {
			continue
		}

		/* START ALPHA CHANGE 2013 -- require the packed proof flag before admitting a registry key -- */
		if !registryValidator.BlsProofVerified {
			return nil, fmt.Errorf(
				"active registry validator %s has no verified BLS possession proof",
				operator,
			)
		}
		if len(registryValidator.BlsKey) != bls.PublicKeySize {
			return nil, fmt.Errorf(
				"active registry validator %s has a noncanonical BLS key length",
				operator,
			)
		}
		/* FINISH ALPHA CHANGE 2013 -- require the packed proof flag before admitting a registry key -- */

		blsKey, err := bls.UnmarshalPublicKey(registryValidator.BlsKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode registry BLS key for validator %s: %w", operator, err)
		}

		newValidatorSet = append(newValidatorSet, &validator.ValidatorMetadata{
			Address: registryValidator.Operator,
			BlsKey:  blsKey,
			/* START ALPHA CHANGE 171 -- use equal consensus voting power for registry validators -- */
			VotingPower: big.NewInt(1),
			/* FINISH ALPHA CHANGE 171 -- use equal consensus voting power for registry validators -- */
			IsActive: true,
		})
	}

	sort.SliceStable(newValidatorSet, func(i, j int) bool {
		return newValidatorSet[i].Address.String() < newValidatorSet[j].Address.String()
	})

	if v.maxValidatorSetSize > 0 && len(newValidatorSet) > v.maxValidatorSetSize {
		newValidatorSet = newValidatorSet[:v.maxValidatorSetSize]
	}

	/* START ALPHA CHANGE 279 -- reject empty authoritative INNBC registry validator set -- */
	if len(newValidatorSet) == 0 {
		return nil, errNoActiveRegistryValidators
	}
	/* FINISH ALPHA CHANGE 279 -- reject empty authoritative INNBC registry validator set -- */

	return newValidatorSet, nil
}

/* FINISH ALPHA CHANGE 139 -- add INNBC-native validator registry stake manager -- */
