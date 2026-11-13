/* START ALPHA CHANGE 2898 -- verify signed consensus delivery under the interim four MiB limit -- */

package innbcbft

import (
	"context"
	"testing"
	"time"

	ibftproto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"
)

const sizeInterimGossipLimit = 4 * 1024 * 1024

func sizeInterimDelivery(
	t *testing.T,
	fixture *sizeFixture,
	candidate *ibftproto.Message,
) {
	t.Helper()

	wire := sizeLogMessage(t, "interim_limit_candidate", fixture, candidate)
	require.Greater(t, wire.rpcBytes, pubsub.DefaultMaxMessageSize)
	require.LessOrEqual(t, wire.rpcBytes, sizeInterimGossipLimit)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	var hosts []host.Host
	var topics []*pubsub.Topic
	var subscriptions []*pubsub.Subscription

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

	for _, scalar := range []byte{31, 32} {
		node, err := libp2p.New(
			libp2p.Identity(sizeLibp2pKey(t, scalar)),
			libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
			libp2p.DisableRelay(),
		)
		require.NoError(t, err)
		hosts = append(hosts, node)
	}
	sender, receiver := hosts[0], hosts[1]

	tracer := &sizeRPCTracer{
		target:  receiver.ID(),
		payload: wire.payload,
		ready:   make(chan struct{}),
		sent:    make(chan int, 8),
		dropped: make(chan int, 8),
	}

	senderPubsub, err := pubsub.NewGossipSub(
		ctx,
		sender,
		pubsub.WithMaxMessageSize(sizeInterimGossipLimit),
		pubsub.WithRawTracer(tracer),
	)
	require.NoError(t, err)

	receiverPubsub, err := pubsub.NewGossipSub(
		ctx,
		receiver,
		pubsub.WithMaxMessageSize(sizeInterimGossipLimit),
	)
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
	sizeWaitSignal(t, tracer.ready, "interim-limit GossipSub mesh did not become ready")

	require.NoError(t, senderTopic.Publish(ctx, wire.payload))
	observedSize := sizeWaitRPCObservation(
		t,
		tracer.sent,
		"the signed proposal was not sent under the interim limit",
	)
	require.Greater(t, observedSize, pubsub.DefaultMaxMessageSize)
	require.LessOrEqual(t, observedSize, sizeInterimGossipLimit)

	received, err := receiverSubscription.Next(ctx)
	require.NoError(t, err)
	require.Equal(t, sender.ID(), received.GetFrom())
	require.Equal(t, wire.payload, received.Data)

	decoded := new(ibftproto.Message)
	require.NoError(t, protobuf.Unmarshal(received.Data, decoded))
	require.True(t, fixture.runtimes[0].IsValidValidator(decoded))
	require.True(t, isStructurallyValidIBFTMessage(decoded))
	require.True(t, fixture.runtimes[0].IsValidProposalHash(
		decoded.GetPreprepareData().Proposal,
		decoded.GetPreprepareData().ProposalHash,
	))

	t.Logf(
		"formerly oversized signed proposal delivered; observed_rpc=%d interim_limit=%d",
		observedSize, sizeInterimGossipLimit,
	)
}

func TestConsensusWireTransport_InterimLimit(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		validators   int
		transactions int
		data         int
		round        int
		copies       int
	}{
		{
			name:         "fresh_large_block",
			validators:   4,
			transactions: 9,
			data:         120_000,
			round:        0,
			copies:       1,
		},
		{
			name:         "first_prepared_reproposal",
			validators:   4,
			transactions: 2,
			data:         100_000,
			round:        1,
			copies:       7,
		},
		{
			name:         "four_validators_third_prepared_reproposal",
			validators:   4,
			transactions: 1,
			data:         32 * 1024,
			round:        3,
			copies:       79,
		},
		{
			name:         "seven_validators_third_prepared_reproposal",
			validators:   7,
			transactions: 1,
			data:         4 * 1024,
			round:        3,
			copies:       311,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			require.LessOrEqual(t, testCase.round, sizeMaxReprepares)
			fixture := newSizeFixture(
				t,
				testCase.validators,
				testCase.transactions,
				testCase.data,
			)

			var pc *ibftproto.PreparedCertificate
			var candidate *ibftproto.Message

			for round := 0; round <= testCase.round; round++ {
				candidate = fixture.proposal(t, uint64(round), pc)
				if round < testCase.round {
					results := sizePrepareAndAdvance(t, fixture, candidate, 1)
					pc = sizeCanonicalPC(t, results[0])
				}
			}

			require.Equal(t, testCase.copies, sizeBlockCopies(t, candidate, fixture.raw))
			sizeAcceptProposal(t, fixture, candidate)
			sizeInterimDelivery(t, fixture, candidate)
		})
	}
}

/* FINISH ALPHA CHANGE 2898 -- verify signed consensus delivery under the interim four MiB limit -- */
