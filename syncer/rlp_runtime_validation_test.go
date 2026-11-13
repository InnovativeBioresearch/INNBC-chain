/* START ALPHA CHANGE 3016 -- protect downloaded block decoding through the shared entry point -- */
package syncer

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRLP_SyncBlockDecoding(t *testing.T) {
	valid := (&types.Block{
		Header: &types.Header{Number: 1, GasLimit: 1_000_000},
	}).MarshalRLP()

	for _, raw := range [][]byte{
		{0xc5, 0xc4, 0xc3, 0xc2, 0xc1, 0xc0},
		{0xf8},
		{0xc3, 0xc0, 0xc0, 0xc0},
		append(append([]byte(nil), valid...), 0x80),
	} {
		block, err := fromProto(&proto.Block{Block: raw})
		require.ErrorIs(t, err, types.ErrInvalidRLPStructure)
		require.Nil(t, block)
	}

	block, err := fromProto(&proto.Block{Block: valid})
	require.NoError(t, err)
	require.Equal(t, valid, block.MarshalRLP())
}

/* FINISH ALPHA CHANGE 3016 -- protect downloaded block decoding through the shared entry point -- */
