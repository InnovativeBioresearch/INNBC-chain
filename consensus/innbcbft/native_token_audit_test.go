/* START ALPHA CHANGE 2404 -- test native token and burn vault through the INNBC execution engine -- */
package innbcbft

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
)

func TestContractAudit_NativeTokenAndBurnVault(t *testing.T) {
	owner := types.StringToAddress("0x70001")
	recipient := types.StringToAddress("0x70002")
	spender := types.StringToAddress("0x70003")
	issuer := types.StringToAddress("0x70004")
	token := contracts.NativeERC20TokenContract
	vault := contracts.INNBCFeeBurnVaultContract

	tr := newTestTransition(t, map[types.Address]*chain.GenesisAccount{
		owner: {
			Balance: big.NewInt(1000),
		},
		token: {
			Code: contractsapi.INNBCNativeToken.DeployedBytecode,
		},
		vault: {
			Code: contractsapi.INNBCFeeBurnVault.DeployedBytecode,
		},
	})

	call := func(
		caller types.Address,
		target types.Address,
		method string,
		args ...any,
	) *runtime.ExecutionResult {
		t.Helper()

		abiMethod := contractsapi.INNBCNativeToken.Abi.Methods[method]
		if target == vault {
			abiMethod = contractsapi.INNBCFeeBurnVault.Abi.Methods[method]
		}
		require.NotNil(t, abiMethod, "missing method %s", method)

		input, err := abiMethod.Encode(args)
		require.NoError(t, err)

		result := tr.Call2(caller, target, input, big.NewInt(0), 10_000_000)
		require.NotNil(t, result)
		require.NoError(t, tr.Error())

		return result
	}

	readUint := func(method string, args ...any) *big.Int {
		t.Helper()

		result := call(owner, token, method, args...)
		require.NoError(t, result.Err)
		require.Len(t, result.ReturnValue, 32)

		return new(big.Int).SetBytes(result.ReturnValue)
	}

	checkBalance := func(address types.Address, expected int64) {
		t.Helper()

		actual := tr.GetBalance(address)
		require.NoError(t, tr.Error())
		require.Zero(t, actual.Cmp(big.NewInt(expected)))
	}

	initArgs := []any{
		types.ZeroAddress,
		issuer,
		types.ZeroAddress,
		"INNBC",
		"INNBC",
		uint8(18),
		big.NewInt(1000),
	}

	result := call(owner, token, "initialize", initArgs...)
	require.ErrorIs(t, result.Err, runtime.ErrExecutionReverted)

	result = call(contracts.SystemCaller, token, "initialize", initArgs...)
	require.NoError(t, result.Err)
	require.Zero(t, readUint("totalSupply").Cmp(big.NewInt(1000)))

	result = call(contracts.SystemCaller, token, "initialize", initArgs...)
	require.ErrorIs(t, result.Err, runtime.ErrExecutionReverted)
	require.Zero(t, readUint("totalSupply").Cmp(big.NewInt(1000)))

	result = call(owner, token, "transfer", recipient, big.NewInt(100))
	require.NoError(t, result.Err)
	checkBalance(owner, 900)
	checkBalance(recipient, 100)

	result = call(owner, token, "approve", spender, big.NewInt(1200))
	require.NoError(t, result.Err)

	/*
		Allowance is sufficient but the owner's native balance is not.
		The failed precompile call must also roll back allowance spending.
	*/
	result = call(
		spender, token, "transferFrom",
		owner, recipient, big.NewInt(1001),
	)
	require.ErrorIs(t, result.Err, runtime.ErrExecutionReverted)
	require.Zero(t, readUint("allowance", owner, spender).Cmp(big.NewInt(1200)))
	checkBalance(owner, 900)
	checkBalance(recipient, 100)

	result = call(
		spender, token, "transferFrom",
		owner, recipient, big.NewInt(100),
	)
	require.NoError(t, result.Err)
	require.Zero(t, readUint("allowance", owner, spender).Cmp(big.NewInt(1100)))
	checkBalance(owner, 800)
	checkBalance(recipient, 200)

	result = call(owner, token, "issueEmergencyBlockReward", recipient, big.NewInt(50))
	require.ErrorIs(t, result.Err, runtime.ErrExecutionReverted)
	checkBalance(recipient, 200)
	require.Zero(t, readUint("totalSupply").Cmp(big.NewInt(1000)))

	result = call(issuer, token, "issueEmergencyBlockReward", recipient, big.NewInt(50))
	require.NoError(t, result.Err)
	checkBalance(recipient, 250)
	require.Zero(t, readUint("totalSupply").Cmp(big.NewInt(1050)))

	/*
		A failed burn must restore the supply decrement performed before
		the native-transfer precompile rejects insufficient funds.
	*/
	result = call(recipient, token, "burn", big.NewInt(251))
	require.ErrorIs(t, result.Err, runtime.ErrExecutionReverted)
	checkBalance(recipient, 250)
	require.Zero(t, readUint("totalSupply").Cmp(big.NewInt(1050)))

	result = call(recipient, token, "burn", big.NewInt(25))
	require.NoError(t, result.Err)
	checkBalance(recipient, 225)
	require.Zero(t, readUint("totalSupply").Cmp(big.NewInt(1025)))

	result = call(owner, vault, "initialize", token)
	require.NoError(t, result.Err)

	result = tr.Call2(owner, vault, nil, big.NewInt(200), 10_000_000)
	require.NotNil(t, result)
	require.NoError(t, result.Err)
	require.NoError(t, tr.Error())
	checkBalance(owner, 600)
	checkBalance(vault, 200)

	result = call(spender, vault, "burn")
	require.NoError(t, result.Err)
	checkBalance(vault, 0)
	require.Zero(t, readUint("totalSupply").Cmp(big.NewInt(825)))

	result = call(spender, vault, "burn")
	require.ErrorIs(t, result.Err, runtime.ErrExecutionReverted)
	require.Zero(t, readUint("totalSupply").Cmp(big.NewInt(825)))
}

/* FINISH ALPHA CHANGE 2404 -- test native token and burn vault through the INNBC execution engine -- */
