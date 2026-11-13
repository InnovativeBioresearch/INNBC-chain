<!-- START ALPHA CHANGE 1032 -- replace legacy bridge INNBCBFT overview with current INNBC consensus overview -->

## Overview

INNBC BFT is the consensus mechanism used by INNBC Chain.

It is based on the Istanbul Byzantine Fault Tolerance protocol family and provides fast finality through validator voting, proposer rotation, and epoch-based validator-set updates.

The current INNBC BFT runtime does not use the removed legacy bridge, rootchain, StakeManager, SupernetManager, checkpoint, or burn-contract flow.

## Consensus Engine

The consensus engine is responsible for:

| Area | Purpose |
| --- | --- |
| Proposal | Selecting the validator that proposes the next block. |
| Voting | Collecting validator votes for the proposed block. |
| Finality | Sealing one finalized block at each height. |
| Epoch handling | Rebuilding validator-set state at epoch boundaries. |
| Validator registry integration | Reading active validator state from INNBC-native contracts. |

## Validator Set

Validators are tracked by the INNBC validator registry and validator governance contracts.

Each validator has:

| Field | Purpose |
| --- | --- |
| ECDSA address | Validator account and signer identity. |
| BLS public key | Consensus signing identity. |
| LibP2P peer ID | Network identity for peer connectivity. |
| Stake | Native-token stake locked for validator eligibility. |
| Active status | Whether the validator is part of the active validator set. |
| Whitelist status | Whether governance has approved validator participation. |

## Network Discovery

Validator peer exchange is handled by the INNBC peer-directory service.

The current flow does not use legacy bootnodes. Validators publish signed peer information to the peer directory and discover other registered validators through the peer-directory API.

## Finality Model

INNBC BFT provides immediate finality once a block is sealed. A finalized block is not expected to be reorganized under normal consensus assumptions unless the validator set violates Byzantine fault tolerance assumptions.

<!-- FINISH ALPHA CHANGE 1032 -- replace legacy bridge INNBCBFT overview with current INNBC consensus overview -->