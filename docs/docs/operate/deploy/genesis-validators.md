<!-- START ALPHA CHANGE 807 -- rewrite validator setup docs for INNBC registry workflow -->
This section explains how to prepare validator identities and collect the public information needed for INNBC validator registry registration.

The current INNBC validator flow does not use the removed legacy rootchain, SupernetManager, StakeManager, bridge, or child-token registration commands.

## Validator Identity Files

Each validator node has its own data directory. A validator identity contains three private key files:

<!-- START ALPHA CHANGE 1641 -- document independently stored validator identities -->
```text
keys/validator-ecdsa/validator.key
keys/validator-bls/validator-bls.key
keys/libp2p/libp2p.key
```

The keys have different roles:

```text
validator.key      ECDSA validator/operator identity
validator-bls.key  BLS consensus-signing identity
libp2p.key         authenticated network peer identity
```
<!-- FINISH ALPHA CHANGE 1641 -- document independently stored validator identities -->

Do not expose these files publicly.

## Prepare Empty Folders

Use `prepare` when you already have key files and want the software to create the folder structure without generating new keys.

```bash
sudo innbc-chain node prepare
```

For multiple validators:

```bash
sudo innbc-chain node prepare 10 --mode private-local
```

This creates identity folders such as:

<!-- START ALPHA CHANGE 1642 -- show canonical prepared identity folders -->
```text
/var/lib/innbc/node1/keys/validator-ecdsa/
/var/lib/innbc/node1/keys/validator-bls/
/var/lib/innbc/node1/keys/libp2p/
```

After preparing the folder, copy the key files into:

```text
/var/lib/innbc/node1/keys/validator-ecdsa/validator.key
/var/lib/innbc/node1/keys/validator-bls/validator-bls.key
/var/lib/innbc/node1/keys/libp2p/libp2p.key
```
<!-- FINISH ALPHA CHANGE 1642 -- show canonical prepared identity folders -->

## Create New Keys

Use `create` when you want the software to generate new validator key files.

```bash
sudo innbc-chain node create
```

For multiple validators:

```bash
sudo innbc-chain node create 10 --mode private-local
```

<!-- START ALPHA CHANGE 3374 -- document bounded allocation and read-only registration inspection -->
Each creation invocation accepts one to ten new identities. Allocation selects available numbers within node1–node100 and fills gaps. Either an existing identity path or an existing environment path reserves its number.

The complete requested allocation is checked before writing, and occupied destinations are not overwritten. A later execution failure can leave completed nodes and an incomplete current slot.

## Validate and Print Public Registration Information

After creation, or after installing all keys into a prepared folder, inspect one node:

```bash
sudo innbc-chain node info node1
```

Inspect all discovered slots:

```bash
sudo innbc-chain node info all
```

Add `--json` for structured output. For another installation, pass both `--base-dir` and `--config-dir`.

The command validates stored keys and independently checks their environment files. Its public identity information includes:

```text
operator address
BLS public key
operator-bound BLS proof
libp2p peer ID
private-local multiaddr
external-validator multiaddr template
```

Private keys are read internally but are not printed. Inspection does not initialize storage, repair partial identities or regenerate keys.

An invalid occupied slot causes a nonzero exit after reporting. Completely absent interior slots produce warnings, without inventing missing nodes beyond the highest discovered identity or environment path.

Compare the reported identity against the intended genesis or registry identity. Inspection does not perform that external comparison automatically.

See the [inspection reference](../../node-env/README.md#read-only-node-inspection) for validation scope and limitations.
<!-- FINISH ALPHA CHANGE 3374 -- document bounded allocation and read-only registration inspection -->

## Registry Inputs

<!-- START ALPHA CHANGE 3374 -- include the operator-bound proof in documented registry inputs -->
The INNBC validator registry registration uses:

```text
rewardAddress
blsKey
blsProof
libp2pPeerID
stakeAmount
```

`node info` provides the public values derived from the validator identity:

```text
operator address
blsKey
blsProof
libp2pPeerID
```

The public `blsProof` binds the exact BLS public key to the operator address.
<!-- FINISH ALPHA CHANGE 3374 -- include the operator-bound proof in documented registry inputs -->

The `rewardAddress` is a manual input. It does not have to be derived from the node keys.

The `stakeAmount` is a manual input and must satisfy the current validator registry minimum stake.

## Private-Local Multiaddr

For local testing or controlled same-host operation, the private-local multiaddr uses loopback networking:

```text
/ip4/127.0.0.1/tcp/<port>/p2p/<peer-id>
```

## External-Validator Multiaddr

For validators running across different hosts, use the external-validator template printed by `node info` and replace `<server-ip>` with the server address that other validators should dial:

```text
/ip4/<server-ip>/tcp/<port>/p2p/<peer-id>
```

Firewall and network exposure are server configuration concerns. The node software can bind to private-local or external-validator addresses, but the server firewall decides what is reachable from outside.
<!-- FINISH ALPHA CHANGE 807 -- rewrite validator setup docs for INNBC registry workflow -->