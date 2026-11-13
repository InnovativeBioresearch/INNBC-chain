/* START ALPHA CHANGE 2938 -- verify raw submission preserves signed bytes and rejects altered fields -- */
package jsonrpc

import (
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/forkmanager"
	"github.com/innovativebioresearch/innbc-chain/txpool"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

type dynamicFeeFieldsPoolStore struct {
	header *types.Header
}

func (s *dynamicFeeFieldsPoolStore) Header() *types.Header {
	return s.header
}

func (*dynamicFeeFieldsPoolStore) GetNonce(
	types.Hash, types.Address,
) (uint64, error) {
	return 0, nil
}

func (*dynamicFeeFieldsPoolStore) GetBalance(
	types.Hash, types.Address,
) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}

func (*dynamicFeeFieldsPoolStore) GetBlockByHash(
	types.Hash, bool,
) (*types.Block, bool) {
	return nil, false
}

func (*dynamicFeeFieldsPoolStore) CalculateBaseFee(
	header *types.Header,
) (uint64, error) {
	return header.BaseFee, nil
}

type dynamicFeeFieldsRPCStore struct {
	ethStore
	pool     *txpool.TxPool
	calls    int
	accepted *types.Transaction
}

func (s *dynamicFeeFieldsRPCStore) AddTx(tx *types.Transaction) error {
	s.calls++

	if err := s.pool.AddTx(tx); err != nil {
		return err
	}

	s.accepted = tx

	return nil
}

func dynamicFeeFieldsEndpoint(
	t *testing.T,
) (*Eth, *dynamicFeeFieldsRPCStore, *types.Transaction) {
	t.Helper()

	// Install the real typed transaction-hash implementation without
	// clearing unrelated fork registrations. These tests are not parallel.
	const testFork = "dynamic_fee_fields_rpc_test"

	fm := forkmanager.GetInstance()
	fm.RegisterFork(testFork, nil)
	require.NoError(t, fm.RegisterHandler(
		testFork,
		forkmanager.HandlerDesc("txHash"),
		&types.TransactionHashForkV2{},
	))
	require.NoError(t, fm.ActivateFork(testFork, 0))
	t.Cleanup(func() {
		require.NoError(t, fm.DeactivateFork(testFork))
	})

	store := &dynamicFeeFieldsPoolStore{
		header: &types.Header{
			GasLimit: 1_000_000,
			BaseFee:  1,
		},
	}

	pool, err := txpool.NewTxPool(
		hclog.NewNullLogger(),
		chain.AllForksEnabled.Copy(),
		store,
		nil,
		nil,
		&txpool.Config{
			ChainID:            big.NewInt(100),
			PriceLimit:         1,
			MaxSlots:           64,
			MaxAccountEnqueued: 16,
		},
	)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	signer := crypto.NewLondonSigner(
		100, true, crypto.NewEIP155Signer(100, true),
	)
	pool.SetSigner(signer)
	require.NoError(t, pool.SetBaseFee(store.header))
	pool.Start()

	key, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	to := types.StringToAddress("0x9002")
	signed, err := signer.SignTx(&types.Transaction{
		Type:      types.DynamicFeeTx,
		ChainID:   big.NewInt(100),
		To:        &to,
		Gas:       21000,
		Value:     big.NewInt(1),
		GasPrice:  new(big.Int),
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
	}, key)
	require.NoError(t, err)

	rpcStore := &dynamicFeeFieldsRPCStore{pool: pool}

	return newTestEthEndpoint(rpcStore), rpcStore, signed
}

func dynamicFeeFieldsReplaceAccessList(
	t *testing.T,
	raw []byte,
	kind string,
) []byte {
	t.Helper()

	p := &fastrlp.Parser{}
	value, err := p.Parse(raw[1:])
	require.NoError(t, err)

	fields, err := value.GetElems()
	require.NoError(t, err)
	require.Len(t, fields, 12)

	a := &fastrlp.Arena{}

	var accessList *fastrlp.Value
	switch kind {
	case "empty_bytes":
		accessList = a.NewNull()
	case "nonempty_bytes":
		accessList = a.NewBytes([]byte{1})
	case "nonempty_list":
		entry := a.NewArray()
		entry.Set(a.NewBytes(make([]byte, 20)))

		keys := a.NewArray()
		keys.Set(a.NewBytes(make([]byte, 32)))
		entry.Set(keys)

		accessList = a.NewArray()
		accessList.Set(entry)
	default:
		t.Fatalf("unknown access-list fixture %q", kind)
	}

	payload := a.NewArray()
	for i, field := range fields {
		if i == 8 {
			payload.Set(accessList)
		} else {
			payload.Set(field)
		}
	}

	return append([]byte{byte(types.DynamicFeeTx)}, payload.MarshalTo(nil)...)
}

func TestDynamicFeeFields_RawSubmissionHash(t *testing.T) {
	endpoint, store, signed := dynamicFeeFieldsEndpoint(t)

	raw := append([]byte(nil), signed.MarshalRLP()...)
	expectedHash := crypto.Keccak256Hash(raw).String()

	result, err := endpoint.SendRawTransaction(argBytes(raw))
	require.NoError(t, err)
	require.Equal(t, expectedHash, result)
	require.Equal(t, 1, store.calls)
	require.NotNil(t, store.accepted)
	require.Equal(t, raw, store.accepted.MarshalRLP())
	require.Equal(t, big.NewInt(100), store.accepted.ChainID)
	require.Equal(t, expectedHash, store.accepted.Hash.String())
}

func TestDynamicFeeFields_RawSubmissionRejectsAlteredID(t *testing.T) {
	high := new(big.Int).Lsh(big.NewInt(1), 64)
	high.Add(high, big.NewInt(100))

	tests := []struct {
		name string
		id   *big.Int
	}{
		{"zero", new(big.Int)},
		{"different", big.NewInt(101)},
		{"same_low_64_bits", high},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, store, signed := dynamicFeeFieldsEndpoint(t)

			// Preserve the signature and alter only the encoded chain ID.
			signed.ChainID = new(big.Int).Set(tt.id)
			raw := signed.MarshalRLP()

			result, err := endpoint.SendRawTransaction(argBytes(raw))
			require.ErrorIs(t, err, types.ErrInvalidTxChainID)
			require.Nil(t, result)
			require.Equal(t, 1, store.calls)
			require.Nil(t, store.accepted)
		})
	}
}

func TestDynamicFeeFields_RawSubmissionRejectsAccessList(t *testing.T) {
	tests := []struct {
		name string
		want error
	}{
		{"empty_bytes", types.ErrInvalidTxAccessList},
		{"nonempty_bytes", types.ErrInvalidTxAccessList},
		{"nonempty_list", types.ErrUnsupportedTxAccessList},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, store, signed := dynamicFeeFieldsEndpoint(t)

			raw := dynamicFeeFieldsReplaceAccessList(
				t, signed.MarshalRLP(), tt.name,
			)

			result, err := endpoint.SendRawTransaction(argBytes(raw))
			require.ErrorIs(t, err, tt.want)
			require.Nil(t, result)

			// The decoder rejects the field before entering the pool.
			require.Zero(t, store.calls)
			require.Nil(t, store.accepted)
		})
	}
}

/* FINISH ALPHA CHANGE 2938 -- verify raw submission preserves signed bytes and rejects altered fields -- */
