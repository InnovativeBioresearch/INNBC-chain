/* START ALPHA CHANGE 3897 -- reject terminal transaction nonces without mutating pool ownership -- */

package txpool

import (
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func nonceLimitTestPool(
	t *testing.T,
	stateNonce uint64,
) (*TxPool, crypto.TxSigner) {
	t.Helper()

	header := mockHeader.Copy()
	header.BaseFee = 1

	store := NewDefaultMockStore(header)
	store.nonce = stateNonce

	pool, err := newTestPool(store)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	signer := crypto.NewLondonSigner(
		100, true, crypto.NewEIP155Signer(100, true),
	)
	pool.SetSigner(signer)
	pool.SetSealing(true)
	require.NoError(t, pool.SetBaseFee(header))

	// Promotion is driven explicitly, without background pipeline workers.
	pool.promoteReqCh = make(chan promoteRequest, 1)

	return pool, signer
}

func nonceLimitSignedTransaction(
	t *testing.T,
	owner *eoa,
	signer crypto.TxSigner,
	txType types.TxType,
	nonce uint64,
	creation bool,
) *types.Transaction {
	t.Helper()

	tx := &types.Transaction{
		Type:     txType,
		Nonce:    nonce,
		Gas:      100_000,
		Value:    big.NewInt(1),
		GasPrice: big.NewInt(2),
	}

	if !creation {
		to := addr5
		tx.To = &to
	}

	if txType == types.DynamicFeeTx {
		tx.ChainID = big.NewInt(100)
		tx.GasPrice = new(big.Int)
		tx.GasTipCap = big.NewInt(1)
		tx.GasFeeCap = big.NewInt(2)
	}

	signed := owner.signTx(t, tx, signer)
	signed.ComputeHash(0)

	return signed
}

func TestNonceLimit_PoolRejectsMaximum(t *testing.T) {
	const maximum = ^uint64(0)

	for _, txType := range []types.TxType{
		types.LegacyTx, types.DynamicFeeTx,
	} {
		for _, stateNonce := range []uint64{0, maximum - 1, maximum} {
			for _, creation := range []bool{false, true} {
				t.Run(
					fmt.Sprintf("%s/state_%d/creation_%t", txType, stateNonce, creation),
					func(t *testing.T) {
						pool, signer := nonceLimitTestPool(t, stateNonce)
						owner := new(eoa).create(t)
						tx := nonceLimitSignedTransaction(
							t, owner, signer, txType, maximum, creation,
						)

						require.ErrorIs(
							t, pool.addTx(local, tx), state.ErrNonceUintOverflow,
						)
						pool.closeWG.Wait()

						require.Zero(t, pool.gauge.read())
						require.Zero(t, atomic.LoadInt64(&pool.pending))
						require.Zero(t, pool.Length())
						require.Zero(t, atomic.LoadUint64(&pool.accounts.count))
						require.Nil(t, pool.accounts.get(owner.Address))

						_, found := pool.GetPendingTx(tx.Hash)
						require.False(t, found)

						pool.index.RLock()
						indexed := len(pool.index.all)
						senders := len(pool.index.bySender)
						pool.index.RUnlock()
						require.Zero(t, indexed)
						require.Zero(t, senders)

						select {
						case <-pool.promoteReqCh:
							t.Fatal("rejected terminal nonce requested promotion")
						default:
						}
					},
				)
			}
		}
	}
}

func TestNonceLimit_PoolLastUsableNonce(t *testing.T) {
	const maximum = ^uint64(0)

	for _, txType := range []types.TxType{
		types.LegacyTx, types.DynamicFeeTx,
	} {
		for _, creation := range []bool{false, true} {
			t.Run(
				fmt.Sprintf("%s/creation_%t", txType, creation),
				func(t *testing.T) {
					pool, signer := nonceLimitTestPool(t, maximum-1)
					owner := new(eoa).create(t)
					last := nonceLimitSignedTransaction(
						t, owner, signer, txType, maximum-1, creation,
					)

					require.NoError(t, pool.addTx(local, last))
					pool.closeWG.Wait()

					select {
					case request := <-pool.promoteReqCh:
						require.Equal(t, owner.Address, request.account)
						pool.handlePromoteRequest(request)
					default:
						t.Fatal("last usable nonce did not request promotion")
					}

					assertLastTransaction := func() {
						t.Helper()

						a := pool.accounts.get(owner.Address)
						require.NotNil(t, a)

						func() {
							require.True(t, a.lockActive())
							defer a.unlockActive()

							require.Zero(t, a.enqueued.length())
							require.Equal(t, uint64(1), a.promoted.length())
							require.Same(t, last, a.promoted.peek())
							require.Equal(t, maximum, a.getNonce())
							require.Len(t, a.nonceToTx.mapping, 1)
							require.Same(t, last, a.nonceToTx.mapping[maximum-1])
						}()

						pendingNonce, err := pool.GetNonce(owner.Address)
						require.NoError(t, err)
						require.Equal(t, maximum, pendingNonce)

						stored, found := pool.GetPendingTx(last.Hash)
						require.True(t, found)
						require.Same(t, last, stored)
						require.Equal(t, slotsRequired(last), pool.gauge.read())
						require.Equal(t, int64(1), atomic.LoadInt64(&pool.pending))
						require.Equal(t, uint64(1), pool.Length())
						require.Equal(
							t, uint64(1), atomic.LoadUint64(&pool.accounts.count),
						)
					}

					assertLastTransaction()

					terminal := nonceLimitSignedTransaction(
						t, owner, signer, txType, maximum, creation,
					)
					require.ErrorIs(
						t, pool.addTx(local, terminal), state.ErrNonceUintOverflow,
					)
					pool.closeWG.Wait()

					assertLastTransaction()

					_, found := pool.GetPendingTx(terminal.Hash)
					require.False(t, found)

					select {
					case <-pool.promoteReqCh:
						t.Fatal("rejected terminal nonce requested promotion")
					default:
					}
				},
			)
		}
	}
}

/* FINISH ALPHA CHANGE 3897 -- reject terminal transaction nonces without mutating pool ownership -- */
