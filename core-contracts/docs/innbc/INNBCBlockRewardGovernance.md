# INNBCBlockRewardGovernance









## Methods

### BASIS_POINTS_DENOMINATOR

```solidity
function BASIS_POINTS_DENOMINATOR() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### EMERGENCY_REWARD_COOLDOWN_BLOCKS

```solidity
function EMERGENCY_REWARD_COOLDOWN_BLOCKS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### EMERGENCY_REWARD_ONE_PERCENT_BPS

```solidity
function EMERGENCY_REWARD_ONE_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### EMERGENCY_REWARD_THREE_PERCENT_BPS

```solidity
function EMERGENCY_REWARD_THREE_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### EMERGENCY_REWARD_TWO_PERCENT_BPS

```solidity
function EMERGENCY_REWARD_TWO_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### FEE_BURN_FIFTY_PERCENT_BPS

```solidity
function FEE_BURN_FIFTY_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### FEE_BURN_NINETY_PERCENT_BPS

```solidity
function FEE_BURN_NINETY_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### FEE_BURN_ONE_HUNDRED_PERCENT_BPS

```solidity
function FEE_BURN_ONE_HUNDRED_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### FEE_BURN_SEVENTY_PERCENT_BPS

```solidity
function FEE_BURN_SEVENTY_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### FEE_BURN_TEN_PERCENT_BPS

```solidity
function FEE_BURN_TEN_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### FEE_BURN_TWENTY_FIVE_PERCENT_BPS

```solidity
function FEE_BURN_TWENTY_FIVE_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### FEE_BURN_ZERO_PERCENT_BPS

```solidity
function FEE_BURN_ZERO_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### communityRewardPool

```solidity
function communityRewardPool() external view returns (address)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

### createOnePercentEmergencyRewardProposal

```solidity
function createOnePercentEmergencyRewardProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createThreePercentEmergencyRewardProposal

```solidity
function createThreePercentEmergencyRewardProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createTwoPercentEmergencyRewardProposal

```solidity
function createTwoPercentEmergencyRewardProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUpdateMaxBaseFeeProposal

```solidity
function createUpdateMaxBaseFeeProposal(uint256 newMaxBaseFeeWei) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| newMaxBaseFeeWei | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseFiftyPercentFeeBurnRateProposal

```solidity
function createUseFiftyPercentFeeBurnRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseNinetyPercentFeeBurnRateProposal

```solidity
function createUseNinetyPercentFeeBurnRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseOneHundredPercentFeeBurnRateProposal

```solidity
function createUseOneHundredPercentFeeBurnRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseSeventyPercentFeeBurnRateProposal

```solidity
function createUseSeventyPercentFeeBurnRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseTenPercentFeeBurnRateProposal

```solidity
function createUseTenPercentFeeBurnRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseTwentyFivePercentFeeBurnRateProposal

```solidity
function createUseTwentyFivePercentFeeBurnRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseZeroFeeBurnRateProposal

```solidity
function createUseZeroFeeBurnRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### emergencyRewardProposalActive

```solidity
function emergencyRewardProposalActive() external view returns (bool)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | bool | undefined |

### feeBurnRateBps

```solidity
function feeBurnRateBps() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### finalizeProposal

```solidity
function finalizeProposal(uint256 proposalID) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### getActiveProposalIDs

```solidity
function getActiveProposalIDs() external view returns (uint256[])
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256[] | undefined |

### hasVoted

```solidity
function hasVoted(uint256, address) external view returns (bool)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |
| _1 | address | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | bool | undefined |

### initialize

```solidity
function initialize(address validatorRegistry_, address nativeToken_, address communityRewardPool_) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| validatorRegistry_ | address | undefined |
| nativeToken_ | address | undefined |
| communityRewardPool_ | address | undefined |

### initialized

```solidity
function initialized() external view returns (bool)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | bool | undefined |

### lastEmergencyRewardBlock

```solidity
function lastEmergencyRewardBlock() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### maxBaseFeeWei

```solidity
function maxBaseFeeWei() external view returns (uint256)
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

### nextProposalID

```solidity
function nextProposalID() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### proposals

```solidity
function proposals(uint256) external view returns (enum INNBCBlockRewardGovernance.ProposalType proposalType, uint256 emergencyRewardRateBps, uint256 newFeeBurnRateBps, uint256 newMaxBaseFeeWei, uint256 yesVotes, uint256 noVotes, uint256 deadlineBlock, bool finalized, bool passed)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalType | enum INNBCBlockRewardGovernance.ProposalType | undefined |
| emergencyRewardRateBps | uint256 | undefined |
| newFeeBurnRateBps | uint256 | undefined |
| newMaxBaseFeeWei | uint256 | undefined |
| yesVotes | uint256 | undefined |
| noVotes | uint256 | undefined |
| deadlineBlock | uint256 | undefined |
| finalized | bool | undefined |
| passed | bool | undefined |

### validatorRegistry

```solidity
function validatorRegistry() external view returns (address)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

### vote

```solidity
function vote(uint256 proposalID, bool support) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |
| support | bool | undefined |



## Events

### EmergencyRewardDistributed

```solidity
event EmergencyRewardDistributed(uint256 indexed proposalID, uint256 totalReward, uint256 perValidatorReward, uint256 emergencyRewardRateBps)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID `indexed` | uint256 | undefined |
| totalReward  | uint256 | undefined |
| perValidatorReward  | uint256 | undefined |
| emergencyRewardRateBps  | uint256 | undefined |

### FeeBurnRateUpdated

```solidity
event FeeBurnRateUpdated(uint256 feeBurnRateBps)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| feeBurnRateBps  | uint256 | undefined |

### MaxBaseFeeUpdated

```solidity
event MaxBaseFeeUpdated(uint256 maxBaseFeeWei)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| maxBaseFeeWei  | uint256 | undefined |

### ProposalCreated

```solidity
event ProposalCreated(uint256 indexed proposalID, enum INNBCBlockRewardGovernance.ProposalType proposalType, uint256 deadlineBlock)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID `indexed` | uint256 | undefined |
| proposalType  | enum INNBCBlockRewardGovernance.ProposalType | undefined |
| deadlineBlock  | uint256 | undefined |

### ProposalFinalized

```solidity
event ProposalFinalized(uint256 indexed proposalID, bool passed)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID `indexed` | uint256 | undefined |
| passed  | bool | undefined |

### VoteCast

```solidity
event VoteCast(uint256 indexed proposalID, address indexed voter, bool support)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID `indexed` | uint256 | undefined |
| voter `indexed` | address | undefined |
| support  | bool | undefined |



