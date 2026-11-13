/* START ALPHA CHANGE 2152 -- test failed account reads and retained publication errors -- */
package state

import (
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/innovativebioresearch/innbc-chain/types"
)

type issue7AAccountReadSnapshot struct {
	accountErr error
	commitErr  error
	commits    int
}

func (s *issue7AAccountReadSnapshot) GetAccount(
	types.Address,
) (*Account, error) {
	return nil, s.accountErr
}

func (s *issue7AAccountReadSnapshot) GetStorage(
	types.Address,
	types.Hash,
	types.Hash,
) (types.Hash, error) {
	return types.ZeroHash, nil
}

func (s *issue7AAccountReadSnapshot) GetCode(
	hash types.Hash,
) ([]byte, bool, error) {
	if hash == types.EmptyCodeHash {
		return []byte{}, true, nil
	}

	return nil, false, nil
}

func (s *issue7AAccountReadSnapshot) Commit(
	[]*Object,
) (Snapshot, []byte, error) {
	s.commits++

	if s.commitErr != nil {
		return nil, nil, s.commitErr
	}

	return s, types.EmptyRootHash.Bytes(), nil
}

func TestIssue7A_FailedAccountReadDoesNotCreateObject(t *testing.T) {
	for _, operation := range []string{"create", "add balance", "set storage"} {
		t.Run(operation, func(t *testing.T) {
			injected := errors.New("injected account read failure")
			snapshot := &issue7AAccountReadSnapshot{accountErr: injected}
			txn := NewTxn(snapshot)
			address := types.StringToAddress("0x5001")
			checkpoint := txn.Snapshot()

			switch operation {
			case "create":
				txn.CreateAccount(address)
			case "add balance":
				txn.AddBalance(address, big.NewInt(1))
			case "set storage":
				txn.SetState(
					address,
					types.StringToHash("0x01"),
					types.StringToHash("0x2a"),
				)
			}

			require.ErrorIs(t, txn.Error(), injected)
			require.ErrorIs(t, txn.Error(), ErrLocalStateFailure)

			_, exists := txn.GetRadix().Get(address.Bytes())
			require.False(t, exists)

			first := txn.Error()
			require.NoError(t, txn.RevertToSnapshot(checkpoint))
			require.Equal(t, first, txn.Error())

			// A later successful backend read must not make this failed
			// transaction usable for account mutation.
			snapshot.accountErr = nil
			txn.CreateAccount(address)

			_, exists = txn.GetRadix().Get(address.Bytes())
			require.False(t, exists)

			objects, err := txn.Commit(false)
			require.Nil(t, objects)
			require.Equal(t, first, err)
		})
	}
}

func TestIssue7A_GenuinelyAbsentAccountCanBeCreated(t *testing.T) {
	txn := NewTxn(&issue7AAccountReadSnapshot{})
	address := types.StringToAddress("0x5002")

	txn.CreateAccount(address)
	require.NoError(t, txn.Error())

	account, exists := txn.GetAccount(address)
	require.True(t, exists)
	require.NotNil(t, account)
	require.Zero(t, account.Balance.Sign())
	require.Equal(t, uint64(0), account.Nonce)
}

func TestIssue7A_TransitionCommitFailureCannotBeRetried(t *testing.T) {
	injected := errors.New("injected snapshot publication failure")
	snapshot := &issue7AAccountReadSnapshot{commitErr: injected}
	txn := NewTxn(snapshot)

	transition := &Transition{
		state: txn,
		snap:  snapshot,
	}

	result, root, err := transition.Commit()
	require.Nil(t, result)
	require.Equal(t, types.ZeroHash, root)
	require.ErrorIs(t, err, injected)
	require.ErrorIs(t, err, ErrLocalStateFailure)
	require.Equal(t, 1, snapshot.commits)

	first := txn.Error()
	require.ErrorIs(t, first, injected)

	// Repairing the backend does not rehabilitate this execution object.
	snapshot.commitErr = nil

	result, root, err = transition.Commit()
	require.Nil(t, result)
	require.Equal(t, types.ZeroHash, root)
	require.Equal(t, first, err)
	require.Equal(t, 1, snapshot.commits)
}

/* FINISH ALPHA CHANGE 2152 -- test failed account reads and retained publication errors -- */
