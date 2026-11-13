/* START ALPHA CHANGE 3094 -- decode unsigned 256-bit RPC quantities with bounded parsing -- */
package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/big"
)

var errInvalidUint256 = errors.New(
	"value must be an unsigned integer between 0 and 2^256-1",
)

type argUint256 big.Int

func (a *argUint256) UnmarshalText(input []byte) error {
	base := 10
	maxDigits := 78

	if len(input) >= 2 && input[0] == '0' && input[1] == 'x' {
		base = 16
		maxDigits = 64
		input = input[2:]
	}

	if len(input) == 0 {
		return errInvalidUint256
	}

	// Leading zeros do not increase the significant width. Scanning them
	// does not allocate a large integer.
	first := 0
	for first < len(input) && input[first] == '0' {
		first++
	}
	significant := input[first:]

	if len(significant) > maxDigits {
		return errInvalidUint256
	}

	for _, digit := range significant {
		valid := digit >= '0' && digit <= '9'
		if base == 16 {
			valid = valid ||
				(digit >= 'a' && digit <= 'f') ||
				(digit >= 'A' && digit <= 'F')
		}
		if !valid {
			return errInvalidUint256
		}
	}

	value := new(big.Int)
	if len(significant) != 0 {
		if _, ok := value.SetString(string(significant), base); !ok {
			return errInvalidUint256
		}
	}

	// A 78-digit decimal value can still exceed the uint256 maximum.
	if value.BitLen() > 256 {
		return errInvalidUint256
	}

	// Assign only after complete validation, preserving the receiver on error.
	*a = argUint256(*value)

	return nil
}

func (a *argUint256) UnmarshalJSON(input []byte) error {
	input = bytes.TrimSpace(input)
	if len(input) == 0 {
		return errInvalidUint256
	}

	if input[0] == '"' {
		var text string
		if err := json.Unmarshal(input, &text); err != nil {
			return err
		}
		return a.UnmarshalText([]byte(text))
	}

	// Integer JSON tokens are decimal. Reject other token types, signs,
	// fractions, exponent notation and invalid leading-zero number tokens.
	if input[0] < '0' || input[0] > '9' {
		return errInvalidUint256
	}
	if len(input) > 1 && input[0] == '0' {
		return errInvalidUint256
	}

	return a.UnmarshalText(input)
}

func (a argUint256) MarshalText() ([]byte, error) {
	value := (*big.Int)(&a)
	if value.Sign() < 0 || value.BitLen() > 256 {
		return nil, errInvalidUint256
	}

	return []byte("0x" + value.Text(16)), nil
}

/* FINISH ALPHA CHANGE 3094 -- decode unsigned 256-bit RPC quantities with bounded parsing -- */
