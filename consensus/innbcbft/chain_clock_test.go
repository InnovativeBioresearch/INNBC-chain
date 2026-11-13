/* START ALPHA CHANGE 3322 -- replace wall-clock allowance tests with chain-clock validation -- */
package innbcbft

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/forkmanager"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func chainClockTestHeaders(parentTimestamp, timestamp uint64) (*types.Header, *types.Header) {
	parent := &types.Header{
		Number:    10,
		Timestamp: parentTimestamp,
	}
	parent.ComputeHash()

	header := &types.Header{
		ParentHash: parent.Hash,
		Number:     parent.Number + 1,
		Timestamp:  timestamp,
		ExtraData:  make([]byte, ExtraVanity),
		MixHash:    INNBCBFTMixDigest,
		Difficulty: 1,
		GasLimit:   100,
	}
	header.ComputeHash()

	return parent, header
}

func TestChainClock_HeaderValidation(t *testing.T) {
	for _, parentTimestamp := range []uint64{100, 4_000_000_000} {
		for _, seconds := range []uint64{1, 2, 5} {
			t.Run(fmt.Sprintf("parent=%d/interval=%d", parentTimestamp, seconds), func(t *testing.T) {
				interval := time.Duration(seconds) * time.Second
				parent, header := chainClockTestHeaders(
					parentTimestamp,
					parentTimestamp+seconds,
				)

				require.NoError(t, validateHeaderFields(parent, header, interval))

				for _, invalid := range []uint64{
					parentTimestamp - 1,
					parentTimestamp,
					parentTimestamp + seconds + 1,
				} {
					header.Timestamp = invalid
					header.ComputeHash()
					require.ErrorContains(t,
						validateHeaderFields(parent, header, interval),
						"invalid block timestamp",
					)
				}

				if seconds > 1 {
					header.Timestamp = parentTimestamp + seconds - 1
					header.ComputeHash()
					require.ErrorContains(t,
						validateHeaderFields(parent, header, interval),
						"invalid block timestamp",
					)
				}
			})
		}
	}
}

func TestChainClock_IncomingRejectedBeforeExecution(t *testing.T) {
	for _, timestamp := range []uint64{99, 100, 101, 103} {
		t.Run(fmt.Sprint(timestamp), func(t *testing.T) {
			parent, header := chainClockTestHeaders(100, timestamp)
			backend := new(blockchainMock)
			f := &fsm{
				parent:  parent,
				backend: backend,
				config: &INNBCBFTConfig{
					BlockTime: common.Duration{Duration: 2 * time.Second},
				},
			}

			err := f.Validate((&types.Block{Header: header}).MarshalRLP())
			require.ErrorContains(t, err, "invalid block timestamp")
			backend.AssertNotCalled(t, "ProcessBlock", mock.Anything, mock.Anything)
			require.Nil(t, f.target)
		})
	}
}

func TestChainClock_FinalizedHeaderRejectsIncorrectInterval(t *testing.T) {
	p := &INNBCBFT{
		consensusConfig: &INNBCBFTConfig{
			BlockTime: common.Duration{Duration: 2 * time.Second},
		},
	}

	for _, timestamp := range []uint64{99, 100, 101, 103} {
		parent, header := chainClockTestHeaders(100, timestamp)
		require.ErrorContains(t,
			p.verifyHeaderImpl(parent, header, nil),
			"invalid block timestamp",
		)
	}
}

func TestChainClock_RetiredConfigurationBoundaries(t *testing.T) {
	for _, value := range []any{nil, uint64(0), uint64(10)} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			engine := map[string]any{
				"blockTime":               "2s",
				"blockTimeDrift":          value,
				"validatorRegistryConfig": testValidatorRegistryConfig(types.ZeroAddress),
			}

			_, err := GetINNBCBFTConfig(&chain.Chain{
				Params: &chain.Params{
					Engine: map[string]any{ConsensusName: engine},
				},
			})
			require.ErrorContains(t, err, "retired")

			instance, err := Factory(&consensus.Params{
				Logger: hclog.NewNullLogger(),
				Config: &consensus.Config{Config: engine},
			})
			require.ErrorContains(t, err, "retired")
			require.Nil(t, instance)

			raw, err := json.Marshal(engine)
			require.NoError(t, err)
			var params forkmanager.ForkParams
			require.ErrorContains(t, json.Unmarshal(raw, &params), "retired")
		})
	}
}

/* FINISH ALPHA CHANGE 3322 -- replace wall-clock allowance tests with chain-clock validation -- */
