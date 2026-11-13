/* START ALPHA CHANGE 2847 -- regress discovery rejection before peerstore and routing insertion -- */
package discovery

import (
	"fmt"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/network/common"
	networkTesting "github.com/innovativebioresearch/innbc-chain/network/testing"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryAddressValidation_AddresslessRecordNeverReachesInsertion(t *testing.T) {
	peers := getRandomPeers(t, 2)
	addresslessPeer := peers[0]
	validPeer := peers[1]

	stored := make(map[peer.ID]*peer.AddrInfo)
	discovery, err := newDiscoveryService(
		func(server *networkTesting.MockNetworkingServer) {
			server.HookIsPeerAllowed(func(peer.ID) bool {
				return true
			})
			server.HookAddToPeerStore(func(info *peer.AddrInfo) {
				stored[info.ID] = info
			})
			server.GetMockPeerMetrics().HookLatencyEWMA(
				func(peer.ID) time.Duration {
					return 0
				},
			)
		},
	)
	require.NoError(t, err)
	t.Cleanup(discovery.Close)

	validAddress, err := common.AddrInfoToString(validPeer)
	require.NoError(t, err)

	discovery.addPeersToTable([]string{
		fmt.Sprintf("/p2p/%s", addresslessPeer.ID),
		validAddress,
	})

	require.NotContains(t, stored, addresslessPeer.ID)
	require.Contains(t, stored, validPeer.ID)
	require.ElementsMatch(t, []peer.ID{validPeer.ID}, discovery.RoutingTablePeers())
}

/* FINISH ALPHA CHANGE 2847 -- regress discovery rejection before peerstore and routing insertion -- */
