/* START ALPHA CHANGE 3813 -- reject BLOCKHASH local failures across execution and rollback -- */

package state

import (
	"errors"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestBlockHashFailure_RollbackAndRevert(t *testing.T) {
	for _, revert := range []bool{false, true} {
		name := "stop"
		if revert {
			name = "revert"
		}

		t.Run(name, func(t *testing.T) {
			code := []byte{
				0x60, 0x01, 0x60, 0x00, 0x55, // slot0 = 1
				0x60, 0x2c, 0x40, 0x50, // BLOCKHASH(44); POP
				0x60, 0x2c, 0x40, 0x50, // repeat after failure
			}
			if revert {
				code = append(code, 0x60, 0x00, 0x60, 0x00, 0xfd)
			} else {
				code = append(code, 0x00)
			}

			_, backend, tr, tx := issue7AExecutionFixture(t, code)
			tr.ctx.Number = 300
			tx.Value = big.NewInt(7)

			original := types.StringToHash("0x09")
			tr.SetState(issue7ATarget, types.ZeroHash, original)
			balance := new(big.Int).Set(tr.GetBalance(issue7ASender))
			checkpoint := tr.state.Snapshot()

			cause := errors.New("injected BLOCKHASH header read failure")
			calls := 0
			tr.getHash = func(number uint64) (types.Hash, error) {
				calls++
				require.Equal(t, uint64(44), number)

				// Confirm actual execution changes before injecting the failure.
				require.Equal(t, types.StringToHash("0x01"),
					tr.GetStorage(issue7ATarget, types.ZeroHash))
				require.Equal(t, uint64(1), tr.GetNonce(issue7ASender))
				require.Zero(t,
					tr.GetBalance(issue7ATarget).Cmp(big.NewInt(7)))

				// The error must win even if a hash is also supplied.
				return types.StringToHash("0xfeed"), cause
			}

			err := tr.Write(tx)
			issue7ARequireRejected(t, tr, backend.snapshot, err, cause)
			require.Contains(t, err.Error(), "read BLOCKHASH 44")
			require.Equal(t, 1, calls)

			require.Equal(t, original,
				tr.GetStorage(issue7ATarget, types.ZeroHash))
			require.Equal(t, uint64(0), tr.GetNonce(issue7ASender))
			require.Zero(t, tr.GetBalance(issue7ASender).Cmp(balance))
			require.Zero(t, tr.GetBalance(issue7ATarget).Sign())

			first := tr.Error()
			require.NoError(t, tr.state.RevertToSnapshot(checkpoint))
			require.Same(t, first, tr.Error())

			require.Equal(t, types.ZeroHash, tr.GetBlockHash(298))
			require.ErrorIs(t, tr.Write(tx), cause)
			require.Same(t, first, tr.Error())
			require.Equal(t, 1, calls)
			require.Empty(t, tr.Receipts())
			require.Zero(t, tr.TotalGas())
			require.Zero(t, backend.snapshot.commits)
		})
	}
}

func TestBlockHashFailure_NormalResults(t *testing.T) {
	hash := types.StringToHash("0x123456")

	for _, tc := range []struct {
		name   string
		number uint16
		hash   types.Hash
		calls  int
	}{
		{"parent", 299, hash, 1},
		{"oldest allowed", 44, hash, 1},
		{"legitimate zero", 299, types.ZeroHash, 1},
		{"too old", 43, types.ZeroHash, 0},
		{"current", 300, types.ZeroHash, 0},
		{"future", 301, types.ZeroHash, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := []byte{
				0x60, 0x01, 0x60, 0x00, 0x55, // slot0 = 1
				0x61, byte(tc.number >> 8), byte(tc.number),
				0x40,                   // BLOCKHASH(number)
				0x60, 0x01, 0x55, 0x00, // slot1 = result; STOP
			}

			_, backend, tr, tx := issue7AExecutionFixture(t, code)
			tr.ctx.Number = 300

			calls := 0
			tr.getHash = func(number uint64) (types.Hash, error) {
				calls++
				require.Equal(t, uint64(tc.number), number)

				return tc.hash, nil
			}

			require.NoError(t, tr.Write(tx))
			require.NoError(t, tr.Error())
			require.Equal(t, tc.calls, calls)
			require.Equal(t, types.StringToHash("0x01"),
				tr.GetStorage(issue7ATarget, types.ZeroHash))
			require.Equal(t, tc.hash,
				tr.GetStorage(issue7ATarget, types.StringToHash("0x01")))
			require.Equal(t, uint64(1), tr.GetNonce(issue7ASender))

			require.Len(t, tr.Receipts(), 1)
			receipt := tr.Receipts()[0]
			require.NotNil(t, receipt.Status)
			require.Equal(t, types.ReceiptSuccess, *receipt.Status)
			require.Greater(t, tr.TotalGas(), uint64(0))
			require.Equal(t, receipt.GasUsed, tr.TotalGas())

			_, _, err := tr.Commit()
			require.NoError(t, err)
			require.Equal(t, 1, backend.snapshot.commits)
		})
	}
}

func TestBlockHashFailure_NestedCall(t *testing.T) {
	_, backend, tr, tx := issue7AExecutionFixture(t, nil)
	tr.ctx.Number = 300

	inner := types.StringToAddress("0x7003")
	innerCode := []byte{
		0x60, 0x2c, 0x40, 0x50, // BLOCKHASH(44); POP
		0x60, 0x00, 0x60, 0x00, 0xfd, // REVERT
	}
	tr.state.SetCode(inner, innerCode)

	// Call the inner contract, discard its success flag and STOP.
	outer := []byte{
		0x60, 0x00, 0x60, 0x00, 0x60, 0x00,
		0x60, 0x00, 0x60, 0x00, 0x73,
	}
	outer = append(outer, inner.Bytes()...)
	outer = append(outer, 0x62, 0x01, 0x86, 0xa0, 0xf1, 0x50, 0x00)
	tr.state.SetCode(issue7ATarget, outer)

	cause := errors.New("injected inner BLOCKHASH failure")
	calls := 0
	tr.getHash = func(number uint64) (types.Hash, error) {
		calls++
		require.Equal(t, uint64(44), number)

		return types.ZeroHash, cause
	}

	result, err := tr.Apply(tx)
	require.Nil(t, result)
	issue7ARequireRejected(t, tr, backend.snapshot, err, cause)
	require.Equal(t, 1, calls)
	require.Equal(t, uint64(0), tr.GetNonce(issue7ASender))
}

/* FINISH ALPHA CHANGE 3813 -- reject BLOCKHASH local failures across execution and rollback -- */
