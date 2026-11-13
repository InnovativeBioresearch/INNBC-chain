/* START ALPHA CHANGE 3574 -- test live receipt completeness and background delivery after rejection -- */
package jsonrpc

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type liveReceiptCountStore struct {
	*mockStore

	blocks map[types.Hash]*types.Block
}

func newLiveReceiptCountStore(blocks ...*types.Block) *liveReceiptCountStore {
	store := &liveReceiptCountStore{
		mockStore: newMockStore(),
		blocks:    make(map[types.Hash]*types.Block),
	}

	for _, block := range blocks {
		store.blocks[block.Hash()] = block
		store.addHeader(block.Header)
	}

	return store
}

func (s *liveReceiptCountStore) GetBlockByHash(
	hash types.Hash,
	full bool,
) (*types.Block, bool) {
	if block, ok := s.blocks[hash]; ok {
		return block, true
	}
	return s.mockStore.GetBlockByHash(hash, full)
}

func (s *liveReceiptCountStore) GetBlockByNumber(
	number uint64,
	full bool,
) (*types.Block, bool) {
	for _, block := range s.blocks {
		if block.Number() == number {
			return block, true
		}
	}
	return s.mockStore.GetBlockByNumber(number, full)
}

func liveReceiptCountBlock(
	number uint64,
	count int,
) (*types.Block, []*types.Receipt) {
	block := &types.Block{
		Header: &types.Header{
			Number: number,
			Hash:   types.BytesToHash([]byte{0xa0, byte(number)}),
		},
	}

	// The block always has three transactions, independently of receipt count.
	for index := range 3 {
		block.Transactions = append(block.Transactions, &types.Transaction{
			Hash: types.BytesToHash([]byte{
				byte(number), byte(index + 0x10),
			}),
		})
	}

	receipts := make([]*types.Receipt, count)
	for index := range receipts {
		// Keep every transaction hash zero to exercise hash reconstruction,
		// including the excess receipt in the four-receipt regression.
		source := &types.Receipt{}

		// The middle transaction has a receipt without logs.
		if index != 1 {
			for logIndex := range 2 {
				source.Logs = append(source.Logs, &types.Log{
					Address: types.BytesToAddress([]byte{byte(logIndex + 1)}),
					Topics: []types.Hash{
						types.BytesToHash([]byte{byte(logIndex + 1)}),
					},
					Data: []byte{byte(index), byte(logIndex)},
				})
			}
		}

		receipts[index] = source
	}

	return block, receipts
}

func liveReceiptCountFilters(
	t *testing.T,
	manager *FilterManager,
) []*logFilter {
	t.Helper()

	var filters []*logFilter
	for _, query := range []*LogQuery{
		{},
		{
			Addresses: []types.Address{
				types.BytesToAddress([]byte{1}),
			},
			Topics: [][]types.Hash{
				{types.BytesToHash([]byte{1})},
			},
		},
	} {
		id := manager.NewLogFilter(query, nil)
		require.NotEmpty(t, id)

		filter, err := manager.GetLogFilterFromID(id)
		require.NoError(t, err)
		filters = append(filters, filter)
	}

	return filters
}

func TestLiveReceiptCount_RejectsIncomplete(t *testing.T) {
	for _, count := range []int{0, 2, 4} {
		t.Run(fmt.Sprintf("receipts_%d", count), func(t *testing.T) {
			block, receipts := liveReceiptCountBlock(1, count)
			store := newLiveReceiptCountStore(block)
			store.receipts = map[types.Hash][]*types.Receipt{
				block.Hash(): receipts,
			}

			manager := NewFilterManager(hclog.NewNullLogger(), store, 256)
			t.Cleanup(manager.Close)
			filters := liveReceiptCountFilters(t, manager)

			prior := &Log{
				TxHash: types.BytesToHash([]byte{0xee}),
			}
			for _, filter := range filters {
				filter.appendLog(prior)
			}

			var err error
			require.NotPanics(t, func() {
				err = manager.appendLogsToFilters(toBlock(block, false))
			})

			require.EqualError(t, err, fmt.Sprintf(
				"receipt count mismatch for block %s: expected 3, got %d",
				block.Hash().String(), count,
			))

			// No filter may receive partial logs or lose earlier queued logs.
			for _, filter := range filters {
				require.Equal(t, []*Log{prior}, filter.takeLogUpdates())
			}

			// Rejection must precede even transaction-hash reconstruction.
			for _, source := range receipts {
				require.Equal(t, types.ZeroHash, source.TxHash)
			}
		})
	}
}

func TestLiveReceiptCount_CompleteDelivery(t *testing.T) {
	block, receipts := liveReceiptCountBlock(1, 3)
	store := newLiveReceiptCountStore(block)
	store.receipts = map[types.Hash][]*types.Receipt{
		block.Hash(): receipts,
	}

	manager := NewFilterManager(hclog.NewNullLogger(), store, 256)
	t.Cleanup(manager.Close)
	filters := liveReceiptCountFilters(t, manager)

	require.NoError(t, manager.appendLogsToFilters(toBlock(block, false)))

	allLogs := filters[0].takeLogUpdates()
	require.Len(t, allLogs, 4)

	expectedTransactions := []int{0, 0, 2, 2}
	for index, log := range allLogs {
		txIndex := expectedTransactions[index]
		source := receipts[txIndex].Logs[index%2]

		require.Equal(t, uint64(index), uint64(log.LogIndex))
		require.Equal(t, uint64(txIndex), uint64(log.TxIndex))
		require.Equal(t, block.Transactions[txIndex].Hash, log.TxHash)
		require.Equal(t, block.Hash(), log.BlockHash)
		require.Equal(t, block.Number(), uint64(log.BlockNumber))
		require.Equal(t, source.Address, log.Address)
		require.Equal(t, source.Topics, log.Topics)
		require.Equal(t, source.Data, []byte(log.Data))
	}

	selectedLogs := filters[1].takeLogUpdates()
	require.Len(t, selectedLogs, 2)

	for index, expectedLogIndex := range []uint64{0, 2} {
		txIndex := []int{0, 2}[index]
		log := selectedLogs[index]

		require.Equal(t, expectedLogIndex, uint64(log.LogIndex))
		require.Equal(t, uint64(txIndex), uint64(log.TxIndex))
		require.Equal(t, block.Transactions[txIndex].Hash, log.TxHash)
		require.Equal(t, types.BytesToAddress([]byte{1}), log.Address)
		require.Equal(t,
			[]types.Hash{types.BytesToHash([]byte{1})},
			log.Topics,
		)
	}
}

func TestLiveReceiptCount_EmptyBlock(t *testing.T) {
	block, receipts := liveReceiptCountBlock(1, 0)
	block.Transactions = nil

	store := newLiveReceiptCountStore(block)
	store.receipts = map[types.Hash][]*types.Receipt{
		block.Hash(): receipts,
	}

	manager := NewFilterManager(hclog.NewNullLogger(), store, 256)
	t.Cleanup(manager.Close)
	filters := liveReceiptCountFilters(t, manager)

	require.NoError(t, manager.appendLogsToFilters(toBlock(block, false)))

	for _, filter := range filters {
		require.Empty(t, filter.takeLogUpdates())
	}
}

type liveReceiptCountErrorWriter struct {
	message string
	seen    chan struct{}
}

func (w *liveReceiptCountErrorWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), w.message) {
		select {
		case w.seen <- struct{}{}:
		default:
		}
	}
	return len(data), nil
}

func TestLiveReceiptCount_BackgroundContinues(t *testing.T) {
	badBlock, badReceipts := liveReceiptCountBlock(1, 2)
	goodBlock, goodReceipts := liveReceiptCountBlock(2, 3)

	store := newLiveReceiptCountStore(badBlock, goodBlock)
	store.receipts = map[types.Hash][]*types.Receipt{
		badBlock.Hash():  badReceipts,
		goodBlock.Hash(): goodReceipts,
	}

	observer := &liveReceiptCountErrorWriter{
		message: fmt.Sprintf(
			"receipt count mismatch for block %s: expected 3, got 2",
			badBlock.Hash().String(),
		),
		seen: make(chan struct{}, 1),
	}
	logger := hclog.New(&hclog.LoggerOptions{
		Level:  hclog.Error,
		Output: observer,
	})
	manager := NewFilterManager(logger, store, 256)

	runDone := make(chan struct{})
	t.Cleanup(func() {
		manager.Close()

		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
			t.Error("filter manager did not stop after Close")
		}
	})

	go func() {
		defer close(runDone)
		manager.Run()
	}()

	filterID := manager.NewLogFilter(&LogQuery{}, nil)
	require.NotEmpty(t, filterID)

	select {
	case store.subscription.GetEventCh() <- &blockchain.Event{
		NewChain: []*types.Header{badBlock.Header},
	}:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not receive the malformed block")
	}

	// The exact error report proves the malformed event was processed;
	// delivery of the next event alone would not establish that.
	select {
	case <-observer.seen:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not report the malformed block's receipt mismatch")
	}

	updates, err := manager.GetFilterChanges(filterID)
	require.NoError(t, err)
	require.Empty(t, updates)

	select {
	case store.subscription.GetEventCh() <- &blockchain.Event{
		NewChain: []*types.Header{goodBlock.Header},
	}:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not receive the subsequent valid block")
	}

	var delivered []*Log
	var pollErr error
	require.Eventually(t, func() bool {
		updates, err := manager.GetFilterChanges(filterID)
		if err != nil {
			pollErr = err
			return true
		}

		logs, ok := updates.([]*Log)
		if !ok {
			pollErr = fmt.Errorf("unexpected filter update type %T", updates)
			return true
		}

		delivered = append(delivered, logs...)
		return len(delivered) >= 4
	}, 5*time.Second, time.Millisecond)

	require.NoError(t, pollErr)
	require.Len(t, delivered, 4)
	for index, log := range delivered {
		require.Equal(t, goodBlock.Hash(), log.BlockHash)
		require.Equal(t, uint64(index), uint64(log.LogIndex))
	}
}

/* FINISH ALPHA CHANGE 3574 -- test live receipt completeness and background delivery after rejection -- */
