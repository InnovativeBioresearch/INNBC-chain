package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_ExtendByteSlice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		length    int
		newLength int
	}{
		{"With trimming", 4, 2},
		{"Without trimming", 4, 8},
		{"Without trimming (same lengths)", 4, 4},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			originalSlice := make([]byte, c.length)
			for i := 0; i < c.length; i++ {
				originalSlice[i] = byte(i * 2)
			}

			newSlice := ExtendByteSlice(originalSlice, c.newLength)
			require.Len(t, newSlice, c.newLength)
			if c.length > c.newLength {
				require.Equal(t, originalSlice[:c.newLength], newSlice)
			} else {
				require.Equal(t, originalSlice, newSlice[:c.length])
			}
		})
	}
}

func Test_BigIntDivCeil(t *testing.T) {
	t.Parallel()

	cases := []struct {
		a      int64
		b      int64
		result int64
	}{
		{a: 10, b: 3, result: 4},
		{a: 13, b: 6, result: 3},
		{a: -1, b: 3, result: 0},
		{a: -20, b: 3, result: -6},
		{a: -15, b: 3, result: -5},
	}

	for _, c := range cases {
		require.Equal(t, c.result, BigIntDivCeil(big.NewInt(c.a), big.NewInt(c.b)).Int64())
	}
}

/* START ALPHA CHANGE 3322 -- cover chain-clock arithmetic and configuration rejection -- */
func TestChainClock_ExpectedTimestamp(t *testing.T) {
	cases := []struct {
		name     string
		parent   uint64
		interval time.Duration
		want     uint64
		wantErr  string
	}{
		{"one_second", 100, time.Second, 101, ""},
		{"two_seconds", 100, 2 * time.Second, 102, ""},
		{"five_seconds", 100, 5 * time.Second, 105, ""},
		{"negative", 100, -time.Second, 0, "at least 1s"},
		{"zero", 100, 0, 0, "at least 1s"},
		{"subsecond", 100, time.Millisecond, 0, "at least 1s"},
		{"fractional", 100, 1500 * time.Millisecond, 0, "whole number"},
		{"uint64_overflow", math.MaxUint64, time.Second, 0, "overflows uint64"},
		{"unsupported_parent", uint64(math.MaxInt64) + 1, time.Second, 0, "execution range"},
		{"execution_overflow", math.MaxInt64, time.Second, 0, "execution range"},
		{"last_supported_timestamp", math.MaxInt64 - 1, time.Second, math.MaxInt64, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpectedBlockTimestamp(tc.parent, tc.interval)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Zero(t, got)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestChainClock_DecodeConfiguration(t *testing.T) {
	cases := []struct {
		raw     string
		want    time.Duration
		wantErr string
	}{
		{`{"blockTime":"1s"}`, time.Second, ""},
		{`{"blockTime":"2s"}`, 2 * time.Second, ""},
		{`{"blockTime":"5s"}`, 5 * time.Second, ""},
		{`{}`, 0, "blockTime"},
		{`{"blockTime":null}`, 0, "blockTime"},
		{`{"blockTime":"0s"}`, 0, "at least 1s"},
		{`{"blockTime":"1.5s"}`, 0, "whole number"},
		{`{"blockTime":"invalid"}`, 0, "blockTime"},
		{`{"blockTime":"2s","blockTimeDrift":0}`, 0, "retired"},
		{`{"blockTime":"2s","blockTimeDrift":null}`, 0, "retired"},
		{`{"blockTime":"2s","blockTimeDrift":10}`, 0, "retired"},
		{`{"blockTime":"2s","BlockTimeDrift":10}`, 0, "retired"},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := DecodeBlockTimeConfig([]byte(tc.raw))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Zero(t, got)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

/* FINISH ALPHA CHANGE 3322 -- cover chain-clock arithmetic and configuration rejection -- */

func Test_Duration_Marshal_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("use duration standalone", func(t *testing.T) {
		t.Parallel()

		original := &Duration{Duration: 5 * time.Minute}
		originalRaw, err := original.MarshalJSON()
		require.NoError(t, err)

		other := &Duration{}
		require.NoError(t, other.UnmarshalJSON(originalRaw))
		require.Equal(t, original, other)
	})

	t.Run("use duration in wrapper struct", func(t *testing.T) {
		t.Parallel()

		type timer struct {
			Elapsed Duration `json:"elapsed"`
		}

		dur, err := time.ParseDuration("2h35m21s")
		require.NoError(t, err)

		origTimer := &timer{Elapsed: Duration{dur}}

		timerRaw, err := json.Marshal(origTimer)
		require.NoError(t, err)

		var otherTimer *timer
		require.NoError(t, json.Unmarshal(timerRaw, &otherTimer))
		require.Equal(t, origTimer, otherTimer)
	})
}

func TestRetryForever_AlwaysReturnError_ShouldNeverEnd(t *testing.T) {
	interval := time.Millisecond * 10
	ended := false

	go func() {
		RetryForever(context.Background(), interval, func(ctx context.Context) error {
			return errors.New("")
		})

		ended = true
	}()
	time.Sleep(interval * 10)
	require.False(t, ended)
}

func TestRetryForever_CancelContext_ShouldEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cancel()
	RetryForever(ctx, time.Millisecond*100, func(ctx context.Context) error {
		return errors.New("")
	})
	require.True(t, errors.Is(ctx.Err(), context.Canceled))
}

func Test_SafeAddUint64(t *testing.T) {
	cases := []struct {
		a        uint64
		b        uint64
		result   uint64
		overflow bool
	}{
		{
			a:      10,
			b:      4,
			result: 14,
		},
		{
			a:      0,
			b:      5,
			result: 5,
		},
		{
			a:        math.MaxUint64,
			b:        3,
			result:   0,
			overflow: true,
		},
		{
			a:        math.MaxUint64,
			b:        math.MaxUint64,
			result:   0,
			overflow: true,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d case", i+1), func(t *testing.T) {
			actualResult, actualOverflow := SafeAddUint64(c.a, c.b)
			require.Equal(t, c.result, actualResult)
			require.Equal(t, c.overflow, actualOverflow)
		})
	}
}
