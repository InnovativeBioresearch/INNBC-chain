/* START ALPHA CHANGE 3595 -- verify common newcomer priorities across batch validator updates -- */
package innbcbft

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestProposerBatchPriority_CommonBase(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		oldCount     int
		retained     int
		added        int
		wantRetained int64
		wantAdded    int64
	}{
		{
			name:         "single_newcomer",
			oldCount:     6,
			retained:     6,
			added:        1,
			wantRetained: 1,
			wantAdded:    -6,
		},
		{
			name:         "six_existing_plus_two_newcomers",
			oldCount:     6,
			retained:     6,
			added:        2,
			wantRetained: 3,
			wantAdded:    -6,
		},
		{
			name:         "larger_batch",
			oldCount:     6,
			retained:     6,
			added:        10,
			wantRetained: 12,
			wantAdded:    -6,
		},
		{
			name:         "additions_with_removals",
			oldCount:     6,
			retained:     4,
			added:        2,
			wantRetained: 3,
			wantAdded:    -6,
		},
		{
			name:         "removals_require_rescaling",
			oldCount:     10,
			retained:     2,
			added:        3,
			wantRetained: 5,
			wantAdded:    -2,
		},
		{
			name:      "all_existing_validators_replaced",
			oldCount:  8,
			retained:  0,
			added:     2,
			wantAdded: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, order := range []struct {
				name    string
				reverse bool
			}{
				{name: "forward"},
				{name: "reversed_newcomers", reverse: true},
			} {
				t.Run(order.name, func(t *testing.T) {
					oldValidators := make(validator.AccountSet, tc.oldCount)
					for i := range oldValidators {
						oldValidators[i] = &validator.ValidatorMetadata{
							Address:     types.Address{19: byte(i + 1)},
							VotingPower: big.NewInt(1),
							IsActive:    true,
						}
					}

					snapshot := NewProposerSnapshot(1, oldValidators)

					addedValidators := make(validator.AccountSet, tc.added)
					for i := range addedValidators {
						addedValidators[i] = &validator.ValidatorMetadata{
							Address:     types.Address{19: byte(tc.oldCount + i + 1)},
							VotingPower: big.NewInt(1),
							IsActive:    true,
						}
					}

					nextValidators := make(
						validator.AccountSet,
						0,
						tc.retained+tc.added,
					)
					nextValidators = append(
						nextValidators,
						oldValidators[:tc.retained]...,
					)

					for i := range addedValidators {
						index := i
						if order.reverse {
							index = len(addedValidators) - 1 - i
						}

						nextValidators = append(
							nextValidators,
							addedValidators[index],
						)
					}

					require.NoError(t, updateValidators(snapshot, nextValidators))
					require.Len(t, snapshot.Validators, tc.retained+tc.added)
					require.Zero(
						t,
						snapshot.GetTotalVotingPower().Cmp(
							big.NewInt(int64(tc.retained+tc.added)),
						),
						"validator updates must preserve the expected voting power",
					)

					for i, member := range snapshot.Validators {
						require.Equal(
							t,
							nextValidators[i].Address,
							member.Metadata.Address,
						)
						require.Zero(
							t,
							member.Metadata.VotingPower.Cmp(big.NewInt(1)),
							"voting power changed at index %d",
							i,
						)

						wantPriority := tc.wantAdded
						if i < tc.retained {
							wantPriority = tc.wantRetained
						}

						require.Zero(
							t,
							member.ProposerPriority.Cmp(big.NewInt(wantPriority)),
							"wrong normalized priority at index %d: got %s, want %d",
							i,
							member.ProposerPriority.String(),
							wantPriority,
						)
					}

					for _, member := range oldValidators {
						require.Zero(
							t,
							member.VotingPower.Cmp(big.NewInt(1)),
							"original voting power must remain unchanged",
						)
					}

					if tc.added > 1 {
						first := snapshot.Validators[tc.retained].ProposerPriority
						second := snapshot.Validators[tc.retained+1].ProposerPriority

						require.NotSame(t, first, second)

						secondBefore := new(big.Int).Set(second)
						first.Add(first, big.NewInt(1))

						require.Zero(
							t,
							second.Cmp(secondBefore),
							"changing one newcomer's priority must not change another",
						)
					}
				})
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3595 -- verify common newcomer priorities across batch validator updates -- */
