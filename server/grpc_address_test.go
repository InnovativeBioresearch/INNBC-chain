package server

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2563 -- test loopback operator address enforcement -- */

type operatorGRPCAddressCase struct {
	name    string
	address *net.TCPAddr
	allowed bool
}

func operatorGRPCAddressCases() []operatorGRPCAddressCase {
	cases := []operatorGRPCAddressCase{
		{name: "missing address"},
		{
			name:    "missing resolved IP",
			address: &net.TCPAddr{Port: 9632},
		},
	}

	for _, candidate := range []struct {
		name    string
		ip      string
		allowed bool
	}{
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv4 loopback range", "127.0.0.2", true},
		{"IPv6 loopback", "::1", true},
		{"mapped IPv4 loopback", "::ffff:127.0.0.1", true},
		{"IPv4 wildcard", "0.0.0.0", false},
		{"IPv6 wildcard", "::", false},
		{"mapped IPv4 wildcard", "::ffff:0.0.0.0", false},
		{"IPv4 LAN", "192.168.1.10", false},
		{"IPv4 private", "10.0.0.10", false},
		{"IPv4 public", "8.8.8.8", false},
		{"IPv6 local", "fd00::1", false},
		{"IPv6 link local", "fe80::1", false},
		{"IPv6 public", "2001:4860:4860::8888", false},
		{"mapped non-loopback", "::ffff:192.168.1.10", false},
	} {
		cases = append(cases, operatorGRPCAddressCase{
			name: candidate.name,
			address: &net.TCPAddr{
				IP:   net.ParseIP(candidate.ip),
				Port: 9632,
			},
			allowed: candidate.allowed,
		})
	}

	return cases
}

func TestOperatorGRPCAddress_Validation(t *testing.T) {
	for _, tc := range operatorGRPCAddressCases() {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOperatorGRPCAddress(tc.address)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "loopback")
			}
		})
	}
}

func TestOperatorGRPCAddress_NewServerValidatesBeforeStartup(t *testing.T) {
	for _, tc := range operatorGRPCAddressCases() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			logPath := filepath.Join(root, "node.log")
			dataPath := filepath.Join(root, "node")

			instance, err := NewServer(&Config{
				Chain: &chain.Chain{
					Params: &chain.Params{
						/* START ALPHA CHANGE 3544 -- reach operator-address validation with a valid chain ID -- */
						ChainID: 1982,
						/* FINISH ALPHA CHANGE 3544 -- reach operator-address validation with a valid chain ID -- */
						Forks: &chain.Forks{
							chain.London:    chain.NewFork(0),
							chain.LondonFix: chain.NewFork(0),
						},
					},
				},
				GRPCAddr:    tc.address,
				LogFilePath: logPath,
				DataDir:     dataPath,
			})

			require.Nil(t, instance)

			if tc.allowed {
				/* Stop at the next validation boundary without starting a node. */
				require.ErrorContains(t, err, "JSON-RPC configuration is missing")
			} else {
				require.ErrorContains(t, err, "loopback")
			}

			_, logErr := os.Stat(logPath)
			require.True(t, os.IsNotExist(logErr))

			_, dataErr := os.Stat(dataPath)
			require.True(t, os.IsNotExist(dataErr))
		})
	}
}

func TestOperatorGRPCAddress_ListenerBoundaryRejectsNonLoopback(t *testing.T) {
	for _, tc := range operatorGRPCAddressCases() {
		if tc.allowed {
			continue
		}

		t.Run(tc.name, func(t *testing.T) {
			instance := &Server{
				config: &Config{GRPCAddr: tc.address},
			}

			/* A missing grpcServer proves rejection precedes service registration. */
			require.NotPanics(t, func() {
				require.ErrorContains(t, instance.setupGRPC(), "loopback")
			})
		})
	}
}

/* FINISH ALPHA CHANGE 2563 -- test loopback operator address enforcement -- */
