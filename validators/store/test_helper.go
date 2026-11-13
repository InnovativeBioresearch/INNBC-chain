package store

import (
	/* START ALPHA CHANGE 742 -- remove legacy signer dependency from validator store test helper -- */
	/* FINISH ALPHA CHANGE 742 -- remove legacy signer dependency from validator store test helper -- */
	"github.com/innovativebioresearch/innbc-chain/types"
)

// Utilities for test
const (
	TestEpochSize = 100
)

/* START ALPHA CHANGE 743 -- remove unused legacy mock signer helper -- */
/* FINISH ALPHA CHANGE 743 -- remove unused legacy mock signer helper -- */

type MockBlockchain struct {
	HeaderFn            func() *types.Header
	GetHeaderByNumberFn func(uint64) (*types.Header, bool)
}

func (m *MockBlockchain) Header() *types.Header {
	return m.HeaderFn()
}

func (m *MockBlockchain) GetHeaderByNumber(height uint64) (*types.Header, bool) {
	return m.GetHeaderByNumberFn(height)
}
