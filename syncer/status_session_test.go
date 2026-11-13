/* START ALPHA CHANGE 2828 -- regress owned queued startup and final status publication -- */
package syncer

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/go-hclog"
	chainnetwork "github.com/innovativebioresearch/innbc-chain/network"
	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

/* Explicit local ownership model used only by isolated status fixtures. */
type testStatusNetwork struct {
	Network
	lock     sync.Mutex
	sessions map[peer.ID]*chainnetwork.PeerSession
}

func (n *testStatusNetwork) PeerSession(id peer.ID) *chainnetwork.PeerSession {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.sessions == nil {
		n.sessions = make(map[peer.ID]*chainnetwork.PeerSession)
	}

	session := n.sessions[id]
	if session == nil {
		session = &chainnetwork.PeerSession{}
		n.sessions[id] = session
	}

	return session
}

func (n *testStatusNetwork) WithPeerSession(
	id peer.ID,
	session *chainnetwork.PeerSession,
	publish func(),
) bool {
	n.lock.Lock()
	defer n.lock.Unlock()

	if session == nil || n.sessions[id] != session {
		return false
	}

	publish()

	return true
}

/* START ALPHA CHANGE 2839 -- model atomic session absence during fixture status removal -- */
func (n *testStatusNetwork) WithNoPeerSession(id peer.ID, remove func()) bool {
	if remove == nil {
		return false
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	if n.sessions[id] != nil {
		return false
	}

	remove()

	return true
}

/* FINISH ALPHA CHANGE 2839 -- model atomic session absence during fixture status removal -- */

func (n *testStatusNetwork) endSession(id peer.ID) {
	n.lock.Lock()
	delete(n.sessions, id)
	n.lock.Unlock()
}

/*
Existing synchronous fixture calls explicitly acquire test ownership here.
No bare-value publication helper remains in production code.
*/
func (s *syncer) putToPeerMap(status *NoForkPeer, current ...func() bool) {
	if status == nil {
		return
	}

	s.publishPeerStatus(&PeerStatusUpdate{
		Status:  status,
		Session: s.network.PeerSession(status.ID),
	}, current...)
}

func sessionStatusFixture() (*syncer, *testStatusNetwork) {
	n := &testStatusNetwork{}

	return &syncer{
		logger:      hclog.NewNullLogger(),
		network:     n,
		peerMap:     new(PeerMap),
		closeCh:     make(chan struct{}),
		newStatusCh: make(chan struct{}, 1),
	}, n
}

func TestPeerStatusSession_QueuedAndStartupResultsCannotCrossSession(t *testing.T) {
	for _, startup := range []bool{false, true} {
		name := "status channel"
		if startup {
			name = "startup collection"
		}

		t.Run(name, func(t *testing.T) {
			s, n := sessionStatusFixture()
			id := peer.ID("validator")

			old := &PeerStatusUpdate{
				Status:  statusOwnershipValue(id, 100),
				Session: n.PeerSession(id),
			}
			queue := make(chan *PeerStatusUpdate, 1)
			queue <- old
			close(queue)

			n.endSession(id)
			fresh := &PeerStatusUpdate{
				Status:  statusOwnershipValue(id, 200),
				Session: n.PeerSession(id),
			}
			require.NotSame(t, old.Session, fresh.Session)
			s.publishPeerStatus(fresh)

			s.syncPeerClient = &mockSyncPeerClient{
				getPeerStatusUpdateChHandler: func() <-chan *PeerStatusUpdate {
					return queue
				},
				getConnectedPeerStatusUpdatesHandler: func() []*PeerStatusUpdate {
					return []*PeerStatusUpdate{old}
				},
			}

			if startup {
				s.initializePeerMap()
			} else {
				s.startPeerStatusUpdateProcess()
			}

			value, present := s.peerMap.Load(id.String())
			require.True(t, present)
			require.Same(t, fresh.Status, value)
		})
	}
}

type sessionPublicationGateNetwork struct {
	Network
	entered chan struct{}
	release chan struct{}
}

func (n *sessionPublicationGateNetwork) WithPeerSession(
	id peer.ID,
	session *chainnetwork.PeerSession,
	publish func(),
) bool {
	close(n.entered)
	<-n.release

	return n.Network.WithPeerSession(id, session, publish)
}

func TestPeerStatusSession_InvalidationBeforeFinalPublication(t *testing.T) {
	s, owner := sessionStatusFixture()
	id := peer.ID("validator")
	update := &PeerStatusUpdate{
		Status:  statusOwnershipValue(id, 100),
		Session: owner.PeerSession(id),
	}
	gate := &sessionPublicationGateNetwork{
		Network: owner,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	s.network = gate

	finished := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(gate.release) })
		statusOwnershipSignal(t, finished)
	})

	go func() {
		defer close(finished)
		s.publishPeerStatus(update)
	}()
	statusOwnershipSignal(t, gate.entered)

	owner.endSession(id)
	replacement := owner.PeerSession(id)
	require.NotSame(t, update.Session, replacement)

	releaseOnce.Do(func() { close(gate.release) })
	statusOwnershipSignal(t, finished)

	_, present := s.peerMap.Load(id.String())
	require.False(t, present, "ended ownership passed final publication")
}

type sessionQueryService struct {
	proto.UnimplementedSyncPeerServer
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (service *sessionQueryService) GetStatus(
	ctx context.Context,
	_ *emptypb.Empty,
) (*proto.SyncPeerStatus, error) {
	service.calls.Add(1)
	close(service.entered)

	select {
	case <-service.release:
		return &proto.SyncPeerStatus{Number: 100}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type sessionQueryNetwork struct {
	*rpcLifecycleNetwork
	owner *testStatusNetwork
	id    peer.ID
}

func (n *sessionQueryNetwork) PeerSession(id peer.ID) *chainnetwork.PeerSession {
	return n.owner.PeerSession(id)
}

func (n *sessionQueryNetwork) WithPeerSession(
	id peer.ID,
	session *chainnetwork.PeerSession,
	publish func(),
) bool {
	return n.owner.WithPeerSession(id, session, publish)
}

/* START ALPHA CHANGE 2840 -- delegate session absence checks in the query fixture -- */
func (n *sessionQueryNetwork) WithNoPeerSession(id peer.ID, remove func()) bool {
	return n.owner.WithNoPeerSession(id, remove)
}

/* FINISH ALPHA CHANGE 2840 -- delegate session absence checks in the query fixture -- */

func (n *sessionQueryNetwork) Peers() []*chainnetwork.PeerConnInfo {
	return []*chainnetwork.PeerConnInfo{
		{Info: peer.AddrInfo{ID: n.id}},
	}
}

func TestPeerStatusSession_StartupRPCRetainsPreQueryOwnership(t *testing.T) {
	service := &sessionQueryService{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	transport, _ := newOwnershipRPCNetwork(t, service)
	owner := &testStatusNetwork{}
	id := peer.ID("validator")
	oldToken := owner.PeerSession(id)
	n := &sessionQueryNetwork{
		rpcLifecycleNetwork: transport,
		owner:               owner,
		id:                  id,
	}
	client := NewSyncPeerClient(hclog.NewNullLogger(), n, nil)
	t.Cleanup(client.Close)

	s := &syncer{
		logger:         hclog.NewNullLogger(),
		network:        n,
		syncPeerClient: client,
		peerMap:        new(PeerMap),
		closeCh:        make(chan struct{}),
		newStatusCh:    make(chan struct{}, 1),
	}

	finished := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(service.release) })
		statusOwnershipSignal(t, finished)
	})

	go func() {
		defer close(finished)
		s.initializePeerMap()
	}()
	statusOwnershipSignal(t, service.entered)

	owner.endSession(id)
	newToken := owner.PeerSession(id)
	require.NotSame(t, oldToken, newToken)

	fresh := statusOwnershipValue(id, 200)
	s.publishPeerStatus(&PeerStatusUpdate{
		Status:  fresh,
		Session: newToken,
	})

	releaseOnce.Do(func() { close(service.release) })
	statusOwnershipSignal(t, finished)

	value, present := s.peerMap.Load(id.String())
	require.True(t, present)
	require.Same(t, fresh, value)
	require.Equal(t, int32(1), service.calls.Load())
}

func TestPeerStatusSession_MissingOwnershipAndShutdownAreRejected(t *testing.T) {
	s, n := sessionStatusFixture()
	id := peer.ID("validator")
	status := statusOwnershipValue(id, 100)

	s.publishPeerStatus(&PeerStatusUpdate{Status: status})
	_, present := s.peerMap.Load(id.String())
	require.False(t, present)

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
	}

	s.peerStateLock.Lock()
	close(s.closeCh)
	s.peerStateLock.Unlock()

	s.startPeerStatusUpdateProcess()

	_, present = s.peerMap.Load(id.String())
	require.False(t, present)
	require.Empty(t, queue, "shutdown left the delayed envelope queued")
	select {
	case <-s.newStatusCh:
		t.Fatal("rejected shutdown result emitted a status notification")
	default:
	}
}

/* FINISH ALPHA CHANGE 2828 -- regress owned queued startup and final status publication -- */
