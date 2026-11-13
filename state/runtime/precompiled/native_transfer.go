package precompiled

import (
	"math/big"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
)

type nativeTransfer struct{}

func (c *nativeTransfer) gas(input []byte, _ *chain.ForksInTime) uint64 {
	return 21000
}

func (c *nativeTransfer) run(input []byte, caller types.Address, host runtime.Host) ([]byte, error) {
	if len(input) < 96 {
		return abiBoolFalse, runtime.ErrInvalidInputData
	}

	// check if caller is native token contract
	if caller != contracts.NativeERC20TokenContract {
		return abiBoolFalse, runtime.ErrUnauthorizedCaller
	}

	from := types.BytesToAddress(input[0:32])
	to := types.BytesToAddress(input[32:64])
	amount := new(big.Int).SetBytes(input[64:96])

	/* START ALPHA CHANGE 951 -- treat zero-address native transfer as trusted mint -- */
	if from == types.ZeroAddress {
		host.AddBalance(to, amount)

		return abiBoolTrue, nil
	}
	/* FINISH ALPHA CHANGE 951 -- treat zero-address native transfer as trusted mint -- */

	// state changes
	if err := host.Transfer(from, to, amount); err != nil {
		return abiBoolFalse, err
	}

	return abiBoolTrue, nil
}
