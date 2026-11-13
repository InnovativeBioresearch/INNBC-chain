/* START ALPHA CHANGE 3247 -- verify concurrent nonce propagation and spending newly received funds -- */
package root

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestCurrentWorkflow_ConcurrentAndChainedTransfers(t *testing.T) {
	f, keys, accounts := cwScenario(t, 4, 1800, nil)
	node := f.nodes[0]
	recipient := types.BytesToAddress([]byte{0xb9, 0x01})

	const count = 12
	firstNonce := cwNonce(t, node, accounts[0], "latest")
	beforeSender := cwBalance(t, node, accounts[0].String(), "latest")
	beforeRecipient := cwBalance(t, node, recipient.String(), "latest")

	txs := make([]*types.Transaction, count)
	raw := make([][]byte, count)
	for index := range txs {
		txs[index] = &types.Transaction{
			Nonce: firstNonce + uint64(index),
			To:    recipient.Ptr(),
			Gas:   21_000,
			Value: big.NewInt(1),
		}
		if index%2 == 1 {
			txs[index].Type = types.DynamicFeeTx
		}
		raw[index] = cwSignedRaw(t, keys[0], txs[index])
	}

	type submission struct {
		index int
		hash  string
		err   error
	}
	results := make(chan submission, count)

	for index := range txs {
		url := f.nodes[index%len(f.nodes)].rpc
		payload := cwHex(raw[index])
		go func(index int, url, payload string) {
			var hash string
			err := cwRPC(url, "eth_sendRawTransaction", &hash, payload)
			results <- submission{index: index, hash: hash, err: err}
		}(index, url, payload)
	}

	hashes := make([]string, count)
	seen := make(map[string]bool)
	for range count {
		result := <-results
		require.NoError(t, result.err, "submission %d failed", result.index)
		require.Len(t, result.hash, 66)
		require.False(t, seen[result.hash], "duplicate transaction hash")
		seen[result.hash] = true
		hashes[result.index] = result.hash
	}

	expectedDebit := big.NewInt(count)
	for index, hash := range hashes {
		receipt := f.receipt(t, hash)
		require.Equal(t, "0x1", receipt.Status)
		require.Equal(t, uint64(21_000), cwUint64(t, receipt.GasUsed))

		transaction := cwRead[cwRPCTransaction](
			t, node, "eth_getTransactionByHash", hash,
		)
		require.Equal(t, hash, transaction.Hash)
		require.Equal(t, receipt.BlockHash, transaction.BlockHash)
		require.Equal(t, receipt.BlockNumber, transaction.BlockNumber)
		require.Equal(t, firstNonce+uint64(index),
			cwUint64(t, transaction.Nonce))
		require.Equal(t, uint64(txs[index].Type),
			cwUint64(t, transaction.Type))

		var from, to types.Address
		require.NoError(t, from.UnmarshalText([]byte(transaction.From)))
		require.NoError(t, to.UnmarshalText([]byte(transaction.To)))
		require.Equal(t, accounts[0], from)
		require.Equal(t, recipient, to)
		require.Equal(t, "1", cwQuantity(t, transaction.Value).String())

		block := cwGetBlock(t, node, receipt.BlockNumber)
		price := big.NewInt(2_000_000_000)
		if txs[index].Type == types.DynamicFeeTx {
			price = new(big.Int).Add(
				cwQuantity(t, block.BaseFee), big.NewInt(100_000_000),
			)
			if price.Cmp(big.NewInt(2_000_000_000)) > 0 {
				price.SetInt64(2_000_000_000)
			}
		}
		require.Equal(t, "0x"+price.Text(16), receipt.EffectiveGasPrice)
		expectedDebit.Add(expectedDebit,
			new(big.Int).Mul(big.NewInt(21_000), price))
	}

	for _, running := range f.nodes {
		require.Equal(t, firstNonce+count,
			cwNonce(t, running, accounts[0], "latest"))
		require.Equal(t, firstNonce+count,
			cwNonce(t, running, accounts[0], "pending"))
		actualIncrease := new(big.Int).Sub(
			cwBalance(t, running, recipient.String(), "latest"),
			beforeRecipient,
		)
		require.Equal(t, fmt.Sprint(count), actualIncrease.String())
	}

	actualDebit := new(big.Int).Sub(
		beforeSender, cwBalance(t, node, accounts[0].String(), "latest"),
	)
	require.Equal(t, expectedDebit.String(), actualDebit.String())

	// The next two accounts were not premined. Each must be able to spend
	// funds received through the preceding committed transfer.
	firstKey, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)
	secondKey, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)
	first := crypto.PubKeyToAddress(&firstKey.PublicKey)
	second := crypto.PubKeyToAddress(&secondKey.PublicKey)

	require.Zero(t, cwBalance(t, node, first.String(), "latest").Sign())
	require.Zero(t, cwBalance(t, node, second.String(), "latest").Sign())

	funded := f.send(t, keys[0], &types.Transaction{
		To: first.Ptr(), Gas: 21_000, Value: big.NewInt(10_000_000_000_000_000),
	})
	require.Equal(t, "0x1", funded.Status)

	forwarded := f.send(t, firstKey, &types.Transaction{
		Type: types.DynamicFeeTx,
		To:   second.Ptr(), Gas: 21_000, Value: big.NewInt(1_000_000_000_000_000),
	})
	require.Equal(t, "0x1", forwarded.Status)

	spent := f.send(t, secondKey, &types.Transaction{
		To: recipient.Ptr(), Gas: 21_000, Value: big.NewInt(1),
	})
	require.Equal(t, "0x1", spent.Status)

	for _, running := range f.nodes {
		require.Equal(t, uint64(1), cwNonce(t, running, first, "latest"))
		require.Equal(t, uint64(1), cwNonce(t, running, second, "latest"))
		expected := new(big.Int).Add(beforeRecipient, big.NewInt(count+1))
		require.Equal(t, expected.String(),
			cwBalance(t, running, recipient.String(), "latest").String())
	}

	cwAssertScenarioPreserved(t, f)
}

/* FINISH ALPHA CHANGE 3247 -- verify concurrent nonce propagation and spending newly received funds -- */
