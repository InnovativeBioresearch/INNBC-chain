/* START ALPHA CHANGE 1692 -- expose independent snapshot builder command -- */
package build

import (
	"fmt"

	/* START ALPHA CHANGE 3280 -- terminate failed snapshot commands after helper cleanup and output -- */
	"os"
	/* FINISH ALPHA CHANGE 3280 -- terminate failed snapshot commands after helper cleanup and output -- */

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/command"
	serverconfig "github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	"github.com/spf13/cobra"
)

const (
	chainFlag           = "chain"
	sourceDataDirFlag   = "source-data-dir"
	outputRootFlag      = "output-root"
	retentionBlocksFlag = "retention-blocks"
	segmentBlocksFlag   = "segment-blocks"
	/* START ALPHA CHANGE 1781 -- expose one destination MDBX map size flag -- */
	mdbxMapSizeGBFlag = "mdbx-map-size-gb"
	/* FINISH ALPHA CHANGE 1781 -- expose one destination MDBX map size flag -- */
)

type params struct {
	chainPath     string
	sourceDataDir string
	outputRoot    string

	retentionBlocks uint64
	segmentBlocks   uint64

	mdbxMapSizeGB uint64
	/* START ALPHA CHANGE 3471 -- retain parsed snapshot page-size flag -- */
	mdbxPageSizeBytes serverconfig.MDBXPageSize
	/* FINISH ALPHA CHANGE 3471 -- retain parsed snapshot page-size flag -- */
}

func GetCommand() *cobra.Command {
	buildParams := &params{
		chainPath:       "/etc/innbc/genesis.json",
		sourceDataDir:   "/var/lib/innbc/node1",
		outputRoot:      "/var/lib/innbc/snapshots",
		retentionBlocks: serverconfig.DefaultHistoryRetentionBlocks,
		segmentBlocks:   serverconfig.DefaultHistorySegmentBlocks,
		mdbxMapSizeGB:   serverconfig.DefaultMDBXMapSizeGB,
	}

	/* START ALPHA CHANGE 1825 -- align snapshot command with segmented artifact terminology -- */
	cmd := &cobra.Command{
		Use:   "build [segmented|checkpoint]",
		Short: "Builds and atomically publishes a snapshot from a live full-history node",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			/* START ALPHA CHANGE 3280 -- preserve failure exit status in JSON mode -- */
			if exitCode := runCommand(cmd, args, buildParams); exitCode != 0 {
				os.Exit(exitCode)
			}
			/* FINISH ALPHA CHANGE 3280 -- preserve failure exit status in JSON mode -- */
		},
	}
	/* FINISH ALPHA CHANGE 1825 -- align snapshot command with segmented artifact terminology -- */

	cmd.Flags().StringVar(
		&buildParams.chainPath,
		chainFlag,
		buildParams.chainPath,
		"the INNBC chain configuration file",
	)
	cmd.Flags().StringVar(
		&buildParams.sourceDataDir,
		sourceDataDirFlag,
		buildParams.sourceDataDir,
		"the full-history source node data directory",
	)
	cmd.Flags().StringVar(
		&buildParams.outputRoot,
		outputRootFlag,
		buildParams.outputRoot,
		"the root directory for published snapshots",
	)
	cmd.Flags().Uint64Var(
		&buildParams.retentionBlocks,
		retentionBlocksFlag,
		buildParams.retentionBlocks,
		"the number of recent canonical blocks included in the snapshot",
	)
	cmd.Flags().Uint64Var(
		&buildParams.segmentBlocks,
		segmentBlocksFlag,
		buildParams.segmentBlocks,
		"the block count per recent-history segment",
	)
	/* START ALPHA CHANGE 1782 -- register one snapshot destination MDBX map size -- */
	cmd.Flags().Uint64Var(
		&buildParams.mdbxMapSizeGB,
		mdbxMapSizeGBFlag,
		buildParams.mdbxMapSizeGB,
		"the MDBX map size in GiB for destination blockchain and trie storage",
	)
	/* FINISH ALPHA CHANGE 1782 -- register one snapshot destination MDBX map size -- */
	/* START ALPHA CHANGE 3471 -- expose defaultable snapshot destination pages -- */
	cmd.Flags().Var(
		&buildParams.mdbxPageSizeBytes,
		"mdbx-page-size-bytes",
		"destination MDBX page size in bytes; blank selects 4096",
	)
	/* FINISH ALPHA CHANGE 3471 -- expose defaultable snapshot destination pages -- */

	return cmd
}

/* START ALPHA CHANGE 3280 -- return snapshot command status after builder cleanup and formatted output -- */
func runCommand(cmd *cobra.Command, args []string, buildParams *params) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	kind, err := parseKind(args[0])
	if err != nil {
		outputter.SetError(err)

		return 1
	}

	result, err := snapshotbuilder.Build(snapshotbuilder.Config{
		Kind:            kind,
		ChainPath:       buildParams.chainPath,
		SourceDataDir:   buildParams.sourceDataDir,
		OutputRoot:      buildParams.outputRoot,
		RetentionBlocks: buildParams.retentionBlocks,
		SegmentBlocks:   buildParams.segmentBlocks,
		MDBXMapSizeGB:   buildParams.mdbxMapSizeGB,
		/* START ALPHA CHANGE 3471 -- forward snapshot creation pages -- */
		MDBXPageSizeBytes: uint64(buildParams.mdbxPageSizeBytes),
		/* FINISH ALPHA CHANGE 3471 -- forward snapshot creation pages -- */
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:  "snapshot-builder",
			Level: hclog.Info,
		}),
	})
	if err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(snapshotBuildResultFromBuilder(result))

	return 0
}

/* FINISH ALPHA CHANGE 3280 -- return snapshot command status after builder cleanup and formatted output -- */

/* START ALPHA CHANGE 1826 -- parse canonical segmented snapshot command name -- */
func parseKind(raw string) (snapshotbuilder.Kind, error) {
	switch raw {
	case "segmented":
		return snapshotbuilder.KindSegmented, nil
	case "checkpoint":
		return snapshotbuilder.KindCheckpoint, nil
	default:
		return "", fmt.Errorf(
			"unsupported snapshot type %q; expected segmented or checkpoint",
			raw,
		)
	}
}

/* FINISH ALPHA CHANGE 1826 -- parse canonical segmented snapshot command name -- */

/* FINISH ALPHA CHANGE 1692 -- expose independent snapshot builder command -- */
