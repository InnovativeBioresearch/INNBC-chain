package innbcbft

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage/memory"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 1255 -- cover direct consensus proposal header validation -- */
func TestBlockchainWrapper_ProcessBlock_RejectsInvalidProposalHeaderFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		mutate         func(*types.Header)
		expectErr      error
		expectContains string
	}{
		{
			name: "base fee",
			mutate: func(header *types.Header) {
				header.BaseFee++
			},
			expectErr: blockchain.ErrInvalidBaseFee,
		},
		{
			name: "gas limit transition",
			mutate: func(header *types.Header) {
				header.GasLimit *= 2
			},
			expectContains: "invalid gas limit",
		},
		{
			name: "sha3 uncles",
			mutate: func(header *types.Header) {
				header.Sha3Uncles = types.ZeroHash
			},
			expectErr: blockchain.ErrInvalidSha3Uncles,
		},
		{
			name: "transaction root",
			mutate: func(header *types.Header) {
				header.TxRoot = types.ZeroHash
			},
			expectErr: blockchain.ErrInvalidTxRoot,
		},
		{
			name: "state root",
			mutate: func(header *types.Header) {
				header.StateRoot = types.Hash{0x1}
			},
			expectErr: blockchain.ErrInvalidStateRoot,
		},
		{
			name: "gas used",
			mutate: func(header *types.Header) {
				header.GasUsed = 1
			},
			expectErr: blockchain.ErrInvalidGasUsed,
		},
		{
			name: "receipts root",
			mutate: func(header *types.Header) {
				header.ReceiptsRoot = types.ZeroHash
			},
			expectErr: blockchain.ErrInvalidReceiptsRoot,
		},
		{
			name: "logs bloom",
			mutate: func(header *types.Header) {
				header.LogsBloom[0] = 1
			},
			expectErr: blockchain.ErrInvalidLogsBloom,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wrapper := newTestBlockchainWrapper(t)
			parent := wrapper.blockchain.Header()
			proposal := newValidEmptyProposal(t, wrapper, parent)

			tt.mutate(proposal.Header)
			proposal.Header.ComputeHash()

			_, err := wrapper.ProcessBlock(parent, proposal)
			require.Error(t, err)

			if tt.expectErr != nil {
				require.True(t, errors.Is(err, tt.expectErr), "expected %v, got %v", tt.expectErr, err)
			}

			if tt.expectContains != "" {
				require.Contains(t, err.Error(), tt.expectContains)
			}
		})
	}
}

func TestBlockchainWrapper_ProcessBlock_AcceptsValidProposal(t *testing.T) {
	t.Parallel()

	wrapper := newTestBlockchainWrapper(t)
	parent := wrapper.blockchain.Header()
	proposal := newValidEmptyProposal(t, wrapper, parent)

	fullBlock, err := wrapper.ProcessBlock(parent, proposal)
	require.NoError(t, err)
	require.Equal(t, proposal.Header.Hash, fullBlock.Block.Header.Hash)
	require.Equal(t, proposal.Header.StateRoot, fullBlock.Block.Header.StateRoot)
	require.Equal(t, proposal.Header.GasUsed, fullBlock.Block.Header.GasUsed)
	require.Equal(t, proposal.Header.ReceiptsRoot, fullBlock.Block.Header.ReceiptsRoot)
	require.Equal(t, proposal.Header.LogsBloom, fullBlock.Block.Header.LogsBloom)
}

func newTestBlockchainWrapper(t *testing.T) *blockchainWrapper {
	t.Helper()

	logger := hclog.NewNullLogger()
	config := &chain.Chain{
		Genesis: &chain.Genesis{
			GasLimit:           1_000_000,
			BaseFee:            1,
			BaseFeeEM:          chain.GenesisBaseFeeEM,
			BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
		},
		Params: &chain.Params{
			ChainID: 100,
			Forks:   chain.AllForksEnabled,
		},
	}

	executor := state.NewExecutor(config.Params, itrie.NewState(itrie.NewMemoryStorage()), logger)
	/* START ALPHA CHANGE 3810 -- adapt proposal wrapper fixture to checked hash lookups -- */
	executor.GetHash = func(header *types.Header) func(i uint64) (types.Hash, error) {
		return func(i uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt proposal wrapper fixture to checked hash lookups -- */

	db, err := memory.NewMemoryStorage(logger)
	require.NoError(t, err)

	bc, err := blockchain.NewBlockchain(
		logger,
		db,
		config,
		&blockchain.MockVerifier{},
		executor,
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, bc.ComputeGenesis())

	return &blockchainWrapper{
		executor:   executor,
		blockchain: bc,
	}
}

func newValidEmptyProposal(t *testing.T, wrapper *blockchainWrapper, parent *types.Header) *types.Block {
	t.Helper()

	baseFee, err := wrapper.blockchain.CalculateBaseFee(parent)
	require.NoError(t, err)

	header := &types.Header{
		ParentHash:   parent.Hash,
		Number:       parent.Number + 1,
		Miner:        types.ZeroAddress.Bytes(),
		Difficulty:   1,
		StateRoot:    parent.StateRoot,
		TxRoot:       types.EmptyRootHash,
		ReceiptsRoot: types.EmptyRootHash,
		Sha3Uncles:   types.EmptyUncleHash,
		GasLimit:     parent.GasLimit,
		GasUsed:      0,
		BaseFee:      baseFee,
		Timestamp:    parent.Timestamp + 1,
		MixHash:      INNBCBFTMixDigest,
	}

	header.ComputeHash()

	return &types.Block{
		Header: header,
	}
}

/* FINISH ALPHA CHANGE 1255 -- cover direct consensus proposal header validation -- */
