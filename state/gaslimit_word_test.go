/* START ALPHA CHANGE 3591 -- verify full-width header gas limits through execution and gas-pool accounting -- */
package state

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/state/runtime/evm"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGasLimitWord_HeaderExecution(t *testing.T) {
	code := []byte{
		evm.GASLIMIT,
		evm.PUSH1, 0x00,
		evm.MSTORE,
		evm.PUSH1, 0x20,
		evm.PUSH1, 0x00,
		evm.RETURN,
	}

	for _, gasLimit := range []uint64{
		5_242_880,
		math.MaxInt64,
		uint64(math.MaxInt64) + 1,
		math.MaxUint64,
	} {
		t.Run(fmt.Sprintf("limit_%d", gasLimit), func(t *testing.T) {
			executor, _, _, transaction := issue7AExecutionFixture(t, code)

			header := &types.Header{
				Number:   1,
				GasLimit: gasLimit,
				BaseFee:  1,
			}

			tr, err := executor.BeginTxn(
				types.EmptyRootHash,
				header,
				types.ZeroAddress,
			)
			require.NoError(t, err)

			// Exercise the actual header-to-context conversion and pool setup.
			require.Equal(t, gasLimit, uint64(tr.GetTxContext().GasLimit))
			require.Equal(t, gasLimit, tr.gasPool)

			const transactionGas = uint64(50000)
			transaction.Gas = transactionGas
			transaction.ComputeHash(header.Number)

			result, err := tr.Apply(transaction.Copy())
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NoError(t, result.Err)
			require.NoError(t, tr.Error())

			// Compare exact 32-byte output against the original header value.
			var want [32]byte
			binary.BigEndian.PutUint64(want[24:], header.GasLimit)
			require.Equal(t, want[:], result.ReturnValue)

			// Empty-input call: 21,000 intrinsic gas.
			// GASLIMIT, three PUSH1s, MSTORE and expansion: 17 gas.
			const wantGasUsed = uint64(21000 + 17)
			require.Equal(t, wantGasUsed, result.GasUsed)
			require.Equal(t, transactionGas-wantGasUsed, result.GasLeft)
			require.Equal(t, gasLimit-wantGasUsed, tr.gasPool)
		})
	}
}

/* FINISH ALPHA CHANGE 3591 -- verify full-width header gas limits through execution and gas-pool accounting -- */
