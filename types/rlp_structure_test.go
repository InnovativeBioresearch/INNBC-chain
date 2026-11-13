/* START ALPHA CHANGE 2978 -- verify bounded RLP structure and the supported dynamic block encoding -- */
package types

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRLPStructure_Boundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input []byte
		depth int
		want  error
	}{
		{"single byte", []byte{0x00}, 4, nil},
		{"empty string", []byte{0x80}, 4, nil},
		{"empty list", []byte{0xc0}, 4, nil},
		{"four list levels", []byte{0xc3, 0xc2, 0xc1, 0xc0}, 4, nil},
		{"five list levels", []byte{0xc4, 0xc3, 0xc2, 0xc1, 0xc0}, 4, ErrRLPNestingTooDeep},
		{"empty input", nil, 4, ErrInvalidRLPStructure},
		{"invalid depth", []byte{0xc0}, 0, ErrInvalidRLPStructure},
		{"unsupported depth", []byte{0xc0}, 65, ErrInvalidRLPStructure},
		{"trailing item", []byte{0x80, 0x80}, 4, ErrInvalidRLPStructure},
		{"child crosses parent", []byte{0xc1, 0x81, 0x80}, 4, ErrInvalidRLPStructure},
		{"missing length", []byte{0xb8}, 4, ErrInvalidRLPStructure},
		{"missing payload", []byte{0x81}, 4, ErrInvalidRLPStructure},
		{"noncanonical single byte", []byte{0x81, 0x00}, 4, ErrInvalidRLPStructure},
		{"unnecessary long form", []byte{0xf8, 0x01, 0xc0}, 4, ErrInvalidRLPStructure},
		{"leading zero length", []byte{0xf9, 0x00, 0x38}, 4, ErrInvalidRLPStructure},
		{
			"maximum unsigned length",
			[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			4,
			ErrInvalidRLPStructure,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]byte(nil), tc.input...)
			err := ValidateRLPStructure(tc.input, tc.depth)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
			require.True(t, bytes.Equal(before, tc.input))
		})
	}

	// A byte-string payload is opaque, even when its bytes look like lists.
	opaque := append([]byte{0xa0}, bytes.Repeat([]byte{0xff}, 32)...)
	require.NoError(t, ValidateRLPStructure(opaque, 1))

	longString := append([]byte{0xb8, 0x38}, bytes.Repeat([]byte{0x00}, 56)...)
	require.NoError(t, ValidateRLPStructure(longString, 1))

	longList := append([]byte{0xf8, 0x38}, bytes.Repeat([]byte{0x80}, 56)...)
	require.NoError(t, ValidateRLPStructure(longList, 1))
}

func TestRLPStructure_CurrentDynamicBlock(t *testing.T) {
	to := BytesToAddress([]byte{1})
	block := &Block{
		Header: &Header{Number: 1},
		Transactions: []*Transaction{
			{
				Type:      DynamicFeeTx,
				ChainID:   big.NewInt(1),
				GasTipCap: big.NewInt(1),
				GasFeeCap: big.NewInt(2),
				Gas:       21000,
				To:        &to,
				Value:     big.NewInt(0),
				V:         big.NewInt(0),
				R:         big.NewInt(1),
				S:         big.NewInt(1),
			},
		},
	}

	// This is an encoding fixture, not a valid signed/executed block.
	data := block.MarshalRLP()
	require.NoError(t, ValidateRLPStructure(data, BlockRLPMaxDepth))
	require.ErrorIs(t, ValidateRLPStructure(data, 3), ErrRLPNestingTooDeep)

	var decoded Block
	require.NoError(t, decoded.UnmarshalRLP(data))
	require.Len(t, decoded.Transactions, 1)
	require.Equal(t, DynamicFeeTx, decoded.Transactions[0].Type)
	require.Zero(t, decoded.Transactions[0].ChainID.Cmp(big.NewInt(1)))
}

/* FINISH ALPHA CHANGE 2978 -- verify bounded RLP structure and the supported dynamic block encoding -- */
