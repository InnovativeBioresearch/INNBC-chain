/* START ALPHA CHANGE 982 -- add isolated transparent proxy admin executor -- */
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

contract INNBCProxyAdminExecutor {
    address private constant VALIDATOR_SET_PROXY = address(0x101);
    /* START ALPHA CHANGE 3773 -- retire the Merkle target from the proxy upgrade executor -- */
    address private constant BLS_PROXY = address(0x102);
    address private constant INNBC_VALIDATOR_REGISTRY_PROXY = address(0x107);
    /* FINISH ALPHA CHANGE 3773 -- retire the Merkle target from the proxy upgrade executor -- */
    address private constant INNBC_TREASURY_DAO_PROXY = address(0x108);
    address private constant INNBC_VALIDATOR_GOVERNANCE_PROXY = address(0x109);
    address private constant INNBC_BLOCK_REWARD_GOVERNANCE_PROXY = address(0x10A);
    address private constant INNBC_FEE_BURN_VAULT_PROXY = address(0x10B);
    address private constant INNBC_COMMUNITY_REWARD_POOL_PROXY = address(0x10D);
    address private constant INNBC_NATIVE_TOKEN_PROXY = address(0x1010);

    address public governance;
    bool public initialized;

    event GovernanceInitialized(address indexed governance);
    event UpgradeExecuted(address indexed proxy, address indexed implementation);

    modifier onlyGovernance() {
        require(initialized, "INNBC Proxy Admin Executor: not initialized");
        require(msg.sender == governance, "INNBC Proxy Admin Executor: sender not governance");
        _;
    }

    function initialize(address governance_) external {
        require(!initialized, "INNBC Proxy Admin Executor: already initialized");
        require(governance_ != address(0), "INNBC Proxy Admin Executor: zero governance");

        initialized = true;
        governance = governance_;

        emit GovernanceInitialized(governance_);
    }

    function executeUpgrade(
        address proxy,
        address implementation,
        bytes calldata data
    ) external onlyGovernance {
        require(_isAllowedProxy(proxy), "INNBC Proxy Admin Executor: proxy not allowed");
        require(implementation != address(0), "INNBC Proxy Admin Executor: zero implementation");
        require(implementation.code.length > 0, "INNBC Proxy Admin Executor: implementation has no code");

        bool success;
        bytes memory returnData;

        if (data.length == 0) {
            (success, returnData) = proxy.call(abi.encodeWithSignature("upgradeTo(address)", implementation));
        } else {
            (success, returnData) = proxy.call(
                abi.encodeWithSignature("upgradeToAndCall(address,bytes)", implementation, data)
            );
        }

        if (!success) {
            assembly {
                revert(add(returnData, 32), mload(returnData))
            }
        }

        emit UpgradeExecuted(proxy, implementation);
    }

    /* START ALPHA CHANGE 3773 -- retire the Merkle target from the proxy upgrade executor -- */
    function _isAllowedProxy(address proxy) private pure returns (bool) {
        return proxy == VALIDATOR_SET_PROXY ||
            proxy == BLS_PROXY ||
            proxy == INNBC_VALIDATOR_REGISTRY_PROXY ||
            proxy == INNBC_TREASURY_DAO_PROXY ||
            proxy == INNBC_VALIDATOR_GOVERNANCE_PROXY ||
            proxy == INNBC_BLOCK_REWARD_GOVERNANCE_PROXY ||
            proxy == INNBC_FEE_BURN_VAULT_PROXY ||
            proxy == INNBC_COMMUNITY_REWARD_POOL_PROXY ||
            proxy == INNBC_NATIVE_TOKEN_PROXY;
    }
    /* FINISH ALPHA CHANGE 3773 -- retire the Merkle target from the proxy upgrade executor -- */
}
/* FINISH ALPHA CHANGE 982 -- add isolated transparent proxy admin executor -- */