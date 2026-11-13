package command

import (
	"fmt"

	"github.com/umbracle/ethgo"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/server"
)

const (
	DefaultGenesisFileName = "genesis.json"
	/*START ALPHA CHANGE 10 -- set default generated chain name to INNBC Chain --*/
	DefaultChainName = "innbc-chain"
	/*FINISH ALPHA CHANGE 10 -- set default generated chain name to INNBC Chain --*/
	/* START ALPHA CHANGE 3047 -- default new genesis configurations to the INNBC chain ID -- */
	DefaultChainID = 1982
	/* FINISH ALPHA CHANGE 3047 -- default new genesis configurations to the INNBC chain ID -- */
	DefaultConsensus                 = server.INNBCBFTConsensus
	DefaultGenesisGasUsed            = 458752  // 0x70000
	DefaultGenesisGasLimit           = 5242880 // 0x500000
	DefaultGenesisBaseFeeEM          = chain.GenesisBaseFeeEM
	DefaultGenesisBaseFeeChangeDenom = chain.BaseFeeChangeDenom
)

var (
	DefaultStake                = ethgo.Ether(1e6)
	DefaultPremineBalance       = ethgo.Ether(1e6)
	DefaultGenesisBaseFee       = chain.GenesisBaseFee
	DefaultGenesisBaseFeeConfig = fmt.Sprintf(
		"%d:%d:%d",
		DefaultGenesisBaseFee,
		DefaultGenesisBaseFeeEM,
		DefaultGenesisBaseFeeChangeDenom,
	)
)

const (
	JSONOutputFlag  = "json"
	GRPCAddressFlag = "grpc-address"
	JSONRPCFlag     = "jsonrpc"
)

// GRPCAddressFlagLEGACY Legacy flag that needs to be present to preserve backwards
// compatibility with running clients
const (
	GRPCAddressFlagLEGACY = "grpc"
)
