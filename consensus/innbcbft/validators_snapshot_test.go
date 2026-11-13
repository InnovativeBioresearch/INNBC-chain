package innbcbft

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestValidatorsSnapshotCache_GetSnapshot_Build(t *testing.T) {
	t.Parallel()
	assertions := require.New(t)

	const (
		totalValidators  = 10
		validatorSetSize = 5
		epochSize        = uint64(10)
	)

	allValidators := validator.NewTestValidators(t, totalValidators).GetPublicIdentities()

	var oddValidators, evenValidators validator.AccountSet

	for i := 0; i < totalValidators; i++ {
		if i%2 == 0 {
			evenValidators = append(evenValidators, allValidators[i])
		} else {
			oddValidators = append(oddValidators, allValidators[i])
		}
	}

	headersMap := &testHeadersMap{headersByNumber: make(map[uint64]*types.Header)}

	createHeaders(t, headersMap, 0, epochSize-1, 1, nil, allValidators[:validatorSetSize])
	createHeaders(t, headersMap, epochSize, 2*epochSize-1, 2, allValidators[:validatorSetSize], allValidators[validatorSetSize:])
	createHeaders(t, headersMap, 2*epochSize, 3*epochSize-1, 3, allValidators[validatorSetSize:], oddValidators)
	createHeaders(t, headersMap, 3*epochSize, 4*epochSize-1, 4, oddValidators, evenValidators)

	var cases = []struct {
		blockNumber       uint64
		expectedSnapshot  validator.AccountSet
		validatorsOverlap bool
		parents           []*types.Header
	}{
		{4, allValidators[:validatorSetSize], false, nil},
		{1 * epochSize, allValidators[validatorSetSize:], false, nil},
		{13, allValidators[validatorSetSize:], false, nil},
		{27, oddValidators, true, nil},
		{36, evenValidators, true, nil},
		{4, allValidators[:validatorSetSize], false, headersMap.getHeaders()},
		{13, allValidators[validatorSetSize:], false, headersMap.getHeaders()},
		{27, oddValidators, true, headersMap.getHeaders()},
		{36, evenValidators, true, headersMap.getHeaders()},
	}

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}

	for _, c := range cases {
		snapshot, err := testValidatorsCache.GetSnapshot(c.blockNumber, c.parents, nil)

		assertions.NoError(err)
		assertions.Len(snapshot, c.expectedSnapshot.Len())

		if c.validatorsOverlap {
			for _, validator := range c.expectedSnapshot {
				// Order of validators is not preserved, because there are overlapping between validators set.
				// In that case, at the beginning of the set are the ones preserved from the previous validator set.
				// Newly validators are added to the end after the one from previous validator set.
				assertions.True(snapshot.ContainsAddress(validator.Address))
			}
		} else {
			assertions.Equal(c.expectedSnapshot, snapshot)
		}

		assertions.NoError(testValidatorsCache.cleanValidatorsCache())

		if c.parents != nil {
			blockchainMock.AssertNotCalled(t, "GetHeaderByNumber")
		}
	}
}

func TestValidatorsSnapshotCache_GetSnapshot_FetchFromCache(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	const (
		totalValidators  = 10
		validatorSetSize = 5
	)

	allValidators := validator.NewTestValidators(t, totalValidators).GetPublicIdentities()
	epochOneValidators := validator.AccountSet{allValidators[0], allValidators[len(allValidators)-1]}
	epochTwoValidators := allValidators[1 : len(allValidators)-2]

	headersMap := &testHeadersMap{headersByNumber: make(map[uint64]*types.Header)}
	createHeaders(t, headersMap, 0, 9, 1, nil, allValidators)
	createHeaders(t, headersMap, 10, 19, 2, allValidators, epochOneValidators)
	createHeaders(t, headersMap, 20, 29, 3, epochOneValidators, epochTwoValidators)

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}

	require.NoError(testValidatorsCache.storeSnapshot(&validatorSnapshot{1, 10, epochOneValidators}, nil))
	require.NoError(testValidatorsCache.storeSnapshot(&validatorSnapshot{2, 20, epochTwoValidators}, nil))

	// Fetch snapshot from in memory cache
	snapshot, err := testValidatorsCache.GetSnapshot(10, nil, nil)
	require.NoError(err)
	require.Equal(epochOneValidators, snapshot)

	// Invalidate in memory cache
	testValidatorsCache.snapshots = map[uint64]*validatorSnapshot{}
	require.NoError(testValidatorsCache.state.EpochStore.removeAllValidatorSnapshots())
	// Fetch snapshot from database
	snapshot, err = testValidatorsCache.GetSnapshot(10, nil, nil)
	require.NoError(err)
	require.Equal(epochOneValidators, snapshot)

	snapshot, err = testValidatorsCache.GetSnapshot(20, nil, nil)
	require.NoError(err)
	require.Equal(epochTwoValidators, snapshot)
}

/* START ALPHA CHANGE 973 -- prevent future validator snapshot from satisfying older epoch lookup -- */
func TestValidatorsSnapshotCache_GetLastCachedSnapshot_DoesNotReturnFutureDatabaseSnapshot(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	allValidators := validator.NewTestValidators(t, 4).GetPublicIdentities()
	epochOneValidators := allValidators[:2]
	epochTwoValidators := allValidators[2:]

	cache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), new(blockchainMock)),
	}

	require.NoError(cache.storeSnapshot(&validatorSnapshot{1, 10, epochOneValidators}, nil))
	require.NoError(cache.storeSnapshot(&validatorSnapshot{2, 20, epochTwoValidators}, nil))

	cache.snapshots = map[uint64]*validatorSnapshot{}

	snapshot, err := cache.getLastCachedSnapshot(1, nil)
	require.NoError(err)
	require.NotNil(snapshot)
	require.Equal(uint64(1), snapshot.Epoch)
	require.Equal(epochOneValidators, snapshot.Snapshot)
}

/* FINISH ALPHA CHANGE 973 -- prevent future validator snapshot from satisfying older epoch lookup -- */

func TestValidatorsSnapshotCache_Cleanup(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	blockchainMock := new(blockchainMock)
	cache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}
	snapshot := validator.NewTestValidators(t, 3).GetPublicIdentities()
	maxEpoch := uint64(0)

	for i := uint64(0); i < validatorSnapshotLimit; i++ {
		require.NoError(cache.storeSnapshot(&validatorSnapshot{i, i * 10, snapshot}, nil))
		maxEpoch++
	}

	require.NoError(cache.cleanup(nil))

	// assertions for remaining snapshots in the in memory cache
	require.Len(cache.snapshots, numberOfSnapshotsToLeaveInMemory)

	currentEpoch := maxEpoch

	for i := 0; i < numberOfSnapshotsToLeaveInMemory; i++ {
		currentEpoch--
		currentSnapshot, snapExists := cache.snapshots[currentEpoch]
		require.True(snapExists, fmt.Sprintf("failed to fetch in memory snapshot for epoch %d", currentEpoch))
		require.Equal(snapshot, currentSnapshot.Snapshot, fmt.Sprintf("snapshots for epoch %d are not equal", currentEpoch))
	}

	stats, err := cache.state.EpochStore.validatorSnapshotsDBStats()
	require.NoError(err)

	// assertions for remaining snapshots in database
	/* START ALPHA CHANGE 1548 -- assert MDBX validator snapshot entry count -- */
	require.Equal(uint64(numberOfSnapshotsToLeaveInDB), stats.Entries)
	/* FINISH ALPHA CHANGE 1548 -- assert MDBX validator snapshot entry count -- */

	currentEpoch = maxEpoch

	for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ {
		currentEpoch--
		currentSnapshot, err := cache.state.EpochStore.getValidatorSnapshot(currentEpoch)
		require.NoError(err, fmt.Sprintf("failed to fetch database snapshot for epoch %d", currentEpoch))
		require.Equal(snapshot, currentSnapshot.Snapshot, fmt.Sprintf("snapshots for epoch %d are not equal", currentEpoch))
	}
}

func TestValidatorsSnapshotCache_ComputeSnapshot_UnknownBlock(t *testing.T) {
	t.Parallel()
	assertions := assert.New(t)

	const (
		totalValidators  = 15
		validatorSetSize = totalValidators / 2
		epochSize        = uint64(10)
	)

	allValidators := validator.NewTestValidators(t, totalValidators).GetPublicIdentities()
	headersMap := &testHeadersMap{}
	headersMap.addHeader(createValidatorDeltaHeader(t, 0, 0, nil, allValidators[:validatorSetSize]))
	headersMap.addHeader(createValidatorDeltaHeader(t, 1*epochSize, 1, allValidators[:validatorSetSize], allValidators[validatorSetSize:]))

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}

	snapshot, err := testValidatorsCache.computeSnapshot(nil, 5*epochSize, nil)
	assertions.Nil(snapshot)
	assertions.ErrorContains(err, "unknown block. Block number=50")
}

func TestValidatorsSnapshotCache_ComputeSnapshot_IncorrectExtra(t *testing.T) {
	t.Parallel()
	assertions := assert.New(t)

	const (
		totalValidators  = 6
		validatorSetSize = totalValidators / 2
		epochSize        = uint64(10)
	)

	allValidators := validator.NewTestValidators(t, totalValidators).GetPublicIdentities()
	headersMap := &testHeadersMap{}
	invalidHeader := createValidatorDeltaHeader(t, 1*epochSize, 1, allValidators[:validatorSetSize], allValidators[validatorSetSize:])
	invalidHeader.ExtraData = []byte{0x2, 0x7}
	headersMap.addHeader(invalidHeader)

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}

	snapshot, err := testValidatorsCache.computeSnapshot(nil, 1*epochSize, nil)
	assertions.Nil(snapshot)
	assertions.ErrorContains(err, "failed to decode extra from the block#10: wrong extra size: 2")
}

func TestValidatorsSnapshotCache_ComputeSnapshot_ApplyDeltaFail(t *testing.T) {
	t.Parallel()
	assertions := assert.New(t)

	const (
		totalValidators  = 6
		validatorSetSize = totalValidators / 2
		epochSize        = uint64(10)
	)

	allValidators := validator.NewTestValidators(t, totalValidators).GetPublicIdentities()
	headersMap := &testHeadersMap{}
	headersMap.addHeader(createValidatorDeltaHeader(t, 0, 0, nil, allValidators[:validatorSetSize]))
	headersMap.addHeader(createValidatorDeltaHeader(t, 1*epochSize, 1, nil, allValidators[:validatorSetSize]))

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}

	snapshot, err := testValidatorsCache.computeSnapshot(&validatorSnapshot{0, 0, allValidators}, 1*epochSize, nil)
	assertions.Nil(snapshot)
	assertions.ErrorContains(err, "failed to apply delta to the validators snapshot, block#10")
}

func TestValidatorsSnapshotCache_Empty(t *testing.T) {
	t.Parallel()

	headersMap := &testHeadersMap{headersByNumber: make(map[uint64]*types.Header)}

	createHeaders(t, headersMap, 0, 1, 1, nil, nil)

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}

	_, err := testValidatorsCache.GetSnapshot(1, nil, nil)
	assert.ErrorContains(t, err, "validator snapshot is empty for block")
}

/* START ALPHA CHANGE 2578 -- cover checkpoint guards, genesis snapshots, and sparse cache selection -- */
func TestValidatorsSnapshotCache_EpochGuards(t *testing.T) {
	t.Parallel()

	accounts := validator.NewTestValidators(t, 1).GetPublicIdentities()

	for _, tc := range []struct {
		name        string
		blockNumber uint64
		extra       *Extra
		errorText   string
	}{
		{
			name:        "missing checkpoint in ordinary block",
			blockNumber: 1,
			extra:       &Extra{},
			errorText:   "checkpoint data for block 1 is missing",
		},
		{
			name:        "missing checkpoint before empty delta inspection",
			blockNumber: 1,
			extra:       &Extra{Validators: &validator.ValidatorSetDelta{}},
			errorText:   "checkpoint data for block 1 is missing",
		},
		{
			name:        "ordinary block with epoch zero",
			blockNumber: 1,
			extra:       &Extra{Checkpoint: &CheckpointData{}},
			errorText:   "invalid checkpoint epoch for non-genesis block 1",
		},
		{
			name:        "epoch zero with empty delta",
			blockNumber: 1,
			extra: &Extra{
				Checkpoint: &CheckpointData{},
				Validators: &validator.ValidatorSetDelta{},
			},
			errorText: "invalid checkpoint epoch for non-genesis block 1",
		},
		{
			name:        "epoch zero with nonempty delta",
			blockNumber: 1,
			extra: &Extra{
				Checkpoint: &CheckpointData{},
				Validators: &validator.ValidatorSetDelta{Added: accounts},
			},
			errorText: "invalid checkpoint epoch for non-genesis block 1",
		},
		{
			name:        "malformed genesis cannot underflow",
			blockNumber: 0,
			extra:       &Extra{Checkpoint: &CheckpointData{}},
			errorText:   "cannot derive validator snapshot epoch",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := &types.Header{
				Number:    tc.blockNumber,
				ExtraData: tc.extra.MarshalRLPTo(nil),
			}
			chain := new(blockchainMock)
			chain.On("GetHeaderByNumber", tc.blockNumber).Return(header, true).Once()

			cache := newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), chain)
			snapshot, err := cache.GetSnapshot(tc.blockNumber, nil, nil)

			require.ErrorContains(t, err, tc.errorText)
			require.Nil(t, snapshot)
			require.Empty(t, cache.snapshots)
			chain.AssertNotCalled(t, "GetHeaderByNumber", tc.blockNumber+1)
			chain.AssertExpectations(t)

			stats, err := cache.state.EpochStore.validatorSnapshotsDBStats()
			require.NoError(t, err)
			require.Equal(t, uint64(0), stats.Entries)
		})
	}
}

func TestValidatorsSnapshotCache_EpochZeroGenesisPreserved(t *testing.T) {
	t.Parallel()

	accounts := validator.NewTestValidators(t, 3).GetPublicIdentities()
	genesis := &types.Header{
		Number:    0,
		ExtraData: BuildGenesisExtraData(accounts),
	}
	first := &types.Header{
		Number: 1,
		ExtraData: (&Extra{
			Checkpoint: &CheckpointData{EpochNumber: 1},
		}).MarshalRLPTo(nil),
	}

	chain := new(blockchainMock)
	chain.On("GetHeaderByNumber", uint64(0)).Return(genesis, true).Twice()
	chain.On("GetHeaderByNumber", uint64(1)).Return(first, true).Once()

	cache := newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), chain)

	snapshot, err := cache.GetSnapshot(0, nil, nil)
	require.NoError(t, err)
	require.Equal(t, accounts, snapshot)

	snapshot, err = cache.GetSnapshot(1, nil, nil)
	require.NoError(t, err)
	require.Equal(t, accounts, snapshot)

	chain.AssertNotCalled(t, "GetHeaderByNumber", uint64(2))
	chain.AssertExpectations(t)
}

func TestValidatorsSnapshotCache_GetLastCachedSnapshot_SparseEpochs(t *testing.T) {
	t.Parallel()

	const largeEpoch uint64 = 1_000_000_000_000_000_000
	accounts := validator.NewTestValidators(t, 1).GetPublicIdentities()

	for _, tc := range []struct {
		name          string
		requested     uint64
		memory        []uint64
		database      []uint64
		expectedEpoch uint64
		found         bool
	}{
		{"huge gap with future memory entry", largeEpoch, []uint64{0, 7, largeEpoch + 1}, nil, 7, true},
		{"maximum unsigned request", ^uint64(0), []uint64{0, 7, largeEpoch}, nil, largeEpoch, true},
		{"empty cache at maximum unsigned request", ^uint64(0), nil, nil, 0, false},
		{"database newer than memory", largeEpoch, []uint64{7}, []uint64{8}, 8, true},
		{"memory newer than database", largeEpoch, []uint64{9}, []uint64{8}, 9, true},
		{"future database entry excluded", 5, []uint64{0, 4, 6}, []uint64{6}, 4, true},
		{"only future entries at epoch zero", 0, []uint64{1}, []uint64{2}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := newValidatorsSnapshotCache(
				hclog.NewNullLogger(), newTestState(t), new(blockchainMock),
			)

			for _, epoch := range tc.memory {
				cache.snapshots[epoch] = &validatorSnapshot{
					Epoch:    epoch,
					Snapshot: accounts,
				}
			}
			/* Nil entries must not be selected as usable snapshots. */
			cache.snapshots[3] = nil

			for _, epoch := range tc.database {
				require.NoError(t, cache.state.EpochStore.insertValidatorSnapshot(
					&validatorSnapshot{Epoch: epoch, Snapshot: accounts}, nil,
				))
			}

			snapshot, err := cache.getLastCachedSnapshot(tc.requested, nil)
			require.NoError(t, err)

			if !tc.found {
				require.Nil(t, snapshot)

				return
			}

			require.NotNil(t, snapshot)
			require.Equal(t, tc.expectedEpoch, snapshot.Epoch)
			require.Equal(t, accounts, snapshot.Snapshot)
			require.LessOrEqual(t, snapshot.Epoch, tc.requested)
		})
	}
}

func TestValidatorsSnapshotCache_GetLastCachedSnapshot_ExactHit(t *testing.T) {
	t.Parallel()

	expected := &validatorSnapshot{Epoch: 9}
	cache := &validatorsSnapshotCache{
		snapshots: map[uint64]*validatorSnapshot{9: expected},
	}

	/* No state or logger is supplied: an exact hit must return before using either. */
	snapshot, err := cache.getLastCachedSnapshot(9, nil)
	require.NoError(t, err)
	require.Same(t, expected, snapshot)
}

/* FINISH ALPHA CHANGE 2578 -- cover checkpoint guards, genesis snapshots, and sparse cache selection -- */

func createHeaders(t *testing.T, headersMap *testHeadersMap,
	fromBlock, toBlock, epoch uint64, oldValidators, newValidators validator.AccountSet) {
	t.Helper()

	headersMap.addHeader(createValidatorDeltaHeader(t, fromBlock, epoch-1, oldValidators, newValidators))

	for i := fromBlock + 1; i <= toBlock; i++ {
		headersMap.addHeader(createValidatorDeltaHeader(t, i, epoch, nil, nil))
	}
}

func createValidatorDeltaHeader(t *testing.T, blockNumber, epoch uint64, oldValidatorSet, newValidatorSet validator.AccountSet) *types.Header {
	t.Helper()

	delta, _ := validator.CreateValidatorSetDelta(oldValidatorSet, newValidatorSet)
	extra := &Extra{Validators: delta, Checkpoint: &CheckpointData{EpochNumber: epoch}}

	return &types.Header{
		Number:    blockNumber,
		ExtraData: extra.MarshalRLPTo(nil),
	}
}

type testValidatorsCache struct {
	*validatorsSnapshotCache
}

func (c *testValidatorsCache) cleanValidatorsCache() error {
	c.snapshots = make(map[uint64]*validatorSnapshot)

	return c.state.EpochStore.removeAllValidatorSnapshots()
}
