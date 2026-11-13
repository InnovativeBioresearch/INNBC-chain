# INNBCTreasuryDAO









## Methods

### availableTreasuryBalance

```solidity
function availableTreasuryBalance() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### createBurnProposal

```solidity
function createBurnProposal(uint256 amount) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| amount | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createERC20TransferProposal

```solidity
function createERC20TransferProposal(address token, address target, uint256 amount) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| token | address | undefined |
| target | address | undefined |
| amount | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createLockProposal

```solidity
function createLockProposal(uint256 amount) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| amount | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createTransferProposal

```solidity
function createTransferProposal(address target, uint256 amount) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| target | address | undefined |
| amount | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalID | uint256 | undefined |

### createUnlockProposal

```solidity
function createUnlockProposal(uint256 amount) external nonpayable returns (uint256 proposalID)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| amount | uint256 | undefined |

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
function initialize(address validatorRegistry_, address nativeToken_) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| validatorRegistry_ | address | undefined |
| nativeToken_ | address | undefined |

### initialized

```solidity
function initialized() external view returns (bool)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | bool | undefined |

### lockedTreasuryBalance

```solidity
function lockedTreasuryBalance() external view returns (uint256)
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
function proposals(uint256) external view returns (enum INNBCTreasuryDAO.ProposalType proposalType, address target, address token, uint256 amount, uint256 yesVotes, uint256 noVotes, uint256 deadlineBlock, bool finalized, bool passed)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| proposalType | enum INNBCTreasuryDAO.ProposalType | undefined |
| target | address | undefined |
| token | address | undefined |
| amount | uint256 | undefined |
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
event ProposalCreated(uint256 indexed proposalID, enum INNBCTreasuryDAO.ProposalType proposalType, address indexed target, uint256 amount, uint256 deadlineBlock)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proposalID `indexed` | uint256 | undefined |
| proposalType  | enum INNBCTreasuryDAO.ProposalType | undefined |
| target `indexed` | address | undefined |
| amount  | uint256 | undefined |
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

### TreasuryBurn

```solidity
event TreasuryBurn(uint256 amount)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| amount  | uint256 | undefined |

### TreasuryDeposit

```solidity
event TreasuryDeposit(address indexed sender, uint256 amount)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| sender `indexed` | address | undefined |
| amount  | uint256 | undefined |

### TreasuryERC20Transfer

```solidity
event TreasuryERC20Transfer(address indexed token, address indexed target, uint256 amount)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| token `indexed` | address | undefined |
| target `indexed` | address | undefined |
| amount  | uint256 | undefined |

### TreasuryLocked

```solidity
event TreasuryLocked(uint256 amount, uint256 lockedTreasuryBalance)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| amount  | uint256 | undefined |
| lockedTreasuryBalance  | uint256 | undefined |

### TreasuryTransfer

```solidity
event TreasuryTransfer(address indexed target, uint256 amount)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| target `indexed` | address | undefined |
| amount  | uint256 | undefined |

### TreasuryUnlocked

```solidity
event TreasuryUnlocked(uint256 amount, uint256 lockedTreasuryBalance)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| amount  | uint256 | undefined |
| lockedTreasuryBalance  | uint256 | undefined |

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



