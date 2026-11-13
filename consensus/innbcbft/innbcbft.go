/* START ALPHA CHANGE 103 -- describe INNBCBFT as sovereign consensus without bridge feature -- */
// Package innbcbft implements INNBC BFT consensus for the INNBC Chain sovereign L1.
/* FINISH ALPHA CHANGE 103 -- describe INNBCBFT as sovereign consensus without bridge feature -- */
package innbcbft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"

	/* START ALPHA CHANGE 1521 -- remove bbolt from the INNBCBFT backend contract -- */
	/* FINISH ALPHA CHANGE 1521 -- remove bbolt from the INNBCBFT backend contract -- */

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/forkmanager"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/helper/progress"
	"github.com/innovativebioresearch/innbc-chain/network"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/syncer"
	"github.com/innovativebioresearch/innbc-chain/types"
)

const (
	minSyncPeers = 2
	pbftProto    = "/pbft/0.2"
	/* START ALPHA CHANGE 327 -- remove bridge gossip protocol constant -- */
	/* FINISH ALPHA CHANGE 327 -- remove bridge gossip protocol constant -- */

	/* START ALPHA CHANGE 1441 -- bound consensus retry loops after fast failures -- */
	consensusFailureRetryDelay = 500 * time.Millisecond
	runtimeAlignmentRetryDelay = 100 * time.Millisecond
	/* FINISH ALPHA CHANGE 1441 -- bound consensus retry loops after fast failures -- */
)

/* START ALPHA CHANGE 1442 -- describe runtime-applied synchronized blocks -- */
type runtimeSyncedBlock struct {
	number uint64
	hash   types.Hash
}

/* FINISH ALPHA CHANGE 1442 -- describe runtime-applied synchronized blocks -- */

/* START ALPHA CHANGE 104 -- remove missing bridge config error from sovereign genesis path -- */
/* FINISH ALPHA CHANGE 104 -- remove missing bridge config error from sovereign genesis path -- */

// innbcbftBackend is an interface defining innbcbft methods needed by fsm and sync tracker
type innbcbftBackend interface {
	// GetValidators retrieves validator set for the given block
	GetValidators(blockNumber uint64, parents []*types.Header) (validator.AccountSet, error)

	// GetValidators retrieves validator set for the given block.
	// Function expects that the consensus state transaction is already open.
	/* START ALPHA CHANGE 1522 -- pass MDBX consensus transactions through validator lookup -- */
	GetValidatorsWithTx(blockNumber uint64, parents []*types.Header,
		dbTx *stateTx) (validator.AccountSet, error)
	/* FINISH ALPHA CHANGE 1522 -- pass MDBX consensus transactions through validator lookup -- */
}

// Factory is the factory function to create a discovery consensus
func Factory(params *consensus.Params) (consensus.Consensus, error) {
	logger := params.Logger.Named("innbcbft")

	setupHeaderHashFunc()

	/* START ALPHA CHANGE 1056 -- create cancellable sync retry context for clean shutdown -- */
	syncCtx, syncCancel := context.WithCancel(context.Background())

	innbcbft := &INNBCBFT{
		config:     params,
		closeCh:    make(chan struct{}),
		logger:     logger,
		txPool:     params.TxPool,
		syncCtx:    syncCtx,
		syncCancel: syncCancel,
		/* START ALPHA CHANGE 1445 -- initialize fatal and runtime-applied channels -- */
		fatalStopCh:          make(chan struct{}),
		fatalErrCh:           make(chan error, 1),
		runtimeSyncedBlockCh: make(chan runtimeSyncedBlock, 1),
		/* FINISH ALPHA CHANGE 1445 -- initialize fatal and runtime-applied channels -- */
	}
	/* FINISH ALPHA CHANGE 1056 -- create cancellable sync retry context for clean shutdown -- */

	// initialize innbcbft consensus config
	customConfigJSON, err := json.Marshal(params.Config.Config)
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 3314 -- validate clock configuration during direct consensus construction -- */
	if _, err := common.DecodeBlockTimeConfig(customConfigJSON); err != nil {
		syncCancel()

		return nil, err
	}
	/* FINISH ALPHA CHANGE 3314 -- validate clock configuration during direct consensus construction -- */

	err = json.Unmarshal(customConfigJSON, &innbcbft.consensusConfig)
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 2645 -- validate genesis stake in direct consensus construction -- */
	if err := innbcbft.consensusConfig.ValidatorRegistryConfig.validateGenesisMinimumStake(); err != nil {
		syncCancel()

		return nil, err
	}
	/* FINISH ALPHA CHANGE 2645 -- validate genesis stake in direct consensus construction -- */

	/* START ALPHA CHANGE 3338 -- reject missing native-token configuration during direct consensus construction -- */
	if err := ValidateNativeTokenConfig(innbcbft.consensusConfig.NativeTokenConfig); err != nil {
		syncCancel()

		return nil, err
	}
	/* FINISH ALPHA CHANGE 3338 -- reject missing native-token configuration during direct consensus construction -- */

	/* START ALPHA CHANGE 3439 -- reject unsupported epoch sizes during direct consensus construction -- */
	if err := ValidateEpochSize(innbcbft.consensusConfig.EpochSize); err != nil {
		syncCancel()

		return nil, err
	}
	/* FINISH ALPHA CHANGE 3439 -- reject unsupported epoch sizes during direct consensus construction -- */

	return innbcbft, nil
}

type INNBCBFT struct {
	// closeCh is used to signal that consensus protocol is stopped
	closeCh chan struct{}

	/* START ALPHA CHANGE 1057 -- track sync retry context for clean shutdown -- */
	// syncCtx controls the background sync retry loop.
	syncCtx context.Context

	// syncCancel stops the background sync retry loop.
	syncCancel context.CancelFunc
	/* FINISH ALPHA CHANGE 1057 -- track sync retry context for clean shutdown -- */

	/* START ALPHA CHANGE 1070 -- make INNBCBFT close idempotent -- */
	closeOnce sync.Once
	closeErr  error

	/* START ALPHA CHANGE 1374 -- wait for INNBCBFT background goroutines on close -- */
	closeWG sync.WaitGroup

	/* START ALPHA CHANGE 1429 -- gate consensus work before shutdown waiting begins -- */
	closeLock sync.Mutex
	closing   bool
	/* FINISH ALPHA CHANGE 1429 -- gate consensus work before shutdown waiting begins -- */
	/* FINISH ALPHA CHANGE 1374 -- wait for INNBCBFT background goroutines on close -- */
	/* FINISH ALPHA CHANGE 1070 -- make INNBCBFT close idempotent -- */

	/* START ALPHA CHANGE 1443 -- report and internally stop on fatal consensus errors -- */
	fatalOnce   sync.Once
	fatalStopCh chan struct{}
	fatalErrCh  chan error
	/* FINISH ALPHA CHANGE 1443 -- report and internally stop on fatal consensus errors -- */

	// ibft is the ibft engine
	ibft *IBFTConsensusWrapper

	// state is reference to the struct which encapsulates consensus data persistence logic
	state *State

	// consensus parameters
	config *consensus.Params

	// consensusConfig is genesis configuration for innbcbft consensus protocol
	consensusConfig *INNBCBFTConfig

	// blockchain is a reference to the blockchain object
	blockchain blockchainBackend

	// runtime handles consensus runtime features like epoch, state and event management
	runtime *consensusRuntime

	// block time duration
	blockTime time.Duration

	// dataDir is the data directory to store the info
	dataDir string

	// reference to the syncer
	syncer syncer.Syncer

	/* START ALPHA CHANGE 1444 -- notify consensus after synchronized runtime advancement -- */
	runtimeSyncedBlockCh chan runtimeSyncedBlock
	/* FINISH ALPHA CHANGE 1444 -- notify consensus after synchronized runtime advancement -- */

	// topic for consensus engine messages
	consensusTopic *network.Topic

	/* START ALPHA CHANGE 328 -- remove bridge gossip topic from INNBCBFT -- */
	/* FINISH ALPHA CHANGE 328 -- remove bridge gossip topic from INNBCBFT -- */

	// key encapsulates ECDSA address and BLS signing logic
	key *wallet.Key

	// validatorsCache represents cache of validators snapshots
	validatorsCache *validatorsSnapshotCache

	// logger
	logger hclog.Logger

	// tx pool as interface
	txPool txPoolInterface
}

func GenesisPostHookFactory(config *chain.Chain, engineName string) func(txn *state.Transition) error {
	return func(transition *state.Transition) error {
		innbcBFTConfig, err := GetINNBCBFTConfig(config)
		if err != nil {
			return err
		}

		/* START ALPHA CHANGE 3105 -- validate genesis hook allocations and omit absent balances from initial supply -- */
		if err := config.ValidateGenesisAllocations(); err != nil {
			return err
		}

		// The zero address remains excluded from initial native-token supply.
		// An omitted balance contributes zero without changing the allocation.
		initialTotalSupply := big.NewInt(0)

		for addr, alloc := range config.Genesis.Alloc {
			if addr == types.ZeroAddress || alloc.Balance == nil {
				continue
			}

			initialTotalSupply.Add(initialTotalSupply, alloc.Balance)
		}
		/* FINISH ALPHA CHANGE 3105 -- validate genesis hook allocations and omit absent balances from initial supply -- */

		/* START ALPHA CHANGE 105 -- remove mandatory rootchain bridge config from genesis startup -- */
		proxyAddrMapping := contracts.GetProxyImplementationMapping()
		/* FINISH ALPHA CHANGE 105 -- remove mandatory rootchain bridge config from genesis startup -- */

		/* START ALPHA CHANGE 488 -- remove legacy EIP1559 bridge burn proxy mapping -- */
		/* FINISH ALPHA CHANGE 488 -- remove legacy EIP1559 bridge burn proxy mapping -- */

		/* START ALPHA CHANGE 558 -- remove custom reward-token proxy mapping from genesis startup -- */
		/* FINISH ALPHA CHANGE 558 -- remove custom reward-token proxy mapping from genesis startup -- */

		/* START ALPHA CHANGE 992 -- initialize system proxies under isolated proxy admin executor -- */
		if err = initProxies(transition, contracts.INNBCProxyAdminExecutorContract, proxyAddrMapping); err != nil {
			return err
		}

		if err = initProxyAdminExecutor(transition); err != nil {
			return err
		}

		if err = initProxyAdminGovernance(transition); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 992 -- initialize system proxies under isolated proxy admin executor -- */

		/* START ALPHA CHANGE 102 -- initialize INNBC validator registry after genesis proxy setup -- */
		if err = initValidatorRegistry(innbcBFTConfig, transition); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 102 -- initialize INNBC validator registry after genesis proxy setup -- */

		/* START ALPHA CHANGE 275 -- initialize INNBC validator governance after registry setup -- */
		if err = initValidatorGovernance(innbcBFTConfig, transition); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 275 -- initialize INNBC validator governance after registry setup -- */

		// initialize ValidatorSet SC
		if err = initValidatorSet(innbcBFTConfig, transition); err != nil {
			return err
		}

		/* START ALPHA CHANGE 559 -- remove pre-funded reward pool genesis startup hooks -- */
		/* FINISH ALPHA CHANGE 559 -- remove pre-funded reward pool genesis startup hooks -- */

		/* START ALPHA CHANGE 106 -- remove Polygon bridge predicate initialization from genesis startup -- */
		/* FINISH ALPHA CHANGE 106 -- remove Polygon bridge predicate initialization from genesis startup -- */

		/* START ALPHA CHANGE 780 -- always initialize INNBC native token governance-issued implementation -- */
		params := &contractsapi.InitializeINNBCNativeTokenFn{
			Predicate_:         types.ZeroAddress,
			BlockRewardIssuer_: contracts.INNBCBlockRewardGovernanceContract,
			RootToken_:         types.ZeroAddress,
			Name_:              innbcBFTConfig.NativeTokenConfig.Name,
			Symbol_:            innbcBFTConfig.NativeTokenConfig.Symbol,
			Decimals_:          innbcBFTConfig.NativeTokenConfig.Decimals,
			TokenSupply_:       initialTotalSupply,
		}

		input, err := params.EncodeAbi()
		if err != nil {
			return err
		}

		if err = callContract(contracts.SystemCaller,
			contracts.NativeERC20TokenContract, input, "INNBCNativeToken", transition); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 780 -- always initialize INNBC native token governance-issued implementation -- */

		/* START ALPHA CHANGE 805 -- initialize INNBC fee burn vault after native token setup -- */
		if err = initFeeBurnVault(transition); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 805 -- initialize INNBC fee burn vault after native token setup -- */

		/* START ALPHA CHANGE 962 -- initialize INNBC community reward pool before block reward governance -- */
		if err = initCommunityRewardPool(innbcBFTConfig, transition); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 962 -- initialize INNBC community reward pool before block reward governance -- */

		/* START ALPHA CHANGE 277 -- initialize INNBC block reward governance after native token setup -- */
		if err = initBlockRewardGovernance(innbcBFTConfig, transition); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 277 -- initialize INNBC block reward governance after native token setup -- */

		/* START ALPHA CHANGE 246 -- initialize INNBC treasury DAO after native token setup -- */
		if err = initTreasuryDAO(transition); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 246 -- initialize INNBC treasury DAO after native token setup -- */

		return nil
	}
}

func ForkManagerFactory(forks *chain.Forks) error {
	// place fork manager handler registration here
	return nil
}

/* START ALPHA CHANGE 3890 -- calculate synchronization timeouts without duration overflow -- */
// syncBlockTimeout triples a block interval expressed in whole seconds,
// saturating at the maximum representable duration.
func syncBlockTimeout(blockTimeSeconds uint64) time.Duration {
	const maximum = time.Duration(1<<63 - 1)
	const durationPerSecond = 3 * time.Second

	// Check before converting or multiplying.
	if blockTimeSeconds > uint64(maximum/durationPerSecond) {
		return maximum
	}

	return time.Duration(blockTimeSeconds) * durationPerSecond
}

/* FINISH ALPHA CHANGE 3890 -- calculate synchronization timeouts without duration overflow -- */

// Initialize initializes the consensus (e.g. setup data)
func (p *INNBCBFT) Initialize() error {
	p.logger.Info("initializing innbcbft...")

	/* START ALPHA CHANGE 2011 -- validate proved genesis metadata before initializing consensus snapshots -- */
	genesisHeader, ok := p.config.Blockchain.GetHeaderByNumber(0)
	if !ok || genesisHeader == nil {
		return fmt.Errorf("genesis header is unavailable")
	}
	if err := ValidateGenesisExtraData(
		p.consensusConfig,
		genesisHeader.ExtraData,
	); err != nil {
		return fmt.Errorf("invalid INNBCBFT genesis: %w", err)
	}
	/* FINISH ALPHA CHANGE 2011 -- validate proved genesis metadata before initializing consensus snapshots -- */

	// read account
	account, err := wallet.NewAccountFromSecret(p.config.SecretsManager)
	if err != nil {
		return fmt.Errorf("failed to read account data. Error: %w", err)
	}

	// set key
	p.key = wallet.NewKey(account)

	/* START ALPHA CHANGE 3891 -- pass an overflow-safe block-download timeout to synchronization -- */
	// create and set syncer
	p.syncer = syncer.NewSyncer(
		p.config.Logger.Named("syncer"),
		p.config.Network,
		p.config.Blockchain,
		syncBlockTimeout(p.config.BlockTime),
	)
	/* FINISH ALPHA CHANGE 3891 -- pass an overflow-safe block-download timeout to synchronization -- */

	// set blockchain backend
	p.blockchain = &blockchainWrapper{
		blockchain: p.config.Blockchain,
		executor:   p.config.Executor,
	}

	// create bridge and consensus topics
	if err = p.createTopics(); err != nil {
		return fmt.Errorf("cannot create topics: %w", err)
	}

	// set block time
	p.blockTime = time.Duration(p.config.BlockTime)

	/* START ALPHA CHANGE 1631 -- use the dedicated consensus database directory directly -- */
	p.dataDir = p.config.Config.Path
	// create the data dir if not exists
	if err = common.CreateDirSafe(p.dataDir, 0750); err != nil {
		/* FINISH ALPHA CHANGE 1631 -- use the dedicated consensus database directory directly -- */
		return fmt.Errorf("failed to create data directory. Error: %w", err)
	}

	/*START ALPHA CHANGE 57 -- update state constructor call after logger parameter removal --*/
	stt, err := newState(filepath.Join(p.dataDir, stateFileName), p.closeCh)
	/*FINISH ALPHA CHANGE 57 -- update state constructor call after logger parameter removal --*/
	if err != nil {
		return fmt.Errorf("failed to create state instance. Error: %w", err)
	}

	p.state = stt
	p.validatorsCache = newValidatorsSnapshotCache(p.config.Logger, stt, p.blockchain)

	// create runtime
	if err := p.initRuntime(); err != nil {
		return err
	}

	p.ibft = newIBFTConsensusWrapper(p.logger, p.runtime, p)

	if err = p.subscribeToIbftTopic(); err != nil {
		return fmt.Errorf("IBFT topic subscription failed: %w", err)
	}

	return nil
}

func ForkManagerInitialParamsFactory(config *chain.Chain) (*forkmanager.ForkParams, error) {
	pbftConfig, err := GetINNBCBFTConfig(config)
	if err != nil {
		return nil, err
	}

	return &forkmanager.ForkParams{
		MaxValidatorSetSize: &pbftConfig.MaxValidatorSetSize,
		EpochSize:           &pbftConfig.EpochSize,
		SprintSize:          &pbftConfig.SprintSize,
		/* START ALPHA CHANGE 3314 -- omit retired drift from initial fork parameters -- */
		BlockTime: &pbftConfig.BlockTime,
		/* FINISH ALPHA CHANGE 3314 -- omit retired drift from initial fork parameters -- */
	}, nil
}

/* START ALPHA CHANGE 1430 -- register consensus runtime work safely -- */
func (p *INNBCBFT) enterCloseWork() bool {
	p.closeLock.Lock()
	defer p.closeLock.Unlock()

	if p.closing {
		return false
	}

	p.closeWG.Add(1)

	return true
}

func (p *INNBCBFT) leaveCloseWork() {
	p.closeWG.Done()
}

func (p *INNBCBFT) startCloseWorker(fn func()) bool {
	if !p.enterCloseWork() {
		return false
	}

	go func() {
		defer p.leaveCloseWork()

		fn()
	}()

	return true
}

/* FINISH ALPHA CHANGE 1430 -- register consensus runtime work safely -- */

/* START ALPHA CHANGE 1446 -- publish fatal consensus errors exactly once -- */
func (p *INNBCBFT) FatalErrorCh() <-chan error {
	return p.fatalErrCh
}

func (p *INNBCBFT) ReportFatalError(err error) {
	if err == nil {
		return
	}

	p.fatalOnce.Do(func() {
		if p.logger != nil {
			p.logger.Error("fatal consensus error", "err", err)
		}

		if p.syncCancel != nil {
			p.syncCancel()
		}

		if p.fatalStopCh != nil {
			close(p.fatalStopCh)
		}

		if p.fatalErrCh != nil {
			select {
			case p.fatalErrCh <- err:
			default:
			}
		}
	})
}

func (p *INNBCBFT) notifyRuntimeSyncedBlockApplied(fullBlock *types.FullBlock) {
	if fullBlock == nil || fullBlock.Block == nil || fullBlock.Block.Header == nil {
		return
	}

	notification := runtimeSyncedBlock{
		number: fullBlock.Block.Number(),
		hash:   fullBlock.Block.Hash(),
	}

	select {
	case p.runtimeSyncedBlockCh <- notification:
	case <-p.closeCh:
	case <-p.fatalStopCh:
	default:
	}
}

func (p *INNBCBFT) waitForConsensusRetry(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-p.closeCh:
		return false
	case <-p.fatalStopCh:
		return false
	}
}

/* FINISH ALPHA CHANGE 1446 -- publish fatal consensus errors exactly once -- */

// Start starts the consensus and servers
func (p *INNBCBFT) Start() error {
	p.logger.Info("starting innbcbft consensus", "signer", p.key.String())

	// start syncer (also initializes peer map)
	if err := p.syncer.Start(); err != nil {
		return fmt.Errorf("failed to start syncer. Error: %w", err)
	}

	// sync concurrently, retrying indefinitely
	/* START ALPHA CHANGE 1058 -- stop sync retry loop during consensus shutdown -- */
	/* START ALPHA CHANGE 1375 -- track sync retry loop shutdown completion -- */
	/* START ALPHA CHANGE 1431 -- admit sync retry worker through shutdown gate -- */
	if !p.startCloseWorker(func() {
		common.RetryForever(p.syncCtx, time.Second, func(context.Context) error {
			/* START ALPHA CHANGE 1447 -- stop sync after fatal runtime advancement failure -- */
			blockHandler := func(b *types.FullBlock) bool {
				advanced, err := p.runtime.OnBlockInserted(b)
				if err != nil {
					p.ReportFatalError(
						fmt.Errorf(
							"failed to advance consensus runtime after synchronized block %d: %w",
							b.Block.Number(),
							err,
						),
					)

					return true
				}

				if advanced {
					p.notifyRuntimeSyncedBlockApplied(b)
				}

				return false
			}
			/* FINISH ALPHA CHANGE 1447 -- stop sync after fatal runtime advancement failure -- */
			if err := p.syncer.Sync(blockHandler); err != nil {
				p.logger.Error("blocks synchronization failed", "error", err)

				return err
			}

			return nil
		})
	}) {
		return errors.New("innbcbft is shutting down")
	}
	/* FINISH ALPHA CHANGE 1431 -- admit sync retry worker through shutdown gate -- */
	/* FINISH ALPHA CHANGE 1375 -- track sync retry loop shutdown completion -- */
	/* FINISH ALPHA CHANGE 1058 -- stop sync retry loop during consensus shutdown -- */

	// start consensus runtime
	if err := p.startRuntime(); err != nil {
		return fmt.Errorf("consensus runtime start failed: %w", err)
	}

	// start state DB process
	/* START ALPHA CHANGE 1376 -- track INNBCBFT stats loop shutdown completion -- */
	/* START ALPHA CHANGE 1432 -- admit state stats worker through shutdown gate -- */
	if !p.startCloseWorker(p.state.startStatsReleasing) {
		return errors.New("innbcbft is shutting down")
	}
	/* FINISH ALPHA CHANGE 1432 -- admit state stats worker through shutdown gate -- */
	/* FINISH ALPHA CHANGE 1376 -- track INNBCBFT stats loop shutdown completion -- */

	/* START ALPHA CHANGE 153 -- prune legacy rootchain metrics startup from sovereign INNBC runtime -- */
	/* INNBC Chain does not publish rootchain bridge metrics in sovereign mode. */
	/* FINISH ALPHA CHANGE 153 -- prune legacy rootchain metrics startup from sovereign INNBC runtime -- */

	return nil
}

// initRuntime creates consensus runtime
func (p *INNBCBFT) initRuntime() error {
	runtimeConfig := &runtimeConfig{
		INNBCBFTConfig:  p.consensusConfig,
		Key:             p.key,
		DataDir:         p.dataDir,
		State:           p.state,
		blockchain:      p.blockchain,
		innbcbftBackend: p,
		txPool:          p.txPool,
		/* START ALPHA CHANGE 1456 -- connect runtime fatal failures to consensus owner -- */
		reportFatalError: p.ReportFatalError,
		/* FINISH ALPHA CHANGE 1456 -- connect runtime fatal failures to consensus owner -- */
		/* START ALPHA CHANGE 324 -- remove bridge topic from runtime config construction -- */
		numBlockConfirmations: p.config.NumBlockConfirmations,
		/* FINISH ALPHA CHANGE 324 -- remove bridge topic from runtime config construction -- */
		consensusConfig: p.config.Config,
	}

	runtime, err := newConsensusRuntime(p.logger, runtimeConfig)
	if err != nil {
		return err
	}

	p.runtime = runtime

	return nil
}

// startRuntime starts consensus runtime
func (p *INNBCBFT) startRuntime() error {
	/* START ALPHA CHANGE 1377 -- track consensus protocol loop shutdown completion -- */
	/* START ALPHA CHANGE 1433 -- admit consensus protocol through shutdown gate -- */
	if !p.startCloseWorker(p.startConsensusProtocol) {
		return errors.New("innbcbft is shutting down")
	}
	/* FINISH ALPHA CHANGE 1433 -- admit consensus protocol through shutdown gate -- */
	/* FINISH ALPHA CHANGE 1377 -- track consensus protocol loop shutdown completion -- */

	return nil
}

/* START ALPHA CHANGE 2193 -- use the consensus local-failure budget during FSM preparation -- */
func (p *INNBCBFT) logFSMPreparationError(
	height uint64,
	err error,
	lastOrdinaryLog *time.Time,
) {
	if err == nil || errors.Is(err, errRuntimeHeadChanged) {
		return
	}

	if state.LogLocalStateFailure(p.logger, state.LocalStateLogConsensus, err) {
		return
	}

	if time.Since(*lastOrdinaryLog) < time.Second {
		return
	}

	p.logger.Error(
		"failed to prepare consensus FSM",
		"block number", height,
		"error", err,
	)
	*lastOrdinaryLog = time.Now()
}

/* FINISH ALPHA CHANGE 2193 -- use the consensus local-failure budget during FSM preparation -- */

func (p *INNBCBFT) startConsensusProtocol() {
	/* START ALPHA CHANGE 155 -- use validator-aware peer wait for sovereign INNBC bootstrap -- */
	if !p.waitForValidatorPeers() {
		return
	}

	p.logger.Debug("required validator peers connected")
	/* FINISH ALPHA CHANGE 155 -- use validator-aware peer wait for sovereign INNBC bootstrap -- */

	/* START ALPHA CHANGE 1449 -- cancel only after synchronized runtime advancement -- */
	syncerBlockCh := p.runtimeSyncedBlockCh
	/* FINISH ALPHA CHANGE 1449 -- cancel only after synchronized runtime advancement -- */

	var (
		sequenceCh   <-chan struct{}
		stopSequence func()
	)

	/* START ALPHA CHANGE 1452 -- require stable runtime and blockchain alignment before consensus -- */
	var (
		lastHeightMismatchLog time.Time
		lastFSMErrorLog       time.Time
	)

	for {
		/* START ALPHA CHANGE 2586 -- stop before replacing a failed committed decision -- */
		if err := p.ibft.SequenceFailure(); err != nil {
			p.ReportFatalError(err)

			return
		}
		/* FINISH ALPHA CHANGE 2586 -- stop before replacing a failed committed decision -- */

		latestHeader := p.blockchain.CurrentHeader()

		if latestHeader == nil || !p.runtime.IsAlignedWith(latestHeader) {
			if time.Since(lastHeightMismatchLog) >= time.Second {
				runtimeHeight := uint64(0)
				runtimeHash := types.ZeroHash

				p.runtime.lock.RLock()
				if p.runtime.lastBuiltBlock != nil {
					runtimeHeight = p.runtime.lastBuiltBlock.Number
					runtimeHash = p.runtime.lastBuiltBlock.Hash
				}
				p.runtime.lock.RUnlock()

				blockchainHeight := uint64(0)
				blockchainHash := types.ZeroHash
				if latestHeader != nil {
					blockchainHeight = latestHeader.Number
					blockchainHash = latestHeader.Hash
				}

				p.logger.Warn(
					"waiting for consensus runtime to match blockchain head",
					"runtimeHeight", runtimeHeight,
					"runtimeHash", runtimeHash,
					"blockchainHeight", blockchainHeight,
					"blockchainHash", blockchainHash,
				)

				lastHeightMismatchLog = time.Now()
			}

			if !p.waitForConsensusRetry(runtimeAlignmentRetryDelay) {
				return
			}

			continue
		}

		/* START ALPHA CHANGE 160 -- use initialized runtime epoch validators during consensus loop -- */
		p.runtime.lock.RLock()
		currentValidators := p.runtime.epoch.Validators
		p.runtime.lock.RUnlock()
		/* FINISH ALPHA CHANGE 160 -- use initialized runtime epoch validators during consensus loop -- */
		/* FINISH ALPHA CHANGE 1452 -- require stable runtime and blockchain alignment before consensus -- */

		isValidator := currentValidators.ContainsNodeID(p.key.String())
		p.runtime.setIsActiveValidator(isValidator)

		p.txPool.SetSealing(isValidator) // update tx pool

		if isValidator {
			/* START ALPHA CHANGE 1453 -- start sequence from the exact prepared FSM height -- */
			sequenceHeight, err := p.runtime.PrepareFSM(latestHeader)
			if err != nil {
				if errors.Is(err, errNoActiveRegistryValidators) {
					p.ReportFatalError(
						fmt.Errorf(
							"INNBC validator registry has no active validators at block %d: %w",
							latestHeader.Number,
							err,
						),
					)

					return
				}

				/* START ALPHA CHANGE 2193 -- distinguish local failures from ordinary FSM diagnostics -- */
				p.logFSMPreparationError(
					latestHeader.Number,
					err,
					&lastFSMErrorLog,
				)
				/* FINISH ALPHA CHANGE 2193 -- distinguish local failures from ordinary FSM diagnostics -- */

				if !p.waitForConsensusRetry(consensusFailureRetryDelay) {
					return
				}

				continue
			}

			currentHeader := p.blockchain.CurrentHeader()
			if !sameBlockHeader(currentHeader, latestHeader) {
				if !p.waitForConsensusRetry(runtimeAlignmentRetryDelay) {
					return
				}

				continue
			}

			sequenceCh, stopSequence = p.ibft.runSequence(sequenceHeight)
			/* FINISH ALPHA CHANGE 1453 -- start sequence from the exact prepared FSM height -- */
		}

		now := time.Now().UTC()
		sequenceFailedFast := false
		/* START ALPHA CHANGE 944 -- periodically recheck idle non-validator consensus status -- */
		var idleRecheckCh <-chan time.Time
		if !isValidator {
			idleRecheckCh = time.After(2 * time.Second)
		}
		/* FINISH ALPHA CHANGE 944 -- periodically recheck idle non-validator consensus status -- */
		/* START ALPHA CHANGE 1454 -- stop on fatal errors and back off fast sequence failures -- */
		select {
		case appliedBlock := <-syncerBlockCh:
			if isValidator {
				stopSequence()
				p.logger.Info(
					"canceled sequence after synchronized runtime advancement",
					"sequence", latestHeader.Number+1,
					"appliedBlock", appliedBlock.number,
					"appliedHash", appliedBlock.hash,
				)
			}
		case <-sequenceCh:
			/* START ALPHA CHANGE 2586 -- propagate committed-proposal failures to the runtime supervisor -- */
			if err := p.ibft.SequenceFailure(); err != nil {
				p.ReportFatalError(err)

				return
			}
			/* FINISH ALPHA CHANGE 2586 -- propagate committed-proposal failures to the runtime supervisor -- */

			sequenceFailedFast = time.Since(now) < consensusFailureRetryDelay
		case <-idleRecheckCh:
			/* START ALPHA CHANGE 944 -- periodically recheck idle non-validator consensus status -- */
			p.logger.Debug("rechecking validator status while consensus is idle")
			/* FINISH ALPHA CHANGE 944 -- periodically recheck idle non-validator consensus status -- */
		case <-p.fatalStopCh:
			if isValidator {
				stopSequence()
			}

			return
		case <-p.closeCh:
			if isValidator {
				stopSequence()
			}

			return
		}

		elapsed := time.Since(now)
		p.logger.Debug("time to run the sequence", "seconds", elapsed)

		if sequenceFailedFast {
			if !p.waitForConsensusRetry(consensusFailureRetryDelay - elapsed) {
				return
			}
		}
		/* FINISH ALPHA CHANGE 1454 -- stop on fatal errors and back off fast sequence failures -- */
	}
}

/* START ALPHA CHANGE 156 -- derive startup peer wait from current validator set -- */
func (p *INNBCBFT) waitForValidatorPeers() bool {
	requiredPeers := p.requiredValidatorPeers()

	if requiredPeers == 0 {
		p.logger.Info("single-validator bootstrap detected; starting consensus without peer wait")

		return true
	}

	for {
		timer := time.NewTimer(2 * time.Second)

		select {
		case <-p.closeCh:
			timer.Stop()

			return false
		case <-p.fatalStopCh:
			timer.Stop()

			return false
		case <-timer.C:
		}

		/* START ALPHA CHANGE 3830 -- refresh startup peer requirements after synchronized committee changes -- */
		requiredPeers = p.requiredValidatorPeers()
		/* FINISH ALPHA CHANGE 3830 -- refresh startup peer requirements after synchronized committee changes -- */

		connectedPeers := len(p.config.Network.Peers())
		if connectedPeers >= requiredPeers {
			break
		}

		p.logger.Debug(
			"waiting for validator peers before starting consensus",
			"connectedPeers", connectedPeers,
			"requiredPeers", requiredPeers,
		)
	}

	return true
}

func (p *INNBCBFT) requiredValidatorPeers() int {
	/* START ALPHA CHANGE 159 -- derive peer wait from initialized runtime epoch validators -- */
	p.runtime.lock.RLock()
	currentValidators := p.runtime.epoch.Validators
	p.runtime.lock.RUnlock()

	validatorPeers := currentValidators.Len() - 1
	if validatorPeers < 0 {
		return 0
	}

	if validatorPeers < minSyncPeers {
		return validatorPeers
	}

	return minSyncPeers
	/* FINISH ALPHA CHANGE 159 -- derive peer wait from initialized runtime epoch validators -- */
}

/* FINISH ALPHA CHANGE 156 -- derive startup peer wait from current validator set -- */

// Close closes the connection
func (p *INNBCBFT) Close() error {
	/* START ALPHA CHANGE 1071 -- close INNBCBFT resources only once -- */
	p.closeOnce.Do(func() {
		/* START ALPHA CHANGE 1059 -- cancel sync retry loop before closing syncer -- */
		if p.syncCancel != nil {
			p.syncCancel()
		}

		/* START ALPHA CHANGE 1435 -- stop admission before waiting and closing consensus resources -- */
		p.closeLock.Lock()
		p.closing = true
		close(p.closeCh)
		p.closeLock.Unlock()
		/* FINISH ALPHA CHANGE 1435 -- stop admission before waiting and closing consensus resources -- */

		if p.syncer != nil {
			if err := p.syncer.Close(); err != nil {
				p.closeErr = errors.Join(p.closeErr, err)
			}
		}
		/* FINISH ALPHA CHANGE 1059 -- cancel sync retry loop before closing syncer -- */

		/* START ALPHA CHANGE 1379 -- wait before closing consensus runtime resources -- */
		p.closeWG.Wait()

		/* START ALPHA CHANGE 1436 -- close consensus topic, runtime, and persistent state -- */
		if p.consensusTopic != nil {
			p.consensusTopic.Close()
		}

		if p.runtime != nil {
			p.runtime.close()
		}

		if p.state != nil {
			if err := p.state.Close(); err != nil {
				p.closeErr = errors.Join(
					p.closeErr,
					fmt.Errorf("failed to close consensus state: %w", err),
				)
			}
		}
		/* FINISH ALPHA CHANGE 1436 -- close consensus topic, runtime, and persistent state -- */
		/* FINISH ALPHA CHANGE 1379 -- wait before closing consensus runtime resources -- */
	})

	return p.closeErr
	/* FINISH ALPHA CHANGE 1071 -- close INNBCBFT resources only once -- */
}

// GetSyncProgression retrieves the current sync progression, if any
func (p *INNBCBFT) GetSyncProgression() *progress.Progression {
	return p.syncer.GetSyncProgression()
}

// VerifyHeader implements consensus.Engine and checks whether a header conforms to the consensus rules
func (p *INNBCBFT) VerifyHeader(header *types.Header) error {
	// Short circuit if the header is known
	if _, ok := p.blockchain.GetHeaderByHash(header.Hash); ok {
		return nil
	}

	parent, ok := p.blockchain.GetHeaderByHash(header.ParentHash)
	if !ok {
		return fmt.Errorf(
			"unable to get parent header by hash for block number %d",
			header.Number,
		)
	}

	/* START ALPHA CHANGE 3314 -- verify finalized timestamps against the same configured interval -- */
	return p.verifyHeaderImpl(parent, header, nil)
}

func (p *INNBCBFT) verifyHeaderImpl(parent, header *types.Header, parents []*types.Header) error {
	// validate header fields
	if err := validateHeaderFields(parent, header, p.consensusConfig.BlockTime.Duration); err != nil {
		/* FINISH ALPHA CHANGE 3314 -- verify finalized timestamps against the same configured interval -- */
		return fmt.Errorf("failed to validate header for block %d. error = %w", header.Number, err)
	}

	// decode the extra data
	extra, err := GetIbftExtra(header.ExtraData)
	if err != nil {
		return fmt.Errorf("failed to verify header for block %d. get extra error = %w", header.Number, err)
	}

	// validate extra data
	return extra.ValidateFinalizedData(
		header, parent, parents, p.blockchain.GetChainID(), p, signer.DomainCheckpointManager, p.logger)
}

func (p *INNBCBFT) GetValidators(blockNumber uint64, parents []*types.Header) (validator.AccountSet, error) {
	return p.validatorsCache.GetSnapshot(blockNumber, parents, nil)
}

func (p *INNBCBFT) GetValidatorsWithTx(blockNumber uint64, parents []*types.Header,
	/* START ALPHA CHANGE 1523 -- accept MDBX consensus transactions in validator lookup -- */
	dbTx *stateTx) (validator.AccountSet, error) {
	/* FINISH ALPHA CHANGE 1523 -- accept MDBX consensus transactions in validator lookup -- */
	return p.validatorsCache.GetSnapshot(blockNumber, parents, dbTx)
}

// ProcessHeaders updates the snapshot based on the verified headers
func (p *INNBCBFT) ProcessHeaders(_ []*types.Header) error {
	// Not required
	return nil
}

// GetBlockCreator retrieves the block creator (or signer) given the block header
func (p *INNBCBFT) GetBlockCreator(h *types.Header) (types.Address, error) {
	return types.BytesToAddress(h.Miner), nil
}

/* START ALPHA CHANGE 2293 -- reject unknown and duplicate state transactions before state commit -- */
// PreCommitState checks state transaction structure without registry access,
// historical lookups, or additional execution.
func (p *INNBCBFT) PreCommitState(block *types.Block, _ *state.Transition) error {
	_, err := findCommitEpochTransaction(block.Transactions)

	return err
}

/* FINISH ALPHA CHANGE 2293 -- reject unknown and duplicate state transactions before state commit -- */

/* START ALPHA CHANGE 418 -- remove INNBCBFT bridge provider method -- */
/* FINISH ALPHA CHANGE 418 -- remove INNBCBFT bridge provider method -- */

// FilterExtra is an implementation of Consensus interface
func (p *INNBCBFT) FilterExtra(extra []byte) ([]byte, error) {
	return GetIbftExtraClean(extra)
}

// initProxies initializes proxy contracts, that allow upgradeability of contracts implementation
func initProxies(transition *state.Transition, admin types.Address,
	proxyToImplMap map[types.Address]types.Address) error {
	for proxyAddress, implAddress := range proxyToImplMap {
		protectSetupProxyFn := &contractsapi.ProtectSetUpProxyGenesisProxyFn{Initiator: contracts.SystemCaller}

		proxyInput, err := protectSetupProxyFn.EncodeAbi()
		if err != nil {
			return fmt.Errorf("GenesisProxy.protectSetUpProxy params encoding failed: %w", err)
		}

		err = callContract(contracts.SystemCaller, proxyAddress, proxyInput, "GenesisProxy.protectSetUpProxy", transition)
		if err != nil {
			return err
		}

		setUpproxyFn := &contractsapi.SetUpProxyGenesisProxyFn{
			Logic: implAddress,
			Admin: admin,
			Data:  []byte{},
		}

		proxyInput, err = setUpproxyFn.EncodeAbi()
		if err != nil {
			return fmt.Errorf("GenesisProxy.setUpProxy params encoding failed: %w", err)
		}

		err = callContract(contracts.SystemCaller, proxyAddress, proxyInput, "GenesisProxy.setUpProxy", transition)
		if err != nil {
			return err
		}
	}

	return nil
}

/* START ALPHA CHANGE 490 -- remove legacy EIP1559 bridge burn contract resolver -- */
/* FINISH ALPHA CHANGE 490 -- remove legacy EIP1559 bridge burn contract resolver -- */
