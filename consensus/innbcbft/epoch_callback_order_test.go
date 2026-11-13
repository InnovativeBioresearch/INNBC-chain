/* START ALPHA CHANGE 3888 -- test epoch recovery across reordered block callbacks -- */
package innbcbft

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const epochCallbackSize = uint64(30)

type epochCallbackFixture struct {
	runtime            *consensusRuntime
	headers            map[uint64]*types.Header
	chain              *blockchainMock
	backend            *innbcbftBackendMock
	system             *systemStateMock
	pool               *txPoolMock
	expectedValidators validator.AccountSet
}

func newEpochCallbackFixture(
	t *testing.T,
	initialHead uint64,
	changeCommittee bool,
) *epochCallbackFixture {
	t.Helper()

	identities := validator.NewTestValidators(t, 5)
	currentValidators := identities.GetPublicIdentities("0", "1", "2", "3")
	nextValidators := currentValidators

	var delta *validator.ValidatorSetDelta
	if changeCommittee {
		nextValidators = identities.GetPublicIdentities("0", "1", "2", "4")

		var err error
		delta, err = validator.CreateValidatorSetDelta(
			currentValidators,
			nextValidators,
		)
		require.NoError(t, err)
	}

	headers := make(map[uint64]*types.Header)
	for number := uint64(0); number <= 62; number++ {
		epochNumber := uint64(0)
		if number > 0 {
			epochNumber = (number-1)/epochCallbackSize + 1
		}

		extra := &Extra{
			Checkpoint: &CheckpointData{
				EpochNumber: epochNumber,
			},
		}
		if changeCommittee && number == epochCallbackSize {
			extra.Validators = delta
		}

		header := &types.Header{
			Number:    number,
			Timestamp: 2 * number,
			GasLimit:  types.StateTransactionGasLimit,
			BaseFee:   1,
			ExtraData: extra.MarshalRLPTo(nil),
		}
		if number > 0 {
			header.ParentHash = headers[number-1].Hash
		}
		header.ComputeHash()
		headers[number] = header
	}

	chain := new(blockchainMock)
	chain.On("GetHeaderByNumber", mock.Anything).
		Return(func(number uint64) *types.Header {
			return headers[number]
		}).Maybe()
	chain.On("GetStateProviderForBlock", mock.Anything).
		Return(new(stateProviderMock)).Maybe()

	system := new(systemStateMock)
	chain.On("GetSystemState", mock.Anything).Return(system).Maybe()

	backend := new(innbcbftBackendMock)
	backend.On(
		"GetValidatorsWithTx",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nextValidators).Maybe()

	pool := new(txPoolMock)
	pool.On("ResetWithHeaders", mock.Anything).Maybe()

	state := newTestState(t)
	initialSnapshot := NewProposerSnapshot(initialHead+1, currentValidators)
	require.NoError(t,
		state.ProposerSnapshotStore.writeProposerSnapshot(initialSnapshot, nil))
	require.NoError(t, state.EpochStore.insertEpoch(1, nil))

	config := &runtimeConfig{
		INNBCBFTConfig: &INNBCBFTConfig{
			EpochSize: epochCallbackSize,
		},
		Key:             wallet.NewKey(identities.GetPrivateIdentities("0")[0]),
		State:           state,
		blockchain:      chain,
		innbcbftBackend: backend,
		txPool:          pool,
	}

	runtime := &consensusRuntime{
		config:         config,
		state:          state,
		logger:         hclog.NewNullLogger(),
		lastBuiltBlock: headers[initialHead],
		epoch: &epochMetadata{
			Number:            1,
			FirstBlockInEpoch: 1,
			Validators:        currentValidators,
		},
		proposerCalculator: NewProposerCalculatorFromSnapshot(
			initialSnapshot,
			config,
			hclog.NewNullLogger(),
		),
		stakeManager: &dummyStakeManager{},
	}

	t.Cleanup(func() {
		chain.AssertExpectations(t)
		backend.AssertExpectations(t)
		system.AssertExpectations(t)
		pool.AssertExpectations(t)
	})

	return &epochCallbackFixture{
		runtime:            runtime,
		headers:            headers,
		chain:              chain,
		backend:            backend,
		system:             system,
		pool:               pool,
		expectedValidators: nextValidators,
	}
}

func TestEpochCallback_OrderingAndCatchUp(t *testing.T) {
	tests := []struct {
		name            string
		initialHead     uint64
		callbacks       []uint64
		epoch           uint64
		firstBlock      uint64
		restartCalls    int
		changeCommittee bool
		checkFSM        bool
	}{
		{
			name:         "ordered_boundary",
			initialHead:  29,
			callbacks:    []uint64{30, 31},
			epoch:        2,
			firstBlock:   31,
			restartCalls: 1,
		},
		{
			name:         "successor_before_boundary",
			initialHead:  29,
			callbacks:    []uint64{31, 30, 32},
			epoch:        2,
			firstBlock:   31,
			restartCalls: 1,
			checkFSM:     true,
		},
		{
			name:        "gap_inside_epoch",
			initialHead: 25,
			callbacks:   []uint64{29, 26},
			epoch:       1,
			firstBlock:  1,
		},
		{
			name:         "multiple_epoch_gap",
			initialHead:  29,
			callbacks:    []uint64{61, 30, 60, 62},
			epoch:        3,
			firstBlock:   61,
			restartCalls: 1,
			checkFSM:     true,
		},
		{
			name:            "committee_change",
			initialHead:     29,
			callbacks:       []uint64{31, 30, 32},
			epoch:           2,
			firstBlock:      31,
			restartCalls:    1,
			changeCommittee: true,
			checkFSM:        true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newEpochCallbackFixture(
				t,
				test.initialHead,
				test.changeCommittee,
			)
			if test.restartCalls > 0 {
				f.system.On("GetEpoch").
					Return(test.epoch).Times(test.restartCalls)
			}

			latest := test.initialHead
			advancements := 0

			for _, number := range test.callbacks {
				advanced, err := f.runtime.OnBlockInserted(&types.FullBlock{
					Block: &types.Block{
						Header: f.headers[number],
					},
				})
				require.NoError(t, err)
				require.Equal(t, number > latest, advanced)

				if number > latest {
					latest = number
					advancements++
				}
				require.Same(t, f.headers[latest], f.runtime.lastBuiltBlock)
			}

			require.Equal(t, test.epoch, f.runtime.epoch.Number)
			require.Equal(t, test.firstBlock, f.runtime.epoch.FirstBlockInEpoch)
			require.Equal(t,
				f.expectedValidators.GetAddresses(),
				f.runtime.epoch.Validators.GetAddresses(),
			)
			require.True(t, f.runtime.IsAlignedWith(f.headers[latest]))

			snapshot, ok := f.runtime.proposerCalculator.GetSnapshot()
			require.True(t, ok)
			require.Equal(t, latest+1, snapshot.Height)

			addresses := make([]types.Address, len(snapshot.Validators))
			for i, entry := range snapshot.Validators {
				addresses[i] = entry.Metadata.Address
			}
			require.Equal(t, f.expectedValidators.GetAddresses(), addresses)

			persisted, err := f.runtime.state.ProposerSnapshotStore.
				getProposerSnapshot(nil)
			require.NoError(t, err)
			require.NotNil(t, persisted)
			require.Equal(t, latest+1, persisted.Height)

			require.True(t,
				f.runtime.state.EpochStore.isEpochInserted(test.epoch))
			if test.epoch != 1 {
				require.False(t,
					f.runtime.state.EpochStore.isEpochInserted(1))
			}

			validatorReads := test.restartCalls
			if test.changeCommittee {
				// Proposer replay also reads the changed set at block 30.
				validatorReads++
			}
			f.chain.AssertNumberOfCalls(
				t, "GetStateProviderForBlock", test.restartCalls)
			f.system.AssertNumberOfCalls(t, "GetEpoch", test.restartCalls)
			f.backend.AssertNumberOfCalls(
				t, "GetValidatorsWithTx", validatorReads)
			f.pool.AssertNumberOfCalls(
				t, "ResetWithHeaders", advancements)

			if test.checkFSM {
				f.chain.On("NewBlockBuilder").
					Return(&BlockBuilder{}, nil).Once()

				height, err := f.runtime.PrepareFSM(f.headers[latest])
				require.NoError(t, err)
				require.Equal(t, latest+1, height)
				require.Equal(t, test.epoch, f.runtime.fsm.epochNumber)
				require.False(t, f.runtime.fsm.isEndOfEpoch)
				require.Equal(t,
					latest+1,
					f.runtime.fsm.proposerSnapshot.Height,
				)
			}
		})
	}
}

func TestEpochCallback_EpochReadFailureRollsBackAndRetries(t *testing.T) {
	f := newEpochCallbackFixture(t, 29, false)
	originalEpoch := f.runtime.epoch

	before, ok := f.runtime.proposerCalculator.GetSnapshot()
	require.True(t, ok)

	cause := errors.New("forced epoch read failure")
	f.system.On("GetEpoch").Return(uint64(0), cause).Once()

	block := &types.FullBlock{
		Block: &types.Block{
			Header: f.headers[31],
		},
	}
	advanced, err := f.runtime.OnBlockInserted(block)
	require.ErrorIs(t, err, cause)
	require.False(t, advanced)

	require.Same(t, f.headers[29], f.runtime.lastBuiltBlock)
	require.Same(t, originalEpoch, f.runtime.epoch)

	after, ok := f.runtime.proposerCalculator.GetSnapshot()
	require.True(t, ok)
	require.Equal(t, before, after)

	persisted, err := f.runtime.state.ProposerSnapshotStore.
		getProposerSnapshot(nil)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Equal(t, uint64(30), persisted.Height)
	require.True(t, f.runtime.state.EpochStore.isEpochInserted(1))
	require.False(t, f.runtime.state.EpochStore.isEpochInserted(2))

	// Retry directly to prove the failed transaction left recoverable state.
	// Production fatal-error handling remains unchanged.
	f.system.On("GetEpoch").Return(uint64(2)).Once()

	advanced, err = f.runtime.OnBlockInserted(block)
	require.NoError(t, err)
	require.True(t, advanced)
	require.Same(t, f.headers[31], f.runtime.lastBuiltBlock)
	require.Equal(t, uint64(2), f.runtime.epoch.Number)
	require.Equal(t, uint64(31), f.runtime.epoch.FirstBlockInEpoch)

	after, ok = f.runtime.proposerCalculator.GetSnapshot()
	require.True(t, ok)
	require.Equal(t, uint64(32), after.Height)

	persisted, err = f.runtime.state.ProposerSnapshotStore.
		getProposerSnapshot(nil)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Equal(t, uint64(32), persisted.Height)
	require.True(t, f.runtime.state.EpochStore.isEpochInserted(2))
	require.False(t, f.runtime.state.EpochStore.isEpochInserted(1))
}

/* FINISH ALPHA CHANGE 3888 -- test epoch recovery across reordered block callbacks -- */
