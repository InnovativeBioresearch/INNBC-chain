/* START ALPHA CHANGE 3885 -- test recovery of canonical balances beyond 256 bits -- */
package itrie

import (
	"math/big"
	"testing"

	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func TestSnapshotBalance_CanonicalWidths(t *testing.T) {
	limit := new(big.Int).Lsh(big.NewInt(1), 256)

	tests := []struct {
		name    string
		balance *big.Int
	}{
		{"zero", new(big.Int)},
		{"one", big.NewInt(1)},
		{"maximum_uint256", new(big.Int).Sub(limit, big.NewInt(1))},
		{"two_to_256", new(big.Int).Set(limit)},
		{"two_to_264", new(big.Int).Lsh(big.NewInt(1), 264)},
		{"two_to_512", new(big.Int).Lsh(big.NewInt(1), 512)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := snapshotTestAccountRLP(
				nil,
				test.balance.Bytes(),
				types.EmptyRootHash.Bytes(),
				types.EmptyCodeHash.Bytes(),
				false,
			)

			account, err := decodeStrictSnapshotAccount(raw)
			require.NoError(t, err)
			require.NotNil(t, account)
			require.Zero(t, account.Balance.Cmp(test.balance))

			// Recovery must preserve the exact canonical account encoding.
			arena := &fastrlp.Arena{}
			require.Equal(t, raw, account.MarshalWith(arena).MarshalTo(nil))
		})
	}
}

func TestSnapshotBalance_ZeroAddressTransferSurvivesRecovery(t *testing.T) {
	source := NewMemoryStorage()
	target := NewMemoryStorage()
	t.Cleanup(func() {
		require.NoError(t, target.Close())
		require.NoError(t, source.Close())
	})

	limit := new(big.Int).Lsh(big.NewInt(1), 256)
	maximum := new(big.Int).Sub(limit, big.NewInt(1))
	sender := types.StringToAddress("0x1001")

	// Persist the genesis-like balances before applying the transfer.
	initialSnapshot := NewState(source).NewSnapshot()
	initialTxn := statepkg.NewTxn(initialSnapshot)
	initialTxn.SetBalance(types.ZeroAddress, maximum)
	initialTxn.SetBalance(sender, big.NewInt(1))

	objects, err := initialTxn.Commit(false)
	require.NoError(t, err)

	_, initialRootBytes, err := initialSnapshot.Commit(objects)
	require.NoError(t, err)

	initialRoot := types.BytesToHash(initialRootBytes)
	require.NoError(t, ValidateCompleteState(initialRoot, source))

	// Use a fresh state instance to read the persisted initial balances.
	transferSnapshot, err := NewState(source).NewSnapshotAt(initialRoot)
	require.NoError(t, err)

	// Apply the balance changes of a one-wei transfer to zero.
	transferTxn := statepkg.NewTxn(transferSnapshot)
	require.NoError(t, transferTxn.SubBalance(sender, big.NewInt(1)))
	transferTxn.AddBalance(types.ZeroAddress, big.NewInt(1))
	transferTxn.SetNonce(sender, 1)

	objects, err = transferTxn.Commit(false)
	require.NoError(t, err)

	_, rootBytes, err := transferSnapshot.Commit(objects)
	require.NoError(t, err)
	root := types.BytesToHash(rootBytes)

	// Live serialization must retain the exact 257-bit balance.
	liveSnapshot, err := NewState(source).NewSnapshotAt(root)
	require.NoError(t, err)

	liveAccount, err := liveSnapshot.GetAccount(types.ZeroAddress)
	require.NoError(t, err)
	require.NotNil(t, liveAccount)
	require.Zero(t, liveAccount.Balance.Cmp(limit))
	require.Len(t, liveAccount.Balance.Bytes(), 33)

	arena := &fastrlp.Arena{}
	expectedAccountRLP := liveAccount.MarshalWith(arena).MarshalTo(nil)

	// Exercise the shared recovery validator and snapshot-copy path.
	require.NoError(t, ValidateCompleteState(root, source))
	require.NoError(t, CopyTrie(root.Bytes(), source, target, nil, false))
	require.NoError(t, ValidateCompleteState(root, target))

	// Reopen the independent destination using the original state root.
	recoveredSnapshot, err := NewState(target).NewSnapshotAt(root)
	require.NoError(t, err)

	recoveredAccount, err := recoveredSnapshot.GetAccount(types.ZeroAddress)
	require.NoError(t, err)
	require.NotNil(t, recoveredAccount)
	require.Zero(t, recoveredAccount.Balance.Cmp(limit))

	recoveredArena := &fastrlp.Arena{}
	require.Equal(t,
		expectedAccountRLP,
		recoveredAccount.MarshalWith(recoveredArena).MarshalTo(nil),
	)

	recoveredSender, err := recoveredSnapshot.GetAccount(sender)
	require.NoError(t, err)
	require.NotNil(t, recoveredSender)
	require.Zero(t, recoveredSender.Balance.Sign())
	require.Equal(t, uint64(1), recoveredSender.Nonce)
}

func TestSnapshotBalance_RetainsStrictIntegerChecks(t *testing.T) {
	wideBalance := new(big.Int).Lsh(big.NewInt(1), 256).Bytes()

	tests := []struct {
		name    string
		nonce   []byte
		balance []byte
		message string
	}{
		{
			name:    "nonce_exceeds_eight_bytes",
			nonce:   append([]byte{1}, make([]byte, 8)...),
			balance: wideBalance,
			message: "snapshot account nonce: snapshot integer exceeds 8 bytes",
		},
		{
			name:    "wide_balance_has_leading_zero",
			balance: append([]byte{0}, wideBalance...),
			message: "snapshot account balance: snapshot integer has a leading zero",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := snapshotTestAccountRLP(
				test.nonce,
				test.balance,
				types.EmptyRootHash.Bytes(),
				types.EmptyCodeHash.Bytes(),
				false,
			)

			_, err := decodeStrictSnapshotAccount(raw)
			require.ErrorContains(t, err, test.message)
		})
	}
}

/* FINISH ALPHA CHANGE 3885 -- test recovery of canonical balances beyond 256 bits -- */
