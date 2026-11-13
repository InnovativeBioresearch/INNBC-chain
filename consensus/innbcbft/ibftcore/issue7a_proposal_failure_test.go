/* START ALPHA CHANGE 2188 -- prevent preprepare construction after an empty proposal result -- */
package ibftcore

import (
	"context"
	"fmt"
	"testing"
	"time"

	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/stretchr/testify/require"
)

type issue7AProposalBuildBackend struct {
	Backend

	proposal        []byte
	buildCalls      int
	preprepareCalls int
	buildView       *proto.View
	preprepareView  *proto.View
	certificate     *proto.RoundChangeCertificate
	rawProposal     []byte
	result          *proto.Message
}

func (b *issue7AProposalBuildBackend) BuildProposal(view *proto.View) []byte {
	b.buildCalls++
	b.buildView = view

	return b.proposal
}

/* START ALPHA CHANGE 2587 -- preserve unsuccessful-build coverage with execution results -- */
func (b *issue7AProposalBuildBackend) BuildExecutedProposal(
	view *proto.View,
) ([]byte, *ProposalExecution) {
	raw := b.BuildProposal(view)
	if len(raw) == 0 {
		return raw, nil
	}

	return raw, &ProposalExecution{Value: append([]byte(nil), raw...)}
}

/* FINISH ALPHA CHANGE 2587 -- preserve unsuccessful-build coverage with execution results -- */

func (b *issue7AProposalBuildBackend) BuildPrePrepareMessage(
	rawProposal []byte,
	certificate *proto.RoundChangeCertificate,
	view *proto.View,
) *proto.Message {
	b.preprepareCalls++
	b.rawProposal = rawProposal
	b.certificate = certificate
	b.preprepareView = view

	return b.result
}

func TestIssue7A_ProposalBuildStopsBeforeEmptyPreprepare(t *testing.T) {
	for _, round := range []uint64{0, 1} {
		for _, testCase := range []struct {
			name     string
			proposal []byte
		}{
			{name: "nil", proposal: nil},
			{name: "empty", proposal: []byte{}},
			{name: "successful", proposal: roundChangePolicyProposal},
		} {
			t.Run(fmt.Sprintf("round_%d/%s", round, testCase.name), func(t *testing.T) {
				engine := newRoundChangePolicyTestEngine(t, nil)
				view := &proto.View{Height: 100, Round: round}
				engine.state.setView(view)

				backend := &issue7AProposalBuildBackend{
					Backend:  engine.backend,
					proposal: testCase.proposal,
					result: &proto.Message{
						View: view,
						Type: proto.MessageType_PREPREPARE,
					},
				}
				engine.backend = backend

				if round > 0 {
					engine.AddMessage(newRoundChangePolicyMessage("A", 100, round))
					engine.AddMessage(newRoundChangePolicyMessage("B", 100, round))
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				result := engine.buildProposal(ctx, view)

				// Also proves the later-round case reached construction,
				// rather than returning nil because its RCC timed out.
				require.Equal(t, 1, backend.buildCalls)
				require.Equal(t, view, backend.buildView)

				if len(testCase.proposal) == 0 {
					require.Nil(t, result)
					require.Zero(t, backend.preprepareCalls)

					return
				}

				require.Same(t, backend.result, result)
				require.Equal(t, 1, backend.preprepareCalls)
				require.Equal(t, testCase.proposal, backend.rawProposal)
				require.Equal(t, view, backend.preprepareView)

				if round == 0 {
					require.Nil(t, backend.certificate)
				} else {
					require.NotNil(t, backend.certificate)
					require.Len(t, backend.certificate.RoundChangeMessages, 2)
				}
			})
		}
	}
}

/* FINISH ALPHA CHANGE 2188 -- prevent preprepare construction after an empty proposal result -- */
