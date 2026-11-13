package network

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/innovativebioresearch/innbc-chain/network/common"
	/* START ALPHA CHANGE 2227 -- use the shared bounded directory exchange client -- */
	"github.com/innovativebioresearch/innbc-chain/network/peerdirectory"
	/* FINISH ALPHA CHANGE 2227 -- use the shared bounded directory exchange client -- */
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

/* START ALPHA CHANGE 2237 -- remove the timeout superseded by the shared exchange client -- */
const (
	/* START ALPHA CHANGE 1361 -- cap peer directory successful response bodies -- */
	maxPeerDirectoryResponseBodyBytes int64 = 1024 * 1024
	/* FINISH ALPHA CHANGE 1361 -- cap peer directory successful response bodies -- */
)

/* FINISH ALPHA CHANGE 2237 -- remove the timeout superseded by the shared exchange client -- */

type peerDirectoryExchangeRequest struct {
	PeerID    string `json:"peer_id"`
	Multiaddr string `json:"multiaddr"`
	PublicKey string `json:"public_key"`
	Timestamp int64  `json:"timestamp_unix"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

type peerDirectoryExchangeResponse struct {
	Peers []string `json:"peers"`
}

/* START ALPHA CHANGE 1215 -- expose peer directory refresh result for startup and manual refresh -- */
type PeerDirectoryRefreshResult struct {
	Peers []string
}

/* FINISH ALPHA CHANGE 1215 -- expose peer directory refresh result for startup and manual refresh */

func (s *Server) startPeerDirectoryRefresh() {
	if !s.peerDirectoryRefreshConfigured() {
		return
	}

	/* START ALPHA CHANGE 2744 -- keep directory readiness pending until a validated response -- */
	/* Construction already initialized the restricted admission policy. */
	/* FINISH ALPHA CHANGE 2744 -- keep directory readiness pending until a validated response -- */

	/* START ALPHA CHANGE 1216 -- refresh validator peer directory once at startup -- */
	go func() {
		if _, err := s.refreshPeerDirectory("startup"); err != nil {
			s.logger.Warn("failed to refresh validator peer directory", "source", "startup", "err", err)
		}
	}()
	/* FINISH ALPHA CHANGE 1216 -- refresh validator peer directory once at startup -- */
}

/* START ALPHA CHANGE 1217 -- add manual validator peer directory refresh entrypoint -- */
func (s *Server) RefreshPeerDirectory() (*PeerDirectoryRefreshResult, error) {
	if !s.peerDirectoryRefreshConfigured() {
		return nil, fmt.Errorf("validator peer directory refresh is not enabled for this node")
	}

	return s.refreshPeerDirectory("manual")
}

/* START ALPHA CHANGE 2744 -- share the directory restriction predicate -- */
func (s *Server) peerDirectoryRefreshConfigured() bool {
	return peerDirectoryRestricted(s.config)
}

/* FINISH ALPHA CHANGE 2744 -- share the directory restriction predicate -- */

func (s *Server) refreshPeerDirectory(source string) (*PeerDirectoryRefreshResult, error) {
	/* START ALPHA CHANGE 1368 -- serialize peer directory client refresh application -- */
	s.peerDirectoryRefreshLock.Lock()
	defer s.peerDirectoryRefreshLock.Unlock()
	/* FINISH ALPHA CHANGE 1368 -- serialize peer directory client refresh application -- */

	result, err := s.exchangePeerDirectory()
	if err != nil {
		return nil, err
	}

	s.logger.Info("validator peer directory refreshed", "source", source, "peers", len(result.Peers))

	return result, nil
}

/* FINISH ALPHA CHANGE 1217 -- add manual validator peer directory refresh entrypoint */

/* START ALPHA CHANGE 2227 -- cancel directory requests and overload retries on shutdown -- */
func (s *Server) exchangePeerDirectory() (*PeerDirectoryRefreshResult, error) {
	select {
	case <-s.closeCh:
		return nil, context.Canceled
	default:
	}

	ctx, cancel := context.WithCancel(context.Background())
	watcherDone := make(chan struct{})

	go func() {
		defer close(watcherDone)

		select {
		case <-s.closeCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	defer func() {
		cancel()
		<-watcherDone
	}()

	respBody, err := peerdirectory.Exchange(
		ctx,
		s.config.PeerDirectoryURL,
		func() ([]byte, error) {
			localAddr, err := common.AddrInfoToString(s.AddrInfo())
			if err != nil {
				return nil, err
			}

			reqBody, err := s.buildPeerDirectoryExchangeRequest(localAddr)
			if err != nil {
				return nil, err
			}

			return json.Marshal(reqBody)
		},
	)
	if err != nil {
		return nil, err
	}

	var exchangeResp peerDirectoryExchangeResponse
	if err := json.Unmarshal(respBody, &exchangeResp); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-s.closeCh:
		return nil, context.Canceled
	default:
	}

	result := &PeerDirectoryRefreshResult{
		Peers: exchangeResp.Peers,
	}

	if err := s.applyPeerDirectoryPeers(result.Peers); err != nil {
		return nil, err
	}

	return result, nil
}

/* FINISH ALPHA CHANGE 2227 -- cancel directory requests and overload retries on shutdown -- */

func (s *Server) buildPeerDirectoryExchangeRequest(localAddr string) (*peerDirectoryExchangeRequest, error) {
	nonce, err := newPeerDirectoryNonce()
	if err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 698 -- include libp2p public key in peer directory exchange -- */
	publicKeyBytes, err := crypto.MarshalPublicKey(s.networkKey.GetPublic())
	if err != nil {
		return nil, err
	}

	req := &peerDirectoryExchangeRequest{
		PeerID:    s.host.ID().String(),
		Multiaddr: localAddr,
		PublicKey: hex.EncodeToString(publicKeyBytes),
		Timestamp: time.Now().UTC().Unix(),
		Nonce:     nonce,
	}
	/* FINISH ALPHA CHANGE 698 -- include libp2p public key in peer directory exchange -- */

	signature, err := s.networkKey.Sign(peerDirectorySignatureMessage(req))
	if err != nil {
		return nil, err
	}

	req.Signature = hex.EncodeToString(signature)

	return req, nil
}

/* START ALPHA CHANGE 1364 -- atomically replace peer directory allowlist from validated response -- */
func (s *Server) applyPeerDirectoryPeers(peerAddrs []string) error {
	localPeerID := s.host.ID()
	peerInfosByID := make(map[peer.ID]*peer.AddrInfo, len(peerAddrs)+1)

	for _, peerAddr := range peerAddrs {
		peerInfo, err := common.StringToAddrInfo(peerAddr)
		if err != nil {
			return fmt.Errorf("failed to parse validator peer directory address %q: %w", peerAddr, err)
		}

		/* START ALPHA CHANGE 2831 -- validate every directory address before publishing membership -- */
		if len(peerInfo.Addrs) == 0 {
			return fmt.Errorf("validator peer directory address %q has no transport address", peerAddr)
		}
		/* FINISH ALPHA CHANGE 2831 -- validate every directory address before publishing membership -- */

		peerInfosByID[peerInfo.ID] = peerInfo
	}

	/* START ALPHA CHANGE 2744 -- construct the replacement membership list once -- */
	allowedPeers := make([]peer.ID, 0, len(peerInfosByID)+1)
	allowedPeers = append(allowedPeers, localPeerID)

	for peerID := range peerInfosByID {
		allowedPeers = append(allowedPeers, peerID)
	}
	/* FINISH ALPHA CHANGE 2744 -- construct the replacement membership list once -- */

	/* START ALPHA CHANGE 2744 -- publish validated membership and reject late shutdown responses -- */
	if !s.replaceAllowedPeers(allowedPeers) {
		return context.Canceled
	}
	/* FINISH ALPHA CHANGE 2744 -- publish validated membership and reject late shutdown responses -- */

	for peerID, peerInfo := range peerInfosByID {
		if peerID == localPeerID {
			continue
		}

		s.host.Peerstore().ClearAddrs(peerID)
		s.AddToPeerStore(peerInfo)

		if !s.IsConnected(peerID) {
			s.addToDialQueue(peerInfo, common.PriorityRequestedDial)
		}
	}

	return nil
}

/* FINISH ALPHA CHANGE 1364 -- atomically replace peer directory allowlist from validated response */

func peerDirectorySignatureMessage(req *peerDirectoryExchangeRequest) []byte {
	// START ALPHA CHANGE 1019 -- use fmt.Appendf for validator peer directory signature message --
	return fmt.Appendf(
		nil,
		"innbc-peer-directory-v1\n%s\n%s\n%s\n%d\n%s",
		req.PeerID,
		req.Multiaddr,
		req.PublicKey,
		req.Timestamp,
		req.Nonce,
	)
	// FINISH ALPHA CHANGE 1019 -- use fmt.Appendf for validator peer directory signature message --
}

func newPeerDirectoryNonce() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	return hex.EncodeToString(nonce), nil
}
