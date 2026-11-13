// START ALPHA CHANGE 3800 -- synchronize throttling admission release and rejection checks --
package jsonrpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestThrottling(t *testing.T) {
	t.Parallel()

	const (
		capacity         = 5
		admissionTimeout = 5 * time.Second
		parentTimeout    = 250 * time.Millisecond
		watchdog         = 15 * time.Second
	)

	th := NewThrottling(capacity, admissionTimeout)
	ctx, cancel := context.WithCancel(context.Background())

	var workers sync.WaitGroup
	t.Cleanup(func() {
		cancel()

		done := make(chan struct{})
		go func() {
			workers.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(watchdog):
			t.Error("throttling workers did not stop after cancellation")
		}
	})

	type requestResult struct {
		value any
		err   error
	}

	type heldRequest struct {
		entered chan struct{}
		release chan struct{}
		result  chan requestResult
	}

	startRequest := func(value int) *heldRequest {
		request := &heldRequest{
			entered: make(chan struct{}),
			release: make(chan struct{}),
			result:  make(chan requestResult, 1),
		}

		workers.Add(1)
		go func() {
			defer workers.Done()

			result, err := th.AttemptRequest(ctx, func() (any, error) {
				// Entry proves that the real admission method acquired a slot.
				close(request.entered)

				select {
				case <-request.release:
					return value, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			})

			// Buffered reporting also allows cleanup after a failed assertion.
			request.result <- requestResult{value: result, err: err}
		}()

		return request
	}

	waitEntered := func(request *heldRequest) {
		t.Helper()

		select {
		case <-request.entered:
		case result := <-request.result:
			require.NoError(t, result.err,
				"request failed before its handler entered")
			t.Fatal("request returned without entering its held handler")
		case <-time.After(watchdog):
			t.Fatal("timed out waiting for an admitted handler")
		}
	}

	waitSuccess := func(request *heldRequest, expected int) {
		t.Helper()

		select {
		case result := <-request.result:
			require.NoError(t, result.err)
			require.Equal(t, expected, result.value)
		case <-time.After(watchdog):
			t.Fatal("released request did not complete")
		}
	}

	assertFull := func(parent context.Context) {
		t.Helper()

		invoked := false
		result, err := th.AttemptRequest(parent, func() (any, error) {
			invoked = true
			return -1, nil
		})

		require.ErrorIs(t, err, errRequestLimitExceeded)
		require.Nil(t, result)
		require.False(t, invoked,
			"an additional handler ran while all five slots were occupied")
	}

	// Admit five requests and explicitly confirm that every slot is occupied.
	held := make([]*heldRequest, capacity)
	for index := range held {
		held[index] = startRequest(100)
	}
	for _, request := range held {
		waitEntered(request)
	}

	// Keep all handlers blocked while exercising the configured timeout.
	// The parent remains live throughout this rejection.
	assertFull(ctx)
	require.NoError(t, ctx.Err())

	// Wait for completion before reusing the released slot. AttemptRequest's
	// deferred semaphore release happens before this result is reported.
	close(held[0].release)
	waitSuccess(held[0], 100)

	replacement := startRequest(10)
	waitEntered(replacement)

	// Capacity is full again. Verify that a shorter parent deadline also
	// rejects the additional request without invoking its handler.
	shortCtx, shortCancel := context.WithTimeout(ctx, parentTimeout)
	defer shortCancel()

	assertFull(shortCtx)
	require.ErrorIs(t, shortCtx.Err(), context.DeadlineExceeded)

	// Release and collect the remaining five successful requests.
	for _, request := range held[1:] {
		close(request.release)
	}
	close(replacement.release)

	for _, request := range held[1:] {
		waitSuccess(request, 100)
	}
	waitSuccess(replacement, 10)

	// Completed requests must leave capacity available for subsequent work.
	result, err := th.AttemptRequest(ctx, func() (any, error) {
		return 1, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, result)
}

// FINISH ALPHA CHANGE 3800 -- synchronize throttling admission release and rejection checks --
