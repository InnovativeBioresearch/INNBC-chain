<!-- START ALPHA CHANGE 3779 -- retire the unused Solidity Merkle library and system deployment -->

# Common Cryptographic Contracts

This directory contains the Solidity BLS implementation.

The directory name reflects shared implementation utilities, not an active rootchain/child-chain deployment model.

## `BLS.sol`

The validator registry uses the BLS contract to verify operator-bound proofs of possession. The implementation uses BN256 cryptography, standard EVM precompiles and the modular arithmetic helpers in `../lib/ModExp.sol`.

The unused standalone `BN256G2.sol` contract and its `IBN256G2` interface have been removed. The standard BN256 addition, multiplication and pairing precompiles at `0x06`, `0x07` and `0x08` remain available. Active BLS verification does not depend on the removed standalone contract.

## Retired Merkle Deployment

The unused Solidity `Merkle.sol` library and its genesis allocations at `0x103` and `0x1031` have been removed. System-upgrade governance no longer includes the former Merkle proxy.

This retirement does not remove the separate Go `merkle-tree` package.

Generated contract references are under [`docs/`](../../docs/).

<!-- FINISH ALPHA CHANGE 3779 -- retire the unused Solidity Merkle library and system deployment -->
