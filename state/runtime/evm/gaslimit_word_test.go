/* START ALPHA CHANGE 3590 -- verify unsigned GASLIMIT stack values and unchanged gas charging -- */
package evm

import (
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/stretchr/testify/require"
)

type gasLimitWordHost struct {
	mockHost

	gasLimit int64
}

func (h *gasLimitWordHost) GetTxContext() runtime.TxContext {
	return runtime.TxContext{
		GasLimit: h.gasLimit,
	}
}

func TestGasLimitWord_StackAndGas(t *testing.T) {
	for _, gasLimit := range []uint64{
		0,
		1,
		5_242_880,
		math.MaxInt64,
		uint64(math.MaxInt64) + 1,
		math.MaxUint64,
	} {
		t.Run(fmt.Sprintf("limit_%d", gasLimit), func(t *testing.T) {
			s := &state{
				code: []byte{GASLIMIT, byte(STOP)},
				gas:  2,
				host: &gasLimitWordHost{
					gasLimit: int64(gasLimit),
				},
			}

			s.push(big.NewInt(0xfeed))

			_, err := s.Run()
			require.NoError(t, err)
			require.Zero(t, s.gas,
				"GASLIMIT must retain its two-gas charge")

			require.Equal(t, 2, s.sp)

			got := s.pop()
			require.GreaterOrEqual(t, got.Sign(), 0)
			require.Zero(t, got.Cmp(
				new(big.Int).SetUint64(gasLimit),
			))

			require.Zero(t, s.pop().Cmp(big.NewInt(0xfeed)),
				"GASLIMIT must preserve earlier stack items")
			require.Zero(t, s.sp)
		})
	}
}

/* FINISH ALPHA CHANGE 3590 -- verify unsigned GASLIMIT stack values and unchanged gas charging -- */
