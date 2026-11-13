// START ALPHA CHANGE 2307 -- reclaim unused accounts without losing active ownership --
package txpool

import (
	"sync/atomic"

	"github.com/innovativebioresearch/innbc-chain/types"
)

// lockActive acquires the established account mutation lock order.
// A caller holding an old pointer must retry lookup if retirement won
// the race before these locks were acquired.
func (a *account) lockActive() bool {
	a.promoted.lock(true)
	a.enqueued.lock(true)
	a.nonceToTx.lock()

	if a.retired {
		a.unlockActive()

		return false
	}

	return true
}

func (a *account) unlockActive() {
	a.nonceToTx.unlock()
	a.enqueued.unlock()
	a.promoted.unlock()
}

// lockAccountForAdmission returns a currently registered account with
// all account mutation locks held.
//
// The caller already holds resetMu for reading through lockReconciled,
// so a retry cannot cross a pool reset.
func (p *TxPool) lockAccountForAdmission(address types.Address) (*account, error) {
	for {
		a, err := p.getOrCreateAccount(address)
		if err != nil {
			return nil, err
		}

		if a.lockActive() {
			return a, nil
		}
	}
}

// START ALPHA CHANGE 2348 -- document shared ownership through proposal selection --
// retireIdleAccountLocked removes only an account with no remaining work.
//
// The caller must hold promoted, enqueued and nonceToTx locks.
// Proposal selection retains queue and index ownership. Successful reset
// removes consumed transactions before testing retirement eligibility.
// FINISH ALPHA CHANGE 2348 -- document shared ownership through proposal selection --
func (p *TxPool) retireIdleAccountLocked(address types.Address, a *account) {
	if a.retired ||
		a.promoted.length() != 0 ||
		a.enqueued.length() != 0 ||
		len(a.nonceToTx.mapping) != 0 ||
		p.index.hasSender(address) {
		return
	}

	p.accounts.lifecycleMu.Lock()
	defer p.accounts.lifecycleMu.Unlock()

	// Pointer identity prevents an old operation from removing a newer
	// account created for the same address.
	if p.accounts.CompareAndDelete(address, a) {
		a.retired = true
		atomic.AddUint64(&p.accounts.count, ^uint64(0))
	}
}

// retireIdleAccount obtains the account locks before testing eligibility.
// It is safe to pass an account pointer captured before another operation
// retired it.
func (p *TxPool) retireIdleAccount(address types.Address, a *account) {
	if a == nil || !a.lockActive() {
		return
	}
	defer a.unlockActive()

	p.retireIdleAccountLocked(address, a)
}

// FINISH ALPHA CHANGE 2307 -- reclaim unused accounts without losing active ownership --
