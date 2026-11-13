/* START ALPHA CHANGE 2222 -- test authorization before nonce-cache consumption -- */
package peerdirectory

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libp2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type authorizationOrderVerifier struct {
	allowed map[peer.ID]struct{}
	err     error
	calls   int64
}

func (v *authorizationOrderVerifier) IsValidatorPeerAllowed(id peer.ID) (bool, error) {
	allowed, err := v.GetAllowedValidatorPeerIDs()
	if err != nil {
		return false, err
	}

	_, ok := allowed[id]

	return ok, nil
}

func (v *authorizationOrderVerifier) GetAllowedValidatorPeerIDs() (map[peer.ID]struct{}, error) {
	atomic.AddInt64(&v.calls, 1)

	if v.err != nil {
		return nil, v.err
	}

	result := make(map[peer.ID]struct{}, len(v.allowed))
	for id := range v.allowed {
		result[id] = struct{}{}
	}

	return result, nil
}

func newAuthorizationOrderRequest(t *testing.T) (peer.ID, *ExchangeRequest) {
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
		PeerID: id.String(),
		Multiaddr: fmt.Sprintf(
			"/ip4/127.0.0.1/tcp/30301/p2p/%s",
			id.String(),
		),
		PublicKey: hex.EncodeToString(publicKeyBytes),
		Timestamp: time.Now().Unix(),
		Nonce:     "authorization-order-nonce",
	}

	signature, err := privateKey.Sign(peerDirectorySignatureMessage(req))
	require.NoError(t, err)
	req.Signature = hex.EncodeToString(signature)

	return id, req
}

func serveAuthorizationOrderRequest(
	handler http.Handler,
	body []byte,
) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		ExchangePath,
		bytes.NewReader(body),
	)
	handler.ServeHTTP(recorder, request)

	return recorder
}

func TestExchangeAuthorizationPrecedesNonceConsumption(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"unauthorized",
		"registry_failure",
		"invalid_signature",
		"missing_verifier",
	} {
		t.Run(name, func(t *testing.T) {
			id, req := newAuthorizationOrderRequest(t)
			verifier := &authorizationOrderVerifier{
				allowed: map[peer.ID]struct{}{id: {}},
			}

			var backend ValidatorVerifier = verifier
			wantStatus := http.StatusForbidden
			wantCalls := int64(1)

			switch name {
			case "unauthorized":
				verifier.allowed = map[peer.ID]struct{}{}

			case "registry_failure":
				verifier.err = errors.New("injected registry failure")
				wantStatus = http.StatusInternalServerError

			case "invalid_signature":
				// Change signed content while preserving valid request syntax.
				req.Nonce += "-tampered"
				wantStatus = http.StatusUnauthorized
				wantCalls = 0

			case "missing_verifier":
				backend = nil
				wantCalls = 0
			}

			server := NewServer(backend)

			// An expired entry proves rejected requests do not even run
			// nonce pruning before authorization.
			expiredAt := time.Now().Add(-time.Hour)
			server.usedNonces["expired-sentinel"] = expiredAt

			body, err := json.Marshal(req)
			require.NoError(t, err)

			response := serveAuthorizationOrderRequest(server.Handler(), body)

			require.Equal(t, wantStatus, response.Code)
			require.Equal(t, wantCalls, atomic.LoadInt64(&verifier.calls))
			require.Equal(t, map[string]time.Time{
				"expired-sentinel": expiredAt,
			}, server.usedNonces)
			require.Empty(t, server.peers)
		})
	}
}

func TestExchangeAuthorizedRequestConsumesNonceAndRejectsReplay(t *testing.T) {
	t.Parallel()

	id, req := newAuthorizationOrderRequest(t)
	verifier := &authorizationOrderVerifier{
		allowed: map[peer.ID]struct{}{id: {}},
	}
	server := NewServer(verifier)
	server.usedNonces["expired-sentinel"] = time.Now().Add(-time.Hour)

	body, err := json.Marshal(req)
	require.NoError(t, err)

	handler := server.Handler()
	first := serveAuthorizationOrderRequest(handler, body)
	require.Equal(t, http.StatusOK, first.Code)

	var response ExchangeResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &response))
	require.ElementsMatch(t, []string{req.Multiaddr}, response.Peers)

	nonceKey := fmt.Sprintf("%s:%s", id, req.Nonce)
	require.Len(t, server.usedNonces, 1)
	require.Contains(t, server.usedNonces, nonceKey)
	require.NotContains(t, server.usedNonces, "expired-sentinel")
	expiresAt := server.usedNonces[nonceKey]

	second := serveAuthorizationOrderRequest(handler, body)
	require.Equal(t, http.StatusUnauthorized, second.Code)

	var failure map[string]string
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &failure))
	require.Equal(t, errNonceAlreadyUsed.Error(), failure["error"])

	require.Len(t, server.usedNonces, 1)
	require.Equal(t, expiresAt, server.usedNonces[nonceKey])
	require.Equal(t, map[peer.ID]string{id: req.Multiaddr}, server.peers)

	// This patch deliberately authorizes before checking the nonce.
	// It does not add a pre-registry replay filter or registry rate limit.
	require.Equal(t, int64(2), atomic.LoadInt64(&verifier.calls))
}

func TestExchangeConcurrentReplayAcceptsExactlyOnce(t *testing.T) {
	t.Parallel()

	id, req := newAuthorizationOrderRequest(t)
	verifier := &authorizationOrderVerifier{
		allowed: map[peer.ID]struct{}{id: {}},
	}
	server := NewServer(verifier)

	body, err := json.Marshal(req)
	require.NoError(t, err)

	const requests = 8

	handler := server.Handler()
	start := make(chan struct{})
	statuses := make(chan int, requests)
	var workers sync.WaitGroup

	for i := 0; i < requests; i++ {
		workers.Add(1)

		go func() {
			defer workers.Done()
			<-start

			response := serveAuthorizationOrderRequest(handler, body)
			statuses <- response.Code
		}()
	}

	close(start)
	workers.Wait()
	close(statuses)

	accepted := 0
	rejected := 0

	for status := range statuses {
		switch status {
		case http.StatusOK:
			accepted++
		case http.StatusUnauthorized:
			rejected++
		default:
			t.Errorf("unexpected exchange status: %d", status)
		}
	}

	require.Equal(t, 1, accepted)
	require.Equal(t, requests-1, rejected)
	require.Len(t, server.usedNonces, 1)
	require.Equal(t, map[peer.ID]string{id: req.Multiaddr}, server.peers)
	require.Equal(t, int64(requests), atomic.LoadInt64(&verifier.calls))
}

/* FINISH ALPHA CHANGE 2222 -- test authorization before nonce-cache consumption -- */
