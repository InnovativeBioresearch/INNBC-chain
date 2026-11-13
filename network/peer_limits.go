package network

import (
	"fmt"
	"math"
)

/* START ALPHA CHANGE 2525 -- bound configured peer capacity --

MaxConfiguredPeers limits each node's resolved connection configuration.
It is not a limit on the chain's validator count or a tested capacity claim.

FINISH ALPHA CHANGE 2525 -- bound configured peer capacity -- */

const MaxConfiguredPeers int64 = 1000

/* START ALPHA CHANGE 2521 -- validate resolved network peer limits --

The CLI unset sentinel must be resolved before constructing the network.
Zero remains valid. This validates numeric bounds without requiring the
total limit to equal the sum of the directional limits.

FINISH ALPHA CHANGE 2521 -- validate resolved network peer limits -- */

func (c *Config) ValidatePeerLimits() error {
	if c == nil {
		return fmt.Errorf("network configuration is required")
	}

	limits := []struct {
		name  string
		value int64
	}{
		{"max peers", c.MaxPeers},
		{"max inbound peers", c.MaxInboundPeers},
		{"max outbound peers", c.MaxOutboundPeers},
	}

	for _, limit := range limits {
		if limit.value < 0 {
			return fmt.Errorf("%s must be non-negative, got %d", limit.name, limit.value)
		}
	}

	if c.MaxInboundPeers > math.MaxInt64-c.MaxOutboundPeers {
		return fmt.Errorf("combined inbound and outbound peer limits exceed int64 capacity")
	}

	/* START ALPHA CHANGE 2525 -- reject excessive resolved connection limits -- */
	for _, limit := range limits {
		if limit.value > MaxConfiguredPeers {
			return fmt.Errorf(
				"%s must not exceed %d, got %d",
				limit.name,
				MaxConfiguredPeers,
				limit.value,
			)
		}
	}

	combined := c.MaxInboundPeers + c.MaxOutboundPeers
	if combined > MaxConfiguredPeers {
		return fmt.Errorf(
			"combined inbound and outbound peer limits must not exceed %d, got %d",
			MaxConfiguredPeers,
			combined,
		)
	}
	/* FINISH ALPHA CHANGE 2525 -- reject excessive resolved connection limits -- */

	return nil
}
