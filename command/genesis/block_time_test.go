/* START ALPHA CHANGE 2507 -- test genesis block-time validation and its integration -- */
package genesis

import (
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/server"
	"github.com/stretchr/testify/require"
)

func TestGenesisBlockTime_Boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		duration time.Duration
		valid    bool
	}{
		{"negative", -time.Second, false},
		{"zero", 0, false},
		{"half second", 500 * time.Millisecond, false},
		{"just below minimum", time.Second - time.Nanosecond, false},
		/* START ALPHA CHANGE 2517 -- cover whole-second and fractional genesis intervals -- */
		{"minimum", time.Second, true},
		{"fractional above minimum", 1500 * time.Millisecond, false},
		{"one nanosecond above minimum", time.Second + time.Nanosecond, false},
		{"just below two seconds", 2*time.Second - time.Nanosecond, false},
		{"fractional above two seconds", 2500 * time.Millisecond, false},
		{"two seconds", 2 * time.Second, true},
		{"default", defaultBlockTime, true},
		{"larger", 5 * time.Second, true},
		/* FINISH ALPHA CHANGE 2517 -- cover whole-second and fractional genesis intervals -- */
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &genesisParams{blockTime: tc.duration}
			err := p.validateBlockTime()

			if tc.valid {
				require.NoError(t, err)
				return
			}

			/* START ALPHA CHANGE 2517 -- distinguish minimum and fractional duration errors -- */
			if tc.duration < time.Second {
				require.EqualError(t, err,
					"--block-time must be at least 1s, got "+tc.duration.String())
			} else {
				require.EqualError(t, err,
					"--block-time must be a whole number of seconds, got "+tc.duration.String())
			}
			/* FINISH ALPHA CHANGE 2517 -- distinguish minimum and fractional duration errors -- */
		})
	}
}

func TestGenesisBlockTime_ValidateFlagsRejectsInvalidDuration(t *testing.T) {
	t.Parallel()

	/* START ALPHA CHANGE 2517 -- exercise fractional rejection through genesis flag validation -- */
	for _, duration := range []time.Duration{
		-time.Second,
		0,
		500 * time.Millisecond,
		time.Second - time.Nanosecond,
		time.Second + time.Nanosecond,
		1500 * time.Millisecond,
		2500 * time.Millisecond,
	} {
		/* FINISH ALPHA CHANGE 2517 -- exercise fractional rejection through genesis flag validation -- */
		t.Run(duration.String(), func(t *testing.T) {
			p := &genesisParams{
				/* START ALPHA CHANGE 3538 -- reach block-time validation with a valid chain ID -- */
				chainID: 1982,
				/* FINISH ALPHA CHANGE 3538 -- reach block-time validation with a valid chain ID -- */
				consensusRaw: string(server.INNBCBFTConsensus),
				blockTime:    duration,
			}

			/* START ALPHA CHANGE 2517 -- verify fractional errors reach the genesis command boundary -- */
			err := p.validateFlags()
			if duration < time.Second {
				require.EqualError(t, err,
					"--block-time must be at least 1s, got "+duration.String())
			} else {
				require.EqualError(t, err,
					"--block-time must be a whole number of seconds, got "+duration.String())
			}
			/* FINISH ALPHA CHANGE 2517 -- verify fractional errors reach the genesis command boundary -- */
		})
	}
}

/* FINISH ALPHA CHANGE 2507 -- test genesis block-time validation and its integration -- */
