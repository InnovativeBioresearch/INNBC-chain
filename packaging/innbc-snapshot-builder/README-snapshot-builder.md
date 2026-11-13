<!-- START ALPHA CHANGE 1849 -- document standalone snapshot builder package -->
# INNBC Snapshot Builder

<!-- START ALPHA CHANGE 2271 -- distinguish packaged snapshot operations from database backups -->
This package installs the configuration and one-shot wrapper for independently building INNBC snapshots, historical archives and linked recovery checkpoints from a full-history node.

<!-- START ALPHA CHANGE 2282 -- document environment-driven archival backup dispatch -->
This guide also documents standalone blockchain database backups. Database backups are archival artifacts, not startup snapshots. Production operators can run `sudo innbc-snapshot-builder backup` using the installed environment file. Direct `innbc-chain snapshot backup` invocation remains available for explicit paths, including local testing.
<!-- FINISH ALPHA CHANGE 2282 -- document environment-driven archival backup dispatch -->
<!-- FINISH ALPHA CHANGE 2271 -- distinguish packaged snapshot operations from database backups -->

## Prerequisite

The shared INNBC binary must already exist at:

```text
/usr/local/bin/innbc-chain
```

<!-- START ALPHA CHANGE 2272 -- document database backup and optional reconstruction prerequisites -->
It is installed by the separate `innbc-nodeclient` package. The installed binary must contain the commands required for the selected operation:

- `snapshot archive` for the historical transaction archive;
- `snapshot recovery-checkpoint` for linked recovery checkpoints;
- `snapshot backup` for standalone blockchain database backups;
- `node trie-rebuild` for optional offline trie reconstruction.

Installing this wrapper package does not replace the node binary.

Database backups require a unified blockchain database containing complete canonical history from genesis. Segmented recent-history databases are not supported by the backup command.
<!-- FINISH ALPHA CHANGE 2272 -- document database backup and optional reconstruction prerequisites -->

## Installed Files

```text
/etc/innbc/snapshot-builder.env.example
/etc/innbc/snapshot-builder.env
/usr/local/bin/innbc-snapshot-builder
```

The installer preserves an existing `/etc/innbc/snapshot-builder.env`.

<!-- START ALPHA CHANGE 1929 -- document the unprivileged production ownership boundary -->
<!-- START ALPHA CHANGE 2282 -- document all default builder output directories -->
The installer creates or reuses the non-login `innbc:innbc` system account and assigns `/var/lib/innbc/snapshots`, `/var/lib/innbc/history-archive`, and `/var/lib/innbc/database-backups` to it.
<!-- FINISH ALPHA CHANGE 2282 -- document all default builder output directories -->

The operator command remains `sudo innbc-snapshot-builder <command>` so the root-owned configuration can be read. Before starting the Go process, the wrapper drops privileges and executes it as `innbc`. The source node directory must therefore already be owned by or accessible to `innbc`.

Custom output directories must also be writable by `innbc`; the installer creates only the default directories.
<!-- FINISH ALPHA CHANGE 1929 -- document the unprivileged production ownership boundary -->

It does not install a systemd service because snapshot creation is an explicit one-shot operation. It does not modify the node binary, genesis, node environment files, validator identities, source chain data, or existing published snapshots.

## Install

```bash
sudo /opt/innbc-snapshot-builder/scripts/install.sh
```

Review the active configuration:

```bash
sudo nano /etc/innbc/snapshot-builder.env
```

Default configuration:

```ini
INNBC_SNAPSHOT_BUILDER_CHAIN=/etc/innbc/genesis.json
INNBC_SNAPSHOT_BUILDER_SOURCE_DATA_DIR=/var/lib/innbc/node1
INNBC_SNAPSHOT_BUILDER_OUTPUT_ROOT=/var/lib/innbc/snapshots
INNBC_SNAPSHOT_BUILDER_RETENTION_BLOCKS=43200
INNBC_SNAPSHOT_BUILDER_SEGMENT_BLOCKS=1800
# START ALPHA CHANGE 3082 -- document independent full and recent destination allowances --
INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB=1024
INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB=128
# START ALPHA CHANGE 3509 -- document builder destination database pages --
INNBC_SNAPSHOT_BUILDER_MDBX_PAGE_SIZE_BYTES=4096
# FINISH ALPHA CHANGE 3509 -- document builder destination database pages --
# FINISH ALPHA CHANGE 3082 -- document independent full and recent destination allowances --
# START ALPHA CHANGE 2282 -- document the independent backup output setting --
INNBC_DATABASE_BACKUP_OUTPUT_ROOT=/var/lib/innbc/database-backups
# FINISH ALPHA CHANGE 2282 -- document the independent backup output setting --

# START ALPHA CHANGE 2091 -- configure archive and recovery operations --
INNBC_HISTORY_ARCHIVE_OUTPUT_ROOT=/var/lib/innbc/history-archive
INNBC_HISTORY_ARCHIVE_SEGMENT_BLOCKS=1800
INNBC_HISTORY_ARCHIVE_MAX_INDEX_RECORDS=2000000
INNBC_RECOVERY_CHECKPOINT_MINIMUM_RETENTION_BLOCKS=43200
# FINISH ALPHA CHANGE 2091 -- configure archive and recovery operations --
```

<!-- START ALPHA CHANGE 2091 -- explain upgrade and index-limit policy -->
<!-- START ALPHA CHANGE 3082 -- document explicit launch configuration for each builder operation -->
Review `/etc/innbc/snapshot-builder.env` before launch.

| Operation | MDBX setting | Configured allowance |
| --- | --- | --- |
| `segmented` | `INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB` | 128 GiB |
| `checkpoint` | `INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB` | 1024 GiB |
| `recovery-checkpoint` | `INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB` | 1024 GiB |
| `archive` | `INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB` | 1024 GiB per destination database |
| `backup` | `INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB` | 1024 GiB |

Segmented dispatch requires its explicit setting. Missing or empty configuration is rejected before invoking the binary; no fallback for an older environment is supplied.

Backup additionally requires `INNBC_DATABASE_BACKUP_OUTPUT_ROOT`. Segmented and checkpoint operations do not require the backup output setting.

The builder opens the full-history source read-only using stored database geometry. Its destination allowances do not require the source to use identical settings; the exported data must fit the chosen destination.

A configured allowance is not an upfront disk allocation.

<!-- START ALPHA CHANGE 3509 -- document explicit page selection and locked archive compatibility -->
Direct CLI commands do not load this environment file. Their shared map-size default is 1024 GiB; explicitly pass `--mdbx-map-size-gb 128` for a direct segmented build targeting recent nodes.

`INNBC_SNAPSHOT_BUILDER_MDBX_PAGE_SIZE_BYTES` selects destination database pages for all packaged builder operations. Its default is 4096 bytes. Missing settings and blank textual values select 4096; supported explicit values are 4096, 8192, 16384, 32768 and 65536. Invalid nonempty values are rejected. Direct commands use `--mdbx-page-size-bytes`.

Sources retain their stored geometry, so an operation can read a 4096-byte source and construct an 8192-byte destination. Existing archives and reused recovery artifacts must match the selected destination policy. Snapshot consumers must match the resulting snapshot's blockchain and trie page sizes.

Recovery-checkpoint construction rechecks archive compatibility after acquiring both archive and checkpoint publication locks, before archive selection or checkpoint staging. An incompatible archive published after preflight is rejected while existing publications remain intact and both locks are released.

The direct examples below explicitly use 4096-byte pages. For trie reconstruction, use the page size of the retained blockchain database, including when reconstructing a downloaded backup.
<!-- FINISH ALPHA CHANGE 3509 -- document explicit page selection and locked archive compatibility -->
<!-- FINISH ALPHA CHANGE 3082 -- document explicit launch configuration for each builder operation -->

The default index limit is 2,000,000 records per archive segment. The supported range is 1 through 4,000,000. This limits record count, not total process memory: slice allocation, decoded blocks, MDBX, and validation consume additional memory.

Archive segment size is fixed when an archive is created. Do not change it to work around a full segment in an existing archive. The record limit may be increased only up to the supported maximum.

Every archive manifest describes the complete archive. Do not remove earlier entries to work around the manifest-size limit.
<!-- FINISH ALPHA CHANGE 2091 -- explain upgrade and index-limit policy -->

<!-- START ALPHA CHANGE 2273 -- document standalone database backup and optional local trie reconstruction -->
## Create A Standalone Blockchain Database Backup

This operation creates a consistent archival copy of the blockchain database and preserves the supplied genesis configuration byte-for-byte. Normal source-node operation may continue while the copy is produced.

It does not copy the trie, consensus database or node keys. It does not create startup snapshot metadata, update a `latest.json` pointer, or automatically activate anything in a node.

### Prepare The Output Directory

<!-- START ALPHA CHANGE 3082 -- document backup directory preparation -->
The package installer creates `/var/lib/innbc/database-backups` with `innbc` ownership. The following command can also explicitly prepare that directory:
<!-- FINISH ALPHA CHANGE 3082 -- document backup directory preparation -->

```bash
sudo install -d -o innbc -g innbc -m 0750 \
  /var/lib/innbc/database-backups
```

The backup output must be outside the source node's data directory.

### Run The Backup

<!-- START ALPHA CHANGE 2282 -- document wrapper and direct backup execution -->
After configuring `/etc/innbc/snapshot-builder.env`, run:

```bash
sudo innbc-snapshot-builder backup
```

The wrapper reads:

- `INNBC_SNAPSHOT_BUILDER_CHAIN`;
- `INNBC_SNAPSHOT_BUILDER_SOURCE_DATA_DIR`;
- `INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB`;
- `INNBC_DATABASE_BACKUP_OUTPUT_ROOT`.

It drops privileges to `innbc` before executing the backup. Snapshot retention,
segment geometry and historical-archive settings are not required for this
operation.

Alternatively, invoke the shared binary directly:

```bash
sudo runuser --user innbc -- \
  /usr/local/bin/innbc-chain snapshot backup \
  --chain /etc/innbc/genesis.json \
  --data-dir /var/lib/innbc/node1 \
  --out /var/lib/innbc/database-backups \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3509 -- document direct backup storage geometry --
# FINISH ALPHA CHANGE 3509 -- document direct backup storage geometry --
```

This direct command does not read `/etc/innbc/snapshot-builder.env`. Its source,
genesis, output directory and destination MDBX map size come from the
command-line options.
<!-- FINISH ALPHA CHANGE 2282 -- document wrapper and direct backup execution -->

Each successful invocation creates a uniquely named directory:

```text
database-backups/
  database-backup-<height>-<UTC-timestamp>-<unique-suffix>/
    backup.json
    genesis.json
    chain-data/
      blockchain/
        mdbx.dat
```

`backup.json` records the captured head number, block hash, state root, chain identity, record count, record digest, and checksums of the preserved genesis file and completed database file.

Previous completed backups remain in place. The command does not implement automatic scheduling, compression, upload or retention cleanup.

### Consistency And Publication

The command copies every blockchain record through one fixed MDBX read transaction. Source writes committed afterward are not included in that captured view.

It writes the destination in bounded batches, closes and reopens it, and checks that its record count and length-framed SHA-256 digest match the captured source records. It also checks the head records, genesis header identity, canonical parent continuity, and readable block bodies and receipts through the captured head.

The genesis configuration is parsed from the same bytes preserved in the backup.

Only after copying and validation does the command publish the completed directory. It reuses the builder lock in the backup output root to serialize publication; that lock does not stop the source node.

These checks establish copy consistency and readable continuous history. They do not execute historical transactions, independently regenerate genesis allocations, or authenticate an untrusted source chain.

### Download And Store Offline

The recommended workflow is:

1. Create the backup on the server containing the full-history source.
2. Download the completed directory to a local machine.
3. Compare the downloaded database and genesis checksums with `backup.json`.
4. Preserve an unchanged archival copy offline.
5. If a trie is needed, prepare a separate working copy for reconstruction.

Do not configure a database backup as `INNBC_SNAPSHOT_SOURCE_DIR`. Nodes do not automatically consume these archival directories.

### Optional Trie Reconstruction

Trie reconstruction is a separate, optional operation. It can run on the server or on a local machine. Running it locally is recommended when practical to keep the historical replay workload off the production server.

For example, after copying the downloaded archive into a working directory at `/home/operator/innbc-backup-work`, use a compatible localtestclient binary:

```bash
./localtestclient/bin/innbc-chain node trie-rebuild \
  --chain /home/operator/innbc-backup-work/genesis.json \
  --data-dir /home/operator/innbc-backup-work \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3509 -- document reconstruction database geometry --
# FINISH ALPHA CHANGE 3509 -- document reconstruction database geometry --
```

Use the genesis configuration preserved in the backup, not the local test network's genesis.

The working directory must contain:

```text
innbc-backup-work/
  genesis.json
  chain-data/
    blockchain/
      mdbx.dat
```

Its `chain-data/trie` directory must be absent or completely empty. No running node or other maintenance process may use the target directory during reconstruction.

The existing trie-rebuild requirements apply: complete unified history from genesis, the original genesis configuration, and allocation-based genesis initialization. A nonzero INNBCBFT `InitialTrieRoot` is not supported by the reconstruction command.

Replay checks execution commitments and produces a replacement trie. It does not supply node keys or turn the archival backup into a complete validator installation.

### Operational Considerations

Do not delete, replace, restore or prune the source database during backup creation. Normal source writes are supported; concurrent source maintenance is not.

The fixed MDBX read transaction can retain older database pages while the source advances. Allow sufficient disk space for source growth and the destination copy. Copying, verification and checksumming also consume I/O.

A forcibly interrupted process may leave a hidden `.database-backup-building-*` directory. Such a directory is unfinished staging, not a completed backup. Review it only after confirming the corresponding backup process has stopped.

The command produces a local archive. A successful command does not establish that a subsequent download, upload or offline-storage copy succeeded.
<!-- FINISH ALPHA CHANGE 2273 -- document standalone database backup and optional local trie reconstruction -->

## Build a Segmented Snapshot

```bash
sudo innbc-snapshot-builder segmented
```

This publishes the recent-history artifact under:

```text
/var/lib/innbc/snapshots/segmented/
```

## Build a Checkpoint Snapshot

```bash
sudo innbc-snapshot-builder checkpoint
```

This publishes the full-node Plan B artifact under:

```text
/var/lib/innbc/snapshots/checkpoint/
```

<!-- START ALPHA CHANGE 2091 -- document archive continuity recovery and operational limits -->
## Build Or Extend The Historical Archive

```bash
sudo innbc-snapshot-builder archive
```

The archive is published under `/var/lib/innbc/history-archive` by default. It contains immutable MDBX block segments, segment-local transaction-hash indexes, immutable manifests, and an archive `latest.json`.

It preserves the transaction bodies, receipts, receipt logs, contract-creation receipt data, and containing-block information needed by a future historical transaction reader. This package does not implement that reader or add archive-backed JSON-RPC endpoints.

It does not preserve historical state tries, historical balances, historical contract calls, historical storage queries, or complete address-activity indexes. Complete historical log searching is also outside this feature.

Archive data is not bootable node data. Never configure an archive directory through `INNBC_SNAPSHOT_SOURCE_DIR`.

Only complete configured block ranges are published. When no new complete range exists, published segments, manifests, and the archive pointer remain unchanged. Acquiring a builder lock and opening MDBX may create or update runtime lock files; those are not immutable archive payloads.

A fresh archive needs block history beginning at block 1 and the retained genesis header. An existing archive can continue from a checkpoint-restored full-mode MDBX source.

If the archive currently ends at P, the next required block is P+1. The source history floor must not exceed P+1. If P remains available, its hash must match the archive. If the source begins at P+1, that block's parent must match the archived hash of P.

A segmented recent-history source is not supported by this full-mode source-opening path.

## Build A Linked Recovery Checkpoint

```bash
sudo innbc-snapshot-builder archive
sudo innbc-snapshot-builder recovery-checkpoint
```

The second command requires an existing valid archive. It does not extend that archive.

The builder exports consensus state to select checkpoint C. It chooses the newest completed archive-segment boundary A satisfying the configured minimum retention, then physically copies checkpoint history A through C.

The resulting coverage is:

```text
archive:             blocks 1 ... archive watermark
checkpoint history:  blocks A ... C
checkpoint state:    state at C
```

Coverage overlaps by at least block A. The archive watermark may extend beyond A or C.

The builder reads the actual archived header at A and compares it with the actual checkpoint header. When A is earlier than C, it also checks the parent connection from A+1.

The checkpoint contains `recovery-set.json`, which pins the exact immutable archive manifest used. The ordinary checkpoint `latest.json` is its only publication pointer. There is no separate recovery-set output root.

Recovery verification checks complete payload checksums through A and decodes the corresponding index files. It is I/O intensive for a mature archive and holds the archive writer lock throughout.

Recovery does not alter published archive payloads, manifests, or pointers. Builder locking and the MDBX boundary read can create runtime lock files.

## Retry And Preservation

A completed recovery checkpoint remains available when publishing its latest pointer fails. An exact same-height retry validates and reuses the existing artifact. It uses the archive manifest pinned by that artifact, even if archive `latest.json` has advanced.

The retry still needs an appropriate source database and the same exported consensus checkpoint height. If the live source advances, the command may select a new checkpoint instead of repairing the earlier one.

A same-height ordinary checkpoint or a recovery artifact with conflicting identity or retention is rejected rather than overwritten.

Ordinary snapshot cleanup preserves a previous checkpoint containing `recovery-set.json`. Malformed or unreadable recovery metadata also causes preservation and an error log. These retained checkpoints can therefore consume storage until deliberately reviewed by the operator.

## Required Plan B Order

1. Retain the original source database.
2. Build or extend the archive.
3. Build and validate the recovery checkpoint.
4. Verify the recovery relationship and the required archive coverage.
5. Copy the archive to its intended durable storage and independently verify the copied files.
6. Ensure blocks produced after C remain recoverable from another trusted source, or stop production at C before cutover.
7. Only after those checks, perform the planned checkpoint activation and original-history removal.

A successful local build does not prove that a remote upload succeeded.

Do not delete, replace, prune, or activate another database over the source while archive or recovery construction is running. Builder locks coordinate cooperating builders, not the live node's database replacement or pruning operations.

For remote archive publication, upload all referenced segment files first, then the immutable manifest, and publish `latest.json` last. Existing immutable objects must remain available to readers using older manifests.

Checksums establish consistency under the trusted internal-builder model. They do not independently authenticate artifacts from an untrusted source.

## Execution Model

Ordinary snapshot construction:

```text
acquire snapshot-kind writer lock
→ export consensus state and select checkpoint C
→ open source blockchain and trie
→ copy retained blockchain history and state at C
→ validate the artifact
→ publish the directory and ordinary latest pointer
→ clean eligible superseded snapshots
→ exit
```

Archive construction:

```text
acquire archive writer lock
→ capture a fixed source head number and hash
→ validate archive continuity
→ export and validate new complete segments
→ publish immutable segments and manifest
→ publish archive latest.json last
→ exit
```

Recovery-checkpoint construction:

```text
acquire archive and checkpoint writer locks
→ export consensus state and select checkpoint C
→ load the current or pinned archive manifest
→ select and verify shared boundary A
→ build and validate checkpoint history A ... C and state at C
→ verify the actual boundary headers
→ write recovery-set.json inside staging
→ publish the completed checkpoint
→ publish ordinary checkpoint latest.json last
→ exit
```

The validator may remain online during creation, subject to the source-consistency restrictions above. Cutover must separately account for blocks produced after the exported checkpoint.

<!-- START ALPHA CHANGE 2110 -- document complete executable-state and consensus-file validation -->
## Snapshot Integrity Format

Bootable segmented and checkpoint snapshots use snapshot format version 2. Regenerate older bootable snapshots; the reader does not accept format version 1.

Before publication and again during staged activation, validation requires:

- the retained checkpoint header to match the manifest's number, hash, and state root;
- every reachable account-trie and contract-storage node to exist and match its stored hash reference;
- account and storage values to pass strict encoding checks;
- every referenced contract bytecode object to exist and match its code hash;
- the exported consensus MDBX data file to match its committed size and SHA-256;
- the proposer checkpoint height to match the blockchain checkpoint.

The consensus commitment excludes runtime lock files. It detects later corruption or substitution of the exported database; it does not independently prove the semantic correctness or provenance of its records.

Complete-state validation runs once on the closed, reopened staged artifact during fresh construction and independently during activation. It does not add work to ordinary block execution.

An exact recovery retry validates the previously published artifact using that artifact's own commitments and pinned archive manifest. It does not require a newly exported MDBX database to reproduce the same physical file checksum.

The historical transaction archive format is separate and is not changed by this bootable-snapshot format update.
<!-- FINISH ALPHA CHANGE 2110 -- document complete executable-state and consensus-file validation -->

## Validation Scope

The storage integration tests construct synthetic blockchain and consensus records. They verify archive preservation, transaction-to-receipt lookup, retained history, boundary checks, retries, and publication behavior. They do not demonstrate production validator boot or consensus execution.

The pointer-failure tests cause an actual filesystem error while opening the temporary latest-pointer file. They do not simulate power loss or injected directory-sync failures.

<!-- START ALPHA CHANGE 3398 -- distinguish implemented synchronization-boundary tests from host crash testing -->
Ordinary checkpoint and segmented snapshot regressions now verify synchronization-before-publication ordering and inject a failure at the staging-synchronization boundary. They check preservation of the previous publication and cleanup of unsuccessful staging.

These tests do not simulate host power loss or individual kernel/filesystem synchronization failures. Before destructive production cutover, validate startup and synchronization using the intended genesis and real chain data. Actual crash durability on the deployed filesystem remains a separate verification task.
<!-- FINISH ALPHA CHANGE 3398 -- distinguish implemented synchronization-boundary tests from host crash testing -->
<!-- FINISH ALPHA CHANGE 2091 -- document archive continuity recovery and operational limits -->
<!-- FINISH ALPHA CHANGE 1849 -- document standalone snapshot builder package -->