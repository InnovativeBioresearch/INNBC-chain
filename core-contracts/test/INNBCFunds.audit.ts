/* START ALPHA CHANGE 2402 -- test treasury reward accounting and atomic failure handling -- */
import { expect } from "chai";
import { ethers } from "hardhat";
import {
  ZERO,
  approve,
  deploy,
  financeFixture,
  mined,
  proposal,
  resetAuditChain,
} from "./helpers/innbcAudit";

describe("INNBC audit: treasury", function () {
  beforeEach(resetAuditChain);

  it("enforces locked balances and executes approved transfers once", async function () {
    const f = await financeFixture();
    const recipient = f.accounts[4];

    await mined(f.outsider.sendTransaction({ to: f.treasury.address, value: 1000 }));

    const lock = await proposal(f.treasury, "createLockProposal", [700]);
    await approve(f.treasury, lock, f.validators.slice(0, 2));
    await mined(f.treasury.finalizeProposal(lock));

    expect(await f.treasury.lockedTreasuryBalance()).to.equal(700);
    expect(await f.treasury.availableTreasuryBalance()).to.equal(300);
    await expect(
      f.treasury.createTransferProposal(recipient.address, 301)
    ).to.be.revertedWith("INNBC Treasury: amount exceeds available balance");

    const unlock = await proposal(f.treasury, "createUnlockProposal", [200]);
    await approve(f.treasury, unlock, f.validators.slice(0, 2));
    await mined(f.treasury.finalizeProposal(unlock));

    const transfer = await proposal(f.treasury, "createTransferProposal", [
      recipient.address, 500,
    ]);
    await approve(f.treasury, transfer, f.validators.slice(0, 2));

    const before = await ethers.provider.getBalance(recipient.address);
    await mined(f.treasury.connect(f.outsider).finalizeProposal(transfer));
    expect(await ethers.provider.getBalance(recipient.address)).to.equal(before.add(500));
    expect(await ethers.provider.getBalance(f.treasury.address)).to.equal(500);
    expect(await f.treasury.availableTreasuryBalance()).to.equal(0);

    await expect(
      f.treasury.finalizeProposal(transfer)
    ).to.be.revertedWith("INNBC Treasury: proposal finalized");
  });

  it("rolls back failed native transfers and allows a later successful retry", async function () {
    const f = await financeFixture();
    const receiver = await deploy("INNBCAuditReceiver");
    await mined(receiver.configure(true, false));
    await mined(f.outsider.sendTransaction({ to: f.treasury.address, value: 100 }));

    const id = await proposal(f.treasury, "createTransferProposal", [receiver.address, 100]);
    await approve(f.treasury, id, f.validators.slice(0, 2));

    await expect(
      f.treasury.finalizeProposal(id)
    ).to.be.revertedWith("INNBC Treasury: transfer failed");

    expect((await f.treasury.proposals(id)).finalized).to.equal(false);
    expect(await f.treasury.getActiveProposalIDs()).to.deep.equal([id]);
    expect(await ethers.provider.getBalance(f.treasury.address)).to.equal(100);

    await mined(receiver.configure(false, false));
    await mined(f.treasury.finalizeProposal(id));
    expect(await ethers.provider.getBalance(receiver.address)).to.equal(100);
  });

  it("rolls back a failed external-token transfer and rejects native-token misuse", async function () {
    const f = await financeFixture();
    const external = await deploy("INNBCAuditToken");
    await mined(external.mint(f.treasury.address, 500));

    await expect(
      f.treasury.createERC20TransferProposal(
        f.token.address, f.outsider.address, 100
      )
    ).to.be.revertedWith("INNBC Treasury: use native transfer for INNBC");

    const id = await proposal(f.treasury, "createERC20TransferProposal", [
      external.address, f.outsider.address, 100,
    ]);
    await approve(f.treasury, id, f.validators.slice(0, 2));
    await mined(external.setFailTransfers(true));

    await expect(f.treasury.finalizeProposal(id)).to.be.reverted;
    expect((await f.treasury.proposals(id)).finalized).to.equal(false);
    expect(await external.balanceOf(f.treasury.address)).to.equal(500);

    await mined(external.setFailTransfers(false));
    await mined(f.treasury.finalizeProposal(id));
    expect(await external.balanceOf(f.outsider.address)).to.equal(100);
    expect(await external.balanceOf(f.treasury.address)).to.equal(400);
  });

  it("preserves the proposal and funds when its burn dependency rejects", async function () {
    const f = await financeFixture();
    await mined(f.outsider.sendTransaction({ to: f.treasury.address, value: 100 }));
    await mined(f.token.setFailBurn(true));

    const id = await proposal(f.treasury, "createBurnProposal", [100]);
    await approve(f.treasury, id, f.validators.slice(0, 2));

    await expect(
      f.treasury.finalizeProposal(id)
    ).to.be.revertedWith("INNBC Treasury: burn failed");

    expect((await f.treasury.proposals(id)).finalized).to.equal(false);
    expect(await f.treasury.getActiveProposalIDs()).to.deep.equal([id]);
    expect(await ethers.provider.getBalance(f.treasury.address)).to.equal(100);
  });
});

describe("INNBC audit: rewards and fee governance", function () {
  beforeEach(resetAuditChain);

  it("requires unanimous emergency approval and rolls back partial issuance", async function () {
    const f = await financeFixture();
    await mined(f.token.mint(f.outsider.address, 51));
    const originalSupply = await f.token.totalSupply();

    const id = await proposal(f.rewardsGovernance, "createTwoPercentEmergencyRewardProposal");
    await approve(f.rewardsGovernance, id, f.validators.slice(0, 2));
    await expect(
      f.rewardsGovernance.finalizeProposal(id)
    ).to.be.revertedWith("INNBC Block Reward: proposal still active");

    await approve(f.rewardsGovernance, id, [f.validators[2]]);
    await mined(f.token.setFailMintRecipient(f.rewards[2].address));

    await expect(
      f.rewardsGovernance.finalizeProposal(id)
    ).to.be.revertedWith("INNBC Block Reward: emergency block reward issue failed");

    expect(await f.token.totalSupply()).to.equal(originalSupply);
    for (const recipient of f.rewards) {
      expect(await f.token.balanceOf(recipient.address)).to.equal(0);
    }
    expect((await f.rewardsGovernance.proposals(id)).finalized).to.equal(false);
    expect(await f.rewardsGovernance.emergencyRewardProposalActive()).to.equal(true);
    expect(await f.rewardsGovernance.lastEmergencyRewardBlock()).to.equal(0);

    await mined(f.token.setFailMintRecipient(ZERO));
    await mined(f.rewardsGovernance.connect(f.outsider).finalizeProposal(id));

    const each = originalSupply.mul(200).div(10000).div(3);
    expect(await f.token.totalSupply()).to.equal(originalSupply.add(each.mul(3)));
    for (const recipient of f.rewards) {
      expect(await f.token.balanceOf(recipient.address)).to.equal(each);
    }

    expect(await f.rewardsGovernance.emergencyRewardProposalActive()).to.equal(false);
    await expect(
      f.rewardsGovernance.createOnePercentEmergencyRewardProposal()
    ).to.be.revertedWith("INNBC Block Reward: emergency reward cooldown active");
  });

  it("a rejected emergency proposal releases its active flag without starting cooldown", async function () {
    const f = await financeFixture();
    const id = await proposal(f.rewardsGovernance, "createOnePercentEmergencyRewardProposal");

    await mined(f.rewardsGovernance.connect(f.validators[0]).vote(id, false));
    await mined(f.rewardsGovernance.finalizeProposal(id));

    expect((await f.rewardsGovernance.proposals(id)).passed).to.equal(false);
    expect(await f.rewardsGovernance.emergencyRewardProposalActive()).to.equal(false);
    expect(await f.rewardsGovernance.lastEmergencyRewardBlock()).to.equal(0);

    await proposal(f.rewardsGovernance, "createTwoPercentEmergencyRewardProposal");
  });

  it("enforces the combined fee split at execution in both directions", async function () {
    const f = await financeFixture();

    const burn = await proposal(f.rewardsGovernance, "createUseSeventyPercentFeeBurnRateProposal");
    const community = await proposal(f.pool, "createUseFiftyPercentCommunityRewardRateProposal");
    await approve(f.rewardsGovernance, burn, f.validators.slice(0, 2));
    await approve(f.pool, community, f.validators.slice(0, 2));
    await mined(f.rewardsGovernance.finalizeProposal(burn));

    await expect(
      f.pool.finalizeProposal(community)
    ).to.be.revertedWith("INNBC Community Reward: fee split too high");
    expect((await f.pool.proposals(community)).finalized).to.equal(false);
    expect(await f.pool.communityRewardRateBps()).to.equal(0);

    const smaller = await proposal(f.pool, "createUseTenPercentCommunityRewardRateProposal");
    await approve(f.pool, smaller, f.validators.slice(0, 2));
    await mined(f.pool.finalizeProposal(smaller));

    const excessive = await proposal(
      f.rewardsGovernance, "createUseOneHundredPercentFeeBurnRateProposal"
    );
    await approve(f.rewardsGovernance, excessive, f.validators.slice(0, 2));
    await expect(
      f.rewardsGovernance.finalizeProposal(excessive)
    ).to.be.revertedWith("INNBC Block Reward: fee split too high");

    expect(await f.rewardsGovernance.feeBurnRateBps()).to.equal(7000);
    expect(await f.pool.communityRewardRateBps()).to.equal(1000);
    expect((await f.rewardsGovernance.proposals(excessive)).finalized).to.equal(false);
  });

  it("stores approved fee parameters in the runtime-readable slots", async function () {
    const f = await financeFixture();

    const cases = [
      {
        contract: f.rewardsGovernance,
        method: "createUseTenPercentFeeBurnRateProposal",
        args: [],
        slot: "innbc.blockRewardGovernance.feeBurnRateBps",
        expected: 1000,
      },
      {
        contract: f.pool,
        method: "createUseFivePercentCommunityRewardRateProposal",
        args: [],
        slot: "innbc.communityRewardPool.communityRewardRateBps",
        expected: 500,
      },
      {
        contract: f.rewardsGovernance,
        method: "createUpdateMaxBaseFeeProposal",
        args: [123456],
        slot: "innbc.blockRewardGovernance.maxBaseFeeWei",
        expected: 123456,
      },
    ];

    for (const entry of cases) {
      const id = await proposal(entry.contract, entry.method, entry.args);
      await approve(entry.contract, id, f.validators.slice(0, 2));
      await mined(entry.contract.finalizeProposal(id));

      const stored = await ethers.provider.getStorageAt(
        entry.contract.address,
        ethers.utils.id(entry.slot)
      );
      expect(ethers.BigNumber.from(stored)).to.equal(entry.expected);
    }

    const disable = await proposal(
      f.rewardsGovernance, "createUpdateMaxBaseFeeProposal", [0]
    );
    await approve(f.rewardsGovernance, disable, f.validators.slice(0, 2));
    await mined(f.rewardsGovernance.finalizeProposal(disable));
    expect(await f.rewardsGovernance.maxBaseFeeWei()).to.equal(0);
  });
});

describe("INNBC audit: community reward accounting", function () {
  beforeEach(resetAuditChain);

  it("allocates pre-stake rewards and never treats new stake as reward", async function () {
    const f = await financeFixture();
    const first = f.accounts[3];
    const second = f.accounts[4];

    await mined(f.outsider.sendTransaction({ to: f.pool.address, value: 90 }));
    await mined(f.pool.connect(first).stake({ value: 30 }));
    expect(await f.pool.earned(first.address)).to.equal(90);

    await mined(f.pool.connect(second).stake({ value: 70 }));
    expect(await f.pool.earned(first.address)).to.equal(90);
    expect(await f.pool.earned(second.address)).to.equal(0);

    await mined(f.outsider.sendTransaction({ to: f.pool.address, value: 100 }));
    expect(await f.pool.earned(first.address)).to.equal(120);
    expect(await f.pool.earned(second.address)).to.equal(70);

    await mined(f.pool.connect(first).claim());
    await mined(f.pool.connect(second).claim());
    expect(await ethers.provider.getBalance(f.pool.address)).to.equal(100);
    expect(await f.pool.totalStaked()).to.equal(100);

    await mined(f.pool.connect(first).unstake(30));
    await mined(f.pool.connect(second).unstake(70));
    expect(await ethers.provider.getBalance(f.pool.address)).to.equal(0);
    expect(await f.pool.totalStaked()).to.equal(0);
  });

  it("leaves rounding dust in the pool without overpaying", async function () {
    const f = await financeFixture();
    const first = f.accounts[3];
    const second = f.accounts[4];

    await mined(f.pool.connect(first).stake({ value: 1 }));
    await mined(f.pool.connect(second).stake({ value: 2 }));
    await mined(f.outsider.sendTransaction({ to: f.pool.address, value: 10 }));

    expect(await f.pool.earned(first.address)).to.equal(3);
    expect(await f.pool.earned(second.address)).to.equal(6);

    await mined(f.pool.connect(first).claim());
    await mined(f.pool.connect(second).claim());
    expect(await f.pool.accountedRewardBalance()).to.equal(1);
    expect(await ethers.provider.getBalance(f.pool.address)).to.equal(4);

    await expect(f.pool.connect(first).claim()).to.be.revertedWith(
      "INNBC Community Reward: zero reward"
    );
  });

  it("preserves rewards and principal on payment failure and blocks reentry", async function () {
    const f = await financeFixture();
    const receiver = await deploy("INNBCAuditReceiver");

    await mined(receiver.stakeIn(f.pool.address, { value: 100 }));
    await mined(f.outsider.sendTransaction({ to: f.pool.address, value: 50 }));
    await mined(receiver.configure(true, false));

    await expect(receiver.claimFromPool()).to.be.revertedWith(
      "INNBC Community Reward: native transfer failed"
    );
    expect(await f.pool.earned(receiver.address)).to.equal(50);
    expect(await f.pool.stakedBalance(receiver.address)).to.equal(100);

    await expect(receiver.unstakeFromPool(100)).to.be.revertedWith(
      "INNBC Community Reward: native transfer failed"
    );
    expect(await f.pool.totalStaked()).to.equal(100);
    expect(await ethers.provider.getBalance(f.pool.address)).to.equal(150);

    await mined(receiver.configure(false, true));
    await mined(receiver.claimFromPool());
    expect(await receiver.reentryAttempted()).to.equal(true);
    expect(await receiver.reentrySucceeded()).to.equal(false);
    expect(await ethers.provider.getBalance(receiver.address)).to.equal(50);
    expect(await f.pool.earned(receiver.address)).to.equal(0);

    await mined(receiver.configure(false, false));
    await mined(receiver.unstakeFromPool(100));
    expect(await ethers.provider.getBalance(receiver.address)).to.equal(150);
    expect(await f.pool.totalStaked()).to.equal(0);
  });
});

describe("INNBC audit: burn vault dependency failures", function () {
  beforeEach(resetAuditChain);

  it("rejects empty burns and preserves funds when the token rejects", async function () {
    const f = await financeFixture();

    await expect(f.vault.burn()).to.be.revertedWith(
      "INNBC Fee Burn Vault: zero balance"
    );

    await mined(f.outsider.sendTransaction({ to: f.vault.address, value: 100 }));
    await mined(f.token.setFailBurn(true));

    await expect(f.vault.burn()).to.be.revertedWith(
      "INNBC Fee Burn Vault: burn failed"
    );
    expect(await ethers.provider.getBalance(f.vault.address)).to.equal(100);
  });
});

/* FINISH ALPHA CHANGE 2402 -- test treasury reward accounting and atomic failure handling -- */