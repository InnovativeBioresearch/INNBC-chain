// SPDX-License-Identifier: MIT
pragma solidity 0.8.19;

/* START ALPHA CHANGE 3418 -- expose epoch commitments without VSET balances or withdrawals -- */
struct Epoch {
    uint256 startBlock;
    uint256 endBlock;
    bytes32 epochRoot;
}

/**
 * @title IValidatorSet
 * @notice Stores system-committed INNBC epoch records.
 * @dev Committee eligibility and native-INNBC staking are managed separately
 * by INNBCValidatorRegistry.
 */
interface IValidatorSet {
    event NewEpoch(uint256 indexed id, uint256 indexed startBlock, uint256 indexed endBlock, bytes32 epochRoot);

    /// @notice Commits a new epoch.
    /// @dev Callable only by the system caller.
    function commitEpoch(uint256 id, Epoch calldata epoch) external;

    /// @notice Number of blocks in an epoch.
    /// @dev Each committed range must contain a multiple of this number.
    // slither-disable-next-line naming-convention
    function EPOCH_SIZE() external view returns (uint256);

    /// @notice Total number of blocks in a committed epoch.
    function totalBlocks(uint256 epochId) external view returns (uint256 length);
}
/* FINISH ALPHA CHANGE 3418 -- expose epoch commitments without VSET balances or withdrawals -- */
