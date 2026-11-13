/* START ALPHA CHANGE 2400 -- test initialization and validator registry lifecycle -- */
import { expect } from "chai";
import {
  ZERO,
  PROOF,
  approve,
  deploy,
  keyBytes,
  mined,
  proposal,
  registryFixture,
  resetAuditChain,
} from "./helpers/innbcAudit";
import { ethers } from "hardhat";

describe("INNBC audit: initialization", function () {
  beforeEach(resetAuditChain);

  const cases = [
    {
      name: "INNBCValidatorRegistry",
      args: (a: string, b: string, c: string) => [a, 100, 1, b, c, 2],
      zeroAddresses: [0, 3, 4],
      zeroNumbers: [1, 2, 5],
      getters: ["genesisValidator", "validatorGovernance", "nativeToken"],
      getterArgs: [0, 3, 4],
    },
    {
      name: "INNBCValidatorGovernance",
      args: (a: string) => [a, 1],
      zeroAddresses: [0],
      zeroNumbers: [1],
      getters: ["validatorRegistry"],
      getterArgs: [0],
    },
    {
      name: "INNBCTreasuryDAO",
      args: (a: string, b: string) => [a, b],
      zeroAddresses: [0, 1],
      zeroNumbers: [],
      getters: ["validatorRegistry", "nativeToken"],
      getterArgs: [0, 1],
    },
    {
      name: "INNBCBlockRewardGovernance",
      args: (a: string, b: string, c: string) => [a, b, c],
      zeroAddresses: [0, 1, 2],
      zeroNumbers: [],
      getters: ["validatorRegistry", "nativeToken", "communityRewardPool"],
      getterArgs: [0, 1, 2],
    },
    {
      name: "INNBCCommunityRewardPool",
      args: (a: string, b: string) => [a, b],
      zeroAddresses: [0, 1],
      zeroNumbers: [],
      getters: ["validatorRegistry", "blockRewardGovernance"],
      getterArgs: [0, 1],
    },
    {
      name: "INNBCFeeBurnVault",
      args: (a: string) => [a],
      zeroAddresses: [0],
      zeroNumbers: [],
      getters: ["nativeToken"],
      getterArgs: [0],
    },
    {
      name: "INNBCProxyAdminGovernance",
      args: (a: string, b: string) => [a, b],
      zeroAddresses: [0, 1],
      zeroNumbers: [],
      getters: ["validatorRegistry", "proxyAdminExecutor"],
      getterArgs: [0, 1],
    },
    {
      name: "INNBCProxyAdminExecutor",
      args: (a: string) => [a],
      zeroAddresses: [0],
      zeroNumbers: [],
      getters: ["governance"],
      getterArgs: [0],
    },
  ];

  for (const entry of cases) {
    it(`${entry.name}: rejects invalid initialization and initializes once`, async function () {
      const [a, b, c] = await ethers.getSigners();
      const args: any[] = entry.args(a.address, b.address, c.address);
      const contract = await deploy(entry.name);

      for (const index of entry.zeroAddresses) {
        const invalid = [...args];
        invalid[index] = ZERO;
        await expect(contract.initialize(...invalid)).to.be.reverted;
        expect(await contract.initialized()).to.equal(false);
      }

      for (const index of entry.zeroNumbers) {
        const invalid = [...args];
        invalid[index] = 0;
        await expect(contract.initialize(...invalid)).to.be.reverted;
        expect(await contract.initialized()).to.equal(false);
      }

      await mined(contract.initialize(...args));
      expect(await contract.initialized()).to.equal(true);

      for (let i = 0; i < entry.getters.length; i++) {
        expect(await contract[entry.getters[i]]()).to.equal(
          args[entry.getterArgs[i]]
        );
      }

      await expect(contract.initialize(...args)).to.be.reverted;
    });
  }
});

describe("INNBC audit: registry lifecycle with controlled BLS responses", function () {
  beforeEach(resetAuditChain);

  it("requires governance admission and preserves unique identities", async function () {
    const f = await registryFixture();
    const candidate = f.accounts[3];
    const reward = f.accounts[9];

    await mined(f.token.mint(candidate.address, 1000));
    await mined(f.token.connect(candidate).approve(f.registry.address, 1000));

    await expect(
      f.registry.connect(candidate).registerValidator(
        f.rewards[0].address, keyBytes(4), PROOF, "candidate-peer", 100
      )
    ).to.be.revertedWith("INNBC Registry: reward address already used");

    await expect(
      f.registry.connect(candidate).registerValidator(
        reward.address, keyBytes(1), PROOF, "candidate-peer", 100
      )
    ).to.be.revertedWith("INNBC Registry: BLS key already used");

    await expect(
      f.registry.connect(candidate).registerValidator(
        reward.address, keyBytes(4), PROOF, "audit-peer-0", 100
      )
    ).to.be.revertedWith("INNBC Registry: peer ID already used");

    expect(await f.token.balanceOf(candidate.address)).to.equal(1000);
    expect((await f.registry.validators(candidate.address)).registered).to.equal(false);

    await mined(
      f.registry.connect(candidate).registerValidator(
        reward.address, keyBytes(4), PROOF, "candidate-peer", 100
      )
    );

    const record = await f.registry.validators(candidate.address);
    expect(record.registered).to.equal(true);
    expect(record.blsProofVerified).to.equal(true);
    expect(record.whitelisted).to.equal(false);
    expect(await f.registry.activeValidatorCount()).to.equal(3);

    await expect(
      f.registry.connect(f.outsider).whitelistValidator(candidate.address)
    ).to.be.revertedWith("INNBC Registry: sender not validator governance");

    const id = await proposal(f.governance, "createWhitelistProposal", [
      candidate.address,
    ]);
    await approve(f.governance, id, f.validators.slice(0, 2));
    await mined(f.governance.connect(f.outsider).finalizeProposal(id));

    expect((await f.registry.validators(candidate.address)).whitelisted).to.equal(true);
    expect(await f.registry.activeValidatorCount()).to.equal(4);
    expect(await f.registry.getActiveValidators()).to.include(candidate.address);
  });

  it("rolls back registration when BLS or staking transfer fails", async function () {
    const f = await registryFixture();
    const candidate = f.accounts[3];

    await mined(f.token.mint(candidate.address, 1000));
    await mined(f.token.connect(candidate).approve(f.registry.address, 1000));

    const register = () =>
      f.registry.connect(candidate).registerValidator(
        f.accounts[9].address, keyBytes(4), PROOF, "candidate-peer", 100
      );

    for (const response of [[false, true], [true, false]]) {
      await mined(f.bls.configure(...response));
      await expect(register()).to.be.revertedWith("INNBC Registry: invalid BLS proof");
    }

    await mined(f.bls.configure(true, true));
    await mined(f.token.setFailTransfers(true));
    await expect(register()).to.be.revertedWith("INNBC Registry: stake transfer failed");

    expect((await f.registry.validators(candidate.address)).registered).to.equal(false);
    expect(await f.registry.activeValidatorCount()).to.equal(3);
    expect(await f.token.balanceOf(candidate.address)).to.equal(1000);

    await mined(f.token.setFailTransfers(false));
    await mined(register());
    expect(await f.token.balanceOf(candidate.address)).to.equal(900);
    expect((await f.registry.validators(candidate.address)).stake).to.equal(100);
  });

  it("keeps stake and active membership consistent across failed withdrawals", async function () {
    const f = await registryFixture();
    const operator = f.validators[0];

    await mined(f.registry.connect(operator).increaseStake(50));
    expect((await f.registry.validators(operator.address)).stake).to.equal(150);

    await mined(f.token.setFailTransfers(true));
    await expect(
      f.registry.connect(operator).unstake(60)
    ).to.be.revertedWith("INNBC Registry: unstake transfer failed");

    let record = await f.registry.validators(operator.address);
    expect(record.stake).to.equal(150);
    expect(record.whitelisted).to.equal(true);
    expect(await f.registry.activeValidatorCount()).to.equal(3);

    await mined(f.token.setFailTransfers(false));
    await mined(f.registry.connect(operator).unstake(60));

    record = await f.registry.validators(operator.address);
    expect(record.stake).to.equal(90);
    expect(record.whitelisted).to.equal(false);
    expect(await f.registry.activeValidatorCount()).to.equal(2);
    expect(await f.registry.getActiveValidators()).not.to.include(operator.address);
    expect(await f.token.balanceOf(operator.address)).to.equal(910);

    await expect(
      f.registry.connect(operator).unstake(91)
    ).to.be.revertedWith("INNBC Registry: unstake exceeds stake");
  });

  it("preserves old BLS identity on failure and deactivates after successful rotation", async function () {
    const f = await registryFixture();
    const operator = f.validators[0];

    await expect(
      f.registry.connect(operator).updateBLSKey(keyBytes(1), PROOF)
    ).to.be.revertedWith("INNBC Registry: BLS key unchanged");

    await expect(
      f.registry.connect(operator).updateBLSKey(keyBytes(2), PROOF)
    ).to.be.revertedWith("INNBC Registry: BLS key already used");

    await mined(f.bls.configure(false, true));
    await expect(
      f.registry.connect(operator).updateBLSKey(keyBytes(9), PROOF)
    ).to.be.revertedWith("INNBC Registry: invalid BLS proof");

    let record = await f.registry.validators(operator.address);
    expect(record.blsKey).to.equal(keyBytes(1));
    expect(record.whitelisted).to.equal(true);

    await mined(f.bls.configure(true, true));
    await mined(f.registry.connect(operator).updateBLSKey(keyBytes(9), PROOF));

    record = await f.registry.validators(operator.address);
    expect(record.blsKey).to.equal(keyBytes(9));
    expect(record.blsProofVerified).to.equal(true);
    expect(record.whitelisted).to.equal(false);
    expect(await f.registry.activeValidatorCount()).to.equal(2);
  });

  it("releases replaced reward and peer identities without changing membership", async function () {
    const f = await registryFixture();
    const [first, second] = f.validators;

    await expect(
      f.registry.connect(first).updateRewardAddress(f.rewards[1].address)
    ).to.be.revertedWith("INNBC Registry: reward address already used");
    await expect(
      f.registry.connect(first).updateLibP2PPeerID("")
    ).to.be.revertedWith("INNBC Registry: empty peer ID");

    await mined(f.registry.connect(first).updateRewardAddress(f.accounts[9].address));
    await mined(f.registry.connect(second).updateRewardAddress(f.rewards[0].address));

    await mined(f.registry.connect(first).updateLibP2PPeerID("replacement-peer"));
    await mined(f.registry.connect(second).updateLibP2PPeerID("audit-peer-0"));

    expect((await f.registry.validators(second.address)).rewardAddress).to.equal(
      f.rewards[0].address
    );
    expect((await f.registry.validators(second.address)).libp2pPeerID).to.equal(
      "audit-peer-0"
    );
    expect(await f.registry.activeValidatorCount()).to.equal(3);
  });
});

/* FINISH ALPHA CHANGE 2400 -- test initialization and validator registry lifecycle -- */