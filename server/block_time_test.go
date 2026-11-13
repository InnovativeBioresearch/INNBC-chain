/* START ALPHA CHANGE 2518 -- test whole-second startup block-time validation -- */
package server

import (
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/stretchr/testify/require"
)

func TestExtractBlockTime_WholeSeconds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		want  time.Duration
		valid bool
	}{
		{"negative", "-1s", 0, false},
		{"zero", "0s", 0, false},
		{"below minimum", "500ms", 0, false},
		{"fractional", "1.5s", 0, false},
		{"nanosecond fraction", "1.000000001s", 0, false},
		{"fractional above two seconds", "2.5s", 0, false},
		{"minimum", "1s", time.Second, true},
		{"default", "2s", 2 * time.Second, true},
		{"larger", "5s", 5 * time.Second, true},
		{"equivalent milliseconds", "2000ms", 2 * time.Second, true},
		{
			"duration object fractional",
			common.Duration{Duration: 1500 * time.Millisecond},
			0,
			false,
		},
		{
			"duration object whole seconds",
			common.Duration{Duration: 2 * time.Second},
			2 * time.Second,
			true,
		},
		{"numeric nanoseconds fractional", int64(1500 * time.Millisecond), 0, false},
		{"numeric nanoseconds whole seconds", int64(2 * time.Second), 2 * time.Second, true},
		{"malformed duration", "invalid", 0, false},
		{"null", nil, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractBlockTime(map[string]interface{}{
				"blockTime": tc.value,
			})

			if !tc.valid {
				require.ErrorIs(t, err, errBlockTimeInvalid)
				require.Zero(t, got.Duration)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, got.Duration)
		})
	}

	t.Run("missing", func(t *testing.T) {
		_, err := extractBlockTime(map[string]interface{}{})
		require.ErrorIs(t, err, errBlockTimeMissing)
	})
}

/* FINISH ALPHA CHANGE 2518 -- test whole-second startup block-time validation -- */
