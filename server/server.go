package server

/* START ALPHA CHANGE 3158 -- modernize empty interface spelling to any throughout this file -- */
/* FINISH ALPHA CHANGE 3158 -- modernize empty interface spelling to any throughout this file -- */

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx" //ALPHA CHANGE 754 -- import MDBX blockchain storage backend
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage/memory"

	/* START ALPHA CHANGE 1569 -- import segmented recent-history storage -- */
	segmentedstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/segmented"
	/* FINISH ALPHA CHANGE 1569 -- import segmented recent-history storage -- */
	consensusINNBCBFT "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/forkmanager"
	"github.com/innovativebioresearch/innbc-chain/gasprice"

	"github.com/hashicorp/go-hclog"
	/* START ALPHA CHANGE 3268 -- remove the retired RLP importer dependency -- */
	/* FINISH ALPHA CHANGE 3268 -- remove the retired RLP importer dependency -- */
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/helper/common"

	/* START ALPHA CHANGE 1625 -- use canonical chain data paths -- */
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	/* START ALPHA CHANGE 3462 -- preflight existing database geometry before startup side effects -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3462 -- preflight existing database geometry before startup side effects -- */
	/* FINISH ALPHA CHANGE 1625 -- use canonical chain data paths -- */
	"github.com/innovativebioresearch/innbc-chain/helper/progress"
	"github.com/innovativebioresearch/innbc-chain/jsonrpc"
	"github.com/innovativebioresearch/innbc-chain/network"
	"github.com/innovativebioresearch/innbc-chain/secrets"

	/* START ALPHA CHANGE 3219 -- initialize server identity storage through the local factory -- */
	"github.com/innovativebioresearch/innbc-chain/secrets/local"
	/* FINISH ALPHA CHANGE 3219 -- initialize server identity storage through the local factory -- */
	"github.com/innovativebioresearch/innbc-chain/server/proto"

	/* START ALPHA CHANGE 1703 -- consume complete snapshot artifacts during startup -- */
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	/* FINISH ALPHA CHANGE 1703 -- consume complete snapshot artifacts during startup -- */
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/addresslist"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/tracer"
	"github.com/innovativebioresearch/innbc-chain/txpool"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/innovativebioresearch/innbc-chain/validate"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

var (
	errBlockTimeMissing = errors.New("block time configuration is missing")
	errBlockTimeInvalid = errors.New("block time configuration is invalid")
)

// Server is the central manager of the blockchain client
type Server struct {
	logger       hclog.Logger
	config       *Config
	state        state.State
	stateStorage itrie.Storage

	consensus consensus.Consensus

	// blockchain stack
	blockchain *blockchain.Blockchain
	chain      *chain.Chain

	// state executor
	executor *state.Executor

	// jsonrpc stack
	jsonrpcServer *jsonrpc.JSONRPC

	// system grpc server
	grpcServer *grpc.Server

	// libp2p network
	network *network.Server

	// transaction pool
	txpool *txpool.TxPool

	prometheusServer *http.Server

	// secrets manager
	secretsManager secrets.SecretsManager

	/* START ALPHA CHANGE 3268 -- remove retired RLP restore progression ownership -- */
	/* FINISH ALPHA CHANGE 3268 -- remove retired RLP restore progression ownership -- */

	// gasHelper is providing functions regarding gas and fees
	gasHelper *gasprice.GasHelper

	/* START ALPHA CHANGE 1073 -- make server shutdown idempotent -- */
	closeOnce sync.Once
	/* FINISH ALPHA CHANGE 1073 -- make server shutdown idempotent -- */
}

// newFileLogger returns logger instance that writes all logs to a specified file.
// If log file can't be created, it returns an error
func newFileLogger(config *Config) (hclog.Logger, error) {
	logFileWriter, err := os.Create(config.LogFilePath)
	if err != nil {
		return nil, fmt.Errorf("could not create log file, %w", err)
	}

	return hclog.New(&hclog.LoggerOptions{
		/* START ALPHA CHANGE 157 -- rebrand file logger root name to INNBC -- */
		Name: "innbc",
		/* FINISH ALPHA CHANGE 157 -- rebrand file logger root name to INNBC -- */
		Level:      config.LogLevel,
		Output:     logFileWriter,
		JSONFormat: config.JSONLogFormat,
	}), nil
}

// newCLILogger returns minimal logger instance that sends all logs to standard output
func newCLILogger(config *Config) hclog.Logger {
	return hclog.New(&hclog.LoggerOptions{
		/* START ALPHA CHANGE 158 -- rebrand console logger root name to INNBC -- */
		Name: "innbc",
		/* FINISH ALPHA CHANGE 158 -- rebrand console logger root name to INNBC -- */
		Level:      config.LogLevel,
		JSONFormat: config.JSONLogFormat,
	})
}

// newLoggerFromConfig creates a new logger which logs to a specified file.
// If log file is not set it outputs to standard output ( console ).
// If log file is specified, and it can't be created the server command will error out
func newLoggerFromConfig(config *Config) (hclog.Logger, error) {
	if config.LogFilePath != "" {
		fileLoggerInstance, err := newFileLogger(config)
		if err != nil {
			return nil, err
		}

		return fileLoggerInstance, nil
	}

	return newCLILogger(config), nil
}

// NewServer creates a new Minimal server, using the passed in configuration
func NewServer(config *Config) (*Server, error) {
	/* START ALPHA CHANGE 2391 -- validate London correction before server startup side effects -- */
	if config == nil {
		return nil, fmt.Errorf("server config is missing")
	}

	if err := config.Chain.ValidateLondonFix(); err != nil {
		return nil, fmt.Errorf("unsupported chain configuration: %w", err)
	}
	/* FINISH ALPHA CHANGE 2391 -- validate London correction before server startup side effects -- */

	/* START ALPHA CHANGE 3052 -- validate chain ID before server startup side effects -- */
	if err := config.Chain.ValidateChainID(); err != nil {
		return nil, fmt.Errorf("unsupported chain configuration: %w", err)
	}
	/* FINISH ALPHA CHANGE 3052 -- validate chain ID before server startup side effects -- */

	/* START ALPHA CHANGE 2561 -- reject non-loopback operator addresses before server startup -- */
	if err := validateOperatorGRPCAddress(config.GRPCAddr); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2561 -- reject non-loopback operator addresses before server startup -- */

	/* START ALPHA CHANGE 2646 -- validate genesis stake before fresh or existing database startup -- */
	if _, exists := config.Chain.Params.Engine[consensusINNBCBFT.ConsensusName]; exists {
		if _, err := consensusINNBCBFT.GetINNBCBFTConfig(config.Chain); err != nil {
			return nil, fmt.Errorf("invalid INNBCBFT configuration: %w", err)
		}
	}
	/* FINISH ALPHA CHANGE 2646 -- validate genesis stake before fresh or existing database startup -- */

	/* START ALPHA CHANGE 2456 -- resolve fee-history budgets before startup side effects -- */
	if config.JSONRPC == nil {
		return nil, fmt.Errorf("JSON-RPC configuration is missing")
	}

	/* START ALPHA CHANGE 2484 -- normalize log-range limits for direct server callers -- */
	config.JSONRPC.BlockRangeLimit = jsonrpc.ResolveBlockRangeLimit(config.JSONRPC.BlockRangeLimit)
	/* FINISH ALPHA CHANGE 2484 -- normalize log-range limits for direct server callers -- */

	feeHistoryConfig, err := gasprice.ResolveFeeHistoryConfig(&gasprice.Config{
		FeeHistoryBlockLimit:      config.JSONRPC.FeeHistoryBlockLimit,
		FeeHistoryPercentileLimit: config.JSONRPC.FeeHistoryPercentileLimit,
		FeeHistoryResultLimit:     config.JSONRPC.FeeHistoryResultLimit,
	})
	if err != nil {
		return nil, err
	}

	config.JSONRPC.FeeHistoryBlockLimit = feeHistoryConfig.FeeHistoryBlockLimit
	config.JSONRPC.FeeHistoryPercentileLimit = feeHistoryConfig.FeeHistoryPercentileLimit
	config.JSONRPC.FeeHistoryResultLimit = feeHistoryConfig.FeeHistoryResultLimit
	/* FINISH ALPHA CHANGE 2456 -- resolve fee-history budgets before startup side effects -- */

	/* START ALPHA CHANGE 3103 -- validate genesis allocations before server startup side effects -- */
	if err := config.Chain.ValidateGenesisAllocations(); err != nil {
		return nil, fmt.Errorf("unsupported chain configuration: %w", err)
	}
	/* FINISH ALPHA CHANGE 3103 -- validate genesis allocations before server startup side effects -- */

	/* START ALPHA CHANGE 3140 -- reject unrepresentable recent-history counts before server side effects -- */
	if config.History != nil && config.History.Mode == "recent" {
		if _, err := segmentedstorage.RetainedSegmentCount(
			config.History.SegmentBlocks,
			config.History.RetentionBlocks,
		); err != nil {
			return nil, fmt.Errorf("invalid recent-history configuration: %w", err)
		}
	}
	/* FINISH ALPHA CHANGE 3140 -- reject unrepresentable recent-history counts before server side effects -- */

	/* START ALPHA CHANGE 3462 -- reject incompatible local databases before writable startup -- */
	if config.History == nil {
		return nil, errors.New("history configuration is missing")
	}
	pageSize, err := mdbxconfig.Resolve(config.History.MDBXPageSizeBytes)
	if err != nil {
		return nil, err
	}
	config.History.MDBXPageSizeBytes = uint64(pageSize)

	if config.DataDir != "" {
		if err := mdbxconfig.CheckChainData(
			datadir.ChainData(config.DataDir),
			config.History.MDBXPageSizeBytes,
		); err != nil {
			return nil, err
		}
	}
	/* FINISH ALPHA CHANGE 3462 -- reject incompatible local databases before writable startup -- */

	logger, err := newLoggerFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("could not setup new logger instance, %w", err)
	}

	/* START ALPHA CHANGE 3268 -- construct the server without the retired archive importer -- */
	m := &Server{
		logger:     logger.Named("server"),
		config:     config,
		chain:      config.Chain,
		grpcServer: grpc.NewServer(grpc.UnaryInterceptor(unaryInterceptor)),
	}
	/* FINISH ALPHA CHANGE 3268 -- construct the server without the retired archive importer -- */

	/* START ALPHA CHANGE 711 -- remove legacy IBFT startup notice -- */
	/* FINISH ALPHA CHANGE 711 -- remove legacy IBFT startup notice -- */

	m.logger.Info("Data dir", "path", config.DataDir)

	/* START ALPHA CHANGE 1231 -- log configured advertised peer addresses once at startup -- */
	natAddr := ""
	if config.Network.NatAddr != nil {
		natAddr = config.Network.NatAddr.String()
	}

	dnsAddr := ""
	if config.Network.DNS != nil {
		dnsAddr = config.Network.DNS.String()
	}

	m.logger.Info(
		"network advertised address settings configured",
		"libp2p", config.Network.Addr.String(),
		"nat", natAddr,
		"dns", dnsAddr,
	)
	/* FINISH ALPHA CHANGE 1231 -- log configured advertised peer addresses once at startup -- */

	/* START ALPHA CHANGE 1784 -- log the node-wide MDBX map size -- */
	m.logger.Info(
		"MDBX storage map size configured",
		"mapSizeGB", config.History.MDBXMapSizeGB,
		/* START ALPHA CHANGE 3462 -- log the resolved page size -- */
		"pageSizeBytes", config.History.MDBXPageSizeBytes,
		/* FINISH ALPHA CHANGE 3462 -- log the resolved page size -- */
	)
	/* FINISH ALPHA CHANGE 1784 -- log the node-wide MDBX map size -- */

	/* START ALPHA CHANGE 1704 -- log snapshot bootstrap policy -- */
	m.logger.Info(
		"snapshot bootstrap policy configured",
		"historyMode", config.History.Mode,
		"snapshotSourceDir", config.History.SnapshotSourceDir,
		"snapshotBootstrapRequired", config.History.SnapshotBootstrapRequired,
	)
	/* FINISH ALPHA CHANGE 1704 -- log snapshot bootstrap policy -- */

	/* START ALPHA CHANGE 1172 -- log configured JSON-RPC public safety limits once at startup -- */
	m.logger.Info(
		"JSON-RPC limits configured",
		"batchRequestLimit", config.JSONRPC.BatchLengthLimit,
		"blockRangeLimit", config.JSONRPC.BlockRangeLimit,
		/* START ALPHA CHANGE 2456 -- log effective fee-history budgets once -- */
		"feeHistoryBlockLimit", config.JSONRPC.FeeHistoryBlockLimit,
		"feeHistoryPercentileLimit", config.JSONRPC.FeeHistoryPercentileLimit,
		"feeHistoryResultLimit", config.JSONRPC.FeeHistoryResultLimit,
		/* FINISH ALPHA CHANGE 2456 -- log effective fee-history budgets once -- */
	)
	/* FINISH ALPHA CHANGE 1172 -- log configured JSON-RPC public safety limits once at startup -- */

	/* START ALPHA CHANGE 945 -- install INNBCBFT header hashing before snapshot validation -- */
	engineName := m.config.Chain.Params.GetEngine()
	if ConsensusType(engineName) == INNBCBFTConsensus {
		consensusINNBCBFT.SetupHeaderHashFunc()
	}
	/* FINISH ALPHA CHANGE 945 -- install INNBCBFT header hashing before snapshot validation -- */

	/* START ALPHA CHANGE 1705 -- activate complete snapshot before creating runtime databases -- */
	var snapshotBootManifest *snapshotbuilder.Manifest

	if config.DataDir != "" {
		snapshotBootManifest, err = m.activateConfiguredSnapshot()
		if err != nil {
			return nil, fmt.Errorf("snapshot bootstrap failed: %w", err)
		}
	}
	/* FINISH ALPHA CHANGE 1705 -- activate complete snapshot before creating runtime databases -- */

	var dirPaths = []string{
		/* START ALPHA CHANGE 1626 -- isolate mutable chain databases from node identities -- */
		filepath.Join(datadir.ChainDataDirName, datadir.BlockchainDirName),
		filepath.Join(datadir.ChainDataDirName, datadir.TrieDirName),
		filepath.Join(datadir.ChainDataDirName, datadir.ConsensusDirName),
		/* FINISH ALPHA CHANGE 1626 -- isolate mutable chain databases from node identities -- */
	}

	// Generate all the paths in the dataDir
	if err := common.SetupDataDir(config.DataDir, dirPaths, 0770); err != nil {
		return nil, fmt.Errorf("failed to create data directories: %w", err)
	}

	var db storage.Storage
	{
		if m.config.DataDir == "" {
			db, err = memory.NewMemoryStorage(nil)
			if err != nil {
				return nil, err
			}
		} else {
			/* START ALPHA CHANGE 1628 -- open blockchain storage inside chain-data -- */
			blockchainPath := datadir.Blockchain(m.config.DataDir)
			/* FINISH ALPHA CHANGE 1628 -- open blockchain storage inside chain-data -- */
			mdbxMapSize := mdbxMapSizeBytes(m.config.History.MDBXMapSizeGB)

			/* START ALPHA CHANGE 1570 -- segment only recent-history blockchain storage -- */
			if m.config.History.Mode == "recent" {
				db, err = segmentedstorage.NewStorage(
					blockchainPath,
					m.logger,
					m.config.History.SegmentBlocks,
					m.config.History.RetentionBlocks,
					mdbxMapSize,
					/* START ALPHA CHANGE 3462 -- configure recent-history creation geometry -- */
					config.History.MDBXPageSizeBytes,
					/* FINISH ALPHA CHANGE 3462 -- configure recent-history creation geometry -- */
				)
			} else {
				db, err = mdbxstorage.NewMDBXStorageWithMapSize(
					blockchainPath,
					m.logger,
					mdbxMapSize,
					/* START ALPHA CHANGE 3462 -- configure full-history creation geometry -- */
					config.History.MDBXPageSizeBytes,
					/* FINISH ALPHA CHANGE 3462 -- configure full-history creation geometry -- */
				)
			}
			/* FINISH ALPHA CHANGE 1570 -- segment only recent-history blockchain storage -- */

			if err != nil {
				return nil, err
			}
		}
	}

	if config.Telemetry.PrometheusAddr != nil {
		// Only setup telemetry if `PrometheusAddr` has been configured.
		if err := m.setupTelemetry(); err != nil {
			return nil, err
		}

		m.prometheusServer = m.startPrometheusServer(config.Telemetry.PrometheusAddr)
	}

	// Set up datadog profiler
	/*START ALPHA CHANGE 24 -- fix DataDog profiler error check to use ddErr --*/
	if ddErr := m.enableDataDogProfiler(); ddErr != nil {
		m.logger.Error("DataDog profiler setup failed", "err", ddErr.Error())
	}
	/*FINISH ALPHA CHANGE 24 -- fix DataDog profiler error check to use ddErr --*/

	// Set up the secrets manager
	if err := m.setupSecretsManager(); err != nil {
		return nil, fmt.Errorf("failed to set up the secrets manager: %w", err)
	}

	// start libp2p
	{
		netConfig := config.Network
		netConfig.Chain = m.config.Chain
		/* START ALPHA CHANGE 1627 -- keep network identity lookup rooted at the node directory -- */
		netConfig.DataDir = m.config.DataDir
		/* FINISH ALPHA CHANGE 1627 -- keep network identity lookup rooted at the node directory -- */
		netConfig.SecretsManager = m.secretsManager

		network, err := network.NewServer(logger, netConfig)
		if err != nil {
			return nil, err
		}
		m.network = network
	}

	// start blockchain object
	stateStorage, err := itrie.NewMDBXStorageWithMapSize(
		/* START ALPHA CHANGE 1629 -- open trie storage inside chain-data -- */
		datadir.Trie(m.config.DataDir),
		/* FINISH ALPHA CHANGE 1629 -- open trie storage inside chain-data -- */
		logger,
		mdbxMapSizeBytes(m.config.History.MDBXMapSizeGB),
		/* START ALPHA CHANGE 3462 -- configure live trie creation geometry -- */
		config.History.MDBXPageSizeBytes,
		/* FINISH ALPHA CHANGE 3462 -- configure live trie creation geometry -- */
	)
	if err != nil {
		return nil, err
	}

	m.stateStorage = stateStorage

	st := itrie.NewState(stateStorage)
	m.state = st

	m.executor = state.NewExecutor(config.Chain.Params, st, logger)

	// custom write genesis hook per consensus engine
	if factory, exists := genesisCreationFactory[ConsensusType(engineName)]; exists {
		m.executor.GenesisPostHook = factory(m.config.Chain, engineName)
	}

	// apply allow list contracts deployer genesis data
	if m.config.Chain.Params.ContractDeployerAllowList != nil {
		addresslist.ApplyGenesisAllocs(m.config.Chain.Genesis, contracts.AllowListContractsAddr,
			m.config.Chain.Params.ContractDeployerAllowList)
	}

	// apply block list contracts deployer genesis data
	if m.config.Chain.Params.ContractDeployerBlockList != nil {
		addresslist.ApplyGenesisAllocs(m.config.Chain.Genesis, contracts.BlockListContractsAddr,
			m.config.Chain.Params.ContractDeployerBlockList)
	}

	// apply transactions execution allow list genesis data
	if m.config.Chain.Params.TransactionsAllowList != nil {
		addresslist.ApplyGenesisAllocs(m.config.Chain.Genesis, contracts.AllowListTransactionsAddr,
			m.config.Chain.Params.TransactionsAllowList)
	}

	// apply transactions execution block list genesis data
	if m.config.Chain.Params.TransactionsBlockList != nil {
		addresslist.ApplyGenesisAllocs(m.config.Chain.Genesis, contracts.BlockListTransactionsAddr,
			m.config.Chain.Params.TransactionsBlockList)
	}

	/* START ALPHA CHANGE 442 -- remove bridge allow/block list genesis allocation application -- */
	/* FINISH ALPHA CHANGE 442 -- remove bridge allow/block list genesis allocation application -- */

	var genesisRoot types.Hash

	/* START ALPHA CHANGE 1706 -- reuse stored genesis commitment after complete activation -- */
	storedRoot, hasStoredRoot, err := storedGenesisStateRoot(db)
	if err != nil {
		return nil, err
	}

	if hasStoredRoot {
		genesisRoot = storedRoot
		config.Chain.Genesis.StateRoot = storedRoot

		if snapshotBootManifest != nil {
			logger.Info(
				"using activated snapshot chain state",
				"snapshotKind", snapshotBootManifest.SnapshotKind,
				"snapshotBlock", snapshotBootManifest.BlockNumber,
				"snapshotStateRoot", snapshotBootManifest.StateRoot.String(),
				"genesisStateRoot", storedRoot.String(),
			)
		}
	} else {
		var initialStateRoot = types.ZeroHash

		if ConsensusType(engineName) == INNBCBFTConsensus {
			innbcBFTConfig, err := consensusINNBCBFT.GetINNBCBFTConfig(config.Chain)
			if err != nil {
				return nil, err
			}

			if innbcBFTConfig.InitialTrieRoot != types.ZeroHash {
				checkedInitialTrieRoot, err := itrie.HashChecker(innbcBFTConfig.InitialTrieRoot.Bytes(), stateStorage)
				if err != nil {
					return nil, fmt.Errorf("error on state root verification %w", err)
				}

				if checkedInitialTrieRoot != innbcBFTConfig.InitialTrieRoot {
					return nil, errors.New("invalid initial state root")
				}

				logger.Info("Initial state root checked and correct")

				initialStateRoot = innbcBFTConfig.InitialTrieRoot
			}
		}

		genesisRoot, err = m.executor.WriteGenesis(config.Chain.Genesis.Alloc, initialStateRoot)
		if err != nil {
			return nil, err
		}

		// compute the genesis root state
		config.Chain.Genesis.StateRoot = genesisRoot
	}
	/* FINISH ALPHA CHANGE 1706 -- reuse stored genesis commitment after complete activation -- */

	if err := initForkManager(engineName, config.Chain); err != nil {
		return nil, err
	}

	// Use the london signer with eip-155 as a fallback one
	var signer crypto.TxSigner = crypto.NewLondonSigner(
		uint64(m.config.Chain.Params.ChainID),
		config.Chain.Params.Forks.IsActive(chain.Homestead, 0),
		crypto.NewEIP155Signer(
			uint64(m.config.Chain.Params.ChainID),
			config.Chain.Params.Forks.IsActive(chain.Homestead, 0),
		),
	)

	// blockchain object
	m.blockchain, err = blockchain.NewBlockchain(
		logger,
		db,
		config.Chain,
		nil,
		m.executor,
		signer,
	)
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 2456 -- use the resolved fee-history configuration -- */
	m.gasHelper, err = gasprice.NewGasHelper(&feeHistoryConfig, m.blockchain)
	if err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2456 -- use the resolved fee-history configuration -- */

	m.executor.GetHash = m.blockchain.GetHashHelper

	{
		hub := &txpoolHub{
			state:      m.state,
			Blockchain: m.blockchain,
		}

		// start transaction pool
		m.txpool, err = txpool.NewTxPool(
			logger,
			m.chain.Params.Forks,
			hub,
			m.grpcServer,
			m.network,
			&txpool.Config{
				MaxSlots:           m.config.MaxSlots,
				PriceLimit:         m.config.PriceLimit,
				MaxAccountEnqueued: m.config.MaxAccountEnqueued,
				ChainID:            big.NewInt(m.config.Chain.Params.ChainID),
			},
		)
		if err != nil {
			return nil, err
		}

		m.txpool.SetSigner(signer)
	}

	{
		// Setup consensus
		if err := m.setupConsensus(); err != nil {
			return nil, err
		}
		m.blockchain.SetConsensus(m.consensus)
	}

	// after consensus is done, we can mine the genesis block in blockchain
	// This is done because consensus might use a custom Hash function so we need
	// to wait for consensus because we do any block hashing like genesis
	if err := m.blockchain.ComputeGenesis(); err != nil {
		return nil, err
	}

	// initialize data in consensus layer
	if err := m.consensus.Initialize(); err != nil {
		return nil, err
	}

	// setup and start grpc server
	if err := m.setupGRPC(); err != nil {
		return nil, err
	}

	if err := m.network.Start(); err != nil {
		return nil, err
	}

	// setup and start jsonrpc server
	if err := m.setupJSONRPC(); err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 3268 -- remove legacy RLP import from node startup -- */
	/* FINISH ALPHA CHANGE 3268 -- remove legacy RLP import from node startup -- */

	/* START ALPHA CHANGE 2192 -- require the initial pool fee before starting consensus -- */
	if err := m.txpool.SetBaseFee(m.blockchain.Header()); err != nil {
		return nil, fmt.Errorf("initialize txpool base fee: %w", err)
	}
	/* FINISH ALPHA CHANGE 2192 -- require the initial pool fee before starting consensus -- */

	// start consensus
	if err := m.consensus.Start(); err != nil {
		return nil, err
	}

	m.txpool.Start()

	return m, nil
}

func unaryInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	// Validate request
	if err := validate.ValidateRequest(req); err != nil {
		return nil, err
	}

	return handler(ctx, req)
}

/* START ALPHA CHANGE 3268 -- remove the legacy RLP restore implementation -- */
/* FINISH ALPHA CHANGE 3268 -- remove the legacy RLP restore implementation -- */

type txpoolHub struct {
	state state.State
	*blockchain.Blockchain
}

/* START ALPHA CHANGE 2161 -- distinguish unavailable account state from legitimate absence -- */
// getAccountImpl is used for fetching account state from both TxPool and JSON-RPC.
func getAccountImpl(world state.State, root types.Hash, addr types.Address) (*state.Account, error) {
	snap, err := world.NewSnapshotAt(root)
	if err != nil {
		return nil, state.WrapLocalStateFailure(
			fmt.Sprintf("open account snapshot %s", root),
			err,
		)
	}

	account, err := snap.GetAccount(addr)
	if err != nil {
		return nil, state.WrapLocalStateFailure(
			fmt.Sprintf("read account %s", addr),
			err,
		)
	}

	if account == nil {
		return nil, jsonrpc.ErrStateNotFound
	}

	return account, nil
}

func (t *txpoolHub) GetNonce(root types.Hash, addr types.Address) (uint64, error) {
	account, err := getAccountImpl(t.state, root, addr)
	if err != nil {
		if errors.Is(err, state.ErrLocalStateFailure) {
			return 0, err
		}
		if errors.Is(err, jsonrpc.ErrStateNotFound) {
			return 0, nil
		}

		return 0, err
	}

	return account.Nonce, nil
}

/* FINISH ALPHA CHANGE 2161 -- distinguish unavailable account state from legitimate absence -- */

/* START ALPHA CHANGE 2161 -- preserve local balance failures before absence handling -- */
func (t *txpoolHub) GetBalance(root types.Hash, addr types.Address) (*big.Int, error) {
	account, err := getAccountImpl(t.state, root, addr)
	if err != nil {
		if errors.Is(err, state.ErrLocalStateFailure) {
			return nil, err
		}
		if errors.Is(err, jsonrpc.ErrStateNotFound) {
			return big.NewInt(0), nil
		}

		return nil, err
	}

	return account.Balance, nil
}

/* FINISH ALPHA CHANGE 2161 -- preserve local balance failures before absence handling -- */

/* START ALPHA CHANGE 3219 -- use local node identity storage without backend selection -- */
// setupSecretsManager opens local identity storage rooted at the node directory.
func (s *Server) setupSecretsManager() error {
	/* START ALPHA CHANGE 3234 -- use the local factory without the retired configuration argument -- */
	manager, err := local.SecretsManagerFactory(
		&secrets.SecretsManagerParams{
			Logger: s.logger,
			Extra: map[string]any{
				secrets.Path: s.config.DataDir,
			},
		},
	)
	/* FINISH ALPHA CHANGE 3234 -- use the local factory without the retired configuration argument -- */
	if err != nil {
		return fmt.Errorf("unable to instantiate local secrets manager, %w", err)
	}

	s.secretsManager = manager

	return nil
}

/* FINISH ALPHA CHANGE 3219 -- use local node identity storage without backend selection -- */

// setupConsensus sets up the consensus mechanism
func (s *Server) setupConsensus() error {
	engineName := s.config.Chain.Params.GetEngine()
	engine, ok := consensusBackends[ConsensusType(engineName)]

	if !ok {
		return fmt.Errorf("consensus engine '%s' not found", engineName)
	}

	engineConfig, ok := s.config.Chain.Params.Engine[engineName].(map[string]any)
	if !ok {
		engineConfig = map[string]any{}
	}

	var (
		blockTime = common.Duration{Duration: 0}
		err       error
	)

	/* START ALPHA CHANGE 712 -- require block time for the only supported INNBCBFT consensus engine -- */
	blockTime, err = extractBlockTime(engineConfig)
	if err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 712 -- require block time for the only supported INNBCBFT consensus engine -- */

	config := &consensus.Config{
		Params: s.config.Chain.Params,
		Config: engineConfig,
		/* START ALPHA CHANGE 1630 -- place consensus persistence inside chain-data -- */
		Path: datadir.Consensus(s.config.DataDir),
		/* FINISH ALPHA CHANGE 1630 -- place consensus persistence inside chain-data -- */
		IsRelayer:   s.config.Relayer,
		RPCEndpoint: s.config.JSONRPC.JSONRPCAddr.String(),
	}

	consensus, err := engine(
		&consensus.Params{
			Context:        context.Background(),
			Config:         config,
			TxPool:         s.txpool,
			Network:        s.network,
			Blockchain:     s.blockchain,
			Executor:       s.executor,
			Grpc:           s.grpcServer,
			Logger:         s.logger,
			SecretsManager: s.secretsManager,
			/* START ALPHA CHANGE 3318 -- convert the validated interval using integer arithmetic -- */
			BlockTime: uint64(blockTime.Duration / time.Second),
			/* FINISH ALPHA CHANGE 3318 -- convert the validated interval using integer arithmetic -- */
			NumBlockConfirmations: s.config.NumBlockConfirmations,
			MetricsInterval:       s.config.MetricsInterval,
		},
	)

	if err != nil {
		return err
	}

	s.consensus = consensus

	return nil
}

// extractBlockTime extracts blockTime parameter from consensus engine configuration.
// If it is missing or invalid, an appropriate error is returned.
/* START ALPHA CHANGE 3318 -- share runtime clock validation and reject retired drift -- */
func extractBlockTime(engineConfig map[string]any) (common.Duration, error) {
	raw, err := json.Marshal(engineConfig)
	if err != nil {
		return common.Duration{}, fmt.Errorf("%w: %v", errBlockTimeInvalid, err)
	}

	if err := common.RejectRetiredBlockTimeDrift(raw); err != nil {
		return common.Duration{}, fmt.Errorf("%w: %v", errBlockTimeInvalid, err)
	}
	if _, exists := engineConfig["blockTime"]; !exists {
		return common.Duration{}, errBlockTimeMissing
	}

	blockTime, err := common.DecodeBlockTimeConfig(raw)
	if err != nil {
		return common.Duration{}, fmt.Errorf("%w: %v", errBlockTimeInvalid, err)
	}

	return common.Duration{Duration: blockTime}, nil
}

/* FINISH ALPHA CHANGE 3318 -- share runtime clock validation and reject retired drift -- */

type jsonRPCHub struct {
	/* START ALPHA CHANGE 3268 -- remove retired restore progression from the RPC hub -- */
	state state.State
	/* FINISH ALPHA CHANGE 3268 -- remove retired restore progression from the RPC hub -- */

	*blockchain.Blockchain
	*txpool.TxPool
	*state.Executor
	*network.Server
	consensus.Consensus
	/* START ALPHA CHANGE 422 -- remove bridge data provider from JSON-RPC hub -- */
	/* FINISH ALPHA CHANGE 422 -- remove bridge data provider from JSON-RPC hub -- */
	gasprice.GasStore
}

func (j *jsonRPCHub) GetPeers() int {
	return len(j.Server.Peers())
}

func (j *jsonRPCHub) GetAccount(root types.Hash, addr types.Address) (*jsonrpc.Account, error) {
	acct, err := getAccountImpl(j.state, root, addr)
	if err != nil {
		return nil, err
	}

	account := &jsonrpc.Account{
		Nonce:   acct.Nonce,
		Balance: new(big.Int).Set(acct.Balance),
	}

	return account, nil
}

// GetForksInTime returns the active forks at the given block height
func (j *jsonRPCHub) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	return j.Executor.GetForksInTime(blockNumber)
}

/* START ALPHA CHANGE 2161 -- propagate RPC storage snapshot and slot failures -- */
func (j *jsonRPCHub) GetStorage(stateRoot types.Hash, addr types.Address, slot types.Hash) ([]byte, error) {
	account, err := getAccountImpl(j.state, stateRoot, addr)
	if err != nil {
		return nil, err
	}

	snap, err := j.state.NewSnapshotAt(stateRoot)
	if err != nil {
		return nil, state.WrapLocalStateFailure(
			fmt.Sprintf("open RPC storage snapshot %s", stateRoot),
			err,
		)
	}

	res, err := snap.GetStorage(addr, account.Root, slot)
	if err != nil {
		return nil, state.WrapLocalStateFailure(
			fmt.Sprintf("read RPC storage for account %s", addr),
			err,
		)
	}

	return res.Bytes(), nil
}

/* FINISH ALPHA CHANGE 2161 -- propagate RPC storage snapshot and slot failures -- */

/* START ALPHA CHANGE 2161 -- distinguish empty RPC bytecode from unavailable referenced code -- */
func (j *jsonRPCHub) GetCode(root types.Hash, addr types.Address) ([]byte, error) {
	account, err := getAccountImpl(j.state, root, addr)
	if err != nil {
		return nil, err
	}

	codeHash := types.BytesToHash(account.CodeHash)
	if codeHash == types.EmptyCodeHash {
		return []byte{}, nil
	}

	code, found, err := j.state.GetCode(codeHash)
	if err != nil {
		return nil, state.WrapLocalStateFailure(
			fmt.Sprintf("read RPC code for account %s", addr),
			err,
		)
	}
	/* START ALPHA CHANGE 2185 -- reject empty referenced bytecode at the RPC boundary -- */
	if !found || len(code) == 0 {
		return nil, state.WrapLocalStateFailure(
			fmt.Sprintf("read RPC code for account %s", addr),
			fmt.Errorf("%w: %s", state.ErrMissingCode, codeHash),
		)
	}
	/* FINISH ALPHA CHANGE 2185 -- reject empty referenced bytecode at the RPC boundary -- */

	return code, nil
}

/* FINISH ALPHA CHANGE 2161 -- distinguish empty RPC bytecode from unavailable referenced code -- */

func (j *jsonRPCHub) ApplyTxn(
	header *types.Header,
	txn *types.Transaction,
	override types.StateOverride,
	nonPayable bool,
) (result *runtime.ExecutionResult, err error) {
	blockCreator, err := j.GetConsensus().GetBlockCreator(header)
	if err != nil {
		return nil, err
	}

	transition, err := j.BeginTxn(header.StateRoot, header, blockCreator)
	if err != nil {
		return
	}

	if override != nil {
		if err = transition.WithStateOverride(override); err != nil {
			return
		}
	}

	transition.SetNonPayable(nonPayable)

	/* START ALPHA CHANGE 2162 -- reject simulation results after retained state failure -- */
	result, err = transition.Apply(txn)
	if stateErr := transition.Error(); stateErr != nil {
		return nil, stateErr
	}

	return
	/* FINISH ALPHA CHANGE 2162 -- reject simulation results after retained state failure -- */
}

// TraceBlock traces all transactions in the given block and returns all results
func (j *jsonRPCHub) TraceBlock(
	block *types.Block,
	tracer tracer.Tracer,
) ([]any, error) {
	if block.Number() == 0 {
		return nil, errors.New("genesis block can't have transaction")
	}

	parentHeader, ok := j.GetHeaderByHash(block.ParentHash())
	if !ok {
		return nil, errors.New("parent header not found")
	}

	blockCreator, err := j.GetConsensus().GetBlockCreator(block.Header)
	if err != nil {
		return nil, err
	}

	transition, err := j.BeginTxn(parentHeader.StateRoot, block.Header, blockCreator)
	if err != nil {
		return nil, err
	}

	transition.SetTracer(tracer)

	results := make([]any, len(block.Transactions))

	for idx, tx := range block.Transactions {
		tracer.Clear()

		/* START ALPHA CHANGE 2162 -- reject failed state reads before exporting block traces -- */
		_, applyErr := transition.Apply(tx)
		if stateErr := transition.Error(); stateErr != nil {
			return nil, stateErr
		}
		if applyErr != nil {
			return nil, applyErr
		}

		if results[idx], err = tracer.GetResult(); err != nil {
			return nil, err
		}

		/* START ALPHA CHANGE 3154 -- finalize each block trace before replaying the next transaction -- */
		if _, err = transition.FinalizeTransaction(); err != nil {
			return nil, err
		}
		/* FINISH ALPHA CHANGE 3154 -- finalize each block trace before replaying the next transaction -- */
		/* FINISH ALPHA CHANGE 2162 -- reject failed state reads before exporting block traces -- */
	}

	return results, nil
}

// TraceTxn traces a transaction in the block, associated with the given hash
func (j *jsonRPCHub) TraceTxn(
	block *types.Block,
	targetTxHash types.Hash,
	tracer tracer.Tracer,
) (any, error) {
	if block.Number() == 0 {
		return nil, errors.New("genesis block can't have transaction")
	}

	parentHeader, ok := j.GetHeaderByHash(block.ParentHash())
	if !ok {
		return nil, errors.New("parent header not found")
	}

	blockCreator, err := j.GetConsensus().GetBlockCreator(block.Header)
	if err != nil {
		return nil, err
	}

	transition, err := j.BeginTxn(parentHeader.StateRoot, block.Header, blockCreator)
	if err != nil {
		return nil, err
	}

	var targetTx *types.Transaction

	for _, tx := range block.Transactions {
		if tx.Hash == targetTxHash {
			targetTx = tx

			break
		}

		/* START ALPHA CHANGE 2162 -- stop trace replay after a local state failure -- */
		// Execute transactions without tracer until reaching the target transaction.
		_, applyErr := transition.Apply(tx)
		if stateErr := transition.Error(); stateErr != nil {
			return nil, stateErr
		}
		if applyErr != nil {
			return nil, applyErr
		}

		/* START ALPHA CHANGE 3154 -- finalize preceding transactions before tracing the target -- */
		if _, err := transition.FinalizeTransaction(); err != nil {
			return nil, err
		}
		/* FINISH ALPHA CHANGE 3154 -- finalize preceding transactions before tracing the target -- */
		/* FINISH ALPHA CHANGE 2162 -- stop trace replay after a local state failure -- */
	}

	if targetTx == nil {
		return nil, errors.New("target tx not found")
	}

	transition.SetTracer(tracer)

	/* START ALPHA CHANGE 2162 -- reject failed state reads before exporting transaction traces -- */
	_, applyErr := transition.Apply(targetTx)
	if stateErr := transition.Error(); stateErr != nil {
		return nil, stateErr
	}
	if applyErr != nil {
		return nil, applyErr
	}

	/* START ALPHA CHANGE 3154 -- capture the target trace before transaction cleanup -- */
	result, err := tracer.GetResult()
	if err != nil {
		return nil, err
	}
	if _, err = transition.FinalizeTransaction(); err != nil {
		return nil, err
	}

	return result, nil
	/* FINISH ALPHA CHANGE 3154 -- capture the target trace before transaction cleanup -- */
	/* FINISH ALPHA CHANGE 2162 -- reject failed state reads before exporting transaction traces -- */
}

func (j *jsonRPCHub) TraceCall(
	tx *types.Transaction,
	parentHeader *types.Header,
	tracer tracer.Tracer,
) (any, error) {
	blockCreator, err := j.GetConsensus().GetBlockCreator(parentHeader)
	if err != nil {
		return nil, err
	}

	transition, err := j.BeginTxn(parentHeader.StateRoot, parentHeader, blockCreator)
	if err != nil {
		return nil, err
	}

	transition.SetTracer(tracer)

	/* START ALPHA CHANGE 2162 -- reject failed state reads before exporting call traces -- */
	_, applyErr := transition.Apply(tx)
	if stateErr := transition.Error(); stateErr != nil {
		return nil, stateErr
	}
	if applyErr != nil {
		return nil, applyErr
	}

	return tracer.GetResult()
	/* FINISH ALPHA CHANGE 2162 -- reject failed state reads before exporting call traces -- */
}

/* START ALPHA CHANGE 3268 -- report the retained consensus synchronization progression -- */
func (j *jsonRPCHub) GetSyncProgression() *progress.Progression {
	return j.Consensus.GetSyncProgression()
}

/* FINISH ALPHA CHANGE 3268 -- report the retained consensus synchronization progression -- */

// SETUP //

// setupJSONRCP sets up the JSONRPC server, using the set configuration
func (s *Server) setupJSONRPC() error {
	hub := &jsonRPCHub{
		/* START ALPHA CHANGE 3268 -- wire the RPC hub without legacy restore state -- */
		state:      s.state,
		Blockchain: s.blockchain,
		TxPool:     s.txpool,
		Executor:   s.executor,
		Consensus:  s.consensus,
		Server:     s.network,
		/* FINISH ALPHA CHANGE 3268 -- wire the RPC hub without legacy restore state -- */
		/* START ALPHA CHANGE 423 -- stop wiring bridge provider into JSON-RPC hub -- */
		/* FINISH ALPHA CHANGE 423 -- stop wiring bridge provider into JSON-RPC hub -- */
		GasStore: s.gasHelper,
	}

	conf := &jsonrpc.Config{
		Store:                    hub,
		Addr:                     s.config.JSONRPC.JSONRPCAddr,
		ChainID:                  uint64(s.config.Chain.Params.ChainID),
		ChainName:                s.chain.Name,
		AccessControlAllowOrigin: s.config.JSONRPC.AccessControlAllowOrigin,
		PriceLimit:               s.config.PriceLimit,
		BatchLengthLimit:         s.config.JSONRPC.BatchLengthLimit,
		BlockRangeLimit:          s.config.JSONRPC.BlockRangeLimit,
		/* START ALPHA CHANGE 2456 -- share the effective percentile limit with RPC validation -- */
		FeeHistoryPercentileLimit: s.config.JSONRPC.FeeHistoryPercentileLimit,
		/* FINISH ALPHA CHANGE 2456 -- share the effective percentile limit with RPC validation -- */
		ConcurrentRequestsDebug: s.config.JSONRPC.ConcurrentRequestsDebug,
		WebSocketReadLimit:      s.config.JSONRPC.WebSocketReadLimit,
		/* START ALPHA CHANGE 2249 -- install configured RPC admission limits -- */
		Admission: s.config.JSONRPC.Admission,
		/* FINISH ALPHA CHANGE 2249 -- install configured RPC admission limits -- */
	}

	srv, err := jsonrpc.NewJSONRPC(s.logger, conf)
	if err != nil {
		return err
	}

	s.jsonrpcServer = srv

	return nil
}

/* setupGRPC registers the operator service and starts its loopback listener. */
func (s *Server) setupGRPC() error {
	/* START ALPHA CHANGE 2561 -- enforce loopback binding at the listener boundary -- */
	if err := validateOperatorGRPCAddress(s.config.GRPCAddr); err != nil {
		return err
	}

	listenAddress := s.config.GRPCAddr.String()
	/* FINISH ALPHA CHANGE 2561 -- enforce loopback binding at the listener boundary -- */

	proto.RegisterSystemServer(s.grpcServer, &systemService{server: s})

	lis, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return err
	}

	// Start server with infinite retries
	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			s.logger.Error(err.Error())
		}
	}()

	s.logger.Info("GRPC server running", "addr", s.config.GRPCAddr.String())

	return nil
}

// Chain returns the chain object of the client
func (s *Server) Chain() *chain.Chain {
	return s.chain
}

// JoinPeer attempts to add a new peer to the networking server
func (s *Server) JoinPeer(rawPeerMultiaddr string) error {
	return s.network.JoinPeer(rawPeerMultiaddr)
}

/* START ALPHA CHANGE 1218 -- expose manual peer directory refresh through server -- */
func (s *Server) RefreshPeerDirectory() (*network.PeerDirectoryRefreshResult, error) {
	return s.network.RefreshPeerDirectory()
}

/* FINISH ALPHA CHANGE 1218 -- expose manual peer directory refresh through server */

/* START ALPHA CHANGE 1061 -- convert MDBX map size GiB config to bytes -- */
func mdbxMapSizeBytes(sizeGB uint64) int {
	return int(sizeGB * 1024 * 1024 * 1024)
}

/* FINISH ALPHA CHANGE 1061 -- convert MDBX map size GiB config to bytes -- */

/* START ALPHA CHANGE 1461 -- expose fatal consensus errors to the process owner -- */
type fatalErrorProvider interface {
	FatalErrorCh() <-chan error
}

func (s *Server) FatalErrorCh() <-chan error {
	if s.consensus == nil {
		return nil
	}

	provider, ok := s.consensus.(fatalErrorProvider)
	if !ok {
		return nil
	}

	return provider.FatalErrorCh()
}

/* FINISH ALPHA CHANGE 1461 -- expose fatal consensus errors to the process owner -- */

// Close closes the Minimal server (blockchain, networking, consensus)
func (s *Server) Close() {
	/* START ALPHA CHANGE 1074 -- run server close sequence only once -- */
	s.closeOnce.Do(func() {
		/* FINISH ALPHA CHANGE 1074 -- run server close sequence only once -- */
		var closeErr error

		joinCloseErr := func(component string, err error) {
			if err == nil {
				return
			}

			s.logger.Error("failed to close component", "component", component, "err", err.Error())
			closeErr = errors.Join(closeErr, fmt.Errorf("%s close failed: %w", component, err))
		}

		/* START ALPHA CHANGE 1370 -- stop external intake before closing runtime dependencies -- */
		if s.jsonrpcServer != nil {
			joinCloseErr("jsonrpc", s.jsonrpcServer.Close())
		}

		if s.grpcServer != nil {
			joinCloseErr("grpc", stopGRPCServer(s.grpcServer, 5*time.Second))
		}

		if s.prometheusServer != nil {
			joinCloseErr("prometheus", shutdownHTTPServer(s.prometheusServer, 5*time.Second))
		}
		/* FINISH ALPHA CHANGE 1370 -- stop external intake before closing runtime dependencies -- */

		if s.consensus != nil {
			joinCloseErr("consensus", s.consensus.Close())
		}

		if s.txpool != nil {
			s.txpool.Close()
		}

		if s.network != nil {
			joinCloseErr("networking", s.network.Close())
		}

		if s.blockchain != nil {
			joinCloseErr("blockchain", s.blockchain.Close())
		}

		if s.stateStorage != nil {
			joinCloseErr("trie storage", s.stateStorage.Close())
		}

		s.closeDataDogProfiler()

		if closeErr != nil {
			s.logger.Error("server shutdown completed with errors", "err", closeErr.Error())
		}
		/* START ALPHA CHANGE 1075 -- finish idempotent server close sequence -- */
	})
	/* FINISH ALPHA CHANGE 1075 -- finish idempotent server close sequence -- */
}

/* START ALPHA CHANGE 1371 -- add bounded listener shutdown helpers -- */
func shutdownHTTPServer(srv *http.Server, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		forceErr := srv.Close()
		if forceErr != nil && !errors.Is(forceErr, http.ErrServerClosed) {
			return errors.Join(err, fmt.Errorf("forced HTTP close failed: %w", forceErr))
		}

		return err
	}

	return nil
}

func stopGRPCServer(srv *grpc.Server, timeout time.Duration) error {
	done := make(chan struct{})

	go func() {
		srv.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		srv.Stop()

		select {
		case <-done:
		case <-time.After(time.Second):
		}

		return fmt.Errorf("grpc graceful stop timed out after %s", timeout)
	}
}

/* FINISH ALPHA CHANGE 1371 -- add bounded listener shutdown helpers -- */

// Entry is a consensus configuration entry
type Entry struct {
	Enabled bool
	Config  map[string]any
}

func (s *Server) startPrometheusServer(listenAddr *net.TCPAddr) *http.Server {
	srv := &http.Server{
		Addr: listenAddr.String(),
		Handler: promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer, promhttp.HandlerFor(
				prometheus.DefaultGatherer,
				promhttp.HandlerOpts{},
			),
		),
		ReadHeaderTimeout: 60 * time.Second,
	}

	s.logger.Info("Prometheus server started", "addr=", listenAddr.String())

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				s.logger.Error("Prometheus HTTP server ListenAndServe", "err", err)
			}
		}
	}()

	return srv
}

func initForkManager(engineName string, config *chain.Chain) error {
	/* START ALPHA CHANGE 2392 -- reject legacy London schedules before changing fork handlers -- */
	if err := config.ValidateLondonFix(); err != nil {
		return fmt.Errorf("unsupported chain configuration: %w", err)
	}
	/* FINISH ALPHA CHANGE 2392 -- reject legacy London schedules before changing fork handlers -- */

	var initialParams *forkmanager.ForkParams

	if factory := forkManagerInitialParamsFactory[ConsensusType(engineName)]; factory != nil {
		params, err := factory(config)
		if err != nil {
			return err
		}

		initialParams = params
	}

	fm := forkmanager.GetInstance()

	// clear everything in forkmanager (if there was something because of tests) and register initial fork
	fm.Clear()
	fm.RegisterFork(forkmanager.InitialFork, initialParams)

	// Register forks
	for name, f := range *config.Params.Forks {
		// check if fork is not supported by current edge version
		if _, found := (*chain.AllForksEnabled)[name]; !found {
			return fmt.Errorf("fork is not available: %s", name)
		}

		fm.RegisterFork(name, f.Params)
	}

	// Register handlers and additional forks here
	if err := types.RegisterTxHashFork(chain.TxHashWithType); err != nil {
		return err
	}

	// Register Handler for London fork fix
	if err := state.RegisterLondonFixFork(chain.LondonFix); err != nil {
		return err
	}

	if factory := forkManagerFactory[ConsensusType(engineName)]; factory != nil {
		if err := factory(config.Params.Forks); err != nil {
			return err
		}
	}

	// Activate initial fork
	if err := fm.ActivateFork(forkmanager.InitialFork, uint64(0)); err != nil {
		return err
	}

	// Activate forks
	for name, f := range *config.Params.Forks {
		if err := fm.ActivateFork(name, f.Block); err != nil {
			return err
		}
	}

	return nil
}
