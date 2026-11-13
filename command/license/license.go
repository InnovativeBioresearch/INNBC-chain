package license

import (
	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/spf13/cobra"

	"github.com/innovativebioresearch/innbc-chain/licenses"
)

func GetCommand() *cobra.Command {
	return &cobra.Command{
		Use: "license",
		/*START ALPHA CHANGE 9 -- rebrand license command short description --*/
		Short: "Returns INNBC Chain license and dependency attributions",
		/*FINISH ALPHA CHANGE 9 -- rebrand license command short description --*/
		Args: cobra.NoArgs,
		Run:  runCommand,
	}
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	bsdLicenses, err := licenses.GetBSDLicenses()
	if err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(
		&LicenseResult{
			BSDLicenses: bsdLicenses,
		},
	)
}
