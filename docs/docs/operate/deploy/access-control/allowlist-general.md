<!-- START ALPHA CHANGE 1036 -- remove legacy bridge ACL examples from access-control docs -->

## Understanding ACL And Address Roles

Access control lists manage address-level permissions in INNBC Chain.

Current INNBC access-control docs cover contract deployer and transaction allow/block lists. The removed legacy bridge allow/block lists are not part of the current INNBC runtime.

## Rules

- Allowlists and blocklists are configured at network initialization through genesis.
- Admin addresses are configured at genesis.
- For a given list type, do not enable both an allowlist and a blocklist.
- The system transaction address is excluded from allowlist and blocklist validation.
- Runtime behavior depends on the specific list type being enforced.

## ABI Shape

```shell
function setAdmin(address)
function setEnabled(address)
function setNone(address)
function readAddressList(address) returns (uint256)
```

<!-- START ALPHA CHANGE 3405 -- document list-dependent ACL role effects -->
Roles:

| Role | Allowlist effect | Blocklist effect | Administration |
| --- | --- | --- | --- |
| `NoRole` | Does not satisfy the allowlist | Not blocked by this list | Cannot update roles |
| `EnabledRole` | Satisfies the allowlist | Blocked by this list | Cannot update roles |
| `AdminRole` | Satisfies the allowlist | Not blocked by this list | Can update other accounts' roles |

Calling `setEnabled` on a blocklist blocks the selected address. It does not grant access.

These are ACL outcomes, not guarantees that every other transaction or execution check will pass.
<!-- FINISH ALPHA CHANGE 3405 -- document list-dependent ACL role effects -->

## Supported ACL Types

<!-- START ALPHA CHANGE 3405 -- distinguish enabled and administrator admission from blocklist membership -->
| ACL | Purpose |
| --- | --- |
| Contract deployer allowlist | Permits deployment by addresses with `EnabledRole` or `AdminRole`. |
| Contract deployer blocklist | Rejects deployment by addresses with `EnabledRole`. |
| Transactions allowlist | Permits the controlled transaction execution by callers with `EnabledRole` or `AdminRole`. |
| Transactions blocklist | Rejects the controlled transaction execution by callers with `EnabledRole`. |
<!-- FINISH ALPHA CHANGE 3405 -- distinguish enabled and administrator admission from blocklist membership -->

## ethers.js Example

<!-- START ALPHA CHANGE 3405 -- sign administrative ACL transactions locally with an authorized wallet -->

This ethers v5 example requires:

- `INNBC_RPC_URL`: the node's JSON-RPC URL.
- `INNBC_ACL_ADMIN_PRIVATE_KEY`: an administrator signing key, supplied through a protected environment.
- `INNBC_DEPLOYER_ALLOWLIST_ADDRESS`: the contract-deployer allowlist address.
- `INNBC_TX_ALLOWLIST_ADDRESS`: the transaction allowlist address.
- `INNBC_ACL_ACCOUNT_A` and `INNBC_ACL_ACCOUNT_B`: addresses used by the example.

The signer must hold `AdminRole` on both target lists. The example enables and then removes each selected account. It signs locally and does not depend on node-managed accounts.

```javascript
const ethers = require('ethers');

async function main() {
  const rpcURL = process.env.INNBC_RPC_URL;
  const adminKey = process.env.INNBC_ACL_ADMIN_PRIVATE_KEY;
  const deployerListAddress = process.env.INNBC_DEPLOYER_ALLOWLIST_ADDRESS;
  const transactionListAddress = process.env.INNBC_TX_ALLOWLIST_ADDRESS;
  const addrAlice = process.env.INNBC_ACL_ACCOUNT_A;
  const addrBob = process.env.INNBC_ACL_ACCOUNT_B;

  if (!rpcURL || !adminKey) {
    throw new Error('RPC URL and administrator key are required');
  }

  for (const address of [
    deployerListAddress, transactionListAddress, addrAlice, addrBob
  ]) {
    if (!address || !ethers.utils.isAddress(address)) {
      throw new Error('Valid ACL and account addresses are required');
    }
  }

  const provider = new ethers.providers.JsonRpcProvider(rpcURL);
  const signer = new ethers.Wallet(adminKey, provider);

  const ACLInterface = [
    'function setEnabled(address)',
    'function setNone(address)',
    'function readAddressList(address) view returns (uint256)'
  ];

  const contractDeployerAllowList = new ethers.Contract(
    deployerListAddress, ACLInterface, signer
  );
  const transactionsAllowList = new ethers.Contract(
    transactionListAddress, ACLInterface, signer
  );

  for (const list of [contractDeployerAllowList, transactionsAllowList]) {
    const role = await list.readAddressList(signer.address);
    if (!role.eq(2)) {
      throw new Error('Signer must be an administrator on both lists');
    }
  }

  await (await contractDeployerAllowList.setEnabled(addrAlice)).wait();
  await (await contractDeployerAllowList.setNone(addrAlice)).wait();

  await (await transactionsAllowList.setEnabled(addrBob)).wait();
  await (await transactionsAllowList.setNone(addrBob)).wait();
}

main().catch(() => {
  console.error('ACL example failed; verify configuration, permissions and RPC access.');
  process.exitCode = 1;
});
```

Only an account with the administrator role for the target list can change membership. Do not print or commit the administrator private key.

<!-- FINISH ALPHA CHANGE 3405 -- sign administrative ACL transactions locally with an authorized wallet -->

<!-- FINISH ALPHA CHANGE 1036 -- remove legacy bridge ACL examples from access-control docs -->