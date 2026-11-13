package blockchain

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/helper/hex"
	"github.com/innovativebioresearch/innbc-chain/state"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage/memory"
	"github.com/innovativebioresearch/innbc-chain/types"
)

func TestGenesis(t *testing.T) {
	b := NewTestBlockchain(t, nil)

	// add genesis block
	genesis := &types.Header{Difficulty: 1, Number: 0}
	genesis.ComputeHash()

	assert.NoError(t, b.writeGenesisImpl(genesis))

	header := b.Header()
	assert.Equal(t, header.Hash, genesis.Hash)
}

type dummyChain struct {
	headers map[byte]*types.Header
}

func (c *dummyChain) add(h *header) error {
	if _, ok := c.headers[h.hash]; ok {
		return fmt.Errorf("hash already imported")
	}

	var parent types.Hash
	if h.number != 0 {
		p, ok := c.headers[h.parent]
		if !ok {
			return fmt.Errorf("parent not found %v", h.parent)
		}

		parent = p.Hash
	}

	hh := &types.Header{
		ParentHash: parent,
		Number:     h.number,
		Difficulty: h.diff,
		ExtraData:  []byte{h.hash},
	}

	hh.ComputeHash()
	c.headers[h.hash] = hh

	return nil
}

type header struct {
	hash   byte
	parent byte
	number uint64
	diff   uint64
}

func (h *header) Parent(parent byte) *header {
	h.parent = parent
	h.number = uint64(parent) + 1

	return h
}

func (h *header) Diff(d uint64) *header {
	h.diff = d

	return h
}

func (h *header) Number(d uint64) *header {
	h.number = d

	return h
}

func mock(number byte) *header {
	return &header{
		hash:   number,
		parent: number - 1,
		number: uint64(number),
		diff:   uint64(number),
	}
}

/* START ALPHA CHANGE 808 -- replace TD fork-choice tests with canonical append tests -- */
func TestInsertHeaders(t *testing.T) {
	type evnt struct {
		NewChain []*header
	}

	type headerEvnt struct {
		header *header
		event  *evnt
	}

	var cases = []struct {
		Name    string
		History []*headerEvnt
		Head    *header
		Chain   []*header
	}{
		{
			Name: "Genesis",
			History: []*headerEvnt{
				{
					header: mock(0x0),
				},
			},
			Head: mock(0x0),
			Chain: []*header{
				mock(0x0),
			},
		},
		{
			Name: "Linear",
			History: []*headerEvnt{
				{
					header: mock(0x0),
				},
				{
					header: mock(0x1),
					event: &evnt{
						NewChain: []*header{
							mock(0x1),
						},
					},
				},
				{
					header: mock(0x2),
					event: &evnt{
						NewChain: []*header{
							mock(0x2),
						},
					},
				},
			},
			Head: mock(0x2),
			Chain: []*header{
				mock(0x0),
				mock(0x1),
				mock(0x2),
			},
		},
	}

	for _, cc := range cases {
		t.Run(cc.Name, func(t *testing.T) {
			b := NewTestBlockchain(t, nil)

			chain := dummyChain{
				headers: map[byte]*types.Header{},
			}
			for _, i := range cc.History {
				if err := chain.add(i.header); err != nil {
					t.Fatal(err)
				}
			}

			checkEvents := func(a []*header, b []*types.Header) {
				if len(a) != len(b) {
					t.Fatal("bad size")
				}
				for indx := range a {
					if chain.headers[a[indx].hash].Hash != b[indx].Hash {
						t.Fatal("bad")
					}
				}
			}

			if err := b.writeGenesisImpl(chain.headers[0x0]); err != nil {
				t.Fatal(err)
			}

			sub := b.SubscribeEvents()

			for i := 1; i < len(cc.History); i++ {
				headers := []*types.Header{chain.headers[cc.History[i].header.hash]}
				if err := b.WriteHeadersWithBodies(headers); err != nil {
					t.Fatal(err)
				}

				event := sub.GetEvent()
				checkEvents(cc.History[i].event.NewChain, event.NewChain)
			}

			head := b.Header()

			expected, ok := chain.headers[cc.Head.hash]
			assert.True(t, ok)
			assert.Equal(t, head.Hash, expected.Hash)

			if cc.Chain != nil {
				for indx, i := range cc.Chain {
					block, _ := b.GetBlockByNumber(uint64(indx), true)
					if block.Hash().String() != chain.headers[i.hash].Hash.String() {
						t.Fatal("bad")
					}
				}
			}
		})
	}
}

func TestRejectNonCanonicalHeader(t *testing.T) {
	b := NewTestBlockchain(t, nil)

	chain := dummyChain{
		headers: map[byte]*types.Header{},
	}

	for _, h := range []*header{
		mock(0x0),
		mock(0x1),
		mock(0x2).Parent(0x0).Number(1),
	} {
		if err := chain.add(h); err != nil {
			t.Fatal(err)
		}
	}

	if err := b.writeGenesisImpl(chain.headers[0x0]); err != nil {
		t.Fatal(err)
	}

	require.NoError(t, b.WriteHeadersWithBodies([]*types.Header{chain.headers[0x1]}))
	require.ErrorContains(t, b.WriteHeadersWithBodies([]*types.Header{chain.headers[0x2]}), "non-canonical header")

	head := b.Header()
	assert.Equal(t, chain.headers[0x1].Hash, head.Hash)
}

/* FINISH ALPHA CHANGE 808 -- replace TD fork-choice tests with canonical append tests -- */

func TestForkUnknownParents(t *testing.T) {
	b := NewTestBlockchain(t, nil)

	h0 := NewTestHeaders(10)
	h1 := AppendNewTestHeaders(h0[:5], 10)

	/* START ALPHA CHANGE 809 -- initialize fork-parent test without total difficulty -- */
	// Write genesis
	batchWriter := storage.NewBatchWriter(b.db)

	batchWriter.PutCanonicalHeader(h0[0])

	assert.NoError(t, b.writeBatchAndUpdate(batchWriter, h0[0], true))
	/* FINISH ALPHA CHANGE 809 -- initialize fork-parent test without total difficulty -- */

	// Write 10 headers
	assert.NoError(t, b.WriteHeadersWithBodies(h0[1:]))

	// Cannot write this header because the father h1[11] is not known
	assert.Error(t, b.WriteHeadersWithBodies([]*types.Header{h1[12]}))
}

func TestBlockchainWriteBody(t *testing.T) {
	t.Parallel()

	var (
		addr = types.StringToAddress("1")
	)

	newChain := func(
		t *testing.T,
		txFromByTxHash map[types.Hash]types.Address,
	) *Blockchain {
		/* START ALPHA CHANGE 164 -- remove unused test chain path parameter after in-memory storage migration -- */
		t.Helper()
		/* FINISH ALPHA CHANGE 164 -- remove unused test chain path parameter after in-memory storage migration -- */

		dbStorage, err := memory.NewMemoryStorage(nil)
		assert.NoError(t, err)

		chain := &Blockchain{
			db: dbStorage,
			txSigner: &mockSigner{
				txFromByTxHash: txFromByTxHash,
			},
		}

		return chain
	}

	t.Run("should succeed if tx has from field", func(t *testing.T) {
		t.Parallel()

		tx := &types.Transaction{
			Value: big.NewInt(10),
			V:     big.NewInt(1),
			From:  addr,
		}

		block := &types.Block{
			Header: &types.Header{},
			Transactions: []*types.Transaction{
				tx,
			},
		}

		tx.ComputeHash(1)
		block.Header.ComputeHash()

		txFromByTxHash := map[types.Hash]types.Address{}

		/* START ALPHA CHANGE 165 -- drop obsolete test chain path argument -- */
		chain := newChain(t, txFromByTxHash)
		/* FINISH ALPHA CHANGE 165 -- drop obsolete test chain path argument -- */
		defer chain.db.Close()
		batchWriter := storage.NewBatchWriter(chain.db)

		assert.NoError(
			t,
			chain.writeBody(batchWriter, block),
		)
		assert.NoError(t, batchWriter.WriteBatch())
	})

	t.Run("should return error if tx doesn't have from and recovering address fails", func(t *testing.T) {
		t.Parallel()

		tx := &types.Transaction{
			Value: big.NewInt(10),
			V:     big.NewInt(1),
		}

		block := &types.Block{
			Header: &types.Header{},
			Transactions: []*types.Transaction{
				tx,
			},
		}

		tx.ComputeHash(1)
		block.Header.ComputeHash()

		txFromByTxHash := map[types.Hash]types.Address{}

		/* START ALPHA CHANGE 166 -- drop obsolete test chain path argument -- */
		chain := newChain(t, txFromByTxHash)
		/* FINISH ALPHA CHANGE 166 -- drop obsolete test chain path argument -- */
		defer chain.db.Close()
		batchWriter := storage.NewBatchWriter(chain.db)

		assert.ErrorIs(
			t,
			errRecoveryAddressFailed,
			chain.writeBody(batchWriter, block),
		)
		assert.NoError(t, batchWriter.WriteBatch())
	})

	t.Run("should recover from address and store to storage", func(t *testing.T) {
		t.Parallel()

		tx := &types.Transaction{
			Value: big.NewInt(10),
			V:     big.NewInt(1),
		}

		block := &types.Block{
			Header: &types.Header{},
			Transactions: []*types.Transaction{
				tx,
			},
		}

		tx.ComputeHash(1)
		block.Header.ComputeHash()

		txFromByTxHash := map[types.Hash]types.Address{
			tx.Hash: addr,
		}

		/* START ALPHA CHANGE 167 -- drop obsolete test chain path argument -- */
		chain := newChain(t, txFromByTxHash)
		/* FINISH ALPHA CHANGE 167 -- drop obsolete test chain path argument -- */
		defer chain.db.Close()
		batchWriter := storage.NewBatchWriter(chain.db)

		batchWriter.PutHeader(block.Header)

		assert.NoError(t, chain.writeBody(batchWriter, block))

		assert.NoError(t, batchWriter.WriteBatch())

		readBody, ok := chain.readBody(block.Hash())
		assert.True(t, ok)

		assert.Equal(t, addr, readBody.Transactions[0].From)
	})
}

func Test_recoverFromFieldsInBlock(t *testing.T) {
	t.Parallel()

	var (
		addr1 = types.StringToAddress("1")
		addr2 = types.StringToAddress("1")
		addr3 = types.StringToAddress("1")
	)

	computeTxHashes := func(txs ...*types.Transaction) {
		for _, tx := range txs {
			tx.ComputeHash(1)
		}
	}

	t.Run("should succeed", func(t *testing.T) {
		t.Parallel()

		txFromByTxHash := map[types.Hash]types.Address{}
		chain := &Blockchain{
			txSigner: &mockSigner{
				txFromByTxHash: txFromByTxHash,
			},
		}

		tx1 := &types.Transaction{Nonce: 0, From: addr1}
		tx2 := &types.Transaction{Nonce: 1, From: types.ZeroAddress}

		computeTxHashes(tx1, tx2)

		txFromByTxHash[tx2.Hash] = addr2

		block := &types.Block{
			Transactions: []*types.Transaction{
				tx1,
				tx2,
			},
		}

		assert.NoError(
			t,
			chain.recoverFromFieldsInBlock(block),
		)
	})

	t.Run("should stop and return error if recovery fails", func(t *testing.T) {
		t.Parallel()

		txFromByTxHash := map[types.Hash]types.Address{}
		chain := &Blockchain{
			txSigner: &mockSigner{
				txFromByTxHash: txFromByTxHash,
			},
		}

		tx1 := &types.Transaction{Nonce: 0, From: types.ZeroAddress}
		tx2 := &types.Transaction{Nonce: 1, From: types.ZeroAddress}
		tx3 := &types.Transaction{Nonce: 2, From: types.ZeroAddress}

		computeTxHashes(tx1, tx2, tx3)

		// returns only addresses for tx1 and tx3
		txFromByTxHash[tx1.Hash] = addr1
		txFromByTxHash[tx3.Hash] = addr3

		block := &types.Block{
			Transactions: []*types.Transaction{
				tx1,
				tx2,
				tx3,
			},
		}

		assert.ErrorIs(
			t,
			chain.recoverFromFieldsInBlock(block),
			errRecoveryAddressFailed,
		)

		assert.Equal(t, addr1, tx1.From)
		assert.Equal(t, types.ZeroAddress, tx2.From)
		assert.Equal(t, types.ZeroAddress, tx3.From)
	})
}

func Test_recoverFromFieldsInTransactions(t *testing.T) {
	t.Parallel()

	var (
		addr1 = types.StringToAddress("1")
		addr2 = types.StringToAddress("1")
		addr3 = types.StringToAddress("1")
	)

	computeTxHashes := func(txs ...*types.Transaction) {
		for _, tx := range txs {
			tx.ComputeHash(1)
		}
	}

	t.Run("should succeed", func(t *testing.T) {
		t.Parallel()

		txFromByTxHash := map[types.Hash]types.Address{}
		chain := &Blockchain{
			logger: hclog.NewNullLogger(),
			txSigner: &mockSigner{
				txFromByTxHash: txFromByTxHash,
			},
		}

		tx1 := &types.Transaction{Nonce: 0, From: addr1}
		tx2 := &types.Transaction{Nonce: 1, From: types.ZeroAddress}

		computeTxHashes(tx1, tx2)

		txFromByTxHash[tx2.Hash] = addr2

		transactions := []*types.Transaction{
			tx1,
			tx2,
		}

		assert.True(
			t,
			chain.recoverFromFieldsInTransactions(transactions),
		)
	})

	t.Run("should succeed even though recovery fails for some transactions", func(t *testing.T) {
		t.Parallel()

		txFromByTxHash := map[types.Hash]types.Address{}
		chain := &Blockchain{
			logger: hclog.NewNullLogger(),
			txSigner: &mockSigner{
				txFromByTxHash: txFromByTxHash,
			},
		}

		tx1 := &types.Transaction{Nonce: 0, From: types.ZeroAddress}
		tx2 := &types.Transaction{Nonce: 1, From: types.ZeroAddress}
		tx3 := &types.Transaction{Nonce: 2, From: types.ZeroAddress}

		computeTxHashes(tx1, tx2, tx3)

		// returns only addresses for tx1 and tx3
		txFromByTxHash[tx1.Hash] = addr1
		txFromByTxHash[tx3.Hash] = addr3

		transactions := []*types.Transaction{
			tx1,
			tx2,
			tx3,
		}

		assert.True(t, chain.recoverFromFieldsInTransactions(transactions))

		assert.Equal(t, addr1, tx1.From)
		assert.Equal(t, types.ZeroAddress, tx2.From)
		assert.Equal(t, addr3, tx3.From)
	})

	t.Run("should return false if all transactions has from field", func(t *testing.T) {
		t.Parallel()

		txFromByTxHash := map[types.Hash]types.Address{}
		chain := &Blockchain{
			logger: hclog.NewNullLogger(),
			txSigner: &mockSigner{
				txFromByTxHash: txFromByTxHash,
			},
		}

		tx1 := &types.Transaction{Nonce: 0, From: addr1}
		tx2 := &types.Transaction{Nonce: 1, From: addr2}

		computeTxHashes(tx1, tx2)

		txFromByTxHash[tx2.Hash] = addr2

		transactions := []*types.Transaction{
			tx1,
			tx2,
		}

		assert.False(
			t,
			chain.recoverFromFieldsInTransactions(transactions),
		)
	})
}

func TestBlockchainReadBody(t *testing.T) {
	dbStorage, err := memory.NewMemoryStorage(nil)
	assert.NoError(t, err)

	txFromByTxHash := make(map[types.Hash]types.Address)
	addr := types.StringToAddress("1")

	b := &Blockchain{
		logger: hclog.NewNullLogger(),
		db:     dbStorage,
		txSigner: &mockSigner{
			txFromByTxHash: txFromByTxHash,
		},
	}

	batchWriter := storage.NewBatchWriter(b.db)

	tx := &types.Transaction{
		Value: big.NewInt(10),
		V:     big.NewInt(1),
	}

	tx.ComputeHash(1)

	block := &types.Block{
		Header: &types.Header{},
		Transactions: []*types.Transaction{
			tx,
		},
	}

	block.Header.ComputeHash()

	txFromByTxHash[tx.Hash] = types.ZeroAddress

	/* START ALPHA CHANGE 810 -- write canonical body test header without total difficulty -- */
	batchWriter.PutCanonicalHeader(block.Header)
	/* FINISH ALPHA CHANGE 810 -- write canonical body test header without total difficulty -- */

	require.NoError(t, b.writeBody(batchWriter, block))

	assert.NoError(t, batchWriter.WriteBatch())

	txFromByTxHash[tx.Hash] = addr

	readBody, found := b.readBody(block.Hash())

	assert.True(t, found)
	assert.Equal(t, addr, readBody.Transactions[0].From)
}

/* START ALPHA CHANGE 1591 -- remove superseded record-level blockchain pruning test -- */
/* FINISH ALPHA CHANGE 1591 -- remove superseded record-level blockchain pruning test -- */

func TestCalculateGasLimit(t *testing.T) {
	tests := []struct {
		name             string
		blockGasTarget   uint64
		parentGasLimit   uint64
		expectedGasLimit uint64
	}{
		{
			name:             "should increase next gas limit towards target",
			blockGasTarget:   25000000,
			parentGasLimit:   20000000,
			expectedGasLimit: 20000000/1024 + 20000000,
		},
		{
			name:             "should decrease next gas limit towards target",
			blockGasTarget:   25000000,
			parentGasLimit:   26000000,
			expectedGasLimit: 26000000 - 26000000/1024,
		},
		{
			name:             "should not alter gas limit when exactly the same",
			blockGasTarget:   25000000,
			parentGasLimit:   25000000,
			expectedGasLimit: 25000000,
		},
		{
			name:             "should increase to the exact gas target if adding the delta surpasses it",
			blockGasTarget:   25000000 + 25000000/1024 - 100, // - 100 so that it takes less than the delta to reach it
			parentGasLimit:   25000000,
			expectedGasLimit: 25000000 + 25000000/1024 - 100,
		},
		{
			name:             "should decrease to the exact gas target if subtracting the delta surpasses it",
			blockGasTarget:   25000000 - 25000000/1024 + 100, // + 100 so that it takes less than the delta to reach it
			parentGasLimit:   25000000,
			expectedGasLimit: 25000000 - 25000000/1024 + 100,
		},
		/* START ALPHA CHANGE 2060 -- test mandatory gas floor across every calculator path -- */
		{
			name:             "zero target clamps an invalid parent to the mandatory floor",
			blockGasTarget:   0,
			parentGasLimit:   types.StateTransactionGasLimit - 1,
			expectedGasLimit: types.StateTransactionGasLimit,
		},
		{
			name:             "zero target preserves a parent at the mandatory floor",
			blockGasTarget:   0,
			parentGasLimit:   types.StateTransactionGasLimit,
			expectedGasLimit: types.StateTransactionGasLimit,
		},
		{
			name:             "below-floor target cannot pull the result below the mandatory floor",
			blockGasTarget:   types.StateTransactionGasLimit - 1,
			parentGasLimit:   types.StateTransactionGasLimit + 500,
			expectedGasLimit: types.StateTransactionGasLimit,
		},
		{
			name:             "exact mandatory target remains at the mandatory floor",
			blockGasTarget:   types.StateTransactionGasLimit,
			parentGasLimit:   types.StateTransactionGasLimit,
			expectedGasLimit: types.StateTransactionGasLimit,
		},
		{
			name:             "generated INNBC target remains unchanged when reached",
			blockGasTarget:   5_242_880,
			parentGasLimit:   5_242_880,
			expectedGasLimit: 5_242_880,
		},
		{
			name:             "upward calculation near uint64 maximum reaches target without overflow",
			blockGasTarget:   math.MaxUint64,
			parentGasLimit:   math.MaxUint64 - 100,
			expectedGasLimit: math.MaxUint64,
		},
		/* FINISH ALPHA CHANGE 2060 -- test mandatory gas floor across every calculator path -- */
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storageCallback := func(storage *storage.MockStorage) {
				storage.HookReadHeader(func(hash types.Hash) (*types.Header, error) {
					return &types.Header{
						// This is going to be the parent block header
						GasLimit: tt.parentGasLimit,
					}, nil
				})
			}

			b, blockchainErr := NewMockBlockchain(map[TestCallbackType]interface{}{
				StorageCallback: storageCallback,
			})
			if blockchainErr != nil {
				t.Fatalf("unable to construct the blockchain, %v", blockchainErr)
			}

			b.config.Params = &chain.Params{
				BlockGasTarget: tt.blockGasTarget,
			}

			nextGas, err := b.CalculateGasLimit(1)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedGasLimit, nextGas)
		})
	}
}

// TestGasPriceAverage tests the average gas price of the
// blockchain
func TestGasPriceAverage(t *testing.T) {
	/* START ALPHA CHANGE 1949 -- cover cumulative gas-price average edge cases -- */
	testTable := []struct {
		name               string
		previousAverage    *big.Int
		previousCount      *big.Int
		newValues          []*big.Int
		expectedNewAverage *big.Int
	}{
		{
			"no previous average data",
			big.NewInt(0),
			big.NewInt(0),
			[]*big.Int{
				big.NewInt(1),
				big.NewInt(2),
				big.NewInt(3),
				big.NewInt(4),
				big.NewInt(5),
			},
			big.NewInt(3),
		},
		{
			"previous average data",
			// For example (5 + 5 + 5 + 5 + 5) / 5
			big.NewInt(5),
			big.NewInt(5),
			[]*big.Int{
				big.NewInt(1),
				big.NewInt(2),
				big.NewInt(3),
			},
			// (5 * 5 + 1 + 2 + 3) / 8
			big.NewInt(3),
		},
		{
			"cumulative formula differs from legacy rolling formula",
			big.NewInt(100),
			big.NewInt(10),
			[]*big.Int{
				big.NewInt(200),
			},
			// (100 * 10 + 200) / 11
			big.NewInt(109),
		},
		{
			"new batch larger than previous sample count",
			big.NewInt(100),
			big.NewInt(1),
			[]*big.Int{
				big.NewInt(1),
				big.NewInt(1),
			},
			// (100 * 1 + 1 + 1) / 3
			big.NewInt(34),
		},
		{
			"empty update preserves existing average",
			big.NewInt(123),
			big.NewInt(7),
			[]*big.Int{},
			big.NewInt(123),
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			previousCount := new(big.Int).Set(testCase.previousCount)

			// Setup the mock data
			blockchain := NewTestBlockchain(t, nil)
			blockchain.gpAverage.price = new(big.Int).Set(testCase.previousAverage)
			blockchain.gpAverage.count = new(big.Int).Set(testCase.previousCount)

			// Update the average gas price
			blockchain.updateGasPriceAvg(testCase.newValues)

			// Make sure the average gas price count is correct
			assert.Equal(
				t,
				int64(len(testCase.newValues))+previousCount.Int64(),
				blockchain.gpAverage.count.Int64(),
			)

			// Make sure the average gas price is correct
			assert.Equal(t, testCase.expectedNewAverage.String(), blockchain.gpAverage.price.String())
		})
	}
	/* FINISH ALPHA CHANGE 1949 -- cover cumulative gas-price average edge cases -- */
}

// TestBlockchain_VerifyBlockParent verifies that parent block verification
// errors are handled correctly
func TestBlockchain_VerifyBlockParent(t *testing.T) {
	t.Parallel()

	emptyHeader := &types.Header{
		Hash:       types.ZeroHash,
		ParentHash: types.ZeroHash,
	}
	emptyHeader.ComputeHash()

	t.Run("Missing parent block", func(t *testing.T) {
		t.Parallel()

		// Set up the storage callback
		storageCallback := func(storage *storage.MockStorage) {
			storage.HookReadHeader(func(hash types.Hash) (*types.Header, error) {
				return nil, errors.New("not found")
			})
		}

		blockchain, err := NewMockBlockchain(map[TestCallbackType]interface{}{
			StorageCallback: storageCallback,
		})
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		// Create a dummy block
		block := &types.Block{
			Header: &types.Header{
				ParentHash: types.ZeroHash,
			},
		}

		assert.ErrorIs(t, blockchain.verifyBlockParent(block), ErrParentNotFound)
	})

	t.Run("Parent hash mismatch", func(t *testing.T) {
		t.Parallel()

		// Set up the storage callback
		storageCallback := func(storage *storage.MockStorage) {
			storage.HookReadHeader(func(hash types.Hash) (*types.Header, error) {
				return emptyHeader.Copy(), nil
			})
		}

		blockchain, err := NewMockBlockchain(map[TestCallbackType]interface{}{
			StorageCallback: storageCallback,
		})
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		// Create a dummy block whose parent hash will
		// not match the computed parent hash
		block := &types.Block{
			Header: emptyHeader.Copy(),
		}

		assert.ErrorIs(t, blockchain.verifyBlockParent(block), ErrParentHashMismatch)
	})

	t.Run("Invalid block sequence", func(t *testing.T) {
		t.Parallel()

		// Set up the storage callback
		storageCallback := func(storage *storage.MockStorage) {
			storage.HookReadHeader(func(hash types.Hash) (*types.Header, error) {
				return emptyHeader.Copy(), nil
			})
		}

		blockchain, err := NewMockBlockchain(map[TestCallbackType]interface{}{
			StorageCallback: storageCallback,
		})
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		// Create a dummy block with a number much higher than the parent
		block := &types.Block{
			Header: &types.Header{
				Number: 10,
			},
		}

		assert.ErrorIs(t, blockchain.verifyBlockParent(block), ErrParentHashMismatch)
	})

	t.Run("Invalid block sequence", func(t *testing.T) {
		t.Parallel()

		// Set up the storage callback
		storageCallback := func(storage *storage.MockStorage) {
			storage.HookReadHeader(func(hash types.Hash) (*types.Header, error) {
				return emptyHeader.Copy(), nil
			})
		}

		blockchain, err := NewMockBlockchain(map[TestCallbackType]interface{}{
			StorageCallback: storageCallback,
		})
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		// Create a dummy block with a number much higher than the parent
		block := &types.Block{
			Header: &types.Header{
				Number:     10,
				ParentHash: emptyHeader.Copy().Hash,
			},
		}

		assert.ErrorIs(t, blockchain.verifyBlockParent(block), ErrInvalidBlockSequence)
	})
	/* START ALPHA CHANGE 1256 -- cover safe gas-limit validation arithmetic -- */
	t.Run("Invalid block gas limit across full uint64 range", func(t *testing.T) {
		t.Parallel()

		parentHeader := emptyHeader.Copy()
		parentHeader.GasLimit = math.MaxUint64
		parentHeader.ComputeHash()

		blockchain, err := NewMockBlockchain(nil)
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		block := &types.Block{
			Header: &types.Header{
				Number:     1,
				ParentHash: parentHeader.Hash,
				GasLimit:   types.StateTransactionGasLimit,
			},
		}

		err = blockchain.VerifyGasLimit(block.Header, parentHeader)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid gas limit")
	})

	/* START ALPHA CHANGE 2061 -- test mandatory gas floor including the genesis early-return boundary -- */
	t.Run("Genesis header below mandatory gas floor is rejected", func(t *testing.T) {
		t.Parallel()

		blockchain, err := NewMockBlockchain(nil)
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		genesisHeader := &types.Header{
			Number:   0,
			GasLimit: types.StateTransactionGasLimit - 1,
		}

		err = blockchain.VerifyGasLimit(genesisHeader, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "minimum = 1000000")
	})

	t.Run("Mandatory state transaction gas limit boundary", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name          string
			gasLimit      uint64
			expectedError bool
		}{
			{
				name:          "one below floor is rejected",
				gasLimit:      types.StateTransactionGasLimit - 1,
				expectedError: true,
			},
			{
				name:     "exact floor is accepted",
				gasLimit: types.StateTransactionGasLimit,
			},
			{
				name:     "generated INNBC limit is accepted",
				gasLimit: 5_242_880,
			},
		}

		blockchain, err := NewMockBlockchain(nil)
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				parentHeader := emptyHeader.Copy()
				parentHeader.GasLimit = test.gasLimit
				parentHeader.ComputeHash()

				header := &types.Header{
					Number:     1,
					ParentHash: parentHeader.Hash,
					GasLimit:   test.gasLimit,
				}

				err := blockchain.VerifyGasLimit(header, parentHeader)
				if test.expectedError {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "minimum = 1000000")

					return
				}

				assert.NoError(t, err)
			})
		}
	})

	t.Run("Decrease below mandatory floor is rejected even within parent delta", func(t *testing.T) {
		t.Parallel()

		parentHeader := emptyHeader.Copy()
		parentHeader.GasLimit = types.StateTransactionGasLimit + 500
		parentHeader.ComputeHash()

		blockchain, err := NewMockBlockchain(nil)
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		header := &types.Header{
			Number:     1,
			ParentHash: parentHeader.Hash,
			GasLimit:   types.StateTransactionGasLimit - 1,
		}

		err = blockchain.VerifyGasLimit(header, parentHeader)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "minimum = 1000000")
	})
	/* FINISH ALPHA CHANGE 2061 -- test mandatory gas floor including the genesis early-return boundary -- */
	/* FINISH ALPHA CHANGE 1256 -- cover safe gas-limit validation arithmetic -- */
}

// TestBlockchain_VerifyBlockBody makes sure that the block body is verified correctly
func TestBlockchain_VerifyBlockBody(t *testing.T) {
	t.Parallel()

	emptyHeader := &types.Header{
		Hash:       types.ZeroHash,
		ParentHash: types.ZeroHash,
	}

	t.Run("Invalid SHA3 Uncles root", func(t *testing.T) {
		t.Parallel()

		blockchain, err := NewMockBlockchain(nil)
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		block := &types.Block{
			Header: &types.Header{
				Sha3Uncles: types.ZeroHash,
			},
		}

		_, err = blockchain.verifyBlockBody(block)
		assert.ErrorIs(t, err, ErrInvalidSha3Uncles)
	})

	t.Run("Invalid Transactions root", func(t *testing.T) {
		t.Parallel()

		blockchain, err := NewMockBlockchain(nil)
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		block := &types.Block{
			Header: &types.Header{
				Sha3Uncles: types.EmptyUncleHash,
			},
		}

		_, err = blockchain.verifyBlockBody(block)
		assert.ErrorIs(t, err, ErrInvalidTxRoot)
	})

	t.Run("Invalid execution result - missing parent", func(t *testing.T) {
		t.Parallel()

		// Set up the storage callback
		storageCallback := func(storage *storage.MockStorage) {
			storage.HookReadHeader(func(hash types.Hash) (*types.Header, error) {
				return nil, errors.New("not found")
			})
		}

		blockchain, err := NewMockBlockchain(map[TestCallbackType]interface{}{
			StorageCallback: storageCallback,
		})
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		block := &types.Block{
			Header: &types.Header{
				Sha3Uncles: types.EmptyUncleHash,
				TxRoot:     types.EmptyRootHash,
			},
		}

		_, err = blockchain.verifyBlockBody(block)
		assert.ErrorIs(t, err, ErrParentNotFound)
	})

	t.Run("Invalid execution result - unable to fetch block creator", func(t *testing.T) {
		t.Parallel()

		errBlockCreatorNotFound := errors.New("not found")

		// Set up the storage callback
		storageCallback := func(storage *storage.MockStorage) {
			// This is used for parent fetching
			storage.HookReadHeader(func(hash types.Hash) (*types.Header, error) {
				return emptyHeader.Copy(), nil
			})
		}

		// Set up the verifier callback
		verifierCallback := func(verifier *MockVerifier) {
			// This is used for error-ing out on the block creator fetch
			verifier.HookGetBlockCreator(func(t *types.Header) (types.Address, error) {
				return types.ZeroAddress, errBlockCreatorNotFound
			})
		}

		blockchain, err := NewMockBlockchain(map[TestCallbackType]interface{}{
			StorageCallback:  storageCallback,
			VerifierCallback: verifierCallback,
		})
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		block := &types.Block{
			Header: &types.Header{
				Sha3Uncles: types.EmptyUncleHash,
				TxRoot:     types.EmptyRootHash,
			},
		}

		_, err = blockchain.verifyBlockBody(block)
		assert.ErrorIs(t, err, errBlockCreatorNotFound)
	})

	t.Run("Invalid execution result - unable to execute transactions", func(t *testing.T) {
		t.Parallel()

		errUnableToExecute := errors.New("unable to execute transactions")

		// Set up the storage callback
		storageCallback := func(storage *storage.MockStorage) {
			// This is used for parent fetching
			storage.HookReadHeader(func(hash types.Hash) (*types.Header, error) {
				return emptyHeader.Copy(), nil
			})
		}

		executorCallback := func(executor *mockExecutor) {
			// This is executor processing
			executor.HookProcessBlock(func(
				hash types.Hash,
				block *types.Block,
				address types.Address,
			) (*state.Transition, error) {
				return nil, errUnableToExecute
			})
		}

		blockchain, err := NewMockBlockchain(map[TestCallbackType]interface{}{
			StorageCallback:  storageCallback,
			ExecutorCallback: executorCallback,
		})
		if err != nil {
			t.Fatalf("unable to instantiate new blockchain, %v", err)
		}

		block := &types.Block{
			Header: &types.Header{
				Sha3Uncles: types.EmptyUncleHash,
				TxRoot:     types.EmptyRootHash,
			},
		}

		_, err = blockchain.verifyBlockBody(block)
		assert.ErrorIs(t, err, errUnableToExecute)
	})
}

func TestBlockchain_CalculateBaseFee(t *testing.T) {
	t.Parallel()

	tests := []struct {
		blockNumber    uint64
		parentBaseFee  uint64
		parentGasLimit uint64
		parentGasUsed  uint64
		/* START ALPHA CHANGE 1151 -- test optional governance max base fee cap -- */
		maxBaseFeeWei uint64
		/* FINISH ALPHA CHANGE 1151 -- test optional governance max base fee cap -- */
		expectedBaseFee      uint64
		elasticityMultiplier uint64
	}{
		// START ALPHA CHANGE 1008 -- update base fee expectations for one wei floor --
		{6, chain.GenesisBaseFee, 20000000, 10000000, 0, chain.GenesisBaseFee, 2}, // usage == target
		{6, chain.GenesisBaseFee, 20000000, 10000000, 0, 2, 4},                    // usage above target increases by at least one wei
		{6, chain.GenesisBaseFee, 20000000, 9000000, 0, 1, 2},                     // usage below target floors at one wei
		{6, chain.GenesisBaseFee, 20000000, 9000000, 0, 2, 4},                     // usage above target increases by at least one wei
		{6, chain.GenesisBaseFee, 20000000, 11000000, 0, 2, 2},                    // usage above target increases by at least one wei
		{6, chain.GenesisBaseFee, 20000000, 11000000, 0, 2, 4},                    // usage above target increases by at least one wei
		{6, chain.GenesisBaseFee, 20000000, 20000000, 0, 2, 2},                    // usage full increases by at least one wei
		{6, chain.GenesisBaseFee, 20000000, 20000000, 0, 2, 4},                    // usage full increases by at least one wei
		{6, chain.GenesisBaseFee, 20000000, 0, 0, 1, 2},                           // usage 0 floors at one wei
		{6, chain.GenesisBaseFee, 20000000, 0, 0, 1, 4},                           // usage 0 floors at one wei
		// FINISH ALPHA CHANGE 1008 -- update base fee expectations for one wei floor --
		// START ALPHA CHANGE 1071 -- test overflow-safe base fee calculation --
		{6, 7761102613133, 5242880, 5242880, 0, 8731240439774, 2}, // full block delta exceeds uint64 if multiplied before division
		{6, math.MaxUint64 - 1, 20000000, 20000000, 0, math.MaxUint64, 2},
		{6, 100, 1, 1, 0, 100, 2}, // malformed zero gas target does not panic
		// FINISH ALPHA CHANGE 1071 -- test overflow-safe base fee calculation --
		// START ALPHA CHANGE 1152 -- test governance max base fee cap behavior --
		{6, 100, 20000000, 20000000, 2, 2, 2},
		{6, 100, 20000000, 20000000, 1, 1, 2},
		{6, 100, 20000000, 10000000, 50, 50, 2},
		// FINISH ALPHA CHANGE 1152 -- test governance max base fee cap behavior --
	}

	for i, test := range tests {
		i := i
		test := test

		t.Run(fmt.Sprintf("test case #%d", i+1), func(t *testing.T) {
			t.Parallel()

			blockchain := &Blockchain{
				executor: &mockExecutor{
					/* START ALPHA CHANGE 1153 -- inject governance max base fee cap into base fee tests -- */
					readMaxBaseFeeWeiFn: func(root types.Hash) (uint64, error) {
						return test.maxBaseFeeWei, nil
					},
					/* FINISH ALPHA CHANGE 1153 -- inject governance max base fee cap into base fee tests -- */
				},
				config: &chain.Chain{
					Params: &chain.Params{
						Forks: &chain.Forks{
							chain.London: chain.NewFork(5),
						},
					},
					Genesis: &chain.Genesis{
						BaseFeeEM:          test.elasticityMultiplier,
						BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
					},
				},
			}

			parent := &types.Header{
				Number:   test.blockNumber,
				GasLimit: test.parentGasLimit,
				GasUsed:  test.parentGasUsed,
				BaseFee:  test.parentBaseFee,
			}

			got, err := blockchain.CalculateBaseFee(parent)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedBaseFee, got, fmt.Sprintf("expected %d, got %d", test.expectedBaseFee, got))
		})
	}
}

func TestBlockchain_WriteFullBlock(t *testing.T) {
	t.Parallel()

	getKey := func(p []byte, k []byte) []byte {
		return append(append(make([]byte, 0, len(p)+len(k)), p...), k...)
	}
	db := map[string][]byte{}
	consensusMock := &MockVerifier{
		processHeadersFn: func(hs []*types.Header) error {
			assert.Len(t, hs, 1)

			return nil
		},
	}

	storageMock := storage.NewMockStorage()
	storageMock.HookNewBatch(func() storage.Batch {
		return memory.NewBatchMemory(db)
	})

	bc := &Blockchain{
		gpAverage: &gasPriceAverage{
			count: new(big.Int),
		},
		logger:    hclog.NewNullLogger(),
		db:        storageMock,
		consensus: consensusMock,
		config: &chain.Chain{
			Params: &chain.Params{
				Forks: &chain.Forks{
					chain.London: chain.NewFork(5),
				},
			},
			Genesis: &chain.Genesis{
				BaseFeeEM: 4,
			},
		},
		stream: newEventStream(),
	}

	/* START ALPHA CHANGE 820 -- remove test total-difficulty cache setup -- */
	bc.headersCache, _ = lru.New(10)

	existingHeader := &types.Header{Number: 1}
	/* FINISH ALPHA CHANGE 820 -- remove test total-difficulty cache setup -- */
	header := &types.Header{
		Number: 2,
	}
	receipts := []*types.Receipt{
		{GasUsed: 100},
		{GasUsed: 200},
	}
	tx := &types.Transaction{
		Value: big.NewInt(1),
	}

	tx.ComputeHash(1)
	header.ComputeHash()
	existingHeader.ComputeHash()
	/* START ALPHA CHANGE 821 -- initialize test canonical head without total difficulty -- */
	bc.currentHeader.Store(existingHeader)

	header.ParentHash = existingHeader.Hash
	/* FINISH ALPHA CHANGE 821 -- initialize test canonical head without total difficulty -- */
	bc.txSigner = &mockSigner{
		txFromByTxHash: map[types.Hash]types.Address{
			tx.Hash: {1, 2},
		},
	}

	// already existing block write
	err := bc.WriteFullBlock(&types.FullBlock{
		Block: &types.Block{
			Header:       existingHeader,
			Transactions: []*types.Transaction{tx},
		},
		Receipts: receipts,
	}, "innbcbft")

	require.NoError(t, err)
	require.Equal(t, 0, len(db))
	require.Equal(t, uint64(1), bc.currentHeader.Load().Number)

	/* START ALPHA CHANGE 2594 -- exercise fresh writes and exact-head confirmation with the storage fixture -- */
	fullBlock := &types.FullBlock{
		Block: &types.Block{
			Header:       header,
			Transactions: []*types.Transaction{tx},
		},
		Receipts: receipts,
	}

	err = bc.WriteFullBlockConfirmed(fullBlock, "innbcbft")
	require.NoError(t, err)

	err = bc.WriteFullBlockConfirmed(fullBlock, "innbcbft")
	require.NoError(t, err)
	/* FINISH ALPHA CHANGE 2594 -- exercise fresh writes and exact-head confirmation with the storage fixture -- */
	/* START ALPHA CHANGE 822 -- expect no total-difficulty DB entry in full block write test -- */
	require.Equal(t, 7, len(db))
	/* FINISH ALPHA CHANGE 822 -- expect no total-difficulty DB entry in full block write test -- */
	require.Equal(t, uint64(2), bc.currentHeader.Load().Number)
	require.NotNil(t, db[hex.EncodeToHex(getKey(storage.BODY, header.Hash.Bytes()))])
	require.NotNil(t, db[hex.EncodeToHex(getKey(storage.TX_LOOKUP_PREFIX, tx.Hash.Bytes()))])
	require.NotNil(t, db[hex.EncodeToHex(getKey(storage.HEADER, header.Hash.Bytes()))])
	require.NotNil(t, db[hex.EncodeToHex(getKey(storage.HEAD, storage.HASH))])
	require.NotNil(t, db[hex.EncodeToHex(getKey(storage.CANONICAL, common.EncodeUint64ToBytes(header.Number)))])
	/* START ALPHA CHANGE 823 -- remove total-difficulty assertion from full block write test -- */
	require.NotNil(t, db[hex.EncodeToHex(getKey(storage.HEAD, storage.NUMBER))])
	require.NotNil(t, db[hex.EncodeToHex(getKey(storage.RECEIPTS, header.Hash.Bytes()))])
	/* FINISH ALPHA CHANGE 823 -- remove total-difficulty assertion from full block write test -- */
}
