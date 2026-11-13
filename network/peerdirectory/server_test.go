package peerdirectory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	libp2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 1128 -- test peer directory exchange body limit -- */

func TestHandleExchangeRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	server := NewServer(nil)
	body := bytes.Repeat([]byte(" "), int(maxExchangeRequestBodyBytes)+1)

	req := httptest.NewRequest(http.MethodPost, ExchangePath, bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.True(t, strings.Contains(rec.Body.String(), "request body too large"))
}

/* FINISH ALPHA CHANGE 1128 -- test peer directory exchange body limit -- */

/* START ALPHA CHANGE 1365 -- test peer directory registry snapshot refresh -- */

type mockValidatorVerifier struct {
	allowed map[peer.ID]struct{}
	err     error
}

func (m *mockValidatorVerifier) IsValidatorPeerAllowed(peerID peer.ID) (bool, error) {
	allowedPeerIDs, err := m.GetAllowedValidatorPeerIDs()
	if err != nil {
		return false, err
	}

	_, ok := allowedPeerIDs[peerID]

	return ok, nil
}

func (m *mockValidatorVerifier) GetAllowedValidatorPeerIDs() (map[peer.ID]struct{}, error) {
	if m.err != nil {
		return nil, m.err
	}

	allowedPeerIDs := make(map[peer.ID]struct{}, len(m.allowed))
	for peerID := range m.allowed {
		allowedPeerIDs[peerID] = struct{}{}
	}

	return allowedPeerIDs, nil
}

func TestStoreAndListPeersRevalidatesStoredPeers(t *testing.T) {
	t.Parallel()

	activePeer := peer.ID("active")
	removedPeer := peer.ID("removed")

	server := NewServer(&mockValidatorVerifier{
		allowed: map[peer.ID]struct{}{
			activePeer: {},
		},
	})

	server.peers[removedPeer] = "removed-addr"

	// ALPHA CHANGE 2221: supply the nonce consumed after authorization.
	peers, err := server.storeAndListPeers(activePeer, "active-addr", "active-request")

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"active-addr"}, peers)
	require.Equal(t, map[peer.ID]string{
		activePeer: "active-addr",
	}, server.peers)
}

func TestStoreAndListPeersUpdatesExistingPeerAddress(t *testing.T) {
	t.Parallel()

	activePeer := peer.ID("active")

	server := NewServer(&mockValidatorVerifier{
		allowed: map[peer.ID]struct{}{
			activePeer: {},
		},
	})

	server.peers[activePeer] = "old-addr"

	// ALPHA CHANGE 2221: supply the nonce consumed after authorization.
	peers, err := server.storeAndListPeers(activePeer, "new-addr", "address-update")

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"new-addr"}, peers)
	require.Equal(t, "new-addr", server.peers[activePeer])
}

func TestStoreAndListPeersRejectsRequesterOutsideRegistrySnapshot(t *testing.T) {
	t.Parallel()

	activePeer := peer.ID("active")

	server := NewServer(&mockValidatorVerifier{
		allowed: map[peer.ID]struct{}{},
	})

	// ALPHA CHANGE 2221: supply the nonce consumed after authorization.
	peers, err := server.storeAndListPeers(activePeer, "active-addr", "active-request")

	require.Nil(t, peers)
	require.ErrorIs(t, err, errPeerNotAllowed)
	require.Empty(t, server.peers)
}

func TestStoreAndListPeersPreservesRecordsOnVerifierError(t *testing.T) {
	t.Parallel()

	activePeer := peer.ID("active")
	storedPeer := peer.ID("stored")
	verifierErr := errors.New("registry unavailable")

	server := NewServer(&mockValidatorVerifier{
		err: verifierErr,
	})

	server.peers[storedPeer] = "stored-addr"

	// ALPHA CHANGE 2221: supply the nonce consumed after authorization.
	peers, err := server.storeAndListPeers(activePeer, "active-addr", "active-request")

	require.Nil(t, peers)
	require.ErrorIs(t, err, verifierErr)
	require.Equal(t, map[peer.ID]string{
		storedPeer: "stored-addr",
	}, server.peers)
}

func TestStoreAndListPeersSerializesConcurrentRefreshes(t *testing.T) {
	t.Parallel()

	peerA := peer.ID("active-a")
	peerB := peer.ID("active-b")

	server := NewServer(&mockValidatorVerifier{
		allowed: map[peer.ID]struct{}{
			peerA: {},
			peerB: {},
		},
	})

	errCh := make(chan error, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		// ALPHA CHANGE 2221: supply an independent exchange nonce.
		_, err := server.storeAndListPeers(peerA, "addr-a", "peer-a-request")
		errCh <- err
	}()

	go func() {
		defer wg.Done()

		// ALPHA CHANGE 2221: supply an independent exchange nonce.
		_, err := server.storeAndListPeers(peerB, "addr-b", "peer-b-request")
		errCh <- err
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	require.Equal(t, map[peer.ID]string{
		peerA: "addr-a",
		peerB: "addr-b",
	}, server.peers)
}

/* FINISH ALPHA CHANGE 1365 -- test peer directory registry snapshot refresh -- */

/* START ALPHA CHANGE 1918 -- test persistent peer directory cache recovery and filtering -- */

func newPersistentPeerTestAddress(t *testing.T, port int) (peer.ID, string) {
	t.Helper()

	privateKey, _, err := libp2pCrypto.GenerateKeyPair(libp2pCrypto.Secp256k1, 256)
	require.NoError(t, err)

	peerID, err := peer.IDFromPrivateKey(privateKey)
	require.NoError(t, err)

	return peerID, fmt.Sprintf(
		"/ip4/127.0.0.1/tcp/%d/p2p/%s",
		port,
		peerID.String(),
	)
}

func TestPeerCacheSurvivesServerRestart(t *testing.T) {
	t.Parallel()

	peerID, peerAddr := newPersistentPeerTestAddress(t, 30301)
	cachePath := filepath.Join(t.TempDir(), "peers.json")
	verifier := &mockValidatorVerifier{
		allowed: map[peer.ID]struct{}{
			peerID: {},
		},
	}

	server := NewServer(verifier)
	require.NoError(t, server.EnablePeerCache(cachePath))

	// ALPHA CHANGE 2221: supply the nonce consumed after authorization.
	peers, err := server.storeAndListPeers(peerID, peerAddr, "persistent-request")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{peerAddr}, peers)

	restartedServer := NewServer(verifier)
	require.NoError(t, restartedServer.EnablePeerCache(cachePath))
	require.Equal(t, map[peer.ID]string{
		peerID: peerAddr,
	}, restartedServer.peers)
}

func TestPeerCachePersistsRegistryRemoval(t *testing.T) {
	t.Parallel()

	peerA, peerAddrA := newPersistentPeerTestAddress(t, 30301)
	peerB, peerAddrB := newPersistentPeerTestAddress(t, 30302)
	cachePath := filepath.Join(t.TempDir(), "peers.json")
	verifier := &mockValidatorVerifier{
		allowed: map[peer.ID]struct{}{
			peerA: {},
			peerB: {},
		},
	}

	server := NewServer(verifier)
	require.NoError(t, server.EnablePeerCache(cachePath))

	// ALPHA CHANGE 2221: use a distinct nonce for each successful exchange.
	_, err := server.storeAndListPeers(peerA, peerAddrA, "peer-a-initial")
	require.NoError(t, err)

	// ALPHA CHANGE 2221: use a distinct nonce for each successful exchange.
	_, err = server.storeAndListPeers(peerB, peerAddrB, "peer-b-initial")
	require.NoError(t, err)

	delete(verifier.allowed, peerB)

	// ALPHA CHANGE 2221: revalidation is a new exchange, not a replay.
	peers, err := server.storeAndListPeers(peerA, peerAddrA, "peer-a-refresh")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{peerAddrA}, peers)

	restartedServer := NewServer(verifier)
	require.NoError(t, restartedServer.EnablePeerCache(cachePath))
	require.Equal(t, map[peer.ID]string{
		peerA: peerAddrA,
	}, restartedServer.peers)
}

func TestPeerCacheRegeneratesCorruptedFile(t *testing.T) {
	t.Parallel()

	cachePath := filepath.Join(t.TempDir(), "peers.json")
	require.NoError(t, os.WriteFile(cachePath, []byte("{"), 0o600))

	server := NewServer(nil)
	require.NoError(t, server.EnablePeerCache(cachePath))
	require.Empty(t, server.peers)

	cacheBytes, err := os.ReadFile(cachePath)
	require.NoError(t, err)

	var cache peerCacheFile
	require.NoError(t, json.Unmarshal(cacheBytes, &cache))
	require.Empty(t, cache.Peers)
}

/* FINISH ALPHA CHANGE 1918 -- test persistent peer directory cache recovery and filtering -- */
