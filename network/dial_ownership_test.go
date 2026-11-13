/* START ALPHA CHANGE 2769 -- regress dial recovery independent slot ownership and joined shutdown -- */
package network

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/innovativebioresearch/innbc-chain/network/dial"

	/* START ALPHA CHANGE 2856 -- exercise discovery cleanup in dial ownership regressions -- */
	"github.com/innovativebioresearch/innbc-chain/network/discovery"
	/* FINISH ALPHA CHANGE 2856 -- exercise discovery cleanup in dial ownership regressions -- */
	peerEvent "github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/innovativebioresearch/innbc-chain/network/identity"
	"github.com/libp2p/go-libp2p/core/host"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	/* START ALPHA CHANGE 2779 -- construct distinct reconnect addresses in ownership regressions -- */
	"github.com/multiformats/go-multiaddr"
	/* FINISH ALPHA CHANGE 2779 -- construct distinct reconnect addresses in ownership regressions -- */
	"github.com/stretchr/testify/require"
)

type ownedDialTestNetwork struct {
	libnetwork.Network
	connections []*admissionTestConn
}

func (n *ownedDialTestNetwork) Conns() []libnetwork.Conn {
	var result []libnetwork.Conn
	for _, conn := range n.connections {
		if !conn.IsClosed() {
			result = append(result, conn)
		}
	}

	return result
}

type ownedDialTestHost struct {
	host.Host
	network libnetwork.Network
	connect func(context.Context, peer.AddrInfo) error
}

func (h *ownedDialTestHost) Network() libnetwork.Network {
	return h.network
}

func (h *ownedDialTestHost) Connect(ctx context.Context, info peer.AddrInfo) error {
	return h.connect(ctx, info)
}

func ownedDialFixture(
	t *testing.T,
	restricted bool,
	connections []*admissionTestConn,
	connect func(*Server, context.Context, peer.AddrInfo) error,
) (*Server, <-chan struct{}) {
	t.Helper()

	s := admissionFixture(t, restricted)
	h := s.host
	s.host = &ownedDialTestHost{
		Host: h,
		network: &ownedDialTestNetwork{
			Network:     h.Network(),
			connections: connections,
		},
		connect: func(ctx context.Context, info peer.AddrInfo) error {
			return connect(s, ctx, info)
		},
	}
	s.closeCh = make(chan struct{})
	s.dialQueue = dial.NewDialQueue()
	s.connectionCounts = NewBlankConnectionInfo(32, 1)
	require.True(t, s.startPeerAdmission())

	finished := make(chan struct{})
	t.Cleanup(func() {
		for _, conn := range connections {
			_ = conn.Close()
			s.removePeerConnection(conn)
		}

		require.NoError(t, s.Close())
		admissionSignal(t, finished)
	})

	go func() {
		defer close(finished)
		s.runDial()
	}()

	return s, finished
}

func TestPeerDialOwnership_EarlyRejectionAllowsNextAttempt(t *testing.T) {
	first := &admissionTestConn{
		id: "rejected", remote: "first", direction: libnetwork.DirOutbound,
	}
	second := &admissionTestConn{
		id: "healthy", remote: "second", direction: libnetwork.DirOutbound,
	}

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondResult := make(chan error, 1)
	var releaseOnce sync.Once
	var firstReturnedSuccess atomic.Bool

	s, _ := ownedDialFixture(
		t,
		true,
		[]*admissionTestConn{first, second},
		func(s *Server, ctx context.Context, info peer.AddrInfo) error {
			switch info.ID {
			case first.RemotePeer():
				allowed, _ := (admissionGater{policy: s.admission}).InterceptUpgraded(first)
				if !allowed {
					return errors.New("first attempt did not pass the connection gate")
				}

				close(firstEntered)
				select {
				case <-releaseFirst:
				case <-ctx.Done():
					return ctx.Err()
				}

				if !s.replaceAllowedPeers([]peer.ID{second.RemotePeer()}) {
					return errors.New("membership replacement failed")
				}

				service := identity.NewIdentityService(s, s.logger, 1, s.host.ID())
				service.GetNotifyBundle().ConnectedF(nil, first)
				service.Close()
				s.removePeerConnection(first)

				/* Model the pinned host's successful return after callback rejection. */
				firstReturnedSuccess.Store(true)

				return nil
			case second.RemotePeer():
				_, err := s.ConnectionContext(second)
				if err == nil {
					err = s.AddPeerConnection(second)
				}
				secondResult <- err

				return err
			default:
				return errors.New("unexpected dial target")
			}
		},
	)
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })

	require.True(t, s.replaceAllowedPeers([]peer.ID{
		first.RemotePeer(), second.RemotePeer(),
	}))

	s.dialQueue.AddTask(&peer.AddrInfo{ID: first.RemotePeer()}, common.PriorityRequestedDial)
	admissionSignal(t, firstEntered)

	s.dialQueue.AddTask(&peer.AddrInfo{ID: second.RemotePeer()}, common.PriorityRequestedDial)
	releaseOnce.Do(func() { close(releaseFirst) })

	select {
	case err := <-secondResult:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("early rejection stranded the only outbound dial slot")
	}

	require.True(t, firstReturnedSuccess.Load())
	require.True(t, first.closed.Load())
	require.False(t, s.hasPeer(first.RemotePeer()))
	require.True(t, s.isRegisteredConnection(second))
	require.Equal(t, int64(1), s.connectionCounts.GetOutboundConnCount())
}

func TestPeerDialOwnership_UnrelatedFailureDoesNotReleaseOccupiedSlot(t *testing.T) {
	first := &admissionTestConn{
		id: "first", remote: "first", direction: libnetwork.DirOutbound,
	}
	second := &admissionTestConn{
		id: "second", remote: "second", direction: libnetwork.DirOutbound,
	}
	firstResult := make(chan error, 1)
	secondResult := make(chan error, 1)

	s, _ := ownedDialFixture(
		t,
		false,
		[]*admissionTestConn{first, second},
		func(s *Server, _ context.Context, info peer.AddrInfo) error {
			conn := first
			result := firstResult
			if info.ID == second.RemotePeer() {
				conn = second
				result = secondResult
			}

			_, err := s.ConnectionContext(conn)
			if err == nil {
				err = s.AddPeerConnection(conn)
			}
			result <- err

			return err
		},
	)

	s.dialQueue.AddTask(&peer.AddrInfo{ID: first.RemotePeer()}, common.PriorityRequestedDial)
	select {
	case err := <-firstResult:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("first outbound connection did not establish")
	}

	s.dialQueue.AddTask(&peer.AddrInfo{ID: second.RemotePeer()}, common.PriorityRequestedDial)
	s.emitEvent("unrelated", peerEvent.PeerFailedToConnect)

	/* Observe the forbidden extra dial while the owning transport remains alive. */
	select {
	case err := <-secondResult:
		t.Fatalf("unrelated failure released an occupied dial slot: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, first.Close())
	s.removePeerConnection(first)

	select {
	case err := <-secondResult:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("owned transport removal did not release the dial slot")
	}

	require.True(t, s.isRegisteredConnection(second))
	require.Equal(t, int64(1), s.connectionCounts.GetOutboundConnCount())
}

func TestPeerDialOwnership_ShutdownCancelsBlockedConnect(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})

	s, managerFinished := ownedDialFixture(
		t,
		false,
		nil,
		func(_ *Server, ctx context.Context, _ peer.AddrInfo) error {
			close(entered)
			defer close(exited)

			<-ctx.Done()

			return ctx.Err()
		},
	)

	s.dialQueue.AddTask(&peer.AddrInfo{ID: "blocked"}, common.PriorityRequestedDial)
	admissionSignal(t, entered)

	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()

	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not cancel and join the blocked dial")
	}

	admissionSignal(t, exited)
	admissionSignal(t, managerFinished)
}

/* START ALPHA CHANGE 2780 -- regress same peer reconnect during old worker cleanup and reauthorization -- */

type reconnectObservationNetwork struct {
	libnetwork.Network
	witness peer.ID
	reached chan struct{}
	once    sync.Once
}

func (n *reconnectObservationNetwork) Connectedness(id peer.ID) libnetwork.Connectedness {
	if id == n.witness {
		n.once.Do(func() { close(n.reached) })
	}

	return n.Network.Connectedness(id)
}

func TestPeerDialOwnership_SamePeerReconnectDuringCleanup(t *testing.T) {
	for _, reauthorize := range []bool{false, true} {
		name := "ordinary reconnect"
		if reauthorize {
			name = "removal followed by reauthorization"
		}

		t.Run(name, func(t *testing.T) {
			id := peer.ID("validator")
			witnessID := peer.ID("queue-witness")

			old := &admissionTestConn{
				id: "old", remote: id, direction: libnetwork.DirOutbound,
			}
			replacement := &admissionTestConn{
				id: "replacement", remote: id, direction: libnetwork.DirOutbound,
			}
			replacement.closed.Store(true)

			firstAddress, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/30301")
			require.NoError(t, err)
			secondAddress, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/30302")
			require.NoError(t, err)
			latestAddress, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/30303")
			require.NoError(t, err)

			firstInfo := &peer.AddrInfo{
				ID: id, Addrs: []multiaddr.Multiaddr{firstAddress},
			}
			secondInfo := &peer.AddrInfo{
				ID: id, Addrs: []multiaddr.Multiaddr{secondAddress},
			}
			latestInfo := &peer.AddrInfo{
				ID: id, Addrs: []multiaddr.Multiaddr{latestAddress},
			}

			oldRemoved := make(chan struct{})
			releaseOld := make(chan struct{})
			firstResult := make(chan error, 1)
			replacementResult := make(chan error, 1)
			replacementInfo := make(chan peer.AddrInfo, 1)
			witnessReached := make(chan struct{})
			var releaseOnce sync.Once
			var attempts atomic.Int32
			var witnessAttempts atomic.Int32

			s, _ := ownedDialFixture(
				t,
				true,
				[]*admissionTestConn{old, replacement},
				func(s *Server, ctx context.Context, info peer.AddrInfo) error {
					if info.ID == witnessID {
						witnessAttempts.Add(1)

						/* No retained transport: this witness immediately returns its slot. */
						return nil
					}
					if info.ID != id {
						return errors.New("unexpected reconnect target")
					}

					switch attempts.Add(1) {
					case 1:
						_, err := s.ConnectionContext(old)
						if err == nil {
							err = s.AddPeerConnection(old)
						}
						if err == nil {
							err = old.Close()
						}
						if err == nil {
							s.removePeerConnection(old)
						}

						firstResult <- err
						close(oldRemoved)
						if err != nil {
							return err
						}

						/* The transport is gone, but its dial worker still owns the slot. */
						select {
						case <-releaseOld:
							return nil
						case <-ctx.Done():
							return ctx.Err()
						}

					case 2:
						replacement.closed.Store(false)
						_, err := s.ConnectionContext(replacement)
						if err == nil {
							err = s.AddPeerConnection(replacement)
						}
						replacementInfo <- info
						replacementResult <- err

						return err

					default:
						return errors.New("more than one replacement attempt")
					}
				},
			)
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(releaseOld) })
			})

			h := s.host.(*ownedDialTestHost)
			h.network = &reconnectObservationNetwork{
				Network: h.network,
				witness: witnessID,
				reached: witnessReached,
			}

			require.True(t, s.replaceAllowedPeers([]peer.ID{id, witnessID}))

			/* START ALPHA CHANGE 2857 -- retain the production removal callback in the paused worker regression -- */
			routingTable, err := s.newDiscoveryRoutingTable()
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, routingTable.Close())
			})

			/*
				This test explicitly schedules every dial request. Disable
				automatic discovery additions while retaining the production
				PeerRemoved callback unchanged.
			*/
			routingTable.PeerAdded = func(peer.ID) {}

			added, err := routingTable.TryAddPeer(id, false, false)
			require.NoError(t, err)
			require.True(t, added)
			require.Contains(t, routingTable.ListPeers(), id)

			discoveryService := discovery.NewDiscoveryService(s, routingTable, s.logger)
			t.Cleanup(discoveryService.Close)
			/* FINISH ALPHA CHANGE 2857 -- retain the production removal callback in the paused worker regression -- */

			s.dialQueue.AddTask(firstInfo, common.PriorityRequestedDial)

			admissionSignal(t, oldRemoved)
			select {
			case err := <-firstResult:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("old connection cleanup did not report its result")
			}
			require.False(t, s.hasPeer(id))

			if reauthorize {
				require.True(t, s.replaceAllowedPeers([]peer.ID{witnessID}))
				require.False(t, s.IsPeerAllowed(id))
				require.True(t, s.replaceAllowedPeers([]peer.ID{id, witnessID}))
			}

			s.dialQueue.AddTask(secondInfo, common.PriorityRequestedDial)
			s.dialQueue.AddTask(latestInfo, common.PriorityRequestedDial)

			/* START ALPHA CHANGE 2858 -- evict the old route after submitting a fresh same peer reconnect -- */
			discoveryService.HandleNetworkEvent(&peerEvent.PeerEvent{
				PeerID: id,
				Type:   peerEvent.PeerDisconnected,
				Current: func() bool {
					return !s.hasPeer(id)
				},
			})
			require.NotContains(t, routingTable.ListPeers(), id)
			/* FINISH ALPHA CHANGE 2858 -- evict the old route after submitting a fresh same peer reconnect -- */

			/*
				The lower-priority witness proves the dial loop advanced through
				the queue while the old worker still retained ownership.
				The retained reconnect must survive discovery eviction and wait
				until the old worker releases its slot and task ownership.
			*/
			s.dialQueue.AddTask(
				&peer.AddrInfo{ID: witnessID},
				common.PriorityRandomDial,
			)
			admissionSignal(t, witnessReached)

			require.Equal(t, int32(1), attempts.Load())
			select {
			case err := <-replacementResult:
				t.Fatalf("replacement started before old ownership release: %v", err)
			default:
			}

			releaseOnce.Do(func() { close(releaseOld) })

			select {
			case err := <-replacementResult:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("same-peer reconnect was lost during old-worker cleanup")
			}

			select {
			case info := <-replacementInfo:
				require.Equal(t, id, info.ID)
				require.Len(t, info.Addrs, 1)
				require.Equal(t, latestAddress.String(), info.Addrs[0].String())
			case <-time.After(5 * time.Second):
				t.Fatal("replacement did not report its selected address")
			}

			require.Equal(t, int32(2), attempts.Load())
			require.Equal(t, int32(1), witnessAttempts.Load())
			require.True(t, s.isRegisteredConnection(replacement))
			require.Equal(t, int64(1), s.connectionCounts.GetOutboundConnCount())
			require.Nil(t, s.dialQueue.PopDialTask(), "duplicate reconnect remained queued")
		})
	}
}

/* FINISH ALPHA CHANGE 2780 -- regress same peer reconnect during old worker cleanup and reauthorization -- */

/* FINISH ALPHA CHANGE 2769 -- regress dial recovery independent slot ownership and joined shutdown -- */
