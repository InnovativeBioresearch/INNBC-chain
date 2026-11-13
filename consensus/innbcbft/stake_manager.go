package innbcbft

import (
	"encoding/json"
	"errors"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
	/* START ALPHA CHANGE 374 -- remove legacy event subscriber imports from stake manager -- */
	/* FINISH ALPHA CHANGE 374 -- remove legacy event subscriber imports from stake manager -- */
)

/* START ALPHA CHANGE 278 -- add terminal error for empty INNBC registry validator set -- */
var errNoActiveRegistryValidators = errors.New("INNBC validator registry has no active validators; consensus cannot continue")

/* FINISH ALPHA CHANGE 278 -- add terminal error for empty INNBC registry validator set -- */

/* START ALPHA CHANGE 141 -- prune legacy rootchain stake manager implementation -- */
// StakeManager interface provides functions for handling validator-set updates.
type StakeManager interface {
	/* START ALPHA CHANGE 375 -- remove legacy event subscriber requirement from stake manager -- */
	PostBlock(req *PostBlockRequest) error
	UpdateValidatorSet(epoch uint64, currentValidatorSet validator.AccountSet) (*validator.ValidatorSetDelta, error)
	/* FINISH ALPHA CHANGE 375 -- remove legacy event subscriber requirement from stake manager -- */
}

var _ StakeManager = (*dummyStakeManager)(nil)

// dummyStakeManager is a dummy implementation of StakeManager interface
// used only for unit testing.
type dummyStakeManager struct{}

func (d *dummyStakeManager) PostBlock(req *PostBlockRequest) error { return nil }

func (d *dummyStakeManager) UpdateValidatorSet(
	epoch uint64,
	currentValidatorSet validator.AccountSet,
) (*validator.ValidatorSetDelta, error) {
	return &validator.ValidatorSetDelta{}, nil
}

/* START ALPHA CHANGE 376 -- remove legacy event subscriber methods from dummy stake manager -- */
/* FINISH ALPHA CHANGE 376 -- remove legacy event subscriber methods from dummy stake manager -- */

type validatorSetState struct {
	BlockNumber          uint64            `json:"block"`
	EpochID              uint64            `json:"epoch"`
	UpdatedAtBlockNumber uint64            `json:"updated_at_block"`
	Validators           validatorStakeMap `json:"validators"`
}

func (vs validatorSetState) Marshal() ([]byte, error) {
	return json.Marshal(vs)
}

func (vs *validatorSetState) Unmarshal(b []byte) error {
	return json.Unmarshal(b, vs)
}

// validatorStakeMap holds ValidatorMetadata for each validator address.
type validatorStakeMap map[types.Address]*validator.ValidatorMetadata

// newValidatorStakeMap returns a new instance of validatorStakeMap.
func newValidatorStakeMap(validatorSet validator.AccountSet) validatorStakeMap {
	stakeMap := make(validatorStakeMap, len(validatorSet))

	for _, v := range validatorSet {
		stakeMap[v.Address] = v.Copy()
	}

	return stakeMap
}

/* FINISH ALPHA CHANGE 141 -- prune legacy rootchain stake manager implementation -- */
