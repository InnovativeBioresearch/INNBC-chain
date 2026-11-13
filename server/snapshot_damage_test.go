/* START ALPHA CHANGE 2108 -- reject damaged executable and consensus state before snapshot activation -- */
package server

import (
	"os"
	"path/filepath"
	"testing"

	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestSnapshotActivationRejectsDamagedCommittedComponents(t *testing.T) {
	for _, kind := range []snapshotbuilder.Kind{
		snapshotbuilder.KindCheckpoint,
		snapshotbuilder.KindSegmented,
	} {
		for _, damage := range []string{"code", "storage", "consensus"} {
			t.Run(string(kind)+"/"+damage, func(t *testing.T) {
				logger := hclog.NewNullLogger()
				source := filepath.Join(t.TempDir(), "source")
				chainPath := filepath.Join(t.TempDir(), "genesis.json")
				chainConfig, _, _ := writeSnapshotIntegrationSource(
					t, source, chainPath, 4, logger,
				)

				output := t.TempDir()
				segmentBlocks := uint64(0)
				mode := "full"
				if kind == snapshotbuilder.KindSegmented {
					segmentBlocks = 1
					mode = "recent"
				}

				result, err := snapshotbuilder.Build(snapshotbuilder.Config{
					Kind:            kind,
					ChainPath:       chainPath,
					SourceDataDir:   source,
					OutputRoot:      output,
					RetentionBlocks: 2,
					SegmentBlocks:   segmentBlocks,
					MDBXMapSizeGB:   1,
					Logger:          logger,
				})
				require.NoError(t, err)

				expected := "snapshot code hash mismatch"
				if damage == "consensus" {
					corruptSnapshotConsensusAtSameHeight(t, result.SnapshotDir)
					expected = "snapshot consensus data"
				} else {
					trieStorage, err := itrie.NewMDBXStorageWithMapSize(
						datadir.Trie(result.SnapshotDir),
						logger,
						snapshotIntegrationMDBXMapSize,
					)
					require.NoError(t, err)

					snapshot, err := itrie.NewState(trieStorage).NewSnapshotAt(result.StateRoot)
					require.NoError(t, err)
					account, err := snapshot.GetAccount(snapshotIntegrationContractAddress)
					require.NoError(t, err)
					require.NotNil(t, account)

					key := itrie.GetCodeKey(types.BytesToHash(account.CodeHash))
					if damage == "storage" {
						key = account.Root.Bytes()
						expected = "snapshot storage trie"
					}
					require.NoError(t, trieStorage.Put(key, []byte{0xc0}))
					require.NoError(t, trieStorage.Close())
				}

				target := filepath.Join(t.TempDir(), "target")
				instance := &Server{
					config: &Config{
						DataDir: target,
						Chain:   chainConfig,
						History: &HistoryConfig{
							Mode:                      mode,
							RetentionBlocks:           2,
							SegmentBlocks:             1,
							SnapshotSourceDir:         filepath.Join(output, string(kind)),
							SnapshotBootstrapRequired: true,
							MDBXMapSizeGB:             1,
						},
					},
					logger: logger,
				}

				activated, err := instance.activateConfiguredSnapshot()
				require.Nil(t, activated)
				require.ErrorContains(t, err, expected)
				require.NoDirExists(t, datadir.ChainData(target))

				_, statErr := os.Stat(result.SnapshotDir)
				require.NoError(t, statErr)
			})
		}
	}
}

func corruptSnapshotConsensusAtSameHeight(t *testing.T, artifactDir string) {
	t.Helper()

	env, err := mdbxgo.NewEnv(mdbxgo.Default)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = env.Close()
	})
	require.NoError(t, env.SetGeometry(
		-1, -1, snapshotIntegrationMDBXMapSize, -1, -1, 4096,
	))
	require.NoError(t, env.SetOption(mdbxgo.OptMaxDB, 4))
	require.NoError(t, env.Open(
		filepath.Join(datadir.Consensus(artifactDir), "consensusState"),
		0,
		0600,
	))

	require.NoError(t, env.Update(func(tx *mdbxgo.Txn) error {
		dbi, err := tx.OpenDBISimple("epochs", 0)
		if err != nil {
			return err
		}

		return tx.Put(
			dbi,
			[]byte("substituted-same-height-record"),
			[]byte("different consensus content"),
			0,
		)
	}))
	require.NoError(t, env.Close())
}

/* FINISH ALPHA CHANGE 2108 -- reject damaged executable and consensus state before snapshot activation -- */
