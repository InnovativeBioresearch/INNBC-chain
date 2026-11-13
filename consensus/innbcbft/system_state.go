/* START ALPHA CHANGE 385 -- restore INNBC-native system state reader without bridge state-sync logic -- */
package innbcbft

import (
	"fmt"
	"math/big"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
)

type SystemState interface {
	GetEpoch() (uint64, error)
	/* START ALPHA CHANGE 451 -- remove state-sync committed index from system state -- */
	/* FINISH ALPHA CHANGE 451 -- remove state-sync committed index from system state -- */
}

type systemState struct {
	validatorSetAddr types.Address
	provider         contract.Provider
}

/* START ALPHA CHANGE 452 -- remove bridge state receiver parameter from system state constructor -- */
func NewSystemState(
	validatorSetAddr types.Address,
	provider contract.Provider,
) SystemState {
	/* FINISH ALPHA CHANGE 452 -- remove bridge state receiver parameter from system state constructor -- */
	return &systemState{
		validatorSetAddr: validatorSetAddr,
		provider:         provider,
	}
}

func (s *systemState) GetEpoch() (uint64, error) {
	method := contractsapi.ValidatorSet.Abi.GetMethod("currentEpochId")

	input, err := method.Encode([]interface{}{})
	if err != nil {
		return 0, err
	}

	raw, err := s.provider.Call(ethgo.Address(s.validatorSetAddr), input, nil)
	if err != nil {
		return 0, err
	}

	decoded, err := method.Decode(raw)
	if err != nil {
		return 0, err
	}

	for _, value := range decoded {
		epoch, ok := value.(*big.Int)
		if !ok {
			continue
		}

		return epoch.Uint64(), nil
	}

	return 0, fmt.Errorf("currentEpochId returned no uint256 value")
}

/* START ALPHA CHANGE 453 -- remove state-sync committed index reader from system state -- */
/* FINISH ALPHA CHANGE 453 -- remove state-sync committed index reader from system state -- */

/* FINISH ALPHA CHANGE 385 -- restore INNBC-native system state reader without bridge state-sync logic -- */
