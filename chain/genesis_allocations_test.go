/* START ALPHA CHANGE 3106 -- test genesis allocation import policy and nonmutating validation -- */
package chain

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisAllocations_ImportPolicy(t *testing.T) {
	address := types.StringToAddress("0xa001")
	overflow := new(big.Int).Lsh(big.NewInt(1), 256)

	for _, tc := range []struct {
		name    string
		account string
		wantErr string
		wantNil bool
	}{
		{"null_account", `null`, "account must not be null", false},
		{"empty_account", `{}`, "", true},
		{"null_balance", `{"balance":null}`, "", true},
		{"code_without_balance", `{"code":"0x00"}`, "", true},
		{"zero_balance", `{"balance":"0x0"}`, "", false},
		{"positive_balance", `{"balance":"17"}`, "", false},
		{
			"maximum_balance",
			`{"balance":"0x` + strings.Repeat("f", 64) + `"}`,
			"", false,
		},
		{"negative_balance", `{"balance":"-1"}`, "balance must be nonnegative", false},
		{
			"oversized_balance",
			fmt.Sprintf(`{"balance":%q}`, overflow.String()),
			"balance exceeds unsigned 256-bit range", false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &Chain{
				Name: "genesis-allocation-regression",
				Genesis: &Genesis{
					GasLimit:           GenesisGasLimit,
					BaseFee:            1,
					BaseFeeEM:          GenesisBaseFeeEM,
					BaseFeeChangeDenom: BaseFeeChangeDenom,
				},
				Params: &Params{
					ChainID: 1982,
					Forks:   AllForksEnabled.Copy(),
					/* START ALPHA CHANGE 3325 -- preserve allocation validation with valid clock configuration -- */
					Engine: map[string]any{
						"innbcbft": map[string]any{"blockTime": "1s"},
					},
					/* FINISH ALPHA CHANGE 3325 -- preserve allocation validation with valid clock configuration -- */
				},
			}

			raw, err := json.Marshal(config)
			require.NoError(t, err)

			var document map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &document))
			var genesis map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(document["genesis"], &genesis))

			genesis["alloc"] = json.RawMessage(fmt.Sprintf(
				`{%q:%s}`, address.String(), tc.account,
			))
			document["genesis"], err = json.Marshal(genesis)
			require.NoError(t, err)
			input, err := json.Marshal(document)
			require.NoError(t, err)
			before := append([]byte(nil), input...)

			imported, err := importChain(input)
			require.Equal(t, before, input)

			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.ErrorContains(t, err, address.String())
				require.Nil(t, imported)
				return
			}

			require.NoError(t, err)
			require.NoError(t, imported.ValidateGenesisAllocations())
			account := imported.Genesis.Alloc[address]
			require.NotNil(t, account)
			if tc.wantNil {
				require.Nil(t, account.Balance)
			} else {
				require.NotNil(t, account.Balance)
			}
			if tc.name == "code_without_balance" {
				require.Equal(t, []byte{0x00}, account.Code)
			}
			if tc.name == "maximum_balance" {
				want := new(big.Int).Sub(overflow, big.NewInt(1))
				require.Zero(t, want.Cmp(account.Balance))
			}
		})
	}
}

func TestGenesisAllocations_DirectValidation(t *testing.T) {
	var missing *Chain
	require.EqualError(t, missing.ValidateGenesisAllocations(), "chain config is empty")
	require.EqualError(t, (&Chain{}).ValidateGenesisAllocations(),
		"chain genesis config is missing")

	require.NoError(t, ValidateGenesisAllocations(nil))
	require.NoError(t, ValidateGenesisAllocations(map[types.Address]*GenesisAccount{}))

	for _, address := range []types.Address{
		types.ZeroAddress, types.StringToAddress("0xa002"),
	} {
		err := ValidateGenesisAllocations(map[types.Address]*GenesisAccount{
			address: nil,
		})
		require.ErrorContains(t, err, address.String())
		require.ErrorContains(t, err, "account must not be null")
	}

	address := types.StringToAddress("0xa003")
	account := &GenesisAccount{
		Code:    []byte{0x00},
		Nonce:   3,
		Storage: map[types.Hash]types.Hash{types.ZeroHash: types.StringToHash("0x01")},
	}
	config := &Chain{Genesis: &Genesis{
		Alloc: map[types.Address]*GenesisAccount{address: account},
	}}

	before, err := json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, config.ValidateGenesisAllocations())
	after, err := json.Marshal(config)
	require.NoError(t, err)

	require.Equal(t, before, after)
	require.Same(t, account, config.Genesis.Alloc[address])
	require.Nil(t, account.Balance)
}

/* FINISH ALPHA CHANGE 3106 -- test genesis allocation import policy and nonmutating validation -- */
