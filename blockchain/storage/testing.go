package storage

import (
	/* START ALPHA CHANGE 3170 -- report unconfigured startup inspection in storage mocks -- */
	"fmt"
	/* FINISH ALPHA CHANGE 3170 -- report unconfigured startup inspection in storage mocks -- */
	"math/big"
	"reflect"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/helper/hex"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type PlaceholderStorage func(t *testing.T) (Storage, func())

/* START ALPHA CHANGE 1475 -- count storage writes when testing empty record omission -- */
type countingBatch struct {
	putCount int
}

func (b *countingBatch) Delete(_ []byte) {
}

func (b *countingBatch) Put(_, _ []byte) {
	b.putCount++
}

func (b *countingBatch) Write() error {
	return nil
}

/* FINISH ALPHA CHANGE 1475 -- count storage writes when testing empty record omission -- */

var (
	addr1 = types.StringToAddress("1")
	addr2 = types.StringToAddress("2")

	hash1 = types.StringToHash("1")
	hash2 = types.StringToHash("2")
)

// TestStorage tests a set of tests on a storage
func TestStorage(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	t.Run("testCanonicalChain", func(t *testing.T) {
		testCanonicalChain(t, m)
	})
	/* START ALPHA CHANGE 800 -- remove total-difficulty storage tests -- */
	/* FINISH ALPHA CHANGE 800 -- remove total-difficulty storage tests -- */
	t.Run("testHead", func(t *testing.T) {
		testHead(t, m)
	})
	/* START ALPHA CHANGE 801 -- remove fork storage tests -- */
	/* FINISH ALPHA CHANGE 801 -- remove fork storage tests -- */
	t.Run("testHeader", func(t *testing.T) {
		testHeader(t, m)
	})
	/* START ALPHA CHANGE 1488 -- test reconstructed stored header hash validation -- */
	t.Run("testHeaderHashMismatch", func(t *testing.T) {
		testHeaderHashMismatch(t, m)
	})
	/* FINISH ALPHA CHANGE 1488 -- test reconstructed stored header hash validation -- */
	t.Run("testBody", func(t *testing.T) {
		testBody(t, m)
	})
	t.Run("testWriteCanonicalHeader", func(t *testing.T) {
		testWriteCanonicalHeader(t, m)
	})
	t.Run("testReceipts", func(t *testing.T) {
		testReceipts(t, m)
	})
	/* START ALPHA CHANGE 1476 -- test lossless empty body and receipt omission -- */
	t.Run("testEmptyHeavyBlockDataOmission", func(t *testing.T) {
		testEmptyHeavyBlockDataOmission(t, m)
	})
	/* FINISH ALPHA CHANGE 1476 -- test lossless empty body and receipt omission -- */
	/* START ALPHA CHANGE 1589 -- remove superseded typed-delete storage test registration -- */
	/* FINISH ALPHA CHANGE 1589 -- remove superseded typed-delete storage test registration -- */
}

func testCanonicalChain(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	var cases = []struct {
		Number     uint64
		ParentHash types.Hash
		Hash       types.Hash
	}{
		{
			Number:     1,
			ParentHash: types.StringToHash("111"),
		},
		{
			Number:     1,
			ParentHash: types.StringToHash("222"),
		},
		{
			Number:     2,
			ParentHash: types.StringToHash("111"),
		},
	}

	for _, cc := range cases {
		batch := NewBatchWriter(s)

		h := &types.Header{
			Number:     cc.Number,
			ParentHash: cc.ParentHash,
			ExtraData:  []byte{0x1},
		}

		hash := h.Hash

		batch.PutHeader(h)
		batch.PutCanonicalHash(cc.Number, hash)

		require.NoError(t, batch.WriteBatch())

		data, ok := s.ReadCanonicalHash(cc.Number)
		if !ok {
			t.Fatal("not found")
		}

		if !reflect.DeepEqual(data, hash) {
			t.Fatal("not match")
		}
	}
}

/* START ALPHA CHANGE 802 -- prune total-difficulty storage test helper -- */
func testHead(t *testing.T, m PlaceholderStorage) {
	/* FINISH ALPHA CHANGE 802 -- prune total-difficulty storage test helper -- */
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	for i := uint64(0); i < 5; i++ {
		batch := NewBatchWriter(s)

		h := &types.Header{
			Number:    i,
			ExtraData: []byte{},
		}
		hash := h.Hash

		batch.PutHeader(h)
		batch.PutHeadNumber(i)
		batch.PutHeadHash(hash)

		require.NoError(t, batch.WriteBatch())

		n2, ok := s.ReadHeadNumber()
		if !ok {
			t.Fatal("num not found")
		}

		if n2 != i {
			t.Fatal("bad")
		}

		hash1, ok := s.ReadHeadHash()
		if !ok {
			t.Fatal("hash not found")
		}

		if !reflect.DeepEqual(hash1, hash) {
			t.Fatal("bad")
		}
	}
}

/* START ALPHA CHANGE 803 -- prune fork storage test helper -- */
func testHeader(t *testing.T, m PlaceholderStorage) {
	/* FINISH ALPHA CHANGE 803 -- prune fork storage test helper -- */
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	header := &types.Header{
		Number:     5,
		Difficulty: 17179869184,
		ParentHash: types.StringToHash("11"),
		Timestamp:  10,
		// if not set it will fail
		ExtraData: hex.MustDecodeHex("0x11bbe8db4e347b4e8c937c1c8370e4b5ed33adb3db69cbdb7a38e1e50b1b82fa"),
	}
	header.ComputeHash()

	batch := NewBatchWriter(s)

	batch.PutHeader(header)

	require.NoError(t, batch.WriteBatch())

	header1, err := s.ReadHeader(header.Hash)
	assert.NoError(t, err)

	if !reflect.DeepEqual(header, header1) {
		t.Fatal("bad")
	}
}

/* START ALPHA CHANGE 1489 -- reject reconstructed headers that do not match their storage key -- */
func testHeaderHashMismatch(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	header := &types.Header{
		ParentHash:   types.StringToHash("21"),
		Sha3Uncles:   types.EmptyUncleHash,
		StateRoot:    types.StringToHash("22"),
		TxRoot:       types.EmptyRootHash,
		ReceiptsRoot: types.EmptyRootHash,
		Difficulty:   1,
		Number:       8,
		GasLimit:     30000000,
		Timestamp:    16,
		MixHash:      types.INNBCBFTMixDigest,
		ExtraData:    []byte{0x1},
	}
	header.ComputeHash()

	storedHash := header.Hash

	// Keep the original storage key but mutate the encoded header value.
	header.Timestamp++

	batch := NewBatchWriter(s)
	batch.PutHeader(header)

	require.NoError(t, batch.WriteBatch())

	_, err := s.ReadHeader(storedHash)
	require.ErrorIs(t, err, ErrHeaderHashMismatch)
}

/* FINISH ALPHA CHANGE 1489 -- reject reconstructed headers that do not match their storage key -- */

func testBody(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	header := &types.Header{
		Number:     5,
		Difficulty: 10,
		ParentHash: types.StringToHash("11"),
		Timestamp:  10,
		ExtraData:  []byte{}, // if not set it will fail
	}

	/* START ALPHA CHANGE 1490 -- store test body header under its canonical hash -- */
	header.ComputeHash()
	/* FINISH ALPHA CHANGE 1490 -- store test body header under its canonical hash -- */

	batch := NewBatchWriter(s)

	batch.PutHeader(header)

	require.NoError(t, batch.WriteBatch())

	addr1 := types.StringToAddress("11")
	t0 := &types.Transaction{
		Nonce:    0,
		To:       &addr1,
		Value:    big.NewInt(1),
		Gas:      11,
		GasPrice: big.NewInt(11),
		Input:    []byte{1, 2},
		V:        big.NewInt(1),
	}
	t0.ComputeHash(1)

	addr2 := types.StringToAddress("22")
	t1 := &types.Transaction{
		Nonce:    0,
		To:       &addr2,
		Value:    big.NewInt(1),
		Gas:      22,
		GasPrice: big.NewInt(11),
		Input:    []byte{4, 5},
		V:        big.NewInt(2),
	}
	t1.ComputeHash(1)

	block := types.Block{
		Header:       header,
		Transactions: []*types.Transaction{t0, t1},
	}

	batch2 := NewBatchWriter(s)
	body0 := block.Body()

	batch2.PutBody(header.Hash, body0)

	require.NoError(t, batch2.WriteBatch())

	body1, err := s.ReadBody(header.Hash)
	/* START ALPHA CHANGE 1491 -- stop body storage test before dereferencing a failed read -- */
	require.NoError(t, err)
	require.NotNil(t, body1)
	/* FINISH ALPHA CHANGE 1491 -- stop body storage test before dereferencing a failed read -- */

	// NOTE: reflect.DeepEqual does not seem to work, check the hash of the transactions
	tx0, tx1 := body0.Transactions, body1.Transactions
	if len(tx0) != len(tx1) {
		t.Fatal("lengths are different")
	}

	for indx, i := range tx0 {
		if i.Hash != tx1[indx].Hash {
			t.Fatal("tx not correct")
		}
	}
}

func testReceipts(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	batch := NewBatchWriter(s)

	h := &types.Header{
		Difficulty: 133,
		Number:     11,
		ExtraData:  []byte{},
	}
	h.ComputeHash()

	body := &types.Body{
		Transactions: []*types.Transaction{
			{
				Nonce:    1000,
				Gas:      50,
				GasPrice: new(big.Int).SetUint64(100),
				V:        big.NewInt(11),
			},
		},
	}
	receipts := []*types.Receipt{
		{
			Root:              types.StringToHash("1"),
			CumulativeGasUsed: 10,
			TxHash:            body.Transactions[0].Hash,
			LogsBloom:         types.Bloom{0x1},
			Logs: []*types.Log{
				{
					Address: addr1,
					Topics:  []types.Hash{hash1, hash2},
					Data:    []byte{0x1, 0x2},
				},
				{
					Address: addr2,
					Topics:  []types.Hash{hash1},
				},
			},
		},
		{
			Root:              types.StringToHash("1"),
			CumulativeGasUsed: 10,
			TxHash:            body.Transactions[0].Hash,
			LogsBloom:         types.Bloom{0x1},
			GasUsed:           10,
			ContractAddress:   &types.Address{0x1},
			Logs: []*types.Log{
				{
					Address: addr2,
					Topics:  []types.Hash{hash1},
				},
			},
		},
	}

	batch.PutHeader(h)
	batch.PutBody(h.Hash, body)
	batch.PutReceipts(h.Hash, receipts)

	require.NoError(t, batch.WriteBatch())

	found, err := s.ReadReceipts(h.Hash)
	if err != nil {
		t.Fatal(err)
	}

	assert.True(t, reflect.DeepEqual(receipts, found))
}

/* START ALPHA CHANGE 1477 -- verify lossless empty body and receipt omission -- */
func testEmptyHeavyBlockDataOmission(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	header := &types.Header{
		Number:       12,
		Sha3Uncles:   types.EmptyUncleHash,
		TxRoot:       types.EmptyRootHash,
		ReceiptsRoot: types.EmptyRootHash,
		ExtraData:    []byte{},
	}
	header.ComputeHash()

	// Verify that only the header produces a physical batch Put.
	counting := &countingBatch{}
	countingWriter := &BatchWriter{batch: counting}

	countingWriter.PutHeader(header)
	countingWriter.PutBody(header.Hash, &types.Body{})
	countingWriter.PutReceipts(header.Hash, []*types.Receipt{})

	require.Equal(t, 1, counting.putCount)

	// Verify reconstruction through each real storage backend.
	s, closeFn := m(t)
	defer closeFn()

	batch := NewBatchWriter(s)
	batch.PutHeader(header)
	batch.PutBody(header.Hash, &types.Body{})
	batch.PutReceipts(header.Hash, []*types.Receipt{})

	require.NoError(t, batch.WriteBatch())

	body, err := s.ReadBody(header.Hash)
	require.NoError(t, err)
	require.NotNil(t, body)
	require.Empty(t, body.Transactions)
	require.Empty(t, body.Uncles)

	receipts, err := s.ReadReceipts(header.Hash)
	require.NoError(t, err)
	require.NotNil(t, receipts)
	require.Empty(t, receipts)
}

/* FINISH ALPHA CHANGE 1477 -- verify lossless empty body and receipt omission -- */

/* START ALPHA CHANGE 1590 -- remove superseded typed-delete storage test -- */
/* FINISH ALPHA CHANGE 1590 -- remove superseded typed-delete storage test -- */

func testWriteCanonicalHeader(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	h := &types.Header{
		Number:    100,
		ExtraData: []byte{0x1},
	}
	h.ComputeHash()

	/* START ALPHA CHANGE 804 -- write canonical header test without total difficulty -- */
	batch := NewBatchWriter(s)

	batch.PutCanonicalHeader(h)
	/* FINISH ALPHA CHANGE 804 -- write canonical header test without total difficulty -- */

	require.NoError(t, batch.WriteBatch())

	hh, err := s.ReadHeader(h.Hash)
	assert.NoError(t, err)

	if !reflect.DeepEqual(h, hh) {
		t.Fatal("bad header")
	}

	headHash, ok := s.ReadHeadHash()
	if !ok {
		t.Fatal("not found head hash")
	}

	if headHash != h.Hash {
		t.Fatal("head hash not correct")
	}

	headNum, ok := s.ReadHeadNumber()
	if !ok {
		t.Fatal("not found head num")
	}

	if headNum != h.Number {
		t.Fatal("head num not correct")
	}

	canHash, ok := s.ReadCanonicalHash(h.Number)
	if !ok {
		t.Fatal("not found can hash")
	}

	if canHash != h.Hash {
		t.Fatal("canonical hash not correct")
	}
}

// Storage delegators

type readCanonicalHashDelegate func(uint64) (types.Hash, bool)
type readHeadHashDelegate func() (types.Hash, bool)
type readHeadNumberDelegate func() (uint64, bool)

/* START ALPHA CHANGE 805 -- remove fork and total-difficulty mock delegates -- */
type readHeaderDelegate func(types.Hash) (*types.Header, error)

/* FINISH ALPHA CHANGE 805 -- remove fork and total-difficulty mock delegates -- */
type readBodyDelegate func(types.Hash) (*types.Body, error)

/* START ALPHA CHANGE 828 -- remove unused snapshot mock delegate -- */
type readReceiptsDelegate func(types.Hash) ([]*types.Receipt, error)

/* FINISH ALPHA CHANGE 828 -- remove unused snapshot mock delegate -- */
type readTxLookupDelegate func(types.Hash) (types.Hash, bool)
type closeDelegate func() error
type newBatchDelegate func() Batch

type MockStorage struct {
	/* START ALPHA CHANGE 3170 -- explicitly configure mocked startup inspection -- */
	readStartupStateFn func() (StartupState, error)
	/* FINISH ALPHA CHANGE 3170 -- explicitly configure mocked startup inspection -- */

	readCanonicalHashFn readCanonicalHashDelegate
	readHeadHashFn      readHeadHashDelegate
	readHeadNumberFn    readHeadNumberDelegate
	/* START ALPHA CHANGE 806 -- remove fork and total-difficulty mock hooks -- */
	readHeaderFn readHeaderDelegate
	/* FINISH ALPHA CHANGE 806 -- remove fork and total-difficulty mock hooks -- */
	readBodyFn     readBodyDelegate
	readReceiptsFn readReceiptsDelegate
	readTxLookupFn readTxLookupDelegate
	closeFn        closeDelegate
	newBatchFn     newBatchDelegate
}

func NewMockStorage() *MockStorage {
	return &MockStorage{}
}

/* START ALPHA CHANGE 3170 -- require an explicit startup result from storage mocks -- */
func (m *MockStorage) ReadStartupState() (StartupState, error) {
	if m.readStartupStateFn == nil {
		return StartupState{}, fmt.Errorf("startup-state mock hook not configured")
	}
	return m.readStartupStateFn()
}

func (m *MockStorage) HookReadStartupState(fn func() (StartupState, error)) {
	m.readStartupStateFn = fn
}

/* FINISH ALPHA CHANGE 3170 -- require an explicit startup result from storage mocks -- */

func (m *MockStorage) ReadCanonicalHash(n uint64) (types.Hash, bool) {
	if m.readCanonicalHashFn != nil {
		return m.readCanonicalHashFn(n)
	}

	return types.Hash{}, true
}

func (m *MockStorage) HookReadCanonicalHash(fn readCanonicalHashDelegate) {
	m.readCanonicalHashFn = fn
}

func (m *MockStorage) ReadHeadHash() (types.Hash, bool) {
	if m.readHeadHashFn != nil {
		return m.readHeadHashFn()
	}

	return types.Hash{}, true
}

func (m *MockStorage) HookReadHeadHash(fn readHeadHashDelegate) {
	m.readHeadHashFn = fn
}

func (m *MockStorage) ReadHeadNumber() (uint64, bool) {
	if m.readHeadNumberFn != nil {
		return m.readHeadNumberFn()
	}

	return 0, true
}

func (m *MockStorage) HookReadHeadNumber(fn readHeadNumberDelegate) {
	m.readHeadNumberFn = fn
}

/* START ALPHA CHANGE 807 -- prune fork and total-difficulty mock storage methods -- */
/* FINISH ALPHA CHANGE 807 -- prune fork and total-difficulty mock storage methods -- */

func (m *MockStorage) ReadHeader(hash types.Hash) (*types.Header, error) {
	if m.readHeaderFn != nil {
		return m.readHeaderFn(hash)
	}

	return &types.Header{}, nil
}

func (m *MockStorage) HookReadHeader(fn readHeaderDelegate) {
	m.readHeaderFn = fn
}

func (m *MockStorage) ReadBody(hash types.Hash) (*types.Body, error) {
	if m.readBodyFn != nil {
		return m.readBodyFn(hash)
	}

	return &types.Body{}, nil
}

func (m *MockStorage) HookReadBody(fn readBodyDelegate) {
	m.readBodyFn = fn
}

func (m *MockStorage) ReadReceipts(hash types.Hash) ([]*types.Receipt, error) {
	if m.readReceiptsFn != nil {
		return m.readReceiptsFn(hash)
	}

	return []*types.Receipt{}, nil
}

func (m *MockStorage) HookReadReceipts(fn readReceiptsDelegate) {
	m.readReceiptsFn = fn
}

func (m *MockStorage) ReadTxLookup(hash types.Hash) (types.Hash, bool) {
	if m.readTxLookupFn != nil {
		return m.readTxLookupFn(hash)
	}

	return types.Hash{}, true
}

func (m *MockStorage) HookReadTxLookup(fn readTxLookupDelegate) {
	m.readTxLookupFn = fn
}

func (m *MockStorage) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}

	return nil
}

func (m *MockStorage) HookClose(fn closeDelegate) {
	m.closeFn = fn
}

func (m *MockStorage) HookNewBatch(fn newBatchDelegate) {
	m.newBatchFn = fn
}

func (m *MockStorage) NewBatch() Batch {
	if m.newBatchFn != nil {
		return m.newBatchFn()
	}

	return nil
}
