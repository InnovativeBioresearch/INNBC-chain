/* START ALPHA CHANGE 2174 -- test bounded local state failure logging without sleeps -- */
package state

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

type issue7ALogEntry struct {
	message string
	args    []interface{}
}

type issue7ALogCapture struct {
	hclog.Logger
	mu      sync.Mutex
	entries []issue7ALogEntry
}

func newIssue7ALogCapture() *issue7ALogCapture {
	return &issue7ALogCapture{Logger: hclog.NewNullLogger()}
}

func (*issue7ALogCapture) IsError() bool {
	return true
}

func (l *issue7ALogCapture) Error(message string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = append(l.entries, issue7ALogEntry{
		message: message,
		args:    append([]interface{}(nil), args...),
	})
}

func issue7ALogField(entry issue7ALogEntry, name string) interface{} {
	for i := 0; i+1 < len(entry.args); i += 2 {
		if entry.args[i] == name {
			return entry.args[i+1]
		}
	}

	return nil
}

func TestIssue7A_LocalFailureLoggingWindow(t *testing.T) {
	var limiter localStateFailureLogger
	logger := newIssue7ALogCapture()
	now := time.Unix(1000, 0)
	cause := errors.New("injected database read failure")
	local := WrapLocalStateFailure("read account", cause)

	require.False(t, limiter.log(logger, LocalStateLogRPC, nil, now))
	require.False(t, limiter.log(logger, LocalStateLogRPC, cause, now))
	require.Empty(t, logger.entries)

	require.True(t, limiter.log(logger, LocalStateLogRPC, local, now))
	require.Len(t, logger.entries, 1)
	require.ErrorIs(
		t,
		issue7ALogField(logger.entries[0], "err").(error),
		cause,
	)

	for i := 0; i < 100; i++ {
		require.True(t, limiter.log(
			logger,
			LocalStateLogRPC,
			local,
			now.Add(time.Second),
		))
	}
	require.Len(t, logger.entries, 1)

	// A different fixed component has its own budget.
	require.True(t, limiter.log(logger, LocalStateLogSync, local, now))
	require.Len(t, logger.entries, 2)

	require.True(t, limiter.log(
		logger,
		LocalStateLogRPC,
		local,
		now.Add(localStateLogInterval),
	))
	require.Len(t, logger.entries, 3)
	require.Equal(
		t,
		uint64(100),
		issue7ALogField(logger.entries[2], "suppressed_since_previous_log"),
	)

	require.True(t, limiter.log(
		logger,
		LocalStateLogRPC,
		local,
		now.Add(2*localStateLogInterval),
	))
	require.Equal(
		t,
		uint64(0),
		issue7ALogField(logger.entries[3], "suppressed_since_previous_log"),
	)
}

func TestIssue7A_LocalFailureLoggingConcurrentSuppression(t *testing.T) {
	var limiter localStateFailureLogger
	logger := newIssue7ALogCapture()
	now := time.Unix(2000, 0)
	local := WrapLocalStateFailure("read storage", errors.New("unavailable"))

	const attempts = 100
	var wg sync.WaitGroup
	wg.Add(attempts)

	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			limiter.log(logger, LocalStateLogTxPool, local, now)
		}()
	}
	wg.Wait()

	require.Len(t, logger.entries, 1)

	limiter.log(
		logger,
		LocalStateLogTxPool,
		local,
		now.Add(localStateLogInterval),
	)
	require.Len(t, logger.entries, 2)
	require.Equal(
		t,
		uint64(attempts-1),
		issue7ALogField(logger.entries[1], "suppressed_since_previous_log"),
	)
}

/* FINISH ALPHA CHANGE 2174 -- test bounded local state failure logging without sleeps -- */
