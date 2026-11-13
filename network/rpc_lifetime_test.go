package network

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/network/dial"
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/libp2p/go-libp2p/core/host"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2722 -- regress unlocked protocol establishment and network shutdown cancellation -- */

type rpcBlockingHost struct {
	host.Host
	started chan struct{}
}

func (h *rpcBlockingHost) NewStream(
	ctx context.Context,
	_ peer.ID,
	_ ...protocol.ID,
) (libnetwork.Stream, error) {
	close(h.started)
	<-ctx.Done()

	return nil, ctx.Err()
}

func (h *rpcBlockingHost) SetStreamHandler(
	protocol.ID,
	libnetwork.StreamHandler,
) {
}

func (h *rpcBlockingHost) Close() error { return nil }

func TestRPCProtocol_DialDoesNotHoldRegistryLock(t *testing.T) {
	h := &rpcBlockingHost{started: make(chan struct{})}
	server := &Server{
		logger:    hclog.NewNullLogger(),
		host:      h,
		closeCh:   make(chan struct{}),
		dialQueue: dial.NewDialQueue(),
		protocols: map[string]Protocol{
			"/rpc/a": netgrpc.NewGrpcStream(),
		},
		peers: make(map[peer.ID]*PeerConnInfo),
	}
	t.Cleanup(func() { require.NoError(t, server.Close()) })

	dialDone := make(chan error, 1)
	go func() {
		_, err := server.NewProtoConnectionContext(
			context.Background(),
			"/rpc/a",
			"peer",
		)
		dialDone <- err
	}()

	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("dial did not start")
	}

	registered := make(chan struct{})
	go func() {
		server.RegisterProtocol("/rpc/b", netgrpc.NewGrpcStream())
		close(registered)
	}()

	select {
	case <-registered:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked dial retained the protocol registry lock")
	}

	closed := make(chan struct{})
	go func() {
		_ = server.Close()
		close(closed)
	}()

	select {
	case err := <-dialDone:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("network closure did not cancel dialing")
	}

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("network closure did not join dialing")
	}

	require.NoError(t, server.Close())
}

/* FINISH ALPHA CHANGE 2722 -- regress unlocked protocol establishment and network shutdown cancellation -- */
