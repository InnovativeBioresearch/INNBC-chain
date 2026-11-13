/* START ALPHA CHANGE 62 -- add INNBC validator registry contract source -- */
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.19;

/* START ALPHA CHANGE 2026 -- verify validator possession through the fixed BLS system contract -- */
import "../interfaces/common/IBLS.sol";
/* FINISH ALPHA CHANGE 2026 -- verify validator possession through the fixed BLS system contract -- */

/* START ALPHA CHANGE 282 -- add native token interface for explicit validator staking -- */
interface IINNBCRegistryNativeToken {
    function transferFrom(address from, address to, uint256 amount) external returns (bool);
    function transfer(address to, uint256 amount) external returns (bool);
}
/* FINISH ALPHA CHANGE 282 -- add native token interface for explicit validator staking -- */

contract INNBCValidatorRegistry {
    enum ProposalDurationPreset {
        None,
        Short,
        Standard,
        Long
    }

    struct ValidatorRecord {
        address operator;
        address rewardAddress;
        bytes blsKey;
        string libp2pPeerID;
        uint256 stake;
        bool registered;
        bool whitelisted;
        /* START ALPHA CHANGE 2027 -- pack possession verification into the validator record -- */
        bool blsProofVerified;
        /* FINISH ALPHA CHANGE 2027 -- pack possession verification into the validator record -- */
    }

    uint256 public constant SHORT_PROPOSAL_DURATION_BLOCKS = 3600;
    uint256 public constant STANDARD_PROPOSAL_DURATION_BLOCKS = 43200;

    /* START ALPHA CHANGE 2028 -- define canonical INNBC possession proof parameters -- */
    address private constant BLS_CONTRACT = address(0x102);
    uint256 private constant BLS_PUBLIC_KEY_LENGTH = 128;
    uint256 private constant BLS_PROOF_LENGTH = 64;
    bytes32 private constant BLS_POP_DOMAIN =
        keccak256("DOMAIN_INNBC_BLS_PROOF_OF_POSSESSION");
    /* FINISH ALPHA CHANGE 2028 -- define canonical INNBC possession proof parameters -- */
    /* START ALPHA CHANGE 733 -- reduce long validator proposal duration preset to forty-eight hours -- */
    uint256 public constant LONG_PROPOSAL_DURATION_BLOCKS = 86400;
    /* FINISH ALPHA CHANGE 733 -- reduce long validator proposal duration preset to forty-eight hours -- */

    address public genesisValidator;
    address public validatorGovernance;
    /* START ALPHA CHANGE 283 -- store native token used for explicit validator staking -- */
    address public nativeToken;
    /* FINISH ALPHA CHANGE 283 -- store native token used for explicit validator staking -- */
    bool public initialized;

    bool public registryActivated;
    uint256 public activationEpoch;
    uint256 public minimumStake;

    ProposalDurationPreset public proposalDurationPreset;
    uint256 public proposalDurationBlocks;
    uint256 public activeValidatorCount;

    mapping(address => ValidatorRecord) public validators;

    address[] private activeValidators;
    mapping(address => uint256) private activeValidatorIndexPlusOne;

    mapping(address => address) private rewardAddressOwner;
    mapping(bytes32 => address) private blsKeyOwner;
    mapping(bytes32 => address) private libp2pPeerIDOwner;

    event ValidatorRegistered(address indexed operator, address indexed rewardAddress, uint256 stake);
    event RegistryActivated(address indexed genesisValidator, uint256 activationEpoch);
    event ValidatorWhitelisted(address indexed operator);
    event ValidatorRemoved(address indexed operator);
    event MinimumStakeUpdated(uint256 minimumStake);
    event ProposalDurationUpdated(ProposalDurationPreset proposalDurationPreset, uint256 proposalDurationBlocks);
    event RewardAddressUpdated(address indexed operator, address indexed rewardAddress);
    event BLSKeyUpdated(address indexed operator);
    event LibP2PPeerIDUpdated(address indexed operator, string libp2pPeerID);
    event StakeIncreased(address indexed operator, uint256 amount, uint256 totalStake);
    event StakeWithdrawn(address indexed operator, uint256 amount, uint256 remainingStake);

    modifier onlyRegistered(address operator) {
        require(validators[operator].registered, "INNBC Registry: validator not registered");
        _;
    }

    modifier onlyValidatorGovernance() {
        require(msg.sender == validatorGovernance, "INNBC Registry: sender not validator governance");
        _;
    }

    /* START ALPHA CHANGE 259 -- slim registry initialization around validator source-of-truth state -- */
    function initialize(
        address genesisValidator_,
        uint256 minimumStake_,
        ProposalDurationPreset proposalDurationPreset_,
        address validatorGovernance_,
        address nativeToken_,
        uint256 activationEpoch_
    ) external {
        require(!initialized, "INNBC Registry: already initialized");
        require(genesisValidator_ != address(0), "INNBC Registry: zero genesis validator");
        require(minimumStake_ > 0, "INNBC Registry: minimum stake is zero");
        require(proposalDurationPreset_ != ProposalDurationPreset.None, "INNBC Registry: empty duration preset");
        require(validatorGovernance_ != address(0), "INNBC Registry: zero validator governance");
        /* START ALPHA CHANGE 284 -- initialize native staking token and registry activation epoch -- */
        require(nativeToken_ != address(0), "INNBC Registry: zero native token");
        require(activationEpoch_ > 0, "INNBC Registry: zero activation epoch");
        /* FINISH ALPHA CHANGE 284 -- initialize native staking token and registry activation epoch -- */

        initialized = true;
        genesisValidator = genesisValidator_;
        minimumStake = minimumStake_;
        proposalDurationPreset = proposalDurationPreset_;
        proposalDurationBlocks = _proposalDurationBlocks(proposalDurationPreset_);
        validatorGovernance = validatorGovernance_;
        /* START ALPHA CHANGE 285 -- persist native staking token and activation epoch from genesis config -- */
        nativeToken = nativeToken_;
        activationEpoch = activationEpoch_;
        /* FINISH ALPHA CHANGE 285 -- persist native staking token and activation epoch from genesis config -- */
    }
    /* FINISH ALPHA CHANGE 259 -- slim registry initialization around validator source-of-truth state -- */

    /* START ALPHA CHANGE 2029 -- require canonical possession proof before registering any validator -- */
    function registerValidator(
        address rewardAddress,
        bytes calldata blsKey,
        bytes calldata blsProof,
        string calldata libp2pPeerID,
        uint256 stakeAmount
    ) external {
        require(initialized, "INNBC Registry: not initialized");
        require(rewardAddress != address(0), "INNBC Registry: zero reward address");
        require(blsKey.length == BLS_PUBLIC_KEY_LENGTH, "INNBC Registry: invalid BLS key length");
        require(blsProof.length == BLS_PROOF_LENGTH, "INNBC Registry: invalid BLS proof length");
        require(bytes(libp2pPeerID).length > 0, "INNBC Registry: empty peer ID");
        require(!validators[msg.sender].registered, "INNBC Registry: already registered");
        require(stakeAmount >= minimumStake, "INNBC Registry: stake too low");
        require(
            registryActivated || msg.sender == genesisValidator,
            "INNBC Registry: first validator must be genesis"
        );

        _requireIdentityAvailable(rewardAddress, blsKey, libp2pPeerID, msg.sender);
        _requireValidBLSPoP(msg.sender, blsKey, blsProof);

        if (!registryActivated) {
            registryActivated = true;
            emit RegistryActivated(msg.sender, activationEpoch);
        }

        require(
            IINNBCRegistryNativeToken(nativeToken).transferFrom(msg.sender, address(this), stakeAmount),
            "INNBC Registry: stake transfer failed"
        );

        validators[msg.sender] = ValidatorRecord({
            operator: msg.sender,
            rewardAddress: rewardAddress,
            blsKey: blsKey,
            libp2pPeerID: libp2pPeerID,
            stake: stakeAmount,
            registered: true,
            whitelisted: msg.sender == genesisValidator,
            blsProofVerified: true
        });

        _setIdentityIndexes(msg.sender, rewardAddress, blsKey, libp2pPeerID);

        if (msg.sender == genesisValidator) {
            _activateValidator(msg.sender);
        }

        emit ValidatorRegistered(msg.sender, rewardAddress, stakeAmount);
    }
    /* FINISH ALPHA CHANGE 2029 -- require canonical possession proof before registering any validator -- */

    function increaseStake(uint256 stakeAmount) external onlyRegistered(msg.sender) {
        require(stakeAmount > 0, "INNBC Registry: zero stake");

        /* START ALPHA CHANGE 288 -- pull explicit additional validator stake from native token allowance -- */
        require(
            IINNBCRegistryNativeToken(nativeToken).transferFrom(msg.sender, address(this), stakeAmount),
            "INNBC Registry: stake transfer failed"
        );
        /* FINISH ALPHA CHANGE 288 -- pull explicit additional validator stake from native token allowance -- */

        validators[msg.sender].stake += stakeAmount;

        emit StakeIncreased(msg.sender, stakeAmount, validators[msg.sender].stake);
    }

    function unstake(uint256 amount) external onlyRegistered(msg.sender) {
        require(amount > 0, "INNBC Registry: zero unstake amount");

        ValidatorRecord storage validator = validators[msg.sender];

        require(amount <= validator.stake, "INNBC Registry: unstake exceeds stake");

        validator.stake -= amount;

        if (validator.whitelisted && validator.stake < minimumStake) {
            validator.whitelisted = false;
            _deactivateValidator(msg.sender);
            emit ValidatorRemoved(msg.sender);
        }

        /* START ALPHA CHANGE 289 -- return unstaked validator funds through native token transfer -- */
        require(
            IINNBCRegistryNativeToken(nativeToken).transfer(msg.sender, amount),
            "INNBC Registry: unstake transfer failed"
        );
        /* FINISH ALPHA CHANGE 289 -- return unstaked validator funds through native token transfer -- */

        emit StakeWithdrawn(msg.sender, amount, validator.stake);
    }

    function updateRewardAddress(address rewardAddress) external onlyRegistered(msg.sender) {
        require(rewardAddress != address(0), "INNBC Registry: zero reward address");

        address currentRewardAddress = validators[msg.sender].rewardAddress;
        address currentOwner = rewardAddressOwner[rewardAddress];

        require(currentOwner == address(0) || currentOwner == msg.sender, "INNBC Registry: reward address already used");

        if (currentRewardAddress != rewardAddress) {
            delete rewardAddressOwner[currentRewardAddress];
            rewardAddressOwner[rewardAddress] = msg.sender;
            validators[msg.sender].rewardAddress = rewardAddress;
        }

        emit RewardAddressUpdated(msg.sender, rewardAddress);
    }

    function updateLibP2PPeerID(string calldata libp2pPeerID) external onlyRegistered(msg.sender) {
        require(bytes(libp2pPeerID).length > 0, "INNBC Registry: empty peer ID");

        bytes32 currentPeerIDHash = keccak256(bytes(validators[msg.sender].libp2pPeerID));
        bytes32 newPeerIDHash = keccak256(bytes(libp2pPeerID));
        address currentOwner = libp2pPeerIDOwner[newPeerIDHash];

        require(currentOwner == address(0) || currentOwner == msg.sender, "INNBC Registry: peer ID already used");

        if (currentPeerIDHash != newPeerIDHash) {
            delete libp2pPeerIDOwner[currentPeerIDHash];
            libp2pPeerIDOwner[newPeerIDHash] = msg.sender;
            validators[msg.sender].libp2pPeerID = libp2pPeerID;
        }

        emit LibP2PPeerIDUpdated(msg.sender, libp2pPeerID);
    }

    /* START ALPHA CHANGE 2030 -- verify replacement key possession and reject unchanged updates before pairing -- */
    function updateBLSKey(
        bytes calldata blsKey,
        bytes calldata blsProof
    ) external onlyRegistered(msg.sender) {
        require(blsKey.length == BLS_PUBLIC_KEY_LENGTH, "INNBC Registry: invalid BLS key length");
        require(blsProof.length == BLS_PROOF_LENGTH, "INNBC Registry: invalid BLS proof length");

        ValidatorRecord storage record = validators[msg.sender];
        bytes32 currentBLSKeyHash = keccak256(record.blsKey);
        bytes32 newBLSKeyHash = keccak256(blsKey);

        require(currentBLSKeyHash != newBLSKeyHash, "INNBC Registry: BLS key unchanged");
        require(blsKeyOwner[newBLSKeyHash] == address(0), "INNBC Registry: BLS key already used");

        _requireValidBLSPoP(msg.sender, blsKey, blsProof);

        delete blsKeyOwner[currentBLSKeyHash];
        blsKeyOwner[newBLSKeyHash] = msg.sender;
        record.blsKey = blsKey;
        record.blsProofVerified = true;

        if (record.whitelisted) {
            record.whitelisted = false;
            _deactivateValidator(msg.sender);
            emit ValidatorRemoved(msg.sender);
        }

        emit BLSKeyUpdated(msg.sender);
    }
    /* FINISH ALPHA CHANGE 2030 -- verify replacement key possession and reject unchanged updates before pairing -- */

    /* START ALPHA CHANGE 260 -- expose controller-only validator state mutation hooks -- */
    function whitelistValidator(address operator) external onlyValidatorGovernance onlyRegistered(operator) {
        require(validators[operator].stake >= minimumStake, "INNBC Registry: stake too low");

        if (!validators[operator].whitelisted) {
            validators[operator].whitelisted = true;
            _activateValidator(operator);

            emit ValidatorWhitelisted(operator);
        }
    }

    function removeValidator(address operator) external onlyValidatorGovernance onlyRegistered(operator) {
        if (validators[operator].whitelisted) {
            validators[operator].whitelisted = false;
            _deactivateValidator(operator);

            emit ValidatorRemoved(operator);
        }
    }

    function updateMinimumStake(uint256 newMinimumStake) external onlyValidatorGovernance {
        require(newMinimumStake > 0, "INNBC Registry: minimum stake is zero");

        minimumStake = newMinimumStake;
        _removeUnderStakedActiveValidators();

        emit MinimumStakeUpdated(newMinimumStake);
    }

    function updateProposalDurationPreset(uint8 newProposalDurationPreset) external onlyValidatorGovernance {
        ProposalDurationPreset preset = ProposalDurationPreset(newProposalDurationPreset);

        require(preset != ProposalDurationPreset.None, "INNBC Registry: empty duration preset");

        proposalDurationPreset = preset;
        proposalDurationBlocks = _proposalDurationBlocks(preset);

        emit ProposalDurationUpdated(preset, proposalDurationBlocks);
    }
    /* FINISH ALPHA CHANGE 260 -- expose controller-only validator state mutation hooks -- */

    function getActiveValidators() external view returns (address[] memory) {
        return activeValidators;
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

        revert("INNBC Registry: empty duration preset");
    }

    /* START ALPHA CHANGE 2031 -- reject infinity and verify the exact operator-bound BLS possession message -- */
    function _requireValidBLSPoP(
        address operator,
        bytes calldata blsKey,
        bytes calldata blsProof
    ) private view {
        uint256[2] memory signature = [
            uint256(bytes32(blsProof[0:32])),
            uint256(bytes32(blsProof[32:64]))
        ];

        uint256[4] memory publicKey = [
            uint256(bytes32(blsKey[32:64])),
            uint256(bytes32(blsKey[0:32])),
            uint256(bytes32(blsKey[96:128])),
            uint256(bytes32(blsKey[64:96]))
        ];

        require(
            signature[0] != 0 || signature[1] != 0,
            "INNBC Registry: BLS proof is infinity"
        );
        require(
            publicKey[0] != 0 || publicKey[1] != 0 ||
            publicKey[2] != 0 || publicKey[3] != 0,
            "INNBC Registry: BLS key is infinity"
        );

        uint256[2] memory message = IBLS(BLS_CONTRACT).hashToPoint(
            BLS_POP_DOMAIN,
            abi.encodePacked(operator, blsKey)
        );

        (bool valid, bool callSuccess) = IBLS(BLS_CONTRACT).verifySingle(
            signature,
            publicKey,
            message
        );

        require(callSuccess && valid, "INNBC Registry: invalid BLS proof");
    }
    /* FINISH ALPHA CHANGE 2031 -- reject infinity and verify the exact operator-bound BLS possession message -- */

    function _requireIdentityAvailable(
        address rewardAddress,
        bytes calldata blsKey,
        string calldata libp2pPeerID,
        address operator
    ) private view {
        address existingRewardOwner = rewardAddressOwner[rewardAddress];
        address existingBLSOwner = blsKeyOwner[keccak256(blsKey)];
        address existingPeerOwner = libp2pPeerIDOwner[keccak256(bytes(libp2pPeerID))];

        require(existingRewardOwner == address(0) || existingRewardOwner == operator, "INNBC Registry: reward address already used");
        require(existingBLSOwner == address(0) || existingBLSOwner == operator, "INNBC Registry: BLS key already used");
        require(existingPeerOwner == address(0) || existingPeerOwner == operator, "INNBC Registry: peer ID already used");
    }

    function _setIdentityIndexes(
        address operator,
        address rewardAddress,
        bytes calldata blsKey,
        string calldata libp2pPeerID
    ) private {
        rewardAddressOwner[rewardAddress] = operator;
        blsKeyOwner[keccak256(blsKey)] = operator;
        libp2pPeerIDOwner[keccak256(bytes(libp2pPeerID))] = operator;
    }

    function _activateValidator(address operator) private {
        /* START ALPHA CHANGE 2032 -- enforce proof verification at the common activation gate -- */
        require(
            validators[operator].registered && validators[operator].blsProofVerified,
            "INNBC Registry: BLS proof not verified"
        );
        /* FINISH ALPHA CHANGE 2032 -- enforce proof verification at the common activation gate -- */

        if (activeValidatorIndexPlusOne[operator] != 0) {
            return;
        }

        activeValidators.push(operator);
        activeValidatorIndexPlusOne[operator] = activeValidators.length;
        activeValidatorCount++;
    }

    function _removeUnderStakedActiveValidators() private {
        uint256 index = 0;

        while (index < activeValidators.length) {
            address operator = activeValidators[index];

            if (validators[operator].stake < minimumStake) {
                validators[operator].whitelisted = false;
                _deactivateValidator(operator);
                emit ValidatorRemoved(operator);
            } else {
                index++;
            }
        }
    }

    function _deactivateValidator(address operator) private {
        uint256 indexPlusOne = activeValidatorIndexPlusOne[operator];

        if (indexPlusOne == 0) {
            return;
        }

        uint256 index = indexPlusOne - 1;
        uint256 lastIndex = activeValidators.length - 1;

        if (index != lastIndex) {
            address lastValidator = activeValidators[lastIndex];
            activeValidators[index] = lastValidator;
            activeValidatorIndexPlusOne[lastValidator] = index + 1;
        }

        activeValidators.pop();
        delete activeValidatorIndexPlusOne[operator];
        activeValidatorCount--;
    }
}
/* FINISH ALPHA CHANGE 62 -- add INNBC validator registry contract source -- */