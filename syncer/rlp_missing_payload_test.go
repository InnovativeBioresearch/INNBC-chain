package syncer

/* START ALPHA CHANGE 2435 -- reject malformed peer block payloads without panicking -- */

import (
	"fmt"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/syncer/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func TestRLPTypePayload_SyncDecode(t *testing.T) {
	for _, txType := range []types.TxType{
		types.LegacyTx,
		types.DynamicFeeTx,
		types.StateTx,
	} {
		t.Run(fmt.Sprintf("missing_type_%02x_payload", byte(txType)), func(t *testing.T) {
			arena := new(fastrlp.Arena)
			header := &types.Header{Number: 1, GasLimit: 1000000}

			transactions := arena.NewArray()
			transactions.Set(arena.NewCopyBytes([]byte{byte(txType)}))

			rawBlock := arena.NewArray()
			rawBlock.Set(header.MarshalRLPWith(arena))
			rawBlock.Set(transactions)
			rawBlock.Set(arena.NewNullArray())

			response := &proto.Block{Block: rawBlock.MarshalTo(nil)}

			var decoded *types.Block
			var err error

			require.NotPanics(t, func() {
				decoded, err = fromProto(response)
			})
			require.ErrorContains(t, err, "missing payload after transaction type")
			require.Nil(t, decoded)
		})
	}

	t.Run("valid_block_after_rejections", func(t *testing.T) {
		expected := &types.Block{
			Header: &types.Header{Number: 1, GasLimit: 1000000},
		}
		encoded := expected.MarshalRLP()

		decoded, err := fromProto(&proto.Block{Block: encoded})
		require.NoError(t, err)
		require.NotNil(t, decoded)
		require.Equal(t, encoded, decoded.MarshalRLP())
	})
}

/* FINISH ALPHA CHANGE 2435 -- reject malformed peer block payloads without panicking -- */
