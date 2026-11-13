/* START ALPHA CHANGE 2398 -- add controlled contract audit fixtures -- */
/* SPDX-License-Identifier: Apache-2.0 */
pragma solidity 0.8.19;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";

/*
    Test-only token. This models ERC20 balances and controlled failures.
    It does not model INNBC native-balance precompile behavior.
*/
contract INNBCAuditToken is ERC20 {
    bool public failTransfers;
    bool public failBurn;
    address public failMintRecipient;

    constructor() ERC20("Audit Token", "AUDIT") {}

    function mint(address account, uint256 amount) external {
        _mint(account, amount);
    }

    function setFailTransfers(bool value) external {
        failTransfers = value;
    }

    function setFailBurn(bool value) external {
        failBurn = value;
    }

    function setFailMintRecipient(address account) external {
        failMintRecipient = account;
    }

    function transfer(address to, uint256 amount) public override returns (bool) {
        if (failTransfers) {
            return false;
        }

        return super.transfer(to, amount);
    }

    function transferFrom(
        address from,
        address to,
        uint256 amount
    ) public override returns (bool) {
        if (failTransfers) {
            return false;
        }

        return super.transferFrom(from, to, amount);
    }

    function burn(uint256 amount) external returns (bool) {
        if (failBurn) {
            return false;
        }

        _burn(msg.sender, amount);
        return true;
    }

    function issueEmergencyBlockReward(
        address account,
        uint256 amount
    ) external returns (bool) {
        if (account == failMintRecipient) {
            return false;
        }

        _mint(account, amount);
        return true;
    }
}

/*
    Controlled BLS response for registry state-machine tests.
    This is not a cryptographic verifier.
*/
contract INNBCAuditBLS {
    bool public valid;
    bool public callSuccess;

    function configure(bool valid_, bool callSuccess_) external {
        valid = valid_;
        callSuccess = callSuccess_;
    }

    function hashToPoint(
        bytes32,
        bytes calldata
    ) external pure returns (uint256[2] memory point) {
        point[0] = 1;
        point[1] = 2;
    }

    function verifySingle(
        uint256[2] calldata,
        uint256[4] calldata,
        uint256[2] calldata
    ) external view returns (bool, bool) {
        return (valid, callSuccess);
    }
}

interface IINNBCAuditPool {
    function stake() external payable;
    function unstake(uint256 amount) external;
    function claim() external;
}

contract INNBCAuditReceiver {
    IINNBCAuditPool public pool;
    bool public rejectPayment;
    bool public attemptReentry;
    bool public reentryAttempted;
    bool public reentrySucceeded;

    function configure(bool reject_, bool reenter_) external {
        rejectPayment = reject_;
        attemptReentry = reenter_;
    }

    function stakeIn(address pool_) external payable {
        pool = IINNBCAuditPool(pool_);
        pool.stake{value: msg.value}();
    }

    function claimFromPool() external {
        pool.claim();
    }

    function unstakeFromPool(uint256 amount) external {
        pool.unstake(amount);
    }

    receive() external payable {
        require(!rejectPayment, "AUDIT_PAYMENT_REJECTED");

        if (attemptReentry) {
            reentryAttempted = true;
            (bool success, ) = address(pool).call(
                abi.encodeWithSignature("claim()")
            );
            reentrySucceeded = success;
        }
    }
}

contract INNBCAuditUpgradeV1 {
    uint256 public value;

    function setValue(uint256 value_) external {
        value = value_;
    }

    function version() external pure virtual returns (uint256) {
        return 1;
    }
}

contract INNBCAuditUpgradeV2 is INNBCAuditUpgradeV1 {
    function version() external pure override returns (uint256) {
        return 2;
    }

    function initializeUpgrade(uint256 value_, bool reject_) external {
        value = value_;
        require(!reject_, "AUDIT_UPGRADE_REJECTED");
    }
}

/* FINISH ALPHA CHANGE 2398 -- add controlled contract audit fixtures -- */