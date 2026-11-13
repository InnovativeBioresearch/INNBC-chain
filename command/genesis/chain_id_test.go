/* START ALPHA CHANGE 3053 -- test genesis chain ID bounds defaults and generation -- */
package genesis

import (
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/server"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisChainID_BoundariesAndBuilderRejection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		id    uint64
		valid bool
	}{
		/* START ALPHA CHANGE 3534 -- reject zero and retain positive genesis chain ID boundaries -- */
		{"zero", 0, false},
		{"minimum_supported", 1, true},
		{"previous_default", 100, true},
		{"production", 1982, true},
		{"maximum_supported", math.MaxInt64, true},
		{"first_unsupported", uint64(math.MaxInt64) + 1, false},
		{"maximum_unsigned", math.MaxUint64, false},
		/* FINISH ALPHA CHANGE 3534 -- reject zero and retain positive genesis chain ID boundaries -- */
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "genesis.json")
			p := &genesisParams{
				chainID:      tc.id,
				consensusRaw: string(server.INNBCBFTConsensus),
				genesisPath:  output,
				blockTime:    2 * time.Second,
			}

			err := p.validateChainID()
			require.Equal(t, tc.id, p.chainID)

			if tc.valid {
				require.NoError(t, err)
				return
			}

			/* START ALPHA CHANGE 3534 -- expect the positive genesis chain ID range -- */
			expected := fmt.Sprintf(
				"--chain-id must be between 1 and %d, got %d",
				int64(math.MaxInt64), tc.id,
			)
			/* FINISH ALPHA CHANGE 3534 -- expect the positive genesis chain ID range -- */
			require.EqualError(t, err, expected)
			require.EqualError(t, p.validateFlags(), expected)

			// These direct calls must reject before needing other parameters.
			require.EqualError(t, p.initGenesisConfig(), expected)
			require.Nil(t, p.genesisConfig)
			require.EqualError(t, p.generatePolyBftChainConfig(nil), expected)

			_, statErr := os.Stat(output)
			require.True(t, os.IsNotExist(statErr))
		})
	}
}

func TestGenesisChainID_FlagsDefaultAndOverrides(t *testing.T) {
	// Flag registration uses package-global params. Keep this test sequential
	// and restore the previous pointer without modifying its contents.
	previous := params
	defer func() { params = previous }()

	require.EqualValues(t, 1982, command.DefaultChainID)

	for _, tc := range []struct {
		name  string
		args  []string
		id    uint64
		valid bool
	}{
		/* START ALPHA CHANGE 3534 -- test current and legacy zero rejection without changing defaults -- */
		{"default", nil, 1982, true},
		{"explicit_minimum", []string{"--chain-id", "1"}, 1, true},
		{"explicit_previous_default", []string{"--chain-id", "100"}, 100, true},
		{"explicit_zero", []string{"--chain-id", "0"}, 0, false},
		{"legacy_zero", []string{"--chainid", "0"}, 0, false},
		/* FINISH ALPHA CHANGE 3534 -- test current and legacy zero rejection without changing defaults -- */
		{
			"explicit_maximum",
			[]string{"--chain-id", strconv.FormatUint(math.MaxInt64, 10)},
			math.MaxInt64,
			true,
		},
		{"legacy_override", []string{"--chainid", "100"}, 100, true},
		{
			"oversized_current_flag",
			[]string{"--chain-id", "9223372036854775808"},
			uint64(math.MaxInt64) + 1,
			false,
		},
		{
			"oversized_legacy_flag",
			[]string{"--chainid", "18446744073709551615"},
			math.MaxUint64,
			false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params = &genesisParams{}
			cmd := GetCommand()

			require.Equal(t, "1982", cmd.Flags().Lookup(chainIDFlag).DefValue)
			require.Equal(t, "1982", cmd.Flags().Lookup(chainIDFlagLEGACY).DefValue)
			require.NoError(t, cmd.Flags().Parse(tc.args))
			require.Equal(t, tc.id, params.chainID)

			if tc.valid {
				require.NoError(t, params.validateChainID())
				return
			}

			/* START ALPHA CHANGE 3534 -- verify positive chain ID validation at the pre-run boundary -- */
			// Exercise the command's real pre-run validation boundary.
			require.ErrorContains(t, preRunCommand(cmd, nil),
				"--chain-id must be between 1 and 9223372036854775807")
			/* FINISH ALPHA CHANGE 3534 -- verify positive chain ID validation at the pre-run boundary -- */
		})
	}
}

type genesisChainIDOutput struct {
	io.Writer
}

func (*genesisChainIDOutput) SetError(error)                           {}
func (*genesisChainIDOutput) SetCommandResult(command.CommandResult)   {}
func (*genesisChainIDOutput) WriteOutput()                             {}
func (*genesisChainIDOutput) WriteCommandResult(command.CommandResult) {}

func TestGenesisChainID_GenerationRoundTrip(t *testing.T) {
	// The INNBC builder currently uses package-global params for its output
	// path. Bind that pointer to each fixture and restore it after the test.
	previous := params
	defer func() { params = previous }()

	key, err := bls.UnmarshalPrivateKey([]byte("7"))
	require.NoError(t, err)
	operator := types.StringToAddress("0x10001")
	proof, err := signer.MakeINNBCBLSPoP(key, operator)
	require.NoError(t, err)
	proofBytes, err := proof.Marshal()
	require.NoError(t, err)

	/* START ALPHA CHANGE 3845 -- provide a complete peer identity in chain-ID generation fixtures -- */
	entry := strings.Join([]string{
		"/ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAm22et517T1XhG181xGUDikDxUFrek4CuYTgUJjC38vxZR",
		operator.String(),
		"0x" + hex.EncodeToString(key.PublicKey().Marshal()),
		"0x" + hex.EncodeToString(proofBytes),
	}, ":")
	/* FINISH ALPHA CHANGE 3845 -- provide a complete peer identity in chain-ID generation fixtures -- */

	/* START ALPHA CHANGE 3534 -- round-trip the supported positive chain ID boundaries -- */
	for _, id := range []uint64{1, 100, 1982, math.MaxInt64} {
		/* FINISH ALPHA CHANGE 3534 -- round-trip the supported positive chain ID boundaries -- */
		t.Run(strconv.FormatUint(id, 10), func(t *testing.T) {
			for _, builder := range []string{"generic", "innbcbft"} {
				t.Run(builder, func(t *testing.T) {
					output := filepath.Join(t.TempDir(), "genesis.json")
					p := &genesisParams{
						name:                     "chain-id-regression",
						chainID:                  id,
						consensusRaw:             innbcbft.ConsensusName,
						genesisPath:              output,
						blockTime:                2 * time.Second,
						blockTrackerPollInterval: time.Second,
						blockGasLimit:            command.DefaultGenesisGasLimit,
						baseFeeConfig:            command.DefaultGenesisBaseFeeConfig,
						epochSize:                defaultEpochSize,
						sprintSize:               defaultSprintSize,
						/* START ALPHA CHANGE 3321 -- remove the retired generator fixture parameter -- */
						/* FINISH ALPHA CHANGE 3321 -- remove the retired generator fixture parameter -- */
						minNumValidators:                           1,
						maxNumValidators:                           4,
						validators:                                 []string{entry},
						validatorRegistryMinimumStakeRaw:           "1",
						validatorRegistryProposalDurationPresetRaw: innbcbft.ValidatorRegistryProposalDurationStandard,
					}
					params = p

					require.NoError(t, p.validateFlags())
					require.NoError(t, p.initRawParams())

					if builder == "generic" {
						/* START ALPHA CHANGE 3327 -- supply explicit engine clock configuration to the generic generator fixture -- */
						p.consensusEngineConfig[p.consensusRaw] = map[string]interface{}{
							"blockTime": p.blockTime.String(),
						}
						/* FINISH ALPHA CHANGE 3327 -- supply explicit engine clock configuration to the generic generator fixture -- */
						require.NoError(t, p.generateGenesis())
					} else {
						require.NoError(t, p.generatePolyBftChainConfig(
							&genesisChainIDOutput{Writer: io.Discard},
						))
					}

					before, err := os.ReadFile(output)
					require.NoError(t, err)

					imported, err := chain.ImportFromFile(output)
					require.NoError(t, err)
					require.Equal(t, int64(id), imported.Params.ChainID)
					require.NoError(t, imported.ValidateChainID())
					require.Equal(t, id, p.chainID)

					/* START ALPHA CHANGE 3429 -- verify generated INNBC genesis omits VSET stake and retains proved identities -- */
					if builder == "innbcbft" {
						require.NotContains(t, string(before), `"stake"`)

						config, err := innbcbft.GetINNBCBFTConfig(imported)
						require.NoError(t, err)
						require.Len(t, config.InitialValidatorSet, 1)

						metadata, err := config.InitialValidatorSet[0].ToValidatorMetadata()
						require.NoError(t, err)
						require.Equal(t, operator, metadata.Address)
						require.Equal(t, "1", metadata.VotingPower.String())
						require.True(t, metadata.IsActive)
					}
					/* FINISH ALPHA CHANGE 3429 -- verify generated INNBC genesis omits VSET stake and retains proved identities -- */

					after, err := os.ReadFile(output)
					require.NoError(t, err)
					require.Equal(t, before, after)
				})
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3053 -- test genesis chain ID bounds defaults and generation -- */
