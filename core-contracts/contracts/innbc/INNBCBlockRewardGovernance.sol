/* START ALPHA CHANGE 258 -- add INNBC block reward governance contract source -- */
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

/* START ALPHA CHANGE 3784 -- import full-precision emergency reward arithmetic -- */
import {Math} from "@openzeppelin/contracts/utils/math/Math.sol";
/* FINISH ALPHA CHANGE 3784 -- import full-precision emergency reward arithmetic -- */

import "./IINNBCValidatorRegistry.sol";
/* START ALPHA CHANGE 3555 -- share the effective electorate calculation -- */
import "./INNBCGovernanceElectorate.sol";
/* FINISH ALPHA CHANGE 3555 -- share the effective electorate calculation -- */

interface IINNBCBlockRewardNativeToken {
    function totalSupply() external view returns (uint256);
    /* START ALPHA CHANGE 271 -- use emergency block reward issuance ABI -- */
    function issueEmergencyBlockReward(address account, uint256 amount) external returns (bool);
    /* FINISH ALPHA CHANGE 271 -- use emergency block reward issuance ABI -- */
}

/* START ALPHA CHANGE 949 -- read community reward rate during burn governance finalization -- */
interface IINNBCBlockRewardCommunityRewardPool {
    function communityRewardRateBps() external view returns (uint256);
}
/* FINISH ALPHA CHANGE 949 -- read community reward rate during burn governance finalization -- */

contract INNBCBlockRewardGovernance {
    enum ProposalType {
        EmergencyReward,
        UpdateFeeBurnRate,
        /* START ALPHA CHANGE 1146 -- add max base fee governance proposal type -- */
        UpdateMaxBaseFee
        /* FINISH ALPHA CHANGE 1146 -- add max base fee governance proposal type -- */
    }

    struct Proposal {
        ProposalType proposalType;
        uint256 emergencyRewardRateBps;
        uint256 newFeeBurnRateBps;
        /* START ALPHA CHANGE 1147 -- store proposed max base fee cap value -- */
        uint256 newMaxBaseFeeWei;
        /* FINISH ALPHA CHANGE 1147 -- store proposed max base fee cap value -- */
        uint256 yesVotes;
        uint256 noVotes;
        uint256 deadlineBlock;
        bool finalized;
        bool passed;
    }

    uint256 public constant BASIS_POINTS_DENOMINATOR = 10000;
    uint256 public constant EMERGENCY_REWARD_ONE_PERCENT_BPS = 100;
    uint256 public constant EMERGENCY_REWARD_TWO_PERCENT_BPS = 200;
    uint256 public constant EMERGENCY_REWARD_THREE_PERCENT_BPS = 300;
    uint256 public constant EMERGENCY_REWARD_COOLDOWN_BLOCKS = 1296000;

    uint256 public constant FEE_BURN_ZERO_PERCENT_BPS = 0;
    uint256 public constant FEE_BURN_TEN_PERCENT_BPS = 1000;
    uint256 public constant FEE_BURN_TWENTY_FIVE_PERCENT_BPS = 2500;
    uint256 public constant FEE_BURN_FIFTY_PERCENT_BPS = 5000;
    /* START ALPHA CHANGE 813 -- add high fee burn governance presets -- */
    uint256 public constant FEE_BURN_SEVENTY_PERCENT_BPS = 7000;
    uint256 public constant FEE_BURN_NINETY_PERCENT_BPS = 9000;
    uint256 public constant FEE_BURN_ONE_HUNDRED_PERCENT_BPS = 10000;
    /* FINISH ALPHA CHANGE 813 -- add high fee burn governance presets -- */

    /* START ALPHA CHANGE 802 -- store fee burn rate in explicit runtime-readable slot -- */
    bytes32 private constant FEE_BURN_RATE_BPS_SLOT = keccak256("innbc.blockRewardGovernance.feeBurnRateBps");
    /* START ALPHA CHANGE 1148 -- store optional max base fee in explicit runtime-readable slot -- */
    bytes32 private constant MAX_BASE_FEE_WEI_SLOT = keccak256("innbc.blockRewardGovernance.maxBaseFeeWei");
    /* FINISH ALPHA CHANGE 1148 -- store optional max base fee in explicit runtime-readable slot -- */
    /* FINISH ALPHA CHANGE 802 -- store fee burn rate in explicit runtime-readable slot -- */

    address public validatorRegistry;
    address public nativeToken;
    /* START ALPHA CHANGE 950 -- store community reward pool for fee split safety checks -- */
    address public communityRewardPool;
    /* FINISH ALPHA CHANGE 950 -- store community reward pool for fee split safety checks -- */
    bool public initialized;

    /* START ALPHA CHANGE 803 -- remove layout-dependent fee burn rate storage variable -- */
    /* Fee burn rate is stored in FEE_BURN_RATE_BPS_SLOT for direct deterministic runtime reads. */
    /* FINISH ALPHA CHANGE 803 -- remove layout-dependent fee burn rate storage variable -- */
    uint256 public lastEmergencyRewardBlock;
    bool public emergencyRewardProposalActive;
    uint256 public nextProposalID;

    mapping(uint256 => Proposal) public proposals;
    mapping(uint256 => mapping(address => bool)) public hasVoted;
    /* START ALPHA CHANGE 975 -- track pending block-reward proposal IDs without scanning historical proposals -- */
    uint256[] private activeProposalIDs;
    mapping(uint256 => uint256) private activeProposalIndexPlusOne;
    /* FINISH ALPHA CHANGE 975 -- track pending block-reward proposal IDs without scanning historical proposals -- */

    event ProposalCreated(uint256 indexed proposalID, ProposalType proposalType, uint256 deadlineBlock);
    event VoteCast(uint256 indexed proposalID, address indexed voter, bool support);
    event ProposalFinalized(uint256 indexed proposalID, bool passed);
    event EmergencyRewardDistributed(
        uint256 indexed proposalID,
        uint256 totalReward,
        uint256 perValidatorReward,
        uint256 emergencyRewardRateBps
    );
    event FeeBurnRateUpdated(uint256 feeBurnRateBps);
    /* START ALPHA CHANGE 1149 -- emit max base fee governance updates -- */
    event MaxBaseFeeUpdated(uint256 maxBaseFeeWei);
    /* FINISH ALPHA CHANGE 1149 -- emit max base fee governance updates -- */

    modifier onlyActiveValidator() {
        require(_isActiveValidator(msg.sender), "INNBC Block Reward: sender not active validator");
        _;
    }

    function initialize(address validatorRegistry_, address nativeToken_, address communityRewardPool_) external {
        require(!initialized, "INNBC Block Reward: already initialized");
        require(validatorRegistry_ != address(0), "INNBC Block Reward: zero validator registry");
        require(nativeToken_ != address(0), "INNBC Block Reward: zero native token");
        /* START ALPHA CHANGE 951 -- require community reward pool during block reward governance initialization -- */
        require(communityRewardPool_ != address(0), "INNBC Block Reward: zero community reward pool");
        /* FINISH ALPHA CHANGE 951 -- require community reward pool during block reward governance initialization -- */

        initialized = true;
        validatorRegistry = validatorRegistry_;
        nativeToken = nativeToken_;
        /* START ALPHA CHANGE 951 -- store community reward pool during block reward governance initialization -- */
        communityRewardPool = communityRewardPool_;
        /* FINISH ALPHA CHANGE 951 -- store community reward pool during block reward governance initialization -- */
    }

    /* START ALPHA CHANGE 975 -- expose pending block-reward proposal IDs -- */
    function getActiveProposalIDs() external view returns (uint256[] memory) {
        return activeProposalIDs;
    }
    /* FINISH ALPHA CHANGE 975 -- expose pending block-reward proposal IDs -- */

    function createOnePercentEmergencyRewardProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createEmergencyRewardProposal(EMERGENCY_REWARD_ONE_PERCENT_BPS);
    }

    function createTwoPercentEmergencyRewardProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createEmergencyRewardProposal(EMERGENCY_REWARD_TWO_PERCENT_BPS);
    }

    function createThreePercentEmergencyRewardProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createEmergencyRewardProposal(EMERGENCY_REWARD_THREE_PERCENT_BPS);
    }

    function createUseZeroFeeBurnRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateFeeBurnRateProposal(FEE_BURN_ZERO_PERCENT_BPS);
    }

    function createUseTenPercentFeeBurnRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateFeeBurnRateProposal(FEE_BURN_TEN_PERCENT_BPS);
    }

    function createUseTwentyFivePercentFeeBurnRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateFeeBurnRateProposal(FEE_BURN_TWENTY_FIVE_PERCENT_BPS);
    }

    function createUseFiftyPercentFeeBurnRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateFeeBurnRateProposal(FEE_BURN_FIFTY_PERCENT_BPS);
    }

    /* START ALPHA CHANGE 814 -- add high fee burn governance proposal helpers -- */
    function createUseSeventyPercentFeeBurnRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateFeeBurnRateProposal(FEE_BURN_SEVENTY_PERCENT_BPS);
    }

    function createUseNinetyPercentFeeBurnRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateFeeBurnRateProposal(FEE_BURN_NINETY_PERCENT_BPS);
    }

    function createUseOneHundredPercentFeeBurnRateProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateFeeBurnRateProposal(FEE_BURN_ONE_HUNDRED_PERCENT_BPS);
    }
    /* FINISH ALPHA CHANGE 814 -- add high fee burn governance proposal helpers -- */

    /* START ALPHA CHANGE 1150 -- add governance proposal for optional max base fee cap -- */
    function createUpdateMaxBaseFeeProposal(uint256 newMaxBaseFeeWei) external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateMaxBaseFeeProposal(newMaxBaseFeeWei);
    }
    /* FINISH ALPHA CHANGE 1150 -- add governance proposal for optional max base fee cap -- */

    function vote(uint256 proposalID, bool support) external onlyActiveValidator {
        Proposal storage proposal = proposals[proposalID];

        require(proposal.deadlineBlock != 0, "INNBC Block Reward: proposal not found");
        require(block.number <= proposal.deadlineBlock, "INNBC Block Reward: proposal expired");
        require(!proposal.finalized, "INNBC Block Reward: proposal finalized");
        require(!hasVoted[proposalID][msg.sender], "INNBC Block Reward: already voted");

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

        require(proposal.deadlineBlock != 0, "INNBC Block Reward: proposal not found");
        require(!proposal.finalized, "INNBC Block Reward: proposal finalized");

        /* START ALPHA CHANGE 3555 -- finalize against current validators plus recorded departed voters -- */
        uint256 totalVotes = proposal.yesVotes + proposal.noVotes;
        uint256 electorateSize = INNBCGovernanceElectorate.size(
            IINNBCValidatorRegistry(validatorRegistry), hasVoted[proposalID], totalVotes
        );
        uint256 requiredYesVotes = _requiredYesVotes(proposal.proposalType, electorateSize);
        bool deadlinePassed = block.number > proposal.deadlineBlock;
        bool outcomeSettled = _isProposalOutcomeSettled(
            proposal, totalVotes, requiredYesVotes, electorateSize
        );

        require(deadlinePassed || outcomeSettled, "INNBC Block Reward: proposal still active");

        bool passed = electorateSize > 0 && _proposalPassed(proposal, requiredYesVotes);
        /* FINISH ALPHA CHANGE 3555 -- finalize against current validators plus recorded departed voters -- */

        proposal.finalized = true;
        proposal.passed = passed;
        /* START ALPHA CHANGE 975 -- remove finalized block-reward proposal from pending proposal index -- */
        _removeActiveProposal(proposalID);
        /* FINISH ALPHA CHANGE 975 -- remove finalized block-reward proposal from pending proposal index -- */

        if (passed) {
            if (proposal.proposalType == ProposalType.EmergencyReward) {
                _distributeEmergencyReward(proposalID, proposal.emergencyRewardRateBps);
            } else if (proposal.proposalType == ProposalType.UpdateFeeBurnRate) {
                /* START ALPHA CHANGE 952 -- reject burn updates that exceed total fee split with community rewards -- */
                uint256 communityRewardRateBps =
                    IINNBCBlockRewardCommunityRewardPool(communityRewardPool).communityRewardRateBps();
                require(
                    proposal.newFeeBurnRateBps + communityRewardRateBps <= BASIS_POINTS_DENOMINATOR,
                    "INNBC Block Reward: fee split too high"
                );
                /* FINISH ALPHA CHANGE 952 -- reject burn updates that exceed total fee split with community rewards -- */

                /* START ALPHA CHANGE 804 -- update explicit fee burn rate storage slot on governance approval -- */
                _setFeeBurnRateBps(proposal.newFeeBurnRateBps);
                /* FINISH ALPHA CHANGE 804 -- update explicit fee burn rate storage slot on governance approval -- */
                emit FeeBurnRateUpdated(proposal.newFeeBurnRateBps);
            } else if (proposal.proposalType == ProposalType.UpdateMaxBaseFee) {
                /* START ALPHA CHANGE 1151 -- apply approved max base fee cap proposal -- */
                _setMaxBaseFeeWei(proposal.newMaxBaseFeeWei);
                emit MaxBaseFeeUpdated(proposal.newMaxBaseFeeWei);
                /* FINISH ALPHA CHANGE 1151 -- apply approved max base fee cap proposal -- */
            }
        }

        if (proposal.proposalType == ProposalType.EmergencyReward) {
            emergencyRewardProposalActive = false;
        }

        emit ProposalFinalized(proposalID, passed);
    }

    function _createEmergencyRewardProposal(uint256 emergencyRewardRateBps) private returns (uint256 proposalID) {
        require(!emergencyRewardProposalActive, "INNBC Block Reward: emergency reward proposal active");
        require(
            lastEmergencyRewardBlock == 0 || block.number >= lastEmergencyRewardBlock + EMERGENCY_REWARD_COOLDOWN_BLOCKS,
            "INNBC Block Reward: emergency reward cooldown active"
        );

        emergencyRewardProposalActive = true;

        proposalID = _createProposal(
            ProposalType.EmergencyReward,
            emergencyRewardRateBps,
            0,
            0,
            IINNBCValidatorRegistry(validatorRegistry).proposalDurationBlocks()
        );
    }

    function _createUpdateFeeBurnRateProposal(uint256 newFeeBurnRateBps) private returns (uint256 proposalID) {
        /* START ALPHA CHANGE 805 -- compare fee burn proposal against explicit storage slot value -- */
        require(newFeeBurnRateBps != _feeBurnRateBps(), "INNBC Block Reward: fee burn rate unchanged");
        /* FINISH ALPHA CHANGE 805 -- compare fee burn proposal against explicit storage slot value -- */

        proposalID = _createProposal(
            ProposalType.UpdateFeeBurnRate,
            0,
            newFeeBurnRateBps,
            0,
            IINNBCValidatorRegistry(validatorRegistry).proposalDurationBlocks()
        );
    }

    /* START ALPHA CHANGE 1152 -- create optional max base fee governance proposal -- */
    function _createUpdateMaxBaseFeeProposal(uint256 newMaxBaseFeeWei) private returns (uint256 proposalID) {
        /* START ALPHA CHANGE 3750 -- reject max base fee proposals outside the runtime uint64 range -- */
        require(
            newMaxBaseFeeWei <= type(uint64).max,
            "INNBC Block Reward: max base fee exceeds uint64"
        );
        /* FINISH ALPHA CHANGE 3750 -- reject max base fee proposals outside the runtime uint64 range -- */

        require(newMaxBaseFeeWei != _maxBaseFeeWei(), "INNBC Block Reward: max base fee unchanged");

        proposalID = _createProposal(
            ProposalType.UpdateMaxBaseFee,
            0,
            0,
            newMaxBaseFeeWei,
            IINNBCValidatorRegistry(validatorRegistry).proposalDurationBlocks()
        );
    }
    /* FINISH ALPHA CHANGE 1152 -- create optional max base fee governance proposal -- */

    function _createProposal(
        ProposalType proposalType,
        uint256 emergencyRewardRateBps,
        uint256 newFeeBurnRateBps,
        /* START ALPHA CHANGE 1153 -- carry proposed max base fee cap through proposal storage -- */
        uint256 newMaxBaseFeeWei,
        /* FINISH ALPHA CHANGE 1153 -- carry proposed max base fee cap through proposal storage -- */
        uint256 durationBlocks
    ) private returns (uint256 proposalID) {
        proposalID = nextProposalID++;

        proposals[proposalID] = Proposal({
            proposalType: proposalType,
            emergencyRewardRateBps: emergencyRewardRateBps,
            newFeeBurnRateBps: newFeeBurnRateBps,
            /* START ALPHA CHANGE 1154 -- persist proposed max base fee cap value -- */
            newMaxBaseFeeWei: newMaxBaseFeeWei,
            /* FINISH ALPHA CHANGE 1154 -- persist proposed max base fee cap value -- */
            yesVotes: 0,
            noVotes: 0,
            deadlineBlock: block.number + durationBlocks,
            finalized: false,
            passed: false
        });

        /* START ALPHA CHANGE 975 -- add created block-reward proposal to pending proposal index -- */
        _trackActiveProposal(proposalID);
        /* FINISH ALPHA CHANGE 975 -- add created block-reward proposal to pending proposal index -- */

        emit ProposalCreated(proposalID, proposalType, block.number + durationBlocks);
    }

    /* START ALPHA CHANGE 629 -- pass block reward proposals by required yes votes with explicit unanimous emergency rule -- */
    function _proposalPassed(
        Proposal storage proposal,
        uint256 requiredYesVotes
    ) private view returns (bool) {
        if (proposal.proposalType == ProposalType.EmergencyReward) {
            return proposal.yesVotes == requiredYesVotes;
        }

        return proposal.yesVotes >= requiredYesVotes;
    }
    /* FINISH ALPHA CHANGE 629 -- pass block reward proposals by required yes votes with explicit unanimous emergency rule -- */

    /* START ALPHA CHANGE 3555 -- settle using the same effective electorate as approval -- */
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

        if (proposal.proposalType == ProposalType.EmergencyReward && proposal.noVotes > 0) {
            return true;
        }

        if (proposal.yesVotes >= requiredYesVotes) {
            return true;
        }

        return proposal.yesVotes + remainingVotes < requiredYesVotes;
    }
    /* FINISH ALPHA CHANGE 3555 -- settle using the same effective electorate as approval -- */

    function _distributeEmergencyReward(uint256 proposalID, uint256 emergencyRewardRateBps) private {
        address[] memory activeValidators = IINNBCValidatorRegistry(validatorRegistry).getActiveValidators();

        require(activeValidators.length > 0, "INNBC Block Reward: no active validators");
        require(
            lastEmergencyRewardBlock == 0 || block.number >= lastEmergencyRewardBlock + EMERGENCY_REWARD_COOLDOWN_BLOCKS,
            "INNBC Block Reward: emergency reward cooldown active"
        );

        uint256 totalSupply = IINNBCBlockRewardNativeToken(nativeToken).totalSupply();

        /* START ALPHA CHANGE 3785 -- calculate emergency percentages without intermediate uint256 overflow -- */
        uint256 totalReward = Math.mulDiv(
            totalSupply,
            emergencyRewardRateBps,
            BASIS_POINTS_DENOMINATOR
        );
        /* FINISH ALPHA CHANGE 3785 -- calculate emergency percentages without intermediate uint256 overflow -- */
        /* START ALPHA CHANGE 261 -- complete emergency reward distribution and block reward governance helpers -- */
        uint256 perValidatorReward = totalReward / activeValidators.length;

        require(perValidatorReward > 0, "INNBC Block Reward: emergency reward too small");

        lastEmergencyRewardBlock = block.number;

        for (uint256 index = 0; index < activeValidators.length; index++) {
            address operator = activeValidators[index];

            (
                ,
                address rewardAddress,
                ,
                ,
                ,
                ,
            ) = IINNBCValidatorRegistry(validatorRegistry).validators(operator);

            /* START ALPHA CHANGE 272 -- issue emergency block rewards through native token ABI -- */
            require(
                IINNBCBlockRewardNativeToken(nativeToken).issueEmergencyBlockReward(rewardAddress, perValidatorReward),
                "INNBC Block Reward: emergency block reward issue failed"
            );
            /* FINISH ALPHA CHANGE 272 -- issue emergency block rewards through native token ABI -- */
        }

        emit EmergencyRewardDistributed(
            proposalID,
            perValidatorReward * activeValidators.length,
            perValidatorReward,
            emergencyRewardRateBps
        );
    }

    /* START ALPHA CHANGE 1155 -- expose optional max base fee getter backed by explicit runtime-readable slot -- */
    function maxBaseFeeWei() external view returns (uint256) {
        return _maxBaseFeeWei();
    }

    function _maxBaseFeeWei() private view returns (uint256 value) {
        bytes32 slot = MAX_BASE_FEE_WEI_SLOT;

        assembly {
            value := sload(slot)
        }
    }

    function _setMaxBaseFeeWei(uint256 value) private {
        /* START ALPHA CHANGE 3751 -- enforce the runtime uint64 range before storing a max base fee -- */
        require(
            value <= type(uint64).max,
            "INNBC Block Reward: max base fee exceeds uint64"
        );
        /* FINISH ALPHA CHANGE 3751 -- enforce the runtime uint64 range before storing a max base fee -- */

        bytes32 slot = MAX_BASE_FEE_WEI_SLOT;

        assembly {
            sstore(slot, value)
        }
    }
    /* FINISH ALPHA CHANGE 1155 -- expose optional max base fee getter backed by explicit runtime-readable slot -- */

    /* START ALPHA CHANGE 806 -- expose fee burn rate getter backed by explicit runtime-readable slot -- */
    function feeBurnRateBps() external view returns (uint256) {
        return _feeBurnRateBps();
    }

    function _feeBurnRateBps() private view returns (uint256 value) {
        bytes32 slot = FEE_BURN_RATE_BPS_SLOT;

        assembly {
            value := sload(slot)
        }
    }

    function _setFeeBurnRateBps(uint256 value) private {
        require(value <= BASIS_POINTS_DENOMINATOR, "INNBC Block Reward: fee burn rate too high");

        bytes32 slot = FEE_BURN_RATE_BPS_SLOT;

        assembly {
            sstore(slot, value)
        }
    }
    /* FINISH ALPHA CHANGE 806 -- expose fee burn rate getter backed by explicit runtime-readable slot -- */

    /* START ALPHA CHANGE 975 -- maintain pending block-reward proposal index -- */
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
    /* FINISH ALPHA CHANGE 975 -- maintain pending block-reward proposal index -- */

    function _isActiveValidator(address operator) private view returns (bool) {
        (
            ,
            ,
            ,
            ,
            uint256 stake,
            bool registered,
            bool whitelisted
        ) = IINNBCValidatorRegistry(validatorRegistry).validators(operator);

        return registered &&
            whitelisted &&
            stake >= IINNBCValidatorRegistry(validatorRegistry).minimumStake();
    }

    /* START ALPHA CHANGE 3555 -- derive reward approval thresholds from the effective electorate -- */
    function _requiredYesVotes(
        ProposalType proposalType,
        uint256 electorateSize
    ) private pure returns (uint256) {
        if (proposalType == ProposalType.EmergencyReward) {
            return electorateSize;
        }

        return (electorateSize / 2) + 1;
    }
    /* FINISH ALPHA CHANGE 3555 -- derive reward approval thresholds from the effective electorate -- */
}
/* FINISH ALPHA CHANGE 261 -- complete emergency reward distribution and block reward governance helpers -- */
/* FINISH ALPHA CHANGE 258 -- add INNBC block reward governance contract source -- */
       