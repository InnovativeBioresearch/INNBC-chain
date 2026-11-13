// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

/* START ALPHA CHANGE 977 -- add block reward governance native token test mock -- */
contract MockINNBCBlockRewardNativeToken {
    uint256 public totalSupply;
    mapping(address => uint256) public balanceOf;

    event Transfer(address indexed from, address indexed to, uint256 value);

    constructor(uint256 initialSupply) {
        totalSupply = initialSupply;
    }

    function issueEmergencyBlockReward(address account, uint256 amount) external returns (bool) {
        totalSupply += amount;
        balanceOf[account] += amount;

        emit Transfer(address(0), account, amount);

        return true;
    }
}
/* FINISH ALPHA CHANGE 977 -- add block reward governance native token test mock -- */
