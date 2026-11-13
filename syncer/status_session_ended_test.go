/* START ALPHA CHANGE 2883 -- regress ended sessions through the real network event and sync cleanup pipeline -- */
package syncer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	chainnetwork "github.com/innovativebioresearch/innbc-chain/network"
	"github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/innovativebioresearch/innbc-chain/network/event"
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	netproto "github.com/innovativebioresearch/innbc-chain/network/proto"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/stretchr/testify/require"
)

const sessionEndPipelineWatchdog = 20 * time.Second

func waitSessionEndPipeline(
	t *testing.T,
	signal <-chan struct{},
	message string,
) {
	t.Helper()

	timer := time.NewTimer(sessionEndPipelineWatchdog)
	defer timer.Stop()

	select {
	case <-signal:
	case <-timer.C:
		t.Fatal(message)
	}
}

func runSessionEndCleanup(t *testing.T, message string, cleanup func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		cleanup()
	}()

	waitSessionEndPipeline(t, done, message)
}

type sessionEndHelloCall struct {
	reply  chan bool
	exited chan struct{}
}

type sessionEndIdentityService struct {
	netproto.UnimplementedIdentityServer
	calls chan *sessionEndHelloCall
}

func (service *sessionEndIdentityService) Hello(
	ctx context.Context,
	req *netproto.Status,
) (*netproto.Status, error) {
	call := &sessionEndHelloCall{
		reply:  make(chan bool, 1),
		exited: make(chan struct{}),
	}
	defer close(call.exited)

	select {
	case service.calls <- call:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case reject := <-call.reply:
		chainID := req.Chain
		if reject {
			chainID++
		}

		/*
			A matching response is deliberately temporary: successful
			handshake completion must not register this application peer.
		*/
		return &netproto.Status{
			Metadata:      req.Metadata,
			Chain:         chainID,
			TemporaryDial: true,
		}, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type sessionEndPipelineNetwork struct {
	*chainnetwork.Server

	ctx       context.Context
	ready     chan struct{}
	readyOnce sync.Once
	bridges   sync.WaitGroup

	arrived   chan *event.PeerEvent
	completed chan struct{}
	checked   chan bool

	release     chan struct{}
	releaseOnce sync.Once
}

func (n *sessionEndPipelineNetwork) SubscribeCh(
	ctx context.Context,
) (<-chan *event.PeerEvent, error) {
	source, err := n.Server.SubscribeCh(ctx)
	if err != nil {
		return nil, err
	}

	output := make(chan *event.PeerEvent)
	n.bridges.Add(1)

	go func() {
		defer n.bridges.Done()
		defer close(output)

		for {
			select {
			case <-ctx.Done():
				return
			case <-n.ctx.Done():
				return

			case e, open := <-source:
				if !open {
					return
				}

				if e.Type == event.PeerSessionEnded {
					/*
						This observation proves production cleanup emitted
						the event and the real subscription delivered it.
					*/
					select {
					case n.arrived <- e:
					case <-ctx.Done():
						return
					case <-n.ctx.Done():
						return
					}

					/*
						A controlled gate delays forwarding. Cancellation
						must release it without requiring the gate to open.
					*/
					select {
					case <-n.release:
					case <-ctx.Done():
						return
					case <-n.ctx.Done():
						return
					}
				}

				if e.Type == event.PeerDialCompleted {
					select {
					case n.completed <- struct{}{}:
					case <-ctx.Done():
						return
					case <-n.ctx.Done():
						return
					}
				}

				select {
				case output <- e:
				case <-ctx.Done():
					return
				case <-n.ctx.Done():
					return
				}
			}
		}
	}()

	/*
		SubscribeCh has installed the real subscription before readiness
		is published. No connection is created before the test sees this.
	*/
	n.readyOnce.Do(func() { close(n.ready) })

	return output, nil
}

func (n *sessionEndPipelineNetwork) WithNoPeerSession(
	id peer.ID,
	remove func(),
) bool {
	accepted := n.Server.WithNoPeerSession(id, remove)

	/*
		Observation occurs after the production atomic helper returns.
		The callback itself remains only the production peer-map update.
	*/
	select {
	case n.checked <- accepted:
	case <-n.ctx.Done():
	}

	return accepted
}

func (n *sessionEndPipelineNetwork) allowDelivery() {
	n.releaseOnce.Do(func() { close(n.release) })
}

type sessionEndPipelineFixture struct {
	server    *chainnetwork.Server
	remote    host.Host
	transport *netgrpc.GrpcStream
	service   *sessionEndIdentityService

	network *sessionEndPipelineNetwork
	client  *syncPeerClient
	syncer  *syncer

	cancel    context.CancelFunc
	closeOnce sync.Once
	calls     []*sessionEndHelloCall
}

func newSessionEndPipelineFixture(
	t *testing.T,
	delayDelivery bool,
) *sessionEndPipelineFixture {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	f := &sessionEndPipelineFixture{cancel: cancel}
	t.Cleanup(func() { f.close(t) })

	server, err := chainnetwork.CreateServer(&chainnetwork.CreateServerParams{
		ConfigCallback: func(c *chainnetwork.Config) {
			c.NoDiscover = true
			c.PeerDirectoryEnabled = false
			c.Addr.Port = 0
			c.DataDir = t.TempDir()
		},
		ServerCallback: func(s *chainnetwork.Server) {
			f.server = s
		},
	})
	require.NoError(t, err)
	f.server = server

	remote, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	f.remote = remote

	f.server.SetAllowedPeers([]peer.ID{remote.ID()})

	f.service = &sessionEndIdentityService{
		calls: make(chan *sessionEndHelloCall, 8),
	}
	f.transport = netgrpc.NewGrpcStream()
	netproto.RegisterIdentityServer(f.transport.GrpcServer(), f.service)
	f.transport.Serve()
	remote.SetStreamHandler(
		protocol.ID(common.IdentityProto),
		f.transport.Handler(),
	)

	f.network = &sessionEndPipelineNetwork{
		Server:    server,
		ctx:       ctx,
		ready:     make(chan struct{}),
		arrived:   make(chan *event.PeerEvent, 8),
		completed: make(chan struct{}, 8),
		checked:   make(chan bool, 8),
		release:   make(chan struct{}),
	}
	if !delayDelivery {
		f.network.allowDelivery()
	}

	f.client = NewSyncPeerClient(
		hclog.NewNullLogger(),
		f.network,
		nil,
	).(*syncPeerClient)

	f.syncer = &syncer{
		logger:         hclog.NewNullLogger(),
		network:        f.network,
		syncPeerClient: f.client,
		peerMap:        new(PeerMap),
		closeCh:        make(chan struct{}),
		newStatusCh:    make(chan struct{}, 1),
	}

	/*
		Run the production event workers without starting unrelated
		blockchain subscriptions, status gossip or block-serving services.
	*/
	require.True(t, f.client.startWorker(f.client.startPeerEventProcess))
	require.True(t, f.syncer.startWorker(f.syncer.startPeerConnectionEventProcess))

	waitSessionEndPipeline(
		t,
		f.network.ready,
		"sync client did not install its real network event subscription",
	)

	return f
}

/* START ALPHA CHANGE 2884 -- establish a fresh remote transport before expecting a replacement handshake -- */
func (f *sessionEndPipelineFixture) connect(
	t *testing.T,
) *sessionEndHelloCall {
	t.Helper()

	target := f.server.AddrInfo()
	previousConnections := f.remote.Network().ConnsToPeer(target.ID)

	/*
		Server cleanup does not prove the remote host has discarded its
		old transport. Connect can otherwise return success by reusing it.

		This fixture explicitly requests a new connection. Close any
		remaining remote transports before clearing backoff and dialing.
	*/
	var closeErr error
	runSessionEndCleanup(t, "remote transport cleanup did not finish before reconnect", func() {
		closeErr = f.remote.Network().ClosePeer(target.ID)
	})
	require.NoError(t, closeErr)
	require.Empty(t, f.remote.Network().ConnsToPeer(target.ID),
		"remote host retained a transport before the fresh connection attempt")

	f.remote.Network().(*swarm.Swarm).Backoff().Clear(target.ID)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		sessionEndPipelineWatchdog,
	)
	defer cancel()

	require.NoError(t, f.remote.Connect(ctx, *target))

	connections := f.remote.Network().ConnsToPeer(target.ID)
	require.Len(t, connections, 1,
		"fresh loopback connection did not establish exactly one remote transport")

	connectionID := connections[0].ID()
	for _, previous := range previousConnections {
		require.NotEqual(t, previous.ID(), connectionID,
			"connection attempt reused an old transport")
	}

	timer := time.NewTimer(sessionEndPipelineWatchdog)
	defer timer.Stop()

	select {
	case call := <-f.service.calls:
		f.calls = append(f.calls, call)

		require.NotNil(t, f.server.PeerSession(f.remote.ID()),
			"identity RPC started without an eligible session")
		require.Empty(t, f.server.Peers(),
			"controlled identity handshake completed application registration")

		return call

	case <-timer.C:
		t.Fatalf(
			"remote identity RPC did not reach its controlled handshake barrier: "+
				"transport=%q remote_connectedness=%v allowed=%t eligible_session=%t registered_peers=%d",
			connectionID,
			f.remote.Network().Connectedness(target.ID),
			f.server.IsPeerAllowed(f.remote.ID()),
			f.server.PeerSession(f.remote.ID()) != nil,
			len(f.server.Peers()),
		)

		return nil
	}
}

/* FINISH ALPHA CHANGE 2884 -- establish a fresh remote transport before expecting a replacement handshake -- */

func (f *sessionEndPipelineFixture) publish(
	t *testing.T,
	number uint64,
) *NoForkPeer {
	t.Helper()

	id := f.remote.ID()
	token := f.server.PeerSession(id)
	require.NotNil(t, token)

	status := statusOwnershipValue(id, number)
	f.syncer.publishPeerStatus(&PeerStatusUpdate{
		Status:  status,
		Session: token,
	})

	value, present := f.syncer.peerMap.Load(id.String())
	require.True(t, present, "status publication did not run")
	require.Same(t, status, value)

	return status
}

func (f *sessionEndPipelineFixture) awaitEnded(
	t *testing.T,
) *event.PeerEvent {
	t.Helper()

	timer := time.NewTimer(sessionEndPipelineWatchdog)
	defer timer.Stop()

	select {
	case e := <-f.network.arrived:
		require.Equal(t, event.PeerSessionEnded, e.Type)
		require.Equal(t, f.remote.ID(), e.PeerID)
		require.Nil(t, e.Current)
		require.True(t, e.IsCurrent())

		return e

	case <-timer.C:
		t.Fatal("production connection cleanup did not deliver PeerSessionEnded")

		return nil
	}
}

func (f *sessionEndPipelineFixture) awaitRemovalDecision(
	t *testing.T,
) bool {
	t.Helper()

	timer := time.NewTimer(sessionEndPipelineWatchdog)
	defer timer.Stop()

	select {
	case accepted := <-f.network.checked:
		return accepted

	case <-timer.C:
		t.Fatal("session event did not reach the syncer's atomic removal helper")

		return false
	}
}

func (f *sessionEndPipelineFixture) close(t *testing.T) {
	t.Helper()

	f.closeOnce.Do(func() {
		/*
			Release controlled gates before joining resources. Any RPC
			still waiting at Hello can finish or observe cancellation.
		*/
		if f.network != nil {
			f.network.allowDelivery()
		}
		for _, call := range f.calls {
			select {
			case call.reply <- true:
			default:
			}
		}
		f.cancel()

		if f.syncer != nil {
			f.syncer.peerStateLock.Lock()
			close(f.syncer.closeCh)
			f.syncer.peerStateLock.Unlock()
		}

		if f.client != nil {
			runSessionEndCleanup(t, "sync client shutdown did not join its event worker", func() {
				f.client.Close()
				f.client.Close()
			})
		}
		if f.network != nil {
			runSessionEndCleanup(t, "test subscription bridge did not terminate", func() {
				f.network.bridges.Wait()
			})
		}
		if f.syncer != nil {
			runSessionEndCleanup(t, "sync event consumer did not terminate", func() {
				f.syncer.closeWG.Wait()
			})
		}

		if f.server != nil {
			var err error
			runSessionEndCleanup(t, "production network shutdown did not finish", func() {
				err = f.server.Close()
			})
			require.NoError(t, err)
		}
		if f.transport != nil {
			runSessionEndCleanup(t, "remote identity service did not join its handlers", func() {
				f.transport.Stop()
			})
		}
		if f.remote != nil {
			var err error
			runSessionEndCleanup(t, "remote loopback host did not close", func() {
				err = f.remote.Close()
			})
			require.NoError(t, err)
		}
		for _, call := range f.calls {
			waitSessionEndPipeline(t, call.exited, "controlled identity handler did not exit")
		}
	})
}

func TestPeerStatusSessionEnded_UnregisteredCleanupPipeline(t *testing.T) {
	for _, scenario := range []string{
		"disconnect during handshake",
		"failed handshake",
		"successful temporary handshake",
		"revocation before registration",
	} {
		t.Run(scenario, func(t *testing.T) {
			f := newSessionEndPipelineFixture(t, false)
			call := f.connect(t)
			f.publish(t, 100)

			switch scenario {
			case "disconnect during handshake":
				require.NoError(t, f.remote.Network().ClosePeer(f.server.AddrInfo().ID))

			case "failed handshake":
				call.reply <- true

			case "successful temporary handshake":
				call.reply <- false
				waitSessionEndPipeline(
					t,
					f.network.completed,
					"temporary handshake did not complete through the production identity worker",
				)
				require.Empty(t, f.server.Peers())
				require.NotNil(t, f.server.PeerSession(f.remote.ID()))
				require.NoError(t, f.remote.Network().ClosePeer(f.server.AddrInfo().ID))

			case "revocation before registration":
				f.server.SetAllowedPeers(nil)
				require.Nil(t, f.server.PeerSession(f.remote.ID()))
			}

			f.awaitEnded(t)
			require.True(t, f.awaitRemovalDecision(t))

			_, present := f.syncer.peerMap.Load(f.remote.ID().String())
			require.False(t, present,
				"status survived the last eligible unregistered connection")
		})
	}
}

func TestPeerStatusSessionEnded_DelayedEventPreservesReplacementPipeline(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		name := "disconnect and reconnect"
		if revoke {
			name = "revocation and reauthorization"
		}

		t.Run(name, func(t *testing.T) {
			f := newSessionEndPipelineFixture(t, true)
			f.connect(t)
			oldToken := f.server.PeerSession(f.remote.ID())
			f.publish(t, 100)

			if revoke {
				f.server.SetAllowedPeers(nil)
			} else {
				require.NoError(t, f.remote.Network().ClosePeer(f.server.AddrInfo().ID))
			}

			/*
				The old event has reached the real subscription but is
				held before forwarding to the production sync client.
			*/
			f.awaitEnded(t)
			require.Nil(t, f.server.PeerSession(f.remote.ID()))

			if revoke {
				f.server.SetAllowedPeers([]peer.ID{f.remote.ID()})
			}

			replacementCall := f.connect(t)
			newToken := f.server.PeerSession(f.remote.ID())
			require.NotSame(t, oldToken, newToken)

			fresh := f.publish(t, 200)
			require.Empty(t, f.server.Peers(),
				"replacement must still be awaiting application registration")

			f.network.allowDelivery()
			require.False(t, f.awaitRemovalDecision(t),
				"delayed old cleanup removed an eligible replacement")

			value, present := f.syncer.peerMap.Load(f.remote.ID().String())
			require.True(t, present)
			require.Same(t, fresh, value)
			require.Same(t, newToken, f.server.PeerSession(f.remote.ID()))

			/*
				The replacement now fails before registration. Its own
				event must reach the same pipeline and remove its status.
			*/
			replacementCall.reply <- true

			f.awaitEnded(t)
			require.True(t, f.awaitRemovalDecision(t))

			_, present = f.syncer.peerMap.Load(f.remote.ID().String())
			require.False(t, present)
		})
	}
}

func TestPeerStatusSessionEnded_ShutdownCancelsDelayedDelivery(t *testing.T) {
	f := newSessionEndPipelineFixture(t, true)
	f.connect(t)
	f.publish(t, 100)

	require.NoError(t, f.remote.Network().ClosePeer(f.server.AddrInfo().ID))
	f.awaitEnded(t)

	/*
		Keep the forwarding gate closed. Client cancellation itself must
		release subscription delivery and terminate both event workers.
	*/
	runSessionEndCleanup(t, "client shutdown waited for the unopened forwarding gate", func() {
		f.client.Close()
		f.client.Close()
	})
	runSessionEndCleanup(t, "cancelled subscription bridge remained running", func() {
		f.network.bridges.Wait()
	})
	runSessionEndCleanup(t, "sync event consumer remained running after client shutdown", func() {
		f.syncer.closeWG.Wait()
	})
}

/* START ALPHA CHANGE 2885 -- regress fresh connection requests while the remote still has an old transport -- */
func TestPeerStatusSessionEnded_ConnectHelperCreatesFreshTransport(t *testing.T) {
	f := newSessionEndPipelineFixture(t, false)

	firstCall := f.connect(t)
	targetID := f.server.AddrInfo().ID
	firstConnections := f.remote.Network().ConnsToPeer(targetID)
	require.Len(t, firstConnections, 1)
	firstConnectionID := firstConnections[0].ID()

	/*
		Keep the first transport open deliberately. The original helper
		would let Connect reuse it and then wait for a nonexistent Hello.
	*/
	require.False(t, firstConnections[0].IsClosed())

	secondCall := f.connect(t)

	secondConnections := f.remote.Network().ConnsToPeer(targetID)
	require.Len(t, secondConnections, 1)
	require.NotEqual(t, firstConnectionID, secondConnections[0].ID())
	require.NotSame(t, firstCall, secondCall,
		"fresh transport did not produce a distinct identity RPC")
	require.Empty(t, f.server.Peers(),
		"replacement handshake must remain unregistered at its barrier")

	waitSessionEndPipeline(
		t,
		firstCall.exited,
		"closing the old transport did not cancel its controlled identity RPC",
	)

	/*
		The replacement can still complete a legitimate temporary
		handshake after the old RPC has been cancelled.
	*/
	secondCall.reply <- false
	waitSessionEndPipeline(
		t,
		f.network.completed,
		"fresh transport's temporary handshake did not complete",
	)

	require.NotNil(t, f.server.PeerSession(f.remote.ID()))
	require.Empty(t, f.server.Peers())
}

/* FINISH ALPHA CHANGE 2885 -- regress fresh connection requests while the remote still has an old transport -- */

/* FINISH ALPHA CHANGE 2883 -- regress ended sessions through the real network event and sync cleanup pipeline -- */
