/* START ALPHA CHANGE 3587 -- verify SIGNEXTEND stack requirements and valid execution -- */
package evm

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/stretchr/testify/require"
)

func TestSignExtendStack_Underflow(t *testing.T) {
	maxWord := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256),
		big.NewInt(1),
	)

	for _, tc := range []struct {
		name  string
		stack []*big.Int
	}{
		{name: "empty_stack"},
		{
			name:  "one_operand_zero",
			stack: []*big.Int{big.NewInt(0)},
		},
		{
			name:  "one_operand_32",
			stack: []*big.Int{big.NewInt(32)},
		},
		{
			name:  "one_operand_max_uint256",
			stack: []*big.Int{maxWord},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &state{
				code: []byte{SIGNEXTEND, byte(STOP)},
				gas:  100,
				host: &mockHost{},
			}

			for _, operand := range tc.stack {
				s.push(operand)
			}

			_, err := s.Run()

			var underflow *runtime.StackUnderflowError
			require.ErrorAs(t, err, &underflow)
			require.Equal(t, len(tc.stack), underflow.StackLen)
			require.Equal(t, 2, underflow.Required)
			require.True(t, s.stop)

			// Reject before charging opcode gas or consuming operands.
			require.Equal(t, uint64(100), s.gas)
			require.Equal(t, len(tc.stack), s.sp)

			for index, operand := range tc.stack {
				require.Zero(t, s.stack[index].Cmp(operand))
			}
		})
	}
}

func TestSignExtendStack_ValidResultsAndGas(t *testing.T) {
	for _, firstByte := range []byte{0x01, 0x80} {
		var wordBytes [32]byte
		for index := range wordBytes {
			wordBytes[index] = firstByte + byte(index)
		}
		word := new(big.Int).SetBytes(wordBytes[:])

		for index := range 34 {
			name := fmt.Sprintf("word_%02x/index_%d", firstByte, index)

			t.Run(name, func(t *testing.T) {
				// Build the expected result using byte-level sign filling.
				wantBytes := wordBytes
				if index < 32 {
					signAt := 31 - index
					var fill byte
					if wordBytes[signAt]&0x80 != 0 {
						fill = 0xff
					}
					for byteIndex := range signAt {
						wantBytes[byteIndex] = fill
					}
				}
				want := new(big.Int).SetBytes(wantBytes[:])

				s := &state{
					code: []byte{SIGNEXTEND, byte(STOP)},
					gas:  5,
					host: &mockHost{},
				}

				s.push(big.NewInt(0xfeed))
				s.push(word)
				s.push(big.NewInt(int64(index)))

				_, err := s.Run()
				require.NoError(t, err)

				// SIGNEXTEND costs five gas; STOP costs zero.
				require.Zero(t, s.gas)

				// Two operands become one result, preserving earlier items.
				require.Equal(t, 2, s.sp)
				require.Zero(t, s.pop().Cmp(want))
				require.Zero(t, s.pop().Cmp(big.NewInt(0xfeed)))
				require.Zero(t, s.sp)
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3587 -- verify SIGNEXTEND stack requirements and valid execution -- */
