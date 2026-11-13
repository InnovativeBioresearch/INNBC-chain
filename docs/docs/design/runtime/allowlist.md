## Overview

<!-- START ALPHA CHANGE 1043 -- rename ACL overview for INNBC -->
INNBC Chain supports access control lists (ACLs) to manage and control access to specific network resources, contracts, or functionality. Network operators can limit access to certain addresses using ACLs, ensuring a controlled and secure environment for their applications.
<!-- FINISH ALPHA CHANGE 1043 -- rename ACL overview for INNBC -->

## Roles

<!-- START ALPHA CHANGE 3404 -- distinguish stored ACL roles from allowlist and blocklist policy -->
The ACL stores three role values. Their effect depends on the list being enforced:

| Role | Allowlist check | Blocklist check | Can manage roles |
| --- | --- | --- | --- |
| `NoRole` | Rejected | Not blocked by this list | No |
| `EnabledRole` | Accepted | Blocked | No |
| `AdminRole` | Accepted | Not blocked by this list | Yes |

`EnabledRole` means membership in the selected list. It grants access on an allowlist and denies access on a blocklist.

These results describe the ACL check only; other transaction and execution requirements still apply. An administrator can modify other accounts' roles but cannot remove its own administrator role through the same caller.
<!-- FINISH ALPHA CHANGE 3404 -- distinguish stored ACL roles from allowlist and blocklist policy -->

## Initializing and Managing Admins in ACLs

To effectively manage ACLs, at least one address with the Admin role should be assigned when initializing each list in your blockchain application. Admins have the authority to modify the roles of other addresses, granting or revoking access to resources or functionalities as needed.

When setting up ACLs, you should:

- Select trusted addresses to serve as admins.
- Assign the `Admin` role to these addresses using the appropriate flag for each list type.
- Admin addresses will manage the respective ACLs, including adding or removing new addresses. Ensure that the admin addresses are secure and that only authorized users have access to them to maintain the integrity and security of your application.

## Designating Multiple Admins

You can designate multiple admin addresses for redundancy and better access control management. Having more than one admin can help:

- Prevent single points of failure.
- Distribute the responsibility of managing the ACLs among multiple trusted entities.

However, assigning multiple admins also increases the potential attack surface. It is essential to strike the right balance between security and flexibility when managing admin addresses or use your best judgment based on the needs and purpose of the network.

## ACL Types

INNBC Chain supports several types of ACLs:

- **Contract Deployer Allow/Block Lists**: Controls which can deploy contracts on the network.
- **Transactions Allow/Block Lists**: Controls which addresses can send transactions on the network.
<!-- START ALPHA CHANGE 803 -- remove legacy bridge ACL type from runtime docs -->
<!-- FINISH ALPHA CHANGE 803 -- remove legacy bridge ACL type from runtime docs -->

Each list type serves a different purpose and can be configured separately to provide fine-grained control over various aspects of the network. Network operators can use a combination of these lists to maintain a secure and controlled environment for their applications.

<!-- START ALPHA CHANGE 804 -- remove legacy bridge ACL section from runtime docs -->
<!-- FINISH ALPHA CHANGE 804 -- remove legacy bridge ACL section from runtime docs -->

## Current Limitations

- <!-- START ALPHA CHANGE 3404 -- identify all implemented ACL role values -->**Fixed role definitions**: The implementation provides `NoRole`, `EnabledRole` and `AdminRole`. List policy determines whether membership permits or blocks an action. Additional role values require an implementation change.<!-- FINISH ALPHA CHANGE 3404 -- identify all implemented ACL role values -->
- **Granularity**: The current implementation applies the ACLs at the network level. More granular control may require custom implementation, such as using ACLs for specific contracts or functionalities.
- **Static call limitation**: Write operations are not allowed in static calls, which might be a constraint in specific scenarios where you would want to perform a write operation within a static call.
