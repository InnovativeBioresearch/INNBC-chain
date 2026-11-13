/* START ALPHA CHANGE 3588 -- verify SIGNEXTEND underflow rolls back call state -- */
package state

import (
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/evm"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestSignExtendStack_CallRollback(t *testing.T) {
	caller := types.StringToAddress(
		"0x1000000000000000000000000000000000000001",
	)
	target := types.StringToAddress(
		"0x1000000000000000000000000000000000000002",
	)

	slot0 := types.ZeroHash
	slot1 := types.BytesToHash([]byte{1})
	originalValue := types.BytesToHash([]byte{9})

	for _, tc := range []struct {
		name     string
		operands []byte
		count    int
		valid    bool
	}{
		{
			name: "empty_stack",
		},
		{
			name:     "one_operand_zero",
			operands: []byte{evm.PUSH1, 0x00},
			count:    1,
		},
		{
			name:     "one_operand_32",
			operands: []byte{evm.PUSH1, 0x20},
			count:    1,
		},
		{
			name: "valid_two_operands",
			operands: []byte{
				evm.PUSH1, 0x7f,
				evm.PUSH1, 0x00,
			},
			count: 2,
			valid: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := newStateWithPreState(map[types.Address]*PreState{
				caller: {
					Balance: 1000,
				},
				target: {
					Balance: 50,
					State: map[types.Hash]types.Hash{
						slot0: originalValue,
					},
				},
			})

			tr := NewTransition(
				chain.AllForksEnabled.At(0),
				snapshot,
				newTxn(snapshot),
			)
			tr.logger = hclog.NewNullLogger()
			tr.ctx.BaseFee = big.NewInt(0)

			code := []byte{
				// Write slot zero before reaching SIGNEXTEND.
				evm.PUSH1, 0x01,
				evm.PUSH1, 0x00,
				evm.SSTORE,
			}
			code = append(code, tc.operands...)
			code = append(code,
				evm.SIGNEXTEND,
				// This later write must not survive underflow.
				evm.PUSH1, 0x02,
				evm.PUSH1, 0x01,
				evm.SSTORE,
				byte(evm.STOP),
			)

			tr.state.SetCode(target, code)
			require.NoError(t, tr.Error())

			const gas = uint64(100000)
			result := tr.Call2(caller, target, nil, big.NewInt(7), gas)

			require.NotNil(t, result)
			require.NoError(t, tr.Error())

			if tc.valid {
				require.NoError(t, result.Err)
				require.Greater(t, result.GasLeft, uint64(0))
				require.Equal(t,
					types.BytesToHash([]byte{1}),
					tr.state.GetState(target, slot0),
				)
				require.Equal(t,
					types.BytesToHash([]byte{2}),
					tr.state.GetState(target, slot1),
				)
				require.Zero(t, tr.GetBalance(caller).Cmp(big.NewInt(993)))
				require.Zero(t, tr.GetBalance(target).Cmp(big.NewInt(57)))
			} else {
				var underflow *runtime.StackUnderflowError
				require.ErrorAs(t, result.Err, &underflow)
				require.Equal(t, tc.count, underflow.StackLen)
				require.Equal(t, 2, underflow.Required)
				require.Zero(t, result.GasLeft)
				require.Equal(t, gas, result.GasUsed)

				// Restore the earlier write and reject the later marker.
				require.Equal(t, originalValue, tr.state.GetState(target, slot0))
				require.Equal(t, types.ZeroHash, tr.state.GetState(target, slot1))

				// The failed call must also roll back its value transfer.
				require.Zero(t, tr.GetBalance(caller).Cmp(big.NewInt(1000)))
				require.Zero(t, tr.GetBalance(target).Cmp(big.NewInt(50)))
			}

			require.NoError(t, tr.Error())
		})
	}
}

/* FINISH ALPHA CHANGE 3588 -- verify SIGNEXTEND underflow rolls back call state -- */
