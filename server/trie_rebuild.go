/* START ALPHA CHANGE 2112 -- rebuild an offline full-history node trie through verified replay -- */
package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"

	/* START ALPHA CHANGE 3470 -- preserve node geometry when rebuilding its trie -- */
	"github.com/innovativebioresearch/innbc-chain/helper/mdbxconfig"
	/* FINISH ALPHA CHANGE 3470 -- preserve node geometry when rebuilding its trie -- */
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/addresslist"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/innovativebioresearch/innbc-chain/types/buildroot"
)

type TrieRebuildConfig struct {
	ChainPath     string
	DataDir       string
	MDBXMapSizeGB uint64
	/* START ALPHA CHANGE 3470 -- configure replacement trie pages -- */
	MDBXPageSizeBytes uint64
	/* FINISH ALPHA CHANGE 3470 -- configure replacement trie pages -- */
	Logger hclog.Logger
}

type TrieRebuildResult struct {
	BlockNumber uint64     `json:"block_number"`
	BlockHash   types.Hash `json:"block_hash"`
	StateRoot   types.Hash `json:"state_root"`
	TriePath    string     `json:"trie_path"`
}

// RebuildTrieDatabase requires exclusive OPERATIONAL use of DataDir:
// the node and all other database maintenance processes must be stopped.
// No process lock is acquired.
//
// Only a genesis-origin, unified full-history blockchain database is accepted.
// Consensus data, block history, node keys, and the genesis file are not changed.
// This is a standalone command operation; it initializes process-global fork
// handlers and must not be called inside a running server.
func RebuildTrieDatabase(
	ctx context.Context,
	config TrieRebuildConfig,
) (*TrieRebuildResult, error) {
	if ctx == nil {
		return nil, errors.New("trie rebuild context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if config.DataDir == "" || config.ChainPath == "" {
		return nil, errors.New("data directory and chain configuration are required")
	}

	logger := config.Logger
	if logger == nil {
		logger = hclog.NewNullLogger()
	}

	dataDir, err := filepath.Abs(config.DataDir)
	if err != nil {
		return nil, err
	}
	chainData := datadir.ChainData(dataDir)
	triePath := datadir.Trie(dataDir)

	if err := requireTrieRebuildDirectory(dataDir); err != nil {
		return nil, err
	}
	if err := requireTrieRebuildDirectory(chainData); err != nil {
		return nil, err
	}
	if err := requireEmptyTrieRebuildTarget(triePath); err != nil {
		return nil, err
	}

	const gib = uint64(1024 * 1024 * 1024)
	maxInt := uint64(^uint(0) >> 1)
	if config.MDBXMapSizeGB == 0 || config.MDBXMapSizeGB > maxInt/gib {
		return nil, errors.New("invalid MDBX map size in GiB")
	}
	mapSize := int(config.MDBXMapSizeGB * gib)
	/* START ALPHA CHANGE 3470 -- keep the replacement trie compatible with retained blockchain storage -- */
	if _, err := mdbxconfig.Resolve(config.MDBXPageSizeBytes); err != nil {
		return nil, err
	}
	if err := mdbxconfig.CheckTree(
		datadir.Blockchain(dataDir), config.MDBXPageSizeBytes,
	); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3470 -- keep the replacement trie compatible with retained blockchain storage -- */

	chainConfig, err := chain.ImportFromFile(config.ChainPath)
	if err != nil {
		return nil, fmt.Errorf("load rebuild genesis: %w", err)
	}
	if chainConfig.Genesis == nil ||
		chainConfig.Params == nil ||
		chainConfig.Params.Forks == nil {
		return nil, errors.New("incomplete rebuild chain configuration")
	}
	if chainConfig.Params.GetEngine() != innbcbft.ConsensusName {
		return nil, errors.New("trie rebuild requires INNBCBFT")
	}

	consensusConfig, err := innbcbft.GetINNBCBFTConfig(chainConfig)
	if err != nil {
		return nil, err
	}
	if consensusConfig.InitialTrieRoot != types.ZeroHash {
		return nil, errors.New("trie rebuild requires genesis initialization from allocations")
	}

	innbcbft.SetupHeaderHashFunc()

	blockchainPath := datadir.Blockchain(dataDir)
	if err := requireTrieRebuildDirectory(blockchainPath); err != nil {
		return nil, err
	}
	info, err := os.Lstat(filepath.Join(blockchainPath, "mdbx.dat"))
	if err != nil {
		return nil, fmt.Errorf("full-history blockchain database is unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("blockchain mdbx.dat must be a regular file")
	}

	source, err := mdbxstorage.NewMDBXStorageReadOnly(blockchainPath, logger)
	if err != nil {
		return nil, fmt.Errorf("open rebuild block history: %w", err)
	}
	sourceOpen := true
	defer func() {
		if sourceOpen {
			_ = source.Close()
		}
	}()

	history, ok := source.(storage.HistoryAvailabilityStorage)
	if !ok || history.HistoryFrom() != 0 {
		return nil, errors.New("trie rebuild requires complete block history from genesis")
	}

	headNumber, ok := source.ReadHeadNumber()
	if !ok || headNumber == math.MaxUint64 {
		return nil, errors.New("missing or unsupported source head number")
	}
	head, err := readTrieRebuildHeader(source, headNumber)
	if err != nil {
		return nil, err
	}
	if err := verifyTrieRebuildHead(source, head); err != nil {
		return nil, err
	}
	genesis, err := readTrieRebuildHeader(source, 0)
	if err != nil {
		return nil, err
	}

	stage, err := os.MkdirTemp(chainData, ".trie-rebuild-")
	if err != nil {
		return nil, err
	}
	defer func() {
		// This path is unique to this invocation. After successful rename it
		// no longer exists; the published trie is never removed here.
		_ = os.RemoveAll(stage)
	}()
	if err := os.Chmod(stage, 0755); err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 3470 -- create the replacement trie using the selected pages -- */
	trieStorage, err := itrie.NewMDBXStorageWithMapSize(
		stage, logger, mapSize, config.MDBXPageSizeBytes,
	)
	/* FINISH ALPHA CHANGE 3470 -- create the replacement trie using the selected pages -- */
	if err != nil {
		return nil, err
	}
	trieOpen := true
	defer func() {
		if trieOpen {
			_ = trieStorage.Close()
		}
	}()

	executor := newTrieRebuildExecutor(chainConfig, trieStorage, logger)

	// Match ordinary startup: execute configured genesis initialization,
	// then install the normal runtime fork handlers.
	root, err := executor.WriteGenesis(chainConfig.Genesis.Alloc, types.ZeroHash)
	if err != nil {
		return nil, fmt.Errorf("rebuild genesis state: %w", err)
	}

	expectedGenesis := chainConfig.Genesis.GenesisHeader()
	expectedGenesis.StateRoot = root
	expectedGenesis.ComputeHash()
	if root != genesis.StateRoot || expectedGenesis.Hash != genesis.Hash {
		return nil, errors.New("reconstructed genesis does not match stored block zero")
	}
	if err := initForkManager(innbcbft.ConsensusName, chainConfig); err != nil {
		return nil, fmt.Errorf("initialize replay forks: %w", err)
	}

	logger.Info("rebuilding trie from canonical history", "target", head.Number)

	if err := replayTrieRebuildBlocks(
		ctx, source, executor, genesis, head, logger,
	); err != nil {
		return nil, err
	}

	closeErr := trieStorage.Close()
	trieOpen = false
	if closeErr != nil {
		return nil, fmt.Errorf("close rebuilt trie: %w", closeErr)
	}

	// Use the complete-state validator introduced for snapshot integrity,
	// against a newly opened database rather than the execution caches.
	checkedStorage, err := itrie.NewMDBXStorageReadOnly(stage, logger)
	if err != nil {
		return nil, fmt.Errorf("reopen rebuilt trie: %w", err)
	}
	checkErr := itrie.ValidateCompleteState(head.StateRoot, checkedStorage)
	closeErr = checkedStorage.Close()
	if err := errors.Join(checkErr, closeErr); err != nil {
		return nil, fmt.Errorf("validate rebuilt executable state: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := verifyTrieRebuildHead(source, head); err != nil {
		return nil, err
	}
	closeErr = source.Close()
	sourceOpen = false
	if closeErr != nil {
		return nil, fmt.Errorf("close source block history: %w", closeErr)
	}

	if err := syncTrieRebuildStage(stage); err != nil {
		return nil, fmt.Errorf("synchronize rebuilt trie: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := publishTrieRebuild(stage, triePath); err != nil {
		return nil, err
	}

	return &TrieRebuildResult{
		BlockNumber: head.Number,
		BlockHash:   head.Hash,
		StateRoot:   head.StateRoot,
		TriePath:    triePath,
	}, nil
}

// Apply the same allocation transformations and genesis hook as server startup.
// The supplied configuration is local to this standalone rebuild.
func newTrieRebuildExecutor(
	config *chain.Chain,
	trieStorage itrie.Storage,
	logger hclog.Logger,
) *state.Executor {
	if config.Genesis.Alloc == nil {
		config.Genesis.Alloc = make(map[types.Address]*chain.GenesisAccount)
	}
	if config.Params.ContractDeployerAllowList != nil {
		addresslist.ApplyGenesisAllocs(
			config.Genesis,
			contracts.AllowListContractsAddr,
			config.Params.ContractDeployerAllowList,
		)
	}
	if config.Params.ContractDeployerBlockList != nil {
		addresslist.ApplyGenesisAllocs(
			config.Genesis,
			contracts.BlockListContractsAddr,
			config.Params.ContractDeployerBlockList,
		)
	}
	if config.Params.TransactionsAllowList != nil {
		addresslist.ApplyGenesisAllocs(
			config.Genesis,
			contracts.AllowListTransactionsAddr,
			config.Params.TransactionsAllowList,
		)
	}
	if config.Params.TransactionsBlockList != nil {
		addresslist.ApplyGenesisAllocs(
			config.Genesis,
			contracts.BlockListTransactionsAddr,
			config.Params.TransactionsBlockList,
		)
	}

	executor := state.NewExecutor(config.Params, itrie.NewState(trieStorage), logger)
	executor.GenesisPostHook = innbcbft.GenesisPostHookFactory(
		config, innbcbft.ConsensusName,
	)

	return executor
}

func replayTrieRebuildBlocks(
	ctx context.Context,
	source storage.Storage,
	executor *state.Executor,
	genesis *types.Header,
	head *types.Header,
	logger hclog.Logger,
) error {
	/* START ALPHA CHANGE 3809 -- preserve missing in-range BLOCKHASH failures during trie replay -- */

	// BLOCKHASH only needs the previous 256 canonical hashes.
	hashes := map[uint64]types.Hash{0: genesis.Hash}
	executor.GetHash = func(header *types.Header) state.GetHashByNumber {
		blockNumber := header.Number

		return func(number uint64) (types.Hash, error) {
			if number >= blockNumber || blockNumber-number > 256 {
				return types.ZeroHash, nil
			}

			hash, ok := hashes[number]
			if !ok {
				return types.ZeroHash, fmt.Errorf(
					"missing replay BLOCKHASH(%d) for block %d",
					number,
					blockNumber,
				)
			}

			return hash, nil
		}
	}

	/* FINISH ALPHA CHANGE 3809 -- preserve missing in-range BLOCKHASH failures during trie replay -- */

	/* START ALPHA CHANGE 2295 -- describe stateless consensus checks during trie replay -- */
	// These INNBCBFT methods do not require a running consensus instance:
	// GetBlockCreator uses Header.Miner; PreCommitState checks only the
	// supplied state transaction inputs and rejects duplicate epoch calls.
	backend := new(innbcbft.INNBCBFT)
	parent := genesis
	/* FINISH ALPHA CHANGE 2295 -- describe stateless consensus checks during trie replay -- */

	for number := uint64(1); number <= head.Number; number++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := readTrieRebuildHeader(source, number)
		if err != nil {
			return err
		}
		if header.ParentHash != parent.Hash {
			return fmt.Errorf("canonical parent mismatch at block %d", number)
		}

		body, err := source.ReadBody(header.Hash)
		if err != nil {
			return fmt.Errorf("read block %d body: %w", number, err)
		}
		if body == nil {
			return fmt.Errorf("missing block %d body", number)
		}
		for _, tx := range body.Transactions {
			if tx == nil || tx.Gas > header.GasLimit {
				return fmt.Errorf("invalid transaction in block %d", number)
			}
			// Recompute ordinary senders from their signatures during Write.
			// State transactions retain their stored system sender.
			if tx.Type == types.LegacyTx || tx.Type == types.DynamicFeeTx {
				tx.From = types.ZeroAddress
			}
		}
		if buildroot.CalculateTransactionsRoot(body.Transactions, number) != header.TxRoot {
			return fmt.Errorf("transaction root mismatch at block %d", number)
		}
		if buildroot.CalculateUncleRoot(body.Uncles) != header.Sha3Uncles {
			return fmt.Errorf("uncle root mismatch at block %d", number)
		}

		block := &types.Block{
			Header:       header,
			Transactions: body.Transactions,
			Uncles:       body.Uncles,
		}
		creator, err := backend.GetBlockCreator(header)
		if err != nil {
			return err
		}
		transition, err := executor.ProcessBlock(parent.StateRoot, block, creator)
		if err != nil {
			return fmt.Errorf("execute block %d: %w", number, err)
		}
		/* START ALPHA CHANGE 2157 -- retain local state failure precedence during offline replay -- */
		hookErr := backend.PreCommitState(block, transition)
		if stateErr := transition.Error(); stateErr != nil {
			return fmt.Errorf("pre-commit replay block %d state: %w", number, stateErr)
		}
		if hookErr != nil {
			return fmt.Errorf("pre-commit block %d: %w", number, hookErr)
		}
		/* FINISH ALPHA CHANGE 2157 -- retain local state failure precedence during offline replay -- */

		_, root, err := transition.Commit()
		if err != nil {
			return fmt.Errorf("commit replay block %d: %w", number, err)
		}
		if root != header.StateRoot {
			return fmt.Errorf("state root mismatch at block %d", number)
		}
		if transition.TotalGas() != header.GasUsed {
			return fmt.Errorf("gas used mismatch at block %d", number)
		}
		receipts := transition.Receipts()
		if len(receipts) != len(body.Transactions) ||
			buildroot.CalculateReceiptsRoot(receipts) != header.ReceiptsRoot ||
			types.CreateBloom(receipts) != header.LogsBloom {
			return fmt.Errorf("receipt commitment mismatch at block %d", number)
		}

		hashes[number] = header.Hash
		if number >= 256 {
			delete(hashes, number-256)
		}
		parent = header

		if number%1000 == 0 || number == head.Number {
			logger.Info("trie replay progress", "block", number, "target", head.Number)
		}
	}

	if parent.Hash != head.Hash || parent.StateRoot != head.StateRoot {
		return errors.New("replay did not reach the captured source head")
	}

	return nil
}

func readTrieRebuildHeader(
	source storage.Storage,
	number uint64,
) (*types.Header, error) {
	hash, ok := source.ReadCanonicalHash(number)
	if !ok || hash == types.ZeroHash {
		return nil, fmt.Errorf("missing canonical block %d", number)
	}
	header, err := source.ReadHeader(hash)
	if err != nil {
		return nil, fmt.Errorf("read header %d: %w", number, err)
	}
	if header == nil || header.Number != number || header.Hash != hash {
		return nil, fmt.Errorf("canonical header mismatch at block %d", number)
	}

	return header, nil
}

func verifyTrieRebuildHead(source storage.Storage, expected *types.Header) error {
	number, ok := source.ReadHeadNumber()
	if !ok || number != expected.Number {
		return errors.New("source head changed during trie rebuild; keep the node stopped")
	}
	hash, ok := source.ReadHeadHash()
	if !ok || hash != expected.Hash {
		return errors.New("source head hash changed during trie rebuild")
	}
	header, err := readTrieRebuildHeader(source, expected.Number)
	if err != nil {
		return err
	}
	if header.Hash != expected.Hash {
		return errors.New("source canonical head changed during trie rebuild")
	}

	return nil
}

/* FINISH ALPHA CHANGE 2112 -- rebuild an offline full-history node trie through verified replay -- */
