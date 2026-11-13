/* START ALPHA CHANGE 3122 -- test aggregate genesis supply limits and nonmutating import validation -- */
package chain

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisSupply_ValidationAndImport(t *testing.T) {
	maximum := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256),
		big.NewInt(1),
	)
	half := new(big.Int).Lsh(big.NewInt(1), 255)

	cases := []struct {
		name        string
		balances    []*big.Int
		zeroBalance *big.Int
		wantErr     string
	}{
		{
			name: "zero_supply",
			balances: []*big.Int{
				new(big.Int), nil,
			},
		},
		{
			name:     "single_maximum",
			balances: []*big.Int{new(big.Int).Set(maximum)},
		},
		{
			name: "split_exact_maximum",
			balances: []*big.Int{
				new(big.Int).Sub(new(big.Int).Set(maximum), big.NewInt(1)),
				big.NewInt(1),
			},
		},
		{
			name: "maximum_plus_one",
			balances: []*big.Int{
				new(big.Int).Set(maximum), big.NewInt(1),
			},
			wantErr: "initial native-token supply exceeds unsigned 256-bit range",
		},
		{
			name: "individually_valid_half_balances_overflow",
			balances: []*big.Int{
				new(big.Int).Set(half), new(big.Int).Set(half),
			},
			wantErr: "initial native-token supply exceeds unsigned 256-bit range",
		},
		{
			name: "omitted_and_zero_balances",
			balances: []*big.Int{
				nil, new(big.Int), new(big.Int).Set(maximum),
			},
		},
		{
			name:        "zero_address_excluded",
			balances:    []*big.Int{new(big.Int).Set(maximum)},
			zeroBalance: new(big.Int).Set(maximum),
		},
		{
			name:        "zero_address_still_individually_validated",
			balances:    []*big.Int{new(big.Int)},
			zeroBalance: new(big.Int).Lsh(big.NewInt(1), 256),
			wantErr:     "balance exceeds unsigned 256-bit range",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			alloc := make(map[types.Address]*GenesisAccount)
			for index, balance := range tt.balances {
				address := types.BytesToAddress([]byte{0xb0, byte(index + 1)})
				alloc[address] = &GenesisAccount{Balance: balance}
			}
			if tt.zeroBalance != nil {
				alloc[types.ZeroAddress] = &GenesisAccount{
					Balance: tt.zeroBalance,
				}
			}

			config := &Chain{
				Name: "genesis-supply-regression",
				Genesis: &Genesis{
					GasLimit:           GenesisGasLimit,
					BaseFee:            1,
					BaseFeeEM:          GenesisBaseFeeEM,
					BaseFeeChangeDenom: BaseFeeChangeDenom,
					Alloc:              alloc,
				},
				Params: &Params{
					ChainID: 1982,
					Forks:   AllForksEnabled.Copy(),
					/* START ALPHA CHANGE 3325 -- preserve supply validation with valid clock configuration -- */
					Engine: map[string]any{
						"innbcbft": map[string]any{"blockTime": "1s"},
					},
					/* FINISH ALPHA CHANGE 3325 -- preserve supply validation with valid clock configuration -- */
				},
			}
			before, err := json.Marshal(config)
			require.NoError(t, err)

			err = config.ValidateGenesisAllocations()
			imported, importErr := importChain(before)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.ErrorContains(t, importErr, tt.wantErr)
				require.Nil(t, imported)
			} else {
				require.NoError(t, err)
				require.NoError(t, importErr)
				require.NotNil(t, imported)
				require.NoError(t, imported.ValidateGenesisAllocations())
			}

			after, marshalErr := json.Marshal(config)
			require.NoError(t, marshalErr)
			require.Equal(t, before, after)

			for index, balance := range tt.balances {
				address := types.BytesToAddress([]byte{0xb0, byte(index + 1)})
				if balance == nil {
					require.Nil(t, alloc[address].Balance)
				} else {
					require.Same(t, balance, alloc[address].Balance)
				}
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3122 -- test aggregate genesis supply limits and nonmutating import validation -- */
