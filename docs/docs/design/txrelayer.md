## What is a TxRelayer used for?

A TxRelayer is a component that facilitates the creation and sending of transactions. It provides a simple interface for executing a message call without creating a dedicated transaction and sending signed transactions to a blockchain.

<!-- START ALPHA CHANGE 1039 -- rename TxRelayer docs for INNBC -->
## TxRelayer in INNBC

In INNBC Chain, the TxRelayer module contains a JSON-RPC client and wallet helper for signing and submitting transactions to an EVM-compatible node.
<!-- FINISH ALPHA CHANGE 1039 -- rename TxRelayer docs for INNBC -->

<!-- START ALPHA CHANGE 3390 -- distinguish supported signed submission from legacy account-managed testing -->

## Using the TxRelayer

`NewTxRelayer` accepts options such as `WithClient`, `WithIPAddress`, `WithReceiptTimeout` and `WithNumRetries`. The default endpoint is `http://127.0.0.1:8545`.

The default receipt polling interval is 50 milliseconds, with 1000 attempts. The `receiptTimeout` field is the delay between attempts, not a 50-millisecond deadline for the complete operation.

`Call` performs an `eth_call` using the pending selector and returns the RPC result as a string.

`SendTransaction` obtains the sender's pending nonce and chain ID, fills missing fee and gas settings, signs the transaction locally and submits its encoded bytes through `eth_sendRawTransaction`.

`SendTransactionLocal` remains a testing helper for external endpoints providing account-managed transaction submission. It relies on account enumeration and `eth_sendTransaction`. INNBC does not provide that wallet-management workflow; use locally signed submission against an INNBC node.

<!-- FINISH ALPHA CHANGE 3390 -- distinguish supported signed submission from legacy account-managed testing -->
