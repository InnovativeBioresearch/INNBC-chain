/* START ALPHA CHANGE 3175 -- verify typed hexadecimal rejection and preserve valid conversion compatibility -- */
package types

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypedHex_HashAcceptedFormats(t *testing.T) {
	tail := bytes.Repeat([]byte{0xab}, HashLength)

	cases := []struct {
		name  string
		input string
		want  Hash
	}{
		{"empty", "", ZeroHash},
		{"empty_prefixed", "0x", ZeroHash},
		{"single_digit", "0x1", BytesToHash([]byte{1})},
		{"prefixless_even", "1234", BytesToHash([]byte{0x12, 0x34})},
		{"prefixed_odd", "0xabc", BytesToHash([]byte{0x0a, 0xbc})},
		{"prefixless_odd", "abc", BytesToHash([]byte{0x0a, 0xbc})},
		{"uppercase_digits", "0xAbCd", BytesToHash([]byte{0xab, 0xcd})},
		{"leading_zeros", "0x0001", BytesToHash([]byte{1})},
		{"exact_length", "0x" + strings.Repeat("ab", HashLength), BytesToHash(tail)},
		{"oversized", "0xbb" + strings.Repeat("ab", HashLength), BytesToHash(tail)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := BytesToHash([]byte{0xee})
			require.NoError(t, value.UnmarshalText([]byte(tc.input)))
			require.Equal(t, tc.want, value)

			encoded, err := json.Marshal(tc.input)
			require.NoError(t, err)

			value = BytesToHash([]byte{0xee})
			require.NoError(t, json.Unmarshal(encoded, &value))
			require.Equal(t, tc.want, value)

			roundTrip, err := json.Marshal(value)
			require.NoError(t, err)

			var decoded Hash
			require.NoError(t, json.Unmarshal(roundTrip, &decoded))
			require.Equal(t, tc.want, decoded)
		})
	}
}

func TestTypedHex_AddressAcceptedFormats(t *testing.T) {
	full := bytes.Repeat([]byte{0x11}, AddressLength)
	odd := append([]byte{0x01}, bytes.Repeat([]byte{0x11}, AddressLength-1)...)

	cases := []struct {
		name  string
		input string
		want  Address
	}{
		{"prefixed", "0x" + strings.Repeat("11", AddressLength), BytesToAddress(full)},
		{"prefixless", strings.Repeat("11", AddressLength), BytesToAddress(full)},
		{"mixed_case", "0x" + strings.Repeat("Ab", AddressLength),
			BytesToAddress(bytes.Repeat([]byte{0xab}, AddressLength))},
		{"odd_prefixed", "0x" + strings.Repeat("1", 39), BytesToAddress(odd)},
		{"odd_prefixless", strings.Repeat("1", 39), BytesToAddress(odd)},
		{"zero", "0x" + strings.Repeat("0", 40), ZeroAddress},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := BytesToAddress([]byte{0xee})
			require.NoError(t, value.UnmarshalText([]byte(tc.input)))
			require.Equal(t, tc.want, value)

			encoded, err := json.Marshal(tc.input)
			require.NoError(t, err)

			value = BytesToAddress([]byte{0xee})
			require.NoError(t, json.Unmarshal(encoded, &value))
			require.Equal(t, tc.want, value)

			roundTrip, err := json.Marshal(value)
			require.NoError(t, err)

			var decoded Address
			require.NoError(t, json.Unmarshal(roundTrip, &decoded))
			require.Equal(t, tc.want, decoded)
		})
	}
}

func TestTypedHex_InvalidCharactersPreserveReceivers(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"invalid", "0xzz"},
		{"partial", "0x1234zz"},
		{"prefixless_partial", "12zz"},
		{"underscore", "0x12_34"},
		{"whitespace", "0x12 34"},
		{"uppercase_prefix", "0X12"},
		{"address_suffix", "0x" + strings.Repeat("11", AddressLength) + "zz"},
		{"hash_suffix", "0x" + strings.Repeat("11", HashLength) + "zz"},
		{"oversized_invalid_leading", "0xzz" + strings.Repeat("11", HashLength+1)},
		{"oversized_invalid_middle", "0x11zz" + strings.Repeat("11", HashLength+1)},
	}

	for _, tc := range cases {
		for _, throughJSON := range []bool{false, true} {
			mode := "text"
			if throughJSON {
				mode = "json"
			}

			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				hash := BytesToHash([]byte{0xaa, 0xbb})
				address := BytesToAddress([]byte{0xcc, 0xdd})
				beforeHash := hash
				beforeAddress := address

				var hashErr, addressErr error
				if throughJSON {
					encoded, err := json.Marshal(tc.input)
					require.NoError(t, err)

					hashErr = json.Unmarshal(encoded, &hash)
					addressErr = json.Unmarshal(encoded, &address)
				} else {
					hashErr = hash.UnmarshalText([]byte(tc.input))
					addressErr = address.UnmarshalText([]byte(tc.input))
				}

				require.Error(t, hashErr)
				require.Error(t, addressErr)
				require.Equal(t, beforeHash, hash)
				require.Equal(t, beforeAddress, address)

				var invalidHashByte hex.InvalidByteError
				var invalidAddressByte hex.InvalidByteError
				require.ErrorAs(t, hashErr, &invalidHashByte)
				require.ErrorAs(t, addressErr, &invalidAddressByte)
			})
		}
	}
}

func TestTypedHex_AddressLengthFailuresPreserveReceiver(t *testing.T) {
	for _, input := range []string{
		"",
		"0x",
		"0x1",
		"0x" + strings.Repeat("11", AddressLength-1),
		"0x" + strings.Repeat("11", AddressLength+1),
	} {
		t.Run(input, func(t *testing.T) {
			value := BytesToAddress([]byte{0xaa})
			before := value

			require.Error(t, value.UnmarshalText([]byte(input)))
			require.Equal(t, before, value)

			encoded, err := json.Marshal(input)
			require.NoError(t, err)

			require.Error(t, json.Unmarshal(encoded, &value))
			require.Equal(t, before, value)
		})
	}
}

func TestTypedHex_OptionalJSONFields(t *testing.T) {
	type fields struct {
		Address *Address `json:"address"`
		Hash    *Hash    `json:"hash"`
	}

	for _, input := range []string{
		`{}`,
		`{"address":null,"hash":null}`,
	} {
		var decoded fields
		require.NoError(t, json.Unmarshal([]byte(input), &decoded))
		require.Nil(t, decoded.Address)
		require.Nil(t, decoded.Hash)
	}

	var decoded fields
	require.NoError(t, json.Unmarshal([]byte(
		`{"address":"0x0000000000000000000000000000000000000000","hash":"0x"}`,
	), &decoded))
	require.NotNil(t, decoded.Address)
	require.NotNil(t, decoded.Hash)
	require.Equal(t, ZeroAddress, *decoded.Address)
	require.Equal(t, ZeroHash, *decoded.Hash)
}

func TestTypedHex_ConvenienceConversionsUnchanged(t *testing.T) {
	require.Equal(t, []byte{0x12, 0x34}, StringToBytes("0x1234zz"))
	require.Equal(t, BytesToHash([]byte{0x12, 0x34}), StringToHash("0x1234zz"))
	require.Equal(t, BytesToAddress([]byte{0x12, 0x34}), StringToAddress("0x1234zz"))

	require.Equal(t, ZeroHash, StringToHash("0xzz"))
	require.Equal(t, ZeroAddress, StringToAddress("0xzz"))

	require.Equal(t,
		BytesToHash(bytes.Repeat([]byte{0xab}, HashLength)),
		StringToHash("0xbb"+strings.Repeat("ab", HashLength)),
	)
}

/* FINISH ALPHA CHANGE 3175 -- verify typed hexadecimal rejection and preserve valid conversion compatibility -- */
