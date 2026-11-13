package innbcbft

import (
	"fmt"
	"math/big"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
)

/* START ALPHA CHANGE 138 -- add INNBC validator registry chain-state reader -- */
type ValidatorRegistryState interface {
	IsRegistryActivated() (bool, error)
	GetActivationEpoch() (uint64, error)
	GetActiveValidators() ([]types.Address, error)
	GetValidator(operator types.Address) (*contractsapi.INNBCValidatorRegistryValidator, error)
}

var _ ValidatorRegistryState = (*ValidatorRegistryStateImpl)(nil)

type ValidatorRegistryStateImpl struct {
	registryContract *contract.Contract
}

func NewValidatorRegistryState(registryAddr types.Address, provider contract.Provider) *ValidatorRegistryStateImpl {
	return &ValidatorRegistryStateImpl{
		registryContract: contract.NewContract(
			ethgo.Address(registryAddr),
			contractsapi.INNBCValidatorRegistry.Abi,
			contract.WithProvider(provider),
		),
	}
}

func (s *ValidatorRegistryStateImpl) IsRegistryActivated() (bool, error) {
	rawResult, err := s.registryContract.Call("registryActivated", ethgo.Latest)
	if err != nil {
		return false, err
	}

	return registryBoolResult(rawResult, "registryActivated", "0")
}

func (s *ValidatorRegistryStateImpl) GetActivationEpoch() (uint64, error) {
	rawResult, err := s.registryContract.Call("activationEpoch", ethgo.Latest)
	if err != nil {
		return 0, err
	}

	activationEpoch, err := registryBigIntResult(rawResult, "activationEpoch", "0")
	if err != nil {
		return 0, err
	}

	return activationEpoch.Uint64(), nil
}

func (s *ValidatorRegistryStateImpl) GetActiveValidators() ([]types.Address, error) {
	rawResult, err := s.registryContract.Call("getActiveValidators", ethgo.Latest)
	if err != nil {
		return nil, err
	}

	value, err := registryResultValue(rawResult, "0")
	if err != nil {
		return nil, err
	}

	return registryAddressSliceValue(value)
}

func (s *ValidatorRegistryStateImpl) GetValidator(
	operator types.Address,
) (*contractsapi.INNBCValidatorRegistryValidator, error) {
	rawResult, err := s.registryContract.Call("validators", ethgo.Latest, ethgo.Address(operator))
	if err != nil {
		return nil, err
	}

	operatorAddress, err := registryAddressResult(rawResult, "operator", "0")
	if err != nil {
		return nil, err
	}

	rewardAddress, err := registryAddressResult(rawResult, "rewardAddress", "1")
	if err != nil {
		return nil, err
	}

	blsKey, err := registryBytesResult(rawResult, "blsKey", "2")
	if err != nil {
		return nil, err
	}

	libp2pPeerID, err := registryStringResult(rawResult, "libp2pPeerID", "3")
	if err != nil {
		return nil, err
	}

	stake, err := registryBigIntResult(rawResult, "stake", "4")
	if err != nil {
		return nil, err
	}

	registered, err := registryBoolResult(rawResult, "registered", "5")
	if err != nil {
		return nil, err
	}

	whitelisted, err := registryBoolResult(rawResult, "whitelisted", "6")
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 2034 -- read possession status with the existing validator record call -- */
	proofVerified, err := registryBoolResult(rawResult, "blsProofVerified", "7")
	if err != nil {
		return nil, err
	}

	return &contractsapi.INNBCValidatorRegistryValidator{
		Operator:         operatorAddress,
		RewardAddress:    rewardAddress,
		BlsKey:           blsKey,
		Libp2pPeerID:     libp2pPeerID,
		Stake:            stake,
		Registered:       registered,
		Whitelisted:      whitelisted,
		BlsProofVerified: proofVerified,
	}, nil
	/* FINISH ALPHA CHANGE 2034 -- read possession status with the existing validator record call -- */
}

func registryResultValue(rawResult map[string]any, keys ...string) (any, error) {
	for _, key := range keys {
		if value, ok := rawResult[key]; ok {
			return value, nil
		}
	}

	return nil, fmt.Errorf("failed to decode registry result field %v", keys)
}

func registryBoolResult(rawResult map[string]any, keys ...string) (bool, error) {
	value, err := registryResultValue(rawResult, keys...)
	if err != nil {
		return false, err
	}

	boolValue, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("failed to decode registry bool field %v", keys)
	}

	return boolValue, nil
}

func registryBigIntResult(rawResult map[string]any, keys ...string) (*big.Int, error) {
	value, err := registryResultValue(rawResult, keys...)
	if err != nil {
		return nil, err
	}

	switch typedValue := value.(type) {
	case *big.Int:
		if typedValue == nil {
			return nil, fmt.Errorf("failed to decode nil registry integer field %v", keys)
		}

		return new(big.Int).Set(typedValue), nil
	case big.Int:
		return new(big.Int).Set(&typedValue), nil
	default:
		return nil, fmt.Errorf("failed to decode registry integer field %v", keys)
	}
}

func registryAddressResult(rawResult map[string]any, keys ...string) (types.Address, error) {
	value, err := registryResultValue(rawResult, keys...)
	if err != nil {
		return types.ZeroAddress, err
	}

	return registryAddressValue(value)
}

func registryAddressValue(value any) (types.Address, error) {
	switch typedValue := value.(type) {
	case ethgo.Address:
		return types.Address(typedValue), nil
	case types.Address:
		return typedValue, nil
	case string:
		return types.StringToAddress(typedValue), nil
	default:
		return types.ZeroAddress, fmt.Errorf("failed to decode registry address")
	}
}

func registryBytesResult(rawResult map[string]any, keys ...string) ([]byte, error) {
	value, err := registryResultValue(rawResult, keys...)
	if err != nil {
		return nil, err
	}

	bytesValue, ok := value.([]byte)
	if !ok {
		return nil, fmt.Errorf("failed to decode registry bytes field %v", keys)
	}

	return append([]byte(nil), bytesValue...), nil
}

func registryStringResult(rawResult map[string]any, keys ...string) (string, error) {
	value, err := registryResultValue(rawResult, keys...)
	if err != nil {
		return "", err
	}

	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("failed to decode registry string field %v", keys)
	}

	return stringValue, nil
}

func registryAddressSliceValue(value any) ([]types.Address, error) {
	switch typedValue := value.(type) {
	case []ethgo.Address:
		addresses := make([]types.Address, len(typedValue))
		for i, address := range typedValue {
			addresses[i] = types.Address(address)
		}

		return addresses, nil
	case []types.Address:
		return append([]types.Address(nil), typedValue...), nil
	case []string:
		addresses := make([]types.Address, len(typedValue))
		for i, address := range typedValue {
			addresses[i] = types.StringToAddress(address)
		}

		return addresses, nil
	case []any:
		addresses := make([]types.Address, len(typedValue))
		for i, address := range typedValue {
			decodedAddress, err := registryAddressValue(address)
			if err != nil {
				return nil, err
			}

			addresses[i] = decodedAddress
		}

		return addresses, nil
	default:
		return nil, fmt.Errorf("failed to decode registry active validator list")
	}
}

/* FINISH ALPHA CHANGE 138 -- add INNBC validator registry chain-state reader -- */
