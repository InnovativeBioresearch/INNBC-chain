/* START ALPHA CHANGE 2589 -- test execution isolation, insertion ownership, stale results, and failure phases -- */
package innbcbft

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	core "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftcore"
	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

/*
CommitBlock and runtime advancement are controlled independently.
These tests verify placement and ownership, not disk durability or
complete network consensus.
*/
func newExecutionOwnershipRuntime(
	t *testing.T,
) (*consensusRuntime, *blockchainMock, *validator.TestValidators) {
	t.Helper()

	validators := validator.NewTestValidators(t, 3)
	accounts := validators.GetPublicIdentities()

	parent := &types.Header{
		ExtraData: BuildGenesisExtraData(accounts),
	}
	parent.ComputeHash()

	chain := new(blockchainMock)
	logger := hclog.NewNullLogger()

	config := &runtimeConfig{
		blockchain:       chain,
		reportFatalError: func(error) {},
	}

	runtime := &consensusRuntime{
		config:         config,
		logger:         logger,
		lastBuiltBlock: parent,
		epoch: &epochMetadata{
			Number:     1,
			Validators: accounts,
		},
		proposerCalculator: NewProposerCalculatorFromSnapshot(
			NewProposerSnapshot(1, accounts),
			config,
			logger,
		),
		fsm: &fsm{
			parent:      parent,
			backend:     chain,
			validators:  validators.ToValidatorSet(),
			epochNumber: 1,
			config:      testChainClockConfig(),
			logger:      logger,
		},
	}

	return runtime, chain, validators
}

func executionOwnershipBlock(
	t *testing.T,
	runtime *consensusRuntime,
	marker byte,
) *types.FullBlock {
	t.Helper()

	hash, err := runtime.fsm.validators.Accounts().Hash()
	require.NoError(t, err)

	extra := &Extra{
		Checkpoint: &CheckpointData{
			EpochNumber:           1,
			CurrentValidatorsHash: hash,
			NextValidatorsHash:    hash,
		},
	}

	/* START ALPHA CHANGE 3320 -- preserve execution-ownership coverage with valid timestamps -- */
	block := createDummyStateBlock(
		runtime.fsm.parent.Number+1,
		runtime.fsm.parent.Hash,
		extra.MarshalRLPTo(nil),
		runtime.fsm.parent.Timestamp,
	)
	/* FINISH ALPHA CHANGE 3320 -- preserve execution-ownership coverage with valid timestamps -- */
	block.Block.Header.Miner = []byte{marker}
	block.Block.Header.ComputeHash()

	return block
}

func executionOwnershipSeals(
	t *testing.T,
	validators *validator.TestValidators,
	block *types.FullBlock,
) []*messages.CommittedSeal {
	t.Helper()

	extra, err := GetIbftExtra(block.Block.Header.ExtraData)
	require.NoError(t, err)

	hash, err := extra.Checkpoint.Hash(0, block.Block.Number(), block.Block.Hash())
	require.NoError(t, err)

	account := validators.GetPrivateIdentities("0")[0]
	signature, err := account.Bls.Sign(hash.Bytes(), signer.DomainCheckpointManager)
	require.NoError(t, err)

	rawSignature, err := signature.Marshal()
	require.NoError(t, err)

	return []*messages.CommittedSeal{
		{
			Signer:    validators.GetPublicIdentities("0")[0].Address.Bytes(),
			Signature: rawSignature,
		},
	}
}

func expectExecutionOwnershipBlock(
	chain *blockchainMock,
	block *types.FullBlock,
) *mock.Call {
	expectedHash := block.Block.Hash()

	return chain.On(
		"ProcessBlock",
		mock.Anything,
		mock.MatchedBy(func(candidate *types.Block) bool {
			return candidate != nil && candidate.Hash() == expectedHash
		}),
	).Return(block, nil).Once()
}

func TestExecutionOwnership_RuntimeBThenAAndInsertionOverlap(t *testing.T) {
	runtime, chain, validators := newExecutionOwnershipRuntime(t)

	blockB := executionOwnershipBlock(t, runtime, 2)
	rawB := blockB.Block.MarshalRLP()
	expectExecutionOwnershipBlock(chain, blockB)

	resultB := runtime.ExecuteProposal(rawB)
	require.NotNil(t, resultB)
	require.Nil(t, runtime.fsm.target)

	blockA := executionOwnershipBlock(t, runtime, 1)
	rawA := blockA.Block.MarshalRLP()

	started := make(chan struct{})
	release := make(chan struct{})

	var releaseOnce sync.Once
	unblock := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer unblock()

	expectExecutionOwnershipBlock(chain, blockA).Run(func(mock.Arguments) {
		close(started)
		<-release
	})

	done := make(chan *core.ProposalExecution, 1)
	go func() {
		done <- runtime.ExecuteProposal(rawA)
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("proposal A never reached execution")
	}

	originalExtra := append([]byte(nil), blockB.Block.Header.ExtraData...)

	chain.On("CommitBlock", mock.MatchedBy(func(block *types.FullBlock) bool {
		if block == nil || block.Block == nil || block.Block.Hash() != blockB.Block.Hash() {
			return false
		}

		extra, err := GetIbftExtra(block.Block.Header.ExtraData)

		return err == nil && extra.Committed != nil
	})).Return(nil).Once()

	advancements := 0
	require.NoError(t, runtime.insertExecutedProposal(
		&proto.Proposal{RawProposal: rawB},
		executionOwnershipSeals(t, validators, blockB),
		resultB,
		func(block *types.FullBlock) error {
			advancements++
			require.Equal(t, blockB.Block.Hash(), block.Block.Hash())

			return nil
		},
	))

	require.Equal(t, 1, advancements)
	require.Equal(t, originalExtra, blockB.Block.Header.ExtraData)
	require.Nil(t, runtime.fsm.target)

	unblock()

	select {
	case resultA := <-done:
		require.NotNil(t, resultA)
		require.NotSame(t, resultB, resultA)
	case <-time.After(5 * time.Second):
		t.Fatal("proposal A did not finish")
	}

	require.Nil(t, runtime.fsm.target)
	require.True(t, runtime.IsCurrentExecution(rawB, resultB))
	chain.AssertExpectations(t)
}

func TestExecutionOwnership_RuntimeLocalConstruction(t *testing.T) {
	runtime, chain, _ := newExecutionOwnershipRuntime(t)

	block := executionOwnershipBlock(t, runtime, 1)
	builder := newBlockBuilderMock(block)
	runtime.fsm.blockBuilder = builder

	raw, execution := runtime.BuildExecutedProposal(&proto.View{
		Height: 1,
		Round:  0,
	})

	require.NotEmpty(t, raw)
	require.NotNil(t, execution)
	require.True(t, runtime.IsCurrentExecution(raw, execution))
	require.Nil(t, runtime.fsm.target)

	chain.AssertNotCalled(t, "ProcessBlock", mock.Anything, mock.Anything)
	builder.AssertExpectations(t)
}

func TestExecutionOwnership_RuntimeRejectsStaleResults(t *testing.T) {
	for _, scenario := range []string{
		"foreign FSM",
		"runtime parent changed",
		"execution parent changed",
		"execution height changed",
		"raw proposal changed",
	} {
		t.Run(scenario, func(t *testing.T) {
			runtime, chain, _ := newExecutionOwnershipRuntime(t)
			block := executionOwnershipBlock(t, runtime, 1)
			raw := block.Block.MarshalRLP()
			expectExecutionOwnershipBlock(chain, block)

			execution := runtime.ExecuteProposal(raw)
			require.NotNil(t, execution)

			payload := executionPayload(execution)
			require.NotNil(t, payload)

			switch scenario {
			case "foreign FSM":
				replacement := *runtime.fsm
				runtime.fsm = &replacement
			case "runtime parent changed":
				runtime.lastBuiltBlock = runtime.lastBuiltBlock.Copy()
				runtime.lastBuiltBlock.Hash = types.Hash{99}
			case "execution parent changed":
				payload.machine.parent.Hash = types.Hash{99}
			case "execution height changed":
				payload.machine.target.Block.Header.Number++
			case "raw proposal changed":
				raw = append(append([]byte(nil), raw...), 0)
			}

			require.False(t, runtime.IsCurrentExecution(raw, execution))

			err := runtime.insertExecutedProposal(
				&proto.Proposal{RawProposal: raw},
				nil,
				execution,
				func(*types.FullBlock) error {
					t.Fatal("stale result reached runtime advancement")

					return nil
				},
			)

			var failure *proposalInsertionFailure
			require.ErrorAs(t, err, &failure)
			require.Equal(t, "selection", failure.Phase)

			chain.AssertNotCalled(t, "CommitBlock", mock.Anything)
			chain.AssertExpectations(t)
		})
	}
}

func TestExecutionOwnership_RuntimeInsertionFailurePhases(t *testing.T) {
	for _, phase := range []string{"insertion", "advancement"} {
		t.Run(phase, func(t *testing.T) {
			runtime, chain, validators := newExecutionOwnershipRuntime(t)
			block := executionOwnershipBlock(t, runtime, 1)
			raw := block.Block.MarshalRLP()
			expectExecutionOwnershipBlock(chain, block)

			execution := runtime.ExecuteProposal(raw)
			require.NotNil(t, execution)

			originalExtra := append([]byte(nil), block.Block.Header.ExtraData...)
			cause := errors.New("injected failure")

			var commitErr error
			if phase == "insertion" {
				commitErr = cause
			}
			chain.On("CommitBlock", mock.Anything).Return(commitErr).Once()

			fatals := 0
			runtime.config.reportFatalError = func(err error) {
				fatals++
				require.ErrorIs(t, err, cause)
			}

			advancements := 0
			err := runtime.insertExecutedProposal(
				&proto.Proposal{RawProposal: raw},
				executionOwnershipSeals(t, validators, block),
				execution,
				func(*types.FullBlock) error {
					advancements++

					return cause
				},
			)

			var failure *proposalInsertionFailure
			require.ErrorAs(t, err, &failure)
			require.ErrorIs(t, err, cause)
			require.Equal(t, phase, failure.Phase)
			require.Equal(t, 1, fatals)
			require.Equal(t, originalExtra, block.Block.Header.ExtraData)
			require.Nil(t, runtime.fsm.target)

			if phase == "insertion" {
				require.Zero(t, advancements)
			} else {
				require.Equal(t, 1, advancements)
			}

			chain.AssertExpectations(t)
		})
	}
}

func TestExecutionOwnership_RuntimeAlreadyAppliedExactHead(t *testing.T) {
	runtime, chain, _ := newExecutionOwnershipRuntime(t)

	block := executionOwnershipBlock(t, runtime, 1)
	raw := block.Block.MarshalRLP()
	expectExecutionOwnershipBlock(chain, block)

	execution := runtime.ExecuteProposal(raw)
	require.NotNil(t, execution)

	runtime.lastBuiltBlock = block.Block.Header.Copy()

	/* START ALPHA CHANGE 2619 -- require canonical confirmation even for an already applied runtime head -- */
	chain.On("ConfirmCanonicalBlock", block.Block.Number(), block.Block.Hash()).Return(nil).Once()
	/* FINISH ALPHA CHANGE 2619 -- require canonical confirmation even for an already applied runtime head -- */

	require.NoError(t, runtime.insertExecutedProposal(
		&proto.Proposal{RawProposal: raw},
		nil,
		execution,
		func(*types.FullBlock) error {
			t.Fatal("already applied block reached runtime advancement")

			return nil
		},
	))

	chain.AssertNotCalled(t, "CommitBlock", mock.Anything)
	chain.AssertExpectations(t)
}

/* FINISH ALPHA CHANGE 2589 -- test execution isolation, insertion ownership, stale results, and failure phases -- */

/* START ALPHA CHANGE 2620 -- reject another runtime's result even after canonical height is covered -- */
func TestExecutionOwnership_RuntimeRejectsForeignRuntime(t *testing.T) {
	runtime, chain, _ := newExecutionOwnershipRuntime(t)
	block := executionOwnershipBlock(t, runtime, 1)
	raw := block.Block.MarshalRLP()
	expectExecutionOwnershipBlock(chain, block)

	execution := runtime.ExecuteProposal(raw)
	require.NotNil(t, execution)

	foreign := &consensusRuntime{
		config:         runtime.config,
		logger:         runtime.logger,
		fsm:            runtime.fsm,
		lastBuiltBlock: block.Block.Header.Copy(),
	}
	require.False(t, foreign.IsCurrentExecution(raw, execution))

	err := foreign.insertExecutedProposal(
		&proto.Proposal{RawProposal: raw},
		nil,
		execution,
		func(*types.FullBlock) error {
			t.Fatal("foreign execution reached runtime advancement")

			return nil
		},
	)

	var failure *proposalInsertionFailure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "selection", failure.Phase)
	chain.AssertNotCalled(t, "ConfirmCanonicalBlock", mock.Anything, mock.Anything)
	chain.AssertNotCalled(t, "CommitBlock", mock.Anything)
	chain.AssertExpectations(t)
}

/* FINISH ALPHA CHANGE 2620 -- reject another runtime's result even after canonical height is covered -- */
