/* START ALPHA CHANGE 2859 -- regress discovery eviction preserving pending and runnable reconnect requests -- */
package network

import (
	"context"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/innovativebioresearch/innbc-chain/network/dial"
	"github.com/innovativebioresearch/innbc-chain/network/discovery"
	peerEvent "github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryDialOwnership_RemovalPreservesPendingAndRunnableRequests(t *testing.T) {
	for _, eventType := range []peerEvent.PeerEventType{
		peerEvent.PeerDisconnected,
		peerEvent.PeerFailedToConnect,
	} {
		t.Run(eventType.String(), func(t *testing.T) {
			for _, pending := range []bool{true, false} {
				name := "pending request"
				if !pending {
					name = "runnable request"
				}

				t.Run(name, func(t *testing.T) {
					s := admissionFixture(t, true)
					s.dialQueue = dial.NewDialQueue()
					t.Cleanup(s.dialQueue.Close)
					t.Cleanup(s.stopPeerAdmission)

					id := peer.ID("validator")
					require.True(t, s.startPeerAdmission())
					require.True(t, s.replaceAllowedPeers([]peer.ID{id}))

					routingTable, err := s.newDiscoveryRoutingTable()
					require.NoError(t, err)
					t.Cleanup(func() {
						require.NoError(t, routingTable.Close())
					})

					/*
						Requests are scheduled explicitly. Keep the production
						removal callback while disabling automatic additions.
					*/
					routingTable.PeerAdded = func(peer.ID) {}

					added, err := routingTable.TryAddPeer(id, false, false)
					require.NoError(t, err)
					require.True(t, added)
					require.Contains(t, routingTable.ListPeers(), id)

					discoveryService := discovery.NewDiscoveryService(
						s,
						routingTable,
						s.logger,
					)
					t.Cleanup(discoveryService.Close)

					firstAddress, err := multiaddr.NewMultiaddr(
						"/ip4/127.0.0.1/tcp/30301",
					)
					require.NoError(t, err)
					secondAddress, err := multiaddr.NewMultiaddr(
						"/ip4/127.0.0.1/tcp/30302",
					)
					require.NoError(t, err)
					latestAddress, err := multiaddr.NewMultiaddr(
						"/ip4/127.0.0.1/tcp/30303",
					)
					require.NoError(t, err)

					firstInfo := &peer.AddrInfo{
						ID:    id,
						Addrs: []multiaddr.Multiaddr{firstAddress},
					}
					secondInfo := &peer.AddrInfo{
						ID:    id,
						Addrs: []multiaddr.Multiaddr{secondAddress},
					}
					latestInfo := &peer.AddrInfo{
						ID:    id,
						Addrs: []multiaddr.Multiaddr{latestAddress},
					}

					waitCtx, cancel := context.WithTimeout(
						context.Background(),
						5*time.Second,
					)
					defer cancel()

					s.dialQueue.AddTask(firstInfo, common.PriorityRequestedDial)

					/*
						Consume the initial notification so the later wait
						observes publication of the retained replacement.
					*/
					require.False(t, s.dialQueue.Wait(waitCtx))
					owner := s.dialQueue.PopDialTask()
					require.NotNil(t, owner)
					require.Same(t, firstInfo, owner.GetAddrInfo())

					s.dialQueue.AddTask(secondInfo, common.PriorityRequestedDial)
					s.dialQueue.AddTask(latestInfo, common.PriorityRequestedDial)

					if !pending {
						/*
							The replacement is already runnable when delayed
							discovery cleanup arrives.
						*/
						s.dialQueue.FinishDial(owner)
					}

					discoveryService.HandleNetworkEvent(&peerEvent.PeerEvent{
						PeerID: id,
						Type:   eventType,
						Current: func() bool {
							return !s.hasPeer(id)
						},
					})
					require.NotContains(t, routingTable.ListPeers(), id)

					if pending {
						/*
							Eviction must neither discard the pending request
							nor release its old owner's reservation.
						*/
						require.Nil(t, s.dialQueue.PopDialTask())
						s.dialQueue.FinishDial(owner)
					}

					require.False(
						t,
						s.dialQueue.Wait(waitCtx),
						"discovery eviction lost the retained replacement notification",
					)

					replacement := s.dialQueue.PopDialTask()
					require.NotNil(
						t,
						replacement,
						"discovery eviction discarded a fresh reconnect request",
					)
					require.Same(t, latestInfo, replacement.GetAddrInfo())
					require.NotSame(t, owner, replacement)
					require.Nil(
						t,
						s.dialQueue.PopDialTask(),
						"repeated requests must retain only one replacement",
					)

					s.dialQueue.FinishDial(replacement)
					require.Nil(t, s.dialQueue.PopDialTask())
				})
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2859 -- regress discovery eviction preserving pending and runnable reconnect requests -- */
