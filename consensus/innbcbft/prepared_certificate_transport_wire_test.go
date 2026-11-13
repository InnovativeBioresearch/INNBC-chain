// START ALPHA CHANGE 3790 -- test prepared certificates across protobuf encoding --
package innbcbft

import (
	"testing"

	ibftproto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"
)

// These tests check transport structure. Committee membership, signatures
// and sufficient voting power remain separate core/runtime checks.
func preparedTransportEnvelope(
	prepares []*ibftproto.Message,
	nested bool,
) *ibftproto.Message {
	proposal := &ibftproto.Proposal{
		RawProposal: []byte("prepared-block"),
		Round:       0,
	}
	preprepare := &ibftproto.Message{
		View: &ibftproto.View{Height: 100, Round: 0},
		Type: ibftproto.MessageType_PREPREPARE,
		Payload: &ibftproto.Message_PreprepareData{
			PreprepareData: &ibftproto.PrePrepareMessage{
				Proposal:     proposal,
				ProposalHash: []byte("prepared-hash"),
			},
		},
	}
	roundChange := &ibftproto.Message{
		View: &ibftproto.View{Height: 100, Round: 1},
		Type: ibftproto.MessageType_ROUND_CHANGE,
		Payload: &ibftproto.Message_RoundChangeData{
			RoundChangeData: &ibftproto.RoundChangeMessage{
				LastPreparedProposal: proposal,
				LatestPreparedCertificate: &ibftproto.PreparedCertificate{
					ProposalMessage: preprepare,
					PrepareMessages: prepares,
				},
			},
		},
	}
	if !nested {
		return roundChange
	}

	return &ibftproto.Message{
		View: &ibftproto.View{Height: 100, Round: 1},
		Type: ibftproto.MessageType_PREPREPARE,
		Payload: &ibftproto.Message_PreprepareData{
			PreprepareData: &ibftproto.PrePrepareMessage{
				Proposal: &ibftproto.Proposal{
					RawProposal: append([]byte(nil), proposal.RawProposal...),
					Round:       1,
				},
				ProposalHash: []byte("prepared-hash"),
				Certificate: &ibftproto.RoundChangeCertificate{
					RoundChangeMessages: []*ibftproto.Message{roundChange},
				},
			},
		},
	}
}

func preparedTransportCertificate(
	message *ibftproto.Message,
	nested bool,
) *ibftproto.PreparedCertificate {
	if nested {
		message = message.GetPreprepareData().Certificate.RoundChangeMessages[0]
	}

	return message.GetRoundChangeData().LatestPreparedCertificate
}

func preparedTransportRoundTrip(
	t *testing.T,
	message *ibftproto.Message,
) *ibftproto.Message {
	t.Helper()

	raw, err := protobuf.Marshal(message)
	require.NoError(t, err)

	decoded := new(ibftproto.Message)
	require.NoError(t, protobuf.Unmarshal(raw, decoded))

	return decoded
}

func TestPreparedCertificateTransportWire_EmptyPrepareRepresentations(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "round_change"
		if nested {
			name = "preprepare_with_round_change_certificate"
		}

		t.Run(name, func(t *testing.T) {
			nilMessage := preparedTransportEnvelope(nil, nested)
			emptyMessage := preparedTransportEnvelope(
				make([]*ibftproto.Message, 0),
				nested,
			)

			nilWire, err := protobuf.Marshal(nilMessage)
			require.NoError(t, err)
			emptyWire, err := protobuf.Marshal(emptyMessage)
			require.NoError(t, err)
			require.Equal(t, nilWire, emptyWire,
				"protobuf cannot preserve nil versus allocated-empty repeated lists")

			for _, representation := range []string{
				"allocated_empty",
				"nil",
				"wire_round_trip",
			} {
				t.Run(representation, func(t *testing.T) {
					message := preparedTransportEnvelope(
						make([]*ibftproto.Message, 0),
						nested,
					)

					switch representation {
					case "nil":
						preparedTransportCertificate(message, nested).PrepareMessages = nil
					case "wire_round_trip":
						message = preparedTransportRoundTrip(t, message)
						require.Nil(t,
							preparedTransportCertificate(message, nested).PrepareMessages)
					}

					require.True(t, isStructurallyValidIBFTMessage(message),
						"an empty prepare list is structurally valid; quorum is checked separately")
				})
			}
		})
	}
}

func TestPreparedCertificateTransportWire_InvalidPrepareMember(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, wire := range []bool{false, true} {
			name := "round_change"
			if nested {
				name = "nested_preprepare"
			}
			if wire {
				name += "/wire"
			} else {
				name += "/direct"
			}

			t.Run(name, func(t *testing.T) {
				message := preparedTransportEnvelope(
					[]*ibftproto.Message{nil},
					nested,
				)
				if wire {
					message = preparedTransportRoundTrip(t, message)
					require.Len(t,
						preparedTransportCertificate(message, nested).PrepareMessages, 1)
				}

				require.NotPanics(t, func() {
					require.False(t, isStructurallyValidIBFTMessage(message),
						"one missing/empty prepare message is not a zero-member list")
				})
			})
		}
	}
}

func TestPreparedCertificateTransportWire_DefaultValuedFields(t *testing.T) {
	for _, kind := range []ibftproto.MessageType{
		ibftproto.MessageType_PREPREPARE,
		ibftproto.MessageType_PREPARE,
		ibftproto.MessageType_COMMIT,
		ibftproto.MessageType_ROUND_CHANGE,
	} {
		t.Run(kind.String(), func(t *testing.T) {
			message := &ibftproto.Message{
				View: &ibftproto.View{Height: 100, Round: 0},
				Type: kind,
			}

			switch kind {
			case ibftproto.MessageType_PREPREPARE:
				message.Payload = &ibftproto.Message_PreprepareData{
					PreprepareData: &ibftproto.PrePrepareMessage{
						Proposal: &ibftproto.Proposal{
							RawProposal: []byte{1},
							Round:       0,
						},
					},
				}
			case ibftproto.MessageType_PREPARE:
				message.Payload = &ibftproto.Message_PrepareData{
					PrepareData: &ibftproto.PrepareMessage{
						ProposalHash: []byte{1},
					},
				}
			case ibftproto.MessageType_COMMIT:
				message.Payload = &ibftproto.Message_CommitData{
					CommitData: &ibftproto.CommitMessage{
						ProposalHash:  []byte{1},
						CommittedSeal: []byte{2},
					},
				}
			case ibftproto.MessageType_ROUND_CHANGE:
				message.Payload = &ibftproto.Message_RoundChangeData{
					RoundChangeData: &ibftproto.RoundChangeMessage{},
				}
			}

			decoded := preparedTransportRoundTrip(t, message)
			require.NotNil(t, decoded.View)
			require.Zero(t, decoded.View.Round)
			require.Equal(t, kind, decoded.Type)
			require.True(t, isStructurallyValidIBFTMessage(decoded))

			decoded.View = nil
			require.False(t, isStructurallyValidIBFTMessage(decoded))
		})
	}
}

// FINISH ALPHA CHANGE 3790 -- test prepared certificates across protobuf encoding --
