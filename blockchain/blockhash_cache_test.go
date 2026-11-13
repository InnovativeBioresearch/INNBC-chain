/* START ALPHA CHANGE 3803 -- verify bounded BLOCKHASH reads and unchanged bytecode results and gas -- */

package blockchain

import (
	"fmt"
	"math"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/state/runtime/evm"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

type blockHashCacheFixture struct {
	blockchain *Blockchain
	headers    map[types.Hash]*types.Header
	reads      int
}

func blockHashCacheHeaders(height uint64, branch byte) []*types.Header {
	headers := make([]*types.Header, height+1)

	for number := range headers {
		header := &types.Header{
			Number:    uint64(number),
			GasLimit:  5_242_880,
			Timestamp: uint64(number) + 1,
		}

		if number > 0 {
			header.ParentHash = headers[number-1].Hash
			header.ExtraData = []byte{branch}
		}

		header.ComputeHash()
		headers[number] = header
	}

	return headers
}

func newBlockHashCacheFixture(
	t *testing.T,
	chains ...[]*types.Header,
) *blockHashCacheFixture {
	t.Helper()

	fixture := &blockHashCacheFixture{
		headers: make(map[types.Hash]*types.Header),
	}

	for _, headers := range chains {
		for _, header := range headers {
			fixture.headers[header.Hash] = header
		}
	}

	db := storage.NewMockStorage()
	db.HookReadHeader(func(hash types.Hash) (*types.Header, error) {
		fixture.reads++

		header, ok := fixture.headers[hash]
		if !ok {
			return nil, fmt.Errorf("missing test header %s", hash.String())
		}

		// readHeader recomputes the hash before caching the returned header.
		return header.Copy(), nil
	})
	db.HookReadCanonicalHash(func(uint64) (types.Hash, bool) {
		t.Fatal("BLOCKHASH ancestry must follow parent hashes")

		return types.ZeroHash, false
	})

	fixture.blockchain = &Blockchain{db: db}
	require.NoError(t, fixture.blockchain.initCaches(defaultCacheSize))

	return fixture
}

/* START ALPHA CHANGE 3811 -- adapt cache regressions to checked ancestor lookups -- */

func blockHashCacheLookup(
	t *testing.T,
	b *Blockchain,
	header *types.Header,
) func(uint64) types.Hash {
	t.Helper()

	lookup := b.GetHashHelper(header)

	return func(number uint64) types.Hash {
		t.Helper()

		hash, err := lookup(number)
		require.NoError(t, err)

		return hash
	}
}

func TestBlockHashCache_RepeatedOldest(t *testing.T) {
	headers := blockHashCacheHeaders(300, 1)
	fixture := newBlockHashCacheFixture(t, headers)
	lookup := blockHashCacheLookup(t, fixture.blockchain, headers[300])

	require.Equal(t, headers[44].Hash, lookup(44))
	require.Equal(t, 255, fixture.reads)

	// Cached execution hashes must survive eviction from the shared header LRU.
	fixture.blockchain.headersCache.Purge()

	for range 1000 {
		require.Equal(t, headers[44].Hash, lookup(44))
		require.Equal(t, 255, fixture.reads,
			"repeated lookups must not reread ancestor headers")
	}
}

func TestBlockHashCache_IncrementalMixedRequests(t *testing.T) {
	headers := blockHashCacheHeaders(300, 1)
	fixture := newBlockHashCacheFixture(t, headers)
	lookup := blockHashCacheLookup(t, fixture.blockchain, headers[300])

	requests := []struct {
		number uint64
		reads  int
	}{
		{299, 0},
		{250, 49},
		{275, 49},
		{44, 255},
		{45, 255},
		{299, 255},
	}

	for _, request := range requests {
		fixture.blockchain.headersCache.Purge()

		require.Equal(t, headers[request.number].Hash, lookup(request.number))
		require.Equal(t, request.reads, fixture.reads,
			"only previously unknown ancestry may require backend reads")
	}
}

func TestBlockHashCache_WindowAndEarlyBlocks(t *testing.T) {
	for _, height := range []uint64{0, 1, 2, 256, 300} {
		t.Run(fmt.Sprintf("height_%d", height), func(t *testing.T) {
			headers := blockHashCacheHeaders(height, 1)
			fixture := newBlockHashCacheFixture(t, headers)
			lookup := blockHashCacheLookup(t, fixture.blockchain, headers[height])

			limit := height
			if limit > 256 {
				limit = 256
			}

			for distance := uint64(1); distance <= limit; distance++ {
				number := height - distance
				require.Equal(t, headers[number].Hash, lookup(number))
			}

			wantReads := 0
			if limit > 0 {
				wantReads = int(limit - 1)
			}

			require.Equal(t, wantReads, fixture.reads)

			invalid := []uint64{height, height + 1, math.MaxUint64}
			if height > 256 {
				invalid = append(invalid, height-257, 0)
			}

			fixture.blockchain.headersCache.Purge()

			for _, number := range invalid {
				require.Equal(t, types.ZeroHash, lookup(number))
			}

			require.Equal(t, wantReads, fixture.reads,
				"out-of-window requests must not read headers")
		})
	}
}

func TestBlockHashCache_IndependentAnchors(t *testing.T) {
	first := blockHashCacheHeaders(300, 1)
	second := blockHashCacheHeaders(300, 2)
	fixture := newBlockHashCacheFixture(t, first, second)

	candidate := first[300].Copy()
	firstLookup := blockHashCacheLookup(t, fixture.blockchain, candidate)
	secondLookup := blockHashCacheLookup(t, fixture.blockchain, second[300])

	// The helper must retain the number and parent captured at construction.
	candidate.Number = 301
	candidate.ParentHash = second[300].ParentHash

	require.Equal(t, first[44].Hash, firstLookup(44))
	require.Equal(t, 255, fixture.reads)

	fixture.blockchain.headersCache.Purge()

	require.Equal(t, second[44].Hash, secondLookup(44))
	require.Equal(t, 510, fixture.reads)

	fixture.blockchain.headersCache.Purge()

	for _, number := range []uint64{44, 45, 200, 299} {
		require.Equal(t, first[number].Hash, firstLookup(number))
		require.Equal(t, second[number].Hash, secondLookup(number))
	}

	require.Equal(t, types.ZeroHash, firstLookup(300))
	require.Equal(t, 510, fixture.reads,
		"independent helpers must retain their own ancestry")
}

func TestBlockHashCache_FailedReadPreservesPrefixAndRetries(t *testing.T) {
	headers := blockHashCacheHeaders(300, 1)
	fixture := newBlockHashCacheFixture(t, headers)
	lookup := fixture.blockchain.GetHashHelper(headers[300])

	missing := headers[270]
	delete(fixture.headers, missing.Hash)

	hash, err := lookup(250)
	require.Equal(t, types.ZeroHash, hash)
	require.ErrorContains(t, err, "missing test header")
	require.Equal(t, 30, fixture.reads)

	fixture.blockchain.headersCache.Purge()

	for _, number := range []uint64{280, 270} {
		hash, err = lookup(number)
		require.NoError(t, err)
		require.Equal(t, headers[number].Hash, hash)
	}
	require.Equal(t, 30, fixture.reads)

	hash, err = lookup(250)
	require.Equal(t, types.ZeroHash, hash)
	require.ErrorContains(t, err, "missing test header")
	require.Equal(t, 31, fixture.reads)

	fixture.headers[missing.Hash] = missing
	fixture.blockchain.headersCache.Purge()

	hash, err = lookup(250)
	require.NoError(t, err)
	require.Equal(t, headers[250].Hash, hash)
	require.Equal(t, 51, fixture.reads,
		"repair must require only the remaining 20 ancestor reads")

	fixture.blockchain.headersCache.Purge()

	hash, err = lookup(250)
	require.NoError(t, err)
	require.Equal(t, headers[250].Hash, hash)
	require.Equal(t, 51, fixture.reads,
		"a failed lookup must not permanently cache a zero result")
}

type blockHashCacheHost struct {
	runtime.Host

	number int64
	lookup func(uint64) types.Hash
}

func (h *blockHashCacheHost) GetTxContext() runtime.TxContext {
	return runtime.TxContext{Number: h.number}
}

func (h *blockHashCacheHost) GetBlockHash(number int64) types.Hash {
	return h.lookup(uint64(number))
}

func (h *blockHashCacheHost) GetTracer() runtime.VMTracer {
	return nil
}

func TestBlockHashCache_BytecodeResultsGasAndReads(t *testing.T) {
	headers := blockHashCacheHeaders(300, 1)
	fixture := newBlockHashCacheFixture(t, headers)

	host := &blockHashCacheHost{
		number: 300,
		lookup: blockHashCacheLookup(t, fixture.blockchain, headers[300]),
	}

	const repetitions = 1000

	code := make([]byte, 0, repetitions*4+11)
	for range repetitions {
		// PUSH1 44; BLOCKHASH; POP.
		code = append(code, 0x60, 44, 0x40, 0x50)
	}

	// Return BLOCKHASH(44) as a 32-byte word:
	// PUSH1 44; BLOCKHASH; PUSH1 0; MSTORE;
	// PUSH1 32; PUSH1 0; RETURN.
	code = append(code,
		0x60, 44, 0x40, 0x60, 0x00, 0x52,
		0x60, 0x20, 0x60, 0x00, 0xf3,
	)

	// Each repetition costs 25 gas. The final return costs 38 gas,
	// including the first word of memory expansion.
	const executionGas = uint64(repetitions*25 + 38)

	result := evm.NewEVM().Run(
		&runtime.Contract{
			Code: code,
			Gas:  executionGas,
		},
		host,
		&chain.ForksInTime{},
	)

	require.NoError(t, result.Err)
	require.Equal(t, headers[44].Hash.Bytes(), result.ReturnValue)
	require.Equal(t, executionGas, result.GasUsed)
	require.Zero(t, result.GasLeft)
	require.Equal(t, 255, fixture.reads,
		"1001 executed BLOCKHASH instructions must share the ancestor cache")
}

/* FINISH ALPHA CHANGE 3811 -- adapt cache regressions to checked ancestor lookups -- */
/* FINISH ALPHA CHANGE 3803 -- verify bounded BLOCKHASH reads and unchanged bytecode results and gas -- */
