// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

/* START ALPHA CHANGE 976 -- add block reward governance community pool test mock -- */
contract MockINNBCBlockRewardCommunityRewardPool {
    uint256 public communityRewardRateBps;

    function setCommunityRewardRateBps(uint256 value) external {
        communityRewardRateBps = value;
    }
}
/* FINISH ALPHA CHANGE 976 -- add block reward governance community pool test mock -- */
