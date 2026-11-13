/* START ALPHA CHANGE 3135 -- test strict allocation keys and duplicate decoded address rejection -- */
package chain

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func genesisAddressDocument(
	t *testing.T,
	entries map[string]*GenesisAccount,
) []byte {
	t.Helper()

	config := &Chain{
		Name: "genesis-address-regression",
		Genesis: &Genesis{
			GasLimit:           GenesisGasLimit,
			BaseFee:            1,
			BaseFeeEM:          GenesisBaseFeeEM,
			BaseFeeChangeDenom: BaseFeeChangeDenom,
		},
		Params: &Params{
			ChainID: 1982,
			Forks:   AllForksEnabled.Copy(),
			/* START ALPHA CHANGE 3325 -- preserve address validation with valid clock configuration -- */
			Engine: map[string]any{
				"innbcbft": map[string]any{"blockTime": "1s"},
			},
			/* FINISH ALPHA CHANGE 3325 -- preserve address validation with valid clock configuration -- */
		},
	}
	raw, err := json.Marshal(config)
	require.NoError(t, err)

	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &document))
	var genesis map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(document["genesis"], &genesis))

	genesis["alloc"], err = json.Marshal(entries)
	require.NoError(t, err)
	document["genesis"], err = json.Marshal(genesis)
	require.NoError(t, err)
	input, err := json.Marshal(document)
	require.NoError(t, err)

	return input
}

func requireGenesisAddressRejected(
	t *testing.T,
	input []byte,
	expectedError string,
) {
	t.Helper()

	before := append([]byte(nil), input...)
	imported, err := importChain(input)
	require.Nil(t, imported)
	require.ErrorContains(t, err, expectedError)
	require.Equal(t, before, input)

	path := filepath.Join(t.TempDir(), "genesis.json")
	require.NoError(t, os.WriteFile(path, input, 0o600))
	imported, err = ImportFromFile(path)
	require.Nil(t, imported)
	require.ErrorContains(t, err, expectedError)
	afterFile, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, input, afterFile)

	var document struct {
		Genesis json.RawMessage `json:"genesis"`
	}
	require.NoError(t, json.Unmarshal(input, &document))

	// A failed decode must not publish a partially converted allocation map.
	marker := types.StringToAddress("0xc001")
	account := &GenesisAccount{Balance: big.NewInt(99)}
	target := &Genesis{
		Alloc: map[types.Address]*GenesisAccount{marker: account},
	}
	beforeAlloc, err := json.Marshal(target.Alloc)
	require.NoError(t, err)

	err = json.Unmarshal(document.Genesis, target)
	require.ErrorContains(t, err, expectedError)
	require.Len(t, target.Alloc, 1)
	require.Same(t, account, target.Alloc[marker])
	afterAlloc, err := json.Marshal(target.Alloc)
	require.NoError(t, err)
	require.Equal(t, beforeAlloc, afterAlloc)
}

func TestGenesisAddress_ImportAcceptedFormats(t *testing.T) {
	lower := strings.Repeat("ab", 20)
	for index, key := range []string{
		"0x" + lower,
		lower,
		"0x" + strings.ToUpper(lower),
		strings.ToUpper(lower),
		"0x" + strings.Repeat("aB", 20),
		types.ZeroAddress.String(),
	} {
		t.Run(fmt.Sprintf("%d", index), func(t *testing.T) {
			account := &GenesisAccount{
				Balance: big.NewInt(17),
				Nonce:   3,
				Code:    []byte{0x00},
				Storage: map[types.Hash]types.Hash{
					types.ZeroHash: types.StringToHash("0x01"),
				},
			}
			input := genesisAddressDocument(t, map[string]*GenesisAccount{
				key: account,
			})

			imported, err := importChain(input)
			require.NoError(t, err)
			require.Len(t, imported.Genesis.Alloc, 1)

			stored := imported.Genesis.Alloc[types.StringToAddress(key)]
			require.NotNil(t, stored)
			require.Zero(t, account.Balance.Cmp(stored.Balance))
			require.Equal(t, account.Nonce, stored.Nonce)
			require.Equal(t, account.Code, stored.Code)
			require.Equal(t, account.Storage, stored.Storage)
		})
	}
}

func TestGenesisAddress_ImportRejectsMalformedKeys(t *testing.T) {
	valid := "0x" + strings.Repeat("ab", 20)
	keys := []string{
		"",
		"0x",
		"0xzz",
		"zz",
		"0x12345",
		"0x" + strings.Repeat("a", 39),
		"0x" + strings.Repeat("a", 42),
		"0x" + strings.Repeat("ab", 19) + "zz",
		"0X" + strings.Repeat("ab", 20),
		" 0x" + strings.Repeat("ab", 20),
		"0x" + strings.Repeat("ab", 20) + " ",
	}

	for index, key := range keys {
		t.Run(fmt.Sprintf("%d", index), func(t *testing.T) {
			input := genesisAddressDocument(t, map[string]*GenesisAccount{
				key:   {Balance: big.NewInt(17)},
				valid: {Balance: big.NewInt(23)},
			})
			requireGenesisAddressRejected(t, input,
				"invalid genesis allocation address")
		})
	}
}

func TestGenesisAddress_ImportRejectsDuplicateDecodedAddresses(t *testing.T) {
	lower := strings.Repeat("ab", 20)
	zero := strings.Repeat("0", 40)

	cases := []struct {
		name    string
		first   string
		second  string
		nullOne bool
	}{
		{"prefix_alias", "0x" + lower, lower, false},
		{"case_alias", "0x" + lower, "0x" + strings.ToUpper(lower), false},
		{"zero_address_alias", "0x" + zero, zero, false},
		{"null_account_alias", "0x" + lower, lower, true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			first := &GenesisAccount{Balance: big.NewInt(17)}
			if tt.nullOne {
				first = nil
			}
			input := genesisAddressDocument(t, map[string]*GenesisAccount{
				tt.first:  first,
				tt.second: {Balance: big.NewInt(23)},
			})
			requireGenesisAddressRejected(t, input,
				"duplicate decoded genesis allocation address")

			// Rejection must not depend on map iteration order.
			for range 20 {
				imported, err := importChain(input)
				require.Nil(t, imported)
				require.ErrorContains(t, err,
					"duplicate decoded genesis allocation address")
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3135 -- test strict allocation keys and duplicate decoded address rejection -- */
