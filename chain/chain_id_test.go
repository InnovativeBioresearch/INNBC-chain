/* START ALPHA CHANGE 3054 -- test chain ID validation at configuration import -- */
package chain

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChainIDConfiguration_ValidationAndImport(t *testing.T) {
	for _, tc := range []struct {
		name  string
		id    int64
		valid bool
	}{
		/* START ALPHA CHANGE 3533 -- test the positive chain ID configuration range -- */
		{"zero", 0, false},
		{"minimum_supported", 1, true},
		{"previous_default", 100, true},
		{"production", 1982, true},
		{"maximum_supported", math.MaxInt64, true},
		{"negative_one", -1, false},
		{"wrapped_unsigned_boundary", math.MinInt64, false},
		/* FINISH ALPHA CHANGE 3533 -- test the positive chain ID configuration range -- */
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &Chain{
				Name: "chain-id-regression",
				Genesis: &Genesis{
					GasLimit:           5_242_880,
					BaseFee:            1,
					BaseFeeEM:          GenesisBaseFeeEM,
					BaseFeeChangeDenom: BaseFeeChangeDenom,
				},
				Params: &Params{
					ChainID: tc.id,
					Forks:   AllForksEnabled.Copy(),
					/* START ALPHA CHANGE 3325 -- preserve chain-ID validation with valid clock configuration -- */
					Engine: map[string]interface{}{
						"innbcbft": map[string]interface{}{"blockTime": "1s"},
					},
					/* FINISH ALPHA CHANGE 3325 -- preserve chain-ID validation with valid clock configuration -- */
				},
			}

			before, err := json.Marshal(config)
			require.NoError(t, err)

			validationErr := config.ValidateChainID()
			importValidationErr := validateImportedChain(config)

			path := filepath.Join(t.TempDir(), "genesis.json")
			require.NoError(t, os.WriteFile(path, before, 0o600))

			imported, importErr := ImportFromFile(path)
			if tc.valid {
				require.NoError(t, validationErr)
				require.NoError(t, importValidationErr)
				require.NoError(t, importErr)
				require.NotNil(t, imported)
				require.Equal(t, tc.id, imported.Params.ChainID)
			} else {
				/* START ALPHA CHANGE 3533 -- expect positive chain ID validation errors -- */
				require.ErrorContains(t, validationErr,
					"chain ID must be positive")
				require.ErrorContains(t, importValidationErr,
					"chain ID must be positive")
				require.ErrorContains(t, importErr,
					"chain ID must be positive")
				/* FINISH ALPHA CHANGE 3533 -- expect positive chain ID validation errors -- */
				require.Nil(t, imported)
			}

			after, err := json.Marshal(config)
			require.NoError(t, err)
			require.Equal(t, before, after)

			onDisk, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, onDisk)
		})
	}
}

/* START ALPHA CHANGE 3322 -- validate imported clock settings and genesis execution range -- */
func TestChainClock_ImportedConfiguration(t *testing.T) {
	cases := []struct {
		name      string
		engine    map[string]any
		timestamp uint64
		wantErr   string
	}{
		{"one_second", map[string]any{"blockTime": "1s"}, 100, ""},
		{"two_seconds", map[string]any{"blockTime": "2s"}, 100, ""},
		{"missing_interval", map[string]any{}, 100, "blockTime"},
		{"null_interval", map[string]any{"blockTime": nil}, 100, "blockTime"},
		{"zero_interval", map[string]any{"blockTime": "0s"}, 100, "at least 1s"},
		{"fractional_interval", map[string]any{"blockTime": "1.5s"}, 100, "whole number"},
		{"retired_zero", map[string]any{"blockTime": "2s", "blockTimeDrift": 0}, 100, "retired"},
		{"retired_null", map[string]any{"blockTime": "2s", "blockTimeDrift": nil}, 100, "retired"},
		{"unsupported_genesis", map[string]any{"blockTime": "2s"}, math.MaxUint64, "invalid genesis timestamp"},
		{"no_supported_successor", map[string]any{"blockTime": "2s"}, math.MaxInt64, "execution range"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Chain{
				Genesis: &Genesis{
					Timestamp:          tc.timestamp,
					GasLimit:           5_242_880,
					BaseFee:            1,
					BaseFeeEM:          GenesisBaseFeeEM,
					BaseFeeChangeDenom: BaseFeeChangeDenom,
				},
				Params: &Params{
					/* START ALPHA CHANGE 3533 -- preserve clock validation with a valid chain ID -- */
					ChainID: 1982,
					/* FINISH ALPHA CHANGE 3533 -- preserve clock validation with a valid chain ID -- */
					Forks:  AllForksEnabled.Copy(),
					Engine: map[string]any{"innbcbft": tc.engine},
				},
			}

			raw, err := json.Marshal(config)
			require.NoError(t, err)
			imported, err := importChain(raw)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, imported)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, imported)
		})
	}
}

/* FINISH ALPHA CHANGE 3322 -- validate imported clock settings and genesis execution range -- */

/* START ALPHA CHANGE 3548 -- test missing chain IDs using an otherwise valid imported configuration -- */
func TestChainIDConfiguration_RejectsMissingOrNull(t *testing.T) {
	config := &Chain{
		Name: "missing-chain-id-regression",
		Genesis: &Genesis{
			GasLimit:           5_242_880,
			BaseFee:            1,
			BaseFeeEM:          GenesisBaseFeeEM,
			BaseFeeChangeDenom: BaseFeeChangeDenom,
		},
		Params: &Params{
			ChainID: 1,
			Forks:   AllForksEnabled.Copy(),
			Engine: map[string]any{
				"innbcbft": map[string]any{"blockTime": "1s"},
			},
		},
	}

	original, err := json.Marshal(config)
	require.NoError(t, err)

	// Establish that the unchanged fixture imports successfully.
	valid, err := importChain(original)
	require.NoError(t, err)
	require.NotNil(t, valid)
	require.EqualValues(t, 1, valid.Params.ChainID)

	for _, mode := range []string{"omitted", "explicit_null"} {
		t.Run(mode, func(t *testing.T) {
			var document map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(original, &document))

			var params map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(document["params"], &params))

			if mode == "omitted" {
				delete(params, "chainID")
			} else {
				params["chainID"] = json.RawMessage("null")
			}

			encodedParams, err := json.Marshal(params)
			require.NoError(t, err)
			document["params"] = encodedParams

			candidate, err := json.Marshal(document)
			require.NoError(t, err)

			imported, err := importChain(candidate)
			require.Nil(t, imported)
			require.EqualError(t, err, "chain ID must be positive, got 0")
		})
	}
}

/* FINISH ALPHA CHANGE 3548 -- test missing chain IDs using an otherwise valid imported configuration -- */

func TestChainIDConfiguration_MissingParameters(t *testing.T) {
	var missing *Chain
	require.EqualError(t, missing.ValidateChainID(),
		"chain params config is missing")
	require.EqualError(t, (&Chain{}).ValidateChainID(),
		"chain params config is missing")
}

func TestChainIDConfiguration_UnsignedOverflowInJSON(t *testing.T) {
	for _, raw := range []string{
		`{"params":{"chainID":9223372036854775808}}`,
		`{"params":{"chainID":18446744073709551615}}`,
	} {
		imported, err := importChain([]byte(raw))
		require.Error(t, err)
		require.Nil(t, imported)

		var decodeErr *json.UnmarshalTypeError
		require.ErrorAs(t, err, &decodeErr)
	}
}

/* FINISH ALPHA CHANGE 3054 -- test chain ID validation at configuration import -- */
