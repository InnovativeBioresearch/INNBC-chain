/* START ALPHA CHANGE 3134 -- reject malformed premine flags before generation and preserve repeated CLI entries -- */
package genesis

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisAddress_CommandRejectsBeforeGeneration(t *testing.T) {
	previous := params
	defer func() { params = previous }()

	const privateKey = "PRIVATE_KEY_SENTINEL"
	addresses := []string{
		"",
		"0xzz",
		"0x12345",
		"0x" + strings.Repeat("a", 39),
		"0x" + strings.Repeat("a", 42),
		"0x" + strings.Repeat("ab", 19) + "zz",
	}

	for index, address := range addresses {
		t.Run(fmt.Sprintf("%d", index), func(t *testing.T) {
			for _, existing := range []bool{false, true} {
				root := t.TempDir()
				output := filepath.Join(root, "genesis.json")
				original := []byte("existing genesis must remain unchanged")
				if existing {
					require.NoError(t, os.WriteFile(output, original, 0o600))
				}

				params = &genesisParams{}
				cmd := GetCommand()
				require.NoError(t, cmd.ParseFlags([]string{
					"--dir", output,
					"--premine", address + ":17:" + privateKey,
				}))

				// Exercise the actual genesis command's pre-run validation.
				err := cmd.PreRunE(cmd, nil)
				require.ErrorContains(t, err, "invalid premine entry")
				require.ErrorContains(t, err, "invalid premine address")
				require.NotContains(t, err.Error(), privateKey)
				require.Empty(t, params.premineInfos)

				if existing {
					content, readErr := os.ReadFile(output)
					require.NoError(t, readErr)
					require.Equal(t, original, content)
				} else {
					_, statErr := os.Stat(output)
					require.True(t, os.IsNotExist(statErr))
				}
			}
		})
	}
}

func TestGenesisAddress_RepeatedPremineAliasesPreserveOrder(t *testing.T) {
	lower := strings.Repeat("ab", 20)
	raw := []string{
		"0x" + lower + ":17:first-key",
		strings.ToUpper(lower) + ":23:second-key",
	}
	before := append([]string(nil), raw...)
	p := &genesisParams{premine: raw}

	require.NoError(t, p.parsePremineInfo())
	require.Equal(t, before, p.premine)
	require.Len(t, p.premineInfos, 2)

	expected := types.StringToAddress("0x" + lower)
	require.Equal(t, expected, p.premineInfos[0].Address)
	require.Equal(t, expected, p.premineInfos[1].Address)
	require.Zero(t, big.NewInt(17).Cmp(p.premineInfos[0].Amount))
	require.Zero(t, big.NewInt(23).Cmp(p.premineInfos[1].Amount))
	require.Equal(t, "first-key", p.premineInfos[0].Key)
	require.Equal(t, "second-key", p.premineInfos[1].Key)
}

/* FINISH ALPHA CHANGE 3134 -- reject malformed premine flags before generation and preserve repeated CLI entries -- */
