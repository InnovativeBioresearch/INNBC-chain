# INNBCProxyAdminGovernance









## Methods

### createUpgradeProposal

```solidity
function createUpgradeProposal(address proxy, address implementation, bytes data) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proxy | address | undefined |
| implementation | address | undefined |
| data | bytes | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### executeUpgrade

```solidity
function executeUpgrade(uint256 proposalID) external nonpayable
```





#### Parameters

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
function initialize(address validatorRegistry_, address proxyAdminExecutor_) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| validatorRegistry_ | address | undefined |
| proxyAdminExecutor_ | address | undefined |

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

### proposals

```solidity
function proposals(uint256) external view returns (address proxy, address implementation, bytes data, uint256 yesVotes, uint256 noVotes, uint256 deadlineBlock, bool finalized, bool passed, bool executed)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proxy | address | undefined |
| implementation | address | undefined |
| data | bytes | undefined |
| yesVotes | uint256 | undefined |
| noVotes | uint256 | undefined |
| deadlineBlock | uint256 | undefined |
| finalized | bool | undefined |
| passed | bool | undefined |
| executed | bool | undefined |

### proxyAdminExecutor

```solidity
function proxyAdminExecutor() external view returns (address)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

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

### ProposalFinalized

```solidity
event ProposalFinalized(uint256 indexed proposalID, bool passed)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID `indexed` | uint256 | undefined |
| passed  | bool | undefined |

### UpgradeExecuted

```solidity
event UpgradeExecuted(uint256 indexed proposalID, address indexed proxy, address indexed implementation)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID `indexed` | uint256 | undefined |
| proxy `indexed` | address | undefined |
| implementation `indexed` | address | undefined |

### UpgradeProposalCreated

```solidity
event UpgradeProposalCreated(uint256 indexed proposalID, address indexed proxy, address indexed implementation, uint256 deadlineBlock)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID `indexed` | uint256 | undefined |
| proxy `indexed` | address | undefined |
| implementation `indexed` | address | undefined |
| deadlineBlock  | uint256 | undefined |

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



