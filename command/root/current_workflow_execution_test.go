/* START ALPHA CHANGE 3239 -- exercise the current isolated INNBC network and execution workflow -- */
package root

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

func cwRPC(url, method string, result any, params ...any) error {
	if params == nil {
		params = []any{}
	}

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("RPC HTTP status %d", response.StatusCode)
	}

	return cwDecodeRPC(response.Body, result)
}

func cwRead[T any](t *testing.T, node *cwNode, method string, params ...any) T {
	t.Helper()

	var result T
	require.NoError(t, cwRPC(node.rpc, method, &result, params...))

	return result
}

func cwQuantity(t *testing.T, value string) *big.Int {
	t.Helper()

	require.True(t, strings.HasPrefix(value, "0x"), "invalid quantity %q", value)
	number, ok := new(big.Int).SetString(strings.TrimPrefix(value, "0x"), 16)
	require.True(t, ok, "invalid quantity %q", value)
	require.NotNil(t, number)
	require.GreaterOrEqual(t, number.Sign(), 0)

	return number
}

func cwUint64(t *testing.T, value string) uint64 {
	t.Helper()

	number := cwQuantity(t, value)
	require.True(t, number.IsUint64(), "quantity exceeds uint64")

	return number.Uint64()
}

func cwHex(value []byte) string {
	return "0x" + hex.EncodeToString(value)
}

func cwWord(value *big.Int) string {
	return fmt.Sprintf("0x%064x", value)
}

func cwABI(t *testing.T, signature string, values ...any) []byte {
	t.Helper()

	input, err := abi.MustNewMethod(signature).Encode(values)
	require.NoError(t, err)

	return input
}

func cwCall(
	t *testing.T,
	node *cwNode,
	to types.Address,
	input []byte,
	tag string,
) string {
	t.Helper()

	return cwRead[string](t, node, "eth_call", map[string]any{
		"to":   to.String(),
		"data": cwHex(input),
	}, tag)
}

func cwBalance(t *testing.T, node *cwNode, address, tag string) *big.Int {
	t.Helper()

	return cwQuantity(t, cwRead[string](t, node, "eth_getBalance", address, tag))
}

func cwGetBlock(t *testing.T, node *cwNode, tag string) cwBlock {
	t.Helper()

	block := cwRead[cwBlock](t, node, "eth_getBlockByNumber", tag, false)
	require.NotEmpty(t, block.Hash)

	return block
}

func (f *cwFixture) waitHeight(t *testing.T, nodes []*cwNode, height uint64) {
	t.Helper()

	f.wait(t, fmt.Sprintf("all selected nodes to reach block %d", height), func() (bool, error) {
		for _, node := range nodes {
			var value string
			if err := cwRPC(node.rpc, "eth_blockNumber", &value); err != nil {
				return false, err
			}
			if cwUint64(t, value) < height {
				return false, nil
			}
		}
		return true, nil
	})
}

func (f *cwFixture) receipt(t *testing.T, hash string) cwReceipt {
	t.Helper()

	var receipt cwReceipt
	f.wait(t, "the same receipt on every running node", func() (bool, error) {
		var expected *cwReceipt

		for _, node := range f.nodes {
			if node.process.stopped {
				continue
			}

			/* START ALPHA CHANGE 3251 -- require receipt blocks to be visible through each current canonical head -- */
			var actual *cwReceipt
			if err := cwRPC(node.rpc, "eth_getTransactionReceipt", &actual, hash); err != nil {
				return false, err
			}
			if actual == nil {
				return false, nil
			}

			var latest string
			if err := cwRPC(node.rpc, "eth_blockNumber", &latest); err != nil {
				return false, err
			}
			if cwUint64(t, latest) < cwUint64(t, actual.BlockNumber) {
				return false, fmt.Errorf(
					"node %s head %s has not reached receipt block %s",
					node.rpc, latest, actual.BlockNumber,
				)
			}

			var canonical *cwBlock
			if err := cwRPC(node.rpc, "eth_getBlockByNumber", &canonical,
				actual.BlockNumber, false); err != nil {
				return false, err
			}
			if canonical == nil {
				return false, fmt.Errorf(
					"node %s has no canonical receipt block %s",
					node.rpc, actual.BlockNumber,
				)
			}
			if canonical.Hash != actual.BlockHash {
				return false, fmt.Errorf(
					"node %s canonical hash %s differs from receipt hash %s",
					node.rpc, canonical.Hash, actual.BlockHash,
				)
			}

			if expected == nil {
				expected = actual
			} else if *expected != *actual {
				return false, fmt.Errorf("nodes returned different receipts")
			}
			/* FINISH ALPHA CHANGE 3251 -- require receipt blocks to be visible through each current canonical head -- */
		}

		if expected == nil {
			return false, fmt.Errorf("no running node returned a receipt")
		}
		receipt = *expected
		return true, nil
	})

	require.Equal(t, hash, receipt.Hash)
	require.NotEmpty(t, receipt.BlockHash)

	return receipt
}

func (f *cwFixture) refresh(t *testing.T) {
	t.Helper()

	// The second pass requeues connections after every node has published
	// its membership policy during the first pass.
	for range 2 {
		for _, node := range f.nodes {
			if node.process.stopped {
				continue
			}
			cwRequireCLI(t, f.dir,
				"peerdirectory", "refresh",
				"--grpc-address", node.grpc,
			)
		}
	}
}

func (f *cwFixture) startCluster(t *testing.T, genesisArgs []string) {
	t.Helper()

	directoryAddr, releaseDirectory := cwReserveAddress(t)
	directoryURL := "http://" + directoryAddr + "/api/v1/peers/exchange"

	type ports struct {
		rpc, grpc, p2p string
		release        []func()
	}
	allocated := make([]ports, len(f.nodes))

	for index, node := range f.nodes {
		rpc, releaseRPC := cwReserveAddress(t)
		grpc, releaseGRPC := cwReserveAddress(t)
		p2p, releaseP2P := cwReserveAddress(t)

		allocated[index] = ports{
			rpc: rpc, grpc: grpc, p2p: p2p,
			release: []func(){releaseRPC, releaseGRPC, releaseP2P},
		}
		node.rpc = "http://" + rpc
		node.grpc = grpc
		_, port, err := net.SplitHostPort(p2p)
		require.NoError(t, err)

		node.multiaddr = fmt.Sprintf(
			"/ip4/127.0.0.1/tcp/%s/p2p/%s",
			port, node.info[cwPeerField],
		)

		genesisArgs = append(genesisArgs, "--validators", strings.Join([]string{
			node.multiaddr,
			node.info[cwOperatorField],
			node.info[cwBLSField],
			node.info[cwProofField],
		}, ":"))
	}

	cwRequireCLI(t, f.dir, genesisArgs...)
	var err error
	f.genesis, err = os.ReadFile(f.chain)
	require.NoError(t, err)

	config, err := innbcbft.LoadINNBCBFTConfig(f.chain)
	require.NoError(t, err)
	require.Len(t, config.InitialValidatorSet, len(f.nodes))

	for index, node := range f.nodes {
		validator := config.InitialValidatorSet[index]
		require.Equal(t, node.info[cwOperatorField], validator.Address.String())
		require.Equal(t, node.multiaddr, validator.MultiAddr)
		/* START ALPHA CHANGE 3241 -- compare public CLI values with prefixless genesis BLS fields -- */
		require.Equal(t, strings.TrimPrefix(node.info[cwBLSField], "0x"),
			validator.BlsKey)
		require.Equal(t, strings.TrimPrefix(node.info[cwProofField], "0x"),
			validator.BlsProof)
		/* FINISH ALPHA CHANGE 3241 -- compare public CLI values with prefixless genesis BLS fields -- */

		port := allocated[index]
		node.args = []string{
			"server",
			"--chain", node.env["INNBC_CHAIN"],
			"--data-dir", node.env["INNBC_DATA_DIR"],
			"--jsonrpc", port.rpc,
			"--grpc-address", port.grpc,
			"--libp2p", port.p2p,
			"--history-mode", node.env["INNBC_HISTORY_MODE"],
			"--peer-directory-enabled=true",
			"--peer-directory-url", directoryURL,
			"--no-discover",
			"--seal",
			"--log-level", "ERROR",
			// Bound temporary test databases; provisioning defaults are
			// checked separately and remain unchanged.
			"--mdbx-map-size-gb", "1",
			"--snapshot-source-dir=",
			"--snapshot-bootstrap-required=false",
		}
		if node.env["INNBC_HISTORY_MODE"] == "recent" {
			node.args = append(node.args,
				"--history-retention-blocks", node.env["INNBC_HISTORY_RETENTION_BLOCKS"],
				"--history-segment-blocks", node.env["INNBC_HISTORY_SEGMENT_BLOCKS"],
			)
		}

		for _, release := range port.release {
			release()
		}
		node.process = f.start(t, true, node.args...)
	}

	// RPC genesis access does not require consensus peers.
	for _, node := range f.nodes {
		f.wait(t, "local genesis RPC readiness", func() (bool, error) {
			var block *cwBlock
			err := cwRPC(node.rpc, "eth_getBlockByNumber", &block, "0x0", false)
			return block != nil && block.Hash != "", err
		})
	}

	cachePath := filepath.Join(f.dir, "directory-peers.json")
	releaseDirectory()
	f.start(t, false,
		"peerdirectory",
		"--listen", directoryAddr,
		"--jsonrpc", f.nodes[0].rpc,
		"--validator-registry", contracts.INNBCValidatorRegistryContract.String(),
		"--chain", f.chain,
		"--cache-file", cachePath,
	)

	f.wait(t, "the local peer-directory listener", func() (bool, error) {
		connection, err := net.DialTimeout("tcp", directoryAddr, 200*time.Millisecond)
		if err != nil {
			return false, err
		}
		_ = connection.Close()
		return true, nil
	})

	var lastResponse struct {
		Peers []string `json:"peers"`
	}
	for _, node := range f.nodes {
		output := cwRequireCLI(t, f.dir,
			"peerdirectory", "exchange",
			"--url", directoryURL,
			"--data-dir", node.dir,
			"--multiaddr", node.multiaddr,
		)
		require.NoError(t, json.Unmarshal([]byte(output), &lastResponse))
	}

	expectedPeers := make([]string, len(f.nodes))
	for index, node := range f.nodes {
		expectedPeers[index] = node.multiaddr
	}
	require.ElementsMatch(t, expectedPeers, lastResponse.Peers)

	// A valid local identity outside the genesis committee must be denied.
	outsider := cwProvision(t, 1, "private-local")
	outsiderAddress := "/ip4/127.0.0.1/tcp/30301/p2p/" +
		outsider.nodes[0].info[cwPeerField]
	_, err = cwCLI(t, outsider.dir,
		"peerdirectory", "exchange",
		"--url", directoryURL,
		"--data-dir", outsider.nodes[0].dir,
		"--multiaddr", outsiderAddress,
	)
	require.Error(t, err, "directory admitted an identity outside the committee")

	f.refresh(t)
	f.waitHeight(t, f.nodes, 3)

	expectedGenesis := cwGetBlock(t, f.nodes[0], "0x0").Hash
	expectedBlock := cwGetBlock(t, f.nodes[0], "0x2")
	parent := cwGetBlock(t, f.nodes[0], "0x1")
	require.Equal(t, parent.Hash, expectedBlock.ParentHash)
	/* START ALPHA CHANGE 3328 -- assert the exact configured chain-clock increment -- */
	require.Equal(t,
		cwUint64(t, parent.Timestamp)+uint64(config.BlockTime.Duration/time.Second),
		cwUint64(t, expectedBlock.Timestamp),
		"block timestamp must advance by exactly the configured block interval",
	)
	/* FINISH ALPHA CHANGE 3328 -- assert the exact configured chain-clock increment -- */

	for _, node := range f.nodes {
		require.Equal(t, expectedGenesis, cwGetBlock(t, node, "0x0").Hash)
		require.Equal(t, expectedBlock.Hash, cwGetBlock(t, node, "0x2").Hash)
	}
}

func (f *cwFixture) send(
	t *testing.T,
	key *ecdsa.PrivateKey,
	tx *types.Transaction,
) cwReceipt {
	t.Helper()

	sender := crypto.PubKeyToAddress(&key.PublicKey)
	tx.Nonce = cwUint64(t, cwRead[string](
		t, f.nodes[0], "eth_getTransactionCount", sender.String(), "latest",
	))
	if tx.Value == nil {
		tx.Value = new(big.Int)
	}
	if tx.Gas == 0 {
		tx.Gas = 500_000
	}
	if tx.Type == types.DynamicFeeTx {
		tx.ChainID = big.NewInt(1982)
		if tx.GasFeeCap == nil {
			tx.GasFeeCap = big.NewInt(2_000_000_000)
		}
		if tx.GasTipCap == nil {
			tx.GasTipCap = big.NewInt(100_000_000)
		}
	} else if tx.GasPrice == nil {
		tx.GasPrice = big.NewInt(2_000_000_000)
	}

	/* START ALPHA CHANGE 3252 -- expose nonce disagreements and verify consumption after every sequential transaction -- */
	signer := crypto.NewSigner(chain.AllForksEnabled.At(1), 1982)
	signed, err := signer.SignTx(tx, key)
	require.NoError(t, err)

	// Submit through a different node from the one used for state assertions.
	var hash string
	err = cwRPC(f.nodes[1].rpc, "eth_sendRawTransaction",
		&hash, cwHex(signed.MarshalRLP()))
	if err != nil {
		for _, node := range f.nodes {
			if node.process.stopped {
				continue
			}
			t.Logf("submission nonce=%d sender=%s node=%s latest=%d pending=%d",
				tx.Nonce, sender.String(), node.rpc,
				cwNonce(t, node, sender, "latest"),
				cwNonce(t, node, sender, "pending"))
		}
	}
	require.NoError(t, err, "submit sender %s nonce %d", sender.String(), tx.Nonce)

	receipt := f.receipt(t, hash)
	f.checkFees(t, sender, signed, receipt)

	// A mined execution failure still consumes the sender's transaction nonce.
	for _, node := range f.nodes {
		if node.process.stopped {
			continue
		}
		require.Equal(t, tx.Nonce+1,
			cwNonce(t, node, sender, receipt.BlockNumber),
			"incorrect committed nonce: node=%s sender=%s block=%s status=%s",
			node.rpc, sender.String(), receipt.BlockNumber, receipt.Status)
		require.Equal(t, tx.Nonce+1,
			cwNonce(t, node, sender, "latest"),
			"incorrect latest nonce after sequential transaction: node=%s sender=%s",
			node.rpc, sender.String())
	}

	return receipt
	/* FINISH ALPHA CHANGE 3252 -- expose nonce disagreements and verify consumption after every sequential transaction -- */
}

func (f *cwFixture) checkFees(
	t *testing.T,
	sender types.Address,
	tx *types.Transaction,
	receipt cwReceipt,
) {
	t.Helper()

	node := f.nodes[0]
	number := cwUint64(t, receipt.BlockNumber)
	require.Greater(t, number, uint64(0))
	parentTag := fmt.Sprintf("0x%x", number-1)
	block := cwGetBlock(t, node, receipt.BlockNumber)

	require.Equal(t, receipt.BlockHash, block.Hash)
	require.Len(t, block.Transactions, 1,
		"fee assertions require one user transaction in this fixture block")
	require.NotEqual(t, strings.ToLower(sender.String()), strings.ToLower(block.Miner))

	baseFee := cwQuantity(t, block.BaseFee)
	price := new(big.Int)
	if tx.Type == types.DynamicFeeTx {
		price.Add(baseFee, tx.GasTipCap)
		if price.Cmp(tx.GasFeeCap) > 0 {
			price.Set(tx.GasFeeCap)
		}
	} else {
		price.Set(tx.GasPrice)
	}
	require.Equal(t, "0x"+price.Text(16), receipt.EffectiveGasPrice)

	gasUsed := cwQuantity(t, receipt.GasUsed)
	require.Positive(t, gasUsed.Sign())
	require.LessOrEqual(t, gasUsed.Uint64(), tx.Gas)

	totalFee := new(big.Int).Mul(gasUsed, price)
	baseAmount := new(big.Int).Mul(gasUsed, baseFee)

	readRate := func(address types.Address, method string) *big.Int {
		input := crypto.Keccak256([]byte(method))[:4]
		return cwQuantity(t, cwCall(t, node, address, input, parentTag))
	}

	burnRate := readRate(
		contracts.INNBCBlockRewardGovernanceContract, "feeBurnRateBps()",
	)
	communityRate := readRate(
		contracts.INNBCCommunityRewardPoolContract, "communityRewardRateBps()",
	)

	share := func(rate *big.Int) *big.Int {
		value := new(big.Int).Mul(baseAmount, rate)
		return value.Div(value, big.NewInt(10_000))
	}
	burn := share(burnRate)
	community := share(communityRate)
	validator := new(big.Int).Sub(new(big.Int).Set(totalFee), burn)
	validator.Sub(validator, community)

	for _, recipient := range []struct {
		address string
		amount  *big.Int
	}{
		{contracts.INNBCFeeBurnVaultContract.String(), burn},
		{contracts.INNBCCommunityRewardPoolContract.String(), community},
		{block.Miner, validator},
	} {
		before := cwBalance(t, node, recipient.address, parentTag)
		after := cwBalance(t, node, recipient.address, receipt.BlockNumber)
		require.Zero(t, new(big.Int).Sub(after, before).Cmp(recipient.amount),
			"incorrect fee credit for %s", recipient.address)
	}

	expectedDebit := new(big.Int).Set(totalFee)
	if receipt.Status == "0x1" {
		expectedDebit.Add(expectedDebit, tx.Value)
	}
	before := cwBalance(t, node, sender.String(), parentTag)
	after := cwBalance(t, node, sender.String(), receipt.BlockNumber)
	require.Zero(t, new(big.Int).Sub(before, after).Cmp(expectedDebit),
		"incorrect sender debit")
}

func TestCurrentWorkflow_NetworkExecutionAndRestart(t *testing.T) {
	if os.Getenv("INNBC_WORKFLOW_E2E") != "1" {
		t.Skip("set INNBC_WORKFLOW_E2E=1 to run the isolated four-node integration test")
	}

	f := cwProvision(t, 4, "private-local")

	adminKey, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)
	actorKey, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)
	admin := crypto.PubKeyToAddress(&adminKey.PublicKey)
	actor := crypto.PubKeyToAddress(&actorKey.PublicKey)
	recipient := types.BytesToAddress([]byte{0x81, 0x23})

	premine := new(big.Int).Exp(big.NewInt(10), big.NewInt(27), nil)
	f.startCluster(t, []string{
		"genesis",
		"--dir", f.chain,
		"--chain-id", "1982",
		"--consensus", "innbcbft",
		"--block-time", "1s",
		// Keep this test within the initial committee's bootstrap epoch.
		"--epoch-size", "1800",
		"--sprint-size", "5",
		"--base-fee-config", "1000000000:2:8",
		"--premine", admin.String() + ":" + premine.String(),
		"--premine", actor.String() + ":" + premine.String(),
		"--contract-deployer-allow-list-admin", admin.String(),
	})

	node := f.nodes[0]
	require.Equal(t, "0x7be", cwRead[string](t, node, "eth_chainId"))

	// System contracts are allocated and initialized by the current genesis path.
	for proxy, implementation := range contracts.GetProxyImplementationMapping() {
		require.NotEqual(t, "0x",
			cwRead[string](t, node, "eth_getCode", proxy.String(), "0x0"))
		require.NotEqual(t, "0x",
			cwRead[string](t, node, "eth_getCode", implementation.String(), "0x0"))
	}

	/* START ALPHA CHANGE 3242 -- include the configured allowlist account in expected genesis supply -- */
	for _, funded := range []types.Address{admin, actor} {
		require.Equal(t, premine.String(),
			cwBalance(t, node, funded.String(), "0x0").String(),
			"incorrect explicit genesis premine")
	}

	require.Equal(t, "1",
		cwBalance(t, node, contracts.AllowListContractsAddr.String(), "0x0").String(),
		"the configured deployment allowlist requires its one-wei initialization balance")

	totalSupplyInput := crypto.Keccak256([]byte("totalSupply()"))[:4]
	expectedSupply := new(big.Int).Mul(new(big.Int).Set(premine), big.NewInt(2))
	expectedSupply.Add(expectedSupply, big.NewInt(1))

	actualSupply := cwQuantity(t, cwCall(
		t, node, contracts.NativeERC20TokenContract, totalSupplyInput, "0x0",
	))
	require.Equal(t, expectedSupply.String(), actualSupply.String(),
		"native-token supply must include explicit premines and the allowlist initialization balance")
	/* FINISH ALPHA CHANGE 3242 -- include the configured allowlist account in expected genesis supply -- */

	issuerInput := crypto.Keccak256([]byte("blockRewardIssuer()"))[:4]
	expectedIssuer := new(big.Int).SetBytes(contracts.INNBCBlockRewardGovernanceContract.Bytes())
	require.Equal(t, cwWord(expectedIssuer), cwCall(
		t, node, contracts.NativeERC20TokenContract, issuerInput, "0x0",
	))

	for _, validator := range f.nodes {
		require.Zero(t, cwBalance(t, node, validator.info[cwOperatorField], "0x0").Sign(),
			"validators must not receive an implicit premine")
	}

	// Exercise legacy full-width prices and both dynamic-price branches.
	for _, tx := range []*types.Transaction{
		{
			To: recipient.Ptr(), Gas: 21_000, Value: big.NewInt(1),
			GasPrice: new(big.Int).Add(
				new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(7),
			),
		},
		{
			Type: types.DynamicFeeTx,
			To:   recipient.Ptr(), Gas: 21_000, Value: big.NewInt(1),
		},
		{
			Type: types.DynamicFeeTx,
			To:   recipient.Ptr(), Gas: 21_000, Value: big.NewInt(1),
			GasFeeCap: big.NewInt(2_000_000_000),
			GasTipCap: big.NewInt(2_000_000_000),
		},
	} {
		receipt := f.send(t, adminKey, tx)
		require.Equal(t, "0x1", receipt.Status)
		require.Equal(t, uint64(21_000), cwUint64(t, receipt.GasUsed))
	}

	for _, running := range f.nodes {
		require.Equal(t, "3",
			cwBalance(t, running, recipient.String(), "latest").String())
	}

	// A distinct, already-consumed nonce must be rejected.
	stale := &types.Transaction{
		Nonce: 0, To: recipient.Ptr(), Gas: 21_000,
		GasPrice: big.NewInt(2_000_000_000), Value: big.NewInt(99),
	}
	signed, err := crypto.NewSigner(chain.AllForksEnabled.At(1), 1982).SignTx(stale, adminKey)
	require.NoError(t, err)
	var rejectedHash string
	require.Error(t, cwRPC(
		node.rpc, "eth_sendRawTransaction", &rejectedHash, cwHex(signed.MarshalRLP()),
	))
	require.Equal(t, "0x3",
		cwRead[string](t, node, "eth_getTransactionCount", admin.String(), "latest"))

	// Deployment authorization is intentionally enabled for this test genesis.
	denied := f.send(t, actorKey, &types.Transaction{
		Input: append([]byte(nil), contractsapi.TestSimple.Bytecode...),
	})
	require.Equal(t, "0x0", denied.Status)

	grant := f.send(t, adminKey, &types.Transaction{
		To:    contracts.AllowListContractsAddr.Ptr(),
		Input: cwABI(t, "function setEnabled(address)", actor),
	})
	require.Equal(t, "0x1", grant.Status)

	roleInput := cwABI(t, "function readAddressList(address) returns (uint256)", actor)
	require.Equal(t, "1", cwQuantity(t, cwCall(
		t, node, contracts.AllowListContractsAddr, roleInput, "latest",
	)).String())

	deployed := f.send(t, actorKey, &types.Transaction{
		Input: append([]byte(nil), contractsapi.TestSimple.Bytecode...),
	})
	require.Equal(t, "0x1", deployed.Status)
	require.NotEmpty(t, deployed.ContractAddress)

	var target types.Address
	require.NoError(t, target.UnmarshalText([]byte(deployed.ContractAddress)))
	require.NotEqual(t, "0x",
		cwRead[string](t, node, "eth_getCode", target.String(), "latest"))

	setInput := cwABI(t, "function setValue(uint256)", big.NewInt(7))
	estimated := cwRead[string](t, node, "eth_estimateGas", map[string]any{
		"from": admin.String(),
		"to":   target.String(),
		"data": cwHex(setInput),
	})
	written := f.send(t, adminKey, &types.Transaction{
		To: target.Ptr(), Input: setInput, Gas: cwUint64(t, estimated),
	})
	require.Equal(t, "0x1", written.Status)

	getInput := crypto.Keccak256([]byte("getValue()"))[:4]
	require.Equal(t, cwWord(big.NewInt(7)), cwCall(t, node, target, getInput, "latest"))
	require.Equal(t, cwWord(big.NewInt(7)),
		cwRead[string](t, node, "eth_getStorageAt",
			target.String(), cwWord(new(big.Int)), "latest"))

	// A simulation override must not alter committed storage.
	overridden := cwRead[string](t, node, "eth_call",
		map[string]any{
			"from": admin.String(), "to": target.String(), "data": cwHex(getInput),
		},
		"latest",
		map[string]any{
			target.String(): map[string]any{
				"stateDiff": map[string]string{
					cwWord(new(big.Int)): cwWord(big.NewInt(3)),
				},
			},
		},
	)
	require.Equal(t, cwWord(big.NewInt(3)), overridden)
	require.Equal(t, cwWord(big.NewInt(7)), cwCall(t, node, target, getInput, "latest"))

	/* START ALPHA CHANGE 3243 -- verify debug methods remain unavailable on the actual public RPC listener -- */
	traceConfig := map[string]any{
		"disableStructLogs": true,
		"timeout":           "5s",
	}

	for _, request := range []struct {
		method string
		params []any
	}{
		{
			method: "debug_traceTransaction",
			params: []any{written.Hash, traceConfig},
		},
		{
			method: "debug_traceBlockByNumber",
			params: []any{written.BlockNumber, traceConfig},
		},
		{
			method: "debug_traceCall",
			params: []any{
				map[string]any{
					"from": admin.String(),
					"to":   target.String(),
					"data": cwHex(getInput),
				},
				written.BlockNumber,
				traceConfig,
			},
		},
	} {
		var result json.RawMessage
		err := cwRPC(node.rpc, request.method, &result, request.params...)
		require.EqualError(t, err, fmt.Sprintf(
			"RPC error -32601: the method %s does not exist/is not available",
			request.method,
		))
	}

	require.Equal(t, cwWord(big.NewInt(7)),
		cwCall(t, node, target, getInput, "latest"))
	/* FINISH ALPHA CHANGE 3243 -- verify debug methods remain unavailable on the actual public RPC listener -- */

	// Enabled deployment permission does not confer list-administrator permission.
	unauthorizedGrant := f.send(t, actorKey, &types.Transaction{
		To:    contracts.AllowListContractsAddr.Ptr(),
		Input: cwABI(t, "function setEnabled(address)", recipient),
	})
	require.Equal(t, "0x0", unauthorizedGrant.Status)

	// An ordinary funded account cannot directly issue emergency block rewards.
	unauthorizedIssue := f.send(t, adminKey, &types.Transaction{
		To: contracts.NativeERC20TokenContract.Ptr(),
		Input: cwABI(
			t, "function issueEmergencyBlockReward(address,uint256) returns (bool)",
			admin, big.NewInt(1),
		),
	})
	require.Equal(t, "0x0", unauthorizedIssue.Status)
	require.Zero(t, cwQuantity(t, cwCall(
		t, node, contracts.NativeERC20TokenContract, totalSupplyInput, "latest",
	)).Cmp(expectedSupply))

	// Three remaining validators must continue while the fourth is offline.
	stopped := f.nodes[3]
	cwStop(t, stopped.process)
	before := cwUint64(t, cwRead[string](t, node, "eth_blockNumber"))

	offlineReceipt := f.send(t, adminKey, &types.Transaction{
		Type: types.DynamicFeeTx,
		To:   recipient.Ptr(), Gas: 21_000, Value: big.NewInt(1),
	})
	require.Equal(t, "0x1", offlineReceipt.Status)
	f.waitHeight(t, f.nodes[:3], before+3)

	stopped.process = f.start(t, true, stopped.args...)
	f.wait(t, "restarted node RPC readiness", func() (bool, error) {
		var height string
		err := cwRPC(stopped.rpc, "eth_blockNumber", &height)
		return height != "", err
	})
	f.refresh(t)

	targetHeight := cwUint64(t, cwRead[string](t, node, "eth_blockNumber")) + 2
	f.waitHeight(t, f.nodes, targetHeight)
	recovered := f.receipt(t, offlineReceipt.Hash)
	require.Equal(t, offlineReceipt, recovered)

	tag := fmt.Sprintf("0x%x", targetHeight)
	expectedBlock := cwGetBlock(t, node, tag)
	for _, running := range f.nodes {
		require.Equal(t, expectedBlock.Hash, cwGetBlock(t, running, tag).Hash)
		require.Equal(t, "4",
			cwBalance(t, running, recipient.String(), tag).String())
		require.Equal(t, cwWord(big.NewInt(7)),
			cwCall(t, running, target, getInput, tag))
		cwAssertKeysUnchanged(t, running)
	}

	// This test deliberately does not cross registry activation or pruning.
	require.Less(t, targetHeight, uint64(1800))
	actualGenesis, err := os.ReadFile(f.chain)
	require.NoError(t, err)
	require.True(t, bytes.Equal(f.genesis, actualGenesis),
		"runtime execution changed the genesis configuration")
}

/* FINISH ALPHA CHANGE 3239 -- exercise the current isolated INNBC network and execution workflow -- */
