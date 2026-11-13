/* START ALPHA CHANGE 2989 -- cover transfer estimation ceilings and actual destination execution -- */
package jsonrpc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type estimateTransferStore struct {
	ethStore

	t          *testing.T
	header     *types.Header
	historical *types.Header
	sender     types.Address
	target     types.Address
	balances   map[types.Address]*big.Int

	accountFn func(types.Hash, types.Address) (*Account, error)
	applyFn   func(*types.Header, *types.Transaction) (*runtime.ExecutionResult, error)

	readRoots []types.Hash
	probes    []uint64
	headers   []*types.Header
	fees      []*big.Int
	results   []*runtime.ExecutionResult
}

func (s *estimateTransferStore) Header() *types.Header {
	return s.header
}

func (s *estimateTransferStore) GetHeaderByNumber(number uint64) (*types.Header, bool) {
	if s.historical != nil && s.historical.Number == number {
		return s.historical, true
	}
	if s.header.Number == number {
		return s.header, true
	}

	return nil, false
}

func (s *estimateTransferStore) GetAccount(
	root types.Hash,
	address types.Address,
) (*Account, error) {
	s.readRoots = append(s.readRoots, root)
	if s.accountFn != nil {
		return s.accountFn(root, address)
	}

	balance, exists := s.balances[address]
	if !exists {
		return nil, ErrStateNotFound
	}

	return &Account{Balance: new(big.Int).Set(balance)}, nil
}

func (s *estimateTransferStore) GetNonce(types.Address) (uint64, error) {
	return 0, nil
}

func (s *estimateTransferStore) GetForksInTime(number uint64) chain.ForksInTime {
	return chain.AllForksEnabled.At(number)
}

func (s *estimateTransferStore) GetBaseFee() uint64 {
	return s.header.BaseFee
}

func (s *estimateTransferStore) GetAvgGasPrice() *big.Int {
	return big.NewInt(1)
}

func (s *estimateTransferStore) MaxPriorityFeePerGas() (*big.Int, error) {
	return new(big.Int), nil
}

func (s *estimateTransferStore) ApplyTxn(
	header *types.Header,
	tx *types.Transaction,
	override types.StateOverride,
	nonPayable bool,
) (*runtime.ExecutionResult, error) {
	s.t.Helper()
	require.True(s.t, nonPayable, "estimation must remain a simulation")
	require.Nil(s.t, override)

	s.probes = append(s.probes, tx.Gas)
	s.headers = append(s.headers, header)
	s.fees = append(s.fees, new(big.Int).Set(tx.GetGasFeeCap()))

	result := &runtime.ExecutionResult{}
	var err error
	if s.applyFn != nil {
		result, err = s.applyFn(header, tx)
	}
	s.results = append(s.results, result)

	return result, err
}

func estimateTransferFixture(t *testing.T) (*Eth, *estimateTransferStore, *txnArgs) {
	t.Helper()

	sender := types.StringToAddress("0x3031")
	target := types.StringToAddress("0x3032")
	store := &estimateTransferStore{
		t:      t,
		sender: sender,
		target: target,
		header: &types.Header{
			Number:    2,
			StateRoot: types.StringToHash("0x02"),
			GasLimit:  100_000,
			BaseFee:   1,
		},
		balances: map[types.Address]*big.Int{
			sender: big.NewInt(1_000_000_000),
		},
	}
	args := &txnArgs{
		From:     &sender,
		To:       &target,
		Value:    argBytesPtr([]byte{1}),
		GasPrice: argBytesPtr([]byte{1}),
	}

	return newTestEthEndpoint(store), store, args
}

func TestEstimateTransfer_PreExecutionCeilings(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*estimateTransferStore, *txnArgs)
		wantErr   error
	}{
		{
			name: "explicit_zero_gas",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				a.Gas = argUintPtr(0)
			},
			wantErr: state.ErrNotEnoughIntrinsicGas,
		},
		{
			name: "explicit_gas_below_intrinsic",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				a.Gas = argUintPtr(20_000)
			},
			wantErr: state.ErrNotEnoughIntrinsicGas,
		},
		{
			name: "block_ceiling_below_intrinsic",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				s.header.GasLimit = 20_000
				a.Gas = argUintPtr(100_000)
			},
			wantErr: state.ErrNotEnoughIntrinsicGas,
		},
		{
			name: "value_reserved_before_gas",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				s.balances[s.sender] = big.NewInt(21_000)
			},
			wantErr: state.ErrNotEnoughIntrinsicGas,
		},
		{
			name: "balance_equal_to_value_with_positive_fee",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				s.balances[s.sender] = big.NewInt(1)
			},
			wantErr: state.ErrNotEnoughIntrinsicGas,
		},
		{
			name: "value_unaffordable_even_with_zero_fee",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				s.balances[s.sender] = new(big.Int)
				a.GasPrice = argBytesPtr([]byte{0})
			},
			wantErr: runtime.ErrInsufficientBalance,
		},
		{
			name: "explicit_zero_sender_is_supplied",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				zero := types.ZeroAddress
				a.From = &zero
				a.GasPrice = argBytesPtr([]byte{0})
			},
			wantErr: runtime.ErrInsufficientBalance,
		},
		{
			name: "dynamic_maximum_cap_not_effective_price",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				a.Type = argUintPtr(uint64(types.DynamicFeeTx))
				a.GasPrice = nil
				a.GasTipCap = argBytesPtr([]byte{1})
				a.GasFeeCap = argBytesPtr([]byte{10})
				s.balances[s.sender] = big.NewInt(42_001)
			},
			wantErr: state.ErrNotEnoughIntrinsicGas,
		},
		{
			name: "omitted_legacy_fee_is_defaulted_before_funding",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				a.GasPrice = nil
				s.balances[s.sender] = big.NewInt(1)
			},
			wantErr: state.ErrNotEnoughIntrinsicGas,
		},
		{
			name: "omitted_dynamic_cap_is_defaulted_before_funding",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				a.Type = argUintPtr(uint64(types.DynamicFeeTx))
				a.GasPrice = nil
				a.GasFeeCap = nil
				s.balances[s.sender] = big.NewInt(1)
			},
			wantErr: state.ErrNotEnoughIntrinsicGas,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, store, args := estimateTransferFixture(t)
			tt.configure(store, args)

			result, err := endpoint.EstimateGas(args, nil)

			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, result)
			require.Empty(t, store.probes, "rejected requests must not execute")
		})
	}
}

func TestEstimateTransfer_OneProbeAndFullWidthFunding(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*estimateTransferStore, *txnArgs)
		wantFee   *big.Int
	}{
		{
			name:    "ordinary_funded_transfer",
			wantFee: big.NewInt(1),
		},
		{
			name: "balance_equal_to_value_with_zero_legacy_fee",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				s.balances[s.sender] = big.NewInt(1)
				a.GasPrice = argBytesPtr([]byte{0})
			},
			wantFee: new(big.Int),
		},
		{
			name: "balance_equal_to_value_with_zero_dynamic_cap",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				s.balances[s.sender] = big.NewInt(1)
				a.Type = argUintPtr(uint64(types.DynamicFeeTx))
				a.GasPrice = nil
				a.GasFeeCap = argBytesPtr([]byte{0})
			},
			wantFee: new(big.Int),
		},
		{
			name: "allowance_above_max_uint64_does_not_wrap",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				balance := new(big.Int).Lsh(big.NewInt(1), 128)
				s.balances[s.sender] = balance.Add(balance, big.NewInt(1))
			},
			wantFee: big.NewInt(1),
		},
		{
			name: "large_fee_uses_full_width_division",
			configure: func(s *estimateTransferStore, a *txnArgs) {
				fee := new(big.Int).Lsh(big.NewInt(1), 128)
				a.GasPrice = argBytesPtr(fee.Bytes())
				balance := new(big.Int).Mul(fee, new(big.Int).SetUint64(state.TxGas))
				s.balances[s.sender] = balance.Add(balance, big.NewInt(1))
			},
			wantFee: new(big.Int).Lsh(big.NewInt(1), 128),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, store, args := estimateTransferFixture(t)
			if tt.configure != nil {
				tt.configure(store, args)
			}
			before := new(big.Int).Set(store.balances[store.sender])

			result, err := endpoint.EstimateGas(args, nil)

			require.NoError(t, err)
			require.Equal(t, argUint64(state.TxGas), result)
			require.Equal(t, []uint64{state.TxGas}, store.probes)
			require.Zero(t, store.fees[0].Cmp(tt.wantFee))
			require.Zero(t, store.balances[store.sender].Cmp(before))
			require.Nil(t, args.Gas)
			require.Nil(t, args.Nonce)
		})
	}
}

func TestEstimateTransfer_OmittedSenderAndGasRemainOmitted(t *testing.T) {
	endpoint, store, args := estimateTransferFixture(t)
	args.From = nil
	args.GasPrice = nil

	store.applyFn = func(
		header *types.Header,
		tx *types.Transaction,
	) (*runtime.ExecutionResult, error) {
		require.Equal(t, types.ZeroAddress, tx.From)
		require.Equal(t, uint64(0), tx.Nonce)

		return &runtime.ExecutionResult{}, nil
	}

	for i := 0; i < 2; i++ {
		result, err := endpoint.EstimateGas(args, nil)
		require.NoError(t, err)
		require.Equal(t, argUint64(state.TxGas), result)
		require.Nil(t, args.From)
		require.Nil(t, args.Gas)
		require.Nil(t, args.GasPrice)
		require.Nil(t, args.Nonce)
	}

	require.Equal(t, []uint64{state.TxGas, state.TxGas}, store.probes)
	require.Empty(t, store.readRoots)
}

func TestEstimateTransfer_FailedIntrinsicProbeContinuesWithinCeiling(t *testing.T) {
	tests := []struct {
		name     string
		applyErr error
		evmErr   error
	}{
		{
			name:     "intrinsic_application_error",
			applyErr: state.NewTransitionApplicationError(state.ErrNotEnoughIntrinsicGas, false),
		},
		{name: "out_of_gas", evmErr: runtime.ErrOutOfGas},
		{name: "code_store_out_of_gas", evmErr: runtime.ErrCodeStoreOutOfGas},
		{name: "gas_dependent_revert", evmErr: runtime.ErrExecutionReverted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, store, args := estimateTransferFixture(t)
			args.Gas = argUintPtr(40_000)

			store.applyFn = func(
				header *types.Header,
				tx *types.Transaction,
			) (*runtime.ExecutionResult, error) {
				if tx.Gas < 35_000 {
					return &runtime.ExecutionResult{Err: tt.evmErr}, tt.applyErr
				}

				return &runtime.ExecutionResult{}, nil
			}

			result, err := endpoint.EstimateGas(args, nil)

			require.NoError(t, err)
			require.Equal(t, argUint64(35_000), result)
			require.Equal(t, state.TxGas, store.probes[0])
			for _, gas := range store.probes {
				require.LessOrEqual(t, gas, uint64(40_000))
			}
		})
	}
}

func TestEstimateTransfer_BlockAndFundingCeilingsBoundSearch(t *testing.T) {
	for _, fundingLimited := range []bool{false, true} {
		name := "block"
		if fundingLimited {
			name = "funding"
		}

		t.Run(name, func(t *testing.T) {
			endpoint, store, args := estimateTransferFixture(t)
			args.Gas = argUintPtr(90_000)

			if fundingLimited {
				store.balances[store.sender] = big.NewInt(30_001)
			} else {
				store.header.GasLimit = 30_000
			}

			store.applyFn = func(
				header *types.Header,
				tx *types.Transaction,
			) (*runtime.ExecutionResult, error) {
				if tx.Gas < 35_000 {
					return &runtime.ExecutionResult{Err: runtime.ErrOutOfGas}, nil
				}

				return &runtime.ExecutionResult{}, nil
			}

			_, err := endpoint.EstimateGas(args, nil)

			require.ErrorIs(t, err, runtime.ErrOutOfGas)
			require.Equal(t, state.TxGas, store.probes[0])
			for _, gas := range store.probes {
				require.LessOrEqual(t, gas, uint64(30_000))
			}
		})
	}
}

func TestEstimateTransfer_LocalFailureStopsIntrinsicProbe(t *testing.T) {
	causes := []error{
		state.ErrNotEnoughIntrinsicGas,
		runtime.ErrOutOfGas,
		runtime.ErrCodeStoreOutOfGas,
		runtime.ErrExecutionReverted,
	}

	for _, cause := range causes {
		for _, resultFailure := range []bool{false, true} {
			name := cause.Error() + "/apply"
			if resultFailure {
				name = cause.Error() + "/result"
			}

			t.Run(name, func(t *testing.T) {
				endpoint, store, args := estimateTransferFixture(t)
				localErr := state.WrapLocalStateFailure("intrinsic probe", cause)

				store.applyFn = func(
					header *types.Header,
					tx *types.Transaction,
				) (*runtime.ExecutionResult, error) {
					if resultFailure {
						return &runtime.ExecutionResult{Err: localErr}, nil
					}

					return nil, localErr
				}

				result, err := endpoint.EstimateGas(args, nil)

				require.Nil(t, result)
				require.ErrorIs(t, err, state.ErrLocalStateFailure)
				require.ErrorIs(t, err, cause)
				require.Equal(t, []uint64{state.TxGas}, store.probes)
			})
		}
	}
}

func TestEstimateTransfer_SelectedHeaderBoundsFundingAndExecution(t *testing.T) {
	endpoint, store, args := estimateTransferFixture(t)
	historical := &types.Header{
		Number:    1,
		StateRoot: types.StringToHash("0x01"),
		GasLimit:  40_000,
		BaseFee:   1,
	}
	store.historical = historical
	historicalBalance := big.NewInt(40_001)

	store.accountFn = func(root types.Hash, address types.Address) (*Account, error) {
		balance := big.NewInt(1_000_000_000)
		if root == historical.StateRoot {
			balance = historicalBalance
		}

		return &Account{Balance: new(big.Int).Set(balance)}, nil
	}
	store.applyFn = func(
		header *types.Header,
		tx *types.Transaction,
	) (*runtime.ExecutionResult, error) {
		require.Same(t, historical, header)
		if tx.Gas < 40_000 {
			return &runtime.ExecutionResult{Err: runtime.ErrOutOfGas}, nil
		}

		return &runtime.ExecutionResult{}, nil
	}

	number := BlockNumber(historical.Number)
	result, err := endpoint.EstimateGas(args, &number)

	require.NoError(t, err)
	require.Equal(t, argUint64(40_000), result)
	require.Equal(t, historical.StateRoot, store.readRoots[len(store.readRoots)-1])
	for _, gas := range store.probes {
		require.LessOrEqual(t, gas, historical.GasLimit)
	}

	historicalBalance = big.NewInt(20_001)
	store.probes = nil

	result, err = endpoint.EstimateGas(args, &number)

	require.Nil(t, result)
	require.ErrorIs(t, err, state.ErrNotEnoughIntrinsicGas)
	require.Empty(t, store.probes)
}

func TestEstimateTransfer_AccountFailureAndMissingResult(t *testing.T) {
	t.Run("local_account_failure_is_not_absence", func(t *testing.T) {
		endpoint, store, args := estimateTransferFixture(t)
		localErr := state.WrapLocalStateFailure("account read", ErrStateNotFound)
		store.accountFn = func(types.Hash, types.Address) (*Account, error) {
			return nil, localErr
		}

		result, err := endpoint.EstimateGas(args, nil)

		require.Nil(t, result)
		require.ErrorIs(t, err, state.ErrLocalStateFailure)
		require.Empty(t, store.probes)
	})

	t.Run("missing_execution_result", func(t *testing.T) {
		endpoint, store, args := estimateTransferFixture(t)
		store.applyFn = func(
			*types.Header,
			*types.Transaction,
		) (*runtime.ExecutionResult, error) {
			return nil, nil
		}

		result, err := endpoint.EstimateGas(args, nil)

		require.Nil(t, result)
		require.ErrorContains(t, err, "no execution result")
		require.Equal(t, []uint64{state.TxGas}, store.probes)
	})

	t.Run("missing_argument", func(t *testing.T) {
		endpoint, store, _ := estimateTransferFixture(t)

		result, err := endpoint.EstimateGas(nil, nil)

		require.Nil(t, result)
		require.Error(t, err)
		require.Empty(t, store.probes)
	})
}

func estimateTransferExecutionFixture(
	t *testing.T,
	code []byte,
	precompile bool,
) (*Eth, *estimateTransferStore, *txnArgs, *state.Executor) {
	t.Helper()

	endpoint, store, args := estimateTransferFixture(t)
	if precompile {
		store.target = types.StringToAddress("0x04")
		target := store.target
		args.To = &target
	}

	storage := itrie.NewMemoryStorage()
	t.Cleanup(func() {
		require.NoError(t, storage.Close())
	})

	executor := state.NewExecutor(
		&chain.Params{ChainID: 100, Forks: chain.AllForksEnabled},
		itrie.NewState(storage),
		hclog.NewNullLogger(),
	)
	/* START ALPHA CHANGE 3810 -- adapt estimation fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) state.GetHashByNumber {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt estimation fixture to checked hash lookups -- */

	root, err := executor.WriteGenesis(
		map[types.Address]*chain.GenesisAccount{
			store.sender: {
				Balance: big.NewInt(1_000_000_000),
			},
			store.target: {
				Balance: new(big.Int),
				Code:    append([]byte(nil), code...),
			},
		},
		types.ZeroHash,
	)
	require.NoError(t, err)
	store.header.StateRoot = root

	store.accountFn = func(root types.Hash, address types.Address) (*Account, error) {
		snapshot, err := executor.StateAt(root)
		if err != nil {
			return nil, err
		}
		txn := state.NewTxn(snapshot)
		balance := new(big.Int).Set(txn.GetBalance(address))
		nonce := txn.GetNonce(address)
		if err := txn.Error(); err != nil {
			return nil, err
		}

		return &Account{Balance: balance, Nonce: nonce}, nil
	}

	store.applyFn = func(
		header *types.Header,
		tx *types.Transaction,
	) (*runtime.ExecutionResult, error) {
		transition, err := executor.BeginTxn(
			header.StateRoot,
			header,
			types.ZeroAddress,
		)
		if err != nil {
			return nil, err
		}

		transition.SetNonPayable(true)
		result, err := transition.Apply(tx)
		if stateErr := transition.Error(); stateErr != nil {
			return nil, stateErr
		}

		// Each probe has its own transition. Never commit simulation changes.
		return result, err
	}

	return endpoint, store, args, executor
}

func TestEstimateTransfer_ActualExecution(t *testing.T) {
	tests := []struct {
		name       string
		code       []byte
		precompile bool
		gas        *argUint64
		wantGas    uint64
		wantErr    error
	}{
		{
			name:    "ordinary_account",
			wantGas: state.TxGas,
		},
		{
			name: "empty_input_contract_writes_storage",
			// PUSH1 1; PUSH1 0; SSTORE; STOP.
			code: []byte{0x60, 0x01, 0x60, 0x00, 0x55, 0x00},
		},
		{
			name:    "contract_needs_more_than_caller_ceiling",
			code:    []byte{0x60, 0x01, 0x60, 0x00, 0x55, 0x00},
			gas:     argUintPtr(30_000),
			wantErr: runtime.ErrOutOfGas,
		},
		{
			name: "reverting_contract_returns_data",
			// Store 0xdead, then revert with its final two bytes.
			code: []byte{
				0x61, 0xde, 0xad, 0x60, 0x00, 0x52,
				0x60, 0x02, 0x60, 0x1e, 0xfd,
			},
			wantErr: runtime.ErrExecutionReverted,
		},
		{
			name: "gas_dependent_revert",
			// Revert unless GAS is greater than 10,000.
			// The successful branch starts at JUMPDEST offset 13.
			code: []byte{
				0x61, 0x27, 0x10, 0x5a, 0x11,
				0x60, 0x0d, 0x57,
				0x60, 0x00, 0x60, 0x00, 0xfd,
				0x5b, 0x00,
			},
		},
		{
			name:       "identity_precompile_without_bytecode",
			precompile: true,
			wantGas:    state.TxGas + 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, store, args, executor := estimateTransferExecutionFixture(
				t, tt.code, tt.precompile,
			)
			args.Gas = tt.gas

			result, err := endpoint.EstimateGas(args, nil)

			require.NotEmpty(t, store.probes)
			require.Equal(t, state.TxGas, store.probes[0])
			for _, gas := range store.probes {
				require.LessOrEqual(t, gas, store.header.GasLimit)
				if tt.gas != nil {
					require.LessOrEqual(t, gas, uint64(*tt.gas))
				}
			}

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				if errors.Is(tt.wantErr, runtime.ErrExecutionReverted) {
					require.Equal(t, []byte("dead"), result)
				}
			} else {
				require.NoError(t, err)
				require.IsType(t, argUint64(0), result)
				estimated := uint64(result.(argUint64))

				if tt.wantGas != 0 {
					require.Equal(t, tt.wantGas, estimated)
				} else {
					require.Greater(t, estimated, state.TxGas)
				}
				if tt.name == "ordinary_account" {
					require.Len(t, store.probes, 1)
				}
				if tt.name == "gas_dependent_revert" {
					sawRevert := false
					for _, execution := range store.results {
						if execution != nil && execution.Reverted() {
							sawRevert = true
						}
					}
					require.True(t, sawRevert, "search must pass a reverting lower allowance")
				}

				// Confirm success and the immediately lower failing allowance
				// using fresh real execution transitions.
				tx := &types.Transaction{
					From:     store.sender,
					To:       args.To,
					Value:    big.NewInt(1),
					GasPrice: big.NewInt(1),
					Gas:      estimated,
				}
				execution, applyErr := store.ApplyTxn(store.header, tx, nil, true)
				require.NoError(t, applyErr)
				require.NotNil(t, execution)
				require.NoError(t, execution.Err)

				tx.Gas = estimated - 1
				execution, applyErr = store.ApplyTxn(store.header, tx, nil, true)
				require.True(t,
					applyErr != nil || (execution != nil && execution.Failed()),
					"the immediately lower allowance must fail",
				)
			}

			// Simulation must not publish value, nonce or storage changes.
			snapshot, snapshotErr := executor.StateAt(store.header.StateRoot)
			require.NoError(t, snapshotErr)
			unchanged := state.NewTxn(snapshot)
			require.Zero(t, unchanged.GetBalance(store.sender).Cmp(big.NewInt(1_000_000_000)))
			require.Zero(t, unchanged.GetBalance(store.target).Sign())
			require.Zero(t, unchanged.GetNonce(store.sender))
			require.Equal(t, types.ZeroHash, unchanged.GetState(store.target, types.ZeroHash))
			if tt.precompile {
				require.Empty(t, unchanged.GetCode(store.target))
			}
			require.NoError(t, unchanged.Error())
		})
	}
}

func TestEstimateTransfer_OmittedSenderDoesNotCreateFunds(t *testing.T) {
	endpoint, store, args, _ := estimateTransferExecutionFixture(t, nil, false)
	args.From = nil

	_, err := endpoint.EstimateGas(args, nil)

	require.ErrorIs(t, err, runtime.ErrInsufficientBalance)
	require.Equal(t, []uint64{state.TxGas}, store.probes)
	require.Nil(t, args.From)
	require.Nil(t, args.Gas)
}

/* FINISH ALPHA CHANGE 2989 -- cover transfer estimation ceilings and actual destination execution -- */
