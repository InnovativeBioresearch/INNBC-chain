package state

/* START ALPHA CHANGE 2153 -- remove the obsolete missing-account error import -- */
import (
	"math"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/* FINISH ALPHA CHANGE 2153 -- remove the obsolete missing-account error import -- */

type mockSnapshot struct {
	state map[types.Address]*PreState
}

/* START ALPHA CHANGE 2153 -- migrate the healthy snapshot storage fixture -- */
func (m *mockSnapshot) GetStorage(addr types.Address, root types.Hash, key types.Hash) (types.Hash, error) {
	raw, ok := m.state[addr]
	if !ok {
		return types.Hash{}, nil
	}

	res, ok := raw.State[key]
	if !ok {
		return types.Hash{}, nil
	}

	return res, nil
}

/* FINISH ALPHA CHANGE 2153 -- migrate the healthy snapshot storage fixture -- */

/* START ALPHA CHANGE 2153 -- represent healthy account absence and empty code correctly -- */
func (m *mockSnapshot) GetAccount(addr types.Address) (*Account, error) {
	raw, ok := m.state[addr]
	if !ok {
		return nil, nil
	}

	acct := &Account{
		Balance:  new(big.Int).SetUint64(raw.Balance),
		Nonce:    raw.Nonce,
		Root:     emptyStateHash,
		CodeHash: types.EmptyCodeHash.Bytes(),
	}

	return acct, nil
}

/* FINISH ALPHA CHANGE 2153 -- represent healthy account absence and empty code correctly -- */

/* START ALPHA CHANGE 2153 -- represent canonical empty code as a successful read -- */
func (m *mockSnapshot) GetCode(hash types.Hash) ([]byte, bool, error) {
	if hash == types.EmptyCodeHash {
		return []byte{}, true, nil
	}

	return nil, false, nil
}

/* FINISH ALPHA CHANGE 2153 -- represent canonical empty code as a successful read -- */

func (m *mockSnapshot) Commit(objs []*Object) (Snapshot, []byte, error) {
	return nil, nil, nil
}

func newStateWithPreState(preState map[types.Address]*PreState) Snapshot {
	return &mockSnapshot{state: preState}
}

func newTestTxn(p map[types.Address]*PreState) *Txn {
	return newTxn(newStateWithPreState(p))
}

func TestSnapshotUpdateData(t *testing.T) {
	txn := newTestTxn(defaultPreState)

	txn.SetState(addr1, hash1, hash1)
	assert.Equal(t, hash1, txn.GetState(addr1, hash1))

	ss := txn.Snapshot()
	txn.SetState(addr1, hash1, hash2)
	assert.Equal(t, hash2, txn.GetState(addr1, hash1))

	assert.NoError(t, txn.RevertToSnapshot(ss))
	assert.Equal(t, hash1, txn.GetState(addr1, hash1))
}

func TestIncrNonce(t *testing.T) {
	t.Parallel()

	var (
		address0               = types.StringToAddress("0")
		address1               = types.StringToAddress("1")
		maxUint64NonceValue    = uint64(math.MaxUint64)
		nonMaxUint64NonceValue = uint64(3)
	)

	txn := newTestTxn(defaultPreState)

	txn.SetNonce(address0, maxUint64NonceValue)
	txn.SetNonce(address1, nonMaxUint64NonceValue)

	require.Error(t, txn.IncrNonce(address0))
	require.NoError(t, txn.IncrNonce(address1))
	require.Equal(t, nonMaxUint64NonceValue+1, txn.GetNonce(address1))
}
