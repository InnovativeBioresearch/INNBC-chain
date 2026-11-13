/* START ALPHA CHANGE 3244 -- support independent current-workflow regression scenarios -- */
package root

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

// Every scenario creates its own identities, genesis, directory and databases.
// Additional flags configure current features without modifying production code.
func cwScenario(
	t *testing.T,
	count int,
	epochSize uint64,
	configure func([3]types.Address) []string,
) (*cwFixture, [3]*ecdsa.PrivateKey, [3]types.Address) {
	t.Helper()

	if os.Getenv("INNBC_WORKFLOW_E2E") != "1" {
		t.Skip("set INNBC_WORKFLOW_E2E=1 to run current-workflow integration tests")
	}

	f := cwProvision(t, count, "private-local")
	var keys [3]*ecdsa.PrivateKey
	var addresses [3]types.Address

	for index := range keys {
		key, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)
		keys[index] = key
		addresses[index] = crypto.PubKeyToAddress(&key.PublicKey)
	}

	args := []string{
		"genesis",
		"--dir", f.chain,
		"--chain-id", "1982",
		"--consensus", "innbcbft",
		"--block-time", "1s",
		"--epoch-size", fmt.Sprint(epochSize),
		"--sprint-size", "1",
		"--base-fee-config", "1000000000:2:8",
		"--native-token-config", "Workflow INNBC:WINNBC:9",
	}

	premine := new(big.Int).Exp(big.NewInt(10), big.NewInt(27), nil)
	for _, address := range addresses {
		args = append(args, "--premine", address.String()+":"+premine.String())
	}
	if configure != nil {
		args = append(args, configure(addresses)...)
	}

	f.startCluster(t, args)

	return f, keys, addresses
}

// Sign before launching concurrent RPC submissions. No test assertions run
// inside the submission goroutines.
func cwSignedRaw(
	t *testing.T,
	key *ecdsa.PrivateKey,
	tx *types.Transaction,
) []byte {
	t.Helper()

	if tx.Value == nil {
		tx.Value = new(big.Int)
	}
	if tx.Gas == 0 {
		tx.Gas = 21_000
	}

	if tx.Type == types.DynamicFeeTx {
		tx.ChainID = big.NewInt(1982)
		tx.GasPrice = nil
		if tx.GasFeeCap == nil {
			tx.GasFeeCap = big.NewInt(2_000_000_000)
		}
		if tx.GasTipCap == nil {
			tx.GasTipCap = big.NewInt(100_000_000)
		}
	} else if tx.GasPrice == nil {
		tx.GasPrice = big.NewInt(2_000_000_000)
	}

	signed, err := crypto.NewSigner(
		chain.AllForksEnabled.At(1), 1982,
	).SignTx(tx, key)
	require.NoError(t, err)

	return signed.MarshalRLP()
}

func cwNonce(t *testing.T, node *cwNode, address types.Address, tag any) uint64 {
	t.Helper()

	return cwUint64(t, cwRead[string](
		t, node, "eth_getTransactionCount", address.String(), tag,
	))
}

func cwAssertScenarioPreserved(t *testing.T, f *cwFixture) {
	t.Helper()

	actual, err := os.ReadFile(f.chain)
	require.NoError(t, err)
	require.True(t, bytes.Equal(f.genesis, actual),
		"the scenario changed its genesis configuration")

	for _, node := range f.nodes {
		cwAssertKeysUnchanged(t, node)
	}
}

/* FINISH ALPHA CHANGE 3244 -- support independent current-workflow regression scenarios -- */
