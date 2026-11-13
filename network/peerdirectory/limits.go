/* START ALPHA CHANGE 2224 -- bound directory request rate, concurrency and identity tracking -- */
package peerdirectory

import (
	"container/list"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/time/rate"
)

type Limits struct {
	GlobalRPS     int
	GlobalBurst   int
	PeerInterval  time.Duration
	PeerBurst     int
	MaxConcurrent int
	MaxPeers      int
}

func DefaultLimits() Limits {
	return Limits{
		GlobalRPS:     20,
		GlobalBurst:   64,
		PeerInterval:  5 * time.Second,
		PeerBurst:     8,
		MaxConcurrent: 32,
		MaxPeers:      4096,
	}
}

func (c Limits) Validate() error {
	if c.GlobalRPS < 1 || c.GlobalRPS > 10000 {
		return fmt.Errorf("directory global rate must be between 1 and 10000 requests/second")
	}
	if c.GlobalBurst < 1 || c.GlobalBurst > 10000 {
		return fmt.Errorf("directory global burst must be between 1 and 10000")
	}
	if c.PeerInterval < time.Millisecond || c.PeerInterval > 10*time.Second {
		return fmt.Errorf("directory peer interval must be between 1ms and 10s")
	}
	if c.PeerBurst < 1 || c.PeerBurst > 64 {
		return fmt.Errorf("directory peer burst must be between 1 and 64")
	}
	if c.MaxConcurrent < 1 || c.MaxConcurrent > 1024 {
		return fmt.Errorf("directory concurrent exchange limit must be between 1 and 1024")
	}
	if c.MaxPeers < 1 || c.MaxPeers > 65536 {
		return fmt.Errorf("directory tracked-peer limit must be between 1 and 65536")
	}

	return nil
}

type peerRateEntry struct {
	id       peer.ID
	limiter  *rate.Limiter
	lastSeen time.Time
}

type exchangeLimits struct {
	config Limits
	global *rate.Limiter
	slots  chan struct{}
	now    func() time.Time

	mu      sync.Mutex
	peers   map[peer.ID]*list.Element
	oldest  list.List
	idleTTL time.Duration
}

func newExchangeLimits(config Limits) *exchangeLimits {
	// Never remove an idle entry before its bucket could refill fully.
	// This prevents eviction from granting an identity an early new burst.
	idleTTL := 10 * time.Minute
	if refill := time.Duration(config.PeerBurst) * config.PeerInterval; refill > idleTTL {
		idleTTL = refill
	}

	return &exchangeLimits{
		config:  config,
		global:  rate.NewLimiter(rate.Limit(config.GlobalRPS), config.GlobalBurst),
		slots:   make(chan struct{}, config.MaxConcurrent),
		now:     time.Now,
		peers:   make(map[peer.ID]*list.Element),
		idleTTL: idleTTL,
	}
}

// admit never waits for a slot. A successful call must be paired with release.
func (l *exchangeLimits) admit() int {
	if !l.global.AllowN(l.now(), 1) {
		return http.StatusTooManyRequests
	}

	select {
	case l.slots <- struct{}{}:
		return 0
	default:
		return http.StatusServiceUnavailable
	}
}

func (l *exchangeLimits) release() {
	<-l.slots
}

// allowPeer must only be called after verifying ownership of the peer ID.
func (l *exchangeLimits) allowPeer(id peer.ID) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	element := l.peers[id]

	if element == nil {
		if len(l.peers) >= l.config.MaxPeers {
			oldest := l.oldest.Front()
			if oldest == nil {
				return http.StatusServiceUnavailable
			}

			entry := oldest.Value.(*peerRateEntry)
			if now.Sub(entry.lastSeen) < l.idleTTL {
				return http.StatusServiceUnavailable
			}

			delete(l.peers, entry.id)
			l.oldest.Remove(oldest)
		}

		entry := &peerRateEntry{
			id:       id,
			limiter:  rate.NewLimiter(rate.Every(l.config.PeerInterval), l.config.PeerBurst),
			lastSeen: now,
		}
		element = l.oldest.PushBack(entry)
		l.peers[id] = element
	}

	entry := element.Value.(*peerRateEntry)
	entry.lastSeen = now
	l.oldest.MoveToBack(element)

	if !entry.limiter.AllowN(now, 1) {
		return http.StatusTooManyRequests
	}

	return 0
}

func writeLimitResponse(w http.ResponseWriter, status int, retryAfter time.Duration) {
	seconds := int64((retryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}

	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	w.Header().Set("Cache-Control", "no-store")

	message := "peer directory temporarily at capacity"
	if status == http.StatusTooManyRequests {
		message = "peer directory request rate exceeded"
	}

	// Deliberately do not log every rejected request.
	writeError(w, status, message)
}

/* FINISH ALPHA CHANGE 2224 -- bound directory request rate, concurrency and identity tracking -- */
