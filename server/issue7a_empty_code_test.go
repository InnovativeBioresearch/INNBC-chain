/* START ALPHA CHANGE 2187 -- test RPC rejection of present empty referenced bytecode -- */
package server

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type issue7ARPCEmptyCodeState struct {
	state.State
	reads int
}

func (s *issue7ARPCEmptyCodeState) GetCode(types.Hash) ([]byte, bool, error) {
	s.reads++

	return []byte{}, true, nil
}

func TestIssue7A_RPCRejectsPresentEmptyCode(t *testing.T) {
	hub, backend, header, tx := issue7ARPCFixture(t)
	emptyCodeState := &issue7ARPCEmptyCodeState{State: backend}
	hub.state = emptyCodeState

	code, err := hub.GetCode(header.StateRoot, backend.target)
	require.ErrorIs(t, err, state.ErrLocalStateFailure)
	require.ErrorIs(t, err, state.ErrMissingCode)
	require.Nil(t, code)
	require.Equal(t, 1, emptyCodeState.reads)

	// The sender is an ordinary account with canonical empty code.
	// Its lookup must succeed without reading a bytecode record.
	code, err = hub.GetCode(header.StateRoot, tx.From)
	require.NoError(t, err)
	require.Empty(t, code)
	require.Equal(t, 1, emptyCodeState.reads)
}

/* FINISH ALPHA CHANGE 2187 -- test RPC rejection of present empty referenced bytecode -- */
