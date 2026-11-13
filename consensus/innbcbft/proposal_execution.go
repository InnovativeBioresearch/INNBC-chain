/* START ALPHA CHANGE 2583 -- own execution results independently of speculative validation -- */
package innbcbft

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	core "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftcore"
	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	chainstate "github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
)

/*
The shared FSM supplies immutable configuration for a height.
Construction and execution run on private copies.

The result mutex protects insertion and all result inspection.
No map retains speculative results after their workers finish.
*/
type proposalExecutionPayload struct {
	mu sync.Mutex

	/* START ALPHA CHANGE 2610 -- bind execution results to their originating runtime -- */
	runtime *consensusRuntime
	/* FINISH ALPHA CHANGE 2610 -- bind execution results to their originating runtime -- */
	owner   *fsm
	machine *fsm
	raw     []byte

	parentNumber uint64
	parentHash   types.Hash
	height       uint64
	blockHash    types.Hash
}

/*
"insertion" means CommitBlock did not return success; the durability of a
failed write is not assumed. "advancement" means CommitBlock returned
success, but advancing the local runtime failed.
*/
type proposalInsertionFailure struct {
	Phase string
	Err   error
}

func (e *proposalInsertionFailure) Error() string {
	return fmt.Sprintf("committed proposal %s failure: %v", e.Phase, e.Err)
}

func (e *proposalInsertionFailure) Unwrap() error {
	return e.Err
}

func (c *consensusRuntime) copyProposalFSM() (*fsm, *fsm) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	owner := c.fsm
	if owner == nil || owner.parent == nil {
		return nil, nil
	}

	working := *owner
	working.parent = owner.parent.Copy()
	working.target = nil

	return owner, &working
}

/* START ALPHA CHANGE 2611 -- capture immutable runtime ownership with each execution result -- */
func (c *consensusRuntime) captureProposalExecution(
	owner *fsm,
	machine *fsm,
	raw []byte,
) *core.ProposalExecution {
	if owner == nil || machine == nil || machine.parent == nil ||
		machine.target == nil || machine.target.Block == nil ||
		machine.target.Block.Header == nil || len(raw) == 0 {
		return nil
	}

	parent := machine.parent
	block := machine.target.Block

	if parent.Number == ^uint64(0) ||
		block.Number() != parent.Number+1 ||
		block.Header.ParentHash != parent.Hash {
		return nil
	}

	return &core.ProposalExecution{
		Value: &proposalExecutionPayload{
			runtime:      c,
			owner:        owner,
			machine:      machine,
			raw:          append([]byte(nil), raw...),
			parentNumber: parent.Number,
			parentHash:   parent.Hash,
			height:       block.Number(),
			blockHash:    block.Hash(),
		},
	}
}

/* FINISH ALPHA CHANGE 2611 -- capture immutable runtime ownership with each execution result -- */

func (c *consensusRuntime) ExecuteProposal(raw []byte) *core.ProposalExecution {
	owner, working := c.copyProposalFSM()
	if working == nil {
		return nil
	}

	if err := working.Validate(raw); err != nil {
		if errors.Is(err, chainstate.ErrLocalStateFailure) {
			recordLocalConsensusStateFailure()
			chainstate.LogLocalStateFailure(
				c.logger,
				chainstate.LocalStateLogConsensus,
				err,
			)

			return nil
		}

		recordRejectedConsensusMessage()

		return nil
	}

	/* START ALPHA CHANGE 2612 -- capture received proposals through their originating runtime -- */
	return c.captureProposalExecution(owner, working, raw)
	/* FINISH ALPHA CHANGE 2612 -- capture received proposals through their originating runtime -- */
}

func (c *consensusRuntime) BuildExecutedProposal(
	view *proto.View,
) ([]byte, *core.ProposalExecution) {
	if view == nil {
		return nil, nil
	}

	sharedData, err := c.getGuardedData()
	if err != nil {
		if !chainstate.LogLocalStateFailure(c.logger, chainstate.LocalStateLogConsensus, err) {
			c.logger.Error("unable to build proposal", "error", err)
		}

		return nil, nil
	}

	if sharedData.lastBuiltBlock == nil ||
		sharedData.lastBuiltBlock.Number == ^uint64(0) ||
		sharedData.lastBuiltBlock.Number+1 != view.Height {
		c.logger.Error("unable to build proposal, due to lack of parent block",
			"current height", view.Height)

		return nil, nil
	}

	owner, working := c.copyProposalFSM()
	if working == nil ||
		!sameBlockHeader(working.parent, sharedData.lastBuiltBlock) {
		return nil, nil
	}

	raw, err := working.BuildProposal(view.Round)
	if err != nil {
		if !chainstate.LogLocalStateFailure(c.logger, chainstate.LocalStateLogConsensus, err) {
			c.logger.Error("unable to build proposal", "blockNumber", view, "error", err)
		}

		return nil, nil
	}

	/* START ALPHA CHANGE 2613 -- capture locally built proposals through their originating runtime -- */
	execution := c.captureProposalExecution(owner, working, raw)
	if execution == nil {
		return nil, nil
	}
	/* FINISH ALPHA CHANGE 2613 -- capture locally built proposals through their originating runtime -- */

	return raw, execution
}

/* The caller holds p.mu. */
func (p *proposalExecutionPayload) matches(raw []byte) bool {
	if p.owner == nil || p.owner.parent == nil ||
		p.machine == nil || p.machine.parent == nil ||
		p.machine.target == nil || p.machine.target.Block == nil ||
		p.machine.target.Block.Header == nil ||
		!bytes.Equal(p.raw, raw) {
		return false
	}

	parent := p.machine.parent
	block := p.machine.target.Block

	return p.parentNumber != ^uint64(0) &&
		p.height == p.parentNumber+1 &&
		p.owner.parent.Number == p.parentNumber &&
		p.owner.parent.Hash == p.parentHash &&
		parent.Number == p.parentNumber &&
		parent.Hash == p.parentHash &&
		block.Number() == p.height &&
		block.Header.ParentHash == p.parentHash &&
		block.Hash() == p.blockHash
}

/* The caller holds p.mu. Runtime metadata is inspected under c.lock. */
func (c *consensusRuntime) executionStatus(
	p *proposalExecutionPayload,
	raw []byte,
) (currentParent, alreadyApplied bool) {
	/* START ALPHA CHANGE 2614 -- reject foreign runtime results before adoption or insertion -- */
	if p.runtime != c || !p.matches(raw) {
		return false, false
	}
	/* FINISH ALPHA CHANGE 2614 -- reject foreign runtime results before adoption or insertion -- */

	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.fsm != p.owner || c.lastBuiltBlock == nil {
		return false, false
	}

	head := c.lastBuiltBlock

	return head.Number == p.parentNumber && head.Hash == p.parentHash,
		head.Number == p.height && head.Hash == p.blockHash
}

func executionPayload(execution *core.ProposalExecution) *proposalExecutionPayload {
	if execution == nil {
		return nil
	}

	payload, _ := execution.Value.(*proposalExecutionPayload)

	return payload
}

func (c *consensusRuntime) IsCurrentExecution(
	raw []byte,
	execution *core.ProposalExecution,
) bool {
	payload := executionPayload(execution)
	if payload == nil {
		return false
	}

	payload.mu.Lock()
	defer payload.mu.Unlock()

	currentParent, _ := c.executionStatus(payload, raw)

	return currentParent
}

/*
The selected source header is never sealed in place. Transactions and
receipts remain exclusively accessed through this result during insertion.
The mutex stays held through runtime advancement.

No automatic retry is performed after either failure phase.
*/
func (c *consensusRuntime) insertExecutedProposal(
	proposal *proto.Proposal,
	committedSeals []*messages.CommittedSeal,
	execution *core.ProposalExecution,
	advance func(*types.FullBlock) error,
) (err error) {
	defer func() {
		if err != nil && c.config != nil && c.config.reportFatalError != nil {
			c.config.reportFatalError(err)
		}
	}()

	failure := func(phase string, cause error) error {
		return &proposalInsertionFailure{Phase: phase, Err: cause}
	}

	payload := executionPayload(execution)
	if proposal == nil || payload == nil || advance == nil {
		return failure("selection", errors.New("missing proposal, execution, or advancement callback"))
	}

	payload.mu.Lock()
	defer payload.mu.Unlock()

	/* START ALPHA CHANGE 2615 -- complete already canonical decisions after runtime advancement -- */
	currentParent, _ := c.executionStatus(payload, proposal.RawProposal)
	if !currentParent {
		if payload.runtime != c || !payload.matches(proposal.RawProposal) {
			return failure("selection", errors.New("stale or mismatched proposal execution"))
		}

		c.lock.RLock()
		head := c.lastBuiltBlock
		runtimeCovered := head != nil &&
			(head.Number > payload.height ||
				(head.Number == payload.height && head.Hash == payload.blockHash))
		c.lock.RUnlock()

		if !runtimeCovered {
			return failure("selection", errors.New("stale or mismatched proposal execution"))
		}

		if c.config == nil || c.config.blockchain == nil {
			return failure("selection", errors.New("missing canonical confirmation backend"))
		}

		/*
			Runtime advancement may replace the current FSM. The result must
			still belong to this runtime and match its original proposal.
			Canonical confirmation permits completion only; IsCurrentExecution
			continues to reject this result for adoption or voting.
		*/
		if confirmErr := c.config.blockchain.ConfirmCanonicalBlock(
			payload.height, payload.blockHash,
		); confirmErr != nil {
			return failure("insertion", confirmErr)
		}

		/* Do not rewrite the block or replay runtime effects already covered. */
		return nil
	}
	/* FINISH ALPHA CHANGE 2615 -- complete already canonical decisions after runtime advancement -- */

	source := payload.machine.target

	working := *payload.machine
	working.target = &types.FullBlock{
		Block: &types.Block{
			Header:       source.Block.Header.Copy(),
			Transactions: source.Block.Transactions,
			Uncles:       source.Block.Uncles,
		},
		Receipts: source.Receipts,
	}

	fullBlock, insertErr := working.Insert(proposal.RawProposal, committedSeals)
	if insertErr != nil {
		if !chainstate.LogLocalStateFailure(c.logger, chainstate.LocalStateLogConsensus, insertErr) {
			c.logger.Error("cannot insert proposal", "error", insertErr)
		}

		return failure("insertion", insertErr)
	}

	if advanceErr := advance(fullBlock); advanceErr != nil {
		return failure("advancement", advanceErr)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2583 -- own execution results independently of speculative validation -- */
