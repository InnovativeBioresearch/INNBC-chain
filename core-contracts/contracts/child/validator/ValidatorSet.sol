// SPDX-License-Identifier: MIT
pragma solidity 0.8.19;

/* START ALPHA CHANGE 3417 -- retain ValidatorSet epoch tracking and retire VSET accounting -- */
import "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import "../../interfaces/child/validator/IValidatorSet.sol";
import "../System.sol";

/**
 * @notice Stores system-committed INNBC epoch records.
 * @dev Committee eligibility and native-INNBC staking belong to INNBCValidatorRegistry.
 */
contract ValidatorSet is IValidatorSet, Initializable, System {
    // slither-disable-next-line naming-convention
    uint256 public EPOCH_SIZE;

    uint256 public currentEpochId;

    mapping(uint256 => Epoch) public epochs;
    uint256[] public epochEndBlocks;

    constructor() {
        _disableInitializers();
    }

    /* START ALPHA CHANGE 3446 -- enforce the two-block minimum during epoch initialization -- */
    /**
     * @notice Initializes epoch tracking during genesis.
     * @dev Runtime-bytecode genesis allocations do not execute the constructor.
     * The system-caller restriction therefore also protects those allocations.
     * @param newEpochSize Epoch length in blocks; must be at least two.
     */
    function initialize(uint256 newEpochSize) public onlySystemCall initializer {
        require(newEpochSize >= 2, "INVALID_INPUT");
        EPOCH_SIZE = newEpochSize;
        epochEndBlocks.push(0);
        currentEpochId = 1;
    }
    /* FINISH ALPHA CHANGE 3446 -- enforce the two-block minimum during epoch initialization -- */

    /**
     * @inheritdoc IValidatorSet
     */
    function commitEpoch(uint256 id, Epoch calldata epoch) external onlySystemCall {
        uint256 newEpochId = currentEpochId++;
        require(id == newEpochId, "UNEXPECTED_EPOCH_ID");
        require(epoch.endBlock > epoch.startBlock, "NO_BLOCKS_COMMITTED");
        require((epoch.endBlock - epoch.startBlock + 1) % EPOCH_SIZE == 0, "EPOCH_MUST_BE_DIVISIBLE_BY_EPOCH_SIZE");
        require(epochs[newEpochId - 1].endBlock + 1 == epoch.startBlock, "INVALID_START_BLOCK");
        epochs[newEpochId] = epoch;
        epochEndBlocks.push(epoch.endBlock);
        emit NewEpoch(id, epoch.startBlock, epoch.endBlock, epoch.epochRoot);
    }

    /**
     * @inheritdoc IValidatorSet
     */
    function totalBlocks(uint256 epochId) external view returns (uint256 length) {
        uint256 endBlock = epochs[epochId].endBlock;
        length = endBlock == 0 ? 0 : endBlock - epochs[epochId].startBlock + 1;
    }
}
/* FINISH ALPHA CHANGE 3417 -- retain ValidatorSet epoch tracking and retire VSET accounting -- */
