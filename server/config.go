package server

import (
	"net"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/chain"
	/* START ALPHA CHANGE 2249 -- carry native RPC admission into runtime configuration -- */
	"github.com/innovativebioresearch/innbc-chain/jsonrpc"
	/* FINISH ALPHA CHANGE 2249 -- carry native RPC admission into runtime configuration -- */
	"github.com/innovativebioresearch/innbc-chain/network"
	/* START ALPHA CHANGE 3220 -- remove remote identity configuration from the server API -- */ /* FINISH ALPHA CHANGE 3220 -- remove remote identity configuration from the server API -- */)

const DefaultGRPCPort int = 9632
const DefaultJSONRPCPort int = 8545

// Config is used to parametrize the minimal client
type Config struct {
	Chain *chain.Chain

	JSONRPC    *JSONRPC
	GRPCAddr   *net.TCPAddr
	LibP2PAddr *net.TCPAddr

	PriceLimit         uint64
	MaxAccountEnqueued uint64
	MaxSlots           uint64

	Telemetry *Telemetry
	Network   *network.Config

	DataDir string
	/* START ALPHA CHANGE 3267 -- remove retired RLP restore runtime settings -- */
	/* FINISH ALPHA CHANGE 3267 -- remove retired RLP restore runtime settings -- */

	Seal bool

	/* START ALPHA CHANGE 3220 -- identity storage is local and selected by DataDir -- */
	/* FINISH ALPHA CHANGE 3220 -- identity storage is local and selected by DataDir -- */

	LogLevel hclog.Level

	JSONLogFormat bool

	LogFilePath string

	Relayer bool

	NumBlockConfirmations uint64
	MetricsInterval       time.Duration

	/* START ALPHA CHANGE 836 -- add local history runtime config -- */
	History *HistoryConfig
	/* FINISH ALPHA CHANGE 836 -- add local history runtime config -- */
}

/* START ALPHA CHANGE 837 -- define local history runtime config -- */
type HistoryConfig struct {
	Mode            string
	RetentionBlocks uint64
	/* START ALPHA CHANGE 1561 -- configure recent-history physical segment size -- */
	SegmentBlocks uint64
	/* FINISH ALPHA CHANGE 1561 -- configure recent-history physical segment size -- */
	/* START ALPHA CHANGE 1783 -- use one node-wide MDBX map size -- */
	SnapshotSourceDir         string
	SnapshotBootstrapRequired bool
	MDBXMapSizeGB             uint64
	/* START ALPHA CHANGE 3461 -- carry blockchain and trie page-size policy -- */
	MDBXPageSizeBytes uint64
	/* FINISH ALPHA CHANGE 3461 -- carry blockchain and trie page-size policy -- */
	/* FINISH ALPHA CHANGE 1783 -- use one node-wide MDBX map size -- */
}

/* FINISH ALPHA CHANGE 837 -- define local history runtime config -- */

// Telemetry holds the config details for metric services
type Telemetry struct {
	PrometheusAddr *net.TCPAddr
}

// JSONRPC holds the config details for the JSON-RPC server
type JSONRPC struct {
	JSONRPCAddr              *net.TCPAddr
	AccessControlAllowOrigin []string
	BatchLengthLimit         uint64
	BlockRangeLimit          uint64
	/* START ALPHA CHANGE 2455 -- carry all runtime fee-history budgets -- */
	FeeHistoryBlockLimit      uint64
	FeeHistoryPercentileLimit uint64
	FeeHistoryResultLimit     uint64
	/* FINISH ALPHA CHANGE 2455 -- carry all runtime fee-history budgets -- */
	ConcurrentRequestsDebug uint64
	WebSocketReadLimit      uint64

	/* START ALPHA CHANGE 2249 -- retain RPC admission configuration -- */
	Admission *jsonrpc.AdmissionConfig
	/* FINISH ALPHA CHANGE 2249 -- retain RPC admission configuration -- */
}
