/* START ALPHA CHANGE 2992 -- test selected-state estimation nonces and head advancement -- */
package jsonrpc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func estimateNonceExecutionStates(
	t *testing.T,
	code []byte,
) (*Eth, *estimateTransferStore, *txnArgs, *state.Executor) {
	t.Helper()

	endpoint, store, args, executor := estimateTransferExecutionFixture(
		t, code, false,
	)

	// Preserve the fixture's balances and code while constructing two
	// actual state roots with different sender nonces.
	historicalRoot, err := executor.WriteGenesis(
		map[types.Address]*chain.GenesisAccount{
			store.sender: {Nonce: 3},
		},
		store.header.StateRoot,
	)
	require.NoError(t, err)

	latestRoot, err := executor.WriteGenesis(
		map[types.Address]*chain.GenesisAccount{
			store.sender: {Nonce: 5},
		},
		historicalRoot,
	)
	require.NoError(t, err)
	require.NotEqual(t, historicalRoot, latestRoot)

	historical := store.header.Copy()
	historical.Number = 1
	historical.StateRoot = historicalRoot

	latest := store.header.Copy()
	latest.Number = 2
	latest.StateRoot = latestRoot

	store.historical = historical
	store.header = latest

	return endpoint, store, args, executor
}

func TestEstimateNonce_SelectedState(t *testing.T) {
	tests := []struct {
		name          string
		historical    bool
		pending       bool
		suppliedNonce bool
		code          []byte
	}{
		{
			name:       "historical_ordinary_transfer",
			historical: true,
		},
		{
			name:          "historical_contract_overrides_supplied_nonce",
			historical:    true,
			suppliedNonce: true,
			// PUSH1 1; PUSH1 0; SSTORE; STOP.
			code: []byte{0x60, 0x01, 0x60, 0x00, 0x55, 0x00},
		},
		{
			name:          "latest_overrides_supplied_nonce",
			suppliedNonce: true,
		},
		{
			name:    "pending_alias_uses_selected_header_state",
			pending: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, store, args, executor := estimateNonceExecutionStates(
				t, tt.code,
			)

			selected := store.header
			wantNonce := uint64(5)
			var number *BlockNumber
			if tt.historical {
				selected = store.historical
				wantNonce = 3
				block := BlockNumber(selected.Number)
				number = &block
			} else if tt.pending {
				block := PendingBlockNumber
				number = &block
			}

			if tt.suppliedNonce {
				args.Nonce = argUintPtr(99)
			}
			requestBefore := *args

			execute := store.applyFn
			store.applyFn = func(
				header *types.Header,
				tx *types.Transaction,
			) (*runtime.ExecutionResult, error) {
				require.Same(t, selected, header)
				require.Equal(t, wantNonce, tx.Nonce)

				return execute(header, tx)
			}

			result, err := endpoint.EstimateGas(args, number)

			require.NoError(t, err)
			require.IsType(t, argUint64(0), result)
			if len(tt.code) == 0 {
				require.Equal(t, argUint64(state.TxGas), result)
				require.Len(t, store.probes, 1)
			} else {
				require.Greater(t, uint64(result.(argUint64)), state.TxGas)
				require.Greater(t, len(store.probes), 1)
			}

			// One selected-state read supplies both nonce and balance.
			require.Equal(t, []types.Hash{selected.StateRoot}, store.readRoots)
			require.Equal(t, requestBefore, *args)

			// No simulation publishes a nonce increment at either root.
			for _, expected := range []struct {
				header *types.Header
				nonce  uint64
			}{
				{store.historical, 3},
				{store.header, 5},
			} {
				snapshot, err := executor.StateAt(expected.header.StateRoot)
				require.NoError(t, err)
				txn := state.NewTxn(snapshot)
				require.Equal(t, expected.nonce, txn.GetNonce(store.sender))
				require.NoError(t, txn.Error())
			}
		})
	}
}

type estimateNonceMovingHeadStore struct {
	*estimateTransferStore

	selected    *types.Header
	advanced    *types.Header
	headerCalls int
}

func (s *estimateNonceMovingHeadStore) Header() *types.Header {
	s.headerCalls++
	if s.headerCalls == 1 {
		// The selected header is returned just as the current head advances.
		s.estimateTransferStore.header = s.advanced

		return s.selected
	}

	return s.advanced
}

func TestEstimateNonce_HeadAdvanceKeepsCapturedState(t *testing.T) {
	_, store, args, _ := estimateNonceExecutionStates(
		t,
		[]byte{0x60, 0x01, 0x60, 0x00, 0x55, 0x00},
	)
	moving := &estimateNonceMovingHeadStore{
		estimateTransferStore: store,
		selected:              store.historical,
		advanced:              store.header,
	}
	endpoint := newTestEthEndpoint(moving)

	execute := store.applyFn
	store.applyFn = func(
		header *types.Header,
		tx *types.Transaction,
	) (*runtime.ExecutionResult, error) {
		require.Same(t, moving.selected, header)
		require.Equal(t, uint64(3), tx.Nonce)

		return execute(header, tx)
	}

	result, err := endpoint.EstimateGas(args, nil)

	require.NoError(t, err)
	require.IsType(t, argUint64(0), result)
	require.Greater(t, uint64(result.(argUint64)), state.TxGas)
	require.Greater(t, len(store.probes), 1)
	require.Equal(t, 1, moving.headerCalls)
	require.Equal(t, []types.Hash{moving.selected.StateRoot}, store.readRoots)
	require.Same(t, moving.advanced, store.header)
	require.Nil(t, args.Nonce)
}

func TestEstimateNonce_GenuineAbsenceUsesZeroWithoutLatestFallback(t *testing.T) {
	endpoint, store, args := estimateTransferFixture(t)
	historical := store.header.Copy()
	historical.Number = 1
	historical.StateRoot = types.StringToHash("0x01")
	store.historical = historical

	args.Nonce = argUintPtr(99)
	args.Value = argBytesPtr([]byte{0})
	args.GasPrice = argBytesPtr([]byte{0})
	args.Gas = argUintPtr(state.TxGas)

	store.accountFn = func(root types.Hash, address types.Address) (*Account, error) {
		require.Equal(t, historical.StateRoot, root)
		require.Equal(t, store.sender, address)

		return nil, ErrStateNotFound
	}
	store.applyFn = func(
		header *types.Header,
		tx *types.Transaction,
	) (*runtime.ExecutionResult, error) {
		require.Same(t, historical, header)
		require.Equal(t, uint64(0), tx.Nonce)

		return &runtime.ExecutionResult{}, nil
	}

	number := BlockNumber(historical.Number)
	result, err := endpoint.EstimateGas(args, &number)

	require.NoError(t, err)
	require.Equal(t, argUint64(state.TxGas), result)
	require.Equal(t, []types.Hash{historical.StateRoot}, store.readRoots)
	require.Equal(t, []uint64{state.TxGas}, store.probes)
	require.Equal(t, argUint64(99), *args.Nonce)
}

func TestEstimateNonce_ExplicitZeroSenderUsesAccountNonce(t *testing.T) {
	endpoint, store, args := estimateTransferFixture(t)
	zero := types.ZeroAddress
	args.From = &zero
	args.Nonce = argUintPtr(99)

	store.accountFn = func(root types.Hash, address types.Address) (*Account, error) {
		require.Equal(t, store.header.StateRoot, root)
		require.Equal(t, types.ZeroAddress, address)

		return &Account{
			Nonce:   7,
			Balance: big.NewInt(1_000_000_000),
		}, nil
	}
	store.applyFn = func(
		header *types.Header,
		tx *types.Transaction,
	) (*runtime.ExecutionResult, error) {
		require.Equal(t, types.ZeroAddress, tx.From)
		require.Equal(t, uint64(7), tx.Nonce)

		return &runtime.ExecutionResult{}, nil
	}

	result, err := endpoint.EstimateGas(args, nil)

	require.NoError(t, err)
	require.Equal(t, argUint64(state.TxGas), result)
	require.Equal(t, []types.Hash{store.header.StateRoot}, store.readRoots)
	require.Equal(t, []uint64{state.TxGas}, store.probes)
	require.Equal(t, argUint64(99), *args.Nonce)
}

func TestEstimateNonce_AccountFailuresDoNotExecute(t *testing.T) {
	readErr := errors.New("injected account read failure")
	localErr := state.WrapLocalStateFailure("selected account", ErrStateNotFound)

	tests := []struct {
		name    string
		account *Account
		readErr error
		wantErr error
	}{
		{
			name:    "read_failure",
			readErr: readErr,
			wantErr: readErr,
		},
		{
			name:    "local_failure_is_not_absence",
			readErr: localErr,
			wantErr: state.ErrLocalStateFailure,
		},
		{
			name:    "nil_account_without_error",
			wantErr: state.ErrMalformedAccount,
		},
		{
			name:    "nil_balance",
			account: &Account{Nonce: 3},
			wantErr: state.ErrMalformedAccount,
		},
		{
			name:    "negative_balance",
			account: &Account{Nonce: 3, Balance: big.NewInt(-1)},
			wantErr: state.ErrMalformedAccount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, store, args := estimateTransferFixture(t)
			requestBefore := *args
			store.accountFn = func(root types.Hash, address types.Address) (*Account, error) {
				require.Equal(t, store.header.StateRoot, root)

				return tt.account, tt.readErr
			}

			result, err := endpoint.EstimateGas(args, nil)

			require.Nil(t, result)
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantErr == state.ErrMalformedAccount {
				require.ErrorIs(t, err, state.ErrLocalStateFailure)
			}
			if tt.readErr == localErr {
				require.ErrorIs(t, err, ErrStateNotFound)
			}
			require.Empty(t, store.probes)
			require.Equal(t, []types.Hash{store.header.StateRoot}, store.readRoots)
			require.Equal(t, requestBefore, *args)
		})
	}
}

/* FINISH ALPHA CHANGE 2992 -- test selected-state estimation nonces and head advancement -- */
