/* START ALPHA CHANGE 3172 -- verify recent startup does not hide damaged segments or missing genesis -- */
package segmented

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-hclog"
	bcstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/stretchr/testify/require"
)

type startupFailingSegment struct {
	bcstorage.Storage
	cause error
}

func (s *startupFailingSegment) ReadStartupState() (bcstorage.StartupState, error) {
	return bcstorage.StartupState{}, s.cause
}

func newStartupSegmentFixture(t *testing.T) *Storage {
	t.Helper()
	s, err := NewStorage(t.TempDir(), hclog.NewNullLogger(), 2, 4, 256*1024*1024)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	return s
}

func TestStartupMetadata_SegmentedEmptyAndPruned(t *testing.T) {
	s := newStartupSegmentFixture(t)
	startup, err := s.ReadStartupState()
	require.NoError(t, err)
	require.True(t, startup.Empty)

	for number := uint64(0); number <= 5; number++ {
		require.NoError(t, writeTestCanonicalHeader(s, number))
	}
	startup, err = s.ReadStartupState()
	require.NoError(t, err)
	require.False(t, startup.Empty)
	require.True(t, startup.HasGenesis)
	require.Equal(t, uint64(5), startup.Head.Number)
	_, retained := s.ReadCanonicalHash(1)
	require.False(t, retained)

	// A genuinely empty newly opened segment must not replace the valid head.
	s.lock.Lock()
	_, err = s.openSegmentLocked(7)
	s.lock.Unlock()
	require.NoError(t, err)
	startup, err = s.ReadStartupState()
	require.NoError(t, err)
	require.Equal(t, uint64(5), startup.Head.Number)
}

func TestStartupMetadata_SegmentedReadFailureDoesNotFallBack(t *testing.T) {
	s := newStartupSegmentFixture(t)
	for number := uint64(0); number <= 5; number++ {
		require.NoError(t, writeTestCanonicalHeader(s, number))
	}

	cause := errors.New("injected latest-segment startup read failure")
	s.segments[5] = &startupFailingSegment{Storage: s.segments[5], cause: cause}

	_, err := s.ReadStartupState()
	require.ErrorIs(t, err, cause)
	require.ErrorContains(t, err, "segment 5")
}

func TestStartupMetadata_SegmentedRecordsRequireGenesis(t *testing.T) {
	s := newStartupSegmentFixture(t)
	for number := uint64(0); number <= 5; number++ {
		require.NoError(t, writeTestCanonicalHeader(s, number))
	}

	genesis, found := s.ReadCanonicalHash(0)
	require.True(t, found)
	batch := s.segments[0].NewBatch()
	for _, key := range [][]byte{
		append(append([]byte(nil), bcstorage.HEAD...), bcstorage.HASH...),
		append(append([]byte(nil), bcstorage.HEAD...), bcstorage.NUMBER...),
		append(append([]byte(nil), bcstorage.CANONICAL...), common.EncodeUint64ToBytes(0)...),
		append(append([]byte(nil), bcstorage.HEADER...), genesis.Bytes()...),
	} {
		batch.Delete(key)
	}
	require.NoError(t, batch.Write())

	_, err := s.ReadStartupState()
	require.ErrorContains(t, err, "permanent genesis storage is empty")
}

/* FINISH ALPHA CHANGE 3172 -- verify recent startup does not hide damaged segments or missing genesis -- */
