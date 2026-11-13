/* START ALPHA CHANGE 2263 -- expose standalone archival blockchain database backups -- */
package backup

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"

	"github.com/innovativebioresearch/innbc-chain/command"
	serverconfig "github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
)

type params struct {
	chainPath     string
	dataDir       string
	outputRoot    string
	mdbxMapSizeGB uint64
	/* START ALPHA CHANGE 3473 -- retain backup destination page flag -- */
	mdbxPageSizeBytes serverconfig.MDBXPageSize
	/* FINISH ALPHA CHANGE 3473 -- retain backup destination page flag -- */
}

type commandResult struct {
	*snapshotbuilder.DatabaseBackupResult
}

func (r *commandResult) GetOutput() string {
	return fmt.Sprintf(
		"\n[BLOCKCHAIN DATABASE BACKUP CREATED]\n"+
			"Directory: %s\n"+
			"Block: %d\n"+
			"Block hash: %s\n"+
			"State root: %s\n"+
			"Trie reconstruction: optional, separately invoked\n",
		r.Directory,
		r.BlockNumber,
		r.BlockHash,
		r.StateRoot,
	)
}

func GetCommand() *cobra.Command {
	p := &params{
		mdbxMapSizeGB: serverconfig.DefaultMDBXMapSizeGB,
	}

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Archives a consistent full-history blockchain database and genesis file",
		Long: "Copies a unified blockchain database from one fixed MDBX read view.\n" +
			"The source node may continue normal operation.\n" +
			"Do not replace, prune or restore the source database during export.\n" +
			"The output contains no trie, consensus database, keys or startup snapshot pointer.\n" +
			"Optional trie reconstruction uses the separate node trie-rebuild command.",
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			if status := runCommand(cmd, p); status != 0 {
				os.Exit(status)
			}
		},
	}

	cmd.Flags().StringVar(
		&p.chainPath, "chain", "",
		"the original genesis configuration to preserve",
	)
	cmd.Flags().StringVar(
		&p.dataDir, "data-dir", "",
		"the source full-history node data directory",
	)
	cmd.Flags().StringVar(
		&p.outputRoot, "out", "",
		"the separate directory containing uniquely named database backups",
	)
	cmd.Flags().Uint64Var(
		&p.mdbxMapSizeGB, "mdbx-map-size-gb", p.mdbxMapSizeGB,
		"the destination blockchain MDBX map size in GiB",
	)
	/* START ALPHA CHANGE 3473 -- expose backup database creation pages -- */
	cmd.Flags().Var(
		&p.mdbxPageSizeBytes,
		"mdbx-page-size-bytes",
		"destination MDBX page size in bytes; blank selects 4096",
	)
	/* FINISH ALPHA CHANGE 3473 -- expose backup database creation pages -- */

	return cmd
}

func runCommand(cmd *cobra.Command, p *params) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ctx, cancel := signal.NotifyContext(
		cmd.Context(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	result, err := snapshotbuilder.BuildDatabaseBackup(
		ctx,
		snapshotbuilder.DatabaseBackupConfig{
			ChainPath:     p.chainPath,
			DataDir:       p.dataDir,
			OutputRoot:    p.outputRoot,
			MDBXMapSizeGB: p.mdbxMapSizeGB,
			/* START ALPHA CHANGE 3473 -- forward backup creation pages -- */
			MDBXPageSizeBytes: uint64(p.mdbxPageSizeBytes),
			/* FINISH ALPHA CHANGE 3473 -- forward backup creation pages -- */
			Logger: hclog.New(&hclog.LoggerOptions{
				Name:   "database-backup",
				Level:  hclog.Info,
				Output: os.Stderr,
			}),
		},
	)
	if err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(&commandResult{
		DatabaseBackupResult: result,
	})

	return 0
}

/* FINISH ALPHA CHANGE 2263 -- expose standalone archival blockchain database backups -- */
