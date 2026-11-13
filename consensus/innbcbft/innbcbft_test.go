package innbcbft

import (
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/helper/progress"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/txpool"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	/* START ALPHA CHANGE 604 -- remove ethgo import used only by deleted reward config test fields -- */ /* FINISH ALPHA CHANGE 604 -- remove ethgo import used only by deleted reward config test fields -- */)

// the test initializes innbcbft and chain mock (map of headers) after which a new header is verified
// firstly, two invalid situation of header verifications are triggered (missing Committed field and invalid validators for ParentCommitted)
// afterwards, valid inclusion into the block chain is checked
// and at the end there is a situation when header is already a part of blockchain
func TestINNBCBFT_VerifyHeader(t *testing.T) {
	t.Parallel()

	const (
		allValidatorsSize = 6 // overall there are 6 validators
		validatorSetSize  = 5 // only 5 validators are active at the time
		fixedEpochSize    = uint64(10)
	)

	updateHeaderExtra := func(header *types.Header,
		validators *validator.ValidatorSetDelta,
		parentSignature *Signature,
		checkpointData *CheckpointData,
		committedAccounts []*wallet.Account) *Signature {
		extra := &Extra{
			Validators: validators,
			Parent:     parentSignature,
			Checkpoint: checkpointData,
			Committed:  &Signature{},
		}

		if extra.Checkpoint == nil {
			extra.Checkpoint = &CheckpointData{}
		}

		header.ExtraData = extra.MarshalRLPTo(nil)
		header.ComputeHash()

		if len(committedAccounts) > 0 {
			checkpointHash, err := extra.Checkpoint.Hash(0, header.Number, header.Hash)
			require.NoError(t, err)

			extra.Committed = createSignature(t, committedAccounts, checkpointHash, signer.DomainCheckpointManager)
			header.ExtraData = extra.MarshalRLPTo(nil)
		}

		return extra.Committed
	}

	// create all validators
	validators := validator.NewTestValidators(t, allValidatorsSize)

	// create configuration
	innbcBFTConfig := INNBCBFTConfig{
		InitialValidatorSet: validators.GetParamValidators(),
		EpochSize:           fixedEpochSize,
		SprintSize:          5,
		/* START ALPHA CHANGE 3320 -- retain finalized-header signature coverage with an explicit interval -- */
		BlockTime: testChainClockConfig().BlockTime,
		/* FINISH ALPHA CHANGE 3320 -- retain finalized-header signature coverage with an explicit interval -- */
	}

	validatorSet := validators.GetPublicIdentities()
	accounts := validators.GetPrivateIdentities()

	// calculate validators before and after the end of the first epoch
	validatorSetParent, validatorSetCurrent := validatorSet[:len(validatorSet)-1], validatorSet[1:]
	accountSetParent, accountSetCurrent := accounts[:len(accounts)-1], accounts[1:]

	// create header map to simulate blockchain
	headersMap := &testHeadersMap{}

	// create genesis header
	genesisDelta, err := validator.CreateValidatorSetDelta(nil, validatorSetParent)
	require.NoError(t, err)

	genesisHeader := &types.Header{Number: 0}
	updateHeaderExtra(genesisHeader, genesisDelta, nil, nil, nil)

	// add genesis header to map
	headersMap.addHeader(genesisHeader)

	// create headers from 1 to 9
	for i := uint64(1); i < innbcBFTConfig.EpochSize; i++ {
		delta, err := validator.CreateValidatorSetDelta(validatorSetParent, validatorSetParent)
		require.NoError(t, err)

		header := &types.Header{Number: i}
		updateHeaderExtra(header, delta, nil, &CheckpointData{EpochNumber: 1}, nil)

		// add headers from 1 to 9 to map (blockchain imitation)
		headersMap.addHeader(header)
	}

	// mock blockchain
	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)
	blockchainMock.On("GetHeaderByHash", mock.Anything).Return(headersMap.getHeaderByHash)

	// create innbcbft with appropriate mocks
	innbcbft := &INNBCBFT{
		closeCh:         make(chan struct{}),
		logger:          hclog.NewNullLogger(),
		consensusConfig: &innbcBFTConfig,
		blockchain:      blockchainMock,
		validatorsCache: newValidatorsSnapshotCache(
			hclog.NewNullLogger(),
			newTestState(t),
			blockchainMock,
		),
	}

	// create parent header (block 10)
	parentDelta, err := validator.CreateValidatorSetDelta(validatorSetParent, validatorSetCurrent)
	require.NoError(t, err)

	// START ALPHA CHANGE 1022 -- keep VerifyHeader test timestamps safely inside drift window --
	// FINISH ALPHA CHANGE 1022 -- keep VerifyHeader test timestamps safely inside drift window --
	/* START ALPHA CHANGE 1909 -- make signature verification fixture independent of wall-clock adjustments -- */
	const parentTimestamp = uint64(1)

	parentHeader := &types.Header{
		Number:    innbcBFTConfig.EpochSize,
		Timestamp: parentTimestamp,
	}
	/* FINISH ALPHA CHANGE 1909 -- make signature verification fixture independent of wall-clock adjustments -- */
	parentCommitment := updateHeaderExtra(parentHeader, parentDelta, nil, &CheckpointData{EpochNumber: 1}, accountSetParent)

	// add parent header to map
	headersMap.addHeader(parentHeader)

	// create current header (block 11) with all appropriate fields required for validation
	currentDelta, err := validator.CreateValidatorSetDelta(validatorSetCurrent, validatorSetCurrent)
	require.NoError(t, err)

	currentHeader := &types.Header{
		Number:     innbcBFTConfig.EpochSize + 1,
		ParentHash: parentHeader.Hash,
		Timestamp:  parentHeader.Timestamp + 1,
		MixHash:    INNBCBFTMixDigest,
		Difficulty: 1,
	}
	updateHeaderExtra(currentHeader, currentDelta, nil,
		&CheckpointData{
			EpochNumber:           2,
			CurrentValidatorsHash: types.StringToHash("Foo"),
			NextValidatorsHash:    types.StringToHash("Bar"),
		}, nil)

	currentHeader.Hash[0] = currentHeader.Hash[0] + 1
	assert.ErrorContains(t, innbcbft.VerifyHeader(currentHeader), "invalid header hash")

	// omit Parent field (parent signature) intentionally
	updateHeaderExtra(currentHeader, currentDelta, nil,
		&CheckpointData{
			EpochNumber:           1,
			CurrentValidatorsHash: types.StringToHash("Foo"),
			NextValidatorsHash:    types.StringToHash("Bar")},
		accountSetCurrent)

	// since parent signature is intentionally disregarded the following error is expected
	assert.ErrorContains(t, innbcbft.VerifyHeader(currentHeader), "failed to verify signatures for parent of block")

	/* START ALPHA CHANGE 2294 -- use actual validator hashes in the valid finalized header fixture -- */
	currentValidatorsHash, err := validatorSetCurrent.Hash()
	require.NoError(t, err)

	updateHeaderExtra(currentHeader, currentDelta, parentCommitment,
		&CheckpointData{
			EpochNumber:           2,
			CurrentValidatorsHash: currentValidatorsHash,
			NextValidatorsHash:    currentValidatorsHash},
		accountSetCurrent)

	assert.NoError(t, innbcbft.VerifyHeader(currentHeader))
	/* FINISH ALPHA CHANGE 2294 -- use actual validator hashes in the valid finalized header fixture -- */

	// clean validator snapshot cache (re-instantiate it), submit invalid validator set for parent signature and expect the following error
	innbcbft.validatorsCache = newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock)
	assert.NoError(t, innbcbft.validatorsCache.storeSnapshot(
		&validatorSnapshot{Epoch: 0, Snapshot: validatorSetCurrent}, nil)) // invalid validator set is submitted
	assert.NoError(t, innbcbft.validatorsCache.storeSnapshot(
		&validatorSnapshot{Epoch: 1, Snapshot: validatorSetCurrent}, nil))
	assert.ErrorContains(t, innbcbft.VerifyHeader(currentHeader), "failed to verify signatures for parent of block")

	// clean validators cache again and set valid snapshots
	innbcbft.validatorsCache = newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock)
	assert.NoError(t, innbcbft.validatorsCache.storeSnapshot(
		&validatorSnapshot{Epoch: 0, Snapshot: validatorSetParent}, nil))
	assert.NoError(t, innbcbft.validatorsCache.storeSnapshot(
		&validatorSnapshot{Epoch: 1, Snapshot: validatorSetCurrent}, nil))
	assert.NoError(t, innbcbft.VerifyHeader(currentHeader))

	// add current header to the blockchain (headersMap) and try validating again
	headersMap.addHeader(currentHeader)
	assert.NoError(t, innbcbft.VerifyHeader(currentHeader))
}

func TestINNBCBFT_Close(t *testing.T) {
	t.Parallel()

	/* START ALPHA CHANGE 1072 -- verify INNBCBFT close is idempotent -- */
	t.Run("successful close only closes resources once", func(t *testing.T) {
		t.Parallel()

		syncer := &syncerMock{}
		syncer.On("Close").Return(error(nil)).Once()

		/* START ALPHA CHANGE 1439 -- verify INNBCBFT releases its consensus database -- */
		consensusState := newTestState(t)
		/* FINISH ALPHA CHANGE 1439 -- verify INNBCBFT releases its consensus database -- */

		innbcbft := INNBCBFT{
			closeCh: make(chan struct{}),
			syncer:  syncer,
			runtime: &consensusRuntime{
				/* START ALPHA CHANGE 323 -- remove bridge dummy managers from close test runtime -- */
				/* FINISH ALPHA CHANGE 323 -- remove bridge dummy managers from close test runtime -- */
			},
			state: consensusState,
		}

		assert.NoError(t, innbcbft.Close())
		assert.NoError(t, innbcbft.Close())

		<-innbcbft.closeCh

		/* START ALPHA CHANGE 1440 -- confirm operations fail after consensus state closure -- */
		_, err := consensusState.beginDBTransaction(false)
		assert.Error(t, err)
		/* FINISH ALPHA CHANGE 1440 -- confirm operations fail after consensus state closure -- */

		syncer.AssertExpectations(t)
	})

	t.Run("syncer close error is returned after completing shutdown", func(t *testing.T) {
		t.Parallel()

		errExpected := errors.New("something")
		syncer := &syncerMock{}
		syncer.On("Close").Return(errExpected).Once()

		innbcbft := INNBCBFT{
			closeCh: make(chan struct{}),
			syncer:  syncer,
			runtime: &consensusRuntime{},
		}

		assert.ErrorIs(t, innbcbft.Close(), errExpected)
		assert.ErrorIs(t, innbcbft.Close(), errExpected)

		select {
		case <-innbcbft.closeCh:
		case <-time.After(time.Millisecond * 100):
			assert.Fail(t, "channel closing was not invoked")
		}

		syncer.AssertExpectations(t)
	})
	/* FINISH ALPHA CHANGE 1072 -- verify INNBCBFT close is idempotent -- */
}

/* START ALPHA CHANGE 1469 -- test fatal consensus signaling is one-shot -- */
func TestINNBCBFT_ReportFatalErrorIsOneShot(t *testing.T) {
	t.Parallel()

	firstErr := errors.New("first fatal error")
	secondErr := errors.New("second fatal error")

	innbcbft := &INNBCBFT{
		logger:      hclog.NewNullLogger(),
		fatalStopCh: make(chan struct{}),
		fatalErrCh:  make(chan error, 1),
	}

	innbcbft.ReportFatalError(firstErr)
	innbcbft.ReportFatalError(secondErr)

	select {
	case fatalErr := <-innbcbft.FatalErrorCh():
		require.ErrorIs(t, fatalErr, firstErr)
	case <-time.After(time.Second):
		t.Fatal("fatal consensus error was not reported")
	}

	select {
	case <-innbcbft.fatalStopCh:
	default:
		t.Fatal("fatal stop channel was not closed")
	}

	select {
	case unexpectedErr := <-innbcbft.FatalErrorCh():
		t.Fatalf("unexpected second fatal error: %v", unexpectedErr)
	default:
	}
}

/* FINISH ALPHA CHANGE 1469 -- test fatal consensus signaling is one-shot -- */

func TestINNBCBFT_GetSyncProgression(t *testing.T) {
	t.Parallel()

	result := &progress.Progression{}

	syncer := &syncerMock{}
	syncer.On("GetSyncProgression", mock.Anything).Return(result).Once()

	innbcbft := INNBCBFT{
		syncer: syncer,
	}

	assert.Equal(t, result, innbcbft.GetSyncProgression())
}

func Test_Factory(t *testing.T) {
	t.Parallel()

	const epochSize = uint64(141)

	txPool := &txpool.TxPool{}

	params := &consensus.Params{
		TxPool: txPool,
		Logger: hclog.Default(),
		Config: &consensus.Config{
			/* START ALPHA CHANGE 2647 -- supply supported genesis stake in the factory fixture -- */
			Config: map[string]any{
				/* START ALPHA CHANGE 3320 -- supply a valid clock in the factory fixture -- */
				"EpochSize":               epochSize,
				"blockTime":               "1s",
				"validatorRegistryConfig": testValidatorRegistryConfig(types.ZeroAddress),
				/* FINISH ALPHA CHANGE 3320 -- supply a valid clock in the factory fixture -- */
				/* START ALPHA CHANGE 3339 -- preserve successful factory coverage with required native-token configuration -- */
				"nativeTokenConfig": DefaultTokenConfig,
				/* FINISH ALPHA CHANGE 3339 -- preserve successful factory coverage with required native-token configuration -- */
			},
			/* FINISH ALPHA CHANGE 2647 -- supply supported genesis stake in the factory fixture -- */
		},
	}

	r, err := Factory(params)

	require.NoError(t, err)
	require.NotNil(t, r)

	innbcbft, ok := r.(*INNBCBFT)
	require.True(t, ok)

	assert.Equal(t, txPool, innbcbft.txPool)
	assert.Equal(t, epochSize, innbcbft.consensusConfig.EpochSize)
	assert.Equal(t, params, innbcbft.config)
}

func Test_GenesisPostHookFactory(t *testing.T) {
	t.Parallel()

	const (
		epochSize     = 15
		maxValidators = 150
	)

	validators := validator.NewTestValidators(t, 6)
	/* START ALPHA CHANGE 120 -- remove bridge config expectation from genesis post-hook tests -- */
	cases := []struct {
		name        string
		config      *INNBCBFTConfig
		expectedErr error
	}{
		/* FINISH ALPHA CHANGE 120 -- remove bridge config expectation from genesis post-hook tests -- */
		/* START ALPHA CHANGE 786 -- test mandatory INNBC native token genesis post-hook path -- */
		{
			name: "mandatory INNBC native token",
			config: &INNBCBFTConfig{
				InitialValidatorSet: validators.GetParamValidators(),
				EpochSize:           epochSize,
				/* START ALPHA CHANGE 3320 -- reach native-token initialization with valid clock configuration -- */
				BlockTime: testChainClockConfig().BlockTime,
				/* FINISH ALPHA CHANGE 3320 -- reach native-token initialization with valid clock configuration -- */
				NativeTokenConfig:       &TokenConfig{Name: "Test INNBC", Symbol: "TINNBC", Decimals: 18},
				MaxValidatorSetSize:     maxValidators,
				ValidatorRegistryConfig: testValidatorRegistryConfig(validators.GetParamValidators()[0].Address),
			},
		},
		/* FINISH ALPHA CHANGE 786 -- test mandatory INNBC native token genesis post-hook path -- */
		/* START ALPHA CHANGE 123 -- remove missing bridge configuration test case -- */
		/* FINISH ALPHA CHANGE 123 -- remove missing bridge configuration test case -- */
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			params := &chain.Params{
				/* START ALPHA CHANGE 124 -- remove bridge allow-list from genesis post-hook test params -- */
				Engine: map[string]any{ConsensusName: tc.config},
				/* FINISH ALPHA CHANGE 124 -- remove bridge allow-list from genesis post-hook test params -- */
			}
			chainConfig := &chain.Chain{Params: params, Genesis: &chain.Genesis{Alloc: make(map[types.Address]*chain.GenesisAccount)}}
			initHandler := GenesisPostHookFactory(chainConfig, ConsensusName)
			require.NotNil(t, initHandler)

			transition := newTestTransition(t, nil)
			if tc.expectedErr == nil {
				require.NoError(t, initHandler(transition))
			} else {
				require.ErrorIs(t, initHandler(transition), tc.expectedErr)
			}
		})
	}
}

/* START ALPHA CHANGE 394 -- restore local genesis transition helper for INNBCBFT tests -- */
func newTestTransition(t *testing.T, alloc map[types.Address]*chain.GenesisAccount) *state.Transition {
	t.Helper()

	st := itrie.NewState(itrie.NewMemoryStorage())

	executor := state.NewExecutor(&chain.Params{
		Forks: chain.AllForksEnabled,
	}, st, hclog.NewNullLogger())

	rootHash, err := executor.WriteGenesis(alloc, types.Hash{})
	require.NoError(t, err)

	/* START ALPHA CHANGE 3810 -- adapt consensus transition fixture to checked hash lookups -- */
	executor.GetHash = func(h *types.Header) state.GetHashByNumber {
		return func(i uint64) (types.Hash, error) {
			return rootHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt consensus transition fixture to checked hash lookups -- */

	transition, err := executor.BeginTxn(
		rootHash,
		&types.Header{},
		types.ZeroAddress,
	)
	require.NoError(t, err)

	return transition
}

/* FINISH ALPHA CHANGE 394 -- restore local genesis transition helper for INNBCBFT tests -- */
