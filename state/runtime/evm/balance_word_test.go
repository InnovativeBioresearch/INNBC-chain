/* START ALPHA CHANGE 3818 -- verify balance word bounds stack behavior and unchanged gas charging -- */

package evm

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type balanceWordHost struct {
	mockHost

	balance *big.Int
	reads   int
	address types.Address
}

func (h *balanceWordHost) GetBalance(address types.Address) *big.Int {
	h.reads++
	h.address = address

	// Return the original pointer so unintended mutation is detectable.
	return h.balance
}

type balanceWordOperation struct {
	name   string
	op     OpCode
	config chain.ForksInTime
	gas    uint64
}

func balanceWordOperations() []balanceWordOperation {
	return []balanceWordOperation{
		{
			"BALANCE/frontier",
			BALANCE,
			chain.ForksInTime{},
			20,
		},
		{
			"BALANCE/eip150",
			BALANCE,
			chain.ForksInTime{EIP150: true},
			400,
		},
		{
			"BALANCE/istanbul",
			BALANCE,
			chain.ForksInTime{Istanbul: true},
			700,
		},
		{
			"SELFBALANCE/istanbul",
			SELFBALANCE,
			chain.ForksInTime{Istanbul: true},
			5,
		},
	}
}

func balanceWordPrefix(op OpCode, address types.Address) []byte {
	if op == SELFBALANCE {
		return []byte{byte(op)}
	}

	// PUSH20 has no named constant in this opcode table.
	code := append([]byte{0x73}, address.Bytes()...)

	return append(code, byte(op))
}

func TestBalanceWord_ExecutionStackAndHostValue(t *testing.T) {
	limit := new(big.Int).Lsh(big.NewInt(1), 256)
	maximum := new(big.Int).Sub(new(big.Int).Set(limit), big.NewInt(1))

	values := []struct {
		name  string
		value *big.Int
		valid bool
	}{
		{"zero", big.NewInt(0), true},
		{"one", big.NewInt(1), true},
		{"maximum", maximum, true},
		{"overflow", limit, false},
		{"negative", big.NewInt(-1), false},
		{"nil", nil, false},
	}

	query := types.StringToAddress("0x1001")
	self := types.StringToAddress("0x1002")

	for _, operation := range balanceWordOperations() {
		for _, value := range values {
			t.Run(operation.name+"/"+value.name, func(t *testing.T) {
				host := &balanceWordHost{}
				var original *big.Int

				if value.value != nil {
					host.balance = new(big.Int).Set(value.value)
					original = new(big.Int).Set(value.value)
				}

				checkUnchanged := func() {
					t.Helper()

					if original == nil {
						require.Nil(t, host.balance)
					} else {
						require.Zero(t, host.balance.Cmp(original))
					}
				}

				prefix := balanceWordPrefix(operation.op, query)
				prefixGas := operation.gas
				wantAddress := self

				if operation.op == BALANCE {
					prefixGas += 3 // PUSH20
					wantAddress = query
				}

				// Store and return the balance as one EVM word.
				code := append([]byte(nil), prefix...)
				code = append(code,
					PUSH1, 0x00, MSTORE,
					PUSH1, 0x20, PUSH1, 0x00, RETURN,
				)

				const gas = uint64(2000)
				contract := newMockContract(big.NewInt(0), gas, code)
				contract.Address = self

				var result *runtime.ExecutionResult
				require.NotPanics(t, func() {
					result = NewEVM().Run(contract, host, &operation.config)
				})

				require.NotNil(t, result)
				require.Equal(t, 1, host.reads)
				require.Equal(t, wantAddress, host.address)
				checkUnchanged()

				if value.valid {
					require.NoError(t, result.Err)
					require.Len(t, result.ReturnValue, 32)
					require.Zero(t,
						new(big.Int).SetBytes(result.ReturnValue).Cmp(original))

					// Three PUSH1s, MSTORE and first-word memory expansion.
					wantGas := prefixGas + 15
					require.Equal(t, wantGas, result.GasUsed)
					require.Equal(t, gas-wantGas, result.GasLeft)
				} else {
					require.ErrorIs(t, result.Err, errBalanceOutOfBounds)
					require.Empty(t, result.ReturnValue)
					require.Zero(t, result.GasLeft)
					require.Equal(t, gas, result.GasUsed)
				}

				// Execute again with an earlier stack item that must survive.
				stackCode := append([]byte(nil), prefix...)
				stackCode = append(stackCode, byte(STOP))

				s := &state{
					code:   stackCode,
					gas:    gas,
					host:   host,
					msg:    contract,
					config: &operation.config,
				}
				s.push(big.NewInt(0xfeed))

				var err error
				require.NotPanics(t, func() {
					_, err = s.Run()
				})

				require.True(t, s.stop)
				require.Equal(t, gas-prefixGas, s.gas)
				require.Equal(t, 2, host.reads)
				require.Equal(t, wantAddress, host.address)

				if value.valid {
					require.NoError(t, err)
					require.Equal(t, 2, s.sp)
					require.Zero(t, s.pop().Cmp(original))
				} else {
					require.ErrorIs(t, err, errBalanceOutOfBounds)
					require.Equal(t, 1, s.sp,
						"an invalid balance must never reach the stack")
				}

				require.Zero(t, s.pop().Cmp(big.NewInt(0xfeed)))
				require.Zero(t, s.sp)
				checkUnchanged()
			})
		}
	}
}

func TestBalanceWord_GasBeforeHostRead(t *testing.T) {
	limit := new(big.Int).Lsh(big.NewInt(1), 256)
	maximum := new(big.Int).Sub(new(big.Int).Set(limit), big.NewInt(1))

	for _, operation := range balanceWordOperations() {
		for _, enough := range []bool{false, true} {
			name := "insufficient"
			if enough {
				name = "exact"
			}

			t.Run(operation.name+"/"+name, func(t *testing.T) {
				host := &balanceWordHost{
					balance: new(big.Int).Set(limit),
				}

				code := balanceWordPrefix(operation.op, types.ZeroAddress)
				code = append(code, byte(STOP))

				gas := operation.gas
				if operation.op == BALANCE {
					gas += 3
				}

				if enough {
					host.balance.Set(maximum)
				} else {
					gas--
				}

				contract := newMockContract(big.NewInt(0), gas, code)
				result := NewEVM().Run(contract, host, &operation.config)

				require.Zero(t, result.GasLeft)
				require.Equal(t, gas, result.GasUsed)

				if enough {
					require.NoError(t, result.Err)
					require.Equal(t, 1, host.reads)
					require.Zero(t, host.balance.Cmp(maximum))
				} else {
					require.ErrorIs(t, result.Err, runtime.ErrOutOfGas)
					require.Zero(t, host.reads)
					require.Zero(t, host.balance.Cmp(limit))
				}
			})
		}
	}
}

func TestBalanceWord_SelfBalanceBeforeIstanbul(t *testing.T) {
	host := &balanceWordHost{}
	const gas = uint64(1000)

	contract := newMockContract(
		big.NewInt(0),
		gas,
		[]byte{SELFBALANCE, byte(STOP)},
	)

	result := NewEVM().Run(
		contract,
		host,
		&chain.ForksInTime{EIP150: true},
	)

	require.ErrorIs(t, result.Err, errOpCodeNotFound)
	require.Zero(t, host.reads)
	require.Zero(t, result.GasLeft)
	require.Equal(t, gas, result.GasUsed)
}

/* FINISH ALPHA CHANGE 3818 -- verify balance word bounds stack behavior and unchanged gas charging -- */
