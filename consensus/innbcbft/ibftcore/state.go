/* START ALPHA CHANGE 1852 -- move IBFT state into INNBC ownership -- */
package ibftcore

/* FINISH ALPHA CHANGE 1852 -- move IBFT state into INNBC ownership -- */

import (
	"sync"

	/* START ALPHA CHANGE 1862 -- use INNBC-owned IBFT message packages -- */
	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	/* FINISH ALPHA CHANGE 1862 -- use INNBC-owned IBFT message packages -- */)

type stateType uint8

const (
	newRound stateType = iota
	prepare
	commit
	fin
)

func (s stateType) String() string {
	switch s {
	case newRound:
		return "new round"
	case prepare:
		return "prepare"
	case commit:
		return "commit"
	case fin:
		return "fin"
	}

	return ""
}

type state struct {
	sync.RWMutex

	//	current view (sequence, round)
	view *proto.View

	// latestPC is the latest prepared certificate
	latestPC *proto.PreparedCertificate

	// latestPreparedProposal is the proposal
	// for which Q(N)-1 PREPARE messages were received
	latestPreparedProposal *proto.Proposal

	//	accepted proposal for current round
	proposalMessage *proto.Message

	/* START ALPHA CHANGE 2585 -- retain only selected and latest prepared execution results -- */
	execution               *ProposalExecution
	latestPreparedExecution *ProposalExecution
	inserted                bool
	/* FINISH ALPHA CHANGE 2585 -- retain only selected and latest prepared execution results -- */

	//	validated commit seals
	seals []*messages.CommittedSeal

	//	flags for different states
	roundStarted bool

	name stateType
}

func (s *state) getView() *proto.View {
	s.RLock()
	defer s.RUnlock()

	return &proto.View{
		Height: s.view.Height,
		Round:  s.view.Round,
	}
}

func (s *state) reset(height uint64) {
	s.Lock()
	defer s.Unlock()

	s.seals = nil
	s.roundStarted = false
	s.name = newRound
	s.proposalMessage = nil
	s.latestPC = nil
	s.latestPreparedProposal = nil

	/* START ALPHA CHANGE 2585 -- release execution results when resetting the height -- */
	s.execution = nil
	s.latestPreparedExecution = nil
	s.inserted = false
	/* FINISH ALPHA CHANGE 2585 -- release execution results when resetting the height -- */

	s.view = &proto.View{
		Height: height,
		Round:  0,
	}
}

func (s *state) getLatestPC() *proto.PreparedCertificate {
	s.RLock()
	defer s.RUnlock()

	return s.latestPC
}

func (s *state) getLatestPreparedProposal() *proto.Proposal {
	s.RLock()
	defer s.RUnlock()

	return s.latestPreparedProposal
}

func (s *state) getProposalMessage() *proto.Message {
	s.RLock()
	defer s.RUnlock()

	return s.proposalMessage
}

func (s *state) getProposalHash() []byte {
	s.RLock()
	defer s.RUnlock()

	return messages.ExtractProposalHash(s.proposalMessage)
}

/* START ALPHA CHANGE 2585 -- require explicit execution ownership when selecting a proposal -- */
func (s *state) setProposalMessage(proposalMessage *proto.Message) {
	s.Lock()
	defer s.Unlock()

	s.proposalMessage = proposalMessage
	s.execution = nil
}

/* FINISH ALPHA CHANGE 2585 -- require explicit execution ownership when selecting a proposal -- */

func (s *state) getRound() uint64 {
	s.RLock()
	defer s.RUnlock()

	return s.view.Round
}

func (s *state) getHeight() uint64 {
	s.RLock()
	defer s.RUnlock()

	return s.view.Height
}

func (s *state) getProposal() *proto.Proposal {
	s.RLock()
	defer s.RUnlock()

	if s.proposalMessage != nil {
		return messages.ExtractProposal(s.proposalMessage)
	}

	return nil
}

func (s *state) getRawDataFromProposal() []byte {
	proposal := s.getProposal()

	if proposal != nil {
		return proposal.RawProposal
	}

	return nil
}

func (s *state) getCommittedSeals() []*messages.CommittedSeal {
	s.RLock()
	defer s.RUnlock()

	return s.seals
}

func (s *state) getStateName() stateType {
	s.RLock()
	defer s.RUnlock()

	return s.name
}

func (s *state) changeState(name stateType) {
	s.Lock()
	defer s.Unlock()

	s.name = name
}

func (s *state) setRoundStarted(started bool) {
	s.Lock()
	defer s.Unlock()

	s.roundStarted = started
}

func (s *state) setView(view *proto.View) {
	s.Lock()
	defer s.Unlock()

	s.view = view
}

func (s *state) setCommittedSeals(seals []*messages.CommittedSeal) {
	s.Lock()
	defer s.Unlock()

	s.seals = seals
}

func (s *state) newRound() {
	s.Lock()
	defer s.Unlock()

	if !s.roundStarted {
		// Round is not yet started, kick the round off
		s.name = newRound
		s.roundStarted = true
	}
}

func (s *state) finalizePrepare(
	certificate *proto.PreparedCertificate,
	latestPPB *proto.Proposal,
) {
	s.Lock()
	defer s.Unlock()

	s.latestPC = certificate
	s.latestPreparedProposal = latestPPB

	/* START ALPHA CHANGE 2585 -- preserve execution results for prepared proposal reuse -- */
	s.latestPreparedExecution = s.execution
	/* FINISH ALPHA CHANGE 2585 -- preserve execution results for prepared proposal reuse -- */

	// Move to the commit state
	s.name = commit
}
