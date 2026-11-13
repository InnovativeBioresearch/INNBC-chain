<!-- START ALPHA CHANGE 999 -- document standalone peer directory service package -->
# INNBC Peer Directory Service

This package installs only the INNBC validator peer directory service wiring.

Prerequisite:

/usr/local/bin/innbc-chain

The binary is installed by the separate innbc-nodeclient package.

<!-- START ALPHA CHANGE 2235 -- document native directory overload protection -->
## Native Request Limits

The directory applies the same native controls to loopback and externally forwarded requests:

| Setting | Default |
| --- | --- |
| `INNBC_PEER_DIRECTORY_REQUEST_RATE` | 20 admitted requests/second |
| `INNBC_PEER_DIRECTORY_REQUEST_BURST` | 64 |
| `INNBC_PEER_DIRECTORY_PEER_INTERVAL` | 5s per replenished identity token |
| `INNBC_PEER_DIRECTORY_PEER_BURST` | 8 |
| `INNBC_PEER_DIRECTORY_MAX_CONCURRENT` | 32 admitted exchanges, including registry waiters |
| `INNBC_PEER_DIRECTORY_MAX_TRACKED_PEERS` | 4096 |

Set overrides in `/etc/innbc/peerdirectory.env`. Existing env files are preserved during installation; the service supplies the defaults when these new settings are absent.

Global admission happens before body decoding and signature verification. Identity-specific admission happens only after signature verification, so an unsigned claim to another validator's peer ID cannot consume that validator's identity allowance.

Registry authorization remains authoritative. Neither the persistent peer-address JSON nor the rate-limiter map grants validator membership.

Exhausted rate allowances return HTTP 429. Exhausted concurrency or identity-tracking capacity returns HTTP 503. Both include `Retry-After`. These responses do not ban a peer or change its whitelist status.

The tracked-identity map has a fixed capacity. It replaces only sufficiently idle entries, after their token buckets could naturally refill. It does not maintain an unbounded map of arbitrary identities.

The server does not log every rejected request.

Node startup/manual refresh and the explicit directory exchange command retry HTTP 429 and 503 up to five total attempts, with backoff and jitter. Retry delays are capped at ten seconds before jitter, and the complete client operation has a 60-second deadline. Node shutdown cancels its exchange. Authentication failures, other HTTP errors and transport failures are not automatically retried.

A startup refresh that exhausts its retries still reports failure and may require an operator-triggered refresh. There is no new periodic refresh loop.

The directory uses HTTP/HTTPS for registry RPC, with a five-second timeout per call and a four-MiB response limit. One allowlist lookup can make multiple calls; this is not a five-second deadline for the whole lookup.

These controls bound application work but do not guarantee service availability against every flood or protect network bandwidth from saturation. They can complement Nginx controls. Tune rates, burst allowances and concurrency for validator startup patterns and measured registry latency.
<!-- FINISH ALPHA CHANGE 2235 -- document native directory overload protection -->

<!-- START ALPHA CHANGE 1929 -- document the unprivileged production ownership boundary -->
The installer creates or reuses the non-login `innbc:innbc` system account. The peer-directory service runs as that account rather than as root.

The installer assigns only `/var/lib/innbc/peer-directory` to the runtime account. The binary, configuration, genesis, and systemd unit remain root-owned. Reinstalling the package preserves the existing authenticated peer cache.
<!-- FINISH ALPHA CHANGE 1929 -- document the unprivileged production ownership boundary -->

Installed configuration and service files:

/etc/innbc/peerdirectory.env.example
/etc/innbc/peerdirectory.env
/etc/systemd/system/innbc-peerdirectory.service

<!-- START ALPHA CHANGE 1935 -- document mandatory peer-directory runtime configuration -->
The active environment file is mandatory:

```text
/etc/innbc/peerdirectory.env
```

The installer creates this file from the packaged example when it does not already exist and preserves it during later installations.

If the file is subsequently deleted or becomes unreadable, systemd refuses to start the peer-directory service before executing the INNBC binary. It does not silently fall back to the settings embedded in the service template.
<!-- FINISH ALPHA CHANGE 1935 -- document mandatory peer-directory runtime configuration -->

<!-- START ALPHA CHANGE 1922 -- document persistent peer directory state -->
Runtime peer-address state is retained in:

/var/lib/innbc/peer-directory/peers.json

The file contains only authenticated validator peer IDs and their last registered libp2p multiaddresses. Current authorization continues to be checked against the validator registry whenever the directory is called.
<!-- FINISH ALPHA CHANGE 1922 -- document persistent peer directory state -->

The installer does not touch:

/usr/local/bin/innbc-chain
/etc/innbc/genesis.json
/etc/innbc/node1.env
/etc/innbc/node2.env
/etc/innbc/node3.env
/etc/innbc/node4.env
/etc/innbc/node5.env
/etc/innbc/node6.env

Install:

sudo /opt/innbc-peerdirectory-service/scripts/install.sh

Start and enable after reboot:

sudo systemctl enable --now innbc-peerdirectory

Check status:

sudo systemctl status innbc-peerdirectory --no-pager

Follow logs:

sudo journalctl -u innbc-peerdirectory -f

Stop:

sudo systemctl stop innbc-peerdirectory

Disable:

sudo systemctl disable --now innbc-peerdirectory

Default local endpoint:

http://127.0.0.1:8080/api/v1/peers/exchange

For private-local validator deployments, keep the service bound to 127.0.0.1 and do not expose port 8080 publicly.

<!-- START ALPHA CHANGE 1923 -- document persistent and manually refreshed peer-directory operation -->
## Persistent peer-address cache

The peer-directory service persists authenticated validator addresses in:

```text
/var/lib/innbc/peer-directory/peers.json
```

The cache location is configured through:

```text
INNBC_PEER_DIRECTORY_CACHE_FILE=/var/lib/innbc/peer-directory/peers.json
```

The cache contains the last authenticated libp2p multiaddress registered by each validator. It does not grant validator authority. Whenever a node calls the peer directory, the service reads the current validator state and removes cached identities that are no longer registered and whitelisted.

Stopping or restarting `innbc-peerdirectory` does not erase the address list. Running validators do not need to register again merely because the service restarted.

If the cache is missing, the service creates an empty valid cache. If it is corrupted, the service logs that the cache was corrupted and regenerated, removes it, and creates a new empty cache.

## Registering a new validator address

After the validator has been registered and whitelisted on-chain:

1. Start the new validator node.
2. Run a manual peer-directory refresh against the new node’s operator gRPC endpoint.
3. Refresh every existing validator so each node downloads the updated address list.

Run this first against the newly started node:

```bash
/usr/local/bin/innbc-chain peerdirectory refresh \
  --grpc-address <new-node-grpc-address>
```

For example:

```bash
/usr/local/bin/innbc-chain peerdirectory refresh \
  --grpc-address 127.0.0.1:9633
```

This makes the running node perform a signed exchange. The peer directory authenticates the node, confirms its peer ID against the active validator registry, and stores its advertised libp2p multiaddress in `peers.json`.

Then run the same command against every other running validator:

```bash
/usr/local/bin/innbc-chain peerdirectory refresh \
  --grpc-address <existing-node-grpc-address>
```

For validators using local gRPC ports `9632`, `9633`, and `9634`, the complete operation is:

```bash
/usr/local/bin/innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9633

/usr/local/bin/innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9632
/usr/local/bin/innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9633
/usr/local/bin/innbc-chain peerdirectory refresh --grpc-address 127.0.0.1:9634
```

The first command explicitly registers the new node. The following pass ensures that every running validator receives the complete updated list. Repeating the command for the new node is harmless and ensures that it also receives the final list.

When validators run on separate hosts, execute the refresh locally on each validator host against that node’s local operator gRPC address. The operator gRPC interface should not be exposed publicly merely to perform this operation.

## Removing or changing a validator

After removing a validator, changing its whitelist status, changing its registered libp2p peer ID, or changing its advertised multiaddress, manually refresh all remaining validator nodes:

```bash
/usr/local/bin/innbc-chain peerdirectory refresh \
  --grpc-address <node-grpc-address>
```

Each exchange re-reads the current validator registry. Cached entries that are no longer registered and whitelisted are removed from `peers.json`, and each refreshed node replaces its local network allowlist.

## Restart behavior

A routine peer-directory restart requires no special validator action:

```text
service stops
→ peers.json remains on disk
→ service restarts
→ previous authenticated address map is restored
```

Manual re-registration is required only if:

- `peers.json` was deleted;
- the cache was corrupted and regenerated;
- a validator changed its libp2p identity or advertised address.
<!-- FINISH ALPHA CHANGE 1923 -- document persistent and manually refreshed peer-directory operation -->
<!-- FINISH ALPHA CHANGE 999 -- document standalone peer directory service package -->