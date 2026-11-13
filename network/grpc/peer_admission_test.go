/* START ALPHA CHANGE 2757 -- regress authorization on reused unary and streaming RPC transports -- */
package grpc

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	networkProto "github.com/innovativebioresearch/innbc-chain/network/proto"
	syncProto "github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	rawGrpc "google.golang.org/grpc"
)

type admissionBlockService struct {
	syncProto.UnimplementedSyncPeerServer
	calls atomic.Int32
}

func (s *admissionBlockService) GetBlocks(
	_ *syncProto.GetBlocksRequest,
	stream syncProto.SyncPeer_GetBlocksServer,
) error {
	s.calls.Add(1)

	return stream.Send(&syncProto.Block{Block: []byte("block")})
}

func admissionRPCConnection(
	t *testing.T,
	service *GrpcStream,
) *rawGrpc.ClientConn {
	t.Helper()

	left, right := rpcStreamPair(t)
	t.Cleanup(func() {
		_ = left.Reset()
		_ = right.Reset()
		service.Stop()
	})

	service.Serve()
	handedOff := make(chan struct{})
	go func() {
		defer close(handedOff)
		service.Handler()(right)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := WrapClientContext(ctx, left)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	waitRPCSignal(t, handedOff)

	return conn
}

func TestPeerRPCAdmission_ReusedUnaryConnection(t *testing.T) {
	var allowed atomic.Bool
	var calls atomic.Int32
	allowed.Store(true)

	service := NewGrpcStream()
	service.SetPeerAuthorizer(func(id peer.ID) bool {
		return id == "remote" && allowed.Load()
	})
	networkProto.RegisterTestServiceServer(service.GrpcServer(), &rpcEchoService{
		hello: func(_ context.Context, req *networkProto.GenericMessage) (*networkProto.GenericMessage, error) {
			calls.Add(1)

			return req, nil
		},
	})
	conn := admissionRPCConnection(t, service)
	client := networkProto.NewTestServiceClient(conn)

	call := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := client.SayHello(ctx, &networkProto.GenericMessage{Message: "hello"})

		return err
	}

	require.NoError(t, call())
	allowed.Store(false)
	require.Error(t, call())
	require.Equal(t, int32(1), calls.Load())

	allowed.Store(true)
	require.NoError(t, call())
	require.Equal(t, int32(2), calls.Load())
}

func TestPeerRPCAdmission_ReusedStreamingConnection(t *testing.T) {
	var allowed atomic.Bool
	allowed.Store(true)

	service := NewGrpcStream()
	service.SetPeerAuthorizer(func(id peer.ID) bool {
		return id == "remote" && allowed.Load()
	})
	backend := &admissionBlockService{}
	syncProto.RegisterSyncPeerServer(service.GrpcServer(), backend)
	conn := admissionRPCConnection(t, service)
	client := syncProto.NewSyncPeerClient(conn)

	call := func(expectSuccess bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		stream, err := client.GetBlocks(ctx, &syncProto.GetBlocksRequest{})
		if !expectSuccess {
			if err == nil {
				_, err = stream.Recv()
			}
			require.Error(t, err)

			return
		}

		require.NoError(t, err)
		block, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, []byte("block"), block.Block)
		_, err = stream.Recv()
		require.ErrorIs(t, err, io.EOF)
	}

	call(true)
	allowed.Store(false)
	call(false)
	require.Equal(t, int32(1), backend.calls.Load())

	allowed.Store(true)
	call(true)
	require.Equal(t, int32(2), backend.calls.Load())
}

func TestPeerRPCAdmission_AlreadyAdmittedHandlerCanComplete(t *testing.T) {
	var allowed atomic.Bool
	allowed.Store(true)

	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	service := NewGrpcStream()
	service.SetPeerAuthorizer(func(peer.ID) bool { return allowed.Load() })
	networkProto.RegisterTestServiceServer(service.GrpcServer(), &rpcEchoService{
		hello: func(ctx context.Context, req *networkProto.GenericMessage) (*networkProto.GenericMessage, error) {
			close(entered)

			select {
			case <-release:
				return req, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})
	conn := admissionRPCConnection(t, service)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	result := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := networkProto.NewTestServiceClient(conn).SayHello(
			ctx,
			&networkProto.GenericMessage{Message: "already admitted"},
		)
		result <- err
	}()

	waitRPCSignal(t, entered)
	allowed.Store(false)
	releaseOnce.Do(func() { close(release) })

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("admitted RPC did not complete")
	}
}

/* FINISH ALPHA CHANGE 2757 -- regress authorization on reused unary and streaming RPC transports -- */
