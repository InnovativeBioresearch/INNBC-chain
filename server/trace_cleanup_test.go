/* START ALPHA CHANGE 3156 -- compare tracing transaction boundaries with normal block execution -- */
package server

import (
	"math/big"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage/memory"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/tracer/structtracer"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type traceCleanupStorage struct {
	itrie.Storage
	writes uint64
}

type traceCleanupBatch struct {
	itrie.Batch
	owner *traceCleanupStorage
}

func (s *traceCleanupStorage) Batch() itrie.Batch {
	return &traceCleanupBatch{Batch: s.Storage.Batch(), owner: s}
}

func (b *traceCleanupBatch) Write() error {
	if err := b.Batch.Write(); err != nil {
		return err
	}
	b.owner.writes++
	return nil
}

type traceCleanupFixture struct {
	hub         *jsonRPCHub
	storage     *traceCleanupStorage
	root        types.Hash
	block       *types.Block
	sender      types.Address
	victim      types.Address
	beneficiary types.Address
}

type traceCleanupStateView struct {
	Nonce       uint64
	Sender      *big.Int
	Fees        *big.Int
	Beneficiary *big.Int
	Slot        types.Hash
	Exists      bool
	Code        []byte
}

func traceCleanupView(
	transition *state.Transition,
	f *traceCleanupFixture,
) traceCleanupStateView {
	return traceCleanupStateView{
		Nonce:       transition.GetNonce(f.sender),
		Sender:      new(big.Int).Set(transition.GetBalance(f.sender)),
		Fees:        new(big.Int).Set(transition.GetBalance(types.ZeroAddress)),
		Beneficiary: new(big.Int).Set(transition.GetBalance(f.beneficiary)),
		Slot:        transition.GetStorage(f.victim, types.ZeroHash),
		Exists:      transition.AccountExists(f.victim),
		Code:        append([]byte(nil), transition.GetCode(f.victim)...),
	}
}

func newTraceCleanupFixture(
	t *testing.T,
	scenario string,
	txType types.TxType,
) *traceCleanupFixture {
	t.Helper()

	logger := hclog.NewNullLogger()
	raw := &traceCleanupStorage{Storage: itrie.NewMemoryStorage()}
	t.Cleanup(func() {
		require.NoError(t, raw.Close())
	})

	key, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	f := &traceCleanupFixture{
		storage:     raw,
		sender:      crypto.PubKeyToAddress(&key.PublicKey),
		victim:      types.StringToAddress("0x8402"),
		beneficiary: types.StringToAddress("0x8403"),
	}
	observer := types.StringToAddress("0x8404")

	// LOG0; load the supplied word; write slot zero; STOP.
	code := []byte{
		0x60, 0x00, 0x60, 0x00, 0xa0,
		0x60, 0x00, 0x35, 0x60, 0x00, 0x55, 0x00,
	}
	var observerCode []byte
	balance := big.NewInt(1)
	slots := map[types.Hash]types.Hash{
		types.ZeroHash: types.StringToHash("0x01"),
	}

	/* START ALPHA CHANGE 3157 -- use a tagged switch for trace fixture account scenarios -- */
	switch scenario {
	case "delete_then_access":
		// PUSH20 beneficiary; SELFDESTRUCT.
		code = append([]byte{0x73}, f.beneficiary.Bytes()...)
		code = append(code, 0xff)

		// Return EXTCODESIZE(victim) as one word.
		observerCode = append([]byte{0x73}, f.victim.Bytes()...)
		observerCode = append(observerCode,
			0x3b, 0x60, 0x00, 0x52,
			0x60, 0x20, 0x60, 0x00, 0xf3,
		)
	case "empty_account_then_access":
		code = nil
		balance = new(big.Int)
		slots = nil

		// Return EXTCODEHASH(victim) as one word.
		observerCode = append([]byte{0x73}, f.victim.Bytes()...)
		observerCode = append(observerCode,
			0x3f, 0x60, 0x00, 0x52,
			0x60, 0x20, 0x60, 0x00, 0xf3,
		)
	}
	/* FINISH ALPHA CHANGE 3157 -- use a tagged switch for trace fixture account scenarios -- */

	params := &chain.Params{ChainID: 1982, Forks: chain.AllForksEnabled}
	world := itrie.NewState(raw)
	executor := state.NewExecutor(params, world, logger)
	f.root, err = executor.WriteGenesis(
		map[types.Address]*chain.GenesisAccount{
			f.sender: {Balance: big.NewInt(1_000_000_000)},
			f.victim: {
				Balance: balance,
				Code:    code,
				Storage: slots,
			},
			f.beneficiary: {Balance: big.NewInt(1)},
			observer:      {Balance: big.NewInt(1), Code: observerCode},
		},
		types.ZeroHash,
	)
	require.NoError(t, err)

	backend := &issue7ARPCState{State: world, target: f.victim}
	executor = state.NewExecutor(params, backend, logger)
	/* START ALPHA CHANGE 3810 -- adapt trace cleanup fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt trace cleanup fixture to checked hash lookups -- */

	parent := &types.Header{
		Number:    0,
		StateRoot: f.root,
		GasLimit:  5_242_880,
		BaseFee:   1,
		Timestamp: 1,
	}
	parent.ComputeHash()

	db, err := memory.NewMemoryStorage(logger)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	writer := storage.NewBatchWriter(db)
	writer.PutHeader(parent)
	require.NoError(t, writer.WriteBatch())

	engine := &issue7ARPCConsensus{}
	chainBackend, err := blockchain.NewBlockchain(
		logger, db, &chain.Chain{Params: params}, engine, executor, nil,
	)
	require.NoError(t, err)

	f.hub = &jsonRPCHub{
		state:      backend,
		Executor:   executor,
		Blockchain: chainBackend,
		Consensus:  engine,
	}
	f.block = &types.Block{Header: &types.Header{
		Number:     1,
		ParentHash: parent.Hash,
		StateRoot:  f.root,
		GasLimit:   parent.GasLimit,
		BaseFee:    parent.BaseFee,
		Timestamp:  3,
	}}

	type step struct {
		to    types.Address
		value uint64
		gas   uint64
	}
	steps := []step{
		{to: f.victim, value: 0, gas: 100_000},
		{to: f.victim, value: 2, gas: 100_000},
	}
	/* START ALPHA CHANGE 3157 -- use a tagged switch for trace replay sequences -- */
	switch scenario {
	case "clear_then_failed_then_success":
		steps = []step{
			{to: f.victim, value: 0, gas: 100_000},
			{to: f.victim, value: 2, gas: 30_000},
			{to: f.victim, value: 2, gas: 100_000},
		}
	case "delete_then_access", "empty_account_then_access":
		steps[1].to = observer
	}
	/* FINISH ALPHA CHANGE 3157 -- use a tagged switch for trace replay sequences -- */

	signer := crypto.NewSigner(chain.AllForksEnabled.At(1), 1982)
	for index, current := range steps {
		tx := &types.Transaction{
			Type:     txType,
			Nonce:    uint64(index),
			To:       current.to.Ptr(),
			Gas:      current.gas,
			GasPrice: big.NewInt(2),
			Value:    new(big.Int),
		}
		if scenario == "clear_then_add" ||
			scenario == "clear_then_failed_then_success" {
			tx.Input = new(big.Int).SetUint64(current.value).
				FillBytes(make([]byte, 32))
		}
		if txType == types.DynamicFeeTx {
			tx.ChainID = big.NewInt(1982)
			tx.GasPrice = new(big.Int)
			tx.GasFeeCap = big.NewInt(10)
			tx.GasTipCap = big.NewInt(1)
		}

		signed, err := signer.SignTx(tx, key)
		require.NoError(t, err)
		// Apply receives recovered sender metadata in these replay fixtures.
		signed.From, err = signer.Sender(signed)
		require.NoError(t, err)
		require.Equal(t, f.sender, signed.From)
		signed.ComputeHash(1)
		f.block.Transactions = append(f.block.Transactions, signed)
	}

	return f
}

func TestTraceCleanup_BlockAndTransactionReplay(t *testing.T) {
	for _, scenario := range []string{
		"clear_then_add",
		"clear_then_failed_then_success",
		"delete_then_access",
		"empty_account_then_access",
	} {
		for _, txType := range []types.TxType{
			types.LegacyTx, types.DynamicFeeTx,
		} {
			format := "legacy"
			if txType == types.DynamicFeeTx {
				format = "dynamic"
			}
			t.Run(scenario+"/"+format, func(t *testing.T) {
				f := newTraceCleanupFixture(t, scenario, txType)
				before := f.storage.writes

				// Establish the reference through the normal Write path.
				reference, err := f.hub.BeginTxn(
					f.root, f.block.Header, types.ZeroAddress,
				)
				require.NoError(t, err)
				referenceTracer := structtracer.NewStructTracer(
					structtracer.Config{},
				)
				reference.SetTracer(referenceTracer)

				expected := make([]*structtracer.StructTraceResult,
					len(f.block.Transactions))
				views := make([]traceCleanupStateView, len(expected))

				for index, tx := range f.block.Transactions {
					referenceTracer.Clear()
					require.NoError(t, reference.Write(tx.Copy()))
					result, err := referenceTracer.GetResult()
					require.NoError(t, err)
					expected[index] = result.(*structtracer.StructTraceResult)
					views[index] = traceCleanupView(reference, f)

					failed := scenario == "clear_then_failed_then_success" &&
						index == 1
					require.Equal(t, failed, expected[index].Failed)
					require.Equal(t, reference.Receipts()[index].GasUsed,
						expected[index].Gas)
					require.Zero(t, reference.GetRefund())
					require.Empty(t, reference.Txn().Logs())
				}

				if scenario == "delete_then_access" ||
					scenario == "empty_account_then_access" {
					require.False(t, views[len(views)-1].Exists)
					require.Equal(t, strings.Repeat("0", 64),
						expected[len(expected)-1].ReturnValue)
				} else {
					require.Equal(t, types.StringToHash("0x02"),
						views[len(views)-1].Slot)
					require.NotEmpty(t, reference.Receipts()[0].Logs)
				}

				require.Equal(t, before, f.storage.writes)

				t.Run("block", func(t *testing.T) {
					var replay *state.Transition
					f.hub.Executor.PostHook = func(current *state.Transition) {
						replay = current
					}
					defer func() { f.hub.Executor.PostHook = nil }()

					results, err := f.hub.TraceBlock(
						f.block,
						structtracer.NewStructTracer(structtracer.Config{}),
					)
					require.NoError(t, err)
					require.Len(t, results, len(expected))
					for index := range expected {
						require.Equal(t, expected[index], results[index])
					}
					require.NotNil(t, replay)
					require.Equal(t, views[len(views)-1],
						traceCleanupView(replay, f))
					require.Zero(t, replay.GetRefund())
					require.Empty(t, replay.Txn().Logs())
					require.Empty(t, replay.Receipts())
					require.NoError(t, replay.Error())
					require.Equal(t, before, f.storage.writes)
				})

				// Exercise an early target, a failed target where present,
				// and later targets requiring successful/failed prefix replay.
				for index, tx := range f.block.Transactions {
					t.Run("transaction/"+tx.Hash.String(), func(t *testing.T) {
						var replay *state.Transition
						f.hub.Executor.PostHook = func(current *state.Transition) {
							replay = current
						}
						defer func() { f.hub.Executor.PostHook = nil }()

						result, err := f.hub.TraceTxn(
							f.block, tx.Hash,
							structtracer.NewStructTracer(structtracer.Config{}),
						)
						require.NoError(t, err)
						require.Equal(t, expected[index], result)
						require.NotNil(t, replay)
						require.Equal(t, views[index], traceCleanupView(replay, f))
						require.Zero(t, replay.GetRefund())
						require.Empty(t, replay.Txn().Logs())
						require.Empty(t, replay.Receipts())
						require.NoError(t, replay.Error())
						require.Equal(t, before, f.storage.writes)
					})
				}

				// Replay must leave the persisted parent state untouched.
				parent, err := itrie.NewState(f.storage).NewSnapshotAt(f.root)
				require.NoError(t, err)
				account, err := parent.GetAccount(f.sender)
				require.NoError(t, err)
				require.NotNil(t, account)
				require.Zero(t, account.Nonce)
				require.Zero(t, account.Balance.Cmp(big.NewInt(1_000_000_000)))
				require.Equal(t, before, f.storage.writes)
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3156 -- compare tracing transaction boundaries with normal block execution -- */
