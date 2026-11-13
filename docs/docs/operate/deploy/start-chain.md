<!-- START ALPHA CHANGE 1030 -- replace legacy polygon-edge start-chain docs with INNBC service workflow -->

# Start Validator Services

The current INNBC production deployment uses systemd-managed validator node services.

It does not use the removed legacy Polygon Edge cluster script, rootchain deployment, bridge setup, StakeManager, SupernetManager, or burn-contract configuration.

<!-- START ALPHA CHANGE 1478 -- document explicit validator and peer directory lifecycle ownership -->

## Understand Runtime And Boot State

Systemd tracks whether a service is running separately from whether it should start automatically at boot:

```text
active:
  the service is running now

inactive:
  the service is not running now

enabled:
  the service is configured to start automatically at boot

disabled:
  the service is not configured to start automatically at boot
```

A service can therefore be:

```text
active and enabled:
  running now and configured to return after reboot

active and disabled:
  running now but not configured to return after reboot

inactive and enabled:
  stopped now but configured to start at the next reboot

inactive and disabled:
  stopped now and not configured to start at boot
```

Creating or preparing a validator identity does not mean that its service should be enabled. Only validators deliberately intended and authorized to run should be started or enabled.

## INNBC Node Command Semantics

Start one validator and enable it at boot:

```bash
sudo innbc-chain node start node1
```

The command executes the equivalent of:

```bash
sudo systemctl enable --now innbc-node@node1.service
```

It has two effects:

```text
1. node1 starts immediately
2. node1 is enabled for future server boots
```

Stop node1 temporarily:

```bash
sudo innbc-chain node stop node1
```

This executes the equivalent of:

```bash
sudo systemctl stop innbc-node@node1.service
```

It stops node1 now but preserves its boot enablement. If node1 remains enabled, it will start again at the next server boot.

Stop node1 and remove its boot enablement:

```bash
sudo innbc-chain node disable node1
```

This executes the equivalent of:

```bash
sudo systemctl disable --now innbc-node@node1.service
```

Restart node1 without changing whether it is enabled:

```bash
sudo innbc-chain node restart node1
```

Check node1:

```bash
sudo innbc-chain node status node1
```

## Direct Systemd Command Semantics

Use systemd directly when runtime and boot state must be controlled separately.

Start node1 now without changing boot enablement:

```bash
sudo systemctl start innbc-node@node1.service
```

Stop node1 now without changing boot enablement:

```bash
sudo systemctl stop innbc-node@node1.service
```

Enable node1 at future boots without starting it now:

```bash
sudo systemctl enable innbc-node@node1.service
```

Disable future boot startup without stopping a currently running node:

```bash
sudo systemctl disable innbc-node@node1.service
```

Start and enable node1 together:

```bash
sudo systemctl enable --now innbc-node@node1.service
```

Stop and disable node1 together:

```bash
sudo systemctl disable --now innbc-node@node1.service
```

A critical systemd rule is:

```text
systemctl enable:
  does not start the service immediately unless --now is used
  does not require the service to have been started previously
  causes the service to start automatically at the next boot
```

Do not enable unused validators in preparation for possible future use. An enabled validator can begin running after a server reboot even if it has never previously been started manually.

## Start An Intended Validator

<!-- START ALPHA CHANGE 1933 -- document mandatory per-node service configuration -->
Before starting a validator, confirm that its matching environment file exists:

```text
innbc-node@node1 → /etc/innbc/node1.env
innbc-node@node2 → /etc/innbc/node2.env
```

The production systemd unit treats this file as mandatory. A missing or unreadable file causes the service to fail before the node binary is launched, preventing accidental fallback to another node’s ports or runtime role.
<!-- FINISH ALPHA CHANGE 1933 -- document mandatory per-node service configuration -->

For a persistent validator that should return after server reboot:

```bash
sudo innbc-chain node start node1
```

For a temporary run that must not change boot configuration:

```bash
sudo systemctl start innbc-node@node1.service
```

Check runtime and boot state separately:

```bash
systemctl is-active innbc-node@node1.service
systemctl is-enabled innbc-node@node1.service
```

Verify its JSON-RPC endpoint when configured:

```bash
cast block-number --rpc-url http://127.0.0.1:8545
```

## Start The Peer Directory Independently

The peer-directory and validator services have independent lifecycle ownership.

Starting peer-directory must not start, stop, enable, disable, or restart any validator.

The packaged peer-directory unit depends only on network availability:

```ini
After=network-online.target
Wants=network-online.target
```

It must not contain:

```ini
Wants=innbc-node@node1.service
```

Start peer-directory temporarily without changing boot enablement:

```bash
sudo systemctl start innbc-peerdirectory.service
```

Start peer-directory and enable it at boot:

```bash
sudo systemctl enable --now innbc-peerdirectory.service
```

Stop peer-directory temporarily:

```bash
sudo systemctl stop innbc-peerdirectory.service
```

Stop peer-directory and remove boot enablement:

```bash
sudo systemctl disable --now innbc-peerdirectory.service
```

Check its states separately:

```bash
systemctl is-active innbc-peerdirectory.service
systemctl is-enabled innbc-peerdirectory.service
```

Confirm that its effective dependencies do not include a validator:

```bash
systemctl show innbc-peerdirectory.service \
  -p Wants \
  -p After
```

The configured JSON-RPC endpoint is a functional data dependency, not lifecycle ownership. Peer-directory reads authoritative validator-registry state through that endpoint when processing exchanges, but it does not control the process providing the endpoint.

For normal operation, start the intended validator or RPC backend before expecting registry-dependent exchanges to succeed.

## Enable Only Intended Validators

Enable approved validators individually:

```bash
sudo systemctl enable innbc-node@node1.service
sudo systemctl enable innbc-node@node2.service
```

Do not enable node3 through node6 unless those validators are deliberately intended to run.

Audit every configured validator:

```bash
systemctl is-active innbc-node@node{1..6}.service
systemctl is-enabled innbc-node@node{1..6}.service
```

An unused validator should normally report:

```text
inactive
disabled
```

## Start All Configured Nodes

The following command starts and enables every configured validator:

```bash
sudo innbc-chain node start all
```

It applies the equivalent of `systemctl enable --now` to every configured node discovered by the node-management command.

Use `node start all` only when every configured validator is intentionally approved to run and return after reboot.

For staged validator admission, start each approved validator explicitly:

```bash
sudo innbc-chain node start node1
```

<!-- FINISH ALPHA CHANGE 1478 -- document explicit validator and peer directory lifecycle ownership -->

<!-- START ALPHA CHANGE 1251 -- document manual peer directory refresh after validator changes -->
## Refresh Peer Discovery

Nodes refresh peer-directory data once at startup. Refresh every running validator through its local gRPC address after:

- validator addition;
- validator removal;
- whitelist or unwhitelist changes;
- validator libp2p address changes;
- peer-directory service startup when nodes may have attempted their one-shot startup refresh too early.

```bash
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9632
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9633
innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9634
```
<!-- FINISH ALPHA CHANGE 1251 -- document manual peer directory refresh after validator changes -->

## Inspect Logs

```bash
sudo journalctl -u innbc-node@node1 -f
sudo journalctl -u innbc-peerdirectory -f
```

<!-- FINISH ALPHA CHANGE 1030 -- replace legacy polygon-edge start-chain docs with INNBC service workflow -->