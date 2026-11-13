/* START ALPHA CHANGE 2195 -- test FSM preparation local-failure logging policy -- */
package innbcbft

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/stretchr/testify/require"
)

func TestIssue7A_FSMPreparationUsesLocalFailureLimiter(t *testing.T) {
	var output bytes.Buffer
	engine := &INNBCBFT{
		logger: hclog.New(&hclog.LoggerOptions{
			Name:   "issue7a-fsm",
			Level:  hclog.Error,
			Output: &output,
		}),
	}

	var lastOrdinaryLog time.Time
	localErr := state.WrapLocalStateFailure(
		"prepare FSM",
		errors.New("injected governance read failure"),
	)

	for attempt := 0; attempt < 100; attempt++ {
		engine.logFSMPreparationError(10, localErr, &lastOrdinaryLog)
	}

	require.True(t, lastOrdinaryLog.IsZero())
	require.NotContains(t, output.String(), "failed to prepare consensus FSM")
	require.LessOrEqual(
		t,
		strings.Count(output.String(), "local state unavailable; operation aborted"),
		1,
	)

	output.Reset()
	ordinary := errors.New("ordinary FSM preparation failure")
	engine.logFSMPreparationError(10, ordinary, &lastOrdinaryLog)

	require.False(t, lastOrdinaryLog.IsZero())
	require.Equal(
		t,
		1,
		strings.Count(output.String(), "failed to prepare consensus FSM"),
	)

	// The ordinary retry throttle remains in effect.
	lastOrdinaryLog = time.Now().Add(time.Hour)
	engine.logFSMPreparationError(10, ordinary, &lastOrdinaryLog)
	require.Equal(
		t,
		1,
		strings.Count(output.String(), "failed to prepare consensus FSM"),
	)

	// A changed head remains quiet even when the ordinary window is open.
	output.Reset()
	lastOrdinaryLog = time.Time{}
	engine.logFSMPreparationError(10, errRuntimeHeadChanged, &lastOrdinaryLog)
	engine.logFSMPreparationError(10, nil, &lastOrdinaryLog)

	require.Empty(t, output.String())
	require.True(t, lastOrdinaryLog.IsZero())
}

/* FINISH ALPHA CHANGE 2195 -- test FSM preparation local-failure logging policy -- */
