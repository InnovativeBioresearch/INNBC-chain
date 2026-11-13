/* START ALPHA CHANGE 2234 -- cancel active directory refresh on network shutdown -- */
package network

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestDirectoryRefreshStopsOnShutdown(t *testing.T) {
	server, err := CreateServer(&CreateServerParams{
		ConfigCallback: func(c *Config) {
			c.NoDiscover = true
			c.PeerDirectoryEnabled = true
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })

	existing := peer.ID("existing")
	server.SetAllowedPeers([]peer.ID{server.host.ID(), existing})

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(entered) })

		select {
		case <-r.Context().Done():
			return
		case <-release:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer httpServer.Close()
	defer close(release)

	server.config.PeerDirectoryURL = httpServer.URL
	result := make(chan error, 1)

	go func() {
		_, err := server.exchangePeerDirectory()
		result <- err
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("directory request did not start")
	}

	require.NoError(t, server.Close())

	select {
	case err := <-result:
		require.True(t, errors.Is(err, context.Canceled), "unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("directory refresh did not stop after shutdown")
	}
}

/* FINISH ALPHA CHANGE 2234 -- cancel active directory refresh on network shutdown -- */
