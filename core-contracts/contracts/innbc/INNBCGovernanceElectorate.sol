/* START ALPHA CHANGE 3552 -- count current validators and departed proposal voters once -- */
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

import "./IINNBCValidatorRegistry.sol";

library INNBCGovernanceElectorate {
    function size(
        IINNBCValidatorRegistry registry,
        mapping(address => bool) storage proposalVoters,
        uint256 totalVotes
    ) internal view returns (uint256) {
        address[] memory activeValidators = registry.getActiveValidators();
        uint256 activeVoters = 0;

        for (uint256 index = 0; index < activeValidators.length; index++) {
            if (proposalVoters[activeValidators[index]]) {
                activeVoters++;
            }
        }

        // Current active validators plus voters who have since departed.
        return activeValidators.length + (totalVotes - activeVoters);
    }
}
/* FINISH ALPHA CHANGE 3552 -- count current validators and departed proposal voters once -- */