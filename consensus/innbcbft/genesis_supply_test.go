/* START ALPHA CHANGE 3127 -- verify aggregate supply rejection and exact native-token initialization -- */
package innbcbft

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisSupply_HookRejectsBeforeExecution(t *testing.T) {
	config := genesisAllocationsHookConfig(t)
	maximum := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1),
	)
	config.Genesis.Alloc[types.StringToAddress("0xb501")] =
		&chain.GenesisAccount{Balance: maximum}
	config.Genesis.Alloc[types.StringToAddress("0xb502")] =
		&chain.GenesisAccount{Balance: big.NewInt(1)}

	before, err := json.Marshal(config.Genesis.Alloc)
	require.NoError(t, err)

	// Rejection must occur before accessing the transition or executing contracts.
	err = GenesisPostHookFactory(config, ConsensusName)(nil)
	require.ErrorContains(t, err,
		"initial native-token supply exceeds unsigned 256-bit range")

	after, err := json.Marshal(config.Genesis.Alloc)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestGenesisSupply_ExactMaximumInitialization(t *testing.T) {
	config := genesisAllocationsHookConfig(t)
	a := types.StringToAddress("0xb503")
	b := types.StringToAddress("0xb504")
	require.NotContains(t, config.Genesis.Alloc, a)
	require.NotContains(t, config.Genesis.Alloc, b)

	maximum := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1),
	)

	existingSupply := new(big.Int)
	for address, account := range config.Genesis.Alloc {
		if address != types.ZeroAddress && account.Balance != nil {
			existingSupply.Add(existingSupply, account.Balance)
		}
	}

	// Preserve all required system allocations while making the final
	// nonzero-address supply exactly the uint256 maximum.
	balanceA := new(big.Int).Sub(new(big.Int).Set(maximum), existingSupply)
	balanceA.Sub(balanceA, big.NewInt(1))
	require.Positive(t, balanceA.Sign())

	config.Genesis.Alloc[a] = &chain.GenesisAccount{
		Balance: balanceA,
	}
	config.Genesis.Alloc[b] = &chain.GenesisAccount{
		Balance: big.NewInt(1),
	}
	config.Genesis.Alloc[types.ZeroAddress] = &chain.GenesisAccount{
		Balance: new(big.Int).Set(maximum),
	}
	require.NoError(t, config.ValidateGenesisAllocations())

	before, err := json.Marshal(config.Genesis.Alloc)
	require.NoError(t, err)

	storage := itrie.NewMemoryStorage()
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	world := itrie.NewState(storage)
	executor := state.NewExecutor(config.Params, world, hclog.NewNullLogger())
	/* START ALPHA CHANGE 3810 -- adapt genesis supply fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) state.GetHashByNumber {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt genesis supply fixture to checked hash lookups -- */
	executor.GenesisPostHook = GenesisPostHookFactory(config, ConsensusName)

	root, err := executor.WriteGenesis(config.Genesis.Alloc, types.ZeroHash)
	require.NoError(t, err)

	snapshot, err := world.NewSnapshotAt(root)
	require.NoError(t, err)

	for address, expected := range map[types.Address]*big.Int{
		a:                 balanceA,
		b:                 big.NewInt(1),
		types.ZeroAddress: maximum,
	} {
		account, accountErr := snapshot.GetAccount(address)
		require.NoError(t, accountErr)
		require.NotNil(t, account)
		require.Zero(t, expected.Cmp(account.Balance))
	}

	transition, err := executor.BeginTxn(
		root,
		&types.Header{Number: 1, GasLimit: 10_000_000, BaseFee: 1},
		types.ZeroAddress,
	)
	require.NoError(t, err)

	input, err := contractsapi.INNBCNativeToken.Abi.Methods["totalSupply"].Encode([]any{})
	require.NoError(t, err)
	result := transition.Call2(
		a, contracts.NativeERC20TokenContract,
		input, new(big.Int), 1_000_000,
	)
	require.NotNil(t, result)
	require.NoError(t, result.Err)
	require.NoError(t, transition.Error())
	require.Len(t, result.ReturnValue, 32)
	require.Zero(t, maximum.Cmp(new(big.Int).SetBytes(result.ReturnValue)))

	after, err := json.Marshal(config.Genesis.Alloc)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

/* FINISH ALPHA CHANGE 3127 -- verify aggregate supply rejection and exact native-token initialization -- */
