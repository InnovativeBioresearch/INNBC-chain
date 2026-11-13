/* START ALPHA CHANGE 2584 -- coordinate proposal execution ownership without a shared target -- */
package ibftcore

import (
	"bytes"
	"context"

	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
)

/*
Value is opaque to the core. The backend verifies its identity and owns
its mutable execution data.
*/
type ProposalExecution struct {
	Value any
}

type proposalCandidate struct {
	message   *proto.Message
	execution *ProposalExecution
}

func candidateProposal(candidate *proposalCandidate) *proto.Proposal {
	if candidate == nil || candidate.message == nil ||
		candidate.message.Type != proto.MessageType_PREPREPARE {
		return nil
	}

	data := candidate.message.GetPreprepareData()
	if data == nil {
		return nil
	}

	return data.Proposal
}

func (s *state) getExecution() *ProposalExecution {
	s.RLock()
	defer s.RUnlock()

	return s.execution
}

func (s *state) preparedExecution(raw []byte) *ProposalExecution {
	s.RLock()
	defer s.RUnlock()

	if s.latestPreparedProposal == nil ||
		!bytes.Equal(s.latestPreparedProposal.RawProposal, raw) {
		return nil
	}

	return s.latestPreparedExecution
}

func (s *state) adoptExecutedProposal(candidate *proposalCandidate) bool {
	proposal := candidateProposal(candidate)
	if proposal == nil || candidate.execution == nil ||
		candidate.message.View == nil {
		return false
	}

	s.Lock()
	defer s.Unlock()

	view := candidate.message.View
	if s.view == nil || view.Height != s.view.Height ||
		view.Round != s.view.Round || proposal.Round != view.Round ||
		s.inserted {
		return false
	}

	s.proposalMessage = candidate.message
	s.execution = candidate.execution
	s.name = prepare

	return true
}

func (s *state) markInserted() {
	s.Lock()
	defer s.Unlock()

	s.inserted = true
}

func (s *state) wasInserted() bool {
	s.RLock()
	defer s.RUnlock()

	return s.inserted
}

// SequenceFailure remains latched until the consensus instance is replaced.
func (i *IBFT) SequenceFailure() error {
	i.failureMutex.Lock()
	defer i.failureMutex.Unlock()

	return i.failureErr
}

func (i *IBFT) recordSequenceFailure(err error) {
	if err == nil {
		return
	}

	i.failureMutex.Lock()
	defer i.failureMutex.Unlock()

	if i.failureErr != nil {
		return
	}

	i.failureErr = err
	if i.proposalFailure != nil {
		close(i.proposalFailure)
	}
}

func (i *IBFT) highestValidFutureCandidate(
	view *proto.View,
) (*proposalCandidate, uint64) {
	if view == nil {
		return nil, 0
	}

	for distance := maxFutureRoundDistance; distance > 0; distance-- {
		if distance > ^uint64(0)-view.Round {
			continue
		}

		round := view.Round + distance
		candidate := i.handlePrePrepareCandidate(&proto.View{
			Height: view.Height,
			Round:  round,
		})
		if candidate != nil {
			return candidate, round
		}
	}

	return nil, 0
}

func (i *IBFT) handlePrePrepareCandidate(view *proto.View) *proposalCandidate {
	if view == nil {
		return nil
	}

	var candidate *proposalCandidate

	isValidPrePrepare := func(message *proto.Message) bool {
		var valid bool

		if view.Round == 0 {
			valid = i.validateProposal0(message, view)
		} else {
			valid = i.validateProposal(message, view)
		}

		if !valid {
			return false
		}

		/*
			Only one valid candidate is executed in this scan. Further
			messages still undergo protocol validation and pruning.
		*/
		if candidate != nil {
			return true
		}

		proposal := messages.ExtractProposal(message)
		execution := i.state.preparedExecution(proposal.RawProposal)
		if execution == nil {
			execution = i.backend.ExecuteProposal(proposal.RawProposal)
		}

		if execution == nil ||
			!i.backend.IsCurrentExecution(proposal.RawProposal, execution) {
			return false
		}

		candidate = &proposalCandidate{
			message:   message,
			execution: execution,
		}

		return true
	}

	i.messages.GetValidMessages(
		view,
		proto.MessageType_PREPREPARE,
		isValidPrePrepare,
	)

	return candidate
}

func (i *IBFT) buildCandidate(
	ctx context.Context,
	view *proto.View,
) *proposalCandidate {
	var execution *ProposalExecution

	message := i.buildProposalMessage(ctx, view, &execution)
	if message == nil || execution == nil {
		return nil
	}

	return &proposalCandidate{
		message:   message,
		execution: execution,
	}
}

func (i *IBFT) buildProposalMessage(
	ctx context.Context,
	view *proto.View,
	execution **ProposalExecution,
) *proto.Message {
	if view == nil || execution == nil || ctx.Err() != nil {
		return nil
	}

	height, round := view.Height, view.Round

	if round == 0 {
		raw, result := i.backend.BuildExecutedProposal(&proto.View{
			Height: height,
			Round:  round,
		})
		if len(raw) == 0 || result == nil {
			return nil
		}

		*execution = result

		return i.backend.BuildPrePrepareMessage(
			raw,
			nil,
			&proto.View{Height: height, Round: round},
		)
	}

	rcc := i.waitForRCC(ctx, height, round)
	if rcc == nil || ctx.Err() != nil {
		return nil
	}

	var (
		previousProposal []byte
		maxRound         uint64
	)

	for _, msg := range rcc.RoundChangeMessages {
		latestPC := messages.ExtractLatestPC(msg)
		if latestPC == nil {
			continue
		}

		proposal := messages.ExtractProposal(latestPC.ProposalMessage)
		preparedRound := proposal.Round

		if previousProposal != nil && preparedRound <= maxRound {
			continue
		}

		lastPrepared := messages.ExtractLastPreparedProposal(msg)
		if lastPrepared == nil {
			continue
		}

		previousProposal = lastPrepared.RawProposal
		maxRound = preparedRound
	}

	if previousProposal == nil {
		raw, result := i.backend.BuildExecutedProposal(&proto.View{
			Height: height,
			Round:  round,
		})
		if len(raw) == 0 || result == nil {
			return nil
		}

		*execution = result

		return i.backend.BuildPrePrepareMessage(
			raw,
			rcc,
			&proto.View{Height: height, Round: round},
		)
	}

	/*
		A locally prepared result is reused without EVM execution.
		A prepared block learned from other validators must be executed
		once if this node does not already own its execution result.
	*/
	result := i.state.preparedExecution(previousProposal)
	if result == nil {
		result = i.backend.ExecuteProposal(previousProposal)
	}

	if ctx.Err() != nil || result == nil ||
		!i.backend.IsCurrentExecution(previousProposal, result) {
		return nil
	}

	*execution = result

	return i.backend.BuildPrePrepareMessage(
		previousProposal,
		rcc,
		&proto.View{Height: height, Round: round},
	)
}

/* FINISH ALPHA CHANGE 2584 -- coordinate proposal execution ownership without a shared target -- */
