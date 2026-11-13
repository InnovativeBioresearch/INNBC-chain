// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

/* START ALPHA CHANGE 978 -- add block reward governance validator registry test mock -- */
contract MockINNBCValidatorRegistry {
    struct Validator {
        address validatorOperator;
        address rewardAddress;
        bytes blsKey;
        string libp2pPeerID;
        uint256 stake;
        bool registered;
        bool whitelisted;
    }

    uint256 public minimumStake;
    uint256 public proposalDurationBlocks = 100;
    address public nativeToken;

    address[] private activeValidators;
    mapping(address => Validator) private validatorData;

    function activeValidatorCount() external view returns (uint256) {
        return activeValidators.length;
    }

    function getActiveValidators() external view returns (address[] memory) {
        return activeValidators;
    }

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
        )
    {
        Validator storage validator = validatorData[operator];

        return (
            validator.validatorOperator,
            validator.rewardAddress,
            validator.blsKey,
            validator.libp2pPeerID,
            validator.stake,
            validator.registered,
            validator.whitelisted
        );
    }

    function addActiveValidator(address operator, address rewardAddress, uint256 stake) external {
        validatorData[operator] = Validator({
            validatorOperator: operator,
            rewardAddress: rewardAddress,
            blsKey: "",
            libp2pPeerID: "",
            stake: stake,
            registered: true,
            whitelisted: true
        });
        activeValidators.push(operator);
    }

    function whitelistValidator(address) external {}

    function removeValidator(address) external {}

    function updateMinimumStake(uint256 newMinimumStake) external {
        minimumStake = newMinimumStake;
    }

    function updateProposalDurationPreset(uint8) external {}
}
/* FINISH ALPHA CHANGE 978 -- add block reward governance validator registry test mock -- */