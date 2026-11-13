/* START ALPHA CHANGE 3872 -- regress generation-only genesis peer identity uniqueness -- */

package genesis

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	networkcommon "github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/innovativebioresearch/innbc-chain/secrets"
	"github.com/stretchr/testify/require"
)

func TestGenesisPeerIDUniqueness_CommandGeneration(t *testing.T) {
	// Genesis commands use package-global params. Keep these cases sequential.
	previous := params
	t.Cleanup(func() {
		params = previous
	})

	cases := []struct {
		name               string
		count              int
		duplicate          bool
		differentTransport bool
	}{
		{
			name:  "single_validator",
			count: 1,
		},
		{
			name:  "distinct_peer_ids",
			count: 2,
		},
		{
			name:      "duplicate_same_address",
			count:     2,
			duplicate: true,
		},
		{
			name:               "duplicate_different_transport",
			count:              2,
			duplicate:          true,
			differentTransport: true,
		},
	}

	for _, workflow := range []string{"explicit", "directory"} {
		t.Run(workflow, func(t *testing.T) {
			for _, tc := range cases {
				// Directory input contains identity-only addresses.
				if workflow == "directory" && tc.differentTransport {
					continue
				}

				t.Run(tc.name, func(t *testing.T) {
					root := t.TempDir()
					var firstNetworkKey []byte

					for index := range tc.count {
						directory := filepath.Join(
							root,
							fmt.Sprintf("validator-%d", index+1),
						)

						withNetwork := !tc.duplicate || index == 0
						manager, _, _ := genesisBootstrapSecrets(
							t, directory, withNetwork,
						)

						if index == 0 {
							key, err := manager.GetSecret(secrets.NetworkKey)
							require.NoError(t, err)
							firstNetworkKey = key
						} else if tc.duplicate {
							// Share only the network key. Operator and BLS
							// identities remain independently generated.
							require.NoError(t, manager.SetSecret(
								secrets.NetworkKey,
								firstNetworkKey,
							))
						}
					}

					entries, err := ReadValidatorsByPrefix(root, "validator-")
					require.NoError(t, err)
					require.Len(t, entries, tc.count)

					if tc.count > 1 {
						require.NotEqual(t, entries[0].Address, entries[1].Address)
						require.NotEqual(t, entries[0].BlsKey, entries[1].BlsKey)
					}

					output := filepath.Join(root, "genesis.json")
					args := []string{"--dir", output}

					if workflow == "explicit" {
						for index, entry := range entries {
							transport := "/ip4/127.0.0.1/tcp/30301"
							if tc.differentTransport && index > 0 {
								transport = "/ip4/127.0.0.2/tcp/30302"
							}

							entry.MultiAddr = transport + entry.MultiAddr
							args = append(args, "--validators", strings.Join(
								[]string{
									entry.MultiAddr,
									entry.Address.String(),
									"0x" + entry.BlsKey,
									"0x" + entry.BlsProof,
								},
								":",
							))
						}
					} else {
						args = append(args,
							"--validators-path", root,
							"--validators-prefix", "validator-",
						)
					}

					params = &genesisParams{}
					cmd := GetCommand()
					require.NoError(t, cmd.ParseFlags(args))
					require.NoError(t, cmd.PreRunE(cmd, nil))

					err = params.generatePolyBftChainConfig(
						&genesisChainIDOutput{Writer: io.Discard},
					)

					if tc.duplicate {
						info, parseErr := networkcommon.StringToAddrInfo(
							entries[0].MultiAddr,
						)
						require.NoError(t, parseErr)

						require.ErrorContains(t, err, "duplicate genesis peer ID")
						require.ErrorContains(t, err, info.ID.String())
						require.ErrorContains(t, err, entries[0].Address.String())
						require.ErrorContains(t, err, entries[1].Address.String())

						_, statErr := os.Stat(output)
						require.ErrorIs(t, statErr, os.ErrNotExist)

						return
					}

					require.NoError(t, err)

					generated, err := chain.ImportFromFile(output)
					require.NoError(t, err)

					config, err := innbcbft.GetINNBCBFTConfig(generated)
					require.NoError(t, err)
					require.Len(t, config.InitialValidatorSet, len(entries))

					for index, entry := range config.InitialValidatorSet {
						require.Equal(t, entries[index].Address, entry.Address)
						require.Equal(t, entries[index].BlsKey, entry.BlsKey)
						require.Equal(t, entries[index].BlsProof, entry.BlsProof)
						require.Equal(t, entries[index].MultiAddr, entry.MultiAddr)
					}
				})
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3872 -- regress generation-only genesis peer identity uniqueness -- */
