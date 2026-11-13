/* START ALPHA CHANGE 2084 -- test bounded historical transaction index encoding -- */
package snapshotbuilder

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestTransactionIndexRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), archiveIndexFileName)

	records := []TransactionIndexRecord{
		{
			Hash:             archiveIndexTestHash(3),
			BlockNumber:      8,
			TransactionIndex: 2,
		},
		{
			Hash:             archiveIndexTestHash(1),
			BlockNumber:      2,
			TransactionIndex: 0,
		},
		{
			Hash:             archiveIndexTestHash(2),
			BlockNumber:      5,
			TransactionIndex: 1,
		},
	}

	require.NoError(t, writeTransactionIndex(path, records, 10))

	decoded, err := readTransactionIndex(path, 10)
	require.NoError(t, err)
	require.Equal(t, []TransactionIndexRecord{
		{
			Hash:             archiveIndexTestHash(1),
			BlockNumber:      2,
			TransactionIndex: 0,
		},
		{
			Hash:             archiveIndexTestHash(2),
			BlockNumber:      5,
			TransactionIndex: 1,
		},
		{
			Hash:             archiveIndexTestHash(3),
			BlockNumber:      8,
			TransactionIndex: 2,
		},
	}, decoded)
}

func TestTransactionIndexRejectsDuplicateHashes(t *testing.T) {
	path := filepath.Join(t.TempDir(), archiveIndexFileName)
	hash := archiveIndexTestHash(1)

	err := writeTransactionIndex(
		path,
		[]TransactionIndexRecord{
			{
				Hash:             hash,
				BlockNumber:      1,
				TransactionIndex: 0,
			},
			{
				Hash:             hash,
				BlockNumber:      2,
				TransactionIndex: 0,
			},
		},
		10,
	)

	require.ErrorContains(t, err, "duplicate archived transaction hash")
	require.NoFileExists(t, path)
}

func TestTransactionIndexRejectsRecordLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), archiveIndexFileName)

	err := writeTransactionIndex(
		path,
		[]TransactionIndexRecord{
			{
				Hash:             archiveIndexTestHash(1),
				BlockNumber:      1,
				TransactionIndex: 0,
			},
			{
				Hash:             archiveIndexTestHash(2),
				BlockNumber:      1,
				TransactionIndex: 1,
			},
		},
		1,
	)

	require.ErrorContains(t, err, "exceeding configured maximum")
	require.NoFileExists(t, path)
}

func TestTransactionIndexRejectsCorruptEncoding(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(t *testing.T, path string)
		errorText string
	}{
		{
			name: "invalid magic",
			mutate: func(t *testing.T, path string) {
				t.Helper()

				file, err := os.OpenFile(path, os.O_WRONLY, 0)
				require.NoError(t, err)
				_, err = file.WriteAt([]byte("BADMAGIC"), 0)
				require.NoError(t, err)
				require.NoError(t, file.Close())
			},
			errorText: "magic is invalid",
		},
		{
			name: "unsupported version",
			mutate: func(t *testing.T, path string) {
				t.Helper()

				var encoded [8]byte
				binary.BigEndian.PutUint64(
					encoded[:],
					TransactionIndexVersion+1,
				)

				file, err := os.OpenFile(path, os.O_WRONLY, 0)
				require.NoError(t, err)
				_, err = file.WriteAt(encoded[:], 8)
				require.NoError(t, err)
				require.NoError(t, file.Close())
			},
			errorText: "unsupported transaction index version",
		},
		{
			name: "nonzero reserved bytes",
			mutate: func(t *testing.T, path string) {
				t.Helper()

				file, err := os.OpenFile(path, os.O_WRONLY, 0)
				require.NoError(t, err)
				_, err = file.WriteAt([]byte{1}, 24)
				require.NoError(t, err)
				require.NoError(t, file.Close())
			},
			errorText: "reserved header bytes are not zero",
		},
		{
			name: "truncated record",
			mutate: func(t *testing.T, path string) {
				t.Helper()

				info, err := os.Stat(path)
				require.NoError(t, err)
				require.NoError(t, os.Truncate(path, info.Size()-1))
			},
			errorText: "size mismatch",
		},
		{
			name: "trailing data",
			mutate: func(t *testing.T, path string) {
				t.Helper()

				file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
				require.NoError(t, err)
				_, err = file.Write([]byte{1})
				require.NoError(t, err)
				require.NoError(t, file.Close())
			},
			errorText: "size mismatch",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), archiveIndexFileName)

			require.NoError(t, writeTransactionIndex(
				path,
				[]TransactionIndexRecord{
					{
						Hash:             archiveIndexTestHash(1),
						BlockNumber:      1,
						TransactionIndex: 0,
					},
				},
				10,
			))

			test.mutate(t, path)

			_, err := readTransactionIndex(path, 10)
			require.ErrorContains(t, err, test.errorText)
		})
	}
}

func TestTransactionIndexRejectsUnsupportedLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), archiveIndexFileName)

	require.Error(
		t,
		writeTransactionIndex(path, nil, 0),
	)
	require.Error(
		t,
		writeTransactionIndex(
			path,
			nil,
			MaximumArchiveIndexRecords+1,
		),
	)
	require.Error(
		t,
		func() error {
			_, err := readTransactionIndex(path, 0)

			return err
		}(),
	)
}

func archiveIndexTestHash(value byte) types.Hash {
	var hash types.Hash
	hash[len(hash)-1] = value

	return hash
}

/* FINISH ALPHA CHANGE 2084 -- test bounded historical transaction index encoding -- */
