/* START ALPHA CHANGE 3180 -- verify genesis access-list rejection before generation and preserve valid configuration -- */
package genesis

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type genesisAccessListTestFlag struct {
	name  string
	field func(*genesisParams) *[]string
}

func genesisAccessListTestFlags() []genesisAccessListTestFlag {
	return []genesisAccessListTestFlag{
		{contractDeployerAllowListAdminFlag,
			func(p *genesisParams) *[]string { return &p.contractDeployerAllowListAdmin }},
		{contractDeployerAllowListEnabledFlag,
			func(p *genesisParams) *[]string { return &p.contractDeployerAllowListEnabled }},
		{contractDeployerBlockListAdminFlag,
			func(p *genesisParams) *[]string { return &p.contractDeployerBlockListAdmin }},
		{contractDeployerBlockListEnabledFlag,
			func(p *genesisParams) *[]string { return &p.contractDeployerBlockListEnabled }},
		{transactionsAllowListAdminFlag,
			func(p *genesisParams) *[]string { return &p.transactionsAllowListAdmin }},
		{transactionsAllowListEnabledFlag,
			func(p *genesisParams) *[]string { return &p.transactionsAllowListEnabled }},
		{transactionsBlockListAdminFlag,
			func(p *genesisParams) *[]string { return &p.transactionsBlockListAdmin }},
		{transactionsBlockListEnabledFlag,
			func(p *genesisParams) *[]string { return &p.transactionsBlockListEnabled }},
	}
}

func genesisAccessListSnapshot(p *genesisParams) map[string][]string {
	result := make(map[string][]string)
	for _, flag := range genesisAccessListTestFlags() {
		result[flag.name] = slices.Clone(*flag.field(p))
	}
	return result
}

func genesisAccessListMalformedInputs() []struct {
	name    string
	address string
} {
	return []struct {
		name    string
		address string
	}{
		{"empty", ""},
		{"invalid", "0xzz"},
		{"short", "0x1234"},
		{"odd_length", "0x" + strings.Repeat("1", 39)},
		{"oversized", "0x" + strings.Repeat("11", 21)},
		{"partial", "0x" + strings.Repeat("11", 19) + "zz"},
		{"valid_prefix_invalid_suffix", "0x" + strings.Repeat("11", 20) + "zz"},
		{"uppercase_prefix", "0X" + strings.Repeat("11", 20)},
		{"leading_whitespace", " 0x" + strings.Repeat("11", 20)},
		{"trailing_whitespace", "0x" + strings.Repeat("11", 20) + " "},
	}
}

func genesisAccessListRequireOutput(
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

func TestGenesisAccessList_DefaultsAndAcceptedFormats(t *testing.T) {
	p := &genesisParams{}
	require.NoError(t, p.validateAccessListAddresses())

	for _, flag := range genesisAccessListTestFlags() {
		*flag.field(p) = []string{}
	}
	before := genesisAccessListSnapshot(p)
	require.NoError(t, p.validateAccessListAddresses())
	require.Equal(t, before, genesisAccessListSnapshot(p))

	formats := []string{
		"0x" + strings.Repeat("ab", 20),
		strings.Repeat("ab", 20),
		"0x" + strings.Repeat("Ab", 20),
		strings.Repeat("AB", 20),
		types.ZeroAddress.String(),
	}

	for _, flag := range genesisAccessListTestFlags() {
		t.Run(flag.name, func(t *testing.T) {
			p := &genesisParams{}
			*flag.field(p) = append(slices.Clone(formats), formats[0])

			before := genesisAccessListSnapshot(p)
			require.NoError(t, p.validateAccessListAddresses())
			require.Equal(t, before, genesisAccessListSnapshot(p))
		})
	}
}

func TestGenesisAccessList_InvalidEntriesPreserveInput(t *testing.T) {
	valid := "0x" + strings.Repeat("11", 20)

	for _, flag := range genesisAccessListTestFlags() {
		for _, input := range genesisAccessListMalformedInputs() {
			t.Run(flag.name+"/"+input.name, func(t *testing.T) {
				p := &genesisParams{}
				for _, other := range genesisAccessListTestFlags() {
					*other.field(p) = []string{valid}
				}
				*flag.field(p) = []string{valid, input.address, valid}

				before := genesisAccessListSnapshot(p)
				err := p.validateAccessListAddresses()

				require.ErrorContains(t, err, "invalid --"+flag.name+" address")
				require.ErrorContains(t, err, fmt.Sprintf("%q", input.address))
				require.NotNil(t, errors.Unwrap(err))
				require.Equal(t, before, genesisAccessListSnapshot(p))
			})
		}
	}
}

func TestGenesisAccessList_CommandRejectsBeforeGeneration(t *testing.T) {
	// Flag registration uses package-global params. Keep this test sequential.
	previous := params
	defer func() { params = previous }()

	valid := "0x" + strings.Repeat("11", 20)

	for _, flag := range genesisAccessListTestFlags() {
		for _, input := range genesisAccessListMalformedInputs() {
			for _, existing := range []bool{false, true} {
				name := fmt.Sprintf("%s/%s/existing_%t",
					flag.name, input.name, existing)

				t.Run(name, func(t *testing.T) {
					output := filepath.Join(t.TempDir(), "genesis.json")
					original := []byte("existing genesis must remain unchanged")
					if existing {
						require.NoError(t, os.WriteFile(output, original, 0o600))
					}

					params = &genesisParams{}
					cmd := GetCommand()
					require.NoError(t, cmd.ParseFlags([]string{
						"--dir", output,
						"--" + flag.name, valid,
						"--" + flag.name, input.address,
					}))

					before := genesisAccessListSnapshot(params)
					err := cmd.PreRunE(cmd, nil)

					require.ErrorContains(t, err, "invalid --"+flag.name+" address")
					require.ErrorContains(t, err, fmt.Sprintf("%q", input.address))
					require.Equal(t, before, genesisAccessListSnapshot(params))
					require.Nil(t, params.genesisConfig)

					genesisAccessListRequireOutput(t, output, existing, original)
				})
			}
		}
	}
}

func TestGenesisAccessList_DirectBuildersRejectBeforeWork(t *testing.T) {
	malformed := "0x" + strings.Repeat("11", 20) + "zz"

	for _, flag := range genesisAccessListTestFlags() {
		for _, builder := range []string{
			"init_generic",
			"generate_generic",
			"generate_innbcbft",
		} {
			for _, existing := range []bool{false, true} {
				name := fmt.Sprintf("%s/%s/existing_%t",
					flag.name, builder, existing)

				t.Run(name, func(t *testing.T) {
					output := filepath.Join(t.TempDir(), "genesis.json")
					original := []byte("existing genesis must remain unchanged")
					if existing {
						require.NoError(t, os.WriteFile(output, original, 0o600))
					}

					previousConfig := &chain.Chain{Name: "unchanged"}
					p := &genesisParams{
						/* START ALPHA CHANGE 3540 -- reach direct-builder access-list validation with a valid chain ID -- */
						chainID: 1982,
						/* FINISH ALPHA CHANGE 3540 -- reach direct-builder access-list validation with a valid chain ID -- */
						genesisPath:   output,
						genesisConfig: previousConfig,
					}
					*flag.field(p) = []string{malformed}
					before := genesisAccessListSnapshot(p)

					// Other builder inputs are deliberately absent. The list
					// guard must reject before those inputs are accessed.
					var err error
					switch builder {
					case "init_generic":
						err = p.initGenesisConfig()
					case "generate_generic":
						err = p.generateGenesis()
					case "generate_innbcbft":
						err = p.generatePolyBftChainConfig(nil)
					}

					require.ErrorContains(t, err, "invalid --"+flag.name+" address")
					require.ErrorContains(t, err, fmt.Sprintf("%q", malformed))
					require.Same(t, previousConfig, p.genesisConfig)
					require.Equal(t, before, genesisAccessListSnapshot(p))

					genesisAccessListRequireOutput(t, output, existing, original)
				})
			}
		}
	}
}

func TestGenesisAccessList_GenerationPreservesListPolicy(t *testing.T) {
	previous := params
	defer func() { params = previous }()

	key, err := bls.UnmarshalPrivateKey([]byte("7"))
	require.NoError(t, err)

	operator := types.StringToAddress("0x10001")
	proof, err := signer.MakeINNBCBLSPoP(key, operator)
	require.NoError(t, err)

	proofBytes, err := proof.Marshal()
	require.NoError(t, err)

	/* START ALPHA CHANGE 3846 -- provide a complete peer identity in access-list generation fixtures -- */
	validatorEntry := strings.Join([]string{
		"/ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAm22et517T1XhG181xGUDikDxUFrek4CuYTgUJjC38vxZR",
		operator.String(),
		"0x" + hex.EncodeToString(key.PublicKey().Marshal()),
		"0x" + hex.EncodeToString(proofBytes),
	}, ":")
	/* FINISH ALPHA CHANGE 3846 -- provide a complete peer identity in access-list generation fixtures -- */

	lower := strings.Repeat("ab", 20)
	admins := []string{
		"0x" + lower,
		strings.ToUpper(lower),
		types.ZeroAddress.String(),
	}
	enabled := []string{
		"0x" + strings.Repeat("Cd", 20),
		types.ZeroAddress.String(),
	}

	wantAdmins := []types.Address{
		types.StringToAddress("0x" + lower),
		types.StringToAddress("0x" + lower),
		types.ZeroAddress,
	}
	wantEnabled := []types.Address{
		types.StringToAddress("0x" + strings.Repeat("cd", 20)),
		types.ZeroAddress,
	}

	for _, tc := range []struct {
		name       string
		hasAdmins  bool
		hasEnabled bool
	}{
		{"defaults", false, false},
		{"enabled_without_admin", false, true},
		{"active_lists", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "genesis.json")

			params = &genesisParams{}
			cmd := GetCommand()
			require.NoError(t, cmd.ParseFlags([]string{
				"--dir", output,
				"--validators", validatorEntry,
			}))
			p := params

			for _, flag := range genesisAccessListTestFlags() {
				if strings.HasSuffix(flag.name, "-admin") {
					if tc.hasAdmins {
						*flag.field(p) = slices.Clone(admins)
					}
				} else if tc.hasEnabled {
					*flag.field(p) = slices.Clone(enabled)
				}
			}

			before := genesisAccessListSnapshot(p)
			require.NoError(t, cmd.PreRunE(cmd, nil))
			require.NoError(t, p.generatePolyBftChainConfig(
				&genesisChainIDOutput{Writer: io.Discard},
			))
			require.Equal(t, before, genesisAccessListSnapshot(p))

			imported, err := chain.ImportFromFile(output)
			require.NoError(t, err)

			lists := []*chain.AddressListConfig{
				imported.Params.ContractDeployerAllowList,
				imported.Params.ContractDeployerBlockList,
				imported.Params.TransactionsAllowList,
				imported.Params.TransactionsBlockList,
			}

			for _, list := range lists {
				if !tc.hasAdmins {
					require.Nil(t, list)
					continue
				}

				require.NotNil(t, list)
				require.Equal(t, wantAdmins, list.AdminAddresses)
				require.Equal(t, wantEnabled, list.EnabledAddresses)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3180 -- verify genesis access-list rejection before generation and preserve valid configuration -- */
