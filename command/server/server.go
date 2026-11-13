package server

import (
	"fmt"
	"os"

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/innovativebioresearch/innbc-chain/command/server/export"
	"github.com/innovativebioresearch/innbc-chain/server"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	serverCmd := &cobra.Command{
		Use: "server",
		/*START ALPHA CHANGE 5 -- rebrand server command short description --*/
		Short: "The default command that starts the INNBC Chain client, by bootstrapping all modules together",
		/*FINISH ALPHA CHANGE 5 -- rebrand server command short description --*/
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	helper.RegisterGRPCAddressFlag(serverCmd)
	helper.RegisterLegacyGRPCAddressFlag(serverCmd)

	/* START ALPHA CHANGE 2562 -- describe enforced operator listener restrictions -- */
	const grpcListenUsage = "operator gRPC listen address; must resolve to an IPv4 or IPv6 loopback IP (for example 127.0.0.1:9632 or [::1]:9632)"
	serverCmd.PersistentFlags().Lookup(command.GRPCAddressFlag).Usage = grpcListenUsage
	serverCmd.PersistentFlags().Lookup(command.GRPCAddressFlagLEGACY).Usage = grpcListenUsage
	/* FINISH ALPHA CHANGE 2562 -- describe enforced operator listener restrictions -- */

	helper.RegisterJSONRPCFlag(serverCmd)

	registerSubcommands(serverCmd)
	setFlags(serverCmd)

	return serverCmd
}

func registerSubcommands(baseCmd *cobra.Command) {
	baseCmd.AddCommand(
		// server export
		export.GetCommand(),
	)
}

func setFlags(cmd *cobra.Command) {
	defaultConfig := config.DefaultConfig()

	/* START ALPHA CHANGE 2250 -- expose configurable native RPC admission budgets -- */
	if params.rawConfig.JSONRPCAdmission == nil {
		params.rawConfig.JSONRPCAdmission = defaultConfig.JSONRPCAdmission
	}
	admission := params.rawConfig.JSONRPCAdmission
	defaultAdmission := defaultConfig.JSONRPCAdmission

	for _, setting := range []struct {
		target       *int
		name         string
		defaultValue int
		description  string
	}{
		{&admission.RequestRate, "json-rpc-request-rate", defaultAdmission.RequestRate,
			"global RPC HTTP request and WebSocket message rate per second"},
		{&admission.RequestBurst, "json-rpc-request-burst", defaultAdmission.RequestBurst,
			"global RPC transport burst allowance"},
		{&admission.CallRate, "json-rpc-call-rate", defaultAdmission.CallRate,
			"global RPC method-call rate per second, counting every batch member"},
		{&admission.CallBurst, "json-rpc-call-burst", defaultAdmission.CallBurst,
			"RPC call burst allowance; a batch must fit within this allowance"},
		{&admission.MaxConcurrent, "json-rpc-max-concurrent", defaultAdmission.MaxConcurrent,
			"maximum admitted RPC requests/messages, without a waiting queue"},
		{&admission.MaxExpensive, "json-rpc-max-expensive", defaultAdmission.MaxExpensive,
			"maximum concurrent RPC simulations and expensive historical queries"},
		{&admission.MaxWebSockets, "json-rpc-max-websockets", defaultAdmission.MaxWebSockets,
			"maximum RPC WebSocket connections"},
		{&admission.MaxFilters, "json-rpc-max-filters", defaultAdmission.MaxFilters,
			"maximum combined RPC filters and subscriptions"},
	} {
		cmd.Flags().IntVar(
			setting.target,
			setting.name,
			setting.defaultValue,
			setting.description,
		)
	}

	cmd.Flags().Int64Var(
		&admission.MaxBodyBytes,
		"json-rpc-max-body-bytes",
		defaultAdmission.MaxBodyBytes,
		"maximum RPC HTTP body size and upper bound on WebSocket message size",
	)
	/* FINISH ALPHA CHANGE 2250 -- expose configurable native RPC admission budgets -- */

	cmd.Flags().StringVar(
		&params.rawConfig.LogLevel,
		command.LogLevelFlag,
		defaultConfig.LogLevel,
		"the log level for console output",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.GenesisPath,
		genesisPathFlag,
		defaultConfig.GenesisPath,
		"the genesis file used for starting the chain",
	)

	cmd.Flags().StringVar(
		&params.configPath,
		configFlag,
		"",
		"the path to the CLI config. Supports .json, .hcl, .yaml, .yml",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.DataDir,
		dataDirFlag,
		defaultConfig.DataDir,
		/*START ALPHA CHANGE 6 -- rebrand server data directory flag description --*/
		"the data directory used for storing INNBC Chain client data",
		/*FINISH ALPHA CHANGE 6 -- rebrand server data directory flag description --*/
	)

	cmd.Flags().StringVar(
		&params.rawConfig.Network.Libp2pAddr,
		libp2pAddressFlag,
		defaultConfig.Network.Libp2pAddr,
		"the address and port for the libp2p service",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.Telemetry.PrometheusAddr,
		prometheusAddressFlag,
		"",
		"the address and port for the prometheus instrumentation service (address:port). "+
			"If only port is defined (:port) it will bind to 0.0.0.0:port",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.Network.NatAddr,
		natFlag,
		"",
		"the external IP address without port, as can be seen by peers",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.Network.DNSAddr,
		dnsFlag,
		"",
		"the host DNS address which can be used by a remote peer for connection",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.BlockGasTarget,
		blockGasTargetFlag,
		defaultConfig.BlockGasTarget,
		"the target block gas limit for the chain. If omitted, the value of the parent block is used",
	)

	/* START ALPHA CHANGE 3220 -- remove the server remote secrets configuration flag -- */
	/* FINISH ALPHA CHANGE 3220 -- remove the server remote secrets configuration flag -- */

	/* START ALPHA CHANGE 3266 -- retire legacy RLP restore flags -- */
	/* FINISH ALPHA CHANGE 3266 -- retire legacy RLP restore flags -- */

	cmd.Flags().BoolVar(
		&params.rawConfig.ShouldSeal,
		sealFlag,
		defaultConfig.ShouldSeal,
		"the flag indicating that the client should seal blocks",
	)

	cmd.Flags().BoolVar(
		&params.rawConfig.Network.NoDiscover,
		command.NoDiscoverFlag,
		defaultConfig.Network.NoDiscover,
		"prevent the client from discovering other peers",
	)

	/* START ALPHA CHANGE 676 -- add peer directory CLI options -- */
	cmd.Flags().BoolVar(
		&params.rawConfig.Network.PeerDirectoryEnabled,
		peerDirectoryEnabledFlag,
		defaultConfig.Network.PeerDirectoryEnabled,
		"enable authenticated INNBC validator peer directory refresh",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.Network.PeerDirectoryURL,
		peerDirectoryURLFlag,
		defaultConfig.Network.PeerDirectoryURL,
		"the authenticated INNBC validator peer directory exchange endpoint",
	)

	/* FINISH ALPHA CHANGE 676 -- add peer directory CLI options -- */

	cmd.Flags().Int64Var(
		&params.rawConfig.Network.MaxPeers,
		maxPeersFlag,
		-1,
		"the client's max number of peers allowed",
	)
	// override default usage value
	cmd.Flag(maxPeersFlag).DefValue = fmt.Sprintf("%d", defaultConfig.Network.MaxPeers)

	cmd.Flags().Int64Var(
		&params.rawConfig.Network.MaxInboundPeers,
		maxInboundPeersFlag,
		-1,
		"the client's max number of inbound peers allowed",
	)
	// override default usage value
	cmd.Flag(maxInboundPeersFlag).DefValue = fmt.Sprintf("%d", defaultConfig.Network.MaxInboundPeers)
	cmd.MarkFlagsMutuallyExclusive(maxPeersFlag, maxInboundPeersFlag)

	cmd.Flags().Int64Var(
		&params.rawConfig.Network.MaxOutboundPeers,
		maxOutboundPeersFlag,
		-1,
		"the client's max number of outbound peers allowed",
	)
	// override default usage value
	cmd.Flag(maxOutboundPeersFlag).DefValue = fmt.Sprintf("%d", defaultConfig.Network.MaxOutboundPeers)
	cmd.MarkFlagsMutuallyExclusive(maxPeersFlag, maxOutboundPeersFlag)

	cmd.Flags().Uint64Var(
		&params.rawConfig.TxPool.PriceLimit,
		priceLimitFlag,
		defaultConfig.TxPool.PriceLimit,
		fmt.Sprintf(
			"the minimum gas price limit to enforce for acceptance into the pool (default %d)",
			defaultConfig.TxPool.PriceLimit,
		),
	)

	cmd.Flags().Uint64Var(
		&params.rawConfig.TxPool.MaxSlots,
		maxSlotsFlag,
		defaultConfig.TxPool.MaxSlots,
		"maximum slots in the pool",
	)

	cmd.Flags().Uint64Var(
		&params.rawConfig.TxPool.MaxAccountEnqueued,
		maxEnqueuedFlag,
		defaultConfig.TxPool.MaxAccountEnqueued,
		"maximum number of enqueued transactions per account",
	)

	cmd.Flags().StringArrayVar(
		&params.rawConfig.CorsAllowedOrigins,
		corsOriginFlag,
		defaultConfig.Headers.AccessControlAllowOrigins,
		"the CORS header indicating whether any JSON-RPC response can be shared with the specified origin",
	)

	cmd.Flags().Uint64Var(
		&params.rawConfig.JSONRPCBatchRequestLimit,
		jsonRPCBatchRequestLimitFlag,
		defaultConfig.JSONRPCBatchRequestLimit,
		"max length to be considered when handling json-rpc batch requests, value of 0 disables it",
	)

	/* START ALPHA CHANGE 2485 -- document the safe zero log-range fallback -- */
	cmd.Flags().Uint64Var(
		&params.rawConfig.JSONRPCBlockRangeLimit,
		jsonRPCBlockRangeLimitFlag,
		defaultConfig.JSONRPCBlockRangeLimit,
		"maximum toBlock-fromBlock difference for historical log queries "+
			"(e.g. eth_getLogs); 0 selects the default of 256",
	)
	/* FINISH ALPHA CHANGE 2485 -- document the safe zero log-range fallback -- */

	/* START ALPHA CHANGE 2454 -- expose configurable fee-history budgets -- */
	cmd.Flags().Uint64Var(
		&params.rawConfig.JSONRPCFeeHistoryBlockLimit,
		jsonRPCFeeHistoryBlockLimitFlag,
		defaultConfig.JSONRPCFeeHistoryBlockLimit,
		"maximum blocks served by eth_feeHistory; 0 selects the code default of 256",
	)
	cmd.Flags().Uint64Var(
		&params.rawConfig.JSONRPCFeeHistoryPercentileLimit,
		jsonRPCFeeHistoryPercentileLimitFlag,
		defaultConfig.JSONRPCFeeHistoryPercentileLimit,
		"maximum requested reward percentiles for eth_feeHistory (1-100); 0 selects the code default of 10",
	)
	cmd.Flags().Uint64Var(
		&params.rawConfig.JSONRPCFeeHistoryResultLimit,
		jsonRPCFeeHistoryResultLimitFlag,
		defaultConfig.JSONRPCFeeHistoryResultLimit,
		"maximum eth_feeHistory result units: effective blocks times max(1, percentile count); "+
			"0 selects the code default of 2560",
	)
	/* FINISH ALPHA CHANGE 2454 -- expose configurable fee-history budgets -- */

	cmd.Flags().StringVar(
		&params.rawConfig.LogFilePath,
		logFileLocationFlag,
		defaultConfig.LogFilePath,
		"write all logs to the file at specified location instead of writing them to console",
	)

	cmd.Flags().BoolVar(
		&params.rawConfig.Relayer,
		relayerFlag,
		defaultConfig.Relayer,
		/* START ALPHA CHANGE 528 -- remove state-sync wording from relayer flag description -- */
		"mark this node as an INNBCBFT relayer",
		/* FINISH ALPHA CHANGE 528 -- remove state-sync wording from relayer flag description -- */
	)

	cmd.Flags().Uint64Var(
		&params.rawConfig.NumBlockConfirmations,
		numBlockConfirmationsFlag,
		defaultConfig.NumBlockConfirmations,
		"minimal number of child blocks required for the parent block to be considered final",
	)

	cmd.Flags().Uint64Var(
		&params.rawConfig.ConcurrentRequestsDebug,
		concurrentRequestsDebugFlag,
		defaultConfig.ConcurrentRequestsDebug,
		"maximal number of concurrent requests for debug endpoints",
	)

	cmd.Flags().Uint64Var(
		&params.rawConfig.WebSocketReadLimit,
		webSocketReadLimitFlag,
		defaultConfig.WebSocketReadLimit,
		"maximum size in bytes for a message read from the peer by websocket",
	)

	cmd.Flags().DurationVar(
		&params.rawConfig.MetricsInterval,
		metricsIntervalFlag,
		defaultConfig.MetricsInterval,
		"the interval (in seconds) at which special metrics are generated. a value of zero means the metrics are disabled",
	)

	/* START ALPHA CHANGE 838 -- register local history retention CLI flags -- */
	cmd.Flags().StringVar(
		&params.rawConfig.History.Mode,
		historyModeFlag,
		defaultConfig.History.Mode,
		"local history mode: full or recent",
	)

	cmd.Flags().Uint64Var(
		&params.rawConfig.History.RetentionBlocks,
		historyRetentionBlocksFlag,
		defaultConfig.History.RetentionBlocks,
		"number of recent blocks whose heavy history is retained in recent mode",
	)

	/* START ALPHA CHANGE 1568 -- configure recent-history physical segment size -- */
	cmd.Flags().Uint64Var(
		&params.rawConfig.History.SegmentBlocks,
		historySegmentBlocksFlag,
		defaultConfig.History.SegmentBlocks,
		"number of canonical blocks stored in each recent-history MDBX segment",
	)
	/* FINISH ALPHA CHANGE 1568 -- configure recent-history physical segment size -- */

	/* START ALPHA CHANGE 1726 -- remove obsolete in-node snapshot producer flags -- */
	/* FINISH ALPHA CHANGE 1726 -- remove obsolete in-node snapshot producer flags -- */

	/* START ALPHA CHANGE 878 -- register snapshot source CLI flag -- */
	cmd.Flags().StringVar(
		&params.rawConfig.History.SnapshotSourceDir,
		snapshotSourceDirFlag,
		defaultConfig.History.SnapshotSourceDir,
		"snapshot-kind directory containing latest.json and versioned snapshot artifacts",
	)

	/* START ALPHA CHANGE 1316 -- register strict snapshot bootstrap CLI flag -- */
	cmd.Flags().BoolVar(
		&params.rawConfig.History.SnapshotBootstrapRequired,
		snapshotBootstrapRequiredFlag,
		defaultConfig.History.SnapshotBootstrapRequired,
		"fail startup if configured snapshot bootstrap cannot be discovered, staged, validated, or activated",
	)
	/* FINISH ALPHA CHANGE 1316 -- register strict snapshot bootstrap CLI flag -- */

	/* START ALPHA CHANGE 1790 -- register one node-wide MDBX map size -- */
	cmd.Flags().Uint64Var(
		&params.rawConfig.History.MDBXMapSizeGB,
		mdbxMapSizeGBFlag,
		defaultConfig.History.MDBXMapSizeGB,
		"MDBX map size limit in GiB for blockchain and live trie storage",
	)
	/* START ALPHA CHANGE 3458 -- expose page-size selection with blank-value defaulting -- */
	cmd.Flags().Var(
		&params.rawConfig.History.MDBXPageSizeBytes,
		"mdbx-page-size-bytes",
		"MDBX page size in bytes; default 4096; must match existing blockchain and trie databases",
	)
	/* FINISH ALPHA CHANGE 3458 -- expose page-size selection with blank-value defaulting -- */
	/* FINISH ALPHA CHANGE 1790 -- register one node-wide MDBX map size -- */
	/* FINISH ALPHA CHANGE 878 -- register snapshot source CLI flag -- */
	/* FINISH ALPHA CHANGE 838 -- register local history retention CLI flags -- */

	setLegacyFlags(cmd)

	/* START ALPHA CHANGE 718 -- stop registering hidden legacy dev server flags -- */
	/* FINISH ALPHA CHANGE 718 -- stop registering hidden legacy dev server flags -- */
}

// setLegacyFlags sets the legacy flags to preserve backwards compatibility
// with running partners
func setLegacyFlags(cmd *cobra.Command) {
	// Legacy IBFT base timeout flag
	cmd.Flags().Uint64Var(
		&params.ibftBaseTimeoutLegacy,
		ibftBaseTimeoutFlagLEGACY,
		0,
		"",
	)

	_ = cmd.Flags().MarkHidden(ibftBaseTimeoutFlagLEGACY)
}

/* START ALPHA CHANGE 719 -- remove hidden legacy dev server flag registration -- */
/* FINISH ALPHA CHANGE 719 -- remove hidden legacy dev server flag registration -- */

func runPreRun(cmd *cobra.Command, _ []string) error {
	// Set the grpc and json ip:port bindings
	// The config file will have precedence over --flag
	params.setRawGRPCAddress(helper.GetGRPCAddress(cmd))
	params.setRawJSONRPCAddress(helper.GetJSONRPCAddress(cmd))
	params.setJSONLogFormat(helper.GetJSONLogFormat(cmd))

	/* START ALPHA CHANGE 1728 -- remove obsolete snapshot producer flag tracking -- */
	/* FINISH ALPHA CHANGE 1728 -- remove obsolete snapshot producer flag tracking -- */

	// Check if the config file has been specified
	// Config file settings will override JSON-RPC and GRPC address values
	if isConfigFileSpecified(cmd) {
		if err := params.initConfigFromFile(); err != nil {
			return err
		}
	}

	if err := params.initRawParams(); err != nil {
		return err
	}

	return nil
}

func isConfigFileSpecified(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(configFlag)
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)

	if err := runServerLoop(params.generateConfig(), outputter); err != nil {
		outputter.SetError(err)
		outputter.WriteOutput()

		/* START ALPHA CHANGE 1463 -- exit nonzero after fatal server failure in every output mode -- */
		// CLI output already exits inside WriteOutput. JSON output returns, so
		// explicitly preserve failure status for systemd and other supervisors.
		os.Exit(1)
		/* FINISH ALPHA CHANGE 1463 -- exit nonzero after fatal server failure in every output mode -- */
	}
}

func runServerLoop(
	config *server.Config,
	outputter command.OutputFormatter,
) error {
	serverInstance, err := server.NewServer(config)
	if err != nil {
		return err
	}

	/* START ALPHA CHANGE 1464 -- listen for fatal consensus errors alongside process signals -- */
	return helper.HandleSignalsOrError(
		serverInstance.Close,
		outputter,
		serverInstance.FatalErrorCh(),
	)
	/* FINISH ALPHA CHANGE 1464 -- listen for fatal consensus errors alongside process signals -- */
}
