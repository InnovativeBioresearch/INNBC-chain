package chain

import (
	"encoding/json"
	"math/big"
	"reflect"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
)

var emptyAddr types.Address

func addr(str string) types.Address {
	return types.StringToAddress(str)
}

func hash(str string) types.Hash {
	return types.StringToHash(str)
}

func TestGenesisAlloc(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input  string
		output map[types.Address]GenesisAccount
	}{
		{
			input: `{
				"0x0000000000000000000000000000000000000000": {
					"balance": "0x11"
				}
			}`,
			output: map[types.Address]GenesisAccount{
				emptyAddr: {
					Balance: big.NewInt(17),
				},
			},
		},
		{
			input: `{
				"0x0000000000000000000000000000000000000000": {
					"balance": "0x11",
					"nonce": "0x100",
					"storage": {
						"` + hash("1").String() + `": "` + hash("3").String() + `",
						"` + hash("2").String() + `": "` + hash("4").String() + `"
					}
				}
			}`,
			output: map[types.Address]GenesisAccount{
				emptyAddr: {
					Balance: big.NewInt(17),
					Nonce:   256,
					Storage: map[types.Hash]types.Hash{
						hash("1"): hash("3"),
						hash("2"): hash("4"),
					},
				},
			},
		},
		{
			input: `{
				"0x0000000000000000000000000000000000000000": {
					"balance": "0x11"
				},
				"0x0000000000000000000000000000000000000001": {
					"balance": "0x12"
				}
			}`,
			output: map[types.Address]GenesisAccount{
				addr("0"): {
					Balance: big.NewInt(17),
				},
				addr("1"): {
					Balance: big.NewInt(18),
				},
			},
		},
	}

	for _, c := range cases {
		c := c

		t.Run("", func(t *testing.T) {
			t.Parallel()

			var dec map[types.Address]GenesisAccount
			if err := json.Unmarshal([]byte(c.input), &dec); err != nil {
				if c.output != nil {
					t.Fatal(err)
				}
			} else if !reflect.DeepEqual(dec, c.output) {
				t.Fatal("bad")
			}
		})
	}
}

func TestGenesisX(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input  string
		output *Genesis
	}{
		{
			input: `{
				"difficulty": "0x12",
				"gasLimit": "0x11",
				"alloc": {
					"0x0000000000000000000000000000000000000000": {
						"balance": "0x11"
					},
					"0x0000000000000000000000000000000000000001": {
						"balance": "0x12"
					}
				}
			}`,
			output: &Genesis{
				Difficulty: 18,
				GasLimit:   17,
				Alloc: map[types.Address]*GenesisAccount{
					emptyAddr: {
						Balance: big.NewInt(17),
					},
					addr("1"): {
						Balance: big.NewInt(18),
					},
				},
			},
		},
	}

	for _, c := range cases {
		c := c

		t.Run("", func(t *testing.T) {
			t.Parallel()

			var dec *Genesis
			if err := json.Unmarshal([]byte(c.input), &dec); err != nil {
				if c.output != nil {
					t.Fatal(err)
				}
			} else if !reflect.DeepEqual(dec, c.output) {
				t.Fatal("bad")
			}
		})
	}
}

/* START ALPHA CHANGE 2054 -- test imported block gas capacity invariants -- */
func TestValidateImportedChainBlockGasCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		genesisGasLimit uint64
		blockGasTarget  uint64
		expectedError   string
	}{
		{
			name:            "zero genesis gas limit uses valid fallback",
			genesisGasLimit: 0,
		},
		{
			name:            "rejects genesis gas limit below mandatory transaction requirement",
			genesisGasLimit: types.StateTransactionGasLimit - 1,
			expectedError:   "genesis gasLimit must be at least mandatory state transaction gas limit 1000000",
		},
		{
			name:            "accepts exact mandatory transaction gas limit",
			genesisGasLimit: types.StateTransactionGasLimit,
		},
		{
			name:            "accepts generated INNBC gas limit",
			genesisGasLimit: 5_242_880,
		},
		{
			name:            "rejects nonzero block gas target below mandatory transaction requirement",
			genesisGasLimit: 5_242_880,
			blockGasTarget:  types.StateTransactionGasLimit - 1,
			expectedError:   "block gas target must be zero or at least mandatory state transaction gas limit 1000000",
		},
		{
			name:            "accepts zero block gas target",
			genesisGasLimit: 5_242_880,
			blockGasTarget:  0,
		},
		{
			name:            "accepts exact mandatory transaction block gas target",
			genesisGasLimit: 5_242_880,
			blockGasTarget:  types.StateTransactionGasLimit,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			chainConfig := &Chain{
				Genesis: &Genesis{
					GasLimit:           test.genesisGasLimit,
					BaseFee:            1,
					BaseFeeEM:          GenesisBaseFeeEM,
					BaseFeeChangeDenom: BaseFeeChangeDenom,
				},
				Params: &Params{
					/* START ALPHA CHANGE 3536 -- preserve gas-capacity coverage with a valid chain ID -- */
					ChainID: 1982,
					/* FINISH ALPHA CHANGE 3536 -- preserve gas-capacity coverage with a valid chain ID -- */
					/* START ALPHA CHANGE 2393 -- retain a supported fork schedule in gas capacity fixtures -- */
					Forks: AllForksEnabled.Copy(),
					/* FINISH ALPHA CHANGE 2393 -- retain a supported fork schedule in gas capacity fixtures -- */
					/* START ALPHA CHANGE 3325 -- preserve gas-capacity coverage with valid clock configuration -- */
					Engine: map[string]any{
						"innbcbft": map[string]any{"blockTime": "1s"},
					},
					/* FINISH ALPHA CHANGE 3325 -- preserve gas-capacity coverage with valid clock configuration -- */
					BlockGasTarget: test.blockGasTarget,
				},
			}

			err := validateImportedChain(chainConfig)
			if test.expectedError != "" {
				if err == nil || err.Error() != test.expectedError {
					t.Fatalf("expected %q, got %v", test.expectedError, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2054 -- test imported block gas capacity invariants -- */
