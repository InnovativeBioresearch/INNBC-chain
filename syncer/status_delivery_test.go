/* START ALPHA CHANGE 2850 -- regress direct gossip delivery and shutdown under status backpressure -- */
package syncer

import (
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/network"
	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type statusDeliveryNetwork struct {
	Network
	entered chan struct{}
}

func (n *statusDeliveryNetwork) GetPeerDistance(id peer.ID) *big.Int {
	distance := n.Network.GetPeerDistance(id)

	select {
	case n.entered <- struct{}{}:
	default:
	}

	return distance
}

func TestPeerStatusDelivery_BoundedCallbackAndBackpressureShutdown(t *testing.T) {
	receiver := newTestNetwork(t)
	sender := newTestNetwork(t)

	require.NoError(t, network.JoinAndWaitMultiple(
		network.DefaultJoinTimeout,
		receiver,
		sender,
	))

	n := &statusDeliveryNetwork{
		Network: receiver,
		entered: make(chan struct{}, 64),
	}
	client := NewSyncPeerClient(hclog.NewNullLogger(), n, nil).(*syncPeerClient)

	/* An unread channel guarantees local delivery backpressure. */
	client.peerStatusUpdateCh = make(chan *PeerStatusUpdate)

	/*
		Holding worker admission distinguishes the direct topic callback from
		the old callback that delegated every message to startWorker.
	*/
	client.workerLock.Lock()
	workerAdmissionLocked := true

	t.Cleanup(func() {
		if workerAdmissionLocked {
			client.workerLock.Unlock()
			workerAdmissionLocked = false
		}
		client.Close()
	})

	require.NoError(t, client.startGossip())

	publisher, err := sender.NewTopic(statusTopicName, &proto.SyncPeerStatus{})
	require.NoError(t, err)
	t.Cleanup(publisher.Close)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()

	var number uint64
	var received *PeerStatusUpdate

	/*
		Publish distinct messages until subscription propagation permits delivery.
		No fixed sleep is used to assume that the subscription is ready.
	*/
	for received == nil {
		select {
		case <-ticker.C:
			number++
			require.NoError(t, publisher.Publish(&proto.SyncPeerStatus{
				Number: number,
			}))
		case update := <-client.peerStatusUpdateCh:
			require.NotNil(t, update)
			received = update
		case <-deadline.C:
			t.Fatal("status delivery did not proceed while worker admission was locked")
		}
	}

	require.Equal(t, sender.AddrInfo().ID, received.Status.ID)
	require.NotNil(t, received.Session)
	require.Same(t, receiver.PeerSession(received.Status.ID), received.Session)

	client.workerLock.Unlock()
	workerAdmissionLocked = false

	/* Remove earlier observations before waiting for a new delivery attempt. */
drain:
	for {
		select {
		case <-n.entered:
		default:
			break drain
		}
	}

	backpressureDeadline := time.NewTimer(15 * time.Second)
	defer backpressureDeadline.Stop()

	entered := false
	for !entered {
		select {
		case <-ticker.C:
			number++
			require.NoError(t, publisher.Publish(&proto.SyncPeerStatus{
				Number: number,
			}))
		case <-n.entered:
			entered = true
		case <-backpressureDeadline.C:
			t.Fatal("no callback entered the unread status delivery path")
		}
	}

	/*
		No consumer reads the status channel now. Close must cancel delivery,
		join the topic callbacks, and only then close that channel.
	*/
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		client.Close()
	}()
	statusOwnershipSignal(t, closed)

	_, open := <-client.peerStatusUpdateCh
	require.False(t, open)
	require.True(t, client.peerStatusUpdateChClosed)
	require.Nil(t, client.topic)
}

/* FINISH ALPHA CHANGE 2850 -- regress direct gossip delivery and shutdown under status backpressure -- */
