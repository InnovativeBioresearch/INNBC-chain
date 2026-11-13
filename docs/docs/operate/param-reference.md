<!-- START ALPHA CHANGE 1035 -- replace stale legacy parameter reference with current INNBC parameter reference -->

# Configuration Parameter Reference

This page lists the current high-value INNBC CLI parameters.

For the complete live list, use:

```bash
innbc-chain genesis --help
innbc-chain server --help
innbc-chain node --help
innbc-chain snapshot build --help
innbc-chain peerdirectory --help
```

The current INNBC runtime does not use the removed legacy bridge, rootchain, IBFT, bootnode, burn-contract, StakeManager, SupernetManager, reward-wallet, or custom reward-token flags.

## Genesis

| Parameter | Description |
| --- | --- |
| `--dir` | Output path for the generated genesis file. |
| `--chain-id` | Numeric chain ID. |
| `--name` | Chain name. |
| `--premine` | Premined account and balance, repeatable. |
| `--block-gas-limit` | Maximum block gas limit. |
| `--genesis-base-fee-config` | Initial base fee, elasticity multiplier, and change denominator. |
| `--consensus` | Consensus protocol. Current default is `innbcbft`. |
| `--epoch-size` | Number of blocks per epoch. |
| `--min-validator-count` | Minimum validator count. |
| `--max-validator-count` | Maximum validator count. |
| `--validators` | Explicit validator entries in `<multiaddr>:<ECDSA address>:<BLS key>:<BLS proof>` format. |
<!-- START ALPHA CHANGE 2045 -- describe mandatory explicit genesis possession proofs -->
The BLS key and proof must encode exactly 128 and 64 bytes respectively. Optional `0x` prefixes are accepted. The proof must match the operator and key; length checks alone are insufficient. IPv6 multiaddresses are supported.
<!-- FINISH ALPHA CHANGE 2045 -- describe mandatory explicit genesis possession proofs -->
| `--validators-path` | Root path containing validator secrets. |
| `--validators-prefix` | Folder prefix for validator secret directories. |
| `--sprint-size` | Number of blocks in a sprint. |
<!-- START ALPHA CHANGE 2519 -- document whole-second block-time requirements -->
| `--block-time` | Chain-clock increment and production pacing interval. Defaults to `2s`; must be at least `1s` and an exact multiple of one second. Every non-genesis timestamp must equal its parent's timestamp plus this interval. Outages do not advance chain time. | <!-- ALPHA CHANGE 3324 -->
<!-- FINISH ALPHA CHANGE 2519 -- document whole-second block-time requirements -->
| `--trieroot` | <!-- START ALPHA CHANGE 3399 -- distinguish an external trie-root commitment from retired regenesis copying -->External initial trie-root commitment recorded in INNBCBFT configuration. Supplying it does not copy, construct or recover a trie database.<!-- FINISH ALPHA CHANGE 3399 -- distinguish an external trie-root commitment from retired regenesis copying --> |
| `--native-token-config` | Native token configuration. |
<!-- START ALPHA CHANGE 3324 -- remove the retired drift parameter -->
| `--block-tracker-poll-interval` | Internal block-tracker polling interval. |
<!-- FINISH ALPHA CHANGE 3324 -- remove the retired drift parameter -->
| `--validator-registry-minimum-stake` | Initial registry minimum in wei. Defaults to `1` and must equal `1`. Node startup and Go registry initialization enforce the same genesis requirement. Governance may change the live contract minimum later. |
| `--validator-registry-proposal-duration-preset` | Validator governance proposal duration preset: `short`, `standard`, or `long`. |

## Server

| Parameter | Description |
| --- | --- |
| `--chain` | Genesis file used to start the node. |
| `--data-dir` | Node data directory. |
| `--grpc-address` | Operator gRPC listen address. Must resolve to an IPv4 or IPv6 loopback IP, such as `127.0.0.1:9632` or `[::1]:9632`. Explicit wildcard, LAN, and public addresses are rejected at startup. An omitted host uses the existing localhost default. |
| `--jsonrpc` | JSON-RPC listen address. |
| `--libp2p` | LibP2P listen address. |
| `--log-level` | Console log level. |
| `--log-to` | Optional log file path. |
| `--price-limit` | Minimum gas price accepted into the transaction pool. |
| `--max-slots` | Maximum transaction pool slots. |
| `--max-enqueued` | Maximum enqueued transactions per account. |
| `--access-control-allow-origins` | JSON-RPC CORS origins. |
| `--json-rpc-batch-request-limit` | JSON-RPC batch request limit. |
| `--json-rpc-block-range-limit` | <!-- START ALPHA CHANGE 2488 -- document finite historical log-range defaults -->Maximum `toBlock - fromBlock` difference for historical log queries. Default: `256`. Zero selects `256`; positive overrides remain supported. Inclusive endpoints can cover one additional block. Independent of fee-history limits.<!-- FINISH ALPHA CHANGE 2488 -- document finite historical log-range defaults --> |
| `--json-rpc-fee-history-block-limit` | <!-- START ALPHA CHANGE 2469 -- document configurable fee-history budgets -->Maximum served blocks; default 256. Zero selects the default. |
| `--json-rpc-fee-history-percentile-limit` | Maximum requested percentiles, 1-100; default 10. Zero selects the default. |
| `--json-rpc-fee-history-result-limit` | Maximum `served blocks * max(1, requested percentiles)`; default 2560. Zero selects the default.<!-- FINISH ALPHA CHANGE 2469 -- document configurable fee-history budgets --> |
| `--max-peers` | <!-- START ALPHA CHANGE 2528 -- document bounded peer connection settings -->Maximum direct peer connections per node; default 40. Resolved values must be 0-1000. `-1` means unset. Cannot be combined with directional peer flags. |
| `--max-inbound-peers` | Maximum inbound connections; default 32. Resolved values must be 0-1000, and inbound plus outbound must not exceed 1000. `-1` means unset. |
| `--max-outbound-peers` | Maximum outbound connections; default 8. Resolved values must be 0-1000, and inbound plus outbound must not exceed 1000. `-1` means unset. This ceiling bounds dial-slot initialization work; it does not limit the chain's validator count.<!-- FINISH ALPHA CHANGE 2528 -- document bounded peer connection settings --> |
| `--no-discover` | Disable peer discovery. |
| `--nat` | External IP advertised to peers. |
| `--dns` | External DNS name advertised to peers. |
| `--prometheus` | Prometheus metrics listen address. |
| `--metrics-interval` | Metrics generation interval. |
| `--history-mode` | Local history mode: `full` uses standard continuous storage; `recent` uses physically removable block-range segments. |
| `--history-retention-blocks` | Number of post-genesis blocks retained by recent segmented storage. Must be divisible by the segment size. |
| `--history-segment-blocks` | Number of blocks in each independently removable recent-history MDBX segment. |
| `--snapshot-source-dir` | Snapshot-kind directory containing `latest.json`; normally `snapshots/segmented` for recent nodes or `snapshots/checkpoint` for full Plan B bootstrap. |
| `--snapshot-bootstrap-required` | Requires snapshot activation when the selected startup policy permits it; missing or invalid required snapshots stop startup. |
| `--mdbx-map-size-gb` | <!-- START ALPHA CHANGE 3084 -- document server storage defaults and role overrides -- -->MDBX growth allowance in GiB. Code default: 1024. Generated full nodes use 1024; generated recent nodes explicitly use 128. The setting applies independently to blockchain and trie storage; recent blockchain storage distributes its allowance across segments. Consensus uses a separate setting. This is not upfront physical allocation.<!-- FINISH ALPHA CHANGE 3084 -- document server storage defaults and role overrides -- --> |
| `--mdbx-page-size-bytes` | <!-- START ALPHA CHANGE 3507 -- document server database pages inside the parameter table -->Blockchain and trie page size in bytes. Default and blank textual value: 4096. Supported values: 4096, 8192, 16384, 32768, 65536. Existing databases and selected snapshots must match; mismatch stops startup even when snapshot bootstrap is optional. Packaged services read `INNBC_MDBX_PAGE_SIZE_BYTES`; direct foreground commands require an explicit flag for overrides. Consensus retains its separate fixed page size.<!-- FINISH ALPHA CHANGE 3507 -- document server database pages inside the parameter table --> |
| `--peer-directory-enabled` | Enables peer-directory based peer exchange. |
| `--peer-directory-url` | Peer-directory exchange endpoint. |

## Snapshot Builder

| Parameter | Description |
| --- | --- |
| `snapshot build segmented` | <!-- START ALPHA CHANGE 1830 -- align snapshot parameter reference with artifact kinds -->Build and publish a segmented recent-history snapshot. |
| `snapshot build checkpoint` | Build and publish a standard-layout Plan B checkpoint snapshot.<!-- FINISH ALPHA CHANGE 1830 -- align snapshot parameter reference with artifact kinds --> |
| `--chain` | Genesis configuration used to validate the persisted chain identity. |
| `--source-data-dir` | Full-history source node directory opened for read-only snapshot access. |
| `--output-root` | Parent directory under which `segmented` and `checkpoint` roots are maintained. |
| `--mdbx-page-size-bytes` | <!-- START ALPHA CHANGE 3507 -- document maintenance pages inside the parameter table -->Destination database page size; default and blank textual value: 4096. Also supported by snapshot archive, backup, recovery-checkpoint and node trie-rebuild. Sources retain stored geometry. Existing archives and reused recovery artifacts must match. Recovery construction rechecks archive compatibility under both publication locks. Trie rebuild must match retained blockchain storage. The packaged builder reads `INNBC_SNAPSHOT_BUILDER_MDBX_PAGE_SIZE_BYTES`; direct commands require their own flag for overrides.<!-- FINISH ALPHA CHANGE 3507 -- document maintenance pages inside the parameter table --> |
| `--retention-blocks` | Number of canonical post-genesis blocks copied into the artifact. |
| `--segment-blocks` | Segment geometry used by segmented snapshots. |
| `--mdbx-map-size-gb` | <!-- START ALPHA CHANGE 3084 -- document direct snapshot destination settings -- -->Destination MDBX allowance in GiB; direct CLI default: 1024. Explicitly pass 128 for segmented snapshots targeting recent nodes. The packaged wrapper reads separate general and segmented allowances from its builder environment.<!-- FINISH ALPHA CHANGE 3084 -- document direct snapshot destination settings -- --> |

## Node Service Management

<!-- START ALPHA CHANGE 3376 -- document creation limits and the read-only inspection interface -->

| Command or option | Purpose |
| --- | --- |
| `innbc-chain node prepare [count]` | Prepare node1 through the requested count, limited to 1–10, without generating keys. |
| `innbc-chain node create [count]` | Create 1–10 new identities per invocation, allocating available numbers within node1–node100. |
| `--snapshot-root` | Shared snapshot parent written into environments generated by preparation and creation; default `/var/lib/innbc/snapshots`. |
| `innbc-chain node info nodeN` | Validate one stored identity and independently inspect its public environment; N is 1–100. |
| `innbc-chain node info all` | Inspect through the highest discovered identity or environment number, including interior missing-slot warnings. |
| `--base-dir` | Identity base directory for creation, preparation and inspection; default `/var/lib/innbc`. |
| `--config-dir` | Environment directory for creation, preparation and inspection; default `/etc/innbc`. |
| `--json` with `node info` | Emit a structured public inspection report; terminal failures use standard error and status 1. |
| `innbc-chain node start` | Start one node service or all configured node services. |
| `innbc-chain node stop` | Stop one node service or all configured node services. |
| `innbc-chain node restart` | Restart one node service or all configured node services. |
| `innbc-chain node status` | Show systemd status for one node service or all configured node services. |

Creation checks capacity before writing, but later execution failures can leave partial progress. Inspection is read-only: it reports missing or invalid occupied slots without repairing them or printing private keys.

Inspection does not infer absent trailing nodes or establish full runtime configuration validity. See the [inspection reference](../node-env/README.md#read-only-node-inspection).

<!-- FINISH ALPHA CHANGE 3376 -- document creation limits and the read-only inspection interface -->

## Peer Directory

| Parameter | Description |
| --- | --- |
| `--listen` | Peer-directory API listen address. |
| `--jsonrpc` | JSON-RPC endpoint used to read validator registry state. |
| `--validator-registry` | INNBC validator registry contract address. |
| `--chain` | Genesis chain config used to read bootstrap validators. |

## Peer Directory Exchange

| Parameter | Description |
| --- | --- |
| `--url` | Peer-directory exchange endpoint. |
| `--data-dir` | Validator node data directory containing signing keys. |
| `--multiaddr` | Validator LibP2P multiaddr to publish. |

<!-- FINISH ALPHA CHANGE 1035 -- replace stale legacy parameter reference with current INNBC parameter reference -->