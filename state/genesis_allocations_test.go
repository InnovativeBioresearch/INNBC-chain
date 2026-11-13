/* START ALPHA CHANGE 3109 -- test direct genesis validation and omitted-balance state semantics -- */
package state_test

import (
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type genesisAllocationCountingState struct {
	statepkg.State
	opens int
}

func (s *genesisAllocationCountingState) NewSnapshot() statepkg.Snapshot {
	s.opens++
	return s.State.NewSnapshot()
}

func (s *genesisAllocationCountingState) NewSnapshotAt(root types.Hash) (statepkg.Snapshot, error) {
	s.opens++
	return s.State.NewSnapshotAt(root)
}

func TestGenesisAllocations_ExecutorRejectsBeforeStateAccess(t *testing.T) {
	address := types.StringToAddress("0xa301")

	for _, tc := range []struct {
		name    string
		account *chain.GenesisAccount
		wantErr string
	}{
		{"null_account", nil, "account must not be null"},
		{
			"negative_balance",
			&chain.GenesisAccount{Balance: big.NewInt(-1)},
			"balance must be nonnegative",
		},
		{
			"oversized_balance",
			&chain.GenesisAccount{Balance: new(big.Int).Lsh(big.NewInt(1), 256)},
			"balance exceeds unsigned 256-bit range",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storage := itrie.NewMemoryStorage()
			t.Cleanup(func() { require.NoError(t, storage.Close()) })
			backend := &genesisAllocationCountingState{State: itrie.NewState(storage)}
			executor := statepkg.NewExecutor(
				&chain.Params{ChainID: 1982, Forks: chain.AllForksEnabled.Copy()},
				backend, hclog.NewNullLogger(),
			)
			hooks := 0
			executor.GenesisPostHook = func(*statepkg.Transition) error {
				hooks++
				return nil
			}

			for _, initialRoot := range []types.Hash{
				types.ZeroHash, types.StringToHash("0x01"),
			} {
				root, err := executor.WriteGenesis(
					map[types.Address]*chain.GenesisAccount{address: tc.account},
					initialRoot,
				)
				require.Equal(t, types.ZeroHash, root)
				require.ErrorContains(t, err, tc.wantErr)
				require.ErrorContains(t, err, address.String())
				require.False(t, errors.Is(err, statepkg.ErrLocalStateFailure))
				require.Zero(t, backend.opens)
				require.Zero(t, hooks)
			}
		})
	}
}

func TestGenesisAllocations_ExecutorPreservesOmittedBalanceSemantics(t *testing.T) {
	storage := itrie.NewMemoryStorage()
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	world := itrie.NewState(storage)
	executor := statepkg.NewExecutor(
		&chain.Params{ChainID: 1982, Forks: chain.AllForksEnabled.Copy()},
		world, hclog.NewNullLogger(),
	)

	address := types.StringToAddress("0xa302")
	key := types.StringToHash("0x01")
	value := types.StringToHash("0x02")

	initialRoot, err := executor.WriteGenesis(
		map[types.Address]*chain.GenesisAccount{
			address: {Balance: big.NewInt(8)},
		},
		types.ZeroHash,
	)
	require.NoError(t, err)

	account := &chain.GenesisAccount{
		Code:    []byte{0x00},
		Nonce:   3,
		Storage: map[types.Hash]types.Hash{key: value},
	}
	alloc := map[types.Address]*chain.GenesisAccount{address: account}
	before, err := json.Marshal(alloc)
	require.NoError(t, err)

	root, err := executor.WriteGenesis(alloc, initialRoot)
	require.NoError(t, err)
	snapshot, err := world.NewSnapshotAt(root)
	require.NoError(t, err)
	stored, err := snapshot.GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Zero(t, stored.Balance.Cmp(big.NewInt(8)))
	require.Equal(t, uint64(3), stored.Nonce)
	code, found, err := snapshot.GetCode(types.BytesToHash(stored.CodeHash))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, account.Code, code)
	actualStorage, err := snapshot.GetStorage(address, stored.Root, key)
	require.NoError(t, err)
	require.Equal(t, value, actualStorage)

	originalSnapshot, err := world.NewSnapshotAt(initialRoot)
	require.NoError(t, err)
	original, err := originalSnapshot.GetAccount(address)
	require.NoError(t, err)
	require.Zero(t, original.Balance.Cmp(big.NewInt(8)))
	require.Zero(t, original.Nonce)

	// Code materializes this account in both cases. Adding an explicit zero
	// must preserve the same fresh-state result as an omitted balance.
	omittedRoot, err := executor.WriteGenesis(alloc, types.ZeroHash)
	require.NoError(t, err)
	explicit := *account
	explicit.Balance = new(big.Int)
	explicitRoot, err := executor.WriteGenesis(
		map[types.Address]*chain.GenesisAccount{address: &explicit},
		types.ZeroHash,
	)
	require.NoError(t, err)
	require.Equal(t, omittedRoot, explicitRoot)

	after, err := json.Marshal(alloc)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Nil(t, account.Balance)
}

/* FINISH ALPHA CHANGE 3109 -- test direct genesis validation and omitted-balance state semantics -- */
