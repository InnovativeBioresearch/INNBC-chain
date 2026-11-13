/* START ALPHA CHANGE 2166 -- test RPC state failures and simulation result rejection -- */
package server

import (
	"errors"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/jsonrpc"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/tracer"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/tracer/structtracer"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type issue7ARPCState struct {
	state.State

	target      types.Address
	openErr     error
	accountErr  error
	storageErr  error
	codeErr     error
	missingCode bool
}

type issue7ARPCSnapshot struct {
	state.Snapshot
	owner *issue7ARPCState
}

func (s *issue7ARPCState) NewSnapshotAt(root types.Hash) (state.Snapshot, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}

	snapshot, err := s.State.NewSnapshotAt(root)
	if err != nil {
		return nil, err
	}

	return &issue7ARPCSnapshot{Snapshot: snapshot, owner: s}, nil
}

func (s *issue7ARPCState) GetCode(hash types.Hash) ([]byte, bool, error) {
	if hash != types.EmptyCodeHash {
		if s.codeErr != nil {
			return nil, false, s.codeErr
		}
		if s.missingCode {
			return nil, false, nil
		}
	}

	return s.State.GetCode(hash)
}

func (s *issue7ARPCSnapshot) GetAccount(address types.Address) (*state.Account, error) {
	if address == s.owner.target && s.owner.accountErr != nil {
		return nil, s.owner.accountErr
	}

	return s.Snapshot.GetAccount(address)
}

func (s *issue7ARPCSnapshot) GetStorage(
	address types.Address,
	root types.Hash,
	slot types.Hash,
) (types.Hash, error) {
	if address == s.owner.target && s.owner.storageErr != nil {
		return types.ZeroHash, s.owner.storageErr
	}

	return s.Snapshot.GetStorage(address, root, slot)
}

func (s *issue7ARPCSnapshot) GetCode(hash types.Hash) ([]byte, bool, error) {
	return s.owner.GetCode(hash)
}

type issue7ARPCConsensus struct {
	consensus.Consensus
}

func (*issue7ARPCConsensus) GetBlockCreator(*types.Header) (types.Address, error) {
	return types.ZeroAddress, nil
}

type issue7ARPCTracer struct {
	tracer.Tracer
	results int
}

func (t *issue7ARPCTracer) GetResult() (interface{}, error) {
	t.results++

	return t.Tracer.GetResult()
}

func issue7ARPCFixture(
	t *testing.T,
) (*jsonRPCHub, *issue7ARPCState, *types.Header, *types.Transaction) {
	t.Helper()

	sender := types.StringToAddress("0x7301")
	target := types.StringToAddress("0x7302")
	params := &chain.Params{
		ChainID: 100,
		Forks:   chain.AllForksEnabled,
	}

	world := itrie.NewState(itrie.NewMemoryStorage())
	executor := state.NewExecutor(params, world, hclog.NewNullLogger())
	root, err := executor.WriteGenesis(
		map[types.Address]*chain.GenesisAccount{
			sender: {
				Balance: big.NewInt(1_000_000_000),
			},
			target: {
				Balance: big.NewInt(0),
				Code:    []byte{0x60, 0x00, 0x54, 0x50, 0x00},
				Storage: map[types.Hash]types.Hash{
					types.ZeroHash: types.StringToHash("0x2a"),
				},
			},
		},
		types.ZeroHash,
	)
	require.NoError(t, err)

	failing := &issue7ARPCState{State: world, target: target}
	executor = state.NewExecutor(params, failing, hclog.NewNullLogger())
	/* START ALPHA CHANGE 3810 -- adapt RPC failure fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt RPC failure fixture to checked hash lookups -- */

	engine := &issue7ARPCConsensus{}
	chainBackend := new(blockchain.Blockchain)
	chainBackend.SetConsensus(engine)

	hub := &jsonRPCHub{
		state:      failing,
		Executor:   executor,
		Blockchain: chainBackend,
		Consensus:  engine,
	}
	header := &types.Header{
		Number:    1,
		StateRoot: root,
		GasLimit:  1_000_000,
		BaseFee:   1,
	}
	tx := &types.Transaction{
		From:     sender,
		To:       target.Ptr(),
		Gas:      100_000,
		GasPrice: big.NewInt(1),
		Value:    new(big.Int),
	}

	return hub, failing, header, tx
}

func TestIssue7A_RPCDirectReadFailures(t *testing.T) {
	for _, operation := range []string{
		"snapshot",
		"account",
		"storage",
		"code",
		"missing code",
		"nonce",
		"balance",
	} {
		t.Run(operation, func(t *testing.T) {
			hub, backend, header, _ := issue7ARPCFixture(t)
			cause := errors.New("injected RPC state failure")
			poolHub := &txpoolHub{state: backend}
			var err error

			switch operation {
			case "snapshot":
				backend.openErr = cause
				_, err = hub.GetAccount(header.StateRoot, backend.target)
			case "account":
				backend.accountErr = cause
				_, err = hub.GetAccount(header.StateRoot, backend.target)
			case "storage":
				backend.storageErr = cause
				var data []byte
				data, err = hub.GetStorage(header.StateRoot, backend.target, types.ZeroHash)
				require.Nil(t, data)
			case "code":
				backend.codeErr = cause
				var data []byte
				data, err = hub.GetCode(header.StateRoot, backend.target)
				require.Nil(t, data)
			case "missing code":
				backend.missingCode = true
				cause = state.ErrMissingCode
				var data []byte
				data, err = hub.GetCode(header.StateRoot, backend.target)
				require.Nil(t, data)
			case "nonce":
				backend.accountErr = cause
				_, err = poolHub.GetNonce(header.StateRoot, backend.target)
			case "balance":
				backend.accountErr = cause
				_, err = poolHub.GetBalance(header.StateRoot, backend.target)
			}

			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, state.ErrLocalStateFailure)
		})
	}
}

func TestIssue7A_RPCHealthyAbsenceAndEmptyCode(t *testing.T) {
	hub, backend, header, tx := issue7ARPCFixture(t)
	poolHub := &txpoolHub{state: backend}
	missing := types.StringToAddress("0x73ff")

	_, err := hub.GetAccount(header.StateRoot, missing)
	require.ErrorIs(t, err, jsonrpc.ErrStateNotFound)
	require.NotErrorIs(t, err, state.ErrLocalStateFailure)

	nonce, err := poolHub.GetNonce(header.StateRoot, missing)
	require.NoError(t, err)
	require.Zero(t, nonce)

	balance, err := poolHub.GetBalance(header.StateRoot, missing)
	require.NoError(t, err)
	require.Zero(t, balance.Sign())

	code, err := hub.GetCode(header.StateRoot, tx.From)
	require.NoError(t, err)
	require.Empty(t, code)

	slot, err := hub.GetStorage(
		header.StateRoot,
		backend.target,
		types.StringToHash("0x9999"),
	)
	require.NoError(t, err)
	require.Equal(t, types.ZeroHash.Bytes(), slot)
}

func TestIssue7A_RPCBackendFailureCannotBecomeAbsence(t *testing.T) {
	hub, backend, header, _ := issue7ARPCFixture(t)
	backend.accountErr = jsonrpc.ErrStateNotFound

	_, err := hub.GetAccount(header.StateRoot, backend.target)
	require.ErrorIs(t, err, jsonrpc.ErrStateNotFound)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)

	poolHub := &txpoolHub{state: backend}
	_, err = poolHub.GetNonce(header.StateRoot, backend.target)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)

	_, err = poolHub.GetBalance(header.StateRoot, backend.target)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
}

func TestIssue7A_RPCSimulationAndTraceRejectStorageFailure(t *testing.T) {
	for _, tracing := range []bool{false, true} {
		t.Run(map[bool]string{false: "call", true: "trace"}[tracing], func(t *testing.T) {
			hub, backend, header, tx := issue7ARPCFixture(t)
			cause := errors.New("injected simulated storage failure")
			backend.storageErr = cause

			if tracing {
				spy := &issue7ARPCTracer{
					Tracer: structtracer.NewStructTracer(structtracer.Config{}),
				}
				result, err := hub.TraceCall(tx, header, spy)
				require.Nil(t, result)
				require.ErrorIs(t, err, cause)
				require.ErrorIs(t, err, state.ErrLocalStateFailure)
				require.Zero(t, spy.results)

				return
			}

			result, err := hub.ApplyTxn(header, tx, nil, true)
			require.Nil(t, result)
			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, state.ErrLocalStateFailure)
		})
	}
}

func TestIssue7A_RPCSimulationPreservesOrdinaryRevert(t *testing.T) {
	hub, backend, header, tx := issue7ARPCFixture(t)
	override := types.StateOverride{
		backend.target: types.OverrideAccount{
			Code: []byte{0x60, 0x00, 0x60, 0x00, 0xfd},
		},
	}

	result, err := hub.ApplyTxn(header, tx, override, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.ErrorIs(t, result.Err, runtime.ErrExecutionReverted)
	require.NotErrorIs(t, result.Err, state.ErrLocalStateFailure)
}

/* FINISH ALPHA CHANGE 2166 -- test RPC state failures and simulation result rejection -- */
