// START ALPHA CHANGE 3794 -- test singleton startup and restart without directory availability --
package server

import (
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi/testutil"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/network"
	networkcommon "github.com/innovativebioresearch/innbc-chain/network/common"
	secretshelper "github.com/innovativebioresearch/innbc-chain/secrets/helper"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type singleDirectoryFixture struct {
	dataDir   string
	chainPath string
	genesis   []byte
	account   *wallet.Account
	keyFiles  map[string][]byte
	directory *httptest.Server
	available atomic.Bool
	offered   atomic.Value
	requests  atomic.Uint64
}

func newSingleDirectoryFixture(t *testing.T) *singleDirectoryFixture {
	t.Helper()

	root := t.TempDir()
	f := &singleDirectoryFixture{
		dataDir:   filepath.Join(root, "node1"),
		chainPath: filepath.Join(root, "genesis.json"),
		keyFiles:  make(map[string][]byte),
	}
	validators := validator.NewTestValidators(t, 1)
	f.account = validators.GetPrivateIdentities()[0]
	initial := validators.GetParamValidators()

	manager, err := secretshelper.SetupLocalSecretsManager(f.dataDir)
	require.NoError(t, err)
	require.NoError(t, f.account.Save(manager))
	_, err = secretshelper.InitNetworkingPrivateKey(manager)
	require.NoError(t, err)
	peerID, err := secretshelper.LoadNodeID(manager)
	require.NoError(t, err)
	require.NotEmpty(t, peerID)

	initial[0].MultiAddr = "/ip4/127.0.0.1/tcp/30301/p2p/" + peerID
	token := *innbcbft.DefaultTokenConfig
	consensusConfig := &innbcbft.INNBCBFTConfig{
		InitialValidatorSet: initial,
		BlockTime:           common.Duration{Duration: time.Second},
		EpochSize:           2,
		SprintSize:          1,
		NativeTokenConfig:   &token,
		MinValidatorSetSize: 1,
		MaxValidatorSetSize: 100,
		BlockTrackerPollInterval: common.Duration{
			Duration: time.Second,
		},
		ValidatorRegistryConfig: &innbcbft.ValidatorRegistryConfig{
			GenesisValidator:       initial[0].Address,
			MinimumStake:           big.NewInt(1),
			ProposalDurationPreset: innbcbft.ValidatorRegistryProposalDurationStandard,
			ActivationEpoch:        2,
		},
	}
	metadata, err := innbcbft.BuildGenesisValidatorSet(consensusConfig)
	require.NoError(t, err)
	alloc := testutil.GenesisAllocations(t)
	alloc[f.account.Address()] = &chain.GenesisAccount{
		Balance: new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil),
	}

	config := &chain.Chain{
		Name: "single-validator-directory-regression",
		Genesis: &chain.Genesis{
			Timestamp:          1_700_000_000,
			GasLimit:           5_242_880,
			GasUsed:            458_752,
			Mixhash:            innbcbft.INNBCBFTMixDigest,
			BaseFee:            chain.GenesisBaseFee,
			BaseFeeEM:          chain.GenesisBaseFeeEM,
			BaseFeeChangeDenom: chain.BaseFeeChangeDenom,
			ExtraData:          innbcbft.BuildGenesisExtraData(metadata),
			Alloc:              alloc,
		},
		Params: &chain.Params{
			ChainID: 1982,
			Forks:   chain.AllForksEnabled.Copy(),
			Engine: map[string]interface{}{
				innbcbft.ConsensusName: consensusConfig,
			},
		},
	}
	f.genesis, err = json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f.chainPath, f.genesis, 0600))

	for _, path := range []string{
		datadir.ValidatorKey(f.dataDir),
		datadir.ValidatorBLSKey(f.dataDir),
		datadir.LibP2PKey(f.dataDir),
	} {
		f.keyFiles[path], err = os.ReadFile(path)
		require.NoError(t, err)
		require.NotEmpty(t, f.keyFiles[path])
	}

	f.offered.Store("")
	f.directory = httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		f.requests.Add(1)
		if !f.available.Load() {
			http.Error(w, "directory deliberately unavailable",
				http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string][]string{
			"peers": {f.offered.Load().(string)},
		})
	}))
	t.Cleanup(f.directory.Close)
	return f
}

func singleDirectoryClose(t *testing.T, node *Server) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		node.Close()
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("single-validator server did not close")
	}
}

func (f *singleDirectoryFixture) start(t *testing.T, enabled bool) *Server {
	t.Helper()

	// Import fresh decoded maps on each start. The preceding server may
	// have populated its own in-memory genesis state root.
	config, err := chain.ImportFromFile(f.chainPath)
	require.NoError(t, err)
	networkConfig := network.DefaultConfig()
	networkConfig.Addr = &net.TCPAddr{IP: net.ParseIP("127.0.0.1")}
	networkConfig.NoDiscover = true
	networkConfig.PeerDirectoryEnabled = enabled
	networkConfig.PeerDirectoryURL = f.directory.URL + "/api/v1/peers/exchange"

	node, err := NewServer(&Config{
		Chain: config,
		JSONRPC: &JSONRPC{
			JSONRPCAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1")},
		},
		GRPCAddr:           &net.TCPAddr{IP: net.ParseIP("127.0.0.1")},
		Telemetry:          &Telemetry{},
		Network:            networkConfig,
		DataDir:            f.dataDir,
		Seal:               true,
		PriceLimit:         1,
		MaxSlots:           4096,
		MaxAccountEnqueued: 128,
		LogLevel:           hclog.Error,
		MetricsInterval:    time.Second,
		History: &HistoryConfig{
			Mode:              "full",
			MDBXMapSizeGB:     1,
			MDBXPageSizeBytes: 4096,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { singleDirectoryClose(t, node) })
	return node
}

func singleDirectoryWaitHeight(t *testing.T, node *Server, height uint64) {
	t.Helper()

	require.Eventually(t, func() bool {
		header := node.blockchain.Header()
		return header != nil && header.Number >= height
	}, 30*time.Second, 20*time.Millisecond,
		"single validator did not commit through block %d", height)
}

func (f *singleDirectoryFixture) assertPreserved(t *testing.T) {
	t.Helper()

	actual, err := os.ReadFile(f.chainPath)
	require.NoError(t, err)
	require.Equal(t, f.genesis, actual)
	for path, expected := range f.keyFiles {
		actual, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, expected, actual, "identity changed at %s", path)
	}
}

func singleDirectoryReceipt(
	t *testing.T,
	node *Server,
	hash types.Hash,
) types.Hash {
	t.Helper()

	var blockHash types.Hash
	var receipt *types.Receipt
	require.Eventually(t, func() bool {
		var found bool
		blockHash, found = node.blockchain.ReadTxLookup(hash)
		if !found {
			return false
		}
		receipts, err := node.blockchain.GetReceiptsByHash(blockHash)
		if err != nil {
			return false
		}
		for _, candidate := range receipts {
			if candidate != nil && candidate.TxHash == hash {
				receipt = candidate
				return true
			}
		}
		return false
	}, 30*time.Second, 20*time.Millisecond,
		"funded transaction was not committed")

	require.NotNil(t, receipt.Status)
	require.Equal(t, types.ReceiptSuccess, *receipt.Status)
	return blockHash
}

// No t.Parallel: fork handlers and header hashing are process-global.
func TestSingleValidatorStartup_DirectoryFailureAndRestart(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "directory_disabled_control"
		if enabled {
			name = "directory_enabled_but_unavailable"
		}

		t.Run(name, func(t *testing.T) {
			f := newSingleDirectoryFixture(t)
			node := f.start(t, enabled)
			genesisHash := node.blockchain.Genesis()

			// Commit blocks 1, 2 (epoch end) and 3 without a successful
			// directory response, refresh, env file or remote peer.
			singleDirectoryWaitHeight(t, node, 3)
			for number := uint64(1); number <= 3; number++ {
				header, ok := node.blockchain.GetHeaderByNumber(number)
				require.True(t, ok)
				parent, ok := node.blockchain.GetHeaderByNumber(number - 1)
				require.True(t, ok)
				require.Equal(t, parent.Hash, header.ParentHash)
				require.Equal(t, parent.Timestamp+1, header.Timestamp)
			}
			boundary, ok := node.blockchain.GetBlockByNumber(2, true)
			require.True(t, ok)
			require.Len(t, boundary.Transactions, 1)
			require.Equal(t, types.StateTx, boundary.Transactions[0].Type)
			require.Empty(t, node.network.Peers())
			if enabled {
				require.Greater(t, f.requests.Load(), uint64(0))
			} else {
				require.Zero(t, f.requests.Load())
			}

			key, err := f.account.GetEcdsaPrivateKey()
			require.NoError(t, err)
			recipient := types.StringToAddress("0x123456")
			tx, err := crypto.NewEIP155Signer(1982, true).SignTx(
				&types.Transaction{
					Nonce:    0,
					To:       &recipient,
					Gas:      21_000,
					GasPrice: big.NewInt(1_000_000_000),
					Value:    big.NewInt(1),
				}, key)
			require.NoError(t, err)
			tx.ComputeHash(1)
			require.NoError(t, node.txpool.AddTx(tx))
			receiptBlock := singleDirectoryReceipt(t, node, tx.Hash)

			if enabled {
				address, err := networkcommon.AddrInfoToString(node.network.AddrInfo())
				require.NoError(t, err)
				f.offered.Store(address)
				f.available.Store(true)
				response, err := node.RefreshPeerDirectory()
				require.NoError(t, err)
				require.Equal(t, []string{address}, response.Peers)
				singleDirectoryWaitHeight(t, node, node.blockchain.Header().Number+2)
				f.available.Store(false)
			}

			f.assertPreserved(t)
			singleDirectoryClose(t, node)

			// Capture the head after shutdown. Commits during shutdown
			// must not count as progress after restart.
			finalHeight := node.blockchain.Header().Number
			requestsBeforeRestart := f.requests.Load()

			restarted := f.start(t, enabled)
			require.Equal(t, genesisHash, restarted.blockchain.Genesis())
			singleDirectoryWaitHeight(t, restarted, finalHeight+2)
			require.Equal(t, receiptBlock,
				singleDirectoryReceipt(t, restarted, tx.Hash))

			if enabled {
				require.Greater(t, f.requests.Load(), requestsBeforeRestart)
			} else {
				require.Zero(t, f.requests.Load())
			}
			f.assertPreserved(t)
		})
	}
}

// FINISH ALPHA CHANGE 3794 -- test singleton startup and restart without directory availability --
