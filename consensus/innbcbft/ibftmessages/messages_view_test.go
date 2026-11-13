/* START ALPHA CHANGE 1895 -- test bounded consensus message retention -- */
package ibftmessages

import (
	"testing"

	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/stretchr/testify/require"
)

func TestMessagesPruneByView(t *testing.T) {
	t.Parallel()

	messageStore := NewMessages()
	t.Cleanup(messageStore.Close)

	views := []*proto.View{
		{Height: 9, Round: 8},
		{Height: 10, Round: 1},
		{Height: 10, Round: 4},
		{Height: 10, Round: 6},
		{Height: 11, Round: 0},
	}

	for index, view := range views {
		messageStore.AddMessage(&proto.Message{
			From: []byte{byte(index + 1)},
			View: view,
			Type: proto.MessageType_ROUND_CHANGE,
			Payload: &proto.Message_RoundChangeData{
				RoundChangeData: &proto.RoundChangeMessage{},
			},
		})
	}

	messageStore.PruneByView(&proto.View{Height: 10, Round: 4})

	require.Empty(t, messageStore.GetValidMessages(
		views[0],
		proto.MessageType_ROUND_CHANGE,
		func(*proto.Message) bool { return true },
	))
	require.Empty(t, messageStore.GetValidMessages(
		views[1],
		proto.MessageType_ROUND_CHANGE,
		func(*proto.Message) bool { return true },
	))
	require.Len(t, messageStore.GetValidMessages(
		views[2],
		proto.MessageType_ROUND_CHANGE,
		func(*proto.Message) bool { return true },
	), 1)
	require.Len(t, messageStore.GetValidMessages(
		views[3],
		proto.MessageType_ROUND_CHANGE,
		func(*proto.Message) bool { return true },
	), 1)
	require.Len(t, messageStore.GetValidMessages(
		views[4],
		proto.MessageType_ROUND_CHANGE,
		func(*proto.Message) bool { return true },
	), 1)
}

/* FINISH ALPHA CHANGE 1895 -- test bounded consensus message retention -- */
