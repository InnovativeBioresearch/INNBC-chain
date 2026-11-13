/* START ALPHA CHANGE 3308 -- validate direct initialization before configuration access -- */
package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/stretchr/testify/require"
)

func TestServerConfigSections_RejectBeforeInitialization(t *testing.T) {
	for _, missing := range []string{"config", "network", "telemetry", "tx_pool"} {
		t.Run(missing, func(t *testing.T) {
			raw := config.DefaultConfig()

			// Initialization would normally resolve this zero into a default.
			// Structural rejection must occur before that mutation.
			raw.JSONRPCFeeHistoryBlockLimit = 0

			wantError := fmt.Sprintf(
				"invalid server configuration: section %q must not be null",
				missing,
			)

			switch missing {
			case "config":
				raw = nil
				wantError = "server configuration must not be nil"
			case "network":
				raw.Network = nil
			case "telemetry":
				raw.Telemetry = nil
			case "tx_pool":
				raw.TxPool = nil
			}

			p := &serverParams{rawConfig: raw}
			var err error

			require.NotPanics(t, func() {
				err = p.initRawParams()
			})

			require.EqualError(t, err, wantError)
			require.Nil(t, p.genesisConfig)

			if raw != nil {
				require.Zero(t, raw.JSONRPCFeeHistoryBlockLimit)
			}
		})
	}
}

func TestServerConfigSections_OptionalPoliciesDuringInitialization(t *testing.T) {
	dir := t.TempDir()
	genesisPath := filepath.Join(dir, "genesis.json")
	dataDir := filepath.Join(dir, "runtime-data")

	// This fixture exercises parameter initialization, without starting a node.
	genesis := &chain.Chain{
		Name: "configuration-sections-test",
		Genesis: &chain.Genesis{
			GasLimit:           5_242_880,
			BaseFee:            1,
			BaseFeeEM:          chain.GenesisBaseFeeEM,
			BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
		},
		Params: &chain.Params{
			/* START ALPHA CHANGE 3543 -- preserve optional-section initialization with a valid chain ID -- */
			ChainID: 1982,
			/* FINISH ALPHA CHANGE 3543 -- preserve optional-section initialization with a valid chain ID -- */
			Forks: chain.AllForksEnabled.Copy(),
			/* START ALPHA CHANGE 3325 -- preserve optional-section initialization coverage with valid clock configuration -- */
			Engine: map[string]any{
				"innbcbft": map[string]any{"blockTime": "1s"},
			},
			/* FINISH ALPHA CHANGE 3325 -- preserve optional-section initialization coverage with valid clock configuration -- */
		},
	}

	encoded, err := json.Marshal(genesis)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(genesisPath, encoded, 0600))

	raw := config.DefaultConfig()
	raw.GenesisPath = genesisPath
	raw.DataDir = dataDir
	raw.GRPCAddr = "127.0.0.1:0"
	raw.JSONRPCAddr = "127.0.0.1:0"
	raw.Network.Libp2pAddr = "127.0.0.1:0"
	raw.History = nil
	raw.JSONRPCAdmission = nil

	p := &serverParams{rawConfig: raw}

	require.NoError(t, p.initRawParams())
	require.NotNil(t, p.genesisConfig)

	wantHistory := config.DefaultConfig().History
	wantHistory.RetentionBlocks = 0
	wantHistory.SegmentBlocks = 0

	require.Equal(t, wantHistory, raw.History)
	require.Nil(t, raw.JSONRPCAdmission)
	require.NoError(t, raw.JSONRPCAdmission.Validate())

	runtime := p.generateConfig()
	require.Equal(t, config.DefaultHistoryMode, runtime.History.Mode)
	require.Nil(t, runtime.JSONRPC.Admission)
	require.NoDirExists(t, dataDir)
}

/* FINISH ALPHA CHANGE 3308 -- validate direct initialization before configuration access -- */
