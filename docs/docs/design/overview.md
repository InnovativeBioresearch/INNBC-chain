<!-- START ALPHA CHANGE 3384 -- remove the obsolete Supernets architecture illustration -->
This page summarizes the INNBC runtime components. The repository's main README is the primary reference for the current architecture and its documented constraints.
<!-- FINISH ALPHA CHANGE 3384 -- remove the obsolete Supernets architecture illustration -->

## Components

The following table breaks down each of these components.

| Component | Description | Link |
| --- | --- | --- |
| libp2p | Provides the networking layer for Edge and is designed for peer-to-peer network architectures. | [<ins>libp2p Overview</ins>](libp2p.md) |
<!-- START ALPHA CHANGE 800 -- remove legacy bridge component from design overview -->
<!-- FINISH ALPHA CHANGE 800 -- remove legacy bridge component from design overview -->
| Mempool | <!-- START ALPHA CHANGE 3384 -- distinguish pending transactions from consensus signature aggregation -->Stores admitted pending transactions for proposal selection and transaction gossip; implemented by the transaction pool.<!-- FINISH ALPHA CHANGE 3384 -- distinguish pending transactions from consensus signature aggregation --> | [<ins>mempool Overview</ins>](mempool.md) |
<!-- START ALPHA CHANGE 801 -- describe INNBCBFT without legacy bridge and staking wording -->
| Consensus | INNBC BFT serves as the consensus mechanism and coordinates validator block production, signatures, and validator-set transitions. | [<ins>INNBC BFT Overview</ins>](consensus/innbcbft/overview.md) |
<!-- FINISH ALPHA CHANGE 801 -- describe INNBCBFT without legacy bridge and staking wording -->
| Blockchain | Coordinates everything in the system, curates state transitions, and is responsible for state changes when a new block is added to the chain. | [<ins>Blockchain Overview</ins>](blockchain.md) |
| Runtime (EVM) | Uses the EVM as the runtime environment for executing smart contracts. | [<ins>Runtime Overview</ins>](runtime/overview.md) |
| TxPool | Represents the transaction pool, closely linked with other modules in the system. | [<ins>TxPool Overview</ins>](txpool.md) |
| JSON-RPC | Facilitates interaction between dApp developers and the blockchain, allowing developers to issue JSON-RPC requests to an Edge node and receive responses. | [<ins>JSON-RPC Overview</ins>](jsonrpc.md) |
| gRPC | Essential for operator interactions, allowing node operators to interact with the client easily and providing a seamless user experience. | [<ins>gRPC Overview</ins>](grpc.md) |
