/* START ALPHA CHANGE 2078 -- expose historical archive construction with reliable failure exit status -- */
package archive

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
	chainFlag           = "chain"
	sourceDataDirFlag   = "source-data-dir"
	outputRootFlag      = "output-root"
	segmentBlocksFlag   = "segment-blocks"
	maxIndexRecordsFlag = "max-index-records"
	mdbxMapSizeGBFlag   = "mdbx-map-size-gb"
)

type params struct {
	chainPath     string
	sourceDataDir string
	outputRoot    string

	segmentBlocks   uint64
	maxIndexRecords uint64
	mdbxMapSizeGB   uint64
	/* START ALPHA CHANGE 3472 -- retain archive destination page flag -- */
	mdbxPageSizeBytes serverconfig.MDBXPageSize
	/* FINISH ALPHA CHANGE 3472 -- retain archive destination page flag -- */
}

type ArchiveResult struct {
	PreviousArchivedThrough uint64 `json:"previous_archived_through"`
	ArchivedThrough         uint64 `json:"archived_through"`
	SegmentsAdded           uint64 `json:"segments_added"`
	TransactionsAdded       uint64 `json:"transactions_added"`
	ManifestPath            string `json:"manifest_path"`
	LatestPath              string `json:"latest_path"`
}

func GetCommand() *cobra.Command {
	archiveParams := &params{
		chainPath:       "/etc/innbc/genesis.json",
		sourceDataDir:   "/var/lib/innbc/node1",
		outputRoot:      "/var/lib/innbc/history-archive",
		segmentBlocks:   serverconfig.DefaultHistorySegmentBlocks,
		maxIndexRecords: snapshotbuilder.DefaultArchiveMaxIndexRecords,
		mdbxMapSizeGB:   serverconfig.DefaultMDBXMapSizeGB,
	}

	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Builds or extends the uncompressed historical transaction archive",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			// runCommand prints the result before returning. The plain error
			// formatter already exits; this also enforces failure in JSON mode.
			if exitCode := runCommand(cmd, archiveParams); exitCode != 0 {
				os.Exit(exitCode)
			}
		},
	}

	cmd.Flags().StringVar(
		&archiveParams.chainPath,
		chainFlag,
		archiveParams.chainPath,
		"the INNBC chain configuration file",
	)
	cmd.Flags().StringVar(
		&archiveParams.sourceDataDir,
		sourceDataDirFlag,
		archiveParams.sourceDataDir,
		"the source node data directory",
	)
	cmd.Flags().StringVar(
		&archiveParams.outputRoot,
		outputRootFlag,
		archiveParams.outputRoot,
		"the historical archive publication root",
	)
	cmd.Flags().Uint64Var(
		&archiveParams.segmentBlocks,
		segmentBlocksFlag,
		archiveParams.segmentBlocks,
		"the number of canonical blocks in each immutable archive segment",
	)
	cmd.Flags().Uint64Var(
		&archiveParams.maxIndexRecords,
		maxIndexRecordsFlag,
		archiveParams.maxIndexRecords,
		fmt.Sprintf(
			"the maximum transaction-index records per segment (1 through %d)",
			snapshotbuilder.MaximumArchiveIndexRecords,
		),
	)
	cmd.Flags().Uint64Var(
		&archiveParams.mdbxMapSizeGB,
		mdbxMapSizeGBFlag,
		archiveParams.mdbxMapSizeGB,
		"the MDBX map size in GiB for each archive segment",
	)
	/* START ALPHA CHANGE 3472 -- expose archive database creation pages -- */
	cmd.Flags().Var(
		&archiveParams.mdbxPageSizeBytes,
		"mdbx-page-size-bytes",
		"destination MDBX page size in bytes; blank selects 4096; existing archives must match",
	)
	/* FINISH ALPHA CHANGE 3472 -- expose archive database creation pages -- */

	return cmd
}

func runCommand(cmd *cobra.Command, archiveParams *params) int {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	result, err := snapshotbuilder.BuildArchive(
		snapshotbuilder.ArchiveConfig{
			ChainPath:       archiveParams.chainPath,
			SourceDataDir:   archiveParams.sourceDataDir,
			OutputRoot:      archiveParams.outputRoot,
			SegmentBlocks:   archiveParams.segmentBlocks,
			MaxIndexRecords: archiveParams.maxIndexRecords,
			MDBXMapSizeGB:   archiveParams.mdbxMapSizeGB,
			/* START ALPHA CHANGE 3472 -- forward archive creation pages -- */
			MDBXPageSizeBytes: uint64(archiveParams.mdbxPageSizeBytes),
			/* FINISH ALPHA CHANGE 3472 -- forward archive creation pages -- */
			Logger: hclog.New(&hclog.LoggerOptions{
				Name:  "history-archive-builder",
				Level: hclog.Info,
			}),
		},
	)
	if err != nil {
		outputter.SetError(err)

		return 1
	}

	outputter.SetCommandResult(&ArchiveResult{
		PreviousArchivedThrough: result.PreviousArchivedThrough,
		ArchivedThrough:         result.ArchivedThrough,
		SegmentsAdded:           result.SegmentsAdded,
		TransactionsAdded:       result.TransactionsAdded,
		ManifestPath:            result.ManifestPath,
		LatestPath:              result.LatestPath,
	})

	return 0
}

func (r *ArchiveResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[HISTORY ARCHIVE BUILT]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Previous Archived Through|%d", r.PreviousArchivedThrough),
		fmt.Sprintf("Archived Through|%d", r.ArchivedThrough),
		fmt.Sprintf("Segments Added|%d", r.SegmentsAdded),
		fmt.Sprintf("Transactions Indexed|%d", r.TransactionsAdded),
		fmt.Sprintf("Archive Manifest|%s", r.ManifestPath),
		fmt.Sprintf("Archive Latest Pointer|%s", r.LatestPath),
	}))

	return buffer.String()
}

/* FINISH ALPHA CHANGE 2078 -- expose historical archive construction with reliable failure exit status -- */
