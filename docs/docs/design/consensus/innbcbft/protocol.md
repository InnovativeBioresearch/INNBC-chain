<!-- START ALPHA CHANGE 1033 -- replace legacy bridge protocol docs with current INNBCBFT protocol docs -->

## State Transitions

INNBC BFT reaches consensus through repeated block-height state transitions.

At each height, one validator proposes a block. The active validator set votes on the proposal. If the required voting threshold is reached, the block is sealed and appended to the chain.

The protocol refers to the current block height as the sequence.

## Rounds

A block may be finalized in round 0 when the network is healthy and validators agree quickly.

If the validator set does not reach agreement in the current round, the protocol advances to the next round and another proposer may attempt to seal the same height.

<!-- START ALPHA CHANGE 3387 -- remove local-clock drift as a block timestamp validity condition -->
Repeated high-round sealing warrants investigation of proposer availability, connectivity, processing delays and consensus timeouts. Header timestamp validity uses the strict parent-derived chain clock and does not compare against a validator's local wall clock.
<!-- FINISH ALPHA CHANGE 3387 -- remove local-clock drift as a block timestamp validity condition -->

## Validator Set

INNBC BFT uses an epoch-based validator set.

<!-- START ALPHA CHANGE 2046 -- document admission-time protection of aggregate BLS verification -->
The active validator set is rebuilt from INNBC-native validator registry state. Validators are identified by ECDSA address, BLS public key, and LibP2P peer identity.

Every genesis and registry validator key must have a valid proof of possession. The proof signs the 20-byte operator address followed by the canonical 128-byte public key, under the Keccak-256 digest of `DOMAIN_INNBC_BLS_PROOF_OF_POSSESSION`.

Genesis proofs are checked before consensus initialization, and the proved set must match canonical genesis header extra data. Registry proofs are checked during registration and BLS-key replacement. The verification flag is stored in the validator record and checked at activation and registry-set construction.

Proofs are not reverified during ordinary commit or aggregate-certificate verification. This preserves the existing same-message aggregate verification path while preventing unproved keys from entering its admitted validator sets.
<!-- FINISH ALPHA CHANGE 2046 -- document admission-time protection of aggregate BLS verification -->

At epoch boundaries, the consensus runtime refreshes validator state and proposer calculation input.

## Staking And Validator Admission

Validator admission is handled by INNBC-native contracts.

A validator must satisfy registry requirements, including the configured minimum stake and governance-controlled whitelist status, before becoming active.

The current INNBC protocol does not use the removed legacy Polygon rootchain, bridge, StakeManager, SupernetManager, checkpoint, or burn-contract flow.

## Slashing

Slashing mechanics are not currently described as a finalized public operator workflow in this document.

Potential malicious behavior includes double-signing, equivocation, or other behavior that attempts to violate consensus safety.

<!-- START ALPHA CHANGE 3329 -- replace retired timestamp drift documentation with the strict chain clock -->

## Chain-Clock Timestamp Validation

For every non-genesis block, INNBC BFT requires:

```text
timestamp = parent timestamp + configured block interval
```

Genesis supplies the initial timestamp. The block interval is configured through `blockTime`, must be at least one second, and must be an exact whole number of seconds.

Block construction and validation use the same checked calculation. Timestamp addition overflow and values outside the execution context's supported signed 64-bit range are rejected.

The invariant applies to locally constructed proposals, incoming proposals before execution, and finalized-header verification. Timestamp validity does not consult the validator's local clock.

The former future-time allowance and `blockTimeDrift` configuration have been retired. Explicit use of the retired field is rejected, including zero and null values.

INNBC timestamps measure chain progression. Production delays and outages do not advance the chain clock. Timestamp-dependent contract deadlines follow this clock rather than elapsed real-world time.

Production pacing, consensus timeouts and network timers remain operational timers; they do not select block timestamps or determine timestamp validity.

<!-- FINISH ALPHA CHANGE 3329 -- replace retired timestamp drift documentation with the strict chain clock -->

<!-- FINISH ALPHA CHANGE 1033 -- replace legacy bridge protocol docs with current INNBCBFT protocol docs -->