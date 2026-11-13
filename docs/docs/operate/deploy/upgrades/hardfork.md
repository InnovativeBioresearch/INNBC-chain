<!-- START ALPHA CHANGE 1047 -- replace stale Edge hardfork guide with INNBC upgrade guidance -->

# Upgrade Using A Hardfork

A hardfork introduces new consensus, execution, or chain-configuration behavior at a specific block height.

For INNBC Chain, hardfork planning should be treated as a coordinated network upgrade. Validators must run compatible software before the activation block is reached.

## When A Hardfork Is Needed

A hardfork may be required for changes such as:

| Change Type | Example |
| --- | --- |
| Consensus rules | Header validation, timestamp validation, proposer selection, validator-set transitions. |
| Execution rules | EVM behavior, gas accounting, opcode support, transaction validation. |
| System contracts | Validator registry, governance, native token, or validator-set contract behavior. |
| Chain parameters | Fork activation blocks or fork-specific runtime parameters. |

## Implementation Shape

A typical hardfork change includes:

1. Add or update the fork name in chain configuration code.
2. Define the activation block.
3. Add fork-aware runtime behavior.
4. Add tests before and after the activation block.
5. Build and distribute the updated binary.
6. Coordinate validator upgrade timing.
7. Monitor the activation block and post-fork behavior.

## Genesis And Fork Configuration

Fork activation data is written into the chain configuration.

For a chain that has not launched publicly, regenerating genesis is acceptable when intentional.

For a public chain, changing genesis is not a routine upgrade. Public-chain upgrades require a coordinated fork plan and validator rollout.

## Validator Rollout

Before the activation block:

```text
all validators should run the upgraded binary
```

After the activation block:

```text
all validators should enforce the same fork rules
```

If validators run incompatible fork logic, consensus can halt or split.

## Verification

A hardfork change should be verified with:

```bash
go test ./...
go vet ./...
```

Then test a local chain across the activation block and confirm that all validators continue producing blocks with matching state.

<!-- FINISH ALPHA CHANGE 1047 -- replace stale Edge hardfork guide with INNBC upgrade guidance -->