/* START ALPHA CHANGE 256 -- add INNBC validator governance contract source -- */
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

import "./IINNBCValidatorRegistry.sol";
/* START ALPHA CHANGE 3553 -- share the effective electorate calculation -- */
import "./INNBCGovernanceElectorate.sol";
/* FINISH ALPHA CHANGE 3553 -- share the effective electorate calculation -- */

contract INNBCValidatorGovernance {
    enum ProposalType {
        Whitelist,
        Remove,
        EmergencyRemove,
        UpdateMinimumStake,
        UpdateProposalDuration
    }

    enum ProposalDurationPreset {
        None,
        Short,
        Standard,
        Long
    }

    struct Proposal {
        ProposalType proposalType;
        address target;
        uint256 newMinimumStake;
        ProposalDurationPreset newProposalDurationPreset;
        uint256 yesVotes;
        uint256 noVotes;
        uint256 deadlineBlock;
        bool finalized;
        bool passed;
    }

    uint256 public constant SHORT_PROPOSAL_DURATION_BLOCKS = 3600;
    uint256 public constant STANDARD_PROPOSAL_DURATION_BLOCKS = 43200;
    /* START ALPHA CHANGE 734 -- reduce long validator governance proposal duration preset to forty-eight hours -- */
    uint256 public constant LONG_PROPOSAL_DURATION_BLOCKS = 86400;
    /* FINISH ALPHA CHANGE 734 -- reduce long validator governance proposal duration preset to forty-eight hours -- */
    /* START ALPHA CHANGE 632 -- reduce emergency validator removal vote window to one minute -- */
    uint256 public constant EMERGENCY_REMOVE_PROPOSAL_DURATION_BLOCKS = 30;
    /* FINISH ALPHA CHANGE 632 -- reduce emergency validator removal vote window to one minute -- */

    address public validatorRegistry;
    bool public initialized;

    ProposalDurationPreset public proposalDurationPreset;
    uint256 public proposalDurationBlocks;
    uint256 public nextProposalID;

    mapping(uint256 => Proposal) public proposals;
    mapping(uint256 => mapping(address => bool)) public hasVoted;
    /* START ALPHA CHANGE 975 -- track pending validator-governance proposal IDs without scanning historical proposals -- */
    uint256[] private activeProposalIDs;
    mapping(uint256 => uint256) private activeProposalIndexPlusOne;
    /* FINISH ALPHA CHANGE 975 -- track pending validator-governance proposal IDs without scanning historical proposals -- */

    event ProposalCreated(uint256 indexed proposalID, ProposalType proposalType, address indexed target, uint256 deadlineBlock);
    event VoteCast(uint256 indexed proposalID, address indexed voter, bool support);
    event ProposalFinalized(uint256 indexed proposalID, bool passed);
    event ProposalDurationUpdated(ProposalDurationPreset proposalDurationPreset, uint256 proposalDurationBlocks);

    modifier onlyActiveValidator() {
        require(_isActiveValidator(msg.sender), "INNBC Validator Governance: sender not active validator");
        _;
    }

    function initialize(address validatorRegistry_, ProposalDurationPreset proposalDurationPreset_) external {
        require(!initialized, "INNBC Validator Governance: already initialized");
        require(validatorRegistry_ != address(0), "INNBC Validator Governance: zero validator registry");
        require(proposalDurationPreset_ != ProposalDurationPreset.None, "INNBC Validator Governance: empty duration preset");

        initialized = true;
        validatorRegistry = validatorRegistry_;
        proposalDurationPreset = proposalDurationPreset_;
        proposalDurationBlocks = _proposalDurationBlocks(proposalDurationPreset_);
    }

    /* START ALPHA CHANGE 975 -- expose pending validator-governance proposal IDs -- */
    function getActiveProposalIDs() external view returns (uint256[] memory) {
        return activeProposalIDs;
    }
    /* FINISH ALPHA CHANGE 975 -- expose pending validator-governance proposal IDs -- */

    function createWhitelistProposal(address target) external onlyActiveValidator returns (uint256 proposalID) {
        (
            ,
            ,
            ,
            ,
            uint256 stake,
            bool registered,
            bool whitelisted
        ) = IINNBCValidatorRegistry(validatorRegistry).validators(target);

        require(registered, "INNBC Validator Governance: target not registered");
        require(!whitelisted, "INNBC Validator Governance: target already whitelisted");
        require(stake >= IINNBCValidatorRegistry(validatorRegistry).minimumStake(), "INNBC Validator Governance: target stake too low");

        proposalID = _createProposal(
            ProposalType.Whitelist,
            target,
            0,
            ProposalDurationPreset.None,
            proposalDurationBlocks
        );
    }

    function createRemoveProposal(address target) external onlyActiveValidator returns (uint256 proposalID) {
        (
            ,
            ,
            ,
            ,
            ,
            bool registered,
            bool whitelisted
        ) = IINNBCValidatorRegistry(validatorRegistry).validators(target);

        require(registered, "INNBC Validator Governance: target not registered");
        require(whitelisted, "INNBC Validator Governance: target not whitelisted");

        proposalID = _createProposal(
            ProposalType.Remove,
            target,
            0,
            ProposalDurationPreset.None,
            proposalDurationBlocks
        );
    }

    function createEmergencyRemoveProposal(address target) external onlyActiveValidator returns (uint256 proposalID) {
        (
            ,
            ,
            ,
            ,
            ,
            bool registered,
            bool whitelisted
        ) = IINNBCValidatorRegistry(validatorRegistry).validators(target);

        require(registered, "INNBC Validator Governance: target not registered");
        require(whitelisted, "INNBC Validator Governance: target not whitelisted");

        proposalID = _createProposal(
            ProposalType.EmergencyRemove,
            target,
            0,
            ProposalDurationPreset.None,
            EMERGENCY_REMOVE_PROPOSAL_DURATION_BLOCKS
        );
    }

    function createUpdateMinimumStakeProposal(uint256 newMinimumStake) external onlyActiveValidator returns (uint256 proposalID) {
        require(newMinimumStake > 0, "INNBC Validator Governance: minimum stake is zero");
        require(
            newMinimumStake != IINNBCValidatorRegistry(validatorRegistry).minimumStake(),
            "INNBC Validator Governance: minimum stake unchanged"
        );

        proposalID = _createProposal(
            ProposalType.UpdateMinimumStake,
            address(0),
            newMinimumStake,
            ProposalDurationPreset.None,
            proposalDurationBlocks
        );
    }

    function createUseShortProposalDurationProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateProposalDurationProposal(ProposalDurationPreset.Short);
    }

    function createUseStandardProposalDurationProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateProposalDurationProposal(ProposalDurationPreset.Standard);
    }

    function createUseLongProposalDurationProposal() external onlyActiveValidator returns (uint256 proposalID) {
        proposalID = _createUpdateProposalDurationProposal(ProposalDurationPreset.Long);
    }

    function vote(uint256 proposalID, bool support) external onlyActiveValidator {
        Proposal storage proposal = proposals[proposalID];

        require(proposal.deadlineBlock != 0, "INNBC Validator Governance: proposal not found");
        require(block.number <= proposal.deadlineBlock, "INNBC Validator Governance: proposal expired");
        require(!proposal.finalized, "INNBC Validator Governance: proposal finalized");
        require(!hasVoted[proposalID][msg.sender], "INNBC Validator Governance: already voted");

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

        require(proposal.deadlineBlock != 0, "INNBC Validator Governance: proposal not found");
        require(!proposal.finalized, "INNBC Validator Governance: proposal finalized");

        /* START ALPHA CHANGE 3553 -- finalize against current validators plus recorded departed voters -- */
        uint256 totalVotes = proposal.yesVotes + proposal.noVotes;
        uint256 electorateSize = INNBCGovernanceElectorate.size(
            IINNBCValidatorRegistry(validatorRegistry), hasVoted[proposalID], totalVotes
        );
        uint256 requiredYesVotes = _requiredYesVotes(electorateSize);
        bool deadlinePassed = block.number > proposal.deadlineBlock;
        bool outcomeSettled = _isProposalOutcomeSettled(
            proposal, totalVotes, requiredYesVotes, electorateSize
        );

        require(deadlinePassed || outcomeSettled, "INNBC Validator Governance: proposal still active");

        bool passed = electorateSize > 0 && proposal.yesVotes >= requiredYesVotes;
        /* FINISH ALPHA CHANGE 3553 -- finalize against current validators plus recorded departed voters -- */

        if (passed && proposal.proposalType == ProposalType.Whitelist) {
            (
                ,
                ,
                ,
                ,
                uint256 stake,
                ,
            ) = IINNBCValidatorRegistry(validatorRegistry).validators(proposal.target);

            if (stake < IINNBCValidatorRegistry(validatorRegistry).minimumStake()) {
                passed = false;
            }
        }

        proposal.finalized = true;
        proposal.passed = passed;
        /* START ALPHA CHANGE 975 -- remove finalized validator-governance proposal from pending proposal index -- */
        _removeActiveProposal(proposalID);
        /* FINISH ALPHA CHANGE 975 -- remove finalized validator-governance proposal from pending proposal index -- */

        if (passed) {
            if (proposal.proposalType == ProposalType.Whitelist) {
                IINNBCValidatorRegistry(validatorRegistry).whitelistValidator(proposal.target);
            } else if (
                proposal.proposalType == ProposalType.Remove ||
                proposal.proposalType == ProposalType.EmergencyRemove
            ) {
                IINNBCValidatorRegistry(validatorRegistry).removeValidator(proposal.target);
            } else if (proposal.proposalType == ProposalType.UpdateMinimumStake) {
                IINNBCValidatorRegistry(validatorRegistry).updateMinimumStake(proposal.newMinimumStake);
            } else if (proposal.proposalType == ProposalType.UpdateProposalDuration) {
                proposalDurationPreset = proposal.newProposalDurationPreset;
                proposalDurationBlocks = _proposalDurationBlocks(proposal.newProposalDurationPreset);

                IINNBCValidatorRegistry(validatorRegistry).updateProposalDurationPreset(uint8(proposal.newProposalDurationPreset));

                emit ProposalDurationUpdated(proposal.newProposalDurationPreset, proposalDurationBlocks);
            }
        }

        emit ProposalFinalized(proposalID, passed);
    }

    function _createUpdateProposalDurationProposal(ProposalDurationPreset newProposalDurationPreset)
        private
        returns (uint256 proposalID)
    {
        require(newProposalDurationPreset != ProposalDurationPreset.None, "INNBC Validator Governance: empty duration preset");
        require(newProposalDurationPreset != proposalDurationPreset, "INNBC Validator Governance: duration unchanged");

        proposalID = _createProposal(
            ProposalType.UpdateProposalDuration,
            address(0),
            0,
            newProposalDurationPreset,
            proposalDurationBlocks
        );
    }

    function _createProposal(
        ProposalType proposalType,
        address target,
        uint256 newMinimumStake,
        ProposalDurationPreset newProposalDurationPreset,
        uint256 durationBlocks
    ) private returns (uint256 proposalID) {
        proposalID = nextProposalID++;

        proposals[proposalID] = Proposal({
            proposalType: proposalType,
            target: target,
            newMinimumStake: newMinimumStake,
            newProposalDurationPreset: newProposalDurationPreset,
            yesVotes: 0,
            noVotes: 0,
            deadlineBlock: block.number + durationBlocks,
            finalized: false,
            passed: false
        });

        /* START ALPHA CHANGE 975 -- add created validator-governance proposal to pending proposal index -- */
        _trackActiveProposal(proposalID);
        /* FINISH ALPHA CHANGE 975 -- add created validator-governance proposal to pending proposal index -- */

        emit ProposalCreated(proposalID, proposalType, target, block.number + durationBlocks);
    }

    function _proposalDurationBlocks(ProposalDurationPreset preset) private pure returns (uint256) {
        if (preset == ProposalDurationPreset.Short) {
            return SHORT_PROPOSAL_DURATION_BLOCKS;
        }

        if (preset == ProposalDurationPreset.Standard) {
            return STANDARD_PROPOSAL_DURATION_BLOCKS;
        }

        if (preset == ProposalDurationPreset.Long) {
            return LONG_PROPOSAL_DURATION_BLOCKS;
        }

        revert("INNBC Validator Governance: empty duration preset");
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
        ) = IINNBCValidatorRegistry(validatorRegistry).validators(operator);

        return registered &&
            whitelisted &&
            stake >= IINNBCValidatorRegistry(validatorRegistry).minimumStake();
    }

    /* START ALPHA CHANGE 3553 -- derive majority from the effective electorate -- */
    function _requiredYesVotes(uint256 electorateSize) private pure returns (uint256) {
        return (electorateSize / 2) + 1;
    }
    /* FINISH ALPHA CHANGE 3553 -- derive majority from the effective electorate -- */

    /* START ALPHA CHANGE 975 -- maintain pending validator-governance proposal index -- */
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
    /* FINISH ALPHA CHANGE 975 -- maintain pending validator-governance proposal index -- */

    /* START ALPHA CHANGE 3553 -- settle using the same effective electorate as approval -- */
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
    /* FINISH ALPHA CHANGE 3553 -- settle using the same effective electorate as approval -- */
}
/* FINISH ALPHA CHANGE 256 -- add INNBC validator governance contract source -- */