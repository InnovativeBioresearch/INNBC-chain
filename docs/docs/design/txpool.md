<!-- START ALPHA CHANGE 3389 -- replace the retired enqueue workflow and incorrect positive-value requirement -->

## TxPool in INNBC

The transaction pool validates and stores pending transactions by sender and nonce.

`addTx` performs admission and updates the sender's queues and transaction ownership. Promotion makes contiguous eligible nonces available for proposal selection; the current implementation does not use the former `handleEnqueueRequest` workflow.

Validation covers transaction type and size, signature and chain identity, nonnegative value, fees, nonce, affordability and applicable admission rules. Zero-value transactions are permitted when their other requirements are satisfied.

Replacement of an existing sender nonce must satisfy the implemented replacement-price rules. State-read failures are returned rather than interpreted as zero balances or nonces.

`Prepare` starts proposal selection from current promoted account heads. `Peek` selects a transaction, and proposal-local progress offers subsequent eligible nonces without prematurely deleting unconfirmed shared-pool transactions.

Canonical events and account reconciliation update nonces and remove stale entries. Capacity accounting and cleanup also govern retention; admission is not a guarantee of confirmation.

`addGossipTx` decodes network transactions and routes them through admission. The main README documents the current safeguards and accepted limitations.

<!-- FINISH ALPHA CHANGE 3389 -- replace the retired enqueue workflow and incorrect positive-value requirement -->
