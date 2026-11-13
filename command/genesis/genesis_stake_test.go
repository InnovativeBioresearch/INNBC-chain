package genesis

import (
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2649 -- test genesis command rejection of unsupported minimum stakes -- */

func TestGenesisMinimumStake_Parameters(t *testing.T) {
	cases := []struct {
		raw   string
		valid bool
	}{
		{"", false},
		{"invalid", false},
		{"-1", false},
		{"0", false},
		{"1", true},
		{" 1 ", true},
		{"01", true},
		{"2", false},
		{"100", false},
		{"1000000000000000000", false},
		{"18446744073709551617", false},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			p := &genesisParams{
				validatorRegistryMinimumStakeRaw:           tc.raw,
				validatorRegistryProposalDurationPresetRaw: innbcbft.ValidatorRegistryProposalDurationStandard,
			}

			err := p.validateValidatorRegistryParams()
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, p.validatorRegistryMinimumStake)
				require.Zero(t, p.validatorRegistryMinimumStake.Cmp(big.NewInt(1)))

				return
			}

			require.ErrorIs(t, err, innbcbft.ErrInvalidGenesisMinimumStake)
			require.Nil(t, p.validatorRegistryMinimumStake)
		})
	}
}

func TestGenesisMinimumStake_ValidateFlags(t *testing.T) {
	for _, raw := range []string{"", "-1", "0", "2", "100", "1000000000000000000"} {
		t.Run(raw, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "genesis.json")

			p := &genesisParams{
				/* START ALPHA CHANGE 3539 -- reach minimum-stake validation with a valid chain ID -- */
				chainID: 1982,
				/* FINISH ALPHA CHANGE 3539 -- reach minimum-stake validation with a valid chain ID -- */
				consensusRaw:                     innbcbft.ConsensusName,
				genesisPath:                      output,
				blockTime:                        2 * time.Second,
				blockTrackerPollInterval:         time.Second,
				blockGasLimit:                    types.StateTransactionGasLimit,
				baseFeeConfig:                    "1:2:8",
				validatorRegistryMinimumStakeRaw: raw,
				validatorRegistryProposalDurationPresetRaw: innbcbft.ValidatorRegistryProposalDurationStandard,
			}

			require.ErrorIs(t, p.validateFlags(), innbcbft.ErrInvalidGenesisMinimumStake)

			_, err := os.Stat(output)
			require.True(t, os.IsNotExist(err))
		})
	}
}

/* FINISH ALPHA CHANGE 2649 -- test genesis command rejection of unsupported minimum stakes -- */
