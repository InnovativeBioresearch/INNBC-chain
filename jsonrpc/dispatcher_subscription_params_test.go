/* START ALPHA CHANGE 2184 -- test missing logs filter through websocket dispatch -- */
package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDispatcherLogsSubscriptionMissingFilter(t *testing.T) {
	dispatcher := &Dispatcher{}

	require.NotPanics(t, func() {
		body, err := dispatcher.HandleWs(
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_subscribe","params":["logs"]}`),
			nil,
		)
		require.NoError(t, err)

		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		require.NoError(t, json.Unmarshal(body, &response))
		require.Equal(t, 1, response.ID)
		require.NotNil(t, response.Error)
		require.Equal(t, -32602, response.Error.Code)
		require.Equal(t, "Missing logs filter parameter", response.Error.Message)
		require.True(t, len(response.Result) == 0 || string(response.Result) == "null")
	})
}

/* FINISH ALPHA CHANGE 2184 -- test missing logs filter through websocket dispatch -- */
