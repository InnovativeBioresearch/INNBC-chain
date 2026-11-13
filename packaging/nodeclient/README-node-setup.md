<!-- START ALPHA CHANGE 749 -- add production nodeclient setup notes -->
# INNBC Nodeclient

This bundle installs the INNBC node binary and shared chain configuration.

<!-- START ALPHA CHANGE 2284 -- document configurable production RPC admission -->
## RPC Admission Settings

The production systemd service reads `/etc/innbc/<node>.env` and passes the
following settings to the node:

```ini
INNBC_JSONRPC_REQUEST_RATE=100
INNBC_JSONRPC_REQUEST_BURST=200
INNBC_JSONRPC_CALL_RATE=200
INNBC_JSONRPC_CALL_BURST=400
INNBC_JSONRPC_MAX_CONCURRENT=32
INNBC_JSONRPC_MAX_EXPENSIVE=4
INNBC_JSONRPC_MAX_BODY_BYTES=1048576
INNBC_JSONRPC_MAX_WEBSOCKETS=64
INNBC_JSONRPC_MAX_FILTERS=1024
```

Newly generated environments include these settings. The updated service also
supplies the same defaults when preserved older env files omit them. Explicit
values in a node's env file override the service defaults.

All values must be positive, and maximum expensive concurrency must not exceed
maximum overall concurrency. These are operating limits, not throughput
guarantees.

After changing a node's env settings, restart that node when operationally
appropriate. Changing an env file alone does not update a running process.
Installing a changed systemd unit also requires a systemd daemon reload.

Direct foreground commands do not automatically read these env files. Their
equivalent flags are `--json-rpc-request-rate`, `--json-rpc-request-burst`,
`--json-rpc-call-rate`, `--json-rpc-call-burst`, `--json-rpc-max-concurrent`,
`--json-rpc-max-expensive`, `--json-rpc-max-body-bytes`,
`--json-rpc-max-websockets`, and `--json-rpc-max-filters`.

Omitting these flags uses the same Go defaults shown above.
<!-- FINISH ALPHA CHANGE 2284 -- document configurable production RPC admission -->

## Build The Nodeclient Bundle

From the repository root, build the release folder with:

make nodeclient

This creates:

dist/innbc-nodeclient

The generated folder contains the node binary, genesis file, install script, systemd service template, example configuration files, and this setup guide.

<!-- START ALPHA CHANGE 999 -- document separate peer directory service package output -->
The same build command also copies the standalone peer directory service package to:

dist/innbc-peerdirectory-service

That package has its own installer and does not touch the node binary, genesis, node env files, validator keys, or chain data.
<!-- FINISH ALPHA CHANGE 999 -- document separate peer directory service package output -->

<!-- START ALPHA CHANGE 757 -- document MDBX nodeclient storage behavior -->
The node binary stores persistent blockchain and trie data with MDBX. Existing LevelDB node data directories are not compatible with this bundle; use a fresh node data directory or restore from an MDBX-compatible backup.
<!-- FINISH ALPHA CHANGE 757 -- document MDBX nodeclient storage behavior -->

Installed files:

/usr/local/bin/innbc-chain
/etc/innbc/genesis.json
/etc/innbc/node.example.yaml
/etc/innbc/node.private-local.env.example
/etc/innbc/node.external-validator.env.example
/etc/systemd/system/innbc-node@.service

The installer creates only base directories:

/etc/innbc
/var/lib/innbc
/var/log/innbc

<!-- START ALPHA CHANGE 758 -- document MDBX data directory ownership -->
Each validator stores replaceable MDBX runtime data under its dedicated `chain-data` directory:

/var/lib/innbc/node1/chain-data/blockchain
/var/lib/innbc/node1/chain-data/trie
/var/lib/innbc/node1/chain-data/consensus

Private node identities are stored separately under `/var/lib/innbc/node1/keys`.
<!-- FINISH ALPHA CHANGE 758 -- document MDBX data directory ownership -->

It does not create validator identity folders. Validator identities are created later with explicit node-management commands.

<!-- START ALPHA CHANGE 1929 -- document the unprivileged production ownership boundary -->
## Runtime account and node ownership

The production installers create one non-login system account:

```text
innbc:innbc
```

Validator services run as this account rather than as root. Package-managed executables, configuration files, genesis, and systemd units remain root-owned.

The installer does not create, recursively modify, or assume the number of `/var/lib/innbc/nodeN` directories. After creating a node with `node create`, preparing it with `node prepare`, or uploading an existing identity directory, assign only that node directory to the runtime account before starting its service:

```bash
sudo chown -R innbc:innbc /var/lib/innbc/node1
```

For additional nodes, repeat the command using their exact installed directories:

```bash
sudo chown -R innbc:innbc /var/lib/innbc/node2
sudo chown -R innbc:innbc /var/lib/innbc/node3
```

Do not apply recursive ownership changes to `/etc`, `/usr/local`, `/opt`, or the complete server filesystem.

The node start, stop, restart, status, and disable commands remain unchanged. Systemd automatically launches the underlying validator process as `innbc`.
<!-- FINISH ALPHA CHANGE 1929 -- document the unprivileged production ownership boundary -->

<!-- START ALPHA CHANGE 2740 -- document editable external and private networking defaults -->
## External-Validator Mode

Use `--mode external-validator` with `node create` or `node prepare` for externally reachable production validators using the centralized public peer directory.

The node1 networking defaults are:

```ini
INNBC_JSONRPC_ADDR=127.0.0.1:8545
INNBC_GRPC_ADDR=127.0.0.1:9632
INNBC_LIBP2P_ADDR=0.0.0.0:30301
INNBC_NAT_ADDR=
INNBC_DNS_ADDR=
INNBC_PEER_DIRECTORY_ENABLED=true
INNBC_PEER_DIRECTORY_URL=https://innbcchain.io/api/v1/peers/exchange
```

Configure the advertised public IPv4 address in `INNBC_NAT_ADDR`, or a DNS prefix such as `/dns4/validator.example.com` in `INNBC_DNS_ADDR`. NAT takes precedence if both are set. The advertised TCP port is the listening port.

For direct exposure, use a reachable P2P listener. For a same-server nginx TCP proxy, change the backend listener to `127.0.0.1:30301`, advertise the gateway address, and separately configure forwarding from the public endpoint to that backend.

The node does not install or configure the proxy. The public directory URL is an HTTP address exchange, not a P2P forwarding endpoint.

Expose only the intended public P2P endpoint. Keep management gRPC private, and publish JSON-RPC separately through an HTTP proxy when required.

## Private-Local Mode

Use `--mode private-local`, the default, for local testing and private deployments.

The node1 networking defaults are:

```ini
INNBC_JSONRPC_ADDR=127.0.0.1:8545
INNBC_GRPC_ADDR=127.0.0.1:9632
INNBC_LIBP2P_ADDR=127.0.0.1:30301
INNBC_NAT_ADDR=
INNBC_DNS_ADDR=
INNBC_PEER_DIRECTORY_ENABLED=true
INNBC_PEER_DIRECTORY_URL=http://127.0.0.1:8080/api/v1/peers/exchange
```

Both modes include the advertised-address placeholders and use the same runtime. Either env file can be customized for private interfaces, direct public exposure, or a TCP proxy deployment. Additional nodes receive incremented default ports.

Restart a node after editing its env. An authorized directory exchange replaces its stored address under the unchanged peer ID; refresh other running nodes through the normal operator workflow.

Keep private P2P and directory listeners unexposed unless intentionally publishing them through the configured gateway.

The complete environment reference is `docs/docs/node-env/README.md` in the source repository.
<!-- FINISH ALPHA CHANGE 2740 -- document editable external and private networking defaults -->


## Service Profiles

Each validator service uses one env file:

/etc/innbc/node1.env
/etc/innbc/node2.env

<!-- START ALPHA CHANGE 1933 -- document mandatory per-node service configuration -->
The environment file is mandatory. The systemd instance name determines its exact path:

```text
innbc-node@node1 → /etc/innbc/node1.env
innbc-node@node2 → /etc/innbc/node2.env
```

If the matching file is missing or unreadable, systemd refuses to start the service before executing `innbc-chain`. It does not silently start the node with template defaults.

Both `node create` and `node prepare` generate the required environment file as part of the normal provisioning workflow.
<!-- FINISH ALPHA CHANGE 1933 -- document mandatory per-node service configuration -->

Fresh node identities create folders, keys, and env files automatically:

sudo innbc-chain node create
sudo innbc-chain node create 10 --mode private-local
sudo innbc-chain node create 10 --mode external-validator

<!-- START ALPHA CHANGE 3370 -- document bounded creation and post-creation inspection in distributed setup notes -->
These are alternative invocation examples. Each invocation accepts one to ten new identities and allocates available numbers within node1–node100. Existing identity or environment paths reserve their numbers.

The complete allocation is checked before writing. A later filesystem or key-generation failure can leave completed nodes and an incomplete current slot; there is no automatic rollback.

After creation, inspect the stored identities and environments:

```bash
sudo innbc-chain node info all
```

Inspection is read-only. It reports incomplete slots rather than repairing them. Rerunning `node create` allocates available slots and does not repair an occupied partial identity.
<!-- FINISH ALPHA CHANGE 3370 -- document bounded creation and post-creation inspection in distributed setup notes -->

<!-- START ALPHA CHANGE 820 -- document explicit genesis path override for generated node env files -->
By default, generated env files use:

/etc/innbc/genesis.json

For local test workspaces or custom installs, override the generated env chain path with:

--chain <path-to-genesis.json>
<!-- FINISH ALPHA CHANGE 820 -- document explicit genesis path override for generated node env files -->

<!-- START ALPHA CHANGE 768 -- document prepared node identity workflow instead of import workflow -->
Prepared node identities create empty folders and env files, but do not create keys:

sudo innbc-chain node prepare
sudo innbc-chain node prepare 10 --mode private-local
sudo innbc-chain node prepare 10 --mode external-validator

Use prepare when an existing validator identity must be placed manually into the node folder. After prepare, copy the existing key files into:

<!-- START ALPHA CHANGE 1639 -- document isolated production identity paths -->
/var/lib/innbc/node1/keys/validator-ecdsa/validator.key
/var/lib/innbc/node1/keys/validator-bls/validator-bls.key
/var/lib/innbc/node1/keys/libp2p/libp2p.key
<!-- FINISH ALPHA CHANGE 1639 -- document isolated production identity paths -->

The key files are intentionally not created by prepare. Missing or invalid keys will prevent the node from starting correctly.
<!-- FINISH ALPHA CHANGE 768 -- document prepared node identity workflow instead of import workflow -->

<!-- START ALPHA CHANGE 774 -- document public node info command for validator registry registration -->
<!-- START ALPHA CHANGE 3370 -- document read-only public identity and environment inspection -->
After creating an identity, or after copying all keys into a prepared identity, inspect one node:

```bash
sudo innbc-chain node info node1
```

Inspect all discovered slots:

```bash
sudo innbc-chain node info all
```

Supported names are node1–node100. The default identity base is `/var/lib/innbc`; the default environment directory is `/etc/innbc`. Pass both `--base-dir` and `--config-dir` for custom installations. Add `--json` for structured output.

Inspection validates stored keys without initializing storage or modifying files. It reports identity and environment status independently and displays only public identity values and recognized public environment settings.

An invalid occupied slot causes status 1 after reporting. Completely absent interior slots produce warnings without causing failure. Discovery ends at the highest number having an identity or environment path; it cannot infer an intended trailing node for which nothing remains.

Private-key contents are not printed. Unknown environment variables are omitted, and displayed peer-directory URLs omit user information, query parameters and fragments.

Environment inspection checks stored assignments and selected structural requirements. It does not execute the file, validate every runtime setting, verify service ownership or prove that a node can start. Compare the public identity with a trusted expected identity when checking genesis or registry membership.
<!-- FINISH ALPHA CHANGE 3370 -- document read-only public identity and environment inspection -->

The public identity fields include:

<!-- START ALPHA CHANGE 2043 -- document proof-bearing validator registration -->
Operator address / registry msg.sender
BLS public key / registerValidator blsKey
BLS proof / registerValidator blsProof
Libp2p peer ID / registerValidator libp2pPeerID
Private-local multiaddr
External-validator multiaddr template

The registry call is:

registerValidator(rewardAddress, blsKey, blsProof, libp2pPeerID, stakeAmount)

The BLS proof is public and binds the operator address to the exact BLS public key. Use the proof printed by the revised `node info` command.

Changing a registered BLS key requires:

updateBLSKey(blsKey, blsProof)

The replacement proof must match the same operator and the replacement key. A successful key change removes whitelist status until governance approves the validator again. An unchanged-key update is rejected.
<!-- FINISH ALPHA CHANGE 2043 -- document proof-bearing validator registration -->

The operator address is the wallet that must call the registry. The rewardAddress and stakeAmount are manual registration inputs, not values derived from the node keys. Choose a stakeAmount greater than or equal to the current minimumStake.
<!-- FINISH ALPHA CHANGE 774 -- document public node info command for validator registry registration -->

The example env files are installed only as references:

/etc/innbc/node.private-local.env.example
/etc/innbc/node.external-validator.env.example

## Local Testclient Workspace

<!-- START ALPHA CHANGE 769 -- document local nodeclient test workspace build flow -->
For local testing without installing the production bundle, build:

make localtestclient

This creates localtestclient/ with its own binary, genesis copy, config folder, and nodes folder. It does not touch dist/innbc-nodeclient and it does not install anything into system directories.

Local node env files should be generated with the local genesis path and shared snapshot root:

<!-- START ALPHA CHANGE 1973 -- document independent local snapshot roots -->
./localtestclient/bin/innbc-chain node prepare 2 --base-dir localtestclient/nodes --config-dir localtestclient/config --chain localtestclient/genesis.json --snapshot-root localtestclient/snapshots --mode private-local

or:

./localtestclient/bin/innbc-chain node create 2 --base-dir localtestclient/nodes --config-dir localtestclient/config --chain localtestclient/genesis.json --snapshot-root localtestclient/snapshots --mode private-local
<!-- FINISH ALPHA CHANGE 1973 -- document independent local snapshot roots -->

<!-- START ALPHA CHANGE 3370 -- inspect the matching local identity and environment directories -->
After creation, or after importing all keys into prepared folders, inspect from the repository root:

```bash
./localtestclient/bin/innbc-chain node info all \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config
```

Include both paths so inspection reads the local environments rather than `/etc/innbc`.
<!-- FINISH ALPHA CHANGE 3370 -- inspect the matching local identity and environment directories -->
<!-- FINISH ALPHA CHANGE 769 -- document local nodeclient test workspace build flow -->

## Node Management Commands

Create one fresh validator identity:

sudo innbc-chain node create

Create multiple fresh validator identities:

sudo innbc-chain node create 10

Create private-local node identities:

sudo innbc-chain node create 10 --mode private-local

Create external-validator node identities:

sudo innbc-chain node create 10 --mode external-validator

<!-- START ALPHA CHANGE 770 -- document empty node preparation command -->
Prepare one empty validator identity folder for manually copied keys:

sudo innbc-chain node prepare

Prepare multiple empty validator identity folders:

sudo innbc-chain node prepare 10

<!-- START ALPHA CHANGE 3370 -- inspect manually installed identity files before startup -->
After copying keys into a prepared folder, run `sudo innbc-chain node info nodeN` for that node, compare its public identity with the expected identity, and review its environment result before using the normal start command.
<!-- FINISH ALPHA CHANGE 3370 -- inspect manually installed identity files before startup -->
<!-- FINISH ALPHA CHANGE 770 -- document empty node preparation command -->

<!-- START ALPHA CHANGE 775 -- document node info command in management command list -->
<!-- START ALPHA CHANGE 3370 -- list individual and discovered-node inspection commands -->
Validate and inspect one node's stored identity and public environment:

```bash
sudo innbc-chain node info node1
```

Inspect all discovered slots, including interior missing-slot warnings:

```bash
sudo innbc-chain node info all
```

Print structured inspection output:

```bash
sudo innbc-chain node info all --json
```

These commands are read-only and do not print private keys.
<!-- FINISH ALPHA CHANGE 3370 -- list individual and discovered-node inspection commands -->
<!-- FINISH ALPHA CHANGE 775 -- document node info command in management command list -->

Start one node in the background and enable it after server reboot:

sudo innbc-chain node start node1

Stop one node now:

sudo innbc-chain node stop node1

Restart one node:

sudo innbc-chain node restart node1

Check one node:

sudo innbc-chain node status node1

Start all configured nodes in the background and enable them after server reboot:

sudo innbc-chain node start all

Stop all configured nodes now:

sudo innbc-chain node stop all

Restart all configured nodes:

sudo innbc-chain node restart all

Check all configured nodes:

sudo innbc-chain node status all

Stop one node and disable automatic startup after server reboot:

sudo innbc-chain node disable node1

Stop all configured nodes and disable automatic startup after server reboot:

sudo innbc-chain node disable all

<!-- START ALPHA CHANGE 856 -- document full and recent history node modes -->
## History Mode Commands

The default node history mode is full:

INNBC_HISTORY_MODE=full

Full mode keeps the complete canonical blockchain in one continuous blockchain MDBX database.

<!-- START ALPHA CHANGE 1659 -- document packaged recent-history physical layout -->
<!-- START ALPHA CHANGE 1860 -- document current recent-history node settings -->
Recent mode stores block zero in a permanent genesis MDBX database and stores post-genesis canonical blockchain records in independently removable MDBX segment directories:

```text
INNBC_HISTORY_MODE=recent
INNBC_HISTORY_RETENTION_BLOCKS=43200
INNBC_HISTORY_SEGMENT_BLOCKS=1800
INNBC_SNAPSHOT_SOURCE_DIR=/var/lib/innbc/snapshots/segmented
INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED=false
INNBC_MDBX_MAP_SIZE_GB=128
# START ALPHA CHANGE 3508 -- document packaged node database pages --
INNBC_MDBX_PAGE_SIZE_BYTES=4096
# FINISH ALPHA CHANGE 3508 -- document packaged node database pages --
```

The resulting blockchain directory is:

```text
chain-data/blockchain/
├── genesis/
│   ├── mdbx.dat
│   └── mdbx.lck
└── segments/
    ├── segment-00000000000000000001-00000000000000001800/
    ├── segment-00000000000000001801-00000000000000003600/
    └── ...
```

At a two-second block time, the production defaults retain approximately 24 hours of post-genesis history in one-hour segments. When a completed segment expires, its MDBX environment is closed and its directory is physically removed. The permanent genesis database, trie, consensus database, and node identities remain outside segment retirement.

The retention value must be at least one segment and exactly divisible by the segment size.
<!-- FINISH ALPHA CHANGE 1860 -- document current recent-history node settings -->
<!-- FINISH ALPHA CHANGE 1659 -- document packaged recent-history physical layout -->

For a production service node, edit the node env file before starting or restarting the service:

/etc/innbc/node1.env

Then restart:

sudo innbc-chain node restart node1

For local one-shot testing, pass the flags directly:

<!-- START ALPHA CHANGE 1861 -- document current local recent-history node command -->
<!-- START ALPHA CHANGE 2213 -- use production settings for normal local testing -->
The following local command uses production history defaults: 43,200 retained blocks in 1,800-block segments. At a two-second block interval, segment rotation occurs approximately hourly and retained history ranges from approximately 23 to 24 hours.
<!-- FINISH ALPHA CHANGE 2213 -- use production settings for normal local testing -->

/* START ALPHA CHANGE 2493 -- distinguish event-log range limits from fee history --

INNBC_JSONRPC_BLOCK_RANGE_LIMIT controls historical event-log queries,
such as eth_getLogs. Its default is 256. Setting it to zero also selects
256; a positive value such as 512 overrides the default.

The limit bounds toBlock - fromBlock. Because both endpoints are included,
a difference of 256 can cover 257 blocks. Oversized ranges are rejected
rather than silently truncated.

For foreground commands, use --json-rpc-block-range-limit.
This setting is independent of the fee-history settings below.

Generated full-history environments include this override. Generated
recent-node environments omit it and inherit the code default.
The shared systemd service supplies zero when the variable is absent;
the application resolves it to 256. A positive per-node environment value
overrides the fallback. Existing environment files are not rewritten.

FINISH ALPHA CHANGE 2493 -- distinguish event-log range limits from fee history -- */

/* START ALPHA CHANGE 2468 -- document packaged fee-history configuration --

Full-history node environments expose these RPC settings:

```dotenv
INNBC_JSONRPC_FEE_HISTORY_BLOCK_LIMIT=256
INNBC_JSONRPC_FEE_HISTORY_PERCENTILE_LIMIT=10
INNBC_JSONRPC_FEE_HISTORY_RESULT_LIMIT=2560
```

Zero selects the corresponding code default. The percentile-count setting
accepts 1 through 100. The independent result budget is:

```text
served blocks * max(1, requested percentiles)
```

For 512 blocks and 100 percentiles, set the three values to 512, 100,
and 51200. Restart the node service after changing its environment.

Recent-node environments generated by `node create` or `node prepare`
omit these overrides and inherit the code defaults. Existing environment
files are not automatically rewritten.

Foreground local commands do not load the environment files. Their
equivalent flags are `--json-rpc-fee-history-block-limit`,
`--json-rpc-fee-history-percentile-limit`, and
`--json-rpc-fee-history-result-limit`.

These limits affect historical RPC reporting, not EVM execution or
consensus rules. Raising the budgets permits more work per request.

FINISH ALPHA CHANGE 2468 -- document packaged fee-history configuration -- */

/* START ALPHA CHANGE 2501 -- inherit historical query defaults in packaged recent-node instructions --

The recent-node command below omits event-log range and fee-history
overrides and uses the code defaults.

FINISH ALPHA CHANGE 2501 -- inherit historical query defaults in packaged recent-node instructions -- */

```bash
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
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3508 -- document local server database geometry --
# FINISH ALPHA CHANGE 3508 -- document local server database geometry --
```
<!-- FINISH ALPHA CHANGE 1861 -- document current local recent-history node command -->

<!-- START ALPHA CHANGE 2214 -- document optional accelerated rotation with execution ancestry -->
For explicitly accelerated local rotation testing, replace the two history flags with:

```bash
  --history-retention-blocks 1080 \
  --history-segment-blocks 30 \
```

At a two-second block interval, a segment opens approximately every minute. Once the retention window fills, between 1,051 and 1,080 blocks remain after rotation.

This is a conservative testing recommendation, not the standard configuration or the absolute minimum accepted by the software.

<!-- START ALPHA CHANGE 2215 -- describe insufficient retention as an unsafe configuration example -->
Avoid settings such as retention 120 blocks with 30-block segments: immediately after rotation, they retain only 91 blocks. This is insufficient to preserve the ancestry needed for the EVM's preceding-256-block `BLOCKHASH` window and can cause execution results to differ from a full-history node.
<!-- FINISH ALPHA CHANGE 2215 -- describe insufficient retention as an unsafe configuration example -->

For custom settings, use at least 1,024 blocks under the formula `retention blocks - segment blocks + 1`, with retention divisible by segment size. Current startup checks do not enforce this safety recommendation. RPC query limits do not change the EVM requirement.

Use matching geometry when building and activating a segmented snapshot. The snapshot must preserve the required ancestry; increasing retention afterward cannot restore omitted headers.

See the [node environment reference](../../docs/docs/node-env/README.md#localtestclient-foreground-runs) for the complete local workflow.
<!-- FINISH ALPHA CHANGE 2214 -- document optional accelerated rotation with execution ancestry -->

<!-- START ALPHA CHANGE 1603 -- document recent segment availability and incompatible layouts -->
Recent-history nodes are validator and synchronization nodes, not full-history RPC backends. Once an expired segment has been removed, its headers, canonical mappings, bodies, receipts, and transaction lookup records are no longer locally available.

Use a full-history node for public RPC history queries and for synchronization requests older than the retained recent-history window.

Full and recent modes use different physical blockchain layouts. Do not switch an existing blockchain data directory between these modes by changing only the configuration flag. Rebuild the node's blockchain data from genesis or from an appropriate snapshot when changing its history role.

Do not change `INNBC_HISTORY_SEGMENT_BLOCKS` for an existing recent-history database. Existing segment boundaries are validated at startup, and incompatible geometry is rejected.
<!-- FINISH ALPHA CHANGE 1603 -- document recent segment availability and incompatible layouts -->

<!-- START ALPHA CHANGE 1862 -- document independent snapshot artifacts and startup matrix -->
## Independent Snapshot Builder

Snapshot production is an explicit, one-shot operation performed outside the running node process. The standalone package installs:

```text
/etc/innbc/snapshot-builder.env
/usr/local/bin/innbc-snapshot-builder
```

Build a segmented recent-history snapshot:

```bash
sudo innbc-snapshot-builder segmented
```

Build a standard-layout checkpoint snapshot for full-node Plan B recovery:

```bash
sudo innbc-snapshot-builder checkpoint
```

Both commands read the running full-history node databases through concurrent read-only MDBX access. They do not ask node1 to construct the snapshot and do not stop block production.

The two snapshot kinds are published separately:

```text
/var/lib/innbc/snapshots/
├── segmented/
│   ├── latest.json
│   └── block-<number>-<hash-prefix>/
│       ├── manifest.json
│       └── chain-data/
└── checkpoint/
    ├── latest.json
    └── block-<number>-<hash-prefix>/
        ├── manifest.json
        └── chain-data/
```

Each `chain-data` directory contains directly usable blockchain, trie, and consensus storage. No genesis-to-head `headers.rlp.hex` artifact is produced.

A new artifact is completely built and validated before its directory and `latest.json` pointer are published. After successful publication, the superseded artifact is removed.

## Snapshot Startup Matrix

Recent nodes point directly to the segmented kind directory:

```text
INNBC_HISTORY_MODE=recent
INNBC_SNAPSHOT_SOURCE_DIR=/var/lib/innbc/snapshots/segmented
INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED=false
```

<!-- START ALPHA CHANGE 3508 -- document page compatibility during snapshot startup -->
With `false`, a recent node normally uses a valid newer segmented snapshot when available and otherwise continues from local data or normal synchronization. A selected snapshot with incompatible database page sizes stops startup even in optional mode. Incompatible existing local databases also stop startup. With `true`, a recent node without an established local chain must activate a valid segmented snapshot or startup fails.

Packaged services read `INNBC_MDBX_PAGE_SIZE_BYTES`, defaulting to 4096. Direct local server commands do not load node environment files; the example passes `--mdbx-page-size-bytes 4096` explicitly. Use the stored database page size when starting an existing node. Changing this setting does not convert existing databases.
<!-- FINISH ALPHA CHANGE 3508 -- document page compatibility during snapshot startup -->

Full nodes point directly to the checkpoint kind directory:

```text
INNBC_HISTORY_MODE=full
INNBC_SNAPSHOT_SOURCE_DIR=/var/lib/innbc/snapshots/checkpoint
INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED=false
```

A full node with `false` does not attempt checkpoint activation. It resumes its local chain or synchronizes normally.

Plan B is activated only with:

```text
INNBC_HISTORY_MODE=full
INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED=true
```

The full node must have no established local chain. Move or remove only its `chain-data` directory before Plan B bootstrap while preserving its `keys` directory. A missing, invalid, corrupted, or wrong-kind checkpoint then causes startup to fail.

An established local chain is never overwritten by full-node checkpoint activation.
<!-- FINISH ALPHA CHANGE 1862 -- document independent snapshot artifacts and startup matrix -->
<!-- FINISH ALPHA CHANGE 856 -- document full and recent history node modes -->
<!-- FINISH ALPHA CHANGE 749 -- add production nodeclient setup notes -->