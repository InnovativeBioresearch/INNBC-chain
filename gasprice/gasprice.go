package gasprice

import (
	/* START ALPHA CHANGE 2450 -- report invalid fee-history configuration -- */
	"fmt"
	/* FINISH ALPHA CHANGE 2450 -- report invalid fee-history configuration -- */
	"math/big"

	lru "github.com/hashicorp/golang-lru"
	"github.com/innovativebioresearch/innbc-chain/types"
)

/* START ALPHA CHANGE 2450 -- normalize configurable fee-history budgets -- */
var DefaultGasHelperConfig = &Config{
	FeeHistoryBlockLimit:      DefaultFeeHistoryBlockLimit,
	FeeHistoryPercentileLimit: DefaultFeeHistoryPercentileLimit,
	FeeHistoryResultLimit:     DefaultFeeHistoryResultLimit,
}

type Config struct {
	FeeHistoryBlockLimit      uint64
	FeeHistoryPercentileLimit uint64
	FeeHistoryResultLimit     uint64
}

/*
ResolveFeeHistoryConfig returns an independent normalized configuration.
Zero selects a code default; it never disables a limit.
*/
func ResolveFeeHistoryConfig(config *Config) (Config, error) {
	var resolved Config
	if config != nil {
		resolved = *config
	}

	if resolved.FeeHistoryBlockLimit == 0 {
		resolved.FeeHistoryBlockLimit = DefaultFeeHistoryBlockLimit
	}
	if resolved.FeeHistoryPercentileLimit == 0 {
		resolved.FeeHistoryPercentileLimit = DefaultFeeHistoryPercentileLimit
	}
	if resolved.FeeHistoryResultLimit == 0 {
		resolved.FeeHistoryResultLimit = DefaultFeeHistoryResultLimit
	}

	if resolved.FeeHistoryPercentileLimit > uint64(MaxFeeHistoryPercentiles) {
		return Config{}, fmt.Errorf(
			"fee-history percentile limit must be between 1 and %d, or 0 for default",
			MaxFeeHistoryPercentiles,
		)
	}

	/*
		Accepted requests allocate blockCount+1 base-fee elements.
		Keep the result budget representable as an int with room for
		that extra element. This is a representation bound, not a
		promise that every operator-selected budget is inexpensive.
	*/
	maxResultLimit := uint64(^uint(0)>>1) - 1
	if resolved.FeeHistoryResultLimit > maxResultLimit {
		return Config{}, fmt.Errorf(
			"fee-history result limit must not exceed %d",
			maxResultLimit,
		)
	}

	return resolved, nil
}

/* FINISH ALPHA CHANGE 2450 -- normalize configurable fee-history budgets -- */

// Blockchain is the interface representing blockchain
type Blockchain interface {
	GetBlockByNumber(number uint64, full bool) (*types.Block, bool)
	/* START ALPHA CHANGE 1940 -- use persisted receipt gas and canonical next-base-fee calculation -- */
	GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error)
	CalculateBaseFee(parent *types.Header) (uint64, error)
	/* FINISH ALPHA CHANGE 1940 -- use persisted receipt gas and canonical next-base-fee calculation -- */
	Header() *types.Header
}

// GasStore interface is providing functions regarding gas and fees
type GasStore interface {
	// MaxPriorityFeePerGas calculates the priority fee needed for transaction to be included in a block
	MaxPriorityFeePerGas() (*big.Int, error)
	// FeeHistory returns the collection of historical gas information
	FeeHistory(uint64, uint64, []float64) (*FeeHistoryReturn, error)
}

var _ GasStore = (*GasHelper)(nil)

// GasHelper struct implements functions from the GasStore interface
type GasHelper struct {
	/* START ALPHA CHANGE 2450 -- retain resolved fee-history budgets -- */
	feeHistoryBlockLimit      uint64
	feeHistoryPercentileLimit uint64
	feeHistoryResultLimit     uint64
	/* FINISH ALPHA CHANGE 2450 -- retain resolved fee-history budgets -- */
	// backend is an abstraction of blockchain
	backend Blockchain

	historyCache *lru.Cache
}

// NewGasHelper is the constructor function for GasHelper struct
/* START ALPHA CHANGE 2450 -- validate budgets for direct helper callers -- */
func NewGasHelper(config *Config, backend Blockchain) (*GasHelper, error) {
	resolved, err := ResolveFeeHistoryConfig(config)
	if err != nil {
		return nil, err
	}

	cache, err := lru.New(100)
	if err != nil {
		return nil, err
	}

	return &GasHelper{
		feeHistoryBlockLimit:      resolved.FeeHistoryBlockLimit,
		feeHistoryPercentileLimit: resolved.FeeHistoryPercentileLimit,
		feeHistoryResultLimit:     resolved.FeeHistoryResultLimit,
		backend:                   backend,
		historyCache:              cache,
	}, nil
}

/* FINISH ALPHA CHANGE 2450 -- validate budgets for direct helper callers -- */

// START ALPHA CHANGE 1211 -- return base fee as priority fee without historical sampling --
// MaxPriorityFeePerGas returns a constant-time priority fee recommendation.
// INNBC uses the current base fee as the default priority fee so fee suggestions
// stay congestion-aware without scanning historical blocks or sampled tips.
func (g *GasHelper) MaxPriorityFeePerGas() (*big.Int, error) {
	currentBaseFee := new(big.Int).SetUint64(g.backend.Header().BaseFee)
	if currentBaseFee.Sign() == 0 {
		return big.NewInt(1), nil
	}

	return currentBaseFee, nil
}

// FINISH ALPHA CHANGE 1211 -- return base fee as priority fee without historical sampling
