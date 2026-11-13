/* START ALPHA CHANGE 2849 -- regress delayed disconnect cleanup against replacement status publication -- */
package syncer

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestPeerStatusRemoval_PendingReplacementKeepsFreshStatus(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		name := "disconnect"
		if revoke {
			name = "revocation"
		}

		t.Run(name, func(t *testing.T) {
			s, n := statusOwnershipFixture()
			id := peer.ID("validator")

			oldToken := n.PeerSession(id)
			s.publishPeerStatus(&PeerStatusUpdate{
				Status:  statusOwnershipValue(id, 100),
				Session: oldToken,
			})

			if revoke {
				n.allowed.Store(false)
			}
			n.connected.Store(false)
			n.sessions.endSession(id)

			n.allowed.Store(true)
			n.connected.Store(true)
			newToken := n.PeerSession(id)
			require.NotSame(t, oldToken, newToken)

			fresh := statusOwnershipValue(id, 200)
			s.publishPeerStatus(&PeerStatusUpdate{
				Status:  fresh,
				Session: newToken,
			})

			/*
				The old disconnect's application-table check can still return true
				while the eligible replacement has not completed registration.
			*/
			s.removeFromPeerMap(id, func() bool { return true })

			value, present := s.peerMap.Load(id.String())
			require.True(t, present)
			require.Same(t, fresh, value)

			n.connected.Store(false)
			n.sessions.endSession(id)
			s.removeFromPeerMap(id, func() bool { return true })

			_, present = s.peerMap.Load(id.String())
			require.False(t, present)
		})
	}
}

func TestPeerStatusRemoval_RevocationStillRemovesStatus(t *testing.T) {
	s, n := statusOwnershipFixture()
	id := peer.ID("validator")
	s.putToPeerMap(statusOwnershipValue(id, 100))

	n.allowed.Store(false)
	n.sessions.endSession(id)
	s.removeFromPeerMap(id, func() bool { return true })

	_, present := s.peerMap.Load(id.String())
	require.False(t, present)
}

/* FINISH ALPHA CHANGE 2849 -- regress delayed disconnect cleanup against replacement status publication -- */
