/* START ALPHA CHANGE 3189 -- reject damaged local history before snapshot replacement and preserve bootstrap policy -- */
package server

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	segmentedstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/segmented"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type snapshotInspectionStorage struct {
	storage.Storage
	startup  storage.StartupState
	readErr  error
	closeErr error
	reads    int
	closes   int
}

func (s *snapshotInspectionStorage) ReadStartupState() (storage.StartupState, error) {
	s.reads++
	return s.startup, s.readErr
}

func (s *snapshotInspectionStorage) Close() error {
	s.closes++
	return s.closeErr
}

func TestSnapshotStartup_ReaderErrorsAndClassification(t *testing.T) {
	readFailure := errors.New("injected startup read failure")
	closeFailure := errors.New("injected inspection close failure")
	genesis := (&types.Header{Number: 0}).ComputeHash()
	head := (&types.Header{Number: 6}).ComputeHash()

	valid := storage.StartupState{
		Head: head, HasGenesis: true, GenesisHash: genesis.Hash,
	}

	tests := []struct {
		name     string
		startup  storage.StartupState
		readErr  error
		closeErr error
		wantErr  bool
		wantHead uint64
		usable   bool
	}{
		{
			name:    "empty",
			startup: storage.StartupState{Empty: true},
		},
		{
			name: "genesis_only",
			startup: storage.StartupState{
				Head: genesis, HasGenesis: true, GenesisHash: genesis.Hash,
			},
		},
		{
			name: "history", startup: valid, wantHead: 6, usable: true,
		},
		{
			name:    "missing_head",
			startup: storage.StartupState{HasGenesis: true},
			wantErr: true,
		},
		{
			name:    "missing_genesis",
			startup: storage.StartupState{Head: head},
			wantErr: true,
		},
		{
			name:    "inconsistent_empty",
			startup: storage.StartupState{Empty: true, Head: head},
			wantErr: true,
		},
		{
			name: "read_error", readErr: readFailure, wantErr: true,
		},
		{
			name: "close_error", startup: valid,
			closeErr: closeFailure, wantErr: true,
		},
		{
			name: "both_errors", readErr: readFailure,
			closeErr: closeFailure, wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &snapshotInspectionStorage{
				startup:  test.startup,
				readErr:  test.readErr,
				closeErr: test.closeErr,
			}

			/* START ALPHA CHANGE 3213 -- verify inspection retains identity only after successful initialized-state validation -- */
			number, usable, genesisHash, err := inspectSnapshotLocalHead(reader)
			require.Equal(t, 1, reader.reads)
			require.Equal(t, 1, reader.closes)

			if test.wantErr {
				require.Error(t, err)
				require.Zero(t, number)
				require.False(t, usable)
				require.Nil(t, genesisHash)
				if test.readErr != nil {
					require.ErrorIs(t, err, test.readErr)
				}
				if test.closeErr != nil {
					require.ErrorIs(t, err, test.closeErr)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.wantHead, number)
			require.Equal(t, test.usable, usable)

			if test.startup.Empty {
				require.Nil(t, genesisHash)
			} else {
				require.NotNil(t, genesisHash)
				require.Equal(t, test.startup.GenesisHash, *genesisHash)
			}
			/* FINISH ALPHA CHANGE 3213 -- verify inspection retains identity only after successful initialized-state validation -- */
		})
	}
}

type snapshotStartupFixture struct {
	mode          string
	chain         *chain.Chain
	genesisRoot   types.Hash
	sourceDataDir string
	snapshotRoot  string
	checkpoint    *types.Header
}

func newSnapshotStartupFixture(t *testing.T, mode string) *snapshotStartupFixture {
	t.Helper()

	logger := hclog.NewNullLogger()
	source := filepath.Join(t.TempDir(), "source")
	chainPath := filepath.Join(t.TempDir(), "genesis.json")
	config, root, checkpoint := writeSnapshotIntegrationSource(
		t, source, chainPath, 5, logger,
	)

	kind := snapshotbuilder.KindCheckpoint
	if mode == "recent" {
		kind = snapshotbuilder.KindSegmented
	}

	output := t.TempDir()
	buildConfig := snapshotbuilder.Config{
		Kind:            kind,
		ChainPath:       chainPath,
		SourceDataDir:   source,
		OutputRoot:      output,
		RetentionBlocks: 4,
		MDBXMapSizeGB:   1,
		Logger:          logger,
	}
	if mode == "recent" {
		buildConfig.SegmentBlocks = 2
	}

	result, err := snapshotbuilder.Build(buildConfig)
	require.NoError(t, err)

	manifest, err := snapshotbuilder.ValidatePublishedArtifact(
		result.SnapshotDir, kind, 1, logger,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(5), manifest.BlockNumber)

	return &snapshotStartupFixture{
		mode:          mode,
		chain:         config,
		genesisRoot:   root,
		sourceDataDir: source,
		snapshotRoot:  filepath.Join(output, string(kind)),
		checkpoint:    checkpoint,
	}
}

func (f *snapshotStartupFixture) newServer(t *testing.T, required bool) *Server {
	t.Helper()

	return &Server{
		config: &Config{
			DataDir: filepath.Join(t.TempDir(), "node"),
			Chain:   f.chain,
			History: &HistoryConfig{
				Mode:                      f.mode,
				SegmentBlocks:             2,
				RetentionBlocks:           4,
				SnapshotSourceDir:         f.snapshotRoot,
				SnapshotBootstrapRequired: required,
				MDBXMapSizeGB:             1,
			},
		},
		logger: hclog.NewNullLogger(),
	}
}

// height -2 leaves storage absent; -1 creates empty databases; zero writes
// genesis only. Positive heights write a canonical prefix matching the source.
func (f *snapshotStartupFixture) seedLocal(
	t *testing.T,
	instance *Server,
	height int,
) *types.Header {
	t.Helper()

	if height < -1 {
		return nil
	}

	database := openActivatedSnapshotBlockchain(
		t, instance.config.DataDir, f.mode, 2, 4, instance.logger,
	)
	defer func() {
		if database != nil {
			require.NoError(t, database.Close())
		}
	}()

	if height == -1 {
		require.NoError(t, database.Close())
		database = nil
		return nil
	}

	genesis := f.chain.Genesis.GenesisHeader()
	genesis.StateRoot = f.genesisRoot
	genesis.ComputeHash()

	writer := storage.NewBatchWriterForBlock(database, 0)
	writer.PutCanonicalHeader(genesis)
	require.NoError(t, writer.WriteBatch())

	previous := genesis
	for index := range height {
		number := uint64(index + 1)
		header := (&types.Header{
			ParentHash:   previous.Hash,
			Number:       number,
			StateRoot:    f.genesisRoot,
			TxRoot:       types.EmptyRootHash,
			ReceiptsRoot: types.EmptyRootHash,
			Sha3Uncles:   types.EmptyUncleHash,
			GasLimit:     genesis.GasLimit,
			Difficulty:   genesis.Difficulty,
			BaseFee:      genesis.BaseFee,
			ExtraData:    f.chain.Genesis.ExtraData,
		}).ComputeHash()

		if number == 5 {
			require.Equal(t, f.checkpoint.Hash, header.Hash)
		}

		writer = storage.NewBatchWriterForBlock(database, number)
		writer.PutCanonicalHeader(header)
		require.NoError(t, writer.WriteBatch())
		previous = header
	}

	require.NoError(t, database.Close())
	database = nil

	require.NoError(t, copySnapshotDirectory(
		datadir.Trie(f.sourceDataDir),
		datadir.Trie(instance.config.DataDir),
	))
	writeSnapshotIntegrationConsensusState(
		t, datadir.Consensus(instance.config.DataDir), uint64(height),
	)

	return previous
}

func openSnapshotStartupReader(t *testing.T, instance *Server) storage.Storage {
	t.Helper()

	var reader storage.Storage
	var err error
	if instance.config.History.Mode == "recent" {
		reader, err = segmentedstorage.NewStorageReadOnly(
			datadir.Blockchain(instance.config.DataDir),
			instance.logger,
			2,
			4,
		)
	} else {
		reader, err = mdbxstorage.NewMDBXStorageReadOnly(
			datadir.Blockchain(instance.config.DataDir), instance.logger,
		)
	}
	require.NoError(t, err)
	return reader
}

func damageSnapshotStartupMetadata(
	t *testing.T,
	instance *Server,
	field string,
	short bool,
) {
	t.Helper()

	database := openActivatedSnapshotBlockchain(
		t, instance.config.DataDir, instance.config.History.Mode,
		2, 4, instance.logger,
	)
	defer func() { require.NoError(t, database.Close()) }()

	blockNumber := uint64(6)
	var key []byte

	switch field {
	case "number":
		key = append(append([]byte(nil), storage.HEAD...), storage.NUMBER...)
	case "hash":
		key = append(append([]byte(nil), storage.HEAD...), storage.HASH...)
	case "genesis":
		blockNumber = 0
		key = append(
			append([]byte(nil), storage.CANONICAL...),
			common.EncodeUint64ToBytes(0)...,
		)
	default:
		t.Fatalf("unknown damage field %q", field)
	}

	var batch storage.Batch
	if targeted, ok := database.(storage.BlockBatchStorage); ok {
		batch = targeted.NewBatchForBlock(blockNumber)
	} else {
		batch = database.NewBatch()
	}

	if short {
		batch.Put(key, []byte{1})
	} else {
		batch.Delete(key)
	}
	require.NoError(t, batch.Write())
}

func snapshotStartupFiles(t *testing.T, root string) map[string][sha256.Size]byte {
	t.Helper()

	result := make(map[string][sha256.Size]byte)
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return result
	} else {
		require.NoError(t, err)
	}

	require.NoError(t, filepath.WalkDir(root, func(
		path string, entry fs.DirEntry, walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			result[relative+"/"] = [sha256.Size]byte{}
			return nil
		}
		// Read-only MDBX access may update reader-lock bookkeeping.
		if entry.Name() == "mdbx.lck" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[relative] = sha256.Sum256(raw)
		return nil
	}))

	return result
}

func TestSnapshotStartup_DamagedLocalHistoryCannotActivateSnapshot(t *testing.T) {
	for _, mode := range []string{"recent", "full"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newSnapshotStartupFixture(t, mode)

			damages := []struct {
				name    string
				field   string
				short   bool
				recover bool
			}{
				{name: "missing_head_number", field: "number"},
				{name: "short_head_number", field: "number", short: true},
				{name: "missing_head_hash", field: "hash"},
				{name: "missing_genesis_mapping", field: "genesis"},
				{
					name:    "recovered_missing_head_number",
					field:   "number",
					recover: true,
				},
			}

			for _, damage := range damages {
				for _, required := range []bool{false, true} {
					t.Run(fmt.Sprintf(
						"%s/required=%t", damage.name, required,
					), func(t *testing.T) {
						instance := fixture.newServer(t, required)
						head := fixture.seedLocal(t, instance, 6)
						damageSnapshotStartupMetadata(
							t, instance, damage.field, damage.short,
						)

						// Establish that this fixture reaches the original
						// fallback: actual head 6, weak reported head 4.
						if mode == "recent" && damage.field == "number" {
							reader := openSnapshotStartupReader(t, instance)
							number, found := reader.ReadHeadNumber()
							require.True(t, found)
							require.Equal(t, uint64(4), number)
							require.NoError(t, reader.Close())
						}

						live := datadir.ChainData(instance.config.DataDir)
						before := snapshotStartupFiles(t, live)

						// Recovery is intentionally allowed to restore the
						// original tree before strict local inspection.
						if damage.recover {
							require.NoError(t, os.Rename(
								live,
								filepath.Join(
									instance.config.DataDir,
									snapshotActivationBackupName,
								),
							))
							require.NoError(t, createSnapshotActivationMarker(
								instance.config.DataDir,
							))
						}

						activated, err := instance.activateConfiguredSnapshot()
						require.ErrorContains(t, err, "startup")
						require.Nil(t, activated)

						require.Equal(t, before, snapshotStartupFiles(t, live))
						require.NoDirExists(t, filepath.Join(
							instance.config.DataDir, snapshotActivationStageName,
						))
						require.NoDirExists(t, filepath.Join(
							instance.config.DataDir, snapshotActivationBackupName,
						))
						require.NoFileExists(t, filepath.Join(
							instance.config.DataDir, snapshotActivationMarkerName,
						))

						reader := openSnapshotStartupReader(t, instance)
						canonical, found := reader.ReadCanonicalHash(6)
						require.True(t, found)
						require.Equal(t, head.Hash, canonical)
						stored, err := reader.ReadHeader(head.Hash)
						require.NoError(t, err)
						require.Equal(t, head.Hash, stored.Hash)
						require.NoError(t, reader.Close())
					})
				}
			}
		})
	}
}

func TestSnapshotStartup_PreservesHealthyBootstrapPolicy(t *testing.T) {
	for _, mode := range []string{"recent", "full"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newSnapshotStartupFixture(t, mode)

			states := []struct {
				name   string
				height int
			}{
				{name: "absent", height: -2},
				{name: "empty_database", height: -1},
				{name: "genesis_only", height: 0},
				{name: "snapshot_newer", height: 4},
				{name: "snapshot_equal", height: 5},
				{name: "snapshot_older", height: 6},
			}

			for _, state := range states {
				for _, required := range []bool{false, true} {
					t.Run(fmt.Sprintf(
						"%s/required=%t", state.name, required,
					), func(t *testing.T) {
						instance := fixture.newServer(t, required)
						fixture.seedLocal(t, instance, state.height)
						before := snapshotStartupFiles(
							t, datadir.ChainData(instance.config.DataDir),
						)

						wantActivation := mode == "recent" && state.height < 5
						if mode == "full" {
							wantActivation = required && state.height <= 0
						}

						activated, err := instance.activateConfiguredSnapshot()
						require.NoError(t, err)

						if wantActivation {
							require.NotNil(t, activated)
							require.Equal(t, uint64(5), activated.BlockNumber)
						} else {
							require.Nil(t, activated)
							require.Equal(t, before, snapshotStartupFiles(
								t, datadir.ChainData(instance.config.DataDir),
							))
						}

						/* START ALPHA CHANGE 3213 -- preserve existing bootstrap behavior while checking the retained genesis identity -- */
						number, usable, genesisHash, err := instance.localBlockchainHead()
						require.NoError(t, err)
						expected := uint64(0)
						if state.height > 0 {
							expected = uint64(state.height)
						}
						if wantActivation {
							expected = 5
						}
						require.Equal(t, expected, number)
						require.Equal(t, expected > 0, usable)

						if wantActivation || state.height >= 0 {
							genesis := fixture.chain.Genesis.GenesisHeader()
							genesis.StateRoot = fixture.genesisRoot
							genesis.ComputeHash()

							require.NotNil(t, genesisHash)
							require.Equal(t, genesis.Hash, *genesisHash)
						} else {
							require.Nil(t, genesisHash)
						}
						/* FINISH ALPHA CHANGE 3213 -- preserve existing bootstrap behavior while checking the retained genesis identity -- */
					})
				}
			}
		})
	}
}

func TestSnapshotStartup_RejectsMissingBlockchainWithRemainingData(t *testing.T) {
	for _, mode := range []string{"recent", "full"} {
		for _, component := range []string{
			datadir.TrieDirName,
			datadir.ConsensusDirName,
		} {
			t.Run(mode+"/"+component, func(t *testing.T) {
				dataDir := t.TempDir()
				componentPath := filepath.Join(datadir.ChainData(dataDir), component)
				require.NoError(t, os.MkdirAll(componentPath, 0700))
				require.NoError(t, os.WriteFile(
					filepath.Join(componentPath, "existing-data"),
					[]byte("preserve"), 0600,
				))

				before := snapshotStartupFiles(t, datadir.ChainData(dataDir))
				instance := &Server{
					config: &Config{
						DataDir: dataDir,
						History: &HistoryConfig{
							Mode:            mode,
							SegmentBlocks:   2,
							RetentionBlocks: 4,
							MDBXMapSizeGB:   1,
						},
					},
					logger: hclog.NewNullLogger(),
				}

				activated, err := instance.activateConfiguredSnapshot()
				require.ErrorContains(t, err, "contains data")
				require.Nil(t, activated)
				require.NoDirExists(t, datadir.Blockchain(dataDir))
				require.Equal(t, before, snapshotStartupFiles(
					t, datadir.ChainData(dataDir),
				))
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3189 -- reject damaged local history before snapshot replacement and preserve bootstrap policy -- */
