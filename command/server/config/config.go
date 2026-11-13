package config

import (
	"encoding/json"
	"fmt"
	"os"

	/* START ALPHA CHANGE 3267 -- remove restore-only numeric parsing -- */
	/* FINISH ALPHA CHANGE 3267 -- remove restore-only numeric parsing -- */
	"strings"
	"time"

	"github.com/hashicorp/hcl"
	/* START ALPHA CHANGE 3267 -- remove the retired archive configuration dependency -- */
	/* FINISH ALPHA CHANGE 3267 -- remove the retired archive configuration dependency -- */
	/* START ALPHA CHANGE 2451 -- share fee-history defaults with execution-independent RPC helpers -- */
	"github.com/innovativebioresearch/innbc-chain/gasprice"
	/* FINISH ALPHA CHANGE 2451 -- share fee-history defaults with execution-independent RPC helpers -- */
	/* START ALPHA CHANGE 2249 -- configure native RPC admission -- */
	"github.com/innovativebioresearch/innbc-chain/jsonrpc"
	/* FINISH ALPHA CHANGE 2249 -- configure native RPC admission -- */
	/* START ALPHA CHANGE 3457 -- decode page-size configuration consistently -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3457 -- decode page-size configuration consistently -- */
	"github.com/innovativebioresearch/innbc-chain/network"
	"gopkg.in/yaml.v3"
)

/* START ALPHA CHANGE 3457 -- expose the shared page-size flag value to command packages -- */
type MDBXPageSize = mdbxconfig.PageSize

/* FINISH ALPHA CHANGE 3457 -- expose the shared page-size flag value to command packages -- */

// Config defines the server configuration params
type Config struct {
	GenesisPath string `json:"chain_config" yaml:"chain_config"`
	/* START ALPHA CHANGE 3220 -- remove the unused remote secrets configuration field -- */
	/* FINISH ALPHA CHANGE 3220 -- remove the unused remote secrets configuration field -- */
	DataDir        string     `json:"data_dir" yaml:"data_dir"`
	BlockGasTarget string     `json:"block_gas_target" yaml:"block_gas_target"`
	GRPCAddr       string     `json:"grpc_addr" yaml:"grpc_addr"`
	JSONRPCAddr    string     `json:"jsonrpc_addr" yaml:"jsonrpc_addr"`
	Telemetry      *Telemetry `json:"telemetry" yaml:"telemetry"`
	Network        *Network   `json:"network" yaml:"network"`
	ShouldSeal     bool       `json:"seal" yaml:"seal"`
	TxPool         *TxPool    `json:"tx_pool" yaml:"tx_pool"`
	LogLevel       string     `json:"log_level" yaml:"log_level"`
	/* START ALPHA CHANGE 3267 -- remove retired RLP restore configuration fields -- */
	/* FINISH ALPHA CHANGE 3267 -- remove retired RLP restore configuration fields -- */
	Headers                  *Headers `json:"headers" yaml:"headers"`
	LogFilePath              string   `json:"log_to" yaml:"log_to"`
	JSONRPCBatchRequestLimit uint64   `json:"json_rpc_batch_request_limit" yaml:"json_rpc_batch_request_limit"`
	JSONRPCBlockRangeLimit   uint64   `json:"json_rpc_block_range_limit" yaml:"json_rpc_block_range_limit"`
	/* START ALPHA CHANGE 2451 -- expose all fee-history budgets in server configuration -- */
	JSONRPCFeeHistoryBlockLimit      uint64 `json:"json_rpc_fee_history_block_limit" yaml:"json_rpc_fee_history_block_limit"`
	JSONRPCFeeHistoryPercentileLimit uint64 `json:"json_rpc_fee_history_percentile_limit" yaml:"json_rpc_fee_history_percentile_limit"`
	JSONRPCFeeHistoryResultLimit     uint64 `json:"json_rpc_fee_history_result_limit" yaml:"json_rpc_fee_history_result_limit"`
	/* FINISH ALPHA CHANGE 2451 -- expose all fee-history budgets in server configuration -- */
	JSONLogFormat      bool     `json:"json_log_format" yaml:"json_log_format"`
	CorsAllowedOrigins []string `json:"cors_allowed_origins" yaml:"cors_allowed_origins"`

	Relayer               bool   `json:"relayer" yaml:"relayer"`
	NumBlockConfirmations uint64 `json:"num_block_confirmations" yaml:"num_block_confirmations"`

	ConcurrentRequestsDebug uint64 `json:"concurrent_requests_debug" yaml:"concurrent_requests_debug"`
	WebSocketReadLimit      uint64 `json:"web_socket_read_limit" yaml:"web_socket_read_limit"`

	/* START ALPHA CHANGE 2249 -- expose structured RPC admission configuration -- */
	JSONRPCAdmission *jsonrpc.AdmissionConfig `json:"json_rpc_admission" yaml:"json_rpc_admission"`
	/* FINISH ALPHA CHANGE 2249 -- expose structured RPC admission configuration -- */

	MetricsInterval time.Duration `json:"metrics_interval" yaml:"metrics_interval"`

	/* START ALPHA CHANGE 829 -- add local history retention config section -- */
	History *History `json:"history" yaml:"history"`
	/* FINISH ALPHA CHANGE 829 -- add local history retention config section -- */
}

/* START ALPHA CHANGE 830 -- define local history retention config -- */
// History defines local node history and snapshot settings.
// This is local runtime policy, not genesis consensus data.
type History struct {
	Mode            string `json:"mode" yaml:"mode"`
	RetentionBlocks uint64 `json:"retention_blocks" yaml:"retention_blocks"`
	/* START ALPHA CHANGE 1562 -- configure recent-history physical segment size -- */
	SegmentBlocks uint64 `json:"segment_blocks" yaml:"segment_blocks"`
	/* FINISH ALPHA CHANGE 1562 -- configure recent-history physical segment size -- */
	/* START ALPHA CHANGE 1786 -- use one MDBX map size in node configuration -- */
	SnapshotSourceDir         string `json:"snapshot_source_dir" yaml:"snapshot_source_dir"`
	SnapshotBootstrapRequired bool   `json:"snapshot_bootstrap_required" yaml:"snapshot_bootstrap_required"`
	MDBXMapSizeGB             uint64 `json:"mdbx_map_size_gb" yaml:"mdbx_map_size_gb"`
	/* START ALPHA CHANGE 3457 -- configure blockchain and trie page size -- */
	/* START ALPHA CHANGE 3497 -- decode HCL page settings through the shared parser -- */
	MDBXPageSizeBytes MDBXPageSize `json:"mdbx_page_size_bytes" yaml:"mdbx_page_size_bytes" hcl:"-"`
	/* FINISH ALPHA CHANGE 3497 -- decode HCL page settings through the shared parser -- */
	/* FINISH ALPHA CHANGE 3457 -- configure blockchain and trie page size -- */
	/* FINISH ALPHA CHANGE 1786 -- use one MDBX map size in node configuration -- */
}

/* FINISH ALPHA CHANGE 830 -- define local history retention config -- */

// Telemetry holds the config details for metric services.
type Telemetry struct {
	PrometheusAddr string `json:"prometheus_addr" yaml:"prometheus_addr"`
}

// Network defines the network configuration params
type Network struct {
	NoDiscover       bool   `json:"no_discover" yaml:"no_discover"`
	Libp2pAddr       string `json:"libp2p_addr" yaml:"libp2p_addr"`
	NatAddr          string `json:"nat_addr" yaml:"nat_addr"`
	DNSAddr          string `json:"dns_addr" yaml:"dns_addr"`
	MaxPeers         int64  `json:"max_peers,omitempty" yaml:"max_peers,omitempty"`
	MaxOutboundPeers int64  `json:"max_outbound_peers,omitempty" yaml:"max_outbound_peers,omitempty"`
	MaxInboundPeers  int64  `json:"max_inbound_peers,omitempty" yaml:"max_inbound_peers,omitempty"`

	/* START ALPHA CHANGE 1230 -- expose one-shot peer directory config in server config file -- */
	PeerDirectoryEnabled bool   `json:"peer_directory_enabled" yaml:"peer_directory_enabled"`
	PeerDirectoryURL     string `json:"peer_directory_url" yaml:"peer_directory_url"`
	/* FINISH ALPHA CHANGE 1230 -- expose one-shot peer directory config in server config file -- */
}

// TxPool defines the TxPool configuration params
type TxPool struct {
	PriceLimit         uint64 `json:"price_limit" yaml:"price_limit"`
	MaxSlots           uint64 `json:"max_slots" yaml:"max_slots"`
	MaxAccountEnqueued uint64 `json:"max_account_enqueued" yaml:"max_account_enqueued"`
}

// Headers defines the HTTP response headers required to enable CORS.
type Headers struct {
	AccessControlAllowOrigins []string `json:"access_control_allow_origins" yaml:"access_control_allow_origins"`
}

const (
	// BlockTimeMultiplierForTimeout Multiplier to get IBFT timeout from block time
	// timeout is calculated when IBFT timeout is not specified
	BlockTimeMultiplierForTimeout uint64 = 5

	// DefaultJSONRPCBatchRequestLimit maximum length allowed for json_rpc batch requests
	DefaultJSONRPCBatchRequestLimit uint64 = 20

	/* START ALPHA CHANGE 2482 -- share the default historical log-range limit --

	DefaultJSONRPCBlockRangeLimit bounds toBlock - fromBlock for historical
	log queries. A configured zero selects this default.

	FINISH ALPHA CHANGE 2482 -- share the default historical log-range limit -- */
	DefaultJSONRPCBlockRangeLimit uint64 = jsonrpc.DefaultBlockRangeLimit
	/* START ALPHA CHANGE 2451 -- use shared defaults for fee-history configuration -- */
	DefaultJSONRPCFeeHistoryBlockLimit      uint64 = gasprice.DefaultFeeHistoryBlockLimit
	DefaultJSONRPCFeeHistoryPercentileLimit uint64 = gasprice.DefaultFeeHistoryPercentileLimit
	DefaultJSONRPCFeeHistoryResultLimit     uint64 = gasprice.DefaultFeeHistoryResultLimit
	/* FINISH ALPHA CHANGE 2451 -- use shared defaults for fee-history configuration -- */

	// DefaultNumBlockConfirmations minimal number of child blocks required for the parent block to be considered final
	// on ethereum epoch lasts for 32 blocks. more details: https://www.alchemy.com/overviews/ethereum-commitment-levels
	DefaultNumBlockConfirmations uint64 = 64

	// DefaultConcurrentRequestsDebug specifies max number of allowed concurrent requests for debug endpoints
	DefaultConcurrentRequestsDebug uint64 = 32

	// DefaultWebSocketReadLimit specifies max size in bytes for a message read from the peer by Gorrila websocket lib.
	// If a message exceeds the limit,
	// the connection sends a close message to the peer and returns ErrReadLimit to the application.
	DefaultWebSocketReadLimit uint64 = 8192

	// DefaultMetricsInterval specifies the time interval after which Prometheus metrics will be generated.
	// A value of 0 means the metrics are disabled.
	DefaultMetricsInterval time.Duration = time.Second * 8

	/* START ALPHA CHANGE 831 -- add default history retention constants -- */
	DefaultHistoryMode            = "full"
	DefaultHistoryRetentionBlocks = uint64(43200)
	/* START ALPHA CHANGE 1563 -- default recent history to hourly physical segments -- */
	DefaultHistorySegmentBlocks = uint64(1800)
	/* FINISH ALPHA CHANGE 1563 -- default recent history to hourly physical segments -- */
	/* START ALPHA CHANGE 1719 -- remove obsolete automatic snapshot interval default -- */
	/* FINISH ALPHA CHANGE 1719 -- remove obsolete automatic snapshot interval default -- */
	/* START ALPHA CHANGE 3057 -- default full-node MDBX storage to one TiB per database -- */
	DefaultMDBXMapSizeGB = uint64(1024)
	/* FINISH ALPHA CHANGE 3057 -- default full-node MDBX storage to one TiB per database -- */
	/* FINISH ALPHA CHANGE 831 -- add default history retention constants -- */
)

// DefaultConfig returns the default server configuration
func DefaultConfig() *Config {
	defaultNetworkConfig := network.DefaultConfig()

	return &Config{
		GenesisPath:    "./genesis.json",
		DataDir:        "",
		BlockGasTarget: "0x0", // Special value signaling the parent gas limit should be applied
		Network: &Network{
			NoDiscover:       defaultNetworkConfig.NoDiscover,
			MaxPeers:         defaultNetworkConfig.MaxPeers,
			MaxOutboundPeers: defaultNetworkConfig.MaxOutboundPeers,
			MaxInboundPeers:  defaultNetworkConfig.MaxInboundPeers,
			Libp2pAddr: fmt.Sprintf("%s:%d",
				defaultNetworkConfig.Addr.IP,
				defaultNetworkConfig.Addr.Port,
			),
			/* START ALPHA CHANGE 1231 -- default server config to one-shot official peer directory -- */
			PeerDirectoryEnabled: defaultNetworkConfig.PeerDirectoryEnabled,
			PeerDirectoryURL:     defaultNetworkConfig.PeerDirectoryURL,
			/* FINISH ALPHA CHANGE 1231 -- default server config to one-shot official peer directory -- */
		},
		Telemetry:  &Telemetry{},
		ShouldSeal: true,
		TxPool: &TxPool{
			PriceLimit:         0,
			MaxSlots:           4096,
			MaxAccountEnqueued: 128,
		},
		LogLevel: "INFO",
		/* START ALPHA CHANGE 3267 -- remove retired RLP restore defaults -- */
		/* FINISH ALPHA CHANGE 3267 -- remove retired RLP restore defaults -- */
		Headers: &Headers{
			AccessControlAllowOrigins: []string{"*"},
		},
		LogFilePath:              "",
		JSONRPCBatchRequestLimit: DefaultJSONRPCBatchRequestLimit,
		JSONRPCBlockRangeLimit:   DefaultJSONRPCBlockRangeLimit,
		/* START ALPHA CHANGE 2451 -- initialize all fee-history budgets -- */
		JSONRPCFeeHistoryBlockLimit:      DefaultJSONRPCFeeHistoryBlockLimit,
		JSONRPCFeeHistoryPercentileLimit: DefaultJSONRPCFeeHistoryPercentileLimit,
		JSONRPCFeeHistoryResultLimit:     DefaultJSONRPCFeeHistoryResultLimit,
		/* FINISH ALPHA CHANGE 2451 -- initialize all fee-history budgets -- */
		Relayer:                 false,
		NumBlockConfirmations:   DefaultNumBlockConfirmations,
		ConcurrentRequestsDebug: DefaultConcurrentRequestsDebug,
		WebSocketReadLimit:      DefaultWebSocketReadLimit,
		/* START ALPHA CHANGE 2249 -- enable native RPC admission by default -- */
		JSONRPCAdmission: jsonrpc.DefaultAdmissionConfig(),
		/* FINISH ALPHA CHANGE 2249 -- enable native RPC admission by default -- */
		MetricsInterval: DefaultMetricsInterval,
		/* START ALPHA CHANGE 832 -- default nodes to full history mode -- */
		History: &History{
			Mode:            DefaultHistoryMode,
			RetentionBlocks: DefaultHistoryRetentionBlocks,
			/* START ALPHA CHANGE 1564 -- apply default recent-history segment size -- */
			SegmentBlocks: DefaultHistorySegmentBlocks,
			/* FINISH ALPHA CHANGE 1564 -- apply default recent-history segment size -- */
			/* START ALPHA CHANGE 1720 -- default only independent snapshot consumer settings -- */
			SnapshotSourceDir:         "",
			SnapshotBootstrapRequired: false,
			/* FINISH ALPHA CHANGE 1720 -- default only independent snapshot consumer settings -- */
			/* START ALPHA CHANGE 1787 -- default the node-wide MDBX map size -- */
			MDBXMapSizeGB: DefaultMDBXMapSizeGB,
			/* START ALPHA CHANGE 3457 -- preserve the four-KiB default -- */
			MDBXPageSizeBytes: mdbxconfig.DefaultPageSizeBytes,
			/* FINISH ALPHA CHANGE 3457 -- preserve the four-KiB default -- */
			/* FINISH ALPHA CHANGE 1787 -- default the node-wide MDBX map size -- */
		},
		/* FINISH ALPHA CHANGE 832 -- default nodes to full history mode -- */
	}
}

/* START ALPHA CHANGE 3305 -- reject missing required server configuration sections -- */
// ValidateSections checks sections required by server initialization.
// Optional sections retain their existing defaulting policies.
func (c *Config) ValidateSections() error {
	if c == nil {
		return fmt.Errorf("server configuration must not be nil")
	}

	if c.Network == nil {
		return fmt.Errorf(
			"invalid server configuration: section %q must not be null",
			"network",
		)
	}

	if c.Telemetry == nil {
		return fmt.Errorf(
			"invalid server configuration: section %q must not be null",
			"telemetry",
		)
	}

	if c.TxPool == nil {
		return fmt.Errorf(
			"invalid server configuration: section %q must not be null",
			"tx_pool",
		)
	}

	return nil
}

/* FINISH ALPHA CHANGE 3305 -- reject missing required server configuration sections -- */

// ReadConfigFile reads the config file from the specified path, builds a Config object
// and returns it.
//
// Supported file types: .json, .hcl, .yaml, .yml
func ReadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var unmarshalFunc func([]byte, interface{}) error

	switch {
	case strings.HasSuffix(path, ".hcl"):
		/* START ALPHA CHANGE 3497 -- adapt HCL page fields without changing other section decoding -- */
		unmarshalFunc = func(data []byte, output interface{}) error {
			target, ok := output.(*Config)
			if !ok || target == nil {
				return fmt.Errorf("HCL server configuration target is invalid")
			}
			if err := hcl.Unmarshal(data, target); err != nil {
				return err
			}

			// The pinned HCL decoder supports strings and signed integers,
			// but does not directly support the unsigned PageSize type.
			var pageSettings struct {
				History *struct {
					PageSize      *string `hcl:"mdbx_page_size_bytes"`
					FieldPageSize *string `hcl:"MDBXPageSizeBytes"`
				}
			}
			if err := hcl.Unmarshal(data, &pageSettings); err != nil {
				return err
			}

			if target.History != nil {
				// HCL replaces section pointers. Restore this setting's
				// default when the history block omits it.
				size := MDBXPageSize(mdbxconfig.DefaultPageSizeBytes)

				if section := pageSettings.History; section != nil {
					if section.PageSize != nil && section.FieldPageSize != nil {
						return fmt.Errorf(
							"duplicate HCL MDBX page-size settings: use only one spelling",
						)
					}
					value := section.PageSize
					if value == nil {
						value = section.FieldPageSize
					}
					if value != nil {
						if err := size.Set(*value); err != nil {
							return err
						}
					}
				}
				target.History.MDBXPageSizeBytes = size
			}
			return nil
		}
		/* FINISH ALPHA CHANGE 3497 -- adapt HCL page fields without changing other section decoding -- */
	case strings.HasSuffix(path, ".json"):
		unmarshalFunc = json.Unmarshal
	case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
		unmarshalFunc = yaml.Unmarshal
	default:
		return nil, fmt.Errorf("suffix of %s is neither hcl, json, yaml nor yml", path)
	}

	config := DefaultConfig()
	config.Network = new(Network)
	config.Network.MaxPeers = -1
	config.Network.MaxInboundPeers = -1
	config.Network.MaxOutboundPeers = -1

	if err := unmarshalFunc(data, config); err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 3305 -- reject missing required server configuration sections -- */
	if err := config.ValidateSections(); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3305 -- reject missing required server configuration sections -- */

	/* START ALPHA CHANGE 3267 -- remove restore-only HCL configuration decoding -- */
	/* FINISH ALPHA CHANGE 3267 -- remove restore-only HCL configuration decoding -- */

	/* START ALPHA CHANGE 3457 -- reject explicit invalid values from every configuration decoder -- */
	if config.History != nil {
		if err := mdbxconfig.Validate(uint64(config.History.MDBXPageSizeBytes)); err != nil {
			return nil, err
		}
	}
	/* FINISH ALPHA CHANGE 3457 -- reject explicit invalid values from every configuration decoder -- */

	return config, nil
}
