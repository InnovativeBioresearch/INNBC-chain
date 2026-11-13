/* START ALPHA CHANGE 800 -- add INNBC fee burn vault contract source -- */
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

interface IINNBCFeeBurnNativeToken {
    function burn(uint256 amount) external returns (bool);
}

contract INNBCFeeBurnVault {
    address public nativeToken;
    bool public initialized;

    event FeeBurnVaultDeposit(address indexed sender, uint256 amount);
    event FeesBurned(address indexed caller, uint256 amount);

    receive() external payable {
        emit FeeBurnVaultDeposit(msg.sender, msg.value);
    }

    function initialize(address nativeToken_) external {
        require(!initialized, "INNBC Fee Burn Vault: already initialized");
        require(nativeToken_ != address(0), "INNBC Fee Burn Vault: zero native token");

        initialized = true;
        nativeToken = nativeToken_;
    }

    function burn() external {
        uint256 amount = address(this).balance;
        require(amount > 0, "INNBC Fee Burn Vault: zero balance");
        require(IINNBCFeeBurnNativeToken(nativeToken).burn(amount), "INNBC Fee Burn Vault: burn failed");

        emit FeesBurned(msg.sender, amount);
    }
}
/* FINISH ALPHA CHANGE 800 -- add INNBC fee burn vault contract source -- */