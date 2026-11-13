/* START ALPHA CHANGE 3248 -- verify current operator validation membership quorum and epoch recovery -- */
package root

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	serverproto "github.com/innovativebioresearch/innbc-chain/server/proto"
	txpoolproto "github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func cwOperatorConnection(t *testing.T, node *cwNode) *grpc.ClientConn {
	t.Helper()

	connection, err := grpc.NewClient(
		"passthrough:///"+node.grpc,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = connection.Close()
	})

	return connection
}

func TestCurrentWorkflow_OperatorValidationAndMembership(t *testing.T) {
	f, _, accounts := cwScenario(t, 4, 1800, nil)

	for _, node := range f.nodes {
		connection := cwOperatorConnection(t, node)
		system := serverproto.NewSystemClient(connection)
		pool := txpoolproto.NewTxnPoolOperatorClient(connection)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		info, err := system.GetStatus(ctx, &emptypb.Empty{})
		cancel()
		require.NoError(t, err)
		require.Equal(t, int64(1982), info.Network)
		require.NotNil(t, info.Current)
		require.Greater(t, info.Current.Number, int64(0))

		expected := make([]string, 0, len(f.nodes)-1)
		for _, other := range f.nodes {
			if other != node {
				expected = append(expected, other.info[cwPeerField])
			}
		}

		var actual []string
		f.wait(t, "authenticated peers on the operator service", func() (bool, error) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			response, err := system.PeersList(ctx, &emptypb.Empty{})
			if err != nil {
				return false, err
			}
			actual = actual[:0]
			for _, peer := range response.Peers {
				actual = append(actual, peer.Id)
			}
			return len(actual) == len(expected), nil
		})
		require.ElementsMatch(t, expected, actual)

		ctx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
		known, err := system.PeersStatus(ctx, &serverproto.PeersStatusRequest{
			Id: expected[0],
		})
		cancel()
		require.NoError(t, err)
		require.Equal(t, expected[0], known.Id)

		ctx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
		_, err = system.PeersStatus(ctx, &serverproto.PeersStatusRequest{Id: "-1"})
		cancel()
		require.Equal(t, codes.Unknown, status.Code(err))
		require.ErrorContains(t, err,
			"request validation failed: invalid PeersStatusRequest.Id")

		beforeNonce := cwNonce(t, node, accounts[0], "pending")
		ctx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
		_, err = pool.AddTxn(ctx, &txpoolproto.AddTxnReq{
			Raw: nil, From: "12",
		})
		cancel()
		require.Equal(t, codes.Unknown, status.Code(err))
		require.ErrorContains(t, err, "request validation failed:")
		require.ErrorContains(t, err, "AddTxnReq.Raw")
		require.ErrorContains(t, err, "AddTxnReq.From")
		require.Equal(t, beforeNonce, cwNonce(t, node, accounts[0], "pending"))

		// The listener must remain usable after rejected requests.
		ctx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
		_, err = system.GetStatus(ctx, &emptypb.Empty{})
		cancel()
		require.NoError(t, err)
	}

	cwAssertScenarioPreserved(t, f)
}

func TestCurrentWorkflow_QuorumAndEpochRecovery(t *testing.T) {
	for _, count := range []int{4, 5, 6, 7, 8} {
		t.Run(fmt.Sprintf("%d_validators", count), func(t *testing.T) {
			const epochSize = uint64(12)
			f, keys, accounts := cwScenario(t, count, epochSize, nil)
			node := f.nodes[0]

			config, err := innbcbft.LoadINNBCBFTConfig(f.chain)
			require.NoError(t, err)
			validators, err := innbcbft.BuildGenesisValidatorSet(&config)
			require.NoError(t, err)
			require.Len(t, validators, count)
			for _, validator := range validators {
				require.Equal(t, "1", validator.VotingPower.String())
			}

			registryStatus := crypto.Keccak256([]byte("registryActivated()"))[:4]
			require.Zero(t, cwQuantity(t, cwCall(
				t, node, contracts.INNBCValidatorRegistryContract,
				registryStatus, "latest",
			)).Sign())

			// Losing one equal-power validator must still permit production.
			cwStop(t, f.nodes[count-1].process)
			height := cwUint64(t, cwRead[string](t, node, "eth_blockNumber"))
			f.waitHeight(t, f.nodes[:count-1], height+2)

			// Leave one validator running. Allow already committed work to
			// settle before asserting the absence of further finalization.
			for index := 1; index < count-1; index++ {
				cwStop(t, f.nodes[index].process)
			}

			lastHash := ""
			quietSince := time.Now()
			var frozen cwBlock
			f.wait(t, "the isolated validator to settle", func() (bool, error) {
				var block *cwBlock
				err := cwRPC(node.rpc, "eth_getBlockByNumber", &block, "latest", false)
				if err != nil {
					return false, err
				}
				if block == nil {
					return false, fmt.Errorf("isolated node returned no head")
				}
				if block.Hash != lastHash {
					lastHash = block.Hash
					quietSince = time.Now()
				}
				frozen = *block
				return time.Since(quietSince) >= 3*time.Second, nil
			})

			recipient := types.BytesToAddress([]byte{0xc8, 0x01})
			nonce := cwNonce(t, node, accounts[0], "latest")
			raw := cwSignedRaw(t, keys[0], &types.Transaction{
				Nonce: nonce, To: recipient.Ptr(), Gas: 21_000, Value: big.NewInt(1),
			})
			hash := cwRead[string](t, node, "eth_sendRawTransaction", cwHex(raw))
			require.Len(t, hash, 66)

			ticker := time.NewTicker(250 * time.Millisecond)
			deadline := time.NewTimer(6 * time.Second)
			func() {
				defer ticker.Stop()
				defer deadline.Stop()

				for {
					require.Equal(t, frozen.Hash, cwGetBlock(t, node, "latest").Hash)
					require.Nil(t, cwRead[*cwReceipt](
						t, node, "eth_getTransactionReceipt", hash,
					))
					require.Equal(t, nonce, cwNonce(t, node, accounts[0], "latest"))

					select {
					case <-ticker.C:
					case <-deadline.C:
						return
					case <-t.Context().Done():
						t.Fatal("test cancelled during quorum-loss observation")
					}
				}
			}()

			// Restore the same identities and databases.
			for index := 1; index < count; index++ {
				restarted := f.nodes[index]
				restarted.process = f.start(t, true, restarted.args...)
				f.wait(t, "restarted validator RPC", func() (bool, error) {
					var height string
					err := cwRPC(restarted.rpc, "eth_blockNumber", &height)
					return height != "", err
				})
			}
			f.refresh(t)

			receipt := f.receipt(t, hash)
			require.Equal(t, "0x1", receipt.Status)
			require.Greater(t, cwUint64(t, receipt.BlockNumber),
				cwUint64(t, frozen.Number))

			// Cross the next epoch boundary after restoration.
			current := cwUint64(t, cwRead[string](t, node, "eth_blockNumber"))
			boundary := (current/epochSize + 1) * epochSize
			f.waitHeight(t, f.nodes, boundary+1)

			endTag := fmt.Sprintf("0x%x", boundary)
			nextTag := fmt.Sprintf("0x%x", boundary+1)
			end := cwRead[cwDetailedBlock](
				t, node, "eth_getBlockByNumber", endTag, false,
			)
			next := cwRead[cwDetailedBlock](
				t, node, "eth_getBlockByNumber", nextTag, false,
			)
			require.Equal(t, end.Hash, next.ParentHash)

			decodeExtra := func(encoded string) *innbcbft.Extra {
				t.Helper()

				raw, err := hex.DecodeString(encoded[2:])
				require.NoError(t, err)
				extra := new(innbcbft.Extra)
				require.NoError(t, extra.UnmarshalRLP(raw))
				require.NotNil(t, extra.Checkpoint)

				return extra
			}
			endExtra := decodeExtra(end.ExtraData)
			nextExtra := decodeExtra(next.ExtraData)
			require.Equal(t, endExtra.Checkpoint.EpochNumber+1,
				nextExtra.Checkpoint.EpochNumber)
			require.Equal(t, endExtra.Checkpoint.NextValidatorsHash,
				nextExtra.Checkpoint.CurrentValidatorsHash)

			full := cwRead[struct {
				Transactions []struct {
					Type string `json:"type"`
				} `json:"transactions"`
			}](t, node, "eth_getBlockByNumber", endTag, true)

			stateTransactions := 0
			for _, tx := range full.Transactions {
				if tx.Type == "0x7f" {
					stateTransactions++
				}
			}
			require.Equal(t, 1, stateTransactions,
				"the epoch-ending block must include its state transaction")

			epochCounter := cwCall(t, node, contracts.ValidatorSetContract,
				crypto.Keccak256([]byte("currentEpochId()"))[:4], nextTag)
			require.Equal(t, boundary/epochSize+1, cwUint64(t, epochCounter))
			require.Zero(t, cwQuantity(t, cwCall(
				t, node, contracts.INNBCValidatorRegistryContract,
				registryStatus, nextTag,
			)).Sign())

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

/* FINISH ALPHA CHANGE 3248 -- verify current operator validation membership quorum and epoch recovery -- */
