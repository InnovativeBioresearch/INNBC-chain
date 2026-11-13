/* START ALPHA CHANGE 2863 -- reserve handshake capacity atomically and release only its original owner -- */
package network

/* START ALPHA CHANGE 2873 -- support per cleanup reservation idempotence -- */
import (
	"sync"

	libnetwork "github.com/libp2p/go-libp2p/core/network"
)

/* FINISH ALPHA CHANGE 2873 -- support per cleanup reservation idempotence -- */

/*
ReserveConnectionSlot reserves pending capacity for an eligible connection.

The returned cleanup is safe after promotion, disconnect, or repeated cleanup.
The caller must invoke it when its handshake finishes.
*/
func (s *Server) ReserveConnectionSlot(conn libnetwork.Conn) (func(), bool) {
	if s.admission == nil || conn == nil {
		return nil, false
	}

	p := s.admission
	p.lock.Lock()
	defer p.lock.Unlock()

	ownership := p.connections[conn.ID()]
	if ownership == nil ||
		ownership.revoked ||
		!p.allowsLocked(conn.RemotePeer()) ||
		conn.IsClosed() {
		return nil, false
	}

	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	/*
		A duplicate notification for an already registered connection does
		not consume another pending slot.
	*/
	if info := s.peers[conn.RemotePeer()]; info != nil {
		if _, registered := info.connectionRefs[conn.ID()]; registered {
			return func() {}, true
		}
	}

	/*
		Do not give a second operation ownership of an existing reservation.
		The identity service suppresses duplicate pending notifications.
	*/
	if ownership.pendingSlot {
		return nil, false
	}

	direction := conn.Stat().Direction
	if !s.connectionCounts.HasFreeConnectionSlot(direction) {
		return nil, false
	}

	ownership.pendingSlot = true
	s.UpdatePendingConnCount(1, direction)

	id := conn.ID()

	/* START ALPHA CHANGE 2874 -- prevent repeated cleanup from releasing a later reservation on the same owner -- */
	var once sync.Once

	return func() {
		once.Do(func() {
			s.releasePendingSlot(id, ownership)
		})
	}, true
	/* FINISH ALPHA CHANGE 2874 -- prevent repeated cleanup from releasing a later reservation on the same owner -- */
}

func (s *Server) releasePendingSlot(id string, ownership *connectionAdmission) {
	p := s.admission
	if p == nil {
		return
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	/*
		Delayed cleanup must never release a different connection owner,
		even if a test or replacement implementation reuses an identifier.
	*/
	if p.connections[id] != ownership {
		return
	}

	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	s.releasePendingSlotLocked(ownership)
}

/* Requires the admission write lock and peer lock, in that order. */
func (s *Server) releasePendingSlotLocked(ownership *connectionAdmission) {
	if ownership == nil || !ownership.pendingSlot {
		return
	}

	ownership.pendingSlot = false
	s.UpdatePendingConnCount(-1, ownership.conn.Stat().Direction)
}

/* FINISH ALPHA CHANGE 2863 -- reserve handshake capacity atomically and release only its original owner -- */
