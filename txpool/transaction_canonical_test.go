/* START ALPHA CHANGE 3030 -- reject canonical-field violations before gossip and operator pool work -- */
package txpool

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/any"
	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func canonicalPoolReplaceField(
	t *testing.T,
	raw []byte,
	index int,
	replacement []byte,
) []byte {
	t.Helper()

	parser := new(fastrlp.Parser)
	value, err := parser.Parse(raw)
	require.NoError(t, err)
	fields, err := value.GetElems()
	require.NoError(t, err)

	arena := new(fastrlp.Arena)
	list := arena.NewArray()
	for i, field := range fields {
		if i == index {
			list.Set(arena.NewCopyBytes(replacement))
		} else {
			list.Set(field)
		}
	}

	return list.MarshalTo(nil)
}

func TestTransactionCanonical_GossipAndOperator(t *testing.T) {
	transaction := runtimeRLPAdmissionTransaction()
	transaction.To = nil
	valid := transaction.MarshalRLP()

	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{
			"one_byte_recipient",
			canonicalPoolReplaceField(t, valid, 3, []byte{1}),
			types.ErrNonCanonicalTxEncoding,
		},
		{
			"nonce_zero_byte",
			canonicalPoolReplaceField(t, valid, 0, []byte{0}),
			types.ErrNonCanonicalTxEncoding,
		},
		{
			"price_leading_zero",
			canonicalPoolReplaceField(t, valid, 1, []byte{0, 1}),
			types.ErrNonCanonicalTxEncoding,
		},
		{
			"redundant_legacy_prefix",
			append([]byte{0}, valid...),
			types.ErrNonCanonicalTxEncoding,
		},
		{
			"trailing_data",
			append(append([]byte(nil), valid...), 0x80),
			types.ErrInvalidRLPStructure,
		},
	}

	for _, role := range []string{"gossip", "operator"} {
		for _, tc := range tests {
			t.Run(role+"/"+tc.name, func(t *testing.T) {
				header := mockHeader.Copy()
				header.BaseFee = 1
				store := &runtimeRLPAdmissionStore{
					defaultMockStore: NewDefaultMockStore(header),
				}
				pool, err := newTestPool(store)
				require.NoError(t, err)
				t.Cleanup(pool.Close)

				signer := new(runtimeRLPAdmissionSigner)
				pool.SetSigner(signer)
				pool.SetSealing(true)
				require.NoError(t, pool.SetBaseFee(header))
				store.reads = 0

				if role == "gossip" {
					pool.addGossipTx(&proto.Txn{
						Raw: &any.Any{Value: tc.raw},
					}, "")
				} else {
					response, err := pool.AddTxn(context.Background(), &proto.AddTxnReq{
						Raw: &any.Any{Value: tc.raw},
					})
					require.ErrorIs(t, err, tc.want)
					require.Nil(t, response)
				}

				require.Zero(t, signer.calls)
				require.Zero(t, store.reads)
				require.Zero(t, pool.gauge.read())
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3030 -- reject canonical-field violations before gossip and operator pool work -- */
