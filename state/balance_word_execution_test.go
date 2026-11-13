/* START ALPHA CHANGE 3819 -- verify oversized balance reads fail cleanly through real state execution -- */

package state_test

import (
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestBalanceWord_GenesisOverflowCallRollback(t *testing.T) {
	caller := types.StringToAddress("0x9101")
	target := types.StringToAddress("0x9102")
	recipient := types.StringToAddress("0x9103")

	slot0 := types.ZeroHash
	slot1 := types.StringToHash("0x01")
	original := types.StringToHash("0x09")

	limit := new(big.Int).Lsh(big.NewInt(1), 256)
	maximum := new(big.Int).Sub(new(big.Int).Set(limit), big.NewInt(1))

	for _, self := range []bool{false, true} {
		name := "BALANCE"
		if self {
			name = "SELFBALANCE"
		}

		t.Run(name, func(t *testing.T) {
			// Write slot zero before attempting to read the balance.
			code := []byte{
				0x60, 0x01, 0x60, 0x00, 0x55,
			}

			if self {
				code = append(code, 0x47) // SELFBALANCE
			} else {
				code = append(code, 0x60, 0x00, 0x31) // BALANCE(0)
			}

			code = append(code,
				0x60, 0x00, 0x52, // MSTORE
				0x60, 0x02, 0x60, 0x01, 0x55, // later storage marker
				0x00, // STOP
			)

			callee := target
			if self {
				callee = types.ZeroAddress
			}

			alloc := map[types.Address]*chain.GenesisAccount{
				caller: {
					Balance: big.NewInt(1000),
				},
				target: {
					Balance: big.NewInt(50),
				},
				types.ZeroAddress: {
					Balance: new(big.Int).Set(maximum),
				},
			}
			alloc[callee].Code = code
			alloc[callee].Storage = map[types.Hash]types.Hash{
				slot0: original,
			}

			storage := itrie.NewMemoryStorage()
			t.Cleanup(func() {
				require.NoError(t, storage.Close())
			})

			executor := statepkg.NewExecutor(
				&chain.Params{
					ChainID: 1982,
					Forks:   chain.AllForksEnabled,
				},
				itrie.NewState(storage),
				hclog.NewNullLogger(),
			)
			executor.GetHash = func(*types.Header) statepkg.GetHashByNumber {
				return func(uint64) (types.Hash, error) {
					return types.ZeroHash, nil
				}
			}

			root, err := executor.WriteGenesis(alloc, types.ZeroHash)
			require.NoError(t, err)

			tr, err := executor.BeginTxn(
				root,
				&types.Header{
					Number:   1,
					GasLimit: 1_000_000,
					BaseFee:  1,
				},
				types.ZeroAddress,
			)
			require.NoError(t, err)

			const gas = uint64(100000)

			if !self {
				// The zero address has no code in this case. Its successful
				// transfer creates the oversized balance before the test call.
				funding := tr.Call2(
					caller,
					types.ZeroAddress,
					nil,
					big.NewInt(1),
					gas,
				)
				require.NoError(t, funding.Err)
				require.NoError(t, tr.Error())
				require.Zero(t,
					tr.GetBalance(types.ZeroAddress).Cmp(limit))
			}

			callerBefore := new(big.Int).Set(tr.GetBalance(caller))
			calleeBefore := new(big.Int).Set(tr.GetBalance(callee))

			value := big.NewInt(7)
			if self {
				// This call temporarily raises the zero-address balance
				// from the maximum to 2^256 before SELFBALANCE executes.
				value = big.NewInt(1)
			}

			var result *runtime.ExecutionResult
			require.NotPanics(t, func() {
				result = tr.Call2(caller, callee, nil, value, gas)
			})

			require.NotNil(t, result)
			require.EqualError(t, result.Err,
				"balance outside unsigned 256-bit range")
			require.NotErrorIs(t, result.Err, statepkg.ErrLocalStateFailure)
			require.NoError(t, tr.Error())
			require.Zero(t, result.GasLeft)
			require.Equal(t, gas, result.GasUsed)

			// Restore the earlier storage write and reject the later marker.
			require.Equal(t, original, tr.GetStorage(callee, slot0))
			require.Equal(t, types.ZeroHash, tr.GetStorage(callee, slot1))

			// Roll back this failed call's value transfer.
			require.Zero(t, tr.GetBalance(caller).Cmp(callerBefore))
			require.Zero(t, tr.GetBalance(callee).Cmp(calleeBefore))

			expectedZero := maximum
			if !self {
				// Preserve the separate successful transfer made earlier.
				expectedZero = limit
			}
			require.Zero(t,
				tr.GetBalance(types.ZeroAddress).Cmp(expectedZero))

			// This is an ordinary failed call, not a poisoned transition.
			recovery := tr.Call2(
				caller,
				recipient,
				nil,
				big.NewInt(1),
				gas,
			)
			require.NoError(t, recovery.Err)
			require.NoError(t, tr.Error())
			require.Zero(t, tr.GetBalance(recipient).Cmp(big.NewInt(1)))
			require.Zero(t, tr.GetBalance(caller).Cmp(
				new(big.Int).Sub(callerBefore, big.NewInt(1)),
			))
		})
	}
}

/* FINISH ALPHA CHANGE 3819 -- verify oversized balance reads fail cleanly through real state execution -- */
