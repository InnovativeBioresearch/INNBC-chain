/* START ALPHA CHANGE 2486 -- test safe log-range defaults and positive overrides -- */
package jsonrpc

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type blockRangeTestStore struct {
	*mockStore
	blockReads int
}

func (s *blockRangeTestStore) GetBlockByNumber(number uint64, _ bool) (*types.Block, bool) {
	s.blockReads++

	return &types.Block{
		Header: &types.Header{Number: number},
	}, true
}

func TestBlockRangeLimit_Enforcement(t *testing.T) {
	cases := []struct {
		name       string
		configured uint64
		to         BlockNumber
		rejected   bool
	}{
		{"zero accepts default boundary", 0, 257, false},
		{"zero rejects beyond default", 0, 258, true},
		{"explicit default accepts boundary", 256, 257, false},
		{"explicit default rejects beyond boundary", 256, 258, true},
		{"larger override accepts boundary", 512, 513, false},
		{"larger override rejects beyond boundary", 512, 514, true},
		{"smaller override accepts boundary", 1, 2, false},
		{"smaller override rejects beyond boundary", 1, 3, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, direct := range []bool{false, true} {
				name := "constructor"
				if direct {
					name = "direct_struct"
				}

				t.Run(name, func(t *testing.T) {
					store := &blockRangeTestStore{mockStore: newMockStore()}

					var manager *FilterManager
					if direct {
						manager = &FilterManager{
							store:           store,
							blockRangeLimit: tc.configured,
						}
					} else {
						manager = NewFilterManager(
							hclog.NewNullLogger(),
							store,
							tc.configured,
						)
						t.Cleanup(manager.Close)

						expectedLimit := tc.configured
						if expectedLimit == 0 {
							expectedLimit = 256
						}
						require.Equal(t, expectedLimit, manager.blockRangeLimit)
					}

					logs, err := manager.GetLogsForQuery(&LogQuery{
						fromBlock: 1,
						toBlock:   tc.to,
					})

					if tc.rejected {
						require.ErrorIs(t, err, ErrBlockRangeTooHigh)
						require.Zero(t, store.blockReads)
						return
					}

					require.NoError(t, err)
					require.Empty(t, logs)
					require.Equal(t, int(tc.to), store.blockReads)
				})
			}
		})
	}
}

/* FINISH ALPHA CHANGE 2486 -- test safe log-range defaults and positive overrides -- */

/* START ALPHA CHANGE 2503 -- test genesis-only log queries without unsigned underflow -- */
func TestBlockRangeLimit_GenesisOnly(t *testing.T) {
	for _, limit := range []uint64{0, 256, 512} {
		store := &blockRangeTestStore{mockStore: newMockStore()}
		manager := &FilterManager{
			store:           store,
			blockRangeLimit: limit,
		}

		logs, err := manager.GetLogsForQuery(&LogQuery{
			fromBlock: 0,
			toBlock:   0,
		})

		require.NoError(t, err)
		require.NotNil(t, logs)
		require.Empty(t, logs)
		require.Zero(t, store.blockReads)
	}
}

/* FINISH ALPHA CHANGE 2503 -- test genesis-only log queries without unsigned underflow -- */
