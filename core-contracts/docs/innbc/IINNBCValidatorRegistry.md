# IINNBCValidatorRegistry









## Methods

### activeValidatorCount

```solidity
function activeValidatorCount() external view returns (uint256)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### getActiveValidators

```solidity
function getActiveValidators() external view returns (address[])
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address[] | undefined |

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

### removeValidator

```solidity
function removeValidator(address operator) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator | address | undefined |

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

### validators

```solidity
function validators(address operator) external view returns (address validatorOperator, address rewardAddress, bytes blsKey, string libp2pPeerID, uint256 stake, bool registered, bool whitelisted)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator | address | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| validatorOperator | address | undefined |
| rewardAddress | address | undefined |
| blsKey | bytes | undefined |
| libp2pPeerID | string | undefined |
| stake | uint256 | undefined |
| registered | bool | undefined |
| whitelisted | bool | undefined |

### whitelistValidator

```solidity
function whitelistValidator(address operator) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| operator | address | undefined |




