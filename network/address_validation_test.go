/* START ALPHA CHANGE 2845 -- regress addressless response rejection and defensive peerstore handling -- */
package network

import (
	"fmt"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/helper/tests"
	"github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/innovativebioresearch/innbc-chain/network/dial"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func addressValidationPeer(t *testing.T) *peer.AddrInfo {
	t.Helper()

	info, err := peer.AddrInfoFromP2pAddr(tests.GenerateTestMultiAddr(t))
	require.NoError(t, err)

	return info
}

func TestPeerAddressValidation_InvalidResponsePreservesPreviousState(t *testing.T) {
	s := admissionFixture(t, true)
	s.dialQueue = dial.NewDialQueue()
	t.Cleanup(s.dialQueue.Close)

	require.True(t, s.startPeerAdmission())

	oldPeer := addressValidationPeer(t)
	newPeer := addressValidationPeer(t)
	require.True(t, s.replaceAllowedPeers([]peer.ID{oldPeer.ID}))
	s.AddToPeerStore(oldPeer)

	previousAddrs := s.host.Peerstore().Addrs(oldPeer.ID)
	require.NotEmpty(t, previousAddrs)

	response := []string{
		fmt.Sprintf("/ip4/127.0.0.1/tcp/40301/p2p/%s", oldPeer.ID),
		fmt.Sprintf("/p2p/%s", newPeer.ID),
	}

	var applyErr error
	require.NotPanics(t, func() {
		applyErr = s.applyPeerDirectoryPeers(response)
	})
	require.ErrorContains(t, applyErr, "no transport address")

	require.True(t, s.IsPeerAllowed(oldPeer.ID))
	require.False(t, s.IsPeerAllowed(newPeer.ID))
	require.ElementsMatch(t, previousAddrs, s.host.Peerstore().Addrs(oldPeer.ID))
	require.Empty(t, s.host.Peerstore().Addrs(newPeer.ID))
	require.Nil(t, s.dialQueue.PopDialTask())
}

func TestPeerAddressValidation_TransportFormsRemainAccepted(t *testing.T) {
	for _, transport := range []string{
		"/ip4/127.0.0.1/tcp/30301",
		"/ip6/::1/tcp/30301",
		"/dns4/validator.example.com/tcp/30301",
	} {
		t.Run(transport, func(t *testing.T) {
			s := admissionFixture(t, true)
			s.dialQueue = dial.NewDialQueue()
			t.Cleanup(s.dialQueue.Close)
			require.True(t, s.startPeerAdmission())

			info := addressValidationPeer(t)
			address := fmt.Sprintf("%s/p2p/%s", transport, info.ID)

			require.NoError(t, s.applyPeerDirectoryPeers([]string{address}))
			require.True(t, s.IsPeerAllowed(info.ID))

			expected, err := common.StringToAddrInfo(address)
			require.NoError(t, err)
			require.ElementsMatch(t, expected.Addrs, s.host.Peerstore().Addrs(info.ID))
		})
	}
}

func TestPeerAddressValidation_PeerstoreRejectsMissingAddresses(t *testing.T) {
	s := admissionFixture(t, false)
	require.True(t, s.startPeerAdmission())

	info := addressValidationPeer(t)
	s.AddToPeerStore(info)
	previousAddrs := s.host.Peerstore().Addrs(info.ID)

	for _, invalid := range []*peer.AddrInfo{
		nil,
		{ID: info.ID},
		{ID: info.ID, Addrs: []multiaddr.Multiaddr{nil}},
	} {
		require.NotPanics(t, func() {
			s.AddToPeerStore(invalid)
		})
	}

	require.ElementsMatch(t, previousAddrs, s.host.Peerstore().Addrs(info.ID))
}

/* FINISH ALPHA CHANGE 2845 -- regress addressless response rejection and defensive peerstore handling -- */
