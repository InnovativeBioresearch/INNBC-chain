<!-- START ALPHA CHANGE 3396 -- remove absent Supernet libraries and describe retained contract helpers -->

# Contract Libraries and Helpers

This directory contains retained Solidity helpers. Their presence does not imply support for the removed Polygon rootchain or Supernet deployment model.

<!-- START ALPHA CHANGE 3759 -- remove the retired Solidity access-list wrapper from the helper list -->
- `ModExp.sol`: modular arithmetic helpers.
<!-- FINISH ALPHA CHANGE 3759 -- remove the retired Solidity access-list wrapper from the helper list -->
<!-- START ALPHA CHANGE 3755 -- remove unused genesis-set and integer-conversion helpers -->
- `GenesisProxy.sol`: proxy setup for genesis-deployed system contracts.
<!-- FINISH ALPHA CHANGE 3755 -- remove unused genesis-set and integer-conversion helpers -->

<!-- START ALPHA CHANGE 3753 -- document removal of unused local meta-transaction helpers -->
The unused local `EIP712MetaTransaction.sol` and `EIP712Upgradeable.sol` helpers have been removed. They were not integrated into the current system contracts or included in the genesis deployment list.
<!-- FINISH ALPHA CHANGE 3753 -- document removal of unused local meta-transaction helpers -->

<!-- START ALPHA CHANGE 3756 -- document retirement of orphaned validator declarations and test mocks -->
The unused legacy validator queue and storage declarations, rootchain validator-set interface, and orphaned token and clone mocks have been removed from the contract suite. The current validator registry, epoch-tracking interface and contract audit fixtures remain in use.
<!-- FINISH ALPHA CHANGE 3756 -- document retirement of orphaned validator declarations and test mocks -->

Cryptographic verification contracts are in [common](../common/). Generated contract references are under [`docs/`](../../docs/).

`ChildManagerLib` and the removed rootchain stake-manager library are not current components. The main repository README describes the INNBC validator registry and governance model.

<!-- FINISH ALPHA CHANGE 3396 -- remove absent Supernet libraries and describe retained contract helpers -->
