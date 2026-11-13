/* START ALPHA CHANGE 2919 -- verify ordinary EVM behavior at the retired aggregate BLS address -- */
package state

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestBLSRetirement_OrdinaryEVMAddress(t *testing.T) {
	caller := types.StringToAddress("0x70001")
	retired := types.StringToAddress("0x2030")
	const executionGas uint64 = 150000

	tests := []struct {
		name  string
		input []byte
		code  []byte
	}{
		{
			name: "empty_account_empty_input",
		},
		{
			name:  "empty_account_nonempty_input",
			input: []byte{0x01, 0x02, 0x03},
		},
		{
			name:  "ordinary_bytecode_executes",
			input: []byte{0x01, 0x02, 0x03},
			/*
				PUSH1 42, PUSH1 0, MSTORE,
				PUSH1 32, PUSH1 0, RETURN.
			*/
			code: []byte{
				0x60, 0x2a,
				0x60, 0x00,
				0x52,
				0x60, 0x20,
				0x60, 0x00,
				0xf3,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snap := newStateWithPreState(map[types.Address]*PreState{
				caller: {
					Balance: 1000,
				},
			})

			tr := NewTransition(
				chain.ForksInTime{
					Byzantium: true,
					Istanbul:  true,
				},
				snap,
				newTxn(snap),
			)

			if tc.code != nil {
				tr.state.SetCode(retired, tc.code)
				require.NoError(t, tr.Error())
			}

			result := tr.Call2(
				caller,
				retired,
				tc.input,
				big.NewInt(0),
				executionGas,
			)

			require.NotNil(t, result)
			require.NoError(t, tr.Error())
			require.NoError(t, result.Err)

			if tc.code == nil {
				require.Empty(t, result.ReturnValue)
				require.Equal(t, executionGas, result.GasLeft)
			} else {
				expected := make([]byte, 32)
				expected[31] = 42

				require.Equal(t, expected, result.ReturnValue)
				require.Positive(t, result.GasLeft)
				require.Less(t, result.GasLeft, executionGas)
			}

			require.Equal(t, big.NewInt(1000), tr.GetBalance(caller))
			require.NoError(t, tr.Error())
		})
	}
}

/* FINISH ALPHA CHANGE 2919 -- verify ordinary EVM behavior at the retired aggregate BLS address -- */
