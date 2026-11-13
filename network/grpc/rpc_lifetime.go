package grpc

import (
	"context"
	"errors"
	"sync"
	"time"

	rawGrpc "google.golang.org/grpc"
)

/* START ALPHA CHANGE 2670 -- coordinate RPC admission cancellation and connection ownership -- */

const DefaultProtocolTimeout = 100 * time.Second

var ErrRPCClosed = errors.New("RPC lifetime is closed")

// RPCLifetime admits operations before they can block.
// Its zero value is ready for use. It must not be copied after use.
type RPCLifetime struct {
	lock    sync.Mutex
	closing bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// Begin returns an operation context and an idempotent completion function.
// The caller must release its resources before calling finish.
func (l *RPCLifetime) Begin(parent context.Context) (
	context.Context,
	func(),
	error,
) {
	l.lock.Lock()

	if l.closing {
		l.lock.Unlock()

		return nil, nil, ErrRPCClosed
	}

	if err := parent.Err(); err != nil {
		l.lock.Unlock()

		return nil, nil, err
	}

	if l.ctx == nil {
		l.ctx, l.cancel = context.WithCancel(context.Background())
	}

	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(l.ctx, cancel)
	l.wg.Add(1)
	l.lock.Unlock()

	var once sync.Once

	finish := func() {
		once.Do(func() {
			stop()
			cancel()
			l.wg.Done()
		})
	}

	return ctx, finish, nil
}

// Close prevents admission, cancels existing operations, and joins them.
// Concurrent and repeated calls wait for the same admitted operations.
func (l *RPCLifetime) Close() {
	l.lock.Lock()
	l.closing = true
	cancel := l.cancel
	l.lock.Unlock()

	if cancel != nil {
		cancel()
	}

	l.wg.Wait()
}

// OwnClient gives one operation responsibility for its connection.
// Cancellation and normal completion share the same synchronous cleanup.
func OwnClient(ctx context.Context, conn *rawGrpc.ClientConn) func() {
	var once sync.Once

	closeConn := func() {
		once.Do(func() {
			_ = conn.Close()
		})
	}

	stop := context.AfterFunc(ctx, closeConn)

	return func() {
		stop()
		closeConn()
	}
}

/* FINISH ALPHA CHANGE 2670 -- coordinate RPC admission cancellation and connection ownership -- */
