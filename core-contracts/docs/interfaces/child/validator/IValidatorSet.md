# IValidatorSet



> IValidatorSet

Stores system-committed INNBC epoch records.

*Committee eligibility and native-INNBC staking are managed separately by INNBCValidatorRegistry.*

## Methods

### EPOCH_SIZE

```solidity
function EPOCH_SIZE() external view returns (uint256)
```

Number of blocks in an epoch.

*Each committed range must contain a multiple of this number.*


#### Returns

| Name | Type | Description |
|---|---|---|
| _0 | uint256 | undefined |

### commitEpoch

```solidity
function commitEpoch(uint256 id, Epoch epoch) external nonpayable
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| id | uint256 | undefined |
| epoch | Epoch | undefined |

### totalBlocks

```solidity
function totalBlocks(uint256 epochId) external view returns (uint256 length)
```

Total number of blocks in a committed epoch.



#### Parameters

| Name | Type | Description |
|---|---|---|
| epochId | uint256 | undefined |

#### Returns

| Name | Type | Description |
|---|---|---|
| length | uint256 | undefined |



## Events

### NewEpoch

```solidity
event NewEpoch(uint256 indexed id, uint256 indexed startBlock, uint256 indexed endBlock, bytes32 epochRoot)
```





#### Parameters

| Name | Type | Description |
|---|---|---|
| id `indexed` | uint256 | undefined |
| startBlock `indexed` | uint256 | undefined |
| endBlock `indexed` | uint256 | undefined |
| epochRoot  | bytes32 | undefined |



