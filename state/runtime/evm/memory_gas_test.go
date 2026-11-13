/* START ALPHA CHANGE 3118 -- test exact memory charges and rejection before enormous allocations -- */
package evm

import (
	"fmt"
	"math/big"
	"math/bits"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/stretchr/testify/require"
)

// Independent reference arithmetic belongs only in tests.
func memoryExpansionReferenceCost(words uint64) *big.Int {
	w := new(big.Int).SetUint64(words)
	quadratic := new(big.Int).Mul(w, w)
	quadratic.Div(quadratic, big.NewInt(512))

	linear := new(big.Int).Mul(w, big.NewInt(3))
	return quadratic.Add(quadratic, linear)
}

func TestMemoryExpansion_WordCostReference(t *testing.T) {
	// Find the largest word count whose complete charge fits uint64.
	// This also exercises the boundary where the quadratic component fits
	// but adding the linear component would overflow.
	lower, upper := uint64(0), uint64(1)<<37
	for lower+1 < upper {
		middle := lower + (upper-lower)/2
		if memoryExpansionReferenceCost(middle).IsUint64() {
			lower = middle
		} else {
			upper = middle
		}
	}

	quadraticBoundary := new(big.Int).Mul(
		new(big.Int).SetUint64(^uint64(0)),
		big.NewInt(512),
	)
	quadraticBoundary.Sqrt(quadraticBoundary)
	q := quadraticBoundary.Uint64()

	cases := []uint64{
		0, 1, 2, 22, 23, 512, 4096,
		(uint64(1) << 32) - 1,
		uint64(1) << 32,
		(uint64(1) << 32) + 1,
		uint64(1) << 36,
		lower - 1, lower, lower + 1,
		q - 1, q, q + 1,
		uint64(1) << 37,
		^uint64(0) / 3,
		^uint64(0),
	}

	for index, words := range cases {
		t.Run(fmt.Sprintf("%d_%d", index, words), func(t *testing.T) {
			want := memoryExpansionReferenceCost(words)
			got, err := memoryWordGasCost(words)

			if !want.IsUint64() {
				require.ErrorIs(t, err, errGasUintOverflow)
				require.Zero(t, got)
				return
			}

			require.NoError(t, err)
			require.Equal(t, want.Uint64(), got)
		})
	}
}

func TestMemoryExpansion_SizeAndCostBoundaries(t *testing.T) {
	maxInt := uint64(^uint(0) >> 1)
	cases := []uint64{
		0, 1, 31, 32, 33, 63, 64, 65, 1024,
		((uint64(1) << 32) - 1) * 32,
		uint64(1) << 37,
		((uint64(1) << 32) + 1) * 32,
		maxInt - 31,
		maxInt,
		^uint64(0) - 31,
		^uint64(0) - 30,
		^uint64(0),
	}

	for index, size := range cases {
		t.Run(fmt.Sprintf("%d_%d", index, size), func(t *testing.T) {
			rounded := new(big.Int).SetUint64(size)
			rounded.Add(rounded, big.NewInt(31))
			rounded.Div(rounded, big.NewInt(32))
			rounded.Mul(rounded, big.NewInt(32))

			words := new(big.Int).Div(
				new(big.Int).Set(rounded), big.NewInt(32),
			)
			wantCost := memoryExpansionReferenceCost(words.Uint64())

			gotSize, gotCost, err := memoryExpansionSizeAndCost(size)
			if rounded.Cmp(new(big.Int).SetUint64(maxInt)) > 0 ||
				!wantCost.IsUint64() {
				require.ErrorIs(t, err, errGasUintOverflow)
				require.Zero(t, gotSize)
				require.Zero(t, gotCost)
				return
			}

			require.NoError(t, err)
			require.Equal(t, int(rounded.Uint64()), gotSize)
			require.Equal(t, wantCost.Uint64(), gotCost)
		})
	}
}

func TestMemoryExpansion_IncrementalCharges(t *testing.T) {
	const initialGas = uint64(100_000)

	incremental := &state{gas: initialGas}
	for _, size := range []int64{1, 31, 32, 33, 63, 64, 65, 1024} {
		require.True(t, incremental.allocateMemory(
			big.NewInt(0), big.NewInt(size),
		))
		require.NoError(t, incremental.err)
	}

	single := &state{gas: initialGas}
	require.True(t, single.allocateMemory(big.NewInt(0), big.NewInt(1024)))

	require.Equal(t, single.memory, incremental.memory)
	require.Equal(t, single.gas, incremental.gas)
	require.Equal(t, single.lastGasCost, incremental.lastGasCost)
	require.Equal(t, memoryExpansionReferenceCost(32).Uint64(),
		initialGas-incremental.gas)

	memoryBefore := append([]byte(nil), incremental.memory...)
	gasBefore := incremental.gas
	costBefore := incremental.lastGasCost

	// Accessing already allocated memory must charge nothing.
	require.True(t, incremental.allocateMemory(big.NewInt(31), big.NewInt(1)))
	require.Equal(t, memoryBefore, incremental.memory)
	require.Equal(t, gasBefore, incremental.gas)
	require.Equal(t, costBefore, incremental.lastGasCost)
}

func TestMemoryExpansion_FailureAndOperandBehavior(t *testing.T) {
	enormousError := errOutOfGas
	if bits.UintSize == 32 {
		enormousError = errGasUintOverflow
	}

	cases := []struct {
		name     string
		offset   *big.Int
		size     *big.Int
		gas      uint64
		lastCost uint64
		wantErr  error
	}{
		{
			"insufficient_gas", big.NewInt(0), big.NewInt(33),
			2, 3, errOutOfGas,
		},
		{
			"nonmonotonic_cost", big.NewInt(0), big.NewInt(33),
			100, 7, errGasUintOverflow,
		},
		{
			"enormous_expansion", big.NewInt(0),
			new(big.Int).SetUint64(uint64(1) << 37),
			13_000_000_000, 3, enormousError,
		},
		{
			"negative_offset", big.NewInt(-1), big.NewInt(1),
			100, 3, errReturnDataOutOfBounds,
		},
		{
			"negative_size", big.NewInt(0), big.NewInt(-1),
			100, 3, errReturnDataOutOfBounds,
		},
		{
			"offset_above_existing_limit",
			new(big.Int).SetUint64(0xffffffffe1), big.NewInt(1),
			100, 3, errReturnDataOutOfBounds,
		},
		{
			"size_above_existing_limit", big.NewInt(0),
			new(big.Int).SetUint64(0xffffffffe1),
			100, 3, errReturnDataOutOfBounds,
		},
		{
			"zero_length_with_uint64_offset",
			new(big.Int).SetUint64(^uint64(0)), big.NewInt(0),
			100, 3, nil,
		},
		/* START ALPHA CHANGE 3578 -- expect success when a zero-sized range leaves a large offset unused -- */
		{
			"zero_length_with_non_uint64_offset",
			new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(0),
			100, 3, nil,
		},
		/* FINISH ALPHA CHANGE 3578 -- expect success when a zero-sized range leaves a large offset unused -- */
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c := &state{
				memory:      make([]byte, 32),
				gas:         tt.gas,
				lastGasCost: tt.lastCost,
			}
			c.memory[0] = 0xab
			before := append([]byte(nil), c.memory...)

			ok := c.allocateMemory(tt.offset, tt.size)
			if tt.wantErr != nil {
				require.False(t, ok)
				require.ErrorIs(t, c.err, tt.wantErr)
				require.True(t, c.stop)
			} else {
				require.True(t, ok)
				require.NoError(t, c.err)
				require.False(t, c.stop)
			}

			require.Equal(t, before, c.memory)
			require.Equal(t, tt.gas, c.gas)
			require.Equal(t, tt.lastCost, c.lastGasCost)
		})
	}
}

func TestMemoryExpansion_MSTORE8Rejection(t *testing.T) {
	const gas = uint64(13_000_000_000)

	wantErr := errOutOfGas
	if bits.UintSize == 32 {
		wantErr = errGasUintOverflow
	}

	c := &state{
		memory:      make([]byte, 32),
		gas:         gas,
		lastGasCost: 3,
	}
	c.memory[0] = 0x55
	before := append([]byte(nil), c.memory...)

	c.push(big.NewInt(0xab))
	c.push(new(big.Int).SetUint64((uint64(1) << 37) - 1))
	opMStore8(c)

	require.ErrorIs(t, c.err, wantErr)
	require.True(t, c.stop)
	require.Equal(t, before, c.memory)
	require.Equal(t, gas, c.gas)
	require.Equal(t, uint64(3), c.lastGasCost)

	// PUSH1 value; PUSH5 (2^37-1); MSTORE8; STOP.
	code := []byte{
		0x60, 0xab,
		0x64, 0x1f, 0xff, 0xff, 0xff, 0xff,
		0x53,
		0x00,
	}
	result := NewEVM().Run(
		newMockContract(big.NewInt(0), gas, code),
		&mockHost{},
		&chain.ForksInTime{},
	)

	require.ErrorIs(t, result.Err, wantErr)
	require.Zero(t, result.GasLeft)
	require.Equal(t, gas, result.GasUsed)
	require.Empty(t, result.ReturnValue)
}

/* FINISH ALPHA CHANGE 3118 -- test exact memory charges and rejection before enormous allocations -- */
