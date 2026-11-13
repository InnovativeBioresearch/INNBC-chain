/* START ALPHA CHANGE 3559 -- verify governance electorates across validator membership changes -- */
import { expect } from "chai";
import { ethers } from "hardhat";
import {
  PROOF,
  REGISTRY_PROXY,
  approve,
  deploy,
  financeFixture,
  keyBytes,
  mined,
  mineTo,
  proposal,
  resetAuditChain,
} from "./helpers/innbcAudit";

async function votingFixture(kind: string, count = 5) {
  const f = await financeFixture(count);

  if (kind === "validator") {
    return {
      ...f,
      voting: f.governance,
      prefix: "INNBC Validator Governance",
      method: "createUseStandardProposalDurationProposal",
      args: [],
    };
  }

  if (kind === "treasury") {
    await mined(f.outsider.sendTransaction({ to: f.treasury.address, value: 100 }));
    return {
      ...f,
      voting: f.treasury,
      prefix: "INNBC Treasury",
      method: "createLockProposal",
      args: [10],
    };
  }

  if (kind === "block reward") {
    return {
      ...f,
      voting: f.rewardsGovernance,
      prefix: "INNBC Block Reward",
      method: "createUpdateMaxBaseFeeProposal",
      args: [100],
    };
  }

  if (kind === "community") {
    return {
      ...f,
      voting: f.pool,
      prefix: "INNBC Community Reward",
      method: "createUseFivePercentCommunityRewardRateProposal",
      args: [],
    };
  }

  const voting = await deploy("INNBCProxyAdminGovernance");
  const executor = await deploy("INNBCProxyAdminExecutor");
  await mined(executor.initialize(voting.address));
  await mined(voting.initialize(f.registry.address, executor.address));

  return {
    ...f,
    voting,
    prefix: "INNBC Proxy Admin",
    method: "createUpgradeProposal",
    args: [REGISTRY_PROXY, f.pool.address, "0x"],
  };
}

async function admitFourthValidator(
  f: Awaited<ReturnType<typeof financeFixture>>
) {
  const candidate = f.accounts[3];
  const reward = f.accounts[12];

  await mined(f.token.mint(candidate.address, 1000));
  await mined(f.token.connect(candidate).approve(f.registry.address, 1000));
  await mined(
    f.registry.connect(candidate).registerValidator(
      reward.address, keyBytes(99), PROOF, "electorate-new-peer", 100
    )
  );

  const admission = await proposal(f.governance, "createWhitelistProposal", [
    candidate.address,
  ]);
  await approve(f.governance, admission, f.validators.slice(0, 2));
  await mined(f.governance.connect(f.outsider).finalizeProposal(admission));
  expect(await f.registry.activeValidatorCount()).to.equal(4);

  return candidate;
}

for (const kind of ["validator", "treasury", "block reward", "community", "proxy"]) {
  describe(`INNBC electorate: ${kind}`, function () {
    beforeEach(resetAuditChain);

    it("retains departed yes voters without lowering their original threshold", async function () {
      const f = await votingFixture(kind);
      const id = await proposal(f.voting, f.method, f.args);
      await approve(f.voting, id, f.validators.slice(0, 2));

      await mined(f.registry.connect(f.validators[0]).unstake(100));
      await mined(f.registry.connect(f.validators[1]).unstake(100));
      expect(await f.registry.activeValidatorCount()).to.equal(3);

      await expect(f.voting.finalizeProposal(id)).to.be.revertedWith(
        `${f.prefix}: proposal still active`
      );
      expect((await f.voting.proposals(id)).finalized).to.equal(false);
      expect(await f.voting.hasVoted(id, f.validators[0].address)).to.equal(true);

      await approve(f.voting, id, [f.validators[2]]);
      await mined(f.voting.connect(f.outsider).finalizeProposal(id));
      expect((await f.voting.proposals(id)).yesVotes).to.equal(3);
      expect((await f.voting.proposals(id)).passed).to.equal(true);
    });

    it("retains departed no voters in early rejection calculations", async function () {
      const f = await votingFixture(kind);
      const id = await proposal(f.voting, f.method, f.args);
      await approve(f.voting, id, [f.validators[0]]);
      await mined(f.voting.connect(f.validators[1]).vote(id, false));
      await mined(f.registry.connect(f.validators[1]).unstake(100));
      await mined(f.voting.connect(f.validators[2]).vote(id, false));

      await expect(f.voting.finalizeProposal(id)).to.be.revertedWith(
        `${f.prefix}: proposal still active`
      );

      await mined(f.voting.connect(f.validators[3]).vote(id, false));
      await mined(f.voting.finalizeProposal(id));
      const result = await f.voting.proposals(id);
      expect(result.noVotes).to.equal(3);
      expect(result.finalized).to.equal(true);
      expect(result.passed).to.equal(false);
    });

    it("includes a newly admitted validator before the deadline", async function () {
      const f = await votingFixture(kind, 3);
      const id = await proposal(f.voting, f.method, f.args);
      await approve(f.voting, id, f.validators.slice(0, 2));
      const candidate = await admitFourthValidator(f);

      await expect(f.voting.finalizeProposal(id)).to.be.revertedWith(
        `${f.prefix}: proposal still active`
      );
      await approve(f.voting, id, [candidate]);
      await mined(f.voting.finalizeProposal(id));
      expect((await f.voting.proposals(id)).passed).to.equal(true);
    });

    it("uses membership at finalization without reopening an expired voting window", async function () {
      const f = await votingFixture(kind, 3);
      const id = await proposal(f.voting, f.method, f.args);
      await approve(f.voting, id, f.validators.slice(0, 2));
      await mineTo((await f.voting.proposals(id)).deadlineBlock.toNumber() + 1);
      const candidate = await admitFourthValidator(f);

      await expect(f.voting.connect(candidate).vote(id, true)).to.be.revertedWith(
        `${f.prefix}: proposal expired`
      );
      await mined(f.voting.finalizeProposal(id));
      expect((await f.voting.proposals(id)).passed).to.equal(false);
    });

    it("counts a readmitted voter once and preserves duplicate-vote rejection", async function () {
      const f = await votingFixture(kind);
      const id = await proposal(f.voting, f.method, f.args);
      await approve(f.voting, id, f.validators.slice(0, 2));
      const returning = f.validators[0];

      await mined(f.registry.connect(returning).unstake(100));
      await mined(f.registry.connect(returning).increaseStake(100));
      const readmission = await proposal(
        f.governance.connect(f.validators[1]), "createWhitelistProposal",
        [returning.address]
      );
      await approve(f.governance, readmission, f.validators.slice(1, 4));
      await mined(f.governance.finalizeProposal(readmission));
      expect(await f.registry.activeValidatorCount()).to.equal(5);

      await expect(f.voting.connect(returning).vote(id, true)).to.be.revertedWith(
        `${f.prefix}: already voted`
      );
      await approve(f.voting, id, [f.validators[2]]);
      await mined(f.voting.finalizeProposal(id));
      expect((await f.voting.proposals(id)).passed).to.equal(true);
    });

    it("finalizes an empty electorate as rejected", async function () {
      const f = await votingFixture(kind, 1);
      const id = await proposal(f.voting, f.method, f.args);
      await mined(f.registry.connect(f.validators[0]).unstake(100));
      await mined(f.voting.connect(f.outsider).finalizeProposal(id));

      const result = await f.voting.proposals(id);
      expect(result.finalized).to.equal(true);
      expect(result.passed).to.equal(false);
      expect(await f.voting.getActiveProposalIDs()).not.to.deep.include(id);
    });
  });
}

describe("INNBC electorate: emergency rewards", function () {
  beforeEach(resetAuditChain);

  it("requires departed approvals plus every current validator and pays current recipients", async function () {
    const f = await financeFixture(5);
    const id = await proposal(f.rewardsGovernance, "createOnePercentEmergencyRewardProposal");
    await approve(f.rewardsGovernance, id, f.validators.slice(0, 2));
    await mined(f.registry.connect(f.validators[0]).unstake(100));
    await mined(f.registry.connect(f.validators[1]).unstake(100));

    await approve(f.rewardsGovernance, id, [f.validators[2]]);
    await expect(f.rewardsGovernance.finalizeProposal(id)).to.be.revertedWith(
      "INNBC Block Reward: proposal still active"
    );
    await approve(f.rewardsGovernance, id, [f.validators[3]]);
    await expect(f.rewardsGovernance.finalizeProposal(id)).to.be.revertedWith(
      "INNBC Block Reward: proposal still active"
    );
    await approve(f.rewardsGovernance, id, [f.validators[4]]);

    const originalSupply = await f.token.totalSupply();
    const each = originalSupply.mul(100).div(10000).div(3);
    await mined(f.rewardsGovernance.finalizeProposal(id));
    expect((await f.rewardsGovernance.proposals(id)).passed).to.equal(true);
    expect(await f.token.totalSupply()).to.equal(originalSupply.add(each.mul(3)));
    for (let index = 0; index < f.rewards.length; index++) {
      expect(await f.token.balanceOf(f.rewards[index].address)).to.equal(
        index < 2 ? 0 : each
      );
    }
  });

  it("preserves a departed no vote as an emergency veto", async function () {
    const f = await financeFixture(5);
    const id = await proposal(f.rewardsGovernance, "createOnePercentEmergencyRewardProposal");
    await approve(f.rewardsGovernance, id, [f.validators[0]]);
    await mined(f.rewardsGovernance.connect(f.validators[1]).vote(id, false));
    await mined(f.registry.connect(f.validators[1]).unstake(100));
    await approve(f.rewardsGovernance, id, f.validators.slice(2));

    const originalSupply = await f.token.totalSupply();
    await mined(f.rewardsGovernance.finalizeProposal(id));
    expect((await f.rewardsGovernance.proposals(id)).passed).to.equal(false);
    expect(await f.token.totalSupply()).to.equal(originalSupply);
    expect(await f.rewardsGovernance.emergencyRewardProposalActive()).to.equal(false);
    expect(await f.rewardsGovernance.lastEmergencyRewardBlock()).to.equal(0);
  });

  it("requires an additional approval when a validator joins", async function () {
    const f = await financeFixture(3);
    const id = await proposal(f.rewardsGovernance, "createOnePercentEmergencyRewardProposal");
    await approve(f.rewardsGovernance, id, f.validators);
    const candidate = await admitFourthValidator(f);

    await expect(f.rewardsGovernance.finalizeProposal(id)).to.be.revertedWith(
      "INNBC Block Reward: proposal still active"
    );
    await approve(f.rewardsGovernance, id, [candidate]);
    await mined(f.rewardsGovernance.finalizeProposal(id));
    expect((await f.rewardsGovernance.proposals(id)).passed).to.equal(true);
  });

  it("rejects zero-vote unanimity and releases the active proposal", async function () {
    const f = await financeFixture(1);
    const id = await proposal(f.rewardsGovernance, "createOnePercentEmergencyRewardProposal");
    await mined(f.registry.connect(f.validators[0]).unstake(100));
    const originalSupply = await f.token.totalSupply();

    await mined(f.rewardsGovernance.finalizeProposal(id));
    expect((await f.rewardsGovernance.proposals(id)).finalized).to.equal(true);
    expect((await f.rewardsGovernance.proposals(id)).passed).to.equal(false);
    expect(await f.token.totalSupply()).to.equal(originalSupply);
    expect(await f.rewardsGovernance.emergencyRewardProposalActive()).to.equal(false);
    expect(await f.rewardsGovernance.lastEmergencyRewardBlock()).to.equal(0);
  });
});

describe("INNBC electorate: representative gas bound", function () {
  beforeEach(resetAuditChain);

  it("finalizes with 100 active entries within a one-million-gas transaction", async function () {
    const [validator, reward] = await ethers.getSigners();
    const registry = await deploy("MockINNBCValidatorRegistry");
    await mined(registry.addActiveValidator(validator.address, reward.address, 1));
    for (let index = 1; index < 100; index++) {
      const address = ethers.utils.getAddress(
        ethers.utils.hexZeroPad(ethers.utils.hexlify(4096 + index), 20)
      );
      await mined(registry.addActiveValidator(address, address, 1));
    }

    const token = await deploy("INNBCAuditToken");
    const pool = await deploy("INNBCCommunityRewardPool");
    const governance = await deploy("INNBCBlockRewardGovernance");
    await mined(pool.initialize(registry.address, governance.address));
    await mined(governance.initialize(registry.address, token.address, pool.address));

    const id = await proposal(governance, "createUpdateMaxBaseFeeProposal", [100]);
    await mineTo((await governance.proposals(id)).deadlineBlock.toNumber() + 1);
    const receipt = await (
      await governance.finalizeProposal(id, { gasLimit: 1_000_000 })
    ).wait();
    expect(receipt.gasUsed).to.be.lt(1_000_000);
    expect((await governance.proposals(id)).passed).to.equal(false);
  });
});
/* FINISH ALPHA CHANGE 3559 -- verify governance electorates across validator membership changes -- */