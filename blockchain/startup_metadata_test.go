/* START ALPHA CHANGE 3171 -- verify damaged startup metadata never causes genesis or head rewriting -- */
package blockchain

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage/memory"
	segmentedstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/segmented"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type startupMetadataKV struct {
	data     map[string][]byte
	readErr  map[string]error
	emptyErr error
	writes   int
}

func (kv *startupMetadataKV) Get(key []byte) ([]byte, bool, error) {
	if err := kv.readErr[string(key)]; err != nil {
		return nil, false, err
	}
	value, found := kv.data[string(key)]
	return append([]byte(nil), value...), found, nil
}

func (kv *startupMetadataKV) IsEmpty() (bool, error) {
	return len(kv.data) == 0, kv.emptyErr
}

func (*startupMetadataKV) Close() error { return nil }

func (kv *startupMetadataKV) NewBatch() storage.Batch {
	return &startupMetadataBatch{kv: kv, puts: make(map[string][]byte)}
}

type startupMetadataBatch struct {
	kv      *startupMetadataKV
	puts    map[string][]byte
	deletes []string
}

func (b *startupMetadataBatch) Put(key, value []byte) {
	b.puts[string(key)] = append([]byte(nil), value...)
}

func (b *startupMetadataBatch) Delete(key []byte) {
	b.deletes = append(b.deletes, string(key))
}

func (b *startupMetadataBatch) Write() error {
	b.kv.writes++
	for _, key := range b.deletes {
		delete(b.kv.data, key)
	}
	for key, value := range b.puts {
		b.kv.data[key] = value
	}
	return nil
}

func startupMetadataKey(prefix, key []byte) []byte {
	return append(append([]byte(nil), prefix...), key...)
}

func startupMetadataRecords(kv *startupMetadataKV) map[string][]byte {
	result := make(map[string][]byte, len(kv.data))
	for key, value := range kv.data {
		result[key] = append([]byte(nil), value...)
	}
	return result
}

func startupMetadataChain(t *testing.T, db storage.Storage) *Blockchain {
	t.Helper()
	b, err := NewBlockchain(
		hclog.NewNullLogger(),
		db,
		&chain.Chain{
			Genesis: &chain.Genesis{GasLimit: 5_242_880, BaseFee: 1},
			Params:  &chain.Params{ChainID: 1982, Forks: chain.AllForksEnabled},
		},
		nil, nil, nil,
	)
	require.NoError(t, err)
	return b
}

func seedStartupMetadata(
	t *testing.T,
	db storage.Storage,
) (*types.Header, *types.Header) {
	t.Helper()
	genesis := (&chain.Genesis{GasLimit: 5_242_880, BaseFee: 1}).GenesisHeader()
	genesis.ComputeHash()
	writer := storage.NewBatchWriter(db)
	writer.PutCanonicalHeader(genesis)
	require.NoError(t, writer.WriteBatch())

	// A sparse retained suffix must not require scanning missing intermediate blocks.
	head := genesis.Copy()
	head.Number = 42
	head.Timestamp = 84
	head.ParentHash = genesis.Hash
	head.ComputeHash()
	writer = storage.NewBatchWriter(db)
	writer.PutCanonicalHeader(head)
	require.NoError(t, writer.WriteBatch())
	return genesis, head
}

func TestStartupMetadata_RejectsDamagedRecordsWithoutWrites(t *testing.T) {
	for _, fault := range []string{
		"missing_head_hash",
		"missing_head_number",
		"short_head_hash",
		"short_head_number",
		"missing_genesis_mapping",
		"short_genesis_mapping",
		"missing_canonical_head",
		"short_canonical_head",
		"canonical_head_mismatch",
		"header_number_mismatch",
		"missing_head_header",
		"corrupt_head_header",
		"head_header_hash_mismatch",
		"missing_genesis_header",
		"all_identity_metadata_missing",
		"orphan_record_only",
	} {
		t.Run(fault, func(t *testing.T) {
			kv := &startupMetadataKV{data: make(map[string][]byte)}
			db := storage.NewKeyValueStorage(hclog.NewNullLogger(), kv)
			genesis, head := seedStartupMetadata(t, db)

			headHashKey := string(startupMetadataKey(storage.HEAD, storage.HASH))
			headNumberKey := string(startupMetadataKey(storage.HEAD, storage.NUMBER))
			genesisKey := string(startupMetadataKey(
				storage.CANONICAL, common.EncodeUint64ToBytes(0)))
			canonicalKey := string(startupMetadataKey(
				storage.CANONICAL, common.EncodeUint64ToBytes(head.Number)))
			headerKey := string(startupMetadataKey(storage.HEADER, head.Hash.Bytes()))

			switch fault {
			case "missing_head_hash":
				delete(kv.data, headHashKey)
			case "missing_head_number":
				delete(kv.data, headNumberKey)
			case "short_head_hash":
				kv.data[headHashKey] = []byte{1}
			case "short_head_number":
				kv.data[headNumberKey] = []byte{1}
			case "missing_genesis_mapping":
				delete(kv.data, genesisKey)
			case "short_genesis_mapping":
				kv.data[genesisKey] = []byte{1}
			case "missing_canonical_head":
				delete(kv.data, canonicalKey)
			case "short_canonical_head":
				kv.data[canonicalKey] = []byte{1}
			case "canonical_head_mismatch":
				kv.data[canonicalKey] = genesis.Hash.Bytes()
			case "header_number_mismatch":
				kv.data[headNumberKey] = common.EncodeUint64ToBytes(41)
				kv.data[string(startupMetadataKey(
					storage.CANONICAL, common.EncodeUint64ToBytes(41)))] = head.Hash.Bytes()
			case "missing_head_header":
				delete(kv.data, headerKey)
			case "corrupt_head_header":
				kv.data[headerKey] = []byte{0xff}
			case "head_header_hash_mismatch":
				wrong := head.Copy()
				wrong.Timestamp++
				wrong.ComputeHash()
				kv.data[headerKey] = wrong.MarshalStoreRLPTo(nil)
			case "missing_genesis_header":
				delete(kv.data, string(startupMetadataKey(storage.HEADER, genesis.Hash.Bytes())))
			case "all_identity_metadata_missing":
				delete(kv.data, headHashKey)
				delete(kv.data, headNumberKey)
				delete(kv.data, genesisKey)
				delete(kv.data, canonicalKey)
			case "orphan_record_only":
				kv.data = map[string][]byte{"orphan": {1}}
			}

			before := startupMetadataRecords(kv)
			kv.writes = 0
			b := startupMetadataChain(t, db)
			require.Error(t, b.ComputeGenesis())
			require.Zero(t, kv.writes)
			require.Equal(t, before, kv.data)
			require.Nil(t, b.Header())
			require.Equal(t, types.ZeroHash, b.genesis)
		})
	}
}

func TestStartupMetadata_ReadErrorsRemainErrors(t *testing.T) {
	for _, operation := range []string{
		"emptiness", "head_hash", "head_number", "genesis_mapping",
		"canonical_head", "head_header", "genesis_header",
	} {
		t.Run(operation, func(t *testing.T) {
			kv := &startupMetadataKV{
				data:    make(map[string][]byte),
				readErr: make(map[string]error),
			}
			db := storage.NewKeyValueStorage(hclog.NewNullLogger(), kv)
			genesis, head := seedStartupMetadata(t, db)
			cause := errors.New("injected startup read failure")

			keys := map[string][]byte{
				"head_hash":       startupMetadataKey(storage.HEAD, storage.HASH),
				"head_number":     startupMetadataKey(storage.HEAD, storage.NUMBER),
				"genesis_mapping": startupMetadataKey(storage.CANONICAL, common.EncodeUint64ToBytes(0)),
				"canonical_head":  startupMetadataKey(storage.CANONICAL, common.EncodeUint64ToBytes(head.Number)),
				"head_header":     startupMetadataKey(storage.HEADER, head.Hash.Bytes()),
				"genesis_header":  startupMetadataKey(storage.HEADER, genesis.Hash.Bytes()),
			}
			if operation == "emptiness" {
				kv.emptyErr = cause
			} else {
				kv.readErr[string(keys[operation])] = cause
			}

			before := startupMetadataRecords(kv)
			kv.writes = 0
			b := startupMetadataChain(t, db)
			require.ErrorIs(t, b.ComputeGenesis(), cause)
			require.Zero(t, kv.writes)
			require.Equal(t, before, kv.data)
			require.Nil(t, b.Header())
		})
	}
}

func TestStartupMetadata_EmptyAndExistingStorage(t *testing.T) {
	kv := &startupMetadataKV{data: make(map[string][]byte)}
	db := storage.NewKeyValueStorage(hclog.NewNullLogger(), kv)
	b := startupMetadataChain(t, db)

	require.NoError(t, b.ComputeGenesis())
	require.Equal(t, uint64(0), b.Header().Number)
	require.Equal(t, 1, kv.writes)
	require.NoError(t, b.ComputeGenesis())
	require.Equal(t, 1, kv.writes)

	_, head := seedStartupMetadata(t, db)
	kv.writes = 0
	b = startupMetadataChain(t, db)
	require.NoError(t, b.ComputeGenesis())
	require.Equal(t, head.Hash, b.Header().Hash)
	require.Zero(t, kv.writes)

	b = startupMetadataChain(t, db)
	b.config.Genesis.GasLimit++
	require.ErrorContains(t, b.ComputeGenesis(), "genesis file does not match")
	require.Zero(t, kv.writes)
}

type startupMetadataCountingStorage struct {
	storage.Storage
	writes int
}

type startupMetadataCountingBatch struct {
	storage.Batch
	owner *startupMetadataCountingStorage
}

func (s *startupMetadataCountingStorage) NewBatch() storage.Batch {
	return &startupMetadataCountingBatch{Batch: s.Storage.NewBatch(), owner: s}
}

func (s *startupMetadataCountingStorage) NewBatchForBlock(number uint64) storage.Batch {
	batch := s.Storage.NewBatch
	if targeted, ok := s.Storage.(storage.BlockBatchStorage); ok {
		return &startupMetadataCountingBatch{
			Batch: targeted.NewBatchForBlock(number), owner: s,
		}
	}
	return &startupMetadataCountingBatch{Batch: batch(), owner: s}
}

func (b *startupMetadataCountingBatch) Write() error {
	if err := b.Batch.Write(); err != nil {
		return err
	}
	b.owner.writes++
	return nil
}

func TestStartupMetadata_RealBackendsAndReopen(t *testing.T) {
	for _, backend := range []string{"memory", "mdbx", "segmented"} {
		for _, missing := range []string{"head_hash", "head_number"} {
			t.Run(backend+"/"+missing, func(t *testing.T) {
				path := t.TempDir()
				logger := hclog.NewNullLogger()
				open := func() (storage.Storage, error) {
					switch backend {
					case "memory":
						return memory.NewMemoryStorage(logger)
					case "mdbx":
						return mdbxstorage.NewMDBXStorageWithMapSize(path, logger, 64*1024*1024)
					default:
						return segmentedstorage.NewStorage(path, logger, 2, 4, 256*1024*1024)
					}
				}

				db, err := open()
				require.NoError(t, err)
				t.Cleanup(func() {
					if db != nil {
						require.NoError(t, db.Close())
					}
				})

				b := startupMetadataChain(t, db)
				require.NoError(t, b.ComputeGenesis())
				parent := b.Header().Copy()
				for number := uint64(1); number <= 5; number++ {
					head := parent.Copy()
					head.Number = number
					head.Timestamp = number * 2
					head.ParentHash = parent.Hash
					head.ComputeHash()
					writer := storage.NewBatchWriterForBlock(db, number)
					writer.PutCanonicalHeader(head)
					require.NoError(t, writer.WriteBatch())
					parent = head
				}

				spy := &startupMetadataCountingStorage{Storage: db}
				b = startupMetadataChain(t, spy)
				require.NoError(t, b.ComputeGenesis())
				require.Equal(t, uint64(5), b.Header().Number)
				require.Zero(t, spy.writes)

				key := storage.HASH
				if missing == "head_number" {
					key = storage.NUMBER
				}
				var batch storage.Batch
				if targeted, ok := db.(storage.BlockBatchStorage); ok {
					batch = targeted.NewBatchForBlock(5)
				} else {
					batch = db.NewBatch()
				}
				batch.Delete(startupMetadataKey(storage.HEAD, key))
				require.NoError(t, batch.Write())

				if backend != "memory" {
					require.NoError(t, db.Close())
					db = nil
					db, err = open()
					require.NoError(t, err)
				}

				beforeHash, beforeHasHash := db.ReadHeadHash()
				beforeNumber, beforeHasNumber := db.ReadHeadNumber()
				spy = &startupMetadataCountingStorage{Storage: db}
				b = startupMetadataChain(t, spy)
				require.Error(t, b.ComputeGenesis())
				require.Zero(t, spy.writes)
				require.Nil(t, b.Header())

				afterHash, afterHasHash := db.ReadHeadHash()
				afterNumber, afterHasNumber := db.ReadHeadNumber()
				require.Equal(t, beforeHash, afterHash)
				require.Equal(t, beforeHasHash, afterHasHash)
				require.Equal(t, beforeNumber, afterNumber)
				require.Equal(t, beforeHasNumber, afterHasNumber)

				canonical, found := db.ReadCanonicalHash(5)
				require.True(t, found)
				require.Equal(t, parent.Hash, canonical)
				stored, err := db.ReadHeader(parent.Hash)
				require.NoError(t, err)
				require.Equal(t, parent.Hash, stored.Hash)
				genesis, found := db.ReadCanonicalHash(0)
				require.True(t, found)
				require.Equal(t, b.config.Genesis.Hash(), genesis)
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3171 -- verify damaged startup metadata never causes genesis or head rewriting -- */
