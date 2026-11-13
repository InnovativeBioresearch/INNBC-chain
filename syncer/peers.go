package syncer

import (
	"math/big"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

type NoForkPeer struct {
	// identifier
	ID peer.ID
	// peer's latest block number
	Number uint64
	/* START ALPHA CHANGE 1662 -- track peer full-block history availability -- */
	// peer's earliest continuously servable block
	HistoryFrom uint64
	/* FINISH ALPHA CHANGE 1662 -- track peer full-block history availability -- */
	// peer's distance
	Distance *big.Int
}

/* START ALPHA CHANGE 1663 -- determine whether peer can serve requested block -- */
func (p *NoForkPeer) CanServe(blockNumber uint64) bool {
	return p.HistoryFrom <= blockNumber && blockNumber <= p.Number
}

/* FINISH ALPHA CHANGE 1663 -- determine whether peer can serve requested block -- */

func (p *NoForkPeer) IsBetter(t *NoForkPeer) bool {
	if p.Number != t.Number {
		return p.Number > t.Number
	}

	return p.Distance.Cmp(t.Distance) < 0
}

type PeerMap struct {
	sync.Map
}

func NewPeerMap(peers []*NoForkPeer) *PeerMap {
	peerMap := new(PeerMap)

	peerMap.Put(peers...)

	return peerMap
}

func (m *PeerMap) Put(peers ...*NoForkPeer) {
	for _, peer := range peers {
		m.Store(peer.ID.String(), peer)
	}
}

// Remove removes a peer from heap if it exists
func (m *PeerMap) Remove(peerID peer.ID) {
	m.Delete(peerID.String())
}

// BestPeer returns the top of heap
func (m *PeerMap) BestPeer(skipMap map[peer.ID]bool) *NoForkPeer {
	return m.bestPeer(skipMap, nil)
}

/* START ALPHA CHANGE 1664 -- select best peer among those containing requested block -- */
func (m *PeerMap) BestPeerForBlock(
	blockNumber uint64,
	skipMap map[peer.ID]bool,
) *NoForkPeer {
	return m.bestPeer(skipMap, &blockNumber)
}

func (m *PeerMap) bestPeer(
	skipMap map[peer.ID]bool,
	requestedBlock *uint64,
) *NoForkPeer {
	var bestPeer *NoForkPeer

	m.Range(func(key, value interface{}) bool {
		peer, _ := value.(*NoForkPeer)

		if skipMap != nil && skipMap[peer.ID] {
			return true
		}

		if requestedBlock != nil && !peer.CanServe(*requestedBlock) {
			return true
		}

		if bestPeer == nil || peer.IsBetter(bestPeer) {
			bestPeer = peer
		}

		return true
	})

	return bestPeer
}

/* FINISH ALPHA CHANGE 1664 -- select best peer among those containing requested block -- */
