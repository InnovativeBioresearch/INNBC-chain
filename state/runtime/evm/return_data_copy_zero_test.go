/* START ALPHA CHANGE 3567 -- test zero-length return-data source bounds and exceptional execution -- */
package evm

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReturnDataCopyZero_SourceBounds(t *testing.T) {
	maxWord := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256),
		big.NewInt(1),
	)
	tooLargeSource := new(big.Int).Lsh(big.NewInt(1), 64)

	tests := []struct {
		name       string
		dataSize   int
		dataOffset *big.Int
		memOffset  *big.Int
		wantErr    bool
	}{
		{
			"empty_at_end",
			0, big.NewInt(0), big.NewInt(0), false,
		},
		{
			"empty_past_end",
			0, big.NewInt(1), big.NewInt(0), true,
		},
		{
			"nonempty_at_start",
			10, big.NewInt(0), big.NewInt(0), false,
		},
		{
			"nonempty_inside",
			10, big.NewInt(9), big.NewInt(0), false,
		},
		{
			"nonempty_at_end",
			10, big.NewInt(10), big.NewInt(0), false,
		},
		{
			"nonempty_past_end",
			10, big.NewInt(11), big.NewInt(0), true,
		},
		{
			"huge_destination_empty",
			0, big.NewInt(0), maxWord, false,
		},
		{
			"huge_destination_at_end",
			10, big.NewInt(10), maxWord, false,
		},
		{
			"huge_destination_invalid_source",
			10, big.NewInt(11), maxWord, true,
		},
		{
			"source_exceeds_uint64",
			10, tooLargeSource, big.NewInt(0), true,
		},
		{
			"maximum_source",
			10, maxWord, big.NewInt(0), true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expectedMemory := make([]byte, 32)
			expectedMemory[0] = 0xaa
			expectedMemory[31] = 0xbb

			s := &state{
				config:      &allEnabledForks,
				returnData:  make([]byte, tc.dataSize),
				memory:      append([]byte(nil), expectedMemory...),
				gas:         100,
				lastGasCost: 3,
			}

			// Preserve an earlier stack item beneath the three operands.
			s.push(big.NewInt(0xfeed))
			s.push(big.NewInt(0)) // length
			s.push(tc.dataOffset)
			s.push(tc.memOffset)

			opReturnDataCopy(s)

			if tc.wantErr {
				require.ErrorIs(t, s.err, errReturnDataOutOfBounds)
			} else {
				require.NoError(t, s.err)
			}

			require.Equal(t, tc.wantErr, s.stop)

			// Direct handler calls exclude the dispatcher's base charge.
			// Zero-length copies must not expand memory or charge copy gas.
			require.Equal(t, uint64(100), s.gas)
			require.Equal(t, uint64(3), s.lastGasCost)
			require.Zero(t, s.currentConsumedGas)
			require.Equal(t, expectedMemory, s.memory)
			require.Len(t, s.returnData, tc.dataSize)

			require.Equal(t, 1, s.sp)
			require.Zero(t, s.pop().Cmp(big.NewInt(0xfeed)))
		})
	}
}

func TestReturnDataCopyZero_Bytecode(t *testing.T) {
	maxWord := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256),
		big.NewInt(1),
	)

	tests := []struct {
		name       string
		dataOffset *big.Int
		memOffset  *big.Int
		wantErr    bool
	}{
		{
			"empty_at_end",
			big.NewInt(0), big.NewInt(0), false,
		},
		{
			"empty_past_end",
			big.NewInt(1), big.NewInt(0), true,
		},
		{
			"huge_destination_valid",
			big.NewInt(0), maxWord, false,
		},
		{
			"source_exceeds_uint64",
			new(big.Int).Lsh(big.NewInt(1), 64),
			big.NewInt(0),
			true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sourceBytes, destinationBytes [32]byte
			tc.dataOffset.FillBytes(sourceBytes[:])
			tc.memOffset.FillBytes(destinationBytes[:])

			code := []byte{PUSH1, 0x00} // length
			code = append(code, PUSH32)
			code = append(code, sourceBytes[:]...)
			code = append(code, PUSH32)
			code = append(code, destinationBytes[:]...)
			code = append(code,
				RETURNDATACOPY,
				// This marker must execute only after a valid copy.
				PUSH1, 0xab,
				PUSH1, 0x00,
				MSTORE8,
				PUSH1, 0x01,
				PUSH1, 0x00,
				RETURN,
			)

			trace := &mockTracer{}
			host := &mockHost{tracer: trace}
			contract := newMockContract(big.NewInt(0), 100, code)
			result := NewEVM().Run(contract, host, &allEnabledForks)

			var executed []string
			for _, call := range trace.calls {
				if call.name == "ExecuteState" {
					opcode, ok := call.args["opcode"].(string)
					require.True(t, ok)
					executed = append(executed, opcode)
				}
			}

			expectedExecuted := []string{
				"PUSH1",
				"PUSH32",
				"PUSH32",
				"RETURNDATACOPY",
			}

			if tc.wantErr {
				require.ErrorIs(t, result.Err, errReturnDataOutOfBounds)
				require.False(t, result.Reverted())
				require.Empty(t, result.ReturnValue)
				require.Zero(t, result.GasLeft)
				require.Equal(t, uint64(100), result.GasUsed)
			} else {
				require.NoError(t, result.Err)
				require.Equal(t, []byte{0xab}, result.ReturnValue)

				// Seven PUSH instructions: 21 gas.
				// RETURNDATACOPY: 3. MSTORE8: 3. Memory expansion: 3.
				require.Equal(t, uint64(30), result.GasUsed)
				require.Equal(t, uint64(70), result.GasLeft)

				expectedExecuted = append(expectedExecuted,
					"PUSH1",
					"PUSH1",
					"MSTORE8",
					"PUSH1",
					"PUSH1",
					"RETURN",
				)
			}

			// Invalid source offsets must stop before the marker instructions.
			require.Equal(t, expectedExecuted, executed)
		})
	}
}

/* FINISH ALPHA CHANGE 3567 -- test zero-length return-data source bounds and exceptional execution -- */
