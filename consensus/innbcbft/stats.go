package innbcbft

import (
	"time"

	"github.com/armon/go-metrics"
	"github.com/prometheus/client_golang/prometheus"
)

/* START ALPHA CHANGE 1542 -- publish MDBX-native consensus state metrics -- */
// startStatsReleasing periodically publishes MDBX consensus state statistics.
func (s *State) startStatsReleasing() {
	const (
		statUpdatePeriod = 10 * time.Second
		dbSubsystem      = "db"
		namespace        = "innbcbft_state"
	)

	publish := func() {
		info, err := s.db.Info(nil)
		if err != nil {
			return
		}

		metrics.SetGauge(
			[]string{prometheus.BuildFQName(namespace, dbSubsystem, "map_size_bytes")},
			float32(info.MapSize),
		)
		metrics.SetGauge(
			[]string{prometheus.BuildFQName(namespace, dbSubsystem, "used_size_bytes")},
			float32(info.LastPNO+1)*float32(info.PageSize),
		)
		metrics.SetGauge(
			[]string{prometheus.BuildFQName(namespace, dbSubsystem, "recent_transaction_id")},
			float32(info.RecentTxnID),
		)
		metrics.SetGauge(
			[]string{prometheus.BuildFQName(namespace, dbSubsystem, "open_readers")},
			float32(info.NumReaders),
		)
		metrics.SetGauge(
			[]string{prometheus.BuildFQName(namespace, dbSubsystem, "unsynced_bytes")},
			float32(info.UnsyncedBytes),
		)
		metrics.SetGauge(
			[]string{prometheus.BuildFQName(namespace, dbSubsystem, "page_operations_new")},
			float32(info.PageOps.Newly),
		)
		metrics.SetGauge(
			[]string{prometheus.BuildFQName(namespace, dbSubsystem, "page_operations_copy_on_write")},
			float32(info.PageOps.Cow),
		)
		metrics.SetGauge(
			[]string{prometheus.BuildFQName(namespace, dbSubsystem, "page_operations_split")},
			float32(info.PageOps.Split),
		)
		metrics.SetGauge(
			[]string{prometheus.BuildFQName(namespace, dbSubsystem, "page_operations_merge")},
			float32(info.PageOps.Merge),
		)
		metrics.SetGauge(
			[]string{prometheus.BuildFQName(namespace, dbSubsystem, "write_operations")},
			float32(info.PageOps.Wops),
		)
	}

	publish()

	ticker := time.NewTicker(statUpdatePeriod)
	defer ticker.Stop()

	// START ALPHA CHANGE 1021 -- exit INNBCBFT stats loop directly on close signal --
	for {
		select {
		case <-s.close:
			return
		case <-ticker.C:
			publish()
		}
	}
	// FINISH ALPHA CHANGE 1021 -- exit INNBCBFT stats loop directly on close signal --
}

/* FINISH ALPHA CHANGE 1542 -- publish MDBX-native consensus state metrics -- */

/* START ALPHA CHANGE 154 -- prune legacy rootchain metrics publisher from sovereign INNBC runtime -- */
/* INNBC Chain keeps local state database metrics only; rootchain bridge metrics are removed. */
/* FINISH ALPHA CHANGE 154 -- prune legacy rootchain metrics publisher from sovereign INNBC runtime -- */
