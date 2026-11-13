/* START ALPHA CHANGE 3184 -- verify strict external trie-root parsing and rejection before genesis publication -- */
package genesis

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func genesisTrieRootMalformedInputs() []struct {
	name string
	raw  string
} {
	return []struct {
		name string
		raw  string
	}{
		{"invalid_short", "0xzz"},
		{"empty_payload", "0x"},
		{"short_nonzero", "0x1"},
		{"short_zero", "0x0"},
		{"odd_length", "0x" + strings.Repeat("1", 63)},
		{"oversized_odd", "0x" + strings.Repeat("1", 65)},
		{"oversized_even", "0x" + strings.Repeat("11", 33)},
		{"partial_exact_width", "0x" + strings.Repeat("11", 31) + "zz"},
		{"prefixless_invalid", strings.Repeat("11", 31) + "zz"},
		{"valid_prefix_invalid_suffix", "0x" + strings.Repeat("11", 32) + "zz"},
		{"uppercase_prefix", "0X" + strings.Repeat("11", 32)},
		{"leading_whitespace", " 0x" + strings.Repeat("11", 32)},
		{"trailing_whitespace", "0x" + strings.Repeat("11", 32) + " "},
	}
}

func genesisTrieRootRequireOutput(
	t *testing.T,
	output string,
	existing bool,
	original []byte,
) {
	t.Helper()

	if existing {
		content, err := os.ReadFile(output)
		require.NoError(t, err)
		require.Equal(t, original, content)
		return
	}

	_, err := os.Stat(output)
	require.True(t, os.IsNotExist(err))
}

func TestGenesisTrieRoot_AcceptedFormats(t *testing.T) {
	full := types.BytesToHash(bytes.Repeat([]byte{0xab}, types.HashLength))

	cases := []struct {
		name string
		raw  string
		want types.Hash
	}{
		{"empty_default", "", types.ZeroHash},
		{"zero_prefixed", "0x" + strings.Repeat("0", 64), types.ZeroHash},
		{"zero_prefixless", strings.Repeat("0", 64), types.ZeroHash},
		{"prefixed", "0x" + strings.Repeat("ab", 32), full},
		{"prefixless", strings.Repeat("ab", 32), full},
		{"mixed_case", "0x" + strings.Repeat("Ab", 32), full},
		{"uppercase_digits", strings.Repeat("AB", 32), full},
		{
			"leading_zeros",
			"0x" + strings.Repeat("0", 60) + "1234",
			types.BytesToHash([]byte{0x12, 0x34}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &genesisParams{initialStateRoot: tc.raw}

			root, err := p.parseInitialTrieRoot()
			require.NoError(t, err)
			require.Equal(t, tc.want, root)
			require.Equal(t, tc.raw, p.initialStateRoot)
		})
	}
}

func TestGenesisTrieRoot_InvalidInputsPreserveConfiguration(t *testing.T) {
	for _, tc := range genesisTrieRootMalformedInputs() {
		t.Run(tc.name, func(t *testing.T) {
			previousConfig := &chain.Chain{Name: "unchanged"}
			p := &genesisParams{
				initialStateRoot: tc.raw,
				genesisConfig:    previousConfig,
			}

			root, err := p.parseInitialTrieRoot()
			require.ErrorContains(t, err, "invalid --trieroot")
			require.ErrorContains(t, err, fmt.Sprintf("%q", tc.raw))
			require.Equal(t, types.ZeroHash, root)
			require.Equal(t, tc.raw, p.initialStateRoot)
			require.Same(t, previousConfig, p.genesisConfig)
		})
	}
}

func TestGenesisTrieRoot_DecodeErrorIsWrapped(t *testing.T) {
	p := &genesisParams{
		initialStateRoot: "0x" + strings.Repeat("11", 31) + "zz",
	}

	root, err := p.parseInitialTrieRoot()
	require.Error(t, err)
	require.Equal(t, types.ZeroHash, root)

	var invalidByte hex.InvalidByteError
	require.ErrorAs(t, err, &invalidByte)
}

func TestGenesisTrieRoot_CommandRejectsBeforeGeneration(t *testing.T) {
	previous := params
	defer func() { params = previous }()

	for _, tc := range genesisTrieRootMalformedInputs() {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing_%t", tc.name, existing), func(t *testing.T) {
				output := filepath.Join(t.TempDir(), "genesis.json")
				original := []byte("existing genesis must remain unchanged")
				if existing {
					require.NoError(t, os.WriteFile(output, original, 0o600))
				}

				params = &genesisParams{}
				cmd := GetCommand()
				require.NoError(t, cmd.ParseFlags([]string{
					"--dir", output,
					"--trieroot", tc.raw,
				}))

				err := cmd.PreRunE(cmd, nil)
				require.ErrorContains(t, err, "invalid --trieroot")
				require.ErrorContains(t, err, fmt.Sprintf("%q", tc.raw))
				require.Equal(t, tc.raw, params.initialStateRoot)
				require.Nil(t, params.genesisConfig)

				genesisTrieRootRequireOutput(t, output, existing, original)
			})
		}
	}
}

func TestGenesisTrieRoot_DirectBuildersRejectBeforeWork(t *testing.T) {
	for _, tc := range genesisTrieRootMalformedInputs() {
		for _, builder := range []string{
			"init_generic",
			"generate_generic",
			"generate_innbcbft",
		} {
			for _, existing := range []bool{false, true} {
				name := fmt.Sprintf("%s/%s/existing_%t",
					tc.name, builder, existing)

				t.Run(name, func(t *testing.T) {
					output := filepath.Join(t.TempDir(), "genesis.json")
					original := []byte("existing genesis must remain unchanged")
					if existing {
						require.NoError(t, os.WriteFile(output, original, 0o600))
					}

					previousConfig := &chain.Chain{Name: "unchanged"}
					p := &genesisParams{
						/* START ALPHA CHANGE 3541 -- reach direct-builder trie-root validation with a valid chain ID -- */
						chainID: 1982,
						/* FINISH ALPHA CHANGE 3541 -- reach direct-builder trie-root validation with a valid chain ID -- */
						genesisPath:      output,
						initialStateRoot: tc.raw,
						genesisConfig:    previousConfig,
					}

					// Other construction inputs are deliberately absent.
					// Trie-root rejection must happen before accessing them.
					var err error
					switch builder {
					case "init_generic":
						err = p.initGenesisConfig()
					case "generate_generic":
						err = p.generateGenesis()
					case "generate_innbcbft":
						err = p.generatePolyBftChainConfig(nil)
					}

					require.ErrorContains(t, err, "invalid --trieroot")
					require.ErrorContains(t, err, fmt.Sprintf("%q", tc.raw))
					require.Equal(t, tc.raw, p.initialStateRoot)
					require.Same(t, previousConfig, p.genesisConfig)

					genesisTrieRootRequireOutput(t, output, existing, original)
				})
			}
		}
	}
}

func TestGenesisTrieRoot_GenerationRoundTrip(t *testing.T) {
	// Flag registration and the INNBC output path use package-global params.
	// Keep this test sequential and restore the previous pointer.
	previous := params
	defer func() { params = previous }()

	key, err := bls.UnmarshalPrivateKey([]byte("7"))
	require.NoError(t, err)

	operator := types.StringToAddress("0x10001")
	proof, err := signer.MakeINNBCBLSPoP(key, operator)
	require.NoError(t, err)

	proofBytes, err := proof.Marshal()
	require.NoError(t, err)

	/* START ALPHA CHANGE 3847 -- provide a complete peer identity in trie-root generation fixtures -- */
	validatorEntry := strings.Join([]string{
		"/ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAm22et517T1XhG181xGUDikDxUFrek4CuYTgUJjC38vxZR",
		operator.String(),
		"0x" + hex.EncodeToString(key.PublicKey().Marshal()),
		"0x" + hex.EncodeToString(proofBytes),
	}, ":")
	/* FINISH ALPHA CHANGE 3847 -- provide a complete peer identity in trie-root generation fixtures -- */

	full := types.BytesToHash(bytes.Repeat([]byte{0xab}, types.HashLength))

	cases := []struct {
		name string
		omit bool
		raw  string
		want types.Hash
	}{
		{"omitted", true, "", types.ZeroHash},
		{"explicit_empty", false, "", types.ZeroHash},
		{"zero_prefixed", false, "0x" + strings.Repeat("0", 64), types.ZeroHash},
		{"zero_prefixless", false, strings.Repeat("0", 64), types.ZeroHash},
		{"prefixed", false, "0x" + strings.Repeat("ab", 32), full},
		{"prefixless", false, strings.Repeat("ab", 32), full},
		{"mixed_case", false, "0x" + strings.Repeat("Ab", 32), full},
		{
			"leading_zeros",
			false,
			"0x" + strings.Repeat("0", 60) + "1234",
			types.BytesToHash([]byte{0x12, 0x34}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "genesis.json")

			params = &genesisParams{}
			cmd := GetCommand()
			require.Equal(t, "", cmd.Flags().Lookup(trieRootFlag).DefValue)

			args := []string{
				"--dir", output,
				"--validators", validatorEntry,
			}
			if !tc.omit {
				args = append(args, "--trieroot", tc.raw)
			}
			require.NoError(t, cmd.ParseFlags(args))

			p := params
			require.NoError(t, cmd.PreRunE(cmd, nil))
			require.NoError(t, p.generatePolyBftChainConfig(
				&genesisChainIDOutput{Writer: io.Discard},
			))
			require.Equal(t, tc.raw, p.initialStateRoot)

			before, err := os.ReadFile(output)
			require.NoError(t, err)

			imported, err := chain.ImportFromFile(output)
			require.NoError(t, err)

			config, err := innbcbft.GetINNBCBFTConfig(imported)
			require.NoError(t, err)
			require.Equal(t, tc.want, config.InitialTrieRoot)

			after, err := os.ReadFile(output)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

/* FINISH ALPHA CHANGE 3184 -- verify strict external trie-root parsing and rejection before genesis publication -- */
