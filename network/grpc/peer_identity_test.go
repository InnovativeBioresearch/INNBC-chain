/* START ALPHA CHANGE 2908 -- regress strict transport identity extraction -- */

package grpc

import (
	"context"
	"net"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	grpcPeer "google.golang.org/grpc/peer"
)

func TestRPCPeerIdentity_RejectsMissingTransportIdentity(t *testing.T) {
	var nilAddr *wrapLibp2pAddr

	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"nil_context", nil},
		{"ordinary_context", context.Background()},
		{
			"request_metadata",
			metadata.NewIncomingContext(
				context.Background(),
				metadata.Pairs("peer-id", "claimed-peer"),
			),
		},
		{
			"custom_context_without_transport",
			&Context{Context: context.Background(), PeerID: "claimed-peer"},
		},
		{
			"ordinary_ip_address",
			grpcPeer.NewContext(context.Background(), &grpcPeer.Peer{
				Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1")},
			}),
		},
		{
			"nil_wrapped_address",
			grpcPeer.NewContext(context.Background(), &grpcPeer.Peer{Addr: nilAddr}),
		},
		{
			"empty_transport_identity",
			grpcPeer.NewContext(context.Background(), &grpcPeer.Peer{
				Addr: &wrapLibp2pAddr{Addr: &net.TCPAddr{}},
			}),
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			id, err := PeerIDFromContext(testCase.ctx)
			require.Error(t, err)
			require.Empty(t, id)
		})
	}
}

func TestRPCPeerIdentity_UsesTransportDespiteOtherClaims(t *testing.T) {
	expected := peer.ID("transport-peer")
	ctx := grpcPeer.NewContext(context.Background(), &grpcPeer.Peer{
		Addr: &wrapLibp2pAddr{
			Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1")},
			id:   expected,
		},
	})

	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("peer-id", "different-peer"))
	wrapped := &Context{Context: ctx, PeerID: "another-peer"}

	child, cancel := context.WithCancel(wrapped)
	defer cancel()

	id, err := PeerIDFromContext(child)
	require.NoError(t, err)
	require.Equal(t, expected, id)
}

/* FINISH ALPHA CHANGE 2908 -- regress strict transport identity extraction -- */
