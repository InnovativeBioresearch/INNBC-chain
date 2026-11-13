/* START ALPHA CHANGE 1896 -- test bounded views, weak quorum, and timeout saturation -- */
package ibftcore

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/stretchr/testify/require"
)

func TestIsViewWithinBounds(t *testing.T) {
	t.Parallel()

	current := &proto.View{Height: 100, Round: 6}

	testCases := []struct {
		name      string
		candidate *proto.View
		accepted  bool
	}{
		{
			name:      "current view",
			candidate: &proto.View{Height: 100, Round: 6},
			accepted:  true,
		},
		{
			name:      "highest admitted current-height round",
			candidate: &proto.View{Height: 100, Round: 14},
			accepted:  true,
		},
		{
			name:      "round beyond window",
			candidate: &proto.View{Height: 100, Round: 15},
			accepted:  false,
		},
		{
			name:      "stale round",
			candidate: &proto.View{Height: 100, Round: 5},
			accepted:  false,
		},
		{
			name:      "next height round zero",
			candidate: &proto.View{Height: 101, Round: 0},
			accepted:  true,
		},
		{
			name:      "next height future round",
			candidate: &proto.View{Height: 101, Round: 1},
			accepted:  false,
		},
		{
			name:      "distant future height",
			candidate: &proto.View{Height: 102, Round: 0},
			accepted:  false,
		},
		{
			name:      "stale height",
			candidate: &proto.View{Height: 99, Round: 100},
			accepted:  false,
		},
		{
			name:      "nil candidate",
			candidate: nil,
			accepted:  false,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(
				t,
				testCase.accepted,
				isViewWithinBounds(current, testCase.candidate),
			)
		})
	}
}

func TestValidatorManagerHasWeakQuorum(t *testing.T) {
	t.Parallel()

	manager := NewValidatorManager(nil, nil)
	require.NoError(t, manager.setCurrentVotingPower(map[string]*big.Int{
		"A": big.NewInt(1),
		"B": big.NewInt(1),
		"C": big.NewInt(1),
		"D": big.NewInt(1),
	}))

	require.False(t, manager.HasWeakQuorum(map[string]struct{}{
		"A": {},
	}))
	require.True(t, manager.HasWeakQuorum(map[string]struct{}{
		"A": {},
		"B": {},
	}))
	require.False(t, manager.HasQuorum(map[string]struct{}{
		"A": {},
		"B": {},
	}))
	require.True(t, manager.HasQuorum(map[string]struct{}{
		"A": {},
		"B": {},
		"C": {},
	}))
}

func TestGetRoundTimeoutSaturates(t *testing.T) {
	t.Parallel()

	require.Equal(
		t,
		10*time.Second,
		getRoundTimeout(10*time.Second, 0, 0),
	)
	require.Equal(
		t,
		80*time.Second,
		getRoundTimeout(10*time.Second, 0, 3),
	)
	require.Equal(
		t,
		maxRoundTimeout,
		getRoundTimeout(10*time.Second, 0, 63),
	)
}

/* FINISH ALPHA CHANGE 1896 -- test bounded views, weak quorum, and timeout saturation -- */

/* START ALPHA CHANGE 1898 -- test authenticated future-round certificate selection -- */
type roundChangePolicyBackend struct {
	Backend

	validators map[string]struct{}
}

func (b *roundChangePolicyBackend) IsValidValidator(message *proto.Message) bool {
	if message == nil {
		return false
	}

	_, exists := b.validators[string(message.From)]

	return exists
}

/* START ALPHA CHANGE 1903 -- exercise live bounded round-catch-up control flow -- */
var (
	roundChangePolicyProposal     = []byte("proposal")
	roundChangePolicyProposalHash = []byte("proposal-hash")
)

func (b *roundChangePolicyBackend) IsValidProposal(rawProposal []byte) bool {
	return bytes.Equal(rawProposal, roundChangePolicyProposal)
}

/* START ALPHA CHANGE 2587 -- provide execution handles in the controlled consensus backend -- */
func (b *roundChangePolicyBackend) ExecuteProposal(raw []byte) *ProposalExecution {
	if !b.IsValidProposal(raw) {
		return nil
	}

	return &ProposalExecution{Value: append([]byte(nil), raw...)}
}

func (b *roundChangePolicyBackend) IsCurrentExecution(
	raw []byte,
	execution *ProposalExecution,
) bool {
	if execution == nil {
		return false
	}

	executedRaw, ok := execution.Value.([]byte)

	return ok && bytes.Equal(raw, executedRaw)
}

func (b *roundChangePolicyBackend) BuildExecutedProposal(
	_ *proto.View,
) ([]byte, *ProposalExecution) {
	raw := append([]byte(nil), roundChangePolicyProposal...)

	return raw, &ProposalExecution{Value: append([]byte(nil), raw...)}
}

func (b *roundChangePolicyBackend) InsertProposal(
	_ *proto.Proposal,
	_ []*messages.CommittedSeal,
	_ *ProposalExecution,
) error {
	return nil
}

/* FINISH ALPHA CHANGE 2587 -- provide execution handles in the controlled consensus backend -- */

func (b *roundChangePolicyBackend) IsProposer(
	id []byte,
	_ uint64,
	_ uint64,
) bool {
	return bytes.Equal(id, []byte("A"))
}

func (b *roundChangePolicyBackend) IsValidProposalHash(
	proposal *proto.Proposal,
	hash []byte,
) bool {
	return proposal != nil &&
		bytes.Equal(proposal.RawProposal, roundChangePolicyProposal) &&
		bytes.Equal(hash, roundChangePolicyProposalHash)
}

func (b *roundChangePolicyBackend) ID() []byte {
	return []byte("local")
}

func (b *roundChangePolicyBackend) BuildRoundChangeMessage(
	proposal *proto.Proposal,
	certificate *proto.PreparedCertificate,
	view *proto.View,
) *proto.Message {
	return &proto.Message{
		From: []byte("local"),
		View: view,
		Type: proto.MessageType_ROUND_CHANGE,
		Payload: &proto.Message_RoundChangeData{
			RoundChangeData: &proto.RoundChangeMessage{
				LastPreparedProposal:      proposal,
				LatestPreparedCertificate: certificate,
			},
		},
	}
}

type roundChangePolicyTransport struct {
	messages []*proto.Message
}

func (t *roundChangePolicyTransport) Multicast(message *proto.Message) {
	t.messages = append(t.messages, message)
}

func newRoundChangePolicyTestEngine(
	t *testing.T,
	transport Transport,
) *IBFT {
	t.Helper()

	backend := &roundChangePolicyBackend{
		validators: map[string]struct{}{
			"A": {},
			"B": {},
		},
	}

	engine := NewIBFT(nil, backend, transport)
	engine.state.setView(&proto.View{
		Height: 100,
		Round:  1,
	})

	require.NoError(t, engine.validatorManager.setCurrentVotingPower(
		map[string]*big.Int{
			"A": big.NewInt(1),
			"B": big.NewInt(1),
		},
	))

	messageStore := engine.messages.(*messages.Messages)
	t.Cleanup(messageStore.Close)

	return engine
}

func newRoundChangePolicyProposalMessage(
	height uint64,
	round uint64,
) *proto.Message {
	return &proto.Message{
		From: []byte("A"),
		View: &proto.View{
			Height: height,
			Round:  round,
		},
		Type: proto.MessageType_PREPREPARE,
		Payload: &proto.Message_PreprepareData{
			PreprepareData: &proto.PrePrepareMessage{
				Proposal: &proto.Proposal{
					RawProposal: roundChangePolicyProposal,
					Round:       round,
				},
				ProposalHash: roundChangePolicyProposalHash,
				Certificate: &proto.RoundChangeCertificate{
					RoundChangeMessages: []*proto.Message{
						newRoundChangePolicyMessage("A", height, round),
						newRoundChangePolicyMessage("B", height, round),
					},
				},
			},
		},
	}
}

/* FINISH ALPHA CHANGE 1903 -- exercise live bounded round-catch-up control flow -- */

func newRoundChangePolicyMessage(
	sender string,
	height uint64,
	round uint64,
) *proto.Message {
	return &proto.Message{
		From: []byte(sender),
		View: &proto.View{
			Height: height,
			Round:  round,
		},
		Type: proto.MessageType_ROUND_CHANGE,
		Payload: &proto.Message_RoundChangeData{
			RoundChangeData: &proto.RoundChangeMessage{},
		},
	}
}

func TestFutureRoundChangeEvidenceSelectsHighestQualifyingRound(t *testing.T) {
	t.Parallel()

	messageStore := messages.NewMessages()
	t.Cleanup(messageStore.Close)

	backend := &roundChangePolicyBackend{
		validators: map[string]struct{}{
			"A": {},
			"B": {},
		},
	}

	engine := &IBFT{
		state: &state{
			view: &proto.View{
				Height: 100,
				Round:  1,
			},
			name: newRound,
		},
		messages: messageStore,
		backend:  backend,
		validatorManager: NewValidatorManager(
			nil,
			nil,
		),
	}

	require.NoError(t, engine.validatorManager.setCurrentVotingPower(
		map[string]*big.Int{
			"A": big.NewInt(1),
			"B": big.NewInt(1),
		},
	))

	// Round 10 is nine rounds ahead of round 1, so it must be rejected
	// before reaching the message store.
	engine.AddMessage(newRoundChangePolicyMessage("A", 100, 10))

	require.Empty(t, messageStore.GetValidMessages(
		&proto.View{
			Height: 100,
			Round:  10,
		},
		proto.MessageType_ROUND_CHANGE,
		func(*proto.Message) bool {
			return true
		},
	))

	// Round 5 has full quorum from both validators.
	engine.AddMessage(newRoundChangePolicyMessage("A", 100, 5))
	engine.AddMessage(newRoundChangePolicyMessage("B", 100, 5))

	// Round 6 has only one validator. In a two-validator set this is
	// strictly more than one third, but it is not full quorum.
	engine.AddMessage(newRoundChangePolicyMessage("A", 100, 6))

	weakCertificate := engine.handleFutureRoundChangeMessages(
		engine.state.getView(),
		engine.validatorManager.HasWeakQuorum,
	)
	require.NotNil(t, weakCertificate)
	require.Len(t, weakCertificate.RoundChangeMessages, 1)
	require.Equal(
		t,
		uint64(6),
		weakCertificate.RoundChangeMessages[0].View.Round,
	)

	fullCertificate := engine.handleFutureRoundChangeMessages(
		engine.state.getView(),
		engine.validatorManager.HasQuorum,
	)
	require.NotNil(t, fullCertificate)
	require.Len(t, fullCertificate.RoundChangeMessages, 2)

	for _, message := range fullCertificate.RoundChangeMessages {
		require.Equal(t, uint64(5), message.View.Round)
	}
}

/* FINISH ALPHA CHANGE 1898 -- test authenticated future-round certificate selection -- */

/* START ALPHA CHANGE 1903 -- exercise live bounded round-catch-up control flow -- */
func TestFutureProposalWatcherFindsStoredProposalAcrossBoundedWindow(t *testing.T) {
	engine := newRoundChangePolicyTestEngine(t, nil)
	proposal := newRoundChangePolicyProposalMessage(100, 6)

	// Store the proposal before the watcher subscribes. The watcher must still
	// find it by scanning the complete bounded future-round window.
	engine.AddMessage(proposal)

	ctx, cancel := context.WithCancel(context.Background())
	engine.wg.Add(1)
	go engine.watchForFutureProposal(ctx)

	var event newProposalEvent

	select {
	case event = <-engine.newProposal:
	case <-time.After(time.Second):
		cancel()
		engine.wg.Wait()
		t.Fatal("future proposal watcher did not find the stored proposal")
	}

	cancel()
	engine.wg.Wait()

	require.Same(t, proposal, event.proposalMessage)
	require.Equal(t, uint64(6), event.round)
}

func TestRoundChangeWatcherSelectsHigherWeakEvidenceOverLowerFullQuorum(
	t *testing.T,
) {
	engine := newRoundChangePolicyTestEngine(t, nil)

	// Round 5 has full quorum.
	engine.AddMessage(newRoundChangePolicyMessage("A", 100, 5))
	engine.AddMessage(newRoundChangePolicyMessage("B", 100, 5))

	// Round 6 has weak quorum. It is the highest qualifying evidence and must
	// therefore be selected instead of the lower full certificate.
	engine.AddMessage(newRoundChangePolicyMessage("A", 100, 6))

	ctx, cancel := context.WithCancel(context.Background())
	engine.wg.Add(1)
	go engine.watchForRoundChangeCertificates(ctx)

	var event roundChangeEvent

	select {
	case event = <-engine.roundCertificate:
	case <-time.After(time.Second):
		cancel()
		engine.wg.Wait()
		t.Fatal("round-change watcher did not select stored future evidence")
	}

	cancel()
	engine.wg.Wait()

	require.Equal(t, uint64(6), event.round)
	require.True(t, event.weakQuorum)
}

func TestRoundChangeEventRebroadcastsOnlyWeakEvidence(t *testing.T) {
	transport := &roundChangePolicyTransport{}
	engine := newRoundChangePolicyTestEngine(t, transport)

	engine.applyRoundChangeEvent(100, roundChangeEvent{
		round:      6,
		weakQuorum: true,
	})

	require.Equal(t, uint64(6), engine.state.getRound())
	require.Len(t, transport.messages, 1)
	require.Equal(t, proto.MessageType_ROUND_CHANGE, transport.messages[0].Type)
	require.Equal(t, []byte("local"), transport.messages[0].From)
	require.Equal(t, uint64(100), transport.messages[0].View.Height)
	require.Equal(t, uint64(6), transport.messages[0].View.Round)

	transport.messages = nil

	engine.applyRoundChangeEvent(100, roundChangeEvent{
		round:      7,
		weakQuorum: false,
	})

	require.Equal(t, uint64(7), engine.state.getRound())
	require.Empty(t, transport.messages)
}

func TestRoundChangeEvidencePrefersFullQuorumAtEqualRound(t *testing.T) {
	engine := newRoundChangePolicyTestEngine(t, nil)

	engine.AddMessage(newRoundChangePolicyMessage("A", 100, 6))
	engine.AddMessage(newRoundChangePolicyMessage("B", 100, 6))

	event, found := engine.highestFutureRoundChangeEvent(engine.state.getView())

	require.True(t, found)
	require.Equal(t, uint64(6), event.round)
	require.False(t, event.weakQuorum)
}

/* FINISH ALPHA CHANGE 1903 -- exercise live bounded round-catch-up control flow -- */
