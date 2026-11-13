#!/usr/bin/env bash
# START ALPHA CHANGE 767 -- build local INNBC nodeclient test workspace --
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/localtestclient"
PACKAGING_DIR="${ROOT_DIR}/packaging/nodeclient"

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required to build the local INNBC nodeclient test workspace." >&2
  exit 1
fi

if [ ! -f "${ROOT_DIR}/genesis.json" ]; then
  echo "Missing ${ROOT_DIR}/genesis.json. Generate the canonical genesis before building the local test workspace." >&2
  exit 1
fi

mkdir -p \
  "${OUT_DIR}/bin" \
  "${OUT_DIR}/config" \
  "${OUT_DIR}/nodes"

VERSION="${VERSION:-localtest}"
COMMIT_HASH="${COMMIT_HASH:-none}"
BRANCH="${BRANCH:-manual}"
BUILD_TIME="$(date)"

go build -buildvcs=false -o "${OUT_DIR}/bin/innbc-chain" -ldflags="\
  -X 'github.com/innovativebioresearch/innbc-chain/versioning.Version=${VERSION}' \
  -X 'github.com/innovativebioresearch/innbc-chain/versioning.Commit=${COMMIT_HASH}' \
  -X 'github.com/innovativebioresearch/innbc-chain/versioning.Branch=${BRANCH}' \
  -X 'github.com/innovativebioresearch/innbc-chain/versioning.BuildTime=${BUILD_TIME}'" \
  "${ROOT_DIR}/main.go"

cp "${ROOT_DIR}/genesis.json" "${OUT_DIR}/genesis.json"
cp "${PACKAGING_DIR}/README-node-setup.md" "${OUT_DIR}/README-node-setup.md"

# START ALPHA CHANGE 819 -- generate localtest config examples with local genesis paths --
cat > "${OUT_DIR}/config/node.private-local.env.example" <<'ENV'
# START ALPHA CHANGE 751 -- add private-local validator node profile example --
# START ALPHA CHANGE 3377 -- generate inspection guidance in the local private environment example --
# Reference template; inspection reads actual nodeN.env files.
# After provisioning, run from the repository root:
# ./localtestclient/bin/innbc-chain node info all --base-dir localtestclient/nodes --config-dir localtestclient/config
# Inspection is read-only, reports incomplete slots and prints public values only.
# FINISH ALPHA CHANGE 3377 -- generate inspection guidance in the local private environment example --
# Persistent blockchain and trie data are stored with MDBX by the node binary.
INNBC_CHAIN=localtestclient/genesis.json
INNBC_DATA_DIR=localtestclient/nodes/node1
INNBC_JSONRPC_ADDR=127.0.0.1:8545
INNBC_GRPC_ADDR=127.0.0.1:9632
INNBC_LIBP2P_ADDR=127.0.0.1:30301
# START ALPHA CHANGE 1216 -- generate local private optional advertised peer addresses --
INNBC_NAT_ADDR=
INNBC_DNS_ADDR=
# FINISH ALPHA CHANGE 1216 -- generate local private optional advertised peer addresses --
# START ALPHA CHANGE 2275 -- generate local private RPC query and admission limits --
# START ALPHA CHANGE 2491 -- explain local private log-range defaults --
# Maximum toBlock - fromBlock for historical event logs.
# Zero selects 256; positive values override it. Separate from fee history.
# FINISH ALPHA CHANGE 2491 -- explain local private log-range defaults --
INNBC_JSONRPC_BLOCK_RANGE_LIMIT=256
# START ALPHA CHANGE 2466 -- expose full-history fee-history budgets in local examples --
# Zero selects code defaults. Percentile limit: 1-100.
# Result units: served blocks * max(1, requested percentiles).
INNBC_JSONRPC_FEE_HISTORY_BLOCK_LIMIT=256
INNBC_JSONRPC_FEE_HISTORY_PERCENTILE_LIMIT=10
INNBC_JSONRPC_FEE_HISTORY_RESULT_LIMIT=2560
# Recent-node environments omit these three overrides.
# FINISH ALPHA CHANGE 2466 -- expose full-history fee-history budgets in local examples --
INNBC_JSONRPC_REQUEST_RATE=100
INNBC_JSONRPC_REQUEST_BURST=200
INNBC_JSONRPC_CALL_RATE=200
INNBC_JSONRPC_CALL_BURST=400
INNBC_JSONRPC_MAX_CONCURRENT=32
INNBC_JSONRPC_MAX_EXPENSIVE=4
INNBC_JSONRPC_MAX_BODY_BYTES=1048576
INNBC_JSONRPC_MAX_WEBSOCKETS=64
INNBC_JSONRPC_MAX_FILTERS=1024
# FINISH ALPHA CHANGE 2275 -- generate local private RPC query and admission limits --
INNBC_PEER_DIRECTORY_ENABLED=true
INNBC_PEER_DIRECTORY_URL=http://127.0.0.1:8080/api/v1/peers/exchange
# START ALPHA CHANGE 854 -- generate local private-local history mode env defaults --
INNBC_HISTORY_MODE=full
# START ALPHA CHANGE 1760 -- generate local checkpoint snapshot consumer settings --
INNBC_SNAPSHOT_SOURCE_DIR=localtestclient/snapshots/checkpoint
INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED=false
# START ALPHA CHANGE 3069 -- generate full-history MDBX allowances of one TiB --
INNBC_MDBX_MAP_SIZE_GB=1024
# FINISH ALPHA CHANGE 3069 -- generate full-history MDBX allowances of one TiB --
# START ALPHA CHANGE 3483 -- generate private local database page defaults --
INNBC_MDBX_PAGE_SIZE_BYTES=4096
# FINISH ALPHA CHANGE 3483 -- generate private local database page defaults --
# FINISH ALPHA CHANGE 1760 -- generate local checkpoint snapshot consumer settings --
# FINISH ALPHA CHANGE 854 -- generate local private-local history mode env defaults --
# FINISH ALPHA CHANGE 751 -- add private-local validator node profile example --
ENV

cat > "${OUT_DIR}/config/node.external-validator.env.example" <<'ENV'
# START ALPHA CHANGE 752 -- add external validator node profile example --
# START ALPHA CHANGE 3377 -- generate inspection guidance in the local external environment example --
# Reference template; inspection reads actual nodeN.env files.
# After provisioning, run from the repository root:
# ./localtestclient/bin/innbc-chain node info all --base-dir localtestclient/nodes --config-dir localtestclient/config
# Inspection is read-only, reports incomplete slots and prints public values only.
# FINISH ALPHA CHANGE 3377 -- generate inspection guidance in the local external environment example --
# Persistent blockchain and trie data are stored with MDBX by the node binary.
INNBC_CHAIN=localtestclient/genesis.json
INNBC_DATA_DIR=localtestclient/nodes/node1
INNBC_JSONRPC_ADDR=127.0.0.1:8545
INNBC_GRPC_ADDR=127.0.0.1:9632
INNBC_LIBP2P_ADDR=0.0.0.0:30301
# START ALPHA CHANGE 1217 -- generate local external advertised peer address placeholders --
INNBC_NAT_ADDR=
INNBC_DNS_ADDR=
# FINISH ALPHA CHANGE 1217 -- generate local external advertised peer address placeholders --
# START ALPHA CHANGE 2275 -- generate local external RPC query and admission limits --
# START ALPHA CHANGE 2492 -- explain local external log-range defaults --
# Maximum toBlock - fromBlock for historical event logs.
# Zero selects 256; positive values override it. Separate from fee history.
# FINISH ALPHA CHANGE 2492 -- explain local external log-range defaults --
INNBC_JSONRPC_BLOCK_RANGE_LIMIT=256
# START ALPHA CHANGE 2466 -- expose external full-history fee-history budgets in local examples --
# Zero selects code defaults. Percentile limit: 1-100.
# Result units: served blocks * max(1, requested percentiles).
INNBC_JSONRPC_FEE_HISTORY_BLOCK_LIMIT=256
INNBC_JSONRPC_FEE_HISTORY_PERCENTILE_LIMIT=10
INNBC_JSONRPC_FEE_HISTORY_RESULT_LIMIT=2560
# Recent-node environments omit these three overrides.
# FINISH ALPHA CHANGE 2466 -- expose external full-history fee-history budgets in local examples --
INNBC_JSONRPC_REQUEST_RATE=100
INNBC_JSONRPC_REQUEST_BURST=200
INNBC_JSONRPC_CALL_RATE=200
INNBC_JSONRPC_CALL_BURST=400
INNBC_JSONRPC_MAX_CONCURRENT=32
INNBC_JSONRPC_MAX_EXPENSIVE=4
INNBC_JSONRPC_MAX_BODY_BYTES=1048576
INNBC_JSONRPC_MAX_WEBSOCKETS=64
INNBC_JSONRPC_MAX_FILTERS=1024
# FINISH ALPHA CHANGE 2275 -- generate local external RPC query and admission limits --
INNBC_PEER_DIRECTORY_ENABLED=true
INNBC_PEER_DIRECTORY_URL=https://innbcchain.io/api/v1/peers/exchange
# START ALPHA CHANGE 855 -- generate local external-validator history mode env defaults --
INNBC_HISTORY_MODE=full
# START ALPHA CHANGE 1761 -- generate local external checkpoint snapshot consumer settings --
INNBC_SNAPSHOT_SOURCE_DIR=localtestclient/snapshots/checkpoint
INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED=false
# START ALPHA CHANGE 3069 -- generate external full-history MDBX allowances of one TiB --
INNBC_MDBX_MAP_SIZE_GB=1024
# FINISH ALPHA CHANGE 3069 -- generate external full-history MDBX allowances of one TiB --
# START ALPHA CHANGE 3483 -- generate external local database page defaults --
INNBC_MDBX_PAGE_SIZE_BYTES=4096
# FINISH ALPHA CHANGE 3483 -- generate external local database page defaults --
# FINISH ALPHA CHANGE 1761 -- generate local external checkpoint snapshot consumer settings --
# FINISH ALPHA CHANGE 855 -- generate local external-validator history mode env defaults --
# FINISH ALPHA CHANGE 752 -- add external validator node profile example --
ENV

# START ALPHA CHANGE 1772 -- remove unused generated environment profile --
# FINISH ALPHA CHANGE 1772 -- remove unused generated environment profile --

cat > "${OUT_DIR}/config/node.example.yaml" <<'YAML'
# START ALPHA CHANGE 748 -- add example INNBC node environment config values --
# START ALPHA CHANGE 3377 -- distinguish local profile examples from inspected environment files --
# Profile reference only; node info inspects actual nodeN.env files, not this YAML.
# After provisioning, run from the repository root:
# ./localtestclient/bin/innbc-chain node info all --base-dir localtestclient/nodes --config-dir localtestclient/config
# Inspection is read-only and does not repair incomplete identities or environments.
# FINISH ALPHA CHANGE 3377 -- distinguish local profile examples from inspected environment files --
# Persistent blockchain and trie data are stored with MDBX by the node binary.
private_local:
  chain: localtestclient/genesis.json
  data_dir: localtestclient/nodes/node1
  jsonrpc: 127.0.0.1:8545
  libp2p: 127.0.0.1:30301
  # START ALPHA CHANGE 1218 -- document local private optional advertised peer addresses --
  nat: ""
  dns: ""
  # FINISH ALPHA CHANGE 1218 -- document local private optional advertised peer addresses --
  peer_directory_enabled: true
  peer_directory_url: http://127.0.0.1:8080/api/v1/peers/exchange
  # START ALPHA CHANGE 842 -- document full local history defaults for private-local profile --
  history_mode: full
  # START ALPHA CHANGE 1763 -- generate private-local checkpoint snapshot settings --
  snapshot_source_dir: localtestclient/snapshots/checkpoint
  snapshot_bootstrap_required: false
  # START ALPHA CHANGE 3069 -- generate private full-history MDBX allowances of one TiB --
  mdbx_map_size_gb: 1024
  # FINISH ALPHA CHANGE 3069 -- generate private full-history MDBX allowances of one TiB --
  # START ALPHA CHANGE 3510 -- generate private profile database pages --
  mdbx_page_size_bytes: 4096
  # FINISH ALPHA CHANGE 3510 -- generate private profile database pages --
  # FINISH ALPHA CHANGE 1763 -- generate private-local checkpoint snapshot settings --
  # FINISH ALPHA CHANGE 842 -- document full local history defaults for private-local profile --

external_validator:
  chain: localtestclient/genesis.json
  data_dir: localtestclient/nodes/node1
  jsonrpc: 127.0.0.1:8545
  libp2p: 0.0.0.0:30301
  # START ALPHA CHANGE 1219 -- document local external advertised peer address fields --
  nat: ""
  dns: ""
  # FINISH ALPHA CHANGE 1219 -- document local external advertised peer address fields --
  peer_directory_enabled: true
  peer_directory_url: https://innbcchain.io/api/v1/peers/exchange
  # START ALPHA CHANGE 843 -- document full local history defaults for external-validator profile --
  history_mode: full
  # START ALPHA CHANGE 1820 -- generate unified external-validator MDBX and checkpoint settings --
  snapshot_source_dir: localtestclient/snapshots/checkpoint
  snapshot_bootstrap_required: false
  # START ALPHA CHANGE 3069 -- generate external full-history MDBX allowances of one TiB --
  mdbx_map_size_gb: 1024
  # FINISH ALPHA CHANGE 3069 -- generate external full-history MDBX allowances of one TiB --
  # START ALPHA CHANGE 3510 -- generate external profile database pages --
  mdbx_page_size_bytes: 4096
  # FINISH ALPHA CHANGE 3510 -- generate external profile database pages --
  # FINISH ALPHA CHANGE 1820 -- generate unified external-validator MDBX and checkpoint settings --
  # FINISH ALPHA CHANGE 843 -- document full local history defaults for external-validator profile --

# START ALPHA CHANGE 1773 -- remove unused generated YAML profile --
# FINISH ALPHA CHANGE 1773 -- remove unused generated YAML profile --
# FINISH ALPHA CHANGE 748 -- add example INNBC node environment config values --
YAML
# FINISH ALPHA CHANGE 819 -- generate localtest config examples with local genesis paths --

cat > "${OUT_DIR}/README-localtestclient.md" <<'README'
# INNBC Local Testclient

This folder is a local-only test workspace. It is not installed into the system and it does not touch /usr/local/bin, /etc/innbc, /var/lib/innbc, or dist/innbc-nodeclient.

Create fresh local node identities:

# START ALPHA CHANGE 1971 -- generate local env files with the shared snapshot root --
./localtestclient/bin/innbc-chain node create 3 --base-dir localtestclient/nodes --config-dir localtestclient/config --chain localtestclient/genesis.json --snapshot-root localtestclient/snapshots --mode private-local
# FINISH ALPHA CHANGE 1971 -- generate local env files with the shared snapshot root --

Prepare empty local node folders for manually pasted keys:

# START ALPHA CHANGE 1972 -- prepare local env files with the shared snapshot root --
./localtestclient/bin/innbc-chain node prepare 3 --base-dir localtestclient/nodes --config-dir localtestclient/config --chain localtestclient/genesis.json --snapshot-root localtestclient/snapshots --mode private-local
# FINISH ALPHA CHANGE 1972 -- prepare local env files with the shared snapshot root --

After preparing node1, place existing genesis validator keys at:

# START ALPHA CHANGE 1638 -- document isolated localtest identity paths --
localtestclient/nodes/node1/keys/validator-ecdsa/validator.key
localtestclient/nodes/node1/keys/validator-bls/validator-bls.key
localtestclient/nodes/node1/keys/libp2p/libp2p.key
# FINISH ALPHA CHANGE 1638 -- document isolated localtest identity paths --

<!-- START ALPHA CHANGE 3377 -- generate read-only local provisioning inspection instructions -->

## Validate Stored Identities and Environments

Creation accepts up to ten new identities per invocation and allocates available numbers within node1–node100. A later creation failure can leave completed nodes and an incomplete current slot.

After creation, or after installing all keys into prepared folders, inspect one node from the repository root:

```bash
./localtestclient/bin/innbc-chain node info node1 \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config
```

Inspect all discovered slots:

```bash
./localtestclient/bin/innbc-chain node info all \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config
```

Add `--json` for structured output.

Inspection validates keys and independently checks recognized public environment settings. It does not initialize storage, modify files, repair identities or print private keys.

Missing interior slots produce warnings. Invalid occupied slots produce a nonzero exit after reporting, while later inspected nodes remain visible. Discovery does not infer a missing trailing node for which no path remains.

The public registration values include blsKey, the operator-bound blsProof and libp2pPeerID. The operator address identifies the wallet that must call registerValidator. rewardAddress and stakeAmount must be chosen separately.

Inspection checks stored provisioning data, not full runtime readiness or agreement with an independently trusted genesis or registry identity.

<!-- FINISH ALPHA CHANGE 3377 -- generate read-only local provisioning inspection instructions -->

<!-- START ALPHA CHANGE 1852 -- document direct local peer directory startup -->
Run the authenticated peer-directory process in its own terminal:

./localtestclient/bin/innbc-chain peerdirectory \
  --listen 127.0.0.1:8080 \
  --jsonrpc http://127.0.0.1:8545 \
  --validator-registry 0x0000000000000000000000000000000000000107 \
  --chain localtestclient/genesis.json \
  --cache-file localtestclient/peer-directory/peers.json
<!-- FINISH ALPHA CHANGE 1852 -- document direct local peer directory startup -->

<!-- START ALPHA CHANGE 1821 -- document independent local snapshot building and activation -->

<!-- START ALPHA CHANGE 2466 -- document local fee-history override behavior -->
Run node1 locally as a full-history node.

The code defaults are 256 fee-history blocks, 10 requested percentiles,
and 2560 result units. This foreground command does not load environment
files. To override the defaults, supply:
--json-rpc-fee-history-block-limit,
--json-rpc-fee-history-percentile-limit, and
--json-rpc-fee-history-result-limit.

For example, 512 blocks with up to 100 percentiles requires values
512, 100, and 51200 respectively. Zero selects the corresponding default.
<!-- FINISH ALPHA CHANGE 2466 -- document local fee-history override behavior -->

./localtestclient/bin/innbc-chain server \
  --chain localtestclient/genesis.json \
  --data-dir localtestclient/nodes/node1 \
  --grpc-address 127.0.0.1:9632 \
  --jsonrpc 127.0.0.1:8545 \
  --json-rpc-block-range-limit 256 \
  --json-rpc-fee-history-block-limit 256 \
  --json-rpc-fee-history-percentile-limit 10 \
  --json-rpc-fee-history-result-limit 2560 \
  --libp2p 127.0.0.1:30301 \
  --nat="" \
  --dns="" \
  --peer-directory-enabled=true \
  --peer-directory-url http://127.0.0.1:8080/api/v1/peers/exchange \
  --history-mode full \
  --snapshot-source-dir localtestclient/snapshots/checkpoint \
  --snapshot-bootstrap-required=false \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3510 -- generate explicit full-history database pages --
# FINISH ALPHA CHANGE 3510 -- generate explicit full-history database pages --

<!-- START ALPHA CHANGE 1853 -- document normal local recent-history node startup -->
Run node2 in another terminal as a recent-history validator with optional snapshot bootstrap:

/* START ALPHA CHANGE 2499 -- inherit historical query defaults for local recent-node startup --

Event-log range and fee-history overrides are omitted so this recent-history
node uses the code defaults.

FINISH ALPHA CHANGE 2499 -- inherit historical query defaults for local recent-node startup -- */

./localtestclient/bin/innbc-chain server \
  --chain localtestclient/genesis.json \
  --data-dir localtestclient/nodes/node2 \
  --grpc-address 127.0.0.1:9633 \
  --jsonrpc 127.0.0.1:8546 \
  --libp2p 127.0.0.1:30302 \
  --nat="" \
  --dns="" \
  --peer-directory-enabled=true \
  --peer-directory-url http://127.0.0.1:8080/api/v1/peers/exchange \
  --history-mode recent \
  --history-retention-blocks 43200 \
  --history-segment-blocks 1800 \
  --snapshot-source-dir localtestclient/snapshots/segmented \
  --snapshot-bootstrap-required=false \
  --mdbx-map-size-gb 128 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3510 -- generate explicit recent database pages --
# FINISH ALPHA CHANGE 3510 -- generate explicit recent database pages --

<!-- START ALPHA CHANGE 3510 -- generate optional-bootstrap page compatibility guidance -->
With `--snapshot-bootstrap-required=false`, node2 normally uses a valid segmented snapshot when available and otherwise falls back to normal block synchronization. A selected snapshot with incompatible database page sizes stops startup even in optional mode. Incompatible existing local databases also stop startup.

The commands explicitly select 4096-byte pages. Direct commands do not load the generated node environment files. When using another supported page size, update the explicit flags and ensure snapshot consumers match their snapshots. Changing a flag does not convert existing databases.
<!-- FINISH ALPHA CHANGE 3510 -- generate optional-bootstrap page compatibility guidance -->
<!-- FINISH ALPHA CHANGE 1853 -- document normal local recent-history node startup -->

<!-- START ALPHA CHANGE 2207 -- use production history defaults and document accelerated rotation -->
### History Retention And Optional Accelerated Rotation

The local examples use the production history defaults:

- retention: 43,200 blocks;
- segment size: 1,800 blocks;
- retained non-genesis segments: 24.

At a two-second block interval, each complete segment represents approximately one hour. Rotation retires the oldest complete segment when a new segment opens. The remaining 23 complete segments plus the active segment contain between 41,401 and 43,200 blocks, approximately 23–24 hours of history.

These are block-count settings, not wall-clock timers. Different block intervals change the corresponding elapsed time.

For explicitly accelerated local rotation testing, replace the two history flags with:

```bash
  --history-retention-blocks 1080 \
  --history-segment-blocks 30 \
```

This optional configuration creates a new segment approximately every minute at a two-second block interval. Once the retention window fills, it retains between 1,051 and 1,080 blocks. Old-segment retirement begins only after the retained segment count is exceeded.

This is a conservative testing recommendation, not the absolute minimum accepted by the software. The standard examples intentionally retain production values.

<!-- START ALPHA CHANGE 2217 -- document an unsafe retention example without referring to removed instructions -->
Avoid settings such as retention 120 blocks with 30-block segments for general contract-execution testing. They retain only 91 blocks immediately after rotation, while the EVM permits contracts to request hashes from the preceding 256 blocks. Missing ancestry can change execution results once cached headers are unavailable, including after restart.
<!-- FINISH ALPHA CHANGE 2217 -- document an unsafe retention example without referring to removed instructions -->

The requirement applies to validators even when application RPC is served exclusively by a full-history node. The JSON-RPC block-range and fee-history limits do not configure the EVM's `BLOCKHASH` window.

With the current whole-segment pruning policy, the minimum retained non-genesis history after rotation is:

```text
retention blocks - segment blocks + 1
```

For custom settings, use a conservative minimum of at least 1,024 retained blocks under this formula. Retention must also be divisible by segment size. This recommendation is documented here; the current startup checks do not enforce this safety floor.

If testing snapshot bootstrap with accelerated settings, build a matching segmented snapshot using `--retention-blocks 1080 --segment-blocks 30` and use the same geometry in the receiving recent-history node. Changing retention settings cannot restore ancestry omitted from an existing snapshot.
<!-- FINISH ALPHA CHANGE 2207 -- use production history defaults and document accelerated rotation -->

<!-- START ALPHA CHANGE 1859 -- remove redundant recent-history node1 example -->
<!-- FINISH ALPHA CHANGE 1859 -- remove redundant recent-history node1 example -->

Snapshot production is an independent, on-demand command. Build a segmented snapshot from node1 while node1 is running in full-history mode:

<!-- START ALPHA CHANGE 2280 -- keep the complete local snapshot command copyable -->
```bash
./localtestclient/bin/innbc-chain snapshot build segmented \
  --chain localtestclient/genesis.json \
  --source-data-dir localtestclient/nodes/node1 \
  --output-root localtestclient/snapshots \
  --retention-blocks 43200 \
  --segment-blocks 1800 \
  --mdbx-map-size-gb 128 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3510 -- generate segmented snapshot destination pages --
# FINISH ALPHA CHANGE 3510 -- generate segmented snapshot destination pages --
```
<!-- FINISH ALPHA CHANGE 2280 -- keep the complete local snapshot command copyable -->

The segmented snapshot is published under:

localtestclient/snapshots/segmented/latest.json
localtestclient/snapshots/segmented/block-<block-number>-<hash-prefix>/manifest.json
localtestclient/snapshots/segmented/block-<block-number>-<hash-prefix>/chain-data

A checkpoint snapshot can be built separately:

# START ALPHA CHANGE 3085 -- generate the full-mode checkpoint allowance --
./localtestclient/bin/innbc-chain snapshot build checkpoint \
  --chain localtestclient/genesis.json \
  --source-data-dir localtestclient/nodes/node1 \
  --output-root localtestclient/snapshots \
  --retention-blocks 43200 \
  --segment-blocks 1800 \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3510 -- generate checkpoint destination pages --
# FINISH ALPHA CHANGE 3510 -- generate checkpoint destination pages --
# FINISH ALPHA CHANGE 3085 -- generate the full-mode checkpoint allowance --

The checkpoint snapshot is published under localtestclient/snapshots/checkpoint.

<!-- START ALPHA CHANGE 2096 -- document local archive creation and linked recovery checkpoints -->
## Historical Archive And Linked Recovery Checkpoint

Run the following commands from the repository root. They use explicit local paths and do not require the production snapshot-builder wrapper or its environment file.

The local binary must contain the `snapshot archive` and `snapshot recovery-checkpoint` commands. The source node must use full-mode MDBX storage.

### Create Or Extend The Historical Archive

```bash
./localtestclient/bin/innbc-chain snapshot archive \
  --chain localtestclient/genesis.json \
  --source-data-dir localtestclient/nodes/node1 \
  --output-root localtestclient/history-archive \
  --segment-blocks 1800 \
  --max-index-records 2000000 \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3510 -- generate archive destination geometry --
# FINISH ALPHA CHANGE 3510 -- generate archive destination geometry --
```

The builder creates the output directories as needed:

```text
localtestclient/history-archive/
├── latest.json
├── manifest-<sha256>.json
└── segments/
    └── blocks-<from>-<to>-<hash>/
        ├── blockchain/
        │   └── mdbx.dat
        ├── transactions.idx
        └── segment.json
```

Each segment preserves block headers, transaction bodies, receipts, receipt logs, and transaction lookups. Its sorted index maps transaction hashes to block numbers and transaction positions.

<!-- START ALPHA CHANGE 2208 -- document production archive segment size -->
Only complete 1,800-block ranges are published. Repeat the same command to archive newly completed ranges. Keep the segment size unchanged for an existing archive; this revised example does not change the geometry of an archive already created with 30-block segments.
<!-- FINISH ALPHA CHANGE 2208 -- document production archive segment size -->

The archive does not contain state tries, consensus state, validator keys, or node configuration. It is not a bootable snapshot and must never be passed to `--snapshot-source-dir`.

### Create A Checkpoint Linked To The Archive

After creating or extending the archive:

```bash
./localtestclient/bin/innbc-chain snapshot recovery-checkpoint \
  --chain localtestclient/genesis.json \
  --source-data-dir localtestclient/nodes/node1 \
  --snapshot-output-root localtestclient/snapshots \
  --archive-output-root localtestclient/history-archive \
  --minimum-retention-blocks 43200 \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3510 -- generate linked recovery destination geometry --
# FINISH ALPHA CHANGE 3510 -- generate linked recovery destination geometry --
```

<!-- START ALPHA CHANGE 3510 -- generate locked recovery compatibility guidance -->
This command reads the existing archive; it does not extend it. Existing archive databases must match the selected destination page size. Recovery construction rechecks compatibility after acquiring both publication locks, before archive selection or checkpoint staging. Compatibility rejection preserves existing publications and releases both locks.

It publishes:
<!-- FINISH ALPHA CHANGE 3510 -- generate locked recovery compatibility guidance -->

```text
localtestclient/snapshots/checkpoint/
├── latest.json
└── block-<number>-<hash-prefix>/
    ├── manifest.json
    ├── recovery-set.json
    └── chain-data/
```

The builder selects checkpoint C and an archived segment boundary A:

```text
archive history:     blocks 1 ... archive watermark
checkpoint history:  blocks A ... C
checkpoint state:    state at C
```

<!-- START ALPHA CHANGE 2209 -- align linked recovery example with production retention -->
The checkpoint retains at least 43,200 blocks. With 1,800-block archive segments, the first possible shared boundary is A=1800, requiring an exported checkpoint C of at least 44999 and archive coverage through block 1800.
<!-- FINISH ALPHA CHANGE 2209 -- align linked recovery example with production retention -->

The actual block at A must match in both artifacts. `recovery-set.json` pins the archive manifest used. Coverage overlaps by at least one block.

Ordinary checkpoints and recovery checkpoints share the checkpoint output root. An ordinary checkpoint already published at the selected height and hash cannot be overwritten by a recovery checkpoint; use a later checkpoint height or a separate snapshot output root.

### External Storage And Future Transaction Retrieval

The archive files are uncompressed and can be copied to offline storage or uploaded to file/object storage while preserving their relative paths.

Publish segment files first, the immutable manifest next, and archive `latest.json` last. Verify the copied files against their recorded sizes and checksums before relying on the external copy.

A future server application can import the segment indexes into a local transaction resolver, then download and cache the complete MDBX segment needed for a transaction. The manifest alone does not locate an arbitrary transaction hash.

Uploading, downloading, cache management, and archive-backed explorer/RPC queries are not implemented by these commands.

### Preserve Coverage Before Removing Node History

Keep the original database until the archive and linked checkpoint have been verified. Also ensure blocks produced after checkpoint C remain recoverable from another trusted source, or stop production at C before cutover.

The builder does not activate checkpoints or remove source history automatically. Validator identities and configuration require separate backups.

The source node may remain online during construction, but do not delete, replace, or prune its database during export.

See the [snapshot-builder guide](../packaging/innbc-snapshot-builder/README-snapshot-builder.md) for verification, retry behavior, capacity limits, and operational requirements.
<!-- FINISH ALPHA CHANGE 2096 -- document local archive creation and linked recovery checkpoints -->

Boot node2 from the segmented snapshot:

/* START ALPHA CHANGE 2499 -- inherit historical query defaults for recent-node snapshot startup --

Snapshot startup uses the same event-log range and fee-history code defaults
as normal recent-node startup.

FINISH ALPHA CHANGE 2499 -- inherit historical query defaults for recent-node snapshot startup -- */

./localtestclient/bin/innbc-chain server \
  --chain localtestclient/genesis.json \
  --data-dir localtestclient/nodes/node2 \
  --grpc-address 127.0.0.1:9633 \
  --jsonrpc 127.0.0.1:8546 \
  --libp2p 127.0.0.1:30302 \
  --nat="" \
  --dns="" \
  --peer-directory-enabled=true \
  --peer-directory-url http://127.0.0.1:8080/api/v1/peers/exchange \
  --history-mode recent \
  --history-retention-blocks 43200 \
  --history-segment-blocks 1800 \
  --snapshot-source-dir localtestclient/snapshots/segmented \
  --snapshot-bootstrap-required=true \
  --mdbx-map-size-gb 128 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3510 -- generate required-bootstrap database pages --
# FINISH ALPHA CHANGE 3510 -- generate required-bootstrap database pages --

<!-- START ALPHA CHANGE 3510 -- generate strict-bootstrap page compatibility guidance -->
With --snapshot-bootstrap-required=true, startup fails unless a valid snapshot of the correct kind can be activated. With false, a recent node normally falls back to synchronization when no usable snapshot is available. A selected snapshot with incompatible database page sizes stops startup in either mode.
<!-- FINISH ALPHA CHANGE 3510 -- generate strict-bootstrap page compatibility guidance -->

<!-- FINISH ALPHA CHANGE 1821 -- document independent local snapshot building and activation -->

<!-- START ALPHA CHANGE 2281 -- document local database backups and admission defaults -->
## Local Database Backups And Optional Trie Reconstruction

Run these commands from the repository root. Direct localtestclient commands
use explicit CLI options and compiled defaults. They do not automatically read
generated node env files or `/etc/innbc/snapshot-builder.env`.

Create a blockchain database backup from local full-history node1:

```bash
./localtestclient/bin/innbc-chain snapshot backup \
  --chain localtestclient/genesis.json \
  --data-dir localtestclient/nodes/node1 \
  --out localtestclient/database-backups \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3510 -- generate backup destination geometry --
# FINISH ALPHA CHANGE 3510 -- generate backup destination geometry --
```

Normal source-node operation may continue during export. Do not replace,
delete, prune or restore the source database while the backup runs.

The command prints the completed backup directory. It contains `backup.json`,
the preserved `genesis.json`, and `chain-data/blockchain/mdbx.dat`. It contains
no trie, consensus state, keys or startup snapshot pointers.

Preserve the completed archive unchanged. For optional trie reconstruction,
prepare a separate working copy at `localtestclient/backup-work`, then run:

```bash
./localtestclient/bin/innbc-chain node trie-rebuild \
  --chain localtestclient/backup-work/genesis.json \
  --data-dir localtestclient/backup-work \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3510 -- generate trie reconstruction geometry --
# FINISH ALPHA CHANGE 3510 -- generate trie reconstruction geometry --
```

<!-- START ALPHA CHANGE 3510 -- generate backup reconstruction compatibility guidance -->
Use the genesis preserved in the backup, including when reconstructing a
downloaded production backup. Do not substitute the local test network's genesis.

The example assumes the backup's blockchain database uses 4096-byte pages.
For an 8192-byte blockchain database, pass `--mdbx-page-size-bytes 8192`.
Trie reconstruction must match the retained blockchain database's page size.
<!-- FINISH ALPHA CHANGE 3510 -- generate backup reconstruction compatibility guidance -->

The working trie directory must be absent or empty. No node or maintenance
process may use the reconstruction target concurrently. Complete unified
history and the existing allocation-based genesis requirements apply.

Reconstruction is optional and can run locally or on the server. Local replay
is recommended when practical to avoid loading the production server.
Neither command downloads, uploads or schedules backups automatically.

## Local Admission Defaults

The foreground examples omit the following flags because their compiled
defaults already match the production configuration. Add a flag to the
corresponding complete command when an override is required.

For `innbc-chain server`:

| Flag | Default |
| --- | --- |
| `--json-rpc-request-rate` | `100` |
| `--json-rpc-request-burst` | `200` |
| `--json-rpc-call-rate` | `200` |
| `--json-rpc-call-burst` | `400` |
| `--json-rpc-max-concurrent` | `32` |
| `--json-rpc-max-expensive` | `4` |
| `--json-rpc-max-body-bytes` | `1048576` |
| `--json-rpc-max-websockets` | `64` |
| `--json-rpc-max-filters` | `1024` |

For `innbc-chain peerdirectory`:

| Flag | Default |
| --- | --- |
| `--request-rate` | `20` |
| `--request-burst` | `64` |
| `--peer-request-interval` | `5s` |
| `--peer-request-burst` | `8` |
| `--max-concurrent-exchanges` | `32` |
| `--max-tracked-peers` | `4096` |

Production services translate their env settings into these flags. Editing a
local env file alone does not change a directly launched foreground command.

Use distinct JSON-RPC, gRPC and libp2p ports for each simultaneous local node.
<!-- FINISH ALPHA CHANGE 2281 -- document local database backups and admission defaults -->
README

echo "INNBC local testclient workspace created at:"
echo "${OUT_DIR}"
# FINISH ALPHA CHANGE 767 -- build local INNBC nodeclient test workspace --