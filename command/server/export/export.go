package export

import (
	"encoding/json"
	"errors"
	"fmt"

	/* START ALPHA CHANGE 3300 -- propagate configuration export execution failure status -- */
	"os"
	/* FINISH ALPHA CHANGE 3300 -- propagate configuration export execution failure status -- */

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func GetCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "export",
		Short: "export default-config.yaml file with default parameters that can be used to run the server",
		/* START ALPHA CHANGE 3300 -- propagate configuration export execution failure status -- */
		Run: func(cmd *cobra.Command, args []string) {
			if exitCode := runGenerateConfigCommand(cmd, args); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
		/* FINISH ALPHA CHANGE 3300 -- propagate configuration export execution failure status -- */
	}

	setFlags(configCmd)

	return configCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&paramFlagValues.FileType,
		fileTypeFlag,
		"yaml",
		"file type of exported config file (yaml or json)",
	)
}

/* START ALPHA CHANGE 3300 -- propagate configuration export execution failure status -- */
func runGenerateConfigCommand(cmd *cobra.Command, _ []string) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := generateConfig(*config.DefaultConfig()); err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(&cmdResult{
		CommandOutput: "Configuration file successfully exported",
	})

	return 0
}

/* FINISH ALPHA CHANGE 3300 -- propagate configuration export execution failure status -- */

func generateConfig(config config.Config) error {
	config.Network.MaxPeers = -1
	config.Network.MaxInboundPeers = -1
	config.Network.MaxOutboundPeers = -1

	var (
		data []byte
		err  error
	)

	switch paramFlagValues.FileType {
	case "yaml", "yml":
		data, err = yaml.Marshal(config)
	case "json":
		data, err = json.MarshalIndent(config, "", "    ")
	default:
		return errors.New("invalid file type, only yaml and json are supported")
	}

	if err != nil {
		return fmt.Errorf("could not marshal config struct, %w", err)
	}

	if err := common.SaveFileSafe(
		fmt.Sprintf("default-config.%s", paramFlagValues.FileType),
		data,
		0660); err != nil {
		return fmt.Errorf("failed to create config file %w", err)
	}

	return nil
}
