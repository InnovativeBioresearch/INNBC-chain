// Package ibftcore implements the INNBC-owned IBFT consensus state machine.
package ibftcore

import (
	/* START ALPHA CHANGE 1860 -- use INNBC-owned IBFT message packages -- */
	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	/* FINISH ALPHA CHANGE 1860 -- use INNBC-owned IBFT message packages -- */)

// MessageConstructor defines a message constructor interface
// All constructed messages must be signed by a validator for the whole message
type MessageConstructor interface {
	// BuildPrePrepareMessage builds a PREPREPARE message based on the passed in view and proposal
	BuildPrePrepareMessage(
		rawProposal []byte,
		certificate *proto.RoundChangeCertificate,
		view *proto.View,
	) *proto.Message

	// BuildPrepareMessage builds a PREPARE message based on the passed in view and proposal hash
	BuildPrepareMessage(proposalHash []byte, view *proto.View) *proto.Message

	// BuildCommitMessage builds a COMMIT message based on the passed in view and proposal hash
	// Must create a committed seal for proposal hash and include it into the message
	BuildCommitMessage(proposalHash []byte, view *proto.View) *proto.Message

	// BuildRoundChangeMessage builds a ROUND_CHANGE message based on the passed in view,
	// latest prepared proposal, and latest prepared certificate
	BuildRoundChangeMessage(
		proposal *proto.Proposal,
		certificate *proto.PreparedCertificate,
		view *proto.View,
	) *proto.Message
}

// Verifier defines the verifier interface
type Verifier interface {
	// IsValidProposal checks if the proposal is valid
	IsValidProposal(rawProposal []byte) bool

	// IsValidValidator checks if a signature in message is signed by sender
	// Must check the following things:
	// (1) recover the signature and the signer matches from address in message
	// (2) the signer address is one of the validators at the height in message
	IsValidValidator(msg *proto.Message) bool

	// IsProposer checks if the passed in ID is the Proposer for current view (sequence, round)
	IsProposer(id []byte, height, round uint64) bool

	// IsValidProposalHash checks if the hash matches the proposal
	IsValidProposalHash(proposal *proto.Proposal, hash []byte) bool

	// IsValidCommittedSeal checks
	// if signature for proposal hash in committed seal is signed by a validator
	IsValidCommittedSeal(proposalHash []byte, committedSeal *messages.CommittedSeal) bool
}

// Backend defines an interface all backend implementations
// need to implement
/* START ALPHA CHANGE 2581 -- carry proposal execution results through selection and insertion -- */
type Backend interface {
	MessageConstructor
	Verifier
	ValidatorBackend

	// ExecuteProposal validates and executes without selecting an insertion target.
	ExecuteProposal(rawProposal []byte) *ProposalExecution

	// IsCurrentExecution checks proposal identity and the current execution context.
	IsCurrentExecution(rawProposal []byte, execution *ProposalExecution) bool

	// BuildExecutedProposal returns both the proposal and its execution result.
	BuildExecutedProposal(view *proto.View) ([]byte, *ProposalExecution)

	// InsertProposal inserts the selected execution result and reports failures.
	InsertProposal(
		proposal *proto.Proposal,
		committedSeals []*messages.CommittedSeal,
		execution *ProposalExecution,
	) error

	// ID returns the validator's ID
	ID() []byte
}

/* FINISH ALPHA CHANGE 2581 -- carry proposal execution results through selection and insertion -- */
