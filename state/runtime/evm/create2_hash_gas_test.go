/* START ALPHA CHANGE 3584 -- test upfront CREATE2 hashing gas for unfunded creation -- */
package evm

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type create2HashGasHost struct {
	mockHost

	balance      *big.Int
	balanceReads int
	calls        int
	created      *runtime.Contract
}

func (h *create2HashGasHost) GetBalance(types.Address) *big.Int {
	h.balanceReads++
	return new(big.Int).Set(h.balance)
}

func (h *create2HashGasHost) GetNonce(types.Address) uint64 {
	return 7
}

func (h *create2HashGasHost) Callx(
	contract *runtime.Contract,
	_ runtime.Host,
) *runtime.ExecutionResult {
	h.calls++
	h.created = contract

	// Isolate the parent's upfront charges by returning all child gas.
	return &runtime.ExecutionResult{
		GasLeft: contract.Gas,
	}
}

func create2HashGasCode(op OpCode, length byte) []byte {
	// Allocate 64 bytes before creation:
	// two PUSH1 instructions, MSTORE and memory expansion cost 15 gas.
	code := []byte{
		PUSH1, 0x00,
		PUSH1, 0x20,
		MSTORE,
	}

	if op == CREATE2 {
		code = append(code, PUSH1, 0x07) // salt
	}

	// Stack order: [salt,] length, offset, endowment.
	code = append(code,
		PUSH1, length,
		PUSH1, 0x00,
		PUSH1, 0x01,
		byte(op),
	)

	return code
}

func TestCreate2HashGas_UnfundedBoundary(t *testing.T) {
	for _, available := range []uint64{32005, 32006} {
		t.Run(fmt.Sprintf("opcode_gas_%d", available), func(t *testing.T) {
			host := &create2HashGasHost{
				balance: big.NewInt(0),
			}

			code := create2HashGasCode(CREATE2, 32)
			code = append(code, byte(STOP))

			// Memory setup costs 15 gas; four argument pushes cost 12.
			const setupGas = uint64(27)
			gas := setupGas + available

			contract := newMockContract(big.NewInt(0), gas, code)
			result := NewEVM().Run(contract, host, &allEnabledForks)

			require.Zero(t, host.calls,
				"unfunded creation must not invoke child execution")
			require.Nil(t, host.created)

			if available == 32005 {
				require.ErrorIs(t, result.Err, runtime.ErrOutOfGas)
				require.Zero(t, result.GasLeft)
				require.Equal(t, gas, result.GasUsed)
				require.Zero(t, host.balanceReads,
					"hashing gas must be charged before reading balance")
				return
			}

			// Exactly enough upfront gas: creation fails for insufficient
			// funds, pushes zero, and the following STOP succeeds.
			require.NoError(t, result.Err)
			require.Zero(t, result.GasLeft)
			require.Equal(t, gas, result.GasUsed)
			require.Equal(t, 1, host.balanceReads)
		})
	}
}

func TestCreate2HashGas_ChargesAndCreationControls(t *testing.T) {
	const gas = uint64(100000)

	for _, op := range []OpCode{CREATE2, CREATE} {
		for _, funded := range []bool{false, true} {
			for _, tc := range []struct {
				length  byte
				hashGas uint64
			}{
				{length: 0, hashGas: 0},
				{length: 1, hashGas: 6},
				{length: 32, hashGas: 6},
				{length: 33, hashGas: 12},
			} {
				name := fmt.Sprintf(
					"%s/funded_%t/length_%d",
					op.String(), funded, tc.length,
				)

				t.Run(name, func(t *testing.T) {
					balance := big.NewInt(0)
					if funded {
						balance.SetInt64(1)
					}

					host := &create2HashGasHost{
						balance: balance,
					}

					code := create2HashGasCode(op, tc.length)
					code = append(code,
						// Store and return the creation result.
						PUSH1, 0x00,
						MSTORE,
						PUSH1, 0x20,
						PUSH1, 0x00,
						RETURN,
					)

					contract := newMockContract(big.NewInt(0), gas, code)
					result := NewEVM().Run(contract, host, &allEnabledForks)

					require.NoError(t, result.Err)
					require.Len(t, result.ReturnValue, 32)
					require.Equal(t, 1, host.balanceReads)

					// CREATE setup: 15 memory gas + 9 argument-push gas.
					// Storing and returning the result costs another 12.
					wantGas := uint64(32000 + 24 + 12)
					if op == CREATE2 {
						// Additional salt push and init-code hashing.
						wantGas += 3 + tc.hashGas
					}

					require.Equal(t, wantGas, result.GasUsed)
					require.Equal(t, gas-wantGas, result.GasLeft)

					value := new(big.Int).SetBytes(result.ReturnValue)

					if !funded {
						require.Zero(t, value.Sign(),
							"unfunded creation must return zero")
						require.Zero(t, host.calls)
						require.Nil(t, host.created)
						return
					}

					require.Equal(t, 1, host.calls)
					require.NotNil(t, host.created)
					require.Equal(t, runtime.Create, host.created.Type)
					require.Len(t, host.created.Code, int(tc.length))
					require.Zero(t, host.created.Value.Cmp(big.NewInt(1)))

					wantAddress := crypto.CreateAddress(types.ZeroAddress, 7)
					if op == CREATE2 {
						wantAddress = crypto.CreateAddress2(
							types.ZeroAddress,
							types.BytesToHash([]byte{7}),
							make([]byte, int(tc.length)),
						)
					}

					require.Equal(t, wantAddress, host.created.Address)
					require.Zero(t, value.Cmp(
						new(big.Int).SetBytes(wantAddress.Bytes()),
					))
				})
			}
		}
	}
}

/* FINISH ALPHA CHANGE 3584 -- test upfront CREATE2 hashing gas for unfunded creation -- */
