package innbcbft

import (
	"testing"

	/* START ALPHA CHANGE 1876 -- test with INNBC-owned IBFT wire types -- */
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	/* FINISH ALPHA CHANGE 1876 -- test with INNBC-owned IBFT wire types -- */
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 1845 -- test structural consensus message admission -- */
func TestIsStructurallyValidIBFTMessage(t *testing.T) {
	t.Parallel()

	view := &proto.View{Height: 1, Round: 0}
	proposal := &proto.Proposal{
		RawProposal: []byte{1},
		Round:       0,
	}

	validPreprepare := &proto.Message{
		View: view,
		Type: proto.MessageType_PREPREPARE,
		Payload: &proto.Message_PreprepareData{
			PreprepareData: &proto.PrePrepareMessage{
				Proposal: proposal,
			},
		},
	}

	validPrepare := &proto.Message{
		View: view,
		Type: proto.MessageType_PREPARE,
		Payload: &proto.Message_PrepareData{
			PrepareData: &proto.PrepareMessage{
				ProposalHash: []byte{1},
			},
		},
	}

	validCommit := &proto.Message{
		View: view,
		Type: proto.MessageType_COMMIT,
		Payload: &proto.Message_CommitData{
			CommitData: &proto.CommitMessage{
				ProposalHash:  []byte{1},
				CommittedSeal: []byte{2},
			},
		},
	}

	validUnpreparedRoundChange := &proto.Message{
		View: view,
		Type: proto.MessageType_ROUND_CHANGE,
		Payload: &proto.Message_RoundChangeData{
			RoundChangeData: &proto.RoundChangeMessage{},
		},
	}

	validPreparedRoundChange := &proto.Message{
		View: view,
		Type: proto.MessageType_ROUND_CHANGE,
		Payload: &proto.Message_RoundChangeData{
			RoundChangeData: &proto.RoundChangeMessage{
				LastPreparedProposal: proposal,
				LatestPreparedCertificate: &proto.PreparedCertificate{
					ProposalMessage: validPreprepare,
					PrepareMessages: []*proto.Message{
						validPrepare,
					},
				},
			},
		},
	}

	validPreprepareWithCertificate := &proto.Message{
		View: view,
		Type: proto.MessageType_PREPREPARE,
		Payload: &proto.Message_PreprepareData{
			PreprepareData: &proto.PrePrepareMessage{
				Proposal: proposal,
				Certificate: &proto.RoundChangeCertificate{
					RoundChangeMessages: []*proto.Message{
						validUnpreparedRoundChange,
					},
				},
			},
		},
	}

	tests := []struct {
		name    string
		message *proto.Message
		valid   bool
	}{
		{
			name:    "nil message",
			message: nil,
		},
		{
			name: "missing view",
			message: &proto.Message{
				Type: proto.MessageType_PREPARE,
				Payload: &proto.Message_PrepareData{
					PrepareData: &proto.PrepareMessage{},
				},
			},
		},
		{
			name: "unknown message type",
			message: &proto.Message{
				View: view,
				Type: proto.MessageType(99),
			},
		},
		{
			name: "declared type does not match payload",
			message: &proto.Message{
				View: view,
				Type: proto.MessageType_COMMIT,
				Payload: &proto.Message_PrepareData{
					PrepareData: &proto.PrepareMessage{},
				},
			},
		},
		{
			name: "preprepare without proposal",
			message: &proto.Message{
				View: view,
				Type: proto.MessageType_PREPREPARE,
				Payload: &proto.Message_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{},
				},
			},
		},
		{
			name: "preprepare with empty proposal",
			message: &proto.Message{
				View: view,
				Type: proto.MessageType_PREPREPARE,
				Payload: &proto.Message_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						Proposal: &proto.Proposal{},
					},
				},
			},
		},
		{
			name: "round change with proposal but no certificate",
			message: &proto.Message{
				View: view,
				Type: proto.MessageType_ROUND_CHANGE,
				Payload: &proto.Message_RoundChangeData{
					RoundChangeData: &proto.RoundChangeMessage{
						LastPreparedProposal: proposal,
					},
				},
			},
		},
		{
			name: "round change with malformed prepared certificate",
			message: &proto.Message{
				View: view,
				Type: proto.MessageType_ROUND_CHANGE,
				Payload: &proto.Message_RoundChangeData{
					RoundChangeData: &proto.RoundChangeMessage{
						LastPreparedProposal:      proposal,
						LatestPreparedCertificate: &proto.PreparedCertificate{},
					},
				},
			},
		},
		{
			name: "preprepare with nil certificate message",
			message: &proto.Message{
				View: view,
				Type: proto.MessageType_PREPREPARE,
				Payload: &proto.Message_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						Proposal: proposal,
						Certificate: &proto.RoundChangeCertificate{
							RoundChangeMessages: []*proto.Message{nil},
						},
					},
				},
			},
		},
		{
			name:    "valid preprepare",
			message: validPreprepare,
			valid:   true,
		},
		{
			name:    "valid prepare",
			message: validPrepare,
			valid:   true,
		},
		{
			name:    "valid commit",
			message: validCommit,
			valid:   true,
		},
		{
			name:    "valid unprepared round change",
			message: validUnpreparedRoundChange,
			valid:   true,
		},
		{
			name:    "valid prepared round change",
			message: validPreparedRoundChange,
			valid:   true,
		},
		{
			name:    "valid preprepare with round change certificate",
			message: validPreprepareWithCertificate,
			valid:   true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.valid, isStructurallyValidIBFTMessage(test.message))
		})
	}
}

/* FINISH ALPHA CHANGE 1845 -- test structural consensus message admission -- */
