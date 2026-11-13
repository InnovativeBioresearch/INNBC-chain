package innbcbft

import (
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
	/* START ALPHA CHANGE 1524 -- remove bbolt request transaction types -- */ /* FINISH ALPHA CHANGE 1524 -- remove bbolt request transaction types -- */)

type PostBlockRequest struct {
	// FullBlock is a reference of the executed block
	FullBlock *types.FullBlock
	// Epoch is the epoch number of the executed block
	Epoch uint64
	// IsEpochEndingBlock indicates if this was the last block of given epoch
	IsEpochEndingBlock bool
	/* START ALPHA CHANGE 1525 -- carry the caller-owned MDBX consensus transaction -- */
	// DBTx is the opened consensus state transaction used by PostBlock.
	DBTx *stateTx
	/* FINISH ALPHA CHANGE 1525 -- carry the caller-owned MDBX consensus transaction -- */
}

type PostEpochRequest struct {
	// NewEpochID is the id of the new epoch
	NewEpochID uint64

	// FirstBlockOfEpoch is the number of the epoch beginning block
	FirstBlockOfEpoch uint64

	// SystemState is the state of the governance smart contracts
	// after this block
	SystemState SystemState

	// ValidatorSet is the validator set for the new epoch
	ValidatorSet validator.ValidatorSet

	/* START ALPHA CHANGE 1526 -- carry MDBX transactions in retained epoch requests -- */
	// DBTx is the opened consensus state transaction used by PostEpoch.
	DBTx *stateTx
	/* FINISH ALPHA CHANGE 1526 -- carry MDBX transactions in retained epoch requests -- */
}
