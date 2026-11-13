/* START ALPHA CHANGE 816 -- add non-upgradeable proxy admin governance contract -- */
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

import "./IINNBCValidatorRegistry.sol";
/* START ALPHA CHANGE 3557 -- share the effective electorate calculation -- */
import "./INNBCGovernanceElectorate.sol";
/* FINISH ALPHA CHANGE 3557 -- share the effective electorate calculation -- */

/* START ALPHA CHANGE 983 -- route finalized upgrades through isolated proxy admin executor -- */
interface IINNBCProxyAdminExecutor {
    function executeUpgrade(address proxy, address implementation, bytes calldata data) external;
}
/* FINISH ALPHA CHANGE 983 -- route finalized upgrades through isolated proxy admin executor -- */

contract INNBCProxyAdminGovernance {
    struct UpgradeProposal {
        address proxy;
        address implementation;
        bytes data;
        uint256 yesVotes;
        uint256 noVotes;
        uint256 deadlineBlock;
        bool finalized;
        bool passed;
        bool executed;
    }

    address private constant VALIDATOR_SET_PROXY = address(0x101);
    /* START ALPHA CHANGE 3772 -- retire the Merkle target from proxy upgrade governance -- */
    address private constant BLS_PROXY = address(0x102);
    address private constant INNBC_VALIDATOR_REGISTRY_PROXY = address(0x107);
    /* FINISH ALPHA CHANGE 3772 -- retire the Merkle target from proxy upgrade governance -- */
    address private constant INNBC_TREASURY_DAO_PROXY = address(0x108);
    address private constant INNBC_VALIDATOR_GOVERNANCE_PROXY = address(0x109);
    address private constant INNBC_BLOCK_REWARD_GOVERNANCE_PROXY = address(0x10A);
    address private constant INNBC_FEE_BURN_VAULT_PROXY = address(0x10B);
    /* START ALPHA CHANGE 970 -- allow community reward pool proxy upgrades through proxy admin governance -- */
    address private constant INNBC_COMMUNITY_REWARD_POOL_PROXY = address(0x10D);
    /* FINISH ALPHA CHANGE 970 -- allow community reward pool proxy upgrades through proxy admin governance -- */
    address private constant INNBC_NATIVE_TOKEN_PROXY = address(0x1010);

    address public validatorRegistry;
    /* START ALPHA CHANGE 984 -- store isolated proxy admin executor address -- */
    address public proxyAdminExecutor;
    /* FINISH ALPHA CHANGE 984 -- store isolated proxy admin executor address -- */
    bool public initialized;
    uint256 public nextProposalID;

    mapping(uint256 => UpgradeProposal) public proposals;
    mapping(uint256 => mapping(address => bool)) public hasVoted;
    /* START ALPHA CHANGE 975 -- track pending proxy-admin proposal IDs without scanning historical proposals -- */
    uint256[] private activeProposalIDs;
    mapping(uint256 => uint256) private activeProposalIndexPlusOne;
    /* FINISH ALPHA CHANGE 975 -- track pending proxy-admin proposal IDs without scanning historical proposals -- */

    event UpgradeProposalCreated(
        uint256 indexed proposalID,
        address indexed proxy,
        address indexed implementation,
        uint256 deadlineBlock
    );
    event VoteCast(uint256 indexed proposalID, address indexed voter, bool support);
    event ProposalFinalized(uint256 indexed proposalID, bool passed);
    event UpgradeExecuted(uint256 indexed proposalID, address indexed proxy, address indexed implementation);

    modifier onlyActiveValidator() {
        require(_isActiveValidator(msg.sender), "INNBC Proxy Admin: sender not active validator");
        _;
    }

    /* START ALPHA CHANGE 985 -- initialize governance separately from proxy admin executor -- */
    function initialize(address validatorRegistry_, address proxyAdminExecutor_) external {
        require(!initialized, "INNBC Proxy Admin: already initialized");
        require(validatorRegistry_ != address(0), "INNBC Proxy Admin: zero validator registry");
        require(proxyAdminExecutor_ != address(0), "INNBC Proxy Admin: zero proxy admin executor");

        initialized = true;
        validatorRegistry = validatorRegistry_;
        proxyAdminExecutor = proxyAdminExecutor_;
    }
    /* FINISH ALPHA CHANGE 985 -- initialize governance separately from proxy admin executor -- */

    /* START ALPHA CHANGE 975 -- expose pending proxy-admin proposal IDs -- */
    function getActiveProposalIDs() external view returns (uint256[] memory) {
        return activeProposalIDs;
    }
    /* FINISH ALPHA CHANGE 975 -- expose pending proxy-admin proposal IDs -- */

    function createUpgradeProposal(
        address proxy,
        address implementation,
        bytes calldata data
    ) external onlyActiveValidator returns (uint256 proposalID) {
        require(_isAllowedProxy(proxy), "INNBC Proxy Admin: proxy not allowed");
        require(implementation != address(0), "INNBC Proxy Admin: zero implementation");
        require(implementation.code.length > 0, "INNBC Proxy Admin: implementation has no code");

        uint256 deadlineBlock = block.number + IINNBCValidatorRegistry(validatorRegistry).proposalDurationBlocks();

        proposalID = nextProposalID++;
        proposals[proposalID] = UpgradeProposal({
            proxy: proxy,
            implementation: implementation,
            data: data,
            yesVotes: 0,
            noVotes: 0,
            deadlineBlock: deadlineBlock,
            finalized: false,
            passed: false,
            executed: false
        });

        /* START ALPHA CHANGE 975 -- add created proxy-admin proposal to pending proposal index -- */
        _trackActiveProposal(proposalID);
        /* FINISH ALPHA CHANGE 975 -- add created proxy-admin proposal to pending proposal index -- */

        emit UpgradeProposalCreated(proposalID, proxy, implementation, deadlineBlock);
    }

    function vote(uint256 proposalID, bool support) external onlyActiveValidator {
        UpgradeProposal storage proposal = proposals[proposalID];

        require(proposal.deadlineBlock != 0, "INNBC Proxy Admin: proposal not found");
        require(block.number <= proposal.deadlineBlock, "INNBC Proxy Admin: proposal expired");
        require(!proposal.finalized, "INNBC Proxy Admin: proposal finalized");
        require(!hasVoted[proposalID][msg.sender], "INNBC Proxy Admin: already voted");

        hasVoted[proposalID][msg.sender] = true;

        if (support) {
            proposal.yesVotes++;
        } else {
            proposal.noVotes++;
        }

        emit VoteCast(proposalID, msg.sender, support);
    }

    function finalizeProposal(uint256 proposalID) external {
        UpgradeProposal storage proposal = proposals[proposalID];

        require(proposal.deadlineBlock != 0, "INNBC Proxy Admin: proposal not found");
        require(!proposal.finalized, "INNBC Proxy Admin: proposal finalized");

        /* START ALPHA CHANGE 3557 -- finalize against current validators plus recorded departed voters -- */
        uint256 totalVotes = proposal.yesVotes + proposal.noVotes;
        uint256 electorateSize = INNBCGovernanceElectorate.size(
            IINNBCValidatorRegistry(validatorRegistry), hasVoted[proposalID], totalVotes
        );
        uint256 requiredYesVotes = _requiredYesVotes(electorateSize);
        bool deadlinePassed = block.number > proposal.deadlineBlock;
        bool outcomeSettled = _isProposalOutcomeSettled(
            proposal, totalVotes, requiredYesVotes, electorateSize
        );

        require(deadlinePassed || outcomeSettled, "INNBC Proxy Admin: proposal still active");

        bool passed = electorateSize > 0 && proposal.yesVotes >= requiredYesVotes;
        /* FINISH ALPHA CHANGE 3557 -- finalize against current validators plus recorded departed voters -- */

        proposal.finalized = true;
        proposal.passed = passed;
        /* START ALPHA CHANGE 975 -- remove finalized proxy-admin proposal from pending proposal index -- */
        _removeActiveProposal(proposalID);
        /* FINISH ALPHA CHANGE 975 -- remove finalized proxy-admin proposal from pending proposal index -- */

        emit ProposalFinalized(proposalID, passed);
    }

    function executeUpgrade(uint256 proposalID) external {
        UpgradeProposal storage proposal = proposals[proposalID];

        require(proposal.deadlineBlock != 0, "INNBC Proxy Admin: proposal not found");
        require(proposal.finalized, "INNBC Proxy Admin: proposal not finalized");
        require(proposal.passed, "INNBC Proxy Admin: proposal not passed");
        require(!proposal.executed, "INNBC Proxy Admin: proposal executed");
        require(_isAllowedProxy(proposal.proxy), "INNBC Proxy Admin: proxy not allowed");
        require(proposal.implementation.code.length > 0, "INNBC Proxy Admin: implementation has no code");

    /* START ALPHA CHANGE 986 -- execute finalized upgrade through isolated proxy admin executor -- */
    proposal.executed = true;

    IINNBCProxyAdminExecutor(proxyAdminExecutor).executeUpgrade(
        proposal.proxy,
        proposal.implementation,
        proposal.data
    );

    emit UpgradeExecuted(proposalID, proposal.proxy, proposal.implementation);
    /* FINISH ALPHA CHANGE 986 -- execute finalized upgrade through isolated proxy admin executor -- */
    }

    /* START ALPHA CHANGE 3772 -- retire the Merkle target from proxy upgrade governance -- */
    function _isAllowedProxy(address proxy) private pure returns (bool) {
        return proxy == VALIDATOR_SET_PROXY ||
            proxy == BLS_PROXY ||
            proxy == INNBC_VALIDATOR_REGISTRY_PROXY ||
            proxy == INNBC_TREASURY_DAO_PROXY ||
            proxy == INNBC_VALIDATOR_GOVERNANCE_PROXY ||
            proxy == INNBC_BLOCK_REWARD_GOVERNANCE_PROXY ||
            proxy == INNBC_FEE_BURN_VAULT_PROXY ||
            /* START ALPHA CHANGE 971 -- include community reward pool in allowed upgradeable proxies -- */
            proxy == INNBC_COMMUNITY_REWARD_POOL_PROXY ||
            /* FINISH ALPHA CHANGE 971 -- include community reward pool in allowed upgradeable proxies -- */
            proxy == INNBC_NATIVE_TOKEN_PROXY;
    }
    /* FINISH ALPHA CHANGE 3772 -- retire the Merkle target from proxy upgrade governance -- */

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

    /* START ALPHA CHANGE 3557 -- derive majority from the effective electorate -- */
    function _requiredYesVotes(uint256 electorateSize) private pure returns (uint256) {
        return (electorateSize / 2) + 1;
    }
    /* FINISH ALPHA CHANGE 3557 -- derive majority from the effective electorate -- */

    /* START ALPHA CHANGE 975 -- maintain pending proxy-admin proposal index -- */
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
    /* FINISH ALPHA CHANGE 975 -- maintain pending proxy-admin proposal index -- */

    /* START ALPHA CHANGE 3557 -- settle using the same effective electorate as approval -- */
    function _isProposalOutcomeSettled(
        UpgradeProposal storage proposal,
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
    /* FINISH ALPHA CHANGE 3557 -- settle using the same effective electorate as approval -- */
}
/* FINISH ALPHA CHANGE 816 -- add non-upgradeable proxy admin governance contract -- */