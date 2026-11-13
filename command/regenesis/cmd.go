/* START ALPHA CHANGE 3262 -- retire history traversal and retain the state-root lookup command -- */
package regenesis

import (
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "regenesis",
		Short: "Queries block state roots. Only accepts subcommands.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(GetRootCMD())

	return cmd
}

/* FINISH ALPHA CHANGE 3262 -- retire history traversal and retain the state-root lookup command -- */
