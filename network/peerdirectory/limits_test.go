/* START ALPHA CHANGE 2231 -- test bounded admission and authenticated identity quotas -- */
package peerdirectory

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestDirectoryLimitsValidate(t *testing.T) {
	require.NoError(t, DefaultLimits().Validate())

	for _, change := range []func(*Limits){
		func(c *Limits) { c.GlobalRPS = 0 },
		func(c *Limits) { c.GlobalBurst = 0 },
		func(c *Limits) { c.PeerInterval = 0 },
		func(c *Limits) { c.PeerBurst = 0 },
		func(c *Limits) { c.MaxConcurrent = 0 },
		func(c *Limits) { c.MaxPeers = 0 },
	} {
		config := DefaultLimits()
		change(&config)
		_, err := NewServerWithLimits(nil, config)
		require.Error(t, err)
	}
}

func TestDirectoryLimitsGlobalRefillAndSlotRelease(t *testing.T) {
	config := DefaultLimits()
	config.GlobalRPS = 1
	config.GlobalBurst = 2
	config.MaxConcurrent = 1

	limits := newExchangeLimits(config)
	now := time.Now()
	limits.now = func() time.Time { return now }

	require.Zero(t, limits.admit())
	require.Equal(t, http.StatusServiceUnavailable, limits.admit())
	limits.release()

	// Capacity rejection also consumes global admission budget.
	require.Equal(t, http.StatusTooManyRequests, limits.admit())

	now = now.Add(time.Second)
	require.Zero(t, limits.admit())
	limits.release()
}

func TestDirectoryLimitsPeerCapacityAndIdleExpiry(t *testing.T) {
	config := DefaultLimits()
	config.PeerBurst = 1
	config.PeerInterval = time.Second
	config.MaxPeers = 1

	limits := newExchangeLimits(config)
	now := time.Now()
	limits.now = func() time.Time { return now }

	require.Zero(t, limits.allowPeer(peer.ID("a")))
	require.Equal(t, http.StatusTooManyRequests, limits.allowPeer(peer.ID("a")))
	require.Equal(t, http.StatusServiceUnavailable, limits.allowPeer(peer.ID("b")))
	require.Len(t, limits.peers, 1)

	now = now.Add(time.Second)
	require.Zero(t, limits.allowPeer(peer.ID("a")))

	// No active entry is evicted merely because a new identity arrives.
	require.Equal(t, http.StatusServiceUnavailable, limits.allowPeer(peer.ID("b")))

	now = now.Add(limits.idleTTL)
	require.Zero(t, limits.allowPeer(peer.ID("b")))
	require.Len(t, limits.peers, 1)
	require.NotContains(t, limits.peers, peer.ID("a"))
}

func TestDirectoryLimitsConcurrentPeerBurst(t *testing.T) {
	config := DefaultLimits()
	config.PeerBurst = 4

	limits := newExchangeLimits(config)
	now := time.Now()
	limits.now = func() time.Time { return now }

	var accepted atomic.Int64
	var workers sync.WaitGroup

	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if limits.allowPeer(peer.ID("shared")) == 0 {
				accepted.Add(1)
			}
		}()
	}

	workers.Wait()
	require.Equal(t, int64(4), accepted.Load())
	require.Len(t, limits.peers, 1)
}

func TestDirectoryLimitsBeforeDecodeAndBeforeRegistry(t *testing.T) {
	id, req := newAuthorizationOrderRequest(t)
	body, err := json.Marshal(req)
	require.NoError(t, err)

	verifier := &authorizationOrderVerifier{
		allowed: map[peer.ID]struct{}{},
	}
	config := DefaultLimits()
	config.GlobalBurst = 3
	config.PeerBurst = 1

	server, err := NewServerWithLimits(verifier, config)
	require.NoError(t, err)
	now := time.Now()
	server.limits.now = func() time.Time { return now }
	handler := server.Handler()

	// An invalid signature must not allocate or consume a peer bucket.
	tampered := *req
	tampered.Nonce += "-invalid-signature"
	invalidBody, err := json.Marshal(&tampered)
	require.NoError(t, err)

	response := serveAuthorizationOrderRequest(handler, invalidBody)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Empty(t, server.limits.peers)
	require.Zero(t, atomic.LoadInt64(&verifier.calls))

	// A valid identity reaches authorization but is not whitelisted.
	response = serveAuthorizationOrderRequest(handler, body)
	require.Equal(t, http.StatusForbidden, response.Code)
	require.Equal(t, int64(1), atomic.LoadInt64(&verifier.calls))
	require.Contains(t, server.limits.peers, id)

	// Same verified identity is throttled before another registry call.
	response = serveAuthorizationOrderRequest(handler, body)
	require.Equal(t, http.StatusTooManyRequests, response.Code)
	require.NotEmpty(t, response.Header().Get("Retry-After"))
	require.Equal(t, int64(1), atomic.LoadInt64(&verifier.calls))
	require.Empty(t, server.usedNonces)
	require.Empty(t, server.peers)

	// Exhausted global budget is checked before malformed JSON decoding.
	response = serveAuthorizationOrderRequest(handler, []byte("{"))
	require.Equal(t, http.StatusTooManyRequests, response.Code)
	require.Equal(t, int64(1), atomic.LoadInt64(&verifier.calls))
	require.Empty(t, server.limits.slots)
}

type blockingLimitVerifier struct {
	authorizationOrderVerifier
	entered chan struct{}
	release chan struct{}
}

func (v *blockingLimitVerifier) GetAllowedValidatorPeerIDs() (map[peer.ID]struct{}, error) {
	close(v.entered)
	<-v.release

	return v.authorizationOrderVerifier.GetAllowedValidatorPeerIDs()
}

func TestDirectoryLimitsRejectConcurrentOverloadBeforeRegistry(t *testing.T) {
	id, req := newAuthorizationOrderRequest(t)
	body, err := json.Marshal(req)
	require.NoError(t, err)

	verifier := &blockingLimitVerifier{
		authorizationOrderVerifier: authorizationOrderVerifier{
			allowed: map[peer.ID]struct{}{id: {}},
		},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	config := DefaultLimits()
	config.MaxConcurrent = 1

	server, err := NewServerWithLimits(verifier, config)
	require.NoError(t, err)
	handler := server.Handler()

	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(verifier.release) }) }
	defer unblock()

	result := make(chan int, 1)
	go func() {
		result <- serveAuthorizationOrderRequest(handler, body).Code
	}()

	select {
	case <-verifier.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first exchange did not reach registry")
	}

	second := serveAuthorizationOrderRequest(handler, body)
	require.Equal(t, http.StatusServiceUnavailable, second.Code)
	require.Equal(t, "1", second.Header().Get("Retry-After"))

	unblock()

	select {
	case status := <-result:
		require.Equal(t, http.StatusOK, status)
	case <-time.After(5 * time.Second):
		t.Fatal("first exchange did not finish")
	}

	require.Equal(t, int64(1), atomic.LoadInt64(&verifier.calls))
	require.Len(t, server.usedNonces, 1)
	require.Empty(t, server.limits.slots)
}

/* FINISH ALPHA CHANGE 2231 -- test bounded admission and authenticated identity quotas -- */
