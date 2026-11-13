/* START ALPHA CHANGE 3133 -- test strict premine addresses while preserving amount and key handling -- */
package helper

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisAddress_PremineAcceptedFormats(t *testing.T) {
	lower := strings.Repeat("ab", 20)
	expected := types.BytesToAddress(bytes.Repeat([]byte{0xab}, 20))

	formats := []struct {
		name    string
		address string
		want    types.Address
	}{
		{"prefixed_lowercase", "0x" + lower, expected},
		{"prefixless_lowercase", lower, expected},
		{"prefixed_uppercase_digits", "0x" + strings.ToUpper(lower), expected},
		{"prefixless_uppercase_digits", strings.ToUpper(lower), expected},
		{"mixed_case_without_checksum_requirement", "0x" + strings.Repeat("aB", 20), expected},
		{"explicit_zero_address", types.ZeroAddress.String(), types.ZeroAddress},
	}

	suffixes := []struct {
		name   string
		value  string
		amount *big.Int
		key    string
	}{
		{"address_only", "", new(big.Int).Set(command.DefaultPremineBalance), ""},
		{"explicit_zero", ":0", new(big.Int), ""},
		{"decimal_amount", ":17", big.NewInt(17), ""},
		{"hexadecimal_amount", ":0x11", big.NewInt(17), ""},
		{"optional_key", ":17:test-key", big.NewInt(17), "test-key"},
	}

	for _, format := range formats {
		for _, suffix := range suffixes {
			t.Run(format.name+"/"+suffix.name, func(t *testing.T) {
				info, err := ParsePremineInfo(format.address + suffix.value)
				require.NoError(t, err)
				require.NotNil(t, info)
				require.Equal(t, format.want, info.Address)
				require.Zero(t, suffix.amount.Cmp(info.Amount))
				require.Equal(t, suffix.key, info.Key)
			})
		}
	}
}

func TestGenesisAddress_PremineRejectsMalformedAddresses(t *testing.T) {
	const privateKey = "PRIVATE_KEY_SENTINEL"

	addresses := []string{
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

	for index, address := range addresses {
		for _, suffix := range []string{"", ":17", ":17:" + privateKey} {
			t.Run(fmt.Sprintf("%d_%d", index, len(suffix)), func(t *testing.T) {
				info, err := ParsePremineInfo(address + suffix)
				require.Nil(t, info)
				require.ErrorContains(t, err, "invalid premine address")
				require.ErrorContains(t, err, fmt.Sprintf("%q", address))
				require.NotNil(t, errors.Unwrap(err))
				require.NotContains(t, err.Error(), privateKey)
			})
		}
	}
}

func TestGenesisAddress_PremineAmountErrorDoesNotExposeKey(t *testing.T) {
	const privateKey = "PRIVATE_KEY_SENTINEL"
	address := "0x" + strings.Repeat("ab", 20)

	info, err := ParsePremineInfo(address + ":loremIpsum:" + privateKey)
	require.Nil(t, info)
	require.ErrorContains(t, err, "failed to parse amount")
	require.NotNil(t, errors.Unwrap(err))
	require.NotContains(t, err.Error(), privateKey)
}

/* FINISH ALPHA CHANGE 3133 -- test strict premine addresses while preserving amount and key handling -- */
