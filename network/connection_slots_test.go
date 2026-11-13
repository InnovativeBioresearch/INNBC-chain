/* START ALPHA CHANGE 2872 -- regress atomic last slot admission promotion and owned handshake cleanup -- */
package network

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	peerEvent "github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/innovativebioresearch/innbc-chain/network/identity"
	"github.com/innovativebioresearch/innbc-chain/network/proto"
	networkTesting "github.com/innovativebioresearch/innbc-chain/network/testing"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	rawGrpc "google.golang.org/grpc"
)

func slotReservationFixture(
	t *testing.T,
	limit int64,
	connections ...*admissionTestConn,
) *Server {
	t.Helper()

	s := admissionFixture(t, true)
	s.connectionCounts = NewBlankConnectionInfo(limit, limit)

	t.Cleanup(func() {
		s.stopPeerAdmission()
		for _, conn := range connections {
			_ = conn.Close()
			s.removePeerConnection(conn)
		}
	})

	allowed := make([]peer.ID, 0, len(connections))
	for _, conn := range connections {
		allowed = append(allowed, conn.RemotePeer())
	}

	require.True(t, s.startPeerAdmission())
	require.True(t, s.replaceAllowedPeers(allowed))

	for _, conn := range connections {
		_, err := s.ConnectionContext(conn)
		require.NoError(t, err)
	}

	return s
}

func requireSlotCounts(
	t *testing.T,
	s *Server,
	direction libnetwork.Direction,
	active int64,
	pending int64,
) {
	t.Helper()

	if direction == libnetwork.DirInbound {
		require.Equal(t, active, s.connectionCounts.GetInboundConnCount())
		require.Equal(t, pending, s.connectionCounts.GetPendingInboundConnCount())

		return
	}

	require.Equal(t, active, s.connectionCounts.GetOutboundConnCount())
	require.Equal(t, pending, s.connectionCounts.GetPendingOutboundConnCount())
}

func TestConnectionSlotReservation_LastSlotAndPromotion(t *testing.T) {
	for _, direction := range []libnetwork.Direction{
		libnetwork.DirInbound,
		libnetwork.DirOutbound,
	} {
		t.Run(direction.String(), func(t *testing.T) {
			first := &admissionTestConn{
				id: "first", remote: "first-peer", direction: direction,
			}
			second := &admissionTestConn{
				id: "second", remote: "second-peer", direction: direction,
			}
			s := slotReservationFixture(t, 1, first, second)

			type result struct {
				conn    *admissionTestConn
				release func()
				ok      bool
			}

			start := make(chan struct{})
			results := make(chan result, 2)
			finished := make(chan struct{})
			var workers sync.WaitGroup

			workers.Add(2)
			for _, conn := range []*admissionTestConn{first, second} {
				go func(conn *admissionTestConn) {
					defer workers.Done()
					<-start

					release, ok := s.ReserveConnectionSlot(conn)
					results <- result{conn: conn, release: release, ok: ok}
				}(conn)
			}
			go func() {
				workers.Wait()
				close(finished)
			}()

			close(start)

			var winner result
			var loser result
			successes := 0

			for range 2 {
				select {
				case value := <-results:
					if value.ok {
						successes++
						winner = value
					} else {
						loser = value
					}
				case <-time.After(5 * time.Second):
					t.Fatal("concurrent slot reservation did not finish")
				}
			}
			admissionSignal(t, finished)

			require.Equal(t, 1, successes, "only one peer may reserve the last slot")
			require.NotNil(t, winner.release)
			require.Nil(t, loser.release)
			requireSlotCounts(t, s, direction, 0, 1)

			require.NoError(t, s.AddPeerConnection(winner.conn))
			requireSlotCounts(t, s, direction, 1, 0)

			/*
				Promotion must not briefly free capacity, and delayed handshake
				cleanup must not decrement the registered active entry.
			*/
			release, ok := s.ReserveConnectionSlot(loser.conn)
			require.False(t, ok)
			require.Nil(t, release)

			winner.release()
			winner.release()
			requireSlotCounts(t, s, direction, 1, 0)

			require.NoError(t, winner.conn.Close())
			s.removePeerConnection(winner.conn)
			s.removePeerConnection(winner.conn)
			requireSlotCounts(t, s, direction, 0, 0)

			/*
				Once the owner is gone, the previously unsuccessful peer can
				reserve capacity without a permanent exclusion.
			*/
			release, ok = s.ReserveConnectionSlot(loser.conn)
			require.True(t, ok)
			require.NotNil(t, release)
			requireSlotCounts(t, s, direction, 0, 1)

			release()
			release()
			requireSlotCounts(t, s, direction, 0, 0)
		})
	}
}

func TestConnectionSlotReservation_MultipleConnectionsKeepPeerDirectionAccounting(t *testing.T) {
	first := &admissionTestConn{
		id: "first", remote: "validator", direction: libnetwork.DirInbound,
	}
	second := &admissionTestConn{
		id: "second", remote: "validator", direction: libnetwork.DirInbound,
	}
	other := &admissionTestConn{
		id: "other", remote: "other-validator", direction: libnetwork.DirInbound,
	}
	s := slotReservationFixture(t, 2, first, second, other)

	releaseFirst, ok := s.ReserveConnectionSlot(first)
	require.True(t, ok)
	require.NoError(t, s.AddPeerConnection(first))
	releaseFirst()
	requireSlotCounts(t, s, libnetwork.DirInbound, 1, 0)

	releaseSecond, ok := s.ReserveConnectionSlot(second)
	require.True(t, ok)
	requireSlotCounts(t, s, libnetwork.DirInbound, 1, 1)

	releaseOther, ok := s.ReserveConnectionSlot(other)
	require.False(t, ok)
	require.Nil(t, releaseOther)

	require.NoError(t, s.AddPeerConnection(second))
	releaseSecond()
	requireSlotCounts(t, s, libnetwork.DirInbound, 1, 0)

	/*
		The second registered connection does not consume another active
		peer-direction entry.
	*/
	releaseOther, ok = s.ReserveConnectionSlot(other)
	require.True(t, ok)
	require.NoError(t, s.AddPeerConnection(other))
	releaseOther()
	requireSlotCounts(t, s, libnetwork.DirInbound, 2, 0)

	require.NoError(t, first.Close())
	s.removePeerConnection(first)
	releaseFirst()
	require.True(t, s.isRegisteredConnection(second))
	requireSlotCounts(t, s, libnetwork.DirInbound, 2, 0)

	require.NoError(t, second.Close())
	s.removePeerConnection(second)
	releaseSecond()
	requireSlotCounts(t, s, libnetwork.DirInbound, 1, 0)

	require.NoError(t, other.Close())
	s.removePeerConnection(other)
	requireSlotCounts(t, s, libnetwork.DirInbound, 0, 0)
}

/* START ALPHA CHANGE 2875 -- regress repeated cleanup after re reservation on the same connection owner -- */
func TestConnectionSlotReservation_RepeatedCleanupPreservesSameOwnerReservation(t *testing.T) {
	for _, direction := range []libnetwork.Direction{
		libnetwork.DirInbound,
		libnetwork.DirOutbound,
	} {
		t.Run(direction.String(), func(t *testing.T) {
			conn := &admissionTestConn{
				id: "same-owner", remote: "validator", direction: direction,
			}
			other := &admissionTestConn{
				id: "other-owner", remote: "other-validator", direction: direction,
			}
			s := slotReservationFixture(t, 1, conn, other)

			cleanupA, ok := s.ReserveConnectionSlot(conn)
			require.True(t, ok)
			require.NotNil(t, cleanupA)
			requireSlotCounts(t, s, direction, 0, 1)

			cleanupA()
			requireSlotCounts(t, s, direction, 0, 0)

			/*
				Reserve again without disconnecting, replacing ownership,
				or creating a new connection context.
			*/
			cleanupB, ok := s.ReserveConnectionSlot(conn)
			require.True(t, ok)
			require.NotNil(t, cleanupB)
			requireSlotCounts(t, s, direction, 0, 1)

			cleanupA()
			cleanupA()
			requireSlotCounts(t, s, direction, 0, 1)

			/*
				The later reservation must still exclude another peer
				from the last available slot.
			*/
			otherCleanup, ok := s.ReserveConnectionSlot(other)
			require.False(t, ok)
			require.Nil(t, otherCleanup)

			cleanupB()
			cleanupB()
			cleanupA()
			requireSlotCounts(t, s, direction, 0, 0)

			otherCleanup, ok = s.ReserveConnectionSlot(other)
			require.True(t, ok)
			require.NotNil(t, otherCleanup)
			requireSlotCounts(t, s, direction, 0, 1)

			otherCleanup()
			requireSlotCounts(t, s, direction, 0, 0)
		})
	}
}

/* FINISH ALPHA CHANGE 2875 -- regress repeated cleanup after re reservation on the same connection owner -- */

func TestConnectionSlotReservation_StaleReleasePreservesReplacementOwner(t *testing.T) {
	old := &admissionTestConn{
		id: "reused-test-id", remote: "validator", direction: libnetwork.DirInbound,
	}
	replacement := &admissionTestConn{
		id: "reused-test-id", remote: "validator", direction: libnetwork.DirInbound,
	}

	/*
		Production libp2p identifiers are distinct. Deliberate reuse here
		verifies that cleanup checks the owner as well as the identifier.
	*/
	s := slotReservationFixture(t, 1, old)

	oldRelease, ok := s.ReserveConnectionSlot(old)
	require.True(t, ok)
	require.NoError(t, old.Close())
	s.removePeerConnection(old)
	requireSlotCounts(t, s, libnetwork.DirInbound, 0, 0)

	t.Cleanup(func() {
		_ = replacement.Close()
		s.removePeerConnection(replacement)
	})

	_, err := s.ConnectionContext(replacement)
	require.NoError(t, err)
	newRelease, ok := s.ReserveConnectionSlot(replacement)
	require.True(t, ok)
	requireSlotCounts(t, s, libnetwork.DirInbound, 0, 1)

	oldRelease()
	oldRelease()
	requireSlotCounts(t, s, libnetwork.DirInbound, 0, 1)

	newRelease()
	requireSlotCounts(t, s, libnetwork.DirInbound, 0, 0)
}

type slotIdentityBackend struct {
	*Server
	client proto.IdentityClient
	events chan peerEvent.PeerEventType
}

func (b *slotIdentityBackend) NewIdentityClient(
	ctx context.Context,
	_ peer.ID,
) (proto.IdentityClient, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	return b.client, func() {}, nil
}

func (b *slotIdentityBackend) DisconnectConnection(conn libnetwork.Conn, _ string) {
	_ = conn.Close()
	b.Server.removePeerConnection(conn)
}

func (b *slotIdentityBackend) EmitConnectionEvent(
	_ libnetwork.Conn,
	kind peerEvent.PeerEventType,
) {
	b.events <- kind
}

func TestIdentitySlotReservation_CompletionPaths(t *testing.T) {
	for _, mode := range []string{
		"success",
		"temporary",
		"failure",
		"shutdown",
		"revocation",
		"disconnect",
	} {
		t.Run(mode, func(t *testing.T) {
			conn := &admissionTestConn{
				id: "handshake", remote: "validator", direction: libnetwork.DirInbound,
			}
			s := slotReservationFixture(t, 1, conn)
			mock := networkTesting.NewMockNetworkingServer()
			entered := make(chan struct{})
			releaseHello := make(chan struct{})
			var releaseOnce sync.Once

			mock.GetMockIdentityClient().HookHello(func(
				ctx context.Context,
				_ *proto.Status,
				_ ...rawGrpc.CallOption,
			) (*proto.Status, error) {
				close(entered)

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-releaseHello:
				}

				if mode == "failure" {
					return nil, errors.New("controlled handshake failure")
				}

				return &proto.Status{
					Chain:         1,
					TemporaryDial: mode == "temporary",
				}, nil
			})

			backend := &slotIdentityBackend{
				Server: s,
				client: mock.GetMockIdentityClient(),
				events: make(chan peerEvent.PeerEventType, 2),
			}
			service := identity.NewIdentityService(backend, s.logger, 1, s.host.ID())

			t.Cleanup(func() {
				releaseOnce.Do(func() { close(releaseHello) })
				service.Close()
			})

			notify := service.GetNotifyBundle()
			notify.ConnectedF(nil, conn)
			admissionSignal(t, entered)
			requireSlotCounts(t, s, libnetwork.DirInbound, 0, 1)

			/*
				A duplicate notification must not reserve a second slot or
				start another handshake.
			*/
			notify.ConnectedF(nil, conn)
			requireSlotCounts(t, s, libnetwork.DirInbound, 0, 1)

			expectedEvent := peerEvent.PeerDialCompleted
			switch mode {
			case "shutdown":
				s.stopPeerAdmission()
				service.Close()
				expectedEvent = peerEvent.PeerFailedToConnect
			case "revocation":
				require.True(t, s.replaceAllowedPeers(nil))
				expectedEvent = peerEvent.PeerFailedToConnect
			case "disconnect":
				backend.DisconnectConnection(conn, "controlled disconnect")
				expectedEvent = peerEvent.PeerFailedToConnect
			default:
				releaseOnce.Do(func() { close(releaseHello) })
				if mode == "failure" {
					expectedEvent = peerEvent.PeerFailedToConnect
				}
			}

			select {
			case kind := <-backend.events:
				require.Equal(t, expectedEvent, kind)
			case <-time.After(5 * time.Second):
				t.Fatal("handshake did not complete reservation cleanup")
			}
			service.Close()

			expectedActive := int64(0)
			if mode == "success" {
				expectedActive = 1
				require.True(t, s.isRegisteredConnection(conn))
			}
			requireSlotCounts(t, s, libnetwork.DirInbound, expectedActive, 0)

			require.NoError(t, conn.Close())
			s.removePeerConnection(conn)
			s.removePeerConnection(conn)
			requireSlotCounts(t, s, libnetwork.DirInbound, 0, 0)
		})
	}
}

/* FINISH ALPHA CHANGE 2872 -- regress atomic last slot admission promotion and owned handshake cleanup -- */
