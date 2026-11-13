/* START ALPHA CHANGE 2588 -- test proposal adoption, cancellation, prepared reuse, and insertion failure -- */
package ibftcore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/stretchr/testify/require"
)

/*
These tests control execution ordering at the backend boundary.
Actual block execution ownership is tested separately in the runtime package.
*/
type executionOwnershipBackend struct {
	Backend

	started chan struct{}
	release chan struct{}
	once    sync.Once

	executions atomic.Int32
	builds     atomic.Int32
	votes      atomic.Int32
	insertions atomic.Int32
	insertErr  error
}

func (b *executionOwnershipBackend) ExecuteProposal(raw []byte) *ProposalExecution {
	b.executions.Add(1)

	if bytes.Equal(raw, []byte("old")) && b.started != nil {
		b.once.Do(func() { close(b.started) })
		<-b.release
	}

	return &ProposalExecution{Value: append([]byte(nil), raw...)}
}

func (b *executionOwnershipBackend) IsValidProposalHash(
	proposal *proto.Proposal,
	hash []byte,
) bool {
	return proposal != nil &&
		(b.Backend.IsValidProposalHash(proposal, hash) ||
			bytes.Equal(proposal.RawProposal, hash))
}

func (b *executionOwnershipBackend) BuildExecutedProposal(
	view *proto.View,
) ([]byte, *ProposalExecution) {
	b.builds.Add(1)

	return b.Backend.BuildExecutedProposal(view)
}

func (b *executionOwnershipBackend) BuildPrePrepareMessage(
	raw []byte,
	certificate *proto.RoundChangeCertificate,
	view *proto.View,
) *proto.Message {
	message := newRoundChangePolicyProposalMessage(view.Height, view.Round)
	data := message.GetPreprepareData()
	data.Proposal.RawProposal = raw
	data.Certificate = certificate

	if !bytes.Equal(raw, roundChangePolicyProposal) {
		data.ProposalHash = raw
	}

	return message
}

func (b *executionOwnershipBackend) BuildPrepareMessage(
	hash []byte,
	view *proto.View,
) *proto.Message {
	b.votes.Add(1)

	return &proto.Message{
		From: b.ID(),
		View: view,
		Type: proto.MessageType_PREPARE,
		Payload: &proto.Message_PrepareData{
			PrepareData: &proto.PrepareMessage{ProposalHash: hash},
		},
	}
}

func (b *executionOwnershipBackend) InsertProposal(
	proposal *proto.Proposal,
	_ []*messages.CommittedSeal,
	execution *ProposalExecution,
) error {
	b.insertions.Add(1)

	if proposal == nil ||
		!b.IsCurrentExecution(proposal.RawProposal, execution) {
		return errors.New("wrong execution result selected")
	}

	return b.insertErr
}

func newExecutionOwnershipEngine(
	t *testing.T,
) (*IBFT, *executionOwnershipBackend) {
	t.Helper()

	engine := newRoundChangePolicyTestEngine(t, &roundChangePolicyTransport{})
	engine.log = hclog.NewNullLogger()

	backend := &executionOwnershipBackend{Backend: engine.backend}
	engine.backend = backend

	return engine, backend
}

func executionOwnershipMessage(round uint64, raw string) *proto.Message {
	message := newRoundChangePolicyProposalMessage(100, round)
	message.GetPreprepareData().Proposal.RawProposal = []byte(raw)
	message.GetPreprepareData().ProposalHash = []byte(raw)

	if round == 0 {
		message.GetPreprepareData().Certificate = nil
	}

	return message
}

func TestExecutionOwnership_FutureBThenCancelledA(t *testing.T) {
	engine, backend := newExecutionOwnershipEngine(t)
	engine.state.setView(&proto.View{Height: 100, Round: 0})

	future := executionOwnershipMessage(1, "future")
	engine.messages.AddMessage(future)

	selected, round := engine.highestValidFutureCandidate(engine.state.getView())
	require.NotNil(t, selected)
	require.Equal(t, uint64(1), round)
	require.Equal(t, int32(1), backend.executions.Load())
	require.Nil(t, engine.state.getProposalMessage())
	require.Nil(t, engine.state.getExecution())

	backend.started = make(chan struct{})
	backend.release = make(chan struct{})

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(backend.release) })
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.messages.AddMessage(executionOwnershipMessage(0, "old"))

	done := make(chan error, 1)
	go func() {
		done <- engine.runNewRound(ctx)
	}()

	select {
	case <-backend.started:
	case <-time.After(5 * time.Second):
		t.Fatal("old proposal never reached execution")
	}

	cancel()
	release()

	select {
	case err := <-done:
		require.ErrorIs(t, err, errTimeoutExpired)
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled validation did not finish")
	}

	require.Nil(t, engine.state.getProposalMessage())
	require.Nil(t, engine.state.getExecution())
	require.Zero(t, backend.votes.Load())

	engine.moveToNewRound(round)
	require.True(t, engine.acceptProposal(selected))
	require.Same(t, selected.execution, engine.state.getExecution())
	require.Same(t, future, engine.state.getProposalMessage())

	engine.state.changeState(fin)
	require.NoError(t, engine.runFin())
	require.True(t, engine.state.wasInserted())
	require.Equal(t, int32(1), backend.insertions.Load())
	require.Equal(t, int32(2), backend.executions.Load())
}

func TestExecutionOwnership_PreparedReuseRetainsResult(t *testing.T) {
	engine, backend := newExecutionOwnershipEngine(t)
	engine.state.setView(&proto.View{Height: 100, Round: 0})

	proposalMessage := newRoundChangePolicyProposalMessage(100, 0)
	proposalMessage.GetPreprepareData().Certificate = nil

	execution := &ProposalExecution{
		Value: append([]byte(nil), roundChangePolicyProposal...),
	}
	require.True(t, engine.acceptProposal(&proposalCandidate{
		message:   proposalMessage,
		execution: execution,
	}))

	certificate := &proto.PreparedCertificate{
		ProposalMessage: proposalMessage,
		PrepareMessages: []*proto.Message{
			{
				From: []byte("B"),
				View: &proto.View{Height: 100, Round: 0},
				Type: proto.MessageType_PREPARE,
				Payload: &proto.Message_PrepareData{
					PrepareData: &proto.PrepareMessage{
						ProposalHash: roundChangePolicyProposalHash,
					},
				},
			},
		},
	}

	prepared := proposalMessage.GetPreprepareData().Proposal
	engine.state.finalizePrepare(certificate, prepared)
	engine.moveToNewRound(1)

	require.Nil(t, engine.state.getExecution())
	require.Same(t, execution, engine.state.preparedExecution(prepared.RawProposal))

	for _, sender := range []string{"A", "B"} {
		message := newRoundChangePolicyMessage(sender, 100, 1)
		data := message.GetRoundChangeData()
		data.LatestPreparedCertificate = certificate
		data.LastPreparedProposal = prepared
		engine.messages.AddMessage(message)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	candidate := engine.buildCandidate(ctx, engine.state.getView())
	require.NotNil(t, candidate)
	require.Same(t, execution, candidate.execution)
	require.Zero(t, backend.executions.Load())
	require.Zero(t, backend.builds.Load())
	require.True(t, engine.acceptProposal(candidate))

	engine.state.reset(101)

	require.Nil(t, engine.state.getExecution())
	require.Nil(t, engine.state.preparedExecution(prepared.RawProposal))
	require.Nil(t, engine.state.getLatestPreparedProposal())
	require.False(t, engine.state.wasInserted())
}

type executionOwnershipMessages struct {
	Messages
	prunes int
}

func (m *executionOwnershipMessages) PruneByHeight(height uint64) {
	m.prunes++
	m.Messages.PruneByHeight(height)
}

func TestExecutionOwnership_InsertionFailureRetainsDecision(t *testing.T) {
	for _, phase := range []string{"insertion", "advancement"} {
		t.Run(phase, func(t *testing.T) {
			engine, backend := newExecutionOwnershipEngine(t)
			engine.state.setView(&proto.View{Height: 100, Round: 0})

			cause := errors.New("injected failure")
			backend.insertErr = fmt.Errorf("%s failed: %w", phase, cause)

			proposal := newRoundChangePolicyProposalMessage(100, 0)
			execution := &ProposalExecution{
				Value: append([]byte(nil), roundChangePolicyProposal...),
			}
			require.True(t, engine.acceptProposal(&proposalCandidate{
				message:   proposal,
				execution: execution,
			}))

			seals := []*messages.CommittedSeal{{Signer: []byte("A")}}
			engine.state.setCommittedSeals(seals)
			engine.state.changeState(fin)

			store := &executionOwnershipMessages{Messages: engine.messages}
			engine.messages = store

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			engine.runStates(ctx)

			require.ErrorIs(t, engine.SequenceFailure(), cause)
			require.Same(t, proposal, engine.state.getProposalMessage())
			require.Same(t, execution, engine.state.getExecution())
			require.Equal(t, seals, engine.state.getCommittedSeals())
			require.Equal(t, fin, engine.state.getStateName())
			require.False(t, engine.state.wasInserted())
			require.Zero(t, store.prunes)
			require.Equal(t, int32(1), backend.insertions.Load())

			select {
			case <-engine.proposalFailure:
			default:
				t.Fatal("insertion failure was not signalled")
			}

			engine.RunSequence(ctx, 101)

			require.Equal(t, uint64(100), engine.state.getHeight())
			require.Same(t, execution, engine.state.getExecution())
			require.Equal(t, int32(1), backend.insertions.Load())
		})
	}
}

/* FINISH ALPHA CHANGE 2588 -- test proposal adoption, cancellation, prepared reuse, and insertion failure -- */
