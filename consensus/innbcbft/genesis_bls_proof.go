package innbcbft

import (
	"bytes"
	"fmt"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
)

/* START ALPHA CHANGE 2010 -- validate genesis proofs and bind the verified set to canonical header extra data -- */
func BuildGenesisValidatorSet(config *INNBCBFTConfig) (validator.AccountSet, error) {
	if config == nil {
		return nil, fmt.Errorf("missing INNBCBFT genesis configuration")
	}

	count := uint64(len(config.InitialValidatorSet))
	if config.MinValidatorSetSize == 0 ||
		config.MaxValidatorSetSize < config.MinValidatorSetSize {
		return nil, fmt.Errorf("invalid genesis validator set size bounds")
	}
	if count < config.MinValidatorSetSize || count > config.MaxValidatorSetSize {
		return nil, fmt.Errorf(
			"genesis validator count %d is outside [%d, %d]",
			count, config.MinValidatorSetSize, config.MaxValidatorSetSize,
		)
	}

	accounts := make(validator.AccountSet, 0, len(config.InitialValidatorSet))
	addresses := make(map[types.Address]struct{}, len(config.InitialValidatorSet))
	keys := make(map[string]struct{}, len(config.InitialValidatorSet))

	for index, entry := range config.InitialValidatorSet {
		if entry == nil {
			return nil, fmt.Errorf("nil genesis validator at index %d", index)
		}
		if _, exists := addresses[entry.Address]; exists {
			return nil, fmt.Errorf("duplicate genesis validator address %s", entry.Address)
		}

		metadata, err := entry.ToValidatorMetadata()
		if err != nil {
			return nil, fmt.Errorf("genesis validator %d: %w", index, err)
		}

		key := string(metadata.BlsKey.Marshal())
		if _, exists := keys[key]; exists {
			return nil, fmt.Errorf("duplicate genesis BLS public key at index %d", index)
		}

		addresses[entry.Address] = struct{}{}
		keys[key] = struct{}{}
		accounts = append(accounts, metadata)
	}

	return accounts, nil
}

func BuildGenesisExtraData(accounts validator.AccountSet) []byte {
	extra := &Extra{
		Validators: &validator.ValidatorSetDelta{
			Added: accounts,
		},
		Checkpoint: &CheckpointData{},
	}

	return extra.MarshalRLPTo(nil)
}

func ValidateGenesisExtraData(config *INNBCBFTConfig, extraData []byte) error {
	accounts, err := BuildGenesisValidatorSet(config)
	if err != nil {
		return err
	}

	if !bytes.Equal(extraData, BuildGenesisExtraData(accounts)) {
		return fmt.Errorf(
			"genesis extra data does not match the canonical proved validator set",
		)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2010 -- validate genesis proofs and bind the verified set to canonical header extra data -- */
