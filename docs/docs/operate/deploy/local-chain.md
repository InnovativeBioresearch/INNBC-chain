<!-- START ALPHA CHANGE 1029 -- replace legacy local child-chain docs with localtestclient workflow -->

# Build A Local Test Workspace

The current INNBC local testing flow uses the `localtestclient` workspace.

It does not use the removed legacy Polygon Edge cluster script, IBFT flow, rootchain deployment, bridge setup, StakeManager, SupernetManager, or burn-contract configuration.

## Build The Workspace

From the repository root:

```bash
make localtestclient
```

This creates:

```text
localtestclient/
  bin/innbc-chain
  genesis.json
  config/
  nodes/
  README-localtestclient.md
```

## Prepare Local Nodes

Prepare validator folders and env files:

<!-- START ALPHA CHANGE 1976 -- prepare local nodes with the shared snapshot root -->
```bash
./localtestclient/bin/innbc-chain node prepare 2 \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config \
  --chain localtestclient/genesis.json \
  --snapshot-root localtestclient/snapshots \
  --mode private-local
```
<!-- FINISH ALPHA CHANGE 1976 -- prepare local nodes with the shared snapshot root -->

Or create keys and env files together:

<!-- START ALPHA CHANGE 1977 -- create local nodes with the shared snapshot root -->
```bash
./localtestclient/bin/innbc-chain node create 2 \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config \
  --chain localtestclient/genesis.json \
  --snapshot-root localtestclient/snapshots \
  --mode private-local
```
<!-- FINISH ALPHA CHANGE 1977 -- create local nodes with the shared snapshot root -->

<!-- START ALPHA CHANGE 3372 -- validate local identities and environments after provisioning -->

## Inspect Validator Information

Creation accepts up to ten new identities per invocation, using available numbers within node1–node100. A later execution failure can leave partial progress.

After creation, or after copying all keys into prepared folders, inspect individual nodes:

```bash
./localtestclient/bin/innbc-chain node info node1 \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config

./localtestclient/bin/innbc-chain node info node2 \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config
```

Or inspect all discovered slots:

```bash
./localtestclient/bin/innbc-chain node info all \
  --base-dir localtestclient/nodes \
  --config-dir localtestclient/config
```

Add `--json` for structured output.

Inspection validates stored keys, prints public registration information and independently checks recognized environment settings. It does not print private keys, repair incomplete slots or modify files.

Completely absent interior slots produce warnings. Incomplete or invalid occupied slots produce a nonzero exit after reporting, while later nodes remain visible.

See the [inspection reference](../../node-env/README.md#read-only-node-inspection) for discovery rules and validation limits.

<!-- FINISH ALPHA CHANGE 3372 -- validate local identities and environments after provisioning -->

## Next Steps

Start the peer-directory service and validator nodes using the commands in the generated localtestclient README.

<!-- FINISH ALPHA CHANGE 1029 -- replace legacy local child-chain docs with localtestclient workflow -->