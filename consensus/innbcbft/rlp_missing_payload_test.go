package innbcbft

/* START ALPHA CHANGE 2436 -- reject missing proposal transaction payloads before consensus processing -- */

import (
	"fmt"
	"testing"

	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func TestRLPTypePayload_ProposalHashRejectsMalformedBlock(t *testing.T) {
	for _, txType := range []types.TxType{
		types.LegacyTx,
		types.DynamicFeeTx,
		types.StateTx,
	} {
		t.Run(fmt.Sprintf("type_%02x", byte(txType)), func(t *testing.T) {
			arena := new(fastrlp.Arena)
			header := &types.Header{Number: 1, GasLimit: 1000000}

			transactions := arena.NewArray()
			transactions.Set(arena.NewCopyBytes([]byte{byte(txType)}))

			rawBlock := arena.NewArray()
			rawBlock.Set(header.MarshalRLPWith(arena))
			rawBlock.Set(transactions)
			rawBlock.Set(arena.NewNullArray())

			proposal := &proto.Proposal{
				RawProposal: rawBlock.MarshalTo(nil),
			}

			/*
				Decoding must reject this input before requiring a
				blockchain backend or other consensus runtime state.
			*/
			runtime := &consensusRuntime{}
			accepted := true

			require.NotPanics(t, func() {
				accepted = runtime.IsValidProposalHash(proposal, make([]byte, 32))
			})
			require.False(t, accepted)
		})
	}
}

/* FINISH ALPHA CHANGE 2436 -- reject missing proposal transaction payloads before consensus processing -- */
