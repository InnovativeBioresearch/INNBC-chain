/* START ALPHA CHANGE 2666 -- test guarded publication on memory and MDBX storage -- */
package itrie

import (
	"errors"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type commitValidationRecord struct {
	key   []byte
	value []byte
	found bool
}

type commitValidationStorage struct {
	Storage
	t        *testing.T
	writes   int
	writeErr error
	before   map[string]commitValidationRecord
}

type commitValidationBatch struct {
	Batch
	owner *commitValidationStorage
}

func (s *commitValidationStorage) Batch() Batch {
	return &commitValidationBatch{
		Batch: s.Storage.Batch(),
		owner: s,
	}
}

func (b *commitValidationBatch) Put(key, value []byte) {
	if _, exists := b.owner.before[string(key)]; !exists {
		previous, found, err := b.owner.Storage.Get(key)
		require.NoError(b.owner.t, err)

		b.owner.before[string(key)] = commitValidationRecord{
			key:   append([]byte(nil), key...),
			value: append([]byte(nil), previous...),
			found: found,
		}
	}

	b.Batch.Put(key, value)
}

func (b *commitValidationBatch) Write() error {
	b.owner.writes++
	if b.owner.writeErr != nil {
		return b.owner.writeErr
	}

	return b.Batch.Write()
}

func commitValidationInitial(t *testing.T, world *State) statepkg.Snapshot {
	t.Helper()

	snapshot := world.NewSnapshot()
	txn := statepkg.NewTxn(snapshot)
	address := types.StringToAddress("0x9101")

	txn.SetBalance(address, big.NewInt(1))
	txn.SetCode(address, []byte{0x60, 0x01})
	txn.SetState(address, types.StringToHash("0x01"), types.StringToHash("0x2a"))

	objects, err := txn.Commit(false)
	require.NoError(t, err)

	committed, _, err := snapshot.Commit(objects)
	require.NoError(t, err)

	return committed
}

func commitValidationChange(snapshot statepkg.Snapshot) *statepkg.Transition {
	txn := statepkg.NewTxn(snapshot)
	address := types.StringToAddress("0x9101")

	txn.SetBalance(address, big.NewInt(2))
	txn.SetNonce(address, 1)
	txn.SetCode(address, []byte{0x60, 0x02})
	txn.SetState(address, types.StringToHash("0x01"), types.StringToHash("0x2b"))

	return statepkg.NewTransition(chain.ForksInTime{}, snapshot, txn)
}

func TestCommitValidation_Publication(t *testing.T) {
	referenceWorld := NewState(NewMemoryStorage())
	reference := commitValidationChange(commitValidationInitial(t, referenceWorld))
	_, expectedRoot, err := reference.Commit()
	require.NoError(t, err)

	for _, backend := range []string{"memory", "MDBX"} {
		t.Run(backend, func(t *testing.T) {
			for _, mode := range []string{"reject", "accept", "write failure"} {
				t.Run(mode, func(t *testing.T) {
					var backing Storage
					if backend == "memory" {
						backing = NewMemoryStorage()
					} else {
						var openErr error
						backing, openErr = NewMDBXStorage(t.TempDir(), hclog.NewNullLogger())
						require.NoError(t, openErr)
					}

					t.Cleanup(func() {
						require.NoError(t, backing.Close())
					})

					storage := &commitValidationStorage{
						Storage: backing,
						t:       t,
						before:  make(map[string]commitValidationRecord),
					}
					world := NewState(storage)
					initial := commitValidationInitial(t, world)
					transition := commitValidationChange(initial)

					storage.writes = 0
					storage.before = make(map[string]commitValidationRecord)
					cacheKeys := append([]interface{}(nil), world.cache.Keys()...)

					rejection := errors.New("incorrect execution commitment")
					writeFailure := errors.New("injected publication failure")
					if mode == "write failure" {
						storage.writeErr = writeFailure
					}

					callbacks := 0
					var calculatedRoot types.Hash
					committed, root, commitErr := transition.CommitWithValidation(
						func(candidate types.Hash) error {
							callbacks++
							calculatedRoot = candidate
							require.Equal(t, expectedRoot, candidate)

							_, found, readErr := backing.Get(candidate.Bytes())
							require.NoError(t, readErr)
							require.False(t, found)
							require.False(t, world.cache.Contains(candidate))

							if mode == "reject" {
								return rejection
							}

							return nil
						},
					)

					require.Equal(t, 1, callbacks)
					require.Equal(t, expectedRoot, calculatedRoot)

					if mode == "accept" {
						require.NoError(t, commitErr)
						require.NotNil(t, committed)
						require.Equal(t, expectedRoot, root)
						require.Equal(t, 1, storage.writes)

						reopened, openErr := NewState(backing).NewSnapshotAt(root)
						require.NoError(t, openErr)

						address := types.StringToAddress("0x9101")
						account, accountErr := reopened.GetAccount(address)
						require.NoError(t, accountErr)
						require.NotNil(t, account)
						require.Equal(t, uint64(1), account.Nonce)
						require.Zero(t, account.Balance.Cmp(big.NewInt(2)))

						value, storageErr := reopened.GetStorage(
							address, account.Root, types.StringToHash("0x01"),
						)
						require.NoError(t, storageErr)
						require.Equal(t, types.StringToHash("0x2b"), value)

						code, found, codeErr := reopened.GetCode(
							types.BytesToHash(account.CodeHash),
						)
						require.NoError(t, codeErr)
						require.True(t, found)
						require.Equal(t, []byte{0x60, 0x02}, code)

						return
					}

					require.Nil(t, committed)
					require.Equal(t, types.ZeroHash, root)
					require.NotEmpty(t, storage.before)
					require.ElementsMatch(t, cacheKeys, world.cache.Keys())

					for _, previous := range storage.before {
						actual, found, readErr := backing.Get(previous.key)
						require.NoError(t, readErr)
						require.Equal(t, previous.found, found)
						if found {
							require.Equal(t, previous.value, actual)
						}
					}

					newCodeHash := types.BytesToHash(crypto.Keccak256([]byte{0x60, 0x02}))
					_, found, readErr := backing.Get(GetCodeKey(newCodeHash))
					require.NoError(t, readErr)
					require.False(t, found)

					if mode == "reject" {
						require.ErrorIs(t, commitErr, rejection)
						require.False(t, errors.Is(commitErr, statepkg.ErrLocalStateFailure))
						require.NoError(t, transition.Error())
						require.Zero(t, storage.writes)
					} else {
						require.ErrorIs(t, commitErr, writeFailure)
						require.ErrorIs(t, commitErr, statepkg.ErrLocalStateFailure)
						require.ErrorIs(t, transition.Error(), writeFailure)
						require.Equal(t, 1, storage.writes)
					}

					writesBeforeRetry := storage.writes
					_, _, retryErr := transition.Commit()
					require.ErrorIs(t, retryErr, map[string]error{
						"reject":        rejection,
						"write failure": writeFailure,
					}[mode])
					require.Equal(t, writesBeforeRetry, storage.writes)

					_, _, retryErr = transition.CommitWithValidation(func(types.Hash) error {
						callbacks++

						return nil
					})
					require.Error(t, retryErr)
					require.Equal(t, 1, callbacks)
					require.Equal(t, writesBeforeRetry, storage.writes)

					account, accountErr := initial.GetAccount(types.StringToAddress("0x9101"))
					require.NoError(t, accountErr)
					require.NotNil(t, account)
					require.Zero(t, account.Balance.Cmp(big.NewInt(1)))
					require.Zero(t, account.Nonce)
				})
			}
		})
	}
}

type commitValidationUnsupportedSnapshot struct {
	statepkg.Snapshot
}

func TestCommitValidation_UnsupportedSnapshotDoesNotPublish(t *testing.T) {
	storage := &commitValidationStorage{
		Storage: NewMemoryStorage(),
		t:       t,
		before:  make(map[string]commitValidationRecord),
	}
	wrapped := &commitValidationUnsupportedSnapshot{
		Snapshot: NewState(storage).NewSnapshot(),
	}
	txn := statepkg.NewTxn(wrapped)
	txn.SetBalance(types.StringToAddress("0x9102"), big.NewInt(1))
	transition := statepkg.NewTransition(chain.ForksInTime{}, wrapped, txn)

	callbacks := 0
	_, _, err := transition.CommitWithValidation(func(types.Hash) error {
		callbacks++

		return nil
	})

	require.ErrorIs(t, err, statepkg.ErrLocalStateFailure)
	require.ErrorContains(t, err, "does not support guarded commit")
	require.Zero(t, callbacks)
	require.Zero(t, storage.writes)
	require.Empty(t, storage.before)
}

func TestMemoryBatch_Staging(t *testing.T) {
	storage := NewMemoryStorage()
	key := []byte{1}
	value := []byte{2}
	batch := storage.Batch()
	batch.Put(key, value)

	key[0] = 9
	value[0] = 9

	_, found, err := storage.Get([]byte{1})
	require.NoError(t, err)
	require.False(t, found)

	require.NoError(t, batch.Write())

	actual, found, err := storage.Get([]byte{1})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, []byte{2}, actual)

	_, found, err = storage.Get([]byte{9})
	require.NoError(t, err)
	require.False(t, found)
}

/* FINISH ALPHA CHANGE 2666 -- test guarded publication on memory and MDBX storage -- */
