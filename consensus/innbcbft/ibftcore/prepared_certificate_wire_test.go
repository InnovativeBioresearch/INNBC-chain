// START ALPHA CHANGE 3791 -- preserve quorum and authorization across certificate encoding --
package ibftcore

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	ibftproto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	innbcwallet "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/stretchr/testify/require"
	ethwallet "github.com/umbracle/ethgo/wallet"
	protobuf "google.golang.org/protobuf/proto"
)

// Checkpoint/raw-proposal hashing uses the existing synthetic context fixture.
// Validator authorization uses real ECDSA signing and recovery.
type preparedWireBackend struct {
	*checkpointContextBackend
	proposer   []byte
	validators map[string]struct{}
}

func (b *preparedWireBackend) IsValidValidator(message *ibftproto.Message) bool {
	if message == nil {
		return false
	}
	if _, exists := b.validators[string(message.From)]; !exists {
		return false
	}

	payload, err := message.PayloadNoSig()
	if err != nil {
		return false
	}
	recovered, err := innbcwallet.RecoverAddressFromSignature(message.Signature, payload)

	return err == nil && bytes.Equal(message.From, recovered.Bytes())
}

func (b *preparedWireBackend) IsProposer(id []byte, _, _ uint64) bool {
	return bytes.Equal(id, b.proposer)
}

func (b *preparedWireBackend) ID() []byte {
	return append([]byte(nil), b.proposer...)
}

func preparedWireKey(t *testing.T, number byte) *innbcwallet.Key {
	t.Helper()

	raw := make([]byte, 32)
	raw[31] = number
	key, err := ethwallet.NewWalletFromPrivKey(raw)
	require.NoError(t, err)

	return innbcwallet.NewKey(&innbcwallet.Account{Ecdsa: key})
}

func preparedWireSign(
	t *testing.T,
	key *innbcwallet.Key,
	message *ibftproto.Message,
) *ibftproto.Message {
	t.Helper()

	message.Signature = nil
	signed, err := key.SignIBFTMessage(message)
	require.NoError(t, err)

	return signed
}

func newPreparedWireEngine(
	t *testing.T,
	count int,
) (*IBFT, *preparedWireBackend, []*innbcwallet.Key) {
	t.Helper()

	engine, contextBackend, _ := newCheckpointContextEngine(t)
	keys := make([]*innbcwallet.Key, count)
	powers := make(map[string]*big.Int, count)
	backend := &preparedWireBackend{
		checkpointContextBackend: contextBackend,
		validators:               make(map[string]struct{}, count),
	}

	for index := range keys {
		keys[index] = preparedWireKey(t, byte(index+1))
		id := keys[index].Address().Bytes()
		powers[string(id)] = big.NewInt(1)
		backend.validators[string(id)] = struct{}{}
	}

	backend.proposer = keys[0].Address().Bytes()
	engine.backend = backend
	require.NoError(t, engine.validatorManager.setCurrentVotingPower(powers))

	return engine, backend, keys
}

func preparedWirePrepare(
	t *testing.T,
	key *innbcwallet.Key,
	proposal *ibftproto.Message,
) *ibftproto.Message {
	t.Helper()

	return preparedWireSign(t, key, &ibftproto.Message{
		From: key.Address().Bytes(),
		View: &ibftproto.View{
			Height: proposal.View.Height,
			Round:  proposal.View.Round,
		},
		Type: ibftproto.MessageType_PREPARE,
		Payload: &ibftproto.Message_PrepareData{
			PrepareData: &ibftproto.PrepareMessage{
				ProposalHash: append([]byte(nil),
					proposal.GetPreprepareData().ProposalHash...),
			},
		},
	})
}

func preparedWireCertificate(
	t *testing.T,
	keys []*innbcwallet.Key,
	prepares int,
) *ibftproto.PreparedCertificate {
	t.Helper()

	proposal := checkpointContextProposal(0, 0)
	proposal.From = keys[0].Address().Bytes()
	preparedWireSign(t, keys[0], proposal)

	certificate := &ibftproto.PreparedCertificate{
		ProposalMessage: proposal,
		PrepareMessages: make([]*ibftproto.Message, 0, prepares),
	}
	for index := 0; index < prepares; index++ {
		certificate.PrepareMessages = append(certificate.PrepareMessages,
			preparedWirePrepare(t, keys[index+1], proposal))
	}

	return certificate
}

func preparedWireRoundTrip(
	t *testing.T,
	certificate *ibftproto.PreparedCertificate,
) *ibftproto.PreparedCertificate {
	t.Helper()

	raw, err := protobuf.Marshal(certificate)
	require.NoError(t, err)

	decoded := new(ibftproto.PreparedCertificate)
	require.NoError(t, protobuf.Unmarshal(raw, decoded))

	return decoded
}

func TestPreparedCertificateWire_QuorumMatrix(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 6} {
		for prepares := 0; prepares < count; prepares++ {
			for _, representation := range []string{"allocated", "nil", "wire"} {
				if representation == "nil" && prepares != 0 {
					continue
				}

				name := fmt.Sprintf("validators_%d/prepares_%d/%s",
					count, prepares, representation)

				t.Run(name, func(t *testing.T) {
					engine, backend, keys := newPreparedWireEngine(t, count)
					certificate := preparedWireCertificate(t, keys, prepares)

					switch representation {
					case "nil":
						certificate.PrepareMessages = nil
					case "wire":
						certificate = preparedWireRoundTrip(t, certificate)
						if prepares == 0 {
							require.Nil(t, certificate.PrepareMessages)
						}
					}

					require.True(t, backend.IsValidValidator(certificate.ProposalMessage))
					for _, message := range certificate.PrepareMessages {
						require.True(t, backend.IsValidValidator(message))
					}

					requiredVotes := (2*count)/3 + 1
					want := 1+prepares >= requiredVotes
					require.Equal(t, want, engine.validPC(certificate, 1, 100),
						"prepare quorum counts the proposer plus distinct prepare signers")
					require.Zero(t, backend.executions)
					require.Zero(t, backend.prepareVotes)
				})
			}
		}
	}
}

func TestPreparedCertificateWire_RejectsMalformedAndUnauthorized(t *testing.T) {
	cases := []string{
		"missing proposal",
		"missing proposal view",
		"missing prepare view",
		"missing prepare payload",
		"nil prepare member",
		"duplicate prepare signer",
		"proposer among prepares",
		"unknown signer",
		"invalid proposal signature",
		"invalid prepare signature",
		"wrong prepare height",
		"wrong prepare round",
		"wrong prepare hash",
		"wrong contained proposal round",
		"raw proposal hash mismatch",
	}

	for _, name := range cases {
		for _, wire := range []bool{false, true} {
			mode := "direct"
			if wire {
				mode = "wire"
			}

			t.Run(name+"/"+mode, func(t *testing.T) {
				engine, backend, keys := newPreparedWireEngine(t, 4)
				certificate := preparedWireCertificate(t, keys, 2)

				switch name {
				case "missing proposal":
					certificate.ProposalMessage = nil
				case "missing proposal view":
					certificate.ProposalMessage.View = nil
					preparedWireSign(t, keys[0], certificate.ProposalMessage)
				case "missing prepare view":
					certificate.PrepareMessages[0].View = nil
					preparedWireSign(t, keys[1], certificate.PrepareMessages[0])
				case "missing prepare payload":
					certificate.PrepareMessages[0].Payload = nil
					preparedWireSign(t, keys[1], certificate.PrepareMessages[0])
				case "nil prepare member":
					certificate.PrepareMessages[0] = nil
				case "duplicate prepare signer":
					certificate.PrepareMessages[1] =
						preparedWirePrepare(t, keys[1], certificate.ProposalMessage)
				case "proposer among prepares":
					certificate.PrepareMessages[0] =
						preparedWirePrepare(t, keys[0], certificate.ProposalMessage)
				case "unknown signer":
					// Keep the legitimate quorum intact, then add an
					// unauthorized signer so membership must be checked.
					certificate.PrepareMessages = append(
						certificate.PrepareMessages,
						preparedWirePrepare(t, preparedWireKey(t, 99),
							certificate.ProposalMessage),
					)
				case "invalid proposal signature":
					certificate.ProposalMessage.Signature = []byte{0}
				case "invalid prepare signature":
					certificate.PrepareMessages[0].Signature = []byte{0}
				case "wrong prepare height":
					certificate.PrepareMessages[0].View.Height++
					preparedWireSign(t, keys[1], certificate.PrepareMessages[0])
				case "wrong prepare round":
					certificate.PrepareMessages[0].View.Round++
					preparedWireSign(t, keys[1], certificate.PrepareMessages[0])
				case "wrong prepare hash":
					certificate.PrepareMessages[0].GetPrepareData().ProposalHash =
						[]byte("wrong")
					preparedWireSign(t, keys[1], certificate.PrepareMessages[0])
				case "wrong contained proposal round":
					certificate.ProposalMessage.GetPreprepareData().Proposal.Round++
					preparedWireSign(t, keys[0], certificate.ProposalMessage)
				case "raw proposal hash mismatch":
					// All declared hashes and signatures remain consistent.
					// Only the declared hash versus raw proposal is wrong.
					wrongHash := []byte("wrong-declared-hash")
					certificate.ProposalMessage.GetPreprepareData().ProposalHash =
						append([]byte(nil), wrongHash...)
					preparedWireSign(t, keys[0], certificate.ProposalMessage)

					for index, message := range certificate.PrepareMessages {
						message.GetPrepareData().ProposalHash =
							append([]byte(nil), wrongHash...)
						preparedWireSign(t, keys[index+1], message)
					}
				}

				if wire {
					certificate = preparedWireRoundTrip(t, certificate)
				}

				require.NotPanics(t, func() {
					require.False(t, engine.validPC(certificate, 1, 100))
				})
				if name == "raw proposal hash mismatch" {
					require.Equal(t, 1, backend.hashChecks)
				}
				require.Zero(t, backend.executions)
				require.Zero(t, backend.prepareVotes)
			})
		}
	}
}

func TestPreparedCertificateWire_ContextLimits(t *testing.T) {
	for _, wire := range []bool{false, true} {
		name := "direct"
		if wire {
			name = "wire"
		}

		t.Run(name, func(t *testing.T) {
			engine, backend, keys := newPreparedWireEngine(t, 4)
			certificate := preparedWireCertificate(t, keys, 2)
			if wire {
				certificate = preparedWireRoundTrip(t, certificate)
			}

			require.True(t, engine.validPC(certificate, 1, 100))
			require.False(t, engine.validPC(certificate, 0, 100),
				"the certified round must be strictly below the round limit")
			require.False(t, engine.validPC(certificate, 1, 101),
				"the certificate must match the expected height")
			require.True(t, engine.validPC(certificate, 1, 100),
				"rejected context checks must not alter the valid certificate")
			require.Zero(t, backend.executions)
			require.Zero(t, backend.prepareVotes)
		})
	}
}

// FINISH ALPHA CHANGE 3791 -- preserve quorum and authorization across certificate encoding --
