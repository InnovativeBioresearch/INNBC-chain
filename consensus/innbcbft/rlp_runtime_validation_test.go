/* START ALPHA CHANGE 3017 -- reject malformed proposal RLP before consensus execution -- */
package innbcbft

import (
	"testing"

	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRLP_ProposalRejectsBeforeExecution(t *testing.T) {
	valid := (&types.Block{
		Header: &types.Header{Number: 1, GasLimit: 1_000_000},
	}).MarshalRLP()

	for _, raw := range [][]byte{
		{0xc5, 0xc4, 0xc3, 0xc2, 0xc1, 0xc0},
		{0xf8},
		{0xc3, 0xc0, 0xc0, 0xc0},
		append(append([]byte(nil), valid...), 0x80),
	} {
		// No backend/configuration is installed. Rejection must happen
		// during decoding before header validation or execution uses it.
		f := new(fsm)
		var err error
		require.NotPanics(t, func() {
			err = f.Validate(raw)
		})
		require.ErrorIs(t, err, types.ErrInvalidRLPStructure)
		require.Nil(t, f.target)

		runtime := new(consensusRuntime)
		accepted := true
		require.NotPanics(t, func() {
			accepted = runtime.IsValidProposalHash(
				&proto.Proposal{RawProposal: raw},
				make([]byte, 32),
			)
		})
		require.False(t, accepted)
	}
}

/* FINISH ALPHA CHANGE 3017 -- reject malformed proposal RLP before consensus execution -- */
