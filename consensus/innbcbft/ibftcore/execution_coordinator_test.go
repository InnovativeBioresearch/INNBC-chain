/* START ALPHA CHANGE 2596 -- exercise execution ownership through the running sequence coordinator -- */
package ibftcore

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/stretchr/testify/require"
)

type coordinatorTransport struct{}

func (*coordinatorTransport) Multicast(*proto.Message) {}

type coordinatorBackend struct {
	*executionOwnershipBackend
	futureExecuted chan struct{}
	futureOnce     sync.Once
	insertStarted  chan struct{}
	insertRelease  chan struct{}
}

func (*coordinatorBackend) GetVotingPowers(uint64) (map[string]*big.Int, error) {
	return map[string]*big.Int{"A": big.NewInt(1), "B": big.NewInt(1)}, nil
}

func (b *coordinatorBackend) ExecuteProposal(raw []byte) *ProposalExecution {
	execution := b.executionOwnershipBackend.ExecuteProposal(raw)
	if bytes.Equal(raw, []byte("future")) {
		b.futureOnce.Do(func() { close(b.futureExecuted) })
	}

	return execution
}

func (*coordinatorBackend) IsValidCommittedSeal(
	_ []byte,
	seal *messages.CommittedSeal,
) bool {
	return seal != nil && bytes.Equal(seal.Signature, []byte("test-seal"))
}

func (*coordinatorBackend) BuildCommitMessage(hash []byte, view *proto.View) *proto.Message {
	return coordinatorCommit("local", view.Round, hash)
}

func (b *coordinatorBackend) InsertProposal(
	proposal *proto.Proposal,
	seals []*messages.CommittedSeal,
	execution *ProposalExecution,
) error {
	if b.insertStarted != nil {
		close(b.insertStarted)
		<-b.insertRelease
	}

	return b.executionOwnershipBackend.InsertProposal(proposal, seals, execution)
}

/* START ALPHA CHANGE 2599 -- observe old-round cancellation separately from future-event delivery -- */
type coordinatorMessages struct {
	Messages
	ctx                context.Context
	currentQueryGate   <-chan struct{}
	futureReturnGate   <-chan struct{}
	futureUnsubscribed chan struct{}
	roundCancelled     chan struct{}
	futureIDs          sync.Map
	unsubscribeOnce    sync.Once
	cancelOnce         sync.Once
	prunes             atomic.Int32
}

func (m *coordinatorMessages) Subscribe(details messages.SubscriptionDetails) *messages.Subscription {
	sub := m.Messages.Subscribe(details)
	if details.HasMinRound && details.View.Round == 1 {
		switch details.MessageType {
		case proto.MessageType_PREPREPARE, proto.MessageType_ROUND_CHANGE:
			m.futureIDs.Store(sub.ID, details.MessageType)
		}
	}

	return sub
}

func (m *coordinatorMessages) Unsubscribe(id messages.SubscriptionID) {
	m.Messages.Unsubscribe(id)
	if messageType, found := m.futureIDs.LoadAndDelete(id); found {
		switch messageType {
		case proto.MessageType_PREPREPARE:
			m.unsubscribeOnce.Do(func() { close(m.futureUnsubscribed) })
		case proto.MessageType_ROUND_CHANGE:
			/*
				These tests store no ROUND_CHANGE messages. This watcher
				can finish only after its old-round context is cancelled.
			*/
			m.cancelOnce.Do(func() { close(m.roundCancelled) })
		}
	}
}

/* FINISH ALPHA CHANGE 2599 -- observe old-round cancellation separately from future-event delivery -- */

func (m *coordinatorMessages) PruneByHeight(height uint64) {
	m.prunes.Add(1)
	m.Messages.PruneByHeight(height)
}

func (m *coordinatorMessages) GetValidMessages(
	view *proto.View,
	messageType proto.MessageType,
	valid func(*proto.Message) bool,
) []*proto.Message {
	if messageType == proto.MessageType_PREPREPARE &&
		view.Round == 0 && m.currentQueryGate != nil {
		select {
		case <-m.currentQueryGate:
		case <-m.ctx.Done():
			return nil
		}
	}

	result := m.Messages.GetValidMessages(view, messageType, valid)

	if messageType == proto.MessageType_PREPREPARE &&
		view.Round == 1 && len(result) != 0 && m.futureReturnGate != nil {
		select {
		case <-m.futureReturnGate:
		case <-m.ctx.Done():
			return nil
		}
	}

	return result
}

func newCoordinatorEngine(
	t *testing.T,
	ctx context.Context,
) (*IBFT, *coordinatorBackend, *coordinatorMessages) {
	t.Helper()

	engine, ownership := newExecutionOwnershipEngine(t)
	backend := &coordinatorBackend{
		executionOwnershipBackend: ownership,
		futureExecuted:            make(chan struct{}),
	}
	/* START ALPHA CHANGE 2600 -- initialize the round-cancellation handshake -- */
	store := &coordinatorMessages{
		Messages:           engine.messages,
		ctx:                ctx,
		futureUnsubscribed: make(chan struct{}),
		roundCancelled:     make(chan struct{}),
	}
	/* FINISH ALPHA CHANGE 2600 -- initialize the round-cancellation handshake -- */

	engine.backend = backend
	engine.validatorManager.backend = backend
	engine.messages = store
	engine.transport = &coordinatorTransport{}
	engine.baseRoundTimeout = time.Hour

	return engine, backend, store
}

func coordinatorCommit(sender string, round uint64, hash []byte) *proto.Message {
	return &proto.Message{
		From: []byte(sender),
		View: &proto.View{Height: 100, Round: round},
		Type: proto.MessageType_COMMIT,
		Payload: &proto.Message_CommitData{
			CommitData: &proto.CommitMessage{
				ProposalHash:  hash,
				CommittedSeal: []byte("test-seal"),
			},
		},
	}
}

func coordinatorVotes(engine *IBFT, round uint64, raw string) {
	engine.messages.AddMessage(&proto.Message{
		From: []byte("B"),
		View: &proto.View{Height: 100, Round: round},
		Type: proto.MessageType_PREPARE,
		Payload: &proto.Message_PrepareData{
			PrepareData: &proto.PrepareMessage{ProposalHash: []byte(raw)},
		},
	})
	for _, sender := range []string{"A", "B"} {
		engine.messages.AddMessage(coordinatorCommit(sender, round, []byte(raw)))
	}
}

func coordinatorWait(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("coordinator handshake did not complete")
	}
}

func coordinatorRun(
	t *testing.T,
	engine *IBFT,
	ctx context.Context,
	cancel context.CancelFunc,
	release func(),
) <-chan struct{} {
	t.Helper()

	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		release()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("sequence workers did not stop")
		}
	})

	go func() {
		defer close(done)
		engine.RunSequence(ctx, 100)
	}()

	return done
}

func TestExecutionCoordinator_FutureBThenCancelledA(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	engine, backend, store := newCoordinatorEngine(t, ctx)

	backend.started = make(chan struct{})
	backend.release = make(chan struct{})
	store.currentQueryGate = backend.futureExecuted
	store.futureReturnGate = backend.started

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(backend.release) }) }

	engine.messages.AddMessage(executionOwnershipMessage(0, "old"))
	engine.messages.AddMessage(executionOwnershipMessage(1, "future"))
	coordinatorVotes(engine, 1, "future")

	done := coordinatorRun(t, engine, ctx, cancel, release)

	/* START ALPHA CHANGE 2601 -- release A only after its round has actually been cancelled -- */
	/*
		B executes first and A starts before the future event is sent.
		Wait for both event delivery and old-round cancellation before
		allowing A to finish execution.
	*/
	coordinatorWait(t, store.futureUnsubscribed)
	coordinatorWait(t, store.roundCancelled)
	require.NoError(t, ctx.Err())
	require.Equal(t, int32(2), backend.executions.Load())
	require.Nil(t, engine.state.getExecution())
	require.Equal(t, uint64(0), engine.state.getRound())
	/* FINISH ALPHA CHANGE 2601 -- release A only after its round has actually been cancelled -- */

	release()
	coordinatorWait(t, done)

	require.NoError(t, engine.SequenceFailure())
	require.True(t, engine.state.wasInserted())
	require.Equal(t, uint64(1), engine.state.getRound())
	require.Equal(t, []byte("future"), engine.state.getProposal().RawProposal)
	require.Equal(t, []byte("future"), engine.state.getExecution().Value)
	require.Equal(t, int32(1), backend.insertions.Load())
	require.Equal(t, int32(1), backend.votes.Load())
	require.Len(t, engine.state.getCommittedSeals(), 2)
}

func TestExecutionCoordinator_RetainsCompletedDecision(t *testing.T) {
	for _, test := range []struct {
		name   string
		future bool
		fail   bool
	}{
		{"inserted during future teardown", true, false},
		{"failed during future teardown", true, true},
		{"failure while sequence is waiting", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			engine, backend, store := newCoordinatorEngine(t, ctx)

			cause := errors.New("injected insertion failure")
			if test.fail {
				backend.insertErr = cause
			}

			backend.insertStarted = make(chan struct{})
			backend.insertRelease = make(chan struct{})
			var releaseOnce sync.Once
			release := func() {
				releaseOnce.Do(func() { close(backend.insertRelease) })
			}

			engine.messages.AddMessage(executionOwnershipMessage(0, "old"))
			coordinatorVotes(engine, 0, "old")
			done := coordinatorRun(t, engine, ctx, cancel, release)
			coordinatorWait(t, backend.insertStarted)

			selected := engine.state.getExecution()
			require.NotNil(t, selected)
			seals := engine.state.getCommittedSeals()
			require.Len(t, seals, 2)

			/* START ALPHA CHANGE 2602 -- complete insertion while the coordinator is joining cancelled workers -- */
			if test.future {
				engine.AddMessage(executionOwnershipMessage(1, "future"))
				coordinatorWait(t, store.futureUnsubscribed)
				coordinatorWait(t, store.roundCancelled)
				require.NoError(t, ctx.Err())
				require.Equal(t, int32(2), backend.executions.Load())
			}
			/* FINISH ALPHA CHANGE 2602 -- complete insertion while the coordinator is joining cancelled workers -- */

			release()
			coordinatorWait(t, done)

			require.Equal(t, uint64(0), engine.state.getRound())
			require.Equal(t, fin, engine.state.getStateName())
			require.Equal(t, []byte("old"), engine.state.getProposal().RawProposal)
			require.Same(t, selected, engine.state.getExecution())
			require.Equal(t, seals, engine.state.getCommittedSeals())
			require.Equal(t, int32(1), backend.insertions.Load())
			require.Equal(t, int32(1), backend.votes.Load())

			if test.fail {
				require.ErrorIs(t, engine.SequenceFailure(), cause)
				require.False(t, engine.state.wasInserted())
				require.Equal(t, int32(1), store.prunes.Load())

				engine.RunSequence(context.Background(), 101)
				require.Equal(t, uint64(100), engine.state.getHeight())
				require.Same(t, selected, engine.state.getExecution())
			} else {
				require.NoError(t, engine.SequenceFailure())
				require.True(t, engine.state.wasInserted())
				require.Equal(t, int32(2), store.prunes.Load())
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2596 -- exercise execution ownership through the running sequence coordinator -- */
