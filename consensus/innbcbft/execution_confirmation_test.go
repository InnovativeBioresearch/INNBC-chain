/* START ALPHA CHANGE 2597 -- test canonical head changes after runtime selection checks -- */
package innbcbft

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/innovativebioresearch/innbc-chain/types"

	/* START ALPHA CHANGE 2624 -- verify that canonical completion does not call the execution mock's writer -- */
	"github.com/stretchr/testify/mock"
	/* FINISH ALPHA CHANGE 2624 -- verify that canonical completion does not call the execution mock's writer -- */
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2621 -- count completion confirmation separately from canonical writes -- */
type executionConfirmationBackend struct {
	blockchainBackend
	commit  func(*types.FullBlock) error
	confirm func(uint64, types.Hash) error
}

func (b *executionConfirmationBackend) CommitBlock(block *types.FullBlock) error {
	return b.commit(block)
}

func (b *executionConfirmationBackend) ConfirmCanonicalBlock(number uint64, hash types.Hash) error {
	return b.confirm(number, hash)
}

/* FINISH ALPHA CHANGE 2621 -- count completion confirmation separately from canonical writes -- */

func TestExecutionConfirmation_HeadChangesDuringCommit(t *testing.T) {
	for _, name := range []string{"exact", "conflicting", "advanced", "advanced after confirmation"} {
		t.Run(name, func(t *testing.T) {
			runtime, chain, validators := newExecutionOwnershipRuntime(t)
			block := executionOwnershipBlock(t, runtime, 1)
			raw := block.Block.MarshalRLP()

			canonical := blockchain.NewTestBlockchain(t, []*types.Header{runtime.fsm.parent})
			t.Cleanup(func() { require.NoError(t, canonical.Close()) })
			writer := &blockchainWrapper{blockchain: canonical}

			syncBlock := block
			if name == "conflicting" {
				syncBlock = executionOwnershipBlock(t, runtime, 2)
			}
			synced := []*types.FullBlock{syncBlock}
			if name == "advanced" || name == "advanced after confirmation" {
				header := block.Block.Header.Copy()
				header.Number++
				header.ParentHash = block.Block.Hash()
				header.ComputeHash()
				synced = append(synced, &types.FullBlock{
					Block: &types.Block{Header: header},
				})
			}

			commits := 0
			backend := &executionConfirmationBackend{blockchainBackend: chain}
			backend.commit = func(candidate *types.FullBlock) error {
				commits++

				if name == "advanced after confirmation" {
					if err := writer.CommitBlock(candidate); err != nil {
						return err
					}
					require.NoError(t, canonical.WriteFullBlock(synced[1], "syncer"))

					runtime.lock.Lock()
					runtime.lastBuiltBlock = canonical.Header().Copy()
					runtime.lock.Unlock()

					return nil
				}

				/*
					Execution has already passed the runtime parent check.
					Advance the real canonical writer before its commit.
				*/
				for _, fullBlock := range synced {
					require.NoError(t, canonical.WriteFullBlock(fullBlock, "syncer"))
				}

				runtime.lock.Lock()
				runtime.lastBuiltBlock = canonical.Header().Copy()
				runtime.lock.Unlock()

				return writer.CommitBlock(candidate)
			}
			runtime.fsm.backend = backend

			expectExecutionOwnershipBlock(chain, block)
			execution := runtime.ExecuteProposal(raw)
			require.NotNil(t, execution)

			fatals := 0
			runtime.config.reportFatalError = func(error) { fatals++ }

			err := runtime.InsertProposal(
				&proto.Proposal{RawProposal: raw},
				executionOwnershipSeals(t, validators, block),
				execution,
			)

			require.Equal(t, 1, commits)
			/* START ALPHA CHANGE 2622 -- accept matching retained history after the canonical head advances -- */
			if name == "exact" || name == "advanced" || name == "advanced after confirmation" {
				require.NoError(t, err)
				require.Zero(t, fatals)
				header, found := canonical.GetHeaderByNumber(block.Block.Number())
				require.True(t, found)
				require.Equal(t, block.Block.Hash(), header.Hash)
			} else {
				var failure *proposalInsertionFailure
				require.ErrorAs(t, err, &failure)
				require.ErrorIs(t, err, blockchain.ErrCanonicalBlockNotConfirmed)
				require.Equal(t, "insertion", failure.Phase)
				require.Equal(t, 1, fatals)
			}
			/* FINISH ALPHA CHANGE 2622 -- accept matching retained history after the canonical head advances -- */

			require.Equal(t, canonical.Header().Hash, runtime.lastBuiltBlock.Hash)
			require.Nil(t, runtime.fsm.target)
			chain.AssertNumberOfCalls(t, "ProcessBlock", 1)
			chain.AssertNotCalled(t, "CommitBlock", block)
			chain.AssertExpectations(t)
		})
	}
}

/* FINISH ALPHA CHANGE 2597 -- test canonical head changes after runtime selection checks -- */

/* START ALPHA CHANGE 2623 -- exercise canonical completion before selection checks and after FSM replacement -- */
func TestExecutionConfirmation_RuntimeCoveredCanonicalBlock(t *testing.T) {
	for _, test := range []struct {
		name        string
		advanced    bool
		replaceFSM  bool
		conflicting bool
		unavailable bool
		runtimeBack bool
	}{
		{name: "exact"},
		{name: "exact after FSM replacement", replaceFSM: true},
		{name: "advanced", advanced: true},
		{name: "advanced after FSM replacement", advanced: true, replaceFSM: true},
		{name: "conflicting history", advanced: true, conflicting: true},
		{name: "unavailable history", advanced: true, unavailable: true},
		{name: "runtime still at parent", advanced: true, runtimeBack: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, chain, validators := newExecutionOwnershipRuntime(t)
			block := executionOwnershipBlock(t, runtime, 1)
			raw := block.Block.MarshalRLP()
			originalExtra := append([]byte(nil), block.Block.Header.ExtraData...)

			syncBlock := block
			if test.conflicting {
				syncBlock = executionOwnershipBlock(t, runtime, 2)
			}
			syncHead := syncBlock.Block.Header.Copy()
			if test.advanced {
				syncHead.Number++
				syncHead.ParentHash = syncBlock.Block.Hash()
				syncHead.ComputeHash()
			}

			seed := runtime.fsm.parent
			if test.unavailable {
				/* Seed only the higher head, leaving the selected canonical entry absent. */
				seed = syncHead
			}
			canonical := blockchain.NewTestBlockchain(t, []*types.Header{seed})
			t.Cleanup(func() { require.NoError(t, canonical.Close()) })
			writer := &blockchainWrapper{blockchain: canonical}

			commits, confirmations, advancements := 0, 0, 0
			backend := &executionConfirmationBackend{blockchainBackend: chain}
			backend.commit = func(candidate *types.FullBlock) error {
				commits++

				return writer.CommitBlock(candidate)
			}
			backend.confirm = func(number uint64, hash types.Hash) error {
				confirmations++

				return writer.ConfirmCanonicalBlock(number, hash)
			}
			runtime.config.blockchain = backend
			runtime.fsm.backend = backend

			expectExecutionOwnershipBlock(chain, block)
			execution := runtime.ExecuteProposal(raw)
			require.NotNil(t, execution)

			if !test.unavailable {
				require.NoError(t, canonical.WriteFullBlock(syncBlock, "syncer"))
				if test.advanced {
					require.NoError(t, canonical.WriteFullBlock(
						&types.FullBlock{Block: &types.Block{Header: syncHead}}, "syncer",
					))
				}
			}
			if !test.runtimeBack {
				runtime.lastBuiltBlock = canonical.Header().Copy()
			}
			if test.replaceFSM {
				replacement := *runtime.fsm
				replacement.parent = runtime.lastBuiltBlock.Copy()
				runtime.fsm = &replacement
			}

			require.Equal(t, test.runtimeBack, runtime.IsCurrentExecution(raw, execution))
			beforeRuntime := runtime.lastBuiltBlock.Copy()
			beforeCanonical := canonical.Header().Copy()
			fatals := 0
			runtime.config.reportFatalError = func(error) { fatals++ }

			insert := func() error {
				return runtime.insertExecutedProposal(
					&proto.Proposal{RawProposal: raw},
					executionOwnershipSeals(t, validators, block),
					execution,
					func(candidate *types.FullBlock) error {
						advancements++
						require.True(t, test.runtimeBack)
						require.Equal(t, block.Block.Hash(), candidate.Block.Hash())
						runtime.lastBuiltBlock = candidate.Block.Header.Copy()

						return nil
					},
				)
			}

			err := insert()
			if test.conflicting || test.unavailable {
				var failure *proposalInsertionFailure
				require.ErrorAs(t, err, &failure)
				require.ErrorIs(t, err, blockchain.ErrCanonicalBlockNotConfirmed)
				require.Equal(t, "insertion", failure.Phase)
				require.Equal(t, 1, fatals)
				require.Equal(t, beforeRuntime.Hash, runtime.lastBuiltBlock.Hash)
			} else {
				require.NoError(t, err)
				require.NoError(t, insert())
				require.Zero(t, fatals)
				if !test.runtimeBack {
					require.Equal(t, beforeRuntime.Hash, runtime.lastBuiltBlock.Hash)
				}
			}

			if test.runtimeBack {
				require.Equal(t, 1, commits)
				require.Equal(t, 1, advancements)
				require.Equal(t, 1, confirmations)
			} else {
				require.Zero(t, commits)
				require.Zero(t, advancements)
				if test.conflicting || test.unavailable {
					require.Equal(t, 1, confirmations)
				} else {
					require.Equal(t, 2, confirmations)
				}
			}
			require.Equal(t, beforeCanonical.Hash, canonical.Header().Hash)
			require.Equal(t, originalExtra, block.Block.Header.ExtraData)
			require.Nil(t, runtime.fsm.target)
			chain.AssertNumberOfCalls(t, "ProcessBlock", 1)
			chain.AssertNotCalled(t, "CommitBlock", mock.Anything)
			chain.AssertExpectations(t)
		})
	}
}

/* FINISH ALPHA CHANGE 2623 -- exercise canonical completion before selection checks and after FSM replacement -- */
