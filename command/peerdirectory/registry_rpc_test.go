/* START ALPHA CHANGE 2233 -- test registry RPC behavior and HTTP timeout -- */
package peerdirectory

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc/codec"
)

func TestDirectoryRegistryRPCMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request codec.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		result := "0x2a"
		if request.Method == "eth_call" {
			result = "0x"
		} else if request.Method != "eth_blockNumber" {
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  result,
		})
	}))
	defer server.Close()

	client, err := newRegistryRPCClient(server.URL)
	require.NoError(t, err)
	require.Equal(t, registryRPCTimeout, client.client.Timeout)

	number, err := client.BlockNumber()
	require.NoError(t, err)
	require.Equal(t, uint64(42), number)

	code, err := client.Call(&ethgo.CallMsg{}, ethgo.Latest)
	require.NoError(t, err)
	require.Equal(t, "0x", code)
}

func TestDirectoryRegistryRPCTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	client, err := newRegistryRPCClient(server.URL)
	require.NoError(t, err)
	client.client.Timeout = 50 * time.Millisecond

	_, err = client.BlockNumber()
	require.Error(t, err)

	var timeout net.Error
	require.True(t, errors.As(err, &timeout))
	require.True(t, timeout.Timeout())
}

func TestDirectoryRegistryRPCRejectsMalformedCallResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":""}`))
	}))
	defer server.Close()

	client, err := newRegistryRPCClient(server.URL)
	require.NoError(t, err)

	_, err = client.Call(&ethgo.CallMsg{}, ethgo.Latest)
	require.ErrorContains(t, err, "hexadecimal prefix")
}

/* FINISH ALPHA CHANGE 2233 -- test registry RPC behavior and HTTP timeout -- */
