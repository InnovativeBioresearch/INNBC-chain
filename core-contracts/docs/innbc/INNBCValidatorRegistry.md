# INNBCValidatorRegistry









## Methods

### LONG_PROPOSAL_DURATION_BLOCKS

```solidity
function LONG_PROPOSAL_DURATION_BLOCKS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### SHORT_PROPOSAL_DURATION_BLOCKS

```solidity
function SHORT_PROPOSAL_DURATION_BLOCKS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### STANDARD_PROPOSAL_DURATION_BLOCKS

```solidity
function STANDARD_PROPOSAL_DURATION_BLOCKS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### activationEpoch

```solidity
function activationEpoch() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### activeValidatorCount

```solidity
function activeValidatorCount() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### genesisValidator

```solidity
function genesisValidator() external view returns (address)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

### getActiveValidators

```solidity
function getActiveValidators() external view returns (address[])
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address[] | undefined |

### increaseStake

```solidity
function increaseStake(uint256 stakeAmount) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| stakeAmount | uint256 | undefined |

### initialize

```solidity
function initialize(address genesisValidator_, uint256 minimumStake_, enum INNBCValidatorRegistry.ProposalDurationPreset proposalDurationPreset_, address validatorGovernance_, address nativeToken_, uint256 activationEpoch_) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| genesisValidator_ | address | undefined |
| minimumStake_ | uint256 | undefined |
| proposalDurationPreset_ | enum INNBCValidatorRegistry.ProposalDurationPreset | undefined |
| validatorGovernance_ | address | undefined |
| nativeToken_ | address | undefined |
| activationEpoch_ | uint256 | undefined |

### initialized

```solidity
function initialized() external view returns (bool)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | bool | undefined |

### minimumStake

```solidity
function minimumStake() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### nativeToken

```solidity
function nativeToken() external view returns (address)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

### proposalDurationBlocks

```solidity
function proposalDurationBlocks() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### proposalDurationPreset

```solidity
function proposalDurationPreset() external view returns (enum INNBCValidatorRegistry.ProposalDurationPreset)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | enum INNBCValidatorRegistry.ProposalDurationPreset | undefined |

### registerValidator

```solidity
function registerValidator(address rewardAddress, bytes blsKey, bytes blsProof, string libp2pPeerID, uint256 stakeAmount) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| rewardAddress | address | undefined |
| blsKey | bytes | undefined |
| blsProof | bytes | undefined |
| libp2pPeerID | string | undefined |
| stakeAmount | uint256 | undefined |

### registryActivated

```solidity
function registryActivated() external view returns (bool)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | bool | undefined |

### removeValidator

```solidity
function removeValidator(address operator) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator | address | undefined |

### unstake

```solidity
function unstake(uint256 amount) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| amount | uint256 | undefined |

### updateBLSKey

```solidity
function updateBLSKey(bytes blsKey, bytes blsProof) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| blsKey | bytes | undefined |
| blsProof | bytes | undefined |

### updateLibP2PPeerID

```solidity
function updateLibP2PPeerID(string libp2pPeerID) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| libp2pPeerID | string | undefined |

### updateMinimumStake

```solidity
function updateMinimumStake(uint256 newMinimumStake) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| newMinimumStake | uint256 | undefined |

### updateProposalDurationPreset

```solidity
function updateProposalDurationPreset(uint8 newProposalDurationPreset) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| newProposalDurationPreset | uint8 | undefined |

### updateRewardAddress

```solidity
function updateRewardAddress(address rewardAddress) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| rewardAddress | address | undefined |

### validatorGovernance

```solidity
function validatorGovernance() external view returns (address)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

### validators

```solidity
function validators(address) external view returns (address operator, address rewardAddress, bytes blsKey, string libp2pPeerID, uint256 stake, bool registered, bool whitelisted, bool blsProofVerified)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| operator | address | undefined |
| rewardAddress | address | undefined |
| blsKey | bytes | undefined |
| libp2pPeerID | string | undefined |
| stake | uint256 | undefined |
| registered | bool | undefined |
| whitelisted | bool | undefined |
| blsProofVerified | bool | undefined |

### whitelistValidator

```solidity
function whitelistValidator(address operator) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator | address | undefined |



## Events

### BLSKeyUpdated

```solidity
event BLSKeyUpdated(address indexed operator)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator `indexed` | address | undefined |

### LibP2PPeerIDUpdated

```solidity
event LibP2PPeerIDUpdated(address indexed operator, string libp2pPeerID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator `indexed` | address | undefined |
| libp2pPeerID  | string | undefined |

### MinimumStakeUpdated

```solidity
event MinimumStakeUpdated(uint256 minimumStake)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| minimumStake  | uint256 | undefined |

### ProposalDurationUpdated

```solidity
event ProposalDurationUpdated(enum INNBCValidatorRegistry.ProposalDurationPreset proposalDurationPreset, uint256 proposalDurationBlocks)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalDurationPreset  | enum INNBCValidatorRegistry.ProposalDurationPreset | undefined |
| proposalDurationBlocks  | uint256 | undefined |

### RegistryActivated

```solidity
event RegistryActivated(address indexed genesisValidator, uint256 activationEpoch)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| genesisValidator `indexed` | address | undefined |
| activationEpoch  | uint256 | undefined |

### RewardAddressUpdated

```solidity
event RewardAddressUpdated(address indexed operator, address indexed rewardAddress)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator `indexed` | address | undefined |
| rewardAddress `indexed` | address | undefined |

### StakeIncreased

```solidity
event StakeIncreased(address indexed operator, uint256 amount, uint256 totalStake)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator `indexed` | address | undefined |
| amount  | uint256 | undefined |
| totalStake  | uint256 | undefined |

### StakeWithdrawn

```solidity
event StakeWithdrawn(address indexed operator, uint256 amount, uint256 remainingStake)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator `indexed` | address | undefined |
| amount  | uint256 | undefined |
| remainingStake  | uint256 | undefined |

### ValidatorRegistered

```solidity
event ValidatorRegistered(address indexed operator, address indexed rewardAddress, uint256 stake)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator `indexed` | address | undefined |
| rewardAddress `indexed` | address | undefined |
| stake  | uint256 | undefined |

### ValidatorRemoved

```solidity
event ValidatorRemoved(address indexed operator)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator `indexed` | address | undefined |

### ValidatorWhitelisted

```solidity
event ValidatorWhitelisted(address indexed operator)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator `indexed` | address | undefined |



