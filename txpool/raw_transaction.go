/* START ALPHA CHANGE 3009 -- share bounded raw transaction admission decoding -- */
package txpool

import "github.com/innovativebioresearch/innbc-chain/types"

// DecodeRawTransaction applies transaction-pool size policy before decoding.
// Block and historical transaction decoding do not use this admission limit.
func DecodeRawTransaction(raw []byte) (*types.Transaction, error) {
	if len(raw) > MaxTransactionSize {
		return nil, ErrOversizedData
	}

	tx := new(types.Transaction)
	if err := tx.UnmarshalRLP(raw); err != nil {
		return nil, err
	}

	return tx, nil
}

/* FINISH ALPHA CHANGE 3009 -- share bounded raw transaction admission decoding -- */
