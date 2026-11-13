/* START ALPHA CHANGE 2160 -- exercise executor local failures and ordinary failed transactions -- */
package state

import (
	"errors"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

// These failures are injected at the snapshot boundary. Trie/backend fault
// injection remains covered by the batch-1 tests; no alternate executor is used.
type issue7AExecutionSnapshot struct {
	accounts              map[types.Address]*Account
	code                  map[types.Hash][]byte
	accountFailureAddress types.Address
	accountErr            error
	storageErr            error
	codeErr               error
	commitErr             error
	commits               int
}

func (s *issue7AExecutionSnapshot) GetAccount(addr types.Address) (*Account, error) {
	if s.accountErr != nil && addr == s.accountFailureAddress {
		return nil, s.accountErr
	}
	if a := s.accounts[addr]; a != nil {
		return a.Copy(), nil
	}
	return nil, nil
}

func (s *issue7AExecutionSnapshot) GetStorage(
	types.Address,
	types.Hash,
	types.Hash,
) (types.Hash, error) {
	return types.ZeroHash, s.storageErr
}

func (s *issue7AExecutionSnapshot) GetCode(hash types.Hash) ([]byte, bool, error) {
	if hash == types.EmptyCodeHash {
		return []byte{}, true, nil
	}
	if s.codeErr != nil {
		return nil, false, s.codeErr
	}
	code, found := s.code[hash]
	return append([]byte(nil), code...), found, nil
}

func (s *issue7AExecutionSnapshot) Commit([]*Object) (Snapshot, []byte, error) {
	s.commits++
	if s.commitErr != nil {
		return nil, nil, s.commitErr
	}
	return s, types.EmptyRootHash.Bytes(), nil
}

type issue7AExecutionState struct {
	snapshot *issue7AExecutionSnapshot
	openErr  error
}

func (s *issue7AExecutionState) NewSnapshot() Snapshot {
	return s.snapshot
}

func (s *issue7AExecutionState) NewSnapshotAt(types.Hash) (Snapshot, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	return s.snapshot, nil
}

func (s *issue7AExecutionState) GetCode(hash types.Hash) ([]byte, bool, error) {
	return s.snapshot.GetCode(hash)
}

var issue7ASender = types.StringToAddress("0x7001")
var issue7ATarget = types.StringToAddress("0x7002")
var issue7ABadAddress = types.StringToAddress("0x70ff")

func issue7AExecutionFixture(
	t *testing.T,
	code []byte,
) (*Executor, *issue7AExecutionState, *Transition, *types.Transaction) {
	t.Helper()

	snapshot := &issue7AExecutionSnapshot{
		accounts: map[types.Address]*Account{},
		code:     map[types.Hash][]byte{},
	}
	snapshot.accounts[issue7ASender] = &Account{
		Balance:  big.NewInt(1_000_000_000),
		Root:     types.EmptyRootHash,
		CodeHash: types.EmptyCodeHash.Bytes(),
	}

	hash := types.BytesToHash(crypto.Keccak256(code))
	snapshot.accounts[issue7ATarget] = &Account{
		Balance:  new(big.Int),
		Root:     types.StringToHash("0x1234"),
		CodeHash: hash.Bytes(),
	}
	snapshot.code[hash] = append([]byte(nil), code...)

	backend := &issue7AExecutionState{snapshot: snapshot}
	executor := NewExecutor(
		&chain.Params{ChainID: 100, Forks: chain.AllForksEnabled},
		backend,
		hclog.NewNullLogger(),
	)
	/* START ALPHA CHANGE 3810 -- adapt execution-failure fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt execution-failure fixture to checked hash lookups -- */

	transition, err := executor.BeginTxn(
		types.EmptyRootHash,
		&types.Header{Number: 1, GasLimit: 1_000_000, BaseFee: 1},
		types.ZeroAddress,
	)
	require.NoError(t, err)

	tx := &types.Transaction{
		From:     issue7ASender,
		To:       issue7ATarget.Ptr(),
		Gas:      200_000,
		GasPrice: big.NewInt(1),
		Value:    new(big.Int),
	}

	return executor, backend, transition, tx
}

func issue7ARequireRejected(
	t *testing.T,
	tr *Transition,
	s *issue7AExecutionSnapshot,
	err, cause error,
) {
	t.Helper()

	require.ErrorIs(t, err, ErrLocalStateFailure)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, tr.Error(), cause)
	require.Empty(t, tr.Receipts())
	require.Zero(t, tr.TotalGas())

	snapshot, root, commitErr := tr.Commit()
	require.Nil(t, snapshot)
	require.Equal(t, types.ZeroHash, root)
	require.ErrorIs(t, commitErr, cause)
	require.Zero(t, s.commits)
}

func TestIssue7A_ExecutionOpeningAndGovernanceErrors(t *testing.T) {
	for _, name := range []string{
		"begin",
		"cap",
		"genesis",
		"empty block",
		"burn",
		"community",
		"maximum",
	} {
		t.Run(name, func(t *testing.T) {
			executor, backend, _, _ := issue7AExecutionFixture(t, []byte{0x00})
			cause := errors.New("injected snapshot failure")
			backend.openErr = cause

			var err error
			switch name {
			case "begin":
				_, err = executor.BeginTxn(
					types.EmptyRootHash,
					&types.Header{},
					types.ZeroAddress,
				)
			case "cap":
				_, err = executor.ReadMaxBaseFeeWei(types.EmptyRootHash)
			case "genesis":
				_, err = executor.WriteGenesis(nil, types.EmptyRootHash)
			case "empty block":
				_, err = executor.ProcessBlock(
					types.EmptyRootHash,
					&types.Block{Header: &types.Header{}},
					types.ZeroAddress,
				)
			default:
				backend.openErr = nil
				backend.snapshot.accountFailureAddress = issue7ABadAddress
				backend.snapshot.accountErr = cause
				txn := NewTxn(backend.snapshot)

				// First-error precedence also holds when a governance account is absent.
				txn.GetNonce(issue7ABadAddress)

				switch name {
				case "burn":
					_, err = readFeeBurnRateBps(txn)
				case "community":
					_, err = readCommunityRewardRateBps(txn)
				case "maximum":
					_, err = readMaxBaseFeeWei(txn)
				}
			}

			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, ErrLocalStateFailure)
		})
	}
}

func TestIssue7A_ExecutionStateFailurePrecedence(t *testing.T) {
	for _, name := range []string{
		"nonce",
		"upfront balance",
		"code",
		"missing code",
		"storage",
		"fee recipient",
	} {
		t.Run(name, func(t *testing.T) {
			_, backend, tr, tx := issue7AExecutionFixture(
				t,
				[]byte{0x60, 0x00, 0x54, 0x50, 0x00}, // SLOAD; POP; STOP
			)
			cause := errors.New("injected execution read failure")
			var err error

			switch name {
			case "nonce":
				backend.snapshot.accountFailureAddress = issue7ASender
				backend.snapshot.accountErr = cause
				tx.Nonce = 99
				_, err = tr.Apply(tx)
				require.False(t, errors.Is(err, ErrNonceIncorrect))

			case "upfront balance":
				backend.snapshot.accountFailureAddress = issue7ASender
				backend.snapshot.accountErr = cause
				err = tr.subGasLimitPrice(tx)
				require.False(t, errors.Is(err, ErrNotEnoughFundsForGas))

			case "code":
				backend.snapshot.codeErr = cause
				err = tr.Write(tx)

			case "missing code":
				backend.snapshot.code = map[types.Hash][]byte{}
				cause = ErrMissingCode
				err = tr.Write(tx)

			case "storage":
				backend.snapshot.storageErr = cause
				err = tr.Write(tx)

			case "fee recipient":
				backend.snapshot.accountFailureAddress = types.ZeroAddress
				backend.snapshot.accountErr = cause
				err = tr.Write(tx)
			}

			issue7ARequireRejected(t, tr, backend.snapshot, err, cause)
		})
	}
}

func TestIssue7A_OrdinaryErrorsAndPostHookRemainOrdinary(t *testing.T) {
	for _, ordinaryApplyError := range []bool{false, true} {
		t.Run(
			map[bool]string{
				false: "EVM revert receipt",
				true:  "nonce error hook",
			}[ordinaryApplyError],
			func(t *testing.T) {
				_, _, tr, tx := issue7AExecutionFixture(
					t,
					[]byte{0x60, 0x00, 0x60, 0x00, 0xfd},
				)
				hooks := 0
				tr.PostHook = func(*Transition) {
					hooks++
				}

				if ordinaryApplyError {
					tx.Nonce = 1
					result, err := tr.Apply(tx)
					require.Nil(t, result)

					var applicationErr *TransitionApplicationError
					require.ErrorAs(t, err, &applicationErr)
					require.ErrorIs(t, applicationErr.Err, ErrNonceIncorrect)
				} else {
					require.NoError(t, tr.Write(tx))
					require.Len(t, tr.Receipts(), 1)
					require.NotNil(t, tr.Receipts()[0].Status)
					require.Equal(t, types.ReceiptFailed, *tr.Receipts()[0].Status)
					require.Greater(t, tr.TotalGas(), uint64(0))
					require.Equal(t, uint64(1), tr.GetNonce(issue7ASender))
				}

				require.Equal(t, 1, hooks)
				require.NoError(t, tr.Error())
			},
		)
	}
}

func TestIssue7A_ApplyHookFailureAndRollback(t *testing.T) {
	for _, when := range []string{
		"execution",
		"successful apply hook",
		"ordinary error hook",
	} {
		t.Run(when, func(t *testing.T) {
			_, backend, tr, tx := issue7AExecutionFixture(
				t,
				[]byte{0x60, 0x00, 0x54, 0x50, 0x00},
			)
			cause := errors.New("injected hook or execution read failure")
			hooks := 0
			tr.PostHook = func(current *Transition) {
				hooks++
				current.GetNonce(issue7ABadAddress)
			}

			backend.snapshot.accountFailureAddress = issue7ABadAddress
			backend.snapshot.accountErr = cause
			if when == "execution" {
				backend.snapshot.storageErr = cause
			}
			if when == "ordinary error hook" {
				tx.Nonce = 1
			}

			err := tr.Write(tx)
			issue7ARequireRejected(t, tr, backend.snapshot, err, cause)
			require.Equal(t, uint64(0), tr.GetNonce(issue7ASender))

			if when == "execution" {
				require.Zero(t, hooks)
			} else {
				require.Equal(t, 1, hooks)
			}

			first := tr.Error()
			require.ErrorIs(t, tr.Write(tx), cause)
			require.Same(t, first, tr.Error())
		})
	}
}

func TestIssue7A_NestedFailureCannotBecomeSuccessfulOuterCall(t *testing.T) {
	_, backend, tr, tx := issue7AExecutionFixture(t, nil)

	inner := types.StringToAddress("0x7003")
	innerCode := []byte{
		0x60, 0x00, 0x54, 0x50,
		0x60, 0x00, 0x60, 0x00, 0xfd,
	}
	innerHash := types.BytesToHash(crypto.Keccak256(innerCode))
	backend.snapshot.accounts[inner] = &Account{
		Balance:  new(big.Int),
		Root:     types.StringToHash("0x1234"),
		CodeHash: innerHash.Bytes(),
	}
	backend.snapshot.code[innerHash] = innerCode

	// CALL the inner contract, discard its success flag and STOP. An ordinary
	// inner REVERT would permit outer success; a storage error must not.
	outer := []byte{
		0x60, 0x00, 0x60, 0x00, 0x60, 0x00,
		0x60, 0x00, 0x60, 0x00, 0x73,
	}
	outer = append(outer, inner.Bytes()...)
	outer = append(outer, 0x62, 0x01, 0x86, 0xa0, 0xf1, 0x50, 0x00)
	outerHash := types.BytesToHash(crypto.Keccak256(outer))
	backend.snapshot.accounts[issue7ATarget].CodeHash = outerHash.Bytes()
	backend.snapshot.code[outerHash] = outer

	cause := errors.New("injected inner SLOAD failure")
	backend.snapshot.storageErr = cause

	result, err := tr.Apply(tx)
	require.Nil(t, result)
	issue7ARequireRejected(t, tr, backend.snapshot, err, cause)
	require.Equal(t, uint64(0), tr.GetNonce(issue7ASender))
}

func TestIssue7A_DirectCallCreationAndHelpersRejectStateFailure(t *testing.T) {
	for _, name := range []string{
		"Call2",
		"Create2",
		"Callx",
		"Transfer",
		"SetAccountDirectly",
		"SetCodeDirectly",
		"WithStateOverride",
	} {
		t.Run(name, func(t *testing.T) {
			_, backend, tr, _ := issue7AExecutionFixture(t, []byte{0x00})
			cause := errors.New("injected direct read failure")
			backend.snapshot.accountFailureAddress = issue7ABadAddress
			backend.snapshot.accountErr = cause

			var err error
			switch name {
			case "Call2":
				err = tr.Call2(
					issue7ASender,
					issue7ABadAddress,
					nil,
					new(big.Int),
					100000,
				).Err
			case "Create2":
				err = tr.Create2(
					issue7ABadAddress,
					[]byte{0x00},
					new(big.Int),
					100000,
				).Err
			case "Callx":
				contract := runtime.NewContractCall(
					1,
					issue7ASender,
					issue7ASender,
					issue7ABadAddress,
					new(big.Int),
					100000,
					[]byte{0x00},
					nil,
				)
				err = tr.Callx(contract, tr).Err
			case "Transfer":
				err = tr.Transfer(issue7ABadAddress, issue7ATarget, big.NewInt(1))
			case "SetAccountDirectly":
				err = tr.SetAccountDirectly(
					issue7ABadAddress,
					&chain.GenesisAccount{Balance: big.NewInt(1)},
				)
			case "SetCodeDirectly":
				err = tr.SetCodeDirectly(issue7ABadAddress, []byte{0x00})
			case "WithStateOverride":
				n := uint64(2)
				err = tr.WithStateOverride(types.StateOverride{
					issue7ABadAddress: types.OverrideAccount{Nonce: &n},
				})
			}

			issue7ARequireRejected(t, tr, backend.snapshot, err, cause)

			// Once latched, even a no-op transfer and an unrelated helper must fail.
			require.ErrorIs(t, tr.Transfer(issue7ASender, issue7ATarget, nil), cause)
			require.ErrorIs(t, tr.WithStateOverride(nil), cause)
		})
	}
}

func TestIssue7A_ProcessBlockDoesNotReturnPartialExecution(t *testing.T) {
	executor, backend, _, tx := issue7AExecutionFixture(t, []byte{0x00})
	cause := errors.New("injected block bytecode read failure")
	backend.snapshot.codeErr = cause

	transition, err := executor.ProcessBlock(
		types.EmptyRootHash,
		&types.Block{
			Header: &types.Header{
				Number:   1,
				GasLimit: 1_000_000,
				BaseFee:  1,
			},
			Transactions: []*types.Transaction{tx},
		},
		types.ZeroAddress,
	)

	require.Nil(t, transition)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, ErrLocalStateFailure)
	require.Zero(t, backend.snapshot.commits)
}

func TestIssue7A_GenesisFailurePreventsPublication(t *testing.T) {
	for _, name := range []string{"allocation", "hook", "batch", "ordinary hook"} {
		t.Run(name, func(t *testing.T) {
			executor, backend, _, _ := issue7AExecutionFixture(t, []byte{0x00})
			cause := errors.New("injected genesis failure")
			other := errors.New("ordinary hook error")
			hooks := 0

			allocation := map[types.Address]*chain.GenesisAccount{
				issue7ABadAddress: {Balance: big.NewInt(1)},
			}
			executor.GenesisPostHook = func(tr *Transition) error {
				hooks++
				if name == "hook" {
					tr.GetNonce(issue7ABadAddress)
					return other
				}
				if name == "ordinary hook" {
					return other
				}
				return nil
			}

			if name == "allocation" || name == "hook" {
				backend.snapshot.accountFailureAddress = issue7ABadAddress
				backend.snapshot.accountErr = cause
			}
			if name == "hook" {
				allocation = nil
			}
			if name == "batch" {
				backend.snapshot.commitErr = cause
			}

			root, err := executor.WriteGenesis(allocation, types.ZeroHash)
			require.Equal(t, types.ZeroHash, root)

			if name == "ordinary hook" {
				require.ErrorIs(t, err, other)
				require.False(t, errors.Is(err, ErrLocalStateFailure))
			} else {
				require.ErrorIs(t, err, cause)
				require.ErrorIs(t, err, ErrLocalStateFailure)
				require.False(t, errors.Is(err, other))
			}

			if name == "allocation" {
				require.Zero(t, hooks)
			} else {
				require.Equal(t, 1, hooks)
			}
			if name == "batch" {
				require.Equal(t, 1, backend.snapshot.commits)
			} else {
				require.Zero(t, backend.snapshot.commits)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2160 -- exercise executor local failures and ordinary failed transactions -- */

/* START ALPHA CHANGE 2154 -- inject governance reads before semantic interpretation -- */
func TestIssue7A_GovernanceFreshStorageErrors(t *testing.T) {
	for _, name := range []string{"burn", "community", "maximum"} {
		t.Run(name, func(t *testing.T) {
			_, backend, _, _ := issue7AExecutionFixture(t, []byte{0x00})
			address := contracts.INNBCBlockRewardGovernanceContract
			if name == "community" {
				address = contracts.INNBCCommunityRewardPoolContract
			}

			backend.snapshot.accounts[address] = &Account{
				Balance:  new(big.Int),
				Root:     types.StringToHash("0x1234"),
				CodeHash: types.EmptyCodeHash.Bytes(),
			}
			cause := errors.New("injected governance storage error")
			backend.snapshot.storageErr = cause
			txn := NewTxn(backend.snapshot)

			var err error
			switch name {
			case "burn":
				_, err = readFeeBurnRateBps(txn)
			case "community":
				_, err = readCommunityRewardRateBps(txn)
			case "maximum":
				_, err = readMaxBaseFeeWei(txn)
			}

			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, ErrLocalStateFailure)
		})
	}
}

/* FINISH ALPHA CHANGE 2154 -- inject governance reads before semantic interpretation -- */
