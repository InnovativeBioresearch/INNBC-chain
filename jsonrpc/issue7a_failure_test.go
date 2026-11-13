/* START ALPHA CHANGE 2166 -- test RPC absence and gas-search error precedence -- */
package jsonrpc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type issue7AEndpointStore struct {
	ethStore

	header     *types.Header
	readErr    error
	nonceErr   error
	applyErr   error
	resultErr  error
	applyCalls int
	failAt     int
}

func (s *issue7AEndpointStore) Header() *types.Header {
	return s.header
}

func (s *issue7AEndpointStore) GetHeaderByNumber(number uint64) (*types.Header, bool) {
	return s.header, number == s.header.Number
}

func (s *issue7AEndpointStore) GetAccount(types.Hash, types.Address) (*Account, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}

	return &Account{Balance: big.NewInt(1_000_000_000)}, nil
}

func (s *issue7AEndpointStore) GetNonce(types.Address) (uint64, error) {
	return 0, s.nonceErr
}

func (s *issue7AEndpointStore) GetStorage(types.Hash, types.Address, types.Hash) ([]byte, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}

	return types.ZeroHash.Bytes(), nil
}

func (s *issue7AEndpointStore) GetCode(types.Hash, types.Address) ([]byte, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}

	return []byte{}, nil
}

func (s *issue7AEndpointStore) GetForksInTime(uint64) chain.ForksInTime {
	return chain.AllForksEnabled.At(1)
}

func (s *issue7AEndpointStore) GetBaseFee() uint64 {
	return 1
}

func (s *issue7AEndpointStore) ApplyTxn(
	*types.Header,
	*types.Transaction,
	types.StateOverride,
	bool,
) (*runtime.ExecutionResult, error) {
	s.applyCalls++

	if s.failAt == 0 || s.applyCalls == s.failAt {
		return &runtime.ExecutionResult{
			ReturnValue: []byte{0xde, 0xad},
			Err:         s.resultErr,
		}, s.applyErr
	}

	return &runtime.ExecutionResult{}, nil
}

func issue7AEndpointFixture() (*Eth, *issue7AEndpointStore) {
	store := &issue7AEndpointStore{
		header: &types.Header{Number: 1, GasLimit: 100_000, BaseFee: 1},
	}
	endpoint := &Eth{
		store:  store,
		logger: hclog.NewNullLogger(),
	}

	return endpoint, store
}

func issue7AEndpointArgs() *txnArgs {
	sender := types.StringToAddress("0x7401")
	target := types.StringToAddress("0x7402")

	return &txnArgs{
		From:     &sender,
		To:       &target,
		GasPrice: argBytesPtr([]byte{1}),
		Input:    argBytesPtr([]byte{1}),
	}
}

func TestIssue7A_DirectRPCRejectsLocalFailureBeforeAbsence(t *testing.T) {
	for _, operation := range []string{"balance", "storage", "code", "nonce", "pending nonce"} {
		t.Run(operation, func(t *testing.T) {
			endpoint, store := issue7AEndpointFixture()
			localErr := state.WrapLocalStateFailure("injected read", ErrStateNotFound)
			store.readErr = localErr
			store.nonceErr = localErr
			number := LatestBlockNumber
			filter := BlockNumberOrHash{BlockNumber: &number}
			address := types.StringToAddress("0x7401")
			var result interface{}
			var err error

			switch operation {
			case "balance":
				result, err = endpoint.GetBalance(address, filter)
			case "storage":
				result, err = endpoint.GetStorageAt(address, types.ZeroHash, filter)
			case "code":
				result, err = endpoint.GetCode(address, filter)
			case "nonce":
				result, err = endpoint.GetTransactionCount(address, filter)
			case "pending nonce":
				number = PendingBlockNumber
				result, err = endpoint.GetTransactionCount(address, filter)
			}

			require.Nil(t, result)
			require.ErrorIs(t, err, state.ErrLocalStateFailure)
			require.ErrorIs(t, err, ErrStateNotFound)
		})
	}
}

func TestIssue7A_DirectRPCStillAcceptsGenuineAbsence(t *testing.T) {
	endpoint, store := issue7AEndpointFixture()
	store.readErr = ErrStateNotFound
	number := LatestBlockNumber
	filter := BlockNumberOrHash{BlockNumber: &number}
	address := types.StringToAddress("0x74ff")

	balance, err := endpoint.GetBalance(address, filter)
	require.NoError(t, err)
	require.Equal(t, argUintPtr(0), balance)

	storage, err := endpoint.GetStorageAt(address, types.ZeroHash, filter)
	require.NoError(t, err)
	require.Equal(t, argBytesPtr(types.ZeroHash[:]), storage)

	code, err := endpoint.GetCode(address, filter)
	require.NoError(t, err)
	require.Equal(t, "0x", code)

	nonce, err := endpoint.GetTransactionCount(address, filter)
	require.NoError(t, err)
	require.Equal(t, argUintPtr(0), nonce)
}

func TestIssue7A_EstimateGasNeverRetriesLocalFailure(t *testing.T) {
	causes := []error{
		errors.New("injected database read"),
		state.ErrNotEnoughIntrinsicGas,
		runtime.ErrOutOfGas,
		runtime.ErrCodeStoreOutOfGas,
		runtime.ErrExecutionReverted,
	}

	for _, cause := range causes {
		for _, resultFailure := range []bool{false, true} {
			for _, finalProbe := range []bool{false, true} {
				name := cause.Error()
				if resultFailure {
					name += "/result"
				} else {
					name += "/apply"
				}
				if finalProbe {
					name += "/final"
				} else {
					name += "/search"
				}

				t.Run(name, func(t *testing.T) {
					endpoint, store := issue7AEndpointFixture()
					localErr := state.WrapLocalStateFailure("injected estimate", cause)
					if resultFailure {
						store.resultErr = localErr
					} else {
						store.applyErr = localErr
					}
					args := issue7AEndpointArgs()
					/* START ALPHA CHANGE 2988 -- allow the actual intrinsic cost in the final-probe fixture -- */
					if finalProbe {
						args.Gas = argUintPtr(state.TxGas + 16)
					}
					/* FINISH ALPHA CHANGE 2988 -- allow the actual intrinsic cost in the final-probe fixture -- */

					result, err := endpoint.EstimateGas(args, nil)
					require.Nil(t, result)
					require.ErrorIs(t, err, state.ErrLocalStateFailure)
					require.ErrorIs(t, err, cause)
					require.Equal(t, 1, store.applyCalls)
				})
			}
		}
	}
}

func TestIssue7A_EstimateGasStopsAfterEarlierSuccessfulProbe(t *testing.T) {
	endpoint, store := issue7AEndpointFixture()
	cause := errors.New("injected second probe failure")
	store.failAt = 2
	store.applyErr = state.WrapLocalStateFailure("second probe", cause)

	result, err := endpoint.EstimateGas(issue7AEndpointArgs(), nil)
	require.Nil(t, result)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.Equal(t, 2, store.applyCalls)
}

func TestIssue7A_CallDoesNotReturnDataFromLocalFailure(t *testing.T) {
	for _, resultFailure := range []bool{false, true} {
		endpoint, store := issue7AEndpointFixture()
		localErr := state.WrapLocalStateFailure("injected call", runtime.ErrExecutionReverted)
		if resultFailure {
			store.resultErr = localErr
		} else {
			store.applyErr = localErr
		}
		number := LatestBlockNumber

		result, err := endpoint.Call(
			issue7AEndpointArgs(),
			BlockNumberOrHash{BlockNumber: &number},
			nil,
		)
		require.Nil(t, result)
		require.ErrorIs(t, err, state.ErrLocalStateFailure)
		require.Equal(t, 1, store.applyCalls)
	}
}

/* FINISH ALPHA CHANGE 2166 -- test RPC absence and gas-search error precedence -- */
