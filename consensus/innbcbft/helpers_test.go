package innbcbft

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path"
	"testing"
	"time"

	/*START ALPHA CHANGE 60 -- remove hclog import after state constructor update --*/
	"github.com/innovativebioresearch/innbc-chain/bls"
	/*FINISH ALPHA CHANGE 60 -- remove hclog import after state constructor update --*/
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/bitmap"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func createTestKey(t *testing.T) *wallet.Key {
	t.Helper()

	return wallet.NewKey(generateTestAccount(t))
}

func createRandomTestKeys(t *testing.T, numberOfKeys int) []*wallet.Key {
	t.Helper()

	/*START ALPHA CHANGE 37 -- simplify test key slice allocation --*/
	result := make([]*wallet.Key, numberOfKeys)
	/*FINISH ALPHA CHANGE 37 -- simplify test key slice allocation --*/

	for i := 0; i < numberOfKeys; i++ {
		result[i] = wallet.NewKey(generateTestAccount(t))
	}

	return result
}

func createSignature(t *testing.T, accounts []*wallet.Account, hash types.Hash, domain []byte) *Signature {
	t.Helper()

	var signatures bls.Signatures

	var bmp bitmap.Bitmap
	for i, x := range accounts {
		bmp.Set(uint64(i))

		src, err := x.Bls.Sign(hash[:], domain)
		require.NoError(t, err)

		signatures = append(signatures, src)
	}

	aggs, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)

	return &Signature{AggregatedSignature: aggs, Bitmap: bmp}
}

func createTestCommitEpochInput(t *testing.T, epochID uint64,
	epochSize uint64) *contractsapi.CommitEpochValidatorSetFn {
	t.Helper()

	var startBlock uint64 = 0
	if epochID > 1 {
		startBlock = (epochID - 1) * epochSize
	}

	commitEpoch := &contractsapi.CommitEpochValidatorSetFn{
		ID: new(big.Int).SetUint64(epochID),
		Epoch: &contractsapi.Epoch{
			StartBlock: new(big.Int).SetUint64(startBlock + 1),
			EndBlock:   new(big.Int).SetUint64(epochSize * epochID),
			EpochRoot:  types.Hash{},
		},
	}

	return commitEpoch
}

/* START ALPHA CHANGE 574 -- remove obsolete reward distribution test helper -- */
/* FINISH ALPHA CHANGE 574 -- remove obsolete reward distribution test helper -- */

/* START ALPHA CHANGE 409 -- remove unused state sync event test helper -- */
/* FINISH ALPHA CHANGE 409 -- remove unused state sync event test helper -- */

// generateRandomBytes generates byte array with random data of 32 bytes length
func generateRandomBytes(t *testing.T) (result []byte) {
	t.Helper()

	result = make([]byte, types.HashLength)
	_, err := rand.Reader.Read(result)
	require.NoError(t, err, "Cannot generate random byte array content.")

	return
}

// getEpochNumber returns epoch number for given blockNumber and epochSize.
// Epoch number is derived as a result of division of block number and epoch size.
// Since epoch number is 1-based (0 block represents special case zero epoch),
// we are incrementing result by one for non epoch-ending blocks.
func getEpochNumber(t *testing.T, blockNumber, epochSize uint64) uint64 {
	t.Helper()

	if isEndOfPeriod(blockNumber, epochSize) {
		return blockNumber / epochSize
	}

	return blockNumber/epochSize + 1
}

// newTestState creates new instance of state used by tests.
func newTestState(tb testing.TB) *State {
	tb.Helper()

	dir := fmt.Sprintf("/tmp/consensus-temp_%v", time.Now().UTC().Format(time.RFC3339Nano))
	err := os.Mkdir(dir, 0775)

	if err != nil {
		tb.Fatal(err)
	}

	/*START ALPHA CHANGE 61 -- update test state constructor call after logger parameter removal --*/
	/* START ALPHA CHANGE 1551 -- use an MDBX directory for test consensus state -- */
	state, err := newState(path.Join(dir, "state"), make(chan struct{}))
	/* FINISH ALPHA CHANGE 1551 -- use an MDBX directory for test consensus state -- */
	/*FINISH ALPHA CHANGE 61 -- update test state constructor call after logger parameter removal --*/
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		/* START ALPHA CHANGE 1438 -- close test consensus state before removing its directory -- */
		if err := state.Close(); err != nil {
			tb.Fatal(err)
		}
		/* FINISH ALPHA CHANGE 1438 -- close test consensus state before removing its directory -- */

		if err := os.RemoveAll(dir); err != nil {
			tb.Fatal(err)
		}
	})

	return state
}

func generateTestAccount(tb testing.TB) *wallet.Account {
	tb.Helper()

	acc, err := wallet.GenerateAccount()
	require.NoError(tb, err)

	return acc
}

/* START ALPHA CHANGE 125 -- remove unused bridge config test helper -- */
/* FINISH ALPHA CHANGE 125 -- remove unused bridge config test helper -- */
