package innbcbft

/* START ALPHA CHANGE 2159 -- import local failure classification -- */
import (
	"errors"
	"fmt"
	"time"

	hcf "github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/consensus"

	/* START ALPHA CHANGE 3312 -- use shared chain-clock arithmetic -- */
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	/* FINISH ALPHA CHANGE 3312 -- use shared chain-clock arithmetic -- */
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/txpool"
	"github.com/innovativebioresearch/innbc-chain/types"
)

/* FINISH ALPHA CHANGE 2159 -- import local failure classification -- */

//nolint:godox
// TODO: Add opentracing (to be fixed in EVM-540)

// BlockBuilderParams are fields for the block that cannot be changed
type BlockBuilderParams struct {
	// Parent block
	Parent *types.Header

	// Executor
	Executor *state.Executor

	// Coinbase that is signing the block
	Coinbase types.Address

	// GasLimit is the gas limit for the block
	GasLimit uint64

	// duration for one block
	BlockTime time.Duration

	// Logger
	Logger hcf.Logger

	// txPoolInterface implementation
	TxPool txPoolInterface

	// BaseFee is the base fee
	BaseFee uint64
}

func NewBlockBuilder(params *BlockBuilderParams) *BlockBuilder {
	return &BlockBuilder{
		params: params,
	}
}

var _ blockBuilder = &BlockBuilder{}

type BlockBuilder struct {
	// input params for the block
	params *BlockBuilderParams

	// header is the header for the block being created
	header *types.Header

	// transactions are the data included in the block
	txns []*types.Transaction

	// block is a reference to the already built block
	block *types.Block

	// state is in memory state transition
	state *state.Transition
}

// Init initializes block builder before adding transactions and actual block building
func (b *BlockBuilder) Reset() error {
	/* START ALPHA CHANGE 3312 -- calculate a supported deterministic successor timestamp -- */
	timestamp, err := common.ExpectedBlockTimestamp(
		b.params.Parent.Timestamp,
		b.params.BlockTime,
	)
	if err != nil {
		return fmt.Errorf("cannot construct block timestamp: %w", err)
	}
	/* FINISH ALPHA CHANGE 3312 -- calculate a supported deterministic successor timestamp -- */

	b.header = &types.Header{
		ParentHash:   b.params.Parent.Hash,
		Number:       b.params.Parent.Number + 1,
		Miner:        b.params.Coinbase[:],
		Difficulty:   1,
		StateRoot:    types.EmptyRootHash, // this avoids needing state for now
		TxRoot:       types.EmptyRootHash,
		ReceiptsRoot: types.EmptyRootHash, // this avoids needing state for now
		Sha3Uncles:   types.EmptyUncleHash,
		GasLimit:     b.params.GasLimit,
		BaseFee:      b.params.BaseFee,
		/* START ALPHA CHANGE 3312 -- publish the checked chain timestamp -- */
		Timestamp: timestamp,
		/* FINISH ALPHA CHANGE 3312 -- publish the checked chain timestamp -- */
	}

	transition, err := b.params.Executor.BeginTxn(b.params.Parent.StateRoot, b.header, b.params.Coinbase)
	if err != nil {
		return err
	}

	b.state = transition
	b.block = nil
	b.txns = []*types.Transaction{}

	return nil
}

// Block returns the built block if nil, it is not built yet
func (b *BlockBuilder) Block() *types.Block {
	return b.block
}

// Build creates the state and the final block
func (b *BlockBuilder) Build(handler func(h *types.Header)) (*types.FullBlock, error) {
	/* START ALPHA CHANGE 2159 -- refuse final block construction after a retained failure -- */
	if err := b.state.Error(); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 2159 -- refuse final block construction after a retained failure -- */

	if handler != nil {
		handler(b.header)
	}

	_, stateRoot, err := b.state.Commit()
	if err != nil {
		return nil, fmt.Errorf("failed to commit the state changes: %w", err)
	}

	b.header.StateRoot = stateRoot
	b.header.GasUsed = b.state.TotalGas()
	b.header.LogsBloom = types.CreateBloom(b.Receipts())

	// build the block
	b.block = consensus.BuildBlock(consensus.BuildBlockParams{
		Header:   b.header,
		Txns:     b.txns,
		Receipts: b.state.Receipts(),
	})

	b.block.Header.ComputeHash()

	return &types.FullBlock{
		Block:    b.block,
		Receipts: b.state.Receipts(),
	}, nil
}

// WriteTx applies given transaction to the state. If transaction apply fails, it reverts the saved snapshot.
func (b *BlockBuilder) WriteTx(tx *types.Transaction) error {
	/* START ALPHA CHANGE 2159 -- check failed transition before transaction gas classification -- */
	if err := b.state.Error(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2159 -- check failed transition before transaction gas classification -- */

	if tx.Gas > b.params.GasLimit {
		b.params.Logger.Info("Transaction gas limit exceedes block gas limit", "hash", tx.Hash,
			"tx gas limit", tx.Gas, "block gas limt", b.params.GasLimit)

		return txpool.ErrBlockLimitExceeded
	}

	if err := b.state.Write(tx); err != nil {
		return err
	}

	b.txns = append(b.txns, tx)

	return nil
}

// Fill fills the block with transactions from the txpool
/* START ALPHA CHANGE 2159 -- return fatal fill errors immediately without transaction penalties -- */
/* START ALPHA CHANGE 2202 -- keep reset recovery outside active proposal pool consumption -- */
func (b *BlockBuilder) Fill() error {
	if err := b.state.Error(); err != nil {
		return err
	}

	blockTimer := time.NewTimer(b.params.BlockTime)
	defer blockTimer.Stop()

	timerExpired := false

	err := b.params.TxPool.WithReconciledPool(func() error {
		// Reconciliation succeeded, and resets remain excluded until
		// selection and the corresponding pool actions have finished.
		b.params.TxPool.Prepare()

		for {
			select {
			case <-blockTimer.C:
				timerExpired = true

				return b.state.Error()

			default:
				tx := b.params.TxPool.Peek()
				finished, err := b.writeTxPoolTransaction(tx)
				if errors.Is(err, state.ErrLocalStateFailure) {
					return err
				}
				if err != nil {
					b.params.Logger.Debug(
						"Fill transaction error",
						"hash", tx.Hash,
						"err", err,
					)
				}
				if finished {
					return b.state.Error()
				}
			}
		}
	})
	if err != nil {
		// Includes retained reconciliation failures. No transaction
		// is selected or penalized when the boundary rejects the build.
		return err
	}

	// The pool lock has been released. Preserve ordinary block timing
	// without delaying resets while waiting on an empty/exhausted pool.
	if !timerExpired {
		<-blockTimer.C
	}

	return b.state.Error()
}

/* FINISH ALPHA CHANGE 2202 -- keep reset recovery outside active proposal pool consumption -- */

/* FINISH ALPHA CHANGE 2159 -- return fatal fill errors immediately without transaction penalties -- */

// Receipts returns the collection of transaction receipts for given block
func (b *BlockBuilder) Receipts() []*types.Receipt {
	return b.state.Receipts()
}

/* START ALPHA CHANGE 2159 -- classify local failures before any pool action -- */
func (b *BlockBuilder) writeTxPoolTransaction(tx *types.Transaction) (bool, error) {
	if err := b.state.Error(); err != nil {
		return true, err
	}

	if tx == nil {
		return true, nil
	}

	if err := b.WriteTx(tx); err != nil {
		if errors.Is(err, state.ErrLocalStateFailure) {
			return true, err
		}
		if _, ok := err.(*state.GasLimitReachedTransitionApplicationError); ok { //nolint:errorlint
			// stop processing
			return true, err
		} else if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable { //nolint:errorlint
			b.params.TxPool.Demote(tx)

			return false, err
		} else {
			b.params.TxPool.Drop(tx)

			return false, err
		}
	}

	// START ALPHA CHANGE 2347 -- distinguish proposal progress from finalized pool removal --
	// Advance this proposal's selection. The transaction remains in the
	// shared pool until finalized-state reset or ordinary rejection.
	b.params.TxPool.Pop(tx)
	// FINISH ALPHA CHANGE 2347 -- distinguish proposal progress from finalized pool removal --

	return false, nil
}

/* FINISH ALPHA CHANGE 2159 -- classify local failures before any pool action -- */

// GetState returns Transition reference
func (b *BlockBuilder) GetState() *state.Transition {
	return b.state
}
