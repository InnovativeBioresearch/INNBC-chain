// SPDX-License-Identifier: MIT
pragma solidity 0.8.19;

import "../interfaces/Errors.sol";

contract System {
    // pre-compiled contracts
    // slither-disable too-many-digits
    address public constant NATIVE_TRANSFER_PRECOMPILE = 0x0000000000000000000000000000000000002020;
    /* START ALPHA CHANGE 2916 -- remove the retired aggregate BLS address getter -- */
    /* FINISH ALPHA CHANGE 2916 -- remove the retired aggregate BLS address getter -- */
    address public constant ALLOWLIST_PRECOMPILE = 0x0200000000000000000000000000000000000004;
    address public constant BLOCKLIST_PRECOMPILE = 0x0300000000000000000000000000000000000004;

    // internal addrs
    address public constant SYSTEM = 0xffffFFFfFFffffffffffffffFfFFFfffFFFfFFfE;

    // pre-compiled gas consumption
    uint256 public constant NATIVE_TRANSFER_PRECOMPILE_GAS = 21000;
    /* START ALPHA CHANGE 2917 -- remove the retired aggregate BLS gas getter -- */
    /* FINISH ALPHA CHANGE 2917 -- remove the retired aggregate BLS gas getter -- */
    uint256 public constant READ_ADDRESSLIST_GAS = 5000;

    // genesis contracts
    address public constant NATIVE_TOKEN_CONTRACT = 0x0000000000000000000000000000000000001010;

    modifier onlySystemCall() {
        if (msg.sender != SYSTEM) revert Unauthorized("SYSTEMCALL");
        _;
    }

    // slither-disable-next-line unused-state,naming-convention
    uint256[50] private __gap;
}
