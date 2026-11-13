package innbcbft

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/bitmap"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2579 -- test missing checkpoints during epoch-ending detection -- */
func TestHelpers_isEpochEndingBlock_MissingCurrentCheckpoint(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		extra *Extra
	}{
		{"missing extra", nil},
		{"missing checkpoint with nil delta", &Extra{}},
		{"missing checkpoint with empty delta", &Extra{Validators: &validator.ValidatorSetDelta{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := new(blockchainMock)

			ending, err := isEpochEndingBlock(20, tc.extra, chain)
			require.ErrorContains(t, err, "checkpoint data for block 20 is missing")
			require.False(t, ending)

			chain.AssertNotCalled(t, "GetHeaderByNumber", mock.Anything)
		})
	}
}

func TestHelpers_isEpochEndingBlock_MissingNextCheckpoint(t *testing.T) {
	t.Parallel()

	extra := &Extra{
		Checkpoint: &CheckpointData{EpochNumber: 2},
		Validators: &validator.ValidatorSetDelta{},
	}
	next := &types.Header{
		Number:    21,
		ExtraData: (&Extra{}).MarshalRLPTo(nil),
	}

	chain := new(blockchainMock)
	chain.On("GetHeaderByNumber", uint64(21)).Return(next, true).Once()

	ending, err := isEpochEndingBlock(20, extra, chain)
	require.ErrorContains(t, err, "checkpoint data for block 21 is missing")
	require.False(t, ending)
	chain.AssertExpectations(t)
}

/* FINISH ALPHA CHANGE 2579 -- test missing checkpoints during epoch-ending detection -- */

func TestHelpers_isEpochEndingBlock_DeltaNotEmpty(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidators(t, 3).GetPublicIdentities()
	bitmap := bitmap.Bitmap{}
	bitmap.Set(0)

	delta := &validator.ValidatorSetDelta{
		Added:   validators[1:],
		Removed: bitmap,
	}

	extra := &Extra{Validators: delta, Checkpoint: &CheckpointData{EpochNumber: 2}}
	blockNumber := uint64(20)

	isEndOfEpoch, err := isEpochEndingBlock(blockNumber, extra, new(blockchainMock))
	require.NoError(t, err)
	require.True(t, isEndOfEpoch)
}

func TestHelpers_isEpochEndingBlock_NoBlock(t *testing.T) {
	t.Parallel()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(&types.Header{}, false)

	extra := &Extra{Checkpoint: &CheckpointData{EpochNumber: 2}, Validators: &validator.ValidatorSetDelta{}}
	blockNumber := uint64(20)

	isEndOfEpoch, err := isEpochEndingBlock(blockNumber, extra, blockchainMock)
	require.ErrorIs(t, blockchain.ErrNoBlock, err)
	require.False(t, isEndOfEpoch)
}

func TestHelpers_isEpochEndingBlock_EpochsNotTheSame(t *testing.T) {
	t.Parallel()

	blockchainMock := new(blockchainMock)

	nextBlockExtra := &Extra{Checkpoint: &CheckpointData{EpochNumber: 3}, Validators: &validator.ValidatorSetDelta{}}
	nextBlock := &types.Header{
		Number:    21,
		ExtraData: nextBlockExtra.MarshalRLPTo(nil),
	}

	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(nextBlock, true)

	extra := &Extra{Checkpoint: &CheckpointData{EpochNumber: 2}, Validators: &validator.ValidatorSetDelta{}}
	blockNumber := uint64(20)

	isEndOfEpoch, err := isEpochEndingBlock(blockNumber, extra, blockchainMock)
	require.NoError(t, err)
	require.True(t, isEndOfEpoch)
}

func TestHelpers_isEpochEndingBlock_EpochsAreTheSame(t *testing.T) {
	t.Parallel()

	blockchainMock := new(blockchainMock)

	nextBlockExtra := &Extra{Checkpoint: &CheckpointData{EpochNumber: 2}, Validators: &validator.ValidatorSetDelta{}}
	nextBlock := &types.Header{
		Number:    16,
		ExtraData: nextBlockExtra.MarshalRLPTo(nil),
	}

	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(nextBlock, true)

	extra := &Extra{Checkpoint: &CheckpointData{EpochNumber: 2}, Validators: &validator.ValidatorSetDelta{}}
	blockNumber := uint64(15)

	isEndOfEpoch, err := isEpochEndingBlock(blockNumber, extra, blockchainMock)
	require.NoError(t, err)
	require.False(t, isEndOfEpoch)
}
