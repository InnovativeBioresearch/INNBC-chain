package network

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	testproto "github.com/innovativebioresearch/innbc-chain/network/proto"
	/* START ALPHA CHANGE 3879 -- access GossipSub routing readiness in the gossip regression -- */
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	/* FINISH ALPHA CHANGE 3879 -- access GossipSub routing readiness in the gossip regression -- */
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func NumSubscribers(srv *Server, topic string) int {
	return len(srv.ps.ListPeers(topic))
}

func WaitForSubscribers(ctx context.Context, srv *Server, topic string, expectedNumPeers int) error {
	for {
		if n := NumSubscribers(srv, topic); n >= expectedNumPeers {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("canceled")
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}
}

/* START ALPHA CHANGE 3880 -- verify routing readiness and distinct gossip recipients with safe cleanup -- */
func TestSimpleGossip(t *testing.T) {
	const numServers = 9

	sentMessage := fmt.Sprintf("simple-gossip-%d", time.Now().UnixNano())
	readinessMessage := sentMessage + "-readiness"

	servers, createErr := createServers(numServers, nil)
	require.NoError(t, createErr, "Unable to create servers")

	type delivery struct {
		node int
		err  error
	}

	// Each node reports at most once. Buffering and cancellation also keep
	// late callbacks from blocking cleanup after a test failure.
	deliveries := make(chan delivery, numServers)
	callbackCtx, cancelCallbacks := context.WithCancel(context.Background())
	serverTopics := make([]*Topic, numServers)

	/* START ALPHA CHANGE 1908 -- test bounded and joined gossip processing -- */
	t.Cleanup(func() {
		cancelCallbacks()

		for _, topic := range serverTopics {
			if topic != nil {
				topic.Close()
			}
		}

		closeTestServers(t, servers)
	})
	/* FINISH ALPHA CHANGE 1908 -- test bounded and joined gossip processing -- */

	joinErrors := MeshJoin(servers...)
	require.Empty(t, joinErrors, "Unable to join servers [%d], %v", len(joinErrors), joinErrors)

	topicName := "msg-pub-sub"

	for i := range numServers {
		topic, topicErr := servers[i].NewTopic(topicName, &testproto.GenericMessage{})
		require.NoError(t, topicErr, "Unable to create topic")

		serverTopics[i] = topic
		reported := new(atomic.Bool)

		subscribeErr := topic.Subscribe(func(obj any, _ peer.ID) {
			message, ok := obj.(*testproto.GenericMessage)

			if ok && message != nil && message.Message == readinessMessage {
				return
			}

			var deliveryErr error

			switch {
			case !ok || message == nil:
				deliveryErr = fmt.Errorf("unexpected callback value %T", obj)
			case message.Message != sentMessage:
				deliveryErr = fmt.Errorf(
					"unexpected message %q, expected %q",
					message.Message,
					sentMessage,
				)
			}

			if !reported.CompareAndSwap(false, true) {
				return
			}

			select {
			case deliveries <- delivery{node: i, err: deliveryErr}:
			case <-callbackCtx.Done():
			}
		})
		require.NoError(t, subscribeErr, "Unable to subscribe to topic")
	}

	readyCtx, cancelReady := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancelReady()

	// Check every node's topic view, rather than only the publisher's.
	for i, server := range servers {
		err := WaitForSubscribers(
			readyCtx,
			server,
			topicName,
			numServers-1,
		)
		require.NoError(
			t,
			err,
			"server[%d] peer=%s did not discover all topic subscribers",
			i,
			server.host.ID(),
		)
	}

	readinessPayload, err := proto.Marshal(&testproto.GenericMessage{
		Message: readinessMessage,
	})
	require.NoError(t, err)

	/* START ALPHA CHANGE 3881 -- use the router minimum rather than its target mesh size -- */
	requiredMeshPeers := min(pubsub.GossipSubDlo, numServers-1)
	/* FINISH ALPHA CHANGE 3881 -- use the router minimum rather than its target mesh size -- */

	for i, topic := range serverTopics {
		// WithReadiness checks EnoughPeers on GossipSub's serialized
		// router loop. Local publication prevents remote probe traffic.
		err := topic.topic.Publish(
			readyCtx,
			readinessPayload,
			pubsub.WithReadiness(pubsub.MinTopicSize(requiredMeshPeers)),
			pubsub.WithLocalPublication(true),
		)
		require.NoError(
			t,
			err,
			"server[%d] peer=%s did not reach routing readiness",
			i,
			servers[i].host.ID(),
		)
	}

	cancelReady()

	// Preserve a single publication through the production wrapper.
	err = serverTopics[0].Publish(&testproto.GenericMessage{
		Message: sentMessage,
	})
	require.NoError(t, err, "Unable to publish message")

	received := make([]bool, numServers)
	receivedCount := 0

	recordDelivery := func(result delivery) {
		t.Helper()

		require.NoError(
			t,
			result.err,
			"server[%d] peer=%s reported invalid delivery",
			result.node,
			servers[result.node].host.ID(),
		)

		if !received[result.node] {
			received[result.node] = true
			receivedCount++
		}
	}

	// One absolute deadline: additional callbacks do not restart it.
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()

	for receivedCount < numServers {
		select {
		case result := <-deliveries:
			recordDelivery(result)

		case <-timer.C:
			// Include results already queued when the deadline fired.
			for {
				select {
				case result := <-deliveries:
					recordDelivery(result)
				default:
					if receivedCount == numServers {
						return
					}

					missing := make([]string, 0)
					for i, delivered := range received {
						if !delivered {
							missing = append(missing, fmt.Sprintf(
								"server[%d] peer=%s topicPeers=%d/%d",
								i,
								servers[i].host.ID(),
								NumSubscribers(servers[i], topicName),
								numServers-1,
							))
						}
					}

					t.Fatalf(
						"gossip delivery timed out: received %d/%d; missing: %v",
						receivedCount,
						numServers,
						missing,
					)
				}
			}
		}
	}
}

/* FINISH ALPHA CHANGE 3880 -- verify routing readiness and distinct gossip recipients with safe cleanup -- */

func Test_RepeatedClose(t *testing.T) {
	topic := &Topic{
		closeCh: make(chan struct{}),
	}

	// Call Close() twice to ensure that underlying logic (e.g. channel close) is
	// only executed once.
	topic.Close()
	topic.Close()
}

/* START ALPHA CHANGE 1908 -- test bounded and joined gossip processing -- */
func newTopicHandlerTestJob(t *testing.T, value string) topicHandlerJob {
	t.Helper()

	data, err := proto.Marshal(&testproto.GenericMessage{
		Message: value,
	})
	require.NoError(t, err)

	return topicHandlerJob{
		data: data,
		from: peer.ID("test-peer"),
	}
}

func newTopicHandlerTestTopic() *Topic {
	topic := &Topic{
		typ:     reflect.TypeFor[testproto.GenericMessage](),
		closeCh: make(chan struct{}),
	}
	topic.closed.Store(false)

	return topic
}

func TestTopicHandlerConcurrencyIsBounded(t *testing.T) {
	topic := newTopicHandlerTestTopic()
	workerCount := topicHandlerWorkerCount()
	workQueue := make(chan topicHandlerJob, topicHandlerQueueSize)
	releaseHandlers := make(chan struct{})
	startedHandlers := make(chan struct{}, workerCount*2)

	var (
		activeHandlers atomic.Int64
		maxHandlers    atomic.Int64
		cleanupOnce    sync.Once
	)

	cleanup := func() {
		cleanupOnce.Do(func() {
			close(releaseHandlers)
			close(workQueue)
		})

		topic.waitGroup.Wait()
	}
	defer cleanup()

	handler := func(_ any, _ peer.ID) {
		active := activeHandlers.Add(1)

		for {
			previousMaximum := maxHandlers.Load()
			if active <= previousMaximum ||
				maxHandlers.CompareAndSwap(previousMaximum, active) {
				break
			}
		}

		startedHandlers <- struct{}{}
		<-releaseHandlers
		activeHandlers.Add(-1)
	}

	topic.waitGroup.Add(workerCount)
	for range workerCount {
		go topic.runHandlerWorker(workQueue, handler)
	}

	for index := range workerCount * 2 {
		workQueue <- newTopicHandlerTestJob(t, fmt.Sprintf("message-%d", index))
	}

	for range workerCount {
		select {
		case <-startedHandlers:
		case <-time.After(time.Second):
			t.Fatal("not all gossip workers started")
		}
	}

	select {
	case <-startedHandlers:
		t.Fatal("gossip processing exceeded the configured worker limit")
	case <-time.After(100 * time.Millisecond):
	}

	cleanup()

	require.Equal(t, int64(workerCount), maxHandlers.Load())
	require.Zero(t, activeHandlers.Load())
}

func TestTopicHandlerQueueAppliesBackpressure(t *testing.T) {
	topic := newTopicHandlerTestTopic()
	workQueue := make(chan topicHandlerJob, 1)
	workQueue <- newTopicHandlerTestJob(t, "first")

	ctx := t.Context()

	enqueued := make(chan bool, 1)
	go func() {
		enqueued <- topic.enqueueHandlerJob(
			ctx,
			workQueue,
			newTopicHandlerTestJob(t, "second"),
		)
	}()

	select {
	case <-enqueued:
		t.Fatal("enqueue returned while the bounded queue was full")
	case <-time.After(100 * time.Millisecond):
	}

	<-workQueue

	select {
	case result := <-enqueued:
		require.True(t, result)
	case <-time.After(time.Second):
		t.Fatal("enqueue did not resume after queue capacity became available")
	}
}

func TestTopicCloseWaitsForActiveHandlerAndDiscardsQueuedWork(t *testing.T) {
	topic := newTopicHandlerTestTopic()
	workQueue := make(chan topicHandlerJob, 2)
	handlerEntered := make(chan struct{}, 1)
	releaseHandler := make(chan struct{})
	closeReturned := make(chan struct{})
	var (
		handlerCalls atomic.Int64
		releaseOnce  sync.Once
	)

	defer releaseOnce.Do(func() {
		close(releaseHandler)
	})

	handler := func(_ any, _ peer.ID) {
		handlerCalls.Add(1)
		handlerEntered <- struct{}{}
		<-releaseHandler
	}

	topic.waitGroup.Add(1)
	go topic.runHandlerWorker(workQueue, handler)

	workQueue <- newTopicHandlerTestJob(t, "active")
	workQueue <- newTopicHandlerTestJob(t, "queued")

	select {
	case <-handlerEntered:
	case <-time.After(time.Second):
		t.Fatal("gossip handler did not start")
	}

	go func() {
		topic.Close()
		close(closeReturned)
	}()

	select {
	case <-closeReturned:
		t.Fatal("Topic.Close returned while a handler was still active")
	case <-time.After(100 * time.Millisecond):
	}

	releaseOnce.Do(func() {
		close(releaseHandler)
	})

	select {
	case <-closeReturned:
	case <-time.After(time.Second):
		t.Fatal("Topic.Close did not join the active gossip handler")
	}

	require.Equal(t, int64(1), handlerCalls.Load())
}

func TestTopicRejectsSubscriptionAfterClose(t *testing.T) {
	topic := newTopicHandlerTestTopic()
	topic.Close()

	err := topic.Subscribe(func(_ any, _ peer.ID) {})

	require.ErrorIs(t, err, ErrTopicClosed)
}

/* FINISH ALPHA CHANGE 1908 -- test bounded and joined gossip processing -- */
