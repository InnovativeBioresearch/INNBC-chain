// START ALPHA CHANGE 3792 -- regress local gossip while directory admission is pending --
package network

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	testproto "github.com/innovativebioresearch/innbc-chain/network/proto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	googleproto "google.golang.org/protobuf/proto"
)

func localGossipAdmissionPendingServer(t *testing.T) *Server {
	t.Helper()

	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once

	directory := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		enteredOnce.Do(func() { close(entered) })

		select {
		case <-r.Context().Done():
			return
		case <-release:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(func() {
		close(release)
		directory.Close()
	})

	cfg := gossipLifetimeConfig(t)
	cfg.PeerDirectoryEnabled = true
	cfg.PeerDirectoryURL = directory.URL + "/api/v1/peers/exchange"
	cfg.Chain.Params.Engine = map[string]interface{}{
		"innbcbft": map[string]interface{}{},
	}
	require.True(t, peerDirectoryRestricted(cfg))

	server, err := NewServer(hclog.NewNullLogger(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		gossipLifetimeCloseServer(t, server)
		require.NoError(t, server.emitterPeerEvent.Close())
	})

	require.NoError(t, server.Start())
	gossipLifetimeWait(t, entered, "the pending directory request")

	server.admission.lock.RLock()
	started := server.admission.started
	restricted := server.admission.restricted
	initialized := server.admission.initialized
	closed := server.admission.closed
	server.admission.lock.RUnlock()

	require.True(t, started)
	require.True(t, restricted)
	require.False(t, initialized)
	require.False(t, closed)

	return server
}

func TestLocalGossipAdmission_PendingDirectoryPublication(t *testing.T) {
	server := localGossipAdmissionPendingServer(t)
	self := server.host.ID()

	topic, err := server.NewTopic(
		"local-gossip-pending-directory",
		&testproto.GenericMessage{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { gossipLifetimeCloseTopic(t, topic) })

	type delivery struct {
		message string
		author  peer.ID
		valid   bool
	}
	delivered := make(chan delivery, 1)
	var callbacks atomic.Int32

	require.NoError(t, topic.Subscribe(func(obj any, author peer.ID) {
		callbacks.Add(1)
		message, ok := obj.(*testproto.GenericMessage)
		result := delivery{author: author, valid: ok}
		if ok {
			result.message = message.Message
		}
		select {
		case delivered <- result:
		default:
		}
	}))

	require.NoError(t, topic.Publish(&testproto.GenericMessage{
		Message: "local-publication-with-directory-pending",
	}))

	select {
	case result := <-delivered:
		require.True(t, result.valid)
		require.Equal(t, self, result.author)
		require.Equal(t,
			"local-publication-with-directory-pending",
			result.message)
	case <-time.After(gossipLifetimeWatchdog):
		t.Fatal("local publication never reached the real subscriber")
	}

	// Joining topic shutdown bounds the callback count.
	gossipLifetimeCloseTopic(t, topic)
	require.Equal(t, int32(1), callbacks.Load())
	require.ErrorIs(t,
		topic.Publish(&testproto.GenericMessage{Message: "after-close"}),
		ErrTopicClosed)

	server.admission.lock.RLock()
	initialized := server.admission.initialized
	server.admission.lock.RUnlock()
	require.False(t, initialized, "the test must not authorize the directory")

	require.Nil(t, server.PeerSession(self),
		"local delivery must not invent a remote-peer session")
	require.False(t, server.IsPeerAllowed(peer.ID("unknown-remote")))
}

func TestLocalGossipAdmission_PolicyMatrix(t *testing.T) {
	tests := []struct {
		name        string
		started     bool
		initialized bool
		includeSelf bool
		includePeer bool
		revokePeer  bool
		stopped     bool
		wantLocal   bool
		wantPeer    bool
		wantMixed   bool
	}{
		{name: "not_started"},
		{
			name:        "response_before_start",
			initialized: true,
			includeSelf: true,
			includePeer: true,
		},
		{
			name:      "directory_pending",
			started:   true,
			wantLocal: true,
		},
		{
			name:        "initialized_self_only",
			started:     true,
			initialized: true,
			includeSelf: true,
			wantLocal:   true,
		},
		{
			name:        "initialized_self_and_peer",
			started:     true,
			initialized: true,
			includeSelf: true,
			includePeer: true,
			wantLocal:   true,
			wantPeer:    true,
			wantMixed:   true,
		},
		{
			// Directory application normally includes self. This also
			// checks the local-only exception independently of that fact.
			name:        "initialized_self_excluded",
			started:     true,
			initialized: true,
			includePeer: true,
			wantLocal:   true,
			wantPeer:    true,
		},
		{
			name:        "remote_revoked",
			started:     true,
			initialized: true,
			includeSelf: true,
			includePeer: true,
			revokePeer:  true,
			wantLocal:   true,
		},
		{
			name:        "stopped",
			started:     true,
			initialized: true,
			includeSelf: true,
			includePeer: true,
			stopped:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := admissionFixture(t, true)
			self := server.host.ID()
			remote := peer.ID("authorized-remote")
			denied := peer.ID("denied-remote")

			if test.started {
				require.True(t, server.startPeerAdmission())
			}
			if test.initialized {
				var allowed []peer.ID
				if test.includeSelf {
					allowed = append(allowed, self)
				}
				if test.includePeer {
					allowed = append(allowed, remote)
				}
				require.True(t, server.replaceAllowedPeers(allowed))
			}
			if test.revokePeer {
				require.True(t, server.replaceAllowedPeers([]peer.ID{self}))
			}
			if test.stopped {
				server.stopPeerAdmission()
			}

			require.Equal(t, test.wantLocal, server.gossipAllowed(self, self))
			require.Equal(t, test.wantPeer, server.gossipAllowed(remote, remote))
			require.Equal(t, test.wantMixed, server.gossipAllowed(self, remote))
			require.Equal(t, test.wantMixed, server.gossipAllowed(remote, self))

			require.False(t, server.gossipAllowed(denied, self))
			require.False(t, server.gossipAllowed(self, denied))
			require.False(t, server.gossipAllowed(denied, denied))
			require.False(t, server.gossipAllowed("", self))
			require.False(t, server.gossipAllowed(self, ""))

			if test.stopped {
				require.False(t, server.replaceAllowedPeers([]peer.ID{self, remote}))
				require.False(t, server.startPeerAdmission())
				require.False(t, server.gossipAllowed(self, self))
			}
		})
	}
}

func TestLocalGossipAdmission_EmptyDirectoryResponse(t *testing.T) {
	server := admissionFixture(t, true)
	require.True(t, server.startPeerAdmission())

	require.NoError(t, server.applyPeerDirectoryPeers(nil))
	require.True(t, server.gossipAllowed(server.host.ID(), server.host.ID()))
	require.False(t, server.gossipAllowed("unknown", "unknown"))

	server.admission.lock.RLock()
	initialized := server.admission.initialized
	server.admission.lock.RUnlock()
	require.True(t, initialized)
}

func TestLocalGossipAdmission_QueuedDelivery(t *testing.T) {
	for _, state := range []string{"pending", "authorized", "revoked", "stopped"} {
		t.Run(state, func(t *testing.T) {
			server := admissionFixture(t, true)
			require.True(t, server.startPeerAdmission())
			self := server.host.ID()
			remote := peer.ID("remote")

			if state != "pending" {
				require.True(t, server.replaceAllowedPeers([]peer.ID{self, remote}))
			}
			if state == "revoked" {
				require.True(t, server.replaceAllowedPeers([]peer.ID{self}))
			}
			if state == "stopped" {
				server.stopPeerAdmission()
			}

			queue := make(chan topicHandlerJob, 2)
			for _, input := range []struct {
				text string
				id   peer.ID
			}{
				{text: "remote", id: remote},
				{text: "local", id: self},
			} {
				data, err := googleproto.Marshal(&testproto.GenericMessage{
					Message: input.text,
				})
				require.NoError(t, err)
				queue <- topicHandlerJob{
					data: data, from: input.id, receivedFrom: input.id,
				}
			}
			close(queue)

			topic := &Topic{
				logger:    hclog.NewNullLogger(),
				typ:       reflect.TypeOf(testproto.GenericMessage{}),
				closeCh:   make(chan struct{}),
				authorize: server.gossipAllowed,
			}
			topic.waitGroup.Add(1)

			done := make(chan struct{})
			delivered := make(chan string, 2)
			go func() {
				defer close(done)
				topic.runHandlerWorker(queue, func(obj any, _ peer.ID) {
					delivered <- obj.(*testproto.GenericMessage).Message
				})
			}()
			gossipLifetimeWait(t, done, "the completed gossip work queue")
			close(delivered)

			var actual []string
			for message := range delivered {
				actual = append(actual, message)
			}

			var expected []string
			switch state {
			case "authorized":
				expected = []string{"remote", "local"}
			case "pending", "revoked":
				expected = []string{"local"}
			}
			require.Equal(t, expected, actual)
		})
	}
}

// FINISH ALPHA CHANGE 3792 -- regress local gossip while directory admission is pending --
