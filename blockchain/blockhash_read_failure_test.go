/* START ALPHA CHANGE 3812 -- verify BLOCKHASH header failures retain their causes -- */

package blockchain

import (
	"errors"
	"math"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestBlockHashReadFailure_PreservesCauseAndZeroSemantics(t *testing.T) {
	for _, cause := range []error{
		errors.New("injected ancestor backend failure"),
		storage.ErrNotFound,
		storage.ErrHeaderHashMismatch,
	} {
		t.Run(cause.Error(), func(t *testing.T) {
			headers := blockHashCacheHeaders(300, 1)
			fixture := newBlockHashCacheFixture(t, headers)
			db := fixture.blockchain.db.(*storage.MockStorage)

			reads := 0
			db.HookReadHeader(func(types.Hash) (*types.Header, error) {
				reads++

				return nil, cause
			})

			lookup := fixture.blockchain.GetHashHelper(headers[300])

			// The parent hash is already supplied by the candidate header.
			hash, err := lookup(299)
			require.NoError(t, err)
			require.Equal(t, headers[299].Hash, hash)

			for _, number := range []uint64{300, 301, 43, math.MaxUint64} {
				hash, err = lookup(number)
				require.NoError(t, err)
				require.Equal(t, types.ZeroHash, hash)
			}
			require.Zero(t, reads)

			for attempt := 1; attempt <= 2; attempt++ {
				hash, err = lookup(44)
				require.Equal(t, types.ZeroHash, hash)
				require.ErrorIs(t, err, cause)
				require.ErrorContains(t, err, "BLOCKHASH(44)")
				require.Equal(t, attempt, reads)
			}

			// Failed extension must not invalidate the known parent.
			hash, err = lookup(299)
			require.NoError(t, err)
			require.Equal(t, headers[299].Hash, hash)
			require.Equal(t, 2, reads)
		})
	}
}

func TestBlockHashReadFailure_InvalidCachedHeaders(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"wrong type", "invalid header cache entry"},
		{"nil header", (*types.Header)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := blockHashCacheHeaders(300, 1)
			fixture := newBlockHashCacheFixture(t, headers)
			hash := headers[299].Hash
			fixture.blockchain.headersCache.Add(hash, tc.value)

			header, err := fixture.blockchain.readHeaderWithError(hash)
			require.Nil(t, header)
			require.ErrorContains(t, err, "invalid cached header")

			header, ok := fixture.blockchain.readHeader(hash)
			require.Nil(t, header)
			require.False(t, ok)
			require.Zero(t, fixture.reads)
		})
	}
}

func TestBlockHashReadFailure_NilBackendHeader(t *testing.T) {
	headers := blockHashCacheHeaders(300, 1)
	fixture := newBlockHashCacheFixture(t, headers)
	db := fixture.blockchain.db.(*storage.MockStorage)
	db.HookReadHeader(func(types.Hash) (*types.Header, error) {
		return nil, nil
	})

	lookup := fixture.blockchain.GetHashHelper(headers[300])
	hash, err := lookup(298)

	require.Equal(t, types.ZeroHash, hash)
	require.ErrorContains(t, err, "missing header")
}

/* FINISH ALPHA CHANGE 3812 -- verify BLOCKHASH header failures retain their causes -- */
