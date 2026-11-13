/* START ALPHA CHANGE 2401 -- test governance permissions thresholds deadlines and membership changes -- */
import { expect } from "chai";
import {
  REGISTRY_PROXY,
  approve,
  deploy,
  financeFixture,
  mined,
  mineTo,
  proposal,
  resetAuditChain,
} from "./helpers/innbcAudit";

async function votingFixture(kind: string) {
  const f = await financeFixture();

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

for (const kind of ["validator", "treasury", "block reward", "community", "proxy"]) {
  describe(`INNBC audit: ${kind} voting`, function () {
    beforeEach(resetAuditChain);

    it("requires active voters, rejects duplicates, and needs two of three approvals", async function () {
      const f = await votingFixture(kind);

      await expect(
        f.voting.connect(f.outsider)[f.method](...f.args)
      ).to.be.revertedWith(`${f.prefix}: sender not active validator`);

      const id = await proposal(f.voting, f.method, f.args);

      await expect(
        f.voting.connect(f.outsider).vote(id, true)
      ).to.be.revertedWith(`${f.prefix}: sender not active validator`);

      await mined(f.voting.connect(f.validators[0]).vote(id, true));
      await expect(
        f.voting.connect(f.validators[0]).vote(id, true)
      ).to.be.revertedWith(`${f.prefix}: already voted`);

      await expect(
        f.voting.finalizeProposal(id)
      ).to.be.revertedWith(`${f.prefix}: proposal still active`);

      await mined(f.voting.connect(f.validators[1]).vote(id, true));
      await mined(f.voting.connect(f.outsider).finalizeProposal(id));

      const result = await f.voting.proposals(id);
      expect(result.finalized).to.equal(true);
      expect(result.passed).to.equal(true);
      expect(await f.voting.getActiveProposalIDs()).not.to.deep.include(id);

      await expect(
        f.voting.finalizeProposal(id)
      ).to.be.revertedWith(`${f.prefix}: proposal finalized`);
    });

    it("accepts voting at the deadline and rejects voting after it", async function () {
      const f = await votingFixture(kind);
      const id = await proposal(f.voting, f.method, f.args);
      const deadline = (await f.voting.proposals(id)).deadlineBlock.toNumber();

      await mineTo(deadline);
      await f.voting.connect(f.validators[0]).callStatic.vote(id, true);
      expect(await f.voting.hasVoted(id, f.validators[0].address)).to.equal(false);

      await mineTo(deadline + 1);
      await expect(
        f.voting.connect(f.validators[0]).callStatic.vote(id, true)
      ).to.be.revertedWith(`${f.prefix}: proposal expired`);

      await mined(f.voting.connect(f.outsider).finalizeProposal(id));
      const result = await f.voting.proposals(id);
      expect(result.finalized).to.equal(true);
      expect(result.passed).to.equal(false);
      expect(await f.voting.getActiveProposalIDs()).not.to.deep.include(id);
    });

    it("finalizes an early rejection once approval is impossible", async function () {
      const f = await votingFixture(kind);
      const id = await proposal(f.voting, f.method, f.args);

      await mined(f.voting.connect(f.validators[0]).vote(id, false));
      await expect(
        f.voting.finalizeProposal(id)
      ).to.be.revertedWith(`${f.prefix}: proposal still active`);

      await mined(f.voting.connect(f.validators[1]).vote(id, false));
      await mined(f.voting.finalizeProposal(id));

      expect((await f.voting.proposals(id)).passed).to.equal(false);
      expect((await f.voting.proposals(id)).finalized).to.equal(true);
    });

    it("uses current membership and prevents removed validators from voting", async function () {
      const f = await votingFixture(kind);
      const id = await proposal(f.voting, f.method, f.args);

      await mined(f.voting.connect(f.validators[0]).vote(id, true));
      await mined(f.registry.connect(f.validators[1]).unstake(100));
      await mined(f.registry.connect(f.validators[2]).unstake(100));
      expect(await f.registry.activeValidatorCount()).to.equal(1);

      await expect(
        f.voting.connect(f.validators[1]).vote(id, true)
      ).to.be.revertedWith(`${f.prefix}: sender not active validator`);

      await mined(f.voting.connect(f.outsider).finalizeProposal(id));
      expect((await f.voting.proposals(id)).passed).to.equal(true);
    });
  });
}

describe("INNBC audit: validator governance execution", function () {
  beforeEach(resetAuditChain);

  it("updates minimum stake and removes all newly under-staked validators", async function () {
    const f = await financeFixture();
    const id = await proposal(f.governance, "createUpdateMinimumStakeProposal", [101]);

    await approve(f.governance, id, f.validators.slice(0, 2));
    await mined(f.governance.connect(f.outsider).finalizeProposal(id));

    expect(await f.registry.minimumStake()).to.equal(101);
    expect(await f.registry.activeValidatorCount()).to.equal(0);
    expect(await f.registry.getActiveValidators()).to.deep.equal([]);

    for (const validator of f.validators) {
      expect((await f.registry.validators(validator.address)).whitelisted).to.equal(false);
    }
  });

  it("removes finalized IDs correctly when proposals finish out of order", async function () {
    const f = await financeFixture();
    const first = await proposal(f.governance, "createUseStandardProposalDurationProposal");
    const second = await proposal(f.governance, "createUseLongProposalDurationProposal");

    await approve(f.governance, first, f.validators.slice(0, 2));
    await approve(f.governance, second, f.validators.slice(0, 2));
    await mined(f.governance.finalizeProposal(first));

    expect(await f.governance.getActiveProposalIDs()).to.deep.equal([second]);
    expect(await f.registry.proposalDurationBlocks()).to.equal(43200);

    await mined(f.governance.finalizeProposal(second));
    expect(await f.governance.getActiveProposalIDs()).to.deep.equal([]);
    expect(await f.registry.proposalDurationBlocks()).to.equal(86400);
    expect(await f.governance.proposalDurationBlocks()).to.equal(86400);
  });
});

/* FINISH ALPHA CHANGE 2401 -- test governance permissions thresholds deadlines and membership changes -- */