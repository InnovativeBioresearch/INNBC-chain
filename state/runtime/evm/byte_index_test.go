/* START ALPHA CHANGE 3562 -- test full-width BYTE indices and unchanged execution behavior -- */
package evm

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/stretchr/testify/require"
)

func TestByteIndex_SelectionStackAndGas(t *testing.T) {
	// Distinct, nonzero bytes expose reversed indexing and truncated indices.
	var wordBytes [32]byte
	for index := range wordBytes {
		wordBytes[index] = byte(index + 1)
	}
	word := new(big.Int).SetBytes(wordBytes[:])

	type testCase struct {
		index *big.Int
		want  byte
	}

	twoTo64 := new(big.Int).Lsh(big.NewInt(1), 64)
	tests := []testCase{
		{big.NewInt(32), 0},
		{big.NewInt(33), 0},
		{new(big.Int).Lsh(big.NewInt(1), 63), 0},
		{new(big.Int).Set(twoTo64), 0},
		{new(big.Int).Add(twoTo64, big.NewInt(1)), 0},
		{new(big.Int).Add(twoTo64, big.NewInt(31)), 0},
		{
			new(big.Int).Sub(
				new(big.Int).Lsh(big.NewInt(1), 256),
				big.NewInt(1),
			),
			0,
		},
	}

	for index, want := range wordBytes {
		tests = append(tests, testCase{
			index: big.NewInt(int64(index)),
			want:  want,
		})
	}

	for _, tc := range tests {
		t.Run("index_"+tc.index.String(), func(t *testing.T) {
			s := &state{
				/* START ALPHA CHANGE 3563 -- explicitly encode the typed STOP opcode as a byte -- */
				code: []byte{BYTE, byte(STOP)},
				/* FINISH ALPHA CHANGE 3563 -- explicitly encode the typed STOP opcode as a byte -- */
				gas:  3,
				host: &mockHost{},
			}

			// BYTE must leave the earlier stack item untouched.
			s.push(big.NewInt(0xfeed))
			s.push(word)
			s.push(tc.index)

			_, err := s.Run()
			require.NoError(t, err)

			// BYTE consumes exactly three gas; STOP consumes zero.
			require.Zero(t, s.gas)

			// Two operands become one result, preserving the earlier item.
			require.Equal(t, 2, s.sp)
			/* START ALPHA CHANGE 3564 -- compare stack integers numerically -- */
			require.Zero(t, s.pop().Cmp(big.NewInt(int64(tc.want))))
			require.Zero(t, s.pop().Cmp(big.NewInt(0xfeed)))
			/* FINISH ALPHA CHANGE 3564 -- compare stack integers numerically -- */
			require.Zero(t, s.sp)
		})
	}
}

func TestByteIndex_Bytecode(t *testing.T) {
	tests := []struct {
		name  string
		index *big.Int
		want  byte
	}{
		{
			name:  "first_byte",
			index: big.NewInt(0),
			want:  0xab,
		},
		{
			name:  "first_out_of_range",
			index: big.NewInt(32),
			want:  0,
		},
		{
			name:  "reported_2_to_64_truncation",
			index: new(big.Int).Lsh(big.NewInt(1), 64),
			want:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// 0xAB followed by 31 zero bytes.
			wordBytes := [32]byte{0xab}
			var indexBytes [32]byte
			tc.index.FillBytes(indexBytes[:])

			code := append([]byte{PUSH32}, wordBytes[:]...)
			code = append(code, PUSH32)
			code = append(code, indexBytes[:]...)
			code = append(code,
				BYTE,
				PUSH1, 0x00,
				MSTORE8,
				PUSH1, 0x01,
				PUSH1, 0x00,
				RETURN,
			)

			contract := newMockContract(big.NewInt(0), 100, code)
			result := NewEVM().Run(contract, &mockHost{}, &allEnabledForks)

			require.NoError(t, result.Err)
			require.Equal(t, []byte{tc.want}, result.ReturnValue)

			// Five PUSH instructions: 15 gas.
			// BYTE: 3. MSTORE8: 3. Memory expansion: 3. RETURN: 0.
			require.Equal(t, uint64(24), result.GasUsed)
			require.Equal(t, uint64(76), result.GasLeft)
		})
	}
}

func TestByteIndex_InsufficientGasAndStack(t *testing.T) {
	word := big.NewInt(0xab)
	index := new(big.Int).Lsh(big.NewInt(1), 64)

	tests := []struct {
		name    string
		gas     uint64
		stack   []*big.Int
		wantErr error
	}{
		{
			name:    "insufficient_gas",
			gas:     2,
			stack:   []*big.Int{word, index},
			wantErr: errOutOfGas,
		},
		{
			name:  "empty_stack",
			gas:   3,
			stack: nil,
			wantErr: &runtime.StackUnderflowError{
				StackLen: 0,
				Required: 2,
			},
		},
		{
			name:  "one_operand",
			gas:   3,
			stack: []*big.Int{word},
			wantErr: &runtime.StackUnderflowError{
				StackLen: 1,
				Required: 2,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &state{
				/* START ALPHA CHANGE 3563 -- explicitly encode the typed STOP opcode as a byte -- */
				code: []byte{BYTE, byte(STOP)},
				/* FINISH ALPHA CHANGE 3563 -- explicitly encode the typed STOP opcode as a byte -- */
				gas:  tc.gas,
				host: &mockHost{},
			}

			for _, operand := range tc.stack {
				s.push(operand)
			}

			_, err := s.Run()
			require.Equal(t, tc.wantErr, err)

			// Dispatcher rejection must happen before operand consumption.
			require.Equal(t, len(tc.stack), s.sp)
		})
	}
}

/* FINISH ALPHA CHANGE 3562 -- test full-width BYTE indices and unchanged execution behavior -- */
