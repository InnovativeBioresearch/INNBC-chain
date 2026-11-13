/* START ALPHA CHANGE 3579 -- test unused large offsets in zero-sized memory ranges -- */
package evm

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type zeroMemoryRangeOffset struct {
	name  string
	value *big.Int
}

func zeroMemoryRangeOffsets() []zeroMemoryRangeOffset {
	return []zeroMemoryRangeOffset{
		{
			name:  "two_to_64",
			value: new(big.Int).Lsh(big.NewInt(1), 64),
		},
		{
			name: "max_uint256",
			value: new(big.Int).Sub(
				new(big.Int).Lsh(big.NewInt(1), 256),
				big.NewInt(1),
			),
		},
	}
}

func zeroMemoryRangePush32(code []byte, value *big.Int) []byte {
	var word [32]byte
	value.FillBytes(word[:])
	code = append(code, PUSH32)
	return append(code, word[:]...)
}

type zeroMemoryRangeHost struct {
	mockHost

	codeReads int
	calls     int
}

func (h *zeroMemoryRangeHost) GetCode(types.Address) []byte {
	h.codeReads++
	return []byte{byte(STOP)}
}

func (h *zeroMemoryRangeHost) Callx(
	contract *runtime.Contract,
	_ runtime.Host,
) *runtime.ExecutionResult {
	h.calls++
	return &runtime.ExecutionResult{
		ReturnValue: []byte{0xab, 0xcd},
		GasLeft:     contract.Gas,
	}
}

func TestZeroMemoryRange_Helper(t *testing.T) {
	for _, offset := range zeroMemoryRangeOffsets() {
		for _, size := range []int64{0, 1} {
			t.Run(fmt.Sprintf("%s/size_%d", offset.name, size), func(t *testing.T) {
				s := &state{
					memory:      make([]byte, 32),
					gas:         100,
					lastGasCost: 3,
				}
				s.memory[0] = 0xaa
				s.memory[31] = 0xbb
				before := append([]byte(nil), s.memory...)

				ok := s.allocateMemory(offset.value, big.NewInt(size))

				if size == 0 {
					require.True(t, ok)
					require.NoError(t, s.err)
					require.False(t, s.stop)
				} else {
					require.False(t, ok)
					require.ErrorIs(t, s.err, errReturnDataOutOfBounds)
					require.True(t, s.stop)
				}

				require.Equal(t, before, s.memory)
				require.Equal(t, uint64(100), s.gas)
				require.Equal(t, uint64(3), s.lastGasCost)
				require.Zero(t, s.currentConsumedGas)
			})
		}
	}
}

func TestZeroMemoryRange_CopyBytecode(t *testing.T) {
	const gas = uint64(10000)

	for _, tc := range []struct {
		op      OpCode
		wantGas uint64
	}{
		{CALLDATACOPY, 29},
		{CODECOPY, 29},
		{EXTCODECOPY, 729},
	} {
		for _, offset := range zeroMemoryRangeOffsets() {
			for _, size := range []int64{0, 1} {
				name := fmt.Sprintf(
					"%s/%s/size_%d", tc.op.String(), offset.name, size,
				)
				t.Run(name, func(t *testing.T) {
					// Stack order: length, source offset, destination offset.
					code := []byte{PUSH1, byte(size), PUSH1, 0x00}
					code = zeroMemoryRangePush32(code, offset.value)

					if tc.op == EXTCODECOPY {
						// EXTCODECOPY additionally pops the target address.
						code = append(code, PUSH1, 0x01)
					}

					code = append(code,
						byte(tc.op),
						// Capture MSIZE before the following store expands memory.
						MSIZE,
						PUSH1, 0x00,
						MSTORE8,
						PUSH1, 0x01,
						PUSH1, 0x00,
						RETURN,
					)

					host := &zeroMemoryRangeHost{}
					contract := newMockContract(big.NewInt(0), gas, code)
					result := NewEVM().Run(contract, host, &allEnabledForks)

					require.Zero(t, host.calls)

					if size != 0 {
						require.ErrorIs(t, result.Err, errReturnDataOutOfBounds)
						require.Zero(t, result.GasLeft)
						require.Equal(t, gas, result.GasUsed)
						require.Zero(t, host.codeReads)
						return
					}

					require.NoError(t, result.Err)
					require.Equal(t, []byte{0x00}, result.ReturnValue,
						"zero-length copying must leave MSIZE at zero")
					require.Equal(t, tc.wantGas, result.GasUsed)
					require.Equal(t, gas-tc.wantGas, result.GasLeft)

					if tc.op == EXTCODECOPY {
						require.Equal(t, 1, host.codeReads)
					} else {
						require.Zero(t, host.codeReads)
					}
				})
			}
		}
	}
}

func TestZeroMemoryRange_CallOutputBytecode(t *testing.T) {
	const gas = uint64(10000)

	for _, tc := range []struct {
		op      OpCode
		wantGas uint64
	}{
		{CALL, 752},
		{CALLCODE, 752},
		{DELEGATECALL, 749},
		{STATICCALL, 749},
	} {
		for _, offset := range zeroMemoryRangeOffsets() {
			for _, size := range []int64{0, 1} {
				name := fmt.Sprintf(
					"%s/%s/output_size_%d",
					tc.op.String(), offset.name, size,
				)
				t.Run(name, func(t *testing.T) {
					// Output size and offset, then empty input size and offset.
					code := []byte{PUSH1, byte(size)}
					code = zeroMemoryRangePush32(code, offset.value)
					code = append(code,
						PUSH1, 0x00,
						PUSH1, 0x00,
					)

					if tc.op == CALL || tc.op == CALLCODE {
						code = append(code, PUSH1, 0x00) // value
					}

					code = append(code,
						PUSH1, 0x01, // address
						/* START ALPHA CHANGE 3580 -- encode PUSH2 using the existing PUSH1 constant -- */
						PUSH1+1, 0x01, 0xf4, // PUSH2 (0x61): forward 500 gas
						/* FINISH ALPHA CHANGE 3580 -- encode PUSH2 using the existing PUSH1 constant -- */
						byte(tc.op),
						// Stack now contains the call-success flag.
						// Record MSIZE before expanding memory.
						MSIZE,
						PUSH1, 0x01,
						MSTORE8,
						// Record the call-success flag.
						PUSH1, 0x00,
						MSTORE8,
						// Returned data remains available even with no output copy.
						RETURNDATASIZE,
						PUSH1, 0x02,
						MSTORE8,
						PUSH1, 0x03,
						PUSH1, 0x00,
						RETURN,
					)

					host := &zeroMemoryRangeHost{}
					contract := newMockContract(big.NewInt(0), gas, code)
					result := NewEVM().Run(contract, host, &allEnabledForks)

					if size != 0 {
						require.ErrorIs(t, result.Err, errReturnDataOutOfBounds)
						require.Zero(t, result.GasLeft)
						require.Equal(t, gas, result.GasUsed)
						require.Zero(t, host.calls)
						require.Zero(t, host.codeReads)
						return
					}

					require.NoError(t, result.Err)
					require.Equal(t, 1, host.calls,
						"the call must reach its callee")
					require.Equal(t, 1, host.codeReads)
					require.Equal(t, []byte{0x01, 0x00, 0x02}, result.ReturnValue,
						"expect success, zero MSIZE before stores, and two returned bytes")
					require.Equal(t, tc.wantGas, result.GasUsed)
					require.Equal(t, gas-tc.wantGas, result.GasLeft)
				})
			}
		}
	}
}

/* FINISH ALPHA CHANGE 3579 -- test unused large offsets in zero-sized memory ranges -- */
