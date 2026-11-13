# INNBCProxyAdminExecutor









## Methods

### executeUpgrade

```solidity
function executeUpgrade(address proxy, address implementation, bytes data) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proxy | address | undefined |
| implementation | address | undefined |
| data | bytes | undefined |

### governance

```solidity
function governance() external view returns (address)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | address | undefined |

### initialize

```solidity
function initialize(address governance_) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| governance_ | address | undefined |

### initialized

```solidity
function initialized() external view returns (bool)
```






#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | bool | undefined |



## Events

### GovernanceInitialized

```solidity
event GovernanceInitialized(address indexed governance)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| governance `indexed` | address | undefined |

### UpgradeExecuted

```solidity
event UpgradeExecuted(address indexed proxy, address indexed implementation)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| proxy `indexed` | address | undefined |
| implementation `indexed` | address | undefined |



