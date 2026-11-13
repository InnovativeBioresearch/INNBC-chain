<!-- START ALPHA CHANGE 3430 -- document ValidatorSet after VSET accounting retirement -->

# ValidatorSet Epoch Interface

This page describes `core-contracts/contracts/interfaces/child/validator/IValidatorSet.sol` and its implementation in `core-contracts/contracts/child/validator/ValidatorSet.sol`.

ValidatorSet stores system-committed epoch records. Committee eligibility and native-INNBC staking are managed separately by `INNBCValidatorRegistry`.

## Epoch Records

`Epoch` contains:

- `startBlock`.
- `endBlock`.
- `epochRoot`.

The implementation exposes `currentEpochId()`, `epochs(uint256)` and `epochEndBlocks(uint256)`.

## Initialization

The implementation uses `initialize(uint256 newEpochSize)`.

<!-- START ALPHA CHANGE 3451 -- document the minimum supported initializer epoch size -->

Initialization requires the system caller, accepts an epoch size of at least two blocks and runs once per storage context. Zero and one-block epoch sizes revert with `INVALID_INPUT`. Genesis initializes the proxy through the client genesis hook.

This minimum matches the shared Go configuration validation. The additional initializer check preserves the existing function signature and storage layout.

<!-- FINISH ALPHA CHANGE 3451 -- document the minimum supported initializer epoch size -->

The implementation constructor disables initialization for conventional deployments. Genesis installs runtime bytecode directly, so constructor execution is not its protection; the system-caller restriction also applies to those allocations.

## Epoch Commitments

- `commitEpoch(uint256, Epoch)` requires the system caller.
- Epoch identifiers must follow the current epoch.
- Committed ranges must preserve the existing range, divisibility and continuity checks.
- Successful commitments emit `NewEpoch`.
- `EPOCH_SIZE()` exposes the configured epoch size.
- `totalBlocks(uint256)` returns the number of blocks in a committed epoch.

## Retired Accounting

ValidatorSet no longer exposes VSET balances, allowances, token snapshots, minting, burning, unstaking or withdrawal queues. The `ValidatorInit` initializer argument and genesis validator `"stake"` field are retired.

This does not remove the separate registry's native-INNBC staking, minimum-stake requirement or committee eligibility checks.

The storage layout and initialization ABI changed before launch. Generate fresh genesis state from the updated artifacts; this change does not provide an upgrade procedure for old development state.

Use the main repository README for the current validator-registry, staking and consensus model.

<!-- FINISH ALPHA CHANGE 3430 -- document ValidatorSet after VSET accounting retirement -->
