/* START ALPHA CHANGE 2882 -- regress tracked session cleanup notifications and replacement ownership -- */
package network

import (
	"testing"
	"time"

	peerEvent "github.com/innovativebioresearch/innbc-chain/network/event"
	libevent "github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type sessionEndedEventBatch struct {
	ended        []peerEvent.PeerEvent
	disconnected []peerEvent.PeerEvent
}

func subscribeSessionEndedEvents(
	t *testing.T,
	s *Server,
) libevent.Subscription {
	t.Helper()

	sub, err := s.host.EventBus().Subscribe(new(peerEvent.PeerEvent))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sub.Close()) })

	return sub
}

func readSessionEndedEventBatch(
	t *testing.T,
	s *Server,
	sub libevent.Subscription,
	id peer.ID,
) sessionEndedEventBatch {
	t.Helper()

	/*
		All cleanup calls in these fixtures are synchronous. This barrier
		follows their emissions and avoids using a sleep to assert absence.
	*/
	s.emitOwnedPeerEvent(id, peerEvent.PeerDialCompleted, nil)

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	var batch sessionEndedEventBatch

	for {
		select {
		case value, open := <-sub.Out():
			require.True(t, open, "event subscription closed before the barrier")

			e, ok := value.(peerEvent.PeerEvent)
			require.True(t, ok, "unexpected event type")
			require.Equal(t, id, e.PeerID)

			switch e.Type {
			case peerEvent.PeerDialCompleted:
				return batch
			case peerEvent.PeerSessionEnded:
				require.Nil(t, e.Current,
					"session cleanup must not carry an application-registration predicate")
				require.True(t, e.IsCurrent())
				batch.ended = append(batch.ended, e)
			case peerEvent.PeerDisconnected:
				batch.disconnected = append(batch.disconnected, e)
			}

		case <-timer.C:
			t.Fatal("session cleanup events did not reach the subscription barrier")
		}
	}
}

func TestPeerSessionEnded_ExistingEventValuesRemainUnchanged(t *testing.T) {
	require.Equal(t, peerEvent.PeerEventType(0), peerEvent.PeerConnected)
	require.Equal(t, peerEvent.PeerEventType(1), peerEvent.PeerFailedToConnect)
	require.Equal(t, peerEvent.PeerEventType(2), peerEvent.PeerDisconnected)
	require.Equal(t, peerEvent.PeerEventType(3), peerEvent.PeerDialCompleted)
	require.Equal(t, peerEvent.PeerEventType(4), peerEvent.PeerAddedToDialQueue)
	require.Equal(t, peerEvent.PeerEventType(5), peerEvent.PeerSessionEnded)
	require.Equal(t, "PeerSessionEnded", peerEvent.PeerSessionEnded.String())
}

func TestPeerSessionEnded_UnregisteredCleanup(t *testing.T) {
	for _, scenario := range []string{
		"disconnect",
		"session lookup already retired metadata",
		"revocation already retired metadata",
		"shutdown already retired metadata",
	} {
		t.Run(scenario, func(t *testing.T) {
			s, conn := peerSessionFixture(t)
			id := conn.RemotePeer()
			sub := subscribeSessionEndedEvents(t, s)

			require.NotNil(t, s.PeerSession(id))
			require.False(t, s.hasPeer(id),
				"fixture must not complete application registration")

			switch scenario {
			case "disconnect":
				require.NoError(t, conn.Close())
			case "session lookup already retired metadata":
				require.NoError(t, conn.Close())
				require.Nil(t, s.PeerSession(id))
			case "revocation already retired metadata":
				require.True(t, s.replaceAllowedPeers(nil))
				require.Nil(t, s.PeerSession(id))
			case "shutdown already retired metadata":
				s.stopPeerAdmission()
				require.NoError(t, conn.Close())
				require.Nil(t, s.PeerSession(id))
			}

			s.removePeerConnection(conn)

			batch := readSessionEndedEventBatch(t, s, sub, id)
			require.Len(t, batch.ended, 1)
			require.Empty(t, batch.disconnected,
				"unregistered cleanup must not change application disconnect semantics")
			require.True(t, s.WithNoPeerSession(id, func() {}))

			/*
				Repeated notification of this same removal must not emit
				another session event or close its removal signal twice.
			*/
			s.removePeerConnection(conn)
			s.removePeerConnection(conn)

			batch = readSessionEndedEventBatch(t, s, sub, id)
			require.Empty(t, batch.ended)
			require.Empty(t, batch.disconnected)
		})
	}
}

func TestPeerSessionEnded_PendingReplacementThenFailure(t *testing.T) {
	s, old := peerSessionFixture(t)
	id := old.RemotePeer()
	sub := subscribeSessionEndedEvents(t, s)

	oldToken := s.PeerSession(id)
	require.NoError(t, s.AddPeerConnection(old))
	readSessionEndedEventBatch(t, s, sub, id)

	require.NoError(t, old.Close())

	replacement := peerSessionReplacement(t, s)
	replacementToken := s.PeerSession(id)
	require.NotNil(t, replacementToken)
	require.NotSame(t, oldToken, replacementToken)

	s.removePeerConnection(old)

	batch := readSessionEndedEventBatch(t, s, sub, id)
	require.Empty(t, batch.ended,
		"old cleanup must preserve an eligible unregistered replacement")
	require.Len(t, batch.disconnected, 1,
		"registered old connections retain their application disconnect event")
	require.False(t, s.hasPeer(id))
	require.True(t, batch.disconnected[0].IsCurrent())
	require.False(t, s.WithNoPeerSession(id, func() {
		t.Error("status removal ran while the replacement was eligible")
	}))

	/*
		The replacement then fails before AddPeerConnection. Its own
		cleanup must produce the notification missing from the old path.
	*/
	require.NoError(t, replacement.Close())
	s.removePeerConnection(replacement)

	batch = readSessionEndedEventBatch(t, s, sub, id)
	require.Len(t, batch.ended, 1)
	require.Empty(t, batch.disconnected)
	require.True(t, s.WithNoPeerSession(id, func() {}))
}

func TestPeerSessionEnded_AnotherConnectionPreservesSession(t *testing.T) {
	s, first := peerSessionFixture(t)
	id := first.RemotePeer()
	second := peerSessionReplacement(t, s)
	token := s.PeerSession(id)
	sub := subscribeSessionEndedEvents(t, s)

	require.NoError(t, first.Close())
	s.removePeerConnection(first)

	batch := readSessionEndedEventBatch(t, s, sub, id)
	require.Empty(t, batch.ended)
	require.Same(t, token, s.PeerSession(id))
	require.False(t, s.WithNoPeerSession(id, func() {
		t.Error("another eligible connection was ignored")
	}))

	require.NoError(t, second.Close())
	s.removePeerConnection(second)

	batch = readSessionEndedEventBatch(t, s, sub, id)
	require.Len(t, batch.ended, 1)
	require.Nil(t, s.PeerSession(id))
}

/* FINISH ALPHA CHANGE 2882 -- regress tracked session cleanup notifications and replacement ownership -- */
