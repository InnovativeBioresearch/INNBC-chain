/* START ALPHA CHANGE 2187 -- test empty bytecode failure retention before caching -- */
package state

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type issue7AEmptyCodeSnapshot struct {
	Snapshot

	account *Account
	code    []byte
	reads   int
}

func (s *issue7AEmptyCodeSnapshot) GetAccount(types.Address) (*Account, error) {
	return s.account.Copy(), nil
}

func (s *issue7AEmptyCodeSnapshot) GetCode(types.Hash) ([]byte, bool, error) {
	s.reads++

	return s.code, true, nil
}

func TestIssue7A_TxnRejectsPresentEmptyCode(t *testing.T) {
	address := types.StringToAddress("0x7401")
	codeHash := types.BytesToHash(crypto.Keccak256([]byte{0x00}))

	for _, testCase := range []struct {
		name string
		code []byte
	}{
		{name: "nil record", code: nil},
		{name: "empty record", code: []byte{}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			snapshot := &issue7AEmptyCodeSnapshot{
				account: &Account{
					Balance:  big.NewInt(0),
					Root:     types.EmptyRootHash,
					CodeHash: codeHash.Bytes(),
				},
				code: testCase.code,
			}
			txn := NewTxn(snapshot)
			rollback := txn.Snapshot()

			require.Nil(t, txn.GetCode(address))
			require.Equal(t, 1, snapshot.reads)
			require.ErrorIs(t, txn.Error(), ErrLocalStateFailure)
			require.ErrorIs(t, txn.Error(), ErrMissingCode)

			_, cached := txn.codeCache.Get(address)
			require.False(t, cached)

			firstError := txn.Error()
			require.NoError(t, txn.RevertToSnapshot(rollback))
			require.Equal(t, firstError, txn.Error())

			objects, err := txn.Commit(false)
			require.ErrorIs(t, err, ErrLocalStateFailure)
			require.ErrorIs(t, err, ErrMissingCode)
			require.Nil(t, objects)
		})
	}
}

func TestIssue7A_TxnPreservesSuccessfulCodeReads(t *testing.T) {
	address := types.StringToAddress("0x7402")
	validCode := []byte{0x00}

	for _, testCase := range []struct {
		name string
		hash types.Hash
		code []byte
	}{
		{
			name: "canonical empty code",
			hash: types.EmptyCodeHash,
			code: []byte{},
		},
		{
			name: "nonempty STOP code",
			hash: types.BytesToHash(crypto.Keccak256(validCode)),
			code: validCode,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			snapshot := &issue7AEmptyCodeSnapshot{
				account: &Account{
					Balance:  big.NewInt(0),
					Root:     types.EmptyRootHash,
					CodeHash: testCase.hash.Bytes(),
				},
				code: testCase.code,
			}
			txn := NewTxn(snapshot)

			require.Equal(t, testCase.code, txn.GetCode(address))
			require.NoError(t, txn.Error())

			require.Equal(t, testCase.code, txn.GetCode(address))
			require.NoError(t, txn.Error())
			require.Equal(t, 1, snapshot.reads)
		})
	}
}

/* FINISH ALPHA CHANGE 2187 -- test empty bytecode failure retention before caching -- */
