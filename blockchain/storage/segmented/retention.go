/* START ALPHA CHANGE 3138 -- validate retained segment counts before signed conversion -- */
package segmented

import (
	"errors"
	"fmt"
	"math"
)

// RetainedSegmentCount validates recent-history geometry and returns a
// positive segment count representable by the platform's int type.
func RetainedSegmentCount(segmentBlocks, retentionBlocks uint64) (int, error) {
	if segmentBlocks == 0 {
		return 0, errors.New("history segment blocks must be greater than zero")
	}

	if retentionBlocks < segmentBlocks {
		return 0, errors.New("history retention blocks must be at least one segment")
	}

	if retentionBlocks%segmentBlocks != 0 {
		return 0, errors.New("history retention blocks must be divisible by history segment blocks")
	}

	count := retentionBlocks / segmentBlocks
	if count > uint64(math.MaxInt) {
		return 0, fmt.Errorf(
			"history retained segment count %d exceeds platform maximum %d",
			count,
			math.MaxInt,
		)
	}

	return int(count), nil
}

/* FINISH ALPHA CHANGE 3138 -- validate retained segment counts before signed conversion -- */
