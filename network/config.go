package network

import (
	"net"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/secrets"
	"github.com/multiformats/go-multiaddr"
)

// Config details the params for the base networking server
type Config struct {
	NoDiscover       bool                   // flag indicating if the discovery mechanism should be turned on
	Addr             *net.TCPAddr           // the base address
	NatAddr          net.IP                 // the NAT address
	DNS              multiaddr.Multiaddr    // the DNS address
	DataDir          string                 // the base data directory for the client
	MaxPeers         int64                  // the maximum number of peer connections
	MaxInboundPeers  int64                  // the maximum number of inbound peer connections
	MaxOutboundPeers int64                  // the maximum number of outbound peer connections
	Chain            *chain.Chain           // the reference to the chain configuration
	SecretsManager   secrets.SecretsManager // secrets manager used for key storage

	/* START ALPHA CHANGE 1226 -- keep peer directory runtime config one-shot only -- */
	PeerDirectoryEnabled bool
	PeerDirectoryURL     string
	/* FINISH ALPHA CHANGE 1226 -- keep peer directory runtime config one-shot only -- */
}

func DefaultConfig() *Config {
	return &Config{
		// The discovery service is turned on by default
		NoDiscover: false,
		// Addresses are bound to localhost by default
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: DefaultLibp2pPort,
		},
		// The default ratio for outbound / max peer connections is 0.20
		MaxPeers: 40,
		// The default ratio for outbound / inbound connections is 0.25
		MaxInboundPeers:  32,
		MaxOutboundPeers: 8,

		/* START ALPHA CHANGE 1227 -- default to official INNBC peer directory one-shot refresh -- */
		PeerDirectoryEnabled: true,
		PeerDirectoryURL:     DefaultPeerDirectoryURL,
		/* FINISH ALPHA CHANGE 1227 -- default to official INNBC peer directory one-shot refresh -- */
	}
}
