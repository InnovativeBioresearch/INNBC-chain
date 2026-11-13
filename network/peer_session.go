/* START ALPHA CHANGE 2781 -- retain local peer sessions through validated gossip and atomic publication -- */
package network

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	googleproto "google.golang.org/protobuf/proto"
)

/*
PeerSession is an opaque local ownership token.
Its connection metadata is accessed only while holding the admission lock.
*/
type PeerSession struct {
	peerID      peer.ID
	connections map[string]*connectionAdmission
}

/* Called only while holding the admission lock. */
func (session *PeerSession) hasLiveConnectionLocked() bool {
	if session == nil {
		return false
	}

	for _, ownership := range session.connections {
		if !ownership.revoked && !ownership.conn.IsClosed() {
			return true
		}
	}

	return false
}

/* Called only while holding the admission write lock. */
func (p *peerAdmission) currentSessionLocked(id peer.ID) *PeerSession {
	session := p.sessions[id]
	if !p.allowsLocked(id) || !session.hasLiveConnectionLocked() {
		if session != nil {
			clear(session.connections)
			delete(p.sessions, id)
		}

		return nil
	}

	return session
}

/* PeerSession captures the current eligible session without creating one. */
func (s *Server) PeerSession(id peer.ID) *PeerSession {
	if s.admission == nil {
		return nil
	}

	p := s.admission
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.currentSessionLocked(id)
}

/*
WithPeerSession validates ownership and executes a short publication callback
under the admission read lock. The callback must only update local status data:
no networking, admission queries, notifications, storage work or closure.
*/
func (s *Server) WithPeerSession(
	id peer.ID,
	session *PeerSession,
	publish func(),
) bool {
	if s.admission == nil || session == nil || publish == nil {
		return false
	}

	p := s.admission
	p.lock.RLock()
	defer p.lock.RUnlock()

	if session.peerID != id ||
		p.sessions[id] != session ||
		!p.allowsLocked(id) ||
		!session.hasLiveConnectionLocked() {
		return false
	}

	publish()

	return true
}

/* START ALPHA CHANGE 2835 -- order disconnected status removal against eligible session registration -- */
/*
WithNoPeerSession executes a short removal callback only when no eligible
session exists. Eligibility includes connections whose identity handshake
has not yet completed application registration.

The callback runs under the admission read lock and must only update local
status data: no networking, admission queries, notifications or closure.
*/
func (s *Server) WithNoPeerSession(id peer.ID, remove func()) bool {
	if s.admission == nil || remove == nil {
		return false
	}

	p := s.admission
	p.lock.RLock()
	defer p.lock.RUnlock()

	if p.allowsLocked(id) && p.sessions[id].hasLiveConnectionLocked() {
		return false
	}

	remove()

	return true
}

/* FINISH ALPHA CHANGE 2835 -- order disconnected status removal against eligible session registration -- */

/* Immutable local metadata after validation; never serialized onto the wire. */
type validatedPeerSession struct {
	author  peer.ID
	session *PeerSession
}

/* Allows controlled subscription fixtures without changing production delivery. */
type topicSubscription interface {
	Next(context.Context) (*pubsub.Message, error)
	Cancel()
}

/*
Capture only during validation. Missing ownership does not prevent ordinary
pubsub forwarding; session-aware subscribers reject it during local delivery.
Existing validator metadata is preserved.
*/
func (s *Server) capturePeerSessionValidation(
	_ context.Context,
	_ peer.ID,
	msg *pubsub.Message,
) bool {
	if msg == nil || msg.Message == nil {
		return false
	}

	if msg.ValidatorData == nil {
		author := msg.GetFrom()
		msg.ValidatorData = &validatedPeerSession{
			author:  author,
			session: s.PeerSession(author),
		}
	}

	return true
}

/*
NewPeerSessionTopic adds ownership capture specifically for consumers that
require a connected status author. NewTopic retains its existing behavior.
*/
func (s *Server) NewPeerSessionTopic(
	protoID string,
	obj googleproto.Message,
) (*Topic, error) {
	topic, err := s.NewTopic(protoID, obj)
	if err != nil {
		return nil, err
	}

	if err := s.ps.RegisterTopicValidator(
		protoID,
		s.capturePeerSessionValidation,
		pubsub.WithValidatorInline(true),
	); err != nil {
		topic.Close()

		return nil, err
	}

	topic.closeValidation = func() {
		if err := s.ps.UnregisterTopicValidator(protoID); err != nil {
			topic.logger.Debug("unable to unregister session validator", "err", err)
		}
	}

	return topic, nil
}

/* FINISH ALPHA CHANGE 2781 -- retain local peer sessions through validated gossip and atomic publication -- */
