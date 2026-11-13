# INNBCCommunityRewardPool









## Methods

### BASIS_POINTS_DENOMINATOR

```solidity
function BASIS_POINTS_DENOMINATOR() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### COMMUNITY_REWARD_FIFTY_PERCENT_BPS

```solidity
function COMMUNITY_REWARD_FIFTY_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### COMMUNITY_REWARD_FIVE_PERCENT_BPS

```solidity
function COMMUNITY_REWARD_FIVE_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### COMMUNITY_REWARD_TEN_PERCENT_BPS

```solidity
function COMMUNITY_REWARD_TEN_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### COMMUNITY_REWARD_TWENTY_FIVE_PERCENT_BPS

```solidity
function COMMUNITY_REWARD_TWENTY_FIVE_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### COMMUNITY_REWARD_ZERO_PERCENT_BPS

```solidity
function COMMUNITY_REWARD_ZERO_PERCENT_BPS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### REWARD_PRECISION

```solidity
function REWARD_PRECISION() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### accountedRewardBalance

```solidity
function accountedRewardBalance() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### blockRewardGovernance

```solidity
function blockRewardGovernance() external view returns (address)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

### claim

```solidity
function claim() external nonpayable
```






### communityRewardRateBps

```solidity
function communityRewardRateBps() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### createUseFiftyPercentCommunityRewardRateProposal

```solidity
function createUseFiftyPercentCommunityRewardRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseFivePercentCommunityRewardRateProposal

```solidity
function createUseFivePercentCommunityRewardRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseTenPercentCommunityRewardRateProposal

```solidity
function createUseTenPercentCommunityRewardRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseTwentyFivePercentCommunityRewardRateProposal

```solidity
function createUseTwentyFivePercentCommunityRewardRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseZeroCommunityRewardRateProposal

```solidity
function createUseZeroCommunityRewardRateProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### earned

```solidity
function earned(address account) external view returns (uint256)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| account | address | undefined |

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
function initialize(address validatorRegistry_, address blockRewardGovernance_) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| validatorRegistry_ | address | undefined |
| blockRewardGovernance_ | address | undefined |

### initialized

```solidity
function initialized() external view returns (bool)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | bool | undefined |

### nextProposalID

```solidity
function nextProposalID() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### pendingRewards

```solidity
function pendingRewards(address) external view returns (uint256)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### proposals

```solidity
function proposals(uint256) external view returns (enum INNBCCommunityRewardPool.ProposalType proposalType, uint256 newCommunityRewardRateBps, uint256 yesVotes, uint256 noVotes, uint256 deadlineBlock, bool finalized, bool passed)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalType | enum INNBCCommunityRewardPool.ProposalType | undefined |
| newCommunityRewardRateBps | uint256 | undefined |
| yesVotes | uint256 | undefined |
| noVotes | uint256 | undefined |
| deadlineBlock | uint256 | undefined |
| finalized | bool | undefined |
| passed | bool | undefined |

### rewardPerTokenStored

```solidity
function rewardPerTokenStored() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### stake

```solidity
function stake() external payable
```






### stakedBalance

```solidity
function stakedBalance(address) external view returns (uint256)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### totalStaked

```solidity
function totalStaked() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### unstake

```solidity
function unstake(uint256 amount) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| amount | uint256 | undefined |

### userRewardPerTokenPaid

```solidity
function userRewardPerTokenPaid(address) external view returns (uint256)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

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

### CommunityRewardDeposit

```solidity
event CommunityRewardDeposit(address indexed sender, uint256 amount)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| sender `indexed` | address | undefined |
| amount  | uint256 | undefined |

### CommunityRewardRateUpdated

```solidity
event CommunityRewardRateUpdated(uint256 communityRewardRateBps)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| communityRewardRateBps  | uint256 | undefined |

### ProposalCreated

```solidity
event ProposalCreated(uint256 indexed proposalID, enum INNBCCommunityRewardPool.ProposalType proposalType, uint256 deadlineBlock)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID `indexed` | uint256 | undefined |
| proposalType  | enum INNBCCommunityRewardPool.ProposalType | undefined |
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

### RewardClaimed

```solidity
event RewardClaimed(address indexed staker, uint256 amount)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| staker `indexed` | address | undefined |
| amount  | uint256 | undefined |

### Staked

```solidity
event Staked(address indexed staker, uint256 amount)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| staker `indexed` | address | undefined |
| amount  | uint256 | undefined |

### Unstaked

```solidity
event Unstaked(address indexed staker, uint256 amount)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| staker `indexed` | address | undefined |
| amount  | uint256 | undefined |

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



