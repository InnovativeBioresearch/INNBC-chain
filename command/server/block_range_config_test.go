/* START ALPHA CHANGE 2487 -- test log-range flag defaults and runtime normalization -- */
package server

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/stretchr/testify/require"
)

func TestBlockRangeLimit_FlagsAndRuntimeConfiguration(t *testing.T) {
	original := params.rawConfig
	t.Cleanup(func() {
		params.rawConfig = original
	})

	cases := []struct {
		name string
		args []string
		want uint64
	}{
		{"omitted", nil, 256},
		{"zero", []string{"--json-rpc-block-range-limit", "0"}, 256},
		{"larger override", []string{"--json-rpc-block-range-limit", "512"}, 512},
		{"smaller override", []string{"--json-rpc-block-range-limit", "32"}, 32},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params.rawConfig = config.DefaultConfig()
			command := GetCommand()

			require.NoError(t, command.Flags().Parse(tc.args))

			runtime := params.generateConfig()
			require.Equal(t, tc.want, runtime.JSONRPC.BlockRangeLimit)
		})
	}

	t.Run("zero in raw configuration", func(t *testing.T) {
		params.rawConfig = config.DefaultConfig()
		params.rawConfig.JSONRPCBlockRangeLimit = 0

		runtime := params.generateConfig()
		require.Equal(t, uint64(256), runtime.JSONRPC.BlockRangeLimit)
	})
}

/* FINISH ALPHA CHANGE 2487 -- test log-range flag defaults and runtime normalization -- */
