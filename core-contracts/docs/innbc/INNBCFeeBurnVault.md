# INNBCFeeBurnVault









## Methods

### burn

```solidity
function burn() external nonpayable
```






### initialize

```solidity
function initialize(address nativeToken_) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| nativeToken_ | address | undefined |

### initialized

```solidity
function initialized() external view returns (bool)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | bool | undefined |

### nativeToken

```solidity
function nativeToken() external view returns (address)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |



## Events

### FeeBurnVaultDeposit

```solidity
event FeeBurnVaultDeposit(address indexed sender, uint256 amount)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| sender `indexed` | address | undefined |
| amount  | uint256 | undefined |

### FeesBurned

```solidity
event FeesBurned(address indexed caller, uint256 amount)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| caller `indexed` | address | undefined |
| amount  | uint256 | undefined |



