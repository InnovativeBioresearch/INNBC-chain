/* START ALPHA CHANGE 249 -- add shared INNBC validator registry read interface -- */
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

interface IINNBCValidatorRegistry {
    function activeValidatorCount() external view returns (uint256);
    function minimumStake() external view returns (uint256);
    function proposalDurationBlocks() external view returns (uint256);
    /* START ALPHA CHANGE 290 -- expose registry native staking token address -- */
    function nativeToken() external view returns (address);
    /* FINISH ALPHA CHANGE 290 -- expose registry native staking token address -- */

    /* START ALPHA CHANGE 257 -- expose active validator list for block reward governance -- */
    function getActiveValidators() external view returns (address[] memory);
    /* FINISH ALPHA CHANGE 257 -- expose active validator list for block reward governance -- */

    function validators(address operator)
        external
        view
        returns (
            address validatorOperator,
            address rewardAddress,
            bytes memory blsKey,
            string memory libp2pPeerID,
            uint256 stake,
            bool registered,
            bool whitelisted
        );

    /* START ALPHA CHANGE 255 -- add controller mutation hooks for validator governance contract -- */
    function whitelistValidator(address operator) external;
    function removeValidator(address operator) external;
    function updateMinimumStake(uint256 newMinimumStake) external;
    function updateProposalDurationPreset(uint8 newProposalDurationPreset) external;
    /* FINISH ALPHA CHANGE 255 -- add controller mutation hooks for validator governance contract -- */
}
/* FINISH ALPHA CHANGE 249 -- add shared INNBC validator registry read interface -- */