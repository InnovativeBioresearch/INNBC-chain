<!-- START ALPHA CHANGE 1041 -- rename blockchain design docs for INNBC -->
INNBC Chain is built on a common EVM-compatible blockchain design that manages sequential blocks, transaction metadata, receipts, and state database updates.
<!-- FINISH ALPHA CHANGE 1041 -- rename blockchain design docs for INNBC -->

The core blockchain implementation offers various functionalities, such as:

- Adding new blocks to the chain.
- Retrieving blocks using their hash or number.
- <!-- START ALPHA CHANGE 3385 -- describe canonical append instead of retired difficulty fork choice -->Appending validated blocks that directly extend the current canonical head; INNBCBFT does not select competing heads by total difficulty.<!-- FINISH ALPHA CHANGE 3385 -- describe canonical append instead of retired difficulty fork choice -->
- Verifying block headers and gas limits.
- Caching block headers and receipts to improve retrieval speed.
- Updating the chain's average gas price.

To provide these functionalities, the implementation employs several sub-components, including:

- A consensus component responsible for validating new blocks and incorporating them into the chain.
- A database component that persistently stores blockchain data.
- <!-- START ALPHA CHANGE 3385 -- describe notifications without implying active reorganization support -->An event component that notifies other components when new canonical blocks are added.<!-- FINISH ALPHA CHANGE 3385 -- describe notifications without implying active reorganization support -->
- A transaction signer component that verifies transaction signatures and identifies the sender address.
- A gas price calculator component that computes the chain's average gas price.
