/* START ALPHA CHANGE 947 -- add INNBC community reward pool contract source -- */
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

/* START ALPHA CHANGE 3866 -- import full-precision community reward arithmetic -- */
import {Math} from "@openzeppelin/contracts/utils/math/Math.sol";
/* FINISH ALPHA CHANGE 3866 -- import full-precision community reward arithmetic -- */

/* START ALPHA CHANGE 3556 -- use shared registry and electorate interfaces -- */
import "./IINNBCValidatorRegistry.sol";
import "./INNBCGovernanceElectorate.sol";
/* FINISH ALPHA CHANGE 3556 -- use shared registry and electorate interfaces -- */

interface IINNBCCommunityRewardBlockRewardGovernance {
    function feeBurnRateBps() external view returns (uint256);
}

contract INNBCCommunityRewardPool {
    enum ProposalType {
        UpdateCommunityRewardRate
    }

    struct Proposal {
        ProposalType proposalType;
        uint256 newCommunityRewardRateBps;
        uint256 yesVotes;
        uint256 noVotes;
        uint256 deadlineBlock;
        bool finalized;
        bool passed;
    }

    uint256 public constant BASIS_POINTS_DENOMINATOR = 10000;
    uint256 public constant REWARD_PRECISION = 1e18;

    uint256 public constant COMMUNITY_REWARD_ZERO_PERCENT_BPS = 0;
    uint256 public constant COMMUNITY_REWARD_FIVE_PERCENT_BPS = 500;
    uint256 public constant COMMUNITY_REWARD_TEN_PERCENT_BPS = 1000;
    uint256 public constant COMMUNITY_REWARD_TWENTY_FIVE_PERCENT_BPS = 2500;
    uint256 public constant COMMUNITY_REWARD_FIFTY_PERCENT_BPS = 5000;

    bytes32 private constant COMMUNITY_REWARD_RATE_BPS_SLOT =
        keccak256("innbc.communityRewardPool.communityRewardRateBps");

    address public validatorRegistry;
    address public blockRewardGovernance;
    bool public initialized;

    uint256 public totalStaked;
    uint256 public rewardPerTokenStored;
    uint256 public accountedRewardBalance;
    /* START ALPHA CHANGE 948 -- remove unallocated reward bucket so pre-stake rewards remain claimable -- */
    /* FINISH ALPHA CHANGE 948 -- remove unallocated reward bucket so pre-stake rewards remain claimable -- */
    uint256 public nextProposalID;

    mapping(address => uint256) public stakedBalance;
    mapping(address => uint256) public userRewardPerTokenPaid;
    mapping(address => uint256) public pendingRewards;
    mapping(uint256 => Proposal) public proposals;
    mapping(uint256 => mapping(address => bool)) public hasVoted;
    /* START ALPHA CHANGE 975 -- track pending community-reward proposal IDs without scanning historical proposals -- */
    uint256[] private activeProposalIDs;
    mapping(uint256 => uint256) private activeProposalIndexPlusOne;
    /* FINISH ALPHA CHANGE 975 -- track pending community-reward proposal IDs without scanning historical proposals -- */

    bool private locked;

    event CommunityRewardDeposit(address indexed sender, uint256 amount);
    event Staked(address indexed staker, uint256 amount);
    event Unstaked(address indexed staker, uint256 amount);
    event RewardClaimed(address indexed staker, uint256 amount);
    event ProposalCreated(uint256 indexed proposalID, ProposalType proposalType, uint256 deadlineBlock);
    event VoteCast(uint256 indexed proposalID, address indexed voter, bool support);
    event ProposalFinalized(uint256 indexed proposalID, bool passed);
    event CommunityRewardRateUpdated(uint256 communityRewardRateBps);

    modifier onlyActiveValidator() {
        require(_isActiveValidator(msg.sender), "INNBC Community Reward: sender not active validator");
        _;
    }

    modifier nonReentrant() {
        require(!locked, "INNBC Community Reward: reentrant call");
        locked = true;
        _;
        locked = false;
    }

    receive() external payable {
        emit CommunityRewardDeposit(msg.sender, msg.value);
    }

    function initialize(address validatorRegistry_, address blockRewardGovernance_) external {
        require(!initialized, "INNBC Community Reward: already initialized");
        require(validatorRegistry_ != address(0), "INNBC Community Reward: zero validator registry");
        require(blockRewardGovernance_ != address(0), "INNBC Community Reward: zero block reward governance");

        initialized = true;
        validatorRegistry = validatorRegistry_;
        blockRewardGovernance = blockRewardGovernance_;
    }

    function stake() external payable nonReentrant {
        require(msg.value > 0, "INNBC Community Reward: zero stake");

        _updateUser(msg.sender, msg.value);

        stakedBalance[msg.sender] += msg.value;
        totalStaked += msg.value;

        emit Staked(msg.sender, msg.value);
    }

    function unstake(uint256 amount) external nonReentrant {
        require(amount > 0, "INNBC Community Reward: zero unstake");
        require(stakedBalance[msg.sender] >= amount, "INNBC Community Reward: unstake exceeds stake");

        _updateUser(msg.sender, 0);

        stakedBalance[msg.sender] -= amount;
        totalStaked -= amount;

        _safeNativeTransfer(msg.sender, amount);

        emit Unstaked(msg.sender, amount);
    }

    function claim() external nonReentrant {
        _updateUser(msg.sender, 0);

        uint256 reward = pendingRewards[msg.sender];
        require(reward > 0, "INNBC Community Reward: zero reward");

        pendingRewards[msg.sender] = 0;
        accountedRewardBalance -= reward;

        _safeNativeTransfer(msg.sender, reward);

        emit RewardClaimed(msg.sender, reward);
    }

    /* START ALPHA CHANGE 3867 -- calculate projected community rewards with full-precision products -- */
    function earned(address account) external view returns (uint256) {
        uint256 currentRewardPerToken = rewardPerTokenStored;
        uint256 rewardBalance = _rewardBalance(0);

        if (totalStaked > 0 && rewardBalance > accountedRewardBalance) {
            uint256 newRewards = rewardBalance - accountedRewardBalance;
            currentRewardPerToken += Math.mulDiv(
                newRewards,
                REWARD_PRECISION,
                totalStaked
            );
        }

        return pendingRewards[account] +
            Math.mulDiv(
                stakedBalance[account],
                currentRewardPerToken - userRewardPerTokenPaid[account],
                REWARD_PRECISION
            );
    }
    /* FINISH ALPHA CHANGE 3867 -- calculate projected community rewards with full-precision products -- */

    function communityRewardRateBps() external view returns (uint256) {
        return _communityRewardRateBps();
    }

    /* START ALPHA CHANGE 975 -- expose pending community-reward proposal IDs -- */
    function getActiveProposalIDs() external view returns (uint256[] memory) {
        return activeProposalIDs;
    }
    /* FINISH ALPHA CHANGE 975 -- expose pending community-reward proposal IDs -- */

    function createUseZeroCommunityRewardRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateCommunityRewardRateProposal(COMMUNITY_REWARD_ZERO_PERCENT_BPS);
    }

    function createUseFivePercentCommunityRewardRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateCommunityRewardRateProposal(COMMUNITY_REWARD_FIVE_PERCENT_BPS);
    }

    function createUseTenPercentCommunityRewardRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateCommunityRewardRateProposal(COMMUNITY_REWARD_TEN_PERCENT_BPS);
    }

    function createUseTwentyFivePercentCommunityRewardRateProposal()
        external
        onlyActiveValidator
        returns (uint256 proposalID)
    {
        proposalID = _createUpdateCommunityRewardRateProposal(COMMUNITY_REWARD_TWENTY_FIVE_PERCENT_BPS);
    }

    function createUseFiftyPercentCommunityRewardRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateCommunityRewardRateProposal(COMMUNITY_REWARD_FIFTY_PERCENT_BPS);
    }

    function vote(uint256 proposalID, bool support) external onlyActiveValidator {
        Proposal storage proposal = proposals[proposalID];

        require(proposal.deadlineBlock != 0, "INNBC Community Reward: proposal not found");
        require(block.number <= proposal.deadlineBlock, "INNBC Community Reward: proposal expired");
        require(!proposal.finalized, "INNBC Community Reward: proposal finalized");
        require(!hasVoted[proposalID][msg.sender], "INNBC Community Reward: already voted");

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

        require(proposal.deadlineBlock != 0, "INNBC Community Reward: proposal not found");
        require(!proposal.finalized, "INNBC Community Reward: proposal finalized");

        /* START ALPHA CHANGE 3556 -- finalize against current validators plus recorded departed voters -- */
        uint256 totalVotes = proposal.yesVotes + proposal.noVotes;
        uint256 electorateSize = INNBCGovernanceElectorate.size(
            IINNBCValidatorRegistry(validatorRegistry), hasVoted[proposalID], totalVotes
        );
        uint256 requiredYesVotes = _requiredYesVotes(electorateSize);
        bool deadlinePassed = block.number > proposal.deadlineBlock;
        bool outcomeSettled = _isProposalOutcomeSettled(
            totalVotes, proposal.yesVotes, requiredYesVotes, electorateSize
        );

        require(deadlinePassed || outcomeSettled, "INNBC Community Reward: proposal still active");

        bool passed = electorateSize > 0 && proposal.yesVotes >= requiredYesVotes;
        /* FINISH ALPHA CHANGE 3556 -- finalize against current validators plus recorded departed voters -- */

        proposal.finalized = true;
        proposal.passed = passed;
        /* START ALPHA CHANGE 975 -- remove finalized community-reward proposal from pending proposal index -- */
        _removeActiveProposal(proposalID);
        /* FINISH ALPHA CHANGE 975 -- remove finalized community-reward proposal from pending proposal index -- */

        if (passed) {
            uint256 feeBurnRateBps =
                IINNBCCommunityRewardBlockRewardGovernance(blockRewardGovernance).feeBurnRateBps();
            require(
                feeBurnRateBps + proposal.newCommunityRewardRateBps <= BASIS_POINTS_DENOMINATOR,
                "INNBC Community Reward: fee split too high"
            );

            _setCommunityRewardRateBps(proposal.newCommunityRewardRateBps);
            emit CommunityRewardRateUpdated(proposal.newCommunityRewardRateBps);
        }

        emit ProposalFinalized(proposalID, passed);
    }

    function _createUpdateCommunityRewardRateProposal(
        uint256 newCommunityRewardRateBps
    ) private returns (uint256 proposalID) {
        require(
            newCommunityRewardRateBps != _communityRewardRateBps(),
            "INNBC Community Reward: community reward rate unchanged"
        );

        proposalID = nextProposalID++;

        proposals[proposalID] = Proposal({
            proposalType: ProposalType.UpdateCommunityRewardRate,
            newCommunityRewardRateBps: newCommunityRewardRateBps,
            yesVotes: 0,
            noVotes: 0,
            /* START ALPHA CHANGE 3556 -- read proposal duration through the shared registry interface -- */
            deadlineBlock: block.number + IINNBCValidatorRegistry(validatorRegistry).proposalDurationBlocks(),
            /* FINISH ALPHA CHANGE 3556 -- read proposal duration through the shared registry interface -- */
            finalized: false,
            passed: false
        });

        /* START ALPHA CHANGE 975 -- add created community-reward proposal to pending proposal index -- */
        _trackActiveProposal(proposalID);
        /* FINISH ALPHA CHANGE 975 -- add created community-reward proposal to pending proposal index -- */

        emit ProposalCreated(
            proposalID,
            ProposalType.UpdateCommunityRewardRate,
            proposals[proposalID].deadlineBlock
        );
    }

    /* START ALPHA CHANGE 3868 -- record accrued user rewards with full-precision multiplication -- */
    function _updateUser(address account, uint256 pendingStakeValue) private {
        _syncRewards(pendingStakeValue);

        pendingRewards[account] += Math.mulDiv(
            stakedBalance[account],
            rewardPerTokenStored - userRewardPerTokenPaid[account],
            REWARD_PRECISION
        );
        userRewardPerTokenPaid[account] = rewardPerTokenStored;
    }
    /* FINISH ALPHA CHANGE 3868 -- record accrued user rewards with full-precision multiplication -- */

    /* START ALPHA CHANGE 3869 -- synchronize the community reward index with full-precision multiplication -- */
    function _syncRewards(uint256 pendingStakeValue) private {
        /* START ALPHA CHANGE 948 -- leave no-staker rewards claimable by the first active stakers -- */
        if (totalStaked == 0) {
            return;
        }
        /* FINISH ALPHA CHANGE 948 -- leave no-staker rewards claimable by the first active stakers -- */

        uint256 rewardBalance = _rewardBalance(pendingStakeValue);

        if (rewardBalance <= accountedRewardBalance) {
            return;
        }

        uint256 newRewards = rewardBalance - accountedRewardBalance;
        accountedRewardBalance = rewardBalance;

        rewardPerTokenStored += Math.mulDiv(
            newRewards,
            REWARD_PRECISION,
            totalStaked
        );
    }
    /* FINISH ALPHA CHANGE 3869 -- synchronize the community reward index with full-precision multiplication -- */

    function _rewardBalance(uint256 pendingStakeValue) private view returns (uint256) {
        uint256 effectiveBalance = address(this).balance - pendingStakeValue;

        /* START ALPHA CHANGE 948 -- remove unallocated reward exclusion from reward balance -- */
        if (effectiveBalance <= totalStaked) {
            return 0;
        }

        return effectiveBalance - totalStaked;
        /* FINISH ALPHA CHANGE 948 -- remove unallocated reward exclusion from reward balance -- */
    }

    function _communityRewardRateBps() private view returns (uint256 value) {
        bytes32 slot = COMMUNITY_REWARD_RATE_BPS_SLOT;

        assembly {
            value := sload(slot)
        }
    }

    function _setCommunityRewardRateBps(uint256 value) private {
        require(value <= BASIS_POINTS_DENOMINATOR, "INNBC Community Reward: community reward rate too high");

        bytes32 slot = COMMUNITY_REWARD_RATE_BPS_SLOT;

        assembly {
            sstore(slot, value)
        }
    }

    /* START ALPHA CHANGE 975 -- maintain pending community-reward proposal index -- */
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
    /* FINISH ALPHA CHANGE 975 -- maintain pending community-reward proposal index -- */

    /* START ALPHA CHANGE 3556 -- settle using the same effective electorate as approval -- */
    function _isProposalOutcomeSettled(
        uint256 totalVotes,
        uint256 yesVotes,
        uint256 requiredYesVotes,
        uint256 electorateSize
    ) private pure returns (bool) {
        if (electorateSize == 0) {
            return true;
        }

        uint256 remainingVotes = electorateSize - totalVotes;

        if (yesVotes >= requiredYesVotes) {
            return true;
        }

        return yesVotes + remainingVotes < requiredYesVotes;
    }
    /* FINISH ALPHA CHANGE 3556 -- settle using the same effective electorate as approval -- */

    /* START ALPHA CHANGE 3556 -- derive majority from the effective electorate -- */
    function _requiredYesVotes(uint256 electorateSize) private pure returns (uint256) {
        return (electorateSize / 2) + 1;
    }
    /* FINISH ALPHA CHANGE 3556 -- derive majority from the effective electorate -- */

    /* START ALPHA CHANGE 3556 -- preserve eligibility checks through the shared registry interface -- */
    function _isActiveValidator(address operator) private view returns (bool) {
        (
            ,
            ,
            ,
            ,
            uint256 validatorStake,
            bool registered,
            bool whitelisted
        ) = IINNBCValidatorRegistry(validatorRegistry).validators(operator);

        return registered &&
            whitelisted &&
            validatorStake >= IINNBCValidatorRegistry(validatorRegistry).minimumStake();
    }
    /* FINISH ALPHA CHANGE 3556 -- preserve eligibility checks through the shared registry interface -- */

    function _safeNativeTransfer(address to, uint256 amount) private {
        (bool success, ) = to.call{value: amount}("");

        require(success, "INNBC Community Reward: native transfer failed");
    }
}
/* FINISH ALPHA CHANGE 947 -- add INNBC community reward pool contract source -- */
