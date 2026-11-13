package version

import (
	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/versioning"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	return &cobra.Command{
		Use: "version",
		/*START ALPHA CHANGE 8 -- rebrand version command short description --*/
		Short: "Returns the current INNBC Chain version",
		/*FINISH ALPHA CHANGE 8 -- rebrand version command short description --*/
		Args: cobra.NoArgs,
		Run:  runCommand,
	}
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	outputter.SetCommandResult(
		&VersionResult{
			Version:   versioning.Version,
			Commit:    versioning.Commit,
			Branch:    versioning.Branch,
			BuildTime: versioning.BuildTime,
		},
	)
}
