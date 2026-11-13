package txpool

import (
	"sync/atomic"

	"github.com/armon/go-metrics"

	"github.com/innovativebioresearch/innbc-chain/types"
)

const (
	highPressureMark = 80 // 80%
)

// Gauge for measuring pool capacity in slots
type slotGauge struct {
	height uint64 // amount of slots currently occupying the pool
	max    uint64 // max limit
}

// read returns the current height of the gauge.
func (g *slotGauge) read() uint64 {
	return atomic.LoadUint64(&g.height)
}

// increase increases the height of the gauge by the specified slots amount.
func (g *slotGauge) increase(slots uint64) {
	newHeight := atomic.AddUint64(&g.height, slots)
	metrics.SetGauge([]string{txPoolMetrics, "slots_used"}, float32(newHeight))
}

// increaseWithinLimit increases the height of the gauge by the specified slots amount only if the increased height is
// less than max. Returns true if the height is increased.
func (g *slotGauge) increaseWithinLimit(slots uint64) (updated bool) {
	for {
		old := g.read()
		newHeight := old + slots

		if newHeight > g.max {
			return false
		}

		if atomic.CompareAndSwapUint64(&g.height, old, newHeight) {
			metrics.SetGauge([]string{txPoolMetrics, "slots_used"}, float32(newHeight))

			return true
		}
	}
}

// decrease decreases the height of the gauge by the specified slots amount.
func (g *slotGauge) decrease(slots uint64) {
	newHeight := atomic.AddUint64(&g.height, ^(slots - 1))
	metrics.SetGauge([]string{txPoolMetrics, "slots_used"}, float32(newHeight))
}

/* START ALPHA CHANGE 3822 -- calculate txpool pressure without unsigned overflow -- */

// highPressure checks whether occupied slots exceed 80% of capacity.
func (g *slotGauge) highPressure() bool {
	// Preserve floor(highPressureMark * max / 100) without overflowing.
	threshold := (g.max/100)*highPressureMark +
		((g.max%100)*highPressureMark)/100

	return g.read() > threshold
}

/* FINISH ALPHA CHANGE 3822 -- calculate txpool pressure without unsigned overflow -- */

// START ALPHA CHANGE 1023 -- remove unused txpool slot gauge free-slots helper --
// FINISH ALPHA CHANGE 1023 -- remove unused txpool slot gauge free-slots helper --

// slotsRequired calculates the number of slots required for given transaction(s).
func slotsRequired(txs ...*types.Transaction) uint64 {
	slots := uint64(0)
	for _, tx := range txs {
		slots += (tx.Size() + txSlotSize - 1) / txSlotSize
	}

	return slots
}
