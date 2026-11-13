/* START ALPHA CHANGE 3143 -- test recent-history count rejection through CLI configuration -- */
package server

import (
	"math"
	"strconv"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/stretchr/testify/require"
)

func TestRecentHistorySegments_CLIConfiguration(t *testing.T) {
	original := params.rawConfig
	t.Cleanup(func() { params.rawConfig = original })

	for _, tc := range []struct {
		name      string
		mode      string
		retention uint64
		segment   uint64
		wantErr   string
	}{
		{"default_recent", "recent", 43200, 1800, ""},
		{"maximum_count", "recent", uint64(math.MaxInt), 1, ""},
		{"first_unsupported_count", "recent", uint64(math.MaxInt) + 1, 1, "exceeds platform maximum"},
		{"reported_64_bit_input", "recent", uint64(1) << 63, 1, "exceeds platform maximum"},
		{"maximum_unsigned_count", "recent", math.MaxUint64, 1, "exceeds platform maximum"},
		{"large_retention_one_segment", "recent", math.MaxUint64, math.MaxUint64, ""},
		{"full_mode_ignores_segment_geometry", "full", math.MaxUint64, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params.rawConfig = config.DefaultConfig()
			command := GetCommand()

			require.NoError(t, command.Flags().Parse([]string{
				"--history-mode", tc.mode,
				"--history-retention-blocks", strconv.FormatUint(tc.retention, 10),
				"--history-segment-blocks", strconv.FormatUint(tc.segment, 10),
			}))

			err := params.initHistoryConfig()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Equal(t, tc.retention, params.rawConfig.History.RetentionBlocks)
				require.Equal(t, tc.segment, params.rawConfig.History.SegmentBlocks)

				return
			}

			require.NoError(t, err)
			if tc.mode == "full" {
				require.Zero(t, params.rawConfig.History.RetentionBlocks)
				require.Zero(t, params.rawConfig.History.SegmentBlocks)
			} else {
				require.Equal(t, tc.retention, params.rawConfig.History.RetentionBlocks)
				require.Equal(t, tc.segment, params.rawConfig.History.SegmentBlocks)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3143 -- test recent-history count rejection through CLI configuration -- */
