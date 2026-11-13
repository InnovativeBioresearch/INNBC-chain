package txpool

import (
	"sync/atomic"

	/* START ALPHA CHANGE 2164 -- classify pending nonce backend failures -- */
	"github.com/innovativebioresearch/innbc-chain/state"
	/* FINISH ALPHA CHANGE 2164 -- classify pending nonce backend failures -- */
	"github.com/innovativebioresearch/innbc-chain/types"
)

/* QUERY methods */
// Used to query the pool for specific state info.

// GetNonce returns the next nonce for the account
//
// -> Returns the value from the TxPool if the account is initialized in-memory
//
// -> Returns the value from the world state otherwise
/* START ALPHA CHANGE 2164 -- propagate pending nonce fallback failures -- */
func (p *TxPool) GetNonce(addr types.Address) (uint64, error) {
	/* START ALPHA CHANGE 2198 -- reconcile missed resets before returning a pending nonce -- */
	if err := p.lockReconciled(); err != nil {
		return 0, err
	}
	defer p.resetMu.RUnlock()
	/* FINISH ALPHA CHANGE 2198 -- reconcile missed resets before returning a pending nonce -- */

	// START ALPHA CHANGE 2313 -- retry pending nonce lookup after account retirement --
	for {
		account := p.accounts.get(addr)
		if account == nil {
			stateRoot := p.store.Header().StateRoot
			stateNonce, err := p.store.GetNonce(stateRoot, addr)
			if err != nil {
				return 0, state.WrapLocalStateFailure("read pending account nonce", err)
			}

			return stateNonce, nil
		}

		account.promoted.lock(false)
		retired := account.retired
		nonce := account.getNonce()
		account.promoted.unlock()

		if !retired {
			return nonce, nil
		}
	}
	// FINISH ALPHA CHANGE 2313 -- retry pending nonce lookup after account retirement --
}

/* FINISH ALPHA CHANGE 2164 -- propagate pending nonce fallback failures -- */

// GetCapacity returns the current number of slots
// occupied in the pool as well as the max limit
func (p *TxPool) GetCapacity() (uint64, uint64) {
	return p.gauge.read(), p.gauge.max
}

// GetPendingTx returns the transaction by hash in the TxPool (pending txn) [Thread-safe]
func (p *TxPool) GetPendingTx(txHash types.Hash) (*types.Transaction, bool) {
	tx, ok := p.index.get(txHash)
	if !ok {
		return nil, false
	}

	return tx, true
}

// GetTxs gets pending and queued transactions
func (p *TxPool) GetTxs(inclQueued bool) (
	allPromoted, allEnqueued map[types.Address][]*types.Transaction,
) {
	return p.accounts.allTxs(inclQueued)
}

// GetBaseFee returns current base fee
func (p *TxPool) GetBaseFee() uint64 {
	return atomic.LoadUint64(&p.baseFee)
}

/* START ALPHA CHANGE 2191 -- report failed fee refreshes and clear them only after success -- */
// admissionBaseFee returns an error when the latest refresh failed.
// GetBaseFee continues exposing the last successfully calculated cached value.
func (p *TxPool) admissionBaseFee() (uint64, error) {
	p.baseFeeMu.RLock()
	defer p.baseFeeMu.RUnlock()

	if p.baseFeeErr != nil {
		return 0, p.baseFeeErr
	}

	return atomic.LoadUint64(&p.baseFee), nil
}

// SetBaseFee serializes refreshes and retains failures for admission checks.
func (p *TxPool) SetBaseFee(header *types.Header) error {
	p.baseFeeMu.Lock()

	baseFee, err := p.store.CalculateBaseFee(header)
	if err == nil {
		atomic.StoreUint64(&p.baseFee, baseFee)
	}
	p.baseFeeErr = err

	p.baseFeeMu.Unlock()

	if err != nil {
		// Do not hold the fee mutex while writing to the log sink.
		if !state.LogLocalStateFailure(p.logger, state.LocalStateLogTxPool, err) {
			p.logger.Error("failed to update txpool base fee", "err", err)
		}

		return err
	}

	return nil
}

/* FINISH ALPHA CHANGE 2191 -- report failed fee refreshes and clear them only after success -- */
