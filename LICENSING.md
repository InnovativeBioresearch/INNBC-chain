# INNBC Chain Licensing

Original INNBC Chain additions and modifications:
Copyright (c) 2026 Jonathan Fior. All rights reserved, except where separately licensed or otherwise permitted by applicable law.

INNBC Chain was developed by Jonathan Fior at Innovative Bioresearch using a substantially customized Polygon Edge foundation.

Repository:
https://github.com/InnovativeBioresearch/INNBC-chain

## 1. Licensing Structure

<!-- START ALPHA CHANGE 3918 -- distinguish permitted network operation from restricted commercialization -->

This repository contains separately licensed material:

1. Retained upstream material originally licensed under Apache-2.0 remains governed by Apache-2.0.
2. Original copyrightable INNBC Chain additions and modifications identified below are subject to the permissions and conditions in section 3. Official-network operation is permitted without software licensing fees or a separate written agreement. Noncommercial derivatives are permitted subject to attribution and the other stated conditions. Restricted commercialization requires a separate written agreement.
3. Third-party dependencies and material expressly licensed separately retain their respective licenses.

<!-- FINISH ALPHA CHANGE 3918 -- distinguish permitted network operation from restricted commercialization -->

The Apache license text is reproduced unchanged in [LICENSE](LICENSE). Its inclusion preserves the license applicable to the Apache-covered portions.

## 2. Original INNBC Chain Material

The proprietary material covered by this notice consists of original copyrightable code and other original implementation material authored by Jonathan Fior for the INNBC-specific additions and modifications documented in:

[README.md — Major Enhancements Over Polygon Edge](README.md#major-enhancements-over-polygon-edge).

That section currently contains 136 numbered entries describing the project's architecture, enhancements, corrections, hardening, tooling, and operational work.

<!-- START ALPHA CHANGE 3919 -- identify original implementation and packaging material -->

Those descriptions, their source-file references, and the corresponding ALPHA CHANGE annotations help identify the INNBC-specific implementation.

The original INNBC material also includes original contributions to the installation, configuration, node-management, and package-building workflows in:

- `packaging/nodeclient/`;
- `packaging/innbc-peerdirectory-service/`;
- `packaging/innbc-snapshot-builder/`;
- `scripts/build-nodeclient.sh`;
- `scripts/build-localtestclient.sh`;
- `command/node/`;
- the associated original documentation.

These references identify locations containing original contributions. They do not claim ownership of inherited or separately licensed material within those locations.

This notice applies only to original copyrightable material actually present in the relevant distribution and owned by the stated copyright holder.

<!-- FINISH ALPHA CHANGE 3919 -- identify original implementation and packaging material -->

In particular:

- a feature description does not establish ownership of all code implementing that feature;
- an ALPHA CHANGE marker records a change but does not establish ownership of every enclosed line;
- inherited code inside a modified file retains its applicable license;
- historical descriptions of retired features do not claim that the retired implementation remains in the current distribution;
- accepted limitations and descriptions of behavior are not themselves treated as proprietary implementations;
- generated files retain any applicable rights and licensing of the material from which they were generated.

This notice concerns copyrightable expression. It does not claim exclusive rights over general technical ideas, public standards, protocol concepts, upstream techniques, public blockchain facts, or independently authored implementations.

<!-- START ALPHA CHANGE 3920 -- permit official network use and attributed noncommercial derivatives -->

## 3. Permitted Uses and Restricted Commercialization

The following permissions apply to the original INNBC material identified in section 2, to the extent that the stated copyright holder owns and can license that material.

These permissions are non-exclusive and royalty-free, subject to the conditions below. They do not replace the licenses of inherited or separately licensed components.

### 3.1 Official INNBC Chain Network Operation

Individuals and organizations may download, copy, inspect, compile, install, execute, and make modifications necessary to operate the software for participation in the official INNBC Chain network without paying a software licensing fee or obtaining a separate written agreement.

This permission includes:

- running nodes and associated peer-directory or snapshot services;
- seeking validator admission and operating as an eligible validator;
- receiving protocol rewards and transaction fees;
- operating RPC, hosting, maintenance, support, wallet, exchange, explorer, and application services for the official network;
- using the official network as part of commercial business activities;
- making operational, compatibility, security, and maintenance modifications for those permitted activities;
- sharing copies without a software licensing charge to enable those permitted activities, while preserving applicable licensing and copyright notices.

Charging for permitted hosting, support, maintenance, or network services does not by itself require a separate software license agreement. This permission does not authorize selling or licensing the protected implementation as a separate software product.

Applications that merely interact with INNBC Chain through its protocol or RPC interfaces are not treated as derivatives of this client solely because of that interaction.

The official network means the public INNBC Chain network itself, rather than an independent blockchain network created using this implementation.

Permission to use the software does not grant automatic validator or peer admission. Registration, whitelisting, staking, consensus rules, and other applicable governance and technical requirements remain in effect.

No software licensing fee does not mean that transactions, infrastructure, or network participation have no costs. Protocol fees, required stake, and operating expenses remain applicable.

### 3.2 Noncommercial Modifications and Derivatives

Permission is granted to copy, modify, compile, execute, and redistribute the original INNBC material for noncommercial research, education, experimentation, security review, and other noncommercial derivative work, subject to section 3.3.

For this permission, noncommercial means that the relevant activity is not primarily intended for or directed toward commercial advantage or monetary compensation.

The activity's actual purpose determines its classification. A free download does not automatically make a commercial product noncommercial, and an organization's nonprofit status does not automatically make every activity noncommercial.

Examples may include:

- an experimental version used to study consensus behavior;
- a research prototype comparing storage or synchronization approaches;
- a volunteer security-research fork demonstrating a correction;
- a community adaptation for another operating system;
- a noncommercial experimental test network.

These examples qualify only where the actual activity meets the stated conditions.

The official-network permissions in section 3.1 remain applicable even when the operator is a business or receives compensation for a permitted service.

Later commercialization outside section 3.1 requires the separate agreement described in section 3.4.

### 3.3 Attribution and Identification of Modifications

Recipients exercising the permissions above must preserve applicable original copyright, licensing, and attribution notices.

Distributed or publicly offered noncommercial derivatives must clearly identify their origin and preserve the following information in their README and accompanying licensing or credits documentation:

- original developer: Jonathan Fior;
- project attribution: Innovative Bioresearch;
- website: https://www.innovativebioresearch.com/;
- developer profile: https://github.com/InnovativeBioresearch;
- original source: https://github.com/InnovativeBioresearch/INNBC-chain.

A suitable attribution is:

> Based on INNBC Chain, originally developed by [Jonathan Fior](https://github.com/InnovativeBioresearch) at [Innovative Bioresearch](https://www.innovativebioresearch.com/).
>
> Original source: https://github.com/InnovativeBioresearch/INNBC-chain
>
> Modifications by: [name]. No endorsement by the original developer is implied.

Identify the relevant modifications and their dates in a reasonable manner.

Where a derivative has an associated public website or graphical interface, provide the attribution in an accessible About, Credits, or legal-notices area. A command-line-only project may provide it through its documentation or license/credits output.

No advertising banner, original logo, or credit on every page is required. Private experimentation does not require a public announcement, but applicable notices must remain with retained copies.

Distributed copies containing the original INNBC material must retain this licensing notice. Recipients must not represent that they can grant broader permissions for the original material than they actually hold.

Attribution does not imply endorsement and does not substitute for the written agreement required for restricted commercialization.

Merely using the official network or interacting with its RPC interfaces does not require an application's own interface to display client-development credits.

### 3.4 Commercialization Requiring a Written Agreement

Except where section 3.1 or another applicable permission authorizes the activity, a separate written license agreement is required to:

- create or commercialize a separate software product or commercial derivative incorporating the protected original INNBC material;
- create or operate an independent for-profit blockchain fork using that material;
- sell, license for payment, or resell software containing the protected original INNBC material;
- incorporate that material into another commercial product or service outside the permitted official-network activities.

A GitHub repository fork used for inspection or permitted development is not, merely by being a repository fork, an independent commercial blockchain network.

These restrictions apply only to the original material identified in section 2. They do not restrict rights available for inherited or separately licensed material.

The agreement must come from Jonathan Fior or an authorized representative of the relevant copyright holder.

Requests should identify the repository revision, the material concerned, and the intended use.

<!-- FINISH ALPHA CHANGE 3920 -- permit official network use and attributed noncommercial derivatives -->

## 4. Upstream and Separately Licensed Material

The proprietary terms above do not restrict the rights provided by Apache-2.0 for retained Apache-covered material.

They also do not override applicable MIT, BSD, Apache, or other third-party licenses.

Material carrying an applicable separate license declaration, including Solidity SPDX declarations and dependency license notices, retains those terms. This general notice does not silently replace those declarations.

Copyright, attribution, license, and applicable NOTICE information belonging to upstream authors and other contributors must be preserved as required by their respective licenses.

The upstream Polygon Edge and core-contracts attribution includes:

Copyright 2022 Polygon Technology.

The precise license applicable to a component must be determined from its own license declarations and accompanying notices.

## 5. Mixed Source Files

A file may contain both inherited open-source material and original INNBC additions or modifications.

The proprietary claim applies only to the original copyrightable INNBC portions falling within section 2. It does not convert the inherited portions into proprietary material or remove permissions applicable to those inherited portions.

Where ownership or licensing of a particular portion is uncertain, this document must not be read as a claim of ownership over another party's work.

## 6. Public Repository Access

The repository is published for source inspection and technical review, subject to the permissions and restrictions applicable to each component.

Public hosting remains subject to the hosting platform's terms, including any applicable rights to view and fork the repository through that service.

Those platform permissions are not represented here as a general license to use the proprietary portions outside the permissions actually granted.

The customized distribution is source available with mixed licensing. It is not represented as wholly Apache-licensed or wholly open-source software.

## 7. Existing Permissions and Separate Agreements

These terms do not revoke or narrow rights already validly granted under an applicable license or agreement.

A separate written agreement governs the material and permissions it expressly covers.

This notice does not determine the legal effect or licensing scope of an earlier publication.

<!-- START ALPHA CHANGE 3921 -- distinguish accurate attribution from branding endorsement -->

## 8. Branding

Accurate attribution and reasonable descriptive references to INNBC Chain, Jonathan Fior, and Innovative Bioresearch, including the attribution required by section 3.3, are permitted under this notice.

This permission does not authorize misleading claims of endorsement or official status, or grant permission to reuse reserved logo artwork.

INNBC, INNBC Chain, Innovative Bioresearch, and associated logos remain subject to the applicable rights of their owners. This statement does not assert that any mark is registered.

<!-- FINISH ALPHA CHANGE 3921 -- distinguish accurate attribution from branding endorsement -->

## 9. Warranty

To the extent permitted by applicable law, the proprietary material is provided without warranties or commitments of support.

Warranty and liability terms applicable to upstream or third-party material remain governed by their respective licenses. A separate written agreement may provide additional commitments for the material it covers.
