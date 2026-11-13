package network

import (
	"math"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2524 -- reject invalid limits before network side effects -- */

func TestPeerLimits_NewServerRejectsInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		config    *Config
		wantError string
	}{
		{
			name:      "nil configuration",
			wantError: "network configuration is required",
		},
		{
			name:      "negative outbound",
			config:    &Config{MaxOutboundPeers: -2},
			wantError: "max outbound peers must be non-negative, got -2",
		},
		{
			name:      "unresolved outbound sentinel",
			config:    &Config{MaxOutboundPeers: -1},
			wantError: "max outbound peers must be non-negative, got -1",
		},
		{
			name:      "negative inbound",
			config:    &Config{MaxInboundPeers: -1},
			wantError: "max inbound peers must be non-negative, got -1",
		},
		{
			name:      "negative total",
			config:    &Config{MaxPeers: -1},
			wantError: "max peers must be non-negative, got -1",
		},
		{
			name: "directional sum overflow",
			config: &Config{
				MaxInboundPeers:  math.MaxInt64,
				MaxOutboundPeers: 1,
			},
			wantError: "combined inbound and outbound peer limits exceed int64 capacity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				server, err := NewServer(hclog.NewNullLogger(), tt.config)
				require.Nil(t, server)
				require.EqualError(t, err, "invalid network configuration: "+tt.wantError)
			})
		})
	}
}

func TestPeerLimits_ValidResolvedValues(t *testing.T) {
	t.Parallel()

	for _, cfg := range []*Config{
		DefaultConfig(),
		{},
		{MaxPeers: 32, MaxInboundPeers: 32, MaxOutboundPeers: 0},
		{MaxPeers: 40, MaxInboundPeers: 1, MaxOutboundPeers: 1},
	} {
		require.NoError(t, cfg.ValidatePeerLimits())
	}
}

/* FINISH ALPHA CHANGE 2524 -- reject invalid limits before network side effects -- */

/* START ALPHA CHANGE 2526 -- test network peer-capacity ceiling -- */

func TestPeerLimits_NewServerRejectsExcessiveCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		config    Config
		wantError string
	}{
		{
			name:      "total exceeds ceiling",
			config:    Config{MaxPeers: MaxConfiguredPeers + 1},
			wantError: "max peers must not exceed 1000, got 1001",
		},
		{
			name:      "inbound exceeds ceiling",
			config:    Config{MaxInboundPeers: MaxConfiguredPeers + 1},
			wantError: "max inbound peers must not exceed 1000, got 1001",
		},
		{
			name:      "outbound exceeds ceiling",
			config:    Config{MaxOutboundPeers: MaxConfiguredPeers + 1},
			wantError: "max outbound peers must not exceed 1000, got 1001",
		},
		{
			name: "combined capacity exceeds ceiling",
			config: Config{
				MaxPeers:         MaxConfiguredPeers,
				MaxInboundPeers:  MaxConfiguredPeers,
				MaxOutboundPeers: 1,
			},
			wantError: "combined inbound and outbound peer limits must not exceed 1000, got 1001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				server, err := NewServer(hclog.NewNullLogger(), &tt.config)
				require.Nil(t, server)
				require.EqualError(t, err, "invalid network configuration: "+tt.wantError)
			})
		})
	}
}

func TestPeerLimits_AcceptsCapacityBoundary(t *testing.T) {
	t.Parallel()

	for _, cfg := range []*Config{
		{
			MaxPeers:         MaxConfiguredPeers,
			MaxInboundPeers:  MaxConfiguredPeers,
			MaxOutboundPeers: 0,
		},
		{
			MaxPeers:         MaxConfiguredPeers,
			MaxInboundPeers:  0,
			MaxOutboundPeers: MaxConfiguredPeers,
		},
		{
			MaxPeers:         MaxConfiguredPeers,
			MaxInboundPeers:  MaxConfiguredPeers - 1,
			MaxOutboundPeers: 1,
		},
	} {
		require.NoError(t, cfg.ValidatePeerLimits())
	}
}

/* FINISH ALPHA CHANGE 2526 -- test network peer-capacity ceiling -- */
