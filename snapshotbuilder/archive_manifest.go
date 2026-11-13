/* START ALPHA CHANGE 2064 -- define uncompressed historical transaction archive metadata -- */
package snapshotbuilder

/* START ALPHA CHANGE 2070 -- use the concrete archive logger interface -- */
import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/types"
)

/* FINISH ALPHA CHANGE 2070 -- use the concrete archive logger interface -- */

const (
	ArchiveFormatVersion    uint64 = 1
	ArchiveSegmentVersion   uint64 = 1
	TransactionIndexVersion uint64 = 1
	/* START ALPHA CHANGE 2099 -- support cumulative archive manifests up to 128 MiB -- */
	archiveManifestSizeLimit = 128 * 1024 * 1024
	/* FINISH ALPHA CHANGE 2099 -- support cumulative archive manifests up to 128 MiB -- */
	archiveSegmentSizeLimit = 64 * 1024
	archiveLatestSizeLimit  = 64 * 1024

	archiveLatestFileName = "latest.json"
	archiveSegmentsDir    = "segments"
	archiveBlockchainDir  = "blockchain"
	archiveIndexFileName  = "transactions.idx"
	archiveSegmentFile    = "segment.json"
	archiveMDBXDataFile   = "mdbx.dat"
	archiveMDBXLockFile   = "mdbx.lck"
)

/* START ALPHA CHANGE 2071 -- define supported per-segment index record limits -- */
const (
	DefaultArchiveMaxIndexRecords uint64 = 2_000_000
	MaximumArchiveIndexRecords    uint64 = 4_000_000
)

/* FINISH ALPHA CHANGE 2071 -- define supported per-segment index record limits -- */

type ArchiveFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
}

type ArchiveSegment struct {
	From uint64 `json:"from"`
	To   uint64 `json:"to"`

	ParentHash     types.Hash `json:"parentHash"`
	FirstBlockHash types.Hash `json:"firstBlockHash"`
	LastBlockHash  types.Hash `json:"lastBlockHash"`

	TransactionCount uint64 `json:"transactionCount"`

	Directory       string      `json:"directory"`
	SegmentManifest ArchiveFile `json:"segmentManifest"`
}

type ArchiveSegmentManifest struct {
	FormatVersion           uint64 `json:"formatVersion"`
	StorageEncodingVersion  uint64 `json:"storageEncodingVersion"`
	TransactionIndexVersion uint64 `json:"transactionIndexVersion"`

	ChainID          int64      `json:"chainID"`
	GenesisHash      types.Hash `json:"genesisHash"`
	GenesisStateRoot types.Hash `json:"genesisStateRoot"`

	From uint64 `json:"from"`
	To   uint64 `json:"to"`

	ParentHash     types.Hash `json:"parentHash"`
	FirstBlockHash types.Hash `json:"firstBlockHash"`
	LastBlockHash  types.Hash `json:"lastBlockHash"`

	TransactionCount uint64 `json:"transactionCount"`

	BlockchainData   ArchiveFile `json:"blockchainData"`
	TransactionIndex ArchiveFile `json:"transactionIndex"`
}

type ArchiveManifest struct {
	FormatVersion           uint64 `json:"formatVersion"`
	StorageEncodingVersion  uint64 `json:"storageEncodingVersion"`
	TransactionIndexVersion uint64 `json:"transactionIndexVersion"`

	ChainID          int64      `json:"chainID"`
	GenesisHash      types.Hash `json:"genesisHash"`
	GenesisStateRoot types.Hash `json:"genesisStateRoot"`

	SegmentBlocks   uint64 `json:"segmentBlocks"`
	MaxIndexRecords uint64 `json:"maxIndexRecords"`
	ArchivedThrough uint64 `json:"archivedThrough"`

	SourceHeadNumber uint64     `json:"sourceHeadNumber"`
	SourceHeadHash   types.Hash `json:"sourceHeadHash"`

	TransactionCount uint64           `json:"transactionCount"`
	Segments         []ArchiveSegment `json:"segments"`
}

type ArchiveLatest struct {
	FormatVersion uint64 `json:"formatVersion"`

	ArchivedThrough uint64 `json:"archivedThrough"`

	ManifestPath   string `json:"manifestPath"`
	ManifestSHA256 string `json:"manifestSHA256"`
}

type ArchiveConfig struct {
	ChainPath     string
	SourceDataDir string
	OutputRoot    string

	SegmentBlocks   uint64
	MaxIndexRecords uint64
	MDBXMapSizeGB   uint64
	/* START ALPHA CHANGE 3466 -- configure archive database creation pages -- */
	MDBXPageSizeBytes uint64
	/* FINISH ALPHA CHANGE 3466 -- configure archive database creation pages -- */

	/* START ALPHA CHANGE 2070 -- use the concrete archive logger interface -- */
	Logger hclog.Logger
	/* FINISH ALPHA CHANGE 2070 -- use the concrete archive logger interface -- */
}

type ArchiveResult struct {
	PreviousArchivedThrough uint64
	ArchivedThrough         uint64
	SegmentsAdded           uint64
	TransactionsAdded       uint64
	ManifestPath            string
	LatestPath              string
}

func archiveManifestName(digest string) string {
	return fmt.Sprintf("manifest-%s.json", digest)
}

func archiveSegmentDirectoryName(
	from uint64,
	to uint64,
	lastBlockHash types.Hash,
) string {
	hashText := strings.TrimPrefix(lastBlockHash.String(), "0x")

	return fmt.Sprintf(
		"blocks-%020d-%020d-%s",
		from,
		to,
		hashText,
	)
}

/* FINISH ALPHA CHANGE 2064 -- define uncompressed historical transaction archive metadata -- */
