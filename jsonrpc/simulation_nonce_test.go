/* START ALPHA CHANGE 2997 -- test historical call and trace-call nonce ownership -- */
package jsonrpc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/tracer"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/tracer/structtracer"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type simulationNonceStore struct {
	*estimateTransferStore

	executor *state.Executor

	headerCalls int
	calls       int
	lastHeader  *types.Header
	lastTx      *types.Transaction

	advanceFrom *types.Header
	advanceTo   *types.Header
}

func newSimulationNonceFixture(
	t *testing.T,
) (*simulationNonceStore, *txnArgs) {
	t.Helper()

	_, base, args, executor := estimateNonceExecutionStates(
		t,
		[]byte{0x60, 0x01, 0x60, 0x00, 0x55, 0x00},
	)

	base.historical.ComputeHash()
	base.header.ComputeHash()

	args.Gas = argUintPtr(100_000)
	args.Nonce = argUintPtr(99)

	return &simulationNonceStore{
		estimateTransferStore: base,
		executor:              executor,
	}, args
}

func (s *simulationNonceStore) Header() *types.Header {
	s.headerCalls++

	if s.advanceTo != nil && s.headerCalls == 1 {
		s.header = s.advanceTo
		return s.advanceFrom
	}

	return s.header
}

func (s *simulationNonceStore) GetBlockByHash(
	hash types.Hash,
	full bool,
) (*types.Block, bool) {
	require.False(s.t, full)

	for _, header := range []*types.Header{s.historical, s.header} {
		if header != nil && header.Hash == hash {
			return &types.Block{Header: header}, true
		}
	}

	return nil, false
}

func (s *simulationNonceStore) execute(
	tx *types.Transaction,
	header *types.Header,
	override types.StateOverride,
	nonPayable bool,
	trace tracer.Tracer,
) (*runtime.ExecutionResult, error) {
	s.calls++
	s.lastHeader = header
	s.lastTx = tx.Copy()

	transition, err := s.executor.BeginTxn(
		header.StateRoot,
		header,
		types.ZeroAddress,
	)
	if err != nil {
		return nil, err
	}

	if override != nil {
		if err := transition.WithStateOverride(override); err != nil {
			return nil, err
		}
	}

	transition.SetNonPayable(nonPayable)
	if trace != nil {
		transition.SetTracer(trace)
	}

	result, err := transition.Apply(tx)
	if stateErr := transition.Error(); stateErr != nil {
		return nil, stateErr
	}

	// Match production simulation: never commit this transition.
	return result, err
}

func (s *simulationNonceStore) ApplyTxn(
	header *types.Header,
	tx *types.Transaction,
	override types.StateOverride,
	nonPayable bool,
) (*runtime.ExecutionResult, error) {
	require.True(s.t, nonPayable)

	return s.execute(tx, header, override, nonPayable, nil)
}

func (s *simulationNonceStore) TraceCall(
	tx *types.Transaction,
	header *types.Header,
	trace tracer.Tracer,
) (interface{}, error) {
	// Production TraceCall does not enable nonpayable execution.
	_, err := s.execute(tx, header, nil, false, trace)
	if err != nil {
		return nil, err
	}

	return trace.GetResult()
}

func (s *simulationNonceStore) TraceBlock(
	*types.Block,
	tracer.Tracer,
) ([]interface{}, error) {
	return nil, errors.New("unexpected TraceBlock")
}

func (s *simulationNonceStore) TraceTxn(
	*types.Block,
	types.Hash,
	tracer.Tracer,
) (interface{}, error) {
	return nil, errors.New("unexpected TraceTxn")
}

func invokeSimulationNonce(
	store *simulationNonceStore,
	args *txnArgs,
	filter BlockNumberOrHash,
	trace bool,
) (interface{}, error) {
	if trace {
		return NewDebug(store, 1).TraceCall(args, filter, &TraceConfig{})
	}

	return newTestEthEndpoint(store).Call(args, filter, nil)
}

func requireSimulationNonceStateUnchanged(
	t *testing.T,
	store *simulationNonceStore,
) {
	t.Helper()

	for _, expected := range []struct {
		header *types.Header
		nonce  uint64
	}{
		{store.historical, 3},
		{store.header, 5},
	} {
		snapshot, err := store.executor.StateAt(expected.header.StateRoot)
		require.NoError(t, err)

		txn := state.NewTxn(snapshot)
		require.Equal(t, expected.nonce, txn.GetNonce(store.sender))
		require.Zero(t, txn.GetBalance(store.sender).Cmp(
			big.NewInt(1_000_000_000),
		))
		require.Equal(t, types.ZeroHash, txn.GetState(
			store.target,
			types.ZeroHash,
		))
		require.NoError(t, txn.Error())
	}
}

func TestSimulationNonce_SelectedState(t *testing.T) {
	for _, trace := range []bool{false, true} {
		endpoint := "call"
		if trace {
			endpoint = "trace"
		}

		for _, selection := range []string{
			"historical_number",
			"historical_hash",
			"latest",
			"pending",
		} {
			t.Run(endpoint+"/"+selection, func(t *testing.T) {
				store, args := newSimulationNonceFixture(t)
				selected := store.header
				wantNonce := uint64(5)
				filter := BlockNumberOrHash{}

				switch selection {
				case "historical_number":
					selected = store.historical
					wantNonce = 3
					number := BlockNumber(selected.Number)
					filter.BlockNumber = &number
				case "historical_hash":
					selected = store.historical
					wantNonce = 3
					hash := selected.Hash
					filter.BlockHash = &hash
				case "pending":
					number := PendingBlockNumber
					filter.BlockNumber = &number
				}

				before := *args
				result, err := invokeSimulationNonce(
					store, args, filter, trace,
				)

				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 1, store.calls)
				require.Same(t, selected, store.lastHeader)
				require.Equal(t, wantNonce, store.lastTx.Nonce)
				require.Equal(t, []types.Hash{
					selected.StateRoot,
				}, store.readRoots)
				require.Equal(t, before, *args)
				require.Equal(t, argUint64(99), *args.Nonce)

				if selection == "historical_number" ||
					selection == "historical_hash" {
					require.Zero(t, store.headerCalls)
				} else {
					require.Equal(t, 1, store.headerCalls)
				}

				if trace {
					require.IsType(t,
						(*structtracer.StructTraceResult)(nil),
						result,
					)
					traceResult := result.(*structtracer.StructTraceResult)
					require.False(t, traceResult.Failed)
					require.Greater(t, traceResult.Gas, state.TxGas)
					require.NotEmpty(t, traceResult.StructLogs)
				}

				requireSimulationNonceStateUnchanged(t, store)
			})
		}
	}
}

func TestSimulationNonce_HeadAdvance(t *testing.T) {
	for _, trace := range []bool{false, true} {
		name := "call"
		if trace {
			name = "trace"
		}

		t.Run(name, func(t *testing.T) {
			store, args := newSimulationNonceFixture(t)
			selected := store.historical
			advanced := store.header
			store.advanceFrom = selected
			store.advanceTo = advanced

			result, err := invokeSimulationNonce(
				store, args, BlockNumberOrHash{}, trace,
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, store.headerCalls)
			require.Same(t, advanced, store.header)
			require.Same(t, selected, store.lastHeader)
			require.Equal(t, uint64(3), store.lastTx.Nonce)
			require.Equal(t, []types.Hash{
				selected.StateRoot,
			}, store.readRoots)
			require.Equal(t, argUint64(99), *args.Nonce)
			requireSimulationNonceStateUnchanged(t, store)
		})
	}
}

func TestSimulationNonce_CallOverrides(t *testing.T) {
	tests := []struct {
		name       string
		omitSender bool
		zeroSender bool
		override   bool
		unrelated  bool
		nonce      uint64
		wantNonce  uint64
		wantReads  int
	}{
		{
			name:      "supplied_sender_selected_nonce",
			wantNonce: 3,
			wantReads: 1,
		},
		{
			name:      "supplied_sender_override",
			override:  true,
			nonce:     9,
			wantNonce: 9,
		},
		{
			name:     "supplied_sender_explicit_zero_override",
			override: true,
		},
		{
			name:       "omitted_sender_default_zero",
			omitSender: true,
		},
		{
			name:       "omitted_sender_zero_address_override",
			omitSender: true,
			override:   true,
			nonce:      9,
			wantNonce:  9,
		},
		{
			name:       "omitted_sender_explicit_zero_override",
			omitSender: true,
			override:   true,
		},
		{
			name:      "unrelated_override",
			override:  true,
			unrelated: true,
			nonce:     11,
			wantNonce: 3,
			wantReads: 1,
		},
		{
			name:       "supplied_zero_sender_selected_nonce",
			zeroSender: true,
			wantNonce:  7,
			wantReads:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, args := newSimulationNonceFixture(t)
			args.Value = argBytesPtr([]byte{0})
			selected := store.historical
			sender := store.sender

			if tt.omitSender {
				args.From = nil
				sender = types.ZeroAddress
			}
			if tt.zeroSender {
				sender = types.ZeroAddress
				args.From = &sender

				root, err := store.executor.WriteGenesis(
					map[types.Address]*chain.GenesisAccount{
						sender: {
							Nonce:   7,
							Balance: big.NewInt(1_000_000_000),
						},
					},
					selected.StateRoot,
				)
				require.NoError(t, err)
				selected.StateRoot = root
				selected.ComputeHash()
			}

			var overrides *stateOverride
			var overrideAddress types.Address
			if tt.override {
				overrideAddress = sender
				if tt.unrelated {
					overrideAddress = types.StringToAddress("0x9091")
				}

				values := stateOverride{
					overrideAddress: {
						Nonce: argUintPtr(tt.nonce),
					},
				}
				overrides = &values
			}

			before := *args
			number := BlockNumber(selected.Number)
			result, err := newTestEthEndpoint(store).Call(
				args,
				BlockNumberOrHash{BlockNumber: &number},
				overrides,
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, store.calls)
			require.Equal(t, sender, store.lastTx.From)
			require.Equal(t, tt.wantNonce, store.lastTx.Nonce)
			require.Equal(t, before, *args)
			require.Equal(t, argUint64(99), *args.Nonce)
			require.Len(t, store.readRoots, tt.wantReads)
			require.Zero(t, store.headerCalls)

			if tt.wantReads != 0 {
				require.Equal(t, selected.StateRoot, store.readRoots[0])
			}
			if overrides != nil {
				require.Len(t, *overrides, 1)
				account := (*overrides)[overrideAddress]
				require.NotNil(t, account.Nonce)
				require.Equal(t, argUint64(tt.nonce), *account.Nonce)
			}

			requireSimulationNonceStateUnchanged(t, store)

			if tt.zeroSender {
				snapshot, err := store.executor.StateAt(selected.StateRoot)
				require.NoError(t, err)
				txn := state.NewTxn(snapshot)
				require.Equal(t, uint64(7), txn.GetNonce(types.ZeroAddress))
				require.NoError(t, txn.Error())
			}
		})
	}
}

func TestSimulationNonce_GenuineAbsence(t *testing.T) {
	for _, trace := range []bool{false, true} {
		name := "call"
		if trace {
			name = "trace"
		}

		t.Run(name, func(t *testing.T) {
			store, args := newSimulationNonceFixture(t)
			selected := store.historical
			selected.BaseFee = 0
			selected.ComputeHash()

			missing := types.StringToAddress("0x9092")
			args.From = &missing
			args.Value = argBytesPtr([]byte{0})
			args.GasPrice = argBytesPtr([]byte{0})

			store.accountFn = func(
				root types.Hash,
				address types.Address,
			) (*Account, error) {
				require.Equal(t, selected.StateRoot, root)
				require.Equal(t, missing, address)
				return nil, ErrStateNotFound
			}

			number := BlockNumber(selected.Number)
			result, err := invokeSimulationNonce(
				store,
				args,
				BlockNumberOrHash{BlockNumber: &number},
				trace,
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, uint64(0), store.lastTx.Nonce)
			require.Equal(t, []types.Hash{
				selected.StateRoot,
			}, store.readRoots)

			/* START ALPHA CHANGE 2999 -- distinguish fee recommendation from nonce state selection -- */
			require.Equal(t, 1, store.calls)
			require.Same(t, selected, store.lastHeader)

			if trace {
				// TraceCall preserves the supplied zero fee.
				require.Zero(t, store.headerCalls)
				require.Zero(t, store.lastTx.GasPrice.Sign())
			} else {
				// Call's existing zero-fee defaulting reads the current
				// header for its recommendation, without fetching a
				// latest-state sender nonce.
				require.Equal(t, 1, store.headerCalls)
				require.Zero(t, store.lastTx.GasPrice.Cmp(big.NewInt(1)))
			}

			require.Equal(t, argBytes{0}, *args.GasPrice)
			/* FINISH ALPHA CHANGE 2999 -- distinguish fee recommendation from nonce state selection -- */

			require.Equal(t, argUint64(99), *args.Nonce)
			requireSimulationNonceStateUnchanged(t, store)
		})
	}
}

func TestSimulationNonce_AccountFailures(t *testing.T) {
	readErr := errors.New("injected selected-state account failure")
	localErr := state.WrapLocalStateFailure(
		"selected simulation account", ErrStateNotFound,
	)

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
			name: "negative_balance",
			account: &Account{
				Nonce:   3,
				Balance: big.NewInt(-1),
			},
			wantErr: state.ErrMalformedAccount,
		},
	}

	for _, trace := range []bool{false, true} {
		endpoint := "call"
		if trace {
			endpoint = "trace"
		}

		for _, tt := range tests {
			t.Run(endpoint+"/"+tt.name, func(t *testing.T) {
				store, args := newSimulationNonceFixture(t)
				selected := store.historical
				before := *args

				store.accountFn = func(
					root types.Hash,
					address types.Address,
				) (*Account, error) {
					require.Equal(t, selected.StateRoot, root)
					require.Equal(t, store.sender, address)
					return tt.account, tt.readErr
				}

				number := BlockNumber(selected.Number)
				result, err := invokeSimulationNonce(
					store,
					args,
					BlockNumberOrHash{BlockNumber: &number},
					trace,
				)

				require.Nil(t, result)
				require.ErrorIs(t, err, tt.wantErr)
				require.Zero(t, store.calls)
				require.Zero(t, store.headerCalls)
				require.Equal(t, []types.Hash{
					selected.StateRoot,
				}, store.readRoots)
				require.Equal(t, before, *args)
				require.Equal(t, argUint64(99), *args.Nonce)

				if tt.wantErr == state.ErrMalformedAccount {
					require.ErrorIs(t, err, state.ErrLocalStateFailure)
				}
				if tt.readErr == localErr {
					require.ErrorIs(t, err, ErrStateNotFound)
				}
			})
		}
	}
}

func TestSimulationNonce_InvalidInputs(t *testing.T) {
	for _, trace := range []bool{false, true} {
		endpoint := "call"
		if trace {
			endpoint = "trace"
		}

		for _, missingArgument := range []bool{false, true} {
			name := "missing_header"
			if missingArgument {
				name = "missing_argument"
			}

			t.Run(endpoint+"/"+name, func(t *testing.T) {
				store, args := newSimulationNonceFixture(t)
				if missingArgument {
					args = nil
				} else {
					store.header = nil
				}

				result, err := invokeSimulationNonce(
					store, args, BlockNumberOrHash{}, trace,
				)

				require.Nil(t, result)
				require.Error(t, err)
				require.Zero(t, store.calls)
				require.Empty(t, store.readRoots)

				if missingArgument {
					require.ErrorContains(t, err, "required argument 0")
					require.Zero(t, store.headerCalls)
				} else {
					require.ErrorIs(t, err, ErrHeaderNotFound)
				}
			})
		}
	}
}

func TestSimulationNonce_DecodedHashMatchesNonce(t *testing.T) {
	for _, omittedSender := range []bool{false, true} {
		name := "supplied_sender"
		if omittedSender {
			name = "omitted_sender_override"
		}

		t.Run(name, func(t *testing.T) {
			store, args := newSimulationNonceFixture(t)
			selected := store.historical
			wantNonce := uint64(3)
			var overrides *stateOverride

			if omittedSender {
				args.From = nil
				wantNonce = 9
				values := stateOverride{
					types.ZeroAddress: {
						Nonce: argUintPtr(wantNonce),
					},
				}
				overrides = &values
			}

			tx, err := decodeSimulationTxn(
				args, selected, store, overrides,
			)
			require.NoError(t, err)
			require.Equal(t, wantNonce, tx.Nonce)

			expected := tx.Copy()
			expected.ComputeHash(selected.Number)
			require.Equal(t, expected.Hash, tx.Hash)
			require.Equal(t, argUint64(99), *args.Nonce)
		})
	}
}

/* FINISH ALPHA CHANGE 2997 -- test historical call and trace-call nonce ownership -- */
