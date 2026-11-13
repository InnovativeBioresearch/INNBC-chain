/* START ALPHA CHANGE 2827 -- regress peer session preservation invalidation publication and subscription queues -- */
package network

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	googleproto "google.golang.org/protobuf/proto"
)

func peerSessionFixture(t *testing.T) (*Server, *admissionTestConn) {
	t.Helper()

	s := admissionFixture(t, true)
	require.True(t, s.startPeerAdmission())
	require.True(t, s.replaceAllowedPeers([]peer.ID{"validator"}))

	conn := &admissionTestConn{
		id: "old", remote: "validator", direction: libnetwork.DirOutbound,
	}
	_, err := s.ConnectionContext(conn)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		s.removePeerConnection(conn)
	})

	return s, conn
}

func peerSessionReplacement(t *testing.T, s *Server) *admissionTestConn {
	t.Helper()

	conn := &admissionTestConn{
		id: "replacement", remote: "validator", direction: libnetwork.DirInbound,
	}
	_, err := s.ConnectionContext(conn)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		s.removePeerConnection(conn)
	})

	return conn
}

func TestPeerSession_RefreshAndMultipleConnectionsPreserveOwnership(t *testing.T) {
	s, first := peerSessionFixture(t)
	id := first.RemotePeer()
	token := s.PeerSession(id)
	require.NotNil(t, token)

	require.True(t, s.replaceAllowedPeers([]peer.ID{id}))
	require.Same(t, token, s.PeerSession(id))

	second := peerSessionReplacement(t, s)
	require.Same(t, token, s.PeerSession(id))

	require.NoError(t, first.Close())
	s.removePeerConnection(first)
	require.Same(t, token, s.PeerSession(id))
	require.True(t, s.WithPeerSession(id, token, func() {}))

	require.NoError(t, second.Close())
	s.removePeerConnection(second)
	require.Nil(t, s.PeerSession(id))
	require.False(t, s.WithPeerSession(id, token, func() {}))
}

func TestPeerSession_ReplacementAndStaleCleanup(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		name := "closed transport before removal callback"
		if revoke {
			name = "revocation and reauthorization"
		}

		t.Run(name, func(t *testing.T) {
			s, old := peerSessionFixture(t)
			id := old.RemotePeer()
			oldToken := s.PeerSession(id)
			require.NotNil(t, oldToken)

			if revoke {
				require.True(t, s.replaceAllowedPeers(nil))
				require.Nil(t, s.PeerSession(id))
				require.True(t, s.replaceAllowedPeers([]peer.ID{id}))
			} else {
				require.NoError(t, old.Close())
			}

			replacement := peerSessionReplacement(t, s)
			newToken := s.PeerSession(id)
			require.NotNil(t, newToken)
			require.NotSame(t, oldToken, newToken)

			s.removePeerConnection(old)
			require.Same(t, newToken, s.PeerSession(id))
			require.False(t, replacement.IsClosed())

			require.False(t, s.WithPeerSession(id, oldToken, func() {}))
			require.True(t, s.WithPeerSession(id, newToken, func() {}))
			require.False(t, s.WithPeerSession("different", newToken, func() {}))
			require.False(t, s.WithPeerSession(id, nil, func() {}))

			other, _ := peerSessionFixture(t)
			require.False(t, other.WithPeerSession(id, newToken, func() {}))
		})
	}
}

func TestPeerSession_PublicationOrdersWithInvalidation(t *testing.T) {
	s, conn := peerSessionFixture(t)
	id := conn.RemotePeer()
	token := s.PeerSession(id)

	entered := make(chan struct{})
	release := make(chan struct{})
	publicationDone := make(chan bool, 1)
	invalidationStarted := make(chan struct{})
	invalidationDone := make(chan struct{})
	var releaseOnce sync.Once
	var published atomic.Bool

	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		admissionSignal(t, invalidationDone)
	})

	go func() {
		publicationDone <- s.WithPeerSession(id, token, func() {
			/* A controlled barrier stands in for the short production map write. */
			close(entered)
			<-release
			published.Store(true)
		})
	}()
	admissionSignal(t, entered)

	go func() {
		close(invalidationStarted)
		s.replaceAllowedPeers(nil)
		close(invalidationDone)
	}()
	admissionSignal(t, invalidationStarted)

	select {
	case <-invalidationDone:
		t.Fatal("invalidation completed while publication held session ownership")
	case <-time.After(200 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case accepted := <-publicationDone:
		require.True(t, accepted)
	case <-time.After(5 * time.Second):
		t.Fatal("publication did not finish")
	}
	admissionSignal(t, invalidationDone)

	require.True(t, published.Load())
	require.False(t, s.WithPeerSession(id, token, func() {
		t.Fatal("an invalidated token reached publication")
	}))
}

func TestPeerSession_ShutdownReleasesSessionMetadata(t *testing.T) {
	s, conn := peerSessionFixture(t)
	token := s.PeerSession(conn.RemotePeer())
	require.NotNil(t, token)

	s.stopPeerAdmission()

	require.Nil(t, s.PeerSession(conn.RemotePeer()))
	require.False(t, s.WithPeerSession(conn.RemotePeer(), token, func() {}))

	s.admission.lock.RLock()
	require.Empty(t, s.admission.sessions)
	require.Empty(t, token.connections)
	s.admission.lock.RUnlock()
}

type peerSessionSubscription struct {
	messages chan *pubsub.Message
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (sub *peerSessionSubscription) Next(ctx context.Context) (*pubsub.Message, error) {
	sub.once.Do(func() { close(sub.entered) })

	select {
	case <-sub.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case msg := <-sub.messages:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (*peerSessionSubscription) Cancel() {}

func TestPeerSessionGossip_SubscriptionQueueRetainsValidatedOwnership(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		name := "disconnect and reconnect"
		if revoke {
			name = "revoke and reauthorize"
		}

		t.Run(name, func(t *testing.T) {
			s, old := peerSessionFixture(t)
			id := old.RemotePeer()
			oldToken := s.PeerSession(id)

			data, err := googleproto.Marshal(&proto.GenericMessage{Message: "old"})
			require.NoError(t, err)
			topicName := "session-fixture"
			msg := &pubsub.Message{
				Message: &pubsubpb.Message{
					From:  []byte(id),
					Data:  data,
					Topic: &topicName,
				},
				ReceivedFrom: id,
			}

			require.True(t, s.capturePeerSessionValidation(context.Background(), id, msg))
			metadata, ok := msg.ValidatorData.(*validatedPeerSession)
			require.True(t, ok)
			require.Same(t, oldToken, metadata.session)

			sub := &peerSessionSubscription{
				messages: make(chan *pubsub.Message, 1),
				entered:  make(chan struct{}),
				release:  make(chan struct{}),
			}

			/*
				The production validation callback has completed, and this
				controlled subscription queue now contains that exact message.
				The reader is deliberately held before it can consume it.
			*/
			sub.messages <- msg

			topic := &Topic{
				logger:    hclog.NewNullLogger(),
				typ:       reflect.TypeOf(proto.GenericMessage{}),
				closeCh:   make(chan struct{}),
				authorize: s.gossipAllowed,
			}
			ctx, cancel := context.WithCancel(context.Background())
			queue := make(chan topicHandlerJob, 1)
			delivered := make(chan bool, 1)
			var selected atomic.Uint64
			var releaseOnce sync.Once

			t.Cleanup(func() {
				releaseOnce.Do(func() { close(sub.release) })
				cancel()
				topic.Close()
			})

			topic.waitGroup.Add(2)
			go topic.readLoop(ctx, cancel, sub, queue)
			go topic.runMetadataHandlerWorker(queue, func(
				_ any,
				from peer.ID,
				ownership *validatedPeerSession,
			) {
				if ownership == nil {
					delivered <- true

					return
				}

				delivered <- s.WithPeerSession(from, ownership.session, func() {
					selected.Store(100)
				})
			})
			admissionSignal(t, sub.entered)

			if revoke {
				require.True(t, s.replaceAllowedPeers(nil))
				require.True(t, s.replaceAllowedPeers([]peer.ID{id}))
			} else {
				require.NoError(t, old.Close())
			}

			peerSessionReplacement(t, s)
			newToken := s.PeerSession(id)
			require.NotNil(t, newToken)
			require.NotSame(t, oldToken, newToken)

			require.True(t, s.WithPeerSession(id, newToken, func() {
				selected.Store(200)
			}))
			s.removePeerConnection(old)

			/* Revalidation must not replace metadata already attached to the message. */
			require.True(t, s.capturePeerSessionValidation(context.Background(), id, msg))
			require.Same(t, metadata, msg.ValidatorData)

			releaseOnce.Do(func() { close(sub.release) })

			select {
			case accepted := <-delivered:
				require.False(t, accepted, "old subscription message acquired replacement ownership")
			case <-time.After(5 * time.Second):
				t.Fatal("held subscription message was not delivered")
			}
			require.Equal(t, uint64(200), selected.Load())
		})
	}
}

func TestPeerSessionGossip_PreservesForeignMetadataAndUnconnectedAuthors(t *testing.T) {
	s, _ := peerSessionFixture(t)
	require.True(t, s.replaceAllowedPeers([]peer.ID{"validator", "author"}))

	topicName := "session-fixture"
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			From:  []byte("author"),
			Topic: &topicName,
		},
		ReceivedFrom: "validator",
	}

	foreign := &struct{ Value string }{Value: "other validator"}
	msg.ValidatorData = foreign
	require.True(t, s.capturePeerSessionValidation(context.Background(), "validator", msg))
	require.Same(t, foreign, msg.ValidatorData)

	msg.ValidatorData = nil
	require.True(t, s.capturePeerSessionValidation(context.Background(), "validator", msg))
	metadata, ok := msg.ValidatorData.(*validatedPeerSession)
	require.True(t, ok)
	require.Nil(t, metadata.session)

	/* Session capture does not forbid forwarding an allowed, unconnected author. */
	require.True(t, s.gossipAllowed("validator", "author"))
}

/* FINISH ALPHA CHANGE 2827 -- regress peer session preservation invalidation publication and subscription queues -- */
