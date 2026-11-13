package innbcbft

import (
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"

	/* START ALPHA CHANGE 2063 -- execute a real ValidatorSet epoch transition at the gas floor -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/contracts"

	/* FINISH ALPHA CHANGE 2063 -- execute a real ValidatorSet epoch transition at the gas floor -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

/* START ALPHA CHANGE 817 -- test deterministic chain-time proposal timestamp -- */
func TestBlockBuilder_ResetUsesParentTimestampPlusBlockTime(t *testing.T) {
	t.Parallel()

	const parentTimestamp = uint64(100)

	/* START ALPHA CHANGE 820 -- initialize executor hash callback in timestamp test -- */
	executor := state.NewExecutor(&chain.Params{
		ChainID: 100,
		Forks:   &chain.Forks{},
	}, itrie.NewState(itrie.NewMemoryStorage()), hclog.NewNullLogger())

	/* START ALPHA CHANGE 3810 -- adapt timestamp fixture to checked hash lookups -- */
	executor.GetHash = func(header *types.Header) func(i uint64) (types.Hash, error) {
		return func(i uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt timestamp fixture to checked hash lookups -- */
	/* FINISH ALPHA CHANGE 820 -- initialize executor hash callback in timestamp test -- */

	/* START ALPHA CHANGE 3322 -- compare real construction with header validation at multiple intervals -- */
	for _, seconds := range []uint64{1, 2, 5} {
		interval := time.Duration(seconds) * time.Second
		bb := NewBlockBuilder(&BlockBuilderParams{
			BlockTime: interval,
			Parent: &types.Header{
				Timestamp: parentTimestamp,
				StateRoot: types.EmptyRootHash,
			},
			Coinbase: types.ZeroAddress,
			Executor: executor,
			GasLimit: 1_000_000,
			TxPool:   &txPoolMock{},
			Logger:   hclog.NewNullLogger(),
		})

		require.NoError(t, bb.Reset())
		require.Equal(t, parentTimestamp+seconds, bb.header.Timestamp)

		// Supply fields normally finalized by proposal construction.
		bb.header.ExtraData = make([]byte, ExtraVanity)
		bb.header.MixHash = INNBCBFTMixDigest
		bb.header.ComputeHash()
		require.NoError(t, validateHeaderFields(bb.params.Parent, bb.header, interval))
	}
	/* FINISH ALPHA CHANGE 3322 -- compare real construction with header validation at multiple intervals -- */
}

/* FINISH ALPHA CHANGE 817 -- test deterministic chain-time proposal timestamp -- */

func TestBlockBuilder_BuildBlockTxOneFailedTxAndOneTakesTooMuchGas(t *testing.T) {
	t.Parallel()

	const (
		amount        = 1_000
		gasPrice      = 1_000
		gasLimit      = 21000
		blockGasLimit = 21000 * 3
		chainID       = 100
	)

	accounts := [6]*wallet.Account{}

	for i := range accounts {
		accounts[i] = generateTestAccount(t)
	}

	forks := &chain.Forks{}
	logger := hclog.NewNullLogger()
	signer := crypto.NewSigner(forks.At(0), chainID)

	mchain := &chain.Chain{
		Params: &chain.Params{
			ChainID: chainID,
			Forks:   forks,
		},
	}

	mstate := itrie.NewState(itrie.NewMemoryStorage())
	executor := state.NewExecutor(mchain.Params, mstate, logger)

	/* START ALPHA CHANGE 3810 -- preserve encoded fixture hashes with explicit lookup success -- */
	executor.GetHash = func(header *types.Header) func(i uint64) (types.Hash, error) {
		return func(i uint64) (res types.Hash, err error) {
			return types.BytesToHash(common.EncodeUint64ToBytes(i)), nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- preserve encoded fixture hashes with explicit lookup success -- */

	balanceMap := map[types.Address]*chain.GenesisAccount{}

	for i, acc := range accounts {
		// the third tx will fail because of insufficient balance
		if i != 2 {
			balanceMap[types.Address(acc.Ecdsa.Address())] = &chain.GenesisAccount{
				Balance: ethgo.Ether(1),
			}
		}
	}

	hash, err := executor.WriteGenesis(balanceMap, types.ZeroHash)

	require.NoError(t, err)
	require.NotEqual(t, types.ZeroHash, hash)

	// Gas Limit is important to be high for tx pool
	parentHeader := &types.Header{StateRoot: hash, GasLimit: 1_000_000_000_000_000}

	txPool := &txPoolMock{}
	txPool.On("Prepare").Once()

	for i, acc := range accounts {
		receiver := types.Address(acc.Ecdsa.Address())
		privateKey, err := acc.GetEcdsaPrivateKey()

		require.NoError(t, err)

		tx := &types.Transaction{
			Value:    big.NewInt(amount),
			GasPrice: big.NewInt(gasPrice),
			Gas:      gasLimit,
			Nonce:    0,
			To:       &receiver,
		}

		// fifth tx will cause filling to stop
		if i == 4 {
			tx.Gas = blockGasLimit - 1
		}

		tx, err = signer.SignTx(tx, privateKey)
		require.NoError(t, err)

		// all tx until the fifth will be retrieved from the pool
		if i <= 4 {
			txPool.On("Peek").Return(tx).Once()
		}

		// first two and fourth will be added to the block, third will be demoted
		if i == 2 {
			txPool.On("Demote", tx)
		} else if i < 4 {
			txPool.On("Pop", tx)
		}
	}

	bb := NewBlockBuilder(&BlockBuilderParams{
		/* START ALPHA CHANGE 2094 -- allow race-instrumented transaction execution in the builder test -- */
		BlockTime: 2 * time.Second,
		/* FINISH ALPHA CHANGE 2094 -- allow race-instrumented transaction execution in the builder test -- */
		Parent:   parentHeader,
		Coinbase: types.ZeroAddress,
		Executor: executor,
		GasLimit: blockGasLimit,
		TxPool:   txPool,
		Logger:   logger,
	})

	require.NoError(t, bb.Reset())

	/* START ALPHA CHANGE 2159 -- assert successful filling in the healthy block builder fixture -- */
	require.NoError(t, bb.Fill())
	/* FINISH ALPHA CHANGE 2159 -- assert successful filling in the healthy block builder fixture -- */

	fb, err := bb.Build(func(h *types.Header) {
		// fake the logs for bloom
		rs := bb.Receipts()

		if len(rs) == 3 {
			rs[0].Logs = []*types.Log{
				{Address: types.StringToAddress("ff7783")},
			}
			rs[1].Logs = []*types.Log{
				{Address: types.StringToAddress("03bbcc")},
				{Address: types.StringToAddress("112233")},
			}
		}
	})
	require.NoError(t, err)

	txPool.AssertExpectations(t)
	require.Len(t, bb.txns, 3, "Should have 3 transactions but has %d", len(bb.txns))
	require.Len(t, bb.Receipts(), 3)

	// assert logs bloom
	for _, r := range bb.Receipts() {
		for _, l := range r.Logs {
			assert.True(t, fb.Block.Header.LogsBloom.IsLogInBloom(l))
		}
	}

	assert.False(t, fb.Block.Header.LogsBloom.IsLogInBloom(
		&types.Log{Address: types.StringToAddress("999911117777")}))
	assert.False(t, fb.Block.Header.LogsBloom.IsLogInBloom(
		&types.Log{Address: types.StringToAddress("111177779999")}))
}

/* START ALPHA CHANGE 2063 -- execute a real ValidatorSet epoch transition at the gas floor -- */
func TestBlockBuilder_AppliesCommitEpochTransactionAtMinimumBlockGasLimit(t *testing.T) {
	t.Parallel()

	const (
		chainID   = 100
		epochSize = 10
	)

	allocations := map[types.Address]*chain.GenesisAccount{
		contracts.ValidatorSetContract: {
			Balance: big.NewInt(0),
			Code:    contractsapi.GenesisProxy.DeployedBytecode,
		},
		contracts.ValidatorSetContractV1: {
			Balance: big.NewInt(0),
			Code:    contractsapi.ValidatorSet.DeployedBytecode,
		},
	}

	chainParams := &chain.Params{
		ChainID: chainID,
		Forks:   chain.AllForksEnabled,
	}

	executor := state.NewExecutor(
		chainParams,
		itrie.NewState(itrie.NewMemoryStorage()),
		hclog.NewNullLogger(),
	)

	validatorSetConfig := INNBCBFTConfig{
		EpochSize: epochSize,
	}

	executor.GenesisPostHook = func(transition *state.Transition) error {
		proxyToImplementation := map[types.Address]types.Address{
			contracts.ValidatorSetContract: contracts.ValidatorSetContractV1,
		}

		if err := initProxies(
			transition,
			contracts.INNBCProxyAdminExecutorContract,
			proxyToImplementation,
		); err != nil {
			return err
		}

		return initValidatorSet(validatorSetConfig, transition)
	}

	rootHash, err := executor.WriteGenesis(allocations, types.ZeroHash)
	require.NoError(t, err)

	/* START ALPHA CHANGE 3810 -- preserve epoch fixture hashes with explicit lookup success -- */
	executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
		return func(uint64) (types.Hash, error) {
			return rootHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- preserve epoch fixture hashes with explicit lookup success -- */

	parent := &types.Header{
		Number:     epochSize - 1,
		StateRoot:  rootHash,
		GasLimit:   types.StateTransactionGasLimit,
		BaseFee:    1,
		Difficulty: 1,
		Timestamp:  100,
	}
	parent.ComputeHash()

	builder := NewBlockBuilder(&BlockBuilderParams{
		Parent:    parent,
		Executor:  executor,
		Coinbase:  types.ZeroAddress,
		GasLimit:  types.StateTransactionGasLimit,
		BlockTime: 2 * time.Second,
		BaseFee:   1,
		TxPool:    &txPoolMock{},
		Logger:    hclog.NewNullLogger(),
	})

	require.NoError(t, builder.Reset())

	fsm := &fsm{
		parent:           parent,
		commitEpochInput: createTestCommitEpochInput(t, 1, epochSize),
	}

	commitEpochTx, err := fsm.createCommitEpochTx()
	require.NoError(t, err)
	require.Equal(t, uint64(types.StateTransactionGasLimit), commitEpochTx.Gas)

	require.NoError(t, builder.WriteTx(commitEpochTx))

	fullBlock, err := builder.Build(nil)
	require.NoError(t, err)
	require.NotNil(t, fullBlock)
	require.Equal(
		t,
		uint64(types.StateTransactionGasLimit),
		fullBlock.Block.Header.GasLimit,
	)
	require.LessOrEqual(
		t,
		fullBlock.Block.Header.GasUsed,
		uint64(types.StateTransactionGasLimit),
	)
	require.Len(t, fullBlock.Block.Transactions, 1)
	require.Equal(t, commitEpochTx.Hash, fullBlock.Block.Transactions[0].Hash)

	require.Len(t, fullBlock.Receipts, 1)
	receipt := fullBlock.Receipts[0]
	require.Equal(t, commitEpochTx.Hash, receipt.TxHash)
	require.NotNil(t, receipt.Status)
	require.Equal(t, types.ReceiptSuccess, *receipt.Status)

	require.Len(t, receipt.Logs, 1)
	newEpochLog := receipt.Logs[0]
	require.Equal(t, contracts.ValidatorSetContract, newEpochLog.Address)
	require.Len(t, newEpochLog.Topics, 4)
	require.Equal(
		t,
		types.Hash(contractsapi.ValidatorSet.Abi.Events["NewEpoch"].ID()),
		newEpochLog.Topics[0],
	)
	require.Equal(
		t,
		types.BytesToHash(big.NewInt(1).Bytes()),
		newEpochLog.Topics[1],
	)
	require.Equal(
		t,
		types.BytesToHash(big.NewInt(1).Bytes()),
		newEpochLog.Topics[2],
	)
	require.Equal(
		t,
		types.BytesToHash(big.NewInt(epochSize).Bytes()),
		newEpochLog.Topics[3],
	)
	require.Equal(t, make([]byte, types.HashLength), newEpochLog.Data)

	queryTransition, err := executor.BeginTxn(
		fullBlock.Block.Header.StateRoot,
		fullBlock.Block.Header,
		types.ZeroAddress,
	)
	require.NoError(t, err)

	currentEpochID, err := NewSystemState(
		contracts.ValidatorSetContract,
		NewStateProvider(queryTransition),
	).GetEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(2), currentEpochID)

	/* START ALPHA CHANGE 2949 -- measure the executed epoch state transaction archive encoding -- */
	raw := fullBlock.Block.MarshalRLP()
	var decoded types.Block
	require.NoError(t, decoded.UnmarshalRLP(raw))
	require.Equal(t, raw, decoded.MarshalRLP())
	require.Len(t, decoded.Transactions, 1)
	require.Equal(t, types.StateTx, decoded.Transactions[0].Type)

	t.Logf(
		"class=executed_epoch_state_transaction_not_finalized "+
			"block_record_rlp=%d header_rlp=%d transaction_rlp=%d "+
			"extra_data=%d gas_used=%d gas_limit=%d",
		len(raw),
		len(fullBlock.Block.Header.MarshalRLP()),
		len(commitEpochTx.MarshalRLP()),
		len(fullBlock.Block.Header.ExtraData),
		fullBlock.Block.Header.GasUsed,
		fullBlock.Block.Header.GasLimit,
	)
	/* FINISH ALPHA CHANGE 2949 -- measure the executed epoch state transaction archive encoding -- */
}

/* FINISH ALPHA CHANGE 2063 -- execute a real ValidatorSet epoch transition at the gas floor -- */
