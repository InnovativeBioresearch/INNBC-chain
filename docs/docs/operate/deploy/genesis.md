<!-- START ALPHA CHANGE 808 -- rewrite genesis docs for current INNBC genesis model -->
This section explains how to create the INNBC genesis file.

The current INNBC genesis flow uses INNBCBFT consensus, the INNBC native token, the INNBC validator registry, and the INNBC governance contracts. It does not use the removed legacy bridge, rootchain, bootnode, SupernetManager, StakeManager, reward-token, reward-wallet, burn-contract, or child-token flows.

## Overview

The genesis file defines the initial chain state:

```text
chain name
chain ID
initial validator set
native token configuration
premined balances
validator registry settings
governance settings
system contract allocations
```

The genesis file is created with:

```bash
./innbc-chain genesis
```

or, from the repository root during development:

```bash
go run . genesis
```

## Common Flags

| Flag | Description |
| --- | --- |
| `--dir` | Output path for the genesis file. |
| `--chain-id` | EVM chain ID. |
| `--name` | Human-readable chain name. |
| `--consensus` | Consensus engine. INNBC uses `innbcbft`. |
| `--validators` | Initial validators in `<multiaddr>:<ECDSA address>:<BLS public key>:<BLS proof>` format. |
<!-- START ALPHA CHANGE 2044 -- require possession proofs for every genesis validator -->
Every genesis validator must provide a BLS proof of possession bound to its operator address and exact public key. Local-secret genesis generation creates the proof automatically. Explicit validator entries require the proof printed by the revised `node info` command.

Imported genesis files without valid proofs are rejected. The proved validator list must exactly match the canonical validator metadata in genesis extra data. Generate the launch genesis with the rebuilt binary and rebuilt contract artifacts.
<!-- FINISH ALPHA CHANGE 2044 -- require possession proofs for every genesis validator -->
| `--validators-path` | Directory containing validator identity folders. |
| `--validators-prefix` | Prefix used when reading validator identity folders. |
| `--premine` | Premined account and balance in `<address>:<balance>` format. |
| `--native-token-config` | Native token configuration in `<name:symbol:decimals>` format. |
| `--validator-registry-minimum-stake` | Minimum stake required for validator registry eligibility. |
| `--validator-registry-proposal-duration-preset` | Governance proposal duration preset: `short`, `standard`, or `long`. |
| `--block-gas-limit` | Maximum gas allowed in each block. |
| `--block-time` | Target block time. |
| `--epoch-size` | Number of blocks in an epoch. |
| `--sprint-size` | Number of blocks in a sprint. |

## Native Token Configuration

INNBC uses `INNBCNativeToken` as the mandatory native token implementation.

Use:

```text
--native-token-config "INNBC:INNBC:18"
```

The native token is initialized as a system contract at genesis. Emergency block reward issuance is controlled by INNBC governance, not by an arbitrary mint owner.

## Initial Validators

<!-- START ALPHA CHANGE 3375 -- collect proof-bearing genesis inputs through read-only inspection -->
Validators can be supplied directly:

```bash
go run . genesis --validators "/ip4/127.0.0.1/tcp/30301/p2p/<peer-id>:<validator-address>:<bls-public-key>:<bls-proof>"
```

The validator fields are:

```text
P2P multiaddr
ECDSA validator address
BLS public key
operator-bound BLS proof
```

After creating an identity, or after installing all keys into a prepared folder, inspect the local files:

```bash
./localtestclient/bin/innbc-chain node info node1 \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config
```

For production installations:

```bash
sudo innbc-chain node info node1
```

Use `node info all` to inspect all discovered slots, or add `--json` for structured output.

Inspection validates stored keys, derives public registration values and independently checks the environment. It does not print private keys, repair files or automatically compare the identity with an existing genesis or registry.

See the [inspection reference](../../node-env/README.md#read-only-node-inspection).
<!-- FINISH ALPHA CHANGE 3375 -- collect proof-bearing genesis inputs through read-only inspection -->

## Example Genesis Command

<!-- START ALPHA CHANGE 3375 -- supply the mandatory public proof in the complete genesis example -->
```bash
go run . genesis --dir genesis.json --chain-id 1982 --name innbc-mainnet --consensus innbcbft --validators "/ip4/127.0.0.1/tcp/30301/p2p/<peer-id>:<validator-address>:<bls-public-key>:<bls-proof>" --premine 0x0000000000000000000000000000000000000000:1000000000000000000000000000 --premine <operator-address>:1000000000000000000000000 --native-token-config "INNBC:INNBC:18" --validator-registry-minimum-stake 1 --validator-registry-proposal-duration-preset standard
```

Replace:

```text
<peer-id>
<validator-address>
<bls-public-key>
<bls-proof>
<operator-address>
```
<!-- FINISH ALPHA CHANGE 3375 -- supply the mandatory public proof in the complete genesis example -->

with the values for the intended launch configuration.

## Validator Registry Settings

The validator registry minimum stake is configured at genesis:

```bash
--validator-registry-minimum-stake 1
```

The governance proposal duration preset is configured with:

```bash
--validator-registry-proposal-duration-preset standard
```

Allowed presets are:

```text
short
standard
long
```

## Genesis And Peer Directory

The genesis file includes the initial validator set. This lets a single initial validator bootstrap the chain before registry-driven peer discovery has returned peers.

<!-- START ALPHA CHANGE 1245 -- document startup-once and manual peer directory refresh workflow -->
Validator nodes refresh peer information once at startup. After validator additions, removals, whitelist changes, libp2p address changes, or peer-directory startup races, manually refresh existing running nodes through their local gRPC address:

```bash
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9632
```

For multiple local nodes, refresh each running node separately:

```bash
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9632
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9633
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9634
```

The peer directory returns validator peer multiaddrs only to authenticated validator peers. On every exchange, it refreshes its stored peer list against the validator registry, keeps one current multiaddr per approved `libp2pPeerID`, and removes peer IDs that are no longer approved.
<!-- FINISH ALPHA CHANGE 1245 -- document startup-once and manual peer directory refresh workflow -->

## Production Bundle

After generating or updating `genesis.json`, rebuild the production nodeclient bundle:

```bash
make nodeclient
```

The bundle copies the root `genesis.json` into:

```text
dist/innbc-nodeclient/config/genesis.json
```

The genesis file is not regenerated by `make nodeclient`; it is copied from the repository root.
<!-- FINISH ALPHA CHANGE 808 -- rewrite genesis docs for current INNBC genesis model -->