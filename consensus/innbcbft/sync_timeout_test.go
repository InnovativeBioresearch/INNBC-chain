/* START ALPHA CHANGE 3892 -- test synchronization timeout saturation at arithmetic boundaries -- */
package innbcbft

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSyncBlockTimeout_SaturatingArithmetic(t *testing.T) {
	t.Parallel()

	const maximum = time.Duration(1<<63 - 1)
	lastExactSeconds := uint64(maximum / (3 * time.Second))
	largestIntervalSeconds := uint64(maximum / time.Second)

	tests := []struct {
		name    string
		seconds uint64
	}{
		{"zero_preserves_existing_behavior", 0},
		{"one_second", 1},
		{"default_two_seconds", 2},
		{"three_seconds", 3},
		{"one_hour", 3600},
		{"below_last_exact_input", lastExactSeconds - 1},
		{"last_exact_input", lastExactSeconds},
		{"first_overflowing_input", lastExactSeconds + 1},
		{"reported_two_to_32_seconds", uint64(1) << 32},
		{"largest_whole_second_interval", largestIntervalSeconds},
		{"seconds_exceed_signed_range", uint64(1) << 63},
		{"maximum_uint64_seconds", ^uint64(0)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Independent arbitrary-precision oracle: calculate the full
			// product before limiting it to time.Duration's maximum.
			expected := new(big.Int).SetUint64(test.seconds)
			expected.Mul(expected, big.NewInt(3))
			expected.Mul(expected, big.NewInt(int64(time.Second)))

			limit := big.NewInt(int64(maximum))
			if expected.Cmp(limit) > 0 {
				expected.Set(limit)
			}

			actual := syncBlockTimeout(test.seconds)
			require.Equal(t, time.Duration(expected.Int64()), actual)

			if test.seconds > 0 {
				require.Positive(t, int64(actual))
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3892 -- test synchronization timeout saturation at arithmetic boundaries -- */
