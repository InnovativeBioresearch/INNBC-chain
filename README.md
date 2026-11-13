<!-- START ALPHA CHANGE 1471 -- document INNBC architecture and enhancements over Polygon Edge -->

# INNBC Chain

INNBC Chain is a sovereign, EVM-compatible blockchain client built from a substantially customized Polygon Edge foundation, developed by Jonathan Fior at Innovative Bioresearch to be the ultimate blockchain client.INNBC Chain is a sovereign, EVM-compatible blockchain client built from a substantially customized Polygon Edge foundation, developed by Jonathan Fior at Innovative Bioresearch to be the ultimate blockchain client. INNBC Chain represents the planned protocol upgrade and migration of **InnovativeBioresearchCoin (INNBC)**, the Ethereum ERC20 token issued in 2018, to a sovereign Layer 1 network. The native currency adopts the name **INNBC Chain**, retains the **INNBC** symbol, and serves as the network's native gas currency.

The project retains the useful low-level components of Polygon Edge—including its EVM execution environment, Ethereum-compatible data types, JSON-RPC foundation, libp2p networking, and Byzantine fault-tolerant consensus foundation—while replacing or substantially extending major parts of the original client.

INNBC Chain includes:

- INNBCBFT consensus engine
- native INNBC token
- validator registry
- validator governance
- treasury governance
- protocol fee distribution
- proxy upgrade governance
- authenticated validator networking
- native RPC and peer-directory resource protections
- deliberate exclusion of EIP-7702 account delegation to reduce wallet attack surface
- state-error propagation and controlled recovery
- lossless compact MDBX storage
- physically reclaimable segmented recent-history storage
- independent segmented and checkpoint snapshot creation
- independent blockchain backups and offline trie reconstruction
- history-aware peer synchronization
- production node-management workflow
<!-- START ALPHA CHANGE EXPLORER-306 -- link the separately maintained explorer repository -->
- [INNBCScan custom chain explorer, maintained in a separate repository](https://github.com/InnovativeBioresearch/INNBCscan)
<!-- FINISH ALPHA CHANGE EXPLORER-306 -- link the separately maintained explorer repository -->

## Project Status

This repository contains the current INNBC Chain node implementation, core contracts, validator networking, storage and snapshot systems, operator commands, tests, documentation, and production packaging.

Consensus rules, genesis configuration, validator governance, monetary behavior, and system-contract addresses must remain consistent across every validator. Consensus-critical upgrades should be reviewed, tested on a separate network, and deployed through a coordinated chain upgrade.

<!-- START ALPHA CHANGE 1916 -- document genesis generation and validation guarantees -->

## Genesis Generation And Validation Guarantees

The supported production workflow creates `genesis.json` through the `innbc-chain genesis` command. The generated file is a deployment artifact derived from explicit command-line parameters and canonical constants in the node source. It should be regenerated through that command when its configuration changes rather than edited manually.

<!-- START ALPHA CHANGE 2509 -- document genesis block-time validation and close example fence -->
Genesis generation validates the parameters that would otherwise produce an unusable initial chain, including:

- the selected consensus engine;
- genesis base fee, elasticity multiplier, and change denominator;
- compatibility between the block gas limit and base-fee elasticity;
- premine addresses and amounts;
- native-token name, symbol, and decimal metadata;
- INNBCBFT epoch size;
- validator source information;
- minimum and maximum validator-set limits;
<!-- START ALPHA CHANGE 2651 -- document the fixed genesis stake requirement -->
- validator-registry genesis minimum stake of exactly 1 wei;
<!-- FINISH ALPHA CHANGE 2651 -- document the fixed genesis stake requirement -->
- validator-registry proposal-duration preset;
- a block time of at least one second;
- block-tracker polling interval.

When a node imports the resulting file, it independently requires:

- a non-empty chain configuration;
- a genesis configuration;
- chain parameters;
- exactly one consensus engine;
- a positive base fee;
- a positive base-fee elasticity multiplier;
- a positive base-fee change denominator;
- a usable effective genesis gas limit.

<!-- START ALPHA CHANGE 3343 -- document required native-token configuration at initialization boundaries -->

INNBCBFT configuration loading and server startup additionally require a present, non-null `nativeTokenConfig` section. Genesis generation supplies explicit defaults when `--native-token-config` is omitted; supplied genesis configurations with the required section missing or null are rejected rather than silently filled with defaults. The validation and regression coverage are detailed in point 80 under “Native-Token Configuration Presence Validation”.

<!-- FINISH ALPHA CHANGE 3343 -- document required native-token configuration at initialization boundaries -->

<!-- START ALPHA CHANGE 3825 -- distinguish valid initial intervals from future protocol upgrades -->

Genesis generation and node startup require a block-time configuration of at least one second and an exact multiple of one second. Fractional intervals such as `1.5s` are rejected because block-header timestamps store whole seconds. The production default remains `2s`.

A one-second interval is a valid initial configuration for a new chain. Changing an established two-second chain to one second requires a separately implemented and tested protocol upgrade that preserves historical timestamp validation. Changing the configured duration alone does not provide that compatibility.

The examples below identify the current production interval and a possible future upgrade target. Requirements for upgrading an existing chain are documented in point 52.

<!-- FINISH ALPHA CHANGE 3825 -- distinguish valid initial intervals from future protocol upgrades -->

```text
blockTime = 2s
→ current production target

blockTime = 1s
→ valid coordinated upgrade target
```
<!-- FINISH ALPHA CHANGE 2509 -- document genesis block-time validation and close example fence -->

When an existing database is reopened, the node verifies that its persisted canonical genesis block matches the genesis block described by the supplied file. Existing blockchain, trie, and consensus data are preserved when that identity remains unchanged.

<!-- START ALPHA CHANGE 3826 -- qualify consensus configuration upgrade compatibility -->

Consensus configuration is deliberately not frozen into a separate database fingerprint. This permits configuration evolution without automatically discarding existing databases, but it does not establish that every configuration change is compatible with historical validation.

In particular, changing the block interval of an established chain requires a node-software upgrade that selects timestamp rules by activation height. Coordination alone, or replacing the global interval in every node's configuration, is insufficient to preserve fresh synchronization across the change. The production deployment retains the unchanged two-second interval. Mixed consensus configurations are not a supported operating mode.

<!-- FINISH ALPHA CHANGE 3826 -- qualify consensus configuration upgrade compatibility -->

<!-- START ALPHA CHANGE 3056 -- document the INNBC genesis default and supported chain ID range -->

New genesis configurations default to chain ID **1982**, the ID used by INNBC Chain. Both `--chain-id` and the legacy `--chainid` alias use this default. Generating an INNBC genesis therefore does not require an explicit chain-ID override.

<!-- START ALPHA CHANGE 3547 -- document the positive chain ID policy at generation and startup -->

The chain ID remains configurable for isolated networks. Supported values range from **1 through 9223372036854775807** (`MaxInt64`), matching the configuration's signed 64-bit representation. Zero and negative IDs are unsupported.

Genesis flag validation and both genesis builders reject zero and values above `MaxInt64` before conversion or output. Negative CLI values are rejected by the unsigned flag parser. Configuration import and direct server startup reject nonpositive IDs before runtime initialization; values above `MaxInt64` cannot be decoded into the signed configuration field.

Omitting the genesis CLI option selects the default **1982**. Omitting `chainID` from a supplied configuration, explicitly setting it to `null`, or setting it to zero returns an error without substituting that default. Archive, backup and coordinated-recovery workflows retain their positive-ID requirement.

<!-- FINISH ALPHA CHANGE 3547 -- document the positive chain ID policy at generation and startup -->

The new default applies only when generating a genesis without an explicit chain-ID argument. It does not rewrite existing genesis files, migrate existing databases or change frozen historical fixtures.

Changing the chain ID of an established production chain is not a block-time upgrade: it changes transaction-signing and consensus-checkpoint domains and is therefore not a supported live-chain operation.

<!-- FINISH ALPHA CHANGE 3056 -- document the INNBC genesis default and supported chain ID range -->

Fixed system-contract addresses are not serialized as configurable INNBCBFT fields. Genesis deployment and runtime consensus use the canonical addresses defined by the node source, preventing generated configuration from disagreeing with the deployed system-contract layout.

<!-- FINISH ALPHA CHANGE 1916 -- document genesis generation and validation guarantees -->

## Architecture

| Component | Purpose |
| --- | --- |
| INNBCBFT | Finalizes blocks using validator voting, BLS identities, deterministic proposer selection, and epoch-based validator updates |
| Validator Registry | Stores validator identities, BLS keys, libp2p peer IDs, stake, whitelist status, and activation state |
| Validator Governance | Allows eligible validators to manage validator admission and registry policy |
| Block Reward Governance | Controls protocol reward and fee-distribution settings |
| Community Reward Pool | Receives the governance-configured community portion of protocol fees |
| Fee Burn Vault | Receives the governance-configured burned portion of protocol fees |
| Treasury DAO | Provides validator-governed treasury management |
| Proxy Governance | Controls upgrades of approved system-contract proxies through an isolated executor |
| Native INNBC Token | Exposes the native gas token through an ERC-20-compatible contract interface |
| Peer Directory | Authenticates validators and distributes their current libp2p addresses |
| MDBX Storage | <!-- START ALPHA CHANGE 3080 -- distinguish blockchain and trie configuration from consensus storage -->Stores blockchain and trie data with a shared configurable allowance; consensus uses a separate MDBX allowance<!-- FINISH ALPHA CHANGE 3080 -- distinguish blockchain and trie configuration from consensus storage --> |
| Segmented History | Physically removes expired recent-history directories without rewriting the active database |
| Snapshot Builder | Creates validated segmented or checkpoint `chain-data` artifacts through an on-demand command |
| History-Aware Sync | Selects only peers that advertise the next required full-block range |
| JSON-RPC | Provides Ethereum-compatible HTTP and WebSocket APIs with bounded public-history queries |
| System gRPC | <!-- START ALPHA CHANGE 3383 -- remove the retired archive export RPC from the architecture summary -->Provides local operator functions such as status, peer management, block-event subscriptions, and peer-directory refresh<!-- FINISH ALPHA CHANGE 3383 -- remove the retired archive export RPC from the architecture summary --> |

## Supply Governance Policy

INNBC Chain is designed for zero routine supply inflation: ordinary validator rewards come from transaction fees paid in native INNBC, without creating additional currency. Supply remains unchanged by fee redistribution and decreases when tokens are burned.

Governance controls how transaction base fees support the network:

- **Validator rewards:** Validators receive transaction tips and the remaining base fees after the allocations below.
- **Fee burning:** Governance can direct up to 100% of base fees to the `INNBCFeeBurnVault`. Anyone can trigger the vault's burn operation, permanently reducing total supply.
- **Community rewards:** Governance can allocate up to 50% of base fees to the `INNBCCommunityRewardPool`, where users stake native INNBC to share in fee-funded rewards. These rewards do not require new issuance.

Burn and community allocations cannot exceed 100% of base fees combined. Normal operation therefore supports stable or decreasing supply, depending on the configured allocations and completed burns.

Exceptional validator reward issuance is intended only for critical economic emergencies outside normal operation, where insufficient validator resources could threaten continued chain operation. It requires unanimous vote approval of the effective governance electorate, currently permits 1%, 2% or 3% issuance, and enforces a 1,296,000-block cooldown between successful issuances. These dynamics are regulated by the governance protocol contracts.

<!-- START ALPHA CHANGE EXPLORER-058 -- introduce the custom INNBC Chain explorer -->

## INNBCScan — Custom INNBC Chain Explorer

[INNBCScan](https://www.innbcscan.io/) is a custom blockchain explorer built from scratch specifically for INNBC Chain. Its design prioritizes a focused interface, lightweight deployment, and bounded node requests, providing a purpose-built alternative to adapting a larger general-purpose explorer stack.

<!-- START ALPHA CHANGE EXPLORER-300 -- highlight EIP-3091 explorer compatibility -->
**EIP-3091 compatible:** INNBCScan supports standard explorer links for transactions (`/tx/<hash>`), accounts and contracts (`/address/<address>`), blocks (`/block/<number-or-hash>`), and tokens (`/token/<address>`). Token links redirect to the corresponding address page. These standard URL formats let compatible wallets and decentralized applications link directly to the relevant explorer view. See the [EIP-3091 specification](https://eips.ethereum.org/EIPS/eip-3091).
<!-- FINISH ALPHA CHANGE EXPLORER-300 -- highlight EIP-3091 explorer compatibility -->

The interface uses plain HTML, CSS, and browser JavaScript. Its current functionality includes:

- transaction lookup by hash, including status, sender, recipient, value, gas usage, transaction fees, receipt logs, and raw RPC data;
<!-- START ALPHA CHANGE EXPLORER-095 -- describe curated governance explorer pages -->
- address lookup, including native INNBC balance, account nonce, and deployed contract bytecode;
- curated governance and native-token pages with recorded source code, compiler metadata, ABI-generated read/write forms, and injected-wallet interaction;
- a scrollable governance selector displaying contract names and opening their addresses directly;
- supporting-contract Code/Read pages and source-file access to imported libraries and interfaces;
- runtime-bytecode comparison against the packaged contract release before contract interaction;
<!-- START ALPHA CHANGE EXPLORER-301 -- document block lookup and standard explorer links -->
- block lookup by decimal number or block hash, with header details and links to available transactions;
- EIP-3091-compatible transaction, address, block, and token URLs, including curated contract-section links;
<!-- FINISH ALPHA CHANGE EXPLORER-301 -- document block lookup and standard explorer links -->
<!-- FINISH ALPHA CHANGE EXPLORER-095 -- describe curated governance explorer pages -->
- exact gas-price display in wei or Gwei alongside its equivalent in INNBC;
- a latest-block indicator with an explicit data-refresh age.

<!-- START ALPHA CHANGE EXPLORER-302 -- document on-demand block requests -->
Public gas-price and block-number displays read a shared JSON file generated by a small Node.js publisher. During normal operation, the publisher reads the block number every 30 seconds and the suggested gas price every 60 seconds, independently of visitor count. Individual transaction, address, and block searches use on-demand RPC requests. Each block lookup validates the chain ID and fetches one block, without polling or requesting individual transaction receipts.
<!-- FINISH ALPHA CHANGE EXPLORER-302 -- document on-demand block requests -->

<!-- START ALPHA CHANGE EXPLORER-307 -- separate explorer sources and operations from the chain repository -->

INNBCScan is maintained in the separate [INNBCscan repository](https://github.com/InnovativeBioresearch/INNBCscan). Its application source, build tooling, and installation packages are not included in this chain repository.

The explorer is built and deployed independently from the blockchain client. Running `make nodeclient` does not build or install INNBCScan. The explorer does not require validator identities or direct access to chain databases.

See the [INNBCScan build, installation, and operations guide](https://github.com/InnovativeBioresearch/INNBCscan/blob/main/README.md) for its architecture, directory layout, Nginx requirements, service ownership, deployment commands, and update procedures.

<!-- FINISH ALPHA CHANGE EXPLORER-307 -- separate explorer sources and operations from the chain repository -->

<!-- FINISH ALPHA CHANGE EXPLORER-058 -- introduce the custom INNBC Chain explorer -->

## Major Enhancements Over Polygon Edge

### 1. Sovereign Chain Architecture

INNBC Chain removes legacy components that were not applicable to an independent network, including:

- Polygon rootchain and child-chain coupling
- bridge and state-sync infrastructure
- child-token and exit-proof paths
- SupernetManager integration
- legacy StakeManager integration
- burn-contract configuration
- legacy IBFT and development consensus engines
- legacy bootnode discovery
- total-difficulty fork choice
- unused bridge JSON-RPC namespaces
- obsolete rootchain deployment commands and documentation

INNBCBFT is the only supported runtime consensus engine. This reduces dead code, removes obsolete configuration paths, and makes the operational model specific to INNBC Chain.

### 2. Validator-Gated Proof-of-Stake Membership

INNBC Chain uses an on-chain validator registry as the authoritative source of validator eligibility after its configured activation epoch.

Registry records include:

- validator operator address
- reward address
- BLS public key
- libp2p peer ID
- deposited stake
- registration status
- whitelist status

<!-- START ALPHA CHANGE 2652 -- distinguish registry governance from the fixed genesis stake floor -->

A validator must be registered, whitelisted, and satisfy the registry contract's current minimum stake. Genesis configuration must initialize that minimum to exactly 1 wei. Governance may subsequently raise or lower the live contract minimum, which must remain strictly positive.

Consensus retains the fixed 1-wei genesis floor as an additional eligibility check. Every validator eligible under the contract's current minimum also satisfies that floor. Registry validators currently receive equal consensus voting power, while stake acts as an eligibility requirement rather than directly multiplying voting power.

<!-- FINISH ALPHA CHANGE 2652 -- distinguish registry governance from the fixed genesis stake floor -->

The registry transition is epoch-aware. Genesis validators remain authoritative until registry activation, preventing an uninitialized registry from unexpectedly replacing the initial validator set.

An empty authoritative validator set is treated as a fatal configuration or governance condition rather than allowing consensus to continue without eligible validators.

### 3. Validator Governance

INNBC Chain adds validator-governed system contracts for:

- admitting validators
- removing validators
- updating minimum stake
- managing validator-related policy
- controlling block reward and fee-distribution settings
- managing the community reward pool
- administering the treasury
- approving system-contract proxy upgrades

Upgrade execution is separated from governance voting through a dedicated proxy-admin executor. This avoids using an externally owned account as the permanent administrator of critical protocol proxies.

### 4. Deterministic INNBCBFT Consensus

INNBCBFT uses deterministic chain-derived state for validator selection, proposer calculation, epoch transitions, proposal construction, and block validation.

Important consensus improvements include:

- fixed INNBCBFT header difficulty
- deterministic chain-time timestamps
- deterministic proposer-priority calculations
- deep-copied proposer snapshots
- numeric proposer-priority comparisons
- guarded validator snapshot lookup
- stronger direct proposal validation
- synchronized blockchain/runtime advancement
- fatal recovery for unrecoverable consensus-state failures

<!-- START ALPHA CHANGE 3324 -- document the enforced chain-derived clock -->

<!-- START ALPHA CHANGE 3827 -- describe the current single-interval timestamp rule -->

The initial block interval is configured in genesis rather than hard-coded to two seconds. Current production uses `2s`. The current software applies one configured whole-second interval to block construction and timestamp validation: each non-genesis timestamp must equal its parent's timestamp plus that interval.

Historical interval selection is not currently implemented. Changing the interval after a chain has started therefore requires the future protocol-upgrade work described in point 52.

<!-- FINISH ALPHA CHANGE 3827 -- describe the current single-interval timestamp rule -->

INNBC timestamps measure chain progression. They do not promise correspondence with real-world time. Production delays and outages do not advance the chain clock. Timestamp-dependent contract deadlines follow this chain clock.

Production pacing, consensus timeouts and network timers remain operational timers; they do not select block timestamps or determine timestamp validity.

<!-- FINISH ALPHA CHANGE 3324 -- document the enforced chain-derived clock -->

### 5. Hardened Proposal Validation

The direct INNBCBFT proposal path no longer trusts proposal-controlled header fields merely because the proposal has passed through consensus messaging.

Before execution, validators verify:

- the expected parent
- exact parent-derived timestamp equality using the configured block interval <!-- ALPHA CHANGE 3324 -->
- governance-aware base fee
- gas-limit transition
- fixed difficulty
- empty uncle root
- transaction root
- consensus extra-data structure

Transactions are executed once. The locally reconstructed result is then compared with the received proposal, including:

- state root
- gas used
- receipts root
- logs bloom
- transaction root
- uncle root
- final block hash

Gas-limit validation uses safe unsigned arithmetic, avoiding overflow and underflow at extreme values.

This closes an inherited validation gap where the direct proposal path was weaker than finalized-block synchronization verification.

### 6. Synchronized Consensus Runtime Advancement

INNBC Chain fixes a legacy synchronization race in which a raw blockchain event could cancel and restart consensus before the consensus runtime had processed the synchronized block.

The old sequence could produce:

```text
syncer writes block
→ blockchain event cancels current consensus sequence
→ next sequence starts
→ consensus runtime is still one block behind
→ validator backend rejects the new FSM height
→ immediate retries consume CPU and flood logs
```

The corrected sequence is:

```text
syncer writes and verifies the block
→ consensus runtime processes the block
→ critical consensus metadata commits
→ in-memory runtime state is published
→ runtime-applied notification reaches consensus
→ current sequence is cancelled
→ next FSM is constructed from the committed parent
```

Before constructing an FSM, consensus now requires the runtime head and blockchain head to match by both block number and block hash.

FSM construction verifies that its parent remained stable and returns its actual sequence height. Consensus starts exactly that height rather than combining heights obtained independently from different components.

Temporary alignment failures wait with bounded backoff and throttled logging instead of entering a tight retry loop.

### 7. Transactional Consensus-State Publication

Critical consensus metadata is processed through an MDBX transaction.

Proposer-priority changes are calculated on a deep copy. The staged proposer snapshot is written through the caller-owned transaction, and live in-memory state is updated only after the transaction commits successfully.

The following state is therefore published together only after successful persistence:

- runtime head
- current epoch
- proposer snapshot
- validator-related cached state

Non-authoritative runtime side effects, such as txpool cleanup and metrics, are not treated as persisted consensus authority.

If the blockchain has advanced but critical consensus metadata cannot be advanced safely, the validator stops rather than voting with stale state.

### 8. Fatal Consensus Recovery

Unrecoverable runtime advancement failures are propagated beyond the consensus goroutine to the server process owner.

The failure path is:

```text
critical consensus advancement fails
→ synchronization and consensus stop
→ server performs orderly shutdown
→ process exits with a nonzero status
→ the service supervisor restarts the node according to policy
```

Normal duplicate blocks and temporary runtime-alignment delays do not trigger fatal shutdown.

Fatal shutdown is reserved for conditions such as:

- consensus database transaction failure
- persistent storage or filesystem errors
- invalid critical consensus-state transitions
- unrecoverable proposer or validator-state updates
- internal errors that make continued voting unsafe

### 9. Deterministic Canonical-Chain Storage

INNBCBFT provides finality, so INNBC Chain removes the legacy proof-of-work-style total-difficulty fork-choice model.

The blockchain layer now:

- accepts direct canonical descendants
- removes separate total-difficulty storage and caches
- removes persisted fork-head lists
- removes total-difficulty reorganization logic
- updates the canonical head only after a successful batch write
- keeps `Header.Difficulty` for Ethereum compatibility
- requires the fixed INNBCBFT difficulty value

This removes a total-difficulty database record for every block and simplifies canonical-chain ownership around BFT finality.

### 10. Lossless Compact MDBX Storage

Blockchain, immutable-trie, and INNBCBFT consensus persistence use MDBX.

<!-- START ALPHA CHANGE 3080 -- document full-history and recent MDBX allowances -->
Full-history nodes default to 1024 GiB (1 TiB) for the blockchain database and the trie database independently:

```text
INNBC_MDBX_MAP_SIZE_GB=1024
# START ALPHA CHANGE 3504 -- document node database page size --
INNBC_MDBX_PAGE_SIZE_BYTES=4096
# FINISH ALPHA CHANGE 3504 -- document node database page size --
```

Generated recent-history nodes explicitly retain the 128 GiB setting:

```text
INNBC_MDBX_MAP_SIZE_GB=128
# START ALPHA CHANGE 3504 -- document recent database page size --
INNBC_MDBX_PAGE_SIZE_BYTES=4096
# FINISH ALPHA CHANGE 3504 -- document recent database page size --
```

In recent mode, the blockchain allowance is divided across the configured retained segments, subject to the existing per-segment minimum. The permanent genesis database also uses the per-segment allowance. The trie receives the configured node allowance independently.

Consensus persistence uses its separate 1 GiB MDBX allowance; `INNBC_MDBX_MAP_SIZE_GB` does not configure it.

These are configured database growth allowances, not upfront physical disk allocations. Setting 1024 GiB does not immediately allocate a 1 TiB data file.
<!-- FINISH ALPHA CHANGE 3080 -- document full-history and recent MDBX allowances -->

#### Compact header encoding

Canonical block headers contain several fields whose values are frequently fixed or already implied by consensus commitments. Storing those complete values in every database record would repeat hundreds of bytes per block.

INNBC Chain therefore uses a dedicated lossless header representation for internal database storage. A compact flags field indicates which deterministic values were omitted.

The codec can omit:

- the empty uncle hash;
- the empty transaction root;
- the empty receipt root;
- an all-zero logs bloom;
- the fixed INNBCBFT difficulty value of `1`;
- zero gas usage;
- the fixed INNBCBFT mix digest;
- a zero mix hash where applicable;
- a zero nonce.

The fields that identify the particular block remain stored, including:

- parent hash;
- miner address;
- state root;
- block number;
- gas limit;
- timestamp;
- INNBCBFT `ExtraData`;
- base fee;
- every non-default conditional field.

On every database read, the omitted deterministic values are reconstructed to produce the complete ordinary header. Its canonical hash is recomputed and compared with the hash used as the database key. A mismatch is treated as corrupted or inconsistent storage.

The canonical header format used for hashing, consensus, networking, archives, and RPC responses is unchanged. Compact encoding is strictly an internal storage optimization.

This optimization is not limited to empty blocks. Non-empty blocks also benefit whenever fields such as the uncle hash, difficulty, mix digest, or nonce contain their normal INNBCBFT values.

For a typical empty INNBCBFT block, the compact representation avoids approximately 400 bytes of repeated encoded header fields. At a two-second block period—about 15.8 million blocks per year—this represents roughly 6 GB per year of raw header payload before MDBX page and key overhead are considered.

#### Omission of empty body and receipt records

An empty block header already commits to:

```text
TxRoot       = EmptyRootHash
Sha3Uncles   = EmptyUncleHash
ReceiptsRoot = EmptyRootHash
```

Consequently, storing separate encoded empty body and empty receipt records would add no information.

When a block contains neither transactions nor uncles, its body record is not written. When its receipt list is empty, its receipt record is not written.

The read path distinguishes intentional omission from unavailable data by consulting the hash-verified header:

```text
body record missing
+ header commits to no transactions and no uncles
→ reconstruct an empty body

body record missing
+ header commits to a non-empty body
→ return a missing-data error

receipt record missing
+ header commits to an empty receipt root
→ reconstruct an empty receipt list

receipt record missing
+ header commits to non-empty receipts
→ return a missing-data error
```

This eliminates two redundant hash-keyed database records for every empty block. At a two-second block period, merely avoiding their repeated keys and encoded empty values removes approximately another gigabyte of raw key/value material per year, before accounting for MDBX record and page overhead.

Together, compact headers and empty-record omission reduce empty-block storage by several gigabytes per year while preserving exactly the same:

- canonical block hash;
- reconstructed header;
- empty body;
- empty receipt list;
- consensus behavior;
- networking representation;
- JSON-RPC result.

These optimizations are used by both full and recent-history nodes. Recent nodes obtain the same per-block compression and additionally reclaim entire expired segment directories.

### 11. Physically Reclaimable Recent History

INNBC Chain supports `full` and `recent` history modes.

Full mode keeps ordinary canonical history in one continuous blockchain MDBX database.

Recent mode keeps genesis permanently and writes post-genesis blockchain history into block-range MDBX directories. The default geometry is one-hour segments with approximately 24 hours retained.

A closed segment contains every blockchain record for its range. When it expires, the complete directory is physically removed. The active database is never compacted, rewritten, or swapped merely to reclaim old history.

This turns history retention into safe directory-level garbage collection:

```text
write current blocks into active segment
→ close the segment when its range completes
→ continue in a new segment
→ retire an expired closed segment
→ return its physical storage to the filesystem
```

The trie, consensus state, genesis database, and node identities remain separate.

### 12. Independent Snapshot Creation

Snapshot creation is an explicit command:

<!-- START ALPHA CHANGE 1829 -- use canonical segmented snapshot command in project documentation -->
```bash
innbc-chain snapshot build segmented
innbc-chain snapshot build checkpoint
```
<!-- FINISH ALPHA CHANGE 1829 -- use canonical segmented snapshot command in project documentation -->

<!-- START ALPHA CHANGE 2049 -- document internal snapshot trust model and exact validation guarantees -->
<!-- START ALPHA CHANGE 3291 -- describe synchronized snapshot construction and publication -->
It is not an in-node producer loop. The builder independently opens an operator-controlled, genesis-origin full-history node’s MDBX databases for read-only access, constructs a complete artifact, validates it, and synchronizes its files and directories before atomic publication. The latest pointer is durably published before an eligible previous snapshot is retired. The builder then exits. The publication ordering and regression coverage are documented in point 94.
<!-- FINISH ALPHA CHANGE 3291 -- describe synchronized snapshot construction and publication -->

Segmented snapshots use the recent node’s native segmented blockchain layout. Checkpoint snapshots use the standard single-database layout for Plan B recovery.

Both contain:

- the persisted genesis header and its stored state-root commitment;
- a complete retained canonical block suffix;
- blockchain head metadata;
- the trie reachable from the checkpoint state root;
- the corresponding consensus checkpoint state;
- a manifest and atomic latest pointer.

No genesis-to-head `headers.rlp.hex` file is created. Copying only the reachable checkpoint trie also removes unreachable historical trie data from the produced artifact.

These snapshots are an internal storage-optimization and recovery mechanism. Their trusted source is an existing full-history node operated within the same controlled INNBC deployment. They are not designed as publicly downloadable or independently authenticated chain-bootstrap artifacts.

### 13. Validated Snapshot Activation

Snapshots are staged and validated before their `chain-data` directory is activated.

Validation includes:

- format and snapshot-kind checks;
- manifest/latest-pointer consistency;
- compatibility with the configured genesis header fields;
- consistency between the manifest, persisted genesis header, and persisted genesis state root;
- checkpoint block hash and state-root checks;
- exact retained-range validation;
- canonical parent continuity;
- body, transaction-root, uncle-root, receipt-root, and transaction-lookup checks;
- trie-root verification;
- consensus checkpoint consistency.

The configured genesis allocations are not re-executed during snapshot activation. The snapshot supplies the persisted genesis state root, and validation confirms that the stored genesis header, manifest, and configured non-state genesis fields are mutually consistent. These checks detect incomplete copies, corruption, structural inconsistencies, and incompatible artifacts; they do not independently authenticate the provenance of a deliberately fabricated but internally consistent snapshot.

Snapshot provenance therefore depends on operational control of the full-history source node, snapshot-builder process, and snapshot output directory. Operators must not activate snapshots obtained from public or untrusted sources. If externally distributed snapshots are supported in the future, they will require a separately trusted checkpoint hash, verification key, or equivalent trust anchor that is not supplied by the snapshot itself.

Recent nodes consume internally produced segmented snapshots. Full nodes consume internally produced checkpoint snapshots only when Plan B bootstrap is explicitly required and no usable local chain exists.

Interrupted activation is recoverable. Node identities remain outside `chain-data` and are never replaced by snapshot activation.
<!-- FINISH ALPHA CHANGE 2049 -- document internal snapshot trust model and exact validation guarantees -->

### 14. Authenticated Validator Peer Directory

Legacy bootnode discovery has been replaced with an authenticated validator peer-directory model.

Each exchange request includes:

- libp2p peer ID
- advertised multiaddress
- libp2p public key
- timestamp
- nonce
- signature

The peer-directory service verifies:

- request method
- body-size limit
- timestamp freshness
- nonce uniqueness
- public-key/peer-ID correspondence
- multiaddress/peer-ID correspondence
- request signature
- validator registry eligibility

The directory stores one current multiaddress per libp2p peer ID. A new address for the same identity replaces the previous address.

Every exchange uses one consistent registry allowlist snapshot. Stored entries for validators no longer present in that snapshot are removed.

### 15. Validator-Only Network Admission

The node maintains a validator peer allowlist and enforces it across multiple networking stages:

- outbound dial selection
- inbound connection handling
- identity handshake
- discovery requests
- discovered-peer storage
- peer information returned to other validators

This prevents an unauthorized peer from being temporarily accepted merely because it reached a later stage of the networking pipeline.

Peer-directory responses are fully parsed and validated before replacing the local allowlist. Duplicate peer IDs are accepted defensively, with the later address winning. Updated peer IDs have their old peerstore addresses removed before the new address is installed.

Connected peers removed from the allowlist are disconnected.

Peer-directory request and response bodies are bounded to prevent unbounded memory use.

Peer-directory refresh is intentionally one-shot at startup plus manually triggered refresh. Operators should refresh running validators after validator additions, removals, whitelist changes, peer-address changes, or peer-directory startup races.

### 16. Non-Blocking Blockchain Event Delivery

Slow event subscribers can no longer block canonical block insertion or consensus progress.

Blockchain events use bounded, best-effort delivery. If a subscriber does not drain its small buffer, that subscriber can miss an event, but it cannot stop block commitment.

Consumers that require authoritative state must re-read the current canonical state rather than treating the event stream as a durable queue.

This protects consensus from stalled:

- JSON-RPC filters
- WebSocket subscriptions
- gRPC streams
- pruning listeners
- snapshot listeners
- internal monitoring consumers

### 17. History-Aware Hardened Synchronization

Every node advertises its finalized head and the earliest block in its continuous full-block history.

The synchronization subsystem:

- filters out peers that cannot serve the next requested block;
- prefers the highest finalized head among eligible peers;
- retains the existing network-distance tie-break;
- immediately evaluates already-known peers;
- tries another eligible peer after a partial or terminal stream failure;
- preserves successfully verified and committed partial progress;
- retries after a peer-status update or a five-second bounded interval;
- propagates stream cancellation and terminal results explicitly;
- keeps block and terminal delivery ordered;
- exits cleanly during node shutdown;
- tracks and joins background workers.

The same selection logic applies to full and recent nodes. A genesis or historical request therefore selects a peer advertising the required range rather than probing recent nodes that have already retired it.

A synchronized block is verified before insertion. This changes only finalized-block download routing; INNBCBFT voting and finalization remain unchanged.

### 18. Governance-Controlled Fee Distribution

INNBC Chain reads protocol fee settings from explicit governance-controlled storage slots.

For each block, the base-fee portion can be divided between:

- fee burn vault
- community reward pool
- block validator

The transaction priority fee remains assigned to the validator.

The executor reads and validates the applicable governance settings once per block and carries them through the transaction context. Invalid combined fee percentages are rejected.

### 19. Governance-Controlled Maximum Base Fee

Validators can govern an optional maximum EIP-1559 base fee through the block reward governance contract.

A value of `0` disables the ceiling and leaves the base fee fully dynamic. A nonzero value limits base-fee growth while preserving normal adjustment below the cap.

The expected governance-aware base fee is validated by both synchronized-block verification and direct consensus proposal validation.

The base-fee calculation also includes:

- a one-wei minimum floor
- overflow-safe growth arithmetic
- validation against invalid gas targets
- parent-state governance lookup

### 20. Corrected London Fee Validation

The inherited London execution path contained a zero-cap dynamic-fee exception. A transaction with both:

```text
maxFeePerGas = 0
maxPriorityFeePerGas = 0
```

could bypass the normal requirement that `maxFeePerGas` cover the active base fee.

INNBC Chain removes that exception. EIP-1559 transactions must satisfy the base-fee requirement whenever London rules are active.

The effective tip calculation was also corrected to avoid double-counting the base fee.

### 21. Bounded And Chain-Native Gas Recommendations

Public fee recommendation no longer performs an unbounded historical search.

`eth_maxPriorityFeePerGas` uses the current floored base fee as the default INNBC priority-fee recommendation. This is constant-time and congestion-aware without scanning older blocks.

For London transactions, `eth_gasPrice` uses:

```text
base fee + recommended priority fee
```

The addition is overflow-safe and saturates at `math.MaxUint64` instead of wrapping to an unexpectedly small value.

`eth_feeHistory` remains available for clients that explicitly request historical fee data, but its block count is capped by configuration.

### 22. Bounded Public JSON-RPC History Windows

Expensive public RPC history windows use conservative defaults.

The default maximum is 256 blocks for:

- log-range queries
- `eth_feeHistory`

These limits reduce worst-case CPU, memory, and storage pressure from public requests while remaining configurable for private indexers or dedicated RPC infrastructure.

### 23. Ethereum-Compatible Receipt Output

JSON-RPC receipts were aligned more closely with Ethereum/geth behavior.

Receipt output now includes:

- transaction type
- effective gas price
- correct log metadata
- mutually exclusive `status` and legacy `root` fields

A receipt emits the post-Byzantium status or the legacy state root, but never both.

### 24. Public JSON-RPC Surface Reduction

Operator-only and removed bridge namespaces are not registered on the public JSON-RPC dispatcher.

<!-- START ALPHA CHANGE 3406 -- distinguish snapshot construction commands from system operator RPCs -->
Ordinary segmented and checkpoint snapshots use the shared `snapshotbuilder.Build` implementation. Operators can invoke it directly through `innbc-chain snapshot build`, or through the packaged `innbc-snapshot-builder` wrapper, which loads its configuration and executes that same CLI command as the `innbc` user.

Peer-directory refresh is a separate local system gRPC operation invoked through its operator command. Neither operation is exposed through the public Ethereum JSON-RPC interface.
<!-- FINISH ALPHA CHANGE 3406 -- distinguish snapshot construction commands from system operator RPCs -->

Deployments should still place public JSON-RPC behind an appropriately configured reverse proxy with TLS, request-size limits, request-rate limits, and timeouts.

### 25. Deterministic And Dependency-Safe Shutdown

The legacy shutdown sequence could close blockchain or state storage while consensus, synchronization, txpool, RPC, or storage-maintenance consumers were still using those resources.

INNBC Chain now:

1. Stops external HTTP, WebSocket, gRPC, and monitoring intake.
2. Prevents new component work from being admitted.
3. Waits for admitted work to finish.
4. Stops consensus and synchronization.
5. Stops txpool processing.
6. Closes networking.
7. Closes blockchain storage, including recent-history segments.
8. Closes state-trie storage.
9. Releases profiler and remaining runtime resources.

Shutdown is idempotent across:

- server
- INNBCBFT
- syncer
- sync peer client
- txpool
- network server
- discovery
- dial queue
- event subscriptions
- JSON-RPC
- filter manager
- consensus state database

Cleanup continues on a best-effort basis when an individual close operation returns an error.

### 26. Joined JSON-RPC And WebSocket Lifecycle

The JSON-RPC server retains ownership of its HTTP server and listener.

Shutdown now:

- rejects new handlers
- gracefully shuts down normal HTTP requests
- force-closes the listener if graceful shutdown races startup
- closes upgraded WebSocket connections
- waits for WebSocket request workers
- removes WebSocket-owned filters
- closes the dispatcher
- closes and joins the filter manager
- unsubscribes blockchain and txpool event streams

This prevents leaked listeners, filters, subscriptions, and WebSocket goroutines during restart.

### 27. Production Node Packaging

The repository includes production-oriented packaging for:

- the `innbc-chain` binary
- validator node systemd services
- environment-file configuration
- validator identity preparation
- node start, stop, restart, enable, disable, and status commands
- public validator registration information
- the standalone peer-directory service
- configurable history and storage profiles
- manual snapshot creation
- manual peer-directory refresh

Default packaged service settings bind JSON-RPC, gRPC, libp2p, and peer-directory communication to explicitly configured addresses. Operators are responsible for firewall and reverse-proxy policy.

### 28. Modernized Toolchain And Testing

The module targets Go 1.27.2 and uses refreshed gRPC, protobuf, and Go tooling dependencies.

The repository includes:

- unit tests
- consensus tests
- storage tests
- JSON-RPC tests
- synchronization tests
- shutdown regression tests
- fuzz tests
- INNBCBFT end-to-end tests
- property tests
- race-detector support

Passing tests do not replace an independent consensus, economic, smart-contract, and infrastructure audit.

### 29. Structurally Validated Consensus Message Admission

INNBC Chain hardens the inherited IBFT message boundary against malformed messages from an admitted but faulty or compromised validator.

Before an inbound message enters the IBFT message queues, the node verifies structural invariants that every legitimate consensus message must satisfy:

- the message and consensus view exist;
- the message type is one of `PREPREPARE`, `PREPARE`, `COMMIT`, or `ROUND_CHANGE`;
- the protobuf payload variant matches the declared message type;
- pre-prepare messages contain a non-empty proposal;
- prepared certificates contain the required proposal and prepare messages;
- nested round-change and prepared-certificate messages have valid structural forms.

Nil proposals and committed seals are rejected safely rather than being dereferenced.

This closes inherited panic paths where a correctly signed but malformed message could reach code that assumed its view, type, payload, proposal, or certificate fields were already present. Such a message could previously terminate a validator process instead of being discarded.

The admission checks are deliberately structural. They do not change:

- proposer selection;
- validator membership;
- block validation;
- quorum calculation;
- round-change semantics;
- valid proposal acceptance.

Rejected messages increment one fixed-cardinality consensus counter. They do not generate one warning or error log per message and do not attach peer IDs, heights, rounds, or attacker-controlled text to metric labels. This preserves operational visibility without allowing hostile traffic to amplify disk logs or create unbounded metric cardinality.

<!-- START ALPHA CHANGE 1899 -- document bounded and catch-up-aware INNBCBFT consensus -->
### 30. Bounded And Catch-Up-Aware INNBCBFT Consensus

INNBC Chain substantially refactors the inherited IBFT consensus engine to place its implementation, wire protocol, message storage, and round-recovery behavior under direct INNBC ownership.

The consensus implementation now resides entirely in local packages:

```text
consensus/innbcbft/ibftcore
→ consensus state machine, quorum handling, rounds, and timeouts

consensus/innbcbft/ibftmessages
→ bounded consensus-message storage and subscriptions

consensus/innbcbft/ibftproto
→ locally owned consensus wire types and protobuf schema
```

The runtime no longer imports or depends on the external `github.com/0xPolygon/go-ibft` module. The IBFT protobuf source is maintained locally and can be regenerated deterministically with:

```bash
make protoc-ibft
```

This is a substantial architectural refactor rather than only an import-path change. It allows INNBC Chain to define and maintain its own consensus admission policy, memory bounds, round synchronization, certificate validation, recovery behavior, and tests without carrying an opaque runtime dependency on the original legacy implementation.

#### Bounded Consensus Heights

The inherited implementation could accept authenticated consensus messages for any height equal to or greater than the current height. Those messages were stored even when they referred to a block height that the node could not yet process.

A faulty or compromised validator could therefore send correctly signed messages for increasingly distant future heights:

```text
local consensus height = H

received messages:
H + 10
H + 100
H + 1,000
H + 1,000,000
```

Because future-height message buckets were not removed by ordinary old-height pruning, the in-memory message maps could grow without a meaningful upper bound.

INNBC Chain replaces that open-ended policy with a strict height window:

```text
current height H
→ accepted

next height H + 1
→ only round 0 may be admitted

height above H + 1
→ rejected before signature verification and storage

height below H
→ rejected as stale
```

Allowing round-zero material for `H + 1` preserves a small amount of useful network overlap while preventing validators from filling memory with messages for nonexistent distant blocks.

The restriction applies to consensus gossip, not transactions. Transactions remain in the transaction pool until normal proposal selection includes them in a block. A rejected message for a distant future consensus view does not discard a transfer, contract call, or other user transaction.

Consensus messages are temporary coordination data. If a distant view later becomes real, honest validators construct and broadcast the appropriate messages again using the then-current chain state.

#### Bounded Future Rounds

Multiple IBFT rounds can legitimately exist at the same block height. A new round is required when the current proposer is unavailable, cannot construct a valid proposal, or the validators do not assemble sufficient prepare or commit voting power before the round timeout.

Rounds must therefore remain capable of advancing, but a validator does not need to retain messages for every imaginable future round.

For a node currently processing:

```text
height = H
round  = R
```

INNBC Chain accepts only:

```text
H at rounds R through R + 8
H + 1 at round 0
```

Messages below the active round are stale. Messages more than eight rounds ahead are outside the useful recovery window and are rejected before cryptographic signature verification or insertion into the message store.

This creates a bounded working set while preserving enough room for validators that temporarily progress at different speeds.

The eight-round window is measured relative to the node’s current round. It moves forward as the node adopts newer authenticated round evidence:

```text
current round 1
→ admissible through round 9

node safely advances to round 6
→ admissible through round 14
```

The policy therefore bounds memory without imposing a permanent maximum consensus round. A chain can continue beyond round 8 when recovery genuinely requires it; it advances through a moving bounded window instead of allocating storage for arbitrary distant rounds.

#### Authenticated Round Catch-Up

Bounding future rounds alone would prevent memory exhaustion, but it would not solve the operational problem where validators at the same block height become separated by several rounds.

For example:

```text
node1:
height 4321
round  6

node2:
height 4321
round  1
```

The legacy behavior could force node2 to wait through the timeout for rounds 1, 2, 3, 4, and 5 before reaching node1. Because IBFT round timeouts grow exponentially, this could turn a temporary disconnection, validator restart, or post-synchronization consensus re-entry into several minutes of avoidable waiting.

INNBC Chain now evaluates authenticated future `ROUND_CHANGE` evidence and can safely advance directly to the highest qualifying round inside the eight-round window.

There are two distinct evidence thresholds:

```text
weak evidence:
strictly more than one third of active voting power

full certificate:
normal INNBCBFT quorum
```

Weak evidence is used only for catch-up. It tells a lagging validator that enough authenticated voting power has already left earlier rounds for continuing to wait there to be unproductive.

When weak evidence is established, the node:

1. verifies that every counted message belongs to an active validator;
2. verifies that every message refers to the same current block height;
3. verifies that the target round is higher and inside the bounded window;
4. validates any attached prepared proposal and prepared certificate;
5. advances directly to the highest qualifying round;
6. broadcasts its own `ROUND_CHANGE` message for that round.

The node never moves backward to an older round.

Weak evidence cannot:

- finalize a block;
- satisfy prepare quorum;
- satisfy commit quorum;
- make a proposal valid;
- override proposer selection;
- change validator voting power;
- replace the normal INNBCBFT finality threshold.

It only allows a validator to stop waiting in a round that sufficient authenticated validator weight has already abandoned.

<!-- START ALPHA CHANGE 3900 -- document live three-validator round catch-up and avoided timeout waiting -->

#### Live Three-Validator Recovery: Approximately Two Hours Of Waiting Avoided

On 12 November 2026, a live INNBC Chain validator-admission test demonstrated authenticated round catch-up during the transition from two to three validators.

Node 3 downloaded and processed historical blocks through height 338790, then entered consensus for height 338791 with the three-validator committee. The existing validators had already advanced to higher rounds while node 3 was synchronizing.

Node 3 initially progressed through its own round timeouts. After entering round 8, it received qualifying round-10 evidence and jumped directly to round 10, skipping round 9. Block 338791 was committed approximately two seconds later, without restarting the validators.

The following excerpt preserves the node's timestamps and messages:

```text
2026-11-12T07:51:07.795Z [INFO]  innbc.server.innbcbft: round started: round=8
2026-11-12T07:57:33.253Z [INFO]  innbc.server.innbcbft: received future round-change evidence: round=10 weakQuorum=true
2026-11-12T07:57:33.256Z [INFO]  innbc.server.innbcbft: round started: round=10
2026-11-12T07:57:35.313Z [INFO]  innbc.blockchain: new block: number=338791 txs=0 hash=0xa5af43fc54e5ec40387069f0f100573ef3d2431f1c9de28dc5c32da06fccaee6 parent=0x7cbef659587d313c03ea8e3cbf2961874049da6c189cbb3c37769ac68c9ff8c6 source=consensus generation_time_in_seconds=2
2026-11-12T07:57:35.324Z [INFO]  innbc.server.innbcbft: sequence done: height=338791
```

The `weakQuorum=true` message identifies the authenticated catch-up path. Node 3 adopted the higher round and broadcast its own round-change contribution. Normal quorum requirements remained in force.

The observed timeout schedule used a ten-second base with exponential growth:

```text
round 8 timeout = 10 × 2^8 = 2560 seconds = 42 minutes 40 seconds
round 9 timeout = 10 × 2^9 = 5120 seconds = 85 minutes 20 seconds
```

Holding that schedule fixed, node 3's timeout-only progression would have reached round 10 at 09:59:07.795 UTC. Instead, authenticated evidence advanced it there at 07:57:33.256 UTC.

All times below refer to 12 November 2026. Italy was on CET, one hour ahead of UTC.

| Event | Basis | UTC | Italy time, CET |
|---|---|---|---|
| Node 3 enters round 8 | Observed | 07:51:07.795 | 08:51:07.795 |
| Node 3 enters round 9 without catch-up | Calculated timeout-only progression | 08:33:47.795 | 09:33:47.795 |
| Node 3 enters round 10 without catch-up | Calculated timeout-only progression | 09:59:07.795 | 10:59:07.795 |
| Node 3 jumps directly from round 8 to round 10 | Observed | 07:57:33.256 | 08:57:33.256 |
| Block 338791 is committed through consensus | Observed | 07:57:35.313 | 08:57:35.313 |

The difference between calculated timeout-only arrival at round 10 and actual arrival was **2 hours, 1 minute, 34.539 seconds**: approximately **two hours of additional round waiting avoided**. The first recovered block was committed **2.057 seconds after adoption of round 10**. Subsequent blocks resumed at round zero, approximately two seconds apart, with node 3 participating as a proposer.

This comparison estimates avoided local timeout waiting from the recorded schedule; it is not a separate experiment with catch-up disabled. The saving depends on the rounds, timeout configuration, and arrival of qualifying evidence.

The test also demonstrates why catch-up need not occur immediately after historical synchronization: the mechanism requires valid higher-round evidence. It accelerates round alignment once that evidence is available; it does not replace block synchronization or relax consensus safety.

<!-- FINISH ALPHA CHANGE 3900 -- document live three-validator round catch-up and avoided timeout waiting -->

#### Two-Validator Recovery

The catch-up rule is particularly useful during the initial two-validator phase.

With two equally weighted validators:

```text
node1 voting power = 50%
node2 voting power = 50%
```

One authenticated validator represents strictly more than one third of total voting power. Therefore, if node1 is already at round 6 while node2 is still at round 1:

```text
node1 sends authenticated ROUND_CHANGE for round 6
→ node2 verifies node1 is an active validator
→ node2 recognizes weak evidence for round 6
→ node2 moves directly from round 1 to round 6
→ node2 broadcasts its own ROUND_CHANGE for round 6
→ both messages together establish full quorum
→ the designated round-6 proposer can proceed
```

This does not give either validator additional consensus authority. Neither validator can finalize a block alone. It removes unnecessary sequential timeout waiting while preserving the requirement that both equally weighted validators cooperate to reach full quorum.

This recovery is relevant after a new validator has downloaded finalized blocks and reached the current chain height. It accelerates alignment with the active consensus round; it does not replace or weaken finalized-block synchronization.

#### Recovery With Larger Validator Sets

The same mechanism scales according to voting power rather than validator count.

For four equally weighted validators:

```text
one validator  = 25%
two validators = 50%
```

One validator alone does not exceed the one-third catch-up threshold. Two validators do.

For weighted validator sets, the calculation uses the actual registered voting power. A collection of low-weight validators cannot manufacture catch-up evidence merely by sending more messages.

Every sender is counted only once for a specific height and round, and unknown or inactive validator addresses contribute no voting power.

If validators are distributed across several rounds, each node evaluates the authenticated evidence available to it and selects the highest qualifying round inside its current window. Validators can advance toward the leading legitimate round, but they cannot be forced backward and cannot jump to an unbounded attacker-selected value.

#### Prepared-Certificate Safety

A `ROUND_CHANGE` message can carry evidence that a proposal was already prepared in an earlier round. This information cannot be discarded merely because a later round has accumulated more messages.

INNBC Chain therefore does not reduce round-change storage to a single largest integer. It retains the certificate material required to preserve IBFT safety and validates it before using the associated round evidence.

Validation includes:

- the outer round-change sender is an active validator;
- the prepared proposal and prepared certificate are either both present or both absent;
- the certificate contains a pre-prepare message;
- the remaining certificate messages are prepare messages;
- certificate senders are unique;
- certificate messages come from active validators;
- the proposal was sent by the correct proposer for its original view;
- prepare messages do not impersonate the proposer;
- proposal hashes agree;
- certificate messages use the same height and round;
- the prepared round is lower than the target round;
- the certificate carries full quorum voting power.

This preserves the highest safely prepared proposal across round changes while rejecting structurally valid but semantically inconsistent certificate data.

#### Highest Qualifying Round Selection

Future-round evidence is evaluated independently for weak and full quorum.

For example:

```text
round 5:
two-validator full quorum

round 6:
one-validator weak evidence
```

The node can identify:

```text
highest full certificate = round 5
highest weak evidence    = round 6
```

The round-6 weak evidence may advance the local retry round and trigger the node’s own round-change broadcast, but it is not silently promoted into a full certificate.

This separation prevents catch-up optimization from being confused with proposal or finality authorization.

#### Round-Aware Message Reclamation

Consensus message storage is now pruned by both height and round.

When the node advances to a new height:

```text
all message buckets below the new height
→ removed
```

When the node advances from round `R` to a later round:

```text
messages below the adopted round at the current height
→ removed

messages for the adopted round and bounded future rounds
→ retained
```

Messages are indexed by sender within each height, round, and message-type bucket, so repeated messages from the same validator replace that validator’s previous entry rather than growing the bucket indefinitely.

The bounded height window, bounded round window, sender-keyed storage, and round-aware pruning together place a deterministic upper boundary on the amount of useful consensus gossip retained in memory.

<!-- START ALPHA CHANGE 1906 -- document bounded consensus CPU utilization -->
#### Consensus CPU And Scheduling Efficiency

The bounded admission policy also reduces unnecessary CPU work. Consensus messages outside the useful height and round windows are rejected before validator-signature verification and before insertion into the message store:

```text
height above H + 1
current-height round above R + 8
H + 1 message above round 0
→ reject using inexpensive view checks
→ do not perform validator-signature recovery
→ do not acquire message-store write locks
→ do not allocate a new height or round bucket
→ do not run quorum evaluation
→ do not notify consensus subscribers
```

Cryptographic signature recovery and semantic certificate validation remain mandatory for messages inside the admissible window. The optimization does not skip security checks for relevant consensus data; it prevents clearly unusable views from reaching the expensive validation path.

Future-proposal recovery is also deliberately bounded. When notified of possible future proposals, the node scans the complete admitted round window from highest to lowest:

```text
current round = R
scan candidates from R + 8 down to R + 1
maximum inspected round buckets = 8
```

This complete scan handles already-stored proposals and coalesced notifications without becoming an open-ended search. Its maximum work is fixed by the same eight-round policy that bounds message admission.

Round-aware reclamation reduces continuing CPU and garbage-collection pressure as well as memory use. Once the node adopts a newer round, obsolete lower-round buckets are removed and no longer participate in later message lookup, validation, certificate selection, or quorum scans.

Authenticated direct catch-up avoids repeatedly executing every intermediate round:

```text
legacy recovery:
wait for round timeout
→ tear down round workers
→ construct and broadcast ROUND_CHANGE
→ initialize the next round
→ repeat for every intermediate round

INNBC recovery:
validate bounded higher-round evidence
→ jump directly to the highest qualifying round
→ initialize the relevant round once
```

This reduces avoidable timer handling, worker startup and teardown, proposer calculations, message scans, and repeated round-change processing after validator startup, synchronization, or temporary disconnection.

The optimization applies inside the consensus engine. A remote message still incurs basic network reception and decoding before its view can be inspected, but inadmissible future views are prevented from reaching the substantially more expensive authenticated consensus and storage paths.

<!-- FINISH ALPHA CHANGE 1906 -- document bounded consensus CPU utilization -->

#### Preserved Exponential Backoff

INNBC Chain retains exponential IBFT round timeouts. The growing timeout remains valuable because it gives temporarily delayed validators increasing time to reconnect, synchronize, validate, and converge instead of forcing all nodes into an endless high-frequency round-change loop.

The catch-up mechanism improves recovery without replacing that backoff:

```text
no authenticated future evidence
→ normal exponential timeout progression

authenticated higher-round evidence
→ bounded direct catch-up
```

Timeout arithmetic is also saturated safely. Extremely high theoretical round numbers cannot overflow a Go duration and wrap into an immediate or negative timeout.

#### Result

The resulting consensus behavior is:

```text
malformed message
→ structurally rejected

unknown or inactive sender
→ rejected

stale height or round
→ rejected

height beyond H + 1
→ rejected before signature verification and storage

H + 1 above round 0
→ rejected

current-height round beyond R + 8
→ rejected

bounded future round with weak authenticated evidence
→ advance and broadcast local ROUND_CHANGE

bounded future round with full certificate
→ advance using normal quorum authority

obsolete stored view
→ reclaimed after height or round advancement
```

This refactor improves:

- resistance to authenticated memory-exhaustion attacks;
- protection against faulty validators emitting nonsensical future views;
- recovery after validator startup or temporary disconnection;
- alignment between validators at the same height but different rounds;
- two-validator recovery without weakening equal voting authority;
- larger weighted-validator-set recovery;
- retention of prepared-certificate safety;
- deterministic consensus-memory bounds;
<!-- START ALPHA CHANGE 1906 -- document bounded consensus CPU utilization -->
- reduced signature-verification, quorum-scanning, and message-store CPU pressure from irrelevant future views;
- bounded future-proposal scanning with a fixed maximum of eight round buckets;
- reduced round-worker and timer churn through authenticated direct catch-up;
<!-- FINISH ALPHA CHANGE 1906 -- document bounded consensus CPU utilization -->
- timeout-overflow safety;
- maintainability through complete INNBC ownership;
- reproducible local protobuf generation;
- independence from the legacy external IBFT runtime module.

Consensus voting thresholds, block validation, proposer selection, committed-seal verification, transaction execution, and finalized-block synchronization remain unchanged.
<!-- FINISH ALPHA CHANGE 1899 -- document bounded and catch-up-aware INNBCBFT consensus -->

### 31. Authenticated And CPU-Efficient Transaction Replacement

INNBC Chain strengthens transaction replacement while preserving the normal wallet operations used to accelerate or cancel a pending transaction.

A transaction replacement occurs when a sender submits another transaction using the same account nonce:

```text
same authenticated sender
+ same nonce
+ different signed transaction
→ replacement candidate
```

This is legitimate behavior. A user may increase the fee of a transaction that has remained pending, replace its destination or calldata, or cancel it by submitting a higher-priced transaction to the same account with the same nonce.

The inherited txpool accepted a replacement whenever its effective gas price was merely greater than the existing transaction. Consequently, a sender could repeatedly submit replacements using one-wei increases:

```text
100 wei
→ 101 wei
→ 102 wei
→ 103 wei
→ ...
```

Although each new transaction replaced the previous entry rather than growing the txpool indefinitely, every accepted replacement could still trigger signature recovery, state validation, account and queue updates, index replacement, event signalling, promotion work, and transaction gossip.

INNBC Chain replaces this linear one-wei policy with a meaningful proportional bump:

```text
required replacement price
= old price + max(1 wei, ceil(old price × 10%))
```

Examples include:

```text
old price 1 wei
→ minimum replacement 2 wei

old price 7 wei
→ minimum replacement 8 wei

old price 10 wei
→ minimum replacement 11 wei

old price 11 wei
→ minimum replacement 13 wei

old price 100 wei
→ minimum replacement 110 wei
```

The calculation uses arbitrary-precision integer arithmetic, so the percentage multiplication and addition cannot overflow. Fractional wei values are rounded upward, and every accepted replacement must increase the relevant price by at least one wei.

This policy remains suitable for INNBC Chain’s low-fee environment. At very small prices, rounding can produce a relative increase greater than ten percent, but the absolute increase remains only one or two wei. At larger prices, repeated accepted replacements grow geometrically rather than through practically unlimited one-wei steps.

#### Legacy And Dynamic-Fee Replacement Rules

Legacy transactions use their explicit gas price:

```text
new GasPrice
≥ old GasPrice + max(1 wei, ceil(old GasPrice × 10%))
```

When one dynamic-fee transaction replaces another, both EIP-1559 fee fields must independently satisfy the bump:

```text
new GasFeeCap
≥ old GasFeeCap + max(1 wei, ceil(old GasFeeCap × 10%))

and

new GasTipCap
≥ old GasTipCap + max(1 wei, ceil(old GasTipCap × 10%))
```

For example:

```text
old GasFeeCap = 200 wei
old GasTipCap = 10 wei

minimum new GasFeeCap = 220 wei
minimum new GasTipCap = 11 wei
```

Increasing only the fee cap or only the priority-fee cap is insufficient. Requiring both values to rise prevents a transaction from satisfying the replacement policy through a nominal change that does not provide a complete and durable fee improvement.

Cross-type replacements remain supported:

```text
legacy → dynamic fee
dynamic fee → legacy
```

For these transitions, the txpool compares effective gas prices using the current base fee. This preserves compatibility without treating structurally different fee models as if they had identical fields.

#### Authenticated Replacement Ownership

Replacement lookup never trusts a sender address supplied directly in decoded transaction data.

Before checking whether a transaction can replace an existing entry, the txpool:

1. validates the transaction’s basic structure and encoded size;
2. cryptographically verifies the transaction signature;
3. recovers the actual sender address from that signature;
4. verifies that any supplied `From` value matches the recovered sender;
5. validates the fee fields required for a safe comparison.

Only then does it look for an existing transaction with the same authenticated sender and nonce.

Therefore, an attacker cannot replace another account’s pending transaction merely by copying its nonce, destination, fee, or transaction data. A transaction signed with the attacker’s key belongs to the attacker’s txpool account. Replacing another user’s transaction still requires possession of that user’s private key.

#### Early Rejection Before Expensive State Work

After sender authentication and fee validation, an insufficient same-type replacement is rejected before it reaches the more expensive state-dependent admission work:

```text
decode transaction
→ validate basic structure and size
→ recover and verify sender signature
→ validate fee fields
→ locate same-sender, same-nonce transaction
→ enforce replacement bump
→ reject insufficient replacement
```

An early rejection avoids:

```text
state nonce retrieval
less work
account balance retrieval
transaction-cost calculation
intrinsic-gas calculation
block gas-limit validation
txpool slot accounting
queue and index mutation
promotion notification
event publication
transaction rebroadcast
```

Signature recovery is intentionally not skipped. It is the security boundary that proves which account owns the nonce and prevents malicious cross-account replacement.

Passing this early comparison does not admit the transaction. It only permits the candidate to continue through normal validation. The transaction must still satisfy nonce, balance, intrinsic-gas, block-gas-limit, pool-capacity, and all other admission rules.

#### Concurrency-Safe Final Authorization

The replacement policy is checked again while the txpool holds the account’s authoritative queue and nonce-map locks:

```text
authenticated early check
→ remaining transaction validation
→ acquire account insertion locks
→ inspect the transaction currently occupying the nonce
→ repeat the replacement check
→ insert only if the final check succeeds
```

This second check prevents concurrent submissions from exploiting a change between initial inspection and insertion. If another goroutine replaces or removes the old transaction during that interval, the candidate is evaluated against the transaction that actually occupies the nonce slot when insertion occurs.

The early check is therefore a CPU optimization, while the final locked check remains the authoritative admission decision.

#### Quiet Network Rejection With Explicit Wallet Feedback

Expected transaction-policy failures do not need one error-level node log for every rejected submission.

The txpool keeps routine rejection of these conditions out of error logs:

```text
transaction already known
transaction underpriced
replacement transaction underpriced
```

This prevents repeated replacement gossip from becoming a log-amplification mechanism.

The error itself is still returned through the local submission path. A wallet or application submitting an insufficient replacement continues to receive an explicit rejection such as:

```text
replacement tx underpriced
```

The rejected transaction is not inserted, promoted, or rebroadcast. Operational log suppression therefore does not hide the result from the submitting wallet.

#### Result

The replacement path now provides:

- authenticated sender ownership before replacement lookup;
- protection against cross-account transaction replacement;
- a ten-percent proportional bump with upward rounding;
- a minimum absolute increase of one wei;
- independent fee-cap and tip-cap bumps for dynamic-fee transactions;
- continued legacy, dynamic-fee, and cross-type replacement support;
- early rejection before state nonce, balance, cost, and intrinsic-gas work;
- a final concurrency-safe check under account insertion locks;
- preserved wallet-facing rejection errors;
- reduced txpool queue, event, promotion, gossip, CPU, and state-access churn;
- overflow-safe replacement-price arithmetic;
- normal wallet speed-up and cancellation behavior without unlimited one-wei replacement churn.

This is a transaction-pool admission and resource-management improvement. It does not modify transaction signatures, nonce semantics, EVM execution, base-fee calculation, fee charging, refund accounting, block validity, or consensus finality.
<!-- FINISH ALPHA CHANGE 1946 -- document authenticated and CPU-efficient transaction replacement -->

### 32. Corrected Legacy Gas-Price Averaging

INNBC Chain corrected an inherited arithmetic error in the pre-London gas-price average. The accumulator now combines the previous weighted total with new transaction prices and divides by the total sample count. It also safely ignores empty updates. The former formula could produce inaccurate or even negative internal averages when a new block contained more transactions than the previous sample count.

The current genesis enables London from block zero, so `eth_gasPrice` uses the base-fee-and-priority-fee recommendation path, not this legacy average. This correction does not activate pre-London behavior, change fee accounting, or scan historical blocks; it keeps the dormant compatibility path mathematically sound.
<!-- FINISH ALPHA CHANGE 1950 -- document corrected dormant legacy gas-price averaging -->

<!-- START ALPHA CHANGE 2047 -- document end-to-end BLS proof-of-possession validator admission -->
### 33. End-To-End BLS Proof-Of-Possession Validator Admission

INNBC Chain requires every BLS public key entering a consensus validator set to be backed by a valid proof of possession of its corresponding private key. This closes an inherited rogue-key vulnerability in same-message BLS signature aggregation.

Finalized INNBCBFT certificates aggregate the public keys selected by a validator bitmap and verify one aggregate signature over a common message. Without proof of possession, a malicious validator candidate could construct a public key relative to selected honest public keys so that the apparent aggregate public key corresponds to a private scalar controlled by the attacker. If that deliberately constructed key entered an active validator set, the attacker could potentially produce an aggregate certificate that appeared to contain honest validator participation even though those validators had not signed it.

Validator whitelisting, uniqueness checks, and ordinary individual signature verification do not independently close this class of attack. The protection must be enforced before a BLS key is admitted into any validator set used for aggregate verification.

#### INNBC-owned proof format

INNBC implements a dedicated proof-of-possession protocol under the domain:

`DOMAIN_INNBC_BLS_PROOF_OF_POSSESSION`

The signed proof message is the fixed-width concatenation of:

- the validator operator address;
- the exact canonical 128-byte BLS public key.

The proof is a 64-byte BLS signature produced by the private key corresponding to that public key. Binding the proof to both the operator and the complete canonical public-key encoding prevents a proof issued for one operator or key from being reused to admit another identity.

This is a dedicated INNBC protocol domain. Proof signatures cannot be confused with INNBCBFT commit seals, checkpoint signatures, validator-set signatures, or other BLS uses elsewhere in the chain.

#### Replacement for the dormant legacy KOSK concept

The inherited Polygon client contained `MakeKOSKSignature`, a knowledge-of-secret-key helper intended to protect against rogue-key attacks. That helper constructed a signature from an address, manager address, chain ID, and caller-supplied domain, but it was not connected to the active INNBC validator-admission workflow. No production caller required its output before a key entered the consensus validator set.

INNBC retains the underlying knowledge-of-secret-key security principle but implements it as a complete, purpose-specific admission system:

- the proof has its own fixed domain;
- the message binds the operator to the exact BLS public key;
- the node tooling generates the proof from the validator’s real private key;
- genesis parsing requires the proof;
- genesis validator conversion verifies the proof;
- the validator registry verifies the proof during registration;
- BLS-key replacement requires a fresh proof for the replacement key;
- the contract records successful verification with the validator record;
- the common activation gate refuses records without verified possession;
- the Go registry reader refuses to construct an active consensus validator from an unverified record.

This turns an unused legacy helper concept into an end-to-end enforced security invariant.

#### Genesis validator protection

Initial validators do not bypass proof verification.

The node and genesis tooling derive the public proof from the same BLS private key used by the validator. Explicit `--validators` entries use the following structure:

`<P2P multiaddress>:<ECDSA operator address>:<BLS public key>:<BLS proof>`

The parser removes the three fixed trailing fields from the right, preserving compatibility with multiaddresses containing additional colons, including IPv6 multiaddresses.

Before genesis validator metadata can be constructed, INNBC verifies:

- that the validator entry is non-nil;
- that the operator address is nonzero and unique;
- that the BLS public key has the exact canonical length and encoding;
- that the proof has the exact canonical length and encoding;
- that the proof verifies under the supplied public key;
- that the proof message contains the same operator and public key;
- that no BLS public key is duplicated;
- that the initial validator count satisfies the configured validator-set bounds.

During consensus initialization, the proved validator definition is rebuilt into canonical genesis extra data and compared with the validator metadata actually carried by the genesis header. This prevents an imported genesis from presenting valid proofs for one configured validator set while initializing consensus from a different validator set embedded in `extraData`.

A genesis produced through the official generator builds both representations from the same verified metadata.

#### Registry admission and activation

`registerValidator` now accepts:

`registerValidator(rewardAddress, blsKey, blsProof, libp2pPeerID, stakeAmount)`

Before registration changes contract state or transfers stake, the registry performs its inexpensive eligibility, length, ownership, and uniqueness checks. It then verifies the proof against `msg.sender` and the submitted BLS public key.

A successful registration stores `blsProofVerified` inside the validator record. The flag is packed alongside the existing record booleans and is returned by the existing validator getter, avoiding an additional contract call when the client reads validator state.

Governance whitelisting remains a separate step for later validator candidates. The common activation path requires the candidate to be registered and to have a verified BLS proof. Consequently, neither the automatic genesis-validator activation route nor governance whitelisting can place an unproved key into the active set.

#### Safe BLS-key replacement

`updateBLSKey` now accepts both the replacement key and its proof:

`updateBLSKey(blsKey, blsProof)`

The registry first rejects malformed lengths, an unchanged key, or a key already assigned to another operator. It performs the more expensive pairing verification only after those inexpensive conditions pass.

A successful replacement:

- proves possession of the replacement private key;
- atomically replaces the key and its ownership index;
- records the new proof status;
- removes the validator from the active set when it was previously active;
- requires governance to approve activation of the new consensus identity.

A failed replacement reverts the complete transaction and preserves the previous validator record. Changing a consensus key therefore cannot silently retain approval that was granted to a different key.

#### Canonical curve-point enforcement

INNBC closes two point-validation gaps at the cryptographic boundary.

The registry explicitly rejects an all-zero BLS public key or proof. In the underlying curve representation, all-zero coordinates encode the point at infinity. Pairing identities involving infinity could otherwise make an infinity key and infinity proof appear valid.

The BN256 pairing precompile also verifies that every decoded G2 point belongs to the required prime-order subgroup. Successfully decoding coordinates and confirming that a point lies on the curve is not sufficient for BLS security; subgroup membership is a separate requirement.

This precompile correction applies consistently to every contract using the chain’s pairing precompile. Infinity remains valid for the generic pairing primitive where required by pairing semantics, while validator admission separately prohibits infinity as a BLS identity or possession proof.

#### Runtime and performance characteristics

Proof verification occurs only at validator-admission boundaries:

- genesis generation and initialization;
- registry registration;
- BLS-key replacement.

It is not added to:

- ordinary block production;
- transaction execution;
- prepare or commit message processing;
- finalized-certificate verification;
- block synchronization;
- per-block validator signing.

The registry reader checks the already-recorded proof status while assembling registry-derived validator sets. Because that status is returned as part of the existing validator-record call, this does not introduce a separate RPC or contract read for every validator.

The G2 subgroup check adds required validation to executions of the generic BN256 pairing precompile. Those executions remain explicitly invoked and gas-bounded; this is not a continuously running background check.

The normal chain hot path therefore retains the existing BLS aggregation model and performance characteristics. The comparatively expensive proof pairing is paid when a validator registers or changes its BLS key, not for every block.

#### Security result

The validator-admission path now establishes the invariant:

> Every BLS key used by INNBC aggregate-certificate verification has demonstrated possession of its corresponding private key before activation.

The implementation protects:

- the initial genesis validator set;
- later registry candidates;
- genesis-validator activation;
- governance-based activation;
- BLS-key replacement;
- registry-to-consensus validator-set construction;
- infinity and malformed-point boundaries;
- BN256 G2 subgroup correctness.

This is a substantial consensus-security improvement over the inherited Polygon design. It does not change ordinary user transaction signatures or add recurring proof verification to block production. It prevents an admitted validator from exploiting an unproved rogue public key to counterfeit honest participation in an aggregate finality certificate.
<!-- FINISH ALPHA CHANGE 2047 -- document end-to-end BLS proof-of-possession validator admission -->

<!-- START ALPHA CHANGE 2048 -- document crash-recovery performance decision and operational safeguards -->
### 34. Crash Recovery, Round Catch-Up And Signing-State Policy

INNBCBFT provides immediate block finality through PREPREPARE, PREPARE, and COMMIT quorum rather than producing a series of provisional blocks that are finalized later. This preserves the chain’s preferred model: a block is inserted only after the required validator agreement has been established.

Unfinished consensus state remains process-local. If a validator loses power after broadcasting a vote but before inserting the corresponding block, a restart begins from the latest durably inserted block and does not reconstruct that validator’s previous in-memory prepared certificate or outgoing signing decision.

#### Bounded restart convergence

INNBC’s bounded catch-up logic substantially reduces the operational impact of a restart:

- consensus messages are accepted only for the active height and the immediately relevant next height;
- future-round processing is limited to an eight-round window;
- authenticated evidence can advance a lagging validator directly to the relevant round;
- a validator restarting at round 0 can therefore join validators already working at round 6 without waiting through rounds 1–5;
- obsolete rounds are pruned after advancement;
- intermediate timers, proposal work, message retention, and CPU activity are avoided.

This improves restart liveness and reduces the period during which validators operate in different rounds. It does not recreate votes or prepared certificates that disappeared with the terminated process.

#### Persistent signing journal evaluation

A crash-persistent signing journal was evaluated. To provide meaningful protection, every signing decision would have to become durable before its signature was broadcast.

In the normal successful-round path:

- the proposer signs PREPREPARE and then COMMIT;
- every other validator signs PREPARE and then COMMIT;
- every round change creates an additional signing decision;
- an already durable identical retransmission would not require another publication.

At a two-second block interval, the normal path alone would produce:

```text
43,200 blocks per day
× 2 durable signing decisions per block
= approximately 86,400 durable publications per validator per day
```

A conservative file-publication design would require writing the record, synchronizing it, renaming it, and synchronizing its directory before broadcasting the signature. A bounded write-ahead log could reduce metadata work, but it could not remove the synchronous durability barrier without weakening the protection.

The stored bytes would be small; the relevant cost is recurring synchronization latency, filesystem jitter, and additional failure handling directly in the block-production path.

For the current performance-oriented deployment model, INNBC does not add this persistent signing journal by default. The bounded consensus-message policy, authenticated round catch-up, strict validator control, and disciplined server operation were selected instead of adding recurring synchronous writes to every successful block.

#### Required operational safeguards

Production operators must apply the following precautions:

- use graceful service shutdowns whenever possible;
- wait for a validator process to stop completely before maintenance or replacement;
- never run the same validator identity simultaneously in multiple processes or servers;
- never activate an old validator backup without first synchronizing and confirming that no other process controls the same identity;
- keep validator keys and their current node data together during migration;
- avoid restarting multiple validators simultaneously;
- use reliable server power, storage, and shutdown infrastructure;
- after an unexpected shutdown, allow synchronization and authenticated round convergence to complete before treating the validator as healthy;
- monitor the validator’s height and consensus round until they match the active network.

These controls reduce exposure but do not provide mathematical crash-persistent signing protection. An unexpected failure can still erase unfinished local voting state. This is an explicitly accepted residual risk of preserving the current high-performance signing path without synchronous per-decision persistence.
<!-- FINISH ALPHA CHANGE 2048 -- document crash-recovery performance decision and operational safeguards -->

<!-- START ALPHA CHANGE 2095 -- document permanent historical archives and linked checkpoint recovery -->
### 35. Permanent Historical Archives And Linked Checkpoint Recovery

INNBC provides a separate historical archive workflow for moving old transaction and receipt data away from validator storage while preserving it for later retrieval.

The purpose is to separate two storage responsibilities:

- active nodes maintain current chain state, recent history, and newly produced blocks;
- an independent archive preserves older block, transaction, and receipt data in immutable segments suitable for offline storage or external file and object storage.

This allows operators to reduce the historical database footprint on validator servers without discarding the archived transaction history. The archive still consumes storage elsewhere; the benefit is moving that storage requirement away from the machines responsible for running the chain.

#### Independent From Ordinary Snapshots

The package exposes four distinct operations:

| Command | Purpose |
| --- | --- |
| `segmented` | Build an ordinary recent-history snapshot |
| `checkpoint` | Build an ordinary full-node checkpoint snapshot |
| `archive` | Create or extend permanent historical transaction segments |
| `recovery-checkpoint` | Build a checkpoint linked to an existing archive |

The historical archive is not another bootable snapshot format. It must never be configured through `INNBC_SNAPSHOT_SOURCE_DIR`.

Archive creation and linked recovery are explicit operator commands. They do not add archive writes to ordinary block production or require validators to query the archive.

#### Immutable Historical Segments

Run the packaged archive command:

```bash
sudo innbc-snapshot-builder archive
```

The default archive location is:

```text
/var/lib/innbc/history-archive/
```

Each completed archive range contains:

- a separate MDBX database holding canonical headers, block bodies, receipts, and transaction lookups;
- a sorted transaction-hash index identifying each transaction's block number and position;
- segment metadata recording the range, block identities, file sizes, and checksums.

The builder creates fresh databases through the storage API rather than copying an open node database. Published segments are immutable. Later runs add newly completed ranges and publish a new immutable manifest, updating the archive's `latest.json` last.

Incomplete final ranges remain in the source node until they become complete. Archive segment size is fixed when the archive is created.

The archive is uncompressed. There is no automatic cloud upload, remote database service, or global mutable transaction catalog.

#### Preserved Data And Retrieval

The archive preserves:

- transaction bodies and containing-block information;
- transaction receipts, including success or failure status;
- receipt logs;
- contract-creation receipt data.

A future explorer reader can use the segment indexes to construct a local resolver:

```text
transaction hash
→ segment index
→ block number and transaction position
→ archived MDBX segment
→ transaction and receipt
```

That reader can download the required segment into a local cache instead of restoring the complete historical node database. External storage only needs to hold files; it does not need to query MDBX remotely.

The archive builder supplies the stored data and indexes. Archive-backed JSON-RPC endpoints and the explorer download/cache service are separate work.

Historical state tries are not preserved. The archive does not provide historical balances, historical contract calls, historical storage queries, complete address-activity lists, or an index for complete historical log searches.

#### Linked Recovery Without A History Gap

After creating or extending the archive, run:

```bash
sudo innbc-snapshot-builder recovery-checkpoint
```

This command reads the existing archive; it does not extend it.

The builder selects checkpoint height C from exported consensus state. It then selects a completed archive boundary A that leaves at least the configured minimum recent history in the checkpoint.

The resulting coverage is:

```text
historical archive:  blocks 1 ... archive watermark
checkpoint history: blocks A ... C
checkpoint state:   current state at C
```

Block A exists in both artifacts. The builder compares the actual archived and checkpoint headers at that boundary and, when A precedes C, verifies that block A+1 connects to it.

Coverage overlaps by at least one block. The archive may already extend beyond A or C.

The checkpoint contains `recovery-set.json`, which identifies the shared boundary and pins the exact immutable archive manifest used. Its normal checkpoint `latest.json` remains the publication pointer.

After activation, the node has the checkpoint's current balances, contract code, storage, and nonces, together with the retained recent history. It can synchronize or produce subsequent blocks normally. Older transaction history remains in the separate archive.

#### Moving Historical Storage Off The Validator

The intended operational sequence is:

1. Keep the original node database available.
2. Build or extend the historical archive.
3. Create and validate the linked recovery checkpoint.
4. Copy the archive to its intended permanent storage and verify the copied files.
5. Ensure blocks produced after checkpoint C remain recoverable, or stop production at C before cutover.
6. Activate the checkpoint through the supported recovery workflow.
7. Remove the superseded historical database only after the retained state and archive coverage have been verified.

Neither command automatically uploads the archive, activates the checkpoint, or deletes the source database. A successful local build does not establish that an external copy is complete.

For external publication, copy referenced segment files first, the immutable manifest next, and `latest.json` last. Retain immutable files referenced by earlier manifests.

An existing archive can continue growing from a checkpoint-restored full-mode MDBX node, provided that node still retains the next block required by the archive. This continuation path does not accept a segmented recent-history database as its source.

#### Performance And Trust Model

Archive construction is a one-shot operation. Initial export can be I/O intensive; subsequent runs export only new complete ranges. Per-segment transaction-index construction has a configurable record limit.

Recovery verification reads archive payloads through the selected boundary to verify checksums and indexes. It can therefore take substantial time for a mature archive.

The source node may remain online during creation, but its database must not be deleted, replaced, or pruned during export. Builder locks coordinate cooperating builders; they do not prevent unrelated maintenance of the source database.

Checksums and boundary validation establish consistency under the operator-controlled archive model. They do not independently authenticate an untrusted archive.

For configuration, installation, retry behavior, preservation rules, and detailed operating instructions, see the [INNBC Snapshot Builder guide](packaging/innbc-snapshot-builder/README-snapshot-builder.md).
<!-- FINISH ALPHA CHANGE 2095 -- document permanent historical archives and linked checkpoint recovery -->

<!-- START ALPHA CHANGE 2206 -- document fail-closed execution state and local failure recovery -->
### 36. Fail-Closed Execution State And Recoverable Local Failure Handling

INNBC introduces coordinated error propagation across its execution-state database, EVM integration, transaction pool, JSON-RPC service, synchronization, and consensus boundaries.

The central guarantee is:

> Healthy state retains its existing execution behavior. When required state cannot be read reliably, the affected operation fails explicitly instead of accepting an invented zero balance, zero storage value, absent account, or empty contract.

This strengthens the inherited Polygon Edge state engine, where several read paths could convert missing referenced data, backend failures, or decoding errors into ordinary empty results. Other paths could panic rather than propagate the underlying failure.

The improvement concerns the live execution-state database. It applies during ordinary node operation, independently of snapshot creation or activation.

#### Why Empty State And Unreadable State Must Be Different

Zero and absence are normal Ethereum execution inputs:

- an address may have no account;
- an existing contract may have an unused storage slot;
- a storage trie may legitimately be empty;
- an externally owned account may have no bytecode.

Those outcomes must remain valid.

A different situation exists when the database cannot supply data that the state explicitly references. A missing trie node does not establish that an account is absent. A failed storage read does not establish that a slot contains zero. Missing contract bytecode does not establish that the address has no executable code.

The implementation preserves this distinction:

| State access result | INNBC behavior |
| --- | --- |
| Account absent after successful trie traversal | Valid absence |
| Slot absent in a successfully traversed storage trie | Zero value without an error |
| Empty storage root | Valid empty storage |
| Canonical empty-code hash | Valid empty code |
| Referenced trie node missing | Local state failure |
| Database backend returns an error | Local state failure preserving the cause |
| Referenced node or value fails existing decoding rules | Local state failure |
| Nonempty code hash has no code record | Local state failure |
| Nonempty code hash has a present but zero-length code record | Local state failure |

The distinction depends on the access result and its context, not on whether the returned value happens to be zero.

A local state failure does not necessarily mean permanent database corruption. A backend read failure can also prevent reliable execution. The node must reject the unreliable result in either case.

#### First-Error Retention Across EVM Rollback

The EVM-facing interfaces often require a value rather than a value-and-error pair. INNBC preserves those interfaces while retaining state-access failures in `state.Txn`.

The first failure is recorded and exposed through `Txn.Error()` and the enclosing transition. Later failures do not replace the original cause.

A read may temporarily return a neutral value because the EVM interface requires one. That value is not permission to accept the execution result: the enclosing execution boundaries inspect the retained error before accepting receipts, return data, or committed state.

The error remains outside ordinary EVM rollback state.

This matters for nested calls. A contract may legitimately revert an inner call and continue executing, but that rollback cannot erase a database failure encountered while executing the inner call.

The resulting distinction is:

| Execution outcome | Treatment |
| --- | --- |
| Successful execution with reliable state | Accept the normal result |
| Ordinary EVM failure or `REVERT` with reliable state | Preserve normal failed-transaction behavior |
| Required state access fails | Abort the affected operation with a local state error |

Ordinary failed transactions can still produce failed receipts. Ordinary application errors retain the existing `PostHook` behavior. A retained local state failure takes precedence and prevents further acceptance of results derived from unreliable reads.

#### Geth-Informed Architecture

The conceptual model follows established error-handling principles present in the reviewed Geth implementation.

Geth's `StateDB` retains its first database error in `dbErr`, exposes it through `StateDB.Error()`, and refuses commit after a retained failure. Its account and storage readers distinguish normal absence from read errors. Its code reader accepts the canonical empty-code hash but records a failure when required nonempty code is unavailable.

Geth's RPC execution path also checks the retained state error before returning the apparent execution outcome. This prevents a database failure from masquerading as a transaction problem such as insufficient gas.

INNBC adapts these principles to its own architecture:

| Established Geth concept | INNBC adaptation |
| --- | --- |
| First database error retained in `StateDB` | First local state failure retained in `state.Txn` |
| Error accessible through `StateDB.Error()` | Error accessible through transaction and transition boundaries |
| State failure survives ordinary execution rollback | Error retained outside transaction rollback snapshots |
| Commit refuses previously failed state | Transaction and transition commits reject retained failures |
| State error takes precedence over derived execution errors | Precedence enforced through execution, system calls, RPC, and estimation |

This is Geth-informed error propagation, not an identical implementation or a claim of complete client equivalence.

INNBC additionally integrates the model with its immutable-trie storage, MDBX publication ordering, INNBCBFT proposal construction, pool reconciliation, synchronization, and bounded logging.

#### Safe Trie Mutation And Cache Publication

Trie lookup, insertion, and deletion propagate required-node loading failures instead of treating them as successful absence or producing a usable result after failure.

Mutable trie paths are copied as needed to protect previously valid shared state. Shared resolution and hash caches use synchronized publication, and published hash bytes have independent ownership.

Cache behavior follows explicit rules:

- failed account and code reads do not become successful cached results;
- failed trie resolution does not publish a partially resolved node;
- successfully cached valid data remains usable;
- failed mutation cannot produce an accepted new root;
- derived storage and account tries are published to shared state caches only after the database batch succeeds.

The commit order is:

1. Derive account and storage trie updates.
2. Calculate their roots.
3. Publish the database batch successfully.
4. Publish the resulting tries to shared caches.

A failed batch therefore cannot leave a newly derived trie available through the shared cache as though persistence had succeeded.

#### Execution And Consensus Boundaries

State errors are checked across transaction execution, nested calls and contract creation, hooks, direct system-contract calls, genesis initialization, block processing, commit, and offline trie-rebuild replay.

The local failure classification preserves underlying causes through error wrapping and supports `errors.Is`. Callers can distinguish unavailable local state from an ordinary transaction or execution error without matching error strings.

For INNBCBFT:

- proposal construction stops when reliable execution is unavailable;
- proposal validation withholds acceptance when local execution fails;
- local execution failures receive separate diagnostic classification from invalid proposals;
- failed proposal construction stops before creating an empty pre-prepare message.

These controls do not introduce a new consensus rule or alter healthy transaction execution. They prevent the affected node from participating using an unreliable local execution result.

They also do not allow one damaged validator to make healthy validators accept incorrect state.

#### Transaction-Pool Protection And Recovery

A local database failure must not be attributed to the transaction that happened to trigger the read.

The block builder therefore recognizes local state failures before applying transaction-pool actions. For the triggering transaction, it does not call `Pop`, `Demote`, or `Drop`; it aborts candidate construction and returns the local error.

Pool maintenance also retains incomplete reset state. Recovery retries reconcile cached account nonces and obsolete transaction entries against the current chain head, rather than accumulating an unbounded backlog of missed headers.

Required nonce reads and the base-fee refresh must succeed before reconciliation publishes queue and index changes. Repeated failed recovery attempts are bounded by a retry interval.

A successful standalone base-fee refresh does not clear an incomplete pool reset.

Before block construction consumes transactions, `WithReconciledPool` requires successful reconciliation and protects the selection, execution, and corresponding pool actions against concurrent resets.

The guard is released before the builder waits for the remaining block timer. Demotion counters use atomic accesses across builder and maintenance paths.

This prevents stale pool state following a failed reset from being mistaken for a fault in otherwise legitimate transactions.

#### JSON-RPC And Synchronization

Direct account, balance, nonce, storage, and code queries propagate local state failures instead of returning ordinary empty values.

Simulation boundaries apply the same distinction:

- `eth_call` reports the local failure before interpreting a derived EVM result;
- tracing does not return a result after a detected unreliable state read;
- `eth_estimateGas` stops on a state failure instead of treating it as insufficient gas or changing the search bounds.

At the dispatcher boundary, classified local state failures produce JSON-RPC Internal Error (`-32603`), rather than Invalid Request (`-32600`).

Synchronization likewise distinguishes the receiving node's state failure from a bad remote block. A local execution failure does not increment the bad-block metric or place the serving peer on the temporary exclusion list.

The sync loop uses a controlled retry delay. Incoming status notifications cannot accelerate that local-failure retry loop.

A healthy peer cannot repair the receiver's unreadable execution state merely by supplying the same block again.

#### Bounded Operational Logging

Low-level trie access and first-error retention do not log every failed node read.

Repeated local state failures are reported through fixed, process-wide component budgets for RPC, synchronization, consensus, and the transaction pool.

Each component can emit a failure immediately when its logging window is available. Further reports are suppressed for 30 seconds, and the next emitted failure includes the number suppressed.

The limiter:

- uses bounded storage rather than transaction-hash or account-address maps;
- creates no background logging worker;
- releases its suppression mutex before writing to the log sink;
- prevents suppressed local failures from falling through to ordinary per-operation error logging.

This limits repeated-error output during persistent state unavailability. It does not eliminate the cost of handling failed operations, guarantee a nonblocking log sink, or rate-limit every unrelated diagnostic message.

#### Detection And Recovery Are Separate Responsibilities

Error propagation prevents unreliable state from being accepted. It does not automatically reconstruct missing trie data or delete a damaged database.

A transient access failure may succeed on a later operation. Incomplete transaction-pool reconciliation can retry after state access becomes available.

Persistent trie damage requires an operator recovery procedure.

For a full-history node retaining the complete canonical block history and supported original genesis configuration, `node trie-rebuild` can reconstruct a replacement trie offline. Its preconditions and limitations are described under [Offline Trie Database Recovery](#offline-trie-database-recovery).

Validated checkpoint activation provides the separate checkpoint recovery path.

Snapshot integrity validation and live error propagation complement each other: the former validates an artifact before use; the latter protects subsequent execution when required live-state access fails.

#### Performance, Scope And Validation

This implementation adds error propagation, first-error checks, cache-publication coordination, and guarded pool recovery.

It does not introduce:

- runtime trie-node or bytecode hash verification;
- full-state scans during ordinary execution;
- additional consensus rounds or messages;
- new database or wire formats;
- changes to gas accounting or healthy EVM execution rules.

Normal state-root calculation remains part of existing execution. The error-handling change does not add a separate integrity-hashing pass.

Recovery reconciliation performs additional work when an incomplete reset must be repaired. The build guard also coordinates resets with active transaction selection and execution. Performance depends on workload and contention; passing correctness tests is not a throughput measurement.

Validation includes failure-injection tests, legitimate-absence tests, rollback and error-precedence checks, cache-publication tests, execution and RPC boundary tests, pool-recovery tests, and repeated race-detector runs.

<!-- START ALPHA CHANGE 3415 -- replace historical execution goldens with current behavior regressions -->
The historical Issue 7A frozen-genesis and golden-output comparison has been retired. Its fixed genesis hashes and copied contract bytecode are not compatibility requirements for the current pre-launch design.

Current tests build independent configurations from the embedded contract artifacts and execute the normal genesis initialization hook. Trie reconstruction must reproduce the state created by current canonical execution. Focused assertions verify persisted state, receipt commitments, RPC/direct-call agreement, nested-call rollback, parent-state isolation, ordinary reverts, native transfers, nonces and fees.

The replacement execution tests are in `server/current_execution_test.go`. Current-artifact allocations are shared through `consensus/innbcbft/contractsapi/testutil/genesis.go`. Existing allocation, supply, native-token, replay-failure and RPC-failure regressions remain active.

These tests do not pin historical genesis roots or require golden-file regeneration when artifact metadata changes. They are correctness tests, not CPU-performance or throughput benchmarks.
<!-- FINISH ALPHA CHANGE 3415 -- replace historical execution goldens with current behavior regressions -->

These checks provide regression evidence for healthy behavior and explicit failure behavior. They do not prove every possible execution or scheduling interleaving.

Because runtime hash verification is outside this scope, validly encoded but incorrect content stored under a hash may remain undetected. The implemented guarantee concerns detected backend and decoding failures, missing referenced data, and the safe propagation of those failures through the node.
<!-- FINISH ALPHA CHANGE 2206 -- document fail-closed execution state and local failure recovery -->

<!-- START ALPHA CHANGE 2238 -- document verified offline trie reconstruction from full block history -->
### 37. Verified Offline Trie Reconstruction From Full Blockchain History

INNBC adds a dedicated recovery command for a full-history node whose execution-state database needs replacement while its canonical blockchain history remains intact:

`innbc-chain node trie-rebuild`

The command reconstructs the trie database from the original genesis configuration and the locally retained blocks. This provides an operator-controlled recovery path for damaged execution state without discarding the node's complete blockchain history or downloading that history again.

#### How Reconstruction Works

The blockchain database records the canonical blocks and their transactions. The trie database stores the execution state produced by applying those transactions, including account balances, nonces, contract storage, and bytecode.

The rebuild command recreates genesis state using the configured allocations and INNBCBFT genesis initialization. It then executes every canonical block from block 1 through the captured local head.

Replay uses the existing execution engine and configured fork rules. It supplies the preceding canonical block hashes needed by `BLOCKHASH`, obtains the block creator through the INNBCBFT backend, and executes the block transactions through the normal block-processing path.

Ordinary transaction senders are recovered from their signatures. State transactions retain their stored system sender. The replay also invokes the INNBCBFT pre-commit hook.

This reconstructs execution state from its recorded inputs rather than copying potentially damaged trie contents.

#### Verification Before Installation

Reconstruction must reproduce the commitments already recorded in the retained blockchain.

The command checks:

- The reconstructed genesis state root and genesis block hash.
- Canonical header identity and parent continuity.
- Transaction roots and uncle roots.
- The resulting state root after every replayed block.
- Block gas usage.
- Receipt counts, receipt roots, and log blooms.
- Arrival at the captured head with the expected block hash and state root.

A mismatch stops recovery before the replacement trie is installed.

After replay, the command closes the replacement database and reopens it read-only. It then runs the complete executable-state validator against the final head's state root. This verifies the persisted account trie, referenced contract storage, and bytecode, including their integrity checks, independently of the execution caches used during reconstruction.

The source head is checked again before publication.

#### Isolated Replacement And Preserved Data

Recovery builds a new database in a uniquely named `.trie-rebuild-*` staging directory inside `chain-data`.

The destination `chain-data/trie` must be absent or completely empty. The command refuses a nonempty destination and does not delete an existing trie database. Operators must first move any trie data they need to preserve to a separate backup location.

The blockchain database is opened read-only. The command preserves:

- `chain-data/blockchain`;
- `chain-data/consensus`;
- node keys;
- the original genesis file.

After successful replay, complete-state validation, and file synchronization, the completed staging directory is renamed to `chain-data/trie`. Partial replay state is not installed as the replacement database.

Ordinary failures and handled cancellation attempt to clean up the invocation's staging directory. A forced termination or power loss can leave staging data behind; the next invocation does not automatically activate or resume it.

If installation succeeds but synchronization of the parent directory fails, the command explicitly reports that the trie has already been installed. The node must remain stopped while that filesystem error is investigated.

#### Recovery Requirements

This command supports an INNBCBFT node with a unified blockchain database retaining complete canonical history from genesis.

It requires:

- The original genesis configuration matching the retained blockchain.
- Genesis initialization from allocations; a nonzero INNBCBFT `InitialTrieRoot` is not supported.
- Every canonical block needed to replay from genesis to the stored head.
- An absent or completely empty trie destination.
- Sufficient disk space and time for full replay.
- The node and all other database maintenance processes to remain stopped.

A recent-history segmented database does not satisfy these requirements. Neither does a full-mode database restored from a checkpoint that discarded the earlier block history.

Offline operation is an operator requirement. The command does not acquire a process lock or check whether the node service is running. An empty trie directory is a replacement precondition, not proof that the node is stopped.

#### Relationship To Fail-Closed Error Handling

The error-handling system described above prevents detected local state failures from becoming accepted execution results, fabricated RPC values, or transaction penalties.

Trie reconstruction supplies a complementary recovery operation: when the trie needs replacement and full block history remains available, the operator can rebuild execution state and return the node to service after successful completion.

Issue 7A does not add trie-node or bytecode hashing to ordinary runtime reads. Offline reconstruction instead performs block-by-block commitment checks and a final complete-state integrity check during this explicit maintenance operation.

Recovery does not depend on the runtime reader detecting every possible form of corruption. It can also be invoked when replacement is required following an independently diagnosed storage problem.

The command does not repair damaged block history, reconstruct consensus metadata, or independently authenticate an untrusted replacement chain. Its recovery source is the node's retained canonical history.

#### Operational Scope

Rebuilding is an explicit offline operation. It adds no replay work or integrity-scanning work to ordinary block production.

Its duration and disk requirements grow with the history being replayed. It does not compact the result to only the final reachable state; compact checkpoint construction remains a separate workflow.

On success, the command reports the recovered block number, block hash, state root, and trie path. It does not start the node automatically.

See [Offline Trie Database Recovery](#offline-trie-database-recovery) for production and localtest commands, preparation requirements, and interrupted-recovery handling.
<!-- FINISH ALPHA CHANGE 2238 -- document verified offline trie reconstruction from full block history -->

<!-- START ALPHA CHANGE 2239 -- document peer-directory authorization and bounded overload protection -->
### 38. Native Peer-Directory Admission Control And Overload Protection

INNBC hardens validator discovery with earlier registry authorization, native request-rate limits, bounded concurrent work, bounded identity tracking, and controlled client retries.

These protections apply to the peer-directory exchange endpoint regardless of whether it serves local nodes or is deliberately exposed through an external endpoint. They provide application-level protection without requiring an external reverse proxy to implement the basic limits.

#### Authorization Before Nonce-Cache Changes

Signed requests must establish ownership of their claimed libp2p identity and satisfy the validator-registry authorization check.

The exchange path now follows this order:

1. Apply global rate and concurrent-exchange limits.
2. Decode the size-limited request and validate its timestamp, identity, advertised address, and signature.
3. Apply the rate limit for the verified peer identity.
4. Obtain the registry allowlist and check validator authorization.
5. Reject an unauthorized identity immediately.
6. For an authorized identity, check and consume the nonce before updating and returning the registry-filtered peer directory.

Previously, signature-valid requests consumed nonce-cache resources before registry authorization. An unauthorized identity could therefore cause nonce insertion, expiration scans, and nonce-cache mutex contention.

The revised ordering prevents those nonce-cache operations for unauthorized identities. It also preserves signature verification before registry access, so an invalid signature does not trigger an authorization lookup.

The smart contract remains the authorization source. Neither the persistent peer-address JSON file nor the rate-limiter identity map grants validator membership.

Moving authorization earlier does not eliminate registry work for a correctly signed unauthorized request that passes admission limits. The native limits separately constrain how many such requests can reach that work.

#### Layered Request Budgets

The directory uses token-bucket rate limits, allowing controlled bursts while limiting sustained request admission.

| Protection | Default | Purpose |
| --- | --- | --- |
| Global request rate | 20 requests/second | Limits sustained admission before request decoding and signature verification |
| Global burst allowance | 64 requests | Accommodates short bursts such as coordinated node startup |
| Per-identity replenishment | One token every 5 seconds | Limits sustained exchanges from the same verified identity |
| Per-identity burst allowance | 8 requests | Allows a bounded burst of legitimate refresh activity |
| Concurrent admitted exchanges | 32 | Bounds active exchanges, including requests waiting for serialized registry processing |
| Tracked identities | 4096 | Bounds memory used for identity-specific rate limiting |

The concurrency check does not wait for an available admission slot. Requests exceeding capacity receive an immediate overload response.

Identity-specific limiting occurs only after signature verification. An unsigned claim to another validator's peer ID cannot consume that validator's identity allowance.

A requester can generate additional identities, so identity-specific limits are complemented by the global rate, concurrency, and tracking-capacity limits.

#### Bounded Identity Tracking

The limiter does not grow an unrestricted map of requester identities.

When its tracking capacity is reached, it can replace the oldest entry only after that entry has been idle long enough. The idle period is at least ten minutes and cannot be shorter than the time required to refill its token bucket.

This avoids evicting an active identity merely to grant it a fresh burst allowance when it returns. If no entry is eligible for replacement, admission of a new identity receives a temporary capacity response.

The rate-limiter map is separate from the authorized peer-address cache and the nonce replay-protection cache.

#### Explicit Overload Responses

The endpoint distinguishes rate exhaustion from capacity exhaustion:

- HTTP `429 Too Many Requests` indicates an exhausted global or identity-specific rate allowance.
- HTTP `503 Service Unavailable` indicates exhausted concurrency or identity-tracking capacity.

Both responses include `Retry-After` and `Cache-Control: no-store`.

These responses reject the current exchange. They do not ban the peer, change its registry membership, or place it on a consensus exclusion list. A later request can succeed when the applicable allowance or capacity becomes available.

The overload-response path deliberately avoids logging every rejected request. This prevents that rejection path from turning a request flood into an equivalent stream of log writes.

#### Controlled Client Retries

Node directory exchanges and the explicit directory exchange command share bounded retry handling for HTTP `429` and `503`.

The client uses:

- At most five total attempts.
- A ten-second HTTP timeout per attempt.
- A sixty-second deadline for the complete exchange operation.
- Exponential backoff, incorporating `Retry-After` within a ten-second delay cap.
- Up to 250 milliseconds of additional random jitter.
- A freshly constructed signed request, timestamp, and nonce for each attempt.

Random jitter reduces synchronized retries when several nodes encounter overload together.

Authentication failures, other HTTP errors, and transport failures are returned without automatic retry by this helper. Successful response bodies are limited to one MiB, and error-response reads are also bounded.

Node shutdown cancels the node's exchange context, including in-flight HTTP requests and backoff waits. Retries therefore stop when the node is shutting down.

This is bounded retry handling for an exchange, not a new periodic refresh service. Exhausting the retries still returns a failure to the caller.

#### Bounded Registry Access

The directory's registry RPC client accepts HTTP or HTTPS endpoints and applies:

- A five-second timeout to each RPC call.
- A four-MiB response-size limit.
- HTTP-status and JSON-RPC response validation.

An authorization lookup can require multiple RPC calls. The per-call timeout is not a five-second deadline for the entire lookup.

These limits complement the existing request-body limit and HTTP server timeouts. The admitted-exchange limit also bounds how many requests can be executing or waiting for registry processing at once.

#### Configuration And Deployment

The request limits are configurable through peer-directory command flags and the packaged service environment:

| Environment setting | Default |
| --- | --- |
| `INNBC_PEER_DIRECTORY_REQUEST_RATE` | `20` |
| `INNBC_PEER_DIRECTORY_REQUEST_BURST` | `64` |
| `INNBC_PEER_DIRECTORY_PEER_INTERVAL` | `5s` |
| `INNBC_PEER_DIRECTORY_PEER_BURST` | `8` |
| `INNBC_PEER_DIRECTORY_MAX_CONCURRENT` | `32` |
| `INNBC_PEER_DIRECTORY_MAX_TRACKED_PEERS` | `4096` |

The command validates the configured limits before listening. The packaged service supplies defaults when preserved environment files do not yet contain these settings.

The default directory listener remains `127.0.0.1:8080`. A loopback-only directory without external forwarding is not directly reachable by external peers. These protections do not change its binding or expose it to the Internet.

If the endpoint is deliberately exposed, the same native controls remain active. Nginx limits, connection controls, and network-level protection can provide additional layers.

The implementation bounds admitted application work; it does not guarantee availability against network-bandwidth saturation or every distributed flood. Limits should be sized for the deployment's validator startup patterns and registry response times.

See the [peer-directory service guide](packaging/innbc-peerdirectory-service/README-peerdirectory-service.md#native-request-limits) for configuration and operational details.
<!-- FINISH ALPHA CHANGE 2239 -- document peer-directory authorization and bounded overload protection -->

<!-- START ALPHA CHANGE 2260 -- document native RPC admission and overload protection -->
### 39. Native RPC Admission Control, Resource Limits And Safe Overload Handling

INNBC adds native admission protection to its public JSON-RPC server, extending the resource-budget approach introduced for the peer directory to HTTP and WebSocket RPC traffic.

The implementation combines request-rate limits, batch-aware call accounting, bounded concurrency, request-size limits, WebSocket connection limits, filter capacity, and controlled failure cleanup.

These protections operate inside the node. They complement reverse-proxy controls such as Nginx per-IP rate limiting and remain active for requests reaching the RPC listener directly.

The change governs access to RPC resources. It does not modify consensus rules, transaction validity, gas accounting, or the mathematical state transition.

#### Why HTTP Request Limits Alone Are Insufficient

One HTTP request can contain multiple JSON-RPC calls. A request-rate limit therefore does not necessarily bound the number of operations reaching the node.

For example, with the repository's default batch-length limit of 20, ten HTTP requests can contain up to 200 RPC calls. The work also varies significantly by method: reading the chain ID is much cheaper than estimating gas or scanning historical logs.

The revised implementation distinguishes:

- Transport admission: whether another HTTP request or WebSocket message can enter processing.
- Call admission: whether the enclosed RPC calls fit within the shared call-rate allowance.
- Expensive execution: whether another selected simulation or historical query can begin.
- Persistent resources: whether another WebSocket connection, filter, or subscription can be retained.

This prevents a single request-count limit from being treated as a complete execution budget.

#### Shared HTTP And WebSocket Budgets

Each RPC server instance owns one admission controller shared by its HTTP and WebSocket paths.

The rate limits use `golang.org/x/time/rate`, the same token-bucket package used by the peer directory. RPC and peer-directory controllers have separate instances and allowances.

Token buckets permit bounded bursts while limiting sustained admission. Concurrent processing is controlled separately through non-waiting capacity checks.

| Control | Default |
| --- | --- |
| Global transport admission rate | 100 requests/messages per second |
| Transport burst allowance | 200 |
| Global RPC call rate | 200 calls per second |
| Call burst allowance | 400 |
| Concurrent admitted requests/messages | 32 |
| Concurrent selected expensive methods | 4 |
| Maximum HTTP request body | 1 MiB |
| Maximum WebSocket connections | 64 |
| Combined retained filters and subscriptions | 1024 |

WebSocket upgrade requests also pass through transport admission. Once established, each supported incoming WebSocket data message must obtain its own admission.

An idle WebSocket occupies a connection slot, but does not permanently occupy an active request slot.

These defaults are configurable operating limits, not measured throughput guarantees. Appropriate values depend on hardware, state size, workload, and the capacity reserved for the node's other responsibilities.

<!-- START ALPHA CHANGE 2285 -- document production RPC admission environment overrides -->
Production node services expose these defaults through `/etc/innbc/<node>.env`:

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

The updated systemd unit supplies matching defaults for preserved env files that
omit these settings. Newly generated node environments include them explicitly.

Direct localtestclient commands do not automatically read node env files.
They use the same Go defaults unless their `--json-rpc-*` admission flags
override them. Restart a production node when applying changed env settings;
the running process does not reload them automatically.
<!-- FINISH ALPHA CHANGE 2285 -- document production RPC admission environment overrides -->

#### Admission Before Expensive Processing

HTTP transport admission occurs before reading and decoding the request body.

A request that exceeds the transport rate or active-processing capacity is rejected immediately. The application does not add it to an admission waiting queue.

Accepted HTTP requests remain subject to a body-size limit. Both declared oversized bodies and bodies that exceed the limit while being read are rejected before dispatch.

WebSocket messages have their own configured read-size limit, additionally constrained by the shared body-size cap. Setting the WebSocket-specific limit to zero does not disable that shared cap.

The existing batch-length, historical log-range, and fee-history limits continue to apply.

#### Batch-Aware Call Accounting

After decoding and checking batch length, the dispatcher charges one call token for each batch member.

The complete batch must fit within the currently available call allowance before any member executes. If it does not, the dispatcher returns a resource-limit error for each member without executing any of them.

A batch larger than the configured call burst cannot be admitted, even when the separate batch-length setting permits it.

HTTP and WebSocket calls consume the same call budget, preventing the two transports from providing independent allowances on the same RPC server.

Batch rate admission is distinct from subsequent execution. Once admitted, members execute sequentially and can individually succeed or fail. In particular, an expensive-method capacity error can affect one member while other members succeed.

Clients must inspect individual batch responses and must not blindly retry an entire batch containing transaction submissions.

#### Separate Capacity For Expensive Methods

The following methods share a smaller execution allowance:

- `eth_call`
- `eth_estimateGas`
- `eth_getLogs`
- `eth_getFilterLogs`
- `eth_feeHistory`

If that allowance is exhausted, the method returns an explicit resource-limit error before entering the endpoint.

Other methods do not consume this smaller allowance, although they remain subject to the global transport and call budgets. This reduces competition between expensive queries and simpler RPC operations without claiming that every other method has negligible cost.

Execution capacity is released through deferred cleanup, including when endpoint processing returns an error or unwinds through a panic.

#### Explicit Overload Responses

Resource exhaustion is reported separately from transaction validity or execution-state corruption.

| Condition | Behavior |
| --- | --- |
| HTTP transport rate exhausted | HTTP `429 Too Many Requests` |
| HTTP active-processing capacity exhausted | HTTP `503 Service Unavailable` |
| HTTP request body too large | HTTP `413 Request Entity Too Large` |
| RPC call-rate allowance exhausted | JSON-RPC error `-32005` |
| Selected expensive-method capacity exhausted | JSON-RPC error `-32005` |
| Filter or subscription capacity exhausted | JSON-RPC error `-32005` |
| WebSocket connection capacity exhausted | HTTP `503` before upgrade |
| Established WebSocket exceeds transport admission | Connection closes with code `1013`, indicating temporary overload |

The HTTP `429` and `503` admission responses include `Retry-After: 1`. Admission error responses also disable caching.

These outcomes do not ban clients, change validator membership, mark transactions invalid, or classify a healthy execution-state database as corrupted.

The server does not automatically replay rejected RPC requests.

#### WebSocket Lifecycle And Resource Release

WebSocket connections are bounded independently from active RPC requests.

Messages execute sequentially within each connection, avoiding an unbounded queue of per-message worker goroutines. Different connections can still execute concurrently within the shared limits.

The implementation applies:

- A ten-second WebSocket handshake-write timeout.
- A ten-second deadline for each serialized WebSocket message write.
- Deferred release of admitted message capacity on success, error, or panic unwinding.
- Socket closure after a failed write or failed write-deadline setup.
- Removal of every subscription associated with a closed connection.

Closing a failed socket wakes its read handler so the connection's resources can be released.

These deadlines bound network operations. They do not introduce EVM execution cancellation.

#### Filter And Subscription Capacity

The filter manager enforces its combined capacity under its existing lock.

A filter is inserted, and its WebSocket metadata published, only after admission succeeds. Rejected creation leaves the connection's existing filter metadata unchanged and returns an explicit resource-limit error to the RPC caller.

Normal filter removal, expiration, and connection cleanup make capacity available again.

When subscription delivery fails, the manager removes all subscriptions belonging to the affected connection, including subscriptions of another event type. It avoids repeatedly attempting other subscriptions on a connection already known to have failed during that flush.

Successful subscriptions on other connections remain active.

#### Bounded Failure Logging

Overload rejection does not produce a warning for every rejected request.

Ordinary dispatcher warnings are limited to one every five seconds per admission controller. Unexpected subscription-delivery warnings use a separate five-second limiter. Both retain suppressed-failure counts for a subsequent emitted warning.

Normal closed-connection conditions do not produce subscription warnings. Full HTTP request and response payload logging has also been removed.

The existing Issue 7A local-state-failure logging remains separately controlled. Resource-limit errors do not enter the local-state-failure classification.

These controls reduce formatting work, logger contention, and output I/O on the protected failure paths. They do not impose a universal limit on unrelated node logs or debug-level method logging.

#### Configuration

The following server flags configure native admission:

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

Structured server configuration exposes the corresponding settings under `json_rpc_admission`.

Omitting the admission configuration selects defaults. Explicit zero limits are invalid, and the expensive-method concurrency limit cannot exceed the overall concurrency limit.

The limits are local server policy. They are not genesis or consensus parameters.

#### Relationship To Reverse-Proxy Protection

Native limits protect the node's application resources, while a reverse proxy can enforce client-specific and connection-level policies before traffic reaches the node.

The implementation does not trust forwarded-IP headers or maintain a native per-IP limiter. Deployments using Nginx can retain per-IP limits there while the node enforces its aggregate budgets.

This also avoids accidentally treating all proxied clients as one client because their connections originate from loopback.

Neither the RPC listener binding nor public namespace registration is changed by these controls. No validator key or peer-directory authorization is required for ordinary public wallet RPC access.

#### Scope And Validation

The implementation bounds admission rates, selected concurrent work, request bodies, WebSocket connections, and filter counts.

It does not currently provide:

- EVM execution cancellation when a client disconnects.
- A response-size cap.
- A byte limit on each filter's accumulated results.
- Per-client fairness within the shared native budgets.
- Protection against saturation of the server's upstream network connection.

HTTP network timeouts and WebSocket write deadlines must not be interpreted as total execution deadlines.

Validation includes deterministic admission tests, whole-batch rejection before execution, concurrency and capacity recovery, body-limit enforcement, healthy RPC responses, subscription metadata preservation, failed-write cleanup, bounded logging, handshake deadlines, and panic-safe message-slot release.

<!-- START ALPHA CHANGE 3415 -- distinguish historical validation from current regression coverage -->
Focused tests passed repeatedly under the race detector. Broader RPC/server race tests, repository tests, build, and vet checks also passed during implementation. At that implementation stage, the historical Issue 7A healthy-execution golden comparisons remained unchanged. Those frozen comparisons have since been retired in favor of current-artifact execution and rollback regressions.
<!-- FINISH ALPHA CHANGE 3415 -- distinguish historical validation from current regression coverage -->

The feature provides native RPC admission and overload protection while preserving the established execution and consensus rules. Production capacity tuning remains a separate measurement task.
<!-- FINISH ALPHA CHANGE 2260 -- document native RPC admission and overload protection -->

<!-- START ALPHA CHANGE 2268 -- document standalone blockchain database archival backups -->
<!-- START ALPHA CHANGE 2368 -- clarify live blockchain backups and deferred trie reconstruction -->

### 40. Consistent Live Blockchain Backups For Offline Storage And Later Trie Reconstruction

The `snapshot backup` command exports a consistent archival copy of a
full-history node's unified blockchain database while the source node
continues normal operation. It also preserves the supplied original genesis
configuration byte-for-byte and records backup metadata and checksums.

The backup deliberately excludes the trie database. Complete blockchain
history and the original genesis configuration allow executable state to be
reconstructed later with the separate `node trie-rebuild` command, subject
to that command's genesis and history requirements.

This separates two operations: preserving the chain now and performing
expensive state reconstruction only when it is needed. An archival backup
does not need a reconstructed trie merely to be stored offline.

The feature is independent of bootable snapshots, historical archive
segments, and linked recovery checkpoints. Its placement under the
`snapshot` command group is organizational. Nodes do not automatically
discover, activate, or consume these database backups.

The command creates a local output directory. Downloading that directory
to another machine or transferring it to offline storage is a separate
operator action.

<!-- FINISH ALPHA CHANGE 2368 -- clarify live blockchain backups and deferred trie reconstruction -->

#### Consistent Export While The Source Node Runs

The command reads all blockchain records through one fixed MDBX read transaction.
Normal block processing may continue in the source node while export proceeds.
Headers, block bodies, receipts, transaction lookup records and other records
in the blockchain database are copied from the same committed database view.

The destination is written in bounded batches. The command then reopens it and
compares a length-framed SHA-256 digest and record count against the captured
source view. It also checks the recorded head, genesis header identity, canonical
parent continuity, and readable bodies and receipts through the captured head.

The supplied genesis file is preserved exactly and included in backup metadata
with a checksum. The completed MDBX data file also receives a checksum for later
integrity comparisons.

These checks verify archival copying and history availability. They do not
execute historical transactions, regenerate genesis allocations, or prove that
the source chain itself is trustworthy.

#### Create A Backup

<!-- START ALPHA CHANGE 2285 -- document production and local database backup entry points -->
With the updated snapshot-builder package, configure
`/etc/innbc/snapshot-builder.env`:

```ini
INNBC_SNAPSHOT_BUILDER_CHAIN=/etc/innbc/genesis.json
INNBC_SNAPSHOT_BUILDER_SOURCE_DATA_DIR=/var/lib/innbc/node1
# START ALPHA CHANGE 3080 -- document the full-history database backup allowance --
INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB=1024
# START ALPHA CHANGE 3504 -- document builder destination page size --
INNBC_SNAPSHOT_BUILDER_MDBX_PAGE_SIZE_BYTES=4096
# FINISH ALPHA CHANGE 3504 -- document builder destination page size --
# FINISH ALPHA CHANGE 3080 -- document the full-history database backup allowance --
INNBC_DATABASE_BACKUP_OUTPUT_ROOT=/var/lib/innbc/database-backups
```

Then run:

```bash
sudo innbc-snapshot-builder backup
```

<!-- START ALPHA CHANGE 3080 -- document backup configuration before launch -->
The installer prepares the default backup directory. Review the builder
environment before launch and configure the backup output directory and
1024 GiB destination allowance shown above.
<!-- FINISH ALPHA CHANGE 3080 -- document backup configuration before launch -->

For a local full-history test node, run from the repository root:

```bash
./localtestclient/bin/innbc-chain snapshot backup \
  --chain localtestclient/genesis.json \
  --data-dir localtestclient/nodes/node1 \
  --out localtestclient/database-backups \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3504 -- document local backup storage geometry --
# FINISH ALPHA CHANGE 3504 -- document local backup storage geometry --
```

The direct command uses explicit options and does not read the production
snapshot-builder env file.

Alternatively, on the server, invoke the binary directly as the account that
owns the source node database:
<!-- FINISH ALPHA CHANGE 2285 -- document production and local database backup entry points -->

```bash
sudo runuser --user innbc -- \
  /usr/local/bin/innbc-chain snapshot backup \
  --chain /etc/innbc/genesis.json \
  --data-dir /var/lib/innbc/node1 \
  --out /var/lib/innbc/database-backups \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3504 -- document production backup storage geometry --
# FINISH ALPHA CHANGE 3504 -- document production backup storage geometry --
```

The output root must be outside the source node's data directory.

Each invocation publishes a uniquely named directory:

```text
database-backups/
  database-backup-<height>-<UTC-timestamp>-<unique-suffix>/
    backup.json
    genesis.json
    chain-data/
      blockchain/
        mdbx.dat
```

<!-- START ALPHA CHANGE 2369 -- distinguish blockchain archives from complete node backups -->

The archive contains the complete exported blockchain database, including
headers, block bodies, receipts, transaction lookup records, and other
records stored in that database. It also includes the original genesis
file and `backup.json` metadata describing the captured head and checksums.

There is no trie database, consensus database, or private-key material in
the archive. No startup `latest.json` pointer or bootable snapshot manifest
is created.

This is a blockchain archive, not a complete backup of a validator's
identity and operational configuration. Preserve those assets separately
when a full node-system backup is required.

<!-- FINISH ALPHA CHANGE 2369 -- distinguish blockchain archives from complete node backups -->

The existing builder lock serializes publication into the backup output root;
it does not stop the source node. Failed operations are not intentionally
published as completed backups. An interrupted process may leave a hidden
`.database-backup-building-*` directory, which is not a completed archive.

Only unified databases with readable continuous history from genesis are
accepted. Recent segmented databases are not supported by this command.

<!-- START ALPHA CHANGE 2370 -- document export transfer and optional offline reconstruction -->

#### Recommended Archival Workflow

1. Run `snapshot backup` on the machine containing the full-history source.
   The source node may continue normal block processing during export.

2. Wait for successful completion and publication of the uniquely named
   backup directory. Transfer the entire completed directory, including
   `backup.json`, `genesis.json`, and `chain-data/blockchain`.

3. Download or copy that directory to the intended archival location.
   Compare the transferred genesis and database files against the
   checksums recorded in `backup.json`.

4. Preserve an unchanged copy offline. No trie reconstruction is required
   for this archival-storage step.

5. If executable state is needed later, create a separate working copy
   and run `node trie-rebuild` against that copy using a compatible INNBC
   binary, such as the locally built `localtestclient` binary.

The reconstruction target must remain offline throughout replay. The
source node from which the backup was originally exported can remain
online independently.

<!-- FINISH ALPHA CHANGE 2370 -- document export transfer and optional offline reconstruction -->

For example, after preparing a working copy at
`/home/operator/innbc-backup-work`:

```bash
./localtestclient/bin/innbc-chain node trie-rebuild \
  --chain /home/operator/innbc-backup-work/genesis.json \
  --data-dir /home/operator/innbc-backup-work \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3504 -- document backup reconstruction storage geometry --
# FINISH ALPHA CHANGE 3504 -- document backup reconstruction storage geometry --
```

Use the genesis file preserved in the backup, not the local test network's
genesis configuration.

The target trie directory must be absent or completely empty. No running node
or other maintenance process may use the reconstruction target directory.
Existing trie-rebuild requirements apply, including allocation-based genesis
initialization rather than a nonzero INNBCBFT `InitialTrieRoot`.

<!-- START ALPHA CHANGE 2371 -- explain verified reconstruction without implying automatic node restoration -->

Reconstruction is optional and can run on either the server or a local
machine. Local reconstruction is the recommended workflow for keeping
lengthy replay work off the production server; it is not a functional
restriction.

The reconstruction command executes genesis initialization and replays
the preserved blocks. It checks reconstructed state roots, gas usage,
and receipt commitments against the stored history before publishing
the replacement trie.

The backup command itself never starts reconstruction. Export verification
checks copying consistency and readable history; successful replay is a
separate validation step performed when reconstruction is requested.

Rebuilding the trie does not recreate validator private keys or turn the
archive into a bootable snapshot. Consensus databases and node identity
remain outside the scope of this archival feature.

<!-- FINISH ALPHA CHANGE 2371 -- explain verified reconstruction without implying automatic node restoration -->

#### Operational Scope

Normal source writes may continue during backup. Source database replacement,
deletion, restore and other maintenance must not overlap the export.

A long-lived MDBX read transaction can retain older database pages while the
source advances. Allow sufficient disk space for both the backup and source
growth. Copying, verification and checksumming also consume I/O; schedule
periodic backups accordingly.

The command does not schedule itself, compress or upload archives, delete older
backups, or turn an archive into a running validator. Consensus state and node
keys remain separate operational assets.

<!-- FINISH ALPHA CHANGE 2268 -- document standalone blockchain database archival backups -->

<!-- START ALPHA CHANGE 2298 -- document limited finalized-block validation consistency checks -->

### 41. Additional Finalized-Block Consistency Checks Without Additional Registry Reads

Finalized-block validation now performs additional checks using information
already available during certificate verification and local block execution.
A quorum certificate remains required, but does not replace these consistency
checks.

The additional checks include:

- comparing the header logs bloom with the combined blooms of locally executed
  transaction receipts;
- comparing the checkpoint current-validator hash with the validator set already
  obtained for certificate verification;
- applying the declared validator delta and checking that the resulting set
  matches the checkpoint next-validator hash;
- requiring the checkpoint event root to remain zero;
- applying the existing checkpoint epoch-number consistency rules;
- rejecting unknown state-transaction inputs and duplicate epoch-commit
  transactions.

Live proposal validation and the finalized-import pre-commit hook share the
state-transaction structure check. Live validation retains its stronger checks
for when an epoch transaction is required and for its exact expected contents.

The finalized-import structure check runs after transaction execution and before
the trie transition is committed. Offline trie reconstruction also invokes this
existing hook.

#### Performance Scope

These additions do not introduce registry contract calls, historical block
scans, or a second execution of the candidate block.

The bloom comparison combines receipt blooms already generated by local
execution. Checkpoint validation reuses the validator set loaded for certificate
verification. Remaining work consists of transaction-input decoding, validator
delta application, ABI encoding, hashing, and comparisons.

This work has a CPU cost, but does not add transaction gas charges or alter the
fee schedule. No measured throughput improvement is claimed.

The checks are always enforced on their respective validation paths. They are
not skipped according to local cache availability. Failures obtaining the
validator set or executing the block remain errors through the existing paths.

#### Deliberately Deferred Checks

This is a limited improvement, not complete equivalence between live proposal
validation and finalized-block import.

The finalized-import path still does not independently:

- determine the actual epoch boundary and require an epoch transaction there;
- reject an epoch transaction solely because its height is outside that boundary;
- compare an epoch transaction with the exact locally expected epoch input;
- prohibit a validator delta solely because the block is outside epoch end;
- derive the expected validator delta from the registry at the candidate's
  parent state.

These checks are deferred to avoid adding historical metadata retrieval or
parent-state registry queries in this patch.

A matching next-validator hash establishes consistency with the declared delta;
it does not establish that the registry authorized that delta.

Issue 12 therefore remains partially addressed. Full parity requires a separate
design for obtaining authoritative epoch and registry expectations without
duplicating execution or introducing unnecessary repeated reads.

<!-- FINISH ALPHA CHANGE 2298 -- document limited finalized-block validation consistency checks -->

<!-- START ALPHA CHANGE 2367 -- document manual trie garbage collection through recent-node snapshot rebootstrap -->

### 42. Manual Trie Garbage Collection For Recent-History Nodes

Recent-history nodes automatically rotate their blockchain-history segments.
Their trie database is managed separately: ordinary execution persists new
hash-addressed state records without automatically reclaiming every obsolete
historical trie version.

Consequently, blockchain retention settings do not impose a total node-storage
limit. A recent node's trie can continue growing even while old blockchain
segments are regularly removed.

The existing snapshot workflow provides a manual maintenance procedure for
reclaiming obsolete trie data: replace the recent node's runtime databases
with a freshly produced, validated segmented snapshot.

**How space is reclaimed**

The snapshot builder copies the account trie reachable from the selected
checkpoint root, together with its referenced contract storage and bytecode,
into a fresh database.

Historical trie records that are not reachable from that checkpoint state
are not copied. Nodes created long ago are still retained when the checkpoint
state needs them.

This is state-aware copying, not deletion based on the age of individual trie
records. Blockchain headers belong to the blockchain database, not the trie.

**Manual maintenance procedure**

1. Produce a fresh segmented snapshot from the deployment's trusted
   full-history source. Confirm that snapshot creation and validation have
   completed and that the artifact is available through the recent node's
   configured snapshot source.

2. Schedule downtime for the target recent node. Account for the validator
   quorum required by the deployment before stopping a validator.

3. Stop the target node completely. Preserve its original genesis
   configuration, environment configuration, and persistent node keys.
   Never run another instance using the same validator identity.

4. Move the target node's complete `chain-data` directory to a separate
   backup location, leaving the live `chain-data` path absent. This replaces
   blockchain, trie, and consensus databases together through snapshot
   activation. Do not remove only the trie while retaining an unrelated
   blockchain head and consensus database.

5. Confirm that the node is configured with `INNBC_HISTORY_MODE=recent`
   and that `INNBC_SNAPSHOT_SOURCE_DIR` points to the intended segmented
   snapshot source. For this maintenance startup, set
   `INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED=true` so an unavailable or invalid
   snapshot stops startup instead of allowing fallback synchronization.

6. Start the target node using its normal service or local-client command.
   The existing bootstrap workflow stages and validates the snapshot before
   activating its replacement `chain-data`. Node keys remain outside that
   directory and are preserved.

7. Confirm successful snapshot activation, synchronization to the current
   chain head, and normal node operation. Restore the deployment's usual
   snapshot-bootstrap setting if it differs from the maintenance setting.

8. Once the replacement is verified and the old databases are no longer
   needed, remove the separate backup according to the operator's backup
   policy. Keeping that backup on the same disk does not reclaim its space.

Provide enough free space for snapshot staging and any retained backup.
The snapshot source and backup location must remain outside the runtime
directory being replaced.

**Scope and operational limits**

This procedure uses the existing snapshot creation and bootstrap mechanisms.
It does not introduce a new garbage-collection command or an automatic
background trie-pruning process.

The replacement database still contains all state required at the snapshot
checkpoint. Subsequent synchronization and execution create additional trie
records again. Legitimate growth of live accounts, contract storage, and
bytecode also remains necessary.

Operators can repeat this maintenance procedure when measured trie growth
and available disk capacity justify it. The amount of space recovered depends
on how much obsolete historical data has accumulated; no fixed reduction or
maintenance interval is guaranteed.

This procedure is distinct from `node trie-rebuild`. That command reconstructs
state by replaying complete blockchain history and is intended for trie
recovery. Snapshot rebootstrap instead starts from the state copied at a
selected checkpoint and does not require the recent node to retain every
block from genesis.

Recent-history mode therefore provides automatic blockchain-history
retention together with an existing manual route for reclaiming obsolete
trie data. It does not guarantee a permanently fixed total database size.

<!-- FINISH ALPHA CHANGE 2367 -- document manual trie garbage collection through recent-node snapshot rebootstrap -->

<!-- START ALPHA CHANGE 2388 -- document accurate full-precision fee-history rewards -->

### 43. Accurate Historical Fee Rewards With Full Precision And Independent Cache Ownership

INNBC Chain corrects the historical priority-fee statistics returned by
`eth_feeHistory`. Wallets and applications requesting historical rewards
receive gas-weighted samples of ordinary transaction tips, without
contamination from fee-free system transactions or truncation of large
fee values.

**System transactions no longer distort reward percentiles**

Protocol-generated state transactions do not pay ordinary transaction fees.
Previously, including them in reward sampling could produce a negative tip
when subtracting a positive block base fee from their zero gas price.
Converting that result into an unsigned reward produced a misleading statistic.

Fee history now excludes state transactions from both the reward samples
and the gas total used to calculate percentile thresholds. Sample weights
come from the persisted receipts of the included ordinary transactions.

Ordinary transactions with a zero priority fee remain valid samples.
Empty blocks and blocks containing only system transactions return zero
for every requested reward percentile. They do not trigger a backward
search for a priced transaction or substitute the current fee recommendation.

The separate `gasUsedRatio` field continues to describe the entire block.
It uses the block header's total gas consumption and gas limit, including
any gas attributed to system transactions.

**Large rewards retain their exact value**

Reward calculation, cache storage, returned results, and JSON-RPC encoding
preserve arbitrary-precision integers instead of narrowing rewards to
64 bits.

For example:

```text
Effective priority fee:  18446744073709551616 wei
Exact JSON-RPC reward:   "0x10000000000000000"
```

That value is `2^64`. The previous 64-bit conversion could reduce it to
zero; the corrected path preserves it exactly.

Legacy and dynamic-fee transaction rewards retain the existing effective-tip
calculation. Dynamic-fee rewards account for both the priority-fee cap and
the room remaining between the total fee cap and the block base fee.

**Cached rewards have independent ownership**

Arbitrary-precision integers are mutable. Reward values are therefore
copied when entering the cache, when being returned from the cache, and
when converted into RPC quantities.

Changing one returned reward cannot alter another percentile, another
caller's result, or the cached value used by subsequent requests.

Regression tests cover sequential ownership and concurrent consumers of
the same populated cache. The concurrent test mutates caller-owned results
while other requests read cached rewards, then checks that the original
values remain intact. Its backend synchronizes mock counters so those
counters do not introduce unrelated test races.

**Existing bounded fee recommendations remain intact**

This correction preserves the behavior described in point 21:

- `eth_maxPriorityFeePerGas` remains a constant-time recommendation based on
  the current base fee, with a one-wei fallback when that fee is zero.
- London `eth_gasPrice` retains its overflow-safe, saturating addition and
  configured price floor.
- `eth_feeHistory` serves a bounded historical range ending at the
  resolved newest block.
- Requests without reward percentiles avoid block-body and receipt reads.

<!-- START ALPHA CHANGE 2495 -- document safe historical event-log range defaults -->

**Historical event-log queries retain a finite default range**

The independent `--json-rpc-block-range-limit` setting controls historical
event-log queries such as `eth_getLogs`.

Omitting the setting or supplying zero selects 256. Positive values remain
configurable through the flag or the packaged
`INNBC_JSONRPC_BLOCK_RANGE_LIMIT` environment setting.

Generated full-history node environments include this override.
Generated recent-node environments omit it and inherit the code default.
The shared systemd service supplies zero when the variable is absent;
a positive per-node environment value overrides that fallback.

Omitting the override does not disable the RPC listener or its protection.
Existing environment files are not automatically rewritten.

The limit bounds `toBlock - fromBlock`, preserving the existing inclusive
range semantics: a difference of 256 can cover 257 blocks. Requests exceeding
the effective range limit are rejected before the block-reading loop.

The fallback applies to runtime configuration, direct filter-manager
construction, and range enforcement. Zero no longer disables this protection.
Operators can still deliberately increase the limit, which permits more
historical work per request.

This setting does not control fee-history percentiles, blockchain retention,
or the EVM BLOCKHASH window.

<!-- FINISH ALPHA CHANGE 2495 -- document safe historical event-log range defaults -->

<!-- START ALPHA CHANGE 2504 -- explain the public RPC event-log range policy -->

**Comparison with commercial RPC free tiers**

INNBC's default event-log range is conservative for node resource protection,
while allowing substantially larger individual queries than some major providers' free
offerings.

Provider documentation checked on **29 September 2026** lists these
`eth_getLogs` limits:

| Provider | Free access | Paid access |
|---|---|---|
| Alchemy | 10-block range | Ethereum and several other chains have no explicit block-range cap; responses remain capped at 150 MB. |
| QuickNode | 5-block range on the Free Trial plan | 10,000-block range. |
| INNBC public RPC default | Maximum block-number difference of 256, covering up to 257 inclusive blocks | The node operator can configure a different limit. |

Sources: [Alchemy eth_getLogs documentation](https://www.alchemy.com/docs/chains/ethereum/ethereum-api-endpoints/eth-get-logs)
and [QuickNode eth_getLogs documentation](https://www.quicknode.com/docs/ethereum/eth_getLogs).
Provider policies can change and may vary by chain or plan.

This comparison concerns event-log ranges per request, not fee-history
percentiles or overall service capacity. Block times, log density, response
limits, request quotas, and concurrency also affect the work involved.

The finite range works alongside INNBC's RPC admission controls to limit
historical work per accepted request. Applications retrieving longer histories
must split their queries into smaller ranges. This reduces the potential
impact of a single large request; it does not guarantee that every accepted
query is inexpensive or that client pagination is automatic.

<!-- FINISH ALPHA CHANGE 2504 -- explain the public RPC event-log range policy -->

<!-- START ALPHA CHANGE 2470 -- document configurable fee-history resource budgets -->

**Historical fee reporting has bounded, configurable resource budgets**

Code defaults allow at most **256 served blocks**, **10 requested
percentiles**, and **2560 result units** per request.

These are node RPC settings, not genesis parameters or EVM consensus rules.
Operators can change them without regenerating contracts or artifacts.

Generated full-history node environments expose:

```dotenv
INNBC_JSONRPC_FEE_HISTORY_BLOCK_LIMIT=256
INNBC_JSONRPC_FEE_HISTORY_PERCENTILE_LIMIT=10
INNBC_JSONRPC_FEE_HISTORY_RESULT_LIMIT=2560
```

Generated recent-history node environments omit all three overrides and
inherit the code defaults. This does not disable their RPC listeners;
the deployment policy directs public RPC traffic to the full-history node.
Existing environment files are not automatically rewritten.

The equivalent foreground command-line options are:

```text
--json-rpc-fee-history-block-limit 256
--json-rpc-fee-history-percentile-limit 10
--json-rpc-fee-history-result-limit 2560
```

Foreground local commands do not load packaged environment files.
Production services load their per-node environment files; restart the
affected service after changing those settings.

**Zero always selects the corresponding code default. It never disables
a fee-history limit.** The percentile-count setting accepts 1 through 100.
The block and result budgets can be raised explicitly by the operator.

The served block count is the minimum of the requested count, the effective
configured block limit, and the history range through the resolved newest
block, including genesis. A missing or unreadable block in that range
still returns an error; clamping does not restore pruned history.

After range clamping, the independent allocation guard enforces:

```text
effective block count * max(1, requested percentile count) <= result-unit cap
```

Requests without rewards consume one unit per block. Division is used
for the budget check to avoid multiplication overflow. Configuration also
rejects result budgets that cannot safely represent the result-array
element counts on the running platform.

Requests for too many blocks are clamped to the block limit.
Requests exceeding the percentile-count or result-unit limit return
JSON-RPC error `-32005`. Percentile arrays are never silently truncated,
and reward values are never interpolated or fabricated.

For example, an operator can allow 512 blocks with 100 percentiles using:

```dotenv
INNBC_JSONRPC_FEE_HISTORY_BLOCK_LIMIT=512
INNBC_JSONRPC_FEE_HISTORY_PERCENTILE_LIMIT=100
INNBC_JSONRPC_FEE_HISTORY_RESULT_LIMIT=51200
```

All three settings are independent. Increasing one does not automatically
increase the others. Raising these budgets permits more work per request.

The percentile-count limit controls how many positions callers can request,
not which positions they may select. For example, `[1, 50, 100]` requests
three values and fits within the default limit of ten. Percentile values
must be finite, within 0 through 100, and in nondecreasing order.
Fractional values and repeated equal values remain valid.

A default limit of ten accepts requests containing up to ten percentiles.
It is a resource policy, not a guarantee that every wallet or application
requests no more than ten. Operators can raise it up to 100 when needed.

The RPC boundary rejects oversized percentile arrays before resolving the
requested block. The calculation layer independently enforces its resolved
limits, and checks the result budget before historical processing, cache
access, or result allocation.

These controls address request-driven memory amplification and complement
body-size, request-rate, and concurrency limits. The result-unit budget
bounds entries, not every byte allocated or all processing time.

Arbitrary-precision rewards, independent cache ownership, exclusion of
fee-free system transactions, and zero rewards for blocks without eligible
transactions remain unchanged.

<!-- FINISH ALPHA CHANGE 2470 -- document configurable fee-history resource budgets -->

The correction introduces no additional EVM execution or unbounded
historical search.

Receipt-read failures, missing or mismatched receipt data, negative ordinary
transaction tips, and overflow of the included gas total return errors
instead of fabricated reward statistics.

This enhancement concerns historical RPC fee reporting. It does not change
transaction execution charges, balances, fee distribution, validator
payments, or consensus rules.

<!-- FINISH ALPHA CHANGE 2388 -- document accurate full-precision fee-history rewards -->

<!-- START ALPHA CHANGE 2397 -- document mandatory corrected London fee accounting from genesis -->

### 44. Enforced London Fee Correction From Genesis

INNBC Chain requires the `londonfix` fork to activate at block zero.
The canonical genesis already satisfies this requirement. Explicit
validation now prevents imported or programmatically supplied configurations
from selecting the older London fee-accounting implementation.

**Preventing accidental legacy fee accounting**

The legacy implementation can deduct the purchased gas at the transaction's
fee cap while refunding remaining gas at the effective gas price. When the
fee cap exceeds the effective price, this mismatch can overcharge the sender.

The corrected V2 implementation uses the effective gas price for the
upfront deduction, matching the price used for remaining-gas refunds.
INNBC's canonical configuration has selected this corrected implementation
from genesis; the new guard enforces that existing deployment policy.

The required entry under `params.forks` is:

```json
"londonfix": {
  "block": 0
}
```

Missing fork configuration, a missing `londonfix` entry, or activation at
any later block produces an explicit configuration error. Validation
does not silently insert the fork, change its activation height, or
rewrite the genesis file.

**Validation before startup and replay**

The policy is checked at three boundaries:

- Chain import validates the schedule before returning the configuration.
  This covers file-based node startup and the configuration loaded by
  trie reconstruction, snapshot creation, and blockchain backup operations.
- Programmatic server startup validates before creating its logger or
  opening databases.
- Fork-manager initialization validates before clearing or changing
  existing runtime handlers.

Trie reconstruction therefore refuses an unsupported schedule before
opening its blockchain and trie databases. It does not replay legacy
history using a silently substituted fee calculation.

**Preserving the canonical execution behavior**

The legacy implementation and its dependencies remain in the source.
The guard restricts accepted configurations so supported startup and
rebuild paths select V2 from block zero.

This avoids removing or broadly refactoring the existing fork machinery.
It leaves the canonical chain's fee calculation, transaction encoding,
wallet signing, and EVM instruction behavior unchanged.

Historical chains that used the legacy calculation are deliberately
outside this client's supported configuration policy. Operators must
not change an existing chain's fork schedule merely to bypass validation:
doing so can change the results of historical execution.

The additional checks run during configuration loading and initialization.
They add no per-transaction validation step, historical scan, or EVM call.

Regression tests cover rejected schedules, canonical acceptance,
configuration immutability, rejection before logger creation, preservation
of existing handlers after rejected initialization, V2 handler selection,
and early trie-rebuild refusal.

<!-- FINISH ALPHA CHANGE 2397 -- document mandatory corrected London fee accounting from genesis -->

<!-- START ALPHA CHANGE 2432 -- document INNBC-specific testing and legacy harness removal -->

### 45. INNBC Contract And Native Execution Tests With Legacy Fixture Harness Removal

INNBC Chain includes targeted tests for its validator registry, governance contracts, treasury, reward distribution, proxy upgrades, and native-token execution.

The unused top-level Ethereum fixture harness inherited from Polygon Edge has been removed, together with its external `tests/tests` submodule declaration. That harness depended on a large separately downloaded fixture repository and historical fork assumptions. It was excluded from ordinary builds and tests, and required adaptation before it could provide reliable validation of the current client.

Repository setup no longer declares that external fixture download. The removal also eliminates the harness's compile-time embedded-directory dependency. The actively used `helper/tests` package and the repository's other execution, consensus, storage, RPC, and transaction-pool tests remain in place.

**Contract behavior tested with Hardhat**

The INNBC contract tests deploy the actual contract implementations on a local Hardhat test network. Coverage includes:

- Invalid initialization and attempts to initialize contracts twice.
- Validator registration, governance admission, unique identities, stake changes, and BLS-key rotation.
- Active-voter authorization, duplicate-vote rejection, voting deadlines, approval thresholds, and proposal finalization.
- Treasury balance locking, approved transfers, and rollback when a payment or token dependency fails.
- Emergency reward approval, issuance rounding, cooldown behavior, and rollback of partially attempted issuance.
- Combined fee-allocation limits and the explicit storage slots read by the client.
- Community reward accounting, separation of stake principal from rewards, rounding dust, and reentrancy protection.
- Proxy upgrade authorization, approved execution, prevention of repeated execution, and rollback when upgrade initialization fails.

Test-only mocks provide controlled failures for dependencies such as token transfers and BLS verification. This allows the real contracts' failure handling to be exercised reproducibly. Mock BLS responses do not establish cryptographic correctness.

**Integration tests inside the INNBC execution engine**

Go integration tests complement the Hardhat tests by executing embedded contract bytecode through the client's own execution engine:

- Registry admission tests exercise BLS proof-of-possession verification, including wrong-key, wrong-operator, malformed, zero-value, and altered-proof cases.
- Native-token and burn-vault tests exercise initialization permissions, native balance transfers, allowance handling, authorized issuance, burning, and rollback after failed operations.

These integration tests cover chain-specific behavior that an ordinary ERC-20 mock does not reproduce.

**Running the tests**

Run the Solidity contract suite from the repository root:

```bash
(
  cd core-contracts &&
  TS_NODE_TRANSPILE_ONLY=1 ./node_modules/.bin/hardhat test --network hardhat
)
```

Hardhat runs its compilation task before the tests and may update local artifacts, caches, and generated contract documentation. This command does not regenerate the Go client's embedded artifacts or modify genesis.

Run the focused native integration tests separately:

```bash
go test -count=1 ./consensus/innbcbft \
-run '^(TestContractAudit_NativeTokenAndBurnVault|TestRegistryBLSPoPAdmission)$' -v
```

Repeat those integration tests with the race detector:

```bash
go test -race -count=10 ./consensus/innbcbft \
-run '^(TestContractAudit_NativeTokenAndBurnVault|TestRegistryBLSPoPAdmission)$'
```

The Hardhat suite is separate from `go test ./...`; both are needed to exercise their respective coverage.

These tests strengthen validation of INNBC-specific behavior. They do not claim to replace the entire Ethereum conformance suite or prove correctness for every possible contract interaction. Removing the legacy harness changes test infrastructure, not production execution rules, contract implementations, or embedded bytecode.

<!-- FINISH ALPHA CHANGE 2432 -- document INNBC-specific testing and legacy harness removal -->

<!-- START ALPHA CHANGE 2531 -- document malformed typed-payload rejection -->

### 46. Safe Rejection Of Incomplete Typed Block And Receipt Data

The shared RLP list decoder checks that a transaction-type marker has a following payload before advancing to that payload.

Previously, a list ending with a recognized type marker could cause an out-of-range access. For peer-supplied blocks, decoding occurs before finalized-block verification, so a malformed encoding could reach this failure without carrying a valid finalized certificate.

The corrected decoder returns an error instead. The protection applies to both block transaction lists and receipt lists using the shared decoder, including block decoding through synchronization and consensus proposal-hash validation.

This is a constant-time boundary check. Valid typed and legacy encodings retain their existing behavior; the change does not introduce additional state reads or EVM execution.

Regression tests cover trailing type markers, valid encoding round trips, the synchronization decoding boundary, and proposal-hash validation.

<!-- FINISH ALPHA CHANGE 2531 -- document malformed typed-payload rejection -->

<!-- START ALPHA CHANGE 2532 -- document configurable fee-history allocation budgets -->

### 47. Bounded Fee-History Requests With Configurable Resource Budgets

`eth_feeHistory` now bounds both the historical range it serves and the number of reward values a caller can request.

A request-body limit alone does not bound the work of this method. A relatively small JSON array containing many repeated percentiles can otherwise produce a large number of reward allocations across multiple blocks. Rate limits and concurrent-call limits also do not bound the allocation size of one accepted request.

The implementation therefore uses three independent settings:

| Setting | Code default | Purpose |
| --- | --- | --- |
| Fee-history block limit | 256 | Maximum number of historical blocks served by one request. |
| Reward-percentile count limit | 10 | Maximum number of requested percentile positions per block. |
| Result-unit limit | 2560 | Maximum effective blocks multiplied by reward width. |

The result budget is calculated as:

```text
served blocks * max(1, requested percentile count)
```

Requests without reward percentiles still consume one result unit per served block. Result units are an allocation/work budget, not a byte-accurate memory measurement.

The served block count is clamped to the configured block limit and available history. The result-budget check then uses that effective count. Oversized percentile arrays and requests exceeding the result budget are rejected explicitly with JSON-RPC limit error `-32005`; percentile arrays are not silently truncated or approximated.

Validation occurs in the gas-price helper for direct callers as well as at the RPC boundary. The RPC percentile check runs before resolving the requested newest block. Budget arithmetic uses division to avoid overflow from multiplying request-controlled values.

Zero selects the code default for each setting; it does not disable protection. Positive block and result limits remain configurable. The percentile count can be configured from 1 through the fixed maximum of 100.

For example, serving 512 blocks with 100 requested percentiles requires a block limit of at least 512, a percentile limit of 100, and a result budget of at least 51200.

The production environment settings are:

```text
INNBC_JSONRPC_FEE_HISTORY_BLOCK_LIMIT=256
INNBC_JSONRPC_FEE_HISTORY_PERCENTILE_LIMIT=10
INNBC_JSONRPC_FEE_HISTORY_RESULT_LIMIT=2560
```

Generated node1 configuration includes these explicit full-history RPC settings. Generated recent-node environments omit them and use the code defaults through the shared service configuration. An operator can still add explicit overrides to a recent node's environment.

The correction preserves exact gas-weighted percentile selection, arbitrary-precision rewards, exclusion of fee-free system transactions from reward samples, and independent ownership of cached reward values. It does not change `eth_gasPrice`, `eth_maxPriorityFeePerGas`, or execution fee accounting.

Regression tests cover early rejection, configurable limits, zero-to-default behavior, arithmetic boundaries, and RPC error mapping.

<!-- FINISH ALPHA CHANGE 2532 -- document configurable fee-history allocation budgets -->

<!-- START ALPHA CHANGE 2533 -- document finite event-log ranges and genesis-only queries -->

### 48. Finite Event-Log Query Defaults And Safe Genesis-Only Queries

Historical event-log queries use a separate range setting from `eth_feeHistory`:

```text
INNBC_JSONRPC_BLOCK_RANGE_LIMIT=256
```

A zero setting now selects the finite default of 256 instead of disabling the range restriction. Positive overrides remain supported.

This normalization is applied through configuration handling, server startup, direct filter-manager construction, and range enforcement. Excessive ranges are rejected before scanning their blocks.

The existing range convention measures the difference between the ending and starting block numbers:

```text
toBlock - fromBlock <= configured range limit
```

Consequently, a difference of 256 can cover 257 block numbers because both endpoints are included. This setting limits the width of a single historical query, not how old the requested history may be. Clients can retrieve longer available histories using multiple bounded requests.

Generated full-history node1 environments include the explicit setting. Generated recent-node environments use the shared default unless an operator adds an override.

A separate boundary correction handles queries covering only genesis. Under the existing policy of excluding genesis from the log scan, a `0` to `0` query now returns an empty result before unsigned subtraction. It no longer produces a false range-limit rejection.

These limits complement the existing RPC admission controls by bounding the historical range of an individual call. They do not establish a fixed CPU cost or response size for every permitted range, because blocks can contain different amounts of transaction and log data.

Regression tests cover zero normalization, positive overrides, rejection before block reads, and genesis-only queries.

<!-- FINISH ALPHA CHANGE 2533 -- document finite event-log ranges and genesis-only queries -->

<!-- START ALPHA CHANGE 2534 -- document null log-filter validation -->

### 49. Null Log-Filter Rejection Across HTTP And WebSocket RPC

Log-filter RPC methods reject a null filter object before it can be stored or used.

Previously, `eth_newFilter` or a WebSocket logs subscription could register a nil query. A later log-producing block could then trigger a nil-pointer panic in the filter manager's background goroutine. `eth_getLogs` could also fail immediately when given a null query.

The correction validates each relevant boundary:

- `eth_newFilter` and `eth_getLogs` reject nil queries.
- WebSocket logs-subscription decoding rejects a null filter object.
- The dispatcher preserves the intended invalid-parameters error code, `-32602`.
- The filter manager independently refuses to register a nil query and rejects nil direct log-query calls.

Valid empty objects, such as `{}`, remain accepted. Null entries inside topic arrays retain their wildcard meaning; they are distinct from a null filter object.

The protection prevents the malformed query from entering the background delivery path. It does not depend on waiting for filter expiry or recovering from a later panic.

Regression tests exercise HTTP and WebSocket dispatch, direct filter-manager calls, valid empty and topic-wildcard filters, and subsequent log delivery.

<!-- FINISH ALPHA CHANGE 2534 -- document null log-filter validation -->

<!-- START ALPHA CHANGE 2535 -- document consistent whole-second block intervals -->

### 50. Consistent Whole-Second Block-Time Validation

Genesis generation and node startup enforce the same block-interval requirements:

- The interval must be at least one second.
- The interval must be an exact whole number of seconds.

The default remains `2s`. Values such as `1s`, `2s`, and `2000ms` are valid; values such as `500ms`, `1.5s`, and `2.5s` are rejected with an explanatory error.

Previously, genesis generation could accept a duration that startup rejected. Fractional durations of at least one second could also pass startup even though header timestamps store whole seconds.

<!-- START ALPHA CHANGE 3330 -- align interval validation documentation with intentional chain-clock progression -->

Requiring whole-second intervals avoids discarding a fractional component when calculating the next header timestamp. The shared checked timestamp calculation and exact header-validation rule are documented in point 52.

This configuration validation runs during genesis generation, chain configuration import and runtime initialization. It adds no recurring contract calls or historical reads.

Production delays and outages intentionally pause chain-clock progression. The next block still advances the parent timestamp by exactly the configured interval; timestamps are not adjusted to catch up with real-world time.

<!-- FINISH ALPHA CHANGE 3330 -- align interval validation documentation with intentional chain-clock progression -->

Regression tests cover minimum values, fractional values, whole-second equivalents, and startup configuration decoding.

<!-- FINISH ALPHA CHANGE 2535 -- document consistent whole-second block intervals -->

<!-- START ALPHA CHANGE 2536 -- document bounded peer connection configuration -->

### 51. Validated And Bounded Peer Connection Configuration

Peer-limit configuration is validated before networking is constructed.

Previously, a negative outbound value other than the `-1` unset sentinel could reach the dialer's channel allocation and panic after startup had begun. An extremely large positive outbound value could also cause excessive work while initializing one dialing slot per configured connection.

The corrected configuration path:

- Rejects raw values below the `-1` unset sentinel.
- Resolves unset values before runtime use.
- Preserves valid zero limits, including resolving a zero total into zero inbound and outbound limits.
- Checks inbound-plus-outbound arithmetic for overflow.
- Validates resolved values before publishing them to the runtime configuration.

Network construction independently validates the resolved configuration before key setup, listener creation, or dialer startup, protecting direct callers that bypass the CLI.

A shared ceiling of 1000 applies to each resolved total, inbound, and outbound limit. Combined inbound and outbound capacity must also remain at or below 1000.

The ordinary defaults remain:

```text
Total peer connections:    40
Inbound peer connections:  32
Outbound peer connections:  8
```

These are direct connection limits per node, not limits on the number of validators in the chain. The ceiling is a configuration safeguard, not a claim that 1000 simultaneous connections have been performance-tested.

Regression tests cover invalid negative values, unset defaults, zero limits, arithmetic overflow, excessive positive values, and acceptance at the configured ceiling. Constructor tests verify rejection before networking dependencies are accessed.

<!-- FINISH ALPHA CHANGE 2536 -- document bounded peer connection configuration -->

<!-- START ALPHA CHANGE 3324 -- replace future-time allowances with the strict chain clock -->

### 52. Strict Chain-Clock Timestamp Validation

Block construction and header validation share this invariant:

```text
timestamp = parent timestamp + configured block interval
```

The interval must be at least one second and an exact whole number of seconds. Checked integer arithmetic rejects timestamp addition overflow and values outside the execution context's supported signed 64-bit range.

The invariant applies to locally constructed proposals, incoming proposals before execution, and finalized-header verification.

Timestamp validity does not consult the validator's local clock. The former future-time allowance and `blockTimeDrift` configuration have been retired. Explicit use of the retired field is rejected, including zero and null values.

Genesis supplies the initial timestamp. Configuration validation rejects a genesis timestamp that cannot produce a supported successor.

INNBC timestamps measure chain progression rather than elapsed real-world time. Outages and production delays pause that progression. Timestamp-dependent contract deadlines follow the same chain clock.

Production pacing and operational timeouts remain in place. This correction requires no additional transaction replay, contract calls or historical lookups.

<!-- START ALPHA CHANGE 3331 -- document strict chain-clock regression coverage -->

Regression tests cover exact increments at multiple configured intervals, rejection of smaller and larger increments, equal and decreasing timestamps, and valid chain timestamps far behind or ahead of local wall-clock time.

Additional tests cover invalid durations, timestamp addition overflow, execution-range overflow, agreement between construction and validation, rejection of incoming timestamp violations before execution, finalized-header enforcement, and rejection of retired drift configuration and CLI flags.

The current-workflow integration fixture compares RPC block timestamps against the exact interval loaded from its generated genesis configuration. Its assertion requires equality with the parent timestamp plus that interval.

<!-- FINISH ALPHA CHANGE 3331 -- document strict chain-clock regression coverage -->

<!-- START ALPHA CHANGE 3828 -- document deferred historical block-interval upgrade support -->

#### Future Block-Interval Changes

The production deployment retains the two-second block interval. Historical interval selection is intentionally deferred until an interval-changing protocol upgrade is planned.

The current software supplies one configured interval to block construction, local proposal validation, incoming proposal validation and finalized-header verification. Downloaded blocks pass through that finalized-header verification.

Consequently, replacing an established chain's global interval from `2s` to `1s` makes unknown historical two-second headers fail validation: the node expects a one-second increment where those historical blocks correctly contain two seconds.

Already-known headers can bypass verification. An existing node reopening its database and continuing operation therefore does not demonstrate that a fresh or recovering node can synchronize across an interval change.

This limitation does not affect a deployment that retains its original interval. A new chain can also select a supported duration from its beginning; the limitation concerns changing the interval of an established chain.

#### Requirements Before A Future Interval Upgrade

A future upgrade must preserve the original interval for historical blocks and introduce the new interval at an agreed activation height.

For a two-second to one-second transition at height H:

| Child block number | Required timestamp increment |
|---|---:|
| Below H | 2 seconds |
| H and later | 1 second |

Block H receives the first one-second increment relative to block H - 1.

The upgraded software must select the interval using the child block's number and apply the same selection to:

- Block construction and its production pacing.
- Validation of locally constructed proposals.
- Validation of incoming proposals.
- Finalized-header verification used during synchronization.

The existing exact timestamp equality and checked arithmetic must remain enforced. The activation height and historical interval rules must remain available to validators, recovering nodes and future joining nodes.

Before such an upgrade is considered supported, regression coverage must demonstrate:

- Correct construction and validation immediately before, at and after activation.
- Rejection of the wrong interval on either side of activation.
- Fresh synchronization across the transition.
- Restart and catch-up from before activation.
- Consistent production pacing and suitable operational timeouts.

The upgrade must be deployed consistently before activation while preserving the existing genesis block and historical chain data. Block-count retention periods and governance windows must also be reviewed for their changed nominal elapsed durations.

No interval-changing upgrade or environment/CLI activation scheduler is implemented by this documentation. The current operating policy remains an unchanged two-second interval; any future change requires its own reviewed and tested protocol release.

<!-- FINISH ALPHA CHANGE 3828 -- document deferred historical block-interval upgrade support -->

<!-- FINISH ALPHA CHANGE 3324 -- replace future-time allowances with the strict chain clock -->

<!-- START ALPHA CHANGE 2540 -- document RPC filter lifecycle and timeout hardening -->

### 53. Safe Concurrent RPC Filter Removal And Timeout Processing

RPC filter polling, removal, and timeout processing now preserve consistent registration and timeout-heap ownership during concurrent requests.

Previously, a polling request could retrieve a filter, release the manager lock, and later refresh its timeout after another request had uninstalled it. This could reinsert the removed filter into the timeout heap without restoring its registration. Once the detached entry expired, the worker could repeatedly attempt to uninstall its missing ID, consuming CPU and writing repeated warnings.

The correction protects both registration and expiry:

- Timeout refresh checks that the same filter object is still registered while holding the manager's write lock.
- A completed uninstall cannot be undone by a polling request retaining an older filter pointer.
- Expiry processing rechecks the current deadline under the write lock, so an old timer cannot remove a filter whose timeout was successfully refreshed.
- Detached timeout-heap entries are discarded directly, allowing expiry processing to continue.
- A buffered notification retains pending timeout changes until the worker can reschedule.
- The worker reuses a timer, disables its timeout channel when the heap is empty, and stops the timer when it exits.

A poll that obtained its results before a concurrent uninstall may still return those results. It cannot restore the removed filter or its timeout.

The protection applies to polling filters managed by the shared filter manager. Existing filter-capacity limits, WebSocket cleanup, and RPC admission controls remain in place. Request-rate limits alone would not prevent the former internal expiry loop after it had been triggered.

Regression tests force the polling/uninstall interleaving and cover replacement-object identity, refreshed deadlines, detached-entry cleanup by the running worker, and pending timeout notifications.

This change concerns RPC filter lifecycle management. It introduces no consensus changes, contract calls, historical block scans, or additional EVM execution.

<!-- FINISH ALPHA CHANGE 2540 -- document RPC filter lifecycle and timeout hardening -->

<!-- START ALPHA CHANGE 2554 -- document bounded transaction-event subscriptions and safe cleanup -->

### 54. Bounded Transaction-Event Queues And Safe Subscription Recovery

Transaction-event subscriptions now have bounded queue storage and explicit overflow handling. A stalled subscriber can no longer accumulate an unlimited backlog as transaction activity continues.

Previously, a subscriber whose event delivery stopped progressing could retain an ever-growing slice of queued events. The transaction pool's slot limit did not bound this accumulation: transactions and replacements could continue producing events over the lifetime of the subscription.

Each subscription now uses a queue limited to 4,096 pending events:

- Queue storage is allocated on first use.
- Events retain their queue order.
- Consumed entries release their event references.
- Overflow terminates the affected subscription and releases its queued storage.
- Producers do not wait for the subscriber to receive events.
- The subscription worker exits and removes its registration.
- Manager shutdown closes existing subscriptions and rejects new subscriptions.

<!-- START ALPHA CHANGE 2560 -- document global event subscription admission limits -->

The 4,096-event limit applies to each subscription's pending queue. The node also permits at most 32 operator transaction-event stream handlers, with at most 32 ordinary queue registrations and one separately reserved internal JSON-RPC event-feed registration.

Excess operator subscriptions are rejected with gRPC `ResourceExhausted`. Admission is synchronized so concurrent requests cannot exceed the limits. These are fixed code limits shared by all node modes.

An operator slot remains occupied until its handler returns. Queue overflow can release queued events and remove the queue registration while a transport send remains blocked, but it does not release that handler's operator slot prematurely.

The internal reservation allows the JSON-RPC event feed to start or reconnect even when operator capacity is full. Only one internal feed registration is admitted at a time. Cancellation releases that reservation, and manager shutdown rejects new subscriptions.

These limits bound transaction-event subscriptions. They do not cap unrelated gRPC methods or provide authentication.

<!-- FINISH ALPHA CHANGE 2560 -- document global event subscription admission limits -->

For operator gRPC streams, queue overflow is reported as `ResourceExhausted` when the handler can complete. A transport send that is already blocked may remain blocked until the transport progresses or terminates, but the subscription's queue is released independently and no longer accumulates events.

Subscription cleanup also preserves object ownership. Cancellation checks both the subscription ID and the original subscription object while holding the manager lock. If an ID is reused, delayed cleanup from an older worker, gRPC handler, or unsubscribe callback cannot cancel the replacement. Repeated cancellation remains safe and does not decrement the subscription count again.

The internal JSON-RPC transaction-event feed recovers explicitly when its subscription ends:

- Block/log event processing and filter expiry continue while the transaction feed is unavailable.
- Pending-transaction filters are invalidated because their event history may contain a gap.
- WebSocket connections carrying affected pending subscriptions are closed so clients can detect the interruption and recreate their subscriptions. Other filters sharing those connections are removed as part of cleanup.
- Unrelated connections and polling block/log filters remain available.
- New pending-transaction filters are rejected while the feed is unavailable.
- The filter manager retries the local transaction-event subscription at one-second intervals and cancels retry activity on shutdown.

This retry concerns the local transaction pool only. It performs no peer-directory refresh, registry query, gas estimation, or contract execution. Reconnection does not replay missed events or silently restore invalidated client subscriptions.

Regression tests cover queue capacity and ordering, storage release after overflow, concurrent publication and closure, stalled gRPC delivery, shutdown admission, internal RPC feed recovery, and continued filter expiry. Deterministic ID-reuse tests verify that stale cleanup preserves the replacement subscription, its event delivery, and the subscription count.

This hardening protects event delivery and resource ownership without changing transaction admission, execution, fees, or consensus rules.

<!-- FINISH ALPHA CHANGE 2554 -- document bounded transaction-event subscriptions and safe cleanup -->

<!-- START ALPHA CHANGE 2557 -- document synchronized block-filter cursor consumption -->

### 55. Consistent Block-Filter Results During Concurrent Polling

Concurrent `eth_getFilterChanges` requests for the same block filter now consume its pending block updates under the filter's mutex.

Previously, the filter read its cursor without locking and acquired the mutex only when assigning the new cursor. Concurrent requests could collect the same updates. A delayed request could also overwrite a newer cursor with an older position, causing later polls to return previously consumed block hashes.

The corrected operation holds one lock while reading the cursor, collecting updates, and advancing the cursor. This removes the unsynchronized pointer access and prevents concurrent polls from consuming the same updates or moving the cursor backward.

Synchronization is local to each filter. Response encoding and WebSocket writes occur after the cursor lock is released. Concurrent requests may complete in a different order, but cursor consumption remains serialized.

Regression tests exercise concurrent polling with both existing block updates and blocks appended during polling. They check that every published block appears exactly once across the collected responses, that each caller observes increasing block order, and that a final poll returns no already-consumed updates.

This correction affects RPC notification consistency. It adds no historical database scans, contract calls, or EVM execution and does not change consensus rules.

<!-- FINISH ALPHA CHANGE 2557 -- document synchronized block-filter cursor consumption -->

<!-- START ALPHA CHANGE 2564 -- document enforced loopback operator binding -->

### 56. Enforced Loopback Binding For Operator gRPC

The node enforces loopback-only binding for its operator gRPC listener.

The packaged service already defaults to `127.0.0.1:9632`. Runtime validation now prevents an accidental configuration override from exposing the listener directly on wildcard, LAN, or public addresses.

Validation checks the resolved IP address. IPv4 loopback addresses and IPv6 `::1` are accepted, including the usual `127.0.0.1:9632` and `[::1]:9632` configurations. Explicit addresses such as `0.0.0.0:9632`, `[::]:9632`, or a LAN address are rejected with a clear error rather than silently rewritten. The existing address resolver continues to use localhost when the host is omitted.

The shared check runs during server construction before logger creation or runtime startup and again at the gRPC listener boundary before service registration and binding. It applies to CLI configuration, environment-derived configuration, and direct server callers.

This protects the node's own operator listener. It does not prevent a separately configured local proxy, tunnel, or forwarding service from exposing that listener, and it does not authenticate local callers.

Public JSON-RPC and peer networking use separate listeners and retain their own configuration. The operator subscription limits and bounded event queues continue to protect resources accessible through the local operator API.

Regression tests cover accepted loopback addresses, rejected wildcard and non-loopback addresses, missing address configuration, rejection before startup side effects, and enforcement at the listener boundary.

<!-- FINISH ALPHA CHANGE 2564 -- document enforced loopback operator binding -->

<!-- START ALPHA CHANGE 2570 -- document contextual checkpoint-round validation -->

### 57. Contextual Checkpoint-Round Validation Before Proposal Execution

Proposal validation now checks the relationship between the block's embedded checkpoint round and the consensus proposal context.

Previously, an authorized proposer could supply an otherwise acceptable block whose checkpoint declared an enormous round despite the proposal belonging to a small round. Subsequent proposer-priority updates use the embedded round to determine replay work. Accepting inconsistent metadata could therefore cause excessive replay after finalization, or arithmetic overflow when incrementing the maximum unsigned round value.

The contextual correction rejects missing checkpoints and checkpoint rounds greater than the proposal round during the existing proposal-hash validation, before candidate execution.

Prepared certificates receive additional contextual checks:

- The certificate's contained proposal round must equal its proposal message's signed view round.
- The certificate's actual raw proposal must match its claimed hash and satisfy the checkpoint-round check in that certified context.
- The separately supplied last-prepared proposal must match the certificate's round and hash.
- Invalid supplied certificates reject the enclosing proposal instead of being ignored.
- A later-round candidate must match the selected prepared evidence before execution.

Candidate execution occurs only after the applicable contextual and certificate checks succeed, once per successful candidate-validation invocation. Rejected candidates do not reach execution or PREPARE voting through these validation paths.

Legitimate prepared-block reuse remains supported. A block embedding round 2 can be certified in round 5 and reproposed in round 7. The rule permits an earlier embedded round; it does not impose unconditional equality with the later proposing round.

Prepared-proposal matching validates the supplied raw proposal against the certificate's proposal hash once, then compares prepare-message hashes directly. This avoids decoding the same supplied block again for every prepare message. Certificate validation separately checks the certificate's contained raw proposal.

These checks add no registry calls, historical searches, or EVM execution. The runtime comparison reuses an existing decode, while prepared-certificate validation adds raw-proposal validation on that path. No measured CPU-overhead claim is made.

Regression tests cover real checkpoint decoding, missing checkpoints, huge embedded rounds at round zero, certified-round mismatches, raw-proposal/hash mismatches, invalid supplied evidence, candidate disagreement with prepared evidence, and legitimate reuse across successive rounds. Core control-flow tests independently exercise certificate validation, current and future round-change collection, and future-proposal validation. They assert that validation began and that rejected candidates cause neither execution nor PREPARE voting.

Scope limitation: this correction binds checkpoint metadata to its validated proposal context. It does not introduce an absolute consensus-round ceiling or independently bound replay work when the authenticated round itself is very large. The remaining exposure is intentionally retained as the accepted limitation documented below. This correction provides contextual validation, not complete protection against excessive round-based computation.

<!-- FINISH ALPHA CHANGE 2570 -- document contextual checkpoint-round validation -->

<!-- START ALPHA CHANGE 2571 -- document proposer-replay analysis and accepted availability limitation -->

#### Proposer-Replay Analysis and Accepted Limitation

A read-only source analysis examined proposer calculation, priority normalization, validator-set transitions, round progression, message admission, finalized-header validation, and runtime snapshot replay.

The proposer-priority calculation is deterministic for a fixed starting snapshot and requested round. Each iteration adds validator voting powers to their priorities, selects the highest-priority validator using the existing address tie-break, and subtracts total voting power from that validator. Earlier selections therefore affect later selections.

The calculation itself does not depend on network load or message arrival times. Network conditions can affect which rounds consensus reaches, while hardware, validator count, and integer sizes affect calculation time.

An uncached `ProposerSnapshot.CalcProposer` performs `round + 1` priority iterations. `ProposerCalculator.updatePerBlock` separately replays `Checkpoint.BlockRound + 1` iterations when advancing proposer state. Each iteration scans the validator set.

No general acceleration with proven identical proposer selection and resulting priorities was established by this analysis. Any future optimization must preserve the existing normalization, integer rounding, address tie-breaking, accumulated priorities, and validator-set transitions. This result does not establish that such an optimization is impossible.

The existing eight-round admission window predates the contextual correction. It limits how far ahead an incoming message may be relative to the local round; it moves forward as the local round advances. It does not impose an absolute maximum round.

Authenticated catch-up remains supported. For example, a validator at round 1 can adopt qualifying evidence for round 6 without waiting through intermediate rounds. An uncached proposer calculation for destination round 6 still performs seven priority iterations.

#### Accepted Deployment Decision

INNBC intentionally does not add an absolute consensus-round or replay-work ceiling in this correction.

A finite ceiling would introduce a new acceptance rule and require defined behavior when round progression reaches it. This source analysis did not identify a cutoff that preserves all legitimate progression under the existing rules. No benchmarks or runtime experiments were performed. A limit selected from expected operating conditions could prevent progress during prolonged validator unavailability or disagreement.

The deployment therefore retains the existing proposer-priority algorithm and round progression, together with the contextual checkpoint-round checks and existing authenticated catch-up rules. No local elapsed-time acceptance rule, round clamping, or round reset is introduced.

This is an accepted availability-risk compromise intended to avoid an additional consensus restriction and an unproven algorithm change.

#### Residual Availability Risk

The following limitations remain explicitly accepted:

- Very large authenticated rounds can require excessive proposer-calculation work even when the checkpoint round satisfies the contextual inequality.
- Finalized-header validation still requires valid committed signatures and checkpoint consistency, but does not independently impose an absolute checkpoint-round ceiling.
- A finalized block satisfying those checks can still require excessive proposer-state replay during subsequent runtime advancement or recovery.
- Incrementing `MaxUint64` wraps to zero in the existing `round + 1` and `BlockRound + 1` calculations. The priority routine then returns its existing non-positive-iteration error. This arithmetic edge is not corrected by the contextual patch or this documentation decision.

Authentication, quorum requirements, and the moving admission window remain important protections, but do not constitute an absolute computation bound.

The contextual mismatch is corrected. Excessive replay work and arithmetic-edge failures remain documented residual availability risks. 

<!-- FINISH ALPHA CHANGE 2571 -- document proposer-replay analysis and accepted availability limitation -->

<!-- START ALPHA CHANGE 2580 -- document authoritative live epochs and safe validator snapshot lookup -->

### 58. Authoritative Live Epoch Validation And Bounded Snapshot Cache Selection

Live proposal validation now requires the checkpoint epoch to equal the authoritative epoch already held by the consensus FSM.

Previously, the shared checkpoint check permitted either the parent's epoch or its successor without independently establishing whether the proposed height began a new epoch. An otherwise acceptable live proposal could therefore declare an epoch advance prematurely. Subsequent honest proposals using the runtime's actual epoch could then be rejected as an epoch decrease relative to the malformed parent.

The new comparison runs after the missing-checkpoint guard and before parent-signature validation, state-transaction checks, and block execution. It reuses decoded checkpoint data and existing runtime metadata, requiring no additional registry lookup, historical search, or EVM execution.

The existing epoch convention is preserved:

- Genesis has checkpoint epoch zero.
- Ordinary blocks in epoch E declare E.
- The ending block of epoch E still declares E.
- The first block after that boundary declares E + 1.

Prepared-block reuse across consensus rounds remains valid because the proposal stays at the same height and retains the same expected epoch.

Validator snapshot lookup now rejects missing checkpoints and epoch zero on non-genesis blocks before epoch-ending detection or subtraction. An additional guard prevents subtraction from wrapping even for malformed genesis input. Canonical genesis epoch zero remains supported. Epoch-ending detection also checks both the current and successor checkpoints before accessing their epoch fields.

These guards close the path where decrementing epoch zero produced MaxUint64 and caused an effectively nonterminating backward cache search.

Snapshot cache selection now examines actual cached entries instead of counting backward through every numerical epoch. An exact cache hit still returns immediately. Otherwise, the lookup selects the greatest cached epoch at or below the requested epoch, ignores nil and future entries, and retains the existing database lookup and comparison with a newer eligible database snapshot.

The in-memory selection cost therefore depends on the number of cached entries rather than the numerical distance between epochs. No epoch ceiling or additional database query is introduced. Subsequent historical snapshot reconstruction retains its existing behavior.

Regression tests cover first-block validation, ordinary and boundary epochs, validator changes, prepared-block reuse, premature or stale epoch declarations, epoch-zero rejection, missing checkpoints, and rejection before block execution. Snapshot tests cover canonical genesis, unchanged storage after rejected input, exact cache hits, sparse caches, future-entry exclusion, database-versus-memory selection, and MaxUint64 requests.

The tests exercise actual checkpoint decoding and FSM validation with mocked block execution, plus snapshot lookup using the test MDBX state. They do not constitute an end-to-end network voting or running-chain epoch-transition test.

This correction addresses live proposal validation and safe snapshot lookup. It does not repair malformed finalized history, implement the broader finalized-import expectations deferred in section 41, or resolve the separate proposer-replay risks documented in section 57.

<!-- FINISH ALPHA CHANGE 2580 -- document authoritative live epochs and safe validator snapshot lookup -->

<!-- START ALPHA CHANGE 2603 -- document proposal execution ownership and confirmed consensus insertion -->

### 59. Proposal Execution Ownership And Confirmed Consensus Insertion

Proposal execution results now belong explicitly to the proposal selected by consensus.

Previously, construction and validation published their execution results into a shared FSM target. Concurrent current-round and future-round execution could leave the selected proposal paired with another proposal's target. The existing hash comparison prevented insertion of the mismatched block, but local insertion could fail. That error was logged without being returned to the consensus core, which could then follow its normal sequence-completion path.

<!-- START ALPHA CHANGE 2625 -- distinguish current execution ownership from canonical completion -->
Construction and validation run on private FSM copies. Each execution result records its originating runtime, owning FSM, proposal bytes, parent identity, block height, and block hash. Adoption and voting require a current execution result. Insertion rejects mismatched or foreign-runtime results, but can complete an already canonical decision after runtime advancement or FSM replacement by explicitly confirming the selected block's canonical identity.
<!-- FINISH ALPHA CHANGE 2625 -- distinguish current execution ownership from canonical completion -->

Current-round adoption, local construction, and future-round adoption carry the proposal together with its execution result. Future-round adoption occurs after the old workers have been cancelled and joined. A successful insertion or latched failure occurring during teardown takes precedence over a pending future proposal.

A cancelled round worker checks cancellation after validation before adopting its completed candidate. Regression tests force this ordering rather than relying on timing.

Prepared-block reuse remains supported. A retained execution result can accompany the same prepared proposal through later rounds without repeating EVM execution. A prepared proposal learned without a retained result still requires normal validation and execution.

Execution-result access is synchronized. Insertion copies the selected block's header before attaching committed seals, preserving the source execution header. The core retains selected and latest-prepared execution results rather than accumulating a map of speculative results.

**Confirmed consensus insertion**

<!-- START ALPHA CHANGE 2626 -- document targeted confirmation of retained canonical history -->
Consensus commits use `WriteFullBlockConfirmed`. Under the existing canonical write lock, this operation succeeds after:

- writing the selected block through the existing canonical write path;
- confirming that the current canonical head has exactly the selected block's height and hash; or
- reading the canonical hash at the selected height when the head is higher, and confirming that it matches the selected block.

A higher head alone is not proof of insertion. Confirmation uses one targeted canonical-hash lookup, without loading a historical header or body, scanning backward, or executing the EVM. There is no height-distance ceiling. Conflicting or unavailable canonical identities return `ErrCanonicalBlockNotConfirmed`.

If the runtime already covers the selected height, insertion can confirm the decision without rewriting the block or replaying covered runtime effects. The execution result must still belong to the originating runtime and match its original proposal, parent, height, and hash. This completion path does not authorize stale results for adoption or voting.
<!-- FINISH ALPHA CHANGE 2626 -- document targeted confirmation of retained canonical history -->

Insertion errors now reach the consensus core. The core marks insertion successful only after the backend returns success. It latches failures and retains the committed proposal, seals, and execution result in memory instead of silently completing or resetting the sequence.

Failure reporting distinguishes execution-selection failure, insertion failure, and runtime-advancement failure after a successful commit. A failed storage operation is not assumed to have left storage unchanged. These failures enter the existing fatal-recovery path; this change does not introduce automatic retries or persist the retained consensus decision across restart.

**Recovery behavior and performance**

<!-- START ALPHA CHANGE 2627 -- describe advanced-head availability and remaining missing-history recovery -->
Synchronization advancing the head no longer causes rejection solely because the selected height is lower. If retained canonical history confirms the selected hash, insertion succeeds. If runtime processing still needs to cover that block, the existing advancement callback remains active; if runtime processing already covers it, completion preserves the newer runtime state.

Unavailable or pruned canonical entries still require recovery because this correction does not invent proof of insertion. The existing fatal-error policy remains for failed confirmation and other insertion or advancement failures. Head advancement after successful confirmation does not itself invalidate that confirmation.

The correction adds bounded copying, proposal-byte comparisons, execution-result synchronization, constant-time canonical-head comparisons, and one targeted canonical-hash read when confirming a block below the head. It adds no registry calls, backward history scans, or additional EVM execution for insertion or reuse of an already retained prepared result. It does not claim zero CPU overhead or a measured throughput improvement.
<!-- FINISH ALPHA CHANGE 2627 -- describe advanced-head availability and remaining missing-history recovery -->

Proposer selection, round progression, checkpoint-round rules, and epoch rules remain unchanged. This correction introduces no absolute round ceiling.

<!-- START ALPHA CHANGE 2630 -- describe regression coverage without test-run status -->
**Regression coverage**
<!-- FINISH ALPHA CHANGE 2630 -- describe regression coverage without test-run status -->

Regression coverage includes:

- future proposal B completing before current proposal A, with explicit cancellation and worker teardown;
- insertion overlapping speculative validation;
- stale execution-result rejection and preservation of the unsealed source header;
- prepared-result reuse;
- insertion and post-insertion advancement failures;
- successful insertion or failure taking precedence over a pending future proposal;
- exact, conflicting, and advanced canonical heads, including advancement while waiting for the write lock.

The coordinator tests use controlled backends. Runtime confirmation tests exercise the canonical writer with in-memory storage and controlled execution. These tests do not establish disk crash durability or end-to-end network behavior.

<!-- START ALPHA CHANGE 2631 -- retain canonical-history regression coverage without test-run status -->
Additional canonical-history regressions cover matching and conflicting entries, unavailable history, a large height gap with one targeted read, head advancement while waiting for the write lock, runtime advancement before insertion, FSM replacement, foreign-runtime rejection, and repeated completion without repeated writes or runtime effects.
<!-- FINISH ALPHA CHANGE 2631 -- retain canonical-history regression coverage without test-run status -->

<!-- START ALPHA CHANGE 2604 -- explain the accepted synchronization compatibility tradeoff -->

**Synchronization compatibility policy and accepted limitation**

Synchronization intentionally retains the existing duplicate-handling behavior of `WriteFullBlock` and `OnBlockInserted`. An already-covered block height can be skipped without comparing block hashes, and synchronization can still invoke its callback with the supplied block.

<!-- START ALPHA CHANGE 2628 -- retain the separate synchronization callback policy -->
This behavior is retained to preserve existing duplicate processing and legitimate catch-up. The consensus insertion extension confirms selected blocks against retained canonical history, but does not change synchronization's callback contract. Extending confirmation to synchronization also requires defining how skipped candidates, delayed callbacks, and unavailable canonical history affect runtime advancement. That separate coordination change remains deferred.
<!-- FINISH ALPHA CHANGE 2628 -- retain the separate synchronization callback policy -->

The accepted tradeoff is a conditional defensive identity gap: if two different blocks at the same height both pass finalized-block verification, consensus could insert block A while synchronization skips block B and subsequently invokes B's callback. If that callback advances the runtime before A's callback, runtime metadata could follow B while canonical storage contains A.

A conflicting block supplied by a single malicious proposer is insufficient unless it also passes finalized-block verification, including the applicable committed-signature and quorum checks. Concurrent processing of the same legitimate block does not create this mismatch. No reachable conflicting-finalization scenario within the tolerated Byzantine fault threshold was established by the source review.

The existing synchronization behavior is therefore retained as a deliberate compatibility choice. This accepts the conditional identity-check gap; it does not assume conflicting finalized blocks are harmless. The proposal-ownership correction remains in place, while the regression tests described above do not cover the conflicting-block sync-callback ordering.

<!-- FINISH ALPHA CHANGE 2604 -- explain the accepted synchronization compatibility tradeoff -->

<!-- FINISH ALPHA CHANGE 2603 -- document proposal execution ownership and confirmed consensus insertion -->

<!-- START ALPHA CHANGE 2636 -- document synchronized live proposer authorization -->

### 60. Synchronized Live Proposer Authorization

Live proposer checks now serialize access to the FSM's mutable proposer cache.

Previously, `consensusRuntime.IsProposer` held only the runtime's shared read lock while calling `ProposerSnapshot.CalcProposer`. Current-round and future-round workers could therefore calculate different rounds concurrently and interleave the separate cache assignments for `Proposer` and `Round`. A subsequent cache hit could associate a round with another round's proposer, causing inconsistent proposer authorization.

A dedicated runtime mutex now protects the complete operation: checking the cache, copying the snapshot for calculation, calculating the proposer, publishing the cached proposer and round, and comparing the supplied validator identity.

The proposer mutex is acquired before the runtime read lock. Consequently, queued proposer checks hold no runtime lock while waiting. Once a check acquires the proposer mutex, it reads the current FSM under the runtime read lock and keeps that lock throughout calculation. FSM publication remains protected by the runtime's exclusive lock. The runtime read lock is released before the proposer mutex.

This ordering protects the selected snapshot against replacement during an active check while allowing runtime publication to proceed before queued checks begin. Missing FSMs or proposer snapshots return false instead of causing a nil-pointer panic. Existing wrong-height and calculation-error rejection remains active.

The proposer priority algorithm, validator weighting, tie-breaking, and stored priority values remain unchanged. Cache hits remain supported. The correction introduces no round ceiling or new block-validity restriction, and preserves existing round catch-up behavior, including advancement from round 1 to round 6.

The performance tradeoff is serialization:

- A cached-round check adds mutex synchronization around the existing cache read.
- An uncached check performs the existing calculation while other proposer checks wait.
- A waiting check can reuse a result if the requested round remains cached when it proceeds.

No additional snapshot copies, priority iterations, storage reads, registry calls, historical lookups, or EVM execution are introduced by the production correction. An active uncached calculation still holds the runtime read lock until it finishes. Very large rounds retain their existing computational cost and can cause substantial waiting; the separate proposer-replay limitation documented in section 57 remains unchanged.

Regression coverage compares concurrent authorization results with independent serial calculations, checks consistency of the final cached round and proposer, verifies unchanged validator priorities, exercises lock ordering and FSM replacement, and covers missing snapshots, empty validator sets, and wrong heights.

<!-- FINISH ALPHA CHANGE 2636 -- document synchronized live proposer authorization -->

<!-- START ALPHA CHANGE 2653 -- document the mandatory one-wei genesis stake floor -->

### 61. Fixed Genesis Stake Floor And Governance Compatibility

Genesis configuration must specify a validator-registry minimum stake of exactly 1 wei. The genesis command defaults to `--validator-registry-minimum-stake 1` and rejects other numeric values.

Shared validation also protects configuration decoding, node startup, direct consensus construction, registry-manager construction, and the Go contract-initialization path. Startup checks the configured minimum before storage initialization, so reusing an existing database does not bypass this requirement. Missing or invalid values produce an error instead of being silently replaced.

The restriction applies to the genesis configuration. Governance remains able to change the registry contract's live minimum to any positive value permitted by the existing contract.

Consensus retains its additional comparison against the genesis minimum. Fixing that value at 1 wei prevents it from becoming a higher, stale eligibility threshold after governance lowers the live contract minimum. Every contract-eligible validator has at least 1 wei staked and therefore satisfies this fixed floor.

The existing bootstrap committee and registry-activation rules remain unchanged. Solidity contracts, contract artifacts, voting power, and transaction execution rules are unchanged.

The correction adds configuration-time integer checks. It introduces no additional recurring registry calls, EVM execution, or historical reads.

Regression tests cover missing, zero, negative, larger, and valid 1-wei values; command validation; configuration decoding; direct construction; rejection before contract initialization; and rejection before server startup side effects.

<!-- FINISH ALPHA CHANGE 2653 -- document the mandatory one-wei genesis stake floor -->

<!-- START ALPHA CHANGE 2669 -- document execution commitment validation before trie publication -->

### 62. Execution Commitment Validation Before Trie Publication

Previously, live proposal execution and finalized-block verification committed trie changes before comparing the calculated execution result with the supplied header. A proposal with executable transactions but incorrect commitments could therefore persist account, storage, and bytecode records before being rejected. Its canonical balances and nonces remained unchanged, so speculative fee deductions did not charge the canonical account.

Execution now follows this order:

1. Execute transactions once.
2. Calculate the resulting state root and stage the trie records.
3. Validate the existing execution commitments.
4. Publish the staged storage batch only if validation succeeds.
5. Publish derived trie snapshots and cache imported-block receipts only after the storage write succeeds.

Live proposal validation performs all existing reconstructed-header comparisons inside the publication guard, including state root, gas usage, receipt root, bloom, transaction root, uncles root, and reconstructed block hash.

Imported execution validates receipt count, state root, gas usage, receipt root, and bloom before publication. Its receipt-extraction fallback uses the same guarded execution path. The caller no longer repeats the execution-result comparisons after publication.

MDBX continues using its existing staged batch and database write. Memory-storage batches now also stage copied records until `Write`, which publishes them under the storage's shared lock. Rejection therefore prevents publication on both backends.

Commitment mismatches remain ordinary validation errors. They are retained separately from local-state failures, preventing the same rejected transition from publishing through a later ordinary or guarded commit. Actual trie and storage failures retain their existing local-failure classification.

The execution and root-calculation algorithms remain unchanged. The correction introduces no additional registry calls, historical searches, EVM execution passes, trie-root calculations, or MDBX durability operations. Accepted execution uses the existing batch write; incorrect commitments avoid that write.

**Accepted scope and performance choice**

Correctly validated proposals that later lose a round or are cancelled can still persist speculative state. Publication is deliberately guarded by execution validity rather than deferred until consensus finality. This keeps the correction narrow and avoids retaining unpublished batches across round changes, prepared-block reuse, and final insertion, which would require a broader ownership and memory-lifecycle change.

Historical trie retention and the manual reclamation procedure described in section 42 remain applicable. This correction prevents publication caused by incorrect execution commitments; it does not reclaim records already stored.

Regression coverage examines storage records, batch-write activity, and derived-cache entries on memory and MDBX backends. It also covers accepted results, retained rejection, injected publication failures, proposal hash mismatch, imported receipt-count mismatch, receipt-cache ordering, and reuse of cached receipts without another execution.

<!-- FINISH ALPHA CHANGE 2669 -- document execution commitment validation before trie publication -->

<!-- START ALPHA CHANGE 2732 -- document cancellable P2P RPC lifetimes and connection ownership -->

### 63. Cancellable P2P RPC Lifetimes And Connection Ownership

Previously, protocol establishment could wait on an unresponsive peer while holding the global protocol registry lock. Status deadlines and download cancellation did not cover the complete establishment path. Identity handshakes lacked an application deadline, and some client connections and P2P gRPC services lacked explicit cleanup ownership. These conditions could block synchronization, delay shutdown, or retain connections and goroutines.

P2P RPC operations now register with `RPCLifetime` before dialing or launching admitted workers. Admission is serialized against closure. Shutdown prevents new admission, cancels existing operations, and waits for their completion.

Protocol lookup releases the registry lock before network establishment. Cancellation covers libp2p stream establishment and gRPC transport readiness, including protocol negotiation deferred until first I/O. Failed or cancelled establishment resets the stream. Successful establishment clears its transport deadline before handing the connection to its operation owner.

Identity, discovery, status requests, and block downloads own separate client connections. Normal completion and cancellation share idempotent cleanup, which waits for an already-running cancellation cleanup. A status request therefore cannot close an active download's connection.

Download cancellation is registered before establishment can block. Replacing a download cancels the previous operation, while delayed cleanup removes its registration only if that registration still belongs to the same operation. Readers and forwarding workers participate in joined shutdown, and completion is published after ownership cleanup.

<!-- START ALPHA CHANGE 2771 -- clarify connection-specific handshake ownership -->
Identity shutdown joins the complete asynchronous handshake worker, including connection cleanup, pending-connection accounting, and final event delivery. Pending handshakes use atomic admission keyed by connection ID to prevent duplicate notifications from launching another handshake for the same transport connection. Separate connections to the same peer retain independent handshake and cleanup ownership. Discovery shutdown cancels and joins admitted discovery work.
<!-- FINISH ALPHA CHANGE 2771 -- clarify connection-specific handshake ownership -->

Saved protocol connections are detached under the peer lock and closed afterward. Replacement closes the previous connection; attempts to save a connection for a missing peer or a closing server release the supplied connection. Peer removal also releases its saved clients.

P2P gRPC listener closure and service shutdown have separate responsibilities:

- `Close()` cancels acceptance and releases pending stream handoffs.
- `Stop()` separately stops gRPC, cancels active RPCs, and joins serving work and handlers.
- Streams arriving during closure are reset.
- Repeated shutdown is idempotent.

This separation avoids recursively stopping gRPC from its listener's `Close()` method. RPC services and admitted operations are shut down before their storage dependencies are closed.

**Timeouts and recovery**

| Operation | Timeout behavior |
| --- | --- |
| Protocol establishment and gRPC transport readiness | Up to 100 seconds, subject to earlier caller cancellation or deadlines. |
| Identity handshake | 100 seconds covering establishment and the handshake RPC. |
| Discovery request | 100 seconds covering establishment and the discovery RPC. |
| Status request | 10 seconds covering establishment and the status RPC. |
| Block download | A 100-second setup timer is removed after establishment; the download retains shutdown cancellation and the existing per-block timeout. |

<!-- START ALPHA CHANGE 2737 -- explain finite RPC deadlines and continuing download progress -->

Finite establishment and request deadlines bound waits on unresponsive peers, preventing stalled negotiation or RPCs from retaining resources indefinitely and obstructing synchronization or shutdown. The 100-second limit is an operational timeout policy, not a block-validity rule.

Establishment, identity, discovery, and status deadlines measure elapsed time rather than inactivity. Partial traffic does not extend these deadlines. An attempt that exceeds its deadline is cancelled even if it has exchanged some data.

An established download does not inherit a total 100-second lifetime. Successful setup stops the download setup timer and clears the transport deadline. The download retains shutdown cancellation and its separately configured per-block timeout. That timeout is renewed after each decoded block is delivered to the local consumer and paused while the forwarder waits to deliver that decoded block. Partial transport traffic does not renew it. A progressing download can therefore continue beyond 100 seconds without a total session deadline.

Identity-handshake failure follows the existing peer-disconnection path. Status, discovery, and download cleanup closes the connection owned by that operation; it does not intentionally disconnect the whole peer or close another operation's connection. Subsequent attempts remain governed by the existing dialing and synchronization recovery behavior.

Timeouts and transport failures remain peer-operation errors. They do not introduce new block-validity restrictions or fatal consensus-error classification.

<!-- FINISH ALPHA CHANGE 2737 -- explain finite RPC deadlines and continuing download progress -->

The correction adds contexts, timers, admission synchronization, and operation-owned RPC connections. Separate gRPC transports and libp2p streams add networking setup work while removing the previous global lock across network waits. No additional registry calls, historical searches, EVM executions, or consensus calculations are introduced.

**Regression coverage**

The 16 lifecycle regressions cover the following boundaries:

| Test | Coverage |
| --- | --- |
| `TestRPCLifetime_CloseCancelsAndJoins` | Admission, cancellation, joining, and repeated completion or closure. |
| `TestRPCTransport_CancelsBlockedEstablishment` | Cancellation when the remote endpoint never completes transport readiness. |
| `TestRPCTransport_ClearsEstablishmentDeadline` | Deadline removal and continued RPC use after successful setup. |
| `TestRPCListener_CloseUnblocksAcceptAndRejectsLateStreams` | Acceptance cancellation and release of late streams. |
| `TestRPCService_StopJoinsHandlers` | RPC cancellation and joining unfinished handlers. |
| `TestRPCProtocol_DialDoesNotHoldRegistryLock` | Protocol registration during blocked dialing and cancellation during network shutdown. |
| `TestSyncRPCLifetime_CloseCancelsStatusEstablishment` | Cancellation of blocked status establishment. |
| `TestSyncRPCLifetime_CloseStreamPreservesStatusOperation` | Independent status and download cancellation. |
| `TestSyncRPCLifetime_LateCleanupPreservesReplacement` | Preservation of a newer download during delayed cleanup. |
| `TestRPCOwnership_CancelAtReadinessHandoff` | Ownership cleanup when cancellation occurs during the final establishment step. |
| `TestRPCOwnership_CleanupJoinsCancellationClose` | Cleanup joining an already-running transport close. |
| `TestRPCListener_CloseReleasesPendingHandoff` | Closure while a stream handler is waiting for acceptance. |
| `TestIdentityRPCLifetime_CloseJoinsCleanupAndFinalEvent` | Complete handshake-worker joining, duplicate notification handling, and pending accounting. |
| `TestDiscoveryRPCLifetime_CloseJoinsEstablishedRPCCleanup` | Established-RPC cancellation, joined cleanup, and closed admission. |
| `TestSyncRPCOwnership_StatusCleanupPreservesEstablishedDownload` | Continued block delivery after an independent status connection closes. |
| `TestSyncRPCOwnership_CloseCancelsActiveReceiveAndJoinsReaders` | Streaming-RPC cancellation, worker joining, closed outputs, cleared ownership, and rejection of new downloads. |

<!-- START ALPHA CHANGE 2738 -- document additional RPC timing and loopback regression coverage -->

Seven additional timing and loopback regressions complement the 16 lifecycle regressions:

| Test | Coverage |
| --- | --- |
| `TestRPCTransport_DelayedReadinessSucceedsBeforeDeadline` | Establishment remains pending during a controlled remote handoff delay, then succeeds and supports an RPC before its deadline. |
| `TestRPCTransport_RemainsUsableAfterSetupDeadlineExpires` | A fresh RPC succeeds after the original setup context's deadline has actually elapsed, with the transport deadline cleared. |
| `TestRPCProtocol_LoopbackDelayedReadinessSucceeds` | The real protocol-connection path tolerates delayed gRPC readiness over two loopback libp2p hosts; closing the RPC stream preserves the peer connection. |
| `TestRPCProtocol_LoopbackTimeoutThenFreshAttemptSucceeds` | A timed-out establishment attempt is followed by successful establishment on a distinct stream, normal cleanup, and server shutdown. |
| `TestSyncRPCTiming_DownloadOutlivesTransportSetupDeadline` | The actual sync client receives another block after a short transport-setup deadline expires, without the download inheriting that deadline. |
| `TestSyncRPCTiming_PerBlockProgressExtendsDownload` | Successive complete blocks renew the per-block timeout, allowing the session to last longer than one timeout interval. |
| `TestSyncRPCTiming_DownloadSurvivesProductionSetupTimeout` | An established download continues receiving blocks beyond the unchanged production 100-second setup interval, then completes with connection cleanup, cleared registration, and joined client shutdown. |

The long production-timeout regression is explicitly enabled with `INNBC_TEST_LONG_RPC=1`; otherwise it reports a skip. It measures the download interval from successful establishment and requires block receipt at least five seconds beyond the production setup limit.

The regressions use controlled mocks, stream pairs, real gRPC over in-memory transport, and real loopback libp2p hosts. Timing tests coordinate operation entry and handoff through channels, release controlled gates before cleanup, and explicitly join test-created workers. Sync network fixtures register server cleanup to release their resources after each test.

The loopback handoff delays occur after libp2p protocol selection and exercise gRPC readiness. The retry regression exercises an explicitly initiated fresh attempt, rather than an automatic retry scheduler. The download fixtures exercise transport and sync-client lifecycle behavior without consensus execution or finalized-block validation.

Existing network and sync suites cover normal identity, discovery, status, and block-transfer behavior.

<!-- FINISH ALPHA CHANGE 2738 -- document additional RPC timing and loopback regression coverage -->

**Scope**

This correction governs P2P RPC establishment, operation-owned connections, and service shutdown. It does not make synchronous local storage calls interruptible or provide a global join for every networking goroutine. Download handlers check cancellation before each subsequent storage read; an already-running local storage call must return before its handler can finish and shutdown can complete.

<!-- FINISH ALPHA CHANGE 2732 -- document cancellable P2P RPC lifetimes and connection ownership -->

### 64. Consistent Peer Admission, Reconnect Ownership And Sync Status Freshness

INNBC Chain enforces its configured peer-admission policy from host construction through RPC and gossip delivery. Additional ownership checks protect outbound dial capacity, reconnect requests, and asynchronous synchronization status updates.

These changes govern networking authorization and local operation ownership. They do not change consensus membership, transaction execution, block validity, or finality rules.

#### Networking admission and validator authority

Consensus validator authority remains governed by the genesis bootstrap configuration and the on-chain validator registry.

For directory-restricted networking, each node uses its cached directory-derived peer-ID allowlist to decide which authenticated identities may connect and use P2P services. The libp2p security handshake proves possession of the private key corresponding to the peer ID. Knowing another validator's public key, IP address, or advertised multiaddress does not provide that proof.

The directory therefore supplies both discovery addresses and the membership information used by this cached networking policy. It does not independently grant consensus voting authority.

The cache can become stale. Removing a validator from the contract does not immediately remove that identity from every node's cached networking allowlist. Operators must refresh the remaining nodes to propagate the updated membership. Until then, the removed identity may retain networking access under an old list; that access does not itself authorize consensus votes.

Admission checks use authenticated peer identities rather than IP addresses. Multiple validators sharing a public gateway remain distinct identities.

#### Startup, refresh and shutdown

The admission state exists before the listening host is constructed.

For directory-restricted deployments:

- Connections are rejected before the required startup notifications are installed.
- Remote admission remains closed until an initial validated directory response is applied.
- Once initialized, admission permits identities in the cached allowlist.
- A failed subsequent refresh retains the previous policy.
- Shutdown closes admission and prevents a later directory response from reopening it.

Directory exchange remains asynchronous, allowing other node services to finish starting while P2P admission awaits initialization. An unsuccessful initial exchange remains recoverable through the existing manual-refresh workflow.

Private and external validator profiles use the same membership enforcement. Intentionally unrestricted configurations retain their unrestricted membership behavior.

A successful refresh replaces the allowlist atomically. Revocation considers actual transport connections as well as application peer records, and closes captured revoked connections outside the policy and peer locks. Unchanged membership and address-only updates preserve eligible existing connections.

Connection notification and final identity registration recheck admission. This closes the startup window and coordinates registration with concurrent revocation.

<!-- START ALPHA CHANGE 2851 -- document addressless peer record rejection before publication -->
#### Directory and discovery address validation

A peer identity alone, such as `/p2p/<peer-id>`, contains no transport address. Although the libp2p parser accepts that representation, directory registration and discovery require an address that can be supplied to the peerstore.

Directory exchange requests reject addressless records before registry authorization, nonce consumption, or address publication. Nodes validate every record in a directory response before replacing their allowlist or changing peerstore addresses. If any record is malformed or addressless, the response is rejected and the previous policy and peerstore addresses remain intact.

Persisted directory caches receive the same addressless-record check. An invalid cache follows the existing corrupted-cache recovery policy: discard it and regenerate an empty cache for subsequent registrations.

Discovery discards addressless records before peerstore or routing-table insertion. `AddToPeerStore` also defensively rejects nil peer information, empty address lists, and a nil first address before dereferencing them.

These checks do not impose an IP-only restriction or change the general libp2p parser. IPv4, IPv6, DNS, and other address forms remain subject to the existing parsing and networking policy.
<!-- FINISH ALPHA CHANGE 2851 -- document addressless peer record rejection before publication -->

#### RPC and gossip enforcement

Connection gating applies during establishment. Additional checks enforce the current policy when dispatching protocol streams and beginning unary or streaming P2P RPCs, including RPCs carried over an existing transport.

Allowed identities can complete identity handshakes before appearing in the application peer table. Temporary discovery connections retain that ability.

Gossip checks both the immediate forwarding peer and the signed message author, and rechecks authorization before queued application delivery. Ordinary gossip does not require an allowed author to be directly connected when another allowed peer forwards its message.

These checks read cached local policy. They do not query the registry or directory for each connection, RPC, or gossip message.

#### Outbound dial ownership and retained reconnect requests

Outbound attempts own their dial-slot accounting. Early identity rejection and connection cleanup release the attempt's capacity through its ownership path; unrelated completion or failure events cannot release another attempt's slot.

When a new reconnect request arrives while an earlier attempt for the same peer is still active or cleaning up, the queue retains that request. Repeated requests coalesce using the latest submitted address information and the strongest requested priority.

Completion checks the exact task owner, preventing delayed cleanup from completing a replacement task. The previous attempt releases its slot before retained work becomes runnable. Queue deletion or shutdown discards pending work.

The retained request still passes the existing authorization, capacity, connectivity, and shutdown checks. Retention does not guarantee that a connection will be attempted or accepted, and it does not create automatic retries when no new reconnect request was submitted.

#### Session ownership for synchronization status

Synchronization status updates carry an opaque local token identifying their originating eligible peer session. The token is local metadata and is not transmitted over the network.

An eligible session is preserved while at least one tracked, eligible connection remains live. Additional connections, unchanged membership refreshes, and address-only changes preserve its token.

Revocation, loss of the last eligible connection, or shutdown invalidates the session. A subsequent eligible connection receives a distinct token. Cleanup belonging to an old connection cannot invalidate its replacement session.

Status ownership is captured at the source:

- Direct status queries capture the token before establishing or executing the RPC.
- Startup status collection retains that token with each query result.
- Gossip captures ownership during local pubsub validation and carries it through the subscription queue, application queue, callback scheduling, and sync-status channel.
- Queued delivery does not replace the original token with the currently active session's token.

Final publication verifies the token and updates the sync peer map under the same admission lock. Publication and peer-map removal are also serialized. Connection-event freshness checks run before the atomic publication helper, and notifications occur after releasing the locks.

Missing ownership, invalidated sessions, and publication after shutdown are rejected.

Discarding an old status does not disconnect or ban a legitimate replacement connection. For example, a query started in session A may finish after reconnection creates session B. Its result is discarded, while statuses captured in B remain eligible.

Session capture is specific to synchronization-status consumers. Missing status ownership does not prevent ordinary gossip forwarding by an allowed peer.

<!-- START ALPHA CHANGE 2852 -- document bounded status delivery and atomic replacement protection -->
#### Bounded synchronization-status delivery

Gossip status handling runs directly inside the existing topic callbacks instead of launching another worker for each message. The captured session token continues through delivery without being replaced.

The status subscription therefore retains the existing bounded pipeline: 2–8 topic workers, a 128-entry application queue, a 1024-entry pubsub subscription buffer, and the bounded sync-status channel. A slow consumer applies backpressure without creating an unbounded collection of downstream status goroutines.

Under sustained overload, the existing pubsub subscription policy may drop messages when its buffer is full. This deliberately bounds retained work rather than accumulating unlimited workers.

Shutdown signals delivery cancellation before joining the topic callbacks. Blocked status sends can return, and the status output channel closes only after those callbacks have finished.

#### Atomic disconnect cleanup

Disconnect processing checks the event's current ownership before entering an atomic session-absence helper. The helper holds the admission read lock through the eligible-session check and peer-map deletion; its callback performs only the map removal.

If an eligible session exists, cleanup retains the stored status. This includes a replacement connection whose identity handshake has not yet completed application registration, preventing an old disconnect event from deleting status already published by that replacement.

While an eligible session remains, subsequent accepted updates can replace the stored status. When no eligible session remains, disconnect cleanup can remove it. Registration and policy changes cannot occur between the helper's absence check and deletion.

This protection does not require completed application registration, disconnect an eligible replacement, or add another session-ownership map. It uses the existing local admission state and adds no registry calls, historical searches, EVM execution, or timeout changes.
<!-- FINISH ALPHA CHANGE 2852 -- document bounded status delivery and atomic replacement protection -->

#### Deliberately retained freshness boundaries

The protection begins at query capture or local gossip validation. It does not track gossip ownership from raw network receipt and does not impose ordering within one uninterrupted session.

| Situation | Behavior |
| --- | --- |
| Status captured in session A and delivered after replacement by B | Rejected at publication |
| Additional eligible connection within the same uninterrupted session | Existing ownership remains valid |
| Unchanged membership or address-only refresh | Existing ownership remains valid |
| Old connection cleanup after replacement | Replacement session remains intact |
| Gossip waiting before local validation, then validated during B | Can receive B's token |
| Two statuses from the same uninterrupted session arrive out of order | Both remain eligible |
| Missing ownership or publication after shutdown | Rejected |

Consequently, a delayed height-100 status can still follow height 105 within one uninterrupted session. This correction protects session ownership; it does not introduce status sequencing or monotonic-height enforcement.

#### Regression coverage and operational scope

Regression tests cover:

- Startup admission, RPC authorization, gossip forwarding, revocation, and connection-specific cleanup.
- Early identity rejection with actual outbound slot ownership and rejection of unrelated slot-release events.
- Reconnect requests arriving during cleanup, latest-address retention, stale completion, queue deletion, and shutdown.
- Session preservation across additional connections and unchanged refreshes.
- Session replacement after disconnection or revocation, including delayed old-connection cleanup.
- Publication coordinated with invalidation and peer-map removal.
- Delayed startup RPC results, queued statuses, missing ownership, and shutdown.
- Validation metadata retained through controlled subscription and application queue delivery.

<!-- START ALPHA CHANGE 2853 -- document address validation and status lifecycle regression coverage -->
Additional regressions cover:

- Rejection of a directory response containing an addressless record after an earlier valid record, preserving the previous allowlist and peerstore without queuing new dial work.
- Directory request rejection without registry lookup, nonce consumption, or address publication; continued acceptance of IPv4, IPv6, and DNS transport addresses.
- Addressless persisted-cache rejection through the existing regeneration policy, and successful loading of a valid DNS address.
- Defensive peerstore handling of nil or empty addresses, and discovery rejection before peerstore and routing-table insertion.
- Eligible replacements before application registration, revocation followed by reauthorization, delayed old-connection cleanup, and preservation while another eligible connection remains.
- Registration ordered against atomic deletion, replacement-status preservation, and removal after the eligible session ends.
- Direct delivery through a real pubsub subscription while separate worker admission is locked, followed by shutdown with an unread status channel and completed callback cleanup.

These cases are implemented in `network/address_validation_test.go`, `network/peerdirectory/address_validation_test.go`, `network/discovery/address_validation_test.go`, `network/peer_session_removal_test.go`, `syncer/status_removal_test.go`, and `syncer/status_delivery_test.go`.
<!-- FINISH ALPHA CHANGE 2853 -- document address validation and status lifecycle regression coverage -->

The subscription-queue regression uses a controlled subscription fixture. It does not reproduce that exact reconnection ordering inside a real pubsub subscription. The implementation also relies on the pinned pubsub behavior that retains local ValidatorData through validated message delivery.

Existing authorization, capacity, connectivity, and timeout policies remain applicable. A legitimate peer arriving before admission readiness must reconnect afterward; an identity absent from the cached allowlist remains rejected until a successful refresh admits it.

The changes add local map checks, connection inspection, and synchronization. They add no registry calls, historical searches, or EVM execution, and introduce no new timeout policy. No performance percentage or universal networking-termination guarantee is claimed.

<!-- START ALPHA CHANGE 2891 -- document discovery reconnect preservation atomic handshake capacity session cleanup and GossipSub cancellation -->

### 65. Discovery Reconnect Preservation, Atomic Handshake Capacity And Owned Gossip Shutdown

Additional networking lifecycle corrections preserve submitted reconnect work, coordinate handshake capacity, remove synchronization status after unregistered sessions end, and supply GossipSub with an owned shutdown signal.

#### Discovery eviction preserves reconnect requests

Discovery routing-table removal no longer deletes dial-queue requests by peer identity. Routing-table eviction does not own independently submitted reconnect work.

A delayed disconnect or failed-connect event can therefore remove an old route without discarding a newer pending or runnable request for the same peer. Production discovery uses the same routing-table callbacks exercised by the regressions.

Retained requests still pass the existing authorization, capacity, connectivity, and shutdown checks. This correction preserves submitted work; it does not introduce an automatic retry when no request was submitted. Explicit dial-queue deletion and closure retain their cancellation behavior.

#### Atomic handshake capacity reservation

Identity handshakes reserve pending capacity before starting their asynchronous work. Capacity inspection and reservation occur under the admission and peer locks, preventing concurrent handshakes from both reserving the same last available slot.

Successful registration transfers the owned pending reservation into active accounting under those same locks. Other admissions cannot observe a temporarily released slot between reservation and registration.

Failure, temporary-handshake completion, cancellation, disconnect, and shutdown release capacity through the owning cleanup path. Each returned reservation cleanup has its own `sync.Once`, so repeated cleanup from an earlier reservation cannot release a later reservation on the same connection owner.

Active accounting continues to count each registered peer once per direction, rather than counting every transport connection as another active peer. An additional connection still requires temporary handshake capacity under the existing policy.

When capacity is genuinely exhausted, another handshake can be rejected consistently. The correction enforces the configured limits without adding a new membership rule or an arbitrary rejection after a successfully reserved handshake.

#### Session cleanup before application registration

An eligible connection can participate in synchronization-status ownership before its identity handshake completes application registration. Its failure or disconnection must therefore trigger status cleanup even if it never became an application peer.

Owned connection removal determines whether eligible session availability has ended while holding the admission lock. This check also handles session metadata already retired by revocation or a prior lookup.

Cleanup emits `PeerSessionEnded` after releasing network locks. The event is independent of application registration, and repeated removal of an already-removed connection does not emit another notification. Existing event values and application `PeerDisconnected` behavior remain intact.

The sync client forwards the notification to the syncer. It requests a current eligibility check rather than unconditional deletion: `WithNoPeerSession` holds the admission read lock through the session-absence check and sync peer-map removal.

A delayed notification therefore preserves an eligible replacement, including one whose handshake has not yet completed registration. When that replacement also ends without registration, its own cleanup requests removal of the unavailable peer's stored status.

#### Server-owned GossipSub cancellation

Each network server creates a dedicated cancellable context for GossipSub and retains its cancellation function after successful construction.

`Server.Close()` closes peer admission, then cancels GossipSub before joining networking work and closing the libp2p host. Cancellation runs inside the existing idempotent close operation and outside admission, peer, and protocol locks.

Constructor failures release resources already acquired:

- Event-emitter construction failure closes the listening host.
- GossipSub construction failure cancels its context and closes the event emitter and host.
- Cleanup preserves the construction error and includes any cleanup errors.

Explicit application topic closure remains responsible for cancelling subscription readers and joining topic workers and admitted callbacks. GossipSub cancellation does not replace that cleanup.

The cancellation signal allows the pinned pubsub processing and heartbeat loops to exit. It does not synchronously join every internal pubsub goroutine. The heartbeat's initial sleep is not interruptible, and an already-running validator or callback must cooperate with cancellation or return.

#### Regression coverage

| Correction | Coverage |
| --- | --- |
| Discovery reconnect preservation | Production routing-table callbacks preserve pending and runnable requests during disconnect and failed-connect eviction, including a same-peer request submitted while the previous worker is completing cleanup. |
| Handshake capacity | Concurrent last-slot reservations in both directions, atomic promotion, multiple-connection accounting, stale ownership, repeated cleanup after re-reservation on the same owner, and controlled identity completion paths. |
| Session cleanup | Unregistered connections, already-retired session metadata, duplicate removal, multiple eligible connections, and the network event subscription → sync-client forwarding → syncer removal pipeline. |
| Replacement and shutdown ordering | Failed and temporary handshakes, revocation and reauthorization, delayed cleanup preserving an unregistered replacement, that replacement subsequently failing, and cancellation of delayed event delivery. |
| GossipSub lifetime | Local delivery before shutdown, cancellation of the exact construction context, repeated and concurrent closure, topic cleanup before and after cancellation, cooperative in-progress validation, cancellation before worker joining, constructor failures, and repeated construction without startup. |

These regressions are implemented in:

- `network/discovery_dial_ownership_test.go`
- `network/dial_ownership_test.go`
- `network/connection_slots_test.go`
- `network/peer_session_ended_test.go`
- `syncer/status_session_ended_test.go`
- `network/gossip_lifetime_test.go`

The capacity regressions exercise the real reservation method and separately exercise its identity-handshake integration. The GossipSub delivery regression exercises local publication through real validation, subscription, and application delivery.

#### Retained scope and performance boundaries

These corrections add local ownership checks, synchronization, and one cancellable GossipSub context per server. They add no registry calls, historical searches, repeated EVM execution, or new timeout policy, and do not change block-validity or finality rules.

The synchronization-status freshness boundaries described in point 64 remain deliberately unchanged:

- Gossip waiting before local validation can receive the session current when validation runs.
- Statuses within the same uninterrupted session can arrive out of order.

The design continues to protect ownership after query capture or local validation without introducing raw-ingress correlation or status-sequencing machinery. Existing admission readiness, cached authorization, capacity, and operation timeout policies remain applicable.

<!-- FINISH ALPHA CHANGE 2891 -- document discovery reconnect preservation atomic handshake capacity session cleanup and GossipSub cancellation -->

<!-- START ALPHA CHANGE 2899 -- document interim GossipSub capacity and retained recursive evidence limitation -->

### 66. Explicit GossipSub Capacity Upgrade For Larger Consensus Proposals

Every network server explicitly configures a maximum encoded GossipSub RPC size of **4 MiB (4,194,304 bytes)** instead of inheriting the dependency's 1 MiB default.

The limit applies across GossipSub topics. For consensus proposals, the encoded RPC includes the block, round-change and prepared evidence, consensus signatures, and the signed gossip envelope. It is not a raw-block size limit.

<!-- START ALPHA CHANGE 2900 -- distinguish measured transport correction from accepted recursive evidence growth -->

The capacity increase is supported by encoded-message measurements and observed remote delivery. The baseline transport fixtures demonstrate that the inherited 1 MiB setting drops specific fresh proposals and prepared reproposals. The corresponding 4 MiB fixtures demonstrate delivery of those messages, while production-constructor fixtures independently verify the setting applied by `network/server.go`.

The adopted resolution is therefore: **demonstrated 1 MiB transport failures fixed; recursive-evidence growth retained as an accepted limitation**. The evidence supports the measured cases; it does not establish that 4 MiB accommodates every permitted combination of block size, validator count, and successive preparations.

The existing message representation, signatures, certificate validation, proposer selection, and prepared-block reuse rules are preserved. Retaining that representation keeps this correction narrow and avoids introducing changes to signed evidence or restrictions on legitimate round progression. The remaining recursive-growth limitation and its consequences are described below.

<!-- FINISH ALPHA CHANGE 2900 -- distinguish measured transport correction from accepted recursive evidence growth -->

#### Separate synchronization limit

Block synchronization retains gRPC's existing **4 MiB per-response receive limit**.

A synchronization response contains a finalized block and protobuf encoding overhead. It does not contain the complete nested round-change evidence carried by a consensus proposal. The receive limit applies to each response, not the total download.

The numerical equality of these limits does not establish that every permitted block or consensus certificate fits both paths.

#### Deliberately retained recursive-evidence limitation

The 4 MiB GossipSub setting is an interim capacity improvement.

A prepared certificate retains its signed PREPREPARE message, which can contain earlier round-change evidence. Later prepared reproposals can therefore repeat blocks and retain prior certificate structures.

Round progression without another preparation does not automatically add another retained preparation level. Successive reproposals that become prepared can increase that nesting and eventually exceed 4 MiB.

The transport increase does not resolve this recursive growth or establish a universally sufficient message size. The existing representation is retained to keep this interim correction narrow and avoid changing signed evidence or prepared-block reuse semantics. A complete correction requires a separate evidence-representation or transport design.

Messages exceeding the configured envelope remain undeliverable through this gossip path. Successful local publication alone does not establish remote delivery.

#### Resource and deployment implications

Larger permitted messages can consume more buffering, decoding, signing, forwarding, and bandwidth resources. The queue settings remain unchanged; the per-message transport cap does not establish an aggregate node-memory budget.

The change adds no registry calls, historical searches, additional EVM execution, or round ceiling.

<!-- START ALPHA CHANGE 2901 -- document sizing evidence and the source files establishing its scope -->

#### Implementation and measurement evidence

- `network/server.go` defines the 4 MiB encoded-RPC limit and supplies it through `pubsub.WithMaxMessageSize` during production GossipSub construction.
- `network/gossip_size_test.go`, through `TestGossipSize_ProductionConstructorLimit`, exercises that production constructor without overriding its size setting. It demonstrates delivery of the formerly oversized 1,081,720-byte payload and an encoded RPC of approximately 4,190,353 bytes. An encoded RPC of approximately 4,195,472 bytes exceeds the configured limit and produces an observed outbound drop; a smaller message is delivered afterward.
- `consensus/innbcbft/consensus_size_measurement_test.go` measures complete signed gossip envelopes using real consensus message builders and signatures. Its fixtures distinguish timeout-only progression from successive reproposals that become prepared. With four validators, the latter sequence contains 1, 7, 25, and 79 block copies; with seven validators, it contains 1, 11, 61, and 311. The fixtures also exercise rejection of an invalid prepared-evidence signature.
- `consensus/innbcbft/consensus_size_transport_test.go`, through `TestConsensusWireTransport_DefaultLimit`, preserves the 1 MiB baseline. It demonstrates remote delivery below that limit and observed outbound drops for a fresh proposal of approximately 1,054,864 encoded bytes and a prepared reproposal of approximately 1,407,901 encoded bytes.
- `consensus/innbcbft/consensus_size_interim_transport_test.go`, through `TestConsensusWireTransport_InterimLimit`, demonstrates remote delivery under the 4 MiB setting for signed fresh and prepared proposals of approximately 1,081,863, 1,407,901, 2,669,292, and 1,630,691 encoded bytes. The prepared fixtures include successive preparations with four and seven validators.
- `syncer/block_size_transport_test.go`, through `TestSyncBlockSize_DefaultReceiveBoundary`, independently exercises the retained gRPC limit. A 4,193,280-byte protobuf response is delivered; a 4,195,328-byte response is rejected with `ResourceExhausted`.

These observations establish that increasing the GossipSub limit resolves the demonstrated delivery failures. They also establish that the finite transport boundary remains enforced. Successful local publication alone is insufficient evidence: the transport fixtures observe actual remote delivery or an outbound drop followed by successful delivery of a smaller message.

The consensus fixtures use production message builders, signatures, proposer checks, and certificate validation. Their round simulations substitute execution results after constructing the block fixture. Production-constructor payload fixtures isolate transport behavior; synchronization fixtures isolate response-size handling. These scopes do not establish full production-genesis or finalized-chain validation.

The fixtures deliberately bound preparation depth and allocated bytes. Their measurements do not provide a universal upper bound for recursive evidence, and the 4 MiB setting is not claimed to support indefinitely growing prepared certificates.

<!-- FINISH ALPHA CHANGE 2901 -- document sizing evidence and the source files establishing its scope -->

<!-- FINISH ALPHA CHANGE 2899 -- document interim GossipSub capacity and retained recursive evidence limitation -->

<!-- START ALPHA CHANGE 2912 -- document authenticated block serving budgets ownership and recoverable overload -->

### 67. Bounded Concurrent Block Serving With Recoverable Overload

Peer admission limits connections and registered peers. It does not independently limit RPC calls multiplexed over an established gRPC connection.

The synchronization service therefore reserves capacity for each `GetBlocks` handler before accessing blockchain storage.

| Serving budget | Limit |
| --- | --- |
| Active block-serving handlers across the node's sync service | 8 |
| Active block-serving handlers per authenticated peer, across all its connections | 2 |

These are explicit operating policies, not measured hardware-capacity guarantees. The per-peer allowance permits a brief overlap between an older download and its replacement while preventing one identity from occupying the entire global budget.

#### Trusted identity and atomic admission

`network/grpc/grpc.go`, through `PeerIDFromContext`, obtains the caller's peer ID from the authenticated libp2p transport carried in the gRPC context.

Missing transport identity is rejected with `Unauthenticated`. Request metadata, IP addresses, and caller-supplied identity claims are not used as substitutes.

`syncer/block_serving.go` acquires the global and per-peer allowances together under the service's existing lifecycle lock. Admission checks service closure, cancellation, and both limits before incrementing accounting.

`syncer/service.go` acquires the reservation before `Header`, `HistoryFrom`, or block reads. When either budget is full, the handler immediately returns `ResourceExhausted`, without performing blockchain reads or entering a waiting queue.

#### Reservation ownership and shutdown

Each admitted handler owns its reservation until it returns. Deferred cleanup releases both allowances on completion, unavailable history, send failure, cancellation, and shutdown.

Cancellation does not release capacity while a synchronous storage operation is still running. After that read returns, the handler checks cancellation before serializing its block.

Each cleanup has its own `sync.Once`, preventing repeated or concurrent cleanup from releasing a later reservation for the same peer. Per-peer accounting entries are removed when their active count reaches zero.

Shutdown marks the service closed under the same lock used for admission. It releases that lock before stopping gRPC and joining handlers. Reads, serialization, sends, and handler joining do not hold the accounting lock.

#### Availability policy and accepted boundaries

A legitimate new download can receive a temporary busy response while capacity is occupied. This response does not reject a block or instruct the networking layer to disconnect the peer.

The existing sync loop handles incomplete downloads by temporarily excluding the serving peer, trying another eligible peer, and eventually resetting exclusions for another attempt. The production exhausted-peer retry interval remains five seconds. A streaming overload can arrive during `Recv`, so recovery does not depend on the initial `GetBlocks` call returning an error.

`GetStatus` remains outside the download budget. Exhausting serving allowances therefore does not itself reject status queries.

The limiter adds no total download deadline. An admitted, progressing download can continue beyond the existing 100-second establishment timeout.

This correction bounds admitted block-serving work. It does not guarantee fairness under sustained overload, bound all incoming RPC traffic, or forcibly terminate a blocked synchronous storage operation. A handler retains its allowance until it actually returns, preserving the concurrency bound during cancellation and shutdown.

The implementation adds short local accounting operations at admission and completion. It introduces no additional registry calls, historical searches, or EVM execution.

#### Regression coverage

`network/grpc/peer_identity_test.go` verifies rejection of missing transport identity and confirms that conflicting metadata or custom-context claims cannot replace the transport-derived peer ID.

`syncer/block_serving_test.go` covers:

- Atomic competition for the last global and per-peer allowances.
- Repeated cleanup preserving a later reservation.
- Multiple RPCs on one connection and multiple connections sharing one peer budget.
- Different identities sharing the global budget.
- Rejection before any blockchain reads.
- Missing-identity rejection.
- Cancellation during a blocked read retaining capacity until handler return.
- Shutdown closing admission and joining an outstanding read.
- Release after completion, missing history, send failure, and cancellation.
- A controlled blocked send retaining its allowance.
- Status queries while download capacity is exhausted.
- Recovery through another peer or a later retry after an actual streaming overload.

The identified transport fixtures also support the existing service tests and block-size boundary tests through `syncer/service_test.go` and `syncer/rpc_lifetime_test.go`. Existing download-lifetime regressions remain applicable.

The fixtures exercise real gRPC and the production stream wrapper with controlled libp2p connection identities. They do not claim to independently verify the cryptographic libp2p handshake or finalized-block consensus validity.

<!-- FINISH ALPHA CHANGE 2912 -- document authenticated block serving budgets ownership and recoverable overload -->

<!-- START ALPHA CHANGE 2922 -- document complete retirement of the legacy aggregate BLS precompile -->

### 68. Retirement Of The Legacy Aggregate BLS Precompile

<!-- START ALPHA CHANGE 2923 -- highlight the security benefit and scope of aggregate BLS retirement -->

The custom aggregate-signature verification precompile formerly registered at `0x2030` has been removed as a **security improvement that eliminates an unused resource-abuse attack surface**.

Its previous execution path charged a fixed 150,000 execution gas while decoding and cryptographically checking a caller-supplied number of BLS public keys. A caller could request substantial key-processing work without a corresponding increase in that execution charge, potentially slowing validators and disrupting block production. Transaction-size and intrinsic-gas limits constrained the input, but did not make the fixed execution charge proportional to the cryptographic work.

**Removing `0x2030` eliminates this particular attack surface.** No current system operation requires the feature. Retirement preserves the separate BLS checks used by consensus and validator registration, including proof of possession.

The identified defect concerns **resource consumption and availability**. A chain halt, theft, forged signatures, and acceptance of invalid blocks were not demonstrated. This correction removes the specific execution path; it does not claim to eliminate every possible resource-exhaustion attack.

<!-- FINISH ALPHA CHANGE 2923 -- highlight the security benefit and scope of aggregate BLS retirement -->

#### Removed feature

The retirement removes:

- `state/runtime/precompiled/bls_agg_sigs_verification.go` and its former functional tests.
- The aggregate-verification registration in `state/runtime/precompiled/precompiled.go`.
- `BLSAggSigsVerificationPrecompile` from `contracts/system_addresses.go`.
- `DomainStateReceiverString` and `DomainStateReceiver` from `consensus/innbcbft/signer/utils.go`.
- `VALIDATOR_PKCHECK_PRECOMPILE` and `VALIDATOR_PKCHECK_PRECOMPILE_GAS` from `core-contracts/contracts/child/System.sol`.

<!-- START ALPHA CHANGE 3760 -- align retirement documentation with the retained contract suite -->
The retained `System`, `INNBCNativeToken`, and `ValidatorSet` contracts no longer expose the two inherited public getters. Their artifacts, bytecode, TypeChain outputs, and generated documentation are produced through the existing tools. The Go artifact generator embeds these three contracts.

The unused Solidity `AccessList` wrapper and its `IAddressList` interface have been removed. This removal leaves the separate Go transaction and contract-deployer allowlist and blocklist functionality intact.
<!-- FINISH ALPHA CHANGE 3760 -- align retirement documentation with the retained contract suite -->

#### Address behavior after retirement

`0x2030` no longer performs aggregate-signature verification or returns its former ABI boolean result.

Unregistering the precompile does not impose an explicit revert at that address. Ordinary EVM account behavior applies: a zero-value call to an account without code can succeed with empty return data; if ordinary bytecode is present, that bytecode executes.

#### Preserved functionality

The native-transfer precompile at `0x2020`, its caller authorization, and the native-token transfer, issuance, and burn paths remain.

The Go BLS package, active signing domains, Solidity `BLS` and `IBLS`, validator proof-of-possession verification, consensus BLS verification, and standard BN256 precompiles at `0x06`, `0x07`, and `0x08` are retained. Shared ABI boolean initialization and the remaining `System` constants, modifiers, and storage gap are also retained.

#### Regression coverage and genesis ownership

Retirement-specific regressions are provided in:

- `state/runtime/precompiled/bls_retirement_test.go`: absence of the retired registration and preservation of native-transfer and BN256 registrations.
- `state/bls_retirement_test.go`: ordinary execution behavior at the retired address, with and without code.
- `consensus/innbcbft/contractsapi/bls_retirement_test.go`: removal of getters from embedded artifacts and preservation of active BLS interfaces.
<!-- START ALPHA CHANGE 3760 -- align retirement documentation with the retained contract suite -->

- `core-contracts/test/BLSPrecompileRetirement.audit.ts`: removal of getters from the `System`, `INNBCNativeToken`, and `ValidatorSet` Hardhat artifacts and TypeChain factories.

<!-- FINISH ALPHA CHANGE 3760 -- align retirement documentation with the retained contract suite -->

Existing native-transfer, native-token and burn-vault, validator proof-of-possession, and consensus BLS execution tests remain applicable.

<!-- START ALPHA CHANGE 3415 -- retire historical getter exceptions with the frozen baseline -->
The historical frozen-genesis and golden-output fixtures have been retired. Current execution and genesis tests use the embedded production artifacts, so removed contract getters no longer require an exception for historical test bytecode. The intended launch genesis must still be generated separately from the final production artifacts.
<!-- FINISH ALPHA CHANGE 3415 -- retire historical getter exceptions with the frozen baseline -->

<!-- FINISH ALPHA CHANGE 2922 -- document complete retirement of the legacy aggregate BLS precompile -->

<!-- START ALPHA CHANGE 2940 -- document validation and preservation of signed dynamic-fee fields -->

### 69. Dynamic-Fee Signed-Field Validation

Dynamic-fee transactions must retain their submitted signed fields. Admission rejects an invalid chain ID or unsupported access list instead of silently normalizing the transaction.

#### Previous defect and correction

Previously, dynamic-fee sender recovery hashed the configured chain ID without comparing it with the submitted ID. Pool admission subsequently replaced the submitted ID before computing the transaction hash. The decoder also discarded the access-list field while signing and serialization assumed an empty list.

A transaction signed for this chain could therefore have its encoded chain-ID or access-list field altered and be processed as a different normalized transaction. This affected signed-format validation and transaction-hash tracking. Cross-chain theft was not demonstrated.

The correction establishes these boundaries:

- `types/transaction_validation.go` compares dynamic-fee chain IDs using full `big.Int` values. Missing and mismatched IDs are rejected without truncation or mutation.
- `crypto/txsigner.go` hashes the actual dynamic-fee chain ID. Legacy signing behavior remains unchanged.
- `crypto/txsigner_london.go` validates before sender recovery. Local signing initializes an absent ID only on a copied transaction and rejects an explicitly mismatched ID.
- `types/rlp_unmarshal.go` requires an actual empty RLP access list. Nonempty lists and byte-string substitutes are rejected.
- `txpool/txpool.go` validates the configured chain ID and no longer rewrites the submitted ID during admission.
- `state/executor.go` validates in `Transition.Write`, even when `From` is populated, before execution, gas accounting, or receipt publication.

<!-- START ALPHA CHANGE 3547 -- align dynamic-fee chain identity documentation with positive configuration IDs -->

An explicitly encoded zero in a dynamic-fee transaction is not a request to substitute the configured ID. Supported chain configurations require a positive ID, so such a transaction fails the existing exact chain-ID comparison.

<!-- FINISH ALPHA CHANGE 3547 -- align dynamic-fee chain identity documentation with positive configuration IDs -->

The signing hash helper has no error return. A nil chain ID hashes deterministically as encoded zero without modifying the transaction or substituting configuration. Signed admission and block execution reject a missing ID; local signing fills it on its copy before hashing.

#### Supported format and simulation boundary

<!-- START ALPHA CHANGE 2946 -- clarify access-list support and complexity tradeoff -->

INNBC supports EIP-1559 dynamic-fee transactions whose access-list field is an actual empty RLP list. Nonempty access lists are valid Ethereum features but remain unsupported and are rejected; byte-string substitutes are also rejected. This provides partial EIP-1559 transaction support, with nonempty access lists unsupported and rejected.

Retaining this narrower supported format avoids adding nonempty access-list representation, signing, gas accounting and execution handling to the current implementation. This limits implementation and verification complexity; no measured performance improvement is claimed.

The tradeoff is reduced Ethereum transaction-format compatibility: clients must submit an empty access list. This does not establish full Ethereum/EVM compatibility. The security correction is that unsupported signed fields are rejected rather than silently discarded or rewritten; omitting nonempty access-list support is not itself a demonstrated security improvement.

<!-- FINISH ALPHA CHANGE 2946 -- clarify access-list support and complexity tradeoff -->

General `Transition.Apply` remains available for unsigned simulations and tracing. The signed chain-ID requirement is enforced at `Transition.Write` and signed admission boundaries.

Transaction-hash fork behavior remains unchanged. With the existing typed-hash implementation active, accepted canonical raw transactions retain the hash of their original submitted bytes.

#### Regression coverage

The regression files are:

- `crypto/dynamic_fee_validation_test.go`: signing-copy ownership, sender recovery, explicit mismatch rejection, full-width IDs, actual-ID hashing, deterministic nil hashing, and legacy fallback.
- `types/dynamic_fee_validation_test.go`: full integer comparison and strict empty-list decoding.
- `txpool/dynamic_fee_validation_test.go`: real pool admission, rejection before sender/hash mutation, and preservation of the encoded ID.
- `jsonrpc/dynamic_fee_validation_test.go`: raw submission through the real pool, original-byte hashing under the typed-hash implementation, altered-ID rejection, and access-list rejection before pool admission.
- `state/dynamic_fee_validation_test.go`: rejection with and without a cached sender, unchanged state/gas/receipts on rejection, matching-ID execution, and unsigned simulation.

<!-- START ALPHA CHANGE 3415 -- preserve chain-ID coverage without frozen genesis dependencies -->
Existing London signer and pool fixtures explicitly supply their intended chain IDs. Current genesis and execution regressions use current embedded contract artifacts; the historical Issue 7A frozen genesis and golden comparison have been retired.
<!-- FINISH ALPHA CHANGE 3415 -- preserve chain-ID coverage without frozen genesis dependencies -->

This correction adds local integer comparisons and RLP validation. It adds no registry calls, historical reads, contract regeneration, or additional EVM execution.

<!-- FINISH ALPHA CHANGE 2940 -- document validation and preservation of signed dynamic-fee fields -->

<!-- START ALPHA CHANGE 2968 -- document hardened legacy archive restore and export completion -->

<!-- START ALPHA CHANGE 3271 -- retire the legacy RLP startup importer -->
### 70. Retired Legacy RLP Archive Restore — Historical Hardening Record

The legacy `server --restore` importer has been removed together with the top-level RLP `backup` exporter. The `--restore` and `--restore-max-block-bytes` flags, their configuration fields, startup import logic and exclusive archive package are no longer available.

Current snapshot creation, snapshot bootstrap, linked recovery checkpoints, historical archive segments and `snapshot backup` retain their independent implementations.

The remainder of this point records the former importer's hardening and validation before retirement; its command examples and restore-specific settings are historical.
<!-- FINISH ALPHA CHANGE 3271 -- retire the legacy RLP startup importer -->

The previous importer trusted archive-declared lengths before allocation, did not reliably distinguish short reads from complete records, and could report success without reaching the archive's advertised endpoint.

The correction implements:

- an **8 MiB default limit**, or **8,388,608 bytes**, for each complete encoded block record, including its outer RLP prefix;
- an independent **43-byte metadata limit**, with exactly two canonical fields;
- unsigned length decoding, checked arithmetic and platform allocation checks before resizing;
- rejection of oversized declarations before payload allocation or reading;
- exact reads with `io.ReadFull`, distinguishing clean record-boundary EOF from truncated lengths or payloads;
<!-- START ALPHA CHANGE 2985 -- document structural protection before archive and embedded-extra decoding -->
- rejection of noncanonical outer length encodings;
- nonrecursive structural validation before block decoding, checking nested lengths, enclosing-list boundaries, trailing bytes and the current block format's maximum four list levels;
- independent structural validation of embedded INNBCBFT header extra data before its recursive decoder, including the header-hashing path;
- consecutive block heights, parent linkage and matching existing canonical blocks;
<!-- FINISH ALPHA CHANGE 2985 -- document structural protection before archive and embedded-extra decoding -->
- advertised endpoint height and hash checks, followed by confirmation of canonical insertion;
- rejection of premature EOF and trailing data during an actual import;
- file closure, owned signal cleanup and distinguishable cancellation errors.

Imported new blocks still undergo finalized-block verification. These parser and completion checks do not replace consensus validation.

Restore remains incremental: valid blocks imported before a later failure are retained. Nonzero-start archives are supported when the required canonical parent is available.

If the advertised endpoint already exists locally with the matching canonical hash, restore takes the deliberate shortcut of returning success without validating the remainder of the archive. During an actual import, reaching the endpoint requires clean EOF afterward.

Cancellation is checked between operations. It does not forcibly interrupt a blocked filesystem read, verification operation or database call.

#### Basis for the default allowance

Sizing and allocation measurements use:

- `archive/restore_size_measurement_test.go`;
- `archive/restore_size_benchmark_test.go`;
- `consensus/innbcbft/block_builder_test.go`.

The largest measured executed transaction-body fixture encoded a complete block record of **1,251,745 bytes**. The 8 MiB default provides approximately **6.7 times** that size.

These executed fixtures are not claims of fully finalized consensus-valid blocks. Separate synthetic encoding fixtures cover larger extra-data fields and validator metadata. For example, the 256-validator synthetic fixture encoded a complete block record of **48,731 bytes**, including **48,222 bytes** of extra data.

The default is a practical **archive-import resource policy**, not a consensus block-size rule or proof that every legitimate historical block fits. Encoded-byte limits also do not establish a fixed bound on total decoding and execution memory. Allocation benchmark results are not peak-memory measurements.

#### Operator invocation and override

Legacy restore is invoked during server startup:

```bash
innbc-chain server \
  --chain /path/genesis.json \
  --data-dir /path/node \
  --restore /path/backup.rlp
```

Use the default unless a larger allowance is explicitly required. Without a configuration file, a 16 MiB allowance can be selected with:

```bash
innbc-chain server \
  --chain /path/genesis.json \
  --data-dir /path/node \
  --restore /path/backup.rlp \
  --restore-max-block-bytes 16777216
```

When using `--config`, place the allowance in that configuration under the existing configuration-file precedence rules.

JSON field:

```json
"restore_max_block_bytes": 16777216
```

YAML field:

```yaml
restore_max_block_bytes: 16777216
```

HCL field:

```hcl
restore_max_block_bytes = 16777216
```

These are fields to incorporate into the existing server configuration, not complete configuration files.

The allowance must be positive and representable within the platform allocation limit. Zero does not disable the bound. The importer never increases its allowance because an archive declares a larger record.

Increasing the block-record allowance does not increase the independent 43-byte metadata limit.

#### Implementation and regression coverage

The implementation spans:

- `archive/restore.go` and `archive/types.go`;
- `server/config.go` and `server/server.go`;
- `command/server/config/config.go`;
- `command/server/params.go`, `command/server/init.go` and `command/server/server.go`.

`archive/restore_hardening_test.go` covers record boundaries, rejection before payload reads or allocation, extreme lengths, truncation, canonical metadata, fragmented reads, buffer reuse, incomplete imports, sequence and parent errors, conflicting canonical blocks, unsuccessful insertion despite a nil writer error, trailing data, cancellation, file closure, explicit overrides and maximum-height handling.

<!-- START ALPHA CHANGE 2985 -- document restore path wiring and independent progress snapshots -->
`command/server/restore_limits_test.go` covers default and explicit CLI allowances, runtime propagation, JSON/YAML/HCL configuration, and invalid unsigned values. The HCL path explicitly parses the allowance because the pinned decoder does not directly support a `uint64` field.

The named `restore_file` setting is also decoded in HCL, alongside JSON and YAML. `command/server/restore_path_test.go` verifies that both the filename and allowance reach runtime configuration.

Additional structural regressions are in `types/rlp_structure_test.go`, `archive/archive_nesting_test.go` and `consensus/innbcbft/extra_nesting_test.go`. They cover excessive nesting, malformed nested lengths, supported dynamic-fee block encoding, validator metadata and the embedded-extra helper used by header hashing.

`helper/progress/chain.go` returns an independent progression snapshot while holding its read lock. Consumers no longer retain the shared object being updated by restore or sync workers. `helper/progress/progression_snapshot_test.go` checks isolation and concurrent readers.

These structural checks bound decoder nesting. They do not establish a universal bound on total decoded-object or execution memory.
<!-- FINISH ALPHA CHANGE 2985 -- document restore path wiring and independent progress snapshots -->

<!-- START ALPHA CHANGE 3271 -- retire the legacy RLP exporter and its exclusive operator API -->
### 71. Retired Legacy RLP Backup Command — Historical Hardening Record

The top-level legacy RLP `backup` command has been removed together with its companion `server --restore` importer. Their exclusive operator gRPC `Export` and `BlockByNumber` methods, protocol messages, archive implementation and command-specific tests have also been removed.

The supported database-copy command remains `snapshot backup`. Snapshot building, historical archive publication, recovery checkpoints, trie rebuilding, normal block lookup and operator subscriptions retain their existing implementations.

The remainder of this point records the former exporter's behavior and hardening before retirement; its invocation examples are historical.
<!-- FINISH ALPHA CHANGE 3271 -- retire the legacy RLP exporter and its exclusive operator API -->

This feature is not used by the current snapshot-producing logic. It is separate from:

- snapshot creation and snapshot bootstrap;
- historical archive segments and recovery checkpoints;
- `snapshot backup`, which uses the independent consistent MDBX database-copy implementation described in point 40.

<!-- START ALPHA CHANGE 3271 -- identify exporter hardening as historical -->
Before retirement, the legacy exporter's error path was corrected to report incomplete operations accurately.
<!-- FINISH ALPHA CHANGE 3271 -- identify exporter hardening as historical -->

#### Invocation

The command is registered by `command/root/root.go`, defined in `command/backup/backup.go`, and invokes `archive.CreateBackup` through `command/backup/params.go`.

Example:

```bash
innbc-chain backup \
  --grpc-address 127.0.0.1:9632 \
  --out /path/backup.rlp \
  --from 0 \
  --to 1000
```

The source node must be running and have the requested complete block history available. Use its configured operator gRPC address.

`--out` is required. The output file must not already exist. `--from` defaults to zero. Omitting `--to` selects the current head through the existing endpoint-selection logic.

<!-- START ALPHA CHANGE 2985 -- document explicit endpoint failures and inclusive export ranges -->
Completion is checked against the endpoint resolved by `determineTo` and written into archive metadata. Omitting `--to` selects the current head; an explicit target above that head retains the existing clamp to the head.

An explicitly requested historical endpoint must be retrieved and decoded at its requested height. Lookup failures, missing responses, malformed block data and wrong-height responses return errors rather than silently replacing that endpoint with the current head.

The export RPC receives an already resolved inclusive range. Equal endpoints export one block, including `0..0` for genesis alone. Zero is no longer a latest-height sentinel at this RPC boundary. Direct RPC callers must resolve a latest-height request before calling Export.

The handler checks cancellation between block operations and stops before incrementing the final height, avoiding unsigned wraparound at the maximum height. These checks do not forcibly interrupt a blocked storage operation.

The backup command closes its owned gRPC connection when it returns, on both success and failure.
<!-- FINISH ALPHA CHANGE 2985 -- document explicit endpoint failures and inclusive export ranges -->

This command is explicitly invoked by the operator. It is not automatically run during normal server startup.

#### Corrected failure behavior

Previously, cancellation or premature EOF after receiving at least one batch could be reported as successful export. The retained file could therefore contain only a prefix of the advertised archive.

`archive/backup.go` now:

- propagates cancellation and other RPC errors, including errors received after the endpoint batch;
- accepts EOF as successful completion only after receiving the consecutive batch range through the resolved endpoint;
- rejects missing, empty, overlapping, skipped or out-of-range batches;
- checks complete metadata and payload writes;
- owns and releases its termination-signal registration;
- attempts to close and remove failed output files, logging cleanup failures.

The export completion check uses stream batch ranges. It is not an additional finalized-block verification pass or an independent endpoint-hash proof. The hardened `--restore` consumer performs decoded block, canonical endpoint and finalized-block checks when importing.

This closes misleading success reporting in the legacy exporter without changing consensus rules or the current snapshot implementation.

#### Regression coverage

`archive/backup_test.go` supplies explicit endpoints to the existing stream fixtures.

`archive/backup_completion_test.go` covers successful EOF, premature EOF, cancellation before transfer, cancellation after partial transfer, cancellation after the endpoint, other RPC errors, invalid batch ranges and short writes.

<!-- START ALPHA CHANGE 2985 -- document endpoint and production export handler regressions -->
A local gRPC fixture exercises the real `CreateBackup` path, checking that complete output is retained and premature EOF or cancellation removes failed output. It also reads a successful file through the archive reader and checks its metadata and block records.

`archive/backup_target_test.go` covers historical endpoint failures, wrong-height responses, missing status, genesis selection and the retained head-selection policy.

`server/export_range_test.go` additionally connects the real backup client to the production Export and BlockByNumber handlers over local gRPC, backed by in-memory blockchain storage. It checks exact archive bytes for genesis-only, positive one-block and multi-block ranges. Separate handler assertions cover reversed ranges, unavailable blocks, cancellation and maximum-height completion. These storage/export fixtures do not claim consensus-valid finalized execution.
<!-- FINISH ALPHA CHANGE 2985 -- document endpoint and production export handler regressions -->

<!-- FINISH ALPHA CHANGE 2968 -- document hardened legacy archive restore and export completion -->

<!-- START ALPHA CHANGE 2990 -- document simulated transfer estimation and explicit gas ceilings -->

### 72. Gas Estimation Executes Candidate Transfers Within Their Allowances

Previously, `eth_estimateGas` returned intrinsic gas immediately for a positive-value transaction with empty calldata and a destination address. That classification did not establish whether the destination executed contract code or an active precompile.

An empty-input payment to a contract could therefore receive a 21,000 estimate despite requiring additional execution gas or reverting. Consensus execution still executed the destination; the defect affected RPC estimation.

`jsonrpc/eth_endpoint.go` now establishes the effective gas ceiling before attempting the optimized probe:

- The selected block's gas limit supplies the block ceiling.
- An explicitly supplied positive gas value supplies an additional ceiling.
- Explicit zero gas is rejected before simulation. Omitted gas uses the selected block's ceiling.
- For a supplied sender, the estimator checks value affordability and reserves that value before calculating the gas allowance.
- Legacy affordability uses `gasPrice`; dynamic-fee affordability uses the maximum `maxFeePerGas` cap, rather than the effective price.
- The allowance is compared using `big.Int` before conversion, preventing large allowances from wrapping into a smaller gas ceiling.
- A ceiling below the complete intrinsic cost is rejected without simulation.

The relevant fee field is defaulted through the existing recommendation only when omitted. A supplied zero fee remains zero for simulation: no division is performed, but transfer value must still be affordable.

Gas and sender presence are captured before decoding. An explicitly supplied zero sender address is treated as supplied. Requests without `from` retain the default zero simulation caller and skip the explicit-sender funding calculation. They do not receive fabricated funds; execution still checks value transfers against that caller's actual balance.

Decoding operates on a request copy, so supplying defaults does not turn omitted fields into explicit fields when the same request is reused.

<!-- START ALPHA CHANGE 2993 -- document selected-state estimation nonce ownership -->
For a supplied sender, the estimator reads its nonce and balance together from the captured header's state root. That nonce replaces any caller-supplied nonce on the simulation request copy. A genuinely absent account supplies nonce zero and balance zero.

The same account is reused for affordability, avoiding a duplicate account lookup. Decoding does not perform another latest-state nonce lookup. Historical estimates therefore use the nonce expected by their selected state, and head advancement cannot substitute a newer nonce after header selection. Account read failures remain errors; they do not trigger a latest-state fallback.

Requests without `from` retain the existing default zero simulation caller and nonce zero.
<!-- FINISH ALPHA CHANGE 2993 -- document selected-state estimation nonce ownership -->

A candidate transfer is simulated at intrinsic gas. A successful ordinary funded transfer returns that amount after one execution probe. Low-gas exhaustion or a gas-dependent revert proceeds through the existing search within the established ceiling. Local-state failures stop immediately, preserving their classification.

Probes remain nonpayable simulations and do not commit state. Their success alone does not establish fee affordability or normal transaction-admission validity; the funding calculation is performed separately.

<!-- START ALPHA CHANGE 2993 -- distinguish estimator nonce resolution from shared decoding -->
This correction changes RPC estimation behavior without changing consensus execution, intrinsic gas pricing or the existing `eth_call` fee-defaulting behavior. The estimator supplies its selected-state nonce before calling the shared decoder; the decoder's behavior for other callers remains unchanged. The existing gas search's limitations for arbitrary gas-dependent execution remain.
<!-- FINISH ALPHA CHANGE 2993 -- distinguish estimator nonce resolution from shared decoding -->

#### Regression Coverage

`jsonrpc/eth_state_test.go` verifies the one-probe funded-transfer path and rejection without execution data when the ceiling is insufficient.

`jsonrpc/issue7a_failure_test.go` retains local-state failure precedence and uses the actual intrinsic cost for its input-bearing final-probe fixture.

`jsonrpc/estimate_transfer_test.go` covers explicit zero and insufficient gas, block and funding ceilings, value reservation, zero fees, full-width arithmetic, dynamic-fee maximum-cap affordability, supplied zero and omitted senders, request preservation, and recovery after failed intrinsic probes.

<!-- START ALPHA CHANGE 2993 -- document selected-state nonce regressions -->
`jsonrpc/estimate_nonce_test.go` constructs actual in-memory state roots with sender nonces 3 and 5. Real execution verifies historical and latest estimation, replacement of a caller-supplied nonce on the request copy, the existing pending-to-selected-header behavior, and unchanged stored nonces after simulation.

A deterministic head-advance fixture verifies that execution retains the captured state and nonce. Additional regressions cover genuinely absent accounts without latest-state fallback, explicitly supplied zero senders, account failures and malformed account responses. Assertions require one selected-state account lookup for nonce and affordability together.
<!-- FINISH ALPHA CHANGE 2993 -- document selected-state nonce regressions -->

In-memory execution fixtures additionally exercise ordinary transfers, storage-writing contracts, reverting contracts with return data, gas-dependent reverts, and the active identity precompile without bytecode. They check successful and immediately lower failing allowances and verify that simulation does not publish balance, nonce or storage changes.

<!-- FINISH ALPHA CHANGE 2990 -- document simulated transfer estimation and explicit gas ceilings -->

<!-- START ALPHA CHANGE 2998 -- document historical call and trace-call nonce correction -->

### 73. Historical Calls and Trace Calls Use Their Selected Simulation Nonce

Previously, `eth_call` and `debug_traceCall` selected a requested state but decoded a supplied sender's nonce from the latest state. Execution still enforced nonce equality. Historical queries could therefore fail with an incorrect-nonce error, and latest-state queries could encounter the same mismatch if the head advanced between header selection and nonce lookup.

`jsonrpc/helper.go` now provides a simulation-specific decoder used by both endpoints. It works on a request copy and resolves the nonce against the captured header's state root, without another latest-state nonce lookup.

The nonce policy is:

- A supplied sender without a sender nonce override uses its selected-state nonce. Genuine account absence supplies zero; account read failures remain errors.
- A sender nonce override takes precedence, including explicit zero.
- An omitted sender without a relevant nonce override retains the existing zero-caller, nonce-zero policy.
- A nonce override for the zero address applies to that effective caller when `from` is omitted.
- Overrides for unrelated accounts do not change the transaction nonce.

The simulation request replaces any caller-supplied transaction nonce without changing the original arguments or override map. Missing requests, missing headers and malformed account responses produce errors.

`eth_call` retains its existing nonpayable simulation setting and fee defaulting. `debug_traceCall` retains its existing payment and fee checks. Shared transaction decoding, block execution and txpool admission remain unchanged. Simulations do not commit their state changes.

`jsonrpc/simulation_nonce_test.go` covers historical number and hash selection, latest and pending selection, controlled head advancement, nonce overrides, omitted and explicitly supplied zero senders, genuine absence, account failures and malformed responses. In-memory execution verifies both call paths against states with different nonces and confirms that nonce, balance and storage changes are not published.

Gas estimation already received the corresponding selected-state nonce correction described in point 72.

<!-- FINISH ALPHA CHANGE 2998 -- document historical call and trace-call nonce correction -->

<!-- START ALPHA CHANGE 3003 -- document cancellation-aware operator block-event subscriptions -->

### 74. Operator Block-Event Subscriptions Release Resources on Cancellation

The operator gRPC `Subscribe` method streams notifications about newly added canonical blocks.

Previously, its handler waited in `GetEvent()` without observing the RPC context. Cancelling a client while the chain was idle or stalled could leave the handler and subscription retained until another event arrived. Repeated cancellations could accumulate retained resources. Forced gRPC shutdown cancelled stream contexts, but the handler's event wait did not observe that cancellation. Send failures also ended the handler without returning the error.

This was a local resource-cleanup and availability defect. The operator listener's enforced loopback binding reduces exposure, but local cancellation still requires reliable cleanup.

#### Implementation

`server/system_service.go` now gives each handler sole ownership of its subscription:

- Exactly one deferred `UnsubscribeEvents` call is installed immediately after subscription creation.
- The idle wait selects between the RPC context's cancellation signal and the existing event channel.
- Cancellation is checked before waiting, after receiving an event, and before sending its converted notification.
- Cancellation and deadline errors use `status.FromContextError(ctx.Err()).Err()`, preserving the corresponding gRPC status.
- `Send` errors are returned directly.
- A nil event or closed event channel ends the handler and runs deferred cleanup.

The handler does not close the publisher's event channel. Existing event-stream locking coordinates publication and subscription removal.

Healthy subscriptions may remain idle indefinitely. The correction adds no idle timeout, helper goroutine, polling loop, blockchain read, or consensus change.

#### Regression Evidence

`server/subscription_cancellation_test.go` covers:

- Idle cancellation without publishing a subsequent block event.
- Already-cancelled and already-expired contexts.
- Normal delivery followed by continued healthy operation.
- Direct send-error propagation and nil-event completion.
- Repeated subscribe/cancel cycles and cancellation racing with publication.
- Real gRPC client cancellation, connection closure and forced server shutdown.
- Cancellation and forced shutdown while an actual transport send is blocked by backpressure.

`blockchain/testing.go` supplies a fixture using the production event stream and its actual subscription registry. Registry counts are read under the existing lock.

The transport fixtures establish the handler's actual blocked wait before interruption. Backpressure fixtures use fixed receive windows, bounded notifications and stack assertions confirming a wait inside the pinned gRPC transport's write-quota code.

Completion assertions require both handler return and removal from the subscription registry before fixture cleanup runs. A client-side cancellation error alone is insufficient.

Validation passed: focused regressions, 20 race-enabled repetitions, broader tests for `blockchain`, `server`, `jsonrpc`, `syncer` and `helper/progress`, and vet for those packages.

#### Scope and Closure

Issue 7 is closed: operator block-event subscriptions now observe RPC cancellation and release their subscriptions, including during transport backpressure.

This correction does not introduce global blockchain-subscription shutdown or a limit on all active operator streams. `Blockchain.Close` still closes storage without closing every subscription, and production unsubscription closes its separate `closeCh` rather than the event channel. The operator handler owns its subscription until its single deferred cleanup runs.

Graceful gRPC shutdown retains its existing forced-stop fallback. Forced stop cancels stream contexts and interrupts transport sends; the corrected handler then completes its cleanup.

<!-- FINISH ALPHA CHANGE 3003 -- document cancellation-aware operator block-event subscriptions -->

<!-- START ALPHA CHANGE 3020 -- document runtime RLP protection before recursive parsing -->

### 75. Transaction and Block RLP Validation Before Recursive Parsing

Transaction and block decoding now validates supported encoding structure before invoking the recursive RLP parser.

Previously, malformed nested lists could reach recursive parsing and allocate decoded values before transaction field checks, signature verification or block validation. Raw transaction admission also applied the pool's size check only after decoding.

This exposed validators and RPC nodes to avoidable resource consumption through malformed RPC submissions, transaction gossip, downloaded blocks and consensus proposals. The reachable decoding gap was confirmed by source inspection; no exhaustion threshold, parser panic or chain halt was demonstrated.

#### Implementation

`types/rlp_unmarshal.go` protects the shared `Transaction.UnmarshalRLP` and `Block.UnmarshalRLP` entry points.

`types/rlp_runtime_validation.go` checks supported field counts and field types before recursive decoding. It reuses the checked length handling and iterative nesting validation in `types/rlp_structure.go`.

The checks require:

- One complete list-root encoding, with no trailing bytes.
- Valid enclosing lengths and canonical RLP length encodings.
- Nine fields for legacy transactions, ten for state transactions and twelve for dynamic-fee transactions.
- Byte-string transaction fields, except the required empty access list in dynamic-fee transactions.
- Recognition of supported standalone typed-transaction prefixes before validating their payloads.
- Exactly three outer block fields, with supported transaction and uncle-header representations.
- Historical headers with fifteen fields, or sixteen fields including base fee.
- Maximum list depths of one for standalone legacy/state transactions, two for standalone dynamic-fee transactions and four for complete blocks.

Unsupported field lists are rejected without recursively decoding their contents. The structural checks reference the original input and do not allocate decoded values.

`txpool/raw_transaction.go` supplies the shared `DecodeRawTransaction` admission boundary. Raw RPC submission, transaction gossip and operator transaction submission use it to enforce the existing **128 KiB transaction-pool allowance before decoding**. The pool retains its later size check for transactions supplied through other paths.

#### Compatibility and Scope

Supported legacy, dynamic-fee and state encodings retain their decoded contents and hashes. Dynamic-fee transactions still require an empty access list; nonempty access lists remain unsupported.

<!-- START ALPHA CHANGE 3032 -- distinguish structural protection from shared canonical-field validation -->

Calldata remains opaque. Embedded consensus extra data retains its separate structural protection because its encoded contents cannot be inspected through the outer block structure. Address and integer fields also receive the canonical-field checks described in point 76.

Block and historical transaction decoding do not inherit the pool's admission size limit. The generic `UnmarshalRlp` helper and receipt decoders retain their separate behavior. Stored transactions reach the shared transaction-field decoder and therefore receive the canonical-field checks; this does not establish structural protection for every storage envelope or unrelated decoder.

The structural correction protects the reviewed transaction/block entry points before recursive parsing. It does not establish a universal consensus block-size limit, a total-memory bound or protection for every unrelated RLP decoder. Large supported shallow encodings can still require substantial decoding memory.

<!-- FINISH ALPHA CHANGE 3032 -- distinguish structural protection from shared canonical-field validation -->

#### Regression and Performance Evidence

The regression files are:

- `types/rlp_runtime_validation_test.go`
- `txpool/rlp_runtime_validation_test.go`
- `jsonrpc/rlp_runtime_validation_test.go`
- `syncer/rlp_runtime_validation_test.go`
- `consensus/innbcbft/rlp_runtime_validation_test.go`

They cover malformed lengths, deep nesting, trailing bytes, typed payloads, unsupported field shapes, raw admission size boundaries, rejection before admission or proposal execution, valid signed submissions, encoding/hash preservation, historical headers and opaque calldata.

`archive/restore_test.go` now expects structural rejection of metadata supplied as a block, reflecting the earlier validation boundary.

Validation passed: focused regressions, twenty race-enabled repetitions, the broader affected-package suite and vet.

<!-- START ALPHA CHANGE 3033 -- describe the benchmark baseline with shared field validation -->

`types/rlp_runtime_validation_benchmark_test.go` measures three paths:

- `guard_only`: supported structure and canonical-field validation without recursive decoding.
- `guarded_decode`: public transaction/block decoding, including pre-parser validation and shared transaction-field validation.
- `parser_entry_baseline`: direct parser entry that bypasses public preflight but still performs shared transaction-field validation during decoding.

The baseline is therefore not an unvalidated decoder and does not isolate the entire canonical-field correction's overhead.

After the canonical-field correction, the supplied three-run benchmark measured approximately **146.5–150.4 ns** for the legacy transaction guard, **193.4–195.6 ns** for the dynamic-fee guard and **2.60–2.64 µs** for the ten-transaction block guard. Guard-only cases reported **zero bytes allocated and zero allocations per operation**.

These measurements concern the tested encoding fixtures on the reported machine. They demonstrate a small local validation cost; they do not establish zero overhead, universal performance guarantees or absence of allocations during full decoding.

<!-- FINISH ALPHA CHANGE 3033 -- describe the benchmark baseline with shared field validation -->

The reviewed transaction/block pre-parser exposure is closed within this scope.

<!-- FINISH ALPHA CHANGE 3020 -- document runtime RLP protection before recursive parsing -->

<!-- START ALPHA CHANGE 3034 -- document rejection of noncanonical transaction fields and envelopes -->

### 76. Malformed Transaction Fields Are Rejected Instead of Silently Normalized

Transaction decoding now rejects malformed field representations that could previously become a different canonical transaction after decoding and serialization.

Previously, a nonempty recipient whose length differed from twenty bytes became `To = nil`, representing contract creation. Integer decoding accepted leading zero bytes, including a single zero byte instead of the required empty encoding for integer zero. Redundant legacy transaction type prefixes and block type markers could disappear during serialization. Malformed state-transaction sender lengths could also become the zero address.

For example, replacing a signed creation transaction's empty recipient with a one-byte recipient could decode back into the original creation meaning. Signature recovery then operated on that reconstructed transaction. Subject to ordinary admission checks, malformed submitted bytes could therefore be accepted under a canonicalized transaction hash.

This was a format-validation and client-tracking defect. No theft, signature forgery or invalid committed-state acceptance was established.

#### Implementation

`types/transaction_rlp_validation.go` defines shared scalar-field validation and `ErrNonCanonicalTxEncoding`.

The supported representations now require:

- An empty byte-string recipient for contract creation, or exactly twenty bytes for an ordinary destination.
- A twenty-byte zero address to remain an ordinary destination, distinct from contract creation.
- Empty bytes for integer zero and no leading zero byte for positive integers.
- Minimal integer encodings for nonce, gas, prices/caps, value, chain ID where present, and signature integers.
- Exactly twenty bytes for the state-transaction sender, including the zero address.
- Legacy transactions to begin directly with their RLP list, without a redundant `0x00` prefix.
- Legacy transactions inside blocks to omit the redundant legacy type marker.

`types/rlp_runtime_validation.go` applies the field checks before recursive transaction/block parsing and rejects redundant legacy markers in block transaction lists.

`types/rlp_unmarshal.go` applies the same validation in the shared transaction-field decoder before assigning decoded fields. Recipient and state-sender decoding also propagate byte-string errors and explicitly reject invalid lengths.

Existing numeric-range, chain-ID, access-list and signature checks remain applicable. Leading zeros in correctly sized addresses and arbitrary calldata remain permitted.

The complete-input checks from point 75 continue to reject trailing bytes on the guarded standalone transaction and block paths. The generic parser was not changed.

#### Storage Compatibility

Stored transactions also reach the shared transaction-field decoder. Valid serializer-produced storage records retain their encoding and decoded contents. Previously stored malformed transaction fields can now cause decoding errors rather than silent normalization.

The supplied tests cover storage round trips and malformed stored consensus fields. Deployed databases were not inspected, so these results do not establish whether historical malformed records exist.

Receipt formats and unrelated storage-envelope handling retain their separate behavior.

#### Regression Evidence

The regression files are:

- `types/transaction_canonical_test.go`
- `jsonrpc/transaction_canonical_test.go`
- `txpool/transaction_canonical_test.go`

They cover invalid recipient and state-sender lengths, list-valued recipients, nonminimal encodings of every supported transaction integer field, redundant legacy envelopes, block decoding and shared storage decoding.

Valid encoding fixtures cover contract creation, ordinary destinations, the zero address, integer zero and calldata containing leading zeros.

The RPC regressions begin with correctly signed legacy and dynamic-fee creation transactions. Altered encodings are rejected before pool admission; the original transactions enter the real pool with their canonical bytes and expected hashes under the active typed-hash implementation.

Gossip and operator regressions verify rejection before signature recovery, blockchain reads or pool occupancy changes.

Validation passed: focused regressions, interaction tests with the earlier RLP and dynamic-fee corrections, twenty race-enabled repetitions, the broader affected-package suite, vet and the updated benchmarks described in point 75.

The reviewed malformed-transaction normalization issue is closed within this scope. These checks do not add registry calls, historical scans or additional EVM execution.

<!-- FINISH ALPHA CHANGE 3034 -- document rejection of noncanonical transaction fields and envelopes -->

<!-- START ALPHA CHANGE 3039 -- document contract creation rollback and persisted-state regressions -->

### 77. Failed Contract Creation Restores Destination State

Contract creation now restores its creation snapshot when value transfer fails or deployment authorization rejects the caller.

Previously, `state/executor.go` created the destination account and, under EIP158, incremented its nonce before transferring value and checking deployment permissions. The insufficient-transfer and ordinary allowlist/blocklist rejection branches returned failed execution results without restoring that snapshot.

A funded deployment rejected by policy could therefore retain transferred value and a destination account with nonce one despite producing a failed receipt and installing no code. An insufficient-value creation could retain destination account changes. Ordinary failed execution results did not trigger the outer transaction rollback.

This was an execution-state correctness defect. Constructor execution failures already had rollback; the correction closes the earlier failure branches.

#### Implementation and Preserved Semantics

`Transition.applyCreate` uses the shared `revertCreation` helper for failures after the creation snapshot, including:

- Destination account initialization failures.
- Insufficient value transfer.
- Deployment allowlist or blocklist rejection.
- Deployment-policy state-read failures.
- Constructor failure, excessive code size and applicable code-deposit failure.
- Local-state failures during code installation.

The snapshot remains after the creator's nonce increment. Ordinary creation failure therefore restores destination changes and transferred value while preserving the creator's nonce consumption and applicable gas behavior.

Successful rollback preserves the original execution error and gas result. A rollback failure is retained through `Txn.setError` as a local-state failure, preventing acceptance as an ordinary failed receipt or publication of the resulting state. An earlier retained local failure keeps precedence.

The deliberate pre-Homestead code-deposit-out-of-gas exception remains: that branch does not acquire an unconditional rollback.

Deployment-policy rejection requires the corresponding allowlist or blocklist to be configured. Internal EVM `CREATE` and `CREATE2` already check value affordability before calling the host; their policy-rejection paths still require this rollback correction.

#### Regression Evidence

The implementation and regression files are:

- `state/executor.go`
- `state/creation_rollback_test.go`
- `state/immutable-trie/creation_rollback_test.go`

The regressions establish:

- Insufficient-value and policy-rejected creation restore destination changes.
- Previously funded destinations retain their original balances.
- Policy-rejected top-level transactions produce failed receipts with the expected sender nonce and gas charges.
- Internal `CREATE` and `CREATE2` rejection restores child changes even when the parent handles the failure and succeeds.
- Successful creation, constructor revert, address collision, maximum code size and both code-deposit branches retain their expected behavior.
- Local failures prevent receipt acceptance and commit; rollback failures remain retained.
- Committed state remains correct after closing MDBX and reopening it through a fresh state instance.

Supplied validation passed: focused regressions, twenty race-enabled repetitions, broader tests covering `state/...`, `consensus/innbcbft/...`, `jsonrpc` and `server`, and vet over those packages.

#### Scope and Closure

The reviewed early-return creation rollback issue is closed within this scope. The correction uses local snapshot restoration and adds no registry calls, historical searches or repeated constructor execution.

<!-- START ALPHA CHANGE 3045 -- update the separately resolved transaction affordability finding -->
The separate upfront gas-plus-value and dynamic-fee maximum-cap affordability finding is resolved by the execution validation described in point 78.
<!-- FINISH ALPHA CHANGE 3045 -- update the separately resolved transaction affordability finding -->

<!-- FINISH ALPHA CHANGE 3039 -- document contract creation rollback and persisted-state regressions -->

<!-- START ALPHA CHANGE 3046 -- document execution affordability validation and regression evidence -->

### 78. Transaction Execution Checks Maximum Gas Cost Plus Value

Normal transaction execution now verifies the sender's full upfront obligation before debiting gas or executing the transaction:

| Transaction type | Required sender balance |
| --- | --- |
| Legacy | `gas limit × gas price + transfer value` |
| Dynamic fee | `gas limit × maximum fee cap + transfer value` |

Previously, the execution path checked its upfront gas debit without consistently requiring sufficient balance for both gas and value. Under the current effective-price charging path, a dynamic-fee transaction could also pass that check despite insufficient balance for its maximum fee-cap obligation.

This was an execution-validation defect. The correction rejects insufficiently funded transactions before execution, rather than allowing them to proceed to a value-transfer failure or ordinary failed receipt.

#### Implementation and Preserved Behavior

In `state/executor.go`, `Transition.checkTxBalance` uses full-width `big.Int` arithmetic and the transaction type's applicable fee field. A dynamic-fee transaction uses its maximum fee cap even when its effective gas price is lower.

The check runs after fee validation and before the existing upfront gas debit. Rejection leaves sender and destination state, execution context, block gas accounting and receipts unchanged.

A balance below the maximum gas cost returns the existing gas-affordability error. A balance sufficient for gas but insufficient for gas plus value returns the combined-affordability error. Retained local-state failures take precedence over either transaction error.

Affordability validation does not debit the maximum obligation. The existing fork handler still determines the actual upfront debit, effective gas charging and refunds. Zero fees still require sufficient balance for the transfer value.

Nonpayable simulations retain their existing fee-check bypass; value transfers still require funds. System transactions retain their separate validation path. The earlier contract-creation rollback correction remains necessary for failures occurring after creation begins.

#### Regression Evidence

The implementation and regression files are:

- `state/executor.go`
- `state/transaction_affordability_test.go`
- `state/immutable-trie/transaction_affordability_test.go`

The regressions verify:

- Rejection of insufficient gas, combined gas-plus-value and dynamic-fee maximum-cap obligations.
- Rejection with either recovered or cached sender identity, before execution and receipt publication.
- Exact-balance acceptance with the expected actual gas charges.
- Zero-fee value affordability and full-width arithmetic without truncation.
- Local-state failure precedence.
- Preserved simulation and system-transaction behavior.
- Sequential execution against the remaining balance, rather than only the initial balance.
- Imported-block rejection when a later transaction becomes unaffordable.
- Correct committed balances and nonce after closing MDBX and reopening through a fresh state instance.

Supplied validation passed: focused affordability, creation-rollback and execution-state-failure regressions; twenty race-enabled repetitions of affordability and creation-rollback tests; broader tests covering `state/...`, `consensus/innbcbft/...`, `txpool`, `jsonrpc` and `server`; and vet over those packages.

#### Scope and Closure

The reviewed execution affordability finding is closed within this scope. The correction adds local arithmetic and a sender-balance check, with no registry calls, historical searches or additional EVM execution.

It tightens normal transaction acceptance while preserving existing fee charging, creation rollback and the separate simulation and system-transaction paths.

<!-- FINISH ALPHA CHANGE 3046 -- document execution affordability validation and regression evidence -->

<!-- START ALPHA CHANGE 3080 -- document role-specific MDBX defaults and verified configuration -->
### 79. One-TiB Full-History Storage Defaults With Separate Recent-Snapshot Configuration

The shared server configuration, blockchain storage fallback and trie storage fallback now default to **1024 GiB (1 TiB)**.

For a full-history node, this means an independently configured 1 TiB allowance for the blockchain database and 1 TiB for the trie database. It does not mean a combined 1 TiB allocation or immediate reservation of 2 TiB of physical disk space.

Generated node1 environments, full-history packaging templates and the systemd full-history default use:

```ini
INNBC_MDBX_MAP_SIZE_GB=1024
```

Generated recent nodes retain:

```ini
INNBC_MDBX_MAP_SIZE_GB=128
```

Recent blockchain storage continues physically retiring expired segment directories. Trie reclamation remains the existing maintenance workflow: replace the node's chain data using a recent snapshot containing the reachable checkpoint state. This change adds no live trie garbage collector.

The standalone builder explicitly separates its destination allowances:

```ini
INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB=1024
INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB=128
```

| Operation | Packaged destination allowance |
| --- | --- |
| Segmented snapshot for recent nodes | 128 GiB |
| Checkpoint snapshot for full-mode recovery | 1024 GiB |
| Linked recovery checkpoint | 1024 GiB |
| Historical archive databases | 1024 GiB per destination database |
| Standalone blockchain database backup | 1024 GiB |

Segmented dispatch requires its explicit setting. A missing or empty segmented allowance produces an error before the node binary is invoked; the wrapper supplies no compatibility fallback.

Direct snapshot and trie-rebuild commands inherit the shared 1024 GiB default. A direct `snapshot build segmented` command must explicitly pass `--mdbx-map-size-gb 128` for the intended recent-node configuration. Direct commands do not automatically read the packaged builder env file.

The source databases are opened read-only using their stored geometry. Destination settings govern the databases constructed by the operation. Source and destination allowances need not be identical: the copied data must fit the chosen destination allowance. This change introduces no map-size equality rule or snapshot-format change.

<!-- START ALPHA CHANGE 3271 -- remove obsolete RLP backup and restore configuration guidance -->
The legacy top-level RLP `backup` command and `server --restore` importer have been retired. The current snapshot and database-backup commands retain the destination MDBX allowances described above.
<!-- FINISH ALPHA CHANGE 3271 -- remove obsolete RLP backup and restore configuration guidance -->

Consensus retains its separate 1 GiB MDBX allowance.

<!-- START ALPHA CHANGE 3513 -- reference dedicated database page-size customization documentation -->

Database page size is configurable through `INNBC_MDBX_PAGE_SIZE_BYTES`, defaulting to 4096 bytes. The independent `INNBC_MDBX_MAP_SIZE_GB` setting controls the configured growth allowance. See [point 102: Configurable MDBX Page Size and Maximum Storage Capacity](#102-configurable-mdbx-page-size-and-maximum-storage-capacity) for supported values, capacity ceilings, builder configuration and compatibility safeguards.

<!-- FINISH ALPHA CHANGE 3513 -- reference dedicated database page-size customization documentation -->

Supplied validation passed shell syntax checking, packaged builder dispatch tests, focused MDBX geometry and data-preservation regressions, generated node-role assertions, snapshot CLI default and override checks, twenty race-enabled repetitions, affected package suites and vet. These tests do not measure operation with a physically full 1 TiB database.

Generated `localtestclient` and `dist` outputs are produced from the completed source and templates; they are not manually patched.
<!-- FINISH ALPHA CHANGE 3080 -- document role-specific MDBX defaults and verified configuration -->

<!-- START ALPHA CHANGE 3086 -- document simplified genesis generation and accepted epoch-retention constraints -->

### 80. Genesis Optimization: Default Configuration and Recent-History Compatibility

The official genesis generator applies the defaults defined in the node source. Command-line arguments that repeat those defaults can be omitted.

The following table covers the original genesis command and its epoch and block-time settings:

| Option | Code default | Can it be omitted while preserving the example's settings? |
| --- | --- | --- |
| `--dir genesis.json` | `./genesis.json` | Yes; same output location |
| `--chain-id 1982` | `1982` | Yes |
| `--name innbc-chain` | `innbc-chain` | Yes |
| `--validator-registry-minimum-stake 1` | `1` wei | Yes |
| `--validator-registry-proposal-duration-preset standard` | `standard` | Yes |
| `--validators "…"` | Empty explicit validator list | Keep it to specify the example's validator |
| Both `--premine address:amount` entries | Empty premine list | Keep them to fund the specified addresses |
| `--epoch-size` | `30` blocks | Yes; omission selects the default |
| `--block-time` | `2s` | Yes; omission selects the default |

Additional genesis defaults used when their options are omitted are:

| Option | Default |
| --- | --- |
| `--consensus` | `innbcbft` |
| `--block-gas-limit` | `5242880` |
| `--base-fee-config` | `1:2:8`: initial base fee of 1 wei, elasticity multiplier 2, change denominator 8 |
| `--sprint-size` | `5` blocks |
| `--block-time` | `2s`; positive whole-second chain-clock increment | <!-- ALPHA CHANGE 3324 -->
| `--block-tracker-poll-interval` | `1s` |
| `--native-token-config` | Empty argument resolves to `INNBC Chain:INNBC:18` |
| `--min-validator-count` | `1` |
| `--max-validator-count` | `9007199254740990`, the current `common.MaxSafeJSInt` value |
| `--validators-path` | `./`; used when explicit validators are omitted |
| `--validators-prefix` | `test-chain-`; used when explicit validators are omitted |
| `--trieroot` | Empty; no external initial trie root supplied |
| Contract-deployer and transaction allow/block list flags | Empty lists |

The genesis minimum stake is exactly **1 wei**, not one INNBC. The generator enforces this initial value. Registry governance may change the live minimum later.

<!-- START ALPHA CHANGE 3547 -- document positive chain ID validation within genesis configuration policy -->

#### Positive Chain-ID Validation

Supported chain IDs are **1–9223372036854775807**. The production default remains **1982**, including when `--chain-id` and its legacy `--chainid` alias are omitted.

The genesis command validates this range before generation. Both direct genesis builders reuse the same command validator, preventing zero or oversized values from bypassing flag validation.

Configuration import and direct node startup reuse `Chain.ValidateChainID`, which rejects zero and negative IDs. A supplied configuration with an omitted or explicitly null `chainID` also fails validation instead of receiving a replacement value.

This follows the existing configuration-boundary approach used for unsupported block durations and epoch sizes below two. Invalid configuration returns an ordinary error before runtime initialization. Archive, database-backup and coordinated-recovery workflows retain their existing positive-ID restrictions.

Regression coverage includes the minimum and maximum supported IDs, zero and negative rejection, unsigned overflow, both genesis flag spellings, both genesis builders, omitted/null imported IDs, and actual CLI failure statuses in text and JSON modes. Direct startup coverage checks rejection before fresh-directory creation and preservation of an existing directory.

These checks run during configuration loading and initialization. They add no recurring block-processing work and require no Solidity or artifact changes.

<!-- FINISH ALPHA CHANGE 3547 -- document positive chain ID validation within genesis configuration policy -->

There is no built-in default for the particular validator identity or the two premine allocations below. If explicit validators are omitted, the generator attempts to load validator identities from local key directories. A generic premine amount exists for an address supplied without an amount, but it does not automatically select or fund either example address.

#### Minimal Local Genesis Command

Run the following command from the repository root. It creates `./genesis.json`; the generator requires that the output file does not already exist.

```bash
go run . genesis \
  --validators "/ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAm22et517T1XhG181xGUDikDxUFrek4CuYTgUJjC38vxZR:0x6eE8A7Dc1625B46e6474a69b504825858315620c:0x1ec6560f8a2565ab59699a0747dcf4086977bb397d58aa38bbe4f9a8cc2ac1ee1e7d46070bf50755886ecc54f1e9b694da37d2a653f17ee79e879c3c467327e21cd5dc407d06be70c6e763fcb5a1697868163fd13f1a865f777fe2457bf09e510bd83fe7f762b9772b718ae2c124f4244e1eecf90a934f449ac61e855266c9a7:0x13bcb08037ae0425b4735fdf35ed05236add52f109a44fc1436cd06ac0328fb2088175e5892bdee6f1ff0826959625d318771de6e04977f1728ea29821ab0c7d" \
  --premine 0x0000000000000000000000000000000000000108:1000000000000000000000000000 \
  --premine 0x6eE8A7Dc1625B46e6474a69b504825858315620c:1000000000000000000000000
```

This example uses an existing validator identity and a loopback address. Operating that validator requires the matching private keys. Operators using different identities must supply their own peer ID, operator address, BLS public key and proof.

The BLS proof of possession is public information, bound to the operator address and BLS public key under a dedicated proof domain. Publishing it does not disclose the private key or enable creation of new consensus signatures.

The generator writes the resolved defaults and explicit arguments into the genesis file. Nodes use that generated configuration. Changing a generation default later does not modify an existing genesis file.

#### Supported Production Defaults

The current production profile uses:

| Setting | Value |
| --- | --- |
| Genesis epoch length | 30 blocks |
| Genesis block interval | 2 seconds |
| Recent-node retention | 43,200 blocks |
| Recent-node segment length | 1,800 blocks |

At the intended block cadence, an epoch lasts approximately one minute and nominal recent-history retention corresponds to 24 hours. Consensus delays can extend these elapsed times. Segment retirement occurs at segment boundaries, so the available history window varies with the current position within a segment.

These defaults provide sufficient retained headers for reconstructing the current epoch after a recent node restarts.

For the current production profile, retain the documented defaults. Custom settings are intended for isolated local testing and debugging of specific conditions. Any proposed production override requires separate compatibility review.

In particular, increasing `--epoch-size` or reducing recent-node retention can create an incompatible combination. For example, a 60,000-block epoch with 43,200-block retention can discard the beginning of the current epoch before that epoch ends.

The running node may retain the epoch information in memory. After restart, consensus reconstructs the epoch's beginning by walking backward through headers, including the preceding epoch boundary. If those headers have been retired, initialization fails. A snapshot whose retained range omits the required headers can encounter the same problem.

Not every override causes incompatibility. The requirement is that the actual retained header window, accounting for segment boundaries, covers epoch reconstruction. Retaining the supported defaults satisfies this requirement.

<!-- START ALPHA CHANGE 3829 -- relate retention guidance to supported interval upgrades -->

Block time must remain a whole number of seconds and at least `1s`. Changing the interval of an established chain requires the protocol-upgrade work described in point 52. Such a future change also alters the nominal elapsed time represented by block-count retention settings, which must be reviewed before activation.

<!-- FINISH ALPHA CHANGE 3829 -- relate retention guidance to supported interval upgrades -->

#### Accepted Configuration Constraint

The reviewed recent-history epoch-reconstruction issue is closed by acceptance of this documented configuration constraint for the supported default profile.

Custom configurations remain capable of creating an insufficient history window. This epoch-retention resolution adds documentation without adding runtime checks, persisted epoch-start metadata, historical queries or EVM execution.

<!-- START ALPHA CHANGE 3344 -- document native-token configuration validation and closure within point 80 -->

#### Native-Token Configuration Presence Validation

INNBCBFT initialization requires a present, non-null `nativeTokenConfig` section. Previously, a supplied genesis configuration could omit this section or explicitly set it to `null`, then reach native-token initialization and panic when its metadata was dereferenced.

The shared `ValidateNativeTokenConfig` validator now rejects the missing section with an ordinary error. `GetINNBCBFTConfig` applies this check alongside the existing registry minimum-stake and chain-clock validation. Server startup and the genesis initialization hook already use that configuration-loading boundary.

The direct consensus `Factory` applies the same validator and cancels its initialization context before returning this validation error.

Server startup reports:

```text
invalid INNBCBFT configuration: nativeTokenConfig is required and must not be null
```

Genesis-command defaults remain unchanged. Omitting `--native-token-config` causes the generator to write the resolved `INNBC Chain:INNBC:18` configuration. Importing a genesis file with the required section missing or null returns an error rather than silently supplying defaults.

This validator checks section presence only. It does not introduce new metadata restrictions, reject a present empty object, reject zero decimals, or modify supplied metadata. Existing command-level metadata validation remains unchanged.

##### Native-Token Configuration Regression Coverage and Closure

The regression coverage includes:

- `TestNativeTokenConfig_ConfigurationBoundaries` in `consensus/innbcbft/genesis_stake_test.go`: missing/null sections, present configurations, decoding boundaries, direct consensus construction and rejection before genesis-hook execution.
- `TestNativeTokenConfig_NewServer` in `server/genesis_stake_test.go`: controlled startup rejection without a panic or creation of runtime data and log files.
- `TestNativeTokenConfig_CLI` in `command/root/config_sections_test.go`: generated defaults and actual server-command failures for missing/null sections in plain and JSON modes, with exit status 1 and unchanged input files.

The focused tests passed, including 20 repetitions with the race detector. The broader consensus, server, command, snapshot-builder, chain and fork-manager tests also passed, and the corresponding `go vet` command completed without diagnostics.

The reported missing-native-token initialization panic, issue 3, is closed within this scope. Validation runs during configuration loading and initialization; it adds no recurring work to block execution or transaction processing.

<!-- FINISH ALPHA CHANGE 3344 -- document native-token configuration validation and closure within point 80 -->

<!-- START ALPHA CHANGE 3444 -- document shared epoch-size validation during initialization -->

#### Epoch-Size Validation During Configuration Loading and Startup

INNBC requires an epoch size of at least two blocks. The genesis CLI already rejects smaller values.

The shared `ValidateEpochSize` validator also enforces this requirement in `GetINNBCBFTConfig` and direct consensus construction. Existing callers consequently receive the check during server startup, genesis-hook initialization and peer-directory configuration loading.

Missing, null, zero and one-block epoch settings are rejected without substituting a default. Server startup reports:

`invalid INNBCBFT configuration: epochSize must be at least 2`

Server validation occurs before logger creation, database opening or snapshot activation, including when an existing data directory is supplied.

This prevents a one-block configuration from reaching an epoch commitment whose start and end heights are equal, which ValidatorSet rejects.

The production default remains 30 blocks. This minimum-size check does not validate compatibility between custom epoch lengths and recent-history retention; the separate accepted configuration constraint above still applies.

<!-- START ALPHA CHANGE 3449 -- document the additional contract epoch-size safeguard -->

Shared Go validation adds configuration-time comparisons without additional per-block checks, contract calls or transaction execution. That correction closed the reported startup-validation issue without requiring Solidity changes.

An additional safeguard makes `ValidatorSet.initialize(uint256)` reject epoch sizes below two with `INVALID_INPUT`, matching the Go configuration boundary. This comparison executes only during initialization.

The initializer's ABI and storage layout remain unchanged. Its compiled bytecode changes, requiring regeneration of Solidity artifacts, TypeChain outputs and embedded Go artifacts and bindings. Generate the launch genesis from the updated outputs after the remaining pre-launch edits.

<!-- FINISH ALPHA CHANGE 3449 -- document the additional contract epoch-size safeguard -->

Regression coverage exercises configuration loading, direct consensus construction, rejection before genesis-hook execution, server startup without filesystem side effects, preservation of existing files, and actual genesis/server CLI exit behavior in text and JSON modes.

<!-- START ALPHA CHANGE 3445 -- record verified epoch-size validation closure -->

##### Validation and Closure

The focused epoch-size, minimum-stake, native-token, factory and genesis-hook regressions passed, including twenty repetitions with the race detector. Broader command, consensus, server, snapshot-builder, chain and fork-manager tests passed. The corresponding `go vet` checks completed without diagnostics.

The reported one-block epoch configuration issue is closed by the shared initialization validation, source review and passing regression results.

The existing-directory regression verifies preservation of marker files and rejection before runtime initialization; it does not exercise reopening a populated blockchain database.

The separate accepted constraint concerning custom epoch lengths and recent-history retention remains unchanged.

<!-- FINISH ALPHA CHANGE 3445 -- record verified epoch-size validation closure -->

<!-- FINISH ALPHA CHANGE 3444 -- document shared epoch-size validation during initialization -->

<!-- START ALPHA CHANGE 3111 -- document genesis allocation validation and safe omitted-balance initialization -->

#### Genesis Allocation Validation

Genesis allocation validation now rejects malformed account entries before they can cause an initialization panic.

Previously, an allocation such as `"address": null` could pass import validation and reach `WriteGenesis`, which dereferenced the missing account. A nonzero-address account with an omitted or null balance could instead reach the INNBC genesis hook, where initial supply calculation passed that missing balance to `big.Int.Add`.

The ordinary genesis generator already creates account objects with explicit balances. These failures principally concerned imported or manually edited genesis files and allocation maps constructed directly by callers.

<!-- START ALPHA CHANGE 3128 -- document aggregate genesis supply validation within the genesis allocation policy -->

The allocation policy is:

| Allocation input | Behavior |
| --- | --- |
| Null account entry, including at the zero address | Rejected with an error identifying the address |
| Empty account object | Accepted without a balance allocation |
| Account containing code, nonce or storage but no balance | Accepted; supplied fields retain their existing initialization behavior |
| Omitted balance or `"balance": null` | No balance allocation; zero contribution from that entry to initial supply |
| Explicit zero balance | Accepted; existing explicit-zero initialization behavior is preserved |
| Explicit positive balance through 2^256 - 1 | Individually valid; the complete allocation must also pass aggregate supply validation |
| Negative balance or balance above 2^256 - 1 | Rejected with an error identifying the address |
| Sum of explicit balances at nonzero addresses through 2^256 - 1 | Accepted, including the exact maximum |
| Sum of explicit balances at nonzero addresses above 2^256 - 1 | Rejected with a controlled aggregate supply error |

A null account is therefore distinct from a valid account whose balance is absent.

Validation does not replace absent balances with zero-valued objects or mutate the allocation map. When `WriteGenesis` starts from an existing state root, an omitted balance retains the existing balance because no balance addition is performed.

The initial native-token supply calculation skips absent balances and continues excluding the zero address. The zero-address balance must still satisfy the individual nonnegative uint256 balance requirement.

#### Aggregate Initial Native-Token Supply Validation

The native-token contract stores its supply as `uint256`. Its maximum representation is 2^256 - 1 wei, approximately 1.16 × 10^59 INNBC with the configured 18 decimal places. The documented one-billion-INNBC allocation is far below this boundary.

Previously, individually valid balances could produce an oversized combined supply. For example, allocating 2^256 - 1 wei to one nonzero address and 1 wei to another produced a total of 2^256 wei.

Go's `big.Int` represented that total correctly, but the pinned ABI encoder truncated it to 256 bits. The contract could therefore initialize with zero supply while the individual native balances remained allocated. Supply-dependent operations such as burns could subsequently fail.

The shared `chain.ValidateGenesisAllocations` validator now sums explicit balances at all nonzero addresses and rejects the configuration as soon as the combined total exceeds 256 bits. This includes balances allocated to system-contract addresses.

Validation examines the final allocation map. Duplicate `--premine` entries retain the builders' existing replacement behavior; overwritten entries are not incorrectly counted twice.

The rejection message is:

`genesis initial native-token supply exceeds unsigned 256-bit range`

The existing validation callers apply this check before genesis file publication, during configuration import and startup, before direct genesis state access, and before INNBC genesis contract initialization. No separate command-only check, ABI dependency change or Solidity regeneration is required.

#### Aggregate Supply Regression Coverage and Closure

The additional regression files are:

- `chain/genesis_supply_test.go`
- `command/genesis/genesis_supply_test.go`
- `command/helper/genesis_supply_test.go`
- `server/genesis_supply_test.go`
- `state/genesis_supply_test.go`
- `consensus/innbcbft/genesis_supply_test.go`

Coverage includes zero supply, a single maximum balance, a split exact-maximum total, maximum-plus-one rejection, several individually valid balances exceeding the aggregate limit, omitted balances and zero-address exclusion.

Both genesis builders are exercised through premine parsing and generation. Tests verify rejection before creating the output file, acceptance of the exact maximum, and validation of effective allocations after duplicate-address replacement.

Further regressions establish preservation of existing output files and allocation input, startup rejection before data or log creation, executor rejection before snapshot or hook access, and independent hook rejection before contract execution.

An actual genesis execution regression commits state to memory-backed trie storage, reopens the resulting root, verifies allocated balances and reads the native token's `totalSupply` through the EVM. The returned supply must equal the exact uint256 maximum without truncation.

Supplied validation passed focused aggregate-supply and existing allocation regressions, twenty race-enabled repetitions, broader tests covering chain, genesis commands, state, INNBCBFT consensus, server and snapshot packages, and vet over those packages.

The reviewed aggregate genesis supply truncation issue is closed by the applied correction, source review and passing regression results.

This extends validation of configured genesis allocations. It does not add checks to ordinary transaction execution or block production, change token decimals, or introduce a lower supply cap. The correction adds local integer summation without historical reads or additional EVM execution.

<!-- FINISH ALPHA CHANGE 3128 -- document aggregate genesis supply validation within the genesis allocation policy -->

#### Validation Boundaries

The shared allocation validator is applied at these boundaries:

- `command/helper.WriteGenesisConfigToDisk` validates before serialization or file creation. Both genesis builders use this writer.
- Imported-chain validation rejects malformed allocations before returning the configuration.
- `server.NewServer` validates before logger creation, database opening or network startup, including configurations supplied directly in Go.
- `state.Executor.WriteGenesis` validates before opening a snapshot or applying allocations.
- `GenesisPostHookFactory` validates before contract initialization, protecting independent hook callers.

Invalid allocations produce controlled configuration errors. They are not classified as local storage failures.

These checks run during generation, import, startup and explicit genesis reconstruction, including trie rebuilding. They add no validation to ordinary transaction execution, block production or consensus processing. The correction adds no historical reads or additional EVM execution.

#### Allocation Regression Coverage and Closure

The regression files are:

- `chain/genesis_allocations_test.go`
- `command/helper/genesis_allocations_test.go`
- `server/genesis_allocations_test.go`
- `state/genesis_allocations_test.go`
- `consensus/innbcbft/genesis_allocations_test.go`

Coverage includes null accounts, omitted/null balances, explicit zero, supported full-width balances, negative balances and overflow. Tests verify rejection before file publication, startup side effects, snapshot opening or hook execution, and confirm that allocation input remains unchanged.

State regressions reopen committed roots to verify retained balances and initialized code, nonce and storage. They also compare fresh-state roots for a code-bearing account with an omitted balance versus an explicit zero balance.

The INNBC regression executes actual genesis initialization and reads the native token's `totalSupply` through the EVM. It verifies omitted/null balances, explicit zero, a balance above MaxUint64 and the continued zero-address exclusion. These execution regressions use memory-backed trie storage.

Tests were reported passing after application. Source review confirmed all implementation changes and regression files were present and correctly connected.

The reviewed null-account and omitted-balance genesis initialization panic issue is closed by the applied correction and reviewed regression coverage.

<!-- FINISH ALPHA CHANGE 3111 -- document genesis allocation validation and safe omitted-balance initialization -->

<!-- START ALPHA CHANGE 3137 -- document strict premine addresses and unique decoded genesis allocation keys -->

#### Premine Address and Imported Allocation Key Validation

Genesis generation now validates premine addresses before converting them. Genesis import applies the same address validation to allocation keys.

Previously, these inputs used the permissive `StringToAddress` conversion without validation. Invalid hexadecimal input could become an unintended address, short input could be padded, and oversized input could be truncated. For example, `--premine 0xzz:1000000000000000000` could produce an allocation at the zero address instead of rejecting the malformed address.

The accepted address format is:

| Address input | Behavior |
| --- | --- |
| Exactly 40 hexadecimal digits with a lowercase `0x` prefix | Accepted |
| Exactly 40 hexadecimal digits without a prefix | Accepted |
| Lowercase, uppercase or mixed-case hexadecimal digits | Accepted; no checksum requirement |
| Full-length zero address | Accepted as an intentional address |
| Empty, short, oversized or invalid hexadecimal input | Rejected |
| Uppercase `0X` prefix or surrounding whitespace | Rejected |

Premine validation runs before conversion for both address-only and address-plus-amount entries. Errors identify the quoted address and preserve the underlying validation error without including an optional key field. The genesis command reports the failure as an invalid premine entry.

Valid amount parsing, address-only default funding and optional key handling retain their existing behavior. The shared `StringToAddress` conversion remains unchanged; strict validation is applied at these configuration-input boundaries.

#### Unique Decoded Allocation Addresses

Genesis import also rejects distinct allocation keys that decode to the same address. Examples include prefixed versus prefixless spelling, or different capitalization of hexadecimal digits.

Previously, conversion of these distinct keys could overwrite an allocation according to Go map iteration order. Import now returns a controlled error instead. This applies to all decoded addresses, including the zero address, and detects a duplicate even when one account entry is null.

The converted allocation map is published only after its address checks succeed. A failed address-validation pass therefore does not replace an existing allocation map with partially converted entries.

This check concerns distinct key spellings that decode to one address. It does not add detection of repeated identical JSON object-key tokens.

Repeated valid CLI premine entries retain their existing replacement behavior: the later entry for an address determines its final allocation. Existing account, individual-balance and aggregate-supply validation still applies to the final allocation map.

#### Address Regression Coverage and Closure

The address regression files are:

- `command/helper/premine_address_test.go`
- `command/genesis/premine_address_test.go`
- `chain/genesis_address_test.go`

The existing malformed-amount fixture in `command/genesis/params_test.go` now uses a valid full-length address so it continues testing amount validation.

Coverage verifies accepted address formats, malformed and partially decodable inputs, wrapped errors, exclusion of optional keys from errors, unchanged amount/key handling, and preservation of CLI replacement behavior.

Command regressions verify rejection before genesis publication, preserving an existing output file and avoiding creation of a new one. Import regressions verify malformed and duplicate decoded keys are rejected, source files remain unchanged, and a failed conversion preserves the existing allocation map. Repeated duplicate-address cases exercise independence from map iteration order.

Supplied validation passed focused address, premine, allocation and aggregate-supply regressions, twenty race-enabled repetitions, and broader tests covering genesis commands, chain, state, INNBCBFT consensus, server and snapshot packages.

The reviewed malformed premine-address issue is closed by the applied correction, source review and passing regression results. The related imported allocation-key conversion and duplicate decoded-address cases are also covered.

These checks run during configuration parsing and genesis generation/import. They add no recurring address validation to ordinary transaction execution, block production or consensus processing, and require no historical reads or additional EVM execution.

<!-- FINISH ALPHA CHANGE 3137 -- document strict premine addresses and unique decoded genesis allocation keys -->

<!-- START ALPHA CHANGE 3173 -- document startup storage validation before genesis initialization -->

#### Startup Storage Validation Before Genesis Initialization

Startup now distinguishes genuinely empty blockchain storage from unreadable or inconsistent existing storage before permitting genesis initialization.

Previously, the head-hash reader represented both missing records and backend read failures as an unsuccessful lookup. Startup could interpret that result as empty storage and write the genesis block into an existing blockchain database. That write also replaced the persisted head hash and head number with block-zero metadata, while later canonical records could remain.

This concerns initialization of the blockchain database. It does not rewrite `genesis.json`.

The startup policy is:

| Storage condition | Behavior |
| --- | --- |
| Backend confirms there are no stored blockchain key/value records | Initialize the genesis block |
| Existing storage has valid genesis and head metadata matching the configured genesis | Load the recorded head without rewriting genesis |
| Backend inspection or metadata reading returns an error | Reject startup and preserve the error cause |
| Required metadata is missing, malformed or inconsistent | Reject startup without genesis or head-metadata rewriting |
| Records remain but their identifying metadata is absent | Reject startup; do not treat the storage as empty |

Emptiness is established from the actual stored records. Memory storage checks its record map; MDBX checks the database entry count in a read transaction. A missing head-hash record alone no longer establishes emptiness.

Existing storage is inspected using error-preserving reads. Hash records must contain exactly 32 bytes, and the head-number record must contain exactly eight bytes. The canonical mapping at the recorded head number must agree with the recorded head hash. The referenced head header must decode successfully and match its stored hash and recorded number.

The permanent genesis mapping and referenced genesis header are also validated. The stored genesis hash must match the genesis configured for the node.

Recent-history storage validates the permanent genesis database and each retained segment before selecting its highest validated head. Backend-confirmed empty segments can be skipped. An unreadable or inconsistent segment produces an error rather than causing fallback to an older segment. Nonempty recent-history segments require valid permanent genesis storage.

Valid pruned history remains supported: startup does not require already retired historical segments to reappear.

The genesis initialization decision runs under the blockchain write lock. These checks run during startup and use metadata reads and record-count inspection; they do not scan every historical block or add recurring checks to transaction execution or block production.

This is validation rather than automatic repair. Rejected storage is not automatically rewound or repaired. The checks cover the genesis/head metadata needed for this startup decision; they are not a complete integrity scan of historical blocks, trie contents or consensus storage.

#### Startup Storage Regression Coverage and Closure

Regression coverage is provided by:

- `blockchain/startup_metadata_test.go`
- `blockchain/storage/segmented/startup_test.go`

The tests cover missing, malformed and inconsistent metadata; missing or corrupt referenced headers; preserved backend error causes; orphan records; fresh initialization; valid existing storage; configured-genesis mismatch; and recent storage with pruned history.

Rejected startup is checked for zero genesis batch writes and unchanged stored records in the injected backend. Real memory, MDBX and segmented-storage regressions also verify rejection after metadata removal, including database reopening where applicable. Segmented regressions confirm that a read failure does not fall back to an older head.

Supplied validation passed the focused regressions, twenty race-enabled repetitions, broader blockchain, storage, server, sync, archive, consensus and snapshot tests, and `go vet`.

The reported missing/unreadable-head-metadata genesis-rewriting issue is closed by the applied correction, source review and passing regression results.

<!-- FINISH ALPHA CHANGE 3173 -- document startup storage validation before genesis initialization -->

<!-- START ALPHA CHANGE 3191 -- document strict local storage inspection before snapshot selection -->

#### Local Storage Validation Before Snapshot Selection

Snapshot selection now validates existing local blockchain storage before deciding whether a configured snapshot may replace it.

This extends the startup protection described above to the earlier snapshot-selection stage. Previously, snapshot selection used `ReadHeadNumber`, which could represent missing, malformed or unreadable metadata as an unsuccessful lookup. Recent-history storage could then select an older segment's head instead of reporting the damaged latest segment.

For example, local history reaching block 50,000 could appear to reach only block 48,600 when its latest segment's head-number metadata was unavailable. A snapshot at block 49,000 could consequently appear newer and replace the existing chain data before later startup validation examined it.

Both full-history and recent-history inspection now use `ReadStartupState`. The local comparison height comes from the validated head header. Nonempty storage must also provide valid genesis metadata. Recent storage validates the permanent genesis database and each retained segment; unreadable or inconsistent metadata cannot cause fallback to an older segment.

<!-- START ALPHA CHANGE 3215 -- document local genesis identity requirements during snapshot selection -->

The selection policy is:

| Local storage condition | Behavior |
| --- | --- |
| Confirmed empty storage or a genuinely fresh data directory | Preserve the existing fresh-bootstrap policy and trusted-source model |
| Valid genesis-only storage | Retain its validated genesis identity; any candidate bootstrap must match it |
| Valid recent history above genesis | Require matching genesis identity before comparing heights; equal or older snapshots are skipped |
| Valid full history above genesis | Preserve the local chain; checkpoint bootstrap is not attempted |
| Candidate genesis hash differs from the validated local genesis hash | Skip optional bootstrap or fail required bootstrap before staging; preserve local chain data |
| Missing, malformed, inconsistent or unreadable required metadata | Stop startup before selecting or staging a replacement snapshot |
| Blockchain storage is absent or empty while trie or consensus files remain | Reject the incomplete local layout instead of treating it as fresh |

<!-- FINISH ALPHA CHANGE 3215 -- document local genesis identity requirements during snapshot selection -->

Local inspection failures stop startup even when snapshot bootstrap is optional. Optional-snapshot fallback does not override a failure to validate existing local storage. Inspection-close failures also propagate to the startup caller.

#### Read-Only Inspection and Recovery Boundaries

Existing databases are opened read-only using their stored MDBX geometry. The segmented inspection opener does not create missing databases or layout components, apply retention pruning, or delete retired segment directories. Handles opened before a construction failure are closed, and inspection handles are closed before returning the local-head classification.

Read-only MDBX access may still update reader-lock bookkeeping. Database contents and geometry are preserved.

Interrupted-activation recovery continues running before local inspection. That recovery protocol may restore a backup or remove stale activation files. The read-only guarantee applies to local storage inspection, not to the entire startup sequence.

This correction validates startup metadata rather than repairing damaged storage or scanning all historical blocks. Deliberate recovery remains an operator decision. Inspection adds startup work across retained segments, with no recurring checks in transaction execution, block production or consensus processing.

#### Snapshot Selection Regression Coverage and Closure

Coverage is provided by:

- `blockchain/storage/segmented/read_only_test.go`
- `server/snapshot_startup_validation_test.go`
- The local-head fixture in `server/snapshot_test.go`

The activation regressions use actual local history at block 6 and a valid snapshot at block 5. With damaged latest-segment head-number metadata, the legacy reader reports block 4. The corrected activation path rejects startup and preserves the original blockchain, trie and consensus data, with both optional and required snapshot settings.

Additional coverage checks full-history storage, missing genesis metadata, propagated read and close errors, incomplete layouts, incompatible segment geometry, cleanup after partial opening, interrupted-activation recovery, and unchanged healthy bootstrap behavior. Source-preservation comparisons exclude MDBX reader-lock files.

Supplied validation passed the focused regressions, twenty race-enabled repetitions of the new regressions, and complete segmented-storage and server package tests. The previously failing local-head fixture now supplies valid INNBCBFT extra data and asserts distinct, nonzero genesis and block-one hashes.

The reported snapshot-selection bypass and unintended local rollback issue is closed within this scope.

<!-- FINISH ALPHA CHANGE 3191 -- document strict local storage inspection before snapshot selection -->

<!-- START ALPHA CHANGE 3215 -- document preservation of existing local genesis identity during snapshot replacement -->

#### Existing Local Genesis Identity During Snapshot Replacement

Snapshot selection now retains the genesis hash already validated by `ReadStartupState` and requires a candidate snapshot to match it before staging or replacing local chain data.

Previously, local inspection returned the validated head height but discarded the genesis identity. Candidate compatibility checks used the snapshot's own genesis state root together with the configured non-state genesis header fields. A snapshot from different allocations could therefore pass those checks and replace an initialized local chain if its checkpoint appeared newer.

For example, a recent node with valid local history at block 4 could accept a structurally valid snapshot at block 5 whose genesis allocations produced a different genesis hash. The corrected selection path detects the identity mismatch before copying the artifact into activation staging.

Both recent-history and full-history inspection retain the validated local genesis identity. Genesis-only storage also supplies an identity even though it is not classified as usable history above genesis. Genuinely fresh and confirmed-empty storage return no local identity and retain the existing trusted-source bootstrap policy.

Mismatch handling follows the existing effective bootstrap policy:

- Optional bootstrap logs the mismatch, skips the candidate and continues with local storage.
- Required bootstrap returns an error without replacing local storage.
- Recent nodes with usable local history continue treating snapshot bootstrap as optional, including when the configured bootstrap-required flag is set.
- Full-history nodes with usable local history continue skipping checkpoint bootstrap.

Existing staged-artifact validation checks the artifact's actual persisted genesis commitment. Equality between the staged and previously inspected manifests prevents a changed staged manifest from bypassing the local identity comparison.

The correction reuses the existing startup inspection result. It adds no database reads, genesis allocation replay, historical block scan or recurring transaction, EVM or consensus checks. Interrupted-activation recovery retains its existing behavior and still runs before local inspection.

#### Local Genesis Identity Regression Coverage and Closure

Coverage is provided by:

- `server/snapshot_genesis_identity_test.go`
- Updated inspection and healthy-bootstrap regressions in `server/snapshot_startup_validation_test.go`
- Updated genesis-only and advancing-head assertions in `server/snapshot_test.go`

The new regression constructs two real allocation-derived genesis roots with identical non-state header fields. It uses a valid candidate snapshot at block 5 and checks absent storage, empty databases, genesis-only storage and local history at block 4 across recent and full modes with both bootstrap-required settings.

Rejected or skipped candidates preserve local blockchain, trie and consensus files, the local head and genesis identity, and the source snapshot. Preservation comparisons exclude MDBX reader-lock bookkeeping. Existing regressions continue covering matching-chain activation and healthy bootstrap behavior.

Reported tests passed, and read-only source review confirmed the applied implementation. The existing-local-genesis snapshot replacement defect is closed within this scope. Fresh bootstrap continues relying on the documented trusted internal snapshot source.

<!-- FINISH ALPHA CHANGE 3215 -- document preservation of existing local genesis identity during snapshot replacement -->

<!-- START ALPHA CHANGE 3181 -- document strict genesis allow-list and block-list address validation -->

#### Genesis Allow-List and Block-List Address Validation

Genesis generation now validates every supplied administrator and enabled address for deployment and transaction allow/block lists before converting the strings into addresses.

Previously, these flags used the permissive `StringToAddress` conversion without validating the original input. Malformed strings could be padded, truncated or partially decoded into unintended addresses.

For example, `--contract-deployer-allow-list-admin 0xzz` could enable a deployment allowlist with the zero address as its administrator. The intended administrator would receive no role, and unlisted callers could be prevented from deploying contracts.

Malformed entries now produce a controlled error identifying the flag and quoted address. Validation preserves the underlying address-validation error.

All eight flags are covered:

| Access-control list | Administrator flag | Enabled-address flag |
| --- | --- | --- |
| Contract deployment allowlist | `--contract-deployer-allow-list-admin` | `--contract-deployer-allow-list-enabled` |
| Contract deployment blocklist | `--contract-deployer-block-list-admin` | `--contract-deployer-block-list-enabled` |
| Transaction allowlist | `--transactions-allow-list-admin` | `--transactions-allow-list-enabled` |
| Transaction blocklist | `--transactions-block-list-admin` | `--transactions-block-list-enabled` |

The accepted format follows the existing strict genesis address policy:

- Exactly 40 hexadecimal digits.
- An optional lowercase `0x` prefix.
- Lowercase, uppercase or mixed-case hexadecimal digits, without a checksum requirement.
- A correctly formatted zero address remains accepted.

Empty entries, short or oversized addresses, odd-length payloads, invalid hexadecimal characters, uppercase `0X` prefixes and surrounding whitespace are rejected.

Every supplied entry is validated, including enabled-address entries when no administrator is supplied.

#### Validation Boundaries and Preserved List Policy

Validation runs during genesis CLI pre-run and at the direct generic and INNBC genesis-builder entry points. Direct callers therefore cannot bypass the check by omitting CLI validation.

Rejection occurs before genesis construction or output-file publication. The validator does not mutate the supplied address lists.

Existing list behavior remains unchanged:

| Configuration | Behavior |
| --- | --- |
| Default empty lists | No access-control lists are enabled |
| Valid enabled entries without an administrator | The corresponding list remains inactive |
| At least one valid administrator | INNBC generation includes the corresponding list configuration |
| Repeated valid entries or equivalent address spellings | Existing ordering and duplicate entries are preserved |
| Explicit full-length zero address | Accepted under the existing policy |

The shared `StringToAddress` convenience conversion and existing list-construction logic remain unchanged. Strict validation is applied before these generator conversions.

These checks add no recurring work to ordinary transaction execution, block production or consensus processing. This correction validates generator inputs; it does not repair previously generated configurations or introduce a new imported-genesis access-list policy.

#### Access-List Regression Coverage and Closure

`command/genesis/access_list_validation_test.go` covers all eight flags, accepted formats, malformed entries, wrapped errors and preservation of input lists.

CLI regressions exercise actual flag parsing and pre-run validation. Direct-builder regressions verify rejection before other construction inputs are accessed. Both paths verify that rejected input creates no output file and leaves an existing output file unchanged.

Generation and import regressions confirm that default lists remain disabled, valid enabled-only lists remain inactive, and active INNBC lists preserve their administrator/enabled addresses, duplicate ordering and explicit zero entries.

Supplied validation passed focused access-list, premine-address, aggregate-supply and chain-ID regressions, twenty race-enabled repetitions, broader genesis, helper, chain, types, state, consensus and server tests, and `go vet`.

The reported malformed genesis allow/block-list address normalization issue is closed by the applied correction, source review and passing regression results.

<!-- FINISH ALPHA CHANGE 3181 -- document strict genesis allow-list and block-list address validation -->

<!-- START ALPHA CHANGE 3185 -- document strict optional external genesis trie-root parsing -->

#### External Genesis Trie-Root Validation

The optional `--trieroot` argument now uses strict parsing before genesis construction.

Previously, INNBC genesis generation converted this argument using the permissive `StringToHash` convenience function. Invalid characters could produce a zero or partially decoded hash, while short and oversized values could be padded or truncated.

For example, `--trieroot 0xzz` could become the zero hash. During fresh state initialization, that value selected the no-external-root path, allowing allocation-based genesis initialization instead of the external trie initialization requested by the operator.

Malformed input now produces a controlled error identifying `--trieroot` and its quoted value before genesis construction or output publication.

The input policy is:

| Trie-root input | Behavior |
| --- | --- |
| Omitted argument or empty string | Accepted; preserves the no-external-root default |
| Exactly 64 hexadecimal digits with lowercase `0x` prefix | Accepted |
| Exactly 64 hexadecimal digits without a prefix | Accepted |
| Uppercase or mixed-case hexadecimal digits | Accepted |
| Full-length all-zero hash | Accepted; preserves the existing no-external-root meaning |
| Empty prefixed payload such as `"0x"` | Rejected |
| Short values, including `"0x0"` and `"0x1"` | Rejected |
| Odd-length or oversized payload | Rejected |
| Invalid hexadecimal characters, including a valid prefix followed by invalid characters | Rejected |
| Uppercase `0X` prefix or surrounding whitespace | Rejected |

Nonempty input must decode to exactly 32 bytes. Parsing does not pad, truncate or partially accept the supplied root.

The INNBC builder writes the parsed hash directly into `InitialTrieRoot`; it no longer uses `StringToHash` for this field. The shared convenience conversion remains unchanged.

#### Validation Boundaries and Startup Behavior

Validation runs during genesis CLI pre-run and at the direct generic and INNBC builder entry points. Direct callers therefore receive the same rejection without relying on CLI validation.

The parser preserves the original input string and does not mutate an existing genesis configuration. Rejected input creates no output file and leaves an existing output file unchanged.

The empty default remains appropriate for ordinary genesis generation. A correctly formatted zero hash retains its existing sentinel meaning.

Generation validates syntax and width. It does not establish that a supplied nonzero root is available in a trie database. The existing server verification of a nonzero external root during state initialization remains unchanged.

This strict configuration-field policy is separate from the valid hash padding/truncation retained for general typed RPC decoding.

The correction adds generation-time parsing only. It introduces no recurring checks in transaction execution, block production or consensus, and does not add a new imported-genesis trie-root validation pass.

#### Trie-Root Regression Coverage and Closure

`command/genesis/trie_root_validation_test.go` covers accepted formats, exact-width enforcement, wrapped hexadecimal errors and preservation of input/configuration values.

CLI and direct-builder regressions reject malformed roots before other construction inputs are accessed. They verify that no output file is created and that existing output contents remain unchanged.

Generation/import regressions verify the omitted and explicit-empty defaults, full-length zero hashes, prefixed and prefixless nonzero roots, mixed-case digits and roots containing leading zeros. The imported `InitialTrieRoot` must match the expected hash exactly.

Supplied validation passed focused trie-root, access-list, premine-address, aggregate-supply and chain-ID regressions, twenty race-enabled repetitions, and `go vet` across genesis, helper, chain, types, state, consensus and server packages.

The reported malformed external trie-root argument normalization issue is closed by the applied correction, source review and passing regression results.

<!-- FINISH ALPHA CHANGE 3185 -- document strict optional external genesis trie-root parsing -->

<!-- START ALPHA CHANGE 3202 -- document safe legacy regenesis root lookup and block-selector validation -->

#### Legacy Regenesis State-Root Lookup Validation

The registered `regenesis getroot` command now handles unavailable blocks and unsupported block selectors with controlled errors.

This command belongs to the legacy regenesis operator workflow. It queries a remote node and prints a block's state root. It does not generate `genesis.json`, initialize local chain databases or run during ordinary node startup.

Previously, the command checked the RPC error but assumed the returned block was nonnil. The pinned client returns a nil block without an error when a successful RPC response contains `result: null`. Accessing that block's state root then caused a nil-pointer panic.

For example, requesting block 101 from a remote node whose head is block 100 can return null. The command now reports `block unavailable: 0x65` before accessing block fields or printing a state root.

Unavailable historical data receives the same handling when the remote endpoint returns null. Pruning alone does not imply a null response when the remote node can still return a retained header.

#### Supported Legacy Command Block Selectors

The pinned client's block-number formatter also panics for unsupported negative values. The command now validates `--block` before creating the client or making an RPC request.

For this command, the supported selectors are:

| `--block` input | Behavior |
| --- | --- |
| Omitted | Preserve the existing latest-block default |
| Nonnegative number representable by the client's platform `int` type | Query that numeric block |
| `-1` | Query `latest` |
| `-2` | Query `earliest` |
| `-3` | Query `pending` |
| Any other negative value, including `-4` | Reject before client creation or RPC work |
| Positive value exceeding the client's platform `int` maximum | Reject before conversion |

These numeric tag aliases are those of the pinned `ethgo` client used by this command.

On the current 64-bit platform, the nonnegative client range extends through `MaxInt64`. The representability check also prevents narrowing an accepted CLI `int64` value into an unsupported client selector on a 32-bit platform.

A representable numeric selector does not establish that the remote block exists. A null response remains a controlled unavailable-block error.

#### Output and Resource Handling

Existing RPC failures retain their underlying error cause and receive block-selector context.

Only a nil block is classified as unavailable by this check. A returned block containing a zero-valued state root retains its existing output behavior.

Valid responses preserve the output format:

`state root <root> for block <number>`

Output-write errors and short writes are reported as failures. The RPC client is closed when the lookup helper returns, before deferred command output can invoke the normal CLI formatter's error-exit behavior.

Flag values are local to each command instance. The public command retains its existing output formatter; an internal constructor permits tests to inject a formatter that records errors without terminating the test process.

These checks apply only when the diagnostic is explicitly invoked. They add no recurring work to transaction execution, block production or consensus, and do not modify chain storage.

#### Getroot Regression Coverage and Closure

`command/regenesis/get_root_test.go` exercises actual flag parsing and the pinned RPC client against a loopback HTTP server.

Coverage includes:

- The latest default, numeric selectors and all three supported tag aliases.
- The client integer maximum and rejection of unsupported negative values.
- Actual `result: null` responses for latest and future-block requests.
- RPC errors with preserved error types and codes.
- Valid block output, including a returned zero-valued state root.
- Output-write failures and short writes.
- Exactly one output finalization per command invocation.
- Zero RPC requests and no state-root output for rejected selectors.

Tests use an injected output formatter rather than exercising process termination in a subprocess. Additional integer-range cases run conditionally on 32-bit platforms; the supplied 64-bit results do not constitute a 32-bit execution test.

Supplied validation passed focused regressions, twenty race-enabled repetitions, the regenesis package tests with root/command package compilation, and `go vet` across those packages.

<!-- START ALPHA CHANGE 3264 -- preserve getroot closure while recording history retirement -->
The reported unavailable-block dereference issue and the additional unsupported-negative-selector panic are closed by the applied correction, source review and passing regression results. `regenesis getroot` remains available; the separate `regenesis history` diagnostic has been retired as documented in point 84.
<!-- FINISH ALPHA CHANGE 3264 -- preserve getroot closure while recording history retirement -->

<!-- FINISH ALPHA CHANGE 3202 -- document safe legacy regenesis root lookup and block-selector validation -->

<!-- START ALPHA CHANGE 3218 -- document retirement of arbitrary-contract genesis predeployment and preservation of INNBC system-contract initialization -->

#### Retirement of the Legacy Arbitrary-Contract Predeploy Command

The legacy `genesis predeploy` command has been removed from the supported CLI, together with its exclusive helper package and constructor-argument parser.

The command previously loaded an external contract artifact, encoded constructor arguments, executed the constructor in a temporary in-memory EVM, and inserted the resulting account code and storage into an existing genesis configuration.

Its file-replacement implementation deleted the original genesis file before validating, serializing and writing the replacement. An interruption or subsequent failure could therefore leave the original configuration missing or its replacement incomplete.

The command was an explicitly invoked configuration-editing utility. Normal packaged installation and node startup did not invoke it. Retirement eliminates this particular deletion-before-publication path.

#### Supported INNBC System-Contract Genesis Setup

The customized INNBC genesis workflow already provides the system-contract setup required by this chain:

- `deployContracts` in `command/genesis/innbcbft_params.go` allocates the INNBC system-contract implementations and proxies using their compiled runtime bytecode.
- `GenesisPostHookFactory` in `consensus/innbcbft/innbcbft.go` initializes the system proxies, validator components, native token and governance components during genesis state initialization.
- The server connects the consensus genesis hook to the executor, whose `WriteGenesis` path applies allocations and executes the hook.

These existing paths operate independently of the removed arbitrary-contract CLI utility. They remain the supported mechanisms for INNBC system-contract genesis setup. For this deployment workflow, they supersede the need for the legacy utility; the generic external-artifact and constructor-execution CLI feature is retired.

The removed source packages are:

- `command/genesis/predeploy/`
- `helper/predeployment/`, including its dedicated parser tests

No remaining source imports or calls the removed helper.

#### Rejection of Retired Invocations

The remaining `genesis` command now uses `cobra.NoArgs`. It accepts its existing flags while rejecting positional arguments before genesis setup runs.

This prevents an invocation such as `genesis predeploy` from being interpreted as ordinary genesis generation with an ignored positional argument. The retired command is absent from registration and help output; no compatibility alias is retained.

Valid flag-based genesis generation continues through the existing validation and generation callbacks. Other command groups, installed genesis files, shared file writers and INNBC system-contract initialization are outside this removal.

#### Predeploy Retirement Regression Coverage and Closure

`command/root/genesis_predeploy_retirement_test.go` exercises the actual root dispatcher with and without the JSON-output flag.

Coverage verifies:

- Absence of the retired subcommand from registration and help.
- Rejection of the retired name before or after current genesis flags.
- Rejection of legacy predeploy flags.
- Prevention of fallback into ordinary genesis setup or execution.
- Preservation of existing genesis and artifact file contents.
- Absence of a newly requested genesis output file.
- Continued routing of ordinary flag-only invocations to the genesis setup boundary.

The routing regression intercepts setup and execution callbacks to prevent accidental generation during the test. Existing genesis and initialization tests provide the broader execution coverage.

Supplied validation passed the focused retirement regression, twenty race-enabled repetitions, broader genesis, root, helper, state, consensus, server and snapshot package tests, and `go vet`.

<!-- START ALPHA CHANGE 3284 -- record closure of all reported arbitrary-contract predeploy defects through retirement -->

Complete retirement of `genesis predeploy` and its exclusive implementation closes these reported command paths:

- Deleting the original genesis configuration before publishing its replacement.
- Normalizing malformed destination addresses before inserting an allocation.
- Panicking when an arbitrary constructor executes `BASEFEE` with an uninitialized base fee.
- Printing a JSON-mode execution error without returning a failing process status.

These findings are closed by removal of the affected functionality. The current INNBC system-contract allocation and genesis initialization paths remain independent and supported.

This retirement does not make shared file writers atomic or enforce byte-for-byte immutability of genesis configuration files. The separate exit-status correction for the retained `snapshot build` command is documented in point 93.

<!-- FINISH ALPHA CHANGE 3284 -- record closure of all reported arbitrary-contract predeploy defects through retirement -->

<!-- FINISH ALPHA CHANGE 3218 -- document retirement of arbitrary-contract genesis predeployment and preservation of INNBC system-contract initialization -->

<!-- FINISH ALPHA CHANGE 3086 -- document simplified genesis generation and accepted epoch-retention constraints -->

<!-- START ALPHA CHANGE 3091 -- document Homestead enforcement during unprotected legacy signature recovery -->

### 81. Unprotected Legacy Signature Recovery Preserves the Homestead Low-S Rule

Unprotected legacy transactions use signature recovery values `V=27/28` and do not include chain-ID replay protection.

These transactions remain supported. Their signature validation must nevertheless respect the selected fork's Homestead rule, which requires `S` to be at most half the secp256k1 curve order.

Previously, `EIP155Signer.Sender` delegated unprotected legacy recovery to an empty `FrontierSigner{}`. That construction defaulted `isHomestead` to false, permitting high-S signatures even when Homestead was active. The London signer's legacy fallback also reached this branch.

The correction passes the existing fork setting into the fallback:

```go
return NewFrontierSigner(e.isHomestead).Sender(tx)
```

The existing `ValidateSignatureValues` comparison remains unchanged.

#### Resulting Behavior

For otherwise valid signatures:

| Selected fork behavior | Low-S unprotected signature | High-S unprotected signature |
| --- | --- | --- |
| Homestead disabled | Allowed | Allowed |
| Homestead enabled | Allowed | Rejected |

The default INNBC genesis enables Homestead at block zero, so its unprotected legacy transactions must satisfy the low-S rule.

Protected legacy and dynamic-fee transactions retain their existing signature rules. Unprotected transactions retain their existing absence of chain-ID replay protection.

#### Transaction Identity

ECDSA permits an equivalent signature representation obtained by replacing `S` with `N − S` and flipping recovery parity. Both representations can recover the same sender for the same signed transaction meaning, while producing different transaction hashes.

Enforcing the low-S rule prevents acceptance of the alternative high-S representation when Homestead is active.

This correction concerns signature canonicality and transaction identity. The reviewed defect did not establish signature forgery, theft or authority to change the signed destination, value or nonce. Normal nonce enforcement prevents sequential execution of both variants.

The implementation rejects the disallowed signature without rewriting it. Valid transaction encodings and hashes remain unchanged.

#### Regression Evidence

The regression files are:

- `crypto/unprotected_signature_test.go`
- `txpool/unprotected_signature_test.go`
- `state/unprotected_signature_test.go`

The regressions:

- Generate a genuinely unprotected transaction using `NewFrontierSigner`.
- Construct its high-S equivalent on a copy, preserving the original transaction.
- Confirm both variants recover the same sender with Homestead disabled.
- Confirm high-S rejection through EIP-155, London and `NewSigner` with Homestead enabled.
- Exercise the default fork configuration with chain ID 1982.
- Preserve protected-legacy and dynamic-fee signature coverage.
- Verify pool rejection without insertion or slot allocation.
- Exercise execution sender recovery with `From` unset.
- Verify execution rejection before balance, nonce, gas, logs, refund or receipt changes.
- Confirm successful low-S execution and preservation of submitted transaction bytes.

Supplied validation passed focused regressions, twenty race-enabled repetitions, broader tests covering `crypto`, `txpool`, `state/...`, `consensus/innbcbft/...`, `jsonrpc` and `server`, and vet over those packages.

#### Scope and Closure

The reviewed unprotected legacy Homestead-bypass issue is closed by the applied correction and reviewed regression results.

The correction reuses the existing signature check and recovery path. It introduces no additional signature-recovery pass, database lookup or EVM execution.

<!-- FINISH ALPHA CHANGE 3091 -- document Homestead enforcement during unprotected legacy signature recovery -->

<!-- START ALPHA CHANGE 3098 -- document rejection of malformed RPC hexadecimal input -->

### 82. Malformed RPC Hexadecimal Input Rejection

The shared RPC byte decoder now propagates hexadecimal decoding errors. Previously, an invalid field could silently become empty calldata or a zero numeric value, allowing a simulation to process a different request meaning.

Malformed hexadecimal parameters now produce JSON-RPC `-32602` Invalid Params before the endpoint executes. Existing admission controls remain in place and can reject an overloaded request before parameter decoding.

The correction covers fields using the shared decoder, including:

- Transaction `data`, `input`, `value`, `gasPrice`, `gasTipCap` and `gasFeeCap` in call, estimation and trace requests.
- Account override code.
- Raw transaction submission bytes.
- `web3_sha3` input bytes.

For example, `data: "0xzz"` is rejected instead of being simulated as empty calldata. Failed decoding also preserves an existing receiver value because assignment occurs only after successful decoding.

Previously accepted byte formats remain supported: empty strings and `"0x"`, prefixless hexadecimal, odd-length hexadecimal and uppercase or lowercase hexadecimal digits. This correction does not introduce stricter canonical DATA or QUANTITY formatting.

#### Operational Scope

Invalid input is rejected normally; it does not stop the node or consensus. This correction adds no per-rejection warning or error log. The existing generic request log remains available when DEBUG logging is enabled.

Normal request parsing and response costs remain, under the existing RPC admission and request-size controls. Rejected malformed parameters do not reach endpoint execution, transaction-pool admission or EVM simulation.

The change affects RPC input validation. It does not change committed transaction validation or establish that malformed signed raw transactions were previously accepted.

#### Regression Coverage and Closure

`jsonrpc/hex_input_validation_test.go` covers accepted formats, error propagation, receiver preservation, transaction fields, override code, raw submission and SHA3 input. Dispatcher regressions assert that malformed parameters are rejected before endpoint/backend work, with valid requests exercising the expected paths. Existing admission-error precedence is also covered.

Supplied validation passed focused regressions, twenty race-enabled repetitions, broader package tests and vet.

The reviewed malformed-hexadecimal reinterpretation issue is closed by the applied correction and reviewed regression results.

<!-- START ALPHA CHANGE 3177 -- document error-preserving typed hash and address decoding -->

#### Typed Hash and Address Decoding

The shared `types.Hash.UnmarshalText` and `types.Address.UnmarshalText` decoders now propagate hexadecimal decoding errors.

Previously, these methods used `StringToBytes`, which discarded decoding errors. An invalid hash such as `"0xzz"` could become the zero hash. An address containing a valid 20-byte prefix followed by invalid characters could retain that decoded prefix and pass the address-length check.

Both methods now decode into temporary bytes and return any decoding error before assigning the receiver. Address decoding additionally requires exactly 20 decoded bytes before assignment. A failed conversion preserves the existing hash or address value.

This closes the typed-input path separately from the earlier `argBytes` correction.

The affected RPC inputs include:

- Block, transaction and receipt hash parameters.
- Account addresses in balance, nonce, code and storage queries.
- Storage-slot hash parameters.
- Simulation `from` and `to` addresses.
- Object-form block selectors containing `blockHash`.
- Log-filter block hashes, addresses and topics.
- State-override address keys and storage hash keys/values in `state` and `stateDiff`.

Malformed typed parameters produce JSON-RPC `-32602` Invalid Params before endpoint invocation. Existing admission limits can take precedence under overload.

#### Preserved Formatting and Shared-Type Scope

This correction rejects invalid hexadecimal characters; it does not introduce strict hash-length validation.

| Input behavior | Preserved policy |
| --- | --- |
| Optional lowercase `0x` prefix | Supported |
| Prefixless hexadecimal | Supported |
| Uppercase or lowercase hexadecimal digits | Supported |
| Odd-length hexadecimal | Padded with one leading zero digit before decoding |
| Valid short hash | Left-padded to 32 bytes |
| Valid oversized hash | Converted using its final 32 decoded bytes |
| Empty hash string or `"0x"` | Converted to the zero hash |
| Address length | Must decode to exactly 20 bytes |
| Omitted or null optional pointer fields | Existing optional-field behavior preserved |

Invalid characters anywhere in a hash are rejected before padding or truncation can produce an accepted value. This includes invalid characters in the leading portion of an oversized hash.

The existing 39-digit address acceptance through odd-length padding remains supported. Genesis premine and allocation-key validation retain their separate, stricter address policy.

The convenience functions `StringToBytes`, `StringToAddress` and `StringToHash` remain unchanged. Callers using these functions do not acquire the new error-returning validation.

Because the correction belongs to the shared `types` package, it also applies to other text/JSON consumers using these unmarshalling methods, including configuration and operator inputs.

Rejected RPC input remains an ordinary request error. The correction adds no state reads, signature recovery, EVM execution or per-rejection warning logging. It does not change committed signed-transaction validation.

#### Typed Decoder Regression Coverage and Closure

Coverage is provided by:

- `types/typed_hex_test.go`
- `jsonrpc/typed_hex_validation_test.go`

The shared-type tests verify accepted formats, round trips, receiver preservation, address-length errors, optional fields and unchanged convenience conversions. Invalid-character coverage includes partially decoded input and oversized hashes containing invalid leading or middle characters.

Dispatcher regressions cover direct identifiers, simulation addresses, object-form block selectors, log filters and state overrides. Rejected requests must leave endpoint, backend, execution, tracing and transaction-pool counters at zero. Valid counterparts must then reach the expected endpoint and backend paths. Admission-error precedence is also verified.

Supplied validation passed focused typed-decoder, byte-decoder and balance-override regressions, twenty race-enabled repetitions, broader types, RPC, chain, genesis, state, consensus and server tests, and `go vet`.

The reported typed-hash/address hexadecimal-error suppression issue is closed by the applied correction, source review and passing regression results.

<!-- FINISH ALPHA CHANGE 3177 -- document error-preserving typed hash and address decoding -->

<!-- FINISH ALPHA CHANGE 3098 -- document rejection of malformed RPC hexadecimal input -->

<!-- START ALPHA CHANGE 3099 -- document validated unsigned 256-bit RPC balance overrides -->

### 83. Full-Width RPC Balance Overrides

RPC account balance overrides now use a dedicated unsigned 256-bit decoder instead of the previous 64-bit representation. Valid balances above `MaxUint64`, including `"0x10000000000000000"` (2^64), can therefore be used in account-override simulations.

The supported range is zero through 2^256 - 1, inclusive. Values are parsed exactly without conversion through floating-point numbers.

| Balance representation | Meaning |
| --- | --- |
| String beginning with lowercase `0x` | Hexadecimal integer |
| String without that prefix | Decimal integer |
| Integer JSON number | Decimal integer |
| Omitted field or JSON `null` | No balance override |
| Explicit zero | Apply a zero balance override |

For example, `"10"` means decimal ten, while `"0x10"` means sixteen. Leading zeros remain accepted in numeric strings; invalid leading-zero JSON number tokens remain rejected.

The decoder validates characters and significant width before constructing the integer. It permits at most 64 significant hexadecimal digits or 78 decimal digits, then checks the actual bit length. The final check rejects decimal values that have 78 digits but exceed the unsigned 256-bit maximum.

Negative values, signs, fractions, exponent notation, malformed digits, empty numeric strings, empty hexadecimal payloads and out-of-range values are rejected. Unlike an empty byte field, `"0x"` is not a valid balance.

Failed decoding preserves an existing receiver value. Conversion to `types.OverrideAccount` creates an independent `big.Int` copy, preventing mutation of the converted balance from changing the decoded request.

Nonce overrides remain unsigned 64-bit values. Other account override fields retain their existing behavior.

#### Regression Coverage and Closure

`jsonrpc/override_balance_test.go` covers zero, `MaxUint64`, 2^64, larger balances and the unsigned 256-bit maximum across supported representations. It also checks overflow, malformed input, leading zeros, optional fields, receiver preservation and independent balance conversion.

Dispatcher regressions verify that invalid balances return Invalid Params before endpoint/backend work. A real EVM regression executes code that reads and returns the overridden account balance, verifies its exact 32-byte value, and reopens the original committed state root to confirm that balances, code, nonce and storage remain unchanged.

The existing `TestOverrideAccount_ToType` fixture is updated to exercise the new representation.

Supplied validation passed focused and related RPC regressions, twenty race-enabled repetitions, broader tests covering `jsonrpc`, `state/...`, `types` and `server`, and vet over those packages.

The reviewed 64-bit RPC balance-override limitation is closed by the applied correction and reviewed regression results.

This correction adds input validation and a local balance copy. It introduces no additional state lookup or EVM execution beyond the requested simulation and does not change committed balance accounting, transaction affordability or consensus behavior.

<!-- FINISH ALPHA CHANGE 3099 -- document validated unsigned 256-bit RPC balance overrides -->

<!-- START ALPHA CHANGE 3115 -- document legacy history diagnostic error handling and read-only source access -->

<!-- START ALPHA CHANGE 3264 -- document retirement of the legacy historical trie diagnostic -->
### 84. Legacy History Diagnostic Retired

The `regenesis history` command has been removed together with its exclusive implementation, historical range traversal, temporary in-memory trie copying and command-specific tests.

`regenesis getroot` remains available. Shared MDBX readers, trie copying, complete-state validation, snapshot workflows and node recovery implementations remain in use.

The routing regression in `command/root/regenesis_copy_retirement_test.go` verifies rejection of the retired command and flags, successful group help, retained `getroot` routing and absence of database-directory creation in text and JSON modes.

No replacement history diagnostic is introduced by this retirement.

#### Historical Header-Failure Correction

The following records the safety correction and regression evidence that preceded retirement. The implementation and history-specific regression files referenced below have since been removed.

Before retirement, the corrected command stopped on header-read failures before accessing the header or processing its state root.
<!-- FINISH ALPHA CHANGE 3264 -- document retirement of the legacy historical trie diagnostic -->

Previously, the command checked the canonical-hash lookup's earlier `ok` value after calling `ReadHeader`. A successful canonical lookup followed by a missing, corrupt or unreadable header could therefore reach `header.StateRoot` with a nil header and panic.

The correction checks the actual header-read error immediately. Errors identify the block number and canonical hash and preserve the underlying cause through `%w`. An unexpected nil header without an error is also rejected explicitly.

A failed header read cannot reach trie copying, root verification or a success message for that block. Head lookup, canonical lookup, trie validation and output-write failures also stop the diagnostic.

#### Read-Only Source Access and Cleanup

Both the trie and blockchain sources are opened using their existing read-only MDBX constructors.

The blockchain diagnostic no longer opens its source through the writable constructor. Read-only access uses stored database geometry, does not apply the writable default map-size setting, and does not create a missing source database directory.

Both source-close defers remain in place. The source-opening helper returns only after its cleanup has completed, so database handles are closed before the command's deferred output formatter reports an error.

Trie verification continues copying a selected root into temporary memory storage and checking the reconstructed root. Temporary storage is closed after each verification.

#### Preserved Diagnostic Behavior

The command retains its descending range behavior:

- `--to` is included.
- `--from` is excluded.
- `--to 0` resolves the upper bound from the stored head.
- Consecutive identical state roots reuse the preceding successful trie check.

Every selected header is still read before repeated-root detection, so an identical state root does not bypass header-read error handling. Head resolution uses a local range value rather than overwriting the configured flag value.

#### Regression Evidence

The regression files are:

- `command/regenesis/history_validation_test.go`
- `command/regenesis/history_mdbx_test.go`

Injected-reader tests cover header errors with both nil and nonnil returned headers, an unexpected nil header without an error, missing head/canonical data, trie failures and output-write failures. They assert rejection before trie work or inappropriate success reporting and verify preservation of wrapped error causes.

MDBX regressions exercise valid history and canonical references to missing, corrupt and hash-mismatched headers. Fixtures contain canonical mappings and explicitly selected ranges that enter the checking loop.

Source-preservation checks compare database records, stored geometry, page size and committed transaction identity before and after successful and failed diagnostics. They do not require reader-lock filesystem bytes to remain unchanged.

Missing-source tests verify that read-only opening fails without creating the missing database directories.

Supplied validation passed focused regressions, two separate runs of twenty race-enabled repetitions, broader tests covering `command/regenesis`, `command/root`, `blockchain/storage/...` and `state/...`, and vet over those packages.

#### Scope and Closure

The reviewed legacy `regenesis history` header-read panic issue is closed by the applied correction, source review and passing regression results.

This correction affects an explicitly invoked operator diagnostic. It does not change shared header validation, committed state, consensus, block production, P2P processing or ordinary node execution.

<!-- FINISH ALPHA CHANGE 3115 -- document legacy history diagnostic error handling and read-only source access -->

<!-- START ALPHA CHANGE 3120 -- document overflow-safe EVM memory expansion accounting -->

### 85. Overflow-Safe EVM Memory Expansion Accounting

The shared EVM memory-expansion calculation now preserves the complete multiplication result and rejects unrepresentable arithmetic before allocating memory.

Previously, the quadratic charge used `uint64` multiplication directly:

`3 × words + (words × words) / 512`

At sufficiently large sizes, `words × words` could overflow and silently reduce the gas charge.

For example, `MSTORE8` at offset `2^37 - 1` requests 128 GiB of memory and produces `2^32` memory words. The old multiplication wrapped to zero, leaving a memory-expansion charge of only 12,884,901,888 gas. A custom block gas limit of approximately 13 billion could therefore permit the enormous allocation.

The default generated genesis gas limit of 5,242,880 could not fund that example. The correction nevertheless protects the shared calculation independently of that default.

#### Checked Arithmetic and Allocation Ordering

`state/runtime/evm/memory_gas.go` uses `math/bits` to calculate the full 128-bit product. A guarded right shift computes division by 512 without truncating the quadratic charge. Linear multiplication and addition of the two charge components are checked separately.

The shared `allocateMemory` function also checks:

- Addition of offset and size.
- Rounding to complete 32-byte words.
- Conversion of the rounded allocation size to the platform's `int` range.
- That the new total charge is not below the previously charged total.

All arithmetic checks precede gas charging and memory allocation. The cached `lastGasCost` is updated only after charging succeeds.

<!-- START ALPHA CHANGE 3581 -- distinguish nonzero operand limits from unused zero-sized offsets -->

Nonzero memory ranges retain the existing operand limits and checked expansion accounting. Zero-sized ranges ignore their unused memory offset, including values larger than `uint64`, as detailed in point 111. Accessing already allocated memory incurs no additional expansion charge.

<!-- FINISH ALPHA CHANGE 3581 -- distinguish nonzero operand limits from unused zero-sized offsets -->

#### Execution Failure Behavior

Insufficient gas produces the existing EVM out-of-gas error. Unrepresentable arithmetic produces the existing gas-overflow error.

These are ordinary non-revert execution failures. The EVM wrapper consumes the remaining execution gas, and existing transition handling rolls back execution changes while preserving the applicable sender nonce and gas charges.

The reported 13-billion-gas example now fails before attempting the 128 GiB allocation. It can produce a failed transaction receipt without preventing subsequent transactions from being processed.

#### Regression Coverage and Validation

The regression files are:

- `state/runtime/evm/memory_gas_test.go`
- `state/memory_expansion_test.go`

Arithmetic tests compare charges against an independent `big.Int` reference, including sizes around `2^32` words and the boundaries of representable charges. Enormous-size arithmetic tests do not allocate the requested memory.

Additional tests cover word rounding, allocation-size conversion, incremental versus single expansion, existing-memory access, operand rejection, zero-length behavior, and preservation of memory and cached accounting on failure.

The actual `MSTORE8` regression verifies rejection before memory growth and consumption of execution gas. Transition regressions execute a successful storage-write control, verify rollback of that write after memory-expansion failure, check failed-receipt accounting, and confirm that the next transaction succeeds in the same transition.

Supplied validation passed focused regressions, twenty race-enabled repetitions, broader tests covering `state/...`, `consensus/innbcbft/...`, `jsonrpc` and `server`, and vet over those packages.

#### Scope and Closure

The reviewed EVM memory-cost overflow issue is closed by the applied correction, source review and passing regression results.

Production arithmetic uses fixed-width operations; the independent `big.Int` reference is confined to tests. The correction adds no database reads, registry calls or additional EVM execution passes. Runtime overhead was not benchmarked in this validation.

No new genesis gas-limit restriction or universal memory cap is introduced. Exceptionally large gas allowances can still fund exceptionally large allocations when their correctly calculated charges are affordable.

<!-- FINISH ALPHA CHANGE 3120 -- document overflow-safe EVM memory expansion accounting -->

<!-- START ALPHA CHANGE 3146 -- document checked recent-history segment counts and startup rejection -->

### 86. Overflow-Safe Recent-History Segment Configuration

Recent-history configuration now rejects retained-segment counts that cannot be represented by the platform's signed `int` type.

The configured count is:

`retained segments = history retention blocks / history segment blocks`

Previously, validation checked segment geometry but did not check the unsigned-to-signed conversion. On a 64-bit platform, retention of 9,223,372,036,854,775,808 blocks with one block per segment passed geometry validation but converted to a negative segment count.

That negative count could cause a fresh genesis write to access an empty pruning list and panic. With existing history, it could also cause excessive segment retirement.

The standard configuration remains:

| Setting | Value |
| --- | --- |
| History retention blocks | 43,200 |
| History segment blocks | 1,800 |
| Retained segment count | 24 |

This count remains 24 as the chain grows. Increasing the current block number does not increase the configured retention-to-segment ratio or trigger this conversion defect.

#### Shared Validation and Rejection Boundaries

`blockchain/storage/segmented/retention.go` provides the shared `RetainedSegmentCount` helper. It requires:

- A positive segment size.
- Retention covering at least one complete segment.
- Retention divisible by the segment size.
- A resulting count no greater than `math.MaxInt`.

The helper checks the unsigned count before converting it to `int`. Successful validation therefore returns a positive, representable count.

The shared helper is applied at these boundaries:

| Boundary | Rejection behavior |
| --- | --- |
| CLI history configuration | Returns a configuration error before the server starts |
| Direct `server.NewServer` calls | Rejects recent-history geometry before logger creation, snapshot activation, database opening or network startup |
| Segmented storage construction | Rejects before directory creation or database access |
| Segmented snapshot-builder configuration | Rejects before source loading or artifact construction |

An invalid count produces an error identifying the requested count and the platform maximum. It is a configuration failure, not a failed transaction or consensus execution result.

The limit applies to the calculated count, rather than to retention blocks alone. A large retention value can remain representable when its segment size produces a supported count.

Full-history CLI configuration continues disabling segmented retention. Checkpoint snapshots are not subject to the segmented-count restriction.

#### Pruning Behavior and Scope

The storage constructor now uses the checked count when dividing the blockchain MDBX allowance.

The existing pruning calculation remains safe because construction establishes a positive count within the platform's `int` range. The private segment and retention fields are initialized by that constructor and are not subsequently reassigned.

Existing pruning, the per-segment minimum MDBX allowance, permanent genesis retention and trie-reclamation policy remain unchanged.

These checks run during configuration validation and storage initialization. They add no recurring validation to transaction execution, block production or pruning.

This correction is separate from the documented epoch/retention compatibility constraint in point 80. It prevents numeric conversion failure; it does not establish that every custom retention setting preserves enough history for epoch reconstruction.

#### Regression Coverage and Closure

The regression files are:

- `blockchain/storage/segmented/retention_test.go`
- `command/server/history_segment_count_test.go`
- `server/history_segment_count_test.go`
- `snapshotbuilder/history_segment_count_test.go`

Coverage includes invalid geometry, the standard 24-segment configuration, the maximum representable count, the first unsupported count, the reported 64-bit overflow input and the maximum unsigned input.

Tests verify rejection before directory creation, direct server startup effects or snapshot construction. Storage regressions write genesis and subsequent headers, verify successful reopening, compare genesis database bytes around a rejected reopen attempt, and confirm the retained canonical entries and head remain available.

Maximum-count regressions write only blocks 0, 1 and 2; they do not create an enormous number of segments.

Supplied validation passed focused regressions, existing history-configuration and physical-pruning tests, twenty race-enabled repetitions, and broader storage, server, snapshot-builder and snapshot-command package tests.

The reviewed retained-segment conversion overflow and resulting startup panic issue is closed by the applied correction, source review and passing regression results.

<!-- FINISH ALPHA CHANGE 3146 -- document checked recent-history segment counts and startup rejection -->

<!-- START ALPHA CHANGE 3159 -- document transaction-start storage accounting and tracing transaction cleanup -->

### 87. Transaction-Start Storage Gas Accounting and Consistent Trace Replay Cleanup

Storage gas accounting now uses the value of each slot at the beginning of the current top-level transaction, including changes made by earlier successful transactions in the same block.

Previously, block execution reused one transition for all transactions, but `GetCommittedState` read original storage from the block-start snapshot. Current storage therefore included earlier transactions' writes while original storage could still describe the state before those transactions.

This could incorrectly classify a transaction's first storage write as a cheaper dirty-slot rewrite.

Under the existing Istanbul storage rules, the reported examples were:

| Slot before the block | First transaction | Second transaction | Previous second-write charge | Correct second-write charge |
| --- | --- | --- | --- | --- |
| Zero | Stores `1` | Stores `2` | 800 gas | 5,000 gas |
| `1` | Clears the slot | Stores `2` | 800 gas | 20,000 gas |

These figures are the storage instruction charges, excluding intrinsic transaction gas and other instructions.

In the clear-then-add example, the incorrect original value also caused subtraction of a 15,000-gas clearing refund from the second transaction's empty refund counter. Unsigned subtraction wrapped, allowing an unjustified refund subject to the existing half-used-gas cap.

The correction fixes both storage classification and the cross-transaction refund-underflow cause. It applies to the default fork configuration.

#### Reusing the Existing Transaction Snapshot

`Transition.Apply` reuses its existing rollback snapshot as the original-storage baseline for that transaction.

`Txn.GetCommittedState` reads storage from this stable in-memory view first, including earlier successful transactions' writes and explicit slot deletions. Where the view has no overriding slot value, it uses the applicable persistent storage root.

The baseline remains fixed during nested calls and rollback. `Apply` restores the previous baseline reference on every exit.

Account creation records when storage was reset during the active transaction. Account copies preserve that identity, and rollback restores the previous account object. A newly created account therefore starts with empty original storage, while a later transaction correctly sees storage written by its creation transaction.

Deleted accounts, full-storage overrides, state-diff overrides and retained storage-read failures receive explicit handling. Direct setup operations outside `Apply` retain their existing original-storage behavior.

The existing storage gas schedule and refund rules remain in place. No refund clamp or per-slot original-value cache is introduced.

Block execution continues reusing one transition and publishing the final state once. The correction adds no extra transaction snapshot, historical scan, repeated EVM execution or per-transaction database commit.

#### Transaction Cleanup During Trace Replay

A separate tracing issue existed because `TraceBlock` and the preceding-transaction loop in `TraceTxn` called `Apply` repeatedly without the transaction cleanup performed by `Write`.

A previous replayed transaction could leave its refund counter populated. Later transactions could then receive an incorrect refund, altering traced gas usage, simulated balances and subsequent replay behavior. Deleted or empty accounts and transient transaction logs also lacked the corresponding transaction boundary.

`Transition.FinalizeTransaction` now provides the shared cleanup used by normal `Write` execution and these tracing paths. It:

- Applies the existing deleted/empty-account cleanup policy.
- Clears the transaction refund counter.
- Returns and removes the transaction's transient logs.
- Propagates retained state errors and cleanup failures.

`Write` uses the shared method in place of its previous cleanup block, retaining one cleanup pass before accepting gas accounting and the receipt.

`TraceBlock` captures each trace result and then finalizes the transaction before replaying the next one. `TraceTxn` finalizes each preceding transaction and captures the target trace before finalizing the target.

Cleanup also runs after an ordinary failed EVM execution when `Apply` returns no application error. Application errors and retained state failures stop replay.

Finalization does not publish state or create receipts. Tracing uses temporary execution state and leaves the persisted parent state unchanged.

`TraceCall` executes a single transaction in a fresh transition and has no subsequent transaction replay.

#### Regression Coverage and Validation

The original-storage regression files are:

- `state/storage_original_test.go`
- `state/storage_original_execution_test.go`

Their assertions cover transaction boundaries, nested rollback, account creation and deletion, storage overrides, retained read failures and baseline restoration.

Signed execution tests cover legacy and dynamic-fee transactions on memory and MDBX backends. They assert exact gas charges, refunds, receipt status, cumulative gas, sender balances and fee-recipient balances. They also cover insufficient-gas failure followed by successful execution, preservation of the parent state, one final publication batch and an actual MDBX reopen.

Supplied validation passed the original-storage regressions, twenty race-enabled repetitions, related regressions, broader state, consensus, JSON-RPC and server tests, and vet. The reported committed-block original-storage accounting issue is closed by the applied correction, source review and those results.

The additional tracing regression files are:

- `state/trace_cleanup_test.go`
- `server/trace_cleanup_test.go`

These tests assert refund reset, log removal, account cleanup and retained-error handling. Both tracing endpoints are compared with normal `Write` execution across legacy and dynamic-fee transactions, including clearing followed by recreation, failed execution followed by success, deleted-account access and empty-account cleanup.

The tracing assertions also check preservation of persisted parent state and absence of trie publication during replay.

Performance overhead has not been benchmarked. The original-storage correction adds bookkeeping and in-memory lookups; tracing now performs the transaction cleanup already required by normal execution.

<!-- FINISH ALPHA CHANGE 3159 -- document transaction-start storage accounting and tracing transaction cleanup -->

<!-- START ALPHA CHANGE 3164 -- document orderly signal shutdown without competing process deadlines -->

### 88. Orderly Service Shutdown Without Competing CLI Deadlines

Normal signal-initiated shutdown now waits for the existing component cleanup sequence to finish instead of imposing an additional five-second process-exit deadline.

Previously, the CLI started `Server.Close` in a goroutine but returned a shutdown error after five seconds. The server command then exited the process, potentially interrupting cleanup.

That deadline competed with JSON-RPC's own five-second graceful HTTP shutdown allowance. An admitted request waiting for an incomplete body could consume that allowance before HTTP connections were forcibly closed and workers joined. gRPC, monitoring, consensus, txpool, networking and storage still required subsequent cleanup.

The defect could therefore turn an ordinary service stop into abrupt process termination. Resulting database corruption was not demonstrated.

#### Signal Handling and Cleanup Ownership

`command/helper/helper.go` now owns its process signal registration and defers `signal.Stop` when the handler returns.

Shutdown coordination uses an injected signal channel internally, allowing unit tests to exercise the policy without sending signals to the main test process.

The shutdown policy is:

| Event | Behavior |
| --- | --- |
| First SIGINT, SIGTERM or SIGHUP | Starts the cleanup callback once and waits for completion |
| Cleanup completes | Returns normally |
| Second termination signal during signal-initiated cleanup | Returns the existing forced-exit error, allowing the server command to terminate deliberately |
| Fatal consensus error | Waits synchronously for cleanup, then returns the original wrapped fatal error |
| Missing cleanup callback | Completes without invoking a callback |

The automatic five-second CLI timer is removed.

The second-signal escape applies to signal-initiated shutdown. The fatal-error branch retains its existing synchronous cleanup behavior.

#### Component Timeouts and Service Policy

Existing component shutdown timeouts remain intact.

JSON-RPC still attempts graceful HTTP shutdown, forcibly closes HTTP connections if its allowance expires, closes WebSocket connections and joins its workers before completing dispatcher cleanup.

The existing server shutdown order continues stopping external intake before closing consensus, txpool, networking, blockchain storage and trie storage. Individual component errors continue being reported while subsequent cleanup is attempted.

The packaged node service explicitly sets:

`TimeoutStopSec=infinity`

This aligns the supervisor stop policy with waiting for component cleanup. Systemd therefore does not impose an automatic stop deadline through this setting.

If a component is permanently blocked, the first signal can wait indefinitely. Explicit operator intervention remains available. This correction does not make blocked synchronous storage operations cancellable.

The changes affect signal coordination and service shutdown policy. They introduce no recurring consensus, transaction, EVM or fee-processing work.

#### Regression Coverage and Closure

The regression files are:

- `command/helper/shutdown_signals_test.go`
- `jsonrpc/shutdown_incomplete_body_test.go`

Signal-handler tests verify waiting beyond the removed five-second deadline, exactly one cleanup invocation, missing-callback handling, fatal-error preservation, malformed fatal-channel handling and explicit second-signal early return.

An isolated Linux subprocess verifies that signal registration is released when the public handler returns, restoring the normal SIGTERM termination behavior.

The loopback HTTP regression sends an incomplete request body and exercises the real JSON-RPC handler and `JSONRPC.Close`. It verifies expiration of the component's graceful allowance, forced connection closure, handler completion, dispatcher cleanup, continuation to a following cleanup stage and listener reuse. The incomplete request never reaches application dispatch.

Existing regressions also verify idempotent JSON-RPC closure, waiting for active HTTP handlers and closure of active WebSocket connections.

Supplied validation passed focused tests, twenty race-enabled repetitions, broader tests covering command helpers, server commands, common helpers, JSON-RPC and server packages, and vet. The packaging-source check confirmed `TimeoutStopSec=infinity`.

The reviewed premature process-exit defect is closed by the applied correction, source review and passing regression results.

<!-- START ALPHA CHANGE 3550 -- distinguish orderly shutdown from failed server construction -->

This correction governs shutdown after successful server construction. It does not add automatic cleanup when `NewServer` fails partway through construction. Point 105 documents acceptance of that separate limitation for the standalone systemd deployment, where startup failure terminates the process and automatic restart launches a fresh process.

<!-- FINISH ALPHA CHANGE 3550 -- distinguish orderly shutdown from failed server construction -->

<!-- FINISH ALPHA CHANGE 3164 -- document orderly signal shutdown without competing process deadlines -->

<!-- START ALPHA CHANGE 3195 -- document numeric RPC block-selector range validation -->

### 89. Numeric RPC Block Selectors Cannot Become Reserved Tags

Numeric RPC block selectors now receive a range check before conversion to the signed `BlockNumber` representation.

Previously, the decoder parsed an unsigned 64-bit integer and converted it directly to `int64`. Oversized values could become negative numbers matching the internal constants for reserved block tags.

| Supplied numeric selector | Previous interpretation | Corrected behavior |
| --- | --- | --- |
| `"0xffffffffffffffff"` | `earliest` | Rejected |
| `"0xfffffffffffffffe"` | `latest` | Rejected |
| `"0xfffffffffffffffd"` | `pending` | Rejected |

For example, an oversized numeric selector in `eth_getBalance` could previously request the genesis state instead of the specified numeric block. Similar reinterpretation could affect queries and simulations using the shared decoder.

This concerned RPC input meaning. It did not establish modification of canonical storage, acceptance of malformed signed transactions or a consensus bypass.

#### Supported Range and Preserved Behavior

Numeric selectors must be between zero and `MaxInt64`, inclusive:

- Decimal maximum: `9223372036854775807`.
- Hexadecimal maximum: `0x7fffffffffffffff`.

Larger numeric selectors are rejected before signed conversion. Explicit `"earliest"`, `"latest"` and `"pending"` strings retain their existing endpoint-specific behavior.

Accepted hexadecimal strings, decimal strings and supported unquoted JSON integers remain accepted within that range. Integer JSON input is parsed from its textual representation without conversion through `float64`.

The check resides in `jsonrpc/codec.go`, inside `stringToBlockNumber`. It covers direct selectors, object-form `blockNumber` selectors and log-filter `fromBlock`/`toBlock` strings. Existing log-filter treatment of omitted or empty bounds and explicit `"pending"` as latest remains unchanged.

The shared `ParseUint64orHex` helper retains its unsigned range for other callers.

Accepting `MaxInt64` establishes only that the selector is representable. It does not establish that the requested block exists; normal endpoint lookup behavior still applies.

#### Rejection and Execution Cost

Parameter-decoding failures produce JSON-RPC `-32602` Invalid Params before endpoint invocation. Existing request-admission checks retain their earlier precedence.

A rejected numeric selector does not replace an existing `BlockNumber` receiver value.

The production correction adds one integer comparison during numeric selector parsing. It adds no database reads, signature recovery, EVM execution or recurring consensus work. Performance overhead has not been benchmarked.

#### Regression Coverage and Closure

`jsonrpc/block_number_range_test.go` covers:

- Zero, ordinary values, exact integers above the floating-point precision boundary and `MaxInt64`.
- The first unsupported value, all three reserved-tag collisions and values above `MaxUint64`.
- Hexadecimal strings, decimal strings, unquoted JSON integers and object-form selectors.
- Preservation of existing receivers on decoding failure.
- Both log-filter bounds and their existing accepted-tag behavior.
- Rejection before endpoint, backend, simulation, tracing or transaction-pool work.
- Valid block, balance, call, estimation, tracing, fee-history and log-query controls.

Balance controls verify the selected height and account state root. A separate regression confirms that `MaxInt64` reaches ordinary endpoint lookup and can return a missing-header error rather than Invalid Params.

These regressions exercise actual decoding, dispatch and endpoint code with mock backends. They do not execute real EVM transactions.

Supplied validation passed focused regressions, twenty race-enabled repetitions, complete JSON-RPC, types and server package tests, and `go vet` over those packages.

The reported oversized numeric block-selector reinterpretation issue is closed by the applied correction, source review and passing regression results.

<!-- FINISH ALPHA CHANGE 3195 -- document numeric RPC block-selector range validation -->

<!-- START ALPHA CHANGE 3199 -- document full-width effective gas prices in RPC receipts -->

### 90. Full-Width Receipt Effective Gas Prices

`eth_getTransactionReceipt` now preserves the full precision of `effectiveGasPrice`.

Previously, transaction prices and execution calculations used `big.Int`, but RPC receipt conversion narrowed the calculated price through `Uint64()` and stored it in `argUint64`. Prices above the 64-bit range therefore lost their upper bits.

| Actual effective price | Previous RPC output | Corrected RPC output |
| --- | --- | --- |
| Zero | `"0x0"` | `"0x0"` |
| 2^64 | `"0x0"` | `"0x10000000000000000"` |
| 2^64 + 7 | `"0x7"` | `"0x10000000000000007"` |

This could misreport fees to wallets, indexers and accounting tools. Ordinary prices within the unsigned 64-bit range were unaffected.

#### Representation and Scope

The RPC receipt field now uses the existing arbitrary-precision `argBig` quantity representation. `toReceipt` converts the result of `tx.GetGasPrice(header.BaseFee)` directly, without narrowing it.

The field remains present as a hexadecimal JSON string. Zero remains `"0x0"`.

The existing effective-price calculation is unchanged. Dynamic-fee calculation continues using the block's base fee and transaction fee caps. `GetGasPrice` constructs independent integers, so converted prices do not alias the transaction's price fields or another independently converted receipt.

This correction changes RPC reporting only. It does not alter execution charges, sender refunds, fee distribution, consensus receipt commitments or database receipt encoding. No database migration is required.

The existing single price calculation remains in place. The correction adds no database reads, signature recovery, EVM execution or recurring consensus work. Performance overhead has not been benchmarked.

#### Regression Coverage and Closure

`jsonrpc/receipt_price_test.go` covers:

- Zero, ordinary prices, `MaxUint64`, 2^64, 2^64 + 7 and maximum-256-bit conversion.
- Explicit dynamic-fee transactions with no legacy gas price and a nonzero block base fee.
- Independent expected values for both branches of `min(feeCap, baseFee + tipCap)`.
- Successful and failed status receipts, plus legacy root receipts.
- Exact hexadecimal output through actual RPC dispatch and `eth_getTransactionReceipt`.
- Independence from later transaction-price and header mutations.
- Independence between separately converted receipt prices.
- Preservation of source consensus and storage receipt encodings.

The dispatcher fixture also verifies that receipt retrieval retains its expected three backend reads.

These tests use mock stored data. The maximum-value conversion case verifies serialization, not transaction affordability or execution.

The existing `Test_toReceipt` assertion now compares arbitrary-precision values. Existing receipt fixtures covering logs, contract addresses and root/status exclusivity remain covered, together with the full-width fee-history regressions.

Supplied validation passed focused regressions, twenty race-enabled repetitions, complete JSON-RPC, types, gas-price and server package tests, and `go vet` over those packages.

The reported receipt `effectiveGasPrice` truncation issue is closed by the applied correction, source review and passing regression results.

<!-- FINISH ALPHA CHANGE 3199 -- document full-width effective gas prices in RPC receipts -->

<!-- START ALPHA CHANGE 3206 -- document the retained legacy trie-copy utility and strict required roots -->

<!-- START ALPHA CHANGE 3261 -- document retirement of the standalone trie-copy operation -->
### 91. Standalone Legacy Trie-Copy Operation Retired

The standalone `regenesis` trie-copy operation has been removed. Its exclusive implementation, required-root parser and command-specific copy/exit tests have been deleted.

<!-- START ALPHA CHANGE 3264 -- update the remaining regenesis command surface -->
`regenesis` remains a subcommand-only group containing `getroot`. Invoking the group alone displays help. Both standalone copying and the history diagnostic are retired. The former copy flags and history flags are no longer registered; point 84 documents history retirement.
<!-- FINISH ALPHA CHANGE 3264 -- update the remaining regenesis command surface -->

The supported maintenance workflows are:

| Operator purpose | Preferred workflow |
| --- | --- |
| Node bootstrap or checkpoint recovery | Validated snapshot workflows |
| Reclaim obsolete trie data on recent nodes | Segmented-snapshot rebootstrap |
| Reconstruct a trie from complete canonical history | `node trie-rebuild` |

<!-- START ALPHA CHANGE 3264 -- distinguish shared maintenance implementations from retired CLI operations -->
Shared trie-copy and verification implementations remain available to the snapshot builder and current maintenance workflows. Retirement of the legacy CLI operations preserves those shared implementations.
<!-- FINISH ALPHA CHANGE 3264 -- distinguish shared maintenance implementations from retired CLI operations -->

`command/root/regenesis_copy_retirement_test.go` covers group help, rejection of retired flags and unknown subcommands, retained diagnostic routing, and absence of database-directory creation in both text and JSON modes.

#### Historical Root-Validation Correction

The following records the correction applied before retirement. Its command-specific implementation and regression files have since been removed.
<!-- FINISH ALPHA CHANGE 3261 -- document retirement of the standalone trie-copy operation -->

Previously, unchecked `StringToHash` conversion could ignore hexadecimal errors, pad short roots or truncate oversized roots. A valid root followed by an invalid suffix could therefore select the valid decoded prefix.

The required `--stateRoot` input must now contain exactly 64 hexadecimal digits, optionally prefixed with lowercase `0x`. Uppercase and mixed-case hexadecimal digits are accepted. Empty, short, oversized, odd-length, whitespace-containing or invalid hexadecimal input is rejected.

Unlike the optional genesis `--trieroot` argument, an omitted or empty `--stateRoot` is an error. A syntactically valid hash still needs to be usable by the copy operation; parsing does not establish availability.

Root and required-path validation run before either database opens. The command parses the root once and reuses that value for copying, verification and comparison.

The source opens read-only using its stored MDBX geometry. The destination retains the legacy command's direct-copy behavior: a later copy failure can leave partial destination data. This correction does not add atomic snapshot publication.

Parameters belong to each command instance. Command construction produces no copy output, and `getroot` and `history` do not acquire copy-flag requirements.

Shared hash conversions, trie-copy helpers, snapshot construction, trie rebuilding, genesis generation and ordinary node execution are unchanged.

#### Regression Coverage and Closure

`command/regenesis/trie_copy_test.go` checks strict formats, rejection before database opening, an existing root with an appended invalid suffix, destination preservation on malformed input, actual MDBX copies containing contract code and storage, source record/geometry preservation, successful output and subcommand isolation.

Supplied validation passed focused regressions, twenty race-enabled repetitions, broader regenesis/root/genesis/snapshot/trie tests, and `go vet` across the checked packages.

The reported malformed legacy trie-copy root issue is closed by the applied correction, source review and passing regression results.

<!-- FINISH ALPHA CHANGE 3206 -- document the retained legacy trie-copy utility and strict required roots -->

<!-- START ALPHA CHANGE 3210 -- document reliable legacy trie-copy failure reporting and process exit status -->

<!-- START ALPHA CHANGE 3261 -- retain the former trie-copy exit correction as historical documentation -->
### 92. Historical Legacy Trie-Copy Failure Reporting and Process Exit Status

The standalone legacy trie-copy operation has been retired as described in point 91. The following records its error-reporting correction and regression evidence before removal.

The former implementation propagated operational failures through Cobra's error-returning `RunE` callback.
<!-- FINISH ALPHA CHANGE 3261 -- retain the former trie-copy exit correction as historical documentation -->

Previously, output finalization happened during command construction, before execution. Later failures could remain unreported and leave the process with a successful exit status. Moving output handling into execution corrected that lifecycle defect, but JSON-mode failures could still return status zero because the shared JSON formatter prints errors without terminating the process.

The command now returns validation, database-opening, copying, verification and database-close errors to the existing root error handler. Database cleanup completes before the error reaches that handler.

| Outcome | Command output | Process exit status |
| --- | --- | --- |
| Worker failure in text mode | Error message on standard error | `1` |
| Worker failure in JSON mode | `{"error":"..."}` on standard error | `1` |
| Successful copy and verification, with successful database cleanup | Existing success output on standard output | `0` |

A command-local JSON error wrapper preserves escaping and the underlying error cause. Cobra's duplicate error and usage output is suppressed for these returned failures, allowing the root handler to report the command error once. MDBX may separately emit native diagnostic messages.

Success output is produced only after the copy worker and its database cleanup return successfully. Validation failures stop before database access.

The correction remains local to the standalone copy command. Shared formatters, the root error handler, `regenesis getroot`, `regenesis history`, genesis generation, snapshot workflows and ordinary node execution are unchanged.

A later copy failure can still leave partial destination data. A nonzero exit status identifies failure; it does not make destination publication atomic.

#### Regression Coverage and Closure

Unit regressions verify returned command errors, JSON escaping and error-cause preservation.

`command/root/regenesis_exit_test.go` exercises the actual root command in subprocesses, using both text and JSON modes. It checks status `1` for missing arguments, malformed roots, unavailable sources, target-opening failures and copy failures.

Successful copies return status `0`, retain the expected output format and pass complete-state validation after reopening. Failure cases verify that no success result is emitted and the command error is reported once.

Supplied validation passed focused regressions, twenty race-enabled repetitions, broader regenesis/root/genesis/snapshot/trie tests, and `go vet` across the checked packages.

The reported legacy trie-copy failure-exit issue is closed by the applied correction, source review and passing regression results.

<!-- FINISH ALPHA CHANGE 3210 -- document reliable legacy trie-copy failure reporting and process exit status -->

<!-- START ALPHA CHANGE 3285 -- document snapshot build failure exit status and subprocess regression coverage -->

### 93. Snapshot Build Failures Return Nonzero Status in Text and JSON Modes

The retained `snapshot build checkpoint` and `snapshot build segmented` commands now return a failing process status when snapshot-kind parsing or snapshot construction fails.

Previously, the execution callback recorded errors through `SetError` without communicating failure to the process exit path. The JSON formatter printed the error and returned normally, allowing unsuccessful commands to exit with status zero.

`runCommand` in `command/snapshot/build/build.go` now returns status `1` for parsing and builder failures, and status `0` after setting a successful result. The outer Cobra callback applies the returned failure status after the helper returns.

Snapshot-builder cleanup completes before command termination. Deferred output formatting remains in the command helper, and the existing text and JSON result formats are preserved.

| Outcome | Output | Process exit status |
| --- | --- | --- |
| Unsupported snapshot kind or builder failure in text mode | Error on standard error; no success result | `1` |
| Unsupported snapshot kind or builder failure in JSON mode | `{"error":"..."}` on standard error; no success result | `1` |
| Successful checkpoint or segmented build | Existing result format on standard output | `0` |

MDBX may separately emit native diagnostic messages before the formatted JSON error.

The correction does not change shared formatters, the root error handler, snapshot copying, artifact validation or publication logic. It adds no recurring consensus or transaction-execution work.

#### Regression Coverage and Closure

Coverage is provided by:

- `command/root/snapshot_build_exit_fixture_test.go`
- `command/root/snapshot_build_exit_test.go`
- The corrected subprocess helper in `command/snapshot/backup/backup_test.go`

Actual root-command subprocess tests cover unsupported kinds, missing chain configuration, successful MDBX-backed checkpoint and segmented builds, and both text and JSON output modes.

Successful builds retain their result fields and produce artifacts that pass published-artifact validation.

Repeating a build for an already published checkpoint exercises failure after staging and validation. The regression verifies status `1`, absence of success output, removal of temporary `.building-*` directories, preservation of the existing publication and successful validation of that publication afterward. File-preservation comparisons exclude mutable MDBX reader-lock bookkeeping.

Subprocess helpers explicitly exit with status zero if command execution unexpectedly returns normally. This prevents a test-generated failure status from masking the command defect. The backup helper correction changes test behavior only.

Supplied validation passed the focused regressions, twenty race-enabled repetitions, broader command, snapshot-builder, server, consensus, blockchain and trie tests, and `go vet` across the checked packages.

The reported snapshot-build failure-exit defect is closed within this scope. The removed predeploy occurrence is closed by retirement as documented in point 80.

<!-- FINISH ALPHA CHANGE 3285 -- document snapshot build failure exit status and subprocess regression coverage -->

<!-- START ALPHA CHANGE 3292 -- document ordinary snapshot staging durability and scoped closure -->

### 94. Snapshot Staging Is Synchronized Before Publication and Previous-Snapshot Retirement

Ordinary `snapshot build checkpoint` and `snapshot build segmented` construction now synchronizes the completed staging tree before publishing it.

Previously, artifact validation could read successfully from operating-system caches without establishing persistence of every required file and directory entry. The builder could subsequently publish the latest pointer and remove the previous ordinary snapshot. A host crash or power loss could therefore leave an incomplete replacement as observed after recovery, with the previous snapshot already removed.

#### Publication Ordering

The ordinary builder now performs these operations in order:

1. Complete artifact writes, close destination databases, and validate the staged artifact.
2. Synchronize staged files and then directories from children to parents using `syncArchiveTree`.
3. Rename staging into its final location and synchronize the publication directory.
4. Write and synchronize the temporary latest-pointer file, rename it to `latest.json`, and synchronize its parent directory.
5. Retire an eligible previous snapshot under the existing retention policy.

The staging barrier runs after validation, destination-collision checks and loading the existing latest pointer, immediately before the publication rename.

A staging synchronization failure returns an error with its underlying cause preserved. The previous snapshot and existing `latest.json` remain untouched. Existing deferred cleanup attempts to remove that invocation's unpublished `.building-*` directory.

Coordinated recovery-checkpoint construction already synchronizes its completed staging tree through its recovery finalization hook. That path retains its existing synchronization without an additional ordinary-build pass.

#### Regression Coverage and Validation

Coverage is provided by `snapshotbuilder/snapshot_durability_test.go`:

- `TestSnapshotDurability_PublicationOrdering`
- `TestSnapshotDurability_SyncFailurePreservesPrevious`
- `TestSnapshotDurability_ValidationFailureBeforeSync`

Each test covers checkpoint and segmented construction using MDBX-backed fixtures.

The regressions verify that the previous publication remains intact at the synchronization barrier, the synchronized candidate is subsequently published, injected synchronization failures preserve the previous artifact and latest pointer, invalid receipt commitments stop construction before synchronization, and unpublished staging is cleaned up under the tested conditions.

File-preservation comparisons exclude mutable MDBX reader-lock bookkeeping. Synchronization failure injection is internal and per invocation; public `Build` calls select the real synchronization helper.

Supplied validation passed all six focused cases, twenty race-enabled repetitions, broader tests across snapshot-builder, root commands, snapshot commands and server packages, and `go vet` across those packages.

These tests verify ordering and error handling. They do not simulate physical power loss.

#### Scope and Closure

The reported ordinary-snapshot staging durability gap is closed.

The correction adds filesystem synchronization during snapshot construction. It adds no consensus or transaction-execution logic and does not change the source node's canonical databases.

<!-- START ALPHA CHANGE 3335 -- reference the separate genesis-contiguous history-floor correction -->

The separate genesis-contiguous history-floor mismatch is resolved as documented in point 98. Manifest `RetainedFrom = 1` now correctly expects storage `HistoryFrom() = 0`, and validation checks block 1's parent against the validated genesis header.

The durability fixtures retain their truncated suffixes. Separate history-floor regressions cover genesis-contiguous snapshots, truncation boundaries and publication preservation.

<!-- FINISH ALPHA CHANGE 3335 -- reference the separate genesis-contiguous history-floor correction -->

<!-- FINISH ALPHA CHANGE 3292 -- document ordinary snapshot staging durability and scoped closure -->

<!-- START ALPHA CHANGE 3298 -- document explicit historical log errors after physical pruning -->

### 95. Historical Log Queries Reject Unavailable History Without Returning Partial Results

Historical range queries through `eth_getLogs` and `eth_getFilterLogs` now distinguish unavailable historical blocks from requests extending beyond the current head.

Previously, the scanner reported pruned history only when a block body was unavailable but its header remained readable. Physical segment retirement removes canonical mappings, headers and bodies together. Both lookups could therefore fail, causing the scanner to stop and return an empty or partial log array as a successful response.

#### Resulting Behavior

The missing-block path now applies these rules:

| Condition | Behavior |
| --- | --- |
| Header exists but the full block is unavailable | Return `ErrPrunedHistory`. |
| Both lookups fail and the requested height is below the available history floor | Return `ErrPrunedHistory`. |
| Both lookups fail for a height within the current chain and advertised available history | Return `ErrBlockNotFound`. |
| Both lookups fail and current head metadata is unavailable | Return `ErrLatestNotFound`. |
| Both lookups fail above the current head | Preserve normal end-of-range behavior. |

A historical failure discards logs already collected for that request. The RPC response reports an error without presenting an incomplete log array as a successful result.

Fully available ranges continue returning matching logs. Existing block-hash query behavior, genesis exclusion and range limits are preserved.

#### Deployment Policy and Execution Cost

Public historical RPC traffic remains directed to full-history nodes. Healthy genesis-origin full-history nodes do not normally encounter physical history retirement. Recent nodes remain validator and synchronization nodes rather than public historical RPC backends.

The correction also provides explicit failure handling when queries reach recent nodes or checkpoint-origin databases with unavailable earlier history.

Additional head and history-floor checks run only after both block lookups fail. Successful block scans retain their existing path.

The runtime RPC hub already exposes `Blockchain.HistoryFrom()`. Standard storage caches its history floor when opened; segmented storage obtains it from retained segment metadata in memory. The correction introduces no historical database scan, trie traversal, transaction replay, background worker or new persistent index.

#### Regression Coverage and Closure

Coverage is provided by `jsonrpc/log_history_test.go`:

- `TestLogHistory_RangesAndFailureClassification`
- `TestLogHistory_RPCRejectsIncompleteResults`
- `TestLogHistory_PhysicalSegmentRetirement`

The regressions cover unavailable prefixes, ranges crossing the history floor, unexpected gaps after logs have already been collected, missing retained heads, body-only pruning, missing head metadata, healthy full-history and retained-suffix queries, future-height requests, genesis-only queries and range-budget enforcement.

RPC coverage verifies explicit errors without successful log arrays through both endpoints, followed by successful retrieval from an available range.

The physical-retirement regression uses actual segmented MDBX storage and production blockchain readers. It verifies directory removal, disappearance of the canonical mapping and header, failure of both block lookups, and an explicit pruned-history error.

Successful-scan cases assert that no history-floor lookup occurs. Shared RPC and block test stores explicitly provide full-history metadata.

Supplied validation passed the focused regressions, existing log-query, pruning, range-limit and null-filter tests, twenty race-enabled repetitions, broader JSON-RPC/server/blockchain tests, and `go vet` across those packages.

<!-- START ALPHA CHANGE 3524 -- distinguish physical-pruning closure from receipt-count completeness -->
The reported silent empty or partial historical-log response caused by physical history retirement is closed. This correction changes RPC failure handling and adds no consensus, transaction-execution or pruning logic.

The separate rejection of decodable but incomplete receipt lists is documented in [point 104](#104-historical-log-queries-reject-incomplete-receipt-lists).
<!-- FINISH ALPHA CHANGE 3524 -- distinguish physical-pruning closure from receipt-count completeness -->

<!-- FINISH ALPHA CHANGE 3298 -- document explicit historical log errors after physical pruning -->

<!-- START ALPHA CHANGE 3310 -- document controlled rejection of null required server configuration sections -->

### 96. Null Required Server Configuration Sections Return Controlled Startup Errors

Explicitly null `network`, `telemetry` or `tx_pool` sections in JSON/YAML server configuration files now return a clear configuration error instead of reaching unchecked pointer accesses during startup.

For example, `"network": null` returns:

`invalid server configuration: section "network" must not be null`

#### Validation and Preserved Behavior

`Config.ValidateSections` in `command/server/config/config.go` checks the configuration pointer and the three required sections.

Validation runs immediately after decoding in `ReadConfigFile`, and at the beginning of `initRawParams` in `command/server/init.go`, before other initialization settings are accessed or normalized.

For JSON/YAML configuration:

| Input | Behavior |
| --- | --- |
| Section omitted | Preserve existing loader defaults. |
| Empty object `{}` | Preserve existing section values. |
| Partial object | Update supplied fields while retaining other existing values. |
| Explicit null | Return an error naming the required section. |
| Incorrect section type | Preserve the decoder's error. |

YAML `null`, `~` and an empty value after the section name are covered.

The loader retains its separate network initialization with peer limits set to `-1`. HCL retains its existing decoding behavior, including section-pointer replacement.

Optional-section policies remain intact: nil history receives the existing history defaults, and nil RPC admission configuration selects the existing admission defaults.

#### Regression Coverage and Closure

Coverage is provided by:

- `command/server/config/sections_test.go`
- `command/server/config_sections_test.go`
- `command/root/config_sections_test.go`

Regressions cover null sections, omitted and partial sections, empty objects, preserved decoder errors, valid HCL decoding, direct initialization with nil configuration or required sections, and existing optional-section policies.

Actual CLI subprocess tests verify exit status `1`, the named-section error, absence of panic output, and no runtime-data creation in both text and JSON output modes. Pre-run errors retain the existing root error format.

Supplied validation passed the focused regressions, twenty race-enabled repetitions, broader command/JSON-RPC/server tests, and `go vet` across those packages.

The reported null-section startup-panic defect is closed. Validation adds only a few pointer checks during configuration loading and startup.

<!-- FINISH ALPHA CHANGE 3310 -- document controlled rejection of null required server configuration sections -->

### 97. Retained Commands Return Nonzero Status on Execution Failure

Execution failures in `genesis`, `server export`, `regenesis getroot` and `status` now produce exit status **1** in both text and JSON modes. Successful execution retains status **0**.

Previously, these commands could print a JSON error and return normally, causing automation to interpret a failed operation as successful.

#### Execution and Cleanup

Each command now reports its execution outcome through a command-local exit status. In JSON mode, the outer callback applies the failure status after the execution helper finishes and flushes its output. The existing text formatter continues to terminate with status 1 when reporting an error.

Resource-owning operations complete their cleanup before error output and process termination. The `status` command also closes its owned gRPC connection.

Existing output formats are preserved:

- JSON-mode errors remain `{"error":"..."}` on standard error.
- Failed operations do not emit a successful command result.
- Genesis retains its validator progress output.
- `regenesis getroot` retains its textual success output in both modes.

The shared output formatters and root error handler remain unchanged. Argument and pre-run validation continue through their existing error paths.

#### Regression Coverage and Closure

`command/root/command_execution_exit_test.go` exercises actual CLI subprocesses in text and JSON modes. Coverage includes:

- Genesis generation and publication failures.
- Unsupported configuration-export types and destination failures.
- Root-lookup RPC errors and unavailable blocks.
- Status RPC failures.
- Successful execution, expected output and actual process exit codes.

The subprocess helper exits **0** when command execution returns normally, preventing the test itself from manufacturing an expected failure status.

<!-- START ALPHA CHANGE 3352 -- distinguish the original command exit-status correction from additional operator commands -->

The reported execution-failure exit-status defect is closed for these four commands. The separate snapshot-build correction remains documented in point 93. Additional peer, transaction-pool and monitoring command corrections are documented in point 99.

<!-- FINISH ALPHA CHANGE 3352 -- distinguish the original command exit-status correction from additional operator commands -->

This correction adds no consensus or transaction-execution work.

<!-- START ALPHA CHANGE 3336 -- document genesis-contiguous snapshot history-floor validation and closure -->

### 98. Genesis-Contiguous Snapshots Validate the Correct History Floor

Checkpoint and segmented snapshots can now pass history-floor validation when their retained non-genesis range begins at block 1.

Previously, construction recorded `RetainedFrom = 1` and copied genesis separately, followed by every block through the checkpoint. Both storage implementations correctly reported continuous history from genesis as `HistoryFrom() = 0`. Directly comparing those values rejected an otherwise valid artifact before publication.

This affected early snapshots whose checkpoint remained within the configured retention window, including the default 43,200-block window.

#### Preserved Manifest and Storage Semantics

The manifest continues to describe the retained non-genesis range. Genesis remains separately included and validated.

The validator translates the retained-range start into the expected storage history floor:

| Manifest retained-range start | Required storage history floor |
| --- | --- |
| `RetainedFrom = 1` | `HistoryFrom() = 0` |
| `RetainedFrom > 1` | Exactly `RetainedFrom` |

The actual storage floor is read once and must equal the translated expectation. Zero is not a wildcard, and truncated ranges still require their exact starting height.

The correction does not change the manifest format, retention calculations, copied block ranges or either storage implementation's history-floor semantics.

#### Genesis-to-Block-1 Continuity

When `RetainedFrom == 1`, retained-header validation begins with the already validated genesis header as its predecessor.

The existing parent-hash comparison therefore verifies block 1's link to genesis as well as subsequent links within the retained range.

For truncated ranges, validation retains its existing starting behavior because the preceding block is outside the artifact's retained history.

Existing canonical mapping, header, body, transaction-root, receipt-root, transaction-lookup, checkpoint and state validation remain in place.

#### Publication Safety and Cost

Artifact validation still completes before staging synchronization and publication.

A validation failure preserves the previous published snapshot and `latest.json`. Unpublished staging cleanup remains in place. The durability ordering documented in point 94 is preserved.

The correction adds a small floor comparison and, for genesis-contiguous snapshots, one parent-hash comparison using the genesis header already read by validation. It adds no historical scan, transaction replay, contract execution or consensus work.

#### Regression Coverage and Closure

Coverage is provided by these tests in `snapshotbuilder/snapshot_integrity_test.go`:

- `TestSnapshotHistoryFloor_BuildAndReopen`
- `TestSnapshotHistoryFloor_RejectsMissingCanonicalBlocks`
- `TestSnapshotHistoryFloor_FailurePreservesPublication`

Both checkpoint and segmented formats are exercised using real MDBX fixtures.

Coverage includes:

- A checkpoint at block 1.
- Retention larger than and exactly equal to the checkpoint.
- The first truncated range and multi-block segment boundaries.
- The default 43,200-block retention setting using a small source fixture.
- Successful construction, publication, latest-pointer validation and database reopening.
- Rejection of incorrect storage floors for both genesis-contiguous and truncated ranges.
- Rejection of missing first, middle and final canonical mappings after reopening a damaged test copy.
- Rejection of a block-1 parent that differs from genesis while the remaining candidate links and commitments are internally consistent.
- Preservation of the previous publication and cleanup of staging after validation or synchronization failure.
- Rejection of invalid genesis continuity before staging synchronization is reached.

Supplied validation passed the focused history-floor, durability, retained-range and manifest tests. Twenty race-enabled repetitions of the same selection also passed.

The reported genesis-contiguous snapshot history-floor mismatch is closed within this scope.

<!-- FINISH ALPHA CHANGE 3336 -- document genesis-contiguous snapshot history-floor validation and closure -->

<!-- START ALPHA CHANGE 3353 -- document additional operator command failure statuses and subscription cleanup -->

### 99. Peer and Transaction-Pool Commands Report Execution Failures Correctly

Execution failures in the following retained operator commands now produce exit status **1** in both text and JSON modes:

<!-- START ALPHA CHANGE 3528 -- include peer-directory refresh in corrected execution statuses -->

- `peers list`
- `peers status`
- `txpool status`
- `peers add`
- `monitor`
- `txpool subscribe`
- `peerdirectory refresh`

<!-- FINISH ALPHA CHANGE 3528 -- include peer-directory refresh in corrected execution statuses -->

Successful unary operations retain exit status **0**. Subscription commands also return **0** on clean EOF or intentional termination handled through their cancellation context.

Previously, execution failures could print a JSON error while the command returned normally with status 0. Individual `peers add` failures were instead included in its result, allowing complete or partial addition failure to return status 0 in both output modes.

#### Execution Status and Connection Ownership

Execution helpers now return an integer status. Their Cobra callbacks apply a failing process status after the helpers finish.

Unary RPC operations own and close their gRPC connections before final output. Subscription operations also release their connection and signal registration before terminal error reporting.

The shared JSON formatter, text formatter and root error handler remain unchanged. JSON execution errors retain their existing `{"error":"..."}` format on standard error.

#### Peer Addition Results

`peers add` returns status **1** when any requested addition fails, including partial success.

Its detailed result remains available in both output modes, retaining the requested count, successful count, successful addresses and individual errors. Accumulated per-peer errors do not replace that result with a generic formatter error.

Connection-creation failures use the ordinary formatted-error path.

Each invocation resets accumulated successes and errors. Results from an earlier invocation cannot inflate the next invocation's counts or carry old failures into it.

#### Subscription Completion and Shutdown

`monitor` and `txpool subscribe` receive events synchronously using a context registered for interrupt, SIGTERM and SIGHUP before subscription establishment.

This removes the previous receiver goroutine and its unbuffered completion send, which could remain blocked after the signal branch returned.

The completion policy is:

| Outcome | Exit status |
| --- | --- |
| Connection or subscription establishment failure | 1 |
| Unexpected receive or transport failure | 1 |
| Remote cancellation while the local context remains active | 1 |
| Clean EOF | 0 |
| Intentional termination producing cancellation of the local subscription context | 0 |

Events received before a later failure remain printed. That later failure still produces status 1 and one terminal error report.

Each event is written once. Commands do not repeat the last event during final output.

#### Regression Coverage and Closure

`command/root/command_execution_exit_test.go` contains:

<!-- START ALPHA CHANGE 3528 -- document refresh subprocess regression coverage -->

- `TestOperatorCommandExit_Unary`: successful and failed peer-list, peer-status and transaction-pool-status calls, plus successful, completely failed and partially successful peer additions in text and JSON modes.
- `TestOperatorCommandExit_RepeatedAdd`: two invocations in one subprocess, verifying that the second result does not retain the first invocation's successes.
- `TestOperatorCommandExit_Streams`: subscription RPC rejection before the first event, receive failure after an event, remote cancellation, clean EOF and intentional SIGTERM termination for both subscriptions in text and JSON modes.
- `TestOperatorCommandExit_PeerDirectoryRefresh`: successful refresh, unavailable endpoint and controlled RPC rejection in text and JSON modes. Checks include actual process status, preserved success output, one terminal error report and empty standard output on failure.

<!-- FINISH ALPHA CHANGE 3528 -- document refresh subprocess regression coverage -->

The subprocess tests verify actual exit codes, preserved result formats, exactly one terminal error, absence of duplicate event output and release of the test subscription RPC handler.

The subprocess helper exits 0 when command execution returns normally. It does not manufacture the failure status expected by the parent test.

Supplied validation passed:

- The focused `TestOperatorCommandExit_` and existing `TestCommandExecutionExit_` regressions.
- Twenty race-enabled repetitions of that selection.
- The broader `go test ./command/...` suite.
- `go vet ./command/...`, without diagnostics.

<!-- START ALPHA CHANGE 3528 -- document refresh failure reporting and connection cleanup -->

The original additional operator-command execution-failure issue, issue 4, remains closed for its six listed commands. The subsequent review's `peerdirectory refresh` occurrence is also closed. Point 97 retains its original four-command scope.

`peerdirectory refresh` returns exit status **0** on success and **1** on connection or RPC failure in both text and JSON modes. Its existing success result and formatted error output are preserved.

The RPC helper explicitly owns its gRPC connection and closes it before returning to the execution helper. Deferred output therefore runs after connection cleanup, including when the text formatter terminates the process. In JSON mode, the outer Cobra callback applies the returned failure status after formatted output completes.

The shared formatters and root handler remain unchanged.

These corrections affect operator-command reporting and resource cleanup. They add no consensus, transaction-execution or database-format changes.

<!-- FINISH ALPHA CHANGE 3528 -- document refresh failure reporting and connection cleanup -->

<!-- FINISH ALPHA CHANGE 3353 -- document additional operator command failure statuses and subscription cleanup -->

<!-- START ALPHA CHANGE 3369 -- document bounded node creation and read-only provisioning inspection -->

### 100. Bounded Node Creation and Read-Only Identity Inspection

The reported batch-numbering defect is closed. `node create` accepts one to ten new identities per invocation, while automatic allocation and inspection support node1–node100.

Creation plans the complete batch before writing. It selects available numbers in ascending order, including gaps, and treats either an existing node path or an existing environment path as a reservation. Exclusive creation prevents an occupied destination from being overwritten after planning.

The generated JSON-RPC, operator gRPC and libp2p port ranges remain separate within the supported numbering range. `node prepare` retains its deterministic node1-through-count behavior and its ten-node count limit.

#### Read-Only Verification After Provisioning

A later filesystem or key-generation failure can still leave completed nodes and an incomplete current slot. Creation does not implement automatic batch rollback.

The supported diagnostic is:

```bash
sudo innbc-chain node info all \
  --base-dir /var/lib/innbc \
  --config-dir /etc/innbc
```

Inspect one supported node with `node info nodeN`, where N is 1–100. Add `--json` for structured output.

Inspection reads canonical keys directly, validates their encoding and cryptographic usability, derives the public identity and verifies its operator-bound BLS possession proof. It independently reads and checks recognized public environment settings.

It does not initialize storage, create directories, change permissions, repair files or regenerate identities. Private-key contents are not included in the public report or key-decoder diagnostics. Unknown environment settings are omitted, and displayed peer-directory URLs have user information, query parameters and fragments removed.

#### Discovery and Failure Reporting

`node info all` inspects node1 through the highest supported number having either an identity path or an environment path.

Completely absent interior slots produce missing-identity warnings. Occupied incomplete or invalid slots produce status 1 after the report, without hiding later inspected nodes. Valid public identity information remains available when that node's environment is missing or invalid.

A healthy selection returns status 0. An empty installation also returns status 0 and reports that nothing was discovered. Discovery cannot infer an intended trailing node for which no identity or environment path remains.

JSON output preserves the detailed report on standard output and reports the terminal failure on standard error.

#### Validation Scope and Verification

Inspection checks stored identity usability and selected environment structure. It does not establish full runtime configuration validity, service-account access, network availability, genesis membership or that a running service uses those settings.

A different valid replacement key requires comparison against a trusted expected identity; successful decoding alone cannot detect that replacement.

The focused allocation, inspection and workflow tests passed, including twenty race-enabled repetitions. The broader command and secrets suites passed, and `go vet ./command/... ./secrets/...` completed without diagnostics.

The allocation defect is closed. Read-only inspection provides diagnostics for the intentionally retained partial-progress behavior; it does not make creation transactional.

See the [Node Environment Reference](docs/docs/node-env/README.md#read-only-node-inspection) for commands, checks, warnings and limitations.

<!-- FINISH ALPHA CHANGE 3369 -- document bounded node creation and read-only provisioning inspection -->

<!-- START ALPHA CHANGE 3433 -- document pre-launch VSET accounting retirement -->

### 101. ValidatorSet Epoch Tracking and VSET Retirement

ValidatorSet retains epoch tracking at proxy `0x101`, backed by implementation `0x1011`. Consensus continues to commit epochs and read the current epoch through this contract.

The retained epoch machinery includes `EPOCH_SIZE`, `currentEpochId`, `epochs`, `epochEndBlocks`, `commitEpoch`, `totalBlocks` and `NewEpoch`. Existing epoch identifier, range, divisibility, continuity and system-caller checks remain enforced.

#### Removed Accounting

The redundant VSET subsystem is retired:

- ERC-20 balances, allowances and token snapshots.
- Initial VSET minting and the `ValidatorInit[]` initializer argument.
- VSET burning, unstaking, withdrawal queues and withdrawal events.
- The exclusive withdrawal-queue library and data structures.
- The genesis validator `Stake` field, its JSON serialization and printed amount.

The initializer is now `initialize(uint256 newEpochSize)`.

Explicit legacy `"stake"` fields in supplied genesis validator records are rejected, including zero and null values. Regenerate stale genesis configurations rather than retaining an unused zero-stake setting.

#### Initialization Protection

<!-- START ALPHA CHANGE 3450 -- document the contract initializer epoch-size minimum -->

Initialization requires the system caller, an epoch size of at least two blocks and an uninitialized storage context. Zero and one-block epoch sizes revert with `INVALID_INPUT`, matching the shared Go configuration validation.

Rejected initialization leaves the storage context uninitialized. Regression coverage checks rejection of both values followed by successful initialization, while preserving system-caller restrictions and proxy/implementation storage isolation.

<!-- FINISH ALPHA CHANGE 3450 -- document the contract initializer epoch-size minimum -->

The constructor disables initialization for conventional deployments. Genesis assigns runtime bytecode directly and does not execute that constructor; the system-caller restriction therefore protects runtime-preallocated implementations as well as proxy initialization.

Proxy and implementation storage remain separate. Initializing implementation storage through the system caller does not initialize or change the proxy's epoch state.

#### Preserved Native-INNBC Functionality

Genesis validator addresses, BLS keys, operator-bound possession proofs and networking information remain present. Genesis validator metadata continues to assign voting power 1.

The separate INNBC validator registry retains native-INNBC staking, minimum-stake validation, registration, unstaking and committee eligibility checks. VSET retirement does not remove those operations or the native token.

#### Pre-Launch Artifact and State Requirements

The Solidity ABI, storage layout and runtime bytecode change. Regenerate Solidity artifacts, TypeChain factories and the embedded Go artifacts and bindings together.

Generate the launch genesis afterward using the updated client and artifacts. Existing development databases and snapshots are not migration inputs for this change.

Regression coverage exercises actual genesis initialization, valid and invalid epoch commitments, initializer restrictions, proxy/implementation isolation, retired selectors, generated genesis identities and compiled/embedded interfaces.

<!-- FINISH ALPHA CHANGE 3433 -- document pre-launch VSET accounting retirement -->

<!-- START ALPHA CHANGE 3514 -- document independent database page-size and storage-capacity customization -->

### 102. Configurable MDBX Page Size and Maximum Storage Capacity

INNBC separates database page size from the configured database growth allowance. Operators can choose a supported page size for new blockchain and trie databases and independently configure their storage allowance. This supports planning larger future installations while retaining conservative defaults.

The code defaults remain 4096-byte pages and a 1024 GiB (1 TiB) growth allowance. Generated full-history node environments explicitly contain:

```ini
INNBC_MDBX_PAGE_SIZE_BYTES=4096
INNBC_MDBX_MAP_SIZE_GB=1024
```

Generated recent-history environments retain their separate 128 GiB map-size setting. Consensus persistence retains its separate fixed 4096-byte page size and 1 GiB allowance.

#### Supported Page Sizes and Format Ceilings

Use the following decimal byte values for `INNBC_MDBX_PAGE_SIZE_BYTES`, `INNBC_SNAPSHOT_BUILDER_MDBX_PAGE_SIZE_BYTES`, or `--mdbx-page-size-bytes`:

| Page size | Setting value in bytes | MDBX format ceiling per database on supported 64-bit platforms | Approximate decimal TB |
| --- | --- | --- | --- |
| 4 KiB — default | `4096` | 8 TiB | 8.80 TB |
| 8 KiB | `8192` | 16 TiB | 17.59 TB |
| 16 KiB | `16384` | 32 TiB | 35.18 TB |
| 32 KiB | `32768` | 64 TiB | 70.37 TB |
| 64 KiB | `65536` | 128 TiB | 140.74 TB |

These are format ceilings for the pinned MDBX dependency. They describe each individual MDBX database, rather than aggregate node storage or guaranteed usable application-data capacity. Metadata and database overhead consume space, and filesystem, address-space and available-storage limits also apply.

One TiB equals 1,099,511,627,776 bytes. One decimal TB equals 1,000,000,000,000 bytes.

Each recent-history segment is a separate MDBX database. Its configured allowance follows the existing recent-history distribution policy described in point 79.

#### Increasing Capacity Above the Default 1 TiB

Page size and map size control different limits:

| Setting | Purpose |
| --- | --- |
| `INNBC_MDBX_PAGE_SIZE_BYTES` | Selects database creation geometry and its page-addressing capacity ceiling. |
| `INNBC_MDBX_MAP_SIZE_GB` | Sets the configured growth allowance in GiB. |

Increasing page size alone does not increase the configured growth allowance.

For example, a fresh installation using 8 KiB pages with a 2 TiB allowance would use:

```ini
INNBC_MDBX_PAGE_SIZE_BYTES=8192
INNBC_MDBX_MAP_SIZE_GB=2048
```

Here, 8192-byte pages provide a 16 TiB format ceiling, while the configured growth allowance remains 2 TiB. To configure an allowance above the code default of 1 TiB, increase `INNBC_MDBX_MAP_SIZE_GB` above `1024`.

The default 4096-byte page size already supports a format ceiling above 1 TiB. An operator needing a 2 TiB allowance can retain 4096-byte pages and change only the map-size setting. A larger page size is required only when the desired capacity exceeds the smaller page size's format ceiling.

Map-size allowances are not upfront physical disk allocations or guarantees that the host can sustain the selected capacity. Larger pages do not guarantee improved read or write performance.

<!-- START ALPHA CHANGE 3515 -- document conventional storage allowance values and required page sizes -->

#### Common Storage Allowance Values

The following conventional capacity increments provide convenient values for `INNBC_MDBX_MAP_SIZE_GB`. Enter the integer shown, without a unit suffix.

These are configuration reference choices, not estimates of the storage required by a particular node. The code default remains 1024 GiB (1 TiB); generated recent-history nodes use 128 GiB.

| Configured allowance | `INNBC_MDBX_MAP_SIZE_GB` value | Minimum compatible page size for an individual database |
| --- | --- | --- |
| 32 GiB | `32` | 4 KiB — `4096` |
| 64 GiB | `64` | 4 KiB — `4096` |
| 128 GiB | `128` | 4 KiB — `4096` |
| 256 GiB | `256` | 4 KiB — `4096` |
| 512 GiB | `512` | 4 KiB — `4096` |
| 1 TiB — code default | `1024` | 4 KiB — `4096` |
| 2 TiB | `2048` | 4 KiB — `4096` |
| 4 TiB | `4096` | 4 KiB — `4096` |
| 8 TiB | `8192` | 4 KiB — `4096` |
| 16 TiB | `16384` | 8 KiB — `8192` |
| 32 TiB | `32768` | 16 KiB — `16384` |
| 64 TiB | `65536` | 32 KiB — `32768` |
| 128 TiB | `131072` | 64 KiB — `65536` |

The page-size column identifies format compatibility on supported 64-bit platforms, not a performance recommendation. Some rows reach the corresponding page size's format ceiling; usable application-data capacity remains lower because database overhead consumes space. Host and filesystem limits also apply.

For example, `INNBC_MDBX_MAP_SIZE_GB=2048` configures a 2 TiB allowance. `INNBC_MDBX_MAP_SIZE_GB=2` configures only 2 GiB.

The same GiB values apply to:

- `--mdbx-map-size-gb` for direct server and maintenance commands.
- `INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB` for the packaged builder's general destination allowance.
- `INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB` for its segmented-snapshot allowance.

For recent-history blockchain storage, the node allowance is distributed across segments under the existing policy. Its trie receives the configured allowance independently. The page-size requirement must accommodate each individual database.

These conventional increments are suggestions, not an allowlist. Other positive integer GiB values, such as `3000`, are permitted when they satisfy platform and MDBX geometry limits. Zero, negative numbers, fractions, unit suffixes and explicitly blank map-size values are rejected. Omitting the CLI option selects its default.

Selecting another page size requires new compatible databases; changing either environment value does not convert existing database pages.

<!-- FINISH ALPHA CHANGE 3515 -- document conventional storage allowance values and required page sizes -->

#### Explicit Defaults and Invalid-Value Rejection

Generated environment files explicitly show `4096`, making the selected layout visible during inspection. An omitted setting or blank textual value also selects 4096.

Only these explicit byte values are accepted:

`4096`, `8192`, `16384`, `32768`, `65536`.

Values such as `3000`, `4000`, `5000`, `8000`, `0` or `8kb` are rejected rather than rounded. Supply decimal bytes without a unit suffix.

The corresponding CLI options are:

```bash
--mdbx-page-size-bytes 8192 \
--mdbx-map-size-gb 2048
```

These options are supported by server startup and the relevant snapshot and trie-rebuild commands. Direct foreground commands do not automatically read node or packaged-builder environment files. The standard command examples explicitly select 4096-byte pages.

#### Existing-Database and Snapshot Compatibility

Page size is selected when creating a database. Changing the setting does not convert existing databases.

Existing blockchain and trie databases, including active recent-history segments, must match the configured page size. Startup checks compatibility before writable initialization and rejects mismatches.

A selected snapshot must also have matching blockchain and trie page sizes. Compatibility is checked before copying and again before activation. A page-size mismatch stops startup even when snapshot bootstrap is optional.

Interrupted snapshot-activation recovery preserves existing recovery trees when their page sizes are incompatible with the current setting.

An in-place trie rebuild must match the retained blockchain database's page size. For an existing 8192-byte blockchain database, use `--mdbx-page-size-bytes 8192` when reconstructing its trie.

#### Independent Builder Destination Settings

The packaged snapshot builder uses its own settings:

```ini
INNBC_SNAPSHOT_BUILDER_MDBX_PAGE_SIZE_BYTES=4096
INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB=1024
INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB=128
```

The page-size setting applies to newly constructed snapshot, archive, recovery-checkpoint and backup databases.

The general map-size setting applies to checkpoint, archive, recovery-checkpoint and backup destinations. Segmented snapshots use the separate segmented map-size setting. Increase the applicable builder allowance separately when constructing larger destinations.

Source databases are opened read-only using their stored geometry. A builder can therefore read a 4096-byte source and construct an 8192-byte destination, provided the exported data fits the chosen destination allowance.

Consuming nodes must use a page-size setting matching the resulting snapshot. Existing archives and reused recovery artifacts must match the selected destination policy.

#### Recovery Publication Safeguards and Regression Coverage

Recovery-checkpoint construction checks archive compatibility before setup and again after acquiring both archive and checkpoint publication locks. The second check uses the canonical archive directory and runs before archive selection or checkpoint staging.

This rejects an incompatible archive published between the initial check and lock acquisition. Rejection preserves the published archive and previous snapshot, and releases both locks.

Regression coverage includes actual 4096-byte and 8192-byte database creation and reopening, mismatch preservation, snapshot destination selection, archive and recovery compatibility, and backup construction.

A deterministic regression publishes an archive between preflight and lock acquisition. It covers matching settings, mismatches in both directions, source and publication preservation, absence of staging leftovers, and lock release after success or rejection.

The capacity ceilings in the table are derived from the pinned MDBX implementation. They are not results of physically filling databases to those sizes.

These settings change physical database layout and configured storage allowances. They do not change consensus rules, contract artifacts, logical state roots or genesis contents. Compatibility checks run during startup and maintenance operations, adding no consensus or transaction-execution logic.

<!-- FINISH ALPHA CHANGE 3514 -- document independent database page-size and storage-capacity customization -->

<!-- START ALPHA CHANGE 3522 -- document controlled peer-directory rejection of null genesis validators -->

### 103. Peer-Directory Startup Rejects Null Genesis Validator Entries

The peer directory rejects null entries in the supplied genesis validator list before extracting bootstrap peer IDs.

Previously, a configuration containing `"initialValidatorSet": [null]` could reach an unchecked `MultiAddr` dereference and panic during directory startup. A null entry after valid entries had the same problem.

#### Validation and Startup Ordering

`buildBootstrapPeerIDs` in `command/peerdirectory/clicommand.go` checks each validator pointer before accessing its fields. A null entry returns an indexed error, for example:

`nil genesis validator at index 0`

Failure returns a nil allowlist, so entries processed before the error cannot produce a successful partial bootstrap set. The error wording matches the existing consensus genesis-validator check.

The startup caller propagates the error before peer-cache initialization and listener creation. The existing root command handler returns process exit status 1.

Valid multiaddress parsing, peer-ID deduplication and empty-list behavior remain unchanged. This correction does not introduce full consensus-validator validation into the directory helper.

#### Regression Coverage and Closure

Coverage is provided by:

- `TestPeerDirectoryGenesisValidators_BootstrapPeerIDs` in `command/peerdirectory/clicommand_test.go`.
- `TestPeerDirectoryGenesisValidators_CLI` in `command/root/config_sections_test.go`.

The helper regression covers null first entries, null entries after valid entries, valid peer IDs, existing deduplication behavior and malformed multiaddresses. Failure cases verify that no partial allowlist is returned.

The CLI regression generates a current valid genesis, including the required registry, native-token and epoch settings. Its valid validator has a complete bootstrap multiaddress. Temporary copies introduce null entries at indexes 0 and 1.

Actual subprocess checks verify exit status 1, the indexed error, absence of a panic, unchanged genesis contents and no cache-file creation, both with and without `--json`. The subprocess helper exits 0 if command execution returns normally, so it cannot manufacture the expected failure status.

Supplied validation passed focused regressions, twenty race-enabled repetitions, the command and peer-directory package suites, and vet.

The reported peer-directory startup panic is closed. This was an operator-configuration defect; no unauthenticated network trigger or chain-state corruption was established. The correction adds one pointer check per genesis validator during directory startup and introduces no recurring consensus, transaction-execution or request-processing work.

<!-- FINISH ALPHA CHANGE 3522 -- document controlled peer-directory rejection of null genesis validators -->

<!-- START ALPHA CHANGE 3523 -- document exact historical receipt-count validation -->

### 104. Historical Log Queries Reject Incomplete Receipt Lists

Historical log queries require the number of decoded receipts to equal the number of transactions in each nonempty block being scanned.

Every transaction requires a receipt, including failed transactions. A receipt containing no logs is valid; an absent receipt for an existing transaction is incomplete data.

Previously, the scanner rejected receipt lists longer than the transaction list but accepted shorter lists. A successfully decoded record containing two receipts for a three-transaction block could therefore produce a successful partial log response. A stored encoded empty list could produce a misleading empty response.

This finding concerned inconsistent local storage containing a decodable receipt record. It did not establish that ordinary MDBX writes or a host crash create that condition.

#### Validation and Error Propagation

`getLogsFromBlock` in `jsonrpc/filter_manager.go` checks exact count equality before extracting any logs. A mismatch returns a nil result and an error identifying the block and both counts, for example:

`receipt count mismatch for block …: expected 3, got 2`

The shared helper protects block-hash queries, range queries and retained filters' historical queries through `eth_getLogs` and `eth_getFilterLogs`.

When a later block fails validation, the range scanner returns an error and discards logs accumulated from earlier blocks. RPC errors do not carry a successful partial log array.

A receipt-count mismatch describes inconsistent data; it is not classified as physical pruning. The separate unavailable-history correction in point 95 remains closed.

Both existing zero-transaction fast paths remain unchanged: those queries skip receipt reads and return no logs for the empty block.

#### Query-Only Cost and Scope

The correction replaces the existing greater-than comparison with an inequality comparison. It performs no additional database reads, transaction replay or receipt-root calculation.

This check runs during historical log queries. It adds no work to normal block production, consensus or transaction execution.

It establishes receipt-count completeness. It does not authenticate receipt contents or detect corruption that preserves the expected count.

<!-- START ALPHA CHANGE 3525 -- document the accepted historical receipt-integrity performance tradeoff -->

#### Accepted Tradeoff: Receipt Counts Without Repeated Root Verification

Historical log queries deliberately validate receipt counts without recalculating each scanned block's receipt root.

The normal block-execution validation path already checks the calculated receipt root against the block header before publication. Historical RPC reads retain the assumption that subsequently stored receipt contents remain consistent with that validated commitment.

| Historical-read check | Work per nonempty block scanned |
| --- | --- |
| Exact receipt count — enforced | Compare the receipt-list and transaction-list lengths. |
| Receipt-root verification — optional future protection | Encode the consensus receipt contents and calculate their cryptographic trie root for comparison with the loaded block header. |

Root verification could detect alterations to committed receipt fields, including changed or missing logs, even when the receipt count remains correct. The existing root calculator can use the receipts and header already loaded, without additional database reads or transaction replay.

However, unconditional verification would repeat encoding and hashing during historical queries, consuming CPU and temporary memory on the RPC node. The current design avoids that recurring work. No measured overhead or performance improvement is claimed here; any future proposal to enable unconditional verification should first measure its cost under representative historical-query workloads.

The accepted limitation is explicit: local receipt inconsistency that preserves the expected count can still produce inaccurate historical log results. This review did not establish that ordinary MDBX writes or a host crash produce that condition. Count equality must not be described as proof that stored receipt contents match the header commitment.

This is an accepted read-time integrity tradeoff. The demonstrated incomplete-receipt-count defect remains closed. Receipt-root verification is optional additional protection and is not required for that closure.

The decision concerns historical RPC reads. It does not remove execution-time receipt-root validation or change consensus, block production or transaction execution.

<!-- FINISH ALPHA CHANGE 3525 -- document the accepted historical receipt-integrity performance tradeoff -->

#### Regression Coverage and Closure

Coverage in `jsonrpc/log_history_test.go` includes:

- `TestLogReceiptCount_RPCQueries`.
- `TestLogReceiptCount_EmptyBlocksSkipReads`.
- `TestLogReceiptCount_StoredRLP`.

The regressions verify rejection of empty, shorter and excessive lists; acceptance of complete lists with and without logs; failed-transaction receipts without logs; block-hash, range and retained-filter queries; and discarding logs collected before an inconsistent block.

The empty-block regression verifies that neither query path reads receipts for a zero-transaction block.

The stored-RLP regression writes actual MDBX receipt records containing zero, two, three and four receipts for a synthetic three-transaction block. It confirms successful production decoding before testing query rejection or acceptance. The empty record is written explicitly because the normal batch writer omits reconstructible empty receipt lists.

Shared query and fuzz fixtures now contain one receipt per transaction, using receipts without logs where needed. Their existing event counts remain unchanged.

Supplied validation passed the focused count, history, pruning, range-limit and null-filter regressions; twenty race-enabled repetitions; broader JSON-RPC, server and blockchain package suites; and vet.

The reported acceptance of decodable but incomplete receipt lists is closed. No Solidity changes, artifact regeneration or genesis regeneration are required.

<!-- FINISH ALPHA CHANGE 3523 -- document exact historical receipt-count validation -->

<!-- START ALPHA CHANGE 3549 -- document accepted server-construction cleanup limitation in the standalone deployment -->

### 105. Accepted Limitation: Cleanup After Failed Server Construction

`server.NewServer` acquires databases, networking resources and listeners incrementally. A later construction error returns `nil, err` without automatically running the complete cleanup sequence for previously acquired resources.

For example, JSON-RPC binding can fail after the operator listener and networking have started. A caller that remains in the same process receives no completed server handle to close, and previously acquired resources can remain open.

#### Supported Standalone Process Model

The supported packaged node runs as a standalone process under systemd.

The inspected command path in `command/server/server.go` calls `NewServer` once. If construction fails, `runServerLoop` returns the error. The command reports it and exits with status **1** in both text and JSON modes. It does not retry construction inside that process.

Process termination releases that process's sockets, file descriptors and database handles. The packaged `packaging/nodeclient/systemd/innbc-node@.service` specifies `Restart=always` and `RestartSec=5`, so automatic restart launches a fresh process rather than retrying inside the failed process.

Consequently, this constructor limitation does not cause resources owned by the terminated process to remain held across packaged service restarts.

#### Accepted Scope and Revisit Conditions

<!-- START ALPHA CHANGE 3551 -- explain why constructor cleanup refactoring is unnecessary for the current workflow -->

Automatic cleanup after failed server construction is not required by the current standalone CLI and systemd workflow. Startup failure terminates the process, releasing its operating-system resources before any service restart.

The ownership and partial-initialization cleanup refactor is therefore intentionally deferred to avoid unnecessary implementation complexity and regression risk. The constructor limitation remains accepted; it must be reconsidered if the supported process model changes.

The reported finding is closed by explicit acceptance within this deployment scope. The constructor itself has not been changed or made failure-cleanup-safe.

<!-- FINISH ALPHA CHANGE 3551 -- explain why constructor cleanup refactoring is unnecessary for the current workflow -->

Revisit this decision before supporting:

- Repeated construction attempts within one running process.
- Embedding the server in another long-lived application.
- Tests or tools that require recovery after failed construction without process termination.

This acceptance is separate from point 88, which documents orderly shutdown of a successfully constructed server. Process termination is not a claim that deferred application cleanup runs or that completed filesystem writes are rolled back.

No chain-state corruption was established by this finding. The networking constructor's own failure cleanup remains implemented.

This resolution changes documentation only. It introduces no runtime checks, startup ownership refactor or recurring consensus and transaction-processing work.

The assessment is based on read-only inspection of the command flow and packaged service definition. It does not establish the configuration or behavior of an installed service.

<!-- FINISH ALPHA CHANGE 3549 -- document accepted server-construction cleanup limitation in the standalone deployment -->

<!-- START ALPHA CHANGE 3560 -- document proposal electorates that retain votes cast before departure -->

### 106. Governance Approval Counts Current Validators And Recorded Departed Voters

Validator governance, treasury governance, block-reward governance, community-reward governance and proxy-upgrade governance use one effective electorate for proposal finalization.

The effective electorate contains every current active registry validator and every operator who already voted on that proposal, counting each address once. Both yes and no votes remain valid after their operator leaves the active registry.

The calculation uses existing proposal counters and the existing `hasVoted` mapping:

- Total votes are recorded yes votes plus recorded no votes.
- Active voters are current active registry validators whose `hasVoted` entry is true.
- Effective electorate size is current active count plus total votes minus active voters.
- Remaining votes are effective electorate size minus total votes.

Ordinary approval requires more than half of this electorate. Emergency reward approval requires unanimity of this electorate. An effective electorate of zero finalizes as rejected and cannot authorize an operation.

Removing a voter preserves that voter's ballot and place in the electorate. Removing a nonvoter removes that unused place. A newly active validator enters the electorate, and a readmitted voter is counted once and cannot vote twice.

Membership is evaluated when finalization executes. The existing voting deadline is preserved: admission after that deadline can increase the required threshold but does not reopen voting. Finalized decisions remain final.

Early approval, early rejection and final approval use the same effective electorate. There is no fallback that treats recorded votes exceeding the current active count as sufficient to settle the decision.

Emergency reward recipients remain the current active validators. Distribution still requires a nonempty current active list. Existing issuance percentages and cooldown rules are preserved.

The shared internal `INNBCGovernanceElectorate` library scans current registry membership once per finalization. It adds no persistent vote storage and requires no external library deployment. This work occurs when the finalization transaction executes on each validator; blocks without such calls acquire no additional membership scan.

The community reward pool uses the shared `IINNBCValidatorRegistry` interface. Public contract interfaces and existing storage layouts are preserved.

Regression coverage is provided by `core-contracts/test/INNBCGovernance.electorate.ts`, alongside the existing governance and funds tests. It covers departures, retained no votes, joins, readmission, deadlines, empty electorates, emergency unanimity and a representative 100-entry gas-bound case. That case does not establish a universal registry-size or execution-gas guarantee.

<!-- FINISH ALPHA CHANGE 3560 -- document proposal electorates that retain votes cast before departure -->

<!-- START ALPHA CHANGE 3565 -- document full-width EVM BYTE index validation and regression coverage -->

### 107. Full-Width EVM BYTE Index Validation

The `BYTE` opcode validates its complete unsigned 256-bit index before converting it to a machine-sized integer.

Previously, `state/runtime/evm/instructions.go` converted the index using `Int64()` before checking its range. Large indices could lose their upper bits and select a byte instead of returning zero.

For example, an index of `2^64` could truncate to zero. Applied to a word containing `0xAB` followed by 31 zero bytes, the old implementation returned `0xAB` instead of the required zero.

This affected active contract execution: an incorrect byte value could influence return data, branching or subsequent state changes. No specific deployed-contract exploit or consensus split between nodes running identical code was established.

#### Checked Index Conversion

`opByte` compares the full index against the existing `wordSize` value of 32.

- Indices 0–31 select the corresponding byte, counting from the most significant byte.
- Every index of 32 or greater returns zero without reverting.
- Conversion to `uint64` occurs only after establishing that the index is below 32.

The opcode retains its existing two-operand requirement, consumes those operands into one result, and preserves earlier stack entries. Registration and the three-gas opcode charge remain unchanged.

The correction adds no database reads, registry calls or additional execution passes. Its runtime performance was not separately benchmarked.

#### Regression Coverage and Validation

`state/runtime/evm/byte_index_test.go` verifies:

- Correct byte selection for all indices 0–31 using a word with distinct bytes.
- Zero results for indices 32, 33, `2^63`, `2^64`, `2^64 + 1`, `2^64 + 31` and `2^256 - 1`.
- The reported `2^64` example through complete bytecode execution.
- Stack preservation and exact gas accounting.
- Existing insufficient-gas and stack-underflow rejection.

Supplied validation passed the focused regressions, twenty race-enabled repetitions, broader tests covering `state/...`, `blockchain/...`, `consensus/innbcbft/...`, `command/genesis`, `server` and `jsonrpc`, and `go vet` over those packages.

<!-- FINISH ALPHA CHANGE 3565 -- document full-width EVM BYTE index validation and regression coverage -->

<!-- START ALPHA CHANGE 3568 -- document source-bound validation for zero-length return-data copies -->

### 108. Zero-Length RETURNDATACOPY Source-Bound Validation

The `RETURNDATACOPY` opcode validates its source offset even when the requested copy length is zero.

Previously, the zero-length shortcut returned before checking whether a representable source offset exceeded the return-data buffer. For example, an empty buffer with source offset 1 and length 0 could allow execution to continue instead of halting exceptionally.

#### Source Boundaries and Memory Handling

`opReturnDataCopy` in `state/runtime/evm/instructions.go` retains the existing source-offset representability check and validates the buffer boundary before accepting a zero-length copy.

A zero-length copy accepts a source offset at or before the buffer's end. An offset beyond the end produces `errReturnDataOutOfBounds`.

| Return-data size | Source offset | Copy length | Result |
| --- | --- | --- | --- |
| 0 | 0 | 0 | Succeeds |
| 0 | 1 | 0 | Exceptional halt |
| 10 | 10 | 0 | Succeeds |
| 10 | 11 | 0 | Exceptional halt |

Valid zero-length copies skip destination-memory validation and allocation. This includes a destination offset of `2^256 - 1`, because no destination bytes are accessed.

Invalid source offsets stop execution before subsequent instructions. The existing EVM wrapper consumes the remaining execution gas for this exceptional halt.

#### Execution Cost

The zero-length path performs one additional source-bound comparison. It performs no data copying, memory expansion or database access.

The opcode retains its three-operand requirement and three-gas base charge. Nonzero copies retain their existing memory-expansion, copy-gas and source-range checks.

#### Regression Coverage

`state/runtime/evm/return_data_copy_zero_test.go` covers:

- Source offsets at the start, inside, exactly at the end and beyond the return-data buffer.
- Empty and nonempty return-data buffers.
- Source offsets exceeding `uint64`, including the maximum unsigned 256-bit value.
- Valid zero-length copies with maximum-size destination offsets.
- Preservation of existing memory, memory-gas accounting and earlier stack entries.
- Actual bytecode execution and remaining-gas handling.
- Execution traces confirming that invalid copies stop before subsequent marker instructions.

The existing `Test_opReturnDataCopy` regressions also exercise nonzero copying, memory expansion, fork gating and invalid operands.

<!-- FINISH ALPHA CHANGE 3568 -- document source-bound validation for zero-length return-data copies -->

<!-- START ALPHA CHANGE 3572 -- document transaction receipt read failures and completeness validation -->

### 109. Transaction Receipt RPC Validates Read Success and Receipt Completeness

`eth_getTransactionReceipt` validates receipt availability and exact count completeness before indexing a block's receipt list.

Previously, receipt-read failures and empty lists could produce successful `null` responses after the transaction's block had been located. A shorter, successfully decoded list could also cause an out-of-bounds access when requesting a later transaction.

#### Lookup and Error Handling

`GetTransactionReceipt` in `jsonrpc/eth_endpoint.go` first locates the requested transaction in the loaded block.

Unknown transactions retain their successful `null` response. When the requested transaction is absent from the loaded block, the endpoint returns without reading receipts.

Once the transaction is found:

- Receipt-read failures return an error containing the block hash and preserving the underlying Go error cause.
- The decoded receipt count must equal the block's transaction count.
- Empty, shorter and excessive lists are rejected before receipt indexing or log-index calculation.

For example, a three-transaction block with two stored receipts produces:

`receipt count mismatch for block …: expected 3, got 2`

This applies even when requesting the first transaction, whose individual receipt might otherwise be accessible.

Existing pruned-block-body handling remains in place. A receipt-count mismatch identifies inconsistent data and is not automatically classified as physical pruning.

#### Complete Receipt Responses

Complete lists retain the existing receipt formatting, transaction metadata, status, gas fields and logs.

Receipts containing no logs remain valid, including failed-transaction receipts. Block-wide log indices count logs in preceding receipts; a receipt without logs does not advance that index.

#### Query Cost

The correction performs one length comparison after a successful receipt read. It adds no database reads, transaction execution or receipt-root calculation.

This validation runs during receipt RPC requests. It follows the count-completeness approach described in point 104 without recalculating stored receipt roots.

#### Regression Coverage

`jsonrpc/receipt_completeness_test.go` covers:

- Read failures and missing records returning errors with preserved causes.
- Empty, shorter and excessive lists rejected at every transaction position without panicking.
- JSON-RPC error responses without successful result data.
- Complete receipts preserving contents, status and cumulative log indices.
- Unknown transactions returning `null` without receipt reads, including stale lookup entries pointing to blocks that do not contain the requested transaction.
- Actual MDBX receipt records containing zero, two, three and four receipts, with successful stored-RLP decoding established before endpoint validation.

Existing receipt-price, receipt-formatting, pruning and historical-log regressions also exercise the surrounding behavior.

<!-- FINISH ALPHA CHANGE 3572 -- document transaction receipt read failures and completeness validation -->

<!-- START ALPHA CHANGE 3576 -- document receipt-count validation before live log-filter delivery -->

### 110. Live Log Filters Validate Receipt Completeness Before Delivery

Live log-filter delivery requires the decoded receipt count to equal the block's transaction count before processing any receipts.

Previously, a shorter list could deliver incomplete logs. An excessive list containing a receipt without a transaction hash could also reach transaction indexing beyond the block's transaction list.

#### Validation Before Filter Updates

`appendLogsToFilters` in `jsonrpc/filter_manager.go` checks exact count equality after successfully loading the block and before transaction indexing, transaction-hash reconstruction or log appending.

Empty, shorter and excessive lists are rejected when they do not match the transaction count. For example:

`receipt count mismatch for block …: expected 3, got 2`

A rejected block contributes no logs to any log filter. Logs already queued from earlier valid blocks remain intact.

Complete lists retain address/topic matching, transaction-hash reconstruction and block-wide log indices. Those indices count all logs, including logs that do not match a particular filter. Receipts without logs remain valid.

An empty block with zero transactions and zero receipts is also valid.

#### Background Processing and Failure Reporting

The background filter worker logs the returned mismatch error and continues processing subsequent blocks. Block-header notifications are handled independently of log delivery.

Rejecting an inconsistent receipt list does not repair the stored receipts, replay the missing logs or send subscribers an explicit gap notification.

#### Delivery Cost

Validation performs one length comparison per block reaching this stage while log filters are registered. It adds no database reads, receipt-root calculations or additional receipt-processing passes.

The comparison runs in the RPC filter worker and follows the count-completeness approach used by historical log queries and transaction-receipt lookups.

#### Regression Coverage

`jsonrpc/live_receipt_count_test.go` covers:

- Rejection of empty, shorter and excessive lists.
- Excess receipts with zero transaction hashes returning errors without panicking.
- Preservation of previously queued logs across multiple filters.
- Rejection before transaction-hash reconstruction or partial log appending.
- Complete delivery preserving matching, transaction hashes, transaction indices and block-wide log indices.
- Empty blocks with zero receipts.
- Continued background delivery after a malformed block has been processed and reported.
- Worker shutdown and cleanup.

The background regression waits for the malformed block's specific error report before submitting the next valid block, ensuring that subsequent delivery cannot conceal an unprocessed malformed event.

Existing log-delivery and null-filter regressions also exercise the surrounding polling and WebSocket behavior.

<!-- FINISH ALPHA CHANGE 3576 -- document receipt-count validation before live log-filter delivery -->

<!-- START ALPHA CHANGE 3582 -- document unused large offsets in zero-sized EVM memory ranges -->

### 111. Zero-Sized EVM Memory Ranges Accept Unused Large Offsets

Zero-sized EVM memory ranges accept the full unsigned 256-bit offset range because no memory bytes are accessed.

Previously, the shared `allocateMemory` helper checked whether the offset fit within `uint64` before checking the size. A zero-sized operation with an unused offset such as `2^64` could therefore halt exceptionally.

#### Zero-Size Handling Before Offset Validation

`allocateMemory` in `state/runtime/evm/state.go` checks for a zero size first and immediately succeeds.

This leaves existing memory contents, memory size, remaining gas and cached expansion accounting unchanged.

For nonzero sizes, the existing representability checks, operand limits, overflow checks and memory-expansion gas calculation still apply.

| Memory offset | Range size | Result |
| --- | --- | --- |
| `2^64` | 0 | Succeeds without memory expansion |
| `2^256 - 1` | 0 | Succeeds without memory expansion |
| `2^64` | 1 | Exceptional halt |
| `2^256 - 1` | 1 | Exceptional halt |

#### Copy and Call Output Ranges

`CALLDATACOPY`, `CODECOPY` and `EXTCODECOPY` accept unused large destination offsets when their copy length is zero.

`CALL`, `CALLCODE`, `DELEGATECALL` and `STATICCALL` likewise accept unused large output offsets when their output size is zero.

Call return data remains available in the return-data buffer even when no bytes are copied into the caller's memory.

Zero-sized ranges incur no memory-expansion charge. Opcode base charges and other applicable operation costs still apply.

#### Separate Return-Data Source Validation

Memory-offset handling does not bypass the `RETURNDATACOPY` source-bound checks described in point 108.

For a zero-length return-data copy, the source offset must still be at or before the return-data buffer's end. A source offset beyond that boundary produces an exceptional halt even though the destination memory offset is unused.

#### Regression Coverage

`state/runtime/evm/zero_memory_range_test.go` covers:

- Offsets `2^64` and `2^256 - 1` with zero and nonzero sizes.
- Preservation of existing memory and expansion accounting.
- Bytecode execution for all three affected copy opcodes.
- Caller-bytecode execution for all four call variants, with a mock callee returning nonempty data.
- Successful calls without output copying or premature memory expansion.
- Continued rejection of oversized offsets for nonzero ranges.
- Exact gas accounting.

The existing expectation in `state/runtime/evm/memory_gas_test.go` also accepts a zero-sized range with an offset exceeding `uint64`.

Memory-expansion and return-data-copy regressions exercise the surrounding overflow, gas and source-bound protections.

<!-- FINISH ALPHA CHANGE 3582 -- document unused large offsets in zero-sized EVM memory ranges -->

<!-- START ALPHA CHANGE 3585 -- document upfront CREATE2 hashing gas before endowment validation -->

### 112. CREATE2 Charges Hashing Gas Before Endowment Validation

`CREATE2` charges its upfront initialization-code hashing cost even when the executing contract cannot afford the requested endowment.

Previously, an insufficient-balance check returned before this charge. An unfunded creation attempt could therefore consume too little gas or continue execution where an exceptional out-of-gas halt was required.

#### Upfront Gas Ordering

`buildCreateContract` in `state/runtime/evm/instructions.go` charges hashing gas after memory validation and expansion, before checking the creator's endowment balance.

The hashing charge is `6 × ceil(init-code length / 32)`:

| Initialization-code length | Hashing gas |
| --- | ---: |
| 0 bytes | 0 |
| 1 byte | 6 |
| 32 bytes | 6 |
| 33 bytes | 12 |

The existing 32,000-gas opcode base charge remains applicable. Memory expansion, when required, is charged separately.

If the available gas cannot cover the hashing charge, execution halts exceptionally. If the gas is sufficient but the endowment balance is insufficient, creation returns zero without invoking child execution.

For example, with memory already allocated and 32 bytes of initialization code:

- 32,005 gas immediately before `CREATE2` is insufficient and causes an exceptional out-of-gas halt.
- 32,006 gas covers the upfront cost. An unfunded attempt returns zero, allowing a following zero-cost `STOP` to complete.

Ordinary `CREATE` does not acquire a hashing charge. Funded `CREATE2` attempts retain their existing charging and address-construction behavior.

#### Runtime Cost

The correction reorders existing checks. Unfunded attempts now perform the required gas calculation and deduction before returning.

It adds no hashing operation, database read, memory allocation or background work. Actual address hashing remains after the balance check.

#### Regression Coverage

`state/runtime/evm/create2_hash_gas_test.go` covers:

- The 32,005/32,006-gas boundary through actual opcode execution.
- Initialization-code lengths of 0, 1, 32 and 33 bytes.
- Exact gas consumption for funded and unfunded attempts.
- Zero results and absence of child execution for unfunded creation.
- Funded `CREATE2` and ordinary `CREATE` controls using a mock creation host.
- Preservation of creation arguments and expected addresses.

<!-- FINISH ALPHA CHANGE 3585 -- document upfront CREATE2 hashing gas before endowment validation -->

<!-- START ALPHA CHANGE 3592 -- document full-width unsigned GASLIMIT execution -->

### 113. Full-Width Unsigned EVM GASLIMIT Values

The EVM `GASLIMIT` opcode exposes the block header's gas limit as a nonnegative integer across the full supported `uint64` range.

Previously, gas limits above `MaxInt64` became negative when copied into the signed execution-context field. The opcode pushed that signed value directly. For example, a header limit of `18446744073709551615` became `-1`, and storing that stack value with `MSTORE` wrote its magnitude, `1`.

#### Unsigned Value Recovery

`opGasLimit` in `state/runtime/evm/instructions.go` converts the execution-context field back to `uint64` before pushing it with `SetUint64`.

The conversion recovers the original header value because the intermediate signed conversion preserves its 64-bit pattern. Gas-pool initialization already uses the same unsigned recovery.

| Header gas limit | Value exposed by GASLIMIT |
| --- | --- |
| `5242880` | `5242880` |
| `9223372036854775807` (`MaxInt64`) | `9223372036854775807` |
| `9223372036854775808` (`MaxInt64 + 1`) | `9223372036854775808` |
| `18446744073709551615` (`MaxUint64`) | `18446744073709551615` |

The opcode retains its two-gas charge and pushes one value without consuming earlier stack items. Existing configuration validation, default gas limits and gas-pool accounting remain unchanged.

The correction adds no database reads or additional execution passes.

#### Regression Coverage

`state/runtime/evm/gaslimit_word_test.go` verifies nonnegative stack values, preservation of earlier stack items and the exact two-gas charge. Cases include zero, one, the production default and the signed/unsigned boundaries.

`state/gaslimit_word_test.go` exercises the actual header-to-context construction and executes `GASLIMIT → MSTORE → RETURN` through transaction application. It verifies the exact 32-byte result, transaction gas consumption and remaining block gas-pool capacity.

<!-- FINISH ALPHA CHANGE 3592 -- document full-width unsigned GASLIMIT execution -->

<!-- START ALPHA CHANGE 3593 -- document SIGNEXTEND operand validation and rollback behavior -->

### 114. SIGNEXTEND Enforces Its Two-Operand Stack Requirement

The EVM `SIGNEXTEND` opcode requires two stack operands: the byte index and the value whose sign is extended.

Previously, its dispatcher registration required only one operand. An instruction reached with exactly one stack item could consume that item and return without recording an error, allowing execution to continue instead of halting exceptionally.

#### Operand Validation Before Execution

The opcode registration in `state/runtime/evm/dispatch_table.go` declares two required operands. The existing dispatcher rejects an empty or single-operand stack with `StackUnderflowError` before invoking `opSignExtension`.

This requirement applies to every byte index, including indices at or beyond the word boundary.

Valid execution retains the existing sign-extension arithmetic and five-gas charge. Two operands become one result, and earlier stack items remain intact.

#### Exceptional Halt and Call-State Rollback

Stack underflow halts the affected execution frame exceptionally. The frame's remaining gas is consumed, and existing call rollback restores changes made within that frame.

This includes storage writes performed before the invalid instruction and a value transfer into the failed call. Instructions after the underflow are not executed.

#### Runtime Cost

The correction changes the operand count used by the existing dispatcher comparison. It adds no database reads, memory allocations or additional execution passes.

#### Regression Coverage

`state/runtime/evm/signextend_stack_test.go` verifies:

- Empty-stack and single-operand rejection.
- Single operands containing zero, 32 and the maximum unsigned 256-bit value.
- Rejection before operand consumption.
- Positive and negative sign-extension results across indices 0–33.
- Preservation of earlier stack items.
- The unchanged five-gas opcode charge.

`state/signextend_stack_test.go` exercises the real call path. It verifies exceptional gas consumption, restoration of earlier storage contents, prevention of a later storage write and rollback of the call's value transfer. A valid two-operand control verifies successful execution and retained state changes.

<!-- FINISH ALPHA CHANGE 3593 -- document SIGNEXTEND operand validation and rollback behavior -->

<!-- START ALPHA CHANGE 3596 -- document common proposer-priority penalties for batch validator admissions -->

### 115. Common Proposer-Priority Base for Batch Validator Admissions

When multiple validators enter the consensus committee in the same update, each newcomer receives an initial proposer priority calculated from the same total voting-power base. Their admission transactions may occur separately before the epoch transition.

Previously, the newcomer calculation modified its shared `big.Int` working total. Each subsequent newcomer therefore used an increased base and could receive a larger negative penalty, introducing an unintended dependence on processing order.

#### Common Initial Penalty

`updateValidators` in `consensus/innbcbft/proposer_calculator.go` calculates:

```text
P = updated committee voting power + removed validators' voting power

initial newcomer priority = -(P + floor(P / 8))
```

The addition now uses a separate `big.Int` receiver, preserving P throughout the update. Each newcomer receives an independently mutable priority object.

For example, six existing validators plus two newcomers, each with voting power one, produce P = 8. Both newcomers receive an initial priority of -9. With existing priorities at zero, the subsequent centering produces +3 for each existing validator and -6 for both newcomers.

The existing rescaling, centering, address tie-breaking, voting powers and quorum thresholds remain unchanged.

#### Runtime Cost

The correction introduces one additional temporary `big.Int` per newcomer during validator-set updates. It adds no database reads, signature checks or additional iterations to ordinary proposer selection.

#### Regression Coverage

`consensus/innbcbft/proposer_batch_priority_test.go` verifies:

- Single-newcomer behavior.
- Six existing validators plus two newcomers.
- Larger batches without compounding penalties.
- Forward and reversed newcomer order.
- Additions combined with removals.
- Updates requiring priority rescaling.
- Replacement of all existing committee members.
- Expected normalized priorities.
- Unchanged individual and total voting powers.
- Independent newcomer priority objects.

<!-- FINISH ALPHA CHANGE 3596 -- document common proposer-priority penalties for batch validator admissions -->

<!-- START ALPHA CHANGE 3600 -- document address-list out-of-gas accounting and default-disabled configuration -->

### 116. Address-List Out-of-Gas Accounting

The optional deployment and transaction allow/block-list runtimes consume their entire supplied execution-gas allowance when an operation fails with `runtime.ErrOutOfGas`.

Previously, insufficient gas caused the operation to report an out-of-gas error while returning the full execution allowance as unused gas. Internal callers could recover that allowance, and transaction accounting could refund gas that should have been consumed.

#### Corrected Accounting

`AddressList.Run` in `state/runtime/addresslist/addresslist.go` now sets the consumed gas to the supplied allowance specifically when the operation returns `runtime.ErrOutOfGas`.

The resulting execution result preserves the error and reports:

- `GasUsed` equal to the supplied execution allowance.
- `GasLeft` equal to zero.

For internal calls, this consumes the gas forwarded to the failed child call. Gas retained by the parent remains available.

Successful reads retain their 5,000-gas charge. Successful writes retain their 20,000-gas charge. Other error types retain their existing accounting.

#### Default Genesis Configuration

All four optional access-control lists are disabled by default and remain disabled in the current genesis configuration:

- Contract deployment allowlist.
- Contract deployment blocklist.
- Transaction allowlist.
- Transaction blocklist.

The executor creates these runtimes only when their corresponding chain configurations are present. Consequently, the affected address-list runtime path is unreachable under the current genesis configuration.

The correction preserves this default-disabled policy and provides correct out-of-gas accounting for configurations that explicitly enable these lists.

#### Runtime Cost and Regression Coverage

The correction adds one comparison per address-list call and one assignment on an out-of-gas failure. It adds no database reads, allocations or additional processing passes. Disabled lists do not execute this function.

Regression coverage is provided by:

- `state/runtime/addresslist/out_of_gas_test.go`
- `state/addresslist_out_of_gas_test.go`

The regressions verify insufficient-gas reads and writes, exact-cost and surplus-gas controls, preserved roles after failure, unchanged static-write and authorization errors, and internal calls forwarding 4,999, 5,000 and 5,001 gas.

The internal-call regression checks the child success flag, total gas consumption and the parent's remaining gas.

<!-- FINISH ALPHA CHANGE 3600 -- document address-list out-of-gas accounting and default-disabled configuration -->

<!-- START ALPHA CHANGE 3603 -- document full-width legacy signature protection classification -->

### 117. Full-Width Legacy Signature Recovery for Large Chain IDs

Protected legacy transactions now retain the correct recovery path when their signature value `V` exceeds 64 bits.

Previously, `EIP155Signer.Sender` converted `V` to `uint64` before checking whether it represented an unprotected signature. A large protected value could truncate to 27 or 28 and incorrectly select unprotected recovery, causing a valid transaction to be rejected.

#### Corrected Classification

`EIP155Signer.Sender` in `crypto/txsigner_eip155.go` now checks the original `big.Int` bit length before converting it.

Only actual values `V = 27` or `V = 28` select unprotected recovery. Larger protected values remain on the EIP-155 path, where existing full-width arithmetic derives recovery parity zero or one.

For example, the supported chain ID `9223372036854775804` (`MaxInt64 - 3`) produces protected signature values:

- Parity zero: `V = 2^64 + 27`.
- Parity one: `V = 2^64 + 28`.

Both values now recover the correct sender instead of being misclassified after truncation. London's legacy fallback uses the same corrected signer.

#### Configuration and Compatibility

The production default chain ID **1982** was unaffected by this defect and retains its existing behavior.

The supported chain-ID range remains **1 through 9223372036854775807**. Genesis generation, configuration import and startup retain their positive-ID requirements.

Genuine unprotected transactions remain supported. Homestead high-S rejection and protected-transaction chain-ID mismatch rejection remain enforced.

The correction changes signature classification without adding a cryptographic operation, database access or persistent state.

#### Regression Coverage

`crypto/eip155_full_width_test.go` verifies:

- Both recovery parities at chain IDs 1, 1982, `MaxInt64 - 3` and `MaxInt64`.
- Preservation of full-width signature values through RLP encoding and decoding.
- Correct recovery through EIP-155, London's legacy fallback and `NewSigner`.
- Rejection when the configured chain ID does not match the protected signature.
- Preservation of transaction contents during recovery.
- Successful recovery of genuine unprotected low-S signatures.
- Continued Homestead rejection of unprotected high-S signatures.
- Preservation of the existing pre-Homestead high-S behavior.

<!-- FINISH ALPHA CHANGE 3603 -- document full-width legacy signature protection classification -->

<!-- START ALPHA CHANGE 3801 -- document consensus recovery local gossip delivery and runtime regression coverage -->

### 118. Consensus Recovery, Local Gossip Delivery and Runtime Regression Coverage

Consensus certificate validation and local gossip authorization now preserve legitimate operation across serialization, unavailable peer-directory responses and round changes. Additional regression tests exercise these paths alongside committee quorum, authorization, shutdown, restart and transaction-execution checks.

#### Prepared Certificates Across Protobuf Serialization

A prepared certificate contains the proposer's signed PREPREPARE and the additional PREPARE messages required to reach quorum.

For a one-validator committee, the proposer alone satisfies that quorum. Its prepared certificate therefore legitimately contains zero additional PREPARE messages.

Protobuf encodes nil and allocated-empty repeated lists equivalently. Decoding an empty prepare list produces a nil slice. Requiring that slice to be non-nil could reject valid singleton evidence after serialization and prevent recovery following a lost COMMIT.

The structural transport validator in `consensus/innbcbft/transport.go` and `validPC` in `consensus/innbcbft/ibftcore/ibft.go` now accept both representations of an empty prepare list.

The proposal remains mandatory. Every supplied prepare message still undergoes structural validation, and the core continues to enforce:

- Distinct message senders.
- Sufficient voting power under the existing quorum calculation.
- Correct proposer identity and validator authorization.
- Valid signatures.
- Consistent heights, rounds and proposal hashes.
- Consistency between the prepared proposal and its certificate.

For equal-power committees, the resulting requirements are:

| Validators | Required votes | Minimum additional PREPARE messages |
|---:|---:|---:|
| 1 | 1 | 0 |
| 2 | 2 | 1 |
| 3 | 3 | 2 |
| 4 | 3 | 2 |
| 6 | 5 | 4 |

The correction preserves these thresholds and the existing COMMIT quorum requirements.

#### Local Gossip While Directory Membership Is Pending

The shared `gossipAllowed` helper in `network/peer_admission.go` distinguishes local publication from remote networking admission.

A locally published message is eligible for delivery when:

- The immediate forwarder is the local libp2p host.
- The signed message author is that same local host.
- Peer admission has started.
- Peer admission has not closed.

These checks run under the existing admission read lock.

The shared helper applies at all three delivery stages:

1. GossipSub's default message validator.
2. The subscription reader before application queue admission.
3. The queued worker before callback delivery.

Local consensus publication can therefore proceed while an initial peer-directory exchange is pending or unsuccessful. The local path does not initialize directory membership, modify the allowlist or create a remote-peer session.

Remote messages and mixed local/remote identity pairs retain the existing membership checks. Remote connection admission, RPC authorization, revocation and terminal shutdown behavior remain enforced.

A one-validator committee can produce blocks without an available directory or remote peer. Larger committees continue to require sufficient validator votes and the connectivity needed to exchange them.

#### Runtime Cost

Certificate validation reuses the existing proposal, signer, context and voting-power checks.

Local gossip authorization adds a host-identity comparison and lifecycle check under the existing read lock. These corrections add no registry queries, database reads or additional EVM execution passes.

#### Regression Coverage

| Test file | Coverage |
|---|---|
| `consensus/innbcbft/prepared_certificate_transport_wire_test.go` | Nil and allocated-empty prepare lists, actual Protobuf round trips, nested round-change evidence, invalid list members and legitimate default-valued fields. |
| `consensus/innbcbft/ibftcore/prepared_certificate_wire_test.go` | Committees of 1, 2, 3, 4 and 6 validators; insufficient, exact and excess quorum; real ECDSA authorization; duplicate and unauthorized signers; malformed messages; incorrect heights, rounds and hashes. |
| `network/local_gossip_admission_test.go` | Actual local GossipSub publication while directory admission is pending, startup and shutdown boundaries, initialized membership, remote revocation, mixed identities, empty directory responses and queued delivery authorization. |
| `consensus/innbcbft/singleton_prepared_recovery_test.go` | Loss of the first round-zero COMMIT, the production round timeout, serialized prepared ROUND_CHANGE evidence, reproposal of the original block, valid commit seals and exactly one insertion without rebuilding or re-executing the prepared proposal. |
| `server/single_validator_directory_test.go` | Real-server startup with the directory disabled or unavailable, epoch-boundary progression, a funded transaction, successful manual refresh, restart with the directory unavailable, retained receipts and preserved genesis and identity files. |
| `command/root/committee_liveness_test.go` | Healthy CLI workflows with 1, 2, 3, 4 and 6 validators, transaction inclusion, epoch transitions, consistent canonical block hashes, nonce and balance results, and preserved identities and genesis. |
| `server/snapshot_integration_test.go` | Additional block-1 snapshot cases with retention exceeding chain age, covering segmented and checkpoint activation, continued storage writes and reopening. |
| `jsonrpc/throttling_test.go` | Confirmed occupation of five request slots, rejection without handler invocation, configured and parent-context acquisition deadlines, explicit slot release, replacement admission and subsequent capacity reuse. |

The singleton prepared-recovery test runs through both serialized loopback and actual GossipSub with an unavailable directory. It validates production consensus processing, ECDSA signatures, checkpoint hashes and BLS commit seals using controlled block construction and persistence.

The full-server fixture exercises real MDBX storage, matching validator identities, genesis initialization, consensus, txpool admission and committed receipts. The CLI matrix separately exercises complete command-driven node workflows with an available directory.

#### Synchronized Throttling Test

`TestThrottling` confirms handler entry through channels before asserting that all five slots are occupied. Admitted handlers remain blocked until explicitly released, so rejection and replacement checks operate on a known capacity state.

Worker results use buffered channels. Assertions execute in the main test goroutine, and cleanup cancels outstanding work and joins the workers with a watchdog.

The fixture uses a five-second acquisition timeout and a shorter 250-millisecond parent deadline. These are test parameters; production throttling configuration and implementation remain unchanged.

#### Validation

Verification completed with:

- The focused certificate, local-gossip, singleton-recovery, server-lifecycle and snapshot regressions.
- The broader networking, consensus, execution-state, blockchain, genesis, server and JSON-RPC package suites.
- Five repeated race-detector runs across the selected core, runtime, networking and server regressions, including existing peer-admission, peer-session and gossip-lifecycle controls.
- `go vet` across the networking, consensus, execution-state, blockchain, genesis, server and JSON-RPC packages.
- The CLI committee matrix with 1, 2, 3, 4 and 6 validators.
- Twenty ordinary throttling-test runs and forty race-detector runs across two repeated executions.

<!-- FINISH ALPHA CHANGE 3801 -- document consensus recovery local gossip delivery and runtime regression coverage -->

<!-- START ALPHA CHANGE 3804 -- document bounded execution-local BLOCKHASH caching and regressions -->

### 119. Bounded Ancestor Caching for EVM BLOCKHASH

`Blockchain.GetHashHelper` in `blockchain/blockchain.go` now retains ancestor hashes within each execution transition.

Previously, every lookup restarted traversal from the candidate block's parent. Repeated requests near the oldest permitted ancestor could traverse 255 intermediate headers per request and repeatedly evict entries from the shared 100-entry header cache.

#### Incremental Ancestor Lookup

Each helper captures the supplied header's number and parent hash when constructed. Its independent cache is allocated only when a valid ancestor is requested.

The helper:

- Returns the captured parent hash without reading an ancestor header.
- Stores every intermediate ancestor hash encountered during traversal.
- Returns previously discovered hashes directly.
- Extends traversal only when a request requires older, uncached ancestry.
- Follows parent hashes, preserving the supplied header's ancestry.

Each execution transition owns its helper and invokes it serially. Separate transitions maintain independent caches, including when anchored to different parent chains.

#### Bounded Work and Storage

Requests are restricted to the preceding 256 blocks. Range checks occur before subtraction, preventing unsigned underflow for genesis, current-block, future-block and oversized requests.

For healthy, readable ancestry:

| Property | Bound |
|---|---|
| Retained hashes per helper | At most 256 |
| Raw retained hash data | At most 8 KiB |
| Successful ancestor-header lookups per helper | At most 255 |
| Additional header reads for an already cached result | Zero |

These bounds apply across repeated and differently ordered requests throughout the helper's lifetime.

Eviction from the shared header cache does not discard the helper's discovered hashes. The shared cache size and the opcode's 20-gas charge remain unchanged.

If an ancestor header cannot be read, cache extension stops while retaining the known prefix. Subsequent requests can retry the missing header; failed lookups do not create cached zero-hash entries.

#### Regression Coverage

`blockchain/blockhash_cache_test.go` contains deterministic regressions:

| Test | Coverage |
|---|---|
| `TestBlockHashCache_RepeatedOldest` | Repeated oldest-window lookups retain the initial 255-read count after shared-cache eviction. |
| `TestBlockHashCache_IncrementalMixedRequests` | Mixed lookup depths reuse intermediate hashes and read only newly required ancestry. |
| `TestBlockHashCache_WindowAndEarlyBlocks` | Genesis, early heights, the complete permitted window and invalid requests without additional reads. |
| `TestBlockHashCache_IndependentAnchors` | Separate parent chains retain independent results; later mutation of the supplied header does not change a helper's captured anchor. |
| `TestBlockHashCache_FailedReadPreservesPrefixAndRetries` | A failed read preserves discovered ancestry, remains retryable and resumes after the missing header is restored. |
| `TestBlockHashCache_BytecodeResultsGasAndReads` | Actual EVM bytecode executes 1,001 BLOCKHASH instructions, returns the expected 32-byte hash and asserts 25,038 execution gas with only 255 backend header reads. |

The read-count assertions measure backend calls directly rather than relying on elapsed-time thresholds. The bytecode gas assertion excludes transaction intrinsic gas.

<!-- FINISH ALPHA CHANGE 3804 -- document bounded execution-local BLOCKHASH caching and regressions -->

<!-- START ALPHA CHANGE 3814 -- document error-aware BLOCKHASH execution and regression coverage -->

### 120. BLOCKHASH Ancestor-Read Failures Reject Execution

EVM block-hash lookups now distinguish normal zero-hash results from failures reading required ancestry.

Previously, a failed ancestor-header read could become a zero hash without recording an execution error. Contract execution could then continue and produce an apparently successful result based on unavailable local data.

#### Error-Preserving Ancestor Reads

`blockchain/blockchain.go` provides an error-returning cached header reader. It preserves underlying storage errors and returns controlled errors for invalid cached entries or a nil header returned without an error.

`Blockchain.GetHashHelper` carries these errors through its callback using contextual wrapping that preserves the original cause.

| Lookup condition | Behavior |
|---|---|
| Parent hash or previously discovered ancestor available | Returns the known hash without another header read |
| Valid ancestor successfully retrieved | Returns its hash and retains the discovered ancestry |
| Current block, future block or request outside the preceding 256 blocks | Returns zero without an error |
| Required ancestor cannot be read | Returns an error preserving the underlying cause |

A zero hash alone is not classified as a storage failure.

#### Rollback-Resistant Execution Failure

The executor's internal hash callback returns both a hash and an error. `Transition.GetBlockHash` records lookup errors through the existing local-state failure mechanism.

The retained failure:

- Is classified as `ErrLocalStateFailure`.
- Preserves the original error cause.
- Survives contract rollback and REVERT.
- Cannot be suppressed by ignoring a failed internal call.
- Prevents acceptance of transaction gas accounting and receipts.
- Prevents state publication through commit.

Existing execution boundaries restore transaction changes, including storage, nonce and transferred value. An unavailable ancestor therefore does not become either a successful transaction or an ordinary failed-transaction receipt.

Once the transition has retained a failure, subsequent block-hash requests do not invoke its lookup callback again.

The standalone ancestor helper remains retryable and retains its verified prefix after a failed read. A transition that encountered that failure remains rejected; retrying execution requires a fresh transition.

#### Offline Replay

The block-hash provider in `server/trie_rebuild.go` uses the same error-aware callback signature.

Out-of-window requests retain their normal zero result. Missing required entries inside the valid replay window produce controlled errors instead of silently returning zero.

#### Runtime Cost and Cache Bounds

Healthy lookups reuse the existing shared header cache and the execution-local ancestor cache. Error propagation adds no database read or additional execution pass.

The opcode's 20-gas charge and the bounds documented in point 119 remain unchanged: at most 256 retained hashes and 255 successful ancestor-header lookups per helper.

#### Regression Coverage

| Test file | Coverage |
|---|---|
| `blockchain/blockhash_read_failure_test.go` | Backend error causes, missing headers, stored-header hash mismatch errors, normal zero results, invalid cache entries and nil backend headers. |
| `state/blockhash_failure_test.go` | Actual bytecode execution, rollback of preceding changes, failure retention through REVERT and nested calls, blocked receipts and commit, and successful ordinary results. |
| `blockchain/blockhash_cache_test.go` | Preserved cache bounds, explicit lookup errors, prefix retention, repair retries, independent ancestry and unchanged bytecode results and gas. |
| `server/trie_rebuild_test.go` | Executable-history replay using the updated block-hash provider. |

#### Validation

Verification completed with:

- The focused block-hash cache, header-read failure, execution failure and executable-history replay regressions.
- Twenty repeated race-detector runs of the selected regressions across the blockchain, state and server packages.
- The broader blockchain, state, INNBCBFT consensus, genesis, server, JSON-RPC, synchronization and validator package suites.
- `go vet` across those broader packages.

<!-- FINISH ALPHA CHANGE 3814 -- document error-aware BLOCKHASH execution and regression coverage -->

<!-- START ALPHA CHANGE 3820 -- document guarded EVM balance words and execution regressions -->

### 121. EVM Balance Reads Reject Unrepresentable Values

BALANCE and SELFBALANCE validate host-provided balances before placing them on the EVM stack.

Go's native-balance representation uses big.Int, which can hold values exceeding the EVM's unsigned 256-bit word. Previously, an oversized balance could enter the stack and cause a bounds panic when MSTORE attempted to serialize it into a fixed 32-byte buffer.

Both handlers in `state/runtime/evm/instructions.go` now reject unrepresentable values with the controlled execution error `balance outside unsigned 256-bit range`.

#### Accepted Values and Failure Behavior

| Host-provided balance | Result |
|---|---|
| Zero | Accepted |
| Positive value through 2^256 - 1 | Accepted without truncation |
| Value of 2^256 or greater | Exceptional halt before stack insertion |
| Negative value | Exceptional halt before stack insertion |
| Nil balance | Exceptional halt before stack insertion |

The failed frame consumes its execution allowance through the existing exceptional-halt handling. Existing call rollback restores that frame's storage changes and value transfer.

The error is an ordinary EVM execution failure. It does not set `ErrLocalStateFailure`, and a subsequent healthy call can continue using the transition.

<!-- START ALPHA CHANGE 3821 -- distinguish existing issuance limits from EVM balance-read protection -->

#### Supply Limit and Runtime Balance Protection

The native-token contract and the EVM guard protect different boundaries:

| Protection | What it checks | Failure behavior |
|---|---|---|
| Existing native-token issuance limit | Whether increasing `_totalSupply` would exceed the uint256 maximum | The issuance reverts |
| EVM balance-read guard | Whether an individual balance returned to BALANCE or SELFBALANCE fits an unsigned 256-bit word | The execution frame halts exceptionally |

`INNBCNativeToken._issue()` already performs `_totalSupply += amount` using Solidity's checked arithmetic. Issuance that would increase reported supply beyond 2^256 - 1 wei reverts before the corresponding native balance is credited.

The EVM guard checks individual balance reads. It does not calculate total supply, authorize issuance or impose a new minting limit.

Reported `totalSupply()` and the sum of all runtime account balances are distinct under the existing accounting rules:

- Genesis excludes the zero-address allocation from reported initial supply.
- Burning reduces `_totalSupply` while transferring native balance into the zero address.
- Subsequent issuance can create new native balances while previously burned balances remain recorded at zero.

An oversized zero-address balance can therefore coexist with reported supply that remains within uint256. The balance-read guard handles that condition without allowing an oversized value onto the EVM stack.

Together, these protections enforce the existing reported-supply limit during issuance and prevent unrepresentable balance reads from causing the reported EVM panic. The runtime guard leaves stored balances intact; it does not add a general limit to every native-balance credit.

<!-- FINISH ALPHA CHANGE 3821 -- distinguish existing issuance limits from EVM balance-read protection -->

#### Gas and Fork Behavior

BALANCE retains its existing fork-specific charges:

- 20 gas before EIP-150.
- 400 gas with EIP-150.
- 700 gas with Istanbul.

SELFBALANCE retains its five-gas charge and remains unavailable before Istanbul.

Gas availability is checked before reading the host balance. Each enabled opcode fetches the balance once, then checks its presence, sign and width. The guard adds no database read or additional execution pass and does not modify the host-provided integer.

#### Regression Coverage

| Test | Coverage |
|---|---|
| `TestBalanceWord_ExecutionStackAndHostValue` | Actual bytecode execution for both opcodes; zero, one, maximum, oversized, negative and nil values; MSTORE output; stack preservation; host-value immutability; exact valid execution gas and exceptional-halt accounting. |
| `TestBalanceWord_GasBeforeHostRead` | Exact and insufficient gas allowances, including rejection before any balance read when gas is insufficient. |
| `TestBalanceWord_SelfBalanceBeforeIstanbul` | Preservation of the activation rule without invoking the balance reader. |
| `TestBalanceWord_GenesisOverflowCallRollback` | Real memory-backed trie state, accepted maximum zero-address genesis balance, one-wei overflow, controlled BALANCE and SELFBALANCE failures, storage and value rollback, and successful subsequent calls. |

The opcode tests reside in `state/runtime/evm/balance_word_test.go`. The genesis and call-execution regression resides in `state/balance_word_execution_test.go`.

For BALANCE, the integration regression preserves a separate successful transfer that created the oversized zero-address balance, while rolling back the later failed contract call.

For SELFBALANCE, the call's value transfer temporarily creates the oversized balance. The failed call restores the preceding maximum balance and storage.

#### Validation

Verification completed with:

- The focused balance-word opcode and state-execution regressions.
- Twenty repeated race-detector runs across the EVM and state regressions.
- The broader state, blockchain, INNBCBFT consensus, genesis, server and JSON-RPC package suites.
- `go vet` across those broader packages.

<!-- FINISH ALPHA CHANGE 3820 -- document guarded EVM balance words and execution regressions -->

<!-- START ALPHA CHANGE 3824 -- document overflow-safe txpool pressure thresholds and regressions -->

### 122. Overflow-Safe Transaction-Pool Pressure Threshold

The transaction pool calculates its high-pressure threshold without overflowing when an operator configures an exceptionally large `--max-slots` value.

A slot accounts for 32 KiB of transaction data, rounded up. The configured capacity is an accounting limit; setting a large value does not preallocate that entire capacity.

Previously, `slotGauge.highPressure()` calculated `(80 * maxSlots) / 100` using uint64 arithmetic. The multiplication could overflow before division.

For example, with `maxSlots = 2^62`, the multiplication wrapped to zero. Any positive occupancy then incorrectly reported high pressure, allowing a nearly empty pool to reject future-nonce transactions and request pruning.

#### Corrected Calculation

`txpool/slot_gauge.go` separates the capacity into its quotient and remainder before multiplying:

`threshold = (maxSlots / 100) * 80 + ((maxSlots % 100) * 80) / 100`

All divisions are integer divisions. This preserves the exact mathematical result `floor(80 * maxSlots / 100)` throughout the uint64 capacity range without overflowing intermediate operations.

High pressure remains defined as occupied slots strictly greater than that threshold.

| Configured capacity | Correct threshold |
|---|---:|
| 4,096 | 3,276 |
| 2^62 | 3,689,348,814,741,910,323 |
| MaxUint64 | 14,757,395,258,967,641,292 |

At the default capacity, pressure therefore begins at 3,277 occupied slots. Existing default behavior and rounding are preserved.

#### Admission and Pruning Behavior

The existing admission path checks pressure before inserting the incoming transaction:

- Occupancy exactly at the threshold does not activate pressure restrictions.
- Occupancy above the threshold requests pruning and rejects transactions whose nonce exceeds the account's expected nonce.
- Transactions matching the expected nonce remain eligible for admission under the existing validation and capacity checks.
- Pruning removes queued transactions with nonce gaps while retaining transactions matching the expected nonce.

The correction prevents overflow from activating those restrictions prematurely.

#### Configuration and Runtime Scope

The original defect required an extreme custom capacity. It was not reachable with the default 4,096-slot configuration.

The change affects local transaction-pool admission and pruning decisions. It does not change block validity, consensus rules, transaction execution or gas accounting.

The production correction adds only fixed-width integer arithmetic to the existing pressure check. It introduces no allocation, database read or additional processing pass.

This correction covers the pressure-threshold calculation; it does not change the other slot-accounting operations.

#### Regression Coverage

The regressions reside in `txpool/slot_pressure_test.go`.

| Test | Coverage |
|---|---|
| `TestSlotPressure_ThresholdBoundaries` | Independent arbitrary-precision reference calculation; small capacities, default capacity, multiplication-overflow boundaries, 2^62 and MaxUint64; occupancy below, at and above the threshold. |
| `TestSlotPressure_LargeCapacityAdmission` | Actual admission of two one-slot future-nonce transactions at low occupancy, preserved transaction indexes and expected nonces, and absence of erroneous pruning requests. |
| `TestSlotPressure_AdmissionAndPruningBoundaries` | Admission at the exact threshold, future-nonce rejection above it, expected-nonce admission under pressure, pruning signals, nonce-gap removal, account cleanup and preservation of the expected-nonce transaction. |

The admission and pruning regressions cover capacities of 4,096, 2^62 and MaxUint64.

Large occupancy is modeled through the gauge without allocating a correspondingly large transaction collection. Buffered pruning notifications and direct invocation of the existing pruning handler avoid timing-dependent background-worker assertions.

#### Validation

Before the production correction, the new regressions reproduced incorrect thresholds and premature future-nonce rejection.

After the correction, verification completed with:

- All three new regressions and the existing `TestAddTxHighPressure` tests.
- Twenty repeated race-detector runs of those selected tests.
- The broader transaction-pool, server-command, server and INNBCBFT consensus package suites.
- `go vet` across those broader packages.

<!-- FINISH ALPHA CHANGE 3824 -- document overflow-safe txpool pressure thresholds and regressions -->

### 123. Deliberate Exclusion Of EIP-7702 Account Delegation

INNBC Chain deliberately excludes EIP-7702 as a security policy. This keeps protocol-level delegation of ordinary wallet accounts outside the supported transaction model and removes the associated malicious-delegation attack path.

EIP-7702 allows an externally owned account (EOA) to authorize execution of another contract’s code in its own account context. Delegated code can operate on the account’s assets and exercise permissions that other contracts have granted to that account. The authorization requires a valid signature, but its effects can persist beyond the transaction that installs it.

**Security rationale**

The Binance incident of 29 October 2026 illustrates the risk of malicious account delegation. TRM Labs reported that the attacker used 50 presigned EIP-7702 authorizations to make an Ethereum hot wallet execute attacker-controlled sweeper code. This enabled automated asset transfers through the compromised account. TRM also reported ordinary signed transfers on other chains, so EIP-7702 was one component of the incident rather than the cause of every loss.

Binance’s preliminary update on 30 October stated that it had found no evidence of private-key leakage and was investigating compromised infrastructure. A private key does not have to be extracted for an attacker to misuse a system capable of producing valid signatures. The incident’s complete root cause remained under investigation at the time of that update.

**Enforcement in INNBC Chain**

The production client does not implement EIP-7702 authorization lists, delegation installation or delegated account-code resolution.

Its supported transaction types are:

- Legacy transactions: `0x00`.
- Dynamic-fee transactions: `0x02`.
- Internal system state transactions: `0x7f`, excluded from ordinary transaction-pool admission.

EIP-7702 uses transaction type `0x04`. INNBC rejects that type during transaction decoding rather than accepting or silently ignoring its authorization data. The relevant implementation is in `types/transaction.go`, `types/rlp_unmarshal.go` and `txpool/raw_transaction.go`.

**Scope and compatibility**

This policy prevents an EIP-7702 authorization from turning an ordinary INNBC wallet into an executable delegated account. It also excludes legitimate EIP-7702 wallet features, including delegation-based batching and sponsored execution.

Ordinary smart contracts, contract proxies and supported contract-call operations remain available. This restriction does not prevent theft through compromised signing infrastructure, malicious token approvals or vulnerable contracts; protection of wallet keys and signing permissions remains essential.

<!-- START ALPHA CHANGE 3832 -- document startup peer readiness following synchronized committee updates -->

### 124. Startup Peer Requirements Follow Committee Updates

The consensus startup wait refreshes its required peer count from the current runtime epoch committee before each peer-count comparison.

Synchronization runs concurrently with this wait and can publish a different committee after processing an epoch transition.

Previously, `waitForValidatorPeers()` calculated its requirement only once. A recovering node could therefore synchronize successfully into a smaller committee while continuing to wait for the larger committee's peer requirement.

For example, a node starting from a six-validator committee required two remote peers. After synchronization established a two-validator committee, that node could remain blocked despite being connected to its sole remaining counterpart.

#### Current Startup Thresholds

| Current epoch committee size | Required remote peers |
|---|---:|
| One validator | Zero |
| Two validators | One |
| Three or more validators | Two |

These thresholds govern startup readiness. They do not determine or weaken the prepare or commit quorums required by consensus.

A shrinking committee can now release the startup wait once its current requirement is satisfied. A growing committee can raise the requirement while the node is still waiting.

The initial single-validator fast path remains available.

#### Runtime Behavior and Cost

The correction in `consensus/innbcbft/innbcbft.go` reuses `requiredValidatorPeers()` and its existing runtime read lock.

Recalculation occurs during each existing two-second waiting iteration, immediately before comparing the connected peer count. It introduces no additional database read, registry contract call, signature verification or background worker.

Normal shutdown and fatal-error signals continue to terminate the wait.

The peer directory retains its existing role in supplying peer addresses and maintaining network admission. Startup requirements come from the runtime committee. This correction requires no change to directory exchange or refresh behavior.

#### Regression Coverage

The regressions reside in `consensus/innbcbft/startup_peer_wait_test.go`.

| Test | Coverage |
|---|---|
| `TestStartupPeerWait_Thresholds` | Existing requirements for committees of one, two, three, four and six validators. |
| `TestStartupPeerWait_CommitteeShrinks` | Six-to-two transition with one registered peer, and six-to-one transition with zero peers; startup resumes after the requirement changes. |
| `TestStartupPeerWait_CommitteeGrows` | Two-to-six transition while waiting; the requirement increases to two peers, and one peer remains insufficient. |
| `TestStartupPeerWait_StableCommittees` | Successful startup readiness for unchanged committees of one, two and six validators. |
| `TestStartupPeerWait_Cancellation` | Normal shutdown and fatal-stop termination while waiting. |

The tests invoke the actual startup waiting function. Committee replacement uses the runtime lock, and peer counts are controlled through registered networking peer records.

The existing waiting log provides a synchronization point after an unsuccessful poll. Unbuffered observations coordinate subsequent committee changes without fixed sleeps or a production test hook.

Cleanup cancels and joins the waiting goroutine before closing the networking fixture.

These focused tests exercise startup readiness directly. They do not perform governance removals, download historical blocks or execute real peer handshakes.

#### Validation

Before the production correction, the shrinking-committee and growing-committee regressions reproduced the stale requirement. Unchanged-threshold, stable-committee and cancellation checks passed.

After the correction:

- All focused `TestStartupPeerWait_` regressions passed.
- Twenty repeated race-detector runs of those regressions passed.

<!-- FINISH ALPHA CHANGE 3832 -- document startup peer readiness following synchronized committee updates -->

<!-- START ALPHA CHANGE 3838 -- document sync stream sequence enforcement and peer fallback regressions -->

### 125. Synchronization Rejects Non-Progressing Block Streams

`bulkSyncWithPeer` in `syncer/syncer.go` requires each received block to be the next height in the requested download sequence.

Previously, the synchronization consumer silently skipped block zero while the transport renewed its per-block timeout after forwarding each decoded block. An admitted peer advertising a higher head could repeatedly send genesis blocks without advancing synchronization or allowing peer fallback.

This scenario required a malicious or compromised peer passing the existing network admission rules. An arbitrary public RPC client could not directly exercise this path. A small, trusted operator set reduces the exposure but does not replace stream validation.

#### Sequence Enforcement

The download captures the local starting height and requests its successor. Each received block is checked before finalized-block verification, storage import or callback execution.

The check rejects:

- Genesis blocks.
- A first block that does not match the requested starting height.
- Repeated or backward block numbers within the same stream.
- Gaps between successive blocks.
- A zero block following the maximum uint64 height.

The comparison excludes zero before subtracting one from the received number, avoiding arithmetic underflow and avoiding an overflowing successor calculation.

A rejected frame contributes no verification, import or callback work. Successfully imported blocks from the preceding valid portion remain available.

The existing deferred cleanup cancels the offending stream. If the peer has not delivered its advertised head, existing incomplete-download handling temporarily excludes it from that selection pass and tries another eligible peer.

#### Compatibility and Cost

Sequence validation follows the original download request rather than the node's potentially newer current head. A legitimate sequential download therefore remains usable when consensus independently imports the same heights during synchronization.

A valid stream may also extend beyond the peer's earlier advertised head. The serving node captures its current target when handling the request, which can be newer than its preceding status advertisement.

Existing finalized-block verification, duplicate-import behavior and synchronization callbacks remain in place. The accepted synchronization callback identity limitation documented in point 59 is unchanged.

The correction adds a constant-time height check per received block. It introduces no additional database read, signature verification, persistent state or background worker. Existing timeout behavior and peer-directory admission remain unchanged.

#### Regression Coverage

The additional tests are in `syncer/stream_sequence_test.go`.

| Test | Coverage |
|---|---|
| `TestSyncStreamSequence_RejectsMalformed` | Genesis, incorrect initial heights, repetition, backward movement, gaps and the maximum-height boundary; offending frames are rejected before verification, writes or callbacks. |
| `TestSyncStreamSequence_PreservesValidDownloads` | Sequential downloads from zero and nonzero heads, delivery beyond an earlier advertised head, modeled local-head advancement after the request, and the maximum valid height. |
| `TestSyncStreamSequence_OpenMalformedRPCFallsBack` | The preferred peer sends genesis and withholds stream completion; synchronization cancels that RPC and imports blocks from a healthy peer. |

The RPC regression uses the actual synchronization client and RPC transport with controlled verification and storage backends. Its one-hour operational block timeout and short test watchdog distinguish prompt sequence rejection from ordinary timeout recovery.

The existing multi-peer fixture in `syncer/syncer_test.go` now reports its updated imported head. Its final progression expectation is block five because the first peer imports blocks one through four before the second download resumes at five.

#### Validation and Deployment

The focused `TestSyncStreamSequence_`, `TestSync` and `Test_bulkSyncWithPeer` checks passed. Twenty repeated race-detector runs of the same selection also passed.

These results cover the focused test selection; they do not record a broader package test or vet run.

Deploying the runtime correction requires rebuilding and distributing the node binary. It requires no Solidity change, contract-artifact regeneration, new genesis, identity replacement or peer-directory configuration change.

<!-- FINISH ALPHA CHANGE 3838 -- document sync stream sequence enforcement and peer fallback regressions -->

<!-- START ALPHA CHANGE 3852 -- document compatible genesis bootstrap identity generation and validation -->

### 126. Genesis Generation Preserves Peer-Directory Bootstrap Identities

Both supported validator-input workflows now produce metadata compatible with the peer directory's bootstrap peer-ID extraction.

Previously, directory/prefix loading discarded the loaded libp2p identity and omitted each validator's `MultiAddr`. Genesis generation could succeed, but the directory subsequently rejected the empty bootstrap metadata.

Explicit validator input also accepted syntactically valid transport addresses without a peer-ID component, although the downstream directory parser required that identity.

The original defect did not affect deployments using complete explicit validator entries containing `/p2p/<peer-id>`.

#### Directory and Prefix Loading

When explicit `--validators` entries are omitted, `ReadValidatorsByPrefix` in `command/genesis/utils.go` retains the networking identity loaded from each selected validator directory.

Each resulting validator record includes:

- Its existing operator address.
- Its existing BLS public key and generated possession proof.
- `MultiAddr` containing `/p2p/<existing-peer-id>`.

A missing networking identity causes a descriptive generation error. The reader does not create a replacement networking key.

The identity-only representation supplies bootstrap authorization metadata without inventing an IP address or port. The directory's `buildBootstrapPeerIDs` helper extracts the peer ID from that representation.

#### Explicit Validator Input

`getValidatorAccounts` in `command/genesis/innbcbft_params.go` uses the same `network/common.StringToAddrInfo` parser as directory bootstrap extraction.

Accepted inputs include complete IPv4, IPv6 and DNS multiaddresses containing a valid peer identity, as well as identity-only `/p2p/<peer-id>` metadata.

Empty addresses, malformed peer identities and transport-only addresses without a peer identity are rejected during generation.

Valid supplied address strings are retained. Parsing the trailing operator, BLS-key and proof fields from the right continues to preserve IPv6 addresses containing colons.

#### Bootstrap Identity Versus Advertised Address

Genesis bootstrap metadata identifies which peers are initially authorized. It does not itself need to provide a dialable transport address.

Live directory exchange, persisted peer records and discovery retain their existing transport-address requirements. An identity-only genesis entry does not authorize publication of an addressless directory advertisement or bypass the existing identity and signature checks.

Nodes must still advertise their actual connection addresses through the normal directory workflow.

#### Regression Coverage

The additional regressions are in:

- `command/genesis/bootstrap_peer_test.go`.
- `command/peerdirectory/bootstrap_peer_test.go`.

Coverage includes:

- Direct prefix loading and selection of that workflow when explicit validators are absent.
- Preservation of the existing peer ID through validator JSON serialization.
- Correct operator address, BLS public key and valid possession proof.
- Unchanged stored ECDSA, BLS and networking key bytes.
- Controlled rejection of a missing networking key without generating a replacement.
- Acceptance of complete IPv4, IPv6, DNS and identity-only metadata.
- Rejection of empty, malformed and transport-only explicit inputs.
- Directory bootstrap extraction from identity-only and mixed address forms.
- Existing peer-ID deduplication and rejection without returning partial allowlists.
- Continued rejection of addressless directory advertisements and persisted peer records.

Existing genesis-generation fixtures now supply complete peer identities. The dependent CLI fixture replaces its peer component rather than appending a second one.

#### Validation and Deployment

The recorded validation completed successfully:

- Focused genesis, bootstrap extraction and directory address-validation tests.
- Twenty repeated race-detector runs of that focused selection.
- Broader tests for `command/genesis`, `command/peerdirectory`, `command/root`, `network/...`, `consensus/innbcbft/...` and `server`.
- `go vet` across those packages.

The production correction is confined to genesis metadata construction and explicit-input validation. It adds no steady-state consensus or transaction-processing work.

A rebuilt CLI uses the corrected behavior for subsequent genesis generation. Existing valid genesis files and node identities require no changes. No Solidity changes or contract-artifact regeneration are required.

<!-- FINISH ALPHA CHANGE 3852 -- document compatible genesis bootstrap identity generation and validation -->

<!-- START ALPHA CHANGE 3855 -- document txpool queue storage release and preserved cleanup accounting -->

### 127. Transaction-Pool Pruning Releases Removed Payload References

`accountQueue.clear()` in `txpool/queue_account.go` now releases the queue's backing array when removing all its transactions.

Previously, clearing shortened the slice to zero length while retaining its backing array and transaction pointers. Nonce-gap pruning removed those transactions from the indexes and slot count, but an account with promoted transactions could remain active and keep the removed payloads reachable.

This could retain transaction data beyond the live entries represented by pool slot accounting. The scenario was reachable under default settings and did not require corrupted storage or validator privileges.

#### Ownership and Cleanup

The function first preserves the old slice as the returned `removed` list, then sets the stored queue to nil.

This separates the two ownership requirements:

- The queue retains no backing-array reference or capacity after clearing.
- The caller still receives the original transaction pointers needed to remove nonce/hash indexes and update slot and pending counters.

The returned list is not zeroed. Clearing its entries would erase the transaction pointers still required by cleanup.

Once caller cleanup finishes and no other owners retain the removed transactions, their storage becomes eligible for garbage collection. Collection and return of memory to the operating system are not immediate guarantees.

#### Preserved Behavior and Cost

Nonce-gap pruning continues to retain valid promoted transactions and their active account. Full-account maintenance removal continues to clear both queues and retire unused accounts.

Transaction admission, replacement rules, nonce handling, pruning decisions, slot accounting and pending counts retain their existing behavior. The change does not alter transaction execution, fees, block validity or consensus rules.

Clearing introduces no additional scan, transaction copying or allocation. Subsequent queue reuse allocates a new backing array instead of retaining the removed payloads through the previous one.

No performance benchmark or immediate process-RAM reduction is claimed.

#### Regression Coverage

The regressions reside in `txpool/queue_release_test.go`.

| Test | Coverage |
|---|---|
| `TestQueueRelease_ClearOwnershipAndReuse` | Nil, allocated-empty and populated queues; zero retained capacity; intact returned transaction pointers and payloads; independent queue reuse; repeated clearing. |
| `TestQueueRelease_PruningRetainsPromotedPrefix` | Actual nonce-gap pruning while a promoted transaction keeps the account active; released future-queue storage; correct indexes, nonce and counters; repeated pruning; subsequent admission and promotion. |
| `TestQueueRelease_FullAccountDropPreservesAccounting` | Full maintenance removal of both queues; account retirement; complete index cleanup; correct slot and pending counts; harmless repeated cleanup through a retired pointer. |

Pool regressions invoke the actual admission, promotion, pruning and removal methods with controlled signer/state fixtures. Promotion and pruning are coordinated synchronously.

The tests check retained ownership directly. They do not depend on forced garbage collection, finalizers, sleeps or process-memory measurements.

#### Validation and Deployment

The recorded validation passed:

- The new queue-release regressions.
- Existing nonce-gap pruning, drop, Issue 13 ownership/lifecycle and slot-pressure checks.
- Twenty repeated race-detector runs of that focused selection.
- Broader tests for `txpool/...`, `consensus/innbcbft/...`, `server` and `jsonrpc`.
- `go vet` across those packages.

Deploying the correction requires a rebuilt node binary. No new genesis, identity changes, Solidity changes or contract-artifact regeneration are required.

<!-- FINISH ALPHA CHANGE 3855 -- document txpool queue storage release and preserved cleanup accounting -->

<!-- START ALPHA CHANGE 3858 -- document outer synchronization callback termination and fallback regressions -->

### 128. Synchronization Honors Callback Termination Before Peer Fallback

The outer `Sync` loop in `syncer/syncer.go` now honors a callback's termination request before evaluating whether the download reached the peer's advertised head.

Previously, `bulkSyncWithPeer` stopped correctly when its callback returned true, but the outer loop checked incomplete-download fallback first. A callback requesting termination at block 101 while the peer advertised block 110 could therefore cause another download instead of ending synchronization.

The production callback in `consensus/innbcbft/innbcbft.go` requests termination after consensus-runtime advancement fails and reports a fatal error. The previous ordering could allow additional synchronization work before the independent server shutdown completed.

This was a recovery/error-path defect under the current deployment. Ordinary successful synchronization did not trigger it. No permanent halt or inconsistent committed state was established from this ordering.

#### Corrected Behavior

The existing termination check now precedes the incomplete-download branch:

- A callback returning true ends the current `Sync` invocation, including below the advertised head.
- A callback returning false preserves normal synchronization and incomplete-download peer fallback.
- Existing stream closure, progression cleanup and subscription release remain in place.
- Sequence validation, block verification, insertion rules and local-state retry handling retain their existing behavior.

The correction changes control-flow ordering within one production function. It adds no allocations, database operations, locks, timers or additional processing passes.

It does not repair the underlying runtime advancement failure or roll back the block already imported before the callback. Fatal-error reporting and coordinated server shutdown retain their existing responsibilities.

This correction is separate from the accepted conflicting-block callback identity limitation.

#### Regression Coverage

The added regressions reside in `syncer/callback_stop_test.go` and exercise the actual outer synchronization loop with controlled blockchain and client fixtures.

| Test | Coverage |
|---|---|
| `TestSyncCallbackStop_TerminationTakesPriority` | Termination below the advertised head after genesis, after existing history and after a valid prefix; termination at and beyond the earlier advertised head; exact request, verification, write and callback sequences; stream closure and progression cleanup. |
| `TestSyncCallbackStop_FalsePreservesIncompleteDownloadFallback` | A false callback preserves fallback after a short clean stream or a stream error following a valid prefix; the next peer resumes at the next required height; both streams receive cleanup. |

The termination cases leave a later block queued and an alternate serving peer available. Assertions require that neither produces additional processing after the stop request.

Fixtures use buffered block channels and controlled terminal delivery without sleeps or producer goroutines. They test callback-stop handling directly rather than reproducing the underlying consensus-runtime failure.

#### Validation and Deployment

The recorded validation passed:

- Both new callback-stop regressions and all seven subcases.
- Existing outer synchronization, bulk-download and immediate inner callback-termination tests.
- Twenty repeated race-detector runs of that focused selection.
- Broader tests for `syncer/...`, `consensus/innbcbft/...` and `server`.
- `go vet` across those packages.

Deploying the correction requires a rebuilt node binary. No new genesis, identity changes, Solidity changes or contract-artifact regeneration are required.

<!-- FINISH ALPHA CHANGE 3858 -- document outer synchronization callback termination and fallback regressions -->

<!-- START ALPHA CHANGE 3863 -- document proposal price-heap reference release and ownership regressions -->

### 129. Proposal Price Heap Releases Popped Transaction References

`maxPriceQueue.Pop()` in `txpool/queue_priced.go` now clears the discarded backing-array slot before shortening its transaction slice.

Previously, shortening the slice left the popped transaction pointer in the retained backing array. That pointer could keep the transaction and its payload reachable after finalized-nonce cleanup removed the corresponding account-queue and index entries.

A subsequent `Prepare()` replaced the proposal-selection heap. However, a formerly proposing node that left the committee and remained running could retain its final heap without another proposal being prepared.

This was bounded retention of the last selection's working set. No unbounded accumulation, block-validity failure, process crash or chain halt was established.

This correction is separate from the account-queue storage release documented in point 127.

#### Ownership and Garbage Collection

The function saves the transaction pointer in its return variable, clears the discarded array slot to nil, then shortens the slice.

The caller receives the same transaction object with its payload intact. Independent ownership in account queues and indexes remains unchanged until ordinary pool cleanup removes it.

Clearing the discarded reference permits garbage collection once all remaining owners release the transaction. It does not guarantee immediate collection or an immediate reduction in process RAM.

The pointer array retains its capacity for reuse. Live entries remaining in the heap continue to retain their transactions intentionally.

#### Runtime Scope and Cost

The production change adds one constant-time pointer assignment per pop. It introduces no additional scan, allocation, transaction copying or database access.

Transaction ordering, fee comparison, nonce handling, proposal selection, shared-pool accounting and block validity retain their existing behavior.

The existing benchmark receives a fresh pointer-slice copy for each heap construction because heap operations mutate and clear their owned slice slots. This fixture copying does not add production allocations.

#### Regression Coverage

The added regressions reside in `txpool/queue_priced_release_test.go`.

| Test | Coverage |
|---|---|
| `TestPricedQueueRelease_PopOwnershipAndReuse` | Empty and singleton queues; mixed legacy and dynamic-fee transactions with and without a base fee; expected ordering; cleared discarded slots; intact returned pointers and payloads; draining and capacity reuse. |
| `TestPricedQueueRelease_SelectionAndFinalizedCleanup` | Actual pool admission, promotion and proposal selection; preserved shared queue/index ownership and counters after selection; finalized-nonce cleanup while retaining the same executable heap without another `Prepare`; released discarded references and intact returned payload. |

The tests inspect backing-array ownership directly. They do not depend on forced garbage collection, finalizers, sleeps or process-memory measurements.

#### Validation and Deployment

The recorded validation passed:

- Both new price-heap release regressions.
- Existing price-ordering, account-queue release, pop, drop and Issue 13 ownership/lifecycle checks.
- Twenty repeated race-detector runs of that focused selection.
- Broader tests for `txpool/...`, `consensus/innbcbft/...`, `server` and `jsonrpc`.
- `go vet` across those packages.

Repeated benchmark-fixture smoke runs also passed. No comparative performance improvement or measured process-RAM reduction is claimed.

Deploying the production correction requires a rebuilt node binary. No new genesis, identity changes, Solidity changes or contract-artifact regeneration are required.

<!-- FINISH ALPHA CHANGE 3863 -- document proposal price-heap reference release and ownership regressions -->

<!-- START ALPHA CHANGE 3873 -- document generation-only genesis peer identity uniqueness validation -->

### 130. Genesis Generation Rejects Duplicate Validator Peer Identities

The intended production genesis contains one bootstrap validator. Duplicate peer identities across multiple genesis entries cannot occur in that deployment profile. Later validator registration is a separate on-chain mechanism and does not add entries to genesis.

The retained multi-validator genesis-generation capability now includes an additional input-validation guard against accidentally repeating a networking identity.

#### Generation-Time Validation

In `command/genesis/innbcbft_params.go`, `generatePolyBftChainConfig` validates the completed validator list after `getValidatorAccounts` returns.

This location covers both supported input workflows:

- Explicit `--validators` entries.
- Validator identities loaded through `--validators-path` and `--validators-prefix`.

The guard parses each multiaddress, extracts its peer identity and compares the normalized peer ID against identities already encountered.

Different IP addresses or ports do not make a repeated peer identity unique. A duplicate causes generation to return an error containing the peer ID and both conflicting validator addresses before writing the genesis file.

A single otherwise-valid validator passes normally: its identity is recorded once and no duplicate is found. Distinct peer identities also remain accepted, including identities using the same transport address.

Successful generation preserves the supplied validator addresses, BLS keys, possession proofs and multiaddresses.

#### Scope and Cost

This is generation-only validation. It adds one pass over the validator list and a temporary map of encountered peer identities.

It introduces no recurring startup, consensus, transaction-execution or peer-directory check. Existing genesis files are not modified or retroactively validated by this guard.

The change requires a rebuilt CLI binary to protect subsequent genesis generation. No Solidity changes, contract-artifact regeneration or replacement of an existing valid genesis are required.

#### Regression Coverage

`command/genesis/bootstrap_peer_uniqueness_test.go` exercises the actual command preparation and genesis-generation path.

| Input workflow | Scenario | Expected result |
|---|---|---|
| Explicit | One validator | Accepted |
| Explicit | Two distinct peer IDs | Accepted |
| Explicit | Repeated peer ID and identical multiaddress | Rejected |
| Explicit | Repeated peer ID with different transport addresses | Rejected |
| Directory | One validator directory | Accepted |
| Directory | Two distinct networking identities | Accepted |
| Directory | Distinct operator/BLS identities sharing one networking key | Rejected |

Duplicate fixtures retain distinct operator addresses and BLS keys, ensuring rejection specifically exercises peer-identity uniqueness.

Rejected cases verify that no genesis file is created. Accepted cases import the generated file and verify that validator identity fields remain intact.

#### Recorded Validation

The supplied validation results passed:

- All seven new generation cases and existing genesis-bootstrap regressions.
- Twenty repeated race-detector runs of the focused tests.
- Dependent tests for `command/genesis`, `command/peerdirectory`, `consensus/innbcbft/...` and `server`.
- `go vet` across those packages.

<!-- FINISH ALPHA CHANGE 3873 -- document generation-only genesis peer identity uniqueness validation -->

<!-- START ALPHA CHANGE 3876 -- document future transaction limits across delayed promotion -->

### 131. Per-Account Future Transaction Limits Survive Delayed Promotion

Transaction-pool admission now rejects additional future transactions when an account's enqueued count is at or above its configured limit.

Previously, `TxPool.addTx` checked only equality with the limit. Admission of the next expected nonce is intentionally exempt, and promotion occurs asynchronously. That combination could temporarily raise the queue above its limit, allowing another future transaction to bypass the equality check.

For example, with the default allowance of 128 enqueued transactions:

- Nonces 2 through 129 fill the future queue.
- Nonce zero is accepted through the expected-nonce exemption, temporarily increasing the queue to 129.
- Before promotion runs, the old equality comparison could accept nonce 130.
- Promotion moves nonce zero but stops at missing nonce one, leaving an over-limit future queue.

#### Corrected Admission Behavior

In `txpool/txpool.go`, the existing locked admission check now compares the enqueued count using greater-than-or-equal rather than equality.

Additional future transactions receive `ErrMaxEnqueuedLimitReached` throughout the temporarily over-limit interval.

The expected nonce remains admissible so that an account can fill a nonce gap and resume promotion. Consequently, the enqueued count may still temporarily exceed the limit while that expected transaction awaits promotion; the correction prevents additional future transactions from exploiting that interval.

Valid replacements retain their existing handling because the capacity check remains in the new-nonce admission branch. Replacement price requirements continue to apply.

#### Scope and Cost

This correction affects local transaction admission. It does not change block validity, consensus rules, transaction fees or the configured pool-wide slot and high-pressure limits.

The production change replaces one constant-time comparison. It adds no allocation, queue traversal, lock, database read or background worker.

#### Regression Coverage

`txpool/enqueued_limit_test.go` adds `TestEnqueuedLimit_DelayedPromotion`, using the actual default per-account limit of 128.

The fixture deliberately starts no background promotion worker. It invokes real promotion explicitly, reproducing the relevant ordering without sleeps or scheduler-dependent timing.

Coverage includes:

- Rejection at exactly full capacity and temporarily above capacity.
- Preserved expected-nonce admission while full.
- Valid queued replacements while above capacity.
- Valid promoted replacements while the future queue is full.
- Promotion stopping at a retained nonce gap.
- Gap filling, complete promotion and subsequent queue-capacity reuse.
- Preserved slot, pending-count and index accounting after rejection.
- Correct removal of replaced hashes and retention of replacement transactions.

#### Recorded Validation and Deployment

The supplied validation results passed:

- The focused delayed-promotion regression.
- Twenty repeated race-detector runs of that regression.
- Broader tests for `txpool/...`, `consensus/innbcbft/...`, `server` and `jsonrpc`.
- `go vet` across those packages.

The admission-limit defect is closed by the saved correction and reviewed validation results.

Deploying the correction requires a rebuilt node binary. No new genesis, identity changes, Solidity changes or contract-artifact regeneration are required.

<!-- FINISH ALPHA CHANGE 3876 -- document future transaction limits across delayed promotion -->

<!-- START ALPHA CHANGE 3882 -- document collision-safe consensus subscriptions and gossip readiness coverage -->

### 132. Consensus Subscription IDs Preserve Active Ownership

Consensus message subscriptions now check ID availability before registering a new listener.

Previously, `eventManager.subscribe` used the 32-bit value returned by `uuid.New().ID()` without checking whether another active subscription already owned it. A rare collision could overwrite that registration, lose notifications, misdirect cancellation and leave the displaced notification worker running.

#### Runtime Correction

In `consensus/innbcbft/ibftmessages/event_manager.go`, allocation retains the existing UUID-derived candidate and checks the subscription map under the existing manager lock.

If the candidate is occupied, allocation advances through the 32-bit ID space until it finds an unused ID. Existing live registrations remain intact. IDs become available for reuse after cancellation removes their registrations.

Ordinary allocation adds one map lookup. Additional probes occur only when candidate IDs are occupied. The correction introduces no additional database access, worker, signature check or consensus-message processing pass.

Subscription types, wire formats, quorum rules and block-validity rules retain their existing behavior.

#### Deterministic Collision Coverage

`consensus/innbcbft/ibftmessages/subscription_id_collision_test.go` adds `TestSubscriptionID_LiveCollisionOwnership`.

The tests force repeated UUID-derived candidates and cover:

- Multiple simultaneously occupied IDs.
- Crossing the signed-ID boundary.
- Unsigned wraparound through zero.
- Notification delivery to every active subscriber.
- Independent cancellation and worker termination.
- Reuse of a cancelled ID without disturbing other subscribers.
- Consistent registration counts.

The cases run sequentially because they temporarily control UUID randomness. Cleanup stops captured workers and restores the default random source.

#### Gossip-Test Readiness and Delivery Diagnostics

The separate `TestSimpleGossip` regression in `network/gossip_test.go` now checks every server's topic-subscriber view and routing readiness before the real publication.

Readiness uses the pinned GossipSub router's lower mesh bound, `GossipSubDlo`, capped by the available remote-node count. In the nine-node fixture this requires five routing peers. The target mesh size of six is not a mandatory minimum: the router can retain a healthy five-peer mesh.

Readiness probes are local-only and excluded from delivery accounting. The actual test message is published exactly once through the production `Topic.Publish` wrapper.

The test requires delivery to all nine distinct receiving nodes. Duplicate callbacks cannot substitute for a missing recipient. Failures identify the receiving node or list missing node indexes, peer IDs and observed topic-peer counts.

Callback errors are reported through the main test goroutine. An absolute delivery deadline prevents additional callbacks from extending the wait indefinitely. Cleanup cancels callback delivery before joining topic workers.

This is a test-only refinement. The exact cause of the earlier intermittent gossip timeout was not established.

#### Recorded Validation and Deployment

The supplied results passed:

- Broader tests for `consensus/innbcbft/...`, including the deterministic collision regressions.
- Twenty normal and ten race-detector repetitions of the corrected gossip test.
- The broader combined consensus, networking and server test run.
- `go vet` across those packages.
- CLI committee-liveness cases with 1, 2, 3, 4 and 6 validators.

The subscription-ID collision defect is closed by the saved correction and reviewed validation results.

Deploying the runtime correction requires a rebuilt node binary. No new genesis, identity changes, Solidity changes or contract-artifact regeneration are required.

<!-- FINISH ALPHA CHANGE 3882 -- document collision-safe consensus subscriptions and gossip readiness coverage -->

<!-- START ALPHA CHANGE 3886 -- document recovery compatibility with stored arbitrary-precision balances -->

### 133. Recovery Preserves Canonical Account Balances Beyond 256 Bits

Recovery validation now accepts the same arbitrary-precision account balances that live state already serializes.

Previously, the strict recovery decoder imposed a 32-byte account-balance limit. Live account balances use Go's `big.Int` representation and can retain larger values.

For example, an accepted zero-address genesis allocation of `2^256 - 1` wei followed by a one-wei transfer produces a stored balance of `2^256` wei. Its 33-byte representation previously prevented snapshot copying and complete-state validation, including the final validation of an offline trie rebuild.

This scenario requires extreme balances outside the intended initial deployment. The correction resolves the recovery inconsistency without changing live balance accounting.

#### Recovery Correction

In `state/immutable-trie/complete_state.go`, a zero maximum passed to `validateSnapshotInteger` now disables its byte-width limit. Only account-balance validation selects this option.

The remaining validation rules are preserved:

- Account nonces remain limited to eight bytes.
- Contract-storage integers remain limited to 32 bytes.
- Integers must use RLP byte strings without leading zeros.
- Account records must contain exactly four fields.
- Storage roots and code hashes must contain exactly 32 bytes.
- Canonical RLP, trie commitments and referenced executable-state objects remain checked.

The correction applies to account balances generally, matching their existing serialized representation.

`CopyTrie` and `ValidateCompleteState` share the corrected account decoder. Snapshot construction, snapshot validation and rebuilt-trie final validation therefore use the same balance rule.

#### Regression Coverage

`state/immutable-trie/snapshot_balance_test.go` adds:

| Test | Coverage |
|---|---|
| `TestSnapshotBalance_CanonicalWidths` | Exact decoding and canonical round trips for zero, one, the uint256 maximum, `2^256`, `2^264` and `2^512`. |
| `TestSnapshotBalance_ZeroAddressTransferSurvivesRecovery` | Persisted state-level changes corresponding to a one-wei transfer into the maximum-funded zero address; fresh source reads, complete-state validation, independent trie copying and exact recovered account encoding under the original root. |
| `TestSnapshotBalance_RetainsStrictIntegerChecks` | Continued rejection of a nine-byte nonce and a wide balance containing a leading zero. |

Existing strict-decoding and trie-copy tests continue checking malformed encodings, oversized storage values, missing references, hash mismatches, depth limits and write failures.

#### Recorded Validation and Deployment

The supplied results passed:

- Focused balance-recovery, strict-decoding, complete-state and trie-copy tests.
- Twenty race-detector repetitions of the new balance-recovery tests.
- Broader tests for `state/...`, `snapshotbuilder/...` and `server`.
- `go vet` across those packages.

The new extreme-balance regression directly exercises the shared decoder and memory-backed trie recovery paths. Compatibility with snapshot packaging and offline rebuild validation follows from their verified use of those shared functions.

The recovery-width inconsistency is closed.

The existing native-token uint256 supply limit and EVM BALANCE/SELFBALANCE protections remain unchanged. Recovering a stored oversized balance does not make that balance representable as an EVM word.

Deploying this correction requires a rebuilt node binary. No new genesis, identity changes, Solidity changes or contract-artifact regeneration are required.

<!-- FINISH ALPHA CHANGE 3886 -- document recovery compatibility with stored arbitrary-precision balances -->

<!-- START ALPHA CHANGE 3889 -- document epoch reconciliation across reordered block callbacks -->

### 134. Epoch Transitions Survive Reordered Consensus and Synchronization Callbacks

Consensus runtime advancement now reconstructs epoch metadata when a block callback reaches or passes the runtime's stored epoch boundary.

Previously, reconstruction required the callback's height to equal that boundary exactly. Consensus could commit an epoch-ending block before invoking its runtime callback. Concurrent synchronization could then import the following block and invoke its callback first.

That ordering advanced the runtime head past the boundary without refreshing the epoch. The delayed boundary callback was subsequently treated as already handled. The node could remain aligned with canonical height and hash while retaining an old epoch number and rejecting correct live proposals.

The scenario uses consecutive blocks from one legitimate canonical history. It does not require conflicting finalized blocks or malicious peers. The solitary bootstrap-validator phase cannot exercise this remote-ahead scenario; supported multi-validator operation can.

#### Runtime Correction

In `consensus/innbcbft/consensus_runtime.go`, `OnBlockInserted` now checks whether the supplied height has reached or passed the current runtime epoch's ending height.

The comparison uses guarded subtraction, preventing subtraction underflow and avoiding addition overflow. The existing configuration validation requires an epoch size of at least two.

When reconstruction is required, the existing `restartEpoch` function obtains the epoch number and validator set from the supplied block's state and determines that epoch's first block. This also handles a callback gap spanning multiple epochs.

The exact-boundary helper remains unchanged for proposal construction and `PostBlockRequest.IsEpochEndingBlock`. A callback arriving after a boundary is therefore reconciled without classifying its block as an epoch-ending proposal.

Proposer preparation continues processing missing intermediate headers through its existing path. Epoch markers and proposer data use the existing database transaction. The runtime epoch, proposer snapshot and head are published only after commit succeeds.

#### Runtime Cost and Scope

Normal callbacks before the stored epoch boundary add only integer checks. They introduce no additional epoch-state or validator-set reads.

A callback gap crossing a boundary performs the reconstruction work previously omitted. Subsequent callbacks inside the recovered epoch do not repeat that reconstruction.

The correction does not replay every skipped full-block callback or alter the separately accepted conflicting same-height block identity limitation documented in point 59.

#### Deterministic Regression Coverage

`consensus/innbcbft/epoch_callback_order_test.go` adds:

| Case | Verified behavior |
|---|---|
| Ordered boundary callbacks | Ordinary epoch advancement remains correct. |
| Callbacks `31, 30, 32` from runtime height 29 | Block 31 refreshes the epoch; delayed block 30 is harmless; normal processing continues. |
| Gap within one epoch | No unnecessary epoch reconstruction occurs. |
| Gap crossing multiple epochs | The runtime adopts the target epoch and its correct first block. |
| Committee replacement across the boundary | Runtime membership and proposer membership reflect the updated committee. |
| Subsequent FSM preparation | The next FSM uses the correct epoch and proposer height. |
| Injected epoch-read failure | Runtime metadata and persisted proposer progress remain unchanged; a direct unit-test retry succeeds. |

The tests use real MDBX consensus state and controlled callback ordering without sleeps. The direct retry verifies recoverability in the unit test; production fatal-error handling remains unchanged.

#### Recorded Validation and Deployment

The supplied results passed:

- Focused callback-ordering and existing block-insertion tests.
- Twenty race-detector repetitions of the new regressions.
- Broader tests for `consensus/innbcbft/...`, `syncer/...` and `server`.
- `go vet` across those packages.
- CLI committee-liveness cases with 1, 2, 3, 4 and 6 validators.

The ordering regressions deterministically exercise the problematic callback sequence. They do not reproduce a live network scheduling race. The CLI matrix separately verifies ordinary committee liveness.

The skipped epoch-transition issue is closed by the saved correction, source review and passing validation results.

Deploying the correction requires a rebuilt node binary. No new genesis, identity changes, Solidity changes or contract-artifact regeneration are required.

<!-- FINISH ALPHA CHANGE 3889 -- document epoch reconciliation across reordered block callbacks -->

<!-- START ALPHA CHANGE 3893 -- document overflow-safe synchronization timeout calculation -->

### 135. Overflow-Safe Synchronization Timeouts

Synchronization timeout calculation now prevents signed duration overflow when tripling a configured block interval.

Previously, an extreme accepted interval could fit Go's `time.Duration` representation while its tripled synchronization timeout did not. For example, `4294967296s`, approximately 136 years, produced a negative timeout after multiplication overflow. Passing that result to the download timer could cause premature timeout.

This was optional extreme-configuration hardening. The documented two-second deployment was unaffected.

#### Runtime Calculation

In `consensus/innbcbft/innbcbft.go`, `syncBlockTimeout` checks the unsigned whole-second input before converting or multiplying it.

| Input | Result |
|---|---|
| Ordinary representable intervals | Exactly three times the block interval |
| `3074457345` seconds | Last input whose tripled duration fits exactly |
| `3074457346` seconds and larger | Maximum representable duration: `9223372036854775807` nanoseconds |

The production syncer constructor uses this helper.

Normal behavior remains exact: one-second blocks produce a three-second timeout, two-second blocks produce six seconds, and three-second blocks produce nine seconds.

The accepted block-interval range remains unchanged. Zero retains its previous arithmetic result inside the helper; existing configuration validation separately rejects zero block intervals.

The calculation runs when the syncer is constructed. It adds no recurring transaction or block-execution work and does not change block timestamps or header-validation rules.

#### Regression Coverage

`consensus/innbcbft/sync_timeout_test.go` adds `TestSyncBlockTimeout_SaturatingArithmetic`.

Its twelve cases cover:

- Zero and ordinary one-, two-, three- and 3600-second inputs.
- Inputs immediately below, at and above the exact multiplication boundary.
- The reported `2^32`-second example.
- The largest whole-second interval representable by `time.Duration`.
- Inputs exceeding the signed integer range and the maximum `uint64` input.

Expected values are calculated independently using arbitrary-precision `big.Int` multiplication and clamping. Every positive input must produce a positive duration.

The regression verifies arithmetic directly without waiting on timers or reproducing a network timeout.

#### Recorded Validation and Deployment

The supplied results passed:

- The focused regression and all twelve cases.
- Broader tests for `consensus/innbcbft/...`, `syncer/...` and `server`.
- `go vet` across those packages.

The synchronization-timeout overflow issue is closed by the saved correction, source review and passing validation results.

Deploying the correction requires a rebuilt node binary. No new genesis, identity changes, Solidity changes or contract-artifact regeneration are required.

<!-- FINISH ALPHA CHANGE 3893 -- document overflow-safe synchronization timeout calculation -->

<!-- START ALPHA CHANGE 3898 -- document terminal nonce rejection and preserved proposal gas capacity -->

### 136. Terminal Account Nonces Are Rejected Before Admission and Gas Reservation

Ordinary transaction validation now rejects a transaction nonce of `2^64 - 1` before pool admission or execution gas reservation.

Previously, a funded account whose stored nonce was already at that maximum could submit an otherwise admissible transfer. Execution reserved its gas allowance before the checked nonce increment rejected overflow. Transaction state rolled back, but the execution transition's separate gas pool remained depleted.

Proposal selection dropped the rejected transaction and continued using that transition. Subsequent legitimate transactions could therefore encounter an exhausted gas pool, producing an empty or underfilled proposal despite unused block capacity.

The rejected transaction was not mined or charged. Gas capacity was initialized again for the next proposal. No invalid committed-state acceptance or unconditional chain halt was established.

#### Reachability and Default Behavior

Ordinary wallet accounts begin at nonce zero. Normal genesis premine generation leaves that initial nonce unchanged.

The terminal value can be reached through either:

- An imported custom genesis explicitly initializing an account at the maximum nonce.
- Normal runtime nonce increments eventually reaching the same boundary.

Starting from zero, reaching this boundary requires approximately `2^64` nonce increments for one account. The concrete audit scenario used an extreme custom initial nonce and was outside the documented deployment profile.

The nonce is an account counter, independent of native-token supply and account balances.

| Transaction nonce | Corrected behavior |
|---|---|
| Zero | Normal validation and execution |
| `2^64 - 2` | Remains usable; successful execution advances the stored nonce to `2^64 - 1` |
| `2^64 - 1` | Rejected because the sender counter cannot advance further |

Reaching the maximum stored value is not itself an overflow. The existing checked increment already prevents wrapping to zero. The correction rejects transactions at the exhausted value before they can affect proposal gas capacity.

#### Production Correction

Two guards implement the correction:

- `state/executor.go`: `checkAndProcessTx` rejects the terminal nonce with `ErrNonceUintOverflow`, classified as a non-recoverable transaction error. Retained local-state failures keep precedence. Rejection occurs before nonce matching, upfront gas debit, gas-pool reservation and EVM execution.
- `txpool/txpool.go`: `validateTxWithPreStateCheck` rejects the same terminal nonce before serialization, queue publication, slot reservation and promotion.

The execution guard also protects block execution that bypasses the local transaction pool.

The stored nonce remains an unsigned 64-bit value. Existing checked increments and internal contract-creation overflow handling remain in place.

#### Regression Coverage

The new files are:

- `state/nonce_limit_test.go`
- `txpool/nonce_limit_test.go`

| Test | Coverage |
|---|---|
| `TestNonceLimit_ExecutionRejectsMaximum` | Legacy and dynamic-fee transactions; calls and creation; direct Apply and Write; unchanged state, gas capacity, context, receipts, logs and refunds; a subsequent valid transaction succeeds on the same transition. |
| `TestNonceLimit_ExecutionBoundary` | Successful nonce-zero and maximum-minus-one transactions; correct charging; the rejected terminal successor preserves previously successful effects. |
| `TestNonceLimit_RetainedStateFailurePriority` | Existing local-state failure remains authoritative over the nonce rejection. |
| `TestNonceLimit_PoolRejectsMaximum` | Rejection with ordinary, maximum-minus-one and maximum account nonces; no account, index, capacity or promotion publication. |
| `TestNonceLimit_PoolLastUsableNonce` | The last usable transaction is admitted and promoted; the pending nonce reaches the maximum without wrapping; terminal rejection preserves existing ownership and capacity. |

#### Recorded Validation and Deployment

The supplied validation passed:

- Focused nonce-limit regressions.
- Repeated focused race checks.
- Broader tests covering state, txpool, blockchain, INNBCBFT consensus, synchronization, genesis, server and JSON-RPC.
- `go vet` across those packages.

The reported terminal-nonce proposal-capacity issue is closed by the saved guards, source review and passing validation.

Deploying the correction requires a rebuilt client used consistently across validators. No Solidity changes, contract-artifact or binding regeneration, new genesis, identity changes or storage migration are required for the documented deployment.

<!-- FINISH ALPHA CHANGE 3898 -- document terminal nonce rejection and preserved proposal gas capacity -->

## Storage Modes

### Full History

A normally synchronized full node retains complete canonical blockchain history in one continuous MDBX database.

Typical uses include:

- public historical RPC;
- explorers and indexers;
- historical synchronization;
- snapshot source storage;
- historical debugging.

### Recent History

A recent node retains current state and consensus metadata while keeping blockchain history in a bounded sequence of physically removable MDBX directories.

Typical uses include:

- validator operation without unbounded blockchain-history growth;
- deterministic physical history retention;
- short-gap synchronization;
- snapshot-assisted startup.

Recent nodes are not intended as public historical RPC backends.

## Snapshot Workflow

```text
running full-history source
→ on-demand snapshot builder opens MDBX read-only
→ builder captures a fixed consensus checkpoint
→ builder copies the retained blockchain suffix
→ builder copies only the trie reachable from that state root
→ builder validates the complete artifact
→ builder publishes it under segmented/ or checkpoint/
→ consuming node stages and validates chain-data
→ chain-data is activated without replacing node identities
→ node synchronizes blocks produced after the checkpoint
```

A segmented artifact is directly compatible with recent-history storage. A checkpoint artifact provides the standard single-database Plan B layout.

<!-- START ALPHA CHANGE 3504 -- distinguish optional fallback from incompatible database pages -->
Optional recent bootstrap normally falls back to local startup or synchronization when no usable snapshot is available. A selected snapshot with incompatible database page sizes stops startup even when bootstrap is optional. Existing local database incompatibility also stops startup. Required bootstrap fails startup if a valid artifact cannot be activated.
<!-- FINISH ALPHA CHANGE 3504 -- distinguish optional fallback from incompatible database pages -->

## Peer-Directory Workflow

<!-- START ALPHA CHANGE 1924 -- document persistent peer discovery and deterministic manual convergence -->
The peer-directory service stores authenticated validator libp2p addresses but does not grant validator authority.

Validator authority comes from:

```text
genesis validators before registry activation
→ registered and whitelisted validators from the on-chain registry after activation
```

Every signed peer-directory exchange re-reads the current validator authority. The service retains only cached peer identities that remain authorized by that current state.

The authenticated address map is persisted in:

```text
/var/lib/innbc/peer-directory/peers.json
```

Consequently, stopping and restarting the peer-directory service does not normally erase its known validator addresses.

Nodes use an intentionally manual convergence model. Each node performs one exchange during startup, while operators use the explicit refresh command whenever validator membership or addressing changes.

### Adding a validator

After registering and whitelisting a new validator:

1. Start the new validator node.
2. Explicitly refresh the new node so it registers its signed libp2p multiaddress.
3. Refresh every running validator so all nodes receive the updated address list.

Register and refresh the new node:

```bash
innbc-chain peerdirectory refresh \
  --grpc-address <new-node-grpc-address>
```

Then refresh every existing validator:

```bash
innbc-chain peerdirectory refresh \
  --grpc-address <existing-node-grpc-address>
```

For three validators running locally:

```bash
# Explicitly register the newly started node2.
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9633

# Propagate the completed list to every running validator.
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9632
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9633
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9634
```

Calling `peerdirectory refresh` against a node tells that running node to perform its own authenticated exchange with the configured peer-directory endpoint. It does not modify consensus membership.

### Removing or updating a validator

Refresh every remaining running validator after:

- removing a validator;
- changing its whitelist status;
- changing its registered libp2p peer ID;
- changing its advertised libp2p multiaddress.

During the exchange, the directory checks the current registry, removes addresses belonging to identities that are no longer registered and whitelisted, persists the filtered cache, and returns the updated list.

### Restarting the peer directory

A normal peer-directory restart does not require validators to register again:

```text
peer-directory stops
→ peers.json remains on disk
→ peer-directory starts
→ authenticated address cache is restored
```

Manual re-registration is necessary only when the cache is missing, was corrupted and regenerated, or a validator’s identity or address changed.

The cache supplies operational discovery addresses and the peer identities used by each node's cached networking allowlist. Consensus validator authority remains governed by the genesis bootstrap configuration and the active on-chain registry.

Networking admission requires both cryptographic authentication of the libp2p identity and authorization under the node's current cached policy. It does not perform a fresh contract lookup for every connection or message.

A registry change is therefore not an immediate networking-policy update on every node. Refresh the remaining nodes to propagate membership changes and disconnect identities removed from their allowlists. Networking access under a stale list does not itself grant consensus voting authority.

Point 64 describes startup admission, connection ownership, reconnect handling, and synchronization-status session protection.
<!-- FINISH ALPHA CHANGE 1924 -- document persistent peer discovery and deterministic manual convergence -->

## Build Requirements

The repository currently targets:

```text
Go 1.27
```

<!-- START ALPHA CHANGE 3903 -- document explicit builds and directly included INNBC contracts -->

<!-- START ALPHA CHANGE 3907 -- remove the retired standalone lint requirement -->

<!-- START ALPHA CHANGE 3912 -- document manually maintained dependency attributions -->

Additional tools are required only for specific workflows:

- Git for source management
- `protoc` for protobuf regeneration
- Node.js and npm for core-contract compilation

Dependency attributions are maintained manually in `licenses/bsd_licenses.json`. The legacy generator script and its Makefile target have been removed. The existing `license` CLI command continues to display the project license and dependency attributions embedded in the executable.

After editing the attribution JSON, rebuild the executable to include the revised information.

<!-- FINISH ALPHA CHANGE 3912 -- document manually maintained dependency attributions -->

<!-- FINISH ALPHA CHANGE 3907 -- remove the retired standalone lint requirement -->

## Build

Running `make` without a target displays the available commands. It does not build the project or initialize Git submodules.

The customized INNBC Solidity sources are included directly in `core-contracts/` as ordinary project files. Compile these local sources when regenerating contract artifacts and Go bindings. The legacy `download-submodules` target has been removed.

| Command | Behavior |
|---|---|
| `make` | Display available commands |
| `make build` | Build the INNBC Chain binary as `./innbc-chain` |
| `make nodeclient` | Build a fresh binary and generate production installation packages under `dist/` |
| `make compile-core-contracts` | Compile the local customized contracts and regenerate Go bindings |

`make nodeclient` builds its binary directly into `dist/innbc-nodeclient/bin/innbc-chain`; it does not reuse the executable produced by `make build`.

<!-- FINISH ALPHA CHANGE 3903 -- document explicit builds and directly included INNBC contracts -->

Build the node binary:

```bash
make build
```

Build the production nodeclient package:

```bash
make nodeclient
```

Build the local multi-node testing workspace:

```bash
make localtestclient
```

<!-- START ALPHA CHANGE 3913 -- document supplied contract artifacts and dependency installation -->

### Contract Artifacts And JavaScript Dependencies

The source distribution includes the customized Solidity sources, compiled contract artifacts, and generated Go contract data and bindings. It does not include `core-contracts/node_modules/`.

Keep `core-contracts/package.json` and `core-contracts/package-lock.json`: these describe the JavaScript dependencies and their recorded versions.

Building the INNBC Chain node from the supplied generated Go contract files does not require installing the Solidity development dependencies or recompiling the contracts. Use `make build` for the node binary, or `make nodeclient` for the production installation packages. Building a binary does not start a node; installation and runtime configuration are documented separately.

<!-- START ALPHA CHANGE 3914 -- document complete development dependency setup -->

To set up the complete node-and-contract development environment, first install the required Go toolchain, system build tools, Node.js, and npm. Then run these commands from the repository root to download the Go and JavaScript dependencies:

```bash
go mod download
cd core-contracts
npm install
cd ..
```

This downloads the Go dependencies into Go's module cache and creates `core-contracts/node_modules/` for contract development. It preserves the supplied contract artifacts and generated Go bindings without recompiling them.

After dependency setup, use `make build` to build the node executable or `make nodeclient` to generate production installation packages.

<!-- FINISH ALPHA CHANGE 3914 -- document complete development dependency setup -->

To perform the complete contract build workflow instead, run this command from the repository root:

```bash
make compile-core-contracts
```

That target performs:

1. JavaScript dependency installation with `npm install`.
2. Compilation of the local customized Solidity contracts with Hardhat.
3. Generation of Go contract artifact data.
4. Generation of Go contract bindings.

A separate `npm install` step is unnecessary when using this target because installation is already included.

After changing Solidity sources, regenerate the contract artifacts and bindings before rebuilding a node binary intended to include those changes.

<!-- FINISH ALPHA CHANGE 3913 -- document supplied contract artifacts and dependency installation -->

## Test

Run the complete Go test suite:

```bash
go test ./...
```

Run the standard race-enabled repository test target:

```bash
make test
```

Run focused race-detector tests for critical runtime components:

```bash
go test -race \
  ./server \
  ./consensus/innbcbft \
  ./syncer \
  ./txpool \
  ./jsonrpc \
  ./blockchain
```

<!-- START ALPHA CHANGE 3258 -- document current workflow tests and legacy identity framework retirement -->
Run the current node-workflow integration tests with the race detector:

```bash
make test-current-workflow
```

The target enables `INNBC_WORKFLOW_E2E=1` and runs `TestCurrentWorkflow_` tests in `command/root`. The tests launch node and peer-directory subprocesses using the test executable and create isolated identities, genesis configurations and databases.

Coverage includes current identity creation, authenticated peer membership, RPC state selection, transaction propagation and fee accounting, access-control modes, operator request validation, restart synchronization, quorum loss and epoch recovery with four through eight validators.

<!-- START ALPHA CHANGE 3276 -- document consolidation of identity commands around the current node workflow -->
The legacy `innbcbft-secrets` command and its dependent `e2e-innbcbft` framework and tests have been retired. The legacy `secrets init` and `secrets output` commands and their parent CLI group have also been removed.

<!-- START ALPHA CHANGE 3369 -- describe the current supported identity inspection interface -->
Node identity creation uses `node create`, with up to ten new identities per invocation and automatic numbering through node100. Read-only inspection uses `node info nodeN` or `node info all` to validate stored identities, report public registration information including the operator-bound BLS proof, and independently inspect their environment files. Point 100 documents allocation safeguards and the limits of post-creation inspection. Shared local key-storage, wallet and cryptographic helpers remain available to creation and runtime startup.
<!-- FINISH ALPHA CHANGE 3369 -- describe the current supported identity inspection interface -->

<!-- START ALPHA CHANGE 3286 -- document removal of remote identity backend implementations and selection -->

Node identity storage is explicitly local. The Vault, AWS SSM and GCP Secret Manager backend implementations, their selection helpers and remote configuration entry points have been removed. The legacy `secrets generate` configuration generator and the `--secrets-config` CLI option are also retired.

The direct Vault, AWS SDK and GCP Secret Manager SDK requirements are absent from `go.mod`. Shared local key-storage and cryptographic helpers remain in use by current node commands and runtime startup.

<!-- FINISH ALPHA CHANGE 3286 -- document removal of remote identity backend implementations and selection -->

The retired `secrets output` command displayed public identifiers, not private keys. Its removal consolidates the operator interface. Removed legacy expectations are replaced by assertions matching the current runtime behavior.
<!-- FINISH ALPHA CHANGE 3276 -- document consolidation of identity commands around the current node workflow -->

<!-- START ALPHA CHANGE 3279 -- document retirement of the empty consensus CLI wrapper -->
The empty top-level `innbcbft` CLI wrapper has been retired. It had no remaining subcommands or execution callback.

The active `consensus/innbcbft` implementation, server engine registration, genesis initialization and consensus configuration remain intact. Genesis generation continues to support `--consensus innbcbft`.
<!-- FINISH ALPHA CHANGE 3279 -- document retirement of the empty consensus CLI wrapper -->

The integration scenarios require explicit enablement; ordinary test runs without `INNBC_WORKFLOW_E2E=1` skip those scenarios.
<!-- FINISH ALPHA CHANGE 3258 -- document current workflow tests and legacy identity framework retirement -->

Run fuzz tests:

```bash
make fuzz-test
```

Run static analysis:

```bash
go vet ./...
```

<!-- START ALPHA CHANGE 3908 -- document independent custom-rule tests -->

The custom-rule tests in `gorules/` load `rules.go` and their test fixtures directly through the ruleguard analyzer library. They do not require the external `golangci-lint` executable or its configuration.

These tests are included in `go test ./...` and can also be run independently:

```bash
go test ./gorules
```

<!-- FINISH ALPHA CHANGE 3908 -- document independent custom-rule tests -->

<!-- START ALPHA CHANGE 3258 -- locate current workflow coverage in the repository layout -->
## Repository Layout

```text
blockchain/                 Canonical blockchain and storage logic
chain/                      Chain configuration types
command/                    CLI commands
command/root/               Root CLI routing and current-workflow regression tests
consensus/innbcbft/         INNBCBFT consensus and persistent metadata
contracts/                  Embedded contract artifacts
core-contracts/             Solidity system contracts
docs/                       User and architecture documentation
gasprice/                   Gas recommendation and fee-history logic
jsonrpc/                    Ethereum JSON-RPC and WebSocket server
network/                    libp2p networking and peer directory client
network/peerdirectory/      Authenticated peer-directory HTTP service
packaging/                  Production node and peer-directory packages
server/                     Node assembly, pruning, snapshots, and system service
state/                      EVM execution and immutable-trie state
syncer/                     Finalized-block synchronization
txpool/                     Transaction admission and promotion
validators/                 Validator abstractions and stores
```
<!-- FINISH ALPHA CHANGE 3258 -- locate current workflow coverage in the repository layout -->

<!-- START ALPHA CHANGE 2118 -- document offline full-history trie rebuilding -->
## Offline Trie Database Recovery

`node trie-rebuild` reconstructs execution state from the original genesis
configuration and the node's complete local canonical block history.

This operation is for a full-history node whose blockchain database remains
intact but whose trie database needs replacement. It does not download blocks,
repair block history, or reconstruct consensus metadata.

The node and all other database maintenance processes must remain stopped
throughout the operation. Prevent service supervisors from restarting the node.
The command does not acquire a process lock or check service status.

### Preconditions

- Preserve the original genesis configuration.
- Preserve `chain-data/blockchain`, `chain-data/consensus`, and node keys.
- The genesis state must be generated from allocations; configurations using a
  nonzero INNBCBFT `InitialTrieRoot` are not supported by this recovery command.
- The unified blockchain database must retain complete history from genesis.
- `chain-data/trie` must be absent or completely empty.
- Move any trie data that must be preserved to a separate backup location first.
- Run as the account owning the node databases.
- Provide sufficient free disk space for replay into a replacement database.
- Use a binary that includes the `node trie-rebuild` command.

The command refuses a nonempty trie directory and never deletes its contents.
An empty directory is a replacement precondition, not proof that the node is
stopped.

A full-mode node restored from a checkpoint with discarded early history does
not satisfy the complete-history requirement.

### Production command

After stopping the node, preventing automatic restart, and preparing the empty
trie target, run:

```bash
sudo runuser --user innbc -- \
  /usr/local/bin/innbc-chain node trie-rebuild \
  --chain /etc/innbc/genesis.json \
  --data-dir /var/lib/innbc/node1 \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3504 -- document production trie storage geometry --
# FINISH ALPHA CHANGE 3504 -- document production trie storage geometry --
```

Run this operation as `innbc` so the replacement database has the correct
ownership for the production service.

### Localtest command

The localtest builder places the executable in `localtestclient/bin` and copies
the genesis configuration to `localtestclient/genesis.json`.

After stopping the local node and preparing its empty trie target, run this
command from the repository root:

```bash
./localtestclient/bin/innbc-chain node trie-rebuild \
  --chain ./localtestclient/genesis.json \
  --data-dir ./localtestclient/nodes/node1 \
  --mdbx-map-size-gb 1024 \
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3504 -- document local trie storage geometry --
# FINISH ALPHA CHANGE 3504 -- document local trie storage geometry --
```

Run it as the same interactive user that runs the local node. No environment
file or production wrapper is required. Use the genesis configuration that
belongs to that node's retained block history.

### Reconstruction and validation

The command executes the existing genesis initialization and replays all
canonical blocks from block 1 to the captured local head.

It checks:

- Reconstructed genesis identity.
- Canonical block continuity.
- Transaction and uncle roots.
- Execution state roots.
- Gas usage.
- Receipt roots and log blooms.

After replay, it closes and reopens the replacement trie and validates the
complete executable state at the final head.

The replacement is built in a uniquely named `.trie-rebuild-*` directory inside
`chain-data`. Only after validation does the command publish it as
`chain-data/trie`.

Blockchain and consensus records are not rewritten. Node keys and the genesis
file remain unchanged.

The command reports the recovered block number, block hash, state root, and
trie path. It does not start the node. Restart the node only after successful
completion.

### Interrupted or failed rebuilding

Ordinary failures and handled termination signals clean up that invocation's
staging directory. A forced kill or power loss can leave staging data behind.
It is not activated automatically and is not resumed by the next invocation.

Once the node and rebuild process are confirmed stopped, an operator may
remove an abandoned `.trie-rebuild-*` staging directory and start a new rebuild.
Do not remove the blockchain or consensus databases.

Publication renames the completed staging directory to `chain-data/trie`.
If publication succeeds but parent-directory synchronization fails, the command
reports that distinction and leaves the installed trie in place. Keep the node
stopped while investigating that filesystem error.

An existing nonempty trie causes another invocation to refuse. It is never
silently overwritten.

### Scope

This command recreates state through full replay. It does not compact the
result to only the final reachable state. Existing checkpoint construction
remains the separate compact-state workflow.

The retained local block history is the recovery source. The command verifies
execution consistency against that history; it does not independently
authenticate an untrusted replacement chain.

The operation runs offline and adds no work to ordinary block production.
<!-- FINISH ALPHA CHANGE 2118 -- document offline full-history trie rebuilding -->

## Documentation

Detailed documentation is maintained in:

<!-- START ALPHA CHANGE EXPLORER-308 -- distinguish local chain documentation from external explorer documentation -->
```text
docs/
packaging/nodeclient/README-node-setup.md
packaging/innbc-peerdirectory-service/README-peerdirectory-service.md
scripts/README.md
```

Explorer documentation is maintained separately. The [INNBCScan guide](https://github.com/InnovativeBioresearch/INNBCscan/blob/main/README.md) covers the explorer package, its shared JSON publisher, production installation, Nginx integration, and service management.
<!-- FINISH ALPHA CHANGE EXPLORER-308 -- distinguish local chain documentation from external explorer documentation -->

Important entry points include:

- `docs/docs/quickstart.md`
- `docs/docs/operate/install.md`
- `docs/docs/operate/deploy/genesis.md`
- `docs/docs/operate/deploy/genesis-validators.md`
- `docs/docs/operate/deploy/start-chain.md`
- `docs/docs/operate/param-reference.md`
- `docs/docs/design/consensus/innbcbft/overview.md`
- `docs/docs/design/libp2p.md`
- `docs/docs/node-env/README.md`

<!-- START ALPHA CHANGE 1930 -- document production installation and runtime ownership workflow -->
## Production Installation And Runtime Ownership

<!-- START ALPHA CHANGE EXPLORER-309 -- document separately maintained explorer service ownership -->
The INNBC blockchain production packages use the shared, unprivileged `innbc:innbc` system account. Their installers create the account and fixed runtime directories automatically, while the operator retains explicit control over validator identities.

The separately maintained INNBCScan package uses its own `innbcscan:innbcscan` system account. Its publisher code, generated website data, and service definition are managed independently and are not installed by the blockchain packages. See the [explorer operations guide](https://github.com/InnovativeBioresearch/INNBCscan/blob/main/README.md).

These service accounts apply to installed production packages. The repository-local `localtestclient` workspace continues to run under the current interactive user and does not create system users, change system ownership, or install files under `/etc`, `/usr/local`, or `/var/lib`.
<!-- FINISH ALPHA CHANGE EXPLORER-309 -- document separately maintained explorer service ownership -->

### 1. Build the production packages

From the repository root, run:

```bash
make nodeclient
```

This produces three separate packages:

```text
dist/innbc-nodeclient/
dist/innbc-peerdirectory-service/
dist/innbc-snapshot-builder/
```

Their responsibilities are intentionally separated:

```text
innbc-nodeclient
→ installs the shared innbc-chain binary
→ installs the canonical genesis when none is already installed
→ installs node configuration examples
→ installs the templated validator systemd service

innbc-peerdirectory-service
→ installs peer-directory configuration
→ installs the peer-directory systemd service
→ creates persistent peer-address storage

innbc-snapshot-builder
→ installs snapshot-builder configuration
→ installs the one-shot snapshot-builder wrapper
→ creates the shared snapshot output directory
```

The nodeclient package must be installed first because the peer directory and snapshot builder both use:

```text
/usr/local/bin/innbc-chain
```

### 2. Upload the packages

The conventional production upload locations are:

```text
/opt/innbc-nodeclient
/opt/innbc-peerdirectory-service
/opt/innbc-snapshot-builder
```

After uploading, each directory should contain its corresponding generated package.

For example:

```text
/opt/innbc-nodeclient/bin/innbc-chain
/opt/innbc-nodeclient/scripts/install.sh
/opt/innbc-nodeclient/systemd/innbc-node@.service

/opt/innbc-peerdirectory-service/scripts/install.sh
/opt/innbc-peerdirectory-service/systemd/innbc-peerdirectory.service

/opt/innbc-snapshot-builder/scripts/install.sh
/opt/innbc-snapshot-builder/scripts/innbc-snapshot-builder
```

### 3. Install the packages

Install the nodeclient first:

```bash
sudo /opt/innbc-nodeclient/scripts/install.sh
```

Then install the peer directory:

```bash
sudo /opt/innbc-peerdirectory-service/scripts/install.sh
```

Finally, install the snapshot builder:

```bash
sudo /opt/innbc-snapshot-builder/scripts/install.sh
```

The installers require root privileges because they install files under:

```text
/usr/local/bin
/etc/innbc
/etc/systemd/system
/var/lib/innbc
/var/log/innbc
```

Root privileges are used only for installation and filesystem preparation. The long-running validator and peer-directory processes do not run as root.

### 4. Automatic creation of the INNBC runtime account

<!-- START ALPHA CHANGE EXPLORER-061 -- scope shared account creation to blockchain packages -->
Each blockchain-package installer checks for the following system identity:
<!-- FINISH ALPHA CHANGE EXPLORER-061 -- scope shared account creation to blockchain packages -->

```text
User:  innbc
Group: innbc
```

If the group does not exist, the installer creates it as a system group.

If the user does not exist, the installer creates it as a system account with:

```text
home directory: /var/lib/innbc
login shell:    /usr/sbin/nologin
primary group:  innbc
```

The home-directory path is an ownership reference for the service account. The installer does not create an interactive home environment, password, or login session.

The account is deliberately non-login. It exists only so INNBC processes can access their own runtime files without receiving root access to the rest of the server.

The separate `innbc` group provides a consistent filesystem ownership boundary:

```text
innbc:innbc
```

The first name is the runtime user, and the second name is its primary group.

<!-- START ALPHA CHANGE EXPLORER-062 -- clarify blockchain account reuse -->
Every blockchain-package installer uses the same account. If `innbc:innbc` already exists with the expected primary group and a non-interactive shell, it is reused. Reinstalling a blockchain package therefore does not create duplicate users or groups. The independent INNBCScan installer uses `innbcscan:innbcscan` instead.
<!-- FINISH ALPHA CHANGE EXPLORER-062 -- clarify blockchain account reuse -->

If an incompatible pre-existing `innbc` account is found, the installer stops instead of silently changing an unrelated system account.

### 5. Installed ownership boundaries

Package-managed executables, system configuration, genesis, and service definitions remain controlled by root:

```text
/usr/local/bin/innbc-chain
/usr/local/bin/innbc-snapshot-builder
/etc/innbc
/etc/systemd/system/innbc-node@.service
/etc/systemd/system/innbc-peerdirectory.service
```

Runtime directories are assigned to the service account:

```text
/var/lib/innbc
/var/log/innbc
/var/lib/innbc/peer-directory
/var/lib/innbc/snapshots
```

The peer-directory cache is stored under:

```text
/var/lib/innbc/peer-directory/peers.json
```

Published snapshots are stored under:

```text
/var/lib/innbc/snapshots/segmented
/var/lib/innbc/snapshots/checkpoint
```

Validator services receive write access only to the declared runtime paths. The systemd units additionally apply restrictions including:

```text
NoNewPrivileges=true
CapabilityBoundingSet=
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
```

This prevents the validator and peer-directory processes from acquiring new privileges and substantially limits the host resources exposed if a process is compromised.

### 6. Validator identities remain operator-managed

The nodeclient installer intentionally does not create, replace, recursively modify, or assume the number of validator identity directories.

Validator identities may be:

- created directly on the server with `node create`;
- prepared with `node prepare` and populated with existing keys;
- uploaded from a protected provisioning environment.

This distinction protects existing validator identities from package installation and upgrades.

A validator directory uses the following layout:

```text
/var/lib/innbc/nodeN/
├── keys/
│   ├── validator-ecdsa/
│   │   └── validator.key
│   ├── validator-bls/
│   │   └── validator-bls.key
│   └── libp2p/
│       └── libp2p.key
└── chain-data/
    ├── blockchain/
    ├── trie/
    └── consensus/
```

The `keys` directory contains the persistent validator identity. The replaceable MDBX runtime databases are created under `chain-data` when the node starts.

### 7. Create fresh validator identities

<!-- START ALPHA CHANGE 3369 -- explain creation limits and subsequent inspection in the production workflow -->
Each invocation creates up to ten new identities. Available automatic numbers range from node1 to node100; later invocations continue into available higher numbers and fill gaps without overwriting reserved identities or environments.

Insufficient capacity is rejected before writing. A later execution failure can leave partial progress. Inspect the installation with `node info all` before registration or startup; the verification step below documents the command.
<!-- FINISH ALPHA CHANGE 3369 -- explain creation limits and subsequent inspection in the production workflow -->

For a fresh six-node private-local installation, run:

```bash
sudo innbc-chain node create 6 \
  --mode private-local
```

For external-validator configuration, run:

```bash
sudo innbc-chain node create 6 \
  --mode external-validator
```

<!-- START ALPHA CHANGE 2741 -- summarize private and external node configuration profiles -->
`--mode private-local` is the default profile for local testing and private deployments. It generates loopback P2P listeners and the local peer-directory URL. `--mode external-validator` provides defaults for externally reachable production validators: wildcard P2P listeners and `https://innbcchain.io/api/v1/peers/exchange`.

Both `node create` and `node prepare` support these profiles. Both include empty `INNBC_NAT_ADDR` and `INNBC_DNS_ADDR` fields for the operator to fill, and both retain loopback JSON-RPC and management gRPC defaults.

These are editable environment presets using the same runtime. For a same-server TCP proxy, keep the node's P2P listener on loopback, advertise the gateway IP or DNS address, and configure forwarding separately. The advertised TCP port currently matches the listening port.

Restart the node after changing its env and use the normal manual peer-directory refresh workflow to distribute updated addresses. Changing networking configuration does not require regenerating the node identity.

See the [Node Environment Reference](docs/docs/node-env/README.md) for the complete defaults, advertised-address syntax, and direct-versus-proxy deployment guidance.
<!-- FINISH ALPHA CHANGE 2741 -- summarize private and external node configuration profiles -->

The default production paths are:

```text
Node identities: /var/lib/innbc/nodeN
Node env files:  /etc/innbc/nodeN.env
Genesis:         /etc/innbc/genesis.json
```

Never regenerate an established validator identity merely to update the client. In particular, when the canonical genesis identifies the initial validator, the corresponding node identity must continue to match that genesis entry.

### 8. Prepare folders for existing identities

When existing validator keys will be uploaded or restored, prepare the directory and environment layout without generating new keys:

```bash
sudo innbc-chain node prepare 6 \
  --mode private-local
```

Or:

```bash
sudo innbc-chain node prepare 6 \
  --mode external-validator
```

Place each identity in its exact prepared paths:

```text
/var/lib/innbc/node1/keys/validator-ecdsa/validator.key
/var/lib/innbc/node1/keys/validator-bls/validator-bls.key
/var/lib/innbc/node1/keys/libp2p/libp2p.key
```

Repeat the same layout for every additional node.

`node prepare` intentionally does not generate missing keys. A prepared node cannot start correctly until all required identity files have been installed.

### 9. Assign validator-directory ownership

Commands executed through `sudo`, manually uploaded directories, and restored identities may initially be owned by `root` or by the upload account.

Before starting a validator, transfer only that validator directory to the INNBC runtime account.

For the standard six-node layout:

```bash
sudo chown -R innbc:innbc /var/lib/innbc/node1
sudo chown -R innbc:innbc /var/lib/innbc/node2
sudo chown -R innbc:innbc /var/lib/innbc/node3
sudo chown -R innbc:innbc /var/lib/innbc/node4
sudo chown -R innbc:innbc /var/lib/innbc/node5
sudo chown -R innbc:innbc /var/lib/innbc/node6
```

Run commands only for node directories that actually exist on that server.

This ownership step gives the unprivileged validator process access to:

```text
identity keys
blockchain database
state trie
consensus database
runtime metadata
```

The environment files under `/etc/innbc` may remain root-owned. Systemd reads the selected environment file before starting the service as `innbc`.

Do not recursively change ownership on broad system paths such as:

```text
/etc
/usr
/usr/local
/opt
/
```

Only the specific validator directories under `/var/lib/innbc` require recursive ownership assignment.

Repeat the ownership command after replacing or uploading identity files if the upload operation changed their owner.

<!-- START ALPHA CHANGE 3369 -- verify stored identities and environments before service startup -->

### 10. Verify validator identities and environments

Inspect one node:

```bash
sudo innbc-chain node info node1
```

Inspect all discovered slots:

```bash
sudo innbc-chain node info all
```

The default paths are `/var/lib/innbc` and `/etc/innbc`. For another installation, pass both `--base-dir` and `--config-dir`. Add `--json` for structured output.

Inspection validates the stored keys and reports the operator address, BLS public key, operator-bound BLS proof, libp2p peer ID, registration multiaddr templates and recognized public environment settings. Private keys are read internally for validation but are not printed.

Missing or invalid environment data does not suppress a valid public identity. Invalid occupied slots produce status 1 after reporting; completely absent interior slots produce warnings. Inspection does not repair or create files.

Compare the reported public identity against the expected genesis or validator-registry identity. Inspection does not perform that external comparison automatically.

This checks stored provisioning data. It does not prove service-account permissions, complete runtime configuration validity or service readiness. See [point 100](#100-bounded-node-creation-and-read-only-identity-inspection) and the [environment reference](docs/docs/node-env/README.md#read-only-node-inspection).

<!-- FINISH ALPHA CHANGE 3369 -- verify stored identities and environments before service startup -->

### 11. Review active configuration

Review the node environment files:

```text
/etc/innbc/node1.env
/etc/innbc/node2.env
/etc/innbc/node3.env
/etc/innbc/node4.env
/etc/innbc/node5.env
/etc/innbc/node6.env
```

Review the peer-directory configuration:

```text
/etc/innbc/peerdirectory.env
```

<!-- START ALPHA CHANGE 1935 -- document mandatory peer-directory runtime configuration -->
This active configuration file is created automatically by the peer-directory installer and preserved during subsequent installations. It is mandatory at runtime: if it is missing or unreadable, systemd fails the peer-directory service before launching the INNBC binary.
<!-- FINISH ALPHA CHANGE 1935 -- document mandatory peer-directory runtime configuration -->

Review the snapshot-builder configuration:

```text
/etc/innbc/snapshot-builder.env
```

The node environment determines addresses, ports, history mode, snapshot bootstrap source, peer-directory endpoint, and MDBX map size for each validator.

### 12. Start the peer directory

<!-- START ALPHA CHANGE 1935 -- document mandatory peer-directory runtime configuration -->
Before startup, systemd requires:

```text
/etc/innbc/peerdirectory.env
```

The peer-directory installer normally creates this file. If it is missing or unreadable, startup fails explicitly instead of using embedded fallback settings.
<!-- FINISH ALPHA CHANGE 1935 -- document mandatory peer-directory runtime configuration -->

Enable and start the peer-directory service:

```bash
sudo systemctl enable --now innbc-peerdirectory
```

Check its status:

```bash
sudo systemctl status innbc-peerdirectory --no-pager
```

The service is launched by systemd as:

```text
innbc:innbc
```

It does not run as root.

### 13. Start validator nodes

Start one validator:

```bash
sudo innbc-chain node start node1
```

Or start every configured validator:

```bash
sudo innbc-chain node start all
```

<!-- START ALPHA CHANGE 1933 -- document mandatory per-node service configuration -->
Every named validator requires its matching production environment file:

```text
innbc-node@node1 → /etc/innbc/node1.env
innbc-node@node2 → /etc/innbc/node2.env
```

If the required file is missing or unreadable, systemd refuses to start that validator before executing `innbc-chain`. It never silently substitutes the template’s node1-oriented ports, history role, or snapshot settings.

This does not alter the normal provisioning workflow: both `node create` and `node prepare` already generate the corresponding environment files.
<!-- FINISH ALPHA CHANGE 1933 -- document mandatory per-node service configuration -->

The management command invokes systemd. Systemd then launches each underlying validator process with:

```text
User=innbc
Group=innbc
```

The operator can continue using the same management commands:

```bash
sudo innbc-chain node status node1
sudo innbc-chain node stop node1
sudo innbc-chain node restart node1
sudo innbc-chain node disable node1
```

Using `sudo` for the management command does not mean the validator runs as root. Root authorizes the systemd operation, while the systemd unit fixes the actual runtime identity to `innbc`.

### 14. Run the snapshot builder

Snapshot creation remains an explicit one-shot operation:

```bash
sudo innbc-snapshot-builder segmented
```

Or:

```bash
sudo innbc-snapshot-builder checkpoint
```

The wrapper initially runs with `sudo` so it can read the root-owned configuration:

```text
/etc/innbc/snapshot-builder.env
```

After validating that configuration, it executes the actual snapshot-building process through:

```text
runuser --user innbc
```

The Go snapshot builder therefore runs as the unprivileged `innbc` account.

Because the builder reads the source full-history node and writes the snapshot output, the source node directory must be readable by `innbc`, and `/var/lib/innbc/snapshots` must be writable by `innbc`. The normal ownership workflow above provides both conditions.

No resident snapshot-builder service is installed. The process exits after the artifact has been validated and published.

### 15. Package updates and reinstallations

The production installers are designed to be rerun during client updates.

On reinstallation:

```text
the existing innbc user and group are reused
the shared binary is updated
systemd unit files are updated
example configuration files are updated
the installed genesis is preserved when already present
active peer-directory configuration is preserved
active snapshot-builder configuration is preserved
validator identity directories are not touched
chain databases are not touched
the peer-directory cache is not deleted
existing snapshots are not deleted
```

For a controlled binary upgrade, stop the affected services, install the updated packages, and start them again.

For example:

```bash
sudo innbc-chain node stop all
sudo systemctl stop innbc-peerdirectory

sudo /opt/innbc-nodeclient/scripts/install.sh
sudo /opt/innbc-peerdirectory-service/scripts/install.sh
sudo /opt/innbc-snapshot-builder/scripts/install.sh

sudo systemctl start innbc-peerdirectory
sudo innbc-chain node start all
```

Recheck validator-directory ownership after an update only if identities or node files were manually uploaded or replaced. Reinstalling the packages alone does not change those directories.

### 16. Local testclient remains independent

The production service-account model does not alter local development.

Commands under:

```text
localtestclient/bin/innbc-chain
```

continue to run as the current local user. They use repository-relative paths and do not require the `innbc` system account.

The production ownership commands in this chapter must not be applied to the local test workspace.

<!-- START ALPHA CHANGE EXPLORER-310 -- refer explorer installation to its standalone repository -->

### 17. Custom INNBCScan Explorer

INNBCScan is a separate application maintained outside this chain repository. Its source code, build tooling, and deployment packages are available from the [INNBCscan repository](https://github.com/InnovativeBioresearch/INNBCscan).

The blockchain packages do not build, install, start, or update the explorer.

For explorer prerequisites, packaging commands, installation, frontend and publisher updates, service ownership, Nginx integration, and recovery procedures, follow the [INNBCScan build, installation, and operations guide](https://github.com/InnovativeBioresearch/INNBCscan/blob/main/README.md).

Public explorer: https://www.innbcscan.io/

<!-- FINISH ALPHA CHANGE EXPLORER-310 -- refer explorer installation to its standalone repository -->

<!-- FINISH ALPHA CHANGE 1930 -- document production installation and runtime ownership workflow -->

## Security And Trust Boundaries

INNBC Chain includes substantial hardening over its Polygon Edge foundation, but node operators must still protect the surrounding infrastructure.

Important trust boundaries include:

- validator private keys
- BLS keys
- libp2p identity keys
- genesis configuration
- system-contract addresses
- governance permissions
- trusted snapshot sources
- peer-directory availability
- public JSON-RPC reverse proxy
- server filesystem and service account

<!-- START ALPHA CHANGE 1931 -- reference the enforced production runtime boundary -->
Production packages run validator and peer-directory processes under the dedicated non-login `innbc:innbc` service account. Installation and systemd management require root authorization, but the persistent application processes do not retain root privileges.
<!-- FINISH ALPHA CHANGE 1931 -- reference the enforced production runtime boundary -->

Public JSON-RPC should be protected with:

- TLS
- request-size limits
- rate limits
- connection limits
- request and upstream timeouts
- method filtering where appropriate
- monitoring and alerting

<!-- START ALPHA CHANGE 3406 -- document mandatory loopback binding for operator gRPC -->
System operator gRPC must bind to a resolved loopback IP. Startup rejects wildcard, LAN and public listener addresses. The native operator listener does not provide an authenticated non-loopback binding mode.
<!-- FINISH ALPHA CHANGE 3406 -- document mandatory loopback binding for operator gRPC -->

Consensus-critical and monetary changes should receive independent review before deployment.

## Polygon Edge Lineage

INNBC Chain was built from the Polygon Edge codebase. Some low-level architecture, dependencies, terminology, and license history therefore retain their upstream origin.

The current runtime should not be operated using obsolete Polygon Edge bridge, rootchain, Supernet, legacy IBFT, bootnode, burn-contract, or child-token instructions.

Current deployment and operation must follow the INNBC-specific documentation in this repository.

## License

<!-- START ALPHA CHANGE 3923 -- explain network permissions attribution and commercial restrictions -->

**Operating software for the official INNBC Chain network requires no software licensing fee or separate written agreement.** This includes permitted commercial network services. The detailed permissions and conditions are provided in [LICENSING.md](LICENSING.md).

### Official Network Participation

Individuals and organizations may build and run the software for participation in the official INNBC Chain network, including node operation, eligible validator operation, and RPC, hosting, maintenance, support, wallet, exchange, explorer, and application services.

Businesses may use the official network. Receiving protocol rewards or transaction fees, trading INNBC, and charging for permitted network services do not by themselves require a separate software license agreement.

Software permission does not confer automatic validator or peer admission. Registration, whitelisting, staking, consensus rules, and other governance and technical requirements remain applicable.

"No software licensing fee" does not mean that transactions or infrastructure have no costs. Protocol fees, required stake, and operating expenses remain applicable.

### Noncommercial Modifications and Derivatives

Noncommercial modifications and derivative works are permitted subject to the attribution, notice-preservation, and other conditions in [LICENSING.md](LICENSING.md).

Examples may include research prototypes, educational experiments, volunteer security-research forks, community platform adaptations, and noncommercial experimental test networks.

"Noncommercial" concerns the activity's purpose. Providing a free download does not automatically make a commercial product noncommercial.

Distributed or publicly offered noncommercial derivatives must identify their origin. A suitable attribution is:

> Based on INNBC Chain, originally developed by [Jonathan Fior](https://github.com/InnovativeBioresearch) at [Innovative Bioresearch](https://www.innovativebioresearch.com/).
>
> Original source: https://github.com/InnovativeBioresearch/INNBC-chain
>
> Modifications by: [name]. No endorsement by the original developer is implied.

Preserve the applicable original notices and identify modifications and their dates. See section 3.3 of [LICENSING.md](LICENSING.md) for attribution placement and distribution conditions.

### Restricted Commercialization

Except for permitted official-network activities or another applicable permission, commercial software derivatives incorporating the protected original INNBC material, independent for-profit chain forks using that material, and software sales or resale require a separate written license agreement.

Attribution does not substitute for that agreement.

These restrictions concern reuse of the protected implementation. Applications are not treated as derivatives of this client merely because they interact with INNBC Chain through its protocol or RPC interfaces.

### Upstream Material and Licensing Scope

Retained Apache-licensed upstream material remains under Apache-2.0. The unchanged Apache license text is reproduced in [LICENSE](LICENSE).

Upstream attribution includes:

Copyright 2022 Polygon Technology.

Original copyrightable INNBC Chain additions and modifications are governed by [LICENSING.md](LICENSING.md).

The [Major Enhancements Over Polygon Edge](#major-enhancements-over-polygon-edge) section currently documents 136 numbered entries. The licensing notice explains how those descriptions, source references, packaging workflows, and annotations identify original implementation material.

Third-party dependencies and material expressly licensed separately retain their respective licenses. These notices do not revoke rights already validly granted under an applicable license or agreement.

The customized distribution is source available with mixed licensing and commercial restrictions. It is not represented as wholly Apache-licensed or wholly open source.

<!-- FINISH ALPHA CHANGE 3923 -- explain network permissions attribution and commercial restrictions -->

<!-- FINISH ALPHA CHANGE 1471 -- document INNBC architecture and enhancements over Polygon Edge -->