/* START ALPHA CHANGE 3297 -- test explicit historical log failures and physical MDBX segment retirement -- */
package jsonrpc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"

	/* START ALPHA CHANGE 3521 -- exercise actual stored receipt RLP through MDBX -- */
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	/* FINISH ALPHA CHANGE 3521 -- exercise actual stored receipt RLP through MDBX -- */
	segmentedstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/segmented"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type logHistoryTestStore struct {
	*mockStore

	blocks        map[uint64]*types.Block
	missingBodies map[uint64]bool
	floor         uint64
	floorReads    int
	receiptReads  int
}

func newLogHistoryTestStore(floor uint64) *logHistoryTestStore {
	base := newMockStore()
	base.header = &types.Header{
		Number: 5,
		Hash:   types.BytesToHash([]byte{5}),
	}
	base.receipts = make(map[types.Hash][]*types.Receipt)

	store := &logHistoryTestStore{
		mockStore:     base,
		blocks:        make(map[uint64]*types.Block),
		missingBodies: make(map[uint64]bool),
		floor:         floor,
	}

	// One matching log per available block. These fixtures exercise
	// log retrieval, without attempting transaction execution.
	for number := uint64(1); number <= 5; number++ {
		if number < floor {
			continue
		}

		hash := types.BytesToHash([]byte{byte(number)})
		store.blocks[number] = &types.Block{
			Header: &types.Header{
				Number: number,
				Hash:   hash,
			},
			Transactions: []*types.Transaction{
				{Hash: types.BytesToHash([]byte{byte(number), 0xaa})},
			},
		}
		base.receipts[hash] = []*types.Receipt{
			{
				Logs: []*types.Log{
					{
						Address: types.StringToAddress("0x1001"),
						Data:    []byte{byte(number)},
					},
				},
			},
		}
	}

	return store
}

func (s *logHistoryTestStore) HistoryFrom() uint64 {
	s.floorReads++
	return s.floor
}

func (s *logHistoryTestStore) GetBlockByNumber(
	number uint64,
	full bool,
) (*types.Block, bool) {
	block, ok := s.blocks[number]
	if full && s.missingBodies[number] {
		return nil, false
	}
	return block, ok
}

func (s *logHistoryTestStore) GetBlockByHash(
	hash types.Hash,
	full bool,
) (*types.Block, bool) {
	for number, block := range s.blocks {
		if block.Hash() == hash {
			return s.GetBlockByNumber(number, full)
		}
	}
	return nil, false
}

func (s *logHistoryTestStore) GetReceiptsByHash(
	hash types.Hash,
) ([]*types.Receipt, error) {
	s.receiptReads++
	return s.mockStore.GetReceiptsByHash(hash)
}

func TestLogHistory_RangesAndFailureClassification(t *testing.T) {
	tests := []struct {
		name             string
		floor            uint64
		from             BlockNumber
		to               BlockNumber
		missingBlock     uint64
		missingBody      uint64
		nilHead          bool
		wantNumbers      []uint64
		wantError        error
		wantFloorReads   int
		wantReceiptReads int
	}{
		{
			name: "healthy full history",
			from: 1, to: 3,
			wantNumbers:      []uint64{1, 2, 3},
			wantReceiptReads: 3,
		},
		{
			name:  "available retained suffix",
			floor: 3, from: 3, to: 5,
			wantNumbers:      []uint64{3, 4, 5},
			wantReceiptReads: 3,
		},
		{
			name:  "range entirely before history floor",
			floor: 3, from: 1, to: 2,
			wantError:      ErrPrunedHistory,
			wantFloorReads: 1,
		},
		{
			name:  "range crosses history floor",
			floor: 3, from: 2, to: 4,
			wantError:      ErrPrunedHistory,
			wantFloorReads: 1,
		},
		{
			name:  "missing first block inside available history",
			floor: 3, from: 3, to: 4,
			missingBlock:   3,
			wantError:      ErrBlockNotFound,
			wantFloorReads: 1,
		},
		{
			name: "gap discards already collected logs",
			from: 1, to: 3,
			missingBlock:     2,
			wantError:        ErrBlockNotFound,
			wantFloorReads:   1,
			wantReceiptReads: 1,
		},
		{
			name:  "missing retained head",
			floor: 3, from: 5, to: 5,
			missingBlock:   5,
			wantError:      ErrBlockNotFound,
			wantFloorReads: 1,
		},
		{
			name: "retained header with missing body",
			from: 1, to: 3,
			missingBody:      2,
			wantError:        ErrPrunedHistory,
			wantReceiptReads: 1,
		},
		{
			name: "range extends beyond head",
			from: 4, to: 7,
			wantNumbers:      []uint64{4, 5},
			wantReceiptReads: 2,
		},
		{
			name: "range entirely above head",
			from: 6, to: 7,
		},
		{
			name: "missing head metadata",
			from: 6, to: 6,
			nilHead:   true,
			wantError: ErrLatestNotFound,
		},
		{
			name: "genesis-only query remains empty",
			from: 0, to: 0,
		},
		{
			name: "range budget remains enforced",
			from: 1, to: 300,
			wantError: ErrBlockRangeTooHigh,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newLogHistoryTestStore(test.floor)
			if test.missingBlock != 0 {
				delete(store.blocks, test.missingBlock)
			}
			if test.missingBody != 0 {
				store.missingBodies[test.missingBody] = true
			}
			if test.nilHead {
				store.header = nil
			}

			manager := &FilterManager{
				store:           store,
				blockRangeLimit: 256,
			}
			logs, err := manager.GetLogsForQuery(&LogQuery{
				fromBlock: test.from,
				toBlock:   test.to,
			})

			if test.wantError != nil {
				require.ErrorIs(t, err, test.wantError)
				require.Nil(t, logs, "failed ranges must not return partial logs")
			} else {
				require.NoError(t, err)
				require.NotNil(t, logs)

				var numbers []uint64
				for _, log := range logs {
					numbers = append(numbers, uint64(log.BlockNumber))
				}
				require.Equal(t, test.wantNumbers, numbers)
			}

			require.Equal(t, test.wantFloorReads, store.floorReads)
			require.Equal(t, test.wantReceiptReads, store.receiptReads)
		})
	}
}

func logHistoryCallRPC(
	t *testing.T,
	dispatcher *Dispatcher,
	method string,
	params string,
) (json.RawMessage, *ObjectError) {
	t.Helper()

	request := fmt.Appendf(nil,
		`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`,
		method, params,
	)
	raw, err := dispatcher.Handle(request)
	require.NoError(t, err)

	var response struct {
		Result json.RawMessage `json:"result"`
		Error  *ObjectError    `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &response))
	return response.Result, response.Error
}

func TestLogHistory_RPCRejectsIncompleteResults(t *testing.T) {
	store := newLogHistoryTestStore(3)
	dispatcher := newTestDispatcher(
		t, hclog.NewNullLogger(), store,
		&dispatcherParams{
			chainID:                 100,
			blockRangeLimit:         256,
			jsonRPCBatchLengthLimit: 20,
		},
	)
	t.Cleanup(dispatcher.Close)

	filterID := dispatcher.filterManager.NewLogFilter(&LogQuery{
		fromBlock: 2,
		toBlock:   4,
	}, nil)
	require.NotEmpty(t, filterID)

	for _, request := range []struct {
		method string
		params string
	}{
		{
			method: "eth_getLogs",
			params: `[{"fromBlock":"0x2","toBlock":"0x4"}]`,
		},
		{
			method: "eth_getFilterLogs",
			params: fmt.Sprintf(`[%q]`, filterID),
		},
	} {
		result, rpcError := logHistoryCallRPC(
			t, dispatcher, request.method, request.params,
		)
		require.NotNil(t, rpcError, request.method)
		require.Equal(t, ErrPrunedHistory.Error(), rpcError.Message)
		require.Empty(t, result, "an error must not carry a successful log array")
	}

	// A subsequent fully available range must still return its matching logs.
	result, rpcError := logHistoryCallRPC(
		t, dispatcher, "eth_getLogs",
		`[{"fromBlock":"0x3","toBlock":"0x4"}]`,
	)
	require.Nil(t, rpcError)

	var logs []struct {
		BlockNumber string `json:"blockNumber"`
	}
	require.NoError(t, json.Unmarshal(result, &logs))
	require.Len(t, logs, 2)
	require.Equal(t, "0x3", logs[0].BlockNumber)
	require.Equal(t, "0x4", logs[1].BlockNumber)
}

type logHistoryDiskStore struct {
	*mockStore
	chain *blockchain.Blockchain
}

func (s *logHistoryDiskStore) Header() *types.Header {
	return s.chain.Header()
}

func (s *logHistoryDiskStore) HistoryFrom() uint64 {
	return s.chain.HistoryFrom()
}

func (s *logHistoryDiskStore) GetBlockByNumber(
	number uint64,
	full bool,
) (*types.Block, bool) {
	return s.chain.GetBlockByNumber(number, full)
}

func (s *logHistoryDiskStore) GetBlockByHash(
	hash types.Hash,
	full bool,
) (*types.Block, bool) {
	return s.chain.GetBlockByHash(hash, full)
}

func (s *logHistoryDiskStore) GetReceiptsByHash(
	hash types.Hash,
) ([]*types.Receipt, error) {
	return s.chain.GetReceiptsByHash(hash)
}

func TestLogHistory_PhysicalSegmentRetirement(t *testing.T) {
	logger := hclog.NewNullLogger()
	root := t.TempDir()
	database, err := segmentedstorage.NewStorage(
		root, logger, 2, 4, 256*1024*1024,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, database.Close())
	})

	config := &chain.Chain{
		Genesis: &chain.Genesis{
			GasLimit:   1000000,
			Difficulty: 1,
		},
	}

	// Empty blocks isolate the real storage and blockchain-reader paths.
	// This fixture does not perform consensus or transaction execution.
	var (
		parentHash  types.Hash
		retiredHash types.Hash
		retiredPath string
	)
	for number := uint64(0); number <= 5; number++ {
		header := config.Genesis.GenesisHeader()
		header.Number = number
		header.ParentHash = parentHash
		header.Timestamp = number * 2
		header.ComputeHash()

		batch := storage.NewBatchWriterForBlock(database, number)
		batch.PutCanonicalHeader(header)
		require.NoError(t, batch.WriteBatch())

		if number == 1 {
			retiredHash = header.Hash
		}
		if number == 2 {
			entries, err := os.ReadDir(filepath.Join(root, "segments"))
			require.NoError(t, err)
			require.Len(t, entries, 1)
			retiredPath = filepath.Join(root, "segments", entries[0].Name())
		}
		parentHash = header.Hash
	}

	require.NoDirExists(t, retiredPath)
	require.Equal(t, uint64(3), database.HistoryFrom())

	_, exists := database.ReadCanonicalHash(1)
	require.False(t, exists)
	_, err = database.ReadHeader(retiredHash)
	require.Error(t, err)

	localChain, err := blockchain.NewBlockchain(
		logger, database, config, nil, nil, nil,
	)
	require.NoError(t, err)
	require.NoError(t, localChain.ComputeGenesis())

	// Both production lookups must fail after complete segment removal.
	_, exists = localChain.GetBlockByNumber(1, true)
	require.False(t, exists)
	_, exists = localChain.GetBlockByNumber(1, false)
	require.False(t, exists)

	store := &logHistoryDiskStore{
		mockStore: newMockStore(),
		chain:     localChain,
	}
	manager := &FilterManager{
		store:           store,
		blockRangeLimit: 256,
	}

	logs, err := manager.GetLogsForQuery(&LogQuery{
		fromBlock: 1,
		toBlock:   4,
	})
	require.ErrorIs(t, err, ErrPrunedHistory)
	require.Nil(t, logs)

	logs, err = manager.GetLogsForQuery(&LogQuery{
		fromBlock: 3,
		toBlock:   5,
	})
	require.NoError(t, err)
	require.NotNil(t, logs)
	require.Empty(t, logs)
}

/* FINISH ALPHA CHANGE 3297 -- test explicit historical log failures and physical MDBX segment retirement -- */

/* START ALPHA CHANGE 3521 -- verify historical receipt completeness and preserve empty-block fast paths -- */
func newLogReceiptCountTestStore(count int, withLogs bool) *logHistoryTestStore {
	store := newLogHistoryTestStore(0)
	block := store.blocks[2]

	for len(block.Transactions) < 3 {
		block.Transactions = append(block.Transactions, &types.Transaction{
			Hash: types.BytesToHash([]byte{2, byte(len(block.Transactions))}),
		})
	}

	receipts := make([]*types.Receipt, count)
	for index := range receipts {
		receipt := &types.Receipt{
			CumulativeGasUsed: uint64(index+1) * 21000,
			GasUsed:           21000,
		}
		receipt.SetStatus(types.ReceiptSuccess)

		if withLogs {
			receipt.Logs = []*types.Log{
				{
					Address: types.StringToAddress("0x1001"),
					Data:    []byte{byte(index)},
				},
			}
		} else if index == 1 {
			// Failed transactions still require receipts, even without logs.
			receipt.SetStatus(types.ReceiptFailed)
		}

		receipts[index] = receipt
	}

	store.mockStore.receipts[block.Header.Hash] = receipts
	return store
}

func TestLogReceiptCount_RPCQueries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		count    int
		withLogs bool
	}{
		{"empty", 0, false},
		{"shorter", 2, true},
		{"excessive", 4, true},
		{"complete_with_logs", 3, true},
		{"complete_without_logs", 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newLogReceiptCountTestStore(tc.count, tc.withLogs)
			block := store.blocks[2]
			dispatcher := newTestDispatcher(
				t, hclog.NewNullLogger(), store,
				&dispatcherParams{
					chainID:                 100,
					blockRangeLimit:         256,
					jsonRPCBatchLengthLimit: 20,
				},
			)
			t.Cleanup(dispatcher.Close)

			rangeQuery := &LogQuery{fromBlock: 1, toBlock: 2}
			filterID := dispatcher.filterManager.NewLogFilter(rangeQuery, nil)
			require.NotEmpty(t, filterID)

			wantError := fmt.Sprintf(
				"receipt count mismatch for block %s: expected 3, got %d",
				block.Header.Hash.String(), tc.count,
			)

			for _, request := range []struct {
				name      string
				query     *LogQuery
				method    string
				params    string
				priorLogs int
			}{
				{
					name:   "block_hash",
					query:  &LogQuery{BlockHash: &block.Header.Hash},
					method: "eth_getLogs",
					params: fmt.Sprintf(`[{"blockHash":%q}]`, block.Header.Hash.String()),
				},
				{
					name:      "range",
					query:     rangeQuery,
					method:    "eth_getLogs",
					params:    `[{"fromBlock":"0x1","toBlock":"0x2"}]`,
					priorLogs: 1,
				},
				{
					name:      "retained_filter",
					query:     rangeQuery,
					method:    "eth_getFilterLogs",
					params:    fmt.Sprintf(`[%q]`, filterID),
					priorLogs: 1,
				},
			} {
				t.Run(request.name, func(t *testing.T) {
					logs, err := dispatcher.filterManager.GetLogsForQuery(request.query)
					result, rpcError := logHistoryCallRPC(
						t, dispatcher, request.method, request.params,
					)

					if tc.count != 3 {
						require.EqualError(t, err, wantError)
						require.Nil(t, logs,
							"failure must discard logs from earlier blocks")
						require.NotNil(t, rpcError)
						require.Equal(t, wantError, rpcError.Message)
						require.Empty(t, result,
							"failure must not carry a successful log array")
						return
					}

					wantLogs := request.priorLogs
					if tc.withLogs {
						wantLogs += 3
					}

					require.NoError(t, err)
					require.NotNil(t, logs)
					require.Len(t, logs, wantLogs)
					require.Nil(t, rpcError)

					var decoded []json.RawMessage
					require.NoError(t, json.Unmarshal(result, &decoded))
					require.NotNil(t, decoded)
					require.Len(t, decoded, wantLogs)
				})
			}
		})
	}
}

func TestLogReceiptCount_EmptyBlocksSkipReads(t *testing.T) {
	store := newLogReceiptCountTestStore(4, true)
	block := store.blocks[2]
	block.Transactions = nil

	manager := &FilterManager{
		store:           store,
		blockRangeLimit: 256,
	}

	for _, query := range []*LogQuery{
		{BlockHash: &block.Header.Hash},
		{fromBlock: 2, toBlock: 2},
	} {
		logs, err := manager.GetLogsForQuery(query)
		require.NoError(t, err)
		require.NotNil(t, logs)
		require.Empty(t, logs)
	}
	require.Zero(t, store.receiptReads,
		"zero-transaction blocks must retain their receipt-read fast paths")
}

type logReceiptCountDiskStore struct {
	*logHistoryTestStore
	database storage.Storage
}

func (s *logReceiptCountDiskStore) GetReceiptsByHash(
	hash types.Hash,
) ([]*types.Receipt, error) {
	s.receiptReads++
	return s.database.ReadReceipts(hash)
}

func TestLogReceiptCount_StoredRLP(t *testing.T) {
	for _, count := range []int{0, 2, 3, 4} {
		t.Run(fmt.Sprintf("receipts=%d", count), func(t *testing.T) {
			base := newLogReceiptCountTestStore(count, true)
			block := base.blocks[2]
			receipts := types.Receipts(base.mockStore.receipts[block.Header.Hash])
			encoded := receipts.MarshalStoreRLPTo(nil)

			database, err := mdbxstorage.NewMDBXStorageWithMapSize(
				filepath.Join(t.TempDir(), "receipts"),
				hclog.NewNullLogger(),
				64*1024*1024,
			)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, database.Close())
			})

			// Write the raw record explicitly. The ordinary batch writer
			// omits empty receipt lists, so it cannot model a stored,
			// successfully decoded empty list for a nonempty block.
			key := append([]byte(nil), storage.RECEIPTS...)
			key = append(key, block.Header.Hash.Bytes()...)
			batch := database.NewBatch()
			batch.Put(key, encoded)
			require.NoError(t, batch.Write())

			decoded, err := database.ReadReceipts(block.Header.Hash)
			require.NoError(t, err)
			require.Len(t, decoded, count)

			// Block fixtures are synthetic; receipt records use production
			// MDBX storage and stored-RLP decoding.
			store := &logReceiptCountDiskStore{
				logHistoryTestStore: base,
				database:            database,
			}
			manager := &FilterManager{
				store:           store,
				blockRangeLimit: 256,
			}

			for _, query := range []*LogQuery{
				{BlockHash: &block.Header.Hash},
				{fromBlock: 2, toBlock: 2},
			} {
				logs, err := manager.GetLogsForQuery(query)
				if count != 3 {
					require.EqualError(t, err, fmt.Sprintf(
						"receipt count mismatch for block %s: expected 3, got %d",
						block.Header.Hash.String(), count,
					))
					require.Nil(t, logs)
					continue
				}

				require.NoError(t, err)
				require.Len(t, logs, 3)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3521 -- verify historical receipt completeness and preserve empty-block fast paths -- */
