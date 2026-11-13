/* START ALPHA CHANGE 2983 -- verify progression snapshot isolation and concurrent readers -- */
package progress

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProgressionSnapshot_Isolation(t *testing.T) {
	pw := &ProgressionWrapper{
		progression: &Progression{
			/* START ALPHA CHANGE 3268 -- exercise shared progression snapshots using active synchronization -- */
			SyncType: ChainSyncBulk,
			/* FINISH ALPHA CHANGE 3268 -- exercise shared progression snapshots using active synchronization -- */
			StartingBlock: 1,
			CurrentBlock:  2,
			HighestBlock:  3,
		},
	}

	snapshot := pw.GetProgression()
	require.NotNil(t, snapshot)

	pw.UpdateCurrentProgression(4)
	pw.UpdateHighestProgression(5)
	require.Equal(t, uint64(2), snapshot.CurrentBlock)
	require.Equal(t, uint64(3), snapshot.HighestBlock)

	snapshot.StartingBlock = 99
	snapshot.CurrentBlock = 99
	snapshot.HighestBlock = 99

	current := pw.GetProgression()
	require.Equal(t, uint64(1), current.StartingBlock)
	require.Equal(t, uint64(4), current.CurrentBlock)
	require.Equal(t, uint64(5), current.HighestBlock)

	require.Nil(t, (&ProgressionWrapper{}).GetProgression())
}

func TestProgressionSnapshot_ConcurrentReaders(t *testing.T) {
	pw := &ProgressionWrapper{
		progression: &Progression{
			/* START ALPHA CHANGE 3268 -- exercise shared progression snapshots using active synchronization -- */
			SyncType: ChainSyncBulk,
			/* FINISH ALPHA CHANGE 3268 -- exercise shared progression snapshots using active synchronization -- */
			StartingBlock: 1,
		},
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(3)

	go func() {
		defer workers.Done()
		<-start
		for height := uint64(1); height <= 1000; height++ {
			pw.UpdateCurrentProgression(height)
			pw.UpdateHighestProgression(height + 1)
		}
	}()

	for reader := 0; reader < 2; reader++ {
		go func() {
			defer workers.Done()
			<-start
			for iteration := 0; iteration < 1000; iteration++ {
				snapshot := pw.GetProgression()
				// Each reader owns and may change its returned object.
				snapshot.StartingBlock = 99
				snapshot.CurrentBlock = 99
				snapshot.HighestBlock = 99
			}
		}()
	}

	close(start)
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("progression snapshot workers did not finish")
	}

	current := pw.GetProgression()
	require.Equal(t, uint64(1), current.StartingBlock)
	require.Equal(t, uint64(1000), current.CurrentBlock)
	require.Equal(t, uint64(1001), current.HighestBlock)
}

/* FINISH ALPHA CHANGE 2983 -- verify progression snapshot isolation and concurrent readers -- */
