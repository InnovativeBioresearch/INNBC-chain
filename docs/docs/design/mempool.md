<!-- START ALPHA CHANGE 3388 -- describe current transaction-pool selection without age or retention guarantees -->

## What Is the Mempool?

The mempool is the node's in-memory transaction pool. It stores admitted transactions pending possible inclusion in a block.

## Mempool in INNBC

Transactions are organized by sender and nonce. Proposal selection compares eligible account-head transactions using fee-based ordering, including effective tips when a base fee is supplied. Transaction age is not a selection key.

Admission, replacement, capacity limits and account reconciliation determine which transactions remain available. Admission does not guarantee eventual inclusion or retention until confirmation.

Selecting a transaction for a proposal does not itself remove it from the shared pool. Canonical nonce reconciliation and explicit rejection or cleanup determine removal.

Transaction gossip distributes transactions between connected peers. It does not guarantee that every node has an identical pool at every moment.

See the transaction-pool implementation and the main README for the current admission, replacement and reconciliation rules.

<!-- FINISH ALPHA CHANGE 3388 -- describe current transaction-pool selection without age or retention guarantees -->
