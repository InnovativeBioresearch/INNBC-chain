<!-- START ALPHA CHANGE 1024 -- replace legacy cluster script docs with current INNBC build script docs -->

# INNBC Scripts

This directory contains repository helper scripts for building the current INNBC client packages.

The removed legacy Polygon Edge cluster helper is no longer supported. Current local testing uses the `localtestclient` workspace, and production packaging uses the `nodeclient` bundle.

## Build Local Test Workspace

```bash
make localtestclient
```

This creates:

```text
localtestclient/
```

The local test workspace is used for local node testing, local genesis checks, MetaMask RPC checks, gas-fee checks, snapshot tests, and validator registration rehearsal.

## Build Production Nodeclient Bundle

```bash
make nodeclient
```

This creates:

```text
dist/innbc-nodeclient/
dist/innbc-peerdirectory-service/
dist/innbc-snapshot-builder/
```

The nodeclient bundle contains the production binary, genesis copy, systemd service file, example node env files, and installer script.

<!-- START ALPHA CHANGE 3382 -- document the source of distributed provisioning and inspection instructions -->
The production setup guide and configuration examples are copied from `packaging/nodeclient`. Update those source files rather than generated files under `dist/innbc-nodeclient`.

The local workspace copies the same setup guide and generates its local README and configuration examples from `scripts/build-localtestclient.sh`.

Both workflows document read-only post-provisioning inspection with `node info nodeN` and `node info all`. Custom installations must supply matching `--base-dir` and `--config-dir` values.

Creation permits partial progress after an execution failure. Inspection reports incomplete stored identities or environments without modifying or repairing them.
<!-- FINISH ALPHA CHANGE 3382 -- document the source of distributed provisioning and inspection instructions -->

The peer-directory service bundle is separate and installs only peer-directory service files. It does not touch the node binary, node env files, genesis, validator keys, or chain data.

<!-- START ALPHA CHANGE 1850 -- document standalone snapshot builder distribution -->
The snapshot-builder bundle installs its own preserved environment file and one-shot `innbc-snapshot-builder` runner. It has no systemd service and runs only when explicitly invoked to produce a segmented or checkpoint snapshot.
<!-- FINISH ALPHA CHANGE 1850 -- document standalone snapshot builder distribution -->

## Removed Legacy Flow

The old local cluster helper was removed because it depended on legacy Polygon Edge rootchain, bridge, StakeManager, SupernetManager, IBFT, and burn-contract flows that are not part of current INNBC Chain.

<!-- FINISH ALPHA CHANGE 1024 -- replace legacy cluster script docs with current INNBC build script docs -->