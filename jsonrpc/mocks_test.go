package jsonrpc

import (
	"math/big"
	"sync"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
)

type mockAccount struct {
	address types.Address
	code    []byte
	account *Account
	storage map[types.Hash][]byte
}

func (m *mockAccount) Storage(k types.Hash, v []byte) {
	m.storage[k] = v
}

func (m *mockAccount) Code(code []byte) {
	m.code = code
}

func (m *mockAccount) Nonce(n uint64) {
	m.account.Nonce = n
}

func (m *mockAccount) Balance(n uint64) {
	m.account.Balance = new(big.Int).SetUint64(n)
}

type mockHeader struct {
	header   *types.Header
	receipts []*types.Receipt
}

/* START ALPHA CHANGE 815 -- remove old-chain mock event payload after reorg pruning -- */
type mockEvent struct {
	NewChain []*mockHeader
}

/* FINISH ALPHA CHANGE 815 -- remove old-chain mock event payload after reorg pruning -- */

type mockStore struct {
	JSONRPCStore

	header        *types.Header
	subscription  *blockchain.MockSubscription
	txPoolChannel chan *proto.TxPoolEvent
	receiptsLock  sync.Mutex
	receipts      map[types.Hash][]*types.Receipt
	accounts      map[types.Address]*Account

	// headers is the list of historical headers
	historicalHeaders []*types.Header
}

func newMockStore() *mockStore {
	m := &mockStore{
		header:        &types.Header{Number: 0},
		subscription:  blockchain.NewMockSubscription(),
		accounts:      map[types.Address]*Account{},
		txPoolChannel: make(chan *proto.TxPoolEvent),
	}
	m.addHeader(m.header)

	return m
}

func (m *mockStore) addHeader(header *types.Header) {
	if m.historicalHeaders == nil {
		m.historicalHeaders = []*types.Header{}
	}

	m.historicalHeaders = append(m.historicalHeaders, header)
}

func (m *mockStore) headerLoop(cond func(h *types.Header) bool) *types.Header {
	for _, header := range m.historicalHeaders {
		if cond(header) {
			return header
		}
	}

	return nil
}

func (m *mockStore) emitEvent(evnt *mockEvent) {
	m.receiptsLock.Lock()
	if m.receipts == nil {
		m.receipts = map[types.Hash][]*types.Receipt{}
	}

	/* START ALPHA CHANGE 816 -- emit canonical-chain blockchain events only -- */
	bEvnt := &blockchain.Event{
		NewChain: []*types.Header{},
	}
	/* FINISH ALPHA CHANGE 816 -- emit canonical-chain blockchain events only -- */

	for _, i := range evnt.NewChain {
		m.receipts[i.header.Hash] = i.receipts
		bEvnt.NewChain = append(bEvnt.NewChain, i.header)
	}

	/* START ALPHA CHANGE 817 -- stop emitting removed log headers after reorg pruning -- */
	m.receiptsLock.Unlock()
	/* FINISH ALPHA CHANGE 817 -- stop emitting removed log headers after reorg pruning -- */

	m.subscription.Push(bEvnt)
}

func (m *mockStore) emitTxPoolEvent(eventType proto.EventType, txHash string) {
	evt := &proto.TxPoolEvent{
		Type:   eventType,
		TxHash: txHash,
	}

	m.txPoolChannel <- evt
}

func (m *mockStore) GetAccount(root types.Hash, addr types.Address) (*Account, error) {
	if acc, ok := m.accounts[addr]; ok {
		return acc, nil
	}

	return nil, ErrStateNotFound
}

func (m *mockStore) SetAccount(addr types.Address, account *Account) {
	m.accounts[addr] = account
}

func (m *mockStore) Header() *types.Header {
	return m.header
}

/* START ALPHA CHANGE 3295 -- provide explicit full-history metadata in the shared RPC test store -- */
func (m *mockStore) HistoryFrom() uint64 {
	return 0
}

/* FINISH ALPHA CHANGE 3295 -- provide explicit full-history metadata in the shared RPC test store -- */

func (m *mockStore) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	m.receiptsLock.Lock()
	defer m.receiptsLock.Unlock()

	receipts := m.receipts[hash]

	return receipts, nil
}

func (m *mockStore) SubscribeEvents() blockchain.Subscription {
	return m.subscription
}

/* START ALPHA CHANGE 1418 -- wake mock blockchain subscription on unsubscribe -- */
func (m *mockStore) UnsubscribeEvents(subscription blockchain.Subscription) {
	if mockSubscription, ok := subscription.(*blockchain.MockSubscription); ok {
		mockSubscription.Close()
	}
}

/* FINISH ALPHA CHANGE 1418 -- wake mock blockchain subscription on unsubscribe -- */

func (m *mockStore) TxPoolSubscribe(request *proto.SubscribeRequest) (<-chan *proto.TxPoolEvent, func(), error) {
	txPoolUnsubscribe := func() {
		close(m.txPoolChannel)
	}

	return m.txPoolChannel, txPoolUnsubscribe, nil
}

func (m *mockStore) GetHeaderByNumber(num uint64) (*types.Header, bool) {
	header := m.headerLoop(func(header *types.Header) bool {
		return header.Number == num
	})

	return header, header != nil
}

func (m *mockStore) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	header := m.headerLoop(func(header *types.Header) bool {
		return header.Hash == hash
	})

	return &types.Block{Header: header}, header != nil
}

func (m *mockStore) GetBlockByNumber(num uint64, full bool) (*types.Block, bool) {
	header := m.headerLoop(func(header *types.Header) bool {
		return header.Number == num
	})

	return &types.Block{Header: header}, header != nil
}

func (m *mockStore) GetTxs(inclQueued bool) (
	map[types.Address][]*types.Transaction,
	map[types.Address][]*types.Transaction,
) {
	return nil, nil
}

func (m *mockStore) GetCapacity() (uint64, uint64) {
	return 0, 0
}

/* START ALPHA CHANGE 428 -- remove bridge exit proof mock -- */
/* FINISH ALPHA CHANGE 428 -- remove bridge exit proof mock -- */

func (m *mockStore) GetPeers() int {
	return 20
}

/* START ALPHA CHANGE 429 -- remove bridge state sync proof mock -- */
/* FINISH ALPHA CHANGE 429 -- remove bridge state sync proof mock -- */

func (m *mockStore) FilterExtra(extra []byte) ([]byte, error) {
	return extra, nil
}
