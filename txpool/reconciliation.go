/* START ALPHA CHANGE 2197 -- retry incomplete pool reconciliation against current state -- */
package txpool

import (
	/* START ALPHA CHANGE 2200 -- report shutdown before protected pool consumption -- */
	"errors"
	/* FINISH ALPHA CHANGE 2200 -- report shutdown before protected pool consumption -- */
	"fmt"
	"time"

	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
)

const resetRetryInterval = time.Second

/* START ALPHA CHANGE 2200 -- reconcile and protect proposal pool consumption -- */
// WithReconciledPool runs consume only after missed resets have been
// reconciled. It excludes resets for the complete callback, including
// selection, execution and the resulting Pop, Demote or Drop operation.
//
// The callback may use Prepare, Peek, Pop, Demote and Drop. It must not
// call methods which acquire resetMu themselves, including AddTx,
// GetNonce, ResetWithHeaders or WithReconciledPool.
//
// Admission can continue concurrently under the existing account locks.
// Do not include the idle block-timer wait in this callback.
func (p *TxPool) WithReconciledPool(consume func() error) error {
	if !p.enterRuntimeWork() {
		return errors.New("txpool is shutting down")
	}
	defer p.leaveRuntimeWork()

	if err := p.lockReconciled(); err != nil {
		return err
	}
	defer p.resetMu.RUnlock()

	return consume()
}

/* FINISH ALPHA CHANGE 2200 -- reconcile and protect proposal pool consumption -- */

// recordResetFailureLocked retains incomplete reset state.
// The caller must hold resetMu for writing.
func (p *TxPool) recordResetFailureLocked(err error) {
	p.resetErr = err
	p.resetRetryAt = time.Now().Add(resetRetryInterval)
}

// lockReconciled returns with resetMu held for reading on success.
// The caller must then defer p.resetMu.RUnlock().
//
// On failure, no resetMu lock remains held. Requests within the retry
// window receive the retained error without another backend attempt.
func (p *TxPool) lockReconciled() error {
	for {
		p.resetMu.RLock()

		if p.resetErr == nil {
			return nil
		}

		retained := p.resetErr
		if time.Now().Before(p.resetRetryAt) {
			p.resetMu.RUnlock()

			return retained
		}

		p.resetMu.RUnlock()

		// Pending-nonce queries can initiate recovery too. Register
		// that maintenance work so pool shutdown waits for it.
		if !p.enterRuntimeWork() {
			return retained
		}

		p.resetMu.Lock()
		err := p.retryResetLocked()
		p.resetMu.Unlock()

		p.leaveRuntimeWork()

		if err != nil {
			return err
		}

		// Reacquire the read lock and check again. Another reset may
		// have failed between releasing the writer lock and this point.
	}
}

// retryResetLocked retries only when an incomplete reset exists and its
// retry window has elapsed. The caller must hold resetMu for writing.
func (p *TxPool) retryResetLocked() error {
	if p.resetErr == nil {
		return nil
	}

	if time.Now().Before(p.resetRetryAt) {
		return p.resetErr
	}

	if err := p.reconcileCurrentHeadLocked(); err != nil {
		p.recordResetFailureLocked(err)

		return err
	}

	p.resetErr = nil
	p.resetRetryAt = time.Time{}

	return nil
}

// reconcileCurrentHeadLocked repairs missed reset work using current
// account nonces. It does not retain an accumulating block-header backlog.
//
// All account reads and the fee refresh must succeed before queue or
// transaction-index changes are published.
//
// The caller must hold resetMu for writing. This excludes new admissions,
// pending-nonce queries and nonce-hole pruning during reconciliation.
func (p *TxPool) reconcileCurrentHeadLocked() error {
	header := p.store.Header()
	if header == nil {
		err := state.WrapLocalStateFailure(
			"reconcile transaction pool",
			fmt.Errorf("current chain header is unavailable"),
		)
		state.LogLocalStateFailure(p.logger, state.LocalStateLogTxPool, err)

		return err
	}

	addresses := make(map[types.Address]struct{})

	// Include initialized accounts even when their queues are empty:
	// pending-nonce queries can still consume their cached nonce.
	p.accounts.Range(func(key, _ any) bool {
		address := key.(types.Address)
		addresses[address] = struct{}{}

		return true
	})

	// START ALPHA CHANGE 2336 -- collect recovery senders without copying the transaction index --
	// START ALPHA CHANGE 2365 -- clarify indexed sender collection during recovery --
	// Include every indexed sender in the reconciliation set.
	// Proposal selection retains these entries and their queued transactions.
	// FINISH ALPHA CHANGE 2365 -- clarify indexed sender collection during recovery --
	p.index.RLock()
	for address := range p.index.bySender {
		addresses[address] = struct{}{}
	}
	p.index.RUnlock()
	// FINISH ALPHA CHANGE 2336 -- collect recovery senders without copying the transaction index --

	stateNonces := make(map[types.Address]uint64, len(addresses))

	for address := range addresses {
		nonce, err := p.store.GetNonce(header.StateRoot, address)
		if err != nil {
			localErr := state.WrapLocalStateFailure(
				fmt.Sprintf("reconcile pool nonce for %s", address),
				err,
			)
			state.LogLocalStateFailure(
				p.logger,
				state.LocalStateLogTxPool,
				localErr,
			)

			return localErr
		}

		stateNonces[address] = nonce
	}

	// SetBaseFee reports its own error through the existing logging policy.
	// A successful standalone fee refresh cannot clear resetErr.
	if err := p.SetBaseFee(header); err != nil {
		return err
	}

	// START ALPHA CHANGE 2337 -- share confirmed-nonce cleanup between ordinary reset and recovery --
	// resetAccounts prunes queues and updates their counters first,
	// then removes obsolete index-only entries and retires idle accounts.
	p.resetAccounts(stateNonces)
	// FINISH ALPHA CHANGE 2337 -- share confirmed-nonce cleanup between ordinary reset and recovery --

	return nil
}

/* FINISH ALPHA CHANGE 2197 -- retry incomplete pool reconciliation against current state -- */
