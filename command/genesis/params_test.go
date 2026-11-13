package genesis

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/types"
)

func Test_extractNativeTokenMetadata(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		rawConfig   string
		expectedCfg *innbcbft.TokenConfig
		expectErr   bool
	}{
		{
			name:        "default token config",
			rawConfig:   "",
			expectedCfg: innbcbft.DefaultTokenConfig,
			expectErr:   false,
		},
		{
			name:      "not enough params provided",
			rawConfig: "Test:TST",
			expectErr: true,
		},
		{
			name:      "empty name provided",
			rawConfig: ":TST:18",
			expectErr: true,
		},
		{
			name:      "empty symbol provided",
			rawConfig: "Test::18",
			expectErr: true,
		},
		{
			name:      "invalid decimals number provided",
			rawConfig: "Test:TST:9999999999999999999999999999999999999999999999999999999999",
			expectErr: true,
		},
		/* START ALPHA CHANGE 785 -- test simplified mandatory INNBC native token config -- */
		{
			name:      "native token valid config",
			rawConfig: "MyINNBCToken:INNBC:9",
			expectedCfg: &innbcbft.TokenConfig{
				Name:     "MyINNBCToken",
				Symbol:   "INNBC",
				Decimals: 9,
			},
			expectErr: false,
		},
		{
			name:      "native token config rejects obsolete mintable flag parameter",
			rawConfig: "MyToken:MTK:9:false",
			expectErr: true,
		},
		{
			name:      "native token config rejects obsolete owner parameter",
			rawConfig: "MyToken:MTK:9:false:0x123456789",
			expectErr: true,
		},
		/* FINISH ALPHA CHANGE 785 -- test simplified mandatory INNBC native token config -- */
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			p := &genesisParams{nativeTokenConfigRaw: c.rawConfig}
			err := p.extractNativeTokenMetadata()

			if c.expectErr {
				require.Error(t, err)
				require.Nil(t, p.nativeTokenConfig)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expectedCfg, p.nativeTokenConfig)
			}
		})
	}
}

/* START ALPHA CHANGE 958 -- test premine parsing without zero-address reserve requirement -- */
func Test_parsePremineInfo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		premineRaw          []string
		expectedPremines    []*helper.PremineInfo
		expectedParseErrMsg string
	}{
		/* START ALPHA CHANGE 3132 -- keep the balance-error fixture focused on a valid full-length address -- */
		{
			name:                "invalid premine balance",
			premineRaw:          []string{"0x0000000000000000000000000000000000000001:loremIpsum"},
			expectedPremines:    []*helper.PremineInfo{},
			expectedParseErrMsg: "invalid premine entry",
		},
		/* FINISH ALPHA CHANGE 3132 -- keep the balance-error fixture focused on a valid full-length address -- */
		{
			name: "valid premine information without zero address reserve",
			premineRaw: []string{
				fmt.Sprintf("%s:%d", types.StringToAddress("1"), ethgo.Ether(10)),
				types.StringToAddress("12").String(),
			},
			expectedPremines: []*helper.PremineInfo{
				{Address: types.StringToAddress("1"), Amount: ethgo.Ether(10)},
				{Address: types.StringToAddress("12"), Amount: command.DefaultPremineBalance},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			p := &genesisParams{premine: c.premineRaw}
			err := p.parsePremineInfo()
			if c.expectedParseErrMsg != "" {
				require.ErrorContains(t, err, c.expectedParseErrMsg)

				return
			}

			require.NoError(t, err)
			require.Equal(t, c.expectedPremines, p.premineInfos)
		})
	}
}

/* FINISH ALPHA CHANGE 958 -- test premine parsing without zero-address reserve requirement -- */

/* START ALPHA CHANGE 1073 -- test base fee gas target validation -- */
func Test_validateGenesisBaseFeeConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		baseFeeConfig string
		blockGasLimit uint64
		expectedErr   error
	}{
		/* START ALPHA CHANGE 2052 -- test mandatory state transaction genesis capacity -- */
		{
			name:          "zero gas limit uses the valid chain fallback",
			baseFeeConfig: "1:2:8",
			blockGasLimit: 0,
		},
		{
			name:          "rejects gas limit one below mandatory state transaction requirement",
			baseFeeConfig: "1:2:8",
			blockGasLimit: types.StateTransactionGasLimit - 1,
			expectedErr:   errBlockGasLimitBelowStateTransactionGasLimit,
		},
		{
			name:          "accepts exact mandatory state transaction gas limit",
			baseFeeConfig: "1:2:8",
			blockGasLimit: types.StateTransactionGasLimit,
		},
		{
			name:          "accepts generated INNBC block gas limit",
			baseFeeConfig: "1:2:8",
			blockGasLimit: 5_242_880,
		},
		/* FINISH ALPHA CHANGE 2052 -- test mandatory state transaction genesis capacity -- */
		{
			name:          "rejects gas limit below elasticity multiplier",
			baseFeeConfig: "1:1000001:8",
			blockGasLimit: types.StateTransactionGasLimit,
			expectedErr:   errBlockGasLimitTooLow,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			p := &genesisParams{
				baseFeeConfig: c.baseFeeConfig,
				blockGasLimit: c.blockGasLimit,
			}

			err := p.validateGenesisBaseFeeConfig()
			if c.expectedErr != nil {
				require.ErrorIs(t, err, c.expectedErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

/* FINISH ALPHA CHANGE 1073 -- test base fee gas target validation -- */

/* START ALPHA CHANGE 557 -- remove obsolete reward-wallet validation tests after subsidy reward prune -- */
/* FINISH ALPHA CHANGE 557 -- remove obsolete reward-wallet validation tests after subsidy reward prune -- */
