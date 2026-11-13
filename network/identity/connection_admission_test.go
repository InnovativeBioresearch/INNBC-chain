/* START ALPHA CHANGE 2758 -- regress overlapping connection handshakes and stale failure cleanup -- */
package identity

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	networkTesting "github.com/innovativebioresearch/innbc-chain/network/testing"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	rawGrpc "google.golang.org/grpc"
)

type connectionAdmissionKey struct{}

type identityAdmissionConn struct {
	network.Conn
	id string
}

func (c *identityAdmissionConn) ID() string          { return c.id }
func (c *identityAdmissionConn) RemotePeer() peer.ID { return "same-peer" }

func (*identityAdmissionConn) Stat() network.ConnStats {
	return network.ConnStats{
		Stats: network.Stats{Direction: network.DirOutbound},
	}
}

type identityAdmissionBackend struct {
	*networkTesting.MockNetworkingServer
	added  chan string
	closed chan string
}

func (*identityAdmissionBackend) ConnectionContext(conn network.Conn) (context.Context, error) {
	return context.WithValue(context.Background(), connectionAdmissionKey{}, conn.ID()), nil
}

func (s *identityAdmissionBackend) AddPeerConnection(conn network.Conn) error {
	s.added <- conn.ID()

	return nil
}

func (s *identityAdmissionBackend) DisconnectConnection(conn network.Conn, _ string) {
	s.closed <- conn.ID()
}

func TestPeerIdentityAdmission_OldFailureDoesNotOwnReplacement(t *testing.T) {
	mock := networkTesting.NewMockNetworkingServer()
	oldEntered := make(chan struct{})
	newEntered := make(chan struct{})
	releaseOld := make(chan struct{})
	releaseNew := make(chan struct{})
	events := make(chan *event.PeerEvent, 2)

	var oldCalls atomic.Int32
	var newCalls atomic.Int32
	var pending atomic.Int64
	var oldReleaseOnce sync.Once
	var newReleaseOnce sync.Once

	mock.GetMockIdentityClient().HookHello(func(
		ctx context.Context,
		_ *proto.Status,
		_ ...rawGrpc.CallOption,
	) (*proto.Status, error) {
		switch ctx.Value(connectionAdmissionKey{}).(string) {
		case "old":
			oldCalls.Add(1)
			close(oldEntered)
			/* Deliberately delay the old backend beyond cancellation. */
			<-releaseOld
		case "new":
			newCalls.Add(1)
			close(newEntered)
			<-releaseNew
		}

		return &proto.Status{Chain: 1}, nil
	})
	mock.HookUpdatePendingConnCount(func(delta int64, _ network.Direction) {
		pending.Add(delta)
	})
	mock.HookEmitEvent(func(e *event.PeerEvent) { events <- e })

	backend := &identityAdmissionBackend{
		MockNetworkingServer: mock,
		added:                make(chan string, 2),
		closed:               make(chan string, 2),
	}
	service := NewIdentityService(backend, hclog.NewNullLogger(), 1, "local")
	t.Cleanup(func() {
		oldReleaseOnce.Do(func() { close(releaseOld) })
		newReleaseOnce.Do(func() { close(releaseNew) })
		service.Close()
	})

	notify := service.GetNotifyBundle()
	old := &identityAdmissionConn{id: "old"}
	replacement := &identityAdmissionConn{id: "new"}

	notify.ConnectedF(nil, old)
	waitIdentityRPCSignal(t, oldEntered)
	notify.ConnectedF(nil, old)
	require.Equal(t, int32(1), oldCalls.Load())

	notify.DisconnectedF(nil, old)
	notify.ConnectedF(nil, replacement)
	waitIdentityRPCSignal(t, newEntered)
	require.Equal(t, int32(1), newCalls.Load())

	newReleaseOnce.Do(func() { close(releaseNew) })
	select {
	case id := <-backend.added:
		require.Equal(t, "new", id)
	case <-time.After(5 * time.Second):
		t.Fatal("replacement handshake did not register")
	}

	oldReleaseOnce.Do(func() { close(releaseOld) })

	/* Both completion events establish that the workers reached their final paths. */
	for range 2 {
		select {
		case <-events:
		case <-time.After(5 * time.Second):
			t.Fatal("handshake did not reach its final event")
		}
	}
	service.Close()

	select {
	case id := <-backend.closed:
		require.Equal(t, "old", id)
	case <-time.After(5 * time.Second):
		t.Fatal("old handshake did not perform connection-specific cleanup")
	}
	require.Empty(t, backend.closed)
	require.Empty(t, backend.added)
	require.Zero(t, pending.Load())
	require.False(t, service.hasPendingStatus("same-peer"))
}

/* FINISH ALPHA CHANGE 2758 -- regress overlapping connection handshakes and stale failure cleanup -- */
