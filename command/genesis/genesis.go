package genesis

import (
	"fmt"

	/* START ALPHA CHANGE 3299 -- apply genesis execution failure status after output handling -- */
	"os"
	/* FINISH ALPHA CHANGE 3299 -- apply genesis execution failure status after output handling -- */

	"github.com/spf13/cobra"

	/* START ALPHA CHANGE 3216 -- remove the retired genesis predeploy command dependency -- */
	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"

	/* FINISH ALPHA CHANGE 3216 -- remove the retired genesis predeploy command dependency -- */

	/* START ALPHA CHANGE 723 -- remove legacy ibft dependency from genesis CLI defaults -- */
	/* FINISH ALPHA CHANGE 723 -- remove legacy ibft dependency from genesis CLI defaults -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	/* START ALPHA CHANGE 724 -- remove legacy IBFT validator-type import from genesis CLI -- */ /* FINISH ALPHA CHANGE 724 -- remove legacy IBFT validator-type import from genesis CLI -- */)

const defaultEpochSize = 30

/* START ALPHA CHANGE 3216 -- retire genesis predeploy and reject positional fallback into genesis generation -- */
func GetCommand() *cobra.Command {
	genesisCmd := &cobra.Command{
		Use:     "genesis",
		Short:   "Generates the genesis configuration file with the passed in parameters",
		Args:    cobra.NoArgs,
		PreRunE: preRunCommand,
		/* START ALPHA CHANGE 3299 -- apply genesis execution failure status after output handling -- */
		Run: func(cmd *cobra.Command, args []string) {
			if exitCode := runCommand(cmd, args); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
		/* FINISH ALPHA CHANGE 3299 -- apply genesis execution failure status after output handling -- */
	}

	setFlags(genesisCmd)
	setLegacyFlags(genesisCmd)

	return genesisCmd
}

/* FINISH ALPHA CHANGE 3216 -- retire genesis predeploy and reject positional fallback into genesis generation -- */

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.genesisPath,
		dirFlag,
		fmt.Sprintf("./%s", command.DefaultGenesisFileName),
		/*START ALPHA CHANGE 11 -- rebrand genesis output flag description --*/
		"the directory for the INNBC Chain genesis data",
		/*FINISH ALPHA CHANGE 11 -- rebrand genesis output flag description --*/
	)

	cmd.Flags().Uint64Var(
		&params.chainID,
		chainIDFlag,
		command.DefaultChainID,
		"the ID of the chain",
	)

	cmd.Flags().StringVar(
		&params.name,
		nameFlag,
		command.DefaultChainName,
		"the name for the chain",
	)

	cmd.Flags().StringArrayVar(
		&params.premine,
		premineFlag,
		[]string{},
		fmt.Sprintf(
			"the premined accounts and balances (format: <address>[:<balance>]). Default premined balance: %d",
			command.DefaultPremineBalance,
		),
	)

	cmd.Flags().Uint64Var(
		&params.blockGasLimit,
		blockGasLimitFlag,
		command.DefaultGenesisGasLimit,
		"the maximum amount of gas used by all transactions in a block",
	)

	/* START ALPHA CHANGE 503 -- remove legacy burn-contract genesis CLI flag -- */
	/* FINISH ALPHA CHANGE 503 -- remove legacy burn-contract genesis CLI flag -- */

	cmd.Flags().StringVar(
		&params.baseFeeConfig,
		genesisBaseFeeConfigFlag,
		command.DefaultGenesisBaseFeeConfig,
		`initial base fee(in wei), base fee elasticity multiplier, and base fee change denominator
		(provided in the following format: [<baseFee>][:<baseFeeEM>][:<baseFeeChangeDenom>]). 
		BaseFeeChangeDenom represents the value to bound the amount the base fee can change between blocks.
		Default BaseFee is 1 wei, BaseFeeEM is 2 and BaseFeeChangeDenom is 8.
		Note: BaseFee, BaseFeeEM, and BaseFeeChangeDenom should be greater than 0.`,
	)

	/* START ALPHA CHANGE 637 -- remove legacy bootnode genesis CLI flag -- */
	/* FINISH ALPHA CHANGE 637 -- remove legacy bootnode genesis CLI flag -- */

	cmd.Flags().StringVar(
		&params.consensusRaw,
		command.ConsensusFlag,
		string(command.DefaultConsensus),
		"the consensus protocol to be used",
	)

	cmd.Flags().Uint64Var(
		&params.epochSize,
		epochSizeFlag,
		defaultEpochSize,
		"the epoch size for the chain",
	)

	/* START ALPHA CHANGE 990 -- document fixed proxy admin executor genesis wiring -- */
	/* Proxy admin is fixed to INNBCProxyAdminExecutorContract. */
	/* FINISH ALPHA CHANGE 990 -- document fixed proxy admin executor genesis wiring -- */

	/* START ALPHA CHANGE 725 -- remove legacy PoS and IBFT validator genesis CLI flags -- */
	cmd.Flags().Uint64Var(
		&params.minNumValidators,
		command.MinValidatorCountFlag,
		1,
		"the minimum number of validators in the validator set",
	)

	cmd.Flags().Uint64Var(
		&params.maxNumValidators,
		command.MaxValidatorCountFlag,
		common.MaxSafeJSInt,
		"the maximum number of validators in the validator set",
	)

	cmd.Flags().StringVar(
		&params.validatorsPath,
		command.ValidatorRootFlag,
		command.DefaultValidatorRoot,
		"root path containing validators secrets",
	)

	cmd.Flags().StringVar(
		&params.validatorsPrefixPath,
		command.ValidatorPrefixFlag,
		command.DefaultValidatorPrefix,
		"folder prefix names for validators secrets",
	)

	cmd.Flags().StringArrayVar(
		&params.validators,
		command.ValidatorFlag,
		[]string{},
		/* START ALPHA CHANGE 2022 -- document the mandatory explicit genesis possession proof -- */
		"validators defined by user (innbcbft format: <P2P multi address>:<ECDSA address>:<public BLS key>:<BLS proof>)",
		/* FINISH ALPHA CHANGE 2022 -- document the mandatory explicit genesis possession proof -- */
	)

	cmd.MarkFlagsMutuallyExclusive(command.ValidatorFlag, command.ValidatorRootFlag)
	cmd.MarkFlagsMutuallyExclusive(command.ValidatorFlag, command.ValidatorPrefixFlag)
	/* FINISH ALPHA CHANGE 725 -- remove legacy PoS and IBFT validator genesis CLI flags -- */

	// INNBCBFT
	{
		cmd.Flags().Uint64Var(
			&params.sprintSize,
			sprintSizeFlag,
			defaultSprintSize,
			"the number of block included into a sprint",
		)

		/* START ALPHA CHANGE 2515 -- document whole-second genesis block intervals -- */
		cmd.Flags().DurationVar(
			&params.blockTime,
			blockTimeFlag,
			defaultBlockTime,
			"the predefined period which determines block creation frequency; "+
				"must be a whole number of seconds and at least 1s",
		)
		/* FINISH ALPHA CHANGE 2515 -- document whole-second genesis block intervals -- */

		/* START ALPHA CHANGE 547 -- remove pre-funded epoch reward genesis CLI flag -- */
		/* FINISH ALPHA CHANGE 547 -- remove pre-funded epoch reward genesis CLI flag -- */

		// regenesis flag that allows to start from non-empty database
		cmd.Flags().StringVar(
			&params.initialStateRoot,
			trieRootFlag,
			"",
			/* START ALPHA CHANGE 3183 -- describe strict optional external trie-root input -- */
			"optional trie root from the corresponding triedb; "+
				"use exactly 64 hexadecimal digits with optional lowercase 0x prefix; "+
				"empty or a full-length zero hash means no external root",
			/* FINISH ALPHA CHANGE 3183 -- describe strict optional external trie-root input -- */
		)

		cmd.Flags().StringVar(
			&params.nativeTokenConfigRaw,
			nativeTokenConfigFlag,
			"",
			/* START ALPHA CHANGE 784 -- document simplified mandatory INNBC native token config format -- */
			"native token configuration, provided in the following format: "+
				"<name:symbol:decimals count>",
			/* FINISH ALPHA CHANGE 784 -- document simplified mandatory INNBC native token config format -- */
		)

		/* START ALPHA CHANGE 548 -- remove custom reward-token and reward-wallet genesis CLI flags -- */
		/* FINISH ALPHA CHANGE 548 -- remove custom reward-token and reward-wallet genesis CLI flags -- */

		/* START ALPHA CHANGE 3319 -- remove the retired wall-clock drift flag -- */
		/* FINISH ALPHA CHANGE 3319 -- remove the retired wall-clock drift flag -- */

		cmd.Flags().DurationVar(
			&params.blockTrackerPollInterval,
			blockTrackerPollIntervalFlag,
			defaultBlockTrackerPollInterval,
			/* START ALPHA CHANGE 529 -- remove rootchain wording from block tracker flag description -- */
			"interval at which the block tracker polls for the latest tracked block",
			/* FINISH ALPHA CHANGE 529 -- remove rootchain wording from block tracker flag description -- */
		)

		/* START ALPHA CHANGE 73 -- add INNBC validator registry genesis CLI flags -- */
		/* START ALPHA CHANGE 2642 -- describe the mandatory one-wei genesis minimum -- */
		cmd.Flags().StringVar(
			&params.validatorRegistryMinimumStakeRaw,
			validatorRegistryMinimumStakeFlag,
			"1",
			"initial validator registry minimum stake in wei; must be exactly 1; governance may change the live minimum later",
		)
		/* FINISH ALPHA CHANGE 2642 -- describe the mandatory one-wei genesis minimum -- */

		cmd.Flags().StringVar(
			&params.validatorRegistryProposalDurationPresetRaw,
			validatorRegistryProposalDurationPresetFlag,
			innbcbft.ValidatorRegistryProposalDurationStandard,
			"proposal duration preset for INNBC validator registry governance; allowed values: short, standard, long",
		)
		/* FINISH ALPHA CHANGE 73 -- add INNBC validator registry genesis CLI flags -- */
	}

	// Access Control Lists
	{
		cmd.Flags().StringArrayVar(
			&params.contractDeployerAllowListAdmin,
			contractDeployerAllowListAdminFlag,
			[]string{},
			"list of addresses to use as admin accounts in the contract deployer allow list",
		)

		cmd.Flags().StringArrayVar(
			&params.contractDeployerAllowListEnabled,
			contractDeployerAllowListEnabledFlag,
			[]string{},
			"list of addresses to enable by default in the contract deployer allow list",
		)

		cmd.Flags().StringArrayVar(
			&params.contractDeployerBlockListAdmin,
			contractDeployerBlockListAdminFlag,
			[]string{},
			"list of addresses to use as admin accounts in the contract deployer block list",
		)

		cmd.Flags().StringArrayVar(
			&params.contractDeployerBlockListEnabled,
			contractDeployerBlockListEnabledFlag,
			[]string{},
			"list of addresses to enable by default in the contract deployer block list",
		)

		cmd.Flags().StringArrayVar(
			&params.transactionsAllowListAdmin,
			transactionsAllowListAdminFlag,
			[]string{},
			"list of addresses to use as admin accounts in the transactions allow list",
		)

		cmd.Flags().StringArrayVar(
			&params.transactionsAllowListEnabled,
			transactionsAllowListEnabledFlag,
			[]string{},
			"list of addresses to enable by default in the transactions allow list",
		)

		cmd.Flags().StringArrayVar(
			&params.transactionsBlockListAdmin,
			transactionsBlockListAdminFlag,
			[]string{},
			"list of addresses to use as admin accounts in the transactions block list",
		)

		cmd.Flags().StringArrayVar(
			&params.transactionsBlockListEnabled,
			transactionsBlockListEnabledFlag,
			[]string{},
			"list of addresses to enable by default in the transactions block list",
		)

		/* START ALPHA CHANGE 118 -- remove bridge allow/block list CLI flags from genesis command -- */
		/* FINISH ALPHA CHANGE 118 -- remove bridge allow/block list CLI flags from genesis command -- */
	}
}

// setLegacyFlags sets the legacy flags to preserve backwards compatibility
// with running partners
func setLegacyFlags(cmd *cobra.Command) {
	// Legacy chainid flag
	cmd.Flags().Uint64Var(
		&params.chainID,
		chainIDFlagLEGACY,
		command.DefaultChainID,
		"the ID of the chain",
	)

	_ = cmd.Flags().MarkHidden(chainIDFlagLEGACY)
}

func preRunCommand(cmd *cobra.Command, _ []string) error {
	if err := params.validateFlags(); err != nil {
		return err
	}

	helper.SetRequiredFlags(cmd, params.getRequiredFlags())

	return params.initRawParams()
}

/* START ALPHA CHANGE 3299 -- apply genesis execution failure status after output handling -- */
func runCommand(cmd *cobra.Command, _ []string) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	var err error

	if params.isINNBCBFTConsensus() {
		err = params.generatePolyBftChainConfig(outputter)
	} else {
		/* START ALPHA CHANGE 792 -- use fmt.Appendf for genesis notice output -- */
		_, _ = outputter.Write(fmt.Appendf(nil, "%s\n", common.IBFTImportantNotice))
		/* FINISH ALPHA CHANGE 792 -- use fmt.Appendf for genesis notice output -- */
		err = params.generateGenesis()
	}

	if err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(params.getResult())

	return 0
}

/* FINISH ALPHA CHANGE 3299 -- apply genesis execution failure status after output handling -- */
