/* START ALPHA CHANGE 2770 -- regress delayed status ownership and ordered map publication -- */
package syncer

import (
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	/* START ALPHA CHANGE 2822 -- import explicit ownership for status lifecycle fixtures -- */
	chainnetwork "github.com/innovativebioresearch/innbc-chain/network"
	/* FINISH ALPHA CHANGE 2822 -- import explicit ownership for status lifecycle fixtures -- */
	"github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2823 -- model session ownership separately from eligibility flags -- */
type statusOwnershipNetwork struct {
	Network
	allowed   atomic.Bool
	connected atomic.Bool
	sessions  testStatusNetwork
}

func (n *statusOwnershipNetwork) PeerSession(id peer.ID) *chainnetwork.PeerSession {
	if !n.allowed.Load() || !n.connected.Load() {
		return nil
	}

	return n.sessions.PeerSession(id)
}

func (n *statusOwnershipNetwork) WithPeerSession(
	id peer.ID,
	session *chainnetwork.PeerSession,
	publish func(),
) bool {
	if !n.allowed.Load() || !n.connected.Load() {
		return false
	}

	return n.sessions.WithPeerSession(id, session, publish)
}

/* START ALPHA CHANGE 2841 -- model removal against current eligible fixture sessions -- */
func (n *statusOwnershipNetwork) WithNoPeerSession(id peer.ID, remove func()) bool {
	if remove == nil {
		return false
	}

	n.sessions.lock.Lock()
	defer n.sessions.lock.Unlock()

	if n.allowed.Load() &&
		n.connected.Load() &&
		n.sessions.sessions[id] != nil {
		return false
	}

	remove()

	return true
}

/* FINISH ALPHA CHANGE 2841 -- model removal against current eligible fixture sessions -- */

/* FINISH ALPHA CHANGE 2823 -- model session ownership separately from eligibility flags -- */

func (n *statusOwnershipNetwork) IsPeerAllowed(peer.ID) bool {
	return n.allowed.Load()
}

func (n *statusOwnershipNetwork) IsConnected(peer.ID) bool {
	return n.connected.Load()
}

func statusOwnershipFixture() (*syncer, *statusOwnershipNetwork) {
	n := &statusOwnershipNetwork{}
	n.allowed.Store(true)
	n.connected.Store(true)

	return &syncer{
		logger:      hclog.NewNullLogger(),
		network:     n,
		peerMap:     new(PeerMap),
		closeCh:     make(chan struct{}),
		newStatusCh: make(chan struct{}, 1),
	}, n
}

func statusOwnershipValue(id peer.ID, number uint64) *NoForkPeer {
	return &NoForkPeer{
		ID:       id,
		Number:   number,
		Distance: big.NewInt(1),
	}
}

func statusOwnershipSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for status ownership regression signal")
	}
}

func statusOwnershipJoin(t *testing.T, s *syncer) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		s.closeWG.Wait()
		close(done)
	}()
	statusOwnershipSignal(t, done)
}

func TestPeerStatusOwnership_ConnectedResultCannotOutliveItsEvent(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		name := "revocation"
		if replacement {
			name = "replacement"
		}

		t.Run(name, func(t *testing.T) {
			s, n := statusOwnershipFixture()
			id := peer.ID("validator")
			oldStatus := statusOwnershipValue(id, 100)
			newStatus := statusOwnershipValue(id, 200)

			var oldCurrent atomic.Bool
			oldCurrent.Store(true)

			queryEntered := make(chan struct{})
			releaseQuery := make(chan struct{})
			disconnectHandled := make(chan struct{})
			events := make(chan *event.PeerEvent, 2)
			loopFinished := make(chan struct{})
			var releaseOnce sync.Once
			var eventsOnce sync.Once

			s.syncPeerClient = &mockSyncPeerClient{
				/* START ALPHA CHANGE 2824 -- retain the originating token before the delayed query -- */
				statusSession: n.PeerSession,
				/* FINISH ALPHA CHANGE 2824 -- retain the originating token before the delayed query -- */
				getPeerStatusHandler: func(peer.ID) (*NoForkPeer, error) {
					close(queryEntered)
					<-releaseQuery

					return oldStatus, nil
				},
				getPeerConnectionUpdateEventChHandler: func() <-chan *event.PeerEvent {
					return events
				},
			}

			t.Cleanup(func() {
				releaseOnce.Do(func() { close(releaseQuery) })
				eventsOnce.Do(func() { close(events) })
				statusOwnershipSignal(t, loopFinished)
				statusOwnershipJoin(t, s)
			})

			s.putToPeerMap(oldStatus)
			go func() {
				defer close(loopFinished)
				s.startPeerConnectionEventProcess()
			}()

			events <- &event.PeerEvent{
				PeerID:  id,
				Type:    event.PeerConnected,
				Current: oldCurrent.Load,
			}
			statusOwnershipSignal(t, queryEntered)

			oldCurrent.Store(false)
			n.allowed.Store(false)
			n.connected.Store(false)
			/* START ALPHA CHANGE 2825 -- retire fixture ownership before reauthorization -- */
			n.sessions.endSession(id)
			/* FINISH ALPHA CHANGE 2825 -- retire fixture ownership before reauthorization -- */

			events <- &event.PeerEvent{
				PeerID: id,
				Type:   event.PeerDisconnected,
				Current: func() bool {
					return !oldCurrent.Load()
				},
			}
			/* This next event is reached only after disconnect processing returns. */
			events <- &event.PeerEvent{
				PeerID: id,
				Type:   event.PeerDialCompleted,
				Current: func() bool {
					close(disconnectHandled)

					return false
				},
			}
			statusOwnershipSignal(t, disconnectHandled)

			_, present := s.peerMap.Load(id.String())
			require.False(t, present)

			if replacement {
				n.allowed.Store(true)
				n.connected.Store(true)
				s.putToPeerMap(newStatus)
			}

			releaseOnce.Do(func() { close(releaseQuery) })
			eventsOnce.Do(func() { close(events) })
			statusOwnershipSignal(t, loopFinished)
			statusOwnershipJoin(t, s)

			value, present := s.peerMap.Load(id.String())
			if replacement {
				require.True(t, present)
				require.Same(t, newStatus, value)
			} else {
				require.False(t, present)
			}
		})
	}
}

func TestPeerStatusOwnership_PublicationAndRemovalAreSerialized(t *testing.T) {
	/* START ALPHA CHANGE 2842 -- retain fixture ownership for explicit disconnection -- */
	s, n := statusOwnershipFixture()
	/* FINISH ALPHA CHANGE 2842 -- retain fixture ownership for explicit disconnection -- */
	id := peer.ID("validator")

	publicationEntered := make(chan struct{})
	releasePublication := make(chan struct{})
	publicationFinished := make(chan struct{})
	removalEntered := make(chan struct{})
	removalFinished := make(chan struct{})
	var releaseOnce sync.Once

	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releasePublication) })
		statusOwnershipSignal(t, publicationFinished)
		statusOwnershipSignal(t, removalFinished)
	})

	go func() {
		defer close(publicationFinished)
		s.putToPeerMap(statusOwnershipValue(id, 100), func() bool {
			close(publicationEntered)
			<-releasePublication

			return true
		})
	}()
	statusOwnershipSignal(t, publicationEntered)

	go func() {
		defer close(removalFinished)
		s.removeFromPeerMap(id, func() bool {
			close(removalEntered)

			/* START ALPHA CHANGE 2843 -- represent the ended transport before fixture removal -- */
			n.sessions.endSession(id)
			/* FINISH ALPHA CHANGE 2843 -- represent the ended transport before fixture removal -- */

			return true
		})
	}()

	/* Removal must not enter its check while publication owns the state lock. */
	select {
	case <-removalEntered:
		t.Fatal("removal overlapped an admitted status publication")
	case <-time.After(200 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(releasePublication) })
	statusOwnershipSignal(t, publicationFinished)
	statusOwnershipSignal(t, removalEntered)
	statusOwnershipSignal(t, removalFinished)

	_, present := s.peerMap.Load(id.String())
	require.False(t, present)
}

func TestPeerStatusOwnership_RevokedQueuedAndStartupResultsAreRejected(t *testing.T) {
	for _, startup := range []bool{false, true} {
		name := "queued"
		if startup {
			name = "startup"
		}

		t.Run(name, func(t *testing.T) {
			s, n := statusOwnershipFixture()
			id := peer.ID("validator")
			status := statusOwnershipValue(id, 100)

			/* START ALPHA CHANGE 2826 -- preserve source ownership through queued and startup fixtures -- */
			update := &PeerStatusUpdate{
				Status:  status,
				Session: n.PeerSession(id),
			}
			queue := make(chan *PeerStatusUpdate, 1)
			queue <- update
			close(queue)

			s.syncPeerClient = &mockSyncPeerClient{
				getPeerStatusUpdateChHandler: func() <-chan *PeerStatusUpdate {
					return queue
				},
				getConnectedPeerStatusUpdatesHandler: func() []*PeerStatusUpdate {
					return []*PeerStatusUpdate{update}
				},
			}
			/* FINISH ALPHA CHANGE 2826 -- preserve source ownership through queued and startup fixtures -- */

			n.allowed.Store(false)
			n.connected.Store(false)

			if startup {
				s.initializePeerMap()
			} else {
				s.startPeerStatusUpdateProcess()
			}

			_, present := s.peerMap.Load(id.String())
			require.False(t, present)

			/* A subsequent eligible status must still be accepted. */
			n.allowed.Store(true)
			n.connected.Store(true)
			fresh := statusOwnershipValue(id, 200)
			s.putToPeerMap(fresh)

			value, present := s.peerMap.Load(id.String())
			require.True(t, present)
			require.Same(t, fresh, value)
		})
	}
}

func TestPeerStatusOwnership_StaleDisconnectPreservesReplacement(t *testing.T) {
	s, _ := statusOwnershipFixture()
	id := peer.ID("validator")
	replacement := statusOwnershipValue(id, 200)
	s.putToPeerMap(replacement)

	s.removeFromPeerMap(id, func() bool { return false })

	value, present := s.peerMap.Load(id.String())
	require.True(t, present)
	require.Same(t, replacement, value)
}

/* FINISH ALPHA CHANGE 2770 -- regress delayed status ownership and ordered map publication -- */
