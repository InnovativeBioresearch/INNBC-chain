/* START ALPHA CHANGE 2179 -- test block and transaction tracing against local state failures -- */
package server

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/blockchain"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	memory "github.com/innovativebioresearch/innbc-chain/blockchain/storage/memory"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/tracer/structtracer"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestIssue7A_BlockAndTransactionTraceRejectLocalFailure(t *testing.T) {
	for _, operation := range []string{"block", "target transaction", "preceding transaction"} {
		t.Run(operation, func(t *testing.T) {
			hub, backend, parent, tx := issue7ARPCFixture(t)
			parent.ComputeHash()

			logger := hclog.NewNullLogger()
			db, err := memory.NewMemoryStorage(logger)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			writer := storage.NewBatchWriter(db)
			writer.PutHeader(parent)
			require.NoError(t, writer.WriteBatch())

			chainBackend, err := blockchain.NewBlockchain(
				logger,
				db,
				&chain.Chain{
					Params: &chain.Params{
						ChainID: 100,
						Forks:   chain.AllForksEnabled,
					},
				},
				&issue7ARPCConsensus{},
				hub.Executor,
				nil,
			)
			require.NoError(t, err)
			hub.Blockchain = chainBackend

			tx.Hash = types.StringToHash("0x7901")
			block := &types.Block{
				Header: &types.Header{
					Number:     parent.Number + 1,
					ParentHash: parent.Hash,
					StateRoot:  parent.StateRoot,
					GasLimit:   parent.GasLimit,
					BaseFee:    parent.BaseFee,
					Timestamp:  parent.Timestamp + 1,
				},
				Transactions: []*types.Transaction{tx},
			}

			targetHash := tx.Hash
			if operation == "preceding transaction" {
				target := tx.Copy()
				target.Nonce++
				target.Hash = types.StringToHash("0x7902")
				block.Transactions = append(block.Transactions, target)
				targetHash = target.Hash
			}

			cause := errors.New("injected block trace storage failure")
			backend.storageErr = cause

			spy := &issue7ARPCTracer{
				Tracer: structtracer.NewStructTracer(structtracer.Config{}),
			}

			if operation == "block" {
				results, traceErr := hub.TraceBlock(block, spy)
				require.Nil(t, results)
				require.ErrorIs(t, traceErr, cause)
				require.ErrorIs(t, traceErr, state.ErrLocalStateFailure)
			} else {
				result, traceErr := hub.TraceTxn(block, targetHash, spy)
				require.Nil(t, result)
				require.ErrorIs(t, traceErr, cause)
				require.ErrorIs(t, traceErr, state.ErrLocalStateFailure)
			}

			// Neither a failed target execution nor a failed preceding
			// replay may export a trace result.
			require.Zero(t, spy.results)
		})
	}
}

/* FINISH ALPHA CHANGE 2179 -- test block and transaction tracing against local state failures -- */
