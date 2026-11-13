<!-- START ALPHA CHANGE 1038 -- replace legacy bootnode libp2p docs with INNBC peer-directory networking docs -->

## Overview

INNBC Chain uses a decentralized networking layer based on libp2p.

The networking layer provides peer-to-peer connectivity, secure identity handshakes, transaction gossip, consensus messaging, and peer connection management.

## Identity

Each node has a LibP2P identity key. The identity service validates incoming connections and manages peer handshakes.

Validator nodes use their LibP2P peer ID as part of their public validator metadata.

## Peer Discovery

Current INNBC validator discovery is centered on the authenticated peer-directory service.

<!-- START ALPHA CHANGE 1246 -- document startup-once peer directory refresh model -->
Validators publish signed peer information to the peer directory. The smart contract remains the source of truth for approved `libp2pPeerID` values, while the peer directory provides the current dial multiaddr for each approved peer ID.

Nodes refresh peer information once at startup, and operators can trigger a manual refresh on a running node when validator peer data changes. The peer directory stores one current multiaddr per approved `libp2pPeerID`; if the same peer ID publishes a new address, the new address replaces the old one.
<!-- FINISH ALPHA CHANGE 1246 -- document startup-once peer directory refresh model -->

The current INNBC flow does not use the removed legacy bootnode genesis flow.

## Peer Directory Endpoint

A node can be started with a peer-directory exchange URL:

```bash
--peer-directory-enabled=true \
--peer-directory-url http://127.0.0.1:8080/api/v1/peers/exchange
```

<!-- START ALPHA CHANGE 1247 -- document manual peer directory refresh command -->
After validator additions, removals, whitelist changes, libp2p address changes, or a peer-directory startup race, refresh every running validator through local system gRPC:

```bash
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9632
```

A refresh replaces the local validator peer allowlist. Peers removed from the refreshed allowlist are disconnected at the network layer; consensus membership remains enforced separately by the active validator set.
<!-- FINISH ALPHA CHANGE 1247 -- document manual peer directory refresh command -->

The peer-directory service itself is started separately:

```bash
innbc-chain peerdirectory \
  --listen 127.0.0.1:8080 \
  --chain genesis.json \
  --validator-registry 0x0000000000000000000000000000000000000107 \
  --jsonrpc http://127.0.0.1:8545
```

## Gossipsub

Gossipsub is the peer-to-peer message propagation protocol used by INNBC Chain to broadcast network messages efficiently.

It is used by components such as the transaction pool and consensus runtime to relay data between connected peers.

<!-- FINISH ALPHA CHANGE 1038 -- replace legacy bootnode libp2p docs with INNBC peer-directory networking docs -->