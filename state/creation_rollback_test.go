/* START ALPHA CHANGE 3036 -- verify creation rollback and existing failure semantics -- */
package state

import (
	"errors"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/addresslist"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestCreationRollback_EarlyFailures(t *testing.T) {
	for _, policy := range []string{"insufficient_value", "allowlist", "blocklist"} {
		t.Run(policy, func(t *testing.T) {
			for _, prefunded := range []bool{false, true} {
				name := "new_destination"
				if prefunded {
					name = "prefunded_destination"
				}

				t.Run(name, func(t *testing.T) {
					_, _, tr, _ := issue7AExecutionFixture(t, nil)
					destination := crypto.CreateAddress(issue7ASender, 0)

					balance := int64(1000)
					if policy == "insufficient_value" {
						balance = 10
					}
					tr.state.SetBalance(issue7ASender, big.NewInt(balance))

					originalDestinationBalance := int64(0)
					if prefunded {
						originalDestinationBalance = 55
						tr.state.SetBalance(destination, big.NewInt(55))
					}

					switch policy {
					case "allowlist":
						tr.deploymentAllowList = addresslist.NewAddressList(
							tr, contracts.AllowListContractsAddr,
						)
					case "blocklist":
						tr.deploymentBlockList = addresslist.NewAddressList(
							tr, contracts.BlockListContractsAddr,
						)
						tr.deploymentBlockList.SetRole(
							issue7ASender, addresslist.EnabledRole,
						)
					}

					const gas = uint64(100_000)
					result := tr.Create2(
						issue7ASender, []byte{0x00}, big.NewInt(11), gas,
					)
					require.NotNil(t, result)

					if policy == "insufficient_value" {
						require.ErrorIs(t, result.Err, runtime.ErrInsufficientBalance)
						require.Equal(t, gas, result.GasLeft)
					} else {
						require.ErrorIs(t, result.Err, runtime.ErrNotAuth)
						require.Zero(t, result.GasLeft)
					}

					require.NoError(t, tr.Error())
					require.Equal(t, uint64(1), tr.state.GetNonce(issue7ASender))
					require.Zero(t, tr.state.GetBalance(issue7ASender).Cmp(
						big.NewInt(balance),
					))
					require.Equal(t, prefunded, tr.state.Exist(destination))
					require.Zero(t, tr.state.GetNonce(destination))
					require.Empty(t, tr.state.GetCode(destination))
					require.Zero(t, tr.state.GetBalance(destination).Cmp(
						big.NewInt(originalDestinationBalance),
					))
				})
			}
		})
	}
}

func TestCreationRollback_ExistingExecutionSemantics(t *testing.T) {
	tests := []struct {
		name      string
		code      []byte
		gas       uint64
		wantErr   error
		collision bool
		legacy    bool
		keepValue bool
		wantCode  []byte
	}{
		{
			name:      "successful_creation",
			code:      []byte{0x60, 0x01, 0x60, 0x00, 0xf3},
			gas:       100_000,
			keepValue: true,
			wantCode:  []byte{0x00},
		},
		{
			name:    "constructor_revert",
			code:    []byte{0x60, 0x00, 0x60, 0x00, 0xfd},
			gas:     100_000,
			wantErr: runtime.ErrExecutionReverted,
		},
		{
			name:      "address_collision",
			code:      []byte{0x00},
			gas:       100_000,
			wantErr:   runtime.ErrContractAddressCollision,
			collision: true,
		},
		{
			name:    "maximum_code_size",
			code:    []byte{0x61, 0x60, 0x01, 0x60, 0x00, 0xf3},
			gas:     100_000,
			wantErr: runtime.ErrMaxCodeSizeExceeded,
		},
		{
			name:    "homestead_code_deposit_failure",
			code:    []byte{0x60, 0x01, 0x60, 0x00, 0xf3},
			gas:     100,
			wantErr: runtime.ErrCodeStoreOutOfGas,
		},
		{
			name:      "pre_homestead_code_deposit_exception",
			code:      []byte{0x60, 0x01, 0x60, 0x00, 0xf3},
			gas:       100,
			wantErr:   runtime.ErrCodeStoreOutOfGas,
			legacy:    true,
			keepValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, tr, _ := issue7AExecutionFixture(t, nil)
			tr.state.SetBalance(issue7ASender, big.NewInt(1000))
			destination := crypto.CreateAddress(issue7ASender, 0)

			if tt.legacy {
				tr.config = chain.ForksInTime{}
			}
			if tt.collision {
				tr.state.SetBalance(destination, big.NewInt(55))
				tr.state.SetNonce(destination, 2)
			}

			result := tr.Create2(
				issue7ASender, tt.code, big.NewInt(11), tt.gas,
			)
			require.NotNil(t, result)
			if tt.wantErr == nil {
				require.NoError(t, result.Err)
			} else {
				require.ErrorIs(t, result.Err, tt.wantErr)
			}
			require.NoError(t, tr.Error())
			require.Equal(t, uint64(1), tr.state.GetNonce(issue7ASender))

			senderBalance := int64(1000)
			destinationBalance := int64(0)
			destinationNonce := uint64(0)
			exists := false

			if tt.keepValue {
				senderBalance -= 11
				destinationBalance = 11
				exists = true
				if !tt.legacy {
					destinationNonce = 1
				}
			}
			if tt.collision {
				destinationBalance = 55
				destinationNonce = 2
				exists = true
			}

			require.Zero(t, tr.state.GetBalance(issue7ASender).Cmp(
				big.NewInt(senderBalance),
			))
			require.Equal(t, exists, tr.state.Exist(destination))
			require.Equal(t, destinationNonce, tr.state.GetNonce(destination))
			require.Zero(t, tr.state.GetBalance(destination).Cmp(
				big.NewInt(destinationBalance),
			))
			/* START ALPHA CHANGE 3038 -- compare absent bytecode independently of slice representation -- */
			actualCode := tr.state.GetCode(destination)
			require.NoError(t, tr.Error())
			if len(tt.wantCode) == 0 {
				require.Empty(t, actualCode)
			} else {
				require.Equal(t, tt.wantCode, actualCode)
			}
			/* FINISH ALPHA CHANGE 3038 -- compare absent bytecode independently of slice representation -- */

			switch tt.name {
			case "successful_creation", "constructor_revert",
				"pre_homestead_code_deposit_exception":
				require.Greater(t, result.GasLeft, uint64(0))
				require.Less(t, result.GasLeft, tt.gas)
			default:
				require.Zero(t, result.GasLeft)
			}
		})
	}
}

func TestCreationRollback_PolicyReadFailure(t *testing.T) {
	for _, policy := range []string{"allowlist", "blocklist"} {
		t.Run(policy, func(t *testing.T) {
			_, backend, tr, _ := issue7AExecutionFixture(t, nil)
			tr.state.SetBalance(issue7ASender, big.NewInt(1000))
			destination := crypto.CreateAddress(issue7ASender, 0)

			listAddress := contracts.AllowListContractsAddr
			if policy == "blocklist" {
				listAddress = contracts.BlockListContractsAddr
				tr.deploymentBlockList = addresslist.NewAddressList(tr, listAddress)
			} else {
				tr.deploymentAllowList = addresslist.NewAddressList(tr, listAddress)
			}

			cause := errors.New("injected deployment policy storage failure")
			backend.snapshot.accounts[listAddress] = &Account{
				Balance:  new(big.Int),
				Root:     types.StringToHash("0x1234"),
				CodeHash: types.EmptyCodeHash.Bytes(),
			}
			backend.snapshot.storageErr = cause

			result := tr.Create2(
				issue7ASender, []byte{0x00}, big.NewInt(11), 100_000,
			)
			require.ErrorIs(t, result.Err, ErrLocalStateFailure)
			require.ErrorIs(t, result.Err, cause)
			require.ErrorIs(t, tr.Error(), cause)

			require.Zero(t, tr.state.GetBalance(issue7ASender).Cmp(big.NewInt(1000)))
			require.False(t, tr.state.Exist(destination))

			snapshot, root, err := tr.Commit()
			require.Nil(t, snapshot)
			require.Equal(t, types.ZeroHash, root)
			require.ErrorIs(t, err, cause)
			require.Zero(t, backend.snapshot.commits)
			require.Empty(t, tr.Receipts())
		})
	}
}

func TestCreationRollback_RollbackFailureIsRetained(t *testing.T) {
	for _, earlierFailure := range []bool{false, true} {
		name := "rollback_failure"
		if earlierFailure {
			name = "earlier_local_failure_wins"
		}

		t.Run(name, func(t *testing.T) {
			_, backend, tr, tx := issue7AExecutionFixture(t, nil)
			snapshot := tr.state.Snapshot()

			cause := errors.New("earlier retained local failure")
			if earlierFailure {
				tr.state.setError(cause)
			}

			result := tr.revertCreation(snapshot+1, &runtime.ExecutionResult{
				GasLeft: 123,
				Err:     runtime.ErrNotAuth,
			})
			require.ErrorIs(t, result.Err, ErrLocalStateFailure)
			require.Same(t, tr.Error(), result.Err)
			require.NotErrorIs(t, result.Err, runtime.ErrNotAuth)

			if earlierFailure {
				require.ErrorIs(t, result.Err, cause)
			} else {
				require.ErrorContains(t, result.Err, "snapshot id")
			}

			require.ErrorIs(t, tr.Write(tx), ErrLocalStateFailure)
			require.Empty(t, tr.Receipts())
			require.Zero(t, tr.TotalGas())

			committed, root, err := tr.Commit()
			require.Nil(t, committed)
			require.Equal(t, types.ZeroHash, root)
			require.ErrorIs(t, err, ErrLocalStateFailure)
			require.Zero(t, backend.snapshot.commits)
		})
	}
}

/* FINISH ALPHA CHANGE 3036 -- verify creation rollback and existing failure semantics -- */
