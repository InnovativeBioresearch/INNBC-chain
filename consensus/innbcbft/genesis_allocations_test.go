/* START ALPHA CHANGE 3110 -- test genesis hook rejection and actual initialization with omitted balances -- */
package innbcbft

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"

	/* START ALPHA CHANGE 3413 -- share current artifact allocations across genesis tests -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi/testutil"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/helper/common"

	/* FINISH ALPHA CHANGE 3413 -- share current artifact allocations across genesis tests -- */
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 3413 -- initialize allocation regressions from current contract artifacts -- */
func genesisAllocationsHookConfig(t *testing.T) *chain.Chain {
	t.Helper()

	initialValidators := validator.NewTestValidators(t, 1).GetParamValidators()
	tokenConfig := *DefaultTokenConfig

	consensusConfig := &INNBCBFTConfig{
		InitialValidatorSet:      initialValidators,
		BlockTime:                common.Duration{Duration: 2 * time.Second},
		EpochSize:                2,
		SprintSize:               1,
		NativeTokenConfig:        &tokenConfig,
		MinValidatorSetSize:      1,
		MaxValidatorSetSize:      100,
		BlockTrackerPollInterval: common.Duration{Duration: time.Second},
		ValidatorRegistryConfig: &ValidatorRegistryConfig{
			GenesisValidator:       initialValidators[0].Address,
			MinimumStake:           big.NewInt(1),
			ProposalDurationPreset: ValidatorRegistryProposalDurationStandard,
			ActivationEpoch:        2,
		},
	}

	accounts, err := BuildGenesisValidatorSet(consensusConfig)
	require.NoError(t, err)

	config := &chain.Chain{
		Name: "current-allocation-tests",
		Genesis: &chain.Genesis{
			Timestamp:          1_700_000_000,
			GasLimit:           chain.GenesisGasLimit,
			Difficulty:         0,
			Mixhash:            INNBCBFTMixDigest,
			BaseFee:            chain.GenesisBaseFee,
			BaseFeeEM:          chain.GenesisBaseFeeEM,
			BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
			ExtraData:          BuildGenesisExtraData(accounts),
			Alloc:              testutil.GenesisAllocations(t),
		},
		Params: &chain.Params{
			ChainID: 100,
			Forks:   chain.AllForksEnabled.Copy(),
			Engine: map[string]interface{}{
				ConsensusName: consensusConfig,
			},
		},
	}

	payload, err := json.Marshal(config)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "current-genesis.json")
	require.NoError(t, os.WriteFile(path, payload, 0600))

	imported, err := chain.ImportFromFile(path)
	require.NoError(t, err)

	return imported
}

/* FINISH ALPHA CHANGE 3413 -- initialize allocation regressions from current contract artifacts -- */

func TestGenesisAllocations_HookRejectsBeforeExecution(t *testing.T) {
	address := types.StringToAddress("0xa401")

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
			config := genesisAllocationsHookConfig(t)
			config.Genesis.Alloc[address] = tc.account

			// A nil transition proves rejection precedes contract execution.
			err := GenesisPostHookFactory(config, ConsensusName)(nil)
			require.ErrorContains(t, err, tc.wantErr)
			require.ErrorContains(t, err, address.String())
		})
	}

	config := genesisAllocationsHookConfig(t)
	config.Genesis = nil
	require.EqualError(t, GenesisPostHookFactory(config, ConsensusName)(nil),
		"chain genesis config is missing")
}

func TestGenesisAllocations_HookInitializesOmittedBalances(t *testing.T) {
	address := types.StringToAddress("0xa402")
	key := types.StringToHash("0x01")
	value := types.StringToHash("0x02")

	for _, tc := range []struct {
		name    string
		account string
	}{
		{"omitted", `{}`},
		{"null", `{"balance":null}`},
		{"explicit_zero", `{"balance":"0x0"}`},
		{"full_width", `{"balance":"0x10000000000000000"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := genesisAllocationsHookConfig(t)
			require.NotContains(t, config.Genesis.Alloc, address)

			// Capture the expected existing supply independently of the
			// additional account and the excluded zero-address allocation.
			expectedSupply := new(big.Int)
			for addr, existing := range config.Genesis.Alloc {
				if addr != types.ZeroAddress && existing.Balance != nil {
					expectedSupply.Add(expectedSupply, existing.Balance)
				}
			}

			var account chain.GenesisAccount
			require.NoError(t, json.Unmarshal([]byte(tc.account), &account))
			expectedBalance := new(big.Int)
			if account.Balance != nil {
				expectedBalance.Set(account.Balance)
			}
			expectedSupply.Add(expectedSupply, expectedBalance)

			account.Code = []byte{0x00}
			account.Nonce = 3
			account.Storage = map[types.Hash]types.Hash{key: value}
			config.Genesis.Alloc[address] = &account
			config.Genesis.Alloc[types.ZeroAddress] = &chain.GenesisAccount{
				Balance: big.NewInt(999),
			}
			before, err := json.Marshal(config.Genesis.Alloc)
			require.NoError(t, err)

			storage := itrie.NewMemoryStorage()
			t.Cleanup(func() { require.NoError(t, storage.Close()) })
			world := itrie.NewState(storage)
			executor := state.NewExecutor(config.Params, world, hclog.NewNullLogger())
			/* START ALPHA CHANGE 3810 -- adapt genesis allocation fixture to checked hash lookups -- */
			executor.GetHash = func(*types.Header) state.GetHashByNumber {
				return func(uint64) (types.Hash, error) {
					return types.ZeroHash, nil
				}
			}
			/* FINISH ALPHA CHANGE 3810 -- adapt genesis allocation fixture to checked hash lookups -- */
			executor.GenesisPostHook = GenesisPostHookFactory(config, ConsensusName)

			root, err := executor.WriteGenesis(config.Genesis.Alloc, types.ZeroHash)
			require.NoError(t, err)

			snapshot, err := world.NewSnapshotAt(root)
			require.NoError(t, err)
			stored, err := snapshot.GetAccount(address)
			require.NoError(t, err)
			require.NotNil(t, stored)
			require.Zero(t, expectedBalance.Cmp(stored.Balance))
			require.Equal(t, uint64(3), stored.Nonce)
			code, found, err := snapshot.GetCode(types.BytesToHash(stored.CodeHash))
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, account.Code, code)
			actualStorage, err := snapshot.GetStorage(address, stored.Root, key)
			require.NoError(t, err)
			require.Equal(t, value, actualStorage)

			transition, err := executor.BeginTxn(
				root,
				&types.Header{Number: 1, GasLimit: 10_000_000, BaseFee: 1},
				types.ZeroAddress,
			)
			require.NoError(t, err)
			input, err := contractsapi.INNBCNativeToken.Abi.Methods["totalSupply"].Encode([]any{})
			require.NoError(t, err)
			result := transition.Call2(
				address, contracts.NativeERC20TokenContract,
				input, new(big.Int), 1_000_000,
			)
			require.NotNil(t, result)
			require.NoError(t, result.Err)
			require.NoError(t, transition.Error())
			require.Len(t, result.ReturnValue, 32)
			require.Zero(t, expectedSupply.Cmp(new(big.Int).SetBytes(result.ReturnValue)))

			after, err := json.Marshal(config.Genesis.Alloc)
			require.NoError(t, err)
			require.Equal(t, before, after)
			if tc.name == "omitted" || tc.name == "null" {
				require.Nil(t, account.Balance)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3110 -- test genesis hook rejection and actual initialization with omitted balances -- */
