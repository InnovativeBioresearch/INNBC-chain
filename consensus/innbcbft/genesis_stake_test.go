package innbcbft

import (
	/* START ALPHA CHANGE 3340 -- import native-token configuration regression dependencies -- */
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	/* FINISH ALPHA CHANGE 3340 -- import native-token configuration regression dependencies -- */

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2648 -- test fixed genesis stake across configuration and initialization boundaries -- */

func TestGenesisMinimumStake_Validation(t *testing.T) {
	cases := []struct {
		name  string
		value *big.Int
		valid bool
	}{
		{"missing", nil, false},
		{"negative", big.NewInt(-1), false},
		{"zero", big.NewInt(0), false},
		{"one wei", big.NewInt(1), true},
		{"two wei", big.NewInt(2), false},
		{"one whole token", big.NewInt(1_000_000_000_000_000_000), false},
		{"large integer", new(big.Int).Lsh(big.NewInt(1), 256), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var before *big.Int
			if tc.value != nil {
				before = new(big.Int).Set(tc.value)
			}

			err := ValidateGenesisMinimumStake(tc.value)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrInvalidGenesisMinimumStake)
			}

			if before != nil {
				require.Zero(t, before.Cmp(tc.value))
			}
		})
	}
}

func TestGenesisMinimumStake_ConstructionBoundaries(t *testing.T) {
	cases := []struct {
		name     string
		registry *ValidatorRegistryConfig
		valid    bool
	}{
		{"missing registry", nil, false},
		{"missing minimum", &ValidatorRegistryConfig{}, false},
		{"negative", &ValidatorRegistryConfig{MinimumStake: big.NewInt(-1)}, false},
		{"zero", &ValidatorRegistryConfig{MinimumStake: big.NewInt(0)}, false},
		{"one wei", testValidatorRegistryConfig(types.ZeroAddress), true},
		{"two wei", &ValidatorRegistryConfig{MinimumStake: big.NewInt(2)}, false},
		{"larger minimum", &ValidatorRegistryConfig{MinimumStake: big.NewInt(100)}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chainConfig := &chain.Chain{
				Params: &chain.Params{
					Engine: map[string]any{
						ConsensusName: &INNBCBFTConfig{
							/* START ALPHA CHANGE 3320 -- preserve stake validation as the fixture's subject -- */
							BlockTime: testChainClockConfig().BlockTime,
							/* FINISH ALPHA CHANGE 3320 -- preserve stake validation as the fixture's subject -- */
							/* START ALPHA CHANGE 3339 -- preserve stake validation with required native-token configuration -- */
							NativeTokenConfig: DefaultTokenConfig,
							/* FINISH ALPHA CHANGE 3339 -- preserve stake validation with required native-token configuration -- */
							/* START ALPHA CHANGE 3440 -- keep stake configuration coverage independent of epoch validation -- */
							EpochSize: 2,
							/* FINISH ALPHA CHANGE 3440 -- keep stake configuration coverage independent of epoch validation -- */
							ValidatorRegistryConfig: tc.registry,
						},
					},
				},
			}

			decoded, decodeErr := GetINNBCBFTConfig(chainConfig)

			instance, factoryErr := Factory(&consensus.Params{
				Logger: hclog.NewNullLogger(),
				Config: &consensus.Config{
					Config: map[string]any{
						/* START ALPHA CHANGE 3320 -- preserve direct factory stake validation -- */
						"blockTime": "1s",
						/* FINISH ALPHA CHANGE 3320 -- preserve direct factory stake validation -- */
						/* START ALPHA CHANGE 3339 -- preserve direct factory stake coverage with required native-token configuration -- */
						"nativeTokenConfig": DefaultTokenConfig,
						/* FINISH ALPHA CHANGE 3339 -- preserve direct factory stake coverage with required native-token configuration -- */
						/* START ALPHA CHANGE 3440 -- keep direct factory stake coverage independent of epoch validation -- */
						"epochSize": uint64(2),
						/* FINISH ALPHA CHANGE 3440 -- keep direct factory stake coverage independent of epoch validation -- */
						"validatorRegistryConfig": tc.registry,
					},
				},
			})

			manager, managerErr := newValidatorRegistryManager(
				hclog.NewNullLogger(), nil, tc.registry, 100,
			)

			if tc.valid {
				require.NoError(t, decodeErr)
				require.Zero(t, decoded.ValidatorRegistryConfig.MinimumStake.Cmp(big.NewInt(1)))

				require.NoError(t, factoryErr)
				require.NotNil(t, instance)
				instance.(*INNBCBFT).syncCancel()

				require.NoError(t, managerErr)
				require.NotNil(t, manager)

				return
			}

			require.Error(t, decodeErr)
			require.Error(t, factoryErr)
			require.Nil(t, instance)
			require.Error(t, managerErr)
			require.Nil(t, manager)

			/*
				A nil transition ensures invalid configuration is rejected
				before contract execution can be attempted.
			*/
			require.NotPanics(t, func() {
				err := initValidatorRegistry(INNBCBFTConfig{
					ValidatorRegistryConfig: tc.registry,
				}, nil)
				require.Error(t, err)

				if tc.registry != nil {
					require.ErrorIs(t, err, ErrInvalidGenesisMinimumStake)
				}
			})

			/*
				The genesis hook must also reject the configuration before
				accessing genesis allocations or an execution transition.
			*/
			require.NotPanics(t, func() {
				err := GenesisPostHookFactory(chainConfig, ConsensusName)(nil)
				require.Error(t, err)

				if tc.registry != nil {
					require.ErrorIs(t, err, ErrInvalidGenesisMinimumStake)
				}
			})

			if tc.registry != nil {
				require.ErrorIs(t, decodeErr, ErrInvalidGenesisMinimumStake)
				require.ErrorIs(t, factoryErr, ErrInvalidGenesisMinimumStake)
				require.ErrorIs(t, managerErr, ErrInvalidGenesisMinimumStake)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2648 -- test fixed genesis stake across configuration and initialization boundaries -- */

/* START ALPHA CHANGE 3340 -- test required native-token configuration across decoding factory and genesis-hook boundaries -- */

func TestNativeTokenConfig_ConfigurationBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		omit    bool
		token   *TokenConfig
		wantErr bool
	}{
		{
			name:    "omitted",
			omit:    true,
			wantErr: true,
		},
		{
			name:    "explicit_null",
			wantErr: true,
		},
		{
			name:  "empty_object_preserves_presence_only_scope",
			token: &TokenConfig{},
		},
		{
			name: "default_metadata",
			token: &TokenConfig{
				Name:     "INNBC Chain",
				Symbol:   "INNBC",
				Decimals: 18,
			},
		},
		{
			name: "zero_decimals",
			token: &TokenConfig{
				Name:     "Zero Decimal Token",
				Symbol:   "ZERO",
				Decimals: 0,
			},
		},
		{
			name: "custom_metadata",
			token: &TokenConfig{
				Name:     "Custom INNBC",
				Symbol:   "CINNBC",
				Decimals: 9,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			/* START ALPHA CHANGE 3413 -- use current artifacts at native-token configuration boundaries -- */
			// Build an independent configuration from current contract artifacts.
			config := genesisAllocationsHookConfig(t)
			/* FINISH ALPHA CHANGE 3413 -- use current artifacts at native-token configuration boundaries -- */
			engine, ok := config.Params.Engine[ConsensusName].(map[string]any)
			require.True(t, ok)

			if tc.omit {
				delete(engine, "nativeTokenConfig")
			} else {
				// A typed nil pointer serializes as explicit JSON null.
				engine["nativeTokenConfig"] = tc.token
			}

			before, err := json.Marshal(config)
			require.NoError(t, err)

			decoded, decodeErr := GetINNBCBFTConfig(config)
			instance, factoryErr := Factory(&consensus.Params{
				Logger: hclog.NewNullLogger(),
				Config: &consensus.Config{
					Config: engine,
				},
			})

			path := filepath.Join(t.TempDir(), "genesis.json")
			require.NoError(t, os.WriteFile(path, before, 0600))
			loaded, loadErr := LoadINNBCBFTConfig(path)

			if tc.wantErr {
				require.ErrorIs(t, decodeErr, ErrMissingNativeTokenConfig)
				require.ErrorIs(t, factoryErr, ErrMissingNativeTokenConfig)
				require.ErrorIs(t, loadErr, ErrMissingNativeTokenConfig)
				require.Nil(t, instance)
				require.Nil(t, decoded.NativeTokenConfig)
				require.Nil(t, loaded.NativeTokenConfig)

				// A nil transition proves rejection occurs before
				// contract initialization or transition access.
				require.NotPanics(t, func() {
					hookErr := GenesisPostHookFactory(
						config, ConsensusName,
					)(nil)
					require.ErrorIs(t, hookErr, ErrMissingNativeTokenConfig)
				})
			} else {
				require.NoError(t, decodeErr)
				require.NoError(t, factoryErr)
				require.NoError(t, loadErr)
				require.NotNil(t, instance)

				created, ok := instance.(*INNBCBFT)
				require.True(t, ok)
				t.Cleanup(created.syncCancel)

				require.Equal(t, tc.token, decoded.NativeTokenConfig)
				require.Equal(t, tc.token, loaded.NativeTokenConfig)
				require.Equal(t, tc.token,
					created.consensusConfig.NativeTokenConfig)
			}

			after, err := json.Marshal(config)
			require.NoError(t, err)
			require.Equal(t, before, after,
				"validation modified the supplied configuration")

			onDisk, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, onDisk,
				"loading modified the supplied genesis file")
		})
	}
}

/* FINISH ALPHA CHANGE 3340 -- test required native-token configuration across decoding factory and genesis-hook boundaries -- */

/* START ALPHA CHANGE 3441 -- verify epoch sizes across loading construction and genesis-hook boundaries -- */

func TestEpochSize_ConfigurationBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		omit    bool
		wantErr bool
	}{
		{name: "omitted", omit: true, wantErr: true},
		{name: "explicit_null", wantErr: true},
		{name: "zero", value: uint64(0), wantErr: true},
		{name: "one", value: uint64(1), wantErr: true},
		{name: "minimum", value: uint64(2)},
		{name: "production_default", value: uint64(30)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := genesisAllocationsHookConfig(t)
			engine, ok := config.Params.Engine[ConsensusName].(map[string]any)
			require.True(t, ok)

			if tc.omit {
				delete(engine, "epochSize")
			} else {
				engine["epochSize"] = tc.value
			}

			before, err := json.Marshal(config)
			require.NoError(t, err)

			decoded, decodeErr := GetINNBCBFTConfig(config)
			instance, factoryErr := Factory(&consensus.Params{
				Logger: hclog.NewNullLogger(),
				Config: &consensus.Config{Config: engine},
			})

			path := filepath.Join(t.TempDir(), "genesis.json")
			require.NoError(t, os.WriteFile(path, before, 0600))
			loaded, loadErr := LoadINNBCBFTConfig(path)

			if tc.wantErr {
				require.ErrorIs(t, decodeErr, ErrInvalidEpochSize)
				require.ErrorIs(t, factoryErr, ErrInvalidEpochSize)
				require.ErrorIs(t, loadErr, ErrInvalidEpochSize)
				require.Nil(t, instance)

				// A nil transition proves rejection occurs before
				// contract initialization or execution-state access.
				require.NotPanics(t, func() {
					hookErr := GenesisPostHookFactory(
						config, ConsensusName,
					)(nil)
					require.ErrorIs(t, hookErr, ErrInvalidEpochSize)
				})
			} else {
				require.NoError(t, decodeErr)
				require.NoError(t, factoryErr)
				require.NoError(t, loadErr)
				require.NotNil(t, instance)

				created, ok := instance.(*INNBCBFT)
				require.True(t, ok)
				t.Cleanup(created.syncCancel)

				want := tc.value.(uint64)
				require.Equal(t, want, decoded.EpochSize)
				require.Equal(t, want, loaded.EpochSize)
				require.Equal(t, want, created.consensusConfig.EpochSize)
			}

			after, err := json.Marshal(config)
			require.NoError(t, err)
			require.Equal(t, before, after,
				"validation modified the supplied configuration")

			onDisk, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, onDisk,
				"loading modified the supplied genesis file")
		})
	}
}

/* FINISH ALPHA CHANGE 3441 -- verify epoch sizes across loading construction and genesis-hook boundaries -- */
