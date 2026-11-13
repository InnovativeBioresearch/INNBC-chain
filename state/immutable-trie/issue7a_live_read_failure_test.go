/* START ALPHA CHANGE 2178 -- test malformed persisted values through ordinary state reads -- */
package itrie

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func issue7APersistRawValue(
	t *testing.T,
	storage Storage,
	key []byte,
	value []byte,
) types.Hash {
	t.Helper()

	batch := storage.Batch()
	txn := NewTrie().Txn(storage)
	txn.batch = batch
	txn.Insert(key, value)
	require.NoError(t, txn.Error())

	root, err := txn.Hash()
	require.NoError(t, err)
	require.NoError(t, batch.Write())

	return types.BytesToHash(root)
}

func TestIssue7A_LiveAccountReadRejectsMalformedStoredValue(t *testing.T) {
	for _, value := range [][]byte{
		/* START ALPHA CHANGE 2194 -- reject matching account leaves with empty payloads -- */
		nil,
		{},
		/* FINISH ALPHA CHANGE 2194 -- reject matching account leaves with empty payloads -- */
		{0xc0}, // Valid RLP list, but missing account fields.
		{0xff}, // Invalid/truncated RLP.
	} {
		storage := NewMemoryStorage()
		address := types.StringToAddress("0x7801")
		root := issue7APersistRawValue(
			t,
			storage,
			crypto.Keccak256(address.Bytes()),
			value,
		)

		snapshot, err := NewState(storage).NewSnapshotAt(root)
		require.NoError(t, err)

		account, err := snapshot.GetAccount(address)
		require.Nil(t, account)
		require.ErrorIs(t, err, statepkg.ErrLocalStateFailure)
		require.ErrorIs(t, err, statepkg.ErrMalformedAccount)
	}
}

func TestIssue7A_LiveStorageReadRejectsMalformedStoredValue(t *testing.T) {
	for _, value := range [][]byte{
		/* START ALPHA CHANGE 2194 -- reject matching storage leaves with empty payloads -- */
		nil,
		{},
		/* FINISH ALPHA CHANGE 2194 -- reject matching storage leaves with empty payloads -- */
		{0xc0}, // Storage value must be bytes, not an RLP list.
		{0xff}, // Invalid/truncated RLP.
	} {
		storage := NewMemoryStorage()
		address := types.StringToAddress("0x7802")
		slot := types.StringToHash("0x01")
		root := issue7APersistRawValue(
			t,
			storage,
			crypto.Keccak256(slot.Bytes()),
			value,
		)

		snapshot := NewState(storage).NewSnapshot()
		result, err := snapshot.GetStorage(address, root, slot)
		require.Equal(t, types.ZeroHash, result)
		require.ErrorIs(t, err, statepkg.ErrLocalStateFailure)
		require.ErrorIs(t, err, statepkg.ErrMalformedStorage)
	}
}

/* FINISH ALPHA CHANGE 2178 -- test malformed persisted values through ordinary state reads -- */
