# INNBCValidatorGovernance









## Methods

### EMERGENCY_REMOVE_PROPOSAL_DURATION_BLOCKS

```solidity
function EMERGENCY_REMOVE_PROPOSAL_DURATION_BLOCKS() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

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

### createEmergencyRemoveProposal

```solidity
function createEmergencyRemoveProposal(address target) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| target | address | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createRemoveProposal

```solidity
function createRemoveProposal(address target) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| target | address | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUpdateMinimumStakeProposal

```solidity
function createUpdateMinimumStakeProposal(uint256 newMinimumStake) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| newMinimumStake | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseLongProposalDurationProposal

```solidity
function createUseLongProposalDurationProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseShortProposalDurationProposal

```solidity
function createUseShortProposalDurationProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUseStandardProposalDurationProposal

```solidity
function createUseStandardProposalDurationProposal() external nonpayable returns (uint256 proposalID)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createWhitelistProposal

```solidity
function createWhitelistProposal(address target) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| target | address | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

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
function initialize(address validatorRegistry_, enum INNBCValidatorGovernance.ProposalDurationPreset proposalDurationPreset_) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| validatorRegistry_ | address | undefined |
| proposalDurationPreset_ | enum INNBCValidatorGovernance.ProposalDurationPreset | undefined |

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
function proposalDurationPreset() external view returns (enum INNBCValidatorGovernance.ProposalDurationPreset)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | enum INNBCValidatorGovernance.ProposalDurationPreset | undefined |

### proposals

```solidity
function proposals(uint256) external view returns (enum INNBCValidatorGovernance.ProposalType proposalType, address target, uint256 newMinimumStake, enum INNBCValidatorGovernance.ProposalDurationPreset newProposalDurationPreset, uint256 yesVotes, uint256 noVotes, uint256 deadlineBlock, bool finalized, bool passed)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalType | enum INNBCValidatorGovernance.ProposalType | undefined |
| target | address | undefined |
| newMinimumStake | uint256 | undefined |
| newProposalDurationPreset | enum INNBCValidatorGovernance.ProposalDurationPreset | undefined |
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

### ProposalCreated

```solidity
event ProposalCreated(uint256 indexed proposalID, enum INNBCValidatorGovernance.ProposalType proposalType, address indexed target, uint256 deadlineBlock)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID `indexed` | uint256 | undefined |
| proposalType  | enum INNBCValidatorGovernance.ProposalType | undefined |
| target `indexed` | address | undefined |
| deadlineBlock  | uint256 | undefined |

### ProposalDurationUpdated

```solidity
event ProposalDurationUpdated(enum INNBCValidatorGovernance.ProposalDurationPreset proposalDurationPreset, uint256 proposalDurationBlocks)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalDurationPreset  | enum INNBCValidatorGovernance.ProposalDurationPreset | undefined |
| proposalDurationBlocks  | uint256 | undefined |

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



