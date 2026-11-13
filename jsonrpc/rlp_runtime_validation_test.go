/* START ALPHA CHANGE 3015 -- reject malformed raw RPC transactions before pool admission -- */
package jsonrpc

import (
	"bytes"
	"errors"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/txpool"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type runtimeRLPSubmissionStore struct {
	ethStore
	calls int
}

func (s *runtimeRLPSubmissionStore) AddTx(*types.Transaction) error {
	s.calls++
	return errors.New("unexpected pool admission")
}

func TestRuntimeRLP_RPCRejectsBeforeAdmission(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []byte
		want error
	}{
		{
			"oversized_invalid_type",
			bytes.Repeat([]byte{0x01}, txpool.MaxTransactionSize+1),
			txpool.ErrOversizedData,
		},
		{
			"deep_canonical_lists",
			[]byte{0xc5, 0xc4, 0xc3, 0xc2, 0xc1, 0xc0},
			types.ErrInvalidRLPStructure,
		},
		{"truncated_length", []byte{0xf8}, types.ErrInvalidRLPStructure},
		{"trailing_item", []byte{0xc0, 0x80}, types.ErrInvalidRLPStructure},
		{"byte_root", []byte{0x80}, types.ErrInvalidRLPStructure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := new(runtimeRLPSubmissionStore)
			endpoint := &Eth{store: store}

			result, err := endpoint.SendRawTransaction(argBytes(tc.raw))
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, result)
			require.Zero(t, store.calls)
		})
	}
}

func TestRuntimeRLP_RPCValidSignedSubmission(t *testing.T) {
	endpoint, store, signed := dynamicFeeFieldsEndpoint(t)
	raw := signed.MarshalRLP()

	result, err := endpoint.SendRawTransaction(argBytes(raw))
	require.NoError(t, err)
	require.Equal(t, crypto.Keccak256Hash(raw).String(), result)
	require.Equal(t, 1, store.calls)
	require.Equal(t, raw, store.accepted.MarshalRLP())
}

/* FINISH ALPHA CHANGE 3015 -- reject malformed raw RPC transactions before pool admission -- */
