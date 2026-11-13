<!-- START ALPHA CHANGE 806 -- rewrite deployment index for current INNBC workflow -->
This section explains how to deploy and operate the current INNBC validator node software.

The current INNBC deployment flow does not use the removed legacy bridge, rootchain deployment, SupernetManager, StakeManager, bootnodes, or child token templates. Validator onboarding is handled by the INNBC validator registry and validator peer discovery is handled by the INNBC peer directory.

## Prerequisites

Before deployment, make sure the host satisfies the system requirements in [System Requirements](../system.md).

## Deployment Model

A production installation is built from the repository with:

```bash
make nodeclient
```

This creates:

```text
dist/innbc-nodeclient/
  bin/innbc-chain
  config/genesis.json
  config/node.example.yaml
  config/node.private-local.env.example
  config/node.external-validator.env.example
  scripts/install.sh
  systemd/innbc-node@.service
  README-node-setup.md
```

The installer places runtime files under the normal Linux locations:

```text
/usr/local/bin/innbc-chain
/etc/innbc/
/var/lib/innbc/
/var/log/innbc/
```

The installer does not create validator identities. Validator identity folders are created later with explicit node commands.

## Validator Profiles

Two validator profiles are supported:

```text
private-local
external-validator
```

`private-local` is useful for local testing and controlled single-host deployments.

`external-validator` is useful for validators that must communicate across hosts or networks.

## Main Workflow

1. Generate or provide `genesis.json`.
2. Build the production nodeclient bundle.
3. Install the bundle on the server.
4. Create or prepare validator identity folders.
<!-- START ALPHA CHANGE 3373 -- include read-only provisioning checks in deployment -->
5. Inspect stored identities and environments read-only, then collect public validator information.
<!-- FINISH ALPHA CHANGE 3373 -- include read-only provisioning checks in deployment -->
6. Register validators in the INNBC validator registry.
7. Start the peer directory service.
8. Start validator node services.

## Important Commands

Build the production bundle:

```bash
make nodeclient
```

Prepare empty validator folders:

```bash
sudo innbc-chain node prepare
sudo innbc-chain node prepare 10 --mode private-local
sudo innbc-chain node prepare 10 --mode external-validator
```

Create validator folders and keys:

```bash
sudo innbc-chain node create
sudo innbc-chain node create 10 --mode private-local
```

<!-- START ALPHA CHANGE 3373 -- document individual and complete discovered-slot inspection -->
Creation accepts up to ten new identities per invocation and allocates available numbers within node1–node100. A later execution failure can leave partial progress.

Inspect one stored identity and its environment:

```bash
sudo innbc-chain node info node1
```

Inspect all discovered slots:

```bash
sudo innbc-chain node info all
```

Add `--json` for structured output. Inspection is read-only and prints public identity information, including the operator-bound BLS proof, without printing private keys.

Incomplete occupied slots produce a nonzero exit after reporting. Interior absent slots produce warnings. Inspection does not repair files or establish complete service readiness.

For custom paths, pass both `--base-dir` and `--config-dir`. See the [inspection reference](../../node-env/README.md#read-only-node-inspection).
<!-- FINISH ALPHA CHANGE 3373 -- document individual and complete discovered-slot inspection -->

Start, stop, restart, and inspect nodes:

```bash
sudo innbc-chain node start node1
sudo innbc-chain node stop node1
sudo innbc-chain node restart node1
sudo innbc-chain node status node1
```

Use `all` to apply service operations to all configured node services:

```bash
sudo innbc-chain node start all
sudo innbc-chain node restart all
sudo innbc-chain node status all
```

## Next Steps

Use these pages in order:

1. [Configure Genesis](genesis.md)
2. [Configure Initial Validators](genesis-validators.md)
3. [Start the Chain](start-chain.md)
4. [Upgrade Nodes](upgrades/v1.1.md)
<!-- FINISH ALPHA CHANGE 806 -- rewrite deployment index for current INNBC workflow -->