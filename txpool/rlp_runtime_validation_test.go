/* START ALPHA CHANGE 3014 -- verify size-first decoding and rejection before pool work -- */
package txpool

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/golang/protobuf/ptypes/any"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func runtimeRLPAdmissionTransaction() *types.Transaction {
	to := types.StringToAddress("0x9002")
	return &types.Transaction{
		Type:     types.LegacyTx,
		GasPrice: big.NewInt(1),
		Gas:      1_000_000,
		To:       &to,
		Value:    new(big.Int),
		V:        big.NewInt(27),
		R:        big.NewInt(1),
		S:        big.NewInt(1),
	}
}

func TestRuntimeRLP_RawAdmissionSizeBoundary(t *testing.T) {
	tx := runtimeRLPAdmissionTransaction()
	tx.Input = make([]byte, MaxTransactionSize)

	overhead := len(tx.MarshalRLP()) - len(tx.Input)
	tx.Input = tx.Input[:MaxTransactionSize-overhead]
	exact := tx.MarshalRLP()
	require.Len(t, exact, MaxTransactionSize)

	decoded, err := DecodeRawTransaction(exact)
	require.NoError(t, err)
	require.Equal(t, exact, decoded.MarshalRLP())

	tx.Input = append(tx.Input, 0)
	over := tx.MarshalRLP()
	require.Len(t, over, MaxTransactionSize+1)

	decoded, err = DecodeRawTransaction(over)
	require.ErrorIs(t, err, ErrOversizedData)
	require.Nil(t, decoded)

	// Size policy wins before even an invalid type prefix is inspected.
	decoded, err = DecodeRawTransaction(bytes.Repeat([]byte{0x01}, MaxTransactionSize+1))
	require.ErrorIs(t, err, ErrOversizedData)
	require.Nil(t, decoded)
}

type runtimeRLPAdmissionStore struct {
	defaultMockStore
	reads int
}

func (s *runtimeRLPAdmissionStore) Header() *types.Header {
	s.reads++
	return s.defaultMockStore.Header()
}

func (s *runtimeRLPAdmissionStore) GetNonce(root types.Hash, address types.Address) (uint64, error) {
	s.reads++
	return s.defaultMockStore.GetNonce(root, address)
}

func (s *runtimeRLPAdmissionStore) GetBalance(root types.Hash, address types.Address) (*big.Int, error) {
	s.reads++
	return s.defaultMockStore.GetBalance(root, address)
}

func (s *runtimeRLPAdmissionStore) CalculateBaseFee(header *types.Header) (uint64, error) {
	s.reads++
	return s.defaultMockStore.CalculateBaseFee(header)
}

type runtimeRLPAdmissionSigner struct {
	calls int
}

func (s *runtimeRLPAdmissionSigner) Sender(*types.Transaction) (types.Address, error) {
	s.calls++
	return types.ZeroAddress, errors.New("unexpected signature recovery")
}

func TestRuntimeRLP_GossipAndOperatorRejectBeforeAdmission(t *testing.T) {
	valid := runtimeRLPAdmissionTransaction().MarshalRLP()

	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"oversized", bytes.Repeat([]byte{0x01}, MaxTransactionSize+1), ErrOversizedData},
		{"deep_lists", []byte{0xc5, 0xc4, 0xc3, 0xc2, 0xc1, 0xc0}, types.ErrInvalidRLPStructure},
		{"truncated", []byte{0xf8}, types.ErrInvalidRLPStructure},
		{"trailing_data", append(append([]byte(nil), valid...), 0x80), types.ErrInvalidRLPStructure},
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

				switch role {
				case "gossip":
					pool.addGossipTx(&proto.Txn{
						Raw: &any.Any{Value: tc.raw},
					}, "")
				case "operator":
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

func TestRuntimeRLP_OperatorMissingRawRequest(t *testing.T) {
	pool := new(TxPool)
	for _, request := range []*proto.AddTxnReq{nil, {}} {
		response, err := pool.AddTxn(context.Background(), request)
		require.Error(t, err)
		require.Nil(t, response)
	}
}

func TestRuntimeRLP_ValidGossipAndOperatorAdmission(t *testing.T) {
	for _, role := range []string{"gossip", "operator"} {
		t.Run(role, func(t *testing.T) {
			header := mockHeader.Copy()
			header.BaseFee = 1
			pool, err := newTestPool(NewDefaultMockStore(header))
			require.NoError(t, err)
			t.Cleanup(pool.Close)

			signer := crypto.NewLondonSigner(
				100, true, crypto.NewEIP155Signer(100, true),
			)
			pool.SetSigner(signer)
			pool.SetSealing(true)
			require.NoError(t, pool.SetBaseFee(header))
			pool.Start()

			key, err := crypto.GenerateECDSAKey()
			require.NoError(t, err)
			to := types.StringToAddress("0x9002")
			signed, err := signer.SignTx(&types.Transaction{
				Type:      types.DynamicFeeTx,
				ChainID:   big.NewInt(100),
				GasPrice:  new(big.Int),
				GasTipCap: big.NewInt(1),
				GasFeeCap: big.NewInt(2),
				Gas:       21000,
				To:        &to,
				Value:     big.NewInt(1),
			}, key)
			require.NoError(t, err)

			raw := signed.MarshalRLP()
			signed.ComputeHash(header.Number)

			if role == "gossip" {
				pool.addGossipTx(&proto.Txn{
					Raw: &any.Any{Value: raw},
				}, "")
			} else {
				response, err := pool.AddTxn(context.Background(), &proto.AddTxnReq{
					Raw: &any.Any{Value: raw},
				})
				require.NoError(t, err)
				require.Equal(t, signed.Hash.String(), response.TxHash)
			}

			require.Equal(t, uint64(1), pool.gauge.read())
		})
	}
}

/* FINISH ALPHA CHANGE 3014 -- verify size-first decoding and rejection before pool work -- */
