package server

import (
	"errors"
	"net"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/command/server/config"

	/* START ALPHA CHANGE 2483 -- normalize log-range settings in runtime configuration -- */
	"github.com/innovativebioresearch/innbc-chain/jsonrpc"
	/* FINISH ALPHA CHANGE 2483 -- normalize log-range settings in runtime configuration -- */
	"github.com/innovativebioresearch/innbc-chain/network"
	/* START ALPHA CHANGE 3220 -- remove remote secrets configuration parameter dependency -- */
	/* FINISH ALPHA CHANGE 3220 -- remove remote secrets configuration parameter dependency -- */
	"github.com/innovativebioresearch/innbc-chain/server"
	"github.com/multiformats/go-multiaddr"
)

const (
	configFlag                   = "config"
	genesisPathFlag              = "chain"
	dataDirFlag                  = "data-dir"
	libp2pAddressFlag            = "libp2p"
	prometheusAddressFlag        = "prometheus"
	natFlag                      = "nat"
	dnsFlag                      = "dns"
	sealFlag                     = "seal"
	maxPeersFlag                 = "max-peers"
	maxInboundPeersFlag          = "max-inbound-peers"
	maxOutboundPeersFlag         = "max-outbound-peers"
	priceLimitFlag               = "price-limit"
	jsonRPCBatchRequestLimitFlag = "json-rpc-batch-request-limit"
	jsonRPCBlockRangeLimitFlag   = "json-rpc-block-range-limit"
	/* START ALPHA CHANGE 2452 -- name all fee-history budget flags -- */
	jsonRPCFeeHistoryBlockLimitFlag      = "json-rpc-fee-history-block-limit"
	jsonRPCFeeHistoryPercentileLimitFlag = "json-rpc-fee-history-percentile-limit"
	jsonRPCFeeHistoryResultLimitFlag     = "json-rpc-fee-history-result-limit"
	/* FINISH ALPHA CHANGE 2452 -- name all fee-history budget flags -- */
	maxSlotsFlag       = "max-slots"
	maxEnqueuedFlag    = "max-enqueued"
	blockGasTargetFlag = "block-gas-target"
	/* START ALPHA CHANGE 3220 -- retire the remote secrets configuration flag -- */
	/* FINISH ALPHA CHANGE 3220 -- retire the remote secrets configuration flag -- */
	/* START ALPHA CHANGE 3266 -- remove retired RLP restore flag names -- */
	/* FINISH ALPHA CHANGE 3266 -- remove retired RLP restore flag names -- */
	/* START ALPHA CHANGE 715 -- remove hidden legacy dev server flags -- */
	/* FINISH ALPHA CHANGE 715 -- remove hidden legacy dev server flags -- */
	corsOriginFlag      = "access-control-allow-origins"
	logFileLocationFlag = "log-to"

	relayerFlag               = "relayer"
	numBlockConfirmationsFlag = "num-block-confirmations"

	concurrentRequestsDebugFlag = "concurrent-requests-debug"
	webSocketReadLimitFlag      = "websocket-read-limit"

	metricsIntervalFlag = "metrics-interval"

	/* START ALPHA CHANGE 833 -- add local history retention CLI flags -- */
	historyModeFlag            = "history-mode"
	historyRetentionBlocksFlag = "history-retention-blocks"
	/* START ALPHA CHANGE 1566 -- expose recent-history segment size -- */
	historySegmentBlocksFlag = "history-segment-blocks"
	/* FINISH ALPHA CHANGE 1566 -- expose recent-history segment size -- */
	/* START ALPHA CHANGE 1721 -- expose only independent snapshot consumer flags -- */
	snapshotSourceDirFlag         = "snapshot-source-dir"
	snapshotBootstrapRequiredFlag = "snapshot-bootstrap-required"
	/* FINISH ALPHA CHANGE 1721 -- expose only independent snapshot consumer flags -- */
	/* START ALPHA CHANGE 1788 -- expose one node-wide MDBX map size flag -- */
	mdbxMapSizeGBFlag = "mdbx-map-size-gb"
	/* FINISH ALPHA CHANGE 1788 -- expose one node-wide MDBX map size flag -- */
	/* FINISH ALPHA CHANGE 833 -- add local history retention CLI flags -- */

	/* START ALPHA CHANGE 1228 -- keep peer directory server CLI flags one-shot only -- */
	peerDirectoryEnabledFlag = "peer-directory-enabled"
	peerDirectoryURLFlag     = "peer-directory-url"
	/* FINISH ALPHA CHANGE 1228 -- keep peer directory server CLI flags one-shot only -- */
)

// Flags that are deprecated, but need to be preserved for
// backwards compatibility with existing scripts
const (
	ibftBaseTimeoutFlagLEGACY = "ibft-base-timeout"
)

const (
	unsetPeersValue = -1
)

var (
	params = &serverParams{
		rawConfig: &config.Config{
			Telemetry: &config.Telemetry{},
			Network:   &config.Network{},
			TxPool:    &config.TxPool{},
			/* START ALPHA CHANGE 834 -- initialize raw history config for CLI binding -- */
			History: &config.History{},
			/* FINISH ALPHA CHANGE 834 -- initialize raw history config for CLI binding -- */
		},
	}
)

var (
	errInvalidNATAddress = errors.New("could not parse NAT IP address")
)

type serverParams struct {
	rawConfig  *config.Config
	configPath string

	libp2pAddress     *net.TCPAddr
	prometheusAddress *net.TCPAddr
	natAddress        net.IP
	dnsAddress        multiaddr.Multiaddr
	grpcAddress       *net.TCPAddr
	jsonRPCAddress    *net.TCPAddr

	blockGasTarget uint64
	/* START ALPHA CHANGE 716 -- remove legacy dev server mode params -- */
	/* FINISH ALPHA CHANGE 716 -- remove legacy dev server mode params -- */

	ibftBaseTimeoutLegacy uint64

	/* START ALPHA CHANGE 3220 -- retain genesis parameters without remote identity configuration -- */
	genesisConfig *chain.Chain
	/* FINISH ALPHA CHANGE 3220 -- retain genesis parameters without remote identity configuration -- */

	logFileLocation string

	relayer bool

	/* START ALPHA CHANGE 1722 -- remove obsolete automatic snapshot flag tracking -- */
	/* FINISH ALPHA CHANGE 1722 -- remove obsolete automatic snapshot flag tracking -- */
}

func (p *serverParams) isMaxPeersSet() bool {
	return p.rawConfig.Network.MaxPeers != unsetPeersValue
}

func (p *serverParams) isPeerRangeSet() bool {
	return p.rawConfig.Network.MaxInboundPeers != unsetPeersValue ||
		p.rawConfig.Network.MaxOutboundPeers != unsetPeersValue
}

/* START ALPHA CHANGE 3220 -- remove the remote secrets configuration path selector -- */
/* FINISH ALPHA CHANGE 3220 -- remove the remote secrets configuration path selector -- */

func (p *serverParams) isPrometheusAddressSet() bool {
	return p.rawConfig.Telemetry.PrometheusAddr != ""
}

func (p *serverParams) isNATAddressSet() bool {
	return p.rawConfig.Network.NatAddr != ""
}

func (p *serverParams) isDNSAddressSet() bool {
	return p.rawConfig.Network.DNSAddr != ""
}

func (p *serverParams) isLogFileLocationSet() bool {
	return p.rawConfig.LogFilePath != ""
}

/* START ALPHA CHANGE 717 -- remove legacy dev consensus detector from server params -- */
/* FINISH ALPHA CHANGE 717 -- remove legacy dev consensus detector from server params -- */

/* START ALPHA CHANGE 3266 -- remove the retired RLP restore path accessor -- */
/* FINISH ALPHA CHANGE 3266 -- remove the retired RLP restore path accessor -- */

func (p *serverParams) setRawGRPCAddress(grpcAddress string) {
	p.rawConfig.GRPCAddr = grpcAddress
}

func (p *serverParams) setRawJSONRPCAddress(jsonRPCAddress string) {
	p.rawConfig.JSONRPCAddr = jsonRPCAddress
}

func (p *serverParams) setJSONLogFormat(jsonLogFormat bool) {
	p.rawConfig.JSONLogFormat = jsonLogFormat
}

func (p *serverParams) generateConfig() *server.Config {
	return &server.Config{
		Chain: p.genesisConfig,
		JSONRPC: &server.JSONRPC{
			JSONRPCAddr:              p.jsonRPCAddress,
			AccessControlAllowOrigin: p.rawConfig.CorsAllowedOrigins,
			BatchLengthLimit:         p.rawConfig.JSONRPCBatchRequestLimit,
			/* START ALPHA CHANGE 2483 -- resolve zero before passing the log-range limit to the server -- */
			BlockRangeLimit: jsonrpc.ResolveBlockRangeLimit(p.rawConfig.JSONRPCBlockRangeLimit),
			/* FINISH ALPHA CHANGE 2483 -- resolve zero before passing the log-range limit to the server -- */
			/* START ALPHA CHANGE 2452 -- pass all fee-history budgets into runtime configuration -- */
			FeeHistoryBlockLimit:      p.rawConfig.JSONRPCFeeHistoryBlockLimit,
			FeeHistoryPercentileLimit: p.rawConfig.JSONRPCFeeHistoryPercentileLimit,
			FeeHistoryResultLimit:     p.rawConfig.JSONRPCFeeHistoryResultLimit,
			/* FINISH ALPHA CHANGE 2452 -- pass all fee-history budgets into runtime configuration -- */
			ConcurrentRequestsDebug: p.rawConfig.ConcurrentRequestsDebug,
			WebSocketReadLimit:      p.rawConfig.WebSocketReadLimit,
			/* START ALPHA CHANGE 2249 -- pass configured RPC admission limits -- */
			Admission: p.rawConfig.JSONRPCAdmission,
			/* FINISH ALPHA CHANGE 2249 -- pass configured RPC admission limits -- */
		},
		GRPCAddr:   p.grpcAddress,
		LibP2PAddr: p.libp2pAddress,
		Telemetry: &server.Telemetry{
			PrometheusAddr: p.prometheusAddress,
		},
		Network: &network.Config{
			NoDiscover:       p.rawConfig.Network.NoDiscover,
			Addr:             p.libp2pAddress,
			NatAddr:          p.natAddress,
			DNS:              p.dnsAddress,
			DataDir:          p.rawConfig.DataDir,
			MaxPeers:         p.rawConfig.Network.MaxPeers,
			MaxInboundPeers:  p.rawConfig.Network.MaxInboundPeers,
			MaxOutboundPeers: p.rawConfig.Network.MaxOutboundPeers,
			Chain:            p.genesisConfig,
			/* START ALPHA CHANGE 1229 -- pass one-shot peer directory config into network runtime -- */
			PeerDirectoryEnabled: p.rawConfig.Network.PeerDirectoryEnabled,
			PeerDirectoryURL:     p.rawConfig.Network.PeerDirectoryURL,
			/* FINISH ALPHA CHANGE 1229 -- pass one-shot peer directory config into network runtime -- */
		},
		DataDir:            p.rawConfig.DataDir,
		Seal:               p.rawConfig.ShouldSeal,
		PriceLimit:         p.rawConfig.TxPool.PriceLimit,
		MaxSlots:           p.rawConfig.TxPool.MaxSlots,
		MaxAccountEnqueued: p.rawConfig.TxPool.MaxAccountEnqueued,
		/* START ALPHA CHANGE 3220 -- server configuration no longer selects an identity backend -- */
		/* FINISH ALPHA CHANGE 3220 -- server configuration no longer selects an identity backend -- */
		/* START ALPHA CHANGE 3266 -- stop wiring retired RLP restore settings into runtime -- */
		/* FINISH ALPHA CHANGE 3266 -- stop wiring retired RLP restore settings into runtime -- */
		LogLevel:      hclog.LevelFromString(p.rawConfig.LogLevel),
		JSONLogFormat: p.rawConfig.JSONLogFormat,
		LogFilePath:   p.logFileLocation,

		Relayer:               p.relayer,
		NumBlockConfirmations: p.rawConfig.NumBlockConfirmations,
		MetricsInterval:       p.rawConfig.MetricsInterval,
		/* START ALPHA CHANGE 835 -- pass local history settings into server runtime config -- */
		History: &server.HistoryConfig{
			Mode:            p.rawConfig.History.Mode,
			RetentionBlocks: p.rawConfig.History.RetentionBlocks,
			/* START ALPHA CHANGE 1567 -- pass segment size into runtime storage config -- */
			SegmentBlocks: p.rawConfig.History.SegmentBlocks,
			/* FINISH ALPHA CHANGE 1567 -- pass segment size into runtime storage config -- */
			/* START ALPHA CHANGE 1723 -- pass independent snapshot consumer settings -- */
			SnapshotSourceDir:         p.rawConfig.History.SnapshotSourceDir,
			SnapshotBootstrapRequired: p.rawConfig.History.SnapshotBootstrapRequired,
			/* FINISH ALPHA CHANGE 1723 -- pass independent snapshot consumer settings -- */
			/* START ALPHA CHANGE 1789 -- pass the node-wide MDBX map size -- */
			MDBXMapSizeGB: p.rawConfig.History.MDBXMapSizeGB,
			/* START ALPHA CHANGE 3460 -- pass resolved page size into server configuration -- */
			MDBXPageSizeBytes: uint64(p.rawConfig.History.MDBXPageSizeBytes),
			/* FINISH ALPHA CHANGE 3460 -- pass resolved page size into server configuration -- */
			/* FINISH ALPHA CHANGE 1789 -- pass the node-wide MDBX map size -- */
		},
		/* FINISH ALPHA CHANGE 835 -- pass local history settings into server runtime config -- */
	}
}
