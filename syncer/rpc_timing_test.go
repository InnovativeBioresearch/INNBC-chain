/* START ALPHA CHANGE 2735 -- test progressing downloads beyond establishment and per-block intervals -- */

package syncer

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	rawGrpc "google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

const syncTimingWatchdog = 15 * time.Second

func waitSyncTimingSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()

	timer := time.NewTimer(syncTimingWatchdog)
	defer timer.Stop()

	select {
	case <-signal:
	case <-timer.C:
		t.Fatal(message)
	}
}

func closeSyncTimingClient(t *testing.T, client *syncPeerClient) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		client.Close()
	}()
	waitSyncTimingSignal(t, done, "sync client shutdown did not join its work")
}

type syncTimingSession struct {
	blocks   <-chan *types.Block
	terminal <-chan BlockStreamTerminal
	err      error
}

func openSyncTimingSession(
	t *testing.T,
	networkStub *rpcLifecycleNetwork,
	perBlock time.Duration,
	releaseGates func(),
) (*syncPeerClient, syncTimingSession) {
	t.Helper()

	client := NewSyncPeerClient(
		hclog.NewNullLogger(),
		networkStub,
		nil,
	).(*syncPeerClient)

	results := make(chan syncTimingSession, 1)
	startDone := make(chan struct{})

	t.Cleanup(func() {
		/* Gates are released before waiting for production client workers. */
		releaseGates()
		closeSyncTimingClient(t, client)
		waitSyncTimingSignal(t, startDone, "GetBlocks caller did not terminate")
	})

	go func() {
		defer close(startDone)

		blocks, terminal, err := client.GetBlocks("peer", 1, perBlock)
		results <- syncTimingSession{
			blocks:   blocks,
			terminal: terminal,
			err:      err,
		}
	}()

	timer := time.NewTimer(syncTimingWatchdog)
	defer timer.Stop()

	select {
	case session := <-results:
		require.NoError(t, session.err)
		require.NotNil(t, session.blocks)
		require.NotNil(t, session.terminal)
		waitSyncTimingSignal(t, startDone, "GetBlocks caller did not exit")

		return client, session
	case <-timer.C:
		t.Fatal("GetBlocks did not finish establishment")

		return client, syncTimingSession{}
	}
}

func readSyncTimingBlock(t *testing.T, session syncTimingSession) *types.Block {
	t.Helper()

	timer := time.NewTimer(syncTimingWatchdog)
	defer timer.Stop()

	select {
	case block, open := <-session.blocks:
		require.True(t, open, "download ended before the expected block")
		require.NotNil(t, block)

		return block
	case <-timer.C:
		t.Fatal("download did not deliver the expected complete block")

		return nil
	}
}

func sendSyncTimingPermit(t *testing.T, permit chan<- struct{}) {
	t.Helper()

	timer := time.NewTimer(syncTimingWatchdog)
	defer timer.Stop()

	select {
	case permit <- struct{}{}:
	case <-timer.C:
		t.Fatal("download handler did not accept the next block permit")
	}
}

func requireSyncTimingCompletion(
	t *testing.T,
	client *syncPeerClient,
	session syncTimingSession,
	conn *rawGrpc.ClientConn,
) {
	t.Helper()

	timer := time.NewTimer(syncTimingWatchdog)
	defer timer.Stop()

	select {
	case result, open := <-session.terminal:
		require.True(t, open, "terminal channel closed without a result")
		require.NoError(t, result.Err)
	case <-timer.C:
		t.Fatal("download did not publish successful completion")
	}

	select {
	case block, open := <-session.blocks:
		require.False(t, open, "download delivered an unexpected extra block")
		require.Nil(t, block)
	case <-timer.C:
		t.Fatal("block output did not close after completion")
	}

	select {
	case _, open := <-session.terminal:
		require.False(t, open, "download published an extra terminal result")
	case <-timer.C:
		t.Fatal("terminal output did not close after completion")
	}

	require.Equal(t, connectivity.Shutdown, conn.GetState())

	client.blockStreamLock.Lock()
	remaining := len(client.blockStreamCancels)
	client.blockStreamLock.Unlock()
	require.Zero(t, remaining, "completed download retained its registration")

	closeSyncTimingClient(t, client)
}

func takeSyncTimingConnection(
	t *testing.T,
	connections <-chan *rawGrpc.ClientConn,
) *rawGrpc.ClientConn {
	t.Helper()

	timer := time.NewTimer(syncTimingWatchdog)
	defer timer.Stop()

	select {
	case conn := <-connections:
		require.NotNil(t, conn)

		return conn
	case <-timer.C:
		t.Fatal("test transport did not report its established connection")

		return nil
	}
}

type syncTimingConn struct {
	libnetwork.Conn
	addr multiaddr.Multiaddr
}

func (c *syncTimingConn) LocalMultiaddr() multiaddr.Multiaddr  { return c.addr }
func (c *syncTimingConn) RemoteMultiaddr() multiaddr.Multiaddr { return c.addr }
func (*syncTimingConn) LocalPeer() peer.ID                     { return "local" }
func (*syncTimingConn) RemotePeer() peer.ID                    { return "remote" }

type syncTimingStream struct {
	libnetwork.Stream
	socket net.Conn
	info   *syncTimingConn
}

func (s *syncTimingStream) Read(data []byte) (int, error) {
	return s.socket.Read(data)
}

func (s *syncTimingStream) Write(data []byte) (int, error) {
	return s.socket.Write(data)
}

func (s *syncTimingStream) Conn() libnetwork.Conn { return s.info }
func (s *syncTimingStream) Close() error          { return s.socket.Close() }
func (s *syncTimingStream) Reset() error          { return s.socket.Close() }

func (s *syncTimingStream) SetDeadline(deadline time.Time) error {
	return s.socket.SetDeadline(deadline)
}

func (s *syncTimingStream) SetReadDeadline(deadline time.Time) error {
	return s.socket.SetReadDeadline(deadline)
}

func (s *syncTimingStream) SetWriteDeadline(deadline time.Time) error {
	return s.socket.SetWriteDeadline(deadline)
}

type syncTimingSetup struct {
	ctx  context.Context
	conn *rawGrpc.ClientConn
}

type syncTimingTransportResources struct {
	left        *syncTimingStream
	right       *syncTimingStream
	cancelSetup context.CancelFunc
	handoffDone chan struct{}
	conn        *rawGrpc.ClientConn
}

func newSyncTimingTransport(
	t *testing.T,
	service proto.SyncPeerServer,
	setupLifetime time.Duration,
) (*rpcLifecycleNetwork, <-chan syncTimingSetup) {
	t.Helper()

	addr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1234")
	require.NoError(t, err)

	listener := netgrpc.NewGrpcStream()
	t.Cleanup(listener.Stop)
	proto.RegisterSyncPeerServer(listener.GrpcServer(), service)
	listener.Serve()

	setups := make(chan syncTimingSetup, 1)
	var resourcesLock sync.Mutex
	var resources []*syncTimingTransportResources

	t.Cleanup(func() {
		resourcesLock.Lock()
		cached := append([]*syncTimingTransportResources(nil), resources...)
		resourcesLock.Unlock()

		for _, resource := range cached {
			resource.cancelSetup()
			_ = resource.left.Reset()
			_ = resource.right.Reset()

			if resource.conn != nil {
				_ = resource.conn.Close()
			}

			waitSyncTimingSignal(
				t,
				resource.handoffDone,
				"test transport handoff did not terminate",
			)
		}
	})

	networkStub := &rpcLifecycleNetwork{
		dial: func(parent context.Context) (*rawGrpc.ClientConn, error) {
			leftSocket, rightSocket := net.Pipe()
			info := &syncTimingConn{addr: addr}
			setupCtx, cancelSetup := context.WithTimeout(parent, setupLifetime)

			resource := &syncTimingTransportResources{
				left: &syncTimingStream{
					socket: leftSocket,
					info:   info,
				},
				right: &syncTimingStream{
					socket: rightSocket,
					info:   info,
				},
				cancelSetup: cancelSetup,
				handoffDone: make(chan struct{}),
			}

			resourcesLock.Lock()
			resources = append(resources, resource)
			resourcesLock.Unlock()

			go func() {
				defer close(resource.handoffDone)
				listener.Handler()(resource.right)
			}()

			conn, err := netgrpc.WrapClientContext(setupCtx, resource.left)
			if err != nil {
				return nil, err
			}

			resourcesLock.Lock()
			resource.conn = conn
			resourcesLock.Unlock()

			/* Setup expires naturally; production GetBlocks owns the connection. */
			select {
			case setups <- syncTimingSetup{ctx: setupCtx, conn: conn}:
				return conn, nil
			case <-setupCtx.Done():
				_ = conn.Close()

				return nil, setupCtx.Err()
			}
		},
	}

	return networkStub, setups
}

func TestSyncRPCTiming_DownloadOutlivesTransportSetupDeadline(t *testing.T) {
	releaseNext := make(chan struct{})
	abort := make(chan struct{})
	handlerExited := make(chan struct{})
	deadlineSeen := make(chan bool, 1)
	var releaseOnce sync.Once
	var abortOnce sync.Once

	releaseGates := func() {
		releaseOnce.Do(func() { close(releaseNext) })
		abortOnce.Do(func() { close(abort) })
	}

	service := &rpcOwnershipSyncService{
		blocks: func(
			req *proto.GetBlocksRequest,
			stream proto.SyncPeer_GetBlocksServer,
		) error {
			defer close(handlerExited)

			_, hasDeadline := stream.Context().Deadline()
			select {
			case deadlineSeen <- hasDeadline:
			case <-stream.Context().Done():
				return stream.Context().Err()
			}

			first := &types.Block{Header: &types.Header{Number: req.From}}
			if err := stream.Send(toProtoBlock(first)); err != nil {
				return err
			}

			select {
			case <-releaseNext:
			case <-abort:
				return context.Canceled
			case <-stream.Context().Done():
				return stream.Context().Err()
			}

			second := &types.Block{Header: &types.Header{Number: req.From + 1}}

			return stream.Send(toProtoBlock(second))
		},
	}

	networkStub, setups := newSyncTimingTransport(t, service, 2*time.Second)
	client, session := openSyncTimingSession(
		t,
		networkStub,
		10*time.Second,
		releaseGates,
	)

	var setup syncTimingSetup
	timer := time.NewTimer(syncTimingWatchdog)
	defer timer.Stop()

	select {
	case setup = <-setups:
		require.NotNil(t, setup.conn)
	case <-timer.C:
		t.Fatal("test adapter did not report establishment")
	}

	select {
	case hasDeadline := <-deadlineSeen:
		require.False(t, hasDeadline, "download inherited the setup deadline")
	case <-timer.C:
		t.Fatal("download handler did not report its context")
	}

	require.Equal(t, uint64(1), readSyncTimingBlock(t, session).Number())

	deadline, hasDeadline := setup.ctx.Deadline()
	require.True(t, hasDeadline)
	waitSyncTimingSignal(t, setup.ctx.Done(), "transport setup deadline did not expire")
	require.ErrorIs(t, setup.ctx.Err(), context.DeadlineExceeded)
	require.False(t, time.Now().Before(deadline))

	releaseOnce.Do(func() { close(releaseNext) })
	require.Equal(t, uint64(2), readSyncTimingBlock(t, session).Number())

	requireSyncTimingCompletion(t, client, session, setup.conn)
	waitSyncTimingSignal(t, handlerExited, "download handler did not exit")
}

func TestSyncRPCTiming_PerBlockProgressExtendsDownload(t *testing.T) {
	const blockCount = 5
	const perBlock = 2 * time.Second
	const gap = 700 * time.Millisecond

	permit := make(chan struct{})
	abort := make(chan struct{})
	handlerEntered := make(chan struct{})
	handlerExited := make(chan struct{})
	var abortOnce sync.Once

	releaseGates := func() {
		abortOnce.Do(func() { close(abort) })
	}

	service := &rpcOwnershipSyncService{
		blocks: func(
			req *proto.GetBlocksRequest,
			stream proto.SyncPeer_GetBlocksServer,
		) error {
			defer close(handlerExited)
			close(handlerEntered)

			for offset := uint64(0); offset < blockCount; offset++ {
				select {
				case <-permit:
				case <-abort:
					return context.Canceled
				case <-stream.Context().Done():
					return stream.Context().Err()
				}

				block := &types.Block{
					Header: &types.Header{Number: req.From + offset},
				}
				if err := stream.Send(toProtoBlock(block)); err != nil {
					return err
				}
			}

			return nil
		},
	}

	networkStub, connections := newOwnershipRPCNetwork(t, service)
	client, session := openSyncTimingSession(t, networkStub, perBlock, releaseGates)
	conn := takeSyncTimingConnection(t, connections)
	waitSyncTimingSignal(t, handlerEntered, "progressing download did not start")

	received := make([]time.Time, 0, blockCount)

	for number := uint64(1); number <= blockCount; number++ {
		if number > 1 {
			/* This deliberate interval tests renewal between complete blocks. */
			timer := time.NewTimer(gap)
			select {
			case <-timer.C:
			case <-session.terminal:
				timer.Stop()
				t.Fatal("download terminated before the next block interval")
			}
			timer.Stop()
		}

		sendSyncTimingPermit(t, permit)
		block := readSyncTimingBlock(t, session)
		require.Equal(t, number, block.Number())
		received = append(received, time.Now())
	}

	elapsed := received[len(received)-1].Sub(received[0])
	t.Logf("received %d blocks over %s; per-block limit %s", len(received), elapsed, perBlock)
	require.Greater(t, elapsed, perBlock)

	requireSyncTimingCompletion(t, client, session, conn)
	waitSyncTimingSignal(t, handlerExited, "progressing download handler did not exit")
}

func TestSyncRPCTiming_DownloadSurvivesProductionSetupTimeout(t *testing.T) {
	if os.Getenv("INNBC_TEST_LONG_RPC") != "1" {
		t.Skip("set INNBC_TEST_LONG_RPC=1 to cross the production 100-second setup timeout")
	}

	const perBlock = 15 * time.Second
	threshold := netgrpc.DefaultProtocolTimeout + 5*time.Second

	testCtx, cancelTest := context.WithTimeout(
		context.Background(),
		netgrpc.DefaultProtocolTimeout+60*time.Second,
	)
	defer cancelTest()

	permit := make(chan struct{})
	finish := make(chan struct{})
	abort := make(chan struct{})
	handlerEntered := make(chan struct{})
	handlerExited := make(chan struct{})
	var finishOnce sync.Once
	var abortOnce sync.Once

	releaseGates := func() {
		abortOnce.Do(func() { close(abort) })
		finishOnce.Do(func() { close(finish) })
	}

	service := &rpcOwnershipSyncService{
		blocks: func(
			req *proto.GetBlocksRequest,
			stream proto.SyncPeer_GetBlocksServer,
		) error {
			defer close(handlerExited)
			close(handlerEntered)

			for number := req.From; ; number++ {
				select {
				case <-permit:
				case <-finish:
					return nil
				case <-abort:
					return context.Canceled
				case <-testCtx.Done():
					return testCtx.Err()
				case <-stream.Context().Done():
					return stream.Context().Err()
				}

				block := &types.Block{Header: &types.Header{Number: number}}
				if err := stream.Send(toProtoBlock(block)); err != nil {
					return err
				}
			}
		},
	}

	networkStub, connections := newOwnershipRPCNetwork(t, service)
	client, session := openSyncTimingSession(t, networkStub, perBlock, releaseGates)

	/* Measuring from successful return is later than the setup timer's start. */
	started := time.Now()
	conn := takeSyncTimingConnection(t, connections)
	waitSyncTimingSignal(t, handlerEntered, "long download did not start")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var received uint64
	var elapsed time.Duration

	for {
		if received > 0 {
			select {
			case <-ticker.C:
			case <-testCtx.Done():
				t.Fatal("long download exceeded its overall test watchdog")
			}
		}

		sendSyncTimingPermit(t, permit)
		block := readSyncTimingBlock(t, session)
		received++
		require.Equal(t, received, block.Number())

		elapsed = time.Since(started)
		if elapsed >= threshold {
			break
		}

		require.NoError(t, testCtx.Err())
	}

	t.Logf(
		"received %d blocks over %s; production setup limit %s",
		received,
		elapsed,
		netgrpc.DefaultProtocolTimeout,
	)
	require.GreaterOrEqual(t, elapsed, threshold)

	finishOnce.Do(func() { close(finish) })
	requireSyncTimingCompletion(t, client, session, conn)
	waitSyncTimingSignal(t, handlerExited, "long download handler did not exit")
}

/* FINISH ALPHA CHANGE 2735 -- test progressing downloads beyond establishment and per-block intervals -- */
