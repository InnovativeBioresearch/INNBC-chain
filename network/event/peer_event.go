package event

import "github.com/libp2p/go-libp2p/core/peer"

type PeerEventType uint

const (
	PeerConnected        PeerEventType = iota // Emitted when a peer connected
	PeerFailedToConnect                       // Emitted when a peer failed to connect
	PeerDisconnected                          // Emitted when a peer disconnected from node
	PeerDialCompleted                         // Emitted when a peer completed dial
	PeerAddedToDialQueue                      // Emitted when a peer is added to dial queue
	/* START ALPHA CHANGE 2876 -- request status cleanup independently of application registration -- */
	PeerSessionEnded
	/* FINISH ALPHA CHANGE 2876 -- request status cleanup independently of application registration -- */
)

var peerEventToName = map[PeerEventType]string{
	PeerConnected:        "PeerConnected",
	PeerFailedToConnect:  "PeerFailedToConnect",
	PeerDisconnected:     "PeerDisconnected",
	PeerDialCompleted:    "PeerDialCompleted",
	PeerAddedToDialQueue: "PeerAddedToDialQueue",
	/* START ALPHA CHANGE 2877 -- name the eligible session cleanup notification -- */
	PeerSessionEnded: "PeerSessionEnded",
	/* FINISH ALPHA CHANGE 2877 -- name the eligible session cleanup notification -- */
}

type PeerEvent struct {
	// PeerID is the id of the peer that triggered
	// the event
	PeerID peer.ID

	// Type is the type of the event
	Type PeerEventType

	/* START ALPHA CHANGE 2750 -- distinguish stale ownership events from current membership -- */
	Current func() bool
	/* FINISH ALPHA CHANGE 2750 -- distinguish stale ownership events from current membership -- */
}

/* START ALPHA CHANGE 2750 -- let membership consumers reject delayed cleanup events -- */
func (e *PeerEvent) IsCurrent() bool {
	return e != nil && (e.Current == nil || e.Current())
}

/* FINISH ALPHA CHANGE 2750 -- let membership consumers reject delayed cleanup events -- */

func (s PeerEventType) String() string {
	name, ok := peerEventToName[s]
	if !ok {
		return "unknown"
	}

	return name
}
