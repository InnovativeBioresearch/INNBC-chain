// START ALPHA CHANGE 2333 -- track indexed transactions by sender for targeted nonce cleanup --
package txpool

import (
	"sync"

	"github.com/innovativebioresearch/innbc-chain/types"
)

// START ALPHA CHANGE 2362 -- clarify indexed ownership during proposal selection --
// lookupMap tracks admitted transactions, including those selected for
// unfinalized proposals. Selection retains their queue and index ownership.
// Both maps reference the same transaction objects.
// FINISH ALPHA CHANGE 2362 -- clarify indexed ownership during proposal selection --
type lookupMap struct {
	sync.RWMutex
	all      map[types.Hash]*types.Transaction
	bySender map[types.Address]map[types.Hash]*types.Transaction
}

// add inserts the given transaction into the map. Returns false
// if it already exists. [thread-safe]
func (m *lookupMap) add(tx *types.Transaction) bool {
	m.Lock()
	defer m.Unlock()

	if _, exists := m.all[tx.Hash]; exists {
		return false
	}

	if m.all == nil {
		m.all = make(map[types.Hash]*types.Transaction)
	}
	if m.bySender == nil {
		m.bySender = make(map[types.Address]map[types.Hash]*types.Transaction)
	}

	senderTransactions := m.bySender[tx.From]
	if senderTransactions == nil {
		senderTransactions = make(map[types.Hash]*types.Transaction)
		m.bySender[tx.From] = senderTransactions
	}

	m.all[tx.Hash] = tx
	senderTransactions[tx.Hash] = tx

	return true
}

// removeLocked removes a hash from both indexes.
// The caller must hold the lookup map's write lock.
func (m *lookupMap) removeLocked(hash types.Hash) {
	stored, exists := m.all[hash]
	if !exists {
		return
	}

	delete(m.all, hash)

	// A transaction loaded from a block can have an unset From.
	// Account ownership comes from the admitted, indexed transaction.
	senderTransactions := m.bySender[stored.From]
	delete(senderTransactions, hash)

	if len(senderTransactions) == 0 {
		delete(m.bySender, stored.From)
	}
}

// remove removes the given transactions from the map. [thread-safe]
func (m *lookupMap) remove(txs ...*types.Transaction) {
	m.Lock()
	defer m.Unlock()

	for _, tx := range txs {
		m.removeLocked(tx.Hash)
	}
}

// START ALPHA CHANGE 2363 -- clarify confirmed-nonce index cleanup --
// removeBelowNonces removes indexed transactions whose nonces have
// already been consumed according to successfully read account state.
//
// Call this after queue cleanup, which owns slot and pending-counter
// updates. This method changes only the indexes.
//
// Only the supplied senders are inspected. Transactions at or above
// their state nonce, and transactions from other senders, remain indexed.
// FINISH ALPHA CHANGE 2363 -- clarify confirmed-nonce index cleanup --
func (m *lookupMap) removeBelowNonces(stateNonces map[types.Address]uint64) {
	m.Lock()
	defer m.Unlock()

	for address, nonce := range stateNonces {
		for hash, tx := range m.bySender[address] {
			if tx.Nonce < nonce {
				m.removeLocked(hash)
			}
		}
	}
}

// START ALPHA CHANGE 2364 -- clarify sender ownership for unfinalized selections --
// hasSender reports whether the sender owns any indexed transactions,
// including transactions selected for an unfinalized proposal.
// FINISH ALPHA CHANGE 2364 -- clarify sender ownership for unfinalized selections --
func (m *lookupMap) hasSender(address types.Address) bool {
	m.RLock()
	defer m.RUnlock()

	return len(m.bySender[address]) != 0
}

// get returns the transaction associated with the given hash. [thread-safe]
func (m *lookupMap) get(hash types.Hash) (*types.Transaction, bool) {
	m.RLock()
	defer m.RUnlock()

	tx, ok := m.all[hash]
	if !ok {
		return nil, false
	}

	return tx, true
}

// FINISH ALPHA CHANGE 2333 -- track indexed transactions by sender for targeted nonce cleanup --
