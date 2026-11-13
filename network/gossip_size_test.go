/* START ALPHA CHANGE 2897 -- verify the production GossipSub constructor limit and oversized drops -- */

package network

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	testproto "github.com/innovativebioresearch/innbc-chain/network/proto"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"
)

const gossipSizeTopic = "/pbft/0.2"

type gossipSizeTracer struct {
	target    peer.ID
	payload   []byte
	ready     chan struct{}
	readyOnce sync.Once
	sent      chan int
	dropped   chan int
}

var _ pubsub.RawTracer = (*gossipSizeTracer)(nil)

func (*gossipSizeTracer) AddPeer(peer.ID, protocol.ID)          {}
func (*gossipSizeTracer) RemovePeer(peer.ID)                    {}
func (*gossipSizeTracer) Join(string)                           {}
func (*gossipSizeTracer) Leave(string)                          {}
func (*gossipSizeTracer) Prune(peer.ID, string)                 {}
func (*gossipSizeTracer) ValidateMessage(*pubsub.Message)       {}
func (*gossipSizeTracer) DeliverMessage(*pubsub.Message)        {}
func (*gossipSizeTracer) RejectMessage(*pubsub.Message, string) {}
func (*gossipSizeTracer) DuplicateMessage(*pubsub.Message)      {}
func (*gossipSizeTracer) ThrottlePeer(peer.ID)                  {}
func (*gossipSizeTracer) RecvRPC(*pubsub.RPC)                   {}
func (*gossipSizeTracer) UndeliverableMessage(*pubsub.Message)  {}

func (tracer *gossipSizeTracer) Graft(id peer.ID, topic string) {
	if id == tracer.target && topic == gossipSizeTopic {
		tracer.readyOnce.Do(func() { close(tracer.ready) })
	}
}

func (tracer *gossipSizeTracer) record(
	rpc *pubsub.RPC,
	id peer.ID,
	results chan int,
) {
	if id != tracer.target {
		return
	}

	for _, message := range rpc.GetPublish() {
		if bytes.Equal(message.Data, tracer.payload) {
			select {
			case results <- rpc.Size():
			default:
			}
			return
		}
	}
}

func (tracer *gossipSizeTracer) SendRPC(rpc *pubsub.RPC, id peer.ID) {
	tracer.record(rpc, id, tracer.sent)
}

func (tracer *gossipSizeTracer) DropRPC(rpc *pubsub.RPC, id peer.ID) {
	tracer.record(rpc, id, tracer.dropped)
}

func gossipSizeObserve(
	t *testing.T,
	results <-chan int,
	description string,
) int {
	t.Helper()

	timer := time.NewTimer(gossipLifetimeWatchdog)
	defer timer.Stop()

	select {
	case size := <-results:
		return size
	case <-timer.C:
		t.Fatal(description)
		return 0
	}
}

func gossipSizeServer(t *testing.T, tracer pubsub.RawTracer) *Server {
	t.Helper()

	config := gossipLifetimeConfig(t)
	config.Addr.IP = net.ParseIP("127.0.0.1")

	server, err := newServerWithGossipFactories(
		hclog.NewNullLogger(),
		config,
		gossipLifetimeNewEmitter,
		func(
			ctx context.Context,
			node host.Host,
			options ...pubsub.Option,
		) (*pubsub.PubSub, error) {
			/*
				Preserve every production option. Add observation only;
				do not supply or override the message-size setting here.
			*/
			if tracer != nil {
				options = append(options, pubsub.WithRawTracer(tracer))
			}
			return pubsub.NewGossipSub(ctx, node, options...)
		},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		gossipLifetimeCloseServer(t, server)
		require.NoError(t, server.emitterPeerEvent.Close())
	})

	require.NoError(t, server.Start())
	return server
}

func gossipSizePayload(t *testing.T, target int) []byte {
	t.Helper()

	require.Greater(t, target, pubsub.DefaultMaxMessageSize)
	require.LessOrEqual(t, target, maxGossipMessageSize+1024)

	/*
		Use a bounded protobuf fixture to isolate the transport limit.
		Real signed consensus messages are tested separately.
	*/
	message := &testproto.GenericMessage{
		Message: strings.Repeat("x", target-16),
	}
	payload, err := protobuf.Marshal(message)
	require.NoError(t, err)

	padding := target - len(payload)
	require.GreaterOrEqual(t, padding, 0)
	require.LessOrEqual(t, padding, 16)

	message.Message += strings.Repeat("x", padding)
	payload, err = protobuf.Marshal(message)
	require.NoError(t, err)
	require.Len(t, payload, target)

	return payload
}

func TestGossipSize_ProductionConstructorLimit(t *testing.T) {
	require.Equal(t, 4*1024*1024, maxGossipMessageSize)

	for _, testCase := range []struct {
		name    string
		payload int
		deliver bool
	}{
		{
			name:    "formerly_oversized_payload_delivered",
			payload: 1_081_720,
			deliver: true,
		},
		{
			name:    "near_four_mib_delivered",
			payload: maxGossipMessageSize - 4096,
			deliver: true,
		},
		{
			name:    "above_four_mib_dropped",
			payload: maxGossipMessageSize + 1024,
			deliver: false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			payload := gossipSizePayload(t, testCase.payload)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			t.Cleanup(cancel)

			receiver := gossipSizeServer(t, nil)
			tracer := &gossipSizeTracer{
				target:  receiver.host.ID(),
				payload: payload,
				ready:   make(chan struct{}),
				sent:    make(chan int, 8),
				dropped: make(chan int, 8),
			}
			sender := gossipSizeServer(t, tracer)

			senderTopic, err := sender.ps.Join(gossipSizeTopic)
			require.NoError(t, err)
			t.Cleanup(func() { _ = senderTopic.Close() })

			senderSubscription, err := senderTopic.Subscribe()
			require.NoError(t, err)
			t.Cleanup(senderSubscription.Cancel)

			receiverTopic, err := receiver.ps.Join(gossipSizeTopic)
			require.NoError(t, err)
			t.Cleanup(func() { _ = receiverTopic.Close() })

			receiverSubscription, err := receiverTopic.Subscribe()
			require.NoError(t, err)
			t.Cleanup(receiverSubscription.Cancel)

			require.NoError(t, sender.host.Connect(ctx, peer.AddrInfo{
				ID:    receiver.host.ID(),
				Addrs: receiver.host.Addrs(),
			}))
			gossipLifetimeWait(t, tracer.ready, "production GossipSub mesh readiness")

			require.NoError(t, senderTopic.Publish(ctx, payload))

			if testCase.deliver {
				size := gossipSizeObserve(t, tracer.sent,
					"production constructor did not send the permitted message")
				require.Greater(t, size, pubsub.DefaultMaxMessageSize)
				require.LessOrEqual(t, size, maxGossipMessageSize)

				received, receiveErr := receiverSubscription.Next(ctx)
				require.NoError(t, receiveErr)
				require.Equal(t, payload, received.Data)
				require.Equal(t, sender.host.ID(), received.GetFrom())

				decoded := new(testproto.GenericMessage)
				require.NoError(t, protobuf.Unmarshal(received.Data, decoded))
				require.NotEmpty(t, decoded.Message)

				t.Logf(
					"production constructor delivered payload=%d signed_rpc=%d limit=%d",
					len(payload), size, maxGossipMessageSize,
				)
				return
			}

			size := gossipSizeObserve(t, tracer.dropped,
				"production constructor did not report the oversized outbound drop")
			require.Greater(t, size, maxGossipMessageSize)

			/*
				Require positive delivery after the observed drop.
				A receive timeout alone is not evidence of correct rejection.
			*/
			barrier := []byte("production-gossip-size-after-drop")
			require.NoError(t, senderTopic.Publish(ctx, barrier))

			received, receiveErr := receiverSubscription.Next(ctx)
			require.NoError(t, receiveErr)
			require.Equal(t, barrier, received.Data)
			require.Equal(t, sender.host.ID(), received.GetFrom())

			t.Logf(
				"production constructor dropped signed_rpc=%d limit=%d; barrier delivered",
				size, maxGossipMessageSize,
			)
		})
	}
}

/* FINISH ALPHA CHANGE 2897 -- verify the production GossipSub constructor limit and oversized drops -- */
