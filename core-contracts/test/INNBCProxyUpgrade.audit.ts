/* START ALPHA CHANGE 2403 -- test governed upgrades through the actual transparent proxy -- */
import { expect } from "chai";
import { ethers } from "hardhat";
import {
  REGISTRY_PROXY,
  ZERO,
  approve,
  deploy,
  installRuntime,
  mined,
  proposal,
  registryFixture,
  resetAuditChain,
} from "./helpers/innbcAudit";

async function upgradeFixture() {
  const f = await registryFixture();
  const governance = await deploy("INNBCProxyAdminGovernance");
  const executor = await deploy("INNBCProxyAdminExecutor");
  const oldImplementation = await deploy("INNBCAuditUpgradeV1");
  const newImplementation = await deploy("INNBCAuditUpgradeV2");

  await mined(executor.initialize(governance.address));
  await mined(governance.initialize(f.registry.address, executor.address));

  const proxy = await installRuntime("GenesisProxy", REGISTRY_PROXY);
  await mined(proxy.protectSetUpProxy(f.validators[0].address));
  await mined(
    proxy.setUpProxy(
      oldImplementation.address,
      executor.address,
      oldImplementation.interface.encodeFunctionData("setValue", [7])
    )
  );

  const target = await ethers.getContractAt("INNBCAuditUpgradeV2", REGISTRY_PROXY);

  return {
    ...f,
    upgradeGovernance: governance,
    executor,
    proxy,
    target,
    oldImplementation,
    newImplementation,
  };
}

describe("INNBC audit: proxy upgrade boundaries", function () {
  beforeEach(resetAuditChain);

  /* START ALPHA CHANGE 3778 -- reject the retired Merkle target in both upgrade controllers -- */
  it("rejects the retired Merkle proxy in governance and executor", async function () {
    const f = await upgradeFixture();
    const retiredProxy = ethers.utils.getAddress(
      ethers.utils.hexZeroPad("0x0103", 20)
    );

    const nextID = await f.upgradeGovernance.nextProposalID();

    await expect(
      f.upgradeGovernance.createUpgradeProposal(
        retiredProxy, f.newImplementation.address, "0x"
      )
    ).to.be.revertedWith("INNBC Proxy Admin: proxy not allowed");

    expect(await f.upgradeGovernance.nextProposalID()).to.equal(nextID);

    // Authorize a test signer to exercise the executor's target check
    // independently of the governance contract's earlier rejection.
    const isolatedExecutor = await deploy("INNBCProxyAdminExecutor");
    await mined(isolatedExecutor.initialize(f.validators[0].address));

    await expect(
      isolatedExecutor.connect(f.validators[0]).executeUpgrade(
        retiredProxy, f.newImplementation.address, "0x"
      )
    ).to.be.revertedWith("INNBC Proxy Admin Executor: proxy not allowed");

    expect(await f.target.version()).to.equal(1);
    expect(await f.target.value()).to.equal(7);
  });
  /* FINISH ALPHA CHANGE 3778 -- reject the retired Merkle target in both upgrade controllers -- */

  it("rejects unauthorized execution and invalid upgrade targets", async function () {
    const f = await upgradeFixture();

    await expect(
      f.executor.connect(f.outsider).executeUpgrade(
        REGISTRY_PROXY, f.newImplementation.address, "0x"
      )
    ).to.be.revertedWith("INNBC Proxy Admin Executor: sender not governance");

    await expect(
      f.upgradeGovernance.createUpgradeProposal(
        f.outsider.address, f.newImplementation.address, "0x"
      )
    ).to.be.revertedWith("INNBC Proxy Admin: proxy not allowed");

    await expect(
      f.upgradeGovernance.createUpgradeProposal(REGISTRY_PROXY, ZERO, "0x")
    ).to.be.revertedWith("INNBC Proxy Admin: zero implementation");

    await expect(
      f.upgradeGovernance.createUpgradeProposal(
        REGISTRY_PROXY, f.outsider.address, "0x"
      )
    ).to.be.revertedWith("INNBC Proxy Admin: implementation has no code");

    expect(await f.target.version()).to.equal(1);
    expect(await f.target.value()).to.equal(7);
  });

  for (const withData of [false, true]) {
    it(`executes an approved upgrade once, initialization data=${withData}`, async function () {
      const f = await upgradeFixture();
      const data = withData
        ? f.newImplementation.interface.encodeFunctionData(
            "initializeUpgrade", [42, false]
          )
        : "0x";

      const id = await proposal(
        f.upgradeGovernance,
        "createUpgradeProposal",
        [REGISTRY_PROXY, f.newImplementation.address, data]
      );

      await expect(
        f.upgradeGovernance.executeUpgrade(id)
      ).to.be.revertedWith("INNBC Proxy Admin: proposal not finalized");

      await approve(f.upgradeGovernance, id, f.validators.slice(0, 2));
      await mined(f.upgradeGovernance.finalizeProposal(id));

      expect(await f.target.version()).to.equal(1);
      await mined(f.upgradeGovernance.connect(f.outsider).executeUpgrade(id));

      expect(await f.target.version()).to.equal(2);
      expect(await f.target.value()).to.equal(withData ? 42 : 7);
      expect((await f.upgradeGovernance.proposals(id)).executed).to.equal(true);

      await expect(
        f.upgradeGovernance.executeUpgrade(id)
      ).to.be.revertedWith("INNBC Proxy Admin: proposal executed");
    });
  }

  it("rolls back implementation and execution flag when upgrade initialization reverts", async function () {
    const f = await upgradeFixture();
    const data = f.newImplementation.interface.encodeFunctionData(
      "initializeUpgrade", [99, true]
    );

    const id = await proposal(
      f.upgradeGovernance,
      "createUpgradeProposal",
      [REGISTRY_PROXY, f.newImplementation.address, data]
    );
    await approve(f.upgradeGovernance, id, f.validators.slice(0, 2));
    await mined(f.upgradeGovernance.finalizeProposal(id));

    for (let attempt = 0; attempt < 2; attempt++) {
      await expect(
        f.upgradeGovernance.executeUpgrade(id)
      ).to.be.revertedWith("AUDIT_UPGRADE_REJECTED");

      const record = await f.upgradeGovernance.proposals(id);
      expect(record.finalized).to.equal(true);
      expect(record.passed).to.equal(true);
      expect(record.executed).to.equal(false);
      expect(await f.target.version()).to.equal(1);
      expect(await f.target.value()).to.equal(7);
    }
  });

  it("cannot execute a rejected proposal", async function () {
    const f = await upgradeFixture();
    const id = await proposal(
      f.upgradeGovernance,
      "createUpgradeProposal",
      [REGISTRY_PROXY, f.newImplementation.address, "0x"]
    );

    await mined(f.upgradeGovernance.connect(f.validators[0]).vote(id, false));
    await mined(f.upgradeGovernance.connect(f.validators[1]).vote(id, false));
    await mined(f.upgradeGovernance.finalizeProposal(id));

    await expect(
      f.upgradeGovernance.executeUpgrade(id)
    ).to.be.revertedWith("INNBC Proxy Admin: proposal not passed");
    expect(await f.target.version()).to.equal(1);
  });
});

/* FINISH ALPHA CHANGE 2403 -- test governed upgrades through the actual transparent proxy -- */