/* START ALPHA CHANGE 3123 -- test premine aggregate rejection through both genesis builders -- */
package genesis

import (
	"encoding/hex"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisSupply_CommandGeneration(t *testing.T) {
	previous := params
	defer func() { params = previous }()

	key, err := bls.UnmarshalPrivateKey([]byte("7"))
	require.NoError(t, err)
	operator := types.StringToAddress("0x10001")
	proof, err := signer.MakeINNBCBLSPoP(key, operator)
	require.NoError(t, err)
	proofBytes, err := proof.Marshal()
	require.NoError(t, err)

	/* START ALPHA CHANGE 3844 -- provide a complete peer identity in supply-generation fixtures -- */
	entry := strings.Join([]string{
		"/ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAm22et517T1XhG181xGUDikDxUFrek4CuYTgUJjC38vxZR",
		operator.String(),
		"0x" + hex.EncodeToString(key.PublicKey().Marshal()),
		"0x" + hex.EncodeToString(proofBytes),
	}, ":")
	/* FINISH ALPHA CHANGE 3844 -- provide a complete peer identity in supply-generation fixtures -- */

	a := types.StringToAddress("0xb101")
	b := types.StringToAddress("0xb102")
	maximum := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1),
	)
	maximumMinusOne := new(big.Int).Sub(
		new(big.Int).Set(maximum), big.NewInt(1),
	)

	cases := []struct {
		name    string
		premine []string
		wantErr bool
		wantA   *big.Int
	}{
		{
			name: "exact_maximum",
			premine: []string{
				a.String() + ":" + maximumMinusOne.String(),
				b.String() + ":1",
			},
			wantA: maximumMinusOne,
		},
		{
			name: "maximum_plus_one",
			premine: []string{
				a.String() + ":" + maximum.String(),
				b.String() + ":1",
			},
			wantErr: true,
		},
		{
			name: "validate_final_map_after_duplicate_address",
			premine: []string{
				a.String() + ":" + maximum.String(),
				a.String() + ":1",
				b.String() + ":1",
			},
			wantA: big.NewInt(1),
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			for _, builder := range []string{"generic", "innbcbft"} {
				t.Run(builder, func(t *testing.T) {
					output := filepath.Join(t.TempDir(), "genesis.json")
					p := &genesisParams{
						name:                     "genesis-supply-regression",
						chainID:                  uint64(command.DefaultChainID),
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
						minNumValidators:                 1,
						maxNumValidators:                 4,
						validators:                       []string{entry},
						premine:                          append([]string(nil), tt.premine...),
						validatorRegistryMinimumStakeRaw: "1",
						validatorRegistryProposalDurationPresetRaw: innbcbft.ValidatorRegistryProposalDurationStandard,
					}
					params = p

					require.NoError(t, p.validateFlags())
					require.NoError(t, p.initRawParams())

					if builder == "generic" {
						/* START ALPHA CHANGE 3327 -- supply explicit engine clock configuration to the generic supply fixture -- */
						p.consensusEngineConfig[p.consensusRaw] = map[string]interface{}{
							"blockTime": p.blockTime.String(),
						}
						/* FINISH ALPHA CHANGE 3327 -- supply explicit engine clock configuration to the generic supply fixture -- */
						err = p.generateGenesis()
					} else {
						err = p.generatePolyBftChainConfig(
							&genesisChainIDOutput{Writer: io.Discard},
						)
					}

					require.Equal(t, tt.premine, p.premine)
					if tt.wantErr {
						require.ErrorContains(t, err,
							"initial native-token supply exceeds unsigned 256-bit range")
						_, statErr := os.Stat(output)
						require.True(t, os.IsNotExist(statErr))
						return
					}

					require.NoError(t, err)
					/* START ALPHA CHANGE 3322 -- generated genesis must omit the retired drift field -- */
					rawGenesis, readErr := os.ReadFile(output)
					require.NoError(t, readErr)
					require.NotContains(t, string(rawGenesis), `"blockTimeDrift"`)
					/* FINISH ALPHA CHANGE 3322 -- generated genesis must omit the retired drift field -- */
					imported, importErr := chain.ImportFromFile(output)
					require.NoError(t, importErr)
					require.NoError(t, imported.ValidateGenesisAllocations())
					require.Zero(t, tt.wantA.Cmp(imported.Genesis.Alloc[a].Balance))
					require.Zero(t, big.NewInt(1).Cmp(imported.Genesis.Alloc[b].Balance))
				})
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3123 -- test premine aggregate rejection through both genesis builders -- */
