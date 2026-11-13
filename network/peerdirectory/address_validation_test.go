/* START ALPHA CHANGE 2846 -- regress directory request and persisted address validation -- */
package peerdirectory

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	libp2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func addressValidationRequest(
	t *testing.T,
	transport string,
) (peer.ID, *ExchangeRequest) {
	t.Helper()

	privateKey, publicKey, err := libp2pCrypto.GenerateKeyPair(
		libp2pCrypto.Secp256k1,
		256,
	)
	require.NoError(t, err)

	id, err := peer.IDFromPublicKey(publicKey)
	require.NoError(t, err)

	publicKeyBytes, err := libp2pCrypto.MarshalPublicKey(publicKey)
	require.NoError(t, err)

	req := &ExchangeRequest{
		PeerID:    id.String(),
		Multiaddr: fmt.Sprintf("%s/p2p/%s", transport, id),
		PublicKey: hex.EncodeToString(publicKeyBytes),
		Timestamp: time.Now().Unix(),
		Nonce:     "address-validation-nonce",
	}

	signature, err := privateKey.Sign(peerDirectorySignatureMessage(req))
	require.NoError(t, err)
	req.Signature = hex.EncodeToString(signature)

	return id, req
}

func TestPeerDirectoryAddressValidation_AddresslessRequestHasNoPublication(t *testing.T) {
	id, req := addressValidationRequest(t, "")
	verifier := &authorizationOrderVerifier{
		allowed: map[peer.ID]struct{}{id: {}},
	}
	server := NewServer(verifier)

	body, err := json.Marshal(req)
	require.NoError(t, err)

	response := serveAuthorizationOrderRequest(server.Handler(), body)

	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Contains(t, response.Body.String(), "no transport address")
	require.Zero(t, atomic.LoadInt64(&verifier.calls))
	require.Empty(t, server.peers)
	require.Empty(t, server.usedNonces)
}

func TestPeerDirectoryAddressValidation_TransportFormsRemainAccepted(t *testing.T) {
	for _, transport := range []string{
		"/ip4/127.0.0.1/tcp/30301",
		"/ip6/::1/tcp/30301",
		"/dns4/validator.example.com/tcp/30301",
	} {
		t.Run(transport, func(t *testing.T) {
			id, req := addressValidationRequest(t, transport)
			server := NewServer(&authorizationOrderVerifier{
				allowed: map[peer.ID]struct{}{id: {}},
			})

			body, err := json.Marshal(req)
			require.NoError(t, err)

			response := serveAuthorizationOrderRequest(server.Handler(), body)

			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, req.Multiaddr, server.peers[id])
		})
	}
}

func TestPeerDirectoryAddressValidation_CacheRejectsAddresslessRecords(t *testing.T) {
	id, req := addressValidationRequest(t, "")
	path := filepath.Join(t.TempDir(), "peers.json")

	body, err := json.Marshal(peerCacheFile{
		Peers: map[string]string{id.String(): req.Multiaddr},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, body, 0600))

	server := NewServer(nil)
	require.NoError(t, server.EnablePeerCache(path))
	require.Empty(t, server.peers)

	rewritten, err := os.ReadFile(path)
	require.NoError(t, err)

	var cache peerCacheFile
	require.NoError(t, json.Unmarshal(rewritten, &cache))
	require.Empty(t, cache.Peers)
}

func TestPeerDirectoryAddressValidation_CacheAcceptsTransportAddress(t *testing.T) {
	id, req := addressValidationRequest(t, "/dns4/validator.example.com/tcp/30301")
	path := filepath.Join(t.TempDir(), "peers.json")

	body, err := json.Marshal(peerCacheFile{
		Peers: map[string]string{id.String(): req.Multiaddr},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, body, 0600))

	server := NewServer(nil)
	require.NoError(t, server.EnablePeerCache(path))
	require.Equal(t, req.Multiaddr, server.peers[id])
}

/* FINISH ALPHA CHANGE 2846 -- regress directory request and persisted address validation -- */
