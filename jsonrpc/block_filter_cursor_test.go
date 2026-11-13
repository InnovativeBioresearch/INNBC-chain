package jsonrpc

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2556 -- test concurrent block-filter cursor consumption --

Concurrent polls share one filter. Across all responses and a final drain,
every published block must appear exactly once. Each individual caller must
observe increasing block order, and a further poll must return no updates.

FINISH ALPHA CHANGE 2556 -- test concurrent block-filter cursor consumption -- */

func TestBlockFilterCursor_ConcurrentPolling(t *testing.T) {
	for _, concurrentAppend := range []bool{false, true} {
		name := "existing_updates"
		if concurrentAppend {
			name = "concurrent_append"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			const (
				blockCount     = 256
				pollers        = 16
				pollsPerCaller = 32
			)

			manager := NewFilterManager(
				hclog.NewNullLogger(),
				newMockStore(),
				256,
			)
			t.Cleanup(manager.Close)

			id := manager.NewBlockFilter(nil)
			require.NotEmpty(t, id)

			headers := make([]*block, blockCount)
			expected := make([]string, blockCount)
			positions := make(map[string]int, blockCount)

			for i := range blockCount {
				hash := types.StringToHash(fmt.Sprintf("%064x", i+1))
				headers[i] = &block{Hash: hash}
				expected[i] = hash.String()
				positions[expected[i]] = i
			}

			start := make(chan struct{})
			producerDone := make(chan struct{})

			if concurrentAppend {
				go func() {
					defer close(producerDone)
					<-start

					for _, header := range headers {
						manager.blockStream.push(header)
						runtime.Gosched()
					}
				}()
			} else {
				for _, header := range headers {
					manager.blockStream.push(header)
				}
				close(producerDone)
			}

			type pollResult struct {
				hashes []string
				err    error
			}

			results := make(chan pollResult, pollers)

			for range pollers {
				go func() {
					<-start
					var result pollResult

					for range pollsPerCaller {
						raw, err := manager.GetFilterChanges(id)
						if err != nil {
							result.err = err
							break
						}

						hashes, ok := raw.([]string)
						if !ok {
							result.err = fmt.Errorf(
								"unexpected block-filter result type %T",
								raw,
							)
							break
						}

						result.hashes = append(result.hashes, hashes...)
						runtime.Gosched()
					}

					results <- result
				}()
			}

			close(start)

			counts := make(map[string]int, blockCount)
			checkHashes := func(hashes []string) {
				t.Helper()

				previous := -1
				for _, hash := range hashes {
					position, ok := positions[hash]
					require.True(t, ok, "unexpected block hash %s", hash)
					require.Greater(
						t,
						position,
						previous,
						"a caller received duplicate or backward block updates",
					)

					previous = position
					counts[hash]++
				}
			}

			timer := time.NewTimer(10 * time.Second)
			defer timer.Stop()

			for range pollers {
				select {
				case result := <-results:
					require.NoError(t, result.err)
					checkHashes(result.hashes)
				case <-timer.C:
					t.Fatal("concurrent block-filter polls did not finish")
				}
			}

			select {
			case <-producerDone:
			case <-timer.C:
				t.Fatal("block publication did not finish")
			}

			/* Collect any blocks published after the polling workers finished. */
			raw, err := manager.GetFilterChanges(id)
			require.NoError(t, err)

			remaining, ok := raw.([]string)
			require.True(t, ok)
			checkHashes(remaining)

			for _, hash := range expected {
				require.Equal(
					t,
					1,
					counts[hash],
					"block %s must be consumed exactly once",
					hash,
				)
			}

			/* The cursor must remain at the latest published block. */
			raw, err = manager.GetFilterChanges(id)
			require.NoError(t, err)

			remaining, ok = raw.([]string)
			require.True(t, ok)
			require.Empty(t, remaining)
		})
	}
}
