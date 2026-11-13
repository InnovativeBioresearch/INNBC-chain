package server

import (
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus"

	/* START ALPHA CHANGE 709 -- make INNBCBFT the only registered consensus engine -- */
	consensusINNBCBFT "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	/* FINISH ALPHA CHANGE 709 -- make INNBCBFT the only registered consensus engine -- */
	"github.com/innovativebioresearch/innbc-chain/forkmanager"
	/* START ALPHA CHANGE 3219 -- remove selectable identity backend imports from server registration -- */
	/* FINISH ALPHA CHANGE 3219 -- remove selectable identity backend imports from server registration -- */
	"github.com/innovativebioresearch/innbc-chain/state"
)

type GenesisFactoryHook func(config *chain.Chain, engineName string) func(*state.Transition) error

type ConsensusType string

type ForkManagerFactory func(forks *chain.Forks) error

type ForkManagerInitialParamsFactory func(config *chain.Chain) (*forkmanager.ForkParams, error)

const (
	/* START ALPHA CHANGE 738 -- remove legacy dev ibft and dummy consensus constants -- */
	INNBCBFTConsensus ConsensusType = consensusINNBCBFT.ConsensusName
	/* FINISH ALPHA CHANGE 738 -- remove legacy dev ibft and dummy consensus constants -- */
)

var consensusBackends = map[ConsensusType]consensus.Factory{
	/* START ALPHA CHANGE 710 -- remove legacy dev dummy and ibft runtime consensus registration -- */
	INNBCBFTConsensus: consensusINNBCBFT.Factory,
	/* FINISH ALPHA CHANGE 710 -- remove legacy dev dummy and ibft runtime consensus registration -- */
}

/* START ALPHA CHANGE 3219 -- server identity storage uses the local factory directly -- */
/* FINISH ALPHA CHANGE 3219 -- server identity storage uses the local factory directly -- */

var genesisCreationFactory = map[ConsensusType]GenesisFactoryHook{
	INNBCBFTConsensus: consensusINNBCBFT.GenesisPostHookFactory,
}

var forkManagerFactory = map[ConsensusType]ForkManagerFactory{
	INNBCBFTConsensus: consensusINNBCBFT.ForkManagerFactory,
}

var forkManagerInitialParamsFactory = map[ConsensusType]ForkManagerInitialParamsFactory{
	INNBCBFTConsensus: consensusINNBCBFT.ForkManagerInitialParamsFactory,
}

func ConsensusSupported(value string) bool {
	_, ok := consensusBackends[ConsensusType(value)]

	return ok
}
