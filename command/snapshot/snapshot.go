package snapshot

/* START ALPHA CHANGE 2264 -- register the independent database backup command -- */
import (
	"github.com/innovativebioresearch/innbc-chain/command/snapshot/archive"
	"github.com/innovativebioresearch/innbc-chain/command/snapshot/backup"
	"github.com/innovativebioresearch/innbc-chain/command/snapshot/build"
	"github.com/innovativebioresearch/innbc-chain/command/snapshot/recoverycheckpoint"
	"github.com/spf13/cobra"
)

/* FINISH ALPHA CHANGE 2264 -- register the independent database backup command -- */

/* START ALPHA CHANGE 1199 -- add snapshot command group -- */
func GetCommand() *cobra.Command {
	snapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Top level command for local snapshot operations. Only accepts subcommands.",
	}

	/* START ALPHA CHANGE 2264 -- expose archival database backup separately from startup snapshots -- */
	snapshotCmd.AddCommand(
		build.GetCommand(),
		archive.GetCommand(),
		recoverycheckpoint.GetCommand(),
		backup.GetCommand(),
	)
	/* FINISH ALPHA CHANGE 2264 -- expose archival database backup separately from startup snapshots -- */

	return snapshotCmd
}

/* FINISH ALPHA CHANGE 1199 -- add snapshot command group -- */
