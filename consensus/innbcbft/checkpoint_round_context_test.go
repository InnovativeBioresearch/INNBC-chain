package innbcbft

import (
	"testing"

	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2567 -- test real checkpoint round decoding and contextual rejection -- */

func TestCheckpointRoundContext_RuntimeHashValidation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		embeddedRound uint64
		proposalRound uint64
		missing       bool
		accepted      bool
	}{
		{name: "round zero", accepted: true},
		{
			name:          "matching later round",
			embeddedRound: 2,
			proposalRound: 2,
			accepted:      true,
		},
		{
			name:          "prepared block reused later",
			embeddedRound: 2,
			proposalRound: 5,
			accepted:      true,
		},
		{
			name:          "embedded round ahead",
			embeddedRound: 3,
			proposalRound: 2,
		},
		{
			name:          "huge embedded round at round zero",
			embeddedRound: 1_000_000_000_000_000_000,
		},
		{
			name:          "maximum embedded round at round zero",
			embeddedRound: ^uint64(0),
		},
		{
			name:    "missing checkpoint",
			missing: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extra := &Extra{}
			if !tc.missing {
				extra.Checkpoint = &CheckpointData{
					EpochNumber: 1,
					BlockRound:  tc.embeddedRound,
				}
			}

			block := &types.Block{
				Header: &types.Header{
					Number:    10,
					ExtraData: extra.MarshalRLPTo(nil),
				},
			}
			block.Header.ComputeHash()

			hash := types.ZeroHash
			if extra.Checkpoint != nil {
				var err error
				hash, err = extra.Checkpoint.Hash(0, block.Number(), block.Hash())
				require.NoError(t, err)
			}

			runtime := &consensusRuntime{}
			if tc.accepted {
				runtime.config = &runtimeConfig{
					blockchain: new(blockchainMock),
				}
			}

			/*
				Rejected cases deliberately have no runtime configuration.
				They must return before accessing the blockchain backend.
			*/
			require.NotPanics(t, func() {
				require.Equal(t, tc.accepted, runtime.IsValidProposalHash(
					&proto.Proposal{
						RawProposal: block.MarshalRLP(),
						Round:       tc.proposalRound,
					},
					hash.Bytes(),
				))
			})
		})
	}
}

/* FINISH ALPHA CHANGE 2567 -- test real checkpoint round decoding and contextual rejection -- */
