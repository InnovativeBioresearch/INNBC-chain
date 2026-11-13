/* START ALPHA CHANGE 3162 -- verify orderly signal shutdown waiting fatal errors and explicit forced exit -- */
package helper

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/stretchr/testify/require"
)

type shutdownSignalsOutput struct {
	result command.CommandResult
	writes int
}

func (*shutdownSignalsOutput) SetError(error) {}

func (o *shutdownSignalsOutput) SetCommandResult(result command.CommandResult) {
	o.result = result
}

func (o *shutdownSignalsOutput) WriteOutput() {
	o.writes++
}

func (*shutdownSignalsOutput) WriteCommandResult(command.CommandResult) {}

func (*shutdownSignalsOutput) Write(p []byte) (int, error) {
	return len(p), nil
}

func startShutdownSignalHandler(
	closeFn func(),
	outputter command.OutputFormatter,
	fatalErrCh <-chan error,
	signalCh <-chan os.Signal,
) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- handleSignalsOrError(closeFn, outputter, fatalErrCh, signalCh)
	}()
	return done
}

func shutdownSignalResult(t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown handler did not return")
		return nil
	}
}

func shutdownSignalEvent(t *testing.T, event <-chan struct{}, message string) {
	t.Helper()

	select {
	case <-event:
	case <-time.After(3 * time.Second):
		t.Fatal(message)
	}
}

func TestShutdownSignals_WaitsBeyondFiveSeconds(t *testing.T) {
	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGTERM

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var releaseOnce sync.Once
	var calls atomic.Int32
	output := &shutdownSignalsOutput{}

	closeFn := func() {
		calls.Add(1)
		close(started)
		<-release
		close(finished)
	}
	defer func() {
		releaseOnce.Do(func() { close(release) })
		shutdownSignalEvent(t, finished, "cleanup did not finish")
	}()

	done := startShutdownSignalHandler(closeFn, output, nil, signals)
	shutdownSignalEvent(t, started, "cleanup did not start")

	// Keep cleanup blocked beyond the removed production five-second cutoff.
	select {
	case err := <-done:
		t.Fatalf("handler returned before cleanup completed: %v", err)
	case <-time.After(5*time.Second + 250*time.Millisecond):
	}

	require.Equal(t, int32(1), calls.Load())
	releaseOnce.Do(func() { close(release) })
	require.NoError(t, shutdownSignalResult(t, done))
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, 1, output.writes)
	require.NotNil(t, output.result)
	require.Contains(t, output.result.GetOutput(), "Gracefully shutting down client")
}

func TestShutdownSignals_NilCleanup(t *testing.T) {
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	output := &shutdownSignalsOutput{}

	done := startShutdownSignalHandler(nil, output, nil, signals)
	require.NoError(t, shutdownSignalResult(t, done))
	require.Equal(t, 1, output.writes)
	require.NotNil(t, output.result)
}

func TestShutdownSignals_FatalWaitsAndPreservesError(t *testing.T) {
	cause := errors.New("injected fatal consensus failure")
	fatal := make(chan error, 1)
	fatal <- cause

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var releaseOnce sync.Once
	var calls atomic.Int32
	output := &shutdownSignalsOutput{}

	closeFn := func() {
		calls.Add(1)
		close(started)
		<-release
		close(finished)
	}
	defer func() {
		releaseOnce.Do(func() { close(release) })
		shutdownSignalEvent(t, finished, "fatal cleanup did not finish")
	}()

	done := startShutdownSignalHandler(closeFn, output, fatal, nil)
	shutdownSignalEvent(t, started, "fatal cleanup did not start")

	select {
	case err := <-done:
		t.Fatalf("fatal error returned before cleanup completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	err := shutdownSignalResult(t, done)
	require.ErrorIs(t, err, cause)
	require.ErrorContains(t, err, "fatal consensus error")
	require.Equal(t, int32(1), calls.Load())
	require.Zero(t, output.writes)
}

func TestShutdownSignals_FatalChannelPolicy(t *testing.T) {
	for _, mode := range []string{"closed", "nil_error"} {
		t.Run(mode, func(t *testing.T) {
			fatal := make(chan error, 1)
			if mode == "closed" {
				close(fatal)
			} else {
				fatal <- nil
			}

			calls := 0
			output := &shutdownSignalsOutput{}
			err := handleSignalsOrError(
				func() { calls++ },
				output,
				fatal,
				nil,
			)

			require.ErrorContains(t, err,
				"fatal runtime error channel closed without an error")
			require.Equal(t, 1, calls)
			require.Zero(t, output.writes)
		})
	}

	cause := errors.New("fatal error with no cleanup callback")
	fatal := make(chan error, 1)
	fatal <- cause
	require.ErrorIs(t,
		handleSignalsOrError(nil, &shutdownSignalsOutput{}, fatal, nil),
		cause,
	)
}

func TestShutdownSignals_SecondSignalReturnsBeforeCleanup(t *testing.T) {
	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGTERM

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var releaseOnce sync.Once
	var calls atomic.Int32
	output := &shutdownSignalsOutput{}

	closeFn := func() {
		calls.Add(1)
		close(started)
		<-release
		close(finished)
	}
	defer func() {
		releaseOnce.Do(func() { close(release) })
		shutdownSignalEvent(t, finished, "forced-exit fixture cleanup did not finish")
	}()

	done := startShutdownSignalHandler(closeFn, output, nil, signals)
	shutdownSignalEvent(t, started, "cleanup did not start")

	signals <- os.Interrupt
	err := shutdownSignalResult(t, done)
	require.EqualError(t, err, "shutdown by signal channel")
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, 1, output.writes)

	select {
	case <-finished:
		t.Fatal("fixture cleanup unexpectedly finished before its release")
	default:
	}
}

func TestShutdownSignals_ProcessRegistrationStopsOnReturn(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("subprocess termination assertion targets the production Linux platform")
	}

	const childFlag = "INNBC_SHUTDOWN_SIGNAL_REGISTRATION_CHILD"
	if os.Getenv(childFlag) == "1" {
		cause := errors.New("subprocess fatal shutdown")
		fatal := make(chan error, 1)
		fatal <- cause
		calls := 0

		err := HandleSignalsOrError(
			func() { calls++ },
			&shutdownSignalsOutput{},
			fatal,
		)
		require.ErrorIs(t, err, cause)
		require.Equal(t, 1, calls)

		// The public handler must have unregistered SIGTERM before returning.
		// Send an actual process signal only inside this isolated subprocess.
		process, err := os.FindProcess(os.Getpid())
		require.NoError(t, err)
		require.NoError(t, process.Signal(syscall.SIGTERM))

		time.Sleep(250 * time.Millisecond)
		t.Fatal("SIGTERM remained intercepted after the handler returned")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	child := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run=^TestShutdownSignals_ProcessRegistrationStopsOnReturn$",
		"-test.count=1",
	)
	child.Env = append(os.Environ(), childFlag+"=1")
	output, err := child.CombinedOutput()

	require.NoError(t, ctx.Err(), string(output))
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, string(output))
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	require.True(t, status.Signaled(), string(output))
	require.Equal(t, syscall.SIGTERM, status.Signal(), string(output))
}

/* FINISH ALPHA CHANGE 3162 -- verify orderly signal shutdown waiting fatal errors and explicit forced exit -- */
