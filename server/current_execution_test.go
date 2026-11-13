/* START ALPHA CHANGE 3414 -- verify current execution behavior without frozen genesis commitments -- */
package server

import (
	"context"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/blockchain"
	mdbxstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage/mdbx"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/innovativebioresearch/innbc-chain/types/buildroot"
	"github.com/stretchr/testify/require"
)

type currentExecutionContext struct {
	fixture       trieRebuildFixture
	storage       itrie.Storage
	executor      *state.Executor
	hub           *jsonRPCHub
	senderNonce   uint64
	senderBalance *big.Int
}

func newCurrentExecutionContext(t *testing.T) *currentExecutionContext {
	t.Helper()

	fixture := newTrieRebuildFixture(t)
	source, err := mdbxstorage.NewMDBXStorageReadOnly(
		datadir.Blockchain(fixture.config.DataDir),
		fixture.config.Logger,
	)
	require.NoError(t, err)

	/* START ALPHA CHANGE 3416 -- close inspection storage before offline rebuild opens it again -- */
	sourceOpen := true
	t.Cleanup(func() {
		if sourceOpen {
			require.NoError(t, source.Close())
		}
	})
	/* FINISH ALPHA CHANGE 3416 -- close inspection storage before offline rebuild opens it again -- */

	genesis, err := readTrieRebuildHeader(source, 0)
	require.NoError(t, err)
	canonicalHashes := map[uint64]types.Hash{0: genesis.Hash}

	for number := uint64(1); number <= fixture.head.Number; number++ {
		header, err := readTrieRebuildHeader(source, number)
		require.NoError(t, err)
		require.Equal(t, number, header.Number)
		require.Equal(t, canonicalHashes[number-1], header.ParentHash)

		body, err := source.ReadBody(header.Hash)
		require.NoError(t, err)
		require.NotNil(t, body)
		require.NotEmpty(t, body.Transactions)

		receipts, err := source.ReadReceipts(header.Hash)
		require.NoError(t, err)
		require.Len(t, receipts, len(body.Transactions))

		var cumulativeGas uint64
		for index, receipt := range receipts {
			require.NotNil(t, receipt)
			require.NotNil(t, receipt.Status)
			require.Equal(t, types.ReceiptSuccess, *receipt.Status)
			require.Equal(t, body.Transactions[index].Hash, receipt.TxHash)
			require.Equal(t, body.Transactions[index].Type, receipt.TransactionType)

			cumulativeGas += receipt.GasUsed
			require.Equal(t, cumulativeGas, receipt.CumulativeGasUsed)
		}

		require.Equal(t, cumulativeGas, header.GasUsed)
		require.Equal(t,
			buildroot.CalculateTransactionsRoot(body.Transactions, number),
			header.TxRoot,
		)
		require.Equal(t, buildroot.CalculateReceiptsRoot(receipts), header.ReceiptsRoot)
		require.Equal(t, types.CreateBloom(receipts), header.LogsBloom)

		ordinary := receipts[len(receipts)-1]
		require.Len(t, ordinary.Logs, 1)
		require.Equal(t, fixture.contract, ordinary.Logs[0].Address)
		require.Equal(t,
			[]types.Hash{types.StringToHash("0x01")},
			ordinary.Logs[0].Topics,
		)
		require.Equal(t, types.StringToHash("0x2a").Bytes(), ordinary.Logs[0].Data)

		canonicalHashes[number] = header.Hash
	}

	/* START ALPHA CHANGE 3416 -- release the inspection handle before rebuilding -- */
	// All required header hashes have been copied into canonicalHashes.
	// RebuildTrieDatabase opens this database through its own handle.
	closeErr := source.Close()
	sourceOpen = false
	require.NoError(t, closeErr)
	/* FINISH ALPHA CHANGE 3416 -- release the inspection handle before rebuilding -- */

	result, err := RebuildTrieDatabase(context.Background(), fixture.config)
	require.NoError(t, err)
	require.Equal(t, fixture.head.Hash, result.BlockHash)
	require.Equal(t, fixture.head.StateRoot, result.StateRoot)

	storage, err := itrie.NewMDBXStorageWithMapSize(
		result.TriePath, fixture.config.Logger, 1024*1024*1024,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	config, err := chain.ImportFromFile(fixture.config.ChainPath)
	require.NoError(t, err)
	executor := newTrieRebuildExecutor(config, storage, fixture.config.Logger)
	/* START ALPHA CHANGE 3810 -- adapt current-execution fixture to checked hash lookups -- */
	executor.GetHash = func(header *types.Header) state.GetHashByNumber {
		return func(number uint64) (types.Hash, error) {
			if number >= header.Number || header.Number-number > 256 {
				return types.ZeroHash, nil
			}

			hash, exists := canonicalHashes[number]
			require.True(t, exists, "missing canonical test block %d", number)

			return hash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt current-execution fixture to checked hash lookups -- */

	world := itrie.NewState(storage)
	consensusEngine := new(innbcbft.INNBCBFT)
	chainBackend := new(blockchain.Blockchain)
	chainBackend.SetConsensus(consensusEngine)

	hub := &jsonRPCHub{
		state:      world,
		Blockchain: chainBackend,
		Executor:   executor,
		Consensus:  consensusEngine,
	}

	snapshot, err := world.NewSnapshotAt(result.StateRoot)
	require.NoError(t, err)
	sender, err := snapshot.GetAccount(fixture.sender)
	require.NoError(t, err)
	require.NotNil(t, sender)
	require.Equal(t, uint64(2), sender.Nonce)

	return &currentExecutionContext{
		fixture:       fixture,
		storage:       storage,
		executor:      executor,
		hub:           hub,
		senderNonce:   sender.Nonce,
		senderBalance: new(big.Int).Set(sender.Balance),
	}
}

func (c *currentExecutionContext) snapshot(t *testing.T, root types.Hash) state.Snapshot {
	t.Helper()

	snapshot, err := itrie.NewState(c.storage).NewSnapshotAt(root)
	require.NoError(t, err)

	return snapshot
}

func (c *currentExecutionContext) account(
	t *testing.T,
	root types.Hash,
	address types.Address,
) *state.Account {
	t.Helper()

	account, err := c.snapshot(t, root).GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, account)

	return account
}

func (c *currentExecutionContext) word(
	t *testing.T,
	root types.Hash,
	address types.Address,
	slot types.Hash,
) types.Hash {
	t.Helper()

	snapshot := c.snapshot(t, root)
	account, err := snapshot.GetAccount(address)
	require.NoError(t, err)
	require.NotNil(t, account)
	value, err := snapshot.GetStorage(address, account.Root, slot)
	require.NoError(t, err)

	return value
}

func (c *currentExecutionContext) begin(t *testing.T) *state.Transition {
	t.Helper()

	transition, err := c.executor.BeginTxn(
		c.fixture.head.StateRoot, c.fixture.head, types.ZeroAddress,
	)
	require.NoError(t, err)

	return transition
}

func (c *currentExecutionContext) assertSender(
	t *testing.T,
	root types.Hash,
	gasUsed uint64,
	value *big.Int,
) {
	t.Helper()

	sender := c.account(t, root, c.fixture.sender)
	require.Equal(t, c.senderNonce+1, sender.Nonce)

	cost := new(big.Int).Mul(
		new(big.Int).SetUint64(gasUsed),
		new(big.Int).SetUint64(c.fixture.head.BaseFee),
	)
	cost.Add(cost, value)
	expected := new(big.Int).Sub(c.senderBalance, cost)
	require.Zero(t, expected.Cmp(sender.Balance))
}

func TestCurrentExecution_RebuiltStateAndRPC(t *testing.T) {
	// Fork handlers are process-global: do not run this test in parallel.
	c := newCurrentExecutionContext(t)
	root := c.fixture.head.StateRoot

	require.Equal(t, c.fixture.head.ParentHash,
		c.word(t, root, c.fixture.contract, types.ZeroHash))
	require.Equal(t, types.StringToHash("0x2a"),
		c.word(t, root, c.fixture.contract, types.StringToHash("0x01")))

	rpcAccount, err := c.hub.GetAccount(root, c.fixture.sender)
	require.NoError(t, err)
	require.Equal(t, c.senderNonce, rpcAccount.Nonce)
	require.Zero(t, c.senderBalance.Cmp(rpcAccount.Balance))

	for _, slot := range []types.Hash{types.ZeroHash, types.StringToHash("0x01")} {
		actual, err := c.hub.GetStorage(root, c.fixture.contract, slot)
		require.NoError(t, err)
		require.Equal(t, c.word(t, root, c.fixture.contract, slot).Bytes(), actual)
	}

	code, err := c.hub.GetCode(root, c.fixture.contract)
	require.NoError(t, err)
	require.Equal(t, c.fixture.code, code)

	to := c.fixture.contract
	rpcResult, err := c.hub.ApplyTxn(c.fixture.head, &types.Transaction{
		From:     c.fixture.sender,
		To:       &to,
		Nonce:    c.senderNonce,
		Gas:      500000,
		GasPrice: new(big.Int).SetUint64(c.fixture.head.BaseFee),
		Value:    new(big.Int),
	}, nil, true)
	require.NoError(t, err)
	require.NotNil(t, rpcResult)
	require.NoError(t, rpcResult.Err)
	require.Equal(t, types.StringToHash("0x2a").Bytes(), rpcResult.ReturnValue)

	transition := c.begin(t)
	direct := transition.Call2(
		c.fixture.sender, to, nil, new(big.Int), 500000,
	)
	require.NotNil(t, direct)
	require.NoError(t, direct.Err)
	require.NoError(t, transition.Error())
	require.Equal(t, rpcResult.ReturnValue, direct.ReturnValue)

	// Simulations must not change the committed checkpoint.
	sender := c.account(t, root, c.fixture.sender)
	require.Equal(t, c.senderNonce, sender.Nonce)
	require.Zero(t, c.senderBalance.Cmp(sender.Balance))
	require.Equal(t, types.StringToHash("0x2a"),
		c.word(t, root, c.fixture.contract, types.StringToHash("0x01")))
}

func TestCurrentExecution_CommitAndRollback(t *testing.T) {
	// Every branch begins at the same current checkpoint.
	c := newCurrentExecutionContext(t)
	baseRoot := c.fixture.head.StateRoot

	t.Run("nested revert preserves only outer effects", func(t *testing.T) {
		transition := c.begin(t)
		to := c.fixture.outerContract
		result, err := transition.Apply(&types.Transaction{
			From:     c.fixture.sender,
			To:       &to,
			Nonce:    c.senderNonce,
			Gas:      500000,
			GasPrice: new(big.Int).SetUint64(c.fixture.head.BaseFee),
			Value:    new(big.Int),
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NoError(t, result.Err)
		require.Equal(t, types.StringToHash("0x2c").Bytes(), result.ReturnValue)

		logs := transition.Txn().Logs()
		require.Len(t, logs, 1)
		require.Equal(t, to, logs[0].Address)
		require.Equal(t, []types.Hash{types.StringToHash("0x02")}, logs[0].Topics)
		require.Equal(t, types.StringToHash("0x2c").Bytes(), logs[0].Data)

		_, root, err := transition.Commit()
		require.NoError(t, err)
		require.NotEqual(t, baseRoot, root)

		require.Equal(t, types.StringToHash("0x01"),
			c.word(t, root, to, types.StringToHash("0x02")))
		require.Equal(t, types.StringToHash("0x2c"),
			c.word(t, root, to, types.StringToHash("0x03")))
		require.Equal(t, types.ZeroHash,
			c.word(t, root, c.fixture.revertContract, types.StringToHash("0x01")))
		c.assertSender(t, root, result.GasUsed, new(big.Int))

		require.Equal(t, types.ZeroHash,
			c.word(t, baseRoot, to, types.StringToHash("0x02")))
		require.Equal(t, types.ZeroHash,
			c.word(t, baseRoot, to, types.StringToHash("0x03")))
	})

	t.Run("ordinary revert rolls back writes but consumes nonce and gas", func(t *testing.T) {
		transition := c.begin(t)
		to := c.fixture.revertContract
		result, err := transition.Apply(&types.Transaction{
			From:     c.fixture.sender,
			To:       &to,
			Nonce:    c.senderNonce,
			Gas:      500000,
			GasPrice: new(big.Int).SetUint64(c.fixture.head.BaseFee),
			Value:    new(big.Int),
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.ErrorIs(t, result.Err, runtime.ErrExecutionReverted)
		require.Equal(t, types.StringToHash("0x2b").Bytes(), result.ReturnValue)
		require.Empty(t, transition.Txn().Logs())

		_, root, err := transition.Commit()
		require.NoError(t, err)
		require.Equal(t, types.ZeroHash,
			c.word(t, root, to, types.StringToHash("0x01")))
		c.assertSender(t, root, result.GasUsed, new(big.Int))
	})

	t.Run("native transfer persists its amount nonce and fee", func(t *testing.T) {
		recipient := types.StringToAddress(
			"0x7123456789abcdef0123456789abcdef01234567",
		)
		before, err := c.snapshot(t, baseRoot).GetAccount(recipient)
		require.NoError(t, err)
		require.Nil(t, before)

		transition := c.begin(t)
		result, err := transition.Apply(&types.Transaction{
			From:     c.fixture.sender,
			To:       &recipient,
			Nonce:    c.senderNonce,
			Gas:      21000,
			GasPrice: new(big.Int).SetUint64(c.fixture.head.BaseFee),
			Value:    big.NewInt(7),
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NoError(t, result.Err)
		require.Equal(t, uint64(21000), result.GasUsed)
		require.Empty(t, result.ReturnValue)
		require.Empty(t, transition.Txn().Logs())

		_, root, err := transition.Commit()
		require.NoError(t, err)

		account := c.account(t, root, recipient)
		require.Zero(t, account.Balance.Cmp(big.NewInt(7)))
		require.Zero(t, account.Nonce)
		require.Equal(t, types.EmptyCodeHash.Bytes(), account.CodeHash)
		c.assertSender(t, root, result.GasUsed, big.NewInt(7))

		unchanged, err := c.snapshot(t, baseRoot).GetAccount(recipient)
		require.NoError(t, err)
		require.Nil(t, unchanged)
	})

	// Branch publication must preserve the original sender state.
	sender := c.account(t, baseRoot, c.fixture.sender)
	require.Equal(t, c.senderNonce, sender.Nonce)
	require.Zero(t, c.senderBalance.Cmp(sender.Balance))
}

/* FINISH ALPHA CHANGE 3414 -- verify current execution behavior without frozen genesis commitments -- */
