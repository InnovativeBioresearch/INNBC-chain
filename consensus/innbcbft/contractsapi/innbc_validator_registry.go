package contractsapi

import (
	"math/big"

	"github.com/innovativebioresearch/innbc-chain/types"
)

/* START ALPHA CHANGE 100 -- add manual INNBC validator registry initialize binding -- */
type InitializeINNBCValidatorRegistryFn struct {
	GenesisValidator       types.Address `abi:"genesisValidator_"`
	MinimumStake           *big.Int      `abi:"minimumStake_"`
	ProposalDurationPreset uint8         `abi:"proposalDurationPreset_"`
	/* START ALPHA CHANGE 271 -- pass validator governance address into registry initialization -- */
	ValidatorGovernance types.Address `abi:"validatorGovernance_"`
	/* FINISH ALPHA CHANGE 271 -- pass validator governance address into registry initialization -- */
	/* START ALPHA CHANGE 291 -- pass native staking token and activation epoch into registry initialization -- */
	NativeToken     types.Address `abi:"nativeToken_"`
	ActivationEpoch *big.Int      `abi:"activationEpoch_"`
	/* FINISH ALPHA CHANGE 291 -- pass native staking token and activation epoch into registry initialization -- */
}

func (i *InitializeINNBCValidatorRegistryFn) Sig() []byte {
	return INNBCValidatorRegistry.Abi.Methods["initialize"].ID()
}

func (i *InitializeINNBCValidatorRegistryFn) EncodeAbi() ([]byte, error) {
	return INNBCValidatorRegistry.Abi.Methods["initialize"].Encode(i)
}

func (i *InitializeINNBCValidatorRegistryFn) DecodeAbi(buf []byte) error {
	return decodeMethod(INNBCValidatorRegistry.Abi.Methods["initialize"], buf, i)
}

/* FINISH ALPHA CHANGE 100 -- add manual INNBC validator registry initialize binding -- */

/* START ALPHA CHANGE 137 -- add INNBC validator registry record DTO -- */
type INNBCValidatorRegistryValidator struct {
	Operator      types.Address `abi:"operator"`
	RewardAddress types.Address `abi:"rewardAddress"`
	BlsKey        []byte        `abi:"blsKey"`
	Libp2pPeerID  string        `abi:"libp2pPeerID"`
	Stake         *big.Int      `abi:"stake"`
	Registered    bool          `abi:"registered"`
	Whitelisted   bool          `abi:"whitelisted"`
	/* START ALPHA CHANGE 2033 -- expose the packed registry possession verification flag -- */
	BlsProofVerified bool `abi:"blsProofVerified"`
	/* FINISH ALPHA CHANGE 2033 -- expose the packed registry possession verification flag -- */
}

/* FINISH ALPHA CHANGE 137 -- add INNBC validator registry record DTO -- */
