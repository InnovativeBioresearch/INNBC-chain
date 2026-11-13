/* START ALPHA CHANGE 2667 -- reject incorrect proposal commitments before publishing executed state -- */
package innbcbft

import (
	"errors"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage/memory"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type proposalCommitValidationStorage struct {
	itrie.Storage
	writes int
}

type proposalCommitValidationBatch struct {
	itrie.Batch
	owner *proposalCommitValidationStorage
}

func (s *proposalCommitValidationStorage) Batch() itrie.Batch {
	return &proposalCommitValidationBatch{
		Batch: s.Storage.Batch(),
		owner: s,
	}
}

func (b *proposalCommitValidationBatch) Write() error {
	b.owner.writes++

	return b.Batch.Write()
}

func newProposalCommitValidationWrapper(
	t *testing.T,
) (*blockchainWrapper, *proposalCommitValidationStorage) {
	t.Helper()

	logger := hclog.NewNullLogger()
	storage := &proposalCommitValidationStorage{
		Storage: itrie.NewMemoryStorage(),
	}
	world := itrie.NewState(storage)
	snapshot := world.NewSnapshot()
	txn := state.NewTxn(snapshot)
	txn.SetBalance(types.StringToAddress("0x9201"), big.NewInt(1_000_000_000))

	objects, err := txn.Commit(false)
	require.NoError(t, err)
	_, root, err := snapshot.Commit(objects)
	require.NoError(t, err)

	extra := (&Extra{Checkpoint: &CheckpointData{}}).MarshalRLPTo(nil)
	config := &chain.Chain{
		Genesis: &chain.Genesis{
			GasLimit:           1_000_000,
			BaseFee:            1,
			BaseFeeEM:          chain.GenesisBaseFeeEM,
			BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
			StateRoot:          types.BytesToHash(root),
			ExtraData:          extra,
		},
		Params: &chain.Params{
			ChainID: 100,
			Forks:   chain.AllForksEnabled,
		},
	}

	executor := state.NewExecutor(config.Params, world, logger)
	/* START ALPHA CHANGE 3810 -- adapt commitment fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt commitment fixture to checked hash lookups -- */

	db, err := memory.NewMemoryStorage(logger)
	require.NoError(t, err)
	bc, err := blockchain.NewBlockchain(
		logger, db, config, &blockchain.MockVerifier{}, executor, nil,
	)
	require.NoError(t, err)
	require.NoError(t, bc.ComputeGenesis())

	storage.writes = 0

	return &blockchainWrapper{
		executor:   executor,
		blockchain: bc,
	}, storage
}

func buildProposalCommitValidationReference(t *testing.T) *types.Block {
	t.Helper()

	wrapper, _ := newProposalCommitValidationWrapper(t)
	parent := wrapper.blockchain.Header()
	block := newValidEmptyProposal(t, wrapper, parent)
	block.Header.ExtraData = (&Extra{Checkpoint: &CheckpointData{}}).MarshalRLPTo(nil)

	/*
		The direct execution fixture supplies its known sender explicitly.
		Transaction-signature admission is outside this storage regression.
	*/
	transaction := (&types.Transaction{
		From:     types.StringToAddress("0x9201"),
		To:       types.StringToAddress("0x9202").Ptr(),
		Gas:      21_000,
		GasPrice: big.NewInt(2),
		Value:    big.NewInt(1),
		V:        new(big.Int),
		R:        new(big.Int),
		S:        new(big.Int),
	}).ComputeHash(block.Number())

	transition, err := wrapper.executor.BeginTxn(
		parent.StateRoot, block.Header, types.ZeroAddress,
	)
	require.NoError(t, err)
	require.NoError(t, transition.Write(transaction))
	_, root, err := transition.Commit()
	require.NoError(t, err)

	block.Header.StateRoot = root
	block.Header.GasUsed = transition.TotalGas()
	block.Header.LogsBloom = types.CreateBloom(transition.Receipts())

	built := consensus.BuildBlock(consensus.BuildBlockParams{
		Header:   block.Header,
		Txns:     []*types.Transaction{transaction},
		Receipts: transition.Receipts(),
	})
	require.NotEqual(t, types.ZeroHash, built.Header.Hash)

	return built
}

func TestProposalCommitValidation_Publication(t *testing.T) {
	reference := buildProposalCommitValidationReference(t)

	cases := []struct {
		name    string
		mutate  func(*types.Header)
		wantErr error
	}{
		{"accepted", func(*types.Header) {}, nil},
		{"state root", func(h *types.Header) {
			h.StateRoot = types.Hash{0x91}
		}, blockchain.ErrInvalidStateRoot},
		{"gas used", func(h *types.Header) {
			h.GasUsed++
		}, blockchain.ErrInvalidGasUsed},
		{"receipt root", func(h *types.Header) {
			h.ReceiptsRoot = types.Hash{0x92}
		}, blockchain.ErrInvalidReceiptsRoot},
		{"bloom", func(h *types.Header) {
			h.LogsBloom[0] ^= 1
		}, blockchain.ErrInvalidLogsBloom},
		{"hash", func(*types.Header) {}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper, storage := newProposalCommitValidationWrapper(t)
			parent := wrapper.blockchain.Header()
			block := &types.Block{
				Header:       reference.Header.Copy(),
				Transactions: []*types.Transaction{reference.Transactions[0].Copy()},
			}
			tc.mutate(block.Header)
			block.Header.ComputeHash()
			if tc.name == "hash" {
				block.Header.Hash = types.Hash{0x93}
			}

			_, found, err := storage.Get(reference.Header.StateRoot.Bytes())
			require.NoError(t, err)
			require.False(t, found)

			result, err := wrapper.ProcessBlock(parent, block)
			if tc.name == "accepted" {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, reference.Header.StateRoot, result.Block.Header.StateRoot)
				require.Equal(t, reference.Header.Hash, result.Block.Header.Hash)
				require.Equal(t, reference.Header.GasUsed, result.Block.Header.GasUsed)
				require.Equal(t, reference.Header.ReceiptsRoot, result.Block.Header.ReceiptsRoot)
				require.Equal(t, reference.Header.LogsBloom, result.Block.Header.LogsBloom)
				require.Len(t, result.Receipts, 1)
				require.Equal(t, 1, storage.writes)

				snapshot, openErr := wrapper.executor.StateAt(result.Block.Header.StateRoot)
				require.NoError(t, openErr)
				account, accountErr := snapshot.GetAccount(types.StringToAddress("0x9201"))
				require.NoError(t, accountErr)
				require.NotNil(t, account)
				require.Equal(t, uint64(1), account.Nonce)

				return
			}

			require.Nil(t, result)
			require.Error(t, err)
			require.False(t, errors.Is(err, state.ErrLocalStateFailure))
			if tc.name == "hash" {
				require.ErrorContains(t, err, "incorrect block hash")
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}

			require.Zero(t, storage.writes)
			_, found, readErr := storage.Get(reference.Header.StateRoot.Bytes())
			require.NoError(t, readErr)
			require.False(t, found)

			snapshot, openErr := wrapper.executor.StateAt(parent.StateRoot)
			require.NoError(t, openErr)
			account, accountErr := snapshot.GetAccount(types.StringToAddress("0x9201"))
			require.NoError(t, accountErr)
			require.NotNil(t, account)
			require.Zero(t, account.Nonce)
			require.Zero(t, account.Balance.Cmp(big.NewInt(1_000_000_000)))
		})
	}
}

/* FINISH ALPHA CHANGE 2667 -- reject incorrect proposal commitments before publishing executed state -- */
