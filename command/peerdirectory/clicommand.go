package peerdirectory

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/command/peerdirectory/refresh"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/network"
	networkcommon "github.com/innovativebioresearch/innbc-chain/network/common"
	networkpeerdirectory "github.com/innovativebioresearch/innbc-chain/network/peerdirectory"
	"github.com/innovativebioresearch/innbc-chain/secrets"
	"github.com/innovativebioresearch/innbc-chain/secrets/local"
	"github.com/innovativebioresearch/innbc-chain/types"
	libp2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
)

const (
	listenFlag            = "listen"
	jsonRPCFlag           = "jsonrpc"
	validatorRegistryFlag = "validator-registry"

	/* START ALPHA CHANGE 1919 -- bound peer directory HTTP connections and request processing -- */
	peerDirectoryReadHeaderTimeout = 10 * time.Second
	peerDirectoryReadTimeout       = 15 * time.Second
	peerDirectoryWriteTimeout      = 30 * time.Second
	peerDirectoryIdleTimeout       = 60 * time.Second
	peerDirectoryMaxHeaderBytes    = 16 * 1024
	/* FINISH ALPHA CHANGE 1919 -- bound peer directory HTTP connections and request processing -- */

	/* START ALPHA CHANGE 705 -- load peer directory bootstrap validators from chain config -- */
	chainFlag = "chain"
	/* FINISH ALPHA CHANGE 705 -- load peer directory bootstrap validators from chain config -- */

	/* START ALPHA CHANGE 1914 -- configure the persistent peer address cache -- */
	cacheFileFlag = "cache-file"
	/* FINISH ALPHA CHANGE 1914 -- configure the persistent peer address cache -- */

	/* START ALPHA CHANGE 701 -- add manual peer directory signed exchange flags -- */
	exchangeURLFlag       = "url"
	exchangeDataDirFlag   = "data-dir"
	exchangeMultiaddrFlag = "multiaddr"
	/* FINISH ALPHA CHANGE 701 -- add manual peer directory signed exchange flags -- */
)

type params struct {
	listenAddr               string
	jsonRPCAddr              string
	validatorRegistryAddress string

	/* START ALPHA CHANGE 2229 -- configure native directory admission limits -- */
	limits networkpeerdirectory.Limits
	/* FINISH ALPHA CHANGE 2229 -- configure native directory admission limits -- */

	/* START ALPHA CHANGE 705 -- load peer directory bootstrap validators from chain config -- */
	chainPath string
	/* FINISH ALPHA CHANGE 705 -- load peer directory bootstrap validators from chain config -- */

	/* START ALPHA CHANGE 1915 -- retain the peer directory cache path -- */
	cacheFilePath string
	/* FINISH ALPHA CHANGE 1915 -- retain the peer directory cache path -- */
}

var directoryParams = &params{
	listenAddr:    "127.0.0.1:8080",
	jsonRPCAddr:   "http://127.0.0.1:8545",
	cacheFilePath: "peer-directory-peers.json",
	/* START ALPHA CHANGE 2229 -- enable default limits for every deployment -- */
	limits: networkpeerdirectory.DefaultLimits(),
	/* FINISH ALPHA CHANGE 2229 -- enable default limits for every deployment -- */
}

/* START ALPHA CHANGE 702 -- add manual peer directory signed exchange params -- */
type exchangeParams struct {
	url       string
	dataDir   string
	multiaddr string
}

var directoryExchangeParams = &exchangeParams{
	url: "http://127.0.0.1:8080/api/v1/peers/exchange",
}

/* FINISH ALPHA CHANGE 702 -- add manual peer directory signed exchange params -- */

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "peerdirectory",
		Short: "Runs the authenticated INNBC validator peer directory API",
		Args:  cobra.NoArgs,
		RunE:  runCommand,
	}

	cmd.Flags().StringVar(
		&directoryParams.listenAddr,
		listenFlag,
		directoryParams.listenAddr,
		"the address and port for the INNBC validator peer directory API",
	)

	cmd.Flags().StringVar(
		&directoryParams.jsonRPCAddr,
		jsonRPCFlag,
		directoryParams.jsonRPCAddr,
		"the JSON-RPC endpoint used to read the INNBC validator registry",
	)

	cmd.Flags().StringVar(
		&directoryParams.validatorRegistryAddress,
		validatorRegistryFlag,
		"",
		"the INNBC validator registry contract address",
	)

	/* START ALPHA CHANGE 706 -- require chain config for bootstrap validator peer directory checks -- */
	cmd.Flags().StringVar(
		&directoryParams.chainPath,
		chainFlag,
		"",
		"the genesis chain config used to read INNBC bootstrap validators",
	)
	/* FINISH ALPHA CHANGE 706 -- require chain config for bootstrap validator peer directory checks -- */

	/* START ALPHA CHANGE 1916 -- expose the persistent peer address cache path -- */
	cmd.Flags().StringVar(
		&directoryParams.cacheFilePath,
		cacheFileFlag,
		directoryParams.cacheFilePath,
		"the JSON file used to persist authenticated validator peer addresses",
	)
	/* FINISH ALPHA CHANGE 1916 -- expose the persistent peer address cache path -- */

	/* START ALPHA CHANGE 2229 -- expose validated request admission settings -- */
	cmd.Flags().IntVar(
		&directoryParams.limits.GlobalRPS,
		"request-rate",
		directoryParams.limits.GlobalRPS,
		"maximum sustained directory request admissions per second",
	)
	cmd.Flags().IntVar(
		&directoryParams.limits.GlobalBurst,
		"request-burst",
		directoryParams.limits.GlobalBurst,
		"global directory request burst allowance",
	)
	cmd.Flags().DurationVar(
		&directoryParams.limits.PeerInterval,
		"peer-request-interval",
		directoryParams.limits.PeerInterval,
		"refill interval for one verified-identity request token",
	)
	cmd.Flags().IntVar(
		&directoryParams.limits.PeerBurst,
		"peer-request-burst",
		directoryParams.limits.PeerBurst,
		"verified-identity request burst allowance",
	)
	cmd.Flags().IntVar(
		&directoryParams.limits.MaxConcurrent,
		"max-concurrent-exchanges",
		directoryParams.limits.MaxConcurrent,
		"maximum admitted exchanges including registry waiters",
	)
	cmd.Flags().IntVar(
		&directoryParams.limits.MaxPeers,
		"max-tracked-peers",
		directoryParams.limits.MaxPeers,
		"maximum verified identities retained in rate-limiter memory",
	)
	/* FINISH ALPHA CHANGE 2229 -- expose validated request admission settings -- */

	_ = cmd.MarkFlagRequired(validatorRegistryFlag)
	/* START ALPHA CHANGE 706 -- require chain config for bootstrap validator peer directory checks -- */
	_ = cmd.MarkFlagRequired(chainFlag)
	/* FINISH ALPHA CHANGE 706 -- require chain config for bootstrap validator peer directory checks -- */

	/* START ALPHA CHANGE 1222 -- register manual peer directory operator commands -- */
	cmd.AddCommand(
		getExchangeCommand(),
		refresh.GetCommand(),
	)
	/* FINISH ALPHA CHANGE 1222 -- register manual peer directory operator commands -- */

	return cmd
}

/* START ALPHA CHANGE 1920 -- serve the peer directory with bounded HTTP resources -- */

func newPeerDirectoryHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: peerDirectoryReadHeaderTimeout,
		ReadTimeout:       peerDirectoryReadTimeout,
		WriteTimeout:      peerDirectoryWriteTimeout,
		IdleTimeout:       peerDirectoryIdleTimeout,
		MaxHeaderBytes:    peerDirectoryMaxHeaderBytes,
	}
}

/* FINISH ALPHA CHANGE 1920 -- serve the peer directory with bounded HTTP resources -- */

func runCommand(_ *cobra.Command, _ []string) error {
	/* START ALPHA CHANGE 2229 -- validate limits and registry transport before listening -- */
	if err := directoryParams.limits.Validate(); err != nil {
		return err
	}

	rpcClient, err := newRegistryRPCClient(directoryParams.jsonRPCAddr)
	if err != nil {
		return fmt.Errorf("failed to configure registry JSON-RPC endpoint: %w", err)
	}
	/* FINISH ALPHA CHANGE 2229 -- validate limits and registry transport before listening -- */

	/* START ALPHA CHANGE 707 -- use genesis validators until registry activation epoch -- */
	innbcBFTConfig, err := innbcbft.LoadINNBCBFTConfig(directoryParams.chainPath)
	if err != nil {
		return fmt.Errorf("failed to load INNBCBFT chain config: %w", err)
	}

	bootstrapPeerIDs, err := buildBootstrapPeerIDs(innbcBFTConfig)
	if err != nil {
		return err
	}

	if innbcBFTConfig.EpochSize == 0 {
		return fmt.Errorf("invalid INNBCBFT epoch size: 0")
	}

	// ALPHA CHANGE 2229: use the directory-specific bounded HTTP transport.
	ethClient := rpcClient

	verifier := &validatorRegistryVerifier{
		registryState: innbcbft.NewValidatorRegistryState(
			types.StringToAddress(directoryParams.validatorRegistryAddress),
			&jsonRPCContractProvider{client: ethClient},
		),
		eth:              ethClient,
		epochSize:        innbcBFTConfig.EpochSize,
		bootstrapPeerIDs: bootstrapPeerIDs,
	}
	/* FINISH ALPHA CHANGE 707 -- use genesis validators until registry activation epoch -- */

	/* START ALPHA CHANGE 2229 -- apply configured admission limits -- */
	directoryServer, err := networkpeerdirectory.NewServerWithLimits(
		verifier,
		directoryParams.limits,
	)
	if err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2229 -- apply configured admission limits -- */

	/* START ALPHA CHANGE 1917 -- restore the persistent peer address cache before serving requests -- */
	if err := directoryServer.EnablePeerCache(directoryParams.cacheFilePath); err != nil {
		return fmt.Errorf("failed to initialize peer directory cache: %w", err)
	}
	/* FINISH ALPHA CHANGE 1917 -- restore the persistent peer address cache before serving requests -- */

	/* START ALPHA CHANGE 2229 -- open the listener after successful configuration -- */
	listener, err := net.Listen("tcp", directoryParams.listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	fmt.Printf("INNBC validator peer directory running on http://%s%s\n", directoryParams.listenAddr, networkpeerdirectory.ExchangePath)

	httpServer := newPeerDirectoryHTTPServer(directoryServer.Handler())

	return httpServer.Serve(listener)
	/* FINISH ALPHA CHANGE 2229 -- open the listener after successful configuration -- */
}

/* START ALPHA CHANGE 704 -- add manual signed peer directory exchange command -- */
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

func getExchangeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exchange",
		Short: "Runs one signed INNBC validator peer directory exchange and prints returned peers",
		Args:  cobra.NoArgs,
		RunE:  runExchangeCommand,
	}

	cmd.Flags().StringVar(
		&directoryExchangeParams.url,
		exchangeURLFlag,
		directoryExchangeParams.url,
		"the authenticated INNBC validator peer directory exchange endpoint",
	)

	cmd.Flags().StringVar(
		&directoryExchangeParams.dataDir,
		exchangeDataDirFlag,
		"",
		/* START ALPHA CHANGE 1634 -- document canonical libp2p identity location -- */
		"the node data directory containing keys/libp2p/libp2p.key",
		/* FINISH ALPHA CHANGE 1634 -- document canonical libp2p identity location -- */
	)

	cmd.Flags().StringVar(
		&directoryExchangeParams.multiaddr,
		exchangeMultiaddrFlag,
		"",
		"the validator libp2p multiaddr to register",
	)

	_ = cmd.MarkFlagRequired(exchangeDataDirFlag)
	_ = cmd.MarkFlagRequired(exchangeMultiaddrFlag)

	return cmd
}

/* START ALPHA CHANGE 2229 -- retry temporary overload for explicit CLI exchanges too -- */
func runExchangeCommand(cmd *cobra.Command, _ []string) error {
	networkKey, err := readExchangeNetworkKey(directoryExchangeParams.dataDir)
	if err != nil {
		return err
	}

	respBody, err := networkpeerdirectory.Exchange(
		cmd.Context(),
		directoryExchangeParams.url,
		func() ([]byte, error) {
			reqBody, err := buildExchangeRequest(
				networkKey,
				directoryExchangeParams.multiaddr,
			)
			if err != nil {
				return nil, err
			}

			return json.Marshal(reqBody)
		},
	)
	if err != nil {
		return err
	}

	var exchangeResp peerDirectoryExchangeResponse
	if err := json.Unmarshal(respBody, &exchangeResp); err != nil {
		return err
	}

	printedResponse, err := json.MarshalIndent(exchangeResp, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(printedResponse))

	return nil
}

/* FINISH ALPHA CHANGE 2229 -- retry temporary overload for explicit CLI exchanges too -- */

func readExchangeNetworkKey(dataDir string) (libp2pCrypto.PrivKey, error) {
	/* START ALPHA CHANGE 3234 -- preserve local peer-directory signing identity loading -- */
	secretsManager, err := local.SecretsManagerFactory(&secrets.SecretsManagerParams{
		Logger: hclog.NewNullLogger(),
		Extra: map[string]interface{}{
			secrets.Path: dataDir,
		},
	})
	/* FINISH ALPHA CHANGE 3234 -- preserve local peer-directory signing identity loading -- */
	if err != nil {
		return nil, err
	}

	return network.ReadLibp2pKey(secretsManager)
}

func buildExchangeRequest(networkKey libp2pCrypto.PrivKey, multiaddr string) (*peerDirectoryExchangeRequest, error) {
	peerInfo, err := networkcommon.StringToAddrInfo(multiaddr)
	if err != nil {
		return nil, err
	}

	publicKeyBytes, err := libp2pCrypto.MarshalPublicKey(networkKey.GetPublic())
	if err != nil {
		return nil, err
	}

	req := &peerDirectoryExchangeRequest{
		PeerID:    peerInfo.ID.String(),
		Multiaddr: multiaddr,
		PublicKey: hex.EncodeToString(publicKeyBytes),
		Timestamp: time.Now().UTC().Unix(),
		Nonce:     newExchangeNonce(),
	}

	signature, err := networkKey.Sign(peerDirectoryExchangeSignatureMessage(req))
	if err != nil {
		return nil, err
	}

	req.Signature = hex.EncodeToString(signature)

	return req, nil
}

func peerDirectoryExchangeSignatureMessage(req *peerDirectoryExchangeRequest) []byte {
	// START ALPHA CHANGE 1013 -- build peer directory signature message bytes directly --
	return fmt.Appendf(nil,
		"innbc-peer-directory-v1\n%s\n%s\n%s\n%d\n%s",
		req.PeerID,
		req.Multiaddr,
		req.PublicKey,
		req.Timestamp,
		req.Nonce,
	)
	// FINISH ALPHA CHANGE 1013 -- build peer directory signature message bytes directly --
}

func newExchangeNonce() string {
	nonce := make([]byte, 16)

	if _, err := rand.Read(nonce); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}

	return hex.EncodeToString(nonce)
}

/* FINISH ALPHA CHANGE 704 -- add manual signed peer directory exchange command -- */

/* START ALPHA CHANGE 700 -- add read-only JSON-RPC contract provider for peer directory registry checks -- */
type jsonRPCContractProvider struct {
	// ALPHA CHANGE 2229: bound registry HTTP request lifetime.
	client *registryRPCClient
}

func (p *jsonRPCContractProvider) Call(addr ethgo.Address, input []byte, opts *contract.CallOpts) ([]byte, error) {
	block := ethgo.Latest
	from := ethgo.ZeroAddress

	if opts != nil {
		block = opts.Block
		from = opts.From
	}

	rawResult, err := p.client.Call(&ethgo.CallMsg{
		From: from,
		To:   &addr,
		Data: input,
	}, block)
	if err != nil {
		return nil, err
	}

	return hex.DecodeString(rawResult[2:])
}

func (p *jsonRPCContractProvider) Txn(ethgo.Address, ethgo.Key, []byte) (contract.Txn, error) {
	return nil, fmt.Errorf("peer directory registry verifier is read-only")
}

var _ contract.Provider = (*jsonRPCContractProvider)(nil)

/* FINISH ALPHA CHANGE 700 -- add read-only JSON-RPC contract provider for peer directory registry checks -- */

/* START ALPHA CHANGE 708 -- mirror consensus bootstrap validator source of truth in peer directory -- */
type validatorRegistryVerifier struct {
	registryState innbcbft.ValidatorRegistryState
	// ALPHA CHANGE 2229: use the same bounded transport for block-number reads.
	eth       *registryRPCClient
	epochSize uint64

	bootstrapPeerIDs map[peer.ID]struct{}
}

func buildBootstrapPeerIDs(innbcBFTConfig innbcbft.INNBCBFTConfig) (map[peer.ID]struct{}, error) {
	bootstrapPeerIDs := make(map[peer.ID]struct{}, len(innbcBFTConfig.InitialValidatorSet))

	/* START ALPHA CHANGE 3516 -- reject nil genesis validators before bootstrap peer extraction -- */
	for index, genesisValidator := range innbcBFTConfig.InitialValidatorSet {
		if genesisValidator == nil {
			return nil, fmt.Errorf("nil genesis validator at index %d", index)
		}
		/* FINISH ALPHA CHANGE 3516 -- reject nil genesis validators before bootstrap peer extraction -- */

		peerInfo, err := networkcommon.StringToAddrInfo(genesisValidator.MultiAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse genesis validator multiaddr %q: %w", genesisValidator.MultiAddr, err)
		}

		bootstrapPeerIDs[peerInfo.ID] = struct{}{}
	}

	return bootstrapPeerIDs, nil
}

/* START ALPHA CHANGE 1359 -- return one peer directory registry allowlist snapshot -- */
func (v *validatorRegistryVerifier) IsValidatorPeerAllowed(peerID peer.ID) (bool, error) {
	allowedPeerIDs, err := v.GetAllowedValidatorPeerIDs()
	if err != nil {
		return false, err
	}

	_, ok := allowedPeerIDs[peerID]

	return ok, nil
}

func (v *validatorRegistryVerifier) GetAllowedValidatorPeerIDs() (map[peer.ID]struct{}, error) {
	registryActivated, err := v.registryState.IsRegistryActivated()
	if err != nil {
		return nil, err
	}

	if !registryActivated {
		return v.bootstrapPeerIDSnapshot(), nil
	}

	activationEpoch, err := v.registryState.GetActivationEpoch()
	if err != nil {
		return nil, err
	}

	currentEpoch, err := v.currentEpoch()
	if err != nil {
		return nil, err
	}

	if currentEpoch < activationEpoch {
		return v.bootstrapPeerIDSnapshot(), nil
	}

	return v.registryPeerIDSnapshot()
}

func (v *validatorRegistryVerifier) bootstrapPeerIDSnapshot() map[peer.ID]struct{} {
	allowedPeerIDs := make(map[peer.ID]struct{}, len(v.bootstrapPeerIDs))
	for peerID := range v.bootstrapPeerIDs {
		allowedPeerIDs[peerID] = struct{}{}
	}

	return allowedPeerIDs
}

func (v *validatorRegistryVerifier) registryPeerIDSnapshot() (map[peer.ID]struct{}, error) {
	activeValidators, err := v.registryState.GetActiveValidators()
	if err != nil {
		return nil, err
	}

	allowedPeerIDs := make(map[peer.ID]struct{}, len(activeValidators))
	for _, operator := range activeValidators {
		validator, err := v.registryState.GetValidator(operator)
		if err != nil {
			return nil, err
		}

		if !validator.Registered || !validator.Whitelisted {
			continue
		}

		/* START ALPHA CHANGE 2288 -- isolate malformed registry peer IDs from directory availability -- */
		peerID, err := peer.Decode(validator.Libp2pPeerID)
		if err != nil {
			// An unusable discovery identifier must not exclude other validators.
			continue
		}

		allowedPeerIDs[peerID] = struct{}{}
		/* FINISH ALPHA CHANGE 2288 -- isolate malformed registry peer IDs from directory availability -- */
	}

	return allowedPeerIDs, nil
}

/* FINISH ALPHA CHANGE 1359 -- return one peer directory registry allowlist snapshot */

func (v *validatorRegistryVerifier) currentEpoch() (uint64, error) {
	currentBlock, err := v.eth.BlockNumber()
	if err != nil {
		return 0, err
	}

	return currentBlock / v.epochSize, nil
}

var _ networkpeerdirectory.ValidatorVerifier = (*validatorRegistryVerifier)(nil)

/* FINISH ALPHA CHANGE 708 -- mirror consensus bootstrap validator source of truth in peer directory -- */
