/* START ALPHA CHANGE 2595 -- cover canonical identity checks before storage access -- */
package blockchain

import (
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	/* START ALPHA CHANGE 2617 -- inspect targeted canonical reads in confirmation regressions -- */
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	/* FINISH ALPHA CHANGE 2617 -- inspect targeted canonical reads in confirmation regressions -- */
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestWriteFullBlockConfirmed_ExistingHead(t *testing.T) {
	candidate := &types.FullBlock{
		Block: &types.Block{
			Header: &types.Header{Number: 7, Hash: types.Hash{1}},
		},
	}

	for _, test := range []struct {
		name string
		head *types.Header
		ok   bool
	}{
		{"exact", candidate.Block.Header.Copy(), true},
		{"conflicting", &types.Header{Number: 7, Hash: types.Hash{2}}, false},
		{"advanced", &types.Header{Number: 8, Hash: types.Hash{3}}, false},
		{"uninitialized", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			/*
				No database or consensus backend is installed. These cases
				must complete before any storage or snapshot update.
			*/
			b := &Blockchain{logger: hclog.NewNullLogger()}
			if test.head != nil {
				b.setCurrentHeader(test.head)
			}

			err := b.WriteFullBlockConfirmed(candidate, "consensus")
			if test.ok {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrCanonicalBlockNotConfirmed)
			}
		})
	}
}

func TestWriteFullBlockConfirmed_MissingCandidate(t *testing.T) {
	b := &Blockchain{logger: hclog.NewNullLogger()}

	for _, candidate := range []*types.FullBlock{
		nil,
		{},
		{Block: &types.Block{}},
	} {
		require.ErrorIs(t,
			b.WriteFullBlockConfirmed(candidate, "consensus"),
			ErrCanonicalBlockNotConfirmed,
		)
	}
}

func TestWriteFullBlockConfirmed_HeadAdvancesBeforeLock(t *testing.T) {
	b := &Blockchain{logger: hclog.NewNullLogger()}
	b.setCurrentHeader(&types.Header{Number: 6, Hash: types.Hash{1}})
	candidate := &types.FullBlock{
		Block: &types.Block{
			Header: &types.Header{Number: 7, Hash: types.Hash{2}},
		},
	}

	b.writeLock.Lock()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- b.WriteFullBlockConfirmed(candidate, "consensus")
	}()

	<-started
	b.setCurrentHeader(&types.Header{Number: 8, Hash: types.Hash{3}})
	b.writeLock.Unlock()

	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrCanonicalBlockNotConfirmed)
	case <-time.After(5 * time.Second):
		t.Fatal("confirmed write did not complete")
	}
	require.Equal(t, uint64(8), b.Header().Number)
}

func TestWriteFullBlock_LegacyDuplicateBehavior(t *testing.T) {
	b := &Blockchain{logger: hclog.NewNullLogger()}
	b.setCurrentHeader(&types.Header{Number: 8, Hash: types.Hash{3}})

	require.NoError(t, b.WriteFullBlock(&types.FullBlock{
		Block: &types.Block{
			Header: &types.Header{Number: 7, Hash: types.Hash{2}},
		},
	}, "syncer"))
}

/* FINISH ALPHA CHANGE 2595 -- cover canonical identity checks before storage access -- */

/* START ALPHA CHANGE 2618 -- confirm retained history without scans, rewrites, or height-gap limits -- */
func TestWriteFullBlockConfirmed_RetainedCanonicalHistory(t *testing.T) {
	for _, test := range []struct {
		name      string
		head      uint64
		canonical types.Hash
		found     bool
		ok        bool
	}{
		{"matching", 8, types.Hash{1}, true, true},
		{"large height gap", ^uint64(0), types.Hash{1}, true, true},
		{"conflicting", 8, types.Hash{2}, true, false},
		{"unavailable", 8, types.Hash{}, false, false},
	} {
		for _, mode := range []string{"write confirmation", "confirmation only"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				reads := 0
				db := storage.NewMockStorage()
				db.HookReadCanonicalHash(func(number uint64) (types.Hash, bool) {
					reads++
					require.Equal(t, uint64(7), number)

					return test.canonical, test.found
				})
				db.HookNewBatch(func() storage.Batch {
					t.Fatal("confirmation attempted a storage write")

					return nil
				})
				db.HookReadHeader(func(types.Hash) (*types.Header, error) {
					t.Fatal("confirmation attempted to load a header")

					return nil, nil
				})

				b := &Blockchain{db: db, logger: hclog.NewNullLogger()}
				b.setCurrentHeader(&types.Header{Number: test.head, Hash: types.Hash{3}})
				candidate := &types.FullBlock{
					Block: &types.Block{
						Header: &types.Header{Number: 7, Hash: types.Hash{1}},
					},
				}

				var err error
				if mode == "write confirmation" {
					err = b.WriteFullBlockConfirmed(candidate, "consensus")
				} else {
					err = b.ConfirmCanonicalBlock(7, candidate.Block.Hash())
				}

				if test.ok {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, ErrCanonicalBlockNotConfirmed)
				}
				require.Equal(t, 1, reads)
				require.Equal(t, test.head, b.Header().Number)
				require.Equal(t, types.Hash{3}, b.Header().Hash)
			})
		}
	}
}

func TestConfirmCanonicalBlock_HeadIdentity(t *testing.T) {
	for _, test := range []struct {
		name string
		head *types.Header
		ok   bool
	}{
		{"exact", &types.Header{Number: 7, Hash: types.Hash{1}}, true},
		{"conflicting", &types.Header{Number: 7, Hash: types.Hash{2}}, false},
		{"not yet covered", &types.Header{Number: 6, Hash: types.Hash{3}}, false},
		{"uninitialized", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := &Blockchain{logger: hclog.NewNullLogger()}
			if test.head != nil {
				b.setCurrentHeader(test.head)
			}

			err := b.ConfirmCanonicalBlock(7, types.Hash{1})
			if test.ok {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrCanonicalBlockNotConfirmed)
			}
		})
	}
}

func TestWriteFullBlockConfirmed_MatchingHistoryAfterLock(t *testing.T) {
	reads := 0
	db := storage.NewMockStorage()
	db.HookReadCanonicalHash(func(number uint64) (types.Hash, bool) {
		reads++
		if number != 7 {
			return types.Hash{}, false
		}

		return types.Hash{2}, true
	})
	b := &Blockchain{db: db, logger: hclog.NewNullLogger()}
	b.setCurrentHeader(&types.Header{Number: 6, Hash: types.Hash{1}})
	candidate := &types.FullBlock{
		Block: &types.Block{
			Header: &types.Header{Number: 7, Hash: types.Hash{2}},
		},
	}

	b.writeLock.Lock()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- b.WriteFullBlockConfirmed(candidate, "consensus")
	}()

	<-started
	b.setCurrentHeader(&types.Header{Number: 8, Hash: types.Hash{3}})
	b.writeLock.Unlock()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("confirmed write did not complete")
	}
	require.Equal(t, 1, reads)
	require.Equal(t, uint64(8), b.Header().Number)
}

/* FINISH ALPHA CHANGE 2618 -- confirm retained history without scans, rewrites, or height-gap limits -- */
