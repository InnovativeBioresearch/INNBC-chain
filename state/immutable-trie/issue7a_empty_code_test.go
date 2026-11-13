/* START ALPHA CHANGE 2187 -- test empty referenced bytecode and canonical empty code -- */
package itrie

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestIssue7A_StateRejectsPresentEmptyCode(t *testing.T) {
	storage := NewMemoryStorage()
	world := NewState(storage)

	// A single STOP instruction is valid, nonempty bytecode.
	validCode := []byte{0x00}
	codeHash := types.BytesToHash(crypto.Keccak256(validCode))
	require.NotEqual(t, types.EmptyCodeHash, codeHash)

	// Model a damaged record that exists but has no bytes.
	require.NoError(t, storage.SetCode(codeHash, []byte{}))

	raw, found, err := storage.GetCode(codeHash)
	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, raw)

	code, found, err := world.GetCode(codeHash)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.ErrorIs(t, err, state.ErrMissingCode)
	require.False(t, found)
	require.Nil(t, code)

	// Canonical empty code needs no stored bytecode record.
	code, found, err = world.GetCode(types.EmptyCodeHash)
	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, code)

	// A successful subsequent read must return the actual code.
	require.NoError(t, storage.SetCode(codeHash, validCode))

	code, found, err = world.GetCode(codeHash)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, validCode, code)
}

/* FINISH ALPHA CHANGE 2187 -- test empty referenced bytecode and canonical empty code -- */
