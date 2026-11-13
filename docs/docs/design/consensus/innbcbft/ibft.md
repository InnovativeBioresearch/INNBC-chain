<!-- START ALPHA CHANGE 3386 -- replace inherited consensus assumptions with current INNBCBFT behavior -->

## Consensus Engine

INNBCBFT uses the IBFT protocol family with deterministic proposer-priority calculation, authenticated voting and BLS commit certificates.

A block height is a sequence. Multiple rounds can occur at the same height when a proposer is unavailable or the required votes do not arrive before the round timeout. A later round can reuse a previously prepared block under the protocol's certificate rules.

### Voting and Proposer Selection

Current genesis and registry validator construction assign each admitted validator voting power 1. Stake is an eligibility requirement; increasing stake does not directly increase consensus voting power.

Proposer selection is deterministic for the validator snapshot and requested round. It is not a random choice.

Quorum is calculated by the configured consensus rules. With quorum-calculation alignment enabled, the threshold is floor(2 × total voting power / 3) + 1.

### Validator Set

The consensus validator set changes through epoch processing. Genesis supplies the bootstrap set; after registry activation, eligible active validators are obtained from INNBC registry state.

Registry admission requires registration, whitelist status, sufficient stake and verified BLS possession. The legacy rootchain staking, delegation and uptime-based epoch-reward model described by older Polygon Edge documentation does not define the current workflow.

The node-provisioning numbering limit must not be interpreted as a network-wide consensus validator limit.

### Finality and Availability

A quorum-certified block is finalized under the protocol's safety assumptions. Multiple proposals or rounds at one height do not imply that multiple competing finalized heads are accepted.

Canonical storage requires each new block to extend its current head. It does not use proof-of-work total difficulty to switch chains.

Block production still requires sufficient cooperating validators and network connectivity. Round changes and catch-up mechanisms support recovery; they do not guarantee progress when quorum is unavailable.

The main repository README is the primary reference for the implemented consensus protections, operational constraints and accepted compromises.

<!-- FINISH ALPHA CHANGE 3386 -- replace inherited consensus assumptions with current INNBCBFT behavior -->
