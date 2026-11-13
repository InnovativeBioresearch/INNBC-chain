package ibftcore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2568 -- test contextual validation before execution and voting -- */

/*
The eight-byte payload models the checkpoint-round contract.
The runtime package separately tests actual block/checkpoint decoding.

Validator authorization uses the existing controlled test backend.
These tests do not provide cryptographic or database integration coverage.
*/
type checkpointContextBackend struct {
	Backend

	hashChecks      int
	validatorChecks int
	executions      int
	prepareVotes    int
	onHashCheck     func()
}

func (b *checkpointContextBackend) IsValidProposalHash(
	proposal *proto.Proposal,
	hash []byte,
) bool {
	b.hashChecks++

	if b.onHashCheck != nil {
		b.onHashCheck()
	}

	if proposal == nil || len(proposal.RawProposal) != 8 {
		return false
	}

	embeddedRound := binary.BigEndian.Uint64(proposal.RawProposal)
	expected := sha256.Sum256(proposal.RawProposal)

	return embeddedRound <= proposal.Round &&
		bytes.Equal(expected[:], hash)
}

func (b *checkpointContextBackend) IsValidValidator(message *proto.Message) bool {
	b.validatorChecks++

	return b.Backend.IsValidValidator(message)
}

func (b *checkpointContextBackend) IsValidProposal([]byte) bool {
	b.executions++

	return true
}

/* START ALPHA CHANGE 2587 -- retain contextual-test execution counters with owned results -- */
func (b *checkpointContextBackend) ExecuteProposal(raw []byte) *ProposalExecution {
	if !b.IsValidProposal(raw) {
		return nil
	}

	return &ProposalExecution{Value: append([]byte(nil), raw...)}
}

/* FINISH ALPHA CHANGE 2587 -- retain contextual-test execution counters with owned results -- */

func (b *checkpointContextBackend) BuildPrepareMessage(
	hash []byte,
	view *proto.View,
) *proto.Message {
	b.prepareVotes++

	return &proto.Message{
		From: b.ID(),
		View: view,
		Type: proto.MessageType_PREPARE,
		Payload: &proto.Message_PrepareData{
			PrepareData: &proto.PrepareMessage{ProposalHash: hash},
		},
	}
}

func newCheckpointContextEngine(
	t *testing.T,
) (*IBFT, *checkpointContextBackend, *roundChangePolicyTransport) {
	t.Helper()

	transport := &roundChangePolicyTransport{}
	engine := newRoundChangePolicyTestEngine(t, transport)
	engine.log = hclog.NewNullLogger()

	backend := &checkpointContextBackend{Backend: engine.backend}
	engine.backend = backend

	return engine, backend, transport
}

func checkpointContextProposal(round, embeddedRound uint64) *proto.Message {
	raw := make([]byte, 8)
	binary.BigEndian.PutUint64(raw, embeddedRound)
	hash := sha256.Sum256(raw)

	return &proto.Message{
		From: []byte("A"),
		View: &proto.View{Height: 100, Round: round},
		Type: proto.MessageType_PREPREPARE,
		Payload: &proto.Message_PreprepareData{
			PreprepareData: &proto.PrePrepareMessage{
				Proposal: &proto.Proposal{
					RawProposal: raw,
					Round:       round,
				},
				ProposalHash: hash[:],
			},
		},
	}
}

func checkpointContextCertificate(
	round, embeddedRound uint64,
) *proto.PreparedCertificate {
	proposal := checkpointContextProposal(round, embeddedRound)

	return &proto.PreparedCertificate{
		ProposalMessage: proposal,
		PrepareMessages: []*proto.Message{
			{
				From: []byte("B"),
				View: &proto.View{Height: 100, Round: round},
				Type: proto.MessageType_PREPARE,
				Payload: &proto.Message_PrepareData{
					PrepareData: &proto.PrepareMessage{
						ProposalHash: proposal.GetPreprepareData().ProposalHash,
					},
				},
			},
		},
	}
}

func checkpointContextLastPrepared(
	certificate *proto.PreparedCertificate,
) *proto.Proposal {
	proposal := certificate.ProposalMessage.GetPreprepareData().Proposal

	return &proto.Proposal{
		RawProposal: append([]byte(nil), proposal.RawProposal...),
		Round:       proposal.Round,
	}
}

func checkpointContextAttachEvidence(
	proposal *proto.Message,
	certificate *proto.PreparedCertificate,
	lastPrepared *proto.Proposal,
) {
	rcc := &proto.RoundChangeCertificate{}

	for _, sender := range []string{"A", "B"} {
		rc := newRoundChangePolicyMessage(sender, 100, proposal.View.Round)
		rc.GetRoundChangeData().LatestPreparedCertificate = certificate
		rc.GetRoundChangeData().LastPreparedProposal = lastPrepared
		rcc.RoundChangeMessages = append(rcc.RoundChangeMessages, rc)
	}

	proposal.GetPreprepareData().Certificate = rcc
}

var checkpointContextCases = []string{
	"huge round zero",
	"maximum round zero",
	"certificate proposal round mismatch",
	"checkpoint exceeds original certified round",
	"certified raw hash mismatch",
	"last prepared round mismatch",
	"last prepared hash mismatch",
	"invalid supplied certificate",
	"missing last prepared proposal",
	"last prepared without certificate",
	"missing round change certificate",
	"candidate differs from prepared block",
	"candidate exceeds selected prepared round",
	"round zero accepted",
	"later unprepared accepted",
	"prepared reuse accepted",
	"successive prepared reuse accepted",
}

func checkpointContextCase(
	t *testing.T,
	name string,
) (*proto.Message, bool) {
	t.Helper()

	proposal := checkpointContextProposal(5, 2)
	certificate := checkpointContextCertificate(2, 2)
	lastPrepared := checkpointContextLastPrepared(certificate)
	accepted := false
	attach := true

	switch name {
	case "huge round zero":
		proposal = checkpointContextProposal(0, 1_000_000_000_000_000_000)
		attach = false

	case "maximum round zero":
		proposal = checkpointContextProposal(0, ^uint64(0))
		attach = false

	case "certificate proposal round mismatch":
		certificate.ProposalMessage.GetPreprepareData().Proposal.Round = 3

	case "checkpoint exceeds original certified round":
		proposal = checkpointContextProposal(5, 4)
		certificate = checkpointContextCertificate(2, 4)
		lastPrepared = checkpointContextLastPrepared(certificate)

	case "certified raw hash mismatch":
		/*
			The certificate messages still agree on the original hash.
			Only the certificate's contained raw proposal is changed.
			The separately supplied last-prepared proposal stays valid.
		*/
		certificate.ProposalMessage.GetPreprepareData().Proposal.RawProposal =
			checkpointContextProposal(2, 1).GetPreprepareData().Proposal.RawProposal

	case "last prepared round mismatch":
		lastPrepared.Round = 3

	case "last prepared hash mismatch":
		lastPrepared.RawProposal =
			checkpointContextProposal(2, 1).GetPreprepareData().Proposal.RawProposal

	case "invalid supplied certificate":
		certificate = &proto.PreparedCertificate{}

	case "missing last prepared proposal":
		lastPrepared = nil

	case "last prepared without certificate":
		certificate = nil

	case "missing round change certificate":
		attach = false

	case "candidate differs from prepared block":
		proposal = checkpointContextProposal(5, 1)

	case "candidate exceeds selected prepared round":
		proposal = checkpointContextProposal(5, 3)

	case "round zero accepted":
		proposal = checkpointContextProposal(0, 0)
		attach = false
		accepted = true

	case "later unprepared accepted":
		proposal = checkpointContextProposal(5, 5)
		certificate = nil
		lastPrepared = nil
		accepted = true

	case "prepared reuse accepted":
		accepted = true

	case "successive prepared reuse accepted":
		proposal = checkpointContextProposal(7, 2)
		certificate = checkpointContextCertificate(5, 2)
		lastPrepared = checkpointContextLastPrepared(certificate)
		accepted = true

	default:
		t.Fatalf("unknown checkpoint context case %q", name)
	}

	if attach {
		checkpointContextAttachEvidence(proposal, certificate, lastPrepared)
	}

	return proposal, accepted
}

func TestCheckpointRoundContext_ValidationBeforeExecutionAndVoting(t *testing.T) {
	for _, name := range checkpointContextCases {
		t.Run(name, func(t *testing.T) {
			proposal, accepted := checkpointContextCase(t, name)
			engine, backend, transport := newCheckpointContextEngine(t)
			engine.state.setView(proposal.View)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			/*
				For rejected cases, cancel only after validation begins.
				The active validation must finish its checks before execution
				or voting. The call count also detects admission/event failures.
			*/
			if !accepted {
				backend.onHashCheck = cancel
			}

			engine.AddMessage(proposal)
			err := engine.runNewRound(ctx)

			require.Greater(
				t,
				backend.hashChecks,
				0,
				"proposal never reached hash validation",
			)

			if accepted {
				require.NoError(t, err)
				require.Equal(t, 1, backend.executions)
				require.Equal(t, 1, backend.prepareVotes)
				require.Len(t, transport.messages, 1)
				require.Equal(
					t,
					proto.MessageType_PREPARE,
					transport.messages[0].Type,
				)
			} else {
				require.ErrorIs(t, err, errTimeoutExpired)
				require.Zero(t, backend.executions)
				require.Zero(t, backend.prepareVotes)
				require.Empty(t, transport.messages)
			}
		})
	}
}

func TestCheckpointRoundContext_ValidPCIndependently(t *testing.T) {
	for _, tc := range []struct {
		name       string
		accepted   bool
		hashChecks int
	}{
		{name: "valid original", accepted: true, hashChecks: 1},
		{name: "prepared reused again", accepted: true, hashChecks: 1},
		{name: "checkpoint ahead", hashChecks: 1},
		{name: "huge checkpoint", hashChecks: 1},
		{name: "maximum checkpoint", hashChecks: 1},
		{name: "contained proposal round mismatch"},
		{name: "raw hash mismatch", hashChecks: 1},
		{name: "prepare hash mismatch"},
		{name: "certificate round equals limit"},
		{name: "missing proposal view"},
		{name: "missing prepare view"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			certificate := checkpointContextCertificate(2, 2)
			limit := uint64(5)

			switch tc.name {
			case "prepared reused again":
				certificate = checkpointContextCertificate(5, 2)
				limit = 7

			case "checkpoint ahead":
				certificate = checkpointContextCertificate(2, 4)

			case "huge checkpoint":
				certificate = checkpointContextCertificate(
					0,
					1_000_000_000_000_000_000,
				)

			case "maximum checkpoint":
				certificate = checkpointContextCertificate(0, ^uint64(0))

			case "contained proposal round mismatch":
				certificate.ProposalMessage.GetPreprepareData().Proposal.Round = 3

			case "raw hash mismatch":
				certificate.ProposalMessage.GetPreprepareData().Proposal.RawProposal =
					checkpointContextProposal(2, 1).GetPreprepareData().Proposal.RawProposal

			case "prepare hash mismatch":
				certificate.PrepareMessages[0].GetPrepareData().ProposalHash =
					[]byte("different hash")

			case "certificate round equals limit":
				limit = 2

			case "missing proposal view":
				certificate.ProposalMessage.View = nil

			case "missing prepare view":
				certificate.PrepareMessages[0].View = nil
			}

			engine, backend, transport := newCheckpointContextEngine(t)

			require.Equal(t, tc.accepted, engine.validPC(certificate, limit, 100))
			require.Equal(t, tc.hashChecks, backend.hashChecks)
			require.Zero(t, backend.executions)
			require.Zero(t, backend.prepareVotes)
			require.Empty(t, transport.messages)
		})
	}
}

func TestCheckpointRoundContext_ProposalMatchesCertificateSingleHashCheck(t *testing.T) {
	for _, tc := range []struct {
		name       string
		accepted   bool
		hashChecks int
	}{
		{name: "valid", accepted: true, hashChecks: 1},
		{name: "later certificate reuses block", accepted: true, hashChecks: 1},
		{name: "both absent", accepted: true},
		{name: "proposal without certificate"},
		{name: "certificate without proposal"},
		{name: "last prepared round mismatch"},
		{name: "contained proposal round mismatch"},
		{name: "last prepared hash mismatch", hashChecks: 1},
		{name: "prepare hash mismatch", hashChecks: 1},
		{name: "nil prepare message", hashChecks: 1},
		{name: "missing prepare payload", hashChecks: 1},
		{name: "missing proposal view"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			certificate := checkpointContextCertificate(2, 2)
			lastPrepared := checkpointContextLastPrepared(certificate)

			switch tc.name {
			case "later certificate reuses block":
				certificate = checkpointContextCertificate(5, 2)
				lastPrepared = checkpointContextLastPrepared(certificate)

			case "both absent":
				certificate = nil
				lastPrepared = nil

			case "proposal without certificate":
				certificate = nil

			case "certificate without proposal":
				lastPrepared = nil

			case "last prepared round mismatch":
				lastPrepared.Round = 3

			case "contained proposal round mismatch":
				certificate.ProposalMessage.GetPreprepareData().Proposal.Round = 3

			case "last prepared hash mismatch":
				lastPrepared.RawProposal =
					checkpointContextProposal(2, 1).GetPreprepareData().Proposal.RawProposal

			case "prepare hash mismatch":
				certificate.PrepareMessages[0].GetPrepareData().ProposalHash =
					[]byte("different hash")

			case "nil prepare message":
				certificate.PrepareMessages[0] = nil

			case "missing prepare payload":
				certificate.PrepareMessages[0].Payload = nil

			case "missing proposal view":
				certificate.ProposalMessage.View = nil
			}

			engine, backend, transport := newCheckpointContextEngine(t)

			require.Equal(
				t,
				tc.accepted,
				engine.proposalMatchesCertificate(lastPrepared, certificate),
			)
			require.Equal(t, tc.hashChecks, backend.hashChecks)
			require.Zero(t, backend.executions)
			require.Zero(t, backend.prepareVotes)
			require.Empty(t, transport.messages)
		})
	}
}

func TestCheckpointRoundContext_RoundChangeEvidenceCollection(t *testing.T) {
	for _, name := range []string{
		"certificate proposal round mismatch",
		"checkpoint exceeds original certified round",
		"certified raw hash mismatch",
		"last prepared round mismatch",
		"last prepared hash mismatch",
		"invalid supplied certificate",
		"missing last prepared proposal",
		"last prepared without certificate",
		"later unprepared accepted",
		"prepared reuse accepted",
		"successive prepared reuse accepted",
	} {
		t.Run(name, func(t *testing.T) {
			for _, future := range []bool{false, true} {
				mode := "exact"
				if future {
					mode = "future"
				}

				t.Run(mode, func(t *testing.T) {
					proposal, accepted := checkpointContextCase(t, name)
					engine, backend, transport := newCheckpointContextEngine(t)

					currentRound := proposal.View.Round
					if future {
						currentRound = 1
					}

					engine.state.setView(&proto.View{
						Height: 100,
						Round:  currentRound,
					})

					evidence := proposal.GetPreprepareData().Certificate
					require.NotNil(t, evidence)

					for _, rc := range evidence.RoundChangeMessages {
						engine.AddMessage(rc)
					}

					/*
						Verify admission before exercising evidence collection.
						A dropped message must not make a rejection test pass.
					*/
					stored := engine.messages.GetValidMessages(
						proposal.View,
						proto.MessageType_ROUND_CHANGE,
						func(*proto.Message) bool { return true },
					)
					require.Len(t, stored, 2)

					before := backend.validatorChecks

					var rcc *proto.RoundChangeCertificate
					if future {
						rcc = engine.handleFutureRoundChangeMessages(
							engine.state.getView(),
							engine.validatorManager.HasQuorum,
						)
					} else {
						rcc = engine.handleRoundChangeMessage(engine.state.getView())
					}

					require.Greater(t, backend.validatorChecks, before)

					if accepted {
						require.NotNil(t, rcc)
						require.Len(t, rcc.RoundChangeMessages, 2)
						for _, rc := range rcc.RoundChangeMessages {
							require.Equal(t, proposal.View.Round, rc.View.Round)
						}
					} else {
						require.Nil(t, rcc)
					}

					require.Zero(t, backend.executions)
					require.Zero(t, backend.prepareVotes)
					require.Empty(t, transport.messages)
				})
			}
		})
	}
}

func TestCheckpointRoundContext_FutureProposalValidation(t *testing.T) {
	for _, name := range []string{
		"checkpoint exceeds original certified round",
		"certified raw hash mismatch",
		"last prepared round mismatch",
		"invalid supplied certificate",
		"missing last prepared proposal",
		"missing round change certificate",
		"candidate differs from prepared block",
		"candidate exceeds selected prepared round",
		"later unprepared accepted",
		"prepared reuse accepted",
		"successive prepared reuse accepted",
	} {
		t.Run(name, func(t *testing.T) {
			proposal, accepted := checkpointContextCase(t, name)
			engine, backend, transport := newCheckpointContextEngine(t)

			engine.state.setView(&proto.View{Height: 100, Round: 1})
			engine.AddMessage(proposal)

			stored := engine.messages.GetValidMessages(
				proposal.View,
				proto.MessageType_PREPREPARE,
				func(*proto.Message) bool { return true },
			)
			require.Len(t, stored, 1)

			selected, round := engine.highestValidFutureProposal(engine.state.getView())

			require.Greater(t, backend.hashChecks, 0)

			if accepted {
				require.Same(t, proposal, selected)
				require.Equal(t, proposal.View.Round, round)
				require.Equal(t, 1, backend.executions)
			} else {
				require.Nil(t, selected)
				require.Zero(t, round)
				require.Zero(t, backend.executions)
			}

			/*
				Future proposal validation returns a candidate.
				Adoption and voting occur separately in RunSequence.
			*/
			require.Zero(t, backend.prepareVotes)
			require.Empty(t, transport.messages)
		})
	}
}

/* FINISH ALPHA CHANGE 2568 -- test contextual validation before execution and voting -- */
