/* START ALPHA CHANGE 3246 -- verify current RPC state selection metadata and EVM block context -- */
package root

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

type cwDetailedBlock struct {
	cwBlock
	GasLimit   string `json:"gasLimit"`
	Difficulty string `json:"difficulty"`
	ExtraData  string `json:"extraData"`
}

type cwRPCTransaction struct {
	Hash        string `json:"hash"`
	From        string `json:"from"`
	To          string `json:"to"`
	Nonce       string `json:"nonce"`
	Value       string `json:"value"`
	Type        string `json:"type"`
	BlockHash   string `json:"blockHash"`
	BlockNumber string `json:"blockNumber"`
}

func cwABIString(t *testing.T, value string) string {
	t.Helper()

	raw, err := hex.DecodeString(value[2:])
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, abi.MustNewType("tuple(string)").DecodeStruct(raw, &decoded))
	result, ok := decoded["0"].(string)
	require.True(t, ok, "ABI result is not a string")

	return result
}

func TestCurrentWorkflow_RPCStateAndBlockContext(t *testing.T) {
	f, keys, accounts := cwScenario(t, 4, 1800, nil)
	node := f.nodes[0]

	// Verify metadata through the initialized proxy, using the three-field
	// native-token configuration accepted by the current genesis command.
	for signature, expected := range map[string]string{
		"name()":   "Workflow INNBC",
		"symbol()": "WINNBC",
	} {
		value := cwCall(t, node, contracts.NativeERC20TokenContract,
			crypto.Keccak256([]byte(signature))[:4], "0x0")
		require.Equal(t, expected, cwABIString(t, value))
	}
	decimals := cwCall(t, node, contracts.NativeERC20TokenContract,
		crypto.Keccak256([]byte("decimals()"))[:4], "0x0")
	require.Equal(t, "9", cwQuantity(t, decimals).String())

	expectedSupply := new(big.Int).Exp(big.NewInt(10), big.NewInt(27), nil)
	expectedSupply.Mul(expectedSupply, big.NewInt(3))
	supply := cwCall(t, node, contracts.NativeERC20TokenContract,
		crypto.Keccak256([]byte("totalSupply()"))[:4], "0x0")
	require.Equal(t, expectedSupply.String(), cwQuantity(t, supply).String())

	zeroHash := types.ZeroHash.String()
	for _, method := range []string{
		"eth_getTransactionReceipt", "eth_getTransactionByHash",
	} {
		require.Nil(t, cwRead[*cwRPCTransaction](t, node, method, zeroHash))
	}
	require.Nil(t, cwRead[*cwBlock](
		t, node, "eth_getBlockByHash", zeroHash, false,
	))
	require.Nil(t, cwRead[*cwBlock](
		t, node, "eth_getBlockByNumber", "0x7fffffffffffffff", false,
	))

	nonceBefore := cwNonce(t, node, accounts[0], "latest")
	deployment := f.send(t, keys[0], &types.Transaction{
		Input: append([]byte(nil), contractsapi.TestSimple.Bytecode...),
	})
	require.Equal(t, "0x1", deployment.Status)

	var target types.Address
	require.NoError(t, target.UnmarshalText([]byte(deployment.ContractAddress)))
	require.Equal(t, crypto.CreateAddress(accounts[0], nonceBefore), target)

	deployedAt := cwUint64(t, deployment.BlockNumber)
	beforeDeployment := fmt.Sprintf("0x%x", deployedAt-1)
	require.Equal(t, "0x",
		cwRead[string](t, node, "eth_getCode", target.String(), beforeDeployment))

	for _, selector := range []any{
		deployment.BlockNumber,
		map[string]any{"blockHash": deployment.BlockHash},
		"pending",
	} {
		require.Equal(t, cwHex(contractsapi.TestSimple.DeployedBytecode),
			cwRead[string](t, node, "eth_getCode", target.String(), selector))
	}

	require.Equal(t, nonceBefore,
		cwNonce(t, node, accounts[0], beforeDeployment))
	require.Equal(t, nonceBefore+1,
		cwNonce(t, node, accounts[0], map[string]any{
			"blockHash": deployment.BlockHash,
		}))

	blockByHash := cwRead[cwBlock](
		t, node, "eth_getBlockByHash", deployment.BlockHash, false,
	)
	require.Equal(t, deployment.BlockNumber, blockByHash.Number)
	require.Equal(t, deployment.BlockHash, blockByHash.Hash)

	getValue := crypto.Keccak256([]byte("getValue()"))[:4]
	require.Equal(t, cwWord(new(big.Int)),
		cwCall(t, node, target, getValue, deployment.BlockNumber))

	written := f.send(t, keys[0], &types.Transaction{
		To:    target.Ptr(),
		Input: cwABI(t, "function setValue(uint256)", big.NewInt(7)),
	})
	require.Equal(t, "0x1", written.Status)

	slot := cwWord(new(big.Int))
	require.Equal(t, cwWord(new(big.Int)),
		cwRead[string](t, node, "eth_getStorageAt",
			target.String(), slot, deployment.BlockNumber))
	require.Equal(t, cwWord(big.NewInt(7)),
		cwRead[string](t, node, "eth_getStorageAt",
			target.String(), slot, map[string]any{"blockHash": written.BlockHash}))

	// This caller has never been funded or used for a signed transaction.
	emptyKey, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)
	empty := crypto.PubKeyToAddress(&emptyKey.PublicKey)
	require.Zero(t, cwBalance(t, node, empty.String(), "latest").Sign())
	require.Equal(t, uint64(0), cwNonce(t, node, empty, "latest"))
	require.Equal(t, "0x",
		cwRead[string](t, node, "eth_getCode", empty.String(), "latest"))

	call := map[string]any{
		"from": empty.String(), "to": target.String(), "data": cwHex(getValue),
	}
	require.Equal(t, cwWord(big.NewInt(7)),
		cwRead[string](t, node, "eth_call", call, "latest"))

	// Positive-fee estimation cannot purchase intrinsic gas from zero funds.
	var estimate string
	err = cwRPC(node.rpc, "eth_estimateGas", &estimate, map[string]any{
		"from": empty.String(), "to": target.String(),
		"data": cwHex(getValue), "gasPrice": "0x77359400",
	})
	require.ErrorContains(t, err, "below intrinsic gas")

	// An explicit zero fee is preserved for this nonpayable simulation.
	estimate = cwRead[string](t, node, "eth_estimateGas", map[string]any{
		"from": empty.String(), "to": target.String(),
		"data": cwHex(getValue), "gasPrice": "0x0",
	})
	require.GreaterOrEqual(t, cwUint64(t, estimate), uint64(21_000))

	err = cwRPC(node.rpc, "eth_estimateGas", &estimate, map[string]any{
		"from": empty.String(), "to": accounts[1].String(),
		"value": "0x1", "gasPrice": "0x0",
	})
	require.ErrorContains(t, err, "below transfer value")

	require.Zero(t, cwBalance(t, node, empty.String(), "latest").Sign())
	require.Equal(t, uint64(0), cwNonce(t, node, empty, "latest"))
	require.Equal(t, cwWord(big.NewInt(7)),
		cwCall(t, node, target, getValue, "latest"))

	funding := big.NewInt(10_000_000_000_000_000)
	funded := f.send(t, keys[0], &types.Transaction{
		To: empty.Ptr(), Gas: 21_000, Value: new(big.Int).Set(funding),
	})
	require.Equal(t, "0x1", funded.Status)

	estimate = cwRead[string](t, node, "eth_estimateGas", map[string]any{
		"from": empty.String(), "to": accounts[1].String(),
		"value": "0x1", "gasPrice": "0x77359400",
	})
	require.Equal(t, uint64(21_000), cwUint64(t, estimate))

	// A fixed-price ordinary transfer can leave an existing account at zero.
	fee := new(big.Int).Mul(big.NewInt(21_000), big.NewInt(2_000_000_000))
	drainValue := new(big.Int).Sub(new(big.Int).Set(funding), fee)
	drained := f.send(t, emptyKey, &types.Transaction{
		To: accounts[1].Ptr(), Gas: 21_000,
		Value: drainValue, GasPrice: big.NewInt(2_000_000_000),
	})
	require.Equal(t, "0x1", drained.Status)
	require.Zero(t, cwBalance(t, node, empty.String(), "latest").Sign())
	require.Equal(t, uint64(1), cwNonce(t, node, empty, "latest"))
	require.Equal(t, uint64(0), cwNonce(t, node, empty, funded.BlockNumber))

	/* START ALPHA CHANGE 3249 -- record block context independently of the stale Solidity artifact -- */
	// Each instruction group records one header value in a distinct slot.
	// Deploy through an ordinary transaction; no compiler or predeploy is needed.
	contextCode := []byte{
		0x44, 0x60, 0x00, 0x55, // DIFFICULTY -> slot 0
		0x43, 0x60, 0x01, 0x55, // NUMBER -> slot 1
		0x42, 0x60, 0x02, 0x55, // TIMESTAMP -> slot 2
		0x45, 0x60, 0x03, 0x55, // GASLIMIT -> slot 3
		0x46, 0x60, 0x04, 0x55, // CHAINID -> slot 4
		0x48, 0x60, 0x05, 0x55, // BASEFEE -> slot 5
		0x41, 0x60, 0x06, 0x55, // COINBASE -> slot 6
		0x00, // STOP
	}
	codeLength := byte(len(contextCode))
	contextInit := []byte{
		0x60, codeLength, 0x60, 0x0c, 0x60, 0x00, 0x39,
		0x60, codeLength, 0x60, 0x00, 0xf3,
	}

	metadata := f.send(t, keys[0], &types.Transaction{
		Input: append(contextInit, contextCode...),
	})
	require.Equal(t, "0x1", metadata.Status)

	var metadataAddress types.Address
	require.NoError(t, metadataAddress.UnmarshalText([]byte(metadata.ContractAddress)))
	require.Equal(t, cwHex(contextCode),
		cwRead[string](t, node, "eth_getCode",
			metadataAddress.String(), metadata.BlockNumber))

	initialized := f.send(t, keys[0], &types.Transaction{
		To: metadataAddress.Ptr(),
	})
	require.Equal(t, "0x1", initialized.Status)

	header := cwRead[cwDetailedBlock](
		t, node, "eth_getBlockByNumber", initialized.BlockNumber, false,
	)
	require.Equal(t, initialized.BlockHash, header.Hash)

	for index, expected := range []string{
		header.Difficulty,
		header.Number,
		header.Timestamp,
		header.GasLimit,
		"0x7be",
		header.BaseFee,
		header.Miner,
	} {
		slot := cwWord(new(big.Int).SetUint64(uint64(index)))
		require.Equal(t, cwWord(new(big.Int)),
			cwRead[string](t, node, "eth_getStorageAt",
				metadataAddress.String(), slot, metadata.BlockNumber),
			"context slot %d was populated before execution", index)

		actual := cwRead[string](t, node, "eth_getStorageAt",
			metadataAddress.String(), slot, initialized.BlockNumber)
		require.Equal(t, cwWord(cwQuantity(t, expected)), actual,
			"incorrect block-context field %d", index)
	}
	/* FINISH ALPHA CHANGE 3249 -- record block context independently of the stale Solidity artifact -- */

	cwAssertScenarioPreserved(t, f)
}

/* FINISH ALPHA CHANGE 3246 -- verify current RPC state selection metadata and EVM block context -- */
