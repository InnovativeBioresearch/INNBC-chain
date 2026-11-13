/* START ALPHA CHANGE 3126 -- reject aggregate supply overflow before genesis state access -- */
package state_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisSupply_ExecutorRejectsBeforeStateAccess(t *testing.T) {
	maximum := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1),
	)
	alloc := map[types.Address]*chain.GenesisAccount{
		types.StringToAddress("0xb401"): {Balance: maximum},
		types.StringToAddress("0xb402"): {Balance: big.NewInt(1)},
	}

	// An attempted snapshot opening would increment opens and then fail
	// through the deliberately absent backing state.
	backend := &genesisAllocationCountingState{}
	executor := statepkg.NewExecutor(
		&chain.Params{
			ChainID: 1982,
			Forks:   chain.AllForksEnabled.Copy(),
		},
		backend,
		hclog.NewNullLogger(),
	)
	hooks := 0
	executor.GenesisPostHook = func(*statepkg.Transition) error {
		hooks++
		return nil
	}

	for _, initialRoot := range []types.Hash{
		types.ZeroHash, types.StringToHash("0x01"),
	} {
		root, err := executor.WriteGenesis(alloc, initialRoot)
		require.Equal(t, types.ZeroHash, root)
		require.ErrorContains(t, err,
			"initial native-token supply exceeds unsigned 256-bit range")
		require.False(t, errors.Is(err, statepkg.ErrLocalStateFailure))
		require.Zero(t, backend.opens)
		require.Zero(t, hooks)
	}

	require.Zero(t, maximum.Cmp(alloc[types.StringToAddress("0xb401")].Balance))
	require.Zero(t, big.NewInt(1).Cmp(alloc[types.StringToAddress("0xb402")].Balance))
}

/* FINISH ALPHA CHANGE 3126 -- reject aggregate supply overflow before genesis state access -- */
