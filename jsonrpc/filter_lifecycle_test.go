package jsonrpc

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2539 -- test polling removal and timeout lifecycle --

The paused filter stops GetFilterChanges after its read phase has released
the manager lock, but before it attempts the timeout refresh. This forces
the reported uninstall interleaving without relying on scheduler timing.

FINISH ALPHA CHANGE 2539 -- test polling removal and timeout lifecycle -- */

type lifecyclePausedFilter struct {
	*logFilter
	checks atomic.Int32
	paused chan struct{}
	resume chan struct{}
}

func (f *lifecyclePausedFilter) hasWSConn() bool {
	if f.checks.Add(1) == 2 {
		close(f.paused)
		<-f.resume
	}

	return false
}

/* START ALPHA CHANGE 2539 -- cover detached refresh and expiry recovery -- */

func TestFilterLifecycle_PollUninstallDoesNotRestoreTimeout(t *testing.T) {
	t.Parallel()

	manager := NewFilterManager(hclog.NewNullLogger(), newMockStore(), 256)
	t.Cleanup(manager.Close)

	filter := &lifecyclePausedFilter{
		logFilter: &logFilter{
			filterBase: newFilterBase(nil),
			query:      &LogQuery{},
		},
		paused: make(chan struct{}),
		resume: make(chan struct{}),
	}

	id := manager.addFilter(filter)
	require.NotEmpty(t, id)

	/* Disregard the registration-time hasWSConn call. */
	filter.checks.Store(0)

	var resumeOnce sync.Once
	resume := func() {
		resumeOnce.Do(func() {
			close(filter.resume)
		})
	}
	t.Cleanup(resume)

	done := make(chan error, 1)
	go func() {
		_, err := manager.GetFilterChanges(id)
		done <- err
	}()

	select {
	case <-filter.paused:
	case <-time.After(5 * time.Second):
		t.Fatal("poll did not reach the gap before timeout refresh")
	}

	require.True(t, manager.Uninstall(id))
	resume()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("poll did not finish after uninstall")
	}

	manager.RLock()
	_, registered := manager.filters[id]
	heapSize := len(manager.timeouts)
	index := filter.heapIndex
	manager.RUnlock()

	require.False(t, registered)
	require.Zero(t, heapSize)
	require.Equal(t, NoIndexInHeap, index)
}

func TestFilterLifecycle_RefreshRejectsReplacedObject(t *testing.T) {
	t.Parallel()

	manager := NewFilterManager(hclog.NewNullLogger(), newMockStore(), 256)
	t.Cleanup(manager.Close)

	id := manager.NewLogFilter(&LogQuery{}, nil)
	require.NotEmpty(t, id)
	old := manager.getFilterByID(id).getFilterBase()
	require.True(t, manager.Uninstall(id))

	replacement := &logFilter{
		filterBase: newFilterBase(nil),
		query:      &LogQuery{},
	}
	replacement.id = id
	require.Equal(t, id, manager.addFilter(replacement))

	manager.Lock()
	deadline := replacement.expiresAt
	manager.refreshFilterTimeout(old)
	heapSize := len(manager.timeouts)
	head := manager.timeouts[0]
	oldIndex := old.heapIndex
	currentDeadline := replacement.expiresAt
	manager.Unlock()

	require.Equal(t, 1, heapSize)
	require.Same(t, replacement.getFilterBase(), head)
	require.Equal(t, NoIndexInHeap, oldIndex)
	require.Equal(t, deadline, currentDeadline)
}

func TestFilterLifecycle_StaleTimerPreservesRefreshedFilter(t *testing.T) {
	t.Parallel()

	manager := NewFilterManager(hclog.NewNullLogger(), newMockStore(), 256)
	t.Cleanup(manager.Close)

	id := manager.NewLogFilter(&LogQuery{}, nil)
	require.NotEmpty(t, id)
	base := manager.getFilterByID(id).getFilterBase()

	oldDeadline := time.Now().UTC().Add(-time.Minute)

	manager.Lock()
	base.expiresAt = oldDeadline
	heap.Fix(&manager.timeouts, base.heapIndex)
	manager.Unlock()

	selectedID, selectedDeadline := manager.nextTimeoutFilter()
	require.Equal(t, id, selectedID)
	require.Equal(t, oldDeadline, selectedDeadline)

	_, err := manager.GetFilterChanges(id)
	require.NoError(t, err)

	manager.RLock()
	newDeadline := base.expiresAt
	manager.RUnlock()
	require.True(t, newDeadline.After(selectedDeadline))

	/* Simulate the old timer being handled after the successful poll. */
	manager.expireFilters(selectedDeadline)
	require.True(t, manager.Exists(id))

	manager.RLock()
	heapSize := len(manager.timeouts)
	manager.RUnlock()
	require.Equal(t, 1, heapSize)

	/* Expiration at the refreshed deadline still removes the filter. */
	manager.expireFilters(newDeadline)
	require.False(t, manager.Exists(id))

	manager.RLock()
	heapSize = len(manager.timeouts)
	index := base.heapIndex
	manager.RUnlock()
	require.Zero(t, heapSize)
	require.Equal(t, NoIndexInHeap, index)
}

func TestFilterLifecycle_WorkerClearsDetachedTimeout(t *testing.T) {
	t.Parallel()

	manager := NewFilterManager(hclog.NewNullLogger(), newMockStore(), 256)
	t.Cleanup(manager.Close)

	id := manager.NewLogFilter(&LogQuery{}, nil)
	require.NotEmpty(t, id)
	detached := manager.getFilterByID(id).getFilterBase()
	require.True(t, manager.Uninstall(id))

	liveID := manager.NewLogFilter(&LogQuery{}, nil)
	require.NotEmpty(t, liveID)
	live := manager.getFilterByID(liveID).getFilterBase()

	manager.Lock()

	/*
		Seed the state left by the former race. Place a registered expired
		filter behind it to verify that stale-entry cleanup makes progress.
	*/
	detached.expiresAt = time.Now().UTC().Add(-2 * time.Minute)
	manager.timeouts.addFilter(detached)

	live.expiresAt = time.Now().UTC().Add(-time.Minute)
	heap.Fix(&manager.timeouts, live.heapIndex)

	manager.Unlock()

	go manager.Run()

	require.Eventually(t, func() bool {
		manager.RLock()
		defer manager.RUnlock()

		return len(manager.timeouts) == 0 &&
			len(manager.filters) == 0 &&
			detached.heapIndex == NoIndexInHeap &&
			live.heapIndex == NoIndexInHeap
	}, 5*time.Second, 10*time.Millisecond)
}

func TestFilterLifecycle_ChangeNotificationRemainsPending(t *testing.T) {
	t.Parallel()

	manager := NewFilterManager(hclog.NewNullLogger(), newMockStore(), 256)
	t.Cleanup(manager.Close)

	/*
		No worker is receiving yet. Changes must retain one notification
		instead of requiring a receiver to be waiting at that instant.
	*/
	manager.emitSignalToUpdateCh()
	manager.emitSignalToUpdateCh()

	select {
	case <-manager.updateCh:
	default:
		t.Fatal("timeout-change notification was lost without a waiting worker")
	}

	select {
	case <-manager.updateCh:
		t.Fatal("multiple pending notifications were retained")
	default:
	}
}

/* FINISH ALPHA CHANGE 2539 -- cover detached refresh and expiry recovery -- */
