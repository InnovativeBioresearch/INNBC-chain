/* START ALPHA CHANGE 1693 -- report independent snapshot build results -- */
package build

import (
	"bytes"
	"fmt"

	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
)

type SnapshotBuildResult struct {
	SnapshotKind string `json:"snapshot_kind"`
	BlockNumber  uint64 `json:"block_number"`
	BlockHash    string `json:"block_hash"`
	StateRoot    string `json:"state_root"`
	RetainedFrom uint64 `json:"retained_from"`
	RetainedTo   uint64 `json:"retained_to"`
	SnapshotDir  string `json:"snapshot_dir"`
	LatestPath   string `json:"latest_path"`
	Duration     string `json:"duration"`
}

func snapshotBuildResultFromBuilder(
	result *snapshotbuilder.Result,
) *SnapshotBuildResult {
	return &SnapshotBuildResult{
		SnapshotKind: string(result.Kind),
		BlockNumber:  result.BlockNumber,
		BlockHash:    result.BlockHash.String(),
		StateRoot:    result.StateRoot.String(),
		RetainedFrom: result.RetainedFrom,
		RetainedTo:   result.RetainedTo,
		SnapshotDir:  result.SnapshotDir,
		LatestPath:   result.LatestPath,
		Duration:     result.Duration.String(),
	}
}

func (r *SnapshotBuildResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[SNAPSHOT BUILT]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Snapshot Kind|%s", r.SnapshotKind),
		fmt.Sprintf("Block Number|%d", r.BlockNumber),
		fmt.Sprintf("Block Hash|%s", r.BlockHash),
		fmt.Sprintf("State Root|%s", r.StateRoot),
		fmt.Sprintf("Retained From|%d", r.RetainedFrom),
		fmt.Sprintf("Retained To|%d", r.RetainedTo),
		fmt.Sprintf("Snapshot Directory|%s", r.SnapshotDir),
		fmt.Sprintf("Latest Pointer|%s", r.LatestPath),
		fmt.Sprintf("Duration|%s", r.Duration),
	}))

	return buffer.String()
}

/* FINISH ALPHA CHANGE 1693 -- report independent snapshot build results -- */
