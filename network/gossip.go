package network

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/armon/go-metrics"
	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

const (
	// subscribeOutputBufferSize is the bounded go-libp2p subscription queue.
	// When the complete application pipeline is saturated, libp2p applies its
	// existing drop behavior instead of transferring pressure into goroutines.
	subscribeOutputBufferSize = 1024

	/* START ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */
	topicHandlerQueueSize  = 128
	minTopicHandlerWorkers = 2
	maxTopicHandlerWorkers = 8
	/* FINISH ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */
)

var (
	/* START ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */
	ErrTopicClosed = errors.New("topic is closed")
	/* FINISH ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */
)

/* START ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */
/* START ALPHA CHANGE 2749 -- retain both gossip identities through queued delivery -- */
/* START ALPHA CHANGE 2789 -- retain immutable validation ownership through the topic queue -- */
type topicHandlerJob struct {
	data         []byte
	from         peer.ID
	receivedFrom peer.ID
	ownership    *validatedPeerSession
}

/* FINISH ALPHA CHANGE 2789 -- retain immutable validation ownership through the topic queue -- */

/* FINISH ALPHA CHANGE 2749 -- retain both gossip identities through queued delivery -- */

/* FINISH ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */

type Topic struct {
	logger hclog.Logger

	topic   *pubsub.Topic
	typ     reflect.Type
	closeCh chan struct{}
	closed  atomic.Bool

	/* START ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */
	lifecycleLock sync.Mutex
	waitGroup     sync.WaitGroup
	/* FINISH ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */

	/* START ALPHA CHANGE 2749 -- retain delivery-time gossip authorization -- */
	authorize func(peer.ID, peer.ID) bool
	/* FINISH ALPHA CHANGE 2749 -- retain delivery-time gossip authorization -- */
	/* START ALPHA CHANGE 2790 -- retain ownership validator cleanup until topic shutdown -- */
	closeValidation func()
	/* FINISH ALPHA CHANGE 2790 -- retain ownership validator cleanup until topic shutdown -- */
}

func topicHandlerWorkerCount() int {
	workers := runtime.GOMAXPROCS(0)

	if workers < minTopicHandlerWorkers {
		return minTopicHandlerWorkers
	}

	if workers > maxTopicHandlerWorkers {
		return maxTopicHandlerWorkers
	}

	return workers
}

func (t *Topic) createObj() proto.Message {
	message, ok := reflect.New(t.typ).Interface().(proto.Message)
	if !ok {
		return nil
	}

	return message
}

func (t *Topic) Close() {
	/* START ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */
	t.lifecycleLock.Lock()
	if t.closed.Swap(true) {
		t.lifecycleLock.Unlock()

		return
	}

	close(t.closeCh)
	t.lifecycleLock.Unlock()

	// Wait for subscription readers, cancellation watchers, workers, protobuf
	// unmarshalling, and callbacks that were admitted before shutdown.
	t.waitGroup.Wait()

	t.lifecycleLock.Lock()
	defer t.lifecycleLock.Unlock()

	/* START ALPHA CHANGE 2791 -- unregister the owned validator before releasing its topic -- */
	if t.closeValidation != nil {
		t.closeValidation()
		t.closeValidation = nil
	}
	/* FINISH ALPHA CHANGE 2791 -- unregister the owned validator before releasing its topic -- */

	if t.topic != nil {
		t.topic.Close()
		t.topic = nil
	}
	/* FINISH ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */
}

func (t *Topic) Publish(obj proto.Message) error {
	data, err := proto.Marshal(obj)
	if err != nil {
		return err
	}

	/* START ALPHA CHANGE 1907 -- serialize publication against topic shutdown -- */
	t.lifecycleLock.Lock()
	defer t.lifecycleLock.Unlock()

	if t.closed.Load() || t.topic == nil {
		return ErrTopicClosed
	}

	metrics.SetGauge([]string{networkMetrics, "egress_bytes"}, float32(len(data)))

	return t.topic.Publish(context.Background(), data)
	/* FINISH ALPHA CHANGE 1907 -- serialize publication against topic shutdown -- */
}

/* START ALPHA CHANGE 2792 -- deliver captured sessions without changing ordinary gossip callbacks -- */
func (t *Topic) Subscribe(handler func(obj any, from peer.ID)) error {
	return t.subscribeWithMetadata(func(
		obj any,
		from peer.ID,
		_ *validatedPeerSession,
	) {
		handler(obj, from)
	})
}

func (t *Topic) SubscribeWithPeerSession(
	handler func(obj any, from peer.ID, session *PeerSession),
) error {
	return t.subscribeWithMetadata(func(
		obj any,
		from peer.ID,
		ownership *validatedPeerSession,
	) {
		if ownership == nil ||
			ownership.author != from ||
			ownership.session == nil {
			return
		}

		handler(obj, from, ownership.session)
	})
}

func (t *Topic) subscribeWithMetadata(
	handler func(any, peer.ID, *validatedPeerSession),
) error {
	t.lifecycleLock.Lock()
	defer t.lifecycleLock.Unlock()

	if t.closed.Load() || t.topic == nil {
		return ErrTopicClosed
	}

	sub, err := t.topic.Subscribe(pubsub.WithBufferSize(subscribeOutputBufferSize))
	if err != nil {
		return err
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	workQueue := make(chan topicHandlerJob, topicHandlerQueueSize)
	workerCount := topicHandlerWorkerCount()

	/* Register all workers before shutdown can begin joining them. */
	t.waitGroup.Add(workerCount + 2)

	for range workerCount {
		go t.runMetadataHandlerWorker(workQueue, handler)
	}

	go t.watchForClose(ctx, cancelFn)
	go t.readLoop(ctx, cancelFn, sub, workQueue)

	return nil
}

/* FINISH ALPHA CHANGE 2792 -- deliver captured sessions without changing ordinary gossip callbacks -- */

/* START ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */
func (t *Topic) watchForClose(
	ctx context.Context,
	cancelFn context.CancelFunc,
) {
	defer t.waitGroup.Done()

	select {
	case <-t.closeCh:
		cancelFn()
	case <-ctx.Done():
	}
}

func (t *Topic) readLoop(
	ctx context.Context,
	cancelFn context.CancelFunc,
	/* START ALPHA CHANGE 2793 -- preserve production subscription behavior through a narrow interface -- */
	sub topicSubscription,
	/* FINISH ALPHA CHANGE 2793 -- preserve production subscription behavior through a narrow interface -- */
	workQueue chan<- topicHandlerJob,
) {
	defer t.waitGroup.Done()
	defer cancelFn()
	defer sub.Cancel()
	defer close(workQueue)

	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			if errors.Is(err, ctx.Err()) {
				return
			}

			// A subscription error is terminal for this reader. Logging once and
			// returning avoids an error-driven retry loop.
			t.logger.Error("failed to get topic", "err", err)
			metrics.IncrCounter(
				[]string{networkMetrics, "bad_messages"},
				float32(1),
			)

			return
		}

		/* START ALPHA CHANGE 2749 -- authorize gossip before application queue admission -- */
		author := msg.GetFrom()
		if t.authorize != nil && !t.authorize(msg.ReceivedFrom, author) {
			continue
		}

		/* START ALPHA CHANGE 2794 -- carry validation metadata without recapturing current ownership -- */
		ownership, _ := msg.ValidatorData.(*validatedPeerSession)
		if !t.enqueueHandlerJob(ctx, workQueue, topicHandlerJob{
			data:         msg.Data,
			from:         author,
			receivedFrom: msg.ReceivedFrom,
			ownership:    ownership,
		}) {
			return
		}
		/* FINISH ALPHA CHANGE 2794 -- carry validation metadata without recapturing current ownership -- */
		/* FINISH ALPHA CHANGE 2749 -- authorize gossip before application queue admission -- */
	}
}

func (t *Topic) enqueueHandlerJob(
	ctx context.Context,
	workQueue chan<- topicHandlerJob,
	job topicHandlerJob,
) bool {
	select {
	case workQueue <- job:
		return true
	default:
		// Record bounded backpressure without producing one log entry per
		// message or adding attacker-controlled metric labels.
		metrics.IncrCounter(
			[]string{networkMetrics, "gossip_handler_backpressure"},
			float32(1),
		)
	}

	select {
	case workQueue <- job:
		return true
	case <-ctx.Done():
		return false
	}
}

/* START ALPHA CHANGE 2795 -- keep ordinary workers compatible while forwarding validated metadata -- */
func (t *Topic) runHandlerWorker(
	workQueue <-chan topicHandlerJob,
	handler func(obj any, from peer.ID),
) {
	t.runMetadataHandlerWorker(workQueue, func(
		obj any,
		from peer.ID,
		_ *validatedPeerSession,
	) {
		handler(obj, from)
	})
}

/* FINISH ALPHA CHANGE 2795 -- keep ordinary workers compatible while forwarding validated metadata -- */

func (t *Topic) runMetadataHandlerWorker(
	workQueue <-chan topicHandlerJob,
	handler func(any, peer.ID, *validatedPeerSession),
) {
	defer t.waitGroup.Done()

	for {
		// Prefer shutdown over taking additional queued work.
		select {
		case <-t.closeCh:
			return
		default:
		}

		var (
			job  topicHandlerJob
			open bool
		)

		select {
		case <-t.closeCh:
			return
		case job, open = <-workQueue:
			if !open {
				return
			}
		}

		// Serialize callback admission against Close. Once admitted, the worker
		// is already tracked, so Close waits for unmarshalling and the handler.
		t.lifecycleLock.Lock()
		if t.closed.Load() {
			t.lifecycleLock.Unlock()

			return
		}
		t.lifecycleLock.Unlock()

		obj := t.createObj()
		if obj == nil {
			metrics.IncrCounter(
				[]string{networkMetrics, "bad_messages"},
				float32(1),
			)

			continue
		}

		if err := proto.Unmarshal(job.data, obj); err != nil {
			// Malformed gossip is counted without allowing a remote sender to
			// amplify disk logs.
			metrics.IncrCounter(
				[]string{networkMetrics, "bad_messages"},
				float32(1),
			)

			continue
		}

		metrics.SetGauge(
			[]string{networkMetrics, "ingress_bytes"},
			float32(len(job.data)),
		)

		/* START ALPHA CHANGE 2749 -- reject queued gossip revoked before handler admission -- */
		if t.authorize != nil && !t.authorize(job.receivedFrom, job.from) {
			continue
		}
		/* FINISH ALPHA CHANGE 2749 -- reject queued gossip revoked before handler admission -- */

		/* START ALPHA CHANGE 2796 -- retain originating ownership through callback delivery -- */
		handler(obj, job.from, job.ownership)
		/* FINISH ALPHA CHANGE 2796 -- retain originating ownership through callback delivery -- */
	}
}

/* FINISH ALPHA CHANGE 1907 -- bound per-topic gossip processing and shutdown -- */

func (s *Server) NewTopic(protoID string, obj proto.Message) (*Topic, error) {
	topic, err := s.ps.Join(protoID)
	if err != nil {
		return nil, err
	}

	tt := &Topic{
		logger:  s.logger.Named(protoID),
		topic:   topic,
		typ:     reflect.TypeOf(obj).Elem(),
		closeCh: make(chan struct{}),
		/* START ALPHA CHANGE 2749 -- share current membership with topic delivery -- */
		authorize: s.gossipAllowed,
		/* FINISH ALPHA CHANGE 2749 -- share current membership with topic delivery -- */
	}
	tt.closed.Store(false)

	return tt, nil
}
