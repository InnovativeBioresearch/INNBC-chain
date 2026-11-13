<!-- START ALPHA CHANGE 1031 -- replace legacy Polygon overview with current INNBC overview -->

# INNBC Chain

INNBC Chain is a sovereign EVM-compatible blockchain built on a customized Polygon Edge foundation.

The current INNBC runtime uses INNBC BFT consensus, native INNBC gas accounting, validator registry governance, authenticated peer discovery, systemd-managed validator services, compact MDBX storage, physically reclaimable segmented recent history, history-aware synchronization, and independent segmented or checkpoint snapshots.

## Current Architecture

INNBC Chain is designed around a standalone validator network.

It does not use the removed legacy Polygon Edge bridge, rootchain deployment, StakeManager, SupernetManager, bootnode, burn-contract, IBFT, or child-token flows.

The core runtime pieces are:

| Component | Purpose |
| --- | --- |
| INNBC BFT consensus | Finalizes blocks through validator voting and proposer rotation. |
| Native INNBC token | Provides the gas token and ERC-20-compatible native token surface. |
| Validator registry | Stores validator metadata, stake eligibility, whitelist status, and active validator state. |
| Validator governance | Allows active validators to manage validator admission through proposals. |
| Peer directory | Provides authenticated validator peer exchange through the INNBC peer-directory API. |
| Segmented history | Bounds recent blockchain history by physically deleting expired closed MDBX segment directories. |
| Snapshot builder | Creates validated segmented recent-node artifacts and checkpoint Plan B artifacts without an in-node producer loop. |
| History-aware sync | Advertises each peer’s continuous history floor and selects only peers that can serve the requested range. |
| Nodeclient package | Installs the production binary, genesis, examples, systemd service, and node management workflow. |

## Polygon Edge Lineage

INNBC Chain was built from the Polygon Edge codebase, so some low-level architecture, terminology, and upstream dependencies still reflect that foundation.

Historical Polygon Edge references are kept only where they explain origin or upstream protocol lineage. Current deployment and operation should follow the INNBC nodeclient, localtestclient, peer-directory, and validator registry documentation.

<!-- FINISH ALPHA CHANGE 1031 -- replace legacy Polygon overview with current INNBC overview -->