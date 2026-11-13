/* START ALPHA CHANGE 3281 -- construct real MDBX snapshot sources for command exit regressions -- */
package root

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

const snapshotBuildExitMapSize = 1024 * 1024 * 1024

type snapshotBuildExitFixture struct {
	root      string
	chainPath string
	source    string
	output    string
	blockHash types.Hash
	stateRoot types.Hash
}

func newSnapshotBuildExitPaths(t *testing.T) snapshotBuildExitFixture {
	t.Helper()

	root := t.TempDir()
	return snapshotBuildExitFixture{
		root:      root,
		chainPath: filepath.Join(root, "genesis.json"),
		source:    filepath.Join(root, "source"),
		output:    filepath.Join(root, "snapshots"),
	}
}

func (f snapshotBuildExitFixture) args(kind string, jsonOutput bool) []string {
	args := []string{
		"snapshot", "build", kind,
		"--chain", f.chainPath,
		"--source-data-dir", f.source,
		"--output-root", f.output,
		"--retention-blocks", "2",
		"--segment-blocks", "1",
		"--mdbx-map-size-gb", "1",
	}
	if jsonOutput {
		args = append([]string{"--json"}, args...)
	}
	return args
}

func newSnapshotBuildExitFixture(t *testing.T) snapshotBuildExitFixture {
	t.Helper()

	f := newSnapshotBuildExitPaths(t)
	logger := hclog.NewNullLogger()
	innbcbft.SetupHeaderHashFunc()

	config := &chain.Chain{
		Name: "snapshot-command-exit-test",
		Genesis: &chain.Genesis{
			ExtraData:          (&innbcbft.Extra{}).MarshalRLPTo(nil),
			GasLimit:           chain.GenesisGasLimit,
			Difficulty:         chain.GenesisDifficulty,
			BaseFee:            chain.GenesisBaseFee,
			BaseFeeEM:          chain.GenesisBaseFeeEM,
			BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
			Alloc: map[types.Address]*chain.GenesisAccount{
				types.StringToAddress("0x1001"): {
					Balance: big.NewInt(1000000000),
					Nonce:   7,
				},
			},
		},
		Params: &chain.Params{
			Forks:   chain.AllForksEnabled,
			ChainID: 100,
			/* START ALPHA CHANGE 3325 -- preserve subprocess publication coverage with valid clock configuration -- */
			Engine: map[string]interface{}{
				innbcbft.ConsensusName: map[string]interface{}{"blockTime": "1s"},
			},
			/* FINISH ALPHA CHANGE 3325 -- preserve subprocess publication coverage with valid clock configuration -- */
		},
	}

	raw, err := json.MarshalIndent(config, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f.chainPath, raw, 0600))

	func() {
		trie, err := itrie.NewMDBXStorageWithMapSize(
			datadir.Trie(f.source), logger, snapshotBuildExitMapSize,
		)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, trie.Close())
		}()

		executor := statepkg.NewExecutor(
			config.Params, itrie.NewState(trie), logger,
		)
		f.stateRoot, err = executor.WriteGenesis(
			config.Genesis.Alloc, types.ZeroHash,
		)
		require.NoError(t, err)
		require.NotEqual(t, types.EmptyRootHash, f.stateRoot)
	}()

	func() {
		database, err := mdbxstorage.NewMDBXStorageWithMapSize(
			datadir.Blockchain(f.source), logger, snapshotBuildExitMapSize,
		)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, database.Close())
		}()

		genesis := config.Genesis.GenesisHeader()
		genesis.StateRoot = f.stateRoot
		genesis.ComputeHash()
		require.NotEqual(t, types.ZeroHash, genesis.Hash)

		batch := storage.NewBatchWriterForBlock(database, 0)
		batch.PutCanonicalHeader(genesis)
		require.NoError(t, batch.WriteBatch())

		previous := genesis
		for number := uint64(1); number <= 4; number++ {
			header := genesis.Copy()
			header.Number = number
			header.ParentHash = previous.Hash
			header.Timestamp = number * 2
			header.ComputeHash()
			require.NotEqual(t, types.ZeroHash, header.Hash)

			batch := storage.NewBatchWriterForBlock(database, number)
			batch.PutCanonicalHeader(header)
			batch.PutBody(header.Hash, &types.Body{})
			batch.PutReceipts(header.Hash, []*types.Receipt{})
			require.NoError(t, batch.WriteBatch())

			previous = header
		}
		f.blockHash = previous.Hash
	}()

	snapshotBuildExitWriteConsensus(t, f.source, 4)

	return f
}

func snapshotBuildExitWriteConsensus(
	t *testing.T,
	source string,
	checkpoint uint64,
) {
	t.Helper()

	path := filepath.Join(datadir.Consensus(source), "consensusState")
	require.NoError(t, os.MkdirAll(path, 0750))

	environment, err := mdbxgo.NewEnv(mdbxgo.Default)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, environment.Close())
	}()

	require.NoError(t, environment.SetGeometry(
		-1, -1, snapshotBuildExitMapSize, -1, -1, 4096,
	))
	require.NoError(t, environment.SetOption(mdbxgo.OptMaxDB, 4))
	require.NoError(t, environment.Open(path, 0, 0600))

	require.NoError(t, environment.Update(func(transaction *mdbxgo.Txn) error {
		var proposerDB mdbxgo.DBI
		for _, name := range []string{
			"epochs",
			"validatorSnapshots",
			"proposerSnapshot",
			"fullValidatorSetBucket",
		} {
			database, err := transaction.OpenDBISimple(name, mdbxgo.Create)
			if err != nil {
				return err
			}
			if name == "proposerSnapshot" {
				proposerDB = database
			}
		}

		raw, err := json.Marshal(&innbcbft.ProposerSnapshot{
			Height: checkpoint + 1,
		})
		if err != nil {
			return err
		}

		return transaction.Put(
			proposerDB,
			[]byte("proposerSnapshotKey"),
			raw,
			0,
		)
	}))
}

/* FINISH ALPHA CHANGE 3281 -- construct real MDBX snapshot sources for command exit regressions -- */
