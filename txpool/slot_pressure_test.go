/* START ALPHA CHANGE 3823 -- verify txpool pressure arithmetic admission and pruning -- */

package txpool

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

// Use arbitrary-precision arithmetic as an independent reference.
// This helper is test-only and does not affect runtime processing.
func slotPressureReferenceThreshold(maxSlots uint64) uint64 {
	value := new(big.Int).SetUint64(maxSlots)
	value.Mul(value, big.NewInt(80))
	value.Quo(value, big.NewInt(100))

	return value.Uint64()
}

func slotPressureTestPool(t *testing.T, maxSlots uint64) *TxPool {
	t.Helper()

	pool, err := newTestPoolWithSlots(maxSlots)
	require.NoError(t, err)

	pool.SetSigner(&mockSigner{})

	// Capture synchronous pruning requests without starting background workers.
	pool.pruneCh = make(chan struct{}, 1)

	t.Cleanup(pool.Close)

	return pool
}

func slotPressureTestTx(address types.Address, nonce uint64) *types.Transaction {
	return &types.Transaction{
		From:     address,
		To:       &address,
		Nonce:    nonce,
		Value:    big.NewInt(1),
		GasPrice: new(big.Int).SetUint64(defaultPriceLimit),
		Gas:      validGasLimit,
	}
}

func TestSlotPressure_ThresholdBoundaries(t *testing.T) {
	const maxUint64 = ^uint64(0)

	capacities := []uint64{
		0,
		1,
		2,
		4,
		5,
		6,
		99,
		100,
		101,
		4096,
		maxUint64/80 - 1,
		maxUint64 / 80,
		maxUint64/80 + 1,
		uint64(1) << 62,
		maxUint64,
	}

	for _, maxSlots := range capacities {
		t.Run(fmt.Sprintf("max_%d", maxSlots), func(t *testing.T) {
			threshold := slotPressureReferenceThreshold(maxSlots)

			heights := []uint64{0, threshold, maxSlots}
			if threshold > 0 {
				heights = append(heights, threshold-1)
			}
			if threshold < maxSlots {
				heights = append(heights, threshold+1)
			}

			for _, height := range heights {
				gauge := slotGauge{
					height: height,
					max:    maxSlots,
				}

				require.Equal(
					t,
					height > threshold,
					gauge.highPressure(),
					"max_slots=%d height=%d threshold=%d",
					maxSlots,
					height,
					threshold,
				)
			}
		})
	}
}

func TestSlotPressure_LargeCapacityAdmission(t *testing.T) {
	capacities := []uint64{
		4096,
		uint64(1) << 62,
		^uint64(0),
	}

	for _, maxSlots := range capacities {
		t.Run(fmt.Sprintf("max_%d", maxSlots), func(t *testing.T) {
			pool := slotPressureTestPool(t, maxSlots)

			for _, address := range []types.Address{addr1, addr2} {
				// Both accounts expect nonce zero. These transactions
				// must remain admissible while occupancy is low.
				tx := slotPressureTestTx(address, 1)
				require.Equal(t, uint64(1), slotsRequired(tx))
				require.NoError(t, pool.addTx(local, tx))

				stored, found := pool.index.get(tx.Hash)
				require.True(t, found)
				require.Same(t, tx, stored)

				account := pool.accounts.get(address)
				require.NotNil(t, account)
				require.Zero(t, account.getNonce())

				select {
				case <-pool.pruneCh:
					t.Fatal("low occupancy incorrectly requested pruning")
				default:
				}
			}

			require.Equal(t, uint64(2), pool.gauge.read())
			require.False(t, pool.gauge.highPressure())
		})
	}
}

func TestSlotPressure_AdmissionAndPruningBoundaries(t *testing.T) {
	capacities := []uint64{
		4096,
		uint64(1) << 62,
		^uint64(0),
	}

	for _, maxSlots := range capacities {
		t.Run(fmt.Sprintf("max_%d", maxSlots), func(t *testing.T) {
			pool := slotPressureTestPool(t, maxSlots)
			threshold := slotPressureReferenceThreshold(maxSlots)
			require.Greater(t, threshold, uint64(0))

			first := slotPressureTestTx(addr1, 1)
			require.NoError(t, pool.addTx(local, first))
			require.Equal(t, uint64(1), pool.gauge.read())

			// Model other occupied slots using only the gauge.
			// No capacity-sized transaction collection is allocated.
			pool.gauge.increase(threshold - 1)
			require.Equal(t, threshold, pool.gauge.read())
			require.False(t, pool.gauge.highPressure())

			// Admission checks pressure before adding this transaction.
			// Occupancy exactly at the threshold must permit it.
			second := slotPressureTestTx(addr2, 1)
			require.NoError(t, pool.addTx(local, second))
			require.Equal(t, threshold+1, pool.gauge.read())
			require.True(t, pool.gauge.highPressure())

			select {
			case <-pool.pruneCh:
				t.Fatal("occupancy exactly at the threshold requested pruning")
			default:
			}

			// Above the threshold, a future-nonce transaction must
			// still be rejected and must request pruning.
			rejected := slotPressureTestTx(addr3, 1)
			require.ErrorIs(t, pool.addTx(local, rejected), ErrRejectFutureTx)
			require.Equal(t, threshold+1, pool.gauge.read())

			_, found := pool.index.get(rejected.Hash)
			require.False(t, found)
			require.Nil(t, pool.accounts.get(addr3))

			select {
			case <-pool.pruneCh:
			default:
				t.Fatal("high pressure did not request pruning")
			}

			// The expected nonce remains admissible under pressure.
			ready := slotPressureTestTx(addr3, 0)
			require.NoError(t, pool.addTx(local, ready))
			require.Equal(t, threshold+2, pool.gauge.read())

			select {
			case <-pool.pruneCh:
			default:
				t.Fatal("expected-nonce admission did not request pruning")
			}

			// Execute the existing pruning handler directly.
			// It must remove the two nonce-gap queues while retaining
			// the transaction whose nonce is immediately executable.
			pool.pruneAccountsWithNonceHoles()

			require.Equal(t, threshold, pool.gauge.read())
			require.False(t, pool.gauge.highPressure())

			for _, tx := range []*types.Transaction{first, second} {
				_, found := pool.index.get(tx.Hash)
				require.False(t, found)
				require.Nil(t, pool.accounts.get(tx.From))
			}

			stored, found := pool.index.get(ready.Hash)
			require.True(t, found)
			require.Same(t, ready, stored)
			require.NotNil(t, pool.accounts.get(ready.From))
		})
	}
}

/* FINISH ALPHA CHANGE 3823 -- verify txpool pressure arithmetic admission and pruning -- */
