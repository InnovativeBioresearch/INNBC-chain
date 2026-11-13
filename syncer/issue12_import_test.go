/* START ALPHA CHANGE 2300 -- test certified malformed blocks through real finalized import -- */
package syncer_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage/memory"
	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/bitmap"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/syncer"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

// Supply a fixed trusted validator set while delegating certificate and
// checkpoint validation to the production implementation. The embedded engine
// provides the production block-creator, pre-commit, and ProcessHeaders methods.
type issue12ImportConsensus struct {
	*innbcbft.INNBCBFT
	blockchain *blockchain.Blockchain
	validators validator.AccountSet
}

func (v *issue12ImportConsensus) GetValidators(
	blockNumber uint64,
	parents []*types.Header,
) (validator.AccountSet, error) {
	if blockNumber != 0 || len(parents) != 0 {
		return nil, fmt.Errorf("unexpected validator lookup at block %d", blockNumber)
	}

	return v.validators.Copy(), nil
}

func (v *issue12ImportConsensus) VerifyHeader(header *types.Header) error {
	if header.Hash != types.HeaderHash(header) {
		return fmt.Errorf("invalid fixture header hash")
	}

	parent, ok := v.blockchain.GetHeaderByHash(header.ParentHash)
	if !ok {
		return blockchain.ErrParentNotFound
	}

	extra, err := innbcbft.GetIbftExtra(header.ExtraData)
	if err != nil {
		return err
	}

	return extra.ValidateFinalizedData(
		header,
		parent,
		nil,
		uint64(v.blockchain.Config().ChainID),
		v,
		signer.DomainCheckpointManager,
		hclog.NewNullLogger(),
	)
}

type issue12ImportExecutor struct {
	*state.Executor
	executions int
}

func (e *issue12ImportExecutor) ProcessBlock(
	parentRoot types.Hash,
	block *types.Block,
	creator types.Address,
) (*state.Transition, error) {
	e.executions++

	return e.Executor.ProcessBlock(parentRoot, block, creator)
}

// Observe calls without substituting their validation or persistence behavior.
type issue12ImportChain struct {
	*blockchain.Blockchain
	verifications int
	writes        int
}

func (b *issue12ImportChain) VerifyFinalizedBlock(
	block *types.Block,
) (*types.FullBlock, error) {
	b.verifications++

	return b.Blockchain.VerifyFinalizedBlock(block)
}

func (b *issue12ImportChain) WriteFullBlock(
	block *types.FullBlock,
	source string,
) error {
	b.writes++

	return b.Blockchain.WriteFullBlock(block, source)
}

func issue12SignImportCheckpoint(
	t *testing.T,
	accounts []*wallet.Account,
	hash types.Hash,
) *innbcbft.Signature {
	t.Helper()

	var signatures bls.Signatures
	var signers bitmap.Bitmap

	for index, account := range accounts {
		signature, err := account.Bls.Sign(hash[:], signer.DomainCheckpointManager)
		require.NoError(t, err)

		signatures = append(signatures, signature)
		signers.Set(uint64(index))
	}

	aggregated, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)

	return &innbcbft.Signature{
		AggregatedSignature: aggregated,
		Bitmap:              signers,
	}
}

func TestIssue12_SyncImportRejectsCertifiedInconsistency(t *testing.T) {
	// Do not run in parallel: header hashing is process-global.
	// Apply the same extra-data normalization used by INNBCBFT, scoped to
	// this test, without changing the production setup function's sync.Once.
	originalHeaderHash := types.HeaderHash
	types.HeaderHash = func(header *types.Header) types.Hash {
		cleanExtra, err := innbcbft.GetIbftExtraClean(header.ExtraData)
		if err != nil {
			return types.ZeroHash
		}

		cleanHeader := header.Copy()
		cleanHeader.ExtraData = cleanExtra

		return originalHeaderHash(cleanHeader)
	}
	t.Cleanup(func() {
		types.HeaderHash = originalHeaderHash
	})

	const chainID = 100

	identities := validator.NewTestValidators(t, 6)
	validators := identities.GetPublicIdentities()
	validatorHash, err := validators.Hash()
	require.NoError(t, err)

	tests := []struct {
		name           string
		mutate         func(*types.Header, *innbcbft.Extra)
		wantError      string
		wantErrorIs    error
		wantExecutions int
	}{
		{
			name:           "valid certified block is imported",
			wantExecutions: 1,
		},
		{
			name: "incorrect bloom is rejected after execution",
			mutate: func(header *types.Header, _ *innbcbft.Extra) {
				header.LogsBloom[0] = 1
			},
			wantError:      "invalid block logs bloom",
			wantErrorIs:    blockchain.ErrInvalidLogsBloom,
			wantExecutions: 1,
		},
		{
			name: "incorrect current validator hash is rejected",
			mutate: func(_ *types.Header, extra *innbcbft.Extra) {
				extra.Checkpoint.CurrentValidatorsHash = types.BytesToHash([]byte{1})
			},
			wantError: "current validators hashes don't match",
		},
		{
			name: "incorrect next validator hash is rejected",
			mutate: func(_ *types.Header, extra *innbcbft.Extra) {
				extra.Checkpoint.NextValidatorsHash = types.BytesToHash([]byte{2})
			},
			wantError: "next validators hashes don't match",
		},
		{
			name: "nonzero checkpoint event root is rejected",
			mutate: func(_ *types.Header, extra *innbcbft.Extra) {
				extra.Checkpoint.EventRoot = types.BytesToHash([]byte{3})
			},
			wantError: "exit root hash not as expected",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := hclog.NewNullLogger()
			genesisExtra := &innbcbft.Extra{
				Validators: &validator.ValidatorSetDelta{Added: validators.Copy()},
				Checkpoint: &innbcbft.CheckpointData{
					EpochNumber:           0,
					CurrentValidatorsHash: validatorHash,
					NextValidatorsHash:    validatorHash,
				},
			}
			config := &chain.Chain{
				Genesis: &chain.Genesis{
					GasLimit:           2 * types.StateTransactionGasLimit,
					BaseFee:            1,
					BaseFeeEM:          chain.GenesisBaseFeeEM,
					BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
					ExtraData:          genesisExtra.MarshalRLPTo(nil),
				},
				Params: &chain.Params{
					ChainID: chainID,
					Forks:   chain.AllForksEnabled,
				},
			}

			executor := state.NewExecutor(
				config.Params,
				itrie.NewState(itrie.NewMemoryStorage()),
				logger,
			)
			/* START ALPHA CHANGE 3810 -- adapt sync import fixture to checked hash lookups -- */
			executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
				return func(uint64) (types.Hash, error) {
					return types.ZeroHash, nil
				}
			}
			/* FINISH ALPHA CHANGE 3810 -- adapt sync import fixture to checked hash lookups -- */

			genesisRoot, err := executor.WriteGenesis(nil, types.ZeroHash)
			require.NoError(t, err)
			config.Genesis.StateRoot = genesisRoot

			countedExecutor := &issue12ImportExecutor{Executor: executor}
			consensus := &issue12ImportConsensus{
				INNBCBFT:   new(innbcbft.INNBCBFT),
				validators: validators.Copy(),
			}

			db, err := memory.NewMemoryStorage(logger)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			bc, err := blockchain.NewBlockchain(
				logger, db, config, consensus, countedExecutor, nil,
			)
			require.NoError(t, err)
			consensus.blockchain = bc
			executor.GetHash = bc.GetHashHelper
			require.NoError(t, bc.ComputeGenesis())

			parent := bc.Header().Copy()
			baseFee, err := bc.CalculateBaseFee(parent)
			require.NoError(t, err)

			extra := &innbcbft.Extra{
				Checkpoint: &innbcbft.CheckpointData{
					EpochNumber:           1,
					CurrentValidatorsHash: validatorHash,
					NextValidatorsHash:    validatorHash,
				},
			}
			header := &types.Header{
				ParentHash:   parent.Hash,
				Number:       1,
				Miner:        types.ZeroAddress.Bytes(),
				Difficulty:   1,
				StateRoot:    parent.StateRoot,
				TxRoot:       types.EmptyRootHash,
				ReceiptsRoot: types.EmptyRootHash,
				Sha3Uncles:   types.EmptyUncleHash,
				GasLimit:     parent.GasLimit,
				BaseFee:      baseFee,
				Timestamp:    parent.Timestamp + 1,
				MixHash:      innbcbft.INNBCBFTMixDigest,
			}

			// Introduce the inconsistency before calculating the block hash
			// and before signing. Every case has a valid certificate.
			if test.mutate != nil {
				test.mutate(header, extra)
			}

			header.ExtraData = extra.MarshalRLPTo(nil)
			header.ComputeHash()
			require.NotEqual(t, types.ZeroHash, header.Hash)

			checkpointHash, err := extra.Checkpoint.Hash(
				chainID, header.Number, header.Hash,
			)
			require.NoError(t, err)

			extra.Committed = issue12SignImportCheckpoint(
				t, identities.GetPrivateIdentities(), checkpointHash,
			)
			require.NoError(t, extra.Committed.Verify(
				header.Number,
				validators,
				checkpointHash,
				signer.DomainCheckpointManager,
				logger,
			))

			header.ExtraData = extra.MarshalRLPTo(nil)
			candidate := &types.Block{Header: header}

			// Include the serialization boundary used for downloaded blocks.
			var downloaded types.Block
			require.NoError(t, downloaded.UnmarshalRLP(candidate.MarshalRLP()))
			require.Equal(t, candidate.Hash(), downloaded.Hash())

			observed := &issue12ImportChain{Blockchain: bc}
			callbacks := 0

			last, stopped, err := syncer.Issue12RunBulkSyncForTest(
				t,
				observed,
				&downloaded,
				func(imported *types.FullBlock) bool {
					callbacks++
					require.Equal(t, downloaded.Hash(), imported.Block.Hash())

					return true
				},
			)

			require.Equal(t, 1, observed.verifications)
			require.Equal(t, test.wantExecutions, countedExecutor.executions)

			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				if test.wantErrorIs != nil {
					require.ErrorIs(t, err, test.wantErrorIs)
				}

				require.False(t, stopped)
				require.Equal(t, parent.Number, last)
				require.Zero(t, observed.writes)
				require.Zero(t, callbacks)

				// Neither the in-memory head nor persisted canonical head
				// may advance after rejection.
				require.Equal(t, parent.Hash, bc.Header().Hash)
				require.Equal(t, parent.Number, bc.Header().Number)

				persistedHead, ok := db.ReadHeadHash()
				require.True(t, ok)
				require.Equal(t, parent.Hash, persistedHead)

				_, ok = db.ReadCanonicalHash(downloaded.Number())
				require.False(t, ok)

				_, ok = bc.GetHeaderByHash(downloaded.Hash())
				require.False(t, ok)

				return
			}

			require.NoError(t, err)
			require.True(t, stopped)
			require.Equal(t, downloaded.Number(), last)
			require.Equal(t, 1, observed.writes)
			require.Equal(t, 1, callbacks)
			require.Equal(t, downloaded.Hash(), bc.Header().Hash)

			persistedHead, ok := db.ReadHeadHash()
			require.True(t, ok)
			require.Equal(t, downloaded.Hash(), persistedHead)

			canonicalHash, ok := db.ReadCanonicalHash(downloaded.Number())
			require.True(t, ok)
			require.Equal(t, downloaded.Hash(), canonicalHash)

			storedBlock, ok := bc.GetBlockByNumber(downloaded.Number(), true)
			require.True(t, ok)
			require.Equal(t, downloaded.Hash(), storedBlock.Hash())
		})
	}
}

/* FINISH ALPHA CHANGE 2300 -- test certified malformed blocks through real finalized import -- */
