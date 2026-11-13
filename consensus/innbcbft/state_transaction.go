/* START ALPHA CHANGE 386 -- restore INNBC-native state transaction decoder -- */
package innbcbft

import (
	"fmt"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	/* START ALPHA CHANGE 2291 -- share state transaction structure validation -- */
	"github.com/innovativebioresearch/innbc-chain/types"
	/* FINISH ALPHA CHANGE 2291 -- share state transaction structure validation -- */)

func decodeStateTransaction(input []byte) (contractsapi.StateTransactionInput, error) {
	stateTxInputs := []contractsapi.StateTransactionInput{
		&contractsapi.CommitEpochValidatorSetFn{},
		/* START ALPHA CHANGE 573 -- remove obsolete reward distribution state transaction decoder -- */
		/* FINISH ALPHA CHANGE 573 -- remove obsolete reward distribution state transaction decoder -- */
	}

	for _, stateTxInput := range stateTxInputs {
		if err := stateTxInput.DecodeAbi(input); err == nil {
			return stateTxInput, nil
		}
	}

	return nil, fmt.Errorf("unknown INNBC state transaction input")
}

/* START ALPHA CHANGE 2291 -- share state transaction structure validation -- */
// findCommitEpochTransaction rejects unknown state inputs and duplicate epoch
// transactions. It does not determine whether an epoch transaction is required
// at this height or whether its arguments match the expected epoch.
func findCommitEpochTransaction(transactions []*types.Transaction) (*types.Transaction, error) {
	var commitEpochTx *types.Transaction

	for _, tx := range transactions {
		if tx.Type != types.StateTx {
			continue
		}

		decodedStateTx, err := decodeStateTransaction(tx.Input)
		if err != nil {
			return nil, fmt.Errorf("unknown state transaction: tx = %v, err = %w", tx.Hash, err)
		}

		switch stateTxData := decodedStateTx.(type) {
		case *contractsapi.CommitEpochValidatorSetFn:
			if commitEpochTx != nil {
				return nil, errCommitEpochTxSingleExpected
			}

			commitEpochTx = tx
		default:
			return nil, fmt.Errorf("invalid state transaction data type: %v", stateTxData)
		}
	}

	return commitEpochTx, nil
}

/* FINISH ALPHA CHANGE 2291 -- share state transaction structure validation -- */

/* FINISH ALPHA CHANGE 386 -- restore INNBC-native state transaction decoder -- */
