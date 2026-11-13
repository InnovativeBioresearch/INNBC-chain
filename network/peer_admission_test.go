/* START ALPHA CHANGE 2756 -- regress startup membership ownership refresh and gossip delivery -- */
package network

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	peerEvent "github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	"github.com/libp2p/go-libp2p"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/stretchr/testify/require"
	googleproto "google.golang.org/protobuf/proto"
)

type admissionTestConn struct {
	libnetwork.Conn
	id        string
	remote    peer.ID
	direction libnetwork.Direction
	closed    atomic.Bool
	check     func()
}

func (c *admissionTestConn) ID() string          { return c.id }
func (c *admissionTestConn) RemotePeer() peer.ID { return c.remote }

func (c *admissionTestConn) Stat() libnetwork.ConnStats {
	return libnetwork.ConnStats{
		Stats: libnetwork.Stats{Direction: c.direction},
	}
}

func (c *admissionTestConn) IsClosed() bool {
	if c.check != nil {
		c.check()
	}

	return c.closed.Load()
}

func (c *admissionTestConn) Close() error {
	c.closed.Store(true)

	return nil
}

func admissionSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for admission regression signal")
	}
}

func admissionFixture(t *testing.T, restricted bool) *Server {
	t.Helper()

	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, h.Close()) })

	emitter, err := h.EventBus().Emitter(new(peerEvent.PeerEvent))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, emitter.Close()) })

	return &Server{
		logger:           hclog.NewNullLogger(),
		host:             h,
		admission:        newPeerAdmission(restricted),
		peers:            make(map[peer.ID]*PeerConnInfo),
		connectionCounts: NewBlankConnectionInfo(32, 8),
		emitterPeerEvent: emitter,
	}
}

func TestPeerAdmission_ReadinessAndTerminalShutdown(t *testing.T) {
	s := admissionFixture(t, true)
	id := peer.ID("validator")

	require.True(t, s.replaceAllowedPeers([]peer.ID{id}))
	require.False(t, s.IsPeerAllowed(id), "a response alone must not open startup")

	require.True(t, s.startPeerAdmission())
	require.True(t, s.IsPeerAllowed(id))
	require.False(t, s.IsPeerAllowed("unknown"))

	gater := admissionGater{policy: s.admission}
	require.True(t, gater.InterceptPeerDial(id))
	require.False(t, gater.InterceptPeerDial("unknown"))

	s.stopPeerAdmission()
	require.False(t, s.replaceAllowedPeers([]peer.ID{"unknown"}))
	require.False(t, s.startPeerAdmission())
	require.False(t, s.IsPeerAllowed(id))
	require.False(t, gater.InterceptAccept(nil))
}

func TestPeerAdmission_DirectoryPendingAndUnrestrictedStartup(t *testing.T) {
	restricted := admissionFixture(t, true)
	require.True(t, restricted.startPeerAdmission())
	require.False(t, restricted.IsPeerAllowed("validator"))

	unrestricted := admissionFixture(t, false)
	require.False(t, unrestricted.IsPeerAllowed("validator"))
	require.True(t, unrestricted.startPeerAdmission())
	require.True(t, unrestricted.IsPeerAllowed("validator"))

	require.True(t, unrestricted.replaceAllowedPeers([]peer.ID{"selected"}))
	require.True(t, unrestricted.IsPeerAllowed("selected"))
	require.False(t, unrestricted.IsPeerAllowed("validator"))
}

func TestPeerAdmission_StartupRejectsThenAuthorizedReconnectSucceeds(t *testing.T) {
	client, err := CreateServer(&CreateServerParams{
		ConfigCallback: func(c *Config) { c.NoDiscover = true },
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	target, err := CreateServer(&CreateServerParams{
		ConfigCallback: func(c *Config) {
			c.NoDiscover = true
			c.Chain.Params.Engine = map[string]interface{}{
				"innbcbft": map[string]interface{}{},
			}
			c.PeerDirectoryURL = "http://127.0.0.1:1/api/v1/peers/exchange"
		},
		ServerCallback: func(s *Server) {
			t.Cleanup(func() { require.NoError(t, s.Close()) })
			s.SetAllowedPeers([]peer.ID{s.host.ID(), client.host.ID()})
			require.False(t, s.IsPeerAllowed(client.host.ID()))

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			require.Error(t, client.host.Connect(ctx, *s.AddrInfo()))
			require.Empty(t, s.host.Network().Conns())
		},
	})
	require.NoError(t, err)
	require.True(t, target.IsPeerAllowed(client.host.ID()))

	client.host.Network().(*swarm.Swarm).Backoff().Clear(target.host.ID())

	require.NoError(t, JoinAndWait(client, target, 5*time.Second, 5*time.Second))
	require.True(t, client.hasPeer(target.host.ID()))
	require.True(t, target.hasPeer(client.host.ID()))
}

func TestPeerAdmission_StaleCleanupPreservesReplacement(t *testing.T) {
	s := admissionFixture(t, true)
	id := peer.ID("validator")
	require.True(t, s.startPeerAdmission())
	require.True(t, s.replaceAllowedPeers([]peer.ID{id}))

	old := &admissionTestConn{
		id: "old", remote: id, direction: libnetwork.DirOutbound,
	}
	oldContext, err := s.ConnectionContext(old)
	require.NoError(t, err)
	require.NoError(t, s.AddPeerConnection(old))

	require.True(t, s.replaceAllowedPeers(nil))
	admissionSignal(t, oldContext.Done())
	require.True(t, old.closed.Load())

	require.True(t, s.replaceAllowedPeers([]peer.ID{id}))
	replacement := &admissionTestConn{
		id: "replacement", remote: id, direction: libnetwork.DirOutbound,
	}
	_, err = s.ConnectionContext(replacement)
	require.NoError(t, err)
	require.NoError(t, s.AddPeerConnection(replacement))

	require.Error(t, s.AddPeerConnection(old))
	s.removePeerConnection(old)

	require.True(t, s.isRegisteredConnection(replacement))
	require.False(t, replacement.closed.Load())
	require.Equal(t, int64(1), s.connectionCounts.GetOutboundConnCount())

	stale := &peerEvent.PeerEvent{
		PeerID: id,
		Type:   peerEvent.PeerDisconnected,
		Current: func() bool {
			return !s.hasPeer(id)
		},
	}
	require.False(t, stale.IsCurrent())

	s.removePeerConnection(replacement)
	require.False(t, s.hasPeer(id))
	require.Zero(t, s.connectionCounts.GetOutboundConnCount())

	s.admission.lock.RLock()
	retained := len(s.admission.connections)
	s.admission.lock.RUnlock()
	require.Zero(t, retained)
}

func TestPeerAdmission_MultipleConnectionsRetainPeerUntilLastRemoval(t *testing.T) {
	s := admissionFixture(t, false)
	require.True(t, s.startPeerAdmission())

	first := &admissionTestConn{
		id: "first", remote: "validator", direction: libnetwork.DirInbound,
	}
	second := &admissionTestConn{
		id: "second", remote: "validator", direction: libnetwork.DirInbound,
	}
	for _, conn := range []*admissionTestConn{first, second} {
		_, err := s.ConnectionContext(conn)
		require.NoError(t, err)
		require.NoError(t, s.AddPeerConnection(conn))
	}

	require.Equal(t, int64(1), s.connectionCounts.GetInboundConnCount())
	s.removePeerConnection(first)
	require.True(t, s.isRegisteredConnection(second))
	require.Equal(t, int64(1), s.connectionCounts.GetInboundConnCount())

	s.removePeerConnection(second)
	require.False(t, s.hasPeer("validator"))
	require.Zero(t, s.connectionCounts.GetInboundConnCount())
}

func TestPeerAdmission_RegistrationOrdersWithRevocation(t *testing.T) {
	s := admissionFixture(t, true)
	require.True(t, s.startPeerAdmission())
	require.True(t, s.replaceAllowedPeers([]peer.ID{"validator"}))

	conn := &admissionTestConn{
		id: "ordered", remote: "validator", direction: libnetwork.DirOutbound,
	}
	ctx, err := s.ConnectionContext(conn)
	require.NoError(t, err)

	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	conn.check = func() {
		close(entered)
		<-release
	}

	registered := make(chan error, 1)
	go func() { registered <- s.AddPeerConnection(conn) }()
	admissionSignal(t, entered)

	refreshed := make(chan struct{})
	go func() {
		s.replaceAllowedPeers(nil)
		close(refreshed)
	}()

	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-registered:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("registration failed to complete")
	}
	admissionSignal(t, refreshed)
	admissionSignal(t, ctx.Done())
	require.True(t, conn.closed.Load())

	/* Avoid calling the one-shot observation hook again. */
	conn.check = nil
	require.Error(t, s.AddPeerConnection(conn))
	s.removePeerConnection(conn)
	require.False(t, s.hasPeer("validator"))
}

func TestPeerAdmission_QueuedGossipChecksBothIdentities(t *testing.T) {
	for _, revoke := range []peer.ID{"relay", "author", ""} {
		t.Run(string(revoke), func(t *testing.T) {
			s := admissionFixture(t, true)
			require.True(t, s.startPeerAdmission())
			require.True(t, s.replaceAllowedPeers([]peer.ID{
				"relay", "author", "control",
			}))
			require.True(t, s.gossipAllowed("relay", "author"))

			data, err := googleproto.Marshal(&proto.GenericMessage{Message: "queued"})
			require.NoError(t, err)
			control, err := googleproto.Marshal(&proto.GenericMessage{Message: "control"})
			require.NoError(t, err)

			queue := make(chan topicHandlerJob, 2)
			queue <- topicHandlerJob{
				data: data, from: "author", receivedFrom: "relay",
			}
			queue <- topicHandlerJob{
				data: control, from: "control", receivedFrom: "control",
			}
			close(queue)

			allowed := []peer.ID{"control"}
			if revoke != "relay" {
				allowed = append(allowed, "relay")
			}
			if revoke != "author" {
				allowed = append(allowed, "author")
			}
			require.True(t, s.replaceAllowedPeers(allowed))

			topic := &Topic{
				logger:    hclog.NewNullLogger(),
				typ:       reflect.TypeOf(proto.GenericMessage{}),
				closeCh:   make(chan struct{}),
				authorize: s.gossipAllowed,
			}
			topic.waitGroup.Add(1)
			done := make(chan struct{})
			delivered := make(chan string, 2)

			go func() {
				defer close(done)
				topic.runHandlerWorker(queue, func(obj interface{}, _ peer.ID) {
					delivered <- obj.(*proto.GenericMessage).Message
				})
			}()
			admissionSignal(t, done)
			close(delivered)

			var messages []string
			for message := range delivered {
				messages = append(messages, message)
			}

			if revoke == "" {
				/* No direct author connection exists in this fixture. */
				require.Equal(t, []string{"queued", "control"}, messages)
			} else {
				require.Equal(t, []string{"control"}, messages)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2756 -- regress startup membership ownership refresh and gossip delivery -- */
