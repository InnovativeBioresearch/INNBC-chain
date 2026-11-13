package genesis

import (
	"errors"
	"fmt"

	/* START ALPHA CHANGE 3048 -- validate chain IDs before signed configuration conversion -- */
	"math"
	/* FINISH ALPHA CHANGE 3048 -- validate chain IDs before signed configuration conversion -- */
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"

	/* START ALPHA CHANGE 726 -- remove legacy IBFT genesis builder imports -- */
	/* FINISH ALPHA CHANGE 726 -- remove legacy IBFT genesis builder imports -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	/* START ALPHA CHANGE 832 -- remove unused contracts import after fixed proxy admin migration -- */
	/* FINISH ALPHA CHANGE 832 -- remove unused contracts import after fixed proxy admin migration -- */
	"github.com/innovativebioresearch/innbc-chain/server"
	"github.com/innovativebioresearch/innbc-chain/types"
	/* START ALPHA CHANGE 727 -- remove legacy IBFT validator type imports from genesis params -- */ /* FINISH ALPHA CHANGE 727 -- remove legacy IBFT validator type imports from genesis params -- */)

const (
	dirFlag       = "dir"
	nameFlag      = "name"
	premineFlag   = "premine"
	chainIDFlag   = "chain-id"
	epochSizeFlag = "epoch-size"
	/* START ALPHA CHANGE 540 -- remove pre-funded epoch reward genesis flag name -- */
	/* FINISH ALPHA CHANGE 540 -- remove pre-funded epoch reward genesis flag name -- */
	blockGasLimitFlag = "block-gas-limit"
	/* START ALPHA CHANGE 496 -- remove legacy burn-contract genesis flag name -- */
	/* FINISH ALPHA CHANGE 496 -- remove legacy burn-contract genesis flag name -- */
	genesisBaseFeeConfigFlag = "base-fee-config"
	/* START ALPHA CHANGE 728 -- remove legacy PoS genesis flag constant -- */
	/* FINISH ALPHA CHANGE 728 -- remove legacy PoS genesis flag constant -- */
	nativeTokenConfigFlag = "native-token-config"
	/* START ALPHA CHANGE 541 -- remove custom reward-token and reward-wallet genesis flag names -- */
	/* FINISH ALPHA CHANGE 541 -- remove custom reward-token and reward-wallet genesis flag names -- */
	blockTrackerPollIntervalFlag = "block-tracker-poll-interval"
	/* START ALPHA CHANGE 819 -- remove proxy contracts admin flag constant -- */
	/* FINISH ALPHA CHANGE 819 -- remove proxy contracts admin flag constant -- */
	/* START ALPHA CHANGE 68 -- add INNBC validator registry genesis flag names -- */
	validatorRegistryMinimumStakeFlag           = "validator-registry-minimum-stake"
	validatorRegistryProposalDurationPresetFlag = "validator-registry-proposal-duration-preset"
	/* FINISH ALPHA CHANGE 68 -- add INNBC validator registry genesis flag names -- */
)

// Legacy flags that need to be preserved for running clients
const (
	chainIDFlagLEGACY = "chainid"
)

var (
	params = &genesisParams{}
)

var (
	/* START ALPHA CHANGE 736 -- remove unused legacy validator-not-specified error -- */
	/* FINISH ALPHA CHANGE 736 -- remove unused legacy validator-not-specified error -- */
	errUnsupportedConsensus = errors.New("specified consensusRaw not supported")
	errInvalidEpochSize     = errors.New("epoch size must be greater than 1")
	/* START ALPHA CHANGE 542 -- remove reward-wallet and custom reward-token validation errors -- */
	/* FINISH ALPHA CHANGE 542 -- remove reward-wallet and custom reward-token validation errors -- */
	/* START ALPHA CHANGE 955 -- remove obsolete zero-address native token reserve premine requirement -- */
	/* FINISH ALPHA CHANGE 955 -- remove obsolete zero-address native token reserve premine requirement -- */
	errBlockTrackerPollInterval = errors.New("block tracker poll interval must be greater than 0")
	errBaseFeeChangeDenomZero   = errors.New("base fee change denominator must be greater than 0")
	errBaseFeeEMZero            = errors.New("base fee elasticity multiplier must be greater than 0")
	errBaseFeeZero              = errors.New("base fee  must be greater than 0")
	errBlockGasLimitTooLow      = errors.New("block gas limit must be greater than or equal to base fee elasticity multiplier")
	/* START ALPHA CHANGE 2050 -- require genesis capacity for mandatory state transactions -- */
	errBlockGasLimitBelowStateTransactionGasLimit = errors.New(
		"block gas limit must be greater than or equal to mandatory state transaction gas limit",
	)
	/* FINISH ALPHA CHANGE 2050 -- require genesis capacity for mandatory state transactions -- */
	/* START ALPHA CHANGE 69 -- add INNBC validator registry genesis validation errors -- */
	/* START ALPHA CHANGE 2640 -- share the fixed genesis stake validation error -- */
	errValidatorRegistryMinimumStake = innbcbft.ErrInvalidGenesisMinimumStake
	/* FINISH ALPHA CHANGE 2640 -- share the fixed genesis stake validation error -- */
	errValidatorRegistryProposalDurationPreset = errors.New(
		"validator registry proposal duration preset must be one of: short, standard, long")
	/* FINISH ALPHA CHANGE 69 -- add INNBC validator registry genesis validation errors -- */
)

type genesisParams struct {
	genesisPath  string
	name         string
	consensusRaw string
	premine      []string
	/* START ALPHA CHANGE 635 -- remove legacy bootnode genesis parameter -- */
	/* FINISH ALPHA CHANGE 635 -- remove legacy bootnode genesis parameter -- */

	chainID   uint64
	epochSize uint64

	blockGasLimit uint64

	/* START ALPHA CHANGE 497 -- remove legacy burn-contract genesis parameter -- */
	/* FINISH ALPHA CHANGE 497 -- remove legacy burn-contract genesis parameter -- */
	baseFeeConfig       string
	parsedBaseFeeConfig *baseFeeInfo

	// Validators
	/* START ALPHA CHANGE 729 -- remove legacy PoS and IBFT genesis validator state -- */
	minNumValidators     uint64
	maxNumValidators     uint64
	validatorsPath       string
	validatorsPrefixPath string
	validators           []string
	/* FINISH ALPHA CHANGE 729 -- remove legacy PoS and IBFT genesis validator state -- */

	extraData []byte
	consensus server.ConsensusType

	consensusEngineConfig map[string]interface{}

	genesisConfig *chain.Chain

	// INNBCBFT
	sprintSize uint64
	blockTime  time.Duration
	/* START ALPHA CHANGE 543 -- remove pre-funded epoch reward genesis parameter -- */
	/* FINISH ALPHA CHANGE 543 -- remove pre-funded epoch reward genesis parameter -- */
	/* START ALPHA CHANGE 3319 -- remove the retired drift parameter -- */
	/* FINISH ALPHA CHANGE 3319 -- remove the retired drift parameter -- */

	initialStateRoot string

	// access lists
	contractDeployerAllowListAdmin   []string
	contractDeployerAllowListEnabled []string
	contractDeployerBlockListAdmin   []string
	contractDeployerBlockListEnabled []string
	transactionsAllowListAdmin       []string
	transactionsAllowListEnabled     []string
	transactionsBlockListAdmin       []string
	transactionsBlockListEnabled     []string
	/* START ALPHA CHANGE 119 -- remove bridge allow/block list params from genesis state -- */
	/* FINISH ALPHA CHANGE 119 -- remove bridge allow/block list params from genesis state -- */

	nativeTokenConfigRaw string
	nativeTokenConfig    *innbcbft.TokenConfig

	premineInfos []*helper.PremineInfo

	/* START ALPHA CHANGE 544 -- remove reward wallet and custom reward-token genesis parameters -- */
	/* FINISH ALPHA CHANGE 544 -- remove reward wallet and custom reward-token genesis parameters -- */

	blockTrackerPollInterval time.Duration

	/* START ALPHA CHANGE 820 -- remove user-controlled proxy admin genesis parameter -- */
	/* FINISH ALPHA CHANGE 820 -- remove user-controlled proxy admin genesis parameter -- */

	/* START ALPHA CHANGE 70 -- add INNBC validator registry genesis params -- */
	validatorRegistryMinimumStakeRaw           string
	validatorRegistryMinimumStake              *big.Int
	validatorRegistryProposalDurationPresetRaw string
	validatorRegistryProposalDurationPreset    string
	/* FINISH ALPHA CHANGE 70 -- add INNBC validator registry genesis params -- */
}

/* START ALPHA CHANGE 2515 -- require whole-second genesis block intervals -- */
func (p *genesisParams) validateBlockTime() error {
	if p.blockTime < time.Second {
		return fmt.Errorf("--%s must be at least 1s, got %s", blockTimeFlag, p.blockTime)
	}

	if p.blockTime%time.Second != 0 {
		return fmt.Errorf("--%s must be a whole number of seconds, got %s", blockTimeFlag, p.blockTime)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2515 -- require whole-second genesis block intervals -- */

/* START ALPHA CHANGE 3531 -- require a positive representable genesis chain ID -- */
func (p *genesisParams) validateChainID() error {
	if p.chainID == 0 || p.chainID > math.MaxInt64 {
		return fmt.Errorf(
			"--%s must be between 1 and %d, got %d",
			chainIDFlag, int64(math.MaxInt64), p.chainID,
		)
	}

	return nil
}

/* FINISH ALPHA CHANGE 3531 -- require a positive representable genesis chain ID -- */

func (p *genesisParams) validateFlags() error {
	// Check if the consensusRaw is supported
	if !server.ConsensusSupported(p.consensusRaw) {
		return errUnsupportedConsensus
	}

	/* START ALPHA CHANGE 3048 -- validate chain ID before processing genesis parameters -- */
	if err := p.validateChainID(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3048 -- validate chain ID before processing genesis parameters -- */

	/* START ALPHA CHANGE 3179 -- reject malformed access-list flags during genesis pre-run validation -- */
	if err := p.validateAccessListAddresses(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3179 -- reject malformed access-list flags during genesis pre-run validation -- */

	/* START ALPHA CHANGE 3183 -- reject malformed external trie roots during genesis pre-run validation -- */
	if _, err := p.parseInitialTrieRoot(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3183 -- reject malformed external trie roots during genesis pre-run validation -- */

	/* START ALPHA CHANGE 2505 -- reject unsupported block times before genesis generation -- */
	if err := p.validateBlockTime(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2505 -- reject unsupported block times before genesis generation -- */

	if err := p.validateGenesisBaseFeeConfig(); err != nil {
		return err
	}

	/* START ALPHA CHANGE 730 -- remove legacy IBFT validator requirement from genesis validation -- */
	/* FINISH ALPHA CHANGE 730 -- remove legacy IBFT validator requirement from genesis validation -- */

	if err := p.parsePremineInfo(); err != nil {
		return err
	}

	if p.isINNBCBFTConsensus() {
		if err := p.extractNativeTokenMetadata(); err != nil {
			return err
		}

		/* START ALPHA CHANGE 498 -- remove legacy burn-contract genesis validation -- */
		/* FINISH ALPHA CHANGE 498 -- remove legacy burn-contract genesis validation -- */

		/* START ALPHA CHANGE 545 -- remove reward wallet and custom reward-token validation from genesis -- */
		/* FINISH ALPHA CHANGE 545 -- remove reward wallet and custom reward-token validation from genesis -- */

		/* START ALPHA CHANGE 956 -- stop requiring zero-address reserve premine for native token minting -- */
		/* FINISH ALPHA CHANGE 956 -- stop requiring zero-address reserve premine for native token minting -- */

		/*START ALPHA CHANGE 17 -- validate INNBCBFT block tracker poll interval during genesis flag validation --*/
		if err := p.validateBlockTrackerPollInterval(); err != nil {
			return err
		}
		/*FINISH ALPHA CHANGE 17 -- validate INNBCBFT block tracker poll interval during genesis flag validation --*/

		/* START ALPHA CHANGE 821 -- stop validating removed proxy admin genesis flag -- */
		/* FINISH ALPHA CHANGE 821 -- stop validating removed proxy admin genesis flag -- */

		/* START ALPHA CHANGE 71 -- validate INNBC validator registry genesis params -- */
		if err := p.validateValidatorRegistryParams(); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 71 -- validate INNBC validator registry genesis params -- */
	}

	// Check if the genesis file already exists
	if generateError := verifyGenesisExistence(p.genesisPath); generateError != nil {
		return errors.New(generateError.GetMessage())
	}

	// Check that the epoch size is correct
	if p.epochSize < 2 && p.isINNBCBFTConsensus() {
		// Epoch size must be greater than 1, so new transactions have a chance to be added to a block.
		// Otherwise, every block would be an endblock (meaning it will not have any transactions).
		// Check is placed here to avoid additional parsing if epochSize < 2
		return errInvalidEpochSize
	}

	// Validate validatorsPath only if validators information were not provided via CLI flag
	if len(p.validators) == 0 {
		if _, err := os.Stat(p.validatorsPath); err != nil {
			return fmt.Errorf("invalid validators path ('%s') provided. Error: %w", p.validatorsPath, err)
		}
	}

	// Validate min and max validators number
	return command.ValidateMinMaxValidatorsNumber(p.minNumValidators, p.maxNumValidators)
}

/* START ALPHA CHANGE 72 -- validate INNBC validator registry genesis params -- */
func (p *genesisParams) validateValidatorRegistryParams() error {
	/* START ALPHA CHANGE 2641 -- reject unsupported genesis minimum stake values -- */
	minimumStake, ok := new(big.Int).SetString(strings.TrimSpace(p.validatorRegistryMinimumStakeRaw), 10)
	if !ok {
		return errValidatorRegistryMinimumStake
	}

	if err := innbcbft.ValidateGenesisMinimumStake(minimumStake); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 2641 -- reject unsupported genesis minimum stake values -- */

	/* START ALPHA CHANGE 93 -- validate INNBC validator registry proposal duration through canonical parser -- */
	preset := strings.ToLower(strings.TrimSpace(p.validatorRegistryProposalDurationPresetRaw))
	if _, err := innbcbft.ParseValidatorRegistryProposalDurationPreset(preset); err != nil {
		return errValidatorRegistryProposalDurationPreset
	}
	/* FINISH ALPHA CHANGE 93 -- validate INNBC validator registry proposal duration through canonical parser -- */

	p.validatorRegistryMinimumStake = minimumStake
	p.validatorRegistryProposalDurationPreset = preset

	return nil
}

/* FINISH ALPHA CHANGE 72 -- validate INNBC validator registry genesis params -- */

/* START ALPHA CHANGE 731 -- remove legacy IBFT consensus detector from genesis params -- */
/* FINISH ALPHA CHANGE 731 -- remove legacy IBFT consensus detector from genesis params -- */

func (p *genesisParams) isINNBCBFTConsensus() bool {
	return server.ConsensusType(p.consensusRaw) == server.INNBCBFTConsensus
}

/* START ALPHA CHANGE 737 -- remove unused legacy validator source helpers -- */
/* FINISH ALPHA CHANGE 737 -- remove unused legacy validator source helpers -- */

func (p *genesisParams) getRequiredFlags() []string {
	/* START ALPHA CHANGE 669 -- remove legacy bootnode required genesis flag -- */
	return []string{}
	/* FINISH ALPHA CHANGE 669 -- remove legacy bootnode required genesis flag -- */
}

func (p *genesisParams) initRawParams() error {
	p.consensus = server.ConsensusType(p.consensusRaw)

	/* START ALPHA CHANGE 732 -- only initialize INNBCBFT genesis params -- */
	p.consensusEngineConfig = map[string]interface{}{
		p.consensusRaw: map[string]interface{}{},
	}
	/* FINISH ALPHA CHANGE 732 -- only initialize INNBCBFT genesis params -- */

	return nil
}

/* START ALPHA CHANGE 733 -- remove legacy IBFT genesis validator and engine builders -- */
/* FINISH ALPHA CHANGE 733 -- remove legacy IBFT genesis validator and engine builders -- */

func (p *genesisParams) generateGenesis() error {
	if err := p.initGenesisConfig(); err != nil {
		return err
	}

	if err := helper.WriteGenesisConfigToDisk(
		p.genesisConfig,
		p.genesisPath,
	); err != nil {
		return err
	}

	return nil
}

func (p *genesisParams) initGenesisConfig() error {
	/* START ALPHA CHANGE 3049 -- protect direct generic genesis construction from chain ID overflow -- */
	if err := p.validateChainID(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3049 -- protect direct generic genesis construction from chain ID overflow -- */

	/* START ALPHA CHANGE 3179 -- reject malformed access-list input in direct generic genesis construction -- */
	if err := p.validateAccessListAddresses(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3179 -- reject malformed access-list input in direct generic genesis construction -- */

	/* START ALPHA CHANGE 3183 -- reject malformed external trie roots in direct generic genesis construction -- */
	if _, err := p.parseInitialTrieRoot(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3183 -- reject malformed external trie roots in direct generic genesis construction -- */

	/* START ALPHA CHANGE 499 -- keep London fork independent from legacy burn-contract flag -- */
	enabledForks := chain.AllForksEnabled
	/* FINISH ALPHA CHANGE 499 -- keep London fork independent from legacy burn-contract flag -- */

	chainConfig := &chain.Chain{
		Name: p.name,
		Genesis: &chain.Genesis{
			GasLimit:   p.blockGasLimit,
			Difficulty: 1,
			Alloc:      map[types.Address]*chain.GenesisAccount{},
			ExtraData:  p.extraData,
			GasUsed:    command.DefaultGenesisGasUsed,
		},
		Params: &chain.Params{
			ChainID: int64(p.chainID),
			Forks:   enabledForks,
			Engine:  p.consensusEngineConfig,
		},
		/* START ALPHA CHANGE 636 -- stop writing legacy bootnodes to generic genesis -- */
		/* FINISH ALPHA CHANGE 636 -- stop writing legacy bootnodes to generic genesis -- */
	}

	/* START ALPHA CHANGE 500 -- remove legacy burn-contract genesis config population -- */
	chainConfig.Genesis.BaseFee = p.parsedBaseFeeConfig.baseFee
	chainConfig.Genesis.BaseFeeEM = p.parsedBaseFeeConfig.baseFeeEM
	chainConfig.Genesis.BaseFeeChangeDenom = p.parsedBaseFeeConfig.baseFeeChangeDenom
	/* FINISH ALPHA CHANGE 500 -- remove legacy burn-contract genesis config population -- */

	/* START ALPHA CHANGE 734 -- remove legacy IBFT PoS staking predeploy from genesis -- */
	/* FINISH ALPHA CHANGE 734 -- remove legacy IBFT PoS staking predeploy from genesis -- */

	for _, premineInfo := range p.premineInfos {
		chainConfig.Genesis.Alloc[premineInfo.Address] = &chain.GenesisAccount{
			Balance: premineInfo.Amount,
		}
	}

	p.genesisConfig = chainConfig

	return nil
}

/* START ALPHA CHANGE 735 -- remove legacy IBFT PoS staking predeploy helpers -- */
/* FINISH ALPHA CHANGE 735 -- remove legacy IBFT PoS staking predeploy helpers -- */

/* START ALPHA CHANGE 546 -- remove pre-funded reward wallet and custom reward-token validation helper -- */
/* FINISH ALPHA CHANGE 546 -- remove pre-funded reward wallet and custom reward-token validation helper -- */

// parsePremineInfo parses premine flag
func (p *genesisParams) parsePremineInfo() error {
	p.premineInfos = make([]*helper.PremineInfo, 0, len(p.premine))

	for _, premine := range p.premine {
		premineInfo, err := helper.ParsePremineInfo(premine)
		/* START ALPHA CHANGE 3130 -- identify invalid premine entries without exposing their optional key -- */
		if err != nil {
			return fmt.Errorf("invalid premine entry: %w", err)
		}
		/* FINISH ALPHA CHANGE 3130 -- identify invalid premine entries without exposing their optional key -- */

		p.premineInfos = append(p.premineInfos, premineInfo)
	}

	return nil
}

/* START ALPHA CHANGE 957 -- remove obsolete zero-address reserve premine validator -- */
/* FINISH ALPHA CHANGE 957 -- remove obsolete zero-address reserve premine validator -- */

// validateBlockTrackerPollInterval validates block tracker block interval
// which can not be 0
func (p *genesisParams) validateBlockTrackerPollInterval() error {
	if p.blockTrackerPollInterval == 0 {
		/*START ALPHA CHANGE 18 -- use local genesis error so the package-level error is active --*/
		return errBlockTrackerPollInterval
		/*FINISH ALPHA CHANGE 18 -- use local genesis error so the package-level error is active --*/
	}

	return nil
}

/* START ALPHA CHANGE 501 -- remove legacy burn-contract validation -- */
/* FINISH ALPHA CHANGE 501 -- remove legacy burn-contract validation -- */

func (p *genesisParams) validateGenesisBaseFeeConfig() error {
	if p.baseFeeConfig == "" {
		return errors.New("invalid input(empty string) for genesis base fee config flag")
	}

	baseFeeInfo, err := parseBaseFeeConfig(p.baseFeeConfig)
	if err != nil {
		return fmt.Errorf("failed to parse base fee config: %w, provided value %s", err, p.baseFeeConfig)
	}

	p.parsedBaseFeeConfig = baseFeeInfo

	if baseFeeInfo.baseFee == 0 {
		return errBaseFeeZero
	}

	if baseFeeInfo.baseFeeEM == 0 {
		return errBaseFeeEMZero
	}

	if baseFeeInfo.baseFeeChangeDenom == 0 {
		return errBaseFeeChangeDenomZero
	}

	/* START ALPHA CHANGE 1072 -- reject genesis gas target that would divide by zero -- */
	effectiveBlockGasLimit := p.blockGasLimit
	if effectiveBlockGasLimit == 0 {
		effectiveBlockGasLimit = chain.GenesisGasLimit
	}

	/* START ALPHA CHANGE 2051 -- enforce mandatory state transaction capacity in generated genesis -- */
	if effectiveBlockGasLimit < types.StateTransactionGasLimit {
		return errBlockGasLimitBelowStateTransactionGasLimit
	}
	/* FINISH ALPHA CHANGE 2051 -- enforce mandatory state transaction capacity in generated genesis -- */

	if effectiveBlockGasLimit < baseFeeInfo.baseFeeEM {
		return errBlockGasLimitTooLow
	}
	/* FINISH ALPHA CHANGE 1072 -- reject genesis gas target that would divide by zero -- */

	return nil
}

/* START ALPHA CHANGE 822 -- remove EOA proxy admin validation helper -- */
/* FINISH ALPHA CHANGE 822 -- remove EOA proxy admin validation helper -- */

/* START ALPHA CHANGE 502 -- remove legacy burn-contract flag helper -- */
/* FINISH ALPHA CHANGE 502 -- remove legacy burn-contract flag helper -- */

// extractNativeTokenMetadata parses provided native token metadata (such as name, symbol and decimals count)
func (p *genesisParams) extractNativeTokenMetadata() error {
	tokenConfig, err := innbcbft.ParseRawTokenConfig(p.nativeTokenConfigRaw)
	if err != nil {
		return err
	}

	p.nativeTokenConfig = tokenConfig

	return nil
}

func (p *genesisParams) getResult() command.CommandResult {
	return &GenesisResult{
		Message: fmt.Sprintf("\nGenesis written to %s\n", p.genesisPath),
	}
}
