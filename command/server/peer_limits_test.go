package server

import (
	"math"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/innovativebioresearch/innbc-chain/network"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2523 -- cover peer-limit rejection and default resolution -- */

func TestPeerLimits_Resolve(t *testing.T) {
	t.Parallel()

	defaults := network.DefaultConfig()

	tests := []struct {
		name                                 string
		total, inbound, outbound             int64
		wantTotal, wantInbound, wantOutbound int64
	}{
		{
			name:  "unset uses defaults",
			total: -1, inbound: -1, outbound: -1,
			wantTotal:    defaults.MaxPeers,
			wantInbound:  defaults.MaxInboundPeers,
			wantOutbound: defaults.MaxOutboundPeers,
		},
		{
			name:  "outbound override",
			total: -1, inbound: -1, outbound: 2,
			wantTotal:    defaults.MaxInboundPeers + 2,
			wantInbound:  defaults.MaxInboundPeers,
			wantOutbound: 2,
		},
		{
			name:  "inbound override",
			total: -1, inbound: 3, outbound: -1,
			wantTotal:    3 + defaults.MaxOutboundPeers,
			wantInbound:  3,
			wantOutbound: defaults.MaxOutboundPeers,
		},
		{
			name:  "both directions",
			total: -1, inbound: 3, outbound: 2,
			wantTotal: 5, wantInbound: 3, wantOutbound: 2,
		},
		{
			name:  "zero outbound remains valid",
			total: -1, inbound: -1, outbound: 0,
			wantTotal:    defaults.MaxInboundPeers,
			wantInbound:  defaults.MaxInboundPeers,
			wantOutbound: 0,
		},
		{
			name:  "zero total resolves both directions",
			total: 0, inbound: -1, outbound: -1,
			wantTotal: 0, wantInbound: 0, wantOutbound: 0,
		},
		{
			name:  "total preserves existing ratio",
			total: 10, inbound: -1, outbound: -1,
			wantTotal: 10, wantInbound: 8, wantOutbound: 2,
		},
		{
			name:  "directional configuration retains precedence",
			total: 100, inbound: 3, outbound: 2,
			wantTotal: 5, wantInbound: 3, wantOutbound: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &serverParams{
				rawConfig: &config.Config{
					Network: &config.Network{
						MaxPeers:         tt.total,
						MaxInboundPeers:  tt.inbound,
						MaxOutboundPeers: tt.outbound,
					},
				},
			}

			require.NoError(t, p.initPeerLimits())
			require.Equal(t, tt.wantTotal, p.rawConfig.Network.MaxPeers)
			require.Equal(t, tt.wantInbound, p.rawConfig.Network.MaxInboundPeers)
			require.Equal(t, tt.wantOutbound, p.rawConfig.Network.MaxOutboundPeers)
		})
	}
}

func TestPeerLimits_RejectInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		total, inbound, outbound int64
		wantError                string
	}{
		{
			name:  "negative outbound",
			total: -1, inbound: -1, outbound: -2,
			wantError: "--max-outbound-peers must be -1 (unset) or non-negative, got -2",
		},
		{
			name:  "negative inbound",
			total: -1, inbound: -2, outbound: -1,
			wantError: "--max-inbound-peers must be -1 (unset) or non-negative, got -2",
		},
		{
			name:  "negative total",
			total: -2, inbound: -1, outbound: -1,
			wantError: "--max-peers must be -1 (unset) or non-negative, got -2",
		},
		{
			name:  "directional sum overflow",
			total: -1, inbound: math.MaxInt64, outbound: 1,
			wantError: "combined inbound and outbound peer limits exceed int64 capacity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &serverParams{
				rawConfig: &config.Config{
					Network: &config.Network{
						MaxPeers:         tt.total,
						MaxInboundPeers:  tt.inbound,
						MaxOutboundPeers: tt.outbound,
					},
				},
			}

			before := *p.rawConfig.Network
			require.EqualError(t, p.initPeerLimits(), tt.wantError)
			require.Equal(t, before, *p.rawConfig.Network)
		})
	}
}

/* FINISH ALPHA CHANGE 2523 -- cover peer-limit rejection and default resolution -- */

/* START ALPHA CHANGE 2527 -- test peer ceiling after CLI default resolution -- */

func TestPeerLimits_ConfiguredCapacityCeiling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		total, inbound, outbound int64
		wantError                string
	}{
		{
			name:  "total at ceiling",
			total: network.MaxConfiguredPeers, inbound: -1, outbound: -1,
		},
		{
			name:  "inbound at ceiling with outbound disabled",
			total: -1, inbound: network.MaxConfiguredPeers, outbound: 0,
		},
		{
			name:  "outbound at ceiling with inbound disabled",
			total: -1, inbound: 0, outbound: network.MaxConfiguredPeers,
		},
		{
			name:  "total above ceiling",
			total: network.MaxConfiguredPeers + 1, inbound: -1, outbound: -1,
			wantError: "max peers must not exceed 1000, got 1001",
		},
		{
			name:  "inbound above ceiling",
			total: -1, inbound: network.MaxConfiguredPeers + 1, outbound: 0,
			wantError: "max peers must not exceed 1000, got 1001",
		},
		{
			name:  "outbound above ceiling",
			total: -1, inbound: 0, outbound: network.MaxConfiguredPeers + 1,
			wantError: "max peers must not exceed 1000, got 1001",
		},
		{
			name:  "combined directions above ceiling",
			total: -1, inbound: network.MaxConfiguredPeers, outbound: 1,
			wantError: "max peers must not exceed 1000, got 1001",
		},
		{
			name:  "default inbound contributes to total",
			total: -1, inbound: -1, outbound: network.MaxConfiguredPeers,
			wantError: "max peers must not exceed 1000, got 1032",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &serverParams{
				rawConfig: &config.Config{
					Network: &config.Network{
						MaxPeers:         tt.total,
						MaxInboundPeers:  tt.inbound,
						MaxOutboundPeers: tt.outbound,
					},
				},
			}

			before := *p.rawConfig.Network
			err := p.initPeerLimits()

			if tt.wantError != "" {
				require.EqualError(t, err, tt.wantError)
				require.Equal(t, before, *p.rawConfig.Network)
				return
			}

			require.NoError(t, err)
			require.Equal(t, network.MaxConfiguredPeers, p.rawConfig.Network.MaxPeers)
			require.Equal(
				t,
				network.MaxConfiguredPeers,
				p.rawConfig.Network.MaxInboundPeers+p.rawConfig.Network.MaxOutboundPeers,
			)
		})
	}
}

/* FINISH ALPHA CHANGE 2527 -- test peer ceiling after CLI default resolution -- */
