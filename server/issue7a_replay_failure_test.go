/* START ALPHA CHANGE 2157 -- test offline replay aborting on local bytecode failure -- */
package server

import (
	"context"
	"errors"
	"testing"

	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type issue7AReplayCodeStorage struct {
	itrie.Storage
	codeErr error
}

func (s *issue7AReplayCodeStorage) GetCode(hash types.Hash) ([]byte, bool, error) {
	if s.codeErr != nil {
		return nil, false, s.codeErr
	}
	return s.Storage.GetCode(hash)
}

func TestIssue7A_TrieReplayPreservesLocalStateFailure(t *testing.T) {
	/* START ALPHA CHANGE 3413 -- retain replay failure injection against current artifacts -- */
	// Build complete canonical history using current contract artifacts.
	fixture := newTrieRebuildFixture(t)
	/* FINISH ALPHA CHANGE 3413 -- retain replay failure injection against current artifacts -- */

	source, err := mdbxstorage.NewMDBXStorageReadOnly(
		datadir.Blockchain(fixture.config.DataDir),
		fixture.config.Logger,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, source.Close())
	})

	genesis, err := readTrieRebuildHeader(source, 0)
	require.NoError(t, err)

	config, err := chain.ImportFromFile(fixture.config.ChainPath)
	require.NoError(t, err)

	storage := &issue7AReplayCodeStorage{Storage: itrie.NewMemoryStorage()}
	executor := newTrieRebuildExecutor(config, storage, fixture.config.Logger)
	root, err := executor.WriteGenesis(config.Genesis.Alloc, types.ZeroHash)
	require.NoError(t, err)
	require.Equal(t, genesis.StateRoot, root)

	cause := errors.New("injected replay bytecode failure")
	storage.codeErr = cause

	err = replayTrieRebuildBlocks(
		context.Background(),
		source,
		executor,
		genesis,
		fixture.head,
		fixture.config.Logger,
	)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.Contains(t, err.Error(), "execute block")

	// Replay must not rewrite the canonical source when execution fails.
	head, err := readTrieRebuildHeader(source, fixture.head.Number)
	require.NoError(t, err)
	require.Equal(t, fixture.head.Hash, head.Hash)
	require.Equal(t, fixture.head.StateRoot, head.StateRoot)
}

/* FINISH ALPHA CHANGE 2157 -- test offline replay aborting on local bytecode failure -- */
