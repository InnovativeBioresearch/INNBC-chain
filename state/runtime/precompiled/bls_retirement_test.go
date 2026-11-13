/* START ALPHA CHANGE 2918 -- verify aggregate BLS retirement and preserved precompile registration -- */
package precompiled

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestBLSRetirement_Unregistered(t *testing.T) {
	retired := types.StringToAddress("0x2030")

	tests := []struct {
		name  string
		forks chain.ForksInTime
	}{
		{
			name: "without_optional_forks",
		},
		{
			name: "byzantium_and_istanbul",
			forks: chain.ForksInTime{
				Byzantium: true,
				Istanbul:  true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPrecompiled()

			require.NotContains(t, p.contracts, retired)
			require.False(t, p.CanRun(
				&runtime.Contract{CodeAddress: retired},
				nil,
				&tc.forks,
			))
		})
	}
}

func TestBLSRetirement_PreservedPrecompiles(t *testing.T) {
	p := NewPrecompiled()
	forks := chain.ForksInTime{
		Byzantium: true,
		Istanbul:  true,
	}

	require.IsType(
		t,
		&nativeTransfer{},
		p.contracts[contracts.NativeTransferPrecompile],
	)

	tests := []struct {
		name    string
		address types.Address
	}{
		{
			name:    "native_transfer",
			address: contracts.NativeTransferPrecompile,
		},
		{
			name:    "bn256_add",
			address: types.StringToAddress("0x06"),
		},
		{
			name:    "bn256_mul",
			address: types.StringToAddress("0x07"),
		},
		{
			name:    "bn256_pairing",
			address: types.StringToAddress("0x08"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Contains(t, p.contracts, tc.address)
			require.True(t, p.CanRun(
				&runtime.Contract{CodeAddress: tc.address},
				nil,
				&forks,
			))
		})
	}
}

/* FINISH ALPHA CHANGE 2918 -- verify aggregate BLS retirement and preserved precompile registration -- */
