/* START ALPHA CHANGE 3037 -- verify rejected creation through execution and MDBX reopen -- */
package itrie

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/addresslist"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type creationRollbackFixture struct {
	executor *statepkg.Executor
	world    *State
	storage  Storage
	path     string
	key      *ecdsa.PrivateKey
	sender   types.Address
}

func newCreationRollbackFixture(t *testing.T) *creationRollbackFixture {
	t.Helper()

	path := t.TempDir()
	storage, err := NewMDBXStorage(path, hclog.NewNullLogger())
	require.NoError(t, err)

	key, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	world := NewState(storage)
	executor := statepkg.NewExecutor(
		&chain.Params{ChainID: 100, Forks: chain.AllForksEnabled},
		world,
		hclog.NewNullLogger(),
	)
	/* START ALPHA CHANGE 3810 -- adapt creation fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt creation fixture to checked hash lookups -- */

	fixture := &creationRollbackFixture{
		executor: executor,
		world:    world,
		storage:  storage,
		path:     path,
		key:      key,
		sender:   crypto.PubKeyToAddress(&key.PublicKey),
	}

	t.Cleanup(func() {
		if fixture.storage != nil {
			require.NoError(t, fixture.storage.Close())
		}
	})

	return fixture
}

func (f *creationRollbackFixture) transition(
	t *testing.T,
	policy string,
	alloc map[types.Address]*chain.GenesisAccount,
	denied types.Address,
	allowed *types.Address,
) *statepkg.Transition {
	t.Helper()

	listAddress := contracts.AllowListContractsAddr
	storage := make(map[types.Hash]types.Hash)

	switch policy {
	case "allowlist":
		f.executorConfigAllowList(t, true)
		if allowed != nil {
			storage[types.BytesToHash(allowed.Bytes())] =
				types.Hash(addresslist.EnabledRole)
		}
	case "blocklist":
		f.executorConfigAllowList(t, false)
		listAddress = contracts.BlockListContractsAddr
		storage[types.BytesToHash(denied.Bytes())] =
			types.Hash(addresslist.EnabledRole)
	default:
		t.Fatalf("unexpected policy %q", policy)
	}

	alloc[listAddress] = &chain.GenesisAccount{
		Balance: new(big.Int),
		Storage: storage,
	}
	alloc[f.sender] = &chain.GenesisAccount{
		Balance: big.NewInt(10_000_000),
	}

	root, err := f.executor.WriteGenesis(alloc, types.ZeroHash)
	require.NoError(t, err)

	tr, err := f.executor.BeginTxn(
		root,
		&types.Header{
			Number:   1,
			GasLimit: 4_000_000,
			BaseFee:  1,
		},
		types.StringToAddress("0x9201"),
	)
	require.NoError(t, err)

	return tr
}

func (f *creationRollbackFixture) executorConfigAllowList(
	t *testing.T,
	allowList bool,
) {
	t.Helper()

	params := &chain.Params{
		ChainID: 100,
		Forks:   chain.AllForksEnabled,
	}
	if allowList {
		params.ContractDeployerAllowList = &chain.AddressListConfig{}
	} else {
		params.ContractDeployerBlockList = &chain.AddressListConfig{}
	}

	f.executor = statepkg.NewExecutor(params, f.world, hclog.NewNullLogger())
	/* START ALPHA CHANGE 3810 -- adapt creation-policy fixture to checked hash lookups -- */
	f.executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt creation-policy fixture to checked hash lookups -- */
}

func (f *creationRollbackFixture) write(
	t *testing.T,
	tr *statepkg.Transition,
	to *types.Address,
	gas uint64,
	value int64,
	input []byte,
) *types.Receipt {
	t.Helper()

	tx, err := crypto.NewEIP155Signer(100, true).SignTx(
		&types.Transaction{
			Type:     types.LegacyTx,
			To:       to,
			Gas:      gas,
			GasPrice: big.NewInt(1),
			Value:    big.NewInt(value),
			Input:    input,
		},
		f.key,
	)
	require.NoError(t, err)
	require.NoError(t, tr.Write(tx))
	require.NoError(t, tr.Error())

	receipts := tr.Receipts()
	require.Len(t, receipts, 1)
	require.NotNil(t, receipts[0].Status)
	require.Equal(t, receipts[0].GasUsed, tr.TotalGas())

	return receipts[0]
}

func (f *creationRollbackFixture) reopen(
	t *testing.T,
	tr *statepkg.Transition,
) statepkg.Snapshot {
	t.Helper()

	_, root, err := tr.Commit()
	require.NoError(t, err)

	require.NoError(t, f.storage.Close())
	f.storage = nil

	reopened, err := NewMDBXStorageReadOnly(f.path, hclog.NewNullLogger())
	require.NoError(t, err)
	f.storage = reopened

	snapshot, err := NewState(reopened).NewSnapshotAt(root)
	require.NoError(t, err)

	return snapshot
}

func requireCreationRollbackDestination(
	t *testing.T,
	snapshot statepkg.Snapshot,
	address types.Address,
	prefunded bool,
) {
	t.Helper()

	account, err := snapshot.GetAccount(address)
	require.NoError(t, err)

	if !prefunded {
		require.Nil(t, account)
		return
	}

	require.NotNil(t, account)
	require.Zero(t, account.Nonce)
	require.Zero(t, account.Balance.Cmp(big.NewInt(55)))
	require.Equal(t, types.EmptyCodeHash.Bytes(), account.CodeHash)
	require.Equal(t, types.EmptyRootHash, account.Root)
}

func TestCreationRollback_FailedReceiptPersistsCorrectState(t *testing.T) {
	for _, policy := range []string{"allowlist", "blocklist"} {
		t.Run(policy, func(t *testing.T) {
			for _, prefunded := range []bool{false, true} {
				name := "new_destination"
				if prefunded {
					name = "prefunded_destination"
				}

				t.Run(name, func(t *testing.T) {
					fixture := newCreationRollbackFixture(t)
					destination := crypto.CreateAddress(fixture.sender, 0)
					alloc := make(map[types.Address]*chain.GenesisAccount)
					if prefunded {
						alloc[destination] = &chain.GenesisAccount{
							Balance: big.NewInt(55),
						}
					}

					tr := fixture.transition(
						t, policy, alloc, fixture.sender, nil,
					)

					const gas = uint64(150_000)
					receipt := fixture.write(
						t, tr, nil, gas, 11, []byte{0x00},
					)
					require.Equal(t, types.ReceiptFailed, *receipt.Status)
					require.Equal(t, gas, receipt.GasUsed)
					require.Empty(t, receipt.Logs)

					require.Equal(t, uint64(1), tr.GetNonce(fixture.sender))
					require.Zero(t, tr.GetBalance(fixture.sender).Cmp(
						big.NewInt(10_000_000-int64(gas)),
					))

					snapshot := fixture.reopen(t, tr)
					requireCreationRollbackDestination(
						t, snapshot, destination, prefunded,
					)

					sender, err := snapshot.GetAccount(fixture.sender)
					require.NoError(t, err)
					require.NotNil(t, sender)
					require.Equal(t, uint64(1), sender.Nonce)
					require.Zero(t, sender.Balance.Cmp(
						big.NewInt(10_000_000-int64(gas)),
					))
				})
			}
		})
	}
}

func TestCreationRollback_InternalFailureHandledByParent(t *testing.T) {
	for _, policy := range []string{"allowlist", "blocklist"} {
		t.Run(policy, func(t *testing.T) {
			for _, create2 := range []bool{false, true} {
				opName := "CREATE"
				if create2 {
					opName = "CREATE2"
				}

				t.Run(opName, func(t *testing.T) {
					for _, prefunded := range []bool{false, true} {
						name := "new_destination"
						if prefunded {
							name = "prefunded_destination"
						}

						t.Run(name, func(t *testing.T) {
							fixture := newCreationRollbackFixture(t)
							parent := types.StringToAddress("0x9202")
							child := crypto.CreateAddress(parent, 0)

							// Empty initcode, value 11. Record ISZERO(CREATE result)
							// in slot zero, then successfully stop the parent.
							code := []byte{
								0x60, 0x00,
								0x60, 0x00,
								0x60, 0x0b,
								0xf0,
								0x15, 0x60, 0x00, 0x55, 0x00,
							}
							if create2 {
								child = crypto.CreateAddress2(parent, types.ZeroHash, nil)
								code = []byte{
									0x60, 0x00, // salt
									0x60, 0x00, // length
									0x60, 0x00, // offset
									0x60, 0x0b, // value
									0xf5,
									0x15, 0x60, 0x00, 0x55, 0x00,
								}
							}

							alloc := map[types.Address]*chain.GenesisAccount{
								parent: {
									Balance: big.NewInt(1000),
									Code:    code,
								},
							}
							if prefunded {
								alloc[child] = &chain.GenesisAccount{
									Balance: big.NewInt(55),
								}
							}

							tr := fixture.transition(
								t, policy, alloc, parent, &fixture.sender,
							)

							// EIP150 leaves sufficient parent gas for the SSTORE
							// after the rejected child consumes its forwarded gas.
							const gas = uint64(2_000_000)
							receipt := fixture.write(t, tr, &parent, gas, 0, nil)
							require.Equal(t, types.ReceiptSuccess, *receipt.Status)
							require.Greater(t, receipt.GasUsed, uint64(21_000))
							require.Less(t, receipt.GasUsed, gas)

							require.Equal(t, uint64(1), tr.GetNonce(parent))
							require.Zero(t, tr.GetBalance(parent).Cmp(big.NewInt(1000)))
							require.Equal(
								t,
								types.StringToHash("0x01"),
								tr.GetStorage(parent, types.ZeroHash),
							)

							snapshot := fixture.reopen(t, tr)
							requireCreationRollbackDestination(
								t, snapshot, child, prefunded,
							)

							parentAccount, err := snapshot.GetAccount(parent)
							require.NoError(t, err)
							require.NotNil(t, parentAccount)
							require.Equal(t, uint64(1), parentAccount.Nonce)
							require.Zero(t, parentAccount.Balance.Cmp(big.NewInt(1000)))

							stored, err := snapshot.GetStorage(
								parent, parentAccount.Root, types.ZeroHash,
							)
							require.NoError(t, err)
							require.Equal(t, types.StringToHash("0x01"), stored)

							sender, err := snapshot.GetAccount(fixture.sender)
							require.NoError(t, err)
							require.NotNil(t, sender)
							require.Equal(t, uint64(1), sender.Nonce)
							require.Zero(t, sender.Balance.Cmp(
								new(big.Int).Sub(
									big.NewInt(10_000_000),
									new(big.Int).SetUint64(receipt.GasUsed),
								),
							))
						})
					}
				})
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3037 -- verify rejected creation through execution and MDBX reopen -- */
