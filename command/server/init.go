package server

import (
	"errors"
	"fmt"
	"math"
	"net"

	/* START ALPHA CHANGE 3266 -- remove the retired RLP importer dependency -- */
	/* FINISH ALPHA CHANGE 3266 -- remove the retired RLP importer dependency -- */
	/* START ALPHA CHANGE 3139 -- share checked recent-history geometry with storage -- */
	segmentedstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/segmented"
	/* FINISH ALPHA CHANGE 3139 -- share checked recent-history geometry with storage -- */
	"github.com/innovativebioresearch/innbc-chain/command/server/config"
	/* START ALPHA CHANGE 2453 -- validate fee-history settings before startup -- */
	"github.com/innovativebioresearch/innbc-chain/gasprice"
	/* FINISH ALPHA CHANGE 2453 -- validate fee-history settings before startup -- */

	helperCommon "github.com/innovativebioresearch/innbc-chain/helper/common"
	/* START ALPHA CHANGE 3459 -- resolve page-size defaults during command initialization -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3459 -- resolve page-size defaults during command initialization -- */
	"github.com/innovativebioresearch/innbc-chain/network/common"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/network"

	/* START ALPHA CHANGE 3220 -- remove remote secrets configuration loading dependency -- */
	/* FINISH ALPHA CHANGE 3220 -- remove remote secrets configuration loading dependency -- */

	/* START ALPHA CHANGE 2055 -- use mandatory state transaction gas capacity in server validation -- */
	"github.com/innovativebioresearch/innbc-chain/types"
	/* FINISH ALPHA CHANGE 2055 -- use mandatory state transaction gas capacity in server validation -- */ /* START ALPHA CHANGE 720 -- remove legacy dev consensus import from server init -- */ /* FINISH ALPHA CHANGE 720 -- remove legacy dev consensus import from server init -- */)

var (
	errDataDirectoryUndefined = errors.New("data directory not defined")
)

func (p *serverParams) initConfigFromFile() error {
	var parseErr error

	if p.rawConfig, parseErr = config.ReadConfigFile(p.configPath); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) initRawParams() error {
	/* START ALPHA CHANGE 3306 -- validate server sections before reading initialization settings -- */
	if err := p.rawConfig.ValidateSections(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3306 -- validate server sections before reading initialization settings -- */

	/* START ALPHA CHANGE 3266 -- remove retired RLP restore validation from startup -- */
	/* FINISH ALPHA CHANGE 3266 -- remove retired RLP restore validation from startup -- */
	/* START ALPHA CHANGE 2453 -- resolve fee-history defaults before node initialization -- */
	if err := p.initFeeHistoryConfig(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2453 -- resolve fee-history defaults before node initialization -- */

	/* START ALPHA CHANGE 2249 -- reject invalid RPC budgets before node initialization -- */
	if err := p.rawConfig.JSONRPCAdmission.Validate(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2249 -- reject invalid RPC budgets before node initialization -- */

	if err := p.initBlockGasTarget(); err != nil {
		return err
	}

	/* START ALPHA CHANGE 3220 -- startup has no remote identity configuration loading phase -- */
	/* FINISH ALPHA CHANGE 3220 -- startup has no remote identity configuration loading phase -- */

	if err := p.initGenesisConfig(); err != nil {
		return err
	}

	if err := p.initDataDirLocation(); err != nil {
		return err
	}

	/* START ALPHA CHANGE 839 -- validate local history retention config -- */
	if err := p.initHistoryConfig(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 839 -- validate local history retention config -- */

	/* START ALPHA CHANGE 721 -- stop applying legacy dev mode mutations to server config -- */
	/* FINISH ALPHA CHANGE 721 -- stop applying legacy dev mode mutations to server config -- */

	/* START ALPHA CHANGE 2520 -- reject invalid peer limits during initialization -- */
	if err := p.initPeerLimits(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2520 -- reject invalid peer limits during initialization -- */

	p.initLogFileLocation()

	p.relayer = p.rawConfig.Relayer

	return p.initAddresses()
}

/* START ALPHA CHANGE 2453 -- normalize all fee-history configuration inputs -- */
func (p *serverParams) initFeeHistoryConfig() error {
	resolved, err := gasprice.ResolveFeeHistoryConfig(&gasprice.Config{
		FeeHistoryBlockLimit:      p.rawConfig.JSONRPCFeeHistoryBlockLimit,
		FeeHistoryPercentileLimit: p.rawConfig.JSONRPCFeeHistoryPercentileLimit,
		FeeHistoryResultLimit:     p.rawConfig.JSONRPCFeeHistoryResultLimit,
	})
	if err != nil {
		return err
	}

	p.rawConfig.JSONRPCFeeHistoryBlockLimit = resolved.FeeHistoryBlockLimit
	p.rawConfig.JSONRPCFeeHistoryPercentileLimit = resolved.FeeHistoryPercentileLimit
	p.rawConfig.JSONRPCFeeHistoryResultLimit = resolved.FeeHistoryResultLimit

	return nil
}

/* FINISH ALPHA CHANGE 2453 -- normalize all fee-history configuration inputs -- */

func (p *serverParams) initDataDirLocation() error {
	if p.rawConfig.DataDir == "" {
		return errDataDirectoryUndefined
	}

	return nil
}

/* START ALPHA CHANGE 840 -- validate local history mode and block-count settings -- */
func (p *serverParams) initHistoryConfig() error {
	if p.rawConfig.History == nil {
		p.rawConfig.History = config.DefaultConfig().History
	}

	/* START ALPHA CHANGE 1766 -- apply segment geometry only to recent-history nodes -- */
	switch p.rawConfig.History.Mode {
	case "full":
		p.rawConfig.History.RetentionBlocks = 0
		p.rawConfig.History.SegmentBlocks = 0
	case "recent":
		if p.rawConfig.History.RetentionBlocks == 0 {
			return errors.New("history retention blocks must be greater than zero")
		}

		/* START ALPHA CHANGE 3139 -- reject unrepresentable recent-history counts before startup -- */
		if _, err := segmentedstorage.RetainedSegmentCount(
			p.rawConfig.History.SegmentBlocks,
			p.rawConfig.History.RetentionBlocks,
		); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 3139 -- reject unrepresentable recent-history counts before startup -- */
	default:
		return fmt.Errorf(
			"invalid history mode %q: expected full or recent",
			p.rawConfig.History.Mode,
		)
	}
	/* FINISH ALPHA CHANGE 1766 -- apply segment geometry only to recent-history nodes -- */

	/* START ALPHA CHANGE 3459 -- resolve omitted programmatic page-size settings -- */
	pageSize, err := mdbxconfig.Resolve(uint64(p.rawConfig.History.MDBXPageSizeBytes))
	if err != nil {
		return err
	}
	p.rawConfig.History.MDBXPageSizeBytes = config.MDBXPageSize(pageSize)
	/* FINISH ALPHA CHANGE 3459 -- resolve omitted programmatic page-size settings -- */

	/* START ALPHA CHANGE 1791 -- validate one node-wide MDBX map size -- */
	if p.rawConfig.History.MDBXMapSizeGB == 0 {
		return errors.New("MDBX map size must be greater than zero")
	}

	maxMDBXMapSizeGB := uint64(math.MaxInt / (1024 * 1024 * 1024))
	if p.rawConfig.History.MDBXMapSizeGB > maxMDBXMapSizeGB {
		return fmt.Errorf(
			"MDBX map size exceeds platform maximum %d GiB",
			maxMDBXMapSizeGB,
		)
	}
	/* FINISH ALPHA CHANGE 1791 -- validate one node-wide MDBX map size -- */

	/* START ALPHA CHANGE 1665 -- allow explicit checkpoint bootstrap in full mode -- */
	// Recent nodes may use their configured segmented snapshot source in
	// best-effort or required mode. Full nodes ignore their checkpoint source
	// unless strict snapshot bootstrap is explicitly enabled.
	if p.rawConfig.History.SnapshotBootstrapRequired &&
		p.rawConfig.History.SnapshotSourceDir == "" {
		return errors.New("snapshot bootstrap required needs a snapshot source directory")
	}
	/* FINISH ALPHA CHANGE 1665 -- allow explicit checkpoint bootstrap in full mode -- */

	return nil
}

/* FINISH ALPHA CHANGE 840 -- validate local history mode and block-count settings -- */

func (p *serverParams) initLogFileLocation() {
	if p.isLogFileLocationSet() {
		p.logFileLocation = p.rawConfig.LogFilePath
	}
}

/* START ALPHA CHANGE 2055 -- validate configured and effective block gas targets -- */
func validateBlockGasTarget(blockGasTarget uint64) error {
	if blockGasTarget != 0 && blockGasTarget < types.StateTransactionGasLimit {
		return fmt.Errorf(
			"block gas target must be zero or at least mandatory state transaction gas limit %d",
			types.StateTransactionGasLimit,
		)
	}

	return nil
}

func (p *serverParams) initBlockGasTarget() error {
	var parseErr error

	if p.blockGasTarget, parseErr = helperCommon.ParseUint64orHex(
		&p.rawConfig.BlockGasTarget,
	); parseErr != nil {
		return parseErr
	}

	return validateBlockGasTarget(p.blockGasTarget)
}

/* FINISH ALPHA CHANGE 2055 -- validate configured and effective block gas targets -- */

/* START ALPHA CHANGE 3220 -- remove the unused remote secrets configuration loader -- */
/* FINISH ALPHA CHANGE 3220 -- remove the unused remote secrets configuration loader -- */

func (p *serverParams) initGenesisConfig() error {
	var parseErr error

	if p.genesisConfig, parseErr = chain.Import(
		p.rawConfig.GenesisPath,
	); parseErr != nil {
		return parseErr
	}

	// if block-gas-target flag is set override genesis.json value
	if p.blockGasTarget != 0 {
		p.genesisConfig.Params.BlockGasTarget = p.blockGasTarget
	}

	/* START ALPHA CHANGE 2055 -- validate configured and effective block gas targets -- */
	return validateBlockGasTarget(p.genesisConfig.Params.BlockGasTarget)
	/* FINISH ALPHA CHANGE 2055 -- validate configured and effective block gas targets -- */
}

/* START ALPHA CHANGE 722 -- remove legacy dev mode consensus config mutation -- */
/* FINISH ALPHA CHANGE 722 -- remove legacy dev mode consensus config mutation -- */

/* START ALPHA CHANGE 2520 -- resolve and validate peer limits before publishing configuration -- */
func (p *serverParams) initPeerLimits() error {
	raw := p.rawConfig.Network

	for _, limit := range []struct {
		name  string
		value int64
	}{
		{maxPeersFlag, raw.MaxPeers},
		{maxInboundPeersFlag, raw.MaxInboundPeers},
		{maxOutboundPeersFlag, raw.MaxOutboundPeers},
	} {
		if limit.value < unsetPeersValue {
			return fmt.Errorf(
				"--%s must be -1 (unset) or non-negative, got %d",
				limit.name,
				limit.value,
			)
		}
	}

	defaults := network.DefaultConfig()
	resolved := network.Config{
		MaxPeers:         raw.MaxPeers,
		MaxInboundPeers:  raw.MaxInboundPeers,
		MaxOutboundPeers: raw.MaxOutboundPeers,
	}

	switch {
	case !p.isMaxPeersSet() && !p.isPeerRangeSet():
		resolved.MaxPeers = defaults.MaxPeers
		resolved.MaxInboundPeers = defaults.MaxInboundPeers
		resolved.MaxOutboundPeers = defaults.MaxOutboundPeers

	case p.isPeerRangeSet():
		if resolved.MaxInboundPeers == unsetPeersValue {
			resolved.MaxInboundPeers = defaults.MaxInboundPeers
		}
		if resolved.MaxOutboundPeers == unsetPeersValue {
			resolved.MaxOutboundPeers = defaults.MaxOutboundPeers
		}

		if resolved.MaxInboundPeers > math.MaxInt64-resolved.MaxOutboundPeers {
			return fmt.Errorf("combined inbound and outbound peer limits exceed int64 capacity")
		}

		resolved.MaxPeers = resolved.MaxInboundPeers + resolved.MaxOutboundPeers

	default:
		resolved.MaxOutboundPeers = int64(math.Floor(
			float64(resolved.MaxPeers) * network.DefaultDialRatio,
		))
		resolved.MaxInboundPeers = resolved.MaxPeers - resolved.MaxOutboundPeers
	}

	if err := resolved.ValidatePeerLimits(); err != nil {
		return err
	}

	raw.MaxPeers = resolved.MaxPeers
	raw.MaxInboundPeers = resolved.MaxInboundPeers
	raw.MaxOutboundPeers = resolved.MaxOutboundPeers

	return nil
}

/* FINISH ALPHA CHANGE 2520 -- resolve and validate peer limits before publishing configuration -- */

func (p *serverParams) initAddresses() error {
	if err := p.initPrometheusAddress(); err != nil {
		return err
	}

	if err := p.initLibp2pAddress(); err != nil {
		return err
	}

	if err := p.initNATAddress(); err != nil {
		return err
	}

	if err := p.initDNSAddress(); err != nil {
		return err
	}

	if err := p.initJSONRPCAddress(); err != nil {
		return err
	}

	return p.initGRPCAddress()
}

func (p *serverParams) initPrometheusAddress() error {
	if !p.isPrometheusAddressSet() {
		return nil
	}

	var parseErr error

	if p.prometheusAddress, parseErr = helper.ResolveAddr(
		p.rawConfig.Telemetry.PrometheusAddr,
		helper.AllInterfacesBinding,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) initLibp2pAddress() error {
	var parseErr error

	if p.libp2pAddress, parseErr = helper.ResolveAddr(
		p.rawConfig.Network.Libp2pAddr,
		helper.LocalHostBinding,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) initNATAddress() error {
	if !p.isNATAddressSet() {
		return nil
	}

	if p.natAddress = net.ParseIP(
		p.rawConfig.Network.NatAddr,
	); p.natAddress == nil {
		return errInvalidNATAddress
	}

	return nil
}

func (p *serverParams) initDNSAddress() error {
	if !p.isDNSAddressSet() {
		return nil
	}

	var parseErr error

	if p.dnsAddress, parseErr = common.MultiAddrFromDNS(
		p.rawConfig.Network.DNSAddr, p.libp2pAddress.Port,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) initJSONRPCAddress() error {
	var parseErr error

	if p.jsonRPCAddress, parseErr = helper.ResolveAddr(
		p.rawConfig.JSONRPCAddr,
		helper.AllInterfacesBinding,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) initGRPCAddress() error {
	var parseErr error

	if p.grpcAddress, parseErr = helper.ResolveAddr(
		p.rawConfig.GRPCAddr,
		helper.LocalHostBinding,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

/* START ALPHA CHANGE 3266 -- remove the retired RLP restore allowance validator -- */
/* FINISH ALPHA CHANGE 3266 -- remove the retired RLP restore allowance validator -- */
