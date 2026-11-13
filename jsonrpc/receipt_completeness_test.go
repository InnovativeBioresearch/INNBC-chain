/* START ALPHA CHANGE 3570 -- test receipt RPC failures, completeness and stored RLP -- */
package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type receiptCompletenessStore struct {
	*mockBlockStore

	receiptErr      error
	receiptReads    int
	lookupOverrides map[types.Hash]types.Hash
	database        storage.Storage
}

func (s *receiptCompletenessStore) ReadTxLookup(
	hash types.Hash,
) (types.Hash, bool) {
	if blockHash, ok := s.lookupOverrides[hash]; ok {
		return blockHash, true
	}
	return s.mockBlockStore.ReadTxLookup(hash)
}

func (s *receiptCompletenessStore) GetReceiptsByHash(
	hash types.Hash,
) ([]*types.Receipt, error) {
	s.receiptReads++

	if s.receiptErr != nil {
		return nil, s.receiptErr
	}
	if s.database != nil {
		return s.database.ReadReceipts(hash)
	}
	return s.mockBlockStore.GetReceiptsByHash(hash)
}

func newReceiptCompletenessStore(
	count int,
) (*receiptCompletenessStore, *types.Block) {
	base := newMockBlockStore()
	block := newTestBlock(1, types.BytesToHash([]byte{0x91}))

	for index := 0; index < 3; index++ {
		block.Transactions = append(
			block.Transactions,
			newTestTransaction(uint64(index), addr0),
		)
	}
	base.add(block)

	complete := make([]*types.Receipt, 3)
	for index, logCount := range []int{2, 0, 1} {
		source := &types.Receipt{
			CumulativeGasUsed: uint64(index+1) * 21000,
			GasUsed:           21000,
			TxHash:            block.Transactions[index].Hash,
		}
		source.SetStatus(types.ReceiptSuccess)

		if index == 1 {
			// Failed transactions still require receipts without logs.
			source.SetStatus(types.ReceiptFailed)
		}

		for logIndex := 0; logIndex < logCount; logIndex++ {
			source.Logs = append(source.Logs, &types.Log{
				Address: addr1,
				Topics: []types.Hash{
					types.BytesToHash([]byte{
						byte(index + 1), byte(logIndex + 1),
					}),
				},
				Data: []byte{byte(index), byte(logIndex)},
			})
		}

		complete[index] = source
	}

	receipts := make([]*types.Receipt, count)
	for index := range receipts {
		receipts[index] = complete[index%len(complete)]
	}
	base.receipts[block.Hash()] = receipts

	return &receiptCompletenessStore{mockBlockStore: base}, block
}

func receiptCompletenessCallRPC(
	t *testing.T,
	store *receiptCompletenessStore,
	hash types.Hash,
) (json.RawMessage, *ObjectError) {
	t.Helper()

	dispatcher := &Dispatcher{
		logger: hclog.NewNullLogger(),
		params: &dispatcherParams{},
	}
	t.Cleanup(dispatcher.Close)
	require.NoError(t, dispatcher.registerService(
		"eth", newTestEthEndpoint(store),
	))

	return logHistoryCallRPC(
		t,
		dispatcher,
		"eth_getTransactionReceipt",
		fmt.Sprintf(`[%q]`, hash.String()),
	)
}

func TestReceiptCompleteness_Queries(t *testing.T) {
	readFailure := errors.New("receipt storage read failed")

	for _, tc := range []struct {
		name    string
		count   int
		readErr error
	}{
		{"read_failure", 3, readFailure},
		{"missing_record", 3, storage.ErrNotFound},
		{"empty", 0, nil},
		{"shorter", 2, nil},
		{"excessive", 4, nil},
		{"complete", 3, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, txIndex := range []int{0, 1, 2} {
				t.Run(fmt.Sprintf("transaction_%d", txIndex), func(t *testing.T) {
					store, block := newReceiptCompletenessStore(tc.count)
					store.receiptErr = tc.readErr
					tx := block.Transactions[txIndex]

					var result any
					var err error
					require.NotPanics(t, func() {
						result, err = newTestEthEndpoint(store).
							GetTransactionReceipt(tx.Hash)
					})
					require.Equal(t, 1, store.receiptReads)

					raw, rpcError := receiptCompletenessCallRPC(
						t, store, tx.Hash,
					)
					require.Equal(t, 2, store.receiptReads,
						"each endpoint invocation must read receipts once")

					var wantError string
					if tc.readErr != nil {
						require.ErrorIs(t, err, tc.readErr)
						wantError = fmt.Sprintf(
							"failed to read receipts for block %s: %v",
							block.Hash().String(), tc.readErr,
						)
					} else if tc.count != len(block.Transactions) {
						wantError = fmt.Sprintf(
							"receipt count mismatch for block %s: expected 3, got %d",
							block.Hash().String(), tc.count,
						)
					}

					if wantError != "" {
						require.EqualError(t, err, wantError)
						require.False(t, errors.Is(err, ErrPrunedHistory))
						require.Nil(t, result)
						require.NotNil(t, rpcError)
						require.Equal(t, wantError, rpcError.Message)
						require.Empty(t, raw,
							"failure must not carry a successful result")
						return
					}

					require.NoError(t, err)
					require.Nil(t, rpcError)

					got, ok := result.(*receipt)
					require.True(t, ok)
					require.NotNil(t, got)

					source := store.receipts[block.Hash()][txIndex]
					require.Equal(t, tx.Hash, got.TxHash)
					require.Equal(t, block.Hash(), got.BlockHash)
					require.Equal(t, uint64(txIndex), uint64(got.TxIndex))
					require.Equal(t, source.GasUsed, uint64(got.GasUsed))
					require.Equal(t,
						source.CumulativeGasUsed,
						uint64(got.CumulativeGasUsed),
					)
					require.NotNil(t, got.Status)
					require.Equal(t,
						uint64(*source.Status), uint64(*got.Status),
					)

					require.NotNil(t, got.Logs)
					require.Len(t, got.Logs, len(source.Logs))

					// The middle receipt has no logs, so the third
					// transaction's first log still has block-wide index 2.
					baseLogIndex := []uint64{0, 2, 2}[txIndex]
					for index, log := range got.Logs {
						sourceLog := source.Logs[index]
						require.Equal(t, sourceLog.Address, log.Address)
						require.Equal(t, sourceLog.Topics, log.Topics)
						require.Equal(t, sourceLog.Data, []byte(log.Data))
						require.Equal(t, tx.Hash, log.TxHash)
						require.Equal(t, uint64(txIndex), uint64(log.TxIndex))
						require.Equal(t,
							baseLogIndex+uint64(index),
							uint64(log.LogIndex),
						)
					}

					var wire struct {
						TransactionHash string `json:"transactionHash"`
						Logs            []struct {
							LogIndex string `json:"logIndex"`
						} `json:"logs"`
					}
					require.NoError(t, json.Unmarshal(raw, &wire))
					require.Equal(t, tx.Hash.String(), wire.TransactionHash)
					require.NotNil(t, wire.Logs)
					require.Len(t, wire.Logs, len(source.Logs))

					for index, log := range wire.Logs {
						require.Equal(t,
							fmt.Sprintf("0x%x", baseLogIndex+uint64(index)),
							log.LogIndex,
						)
					}
				})
			}
		})
	}
}

func TestReceiptCompleteness_UnknownTransactions(t *testing.T) {
	for _, staleLookup := range []bool{false, true} {
		t.Run(fmt.Sprintf("stale_lookup=%t", staleLookup), func(t *testing.T) {
			store, block := newReceiptCompletenessStore(3)
			store.receiptErr = errors.New("receipts must not be read")
			unknown := types.BytesToHash([]byte{0xff})

			if staleLookup {
				store.lookupOverrides = map[types.Hash]types.Hash{
					unknown: block.Hash(),
				}
			}

			result, err := newTestEthEndpoint(store).
				GetTransactionReceipt(unknown)
			require.NoError(t, err)
			require.Nil(t, result)
			require.Zero(t, store.receiptReads)

			raw, rpcError := receiptCompletenessCallRPC(t, store, unknown)
			require.Nil(t, rpcError)
			require.Equal(t, "null", string(raw))
			require.Zero(t, store.receiptReads)
		})
	}
}

func TestReceiptCompleteness_StoredRLP(t *testing.T) {
	for _, count := range []int{0, 2, 3, 4} {
		t.Run(fmt.Sprintf("receipts=%d", count), func(t *testing.T) {
			store, block := newReceiptCompletenessStore(count)
			blockHash := block.Hash()
			records := types.Receipts(store.receipts[blockHash])
			encoded := records.MarshalStoreRLPTo(nil)

			database, err := mdbxstorage.NewMDBXStorageWithMapSize(
				filepath.Join(t.TempDir(), "receipts"),
				hclog.NewNullLogger(),
				64*1024*1024,
			)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, database.Close())
			})

			// Explicitly store even the encoded empty list; ordinary
			// batch receipt writing can omit reconstructible empty lists.
			key := append([]byte(nil), storage.RECEIPTS...)
			key = append(key, blockHash.Bytes()...)
			batch := database.NewBatch()
			batch.Put(key, encoded)
			require.NoError(t, batch.Write())

			// Establish successful production decoding before checking
			// endpoint rejection. Block and lookup fixtures are synthetic.
			decoded, err := database.ReadReceipts(blockHash)
			require.NoError(t, err)
			require.Len(t, decoded, count)

			store.database = database

			for _, txIndex := range []int{0, 2} {
				var result any
				var receiptErr error
				require.NotPanics(t, func() {
					result, receiptErr = newTestEthEndpoint(store).
						GetTransactionReceipt(block.Transactions[txIndex].Hash)
				})

				if count != len(block.Transactions) {
					require.EqualError(t, receiptErr, fmt.Sprintf(
						"receipt count mismatch for block %s: expected 3, got %d",
						blockHash.String(), count,
					))
					require.Nil(t, result)
					continue
				}

				require.NoError(t, receiptErr)
				require.NotNil(t, result)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3570 -- test receipt RPC failures, completeness and stored RLP -- */
