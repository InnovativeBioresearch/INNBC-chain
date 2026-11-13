/* START ALPHA CHANGE 2848 -- regress atomic status removal against eligible connection sessions -- */
package network

import (
	"sync"
	"testing"
	"time"

	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestPeerSessionRemoval_PendingReplacementPreventsRemoval(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		name := "disconnect"
		if revoke {
			name = "revocation"
		}

		t.Run(name, func(t *testing.T) {
			s, old := peerSessionFixture(t)
			id := old.RemotePeer()

			if revoke {
				require.True(t, s.replaceAllowedPeers(nil))
				require.True(t, s.replaceAllowedPeers([]peer.ID{id}))
			} else {
				require.NoError(t, old.Close())
				s.removePeerConnection(old)
			}

			replacement := peerSessionReplacement(t, s)
			token := s.PeerSession(id)
			require.NotNil(t, token)
			require.False(t, s.hasPeer(id), "application registration has not completed")

			called := false
			require.False(t, s.WithNoPeerSession(id, func() {
				called = true
			}))
			require.False(t, called)

			s.removePeerConnection(old)
			require.Same(t, token, s.PeerSession(id))
			require.False(t, replacement.IsClosed())
			require.False(t, s.WithNoPeerSession(id, func() {
				called = true
			}))
			require.False(t, called)

			require.NoError(t, replacement.Close())
			s.removePeerConnection(replacement)

			require.True(t, s.WithNoPeerSession(id, func() {
				called = true
			}))
			require.True(t, called)
		})
	}
}

func TestPeerSessionRemoval_AnotherConnectionPreservesEligibility(t *testing.T) {
	s, first := peerSessionFixture(t)
	id := first.RemotePeer()
	second := peerSessionReplacement(t, s)

	require.NoError(t, first.Close())
	s.removePeerConnection(first)

	require.False(t, s.WithNoPeerSession(id, func() {
		t.Error("removal ran while another eligible connection remained")
	}))
	require.False(t, s.WithNoPeerSession(id, nil))

	require.NoError(t, second.Close())
	s.removePeerConnection(second)

	called := false
	require.True(t, s.WithNoPeerSession(id, func() {
		called = true
	}))
	require.True(t, called)
}

func TestPeerSessionRemoval_RegistrationOrdersWithDeletion(t *testing.T) {
	s, old := peerSessionFixture(t)
	id := old.RemotePeer()
	require.NoError(t, old.Close())
	s.removePeerConnection(old)

	entered := make(chan struct{})
	release := make(chan struct{})
	removalDone := make(chan struct{})
	removalResult := make(chan bool, 1)

	startRegistration := make(chan struct{})
	registrationStarted := make(chan struct{})
	registrationDone := make(chan struct{})
	registrationResult := make(chan error, 1)

	var releaseOnce sync.Once
	var startOnce sync.Once

	replacement := &admissionTestConn{
		id:        "replacement-after-deletion",
		remote:    id,
		direction: libnetwork.DirInbound,
	}

	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		startOnce.Do(func() { close(startRegistration) })
		admissionSignal(t, removalDone)
		admissionSignal(t, registrationDone)
		_ = replacement.Close()
		s.removePeerConnection(replacement)
	})

	go func() {
		defer close(removalDone)
		removalResult <- s.WithNoPeerSession(id, func() {
			/* Deliberate test barrier inside the atomic removal callback. */
			close(entered)
			<-release
		})
	}()

	go func() {
		defer close(registrationDone)
		<-startRegistration
		close(registrationStarted)
		_, err := s.ConnectionContext(replacement)
		registrationResult <- err
	}()

	admissionSignal(t, entered)
	startOnce.Do(func() { close(startRegistration) })
	admissionSignal(t, registrationStarted)

	select {
	case <-registrationDone:
		t.Fatal("registration completed while atomic deletion still owned the admission lock")
	case <-time.After(200 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	admissionSignal(t, removalDone)
	admissionSignal(t, registrationDone)

	require.True(t, <-removalResult)
	require.NoError(t, <-registrationResult)
	require.NotNil(t, s.PeerSession(id))
	require.False(t, s.WithNoPeerSession(id, func() {
		t.Error("replacement session was ignored after registration")
	}))
}

/* FINISH ALPHA CHANGE 2848 -- regress atomic status removal against eligible connection sessions -- */
