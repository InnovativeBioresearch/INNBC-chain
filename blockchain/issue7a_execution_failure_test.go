/* START ALPHA CHANGE 2157 -- test pre-commit failure precedence without receipt publication -- */
package blockchain

import (
	"errors"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type issue7AHookSnapshot struct {
	state.Snapshot
	cause   error
	commits int
}

func (s *issue7AHookSnapshot) GetAccount(types.Address) (*state.Account, error) {
	return nil, s.cause
}

func (s *issue7AHookSnapshot) Commit(objects []*state.Object) (state.Snapshot, []byte, error) {
	s.commits++
	return s.Snapshot.Commit(objects)
}

func TestIssue7A_PreCommitStateFailurePrecedesHookError(t *testing.T) {
	for _, localFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary hook error", true: "local failure"}[localFailure], func(t *testing.T) {
			bc := NewTestBlockchain(t, nil)
			cause := errors.New("injected pre-commit account failure")
			hookErr := errors.New("ordinary hook error")
			snapshot := &issue7AHookSnapshot{
				Snapshot: itrie.NewState(itrie.NewMemoryStorage()).NewSnapshot(),
			}
			if localFailure {
				snapshot.cause = cause
			}

			tr := state.NewTransition(chain.ForksInTime{}, snapshot, state.NewTxn(snapshot))
			bc.executor = &mockExecutor{
				processBlockFn: func(types.Hash, *types.Block, types.Address) (*state.Transition, error) {
					return tr, nil
				},
			}

			verifier := &MockVerifier{}
			verifier.HookPreCommitState(func(_ *types.Block, transition *state.Transition) error {
				transition.GetNonce(types.StringToAddress("0x7201"))
				return hookErr
			})
			bc.consensus = verifier

			parent := bc.Header()
			header := &types.Header{
				Number:     parent.Number + 1,
				ParentHash: parent.Hash,
				GasLimit:   1_000_000,
			}
			header.ComputeHash()

			result, err := bc.executeBlockTransactions(&types.Block{Header: header})
			require.Nil(t, result)

			if localFailure {
				require.ErrorIs(t, err, cause)
				require.ErrorIs(t, err, state.ErrLocalStateFailure)
				require.False(t, errors.Is(err, hookErr))
			} else {
				require.ErrorIs(t, err, hookErr)
				require.False(t, errors.Is(err, state.ErrLocalStateFailure))
			}

			require.Zero(t, snapshot.commits)
			require.False(t, bc.receiptsCache.Contains(header.Hash))
		})
	}
}

/* FINISH ALPHA CHANGE 2157 -- test pre-commit failure precedence without receipt publication -- */
