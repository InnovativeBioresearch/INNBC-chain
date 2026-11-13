/* START ALPHA CHANGE 3599 -- verify address-list child calls consume only their forwarded gas allowance -- */
package state

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/addresslist"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/evm"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestAddressListOutOfGas_InternalCall(t *testing.T) {
	caller := types.StringToAddress(
		"0x1000000000000000000000000000000000000001",
	)
	parent := types.StringToAddress(
		"0x1000000000000000000000000000000000000002",
	)
	listAddress := contracts.BlockListContractsAddr

	input, err := addresslist.ReadAddressListFunc.Encode(
		[]interface{}{caller},
	)
	require.NoError(t, err)
	require.Len(t, input, 36)

	for _, tc := range []struct {
		forwarded    uint64
		wantSuccess  uint64
		wantChildGas uint64
	}{
		{4999, 0, 4999},
		{5000, 1, 5000},
		{5001, 1, 5000},
	} {
		t.Run(fmt.Sprintf("forwarded_%d", tc.forwarded), func(t *testing.T) {
			snapshot := newStateWithPreState(map[types.Address]*PreState{
				caller: {Balance: 1000},
				parent: {Balance: 0},
			})
			tr := NewTransition(
				chain.AllForksEnabled.At(0),
				snapshot,
				newTxn(snapshot),
			)
			tr.logger = hclog.NewNullLogger()
			tr.ctx.BaseFee = new(big.Int)
			tr.deploymentBlockList = addresslist.NewAddressList(
				tr, listAddress,
			)

			// Copy the 36-byte call input into parent memory.
			code := []byte{
				evm.PUSH1, 0x24,
				evm.PUSH1, 0x00,
				evm.PUSH1, 0x00,
				evm.CALLDATACOPY,

				// CALL arguments: output size/offset, input size/offset,
				// value, destination and forwarded gas.
				evm.PUSH1, 0x00,
				evm.PUSH1, 0x00,
				evm.PUSH1, 0x24,
				evm.PUSH1, 0x00,
				evm.PUSH1, 0x00,
				byte(evm.PUSH1 + 19), // PUSH20
			}
			code = append(code, listAddress.Bytes()...)
			code = append(code,
				byte(evm.PUSH1+1), // PUSH2
				byte(tc.forwarded>>8), byte(tc.forwarded),
				evm.CALL,

				// Return the child success flag and the parent's
				// remaining gas observed by GAS.
				evm.PUSH1, 0x00,
				evm.MSTORE,
				evm.GAS,
				evm.PUSH1, 0x20,
				evm.MSTORE,
				evm.PUSH1, 0x40,
				evm.PUSH1, 0x00,
				evm.RETURN,
			)

			tr.state.SetCode(parent, code)
			require.NoError(t, tr.Error())

			const parentGas = uint64(100000)
			result := tr.Call2(
				caller, parent, input, new(big.Int), parentGas,
			)
			require.NotNil(t, result)
			require.NoError(t, result.Err)
			require.NoError(t, tr.Error())
			require.Len(t, result.ReturnValue, 64)

			success := new(big.Int).SetBytes(result.ReturnValue[:32])
			require.Equal(t, tc.wantSuccess, success.Uint64())

			// Input copying: 24; argument pushes: 21;
			// CALL overhead: 700; result storage and return: 20.
			const parentOverhead = uint64(24 + 21 + 700 + 20)
			wantGasUsed := parentOverhead + tc.wantChildGas
			require.Equal(t, wantGasUsed, result.GasUsed)
			require.Equal(t, parentGas-wantGasUsed, result.GasLeft)

			// Twelve gas is spent after the GAS observation.
			observedGas := new(big.Int).SetBytes(result.ReturnValue[32:])
			require.Equal(t, result.GasLeft+12, observedGas.Uint64())
		})
	}
}

/* FINISH ALPHA CHANGE 3599 -- verify address-list child calls consume only their forwarded gas allowance -- */
