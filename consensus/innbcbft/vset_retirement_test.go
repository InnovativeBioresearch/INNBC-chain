/* START ALPHA CHANGE 3427 -- verify epoch functionality and retired VSET selectors in the client EVM -- */
package innbcbft

import (
	"math/big"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type vsetRetirementFixture struct {
	executor  *state.Executor
	root      types.Hash
	timestamp uint64
}

func newVSETRetirementFixture(t *testing.T) *vsetRetirementFixture {
	t.Helper()

	config := genesisAllocationsHookConfig(t)
	storage := itrie.NewMemoryStorage()
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	executor := state.NewExecutor(
		config.Params,
		itrie.NewState(storage),
		hclog.NewNullLogger(),
	)
	/* START ALPHA CHANGE 3810 -- adapt validator-set fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) state.GetHashByNumber {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt validator-set fixture to checked hash lookups -- */
	executor.GenesisPostHook = GenesisPostHookFactory(config, ConsensusName)

	root, err := executor.WriteGenesis(config.Genesis.Alloc, types.ZeroHash)
	require.NoError(t, err)

	return &vsetRetirementFixture{
		executor:  executor,
		root:      root,
		timestamp: config.Genesis.Timestamp + 2,
	}
}

func (f *vsetRetirementFixture) transition(t *testing.T) *state.Transition {
	t.Helper()

	transition, err := f.executor.BeginTxn(
		f.root,
		&types.Header{
			Number:    1,
			Timestamp: f.timestamp,
			GasLimit:  10_000_000,
			BaseFee:   1,
		},
		types.ZeroAddress,
	)
	require.NoError(t, err)

	return transition
}

func vsetRetirementCall(
	t *testing.T,
	transition *state.Transition,
	caller, target types.Address,
	name string,
	args ...any,
) *runtime.ExecutionResult {
	t.Helper()

	method := contractsapi.ValidatorSet.Abi.GetMethod(name)
	require.NotNil(t, method, "missing ValidatorSet method %s", name)
	input, err := method.Encode(args)
	require.NoError(t, err)

	result := transition.Call2(
		caller, target, input, new(big.Int), 1_000_000,
	)
	require.NotNil(t, result)
	require.NoError(t, transition.Error())

	return result
}

func vsetRetirementUint(
	t *testing.T,
	transition *state.Transition,
	target types.Address,
	name string,
	args ...any,
) uint64 {
	t.Helper()

	result := vsetRetirementCall(
		t, transition, types.StringToAddress("0xa501"),
		target, name, args...,
	)
	require.NoError(t, result.Err)
	require.Len(t, result.ReturnValue, 32)

	value := new(big.Int).SetBytes(result.ReturnValue)
	require.True(t, value.IsUint64())

	return value.Uint64()
}

func vsetRetirementCommit(
	t *testing.T,
	transition *state.Transition,
	caller types.Address,
	id, start, end uint64,
	root types.Hash,
) *runtime.ExecutionResult {
	t.Helper()

	input, err := (&contractsapi.CommitEpochValidatorSetFn{
		ID: new(big.Int).SetUint64(id),
		Epoch: &contractsapi.Epoch{
			StartBlock: new(big.Int).SetUint64(start),
			EndBlock:   new(big.Int).SetUint64(end),
			EpochRoot:  root,
		},
	}).EncodeAbi()
	require.NoError(t, err)

	result := transition.Call2(
		caller, contracts.ValidatorSetContract,
		input, new(big.Int), 1_000_000,
	)
	require.NotNil(t, result)
	require.NoError(t, transition.Error())

	return result
}

func TestVSETRetirement_GenesisAndEpochCommitments(t *testing.T) {
	fixture := newVSETRetirementFixture(t)
	transition := fixture.transition(t)
	proxy := contracts.ValidatorSetContract

	require.Equal(t, uint64(2),
		vsetRetirementUint(t, transition, proxy, "EPOCH_SIZE"))
	require.Equal(t, uint64(1),
		vsetRetirementUint(t, transition, proxy, "currentEpochId"))
	require.Zero(t,
		vsetRetirementUint(t, transition, proxy, "epochEndBlocks", big.NewInt(0)))
	require.Zero(t,
		vsetRetirementUint(t, transition, proxy, "totalBlocks", big.NewInt(1)))

	roots := []types.Hash{
		types.StringToHash("0x1234"),
		types.StringToHash("0x5678"),
	}

	for i, root := range roots {
		id := uint64(i + 1)
		start := uint64(2*i + 1)
		end := start + 1

		result := vsetRetirementCommit(
			t, transition, contracts.SystemCaller,
			id, start, end, root,
		)
		require.NoError(t, result.Err)
		require.Equal(t, id+1,
			vsetRetirementUint(t, transition, proxy, "currentEpochId"))
		require.Equal(t, end,
			vsetRetirementUint(t, transition, proxy, "epochEndBlocks",
				new(big.Int).SetUint64(id)))
		require.Equal(t, uint64(2),
			vsetRetirementUint(t, transition, proxy, "totalBlocks",
				new(big.Int).SetUint64(id)))

		stored := vsetRetirementCall(
			t, transition, types.StringToAddress("0xa501"),
			proxy, "epochs", new(big.Int).SetUint64(id),
		)
		require.NoError(t, stored.Err)
		require.Len(t, stored.ReturnValue, 96)
		require.Equal(t, start,
			new(big.Int).SetBytes(stored.ReturnValue[:32]).Uint64())
		require.Equal(t, end,
			new(big.Int).SetBytes(stored.ReturnValue[32:64]).Uint64())
		require.Equal(t, root,
			types.BytesToHash(stored.ReturnValue[64:96]))
	}

	logs, err := transition.FinalizeTransaction()
	require.NoError(t, err)
	require.Len(t, logs, 2)

	eventID := crypto.Keccak256Hash(
		[]byte("NewEpoch(uint256,uint256,uint256,bytes32)"),
	)
	for i, log := range logs {
		require.Equal(t, proxy, log.Address)
		require.Len(t, log.Topics, 4)
		require.Equal(t, eventID, log.Topics[0])
		require.Equal(t, uint64(i+1),
			new(big.Int).SetBytes(log.Topics[1][:]).Uint64())
		require.Equal(t, uint64(2*i+1),
			new(big.Int).SetBytes(log.Topics[2][:]).Uint64())
		require.Equal(t, uint64(2*i+2),
			new(big.Int).SetBytes(log.Topics[3][:]).Uint64())
		require.Equal(t, roots[i][:], log.Data)
	}

	_, root, err := transition.Commit()
	require.NoError(t, err)
	fixture.root = root

	reopened := fixture.transition(t)
	require.Equal(t, uint64(3),
		vsetRetirementUint(t, reopened, proxy, "currentEpochId"))
	require.Equal(t, uint64(4),
		vsetRetirementUint(t, reopened, proxy, "epochEndBlocks", big.NewInt(2)))
}

func TestVSETRetirement_RejectsInvalidEpochs(t *testing.T) {
	fixture := newVSETRetirementFixture(t)
	outsider := types.StringToAddress("0xa501")

	for _, tc := range []struct {
		name       string
		caller     types.Address
		id         uint64
		start, end uint64
	}{
		{"unauthorized", outsider, 1, 1, 2},
		{"unexpected_id", contracts.SystemCaller, 2, 1, 2},
		{"empty_range", contracts.SystemCaller, 1, 1, 1},
		{"reversed_range", contracts.SystemCaller, 1, 2, 1},
		{"not_divisible", contracts.SystemCaller, 1, 1, 3},
		{"noncontiguous_start", contracts.SystemCaller, 1, 2, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transition := fixture.transition(t)

			result := vsetRetirementCommit(
				t, transition, tc.caller,
				tc.id, tc.start, tc.end, types.ZeroHash,
			)
			require.Error(t, result.Err)

			require.Equal(t, uint64(1),
				vsetRetirementUint(t, transition,
					contracts.ValidatorSetContract, "currentEpochId"))
			require.Zero(t,
				vsetRetirementUint(t, transition,
					contracts.ValidatorSetContract, "totalBlocks", big.NewInt(1)))
		})
	}
}

func TestVSETRetirement_Initialization(t *testing.T) {
	fixture := newVSETRetirementFixture(t)
	outsider := types.StringToAddress("0xa501")
	proxy := contracts.ValidatorSetContract
	implementation := contracts.ValidatorSetContractV1

	for _, target := range []types.Address{proxy, implementation} {
		transition := fixture.transition(t)
		result := vsetRetirementCall(
			t, transition, outsider, target, "initialize", big.NewInt(7),
		)
		require.Error(t, result.Err)

		require.Equal(t, uint64(2),
			vsetRetirementUint(t, transition, proxy, "EPOCH_SIZE"))
		require.Zero(t,
			vsetRetirementUint(t, transition, implementation, "EPOCH_SIZE"))
	}

	transition := fixture.transition(t)

	// Genesis already initialized the proxy. Even the system cannot repeat it.
	result := vsetRetirementCall(
		t, transition, contracts.SystemCaller,
		proxy, "initialize", big.NewInt(7),
	)
	require.Error(t, result.Err)

	/* START ALPHA CHANGE 3448 -- verify the embedded initializer rejects unsupported epoch sizes -- */
	// Runtime allocation bypasses the implementation constructor. Its separate
	// storage is still protected by onlySystemCall and one-time initialization.
	for _, epochSize := range []int64{0, 1} {
		result = vsetRetirementCall(
			t, transition, contracts.SystemCaller,
			implementation, "initialize", big.NewInt(epochSize),
		)
		require.True(t, result.Reverted(),
			"epoch size %d must revert", epochSize)
		require.Zero(t,
			vsetRetirementUint(t, transition, implementation, "EPOCH_SIZE"))
		require.Zero(t,
			vsetRetirementUint(t, transition, implementation, "currentEpochId"))
	}
	/* FINISH ALPHA CHANGE 3448 -- verify the embedded initializer rejects unsupported epoch sizes -- */

	result = vsetRetirementCall(
		t, transition, contracts.SystemCaller,
		implementation, "initialize", big.NewInt(7),
	)
	require.NoError(t, result.Err)
	require.Equal(t, uint64(7),
		vsetRetirementUint(t, transition, implementation, "EPOCH_SIZE"))
	require.Equal(t, uint64(2),
		vsetRetirementUint(t, transition, proxy, "EPOCH_SIZE"))
	require.Equal(t, uint64(1),
		vsetRetirementUint(t, transition, proxy, "currentEpochId"))

	result = vsetRetirementCall(
		t, transition, contracts.SystemCaller,
		implementation, "initialize", big.NewInt(8),
	)
	require.Error(t, result.Err)
	require.Equal(t, uint64(7),
		vsetRetirementUint(t, transition, implementation, "EPOCH_SIZE"))
}

func TestVSETRetirement_RetiredABIAndSelectors(t *testing.T) {
	require.NotNil(t, contractsapi.ValidatorSet)
	require.NotEmpty(t, contractsapi.ValidatorSet.DeployedBytecode)

	method := contractsapi.ValidatorSet.Abi.GetMethod("initialize")
	require.NotNil(t, method)
	require.Equal(t,
		crypto.Keccak256([]byte("initialize(uint256)"))[:4],
		method.ID(),
	)

	input, err := (&contractsapi.InitializeValidatorSetFn{
		NewEpochSize: big.NewInt(2),
	}).EncodeAbi()
	require.NoError(t, err)
	require.Len(t, input, 36)
	require.Equal(t, method.ID(), input[:4])
	require.Equal(t, uint64(2),
		new(big.Int).SetBytes(input[4:]).Uint64())

	for _, event := range []string{
		"Transfer", "Approval", "Snapshot",
		"WithdrawalRegistered", "Withdrawal",
	} {
		require.NotContains(t, contractsapi.ValidatorSet.Abi.Events, event)
	}
	require.Contains(t, contractsapi.ValidatorSet.Abi.Events, "NewEpoch")

	// These belong to active native-token and registry functionality.
	require.NotNil(t,
		contractsapi.INNBCValidatorRegistry.Abi.GetMethod("registerValidator"))
	require.NotNil(t,
		contractsapi.INNBCValidatorRegistry.Abi.GetMethod("unstake"))
	require.NotNil(t,
		contractsapi.INNBCNativeToken.Abi.GetMethod("totalSupply"))
	require.Contains(t, contractsapi.INNBCNativeToken.Abi.Events, "Transfer")

	fixture := newVSETRetirementFixture(t)
	signatures := []string{
		"name()",
		"symbol()",
		"decimals()",
		"totalSupply()",
		"balanceOf(address)",
		"transfer(address,uint256)",
		"allowance(address,address)",
		"approve(address,uint256)",
		"transferFrom(address,address,uint256)",
		"increaseAllowance(address,uint256)",
		"decreaseAllowance(address,uint256)",
		"balanceOfAt(address,uint256)",
		"totalSupplyAt(uint256)",
		"unstake(uint256)",
		"withdraw()",
		"withdrawable(address)",
		"pendingWithdrawals(address)",
		"WITHDRAWAL_WAIT_PERIOD()",
	}

	for _, signature := range signatures {
		name := strings.SplitN(signature, "(", 2)[0]
		require.Nil(t, contractsapi.ValidatorSet.Abi.GetMethod(name))
	}

	signatures = append(signatures, "initialize(uint256,(address,uint256)[])")

	for _, signature := range signatures {
		for _, target := range []types.Address{
			contracts.ValidatorSetContract,
			contracts.ValidatorSetContractV1,
		} {
			t.Run(signature+"/"+target.String(), func(t *testing.T) {
				transition := fixture.transition(t)
				input := make([]byte, 4+96)
				copy(input[:4], crypto.Keccak256([]byte(signature))[:4])

				if signature == "initialize(uint256,(address,uint256)[])" {
					// Old ABI: epoch size 2, array offset 64, empty array.
					input[35] = 2
					input[67] = 64
				} else {
					input[35] = 1
					input[67] = 1
					input[99] = 1
				}

				result := transition.Call2(
					types.StringToAddress("0xa501"),
					target, input, new(big.Int), 1_000_000,
				)
				require.NotNil(t, result)
				require.Error(t, result.Err)
				require.Empty(t, result.ReturnValue)
				require.NoError(t, transition.Error())
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3427 -- verify epoch functionality and retired VSET selectors in the client EVM -- */
