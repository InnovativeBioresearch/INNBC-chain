/* START ALPHA CHANGE 3598 -- verify address-list out-of-gas accounting and preserved call results -- */
package addresslist

import (
	"fmt"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

func TestAddressListOutOfGas_RunAccounting(t *testing.T) {
	caller := types.StringToAddress("0x1001")
	target := types.StringToAddress("0x1002")

	for _, tc := range []struct {
		name     string
		method   *abi.Method
		cost     uint64
		wantRole Role
	}{
		{"read", ReadAddressListFunc, 5000, EnabledRole},
		{"set_admin", SetAdminFunc, 20000, AdminRole},
		{"set_enabled", SetEnabledFunc, 20000, EnabledRole},
		{"set_none", SetNoneFunc, 20000, NoRole},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, supplied := range []uint64{
				0, 1, tc.cost - 1, tc.cost, tc.cost + 1,
			} {
				t.Run(fmt.Sprintf("gas_%d", supplied), func(t *testing.T) {
					a := newMockAddressList()
					a.SetRole(caller, AdminRole)
					a.SetRole(target, EnabledRole)

					input, err := tc.method.Encode(
						[]interface{}{target},
					)
					require.NoError(t, err)

					contract := &runtime.Contract{
						Caller: caller,
						Input:  input,
						Gas:    supplied,
					}
					result := a.Run(contract, nil, nil)
					require.NotNil(t, result)
					require.Equal(t, supplied, contract.Gas)

					if supplied < tc.cost {
						require.ErrorIs(t, result.Err, runtime.ErrOutOfGas)
						require.Equal(t, supplied, result.GasUsed)
						require.Zero(t, result.GasLeft)
						require.Empty(t, result.ReturnValue)
						require.Equal(t, EnabledRole, a.GetRole(target))
					} else {
						require.NoError(t, result.Err)
						require.Equal(t, tc.cost, result.GasUsed)
						require.Equal(t, supplied-tc.cost, result.GasLeft)
						require.Equal(t, tc.wantRole, a.GetRole(target))

						if tc.name == "read" {
							require.Equal(t, EnabledRole.Bytes(), result.ReturnValue)
						} else {
							require.Empty(t, result.ReturnValue)
						}
					}

					require.Equal(t, AdminRole, a.GetRole(caller))
				})
			}
		})
	}
}

func TestAddressListOutOfGas_OtherErrorsUnchanged(t *testing.T) {
	caller := types.StringToAddress("0x1001")
	target := types.StringToAddress("0x1002")

	for _, tc := range []struct {
		name    string
		static  bool
		wantErr error
	}{
		{"static_write", true, errWriteProtection},
		{"unauthorized_write", false, runtime.ErrNotAuth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newMockAddressList()
			a.SetRole(target, EnabledRole)
			if tc.static {
				a.SetRole(caller, AdminRole)
			}

			input, err := SetNoneFunc.Encode(
				[]interface{}{target},
			)
			require.NoError(t, err)

			const supplied = uint64(25000)
			result := a.Run(&runtime.Contract{
				Caller: caller,
				Input:  input,
				Gas:    supplied,
				Static: tc.static,
			}, nil, nil)

			require.NotNil(t, result)
			require.ErrorIs(t, result.Err, tc.wantErr)
			require.Equal(t, uint64(20000), result.GasUsed)
			require.Equal(t, uint64(5000), result.GasLeft)
			require.Empty(t, result.ReturnValue)
			require.Equal(t, EnabledRole, a.GetRole(target))
		})
	}
}

/* FINISH ALPHA CHANGE 3598 -- verify address-list out-of-gas accounting and preserved call results -- */
