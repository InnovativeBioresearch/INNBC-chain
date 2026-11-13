/* START ALPHA CHANGE 2890 -- regress GossipSub cancellation construction cleanup and topic shutdown ordering -- */
package network

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	peerEvent "github.com/innovativebioresearch/innbc-chain/network/event"
	testproto "github.com/innovativebioresearch/innbc-chain/network/proto"
	"github.com/innovativebioresearch/innbc-chain/secrets"
	"github.com/innovativebioresearch/innbc-chain/secrets/local"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

const gossipLifetimeWatchdog = 20 * time.Second

func gossipLifetimeWait(t *testing.T, done <-chan struct{}, description string) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(gossipLifetimeWatchdog):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func gossipLifetimeCloseServer(t *testing.T, s *Server) {
	t.Helper()

	result := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		result <- s.Close()
	}()

	gossipLifetimeWait(t, done, "network server closure")
	require.NoError(t, <-result)
}

func gossipLifetimeCloseTopic(t *testing.T, topic *Topic) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		topic.Close()
	}()

	gossipLifetimeWait(t, done, "application topic closure")
}

func gossipLifetimeConfig(t *testing.T) *Config {
	t.Helper()

	cfg := DefaultConfig()
	cfg.Addr.Port = 0
	cfg.NoDiscover = true
	cfg.PeerDirectoryEnabled = false
	cfg.DataDir = t.TempDir()
	cfg.Chain = &chain.Chain{
		Params: &chain.Params{ChainID: 1},
	}

	/* START ALPHA CHANGE 3235 -- preserve gossip lifecycle fixtures with the local-only factory signature -- */
	manager, err := local.SecretsManagerFactory(
		&secrets.SecretsManagerParams{
			Logger: hclog.NewNullLogger(),
			Extra: map[string]interface{}{
				secrets.Path: cfg.DataDir,
			},
		},
	)
	/* FINISH ALPHA CHANGE 3235 -- preserve gossip lifecycle fixtures with the local-only factory signature -- */
	require.NoError(t, err)
	cfg.SecretsManager = manager

	return cfg
}

func gossipLifetimeNewEmitter(h host.Host) (event.Emitter, error) {
	return h.EventBus().Emitter(new(peerEvent.PeerEvent))
}

func gossipLifetimeServer(t *testing.T, started bool) (*Server, context.Context) {
	t.Helper()

	var constructionContext context.Context

	s, err := newServerWithGossipFactories(
		hclog.NewNullLogger(),
		gossipLifetimeConfig(t),
		gossipLifetimeNewEmitter,
		func(
			ctx context.Context,
			h host.Host,
			options ...pubsub.Option,
		) (*pubsub.PubSub, error) {
			constructionContext = ctx

			return pubsub.NewGossipSub(ctx, h, options...)
		},
	)
	require.NoError(t, err)
	require.NotNil(t, constructionContext)
	require.NoError(t, constructionContext.Err())

	t.Cleanup(func() {
		gossipLifetimeCloseServer(t, s)
		require.NoError(t, s.emitterPeerEvent.Close())
	})

	if started {
		require.NoError(t, s.Start())
	}

	return s, constructionContext
}

func TestGossipLifetime_LocalDeliveryBeforeShutdown(t *testing.T) {
	s, constructionContext := gossipLifetimeServer(t, true)

	topic, err := s.NewTopic(
		"gossip-lifetime-local-delivery",
		&testproto.GenericMessage{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { gossipLifetimeCloseTopic(t, topic) })

	delivered := make(chan string, 1)
	require.NoError(t, topic.Subscribe(func(obj any, _ peer.ID) {
		message, ok := obj.(*testproto.GenericMessage)
		if !ok {
			delivered <- "unexpected message type"

			return
		}

		delivered <- message.Message
	}))

	require.NoError(t, topic.Publish(&testproto.GenericMessage{
		Message: "delivery-before-shutdown",
	}))

	select {
	case value := <-delivered:
		require.Equal(t, "delivery-before-shutdown", value)
	case <-time.After(gossipLifetimeWatchdog):
		t.Fatal("local GossipSub message did not reach the application callback")
	}

	require.NoError(t, constructionContext.Err())
	gossipLifetimeCloseTopic(t, topic)
	gossipLifetimeCloseServer(t, s)
	require.ErrorIs(t, constructionContext.Err(), context.Canceled)
}

func TestGossipLifetime_CloseCancelsConstructionContext(t *testing.T) {
	for _, started := range []bool{false, true} {
		name := "never_started"
		if started {
			name = "started"
		}

		t.Run(name, func(t *testing.T) {
			s, constructionContext := gossipLifetimeServer(t, started)

			gossipLifetimeCloseServer(t, s)
			gossipLifetimeWait(
				t,
				constructionContext.Done(),
				"cancellation of the context supplied to GossipSub",
			)
			require.ErrorIs(t, constructionContext.Err(), context.Canceled)
			require.Empty(t, s.host.Network().ListenAddresses())

			gossipLifetimeCloseServer(t, s)
			require.ErrorIs(t, constructionContext.Err(), context.Canceled)
		})
	}
}

func TestGossipLifetime_ConcurrentClose(t *testing.T) {
	s, constructionContext := gossipLifetimeServer(t, true)

	const callers = 16
	results := make(chan error, callers)
	start := make(chan struct{})
	joined := make(chan struct{})
	var workers sync.WaitGroup

	workers.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer workers.Done()
			<-start
			results <- s.Close()
		}()
	}

	go func() {
		workers.Wait()
		close(joined)
	}()

	close(start)
	gossipLifetimeWait(t, joined, "all concurrent Close callers")

	for i := 0; i < callers; i++ {
		require.NoError(t, <-results)
	}

	require.ErrorIs(t, constructionContext.Err(), context.Canceled)
	require.Empty(t, s.host.Network().ListenAddresses())
}

func TestGossipLifetime_TopicCleanupBeforeAndAfterCancellation(t *testing.T) {
	for _, cancelFirst := range []bool{false, true} {
		name := "topic_first"
		if cancelFirst {
			name = "server_first"
		}

		t.Run(name, func(t *testing.T) {
			s, constructionContext := gossipLifetimeServer(t, true)

			/*
				The session topic also exercises its owned validator
				unregistration during application topic cleanup.
			*/
			topic, err := s.NewPeerSessionTopic(
				"gossip-lifetime-session-topic",
				&testproto.GenericMessage{},
			)
			require.NoError(t, err)
			t.Cleanup(func() { gossipLifetimeCloseTopic(t, topic) })

			require.NoError(t, topic.Subscribe(func(any, peer.ID) {}))

			if cancelFirst {
				gossipLifetimeCloseServer(t, s)
				require.ErrorIs(t, constructionContext.Err(), context.Canceled)
				gossipLifetimeCloseTopic(t, topic)
			} else {
				gossipLifetimeCloseTopic(t, topic)
				require.NoError(t, constructionContext.Err())
				gossipLifetimeCloseServer(t, s)
			}

			require.True(t, topic.closed.Load())
			require.Nil(t, topic.topic)
			require.Nil(t, topic.closeValidation)
			require.ErrorIs(t, constructionContext.Err(), context.Canceled)

			gossipLifetimeCloseTopic(t, topic)
		})
	}
}

func TestGossipLifetime_InProgressValidatorObservesCancellation(t *testing.T) {
	s, constructionContext := gossipLifetimeServer(t, true)

	const topicName = "gossip-lifetime-controlled-validator"
	entered := make(chan context.Context, 1)
	exited := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once

	require.NoError(t, s.ps.RegisterTopicValidator(
		topicName,
		func(ctx context.Context, _ peer.ID, _ *pubsub.Message) bool {
			defer close(exited)
			entered <- ctx

			select {
			case <-ctx.Done():
			case <-release:
			}

			return false
		},
		pubsub.WithValidatorInline(true),
	))

	topic, err := s.NewTopic(topicName, &testproto.GenericMessage{})
	require.NoError(t, err)
	require.NoError(t, topic.Subscribe(func(any, peer.ID) {}))

	publishResult := make(chan error, 1)
	publishDone := make(chan struct{})

	/*
		Release the controlled validator before joining publication or
		closing its topic if an earlier assertion fails.
	*/
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		s.gossipCancel()
		gossipLifetimeWait(t, publishDone, "controlled publication cleanup")
		gossipLifetimeCloseTopic(t, topic)
	})

	go func() {
		defer close(publishDone)
		publishResult <- topic.Publish(&testproto.GenericMessage{
			Message: "controlled-validation",
		})
	}()

	select {
	case validatorContext := <-entered:
		require.True(
			t,
			validatorContext == constructionContext,
			"validator must observe the exact context supplied to GossipSub",
		)
	case <-time.After(gossipLifetimeWatchdog):
		t.Fatal("publication did not enter the controlled validator")
	}

	gossipLifetimeCloseServer(t, s)
	gossipLifetimeWait(t, exited, "validator return after server cancellation")
	gossipLifetimeWait(t, publishDone, "publication return after validator cancellation")
	require.Error(t, <-publishResult)
	require.ErrorIs(t, constructionContext.Err(), context.Canceled)

	gossipLifetimeCloseTopic(t, topic)
}

func TestGossipLifetime_CancellationPrecedesWorkerJoin(t *testing.T) {
	s, constructionContext := gossipLifetimeServer(t, false)

	_, finish, err := s.rpcLifetime.Begin(context.Background())
	require.NoError(t, err)

	release := make(chan struct{})
	exited := make(chan struct{})
	var releaseOnce sync.Once

	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		gossipLifetimeWait(t, exited, "controlled admitted worker cleanup")
	})

	go func() {
		defer close(exited)
		defer finish()

		select {
		case <-constructionContext.Done():
		case <-release:
		}
	}()

	/*
		The admitted worker finishes only after GossipSub cancellation.
		Waiting for this worker before cancellation would deadlock Close.
	*/
	gossipLifetimeCloseServer(t, s)
	gossipLifetimeWait(t, exited, "worker released by GossipSub cancellation")
	require.ErrorIs(t, constructionContext.Err(), context.Canceled)
}

type gossipLifetimeTrackedEmitter struct {
	event.Emitter
	closes                atomic.Int32
	canceledBeforeClosure atomic.Bool
	context               context.Context
}

func (e *gossipLifetimeTrackedEmitter) Close() error {
	e.closes.Add(1)
	e.canceledBeforeClosure.Store(
		e.context != nil && errors.Is(e.context.Err(), context.Canceled),
	)

	return e.Emitter.Close()
}

func TestGossipLifetime_ConstructorFailuresReleaseResources(t *testing.T) {
	for _, failEmitter := range []bool{true, false} {
		name := "gossip_construction"
		if failEmitter {
			name = "emitter_construction"
		}

		t.Run(name, func(t *testing.T) {
			expected := errors.New("controlled construction failure")
			var createdHost host.Host
			var constructionContext context.Context
			var trackedEmitter *gossipLifetimeTrackedEmitter

			t.Cleanup(func() {
				if trackedEmitter != nil && trackedEmitter.closes.Load() == 0 {
					require.NoError(t, trackedEmitter.Close())
				}
				if createdHost != nil {
					require.NoError(t, createdHost.Close())
				}
			})

			s, err := newServerWithGossipFactories(
				hclog.NewNullLogger(),
				gossipLifetimeConfig(t),
				func(h host.Host) (event.Emitter, error) {
					createdHost = h
					require.NotEmpty(t, h.Network().ListenAddresses())

					if failEmitter {
						return nil, expected
					}

					emitter, err := gossipLifetimeNewEmitter(h)
					if err != nil {
						return nil, err
					}

					trackedEmitter = &gossipLifetimeTrackedEmitter{
						Emitter: emitter,
					}

					return trackedEmitter, nil
				},
				func(
					ctx context.Context,
					h host.Host,
					options ...pubsub.Option,
				) (*pubsub.PubSub, error) {
					constructionContext = ctx
					trackedEmitter.context = ctx

					/*
						Exercise an actual GossipSub constructor failure,
						rather than returning before the dependency is called.
					*/
					options = append(options, func(*pubsub.PubSub) error {
						return expected
					})

					return pubsub.NewGossipSub(ctx, h, options...)
				},
			)

			require.Nil(t, s)
			require.ErrorIs(t, err, expected)
			require.NotNil(t, createdHost)
			require.Empty(
				t,
				createdHost.Network().ListenAddresses(),
				"failed construction must release the listening host",
			)

			if failEmitter {
				require.Nil(t, constructionContext)
				require.Nil(t, trackedEmitter)

				return
			}

			require.NotNil(t, constructionContext)
			require.ErrorIs(t, constructionContext.Err(), context.Canceled)
			require.Equal(t, int32(1), trackedEmitter.closes.Load())
			require.True(
				t,
				trackedEmitter.canceledBeforeClosure.Load(),
				"GossipSub cancellation must precede emitter cleanup",
			)
		})
	}
}

func TestGossipLifetime_RepeatedConstructionWithoutStart(t *testing.T) {
	var previous context.Context

	for i := 0; i < 3; i++ {
		s, constructionContext := gossipLifetimeServer(t, false)

		require.NoError(t, constructionContext.Err())
		if previous != nil {
			require.ErrorIs(t, previous.Err(), context.Canceled)
			require.True(t, previous != constructionContext)
		}

		gossipLifetimeCloseServer(t, s)
		require.ErrorIs(t, constructionContext.Err(), context.Canceled)
		require.Empty(t, s.host.Network().ListenAddresses())

		previous = constructionContext
	}
}

/* FINISH ALPHA CHANGE 2890 -- regress GossipSub cancellation construction cleanup and topic shutdown ordering -- */
