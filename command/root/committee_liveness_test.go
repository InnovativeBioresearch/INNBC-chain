// START ALPHA CHANGE 3795 -- cover healthy singleton and multi-validator CLI workflows --
package root

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestCurrentWorkflow_CommitteeLivenessMatrix(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 6} {
		t.Run(fmt.Sprintf("%d_validators", count), func(t *testing.T) {
			const epochSize = uint64(6)
			f, keys, accounts := cwScenario(t, count, epochSize, nil)
			node := f.nodes[0]

			config, err := innbcbft.LoadINNBCBFTConfig(f.chain)
			require.NoError(t, err)
			committee, err := innbcbft.BuildGenesisValidatorSet(&config)
			require.NoError(t, err)
			require.Len(t, committee, count)
			for _, member := range committee {
				require.Equal(t, "1", member.VotingPower.String())
			}

			recipient := types.BytesToAddress([]byte{0xe1, byte(count)})
			require.Zero(t, cwBalance(t, node, recipient.String(), "latest").Sign())
			nonce := cwNonce(t, node, accounts[0], "latest")
			raw := cwSignedRaw(t, keys[0], &types.Transaction{
				Nonce: nonce,
				To:    recipient.Ptr(),
				Gas:   21_000,
				Value: big.NewInt(1),
			})
			hash := cwRead[string](t, node, "eth_sendRawTransaction", cwHex(raw))
			require.Len(t, hash, 66)
			receipt := f.receipt(t, hash)
			require.Equal(t, "0x1", receipt.Status)
			require.Equal(t, uint64(21_000), cwUint64(t, receipt.GasUsed))

			current := cwUint64(t, cwRead[string](t, node, "eth_blockNumber"))
			boundary := (current/epochSize + 1) * epochSize
			f.waitHeight(t, f.nodes, boundary+1)

			endTag := fmt.Sprintf("0x%x", boundary)
			nextTag := fmt.Sprintf("0x%x", boundary+1)
			end := cwRead[cwDetailedBlock](
				t, node, "eth_getBlockByNumber", endTag, false)
			next := cwRead[cwDetailedBlock](
				t, node, "eth_getBlockByNumber", nextTag, false)
			require.Equal(t, end.Hash, next.ParentHash)
			require.Equal(t,
				cwUint64(t, end.Timestamp)+1,
				cwUint64(t, next.Timestamp))

			decodeExtra := func(encoded string) *innbcbft.Extra {
				t.Helper()
				require.True(t, strings.HasPrefix(encoded, "0x"))
				raw, err := hex.DecodeString(strings.TrimPrefix(encoded, "0x"))
				require.NoError(t, err)
				extra := new(innbcbft.Extra)
				require.NoError(t, extra.UnmarshalRLP(raw))
				require.NotNil(t, extra.Checkpoint)
				return extra
			}
			endExtra := decodeExtra(end.ExtraData)
			nextExtra := decodeExtra(next.ExtraData)
			require.Equal(t,
				endExtra.Checkpoint.EpochNumber+1,
				nextExtra.Checkpoint.EpochNumber)
			require.Equal(t,
				endExtra.Checkpoint.NextValidatorsHash,
				nextExtra.Checkpoint.CurrentValidatorsHash)
			require.NotEqual(t, types.ZeroHash,
				nextExtra.Checkpoint.CurrentValidatorsHash)
			require.NotNil(t, endExtra.Validators)
			require.True(t, endExtra.Validators.IsEmpty())
			require.Nil(t, nextExtra.Validators)

			full := cwRead[struct {
				Transactions []struct {
					Type string `json:"type"`
				} `json:"transactions"`
			}](t, node, "eth_getBlockByNumber", endTag, true)

			stateTransactions := 0
			for _, transaction := range full.Transactions {
				if transaction.Type == "0x7f" {
					stateTransactions++
				}
			}
			require.Equal(t, 1, stateTransactions)

			epoch := cwCall(t, node, contracts.ValidatorSetContract,
				crypto.Keccak256([]byte("currentEpochId()"))[:4], nextTag)
			require.Equal(t, boundary/epochSize+1, cwUint64(t, epoch))

			for _, running := range f.nodes {
				require.Equal(t, next.Hash, cwGetBlock(t, running, nextTag).Hash)
				require.Equal(t, nonce+1,
					cwNonce(t, running, accounts[0], nextTag))
				require.Equal(t, "1",
					cwBalance(t, running, recipient.String(), nextTag).String())
			}
			cwAssertScenarioPreserved(t, f)
		})
	}
}

// FINISH ALPHA CHANGE 3795 -- cover healthy singleton and multi-validator CLI workflows --
