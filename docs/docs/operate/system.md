<!-- START ALPHA CHANGE 1028 -- update system requirements for current INNBC deployment -->

## Minimum Hardware Configuration

The minimum storage requirements will change over time as the network grows. Use more than the minimum requirements for a robust full node.

| Component | Minimum Requirement | Recommended |
| --------- | ------------------- | ----------- |
| Processor | 4-core CPU | 8-core CPU |
| Memory | 8 GB RAM | 16 GB RAM |
| Storage | 200 GB SSD | 1 TB SSD |
| Network | Stable broadband connection | Dedicated server with reliable low-latency connectivity |

## Prerequisites

Before starting the deployment flow, make sure you are comfortable with Linux command-line operations and systemd-managed services.

| Prerequisite | Description |
| --- | --- |
| Go | Required on the build machine for `make nodeclient` and `make localtestclient`. |
| jq | Useful for inspecting genesis and JSON-RPC output during validation. |
| cast | Useful for local and server JSON-RPC checks. |
| systemd | Used on Linux servers to manage validator node services and the peer-directory service. |
| Internet connection | Required for dependency downloads, deployment, updates, and validator connectivity. |
| Network security | Make sure required RPC, peer, and service ports are intentionally exposed or kept local according to the deployment profile. |

The current INNBC production path uses the `innbc-chain` binary from the nodeclient bundle. It does not require the removed legacy Polygon Edge Docker cluster flow.

<!-- FINISH ALPHA CHANGE 1028 -- update system requirements for current INNBC deployment -->