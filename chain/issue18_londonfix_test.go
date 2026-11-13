/* START ALPHA CHANGE 2395 -- test London correction policy at configuration import -- */
package chain

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIssue18_ImportedLondonFixPolicy(t *testing.T) {
	t.Parallel()

	var nilMap Forks

	tests := []struct {
		name  string
		forks *Forks
		valid bool
	}{
		{
			name: "missing fork configuration",
		},
		{
			name:  "nil fork map",
			forks: &nilMap,
		},
		{
			name:  "empty fork map",
			forks: &Forks{},
		},
		{
			name: "London without correction",
			forks: &Forks{
				London: NewFork(0),
			},
		},
		{
			name: "correction delayed by one block",
			forks: &Forks{
				London:    NewFork(0),
				LondonFix: NewFork(1),
			},
		},
		{
			name: "correction delayed to maximum block",
			forks: &Forks{
				London:    NewFork(0),
				LondonFix: NewFork(math.MaxUint64),
			},
		},
		{
			name:  "canonical enabled forks",
			forks: AllForksEnabled.Copy(),
			valid: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			config := &Chain{
				Name: "issue18",
				Genesis: &Genesis{
					GasLimit:           5_242_880,
					BaseFee:            1,
					BaseFeeEM:          GenesisBaseFeeEM,
					BaseFeeChangeDenom: BaseFeeChangeDenom,
				},
				Params: &Params{
					/* START ALPHA CHANGE 3537 -- preserve imported fork-policy coverage with a valid chain ID -- */
					ChainID: 1982,
					/* FINISH ALPHA CHANGE 3537 -- preserve imported fork-policy coverage with a valid chain ID -- */
					Forks: test.forks,
					/* START ALPHA CHANGE 3325 -- preserve London-fork validation with valid clock configuration -- */
					Engine: map[string]any{
						"innbcbft": map[string]any{"blockTime": "1s"},
					},
					/* FINISH ALPHA CHANGE 3325 -- preserve London-fork validation with valid clock configuration -- */
				},
			}

			before, err := json.Marshal(config)
			require.NoError(t, err)

			policyErr := config.ValidateLondonFix()
			if test.valid {
				require.NoError(t, policyErr)
			} else {
				require.ErrorContains(
					t, policyErr,
					"INNBC requires londonfix activation at block 0",
				)
			}

			after, err := json.Marshal(config)
			require.NoError(t, err)
			require.Equal(t, before, after)

			path := filepath.Join(t.TempDir(), "genesis.json")
			require.NoError(t, os.WriteFile(path, before, 0o600))

			imported, err := ImportFromFile(path)
			if test.valid {
				require.NoError(t, err)
				require.NotNil(t, imported)
				require.Equal(t, config.Params.Forks, imported.Params.Forks)
				require.NoError(t, imported.ValidateLondonFix())
			} else {
				require.ErrorContains(
					t, err,
					"INNBC requires londonfix activation at block 0",
				)
				require.Nil(t, imported)
			}

			onDisk, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, onDisk)
		})
	}
}

func TestIssue18_LondonFixPolicyRejectsIncompleteConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *Chain
	}{
		{name: "nil chain"},
		{name: "missing params", config: &Chain{}},
		{name: "missing forks", config: &Chain{Params: &Params{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(
				t,
				test.config.ValidateLondonFix(),
				"INNBC requires londonfix activation at block 0",
			)
		})
	}
}

/* FINISH ALPHA CHANGE 2395 -- test London correction policy at configuration import -- */
