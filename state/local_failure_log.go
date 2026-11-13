/* START ALPHA CHANGE 2169 -- bound local state failure logging by fixed component -- */
package state

import (
	"errors"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
)

// LocalStateLogComponent identifies a fixed logging budget.
// Budgets are shared process-wide, including across component instances.
type LocalStateLogComponent uint8

const (
	LocalStateLogRPC LocalStateLogComponent = iota
	LocalStateLogSync
	LocalStateLogConsensus
	LocalStateLogTxPool
	localStateLogOther
	localStateLogComponentCount
)

const localStateLogInterval = 30 * time.Second

var localStateLogNames = [...]string{
	"rpc",
	"sync",
	"consensus",
	"txpool",
	"other",
}

type localStateLogWindow struct {
	next       time.Time
	suppressed uint64
}

type localStateFailureLogger struct {
	mu      sync.Mutex
	windows [localStateLogComponentCount]localStateLogWindow
}

var processLocalStateFailureLogger localStateFailureLogger

// LogLocalStateFailure returns true whenever err is a local state failure,
// including when its log message is suppressed.
//
// Ordinary errors return false and retain their existing caller behavior.
// Suppressed counts are reported on the next emitted failure message; no
// background timer or goroutine is created.
func LogLocalStateFailure(
	logger hclog.Logger,
	component LocalStateLogComponent,
	err error,
) bool {
	return processLocalStateFailureLogger.log(
		logger,
		component,
		err,
		time.Now(),
	)
}

func (l *localStateFailureLogger) log(
	logger hclog.Logger,
	component LocalStateLogComponent,
	err error,
	now time.Time,
) bool {
	if !errors.Is(err, ErrLocalStateFailure) {
		return false
	}

	if logger == nil || !logger.IsError() {
		return true
	}

	if component >= localStateLogComponentCount {
		component = localStateLogOther
	}

	l.mu.Lock()
	window := &l.windows[component]

	if !window.next.IsZero() && now.Before(window.next) {
		// Saturate rather than wrap if the counter ever reaches its limit.
		if window.suppressed != ^uint64(0) {
			window.suppressed++
		}
		l.mu.Unlock()

		return true
	}

	suppressed := window.suppressed
	window.suppressed = 0
	window.next = now.Add(localStateLogInterval)
	l.mu.Unlock()

	// Never hold the suppression mutex while writing to the log sink.
	logger.Error(
		"local state unavailable; operation aborted",
		"component", localStateLogNames[component],
		"err", err,
		"suppressed_since_previous_log", suppressed,
		"repeat_interval", localStateLogInterval,
	)

	return true
}

/* FINISH ALPHA CHANGE 2169 -- bound local state failure logging by fixed component -- */
