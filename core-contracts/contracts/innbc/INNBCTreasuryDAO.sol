/* START ALPHA CHANGE 237 -- add INNBC treasury DAO contract source -- */
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

/* START ALPHA CHANGE 974 -- add safe ERC20 transfer support for treasury-held external tokens -- */
import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
/* FINISH ALPHA CHANGE 974 -- add safe ERC20 transfer support for treasury-held external tokens -- */

/* START ALPHA CHANGE 250 -- use shared INNBC validator registry interface -- */
import "./IINNBCValidatorRegistry.sol";
/* START ALPHA CHANGE 3554 -- share the effective electorate calculation -- */
import "./INNBCGovernanceElectorate.sol";
/* FINISH ALPHA CHANGE 3554 -- share the effective electorate calculation -- */
/* FINISH ALPHA CHANGE 250 -- use shared INNBC validator registry interface -- */

interface IINNBCTreasuryNativeToken {
    function burn(uint256 amount) external returns (bool);
}

contract INNBCTreasuryDAO {
    /* START ALPHA CHANGE 974 -- add safe ERC20 transfer support for treasury-held external tokens -- */
    using SafeERC20 for IERC20;
    /* FINISH ALPHA CHANGE 974 -- add safe ERC20 transfer support for treasury-held external tokens -- */

    enum ProposalType {
        Transfer,
        Burn,
        Lock,
        Unlock,
        /* START ALPHA CHANGE 974 -- add ERC20 transfer proposal type without changing existing enum values -- */
        ERC20Transfer
        /* FINISH ALPHA CHANGE 974 -- add ERC20 transfer proposal type without changing existing enum values -- */
    }

    struct Proposal {
        ProposalType proposalType;
        address target;
        /* START ALPHA CHANGE 974 -- store ERC20 token address for external-token transfer proposals -- */
        address token;
        /* FINISH ALPHA CHANGE 974 -- store ERC20 token address for external-token transfer proposals -- */
        uint256 amount;
        uint256 yesVotes;
        uint256 noVotes;
        uint256 deadlineBlock;
        bool finalized;
        bool passed;
    }

    address public validatorRegistry;
    address public nativeToken;
    bool public initialized;

    uint256 public lockedTreasuryBalance;
    uint256 public nextProposalID;

    mapping(uint256 => Proposal) public proposals;
    mapping(uint256 => mapping(address => bool)) public hasVoted;
    /* START ALPHA CHANGE 975 -- track pending treasury proposal IDs without scanning historical proposals -- */
    uint256[] private activeProposalIDs;
    mapping(uint256 => uint256) private activeProposalIndexPlusOne;
    /* FINISH ALPHA CHANGE 975 -- track pending treasury proposal IDs without scanning historical proposals -- */

    event TreasuryDeposit(address indexed sender, uint256 amount);
    event ProposalCreated(
        uint256 indexed proposalID,
        ProposalType proposalType,
        address indexed target,
        uint256 amount,
        uint256 deadlineBlock
    );
    event VoteCast(uint256 indexed proposalID, address indexed voter, bool support);
    event ProposalFinalized(uint256 indexed proposalID, bool passed);
    event TreasuryTransfer(address indexed target, uint256 amount);
    /* START ALPHA CHANGE 974 -- emit ERC20 transfers executed by treasury governance -- */
    event TreasuryERC20Transfer(address indexed token, address indexed target, uint256 amount);
    /* FINISH ALPHA CHANGE 974 -- emit ERC20 transfers executed by treasury governance -- */
    event TreasuryBurn(uint256 amount);
    event TreasuryLocked(uint256 amount, uint256 lockedTreasuryBalance);
    event TreasuryUnlocked(uint256 amount, uint256 lockedTreasuryBalance);

    modifier onlyActiveValidator() {
        require(_isActiveValidator(msg.sender), "INNBC Treasury: sender not active validator");
        _;
    }

    receive() external payable {
        emit TreasuryDeposit(msg.sender, msg.value);
    }

    function initialize(address validatorRegistry_, address nativeToken_) external {
        require(!initialized, "INNBC Treasury: already initialized");
        require(validatorRegistry_ != address(0), "INNBC Treasury: zero validator registry");
        require(nativeToken_ != address(0), "INNBC Treasury: zero native token");

        initialized = true;
        validatorRegistry = validatorRegistry_;
        nativeToken = nativeToken_;
    }

    function availableTreasuryBalance() public view returns (uint256) {
        return address(this).balance - lockedTreasuryBalance;
    }

    /* START ALPHA CHANGE 975 -- expose pending treasury proposal IDs -- */
    function getActiveProposalIDs() external view returns (uint256[] memory) {
        return activeProposalIDs;
    }
    /* FINISH ALPHA CHANGE 975 -- expose pending treasury proposal IDs -- */

    function createTransferProposal(address target, uint256 amount) external onlyActiveValidator returns (uint256 proposalID) {
        require(target != address(0), "INNBC Treasury: zero transfer target");
        require(amount > 0, "INNBC Treasury: zero amount");
        require(amount <= availableTreasuryBalance(), "INNBC Treasury: amount exceeds available balance");

        proposalID = _createProposal(ProposalType.Transfer, target, address(0), amount);
    }

    /* START ALPHA CHANGE 974 -- allow governance to transfer treasury-held external ERC20 tokens -- */
    function createERC20TransferProposal(
        address token,
        address target,
        uint256 amount
    ) external onlyActiveValidator returns (uint256 proposalID) {
        require(token != address(0), "INNBC Treasury: zero token");
        require(token != nativeToken, "INNBC Treasury: use native transfer for INNBC");
        require(target != address(0), "INNBC Treasury: zero transfer target");
        require(amount > 0, "INNBC Treasury: zero amount");

        proposalID = _createProposal(ProposalType.ERC20Transfer, target, token, amount);
    }
    /* FINISH ALPHA CHANGE 974 -- allow governance to transfer treasury-held external ERC20 tokens -- */

    function createBurnProposal(uint256 amount) external onlyActiveValidator returns (uint256 proposalID) {
        require(amount > 0, "INNBC Treasury: zero amount");
        require(amount <= availableTreasuryBalance(), "INNBC Treasury: amount exceeds available balance");

        proposalID = _createProposal(ProposalType.Burn, address(0), address(0), amount);
    }

    function createLockProposal(uint256 amount) external onlyActiveValidator returns (uint256 proposalID) {
        require(amount > 0, "INNBC Treasury: zero amount");
        require(amount <= availableTreasuryBalance(), "INNBC Treasury: amount exceeds available balance");

        proposalID = _createProposal(ProposalType.Lock, address(0), address(0), amount);
    }

    function createUnlockProposal(uint256 amount) external onlyActiveValidator returns (uint256 proposalID) {
        require(amount > 0, "INNBC Treasury: zero amount");
        require(amount <= lockedTreasuryBalance, "INNBC Treasury: amount exceeds locked balance");

        proposalID = _createProposal(ProposalType.Unlock, address(0), address(0), amount);
    }

    function vote(uint256 proposalID, bool support) external onlyActiveValidator {
        Proposal storage proposal = proposals[proposalID];

        require(proposal.deadlineBlock != 0, "INNBC Treasury: proposal not found");
        require(block.number <= proposal.deadlineBlock, "INNBC Treasury: proposal expired");
        require(!proposal.finalized, "INNBC Treasury: proposal finalized");
        require(!hasVoted[proposalID][msg.sender], "INNBC Treasury: already voted");

        hasVoted[proposalID][msg.sender] = true;

        if (support) {
            proposal.yesVotes++;
        } else {
            proposal.noVotes++;
        }

        emit VoteCast(proposalID, msg.sender, support);
    }

    function finalizeProposal(uint256 proposalID) external {
        Proposal storage proposal = proposals[proposalID];

        require(proposal.deadlineBlock != 0, "INNBC Treasury: proposal not found");
        require(!proposal.finalized, "INNBC Treasury: proposal finalized");

        /* START ALPHA CHANGE 3554 -- finalize against current validators plus recorded departed voters -- */
        uint256 totalVotes = proposal.yesVotes + proposal.noVotes;
        uint256 electorateSize = INNBCGovernanceElectorate.size(
            IINNBCValidatorRegistry(validatorRegistry), hasVoted[proposalID], totalVotes
        );
        uint256 requiredYesVotes = _requiredYesVotes(electorateSize);
        bool deadlinePassed = block.number > proposal.deadlineBlock;
        bool outcomeSettled = _isProposalOutcomeSettled(
            proposal, totalVotes, requiredYesVotes, electorateSize
        );

        require(deadlinePassed || outcomeSettled, "INNBC Treasury: proposal still active");

        bool passed = electorateSize > 0 && proposal.yesVotes >= requiredYesVotes;
        /* FINISH ALPHA CHANGE 3554 -- finalize against current validators plus recorded departed voters -- */

        proposal.finalized = true;
        proposal.passed = passed;
        /* START ALPHA CHANGE 975 -- remove finalized treasury proposal from pending proposal index -- */
        _removeActiveProposal(proposalID);
        /* FINISH ALPHA CHANGE 975 -- remove finalized treasury proposal from pending proposal index -- */

        if (passed) {
            if (proposal.proposalType == ProposalType.Transfer) {
                _transferTreasury(proposal.target, proposal.amount);
            } else if (proposal.proposalType == ProposalType.Burn) {
                _burnTreasury(proposal.amount);
            } else if (proposal.proposalType == ProposalType.Lock) {
                _lockTreasury(proposal.amount);
            } else if (proposal.proposalType == ProposalType.Unlock) {
                _unlockTreasury(proposal.amount);
            /* START ALPHA CHANGE 974 -- execute treasury-held external ERC20 transfer proposals -- */
            } else if (proposal.proposalType == ProposalType.ERC20Transfer) {
                _transferERC20Treasury(proposal.token, proposal.target, proposal.amount);
            /* FINISH ALPHA CHANGE 974 -- execute treasury-held external ERC20 transfer proposals -- */
            }
        }

        emit ProposalFinalized(proposalID, passed);
    }

    function _createProposal(
        ProposalType proposalType,
        address target,
        address token,
        uint256 amount
    ) private returns (uint256 proposalID) {
        proposalID = nextProposalID++;

        /* START ALPHA CHANGE 251 -- read proposal duration through shared validator registry interface -- */
        uint256 deadlineBlock =
            block.number + IINNBCValidatorRegistry(validatorRegistry).proposalDurationBlocks();
        /* FINISH ALPHA CHANGE 251 -- read proposal duration through shared validator registry interface -- */

        proposals[proposalID] = Proposal({
            proposalType: proposalType,
            target: target,
            token: token,
            amount: amount,
            yesVotes: 0,
            noVotes: 0,
            deadlineBlock: deadlineBlock,
            finalized: false,
            passed: false
        });

        /* START ALPHA CHANGE 975 -- add created treasury proposal to pending proposal index -- */
        _trackActiveProposal(proposalID);
        /* FINISH ALPHA CHANGE 975 -- add created treasury proposal to pending proposal index -- */

        emit ProposalCreated(proposalID, proposalType, target, amount, deadlineBlock);
    }

    function _transferTreasury(address target, uint256 amount) private {
        require(amount <= availableTreasuryBalance(), "INNBC Treasury: amount exceeds available balance");

        (bool success, ) = target.call{value: amount}("");
        require(success, "INNBC Treasury: transfer failed");

        emit TreasuryTransfer(target, amount);
    }

    /* START ALPHA CHANGE 974 -- safely transfer external ERC20 balances held by the treasury -- */
    function _transferERC20Treasury(
        address token,
        address target,
        uint256 amount
    ) private {
        IERC20(token).safeTransfer(target, amount);

        emit TreasuryERC20Transfer(token, target, amount);
    }
    /* FINISH ALPHA CHANGE 974 -- safely transfer external ERC20 balances held by the treasury -- */

    function _burnTreasury(uint256 amount) private {
        require(amount <= availableTreasuryBalance(), "INNBC Treasury: amount exceeds available balance");
        require(IINNBCTreasuryNativeToken(nativeToken).burn(amount), "INNBC Treasury: burn failed");

        emit TreasuryBurn(amount);
    }

    function _lockTreasury(uint256 amount) private {
        require(amount <= availableTreasuryBalance(), "INNBC Treasury: amount exceeds available balance");

        lockedTreasuryBalance += amount;

        emit TreasuryLocked(amount, lockedTreasuryBalance);
    }

    function _unlockTreasury(uint256 amount) private {
        require(amount <= lockedTreasuryBalance, "INNBC Treasury: amount exceeds locked balance");

        lockedTreasuryBalance -= amount;

        emit TreasuryUnlocked(amount, lockedTreasuryBalance);
    }

    function _isActiveValidator(address operator) private view returns (bool) {
        (
            ,
            ,
            ,
            ,
            uint256 stake,
            bool registered,
            bool whitelisted
        /* START ALPHA CHANGE 252 -- read validator records through shared validator registry interface -- */
        ) = IINNBCValidatorRegistry(validatorRegistry).validators(operator);

        return registered &&
            whitelisted &&
            stake >= IINNBCValidatorRegistry(validatorRegistry).minimumStake();
        /* FINISH ALPHA CHANGE 252 -- read validator records through shared validator registry interface -- */
    }

    /* START ALPHA CHANGE 3554 -- derive majority from the effective electorate -- */
    function _requiredYesVotes(uint256 electorateSize) private pure returns (uint256) {
        return (electorateSize / 2) + 1;
    }
    /* FINISH ALPHA CHANGE 3554 -- derive majority from the effective electorate -- */

    /* START ALPHA CHANGE 975 -- maintain pending treasury proposal index -- */
    function _trackActiveProposal(uint256 proposalID) private {
        activeProposalIDs.push(proposalID);
        activeProposalIndexPlusOne[proposalID] = activeProposalIDs.length;
    }

    function _removeActiveProposal(uint256 proposalID) private {
        uint256 indexPlusOne = activeProposalIndexPlusOne[proposalID];
        if (indexPlusOne == 0) {
            return;
        }

        uint256 index = indexPlusOne - 1;
        uint256 lastIndex = activeProposalIDs.length - 1;

        if (index != lastIndex) {
            uint256 lastProposalID = activeProposalIDs[lastIndex];
            activeProposalIDs[index] = lastProposalID;
            activeProposalIndexPlusOne[lastProposalID] = indexPlusOne;
        }

        activeProposalIDs.pop();
        delete activeProposalIndexPlusOne[proposalID];
    }
    /* FINISH ALPHA CHANGE 975 -- maintain pending treasury proposal index -- */

    /* START ALPHA CHANGE 3554 -- settle using the same effective electorate as approval -- */
    function _isProposalOutcomeSettled(
        Proposal storage proposal,
        uint256 totalVotes,
        uint256 requiredYesVotes,
        uint256 electorateSize
    ) private view returns (bool) {
        if (electorateSize == 0) {
            return true;
        }

        uint256 remainingVotes = electorateSize - totalVotes;

        if (proposal.yesVotes >= requiredYesVotes) {
            return true;
        }

        return proposal.yesVotes + remainingVotes < requiredYesVotes;
    }
    /* FINISH ALPHA CHANGE 3554 -- settle using the same effective electorate as approval -- */
}
/* FINISH ALPHA CHANGE 237 -- add INNBC treasury DAO contract source -- */