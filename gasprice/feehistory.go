package gasprice

import (
	"encoding/binary"
	"errors"

	/* START ALPHA CHANGE 1941 -- report invalid fee-history receipt data -- */
	"fmt"
	/* FINISH ALPHA CHANGE 1941 -- report invalid fee-history receipt data -- */
	"math"
	"math/big"
	"sort"

	/* START ALPHA CHANGE 1941 -- retain the requested newest header for its child base fee -- */
	"github.com/innovativebioresearch/innbc-chain/types"
	/* FINISH ALPHA CHANGE 1941 -- retain the requested newest header for its child base fee -- */)

/* START ALPHA CHANGE 2448 -- define configurable fee-history defaults -- */
const (
	DefaultFeeHistoryBlockLimit      uint64 = 256
	DefaultFeeHistoryPercentileLimit uint64 = 10
	DefaultFeeHistoryResultLimit     uint64 = 2560

	MaxFeeHistoryPercentiles int = 100
)

/* FINISH ALPHA CHANGE 2448 -- define configurable fee-history defaults -- */

/* START ALPHA CHANGE 2437 -- define fixed fee-history resource limits -- */

var (
	ErrInvalidPercentile        = errors.New("invalid percentile")
	ErrBlockCount               = errors.New("blockCount must be greater than 0")
	ErrBlockNotFound            = errors.New("could not find block")
	ErrTooManyRewardPercentiles = errors.New("fee-history percentile limit exceeded")
	ErrFeeHistoryResultTooLarge = errors.New("fee-history result limit exceeded")
)

/* FINISH ALPHA CHANGE 2437 -- define fixed fee-history resource limits -- */

type cacheKey struct {
	number      uint64
	percentiles string
}

/* START ALPHA CHANGE 2372 -- preserve full-width cached fee-history rewards -- */
type processedFees struct {
	reward       []*big.Int
	baseFee      uint64
	gasUsedRatio float64
}

/* FINISH ALPHA CHANGE 2372 -- preserve full-width cached fee-history rewards -- */

type txGasAndReward struct {
	/* START ALPHA CHANGE 1942 -- weight reward percentiles in gas units -- */
	gasUsed uint64
	/* FINISH ALPHA CHANGE 1942 -- weight reward percentiles in gas units -- */
	reward *big.Int
}

/* START ALPHA CHANGE 2373 -- return arbitrary-precision rewards with independent ownership -- */
type FeeHistoryReturn struct {
	OldestBlock   uint64
	BaseFeePerGas []uint64
	GasUsedRatio  []float64
	Reward        [][]*big.Int
}

func cloneFeeHistoryRewards(values []*big.Int) []*big.Int {
	if values == nil {
		return nil
	}

	result := make([]*big.Int, len(values))
	for i, value := range values {
		result[i] = new(big.Int).Set(value)
	}

	return result
}

/* FINISH ALPHA CHANGE 2373 -- return arbitrary-precision rewards with independent ownership -- */

func (g *GasHelper) FeeHistory(blockCount uint64, newestBlock uint64, rewardPercentiles []float64) (
	*FeeHistoryReturn, error) {
	if blockCount < 1 {
		return &FeeHistoryReturn{0, nil, nil, nil}, ErrBlockCount
	}

	/* START ALPHA CHANGE 2438 -- reject excessive or invalid percentiles before backend access -- */
	/* START ALPHA CHANGE 2449 -- enforce the configured percentile count -- */
	if uint64(len(rewardPercentiles)) > g.feeHistoryPercentileLimit {
		return &FeeHistoryReturn{0, nil, nil, nil}, fmt.Errorf(
			"%w: maximum %d, received %d",
			ErrTooManyRewardPercentiles,
			g.feeHistoryPercentileLimit,
			len(rewardPercentiles),
		)
	}
	/* FINISH ALPHA CHANGE 2449 -- enforce the configured percentile count -- */

	for i, percentile := range rewardPercentiles {
		if math.IsNaN(percentile) || math.IsInf(percentile, 0) ||
			percentile < 0 || percentile > 100 {
			return &FeeHistoryReturn{0, nil, nil, nil}, ErrInvalidPercentile
		}

		if i > 0 && percentile < rewardPercentiles[i-1] {
			return &FeeHistoryReturn{0, nil, nil, nil}, ErrInvalidPercentile
		}
	}

	head := g.backend.Header()
	if head == nil {
		return &FeeHistoryReturn{0, nil, nil, nil}, ErrBlockNotFound
	}
	if newestBlock > head.Number {
		newestBlock = head.Number
	}
	/* FINISH ALPHA CHANGE 2438 -- reject excessive or invalid percentiles before backend access -- */

	/* START ALPHA CHANGE 2439 -- clamp historical reads and bound result allocation -- */
	/* START ALPHA CHANGE 2449 -- apply the resolved block limit -- */
	if blockCount > g.feeHistoryBlockLimit {
		blockCount = g.feeHistoryBlockLimit
	}

	/*
		Include genesis without overflowing at the maximum block number.
		Configuration normalization guarantees a nonzero block limit.
	*/
	/* FINISH ALPHA CHANGE 2449 -- apply the resolved block limit -- */
	availableBlockCount := newestBlock
	if availableBlockCount < math.MaxUint64 {
		availableBlockCount++
	}

	if blockCount > availableBlockCount {
		blockCount = availableBlockCount
	}

	/*
		Use division instead of multiplying request-controlled values.
		Requests without rewards still consume one unit per block.
	*/
	width := uint64(max(1, len(rewardPercentiles)))
	/* START ALPHA CHANGE 2449 -- enforce the configured result budget -- */
	if blockCount > g.feeHistoryResultLimit/width {
		return &FeeHistoryReturn{0, nil, nil, nil}, fmt.Errorf(
			"%w: maximum %d units, effective blocks %d, width %d",
			ErrFeeHistoryResultTooLarge,
			g.feeHistoryResultLimit,
			blockCount,
			width,
		)
	}
	/* FINISH ALPHA CHANGE 2449 -- enforce the configured result budget -- */
	/* FINISH ALPHA CHANGE 2439 -- clamp historical reads and bound result allocation -- */

	var (
		/* START ALPHA CHANGE 1943 -- retain genesis as a valid newest block -- */
		oldestBlock = newestBlock - (blockCount - 1)
		/* FINISH ALPHA CHANGE 1943 -- retain genesis as a valid newest block -- */
		/* START ALPHA CHANGE 2374 -- allocate arbitrary-precision reward rows -- */
		baseFeePerGas = make([]uint64, blockCount+1)
		gasUsedRatio  = make([]float64, blockCount)
		reward        = make([][]*big.Int, blockCount)
		/* FINISH ALPHA CHANGE 2374 -- allocate arbitrary-precision reward rows -- */
		/* START ALPHA CHANGE 1944 -- calculate the child fee from the requested newest block -- */
		newestHeader *types.Header
		/* FINISH ALPHA CHANGE 1944 -- calculate the child fee from the requested newest block -- */
	)

	percentileKey := make([]byte, 8*len(rewardPercentiles))
	for i, p := range rewardPercentiles {
		binary.LittleEndian.PutUint64(percentileKey[i*8:(i+1)*8], math.Float64bits(p))
	}

	/* START ALPHA CHANGE 2440 -- iterate within the bounded range without block-number wraparound -- */
	for offset := uint64(0); offset < blockCount; offset++ {
		i := oldestBlock + offset
		/* FINISH ALPHA CHANGE 2440 -- iterate within the bounded range without block-number wraparound -- */

		cacheKey := cacheKey{number: i, percentiles: string(percentileKey)}
		//cache is hit, load from cache and continue to next block
		if p, ok := g.historyCache.Get(cacheKey); ok {
			processedFee, isOk := p.(*processedFees)
			if !isOk {
				return &FeeHistoryReturn{0, nil, nil, nil}, errors.New("could not convert catched processed fee")
			}

			/* START ALPHA CHANGE 2375 -- detach returned rewards from cache storage -- */
			baseFeePerGas[i-oldestBlock] = processedFee.baseFee
			gasUsedRatio[i-oldestBlock] = processedFee.gasUsedRatio
			reward[i-oldestBlock] = cloneFeeHistoryRewards(processedFee.reward)
			/* FINISH ALPHA CHANGE 2375 -- detach returned rewards from cache storage -- */

			continue
		}

		/* START ALPHA CHANGE 1945 -- avoid reading block bodies when rewards are not requested -- */
		block, ok := g.backend.GetBlockByNumber(i, len(rewardPercentiles) != 0)
		if !ok || block == nil || block.Header == nil {
			return &FeeHistoryReturn{0, nil, nil, nil}, ErrBlockNotFound
		}

		if i == newestBlock {
			newestHeader = block.Header
		}
		/* FINISH ALPHA CHANGE 1945 -- avoid reading block bodies when rewards are not requested -- */

		baseFeePerGas[i-oldestBlock] = block.Header.BaseFee

		/* START ALPHA CHANGE 1077 -- avoid non-finite feeHistory gas ratio for zero gas limit -- */
		if block.Header.GasLimit == 0 {
			gasUsedRatio[i-oldestBlock] = 0
		} else {
			gasUsedRatio[i-oldestBlock] = float64(block.Header.GasUsed) / float64(block.Header.GasLimit)
		}
		/* FINISH ALPHA CHANGE 1077 -- avoid non-finite feeHistory gas ratio for zero gas limit -- */

		if len(rewardPercentiles) == 0 {
			//reward percentiles not requested, skip rest of this loop
			continue
		}

		/* START ALPHA CHANGE 2376 -- calculate full-width rewards from ordinary transactions only -- */
		row := make([]*big.Int, len(rewardPercentiles))
		for j := range row {
			row[j] = new(big.Int)
		}
		reward[i-oldestBlock] = row

		if len(block.Transactions) == 0 {
			continue
		}

		receipts, err := g.backend.GetReceiptsByHash(block.Hash())
		if err != nil {
			return &FeeHistoryReturn{0, nil, nil, nil},
				fmt.Errorf("failed to read receipts for block %d: %w", i, err)
		}
		if len(receipts) != len(block.Transactions) {
			return &FeeHistoryReturn{0, nil, nil, nil},
				fmt.Errorf("receipt count mismatch for block %d: have %d, want %d",
					i, len(receipts), len(block.Transactions))
		}

		sorter := make([]*txGasAndReward, 0, len(block.Transactions))
		baseFee := new(big.Int).SetUint64(block.Header.BaseFee)
		var eligibleGasUsed uint64

		for j, tx := range block.Transactions {
			if tx == nil || receipts[j] == nil {
				return &FeeHistoryReturn{0, nil, nil, nil},
					fmt.Errorf("missing transaction or receipt at index %d of block %d", j, i)
			}

			if tx.Type == types.StateTx {
				continue
			}

			tip := tx.EffectiveGasTip(baseFee)
			if tip.Sign() < 0 {
				return &FeeHistoryReturn{0, nil, nil, nil},
					fmt.Errorf("negative ordinary transaction tip at index %d of block %d", j, i)
			}

			gasUsed := receipts[j].GasUsed
			if gasUsed > math.MaxUint64-eligibleGasUsed {
				return &FeeHistoryReturn{0, nil, nil, nil},
					fmt.Errorf("reward gas total overflows at block %d", i)
			}
			eligibleGasUsed += gasUsed

			sorter = append(sorter, &txGasAndReward{
				gasUsed: gasUsed,
				reward:  new(big.Int).Set(tip),
			})
		}

		if len(sorter) != 0 {
			sort.Slice(sorter, func(i, j int) bool {
				return sorter[i].reward.Cmp(sorter[j].reward) < 0
			})

			txIndex := 0
			sumGasUsed := sorter[0].gasUsed

			for c, percentile := range rewardPercentiles {
				thresholdGasUsed := eligibleGasUsed
				threshold := float64(eligibleGasUsed) * percentile / 100

				/*
					Keep the upper endpoint in integer form. Float rounding
					must not convert the total beyond the uint64 range.
				*/
				if threshold < float64(eligibleGasUsed) {
					thresholdGasUsed = uint64(threshold)
				}

				for sumGasUsed < thresholdGasUsed && txIndex < len(sorter)-1 {
					txIndex++
					sumGasUsed += sorter[txIndex].gasUsed
				}

				row[c].Set(sorter[txIndex].reward)
			}
		}

		blockFees := &processedFees{
			reward:       cloneFeeHistoryRewards(row),
			baseFee:      block.Header.BaseFee,
			gasUsedRatio: gasUsedRatio[i-oldestBlock],
		}
		g.historyCache.Add(cacheKey, blockFees)
		/* FINISH ALPHA CHANGE 2376 -- calculate full-width rewards from ordinary transactions only -- */
	}

	/* START ALPHA CHANGE 1948 -- return the requested newest block's child base fee -- */
	// A cached newest block has no retained header, so read that one header only.
	if newestHeader == nil {
		block, ok := g.backend.GetBlockByNumber(newestBlock, false)
		if !ok || block == nil || block.Header == nil {
			return &FeeHistoryReturn{0, nil, nil, nil}, ErrBlockNotFound
		}
		newestHeader = block.Header
	}

	nextBaseFee, err := g.backend.CalculateBaseFee(newestHeader)
	if err != nil {
		return &FeeHistoryReturn{0, nil, nil, nil},
			fmt.Errorf("failed to calculate base fee after block %d: %w", newestBlock, err)
	}
	baseFeePerGas[blockCount] = nextBaseFee
	/* FINISH ALPHA CHANGE 1948 -- return the requested newest block's child base fee -- */

	return &FeeHistoryReturn{oldestBlock, baseFeePerGas, gasUsedRatio, reward}, nil
}
