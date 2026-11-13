/* START ALPHA CHANGE 3116 -- calculate memory expansion charges without integer truncation -- */
package evm

import "math/bits"

// memoryWordGasCost calculates 3*words + floor(words*words/512).
// It performs no allocation and preserves the complete multiplication result.
func memoryWordGasCost(words uint64) (uint64, error) {
	hi, lo := bits.Mul64(words, words)
	if hi >= 512 {
		return 0, errGasUintOverflow
	}

	// Divide the two-word product by 512 using a checked right shift.
	// The hi < 512 check establishes that the quotient fits uint64.
	quadratic := (hi << 55) | (lo >> 9)

	linearHi, linear := bits.Mul64(words, 3)
	if linearHi != 0 {
		return 0, errGasUintOverflow
	}

	total, carry := bits.Add64(linear, quadratic, 0)
	if carry != 0 {
		return 0, errGasUintOverflow
	}

	return total, nil
}

// memoryExpansionSizeAndCost rounds the requested size to complete words,
// checks the platform allocation range, and calculates its total gas charge.
// It never allocates the requested memory.
func memoryExpansionSizeAndCost(size uint64) (int, uint64, error) {
	rounded, carry := bits.Add64(size, 31, 0)
	if carry != 0 {
		return 0, 0, errGasUintOverflow
	}
	rounded &^= 31

	if rounded > uint64(^uint(0)>>1) {
		return 0, 0, errGasUintOverflow
	}

	cost, err := memoryWordGasCost(rounded / 32)
	if err != nil {
		return 0, 0, err
	}

	return int(rounded), cost, nil
}

/* FINISH ALPHA CHANGE 3116 -- calculate memory expansion charges without integer truncation -- */
