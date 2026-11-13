package peerdirectory

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/innovativebioresearch/innbc-chain/network/common"
	libp2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	ExchangePath = "/api/v1/peers/exchange"

	maxClockSkew = 2 * time.Minute
	nonceTTL     = 5 * time.Minute

	/* START ALPHA CHANGE 1126 -- cap peer directory exchange request bodies -- */
	maxExchangeRequestBodyBytes int64 = 64 * 1024
	/* FINISH ALPHA CHANGE 1126 -- cap peer directory exchange request bodies -- */
)

var (
	errMissingPeerID    = errors.New("missing peer_id")
	errMissingMultiaddr = errors.New("missing multiaddr")
	errMissingPublicKey = errors.New("missing public_key")
	errMissingNonce     = errors.New("missing nonce")
	errMissingSignature = errors.New("missing signature")
	errStaleTimestamp   = errors.New("stale timestamp")
	errPeerIDMismatch   = errors.New("peer_id does not match public key")
	errMultiaddrPeerID  = errors.New("multiaddr does not match peer_id")
	errInvalidSignature = errors.New("invalid signature")
	errNonceAlreadyUsed = errors.New("nonce already used")
	errPeerNotAllowed   = errors.New("peer is not an active validator")
)

/* START ALPHA CHANGE 1355 -- expose peer directory registry allowlist snapshot -- */
type ValidatorVerifier interface {
	IsValidatorPeerAllowed(peer.ID) (bool, error)
	GetAllowedValidatorPeerIDs() (map[peer.ID]struct{}, error)
}

/* FINISH ALPHA CHANGE 1355 -- expose peer directory registry allowlist snapshot */

type ExchangeRequest struct {
	PeerID    string `json:"peer_id"`
	Multiaddr string `json:"multiaddr"`
	PublicKey string `json:"public_key"`
	Timestamp int64  `json:"timestamp_unix"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

type ExchangeResponse struct {
	Peers []string `json:"peers"`
}

/* START ALPHA CHANGE 1912 -- persist authenticated peer directory addresses across service restarts -- */

type peerCacheFile struct {
	Peers map[string]string `json:"peers"`
}

type Server struct {
	verifier ValidatorVerifier

	/* START ALPHA CHANGE 2225 -- retain native directory admission controls -- */
	limits *exchangeLimits
	/* FINISH ALPHA CHANGE 2225 -- retain native directory admission controls -- */

	lock          sync.Mutex
	peers         map[peer.ID]string
	usedNonces    map[string]time.Time
	peerCachePath string

	/* START ALPHA CHANGE 1356 -- serialize peer directory registry refreshes -- */
	exchangeLock sync.Mutex
	/* FINISH ALPHA CHANGE 1356 -- serialize peer directory registry refreshes -- */
}

/* START ALPHA CHANGE 2225 -- initialize default or explicitly configured directory limits -- */
func NewServer(verifier ValidatorVerifier) *Server {
	return newServer(verifier, DefaultLimits())
}

func NewServerWithLimits(verifier ValidatorVerifier, limits Limits) (*Server, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}

	return newServer(verifier, limits), nil
}

func newServer(verifier ValidatorVerifier, limits Limits) *Server {
	return &Server{
		verifier:   verifier,
		limits:     newExchangeLimits(limits),
		peers:      make(map[peer.ID]string),
		usedNonces: make(map[string]time.Time),
	}
}

/* FINISH ALPHA CHANGE 2225 -- initialize default or explicitly configured directory limits -- */

func (s *Server) EnablePeerCache(cachePath string) error {
	if cachePath == "" {
		return errors.New("peer directory cache path is empty")
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.peerCachePath = cachePath

	cacheBytes, err := os.ReadFile(cachePath)
	if errors.Is(err, os.ErrNotExist) {
		s.peers = make(map[peer.ID]string)

		return s.persistPeerCacheLocked(s.peers)
	}

	if err != nil {
		return fmt.Errorf("failed to read peer directory cache %q: %w", cachePath, err)
	}

	var cache peerCacheFile
	if err := json.Unmarshal(cacheBytes, &cache); err != nil {
		return s.regeneratePeerCacheLocked(err)
	}

	loadedPeers := make(map[peer.ID]string, len(cache.Peers))

	for peerIDText, multiaddr := range cache.Peers {
		peerID, err := peer.Decode(peerIDText)
		if err != nil {
			return s.regeneratePeerCacheLocked(
				fmt.Errorf("invalid cached peer ID %q: %w", peerIDText, err),
			)
		}

		peerInfo, err := common.StringToAddrInfo(multiaddr)
		if err != nil {
			return s.regeneratePeerCacheLocked(
				fmt.Errorf("invalid cached multiaddress for peer %q: %w", peerIDText, err),
			)
		}

		/* START ALPHA CHANGE 2832 -- reject addressless persisted directory records -- */
		if len(peerInfo.Addrs) == 0 {
			return s.regeneratePeerCacheLocked(
				fmt.Errorf("cached multiaddress for peer %q has no transport address", peerIDText),
			)
		}
		/* FINISH ALPHA CHANGE 2832 -- reject addressless persisted directory records -- */

		if peerInfo.ID != peerID {
			return s.regeneratePeerCacheLocked(
				fmt.Errorf("cached multiaddress peer ID does not match %q", peerIDText),
			)
		}

		loadedPeers[peerID] = multiaddr
	}

	s.peers = loadedPeers

	return nil
}

func (s *Server) regeneratePeerCacheLocked(cause error) error {
	fmt.Printf(
		"INNBC peer-directory cache corrupted and regenerated: path=%s err=%v\n",
		s.peerCachePath,
		cause,
	)

	if err := os.Remove(s.peerCachePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove corrupted peer directory cache %q: %w", s.peerCachePath, err)
	}

	s.peers = make(map[peer.ID]string)

	return s.persistPeerCacheLocked(s.peers)
}

func (s *Server) persistPeerCacheLocked(peers map[peer.ID]string) error {
	if s.peerCachePath == "" {
		return nil
	}

	persistedPeers := make(map[string]string, len(peers))
	for peerID, multiaddr := range peers {
		persistedPeers[peerID.String()] = multiaddr
	}

	cacheBytes, err := json.MarshalIndent(&peerCacheFile{
		Peers: persistedPeers,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode peer directory cache: %w", err)
	}

	cacheBytes = append(cacheBytes, '\n')

	cacheDirectory := filepath.Dir(s.peerCachePath)
	if err := os.MkdirAll(cacheDirectory, 0o750); err != nil {
		return fmt.Errorf("failed to create peer directory cache directory %q: %w", cacheDirectory, err)
	}

	temporaryPath := s.peerCachePath + ".tmp"

	if err := os.WriteFile(temporaryPath, cacheBytes, 0o600); err != nil {
		return fmt.Errorf("failed to write peer directory cache %q: %w", temporaryPath, err)
	}

	if err := os.Rename(temporaryPath, s.peerCachePath); err != nil {
		_ = os.Remove(temporaryPath)

		return fmt.Errorf("failed to publish peer directory cache %q: %w", s.peerCachePath, err)
	}

	return nil
}

/* FINISH ALPHA CHANGE 1912 -- persist authenticated peer directory addresses across service restarts -- */

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(ExchangePath, s.handleExchange)

	return mux
}

func (s *Server) handleExchange(w http.ResponseWriter, r *http.Request) {
	/* START ALPHA CHANGE 2225 -- reject overload before decoding or signature work -- */
	if status := s.limits.admit(); status != 0 {
		writeLimitResponse(w, status, time.Second)

		return
	}
	defer s.limits.release()

	if r.Context().Err() != nil {
		return
	}
	/* FINISH ALPHA CHANGE 2225 -- reject overload before decoding or signature work -- */

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")

		return
	}

	/* START ALPHA CHANGE 1127 -- limit peer directory JSON decode body size -- */
	r.Body = http.MaxBytesReader(w, r.Body, maxExchangeRequestBodyBytes)
	/* FINISH ALPHA CHANGE 1127 -- limit peer directory JSON decode body size -- */

	var req ExchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}

	peerID, err := s.validateExchangeRequest(&req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())

		return
	}

	/* START ALPHA CHANGE 2225 -- rate-limit verified identities before registry access -- */
	if r.Context().Err() != nil {
		return
	}

	if status := s.limits.allowPeer(peerID); status != 0 {
		retryAfter := time.Second
		if status == http.StatusTooManyRequests {
			retryAfter = s.limits.config.PeerInterval
		}

		writeLimitResponse(w, status, retryAfter)

		return
	}
	/* FINISH ALPHA CHANGE 2225 -- rate-limit verified identities before registry access -- */

	if s.verifier == nil {
		writeError(w, http.StatusForbidden, errPeerNotAllowed.Error())

		return
	}

	/* START ALPHA CHANGE 2218 -- consume exchange nonces only after registry authorization -- */
	peers, err := s.storeAndListPeers(peerID, req.Multiaddr, req.Nonce)
	if err != nil {
		if errors.Is(err, errPeerNotAllowed) {
			writeError(w, http.StatusForbidden, errPeerNotAllowed.Error())

			return
		}

		if errors.Is(err, errNonceAlreadyUsed) {
			writeError(w, http.StatusUnauthorized, errNonceAlreadyUsed.Error())

			return
		}

		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}
	/* FINISH ALPHA CHANGE 2218 -- consume exchange nonces only after registry authorization -- */

	writeJSON(w, http.StatusOK, &ExchangeResponse{Peers: peers})
}

func (s *Server) validateExchangeRequest(req *ExchangeRequest) (peer.ID, error) {
	if req.PeerID == "" {
		return "", errMissingPeerID
	}

	if req.Multiaddr == "" {
		return "", errMissingMultiaddr
	}

	if req.PublicKey == "" {
		return "", errMissingPublicKey
	}

	if req.Nonce == "" {
		return "", errMissingNonce
	}

	if req.Signature == "" {
		return "", errMissingSignature
	}

	if time.Since(time.Unix(req.Timestamp, 0)) > maxClockSkew ||
		time.Until(time.Unix(req.Timestamp, 0)) > maxClockSkew {
		return "", errStaleTimestamp
	}

	peerID, err := peer.Decode(req.PeerID)
	if err != nil {
		return "", err
	}

	peerInfo, err := common.StringToAddrInfo(req.Multiaddr)
	if err != nil {
		return "", err
	}

	/* START ALPHA CHANGE 2833 -- reject addressless directory exchange requests -- */
	if len(peerInfo.Addrs) == 0 {
		return "", errors.New("multiaddr has no transport address")
	}
	/* FINISH ALPHA CHANGE 2833 -- reject addressless directory exchange requests -- */

	if peerInfo.ID != peerID {
		return "", errMultiaddrPeerID
	}

	publicKeyBytes, err := hex.DecodeString(req.PublicKey)
	if err != nil {
		return "", err
	}

	publicKey, err := libp2pCrypto.UnmarshalPublicKey(publicKeyBytes)
	if err != nil {
		return "", err
	}

	publicKeyPeerID, err := peer.IDFromPublicKey(publicKey)
	if err != nil {
		return "", err
	}

	if publicKeyPeerID != peerID {
		return "", errPeerIDMismatch
	}

	signature, err := hex.DecodeString(req.Signature)
	if err != nil {
		return "", err
	}

	ok, err := publicKey.Verify(peerDirectorySignatureMessage(req), signature)
	if err != nil {
		return "", err
	}

	if !ok {
		return "", errInvalidSignature
	}

	/* START ALPHA CHANGE 2219 -- keep request validation free of nonce-cache mutations -- */
	// Registry authorization must succeed before the nonce is consumed.
	return peerID, nil
	/* FINISH ALPHA CHANGE 2219 -- keep request validation free of nonce-cache mutations -- */
}

func (s *Server) consumeNonce(peerID peer.ID, nonce string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.pruneExpiredNonces()

	nonceKey := fmt.Sprintf("%s:%s", peerID, nonce)
	if _, exists := s.usedNonces[nonceKey]; exists {
		return false
	}

	s.usedNonces[nonceKey] = time.Now().UTC().Add(nonceTTL)

	return true
}

func (s *Server) pruneExpiredNonces() {
	now := time.Now().UTC()

	for nonce, expiresAt := range s.usedNonces {
		if now.After(expiresAt) {
			delete(s.usedNonces, nonce)
		}
	}
}

/* START ALPHA CHANGE 1358 -- replace peer directory state from one registry snapshot -- */
/* START ALPHA CHANGE 2220 -- authorize before scanning or inserting exchange nonces -- */
func (s *Server) storeAndListPeers(peerID peer.ID, multiaddr, nonce string) ([]string, error) {
	s.exchangeLock.Lock()
	defer s.exchangeLock.Unlock()

	allowedPeerIDs, err := s.verifier.GetAllowedValidatorPeerIDs()
	if err != nil {
		return nil, err
	}

	if _, ok := allowedPeerIDs[peerID]; !ok {
		return nil, errPeerNotAllowed
	}

	// consumeNonce performs an atomic replay check and insertion under
	// s.lock. Call it before acquiring s.lock for the peer-map update.
	if !s.consumeNonce(peerID, nonce) {
		return nil, errNonceAlreadyUsed
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	/* FINISH ALPHA CHANGE 2220 -- authorize before scanning or inserting exchange nonces -- */

	candidates := make(map[peer.ID]string, len(s.peers)+1)
	for storedPeerID, storedMultiaddr := range s.peers {
		candidates[storedPeerID] = storedMultiaddr
	}
	candidates[peerID] = multiaddr

	retained := make(map[peer.ID]string, len(candidates))
	for storedPeerID, storedMultiaddr := range candidates {
		if _, ok := allowedPeerIDs[storedPeerID]; ok {
			retained[storedPeerID] = storedMultiaddr
		}
	}

	/* START ALPHA CHANGE 1913 -- durably save the registry-filtered peer directory map -- */
	if err := s.persistPeerCacheLocked(retained); err != nil {
		return nil, fmt.Errorf("failed to persist peer directory cache: %w", err)
	}
	/* FINISH ALPHA CHANGE 1913 -- durably save the registry-filtered peer directory map -- */

	s.peers = retained

	peers := make([]string, 0, len(s.peers))
	for _, peerAddr := range s.peers {
		peers = append(peers, peerAddr)
	}

	return peers, nil
}

/* FINISH ALPHA CHANGE 1358 -- replace peer directory state from one registry snapshot */

func peerDirectorySignatureMessage(req *ExchangeRequest) []byte {
	// START ALPHA CHANGE 1020 -- use fmt.Appendf for peer directory service signature message --
	return fmt.Appendf(
		nil,
		"innbc-peer-directory-v1\n%s\n%s\n%s\n%d\n%s",
		req.PeerID,
		req.Multiaddr,
		req.PublicKey,
		req.Timestamp,
		req.Nonce,
	)
	// FINISH ALPHA CHANGE 1020 -- use fmt.Appendf for peer directory service signature message --
}

func writeJSON(w http.ResponseWriter, statusCode int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}
