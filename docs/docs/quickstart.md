<!-- START ALPHA CHANGE 805 -- rewrite quickstart for current INNBC nodeclient workflow -->
Before proceeding, make sure your system meets the [system requirements](operate/system.md).

This quickstart runs the current INNBC client locally with the local testclient workspace. It does not use the removed legacy bridge, rootchain, bootnode, IBFT, or child token flows.

## Build the local testclient

From the repository root:

```bash
go build -buildvcs=false .
make localtestclient
```

This creates:

```text
localtestclient/
  bin/innbc-chain
  genesis.json
  config/
  nodes/
```

## Prepare a validator folder

To create an empty folder structure for one validator:

<!-- START ALPHA CHANGE 1873 -- use local genesis in quickstart node generation -->
<!-- START ALPHA CHANGE 1974 -- use the shared local snapshot root -->
```bash
./localtestclient/bin/innbc-chain node prepare 1 \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config \
  --chain localtestclient/genesis.json \
  --snapshot-root localtestclient/snapshots \
  --mode private-local
```
<!-- FINISH ALPHA CHANGE 1974 -- use the shared local snapshot root -->

To create keys automatically instead:

<!-- START ALPHA CHANGE 1975 -- use the shared local snapshot root -->
```bash
./localtestclient/bin/innbc-chain node create 1 \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config \
  --chain localtestclient/genesis.json \
  --snapshot-root localtestclient/snapshots \
  --mode private-local
```
<!-- FINISH ALPHA CHANGE 1975 -- use the shared local snapshot root -->
<!-- FINISH ALPHA CHANGE 1873 -- use local genesis in quickstart node generation -->

If you prepared an empty node folder, copy the existing validator key files into:

<!-- START ALPHA CHANGE 1643 -- document canonical localtest identity paths -->
```text
localtestclient/nodes/node1/keys/validator-ecdsa/validator.key
localtestclient/nodes/node1/keys/validator-bls/validator-bls.key
localtestclient/nodes/node1/keys/libp2p/libp2p.key
```
<!-- FINISH ALPHA CHANGE 1643 -- document canonical localtest identity paths -->

<!-- START ALPHA CHANGE 3371 -- inspect local provisioning without modifying identities -->

## Validate and inspect local identities

Use `node prepare` with manually supplied keys or `node create` with generated keys. They are alternative provisioning paths.

Creation accepts up to ten new identities per invocation and allocates available numbers within node1–node100. A failed creation can leave partial progress; inspection reports the files that remain without repairing them.

Inspect one node:

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

The command validates stored keys and independently checks the matching environment files. Public registration information includes:

```text
operator address
BLS public key
operator-bound BLS proof
libp2p peer ID
private-local multiaddr
external-validator multiaddr template
```

Private keys are read internally but are not printed. Inspection does not create directories, repair files or regenerate identities.

Invalid occupied slots cause a nonzero exit after reporting. Completely absent interior slots produce warnings. No missing trailing node is inferred beyond the highest discovered identity or environment path.

See the [inspection reference](node-env/README.md#read-only-node-inspection) for validation scope and limitations.

<!-- FINISH ALPHA CHANGE 3371 -- inspect local provisioning without modifying identities -->

## Start the peer directory

In a separate terminal:

<!-- START ALPHA CHANGE 1858 -- provide complete quickstart peer directory command -->
```bash
./localtestclient/bin/innbc-chain peerdirectory \
  --listen 127.0.0.1:8080 \
  --jsonrpc http://127.0.0.1:8545 \
  --validator-registry 0x0000000000000000000000000000000000000107 \
  --chain localtestclient/genesis.json
```
<!-- FINISH ALPHA CHANGE 1858 -- provide complete quickstart peer directory command -->

The peer directory exposes the authenticated validator peer exchange endpoint:

```text
/api/v1/peers/exchange
```

## Start the validator node

In another terminal:

<!-- START ALPHA CHANGE 1874 -- document complete local full-history node command -->
```bash
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
  --mdbx-page-size-bytes 4096 # START ALPHA CHANGE 3506 -- document quickstart database geometry --
# FINISH ALPHA CHANGE 3506 -- document quickstart database geometry --
```
<!-- FINISH ALPHA CHANGE 1874 -- document complete local full-history node command -->

<!-- START ALPHA CHANGE 1248 -- document startup-once peer directory refresh in quickstart -->
For a single-validator bootstrap, the chain can start even before the peer directory has returned peers. With peer-directory discovery enabled, the node refreshes its validator peer list once at startup.
<!-- FINISH ALPHA CHANGE 1248 -- document startup-once peer directory refresh in quickstart -->

## Check block production

```bash
curl -s "http://127.0.0.1:8545" -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

<!-- START ALPHA CHANGE 1249 -- document manual running-node peer directory refresh in quickstart -->
## Refresh peer discovery manually

After another validator is added, removed, whitelisted, unwhitelisted, restarted with a new libp2p address, or after the peer-directory service starts later than a node's startup refresh, refresh each running validator through local system gRPC:

```bash
./localtestclient/bin/innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9632
```

The command prints the current validator peer multiaddrs returned by the peer directory. Each refresh replaces the node's local validator peer allowlist with the latest directory response.
<!-- FINISH ALPHA CHANGE 1249 -- document manual running-node peer directory refresh in quickstart -->

## Build the production nodeclient bundle

```bash
make nodeclient
```

The production bundle is written to:

```text
dist/innbc-nodeclient
```

The bundle contains the node binary, genesis file, config templates, systemd service template, and setup README.
<!-- FINISH ALPHA CHANGE 805 -- rewrite quickstart for current INNBC nodeclient workflow -->