package txpool

import (
	"fmt"
	"math/big"

	"github.com/innovativebioresearch/innbc-chain/types"
)

var mockHeader = &types.Header{
	GasLimit: 4712388,
}

/* MOCK */

type defaultMockStore struct {
	DefaultHeader *types.Header

	getBlockByHashFn   func(types.Hash, bool) (*types.Block, bool)
	calculateBaseFeeFn func(*types.Header) (uint64, error)
	nonce              uint64
}

func NewDefaultMockStore(header *types.Header) defaultMockStore {
	return defaultMockStore{
		DefaultHeader: header,
		calculateBaseFeeFn: func(h *types.Header) (uint64, error) {
			return h.BaseFee, nil
		},
	}
}

func (m defaultMockStore) Header() *types.Header {
	return m.DefaultHeader
}

/* START ALPHA CHANGE 2165 -- migrate successful mock nonce reads -- */
func (m defaultMockStore) GetNonce(types.Hash, types.Address) (uint64, error) {
	return m.nonce, nil
}

/* FINISH ALPHA CHANGE 2165 -- migrate successful mock nonce reads -- */

func (m defaultMockStore) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	if m.getBlockByHashFn != nil {
		return m.getBlockByHashFn(hash, full)
	}

	return nil, false
}

func (m defaultMockStore) GetBalance(types.Hash, types.Address) (*big.Int, error) {
	balance := big.NewInt(0).SetUint64(100000000000000)

	return balance, nil
}

func (m defaultMockStore) CalculateBaseFee(header *types.Header) (uint64, error) {
	if m.calculateBaseFeeFn != nil {
		return m.calculateBaseFeeFn(header)
	}

	return 0, nil
}

type faultyMockStore struct {
}

func (fms faultyMockStore) Header() *types.Header {
	return &types.Header{}
}

/* START ALPHA CHANGE 2165 -- preserve the existing high-nonce fixture -- */
func (fms faultyMockStore) GetNonce(root types.Hash, addr types.Address) (uint64, error) {
	return 99999, nil
}

/* FINISH ALPHA CHANGE 2165 -- preserve the existing high-nonce fixture -- */

func (fms faultyMockStore) GetBlockByHash(hash types.Hash, b bool) (*types.Block, bool) {
	return nil, false
}

func (fms faultyMockStore) GetBalance(root types.Hash, addr types.Address) (*big.Int, error) {
	return nil, fmt.Errorf("unable to fetch account state")
}

func (fms faultyMockStore) CalculateBaseFee(*types.Header) (uint64, error) {
	return 0, nil
}

type mockSigner struct {
}

func (s *mockSigner) Sender(tx *types.Transaction) (types.Address, error) {
	return tx.From, nil
}
