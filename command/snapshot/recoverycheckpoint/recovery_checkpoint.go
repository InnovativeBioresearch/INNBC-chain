/* START ALPHA CHANGE 2079 -- expose recovery checkpoint construction with reliable failure exit status -- */
package recoverycheckpoint

import (
	"bytes"
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	serverconfig "github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	"github.com/spf13/cobra"
)

const (
	chainFlag                  = "chain"
	sourceDataDirFlag          = "source-data-dir"
	snapshotOutputRootFlag     = "snapshot-output-root"
	archiveOutputRootFlag      = "archive-output-root"
	minimumRetentionBlocksFlag = "minimum-retention-blocks"
	mdbxMapSizeGBFlag          = "mdbx-map-size-gb"
)

type params struct {
	chainPath     string
	sourceDataDir string

	snapshotOutputRoot string
	archiveOutputRoot  string

	minimumRetentionBlocks uint64
	mdbxMapSizeGB          uint64
	/* START ALPHA CHANGE 3474 -- retain recovery destination page flag -- */
	mdbxPageSizeBytes serverconfig.MDBXPageSize
	/* FINISH ALPHA CHANGE 3474 -- retain recovery destination page flag -- */
}

type RecoveryCheckpointResult struct {
	CheckpointNumber uint64 `json:"checkpoint_number"`
	CheckpointHash   string `json:"checkpoint_hash"`

	SharedBoundaryNumber uint64 `json:"shared_boundary_number"`
	SharedBoundaryHash   string `json:"shared_boundary_hash"`

	ActualRetentionBlocks uint64 `json:"actual_retention_blocks"`
	ArchiveActualThrough  uint64 `json:"archive_actual_through"`

	CheckpointDir   string `json:"checkpoint_directory"`
	RecoverySetPath string `json:"recovery_set_path"`
	LatestPath      string `json:"latest_path"`
}

func GetCommand() *cobra.Command {
	recoveryParams := &params{
		chainPath:              "/etc/innbc/genesis.json",
		sourceDataDir:          "/var/lib/innbc/node1",
		snapshotOutputRoot:     "/var/lib/innbc/snapshots",
		archiveOutputRoot:      "/var/lib/innbc/history-archive",
		minimumRetentionBlocks: serverconfig.DefaultHistoryRetentionBlocks,
		mdbxMapSizeGB:          serverconfig.DefaultMDBXMapSizeGB,
	}

	cmd := &cobra.Command{
		Use:   "recovery-checkpoint",
		Short: "Builds a bootable checkpoint joined to an existing historical archive",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			// runCommand prints the result before returning. The plain error
			// formatter already exits; this also enforces failure in JSON mode.
			if exitCode := runCommand(cmd, recoveryParams); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
	}

	cmd.Flags().StringVar(
		&recoveryParams.chainPath,
		chainFlag,
		recoveryParams.chainPath,
		"the INNBC chain configuration file",
	)
	cmd.Flags().StringVar(
		&recoveryParams.sourceDataDir,
		sourceDataDirFlag,
		recoveryParams.sourceDataDir,
		"the source node data directory",
	)
	cmd.Flags().StringVar(
		&recoveryParams.snapshotOutputRoot,
		snapshotOutputRootFlag,
		recoveryParams.snapshotOutputRoot,
		"the root directory for bootable snapshots",
	)
	cmd.Flags().StringVar(
		&recoveryParams.archiveOutputRoot,
		archiveOutputRootFlag,
		recoveryParams.archiveOutputRoot,
		"the existing historical archive root",
	)
	cmd.Flags().Uint64Var(
		&recoveryParams.minimumRetentionBlocks,
		minimumRetentionBlocksFlag,
		recoveryParams.minimumRetentionBlocks,
		"the minimum recent block history retained in the checkpoint",
	)
	cmd.Flags().Uint64Var(
		&recoveryParams.mdbxMapSizeGB,
		mdbxMapSizeGBFlag,
		recoveryParams.mdbxMapSizeGB,
		"the MDBX map size in GiB for checkpoint blockchain and trie storage",
	)
	/* START ALPHA CHANGE 3474 -- expose linked recovery database creation pages -- */
	cmd.Flags().Var(
		&recoveryParams.mdbxPageSizeBytes,
		"mdbx-page-size-bytes",
		"destination MDBX page size in bytes; blank selects 4096; linked archives must match",
	)
	/* FINISH ALPHA CHANGE 3474 -- expose linked recovery database creation pages -- */

	return cmd
}

func runCommand(cmd *cobra.Command, recoveryParams *params) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	result, err := snapshotbuilder.BuildRecoveryCheckpoint(
		snapshotbuilder.RecoveryCheckpointConfig{
			ChainPath:              recoveryParams.chainPath,
			SourceDataDir:          recoveryParams.sourceDataDir,
			SnapshotOutputRoot:     recoveryParams.snapshotOutputRoot,
			ArchiveOutputRoot:      recoveryParams.archiveOutputRoot,
			MinimumRetentionBlocks: recoveryParams.minimumRetentionBlocks,
			MDBXMapSizeGB:          recoveryParams.mdbxMapSizeGB,
			/* START ALPHA CHANGE 3474 -- forward recovery creation pages -- */
			MDBXPageSizeBytes: uint64(recoveryParams.mdbxPageSizeBytes),
			/* FINISH ALPHA CHANGE 3474 -- forward recovery creation pages -- */
			Logger: hclog.New(&hclog.LoggerOptions{
				Name:  "recovery-checkpoint-builder",
				Level: hclog.Info,
			}),
		},
	)
	if err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(&RecoveryCheckpointResult{
		CheckpointNumber:      result.CheckpointNumber,
		CheckpointHash:        result.CheckpointHash.String(),
		SharedBoundaryNumber:  result.SharedBoundaryNumber,
		SharedBoundaryHash:    result.SharedBoundaryHash.String(),
		ActualRetentionBlocks: result.ActualRetentionBlocks,
		ArchiveActualThrough:  result.ArchiveActualThrough,
		CheckpointDir:         result.CheckpointDir,
		RecoverySetPath:       result.RecoverySetPath,
		LatestPath:            result.LatestPath,
	})

	return 0
}

func (r *RecoveryCheckpointResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[RECOVERY CHECKPOINT BUILT]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Checkpoint Number|%d", r.CheckpointNumber),
		fmt.Sprintf("Checkpoint Hash|%s", r.CheckpointHash),
		fmt.Sprintf("Shared Boundary Number|%d", r.SharedBoundaryNumber),
		fmt.Sprintf("Shared Boundary Hash|%s", r.SharedBoundaryHash),
		fmt.Sprintf("Actual Retention Blocks|%d", r.ActualRetentionBlocks),
		fmt.Sprintf("Archive Actual Through|%d", r.ArchiveActualThrough),
		fmt.Sprintf("Checkpoint Directory|%s", r.CheckpointDir),
		fmt.Sprintf("Recovery Relationship|%s", r.RecoverySetPath),
		fmt.Sprintf("Checkpoint Latest Pointer|%s", r.LatestPath),
	}))

	return buffer.String()
}

/* FINISH ALPHA CHANGE 2079 -- expose recovery checkpoint construction with reliable failure exit status -- */
