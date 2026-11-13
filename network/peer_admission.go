/* START ALPHA CHANGE 2742 -- share startup membership and connection ownership across P2P admission -- */
package network

import (
	"context"
	"errors"
	"sync"

	"github.com/armon/go-metrics"
	peerEvent "github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/libp2p/go-libp2p/core/control"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	rawGrpc "google.golang.org/grpc"
)

var errPeerAdmission = errors.New("peer admission denied")

/* START ALPHA CHANGE 2761 -- index outbound ownership and distinguish cancellation from removal -- */
type connectionAdmission struct {
	conn    libnetwork.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	revoked bool
	removed chan struct{}
	/* START ALPHA CHANGE 2860 -- bind pending capacity to the exact connection owner -- */
	pendingSlot bool
	/* FINISH ALPHA CHANGE 2860 -- bind pending capacity to the exact connection owner -- */
	/* START ALPHA CHANGE 2782 -- bind connection cleanup to its originating session -- */
	session *PeerSession
	/* FINISH ALPHA CHANGE 2782 -- bind connection cleanup to its originating session -- */
}

type peerAdmission struct {
	lock        sync.RWMutex
	restricted  bool
	started     bool
	initialized bool
	closed      bool
	allowed     map[peer.ID]struct{}
	connections map[string]*connectionAdmission
	outbound    map[peer.ID]map[string]*connectionAdmission
	/* START ALPHA CHANGE 2783 -- index the current eligible session per peer -- */
	sessions map[peer.ID]*PeerSession
	/* FINISH ALPHA CHANGE 2783 -- index the current eligible session per peer -- */
}

/* FINISH ALPHA CHANGE 2761 -- index outbound ownership and distinguish cancellation from removal -- */

func peerDirectoryRestricted(config *Config) bool {
	return config != nil &&
		config.PeerDirectoryEnabled &&
		config.Chain != nil &&
		config.Chain.Params != nil &&
		config.Chain.Params.Engine != nil &&
		config.Chain.Params.Engine["innbcbft"] != nil
}

/* START ALPHA CHANGE 2762 -- initialize the outbound ownership index -- */
func newPeerAdmission(restricted bool) *peerAdmission {
	return &peerAdmission{
		restricted:  restricted,
		allowed:     make(map[peer.ID]struct{}),
		connections: make(map[string]*connectionAdmission),
		outbound:    make(map[peer.ID]map[string]*connectionAdmission),
		/* START ALPHA CHANGE 2784 -- initialize peer session ownership -- */
		sessions: make(map[peer.ID]*PeerSession),
		/* FINISH ALPHA CHANGE 2784 -- initialize peer session ownership -- */
	}
}

/* FINISH ALPHA CHANGE 2762 -- initialize the outbound ownership index -- */

func (p *peerAdmission) allowsLocked(id peer.ID) bool {
	if p.closed || !p.started {
		return false
	}

	if !p.initialized {
		return !p.restricted
	}

	_, allowed := p.allowed[id]

	return allowed
}

func (p *peerAdmission) allows(id peer.ID) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.allowsLocked(id)
}

func (p *peerAdmission) accepts() bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return !p.closed &&
		p.started &&
		(!p.restricted || p.initialized)
}

type admissionGater struct {
	policy *peerAdmission
}

func (g admissionGater) InterceptPeerDial(id peer.ID) bool {
	return g.policy.allows(id)
}

func (g admissionGater) InterceptAddrDial(id peer.ID, _ multiaddr.Multiaddr) bool {
	return g.policy.allows(id)
}

func (g admissionGater) InterceptAccept(libnetwork.ConnMultiaddrs) bool {
	return g.policy.accepts()
}

func (g admissionGater) InterceptSecured(
	_ libnetwork.Direction,
	id peer.ID,
	_ libnetwork.ConnMultiaddrs,
) bool {
	return g.policy.allows(id)
}

func (g admissionGater) InterceptUpgraded(
	conn libnetwork.Conn,
) (bool, control.DisconnectReason) {
	return g.policy.allows(conn.RemotePeer()), 0
}

func (s *Server) startPeerAdmission() bool {
	if s.admission == nil {
		return true
	}

	s.admission.lock.Lock()
	defer s.admission.lock.Unlock()

	if s.admission.closed {
		return false
	}

	s.admission.started = true

	return true
}

func (s *Server) stopPeerAdmission() {
	if s.admission == nil {
		return
	}

	p := s.admission
	p.lock.Lock()
	p.closed = true

	/* START ALPHA CHANGE 2785 -- invalidate sessions and release their connection metadata on shutdown -- */
	for _, session := range p.sessions {
		clear(session.connections)
	}
	clear(p.sessions)
	/* FINISH ALPHA CHANGE 2785 -- invalidate sessions and release their connection metadata on shutdown -- */

	cancels := make([]context.CancelFunc, 0, len(p.connections))
	for _, ownership := range p.connections {
		ownership.revoked = true
		cancels = append(cancels, ownership.cancel)
	}
	p.lock.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Server) replaceAllowedPeers(peerIDs []peer.ID) bool {
	allowed := make(map[peer.ID]struct{}, len(peerIDs))
	for _, id := range peerIDs {
		allowed[id] = struct{}{}
	}

	/* Preserve explicitly constructed test servers without a listening host. */
	if s.admission == nil {
		s.allowedPeersLock.Lock()
		s.allowedPeers = allowed
		s.peerAllowlistReady = true
		s.allowedPeersLock.Unlock()

		return true
	}

	p := s.admission
	p.lock.Lock()
	if p.closed {
		p.lock.Unlock()

		return false
	}

	p.allowed = allowed
	p.initialized = true

	/* START ALPHA CHANGE 2786 -- retire revoked sessions without invalidating unchanged membership -- */
	for id, session := range p.sessions {
		if _, remainsAllowed := allowed[id]; !remainsAllowed {
			clear(session.connections)
			delete(p.sessions, id)
		}
	}
	/* FINISH ALPHA CHANGE 2786 -- retire revoked sessions without invalidating unchanged membership -- */

	revoked := make([]*connectionAdmission, 0)
	for _, ownership := range p.connections {
		if _, ok := allowed[ownership.conn.RemotePeer()]; !ok {
			ownership.revoked = true
			revoked = append(revoked, ownership)
		}
	}
	p.lock.Unlock()

	/* Cancel and close captured connections, never a later peer-wide replacement. */
	for _, ownership := range revoked {
		ownership.cancel()
		_ = ownership.conn.Close()
	}

	/* Also cover transports registered by libp2p before their notification runs. */
	if s.host != nil {
		for _, conn := range s.host.Network().Conns() {
			p.lock.Lock()
			ownership := p.connections[conn.ID()]
			closeConnection := !p.allowsLocked(conn.RemotePeer()) ||
				(ownership != nil && ownership.revoked)
			if closeConnection && ownership != nil {
				ownership.revoked = true
			}
			p.lock.Unlock()

			if closeConnection {
				if ownership != nil {
					ownership.cancel()
				}
				_ = conn.Close()
			}
		}
	}

	return true
}

func (s *Server) ConnectionContext(
	conn libnetwork.Conn,
) (context.Context, error) {
	if s.admission == nil || conn == nil {
		return nil, errPeerAdmission
	}

	p := s.admission
	p.lock.Lock()
	defer p.lock.Unlock()

	if !p.allowsLocked(conn.RemotePeer()) || conn.IsClosed() {
		return nil, errPeerAdmission
	}

	if ownership := p.connections[conn.ID()]; ownership != nil {
		if ownership.revoked {
			return nil, errPeerAdmission
		}

		return ownership.ctx, nil
	}

	/* START ALPHA CHANGE 2763 -- register outbound transport removal ownership -- */
	/* START ALPHA CHANGE 2787 -- preserve live sessions and create distinct replacement ownership -- */
	id := conn.RemotePeer()
	session := p.currentSessionLocked(id)
	if session == nil {
		session = &PeerSession{
			peerID:      id,
			connections: make(map[string]*connectionAdmission),
		}
		p.sessions[id] = session
	}

	ctx, cancel := context.WithCancel(context.Background())
	ownership := &connectionAdmission{
		conn:    conn,
		ctx:     ctx,
		cancel:  cancel,
		removed: make(chan struct{}),
		session: session,
	}
	p.connections[conn.ID()] = ownership
	session.connections[conn.ID()] = ownership
	/* FINISH ALPHA CHANGE 2787 -- preserve live sessions and create distinct replacement ownership -- */

	if conn.Stat().Direction == libnetwork.DirOutbound {
		id := conn.RemotePeer()
		connections := p.outbound[id]
		if connections == nil {
			connections = make(map[string]*connectionAdmission)
			p.outbound[id] = connections
		}

		connections[conn.ID()] = ownership
	}
	/* FINISH ALPHA CHANGE 2763 -- register outbound transport removal ownership -- */

	return ctx, nil
}

/* START ALPHA CHANGE 2861 -- atomically transfer pending capacity into registered peer accounting -- */
func (s *Server) AddPeerConnection(conn libnetwork.Conn) error {
	if s.admission == nil || conn == nil {
		return errPeerAdmission
	}

	id := conn.RemotePeer()
	direction := conn.Stat().Direction
	info := s.host.Peerstore().PeerInfo(id)

	/* Lock order: admission, then peers. No event or network work under either. */
	p := s.admission
	p.lock.Lock()
	ownership := p.connections[conn.ID()]
	if ownership == nil ||
		ownership.revoked ||
		!p.allowsLocked(id) ||
		conn.IsClosed() {
		p.lock.Unlock()

		return errPeerAdmission
	}

	s.peersLock.Lock()
	connectionInfo := s.peers[id]

	/*
		Live handshakes already own a pending reservation and do not recheck
		capacity here. Direct registration helpers without a reservation
		must establish capacity before adding a new peer-direction entry.
	*/
	needsActiveSlot := connectionInfo == nil || !connectionInfo.connDirections[direction]
	if !ownership.pendingSlot &&
		needsActiveSlot &&
		!s.connectionCounts.HasFreeConnectionSlot(direction) {
		s.peersLock.Unlock()
		p.lock.Unlock()

		return errPeerAdmission
	}

	if connectionInfo == nil {
		connectionInfo = &PeerConnInfo{
			Info:            info,
			connDirections:  make(map[libnetwork.Direction]bool),
			protocolStreams: make(map[string]*rawGrpc.ClientConn),
			connectionRefs:  make(map[string]libnetwork.Direction),
		}
		s.peers[id] = connectionInfo
	}
	if connectionInfo.connectionRefs == nil {
		connectionInfo.connectionRefs = make(map[string]libnetwork.Direction)
	}

	if _, exists := connectionInfo.connectionRefs[conn.ID()]; exists {
		s.releasePendingSlotLocked(ownership)
		s.peersLock.Unlock()
		p.lock.Unlock()

		return nil
	}

	connectionInfo.connectionRefs[conn.ID()] = direction
	if !connectionInfo.connDirections[direction] {
		connectionInfo.connDirections[direction] = true
		s.connectionCounts.UpdateConnCountByDirection(1, direction)
		s.updateConnCountMetrics(direction)
	}

	/*
		Reservation, registration, and disconnect use these same locks.
		Another admission cannot observe a released pending slot before
		the corresponding active entry has been established.
	*/
	s.releasePendingSlotLocked(ownership)

	metrics.SetGauge([]string{networkMetrics, "peers"}, float32(len(s.peers)))
	s.peersLock.Unlock()
	p.lock.Unlock()

	s.emitOwnedPeerEvent(id, peerEvent.PeerConnected, func() bool {
		return s.isRegisteredConnection(conn)
	})

	return nil
}

/* FINISH ALPHA CHANGE 2861 -- atomically transfer pending capacity into registered peer accounting -- */

func (s *Server) isRegisteredConnection(conn libnetwork.Conn) bool {
	if s.admission == nil {
		return false
	}

	p := s.admission
	p.lock.RLock()
	defer p.lock.RUnlock()

	ownership := p.connections[conn.ID()]
	if ownership == nil || ownership.revoked || !p.allowsLocked(conn.RemotePeer()) {
		return false
	}

	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	info := s.peers[conn.RemotePeer()]
	if info == nil {
		return false
	}

	_, registered := info.connectionRefs[conn.ID()]

	return registered
}

func (s *Server) DisconnectConnection(conn libnetwork.Conn, reason string) {
	s.logger.Debug("closing owned connection", "peer", conn.RemotePeer(), "reason", reason)

	_ = conn.Close()
}

func (s *Server) removePeerConnection(conn libnetwork.Conn) {
	if s.admission == nil {
		return
	}

	id := conn.RemotePeer()
	p := s.admission
	p.lock.Lock()
	/* START ALPHA CHANGE 2764 -- remove only the disconnected outbound ownership entry -- */
	ownership := p.connections[conn.ID()]
	delete(p.connections, conn.ID())

	/* START ALPHA CHANGE 2788 -- prevent stale connection cleanup from invalidating replacement sessions -- */
	if ownership != nil && ownership.session != nil {
		session := ownership.session
		delete(session.connections, conn.ID())

		if p.sessions[id] == session && !session.hasLiveConnectionLocked() {
			clear(session.connections)
			delete(p.sessions, id)
		}
	}
	/* FINISH ALPHA CHANGE 2788 -- prevent stale connection cleanup from invalidating replacement sessions -- */

	if connections := p.outbound[id]; connections != nil {
		delete(connections, conn.ID())
		if len(connections) == 0 {
			delete(p.outbound, id)
		}
	}
	/* FINISH ALPHA CHANGE 2764 -- remove only the disconnected outbound ownership entry -- */

	/* START ALPHA CHANGE 2878 -- detect ended eligibility even after session metadata was retired -- */
	/*
		Only removal of a tracked owner requests cleanup. A repeated removal
		has no owner and must not produce another session notification.

		Revocation or a prior session lookup may already have retired the
		session entry, so notification does not depend on deleting it here.
	*/
	sessionEnded := ownership != nil &&
		(!p.allowsLocked(id) || !p.sessions[id].hasLiveConnectionLocked())
	/* FINISH ALPHA CHANGE 2878 -- detect ended eligibility even after session metadata was retired -- */

	s.peersLock.Lock()
	/* START ALPHA CHANGE 2862 -- release disconnected pending capacity before later worker cleanup -- */
	s.releasePendingSlotLocked(ownership)
	/* FINISH ALPHA CHANGE 2862 -- release disconnected pending capacity before later worker cleanup -- */
	info := s.peers[id]
	registered := false
	var clients map[string]*rawGrpc.ClientConn

	if info != nil {
		direction, exists := info.connectionRefs[conn.ID()]
		if exists {
			registered = true
			delete(info.connectionRefs, conn.ID())

			directionStillPresent := false
			for _, remainingDirection := range info.connectionRefs {
				if remainingDirection == direction {
					directionStillPresent = true

					break
				}
			}
			if !directionStillPresent && info.connDirections[direction] {
				delete(info.connDirections, direction)
				s.connectionCounts.UpdateConnCountByDirection(-1, direction)
				s.updateConnCountMetrics(direction)
			}

			if len(info.connectionRefs) == 0 {
				delete(s.peers, id)
				clients = info.protocolStreams
				info.protocolStreams = make(map[string]*rawGrpc.ClientConn)
			}
		}
	}
	metrics.SetGauge([]string{networkMetrics, "peers"}, float32(len(s.peers)))
	s.peersLock.Unlock()
	p.lock.Unlock()

	/* START ALPHA CHANGE 2765 -- signal transport removal after peer and counter cleanup -- */
	if ownership != nil {
		ownership.cancel()
		close(ownership.removed)
	}
	for _, client := range clients {
		if client != nil {
			_ = client.Close()
		}
	}

	/* START ALPHA CHANGE 2879 -- notify ended sessions outside locks without an application peer predicate -- */
	if sessionEnded {
		/*
			This requests a current eligibility check. A replacement may
			already exist by delivery time; consumers must preserve it.
		*/
		s.emitOwnedPeerEvent(id, peerEvent.PeerSessionEnded, nil)
	}
	/* FINISH ALPHA CHANGE 2879 -- notify ended sessions outside locks without an application peer predicate -- */

	if registered {
		/* Membership consumers check Current; dial slots have their own owner. */
		s.emitOwnedPeerEvent(id, peerEvent.PeerDisconnected, func() bool {
			return !s.hasPeer(id)
		})
	}
	/* FINISH ALPHA CHANGE 2765 -- signal transport removal after peer and counter cleanup -- */
}

func (s *Server) emitOwnedPeerEvent(
	id peer.ID,
	kind peerEvent.PeerEventType,
	current func() bool,
) {
	if err := s.emitterPeerEvent.Emit(peerEvent.PeerEvent{
		PeerID:  id,
		Type:    kind,
		Current: current,
	}); err != nil {
		s.logger.Debug("failed to emit owned peer event", "peer", id, "err", err)
	}
}

func (s *Server) EmitConnectionEvent(
	conn libnetwork.Conn,
	kind peerEvent.PeerEventType,
) {
	var current func() bool
	if kind == peerEvent.PeerFailedToConnect {
		current = func() bool { return !s.hasPeer(conn.RemotePeer()) }
	}

	s.emitOwnedPeerEvent(conn.RemotePeer(), kind, current)
}

func (s *Server) gossipAllowed(forwarder, author peer.ID) bool {
	if s.admission == nil {
		return s.IsPeerAllowed(forwarder) && s.IsPeerAllowed(author)
	}

	p := s.admission
	p.lock.RLock()
	defer p.lock.RUnlock()

	// START ALPHA CHANGE 3799 -- admit local gossip independently of directory membership --
	if s.host != nil && forwarder == s.host.ID() && author == forwarder {
		return p.started && !p.closed
	}
	// FINISH ALPHA CHANGE 3799 -- admit local gossip independently of directory membership --

	return p.allowsLocked(forwarder) && p.allowsLocked(author)
}

/* FINISH ALPHA CHANGE 2742 -- share startup membership and connection ownership across P2P admission -- */
