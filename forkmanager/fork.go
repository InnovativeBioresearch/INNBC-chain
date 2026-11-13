package forkmanager

/* START ALPHA CHANGE 3317 -- reject retired clock settings when decoding fork parameters -- */
import (
	"encoding/json"

	"github.com/innovativebioresearch/innbc-chain/helper/common"
)

/* FINISH ALPHA CHANGE 3317 -- reject retired clock settings when decoding fork parameters -- */

const InitialFork = "initialfork"

// HandlerDesc gives description for the handler eq: "extra", "proposer_calculator", etc
type HandlerDesc string

// HandlerContainer keeps id of a handler and actual handler
type HandlerContainer struct {
	// ID represents an auto-incremented identifier for a handler
	ID uint
	// Handler represents an actual event handler
	Handler interface{}
}

// Fork structure defines one fork
type Fork struct {
	// Name is name of the fork
	Name string
	// FromBlockNumber indicates the block from which fork becomes enabled
	FromBlockNumber uint64
	// Params are fork consensus parameters
	Params *ForkParams
	// IsActive is false if fork is registered but not activated
	IsActive bool
	// Handlers is a map of all handlers registered for this fork
	Handlers map[HandlerDesc]HandlerContainer
}

// ForkParams hard-coded fork params
type ForkParams struct {
	// MaxValidatorSetSize indicates the maximum size of validator set
	MaxValidatorSetSize *uint64 `json:"maxValidatorSetSize,omitempty"`

	// EpochSize is size of epoch
	EpochSize *uint64 `json:"epochSize,omitempty"`

	// SprintSize is size of sprint
	SprintSize *uint64 `json:"sprintSize,omitempty"`

	// BlockTime is target frequency of blocks production
	BlockTime *common.Duration `json:"blockTime,omitempty"`

	/* START ALPHA CHANGE 3317 -- remove retired drift from fork parameters -- */
	/* FINISH ALPHA CHANGE 3317 -- remove retired drift from fork parameters -- */
}

/* START ALPHA CHANGE 3317 -- validate explicit fork clock settings without changing inheritance -- */
func (fp *ForkParams) UnmarshalJSON(raw []byte) error {
	if err := common.RejectRetiredBlockTimeDrift(raw); err != nil {
		return err
	}

	type plainForkParams ForkParams
	decoded := plainForkParams(*fp)
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}

	if decoded.BlockTime != nil {
		if _, err := common.BlockTimeSeconds(decoded.BlockTime.Duration); err != nil {
			return err
		}
	}

	*fp = ForkParams(decoded)

	return nil
}

/* FINISH ALPHA CHANGE 3317 -- validate explicit fork clock settings without changing inheritance -- */

// Copy creates a deep copy of ForkParams
func (fp *ForkParams) Copy() *ForkParams {
	maxValSetSize := *fp.MaxValidatorSetSize
	epochSize := *fp.EpochSize
	sprintSize := *fp.SprintSize
	/* START ALPHA CHANGE 3317 -- copy the retained block interval -- */
	blockTime := *fp.BlockTime
	/* FINISH ALPHA CHANGE 3317 -- copy the retained block interval -- */

	return &ForkParams{
		MaxValidatorSetSize: &maxValSetSize,
		EpochSize:           &epochSize,
		SprintSize:          &sprintSize,
		/* START ALPHA CHANGE 3317 -- omit retired drift from copied parameters -- */
		BlockTime: &blockTime,
		/* FINISH ALPHA CHANGE 3317 -- omit retired drift from copied parameters -- */
	}
}

// forkHandler defines one custom handler
type forkHandler struct {
	// id - if two handlers start from the same block number, the one with the greater ID should take precedence.
	id uint
	// fromBlockNumber defines block number after handler should be active
	fromBlockNumber uint64
	// handler represents an actual event handler - instance of some structure, function etc
	handler interface{}
}

// forkParamsBlock encapsulates block and actual fork params
type forkParamsBlock struct {
	// fromBlockNumber defines block number after params should be active
	fromBlockNumber uint64
	// params is a pointer to fork params
	params *ForkParams
}
