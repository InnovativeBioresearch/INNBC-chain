/* START ALPHA CHANGE 2907 -- bound expensive block serving globally and by authenticated peer -- */

package syncer

import (
	"context"
	"sync"

	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	maxActiveBlockServing        = 8
	maxActiveBlockServingPerPeer = 2
)

/*
beginBlockServing reserves both allowances together.

The lifecycle lock is held only for admission and accounting.
Reads, serialization, sends and handler joining happen outside it.
*/
func (s *syncPeerService) beginBlockServing(ctx context.Context) (func(), error) {
	id, err := netgrpc.PeerIDFromContext(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	s.lifecycleLock.Lock()

	if s.closed {
		s.lifecycleLock.Unlock()

		return nil, status.Error(codes.Unavailable, "block serving service is closed")
	}

	if err := ctx.Err(); err != nil {
		s.lifecycleLock.Unlock()

		return nil, status.FromContextError(err).Err()
	}

	if s.activeBlockServing >= maxActiveBlockServing ||
		s.blockServingByPeer[id] >= maxActiveBlockServingPerPeer {
		s.lifecycleLock.Unlock()

		return nil, status.Error(codes.ResourceExhausted, "block serving capacity is busy")
	}

	if s.blockServingByPeer == nil {
		s.blockServingByPeer = make(map[peer.ID]int)
	}

	s.activeBlockServing++
	s.blockServingByPeer[id]++
	s.lifecycleLock.Unlock()

	/*
		Each cleanup owns its own Once. Repeated cleanup cannot release
		a later reservation belonging to the same peer.
	*/
	var releaseOnce sync.Once

	return func() {
		releaseOnce.Do(func() {
			s.lifecycleLock.Lock()
			defer s.lifecycleLock.Unlock()

			s.activeBlockServing--

			remaining := s.blockServingByPeer[id] - 1
			if remaining == 0 {
				delete(s.blockServingByPeer, id)
			} else {
				s.blockServingByPeer[id] = remaining
			}
		})
	}, nil
}

/* FINISH ALPHA CHANGE 2907 -- bound expensive block serving globally and by authenticated peer -- */
