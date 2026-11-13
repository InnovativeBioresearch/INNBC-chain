/* START ALPHA CHANGE 2893 -- observe actual GossipSub delivery and oversized drops on loopback -- */

package innbcbft

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	ibftproto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"
)

type sizeRPCTracer struct {
	target    peer.ID
	payload   []byte
	ready     chan struct{}
	readyOnce sync.Once
	sent      chan int
	dropped   chan int
}

var _ pubsub.RawTracer = (*sizeRPCTracer)(nil)

func (*sizeRPCTracer) AddPeer(peer.ID, protocol.ID)          {}
func (*sizeRPCTracer) RemovePeer(peer.ID)                    {}
func (*sizeRPCTracer) Join(string)                           {}
func (*sizeRPCTracer) Leave(string)                          {}
func (*sizeRPCTracer) Prune(peer.ID, string)                 {}
func (*sizeRPCTracer) ValidateMessage(*pubsub.Message)       {}
func (*sizeRPCTracer) DeliverMessage(*pubsub.Message)        {}
func (*sizeRPCTracer) RejectMessage(*pubsub.Message, string) {}
func (*sizeRPCTracer) DuplicateMessage(*pubsub.Message)      {}
func (*sizeRPCTracer) ThrottlePeer(peer.ID)                  {}
func (*sizeRPCTracer) RecvRPC(*pubsub.RPC)                   {}
func (*sizeRPCTracer) UndeliverableMessage(*pubsub.Message)  {}

func (tracer *sizeRPCTracer) Graft(id peer.ID, topic string) {
	if id == tracer.target && topic == pbftProto {
		tracer.readyOnce.Do(func() { close(tracer.ready) })
	}
}

func (tracer *sizeRPCTracer) record(
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

func (tracer *sizeRPCTracer) SendRPC(rpc *pubsub.RPC, id peer.ID) {
	tracer.record(rpc, id, tracer.sent)
}

func (tracer *sizeRPCTracer) DropRPC(rpc *pubsub.RPC, id peer.ID) {
	tracer.record(rpc, id, tracer.dropped)
}

func sizeWaitRPCObservation(
	t *testing.T,
	observations <-chan int,
	description string,
) int {
	t.Helper()

	timer := time.NewTimer(sizeWatchdog)
	defer timer.Stop()

	select {
	case size := <-observations:
		return size
	case <-timer.C:
		t.Fatal(description)
		return 0
	}
}

func sizeLoopbackDelivery(
	t *testing.T,
	fixture *sizeFixture,
	candidate *ibftproto.Message,
	expectDelivery bool,
) {
	t.Helper()

	wire := sizeLogMessage(t, "loopback_candidate", fixture, candidate)
	if expectDelivery {
		require.LessOrEqual(t, wire.rpcBytes, pubsub.DefaultMaxMessageSize)
	} else {
		require.Greater(t, wire.rpcBytes, pubsub.DefaultMaxMessageSize)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	var hosts []host.Host
	var topics []*pubsub.Topic
	var subscriptions []*pubsub.Subscription

	/* Register cleanup before creating resources, including partial construction. */
	t.Cleanup(func() {
		cancel()
		for _, subscription := range subscriptions {
			subscription.Cancel()
		}
		for _, topic := range topics {
			_ = topic.Close()
		}
		for _, node := range hosts {
			require.NoError(t, node.Close())
		}
	})

	sender, err := libp2p.New(
		libp2p.Identity(sizeLibp2pKey(t, 31)),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	hosts = append(hosts, sender)

	receiver, err := libp2p.New(
		libp2p.Identity(sizeLibp2pKey(t, 32)),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	hosts = append(hosts, receiver)

	tracer := &sizeRPCTracer{
		target:  receiver.ID(),
		payload: wire.payload,
		ready:   make(chan struct{}),
		sent:    make(chan int, 8),
		dropped: make(chan int, 8),
	}

	/* Intentionally retain the dependency's unchanged default size limit. */
	senderPubsub, err := pubsub.NewGossipSub(ctx, sender, pubsub.WithRawTracer(tracer))
	require.NoError(t, err)
	receiverPubsub, err := pubsub.NewGossipSub(ctx, receiver)
	require.NoError(t, err)

	senderTopic, err := senderPubsub.Join(pbftProto)
	require.NoError(t, err)
	topics = append(topics, senderTopic)

	senderSubscription, err := senderTopic.Subscribe()
	require.NoError(t, err)
	subscriptions = append(subscriptions, senderSubscription)

	receiverTopic, err := receiverPubsub.Join(pbftProto)
	require.NoError(t, err)
	topics = append(topics, receiverTopic)

	receiverSubscription, err := receiverTopic.Subscribe()
	require.NoError(t, err)
	subscriptions = append(subscriptions, receiverSubscription)

	require.NoError(t, sender.Connect(ctx, peer.AddrInfo{
		ID:    receiver.ID(),
		Addrs: receiver.Addrs(),
	}))
	sizeWaitSignal(t, tracer.ready, "loopback GossipSub mesh did not become ready")

	/*
		This is the same protobuf-to-Topic.Publish boundary used by the
		application. The fixture isolates transport sizing from admission.
	*/
	require.NoError(t, senderTopic.Publish(ctx, wire.payload))

	if expectDelivery {
		observedSize := sizeWaitRPCObservation(t, tracer.sent,
			"the candidate was never sent to the remote peer")
		require.LessOrEqual(t, observedSize, pubsub.DefaultMaxMessageSize)

		received, receiveErr := receiverSubscription.Next(ctx)
		require.NoError(t, receiveErr)
		require.Equal(t, wire.payload, received.Data)
		require.Equal(t, sender.ID(), received.GetFrom())

		decoded := new(ibftproto.Message)
		require.NoError(t, protobuf.Unmarshal(received.Data, decoded))
		require.True(t, fixture.runtimes[0].IsValidValidator(decoded))
		require.True(t, isStructurallyValidIBFTMessage(decoded))
		require.True(t, fixture.runtimes[0].IsValidProposalHash(
			decoded.GetPreprepareData().Proposal,
			decoded.GetPreprepareData().ProposalHash,
		))

		t.Logf("remote delivery succeeded; observed signed RPC bytes=%d limit=%d",
			observedSize, pubsub.DefaultMaxMessageSize)
		return
	}

	droppedSize := sizeWaitRPCObservation(t, tracer.dropped,
		"the oversized candidate did not produce an observed outbound drop")
	require.Greater(t, droppedSize, pubsub.DefaultMaxMessageSize)

	/*
		A positive barrier delivery establishes continued transport usability.
		Do not infer oversized rejection merely from a subscriber timeout.
	*/
	barrier := []byte("consensus-size-test-after-observed-drop")
	require.NoError(t, senderTopic.Publish(ctx, barrier))

	received, receiveErr := receiverSubscription.Next(ctx)
	require.NoError(t, receiveErr)
	require.NotEqual(t, wire.payload, received.Data)
	require.Equal(t, barrier, received.Data)

	t.Logf("oversized RPC dropped; observed bytes=%d limit=%d; subsequent barrier delivered",
		droppedSize, pubsub.DefaultMaxMessageSize)
}

func TestConsensusWireTransport_DefaultLimit(t *testing.T) {
	t.Run("fresh_below_limit", func(t *testing.T) {
		fixture := newSizeFixture(t, 4, 9, 116_000)
		candidate := fixture.proposal(t, 0, nil)
		sizeAcceptProposal(t, fixture, candidate)
		sizeLoopbackDelivery(t, fixture, candidate, true)
	})

	t.Run("fresh_above_limit", func(t *testing.T) {
		fixture := newSizeFixture(t, 4, 9, 117_000)
		candidate := fixture.proposal(t, 0, nil)
		sizeAcceptProposal(t, fixture, candidate)
		sizeLoopbackDelivery(t, fixture, candidate, false)
	})

	t.Run("prepared_reproposal_above_limit", func(t *testing.T) {
		fixture := newSizeFixture(t, 4, 2, 100_000)
		initial := fixture.proposal(t, 0, nil)
		results := sizePrepareAndAdvance(t, fixture, initial, 1)

		candidate := fixture.proposal(t, 1, sizeCanonicalPC(t, results[0]))
		require.Equal(t, 7, sizeBlockCopies(t, candidate, fixture.raw))
		sizeAcceptProposal(t, fixture, candidate)
		sizeLoopbackDelivery(t, fixture, candidate, false)
	})
}

/* FINISH ALPHA CHANGE 2893 -- observe actual GossipSub delivery and oversized drops on loopback -- */
