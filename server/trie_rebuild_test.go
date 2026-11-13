/* START ALPHA CHANGE 2116 -- test offline trie recovery and refusal boundaries -- */
package server

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"

	/* START ALPHA CHANGE 3413 -- use current artifacts and proved test identities -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi/testutil"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"

	/* FINISH ALPHA CHANGE 3413 -- use current artifacts and proved test identities -- */
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/innovativebioresearch/innbc-chain/types/buildroot"
)

type trieRebuildFixture struct {
	config         TrieRebuildConfig
	head           *types.Header
	sender         types.Address
	contract       types.Address
	revertContract types.Address
	outerContract  types.Address
	code           []byte
	revertCode     []byte
	outerCode      []byte
}

/* START ALPHA CHANGE 3413 -- build current test genesis without historical JSON adaptation -- */
func loadCurrentTestGenesis(t *testing.T) *chain.Chain {
	t.Helper()

	initialValidators := validator.NewTestValidators(t, 1).GetParamValidators()
	tokenConfig := *innbcbft.DefaultTokenConfig

	consensusConfig := &innbcbft.INNBCBFTConfig{
		InitialValidatorSet:      initialValidators,
		BlockTime:                common.Duration{Duration: 2 * time.Second},
		EpochSize:                2,
		SprintSize:               1,
		NativeTokenConfig:        &tokenConfig,
		MinValidatorSetSize:      1,
		MaxValidatorSetSize:      100,
		BlockTrackerPollInterval: common.Duration{Duration: time.Second},
		ValidatorRegistryConfig: &innbcbft.ValidatorRegistryConfig{
			GenesisValidator:       initialValidators[0].Address,
			MinimumStake:           big.NewInt(1),
			ProposalDurationPreset: innbcbft.ValidatorRegistryProposalDurationStandard,
			ActivationEpoch:        2,
		},
	}

	accounts, err := innbcbft.BuildGenesisValidatorSet(consensusConfig)
	require.NoError(t, err)

	config := &chain.Chain{
		Name: "current-artifact-tests",
		Genesis: &chain.Genesis{
			Timestamp:          1_700_000_000,
			GasLimit:           chain.GenesisGasLimit,
			Difficulty:         0,
			Mixhash:            innbcbft.INNBCBFTMixDigest,
			BaseFee:            chain.GenesisBaseFee,
			BaseFeeEM:          chain.GenesisBaseFeeEM,
			BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
			ExtraData:          innbcbft.BuildGenesisExtraData(accounts),
			Alloc:              testutil.GenesisAllocations(t),
		},
		Params: &chain.Params{
			ChainID: 100,
			Forks:   chain.AllForksEnabled.Copy(),
			Engine: map[string]interface{}{
				innbcbft.ConsensusName: consensusConfig,
			},
		},
	}

	payload, err := json.Marshal(config)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "current-genesis.json")
	require.NoError(t, os.WriteFile(path, payload, 0600))

	// Preserve strict import coverage and return independently decoded
	// engine maps for tests that deliberately alter configuration fields.
	imported, err := chain.ImportFromFile(path)
	require.NoError(t, err)

	decoded, err := innbcbft.GetINNBCBFTConfig(imported)
	require.NoError(t, err)
	require.NoError(t, innbcbft.ValidateGenesisExtraData(
		&decoded, imported.Genesis.ExtraData,
	))

	return imported
}

/* FINISH ALPHA CHANGE 3413 -- build current test genesis without historical JSON adaptation -- */

// Do not run these tests in parallel: production fork handlers are process-global.
func newTrieRebuildFixture(t *testing.T) trieRebuildFixture {
	t.Helper()

	logger := hclog.NewNullLogger()
	rootDir := t.TempDir()
	dataDir := filepath.Join(rootDir, "node")
	chainPath := filepath.Join(rootDir, "genesis.json")
	require.NoError(t, os.MkdirAll(datadir.ChainData(dataDir), 0755))

	/* START ALPHA CHANGE 3413 -- replay current artifacts with the normal genesis hook -- */
	config := loadCurrentTestGenesis(t)
	/* FINISH ALPHA CHANGE 3413 -- replay current artifacts with the normal genesis hook -- */

	/* START ALPHA CHANGE 2119 -- make the Issue 7A healthy execution fixture deterministic -- */
	privateKeyBytes := make([]byte, 32)
	privateKeyBytes[len(privateKeyBytes)-1] = 1

	key, err := crypto.ParseECDSAPrivateKey(privateKeyBytes)
	require.NoError(t, err)

	sender := crypto.PubKeyToAddress(&key.PublicKey)
	contract := types.StringToAddress("0x123456")
	revertContract := types.StringToAddress("0x123457")
	outerContract := types.StringToAddress("0x123458")

	// Store BLOCKHASH(NUMBER-1) in slot 0 and 42 in slot 1, emit the
	// 32-byte representation of 42 with topic 1, then return that word.
	code := []byte{
		0x60, 0x01, 0x43, 0x03, 0x40, 0x60, 0x00, 0x55,
		0x60, 0x2a, 0x60, 0x01, 0x55,
		0x60, 0x2a, 0x60, 0x00, 0x52,
		0x60, 0x01, 0x60, 0x20, 0x60, 0x00, 0xa1,
		0x60, 0x20, 0x60, 0x00, 0xf3,
	}

	/* START ALPHA CHANGE 2123 -- distinguish an intentional revert and verify log rollback -- */
	// Write 43 to slot 1, emit topic 3 with the word 43, and REVERT with
	// that word. Neither the storage write nor the log may survive.
	revertCode := []byte{
		0x60, 0x2b, 0x60, 0x01, 0x55,
		0x60, 0x2b, 0x60, 0x00, 0x52,
		0x60, 0x03, 0x60, 0x20, 0x60, 0x00, 0xa1,
		0x60, 0x20, 0x60, 0x00, 0xfd,
	}
	/* FINISH ALPHA CHANGE 2123 -- distinguish an intentional revert and verify log rollback -- */

	// Call the reverting contract. Store ISZERO(CALL result) in slot 2 and
	// store 44 in slot 3, proving that the outer execution continued while
	// the inner write was reverted.
	outerCode := []byte{
		0x60, 0x00, // return size
		0x60, 0x00, // return offset
		0x60, 0x00, // input size
		0x60, 0x00, // input offset
		0x60, 0x00, // value
		0x73, // PUSH20 revertContract
	}
	outerCode = append(outerCode, revertContract.Bytes()...)
	/* START ALPHA CHANGE 2123 -- preserve an observable successful outer call -- */
	outerCode = append(outerCode,
		0x61, 0xff, 0xff, // gas
		0xf1,       // CALL
		0x15,       // ISZERO
		0x60, 0x02, // slot 2
		0x55,       // SSTORE
		0x60, 0x2c, // value 44
		0x60, 0x03, // slot 3
		0x55, // SSTORE

		// Emit topic 2 with the word 44, then return that word.
		0x60, 0x2c, 0x60, 0x00, 0x52,
		0x60, 0x02, 0x60, 0x20, 0x60, 0x00, 0xa1,
		0x60, 0x20, 0x60, 0x00, 0xf3,
	)
	/* FINISH ALPHA CHANGE 2123 -- preserve an observable successful outer call -- */

	config.Genesis.Alloc[sender] = &chain.GenesisAccount{
		Balance: new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil),
	}
	config.Genesis.Alloc[contract] = &chain.GenesisAccount{
		Balance: big.NewInt(0),
		Code:    code,
	}
	config.Genesis.Alloc[revertContract] = &chain.GenesisAccount{
		Balance: big.NewInt(0),
		Code:    revertCode,
	}
	config.Genesis.Alloc[outerContract] = &chain.GenesisAccount{
		Balance: big.NewInt(0),
		Code:    outerCode,
	}
	/* FINISH ALPHA CHANGE 2119 -- make the Issue 7A healthy execution fixture deterministic -- */
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(chainPath, raw, 0600))

	innbcbft.SetupHeaderHashFunc()
	source, err := mdbxstorage.NewMDBXStorageWithMapSize(
		datadir.Blockchain(dataDir), logger, 1024*1024*1024,
	)
	require.NoError(t, err)

	trieStorage, err := itrie.NewMDBXStorageWithMapSize(
		datadir.Trie(dataDir), logger, 1024*1024*1024,
	)
	require.NoError(t, err)

	executor := newTrieRebuildExecutor(config, trieStorage, logger)
	genesisRoot, err := executor.WriteGenesis(config.Genesis.Alloc, types.ZeroHash)
	require.NoError(t, err)
	require.NoError(t, initForkManager(innbcbft.ConsensusName, config))

	parent := config.Genesis.GenesisHeader()
	parent.StateRoot = genesisRoot
	parent.ComputeHash()
	batch := storage.NewBatchWriter(source)
	batch.PutCanonicalHeader(parent)
	require.NoError(t, batch.WriteBatch())

	/* START ALPHA CHANGE 3810 -- adapt executable-history fixture to checked hash lookups -- */
	executor.GetHash = func(header *types.Header) state.GetHashByNumber {
		return func(number uint64) (types.Hash, error) {
			if number >= header.Number || header.Number-number > 256 {
				return types.ZeroHash, nil
			}

			hash, ok := source.ReadCanonicalHash(number)
			require.True(t, ok)

			return hash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt executable-history fixture to checked hash lookups -- */

	for number := uint64(1); number <= 2; number++ {
		header := &types.Header{
			ParentHash: parent.Hash,
			Number:     number,
			GasLimit:   config.Genesis.GasLimit,
			BaseFee:    1,
			Difficulty: 1,
			Timestamp:  parent.Timestamp + 2,
			MixHash:    innbcbft.INNBCBFTMixDigest,
			ExtraData:  (&innbcbft.Extra{}).MarshalRLPTo(nil),
		}

		signer := crypto.NewSigner(
			config.Params.Forks.At(number), uint64(config.Params.ChainID),
		)
		tx, err := signer.SignTx(&types.Transaction{
			Nonce:    number - 1,
			To:       &contract,
			Gas:      200000,
			GasPrice: big.NewInt(100),
			Value:    big.NewInt(0),
		}, key)
		require.NoError(t, err)
		tx.ComputeHash(number)

		transactions := []*types.Transaction{tx}
		if number == 2 {
			input, err := (&contractsapi.CommitEpochValidatorSetFn{
				ID: big.NewInt(1),
				Epoch: &contractsapi.Epoch{
					StartBlock: big.NewInt(1),
					EndBlock:   big.NewInt(2),
					EpochRoot:  types.ZeroHash,
				},
			}).EncodeAbi()
			require.NoError(t, err)

			epochTx := (&types.Transaction{
				From:     contracts.SystemCaller,
				To:       &contracts.ValidatorSetContract,
				Type:     types.StateTx,
				Input:    input,
				Gas:      types.StateTransactionGasLimit,
				GasPrice: big.NewInt(0),
			}).ComputeHash(number)
			transactions = append([]*types.Transaction{epochTx}, transactions...)
		}

		header.TxRoot = buildroot.CalculateTransactionsRoot(transactions, number)
		header.Sha3Uncles = buildroot.CalculateUncleRoot(nil)
		block := &types.Block{Header: header, Transactions: transactions}

		transition, err := executor.ProcessBlock(parent.StateRoot, block, types.ZeroAddress)
		require.NoError(t, err)
		_, header.StateRoot, err = transition.Commit()
		require.NoError(t, err)

		receipts := transition.Receipts()
		require.Len(t, receipts, len(transactions))
		for _, receipt := range receipts {
			require.NotNil(t, receipt.Status)
			require.Equal(t, types.ReceiptSuccess, *receipt.Status)
		}
		header.GasUsed = transition.TotalGas()
		header.ReceiptsRoot = buildroot.CalculateReceiptsRoot(receipts)
		header.LogsBloom = types.CreateBloom(receipts)
		header.ComputeHash()

		batch := storage.NewBatchWriter(source)
		batch.PutCanonicalHeader(header)
		batch.PutBody(header.Hash, &types.Body{Transactions: transactions})
		batch.PutReceipts(header.Hash, receipts)
		for _, transaction := range transactions {
			batch.PutTxLookup(transaction.Hash, header.Hash)
		}
		require.NoError(t, batch.WriteBatch())
		parent = header
	}

	require.NoError(t, source.Close())
	require.NoError(t, trieStorage.Close())

	// Simulate an operator removing the damaged trie while the node is stopped.
	require.NoError(t, os.RemoveAll(datadir.Trie(dataDir)))

	require.NoError(t, os.MkdirAll(datadir.Consensus(dataDir), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(datadir.Consensus(dataDir), "preserve-test"),
		[]byte("unchanged consensus data"),
		0600,
	))

	return trieRebuildFixture{
		config: TrieRebuildConfig{
			ChainPath:     chainPath,
			DataDir:       dataDir,
			MDBXMapSizeGB: 1,
			Logger:        logger,
		},
		head:           parent,
		sender:         sender,
		contract:       contract,
		revertContract: revertContract,
		outerContract:  outerContract,
		code:           code,
		revertCode:     revertCode,
		outerCode:      outerCode,
	}
}

func TestTrieRebuildReplaysExecutableHistory(t *testing.T) {
	for _, emptyDirectory := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "empty"}[emptyDirectory], func(t *testing.T) {
			fixture := newTrieRebuildFixture(t)
			triePath := datadir.Trie(fixture.config.DataDir)
			if emptyDirectory {
				require.NoError(t, os.Mkdir(triePath, 0755))
			}

			blockFile := filepath.Join(datadir.Blockchain(fixture.config.DataDir), "mdbx.dat")
			before, err := os.ReadFile(blockFile)
			require.NoError(t, err)

			result, err := RebuildTrieDatabase(context.Background(), fixture.config)
			require.NoError(t, err)
			require.Equal(t, fixture.head.Number, result.BlockNumber)
			require.Equal(t, fixture.head.Hash, result.BlockHash)
			require.Equal(t, fixture.head.StateRoot, result.StateRoot)

			after, err := os.ReadFile(blockFile)
			require.NoError(t, err)
			require.Equal(t, before, after)

			consensus, err := os.ReadFile(filepath.Join(
				datadir.Consensus(fixture.config.DataDir), "preserve-test",
			))
			require.NoError(t, err)
			require.Equal(t, "unchanged consensus data", string(consensus))

			store, err := itrie.NewMDBXStorageReadOnly(triePath, fixture.config.Logger)
			require.NoError(t, err)
			require.NoError(t, itrie.ValidateCompleteState(result.StateRoot, store))

			snapshot, err := itrie.NewState(store).NewSnapshotAt(result.StateRoot)
			require.NoError(t, err)
			account, err := snapshot.GetAccount(fixture.sender)
			require.NoError(t, err)
			require.NotNil(t, account)
			require.Equal(t, uint64(2), account.Nonce)

			contract, err := snapshot.GetAccount(fixture.contract)
			require.NoError(t, err)
			require.NotNil(t, contract)
			/* START ALPHA CHANGE 2153 -- check rebuilt storage and bytecode reads -- */
			checkedStorage1, storageErr1 := snapshot.GetStorage(
				fixture.contract, contract.Root, types.ZeroHash,
			)
			require.NoError(t, storageErr1)
			require.Equal(t, fixture.head.ParentHash, checkedStorage1)

			checkedStorage2, storageErr2 := snapshot.GetStorage(
				fixture.contract, contract.Root, types.StringToHash("0x01"),
			)
			require.NoError(t, storageErr2)
			require.Equal(t, types.StringToHash("0x2a"), checkedStorage2)

			code, ok, codeErr := snapshot.GetCode(types.BytesToHash(contract.CodeHash))
			require.NoError(t, codeErr)
			require.True(t, ok)
			require.Equal(t, fixture.code, code)
			/* FINISH ALPHA CHANGE 2153 -- check rebuilt storage and bytecode reads -- */

			// Verify real epoch-transition state, not only its receipt.
			config, err := chain.ImportFromFile(fixture.config.ChainPath)
			require.NoError(t, err)
			executor := newTrieRebuildExecutor(config, store, fixture.config.Logger)
			/* START ALPHA CHANGE 3810 -- adapt rebuilt-state fixture to checked hash lookups -- */
			executor.GetHash = func(*types.Header) state.GetHashByNumber {
				return func(uint64) (types.Hash, error) {
					return types.ZeroHash, nil
				}
			}
			/* FINISH ALPHA CHANGE 3810 -- adapt rebuilt-state fixture to checked hash lookups -- */
			transition, err := executor.BeginTxn(
				result.StateRoot, fixture.head, types.ZeroAddress,
			)
			require.NoError(t, err)
			epoch, err := innbcbft.NewSystemState(
				contracts.ValidatorSetContract,
				innbcbft.NewStateProvider(transition),
			).GetEpoch()
			require.NoError(t, err)
			require.Equal(t, uint64(2), epoch)
			require.NoError(t, store.Close())

			_, err = RebuildTrieDatabase(context.Background(), fixture.config)
			require.ErrorContains(t, err, "completely empty")

			staging, err := filepath.Glob(filepath.Join(
				datadir.ChainData(fixture.config.DataDir), ".trie-rebuild-*",
			))
			require.NoError(t, err)
			require.Empty(t, staging)
		})
	}
}

func TestTrieRebuildRejectsReplayMismatch(t *testing.T) {
	fixture := newTrieRebuildFixture(t)
	source, err := mdbxstorage.NewMDBXStorageWithMapSize(
		datadir.Blockchain(fixture.config.DataDir),
		fixture.config.Logger,
		1024*1024*1024,
	)
	require.NoError(t, err)

	body, err := source.ReadBody(fixture.head.Hash)
	require.NoError(t, err)
	header := fixture.head.Copy()
	header.StateRoot = types.StringToHash("0x1234")
	header.ComputeHash()
	batch := storage.NewBatchWriter(source)
	batch.PutCanonicalHeader(header)
	batch.PutBody(header.Hash, body)
	require.NoError(t, batch.WriteBatch())
	require.NoError(t, source.Close())

	_, err = RebuildTrieDatabase(context.Background(), fixture.config)
	require.ErrorContains(t, err, "state root mismatch")
	require.NoDirExists(t, datadir.Trie(fixture.config.DataDir))
}

func TestTrieRebuildRejectsWrongGenesisAndCancellation(t *testing.T) {
	fixture := newTrieRebuildFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RebuildTrieDatabase(ctx, fixture.config)
	require.ErrorIs(t, err, context.Canceled)
	require.NoDirExists(t, datadir.Trie(fixture.config.DataDir))

	config, err := chain.ImportFromFile(fixture.config.ChainPath)
	require.NoError(t, err)
	config.Genesis.Alloc[fixture.sender].Balance.Add(
		config.Genesis.Alloc[fixture.sender].Balance, big.NewInt(1),
	)
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fixture.config.ChainPath, raw, 0600))

	_, err = RebuildTrieDatabase(context.Background(), fixture.config)
	require.ErrorContains(t, err, "genesis does not match")
	require.NoDirExists(t, datadir.Trie(fixture.config.DataDir))
}

func TestTrieRebuildTargetAndPublication(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "trie")
	require.NoError(t, requireEmptyTrieRebuildTarget(target))

	require.NoError(t, os.Mkdir(target, 0755))
	require.NoError(t, requireEmptyTrieRebuildTarget(target))
	require.NoError(t, os.WriteFile(filepath.Join(target, "mdbx.dat"), []byte("existing"), 0600))
	require.ErrorContains(t, requireEmptyTrieRebuildTarget(target), "completely empty")

	stage := filepath.Join(root, "stage")
	require.NoError(t, os.Mkdir(stage, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stage, "mdbx.dat"), []byte("replacement"), 0600))
	require.ErrorContains(t, publishTrieRebuild(stage, target), "completely empty")
	original, err := os.ReadFile(filepath.Join(target, "mdbx.dat"))
	require.NoError(t, err)
	require.Equal(t, "existing", string(original))

	require.NoError(t, os.Remove(filepath.Join(target, "mdbx.dat")))
	require.NoError(t, syncTrieRebuildStage(stage))
	require.NoError(t, publishTrieRebuild(stage, target))
	require.NoDirExists(t, stage)
	replacement, err := os.ReadFile(filepath.Join(target, "mdbx.dat"))
	require.NoError(t, err)
	require.Equal(t, "replacement", string(replacement))

	link := filepath.Join(root, "trie-link")
	require.NoError(t, os.Symlink(target, link))
	require.Error(t, requireEmptyTrieRebuildTarget(link))
}

/* FINISH ALPHA CHANGE 2116 -- test offline trie recovery and refusal boundaries -- */
