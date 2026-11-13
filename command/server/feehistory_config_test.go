/* START ALPHA CHANGE 2475 -- test fee-history flags and runtime configuration wiring -- */
package server

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/stretchr/testify/require"
)

func TestFeeHistoryConfig_FlagsAndRuntimeWiring(t *testing.T) {
	original := params.rawConfig
	t.Cleanup(func() {
		params.rawConfig = original
	})

	params.rawConfig = config.DefaultConfig()
	command := GetCommand()

	require.NoError(t, command.Flags().Parse([]string{
		"--json-rpc-fee-history-block-limit", "512",
		"--json-rpc-fee-history-percentile-limit", "100",
		"--json-rpc-fee-history-result-limit", "51200",
	}))
	require.NoError(t, params.initFeeHistoryConfig())

	runtime := params.generateConfig()
	require.Equal(t, uint64(512), runtime.JSONRPC.FeeHistoryBlockLimit)
	require.Equal(t, uint64(100), runtime.JSONRPC.FeeHistoryPercentileLimit)
	require.Equal(t, uint64(51200), runtime.JSONRPC.FeeHistoryResultLimit)

	params.rawConfig.JSONRPCFeeHistoryBlockLimit = 0
	params.rawConfig.JSONRPCFeeHistoryPercentileLimit = 0
	params.rawConfig.JSONRPCFeeHistoryResultLimit = 0
	require.NoError(t, params.initFeeHistoryConfig())

	runtime = params.generateConfig()
	require.Equal(t, uint64(256), runtime.JSONRPC.FeeHistoryBlockLimit)
	require.Equal(t, uint64(10), runtime.JSONRPC.FeeHistoryPercentileLimit)
	require.Equal(t, uint64(2560), runtime.JSONRPC.FeeHistoryResultLimit)

	params.rawConfig.JSONRPCFeeHistoryPercentileLimit = 101
	require.Error(t, params.initFeeHistoryConfig())
}

/* FINISH ALPHA CHANGE 2475 -- test fee-history flags and runtime configuration wiring -- */
