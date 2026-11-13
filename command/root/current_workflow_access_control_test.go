/* START ALPHA CHANGE 3245 -- verify current deployment and transaction access-control modes -- */
package root

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

// Ordinary deployment of a small forwarding contract. Its runtime copies the
// calldata, CALLs the selected list, and returns its 32-byte role result.
// This exercises nested EVM dispatch without a compiler or genesis predeploy.
func cwListForwarder(t *testing.T, list types.Address) []byte {
	t.Helper()

	runtime := []byte{
		0x36, 0x60, 0x00, 0x60, 0x00, 0x37, // copy calldata
		0x60, 0x20, 0x60, 0x00, // output size and offset
		0x36, 0x60, 0x00, // input size and offset
		0x60, 0x00, 0x73, // zero value, PUSH20 destination
	}
	runtime = append(runtime, list.Bytes()...)
	runtime = append(runtime,
		0x5a, 0xf1, 0x50, // GAS, CALL, discard success flag
		0x60, 0x20, 0x60, 0x00, 0xf3, // return 32 bytes
	)
	require.LessOrEqual(t, len(runtime), 255)

	length := byte(len(runtime))
	init := []byte{
		0x60, length, 0x60, 0x0c, 0x60, 0x00, 0x39,
		0x60, length, 0x60, 0x00, 0xf3,
	}

	return append(init, runtime...)
}

func TestCurrentWorkflow_AccessControlModes(t *testing.T) {
	cases := []struct {
		name       string
		flagPrefix string
		list       types.Address
		deployment bool
		allow      bool
	}{
		{
			"deployment_allow", "contract-deployer-allow-list",
			contracts.AllowListContractsAddr, true, true,
		},
		{
			"deployment_block", "contract-deployer-block-list",
			contracts.BlockListContractsAddr, true, false,
		},
		{
			"transactions_allow", "transactions-allow-list",
			contracts.AllowListTransactionsAddr, false, true,
		},
		{
			"transactions_block", "transactions-block-list",
			contracts.BlockListTransactionsAddr, false, false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			/* START ALPHA CHANGE 3250 -- configure the zero caller used by directory and RPC reads -- */
			f, keys, accounts := cwScenario(t, 4, 1800,
				func(addresses [3]types.Address) []string {
					args := []string{
						"--" + tc.flagPrefix + "-admin", addresses[0].String(),
						"--" + tc.flagPrefix + "-enabled", addresses[2].String(),
					}
					if !tc.deployment && tc.allow {
						// The directory performs registry eth_call requests
						// without a signing account. Permit its zero caller.
						args = append(args,
							"--"+tc.flagPrefix+"-enabled",
							types.ZeroAddress.String(),
						)
					}

					return args
				},
			)
			/* FINISH ALPHA CHANGE 3250 -- configure the zero caller used by directory and RPC reads -- */
			node := f.nodes[0]
			recipient := types.BytesToAddress([]byte{0xa8, 0x01})

			role := func(address types.Address) string {
				input := cwABI(t,
					"function readAddressList(address) returns (uint256)", address)
				return cwQuantity(t, cwCall(
					t, node, tc.list, input, "latest",
				)).String()
			}

			/* START ALPHA CHANGE 3250 -- verify explicit RPC caller permissions without enabling unlisted accounts -- */
			require.Equal(t, "2", role(accounts[0]))
			require.Equal(t, "0", role(accounts[1]))
			require.Equal(t, "1", role(accounts[2]))

			if !tc.deployment && tc.allow {
				require.Equal(t, "1", role(types.ZeroAddress))

				input := crypto.Keccak256([]byte("feeBurnRateBps()"))[:4]
				require.NotEmpty(t, cwCall(t, node,
					contracts.INNBCBlockRewardGovernanceContract,
					input, "latest"))

				var result string
				err := cwRPC(node.rpc, "eth_call", &result, map[string]any{
					"from": accounts[1].String(),
					"to":   contracts.INNBCBlockRewardGovernanceContract.String(),
					"data": cwHex(input),
				}, "latest")
				require.ErrorContains(t, err, "not in allow list")
			}
			/* FINISH ALPHA CHANGE 3250 -- verify explicit RPC caller permissions without enabling unlisted accounts -- */

			operation := func(index int, permitted bool) {
				t.Helper()

				tx := &types.Transaction{}
				if tc.deployment {
					tx.Input = append([]byte(nil), contractsapi.TestSimple.Bytecode...)
				} else {
					tx.To = recipient.Ptr()
					tx.Value = big.NewInt(1)
					tx.Gas = 21_000
				}

				receipt := f.send(t, keys[index], tx)
				if permitted {
					require.Equal(t, "0x1", receipt.Status)
				} else {
					require.Equal(t, "0x0", receipt.Status)
				}

				if tc.deployment {
					expected := crypto.CreateAddress(accounts[index], tx.Nonce)
					code := cwRead[string](
						t, node, "eth_getCode", expected.String(), "latest",
					)
					if permitted {
						require.Equal(t, expected.String(), receipt.ContractAddress)
						require.Equal(t, cwHex(contractsapi.TestSimple.DeployedBytecode), code)
					} else {
						require.Equal(t, "0x", code)
					}
				}
			}

			operation(0, true)
			operation(1, !tc.allow)
			operation(2, tc.allow)

			// Deployment lists must not restrict an ordinary value transfer.
			if tc.deployment {
				receipt := f.send(t, keys[1], &types.Transaction{
					To: recipient.Ptr(), Value: big.NewInt(1), Gas: 21_000,
				})
				require.Equal(t, "0x1", receipt.Status)
			}

			change := func(index int, method string, address types.Address) cwReceipt {
				t.Helper()
				return f.send(t, keys[index], &types.Transaction{
					To:    tc.list.Ptr(),
					Input: cwABI(t, "function "+method+"(address)", address),
				})
			}

			granted := change(0, "setEnabled", accounts[1])
			require.Equal(t, "0x1", granted.Status)
			require.Equal(t, "1", role(accounts[1]))
			operation(1, tc.allow)

			if tc.deployment {
				receipt := f.send(t, keys[1], &types.Transaction{
					To: recipient.Ptr(), Value: big.NewInt(1), Gas: 21_000,
				})
				require.Equal(t, "0x1", receipt.Status)
			}

			unauthorized := change(1, "setEnabled", recipient)
			require.Equal(t, "0x0", unauthorized.Status)
			require.Equal(t, "0", role(recipient))

			selfRemoval := change(0, "setNone", accounts[0])
			require.Equal(t, "0x0", selfRemoval.Status)
			require.Equal(t, "2", role(accounts[0]))

			removed := change(0, "setNone", accounts[1])
			require.Equal(t, "0x1", removed.Status)
			require.Equal(t, "0", role(accounts[1]))
			operation(1, !tc.allow)

			forwarder := f.send(t, keys[0], &types.Transaction{
				Input: cwListForwarder(t, tc.list),
			})
			require.Equal(t, "0x1", forwarder.Status)
			require.NotEmpty(t, forwarder.ContractAddress)

			result := cwRead[string](t, node, "eth_call", map[string]any{
				"from": accounts[0].String(),
				"to":   forwarder.ContractAddress,
				"data": cwHex(cwABI(t,
					"function readAddressList(address) returns (uint256)", accounts[0])),
			}, "latest")
			require.Equal(t, cwWord(big.NewInt(2)), result)

			require.Equal(t, "1", role(accounts[2]))
			cwAssertScenarioPreserved(t, f)
		})
	}
}

/* FINISH ALPHA CHANGE 3245 -- verify current deployment and transaction access-control modes -- */
