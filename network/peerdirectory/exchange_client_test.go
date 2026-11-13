/* START ALPHA CHANGE 2232 -- test temporary overload retries and cancellation -- */
package peerdirectory

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDirectoryExchangeRetriesOnlyTemporaryStatuses(t *testing.T) {
	for _, status := range []int{
		http.StatusTooManyRequests,
		http.StatusServiceUnavailable,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusInternalServerError,
	} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int64

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Retry-After", "2")
					w.WriteHeader(status)
					return
				}
				_, _ = w.Write([]byte(`{"peers":[]}`))
			}))
			defer server.Close()

			builds := 0
			waits := 0

			body, err := exchangeWithRetry(
				context.Background(),
				server.Client(),
				server.URL,
				func() ([]byte, error) {
					builds++
					// ALPHA CHANGE 2236: build request bytes directly.
					return fmt.Appendf(nil, `{"attempt":%d}`, builds), nil
				},
				func(_ context.Context, delay time.Duration) error {
					waits++
					require.GreaterOrEqual(t, delay, 2*time.Second)
					require.LessOrEqual(t, delay, 2250*time.Millisecond)
					return nil
				},
			)

			if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
				require.NoError(t, err)
				require.JSONEq(t, `{"peers":[]}`, string(body))
				require.Equal(t, 2, builds)
				require.Equal(t, 1, waits)
				require.Equal(t, int64(2), calls.Load())
			} else {
				require.Error(t, err)
				require.Nil(t, body)
				require.Equal(t, 1, builds)
				require.Zero(t, waits)
				require.Equal(t, int64(1), calls.Load())
			}
		})
	}
}

func TestDirectoryExchangeRetryBound(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	waits := 0
	_, err := exchangeWithRetry(
		context.Background(),
		server.Client(),
		server.URL,
		func() ([]byte, error) { return []byte("{}"), nil },
		func(context.Context, time.Duration) error {
			waits++
			return nil
		},
	)

	require.Error(t, err)
	require.Equal(t, int64(exchangeMaxAttempts), calls.Load())
	require.Equal(t, exchangeMaxAttempts-1, waits)
}

func TestDirectoryExchangeCancellationDuringRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := exchangeWithRetry(
		ctx,
		server.Client(),
		server.URL,
		func() ([]byte, error) { return []byte("{}"), nil },
		func(ctx context.Context, delay time.Duration) error {
			cancel()
			return waitForExchangeRetry(ctx, delay)
		},
	)

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int64(1), calls.Load())
}

func TestDirectoryExchangeRetryAfterBounds(t *testing.T) {
	now := time.Now()

	require.Equal(t, 2*time.Second, exchangeRetryDelay("2", 0, now))
	require.Equal(t, time.Second, exchangeRetryDelay("-1", 0, now))
	require.Equal(t, 4*time.Second, exchangeRetryDelay("invalid", 2, now))
	require.Equal(t, exchangeMaxRetryDelay, exchangeRetryDelay("9223372036854775807", 0, now))
	require.Equal(t, exchangeMaxRetryDelay, exchangeRetryDelay(
		now.Add(time.Hour).UTC().Format(http.TimeFormat),
		0,
		now,
	))
}

/* FINISH ALPHA CHANGE 2232 -- test temporary overload retries and cancellation -- */
