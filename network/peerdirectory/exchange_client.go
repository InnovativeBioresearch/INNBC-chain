/* START ALPHA CHANGE 2226 -- retry temporary directory overload with bounded backoff -- */
package peerdirectory

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

const (
	exchangeAttemptTimeout = 10 * time.Second
	exchangeTotalTimeout   = 60 * time.Second
	exchangeMaxAttempts    = 5
	exchangeMaxRetryDelay  = 10 * time.Second
	exchangeMaxResponse    = 1024 * 1024
)

// Exchange retries only HTTP 429 and 503.
// buildBody is called for every attempt so timestamps and nonces are fresh.
func Exchange(
	ctx context.Context,
	url string,
	buildBody func() ([]byte, error),
) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, exchangeTotalTimeout)
	defer cancel()

	client := &http.Client{Timeout: exchangeAttemptTimeout}

	return exchangeWithRetry(ctx, client, url, buildBody, waitForExchangeRetry)
}

func exchangeWithRetry(
	ctx context.Context,
	client *http.Client,
	url string,
	buildBody func() ([]byte, error),
	wait func(context.Context, time.Duration) error,
) ([]byte, error) {
	for attempt := 0; attempt < exchangeMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		body, err := buildBody()
		if err != nil {
			return nil, err
		}

		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			url,
			bytes.NewReader(body),
		)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json")

		response, err := client.Do(request)
		if err != nil {
			// Transport failures are not silently retried.
			return nil, err
		}

		success := response.StatusCode >= http.StatusOK &&
			response.StatusCode < http.StatusMultipleChoices
		limit := int64(1024)
		if success {
			limit = exchangeMaxResponse
		}

		payload, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
		_ = response.Body.Close()

		if readErr != nil {
			return nil, readErr
		}

		if success {
			if int64(len(payload)) > limit {
				return nil, fmt.Errorf("peer directory response exceeds %d bytes", limit)
			}

			return payload, nil
		}

		if int64(len(payload)) > limit {
			payload = payload[:limit]
		}

		statusErr := fmt.Errorf(
			"peer directory returned status %d: %s",
			response.StatusCode,
			string(payload),
		)
		temporary := response.StatusCode == http.StatusTooManyRequests ||
			response.StatusCode == http.StatusServiceUnavailable

		if !temporary || attempt+1 == exchangeMaxAttempts {
			return nil, statusErr
		}

		delay := exchangeRetryDelay(
			response.Header.Get("Retry-After"),
			attempt,
			time.Now(),
		)
		// Independent jitter reduces synchronized retry bursts.
		delay += time.Duration(rand.Int64N(int64(250*time.Millisecond) + 1))

		if err := wait(ctx, delay); err != nil {
			return nil, err
		}
	}

	panic("unreachable exchange retry state")
}

func exchangeRetryDelay(header string, attempt int, now time.Time) time.Duration {
	delay := time.Second << min(attempt, 3)

	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil && seconds >= 0 {
		// Clamp before conversion to time.Duration to avoid overflow.
		if seconds >= int64(exchangeMaxRetryDelay/time.Second) {
			delay = exchangeMaxRetryDelay
		} else if supplied := time.Duration(seconds) * time.Second; supplied > delay {
			delay = supplied
		}
	} else if date, err := http.ParseTime(header); err == nil {
		if supplied := date.Sub(now); supplied > delay {
			delay = supplied
		}
	}

	if delay > exchangeMaxRetryDelay {
		delay = exchangeMaxRetryDelay
	}

	return delay
}

func waitForExchangeRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

/* FINISH ALPHA CHANGE 2226 -- retry temporary directory overload with bounded backoff -- */
