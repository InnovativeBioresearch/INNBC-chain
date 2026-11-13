/* START ALPHA CHANGE 2297 -- test limited finalized validation and preserved live epoch requirements -- */
package innbcbft

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/bitmap"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIssue12_FinalizedCheckpointConsistency(t *testing.T) {
	const chainID uint64 = 100

	identities := validator.NewTestValidators(t, 6)
	current := identities.GetPublicIdentities()
	currentHash, err := current.Hash()
	require.NoError(t, err)

	removed := bitmap.Bitmap{}
	removed.Set(uint64(len(current) - 1))
	changedDelta := &validator.ValidatorSetDelta{Removed: removed}

	tests := []struct {
		name   string
		delta  *validator.ValidatorSetDelta
		mutate func(*CheckpointData)
		want   string
	}{
		{
			name: "unchanged set with nil delta",
		},
		{
			name:  "unchanged set with empty delta",
			delta: &validator.ValidatorSetDelta{},
		},
		{
			name:  "consistent declared set change",
			delta: changedDelta,
		},
		{
			name: "incorrect current validator hash",
			mutate: func(checkpoint *CheckpointData) {
				checkpoint.CurrentValidatorsHash = types.BytesToHash([]byte{1})
			},
			want: "current validators hashes don't match",
		},
		{
			name: "incorrect next validator hash",
			mutate: func(checkpoint *CheckpointData) {
				checkpoint.NextValidatorsHash = types.BytesToHash([]byte{2})
			},
			want: "next validators hashes don't match",
		},
		{
			name:  "declared change with old next validator hash",
			delta: changedDelta,
			mutate: func(checkpoint *CheckpointData) {
				checkpoint.NextValidatorsHash = currentHash
			},
			want: "next validators hashes don't match",
		},
		{
			name: "nonzero event root",
			mutate: func(checkpoint *CheckpointData) {
				checkpoint.EventRoot = types.BytesToHash([]byte{3})
			},
			want: "exit root hash not as expected",
		},
		{
			name:  "set change with inconsistent epoch increment",
			delta: changedDelta,
			mutate: func(checkpoint *CheckpointData) {
				checkpoint.EpochNumber = 2
			},
			want: "epoch number should not change for epoch-ending block",
		},
		{
			name: "delta adds an existing validator",
			delta: &validator.ValidatorSetDelta{
				Added: validator.AccountSet{current[0]},
			},
			want: "failed to apply declared validator delta",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parentCheckpoint := &CheckpointData{
				EpochNumber:           1,
				CurrentValidatorsHash: currentHash,
				NextValidatorsHash:    currentHash,
			}
			parent := &types.Header{
				Number: 9,
				Hash:   types.BytesToHash([]byte{9}),
				ExtraData: (&Extra{
					Checkpoint: parentCheckpoint,
				}).MarshalRLPTo(nil),
			}

			parentHash, err := parentCheckpoint.Hash(chainID, parent.Number, parent.Hash)
			require.NoError(t, err)

			header := &types.Header{
				Number:     10,
				Hash:       types.BytesToHash([]byte{10}),
				ParentHash: parent.Hash,
			}
			checkpoint := &CheckpointData{
				EpochNumber:           1,
				CurrentValidatorsHash: currentHash,
				NextValidatorsHash:    currentHash,
			}

			next, deltaErr := current.ApplyDelta(test.delta)
			if deltaErr == nil {
				checkpoint.NextValidatorsHash, err = next.Hash()
				require.NoError(t, err)
			}

			if test.mutate != nil {
				test.mutate(checkpoint)
			}

			// Sign the actual checkpoint being tested, including incorrect
			// values. Rejection must therefore come from consistency checks,
			// not from a signature made invalid by later mutation.
			checkpointHash, err := checkpoint.Hash(chainID, header.Number, header.Hash)
			require.NoError(t, err)

			extra := &Extra{
				Validators: test.delta,
				Checkpoint: checkpoint,
				Parent: createSignature(
					t, identities.GetPrivateIdentities(),
					parentHash, signer.DomainCheckpointManager,
				),
				Committed: createSignature(
					t, identities.GetPrivateIdentities(),
					checkpointHash, signer.DomainCheckpointManager,
				),
			}

			backend := new(innbcbftBackendMock)
			backend.On("GetValidators", uint64(9), mock.Anything).
				Return(current).Once()
			backend.On("GetValidators", uint64(8), mock.Anything).
				Return(current).Once()

			err = extra.ValidateFinalizedData(
				header, parent, nil, chainID, backend,
				signer.DomainCheckpointManager, hclog.NewNullLogger(),
			)

			if test.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.want)
			}

			// Only the existing current/parent certificate lookups occur.
			backend.AssertNumberOfCalls(t, "GetValidators", 2)
			backend.AssertExpectations(t)
		})
	}
}

func TestIssue12_FinalizedValidatorReadFailureIsNotSkipped(t *testing.T) {
	readErr := errors.New("validator snapshot unavailable")
	backend := new(innbcbftBackendMock)
	backend.On("GetValidators", uint64(9), mock.Anything).
		Return(nil, readErr).Once()

	extra := &Extra{
		Checkpoint: &CheckpointData{EpochNumber: 1},
		Committed:  &Signature{},
	}

	err := extra.ValidateFinalizedData(
		&types.Header{Number: 10},
		&types.Header{Number: 9},
		nil,
		100,
		backend,
		signer.DomainCheckpointManager,
		hclog.NewNullLogger(),
	)

	require.ErrorIs(t, err, readErr)
	backend.AssertExpectations(t)
}

func TestIssue12_PreCommitStateTransactionStructure(t *testing.T) {
	input, err := createTestCommitEpochInput(t, 1, 10).EncodeAbi()
	require.NoError(t, err)

	commitTx := createStateTransactionWithData(
		10, contracts.ValidatorSetContract, input,
	)
	unknownTx := createStateTransactionWithData(
		10, contracts.ValidatorSetContract, []byte{0xff},
	)
	ordinaryTx := &types.Transaction{
		Type:  types.LegacyTx,
		Input: []byte{0xff},
	}

	tests := []struct {
		name string
		txs  []*types.Transaction
		want string
	}{
		{
			name: "no state transaction",
		},
		{
			name: "ordinary transaction input is not restricted",
			txs:  []*types.Transaction{ordinaryTx},
		},
		{
			name: "one known state transaction",
			txs:  []*types.Transaction{ordinaryTx, commitTx},
		},
		{
			name: "unknown state input",
			txs:  []*types.Transaction{unknownTx},
			want: "unknown state transaction",
		},
		{
			name: "duplicate epoch transaction",
			txs:  []*types.Transaction{commitTx, commitTx.Copy()},
			want: errCommitEpochTxSingleExpected.Error(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// An uninitialized backend and nil transition prove this hook
			// requires no runtime, historical lookup, or state execution.
			backend := new(INNBCBFT)
			err := backend.PreCommitState(
				&types.Block{
					Header:       &types.Header{Number: 10},
					Transactions: test.txs,
				},
				nil,
			)

			if test.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.want)
			}
		})
	}
}

func TestIssue12_LiveEpochRequirementsRemainEnforced(t *testing.T) {
	live := &fsm{
		parent:           &types.Header{Number: 9},
		isEndOfEpoch:     true,
		commitEpochInput: createTestCommitEpochInput(t, 1, 10),
	}

	commitTx, err := live.createCommitEpochTx()
	require.NoError(t, err)

	require.NoError(t, live.VerifyStateTransactions([]*types.Transaction{commitTx}))
	require.ErrorIs(t, live.VerifyStateTransactions(nil), errCommitEpochTxDoesNotExist)
	require.ErrorIs(
		t,
		live.VerifyStateTransactions([]*types.Transaction{commitTx, commitTx.Copy()}),
		errCommitEpochTxSingleExpected,
	)

	wrongInput, err := createTestCommitEpochInput(t, 2, 10).EncodeAbi()
	require.NoError(t, err)
	wrongTx := createStateTransactionWithData(
		10, contracts.ValidatorSetContract, wrongInput,
	)
	require.ErrorContains(
		t,
		live.VerifyStateTransactions([]*types.Transaction{wrongTx}),
		"invalid commit epoch transaction",
	)

	live.isEndOfEpoch = false
	require.NoError(t, live.VerifyStateTransactions(nil))
	require.ErrorIs(
		t,
		live.VerifyStateTransactions([]*types.Transaction{commitTx}),
		errCommitEpochTxNotExpected,
	)
}

/* FINISH ALPHA CHANGE 2297 -- test limited finalized validation and preserved live epoch requirements -- */
