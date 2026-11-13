<!-- START ALPHA CHANGE 3393 -- replace unregistered txpool RPC examples with the local operator interface -->

# Transaction-Pool Operator Interface

The current INNBC JSON-RPC dispatcher does not register the `txpool` namespace. `txpool_content`, `txpool_inspect` and `txpool_status` are not exposed JSON-RPC methods.

Inspect the pool through the local operator command:

```bash
innbc-chain txpool status --grpc-address 127.0.0.1:9632
```

The operator gRPC service provides pool status, raw-transaction admission and event subscriptions. Its status response reports the pool length; it is not the former JSON-RPC pending/queued response.

Keep operator gRPC on its supported loopback interface. Public signed transaction submission remains available through `eth_sendRawTransaction`.

<!-- FINISH ALPHA CHANGE 3393 -- replace unregistered txpool RPC examples with the local operator interface -->
