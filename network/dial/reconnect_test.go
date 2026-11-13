/* START ALPHA CHANGE 2778 -- regress deferred reconnect ownership address freshness cancellation and release races -- */
package dial

import (
	"context"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func reconnectAddrInfo(t *testing.T, id peer.ID, address string) *peer.AddrInfo {
	t.Helper()

	addr, err := multiaddr.NewMultiaddr(address)
	require.NoError(t, err)

	return &peer.AddrInfo{
		ID:    id,
		Addrs: []multiaddr.Multiaddr{addr},
	}
}

func TestDialQueueReconnect_LatestAddressAndStaleCompletion(t *testing.T) {
	q := NewDialQueue()
	t.Cleanup(q.Close)

	id := peer.ID("validator")
	first := reconnectAddrInfo(t, id, "/ip4/127.0.0.1/tcp/30301")
	second := reconnectAddrInfo(t, id, "/ip4/127.0.0.1/tcp/30302")
	latest := reconnectAddrInfo(t, id, "/ip4/127.0.0.1/tcp/30303")
	following := reconnectAddrInfo(t, id, "/ip4/127.0.0.1/tcp/30304")

	q.AddTask(first, common.PriorityRequestedDial)
	owner := q.PopDialTask()
	require.NotNil(t, owner)

	q.AddTask(second, common.PriorityRequestedDial)
	q.AddTask(latest, common.PriorityRandomDial)
	require.Nil(t, q.PopDialTask(), "busy peer must not become runnable")

	/* Another peer can still be popped while this peer retains ownership. */
	other := &peer.AddrInfo{ID: "other"}
	q.AddTask(other, common.PriorityRandomDial)
	otherOwner := q.PopDialTask()
	require.NotNil(t, otherOwner)
	require.Equal(t, other.ID, otherOwner.GetAddrInfo().ID)
	q.FinishDial(otherOwner)

	/* Remove earlier notifications so the next wait tests release notification. */
	for {
		select {
		case <-q.updateCh:
			continue
		default:
		}

		break
	}

	q.FinishDial(owner)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.False(t, q.Wait(ctx), "owner release must notify runnable pending work")

	replacement := q.PopDialTask()
	require.NotNil(t, replacement)
	require.Equal(t, latest, replacement.GetAddrInfo())
	require.Equal(t, uint64(common.PriorityRequestedDial), replacement.priority)
	require.Nil(t, q.PopDialTask(), "requests must coalesce to one replacement")

	q.AddTask(following, common.PriorityRequestedDial)

	/* A duplicate old completion must not release the replacement owner. */
	q.FinishDial(owner)
	require.Nil(t, q.PopDialTask())

	q.FinishDial(replacement)
	next := q.PopDialTask()
	require.NotNil(t, next)
	require.Equal(t, following, next.GetAddrInfo())
	require.Nil(t, q.PopDialTask())
	q.FinishDial(next)

	require.Empty(t, q.inFlight)
	require.Empty(t, q.pending)
}

func TestDialQueueReconnect_QueuedAddressUpdatesWithoutPriorityImprovement(t *testing.T) {
	q := NewDialQueue()
	t.Cleanup(q.Close)

	id := peer.ID("validator")
	first := reconnectAddrInfo(t, id, "/ip4/127.0.0.1/tcp/30301")
	latest := reconnectAddrInfo(t, id, "/ip4/127.0.0.1/tcp/30302")

	q.AddTask(first, common.PriorityRequestedDial)
	q.AddTask(latest, common.PriorityRandomDial)

	task := q.PopDialTask()
	require.NotNil(t, task)
	require.Equal(t, latest, task.GetAddrInfo())
	require.Equal(t, uint64(common.PriorityRequestedDial), task.priority)
	require.Nil(t, q.PopDialTask())
	q.FinishDial(task)
}

func TestDialQueueReconnect_DeleteAndCloseDiscardPendingWork(t *testing.T) {
	t.Run("delete permits a later explicit request", func(t *testing.T) {
		q := NewDialQueue()
		t.Cleanup(q.Close)

		info := &peer.AddrInfo{ID: "validator"}
		q.AddTask(info, common.PriorityRequestedDial)
		owner := q.PopDialTask()
		require.NotNil(t, owner)

		q.AddTask(info, common.PriorityRequestedDial)
		q.DeleteTask(info.ID)
		q.FinishDial(owner)

		require.Nil(t, q.PopDialTask())
		require.Empty(t, q.pending)

		q.AddTask(info, common.PriorityRequestedDial)
		replacement := q.PopDialTask()
		require.NotNil(t, replacement)
		q.FinishDial(replacement)
		require.Empty(t, q.inFlight)
	})

	t.Run("close prevents deferred or new attempts", func(t *testing.T) {
		q := NewDialQueue()
		t.Cleanup(q.Close)

		info := &peer.AddrInfo{ID: "validator"}
		q.AddTask(info, common.PriorityRequestedDial)
		owner := q.PopDialTask()
		require.NotNil(t, owner)

		q.AddTask(info, common.PriorityRequestedDial)
		q.Close()
		q.FinishDial(owner)
		q.AddTask(info, common.PriorityRequestedDial)

		require.Nil(t, q.PopDialTask())
		require.Empty(t, q.inFlight)
		require.Empty(t, q.pending)
		require.Empty(t, q.tasks)
	})
}

func TestDialQueueReconnect_RequestRacingWithRelease(t *testing.T) {
	q := NewDialQueue()
	t.Cleanup(q.Close)

	id := peer.ID("validator")
	first := reconnectAddrInfo(t, id, "/ip4/127.0.0.1/tcp/30301")
	latest := reconnectAddrInfo(t, id, "/ip4/127.0.0.1/tcp/30302")

	for iteration := 0; iteration < 100; iteration++ {
		q.AddTask(first, common.PriorityRequestedDial)
		owner := q.PopDialTask()
		require.NotNil(t, owner)

		start := make(chan struct{})
		added := make(chan struct{})
		released := make(chan struct{})

		go func() {
			defer close(added)
			<-start
			q.AddTask(latest, common.PriorityRequestedDial)
		}()
		go func() {
			defer close(released)
			<-start
			q.FinishDial(owner)
		}()

		close(start)

		select {
		case <-added:
		case <-time.After(5 * time.Second):
			t.Fatal("request submission did not finish")
		}
		select {
		case <-released:
		case <-time.After(5 * time.Second):
			t.Fatal("ownership release did not finish")
		}

		replacement := q.PopDialTask()
		require.NotNil(t, replacement, "iteration %d lost the reconnect request", iteration)
		require.Equal(t, latest, replacement.GetAddrInfo())
		require.Nil(t, q.PopDialTask())

		q.FinishDial(owner)
		q.FinishDial(replacement)
		require.Empty(t, q.inFlight)
		require.Empty(t, q.pending)
	}
}

/* FINISH ALPHA CHANGE 2778 -- regress deferred reconnect ownership address freshness cancellation and release races -- */
